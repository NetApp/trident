// Copyright 2026 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	tridentfake "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
	"github.com/netapp/trident/pkg/convert"
)

func TestMergeTridentNodeRegistrationSpec_preservesProvisionerReady(t *testing.T) {
	t.Parallel()

	existing := &tridentv1.TridentNode{
		Spec: tridentv1.TridentNodeSpec{
			NodeName:         "worker-1",
			IQN:              "iqn.old",
			ProvisionerReady: convert.ToPtr(true),
		},
		Status: tridentv1.TridentNodeStatus{
			PublicationState: "cleanable",
		},
	}
	identity := &tridentv1.TridentNode{
		Spec: tridentv1.TridentNodeSpec{
			NodeName:         "worker-1",
			IQN:              "iqn.new",
			ProvisionerReady: convert.ToPtr(false),
			IPs:              []string{"10.0.0.1"},
		},
	}

	mergeTridentNodeRegistrationSpec(existing, identity)

	require.NotNil(t, existing.Spec.ProvisionerReady)
	assert.True(t, *existing.Spec.ProvisionerReady, "re-registration must not reset cleanup-complete signal")
	assert.Equal(t, "iqn.new", existing.Spec.IQN)
	assert.Equal(t, []string{"10.0.0.1"}, existing.Spec.IPs)
	assert.Equal(t, "cleanable", existing.Status.PublicationState)
}

func TestMergeTridentNodeRegistrationSpec_preservesNodePrepWhenIdentityEmpty(t *testing.T) {
	t.Parallel()

	existing := &tridentv1.TridentNode{
		Spec: tridentv1.TridentNodeSpec{
			NodePrep: k8sruntime.RawExtension{Raw: []byte(`{"enabled":true}`)},
		},
	}
	identity := &tridentv1.TridentNode{
		Spec: tridentv1.TridentNodeSpec{
			NodeName: "worker-1",
		},
	}

	mergeTridentNodeRegistrationSpec(existing, identity)

	assert.JSONEq(t, `{"enabled":true}`, string(existing.Spec.NodePrep.Raw))
}

func TestSyncTridentNodeRootFieldsFromSpec_mirrorsMergedSpec(t *testing.T) {
	t.Parallel()

	existing := &tridentv1.TridentNode{
		Spec: tridentv1.TridentNodeSpec{
			NodeName:         "worker-1",
			IQN:              "iqn.new",
			ProvisionerReady: convert.ToPtr(true),
			NodePrep:         k8sruntime.RawExtension{Raw: []byte(`{"enabled":true}`)},
			HostInfo:         k8sruntime.RawExtension{Raw: []byte(`{"os":{"distro":"rhel"}}`)},
		},
		NodeName: "worker-old",
		IQN:      "iqn.old",
		NodePrep: k8sruntime.RawExtension{Raw: []byte(`{"enabled":false}`)},
	}
	identity := &tridentv1.TridentNode{
		Spec: tridentv1.TridentNodeSpec{
			NodeName: "worker-1",
			IQN:      "iqn.discovered",
		},
	}

	mergeTridentNodeRegistrationSpec(existing, identity)
	syncTridentNodeRootFieldsFromSpec(existing)

	assert.Equal(t, existing.Spec.NodeName, existing.NodeName)
	assert.Equal(t, existing.Spec.IQN, existing.IQN)
	assert.Equal(t, existing.Spec.NodePrep, existing.NodePrep)
	assert.Equal(t, existing.Spec.HostInfo, existing.HostInfo)
	assert.JSONEq(t, `{"enabled":true}`, string(existing.NodePrep.Raw))
}

func TestIsRegistrationComplete(t *testing.T) {
	t.Parallel()

	acked := &tridentv1.TridentNode{
		ObjectMeta: metav1.ObjectMeta{Generation: 2},
		Status: tridentv1.TridentNodeStatus{
			Registered:         true,
			ObservedGeneration: 2,
		},
	}

	tests := map[string]struct {
		node *tridentv1.TridentNode
		want bool
	}{
		"nil node": {node: nil, want: false},
		"registered false": {
			node: &tridentv1.TridentNode{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Status:     tridentv1.TridentNodeStatus{Registered: false, ObservedGeneration: 1},
			},
			want: false,
		},
		"observed generation behind": {
			node: &tridentv1.TridentNode{
				ObjectMeta: metav1.ObjectMeta{Generation: 3},
				Status:     tridentv1.TridentNodeStatus{Registered: true, ObservedGeneration: 2},
			},
			want: false,
		},
		"acknowledged": {node: acked, want: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, isRegistrationComplete(tc.node))
		})
	}
}

