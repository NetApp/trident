// Copyright 2025 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/mocks/mock_utils/mock_filesystem"
	"github.com/netapp/trident/mocks/mock_utils/mock_iscsi"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
)

func TestNewVolumePublishManager(t *testing.T) {
	aPath := "foo"
	v := NewVolumePublishManagerDetailed(aPath, filesystem.New(nil), afero.NewOsFs())
	assert.Equal(t, aPath, v.volumeTrackingInfoPath, "volume publish manager did not contain expected path")
}

func TestGetVolumeTrackingFiles(t *testing.T) {
	osFs := afero.NewMemMapFs()
	v := NewVolumePublishManagerDetailed(config.VolumeTrackingInfoPath, filesystem.New(nil), osFs)

	trackPath := config.VolumeTrackingInfoPath
	_, err := osFs.Create(path.Join(trackPath, "pvc-123"))
	assert.NoError(t, err, "expected a test file to be created")

	files, err := v.GetVolumeTrackingFiles()
	assert.NoError(t, err, "expected no error getting volume tracking files")
	assert.True(t, len(files) == 1, "expected exactly one file to be found")

	_, err = osFs.Create(path.Join(trackPath, "pvc-456"))
	assert.NoError(t, err, "expected a test file to be created")
	_, err = osFs.Create(path.Join(trackPath, "pvc-789"))
	assert.NoError(t, err, "expected a test file to be created")

	files, err = v.GetVolumeTrackingFiles()
	assert.NoError(t, err, "expected no error getting volume tracking files")
	assert.True(t, len(files) == 3, "expected exactly three files to be found")
}

func TestWriteTrackingInfo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockJSONUtils := mock_filesystem.NewMockJSONReaderWriter(mockCtrl)

	osFs := afero.NewMemMapFs()

	defer func(previousJsonRW filesystem.JSONReaderWriter) {
		jsonRW = previousJsonRW
	}(jsonRW)
	jsonRW = mockJSONUtils

	v := NewVolumePublishManagerDetailed("", filesystem.New(nil), osFs)

	volId := "pvc-123"
	fName := volId + ".json"
	trackInfo := &models.VolumeTrackingInfo{}
	trackInfo.FilesystemType = "ext4"
	trackInfo.StagingTargetPath = "."
	_, err := osFs.Create("tmp-" + fName)
	assert.NoError(t, err, "expected a test file to be created")

	mockJSONUtils.EXPECT().WriteJSONFile(gomock.Any(), trackInfo, "tmp-"+fName, "volume tracking info").
		Return(nil)
	err = v.WriteTrackingInfo(ctx, volId, trackInfo)
	assert.NoError(t, err, "no error expected when write succeeds")

	mockJSONUtils.EXPECT().WriteJSONFile(gomock.Any(), trackInfo, "tmp-"+fName, "volume tracking info").
		Return(errors.InvalidJSONError("foo"))
	err = v.WriteTrackingInfo(ctx, volId, trackInfo)
	assert.Error(t, err, "error expected when write tracking info fails")
	assert.True(t, errors.IsInvalidJSONError(err), "expected actual error we threw")
}

func TestReadTrackingInfo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockJSONUtils := mock_filesystem.NewMockJSONReaderWriter(mockCtrl)

	defer func(previousJsonRW filesystem.JSONReaderWriter) {
		jsonRW = previousJsonRW
	}(jsonRW)
	jsonRW = mockJSONUtils

	osFs := afero.NewMemMapFs()
	v := NewVolumePublishManagerDetailed("", filesystem.New(nil), osFs)

	volId := "pvc-123"
	fName := volId + ".json"
	trackInfo := &models.VolumeTrackingInfo{}
	trackInfo.FilesystemType = "ext4"
	trackInfo.StagingTargetPath = "."
	fsType := "ext4"
	emptyTrackInfo := &models.VolumeTrackingInfo{}

	// SetArg sets the supplied argument to the given value. ReadJSONFile accepts an interface{} and unmarshals the JSON
	// into the provided struct. It accepts an interface{} so that it can generically read any JSON file, regardless of
	// the struct that was used to populate it.
	mockJSONUtils.EXPECT().ReadJSONFile(gomock.Any(), emptyTrackInfo, fName, "volume tracking info").
		SetArg(1, *trackInfo).Return(nil)
	trackInfo, err := v.ReadTrackingInfo(context.Background(), volId)
	assert.NoError(t, err, "no error expected when write succeed")
	assert.NotNil(t, trackInfo, "expected a valid tracking info")
	assert.Equal(t, fsType, trackInfo.FilesystemType, "tracking file did not have expected value in it")

	emptyTrackInfo = &models.VolumeTrackingInfo{}
	mockJSONUtils.EXPECT().ReadJSONFile(gomock.Any(), emptyTrackInfo, fName,
		"volume tracking info").Return(errors.NotFoundError("not found"))
	trackInfo, err = v.ReadTrackingInfo(context.Background(), volId)
	assert.Error(t, err, "expected error when reading the file results in an error")
	assert.Nil(t, trackInfo, "expected nil tracking info")
	assert.True(t, errors.IsNotFoundError(err), "expected not found error")
}

