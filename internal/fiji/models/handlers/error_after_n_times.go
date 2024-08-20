package handlers

import (
	"encoding/json"
	"fmt"

	. "github.com/netapp/trident/logging"
)

type ErrorAfterNTimesHandler struct {
	Name      string `json:"name"`
	HitCount  int    `json:"hitCount"`
	PassCount int    `json:"passCount"`
}

func (en *ErrorAfterNTimesHandler) Handle() error {
	Log().Debugf("Firing %s handler.", en.Name)
	// While the passCount is greater than the hitCount, this handler should return nil.
	// When the hitCount exceeds the passCount, this handler should fail indefinitely.
	en.HitCount++
	if en.HitCount <= en.PassCount {
		remaining := en.PassCount - en.HitCount
		Log().Debugf("%v remaining passes from %s handler.", remaining, en.Name)
		return nil
	}
	return fmt.Errorf("fiji error from [%s] handler; infinite errors remaining", en.Name)
}

func NewErrorAfterNTimesHandler(model []byte) (*ErrorAfterNTimesHandler, error) {
	var handler ErrorAfterNTimesHandler
	if err := json.Unmarshal(model, &handler); err != nil {
		return nil, err
	}

	if handler.PassCount <= 0 {
		return nil, fmt.Errorf("invalid value specified for passCount")
	}

	return &handler, nil
}
