// Copyright 2016 NetApp, Inc. All Rights Reserved.

package ontap

import (
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

func (d *OntapNASStorageDriver) CreatePrepare(
	volConfig *storage.VolumeConfig,
) bool {
	// Sanitize the volume name
	volConfig.InternalName = d.GetInternalVolumeName(volConfig.Name)

	// Because the storage prefix specified in the backend config must create
	// a unique set of volume names, we do not need to check whether volumes
	// exist in the backend here.
	return true
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
