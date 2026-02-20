// Copyright 2025 NetApp, Inc. All Rights Reserved.

package capacity

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"

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

// ///////////////////////////////////////////////////////////////////////////
//
// Human-readable quantity formatting
//
// ///////////////////////////////////////////////////////////////////////////

const (
	kiB = int64(1 << 10)
	miB = int64(1 << 20)
	giB = int64(1 << 30)

	// HumanReadableDecimals is the number of decimal places used when formatting
	// sizes (e.g. "20.74Gi"). We round up so the requested size is never less than the computed value.
	HumanReadableDecimals = 2
)

// roundUpToDecimals rounds val up to the given number of decimal places
// (e.g. 20.736 → 20.74 with 2 decimals) so we never request less than the original value.
func roundUpToDecimals(val float64, decimals int) float64 {
	if decimals <= 0 {
		return math.Ceil(val)
	}
	mult := math.Pow(10, float64(decimals))
	return math.Ceil(val*mult) / mult
}

// QuantityToHumanReadableString returns a human-readable decimal string (e.g. "20.74Gi", "512.50Mi").
// Values are rounded up to HumanReadableDecimals (2) so we never request less than the original.
// Example: 22265110461 bytes → "20.74Gi" (20.735999... GiB rounded up to 2 decimals).
func QuantityToHumanReadableString(q resource.Quantity) string {
	bytes := q.Value()
	if bytes < 0 {
		return q.String()
	}
	if bytes == 0 {
		return "0"
	}
	d := HumanReadableDecimals
	switch {
	case bytes >= giB:
		valGi := float64(bytes) / float64(giB)
		valGi = roundUpToDecimals(valGi, d)
		return fmt.Sprintf("%.*fGi", d, valGi)
	case bytes >= miB:
		valMi := float64(bytes) / float64(miB)
		valMi = roundUpToDecimals(valMi, d)
		return fmt.Sprintf("%.*fMi", d, valMi)
	case bytes >= kiB:
		valKi := float64(bytes) / float64(kiB)
		valKi = roundUpToDecimals(valKi, d)
		return fmt.Sprintf("%.*fKi", d, valKi)
	default:
		return fmt.Sprintf("%d", bytes)
	}
}

// Decimal size units (SI: 1000-based) for display of byte counts.
const (
	decimalGB = 1e9
	decimalMB = 1e6
	decimalKB = 1e3
)

// BytesToDecimalSizeString formats a byte count in decimal (SI) units for display, e.g. "51MB", "1.50GB".
func BytesToDecimalSizeString(bytes int64) string {
	if bytes < 0 {
		return fmt.Sprintf("%d", bytes)
	}
	if bytes == 0 {
		return "0"
	}
	switch {
	case bytes >= decimalGB:
		val := roundToTwoDecimals(float64(bytes) / float64(decimalGB))
		return formatDecimalSize(val, "GB")
	case bytes >= decimalMB:
		val := roundToTwoDecimals(float64(bytes) / float64(decimalMB))
		return formatDecimalSize(val, "MB")
	case bytes >= decimalKB:
		val := roundToTwoDecimals(float64(bytes) / float64(decimalKB))
		return formatDecimalSize(val, "KB")
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func formatDecimalSize(val float64, unit string) string {
	if val == float64(int64(val)) {
		return fmt.Sprintf("%d%s", int64(val), unit)
	}
	return fmt.Sprintf("%.2f%s", val, unit)
}

// roundToTwoDecimals rounds to nearest 2 decimal places (for display, e.g. "1.50GB").
// For sizing we use roundUpToDecimals so we never request less than the computed value.
func roundToTwoDecimals(val float64) float64 {
	return math.Round(val*100) / 100
}

// ///////////////////////////////////////////////////////////////////////////
//
// Quantity to bytes (floor)
//
// ///////////////////////////////////////////////////////////////////////////

// ParseQuantityFloorBytes parses a quantity string and returns the floor of its value in bytes.
// Returns 0 if the string is empty or invalid. (e.g. "1.7Gi" → 1825361100, instead of 1825361101).
func ParseQuantityFloorBytes(s string) uint64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	qty, err := resource.ParseQuantity(s)
	if err != nil || qty.Sign() < 0 {
		return 0
	}
	return uint64(math.Floor(qty.AsApproximateFloat64()))
}

// QuantityFloorBytes returns the floor of the quantity's value in bytes. Returns 0 if qty is nil or invalid.
func QuantityFloorBytes(qty *resource.Quantity) uint64 {
	if qty == nil || qty.Sign() < 0 {
		return 0
	}
	return uint64(math.Floor(qty.AsApproximateFloat64()))
}
