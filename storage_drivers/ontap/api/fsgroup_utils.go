// Copyright 2025 NetApp, Inc. All Rights Reserved.

package api

import (
	"fmt"
	"strconv"
)

const (
	// minUnixGroupID and maxUnixGroupID define the inclusive range of valid UNIX group IDs (GIDs)
	// that may be applied to an ONTAP NAS volume.
	minUnixGroupID = 1
	maxUnixGroupID = 4294967294
)

// parseUnixGroupID parses a UNIX group ID (GID) string for the REST path, returning an int64.
// An empty string is treated as "not specified" and returns 0 with no error, meaning no group ID
// is set on the volume. Any non-numeric value, or a value outside the inclusive range
// 1-4294967294, returns an error.
func parseUnixGroupID(unixGroupID string) (int64, error) {
	if unixGroupID == "" {
		return 0, nil
	}

	gid, err := strconv.ParseInt(unixGroupID, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid UNIX group ID %q: %w", unixGroupID, err)
	}

	if gid < minUnixGroupID || gid > maxUnixGroupID {
		return 0, fmt.Errorf("UNIX group ID %d is out of range (must be between %d and %d)",
			gid, minUnixGroupID, maxUnixGroupID)
	}

	return gid, nil
}

// parseUnixGroupIDInt parses a UNIX group ID (GID) string for the ZAPI path, returning an int.
// An empty string is treated as "not specified" and returns 0 with no error, meaning no group ID
// is set on the volume. Any non-numeric value, or a value outside the inclusive range
// 1-4294967294, returns an error.
func parseUnixGroupIDInt(unixGroupID string) (int, error) {
	if unixGroupID == "" {
		return 0, nil
	}

	gid, err := strconv.Atoi(unixGroupID)
	if err != nil {
		return 0, fmt.Errorf("invalid UNIX group ID %q: %w", unixGroupID, err)
	}

	if gid < minUnixGroupID || gid > maxUnixGroupID {
		return 0, fmt.Errorf("UNIX group ID %d is out of range (must be between %d and %d)",
			gid, minUnixGroupID, maxUnixGroupID)
	}

	return gid, nil
}