func TestListVolumeTrackingInfo_FailsToGetVolumeTrackingFiles(t *testing.T) {
	// Set up the volume publish manager.
	trackPath := config.VolumeTrackingInfoPath
	osFs := afero.NewMemMapFs()
	v := NewVolumePublishManagerDetailed(trackPath, filesystem.New(nil), osFs)

	// Without setting up the directory or files in memory, this will fail at GetVolumeTrackingFiles.
	allTrackingInfo, err := v.ListVolumeTrackingInfo(ctx)
	assert.Error(t, err, "expected error")
	assert.Empty(t, allTrackingInfo, "expected no tracking info to exist")
}

func TestListVolumeTrackingInfo_FailsWhenNoTrackingFilesFound(t *testing.T) {
	// Set up the volume tracking directory and publish manager.
	trackPath := config.VolumeTrackingInfoPath
	osFs := afero.NewMemMapFs()
	v := NewVolumePublishManagerDetailed(trackPath, filesystem.New(nil), osFs)
	if err := osFs.Mkdir(trackPath, 0o600); err != nil {
		t.Fatalf("failed to create tracking file directory in memory fs for testing; %v", err)
	}

	allTrackingInfo, err := v.ListVolumeTrackingInfo(ctx)
	assert.Error(t, err, "expected error")
	assert.Nil(t, allTrackingInfo, "expected no tracking info to exist")
}

func TestListVolumeTrackingInfo_FailsToReadTrackingInfo(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	jsonReaderWriter := mock_filesystem.NewMockJSONReaderWriter(mockCtrl)
	defer func(previousJsonRW filesystem.JSONReaderWriter) {
		jsonRW = previousJsonRW
	}(jsonRW)
	jsonRW = jsonReaderWriter

	// Set up the volume tracking directory and files.
	trackPath := config.VolumeTrackingInfoPath
	osFs := afero.NewMemMapFs()
	v := NewVolumePublishManagerDetailed(trackPath, filesystem.New(nil), osFs)
	volumeOne := "pvc-85987a99-648d-4d84-95df-47d0256ca2ab"
	volumeTrackingInfo := &models.VolumeTrackingInfo{
		VolumePublishInfo: models.VolumePublishInfo{},
		StagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/csi.trident.netapp.io/" +
			"6b1f46a23d50f8d6a2e2f24c63c3b6e73f82e8b982bdb41da4eb1d0b49d787dd/globalmount",
		PublishedPaths: map[string]struct{}{
			"/var/lib/kubelet/pods/b9f476af-47f4-42d8-8cfa-70d49394d9e3/volumes/kubernetes.io~csi/" +
				volumeOne + "/mount": {},
		},
	}
	file := path.Join(trackPath, volumeOne) + ".json"
	if _, err := osFs.Create(file); err != nil {
		t.Fatalf("failed to create tracking file in memory fs for testing; %v", err)
	}
	data, err := json.Marshal(volumeTrackingInfo)
	if err != nil {
		t.Fatalf("failed to create tracking data for in memory fs for testing; %v", err)
	}
	if err := afero.WriteFile(osFs, file, data, 0o600); err != nil {
		t.Fatalf("failed to create tracking data for in memory fs for testing; %v", err)
	}

	// Fail to read the tracking file for a given volume.
	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), file, "volume tracking info").
		SetArg(1, *volumeTrackingInfo).
		Return(errors.New("unable to read tracking info"))

	allTrackingInfo, err := v.ListVolumeTrackingInfo(ctx)
	assert.Error(t, err, "expected error")
	assert.Empty(t, allTrackingInfo, "expected no tracking info to exist")

	actualTrackingInfo, ok := allTrackingInfo[volumeOne]
	assert.False(t, ok, "expected false")
	assert.NotEqualValues(t, volumeTrackingInfo, actualTrackingInfo, "expected tracking info to be different")
}

