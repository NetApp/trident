// Copyright 2025 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAssertErrorForCode(t *testing.T) {
	tests := map[string]struct {
		code      string
		assertErr assert.ErrorAssertionFunc
	}{
		"with status 200":     {code: "200", assertErr: assert.NoError},
		"with status 302":     {code: "302", assertErr: assert.NoError},
		"with status 404":     {code: "404", assertErr: assert.Error},
		"with status <empty>": {code: "   ", assertErr: assert.Error},
		"with status xyz":     {code: "xyz", assertErr: assert.Error},
		"with status 99":      {code: "99", assertErr: assert.Error},
		"with status 600":     {code: "600", assertErr: assert.Error},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := assertErrorForCode(tc.code)
			tc.assertErr(t, err)
		})
	}
}
