// Copyright 2016 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"strconv"

	dvp "github.com/netapp/netappdvp/storage_drivers"

	"github.com/netapp/netappdvp/apis/ontap"
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

func (d *OntapNASStorageDriver) GetExternalVolume(name string) (*storage.VolumeExternal, error) {

	internalName := d.GetInternalVolumeName(name)
	volumeAttributes, err := d.API.VolumeGet(internalName)
	if err != nil {
		return nil, err
	}
	volumeExportAttrs := volumeAttributes.VolumeExportAttributes()
	volumeIdAttrs := volumeAttributes.VolumeIdAttributes()
	volumeSecurityAttrs := volumeAttributes.VolumeSecurityAttributes()
	volumeSecurityUnixAttributes := volumeSecurityAttrs.VolumeSecurityUnixAttributes()
	volumeSpaceAttrs := volumeAttributes.VolumeSpaceAttributes()
	volumeSnapshotAttrs := volumeAttributes.VolumeSnapshotAttributes()

	volumeConfig := &storage.VolumeConfig{
		Version:         "1",
		Name:            name,
		InternalName:    internalName,
		Size:            strconv.FormatInt(int64(volumeSpaceAttrs.Size()), 10),
		Protocol:        config.File,
		SnapshotPolicy:  volumeSnapshotAttrs.SnapshotPolicy(),
		ExportPolicy:    volumeExportAttrs.Policy(),
		SnapshotDir:     strconv.FormatBool(volumeSnapshotAttrs.SnapdirAccessEnabled()),
		UnixPermissions: volumeSecurityUnixAttributes.Permissions(),
		StorageClass:    "",
		AccessMode:      config.ReadWriteMany,
		AccessInfo:      storage.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "NFS",
	}

	volume := &storage.VolumeExternal{
		Config:  volumeConfig,
		Backend: d.Name(),
		Pool:    volumeIdAttrs.ContainingAggregateName(),
	}

	return volume, nil
}
