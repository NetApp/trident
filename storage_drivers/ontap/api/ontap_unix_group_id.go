// Copyright 2026 NetApp, Inc. All Rights Reserved.

package api

import (
	"fmt"
	"strconv"

	"github.com/netapp/trident/utils/errors"
)

// MaxUnixGroupID is the largest valid Unix group ID. ONTAP accepts group IDs in the
// range [0, MaxUnixGroupID] (2^31-1).
const MaxUnixGroupID = 2147483647

func invalidUnixGroupIDError(unixGroupID string) error {
	return errors.InvalidInputError(fmt.Sprintf(
		"Invalid unixGroupID %q: must be an integer in the range [0, %d]",
		unixGroupID, MaxUnixGroupID))
}

// ParseUnixGroupID validates and parses a Unix group ID supplied by the application (e.g. via a PVC
// annotation). An empty string means "not specified": Trident leaves the group ID unset and lets
// ONTAP apply its default behaviour, so it returns (0, false, nil). A non-empty value must be a
// base-10 unsigned integer in the range [0, MaxUnixGroupID]; otherwise an InvalidInputError is
// returned so the caller can fail fast with a clear message.
func ParseUnixGroupID(unixGroupID string) (gid int, set bool, err error) {
	if unixGroupID == "" {
		return 0, false, nil
	}

	parsed, parseErr := strconv.Atoi(unixGroupID)
	if parseErr != nil {
		return 0, false, invalidUnixGroupIDError(unixGroupID)
	}

	if parsed < 0 || parsed > MaxUnixGroupID {
		return 0, false, invalidUnixGroupIDError(unixGroupID)
	}

	return parsed, true, nil
}
