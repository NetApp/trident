// Copyright 2024 NetApp, Inc. All Rights Reserved.

package iscsi

import "strings"

// These functions are widely used throughout the codebase,so they could eventually live in utils or some other
// top-level package.
// TODO (vivintw) remove this file once the refactoring is done.

// RemoveStringFromSliceConditionally removes a string from a []string if it meets certain criteria
func RemoveStringFromSliceConditionally(slice []string, s string, fn func(string, string) bool) (result []string) {
	for _, item := range slice {
		if fn(s, item) {
			continue
		}
		result = append(result, item)
	}
	return
}

func IPv6Check(ip string) bool {
	return strings.Count(ip, ":") >= 2
}
