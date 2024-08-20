package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/netapp/trident/logging"
)

type PauseHandler struct {
	Name  string `json:"name"`
	Pause pause  `json:"duration"`
}

func (p *PauseHandler) Handle() error {
	Log().Debugf("Injecting sleep for %s from %s handler.", p.Pause.String(), p.Name)
	time.Sleep(p.Pause.Abs())
	return nil
}

func NewPauseHandler(model []byte) (*PauseHandler, error) {
	var handler PauseHandler
	if err := json.Unmarshal(model, &handler); err != nil {
		return nil, err
	}

	return &handler, nil
}

type pause struct {
	time.Duration
}

func (p *pause) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.String())
}

// UnmarshalJSON allows custom unmarshalling so that "duration" may be specified as an amount of time with units.
// Example: "duration": "500ms".
func (p *pause) UnmarshalJSON(b []byte) error {
	// Check if nothing was supplied or if an empty string was.
	if len(b) == 0 || len(b) == 2 {
		return fmt.Errorf("invalid value specified for duration")
	}

	duration, err := time.ParseDuration(string(b[1 : len(b)-1]))
	if err != nil {
		return err
	}

	if duration <= 0 {
		return fmt.Errorf("invalid value specified for duration")
	}

	// Set the duration
	p.Duration = duration
	return nil
}
