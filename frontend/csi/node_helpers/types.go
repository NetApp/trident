// Copyright 2022 NetApp, Inc. All Rights Reserved.

package nodehelpers

import (
	"context"
	"os"

	"github.com/netapp/trident/utils/models"
)

//go:generate mockgen -destination=../../../mocks/mock_frontend/mock_csi/mock_node_helpers/mock_node_helpers.go github.com/netapp/trident/frontend/csi/node_helpers NodeHelper,VolumePublishManager

const (
	KubernetesHelper = "k8s_csi_node_helper"
	PlainCSIHelper   = "plain_csi_node_helper"
)

type NodeHelper interface {
	AddPublishedPath(ctx context.Context, volumeID, pathToAdd string) error
	RemovePublishedPath(ctx context.Context, volumeID, pathToRemove string) error
	VolumePublishManager
}

// VolumePublishManager is the common interface used by the "helper" objects used by
// the CSI node. The node_helpers supply CO-specific details at certain
// points of CSI workflows.
type VolumePublishManager interface {
	WriteTrackingInfo(context.Context, string, *models.VolumeTrackingInfo) error
	ReadTrackingInfo(context.Context, string) (*models.VolumeTrackingInfo, error)
	DeleteTrackingInfo(context.Context, string) error
	ListVolumeTrackingInfo(context.Context) (map[string]*models.VolumeTrackingInfo, error)
	UpgradeVolumeTrackingFile(context.Context, string, map[string]struct{}, map[string]string) (bool, error)
	DeleteFailedUpgradeTrackingFile(context.Context, os.FileInfo)
	ValidateTrackingFile(context.Context, string) (bool, error)
	GetVolumeTrackingFiles() ([]os.FileInfo, error)
}
