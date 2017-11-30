// Copyright 2016 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"strconv"

	"github.com/netapp/netappdvp/apis/ontap"
	"github.com/netapp/netappdvp/azgo"
	dvp "github.com/netapp/netappdvp/storage_drivers"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
)

// OntapNASStorageDriver is for NFS storage provisioning
type OntapNASStorageDriver struct {
	dvp.OntapNASStorageDriver
}

// Retrieve storage backend capabilities
func (d *OntapNASStorageDriver) GetStorageBackendSpecs(backend *storage.StorageBackend) error {

	backend.Name = "ontapnas_" + d.Config.DataLIF
	poolAttrs := d.GetStoragePoolAttributes()
	return getStorageBackendSpecsCommon(d, backend, poolAttrs)
}

func (d *OntapNASStorageDriver) GetStoragePoolAttributes() map[string]sa.Offer {

	return map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(d.Name()),
		sa.Snapshots:        sa.NewBoolOffer(true),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.Encryption:       sa.NewBoolOffer(d.API.SupportsApiFeature(ontap.NETAPP_VOLUME_ENCRYPTION)),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}
}

func (d *OntapNASStorageDriver) GetVolumeOpts(
	volConfig *storage.VolumeConfig,
	vc *storage.StoragePool,
	requests map[string]sa.Request,
) (map[string]string, error) {
	return getVolumeOptsCommon(volConfig, vc, requests), nil
}

func (d *OntapNASStorageDriver) GetInternalVolumeName(name string) string {
	return getInternalVolumeNameCommon(d.Config.CommonStorageDriverConfig, name)
}

func (d *OntapNASStorageDriver) CreatePrepare(volConfig *storage.VolumeConfig) bool {
	return createPrepareCommon(d, volConfig)
}

func (d *OntapNASStorageDriver) CreateFollowup(
	volConfig *storage.VolumeConfig,
) error {
	volConfig.AccessInfo.NfsServerIP = d.Config.DataLIF
	volConfig.AccessInfo.NfsPath = "/" + volConfig.InternalName
	volConfig.FileSystem = ""
	return nil
}

func (d *OntapNASStorageDriver) GetProtocol() config.Protocol {
	return config.File
}

func (d *OntapNASStorageDriver) GetDriverName() string {
	return d.Config.StorageDriverName
}

func (d *OntapNASStorageDriver) StoreConfig(
	b *storage.PersistentStorageBackendConfig,
) {
	storage.SanitizeCommonStorageDriverConfig(
		d.Config.CommonStorageDriverConfig)
	b.OntapConfig = &d.Config
}

func (d *OntapNASStorageDriver) GetExternalConfig() interface{} {
	return getExternalConfig(d.Config)
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *OntapNASStorageDriver) GetVolumeExternal(name string) (*storage.VolumeExternal, error) {

	volumeAttributes, err := d.API.VolumeGet(name)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(&volumeAttributes), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *OntapNASStorageDriver) GetVolumeExternalWrappers(
	channel chan *storage.VolumeExternalWrapper) {

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes matching the storage prefix
	volumesResponse, err := d.API.VolumeGetAll(*d.Config.StoragePrefix)
	if err = ontap.GetError(volumesResponse, err); err != nil {
		channel <- &storage.VolumeExternalWrapper{nil, err}
		return
	}

	// Convert all volumes to VolumeExternal and write them to the channel
	for _, volume := range volumesResponse.Result.AttributesList() {
		channel <- &storage.VolumeExternalWrapper{d.getVolumeExternal(&volume), nil}
	}
}

// getExternalVolume is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *OntapNASStorageDriver) getVolumeExternal(
	volumeAttrs *azgo.VolumeAttributesType) *storage.VolumeExternal {

	volumeExportAttrs := volumeAttrs.VolumeExportAttributesPtr
	volumeIdAttrs := volumeAttrs.VolumeIdAttributesPtr
	volumeSecurityAttrs := volumeAttrs.VolumeSecurityAttributesPtr
	volumeSecurityUnixAttrs := volumeSecurityAttrs.VolumeSecurityUnixAttributesPtr
	volumeSpaceAttrs := volumeAttrs.VolumeSpaceAttributesPtr
	volumeSnapshotAttrs := volumeAttrs.VolumeSnapshotAttributesPtr

	internalName := string(volumeIdAttrs.Name())
	name := internalName[len(*d.Config.StoragePrefix):]

	volumeConfig := &storage.VolumeConfig{
		Version:         config.OrchestratorAPIVersion,
		Name:            name,
		InternalName:    internalName,
		Size:            strconv.FormatInt(int64(volumeSpaceAttrs.Size()), 10),
		Protocol:        config.File,
		SnapshotPolicy:  volumeSnapshotAttrs.SnapshotPolicy(),
		ExportPolicy:    volumeExportAttrs.Policy(),
		SnapshotDir:     strconv.FormatBool(volumeSnapshotAttrs.SnapdirAccessEnabled()),
		UnixPermissions: volumeSecurityUnixAttrs.Permissions(),
		StorageClass:    "",
		AccessMode:      config.ReadWriteMany,
		AccessInfo:      storage.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
	}

	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   volumeIdAttrs.ContainingAggregateName(),
	}
}
