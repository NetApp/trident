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

// Package predicate defines controller-runtime Predicates that are re-used by COSI controllers to
// filter resource events before controller reconcile. COSI's split controller/sidecar architecture
// means that COSI has many reconcile filter behaviors that can be reused between components but
// where composition of smaller, individual behaviors is helpful.
package predicate

import (
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"

	"github.com/netapp/trident/cosi/third_party/bucketaccess"
)

// AnyCreate returns a predicate that enqueues a reconcile for any Create event.
// The predicate does not enqueue requests for any non-Create events.
func AnyCreate() predicate.Funcs {
	funcs := allFalseFuncs()
	funcs.CreateFunc = func(e event.CreateEvent) bool {
		return true
	}
	return funcs
}

// AnyDelete returns a predicate that enqueues a reconcile for any Delete event.
// The predicate does not enqueue requests for any non-Delete events.
func AnyDelete() predicate.Funcs {
	funcs := allFalseFuncs()
	funcs.DeleteFunc = func(e event.DeleteEvent) bool {
		// TODO: investigate DeleteStateUnknown to avoid reconciling nonexistent resources
		return true
	}
	return funcs
}

// AnyGeneric returns a predicate that enqueues a reconcile for any Generic event.
// The predicate does not enqueue requests for any non-Generic events.
func AnyGeneric() predicate.Funcs {
	funcs := allFalseFuncs()
	funcs.GenericFunc = func(e event.GenericEvent) bool {
		return true
	}
	return funcs
}

// GenerationChangedInUpdateOnly implements a predicate that enqueues a reconcile for Update events
// where the generation changes. For most resources, a generation change means that the resource
// `spec` has changed, ignoring metadata and status changes.
//
// The predicate does not enqueue requests for any Create/Delete/Generic events.
// This ensures that other predicates can effectively filter out undesired non-Update events.
//
// This is a modified implementation of controller-runtime's GenerationChangedPredicate{} which
// does enqueue requests for all Create/Delete/Generic events -- behavior COSI does not always want.
func GenerationChangedInUpdateOnly() predicate.Funcs {
	funcs := allFalseFuncs()
	funcs.UpdateFunc = func(e event.UpdateEvent) bool {
		return predicate.GenerationChangedPredicate{}.Update(e)
	}
	return funcs
}

// DeletionTimestampAdded implements a predicate that enqueues a reconcile for Update events where
// the deletion timestamp is added.
func DeletionTimestampAdded() predicate.Funcs {
	funcs := allFalseFuncs()
	funcs.UpdateFunc = func(e event.UpdateEvent) bool {
		return e.ObjectOld.GetDeletionTimestamp().IsZero() &&
			!e.ObjectNew.GetDeletionTimestamp().IsZero()
	}
	return funcs
}

// ProtectionFinalizerRemoved implements a predicate that enqueues a reconcile for Update events
// where the protection finalizer has been removed. This helps ensure that COSI always has a chance
// to re-apply the protection finalizer when it's needed.
//
// The predicate does not enqueue requests for any Create/Delete/Generic events.
// This ensures that other predicates can effectively filter out undesired non-Update events.
func ProtectionFinalizerRemoved(s *runtime.Scheme) predicate.Funcs {
	funcs := allFalseFuncs()
	funcs.UpdateFunc = func(e event.UpdateEvent) bool {
		old := e.ObjectOld
		new := e.ObjectNew

		if !new.GetDeletionTimestamp().IsZero() {
			return false // don't care if protection finalizer is missing when obj is deleting
		}

		oldHas := ctrlutil.ContainsFinalizer(old, cosiapi.ProtectionFinalizer)
		newHas := ctrlutil.ContainsFinalizer(new, cosiapi.ProtectionFinalizer)
		if oldHas && !newHas {
			logger := ctrl.Log.WithName("predicate")
			logger.Info("protection finalizer was removed from resource",
				"kind", inferKind(new, s), "namespace", new.GetNamespace(), "name", new.GetName())
			return true
		}

		return false
	}
	return funcs
}

