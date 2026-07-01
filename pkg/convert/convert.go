// Copyright 2025 NetApp, Inc. All Rights Reserved.

package convert

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/pkg/collection"
)

// TimestampFormat is a date time pattern used to convert times reported from various sources into a consistent
// date time formats.
const TimestampFormat = "2006-01-02T15:04:05Z"

// ToTitle minimally replaces the deprecated strings.Title() function.
func ToTitle(str string) string {
	return cases.Title(language.Und, cases.NoLower).String(str)
}

// ToPtr converts any value into a pointer to that value.
func ToPtr[T any](v T) *T {
	return &v
}

// ToVal converts any pointer to a value, into the value.
// If the supplied pointer is nil, the zero value of the type is returned.
func ToVal[T any](v *T) T {
	var zero T
	if v == nil {
		return zero
	}
	return *v
}

// PtrToString converts any value into its string representation, or nil
func PtrToString[T any](v *T) string {
	if v == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v", *v)
}

// ToSlicePtrs converts a slice into a slice of pointers
func ToSlicePtrs[T any](slice []T) []*T {
	var result []*T
	for _, s := range slice {
		result = append(result, new(s))
	}
	return result
}

// ToBool wraps strconv.ParseBool to suppress errors. Returns false if strconv.ParseBool would return an error.
func ToBool(b string) bool {
	v, _ := strconv.ParseBool(b)
	return v
}

// ToFormattedBool returns lowercase string value for a valid boolean value, else returns same value with error.
func ToFormattedBool(value string) (string, error) {
	valBool, err := strconv.ParseBool(value)
	if err != nil {
		return value, err
	}

	return strconv.FormatBool(valBool), nil
}

func ToPrintableBoolPtr(bPtr *bool) string {
	if bPtr != nil {
		if *bPtr {
			return "true"
		}
		return "false"
	}
	return "none"
}

func ObjectToBase64String(object any) (string, error) {
	if object == nil {
		return "", fmt.Errorf("cannot encode nil object")
	}

	// Serialize the object data to JSON
	bytes, err := json.Marshal(object)
	if err != nil {
		return "", fmt.Errorf("failed encode object; %v", object)
	}

	// Encode JSON bytes to a string
	return base64.StdEncoding.EncodeToString(bytes), nil
}

func Base64StringToObject(encodedObject string, destination any) error {
	if encodedObject == "" {
		return fmt.Errorf("cannot decode empty encoded string")
	}

	// Decode the data from a string
	bytes, err := base64.StdEncoding.DecodeString(encodedObject)
	if err != nil {
		return fmt.Errorf("failed to decode string; %s", encodedObject)
	}

	// Deserialize the bytes into the destination
	err = json.Unmarshal(bytes, &destination)
	if err != nil {
		return fmt.Errorf("failed to unmarshal bytes into destination of type: %t", reflect.TypeOf(destination))
	}
	return nil
}

// --- Numeric narrowing (value → value) ---

// Int64ToInt32 converts v to int32 after verifying it fits.
func Int64ToInt32(v int64) (int32, error) {
	if v < math.MinInt32 || v > math.MaxInt32 {
		return 0, fmt.Errorf("value %d overflows int32", v)
	}
	return int32(v), nil
}

// Int64ToInt converts v to int after verifying it fits.
func Int64ToInt(v int64) (int, error) {
	if v < math.MinInt || v > math.MaxInt {
		return 0, fmt.Errorf("value %d overflows int", v)
	}
	return int(v), nil
}

// Int64ToUint64 converts non-negative v to uint64.
func Int64ToUint64(v int64) (uint64, error) {
	if v < 0 {
		return 0, fmt.Errorf("value %d overflows uint64", v)
	}
	return uint64(v), nil
}

// Uint64ToInt64 converts v to int64 after verifying it fits.
func Uint64ToInt64(v uint64) (int64, error) {
	if v > math.MaxInt64 {
		return 0, fmt.Errorf("value %d overflows int64", v)
	}
	return int64(v), nil
}

// Uint64ToInt converts v to int after verifying it fits.
func Uint64ToInt(v uint64) (int, error) {
	if v > math.MaxInt {
		return 0, fmt.Errorf("value %d overflows int", v)
	}
	return int(v), nil
}

// Uint64ToInt32 converts v to int32 after verifying it fits.
func Uint64ToInt32(v uint64) (int32, error) {
	if v > math.MaxInt32 {
		return 0, fmt.Errorf("value %d overflows int32", v)
	}
	return int32(v), nil
}

