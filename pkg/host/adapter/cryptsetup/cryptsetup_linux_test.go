// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cryptsetup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
)

type fakeExitError struct {
	code int
	msg  string
}

func (e *fakeExitError) Error() string { return e.msg }
func (e *fakeExitError) ExitCode() int { return e.code }

func assertExitCode(t *testing.T, err error, expected int) {
	t.Helper()
	type exitCoder interface {
		ExitCode() int
	}
	var target exitCoder
	if assert.ErrorAs(t, err, &target) {
		assert.Equal(t, expected, target.ExitCode())
	}
}

// TestFormat covers formatting with full options, minimal options, and error paths.
func TestFormat(t *testing.T) {
	cases := map[string]struct {
		opts      FormatOptions
		mockOut   []byte
		mockErr   error
		assertErr assert.ErrorAssertionFunc
	}{
		"success_full_options": {
			opts:      DefaultFormatOptions(),
			mockOut:   []byte("formatted"),
			mockErr:   nil,
			assertErr: assert.NoError,
		},
		"success_minimal_options": {
			opts:      FormatOptions{},
			mockOut:   []byte("formatted-min"),
			mockErr:   nil,
			assertErr: assert.NoError,
		},
		"failure_exit_error": {
			opts:      DefaultFormatOptions(),
			mockOut:   []byte("device not found"),
			mockErr:   &fakeExitError{code: exitWrongDevice, msg: "device not found"},
			assertErr: assert.Error,
		},
		"failure_non_exit_error": {
			opts:      DefaultFormatOptions(),
			mockOut:   []byte(""),
			mockErr:   errors.New("format failed"),
			assertErr: assert.Error,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mc := mockexec.NewMockCommand(ctrl)

			// Build expected args
			expectedArgs := []string{"luksFormat", "/dev/sdb"}
			if tc.opts.Type != "" {
				expectedArgs = append(expectedArgs, "--type", tc.opts.Type)
			}
			if tc.opts.Cipher != "" {
				expectedArgs = append(expectedArgs, "-c", tc.opts.Cipher)
			}
			if tc.opts.Hash != "" {
				expectedArgs = append(expectedArgs, "--hash", tc.opts.Hash)
			}
			if tc.opts.PBKDF != "" {
				expectedArgs = append(expectedArgs, "--pbkdf", tc.opts.PBKDF)
			}
			if tc.opts.PBKDFIterations != "" {
				expectedArgs = append(expectedArgs, "--pbkdf-force-iterations", tc.opts.PBKDFIterations)
			}

			// Use DoAndReturn to validate variadic args
			mc.EXPECT().ExecuteWithTimeoutAndInput(
				gomock.Any(), "cryptsetup", gomock.Any(), true, "pw", gomock.Any(),
			).DoAndReturn(func(_ context.Context, _ string, _ time.Duration, _ bool, _ string, args ...string) ([]byte, error) {
				assert.Equal(t, expectedArgs, args)
				return tc.mockOut, tc.mockErr
			})

			ad := &adapter{cmd: mc}
			out, err := ad.Format(context.Background(), "/dev/sdb", "pw", tc.opts)
			if tc.assertErr(t, err) && err == nil {
				assert.Equal(t, string(tc.mockOut), out)
			}
		})
	}
}

// TestOpen covers success, exit error (bad passphrase), generic exit error & non-exit error.
func TestOpen(t *testing.T) {
	cases := map[string]struct {
		mockErr   error
		mockOut   []byte
		wantCode  int
		assertErr assert.ErrorAssertionFunc
	}{
		"success": {
			mockErr:   nil,
			mockOut:   []byte("opened"),
			assertErr: assert.NoError,
		},
		"bad_passphrase": {
			mockErr:   &fakeExitError{code: exitNoPerm, msg: "bad pass"},
			mockOut:   []byte(""),
			wantCode:  exitNoPerm,
			assertErr: assert.Error,
		},
		"unexpected_exit_error": {
			mockErr:   &fakeExitError{code: 7, msg: "unexpected"},
			mockOut:   []byte(""),
			wantCode:  7,
			assertErr: assert.Error,
		},
		"non_exit_error": {
			mockErr:   errors.New("raw error"),
			mockOut:   []byte(""),
			assertErr: assert.Error,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mc := mockexec.NewMockCommand(ctrl)
			mc.EXPECT().ExecuteWithTimeoutAndInput(
				gomock.Any(), "cryptsetup", gomock.Any(), true, "pw",
				"open", "/dev/sda", "luks-vol", "--type", luksType, "--allow-discards",
			).Return(tc.mockOut, tc.mockErr)
			ad := &adapter{cmd: mc}
			err := ad.Open(context.Background(), "/dev/sda", "luks-vol", "pw", DefaultOpenOptions())
			tc.assertErr(t, err)
			if tc.wantCode != 0 {
				assertExitCode(t, err, tc.wantCode)
			}
		})
	}
}

