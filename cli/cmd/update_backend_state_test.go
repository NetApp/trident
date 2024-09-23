// Copyright 2024 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/storage"
	execCmd "github.com/netapp/trident/utils/exec"
)

func executeCommandC(root *cobra.Command, args ...string) (c *cobra.Command, output string, err error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	c, err = root.ExecuteC()

	return c, buf.String(), err
}

func newTridentCtlCommand() *cobra.Command {
	tridentCtlCmd := cobra.Command{
		Use: "tridentctl",
	}
	return &tridentCtlCmd
}

func newUpdateCommand() *cobra.Command {
	updateCmd := &cobra.Command{
		Use: "update",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	return updateCmd
}

func newBackendCommand() *cobra.Command {
	backendCmd := &cobra.Command{
		Use: "backend",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	return backendCmd
}

func addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&backendState, "state", "", "", "New backend state")
	cmd.Flags().StringVarP(&userState, "user-state", "", "", "User-defined backend state : (suspended, normal)")
}

var (
	rescueStdout *os.File
	r, w         *os.File
)

func changeSTDOUT() {
	rescueStdout = os.Stdout
	r, w, _ = os.Pipe()
	os.Stdout = w
}

func restoreSTDOUT() {
	if err := w.Close(); err != nil {
		panic(err)
	}
	os.Stdout = rescueStdout
}

func getFakeBackends() []storage.BackendExternal {
	return []storage.BackendExternal{
		{
			Name:        "something",
			BackendUUID: "1234",
			State:       storage.Online,
			UserState:   storage.UserNormal,
			Online:      true,
			Config: map[string]interface{}{
				"storageDriverName": "ontap-nas",
			},
			Volumes: []string{"vol1", "vol2"},
		},
		{
			Name:        "fake",
			BackendUUID: "48467",
			State:       storage.Online,
			UserState:   storage.UserNormal,
			Online:      true,
			Config: map[string]interface{}{
				"storageDriverName": "gcp",
			},
			Volumes: []string{},
		},
	}
}

type cmdValidate struct{ t []string }

func getCmdValidate(t []string) gomock.Matcher {
	return &cmdValidate{t}
}

func (o *cmdValidate) Matches(x interface{}) bool {
	cmdArray, ok := x.([]string)
	if !ok {
		return false
	}
	cmd := strings.Join(cmdArray, " ")
	re := regexp.MustCompile(".*update backend state (--state|--user-state) (failed|normal|suspended).*")
	return re.MatchString(cmd)
}

func (o *cmdValidate) String() string {
	return "must match the regex: " + "`.*update backend state (--state|--user-state) (failed|normal|suspended).*`"
}

