package csi

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/mocks/mock_utils"
	"github.com/netapp/trident/utils"
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
