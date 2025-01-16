package syswrap

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/exec"
)

// errors.Is should work to detect PathError from Execute, but it currently does not. If this test starts to fail
// then the errors.As calls in this package should be converted to errors.Is.
func TestSyswrapUnavailableRequiresAs(t *testing.T) {
	_, err := os.Stat(syswrapBin)
	assert.Error(t, err)
	wd, err := os.Getwd()
	assert.NoError(t, err)
	_, err = exec.NewCommand().Execute(context.Background(), syswrapBin, "1s", "statfs", wd)
	assert.Error(t, err)
	assert.False(t, errors.Is(err, &os.PathError{}))

	var pe *os.PathError
	assert.True(t, errors.As(err, &pe))
}
