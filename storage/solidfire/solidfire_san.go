// Copyright 2016 NetApp, Inc. All Rights Reserved.

package solidfire

import (
	"fmt"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/netapp/netappdvp/apis/sfapi"
	dvp "github.com/netapp/netappdvp/storage_drivers"
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
)

const (
	sfDefaultVolTypeName = "SolidFire-default"
	sfDefaultMinIOPS     = 1000
	sfDefaultMaxIOPS     = 10000
)

// SolidfireSANStorageDriver is for iSCSI storage provisioning
type SolidfireSANStorageDriver struct {
	dvp.SolidfireSANStorageDriver
}

type SolidfireStorageDriverConfigExternal struct {
	*storage.CommonStorageDriverConfigExternal
	TenantName     string
	EndPoint       string
	SVIP           string
	InitiatorIFace string //iface to use of iSCSI initiator
	Types          *[]sfapi.VolType
	AccessGroups   []int64
	UseCHAP        bool
}

// Retrieve storage backend capabilities
func (d *SolidfireSANStorageDriver) GetStorageBackendSpecs(
	backend *storage.StorageBackend,
) error {
	backend.Name = "solidfire_" + strings.Split(d.Config.SVIP, ":")[0]

	volTypes := *d.Client.VolumeTypes
	if len(volTypes) == 0 {
		volTypes = []sfapi.VolType{
			sfapi.VolType{
				Type: sfDefaultVolTypeName,
				QOS: sfapi.QoS{
					MinIOPS: sfDefaultMinIOPS,
					MaxIOPS: sfDefaultMaxIOPS,
					// Leave burst IOPS blank, since we don't do anything with
					// it for storage classes.
				},
			},
		}
	}
	for _, volType := range volTypes {
		pool := storage.NewStoragePool(backend, volType.Type)

		pool.Attributes[sa.Media] = sa.NewStringOffer(sa.SSD)
		pool.Attributes[sa.IOPS] = sa.NewIntOffer(int(volType.QOS.MinIOPS),
			int(volType.QOS.MaxIOPS))
		pool.Attributes[sa.Snapshots] = sa.NewBoolOffer(true)
		pool.Attributes[sa.Clones] = sa.NewBoolOffer(true)
		pool.Attributes[sa.Encryption] = sa.NewBoolOffer(false)
		pool.Attributes[sa.ProvisioningType] = sa.NewStringOffer("thin")
		pool.Attributes[sa.BackendType] = sa.NewStringOffer(d.Name())
		backend.AddStoragePool(pool)

		log.WithFields(log.Fields{
			"attributes": fmt.Sprintf("%+v", pool.Attributes),
			"pool":       pool.Name,
			"backend":    backend.Name,
		}).Debug("Added pool for SolidFire backend.")
	}

	return nil
}

func (d *SolidfireSANStorageDriver) GetInternalVolumeName(name string) string {

	if config.UsingPassthroughStore {
		return strings.Replace(name, "_", "-", -1)
	} else {
		internal := storage.GetCommonInternalVolumeName(d.Config.CommonStorageDriverConfig, name)
		internal = strings.Replace(internal, "_", "-", -1)
		internal = strings.Replace(internal, ".", "-", -1)
		internal = strings.Replace(internal, "--", "-", -1) // Remove any double hyphens
		return internal
	}
}

func (d *SolidfireSANStorageDriver) CreatePrepare(
	volConfig *storage.VolumeConfig,
) bool {
	// 1. Sanitize the volume name
	volConfig.InternalName = d.GetInternalVolumeName(volConfig.Name)

	if volConfig.CloneSourceVolume != "" {
		volConfig.CloneSourceVolumeInternal =
			d.GetInternalVolumeName(volConfig.CloneSourceVolume)
	}

	return true
}

func (d *SolidfireSANStorageDriver) CreateFollowup(volConfig *storage.VolumeConfig) error {
	return d.mapSolidfireLun(volConfig)
}

