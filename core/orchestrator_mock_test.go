// Copyright 2018 NetApp, Inc. All Rights Reserved.

package core

import (
	"reflect"
	"testing"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sc "github.com/netapp/trident/storage_class"
)

func findVolumeInMap(
	t *testing.T, backendMap map[string]*mockBackend, name string,
) *storage.Volume {
	var ret *storage.Volume
	matches := 0
	for _, backend := range backendMap {
		if volume, ok := backend.volumes[name]; ok {
			ret = volume
			matches++
		}
	}
	if matches > 1 {
		// Note that there's nothing in the code that prevents multiple
		// volumes of the same name on different backends, so it's likely that
		// a test is doing something wrong in this case.
		t.Error("Found multiple volume matches; returning last encountered.")
	}
	return ret
}

func TestAddMockBackend(t *testing.T) {
	m := NewMockOrchestrator()
	m.addMockBackend("test-nfs", config.File)
	m.addMockBackend("test-iscsi", config.Block)
	if _, ok := m.mockBackends["test-nfs"]; !ok {
		t.Error("NFS backend not added to mock backends.")
	}
	if _, ok := m.backends["test-nfs"]; !ok {
		t.Error("NFS backend not added to real backends.")
	}
	if _, ok := m.mockBackends["test-iscsi"]; !ok {
		t.Error("NFS backend not added to mock backends.")
	}
	if _, ok := m.backends["test-iscsi"]; !ok {
		t.Error("ISCSI backend not added to real backends.")
	}
}

func addAndRetrieveVolume(
	t *testing.T, vc *storage.VolumeConfig, m *MockOrchestrator,
) {
	_, err := m.AddStorageClass(&sc.Config{Name: vc.StorageClass})
	if err != nil {
		t.Fatalf("Unable to add storage class %s (%s): %v", vc.Name,
			vc.Protocol, err)
	}
	vol, err := m.AddVolume(vc)
	if err != nil {
		t.Fatalf("Unable to add volume %s (%s): %s", vc.Name, vc.Protocol, err)
	}
	if vol.Config != vc {
		t.Fatalf("Wrong config returned for volume %s (%s)", vc.Name,
			vc.Protocol)
	}
	found := findVolumeInMap(t, m.mockBackends, vc.Name)
	if found == nil {
		t.Errorf("Volume %s (%s) not found.", vc.Name,
			string(vc.Protocol))
	}
	if !reflect.DeepEqual(found.ConstructExternal(), vol) {
		t.Error("Found incorrect volume in map.")
	}
	foundVolume := m.GetVolume(vc.Name)
	if foundVolume == nil {
		t.Errorf("Failed to find volume %s (%s)", vc.Name,
			vc.Protocol)
	} else if !reflect.DeepEqual(foundVolume, vol) {
		// Note that both accessor methods return external copies, so we
		// can't rely on pointer equality to validate success.
		t.Errorf("Retrieved incorrect volume for %s (%s)", vc.Name,
			vc.Protocol)
	}
}

func TestAddVolume(t *testing.T) {
	m := NewMockOrchestrator()
	m.addMockBackend("test-nfs", config.File)
	m.addMockBackend("test-iscsi", config.Block)
	for _, v := range []*storage.VolumeConfig{
		{
			Name:         "test-nfs-vol",
			Size:         "10MB",
			Protocol:     config.File,
			StorageClass: "silver",
		},
		{
			Name:         "test-iscsi-vol",
			Size:         "10MB",
			Protocol:     config.Block,
			StorageClass: "silver",
		},
	} {
		addAndRetrieveVolume(t, v, m)
	}
}
