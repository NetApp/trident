// Copyright 2025 NetApp, Inc. All Rights Reserved.

package filesystem

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/logging"
	"github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/utils/errors"
	execCmd "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/models"
)

// MockFs is a struct that embeds the afero in-memory filesystem, so that we can
// implement a version of Open that can return a permissions error.
type MockFs struct {
	afero.MemMapFs
}

var globalLogMutationMutex sync.Mutex

func (m *MockFs) Open(name string) (afero.File, error) {
	return nil, fs.ErrPermission
}

func TestReadJSONFile_Succeeds(t *testing.T) {
	osFs := afero.NewMemMapFs()
	file, err := osFs.Create("foo.json")
	assert.NoError(t, err)

	pubInfo := &models.VolumePublishInfo{}
	pubInfo.NfsServerIP = "1.1.1.1"
	pInfo, err := json.Marshal(pubInfo)
	_, err = file.Write(pInfo)
	assert.NoError(t, err)

	returnedPubInfo := &models.VolumePublishInfo{}
	jsonRW := NewJSONReaderWriter(osFs)
	err = jsonRW.ReadJSONFile(context.Background(), returnedPubInfo, "foo.json", "")
	assert.NoError(t, err, "did not expect error reading valid JSON")
	assert.Equal(t, pubInfo.NfsServerIP, returnedPubInfo.NfsServerIP, "expected read to unmarshal correctly with the"+
		" expected data")
}

func TestReadJSONFile_FailsWithIncompleteJSON(t *testing.T) {
	osFs := afero.NewMemMapFs()
	file, err := osFs.Create("foo.json")
	assert.NoError(t, err)

	// Will cause an unexpected EOF because the start of the file is the beginning token for valid JSON, but the ending
	// token is never found.
	_, err = file.Write([]byte("{"))
	assert.NoError(t, err)

	returnedPubInfo := &models.VolumePublishInfo{}
	jsonReaderWriter := NewJSONReaderWriter(osFs)
	err = jsonReaderWriter.ReadJSONFile(context.Background(), returnedPubInfo, "foo.json", "")
	assert.True(t, errors.IsInvalidJSONError(err), "expected invalidJSONError due to file containing incomplete JSON")
}

func TestReadJSONFile_FailsBecauseUnmarshalTypeError(t *testing.T) {
	osFs := afero.NewMemMapFs()
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
	jsonReaderWriter := NewJSONReaderWriter(osFs)
	err = jsonReaderWriter.ReadJSONFile(context.Background(), returnedPubInfo, "foo.json", "")
	assert.True(t, errors.IsInvalidJSONError(err), "expected invalidJSONError due to unmarshallable type in valid JSON")
}

func TestReadJSONFile_FailsBecauseNoFile(t *testing.T) {
	returnedPubInfo := &models.VolumePublishInfo{}
	jsonReaderWriter := NewJSONReaderWriter(afero.NewMemMapFs())
	err := jsonReaderWriter.ReadJSONFile(context.Background(), returnedPubInfo, "foo.json", "")
	assert.True(t, errors.IsNotFoundError(err), "expected NotFoundError when file doesn't exist")
}

func TestReadJSONFile_FailsWithPermissionsError(t *testing.T) {
	osFs := afero.NewMemMapFs()
	osFs = &MockFs{}
	_, err := osFs.Create("foo.json")
	assert.NoError(t, err)

	returnedPubInfo := &models.VolumePublishInfo{}
	jsonReaderWriter := NewJSONReaderWriter(osFs)
	err = jsonReaderWriter.ReadJSONFile(context.Background(), returnedPubInfo, "foo.json", "")
	assert.True(t, !errors.IsInvalidJSONError(err) && !errors.IsNotFoundError(err), "expected unwrapped error")
}

