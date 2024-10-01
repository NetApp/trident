package step

import "context"

type Step interface {
	GetName() string
	IsRequired() bool
	Apply(ctx context.Context) error
}
