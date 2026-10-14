/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reconciler

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrlpredicate "sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"
	cosiproto "sigs.k8s.io/container-object-storage-interface/proto"

	cosierr "github.com/netapp/trident/cosi/third_party/errors"
	cosipredicate "github.com/netapp/trident/cosi/third_party/predicate"
	"github.com/netapp/trident/cosi/third_party/protocol"
	"github.com/netapp/trident/cosi/third_party/translator"
)

// BucketReconciler reconciles a Bucket object
type BucketReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	DriverInfo DriverInfo
}

// +kubebuilder:rbac:groups=objectstorage.k8s.io,resources=buckets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=objectstorage.k8s.io,resources=buckets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=objectstorage.k8s.io,resources=buckets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *BucketReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx, "driverName", r.DriverInfo.Name)

	bucket := &cosiapi.Bucket{}
	if err := r.Get(ctx, req.NamespacedName, bucket); err != nil {
		if kerrors.IsNotFound(err) {
			logger.V(1).Info("not reconciling nonexistent Bucket")
			return ctrl.Result{}, nil
		}
		// no resource to add status to or report an event for
		logger.Error(err, "failed to get Bucket")
		return ctrl.Result{}, err
	}

	err := r.reconcile(ctx, logger, bucket)
	if err != nil {
		// Record any error as a timestamped error in the status.
		if bucket.Status.ReadyToUse == nil {
			bucket.Status.ReadyToUse = ptr.To(false)
		}
		bucket.Status.Error = cosiapi.NewTimestampedError(time.Now(), err.Error())
		if updErr := r.Status().Update(ctx, bucket); updErr != nil {
			logger.Error(err, "failed to update Bucket status after reconcile error", "updateError", updErr)
			// If status update fails, we must retry the error regardless of the reconcile return.
			// The reconcile needs to run again to make sure the status is eventually updated.
			return reconcile.Result{}, err
		}

		if errors.Is(err, cosierr.NonRetryableError(nil)) {
			return reconcile.Result{}, reconcile.TerminalError(err)
		}
		return reconcile.Result{}, err
	}

	// On success, clear any errors in the status.
	if bucket.Status.Error != nil && !bucket.DeletionTimestamp.IsZero() {
		if bucket.Status.ReadyToUse == nil {
			bucket.Status.ReadyToUse = ptr.To(false)
		}
		bucket.Status.Error = nil
		if err := r.Status().Update(ctx, bucket); err != nil {
			logger.Error(err, "failed to update BucketClaim status after reconcile success")
			// Retry the reconcile so status can be updated eventually.
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *BucketReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cosiapi.Bucket{}).
		WithEventFilter(
			ctrlpredicate.And(
				driverNameMatchesPredicate(r.DriverInfo.Name), // only opt in to reconciles with matching driver name
				ctrlpredicate.Or(
					// this is the primary bucket controller and should reconcile ALL Create/Delete/Generic events
					cosipredicate.AnyCreate(),
					cosipredicate.AnyDelete(),
					cosipredicate.AnyGeneric(),
					// opt in to desired Update events
					cosipredicate.GenerationChangedInUpdateOnly(), // reconcile spec changes
					cosipredicate.DeletionTimestampAdded(),
					cosipredicate.ProtectionFinalizerRemoved(r.Scheme), // re-add protection finalizer if removed
				),
			),
		).
		Named("bucket").
		Complete(r)
}

