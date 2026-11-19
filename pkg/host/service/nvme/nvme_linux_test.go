// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/pkg/host/adapter/block"
	"github.com/netapp/trident/pkg/host/adapter/nvme"
	"github.com/netapp/trident/utils/errors"
	tridentexec "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/models"
)

const (
	testNQN    = "nqn.1992-08.com.netapp:sn.abc:subsystem.sub1"
	testUUID   = "a1b2c3d4-0000-4000-8000-000000000001"
	otherUUID  = "a1b2c3d4-0000-4000-8000-000000000002"
	testIP1    = "10.0.0.1"
	testIP2    = "10.0.0.2"
	subsysDir  = "/sys/class/nvme-subsystem/nvme-subsys0"
	sectorSize = 512
)

// fakeHost is an NVMe host. It is the command runner of the nvme adapter and it owns the sysfs tree
// that the nvme and block adapters read, so nvme-cli commands change what the adapters see.
// The service under test runs against the real adapters; only the host is simulated.
type fakeHost struct {
	tridentexec.Command // Anything the adapters should not run panics.

	fs afero.Fs

	mu          sync.Mutex
	calls       []string
	controllers int
	namespaces  []string // Block device names the host exposes, removed again by a disconnect.

	// Behavior hooks. A nil hook is the successful default.
	versionErr error
	lsmodOut   string
	hostNQN    string
	connect    func(ctx context.Context, ip string) error // Replaces the whole connect command.
	flush      func(ctx context.Context, dev string) error
	disconnect func(ctx context.Context) error
	// disconnectPath runs before a path disconnect (nvme disconnect -d) and may fail it.
	disconnectPath func(ctx context.Context, controller string) error
}

func newFakeHost() *fakeHost {
	return &fakeHost{fs: afero.NewMemMapFs(), hostNQN: "nqn.2014-08.org.nvmexpress:uuid:host1"}
}

// ExecuteWithTimeout implements the one command runner method the nvme adapter uses.
func (h *fakeHost) ExecuteWithTimeout(
	ctx context.Context, name string, _ time.Duration, _ bool, args ...string,
) ([]byte, error) {
	switch name {
	case "cat":
		return []byte(h.hostNQN + "\n"), nil
	case "lsmod":
		return []byte(h.lsmodOut), nil
	case "nvme":
		return h.nvmeCommand(ctx, args)
	}
	return nil, fmt.Errorf("unexpected command %s %v", name, args)
}

func (h *fakeHost) nvmeCommand(ctx context.Context, args []string) ([]byte, error) {
	h.record(strings.Join(args, " "))

	switch args[0] {
	case "version":
		return []byte("nvme version 2.8\n"), h.versionErr
	case "connect":
		ip := argAfter(args, "-a")
		if h.connect != nil {
			return nil, h.connect(ctx, ip)
		}
		h.addController(ip, "live")
		return nil, nil
	case "disconnect":
		if controller := argAfter(args, "-d"); controller != "" {
			if h.disconnectPath != nil {
				if err := h.disconnectPath(ctx, controller); err != nil {
					return nil, err
				}
			}
			h.removeController(controller)
			return nil, nil
		}
		if h.disconnect != nil {
			if err := h.disconnect(ctx); err != nil {
				return nil, err
			}
		}
		h.removeSubsystem()
		return nil, nil
	case "flush":
		if h.flush != nil {
			return nil, h.flush(ctx, args[1])
		}
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected nvme command %v", args)
}

func argAfter(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func (h *fakeHost) record(call string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, call)
}

// commands returns the nvme commands that were run, as "<subcommand> <args>".
func (h *fakeHost) commands() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.calls...)
}

// ran returns the commands that start with the subcommand.
func (h *fakeHost) ran(subcommand string) []string {
	var out []string
	for _, c := range h.commands() {
		if strings.HasPrefix(c, subcommand+" ") || c == subcommand {
			out = append(out, c)
		}
	}
	return out
}

func (h *fakeHost) write(path, content string) {
	if err := afero.WriteFile(h.fs, path, []byte(content+"\n"), 0o644); err != nil {
		panic(err)
	}
}

// addController adds a path (controller) to the subsystem.
func (h *fakeHost) addController(ip, state string) {
	h.mu.Lock()
	name := fmt.Sprintf("nvme%d", h.controllers)
	h.controllers++
	h.mu.Unlock()

	h.write(subsysDir+"/subsysnqn", testNQN)
	h.write(subsysDir+"/"+name+"/state", state)
	h.write(subsysDir+"/"+name+"/transport", "tcp")
	h.write(subsysDir+"/"+name+"/address", "traddr="+ip+",trsvcid=4420")
}

// setState changes the state of the controller that serves the IP.
func (h *fakeHost) setState(name, state string) {
	h.write(subsysDir+"/"+name+"/state", state)
}

// addNamespace exposes a namespace head device. An empty uuid leaves the namespace's uuid unreadable.
// The size is in bytes.
func (h *fakeHost) addNamespace(name, uuid string, size uint64) {
	h.mu.Lock()
	h.namespaces = append(h.namespaces, name)
	h.mu.Unlock()

	if uuid != "" {
		h.write(subsysDir+"/"+name+"/uuid", uuid)
	} else {
		_ = h.fs.MkdirAll(subsysDir+"/"+name, 0o755)
	}
	h.setSize(name, size)
}

func (h *fakeHost) setSize(name string, size uint64) {
	h.write("/sys/block/"+name+"/size", fmt.Sprint(size/sectorSize))
}

// removeController is what a path disconnect does: only that path is gone.
func (h *fakeHost) removeController(name string) {
	_ = h.fs.RemoveAll(subsysDir + "/" + name)
}

// removeSubsystem is what a whole-subsystem disconnect does: every path and namespace is gone.
func (h *fakeHost) removeSubsystem() {
	h.mu.Lock()
	defer h.mu.Unlock()
	_ = h.fs.RemoveAll(subsysDir)
	for _, ns := range h.namespaces {
		_ = h.fs.RemoveAll("/sys/block/" + ns)
	}
	h.namespaces = nil
}

// removeBlockDevice removes only the block device, leaving the subsystem's namespace entry in place.
func (h *fakeHost) removeBlockDevice(name string) {
	_ = h.fs.RemoveAll("/sys/block/" + name)
}

// connectedHost is a host with a subsystem that has live paths to both IPs and one namespace.
func connectedHost() *fakeHost {
	h := newFakeHost()
	h.addController(testIP1, "live")
	h.addController(testIP2, "live")
	h.addNamespace("nvme0n1", testUUID, 1<<30)
	return h
}

// newTestService returns the service on the host's adapters. Poll intervals are short.
func newTestService(h *fakeHost) *service {
	s := New(
		WithNVMeAdapter(nvme.New(nvme.WithCommand(h), nvme.WithFs(h.fs))),
		WithBlockAdapter(block.New(block.WithFs(h.fs))),
	).(*service)
	s.connectInterval = 2 * time.Millisecond
	s.ensureDeviceInterval = 2 * time.Millisecond
	s.ensureSizeInterval = time.Millisecond
	return s
}