// IntToInt32 converts v to int32 after verifying it fits.
func IntToInt32(v int) (int32, error) {
	if v < math.MinInt32 || v > math.MaxInt32 {
		return 0, fmt.Errorf("value %d overflows int32", v)
	}
	return int32(v), nil
}

// IntToByte converts v to byte after verifying it fits.
func IntToByte(v int) (byte, error) {
	if v < 0 || v > math.MaxUint8 {
		return 0, fmt.Errorf("value %d overflows byte", v)
	}
	return byte(v), nil
}

// IntToUint64 converts non-negative v to uint64.
func IntToUint64(v int) (uint64, error) {
	if v < 0 {
		return 0, fmt.Errorf("value %d overflows uint64", v)
	}
	return uint64(v), nil
}

// Int32ToUint64 converts non-negative v to uint64.
func Int32ToUint64(v int32) (uint64, error) {
	if v < 0 {
		return 0, fmt.Errorf("value %d overflows uint64", v)
	}
	return uint64(v), nil
}

// --- String parsing (string → value) ---

func parseIntInRange(s string, min, max int64) (int64, error) {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	if i < min || i > max {
		return 0, fmt.Errorf("value %s is out of range [%d, %d]", s, min, max)
	}
	return i, nil
}

func parseUintInRange(s string, min, max uint64) (uint64, error) {
	u, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, err
	}
	if u < min || u > max {
		return 0, fmt.Errorf("value %s is out of range [%d, %d]", s, min, max)
	}
	return u, nil
}

// ToInt64 parses s as a base-10 int64.
func ToInt64(s string) (int64, error) {
	return parseIntInRange(s, math.MinInt64, math.MaxInt64)
}

// ToUint64 parses s as a base-10 uint64.
func ToUint64(s string) (uint64, error) {
	return parseUintInRange(s, 0, math.MaxUint64)
}

// ToPositiveInt32 parses s into an int32 and returns an error if it is negative or out of range.
func ToPositiveInt32(s string) (int32, error) {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	if i >= 0 && i <= math.MaxInt32 {
		return int32(i), nil
	}
	return 0, fmt.Errorf("value %s is out of range [0, %d]", s, math.MaxInt32)
}

// ToPositiveInt64 parses s into an int64 and returns an error if it is negative.
func ToPositiveInt64(s string) (int64, error) {
	return parseIntInRange(s, 0, math.MaxInt64)
}

// ToPositiveInt parses s into a platform-dependent int and returns an error if it is negative or out of range.
func ToPositiveInt(s string) (int, error) {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	if i >= 0 && i <= math.MaxInt {
		return int(i), nil
	}
	return 0, fmt.Errorf("value %s is out of range [0, %d]", s, math.MaxInt)
}

// ToStringRedacted identifies attributes of a struct, stringifies them such that they can be consumed by the
// struct stringer interface, and redacts elements specified in the redactList.
func ToStringRedacted(structPointer interface{}, redactList []string, configVal interface{}) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = fmt.Sprintf("<panic> in convert#ToStringRedacted; err: %v", r)
		}
	}()

	elements := reflect.ValueOf(structPointer).Elem()

	var output strings.Builder

	for i := 0; i < elements.NumField(); i++ {
		fieldName := elements.Type().Field(i).Name
		switch {
		case fieldName == "Config" && configVal != nil:
			output.WriteString(fmt.Sprintf("%v:%v ", fieldName, configVal))
		case collection.ContainsString(redactList, fieldName):
			output.WriteString(fmt.Sprintf("%v:%v ", fieldName, config.REDACTED))
		default:
			output.WriteString(fmt.Sprintf("%v:%#v ", fieldName, elements.Field(i)))
		}
	}

	out = output.String()
	return
}

func RedactSecretsFromString(stringToSanitize string, replacements map[string]string, useRegex bool) string {
	compileError := "regex matching the secret could not compile, so the entire string has been redacted"

	for key, value := range replacements {
		if useRegex {
			pattern, err := regexp.Compile(key)
			if err != nil {
				return compileError
			}
			stringToSanitize = pattern.ReplaceAllString(stringToSanitize, value)
		} else {
			stringToSanitize = strings.ReplaceAll(stringToSanitize, key, value)
		}
	}

	return stringToSanitize
}

// TruncateString returns the specified string, shortened by dropping characters on the right side to the given limit.
func TruncateString(s string, maxLength int) string {
	if len(s) > maxLength {
		return s[:maxLength]
	}
	return s
}
