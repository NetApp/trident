// Copyright 2026 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sversion "k8s.io/apimachinery/pkg/version"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"

	mockcore "github.com/netapp/trident/mocks/mock_core"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	tridentfake "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/utils/models"
)

func drainFakeRecorderEvents(recorder *record.FakeRecorder) []string {
	var events []string
	for {
		select {
		case event := <-recorder.Events:
			events = append(events, event)
		default:
			return events
		}
	}
}

func TestHelper_RegisterNode(t *testing.T) {
	ctx := context.Background()
	nodeName := "worker-1"

	mockCtrl := gomock.NewController(t)
	mockOrch := mockcore.NewMockOrchestrator(mockCtrl)

	mockOrch.EXPECT().AddNode(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, node *models.Node, _ func(string, string, string)) error {
			assert.Equal(t, map[string]string{"topology.kubernetes.io/region": "us-west"}, node.TopologyLabels)
			return nil
		})
	mockOrch.EXPECT().GetLogLevel(gomock.Any()).Return("debug", nil)
	mockOrch.EXPECT().GetSelectedLoggingWorkflows(gomock.Any()).Return("csi", nil)
	mockOrch.EXPECT().GetSelectedLogLayers(gomock.Any()).Return("all", nil)

	h := &helper{
		orchestrator: mockOrch,
		kubeClient: k8sfake.NewSimpleClientset(&v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   nodeName,
				Labels: map[string]string{"topology.kubernetes.io/region": "us-west"},
			},
		}),
		namespace:   "trident",
		kubeVersion: &k8sversion.Info{GitVersion: "v1.28.0"},
	}

	ack, err := h.RegisterNode(ctx, &models.Node{Name: nodeName}, nil)
	require.NoError(t, err)
	require.NotNil(t, ack)
	assert.Equal(t, map[string]string{"topology.kubernetes.io/region": "us-west"}, ack.TopologyLabels)
	assert.Equal(t, "debug", ack.LogLevel)
	assert.Equal(t, "csi", ack.LogWorkflows)
	assert.Equal(t, "all", ack.LogLayers)
}

func TestHelper_ReconcileTridentNode_LegacyUpdateVersusStatusUpdate(t *testing.T) {
	ctx := context.Background()
	ns := "trident"
	nodeName := "worker-1"
	name := netappv1.NameFix(nodeName)

	mockCtrl := gomock.NewController(t)
	mockOrch := mockcore.NewMockOrchestrator(mockCtrl)

	mockOrch.EXPECT().AddNode(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *models.Node, func(string, string, string)) error {
			return nil
		}).Times(2)
	mockOrch.EXPECT().GetLogLevel(gomock.Any()).Return("info", nil).Times(2)
	mockOrch.EXPECT().GetSelectedLoggingWorkflows(gomock.Any()).Return("all", nil).Times(2)
	mockOrch.EXPECT().GetSelectedLogLayers(gomock.Any()).Return("csi", nil).Times(2)

	kubeClient := k8sfake.NewSimpleClientset(&v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   nodeName,
			Labels: map[string]string{"topology.kubernetes.io/region": "us-west"},
		},
	})

	baseTN := &netappv1.TridentNode{
		TypeMeta: metav1.TypeMeta{APIVersion: "trident.netapp.io/v1", Kind: "TridentNode"},
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       ns,
			ResourceVersion: "1",
			Generation:      1,
		},
		Spec: netappv1.TridentNodeSpec{
			NodeName: nodeName,
		},
		Deleted:          false,
		PublicationState: string(models.NodeClean),
	}

	t.Run("consistent legacy fields skip non-status Update", func(t *testing.T) {
		var legacyUpdates, statusUpdates int
		tridentClient := tridentfake.NewSimpleClientset(baseTN.DeepCopy())
		tridentClient.Fake.PrependReactor("update", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
			if ua, ok := action.(k8stesting.UpdateActionImpl); ok {
				if ua.GetSubresource() == "status" {
					statusUpdates++
				} else {
					legacyUpdates++
				}
			}
			return false, nil, nil
		})

		h := &helper{
			orchestrator:  mockOrch,
			tridentClient: tridentClient,
			kubeClient:    kubeClient,
			namespace:     ns,
			kubeVersion:   &k8sversion.Info{GitVersion: "v1.28.0"},
		}

		nodeCR := baseTN.DeepCopy()
		err := h.ReconcileTridentNode(ctx, nodeCR)
		require.NoError(t, err)
		assert.Equal(t, 0, legacyUpdates, "expected no legacy TridentNode Update when CR is internally consistent")
		assert.Equal(t, 1, statusUpdates, "expected UpdateStatus for controller ack")
	})

	t.Run("legacy fields mismatch triggers Update then UpdateStatus", func(t *testing.T) {
		var legacyUpdates, statusUpdates int
		mismatch := baseTN.DeepCopy()
		// Status takes precedence in Persistent(); legacy PublicationState disagrees so reconcile should patch legacy.
		mismatch.Status.PublicationState = string(models.NodeDirty)
		mismatch.PublicationState = string(models.NodeClean)

		tridentClient := tridentfake.NewSimpleClientset(mismatch.DeepCopy())
		tridentClient.Fake.PrependReactor("update", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
			if ua, ok := action.(k8stesting.UpdateActionImpl); ok {
				if ua.GetSubresource() == "status" {
					statusUpdates++
				} else {
					legacyUpdates++
				}
			}
			return false, nil, nil
		})

		h := &helper{
			orchestrator:  mockOrch,
			tridentClient: tridentClient,
			kubeClient:    kubeClient,
			namespace:     ns,
			kubeVersion:   &k8sversion.Info{GitVersion: "v1.28.0"},
		}

		err := h.ReconcileTridentNode(ctx, mismatch)
		require.NoError(t, err)
		assert.Equal(t, 1, legacyUpdates, "expected legacy TridentNode Update when status and legacy publication state disagree")
		assert.Equal(t, 1, statusUpdates, "expected UpdateStatus for controller ack")
	})
}

