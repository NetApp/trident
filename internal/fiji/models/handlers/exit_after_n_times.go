package handlers

import (
	"encoding/json"
	"fmt"
	"os"

	. "github.com/netapp/trident/logging"
)

type ExitAfterNTimesHandler struct {
	Name      string `json:"name"`
	ExitCode  int    `json:"exitCode"`
	HitCount  int    `json:"hitCount"`
	PassCount int    `json:"passCount"`
}

func (ent *ExitAfterNTimesHandler) Handle() error {
	Log().Debugf("Firing %s handler.", ent.Name)
	ent.HitCount++
	if ent.PassCount <= ent.HitCount {
		Log().Debugf("Injecting exit from %s handler.", ent.Name)
		os.Exit(ent.ExitCode)
	}
	remaining := ent.PassCount - ent.HitCount
	Log().Debugf("%v remaining passes from %s handler.", remaining, ent.Name)
	return nil
}

func NewExitAfterNTimesHandler(model []byte) (*ExitAfterNTimesHandler, error) {
	var handler ExitAfterNTimesHandler
	if err := json.Unmarshal(model, &handler); err != nil {
		return nil, err
	}

	if handler.ExitCode < 0 {
		return nil, fmt.Errorf("invalid value specified for exitCode")
	}
	if handler.PassCount <= 0 {
		return nil, fmt.Errorf("invalid value specified for passCount")
	}

	return &handler, nil
}
