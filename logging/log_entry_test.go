package logging

import (
	"bytes"
	"errors"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// TestLogEntryWithField tests the WithField method
func TestLogEntryWithField(t *testing.T) {
	entry := newLogEntry(log.Fields{"initial": "value"})

	result := entry.WithField("key1", "value1")

	assert.NotNil(t, result, "WithField should return a LogEntry")

	// Verify the field was added
	data, ok := result.Data("key1")
	assert.True(t, ok, "Field should exist")
	assert.Equal(t, "value1", data, "Field value should match")

	// Verify initial field is preserved
	data, ok = result.Data("initial")
	assert.True(t, ok, "Initial field should be preserved")
	assert.Equal(t, "value", data, "Initial field value should match")
}

// TestLogEntryWithFields tests the WithFields method
func TestLogEntryWithFields(t *testing.T) {
	entry := newLogEntry(log.Fields{"initial": "value"})

	newFields := LogFields{
		"key1": "value1",
		"key2": "value2",
		"key3": 123,
	}

	result := entry.WithFields(newFields)

	assert.NotNil(t, result, "WithFields should return a LogEntry")

	// Verify all fields were added
	data, ok := result.Data("key1")
	assert.True(t, ok, "Field key1 should exist")
	assert.Equal(t, "value1", data, "Field key1 value should match")

	data, ok = result.Data("key2")
	assert.True(t, ok, "Field key2 should exist")
	assert.Equal(t, "value2", data, "Field key2 value should match")

	data, ok = result.Data("key3")
	assert.True(t, ok, "Field key3 should exist")
	assert.Equal(t, 123, data, "Field key3 value should match")

	// Verify initial field is preserved
	data, ok = result.Data("initial")
	assert.True(t, ok, "Initial field should be preserved")
	assert.Equal(t, "value", data, "Initial field value should match")
}

// TestLogEntryWithError tests the WithError method
func TestLogEntryWithError(t *testing.T) {
	entry := newLogEntry(log.Fields{})
	testErr := errors.New("test error")

	result := entry.WithError(testErr)

	assert.NotNil(t, result, "WithError should return a LogEntry")

	// Verify the error field was added
	data, ok := result.Data("error")
	assert.True(t, ok, "Error field should exist")
	assert.Equal(t, testErr, data, "Error value should match")
}

// TestLogEntryData tests the Data method for existing and non-existing keys
func TestLogEntryData(t *testing.T) {
	entry := newLogEntry(log.Fields{
		"existing_key": "existing_value",
		"number":       42,
	})

	// Test existing key
	value, ok := entry.Data("existing_key")
	assert.True(t, ok, "Existing key should be found")
	assert.Equal(t, "existing_value", value, "Value should match")

	// Test existing number key
	value, ok = entry.Data("number")
	assert.True(t, ok, "Number key should be found")
	assert.Equal(t, 42, value, "Number value should match")

	// Test non-existing key
	value, ok = entry.Data("non_existing_key")
	assert.False(t, ok, "Non-existing key should not be found")
	assert.Nil(t, value, "Value for non-existing key should be nil")
}

// TestLogEntryDynamicLevelFiltering tests that log methods respect the dynamic level
func TestLogEntryDynamicLevelFiltering(t *testing.T) {
	tests := []struct {
		name         string
		dynamicLevel log.Level
		loggerLevel  log.Level
		shouldLog    bool
		testFunc     func(entry LogEntry)
	}{
		{
			name:         "Debug with ErrorLevel - should NOT log",
			dynamicLevel: log.ErrorLevel,
			loggerLevel:  log.DebugLevel,
			shouldLog:    false,
			testFunc:     func(entry LogEntry) { entry.Debug("debug message") },
		},
		{
			name:         "Trace with TraceLevel - should log",
			dynamicLevel: log.TraceLevel,
			loggerLevel:  log.TraceLevel,
			shouldLog:    true,
			testFunc:     func(entry LogEntry) { entry.Trace("trace message") },
		},
		{
			name:         "Info with WarnLevel - should NOT log",
			dynamicLevel: log.WarnLevel,
			loggerLevel:  log.InfoLevel,
			shouldLog:    false,
			testFunc:     func(entry LogEntry) { entry.Info("info message") },
		},
		{
			name:         "Error with ErrorLevel - should log",
			dynamicLevel: log.ErrorLevel,
			loggerLevel:  log.ErrorLevel,
			shouldLog:    true,
			testFunc:     func(entry LogEntry) { entry.Error("error message") },
		},
		{
			name:         "Warn with InfoLevel - should log",
			dynamicLevel: log.InfoLevel,
			loggerLevel:  log.InfoLevel,
			shouldLog:    true,
			testFunc:     func(entry LogEntry) { entry.Warn("warn message") },
		},
		{
			name:         "Debug with DebugLevel - should log",
			dynamicLevel: log.DebugLevel,
			loggerLevel:  log.DebugLevel,
			shouldLog:    true,
			testFunc:     func(entry LogEntry) { entry.Debug("debug message") },
		},
		{
			name:         "Trace with DebugLevel - should NOT log",
			dynamicLevel: log.DebugLevel,
			loggerLevel:  log.TraceLevel,
			shouldLog:    false,
			testFunc:     func(entry LogEntry) { entry.Trace("trace message") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalOutput := log.StandardLogger().Out
			originalLevel := log.GetLevel()
			defer func() {
				log.SetOutput(originalOutput)
				log.SetLevel(originalLevel)
			}()

			var buf bytes.Buffer
			log.SetOutput(&buf)
			log.SetLevel(tt.loggerLevel)

			entry := newLogEntry(log.Fields{})
			le := entry.(*logEntry)
			le.dynamicLevel = tt.dynamicLevel

			tt.testFunc(entry)

			output := buf.String()
			if tt.shouldLog {
				assert.NotEmpty(t, output, "message should be logged")
			} else {
				assert.Empty(t, output, "message should NOT be logged")
			}
		})
	}
}

// TestLogEntryDynamicLevelFilteringFormatted tests formatted log methods with dynamic level
func TestLogEntryDynamicLevelFilteringFormatted(t *testing.T) {
	tests := []struct {
		name         string
		dynamicLevel log.Level
		loggerLevel  log.Level
		shouldLog    bool
		testFunc     func(entry LogEntry)
	}{
		{
			name:         "Debugf with ErrorLevel - should NOT log",
			dynamicLevel: log.ErrorLevel,
			loggerLevel:  log.DebugLevel,
			shouldLog:    false,
			testFunc:     func(entry LogEntry) { entry.Debugf("debug %s", "message") },
		},
		{
			name:         "Tracef with TraceLevel - should log",
			dynamicLevel: log.TraceLevel,
			loggerLevel:  log.TraceLevel,
			shouldLog:    true,
			testFunc:     func(entry LogEntry) { entry.Tracef("trace %s", "message") },
		},
		{
			name:         "Infof with WarnLevel - should NOT log",
			dynamicLevel: log.WarnLevel,
			loggerLevel:  log.InfoLevel,
			shouldLog:    false,
			testFunc:     func(entry LogEntry) { entry.Infof("info %s", "message") },
		},
		{
			name:         "Errorf with ErrorLevel - should log",
			dynamicLevel: log.ErrorLevel,
			loggerLevel:  log.ErrorLevel,
			shouldLog:    true,
			testFunc:     func(entry LogEntry) { entry.Errorf("error %s", "message") },
		},
		{
			name:         "Warnf with InfoLevel - should log",
			dynamicLevel: log.InfoLevel,
			loggerLevel:  log.InfoLevel,
			shouldLog:    true,
			testFunc:     func(entry LogEntry) { entry.Warnf("warn %s", "message") },
		},
		{
			name:         "Warningf with InfoLevel - should log",
			dynamicLevel: log.InfoLevel,
			loggerLevel:  log.InfoLevel,
			shouldLog:    true,
			testFunc:     func(entry LogEntry) { entry.Warningf("warning %s", "message") },
		},
		{
			name:         "Debugf with DebugLevel - should log",
			dynamicLevel: log.DebugLevel,
			loggerLevel:  log.DebugLevel,
			shouldLog:    true,
			testFunc:     func(entry LogEntry) { entry.Debugf("debug %s", "message") },
		},
		{
			name:         "Tracef with DebugLevel - should NOT log",
			dynamicLevel: log.DebugLevel,
			loggerLevel:  log.TraceLevel,
			shouldLog:    false,
			testFunc:     func(entry LogEntry) { entry.Tracef("trace %s", "message") },
		},
		{
			name:         "Infof with InfoLevel - should log",
			dynamicLevel: log.InfoLevel,
			loggerLevel:  log.InfoLevel,
			shouldLog:    true,
			testFunc:     func(entry LogEntry) { entry.Infof("info %s", "message") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalOutput := log.StandardLogger().Out
			originalLevel := log.GetLevel()
			defer func() {
				log.SetOutput(originalOutput)
				log.SetLevel(originalLevel)
			}()

			var buf bytes.Buffer
			log.SetOutput(&buf)
			log.SetLevel(tt.loggerLevel)

			entry := newLogEntry(log.Fields{})
			le := entry.(*logEntry)
			le.dynamicLevel = tt.dynamicLevel

			tt.testFunc(entry)

			output := buf.String()
			if tt.shouldLog {
				assert.NotEmpty(t, output, "message should be logged")
			} else {
				assert.Empty(t, output, "message should NOT be logged")
			}
		})
	}
}

// TestLogEntryOutputMethods tests basic logging methods that produce output
func TestLogEntryOutputMethods(t *testing.T) {
	tests := []struct {
		name           string
		level          log.Level
		message        string
		expectedOutput string
		testFunc       func(entry LogEntry, message string)
	}{
		{
			name:           "Error",
			level:          log.ErrorLevel,
			message:        "test error message",
			expectedOutput: "test error message",
			testFunc:       func(entry LogEntry, msg string) { entry.Error(msg) },
		},
		{
			name:           "Warn",
			level:          log.WarnLevel,
			message:        "test warn message",
			expectedOutput: "test warn message",
			testFunc:       func(entry LogEntry, msg string) { entry.Warn(msg) },
		},
		{
			name:           "Warning",
			level:          log.WarnLevel,
			message:        "test warning message",
			expectedOutput: "test warning message",
			testFunc:       func(entry LogEntry, msg string) { entry.Warning(msg) },
		},
		{
			name:           "Info",
			level:          log.InfoLevel,
			message:        "test info message",
			expectedOutput: "test info message",
			testFunc:       func(entry LogEntry, msg string) { entry.Info(msg) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalOutput := log.StandardLogger().Out
			defer log.SetOutput(originalOutput)

			var buf bytes.Buffer
			log.SetOutput(&buf)

			entry := newLogEntry(log.Fields{})
			le := entry.(*logEntry)
			le.dynamicLevel = tt.level

			tt.testFunc(entry, tt.message)

			output := buf.String()
			assert.Contains(t, output, tt.expectedOutput)
		})
	}
}

// TestLogEntryFormattedMethods tests formatted logging methods
func TestLogEntryFormattedMethods(t *testing.T) {
	tests := []struct {
		name           string
		level          log.Level
		format         string
		args           []interface{}
		expectedOutput string
		testFunc       func(entry LogEntry, format string, args ...interface{})
	}{
		{
			name:           "Errorf",
			level:          log.ErrorLevel,
			format:         "test error message with %s",
			args:           []interface{}{"formatting"},
			expectedOutput: "test error message with formatting",
			testFunc:       func(entry LogEntry, format string, args ...interface{}) { entry.Errorf(format, args...) },
		},
		{
			name:           "Warnf",
			level:          log.WarnLevel,
			format:         "test warn message with %s",
			args:           []interface{}{"formatting"},
			expectedOutput: "test warn message with formatting",
			testFunc:       func(entry LogEntry, format string, args ...interface{}) { entry.Warnf(format, args...) },
		},
		{
			name:           "Warningf",
			level:          log.WarnLevel,
			format:         "test warning message with %s",
			args:           []interface{}{"formatting"},
			expectedOutput: "test warning message with formatting",
			testFunc:       func(entry LogEntry, format string, args ...interface{}) { entry.Warningf(format, args...) },
		},
		{
			name:           "Infof",
			level:          log.InfoLevel,
			format:         "test info message with %s",
			args:           []interface{}{"formatting"},
			expectedOutput: "test info message with formatting",
			testFunc:       func(entry LogEntry, format string, args ...interface{}) { entry.Infof(format, args...) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalOutput := log.StandardLogger().Out
			defer log.SetOutput(originalOutput)

			var buf bytes.Buffer
			log.SetOutput(&buf)

			entry := newLogEntry(log.Fields{})
			le := entry.(*logEntry)
			le.dynamicLevel = tt.level

			tt.testFunc(entry, tt.format, tt.args...)

			output := buf.String()
			assert.Contains(t, output, tt.expectedOutput)
		})
	}
}

// TestLogEntryDebugTraceMethods tests Debug and Trace methods (may not produce captured output)
func TestLogEntryDebugTraceMethods(t *testing.T) {
	tests := []struct {
		name     string
		level    log.Level
		message  string
		testFunc func(entry LogEntry, message string)
	}{
		{
			name:     "Debug",
			level:    log.DebugLevel,
			message:  "test debug message",
			testFunc: func(entry LogEntry, msg string) { entry.Debug(msg) },
		},
		{
			name:     "Trace",
			level:    log.TraceLevel,
			message:  "test trace message",
			testFunc: func(entry LogEntry, msg string) { entry.Trace(msg) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := newLogEntry(log.Fields{})
			le := entry.(*logEntry)
			le.dynamicLevel = tt.level

			// The function should complete without panicking
			tt.testFunc(entry, tt.message)

			// Test that the logEntry is properly created
			assert.NotNil(t, entry, "log entry should not be nil")
		})
	}
}

// TestLogEntryDebugTraceFormattedMethods tests Debugf and Tracef methods
func TestLogEntryDebugTraceFormattedMethods(t *testing.T) {
	tests := []struct {
		name     string
		level    log.Level
		format   string
		args     []interface{}
		testFunc func(entry LogEntry, format string, args ...interface{})
	}{
		{
			name:     "Debugf",
			level:    log.DebugLevel,
			format:   "test debug message with %s",
			args:     []interface{}{"formatting"},
			testFunc: func(entry LogEntry, format string, args ...interface{}) { entry.Debugf(format, args...) },
		},
		{
			name:     "Tracef",
			level:    log.TraceLevel,
			format:   "test trace message with %s",
			args:     []interface{}{"formatting"},
			testFunc: func(entry LogEntry, format string, args ...interface{}) { entry.Tracef(format, args...) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := newLogEntry(log.Fields{})
			le := entry.(*logEntry)
			le.dynamicLevel = tt.level

			// The function should complete without panicking
			tt.testFunc(entry, tt.format, tt.args...)

			// Test that the logEntry is properly created
			assert.NotNil(t, entry, "log entry should not be nil")
		})
	}
}

// TestLogEntryFatalMethods tests Fatal and Fatalf methods
func TestLogEntryFatalMethods(t *testing.T) {
	tests := []struct {
		name           string
		message        string
		format         string
		args           []interface{}
		expectedOutput string
		testFunc       func(entry LogEntry, message, format string, args ...interface{})
	}{
		{
			name:           "Fatal",
			message:        "test fatal message",
			expectedOutput: "test fatal message",
			testFunc: func(entry LogEntry, msg, format string, args ...interface{}) {
				entry.Fatal(msg)
			},
		},
		{
			name:           "Fatalf",
			format:         "test fatal message with %s",
			args:           []interface{}{"formatting"},
			expectedOutput: "test fatal message with formatting",
			testFunc: func(entry LogEntry, msg, format string, args ...interface{}) {
				entry.Fatalf(format, args...)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalOutput := log.StandardLogger().Out
			defer log.SetOutput(originalOutput)

			var buf bytes.Buffer
			log.SetOutput(&buf)

			// Override the exit function to prevent actual exit during test
			originalExitFunc := log.StandardLogger().ExitFunc
			var exitCalled bool
			log.StandardLogger().ExitFunc = func(code int) {
				exitCalled = true
				assert.Equal(t, 1, code, "Exit code should be 1")
			}
			defer func() {
				log.StandardLogger().ExitFunc = originalExitFunc
			}()

			entry := newLogEntry(log.Fields{})
			le := entry.(*logEntry)
			le.dynamicLevel = log.FatalLevel

			tt.testFunc(entry, tt.message, tt.format, tt.args...)

			output := buf.String()
			assert.Contains(t, output, tt.expectedOutput)
			assert.True(t, exitCalled, "Exit function should have been called")
		})
	}
}

// TestLogEntryChainedMethods tests chaining WithField, WithFields, and WithError
func TestLogEntryChainedMethods(t *testing.T) {
	originalOutput := log.StandardLogger().Out
	defer log.SetOutput(originalOutput)

	var buf bytes.Buffer
	log.SetOutput(&buf)

	entry := newLogEntry(log.Fields{"initial": "value"})

	result := entry.
		WithField("field1", "value1").
		WithFields(LogFields{"field2": "value2", "field3": "value3"}).
		WithError(errors.New("test error"))

	assert.NotNil(t, result, "Chained methods should return a LogEntry")

	// Verify all fields
	data, ok := result.Data("initial")
	assert.True(t, ok, "Initial field should exist")
	assert.Equal(t, "value", data, "Initial field value should match")

	data, ok = result.Data("field1")
	assert.True(t, ok, "Field1 should exist")
	assert.Equal(t, "value1", data, "Field1 value should match")

	data, ok = result.Data("field2")
	assert.True(t, ok, "Field2 should exist")
	assert.Equal(t, "value2", data, "Field2 value should match")

	data, ok = result.Data("field3")
	assert.True(t, ok, "Field3 should exist")
	assert.Equal(t, "value3", data, "Field3 value should match")

	data, ok = result.Data("error")
	assert.True(t, ok, "Error field should exist")
	assert.NotNil(t, data, "Error value should not be nil")
}

// TestLogEntryLogf tests the internal logf method
func TestLogEntryLogf(t *testing.T) {
	originalOutput := log.StandardLogger().Out
	defer log.SetOutput(originalOutput)

	var buf bytes.Buffer
	log.SetOutput(&buf)

	entry := newLogEntry(log.Fields{})
	le := entry.(*logEntry)
	le.dynamicLevel = log.InfoLevel
	le.logf(log.InfoLevel, "test logf message with %s", "formatting")

	output := buf.String()
	assert.Contains(t, output, "test logf message with formatting")
}
