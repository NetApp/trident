package logging

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestAuditLoggerHasAuditEventWhenEnabled(t *testing.T) {
	InitAuditLogger(false)

	defer func(level log.Level) { defaultLogLevel = level }(defaultLogLevel)
	// Set the default log level to panic to make sure that audit log statements are written even if the log level is
	// lower than info.
	defaultLogLevel = log.PanicLevel

	output := captureOutput(t, func() { Audit().Log(context.Background(), AuditGRPCAccess, LogFields{}, "Foo.") })
	assert.True(t, len(output) > 0, "Expected output from the audit logger when enabled.")
	assert.Contains(t, output, auditKey, fmt.Sprintf("Expected the auditKey: %s to be present in the output.", auditKey))
}

func TestAuditLoggerDoesNothingWhenDisabled(t *testing.T) {
	InitAuditLogger(true)

	output := captureOutput(t, func() { Audit().Log(context.Background(), AuditGRPCAccess, LogFields{}, "Foo.") })
	assert.True(t, len(output) == 0, "Expected no output from the audit logger when disabled.")
}

func TestAuditLoggerLogln(t *testing.T) {
	InitAuditLogger(false)

	defer func(level log.Level) { defaultLogLevel = level }(defaultLogLevel)
	// Set the default log level to panic to make sure that audit log statements are written even if the log level is
	// lower than info.
	defaultLogLevel = log.PanicLevel

	output := captureOutput(t, func() { Audit().Logln(context.Background(), AuditGRPCAccess, LogFields{}, "Test message") })
	assert.True(t, len(output) > 0, "Expected output from the audit logger when enabled.")
	assert.Contains(t, output, auditKey, fmt.Sprintf("Expected the auditKey: %s to be present in the output.", auditKey))
	assert.Contains(t, output, "Test message", "Expected message to be present in the output.")
}

func TestAuditLoggerLoglnWhenDisabled(t *testing.T) {
	InitAuditLogger(true)

	output := captureOutput(t, func() { Audit().Logln(context.Background(), AuditGRPCAccess, LogFields{}, "Test message") })
	assert.True(t, len(output) == 0, "Expected no output from the audit logger when disabled.")
}

func TestAuditLoggerLogf(t *testing.T) {
	InitAuditLogger(false)

	defer func(level log.Level) { defaultLogLevel = level }(defaultLogLevel)
	// Set the default log level to panic to make sure that audit log statements are written even if the log level is
	// lower than info.
	defaultLogLevel = log.PanicLevel

	output := captureOutput(t, func() {
		Audit().Logf(context.Background(), AuditGRPCAccess, LogFields{}, "Test message with %s", "formatting")
	})
	assert.True(t, len(output) > 0, "Expected output from the audit logger when enabled.")
	assert.Contains(t, output, auditKey, fmt.Sprintf("Expected the auditKey: %s to be present in the output.", auditKey))
	assert.Contains(t, output, "Test message with formatting", "Expected formatted message to be present in the output.")
}

func TestAuditLoggerLogfWhenDisabled(t *testing.T) {
	InitAuditLogger(true)

	output := captureOutput(t, func() {
		Audit().Logf(context.Background(), AuditGRPCAccess, LogFields{}, "Test message with %s", "formatting")
	})
	assert.True(t, len(output) == 0, "Expected no output from the audit logger when disabled.")
}

func captureOutput(t *testing.T, f func()) string {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(io.Discard)
	f()
	return buf.String()
}