func withTimeout(t *testing.T, d time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

// failFs fails opening any path under a prefix.
type failFs struct {
	afero.Fs
	prefix string
	err    error
}

func (f failFs) Open(name string) (afero.File, error) {
	if strings.HasPrefix(name, f.prefix) {
		return nil, f.err
	}
	return f.Fs.Open(name)
}

func TestPreChecks(t *testing.T) {
	cases := map[string]struct {
		setup   func(h *fakeHost)
		wantErr string
	}{
		"transport built in": {
			setup: func(h *fakeHost) { _ = h.fs.MkdirAll("/sys/module/nvme_tcp", 0o755) },
		},
		"transport loaded module": {
			setup: func(h *fakeHost) { h.lsmodOut = "Module Size Used by\nnvme_tcp 49152 0\n" },
		},
		"nvme cli missing": {
			setup:   func(h *fakeHost) { h.versionErr = fmt.Errorf("executable file not found in $PATH") },
			wantErr: "nvme-cli version: ",
		},
		"transport absent": {
			setup:   func(h *fakeHost) { h.lsmodOut = "Module Size Used by\nnvme_fcloop 16384 0\n" },
			wantErr: "NVMe driver is not loaded on the host",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newFakeHost()
			tc.setup(h)

			err := newTestService(h).PreChecks(context.Background())
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Equal(t, name == "transport absent", errors.Is(err, ErrTransportNotLoaded))
		})
	}

	t.Run("lsmod fails", func(t *testing.T) {
		// The adapter reports the exec failure; the service wraps it as NVMe driver info.
		s := New(WithNVMeAdapter(stubTransport{err: fmt.Errorf("lsmod failed")})).(*service)

		err := s.PreChecks(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "NVMe driver info: ")
		assert.Contains(t, err.Error(), "lsmod failed")
	})
}

// stubTransport is an nvme adapter whose nvme-cli works and whose transport check fails.
type stubTransport struct {
	nvme.Adapter
	err error
}

func (s stubTransport) Version(context.Context) (string, error) { return "nvme version 2.8", nil }
func (s stubTransport) TransportLoaded(context.Context, nvme.Transport) (bool, error) {
	return false, s.err
}

func TestHostNQN(t *testing.T) {
	h := newFakeHost()
	nqn, err := newTestService(h).HostNQN(context.Background())
	require.NoError(t, err)
	assert.Equal(t, h.hostNQN, nqn)

	s := New(WithNVMeAdapter(stubHostNQN{err: fmt.Errorf("no such file")}))
	_, err = s.HostNQN(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get hostnqn; ")
}

type stubHostNQN struct {
	nvme.Adapter
	err error
}

func (s stubHostNQN) HostNQN(context.Context) (string, error) { return "", s.err }

func TestDeviceByUUID(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		dev, err := newTestService(connectedHost()).DeviceByUUID(context.Background(), testNQN, testUUID)

		require.NoError(t, err)
		assert.Equal(t, testUUID, dev.ID())
		assert.Equal(t, "nvme0n1", dev.Name())
		assert.Equal(t, "/dev/nvme0n1", dev.Path())
		assert.Equal(t, uint64(1<<30), dev.Size())
		assert.Equal(t, testNQN, dev.SubsystemNQN())
		assert.Equal(t, models.BlockDeviceDisk, dev.Type())
		assert.Equal(t, models.BlockDeviceInfo{ID: testUUID, Name: "nvme0n1", Type: models.BlockDeviceDisk}, dev.Info())
		assert.Contains(t, dev.String(), testUUID)
	})

	t.Run("uuid case is ignored", func(t *testing.T) {
		h := connectedHost()
		dev, err := newTestService(h).DeviceByUUID(context.Background(), testNQN, strings.ToUpper(testUUID))

		require.NoError(t, err)
		assert.Equal(t, testUUID, dev.ID())
	})

	t.Run("finds the namespace among several", func(t *testing.T) {
		h := connectedHost()
		h.addNamespace("nvme0n2", otherUUID, 2<<30)

		dev, err := newTestService(h).DeviceByUUID(context.Background(), testNQN, otherUUID)
		require.NoError(t, err)
		assert.Equal(t, "nvme0n2", dev.Name())
	})

	t.Run("no subsystem", func(t *testing.T) {
		_, err := newTestService(newFakeHost()).DeviceByUUID(context.Background(), testNQN, testUUID)
		assert.True(t, errors.IsNotFoundError(err), "got %v", err)
	})

	t.Run("no such namespace", func(t *testing.T) {
		_, err := newTestService(connectedHost()).DeviceByUUID(context.Background(), testNQN, otherUUID)
		require.Error(t, err)
		assert.True(t, errors.IsNotFoundError(err))
		assert.Contains(t, err.Error(), "no device found for the given namespace "+otherUUID)
	})

	t.Run("namespace whose uuid is unreadable never matches", func(t *testing.T) {
		h := newFakeHost()
		h.addController(testIP1, "live")
		h.addNamespace("nvme0n1", "", 1<<30)

		_, err := newTestService(h).DeviceByUUID(context.Background(), testNQN, "")
		assert.True(t, errors.IsNotFoundError(err))
	})

	t.Run("block device is gone", func(t *testing.T) {
		h := connectedHost()
		h.removeBlockDevice("nvme0n1")

		_, err := newTestService(h).DeviceByUUID(context.Background(), testNQN, testUUID)
		assert.True(t, errors.IsNotFoundError(err))
	})

	t.Run("sysfs cannot be read", func(t *testing.T) {
		h := connectedHost()
		boom := fmt.Errorf("input/output error")
		s := New(
			WithNVMeAdapter(nvme.New(nvme.WithCommand(h), nvme.WithFs(failFs{h.fs, "/sys/class", boom}))),
			WithBlockAdapter(block.New(block.WithFs(h.fs))),
		)

		_, err := s.DeviceByUUID(context.Background(), testNQN, testUUID)
		require.Error(t, err)
		assert.False(t, errors.IsNotFoundError(err))
		assert.ErrorIs(t, err, boom)
	})
}

func TestEnsureConnections_InvalidArguments(t *testing.T) {
	s := newTestService(newFakeHost())

	err := s.EnsureConnections(context.Background(), "", []string{testIP1})
	assert.ErrorContains(t, err, "empty subsystem NQN")

	err = s.EnsureConnections(context.Background(), testNQN, nil)
	assert.ErrorContains(t, err, "no target IPs")
}

