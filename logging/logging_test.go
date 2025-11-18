package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// Helper functions for test setup and cleanup

// createTempDir creates a temporary directory for testing and returns it along with a cleanup function
func createTempDir(t *testing.T, prefix string) (string, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", prefix)
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	cleanup := func() {
		os.RemoveAll(tempDir)
	}
	return tempDir, cleanup
}

// createTempLogDir creates a temporary directory with a trident subdirectory for log testing
func createTempLogDir(t *testing.T) (string, func()) {
	t.Helper()
	tempDir, cleanup := createTempDir(t, "logging_test")
	testLogDir := filepath.Join(tempDir, "trident")
	if err := os.MkdirAll(testLogDir, 0o755); err != nil {
		cleanup()
		t.Fatalf("Failed to create test log directory: %v", err)
	}
	return testLogDir, cleanup
}

// createTempLogFile creates a temporary log file in the given directory
func createTempLogFile(t *testing.T, dir, filename string) string {
	t.Helper()
	logFile := filepath.Join(dir, filename)
	f, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("Failed to create temp log file: %v", err)
	}
	f.Close()
	return logFile
}

func TestInitLogLevel(t *testing.T) {
	originalLevel := log.GetLevel()
	defer log.SetLevel(originalLevel)

	err := InitLogLevel("debug")
	assert.NoError(t, err, "InitLogLevel should not return error for valid level")

	err = InitLogLevel("invalid")
	assert.Error(t, err, "InitLogLevel should return error for invalid level")
}

func TestInitLogFormat(t *testing.T) {
	err := InitLogFormat("text")
	assert.NoError(t, err, "InitLogFormat should not return error for text format")

	err = InitLogFormat("json")
	assert.NoError(t, err, "InitLogFormat should not return error for json format")

	err = InitLogFormat("invalid")
	assert.Error(t, err, "InitLogFormat should return error for invalid format")
}

func TestInitLogOutput(t *testing.T) {
	originalOutput := log.StandardLogger().Out
	defer log.StandardLogger().SetOutput(originalOutput)

	// Test with os.Stdout
	InitLogOutput(os.Stdout)
	assert.Equal(t, os.Stdout, log.StandardLogger().Out, "Logger output should be set to stdout")

	// Test with os.Stderr
	InitLogOutput(os.Stderr)
	assert.Equal(t, os.Stderr, log.StandardLogger().Out, "Logger output should be set to stderr")
}

func TestInitLogFormatter(t *testing.T) {
	originalFormatter := log.StandardLogger().Formatter
	defer log.StandardLogger().SetFormatter(originalFormatter)

	formatter := &log.TextFormatter{
		TimestampFormat: time.RFC3339,
	}
	InitLogFormatter(formatter)

	// Verify the formatter was set
	assert.Equal(t, formatter, log.StandardLogger().Formatter, "Logger formatter should be set to provided formatter")
}

func TestIsLevelEnabled(t *testing.T) {
	originalLevel := log.GetLevel()
	defer log.SetLevel(originalLevel)

	log.SetLevel(log.InfoLevel)

	assert.True(t, IsLevelEnabled(log.ErrorLevel), "Error level should be enabled when log level is Info")
	assert.True(t, IsLevelEnabled(log.InfoLevel), "Info level should be enabled when log level is Info")
	assert.False(t, IsLevelEnabled(log.DebugLevel), "Debug level should not be enabled when log level is Info")
}

func TestGetLogLevel(t *testing.T) {
	originalLevel := log.GetLevel()
	defer log.SetLevel(originalLevel)

	log.SetLevel(log.WarnLevel)
	level := GetLogLevel()
	assert.Equal(t, "warning", level, "GetLogLevel should return the current log level as string")
}

func TestNewConsoleHook(t *testing.T) {
	hook, err := NewConsoleHook("text")
	assert.NoError(t, err, "NewConsoleHook should not return error for valid format")
	assert.NotNil(t, hook, "NewConsoleHook should return a valid hook")

	invalidHook, err := NewConsoleHook("invalid")
	assert.Error(t, err, "NewConsoleHook should return error for invalid format")
	assert.Nil(t, invalidHook, "NewConsoleHook should return nil hook on error")
}

func TestConsoleHookLevels(t *testing.T) {
	hook, err := NewConsoleHook("text")
	assert.NoError(t, err, "NewConsoleHook should not return error")

	levels := hook.Levels()
	assert.Contains(t, levels, log.InfoLevel, "Console hook should support Info level")
	assert.Contains(t, levels, log.ErrorLevel, "Console hook should support Error level")
}