func TestListVolumeTrackingInfo_SucceedsToListTrackingFileInformation(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	defer func(original filesystem.JSONReaderWriter) { jsonRW = original }(jsonRW)
	jsonReaderWriter := mock_filesystem.NewMockJSONReaderWriter(mockCtrl)
	jsonRW = jsonReaderWriter

	// Set up the volume tracking directory and files.
	trackPath := config.VolumeTrackingInfoPath
	osFs := afero.NewMemMapFs()
	v := NewVolumePublishManagerDetailed(trackPath, filesystem.New(nil), osFs)
	volumeOne := "pvc-85987a99-648d-4d84-95df-47d0256ca2ab"
	volumeTrackingInfo := &models.VolumeTrackingInfo{
		VolumePublishInfo: models.VolumePublishInfo{},
		StagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/csi.trident.netapp.io/" +
			"6b1f46a23d50f8d6a2e2f24c63c3b6e73f82e8b982bdb41da4eb1d0b49d787dd/globalmount",
		PublishedPaths: map[string]struct{}{
			"/var/lib/kubelet/pods/b9f476af-47f4-42d8-8cfa-70d49394d9e3/volumes/kubernetes.io~csi/" +
				volumeOne + "/mount": {},
		},
	}
	file := path.Join(trackPath, volumeOne) + ".json"
	if _, err := osFs.Create(file); err != nil {
		t.Fatalf("failed to create tracking file in memory fs for testing; %v", err)
	}
	data, err := json.Marshal(volumeTrackingInfo)
	if err != nil {
		t.Fatalf("failed to create tracking data for in memory fs for testing; %v", err)
	}
	if err := afero.WriteFile(osFs, file, data, 0o600); err != nil {
		t.Fatalf("failed to create tracking data for in memory fs for testing; %v", err)
	}

	// Happy path.
	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), file, "volume tracking info").
		SetArg(1, *volumeTrackingInfo).
		Return(nil)

	allTrackingInfo, err := v.ListVolumeTrackingInfo(ctx)
	assert.NoError(t, err, "expected no error")
	assert.NotEmpty(t, allTrackingInfo, "expected tracking info to exist")

	actualTrackingInfo, ok := allTrackingInfo[volumeOne]
	assert.True(t, ok, "expected true")
	assert.EqualValues(t, volumeTrackingInfo, actualTrackingInfo, "expected tracking info to be the same")
}

func TestDeleteTrackingInfo(t *testing.T) {
	volName := "pvc-123"

	osFs := afero.NewMemMapFs()
	mockFilesytesm := mock_filesystem.NewMockFilesystem(gomock.NewController(t))
	v := NewVolumePublishManagerDetailed("", mockFilesytesm, osFs)
	mockFilesytesm.EXPECT().DeleteFile(gomock.Any(), gomock.Any(), gomock.Any()).Return("", nil)

	err := v.DeleteTrackingInfo(context.Background(), volName)
	assert.NoError(t, err, "expected no error deleting the tracking info")

	mockFilesytesm.EXPECT().DeleteFile(gomock.Any(), gomock.Any(), gomock.Any()).Return("", errors.InvalidJSONError("foo"))
	err = v.DeleteTrackingInfo(context.Background(), volName)
	assert.Error(t, err, "expected error if delete tracking info fails")
	assert.True(t, errors.IsInvalidJSONError(err), "expected the error we threw")
}

