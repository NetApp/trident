// Copyright 2016 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"fmt"
	"os/exec"
	"strconv"

	log "github.com/Sirupsen/logrus"
	dvp "github.com/netapp/netappdvp/storage_drivers"

	"github.com/netapp/netappdvp/apis/ontap"
	"github.com/netapp/netappdvp/azgo"
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
)

// OntapSANStorageDriver is for iSCSI storage provisioning
type OntapSANStorageDriver struct {
	dvp.OntapSANStorageDriver
}

// Retrieve storage backend capabilities
func (d *OntapSANStorageDriver) GetStorageBackendSpecs(backend *storage.StorageBackend) error {

	backend.Name = "ontapsan_" + d.Config.DataLIF
	poolAttrs := d.GetStoragePoolAttributes()
	return getStorageBackendSpecsCommon(d, backend, poolAttrs)
}

func (d *OntapSANStorageDriver) GetStoragePoolAttributes() map[string]sa.Offer {

	return map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(d.Name()),
		sa.Snapshots:        sa.NewBoolOffer(true),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.Encryption:       sa.NewBoolOffer(d.API.SupportsApiFeature(ontap.NETAPP_VOLUME_ENCRYPTION)),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}
}

func (d *OntapSANStorageDriver) GetVolumeOpts(
	volConfig *storage.VolumeConfig,
	vc *storage.StoragePool,
	requests map[string]sa.Request,
) (map[string]string, error) {
	return getVolumeOptsCommon(volConfig, vc, requests), nil
}

func (d *OntapSANStorageDriver) GetInternalVolumeName(name string) string {
	return getInternalVolumeNameCommon(d.Config.CommonStorageDriverConfig, name)
}

func (d *OntapSANStorageDriver) CreatePrepare(volConfig *storage.VolumeConfig) bool {
	return createPrepareCommon(d, volConfig)
}

func (d *OntapSANStorageDriver) CreateFollowup(volConfig *storage.VolumeConfig) error {
	return d.mapOntapSANLun(volConfig)
}

func (d *OntapSANStorageDriver) mapOntapSANLun(volConfig *storage.VolumeConfig) error {
	var (
		targetIQN string
		lunID     int
	)

	response, err := d.API.IscsiServiceGetIterRequest()
	if response.Result.ResultStatusAttr != "passed" || err != nil {
		return fmt.Errorf("Problem retrieving iSCSI services: %v, %v",
			err, response.Result.ResultErrnoAttr)
	}
	for _, serviceInfo := range response.Result.AttributesList() {
		if serviceInfo.Vserver() == d.Config.SVM {
			targetIQN = serviceInfo.NodeName()
			log.WithFields(log.Fields{
				"volume":    volConfig.Name,
				"targetIQN": targetIQN,
			}).Debug("Successfully discovered target IQN for the volume.")
			break
		}
	}

	// Map LUN
	lunPath := fmt.Sprintf("/vol/%v/lun0", volConfig.InternalName)
	lunID, err = d.API.LunMapIfNotMapped(d.Config.IgroupName, lunPath)
	if err != nil {
		return err
	}

	volConfig.AccessInfo.IscsiTargetPortal = d.Config.DataLIF
	volConfig.AccessInfo.IscsiTargetIQN = targetIQN
	volConfig.AccessInfo.IscsiLunNumber = int32(lunID)
	volConfig.AccessInfo.IscsiIgroup = d.Config.IgroupName
	log.WithFields(log.Fields{
		"volume":          volConfig.Name,
		"volume_internal": volConfig.InternalName,
		"targetIQN":       volConfig.AccessInfo.IscsiTargetIQN,
		"lunNumber":       volConfig.AccessInfo.IscsiLunNumber,
		"igroup":          volConfig.AccessInfo.IscsiIgroup,
	}).Debug("Successfully mapped ONTAP LUN.")

	return nil
}

func (d *OntapSANStorageDriver) GetProtocol() config.Protocol {
	return config.Block
}

func (d *OntapSANStorageDriver) GetDriverName() string {
	return d.Config.StorageDriverName
}

