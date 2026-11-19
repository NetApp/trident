// Copyright 2026 NetApp, Inc. All Rights Reserved.

package luks

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/pkg/host/adapter/block"
	"github.com/netapp/trident/pkg/host/adapter/cryptsetup"
	tridenterrors "github.com/netapp/trident/utils/errors"
)

const emptyRegionSize = block.EmptyCheckSize

type stubBlock struct {
	block.Adapter
	buf []byte
	err error
}

func (b *stubBlock) Read(_ context.Context, _ string, opts block.DataOptions) ([]byte, error) {
	if opts.BlockSize != block.ProbeBlockSize || opts.BlockCount != block.EmptyCheckBlockCount {
		return nil, errors.New("unexpected read size")
	}
	return b.buf, b.err
}

func (b *stubBlock) Zero(context.Context, string, block.DataOptions) error { return nil }

type stubCrypt struct {
	cryptsetup.Adapter
	formatCalls    int
	formatErr      error
	testFn         func(passphrase string) error
	changeKeyCalls int
	changeKeyErr   error
	events         *[]string
}

func (c *stubCrypt) TestPassphrase(_ context.Context, _, _, passphrase string) error {
	return c.testFn(passphrase)
}

func (c *stubCrypt) ChangeKey(_ context.Context, _, _, _, _ string) error {
	c.changeKeyCalls++
	if c.events != nil {
		*c.events = append(*c.events, "changekey")
	}
	return c.changeKeyErr
}

var errControllerUnavailable = errors.New("controller unavailable")

// recordingHelper records the calls to UpdateVolumeLUKSPassphraseNames and fails the calls listed in failOn
// (1-based call numbers).
type recordingHelper struct {
	calls  [][]string
	volume []string
	failOn map[int]bool
	events *[]string
}

func (h *recordingHelper) UpdateVolumeLUKSPassphraseNames(_ context.Context, volume string, names []string) error {
	h.calls = append(h.calls, names)
	h.volume = append(h.volume, volume)
	if h.events != nil {
		*h.events = append(*h.events, "update")
	}
	if h.failOn[len(h.calls)] {
		return errControllerUnavailable
	}
	return nil
}

func (c *stubCrypt) Format(_ context.Context, _, _ string, _ cryptsetup.FormatOptions) (string, error) {
	c.formatCalls++
	if c.events != nil {
		*c.events = append(*c.events, "format")
	}
	return "", c.formatErr
}

func TestConstructMapperName(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
	}{
		"normal": {in: "pvc_123", want: "luks-pvc_123"},
		"empty":  {in: "", want: "luks-"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, constructMapperName(tc.in))
		})
	}
}

