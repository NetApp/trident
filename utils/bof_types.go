package utils

import (
	"context"
)

//go:generate mockgen -destination=../mocks/mock_utils/mock_bof_utils.go github.com/netapp/trident/utils BlockOnFileReconcileUtils

type BlockOnFileReconcileUtils interface {
	GetLoopDeviceAttachedToFile(context.Context, string) (bool, *LoopDevice, error)
	ReconcileBlockOnFileVolumeInfo(ctx context.Context, trackingInfo *VolumeTrackingInfo) (bool, error)
}

type BlockOnFileReconcileHelper struct{}

func NewBlockOnFileReconcileUtils() BlockOnFileReconcileUtils {
	return &BlockOnFileReconcileHelper{}
}
