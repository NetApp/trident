package store

import (
	"errors"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/mocks/mock_internal/mock_fiji/mock_store"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

func newEmptyTestStore() *Store {
	return NewFaultStore()
}

// newTestStore takes a list of functions that return a name and a fault and
// returns a filled fault store.
func newTestStore(fs ...func() (string, Fault)) *Store {
	faults := make(map[string]Fault, len(fs))
	for _, f := range fs {
		name, fault := f()
		faults[name] = fault
	}

	return &Store{
		mutex:  &sync.RWMutex{},
		faults: faults,
	}
}

func TestStore_Add(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	tt := map[string]func(*testing.T){
		"with empty key": func(t *testing.T) {
			store := newEmptyTestStore()
			assert.Panics(t, func() { store.Add("", nil) }, "expected panic")
		},
		"with nil fault": func(t *testing.T) {
			store := newEmptyTestStore()
			assert.Panics(t, func() { store.Add("validFaultName", nil) }, "expected panic")
		},
		"with two faults with the same name": func(t *testing.T) {
			name := "fault-A"

			// Create two different faults.
			faultA := mock_store.NewMockFault(mockCtrl)
			faultB := mock_store.NewMockFault(mockCtrl)

			store := newEmptyTestStore()
			store.Add(name, faultA) // Register the first fault with the name.

			// Ensure that attempting to add a second fault with the same name induces a panic.
			assert.Panics(t, func() { store.Add(name, faultB) }, "expected panic")
		},
		"with two faults with different names": func(t *testing.T) {
			nameA := "fault-A"
			nameB := "fault-B"

			// Create two faults with different names.
			faultA := mock_store.NewMockFault(mockCtrl)
			faultB := mock_store.NewMockFault(mockCtrl)

			store := newEmptyTestStore()
			store.Add(nameA, faultA)
			assert.NotPanics(t, func() { store.Add(nameB, faultB) }, "unexpected panic")
		},
	}

	for name, test := range tt {
		t.Run(name, test)
	}
}

func TestStore_Set(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	tt := map[string]func(*testing.T){
		"with empty key": func(t *testing.T) {
			store := newEmptyTestStore()
			assert.Error(t, store.Set("", nil), "expected error")
		},
		"with nil config": func(t *testing.T) {
			store := newEmptyTestStore()
			assert.Error(t, store.Set("fault-A", nil), "expected error")
		},
		"when fault doesn't exist": func(t *testing.T) {
			name := "fault-A"
			store := newEmptyTestStore()

			// Ensure that attempting to add a second fault with the same name induces a panic.
			assert.Error(t, store.Set(name, nil), "expected error")
		},
		"when fault can't set handler config": func(t *testing.T) {
			name := "fault-A"
			config := []byte(`{"name":"panic" `) // invalid json
			fault := mock_store.NewMockFault(mockCtrl)
			fault.EXPECT().SetHandler(config).Return(errors.New("set handler config error"))

			store := newEmptyTestStore()
			store.Add(name, fault) // Register the first fault with the name.

			assert.Error(t, store.Set(name, config), "expected error")
		},
		"when fault config is set": func(t *testing.T) {
			name := "fault-A"
			config := []byte(`{"name":"panic"}`)
			fault := mock_store.NewMockFault(mockCtrl)
			fault.EXPECT().SetHandler(config).Return(nil)

			store := newEmptyTestStore()
			store.Add(name, fault)

			assert.NoError(t, store.Set(name, config), "expected error")
		},
	}

	for name, test := range tt {
		t.Run(name, test)
	}
}

func TestStore_Reset(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	tt := map[string]func(*testing.T){
		"with empty key": func(t *testing.T) {
			store := newEmptyTestStore()
			assert.Error(t, store.Reset(""), "expected error")
		},
		"when fault doesn't exist": func(t *testing.T) {
			name := "fault-A"
			store := newEmptyTestStore()

			// Ensure that attempting to add a second fault with the same name induces a panic.
			assert.Error(t, store.Reset(name), "expected error")
		},
		"when fault config is reset": func(t *testing.T) {
			name := "fault-A"
			fault := mock_store.NewMockFault(mockCtrl)
			fault.EXPECT().Reset()

			store := newEmptyTestStore()
			store.Add(name, fault)

			assert.NoError(t, store.Reset(name), "expected error")

			f, ok := store.Get(name)
			if !ok {
				t.Fatalf("failed to find test fault in store")
			}
			assert.NotNil(t, f)
		},
	}

	for name, test := range tt {
		t.Run(name, test)
	}
}

func TestStore_Get(t *testing.T) {
	store := newEmptyTestStore()
	fault, exists := store.Get("")
	assert.Nil(t, fault, "expected nil fault")
	assert.False(t, exists, "expected fault to not exist")
}

func TestStore_Exists(t *testing.T) {
	store := newEmptyTestStore()
	exists := store.Exists("")
	assert.False(t, exists, "expected fault to not exist")
}

func TestStore_List(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	faultA, faultB := "faultA", "faultB"

	store := newTestStore(
		func() (string, Fault) {
			return faultA, mock_store.NewMockFault(mockCtrl)
		},
		func() (string, Fault) {
			return faultB, mock_store.NewMockFault(mockCtrl)
		},
	)
	list := store.List()
	assert.NotEmpty(t, list, "expected non-empty list")
	assert.Len(t, list, 2, "expected length to be 2")
}
