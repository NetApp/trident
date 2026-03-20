// Copyright 2026 NetApp, Inc. All Rights Reserved.

package gcp

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage_drivers/gcp/api"
)

var (
	// gcpLabelRegex matches characters that are not allowed in GCP label keys or values.
	// Allowed: lowercase letters, digits, hyphens, underscores, and Unicode letters (\p{L}).
	gcpLabelRegex = regexp.MustCompile(`[^-_a-z0-9\p{L}]`)
)

// DefaultCreateTimeout returns the volume create/delete timeout for the given driver context.
// Docker gets more time since it has no retry mechanism; CSI uses the API default.
func DefaultCreateTimeout(driverContext tridentconfig.DriverContext) time.Duration {
	switch driverContext {
	case tridentconfig.ContextDocker:
		return tridentconfig.DockerCreateTimeout
	default:
		return api.VolumeCreateTimeout
	}
}

// FixGCPLabelKey accepts a label key and modifies it to satisfy GCP label key rules, or returns
// false if not possible.
func FixGCPLabelKey(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	s = strings.ToLower(s)
	s = gcpLabelRegex.ReplaceAllStringFunc(s, func(m string) string {
		return strings.Repeat("_", len(m))
	})
	first, _ := utf8.DecodeRuneInString(s)
	if first == utf8.RuneError || !unicode.IsLower(first) {
		return "", false
	}
	s = convert.TruncateString(s, api.MaxLabelLength)
	return s, true
}

// FixGCPLabelValue accepts a label value and modifies it to satisfy GCP label value rules.
func FixGCPLabelValue(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ToLower(s)
	s = gcpLabelRegex.ReplaceAllStringFunc(s, func(m string) string {
		return strings.Repeat("_", len(m))
	})
	s = convert.TruncateString(s, api.MaxLabelLength)
	return s
}

// ErrRefreshGCNVResourceCache returns a standard error when refreshing the GCNV resource cache fails.
func ErrRefreshGCNVResourceCache(err error) error {
	return fmt.Errorf("could not update GCNV resource cache; %w", err)
}
