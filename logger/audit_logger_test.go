package logger

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestAuditLoggerHasAuditEventWhenEnabled(t *testing.T) {
	InitAuditLogger(false)

	assert.True(t, log.IsLevelEnabled(log.InfoLevel), "Expected audit logger to be info level or higher when enabled.")

	output := captureOutput(t, func() { Audit().Log(context.Background(), AuditGRPCAccess, log.Fields{}, "Foo.") })
	assert.True(t, len(output) > 0, "Expected output from the audit logger when enabled.")
	assert.Contains(t, output, auditKey, fmt.Sprintf("Expected the auditKey: %s to be present in the output.", auditKey))
}

func TestAuditLoggerDoesNothingWhenDisabled(t *testing.T) {
	InitAuditLogger(true)

	output := captureOutput(t, func() { Audit().Log(context.Background(), AuditGRPCAccess, log.Fields{}, "Foo.") })
	assert.True(t, len(output) == 0, "Expected no output from the audit logger when disabled.")
}

func captureOutput(t *testing.T, f func()) string {
	var buf bytes.Buffer
	origOut := log.WithContext(context.Background()).Logger.Out
	log.SetOutput(&buf)
	defer log.SetOutput(origOut)
	f()
	return buf.String()
}

func getLogger(t *testing.T) *auditLogger {
	logr, ok := auditor.(*auditLogger)
	if !ok {
		assert.Fail(t, "Auditor could not be cast to an auditLogger struct type!")
	}
	return logr
}
