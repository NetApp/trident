// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestError(t *testing.T) {
	err := &Error{bin: "nvme", cmd: "version", code: 2, out: "boom"}

	assert.Equal(t, "nvme version exit code 2: output boom", err.Error())
	assert.Equal(t, 2, err.ExitCode())
	assert.Equal(t, "boom", err.Output())
	assert.Equal(t, "nvme", err.Binary())
	assert.Equal(t, "version", err.Cmd())
}

func TestSubsystemNotFoundError(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", &SubsystemNotFoundError{nqn: "nqn.x"})

	assert.True(t, IsSubsystemNotFoundError(err))
	assert.False(t, IsSubsystemNotFoundError(errors.New("other")))
	assert.Equal(t, "NVMe subsystem nqn.x not found", (&SubsystemNotFoundError{nqn: "nqn.x"}).Error())
}
