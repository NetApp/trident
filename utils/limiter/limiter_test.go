package limiter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

var ctx = context.Background

func TestNew(t *testing.T) {
	limID := "tempLimiter"

	tests := []struct {
		name        string
		limID       string
		limiterType LimiterType
		options     []LimiterOption
		expectError bool
	}{
		{
			name:        "Create new limiter",
			limID:       limID,
			limiterType: TypeSemaphoreN,
			options:     nil,
			expectError: false,
		},
		{
			name:        "Using Options",
			limID:       "LimiterWithOptions",
			limiterType: TypeSemaphoreN,
			options:     []LimiterOption{WithSemaphoreNSize(ctx(), 20)},
			expectError: false,
		},
		{
			name:        "Unknown limiter type",
			limID:       "unknownLimiter",
			limiterType: 999,
			options:     nil,
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			limiter, err := New(ctx(), test.limID, test.limiterType, test.options...)
			if test.expectError {
				assert.Error(t, err)
				assert.Nil(t, limiter)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, limiter)
			}
		})
	}
}
