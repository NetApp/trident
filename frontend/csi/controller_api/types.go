// Copyright 2022 NetApp, Inc. All Rights Reserved.

package controllerAPI

//go:generate mockgen -destination=../../../mocks/mock_frontend/mock_csi/mock_controller_api/mock_controller_api.go github.com/netapp/trident/frontend/csi/controller_api TridentController

import (
	"context"
	"net/http"

	"github.com/netapp/trident/utils/models"
)

type TridentController interface {
	InvokeAPI(
		ctx context.Context, requestBody []byte, method, resourcePath string, redactRequestBody,
		redactResponseBody bool,
	) (*http.Response, []byte, error)
	CreateNode(ctx context.Context, node *models.Node) (CreateNodeResponse, error)
	GetNode(ctx context.Context, nodeName string) (*models.NodeExternal, error)
	UpdateNode(ctx context.Context, nodeName string, nodeState *models.NodePublicationStateFlags) error
	GetNodes(ctx context.Context) ([]string, error)
	DeleteNode(ctx context.Context, name string) error
	GetChap(ctx context.Context, volume, node string) (*models.IscsiChapInfo, error)
	UpdateVolumeLUKSPassphraseNames(ctx context.Context, volume string, passphraseNames []string) error
	ListVolumePublicationsForNode(ctx context.Context, nodeName string) ([]*models.VolumePublicationExternal, error)
	// TODO (bpresnel) Enable later with rate-limiting?
	// GetLoggingConfig(ctx context.Context) (string, string, string, error)
}
