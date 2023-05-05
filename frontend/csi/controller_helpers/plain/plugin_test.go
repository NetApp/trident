// Copyright 2022 NetApp, Inc. All Rights Reserved.

package plain

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend/csi"
	controller_helpers "github.com/netapp/trident/frontend/csi/controller_helpers"
	. "github.com/netapp/trident/logging"
	mock "github.com/netapp/trident/mocks/mock_core"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils/errors"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

func TestPluginActivate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	orchestrator.EXPECT().ReconcileVolumePublications(gomock.Any(), gomock.Any()).Return(nil)
	plugin := NewHelper(orchestrator)

	err := plugin.Activate()
	assert.NoError(t, err, "Expected Activate to succeed when ReconcileVolumePublications succeeds")
}

func TestPluginActivate_FailsWithReconcileError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	orchestrator.EXPECT().ReconcileVolumePublications(gomock.Any(), gomock.Any()).Return(errors.New("core error"))
	plugin := NewHelper(orchestrator)

	err := plugin.Activate()
	assert.Error(t, err, "Expected Activate to fail when ReconcileVolumePublications fails")
}

func TestPluginDeactivate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	plugin := NewHelper(orchestrator)

	err := plugin.Deactivate()
	assert.NoError(t, err, "Error is not nil")
}

func TestGetName(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	plugin := NewHelper(orchestrator)

	pluginName := plugin.GetName()
	assert.Equal(t, "plain_csi_helper", pluginName, "Plugin name does not match")
}

func TestVersion(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	plugin := NewHelper(orchestrator)

	pluginVersion := plugin.Version()
	assert.Equal(t, csi.Version, pluginVersion, "Plugin version does not match")
}

func TestGetVolumeConfig(t *testing.T) {
	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	p := NewHelper(orchestrator)
	plugin, ok := p.(controller_helpers.ControllerHelper)
	if !ok {
		t.Fatal("Could not cast the helper to a ControllerHelper!")
	}

	type volumeConfigTest struct {
		volumeName         string
		accessMode         []config.AccessMode
		parameters         map[string]string
		expected           bool
		expectedAccessMode config.AccessMode
	}

	accessMode1 := []config.AccessMode{config.ReadWriteOnce}
	accessMode2 := []config.AccessMode{config.ModeAny, config.ReadWriteOnce, config.ReadOnlyMany}
	accessMode3 := []config.AccessMode{config.ModeAny}
	tests := map[string]volumeConfigTest{
		"volumeConfigWithOneAccessMode": {
			"volume1", accessMode1, make(map[string]string), false, config.ReadWriteOnce,
		},
		"volumeConfigWithMultipleAccessMode": {
			"volume1", accessMode2, make(map[string]string), false, config.ReadWriteMany,
		},
		"volumeConfigParameterNil": {
			"volume1", accessMode3, nil, true, config.ModeAny,
		},
	}

	for testName, test := range tests {
		t.Run(fmt.Sprintf(testName+":"), func(t *testing.T) {
			if test.expected {
				err := fmt.Errorf("error")
				orchestrator.EXPECT().GetStorageClass(ctx, gomock.Any()).Return(nil, err)
			} else {
				storageClassExternal := &storageclass.External{Config: &storageclass.Config{Name: "basicsc"}}
				orchestrator.EXPECT().GetStorageClass(ctx, gomock.Any()).Return(storageClassExternal, nil)
			}

			dummyMap := make([]map[string]string, 0)
			expected := &storage.VolumeConfig{
				Name:                test.volumeName,
				Size:                "100",
				StorageClass:        "basicsc",
				Protocol:            config.Protocol(config.File),
				VolumeMode:          config.VolumeMode(config.Filesystem),
				RequisiteTopologies: dummyMap,
				PreferredTopologies: dummyMap,
				FileSystem:          "fsType",
				AccessMode:          test.expectedAccessMode,
			}
			result, err := plugin.GetVolumeConfig(ctx, test.volumeName, 100, test.parameters,
				config.Protocol(config.File), test.accessMode, config.VolumeMode(config.Filesystem), "fsType",
				dummyMap, dummyMap, nil)

			if test.expected {
				assert.Error(t, err, "Error is nil")
			} else {
				assert.Equal(t, expected, result, "VolumeConfig object does not match")
			}
		})
	}
}

