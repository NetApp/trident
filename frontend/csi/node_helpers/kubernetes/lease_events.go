// Copyright 2026 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"context"

	coordinationv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/locks/distlock"
)

const leaseEventComponent = "trident-csi-node"

// leaseObjectEventRecorder posts lease lifecycle events onto the Kubernetes Lease
// that backs the distributed lock. It implements distlock.EventRecorder.
type leaseObjectEventRecorder struct {
	recorder record.EventRecorder
}

var _ distlock.EventRecorder = &leaseObjectEventRecorder{}

func (r *leaseObjectEventRecorder) Event(
	ctx context.Context, lease *coordinationv1.Lease, eventType, reason, message string,
) {
	if r == nil || r.recorder == nil || lease == nil {
		return
	}

	Logc(ctx).WithFields(LogFields{
		"lease":     lease.Namespace + "/" + lease.Name,
		"leaseUID":  string(lease.UID),
		"eventType": eventType,
		"reason":    reason,
		"message":   message,
	}).Debug("Distributed lock lease event.")

	r.recorder.Event(lease, mapLeaseEventType(eventType), reason, message)
}

func mapLeaseEventType(eventType string) string {
	switch eventType {
	case distlock.EventTypeNormal:
		return v1.EventTypeNormal
	case distlock.EventTypeWarning:
		return v1.EventTypeWarning
	default:
		// Kubernetes Events only support Normal and Warning (no Error type).
		return v1.EventTypeWarning
	}
}

func (h *helper) ensureEventRecorder() {
	if h == nil || h.k8sClients == nil || h.k8sClients.KubeClient == nil {
		return
	}

	h.eventRecorderOnce.Do(func() {
		if h.eventRecorder != nil {
			return
		}
		broadcaster := record.NewBroadcaster()
		broadcaster.StartRecordingToSink(&corev1client.EventSinkImpl{
			Interface: h.k8sClients.KubeClient.CoreV1().Events(""),
		})
		h.eventRecorder = broadcaster.NewRecorder(
			scheme.Scheme,
			v1.EventSource{Component: leaseEventComponent},
		)
	})
}
