//go:build fiji

package fiji

import (
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
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

func TestFIJI_isValidName(t *testing.T) {
	t.Run("with valid input data", func(t *testing.T) {
		validData := []string{
			"faultID1",
			"fault-ID456",
			"faultAfterAttachBeforeEnsureLUKSPassphrase",
		}

		for _, data := range validData {
			assert.True(t, isValidName(data))
		}
	})

	// Test validate with strings that contain disallowed and reserved URI characters.
	t.Run("with invalid input data", func(t *testing.T) {
		invalidData := []string{
			"faultID@!",
			"fault$id",
			"fault_ID<",
			"fault ID",
			"fault.ID",
			"fault~ID",
			"fault_ID456",
			" ",
		}

		for _, data := range invalidData {
			assert.False(t, isValidName(data))
		}
	})
}

func TestFIJI_Register(t *testing.T) {
	tt := map[string]func(*testing.T){
		"with invalid name specified": func(t *testing.T) {
			name := "faultID@!"
			assert.Panics(t, func() { _ = Register(name, "test") }, "expected panic")
		},
		"with new fault name and duplicate fault name": func(t *testing.T) {
			name := "fault-ID456"
			fault := Register(name, "test")
			assert.NotNil(t, fault, "expected non-nil fault")
			// assert.Equal(t, name, fault.Name)
			assert.Panics(t, func() { _ = Register(name, "test") }, "expected panic")
		},
	}

	for name, test := range tt {
		t.Run(name, test)
	}
}

func TestFIJI_RegisterIsAsyncSafe(t *testing.T) {
	totalRegistrations := 500

	// Create the test cases.
	type testCase struct {
		name, location string
	}
	location := "unit_test"
	tests := make([]testCase, 0, totalRegistrations)

	// Create the test case input data.
	for i := 0; i < totalRegistrations; i++ {
		tests = append(tests, testCase{fmt.Sprintf("fault-%v", i), location})
	}
	assert.Len(t, tests, totalRegistrations)

	// Create concurrent routines with some variance in when they fire.
	wg := sync.WaitGroup{}
	for _, fault := range tests {
		wg.Add(1)
		go func() {
			// Sleep for a small amount of time to increase the chance
			// of overlapping calls to Register.
			time.Sleep(getTestJitter(t, int64(50*time.Microsecond)))
			Register(fault.name, fault.location)
			wg.Done()
		}()
	}
	wg.Wait()

	// Ensure each fault exists in the store.
	for _, fault := range tests {
		assert.True(t, faultStore.Exists(fault.name))
	}
}

func getTestJitter(t *testing.T, input int64) time.Duration {
	t.Helper()

	n, err := rand.Int(rand.Reader, big.NewInt(input))
	if err != nil {
		t.Fatalf("failed to calculate test jitter")
	}
	return time.Duration(n.Int64())
}
