// Copyright 2022 NetApp, Inc. All Rights Reserved.

package plain

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend/csi"
	"github.com/netapp/trident/frontend/csi/helpers"
	mock "github.com/netapp/trident/mocks/mock_core"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	log.SetOutput(ioutil.Discard)
	os.Exit(m.Run())
}

func TestPluginActivate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	plugin := NewPlugin(orchestrator)

	err := plugin.Activate()
	assert.NoError(t, err, "Error is not nil")
}

func TestPluginDeactivate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	plugin := NewPlugin(orchestrator)

	err := plugin.Deactivate()
	assert.NoError(t, err, "Error is not nil")
}

func TestGetName(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	plugin := NewPlugin(orchestrator)

	pluginName := plugin.GetName()
	assert.Equal(t, "plain_csi_helper", pluginName, "Plugin name does not match")
}

func TestPluginVersion(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	plugin := NewPlugin(orchestrator)

	pluginVersion := plugin.Version()
	assert.Equal(t, csi.Version, pluginVersion, "Plugin version does not match")
}

func TestGetVolumeConfig(t *testing.T) {
	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	plugin := NewPlugin(orchestrator)

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

func TestPluginGetSnapshotConfig(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	plugin := NewPlugin(orchestrator)

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

func TestPluginGetNodeTopologies(t *testing.T) {
	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	plugin := NewPlugin(orchestrator)

	nodeTopologylabels, err := plugin.GetNodeTopologyLabels(ctx, "node")
	assert.Nil(t, err, "Error is not nil")
	assert.NotNil(t, nodeTopologylabels, "unable to get node topologies")
}

func TestPluginRecordVolumeEvent(t *testing.T) {
	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	plugin := NewPlugin(orchestrator)

	plugin.RecordVolumeEvent(ctx, "node", helpers.EventTypeNormal, "ProvisioningSuccess",
		"provisioned a volume")
}

func TestPluginRecordNodeEvent(t *testing.T) {
	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	plugin := NewPlugin(orchestrator)

	plugin.RecordNodeEvent(ctx, "node", helpers.EventTypeWarning, "ProvisioningSuccess",
		"provisioned a volume")
}

func TestSupportsFeature(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mock.NewMockOrchestrator(mockCtrl)
	plugin := NewPlugin(orchestrator)

	type supportedFeature struct {
		feature  helpers.Feature
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