func TestEnsureConnections(t *testing.T) {
	t.Run("already connected connects nothing", func(t *testing.T) {
		h := connectedHost()

		err := newTestService(h).EnsureConnections(context.Background(), testNQN, []string{testIP1, testIP2})

		require.NoError(t, err)
		assert.Empty(t, h.ran("connect"))
	})

	t.Run("connects only the missing paths", func(t *testing.T) {
		h := newFakeHost()
		h.addController(testIP1, "live")

		err := newTestService(h).EnsureConnections(context.Background(), testNQN, []string{testIP1, testIP2})

		require.NoError(t, err)
		assert.Equal(t, []string{"connect -t tcp -n " + testNQN + " -a " + testIP2 + " -s 4420 -l -1"},
			h.ran("connect"))
	})

	t.Run("a connecting path is left to the kernel", func(t *testing.T) {
		h := newFakeHost()
		h.addController(testIP1, "live")
		h.addController(testIP2, "connecting")

		err := newTestService(h).EnsureConnections(context.Background(), testNQN, []string{testIP1, testIP2})

		require.NoError(t, err)
		assert.Empty(t, h.ran("connect"))
	})

	t.Run("connects every path of a new subsystem", func(t *testing.T) {
		h := newFakeHost()

		err := newTestService(h).EnsureConnections(context.Background(), testNQN, []string{testIP1, testIP2})

		require.NoError(t, err)
		assert.Len(t, h.ran("connect"), 2)
	})

	t.Run("one connect failing is tolerated", func(t *testing.T) {
		h := newFakeHost()
		h.connect = func(_ context.Context, ip string) error {
			if ip == testIP2 {
				return fmt.Errorf("connection refused")
			}
			h.addController(ip, "live")
			return nil
		}

		err := newTestService(h).EnsureConnections(context.Background(), testNQN, []string{testIP1, testIP2})

		require.NoError(t, err)
		assert.Len(t, h.ran("connect"), 2, "the failing IP must not stop the other from being connected")
	})

	t.Run("every connect failing is tolerated when a desired path already exists", func(t *testing.T) {
		h := newFakeHost()
		h.addController(testIP1, "live")
		h.connect = func(context.Context, string) error { return fmt.Errorf("connection refused") }

		err := newTestService(h).EnsureConnections(context.Background(), testNQN, []string{testIP1, testIP2})

		require.NoError(t, err)
	})

	t.Run("a path to an IP nobody asked for does not count", func(t *testing.T) {
		h := newFakeHost()
		h.addController("10.9.9.9", "live")
		h.connect = func(context.Context, string) error { return fmt.Errorf("connection refused") }

		err := newTestService(h).EnsureConnections(
			withTimeout(t, 50*time.Millisecond), testNQN, []string{testIP1, testIP2})

		assert.ErrorContains(t, err, "connection refused")
	})

	t.Run("every connect failing with no path fails with the connect errors", func(t *testing.T) {
		h := newFakeHost()
		h.connect = func(context.Context, string) error { return fmt.Errorf("connection refused") }

		err := newTestService(h).EnsureConnections(
			withTimeout(t, 100*time.Millisecond), testNQN, []string{testIP1, testIP2})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to connect subsystem "+testNQN+" to "+testIP1)
		assert.Contains(t, err.Error(), "failed to connect subsystem "+testNQN+" to "+testIP2)
		assert.Greater(t, len(h.ran("connect")), 2, "a failed attempt must be retried")
	})

	t.Run("a connect that succeeds on a later attempt", func(t *testing.T) {
		h := newFakeHost()
		var attempts int
		h.connect = func(_ context.Context, ip string) error {
			attempts++
			if attempts < 3 {
				return fmt.Errorf("connection refused")
			}
			h.addController(ip, "live")
			return nil
		}

		err := newTestService(h).EnsureConnections(withTimeout(t, 5*time.Second), testNQN, []string{testIP1})

		require.NoError(t, err)
		assert.Len(t, h.ran("connect"), 3)
	})

	t.Run("a hung connect is killed when the context ends", func(t *testing.T) {
		h := newFakeHost()
		h.connect = func(ctx context.Context, _ string) error {
			<-ctx.Done()
			return errors.TimeoutError("command timed out")
		}

		start := time.Now()
		err := newTestService(h).EnsureConnections(withTimeout(t, 50*time.Millisecond), testNQN, []string{testIP1})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "command timed out")
		assert.Less(t, time.Since(start), 2*time.Second)
	})

	t.Run("a subsystem that cannot be read is treated as having no paths", func(t *testing.T) {
		h := connectedHost()
		boom := fmt.Errorf("input/output error")
		s := New(
			WithNVMeAdapter(nvme.New(nvme.WithCommand(h), nvme.WithFs(failFs{h.fs, "/sys/class", boom}))),
			WithBlockAdapter(block.New(block.WithFs(h.fs))),
		).(*service)
		s.connectInterval = time.Millisecond

		err := s.EnsureConnections(context.Background(), testNQN, []string{testIP1})

		require.NoError(t, err)
		assert.NotEmpty(t, h.ran("connect"), "the connect must still be tried")
	})

	t.Run("a context without a deadline is bounded to the connect timeout", func(t *testing.T) {
		h := newFakeHost()
		var deadline time.Time
		h.connect = func(ctx context.Context, ip string) error {
			deadline, _ = ctx.Deadline()
			h.addController(ip, "live")
			return nil
		}

		require.NoError(t, newTestService(h).EnsureConnections(context.Background(), testNQN, []string{testIP1}))
		assert.WithinDuration(t, time.Now().Add(ensureConnectionsTimeout), deadline, 2*time.Second)
	})

	t.Run("the caller's shorter deadline wins", func(t *testing.T) {
		h := newFakeHost()
		var deadline time.Time
		h.connect = func(ctx context.Context, ip string) error {
			deadline, _ = ctx.Deadline()
			h.addController(ip, "live")
			return nil
		}

		err := newTestService(h).EnsureConnections(withTimeout(t, time.Second), testNQN, []string{testIP1})

		require.NoError(t, err)
		assert.WithinDuration(t, time.Now().Add(time.Second), deadline, time.Second)
	})
}

func TestEnsureDevice_InvalidArguments(t *testing.T) {
	s := newTestService(newFakeHost())

	_, err := s.EnsureDevice(context.Background(), "", testUUID)
	assert.ErrorContains(t, err, "empty subsystem NQN")

	_, err = s.EnsureDevice(context.Background(), testNQN, "")
	assert.ErrorContains(t, err, "empty namespace UUID")
}