func (d *OntapSANStorageDriver) StoreConfig(
	b *storage.PersistentStorageBackendConfig,
) {
	storage.SanitizeCommonStorageDriverConfig(
		d.Config.CommonStorageDriverConfig)
	b.OntapConfig = &d.Config
}

func (d *OntapSANStorageDriver) GetExternalConfig() interface{} {
	return getExternalConfig(d.Config)
}

func DiscoverIscsiTarget(targetIP string) error {
	cmd := exec.Command("sudo", "iscsiadm", "-m", "discoverydb", "-t", "st", "-p", targetIP, "--discover")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	} else {
		log.WithFields(log.Fields{
			"target":    targetIP,
			"targetIQN": string(out[:]),
		}).Info("Successful iSCSI target discovery.")
	}
	cmd = exec.Command("sudo", "iscsiadm", "-m", "node", "-p", targetIP, "--login")
	out, err = cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *OntapSANStorageDriver) GetVolumeExternal(name string) (*storage.VolumeExternal, error) {

	volumeAttrs, err := d.API.VolumeGet(name)
	if err != nil {
		return nil, err
	}

	lunPath := fmt.Sprintf("/vol/%v/lun0", name)
	lunAttrs, err := d.API.LunGet(lunPath)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(&lunAttrs, &volumeAttrs), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *OntapSANStorageDriver) GetVolumeExternalWrappers(
	channel chan *storage.VolumeExternalWrapper) {

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes matching the storage prefix
	volumesResponse, err := d.API.VolumeGetAll(*d.Config.StoragePrefix)
	if err = ontap.GetError(volumesResponse, err); err != nil {
		channel <- &storage.VolumeExternalWrapper{nil, err}
		return
	}

	// Get all LUNs named 'lun0' in volumes matching the storage prefix
	lunPathPattern := fmt.Sprintf("/vol/%v/lun0", *d.Config.StoragePrefix+"*")
	lunsResponse, err := d.API.LunGetAll(lunPathPattern)
	if err = ontap.GetError(lunsResponse, err); err != nil {
		channel <- &storage.VolumeExternalWrapper{nil, err}
		return
	}

	// Make a map of volumes for faster correlation with LUNs
	volumeMap := make(map[string]azgo.VolumeAttributesType)
	for _, volumeAttrs := range volumesResponse.Result.AttributesList() {
		internalName := string(volumeAttrs.VolumeIdAttributesPtr.Name())
		volumeMap[internalName] = volumeAttrs
	}

	// Convert all LUNs to VolumeExternal and write them to the channel
	for _, lun := range lunsResponse.Result.AttributesList() {

		volume, ok := volumeMap[lun.Volume()]
		if !ok {
			log.WithField("path", lun.Path()).Warning("Flexvol not found for LUN.")
			continue
		}

		channel <- &storage.VolumeExternalWrapper{d.getVolumeExternal(&lun, &volume), nil}
	}
}

// getExternalVolume is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *OntapSANStorageDriver) getVolumeExternal(
	lunAttrs *azgo.LunInfoType, volumeAttrs *azgo.VolumeAttributesType,
) *storage.VolumeExternal {

	volumeIdAttrs := volumeAttrs.VolumeIdAttributesPtr
	volumeSnapshotAttrs := volumeAttrs.VolumeSnapshotAttributesPtr

	internalName := string(volumeIdAttrs.Name())
	name := internalName[len(*d.Config.StoragePrefix):]

	volumeConfig := &storage.VolumeConfig{
		Version:         config.OrchestratorAPIVersion,
		Name:            name,
		InternalName:    internalName,
		Size:            strconv.FormatInt(int64(lunAttrs.Size()), 10),
		Protocol:        config.Block,
		SnapshotPolicy:  volumeSnapshotAttrs.SnapshotPolicy(),
		ExportPolicy:    "",
		SnapshotDir:     "false",
		UnixPermissions: "",
		StorageClass:    "",
		AccessMode:      config.ReadWriteOnce,
		AccessInfo:      storage.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
	}

	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   volumeIdAttrs.ContainingAggregateName(),
	}
}
