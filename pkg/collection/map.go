// Copyright 2024 NetApp, Inc. All Rights Reserved.

package collection

import (
	"strings"
)

// GetV takes a map, key(s), and a defaultValue; will return the value of the key or defaultValue if none is set.
// If keys is a string of key values separated by "|", then each key is tried in turn.  This allows compatibility
// with deprecated values, i.e. "fstype|fileSystemType".
func GetV(opts map[string]string, keys, defaultValue string) string {
	for _, key := range strings.Split(keys, "|") {
		// Try key first, then do a case-insensitive search
		if value, ok := opts[key]; ok {
			return value
		} else {
			for k, v := range opts {
				if strings.EqualFold(k, key) {
					return v
				}
			}
		}
	}
	return defaultValue
}

type ImmutableMap[K comparable, V any] struct {
	d map[K]V
}

func NewImmutableMap[K comparable, V any](m map[K]V) *ImmutableMap[K, V] {
	if m == nil {
		m = make(map[K]V)
	}
	return &ImmutableMap[K, V]{d: m}
}

func (im *ImmutableMap[K, V]) Get(key K) V {
	return im.d[key]
}

func (im *ImmutableMap[K, V]) GetOk(key K) (V, bool) {
	value, ok := im.d[key]
	return value, ok
}

func (im *ImmutableMap[K, V]) Range(f func(key K, value V) bool) {
	for k, v := range im.d {
		if !f(k, v) {
			break
		}
	}
}

func (im *ImmutableMap[K, V]) Length() int {
	return len(im.d)
}