func TestHelper_ReconcileTridentNode_SpecNodeNameMismatch(t *testing.T) {
	ctx := context.Background()
	ns := "trident"
	nodeName := "worker-1"
	otherNodeName := "other-worker"
	name := netappv1.NameFix(nodeName)

	tridentClient := tridentfake.NewSimpleClientset()
	mismatch := &netappv1.TridentNode{
		TypeMeta: metav1.TypeMeta{APIVersion: "trident.netapp.io/v1", Kind: "TridentNode"},
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  ns,
			Generation: 1,
		},
		Spec: netappv1.TridentNodeSpec{
			NodeName: otherNodeName,
		},
	}
	_, err := tridentClient.TridentV1().TridentNodes(ns).Create(ctx, mismatch, metav1.CreateOptions{})
	require.NoError(t, err)

	recorder := record.NewFakeRecorder(4)
	h := &helper{
		tridentClient: tridentClient,
		kubeClient: k8sfake.NewSimpleClientset(
			&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}},
			&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: otherNodeName}},
		),
		namespace:     ns,
		kubeVersion:   &k8sversion.Info{GitVersion: "v1.28.0"},
		eventRecorder: recorder,
	}

	err = h.ReconcileTridentNode(ctx, mismatch)
	require.NoError(t, err)

	updated, getErr := tridentClient.TridentV1().TridentNodes(ns).Get(ctx, name, metav1.GetOptions{})
	require.NoError(t, getErr)
	assert.False(t, updated.Status.Registered)
	assert.Equal(t, int64(1), updated.Status.ObservedGeneration)

	events := drainFakeRecorderEvents(recorder)
	require.Len(t, events, 2, "expected events on TridentNode identity node and mismatched spec.nodeName")
	for _, event := range events {
		assert.Contains(t, event, tridentNodeRegistrationRejectedReason)
		assert.Contains(t, event, `spec.nodeName "other-worker" does not match TridentNode metadata.name`)
	}
}

