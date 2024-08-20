package models

//go:generate mockgen -destination=../../../mocks/mock_internal/mock_fiji/mock_models/mock_handler.go github.com/netapp/trident/internal/fiji/models FaultHandler

import (
	"fmt"
	"sync"
)

// FaultHandler is an abstraction all concrete fault handler should follow.
type FaultHandler interface {
	Handle() error
}

// FaultPoint is a concrete realization of the Fault interface.
type FaultPoint struct {
	mutex    *sync.RWMutex
	Name     string       `json:"name"`
	Location string       `json:"location"`
	Handler  FaultHandler `json:"handler,omitempty"`
}

// NewFaultPoint creates a new fault point with a default handler that never injects an error.
func NewFaultPoint(name, location string) *FaultPoint {
	return &FaultPoint{
		Name:     name,
		Location: location,
		mutex:    &sync.RWMutex{},
	}
}

func (f *FaultPoint) Inject() error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	// Do nothing if this fault handler hasn't been set.
	if f.Handler == nil {
		return nil
	}
	return f.Handler.Handle()
}

func (f *FaultPoint) Reset() {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.Handler = nil
	return
}

func (f *FaultPoint) IsHandlerSet() bool {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	return f.Handler != nil
}

func (f *FaultPoint) SetHandler(config []byte) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if config == nil {
		return fmt.Errorf("empty model supplied for fault: [%s]", f.Name)
	}

	handler, err := NewFaultHandlerFromModel(config)
	if err != nil {
		return fmt.Errorf("failed to determine new fault handler; %v", err)
	}

	f.Handler = handler
	return nil
}
