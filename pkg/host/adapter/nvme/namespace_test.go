// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNamespace(t *testing.T) {
	ns := &Namespace{name: "nvme0n1", uuid: "ffce3179-8d4b-4c1f-8a0e-6a1c53e3e9a1"}

	assert.Equal(t, "nvme0n1", ns.Name())
	assert.Equal(t, "ffce3179-8d4b-4c1f-8a0e-6a1c53e3e9a1", ns.UUID())
}

func TestNamespace_EmptyUUID(t *testing.T) {
	ns := &Namespace{name: "nvme0n1"}

	assert.Equal(t, "nvme0n1", ns.Name())
	assert.Equal(t, "", ns.UUID())
}
