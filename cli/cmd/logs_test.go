// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testLogContent = "test log content"
	testNodeName   = "test-node"
	testPodName    = "test-pod"
)

var logTestMutex sync.RWMutex

func mockExecKubernetesCLI() func() {
	original := execKubernetesCLI
	execKubernetesCLI = func(args ...string) ([]byte, error) {
		return []byte("mock output"), nil
	}
	return func() { execKubernetesCLI = original }
}

func withLogTestMode(t *testing.T, fn func()) {
	t.Helper()

	logTestMutex.Lock()
	defer logTestMutex.Unlock()

	// Save original state
	originalLogType := logType
	originalArchive := archive
	originalNode := node
	originalSidecars := sidecars
	originalOperatingMode := OperatingMode
	originalZipWriter := zipWriter
	originalLogErrors := logErrors
	originalGetLogs := getLogs
	originalTridentPodName := TridentPodName
	originalTridentPodNamespace := TridentPodNamespace
	originalTridentOperatorPodName := tridentOperatorPodName
	originalTridentOperatorPodNamespace := tridentOperatorPodNamespace

	defer func() {
		logType = originalLogType
		archive = originalArchive
		node = originalNode
		sidecars = originalSidecars
		OperatingMode = originalOperatingMode
		zipWriter = originalZipWriter
		logErrors = originalLogErrors
		getLogs = originalGetLogs
		TridentPodName = originalTridentPodName
		TridentPodNamespace = originalTridentPodNamespace
		tridentOperatorPodName = originalTridentOperatorPodName
		tridentOperatorPodNamespace = originalTridentOperatorPodNamespace
	}()

	fn()
}