func TestEnsureDevice(t *testing.T) {
	t.Run("a visible namespace is returned at once", func(t *testing.T) {
		h := connectedHost()

		dev, err := newTestService(h).EnsureDevice(context.Background(), testNQN, testUUID)

		require.NoError(t, err)
		assert.Equal(t, testUUID, dev.ID())
		assert.Equal(t, "nvme0n1", dev.Name())
		assert.Empty(t, h.commands(), "EnsureDevice must not run nvme-cli")
	})

	t.Run("a namespace that appears late is waited for", func(t *testing.T) {
		h := newFakeHost()
		h.addController(testIP1, "live")
		time.AfterFunc(30*time.Millisecond, func() { h.addNamespace("nvme0n1", testUUID, 1<<30) })

		dev, err := newTestService(h).EnsureDevice(withTimeout(t, 5*time.Second), testNQN, testUUID)

		require.NoError(t, err)
		assert.Equal(t, testUUID, dev.ID())
		assert.Empty(t, h.ran("connect"), "waiting for the namespace must not connect")
	})

	t.Run("a subsystem that appears late is waited for", func(t *testing.T) {
		h := newFakeHost()
		time.AfterFunc(30*time.Millisecond, func() {
			h.addController(testIP1, "live")
			h.addNamespace("nvme0n1", testUUID, 1<<30)
		})

		dev, err := newTestService(h).EnsureDevice(withTimeout(t, 5*time.Second), testNQN, testUUID)

		require.NoError(t, err)
		assert.Equal(t, testUUID, dev.ID())
	})

	t.Run("a namespace that never appears is not found", func(t *testing.T) {
		h := connectedHost()

		_, err := newTestService(h).EnsureDevice(withTimeout(t, 50*time.Millisecond), testNQN, otherUUID)

		require.Error(t, err)
		assert.True(t, errors.IsNotFoundError(err), "got %v", err)
		assert.Contains(t, err.Error(), "timed out waiting for NVMe namespace "+otherUUID)
	})

	t.Run("a subsystem that never appears is not found", func(t *testing.T) {
		_, err := newTestService(newFakeHost()).EnsureDevice(withTimeout(t, 30*time.Millisecond), testNQN, testUUID)

		assert.True(t, errors.IsNotFoundError(err), "got %v", err)
	})

	t.Run("a sysfs read error is retried until the context ends", func(t *testing.T) {
		h := connectedHost()
		boom := fmt.Errorf("input/output error")
		s := New(
			WithNVMeAdapter(nvme.New(nvme.WithCommand(h), nvme.WithFs(failFs{h.fs, "/sys/class", boom}))),
			WithBlockAdapter(block.New(block.WithFs(h.fs))),
		)

		_, err := s.EnsureDevice(withTimeout(t, 30*time.Millisecond), testNQN, testUUID)

		assert.ErrorIs(t, err, boom)
	})
}

func TestEnsureFlush(t *testing.T) {
	device := func(t *testing.T, s *service) *Device {
		t.Helper()
		dev, err := s.DeviceByUUID(context.Background(), testNQN, testUUID)
		require.NoError(t, err)
		return dev
	}

	t.Run("nil device", func(t *testing.T) {
		assert.NoError(t, newTestService(newFakeHost()).EnsureFlush(context.Background(), nil, FlushOptions{}))
	})

	t.Run("flushes the device while a path is live", func(t *testing.T) {
		h := connectedHost()
		s := newTestService(h)

		require.NoError(t, s.EnsureFlush(context.Background(), device(t, s), FlushOptions{}))
		assert.Equal(t, []string{"flush /dev/nvme0n1"}, h.ran("flush"))
	})

	t.Run("one live path is enough", func(t *testing.T) {
		h := connectedHost()
		h.setState("nvme0", "connecting")
		s := newTestService(h)

		require.NoError(t, s.EnsureFlush(context.Background(), device(t, s), FlushOptions{}))
		assert.Len(t, h.ran("flush"), 1)
	})

	t.Run("flush error is returned", func(t *testing.T) {
		h := connectedHost()
		h.flush = func(context.Context, string) error { return fmt.Errorf("io error") }
		s := newTestService(h)

		err := s.EnsureFlush(context.Background(), device(t, s), FlushOptions{})
		require.Error(t, err)
		assert.True(t, IsFlushError(err), "got %T", err)
		assert.Contains(t, err.Error(), "flush /dev/nvme0n1: ")
		assert.Contains(t, err.Error(), "io error")
		assert.NotNil(t, errors.Unwrap(err), "the adapter error is kept")
	})

	t.Run("flush error is ignored when errors are ignored", func(t *testing.T) {
		h := connectedHost()
		h.flush = func(context.Context, string) error { return fmt.Errorf("io error") }
		s := newTestService(h)

		require.NoError(t, s.EnsureFlush(context.Background(), device(t, s), FlushOptions{IgnoreErrors: true}))
		assert.Len(t, h.ran("flush"), 1, "the flush is still tried when a path is live")
	})

	t.Run("force flushes nothing", func(t *testing.T) {
		h := connectedHost()
		s := newTestService(h)

		require.NoError(t, s.EnsureFlush(context.Background(), device(t, s), FlushOptions{Force: true}))
		assert.Empty(t, h.ran("flush"))
	})

	t.Run("force wins over ignoring errors", func(t *testing.T) {
		h := connectedHost()
		s := newTestService(h)

		opts := FlushOptions{Force: true, IgnoreErrors: true}
		require.NoError(t, s.EnsureFlush(context.Background(), device(t, s), opts))
		assert.Empty(t, h.ran("flush"))
	})

	t.Run("no live path fails with a flush error and runs nothing", func(t *testing.T) {
		h := connectedHost()
		h.setState("nvme0", "connecting")
		h.setState("nvme1", "connecting")
		s := newTestService(h)

		err := s.EnsureFlush(context.Background(), device(t, s), FlushOptions{})
		require.Error(t, err)
		assert.True(t, IsFlushError(err), "got %T", err)
		assert.Equal(t, "NVMe subsystem "+testNQN+" has no live path; cannot flush device /dev/nvme0n1", err.Error())
		assert.Nil(t, errors.Unwrap(err))
		assert.Empty(t, h.ran("flush"))
	})

	t.Run("no live path with ignored errors skips the flush", func(t *testing.T) {
		h := connectedHost()
		h.setState("nvme0", "connecting")
		h.setState("nvme1", "deleting")
		s := newTestService(h)

		require.NoError(t, s.EnsureFlush(context.Background(), device(t, s), FlushOptions{IgnoreErrors: true}))
		assert.Empty(t, h.ran("flush"))
	})

	t.Run("a subsystem that is gone has no live path", func(t *testing.T) {
		h := connectedHost()
		s := newTestService(h)
		dev := device(t, s)
		_ = h.fs.RemoveAll(subsysDir)

		assert.True(t, IsFlushError(s.EnsureFlush(context.Background(), dev, FlushOptions{})))
		assert.NoError(t, s.EnsureFlush(context.Background(), dev, FlushOptions{IgnoreErrors: true}))
		assert.Empty(t, h.ran("flush"))
	})

	t.Run("a device that is already gone is not flushed", func(t *testing.T) {
		h := connectedHost()
		s := newTestService(h)
		dev := device(t, s)
		h.removeBlockDevice("nvme0n1")

		require.NoError(t, s.EnsureFlush(context.Background(), dev, FlushOptions{}))
		assert.Empty(t, h.ran("flush"))
	})

	t.Run("a hung flush is killed when the context ends", func(t *testing.T) {
		h := connectedHost()
		h.flush = func(ctx context.Context, _ string) error {
			<-ctx.Done()
			return errors.TimeoutError("command timed out")
		}
		s := newTestService(h)
		dev := device(t, s)

		start := time.Now()
		err := s.EnsureFlush(withTimeout(t, 50*time.Millisecond), dev, FlushOptions{})

		require.Error(t, err)
		assert.True(t, IsFlushError(err), "got %T", err)
		assert.Contains(t, err.Error(), "command timed out")
		assert.Less(t, time.Since(start), 2*time.Second)
	})

	t.Run("a fired fault point is returned before anything else", func(t *testing.T) {
		h := connectedHost()
		s := newTestService(h)
		dev := device(t, s)
		saved := beforeNVMeFlushDevice
		defer func() { beforeNVMeFlushDevice = saved }()
		beforeNVMeFlushDevice = failingInjector{}

		for name, opts := range map[string]FlushOptions{
			"default": {}, "force": {Force: true}, "ignore errors": {IgnoreErrors: true},
		} {
			assert.EqualError(t, s.EnsureFlush(context.Background(), dev, opts), "injected fault", name)
		}
		assert.EqualError(t, s.EnsureFlush(context.Background(), nil, FlushOptions{}), "injected fault")
		assert.Empty(t, h.ran("flush"))
	})

	t.Run("a context without a deadline is bounded", func(t *testing.T) {
		h := connectedHost()
		var deadline time.Time
		h.flush = func(ctx context.Context, _ string) error {
			deadline, _ = ctx.Deadline()
			return nil
		}
		s := newTestService(h)

		require.NoError(t, s.EnsureFlush(context.Background(), device(t, s), FlushOptions{}))
		assert.WithinDuration(t, time.Now().Add(flushTimeout), deadline, 2*time.Second)
	})
}

