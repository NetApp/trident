package handlers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorNTimesHandler(t *testing.T) {
	tt := map[string]struct {
		formatStr   string
		failCount   int
		assertValue assert.ValueAssertionFunc
		assertError assert.ErrorAssertionFunc
	}{
		"with no KVP for count": {
			formatStr:   `{"name":"error-n-times"}`,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with negative value for count": {
			formatStr:   `{"name":"error-n-times", "failCount": %v}`,
			failCount:   -1,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with zero value for count": {
			formatStr:   `{"name":"error-n-times", "failCount": %v}`,
			failCount:   0,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with positive value for count": {
			formatStr:   `{"name":"error-n-times", "failCount": %v}`,
			failCount:   1,
			assertValue: assert.NotNil,
			assertError: assert.NoError,
		},
	}

	for name, test := range tt {
		t.Run(name, func(t *testing.T) {
			modelStr := fmt.Sprintf(test.formatStr, test.failCount)
			handler, err := NewErrorNTimesHandler([]byte(modelStr))
			test.assertError(t, err)
			test.assertValue(t, handler)
		})
	}
}

func TestErrorNTimesHandler_Handle(t *testing.T) {
	count := 5
	modelJSON := fmt.Sprintf(`{"name":"error-n-times", "failCount": %v}`, count)
	handler, err := NewErrorNTimesHandler([]byte(modelJSON))
	assert.NoError(t, err)
	assert.NotNil(t, handler)
	assert.Equal(t, count, handler.FailCount)

	for i := 0; i < count; i++ {
		assert.Error(t, handler.Handle())
	}
	assert.NoError(t, handler.Handle())
}
