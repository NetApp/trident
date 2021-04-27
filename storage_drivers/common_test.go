// Copyright 2021 NetApp, Inc. All Rights Reserved.

package storagedrivers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAreSameCredentials(t *testing.T) {

	type Credentials struct {
		Credential1 map[string]string
		Credential2 map[string]string
		Same        bool
	}

	inputs := []Credentials{
		{
			map[string]string{"name": "secret1", "type": "secret"},
			map[string]string{"name": "secret1", "type": "secret"},
			true,
		},
		{
			map[string]string{"name": "secret1", "type": "secret"},
			map[string]string{"name": "secret1"},
			true,
		},
		{
			map[string]string{"name": "secret1", "type": "secret"},
			map[string]string{"name": "secret1", "type": "random"},
			false,
		},
		{
			map[string]string{"name": "secret1"},
			map[string]string{"name": "secret1", "type": "random"},
			false,
		},
		{
			map[string]string{"name": "", "type": "secret", "randomKey": "randomValue"},
			map[string]string{"name": "", "type": "secret", "randomKey": "randomValue"},
			false,
		},
	}

	for _, input := range inputs {
		areEqual := AreSameCredentials(input.Credential1, input.Credential2)
		assert.Equal(t, areEqual, input.Same)
	}
}

