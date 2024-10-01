// Copyright 2024 NetApp, Inc. All Rights Reserved.

package instruction

import (
	"context"

	"github.com/netapp/trident/internal/nodeprep/step"
	. "github.com/netapp/trident/logging"
)

const defaultInstructionName = "default instructions"

type Default struct {
	name  string
	steps []step.Step
}

func (r *Default) GetName() string {
	if r.name == "" {
		return defaultInstructionName
	}
	return r.name
}

func (r *Default) GetSteps() []step.Step {
	return r.steps
}

func (r *Default) PreCheck(_ context.Context) error {
	return nil
}

func (r *Default) Apply(ctx context.Context) error {
	Log().Infof("Applying %s", r.GetName())
	for _, s := range r.steps {
		if err := executeStep(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func executeStep(context context.Context, step step.Step) error {
	Log().WithField("step", step.GetName()).Info("Executing step")

	if err := step.Apply(context); err != nil {
		if step.IsRequired() {
			return err
		}
		Log().WithError(err).WithField("step", step.GetName()).
			Info("apply failed but step is not required, continuing")
	}

	Log().WithField("step", step.GetName()).
		Info("successfully applied step")
	return nil
}

func (r *Default) PostCheck(_ context.Context) error {
	return nil
}
