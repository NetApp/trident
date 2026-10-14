// Copyright 2026 NetApp, Inc. All Rights Reserved.

package sgapi

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
)

// StorageGRID's field limits, which the COSI names do not line up with.
const (
	// MinBucketNameLength and MaxBucketNameLength bound a bucket name. COSI
	// supplies names of up to 253 characters, so the 63-character ceiling is a
	// real limit rather than a theoretical one: it must be caught locally so the
	// caller gets a clear error instead of an opaque rejection from the grid.
	MinBucketNameLength = 3
	MaxBucketNameLength = 63

	// MinRegionNameLength and MaxRegionNameLength bound a region name.
	MinRegionNameLength = 2
	MaxRegionNameLength = 32

	// MaxAccountNameLength is the ceiling COSI itself applies to account_name,
	// which is a DNS subdomain. It fits inside the 1024-character unique name
	// limit with room to spare, so no truncation is needed there.
	MaxAccountNameLength = 253

	// MaxDisplayNameLength and MaxFullNameLength bound a group's display name
	// and a user's full name. Both are far shorter than an account name, which
	// is why those fields are derived rather than copied.
	MaxDisplayNameLength = 32
	MaxFullNameLength    = 128
)

var (
	ipv4Pattern       = regexp.MustCompile(`^\d{1,3}(\.\d{1,3}){3}$`)
	regionNamePattern = regexp.MustCompile(`^[-a-zA-Z0-9]+$`)
)

// ValidateBucketName checks a name against StorageGRID's bucket constraints
// before any network call is made. Note that bucket names are unique across the
// entire grid rather than per tenant, which no local check can anticipate.
func ValidateBucketName(name string) error {
	invalid := func(reason string) error {
		return &ValidationError{Field: "bucket name", Value: name, Reason: reason}
	}

	if name == "" {
		return &ValidationError{Field: "bucket name", Reason: "is required"}
	}
	if len(name) < MinBucketNameLength || len(name) > MaxBucketNameLength {
		return invalid(fmt.Sprintf("must be %d to %d characters, but is %d",
			MinBucketNameLength, MaxBucketNameLength, len(name)))
	}
	if strings.ToLower(name) != name {
		return invalid("must be lowercase")
	}
	for i := 0; i < len(name); i++ {
		if !isBucketNameChar(name[i]) {
			return invalid(fmt.Sprintf(
				"contains %q; only lowercase letters, digits, hyphens, and dots are allowed",
				string(name[i])))
		}
	}
	if !isAlphanumeric(name[0]) || !isAlphanumeric(name[len(name)-1]) {
		return invalid("must start and end with a letter or a digit")
	}
	if strings.Contains(name, "..") {
		return invalid("must not contain consecutive dots")
	}
	if strings.Contains(name, ".-") || strings.Contains(name, "-.") {
		return invalid("must not contain a dot next to a hyphen")
	}
	if ipv4Pattern.MatchString(name) {
		return invalid("must not be formatted as an IP address")
	}
	return nil
}

// ValidateRegionName checks a region locally, which catches a malformed
// configuration without touching the grid. Whether the region actually exists is
// deliberately left to the grid: regions cannot be created through the tenant
// API, and a cached allowlist would reject a region an administrator has just
// added.
func ValidateRegionName(region string) error {
	invalid := func(reason string) error {
		return &ValidationError{Field: "region", Value: region, Reason: reason}
	}

	if region == "" {
		return &ValidationError{Field: "region", Reason: "is required"}
	}
	if len(region) < MinRegionNameLength || len(region) > MaxRegionNameLength {
		return invalid(fmt.Sprintf("must be %d to %d characters, but is %d",
			MinRegionNameLength, MaxRegionNameLength, len(region)))
	}
	if !regionNamePattern.MatchString(region) {
		return invalid("may contain only letters, digits, and hyphens")
	}
	return nil
}

// ValidateAccountName checks a COSI account name. Unlike a bucket name this has
// no StorageGRID length problem, because it is only ever written to fields that
// hold 1024 characters, so the check is just that it is the DNS subdomain COSI
// promises to send.
func ValidateAccountName(name string) error {
	invalid := func(reason string) error {
		return &ValidationError{Field: "account name", Value: name, Reason: reason}
	}

	if name == "" {
		return &ValidationError{Field: "account name", Reason: "is required"}
	}
	if len(name) > MaxAccountNameLength {
		return invalid(fmt.Sprintf("must be at most %d characters, but is %d",
			MaxAccountNameLength, len(name)))
	}
	if strings.ToLower(name) != name {
		return invalid("must be lowercase")
	}
	for i := 0; i < len(name); i++ {
		if !isBucketNameChar(name[i]) {
			return invalid(fmt.Sprintf(
				"contains %q; only lowercase letters, digits, hyphens, and dots are allowed",
				string(name[i])))
		}
	}
	if !isAlphanumeric(name[0]) || !isAlphanumeric(name[len(name)-1]) {
		return invalid("must start and end with a letter or a digit")
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" {
			return invalid("must not contain an empty label")
		}
		if len(label) > 63 {
			return invalid(fmt.Sprintf("label %q is longer than 63 characters", label))
		}
	}
	return nil
}

// DerivedIdentityName builds the display name for a group or the full name for a
// user. Neither field is a uniqueness key, and both are far shorter than an
// account name, so a short hash of the account name is enough: it is
// deterministic, which is what a re-drive needs, and it stays inside the
// 32-character display name limit.
func DerivedIdentityName(accountName string) string {
	sum := sha256.Sum256([]byte(accountName))
	return fmt.Sprintf("cosi-%x", sum[:4])
}

func isBucketNameChar(ch byte) bool {
	return isAlphanumeric(ch) || ch == '-' || ch == '.'
}

func isAlphanumeric(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
}
