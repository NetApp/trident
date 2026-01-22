// Copyright 2024 NetApp, Inc. All Rights Reserved.

package collection

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetV(t *testing.T) {
	d := make(map[string]string)
	d["key1"] = "value1"

	if val := GetV(d, "key1", "defaultValue"); val != "value1" {
		t.Errorf("Expected '%v' but was %v", "value1", val)
	}

	if val := GetV(d, "key2", "defaultValue"); val != "defaultValue" {
		t.Errorf("Expected '%v' but was %v", "defaultValue", val)
	}
}

func TestImmutableMap_Get(t *testing.T) {
	m := NewImmutableMap(map[string]int{"one": 1, "two": 2})

	assert.Equal(t, 1, m.Get("one"))
}

func TestImmutableMap_GetOk(t *testing.T) {
	m := NewImmutableMap(map[string]int{"one": 1, "two": 2})

	value, ok := m.GetOk("two")
	assert.True(t, ok)
	assert.Equal(t, 2, value)

	var defaultValue int
	value, ok = m.GetOk("three")
	assert.False(t, ok)
	assert.Equal(t, defaultValue, value)
}

func TestImmutableMap_Range(t *testing.T) {
	m := NewImmutableMap(map[string]int{"one": 1, "two": 2, "three": 3})
	sum := 0
	m.Range(func(key string, value int) bool {
		sum += value
		return true
	})
	assert.Equal(t, 6, sum)
}