func testRegistrationTridentNode(name, ns string, generation int64, acked bool) *tridentv1.TridentNode {
	node := &tridentv1.TridentNode{
		TypeMeta: metav1.TypeMeta{APIVersion: "trident.netapp.io/v1", Kind: "TridentNode"},
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  ns,
			Generation: generation,
		},
		Spec: tridentv1.TridentNodeSpec{NodeName: name},
	}
	if acked {
		node.Status = tridentv1.TridentNodeStatus{
			Registered:         true,
			ObservedGeneration: generation,
		}
	}
	return node
}

func withFastRegistrationInfoPoll(t *testing.T) {
	t.Helper()
	origBase, origJitter, origSync := backupAckPollBase, backupAckPollJitter, ackInformerSyncTimeout
	backupAckPollBase = time.Millisecond
	backupAckPollJitter = 0
	ackInformerSyncTimeout = time.Millisecond
	t.Cleanup(func() {
		backupAckPollBase = origBase
		backupAckPollJitter = origJitter
		ackInformerSyncTimeout = origSync
	})
}

func withInformerSyncDisabled(t *testing.T) {
	t.Helper()
	origSync := ackInformerSyncTimeout
	ackInformerSyncTimeout = 0
	t.Cleanup(func() { ackInformerSyncTimeout = origSync })
}

func withSlowBackupAckPoll(t *testing.T) {
	t.Helper()
	origBase, origJitter := backupAckPollBase, backupAckPollJitter
	backupAckPollBase = time.Hour
	backupAckPollJitter = 0
	t.Cleanup(func() {
		backupAckPollBase = origBase
		backupAckPollJitter = origJitter
	})
}

func enqueueAckIfAcknowledged(ackCheck chan *tridentv1.TridentNode, nodeCR *tridentv1.TridentNode) {
	if isRegistrationComplete(nodeCR) {
		select {
		case ackCheck <- nodeCR:
		default:
		}
	}
}

func TestAckCheck_unackedEventsDoNotFillBuffer(t *testing.T) {
	t.Parallel()

	ackCheck := make(chan *tridentv1.TridentNode, 1)
	unacked := testRegistrationTridentNode("worker-1", "trident", 1, false)

	for range 5 {
		enqueueAckIfAcknowledged(ackCheck, unacked)
	}

	select {
	case extra := <-ackCheck:
		t.Fatalf("unacked events must not fill ackCheck buffer: %+v", extra)
	default:
	}
}

func TestAckCheck_onlyEnqueuesAcknowledgedSnapshots(t *testing.T) {
	t.Parallel()

	ackCheck := make(chan *tridentv1.TridentNode, 1)
	unacked := testRegistrationTridentNode("worker-1", "trident", 1, false)
	acked := testRegistrationTridentNode("worker-1", "trident", 1, true)

	enqueueAckIfAcknowledged(ackCheck, unacked)
	enqueueAckIfAcknowledged(ackCheck, acked)

	got := <-ackCheck
	require.True(t, isRegistrationComplete(got))

	select {
	case extra := <-ackCheck:
		t.Fatalf("unexpected extra snapshot in ackCheck: %+v", extra)
	default:
	}
}

