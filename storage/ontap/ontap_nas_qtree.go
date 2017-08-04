// Copyright 2016 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"fmt"

	dvp "github.com/netapp/netappdvp/storage_drivers"
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
)

// OntapNASQtreeStorageDriver is for NFS storage provisioning
type OntapNASQtreeStorageDriver struct {
	dvp.OntapNASQtreeStorageDriver
}

// Retrieve storage backend capabilities
func (d *OntapNASQtreeStorageDriver) GetStorageBackendSpecs(backend *storage.StorageBackend) error {

	backend.Name = "ontapnaseco_" + d.Config.DataLIF
	poolAttrs := d.GetStoragePoolAttributes()
	return getStorageBackendSpecsCommon(d, backend, poolAttrs)
}

func (d *OntapNASQtreeStorageDriver) GetStoragePoolAttributes() map[string]sa.Offer {

	return map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(d.Name()),
		sa.Snapshots:        sa.NewBoolOffer(false),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}
}

func (d *OntapNASQtreeStorageDriver) GetVolumeOpts(
	volConfig *storage.VolumeConfig,
	vc *storage.StoragePool,
	requests map[string]sa.Request,
) (map[string]string, error) {
	return getVolumeOptsCommon(volConfig, vc, requests), nil
}

func (d *OntapNASQtreeStorageDriver) GetInternalVolumeName(name string) string {
	return getInternalVolumeNameCommon(d.Config.CommonStorageDriverConfig, name)
}

func (d *OntapNASQtreeStorageDriver) CreatePrepare(volConfig *storage.VolumeConfig) bool {
	// Sanitize the volume name
	volConfig.InternalName = d.GetInternalVolumeName(volConfig.Name)

	// Because the storage prefix specified in the backend config must create
	// a unique set of volume names, we do not need to check whether volumes
	// exist in the backend here.
	return true
}

func (d *OntapNASQtreeStorageDriver) CreateFollowup(volConfig *storage.VolumeConfig) error {

	// Determine which Flexvol contains the qtree
	exists, flexvol, err := d.API.QtreeExists(volConfig.InternalName, d.OntapNASQtreeStorageDriver.FlexvolNamePrefix())
	if err != nil {
		return fmt.Errorf("Could not determine if qtree %s exists. %v", volConfig.InternalName, err)
	}
	if !exists {
		return fmt.Errorf("Could not find qtree %s", volConfig.InternalName)
	}

	// Set export path info on the volume config
	volConfig.AccessInfo.NfsServerIP = d.Config.DataLIF
	volConfig.AccessInfo.NfsPath = fmt.Sprintf("/%s/%s", flexvol, volConfig.InternalName)

	return nil
}

func (d *OntapNASQtreeStorageDriver) GetProtocol() config.Protocol {
	return config.File
}

func (d *OntapNASQtreeStorageDriver) GetDriverName() string {
	return d.Config.StorageDriverName
}

func (d *OntapNASQtreeStorageDriver) StoreConfig(b *storage.PersistentStorageBackendConfig) {
	storage.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.OntapConfig = &d.Config
}

func (d *OntapNASQtreeStorageDriver) GetExternalConfig() interface{} {
	return getExternalConfig(d.Config)
}