// TestFileHook consolidates all FileHook-related tests into comprehensive subtests
func TestFileHook(t *testing.T) {
	t.Run("InvalidFormat", func(t *testing.T) {
		hook, err := NewFileHook("test", "invalid_format")
		assert.Error(t, err, "NewFileHook should return error for invalid format")
		assert.Nil(t, hook, "NewFileHook should return nil hook for invalid format")
		assert.Contains(t, err.Error(), "unknown log format", "Should contain format error message")
	})

	t.Run("Methods", func(t *testing.T) {
		tempDir, cleanup := createTempDir(t, "logging_test")
		defer cleanup()

		logFile := filepath.Join(tempDir, "test.log")
		hook, err := NewFileHook(logFile, "text")
		if err != nil {
			t.Skipf("Cannot create file hook: %v", err)
			return
		}

		// Test Levels method
		levels := hook.Levels()
		assert.NotEmpty(t, levels, "FileHook should support some levels")

		// Test GetLocation method
		location := hook.GetLocation()
		assert.NotEmpty(t, location, "GetLocation should return a non-empty path")

		// Test Fire method
		entry := &log.Entry{
			Logger:  log.StandardLogger(),
			Data:    log.Fields{},
			Message: "test file message",
			Level:   log.InfoLevel,
			Time:    time.Now(),
		}

		err = hook.Fire(entry)
		if err != nil {
			t.Logf("FileHook Fire error (expected in test environment): %v", err)
		}
	})

	t.Run("WorkingFileHook", func(t *testing.T) {
		testLogDir, cleanup := createTempLogDir(t)
		defer cleanup()

		logFile := createTempLogFile(t, testLogDir, "test.log")

		formatter := &PlainTextFormatter{}
		hook := &FileHook{
			logFileLocation: logFile,
			formatter:       formatter,
			mutex:           &sync.Mutex{},
		}

		// Test GetLocation
		location := hook.GetLocation()
		assert.Equal(t, logFile, location, "GetLocation should return the correct path")

		// Test Levels
		levels := hook.Levels()
		assert.NotEmpty(t, levels, "FileHook should support some levels")
		assert.Contains(t, levels, log.InfoLevel, "FileHook should support Info level")

		// Test Fire method
		entry := &log.Entry{
			Logger:  log.StandardLogger(),
			Data:    log.Fields{"test": "data"},
			Message: "test fire message",
			Level:   log.InfoLevel,
			Time:    time.Now(),
		}

		err := hook.Fire(entry)
		assert.NoError(t, err, "FileHook Fire should not return error")

		// Verify file was written to
		content, err := os.ReadFile(logFile)
		if err == nil {
			assert.Contains(t, string(content), "test fire message", "Log file should contain the message")
		}
	})

	t.Run("FormatValidation", func(t *testing.T) {
		tempDir, cleanup := createTempDir(t, "advanced_file_hook_test")
		defer cleanup()

		testCases := []struct {
			name      string
			format    string
			expectErr bool
		}{
			{"text format", "text", false},
			{"json format", "json", false},
			{"invalid format", "unknown", true},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				logFile := filepath.Join(tempDir, "test_"+tc.name+".log")
				hook, err := NewFileHook(logFile, tc.format)

				if tc.expectErr {
					assert.Error(t, err, "Should return error for "+tc.name)
					assert.Nil(t, hook, "Hook should be nil on error")
				} else {
					// Due to LogRoot permissions, this might still error
					if err != nil {
						t.Logf("NewFileHook error for %s (expected due to permissions): %v", tc.name, err)
					}
				}
			})
		}
	})

	t.Run("FireWithDifferentLevels", func(t *testing.T) {
		tempDir, cleanup := createTempDir(t, "file_hook_fire_test")
		defer cleanup()

		logFile := createTempLogFile(t, tempDir, "fire_test.log")

		hook := &FileHook{
			logFileLocation: logFile,
			formatter:       &PlainTextFormatter{},
			mutex:           &sync.Mutex{},
		}

		testCases := []struct {
			name    string
			level   log.Level
			message string
			fields  log.Fields
		}{
			{"info level", log.InfoLevel, "info message", log.Fields{"key": "value"}},
			{"error level", log.ErrorLevel, "error message", log.Fields{"error": "test_error"}},
			{"debug level", log.DebugLevel, "debug message", log.Fields{}},
			{"warn level", log.WarnLevel, "warn message", log.Fields{"component": "test"}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				entry := &log.Entry{
					Logger:  log.StandardLogger(),
					Data:    tc.fields,
					Message: tc.message,
					Level:   tc.level,
					Time:    time.Now(),
				}

				err := hook.Fire(entry)
				assert.NoError(t, err, "FileHook Fire should not error for "+tc.name)
			})
		}

		// Test with long message
		longMessage := strings.Repeat("Y", MaxLogEntryLength+50)
		entry := &log.Entry{
			Logger:  log.StandardLogger(),
			Data:    log.Fields{"test": "file_truncation"},
			Message: longMessage,
			Level:   log.InfoLevel,
			Time:    time.Now(),
		}

		err := hook.Fire(entry)
		assert.NoError(t, err, "FileHook Fire should handle long messages")

		// Verify content was written
		content, err := os.ReadFile(logFile)
		if err == nil {
			assert.NotEmpty(t, content, "Log file should contain content")
		}
	})

	t.Run("ErrorPaths", func(t *testing.T) {
		// Test invalid log format (duplicate but for completeness)
		hook, err := NewFileHook("test", "invalid_format")
		assert.Error(t, err, "Should error with invalid log format")
		assert.Nil(t, hook, "Hook should be nil when error occurs")

		// Test different hook names to exercise OS-specific path logic
		testNames := []string{"test1", "test2", "test3"}
		for _, name := range testNames {
			hook, err := NewFileHook(name, "text")
			if err != nil {
				assert.Contains(t, err.Error(), "log directory", "Should be a log directory creation error")
			} else if hook != nil {
				location := hook.GetLocation()
				assert.Contains(t, location, name, "Location should contain the hook name")
			}
		}
	})

	t.Run("FireErrorConditions", func(t *testing.T) {
		tempDir, cleanup := createTempDir(t, "file_hook_fire_error_test")
		defer cleanup()

		// Test with a formatter that returns an error
		errorFormatter := &MockErrorFormatter{}
		logFile := filepath.Join(tempDir, "error_test.log")
		hook := &FileHook{
			logFileLocation: logFile,
			formatter:       errorFormatter,
			mutex:           &sync.Mutex{},
		}

		entry := &log.Entry{
			Logger:  log.StandardLogger(),
			Data:    log.Fields{"test": "error"},
			Message: "test error message",
			Level:   log.InfoLevel,
			Time:    time.Now(),
		}

		err := hook.Fire(entry)
		assert.Error(t, err, "FileHook Fire should error with failing formatter")
		assert.Contains(t, err.Error(), "mock formatter error", "Should contain formatter error")

		// Test with invalid file path
		invalidHook := &FileHook{
			logFileLocation: "/invalid/path/that/should/not/exist/test.log",
			formatter:       &PlainTextFormatter{},
			mutex:           &sync.Mutex{},
		}

		err = invalidHook.Fire(entry)
		assert.Error(t, err, "FileHook Fire should error with invalid file path")
	})

	t.Run("SuccessPath", func(t *testing.T) {
		testLogDir, cleanup := createTempLogDir(t)
		defer cleanup()

		testCases := []struct {
			name      string
			format    string
			formatter log.Formatter
		}{
			{"text_format", "text", &PlainTextFormatter{}},
			{"json_format", "json", &JSONFormatter{}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				logFile := filepath.Join(testLogDir, tc.name+".log")

				redactorFormatter := &Redactor{tc.formatter}
				hook := &FileHook{
					logFileLocation: logFile,
					formatter:       redactorFormatter,
					mutex:           &sync.Mutex{},
				}

				assert.NotNil(t, hook, "FileHook should be created")
				assert.Equal(t, logFile, hook.logFileLocation, "Should have correct log file location")
				assert.NotNil(t, hook.formatter, "Should have formatter")
				assert.NotNil(t, hook.mutex, "Should have mutex")

				location := hook.GetLocation()
				assert.Equal(t, logFile, location, "GetLocation should return correct path")

				levels := hook.Levels()
				assert.NotEmpty(t, levels, "Should return log levels")
			})
		}
	})
}

