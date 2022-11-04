package utils

import (
	"context"
)

//go:generate mockgen -destination=../mocks/mock_utils/mock_iscsi_utils.go github.com/netapp/trident/utils IscsiReconcileUtils

type IscsiReconcileUtils interface {
	GetISCSIHostSessionMapForTarget(context.Context, string) map[int]int
	GetSysfsBlockDirsForLUN(int, map[int]int) []string
	GetDevicesForLUN(paths []string) ([]string, error)
	ReconcileISCSIVolumeInfo(ctx context.Context, trackingInfo *VolumeTrackingInfo) (bool, error)
}

type IscsiReconcileHelper struct{}

func NewIscsiReconcileUtils() IscsiReconcileUtils {
	return &IscsiReconcileHelper{}
}
