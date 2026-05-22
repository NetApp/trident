// Copyright 2026 NetApp, Inc. All Rights Reserved.

package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpandPolicyForVolumeListQuery(t *testing.T) {
	tests := []struct {
		name   string
		policy string
		want   string
	}{
		{
			name:   "empty policy is unchanged",
			policy: "",
			want:   "",
		},
		{
			name:   "already pipe-separated is unchanged",
			policy: "default|default-DR",
			want:   "default|default-DR",
		},
		{
			name:   "policy containing pipe elsewhere is unchanged",
			policy: "a|b",
			want:   "a|b",
		},
		{
			name:   "snapshot policy expands to DR variant",
			policy: "default",
			want:   "default|default-DR",
		},
		{
			name:   "DR suffix snapshot policy expands to base name",
			policy: "default-DR",
			want:   "default-DR|default",
		},
		{
			name:   "DR suffix only in middle not treated as DR variant",
			policy: "my-DR-policy",
			want:   "my-DR-policy|my-DR-policy-DR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, expandPolicyForVolumeListQuery(tt.policy))
		})
	}
}