func TestCheckValidLog(t *testing.T) {
	tests := []struct {
		logType     string
		expectError bool
	}{
		{logTypeTrident, false},
		{logTypeTridentACP, false},
		{logTypeAuto, false},
		{logTypeAll, false},
		{logTypeTridentOperator, false},
		{"invalid", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.logType+"_log", func(t *testing.T) {
			withLogTestMode(t, func() {
				logType = tt.logType
				err := checkValidLog()

				if tt.expectError {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), "is not a valid Trident log")
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestWriteLogs(t *testing.T) {
	tests := []struct {
		archive bool
	}{
		{false},
		{true},
	}

	for _, tt := range tests {
		mode := "console"
		if tt.archive {
			mode = "archive"
		}
		t.Run(mode, func(t *testing.T) {
			withLogTestMode(t, func() {
				archive = tt.archive
				logEntry := []byte(testLogContent)

				if tt.archive {
					var buf bytes.Buffer
					zipFileName = "test-archive.zip"
					zipWriter = zip.NewWriter(&buf)
					defer zipWriter.Close()
				}

				oldStdout := os.Stdout
				r, w, _ := os.Pipe()
				os.Stdout = w

				err := writeLogs("test-log", logEntry)
				w.Close()
				os.Stdout = oldStdout

				var outputBuf bytes.Buffer
				io.Copy(&outputBuf, r)
				output := outputBuf.String()

				assert.NoError(t, err)
				if tt.archive {
					assert.Contains(t, output, "Wrote")
					assert.Contains(t, output, "archive file")
				} else {
					assert.Contains(t, output, "test-log log:")
					assert.Contains(t, output, testLogContent)
				}
			})
		})
	}
}

func TestArchiveLogs(t *testing.T) {
	tests := []struct {
		name        string
		logType     string
		expectError bool
		setupMocks  func() func()
	}{
		{"auto_converts_to_all", logTypeAuto, false, nil},
		{"trident_type", logTypeTrident, false, nil},
		{"all_type", logTypeAll, false, nil},
		{"error_writing_errors", logTypeAuto, true, func() func() {
			getLogs = func() error {
				logErrors = []byte("test error")
				return errors.New("test error")
			}
			return func() { getLogs = nil }
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withLogTestMode(t, func() {
				var mockCleanup func()
				if tt.setupMocks != nil {
					mockCleanup = tt.setupMocks()
					defer mockCleanup()
				} else {
					getLogs = func() error { return nil }
				}

				logType = tt.logType
				archive = true

				tmpDir, err := os.MkdirTemp("", "logs-test-")
				require.NoError(t, err)
				defer os.RemoveAll(tmpDir)

				oldDir, _ := os.Getwd()
				os.Chdir(tmpDir)
				defer os.Chdir(oldDir)

				err = archiveLogs()

				if tt.expectError && tt.name == "error_writing_errors" {
					assert.NoError(t, err)
					files, _ := filepath.Glob("support-*.zip")
					assert.NotEmpty(t, files, "Archive file should be created even with errors")
				} else if tt.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					files, _ := filepath.Glob("support-*.zip")
					assert.NotEmpty(t, files, "Archive file should be created")

					if tt.logType == logTypeAuto {
						assert.Equal(t, logTypeAll, logType)
						assert.True(t, previous)
						assert.True(t, sidecars)
					}
				}
			})
		})
	}
}

func TestConsoleLogs(t *testing.T) {
	tests := []struct {
		name          string
		getLogsError  error
		logErrors     []byte
		expectError   bool
		expectMessage string
	}{
		{"success", nil, nil, false, ""},
		{"error_only", errors.New("test error"), nil, true, "test error"},
		{"error_with_logs", errors.New("test error"), []byte("log error"), true, "test error. log error"},
		{"logs_only", nil, []byte("log error"), false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withLogTestMode(t, func() {
				getLogs = func() error {
					logErrors = tt.logErrors
					return tt.getLogsError
				}

				err := consoleLogs()

				if tt.expectError {
					assert.Error(t, err)
					if tt.expectMessage != "" {
						assert.Contains(t, err.Error(), tt.expectMessage)
					}
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestLogRetrieval(t *testing.T) {
	tests := []struct {
		testType    string
		logName     string
		sidecars    bool
		nodeName    string
		expectError bool
		setupFunc   func()
	}{
		{"trident_current", logNameTrident, false, "", false, nil},
		{"trident_previous", logNameTridentPrevious, false, "", false, nil},
		{"acp_current", logNameTridentACP, false, "", false, nil},
		{"acp_previous", logNameTridentACPPrevious, false, "", false, nil},
		{"trident_with_sidecars", logNameTrident, true, "", false, func() {
			originalListTridentSidecars := listTridentSidecars
			listTridentSidecars = func(podName, namespace string) ([]string, error) {
				return []string{"sidecar1", "sidecar2"}, nil
			}
			t.Cleanup(func() { listTridentSidecars = originalListTridentSidecars })
		}},
		{"trident_sidecar_list_error", logNameTrident, true, "", true, func() {
			originalListTridentSidecars := listTridentSidecars
			listTridentSidecars = func(podName, namespace string) ([]string, error) {
				return nil, errors.New("sidecar list error")
			}
			t.Cleanup(func() { listTridentSidecars = originalListTridentSidecars })
		}},

		// Node logs with errors
		{"node_get_error", logNameNode, false, testNodeName, true, func() {
			originalGetTridentNode := getTridentNode
			getTridentNode = func(nodeName, namespace string) (string, error) {
				return "", errors.New("node get error")
			}
			t.Cleanup(func() { getTridentNode = originalGetTridentNode })
		}},
		{"trident_debug_mode", logNameTrident, false, "", false, func() {
			originalDebug := Debug
			Debug = true
			t.Cleanup(func() { Debug = originalDebug })
		}},

		// Node logs
		{"node_current", logNameNode, false, testNodeName, false, func() {
			originalGetTridentNode := getTridentNode
			getTridentNode = func(nodeName, namespace string) (string, error) {
				return testPodName, nil
			}
			t.Cleanup(func() { getTridentNode = originalGetTridentNode })
		}},
		{"node_previous", logNameNodePrevious, false, testNodeName, false, func() {
			originalGetTridentNode := getTridentNode
			getTridentNode = func(nodeName, namespace string) (string, error) {
				return testPodName, nil
			}
			t.Cleanup(func() { getTridentNode = originalGetTridentNode })
		}},
		{"node_invalid", "invalid", false, testNodeName, true, func() {
			originalGetTridentNode := getTridentNode
			getTridentNode = func(nodeName, namespace string) (string, error) {
				return testPodName, nil
			}
			t.Cleanup(func() { getTridentNode = originalGetTridentNode })
		}},

		// All node logs
		{"all_nodes_current", logNameNode, false, "", false, func() {
			originalListTridentNodes := listTridentNodes
			listTridentNodes = func(namespace string) (map[string]string, error) {
				return map[string]string{"node1": "pod1", "node2": "pod2"}, nil
			}
			t.Cleanup(func() { listTridentNodes = originalListTridentNodes })
		}},
		{"all_nodes_previous", logNameNodePrevious, false, "", false, func() {
			originalListTridentNodes := listTridentNodes
			listTridentNodes = func(namespace string) (map[string]string, error) {
				return map[string]string{"node1": "pod1", "node2": "pod2"}, nil
			}
			t.Cleanup(func() { listTridentNodes = originalListTridentNodes })
		}},
		{"all_nodes_invalid", "invalid", false, "", true, func() {
			originalListTridentNodes := listTridentNodes
			listTridentNodes = func(namespace string) (map[string]string, error) {
				return map[string]string{"node1": "pod1", "node2": "pod2"}, nil
			}
			t.Cleanup(func() { listTridentNodes = originalListTridentNodes })
		}},

		// Operator logs
		{"operator_current", logNameTridentOperator, false, "", false, nil},
		{"operator_previous", logNameTridentOperatorPrevious, false, "", false, nil},
		{"operator_invalid", "invalid", false, "", true, nil},
	}

	for _, tt := range tests {
		t.Run(tt.testType, func(t *testing.T) {
			withLogTestMode(t, func() {
				archive = false
				sidecars = tt.sidecars

				// Setup globals
				TridentPodName = testPodName
				TridentPodNamespace = "test-namespace"
				tridentOperatorPodName = testPodName
				tridentOperatorPodNamespace = "test-namespace"

				if tt.setupFunc != nil {
					tt.setupFunc()
				}

				if !strings.Contains(tt.testType, "invalid") && tt.testType != "trident_exec_error" {
					restoreMock := mockExecKubernetesCLI()
					defer restoreMock()
				}

				// Capture stdout
				oldStdout := os.Stdout
				_, w, _ := os.Pipe()
				os.Stdout = w

				var err error
				switch {
				case strings.Contains(tt.testType, "trident"):
					err = getTridentLogs(tt.logName)
				case strings.Contains(tt.testType, "node") && tt.nodeName != "":
					err = getNodeLogs(tt.logName, tt.nodeName)
				case strings.Contains(tt.testType, "all_nodes"):
					err = getAllNodeLogs(tt.logName)
				case strings.Contains(tt.testType, "operator"):
					err = getTridentOperatorLogs(tt.logName)
				}

				w.Close()
				os.Stdout = oldStdout

				if tt.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestAppendError(t *testing.T) {
	tests := []struct {
		oldErrors []byte
		newError  []byte
		expected  string
	}{
		{[]byte{}, []byte("new error"), "new error"},
		{[]byte("old error"), []byte("new error"), "old error. new error"},
		{[]byte("old error."), []byte("new error"), "old error. new error"},
		{[]byte("old error  "), []byte("new error"), "old error. new error"},
		{[]byte("  old error.  "), []byte("  new error  "), "old error.   new error  "},
		{[]byte("existing"), []byte(""), "existing. "},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			result := appendError(tt.oldErrors, tt.newError)
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

func TestAppendErrorf(t *testing.T) {
	tests := []struct {
		oldErrors    []byte
		formatString string
		args         []interface{}
		expected     string
	}{
		{[]byte{}, "error: %s", []interface{}{"test"}, "error: test"},
		{[]byte("old error"), "new error: %d", []interface{}{123}, "old error. new error: 123"},
		{[]byte{}, "error %s: %d", []interface{}{"test", 456}, "error test: 456"},
		{[]byte("existing"), "%s %s", []interface{}{"new", "format"}, "existing. new format"},
		{[]byte("test"), "%v", []interface{}{nil}, "test. <nil>"},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			result := appendErrorf(tt.oldErrors, tt.formatString, tt.args...)
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

func TestLogsCommandRunE(t *testing.T) {
	tests := []struct {
		logType     string
		archive     bool
		expectError bool
	}{
		{logTypeAuto, false, false},
		{logTypeAuto, true, false},
		{logTypeTrident, false, false},
		{logTypeTrident, true, false},
		{"invalid", false, true},
		{"invalid", true, true},
	}

	for _, tt := range tests {
		mode := "console"
		if tt.archive {
			mode = "archive"
		}
		t.Run(tt.logType+"_"+mode, func(t *testing.T) {
			withLogTestMode(t, func() {
				logType = tt.logType
				archive = tt.archive

				getLogs = func() error {
					return nil
				}

				if tt.archive {
					tmpDir, err := os.MkdirTemp("", "logs-test-")
					require.NoError(t, err)
					defer os.RemoveAll(tmpDir)

					oldDir, _ := os.Getwd()
					os.Chdir(tmpDir)
					defer os.Chdir(oldDir)
				}

				err := logsCmd.RunE(logsCmd, []string{})

				if tt.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}
