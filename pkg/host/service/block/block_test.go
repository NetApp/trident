// Copyright 2026 NetApp, Inc. All Rights Reserved.

package block

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	blockadapter "github.com/netapp/trident/pkg/host/adapter/block"
)

// stubFS reports a fixed blkid result and counts probes. Other adapter calls panic.
type stubFS struct {
	blockadapter.Adapter
	fsType string
	err    error
	probes int
}

func (f *stubFS) Type(context.Context, string) (string, error) {
	f.probes++
	return f.fsType, f.err
}

func TestResolveType(t *testing.T) {
	blkidErr := errors.New("blkid failed")
	noSignature := &blockadapter.NoFilesystemSignatureError{}

	cases := map[string]struct {
		requested  string
		volumeMode string
		skipProbe  bool
		onDisk     string
		probeErr   error
		want       string
		wantErr    error
		wantProbes int
	}{
		"explicit type is used without probing": {
			requested: "xfs", volumeMode: "Filesystem", onDisk: "ext4", want: "xfs",
		},
		"explicit raw is used without probing": {
			requested: "raw", volumeMode: "Filesystem", want: "raw",
		},
		"on-disk type is adopted over Filesystem mode": {
			volumeMode: "Filesystem", onDisk: "xfs", want: "xfs", wantProbes: 1,
		},
		"on-disk type is adopted over Block mode": {
			volumeMode: "Block", onDisk: "ext4", want: "ext4", wantProbes: 1,
		},
		"on-disk type is adopted without a volume mode": {
			onDisk: "ext3", want: "ext3", wantProbes: 1,
		},
		"blank device with Filesystem mode defaults to ext4": {
			volumeMode: "Filesystem", probeErr: noSignature, want: "ext4", wantProbes: 1,
		},
		"blank device with Block mode defaults to raw": {
			volumeMode: "Block", probeErr: noSignature, want: "raw", wantProbes: 1,
		},
		"blank device without a volume mode stays unresolved": {
			probeErr: noSignature, want: "", wantProbes: 1,
		},
		"unknown signature with Filesystem mode is an error": {
			volumeMode: "Filesystem", onDisk: "ntfs", wantErr: ErrUnknownFilesystem, wantProbes: 1,
		},
		"unknown signature with Block mode defaults to raw": {
			volumeMode: "Block", onDisk: "ntfs", want: "raw", wantProbes: 1,
		},
		"unknown signature without a volume mode stays unresolved": {
			onDisk: "ntfs", want: "", wantProbes: 1,
		},
		"probe failure is returned": {
			volumeMode: "Filesystem", probeErr: blkidErr, wantErr: blkidErr, wantProbes: 1,
		},
		"skipProbe with Filesystem mode defaults to ext4": {
			volumeMode: "Filesystem", skipProbe: true, onDisk: "xfs", want: "ext4",
		},
		"skipProbe with Block mode defaults to raw": {
			volumeMode: "Block", skipProbe: true, want: "raw",
		},
		"skipProbe without a volume mode stays unresolved": {
			skipProbe: true, want: "",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fs := &stubFS{fsType: tc.onDisk, err: tc.probeErr}
			s := &service{block: fs}

			got, err := s.ResolveType(context.Background(), "/dev/dm-0", tc.requested, tc.volumeMode, tc.skipProbe)
			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
			assert.Equal(t, tc.wantProbes, fs.probes)
		})
	}
}

func TestResolveType_EmptyDevicePath(t *testing.T) {
	s := &service{block: &stubFS{}}
	_, err := s.ResolveType(context.Background(), "", "", "Filesystem", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "device path is required")
}

// stubTopology serves devices by name and dm UUID; a missing one is not found. Other adapter calls panic.
type stubTopology struct {
	blockadapter.Adapter
	byName map[string]*blockadapter.Device
	byUUID map[string]*blockadapter.Device
	err    error
}

func (s *stubTopology) DeviceByName(_ context.Context, name string) (*blockadapter.Device, error) {
	if s.err != nil {
		return nil, s.err
	}
	if dev, ok := s.byName[name]; ok {
		return dev, nil
	}
	return nil, blockadapter.NewDeviceNotFoundErrorForTest(name)
}

func (s *stubTopology) DeviceByDMUUID(_ context.Context, dmUUID string) (*blockadapter.Device, error) {
	if dev, ok := s.byUUID[dmUUID]; ok {
		return dev, nil
	}
	return nil, blockadapter.NewDeviceNotFoundErrorForTest(dmUUID)
}

func TestIsMapperStale(t *testing.T) {
	const uuid = "CRYPT-LUKS2-x"
	readErr := errors.New("sysfs unreadable")
	topology := func(mapper *blockadapter.Device, devices ...string) *stubTopology {
		s := &stubTopology{byName: map[string]*blockadapter.Device{}, byUUID: map[string]*blockadapter.Device{}}
		if mapper != nil {
			s.byUUID[uuid] = mapper
		}
		for _, name := range devices {
			s.byName[name] = blockadapter.NewDeviceForTest(name)
		}
		return s
	}

	tests := map[string]struct {
		topology  *stubTopology
		wantStale bool
		wantErr   error
	}{
		"not on the host":     {topology: topology(nil)},
		"on the base device":  {topology: topology(blockadapter.NewMapperDeviceForTest("dm-0", uuid, "nvme0n1"), "nvme0n1")},
		"on another device":   {topology: topology(blockadapter.NewMapperDeviceForTest("dm-0", uuid, "nvme3n1"), "nvme3n1"), wantStale: true},
		"no slaves":           {topology: topology(blockadapter.NewMapperDeviceForTest("dm-0", uuid)), wantStale: true},
		"every slave is gone": {topology: topology(blockadapter.NewMapperDeviceForTest("dm-0", uuid, "nvme0n1")), wantStale: true},
		"slave cannot be read": {
			topology: func() *stubTopology {
				s := topology(blockadapter.NewMapperDeviceForTest("dm-0", uuid, "nvme0n1"))
				s.err = readErr
				return s
			}(),
			wantErr: readErr,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			stale, err := New(WithBlockAdapter(tc.topology)).IsMapperStale(context.Background(), uuid, "nvme0n1")

			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantStale, stale)
		})
	}
}
