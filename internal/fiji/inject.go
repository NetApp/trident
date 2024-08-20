package fiji

//go:generate mockgen -destination=../../mocks/mock_internal/mock_fiji/mock_injector.go github.com/netapp/trident/internal/fiji Injector

// Injector is an abstraction of a fault point that can inject errors.
// It is up for the configuration of the fault point to dictate the behavior of this under the hood.
// NOTE: Every concrete fault must implement the Injector interface.
type Injector interface {
	Inject() error
}