func TestValidateUpdateBackendStateArguments(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		output string
		error  bool
	}{
		{
			name:   "both flags set",
			args:   []string{"update", "backend", "state", "--state=something", "--user-state=fake"},
			output: "exactly one of --state or --user-state must be specified",
			error:  true,
		},
		{
			name:   "unknown state given in user-state flag",
			args:   []string{"update", "backend", "state", "--user-state=fake"},
			output: "invalid user-state fake specified",
			error:  false,
		},
		{
			name:   "correct state given in user-state flag",
			args:   []string{"update", "backend", "state", "--user-state=suspended"},
			output: "",
			error:  false,
		},
		{
			name:   "correct state given in user-state flag, but in all UPPERCASE letters",
			args:   []string{"update", "backend", "state", "--user-state=suspended"},
			output: "",
			error:  false,
		},
		{
			name:   "correct state given in state flag, but in all UPPERCASE letters",
			args:   []string{"update", "backend", "state", "--state=SUSPENDED"},
			output: "",
			error:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tridentCtlCmd := newTridentCtlCommand()
			updateCmd := newUpdateCommand()
			backendCmd := newBackendCommand()
			stateCmd := &cobra.Command{
				Use:  "state",
				RunE: validateUpdateBackendStateArguments,
			}

			addFlags(stateCmd)
			backendCmd.AddCommand(stateCmd)
			updateCmd.AddCommand(backendCmd)
			tridentCtlCmd.AddCommand(updateCmd)

			c, _, err := executeCommandC(tridentCtlCmd, tt.args...)

			assert.Equal(t, c.Name(), "state", "Expected command to be `state`")
			if tt.error {
				assert.Error(t, err)
				assert.Equal(t, tt.output, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpdateBackendStateRunE(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	defer func(previousCommand execCmd.Command) {
		command = previousCommand
	}(command)

	backends := getFakeBackends()

	command = mockCommand

	tests := []struct {
		name          string
		operatingMode string
		cmd           []string
		error         bool
		backends      []storage.BackendExternal
	}{
		{
			name:          "Tunnel Mode, no flag is set",
			operatingMode: ModeTunnel,
			cmd:           []string{"update", "backend", "state"},
			error:         true,
			backends:      []storage.BackendExternal{backends[0]},
		},
		{
			name:          "Direct mode, and no backend is given",
			operatingMode: ModeDirect,
			cmd:           []string{"update", "backend", "state", "--user-state", "normal"},
			error:         true,
			backends:      []storage.BackendExternal{backends[0], backends[1]},
		},
	}

	// tests which checks the overall functionality of the command.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := range tt.backends {
				// setting state of backends to the state given in the cmd.
				stateArg := tt.cmd[len(tt.cmd)-1]
				tt.backends[i].State = storage.BackendState(stateArg)
				tt.backends[i].UserState = storage.UserBackendState(stateArg)
			}

			changeSTDOUT()
			WriteBackends(tt.backends)
			restoreSTDOUT()
			tableInput, _ := io.ReadAll(r)

			tridentCtlCmd := newTridentCtlCommand()
			updateCmd := newUpdateCommand()
			backendCmd := newBackendCommand()
			stateCmd := &cobra.Command{
				Use:  "state",
				RunE: updateBackendStateRunE,
			}

			addFlags(stateCmd)
			backendCmd.AddCommand(stateCmd)
			updateCmd.AddCommand(backendCmd)
			tridentCtlCmd.AddCommand(updateCmd)

			OperatingMode = tt.operatingMode

			c, buf, err := executeCommandC(tridentCtlCmd, tt.cmd...)
			assert.Equal(t, "state", c.Name(), "Expected command to be `state`")

			if tt.error {
				assert.Error(t, err)
			} else {
				assert.Equal(t, string(tableInput), buf, "Expected output to be same as input")
			}
		})
	}

	// tests which checks whether the correct command is being received or not.
	test := []struct {
		name          string
		operatingMode string
		cmd           []string
		error         bool
	}{
		{
			name:          "Both state and user-state are provided",
			operatingMode: ModeTunnel,
			cmd:           []string{"update", "backend", "state", "--user-state", "normal"},
			error:         true,
		},
		{
			name:          "No flag is set",
			operatingMode: ModeTunnel,
			cmd:           []string{"update", "backend", "state"},
			error:         true,
		},
		{
			name:          "user-state flags is set to normal",
			operatingMode: ModeTunnel,
			cmd:           []string{"update", "backend", "state", "--user-state", "normal"},
			error:         false,
		},
	}

	for _, tt := range test {
		t.Run(tt.name, func(t *testing.T) {
			tridentCtlCmd := newTridentCtlCommand()
			updateCmd := newUpdateCommand()
			backendCmd := newBackendCommand()
			stateCmd := &cobra.Command{
				Use:  "state",
				RunE: updateBackendStateRunE,
			}

			addFlags(stateCmd)
			backendCmd.AddCommand(stateCmd)
			updateCmd.AddCommand(backendCmd)
			tridentCtlCmd.AddCommand(updateCmd)

			c, _, _ := executeCommandC(tridentCtlCmd, tt.cmd...)
			assert.Equal(t, "state", c.Name(), "Expected command to be `state`")
		})
	}
}

