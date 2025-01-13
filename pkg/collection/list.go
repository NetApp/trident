// Copyright 2025 NetApp, Inc. All Rights Reserved.

package collection

import (
	"context"
	"fmt"
	"reflect"
	"strings"
)

// Contains checks to see if a slice (genericSlice) contains an item (genericElement)
func Contains(genericSlice, genericElement interface{}) bool {
	slice, elem := reflect.ValueOf(genericSlice), reflect.ValueOf(genericElement)

	if slice.Kind() != reflect.Slice && slice.Kind() != reflect.Array {
		return false
	}

	if slice.Len() > 1 {
		if slice.Index(0).Type() != elem.Type() {
			return false
		}
	}

	for i := 0; i < slice.Len(); i++ {
		if elem.Interface() == slice.Index(i).Interface() {
			return true
		}
	}

	return false
}

// ContainsString checks to see if a []string contains a string
func ContainsString(slice []string, s string) bool {
	return ContainsStringConditionally(slice, s, func(val1, val2 string) bool { return val1 == val2 })
}

// ContainsStringCaseInsensitive is ContainsString but case-insensitive.
func ContainsStringCaseInsensitive(slice []string, s string) bool {
	matchFunc := func(main, val string) bool {
		return strings.EqualFold(main, val)
	}

	return ContainsStringConditionally(slice, s, matchFunc)
}

// ContainsStringConditionally checks to see if a []string contains a string based on certain criteria
func ContainsStringConditionally(slice []string, s string, fn func(string, string) bool) bool {
	for _, item := range slice {
		if fn(item, s) {
			return true
		}
	}
	return false
}

// ContainsElements checks to see if a slice (genericSlice) contains a list of all/some items (genericElements)
func ContainsElements(genericSlice, genericElements interface{}) (bool, bool) {
	slice, elem := reflect.ValueOf(genericSlice), reflect.ValueOf(genericElements)

	if slice.Kind() != reflect.Slice && slice.Kind() != reflect.Array {
		return false, false
	}

	if elem.Kind() != reflect.Slice && elem.Kind() != reflect.Array {
		return false, false
	}

	containAll := true
	containSome := false
	for i := 0; i < elem.Len(); i++ {
		if Contains(genericSlice, elem.Index(i).Interface()) {
			containSome = true
		} else {
			containAll = false
		}
	}

	if !containSome {
		containAll = false
	}

	return containAll, containSome
}

// RemoveString removes a string from a []string.
func RemoveString(slice []string, s string) (result []string) {
	return RemoveStringConditionally(slice, s, func(val1, val2 string) bool { return val1 == val2 })
}

// RemoveStringConditionally removes a string from a []string if it meets certain criteria.
func RemoveStringConditionally(slice []string, s string, fn func(string, string) bool) (result []string) {
	for _, item := range slice {
		if fn(s, item) {
			continue
		}
		result = append(result, item)
	}
	return
}

// ReplaceAtIndex returns a string with the rune at the specified index replaced.
func ReplaceAtIndex(in string, r rune, index int) (string, error) {
	if index < 0 || index >= len(in) {
		return in, fmt.Errorf("index '%d' out of bounds for string '%s'", index, in)
	}
	out := []rune(in)
	out[index] = r
	return string(out), nil
}

// AppendToStringList appends an item to a string list with a seperator.
func AppendToStringList(stringList, newItem, sep string) string {
	stringListItems := SplitString(context.TODO(), stringList, sep)

	if len(stringListItems) == 0 {
		return newItem
	}

	stringListItems = append(stringListItems, newItem)
	return strings.Join(stringListItems, sep)
}

// SplitString is same as strings.Split except it returns a nil ([]string(nil)) of size 0 instead of
// string slice with an empty string (size 1) when string to be split is empty.
func SplitString(_ context.Context, s, sep string) []string {
	if s == "" {
		return nil
	}

	return strings.Split(s, sep)
}

// StringInSlice checks whether a string is in a list of strings.
func StringInSlice(s string, list []string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
