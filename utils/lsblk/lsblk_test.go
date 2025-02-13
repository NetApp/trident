package lsblk

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	lsblk := NewLsblkUtil()
	assert.NotNil(t, lsblk)
}

func TestNewDetailed(t *testing.T) {
	lsblk := NewLsblkUtilDetailed(nil)
	assert.NotNil(t, lsblk)
}
