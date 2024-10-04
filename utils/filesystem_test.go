package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/utils/errors"
	execCmd "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/models"
)

func TestReadJSONFile_Succeeds(t *testing.T) {
	defer func() { osFs = afero.NewOsFs() }()
	osFs = afero.NewMemMapFs()
	file, err := osFs.Create("foo.json")
	assert.NoError(t, err)

	pubInfo := &models.VolumePublishInfo{}
	pubInfo.NfsServerIP = "1.1.1.1"
	pInfo, err := json.Marshal(pubInfo)
	_, err = file.Write(pInfo)
	assert.NoError(t, err)

	returnedPubInfo := &models.VolumePublishInfo{}
	err = JsonReaderWriter.ReadJSONFile(context.Background(), returnedPubInfo, "foo.json", "")
	assert.NoError(t, err, "did not expect error reading valid JSON")
	assert.Equal(t, pubInfo.NfsServerIP, returnedPubInfo.NfsServerIP, "expected read to unmarshal correctly with the"+
		" expected data")
}

func TestReadJSONFile_FailsWithIncompleteJSON(t *testing.T) {
	defer func() { osFs = afero.NewOsFs() }()
	osFs = afero.NewMemMapFs()
	file, err := osFs.Create("foo.json")
	assert.NoError(t, err)

	// Will cause an unexpected EOF because the start of the file is the beginning token for valid JSON, but the ending
	// token is never found.
	_, err = file.Write([]byte("{"))
	assert.NoError(t, err)

	returnedPubInfo := &models.VolumePublishInfo{}
	err = JsonReaderWriter.ReadJSONFile(context.Background(), returnedPubInfo, "foo.json", "")
	assert.True(t, errors.IsInvalidJSONError(err), "expected invalidJSONError due to file containing incomplete JSON")
}

func TestReadJSONFile_FailsBecauseUnmarshalTypeError(t *testing.T) {
	defer func() { osFs = afero.NewOsFs() }()
	osFs = afero.NewMemMapFs()
	file, err := osFs.Create("foo.json")
	assert.NoError(t, err)

	type BadPublishInfo struct {
		HostIQN bool `json:"hostIQN,omitempty"`
	}

	pubInfo := &BadPublishInfo{}
	pubInfo.HostIQN = true
	pInfo, err := json.Marshal(pubInfo)
	_, err = file.Write(pInfo)
	assert.NoError(t, err)

	returnedPubInfo := &models.VolumePublishInfo{}
	err = JsonReaderWriter.ReadJSONFile(context.Background(), returnedPubInfo, "foo.json", "")
	assert.True(t, errors.IsInvalidJSONError(err), "expected invalidJSONError due to unmarshallable type in valid JSON")
}

func TestReadJSONFile_FailsBecauseNoFile(t *testing.T) {
	returnedPubInfo := &models.VolumePublishInfo{}
	err := JsonReaderWriter.ReadJSONFile(context.Background(), returnedPubInfo, "foo.json", "")
	assert.True(t, errors.IsNotFoundError(err), "expected NotFoundError when file doesn't exist")
}

func TestReadJSONFile_FailsWithPermissionsError(t *testing.T) {
	defer func() { osFs = afero.NewOsFs() }()
	osFs = &MockFs{}
	_, err := osFs.Create("foo.json")
	assert.NoError(t, err)

	returnedPubInfo := &models.VolumePublishInfo{}
	err = JsonReaderWriter.ReadJSONFile(context.Background(), returnedPubInfo, "foo.json", "")
	assert.True(t, !errors.IsInvalidJSONError(err) && !errors.IsNotFoundError(err), "expected unwrapped error")
}

func TestReadJSONFile_FailsWithSyntaxError(t *testing.T) {
	defer func() { osFs = afero.NewOsFs() }()
	osFs = afero.NewMemMapFs()
	file, err := osFs.Create("foo.json")
	assert.NoError(t, err)
	// does not contain the beginning curly brace, so won't cause unexpected EOF
	_, err = file.Write([]byte("garbage"))
	assert.NoError(t, err)

	returnedPubInfo := &models.VolumePublishInfo{}
	err = JsonReaderWriter.ReadJSONFile(context.Background(), returnedPubInfo, "foo.json", "")
	assert.True(t, errors.IsInvalidJSONError(err), "expected invalid JSON if syntax error")
}