func TestUpgradeVolumeTrackingFile(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	jsonReaderWriter := mock_filesystem.NewMockJSONReaderWriter(mockCtrl)
	defer func(previousJsonRW filesystem.JSONReaderWriter) {
		jsonRW = previousJsonRW
	}(jsonRW)
	jsonRW = jsonReaderWriter

	osFs := afero.NewMemMapFs()
	trackPath := "/bar"
	v := NewVolumePublishManagerDetailed(trackPath, filesystem.New(nil), osFs)

	stagePath := "/foo"
	trackInfoAndPath := models.VolumeTrackingInfo{}
	trackInfoAndPath.StagingTargetPath = stagePath
	pubInfoNfsIp := models.VolumePublishInfo{}
	pubInfoNfsIp.NfsServerIP = "1.1.1.1"
	volName := "pvc-123"
	fName := volName + ".json"
	pubPaths := map[string]struct{}{}
	stagedDeviceInfo := path.Join(stagePath, volumePublishInfoFilename)
	trackingInfoFile := path.Join(trackPath, fName)
	tmpTrackingInfoFile := path.Join(trackPath, "tmp-"+fName)

	// Happy path.
	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), trackingInfoFile, "volume tracking info").
		SetArg(1, trackInfoAndPath).
		Return(nil)
	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), stagedDeviceInfo, "publish info").
		SetArg(1, pubInfoNfsIp).
		Return(nil)
	jsonReaderWriter.EXPECT().WriteJSONFile(gomock.Any(), gomock.Any(), tmpTrackingInfoFile, "volume tracking info").
		Return(nil)
	_, err := osFs.Create(tmpTrackingInfoFile)
	assert.NoError(t, err, "expected a test tmp file to be created without error")

	needsDelete, err := v.UpgradeVolumeTrackingFile(context.Background(), volName, pubPaths, nil)
	assert.False(t, needsDelete, "expected to be told to keep tracking file")
	assert.NoError(t, err, "expected tracking file upgrade to work")

	// Legacy tracking file exists, and staging path exists, but no stagedDeviceInfo!
	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), trackingInfoFile, "volume tracking info").
		SetArg(1, trackInfoAndPath).
		Return(nil)
	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), stagedDeviceInfo, "publish info").
		Return(errors.NotFoundError("foo"))
	needsDelete, err = v.UpgradeVolumeTrackingFile(context.Background(), volName, pubPaths, nil)
	assert.True(t, needsDelete, "failure to upgrade should cause the tracking file to be deleted")
	assert.NoError(t, err, "did not expect error if tracking file upgrade failed due to missing file")

	// Legacy tracking file exists, and staging path exists, but stagedDeviceInfo not valid JSON!
	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), trackingInfoFile, "volume tracking info").
		SetArg(1, trackInfoAndPath).
		Return(nil)
	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), stagedDeviceInfo, "publish info").
		Return(errors.InvalidJSONError("foo"))
	needsDelete, err = v.UpgradeVolumeTrackingFile(context.Background(), volName, pubPaths, nil)
	assert.True(t, needsDelete, "failure to upgrade should cause the tracking file to be deleted")
	assert.NoError(t, err, "did not expect error if tracking file upgrade failed to find json in file")

	// Legacy tracking file exists, and staging path exists, but tracking file is not valid JSON!
	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), trackingInfoFile, "volume tracking info").
		Return(errors.InvalidJSONError("foo"))
	needsDelete, err = v.UpgradeVolumeTrackingFile(context.Background(), volName, pubPaths, nil)
	assert.True(t, needsDelete, "failure to upgrade should cause the tracking file to be deleted")
	assert.NoError(t, err, "did not expect error if tracking file upgrade failed to find json in file")
}

