// Copyright 2024 NetApp, Inc. All Rights Reserved.

package step

import "context"

const defaultStepName = "default step"

type DefaultStep struct {
	Name     string
	Required bool
}

func (s *DefaultStep) GetName() string {
	if s.Name == "" {
		return defaultStepName
	}
	return s.Name
}

func (s *DefaultStep) IsRequired() bool {
	return s.Required
}

func (s *DefaultStep) Apply(_ context.Context) error {
	return nil
}
