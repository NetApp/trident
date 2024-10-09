package step

//go:generate mockgen -destination=../../../mocks/mock_internal/mock_nodeprep/mock_step/mock_steps.go github.com/netapp/trident/internal/nodeprep/step Step

import "context"

type Step interface {
	GetName() string
	IsRequired() bool
	Apply(ctx context.Context) error
}
