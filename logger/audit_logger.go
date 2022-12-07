package logger

import (
	"context"

	log "github.com/sirupsen/logrus"
)

const auditKey = "audit"

var auditor AuditLogger

type auditLogger struct{ enabled bool }

func InitAuditLogger(disabled bool) {
	auditor = newAuditLogger(disabled)
}

func Audit() AuditLogger {
	return auditor
}

func newAuditLogger(disabled bool) AuditLogger {
	logr := &auditLogger{}
	logr.enabled = !disabled

	if !disabled {
		// Enforce info level when auditing is enabled. The audit logger isn't a separate logger because it is writing
		// to the same output, and that would require lock management.
		// TODO when all logging calls are done through the logging package, separate the audit logger into a separate
		// one and add a package-level logging lock.
		if !log.IsLevelEnabled(log.InfoLevel) {
			log.Panic("Audit logging cannot be enabled if the log level is set lower than info.")
		}
	}

	return logr
}

func (a *auditLogger) Log(ctx context.Context, event AuditEvent, fields log.Fields, message string) {
	if a.enabled {
		log.WithContext(ctx).WithField(auditKey, event).WithFields(fields).Info(message)
	}
}

func (a *auditLogger) Logln(ctx context.Context, event AuditEvent, fields log.Fields, message string) {
	if a.enabled {
		log.WithContext(ctx).WithField(auditKey, event).WithFields(fields).Infoln(message)
	}
}

func (a *auditLogger) Logf(ctx context.Context, event AuditEvent, fields log.Fields, format string, args ...interface{}) {
	if a.enabled {
		log.WithContext(ctx).WithField(auditKey, event).WithFields(fields).Infof(format, args...)
	}
}