// TestClose covers success, already-closed exit code, unexpected exit & non-exit error.
func TestClose(t *testing.T) {
	cases := map[string]struct {
		mockErr   error
		wantCode  int
		assertErr assert.ErrorAssertionFunc
	}{
		"success": {
			mockErr:   nil,
			assertErr: assert.NoError,
		},
		"already_closed": {
			mockErr:   &fakeExitError{code: exitWrongDevice, msg: "already closed"},
			wantCode:  exitWrongDevice,
			assertErr: assert.Error,
		},
		"unexpected_exit": {
			mockErr:   &fakeExitError{code: 9, msg: "weird"},
			wantCode:  9,
			assertErr: assert.Error,
		},
		"non_exit_error": {
			mockErr:   errors.New("close failed"),
			assertErr: assert.Error,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mc := mockexec.NewMockCommand(ctrl)
			mc.EXPECT().ExecuteWithTimeoutAndInput(
				gomock.Any(), "cryptsetup", execTimeout, true, "",
				"luksClose", "luks-vol",
			).Return([]byte(""), tc.mockErr)
			ad := &adapter{cmd: mc}
			err := ad.Close(context.Background(), "luks-vol")
			tc.assertErr(t, err)
			if tc.wantCode != 0 {
				assertExitCode(t, err, tc.wantCode)
			}
		})
	}
}

// TestResize covers success, incorrect passphrase exit code, unexpected exit & non-exit error.
func TestResize(t *testing.T) {
	cases := map[string]struct {
		mockErr   error
		wantCode  int
		assertErr assert.ErrorAssertionFunc
	}{
		"success": {
			mockErr:   nil,
			assertErr: assert.NoError,
		},
		"incorrect_passphrase": {
			mockErr:   &fakeExitError{code: exitNoPerm, msg: "bad pass"},
			wantCode:  exitNoPerm,
			assertErr: assert.Error,
		},
		"unexpected_exit": {
			mockErr:   &fakeExitError{code: 5, msg: "unexpected"},
			wantCode:  5,
			assertErr: assert.Error,
		},
		"non_exit_error": {
			mockErr:   errors.New("resize failed"),
			assertErr: assert.Error,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mc := mockexec.NewMockCommand(ctrl)
			mc.EXPECT().ExecuteWithTimeoutAndInput(
				gomock.Any(), "cryptsetup", gomock.Any(), true, "pw",
				"resize", "luks-vol",
			).Return([]byte(""), tc.mockErr)
			ad := &adapter{cmd: mc}
			err := ad.Resize(context.Background(), "luks-vol", "pw")
			tc.assertErr(t, err)
			if tc.wantCode != 0 {
				assertExitCode(t, err, tc.wantCode)
			}
		})
	}
}