func TestEnsureSize(t *testing.T) {
	const target = 2 << 30

	device := func(t *testing.T, s *service) *Device {
		t.Helper()
		dev, err := s.DeviceByUUID(context.Background(), testNQN, testUUID)
		require.NoError(t, err)
		return dev
	}

	t.Run("invalid arguments", func(t *testing.T) {
		s := newTestService(connectedHost())
		assert.Error(t, s.EnsureSize(context.Background(), nil, target))
		assert.ErrorContains(t, s.EnsureSize(context.Background(), device(t, s), 0), "greater than 0")
	})

	t.Run("already at the size", func(t *testing.T) {
		h := connectedHost()
		h.setSize("nvme0n1", target)
		s := newTestService(h)

		assert.NoError(t, s.EnsureSize(withTimeout(t, time.Second), device(t, s), target))
	})

	t.Run("larger than the size", func(t *testing.T) {
		h := connectedHost()
		h.setSize("nvme0n1", target+1<<20)
		s := newTestService(h)

		assert.NoError(t, s.EnsureSize(withTimeout(t, time.Second), device(t, s), target))
	})

	t.Run("grows while waiting", func(t *testing.T) {
		h := connectedHost()
		s := newTestService(h)
		dev := device(t, s)
		time.AfterFunc(20*time.Millisecond, func() { h.setSize("nvme0n1", target) })

		assert.NoError(t, s.EnsureSize(withTimeout(t, 5*time.Second), dev, target))
	})

	t.Run("never reaches the size", func(t *testing.T) {
		s := newTestService(connectedHost())

		err := s.EnsureSize(withTimeout(t, 50*time.Millisecond), device(t, s), target)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "did not reach target size")
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("a device that goes away is waited for", func(t *testing.T) {
		h := connectedHost()
		h.setSize("nvme0n1", target)
		s := newTestService(h)
		dev := device(t, s)
		h.removeBlockDevice("nvme0n1")
		time.AfterFunc(20*time.Millisecond, func() { h.setSize("nvme0n1", target) })

		assert.NoError(t, s.EnsureSize(withTimeout(t, 5*time.Second), dev, target))
	})

	t.Run("a device that never returns fails", func(t *testing.T) {
		h := connectedHost()
		s := newTestService(h)
		dev := device(t, s)
		h.removeBlockDevice("nvme0n1")

		assert.Error(t, s.EnsureSize(withTimeout(t, 30*time.Millisecond), dev, target))
	})
}

func TestEnsureDisconnect(t *testing.T) {
	t.Run("invalid arguments", func(t *testing.T) {
		s := newTestService(newFakeHost())
		assert.ErrorContains(t, s.EnsureDisconnect(context.Background(), "", testUUID, false), "empty subsystem NQN")
		assert.ErrorContains(t, s.EnsureDisconnect(context.Background(), testNQN, "", false), "empty namespace UUID")
	})

	t.Run("the last namespace disconnects the subsystem", func(t *testing.T) {
		h := connectedHost()

		require.NoError(t, newTestService(h).EnsureDisconnect(context.Background(), testNQN, testUUID, false))
		assert.Equal(t, []string{"disconnect -n " + testNQN}, h.ran("disconnect"))
	})

	t.Run("the namespace uuid case is ignored", func(t *testing.T) {
		h := connectedHost()

		err := newTestService(h).EnsureDisconnect(context.Background(), testNQN, strings.ToUpper(testUUID), false)
		require.NoError(t, err)
		assert.Len(t, h.ran("disconnect"), 1)
	})

	t.Run("another namespace keeps the subsystem", func(t *testing.T) {
		h := connectedHost()
		h.addNamespace("nvme0n2", otherUUID, 1<<30)

		require.NoError(t, newTestService(h).EnsureDisconnect(context.Background(), testNQN, testUUID, false))
		assert.Empty(t, h.ran("disconnect"))
	})

	t.Run("a namespace with an unreadable uuid keeps the subsystem", func(t *testing.T) {
		h := connectedHost()
		h.addNamespace("nvme0n2", "", 1<<30)

		require.NoError(t, newTestService(h).EnsureDisconnect(context.Background(), testNQN, testUUID, false))
		assert.Empty(t, h.ran("disconnect"))
	})

	t.Run("without a live path nothing can be enumerated and the subsystem stays", func(t *testing.T) {
		h := connectedHost()
		h.setState("nvme0", "connecting")
		h.setState("nvme1", "connecting")

		require.NoError(t, newTestService(h).EnsureDisconnect(context.Background(), testNQN, testUUID, false))
		assert.Empty(t, h.ran("disconnect"))
	})

	t.Run("force disconnects when no path is live", func(t *testing.T) {
		h := connectedHost()
		h.setState("nvme0", "connecting")
		h.setState("nvme1", "connecting")

		require.NoError(t, newTestService(h).EnsureDisconnect(context.Background(), testNQN, testUUID, true))
		assert.Equal(t, []string{"disconnect -n " + testNQN}, h.ran("disconnect"))
	})

	t.Run("force does not override visible namespaces", func(t *testing.T) {
		h := connectedHost()
		h.addNamespace("nvme0n2", otherUUID, 1<<30)

		require.NoError(t, newTestService(h).EnsureDisconnect(context.Background(), testNQN, testUUID, true))
		assert.Empty(t, h.ran("disconnect"))
	})

	t.Run("a subsystem that is already gone is done", func(t *testing.T) {
		h := newFakeHost()

		require.NoError(t, newTestService(h).EnsureDisconnect(context.Background(), testNQN, testUUID, false))
		assert.Empty(t, h.ran("disconnect"))
	})

	t.Run("a sysfs read error is returned and nothing is disconnected", func(t *testing.T) {
		h := connectedHost()
		boom := fmt.Errorf("input/output error")
		s := New(WithNVMeAdapter(nvme.New(nvme.WithCommand(h), nvme.WithFs(failFs{h.fs, "/sys/class", boom}))))

		err := s.EnsureDisconnect(context.Background(), testNQN, testUUID, false)
		assert.ErrorIs(t, err, boom)
		assert.Empty(t, h.ran("disconnect"))
	})

	t.Run("a disconnect error is returned", func(t *testing.T) {
		h := connectedHost()
		h.disconnect = func(context.Context) error { return fmt.Errorf("device busy") }

		err := newTestService(h).EnsureDisconnect(context.Background(), testNQN, testUUID, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "disconnect subsystem "+testNQN+": ")
		assert.Contains(t, err.Error(), "device busy")
	})

	t.Run("the disconnect is killed when the context ends", func(t *testing.T) {
		h := connectedHost()
		h.disconnect = func(ctx context.Context) error {
			<-ctx.Done()
			return errors.TimeoutError("command timed out")
		}

		start := time.Now()
		err := newTestService(h).EnsureDisconnect(withTimeout(t, 50*time.Millisecond), testNQN, testUUID, false)

		require.Error(t, err)
		assert.Less(t, time.Since(start), 2*time.Second)
	})
}

// TestAttachAfterADisconnectRemovesTheSubsystem covers a disconnect racing an attach on the same subsystem.
// Volume B is detached while volume C is still attaching to the same subsystem, and the disconnect lands before
// C's namespace is visible. C's EnsureDevice times out with NotFound and the caller repeats the attach, which
// connects the paths again and succeeds.
func TestAttachAfterADisconnectRemovesTheSubsystem(t *testing.T) {
	h := newFakeHost()
	h.addController(testIP1, "live")
	h.addNamespace("nvme0n1", testUUID, 1<<30) // Volume B, being detached.

	var connects int
	h.connect = func(_ context.Context, ip string) error {
		connects++
		h.addController(ip, "live")
		if connects == 1 {
			// The disconnect for B runs right after C's first connect, before C's namespace is visible.
			return newTestService(h).EnsureDisconnect(context.Background(), testNQN, testUUID, false)
		}
		h.addNamespace("nvme0n2", otherUUID, 1<<30) // Volume C.
		return nil
	}
	s := newTestService(h)
	ips := []string{testIP1, testIP2}

	require.NoError(t, s.EnsureConnections(context.Background(), testNQN, ips))
	assert.Len(t, h.ran("disconnect"), 1, "the disconnect ran while C was attaching")

	_, err := s.EnsureDevice(withTimeout(t, 30*time.Millisecond), testNQN, otherUUID)
	assert.True(t, errors.IsNotFoundError(err), "the namespace was removed with the subsystem; got %v", err)

	// The caller repeats the attach.
	require.NoError(t, s.EnsureConnections(context.Background(), testNQN, ips))
	dev, err := s.EnsureDevice(context.Background(), testNQN, otherUUID)
	require.NoError(t, err)
	assert.Equal(t, otherUUID, dev.ID())
}

// TestDisconnectThenAttach is the other order: the disconnect finishes first and the attach reconnects.
func TestDisconnectThenAttach(t *testing.T) {
	h := connectedHost()
	h.connect = func(_ context.Context, ip string) error {
		h.addController(ip, "live")
		h.addNamespace("nvme0n1", otherUUID, 1<<30)
		return nil
	}
	s := newTestService(h)

	require.NoError(t, s.EnsureDisconnect(context.Background(), testNQN, testUUID, false))
	require.Len(t, h.ran("disconnect"), 1)

	require.NoError(t, s.EnsureConnections(context.Background(), testNQN, []string{testIP1}))
	dev, err := s.EnsureDevice(context.Background(), testNQN, otherUUID)
	require.NoError(t, err)
	assert.Equal(t, otherUUID, dev.ID())
}

// TestAttachesOnOneSubsystemRunInParallel checks that attaches on one subsystem do not serialize.
func TestAttachesOnOneSubsystemRunInParallel(t *testing.T) {
	h := newFakeHost()
	h.addController(testIP1, "live")
	h.addNamespace("nvme0n1", testUUID, 1<<30)
	h.addNamespace("nvme0n2", otherUUID, 1<<30)
	s := newTestService(h)

	// Both attaches need a connect, and each waits for the other to be inside it.
	var inside sync.WaitGroup
	inside.Add(2)
	h.connect = func(_ context.Context, ip string) error {
		inside.Done()
		inside.Wait()
		h.addController(ip, "live")
		return nil
	}

	var wg sync.WaitGroup
	for _, uuid := range []string{testUUID, otherUUID} {
		wg.Go(func() {
			ctx := withTimeout(t, 5*time.Second)
			assert.NoError(t, s.EnsureConnections(ctx, testNQN, []string{testIP2}))
			_, err := s.EnsureDevice(ctx, testNQN, uuid)
			assert.NoError(t, err)
		})
	}
	wg.Wait()
}

func TestSubsystem(t *testing.T) {
	t.Run("invalid arguments", func(t *testing.T) {
		_, err := newTestService(newFakeHost()).Subsystem(context.Background(), "")
		assert.ErrorContains(t, err, "empty subsystem NQN")
	})

	t.Run("the view of a connected subsystem", func(t *testing.T) {
		h := connectedHost()
		h.setState("nvme1", "connecting")
		h.addNamespace("nvme0n2", otherUUID, 1<<30)

		sub, err := newTestService(h).Subsystem(context.Background(), testNQN)

		require.NoError(t, err)
		assert.Equal(t, testNQN, sub.NQN())
		assert.Len(t, sub.Namespaces(), 2)
		assert.True(t, sub.HasLivePath())
		paths := sub.Paths()
		require.Len(t, paths, 2)
		assert.Equal(t, "nvme0", paths[0].Name())
		assert.Equal(t, testIP1, paths[0].Address())
		assert.Equal(t, nvme.PathStateLive, paths[0].State())
		assert.True(t, paths[0].IsLive())
		assert.Equal(t, "nvme1", paths[1].Name())
		assert.Equal(t, testIP2, paths[1].Address())
		assert.Equal(t, nvme.PathStateConnecting, paths[1].State())
		assert.False(t, paths[1].IsLive())
	})

	t.Run("no live path", func(t *testing.T) {
		h := connectedHost()
		h.setState("nvme0", "connecting")
		h.setState("nvme1", "deleting")

		sub, err := newTestService(h).Subsystem(context.Background(), testNQN)

		require.NoError(t, err)
		assert.False(t, sub.HasLivePath())
	})

	t.Run("a subsystem without namespaces", func(t *testing.T) {
		h := newFakeHost()
		h.addController(testIP1, "live")

		sub, err := newTestService(h).Subsystem(context.Background(), testNQN)

		require.NoError(t, err)
		assert.Empty(t, sub.Namespaces())
	})

	t.Run("the paths are a copy", func(t *testing.T) {
		sub, err := newTestService(connectedHost()).Subsystem(context.Background(), testNQN)
		require.NoError(t, err)

		sub.Paths()[0] = &Path{}

		assert.Equal(t, "nvme0", sub.Paths()[0].Name())
	})

	t.Run("a subsystem that is not on the host is not found", func(t *testing.T) {
		sub, err := newTestService(newFakeHost()).Subsystem(context.Background(), testNQN)

		assert.Nil(t, sub)
		assert.True(t, errors.IsNotFoundError(err), "got %v", err)
		assert.ErrorContains(t, err, "no subsystem paths found")
	})

	t.Run("a subsystem with no paths is not found", func(t *testing.T) {
		h := newFakeHost()
		h.write(subsysDir+"/subsysnqn", testNQN)
		h.addNamespace("nvme0n1", testUUID, 1<<30)

		sub, err := newTestService(h).Subsystem(context.Background(), testNQN)

		assert.Nil(t, sub)
		assert.True(t, errors.IsNotFoundError(err), "got %v", err)
	})

	t.Run("a sysfs read error is returned wrapped", func(t *testing.T) {
		h := connectedHost()
		boom := fmt.Errorf("input/output error")
		s := New(WithNVMeAdapter(nvme.New(nvme.WithCommand(h), nvme.WithFs(failFs{h.fs, "/sys/class", boom}))))

		sub, err := s.Subsystem(context.Background(), testNQN)

		assert.Nil(t, sub)
		assert.ErrorIs(t, err, boom)
		assert.False(t, errors.IsNotFoundError(err))
		assert.ErrorContains(t, err, "failed to read NVMe subsystem "+testNQN)
	})
}

func TestConnectPaths(t *testing.T) {
	connectTo := func(ip string) string {
		return "connect -t tcp -n " + testNQN + " -a " + ip + " -s 4420 -l -1"
	}

	t.Run("invalid arguments", func(t *testing.T) {
		h := newFakeHost()

		err := newTestService(h).ConnectPaths(context.Background(), "", []string{testIP1})

		assert.ErrorContains(t, err, "empty subsystem NQN")
		assert.Empty(t, h.ran("connect"))
	})

	t.Run("nothing missing connects nothing", func(t *testing.T) {
		h := connectedHost()

		require.NoError(t, newTestService(h).ConnectPaths(context.Background(), testNQN, []string{testIP1, testIP2}))
		assert.Empty(t, h.ran("connect"))
	})

	t.Run("no target IPs connects nothing", func(t *testing.T) {
		h := connectedHost()

		require.NoError(t, newTestService(h).ConnectPaths(context.Background(), testNQN, nil))
		assert.Empty(t, h.ran("connect"))
	})

	t.Run("a path in any state counts as present", func(t *testing.T) {
		h := connectedHost()
		h.setState("nvme0", "connecting")
		h.setState("nvme1", "deleting")

		require.NoError(t, newTestService(h).ConnectPaths(context.Background(), testNQN, []string{testIP1, testIP2}))
		assert.Empty(t, h.ran("connect"))
	})

	t.Run("connects only the missing paths", func(t *testing.T) {
		h := newFakeHost()
		h.addController(testIP1, "connecting")

		require.NoError(t, newTestService(h).ConnectPaths(context.Background(), testNQN, []string{testIP1, testIP2}))
		assert.Equal(t, []string{connectTo(testIP2)}, h.ran("connect"))
	})

	t.Run("a path to an IP nobody asked for does not count", func(t *testing.T) {
		h := newFakeHost()
		h.addController("10.9.9.9", "live")

		require.NoError(t, newTestService(h).ConnectPaths(context.Background(), testNQN, []string{testIP1}))
		assert.Equal(t, []string{connectTo(testIP1)}, h.ran("connect"))
	})

	t.Run("a subsystem that is not on the host is connected at every IP", func(t *testing.T) {
		h := newFakeHost()

		require.NoError(t, newTestService(h).ConnectPaths(context.Background(), testNQN, []string{testIP1, testIP2}))
		assert.ElementsMatch(t, []string{connectTo(testIP1), connectTo(testIP2)}, h.ran("connect"))
	})

	t.Run("a failed connect is returned and not retried", func(t *testing.T) {
		h := newFakeHost()
		h.connect = func(context.Context, string) error { return fmt.Errorf("connection refused") }

		err := newTestService(h).ConnectPaths(context.Background(), testNQN, []string{testIP1, testIP2})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to connect subsystem "+testNQN+" to "+testIP1)
		assert.Contains(t, err.Error(), "failed to connect subsystem "+testNQN+" to "+testIP2)
		assert.Contains(t, err.Error(), "connection refused")
		assert.Len(t, h.ran("connect"), 2, "a single attempt must not retry")
	})

	t.Run("one failing connect does not stop the other", func(t *testing.T) {
		h := newFakeHost()
		h.connect = func(_ context.Context, ip string) error {
			if ip == testIP1 {
				return fmt.Errorf("connection refused")
			}
			h.addController(ip, "live")
			return nil
		}

		err := newTestService(h).ConnectPaths(context.Background(), testNQN, []string{testIP1, testIP2})

		require.Error(t, err)
		assert.Contains(t, err.Error(), testIP1)
		assert.NotContains(t, err.Error(), testIP2)
		assert.Len(t, h.ran("connect"), 2)
	})

	t.Run("a sysfs read error is returned and nothing is connected", func(t *testing.T) {
		h := connectedHost()
		boom := fmt.Errorf("input/output error")
		s := New(WithNVMeAdapter(nvme.New(nvme.WithCommand(h), nvme.WithFs(failFs{h.fs, "/sys/class", boom}))))

		err := s.ConnectPaths(context.Background(), testNQN, []string{testIP1})

		assert.ErrorIs(t, err, boom)
		assert.Empty(t, h.ran("connect"))
	})

	t.Run("a hung connect is killed when the context ends", func(t *testing.T) {
		h := newFakeHost()
		h.connect = func(ctx context.Context, _ string) error {
			<-ctx.Done()
			return errors.TimeoutError("command timed out")
		}

		start := time.Now()
		err := newTestService(h).ConnectPaths(withTimeout(t, 50*time.Millisecond), testNQN, []string{testIP1})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "command timed out")
		assert.Less(t, time.Since(start), 2*time.Second)
	})
}

