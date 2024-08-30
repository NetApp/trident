// Copyright 2024 NetApp, Inc. All Rights Reserved.

package models

import (
	"fmt"
	"reflect"
	"strings"

	. "github.com/netapp/trident/logging"
)

// temporary utilities till we remove dependency on the original utils package,
// TODO remove this file once the refactoring is done.

const (
	redacted = "<REDACTED>"
)

// Ptr converts any value into a pointer to that value.
func Ptr[T any](v T) *T {
	return &v
}

// PtrToString converts any value into its string representation, or nil
func PtrToString[T any](v *T) string {
	if v == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v", *v)
}

// ToStringRedacted identifies attributes of a struct, stringifies them such that they can be consumed by the
// struct's stringer interface, and redacts elements specified in the redactList.
func ToStringRedacted(structPointer interface{}, redactList []string, configVal interface{}) (out string) {
	defer func() {
		if r := recover(); r != nil {
			Log().Errorf("Panic in utils#ToStringRedacted; err: %v", r)
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
		case SliceContainsString(redactList, fieldName):
			output.WriteString(fmt.Sprintf("%v:%v ", fieldName, redacted))
		default:
			output.WriteString(fmt.Sprintf("%v:%#v ", fieldName, elements.Field(i)))
		}
	}

	out = output.String()
	return
}

// SliceContainsString checks to see if a []string contains a string
func SliceContainsString(slice []string, s string) bool {
	return SliceContainsStringConditionally(slice, s, func(val1, val2 string) bool { return val1 == val2 })
}

// SliceContainsStringConditionally checks to see if a []string contains a string based on certain criteria
func SliceContainsStringConditionally(slice []string, s string, fn func(string, string) bool) bool {
	for _, item := range slice {
		if fn(item, s) {
			return true
		}
	}
	return false
}

// SliceContainsElements checks to see if a slice (genericSlice) contains a list of all/some items (genericElements)
func SliceContainsElements(genericSlice, genericElements interface{}) (bool, bool) {
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
		if SliceContains(genericSlice, elem.Index(i).Interface()) {
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

// SliceContains checks to see if a slice (genericSlice) contains an item (genericElement)
func SliceContains(genericSlice, genericElement interface{}) bool {
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
