package csi

import (
	"context"
	"fmt"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	mockControllerAPI "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_api"
	"github.com/netapp/trident/mocks/mock_utils"
	"github.com/netapp/trident/mocks/mock_utils/mock_luks"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

func TestGetVolumeProtocolFromPublishInfo(t *testing.T) {
	// SMB
	trackInfo := &utils.VolumeTrackingInfo{}
	trackInfo.VolumePublishInfo.SMBPath = "foo"

	proto, err := getVolumeProtocolFromPublishInfo(&trackInfo.VolumePublishInfo)
	assert.Equal(t, config.Protocol("file"), proto)
	assert.NoError(t, err)

	// ISCSI
	trackInfo = &utils.VolumeTrackingInfo{}
	iqn := "foo"
	trackInfo.VolumePublishInfo.IscsiTargetIQN = iqn

	proto, err = getVolumeProtocolFromPublishInfo(&trackInfo.VolumePublishInfo)
	assert.Equal(t, config.Protocol("block"), proto)
	assert.NoError(t, err)

	// Block on file
	trackInfo = &utils.VolumeTrackingInfo{}
	testIP := "1.1.1.1"
	subVolName := "foo"
	trackInfo.VolumePublishInfo.NfsServerIP = testIP
	trackInfo.VolumePublishInfo.SubvolumeName = subVolName

	proto, err = getVolumeProtocolFromPublishInfo(&trackInfo.VolumePublishInfo)
	assert.Equal(t, config.Protocol("blockOnFile"), proto)
	assert.NoError(t, err)

	// NFS
	trackInfo = &utils.VolumeTrackingInfo{}
	testIP = "1.1.1.1"
	trackInfo.VolumePublishInfo.NfsServerIP = testIP

	proto, err = getVolumeProtocolFromPublishInfo(&trackInfo.VolumePublishInfo)
	assert.Equal(t, config.Protocol("file"), proto)
	assert.NoError(t, err)

	// Bad Protocol
	trackInfo = &utils.VolumeTrackingInfo{}
	proto, err = getVolumeProtocolFromPublishInfo(&trackInfo.VolumePublishInfo)
	assert.Error(t, err)
}

func TestPerformProtocolSpecificReconciliation_BadProtocol(t *testing.T) {
	trackInfo := &utils.VolumeTrackingInfo{}
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

	trackInfo := &utils.VolumeTrackingInfo{}
	trackInfo.VolumePublishInfo.IscsiTargetIQN = iqn
	mockIscsiUtils.EXPECT().ReconcileISCSIVolumeInfo(context.Background(), trackInfo).Return(false, nil)
	res, err := performProtocolSpecificReconciliation(context.Background(), trackInfo)
	assert.False(t, res)
	assert.NoError(t, err)

	trackInfo = &utils.VolumeTrackingInfo{}
	trackInfo.VolumePublishInfo.IscsiTargetIQN = iqn
	mockIscsiUtils.EXPECT().ReconcileISCSIVolumeInfo(context.Background(), trackInfo).Return(true, nil)
	res, err = performProtocolSpecificReconciliation(context.Background(), trackInfo)
	assert.True(t, res)
	assert.NoError(t, err)

	trackInfo = &utils.VolumeTrackingInfo{}
	trackInfo.VolumePublishInfo.IscsiTargetIQN = iqn
	mockIscsiUtils.EXPECT().ReconcileISCSIVolumeInfo(context.Background(), trackInfo).Return(true, errors.New("error"))
	res, err = performProtocolSpecificReconciliation(context.Background(), trackInfo)
	assert.False(t, res)
	assert.Error(t, err)
}

func TestPerformProtocolSpecificReconciliation_NFS(t *testing.T) {
	defer func() { iscsiUtils = utils.IscsiUtils; bofUtils = utils.BofUtils }()
	trackInfo := &utils.VolumeTrackingInfo{}
	testIP := "1.1.1.1"
	trackInfo.VolumePublishInfo.NfsServerIP = testIP
	res, err := performProtocolSpecificReconciliation(context.Background(), trackInfo)
	assert.False(t, res)
	assert.NoError(t, err)
}

func TestPerformProtocolSpecificReconciliation_BOF(t *testing.T) {
	defer func() { iscsiUtils = utils.IscsiUtils; bofUtils = utils.BofUtils }()
	mockCtrl := gomock.NewController(t)
	bofUtils = mock_utils.NewMockBlockOnFileReconcileUtils(mockCtrl)
	mockBofUtils, ok := bofUtils.(*mock_utils.MockBlockOnFileReconcileUtils)
	if !ok {
		t.Fatal("can't cast bofUtils to mockBofUtils")
	}

	// Block on file
	trackInfo := &utils.VolumeTrackingInfo{}
	testIP := "1.1.1.1"
	subVolName := "foo"
	trackInfo.VolumePublishInfo.NfsServerIP = testIP
	trackInfo.VolumePublishInfo.SubvolumeName = subVolName

	mockBofUtils.EXPECT().ReconcileBlockOnFileVolumeInfo(context.Background(), trackInfo).Return(true, nil)
	res, err := performProtocolSpecificReconciliation(context.Background(), trackInfo)
	assert.True(t, res)
	assert.NoError(t, err)

	mockBofUtils.EXPECT().ReconcileBlockOnFileVolumeInfo(context.Background(), trackInfo).Return(false, nil)
	res, err = performProtocolSpecificReconciliation(context.Background(), trackInfo)
	assert.False(t, res)
	assert.NoError(t, err)

	mockBofUtils.EXPECT().ReconcileBlockOnFileVolumeInfo(context.Background(), trackInfo).Return(true, errors.New("foo"))
	res, err = performProtocolSpecificReconciliation(context.Background(), trackInfo)
	assert.False(t, res)
	assert.Error(t, err)
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

func TestOverrideRequestedValues(t *testing.T) {
	tests := []struct{
		name string
		req csi.CreateVolumeRequest
		volConfig storage.VolumeConfig
		result storage.VolumeConfig
	}{
		{
			name: "Empty",
		},
		{
			name: "LUKSTrue",
			req: csi.CreateVolumeRequest{
				Parameters: map[string]string{
					"selector": "luks=true",
				},
			},
			result: storage.VolumeConfig{
				LUKSEncryption: "true",
			},
		},
		{
			name: "Multiple",
			req: csi.CreateVolumeRequest{
				Parameters: map[string]string{
					"selector": "efg=yes; luks=true; abc=false",
				},
			},
			result: storage.VolumeConfig{
				LUKSEncryption: "true",
			},
		},
		{
			name: "LUKSFalse",
			req: csi.CreateVolumeRequest{
				Parameters: map[string]string{
					"selector": "efg=yes; luks=false; abc=false",
				},
			},
			volConfig: storage.VolumeConfig{
				LUKSEncryption: "true",
			},
			result: storage.VolumeConfig{
				LUKSEncryption: "false",
			},
		},
		{
			name: "Misconfigured",
			req: csi.CreateVolumeRequest{
				Parameters: map[string]string{
					"selector": "efg:yes; luks:false; abc=false",
				},
			},
			volConfig: storage.VolumeConfig{
				LUKSEncryption: "true",
			},
			result: storage.VolumeConfig{
				LUKSEncryption: "true",
			},
		},
		{
			name: "Mixed",
			req: csi.CreateVolumeRequest{
				Parameters: map[string]string{
					"selector": "efg:yes; luks=false; abc/false",
				},
			},
			volConfig: storage.VolumeConfig{
				LUKSEncryption: "true",
			},
			result: storage.VolumeConfig{
				LUKSEncryption: "false",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func (t *testing.T) {
			overrideRequestedValues(&test.req, &test.volConfig)
			if diff := cmp.Diff(test.volConfig, test.result); diff != "" {
				t.Errorf("overrideRequestedValues(): volConfig differs (+got, -want): %s", diff)
			}
		})
	}
}
