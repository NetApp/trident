// Copyright 2021 NetApp, Inc. All Rights Reserved.

package storagedrivers

import (
	"fmt"
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

func TestEnsureJoinedStringContainsElem(t *testing.T) {
	tests := []struct {
		joined   string
		elem     string
		sep      string
		expected string
	}{
		{
			elem:     "abc",
			sep:      ",",
			expected: "abc",
		},
		{
			joined:   "abc,def",
			elem:     "efg",
			sep:      ",",
			expected: "abc,def,efg",
		},
		{
			joined:   "def",
			elem:     "abc",
			sep:      ".",
			expected: "def.abc",
		},
		{
			joined:   "defabc|123",
			elem:     "abc",
			sep:      "|",
			expected: "defabc|123",
		},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			actual := ensureJoinedStringContainsElem(test.joined, test.elem, test.sep)
			assert.Equal(t, test.expected, actual)
		})
	}
}
