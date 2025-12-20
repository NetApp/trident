// Copyright 2025 NetApp, Inc. All Rights Reserved.

package logging

import (
	"context"
)

const auditKey = "audit"

var auditor AuditLogger

type AuditEvent string

type AuditLogger interface {
	Log(ctx context.Context, event AuditEvent, fields LogFields, message string)
	Logln(ctx context.Context, event AuditEvent, fields LogFields, message string)
	Logf(ctx context.Context, event AuditEvent, fields LogFields, format string, args ...interface{})
}

type auditLogger struct {
	enabled bool
}

func InitAuditLogger(disabled bool) {
	auditor = newAuditLogger(disabled)
}

func Audit() AuditLogger {
	return auditor
}

func newAuditLogger(disabled bool) AuditLogger {
	logr := &auditLogger{}
	logr.enabled = !disabled

	return logr
}

func (a *auditLogger) Log(ctx context.Context, event AuditEvent, fields LogFields, message string) {
	if a.enabled {
		ctx = context.WithValue(ctx, auditKey, event)
		Logc(ctx).WithFields(fields).Info(message)
	}
}

func (a *auditLogger) Logln(ctx context.Context, event AuditEvent, fields LogFields, message string) {
	if a.enabled {
		ctx = context.WithValue(ctx, auditKey, event)
		a.Log(ctx, event, fields, message+"\n")
	}
}

func (a *auditLogger) Logf(ctx context.Context, event AuditEvent, fields LogFields, format string, args ...interface{}) {
	if a.enabled {
		ctx = context.WithValue(ctx, auditKey, event)
		Logc(ctx).WithFields(fields).Infof(format, args...)
	}
}
