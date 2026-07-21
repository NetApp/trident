// Copyright 2026 NetApp, Inc. All Rights Reserved.

package rest

import (
	"context"
	"fmt"
	"time"

	controllerAPI "github.com/netapp/trident/frontend/csi/controller_api"
	"github.com/netapp/trident/frontend/csi/tridentcontroller"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/utils/models"
)

// Client implements tridentcontroller.Client over the Trident controller HTTPS backchannel.
type Client struct {
	restClient controllerAPI.TridentController
}

// NewClient returns a REST-backed controller client for node-side use.
func NewClient(restClient controllerAPI.TridentController) *Client {
	return &Client{restClient: restClient}
}

// RegisterNode registers the node with the controller over the REST backchannel.
func (c *Client) RegisterNode(
	ctx context.Context, node *models.Node, _ time.Duration,
) (*tridentcontroller.RegistrationInfo, error) {
	if c.restClient == nil {
		return nil, fmt.Errorf("controller REST client is not configured")
	}
	resp, err := c.restClient.CreateNode(ctx, node)
	if err != nil {
		return nil, err
	}
	return &tridentcontroller.RegistrationInfo{
		TopologyLabels: resp.TopologyLabels,
		LogLevel:       resp.LogLevel,
		LogWorkflows:   resp.LogWorkflows,
		LogLayers:      resp.LogLayers,
	}, nil
}

// GetDesiredPublications lists desired volume publications for the node from the controller.
func (c *Client) GetDesiredPublications(
	ctx context.Context, nodeName string,
) (map[string]*models.VolumePublicationExternal, error) {
	if c.restClient == nil {
		return nil, fmt.Errorf("controller REST client is not configured")
	}
	publications, err := c.restClient.ListVolumePublicationsForNode(ctx, nodeName)
	if err != nil {
		return nil, err
	}
	desiredState := make(map[string]*models.VolumePublicationExternal, len(publications))
	for _, pub := range publications {
		desiredState[pub.VolumeName] = pub
	}
	return desiredState, nil
}

// GetNodeCleanupStatus reads the node's publication cleanup state from the controller.
func (c *Client) GetNodeCleanupStatus(ctx context.Context, nodeName string) (models.NodePublicationState, error) {
	if c.restClient == nil {
		return "", fmt.Errorf("controller REST client is not configured")
	}
	node, err := c.restClient.GetNode(ctx, nodeName)
	if err != nil {
		return "", err
	}
	return node.PublicationState, nil
}

// MarkNodeCleanupComplete signals completion of local cleanup to the controller.
func (c *Client) MarkNodeCleanupComplete(ctx context.Context, nodeName string) error {
	if c.restClient == nil {
		return fmt.Errorf("controller REST client is not configured")
	}
	return c.restClient.UpdateNode(ctx, nodeName, &models.NodePublicationStateFlags{ProvisionerReady: convert.ToPtr(true)})
}