// TestIsLUKS covers is-luks, not-luks exit code, unexpected exit, non-exit error.
// The not-luks case asserts the carve-out: exit 1 from isLuks is reclassified
// from BadParameterError to NotLUKSDeviceError.
func TestIsLUKS(t *testing.T) {
	cases := map[string]struct {
		mockErr   error
		mockOut   []byte
		wantCode  int
		wantTyped func(t *testing.T, err error)
		assertErr assert.ErrorAssertionFunc
	}{
		"is_luks": {
			mockErr:   nil,
			mockOut:   []byte(""),
			assertErr: assert.NoError,
		},
		"not_luks": {
			mockErr:  &fakeExitError{code: exitWrongParam, msg: "not luks"},
			mockOut:  []byte(""),
			wantCode: exitWrongParam,
			wantTyped: func(t *testing.T, err error) {
				assert.True(t, IsNotLUKSDeviceError(err), "want NotLUKSDeviceError, got %T", err)
				assert.False(t, IsBadParameterError(err), "carve-out must reclassify exit 1 away from BadParameterError")
			},
			assertErr: assert.Error,
		},
		"wrong_device": {
			mockErr:  &fakeExitError{code: exitWrongDevice, msg: "no such device"},
			mockOut:  []byte(""),
			wantCode: exitWrongDevice,
			wantTyped: func(t *testing.T, err error) {
				assert.True(t, IsDeviceNotActiveError(err), "want DeviceNotActiveError, got %T", err)
			},
			assertErr: assert.Error,
		},
		"unexpected_exit": {
			mockErr:   &fakeExitError{code: 7, msg: "unexpected"},
			mockOut:   []byte(""),
			wantCode:  7,
			assertErr: assert.Error,
		},
		"non_exit_error": {
			mockErr:   errors.New("isLuks failed"),
			mockOut:   []byte(""),
			assertErr: assert.Error,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mc := mockexec.NewMockCommand(ctrl)
			mc.EXPECT().ExecuteWithTimeoutAndInput(
				gomock.Any(), "cryptsetup", gomock.Any(), true, "",
				"isLuks", "/dev/sdc",
			).Return(tc.mockOut, tc.mockErr)
			ad := &adapter{cmd: mc}
			err := ad.IsLUKS(context.Background(), "/dev/sdc")
			tc.assertErr(t, err)
			if tc.wantCode != 0 {
				assertExitCode(t, err, tc.wantCode)
			}
			if tc.wantTyped != nil {
				tc.wantTyped(t, err)
			}
		})
	}
}

// TestStatus covers success, device-not-found exit code, unexpected exit & non-exit error.
func TestStatus(t *testing.T) {
	cases := map[string]struct {
		mockErr   error
		mockOut   []byte
		wantCode  int
		assertErr assert.ErrorAssertionFunc
	}{
		"success": {
			mockErr:   nil,
			mockOut:   []byte("active"),
			assertErr: assert.NoError,
		},
		"not_found": {
			mockErr:   &fakeExitError{code: exitWrongDevice, msg: "not found"},
			mockOut:   []byte(""),
			wantCode:  exitWrongDevice,
			assertErr: assert.Error,
		},
		"unexpected_exit": {
			mockErr:   &fakeExitError{code: 11, msg: "unexpected"},
			mockOut:   []byte(""),
			wantCode:  11,
			assertErr: assert.Error,
		},
		"non_exit_error": {
			mockErr:   errors.New("status fail"),
			mockOut:   []byte(""),
			assertErr: assert.Error,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mc := mockexec.NewMockCommand(ctrl)
			mc.EXPECT().ExecuteWithTimeoutAndInput(
				gomock.Any(), "cryptsetup", gomock.Any(), true, "",
				"status", "luks-vol",
			).Return(tc.mockOut, tc.mockErr)
			ad := &adapter{cmd: mc}
			out, err := ad.Status(context.Background(), "luks-vol")
			tc.assertErr(t, err)
			if err == nil {
				assert.Equal(t, string(tc.mockOut), out)
			}
			if tc.wantCode != 0 {
				assertExitCode(t, err, tc.wantCode)
			}
		})
	}
}

