// Copyright 2025 NetApp, Inc. All Rights Reserved.

package maths

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPow(t *testing.T) {
	if Pow(1024, 0) != 1 {
		t.Error("Expected 1024^0 == 1")
	}

	if Pow(1024, 1) != 1024 {
		t.Error("Expected 1024^1 == 1024")
	}

	if Pow(1024, 2) != 1048576 {
		t.Error("Expected 1024^2 == 1048576")
	}

	if Pow(1024, 3) != 1073741824 {
		t.Error("Expected 1024^3 == 1073741824")
	}
}

func TestMinInt64(t *testing.T) {
	assert.Equal(t, int64(2), MinInt64(2, 3))
	assert.Equal(t, int64(2), MinInt64(3, 2))
	assert.Equal(t, int64(-2), MinInt64(-2, 3))
	assert.Equal(t, int64(-2), MinInt64(3, -2))
	assert.Equal(t, int64(-3), MinInt64(3, -3))
	assert.Equal(t, int64(-3), MinInt64(-3, 3))
	assert.Equal(t, int64(0), MinInt64(0, 0))
	assert.Equal(t, int64(2), MinInt64(2, 2))
}
