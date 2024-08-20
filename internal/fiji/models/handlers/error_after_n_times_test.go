package handlers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorAfterNTimesHandler(t *testing.T) {
	tt := map[string]struct {
		formatStr   string
		passCount   int
		assertValue assert.ValueAssertionFunc
		assertError assert.ErrorAssertionFunc
	}{
		"with no KVP for count": {
			formatStr:   `{"name":"error-after-n-times"}`,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with negative exit code": {
			formatStr:   `{"name":"error-after-n-times", "passCount": %v}`,
			passCount:   -1,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with zero exit code": {
			formatStr:   `{"name":"error-after-n-times", "passCount": %v}`,
			passCount:   0,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with valid values": {
			formatStr:   `{"name":"error-after-n-times", "passCount": %v}`,
			passCount:   1,
			assertValue: assert.NotNil,
			assertError: assert.NoError,
		},
	}

	for name, test := range tt {
		t.Run(name, func(t *testing.T) {
			modelStr := fmt.Sprintf(test.formatStr, test.passCount)
			handler, err := NewErrorAfterNTimesHandler([]byte(modelStr))
			test.assertError(t, err)
			test.assertValue(t, handler)
		})
	}
}

func TestErrorAfterNTimesHandler_Handle(t *testing.T) {
	count := 5
	modelJSON := fmt.Sprintf(`{"name":"error-after-n-times", "passCount": %v}`, count)
	handler, err := NewErrorAfterNTimesHandler([]byte(modelJSON))
	assert.NoError(t, err)
	assert.NotNil(t, handler)
	assert.Equal(t, count, handler.PassCount)

	for i := 0; i < count; i++ {
		assert.NoError(t, handler.Handle())
	}
	assert.Error(t, handler.Handle())
}
