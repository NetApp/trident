// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
)

func TestUpdateVolumeCmd_RunE(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	tests := []struct {
		name          string
		operatingMode string
		args          []string
		snapshotDir   string
		poolLevel     bool
		updateSuccess bool
		wantErr       bool
		errorContains string
		useTunnelMock bool
	}{
		{
			name: "direct_mode_success_true", operatingMode: ModeDirect, args: []string{"vol1"},
			snapshotDir: "true", poolLevel: false, updateSuccess: true,
		},
		{
			name: "direct_mode_success_false_pool", operatingMode: ModeDirect, args: []string{"vol2"},
			snapshotDir: "false", poolLevel: true, updateSuccess: true,
		},
		{
			name: "direct_mode_validation_error_no_args", operatingMode: ModeDirect, args: []string{},
			snapshotDir: "true", wantErr: true, errorContains: "volume name not specified",
		},
		{
			name: "direct_mode_validation_error_multiple_args", operatingMode: ModeDirect, args: []string{"vol1", "vol2"},
			snapshotDir: "true", wantErr: true, errorContains: "multiple volume names specified",
		},
		{
			name: "direct_mode_validation_error_empty_snapshot", operatingMode: ModeDirect, args: []string{"vol3"},
			snapshotDir: "", wantErr: true, errorContains: "no value for snapshot directory provided",
		},
		{
			name: "direct_mode_validation_error_invalid_bool", operatingMode: ModeDirect, args: []string{"vol4"},
			snapshotDir: "invalid", wantErr: true, errorContains: "strconv.ParseBool",
		},
		{
			name: "direct_mode_update_fails", operatingMode: ModeDirect, args: []string{"vol5"},
			snapshotDir: "true", poolLevel: false, updateSuccess: false,
			wantErr: true, errorContains: "update failed",
		},
		{
			name: "tunnel_mode_success", operatingMode: ModeTunnel, args: []string{"vol6"},
			snapshotDir: "true", poolLevel: true, useTunnelMock: true,
			wantErr: false,
		},
		{
			name: "tunnel_mode_false_no_pool", operatingMode: ModeTunnel, args: []string{"vol7"},
			snapshotDir: "false", poolLevel: false, useTunnelMock: true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prevOperatingMode := OperatingMode
			prevSnapshotDir := snapshotDirectory
			prevPoolLevel := poolLevel

			defer func() {
				OperatingMode = prevOperatingMode
				snapshotDirectory = prevSnapshotDir
				poolLevel = prevPoolLevel
			}()

			// Set test values
			OperatingMode = tt.operatingMode
			snapshotDirectory = tt.snapshotDir
			poolLevel = tt.poolLevel

			// Mock tunnel command if needed
			if tt.useTunnelMock {
				originalExecKubernetesCLIRaw := execKubernetesCLIRaw
				execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
					mockResponse := `{"volume":{"name":"` + tt.args[0] + `","size":"1Gi","state":"online","protocol":"file","backend":"backend1","pool":"pool1","internalName":"internal_` + tt.args[0] + `","config":{"name":"` + tt.args[0] + `","size":"1Gi"},"accessInfo":{},"snapshotDirectory":"` + tt.snapshotDir + `"}}`

					cmd := exec.Command("echo", mockResponse)
					return cmd
				}
				defer func() {
					execKubernetesCLIRaw = originalExecKubernetesCLIRaw
				}()
			}

			// Mock HTTP for direct mode
			if tt.operatingMode == ModeDirect && len(tt.args) > 0 && tt.snapshotDir != "" && tt.snapshotDir != "invalid" {
				httpmock.Reset()
				if tt.updateSuccess {
					url := fmt.Sprintf("%s/volume/%s", BaseURL(), tt.args[0])
					response := `{"volume":{"name":"` + tt.args[0] + `","size":"1Gi","state":"online","protocol":"file","backend":"backend1","pool":"pool1","internalName":"internal_` + tt.args[0] + `","config":{"name":"` + tt.args[0] + `","size":"1Gi"},"accessInfo":{},"snapshotDirectory":"` + tt.snapshotDir + `"}}`
					httpmock.RegisterResponder("PUT", url, httpmock.NewStringResponder(200, response))
				} else {
					url := fmt.Sprintf("%s/volume/%s", BaseURL(), tt.args[0])
					httpmock.RegisterResponder("PUT", url, httpmock.NewErrorResponder(errors.New("update failed")))
				}
			}

			// Execute
			cmd := &cobra.Command{}
			err := updateVolumeCmd.RunE(cmd, tt.args)

			// Verify
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCmd(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		snapshotDir   string
		wantErr       bool
		errorContains string
	}{
		{
			name: "valid_single_arg_true", args: []string{"volume1"}, snapshotDir: "true",
		},
		{
			name: "valid_single_arg_false", args: []string{"volume2"}, snapshotDir: "false",
		},
		{
			name: "no_args", args: []string{}, snapshotDir: "true",
			wantErr: true, errorContains: "volume name not specified",
		},
		{
			name: "multiple_args", args: []string{"vol1", "vol2"}, snapshotDir: "true",
			wantErr: true, errorContains: "multiple volume names specified",
		},
		{
			name: "empty_snapshot_dir", args: []string{"volume3"}, snapshotDir: "",
			wantErr: true, errorContains: "no value for snapshot directory provided",
		},
		{
			name: "invalid_snapshot_dir", args: []string{"volume4"}, snapshotDir: "invalid",
			wantErr: true, errorContains: "strconv.ParseBool",
		},
		{
			name: "numeric_snapshot_dir", args: []string{"volume5"}, snapshotDir: "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCmd(tt.args, tt.snapshotDir)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpdateVolume(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	tests := []struct {
		name          string
		volumeName    string
		snapDirValue  string
		poolLevelVal  bool
		httpStatus    int
		httpResponse  string
		httpError     error
		wantErr       bool
		errorContains string
	}{
		{
			name: "success_true_pool", volumeName: "vol1", snapDirValue: "true", poolLevelVal: true,
			httpStatus: 200, httpResponse: `{"volume":{"name":"vol1","size":"1Gi","state":"online","protocol":"file","storageClass":"basic","snapshotDirectory":"true","config":{"name":"vol1","size":"1Gi","storageClass":"standard","protocol":"nfs","accessMode":"ReadWriteOnce"}}}`,
		},
		{
			name: "success_false_no_pool", volumeName: "vol2", snapDirValue: "false", poolLevelVal: false,
			httpStatus: 200, httpResponse: `{"volume":{"name":"vol2","size":"2Gi","state":"online","protocol":"block","storageClass":"premium","snapshotDirectory":"false","config":{"name":"vol2","size":"2Gi","storageClass":"premium","protocol":"block","accessMode":"ReadWriteOnce"}}}`,
		},
		{
			name: "http_error", volumeName: "vol3", snapDirValue: "true", poolLevelVal: false,
			httpError: errors.New("network error"), wantErr: true, errorContains: "network error",
		},
		{
			name: "http_404", volumeName: "vol4", snapDirValue: "false", poolLevelVal: true,
			httpStatus: 404, httpResponse: `{"error":"volume not found"}`,
			wantErr: true, errorContains: "failed to update volume",
		},
		{
			name: "http_500", volumeName: "vol5", snapDirValue: "true", poolLevelVal: false,
			httpStatus: 500, httpResponse: `{"error":"internal server error"}`,
			wantErr: true, errorContains: "failed to update volume",
		},
		{
			name: "invalid_json_response", volumeName: "vol6", snapDirValue: "false", poolLevelVal: true,
			httpStatus: 200, httpResponse: `invalid json response`,
			wantErr: true, errorContains: "invalid character",
		},
		{
			name: "numeric_snap_dir", volumeName: "vol7", snapDirValue: "1", poolLevelVal: false,
			httpStatus: 200, httpResponse: `{"volume":{"name":"vol7","size":"1Gi","state":"online","protocol":"file","storageClass":"basic","snapshotDirectory":"true","config":{"name":"vol7","size":"1Gi","storageClass":"standard","protocol":"nfs","accessMode":"ReadWriteOnce"}}}`,
		},
		{
			name: "zero_snap_dir", volumeName: "vol8", snapDirValue: "0", poolLevelVal: true,
			httpStatus: 200, httpResponse: `{"volume":{"name":"vol8","size":"1Gi","state":"online","protocol":"file","storageClass":"basic","snapshotDirectory":"false","config":{"name":"vol8","size":"1Gi","storageClass":"standard","protocol":"nfs","accessMode":"ReadWriteOnce"}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Reset()

			url := fmt.Sprintf("%s/volume/%s", BaseURL(), tt.volumeName)
			if tt.httpError != nil {
				httpmock.RegisterResponder("PUT", url, httpmock.NewErrorResponder(tt.httpError))
			} else {
				httpmock.RegisterResponder("PUT", url, httpmock.NewStringResponder(tt.httpStatus, tt.httpResponse))
			}

			err := updateVolume(tt.volumeName, tt.snapDirValue, tt.poolLevelVal)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