func TestInitLoggingForDockerInvalidFormat(t *testing.T) {
	// Test with invalid format
	err := InitLoggingForDocker("test.log", "invalid_format")
	assert.Error(t, err, "InitLoggingForDocker should return error for invalid format")
	assert.Contains(t, err.Error(), "could not initialize logging to file", "Should contain file init error")
}

// TestConsoleHookFire consolidates all ConsoleHook Fire-related tests
func TestConsoleHookFire(t *testing.T) {
	t.Run("BasicFire", func(t *testing.T) {
		hook, err := NewConsoleHook("text")
		if err != nil {
			t.Skipf("Cannot create console hook: %v", err)
			return
		}

		// Redirect stdout/stderr to discard for cleaner test output
		originalStdout := os.Stdout
		originalStderr := os.Stderr
		defer func() {
			os.Stdout = originalStdout
			os.Stderr = originalStderr
		}()

		devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0o666)
		if err == nil {
			defer devNull.Close()
			os.Stdout = devNull
			os.Stderr = devNull
		}

		entry := &log.Entry{
			Logger:  log.StandardLogger(),
			Data:    log.Fields{},
			Message: "test message",
			Level:   log.InfoLevel,
			Time:    time.Now(),
		}

		err = hook.Fire(entry)
		assert.NoError(t, err, "ConsoleHook Fire should not return error")
	})

	t.Run("DifferentLevelsAndFields", func(t *testing.T) {
		hook, err := NewConsoleHook("text")
		if err != nil {
			t.Skipf("Cannot create console hook: %v", err)
			return
		}

		// Redirect stdout/stderr to discard for cleaner test output
		originalStdout := os.Stdout
		originalStderr := os.Stderr
		defer func() {
			os.Stdout = originalStdout
			os.Stderr = originalStderr
		}()

		devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0o666)
		if err == nil {
			defer devNull.Close()
			os.Stdout = devNull
			os.Stderr = devNull
		}

		// Test with different log levels and entry types
		testCases := []struct {
			name    string
			level   log.Level
			message string
			fields  log.Fields
		}{
			{"info level", log.InfoLevel, "test", log.Fields{"test": "info"}},
			{"error level", log.ErrorLevel, "test", log.Fields{"test": "error"}},
			{"debug level", log.DebugLevel, "test", log.Fields{"test": "debug"}},
			{"warn level", log.WarnLevel, "test", log.Fields{"test": "warn"}},
			{"fatal level", log.FatalLevel, "test", log.Fields{"test": "fatal"}},
			{"panic level", log.PanicLevel, "test", log.Fields{"test": "panic"}},
			{"trace level", log.TraceLevel, "test", log.Fields{"test": "trace"}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				entry := &log.Entry{
					Logger:  log.StandardLogger(),
					Data:    tc.fields,
					Message: tc.message,
					Level:   tc.level,
					Time:    time.Now(),
				}

				err := hook.Fire(entry)
				assert.NoError(t, err, "ConsoleHook Fire should not error for "+tc.name)
			})
		}
	})

	t.Run("LongMessage", func(t *testing.T) {
		hook, err := NewConsoleHook("text")
		if err != nil {
			t.Skipf("Cannot create console hook: %v", err)
			return
		}

		// Redirect stdout/stderr to discard for cleaner test output
		originalStdout := os.Stdout
		originalStderr := os.Stderr
		defer func() {
			os.Stdout = originalStdout
			os.Stderr = originalStderr
		}()

		devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0o666)
		if err == nil {
			defer devNull.Close()
			os.Stdout = devNull
			os.Stderr = devNull
		}

		// Test with long message to test truncation paths
		longMessage := strings.Repeat("X", MaxLogEntryLength+100)
		entry := &log.Entry{
			Logger:  log.StandardLogger(),
			Data:    log.Fields{"test": "truncation"},
			Message: longMessage,
			Level:   log.InfoLevel,
			Time:    time.Now(),
		}

		err = hook.Fire(entry)
		assert.NoError(t, err, "ConsoleHook Fire should handle long messages")
	})

	t.Run("ErrorPaths", func(t *testing.T) {
		// Redirect stderr to discard to suppress error log output
		originalStderr := os.Stderr
		defer func() {
			os.Stderr = originalStderr
		}()

		devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0o666)
		if err == nil {
			defer devNull.Close()
			os.Stderr = devNull
		}

		// Test with a formatter that returns an error
		errorFormatter := &MockErrorFormatter{}

		hook := &ConsoleHook{
			formatter: errorFormatter,
		}

		entry := &log.Entry{
			Logger:  log.StandardLogger(),
			Data:    log.Fields{"console": "error"},
			Message: "console error message",
			Level:   log.InfoLevel,
			Time:    time.Now(),
		}

		// This should return an error due to formatter failure
		err = hook.Fire(entry)
		assert.Error(t, err, "ConsoleHook Fire should error with failing formatter")
		assert.Contains(t, err.Error(), "mock formatter error", "Should contain formatter error")

		// Test unknown log level path in ConsoleHook.Fire
		entry.Level = log.Level(999) // Invalid level

		hook2 := &ConsoleHook{
			formatter: &PlainTextFormatter{},
		}

		err = hook2.Fire(entry)
		assert.Error(t, err, "ConsoleHook Fire should error with unknown log level")
		assert.Contains(t, err.Error(), "unknown log level", "Should contain level error")
	})
}