func TestUpgradeVolumeTrackingFile_MissingDevicePathBeforeUpgrade(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	jsonReaderWriter := mock_filesystem.NewMockJSONReaderWriter(mockCtrl)
	defer func(previousJsonRW filesystem.JSONReaderWriter) {
		jsonRW = previousJsonRW
	}(jsonRW)
	jsonRW = jsonReaderWriter
	osFs := afero.NewMemMapFs()

	stagePath := "/foo"
	trackPath := "/bar"

	v := NewVolumePublishManagerDetailed(trackPath, filesystem.New(nil), osFs)

	basePubInfo := models.VolumePublishInfo{}
	basePubInfo.NfsServerIP = "1.1.1.1"
	basePubInfo.FilesystemType = "somefs"

	trackInfoAndPath := models.VolumeTrackingInfo{}
	trackInfoAndPath.StagingTargetPath = stagePath

	volName := "pvc-123"
	fName := volName + ".json"
	pubPaths := map[string]struct{}{}
	trackingInfoFile := path.Join(trackPath, fName)
	stagedDeviceInfo := path.Join(stagePath, volumePublishInfoFilename)
	tmpTrackingInfoFile := path.Join(trackPath, "tmp-"+fName)

	// Happy path where rawDevicePath exist but not devicePath
	copyPublishInfo := basePubInfo
	copyPublishInfo.IscsiTargetPortal = "someportal"
	copyPublishInfo.DevicePath = ""
	copyPublishInfo.RawDevicePath = "/dev/dm-0"

	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), trackingInfoFile, "volume tracking info").
		SetArg(1, trackInfoAndPath).
		Return(nil)
	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), stagedDeviceInfo, "publish info").
		SetArg(1, copyPublishInfo).
		Return(nil)
	jsonReaderWriter.EXPECT().WriteJSONFile(gomock.Any(), gomock.Any(), tmpTrackingInfoFile, "volume tracking info").
		Return(nil)
	_, err := osFs.Create(tmpTrackingInfoFile)
	assert.NoError(t, err, "expected a test tmp file to be created without error")

	needsDelete, err := v.UpgradeVolumeTrackingFile(context.Background(), volName, pubPaths, nil)
	assert.False(t, needsDelete, "expected to be told to keep tracking file")
	assert.NoError(t, err, "expected tracking file upgrade to work")
	_, err = afero.ReadFile(osFs, trackingInfoFile)
	assert.NoError(t, err, "expected a tracking file to be present")

	// Missing devicePath, rawDevicePath
	copyPublishInfo = basePubInfo
	copyPublishInfo.IscsiTargetPortal = "someportal"
	copyPublishInfo.DevicePath = ""
	copyPublishInfo.RawDevicePath = ""

	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), trackingInfoFile, "volume tracking info").
		SetArg(1, trackInfoAndPath).
		Return(nil)
	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), stagedDeviceInfo, "publish info").
		SetArg(1, copyPublishInfo).
		Return(nil)
	jsonReaderWriter.EXPECT().WriteJSONFile(gomock.Any(), gomock.Any(), tmpTrackingInfoFile, "volume tracking info").
		Return(nil)
	_, err = osFs.Create(tmpTrackingInfoFile)

	needsDelete, err = v.UpgradeVolumeTrackingFile(context.Background(), volName, pubPaths, nil)
	assert.False(t, needsDelete, "expected to be told to keep tracking file")
	assert.NoError(t, err, "expected tracking file upgrade to work")

	// Present both devicePath and rawDevicePath
	copyPublishInfo = basePubInfo
	copyPublishInfo.IscsiTargetPortal = "someportal"
	copyPublishInfo.DevicePath = "/dev/dm-0"
	copyPublishInfo.RawDevicePath = "/dev/dm-1"

	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), trackingInfoFile, "volume tracking info").
		SetArg(1, trackInfoAndPath).
		Return(nil)
	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), stagedDeviceInfo, "publish info").
		SetArg(1, copyPublishInfo).
		Return(nil)
	jsonReaderWriter.EXPECT().WriteJSONFile(gomock.Any(), gomock.Any(), tmpTrackingInfoFile, "volume tracking info").
		Return(nil)
	_, err = osFs.Create(tmpTrackingInfoFile)

	needsDelete, err = v.UpgradeVolumeTrackingFile(context.Background(), volName, pubPaths, nil)
	assert.False(t, needsDelete, "expected to be told to keep tracking file")
	assert.NoError(t, err, "expected tracking file upgrade to work")
}