// BucketAccessHandoffOccurred implements a predicate that enqueues a BucketAccess reconcile for
// Update events where the managing component of the BucketAccess changes, indicating that handoff
// between Controller and Sidecar has occurred in either direction.
//
// The predicate does not enqueue requests for any Create/Delete/Generic events.
// This ensures that other predicates can effectively filter out undesired non-Update events.
func BucketAccessHandoffOccurred(s *runtime.Scheme) predicate.Funcs {
	funcs := allFalseFuncs()
	funcs.UpdateFunc = func(e event.UpdateEvent) bool {
		old := e.ObjectOld
		new := e.ObjectNew

		logger := ctrl.Log.WithName("predicate")

		oldBa, ok := toTypedOrLogError[*cosiapi.BucketAccess](logger.WithValues("oldOrNew", "old"), s, old)
		if !ok {
			return false // not a BucketAccess, so don't manage it
		}
		newBa, ok := toTypedOrLogError[*cosiapi.BucketAccess](logger.WithValues("oldOrNew", "new"), s, new)
		if !ok {
			return false // not a BucketAccess, so don't manage it
		}

		return handoffOccurred(logger, oldBa, newBa)
	}
	return funcs
}

// Internal logic for determining if BucketAccess Controller-Sidecar handoff has occurred.
func handoffOccurred(logger logr.Logger, old, new *cosiapi.BucketAccess) bool {
	oldIsSidecar := bucketaccess.ManagedBySidecar(old)
	newIsSidecar := bucketaccess.ManagedBySidecar(new)
	if oldIsSidecar != newIsSidecar {
		toComponentName := func(isSidecar bool) string {
			if isSidecar {
				return "sidecar"
			}
			return "controller"
		}
		logger.Info("BucketAccess management handoff occurred",
			"namespace", old.GetNamespace(), "name", old.GetName(),
			"oldManagedBy", toComponentName(oldIsSidecar),
			"newManagedBy", toComponentName(newIsSidecar))
		return true
	}
	return false
}

// BucketAccessManagedBySidecar implements a predicate that enqueues a BucketAccess reconcile for
// any event if (and only if) the BucketAccess should be managed by the COSI Sidecar.
func BucketAccessManagedBySidecar(s *runtime.Scheme) predicate.Funcs {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		ba, ok := toTypedOrLogError[*cosiapi.BucketAccess](ctrl.Log.WithName("predicate"), s, object)
		if !ok {
			return false // not a BucketAccess, so don't manage it
		}
		return bucketaccess.ManagedBySidecar(ba)
	})
}

// BucketAccessManagedByController implements a predicate that enqueues a BucketAccess reconcile for
// any event if (and only if) the BucketAccess should be managed by the COSI Controller.
func BucketAccessManagedByController(s *runtime.Scheme) predicate.Funcs {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		ba, ok := toTypedOrLogError[*cosiapi.BucketAccess](ctrl.Log.WithName("predicate"), s, object)
		if !ok {
			return false // not a BucketAccess, so don't manage it
		}
		// Note: cannot simply return predicate.Not() of BucketAccessManagedBySidecar() because
		// any failed type conversion must return false for both Sidecar and Controller
		return !bucketaccess.ManagedBySidecar(ba)
	})
}

// Converts a client object to a typed object. Logs an error if conversion fails.
func toTypedOrLogError[T client.Object](logger logr.Logger, s *runtime.Scheme, object client.Object) (T, bool) {
	typed, ok := object.(T)
	if !ok {
		logger.Error(nil, "failed to convert object with unexpected type",
			"expectedType", fmt.Sprintf("%T", *new(T)),
			"receivedType", fmt.Sprintf("%T", object),
			"kind", inferKind(object, s), "namespace", object.GetNamespace(), "name", object.GetName())
		return *new(T), false
	}
	return typed, true
}

// Makes a best-effort attempt to infer a likely Kind for the object in the schema.
// Useful because controller-runtime predicates don't have GVK info for objects, and logging object
// kind in reusable predicates can help disambiguate resources in logs.
// See: https://github.com/kubernetes-sigs/controller-runtime/issues/1735.
func inferKind(o client.Object, s *runtime.Scheme) string {
	gvks, _, err := s.ObjectKinds(o)
	if err != nil {
		return "unknown"
	}
	for _, gvk := range gvks {
		if len(gvk.Kind) > 0 && len(gvk.Version) > 0 {
			return gvk.Kind
		}
	}
	return "unknown"
}

// Returns a predicate that returns false for all Create/Update/Delete/Generic events.
// Intended to be a base building block for COSI predicates.
func allFalseFuncs() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}