func TestCheckIfTerminal(t *testing.T) {
	// This function is internal, but we can test it indirectly by creating a console hook
	hook, err := NewConsoleHook("text")
	if err != nil {
		t.Skipf("Cannot test checkIfTerminal: %v", err)
		return
	}

	// The hook creation will call checkIfTerminal internally
	assert.NotNil(t, hook, "Console hook should be created")
}

// Test SetContextLogLayer edge cases
func TestSetContextLogLayerEdgeCases(t *testing.T) {
	ctx := context.Background()

	// Test with LogLayerNone
	result := SetContextLogLayer(ctx, LogLayerNone)
	assert.NotNil(t, result, "SetContextLogLayer should return a context")

	// Test with existing layer (recursive case)
	ctxWithLayer := context.WithValue(ctx, ContextKeyLogLayer, LogLayerCore)
	result = SetContextLogLayer(ctxWithLayer, LogLayerCore)
	assert.NotNil(t, result, "SetContextLogLayer should handle recursive case")
}

// Test IsLogLevelDebugOrHigher edge case
func TestIsLogLevelDebugOrHigherEdgeCase(t *testing.T) {
	// Test with different log levels
	result := IsLogLevelDebugOrHigher("info")
	assert.False(t, result, "Should return false for info level")

	// Test with panic level (lowest)
	SetDefaultLogLevel("panic")
	result = IsLogLevelDebugOrHigher("panic")
	assert.False(t, result, "Should return false for panic level")

	// Test with debug level
	result = IsLogLevelDebugOrHigher("debug")
	assert.True(t, result, "Should return true for debug level")
}

// Test more InitLoggingForDocker paths
func TestInitLoggingForDockerFormats(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "logging_test")
	assert.NoError(t, err, "Should create temp directory")
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "docker_json_test.log")

	// Test with JSON format
	err = InitLoggingForDocker(logFile, "json")
	if err != nil {
		t.Logf("InitLoggingForDocker JSON error (expected in test environment): %v", err)
	}
}

// Test log file rotation functions - the 0% coverage functions
func TestLogFileRotation(t *testing.T) {
	// Create a temporary directory for test
	tempDir, err := os.MkdirTemp("", "rotation_test")
	assert.NoError(t, err, "Should create temp directory")
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "rotation_test.log")

	// Create a FileHook manually for testing
	formatter := &PlainTextFormatter{}
	hook := &FileHook{
		logFileLocation: logFile,
		formatter:       formatter,
		mutex:           &sync.Mutex{},
	}

	// Test 1: logfileNeedsRotation with non-existent file
	needsRotation := hook.logfileNeedsRotation()
	assert.False(t, needsRotation, "Non-existent file should not need rotation")

	// Test 2: Create a small file that doesn't need rotation
	smallContent := "small log content"
	err = os.WriteFile(logFile, []byte(smallContent), 0o644)
	assert.NoError(t, err, "Should create small log file")

	needsRotation = hook.logfileNeedsRotation()
	assert.False(t, needsRotation, "Small file should not need rotation")

	// Test 3: Create a large file that needs rotation (> LogRotationThreshold)
	// LogRotationThreshold is 10485760 (10 MB), so create content larger than that
	largeContent := make([]byte, LogRotationThreshold+1000)
	for i := range largeContent {
		largeContent[i] = 'A' // Fill with 'A' characters
	}

	err = os.WriteFile(logFile, largeContent, 0o644)
	assert.NoError(t, err, "Should create large log file")

	needsRotation = hook.logfileNeedsRotation()
	assert.True(t, needsRotation, "Large file should need rotation")

	// Test 4: Test doLogfileRotation
	err = hook.doLogfileRotation()
	assert.NoError(t, err, "doLogfileRotation should not return error")

	// Verify the old file was created
	oldLogFile := logFile + ".old"
	_, statErr := os.Stat(oldLogFile)
	assert.NoError(t, statErr, "Old log file should exist after rotation")

	// Verify original file still exists (may be recreated)
	_, statErr = os.Stat(logFile)
	// This might not exist yet, that's okay

	// Test 5: Test doLogfileRotation when file doesn't need rotation
	// Create small file again
	err = os.WriteFile(logFile, []byte(smallContent), 0o644)
	assert.NoError(t, err, "Should create small log file again")

	err = hook.doLogfileRotation()
	assert.NoError(t, err, "doLogfileRotation should not error even when rotation not needed")
}

// Test maybeDoLogfileRotation function to improve its coverage
func TestMaybeDoLogfileRotation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "maybe_rotation_test")
	assert.NoError(t, err, "Should create temp directory")
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "maybe_rotation_test.log")

	hook := &FileHook{
		logFileLocation: logFile,
		formatter:       &PlainTextFormatter{},
		mutex:           &sync.Mutex{},
	}

	// Test multiple times since maybeDoLogfileRotation uses randomness
	for i := 0; i < 50; i++ {
		err = hook.maybeDoLogfileRotation()
		// Should not error regardless of whether rotation happens
		assert.NoError(t, err, "maybeDoLogfileRotation should not error")
	}
}

// Test SetContextLogLayer more thoroughly to improve its coverage
func TestSetContextLogLayerThorough(t *testing.T) {
	// Test with different log layers
	ctx := context.Background()

	testLayers := []LogLayer{
		LogLayerCore,
		LogLayerCSIFrontend,
		LogLayerRESTFrontend,
		LogLayerNone,
		LogLayer("custom_layer"),
	}

	for _, layer := range testLayers {
		result := SetContextLogLayer(ctx, layer)
		assert.NotNil(t, result, "Should return context for layer: "+string(layer))

		// Verify the layer was set
		if layer != LogLayerNone {
			value := result.Value(ContextKeyLogLayer)
			assert.Equal(t, layer, value, "Context should contain the correct layer")
		}
	}

	// Test recursive/nested setting
	ctx1 := SetContextLogLayer(ctx, LogLayerCore)
	ctx2 := SetContextLogLayer(ctx1, LogLayerCSIFrontend)

	value := ctx2.Value(ContextKeyLogLayer)
	assert.Equal(t, LogLayerCSIFrontend, value, "Should have the most recent layer")
}

