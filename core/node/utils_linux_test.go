// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mock_exec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/utils/errors"
)

func TestRecordInitialLUKSPassphraseName_ExistingHeaderSkipsUpdate(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.Command.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", gomock.Any(), true, "", "isLuks", "/dev/sdz",
	).Return([]byte{}, nil)
	publishInfo := samplePublishInfo(ISCSI)
	publishInfo.LUKSEncryption = "true"

	err := core.recordInitialLUKSPassphraseName(context.Background(), "vol-1", publishInfo, nil)
	assert.NoError(t, err)
}

func TestRecordInitialLUKSPassphraseName_RecordsNameWhenUnformatted(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.Command.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", gomock.Any(), true, "", "isLuks", "/dev/sdz",
	).Return([]byte{}, mock_exec.NewMockExitError(1, "not luks"))
	mocks.Devices.EXPECT().IsDeviceUnformatted(gomock.Any(), "/dev/sdz").Return(true, nil)
	mocks.Controller.MockClient.EXPECT().UpdateVolume(gomock.Any(), "vol-1", luksNamesUpdate("name-a")).Return(nil)
	publishInfo := samplePublishInfo(ISCSI)
	publishInfo.LUKSEncryption = "true"

	err := core.recordInitialLUKSPassphraseName(
		context.Background(), "vol-1", publishInfo,
		map[string]string{"luks-passphrase": "secret", "luks-passphrase-name": "name-a"},
	)
	assert.NoError(t, err)
}

func TestRecordInitialLUKSPassphraseName_NotEmptySkipsUpdate(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.Command.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", gomock.Any(), true, "", "isLuks", "/dev/sdz",
	).Return([]byte{}, mock_exec.NewMockExitError(1, "not luks"))
	mocks.Devices.EXPECT().IsDeviceUnformatted(gomock.Any(), "/dev/sdz").Return(false, nil)
	publishInfo := samplePublishInfo(ISCSI)
	publishInfo.LUKSEncryption = "true"

	err := core.recordInitialLUKSPassphraseName(
		context.Background(), "vol-1", publishInfo,
		map[string]string{"luks-passphrase": "secret", "luks-passphrase-name": "name-a"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "device is not empty")
}

func TestRecordInitialLUKSPassphraseName_UpdateErrorPropagates(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.Command.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", gomock.Any(), true, "", "isLuks", "/dev/sdz",
	).Return([]byte{}, mock_exec.NewMockExitError(1, "not luks"))
	mocks.Devices.EXPECT().IsDeviceUnformatted(gomock.Any(), "/dev/sdz").Return(true, nil)
	mocks.Controller.MockClient.EXPECT().UpdateVolume(gomock.Any(), "vol-1", luksNamesUpdate("name-a")).
		Return(errors.New("controller unavailable"))
	publishInfo := samplePublishInfo(ISCSI)
	publishInfo.LUKSEncryption = "true"

	err := core.recordInitialLUKSPassphraseName(
		context.Background(), "vol-1", publishInfo,
		map[string]string{"luks-passphrase": "secret", "luks-passphrase-name": "name-a"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not record LUKS passphrase name before format")
	assert.Contains(t, err.Error(), "controller unavailable")
}

func TestRecordInitialLUKSPassphraseName_CheckErrorPropagates(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.Command.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", gomock.Any(), true, "", "isLuks", "/dev/sdz",
	).Return([]byte{}, errors.New("device busy"))
	publishInfo := samplePublishInfo(ISCSI)
	publishInfo.LUKSEncryption = "true"

	err := core.recordInitialLUKSPassphraseName(context.Background(), "vol-1", publishInfo, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check if device is LUKS formatted")
	assert.Contains(t, err.Error(), "device busy")
}
