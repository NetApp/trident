package csi

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	mockControllerAPI "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_api"
	"github.com/netapp/trident/mocks/mock_utils"
	"github.com/netapp/trident/mocks/mock_utils/mock_luks"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

func TestGetVolumeProtocolFromPublishInfo(t *testing.T) {
	// SMB
	trackInfo := &models.VolumeTrackingInfo{}
	trackInfo.VolumePublishInfo.SMBPath = "foo"

	proto, err := getVolumeProtocolFromPublishInfo(&trackInfo.VolumePublishInfo)
	assert.Equal(t, config.Protocol("file"), proto)
	assert.NoError(t, err)

	// ISCSI
	trackInfo = &models.VolumeTrackingInfo{}
	iqn := "foo"
	trackInfo.VolumePublishInfo.IscsiTargetIQN = iqn

	proto, err = getVolumeProtocolFromPublishInfo(&trackInfo.VolumePublishInfo)
	assert.Equal(t, config.Protocol("block"), proto)
	assert.NoError(t, err)

	// NFS
	trackInfo = &models.VolumeTrackingInfo{}
	testIP := "1.1.1.1"
	trackInfo.VolumePublishInfo.NfsServerIP = testIP

	proto, err = getVolumeProtocolFromPublishInfo(&trackInfo.VolumePublishInfo)
	assert.Equal(t, config.Protocol("file"), proto)
	assert.NoError(t, err)

	// Bad Protocol
	trackInfo = &models.VolumeTrackingInfo{}
	proto, err = getVolumeProtocolFromPublishInfo(&trackInfo.VolumePublishInfo)
	assert.Error(t, err)
}

func TestPerformProtocolSpecificReconciliation_BadProtocol(t *testing.T) {
	trackInfo := &models.VolumeTrackingInfo{}
	res, err := performProtocolSpecificReconciliation(context.Background(), trackInfo)
	assert.False(t, res)
	assert.Error(t, err)
	assert.True(t, IsInvalidTrackingFileError(err), "expected invalid tracking file when protocol can't be"+
		" determined")
}

func TestPerformProtocolSpecificReconciliation_ISCSI(t *testing.T) {
	defer func() { iscsiUtils = utils.IscsiUtils }()
	mockCtrl := gomock.NewController(t)
	iscsiUtils = mock_utils.NewMockIscsiReconcileUtils(mockCtrl)
	mockIscsiUtils, ok := iscsiUtils.(*mock_utils.MockIscsiReconcileUtils)
	if !ok {
		t.Fatal("can't cast iscsiUtils to mockIscsiUtils")
	}

	iqn := "bar"

	trackInfo := &models.VolumeTrackingInfo{}
	trackInfo.VolumePublishInfo.IscsiTargetIQN = iqn
	mockIscsiUtils.EXPECT().ReconcileISCSIVolumeInfo(context.Background(), trackInfo).Return(false, nil)
	res, err := performProtocolSpecificReconciliation(context.Background(), trackInfo)
	assert.False(t, res)
	assert.NoError(t, err)

	trackInfo = &models.VolumeTrackingInfo{}
	trackInfo.VolumePublishInfo.IscsiTargetIQN = iqn
	mockIscsiUtils.EXPECT().ReconcileISCSIVolumeInfo(context.Background(), trackInfo).Return(true, nil)
	res, err = performProtocolSpecificReconciliation(context.Background(), trackInfo)
	assert.True(t, res)
	assert.NoError(t, err)

	trackInfo = &models.VolumeTrackingInfo{}
	trackInfo.VolumePublishInfo.IscsiTargetIQN = iqn
	mockIscsiUtils.EXPECT().ReconcileISCSIVolumeInfo(context.Background(), trackInfo).Return(true, errors.New("error"))
	res, err = performProtocolSpecificReconciliation(context.Background(), trackInfo)
	assert.False(t, res)
	assert.Error(t, err)
}

func TestPerformProtocolSpecificReconciliation_NFS(t *testing.T) {
	defer func() { iscsiUtils = utils.IscsiUtils }()
	trackInfo := &models.VolumeTrackingInfo{}
	testIP := "1.1.1.1"
	trackInfo.VolumePublishInfo.NfsServerIP = testIP
	res, err := performProtocolSpecificReconciliation(context.Background(), trackInfo)
	assert.False(t, res)
	assert.NoError(t, err)
}

