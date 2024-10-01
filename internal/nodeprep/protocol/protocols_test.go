package protocol_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/internal/nodeprep/protocol"
)

func TestValidateProtocols(t *testing.T) {
	type parameters struct {
		protocols   []string
		assertValid assert.ErrorAssertionFunc
	}
	tests := map[string]parameters{
		"nil protocols": {
			protocols:   nil,
			assertValid: assert.NoError,
		},
		"empty protocols": {
			protocols:   []string{},
			assertValid: assert.NoError,
		},
		"iscsi protocol": {
			protocols:   []string{"iscsi"},
			assertValid: assert.NoError,
		},
		"iscsi protocol with whitespace": {
			protocols:   []string{" iscsi "},
			assertValid: assert.Error,
		},
		"iscsi protocol with mixed case": {
			protocols:   []string{"iScSi"},
			assertValid: assert.Error,
		},
		"iscsi protocol with whitespace and mixed case": {
			protocols:   []string{"\t iScSi "},
			assertValid: assert.Error,
		},
		"iscsi protocol with extra protocols": {
			protocols:   []string{"iscsi", "nvme"},
			assertValid: assert.Error,
		},
		"iscsi protocol with extra whitespace": {
			protocols:   []string{"iscsi", " nvme "},
			assertValid: assert.Error,
		},
		"iscsi protocol with extra whitespace and mixed case": {
			protocols:   []string{"	 iScSi ", " nVmE "},
			assertValid: assert.Error,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			err := protocol.ValidateProtocols(params.protocols)
			params.assertValid(t, err)
		})
	}
}

func TestFormatProtocols(t *testing.T) {
	type parameters struct {
		protocols      []string
		expectedOutput []string
	}

	tests := map[string]parameters{
		"empty protocols": {
			protocols:      []string{},
			expectedOutput: nil,
		},
		"lower case lettered protocols": {
			protocols:      []string{"iscsi", "nvme"},
			expectedOutput: []string{"iscsi", "nvme"},
		},
		"upper case lettered protocols": {
			protocols:      []string{"ISCSI", "NVME"},
			expectedOutput: []string{"iscsi", "nvme"},
		},
		"capitalized protocols": {
			protocols:      []string{"Iscsi", "Nvme"},
			expectedOutput: []string{"iscsi", "nvme"},
		},
		"random capitalization protocols": {
			protocols:      []string{"iScSi", "nVmE"},
			expectedOutput: []string{"iscsi", "nvme"},
		},
		"white spaces in the name of the protocols": {
			protocols:      []string{"\tiscsi ", " nvme "},
			expectedOutput: []string{"iscsi", "nvme"},
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			formattedProtocols := protocol.FormatProtocols(params.protocols)
			assert.Len(t, formattedProtocols, len(params.expectedOutput))
			assert.Equal(t, formattedProtocols, params.expectedOutput)
		})
	}
}
