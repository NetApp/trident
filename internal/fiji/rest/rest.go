package rest

//go:generate mockgen -destination=../../../mocks/mock_internal/mock_fiji/mock_rest/mock_store.go github.com/netapp/trident/internal/fiji/rest FaultStore

import (
	"github.com/netapp/trident/internal/fiji/store"
)

type FaultStore interface {
	Add(string, store.Fault)
	Set(string, []byte) error
	Reset(string) error
	Get(string) (store.Fault, bool)
	Exists(string) bool
	List() []store.Fault
}

// NewFaultInjectionServer is the main constructor for creating the FIJI API server plugin.
func NewFaultInjectionServer(address string, store FaultStore) *Server {
	return NewHTTPServer(address, NewRouter(store))
}
