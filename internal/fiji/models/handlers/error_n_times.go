package handlers

import (
	"encoding/json"
	"fmt"

	. "github.com/netapp/trident/logging"
)

type ErrorNTimesHandler struct {
	Name      string `json:"name"`
	HitCount  int    `json:"hitCount"`
	FailCount int    `json:"failCount"`
}

func (fn *ErrorNTimesHandler) Handle() error {
	Log().Debugf("Firing %s handler.", fn.Name)
	// While the hitCount is less than the failCount, this handler should fail.
	// Once the hitCount exceeds the failCount, this handler should return nil indefinitely.
	if fn.HitCount < fn.FailCount {
		fn.HitCount++
		remaining := fn.FailCount - fn.HitCount
		Log().Debugf("%v remaining errors from %s handler.", remaining, fn.Name)
		return fmt.Errorf("fiji error from [%s] handler; %v errors remaining", fn.Name, remaining)
	}
	Log().Debugf("No errors remaining from %s handler.", fn.Name)
	return nil
}

func NewErrorNTimesHandler(model []byte) (*ErrorNTimesHandler, error) {
	var handler ErrorNTimesHandler
	if err := json.Unmarshal(model, &handler); err != nil {
		return nil, err
	}

	if handler.FailCount <= 0 {
		return nil, fmt.Errorf("invalid value specified for failCount")
	}

	return &handler, nil
}
