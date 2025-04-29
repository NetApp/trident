package handlers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorXTimesAfterYTimesHandler(t *testing.T) {
	tt := map[string]struct {
		formatStr   string
		passCount   int
		failCount   int
		assertValue assert.ValueAssertionFunc
		assertError assert.ErrorAssertionFunc
	}{
		"with no KVP for counts": {
			formatStr:   `{"name":"error-x-times-after-y-times"}`,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with negative passCount": {
			formatStr:   `{"name":"error-x-times-after-y-times", "passCount": %v, "failCount": %v}`,
			passCount:   -1,
			failCount:   3,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with negative failCount": {
			formatStr:   `{"name":"error-x-times-after-y-times", "passCount": %v, "failCount": %v}`,
			passCount:   2,
			failCount:   -1,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with zero passCount": {
			formatStr:   `{"name":"error-x-times-after-y-times", "passCount": %v, "failCount": %v}`,
			passCount:   0,
			failCount:   3,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with zero failCount": {
			formatStr:   `{"name":"error-x-times-after-y-times", "passCount": %v, "failCount": %v}`,
			passCount:   2,
			failCount:   0,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with valid values": {
			formatStr:   `{"name":"error-x-times-after-y-times", "passCount": %v, "failCount": %v}`,
			passCount:   2,
			failCount:   3,
			assertValue: assert.NotNil,
			assertError: assert.NoError,
		},
	}

	for name, test := range tt {
		t.Run(name, func(t *testing.T) {
			modelStr := fmt.Sprintf(test.formatStr, test.passCount, test.failCount)
			handler, err := NewErrorXTimesAfterYTimesHandler([]byte(modelStr))
			test.assertError(t, err)
			test.assertValue(t, handler)
		})
	}
}

func TestErrorXTimesAfterYTimesHandler_Handle(t *testing.T) {
	passCount := 2
	failCount := 3
	modelJSON := fmt.Sprintf(`{"name":"error-x-times-after-y-times", "passCount": %v, "failCount": %v}`, passCount, failCount)
	handler, err := NewErrorXTimesAfterYTimesHandler([]byte(modelJSON))
	assert.NoError(t, err)
	assert.NotNil(t, handler)
	assert.Equal(t, passCount, handler.PassCount)
	assert.Equal(t, failCount, handler.FailCount)

	// Test successful passes
	for i := 0; i < passCount; i++ {
		assert.NoError(t, handler.Handle())
	}

	// Test failures
	for i := 0; i < failCount; i++ {
		assert.Error(t, handler.Handle())
	}

	// Test succeeding indefinitely after failures
	for i := 0; i < 5; i++ { // Arbitrary number of additional calls
		assert.NoError(t, handler.Handle())
	}
}