func TestReadJSONFile_FailsWithSyntaxError(t *testing.T) {
	osFs := afero.NewMemMapFs()
	file, err := osFs.Create("foo.json")
	assert.NoError(t, err)
	// does not contain the beginning curly brace, so won't cause unexpected EOF
	_, err = file.Write([]byte("garbage"))
	assert.NoError(t, err)

	returnedPubInfo := &models.VolumePublishInfo{}
	jsonReaderWriter := NewJSONReaderWriter(osFs)
	err = jsonReaderWriter.ReadJSONFile(context.Background(), returnedPubInfo, "foo.json", "")
	assert.True(t, errors.IsInvalidJSONError(err), "expected invalid JSON if syntax error")
}

func TestReadJSONFile_FailsBecauseEmptyFile(t *testing.T) {
	osFs := afero.NewMemMapFs()
	_, err := osFs.Create("foo.json")
	assert.NoError(t, err)

	returnedPubInfo := &models.VolumePublishInfo{}
	jsonReaderWriter := NewJSONReaderWriter(osFs)
	err = jsonReaderWriter.ReadJSONFile(context.Background(), returnedPubInfo, "foo.json", "")
	assert.Error(t, err, "expected an error from an empty file")
	assert.True(t, errors.IsInvalidJSONError(err), "expected InvalidJSONError due to empty file")
}

func TestWriteJSONFile_Succeeds(t *testing.T) {
	osFs := afero.NewMemMapFs()

	pubInfo := &models.VolumePublishInfo{}
	pubInfo.NfsServerIP = "1.1.1.1"

	jsonReaderWriter := NewJSONReaderWriter(osFs)
	err := jsonReaderWriter.WriteJSONFile(context.Background(), pubInfo, "foo.json", "")
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
	osFs := afero.NewReadOnlyFs(afero.NewMemMapFs())

	pubInfo := &models.VolumePublishInfo{}
	pubInfo.NfsServerIP = "1.1.1.1"

	jsonReaderWriter := NewJSONReaderWriter(osFs)
	err := jsonReaderWriter.WriteJSONFile(context.Background(), pubInfo, "foo.json", "")
	assert.Error(t, err, "expected error writing to read-only filesystem")
}

func TestWriteJSONFile_FailsWritingNotMarshallableData(t *testing.T) {
	osFs := afero.NewMemMapFs()

	pubInfo := make(chan int)

	jsonReaderWriter := NewJSONReaderWriter(osFs)
	err := jsonReaderWriter.WriteJSONFile(context.Background(), &pubInfo, "foo.json", "")
	assert.Error(t, err, "expected error trying to write something that can't be marshalled to JSON")
}

func TestDeleteFile_Succeeds(t *testing.T) {
	osFs := afero.NewMemMapFs()
	fsClient := NewDetailed(execCmd.NewCommand(), osFs, nil)
	_, err := osFs.Create("foo.json")
	assert.NoError(t, err)

	_, err = fsClient.DeleteFile(context.Background(), "foo.json", "")
	assert.NoError(t, err, "did not expect error deleting file")
}

func TestDeleteFile_SucceedsWhenFileDoesntExist(t *testing.T) {
	osFs := afero.NewMemMapFs()
	fsClient := NewDetailed(execCmd.NewCommand(), osFs, nil)

	_, err := fsClient.DeleteFile(context.Background(), "foo.json", "")
	assert.NoError(t, err, "expected no error deleting a file when the file doesn't exist")
}

func TestDeleteFile_FailsOnReadOnlyFilesystem(t *testing.T) {
	osFs := afero.NewMemMapFs()

	_, err := osFs.Create("foo.json")
	assert.NoError(t, err)

	osFs = afero.NewReadOnlyFs(osFs)

	fsClient := NewDetailed(execCmd.NewCommand(), osFs, nil)
	_, err = fsClient.DeleteFile(context.Background(), "foo.json", "")
	assert.Error(t, err, "expected an error deleting a file on a read-only filesystem")
}

