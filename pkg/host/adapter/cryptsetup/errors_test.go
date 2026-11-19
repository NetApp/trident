// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cryptsetup

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIncorrectPassphraseError(t *testing.T) {
	base := errors.New("exec failed")
	err := &IncorrectPassphraseError{err: base}

	assert.Equal(t, "incorrect LUKS passphrase", err.Error())
	assert.ErrorIs(t, err, base)
	assert.True(t, IsIncorrectPassphraseError(err))
	assert.False(t, IsIncorrectPassphraseError(base))
}

func TestNotLUKSDeviceError(t *testing.T) {
	base := errors.New("exec failed")
	err := &NotLUKSDeviceError{err: base}

	assert.Equal(t, "device is not LUKS formatted", err.Error())
	assert.ErrorIs(t, err, base)
	assert.True(t, IsNotLUKSDeviceError(err))
	assert.False(t, IsNotLUKSDeviceError(base))
}

func TestDeviceNotActiveError(t *testing.T) {
	base := errors.New("exec failed")
	err := &DeviceNotActiveError{err: base}

	assert.Equal(t, "LUKS mapper is not active", err.Error())
	assert.ErrorIs(t, err, base)
	assert.True(t, IsDeviceNotActiveError(err))
	assert.False(t, IsDeviceNotActiveError(base))
}

func TestBusyDeviceError(t *testing.T) {
	base := errors.New("exec failed")
	err := &BusyDeviceError{err: base}

	assert.Equal(t, "LUKS device is busy", err.Error())
	assert.ErrorIs(t, err, base)
	assert.True(t, IsBusyDeviceError(err))
	assert.False(t, IsBusyDeviceError(base))
}

func TestBadParameterError(t *testing.T) {
	base := errors.New("exec failed")
	err := &BadParameterError{err: base}

	assert.Equal(t, "cryptsetup invoked with bad parameters", err.Error())
	assert.ErrorIs(t, err, base)
	assert.True(t, IsBadParameterError(err))
	assert.False(t, IsBadParameterError(base))
}

func TestOOMError(t *testing.T) {
	base := errors.New("exec failed")
	err := &OOMError{err: base}

	assert.Equal(t, "cryptsetup oom failure", err.Error())
	assert.ErrorIs(t, err, base)
	assert.True(t, IsOOMError(err))
	assert.False(t, IsOOMError(base))
}
