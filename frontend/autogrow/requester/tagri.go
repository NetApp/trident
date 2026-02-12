// Copyright 2026 NetApp, Inc. All Rights Reserved.

package requester

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autogrowTypes "github.com/netapp/trident/frontend/autogrow/types"
	. "github.com/netapp/trident/logging"
	v1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	utilsErrors "github.com/netapp/trident/utils/errors"
)

// getTAGRI retrieves an existing TAGRI if it exists
// Returns the TAGRI and error from the API call.
// Caller should check for k8serrors.IsNotFound(err) to determine if TAGRI doesn't exist.
func (r *Requester) getTAGRI(ctx context.Context, volumeID string) (*v1.TridentAutogrowRequestInternal, error) {
	name := generateTAGRIName(volumeID)
	tagri, err := r.tridentClient.TridentV1().TridentAutogrowRequestInternals(r.config.TridentNamespace).
		Get(ctx, name, metav1.GetOptions{})

	if err != nil {
		// Check specifically for rate limiting (HTTP 429)
		if k8serrors.IsTooManyRequests(err) {
			// Extract retry-after duration from the error
			var statusErr *k8serrors.StatusError
			if errors.As(err, &statusErr) {
				if statusErr.Status().Details != nil {
					if retryAfterSeconds := statusErr.Status().Details.RetryAfterSeconds; retryAfterSeconds > 0 {
						Logc(ctx).WithFields(LogFields{
							"retryAfter": retryAfterSeconds,
							"volumeID":   volumeID,
						}).Warn("TAGRI check rate limited by API server, will retry after specified duration")
						// Wrap the error with retry hint info
						return nil, utilsErrors.WrapWithReconcileDeferredError(
							fmt.Errorf("rate limited, retry after %d seconds: %w", retryAfterSeconds, err),
							"failed to check existing TAGRI",
						)
					}
				}
			}
		}

		// Return error as-is (including NotFound) - caller will handle
		return nil, err
	}

	// TAGRI exists - return it
	return tagri, nil
}

// createTAGRI creates a new TridentInternalAutogrowRequest CR
func (r *Requester) createTAGRI(
	ctx context.Context,
	event autogrowTypes.VolumeThresholdBreached,
	policy *v1.TridentAutogrowPolicy,
	volumeID string,
	observedUsedPercent float32,
) error {
	var err error

	tagri := &v1.TridentAutogrowRequestInternal{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateTAGRIName(volumeID),
			Namespace: r.config.TridentNamespace,
		},
		Spec: v1.TridentAutogrowRequestInternalSpec{
			Volume:                volumeID,
			ObservedUsedPercent:   observedUsedPercent,
			ObservedUsedBytes:     strconv.FormatInt(event.UsedSize, 10),
			ObservedCapacityBytes: strconv.FormatInt(event.CurrentTotalSize, 10),
			AutogrowPolicyRef: v1.TridentAutogrowRequestInternalPolicyRef{
				Name:       policy.Name,
				Generation: policy.Generation,
			},
			NodeName:  os.Getenv("KUBE_NODE_NAME"), // Set via downward API in DaemonSet
			Timestamp: metav1.NewTime(event.Timestamp),
		},
		Status: v1.TridentAutogrowRequestInternalStatus{
			Phase: string(v1.TridentAutogrowRequestInternalPending),
		},
	}

	_, err = r.tridentClient.TridentV1().
		TridentAutogrowRequestInternals(r.config.TridentNamespace).
		Create(ctx, tagri, metav1.CreateOptions{})

	if err != nil {
		// Check specifically for rate limiting (HTTP 429)
		if k8serrors.IsTooManyRequests(err) {
			// Extract retry-after duration from the error
			var statusErr *k8serrors.StatusError
			if errors.As(err, &statusErr) {
				if statusErr.Status().Details != nil {
					if retryAfterSeconds := statusErr.Status().Details.RetryAfterSeconds; retryAfterSeconds > 0 {
						Logc(ctx).WithFields(LogFields{
							"retryAfter": retryAfterSeconds,
							"tvpName":    event.TVPName,
							"volumeID":   volumeID,
						}).Warn("Autogrow request creation rate limited by API server, will retry after specified duration")

						// Wrap the error with retry hint info
						return utilsErrors.WrapWithReconcileDeferredError(
							fmt.Errorf("rate limited, retry after %d seconds: %w", retryAfterSeconds, err),
							"failed to create TAGRI",
						)
					}
				}
			}
		}
		// Wrap API errors with ReconcileDeferredError to trigger retry
		return utilsErrors.WrapWithReconcileDeferredError(err, "failed to create TAGRI")
	}

	Logc(ctx).WithFields(LogFields{
		"tvpName":             event.TVPName,
		"volumeID":            volumeID,
		"usedSize":            event.UsedSize,
		"currentSize":         event.CurrentTotalSize,
		"observedUsedPercent": observedUsedPercent,
	}).Debug("Created TAGRI with volume metadata")

	return nil
}

// generateTAGRIName creates a deterministic name based on volumeID
func generateTAGRIName(volumeID string) string {
	return fmt.Sprintf("tagri-%s", volumeID)
}
