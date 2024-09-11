package nodeprep

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateProtocols(t *testing.T) {
	tests := []struct {
		name                   string
		protocols              []string
		assertValidUnformatted assert.ErrorAssertionFunc
		assertValidFormatted   assert.ErrorAssertionFunc
	}{
		{"nil protocols", nil, assert.NoError, assert.NoError},
		{"empty protocols", []string{}, assert.NoError, assert.NoError},
		{"iscsi protocol", []string{"iscsi"}, assert.NoError, assert.NoError},
		{"iscsi protocol with whitespace", []string{" iscsi "}, assert.Error, assert.NoError},
		{"iscsi protocol with mixed case", []string{"iScSi"}, assert.Error, assert.NoError},
		{"iscsi protocol with whitespace and mixed case", []string{"\t iScSi "}, assert.Error, assert.NoError},
		{"iscsi protocol with extra protocols", []string{"iscsi", "nvme"}, assert.Error, assert.Error},
		{"iscsi protocol with extra whitespace", []string{"iscsi", " nvme "}, assert.Error, assert.Error},
		{"iscsi protocol with extra whitespace and mixed case", []string{"	 iScSi ", " nVmE "}, assert.Error, assert.Error},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateProtocols(test.protocols)
			test.assertValidUnformatted(t, err)

			formattedProtocols := FormatProtocols(test.protocols)
			err = ValidateProtocols(formattedProtocols)
			test.assertValidFormatted(t, err)
		})
	}
}
