// Copyright 2016 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"fmt"
	"strconv"

	log "github.com/Sirupsen/logrus"
	"github.com/netapp/netappdvp/apis/ontap"
	"github.com/netapp/netappdvp/azgo"
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
		sa.Clones:           sa.NewBoolOffer(false),
		sa.Encryption:       sa.NewBoolOffer(d.API.SupportsApiFeature(ontap.NETAPP_VOLUME_ENCRYPTION)),
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
	return createPrepareCommon(d, volConfig)
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

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *OntapNASQtreeStorageDriver) GetVolumeExternal(name string) (*storage.VolumeExternal, error) {

	qtree, err := d.API.QtreeGet(name, d.FlexvolNamePrefix())
	if err != nil {
		return nil, err
	}

	volume, err := d.API.VolumeGet(qtree.Volume())
	if err != nil {
		return nil, err
	}

	quotaTarget := fmt.Sprintf("/vol/%s/%s", qtree.Volume(), qtree.Qtree())
	quota, err := d.API.QuotaEntryGet(quotaTarget)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(&qtree, &volume, &quota), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *OntapNASQtreeStorageDriver) GetVolumeExternalWrappers(
	channel chan *storage.VolumeExternalWrapper) {

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes matching the storage prefix
	volumesResponse, err := d.API.VolumeGetAll(d.FlexvolNamePrefix())
	if err = ontap.GetError(volumesResponse, err); err != nil {
		channel <- &storage.VolumeExternalWrapper{nil, err}
		return
	}

	// Make a map of volumes for faster correlation with qtrees
	volumeMap := make(map[string]azgo.VolumeAttributesType)
	for _, volumeAttrs := range volumesResponse.Result.AttributesList() {
		internalName := string(volumeAttrs.VolumeIdAttributesPtr.Name())
		volumeMap[internalName] = volumeAttrs
	}

	// Get all quotas in all Flexvols matching the storage prefix
	quotasResponse, err := d.API.QuotaEntryList(d.FlexvolNamePrefix() + "*")
	if err = ontap.GetError(quotasResponse, err); err != nil {
		channel <- &storage.VolumeExternalWrapper{nil, err}
		return
	}

	// Make a map of quotas for faster correlation with qtrees
	quotaMap := make(map[string]azgo.QuotaEntryType)
	for _, quotaAttrs := range quotasResponse.Result.AttributesList() {
		quotaMap[quotaAttrs.QuotaTarget()] = quotaAttrs
	}

	// Get all qtrees in all Flexvols matching the storage prefix
	qtreesResponse, err := d.API.QtreeGetAll(d.FlexvolNamePrefix())
	if err = ontap.GetError(qtreesResponse, err); err != nil {
		channel <- &storage.VolumeExternalWrapper{nil, err}
		return
	}

	// Convert all qtrees to VolumeExternal and write them to the channel
	for _, qtree := range qtreesResponse.Result.AttributesList() {

		// Ignore Flexvol-level qtrees
		if qtree.Qtree() == "" {
			continue
		}

		volume, ok := volumeMap[qtree.Volume()]
		if !ok {
			log.WithField("qtree", qtree.Qtree()).Warning("Flexvol not found for qtree.")
			continue
		}

		quotaTarget := fmt.Sprintf("/vol/%s/%s", qtree.Volume(), qtree.Qtree())
		quota, ok := quotaMap[quotaTarget]
		if !ok {
			log.WithField("qtree", qtree.Qtree()).Warning("Quota rule not found for qtree.")
			continue
		}

		channel <- &storage.VolumeExternalWrapper{d.getVolumeExternal(&qtree, &volume, &quota), nil}
	}
}

// getExternalVolume is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *OntapNASQtreeStorageDriver) getVolumeExternal(
	qtreeAttrs *azgo.QtreeInfoType, volumeAttrs *azgo.VolumeAttributesType,
	quotaAttrs *azgo.QuotaEntryType) *storage.VolumeExternal {

	volumeIdAttrs := volumeAttrs.VolumeIdAttributesPtr
	volumeSnapshotAttrs := volumeAttrs.VolumeSnapshotAttributesPtr

	internalName := qtreeAttrs.Qtree()
	name := internalName[len(*d.Config.StoragePrefix):]

	size, err := strconv.ParseInt(quotaAttrs.DiskLimit(), 10, 64)
	if err != nil {
		size = 0
	} else {
		size *= 1024 // convert KB to bytes
	}

	volumeConfig := &storage.VolumeConfig{
		Version:         config.OrchestratorAPIVersion,
		Name:            name,
		InternalName:    internalName,
		Size:            strconv.FormatInt(size, 10),
		Protocol:        config.File,
		SnapshotPolicy:  volumeSnapshotAttrs.SnapshotPolicy(),
		ExportPolicy:    qtreeAttrs.ExportPolicy(),
		SnapshotDir:     strconv.FormatBool(volumeSnapshotAttrs.SnapdirAccessEnabled()),
		UnixPermissions: qtreeAttrs.Mode(),
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
