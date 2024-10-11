package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/netapp/trident/logging"
)

type PauseNTimesHandler struct {
	Name      string `json:"name"`
	Pause     pause  `json:"duration"`
	HitCount  int    `json:"hitCount"`
	FailCount int    `json:"failCount"`
}

func (p *PauseNTimesHandler) Handle() error {
	Log().Debugf("Firing %s handler.", p.Name)
	// While the hitCount is less than the failCount, this handler should fail.
	// Once the hitCount exceeds the failCount, this handler should return nil indefinitely.
	if p.HitCount < p.FailCount {
		p.HitCount++
		remaining := p.FailCount - p.HitCount
		Log().Debugf("%v remaining errors from %s handler.", remaining, p.Name)
		Log().Debugf("Injecting sleep for %s from %s handler.", p.Pause.String(), p.Name)
		time.Sleep(p.Pause.Abs())
		return nil
	}
	Log().Debugf("No errors remaining from %s handler.", p.Name)
	return nil
}

func NewPauseNTimesHandler(model []byte) (*PauseNTimesHandler, error) {
	var handler PauseNTimesHandler
	if err := json.Unmarshal(model, &handler); err != nil {
		return nil, err
	}

	if handler.FailCount <= 0 {
		return nil, fmt.Errorf("invalid value specified for failCount")
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
