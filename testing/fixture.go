// Copyright 2024 NetApp, Inc. All Rights Reserved.

/*
Copyright 2016 The Kubernetes Authors.

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

package testing

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/watch"
	clientgotesting "k8s.io/client-go/testing"

	"github.com/netapp/trident/internal/crypto"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

// This file is derived from k8s.io/client-go/testing/fixture.go, version 0.26.0.
// It was modified to offer richer functionality, including setting creation & deletion
// timestamps, 'generateName' object names, and resource versions.  It also respects
// finalizers by not deleting (but instead updating with a deletion timestamp) an object
// with finalizer(s), and deleting (instead of updating) when finalizers are removed and
// a deletion timestamp is present.  Note that the K8s community declined to add improvements
// like we have done here:  https://github.com/kubernetes-sigs/controller-runtime/issues/72

// ObjectTracker keeps track of objects. It is intended to be used to
// fake calls to a server by returning objects based on their kind,
// namespace and name.
type ObjectTracker interface {
	// Add adds an object to the tracker. If object being added
	// is a list, its items are added separately.
	Add(obj runtime.Object) error

	// Get retrieves the object by its kind, namespace and name.
	Get(gvr schema.GroupVersionResource, ns, name string) (runtime.Object, error)

	// Create adds an object to the tracker in the specified namespace.
	Create(gvr schema.GroupVersionResource, obj runtime.Object, ns string) (runtime.Object, error)

	// Update updates an existing object in the tracker in the specified namespace.
	Update(gvr schema.GroupVersionResource, obj runtime.Object, ns string) (runtime.Object, error)

	// List retrieves all objects of a given kind in the given
	// namespace. Only non-List kinds are accepted.
	List(gvr schema.GroupVersionResource, gvk schema.GroupVersionKind, ns string) (runtime.Object, error)

	// Delete deletes an existing object from the tracker. If object
	// didn't exist in the tracker prior to deletion, Delete returns
	// no error.
	Delete(gvr schema.GroupVersionResource, ns, name string) error

	// Watch watches objects from the tracker. Watch returns a channel
	// which will push added / modified / deleted object.
	Watch(gvr schema.GroupVersionResource, ns string) (watch.Interface, error)
}

// ObjectScheme abstracts the implementation of common operations on objects.
type ObjectScheme interface {
	runtime.ObjectCreater
	runtime.ObjectTyper
}

// ObjectReaction returns a ReactionFunc that applies core.Action to the given tracker.
func ObjectReaction(tracker ObjectTracker) clientgotesting.ReactionFunc {
	return func(action clientgotesting.Action) (bool, runtime.Object, error) {
		ns := action.GetNamespace()
		gvr := action.GetResource()
		// Here and below we need to switch on implementation types,
		// not on interfaces, as some interfaces are identical
		// (e.g. UpdateAction and CreateAction), so if we use them,
		// updates and creates end up matching the same case branch.
		switch action := action.(type) {

		case clientgotesting.ListActionImpl:
			obj, err := tracker.List(gvr, action.GetKind(), ns)
			return true, obj, err

		case clientgotesting.GetActionImpl:
			obj, err := tracker.Get(gvr, ns, action.GetName())
			return true, obj, err

		case clientgotesting.CreateActionImpl:

			obj := action.GetObject()
			objMeta, err := meta.Accessor(obj)
			if err != nil {
				return true, nil, err
			}
			if action.GetSubresource() == "" {
				obj, err = tracker.Create(gvr, action.GetObject(), ns)
			} else {
				oldObj, getOldObjErr := tracker.Get(gvr, ns, objMeta.GetName())
				if getOldObjErr != nil {
					return true, nil, getOldObjErr
				}
				// Check whether the existing historical object type is the same as the current operation object
				// type that needs to be updated, and if it is the same, perform the update operation.
				if reflect.TypeOf(oldObj) == reflect.TypeOf(action.GetObject()) {
					// TODO: Currently we're handling subresource creation as an update
					// on the enclosing resource. This works for some subresources but
					// might not be generic enough.
					_, err = tracker.Update(gvr, action.GetObject(), ns)
				} else {
					// If the historical object type is different from the current object type, need to make sure
					// we return the object submitted,don't persist the submitted object in the tracker.
					return true, action.GetObject(), nil
				}
			}
			if err != nil {
				return true, nil, err
			}
			return true, obj, nil

		case clientgotesting.UpdateActionImpl:
			_, err := meta.Accessor(action.GetObject())
			if err != nil {
				return true, nil, err
			}
			obj, err := tracker.Update(gvr, action.GetObject(), ns)
			if err != nil {
				return true, nil, err
			}
			return true, obj, nil

		case clientgotesting.DeleteActionImpl:
			err := tracker.Delete(gvr, ns, action.GetName())
			if err != nil {
				return true, nil, err
			}
			return true, nil, nil

		case clientgotesting.PatchActionImpl:
			obj, err := tracker.Get(gvr, ns, action.GetName())
			if err != nil {
				return true, nil, err
			}

			old, err := json.Marshal(obj)
			if err != nil {
				return true, nil, err
			}

			// reset the object in preparation to unmarshal, since unmarshal does not guarantee that fields
			// in obj that are removed by patch are cleared
			value := reflect.ValueOf(obj)
			value.Elem().Set(reflect.New(value.Type().Elem()).Elem())

			switch action.GetPatchType() {
			case types.JSONPatchType:
				patch, err := jsonpatch.DecodePatch(action.GetPatch())
				if err != nil {
					return true, nil, err
				}
				modified, err := patch.Apply(old)
				if err != nil {
					return true, nil, err
				}

				if err = json.Unmarshal(modified, obj); err != nil {
					return true, nil, err
				}
			case types.MergePatchType:
				modified, err := jsonpatch.MergePatch(old, action.GetPatch())
				if err != nil {
					return true, nil, err
				}

				if err := json.Unmarshal(modified, obj); err != nil {
					return true, nil, err
				}
			case types.StrategicMergePatchType, types.ApplyPatchType:
				mergedByte, err := strategicpatch.StrategicMergePatch(old, action.GetPatch(), obj)
				if err != nil {
					return true, nil, err
				}
				if err = json.Unmarshal(mergedByte, obj); err != nil {
					return true, nil, err
				}
			default:
				return true, nil, errors.New("PatchType is not supported")
			}

			if obj, err = tracker.Update(gvr, obj, ns); err != nil {
				return true, nil, err
			} else {
				return true, obj, nil
			}

		default:
			return false, nil, fmt.Errorf("no reaction implemented for %s", action)
		}
	}
}

type tracker struct {
	scheme  ObjectScheme
	decoder runtime.Decoder
	lock    sync.RWMutex
	objects map[schema.GroupVersionResource]map[types.NamespacedName]runtime.Object
	// The value type of watchers is a map of which the key is either a namespace or
	// all/non namespace aka "" and its value is list of fake watchers.
	// Manipulations on resources will broadcast the notification events into the
	// watchers' channel. Note that too many unhandled events (currently 100,
	// see apimachinery/pkg/watch.DefaultChanSize) will cause a panic.
	watchers map[schema.GroupVersionResource]map[string][]*watch.RaceFreeFakeWatcher
}

var _ ObjectTracker = &tracker{}

// NewObjectTracker returns an ObjectTracker that can be used to keep track
// of objects for the fake clientset. Mostly useful for unit tests.
func NewObjectTracker(scheme ObjectScheme, decoder runtime.Decoder) ObjectTracker {
	return &tracker{
		scheme:   scheme,
		decoder:  decoder,
		objects:  make(map[schema.GroupVersionResource]map[types.NamespacedName]runtime.Object),
		watchers: make(map[schema.GroupVersionResource]map[string][]*watch.RaceFreeFakeWatcher),
	}
}

func (t *tracker) List(
	gvr schema.GroupVersionResource, gvk schema.GroupVersionKind, ns string,
) (runtime.Object, error) {
	// Heuristic for list kind: original kind + List suffix. Might
	// not always be true but this tracker has a pretty limited
	// understanding of the actual API model.
	listGVK := gvk
	listGVK.Kind = listGVK.Kind + "List"
	// GVK does have the concept of "internal version". The scheme recognizes
	// the runtime.APIVersionInternal, but not the empty string.
	if listGVK.Version == "" {
		listGVK.Version = runtime.APIVersionInternal
	}

	list, err := t.scheme.New(listGVK)
	if err != nil {
		return nil, err
	}

	if !meta.IsListType(list) {
		return nil, fmt.Errorf("%q is not a list type", listGVK.Kind)
	}

	t.lock.RLock()
	defer t.lock.RUnlock()

	objs, ok := t.objects[gvr]
	if !ok {
		return list, nil
	}

	matchingObjs, err := filterByNamespace(objs, ns)
	if err != nil {
		return nil, err
	}
	if err := meta.SetList(list, matchingObjs); err != nil {
		return nil, err
	}
	return list.DeepCopyObject(), nil
}

func (t *tracker) Watch(gvr schema.GroupVersionResource, ns string) (watch.Interface, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	fakewatcher := watch.NewRaceFreeFake()

	if _, exists := t.watchers[gvr]; !exists {
		t.watchers[gvr] = make(map[string][]*watch.RaceFreeFakeWatcher)
	}
	t.watchers[gvr][ns] = append(t.watchers[gvr][ns], fakewatcher)
	return fakewatcher, nil
}

func (t *tracker) Get(gvr schema.GroupVersionResource, ns, name string) (runtime.Object, error) {
	errNotFound := k8serrors.NewNotFound(gvr.GroupResource(), name)

	t.lock.RLock()
	defer t.lock.RUnlock()

	objs, ok := t.objects[gvr]
	if !ok {
		Log().Debug("GET: GVR not found.")
		return nil, errNotFound
	}

	matchingObj, ok := objs[types.NamespacedName{Namespace: ns, Name: name}]
	if !ok {
		Log().WithFields(LogFields{"Namespace": ns, "Name": name}).Debug("GET: object not found.")
		return nil, errNotFound
	}

	// Only one object should match in the tracker if it works
	// correctly, as Add/Update methods enforce kind/namespace/name
	// uniqueness.
	obj := matchingObj.DeepCopyObject()
	if status, ok := obj.(*metav1.Status); ok {
		if status.Status != metav1.StatusSuccess {
			return nil, &k8serrors.StatusError{ErrStatus: *status}
		}
	}

	Log().WithFields(LogFields{"object": obj}).Debug("GET: object found.")
	return obj, nil
}

func (t *tracker) Add(obj runtime.Object) error {
	if meta.IsListType(obj) {
		return t.addList(obj, false)
	}
	objMeta, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	gvks, _, err := t.scheme.ObjectKinds(obj)
	if err != nil {
		return err
	}

	if partial, ok := obj.(*metav1.PartialObjectMetadata); ok && len(partial.TypeMeta.APIVersion) > 0 {
		gvks = []schema.GroupVersionKind{partial.TypeMeta.GroupVersionKind()}
	}

	if len(gvks) == 0 {
		return fmt.Errorf("no registered kinds for %v", obj)
	}
	for _, gvk := range gvks {
		// NOTE: UnsafeGuessKindToResource is a heuristic and default match. The
		// actual registration in apiserver can specify arbitrary route for a
		// gvk. If a test uses such objects, it cannot preset the tracker with
		// objects via Add(). Instead, it should trigger the Create() function
		// of the tracker, where an arbitrary gvr can be specified.
		gvr, _ := meta.UnsafeGuessKindToResource(gvk)
		// Resource doesn't have the concept of "__internal" version, just set it to "".
		if gvr.Version == runtime.APIVersionInternal {
			gvr.Version = ""
		}

		_, err = t.add(gvr, obj, objMeta.GetNamespace(), false)
		if err != nil {
			return err
		}
	}

	Log().WithFields(LogFields{"object": obj}).Debug("Add: object added.")
	return nil
}

func (t *tracker) Create(gvr schema.GroupVersionResource, obj runtime.Object, ns string) (runtime.Object, error) {
	return t.add(gvr, obj, ns, false)
}

func (t *tracker) Update(gvr schema.GroupVersionResource, obj runtime.Object, ns string) (runtime.Object, error) {
	return t.add(gvr, obj, ns, true)
}

func (t *tracker) getWatches(gvr schema.GroupVersionResource, ns string) []*watch.RaceFreeFakeWatcher {
	watches := make([]*watch.RaceFreeFakeWatcher, 0)
	if t.watchers[gvr] != nil {
		if w := t.watchers[gvr][ns]; w != nil {
			watches = append(watches, w...)
		}
		if ns != metav1.NamespaceAll {
			if w := t.watchers[gvr][metav1.NamespaceAll]; w != nil {
				watches = append(watches, w...)
			}
		}
	}
	return watches
}

func (t *tracker) add(
	gvr schema.GroupVersionResource, obj runtime.Object, ns string, replaceExisting bool,
) (runtime.Object, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	gr := gvr.GroupResource()

	// To avoid the object from being accidentally modified by caller
	// after it's been added to the tracker, we always store the deep
	// copy.
	obj = obj.DeepCopyObject()

	newMeta, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}

	if newMeta.GetGenerateName() != "" {
		if newMeta.GetName() == "" {
			newMeta.SetName(newMeta.GetGenerateName() + strings.ToLower(crypto.RandomString(5)))
		}
	}

	// Set an initial UID if not set
	if newMeta.GetUID() == "" {
		newMeta.SetUID(types.UID(uuid.NewString()))
	}

	// Set an initial resource version if not set
	if newMeta.GetResourceVersion() == "" {
		newMeta.SetResourceVersion("1")
	}

	// Propagate namespace to the new object if it hasn't already been set.
	if len(newMeta.GetNamespace()) == 0 {
		newMeta.SetNamespace(ns)
	}

	if ns != newMeta.GetNamespace() {
		msg := fmt.Sprintf("request namespace does not match object namespace, request: %q object: %q",
			ns, newMeta.GetNamespace())
		return nil, k8serrors.NewBadRequest(msg)
	}

	_, ok := t.objects[gvr]
	if !ok {
		t.objects[gvr] = make(map[types.NamespacedName]runtime.Object)
	}

	namespacedName := types.NamespacedName{Namespace: newMeta.GetNamespace(), Name: newMeta.GetName()}
	if existingObj, ok := t.objects[gvr][namespacedName]; ok {
		if replaceExisting {

			if !cmp.Equal(obj, existingObj) {
				if diff := cmp.Diff(existingObj, obj); diff != "" {
					Log().Debugf("updated object fields (-old +new):%s", diff)

					existingMeta, err := meta.Accessor(existingObj)
					if err != nil {
						return nil, err
					}

					if existingMeta.GetResourceVersion() == "" {
						existingMeta.SetResourceVersion("1")
					}
					if currentResourceVersion, err := strconv.Atoi(existingMeta.GetResourceVersion()); err == nil {
						newMeta.SetResourceVersion(strconv.Itoa(currentResourceVersion + 1))
					}
					Log().WithFields(LogFields{
						"exitingResourceVersion": existingMeta.GetResourceVersion(),
						"updatedResourceVersion": newMeta.GetResourceVersion(),
					}).Debug("Incremented ResourceVersion.")
				}
			} else {
				Log().Debug("No difference, leaving ResourceVersion unchanged.")
			}

			// If this update resulted in an object with a deletion timestamp and no finalizers, delete it.
			if !newMeta.GetDeletionTimestamp().IsZero() && len(newMeta.GetFinalizers()) == 0 {

				Log().WithFields(LogFields{"object": obj}).Debug(
					"add: deleting with deletion timestamp and no finalizers.")

				delete(t.objects[gvr], namespacedName)

				// Notify delete watchers
				for _, w := range t.getWatches(gvr, ns) {
					// To avoid the object from being accidentally modified by watcher
					w.Delete(obj.DeepCopyObject())
				}
			} else {
				t.objects[gvr][namespacedName] = obj

				Log().WithFields(LogFields{"object": obj}).Debug("add: object replaced.")

				// Notify modify watchers
				for _, w := range t.getWatches(gvr, ns) {
					// To avoid the object from being accidentally modified by watcher
					w.Modify(obj.DeepCopyObject())
				}
			}

			return obj, nil
		}

		Log().WithFields(LogFields{"object": obj}).Debug("add: object already exists.")
		return nil, k8serrors.NewAlreadyExists(gr, newMeta.GetName())
	}

	if replaceExisting {
		Log().WithFields(LogFields{"object": obj}).Debug("add: object not found.")

		// Tried to update but no matching object was found.
		return nil, k8serrors.NewNotFound(gr, newMeta.GetName())
	}

	// Set the creation timestamp
	creationTimeStamp := newMeta.GetCreationTimestamp()
	if (&creationTimeStamp).IsZero() {
		newMeta.SetCreationTimestamp(metav1.Now())
	} else {
		return nil, k8serrors.NewBadRequest("new object may not contain a creation timestamp")
	}

	t.objects[gvr][namespacedName] = obj

	for _, w := range t.getWatches(gvr, ns) {
		// To avoid the object from being accidentally modified by watcher
		w.Add(obj.DeepCopyObject())
	}

	Log().WithFields(LogFields{"object": obj}).Debug("add: object added.")
	return obj, nil
}

func (t *tracker) addList(obj runtime.Object, _ /*replaceExisting*/ bool) error {
	list, err := meta.ExtractList(obj)
	if err != nil {
		return err
	}
	errs := runtime.DecodeList(list, t.decoder)
	if len(errs) > 0 {
		return errs[0]
	}
	for _, obj := range list {
		if err := t.Add(obj); err != nil {
			return err
		}
	}
	return nil
}

