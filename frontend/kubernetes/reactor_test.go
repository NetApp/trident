// Copyright 2016 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	log "github.com/Sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/client-go/kubernetes/fake"
	k8s_testing "k8s.io/client-go/testing"
	framework "k8s.io/client-go/tools/cache/testing"
)

// This file defines a k8s_testing.Reactor used for testing purposes (see below).
// It contains no unit tests of its own and was adapted from
// k8s.io/kubernetes/pkg/controller/persistentvolume/framework_test.go,
// in the master branch as of 7/1/2016.

// orchestratorReactor is used to handle the fake Kubeclient's responses to
// PV events and to inject PVC events into the OrchestratorController.
type orchestratorReactor struct {
	volumes     map[string]*v1.PersistentVolume
	claimSource *framework.FakePVCControllerSource
	events      int
	sync.Mutex
}

func (r *orchestratorReactor) React(action k8s_testing.Action) (handled bool, ret runtime.Object, err error) {
	r.Lock()
	defer r.Unlock()

	log.Debug("Reactor received action.")
	r.events++
	switch {
	case action.Matches("create", "persistentvolumes"):
		volume := action.(k8s_testing.UpdateAction).GetObject().(*v1.PersistentVolume)

		// FakeClient does no validation, so we need to verify that the volume
		// wasn't already created.  That said, this is unlikely to fail.
		if _, found := r.volumes[volume.Name]; found {
			return true, nil, fmt.Errorf(
				"Unable to create volume %s:  already exists.", volume.Name)
		}
		r.volumes[volume.Name] = volume
		return true, volume, nil
	case action.Matches("delete", "persistentvolumes"):
		name := action.(k8s_testing.DeleteAction).GetName()

		if _, found := r.volumes[name]; !found {
			return true, nil, fmt.Errorf("Unable to delete volume %s:  not "+
				"found", name)
		}
		delete(r.volumes, name)
		return true, nil, nil
	}
	return false, nil, nil
}

func (r *orchestratorReactor) wait() {
	oldEvents := -1
	newEvents := r.events
	for oldEvents != newEvents {
		time.Sleep(10 * time.Millisecond)
		oldEvents = newEvents
		newEvents = r.events
	}
}

func (r *orchestratorReactor) getVolumeClone(name string) *v1.PersistentVolume {
	var (
		origVolume *v1.PersistentVolume
		found      bool
	)
	r.Lock()
	origVolume, found = r.volumes[name]
	r.Unlock()

	if !found {
		return nil
	}
	return origVolume.DeepCopy()
}

func (r *orchestratorReactor) validateVolumes(t *testing.T,
	expectedList []*v1.PersistentVolume) bool {
	r.Lock()
	defer r.Unlock()

	expectedMap := make(map[string]*v1.PersistentVolume)
	for _, v := range expectedList {
		// We care neither about the resource version nor about the volume
		// phase; those would ordinarily get handled by the API server and/or
		// the real controller.
		v.ResourceVersion = ""
		v.Status.Phase = v1.VolumeBound
		expectedMap[v.Name] = v
	}
	gotMap := make(map[string]*v1.PersistentVolume)
	// Copied shamelessly from
	// k8s.io/kubernetes/pkg/controller/persistentvolume/framework_test.go
	// I originally wasn't doing this, but we need to clear the resource version
	// and set the phase for the DeepEqual to work.
	for name, v := range r.volumes {
		v = v.DeepCopy()
		v.ResourceVersion = ""
		v.Status.Phase = v1.VolumeBound
		if v.Spec.ClaimRef != nil {
			v.Spec.ClaimRef.ResourceVersion = ""
		}
		gotMap[name] = v
	}
	if !reflect.DeepEqual(expectedMap, gotMap) {
		t.Errorf("Mismatch between actual and expected volumes:  %s",
			diff.ObjectDiff(gotMap, expectedMap))
		return false
	}
	return true
}

func newReactor(
	client *fake.Clientset,
	claimSource *framework.FakePVCControllerSource,
) *orchestratorReactor {
	ret := &orchestratorReactor{
		volumes:     make(map[string]*v1.PersistentVolume),
		claimSource: claimSource,
		events:      0,
	}
	client.AddReactor("*", "*", ret.React)
	return ret
}