func TestReadJSONFile_FailsBecauseEmptyFile(t *testing.T) {
	defer func() { osFs = afero.NewOsFs() }()
	osFs = afero.NewMemMapFs()
	_, err := osFs.Create("foo.json")
	assert.NoError(t, err)

	returnedPubInfo := &models.VolumePublishInfo{}
	err = JsonReaderWriter.ReadJSONFile(context.Background(), returnedPubInfo, "foo.json", "")
	assert.Error(t, err, "expected an error from an empty file")
	assert.True(t, errors.IsInvalidJSONError(err), "expected InvalidJSONError due to empty file")
}

func TestWriteJSONFile_Succeeds(t *testing.T) {
	defer func() { osFs = afero.NewOsFs() }()
	osFs = afero.NewMemMapFs()

	pubInfo := &models.VolumePublishInfo{}
	pubInfo.NfsServerIP = "1.1.1.1"

	err := JsonReaderWriter.WriteJSONFile(context.Background(), pubInfo, "foo.json", "")
	assert.NoError(t, err, "did not expect error writing JSON file")
	_, err = osFs.Stat("foo.json")
	assert.NoError(t, err, "expected file to exist after writing it")

	contents, err := afero.ReadFile(osFs, "foo.json")
	writtenPubInfo := &models.VolumePublishInfo{}
	err = json.Unmarshal(contents, writtenPubInfo)
	assert.NoError(t, err, "expected written file's contents to be JSON and unmarshallable")
	assert.Equal(t, pubInfo.NfsServerIP, writtenPubInfo.NfsServerIP, "expected written field value to be present")
}

func TestWriteJSONFile_FailsOnReadOnlyFs(t *testing.T) {
	defer func() { osFs = afero.NewOsFs() }()
	osFs = afero.NewReadOnlyFs(afero.NewMemMapFs())

	pubInfo := &models.VolumePublishInfo{}
	pubInfo.NfsServerIP = "1.1.1.1"

	err := JsonReaderWriter.WriteJSONFile(context.Background(), pubInfo, "foo.json", "")
	assert.Error(t, err, "expected error writing to read-only filesystem")
}

func TestWriteJSONFile_FailsWritingNotMarshallableData(t *testing.T) {
	defer func() { osFs = afero.NewOsFs() }()
	osFs = afero.NewMemMapFs()

	pubInfo := make(chan int)

	err := JsonReaderWriter.WriteJSONFile(context.Background(), &pubInfo, "foo.json", "")
	assert.Error(t, err, "expected error trying to write something that can't be marshalled to JSON")
}

func TestDeleteFile_Succeeds(t *testing.T) {
	defer func() { osFs = afero.NewOsFs() }()
	osFs = afero.NewMemMapFs()
	_, err := osFs.Create("foo.json")
	assert.NoError(t, err)

	_, err = DeleteFile(context.Background(), "foo.json", "")
	assert.NoError(t, err, "did not expect error deleting file")
}

func TestDeleteFile_SucceedsWhenFileDoesntExist(t *testing.T) {
	defer func() { osFs = afero.NewOsFs() }()
	osFs = afero.NewMemMapFs()

	_, err := DeleteFile(context.Background(), "foo.json", "")
	assert.NoError(t, err, "expected no error deleting a file when the file doesn't exist")
}

func TestDeleteFile_FailsOnReadOnlyFilesystem(t *testing.T) {
	defer func() { osFs = afero.NewOsFs() }()
	osFs = afero.NewMemMapFs()
	_, err := osFs.Create("foo.json")
	assert.NoError(t, err)

	osFs = afero.NewReadOnlyFs(osFs)

	_, err = DeleteFile(context.Background(), "foo.json", "")
	assert.Error(t, err, "expected an error deleting a file on a read-only filesystem")
}

