//go:build fiji

package fiji

import (
	"fmt"
	"regexp"
	"sync"

	"github.com/netapp/trident/internal/fiji/models"
	"github.com/netapp/trident/internal/fiji/rest"
	"github.com/netapp/trident/internal/fiji/store"
)

var (
	// These regex's allow us to stop faults being created with specific characters.
	// Two regexes cover reserved and disallowed chars from RFC 3986 and one is a constraint to stop "_", "~", "."
	// from being used for fault names.

	specialChars    = regexp.MustCompile(`[_~.]`)
	reservedChars   = regexp.MustCompile(`[:/?#\[\]@!$&'()*+,;=]`)
	disallowedChars = regexp.MustCompile(`[ \x00-\x1F\x7F"<>\\^` + "`" + `{|}]`)

	faultStore    rest.FaultStore // faultStore is an in-memory singleton for managing fault points.
	initStoreOnce sync.Once       // initStoreOnce protects the fault store from being initialized more than once.
)

// initFaultStore ensures it will never be initialized more than once.
func initFaultStore() {
	initStoreOnce.Do(func() {
		faultStore = store.NewFaultStore()
	})
}

// isValidName fault names are used as resource identifiers, so new fault points must have a URI-compliant fault name.
// This check should stop engineers from adding faults that don't meet our requirements.
func isValidName(name string) bool {
	return !(reservedChars.MatchString(name) || disallowedChars.MatchString(name) || specialChars.MatchString(name))
}

// Register is the top-level entry point for all fault points in Trident.
// Fault points MUST register through this method or a fault will not be recognized for FIJI operations.
func Register(name, location string) *models.FaultPoint {
	if !isValidName(name) {
		panic(fmt.Errorf("invalid characters in fault name \"%s\"", name))
	}

	if faultStore == nil {
		initFaultStore()
	}

	fault := models.NewFaultPoint(name, location)
	faultStore.Add(name, fault)
	return fault
}

// Frontend is a thin structure to allow build tags to control if the FIJI API server runs or not.
// If the `fiji` build tag is specified, this will be used instead of the fake.
type Frontend struct {
	server *rest.Server
}

func NewFrontend(address string) *Frontend {
	if faultStore == nil {
		initFaultStore()
	}

	return &Frontend{rest.NewFaultInjectionServer(address, faultStore)}
}

// Activate is a top-level entry point for enabling FIJI in Trident.
// In a FIJI-enabled build, this will create an HTTP API Server to enable dynamic fault-injection.
// In a non-FIJI build, this will do nothing.
// It is expected that this will only be called once per instance of Trident.
func (f *Frontend) Activate() error {
	return f.server.Activate()
}

// Deactivate is the top-level entry point for disabling FIJI in Trident.
// In a FIJI-enabled build, this will gracefully close the FIJI HTTP API Server.
// In a non-FIJI build, this will do nothing.
// It is expected that this will only be called once per instance of Trident.
func (f *Frontend) Deactivate() error {
	return f.server.Deactivate()
}

func (f *Frontend) GetName() string {
	return f.server.GetName()
}

func (f *Frontend) Version() string {
	return f.server.Version()
}
