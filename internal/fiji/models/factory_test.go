package models

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFaultType_String(t *testing.T) {
	var testStr HandlerType
	testStr = "never"
	assert.NotEmpty(t, testStr.String())
}

func TestFaultHandlerFactory(t *testing.T) {
	tt := map[HandlerType]struct {
		model       string
		assertNil   assert.ValueAssertionFunc
		assertError assert.ErrorAssertionFunc
	}{
		Never: {
			model:       `{"name": "never"}`,
			assertNil:   assert.NotNil,
			assertError: assert.NoError,
		},
		Panic: {
			model:       fmt.Sprintf(`{"name": "panic"}`),
			assertNil:   assert.NotNil,
			assertError: assert.NoError,
		},
		PauseNTimes: {
			model:       fmt.Sprintf(`{"name": "pause-n-times", "duration": "%s", "failCount": %v}`, "500ms", 1),
			assertNil:   assert.NotNil,
			assertError: assert.NoError,
		},
		Exit: {
			model:       fmt.Sprintf(`{"name": "exit", "exitCode": %v}`, 1),
			assertNil:   assert.NotNil,
			assertError: assert.NoError,
		},
		Always: {
			model:       `{"name": "always"}`,
			assertNil:   assert.NotNil,
			assertError: assert.NoError,
		},
		ErrorNTimes: {
			model:       fmt.Sprintf(`{"name": "error-n-times", "failCount": %v}`, 1),
			assertNil:   assert.NotNil,
			assertError: assert.NoError,
		},
		ErrorAfterNTimes: {
			model:       fmt.Sprintf(`{"name": "error-after-n-times", "passCount": %v}`, 1),
			assertNil:   assert.NotNil,
			assertError: assert.NoError,
		},
		ExitAfterNTimes: {
			model:       fmt.Sprintf(`{"name": "exit-after-n-times", "exitCode": %v, "passCount": %v}`, 1, 1),
			assertNil:   assert.NotNil,
			assertError: assert.NoError,
		},
	}

	for name, test := range tt {
		t.Run(string(name), func(t *testing.T) {
			model := []byte(test.model)
			assert.NotNil(t, model)

			handler, err := NewFaultHandlerFromModel(model)
			test.assertNil(t, handler)
			test.assertError(t, err)
		})
	}
}
