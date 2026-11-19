// Copyright 2026 NetApp, Inc. All Rights Reserved.

package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapperName(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
	}{
		"fullPath":  {in: "/dev/mapper/luks-vol", want: "luks-vol"},
		"nameOnly":  {in: "luks-vol", want: "luks-vol"},
		"emptyPath": {in: "/dev/mapper/", want: ""},
		"empty":     {in: "", want: ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, MapperName(tc.in))
		})
	}
}

func TestMapperPath(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
	}{
		"nameOnly":    {in: "luks-vol", want: "/dev/mapper/luks-vol"},
		"alreadyPath": {in: "/dev/mapper/luks-vol", want: "/dev/mapper/luks-vol"},
		"empty":       {in: "", want: "/dev/mapper/"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, MapperPath(tc.in))
		})
	}
}

func TestDMDeviceIsEqual(t *testing.T) {
	a := &DMDevice{uuid: "mpath-123", name: "mpathA"}
	identical := &DMDevice{uuid: "mpath-123", name: "mpathA"}

	cases := map[string]struct {
		a, b *DMDevice
		want bool
	}{
		"same_pointer":    {a: a, b: a, want: true},
		"identical_value": {a: a, b: identical, want: true},
		"both_nil":        {a: nil, b: nil, want: true},
		"nil_vs_non_nil":  {a: a, b: nil, want: false},
		"non_nil_vs_nil":  {a: nil, b: a, want: false},
		"different_uuid":  {a: a, b: &DMDevice{uuid: "mpath-999", name: "mpathA"}, want: false},
		"different_name":  {a: a, b: &DMDevice{uuid: "mpath-123", name: "mpathB"}, want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.a.IsEqual(tc.b))
		})
	}
}
