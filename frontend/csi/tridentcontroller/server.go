// Copyright 2026 NetApp, Inc. All Rights Reserved.

package tridentcontroller

import (
	"context"

	"github.com/netapp/trident/utils/models"
)

// NodeEventRecorder records Kubernetes (or CO) events during registration handling.
type NodeEventRecorder func(eventType, reason, message string)

// RegistrationInfo carries controller-assigned node metadata after successful registration.
type RegistrationInfo struct {
	// TopologyLabels are the topology segments the controller assigned to the node.
	TopologyLabels map[string]string
	// LogLevel is the orchestrator log level the node should adopt after registration.
	LogLevel string
	// LogWorkflows is the orchestrator workflow filter the node should adopt after registration.
	LogWorkflows string
	// LogLayers is the orchestrator layer filter the node should adopt after registration.
	LogLayers string
}

// Server handles inbound node registration regardless of transport.
// Transport adapters (REST handlers, TridentNode CR reconciliation) parse their
// wire format into models.Node, call RegisterNode, and write acknowledgements
// back through their transport.
type Server interface {
	// RegisterNode records node identity in orchestrator state and returns controller-acknowledged metadata.
	RegisterNode(ctx context.Context, node *models.Node, recordEvent NodeEventRecorder) (*RegistrationInfo, error)
}
