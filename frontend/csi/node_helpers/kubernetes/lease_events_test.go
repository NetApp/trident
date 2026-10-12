// Copyright 2026 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"

	clik8sclient "github.com/netapp/trident/cli/k8s_client"
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/pkg/locks/distlock"
	"github.com/netapp/trident/utils/models"
)

func TestLeaseObjectEventRecorder_Event(t *testing.T) {
	fakeRec := record.NewFakeRecorder(10)
	r := &leaseObjectEventRecorder{recorder: fakeRec}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "luks-lock-1",
			Namespace: "trident",
			UID:       "uid-123",
		},
	}
	r.Event(context.Background(), lease, distlock.EventTypeWarning, distlock.ReasonLeaseRetained, "sticky fence")

	select {
	case got := <-fakeRec.Events:
		assert.Contains(t, got, distlock.ReasonLeaseRetained)
		assert.Contains(t, got, "sticky fence")
		assert.Contains(t, got, "Warning")
	case <-time.After(2 * time.Second):
		t.Fatal("expected event on Lease object")
	}
}

func TestLeaseObjectEventRecorder_NilSafe(t *testing.T) {
	r := &leaseObjectEventRecorder{}
	assert.NotPanics(t, func() {
		r.Event(context.Background(), &coordinationv1.Lease{}, distlock.EventTypeWarning, "r", "m")
	})

	r = &leaseObjectEventRecorder{recorder: nil}
	assert.NotPanics(t, func() {
		r.Event(context.Background(), &coordinationv1.Lease{}, distlock.EventTypeWarning, "r", "m")
	})
}

func TestEnsureEventRecorder_CreatesRecorder(t *testing.T) {
	h := newLockForHelper(t)
	h.k8sClients = &clik8sclient.Clients{
		Namespace:  "trident",
		KubeClient: &kubernetes.Clientset{},
	}

	h.ensureEventRecorder()
	require.NotNil(t, h.eventRecorder)
}

func TestLockFor_RWXLUKS_WiresEventRecorder(t *testing.T) {
	h := newLockForHelper(t)
	h.k8sClients = &clik8sclient.Clients{
		Namespace:  "trident",
		KubeClient: &kubernetes.Clientset{},
	}
	fakeRec := record.NewFakeRecorder(10)
	h.eventRecorder = fakeRec

	locker, err := h.LockFor(context.Background(), "luks-lock-1", "node-a", rwxLUKSPublishInfo())
	require.NoError(t, err)
	assert.IsType(t, &distlock.LeaseLock{}, locker)
}

func TestLockFor_NonRWX_NoopEmitsNoLeaseEvents(t *testing.T) {
	h := newLockForHelper(t)
	publishInfo := &models.VolumePublishInfo{
		LUKSEncryption: "false",
		VolumeMode:     string(config.Filesystem),
	}
	locker, err := h.LockFor(context.Background(), "vol-1", "host-1", publishInfo)
	require.NoError(t, err)
	assert.IsType(t, &distlock.NoopLock{}, locker)
}
