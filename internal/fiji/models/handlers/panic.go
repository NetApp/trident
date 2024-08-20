package handlers

import (
	"encoding/json"
	"fmt"

	. "github.com/netapp/trident/logging"
)

type PanicHandler struct {
	Name string `json:"name"`
}

func (p *PanicHandler) Handle() (err error) {
	Log().Debugf("Firing %s handler.", p.Name)
	defer func() {
		Log().Debugf("Injecting panic from %s handler.", p.Name)
		panic(fmt.Errorf("panic from fiji handler %s", p.Name))
	}()
	return
}

func NewPanicHandler(model []byte) (*PanicHandler, error) {
	var handler PanicHandler
	if err := json.Unmarshal(model, &handler); err != nil {
		return nil, err
	}

	return &handler, nil
}
