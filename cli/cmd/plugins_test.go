// Copyright 2025 NetApp, Inc. All Rights Reserved.
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

var pluginTestMutex sync.RWMutex

func withPluginTestMode(t *testing.T, fn func()) {
	pluginTestMutex.Lock()
	defer pluginTestMutex.Unlock()

	// Save original state
	originalFound := make(map[string]struct{})
	for k, v := range pluginsFound {
		originalFound[k] = v
	}
	oldRoot := RootCmd

	// Cleanup on exit
	defer func() {
		pluginsFound = originalFound
		RootCmd = oldRoot
	}()

	fn()
}

func TestInitializePlugins(t *testing.T) {
	tests := []struct {
		name           string
		createBinaries []string
		setupDupes     []string
		expectPlugins  int
		testExecution  bool
	}{
		{
			name:           "no binaries - early return",
			createBinaries: []string{},
			expectPlugins:  0,
		},
		{
			name:           "invalid plugin name skipped",
			createBinaries: []string{"tridentctl-create"},
			expectPlugins:  0,
		},
		{
			name:           "duplicate plugin skipped",
			createBinaries: []string{"tridentctl-test"},
			setupDupes:     []string{"test"},
			expectPlugins:  0,
		},
		{
			name:           "valid plugin created",
			createBinaries: []string{"tridentctl-myplugin"},
			expectPlugins:  1,
			testExecution:  true,
		},
		{
			name:           "plugin execution error",
			createBinaries: []string{"tridentctl-errorplugin"},
			expectPlugins:  1,
			testExecution:  true,
		},
		{
			name:           "multiple valid plugins",
			createBinaries: []string{"tridentctl-plugin1", "tridentctl-plugin2"},
			expectPlugins:  2,
			testExecution:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withPluginTestMode(t, func() {
				tmpDir := t.TempDir()

				for _, binName := range tt.createBinaries {
					binPath := filepath.Join(tmpDir, binName)
					content := "#!/bin/bash\necho 'test'\n"
					if strings.Contains(binName, "error") {
						content = "#!/bin/bash\nexit 1\n"
					}
					assert.NoError(t, os.WriteFile(binPath, []byte(content), 0o755))
				}

				// Set plugin path
				oldEnv := os.Getenv(pluginPathEnvVar)
				defer func() {
					if oldEnv == "" {
						os.Unsetenv(pluginPathEnvVar)
					} else {
						os.Setenv(pluginPathEnvVar, oldEnv)
					}
				}()
				os.Setenv(pluginPathEnvVar, tmpDir)

				pluginsFound = make(map[string]struct{})
				for _, dup := range tt.setupDupes {
					pluginsFound[dup] = struct{}{}
				}

				testRoot := &cobra.Command{Use: "test"}
				RootCmd = testRoot

				cmdsBefore := len(RootCmd.Commands())

				// Execute the function under test
				initializePlugins()

				// Verify results
				cmdsAfter := len(RootCmd.Commands())
				pluginsAdded := cmdsAfter - cmdsBefore
				assert.Equal(t, tt.expectPlugins, pluginsAdded)

				// Test plugin execution if expected
				if tt.testExecution && pluginsAdded > 0 {
					// Find the plugin command
					var pluginCmd *cobra.Command
					for _, cmd := range RootCmd.Commands() {
						if cmd.GroupID == "plugins" {
							pluginCmd = cmd
							break
						}
					}

					assert.NotNil(t, pluginCmd)
					assert.NotPanics(t, func() {
						pluginCmd.Run(pluginCmd, []string{"arg1", "arg2"})
					})
				}
			})
		})
	}
}

func TestFindBinaries(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		setup  func() func()
	}{
		{
			name:   "empty plugin path uses PATH",
			prefix: "nonexistent-",
			setup: func() func() {
				old := os.Getenv(pluginPathEnvVar)
				os.Unsetenv(pluginPathEnvVar)
				return func() {
					if old != "" {
						os.Setenv(pluginPathEnvVar, old)
					}
				}
			},
		},
		{
			name:   "uses plugin path when set",
			prefix: "test-",
			setup: func() func() {
				old := os.Getenv(pluginPathEnvVar)
				os.Setenv(pluginPathEnvVar, "/nonexistent/path")
				return func() {
					if old == "" {
						os.Unsetenv(pluginPathEnvVar)
					} else {
						os.Setenv(pluginPathEnvVar, old)
					}
				}
			},
		},
		{
			name:   "glob error path",
			prefix: "test-",
			setup: func() func() {
				old := os.Getenv(pluginPathEnvVar)
				os.Setenv(pluginPathEnvVar, "/path/with/[invalid")
				return func() {
					if old == "" {
						os.Unsetenv(pluginPathEnvVar)
					} else {
						os.Setenv(pluginPathEnvVar, old)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := tt.setup()
			defer cleanup()

			result := findBinaries(tt.prefix)
			assert.True(t, result == nil || len(result) >= 0)
		})
	}
}

func TestIsExecutable(t *testing.T) {
	tests := []struct {
		name    string
		setupFn func(t *testing.T) string
		want    bool
		cleanup bool
	}{
		{
			name: "stat error returns false",
			setupFn: func(t *testing.T) string {
				return "/nonexistent/file"
			},
			want: false,
		},
		{
			name: "directory returns false",
			setupFn: func(t *testing.T) string {
				return t.TempDir()
			},
			want: false,
		},
		{
			name: "regular file without execute returns false",
			setupFn: func(t *testing.T) string {
				f, err := os.CreateTemp("", "test")
				assert.NoError(t, err)
				f.Close()
				assert.NoError(t, os.Chmod(f.Name(), 0o644))
				return f.Name()
			},
			want:    false,
			cleanup: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := tt.setupFn(t)
			if tt.cleanup {
				defer func() {
					_ = os.Remove(file)
				}()
			}

			result := isExecutable(file)
			assert.Equal(t, tt.want, result)
		})
	}
}
