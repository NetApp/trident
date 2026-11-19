// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
)

func TestNew_Defaults(t *testing.T) {
	a, ok := New().(*adapter)

	assert.True(t, ok)
	assert.NotNil(t, a.cmd)
	assert.IsType(t, afero.NewOsFs(), a.fs)
	assert.Equal(t, "", a.chrt)
}

func TestNew_Options(t *testing.T) {
	fs := afero.NewMemMapFs()
	cmd := mockexec.NewMockCommand(gomock.NewController(t))

	a := New(WithFs(fs), WithCommand(cmd), WithChrootPrefix("/host")).(*adapter)

	assert.Same(t, fs, a.fs)
	assert.Equal(t, cmd, a.cmd)
	assert.Equal(t, "/host", a.chrt)
}

func TestNew_LaterOptionWins(t *testing.T) {
	a := New(WithChrootPrefix("/one"), WithChrootPrefix("/two")).(*adapter)

	assert.Equal(t, "/two", a.chrt)
}