func (r *BucketReconciler) reconcile(ctx context.Context, logger logr.Logger, bucket *cosiapi.Bucket) error {
	if bucket.Spec.DriverName != r.DriverInfo.Name {
		// keep this log to help debug any issues that might arise with predicate logic
		logger.Info("not reconciling bucket with non-matching driver name %q", bucket.Spec.DriverName)
		return nil
	}

	if !bucket.GetDeletionTimestamp().IsZero() {
		// NETAPP LOCAL: upstream's stub removes the protection finalizer and then errors, which
		// deletes the Bucket object and leaks the backend bucket. See cosi/sidecar/bucket_delete.go.
		return r.reconcileDelete(ctx, logger, bucket)
	}

	requiredProtos, err := objectProtocolListFromApiList(bucket.Spec.Protocols)
	if err != nil {
		logger.Error(err, "failed to parse protocol list")
		return cosierr.NonRetryableError(err)
	}

	if err := validateDriverSupportsProtocols(r.DriverInfo, requiredProtos); err != nil {
		logger.Error(err, "protocol(s) are unsupported")
		return cosierr.NonRetryableError(err)
	}

	isStaticProvisioning := bucket.Spec.ExistingBucketID != ""
	if isStaticProvisioning {
		logger = logger.WithValues("provisioningStrategy", "static")
	} else {
		logger = logger.WithValues("provisioningStrategy", "dynamic")
	}

	logger.V(1).Info("reconciling Bucket")

	didAdd := ctrlutil.AddFinalizer(bucket, cosiapi.ProtectionFinalizer)
	if didAdd {
		if err := r.Update(ctx, bucket); err != nil {
			logger.Error(err, "failed to add protection finalizer")
			return fmt.Errorf("failed to add protection finalizer: %w", err)
		}
	}

	var provisionedBucket *provisionedBucketDetails
	if isStaticProvisioning {
		provisionedBucket, err = r.staticProvision(ctx, logger, staticProvisionParams{
			existingBucketID: bucket.Spec.ExistingBucketID,
			requiredProtos:   requiredProtos,
			parameters:       bucket.Spec.Parameters,
			claimRef:         bucket.Spec.BucketClaimRef,
		})
	} else {
		provisionedBucket, err = r.dynamicProvision(ctx, logger, dynamicProvisionParams{
			bucketName:     bucket.Name,
			requiredProtos: requiredProtos,
			parameters:     bucket.Spec.Parameters,
			claimRef:       bucket.Spec.BucketClaimRef,
		})
	}
	if err != nil {
		return err
	}

	// final validation and status updates are the same for dynamic and static provisioning

	if len(provisionedBucket.supportedProtos) == 0 {
		logger.Error(nil, "created bucket supports no protocols")
		return cosierr.NonRetryableError(fmt.Errorf("created bucket supports no protocols"))
	}

	if err := validateBucketSupportsProtocols(provisionedBucket.supportedProtos, bucket.Spec.Protocols); err != nil {
		logger.Error(err, "bucket required protocols missing")
		return cosierr.NonRetryableError(fmt.Errorf("bucket required protocols missing: %w", err))
	}

	bucket.Status = cosiapi.BucketStatus{
		ReadyToUse: ptr.To(true),
		BucketID:   provisionedBucket.bucketId,
		Protocols:  provisionedBucket.supportedProtos,
		BucketInfo: provisionedBucket.allProtoBucketInfo,
		Error:      nil,
	}
	if err := r.Status().Update(ctx, bucket); err != nil {
		logger.Error(err, "failed to update Bucket status after successful bucket creation")
		return fmt.Errorf("failed to update Bucket status after successful bucket creation: %w", err)
	}

	return nil
}

// Details about provisioned bucket for both dynamic and static provisioning.
// A struct with named params allows for future expansion easily.
// When param lists get long, named fields help with readability, review, and maintenance.
type provisionedBucketDetails struct {
	bucketId           string
	supportedProtos    []cosiapi.ObjectProtocol
	allProtoBucketInfo map[string]string
}

// Parameters for dynamic provisioning workflow.
// A struct with named params allows for future expansion easily.
// When param lists get long, named fields help with readability, review, and maintenance.
type dynamicProvisionParams struct {
	bucketName     string
	requiredProtos []*cosiproto.ObjectProtocol
	parameters     map[string]string
	claimRef       cosiapi.BucketClaimReference
}

// Run dynamic provisioning workflow.
func (r *BucketReconciler) dynamicProvision(
	ctx context.Context,
	logger logr.Logger,
	dynamic dynamicProvisionParams,
) (
	details *provisionedBucketDetails,
	err error,
) {
	cr := dynamic.claimRef
	if cr.Name == "" || cr.Namespace == "" || cr.UID == "" {
		// likely a malformed bucket intended for static provisioning (possible COSI controller bug)
		logger.Error(nil, "all bucketClaimRef fields must be set for dynamic provisioning", "bucketClaimRef", cr)
		return nil, cosierr.NonRetryableError(
			fmt.Errorf("all bucketClaimRef fields must be set for dynamic provisioning: %#v", cr))
	}

	resp, err := r.DriverInfo.ProvisionerClient.DriverCreateBucket(ctx,
		&cosiproto.DriverCreateBucketRequest{
			Name:       dynamic.bucketName,
			Protocols:  dynamic.requiredProtos,
			Parameters: dynamic.parameters,
		},
	)
	if err != nil {
		logger.Error(err, "DriverCreateBucketRequest error")
		if rpcErrorIsRetryable(status.Code(err)) {
			return nil, err
		}
		return nil, cosierr.NonRetryableError(err)
	}

	if resp.BucketId == "" {
		logger.Error(nil, "created bucket ID missing")
		// driver behavior is unlikely to change if the request is retried
		return nil, cosierr.NonRetryableError(fmt.Errorf("created bucket ID missing"))
	}

	protoResp := resp.Protocols
	if protoResp == nil {
		logger.Error(nil, "created bucket protocol response missing")
		return nil, cosierr.NonRetryableError(fmt.Errorf("created bucket protocol response missing"))
	}

	var noValidation *translator.ValidationConfig = nil
	supportedProtos, allBucketInfo, err := translator.BucketInfoToApi(protoResp, noValidation)
	if err != nil {
		logger.Error(nil, "errors translating bucket info")
		return nil, cosierr.NonRetryableError(err)
	}

	details = &provisionedBucketDetails{
		bucketId:           resp.BucketId,
		supportedProtos:    supportedProtos,
		allProtoBucketInfo: allBucketInfo,
	}
	return details, nil
}

