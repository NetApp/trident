// Copyright 2025 NetApp, Inc. All Rights Reserved.

package blockdevice

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	client := New()
	assert.NotNil(t, client, "New() should return a non-nil client")
}