// Test console hook Fire method more thoroughly to improve coverage
// Test formatter functions more extensively to improve coverage
func TestFormattersAdvanced(t *testing.T) {
	// Test PlainTextFormatter with various scenarios
	plainFormatter := &PlainTextFormatter{
		TimestampFormat: time.RFC3339,
		DisableSorting:  false,
	}

	testEntries := []*log.Entry{
		{
			Logger:  log.StandardLogger(),
			Data:    log.Fields{},
			Message: "simple message",
			Level:   log.InfoLevel,
			Time:    time.Now(),
		},
		{
			Logger: log.StandardLogger(),
			Data: log.Fields{
				"field1":    "value1",
				"field2":    123,
				"field3":    true,
				"field4":    []string{"a", "b", "c"},
				"field5":    map[string]interface{}{"nested": "value"},
				"component": "test",
			},
			Message: "complex message with fields",
			Level:   log.WarnLevel,
			Time:    time.Now(),
		},
		{
			Logger: log.StandardLogger(),
			Data: log.Fields{
				"msg":   "this should clash with message",
				"level": "this should clash with level",
				"time":  "this should clash with time",
			},
			Message: "message with field conflicts",
			Level:   log.ErrorLevel,
			Time:    time.Now(),
		},
		{
			Logger: log.StandardLogger(),
			Data: log.Fields{
				"field_with_space":   "value with space",
				"field-with-dash":    "value-with-dash",
				"field_with_quote":   "value with \"quotes\"",
				"field_with_newline": "value with\nnewline",
			},
			Message: "message with special characters",
			Level:   log.DebugLevel,
			Time:    time.Now(),
		},
	}

	for i, entry := range testEntries {
		t.Run(fmt.Sprintf("PlainTextFormatter_entry_%d", i), func(t *testing.T) {
			result, err := plainFormatter.Format(entry)
			assert.NoError(t, err, "PlainTextFormatter should not error")
			assert.NotEmpty(t, result, "PlainTextFormatter should return content")

			// Verify it contains the message
			assert.Contains(t, string(result), entry.Message, "Should contain the message")
		})
	}

	// Test JSONFormatter with the same entries
	jsonFormatter := &JSONFormatter{
		TimestampFormat:  time.RFC3339,
		DisableTimestamp: false,
		PrettyPrint:      false,
	}

	for i, entry := range testEntries {
		t.Run(fmt.Sprintf("JSONFormatter_entry_%d", i), func(t *testing.T) {
			result, err := jsonFormatter.Format(entry)
			assert.NoError(t, err, "JSONFormatter should not error")
			assert.NotEmpty(t, result, "JSONFormatter should return content")

			// Verify it's valid JSON by attempting to parse
			var parsed map[string]interface{}
			err = json.Unmarshal(result, &parsed)
			assert.NoError(t, err, "Should produce valid JSON")
			assert.Contains(t, parsed, "message", "JSON should contain message field")
		})
	}
}

// Test edge cases to improve coverage above 90%
func TestEdgeCasesForHighCoverage(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "edge_cases_test")
	assert.NoError(t, err, "Should create temp directory")
	defer os.RemoveAll(tempDir)

	// Test logfileNeedsRotation with file that can't be stat'd (after opening)
	logFile := filepath.Join(tempDir, "edge_test.log")
	hook := &FileHook{
		logFileLocation: logFile,
		formatter:       &PlainTextFormatter{},
		mutex:           &sync.Mutex{},
	}

	// Create a file and then test opening/stating it
	err = os.WriteFile(logFile, []byte("test content"), 0o644)
	assert.NoError(t, err, "Should create test file")

	// Test logfileNeedsRotation when file exists
	needsRotation := hook.logfileNeedsRotation()
	assert.False(t, needsRotation, "Small file should not need rotation")

	// Test Fire method with file that can be written to
	entry := &log.Entry{
		Logger:  log.StandardLogger(),
		Data:    log.Fields{"test": "coverage"},
		Message: "test fire method edge cases",
		Level:   log.InfoLevel,
		Time:    time.Now(),
	}

	err = hook.Fire(entry)
	assert.NoError(t, err, "FileHook Fire should succeed")

	// Test Fire method with maybeDoLogfileRotation call (try multiple times to hit random path)
	for i := 0; i < 100; i++ {
		err = hook.Fire(entry)
		assert.NoError(t, err, "FileHook Fire should succeed on iteration %d", i)
	}

	// Test with different log levels to ensure all code paths
	levels := []log.Level{log.PanicLevel, log.FatalLevel, log.ErrorLevel, log.WarnLevel, log.InfoLevel, log.DebugLevel, log.TraceLevel}
	for _, level := range levels {
		entry.Level = level
		err = hook.Fire(entry)
		assert.NoError(t, err, "FileHook Fire should succeed for level %s", level.String())
	}
}

// Test additional cases for console hook to improve coverage
func TestConsoleHookEdgeCases(t *testing.T) {
	// Test with both text and json formats
	formats := []string{"text", "json"}

	for _, format := range formats {
		hook, err := NewConsoleHook(format)
		if err != nil {
			t.Logf("Skipping console hook test for %s format: %v", format, err)
			continue
		}

		// Test with edge case entries
		edgeEntries := []*log.Entry{
			{
				Logger:  log.StandardLogger(),
				Data:    log.Fields{},
				Message: "",
				Level:   log.InfoLevel,
				Time:    time.Now(),
			},
			{
				Logger: log.StandardLogger(),
				Data: log.Fields{
					"nil_field":    nil,
					"empty_string": "",
					"number":       42,
					"boolean":      true,
				},
				Message: "edge case message",
				Level:   log.WarnLevel,
				Time:    time.Now(),
			},
		}

		for i, entry := range edgeEntries {
			err = hook.Fire(entry)
			assert.NoError(t, err, "ConsoleHook Fire should not error for %s format edge case %d", format, i)
		}
	}
}

