// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cosi

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	. "github.com/netapp/trident/logging"
)

// setupLogging redirects Trident's logger into a buffer for the duration of a
// test. These tests mutate package-level logging state, so they must not run in
// parallel with each other.
func setupLogging(t *testing.T, level, layers string) *bytes.Buffer {
	t.Helper()

	originalLevel := GetDefaultLogLevel()

	if err := InitLogFormat("text"); err != nil {
		t.Fatalf("setting log format: %v", err)
	}
	if err := InitLogLevel(level); err != nil {
		t.Fatalf("setting log level: %v", err)
	}
	if err := SetLogLayers(layers); err != nil {
		t.Fatalf("setting log layers: %v", err)
	}

	var buf bytes.Buffer
	InitLogOutput(&buf)

	t.Cleanup(func() {
		_ = SetLogLayers("")
		_ = InitLogLevel(originalLevel)
		InitLogOutput(os.Stdout)
	})

	return &buf
}

// TestLayerSuppressesDetailByDefault and TestLayerSelectionEnablesTrace are the
// pair that justify hand-writing this sink instead of using a generic
// logr/logrus bridge: COSI logs have to respond to Trident's per-layer log
// selection the way every other subsystem does.
func TestLayerSuppressesDetailByDefault(t *testing.T) {
	buf := setupLogging(t, "info", "")

	newLogger().V(debugVLevel).Info("per-reconcile detail")

	if strings.Contains(buf.String(), "per-reconcile detail") {
		t.Errorf("V(1) must be suppressed at info level, got: %s", buf.String())
	}
}

func TestLayerSelectionEnablesTrace(t *testing.T) {
	buf := setupLogging(t, "info", "cosi_frontend")

	newLogger().V(traceVLevel).Info("deep reconcile detail")

	out := buf.String()
	if !strings.Contains(out, "deep reconcile detail") {
		t.Errorf("selecting the cosi_frontend layer must enable trace for COSI logs, got: %s", out)
	}
	if !strings.Contains(out, "cosi_frontend") {
		t.Errorf("log entry should be tagged with the COSI layer, got: %s", out)
	}
}

func TestLevelMapping(t *testing.T) {
	tests := []struct {
		name  string
		level int
		want  string
	}{
		{"V(0) is info", 0, "level=info"},
		{"V(1) is debug", debugVLevel, "level=debug"},
		{"V(2) is trace", traceVLevel, "level=trace"},
		{"V(5) is still trace", 5, "level=trace"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := setupLogging(t, "trace", "")

			newLogger().V(tc.level).Info("message")

			if !strings.Contains(buf.String(), tc.want) {
				t.Errorf("got %q, want it to contain %q", buf.String(), tc.want)
			}
		})
	}
}

func TestErrorIsAlwaysLogged(t *testing.T) {
	// Info level, no layer selected: the quietest configuration a reconciler
	// runs under. Errors must still reach the log.
	buf := setupLogging(t, "info", "")

	newLogger().Error(errors.New("driver unreachable"), "provisioning failed")

	out := buf.String()
	if !strings.Contains(out, "level=error") || !strings.Contains(out, "provisioning failed") {
		t.Errorf("error not logged, got: %s", out)
	}
	if !strings.Contains(out, "driver unreachable") {
		t.Errorf("wrapped error text missing, got: %s", out)
	}
}

func TestWithNameAccumulates(t *testing.T) {
	buf := setupLogging(t, "info", "")

	// controller-runtime nests names; the reconcilers name themselves, which is
	// what distinguishes the four in one log stream.
	newLogger().WithName("cosi").WithName("bucket").Info("reconciling")

	if !strings.Contains(buf.String(), "cosi.bucket") {
		t.Errorf("expected accumulated logger name, got: %s", buf.String())
	}
}

func TestWithValuesDoesNotMutateParent(t *testing.T) {
	buf := setupLogging(t, "info", "")

	parent := newLogger().WithValues("shared", "yes")
	_ = parent.WithValues("childOnly", "true")
	parent.Info("parent message")

	out := buf.String()
	if !strings.Contains(out, "shared") {
		t.Errorf("parent lost its own field, got: %s", out)
	}
	if strings.Contains(out, "childOnly") {
		t.Errorf("child field leaked into parent, got: %s", out)
	}
}

func TestOddKeyCountIsVisible(t *testing.T) {
	buf := setupLogging(t, "info", "")

	newLogger().Info("message", "pairedKey", "value", "danglingKey")

	out := buf.String()
	if !strings.Contains(out, "pairedKey") {
		t.Errorf("paired field missing, got: %s", out)
	}
	// A caller's mistake should show up rather than disappear silently.
	if !strings.Contains(out, "danglingKey") {
		t.Errorf("dangling key should still be logged, got: %s", out)
	}
}
