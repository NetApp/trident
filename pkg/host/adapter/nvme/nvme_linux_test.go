// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	tridentErrors "github.com/netapp/trident/utils/errors"
)

type fakeExitError struct {
	code int
	msg  string
}

func (e *fakeExitError) Error() string { return e.msg }
func (e *fakeExitError) ExitCode() int { return e.code }

// statErrFs fails every Stat with err, to model a sysfs stat failure that is not "not exist".
type statErrFs struct {
	afero.Fs
	err error
}

func (f statErrFs) Stat(string) (os.FileInfo, error) { return nil, f.err }

func TestParseExecError(t *testing.T) {
	t.Run("exit_error", func(t *testing.T) {
		err := parseExecError("version", []byte("boom"), &fakeExitError{code: 3, msg: "exit status 3"})

		var target *Error
		assert.ErrorAs(t, err, &target)
		assert.Equal(t, 3, target.ExitCode())
		assert.Equal(t, "boom", target.Output())
		assert.Equal(t, "nvme", target.Binary())
		assert.Equal(t, "version", target.Cmd())
	})

	t.Run("not_executable", func(t *testing.T) {
		base := errors.New("executable file not found in $PATH")
		err := parseExecError("version", nil, base)

		assert.ErrorIs(t, err, base)
		var target *Error
		assert.False(t, errors.As(err, &target))
	})
}

func TestVersion(t *testing.T) {
	cases := map[string]struct {
		mockOut   []byte
		mockErr   error
		want      string
		assertErr assert.ErrorAssertionFunc
	}{
		"success": {
			mockOut:   []byte("nvme version 2.8 (git 2.8)\n"),
			want:      "nvme version 2.8 (git 2.8)",
			assertErr: assert.NoError,
		},
		"exit_error": {
			mockErr:   &fakeExitError{code: 1, msg: "exit status 1"},
			assertErr: assert.Error,
		},
		"not_installed": {
			mockErr:   errors.New("executable file not found in $PATH"),
			assertErr: assert.Error,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mc := mockexec.NewMockCommand(ctrl)
			mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", execTimeout, false, "version").
				Return(tc.mockOut, tc.mockErr)

			ad := &adapter{cmd: mc}
			out, err := ad.Version(context.Background())
			tc.assertErr(t, err)
			if err == nil {
				assert.Equal(t, tc.want, out)
			}
		})
	}
}

func TestVersion_ErrorTypes(t *testing.T) {
	ctrl := gomock.NewController(t)
	mc := mockexec.NewMockCommand(ctrl)
	mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", execTimeout, false, "version").
		Return([]byte("usage"), &fakeExitError{code: 1, msg: "exit status 1"})

	_, err := (&adapter{cmd: mc}).Version(context.Background())

	var target *Error
	assert.ErrorAs(t, err, &target)
	assert.Equal(t, 1, target.ExitCode())
}