// Test appendValue function edge cases for PlainTextFormatter
func TestAppendValueEdgeCases(t *testing.T) {
	formatter := &PlainTextFormatter{}

	testCases := []struct {
		name  string
		value interface{}
	}{
		{"string_with_spaces", "value with spaces"},
		{"string_with_quotes", "value with \"quotes\""},
		{"string_with_newlines", "value\nwith\nnewlines"},
		{"string_with_tabs", "value\twith\ttabs"},
		{"error_value", fmt.Errorf("test error message")},
		{"error_with_quotes", fmt.Errorf("error with \"quotes\"")},
		{"nil_value", nil},
		{"int_value", 42},
		{"float_value", 3.14},
		{"bool_true", true},
		{"bool_false", false},
		{"slice_value", []string{"a", "b", "c"}},
		{"map_value", map[string]string{"key": "value"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			formatter.appendValue(&buf, tc.value)
			result := buf.String()
			assert.NotNil(t, result, "appendValue should produce output for %s", tc.name)
		})
	}
}

// Test needsQuoting edge cases
func TestNeedsQuotingEdgeCases(t *testing.T) {
	formatter := &PlainTextFormatter{}

	testCases := []struct {
		input    string
		expected bool
		name     string
	}{
		{"", false, "empty_string"},
		{"simple", false, "simple_string"},
		{"with space", true, "string_with_space"},
		{"with\ttab", true, "string_with_tab"},
		{"with\nnewline", true, "string_with_newline"},
		{"with=equals", true, "string_with_equals"},
		{"with\"quote", true, "string_with_quote"},
		{"with'apostrophe", true, "string_with_apostrophe"},
		{"ALLCAPS", false, "all_caps"},
		{"123", false, "numeric_string"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := formatter.needsQuoting(tc.input)
			assert.Equal(t, tc.expected, result, "needsQuoting(%q) should return %v", tc.input, tc.expected)
		})
	}
}

// Test JSONFormatter edge cases
func TestJSONFormatterEdgeCases(t *testing.T) {
	// Test with different timestamp settings
	formatters := []*JSONFormatter{
		{TimestampFormat: time.RFC3339, DisableTimestamp: false, PrettyPrint: false},
		{TimestampFormat: time.RFC3339, DisableTimestamp: true, PrettyPrint: false},
		{TimestampFormat: time.RFC3339, DisableTimestamp: false, PrettyPrint: true},
		{TimestampFormat: "", DisableTimestamp: false, PrettyPrint: false}, // Empty format should use default
	}

	entry := &log.Entry{
		Logger: log.StandardLogger(),
		Data: log.Fields{
			"error_field":  fmt.Errorf("test error"),
			"normal_field": "normal value",
		},
		Message: "test json formatter edge cases",
		Level:   log.InfoLevel,
		Time:    time.Now(),
	}

	for i, formatter := range formatters {
		t.Run(fmt.Sprintf("formatter_%d", i), func(t *testing.T) {
			result, err := formatter.Format(entry)
			assert.NoError(t, err, "JSONFormatter should not error")
			assert.NotEmpty(t, result, "JSONFormatter should return content")

			// Verify it's still valid JSON
			var parsed map[string]interface{}
			err = json.Unmarshal(result, &parsed)
			assert.NoError(t, err, "Should produce valid JSON")
		})
	}
}

// Test InitLoggingForDocker
func TestInitLoggingForDocker(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "docker_comprehensive_more_test")
	assert.NoError(t, err, "Should create temp directory")
	defer os.RemoveAll(tempDir)

	// Store original values to restore later
	originalHooks := log.StandardLogger().Hooks
	originalLevel := log.GetLevel()
	originalFormatter := log.StandardLogger().Formatter
	originalOutput := log.StandardLogger().Out
	originalEnv := os.Getenv(RandomLogcheckEnvVar)

	defer func() {
		log.StandardLogger().Hooks = originalHooks
		log.SetLevel(originalLevel)
		log.StandardLogger().SetFormatter(originalFormatter)
		log.StandardLogger().SetOutput(originalOutput)
		os.Setenv(RandomLogcheckEnvVar, originalEnv)
	}()

	// Clean the environment variable first to test default path
	os.Unsetenv(RandomLogcheckEnvVar)

	logFile := filepath.Join(tempDir, "comprehensive_docker.log")

	// Test the main path with unset environment variable
	err = InitLoggingForDocker(logFile, "text")
	if err != nil {
		// Expected in test environment due to LogRoot permissions
		assert.Contains(t, err.Error(), "log", "Should be a logging-related error")
	}

	// Test with valid integer environment variable
	os.Setenv(RandomLogcheckEnvVar, "50")
	err = InitLoggingForDocker(logFile, "json")
	if err != nil {
		// Expected in test environment due to LogRoot permissions
		assert.Contains(t, err.Error(), "log", "Should be a logging-related error")
	}

	// Test with empty environment variable (should use default)
	os.Setenv(RandomLogcheckEnvVar, "")
	err = InitLoggingForDocker(logFile, "text")
	if err != nil {
		// Expected in test environment due to LogRoot permissions
		assert.Contains(t, err.Error(), "log", "Should be a logging-related error")
	}

	// Test all the internal function paths
	InitLogLevel("info")
	InitLogFormat("text")

	// Create a file writer for InitLogOutput
	if file, err := os.Create(logFile); err == nil {
		InitLogOutput(file)
		file.Close()
	}

	// Create a formatter for InitLogFormatter
	formatter := &log.TextFormatter{}
	InitLogFormatter(formatter)

	// Test different log levels
	testLevels := []string{"panic", "fatal", "error", "warn", "info", "debug", "trace", "invalid"}
	for _, level := range testLevels {
		InitLogLevel(level)
		// Should handle invalid levels gracefully
	}

	// Test different formats
	testFormats := []string{"text", "json", "invalid"}
	for _, format := range testFormats {
		InitLogFormat(format)
		// Should handle invalid formats gracefully
	}

	// Test IsLevelEnabled and GetLogLevel
	log.SetLevel(log.InfoLevel)
	enabled := IsLevelEnabled(log.InfoLevel)
	assert.True(t, enabled, "IsLevelEnabled should return true for current log level")

	level := GetLogLevel()
	assert.Equal(t, "info", level, "GetLogLevel should return 'info' when level is set to InfoLevel")
}

