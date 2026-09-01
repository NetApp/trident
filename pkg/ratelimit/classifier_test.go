// Copyright 2026 NetApp, Inc. All Rights Reserved.

package ratelimit

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMessageSubstringDecreaseOnError(t *testing.T) {
	decreaseOnError := MessageSubstringDecreaseOnError(
		[]string{"rate_limit_exceeded", "api-requests", "api requests"},
		[]string{"flexvolumesperregion"},
	)

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"non-status error", errors.New("plain error"), false},
		{"unknown ResourceExhausted", status.Error(codes.ResourceExhausted, "something unrecognised"), false},
		{"RATE_LIMIT_EXCEEDED message", status.Error(codes.ResourceExhausted, "reason = RATE_LIMIT_EXCEEDED"), true},
		{"API requests message", status.Error(codes.ResourceExhausted, "quota metric 'API requests' per minute"), true},
		{"api-requests hyphen", status.Error(codes.ResourceExhausted, "quota: api-requests per minute exceeded"), true},
		{"FlexVolumesPerRegion message", status.Error(codes.ResourceExhausted, "FlexVolumesPerRegion exceeded"), false},
		{"exclude wins over include", status.Error(codes.ResourceExhausted, "flexvolumesperregion rate_limit_exceeded"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, decreaseOnError(tt.err))
		})
	}
}

// testAPIDecreaseOnError mirrors the GCNV include/exclude substrings for
// pkg/ratelimit unit tests without importing storage_drivers/gcp/api.
var testAPIDecreaseOnError = MessageSubstringDecreaseOnError(
	[]string{"rate_limit_exceeded", "api-requests", "api requests"},
	[]string{"flexvolumesperregion"},
)