func TestHostNQN(t *testing.T) {
	const nqn = "nqn.2014-08.org.nvmexpress:uuid:4c4c4544-0042-3510-8051-b8c04f4c4e32"

	cases := map[string]struct {
		out       string
		err       error
		want      string
		assertErr assert.ErrorAssertionFunc
	}{
		"single_line":         {out: nqn + "\n", want: nqn, assertErr: assert.NoError},
		"no_trailing_newline": {out: nqn, want: nqn, assertErr: assert.NoError},
		"extra_lines_ignored": {out: nqn + "\nsecond\n", want: nqn, assertErr: assert.NoError},
		"surrounding_spaces":  {out: "  " + nqn + "  \n", want: nqn, assertErr: assert.NoError},
		"empty_file":          {out: "", want: "", assertErr: assert.NoError},
		"cat_fails":           {err: &fakeExitError{code: 1, msg: "exit status 1"}, assertErr: assert.Error},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mc := mockexec.NewMockCommand(ctrl)
			mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "cat", execTimeout, false, "/etc/nvme/hostnqn").
				Return([]byte(tc.out), tc.err)

			got, err := (&adapter{cmd: mc}).HostNQN(context.Background())
			tc.assertErr(t, err)
			if err == nil {
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestTransportLoaded(t *testing.T) {
	const lsmodWithTCP = "Module                  Size  Used by\n" +
		"nvme_tcp               57344  0\n" +
		"nvme_fabrics           32768  1 nvme_tcp\n"
	const lsmodWithoutTCP = "Module                  Size  Used by\n" +
		"nvme_core             200704  0\n"

	cases := map[string]struct {
		setupFs   func(afero.Fs) afero.Fs
		lsmodOut  string
		lsmodErr  error
		runsLsmod bool
		want      bool
		assertErr assert.ErrorAssertionFunc
	}{
		"sysfs_dir_present_skips_lsmod": {
			setupFs: func(fs afero.Fs) afero.Fs {
				assert.NoError(t, fs.MkdirAll("/sys/module/nvme_tcp", 0o755))
				return fs
			},
			want:      true,
			assertErr: assert.NoError,
		},
		"sysfs_missing_lsmod_lists_module": {
			setupFs:   func(fs afero.Fs) afero.Fs { return fs },
			lsmodOut:  lsmodWithTCP,
			runsLsmod: true,
			want:      true,
			assertErr: assert.NoError,
		},
		"sysfs_missing_lsmod_lacks_module": {
			setupFs:   func(fs afero.Fs) afero.Fs { return fs },
			lsmodOut:  lsmodWithoutTCP,
			runsLsmod: true,
			want:      false,
			assertErr: assert.NoError,
		},
		"sysfs_path_is_a_file_falls_through_to_lsmod": {
			setupFs: func(fs afero.Fs) afero.Fs {
				assert.NoError(t, afero.WriteFile(fs, "/sys/module/nvme_tcp", []byte("x"), 0o644))
				return fs
			},
			lsmodOut:  lsmodWithTCP,
			runsLsmod: true,
			want:      true,
			assertErr: assert.NoError,
		},
		"sysfs_stat_error_falls_through_to_lsmod": {
			setupFs:   func(fs afero.Fs) afero.Fs { return statErrFs{Fs: fs, err: os.ErrPermission} },
			lsmodOut:  lsmodWithTCP,
			runsLsmod: true,
			want:      true,
			assertErr: assert.NoError,
		},
		"sysfs_missing_lsmod_fails": {
			setupFs:   func(fs afero.Fs) afero.Fs { return fs },
			lsmodErr:  errors.New("lsmod failed"),
			runsLsmod: true,
			want:      false,
			assertErr: assert.Error,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mc := mockexec.NewMockCommand(ctrl)
			if tc.runsLsmod {
				mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "lsmod", execTimeout, false).
					Return([]byte(tc.lsmodOut), tc.lsmodErr)
			}

			ad := &adapter{cmd: mc, fs: tc.setupFs(afero.NewMemMapFs())}
			got, err := ad.TransportLoaded(context.Background(), TransportTCP)
			tc.assertErr(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTransportLoaded_ChrootPrefix(t *testing.T) {
	fs := afero.NewMemMapFs()
	assert.NoError(t, fs.MkdirAll("/host/sys/module/nvme_tcp", 0o755))

	ad := &adapter{fs: fs, chrt: "/host"}
	got, err := ad.TransportLoaded(context.Background(), TransportTCP)
	assert.NoError(t, err)
	assert.True(t, got)
}

const (
	testNQN  = "nqn.1992-08.com.netapp:sn.6628417f7bec11ef9bf2005056b3e634:subsystem.scspa3014048001-b06e4d9a"
	testNQN2 = "nqn.1992-08.com.netapp:sn.6628417f7bec11ef9bf2005056b3e634:subsystem.other"
	testUUID = "ffce3179-8d4b-4c1f-8a0e-6a1c53e3e9a1"
)

type fsFile struct{ path, content string }

func newFs(t *testing.T, files ...fsFile) afero.Fs {
	t.Helper()
	fs := afero.NewMemMapFs()
	for _, f := range files {
		assert.NoError(t, afero.WriteFile(fs, f.path, []byte(f.content), 0o644))
	}
	return fs
}

func ctrlFiles(subsysDir, name, state, address string) []fsFile {
	return []fsFile{
		{subsysDir + "/" + name + "/state", state + "\n"},
		{subsysDir + "/" + name + "/address", address + "\n"},
		{subsysDir + "/" + name + "/transport", "tcp\n"},
	}
}

func TestSubsystem(t *testing.T) {
	const dir = "/sys/class/nvme-subsystem/nvme-subsys0"

	t.Run("epoch_layout", func(t *testing.T) {
		files := []fsFile{
			{dir + "/subsysnqn", testNQN + "\n"},
			{dir + "/nvme0n1/uuid", testUUID + "\n"},
			{dir + "/ng0n1/uuid", "must-not-be-read\n"},
		}
		const src = ",src_addr=10.211.55.6"
		files = append(files, ctrlFiles(dir, "nvme0", "live", "traddr=10.193.156.158,trsvcid=4420"+src)...)
		files = append(files, ctrlFiles(dir, "nvme1", "connecting", "traddr=10.193.156.159,trsvcid=4420"+src)...)

		sub, err := New(WithFs(newFs(t, files...))).Subsystem(context.Background(), testNQN)

		assert.NoError(t, err)
		assert.Equal(t, testNQN, sub.NQN())
		assert.Equal(t, dir, sub.Dir())
		assert.Len(t, sub.Paths(), 2)
		assert.Equal(t, "nvme0", sub.Paths()[0].Name())
		assert.Equal(t, "10.193.156.158", sub.Paths()[0].Address())
		assert.Equal(t, "4420", sub.Paths()[0].Port())
		assert.Equal(t, TransportTCP, sub.Paths()[0].Transport())
		assert.True(t, sub.Paths()[0].IsLive())
		assert.Equal(t, PathStateConnecting, sub.Paths()[1].State())
		assert.False(t, sub.Paths()[1].IsLive())
		assert.True(t, sub.HasLivePath())
		assert.Len(t, sub.Namespaces(), 1)
		assert.Equal(t, "nvme0n1", sub.Namespaces()[0].Name())
		assert.Equal(t, testUUID, sub.Namespaces()[0].UUID())
	})

	t.Run("space_separated_address", func(t *testing.T) {
		files := []fsFile{{dir + "/subsysnqn", testNQN}}
		files = append(files, ctrlFiles(dir, "nvme0", "live", "traddr=10.193.108.74 trsvcid=4420")...)

		sub, err := New(WithFs(newFs(t, files...))).Subsystem(context.Background(), testNQN)

		assert.NoError(t, err)
		assert.Equal(t, "10.193.108.74", sub.Paths()[0].Address())
		assert.Equal(t, "4420", sub.Paths()[0].Port())
	})

	t.Run("unreadable_namespace_uuid_is_empty", func(t *testing.T) {
		files := []fsFile{{dir + "/subsysnqn", testNQN}, {dir + "/nvme0n1/size", "1\n"}}
		files = append(files, ctrlFiles(dir, "nvme0", "live", "traddr=10.0.0.1,trsvcid=4420")...)

		sub, err := New(WithFs(newFs(t, files...))).Subsystem(context.Background(), testNQN)

		assert.NoError(t, err)
		assert.Len(t, sub.Namespaces(), 1)
		assert.Equal(t, "", sub.Namespaces()[0].UUID())
	})

	t.Run("unreadable_path_attributes_are_empty", func(t *testing.T) {
		files := []fsFile{{dir + "/subsysnqn", testNQN}, {dir + "/nvme0/transport", "tcp"}}

		sub, err := New(WithFs(newFs(t, files...))).Subsystem(context.Background(), testNQN)

		assert.NoError(t, err)
		assert.Len(t, sub.Paths(), 1)
		assert.Equal(t, PathStateUnknown, sub.Paths()[0].State())
		assert.Equal(t, "", sub.Paths()[0].Address())
		assert.False(t, sub.HasLivePath())
	})

	t.Run("selects_matching_subsystem_and_skips_unreadable_one", func(t *testing.T) {
		const dir2 = "/sys/class/nvme-subsystem/nvme-subsys1"
		const dir3 = "/sys/class/nvme-subsystem/nvme-subsys2"
		files := []fsFile{
			{dir + "/subsysnqn", testNQN2},
			{dir + "/nvme0n1/uuid", "other-uuid"},
			{dir3 + "/model", "no subsysnqn here"},
			{dir2 + "/subsysnqn", testNQN},
			{dir2 + "/nvme1n1/uuid", testUUID},
		}
		files = append(files, ctrlFiles(dir, "nvme0", "live", "traddr=10.0.0.1,trsvcid=4420")...)
		files = append(files, ctrlFiles(dir2, "nvme1", "live", "traddr=10.0.0.2,trsvcid=4420")...)

		sub, err := New(WithFs(newFs(t, files...))).Subsystem(context.Background(), testNQN)

		assert.NoError(t, err)
		assert.Equal(t, dir2, sub.Dir())
		assert.Len(t, sub.Paths(), 1)
		assert.Equal(t, "10.0.0.2", sub.Paths()[0].Address())
		assert.Equal(t, testUUID, sub.Namespaces()[0].UUID())
	})

	t.Run("chroot_prefix", func(t *testing.T) {
		files := []fsFile{{"/host" + dir + "/subsysnqn", testNQN}}
		files = append(files, ctrlFiles("/host"+dir, "nvme0", "live", "traddr=10.0.0.1,trsvcid=4420")...)

		ad := New(WithFs(newFs(t, files...)), WithChrootPrefix("/host"))
		sub, err := ad.Subsystem(context.Background(), testNQN)

		assert.NoError(t, err)
		assert.Equal(t, dir, sub.Dir())
		assert.Len(t, sub.Paths(), 1)
	})

	notFound := map[string][]fsFile{
		"no_subsystem_directory": nil,
		"no_matching_nqn":        {{dir + "/subsysnqn", testNQN2}},
		"no_paths":               {{dir + "/subsysnqn", testNQN}, {dir + "/nvme0n1/uuid", testUUID}},
	}
	for name, files := range notFound {
		t.Run("not_found_"+name, func(t *testing.T) {
			sub, err := New(WithFs(newFs(t, files...))).Subsystem(context.Background(), testNQN)

			assert.Nil(t, sub)
			assert.True(t, IsSubsystemNotFoundError(err))
		})
	}

	t.Run("list_error", func(t *testing.T) {
		ad := New(WithFs(readDirErrFs{Fs: newFs(t), err: os.ErrPermission}))
		sub, err := ad.Subsystem(context.Background(), testNQN)

		assert.Nil(t, sub)
		assert.ErrorIs(t, err, os.ErrPermission)
		assert.False(t, IsSubsystemNotFoundError(err))
	})

	t.Run("empty_nqn", func(t *testing.T) {
		_, err := New(WithFs(newFs(t))).Subsystem(context.Background(), "")
		assert.Error(t, err)
	})
}

// readDirErrFs fails opening any directory, to model a sysfs listing failure that is not "not exist".
type readDirErrFs struct {
	afero.Fs
	err error
}

func (f readDirErrFs) Open(string) (afero.File, error) { return nil, f.err }

func TestConnect(t *testing.T) {
	ctx := context.Background()

	cases := map[string]struct {
		nqn, addr string
		opts      []ConnectOption
		want      []string
	}{
		"defaults_match_master": {
			nqn: testNQN, addr: "10.0.0.1",
			want: []string{"connect", "-t", "tcp", "-n", testNQN, "-a", "10.0.0.1", "-s", "4420", "-l", "-1"},
		},
		"explicit_port": {
			nqn: testNQN, addr: "10.0.0.1", opts: []ConnectOption{WithPort("4421")},
			want: []string{"connect", "-t", "tcp", "-n", testNQN, "-a", "10.0.0.1", "-s", "4421", "-l", "-1"},
		},
		"other_transport_has_no_port": {
			nqn: testNQN, addr: "some-address", opts: []ConnectOption{WithTransport(testOtherTransport)},
			want: []string{"connect", "-t", "loop", "-n", testNQN, "-a", "some-address", "-l", "-1"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			mc := mockexec.NewMockCommand(gomock.NewController(t))
			args := make([]any, len(tc.want))
			for i, a := range tc.want {
				args[i] = a
			}
			mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", gomock.Any(), true, args...).Return(nil, nil)

			assert.NoError(t, New(WithCommand(mc)).Connect(ctx, tc.nqn, tc.addr, tc.opts...))
		})
	}

	t.Run("exit_error", func(t *testing.T) {
		mc := mockexec.NewMockCommand(gomock.NewController(t))
		mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", gomock.Any(), true, gomock.Any()).
			Return([]byte("already connected"), &fakeExitError{code: 114, msg: "exit status 114"})

		err := New(WithCommand(mc)).Connect(ctx, testNQN, "10.0.0.1")

		var target *Error
		assert.ErrorAs(t, err, &target)
		assert.Equal(t, 114, target.ExitCode())
		assert.Equal(t, "connect", target.Cmd())
		assert.Equal(t, "already connected", target.Output())
	})

	t.Run("invalid_requests_do_not_run_nvme", func(t *testing.T) {
		ad := New(WithCommand(mockexec.NewMockCommand(gomock.NewController(t))))
		assert.Error(t, ad.Connect(ctx, "", "10.0.0.1"))
		assert.Error(t, ad.Connect(ctx, testNQN, ""))
		assert.Error(t, ad.Connect(ctx, testNQN, "-x"))
		assert.Error(t, ad.Connect(ctx, testNQN, "10.0.0.1", WithTransport("")))
		assert.Error(t, ad.Connect(ctx, testNQN, "10.0.0.1", WithTransport("tcp -x")))
		assert.Error(t, ad.Connect(ctx, testNQN, "10.0.0.1", WithPort("http")))
		assert.Error(t, ad.Connect(ctx, testNQN, "10.0.0.1", WithPort("70000")))
		assert.Error(t, ad.Connect(ctx, testNQN, "addr", WithTransport(testOtherTransport), WithPort("4420")))
	})
}

func TestDisconnect(t *testing.T) {
	ctx := context.Background()

	t.Run("args", func(t *testing.T) {
		mc := mockexec.NewMockCommand(gomock.NewController(t))
		mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", gomock.Any(), true, "disconnect", "-n", testNQN).
			Return(nil, nil)

		assert.NoError(t, New(WithCommand(mc)).Disconnect(ctx, testNQN))
	})

	t.Run("exit_error", func(t *testing.T) {
		mc := mockexec.NewMockCommand(gomock.NewController(t))
		mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", gomock.Any(), true, "disconnect", "-n", testNQN).
			Return([]byte("boom"), &fakeExitError{code: 1, msg: "exit status 1"})

		var target *Error
		assert.ErrorAs(t, New(WithCommand(mc)).Disconnect(ctx, testNQN), &target)
		assert.Equal(t, "disconnect", target.Cmd())
	})

	t.Run("empty_nqn", func(t *testing.T) {
		ad := New(WithCommand(mockexec.NewMockCommand(gomock.NewController(t))))
		assert.Error(t, ad.Disconnect(ctx, ""))
	})
}

func TestDisconnectPath(t *testing.T) {
	ctx := context.Background()

	t.Run("args", func(t *testing.T) {
		mc := mockexec.NewMockCommand(gomock.NewController(t))
		mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", disconnectPathTimeout, true, "disconnect", "-d", "nvme0").
			Return(nil, nil)

		assert.NoError(t, New(WithCommand(mc)).DisconnectPath(ctx, "nvme0"))
	})

	t.Run("exit_error", func(t *testing.T) {
		mc := mockexec.NewMockCommand(gomock.NewController(t))
		mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", gomock.Any(), true, "disconnect", "-d", "nvme0").
			Return([]byte("boom"), &fakeExitError{code: 1, msg: "exit status 1"})

		var target *Error
		assert.ErrorAs(t, New(WithCommand(mc)).DisconnectPath(ctx, "nvme0"), &target)
		assert.Equal(t, "disconnect", target.Cmd())
	})

	t.Run("timeout_is_classifiable", func(t *testing.T) {
		mc := mockexec.NewMockCommand(gomock.NewController(t))
		mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", gomock.Any(), true, "disconnect", "-d", "nvme0").
			Return(nil, tridentErrors.TimeoutError("process killed after timeout"))

		err := New(WithCommand(mc)).DisconnectPath(ctx, "nvme0")

		assert.True(t, tridentErrors.IsTimeoutError(err))
	})

	t.Run("bounded_by_a_shorter_caller_deadline", func(t *testing.T) {
		short, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		within := gomock.Cond(func(d time.Duration) bool { return d > 0 && d <= time.Second })
		mc := mockexec.NewMockCommand(gomock.NewController(t))
		mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", within, true, "disconnect", "-d", "nvme0").
			Return(nil, nil)

		assert.NoError(t, New(WithCommand(mc)).DisconnectPath(short, "nvme0"))
	})

	t.Run("bounded_when_the_caller_deadline_is_longer", func(t *testing.T) {
		long, cancel := context.WithTimeout(ctx, time.Hour)
		defer cancel()
		mc := mockexec.NewMockCommand(gomock.NewController(t))
		mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", disconnectPathTimeout, true, "disconnect", "-d", "nvme0").
			Return(nil, nil)

		assert.NoError(t, New(WithCommand(mc)).DisconnectPath(long, "nvme0"))
	})

	t.Run("not_executable", func(t *testing.T) {
		notFound := errors.New("executable file not found in $PATH")
		mc := mockexec.NewMockCommand(gomock.NewController(t))
		mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", gomock.Any(), true, "disconnect", "-d", "nvme0").
			Return(nil, notFound)

		err := New(WithCommand(mc)).DisconnectPath(ctx, "nvme0")

		assert.ErrorIs(t, err, notFound)
		var target *Error
		assert.False(t, errors.As(err, &target))
	})

	t.Run("invalid_controller_does_not_run_nvme", func(t *testing.T) {
		ad := New(WithCommand(mockexec.NewMockCommand(gomock.NewController(t))))
		assert.Error(t, ad.DisconnectPath(ctx, ""))
		assert.Error(t, ad.DisconnectPath(ctx, "-n"))
		assert.Error(t, ad.DisconnectPath(ctx, "/sys/class/nvme/nvme0"))
		assert.Error(t, ad.DisconnectPath(ctx, "nvme0/.."))
	})
}

func TestFlush(t *testing.T) {
	ctx := context.Background()

	t.Run("args", func(t *testing.T) {
		mc := mockexec.NewMockCommand(gomock.NewController(t))
		mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", gomock.Any(), true, "flush", "/dev/nvme0n1").Return(nil, nil)

		assert.NoError(t, New(WithCommand(mc)).Flush(ctx, "/dev/nvme0n1"))
	})

	t.Run("exit_error", func(t *testing.T) {
		mc := mockexec.NewMockCommand(gomock.NewController(t))
		mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", gomock.Any(), true, "flush", "/dev/nvme0n1").
			Return([]byte("boom"), &fakeExitError{code: 1, msg: "exit status 1"})

		var target *Error
		assert.ErrorAs(t, New(WithCommand(mc)).Flush(ctx, "/dev/nvme0n1"), &target)
		assert.Equal(t, "flush", target.Cmd())
	})

	t.Run("invalid_path", func(t *testing.T) {
		ad := New(WithCommand(mockexec.NewMockCommand(gomock.NewController(t))))
		assert.Error(t, ad.Flush(ctx, ""))
		assert.Error(t, ad.Flush(ctx, "-h"))
	})
}

func TestTransportLoaded_OtherTransports(t *testing.T) {
	t.Run("other_transport_module_dir", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		assert.NoError(t, fs.MkdirAll("/sys/module/nvme_loop", 0o755))

		got, err := (&adapter{fs: fs}).TransportLoaded(context.Background(), testOtherTransport)

		assert.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("tcp_module_does_not_satisfy_other_transport", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		assert.NoError(t, fs.MkdirAll("/sys/module/nvme_tcp", 0o755))
		mc := mockexec.NewMockCommand(gomock.NewController(t))
		mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "lsmod", execTimeout, false).
			Return([]byte("nvme_tcp 57344 0\n"), nil)

		got, err := (&adapter{cmd: mc, fs: fs}).TransportLoaded(context.Background(), testOtherTransport)

		assert.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("module_names_sharing_a_prefix_do_not_match", func(t *testing.T) {
		mc := mockexec.NewMockCommand(gomock.NewController(t))
		mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "lsmod", execTimeout, false).
			Return([]byte("Module Size Used by\nnvme_loopback 20480 0\nnvme_fabrics 36864 1 nvme_loop\n"), nil)

		got, err := (&adapter{cmd: mc, fs: afero.NewMemMapFs()}).TransportLoaded(context.Background(), testOtherTransport)

		assert.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("invalid_transport", func(t *testing.T) {
		got, err := (&adapter{fs: afero.NewMemMapFs()}).TransportLoaded(context.Background(), "../x")

		assert.Error(t, err)
		assert.False(t, got)
	})
}

func TestCommandTimeout(t *testing.T) {
	t.Run("no_deadline_is_unbounded", func(t *testing.T) {
		assert.Equal(t, noTimeout, commandTimeout(context.Background()))
	})

	t.Run("deadline_is_time_left", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		got := commandTimeout(ctx)

		assert.Greater(t, got, 59*time.Second)
		assert.LessOrEqual(t, got, time.Minute)
	})

	t.Run("expired_deadline_is_not_positive", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()

		assert.LessOrEqual(t, commandTimeout(ctx), time.Duration(0))
	})
}

func TestCommands_PassCallerDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	within := gomock.Cond(func(d time.Duration) bool { return d > 0 && d <= time.Minute })

	mc := mockexec.NewMockCommand(gomock.NewController(t))
	mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", within, true,
		"connect", "-t", "tcp", "-n", testNQN, "-a", "10.0.0.1", "-s", "4420", "-l", "-1").Return(nil, nil)
	mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", within, true, "disconnect", "-n", testNQN).Return(nil, nil)
	mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", within, true, "flush", "/dev/nvme0n1").Return(nil, nil)
	ad := New(WithCommand(mc))

	assert.NoError(t, ad.Connect(ctx, testNQN, "10.0.0.1"))
	assert.NoError(t, ad.Disconnect(ctx, testNQN))
	assert.NoError(t, ad.Flush(ctx, "/dev/nvme0n1"))
}

func TestCommands_TimeoutIsClassifiable(t *testing.T) {
	timeout := tridentErrors.TimeoutError("process killed after timeout")
	mc := mockexec.NewMockCommand(gomock.NewController(t))
	mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", gomock.Any(), true, gomock.Any()).Return(nil, timeout)

	err := New(WithCommand(mc)).Connect(context.Background(), testNQN, "10.0.0.1")

	assert.True(t, tridentErrors.IsTimeoutError(err))
	var target *Error
	assert.False(t, errors.As(err, &target))
}

// pathErrFs fails opening one specific path, to model a sysfs directory that vanishes or becomes unreadable
// after its parent was listed.
type pathErrFs struct {
	afero.Fs
	path string
	err  error
}

func (f pathErrFs) Open(name string) (afero.File, error) {
	if name == f.path {
		return nil, f.err
	}
	return f.Fs.Open(name)
}

func TestSubsystem_SubsystemDirectoryUnreadable(t *testing.T) {
	const dir = "/sys/class/nvme-subsystem/nvme-subsys0"
	fs := pathErrFs{Fs: newFs(t, fsFile{dir + "/subsysnqn", testNQN}), path: dir, err: os.ErrPermission}

	sub, err := New(WithFs(fs)).Subsystem(context.Background(), testNQN)

	assert.Nil(t, sub)
	assert.ErrorIs(t, err, os.ErrPermission)
	assert.False(t, IsSubsystemNotFoundError(err))
}

func TestSubsystem_Layouts(t *testing.T) {
	const dir = "/sys/class/nvme-subsystem/nvme-subsys0"
	const otherUUID = "0a1b2c3d-1111-2222-3333-444455556666"

	cases := map[string]struct {
		files      []fsFile
		wantPaths  []string
		wantLive   bool
		wantNSName []string
		wantNSUUID []string
	}{
		"native_multipath_two_controllers": {
			files: append(append([]fsFile{
				{dir + "/subsysnqn", testNQN + "\n"},
				{dir + "/model", "NetApp ONTAP Controller\n"},
				{dir + "/iopolicy", "numa\n"},
				{dir + "/nvme0n1/uuid", testUUID + "\n"},
			}, ctrlFiles(dir, "nvme0", "live", "traddr=10.0.0.1,trsvcid=4420")...),
				ctrlFiles(dir, "nvme1", "live", "traddr=10.0.0.2,trsvcid=4420")...),
			wantPaths:  []string{"nvme0", "nvme1"},
			wantLive:   true,
			wantNSName: []string{"nvme0n1"},
			wantNSUUID: []string{testUUID},
		},
		"hidden_and_generic_devices_are_ignored": {
			files: append([]fsFile{
				{dir + "/subsysnqn", testNQN},
				{dir + "/nvme0n1/uuid", testUUID},
				{dir + "/nvme0c0n1/uuid", otherUUID},
				{dir + "/ng0n1/uuid", otherUUID},
				{dir + "/nvme0n1p1/uuid", otherUUID},
				{dir + "/nvme-subsys0/uuid", otherUUID},
				{dir + "/nvme0n/uuid", otherUUID},
				{dir + "/nvme/uuid", otherUUID},
			}, ctrlFiles(dir, "nvme0", "live", "traddr=10.0.0.1,trsvcid=4420")...),
			wantPaths:  []string{"nvme0"},
			wantLive:   true,
			wantNSName: []string{"nvme0n1"},
			wantNSUUID: []string{testUUID},
		},
		"several_namespaces": {
			files: append([]fsFile{
				{dir + "/subsysnqn", testNQN},
				{dir + "/nvme0n1/uuid", testUUID},
				{dir + "/nvme0n2/uuid", otherUUID},
				{dir + "/nvme0n10/uuid", "tenth"},
			}, ctrlFiles(dir, "nvme0", "live", "traddr=10.0.0.1,trsvcid=4420")...),
			wantPaths:  []string{"nvme0"},
			wantLive:   true,
			wantNSName: []string{"nvme0n1", "nvme0n10", "nvme0n2"},
			wantNSUUID: []string{testUUID, "tenth", otherUUID},
		},
		"no_live_path_namespace_still_listed": {
			files: append(append([]fsFile{
				{dir + "/subsysnqn", testNQN},
				{dir + "/nvme0n1/uuid", testUUID},
			}, ctrlFiles(dir, "nvme0", "connecting", "traddr=10.0.0.1,trsvcid=4420")...),
				ctrlFiles(dir, "nvme1", "deleting", "traddr=10.0.0.2,trsvcid=4420")...),
			wantPaths:  []string{"nvme0", "nvme1"},
			wantLive:   false,
			wantNSName: []string{"nvme0n1"},
			wantNSUUID: []string{testUUID},
		},
		"multipath_off_has_no_head_in_subsystem_dir": {
			files: append([]fsFile{
				{dir + "/subsysnqn", testNQN},
			}, ctrlFiles(dir, "nvme0", "live", "traddr=10.0.0.1,trsvcid=4420")...),
			wantPaths: []string{"nvme0"},
			wantLive:  true,
		},
		"values_are_trimmed": {
			files: append([]fsFile{
				{dir + "/subsysnqn", "  " + testNQN + " \n\n"},
				{dir + "/nvme0n1/uuid", " " + testUUID + "\t\n"},
			}, ctrlFiles(dir, "nvme0", " live ", "  traddr=10.0.0.1,trsvcid=4420  ")...),
			wantPaths:  []string{"nvme0"},
			wantLive:   true,
			wantNSName: []string{"nvme0n1"},
			wantNSUUID: []string{testUUID},
		},
		"controller_numbers_above_nine": {
			files: append(append([]fsFile{
				{dir + "/subsysnqn", testNQN},
			}, ctrlFiles(dir, "nvme10", "live", "traddr=10.0.0.1,trsvcid=4420")...),
				ctrlFiles(dir, "nvme2", "live", "traddr=10.0.0.2,trsvcid=4420")...),
			wantPaths: []string{"nvme10", "nvme2"},
			wantLive:  true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			sub, err := New(WithFs(newFs(t, tc.files...))).Subsystem(context.Background(), testNQN)

			assert.NoError(t, err)
			var paths, nsNames, nsUUIDs []string
			for _, p := range sub.Paths() {
				paths = append(paths, p.Name())
			}
			for _, ns := range sub.Namespaces() {
				nsNames = append(nsNames, ns.Name())
				nsUUIDs = append(nsUUIDs, ns.UUID())
			}
			assert.Equal(t, tc.wantPaths, paths)
			assert.Equal(t, tc.wantLive, sub.HasLivePath())
			assert.Equal(t, tc.wantNSName, nsNames)
			assert.Equal(t, tc.wantNSUUID, nsUUIDs)
		})
	}
}

func TestSubsystem_PathFields(t *testing.T) {
	const dir = "/sys/class/nvme-subsystem/nvme-subsys0"
	files := []fsFile{{dir + "/subsysnqn", testNQN}}
	files = append(files, ctrlFiles(dir, "nvme0", "live", "traddr=fd20::1,trsvcid=4420,src_addr=fd20::2")...)

	sub, err := New(WithFs(newFs(t, files...))).Subsystem(context.Background(), testNQN)

	assert.NoError(t, err)
	assert.Len(t, sub.Paths(), 1)
	p := sub.Paths()[0]
	assert.Equal(t, "nvme0", p.Name())
	assert.Equal(t, "fd20::1", p.Address())
	assert.Equal(t, "4420", p.Port())
	assert.Equal(t, TransportTCP, p.Transport())
	assert.Equal(t, PathStateLive, p.State())
}

func TestSubsystem_ReturnsSnapshot(t *testing.T) {
	const dir = "/sys/class/nvme-subsystem/nvme-subsys0"
	files := []fsFile{{dir + "/subsysnqn", testNQN}}
	files = append(files, ctrlFiles(dir, "nvme0", "live", "traddr=10.0.0.1,trsvcid=4420")...)
	fs := newFs(t, files...)
	ad := New(WithFs(fs))

	first, err := ad.Subsystem(context.Background(), testNQN)
	assert.NoError(t, err)
	assert.NoError(t, afero.WriteFile(fs, dir+"/nvme0/state", []byte("connecting\n"), 0o644))
	second, err := ad.Subsystem(context.Background(), testNQN)
	assert.NoError(t, err)

	assert.True(t, first.HasLivePath())
	assert.False(t, second.HasLivePath())
}

func TestSubsystem_MatchesWholeNQN(t *testing.T) {
	const dir = "/sys/class/nvme-subsystem/nvme-subsys0"
	files := []fsFile{{dir + "/subsysnqn", testNQN + ":suffix"}}
	files = append(files, ctrlFiles(dir, "nvme0", "live", "traddr=10.0.0.1,trsvcid=4420")...)

	sub, err := New(WithFs(newFs(t, files...))).Subsystem(context.Background(), testNQN)

	assert.Nil(t, sub)
	assert.True(t, IsSubsystemNotFoundError(err))
}

func TestSubsystem_EmptyDirectory(t *testing.T) {
	fs := afero.NewMemMapFs()
	assert.NoError(t, fs.MkdirAll("/sys/class/nvme-subsystem", 0o755))

	sub, err := New(WithFs(fs)).Subsystem(context.Background(), testNQN)

	assert.Nil(t, sub)
	assert.True(t, IsSubsystemNotFoundError(err))
}

func TestTransportLoaded_CommandFailures(t *testing.T) {
	cases := map[string]error{
		"lsmod_timeout":   tridentErrors.TimeoutError("process killed after timeout"),
		"lsmod_exit":      &fakeExitError{code: 1, msg: "exit status 1"},
		"lsmod_not_found": errors.New("executable file not found in $PATH"),
	}
	for name, lsmodErr := range cases {
		t.Run(name, func(t *testing.T) {
			mc := mockexec.NewMockCommand(gomock.NewController(t))
			mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "lsmod", execTimeout, false).Return(nil, lsmodErr)

			got, err := (&adapter{cmd: mc, fs: afero.NewMemMapFs()}).TransportLoaded(context.Background(), TransportTCP)

			assert.False(t, got)
			assert.ErrorIs(t, err, lsmodErr)
		})
	}
}

func TestTransportLoaded_SysfsPresentSkipsLsmodForEveryTransport(t *testing.T) {
	for _, tr := range []Transport{TransportTCP, testOtherTransport} {
		t.Run(string(tr), func(t *testing.T) {
			fs := afero.NewMemMapFs()
			assert.NoError(t, fs.MkdirAll("/sys/module/nvme_"+string(tr), 0o755))
			ad := &adapter{cmd: mockexec.NewMockCommand(gomock.NewController(t)), fs: fs}

			got, err := ad.TransportLoaded(context.Background(), tr)

			assert.NoError(t, err)
			assert.True(t, got)
		})
	}
}

func TestTransportLoaded_LsmodParsing(t *testing.T) {
	cases := map[string]struct {
		out  string
		want bool
	}{
		"empty_output":        {"", false},
		"header_only":         {"Module Size Used by\n", false},
		"module_listed":       {"Module Size Used by\nnvme_tcp 57344 0\n", true},
		"module_not_first":    {"Module Size Used by\nnvme_core 1 2\nnvme_tcp 57344 0\n", true},
		"blank_lines":         {"\n\nnvme_tcp 57344 0\n\n", true},
		"dependent_only":      {"nvme_fabrics 36864 1 nvme_tcp\n", false},
		"target_side_module":  {"nvmet_tcp 57344 0\n", false},
		"prefix_of_module":    {"nvme_tcp_extra 57344 0\n", false},
		"no_trailing_newline": {"nvme_tcp 57344 0", true},
		"tab_separated":       {"nvme_tcp\t57344\t0\n", true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			mc := mockexec.NewMockCommand(gomock.NewController(t))
			mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "lsmod", execTimeout, false).Return([]byte(tc.out), nil)

			got, err := (&adapter{cmd: mc, fs: afero.NewMemMapFs()}).TransportLoaded(context.Background(), TransportTCP)

			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestVersion_OutputIsTrimmed(t *testing.T) {
	mc := mockexec.NewMockCommand(gomock.NewController(t))
	mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", execTimeout, false, "version").
		Return([]byte("\n  nvme version 2.8 (git 2.8)  \n\n"), nil)

	got, err := New(WithCommand(mc)).Version(context.Background())

	assert.NoError(t, err)
	assert.Equal(t, "nvme version 2.8 (git 2.8)", got)
}

func TestVersion_TimeoutIsClassifiable(t *testing.T) {
	mc := mockexec.NewMockCommand(gomock.NewController(t))
	mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", execTimeout, false, "version").
		Return(nil, tridentErrors.TimeoutError("process killed after timeout"))

	_, err := New(WithCommand(mc)).Version(context.Background())

	assert.True(t, tridentErrors.IsTimeoutError(err))
}

func TestHostNQN_CommandFailures(t *testing.T) {
	cases := map[string]error{
		"timeout":   tridentErrors.TimeoutError("process killed after timeout"),
		"exit":      &fakeExitError{code: 1, msg: "cat: /etc/nvme/hostnqn: No such file or directory"},
		"not_found": errors.New("executable file not found in $PATH"),
	}
	for name, catErr := range cases {
		t.Run(name, func(t *testing.T) {
			mc := mockexec.NewMockCommand(gomock.NewController(t))
			mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "cat", execTimeout, false, "/etc/nvme/hostnqn").
				Return(nil, catErr)

			got, err := New(WithCommand(mc)).HostNQN(context.Background())

			assert.Equal(t, "", got)
			assert.ErrorIs(t, err, catErr)
		})
	}
}

func TestCommands_NotExecutable(t *testing.T) {
	notFound := errors.New("executable file not found in $PATH")
	ctx := context.Background()

	cases := map[string]struct {
		call func(Adapter) error
		args []any
	}{
		"connect": {
			call: func(a Adapter) error { return a.Connect(ctx, testNQN, "10.0.0.1") },
			args: []any{"connect", "-t", "tcp", "-n", testNQN, "-a", "10.0.0.1", "-s", "4420", "-l", "-1"},
		},
		"disconnect": {
			call: func(a Adapter) error { return a.Disconnect(ctx, testNQN) },
			args: []any{"disconnect", "-n", testNQN},
		},
		"flush": {
			call: func(a Adapter) error { return a.Flush(ctx, "/dev/nvme0n1") },
			args: []any{"flush", "/dev/nvme0n1"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			mc := mockexec.NewMockCommand(gomock.NewController(t))
			mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", gomock.Any(), true, tc.args...).Return(nil, notFound)

			err := tc.call(New(WithCommand(mc)))

			assert.ErrorIs(t, err, notFound)
			var target *Error
			assert.False(t, errors.As(err, &target))
		})
	}
}

func TestCommands_ExpiredDeadlineIsPassedThrough(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	expired := gomock.Cond(func(d time.Duration) bool { return d <= 0 })
	mc := mockexec.NewMockCommand(gomock.NewController(t))
	mc.EXPECT().ExecuteWithTimeout(gomock.Any(), "nvme", expired, true, "flush", "/dev/nvme0n1").
		Return(nil, tridentErrors.TimeoutError("process killed after timeout"))

	err := New(WithCommand(mc)).Flush(ctx, "/dev/nvme0n1")

	assert.True(t, tridentErrors.IsTimeoutError(err))
}

func TestParseExecError_Classification(t *testing.T) {
	t.Run("timeout_is_unwrapped", func(t *testing.T) {
		timeout := tridentErrors.TimeoutError("process killed after timeout")

		err := parseExecError("flush", []byte("ignored"), timeout)

		assert.Equal(t, timeout, err)
	})

	t.Run("wrapped_exit_error_keeps_output", func(t *testing.T) {
		wrapped := fmt.Errorf("outer: %w", &fakeExitError{code: 114, msg: "exit status 114"})

		err := parseExecError("connect", []byte("already connected"), wrapped)

		var target *Error
		assert.ErrorAs(t, err, &target)
		assert.Equal(t, 114, target.ExitCode())
		assert.Equal(t, "already connected", target.Output())
	})

	t.Run("other_error_names_the_command", func(t *testing.T) {
		err := parseExecError("flush", nil, errors.New("boom"))

		assert.EqualError(t, err, "nvme flush failed: boom")
	})
}

func TestSubsystem_SkipsSubsystemWithUnreadableNQNBeforeMatch(t *testing.T) {
	const bad = "/sys/class/nvme-subsystem/nvme-subsys0"
	const good = "/sys/class/nvme-subsystem/nvme-subsys1"
	files := []fsFile{{bad + "/model", "no subsysnqn here"}, {good + "/subsysnqn", testNQN}}
	files = append(files, ctrlFiles(good, "nvme1", "live", "traddr=10.0.0.2,trsvcid=4420")...)

	sub, err := New(WithFs(newFs(t, files...))).Subsystem(context.Background(), testNQN)

	assert.NoError(t, err)
	assert.Equal(t, good, sub.Dir())
}
