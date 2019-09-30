// Copyright 2019 NetApp, Inc. All Rights Reserved.

package helpers

import (
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
)

const (
	KubernetesHelper = "k8s_csi_helper"
	PlainCSIHelper   = "plain_csi_helper"

	EventTypeNormal  = "Normal"
	EventTypeWarning = "Warning"
)

// HybridPlugin is the common interface used by the "helper" objects used by
// the CSI controller.  The helpers supply CO-specific details at certain
// points of CSI workflows.
type HybridPlugin interface {

	// GetVolumeConfig accepts the attributes of a volume being requested by the CSI
	// provisioner, adds in any CO-specific details about the new volume, and returns
	// a VolumeConfig structure as needed by Trident to create a new volume.
	GetVolumeConfig(
		name string, sizeBytes int64, parameters map[string]string,
		protocol config.Protocol, accessModes []config.AccessMode, volumeMode config.VolumeMode, fsType string,
	) (*storage.VolumeConfig, error)

	// GetSnapshotConfig accepts the attributes of a snapshot being requested byt the CSI
	// provisioner, adds in any CO-specific details about the new volume, and returns
	// a SnapshotConfig structure as needed by Trident to create a new snapshot.
	GetSnapshotConfig(volumeName, snapshotName string) (*storage.SnapshotConfig, error)

	// RecordVolumeEvent accepts the name of a CSI volume and writes the specified
	// event message in a manner appropriate to the container orchestrator.
	RecordVolumeEvent(name, eventType, reason, message string)

	// Version returns the version of the CO this helper is managing, or the supported
	// CSI version in the plain-CSI case.  This value is reported in Trident's telemetry.
	Version() string
}