func (d *SolidfireSANStorageDriver) mapSolidfireLun(volConfig *storage.VolumeConfig) error {
	// Add the newly created volume to the default VAG
	name := volConfig.InternalName
	v, err := d.GetVolume(name)
	if err != nil {
		return fmt.Errorf("Could not find SolidFire volume %s: %s", name, err.Error())
	}

	// start volConfig...
	volConfig.AccessInfo.IscsiTargetPortal = d.Config.SVIP
	volConfig.AccessInfo.IscsiTargetIQN = v.Iqn
	volConfig.AccessInfo.IscsiLunNumber = 0
	volConfig.AccessInfo.IscsiInterface = d.Config.InitiatorIFace

	if d.Config.UseCHAP {
		var req sfapi.GetAccountByIDRequest
		req.AccountID = v.AccountID
		a, err := d.Client.GetAccountByID(&req)
		if err != nil {
			return fmt.Errorf("Could not lookup SolidFire account ID %v, error: %+v ", v.AccountID, err)
		}

		// finish volConfig
		volConfig.AccessInfo.IscsiUsername = a.Username
		volConfig.AccessInfo.IscsiInitiatorSecret = a.InitiatorSecret
		volConfig.AccessInfo.IscsiTargetSecret = a.TargetSecret
	} else {

		volumeIDList := []int64{v.VolumeID}
		for _, vagID := range d.Config.AccessGroups {
			req := sfapi.AddVolumesToVolumeAccessGroupRequest{
				VolumeAccessGroupID: vagID,
				Volumes:             volumeIDList,
			}

			err = d.Client.AddVolumesToAccessGroup(&req)
			if err != nil {
				return fmt.Errorf("Could not map SolidFire volume %s to the VAG: %s", name, err.Error())
			}
		}

		// finish volConfig
		volConfig.AccessInfo.IscsiVAGs = d.Config.AccessGroups
	}

	log.WithFields(log.Fields{
		"volume":          volConfig.Name,
		"volume_internal": volConfig.InternalName,
		"targetIQN":       volConfig.AccessInfo.IscsiTargetIQN,
		"lunNumber":       volConfig.AccessInfo.IscsiLunNumber,
		"interface":       volConfig.AccessInfo.IscsiInterface,
		"VAGs":            volConfig.AccessInfo.IscsiVAGs,
		"Username":        volConfig.AccessInfo.IscsiUsername,
		"InitiatorSecret": volConfig.AccessInfo.IscsiInitiatorSecret,
		"TargetSecret":    volConfig.AccessInfo.IscsiTargetSecret,
		"UseCHAP":         d.Config.UseCHAP,
	}).Info("Successfully mapped SolidFire LUN.")

	return nil
}

func (d *SolidfireSANStorageDriver) GetVolumeOpts(
	volConfig *storage.VolumeConfig,
	pool *storage.StoragePool,
	requests map[string]sa.Request,
) (map[string]string, error) {

	opts := make(map[string]string)

	if volConfig.FileSystem != "" {
		opts["fileSystemType"] = volConfig.FileSystem
	}
	if volConfig.BlockSize != "" {
		opts["blocksize"] = volConfig.BlockSize
	}
	if volConfig.QoS != "" {
		opts["qos"] = volConfig.QoS
	}

	if volConfig.QoSType != "" {
		opts["type"] = volConfig.QoSType
	} else if volConfig.QoS != "" {
		opts["type"] = ""
	} else {
		// Default to "pool" name only if we aren't cloning
		if volConfig.CloneSourceVolume != "" {
			opts["type"] = ""
		} else {
			opts["type"] = pool.Name
		}
	}

	return opts, nil
}

func (d *SolidfireSANStorageDriver) GetProtocol() config.Protocol {
	return config.Block
}

func (d *SolidfireSANStorageDriver) GetDriverName() string {
	return d.Config.StorageDriverName
}

func (d *SolidfireSANStorageDriver) StoreConfig(
	b *storage.PersistentStorageBackendConfig,
) {
	storage.SanitizeCommonStorageDriverConfig(
		d.Config.CommonStorageDriverConfig)
	b.SolidfireConfig = &d.Config
}

func (d *SolidfireSANStorageDriver) GetExternalConfig() interface{} {
	endpointHalves := strings.Split(d.Config.EndPoint, "@")
	return &SolidfireStorageDriverConfigExternal{
		CommonStorageDriverConfigExternal: storage.GetCommonStorageDriverConfigExternal(
			d.Config.CommonStorageDriverConfig),
		TenantName:     d.Config.TenantName,
		EndPoint:       fmt.Sprintf("https://%s", endpointHalves[1]),
		SVIP:           d.Config.SVIP,
		InitiatorIFace: d.Config.InitiatorIFace,
		Types:          d.Config.Types,
		AccessGroups:   d.Config.AccessGroups,
		UseCHAP:        d.Config.UseCHAP,
	}
}

