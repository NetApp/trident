package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExitHandler(t *testing.T) {
	handler, err := NewExitHandler([]byte(`{"name":"exit", "exitCode": -1}`))
	assert.Error(t, err)
	assert.Nil(t, handler)

	handler, err = NewExitHandler([]byte(`{"name":"exit", "exitCode": 1}`))
	assert.NoError(t, err)
	assert.NotNil(t, handler)
}
