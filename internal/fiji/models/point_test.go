package models

import (
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	. "github.com/netapp/trident/logging"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

type fakeStore struct {
	lock *sync.RWMutex
	data map[string]*FaultPoint
}

func newFakeStore(faults ...*FaultPoint) *fakeStore {
	data := make(map[string]*FaultPoint, len(faults))
	for _, fault := range faults {
		data[fault.Name] = fault
	}

	return &fakeStore{
		lock: &sync.RWMutex{}, data: data,
	}
}

func (s *fakeStore) Update(name string, model []byte) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	fault, ok := s.data[name]
	if !ok {
		return fmt.Errorf("test fault \"%s\" not found", name)
	}
	return fault.SetHandler(model)
}

func TestFaultPoint_Inject(t *testing.T) {
	point := NewFaultPoint("name", "unit_test")
	assert.NoError(t, point.Inject())
}

func TestFaultPoint_SetHandler(t *testing.T) {
	point := NewFaultPoint("name", "unit_test")
	assert.Error(t, point.SetHandler(nil))

	model := `{"name":"pause", "duration": 300}`
	err := point.SetHandler([]byte(model))
	assert.Error(t, err)

	model = `{"name":"pause","duration":"300ms"}`
	err = point.SetHandler([]byte(model))
	assert.NoError(t, err)
}

// TestFaultPoint_SetHandlerConfig_Async tests that it is safe to use a mutex lock on the fault points
// when consumers try to modify it's internal state. Consumers will have to wait for the
// current inject() call to finish before being able to modify the handler config.
func TestFaultPoint_SetHandler_Async(t *testing.T) {
	point := NewFaultPoint("name", "unit_test")

	// Create a store that has access to the fault point above and a mutex.
	store := newFakeStore(point)
	start := time.Now()
	duration := 100 * time.Millisecond
	modelStr := fmt.Sprintf(`{"name": "pause", "duration": "%s"}`, duration.String())
	err := point.SetHandler([]byte(modelStr))
	assert.NoError(t, err)

	// This starts a routine that will "inject" a pause.
	var errFromSleep error
	go func() {
		// This should induce a sleep for the length of "duration" that holds the point's mutex.
		errFromSleep = point.Inject()
	}()
	// Add a short sleep here to ensure the go routine has time to
	// call "point.Inject" before resetting the handler config.
	time.Sleep(25 * time.Millisecond)

	// The points original handler will sleep, causing the points mutex to be in use.
	err = store.Update(point.Name, []byte(`{"name": "always"}`))
	assert.NoError(t, err)

	// This assertion ensures the store has to wait for the points mutex to unlock before changing the handler.
	after := time.Since(start)
	assert.GreaterOrEqual(t, after, duration)
	afterChangingHandler := time.Now()

	// Ensure the new handler doesn't sleep.
	err = point.Inject()
	assert.LessOrEqual(t, time.Since(afterChangingHandler), duration)
	assert.NotNil(t, err)
	assert.NotEqual(t, err, errFromSleep)
}
