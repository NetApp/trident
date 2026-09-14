// Copyright 2026 NetApp, Inc. All Rights Reserved.

package rest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockcontrollerAPI "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_api"
	"github.com/netapp/trident/utils/models"
)

func TestClient_UpdateVolume(t *testing.T) {
	t.Run("nil restClient returns a configuration error, mirrors every other method", func(t *testing.T) {
		c := NewClient(nil)
		names := []string{"a"}
		update := &models.NodeVolumeUpdate{LUKSPassphraseNames: &names}

		err := c.UpdateVolume(context.Background(), "vol1", update)

		assert.ErrorContains(t, err, "controller REST client is not configured")
	})

	t.Run("delegates to the controller_api client with the exact args", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockRestClient := mockcontrollerAPI.NewMockTridentController(mockCtrl)
		names := []string{"a", "b"}
		update := &models.NodeVolumeUpdate{LUKSPassphraseNames: &names}
		mockRestClient.EXPECT().UpdateVolumeFromNode(gomock.Any(), "vol1", update).Return(nil)

		c := NewClient(mockRestClient)
		err := c.UpdateVolume(context.Background(), "vol1", update)

		assert.NoError(t, err)
	})

	t.Run("propagates an error from the controller_api client", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockRestClient := mockcontrollerAPI.NewMockTridentController(mockCtrl)
		names := []string{"a"}
		update := &models.NodeVolumeUpdate{LUKSPassphraseNames: &names}
		mockRestClient.EXPECT().UpdateVolumeFromNode(gomock.Any(), "vol1", update).Return(assert.AnError)

		c := NewClient(mockRestClient)
		err := c.UpdateVolume(context.Background(), "vol1", update)

		assert.Error(t, err)
	})

	t.Run("an empty update still delegates - the no-op decision lives in controller_api, not here", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockRestClient := mockcontrollerAPI.NewMockTridentController(mockCtrl)
		update := &models.NodeVolumeUpdate{}
		mockRestClient.EXPECT().UpdateVolumeFromNode(gomock.Any(), "vol1", update).Return(nil)

		c := NewClient(mockRestClient)
		err := c.UpdateVolume(context.Background(), "vol1", update)

		assert.NoError(t, err)
	})
}
