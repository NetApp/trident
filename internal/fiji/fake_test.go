//go:build !fiji

package fiji

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

func TestFake_Register(t *testing.T) {
	assert.NotNil(t, Register("", ""), "expected non-nil injector")
}

func TestFake_Frontend(t *testing.T) {
	f := NewFrontend("")
	assert.Nil(t, f.Activate(), "unexpected error")
	assert.Nil(t, f.Deactivate(), "unexpected error")
	assert.Empty(t, f.GetName(), "unexpected name")
	assert.Empty(t, f.Version(), "unexpected version")
}
