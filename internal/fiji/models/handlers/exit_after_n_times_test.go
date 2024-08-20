package handlers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExitAfterNTimesHandler(t *testing.T) {
	tt := map[string]struct {
		formatStr   string
		passCount   int
		exitCode    int
		assertValue assert.ValueAssertionFunc
		assertError assert.ErrorAssertionFunc
	}{
		"with no KVP for count specified": {
			formatStr:   `{"name":"exit-after-n-times"}`,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with invalid type for count specified": {
			formatStr:   `{"name":"exit-after-n-times", "passCount": "test"}`,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with invalid type fo exit code specified": {
			formatStr:   `{"name":"exit-after-n-times", "exitCode": "1"}`,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with invalid type for exit code specified": {
			formatStr:   `{"name":"exit-after-n-times", "passCount": %v,"exitCode": "one"}`,
			passCount:   1,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with negative pass count configured": {
			formatStr:   `{"name":"exit-after-n-times", "passCount": %v,"exitCode": %v}`,
			passCount:   -1,
			exitCode:    1,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with negative exit code configured": {
			formatStr:   `{"name":"exit-after-n-times", "passCount": %v,"exitCode": %v}`,
			passCount:   1,
			exitCode:    -1,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with valid values": {
			formatStr:   `{"name":"exit-after-n-times", "passCount": %v,"exitCode": %v}`,
			passCount:   1,
			exitCode:    0,
			assertValue: assert.NotNil,
			assertError: assert.NoError,
		},
	}

	for name, test := range tt {
		t.Run(name, func(t *testing.T) {
			modelStr := fmt.Sprintf(test.formatStr, test.passCount, test.exitCode)
			handler, err := NewExitAfterNTimesHandler([]byte(modelStr))
			test.assertError(t, err)
			test.assertValue(t, handler)
		})
	}
}
