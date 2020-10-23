// Copyright 2019 NetApp, Inc. All Rights Reserved.

package v1

import (
	"errors"
	"regexp"
	"strings"
)

var ErrNamesDontMatch = errors.New("names do not match")

var nameMatchRegex = regexp.MustCompile(`[^a-z0-9\.\-]`)

func NameFix(n string) string {
	n = strings.ToLower(n)
	n = nameMatchRegex.ReplaceAllString(n, "-")
	return n
}
