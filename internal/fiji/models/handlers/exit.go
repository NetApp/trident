package handlers

import (
	"encoding/json"
	"fmt"
	"os"

	. "github.com/netapp/trident/logging"
)

type ExitHandler struct {
	Name     string `json:"name"`
	ExitCode int    `json:"exitCode"`
}

func (e *ExitHandler) Handle() error {
	Log().Debugf("Firing %s handler", e.Name)
	if e.ExitCode >= 0 {
		Log().Debugf("Injecting exit with code %v from %s handler.", e.ExitCode, e.Name)
		os.Exit(e.ExitCode)
	}
	return nil
}

func NewExitHandler(model []byte) (*ExitHandler, error) {
	var handler ExitHandler
	if err := json.Unmarshal(model, &handler); err != nil {
		return nil, err
	}

	if handler.ExitCode < 0 {
		return nil, fmt.Errorf("invalid value specified for exitCode")
	}

	return &handler, nil
}
