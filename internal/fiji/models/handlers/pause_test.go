package handlers

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPauseHandler(t *testing.T) {
	tt := map[string]struct {
		formatStr   string
		duration    time.Duration
		assertValue assert.ValueAssertionFunc
		assertError assert.ErrorAssertionFunc
	}{
		"with no KVP for duration": {
			formatStr:   `{"name":"pause"}`,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with negative duration": {
			formatStr:   `{"name":"pause", "duration": "%s"}`,
			duration:    -500 * time.Millisecond,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with zero exit code": {
			formatStr:   `{"name":"pause", "duration": "%s"}`,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with valid values": {
			formatStr:   `{"name":"pause", "duration": "%s"}`,
			duration:    500 * time.Millisecond,
			assertValue: assert.NotNil,
			assertError: assert.NoError,
		},
	}

	for name, test := range tt {
		t.Run(name, func(t *testing.T) {
			modelStr := fmt.Sprintf(test.formatStr, test.duration)
			handler, err := NewPauseHandler([]byte(modelStr))
			test.assertError(t, err)
			test.assertValue(t, handler)
		})
	}
}

func TestPauseHandler_Handle(t *testing.T) {
	handler, err := NewPauseHandler([]byte(`{"name":"pause", "duration": "500ms"}`))
	assert.NoError(t, err)
	assert.NotNil(t, handler)

	maximumPauseTime, err := time.ParseDuration("500ms")
	assert.NoError(t, err)

	start := time.Now()
	_ = handler.Handle()
	stop := time.Since(start)
	// Add a padding to handle variability between runs.
	maximumPauseTime += 5000 * time.Microsecond
	assert.GreaterOrEqual(t, maximumPauseTime, stop)
}