// Find/Return items that exist in b but NOT a
func diffSlices(a, b []int64) []int64 {
	var r []int64
	m := make(map[int64]bool)
	for _, s := range a {
		m[s] = true
	}
	for _, s := range b {
		if !m[s] {
			r = append(r, s)
		}
	}
	return r
}

// Verify that the provided list of VAG ID's exist, return list of those that don't
func (d *SolidfireSANStorageDriver) VerifyVags(vags []int64) []int64 {
	var vagIDs []int64

	discovered, err := d.Client.ListVolumeAccessGroups(&sfapi.ListVolumeAccessGroupsRequest{})
	if err != nil {
		log.Fatalf("Failed to retrieve VAGs from SolidFire backend: %+v", err)
	}

	for _, v := range discovered {
		vagIDs = append(vagIDs, v.VAGID)
	}
	return diffSlices(vagIDs, vags)
}

// Add volume ID's in the provided list that aren't already a member of the specified VAG
func (d *SolidfireSANStorageDriver) AddMissingVolumesToVag(vagID int64, vols []int64) error {
	var req sfapi.ListVolumeAccessGroupsRequest
	req.StartVAGID = vagID
	req.Limit = 1

	vags, _ := d.Client.ListVolumeAccessGroups(&req)
	missingVolIDs := diffSlices(vags[0].Volumes, vols)

	var addReq sfapi.AddVolumesToVolumeAccessGroupRequest
	addReq.VolumeAccessGroupID = vagID
	addReq.Volumes = missingVolIDs

	return d.Client.AddVolumesToAccessGroup(&addReq)
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *SolidfireSANStorageDriver) GetVolumeExternal(name string) (*storage.VolumeExternal, error) {

	volume, err := d.GetVolume(name)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(name, &volume), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *SolidfireSANStorageDriver) GetVolumeExternalWrappers(
	channel chan *storage.VolumeExternalWrapper) {

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes
	volumes, err := d.GetVolumes()
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{nil, err}
		return
	}

	// Convert all volumes to VolumeExternal and write them to the channel
	for externalName, volume := range volumes {
		channel <- &storage.VolumeExternalWrapper{d.getVolumeExternal(externalName, &volume), nil}
	}
}

// getExternalVolume is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *SolidfireSANStorageDriver) getVolumeExternal(
	externalName string, volumeAttrs *sfapi.Volume) *storage.VolumeExternal {

	volumeConfig := &storage.VolumeConfig{
		Version:         config.OrchestratorAPIVersion,
		Name:            externalName,
		InternalName:    string(volumeAttrs.Name),
		Size:            strconv.FormatInt(volumeAttrs.TotalSize, 10),
		Protocol:        config.Block,
		SnapshotPolicy:  "",
		ExportPolicy:    "",
		SnapshotDir:     "false",
		UnixPermissions: "",
		StorageClass:    "",
		AccessMode:      config.ReadWriteOnce,
		AccessInfo:      storage.VolumeAccessInfo{},
		BlockSize:       strconv.FormatInt(volumeAttrs.BlockSize, 10),
		FileSystem:      "",
	}

	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   d.getPoolNameFromQoS(volumeAttrs),
	}
}

// getPoolNameFromQoS attempts to map the min/max QoS levels for a volume
// back to the levels defined in the backend config file.  If a match is not
// found, a partial match is sought; failing that, one of the existing "pools"
// is chosen.
func (d *SolidfireSANStorageDriver) getPoolNameFromQoS(volumeAttrs *sfapi.Volume) string {

	volTypes := *d.Client.VolumeTypes
	if len(volTypes) == 0 {
		return sfDefaultVolTypeName
	}

	// Try min/max matches
	for _, volType := range volTypes {
		if volType.QOS.MaxIOPS == volumeAttrs.Qos.MaxIOPS &&
			volType.QOS.MinIOPS == volumeAttrs.Qos.MinIOPS {
			return volType.Type
		}
	}

	// Try min matches
	for _, volType := range volTypes {
		if volType.QOS.MinIOPS == volumeAttrs.Qos.MinIOPS {
			return volType.Type
		}
	}

	// Try max matches
	for _, volType := range volTypes {
		if volType.QOS.MaxIOPS == volumeAttrs.Qos.MaxIOPS {
			return volType.Type
		}
	}

	// Nothing matched, so just return something.
	log.WithFields(log.Fields{
		"volume": volumeAttrs.Name,
		"pool":   volTypes[0].Type,
	}).Debug("Matching QoS type not found, reporting first type for volume.")
	return volTypes[0].Type
}