// TestChangeKey covers validation errors, success, command failure and exit error.
func TestChangeKey(t *testing.T) {
	cases := map[string]struct {
		oldPass   string
		newPass   string
		mockErr   error
		wantCode  int
		assertErr assert.ErrorAssertionFunc
	}{
		"missing_old_passphrase": {
			oldPass:   "",
			newPass:   "new",
			mockErr:   nil,
			assertErr: assert.Error,
		},
		"missing_new_passphrase": {
			oldPass:   "old",
			newPass:   "",
			mockErr:   nil,
			assertErr: assert.Error,
		},
		"success": {
			oldPass:   "old",
			newPass:   "new",
			mockErr:   nil,
			assertErr: assert.NoError,
		},
		"exit_error": {
			oldPass:   "old",
			newPass:   "new",
			mockErr:   &fakeExitError{code: exitNoPerm, msg: "bad pass"},
			wantCode:  exitNoPerm,
			assertErr: assert.Error,
		},
		"non_exit_error": {
			oldPass:   "old",
			newPass:   "new",
			mockErr:   errors.New("change failed"),
			assertErr: assert.Error,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mc := mockexec.NewMockCommand(ctrl)
			if tc.oldPass != "" && tc.newPass != "" {
				mc.EXPECT().ExecuteWithTimeoutAndInput(
					gomock.Any(), "cryptsetup", gomock.Any(), true, tc.newPass,
					"luksChangeKey", "-d", gomock.Any(), "/dev/sdz",
				).Return([]byte(""), tc.mockErr)
			}
			ad := &adapter{cmd: mc}
			err := ad.ChangeKey(context.Background(), "/dev/sdz", "uuid-123", tc.oldPass, tc.newPass)
			tc.assertErr(t, err)
			if tc.wantCode != 0 {
				assertExitCode(t, err, tc.wantCode)
			}
		})
	}
}

// TestTestPassphrase covers success, bad passphrase exit code, unexpected exit, non-exit error.
func TestTestPassphrase(t *testing.T) {
	cases := map[string]struct {
		mockErr   error
		mockOut   []byte
		wantCode  int
		assertErr assert.ErrorAssertionFunc
	}{
		"success": {
			mockErr:   nil,
			mockOut:   []byte(""),
			assertErr: assert.NoError,
		},
		"bad_passphrase": {
			mockErr:   &fakeExitError{code: exitNoPerm, msg: "bad"},
			mockOut:   []byte(""),
			wantCode:  exitNoPerm,
			assertErr: assert.Error,
		},
		"unexpected_exit": {
			mockErr:   &fakeExitError{code: 9, msg: "unexpected"},
			mockOut:   []byte(""),
			wantCode:  9,
			assertErr: assert.Error,
		},
		"non_exit_error": {
			mockErr:   errors.New("test failed"),
			mockOut:   []byte(""),
			assertErr: assert.Error,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mc := mockexec.NewMockCommand(ctrl)
			mc.EXPECT().ExecuteWithTimeoutAndInput(
				gomock.Any(), "cryptsetup", gomock.Any(), true, "pw",
				"open", "/dev/sdx", "luks-vol", "--type", "luks2", "--test-passphrase",
			).Return(tc.mockOut, tc.mockErr)
			ad := &adapter{cmd: mc}
			err := ad.TestPassphrase(context.Background(), "/dev/sdx", "luks-vol", "pw")
			tc.assertErr(t, err)
			if tc.wantCode != 0 {
				assertExitCode(t, err, tc.wantCode)
			}
		})
	}
}

func FuzzTestPassphraseMapperName(f *testing.F) {
	// Seed mapper names.
	f.Add("luks-1")
	f.Add("luks-pvc_123")
	f.Add("my_mapper")
	f.Add("LUKS-UPPER")

	f.Fuzz(func(t *testing.T, mapper string) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mc := mockexec.NewMockCommand(ctrl)

		mc.EXPECT().ExecuteWithTimeoutAndInput(
			gomock.Any(), "cryptsetup", gomock.Any(), true, "pw",
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).DoAndReturn(func(_ context.Context, _ string, _ time.Duration, _ bool, _ string, argv ...string) ([]byte, error) {
			// argv: "open", devicePath, mapperName, "--type", "luks2", "--test-passphrase"
			if len(argv) < 3 {
				t.Fatalf("too few args: %v", argv)
			}
			gotMapper := argv[2]
			assert.Equal(t, mapper, gotMapper, "mapper name not passed through")
			return []byte(""), nil
		})

		ad := &adapter{cmd: mc}
		err := ad.TestPassphrase(context.Background(), "/dev/sdx", mapper, "pw")
		assert.NoError(t, err)
	})
}
