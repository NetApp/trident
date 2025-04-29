package handlers

import (
	"encoding/json"
	"fmt"

	. "github.com/netapp/trident/logging"
)

type ErrorXTimesAfterYTimesHandler struct {
	Name          string `json:"name"`
	HitCount      int    `json:"hitCount"`
	PassCount     int    `json:"passCount"`
	FailCount     int    `json:"failCount"`
	ErrorHitCount int    `json:"errorHitCount"`
}

func (handler *ErrorXTimesAfterYTimesHandler) Handle() error {
	Log().Debugf("Firing %s handler.", handler.Name)

	// While the passCount is greater than the hitCount, this handler should return nil.
	if handler.HitCount < handler.PassCount {
		handler.HitCount++
		remaining := handler.PassCount - handler.HitCount
		Log().Debugf("%v remaining passes from %s handler.", remaining, handler.Name)
		return nil
	}

	// Once passCount is reached, start erroring for FailCount times.
	if handler.ErrorHitCount < handler.FailCount {
		handler.ErrorHitCount++
		remaining := handler.FailCount - handler.ErrorHitCount
		Log().Debugf("%v remaining errors from %s handler.", remaining, handler.Name)
		return fmt.Errorf("fiji error from [%s] handler; %v errors remaining", handler.Name, remaining)
	}

	// After FailCount errors, succeed indefinitely.
	Log().Debugf("No errors remaining from %s handler.", handler.Name)
	return nil
}

func NewErrorXTimesAfterYTimesHandler(model []byte) (*ErrorXTimesAfterYTimesHandler, error) {
	var handler ErrorXTimesAfterYTimesHandler
	if err := json.Unmarshal(model, &handler); err != nil {
		return nil, err
	}

	// Validate PassCount and FailCount
	if handler.PassCount <= 0 {
		return nil, fmt.Errorf("invalid value specified for passCount: must be greater than 0")
	}

	if handler.FailCount <= 0 {
		return nil, fmt.Errorf("invalid value specified for failCount: must be greater than 0")
	}

	return &handler, nil
}
