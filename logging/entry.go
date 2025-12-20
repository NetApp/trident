// Copyright 2025 NetApp, Inc. All Rights Reserved.

package logging

import (
	log "github.com/sirupsen/logrus"
)

const (
	LogSource = "logSource"

	AuditRESTAccess   = AuditEvent("rest")
	AuditGRPCAccess   = AuditEvent("csi")
	AuditDockerAccess = AuditEvent("docker")
)

type LogFields log.Fields

type LogEntry interface {
	WithField(key string, value interface{}) LogEntry
	WithFields(fields LogFields) LogEntry
	WithError(err error) LogEntry
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Warn(args ...interface{})
	Warnf(format string, args ...interface{})
	Warning(args ...interface{})
	Warningf(format string, args ...interface{})
	Info(args ...interface{})
	Infof(format string, args ...interface{})
	Debug(args ...interface{})
	Debugf(format string, args ...interface{})
	Trace(args ...interface{})
	Tracef(format string, args ...interface{})
	Data(key string) (interface{}, bool)
}

type logEntry struct {
	entry        *log.Entry
	dynamicLevel log.Level
}

func newLogEntry(fields log.Fields) LogEntry {
	return &logEntry{
		entry:        log.WithFields(fields),
		dynamicLevel: defaultLogLevel,
	}
}

func (le *logEntry) getEntryFromEntry(entry *log.Entry) LogEntry {
	return &logEntry{
		entry:        entry,
		dynamicLevel: le.dynamicLevel,
	}
}

func (le *logEntry) WithField(key string, value interface{}) LogEntry {
	return le.getEntryFromEntry(le.entry.WithField(key, value))
}

func (le *logEntry) WithFields(fields LogFields) LogEntry {
	return le.getEntryFromEntry(le.entry.WithFields(log.Fields(fields)))
}

func (le *logEntry) WithError(err error) LogEntry {
	return le.getEntryFromEntry(le.entry.WithError(err))
}

func (le *logEntry) Fatal(args ...interface{}) {
	le.log(log.FatalLevel, args...)
	le.entry.Logger.Exit(1)
}

func (le *logEntry) Fatalf(format string, args ...interface{}) {
	le.logf(log.FatalLevel, format, args...)
	le.entry.Logger.Exit(1)
}

func (le *logEntry) Error(args ...interface{}) {
	le.log(log.ErrorLevel, args...)
}

func (le *logEntry) Errorf(format string, args ...interface{}) {
	le.logf(log.ErrorLevel, format, args...)
}

func (le *logEntry) Warn(args ...interface{}) {
	le.log(log.WarnLevel, args...)
}

func (le *logEntry) Warnf(format string, args ...interface{}) {
	le.logf(log.WarnLevel, format, args...)
}

func (le *logEntry) Warning(args ...interface{}) {
	le.log(log.WarnLevel, args...)
}

func (le *logEntry) Warningf(format string, args ...interface{}) {
	le.logf(log.WarnLevel, format, args...)
}

func (le *logEntry) Info(args ...interface{}) {
	le.log(log.InfoLevel, args...)
}

func (le *logEntry) Infof(format string, args ...interface{}) {
	le.logf(log.InfoLevel, format, args...)
}

func (le *logEntry) Debug(args ...interface{}) {
	le.log(log.DebugLevel, args...)
}

func (le *logEntry) Debugf(format string, args ...interface{}) {
	le.logf(log.DebugLevel, format, args...)
}

func (le *logEntry) Trace(args ...interface{}) {
	le.log(log.TraceLevel, args...)
}

func (le *logEntry) Tracef(format string, args ...interface{}) {
	le.logf(log.TraceLevel, format, args...)
}

func (le *logEntry) Data(key string) (interface{}, bool) {
	val, ok := le.entry.Data[key]
	return val, ok
}

func (le *logEntry) log(level log.Level, args ...interface{}) {
	if le.dynamicLevel >= level {
		le.entry.Log(level, args...)
	}
}

func (le *logEntry) logf(level log.Level, format string, args ...interface{}) {
	if le.dynamicLevel >= level {
		le.entry.Logf(level, format, args...)
	}
}