func TestHelper_ReconcileTridentNode_SpecNodeNameMismatch_DedupesEvents(t *testing.T) {
	ctx := context.Background()
	ns := "trident"
	nodeName := "worker-1"
	name := netappv1.NameFix(nodeName)

	tridentClient := tridentfake.NewSimpleClientset()
	mismatch := &netappv1.TridentNode{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Generation: 1},
		Spec:       netappv1.TridentNodeSpec{NodeName: "other-worker"},
	}
	_, err := tridentClient.TridentV1().TridentNodes(ns).Create(ctx, mismatch, metav1.CreateOptions{})
	require.NoError(t, err)

	recorder := record.NewFakeRecorder(8)
	h := &helper{
		tridentClient: tridentClient,
		kubeClient: k8sfake.NewSimpleClientset(
			&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}},
			&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "other-worker"}},
		),
		namespace:     ns,
		kubeVersion:   &k8sversion.Info{GitVersion: "v1.28.0"},
		eventRecorder: recorder,
	}

	require.NoError(t, h.ReconcileTridentNode(ctx, mismatch))
	require.NoError(t, h.ReconcileTridentNode(ctx, mismatch))

	events := drainFakeRecorderEvents(recorder)
	assert.Len(t, events, 2, "repeated reconcile should not emit duplicate rejection events")
}

func TestTridentNodeRegistrationRejectedEventTargets(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"worker-1"}, tridentNodeRegistrationRejectedEventTargets("worker-1", "worker-1"))
	assert.Equal(t,
		[]string{"worker-1", "other-worker"},
		tridentNodeRegistrationRejectedEventTargets("worker-1", "other-worker"),
	)
}

func TestRecordTridentNodeRegistrationRejected_NilRecorder(t *testing.T) {
	t.Parallel()

	h := &helper{}
	assert.NotPanics(t, func() {
		h.recordTridentNodeRegistrationRejected(
			context.Background(), "worker-1", "other-worker", "name mismatch",
		)
	})
}

func TestTridentNodeRegistrationRejectedEventKey(t *testing.T) {
	t.Parallel()

	key := tridentNodeRegistrationRejectedEventKey("worker-1", "other-worker", "detail")
	assert.True(t, strings.Contains(key, "worker-1"))
	assert.True(t, strings.Contains(key, "other-worker"))
	assert.True(t, strings.Contains(key, "detail"))
}

func TestHelper_tridentNodePublicationStateFlags(t *testing.T) {
	ctx := context.Background()
	nodeName := "worker-1"
	provisionerReady := true

	kubeClient := k8sfake.NewSimpleClientset(&v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		Status:     v1.NodeStatus{Conditions: []v1.NodeCondition{nodeReadyConditionTrue}},
	})

	h := &helper{
		kubeClient:        kubeClient,
		tridentClient:     tridentfake.NewSimpleClientset(),
		namespace:         "trident",
		enableForceDetach: true,
	}

	flags, err := h.tridentNodePublicationStateFlags(ctx, nodeName, &netappv1.TridentNode{
		Spec: netappv1.TridentNodeSpec{ProvisionerReady: convert.ToPtr(provisionerReady)},
	})
	require.NoError(t, err)
	require.NotNil(t, flags)
	require.NotNil(t, flags.OrchestratorReady)
	require.NotNil(t, flags.AdministratorReady)
	require.NotNil(t, flags.ProvisionerReady)
	assert.True(t, *flags.OrchestratorReady)
	assert.True(t, *flags.AdministratorReady)
	assert.True(t, *flags.ProvisionerReady)

	flags, err = h.tridentNodePublicationStateFlags(ctx, nodeName, &netappv1.TridentNode{})
	require.NoError(t, err)
	assert.Nil(t, flags.ProvisionerReady)
}

