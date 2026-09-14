// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockNode "github.com/netapp/trident/mocks/mock_core/mock_node"
	mockTridentController "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_tridentcontroller"
	"github.com/netapp/trident/utils/models"
)

// TestNewController_UpdateVolumeReachable locks in the wiring that lets UpdateVolume flow through
// NewController for free: tridentcontroller.Client is embedded in controllerFromClients, so adding
// UpdateVolume to that interface required no change to NewController or controllerFromClients
// themselves - only the mock needed regenerating.
func TestNewController_UpdateVolumeReachable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mockTridentController.NewMockClient(ctrl)
	mockChap := mockNode.NewMockChapClient(ctrl)

	names := []string{"a", "b"}
	update := &models.NodeVolumeUpdate{LUKSPassphraseNames: &names}
	mockClient.EXPECT().UpdateVolume(gomock.Any(), "vol1", update).Return(nil)

	controller := NewController(mockClient, mockChap)

	err := controller.UpdateVolume(context.Background(), "vol1", update)

	assert.NoError(t, err)
}
