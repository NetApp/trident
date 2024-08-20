package models

import (
	"encoding/json"
	"fmt"

	"github.com/netapp/trident/internal/fiji/models/handlers"
)

// HandlerType tells the factory which fault to create.
type HandlerType string

func (t HandlerType) String() string {
	return string(t)
}

const (
	// Never tells the fault to never fail.
	Never HandlerType = "never"
	// Pause tells the fault to pause for 'n' time then resets pause to 0.
	Pause HandlerType = "pause"
	// Panic tells the fault to start panicking.
	// This does not kill the process immediately and any deferred functions execute.
	Panic HandlerType = "panic"
	// Exit kills the process immediately. Deferred functions do not execute.
	Exit HandlerType = "exit"
	// Always tells the fault to error indefinitely.
	Always HandlerType = "always"
	// ErrorNTimes tells the fault to error up to 'n' times then succeed indefinitely.
	ErrorNTimes HandlerType = "error-n-times"
	// ErrorAfterNTimes tells the fault to error after 'n' times indefinitely.
	ErrorAfterNTimes HandlerType = "error-after-n-times"
	// ExitAfterNTimes tells the fault to exit the process after 'n' times.
	ExitAfterNTimes HandlerType = "exit-after-n-times"
)

// NewFaultHandlerFromModel dynamically creates a fault handler for use by a fault.
func NewFaultHandlerFromModel(model []byte) (FaultHandler, error) {
	// Create a temporary structure to pick get the model name.
	getter := struct{ Name string }{}
	if err := json.Unmarshal(model, &getter); err != nil {
		return nil, err
	}
	name := getter.Name

	switch HandlerType(name) {
	case Never:
		return handlers.NewNeverErrorHandler(model)
	case Pause:
		return handlers.NewPauseHandler(model)
	case Panic:
		return handlers.NewPanicHandler(model)
	case Exit:
		return handlers.NewExitHandler(model)
	case Always:
		return handlers.NewAlwaysErrorHandler(model)
	case ErrorNTimes:
		return handlers.NewErrorNTimesHandler(model)
	case ErrorAfterNTimes:
		return handlers.NewErrorAfterNTimesHandler(model)
	case ExitAfterNTimes:
		return handlers.NewExitAfterNTimesHandler(model)
	}

	return nil, fmt.Errorf("invalid value \"%s\" specified for \"Name\" in handler config", name)
}