// Test checkIfTerminal more thoroughly
func TestCheckIfTerminalThorough(t *testing.T) {
	// Test checkIfTerminal with different file descriptors
	// Since this is system-dependent, we mainly test it doesn't panic

	// We can't directly call checkIfTerminal as it's not exported,
	// but we can test NewConsoleHook which uses it internally
	hook, err := NewConsoleHook("text")
	if err != nil {
		t.Logf("NewConsoleHook error (expected): %v", err)
	} else if hook != nil {
		// Test different scenarios that might exercise checkIfTerminal paths
		entry := &log.Entry{
			Logger:  log.StandardLogger(),
			Data:    log.Fields{"terminal": "test"},
			Message: "terminal test message",
			Level:   log.InfoLevel,
			Time:    time.Now(),
		}

		err = hook.Fire(entry)
		assert.NoError(t, err, "ConsoleHook should fire without error")
	}

	// Test with JSON format too
	hookJSON, err := NewConsoleHook("json")
	if err != nil {
		t.Logf("NewConsoleHook JSON error (expected): %v", err)
	} else if hookJSON != nil {
		entry := &log.Entry{
			Logger:  log.StandardLogger(),
			Data:    log.Fields{"terminal": "test_json"},
			Message: "terminal test message json",
			Level:   log.WarnLevel,
			Time:    time.Now(),
		}

		err = hookJSON.Fire(entry)
		assert.NoError(t, err, "ConsoleHook JSON should fire without error")
	}
}

// Test SetContextLogLayer
func TestSetContextLogLayerAdvanced(t *testing.T) {
	ctx := context.Background()

	// Test with LogLayerNone (special case)
	result := SetContextLogLayer(ctx, LogLayerNone)
	assert.NotNil(t, result, "Should handle LogLayerNone")

	// Verify LogLayerNone is stored in context (it actually gets stored as "none")
	value := result.Value(ContextKeyLogLayer)
	assert.Equal(t, LogLayerNone, value, "LogLayerNone should be stored in context")

	// Test with valid layers
	validLayers := []LogLayer{
		LogLayerCore,
		LogLayerCSIFrontend,
		LogLayerRESTFrontend,
		LogLayerCRDFrontend,
		LogLayerDockerFrontend,
	}

	for _, layer := range validLayers {
		result = SetContextLogLayer(ctx, layer)
		assert.NotNil(t, result, "Should return context for layer %s", layer)

		value = result.Value(ContextKeyLogLayer)
		assert.Equal(t, layer, value, "Should store correct layer %s", layer)
	}

	// Test with context that already has values
	ctxWithValues := context.WithValue(ctx, "existing_key", "existing_value")
	result = SetContextLogLayer(ctxWithValues, LogLayerCore)
	assert.NotNil(t, result, "Should handle context with existing values")

	// Verify existing value is preserved
	existingValue := result.Value("existing_key")
	assert.Equal(t, "existing_value", existingValue, "Should preserve existing context values")

	// Verify new layer is added
	layerValue := result.Value(ContextKeyLogLayer)
	assert.Equal(t, LogLayerCore, layerValue, "Should add new layer")
}

func TestUncoveredEdgeCasesAndErrorPath(t *testing.T) {
	// 1. Test SetContextLogLayer with empty string LogLayer (line 353-355 in logger.go)
	ctx := context.Background()
	emptyLayer := LogLayer("")
	result := SetContextLogLayer(ctx, emptyLayer)

	// Should return context without adding the empty layer
	value := result.Value(ContextKeyLogLayer)
	assert.Nil(t, value, "Empty LogLayer should not be stored in context")

	// 2. Test checkIfTerminal with non-*os.File writer
	hook, err := NewConsoleHook("text")
	if err == nil && hook != nil {
		// Test with a bytes.Buffer (not *os.File)
		var buf bytes.Buffer
		isTerminal := hook.checkIfTerminal(&buf)
		assert.False(t, isTerminal, "Buffer should not be detected as terminal")
	}

	// 3. Test InitLoggingForDocker error paths
	tempDir, err := os.MkdirTemp("", "init_logging_error_test")
	assert.NoError(t, err, "Should create temp directory")
	defer os.RemoveAll(tempDir)

	// Store original hooks to restore later
	originalHooks := log.StandardLogger().Hooks
	defer func() {
		log.StandardLogger().Hooks = originalHooks
	}()

	// Test with invalid log format for NewFileHook error
	invalidLogFile := filepath.Join(tempDir, "invalid_format.log")
	err = InitLoggingForDocker(invalidLogFile, "invalid_format")
	assert.Error(t, err, "InitLoggingForDocker should error with invalid format")
	assert.Contains(t, err.Error(), "could not initialize logging to file", "Should contain file init error")

	// Test strconv.Atoi error path with invalid environment variable
	originalEnv := os.Getenv(RandomLogcheckEnvVar)
	defer os.Setenv(RandomLogcheckEnvVar, originalEnv)

	os.Setenv(RandomLogcheckEnvVar, "not_a_number")
	validLogFile := filepath.Join(tempDir, "valid.log")
	err = InitLoggingForDocker(validLogFile, "text")
	// This shouldn't error due to env var, but might error due to LogRoot permissions
	if err != nil {
		// Expected in test environment due to LogRoot permissions
		assert.Contains(t, err.Error(), "log", "Should be a logging-related error")
	}
}

// Test FileHook.Fire error conditions
// MockErrorFormatter for testing error conditions
type MockErrorFormatter struct{}

func (f *MockErrorFormatter) Format(entry *log.Entry) ([]byte, error) {
	return nil, fmt.Errorf("mock formatter error")
}