func TestHelper_ReconcileTridentNode_ProvisionerReadyForceDetach(t *testing.T) {
	ctx := context.Background()
	ns := "trident"
	nodeName := "worker-1"
	name := netappv1.NameFix(nodeName)

	mockCtrl := gomock.NewController(t)
	mockOrch := mockcore.NewMockOrchestrator(mockCtrl)

	expectedFlags := &models.NodePublicationStateFlags{
		OrchestratorReady:  new(true),
		AdministratorReady: new(true),
		ProvisionerReady:   new(true),
	}

	mockOrch.EXPECT().AddNode(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mockOrch.EXPECT().UpdateNode(gomock.Any(), nodeName, expectedFlags).Return(nil)
	mockOrch.EXPECT().GetNode(gomock.Any(), nodeName).Return(&models.NodeExternal{
		Name:             nodeName,
		PublicationState: models.NodeClean,
	}, nil)
	mockOrch.EXPECT().GetLogLevel(gomock.Any()).Return("info", nil)
	mockOrch.EXPECT().GetSelectedLoggingWorkflows(gomock.Any()).Return("all", nil)
	mockOrch.EXPECT().GetSelectedLogLayers(gomock.Any()).Return("csi", nil)

	kubeClient := k8sfake.NewSimpleClientset(&v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   nodeName,
			Labels: map[string]string{"topology.kubernetes.io/region": "us-west"},
		},
		Status: v1.NodeStatus{Conditions: []v1.NodeCondition{nodeReadyConditionTrue}},
	})

	nodeCR := &netappv1.TridentNode{
		TypeMeta: metav1.TypeMeta{APIVersion: "trident.netapp.io/v1", Kind: "TridentNode"},
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       ns,
			ResourceVersion: "1",
			Generation:      1,
		},
		Spec: netappv1.TridentNodeSpec{
			NodeName:         nodeName,
			ProvisionerReady: convert.ToPtr(true),
		},
		PublicationState: string(models.NodeCleanable),
	}

	var legacyUpdate *netappv1.TridentNode
	tridentClient := tridentfake.NewSimpleClientset(nodeCR.DeepCopy())
	tridentClient.Fake.PrependReactor("update", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if ua, ok := action.(k8stesting.UpdateActionImpl); ok && ua.GetSubresource() == "" {
			legacyUpdate = ua.GetObject().(*netappv1.TridentNode)
		}
		return false, nil, nil
	})

	h := &helper{
		orchestrator:      mockOrch,
		tridentClient:     tridentClient,
		kubeClient:        kubeClient,
		namespace:         ns,
		enableForceDetach: true,
		kubeVersion:       &k8sversion.Info{GitVersion: "v1.28.0"},
	}

	err := h.ReconcileTridentNode(ctx, nodeCR)
	require.NoError(t, err)
	require.NotNil(t, legacyUpdate)
	assert.Equal(t, string(models.NodeClean), legacyUpdate.PublicationState)
	assert.Nil(t, legacyUpdate.Spec.ProvisionerReady, "expected controller to clear spec.provisionerReady after clean")

	updated, getErr := tridentClient.TridentV1().TridentNodes(ns).Get(ctx, name, metav1.GetOptions{})
	require.NoError(t, getErr)
	assert.Equal(t, string(models.NodeClean), updated.Status.PublicationState)
}

func TestHelper_updateTridentNodeStatusWithRetry_retriesConflict(t *testing.T) {
	ctx := context.Background()
	const ns = "trident"
	name := netappv1.NameFix("worker-1")

	nodeCR := &netappv1.TridentNode{
		TypeMeta: metav1.TypeMeta{APIVersion: "trident.netapp.io/v1", Kind: "TridentNode"},
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       ns,
			ResourceVersion: "1",
			Generation:      3,
		},
		Spec: netappv1.TridentNodeSpec{NodeName: "worker-1"},
	}

	tridentClient := tridentfake.NewSimpleClientset(nodeCR.DeepCopy())
	statusUpdateAttempts := 0
	tridentClient.Fake.PrependReactor("update", "tridentnodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		updateAction, ok := action.(k8stesting.UpdateActionImpl)
		if !ok || updateAction.GetSubresource() != "status" {
			return false, nil, nil
		}
		statusUpdateAttempts++
		if statusUpdateAttempts == 1 {
			return true, nil, apierrors.NewConflict(
				netappv1.Resource("tridentnodes"), name, fmt.Errorf("resourceVersion mismatch"),
			)
		}
		return false, nil, nil
	})

	h := &helper{
		tridentClient: tridentClient,
		namespace:     ns,
	}

	err := h.updateTridentNodeStatusWithRetry(ctx, ns, name, func(status *netappv1.TridentNodeStatus, generation int64) {
		status.Registered = true
		status.ObservedGeneration = generation
	})
	require.NoError(t, err)
	assert.Equal(t, 2, statusUpdateAttempts)

	updated, getErr := tridentClient.TridentV1().TridentNodes(ns).Get(ctx, name, metav1.GetOptions{})
	require.NoError(t, getErr)
	assert.True(t, updated.Status.Registered)
	assert.Equal(t, int64(3), updated.Status.ObservedGeneration)
}
