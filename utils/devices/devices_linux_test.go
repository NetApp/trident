// Copyright 2024 NetApp, Inc. All Rights Reserved.

// NOTE: This file should only contain functions for handling devices for linux flavor

package devices

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/exec"
)

func mockCryptsetupLuksClose(mock *mockexec.MockCommand) *gomock.Call {
	return mock.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", luksCloseTimeout, true, "", "luksClose", gomock.Any(),
	)
}

func TestEnsureLUKSDeviceClosedWithMaxWaitLimit(t *testing.T) {
	osFs := afero.NewMemMapFs()
	luksDevicePath := "/dev/mapper/luks-test"
	osFs.Create(luksDevicePath)
	client := mockexec.NewMockCommand(gomock.NewController(t))
	deviceClient := NewDetailed(client, osFs)

	type testCase struct {
		name            string
		mockSetup       func(*mockexec.MockCommand)
		expectedError   bool
		expectedErrType error
	}

	testCases := []testCase{
		{
			name: "SucceedsWhenDeviceIsClosed",
			mockSetup: func(mockCommand *mockexec.MockCommand) {
				mockCryptsetupLuksClose(mockCommand).Return([]byte(""), nil)
			},
			expectedError: false,
		},
		{
			name: "FailsBeforeMaxWaitLimit",
			mockSetup: func(mockCommand *mockexec.MockCommand) {
				mockCryptsetupLuksClose(mockCommand).Return([]byte(""), fmt.Errorf("close error"))
			},
			expectedError:   true,
			expectedErrType: fmt.Errorf("%w", errors.New("")),
		},
		{
			name: "FailsWithMaxWaitExceededError",
			mockSetup: func(mockCommand *mockexec.MockCommand) {
				mockCryptsetupLuksClose(mockCommand).Return([]byte(""), fmt.Errorf("close error"))
				LuksCloseDurations[luksDevicePath] = time.Now().Add(-luksCloseMaxWaitDuration - time.Second)
			},
			expectedError:   true,
			expectedErrType: errors.MaxWaitExceededError(""),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup(client)
			err := deviceClient.EnsureLUKSDeviceClosedWithMaxWaitLimit(context.TODO(), luksDevicePath)
			if tc.expectedError {
				assert.Error(t, err)
				if tc.expectedErrType != nil {
					assert.IsType(t, tc.expectedErrType, err)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_CloseLUKSDevice(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	deviceClient := NewDetailed(mockCommand, afero.NewMemMapFs())

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupLuksClose(mockCommand).Return([]byte(""), nil)

	err := deviceClient.CloseLUKSDevice(context.Background(), "/dev/sdb")
	assert.NoError(t, err)
}

func TestEnsureLUKSDeviceClosed_DeviceDoesNotExist(t *testing.T) {
	deviceClient := NewDetailed(exec.NewCommand(), afero.NewMemMapFs())
	err := deviceClient.EnsureLUKSDeviceClosed(context.Background(), "/dev/mapper/luks-test-dev")
	assert.NoError(t, err)
}

func TestEnsureLUKSDeviceClosed_FailsToDetectDevice(t *testing.T) {
	osFs := afero.NewOsFs()
	var b strings.Builder
	b.Grow(1025)
	for i := 0; i < 1025; i++ {
		b.WriteByte('a')
	}
	s := b.String()
	deviceClient := NewDetailed(exec.NewCommand(), osFs)
	err := deviceClient.EnsureLUKSDeviceClosed(context.Background(), "/dev/mapper/"+s)
	assert.Error(t, err)
}

func TestEnsureLUKSDeviceClosed_FailsToCloseDevice(t *testing.T) {
	ctx := context.Background()
	devicePath := "/dev/mapper/luks-test-dev"
	osFs := afero.NewMemMapFs()
	osFs.Create(devicePath)

	deviceClient := NewDetailed(exec.NewCommand(), osFs)
	err := deviceClient.EnsureLUKSDeviceClosed(ctx, devicePath)
	assert.Error(t, err)
}

func TestEnsureLUKSDeviceClosed_ClosesDevice(t *testing.T) {
	ctx := context.Background()
	devicePath := "/dev/mapper/luks-test-dev"
	osFs := afero.NewMemMapFs()
	osFs.Create(devicePath)

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	// Setup mock calls and reassign any clients to their mock counterparts.
	gomock.InOrder(
		mockCryptsetupLuksClose(mockCommand).Return([]byte(""), nil),
	)

	deviceClient := NewDetailed(mockCommand, osFs)
	err := deviceClient.EnsureLUKSDeviceClosed(ctx, devicePath)
	assert.NoError(t, err)
}
