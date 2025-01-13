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
	"github.com/netapp/trident/logging"
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
		result = append(result, ToPtr(s))
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

// ToPositiveInt32 parses s into an int32 and returns an error if it is negative.
func ToPositiveInt32(s string) (int32, error) {
	i, err := parseIntInRange(s, 0, math.MaxInt32)
	return int32(i), err
}

// ToPositiveInt64 parses s into an int64 and returns an error if it is negative.
func ToPositiveInt64(s string) (int64, error) {
	return parseIntInRange(s, 0, math.MaxInt64)
}

// ToPositiveInt parses s into a platform-dependent int and returns an error if it is negative.
func ToPositiveInt(s string) (int, error) {
	i, err := parseIntInRange(s, 0, math.MaxInt)
	return int(i), err
}

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

// ToStringRedacted identifies attributes of a struct, stringifies them such that they can be consumed by the
// struct stringer interface, and redacts elements specified in the redactList.
func ToStringRedacted(structPointer interface{}, redactList []string, configVal interface{}) (out string) {
	defer func() {
		if r := recover(); r != nil {
			logging.Log().Errorf("Panic in utils#ToStringRedacted; err: %v", r)
			out = "<panic>"
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
