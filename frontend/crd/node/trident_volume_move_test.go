// Copyright 2026 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ktesting "k8s.io/client-go/testing"

	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	faketridentclient "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

func fakeTVM(namespace, name string, attachments []*tridentv1.TridentVolumeMoveAttachmentStatus) *tridentv1.TridentVolumeMove {
	return &tridentv1.TridentVolumeMove{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: tridentv1.TridentVolumeMoveStatus{
			State:       models.VolumeMoveStateNodeStaging,
			Attachments: attachments,
		},
	}
}

// TestUpdateTridentVolumeMoveAttachmentStatus_DoesNotClobberSiblings verifies
// that when one node updates its own attachment entry, sibling nodes' entries
// are preserved because the function re-reads the resource from the API before
// writing.
func TestUpdateTridentVolumeMoveAttachmentStatus_DoesNotClobberSiblings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// API state: node-1 already Bridged, node-2 still Pending.
	tvm := fakeTVM(testNamespace, "test-tvm", []*tridentv1.TridentVolumeMoveAttachmentStatus{
		{NodeName: "node-1", State: models.VolumeMoveAttachmentStateBridged, Message: "bridged by node-1"},
		{NodeName: "node-2", State: models.VolumeMoveAttachmentStatePending},
	})

	crdClient := faketridentclient.NewSimpleClientset(tvm)
	controller := &TridentNodeCrdController{
		crdClientset: crdClient,
		nodeName:     "node-2",
	}

	// Simulate the handler having set node-2 to Bridged in the local (cache) copy.
	localTVM := tvm.DeepCopy()
	localAttach := localTVM.Status.AttachmentStatus("node-2")
	require.NotNil(t, localAttach)
	localAttach.State = models.VolumeMoveAttachmentStateBridged
	localAttach.Message = "bridged by node-2"

	err := controller.updateTridentVolumeMoveAttachmentStatus(ctx, localTVM)
	require.NoError(t, err)

	got, err := crdClient.TridentV1().TridentVolumeMoves(testNamespace).Get(ctx, "test-tvm", metav1.GetOptions{})
	require.NoError(t, err)

	node1 := got.Status.AttachmentStatus("node-1")
	require.NotNil(t, node1, "node-1 attachment must exist")
	assert.Equal(t, models.VolumeMoveAttachmentStateBridged, node1.State,
		"node-1 must not be clobbered by node-2's update")
	assert.Equal(t, "bridged by node-1", node1.Message)

	node2 := got.Status.AttachmentStatus("node-2")
	require.NotNil(t, node2, "node-2 attachment must exist")
	assert.Equal(t, models.VolumeMoveAttachmentStateBridged, node2.State)
	assert.Equal(t, "bridged by node-2", node2.Message)
}

// TestUpdateTridentVolumeMoveAttachmentStatus_RetriesOnConflict verifies that
// a 409 Conflict from the API server is retried transparently rather than
// propagated as an error.
func TestUpdateTridentVolumeMoveAttachmentStatus_RetriesOnConflict(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tvm := fakeTVM(testNamespace, "test-tvm", []*tridentv1.TridentVolumeMoveAttachmentStatus{
		{NodeName: "node-1", State: models.VolumeMoveAttachmentStatePending},
	})

	crdClient := faketridentclient.NewSimpleClientset(tvm)

	var updateStatusCalls int32
	crdClient.PrependReactor("update", "tridentvolumemoves", func(action ktesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "status" {
			return false, nil, nil
		}
		call := atomic.AddInt32(&updateStatusCalls, 1)
		if call == 1 {
			return true, nil, apierrors.NewConflict(
				schema.GroupResource{Group: "trident.netapp.io", Resource: "tridentvolumemoves"},
				"test-tvm",
				fmt.Errorf("the object has been modified"),
			)
		}
		return false, nil, nil // fall through to default handler
	})

	controller := &TridentNodeCrdController{
		crdClientset: crdClient,
		nodeName:     "node-1",
	}

	localTVM := tvm.DeepCopy()
	localAttach := localTVM.Status.AttachmentStatus("node-1")
	require.NotNil(t, localAttach)
	localAttach.State = models.VolumeMoveAttachmentStateBridged
	localAttach.Message = "bridged"

	err := controller.updateTridentVolumeMoveAttachmentStatus(ctx, localTVM)
	require.NoError(t, err, "should succeed after retrying the 409 conflict")

	assert.Equal(t, int32(2), atomic.LoadInt32(&updateStatusCalls),
		"UpdateStatus should be called twice: 1 conflict + 1 success")

	got, err := crdClient.TridentV1().TridentVolumeMoves(testNamespace).Get(ctx, "test-tvm", metav1.GetOptions{})
	require.NoError(t, err)

	node1 := got.Status.AttachmentStatus("node-1")
	require.NotNil(t, node1)
	assert.Equal(t, models.VolumeMoveAttachmentStateBridged, node1.State)
}

// TestUpdateTridentVolumeMoveAttachmentStatus_NotFound verifies that a
// NotFound error from the API (TVM deleted) is surfaced correctly and not
// retried.
func TestUpdateTridentVolumeMoveAttachmentStatus_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tvm := fakeTVM(testNamespace, "test-tvm", []*tridentv1.TridentVolumeMoveAttachmentStatus{
		{NodeName: "node-1", State: models.VolumeMoveAttachmentStatePending},
	})

	// Don't seed the fake — the TVM does not exist in the API.
	crdClient := faketridentclient.NewSimpleClientset()
	controller := &TridentNodeCrdController{
		crdClientset: crdClient,
		nodeName:     "node-1",
	}

	localTVM := tvm.DeepCopy()
	localAttach := localTVM.Status.AttachmentStatus("node-1")
	require.NotNil(t, localAttach)
	localAttach.State = models.VolumeMoveAttachmentStateBridged

	err := controller.updateTridentVolumeMoveAttachmentStatus(ctx, localTVM)
	require.Error(t, err)
	assert.True(t, errors.IsNotFoundError(err),
		"error should be a NotFoundError, got: %v", err)
}