func (t *tracker) Delete(gvr schema.GroupVersionResource, ns, name string) error {
	t.lock.Lock()
	defer t.lock.Unlock()

	objs, ok := t.objects[gvr]
	if !ok {
		return k8serrors.NewNotFound(gvr.GroupResource(), name)
	}

	namespacedName := types.NamespacedName{Namespace: ns, Name: name}
	obj, ok := objs[namespacedName]
	if !ok {
		return k8serrors.NewNotFound(gvr.GroupResource(), name)
	}

	newMeta, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	// If this object still has finalizers, just update it with a deletion timestamp if there isn't one already.
	if len(newMeta.GetFinalizers()) > 0 {
		if newMeta.GetDeletionTimestamp() != nil && !newMeta.GetDeletionTimestamp().IsZero() {
			Log().WithFields(LogFields{"object": obj}).Debug(
				"Delete: object has finalizers and deletion timestamp, nothing to do.")
		} else {
			now := metav1.Now()
			newMeta.SetDeletionTimestamp(&now)

			t.objects[gvr][namespacedName] = obj

			Log().WithFields(LogFields{"object": obj}).Debug(
				"Delete: object has finalizers, adding deletion timestamp.")

			// Notify modify watchers
			for _, w := range t.getWatches(gvr, ns) {
				w.Modify(obj.DeepCopyObject())
			}
		}
	} else {
		delete(objs, namespacedName)

		Log().WithFields(LogFields{"object": obj}).Debug("Delete: object deleted.")

		// Notify delete watchers
		for _, w := range t.getWatches(gvr, ns) {
			w.Delete(obj.DeepCopyObject())
		}
	}

	return nil
}

// filterByNamespace returns all objects in the collection that
// match provided namespace. Empty namespace matches
// non-namespaced objects.
func filterByNamespace(objs map[types.NamespacedName]runtime.Object, ns string) ([]runtime.Object, error) {
	var res []runtime.Object

	for _, obj := range objs {
		acc, err := meta.Accessor(obj)
		if err != nil {
			return nil, err
		}
		if ns != "" && acc.GetNamespace() != ns {
			continue
		}
		res = append(res, obj)
	}

	// Sort res to get deterministic order.
	sort.Slice(res, func(i, j int) bool {
		acc1, _ := meta.Accessor(res[i])
		acc2, _ := meta.Accessor(res[j])
		if acc1.GetNamespace() != acc2.GetNamespace() {
			return acc1.GetNamespace() < acc2.GetNamespace()
		}
		return acc1.GetName() < acc2.GetName()
	})
	return res, nil
}
