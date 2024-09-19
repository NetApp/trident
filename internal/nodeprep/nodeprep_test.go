// Copyright 2024 NetApp, Inc. All Rights Reserved.

package nodeprep_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/internal/nodeprep"
)

func TestPrepareNodeProtocols(t *testing.T) {
	type parameters struct {
		protocols        []string
		expectedExitCode int
	}
	tests := map[string]parameters{
		"no protocols": {
			protocols:        nil,
			expectedExitCode: 0,
		},
		"happy path": {
			protocols:        []string{"iscsi"},
			expectedExitCode: 0,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			exitCode := nodeprep.NewNodePrepDetailed(NewFindBinaryMock(nodeprep.PkgMgrYum).FindBinary).PrepareNode(params.protocols)
			assert.Equal(t, params.expectedExitCode, exitCode)
		})
	}
}

func TestPrepareNodePkgMgr(t *testing.T) {
	type parameters struct {
		findBinaryMock   *FindBinaryMock
		expectedExitCode int
	}
	tests := map[string]parameters{
		"supported yum": {
			findBinaryMock:   NewFindBinaryMock(nodeprep.PkgMgrYum),
			expectedExitCode: 0,
		},
		"supported apt": {
			findBinaryMock:   NewFindBinaryMock(nodeprep.PkgMgrApt),
			expectedExitCode: 0,
		},
		"not supported": {
			findBinaryMock:   NewFindBinaryMock("other"),
			expectedExitCode: 1,
		},
		"empty": {
			findBinaryMock:   NewFindBinaryMock(""),
			expectedExitCode: 1,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			exitCode := nodeprep.NewNodePrepDetailed(params.findBinaryMock.FindBinary).PrepareNode([]string{"iscsi"})
			assert.Equal(t, params.expectedExitCode, exitCode)
		})
	}
}

type FindBinaryMock struct {
	binary string
}

func NewFindBinaryMock(binary nodeprep.PkgMgr) *FindBinaryMock {
	return &FindBinaryMock{binary: string(binary)}
}

func (f *FindBinaryMock) FindBinary(binary string) (string, string) {
	if binary == f.binary {
		return "/host", "/bin/" + binary
	}
	return "", ""
}