func TestUpgradeVolumeTrackingFile_MissingDevicePathAfterUpgrade(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	jsonReaderWriter := mock_filesystem.NewMockJSONReaderWriter(mockCtrl)
	defer func(previousJsonRW filesystem.JSONReaderWriter) {
		jsonRW = previousJsonRW
	}(jsonRW)
	jsonRW = jsonReaderWriter
	osFs := afero.NewMemMapFs()

	stagePath := "/foo"
	trackPath := "/bar"

	basePubInfo := models.VolumePublishInfo{}
	basePubInfo.NfsServerIP = "1.1.1.1"
	basePubInfo.FilesystemType = "somefs"

	trackInfoAndPath := models.VolumeTrackingInfo{}
	trackInfoAndPath.StagingTargetPath = stagePath
	trackInfoAndPath.PublishedPaths = map[string]struct{}{
		"path1": {},
		"path2": {},
		"path3": {},
	}

	pvToDeviceMappings := map[string]string{
		"path1": "/dev/dm-0",
		"path4": "/dev/dm-1",
	}

	volName := "pvc-123"
	fName := volName + ".json"
	pubPaths := map[string]struct{}{}
	v := NewVolumePublishManagerDetailed(trackPath, filesystem.New(nil), osFs)
	trackingInfoFile := path.Join(trackPath, fName)
	tmpTrackingInfoFile := path.Join(trackPath, "tmp-"+fName)

	// Happy path where rawDevicePath exist but not devicePath
	copyPublishInfo := basePubInfo
	copyPublishInfo.IscsiTargetPortal = "someportal"
	copyPublishInfo.DevicePath = ""
	copyPublishInfo.RawDevicePath = "/dev/dm-0"

	trackInfoAndPath.VolumePublishInfo = copyPublishInfo

	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), trackingInfoFile, "volume tracking info").
		SetArg(1, trackInfoAndPath).
		Return(nil)
	jsonReaderWriter.EXPECT().WriteJSONFile(gomock.Any(), gomock.Any(), tmpTrackingInfoFile, "volume tracking info").
		Return(nil)
	_, err := osFs.Create(tmpTrackingInfoFile)
	assert.NoError(t, err, "expected a test tmp file to be created without error")

	needsDelete, err := v.UpgradeVolumeTrackingFile(context.Background(), volName, pubPaths, nil)
	assert.False(t, needsDelete, "expected to be told to keep tracking file")
	assert.NoError(t, err, "expected tracking file upgrade to work")
	_, err = afero.ReadFile(osFs, trackingInfoFile)
	assert.NoError(t, err, "expected a tracking file to be present")

	// Happy path where rawDevicePath and devicePath do not exist, but published paths do
	copyPublishInfo = basePubInfo
	copyPublishInfo.IscsiTargetPortal = "someportal"
	copyPublishInfo.DevicePath = ""
	copyPublishInfo.RawDevicePath = ""

	trackInfoAndPath.VolumePublishInfo = copyPublishInfo

	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), trackingInfoFile, "volume tracking info").
		SetArg(1, trackInfoAndPath).
		Return(nil)
	jsonReaderWriter.EXPECT().WriteJSONFile(gomock.Any(), gomock.Any(), tmpTrackingInfoFile, "volume tracking info").
		Return(nil)
	_, err = osFs.Create(tmpTrackingInfoFile)
	assert.NoError(t, err, "expected a test tmp file to be created without error")

	needsDelete, err = v.UpgradeVolumeTrackingFile(context.Background(), volName, pubPaths, pvToDeviceMappings)
	assert.False(t, needsDelete, "expected to be told to keep tracking file")
	assert.NoError(t, err, "expected tracking file upgrade to work")
	_, err = afero.ReadFile(osFs, trackingInfoFile)
	assert.NoError(t, err, "expected a tracking file to be present")

	// Happy path not iSCSI
	copyPublishInfo = basePubInfo
	copyPublishInfo.IscsiTargetPortal = ""

	trackInfoAndPath.VolumePublishInfo = copyPublishInfo

	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), trackingInfoFile, "volume tracking info").
		SetArg(1, trackInfoAndPath).
		Return(nil)

	needsDelete, err = v.UpgradeVolumeTrackingFile(context.Background(), volName, pubPaths, nil)
	assert.False(t, needsDelete, "expected to be told to keep tracking file")
	assert.NoError(t, err, "expected tracking file upgrade to work")

	// Fail to write JSON file error
	copyPublishInfo = basePubInfo
	copyPublishInfo.IscsiTargetPortal = "someportal"
	copyPublishInfo.DevicePath = ""
	copyPublishInfo.RawDevicePath = ""

	trackInfoAndPath.VolumePublishInfo = copyPublishInfo

	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), trackingInfoFile, "volume tracking info").
		SetArg(1, trackInfoAndPath).
		Return(nil)
	jsonReaderWriter.EXPECT().WriteJSONFile(gomock.Any(), gomock.Any(), tmpTrackingInfoFile, "volume tracking info").
		Return(fmt.Errorf("some error"))

	needsDelete, err = v.UpgradeVolumeTrackingFile(context.Background(), volName, pubPaths, pvToDeviceMappings)
	assert.False(t, needsDelete, "expected to be told to keep tracking file")
	assert.NoError(t, err, "expected tracking file upgrade to work")

	// Missing devicePath, rawDevicePath and pvToDeviceMappings
	copyPublishInfo = basePubInfo
	copyPublishInfo.IscsiTargetPortal = "someportal"
	copyPublishInfo.DevicePath = ""
	copyPublishInfo.RawDevicePath = ""

	copyTrackInfoAndPath := trackInfoAndPath
	copyTrackInfoAndPath.PublishedPaths = map[string]struct{}{}

	copyTrackInfoAndPath.VolumePublishInfo = copyPublishInfo

	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), trackingInfoFile, "volume tracking info").
		SetArg(1, copyTrackInfoAndPath).
		Return(nil)

	needsDelete, err = v.UpgradeVolumeTrackingFile(context.Background(), volName, pubPaths, pvToDeviceMappings)
	assert.False(t, needsDelete, "expected to be told to keep tracking file")
	assert.NoError(t, err, "expected tracking file upgrade to work")

	// Present both devicePath and rawDevicePath
	copyPublishInfo = basePubInfo
	copyPublishInfo.IscsiTargetPortal = "someportal"
	copyPublishInfo.DevicePath = "/dev/dm-0"
	copyPublishInfo.RawDevicePath = "/dev/dm-1"

	trackInfoAndPath.VolumePublishInfo = copyPublishInfo

	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), gomock.Any(), trackingInfoFile, "volume tracking info").
		SetArg(1, trackInfoAndPath).
		Return(nil)

	needsDelete, err = v.UpgradeVolumeTrackingFile(context.Background(), volName, pubPaths, pvToDeviceMappings)
	assert.False(t, needsDelete, "expected to be told to keep tracking file")
	assert.NoError(t, err, "expected tracking file upgrade to work")
}