func TestRequireEmpty(t *testing.T) {
	nonZeroAt := func(i int) []byte {
		buf := make([]byte, emptyRegionSize)
		buf[i] = 0x1
		return buf
	}

	cases := map[string]struct {
		buf     []byte
		readErr error
		wantErr string
	}{
		"all zeros":             {buf: make([]byte, emptyRegionSize)},
		"non-zero first byte":   {buf: nonZeroAt(0), wantErr: "is not empty"},
		"non-zero last byte":    {buf: nonZeroAt(emptyRegionSize - 1), wantErr: "is not empty"},
		"filesystem superblock": {buf: nonZeroAt(1080), wantErr: "is not empty"},
		"short read":            {buf: make([]byte, block.ProbeBlockSize), wantErr: "cannot verify"},
		"read error":            {readErr: errors.New("io error"), wantErr: "failed to read device"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := &service{block: &stubBlock{buf: tc.buf, err: tc.readErr}}
			err := s.requireEmpty(context.Background(), "/dev/dm-0")
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestEnsureFormat_RefusesNonEmptyDevice(t *testing.T) {
	buf := make([]byte, emptyRegionSize)
	buf[512] = 0xff
	crypt := &stubCrypt{}
	s := &service{block: &stubBlock{buf: buf}, crypt: crypt}

	err := s.ensureFormat(
		context.Background(), "vol", "/dev/dm-0",
		Secret{Name: "current", Passphrase: "pass"}, cryptsetup.DefaultFormatOptions(),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "device not safe for LUKS format")
	assert.Zero(t, crypt.formatCalls, "must not format a non-empty device")
}

func TestEnsureFormat_FormatsEmptyDevice(t *testing.T) {
	crypt := &stubCrypt{}
	s := &service{
		block:  &stubBlock{buf: make([]byte, emptyRegionSize)},
		crypt:  crypt,
		helper: &recordingHelper{},
	}

	err := s.ensureFormat(
		context.Background(), "vol", "/dev/dm-0",
		Secret{Name: "current", Passphrase: "pass"}, cryptsetup.DefaultFormatOptions(),
	)

	require.NoError(t, err)
	assert.Equal(t, 1, crypt.formatCalls)
}

func TestEnsureFormat_NoPassphrase(t *testing.T) {
	crypt := &stubCrypt{}
	s := &service{block: &stubBlock{buf: make([]byte, emptyRegionSize)}, crypt: crypt}

	err := s.ensureFormat(
		context.Background(), "vol", "/dev/dm-0", Secret{}, cryptsetup.DefaultFormatOptions(),
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEmptyPassphrase)
	assert.Zero(t, crypt.formatCalls)
}

func TestEnsureFormat_RecordsPassphraseNameBeforeFormat(t *testing.T) {
	var events []string
	crypt := &stubCrypt{events: &events}
	helper := &recordingHelper{events: &events}
	s := &service{block: &stubBlock{buf: make([]byte, emptyRegionSize)}, crypt: crypt, helper: helper}

	err := s.ensureFormat(
		context.Background(), "vol", "/dev/dm-0",
		Secret{Name: "current", Passphrase: "pass"}, cryptsetup.DefaultFormatOptions(),
	)

	require.NoError(t, err)
	assert.Equal(t, []string{"update", "format"}, events)
	assert.Equal(t, [][]string{{"current"}}, helper.calls)
	assert.Equal(t, []string{"vol"}, helper.volume)
}

type notLUKSCrypt struct{ stubCrypt }

func (c *notLUKSCrypt) IsLUKS(context.Context, string) error {
	return &cryptsetup.NotLUKSDeviceError{}
}

func TestEnsureDevice_RecordsPassphraseNameAgainstVolume(t *testing.T) {
	crypt := &notLUKSCrypt{stubCrypt{formatErr: errors.New("stop after format")}}
	helper := &recordingHelper{}
	s := &service{block: &stubBlock{buf: make([]byte, emptyRegionSize)}, crypt: crypt, helper: helper}
	opts := DefaultEnsureDeviceOptions()
	opts.VolumeID = "pvc-volume"

	_, err := s.EnsureDevice(
		context.Background(), "internal-id", "/dev/nvme0n1", Secret{Name: "current", Passphrase: "pass"}, Secret{}, opts,
	)

	require.Error(t, err)
	assert.Equal(t, 1, crypt.formatCalls)
	assert.Equal(t, []string{"pvc-volume"}, helper.volume)
}

func TestEnsureFormat_RecordFailureBlocksFormat(t *testing.T) {
	crypt := &stubCrypt{}
	helper := &recordingHelper{failOn: map[int]bool{1: true}}
	s := &service{block: &stubBlock{buf: make([]byte, emptyRegionSize)}, crypt: crypt, helper: helper}

	err := s.ensureFormat(
		context.Background(), "vol", "/dev/dm-0",
		Secret{Name: "current", Passphrase: "pass"}, cryptsetup.DefaultFormatOptions(),
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, errControllerUnavailable)
	assert.Zero(t, crypt.formatCalls, "must not format when the name could not be recorded")
}

func TestEnsureFormat_EmptyPassphraseName(t *testing.T) {
	crypt := &stubCrypt{}
	helper := &recordingHelper{}
	s := &service{block: &stubBlock{buf: make([]byte, emptyRegionSize)}, crypt: crypt, helper: helper}

	err := s.ensureFormat(
		context.Background(), "vol", "/dev/dm-0",
		Secret{Passphrase: "pass"}, cryptsetup.DefaultFormatOptions(),
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEmptyPassphraseName)
	assert.Empty(t, helper.calls)
	assert.Zero(t, crypt.formatCalls)
}

// rotationCrypt accepts the previous passphrase always and the current one only after ChangeKey.
func rotationCrypt(events *[]string) *stubCrypt {
	c := &stubCrypt{events: events}
	c.testFn = func(passphrase string) error {
		switch {
		case passphrase == "new" && c.changeKeyCalls > 0, passphrase == "old":
			return nil
		}
		return &cryptsetup.IncorrectPassphraseError{}
	}
	return c
}

func TestEnsurePassphrase_RotationRecordsNamesAroundChangeKey(t *testing.T) {
	var events []string
	crypt := rotationCrypt(&events)
	helper := &recordingHelper{events: &events}
	s := &service{crypt: crypt, helper: helper}

	err := s.EnsurePassphrase(
		context.Background(), "vol", "/dev/sdx",
		Secret{Name: "new-name", Passphrase: "new"}, Secret{Name: "old-name", Passphrase: "old"}, nil,
	)

	require.NoError(t, err)
	assert.Equal(t, []string{"update", "changekey", "update"}, events)
	assert.Equal(t, [][]string{{"new-name", "old-name"}, {"new-name"}}, helper.calls)
}

func TestEnsurePassphrase_RecordsNamesAgainstVolume(t *testing.T) {
	crypt := rotationCrypt(nil)
	helper := &recordingHelper{}
	s := &service{crypt: crypt, helper: helper}

	err := s.EnsurePassphrase(
		context.Background(), "internal-vol", "/dev/sdx",
		Secret{Name: "new-name", Passphrase: "new"}, Secret{Name: "old-name", Passphrase: "old"},
		&EnsurePassphraseOptions{VolumeID: "pvc-1"},
	)

	require.NoError(t, err)
	assert.Equal(t, []string{"pvc-1", "pvc-1"}, helper.volume)
}

func TestEnsurePassphrase_RecordFailureBeforeRotationBlocksChangeKey(t *testing.T) {
	crypt := rotationCrypt(nil)
	helper := &recordingHelper{failOn: map[int]bool{1: true}}
	s := &service{crypt: crypt, helper: helper}

	err := s.EnsurePassphrase(
		context.Background(), "vol", "/dev/sdx",
		Secret{Name: "new-name", Passphrase: "new"}, Secret{Name: "old-name", Passphrase: "old"}, nil,
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, errControllerUnavailable)
	assert.Zero(t, crypt.changeKeyCalls, "must not rotate when the names could not be recorded")
}

func TestEnsurePassphrase_RecordFailureAfterRotationIsNotAnError(t *testing.T) {
	crypt := rotationCrypt(nil)
	helper := &recordingHelper{failOn: map[int]bool{2: true}}
	s := &service{crypt: crypt, helper: helper}

	err := s.EnsurePassphrase(
		context.Background(), "vol", "/dev/sdx",
		Secret{Name: "new-name", Passphrase: "new"}, Secret{Name: "old-name", Passphrase: "old"}, nil,
	)

	require.NoError(t, err)
	assert.Equal(t, 1, crypt.changeKeyCalls)
	assert.Len(t, helper.calls, 2)
}

func TestEnsurePassphrase_ForceUpdateFailureIsReturned(t *testing.T) {
	crypt := &stubCrypt{testFn: func(string) error { return nil }}
	helper := &recordingHelper{failOn: map[int]bool{1: true}}
	s := &service{crypt: crypt, helper: helper}

	err := s.EnsurePassphrase(
		context.Background(), "vol", "/dev/sdx",
		Secret{Name: "name", Passphrase: "pass"}, Secret{}, &EnsurePassphraseOptions{ForceUpdate: true},
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, errControllerUnavailable)

	// Without ForceUpdate, a working current passphrase makes no controller call.
	helper = &recordingHelper{}
	s = &service{crypt: crypt, helper: helper}
	require.NoError(t, s.EnsurePassphrase(
		context.Background(), "vol", "/dev/sdx", Secret{Name: "name", Passphrase: "pass"}, Secret{}, nil,
	))
	assert.Empty(t, helper.calls)
}

type mapperBlock struct {
	block.Adapter
	dev     *block.Device
	err     error
	dmNames []string
}

func (b *mapperBlock) DeviceByDMName(_ context.Context, name string) (*block.Device, error) {
	b.dmNames = append(b.dmNames, name)
	return b.dev, b.err
}

func TestDeviceByInternalID(t *testing.T) {
	t.Run("found by mapper name", func(t *testing.T) {
		b := &mapperBlock{dev: block.NewCryptDeviceForTest("dm-0", "luks-pvc_1", "CRYPT-LUKS2-x")}

		dev, err := New(WithBlockAdapter(b)).DeviceByInternalID(context.Background(), "pvc_1")

		require.NoError(t, err)
		assert.Equal(t, []string{"luks-pvc_1"}, b.dmNames)
		assert.Equal(t, "CRYPT-LUKS2-x", dev.ID())
		assert.Equal(t, "luks-pvc_1", dev.MapperName())
	})

	t.Run("not found", func(t *testing.T) {
		b := &mapperBlock{err: &block.DeviceNotFoundError{}}

		dev, err := New(WithBlockAdapter(b)).DeviceByInternalID(context.Background(), "pvc_1")

		assert.True(t, tridenterrors.IsNotFoundError(err))
		assert.Nil(t, dev)
	})

	t.Run("not a crypt device", func(t *testing.T) {
		b := &mapperBlock{dev: block.NewDeviceForTest("dm-0")}

		dev, err := New(WithBlockAdapter(b)).DeviceByInternalID(context.Background(), "pvc_1")

		assert.Error(t, err)
		assert.Nil(t, dev)
	})

	t.Run("empty internal ID", func(t *testing.T) {
		b := &mapperBlock{}

		_, err := New(WithBlockAdapter(b)).DeviceByInternalID(context.Background(), "")

		assert.Error(t, err)
		assert.Empty(t, b.dmNames)
	})
}

type rejectingCrypt struct {
	stubCrypt
	openCalls int
}

func (c *rejectingCrypt) Status(context.Context, string) (string, error) {
	return "", &cryptsetup.DeviceNotActiveError{}
}

func (c *rejectingCrypt) Open(context.Context, string, string, string, cryptsetup.OpenOptions) error {
	c.openCalls++
	return &cryptsetup.IncorrectPassphraseError{}
}

func TestEnsureOpen_AllPassphrasesRejected(t *testing.T) {
	crypt := &rejectingCrypt{}
	s := &service{crypt: crypt}

	err := s.ensureOpen(
		context.Background(), "/dev/nvme0n1", "luks-pvc_1", cryptsetup.DefaultOpenOptions(),
		Secret{Name: "current", Passphrase: "a"}, Secret{Name: "previous", Passphrase: "b"},
	)

	require.Error(t, err)
	assert.True(t, tridenterrors.IsAuthError(err))
	// Whelk and users match on this wording for a failed open.
	assert.Contains(t, err.Error(), "could not open LUKS device")
	assert.Equal(t, 2, crypt.openCalls)
}

// deviceCrypt reports isLUKS from IsLUKS, mapperOpen from Status, and openErr and closeErr from Open and Close.
type deviceCrypt struct {
	stubCrypt
	isLUKS     bool
	mapperOpen bool
	openErr    error
	closeErr   error
}

func (c *deviceCrypt) IsLUKS(context.Context, string) error {
	if c.isLUKS {
		return nil
	}
	return &cryptsetup.NotLUKSDeviceError{}
}

func (c *deviceCrypt) Status(context.Context, string) (string, error) {
	if c.mapperOpen {
		return "active", nil
	}
	return "", &cryptsetup.DeviceNotActiveError{}
}

func (c *deviceCrypt) Open(context.Context, string, string, string, cryptsetup.OpenOptions) error {
	return c.openErr
}

func (c *deviceCrypt) Close(context.Context, string) error { return c.closeErr }

// cryptBlock is an empty device with the open mapper luks-pvc_1.
type cryptBlock struct{ stubBlock }

func (b *cryptBlock) DeviceByDMName(context.Context, string) (*block.Device, error) {
	return block.NewCryptDeviceForTest("dm-0", "luks-pvc_1", "CRYPT-LUKS2-x"), nil
}

func (b *cryptBlock) DeviceByDMUUID(context.Context, string) (*block.Device, error) {
	return block.NewCryptDeviceForTest("dm-0", "luks-pvc_1", "CRYPT-LUKS2-x"), nil
}

func TestEnsureDevice_FreshFormat(t *testing.T) {
	current := Secret{Name: "current", Passphrase: "pass"}
	newService := func(crypt *deviceCrypt) *service {
		return &service{
			block:  &cryptBlock{stubBlock{buf: make([]byte, emptyRegionSize)}},
			crypt:  crypt,
			helper: &recordingHelper{},
		}
	}

	t.Run("a device formatted by this call is fresh", func(t *testing.T) {
		crypt := &deviceCrypt{mapperOpen: true}

		dev, err := newService(crypt).EnsureDevice(context.Background(), "pvc_1", "/dev/nvme0n1", current, Secret{}, nil)

		require.NoError(t, err)
		assert.Equal(t, 1, crypt.formatCalls)
		assert.True(t, dev.IsFreshFormat())
	})

	t.Run("a device that was already LUKS is not fresh", func(t *testing.T) {
		crypt := &deviceCrypt{isLUKS: true, mapperOpen: true}

		dev, err := newService(crypt).EnsureDevice(context.Background(), "pvc_1", "/dev/nvme0n1", current, Secret{}, nil)

		require.NoError(t, err)
		assert.Zero(t, crypt.formatCalls)
		assert.False(t, dev.IsFreshFormat())
	})
}

func TestOpenDevice_Busy(t *testing.T) {
	t.Run("busy with the mapper open is success", func(t *testing.T) {
		s := &service{crypt: &deviceCrypt{mapperOpen: true, openErr: &cryptsetup.BusyDeviceError{}}}

		err := s.openDevice(context.Background(), "/dev/nvme0n1", "luks-pvc_1", "pass", cryptsetup.DefaultOpenOptions())

		assert.NoError(t, err)
	})

	t.Run("busy with the mapper not open is returned", func(t *testing.T) {
		s := &service{crypt: &deviceCrypt{openErr: &cryptsetup.BusyDeviceError{}}}

		err := s.openDevice(context.Background(), "/dev/nvme0n1", "luks-pvc_1", "pass", cryptsetup.DefaultOpenOptions())

		assert.ErrorContains(t, err, "failed to open LUKS device luks-pvc_1")
	})
}

func TestEnsureClose_Force(t *testing.T) {
	newService := func(closeErr error) *service {
		return &service{block: &cryptBlock{}, crypt: &deviceCrypt{closeErr: closeErr}}
	}

	t.Run("a busy device is tolerated under force", func(t *testing.T) {
		err := newService(&cryptsetup.BusyDeviceError{}).EnsureClose(context.Background(), "CRYPT-LUKS2-x", true)

		assert.NoError(t, err)
	})

	t.Run("other close errors are returned under force", func(t *testing.T) {
		err := newService(errors.New("dm ioctl failed")).EnsureClose(context.Background(), "CRYPT-LUKS2-x", true)

		assert.ErrorContains(t, err, "dm ioctl failed")
	})

	t.Run("a busy device is returned without force", func(t *testing.T) {
		err := newService(&cryptsetup.BusyDeviceError{}).EnsureClose(context.Background(), "CRYPT-LUKS2-x", false)

		assert.Error(t, err)
	})
}
