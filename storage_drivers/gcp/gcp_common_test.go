// Copyright 2026 NetApp, Inc. All Rights Reserved.

package gcp

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/storage_drivers/gcp/api"
)

func TestGCPCommon_DefaultCreateTimeout(t *testing.T) {
	tests := []struct {
		name     string
		ctx      tridentconfig.DriverContext
		expected time.Duration
	}{
		{"Docker", tridentconfig.ContextDocker, tridentconfig.DockerCreateTimeout},
		{"CSI", tridentconfig.ContextCSI, api.VolumeCreateTimeout},
		{"Empty", tridentconfig.DriverContext(""), api.VolumeCreateTimeout},
		{"Unknown", tridentconfig.DriverContext("k8s"), api.VolumeCreateTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultCreateTimeout(tt.ctx)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestGCPCommon_FixGCPLabelKey(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantOut string
		wantOK  bool
	}{
		{"Empty", "", "", false},
		{"Valid lowercase", "valid_key", "valid_key", true},
		{"Uppercase normalized", "ValidKey", "validkey", true},
		{"Starts with digit", "1key", "", false},
		{"Starts with hyphen", "-key", "", false},
		{"Disallowed chars replaced", "key.with.dots", "key_with_dots", true},
		{"Spaces to underscores", "key with spaces", "key_with_spaces", true},
		{"Unicode letter allowed", "këy", "këy", true},
		{"Multibyte lowercase first rune", "émoji", "émoji", true},
		{"Too long truncated", string(make([]byte, 70)), "", false}, // all zeros, first rune not lower
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOut, gotOK := FixGCPLabelKey(tt.in)
			assert.Equal(t, tt.wantOK, gotOK, "ok")
			if tt.wantOK {
				assert.Equal(t, tt.wantOut, gotOut)
				assert.True(t, len(gotOut) <= api.MaxLabelLength, "length <= MaxLabelLength")
			}
		})
	}
	// Key that is long but valid (starts with letter) gets truncated to 63
	longKey := "a" + string(make([]byte, api.MaxLabelLength+10))
	out, ok := FixGCPLabelKey(longKey)
	assert.True(t, ok)
	assert.Equal(t, api.MaxLabelLength, len(out))
}

func TestGCPCommon_FixGCPLabelValue(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantOut string
	}{
		{"Empty", "", ""},
		{"Valid", "value-123", "value-123"},
		{"Uppercase normalized", "Value", "value"},
		{"Dots replaced", "v.a.l", "v_a_l"},
		{"Spaces replaced", "a b c", "a_b_c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FixGCPLabelValue(tt.in)
			assert.Equal(t, tt.wantOut, got)
			if len(got) > 0 {
				assert.True(t, len(got) <= api.MaxLabelLength, "length <= MaxLabelLength")
			}
		})
	}
	// Long value truncated
	longVal := "v" + string(make([]byte, api.MaxLabelLength+5))
	got := FixGCPLabelValue(longVal)
	assert.Equal(t, api.MaxLabelLength, len(got))
}

func TestGCPCommon_ErrRefreshGCNVResourceCache(t *testing.T) {
	err := errors.New("connection refused")
	wrapped := ErrRefreshGCNVResourceCache(err)
	assert.Error(t, wrapped)
	assert.Contains(t, wrapped.Error(), "could not update GCNV resource cache")
	assert.Contains(t, wrapped.Error(), "connection refused")
}
