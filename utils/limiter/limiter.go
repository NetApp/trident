package limiter

//go:generate mockgen -destination=../../mocks/mock_utils/mock_limiter/mock_limiter.go -package=mock_limiter github.com/netapp/trident/utils/limiter Limiter

import (
	"context"
	"fmt"

	. "github.com/netapp/trident/logging"
)

type Limiter interface {
	Wait(ctx context.Context) error
	Release(ctx context.Context)
}

// LimiterOption Using an option pattern to generalize New function.
type LimiterOption func(limiter Limiter) error

type LimiterType uint16

const (
	TypeSemaphoreN LimiterType = iota
)

// New creates a limiter based on the provided limiterType.
// Note: This function is not thread-safe, so precautions are necessary for concurrent use.
// Ensure the limiter type and options are correctly matched to avoid errors.
func New(ctx context.Context, limID string, limiterType LimiterType, options ...LimiterOption) (Limiter, error) {
	logFields := LogFields{
		"LimiterID":   limID,
		"LimiterType": limiterType,
		"Options":     options,
	}
	Logc(ctx).WithFields(logFields).Debug(">>>> Limiter.New")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< Limiter.New")

	var limiter Limiter

	// We'll create the new Limiter, depending upon the type passed in limiterType arg.
	switch limiterType {
	case TypeSemaphoreN:
		limiter = newSemaphoreN(limID)

	default:
		return nil, fmt.Errorf("unknown limiter type: %T", limiterType)
	}

	// Applying the limiter options.
	for _, option := range options {
		if err := option(limiter); err != nil {
			return nil, err
		}
	}

	Logc(ctx).WithFields(logFields).Debugf("Limiter with limID %s was created", limID)

	return limiter, nil
}