func TestGetDFOutput(t *testing.T) {
	mockDFOutput := []byte(
		"Mounted on                    Filesystem\n" +
			"/run                          tmpfs\n" +
			"/                             /dev/sda2\n",
	)

	type parameters struct {
		name           string
		mockOutput     []byte
		mockError      error
		expectedResult []models.DFInfo
		expectError    bool
	}

	tests := map[string]parameters{
		"Success": {
			mockOutput: mockDFOutput,
			mockError:  nil,
			expectedResult: []models.DFInfo{
				{Target: "/run", Source: "tmpfs"},
				{Target: "/", Source: "/dev/sda2"},
			},
			expectError: false,
		},
		"EmptyOutput": {
			mockOutput:     []byte(""),
			mockError:      nil,
			expectedResult: nil,
			expectError:    false,
		},
		"Error": {
			mockOutput:     nil,
			mockError:      errors.New("error"),
			expectedResult: nil,
			expectError:    true,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockExec := mock_exec.NewMockCommand(mockCtrl)
			fsClient := NewDetailed(mockExec, nil, nil)

			mockExec.EXPECT().Execute(gomock.Any(), "df", "--output=target,source").Return(params.mockOutput, params.mockError)

			result, err := fsClient.GetDFOutput(context.Background())
			if params.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, params.expectedResult, result)
		})
	}
}