// Parameters for static provisioning workflow.
type staticProvisionParams struct {
	existingBucketID string
	requiredProtos   []*cosiproto.ObjectProtocol
	parameters       map[string]string
	claimRef         cosiapi.BucketClaimReference
}

// Run static provisioning workflow.
func (r *BucketReconciler) staticProvision(
	ctx context.Context,
	logger logr.Logger,
	static staticProvisionParams,
) (*provisionedBucketDetails, error) {
	ref := static.claimRef
	if ref.Name == "" || ref.Namespace == "" {
		logger.Error(nil, "bucketClaimRef namespace and name must be set for static provisioning", "bucketClaimRef", ref)
		return nil, cosierr.NonRetryableError(
			fmt.Errorf("bucketClaimRef namespace and name must be set for static provisioning: %#v", ref))
	}

	resp, err := r.DriverInfo.ProvisionerClient.DriverGetBucket(ctx,
		&cosiproto.DriverGetBucketRequest{
			BucketId:   static.existingBucketID,
			Protocols:  static.requiredProtos,
			Parameters: static.parameters,
		},
	)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			err = fmt.Errorf("waiting for backend bucket to exist: %w", err)
			logger.Error(err, "DriverGetBucket error")
			return nil, err
		}

		logger.Error(err, "DriverGetBucket error")
		if rpcErrorIsRetryable(status.Code(err)) {
			return nil, err
		}
		return nil, cosierr.NonRetryableError(err)
	}

	if resp.BucketId == "" {
		logger.Error(nil, "existing bucket ID missing in response")
		return nil, cosierr.NonRetryableError(fmt.Errorf("existing bucket ID missing in response"))
	}

	protoResp := resp.Protocols
	if protoResp == nil {
		logger.Error(nil, "existing bucket protocol response missing")
		return nil, cosierr.NonRetryableError(fmt.Errorf("existing bucket protocol response missing"))
	}

	var noValidation *translator.ValidationConfig = nil
	supportedProtos, allBucketInfo, err := translator.BucketInfoToApi(protoResp, noValidation)
	if err != nil {
		logger.Error(err, "errors translating existing bucket info")
		return nil, cosierr.NonRetryableError(err)
	}

	return &provisionedBucketDetails{
		bucketId:           resp.BucketId,
		supportedProtos:    supportedProtos,
		allProtoBucketInfo: allBucketInfo,
	}, nil
}

// convert an API proto list into an RPC proto message list
func objectProtocolListFromApiList(apiList []cosiapi.ObjectProtocol) ([]*cosiproto.ObjectProtocol, error) {
	errs := []error{}
	out := []*cosiproto.ObjectProtocol{}

	for _, apiProto := range apiList {
		rpcProto, err := protocol.ObjectProtocolTranslator{}.ApiToRpc(apiProto)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, &cosiproto.ObjectProtocol{
			Type: rpcProto,
		})
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("failed to parse protocol list: %w", errors.Join(errs...))
	}
	return out, nil
}

// validate that the required protocols (if given) are supported by the driver
func validateDriverSupportsProtocols(driver DriverInfo, required []*cosiproto.ObjectProtocol) error {
	unsupportedProtos := []string{}

	for _, proto := range required {
		if !driver.SupportsProtocol(proto.Type) {
			unsupportedProtos = append(unsupportedProtos, proto.Type.String())
		}
	}

	if len(unsupportedProtos) > 0 {
		return fmt.Errorf("driver %q does not support protocols: %v", driver.Name, unsupportedProtos)
	}
	return nil
}

// validate the required protocols (if given) are in the supported list (from bucket provisioning results)
func validateBucketSupportsProtocols(supported, required []cosiapi.ObjectProtocol) error {
	unsupported := []string{}
	for _, req := range required {
		if !slices.Contains(supported, req) {
			unsupported = append(unsupported, string(req))
		}
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("required protocols are not supported: %v", unsupported)
	}
	return nil
}
