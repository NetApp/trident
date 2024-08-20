package store

//go:generate mockgen -destination=../../../mocks/mock_internal/mock_fiji/mock_store/mock_fault.go github.com/netapp/trident/internal/fiji/store Fault

import (
	"fmt"
	"sync"
)

// Fault defines behaviors for faults stored in the store.
type Fault interface {
	Inject() error
	Reset()
	IsHandlerSet() bool
	SetHandler([]byte) error
}

type Store struct {
	mutex  *sync.RWMutex
	faults map[string]Fault
}

func NewFaultStore() *Store {
	return &Store{
		mutex:  &sync.RWMutex{},
		faults: make(map[string]Fault),
	}
}

func (s *Store) Add(key string, fault Fault) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if key == "" {
		panic("empty fault name")
	} else if fault == nil {
		panic("nil fault")
	}

	if _, ok := s.faults[key]; ok {
		panic("duplicate fault name detected")
	}
	s.faults[key] = fault
}

func (s *Store) Set(key string, config []byte) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if key == "" {
		return fmt.Errorf("empty fault name")
	} else if config == nil {
		return fmt.Errorf("empty config")
	}

	fault, ok := s.faults[key]
	if !ok {
		return fmt.Errorf("fault [%s] specified in config does not exist", key)
	}

	// This will modify the fault state in-memory, so no need to reassign it at the end.
	if err := fault.SetHandler(config); err != nil {
		return fmt.Errorf("failed to load fault config for fault: [%s]; %v", key, err)
	}

	return nil
}

func (s *Store) Reset(key string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if key == "" {
		return fmt.Errorf("empty fault name")
	}

	fault, ok := s.faults[key]
	if !ok {
		return fmt.Errorf("fault [%s] specified in config does not exist", key)
	}

	fault.Reset()
	return nil
}

func (s *Store) Get(key string) (Fault, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	f, ok := s.faults[key]
	return f, ok
}

func (s *Store) Exists(key string) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	_, ok := s.faults[key]
	return ok
}

func (s *Store) List() []Fault {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	faults := make([]Fault, 0)
	for _, fault := range s.faults {
		faults = append(faults, fault)
	}

	return faults
}