func TestValidateTrackingFile(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockiSCSIUtils := mock_iscsi.NewMockIscsiReconcileUtils(mockCtrl)
	jsonReaderWriter := mock_filesystem.NewMockJSONReaderWriter(mockCtrl)

	osFs := afero.NewMemMapFs()
	defer func(previousJsonRW filesystem.JSONReaderWriter) {
		jsonRW = previousJsonRW
	}(jsonRW)
	jsonRW = jsonReaderWriter

	trackPath := "."
	v := NewVolumePublishManagerDetailed(trackPath, filesystem.New(nil), osFs)

	oldiSCSIUtils := iscsiUtils
	defer func() { iscsiUtils = oldiSCSIUtils }()
	iscsiUtils = mockiSCSIUtils

	stagePath := "/foo"
	volName := "pvc-123"
	fName := volName + ".json"
	fsType := "ext4"
	trackInfo := &models.VolumeTrackingInfo{}
	trackInfo.StagingTargetPath = stagePath
	trackInfo.FilesystemType = fsType
	trackInfo.IscsiTargetIQN = "iqn"
	trackInfo.SANType = sa.ISCSI

	emptyTrackInfo := &models.VolumeTrackingInfo{}

	// If staging path doesn't exist, check protocol specific reconciliation steps.
	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), emptyTrackInfo, fName, gomock.Any()).
		SetArg(1, *trackInfo).Return(nil)
	mockiSCSIUtils.EXPECT().ReconcileISCSIVolumeInfo(gomock.Any(), trackInfo).Return(true, nil)
	needsDelete, err := v.ValidateTrackingFile(context.Background(), volName)
	assert.False(t, needsDelete, "when protocol specific reconciliation returns true, we expect false")
	assert.NoError(t, err, "expected no error during validation")

	// If staging path doesn't exist, check protocol specific reconciliation steps (part 2).
	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), emptyTrackInfo, fName, gomock.Any()).
		SetArg(1, *trackInfo).Return(nil)
	mockiSCSIUtils.EXPECT().ReconcileISCSIVolumeInfo(gomock.Any(), trackInfo).Return(false, nil)
	needsDelete, err = v.ValidateTrackingFile(context.Background(), volName)
	assert.True(t, needsDelete, "when protocol specific reconciliation returns false, we expect true")
	assert.NoError(t, err, "expected no error during validation")

	// If staging path doesn't exist, check protocol specific reconciliation steps (part 3).
	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), emptyTrackInfo, fName, gomock.Any()).
		SetArg(1, *trackInfo).Return(nil)
	mockiSCSIUtils.EXPECT().ReconcileISCSIVolumeInfo(gomock.Any(), trackInfo).Return(false, errors.New("foo"))
	needsDelete, err = v.ValidateTrackingFile(context.Background(), volName)
	assert.False(t, needsDelete, "when protocol specific reconciliation returns an error, we expect false")
	assert.Error(t, err, "we expect a protocol specific reconciliation error to cause an error")

	// If staging path doesn't exist, and the file isn't valid JSON.
	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), emptyTrackInfo, fName, gomock.Any()).
		Return(errors.InvalidJSONError("foo"))
	needsDelete, err = v.ValidateTrackingFile(context.Background(), volName)
	assert.True(t, needsDelete, "if the file is not JSON, we expect to be told to delete it")
	assert.NoError(t, err, "we expect no error when the file is not JSON")

	// If staging path doesn't exist, and all the values we check to determine the protocol of the volume from the
	// publish info are empty strings, then return true, because this is an exceptional case.

	// Make the protocol indeterminate.
	trackInfo.NfsServerIP = ""
	trackInfo.IscsiTargetIQN = ""
	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), emptyTrackInfo, fName, gomock.Any()).
		SetArg(1, *trackInfo).Return(nil)
	needsDelete, err = v.ValidateTrackingFile(context.Background(), volName)
	assert.True(t, needsDelete, "expected to get true if we can't determine the protocol of the volume")
	assert.NoError(t, err, "expected no error if we can't determine the protocol of the volume")

	// If staging path does exist, and the file is JSON, return false, because we know something related to the volume
	// still exists.

	// Make protocol reconcile choose iSCSI.
	trackInfo.IscsiTargetIQN = "iqn"
	err = osFs.Mkdir(stagePath, 0o777)
	assert.NoError(t, err, "expected the test directory to be created without error")

	jsonReaderWriter.EXPECT().ReadJSONFile(gomock.Any(), emptyTrackInfo, fName, gomock.Any()).
		SetArg(1, *trackInfo).Return(nil)
	needsDelete, err = v.ValidateTrackingFile(context.Background(), volName)
	assert.False(t, needsDelete, "expected to be told to keep the volume tracking file")
	assert.NoError(t, err, "expected no error if the staging path exists and the tracking file is JSON")
}