func TestGetSnapshotConfig(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	p := NewHelper(orchestrator)
	plugin, ok := p.(controller_helpers.ControllerHelper)
	if !ok {
		t.Fatal("Could not cast the helper to a ControllerHelper!")
	}

	type snapshotConfigTest struct {
		volumeName   string
		snapshotName string
	}

	tests := map[string]snapshotConfigTest{
		"validVolumeSnapshotValue":  {"volume", "snapshot"},
		"volumeNameIsEmpty":         {"", "snapshot"},
		"snapshotNameIsEmpty":       {"volume", ""},
		"bothVolumeSnapshotIsEmpty": {"", ""},
	}

	for testName, test := range tests {
		t.Run(fmt.Sprintf(testName+":"), func(t *testing.T) {
			expected := &storage.SnapshotConfig{
				Version:    config.OrchestratorAPIVersion,
				Name:       test.snapshotName,
				VolumeName: test.volumeName,
			}
			snapshotConfig, err := plugin.GetSnapshotConfig(test.volumeName, test.snapshotName)
			assert.Nil(t, err, "Error is not nil")
			assert.Equal(t, expected, snapshotConfig, "The snapshotConfig does not match")
		})
	}
}

func TestGetNodeTopologyLabels(t *testing.T) {
	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	p := NewHelper(orchestrator)
	plugin, ok := p.(controller_helpers.ControllerHelper)
	if !ok {
		t.Fatal("Could not cast the helper to a ControllerHelper!")
	}

	nodeTopologylabels, err := plugin.GetNodeTopologyLabels(ctx, "node")
	assert.Nil(t, err, "Error is not nil")
	assert.NotNil(t, nodeTopologylabels, "unable to get node topologies")
}

func TestGetNodePublicationState(t *testing.T) {
	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	p := NewHelper(orchestrator)
	plugin, ok := p.(controller_helpers.ControllerHelper)
	if !ok {
		t.Fatal("Could not cast the helper to a ControllerHelper!")
	}

	flags, err := plugin.GetNodePublicationState(ctx, "node")
	assert.Nil(t, flags, "Flags are not nil")
	assert.Nil(t, err, "Error is not nil")
}

func TestRecordVolumeEvent(t *testing.T) {
	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	p := NewHelper(orchestrator)
	plugin, ok := p.(controller_helpers.ControllerHelper)
	if !ok {
		t.Fatal("Could not cast the helper to a ControllerHelper!")
	}

	plugin.RecordVolumeEvent(ctx, "node", controller_helpers.EventTypeNormal, "ProvisioningSuccess",
		"provisioned a volume")
}

func TestRecordNodeEvent(t *testing.T) {
	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	p := NewHelper(orchestrator)
	plugin, ok := p.(controller_helpers.ControllerHelper)
	if !ok {
		t.Fatal("Could not cast the helper to a ControllerHelper!")
	}

	plugin.RecordNodeEvent(ctx, "node", controller_helpers.EventTypeWarning, "ProvisioningSuccess",
		"provisioned a volume")
}

func TestSupportsFeature(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	p := NewHelper(orchestrator)
	plugin, ok := p.(controller_helpers.ControllerHelper)
	if !ok {
		t.Fatal("Could not cast the helper to a ControllerHelper!")
	}

	type supportedFeature struct {
		feature  controller_helpers.Feature
		expected bool
	}

	supportedTests := map[string]supportedFeature{
		"expandCSITest":       {csi.ExpandCSIVolumes, true},
		"CSIBlockVolumesTest": {csi.CSIBlockVolumes, true},
		"CSIIsNil":            {"", false},
	}

	for _, tc := range supportedTests {
		supported := plugin.SupportsFeature(context.Background(), tc.feature)
		assert.Equal(t, tc.expected, supported, "Feature is not supported")
	}
}
