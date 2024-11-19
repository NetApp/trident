package handlers

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPauseNTimesHandler(t *testing.T) {
	tt := map[string]struct {
		formatStr   string
		duration    time.Duration
		failCount   int
		assertValue assert.ValueAssertionFunc
		assertError assert.ErrorAssertionFunc
	}{
		"with no KVP for duration": {
			formatStr:   `{"name":"pause-n-times", "failCount": %v}`,
			failCount:   1,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with negative duration": {
			formatStr:   `{"name":"pause-n-times", "failCount": %v, "duration": "%s"}`,
			duration:    -500 * time.Millisecond,
			failCount:   1,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with no KVP for count": {
			formatStr:   `{"name":"pause-n-times", "duration": "%s"}`,
			duration:    500 * time.Millisecond,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with negative value for count": {
			formatStr:   `{"name":"pause-n-times", "duration": "%s", "failCount": %v}`,
			duration:    500 * time.Millisecond,
			failCount:   -1,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with zero value for count": {
			formatStr:   `{"name":"pause-n-times", "duration": "%s", "failCount": %v}`,
			duration:    500 * time.Millisecond,
			failCount:   0,
			assertValue: assert.Nil,
			assertError: assert.Error,
		},
		"with Valid value": {
			formatStr:   `{"name":"pause-n-times", "duration": "%s", "failCount": %v}`,
			duration:    500 * time.Millisecond,
			failCount:   1,
			assertValue: assert.NotNil,
			assertError: assert.NoError,
		},
	}

	for name, test := range tt {
		t.Run(name, func(t *testing.T) {
			modelStr := fmt.Sprintf(test.formatStr, test.duration, test.failCount)
			handler, err := NewPauseNTimesHandler([]byte(modelStr))
			test.assertError(t, err)
			test.assertValue(t, handler)
		})
	}
}

func TestPauseNTimesHandler_Handle(t *testing.T) {
	handler, err := NewPauseNTimesHandler([]byte(`{"name":"pause-n-times", "duration": "500ms", "failCount": 1}`))
	assert.NoError(t, err)
	assert.NotNil(t, handler)

	maximumPauseTime, err := time.ParseDuration("500ms")
	assert.NoError(t, err)

	start := time.Now()
	_ = handler.Handle()
	stop := time.Since(start)
	// Add a padding to handle variability between runs.
	maximumPauseTime += 50 * time.Millisecond
	assert.GreaterOrEqual(t, maximumPauseTime, stop)
}