func TestEnsureLUKSVolumePassphrase(t *testing.T) {
	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Passphrase already latest
	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice := mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets := map[string]string{
		"luks-passphrase-name": "A",
		"luks-passphrase":      "passphraseA",
	}
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseA").Return(true, nil)
	err := ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, false)
	assert.NoError(t, err)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Passphrase already latest, force update
	mockCtrl = gomock.NewController(t)
	mockClient = mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets = map[string]string{
		"luks-passphrase-name": "A",
		"luks-passphrase":      "passphraseA",
	}
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseA").Return(true, nil)
	mockClient.EXPECT().UpdateVolumeLUKSPassphraseNames(gomock.Any(), "test-vol", []string{"A"}).Return(nil)
	err = ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, true)
	assert.NoError(t, err)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Passphrase not correct, but previous passphrase is correct, rotation needed
	mockCtrl = gomock.NewController(t)
	mockClient = mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets = map[string]string{
		"luks-passphrase-name":          "B",
		"luks-passphrase":               "passphraseB",
		"previous-luks-passphrase-name": "A",
		"previous-luks-passphrase":      "passphraseA",
	}
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseB").Return(false, nil)
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseA").Return(true, nil)
	mockClient.EXPECT().UpdateVolumeLUKSPassphraseNames(gomock.Any(), "test-vol", []string{"B", "A"}).Return(nil)
	mockLUKSDevice.EXPECT().RotatePassphrase(gomock.Any(), "test-vol", "passphraseA", "passphraseB").Return(nil)
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseB").Return(true, nil)
	mockClient.EXPECT().UpdateVolumeLUKSPassphraseNames(gomock.Any(), "test-vol", []string{"B"}).Return(nil)
	err = ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, false)
	assert.NoError(t, err)
	mockCtrl.Finish()
}

func TestEnsureLUKSVolumePassphrase_Error(t *testing.T) {
	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Checking passphrase is correct errors
	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice := mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets := map[string]string{
		"luks-passphrase-name":          "B",
		"luks-passphrase":               "passphraseB",
		"previous-luks-passphrase-name": "A",
		"previous-luks-passphrase":      "passphraseA",
	}
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseB").Return(false, fmt.Errorf("test error"))
	err := ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, false)
	assert.Error(t, err)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Checking previous passphrase is correct errors
	mockCtrl = gomock.NewController(t)
	mockClient = mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets = map[string]string{
		"luks-passphrase-name":          "B",
		"luks-passphrase":               "passphraseB",
		"previous-luks-passphrase-name": "A",
		"previous-luks-passphrase":      "passphraseA",
	}
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseB").Return(false, nil)
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseA").Return(false, fmt.Errorf("test error"))
	err = ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, false)
	assert.Error(t, err)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Sending pre-rotation passphrases to trident controller fails
	mockCtrl = gomock.NewController(t)
	mockClient = mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets = map[string]string{
		"luks-passphrase-name":          "B",
		"luks-passphrase":               "passphraseB",
		"previous-luks-passphrase-name": "A",
		"previous-luks-passphrase":      "passphraseA",
	}
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseB").Return(false, nil)
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseA").Return(true, nil)
	mockClient.EXPECT().UpdateVolumeLUKSPassphraseNames(gomock.Any(), "test-vol", []string{"B", "A"}).Return(fmt.Errorf("test error"))
	err = ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, false)
	assert.Error(t, err)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Passphrase rotation fails
	mockCtrl = gomock.NewController(t)
	mockClient = mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets = map[string]string{
		"luks-passphrase-name":          "B",
		"luks-passphrase":               "passphraseB",
		"previous-luks-passphrase-name": "A",
		"previous-luks-passphrase":      "passphraseA",
	}
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseB").Return(false, nil)
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseA").Return(true, nil)
	mockClient.EXPECT().UpdateVolumeLUKSPassphraseNames(gomock.Any(), "test-vol", []string{"B", "A"}).Return(nil)
	mockLUKSDevice.EXPECT().RotatePassphrase(gomock.Any(), "test-vol", "passphraseA", "passphraseB").Return(fmt.Errorf("test error"))
	err = ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, false)
	assert.Error(t, err)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Verifying passphrase rotation fails
	mockCtrl = gomock.NewController(t)
	mockClient = mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets = map[string]string{
		"luks-passphrase-name":          "B",
		"luks-passphrase":               "passphraseB",
		"previous-luks-passphrase-name": "A",
		"previous-luks-passphrase":      "passphraseA",
	}
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseB").Return(false, nil)
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseA").Return(true, nil)
	mockClient.EXPECT().UpdateVolumeLUKSPassphraseNames(gomock.Any(), "test-vol", []string{"B", "A"}).Return(nil)
	mockLUKSDevice.EXPECT().RotatePassphrase(gomock.Any(), "test-vol", "passphraseA", "passphraseB").Return(nil)
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseB").Return(true, fmt.Errorf("test error"))
	err = ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, false)
	assert.Error(t, err)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Sending post-rotation passphrases to trident controller fails
	mockCtrl = gomock.NewController(t)
	mockClient = mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets = map[string]string{
		"luks-passphrase-name":          "B",
		"luks-passphrase":               "passphraseB",
		"previous-luks-passphrase-name": "A",
		"previous-luks-passphrase":      "passphraseA",
	}
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseB").Return(false, nil)
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseA").Return(true, nil)
	mockClient.EXPECT().UpdateVolumeLUKSPassphraseNames(gomock.Any(), "test-vol", []string{"B", "A"}).Return(nil)
	mockLUKSDevice.EXPECT().RotatePassphrase(gomock.Any(), "test-vol", "passphraseA", "passphraseB").Return(nil)
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseB").Return(true, nil)
	mockClient.EXPECT().UpdateVolumeLUKSPassphraseNames(gomock.Any(), "test-vol", []string{"B"}).Return(fmt.Errorf("test error"))
	err = ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, false)
	assert.Error(t, err)
	mockCtrl.Finish()
}

