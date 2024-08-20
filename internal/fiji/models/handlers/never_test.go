package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNeverErrorHandler(t *testing.T) {
	neverName := "never"
	handler, err := NewNeverErrorHandler([]byte(`{"name": "never"}`))
	assert.NoError(t, err)
	assert.NotNil(t, handler)

	assert.Equal(t, neverName, handler.Name)
	assert.NoError(t, handler.Handle())
}
