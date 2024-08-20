//go:build !fiji

package fiji

type fakeInjector struct{}

func (f *fakeInjector) Inject() error {
	// Always return nil for the fake injector.
	return nil
}

// Register is the top-level entry point for all fault points in Trident.
// Fault points MUST register through this method or a fault will not be recognized for FIJI operations.
// For non-FIJI enabled builds, this should always return an Injector.
func Register(_, _ string) Injector {
	return &fakeInjector{}
}

// Frontend is a thin structure to allow build tags to control if the FIJI API server runs or not.
// If the `fiji` build tag is not specified, this will be used instead of the real fiji implementation.
type Frontend struct{}

func NewFrontend(_ string) *Frontend {
	return &Frontend{}
}

// Activate is a top-level entry point for enabling FIJI in Trident.
// In a FIJI-enabled build, this will create an HTTP API Server to enable dynamic fault-injection.
// In a non-FIJI build, this will do nothing.
// It is expected that this will only be called once per instance of Trident.
func (f *Frontend) Activate() error {
	// Do nothing
	return nil
}

// Deactivate is the top-level entry point for disabling FIJI in Trident.
// In a FIJI-enabled build, this will gracefully close the FIJI HTTP API Server.
// In a non-FIJI build, this will do nothing.
// It is expected that this will only be called once per instance of Trident.
func (f *Frontend) Deactivate() error {
	// Do nothing
	return nil
}

func (f *Frontend) GetName() string {
	return ""
}

func (f *Frontend) Version() string {
	return ""
}
