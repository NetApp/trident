// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mock_exec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/pkg/locks/distlock"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

func TestAttachISCSIVolume_LUKSFormatErrorPropagates(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(ISCSI)
	publishInfo.LUKSEncryption = "true"

	mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil)
	mocks.Command.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", gomock.Any(), true, "", "isLuks", publishInfo.DevicePath,
	).Return([]byte{}, mock_exec.NewMockExitError(1, "not luks"))
	mocks.Devices.EXPECT().IsDeviceUnformatted(gomock.Any(), publishInfo.DevicePath).Return(true, nil)
	// EnsureVolumeFormattedAndMounted / AddSession must never be reached: the device is not LUKS
	// formatted and no passphrase was supplied, so formatting never starts.

	err := core.attachISCSIVolume(context.Background(), "test-volume", publishInfo, map[string]string{}, distlock.NewNoopLock())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LUKS passphrase cannot be empty")
}

func TestAttachISCSIVolume_InitialPassphraseUpdateErrorSkipsFormat(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(ISCSI)
	publishInfo.LUKSEncryption = "true"
	secrets := map[string]string{
		"luks-passphrase":      "secret",
		"luks-passphrase-name": "name-a",
	}

	mocks.ISCSI.EXPECT().AttachVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil)
	mocks.Command.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", gomock.Any(), true, "", "isLuks", publishInfo.DevicePath,
	).Return([]byte{}, mock_exec.NewMockExitError(1, "not luks"))
	mocks.Devices.EXPECT().IsDeviceUnformatted(gomock.Any(), publishInfo.DevicePath).Return(true, nil)
	mocks.Controller.MockClient.EXPECT().UpdateVolume(
		gomock.Any(), "test-volume", &models.NodeVolumeUpdate{LUKSPassphraseNames: &[]string{"name-a"}},
	).Return(errors.New("controller unavailable"))

	err := core.attachISCSIVolume(context.Background(), "test-volume", publishInfo, secrets, distlock.NewNoopLock())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not record LUKS passphrase name before format")
	assert.Contains(t, err.Error(), "controller unavailable")
}

// The initial passphrase name, the format and the passphrase check run inside the driver (the luks service
// records the name before the format; see its tests). The core's part is to hand the driver the LUKS
// intent and the secrets, under the volume ID the passphrase name is recorded against.
func TestAttachNVMeVolume_PassesLUKSSecretsToDriver(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)
	publishInfo.LUKSEncryption = "true"
	publishInfo.InternalID = "internal-id"
	secrets := map[string]string{
		"luks-passphrase":      "secret",
		"luks-passphrase-name": "name-a",
	}

	mocks.NVMeDriver.EXPECT().Attach(gomock.Any(), "test-volume", gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, spec *models.VolumeAttachSpec) (*models.VolumeAttachState, error) {
			assert.True(t, spec.Encryption.IsLUKS())
			assert.Equal(t, secrets, spec.Secrets)
			assert.Equal(t, "internal-id", spec.InternalID, "the mapper is named from the internal ID")
			return &models.VolumeAttachState{}, nil
		})

	_, err := attachNVMe(core, mocks, publishInfo, secrets)
	assert.NoError(t, err)
}