func TestWaitForRegistrationInfo_unackedDoesNotBlockAck(t *testing.T) {
	withFastRegistrationInfoPoll(t)
	withSlowBackupAckPoll(t)

	const ns = "trident"
	pending := testRegistrationTridentNode("worker-1", ns, 1, false)
	acked := pending.DeepCopy()
	acked.Status = tridentv1.TridentNodeStatus{
		Registered:         true,
		ObservedGeneration: pending.Generation,
	}

	tridentClient := tridentfake.NewSimpleClientset(pending.DeepCopy())
	c := &Client{
		tridentClient:    tridentClient,
		tridentNamespace: ns,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	type result struct {
		ack *tridentv1.TridentNode
		err error
	}
	done := make(chan result, 1)
	start := time.Now()
	go func() {
		ack, err := c.waitForRegistrationInfo(ctx, pending, 0)
		done <- result{ack: ack, err: err}
	}()

	// Wait for informer sync, then apply the ack update. Unacked AddFunc events must
	// not fill ackCheck; only the acknowledged UpdateFunc enqueues the snapshot.
	// withSlowBackupAckPoll ensures a blocked ackCheck cannot be rescued within this
	// timeout (backup poll interval is one hour).
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		_, getErr := tridentClient.TridentV1().TridentNodes(ns).UpdateStatus(
			ctx, acked, metav1.UpdateOptions{},
		)
		if getErr == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	select {
	case res := <-done:
		require.NoError(t, res.err)
		require.NotNil(t, res.ack)
		assert.True(t, isRegistrationComplete(res.ack))
		assert.Less(t, time.Since(start), time.Second,
			"waitForRegistrationInfo should return promptly once acked; blocked ackCheck would time out with slow backup poll")
	case <-ctx.Done():
		t.Fatal("timed out waiting for registration ack; unacked snapshots may have blocked ackCheck")
	}
}

func TestWaitForRegistrationInfo_informerAcknowledged(t *testing.T) {
	withFastRegistrationInfoPoll(t)

	const ns = "trident"
	node := testRegistrationTridentNode("worker-1", ns, 1, true)

	c := &Client{
		tridentClient:    tridentfake.NewSimpleClientset(node.DeepCopy()),
		tridentNamespace: ns,
	}

	ack, err := c.waitForRegistrationInfo(context.Background(), node, 0)
	require.NoError(t, err)
	require.NotNil(t, ack)
	assert.True(t, isRegistrationComplete(ack))
}

func TestWaitForRegistrationInfo_pollAcknowledged(t *testing.T) {
	withFastRegistrationInfoPoll(t)

	const ns = "trident"
	pending := testRegistrationTridentNode("worker-1", ns, 1, false)
	acked := pending.DeepCopy()
	acked.Status = tridentv1.TridentNodeStatus{
		Registered:         true,
		ObservedGeneration: pending.Generation,
	}

	// Keep the pending object out of the informer cache so backup GET polling is exercised
	// instead of a stream of unacknowledged watch events starving the poll timer.
	tridentClient := tridentfake.NewSimpleClientset()
	tridentClient.Fake.PrependReactor("get", "tridentnodes", func(_ k8stesting.Action) (bool, k8sruntime.Object, error) {
		return true, acked.DeepCopy(), nil
	})

	c := &Client{
		tridentClient:    tridentClient,
		tridentNamespace: ns,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	ack, err := c.waitForRegistrationInfo(ctx, pending, 0)
	require.NoError(t, err)
	require.NotNil(t, ack)
	assert.True(t, isRegistrationComplete(ack))
}

func TestWaitForRegistrationInfo_immediatePollWhenInformerSyncFails(t *testing.T) {
	withInformerSyncDisabled(t)
	withSlowBackupAckPoll(t)

	const ns = "trident"
	pending := testRegistrationTridentNode("worker-1", ns, 1, false)
	acked := pending.DeepCopy()
	acked.Status = tridentv1.TridentNodeStatus{
		Registered:         true,
		ObservedGeneration: pending.Generation,
	}

	tridentClient := tridentfake.NewSimpleClientset()
	tridentClient.Fake.PrependReactor("get", "tridentnodes", func(_ k8stesting.Action) (bool, k8sruntime.Object, error) {
		return true, acked.DeepCopy(), nil
	})

	c := &Client{
		tridentClient:    tridentClient,
		tridentNamespace: ns,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	ack, err := c.waitForRegistrationInfo(ctx, pending, 0)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, ack)
	assert.True(t, isRegistrationComplete(ack))
	assert.Less(t, elapsed, time.Second, "first backup GET should not wait for the slow poll interval")
}

func TestWaitForRegistrationInfo_contextCanceled(t *testing.T) {
	const ns = "trident"
	pending := testRegistrationTridentNode("worker-1", ns, 1, false)

	c := &Client{
		tridentClient:    tridentfake.NewSimpleClientset(pending.DeepCopy()),
		tridentNamespace: ns,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	ack, err := c.waitForRegistrationInfo(ctx, pending, 0)
	require.Error(t, err)
	assert.Nil(t, ack)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestWaitForRegistrationInfo_pollGetErrorRetriesUntilCanceled(t *testing.T) {
	withFastRegistrationInfoPoll(t)

	const ns = "trident"
	pending := testRegistrationTridentNode("worker-1", ns, 1, false)

	tridentClient := tridentfake.NewSimpleClientset(pending.DeepCopy())
	tridentClient.Fake.PrependReactor("get", "tridentnodes", func(_ k8stesting.Action) (bool, k8sruntime.Object, error) {
		return true, nil, errors.New("apiserver unavailable")
	})

	c := &Client{
		tridentClient:    tridentClient,
		tridentNamespace: ns,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	ack, err := c.waitForRegistrationInfo(ctx, pending, 0)
	require.Error(t, err)
	assert.Nil(t, ack)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
