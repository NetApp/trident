package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPanicHandler(t *testing.T) {
	handler, err := NewPanicHandler([]byte(`{"name":"panic"}`))
	assert.NoError(t, err)
	assert.NotNil(t, handler)
	assert.Panics(t, func() { handler.Handle() })
}
