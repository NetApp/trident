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
	"github.com/netapp/trident/utils/nvme"
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

func TestAttachNVMeVolume_RecordsInitialPassphraseBeforeFormat(t *testing.T) {
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)
	publishInfo.LUKSEncryption = "true"
	secrets := map[string]string{
		"luks-passphrase":      "secret",
		"luks-passphrase-name": "name-a",
	}

	gomock.InOrder(
		mocks.NVMe.EXPECT().AddPublishedNVMeSession(gomock.Any(), publishInfo),
		mocks.NVMe.EXPECT().AttachNVMeVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
		mocks.Command.EXPECT().ExecuteWithTimeoutAndInput(
			gomock.Any(), "cryptsetup", gomock.Any(), true, "", "isLuks", publishInfo.DevicePath,
		).Return([]byte{}, mock_exec.NewMockExitError(1, "not luks")),
		mocks.Devices.EXPECT().IsDeviceUnformatted(gomock.Any(), publishInfo.DevicePath).Return(true, nil),
		mocks.Controller.MockClient.EXPECT().UpdateVolume(
			gomock.Any(), "test-volume", &models.NodeVolumeUpdate{LUKSPassphraseNames: &[]string{"name-a"}},
		).Return(nil),
		mocks.NVMe.EXPECT().EnsureCryptsetupFormattedAndMappedOnHost(
			gomock.Any(), publishInfo.InternalID, publishInfo, secrets,
		).Return(true, true, nil),
		mocks.NVMe.EXPECT().EnsureVolumeFormattedAndMounted(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(nil),
		mocks.Command.EXPECT().ExecuteWithTimeoutAndInput(
			gomock.Any(), "cryptsetup", gomock.Any(), true, "secret", "open", publishInfo.DevicePath,
			gomock.Any(), "--type", "luks2", "--test-passphrase",
		).Return([]byte{}, nil),
	)

	err := core.attachNVMeVolume(context.Background(), "test-volume", publishInfo, secrets)
	assert.NoError(t, err)
}

// A failure after the volume is mounted must not disconnect the subsystem under the mount. The
// namespace stays published so NodeUnstageVolume can tear it down.
func TestAttachNVMeVolume_PostMountErrorKeepsSessionAndSkipsDisconnect(t *testing.T) {
	withCleanPublishedNVMeSessions(t)
	core, mocks := newTestCore(t)
	publishInfo := samplePublishInfo(NVMe)
	publishInfo.LUKSEncryption = "true"

	mocks.NVMe.EXPECT().AddPublishedNVMeSession(gomock.Any(), publishInfo).
		Do(func(sessions *nvme.NVMeSessions, info *models.VolumePublishInfo) {
			sessions.AddNVMeSession(nvme.NVMeSubsystem{NQN: info.NVMeSubsystemNQN}, info.NVMeTargetIPs)
			sessions.AddNamespaceToSession(info.NVMeSubsystemNQN, info.NVMeNamespaceUUID, nil)
		})
	mocks.NVMe.EXPECT().AttachNVMeVolumeRetry(gomock.Any(), publishInfo, gomock.Any()).Return(nil)
	mocks.Command.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", gomock.Any(), true, "", "isLuks", publishInfo.DevicePath,
	).Return([]byte{}, nil)
	mocks.NVMe.EXPECT().EnsureCryptsetupFormattedAndMappedOnHost(
		gomock.Any(), publishInfo.InternalID, publishInfo, gomock.Nil(),
	).Return(true, true, nil)
	mocks.NVMe.EXPECT().EnsureVolumeFormattedAndMounted(
		gomock.Any(), publishInfo.InternalID, "", publishInfo, true, true,
	).Return(nil)
	// RemovePublishedNVMeSession and NewNVMeSubsystem are intentionally not expected.

	err := core.attachNVMeVolume(context.Background(), "test-volume", publishInfo, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not set LUKS volume passphrase")
	assert.Equal(t, 1, publishedNVMeSessions.GetNamespaceCountForSession(publishInfo.NVMeSubsystemNQN))
}