func TestUpdateBackendState(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	backends := getFakeBackends()

	tests := []struct {
		name                     string
		wantErr                  bool
		urlUpdateBackendResponse string
		urlGetBackendResponse    string
		backends                 []storage.BackendExternal
		getBackendResponse       httpmock.Responder
		updateBackendResponse    httpmock.Responder
		matchOutput              bool
	}{
		{
			name:                     "backend found",
			wantErr:                  false,
			urlUpdateBackendResponse: BaseURL() + "/backend/" + backends[0].Name + "/state",
			urlGetBackendResponse:    BaseURL() + "/backend/" + backends[0].Name,
			backends:                 []storage.BackendExternal{backends[0]},
			updateBackendResponse: func(req *http.Request) (*http.Response, error) {
				resp, err := httpmock.NewJsonResponse(200, rest.UpdateBackendResponse{
					BackendID: backends[0].Name,
					Error:     "",
				})
				if err != nil {
					return httpmock.NewStringResponse(500, ""), nil
				}
				return resp, nil
			},
			getBackendResponse: func(req *http.Request) (*http.Response, error) {
				resp, err := httpmock.NewJsonResponse(200, api.GetBackendResponse{
					Backend: backends[0],
					Error:   "",
				})
				if err != nil {
					return httpmock.NewStringResponse(500, ""), nil
				}
				return resp, nil
			},
			matchOutput: true,
		},
		{
			name:                     "more than one backend given",
			wantErr:                  true,
			backends:                 []storage.BackendExternal{backends[0], backends[1]},
			urlUpdateBackendResponse: BaseURL() + "/backend/" + backends[0].Name + "/state",
			urlGetBackendResponse:    BaseURL() + "/backend/" + backends[0].Name,
			matchOutput:              false,
		},
		{
			name:                     "backend not found",
			wantErr:                  true,
			backends:                 []storage.BackendExternal{backends[1]},
			urlUpdateBackendResponse: BaseURL() + "/backend/" + backends[1].Name + "/state",
			urlGetBackendResponse:    BaseURL() + "/backend/" + backends[1].Name + "fake",
			updateBackendResponse: func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(http.StatusNotFound, ""), nil
			},
			matchOutput: false,
		},
		{
			name:                     "cannot communicate to trident server",
			wantErr:                  true,
			backends:                 []storage.BackendExternal{backends[0]},
			urlUpdateBackendResponse: BaseURL() + "/backend/" + backends[0].Name + "/state",
			urlGetBackendResponse:    BaseURL() + "/backend/" + backends[0].Name,
			updateBackendResponse: func(req *http.Request) (*http.Response, error) {
				return nil, fmt.Errorf("trident server not found")
			},
			matchOutput: false,
		},

		{
			name:                     "no backend given",
			wantErr:                  true,
			backends:                 []storage.BackendExternal{},
			urlUpdateBackendResponse: BaseURL() + "/backend/" + "/state",
			urlGetBackendResponse:    BaseURL() + "/backend/",
			matchOutput:              false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changeSTDOUT()
			WriteBackends(tt.backends)
			restoreSTDOUT()

			tableInput, _ := io.ReadAll(r) // saving stdout output into a var tableInput.

			// Registering the urls to mock.
			httpmock.RegisterResponder("POST", tt.urlUpdateBackendResponse, tt.updateBackendResponse)
			httpmock.RegisterResponder("GET", tt.urlGetBackendResponse, tt.getBackendResponse)

			changeSTDOUT()

			var backendNames []string
			for _, backend := range tt.backends {
				backendNames = append(backendNames, backend.Name)
			}

			err := backendUpdateState(backendNames)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			restoreSTDOUT()

			tableOutput, _ := io.ReadAll(r)

			if tt.matchOutput {
				assert.Equal(t, tableInput, tableOutput, "should be equal")
			}
		})
	}
}
