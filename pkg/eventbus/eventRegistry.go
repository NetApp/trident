// Copyright 2025 NetApp, Inc. All Rights Reserved.

package eventbus

import (
	autogrowTypes "github.com/netapp/trident/frontend/autogrow/types"
	"github.com/netapp/trident/pkg/eventbus/types"
)

// Package eventbus provides a centralized event registry to track all event bus topics.
//
// EVENT REGISTRY - MANDATORY REGISTRATION PATTERN
//
// This file serves as the SINGLE SOURCE OF TRUTH for all event bus topics in the application.
// ALL event buses MUST be declared here to maintain code organization and prevent spaghetti code.

var (
	VolumeThresholdBreachedEventBus types.EventBusMetricsOptions[autogrowTypes.VolumeThresholdBreached]
	VolumesScheduledEventBus        types.EventBusMetricsOptions[autogrowTypes.VolumesScheduled]
	ControllerEventBus              types.EventBusMetricsOptions[autogrowTypes.ControllerEvent]
)