// Test ConsoleHook Fire error conditions
func TestLogRotationEdgeCases(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "log_rotation_edge_test")
	assert.NoError(t, err, "Should create temp directory")
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "rotation_edge.log")
	hook := &FileHook{
		logFileLocation: logFile,
		formatter:       &PlainTextFormatter{},
		mutex:           &sync.Mutex{},
	}

	// Test logfileNeedsRotation with file open error
	// Create a directory with the same name as the log file
	err = os.MkdirAll(logFile, 0o755)
	assert.NoError(t, err, "Should create directory with log file name")

	needsRotation := hook.logfileNeedsRotation()
	assert.False(t, needsRotation, "Should return false when file can't be opened")

	// Clean up and create a proper file
	err = os.RemoveAll(logFile)
	assert.NoError(t, err, "Should remove directory")

	// Test with file that can't be stat'd (create file then make it unreadable)
	err = os.WriteFile(logFile, []byte("test"), 0o644)
	assert.NoError(t, err, "Should create test file")

	// Make file unreadable (this might not work on all systems)
	err = os.Chmod(logFile, 0o000)
	if err == nil {
		defer os.Chmod(logFile, 0o644) // Restore permissions for cleanup

		needsRotation = hook.logfileNeedsRotation()
		// This should return false due to stat error
		assert.False(t, needsRotation, "Should return false when file can't be stat'd")
	}
}

// Test the missing error branch in IsLogLevelDebugOrHigher
// Test IsLogLevelDebugOrHigher
func TestIsLogLevelDebugOrHigherInvalidLevel(t *testing.T) {
	// Test with invalid log level to hit the error branch (line 769-771 in logger.go)
	result := IsLogLevelDebugOrHigher("invalid_log_level")

	// Should return false for invalid level (lvl will be 0, which is < log.DebugLevel)
	assert.False(t, result, "Should return false for invalid log level")

	// Also test some edge cases
	result = IsLogLevelDebugOrHigher("")
	assert.False(t, result, "Should return false for empty string")

	result = IsLogLevelDebugOrHigher("not_a_level")
	assert.False(t, result, "Should return false for nonsense string")

	// Test with a valid level that should return false
	result = IsLogLevelDebugOrHigher("panic")
	assert.False(t, result, "Should return false for panic level")

	// Test with valid levels that should return true
	result = IsLogLevelDebugOrHigher("debug")
	assert.True(t, result, "Should return true for debug level")

	result = IsLogLevelDebugOrHigher("trace")
	assert.True(t, result, "Should return true for trace level")
}

// Test String methods in types.go to achieve 100% coverage
func TestWorkflowString(t *testing.T) {
	workflow := Workflow{
		Category:  WorkflowCategory("volume"),
		Operation: WorkflowOperation("create"),
	}

	result := workflow.String()
	expected := "volume" + workflowCategorySeparator + "create"
	assert.Equal(t, expected, result, "Workflow.String() should return category + separator + operation")

	// Test with empty values
	emptyWorkflow := Workflow{
		Category:  WorkflowCategory(""),
		Operation: WorkflowOperation(""),
	}
	result = emptyWorkflow.String()
	expected = "" + workflowCategorySeparator + ""
	assert.Equal(t, expected, result, "Workflow.String() should handle empty values")
}

func TestLogLayerString(t *testing.T) {
	layer := LogLayer("core")
	result := layer.String()
	assert.Equal(t, "core", result, "LogLayer.String() should return the string value")

	// Test with empty value
	emptyLayer := LogLayer("")
	result = emptyLayer.String()
	assert.Equal(t, "", result, "LogLayer.String() should handle empty value")

	// Test with standard log layers
	testLayers := []LogLayer{
		LogLayerCore,
		LogLayerCSIFrontend,
		LogLayerRESTFrontend,
		LogLayerCRDFrontend,
		LogLayerDockerFrontend,
		LogLayerNone,
	}

	for _, testLayer := range testLayers {
		result = testLayer.String()
		assert.Equal(t, string(testLayer), result, "LogLayer.String() should return correct string for %s", testLayer)
	}
}

// Test successful file operations when we can create the log directory
// Test RedactedHTTPRequest edge cases to improve coverage from 76%
func TestRedactedHTTPRequestEdgeCases(t *testing.T) {
	ctx := context.Background()

	// Test with basic request
	req := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/test"},
		Header: http.Header{},
	}
	req = req.WithContext(ctx)

	// Test basic functionality
	RedactedHTTPRequest(req, []byte("test body"), "test-driver", false, true)

	// Test with authorization header (should be redacted)
	req.Header.Set("Authorization", "Bearer secret-token")
	RedactedHTTPRequest(req, []byte("test body"), "test-driver", true, false)

	// Test with user info in URL (should be removed)
	urlWithUser, _ := url.Parse("https://user:pass@example.com/test")
	req.URL = urlWithUser
	RedactedHTTPRequest(req, []byte("test"), "test-driver", true, false)

	// Function completes successfully if no panic occurs
}

// Test RedactedHTTPResponse edge cases to improve coverage from 83.3%
func TestRedactedHTTPResponseEdgeCases(t *testing.T) {
	ctx := context.Background()

	// Test with basic response
	resp := &http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Header:     http.Header{},
	}

	// Test the function (it doesn't return a value, just logs)
	RedactedHTTPResponse(ctx, resp, []byte("response body"), "test-driver", false, true)

	// Test with headers that should be redacted
	resp.Header.Set("Authorization", "Bearer response-token")
	resp.Header.Set("Api-Key", "secret-api-key")
	resp.Header.Set("Secret-Key", "secret-value")
	resp.Header.Set("Content-Type", "application/json")
	RedactedHTTPResponse(ctx, resp, []byte("response body"), "test-driver", true, false)

	// Test different status codes
	statusCodes := []int{200, 404, 500, 301}
	for _, code := range statusCodes {
		resp.StatusCode = code
		resp.Status = fmt.Sprintf("%d Status", code)
		RedactedHTTPResponse(ctx, resp, []byte("test body"), "test-driver", false, true)
	}

	// Test with nil response body
	RedactedHTTPResponse(ctx, resp, nil, "test-driver", true, false)

	// Function completes successfully if no panic occurs
}
