package handlers

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/netapp/trident/logging"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

func TestAlwaysHandler(t *testing.T) {
	handler, err := NewAlwaysErrorHandler([]byte(`{"name":"always"}`))
	assert.NoError(t, err)
	assert.NotNil(t, handler)
	assert.Error(t, handler.Handle())
}