func TestDeleteFailedUpgradeTrackingFile(t *testing.T) {
	filename := "/var/lib/trident/tracking/tmp-foo"
	osFs := afero.NewMemMapFs()

	v := NewVolumePublishManagerDetailed(config.VolumeTrackingInfoPath, filesystem.NewDetailed(exec.NewCommand(), osFs, nil),
		osFs)

	file, _ := osFs.Create(filename)
	fileinfo, _ := file.Stat()
	v.DeleteFailedUpgradeTrackingFile(context.Background(), fileinfo)
	_, err := osFs.Stat(filename)

	assert.True(t, errors.Is(err, fs.ErrNotExist), "expected an fs.ErrNotExist from deleting a non-existent file.")
}

func TestClearStagedDeviceInfo(t *testing.T) {
	filename := volumePublishInfoFilename
	osFs := afero.NewMemMapFs()
	v := NewVolumePublishManagerDetailed(config.VolumeTrackingInfoPath, filesystem.New(nil), osFs)

	defer func() { osFs = afero.NewOsFs() }()

	// happy path
	_, err := osFs.Create(filename)
	_, statErr := osFs.Stat(filename)
	assert.NoError(t, err, "expected no error creating a test file")
	assert.NoError(t, statErr, "expected to be able to stat just-created test file")
	err = v.clearStagedDeviceInfo(context.Background(), ".", "pvc-123")

	_, statErr = osFs.Stat(filename)
	assert.NoError(t, err, "expected test file to exist before deletion")
	assert.Error(t, statErr, "expected staged device info file to be deleted!")

	// does not exist case
	_, statErr = osFs.Stat(filename)
	assert.Error(t, statErr, "file should not exist")
	err = v.clearStagedDeviceInfo(context.Background(), ".", "pvc-123")
	assert.NoError(t, err, "clear staged tracking file should not fail if the file doesn't exist")
}