func TestEnsureLUKSVolumePassphrase_InvalidSecret(t *testing.T) {
	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: No passphrases
	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice := mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets := map[string]string{}
	err := ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, false)
	assert.Error(t, err)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: passphrase but no passphrase name
	mockCtrl = gomock.NewController(t)
	mockClient = mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets = map[string]string{
		"luks-passphrase": "passphraseA",
	}
	err = ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, false)
	assert.Error(t, err)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: passphrase but empty passphrase name
	mockCtrl = gomock.NewController(t)
	mockClient = mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets = map[string]string{
		"luks-passphrase-name": "A",
		"luks-passphrase":      "",
	}
	err = ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, false)
	assert.Error(t, err)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: passphrase name but no passphrase name
	mockCtrl = gomock.NewController(t)
	mockClient = mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets = map[string]string{
		"luks-passphrase-name": "A",
	}
	err = ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, false)
	assert.Error(t, err)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: passphrase name but empty passphrase name
	mockCtrl = gomock.NewController(t)
	mockClient = mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets = map[string]string{
		"luks-passphrase-name": "A",
		"luks-passphrase":      "",
	}
	err = ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, false)
	assert.Error(t, err)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: passphrase valid, previous luks passphrase valid but no name
	mockCtrl = gomock.NewController(t)
	mockClient = mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets = map[string]string{
		"luks-passphrase-name":     "A",
		"luks-passphrase":          "passphraseA",
		"previous-luks-passphrase": "passphraseB",
	}
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseA").Return(true, nil)
	err = ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, false)
	assert.NoError(t, err)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: passphrase valid, previous luks passphrase valid but empty name
	mockCtrl = gomock.NewController(t)
	mockClient = mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets = map[string]string{
		"luks-passphrase-name":          "A",
		"luks-passphrase":               "passphraseA",
		"previous-luks-passphrase-name": "",
		"previous-luks-passphrase":      "passphraseB",
	}
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseA").Return(true, nil)
	err = ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, false)
	assert.NoError(t, err)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: passphrase valid, previous luks passphrase name valid but no passphrase
	mockCtrl = gomock.NewController(t)
	mockClient = mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets = map[string]string{
		"luks-passphrase-name":          "A",
		"luks-passphrase":               "passphraseA",
		"previous-luks-passphrase-name": "B",
	}
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseA").Return(true, nil)
	err = ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, false)
	assert.NoError(t, err)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: passphrase valid, previous luks passphrase name valid but empty passphrase
	mockCtrl = gomock.NewController(t)
	mockClient = mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets = map[string]string{
		"luks-passphrase-name":          "A",
		"luks-passphrase":               "passphraseA",
		"previous-luks-passphrase-name": "B",
		"previous-luks-passphrase":      "",
	}
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseA").Return(true, nil)
	err = ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, false)
	assert.NoError(t, err)
	mockCtrl.Finish()
}

func TestEnsureLUKSVolumePassphrase_NoCorrectPassphraseProvided(t *testing.T) {
	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Passphrase not correct and no previous specified
	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice := mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets := map[string]string{
		"luks-passphrase-name": "A",
		"luks-passphrase":      "passphraseA",
	}
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseA").Return(false, nil)
	err := ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, false)
	assert.Error(t, err)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Passphrase not correct and incorrect previous
	mockCtrl = gomock.NewController(t)
	mockClient = mockControllerAPI.NewMockTridentController(mockCtrl)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets = map[string]string{
		"luks-passphrase-name":          "B",
		"luks-passphrase":               "passphraseB",
		"previous-luks-passphrase-name": "A",
		"previous-luks-passphrase":      "passphraseA",
	}
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseB").Return(false, nil)
	mockLUKSDevice.EXPECT().CheckPassphrase(gomock.Any(), "passphraseA").Return(false, nil)
	err = ensureLUKSVolumePassphrase(context.TODO(), mockClient, mockLUKSDevice, "test-vol", secrets, false)
	assert.Error(t, err)
	mockCtrl.Finish()
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		b        string
		expected bool
	}{
		{
			b:        "true",
			expected: true,
		},
		{
			b:        "false",
			expected: false,
		},
		{
			b:        "not a value",
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.b, func(t *testing.T) {
			actual := utils.ParseBool(test.b)
			assert.Equal(t, test.expected, actual)
		})
	}
}
