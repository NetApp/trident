// Copyright 2024 NetApp, Inc. All Rights Reserved.

package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTridentOrchestrator_HasTridentInstallationFailed(t *testing.T) {
	torc := TridentOrchestrator{Status: TridentOrchestratorStatus{}}

	tests := []struct {
		name     string
		status   []AppStatus
		expected bool
	}{
		{
			name:     "InstallFailed",
			status:   []AppStatus{AppStatusFailed, AppStatusError},
			expected: true,
		},
		{
			name: "InstallNotFailed",
			status: []AppStatus{
				AppStatusNotInstalled, AppStatusInstalling, AppStatusInstalled, AppStatusUninstalling,
				AppStatusUninstalled, AppStatusUpdating,
			},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, s := range test.status {
				torc.Status.Status = string(s)

				actual := torc.HasTridentInstallationFailed()

				assert.Equal(t, test.expected, actual, "Unexpected Torc status.")
			}
		})
	}
}

func TestTridentOrchestrator_IsTridentOperationInProgress(t *testing.T) {
	torc := TridentOrchestrator{Status: TridentOrchestratorStatus{}}

	tests := []struct {
		name     string
		status   []AppStatus
		expected bool
	}{
		{
			name:     "OperationInProgress",
			status:   []AppStatus{AppStatusNotInstalled, AppStatusInstalling, AppStatusUninstalling, AppStatusUpdating},
			expected: true,
		},
		{
			name:     "OperationNotInProgress",
			status:   []AppStatus{AppStatusInstalled, AppStatusUninstalled, AppStatusFailed, AppStatusError},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, s := range test.status {
				torc.Status.Status = string(s)

				actual := torc.IsTridentOperationInProgress()

				assert.Equal(t, test.expected, actual, "Unexpected Torc status.")
			}
		})
	}
}
