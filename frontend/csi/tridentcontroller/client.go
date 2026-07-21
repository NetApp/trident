// Copyright 2026 NetApp, Inc. All Rights Reserved.

package tridentcontroller

//go:generate mockgen -destination=../../../mocks/mock_frontend/mock_csi/mock_tridentcontroller/mock_tridentcontroller.go github.com/netapp/trident/frontend/csi/tridentcontroller Client,ChangeNotifier

import (
	"context"
	"time"

	controllerAPI "github.com/netapp/trident/frontend/csi/controller_api"
	"github.com/netapp/trident/utils/models"
)

// Client is how the CSI node process reaches the Trident controller.
// Callers express operations (register, read publications, report cleanup); transports implement how.
type Client interface {
	// RegisterNode creates or updates node identity and waits for controller acknowledgement.
	RegisterNode(ctx context.Context, node *models.Node, timeout time.Duration) (*RegistrationInfo, error)
	// GetDesiredPublications returns the controller's desired publication state for the node.
	GetDesiredPublications(ctx context.Context, nodeName string) (map[string]*models.VolumePublicationExternal, error)
	// GetNodeCleanupStatus returns the node's publication cleanup state from the controller.
	GetNodeCleanupStatus(ctx context.Context, nodeName string) (models.NodePublicationState, error)
	// MarkNodeCleanupComplete signals that the node finished local cleanup work.
	MarkNodeCleanupComplete(ctx context.Context, nodeName string) error
}

// ChangeNotifier is optional: CRD mode can push publication/cleanup changes; plain REST uses polling.
type ChangeNotifier interface {
	// StartNodeCleanupWatch delivers a coalesced signal channel when publication or cleanup state changes.
	StartNodeCleanupWatch(ctx context.Context, nodeName string) (<-chan struct{}, func(), error)
}

// ClientFactory builds a transport-specific controller client for the active node helper.
type ClientFactory interface {
	// NewControllerClient selects CRD or REST transport for the active node helper.
	NewControllerClient(restClient controllerAPI.TridentController) (Client, error)
}

// WireClient builds the controller client from a node helper factory and REST backchannel.
func WireClient(factory ClientFactory, restClient controllerAPI.TridentController) (Client, error) {
	return factory.NewControllerClient(restClient)
}
