// Copyright 2026 NetApp, Inc. All Rights Reserved.

package storage_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockstorage "github.com/netapp/trident/mocks/mock_storage"
	"github.com/netapp/trident/storage"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/utils/errors"
)

func TestAddVolume_FollowupFailure(t *testing.T) {
	tests := map[string]struct {
		createErr      error
		retry          bool
		expectDestroy  bool
		expectCreating bool
	}{
		"new create destroys the volume": {
			expectDestroy: true,
		},
		"retry that creates the volume destroys it": {
			retry:         true,
			expectDestroy: true,
		},
		"existing volume without retry is kept": {
			createErr: drivers.NewVolumeExistsError("vol"),
			retry:     false,
		},
		"existing volume on resumed create is kept and stays in progress": {
			createErr:      drivers.NewVolumeExistsError("vol"),
			retry:          true,
			expectCreating: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			driver := mockstorage.NewMockDriver(ctrl)
			driver.EXPECT().Name().Return("fake").AnyTimes()
			driver.EXPECT().GetStorageBackendSpecs(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

			backend, err := storage.NewStorageBackend(context.Background(), driver)
			assert.NoError(t, err)

			volConfig := &storage.VolumeConfig{Name: "vol", InternalName: "trident_vol"}
			pool := storage.NewStoragePool(backend, "pool")

			driver.EXPECT().Create(gomock.Any(), volConfig, pool, gomock.Any()).Return(tc.createErr)
			followupErr := fmt.Errorf("volume info unavailable")
			driver.EXPECT().CreateFollowup(gomock.Any(), volConfig).Return(followupErr)
			if tc.expectDestroy {
				driver.EXPECT().Destroy(gomock.Any(), volConfig).Return(nil).Times(1)
			}

			vol, err := backend.AddVolume(context.Background(), volConfig, pool, nil, tc.retry)

			assert.Nil(t, vol)
			assert.Error(t, err)
			assert.Equal(t, tc.expectCreating, errors.IsVolumeCreatingError(err))
			assert.ErrorContains(t, err, followupErr.Error())
		})
	}
}
