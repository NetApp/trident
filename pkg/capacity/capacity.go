// Copyright 2025 NetApp, Inc. All Rights Reserved.

package capacity

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/pkg/maths"
)

const (
	OneGiB = uint64(1073741824)
)

// ///////////////////////////////////////////////////////////////////////////
//
// Binary units
//
// ///////////////////////////////////////////////////////////////////////////

type sizeUnit2 []string

func (s sizeUnit2) Len() int {
	return len(s)
}

func (s sizeUnit2) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s sizeUnit2) Less(i, j int) bool {
	return len(s[i]) > len(s[j])
}

var (
	lookupTable2 = make(map[string]int)
	units2       = sizeUnit2{}
)

func init() {
	// populate the lookup table for binary suffixes
	lookupTable2["k"] = 1
	lookupTable2["ki"] = 1
	lookupTable2["kib"] = 1
	lookupTable2["m"] = 2
	lookupTable2["mi"] = 2
	lookupTable2["mib"] = 2
	lookupTable2["g"] = 3
	lookupTable2["gi"] = 3
	lookupTable2["gib"] = 3
	lookupTable2["t"] = 4
	lookupTable2["ti"] = 4
	lookupTable2["tib"] = 4
	lookupTable2["p"] = 5
	lookupTable2["pi"] = 5
	lookupTable2["pib"] = 5
	lookupTable2["e"] = 6
	lookupTable2["ei"] = 6
	lookupTable2["eib"] = 6
	lookupTable2["z"] = 7
	lookupTable2["zi"] = 7
	lookupTable2["zib"] = 7
	lookupTable2["y"] = 8
	lookupTable2["yi"] = 8
	lookupTable2["yib"] = 8

	// The slice of units is used to ensure that they are accessed by suffix from longest to
	// shortest, i.e. match 'tib' before matching 'b'.
	for unit := range lookupTable2 {
		units2 = append(units2, unit)
	}
	sort.Sort(units2)
}

// ///////////////////////////////////////////////////////////////////////////
//
// SI units
//
// ///////////////////////////////////////////////////////////////////////////

type sizeUnit10 []string

func (s sizeUnit10) Len() int {
	return len(s)
}

func (s sizeUnit10) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s sizeUnit10) Less(i, j int) bool {
	return len(s[i]) > len(s[j])
}

var (
	lookupTable10 = make(map[string]int)
	units10       = sizeUnit10{}
)

func init() {
	// populate the lookup table for SI suffixes
	lookupTable10["b"] = 0
	lookupTable10["bytes"] = 0
	lookupTable10["kb"] = 1
	lookupTable10["mb"] = 2
	lookupTable10["gb"] = 3
	lookupTable10["tb"] = 4
	lookupTable10["pb"] = 5
	lookupTable10["eb"] = 6
	lookupTable10["zb"] = 7
	lookupTable10["yb"] = 8

	// The slice of units is used to ensure that they are accessed by suffix from longest to
	// shortest, i.e. match 'tib' before matching 'b'.
	for unit := range lookupTable10 {
		units10 = append(units10, unit)
	}
	sort.Sort(units10)
}

// ToBytes converts size to bytes; see also https://en.wikipedia.org/wiki/Kilobyte
func ToBytes(s string) (string, error) {
	// make lowercase so units detection always works
	s = strings.TrimSpace(strings.ToLower(s))

	// first look for binary units
	for _, unit := range units2 {
		if strings.HasSuffix(s, unit) {
			s = strings.TrimSuffix(s, unit)
			if i, err := strconv.ParseInt(s, 10, 0); err != nil {
				return "", fmt.Errorf("invalid size value '%s': %v", s, err)
			} else {
				i = i * maths.Pow(1024, lookupTable2[unit])
				s = strconv.FormatInt(i, 10)
				return s, nil
			}
		}
	}

	// fall back to SI units
	for _, unit := range units10 {
		if strings.HasSuffix(s, unit) {
			s = strings.TrimSuffix(s, unit)
			if i, err := strconv.ParseInt(s, 10, 0); err != nil {
				return "", fmt.Errorf("invalid size value '%s': %v", s, err)
			} else {
				i = i * maths.Pow(1000, lookupTable10[unit])
				s = strconv.FormatInt(i, 10)
				return s, nil
			}
		}
	}

	// no valid units found, so ensure the value is a number
	if _, err := strconv.ParseUint(s, 10, 64); err != nil {
		return "", fmt.Errorf("invalid size value '%s': %v", s, err)
	}

	return s, nil
}

// sizeHasUnits checks whether a size string includes a units suffix.
func sizeHasUnits(size string) bool {
	// make lowercase so units detection always works
	size = strings.TrimSpace(strings.ToLower(size))

	for _, unit := range units2 {
		if strings.HasSuffix(size, unit) {
			return true
		}
	}
	for _, unit := range units10 {
		if strings.HasSuffix(size, unit) {
			return true
		}
	}
	return false
}

// GetVolumeSizeBytes determines the size, in bytes, of a volume from the "size" opt value.  If "size" has a units
// suffix, that is handled here.  If there are no units, the default is GiB.  If size is not in opts, the specified
// default value is parsed identically and used instead.
func GetVolumeSizeBytes(ctx context.Context, opts map[string]string, defaultVolumeSize string) (uint64, error) {
	usingDefaultSize := false
	usingDefaultUnits := false

	// Use the size if specified, else use the configured default size
	size := collection.GetV(opts, "size", "")
	if size == "" {
		size = defaultVolumeSize
		usingDefaultSize = true
	}

	// Default to GiB if no units are present
	if !sizeHasUnits(size) {
		size += "G"
		usingDefaultUnits = true
	}

	// Ensure the size is valid
	sizeBytesStr, err := ToBytes(size)
	if err != nil {
		return 0, err
	}
	sizeBytes, _ := strconv.ParseUint(sizeBytesStr, 10, 64)

	logging.Logc(ctx).WithFields(logging.LogFields{
		"sizeBytes":         sizeBytes,
		"size":              size,
		"usingDefaultSize":  usingDefaultSize,
		"usingDefaultUnits": usingDefaultUnits,
	}).Debug("Determined volume size.")

	return sizeBytes, nil
}

// VolumeSizeWithinTolerance checks to see if requestedSize is within the delta of the currentSize.
// If within the delta true is returned. If not within the delta and requestedSize is less than the
// currentSize false is returned.
func VolumeSizeWithinTolerance(requestedSize, currentSize, delta int64) bool {
	sizeDiff := requestedSize - currentSize
	if sizeDiff < 0 {
		sizeDiff = -sizeDiff
	}

	if sizeDiff <= delta {
		return true
	}
	return false
}