func TestFormatVolume(t *testing.T) {
	ctx := ctx()
	mockCtrl := gomock.NewController(t)
	mockexec := mock_exec.NewMockCommand(mockCtrl)

	tempDevicePath := "/dev/dm-7"
	tempFormatOptions := fmt.Sprintf("-F%s-b 4096%s-T stride=16", FormatOptionsSeparator, FormatOptionsSeparator)
	tempFailureError := "failed"

	ext4Cmd := "mkfs.ext4"
	ext3Cmd := "mkfs.ext3"
	xfsCmd := "mkfs.xfs"

	defer func(previousCommand execCmd.Command) {
		command = previousCommand
	}(command)

	command = mockexec

	tests := []struct {
		message       string
		device        string
		fstype        string
		formatOptions string
		mockSetup     func()
		isError       bool
		stdErrOutput  []byte
	}{
		{
			"ext4 / no error / no format options",
			tempDevicePath,
			fsExt4,
			"",
			func() {
				mockexec.EXPECT().Execute(gomock.Any(), ext4Cmd, gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
			},
			false,
			[]byte(nil),
		},
		{
			"ext4 / no error / format options",
			tempDevicePath,
			fsExt4,
			tempFormatOptions,
			func() {
				mockexec.EXPECT().Execute(gomock.Any(), ext4Cmd, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
			},
			false,
			[]byte(nil),
		},
		{
			"ext4 / error",
			tempDevicePath,
			fsExt4,
			"",
			func() {
				mockexec.EXPECT().Execute(gomock.Any(), ext4Cmd, gomock.Any(), gomock.Any()).Return([]byte(tempFailureError), fmt.Errorf("exit status: 1")).Times(1)
			},
			true,
			[]byte(fmt.Sprintf("error formatting device %v: %v", tempDevicePath, tempFailureError)),
		},
		{
			"ext3 / no error / no format options",
			tempDevicePath,
			fsExt3,
			"",
			func() {
				mockexec.EXPECT().Execute(gomock.Any(), ext3Cmd, gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
			},
			false,
			[]byte(nil),
		},
		{
			"ext3 / no error / format options",
			tempDevicePath,
			fsExt3,
			tempFormatOptions,
			func() {
				mockexec.EXPECT().Execute(gomock.Any(), ext3Cmd, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
			},
			false,
			[]byte(nil),
		},
		{
			"ext3 / error",
			tempDevicePath,
			fsExt3,
			"",
			func() {
				mockexec.EXPECT().Execute(gomock.Any(), ext3Cmd, gomock.Any(), gomock.Any()).Return([]byte(tempFailureError), fmt.Errorf("exit status: 1")).Times(1)
			},
			true,
			[]byte(fmt.Sprintf("error formatting device %v: %v", tempDevicePath, tempFailureError)),
		},
		{
			"xfs / no error / no format options",
			tempDevicePath,
			fsXfs,
			"",
			func() {
				mockexec.EXPECT().Execute(gomock.Any(), xfsCmd, gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
			},
			false,
			[]byte(nil),
		},
		{
			"xfs / no error / format options",
			tempDevicePath,
			fsXfs,
			tempFormatOptions,
			func() {
				mockexec.EXPECT().Execute(gomock.Any(), xfsCmd, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
			},
			false,
			[]byte(nil),
		},
		{
			"xfs / error",
			tempDevicePath,
			fsXfs,
			"",
			func() {
				mockexec.EXPECT().Execute(gomock.Any(), xfsCmd, gomock.Any(), gomock.Any()).Return([]byte(tempFailureError), fmt.Errorf("exit status: 1")).Times(1)
			},
			true,
			[]byte(fmt.Sprintf("error formatting device %v: %v", tempDevicePath, tempFailureError)),
		},
		{
			"unsupported fs / error",
			tempDevicePath,
			"unsupported",
			"",
			func() {},
			true,
			[]byte(fmt.Sprintf("unsupported file system type: %s", "unsupported")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			tt.mockSetup()
			err := formatVolume(ctx, tt.device, tt.fstype, tt.formatOptions)
			if !tt.isError {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