// pathsOf returns the address of every path the host still has for the subsystem, in controller order.
func pathsOf(t *testing.T, h *fakeHost) []string {
	t.Helper()
	sub, err := newTestService(h).Subsystem(context.Background(), testNQN)
	require.NoError(t, err)
	var out []string
	for _, p := range sub.Paths() {
		out = append(out, p.Address())
	}
	return out
}

func TestPrunePaths(t *testing.T) {
	const (
		ip1 = "1.1.1.1"
		ip2 = "2.2.2.2"
		ip3 = "3.3.3.3"
	)
	ctx := context.Background()
	disconnectOf := func(controller string) string { return "disconnect -d " + controller }

	t.Run("invalid arguments", func(t *testing.T) {
		assert.ErrorContains(t, newTestService(newFakeHost()).PrunePaths(ctx, "", nil), "empty subsystem NQN")
	})

	t.Run("keeps the path of a LIF that is still desired", func(t *testing.T) {
		// A LIF that is briefly unreachable is still desired, so its path is left for the kernel to recover.
		h := newFakeHost()
		h.addController(ip1, "live")
		h.addController(ip2, "connecting")

		require.NoError(t, newTestService(h).PrunePaths(ctx, testNQN, []string{ip1, ip2}))

		assert.Empty(t, h.ran("disconnect"))
		assert.Equal(t, []string{ip1, ip2}, pathsOf(t, h))
	})

	t.Run("a steady state runs no nvme command", func(t *testing.T) {
		h := newFakeHost()
		h.addController(ip1, "live")
		h.addController(ip2, "connecting")

		require.NoError(t, newTestService(h).PrunePaths(ctx, testNQN, []string{ip1, ip2}))

		assert.Empty(t, h.commands())
	})

	t.Run("removes an undesired path even when it is live", func(t *testing.T) {
		h := newFakeHost()
		h.addController(ip1, "live")
		h.addController(ip2, "live")

		require.NoError(t, newTestService(h).PrunePaths(ctx, testNQN, []string{ip1}))

		assert.Equal(t, []string{disconnectOf("nvme1")}, h.ran("disconnect"))
		assert.Equal(t, []string{ip1}, pathsOf(t, h))
	})

	t.Run("keeps the only live path until a desired path is live", func(t *testing.T) {
		h := newFakeHost()
		h.addController(ip1, "live")
		h.addController(ip2, "connecting")

		require.NoError(t, newTestService(h).PrunePaths(ctx, testNQN, []string{ip2}))

		assert.Empty(t, h.ran("disconnect"))
		assert.Equal(t, []string{ip1, ip2}, pathsOf(t, h))
	})

	t.Run("never removes the last path", func(t *testing.T) {
		h := newFakeHost()
		h.addController(ip1, "connecting")
		h.addController(ip2, "connecting")

		require.NoError(t, newTestService(h).PrunePaths(ctx, testNQN, nil))

		assert.Equal(t, []string{disconnectOf("nvme0")}, h.ran("disconnect"))
		assert.Equal(t, []string{ip2}, pathsOf(t, h))
	})

	t.Run("a path with no address is kept", func(t *testing.T) {
		h := newFakeHost()
		h.addController(ip1, "live")
		h.addController("", "connecting")

		require.NoError(t, newTestService(h).PrunePaths(ctx, testNQN, []string{ip1}))

		assert.Empty(t, h.ran("disconnect"))
	})

	t.Run("a removed live path no longer counts as live", func(t *testing.T) {
		h := newFakeHost()
		h.addController(ip1, "live")
		h.addController(ip2, "live")
		h.addController(ip3, "connecting")

		require.NoError(t, newTestService(h).PrunePaths(ctx, testNQN, []string{ip3}))

		assert.Equal(t, []string{disconnectOf("nvme0")}, h.ran("disconnect"),
			"the second live path is now the only live path and must stay")
		assert.Equal(t, []string{ip2, ip3}, pathsOf(t, h))
	})

	t.Run("a failed disconnect keeps the path and is not returned", func(t *testing.T) {
		h := newFakeHost()
		h.addController(ip1, "live")
		h.addController(ip2, "connecting")
		h.disconnectPath = func(context.Context, string) error { return fmt.Errorf("nvme disconnect failed") }

		require.NoError(t, newTestService(h).PrunePaths(ctx, testNQN, []string{ip1}))

		assert.Equal(t, []string{disconnectOf("nvme1")}, h.ran("disconnect"))
		assert.Equal(t, []string{ip1, ip2}, pathsOf(t, h))
	})

	t.Run("a failed disconnect does not stop the next path", func(t *testing.T) {
		h := newFakeHost()
		h.addController(ip1, "live")
		h.addController(ip2, "connecting")
		h.addController(ip3, "connecting")
		h.disconnectPath = func(_ context.Context, controller string) error {
			if controller == "nvme1" {
				return fmt.Errorf("nvme disconnect failed")
			}
			return nil
		}

		require.NoError(t, newTestService(h).PrunePaths(ctx, testNQN, []string{ip1}))

		assert.Equal(t, []string{disconnectOf("nvme1"), disconnectOf("nvme2")}, h.ran("disconnect"))
		assert.Equal(t, []string{ip1, ip2}, pathsOf(t, h))
	})

	t.Run("a failed live disconnect leaves the live count alone", func(t *testing.T) {
		h := newFakeHost()
		h.addController(ip1, "live")
		h.addController(ip2, "live")
		h.disconnectPath = func(_ context.Context, controller string) error {
			if controller == "nvme0" {
				return fmt.Errorf("nvme disconnect failed")
			}
			return nil
		}

		require.NoError(t, newTestService(h).PrunePaths(ctx, testNQN, nil))

		assert.Equal(t, []string{disconnectOf("nvme0"), disconnectOf("nvme1")}, h.ran("disconnect"))
		assert.Equal(t, []string{ip1}, pathsOf(t, h))
	})

	t.Run("a fault injected before the disconnect keeps the path", func(t *testing.T) {
		h := newFakeHost()
		h.addController(ip1, "live")
		h.addController(ip2, "connecting")
		saved := beforeNVMeDisconnect
		defer func() { beforeNVMeDisconnect = saved }()
		beforeNVMeDisconnect = failingInjector{}

		require.NoError(t, newTestService(h).PrunePaths(ctx, testNQN, []string{ip1}))

		assert.Empty(t, h.ran("disconnect"))
		assert.Equal(t, []string{ip1, ip2}, pathsOf(t, h))
	})

	t.Run("a subsystem that is not on the host is not found", func(t *testing.T) {
		err := newTestService(newFakeHost()).PrunePaths(ctx, testNQN, []string{ip1})

		assert.True(t, errors.IsNotFoundError(err), "got %v", err)
	})

	t.Run("a subsystem with no paths is not found", func(t *testing.T) {
		h := newFakeHost()
		h.write(subsysDir+"/subsysnqn", testNQN)
		h.addNamespace("nvme0n1", testUUID, 1<<30)

		err := newTestService(h).PrunePaths(ctx, testNQN, nil)

		assert.True(t, errors.IsNotFoundError(err), "got %v", err)
		assert.Empty(t, h.ran("disconnect"))
	})

	t.Run("a sysfs read error is returned", func(t *testing.T) {
		h := connectedHost()
		boom := fmt.Errorf("input/output error")
		s := New(WithNVMeAdapter(nvme.New(nvme.WithCommand(h), nvme.WithFs(failFs{h.fs, "/sys/class", boom}))))

		err := s.PrunePaths(ctx, testNQN, nil)

		assert.ErrorIs(t, err, boom)
		assert.Empty(t, h.ran("disconnect"))
	})
}

// failingInjector is a fault injection point that always fires.
type failingInjector struct{}

func (failingInjector) Inject() error { return fmt.Errorf("injected fault") }

func TestEnsureDisconnect_FaultInjection(t *testing.T) {
	h := connectedHost()
	saved := beforeNVMeDisconnect
	defer func() { beforeNVMeDisconnect = saved }()
	beforeNVMeDisconnect = failingInjector{}

	err := newTestService(h).EnsureDisconnect(context.Background(), testNQN, testUUID, false)

	assert.EqualError(t, err, "injected fault")
	assert.Empty(t, h.ran("disconnect"))
}

func TestIsFlushError(t *testing.T) {
	inner := fmt.Errorf("io error")
	target := newFlushError("flush /dev/nvme0n1: io error", inner)

	assert.True(t, IsFlushError(target))
	assert.True(t, IsFlushError(fmt.Errorf("outer: %w", target)))
	assert.False(t, IsFlushError(inner))
	assert.False(t, IsFlushError(nil))
	assert.Equal(t, "flush /dev/nvme0n1: io error", target.Error())
	assert.Equal(t, inner, target.Unwrap())
	assert.ErrorIs(t, target, inner)
}
