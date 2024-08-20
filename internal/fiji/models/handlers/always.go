package handlers

import (
	"encoding/json"
	"fmt"

	. "github.com/netapp/trident/logging"
)

type AlwaysErrorHandler struct {
	Name string `json:"name"`
}

func (a *AlwaysErrorHandler) Handle() error {
	Log().Debugf("Injecting error from %s handler.", a.Name)
	return fmt.Errorf("fiji error from %s handler", a.Name)
}

func NewAlwaysErrorHandler(model []byte) (*AlwaysErrorHandler, error) {
	var alwaysErrorHandler AlwaysErrorHandler
	if err := json.Unmarshal(model, &alwaysErrorHandler); err != nil {
		return nil, err
	}

	return &alwaysErrorHandler, nil
}
