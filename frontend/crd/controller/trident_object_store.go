// Copyright 2026 NetApp, Inc. All Rights Reserved.

package controller

import (
	"context"
	"fmt"

	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/object"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/utils/errors"
)

func (c *TridentCrdController) handleTridentObjectStore(keyItem *KeyItem) error {
	if keyItem == nil {
		return errors.ReconcileDeferredError("keyItem is nil")
	}

	namespace, name, err := cache.SplitMetaNamespaceKey(keyItem.key)
	if err != nil {
		return fmt.Errorf("invalid object store key %q: %w", keyItem.key, err)
	}

	objectStore, err := c.objectStoresLister.TridentObjectStores(namespace).Get(name)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			return nil
		}
		return errors.WrapWithReconcileDeferredError(err, "could not get TridentObjectStore")
	}

	if !objectStore.DeletionTimestamp.IsZero() || keyItem.event == EventDelete {
		if err = c.orchestrator.DeleteObjectStore(keyItem.ctx, objectStore.Name); err != nil {
			return errors.WrapWithReconcileDeferredError(err, "could not delete object store")
		}
		// Nil means core finished deletion, including a cache miss with no buckets left.
		// A CR that never loaded (for example, its secret was missing) still needs its finalizer
		// removed here. A cache miss that still has buckets returns a conflict above.
		latest, err := c.crdClientset.TridentV1().TridentObjectStores(namespace).Get(keyItem.ctx, name, getOpts)
		if k8sapierrors.IsNotFound(err) {
			return nil
		} else if err != nil {
			return errors.WrapWithReconcileDeferredError(err, "could not get TridentObjectStore")
		}
		if latest.HasTridentFinalizers() {
			if err = c.deleteCreatedObjectStoreSecret(keyItem.ctx, latest); err != nil {
				Logx(keyItem.ctx).WithFields(LogFields{
					"name":      latest.Name,
					"namespace": latest.Namespace,
				}).WithError(err).Error("Could not delete object store secret.")
			}
			copy := latest.DeepCopy()
			copy.RemoveTridentFinalizers()
			_, err = c.crdClientset.TridentV1().TridentObjectStores(namespace).Update(keyItem.ctx, copy, updateOpts)
			if err != nil && !k8sapierrors.IsNotFound(err) {
				return errors.WrapWithReconcileDeferredError(err, "could not remove TridentObjectStore finalizer")
			}
		}
		return nil
	}

	if !objectStore.HasTridentFinalizers() {
		copy := objectStore.DeepCopy()
		copy.AddTridentFinalizers()
		objectStore, err = c.crdClientset.TridentV1().TridentObjectStores(namespace).Update(
			keyItem.ctx, copy, updateOpts,
		)
		if err != nil {
			return errors.WrapWithReconcileDeferredError(err, "could not add TridentObjectStore finalizer")
		}
	}

	store, err := c.objectStoreFromCR(keyItem.ctx, objectStore)
	if err != nil {
		if errors.IsReconcileDeferredError(err) {
			return err
		}
		if k8sapierrors.IsNotFound(err) {
			return errors.WrapWithReconcileDeferredError(err, "object store credentials secret is not available")
		}
		return errors.UnsupportedConfigError("invalid TridentObjectStore spec: %v", err)
	}

	if _, err = c.orchestrator.AddOrUpdateObjectStore(keyItem.ctx, store); err != nil {
		return errors.WrapWithReconcileDeferredError(err, "could not add or update object store")
	}

	Logx(keyItem.ctx).WithFields(LogFields{
		"name":      objectStore.Name,
		"namespace": objectStore.Namespace,
	}).Debug("Reconciled TridentObjectStore.")
	return nil
}

// deleteCreatedObjectStoreSecret removes the Secret tridentctl create wrote for this store.
// A user-supplied credentials secret is left in place.
func (c *TridentCrdController) deleteCreatedObjectStoreSecret(
	ctx context.Context, objectStore *tridentv1.TridentObjectStore,
) error {
	secretName, err := objectStore.Spec.GetSecretName()
	if err != nil || secretName == "" || secretName != objectStore.Name+"-creds" {
		return nil
	}
	err = c.kubeClientset.CoreV1().Secrets(objectStore.Namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
	if err != nil && !k8sapierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (c *TridentCrdController) objectStoreFromCR(
	ctx context.Context, objectStore *tridentv1.TridentObjectStore,
) (object.Store, error) {
	secretName, err := objectStore.Spec.GetSecretName()
	if err != nil {
		Logx(ctx).WithFields(LogFields{
			"name":      objectStore.Name,
			"namespace": objectStore.Namespace,
		}).WithError(err).Info("TridentObjectStore secret name could not be read; requeuing.")
		return nil, errors.WrapWithReconcileDeferredError(err, "could not get TridentObjectStore secret name")
	}

	var secretData map[string][]byte
	if secretName != "" {
		secret, getErr := c.secretsLister.Secrets(objectStore.Namespace).Get(secretName)
		if getErr != nil {
			if k8sapierrors.IsNotFound(getErr) {
				Logx(ctx).WithFields(LogFields{
					"name":       objectStore.Name,
					"namespace":  objectStore.Namespace,
					"secretName": secretName,
				}).Info("TridentObjectStore credentials secret is not available; requeuing.")
				return nil, errors.WrapWithReconcileDeferredError(getErr,
					"object store credentials secret %s/%s is not available", objectStore.Namespace, secretName)
			}
			return nil, getErr
		}
		secretData = secret.Data
		Logx(ctx).WithFields(LogFields{
			"name":      objectStore.Name,
			"namespace": objectStore.Namespace,
		}).Trace("Resolved TridentObjectStore credentials.")
	}

	secretConfig, err := tridentv1.SecretConfig(secretData)
	if err != nil {
		return nil, err
	}
	return objectStore.Persistent(ctx, secretConfig)
}
