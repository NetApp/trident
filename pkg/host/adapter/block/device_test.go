// Copyright 2026 NetApp, Inc. All Rights Reserved.

package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeviceName(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
	}{
		"fullPath":   {in: "/dev/sda", want: "sda"},
		"nameOnly":   {in: "sda", want: "sda"},
		"nestedPath": {in: "/dev/mapper/foo", want: "mapper/foo"},
		"empty":      {in: "", want: "."},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, DeviceName(tc.in))
		})
	}
}

func TestDevicePath(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
	}{
		"shortName": {in: "sda", want: "/dev/sda"},
		"fullPath":  {in: "/dev/sda", want: "/dev/sda"},
		"empty":     {in: "", want: "/dev/"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, DevicePath(tc.in))
		})
	}
}

func TestDeviceIsEqual(t *testing.T) {
	base := &Device{
		name: "dm-0", path: "/dev/dm-0", size: 1024,
		slaves: []string{"sda", "sdb"}, holders: []string{"dm-1"},
		dmInfo: &DMDevice{uuid: "mpath-123", name: "mpathA"},
	}
	identical := &Device{
		name: "dm-0", path: "/dev/dm-0", size: 1024,
		slaves: []string{"sda", "sdb"}, holders: []string{"dm-1"},
		dmInfo: &DMDevice{uuid: "mpath-123", name: "mpathA"},
	}

	cases := map[string]struct {
		a, b *Device
		want bool
	}{
		"same_pointer":      {a: base, b: base, want: true},
		"identical_value":   {a: base, b: identical, want: true},
		"nil_other":         {a: base, b: nil, want: false},
		"different_name":    {a: base, b: &Device{name: "dm-1", path: "/dev/dm-0", size: 1024, slaves: []string{"sda", "sdb"}, holders: []string{"dm-1"}, dmInfo: &DMDevice{uuid: "mpath-123", name: "mpathA"}}, want: false},
		"different_size":    {a: base, b: &Device{name: "dm-0", path: "/dev/dm-0", size: 2048, slaves: []string{"sda", "sdb"}, holders: []string{"dm-1"}, dmInfo: &DMDevice{uuid: "mpath-123", name: "mpathA"}}, want: false},
		"different_slaves":  {a: base, b: &Device{name: "dm-0", path: "/dev/dm-0", size: 1024, slaves: []string{"sdc"}, holders: []string{"dm-1"}, dmInfo: &DMDevice{uuid: "mpath-123", name: "mpathA"}}, want: false},
		"nil_vs_non_nil_dm": {a: &Device{name: "sda"}, b: &Device{name: "sda", dmInfo: &DMDevice{uuid: "x"}}, want: false},
		"both_nil_dm":       {a: &Device{name: "sda", path: "/dev/sda"}, b: &Device{name: "sda", path: "/dev/sda"}, want: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.a.IsEqual(tc.b))
		})
	}
}