func TestFormatVolumeRetry(t *testing.T) {
	originalMaxDuration := formatVolumeMaxRetryDuration
	defer func() { formatVolumeMaxRetryDuration = originalMaxDuration }()
	formatVolumeMaxRetryDuration = 3 * time.Second

	type parameters struct {
		device      string
		fstype      string
		options     string
		getMockCmd  func() *mock_exec.MockCommand
		expectError bool
	}
	tests := map[string]parameters{
		"Success with xfs": {
			device:  "/dev/sda1",
			fstype:  Xfs,
			options: "",
			getMockCmd: func() *mock_exec.MockCommand {
				mockCmd := mock_exec.NewMockCommand(gomock.NewController(t))
				mockCmd.EXPECT().Execute(gomock.Any(), "mkfs.xfs", "-f", "/dev/sda1").Return(nil, nil).Times(1)
				return mockCmd
			},
			expectError: false,
		},
		"Success with ext3": {
			device:  "/dev/sda1",
			fstype:  Ext3,
			options: "",
			getMockCmd: func() *mock_exec.MockCommand {
				mockCmd := mock_exec.NewMockCommand(gomock.NewController(t))
				mockCmd.EXPECT().Execute(gomock.Any(), "mkfs.ext3", "-F", "/dev/sda1").Return(nil, nil).Times(1)
				return mockCmd
			},
			expectError: false,
		},
		"Success with ext4": {
			device:  "/dev/sda1",
			fstype:  Ext4,
			options: "",
			getMockCmd: func() *mock_exec.MockCommand {
				mockCmd := mock_exec.NewMockCommand(gomock.NewController(t))
				mockCmd.EXPECT().Execute(gomock.Any(), "mkfs.ext4", "-F", "/dev/sda1").Return(nil, nil).Times(1)
				return mockCmd
			},
			expectError: false,
		},
		"Unsupported filesystem": {
			device:      "/dev/sda1",
			fstype:      "unsupported",
			options:     "",
			getMockCmd:  nil,
			expectError: true,
		},
		"Error formatting ext4": {
			device:  "/dev/sda1",
			fstype:  Ext4,
			options: "",
			getMockCmd: func() *mock_exec.MockCommand {
				mockCmd := mock_exec.NewMockCommand(gomock.NewController(t))
				mockCmd.EXPECT().Execute(gomock.Any(), "mkfs.ext4", "-F", "/dev/sda1").Return([]byte("error"),
					errors.New("mock error")).AnyTimes()
				return mockCmd
			},
			expectError: true,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			var mockCmd *mock_exec.MockCommand
			if params.getMockCmd != nil {
				mockCmd = params.getMockCmd()
			}
			fsClient := NewDetailed(mockCmd, afero.NewMemMapFs(), nil)
			err := fsClient.formatVolumeRetry(context.Background(), params.device, params.fstype, params.options)
			if params.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func newExitErrorForCode(t *testing.T, code int) error {
	t.Helper()
	exePath, err := os.Executable()
	assert.NoError(t, err)

	// Spawn this test binary as a helper process and force it to exit with the requested code.
	// This gives us a real *exec.ExitError in a cross-platform way (no shell dependency).
	cmd := exec.Command(exePath, "-test.run=TestExitWithCodeHelperProcess", "--", strconv.Itoa(code))
	// Gate helper mode so normal test runs don't execute the forced os.Exit path.
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	err = cmd.Run()
	assert.Error(t, err)
	var exitErr *exec.ExitError
	assert.ErrorAs(t, err, &exitErr)
	return err
}

func TestExitWithCodeHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	for i, arg := range os.Args {
		if arg != "--" || i+1 >= len(os.Args) {
			continue
		}

		code, err := strconv.Atoi(os.Args[i+1])
		if err != nil {
			os.Exit(2)
		}
		os.Exit(code)
	}

	os.Exit(2)
}

func TestRepairVolume(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockExec := mock_exec.NewMockCommand(mockCtrl)
	fsClient := NewDetailed(mockExec, nil, nil)

	type parameters struct {
		device    string
		fstype    string
		mockSetup func()
		logLevel  log.Level
		exitCode  *int
		reason    string
	}
	tests := map[string]parameters{
		"fsck.xfs does nothing": {
			device:    "/dev/sda1",
			fstype:    Xfs,
			mockSetup: func() {},
		},
		"Success with ext3": {
			device: "/dev/sda1",
			fstype: Ext3,
			mockSetup: func() {
				mockExec.EXPECT().Execute(gomock.Any(), "fsck.ext3", "-p", "/dev/sda1").Return(nil, nil).Times(1)
			},
		},
		"Success with ext4": {
			device: "/dev/sda1",
			fstype: Ext4,
			mockSetup: func() {
				mockExec.EXPECT().Execute(gomock.Any(), "fsck.ext4", "-p", "/dev/sda1").Return(nil, nil).Times(1)
			},
		},
		"Unsupported filesystem": {
			device:    "/dev/sda1",
			fstype:    "unsupported",
			mockSetup: func() {},
			logLevel:  log.ErrorLevel,
			reason:    "unsupported_fstype",
		},
		"Error executing fsck": {
			device: "/dev/sda1",
			fstype: Ext4,
			mockSetup: func() {
				mockExec.EXPECT().Execute(gomock.Any(), "fsck.ext4", "-p", "/dev/sda1").Return(nil, errors.New("mock error")).Times(1)
			},
			logLevel: log.ErrorLevel,
			reason:   "fsck_exec_error",
		},
		"Unexpected fsck exit code is handled": {
			device: "/dev/sda1",
			fstype: Ext4,
			mockSetup: func() {
				mockExec.EXPECT().Execute(
					gomock.Any(), "fsck.ext4", "-p", "/dev/sda1",
				).Return(nil, newExitErrorForCode(t, 42)).Times(1)
			},
			logLevel: log.ErrorLevel,
			exitCode: func() *int { v := 42; return &v }(),
			reason:   "fsck_unexpected_exit_code",
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			globalLogMutationMutex.Lock()
			originalDefaultLevel := logging.GetDefaultLogLevel()
			hook := logtest.NewGlobal()
			assert.NoError(t, logging.SetDefaultLogLevel("trace"))
			t.Cleanup(func() {
				hook.Reset()
				assert.NoError(t, logging.SetDefaultLogLevel(originalDefaultLevel))
				globalLogMutationMutex.Unlock()
			})

			params.mockSetup()
			fsClient.RepairVolume(context.Background(), params.device, params.fstype)
			if params.exitCode != nil || params.reason != "" {
				found := false
				for _, e := range hook.AllEntries() {
					if e.Level != params.logLevel {
						continue
					}
					if params.exitCode != nil {
						rawCode, ok := e.Data["exitCode"]
						if !ok {
							continue
						}
						assert.Equal(t, *params.exitCode, rawCode)
					}
					if params.reason != "" {
						rawReason, ok := e.Data["reason"]
						if !ok {
							continue
						}
						assert.Equal(t, params.reason, rawReason)
					}
					found = true
					break
				}
				assert.True(
					t, found, "expected matching log criteria level=%s exitCode=%v reason=%q, got entries: %#v",
					params.logLevel, params.exitCode, params.reason, hook.AllEntries(),
				)
			}
		})
	}
}
