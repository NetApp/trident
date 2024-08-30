package utils

import (
	"context"

	"github.com/netapp/trident/utils/models"
)

//go:generate mockgen -destination=../mocks/mock_utils/mock_iscsi_utils.go github.com/netapp/trident/utils IscsiReconcileUtils

type IscsiReconcileUtils interface {
	GetISCSIHostSessionMapForTarget(context.Context, string) map[int]int
	GetSysfsBlockDirsForLUN(int, map[int]int) []string
	GetMultipathDeviceUUID(string) (string, error)
	GetMultipathDeviceBySerial(context.Context, string) (string, error)
	GetMultipathDeviceDisks(context.Context, string) ([]string, error)
	GetDevicesForLUN(paths []string) ([]string, error)
	ReconcileISCSIVolumeInfo(ctx context.Context, trackingInfo *models.VolumeTrackingInfo) (bool, error)
}

type IscsiReconcileHelper struct{}

func NewIscsiReconcileUtils() IscsiReconcileUtils {
	return &IscsiReconcileHelper{}
}
