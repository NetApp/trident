// Copyright 2025 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	mockcore "github.com/netapp/trident/mocks/mock_core"
	mockhelpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_helpers"
	"github.com/netapp/trident/utils/errors"
)

func TestProbe(t *testing.T) {
	testCases := []struct {
		name              string
		getVersionErr     error
		getVersionReturns string
		expErrCode        codes.Code
		expResponse       *csi.ProbeResponse
	}{
		{
			name:              "Success",
			getVersionErr:     nil,
			expErrCode:        codes.OK,
			getVersionReturns: "test-version",
			expResponse:       &csi.ProbeResponse{},
		},
		{
			name:          "BootstrapError",
			getVersionErr: errors.BootstrapError(errors.New("bootstrap failed")),
			expErrCode:    codes.FailedPrecondition,
			expResponse:   &csi.ProbeResponse{},
		},
		{
			name:          "OtherError",
			getVersionErr: errors.New("some other error"),
			expErrCode:    codes.OK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create a mocked orchestrator
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
			// Create a mocked helper
			mockHelper := mockhelpers.NewMockControllerHelper(mockCtrl)
			// Create an instance of ControllerServer for this test
			controllerServer := generateController(mockOrchestrator, mockHelper)

			mockOrchestrator.EXPECT().GetVersion(gomock.Any()).Return(tc.getVersionReturns, tc.getVersionErr).AnyTimes()
			resp, err := controllerServer.Probe(context.Background(), &csi.ProbeRequest{})

			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				status, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, resp)
		})
	}
}

func TestGetPluginInfo(t *testing.T) {
	testCases := []struct {
		name     string
		plugin   *Plugin
		expected *csi.GetPluginInfoResponse
	}{
		{
			name: "Basic Info",
			plugin: &Plugin{
				name:    "trident-csi",
				version: "24.01.0",
			},
			expected: &csi.GetPluginInfoResponse{
				Name:          "trident-csi",
				VendorVersion: "24.01.0",
			},
		},
		{
			name: "Empty Info",
			plugin: &Plugin{
				name:    "",
				version: "",
			},
			expected: &csi.GetPluginInfoResponse{
				Name:          "",
				VendorVersion: "",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := tc.plugin.GetPluginInfo(context.Background(), &csi.GetPluginInfoRequest{})
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, resp)
		})
	}
}

func TestGetPluginCapabilities(t *testing.T) {
	testCases := []struct {
		name                   string
		topologyInUse          bool
		expectedServiceTypes   []csi.PluginCapability_Service_Type
		expectedExpansionTypes []csi.PluginCapability_VolumeExpansion_Type
	}{
		{
			name:          "No Topology",
			topologyInUse: false,
			expectedServiceTypes: []csi.PluginCapability_Service_Type{
				csi.PluginCapability_Service_CONTROLLER_SERVICE,
				csi.PluginCapability_Service_GROUP_CONTROLLER_SERVICE,
			},
			expectedExpansionTypes: []csi.PluginCapability_VolumeExpansion_Type{
				csi.PluginCapability_VolumeExpansion_ONLINE,
			},
		},
		{
			name:          "With Topology",
			topologyInUse: true,
			expectedServiceTypes: []csi.PluginCapability_Service_Type{
				csi.PluginCapability_Service_CONTROLLER_SERVICE,
				csi.PluginCapability_Service_GROUP_CONTROLLER_SERVICE,
				csi.PluginCapability_Service_VOLUME_ACCESSIBILITY_CONSTRAINTS,
			},
			expectedExpansionTypes: []csi.PluginCapability_VolumeExpansion_Type{
				csi.PluginCapability_VolumeExpansion_ONLINE,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &Plugin{
				topologyInUse: tc.topologyInUse,
			}
			resp, err := plugin.GetPluginCapabilities(context.Background(), &csi.GetPluginCapabilitiesRequest{})

			assert.NoError(t, err)
			assert.NotNil(t, resp)

			var gotServiceTypes []csi.PluginCapability_Service_Type
			var gotExpansionTypes []csi.PluginCapability_VolumeExpansion_Type

			for _, capability := range resp.Capabilities {
				if capability.GetService() != nil {
					gotServiceTypes = append(gotServiceTypes, capability.GetService().GetType())
				} else if capability.GetVolumeExpansion() != nil {
					gotExpansionTypes = append(gotExpansionTypes, capability.GetVolumeExpansion().GetType())
				}
			}

			assert.ElementsMatch(t, tc.expectedServiceTypes, gotServiceTypes)
			assert.ElementsMatch(t, tc.expectedExpansionTypes, gotExpansionTypes)
		})
	}
}
