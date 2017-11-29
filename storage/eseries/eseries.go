// Copyright 2017 NetApp, Inc. All Rights Reserved.

package eseries

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/netapp/netappdvp/apis/eseries"
	dvp "github.com/netapp/netappdvp/storage_drivers"
	"github.com/pborman/uuid"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
)

// EseriesStorageDriver is for iSCSI storage provisioning on E-series
type EseriesStorageDriver struct {
	dvp.ESeriesStorageDriver
}

type EseriesStorageDriverConfigExternal struct {
	*storage.CommonStorageDriverConfigExternal
	Username    string
	ControllerA string
	ControllerB string
	HostDataIP  string
}

// Retrieve storage capabilities and register pools with specified backend.
func (d *EseriesStorageDriver) GetStorageBackendSpecs(backend *storage.StorageBackend) error {

	backend.Name = "eseries_" + d.Config.HostDataIP

	// Get pools
	pools, err := d.API.GetVolumePools("", 0, "")
	if err != nil {
		return fmt.Errorf("Could not get storage pools from array. %v", err)
	}

	for _, pool := range pools {

		vc := storage.NewStoragePool(backend, pool.Label)
		vc.Attributes[sa.BackendType] = sa.NewStringOffer(d.Name())

		// E-series supports both "hdd" and "ssd" media types
		switch pool.DriveMediaType {
		case "hdd":
			vc.Attributes[sa.Media] = sa.NewStringOffer(sa.HDD)
		case "ssd":
			vc.Attributes[sa.Media] = sa.NewStringOffer(sa.SSD)
		}

		// No snapshots, clones or thin provisioning on E-series
		vc.Attributes[sa.Snapshots] = sa.NewBoolOffer(false)
		vc.Attributes[sa.Clones] = sa.NewBoolOffer(false)
		vc.Attributes[sa.Encryption] = sa.NewBoolOffer(false)
		vc.Attributes[sa.ProvisioningType] = sa.NewStringOffer("thick")

		backend.AddStoragePool(vc)

		log.WithFields(log.Fields{
			"attributes": fmt.Sprintf("%+v", vc.Attributes),
			"pool":       pool.Label,
			"backend":    backend.Name,
		}).Debug("EseriesStorageDriver#GetStorageBackendSpecs : Added pool for E-series backend.")
	}

	return nil
}

func (d *EseriesStorageDriver) CreatePrepare(volConfig *storage.VolumeConfig) bool {

	// 1. Sanitize the volume name
	volConfig.InternalName = d.GetInternalVolumeName(volConfig.Name)

	// 2. Ensure no volume with the same name exists on that backend (unnecessary since
	// Step 1 always generates a new UUID-based name)

	return true
}

func (d *EseriesStorageDriver) GetInternalVolumeName(name string) string {

	if config.UsingPassthroughStore {
		// With a passthrough store, the name mapping must remain reversible
		return *d.Config.StoragePrefix + name
	} else {
		// E-series has a 30-character limitation on volume names, so no combination
		// of the usual namespace, PVC name, and PVC UID characters is likely to
		// fit, nor is some Base64 encoding of the same. And unfortunately, the PVC
		// UID is not persisted past the highest levels of Trident. So we borrow a
		// page from the E-series OpenStack driver and return a Base64-encoded form
		// of a new random (version 4) UUID.
		uuid4string := uuid.New()
		b64string, err := d.uuidToBase64(uuid4string)
		if err != nil {
			// This is unlikely, but if the UUID encoding fails, just return the original string (capped to 30 chars)
			if len(name) > 30 {
				return name[0:30]
			}
			return name
		}

		log.WithFields(log.Fields{
			"Name":   name,
			"UUID":   uuid4string,
			"Base64": b64string,
		}).Debug("EseriesStorageDriver#GetInternalVolumeName : Created Base64 UUID for E-series volume name.")

		return b64string
	}
}

func (d *EseriesStorageDriver) GetVolumeOpts(
	volConfig *storage.VolumeConfig,
	pool *storage.StoragePool,
	requests map[string]sa.Request,
) (map[string]string, error) {

	opts := make(map[string]string)

	// Include the pool so that Trident's pool selection is honored by nDVP
	opts["pool"] = pool.Name

	// Include mediaType request if present
	if mediaTypeReq, ok := requests[sa.Media]; ok {
		if mediaType, ok := mediaTypeReq.Value().(string); ok {
			if mediaType == sa.HDD {
				opts["mediaType"] = "hdd"
			} else if mediaType == sa.SSD {
				opts["mediaType"] = "ssd"
			} else {
				log.WithFields(log.Fields{
					"provisioner":      "E-series",
					"method":           "GetVolumeOpts",
					"provisioningType": mediaTypeReq.Value(),
				}).Warnf("Expected 'ssd' or 'hdd' for %s; ignoring.", sa.Media)
			}
		} else {
			log.WithFields(log.Fields{
				"provisioner":      "E-series",
				"method":           "GetVolumeOpts",
				"provisioningType": mediaTypeReq.Value(),
			}).Warnf("Expected string for %s; ignoring.", sa.Media)
		}
	}

	if volConfig.FileSystem != "" {
		opts["fileSystemType"] = volConfig.FileSystem
	}

	log.WithFields(log.Fields{
		"volConfig": volConfig,
		"pool":      pool,
		"requests":  requests,
		"opts":      opts,
	}).Debug("EseriesStorageDriver#GetVolumeOpts")

	return opts, nil
}

func (d *EseriesStorageDriver) CreateFollowup(volConfig *storage.VolumeConfig) error {

	// Get the volume
	name := volConfig.InternalName
	volume, err := d.API.GetVolume(name)
	if err != nil {
		return fmt.Errorf("Could not find volume %s. %v", name, err)
	}
	if !d.API.IsRefValid(volume.VolumeRef) {
		return fmt.Errorf("Could not find volume %s.", name)
	}

	// Get the Target IQN
	targetIQN, err := d.API.GetTargetIQN()
	if err != nil {
		return fmt.Errorf("Could not get target IQN from array. %v", err)
	}

	// Get the Trident Host Group
	hostGroup, err := d.API.GetHostGroup(d.Config.AccessGroup)
	if err != nil {
		return fmt.Errorf("Could not get Host Group %s from array. %v", d.Config.AccessGroup, err)
	}

	// Map the volume directly to the Host Group
	host := eseries.HostEx{
		HostRef:    eseries.NULL_REF,
		ClusterRef: hostGroup.ClusterRef,
	}
	mapping, err := d.API.MapVolume(volume, host)
	if err != nil {
		return fmt.Errorf("Could not map volume %s to Host Group %s. %v", name, hostGroup.Label, err)
	}

	volConfig.AccessInfo.IscsiTargetPortal = d.Config.HostDataIP
	volConfig.AccessInfo.IscsiTargetIQN = targetIQN
	volConfig.AccessInfo.IscsiLunNumber = int32(mapping.LunNumber)

	log.WithFields(log.Fields{
		"volume":          volConfig.Name,
		"volume_internal": volConfig.InternalName,
		"targetIQN":       volConfig.AccessInfo.IscsiTargetIQN,
		"lunNumber":       volConfig.AccessInfo.IscsiLunNumber,
		"hostGroup":       hostGroup.Label,
	}).Debug("EseriesStorageDriver#CreateFollowup : Successfully mapped E-series LUN.")

	return nil
}

func (d *EseriesStorageDriver) GetProtocol() config.Protocol {
	return config.Block
}

func (d *EseriesStorageDriver) GetDriverName() string {
	return d.Config.StorageDriverName
}

func (d *EseriesStorageDriver) StoreConfig(b *storage.PersistentStorageBackendConfig) {
	log.Debugln("EseriesStorageDriver:StoreConfig")

	storage.SanitizeCommonStorageDriverConfig(
		d.Config.CommonStorageDriverConfig)
	b.EseriesConfig = &d.Config
}

func (d *EseriesStorageDriver) GetExternalConfig() interface{} {
	log.Debugln("EseriesStorageDriver:GetExternalConfig")

	return &EseriesStorageDriverConfigExternal{
		CommonStorageDriverConfigExternal: storage.GetCommonStorageDriverConfigExternal(
			d.Config.CommonStorageDriverConfig),
		Username:    d.Config.Username,
		ControllerA: d.Config.ControllerA,
		ControllerB: d.Config.ControllerB,
		HostDataIP:  d.Config.HostDataIP,
	}
}

func (d *EseriesStorageDriver) uuidToBase64(UUID string) (string, error) {

	// Strip out hyphens
	UUID = strings.Replace(UUID, "-", "", -1)

	// Convert hex chars to binary
	var bytes [16]byte
	_, err := hex.Decode(bytes[:], []byte(UUID))
	if err != nil {
		return "", err
	}

	// Convert binary to Base64
	encoded := base64.RawURLEncoding.EncodeToString(bytes[:])

	return encoded, nil
}

func (d *EseriesStorageDriver) base64ToUuid(b64 string) (string, error) {

	// Convert Base64 to binary
	decoded, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("Error decoding Base64 string %s", b64)
	}

	// Convert binary to hex chars
	UUID := hex.EncodeToString(decoded[:])

	// Add hyphens
	UUID = strings.Join([]string{UUID[:8], UUID[8:12], UUID[12:16], UUID[16:20], UUID[20:]}, "-")

	return UUID, nil
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *EseriesStorageDriver) GetVolumeExternal(name string) (*storage.VolumeExternal, error) {

	volumeAttrs, err := d.API.GetVolume(name)
	if err != nil {
		return nil, err
	}
	if volumeAttrs.Label == "" {
		return nil, fmt.Errorf("Volume %s not found.", name)
	}

	poolAttrs, err := d.API.GetVolumePoolByRef(volumeAttrs.VolumeGroupRef)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(&volumeAttrs, &poolAttrs), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *EseriesStorageDriver) GetVolumeExternalWrappers(
	channel chan *storage.VolumeExternalWrapper) {

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes
	volumes, err := d.API.GetVolumes()
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{nil, err}
		return
	}

	// Get all pools
	pools, err := d.API.GetVolumePools("", 0, "")
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{nil, err}
		return
	}

	// Make a map of pools for faster correlation with volumes
	poolMap := make(map[string]eseries.VolumeGroupEx)
	for _, pool := range pools {
		poolMap[pool.VolumeGroupRef] = pool
	}

	prefix := *d.Config.StoragePrefix
	repos_regex, _ := regexp.Compile("^repos_\\d{4}$")

	// Convert all volumes to VolumeExternal and write them to the channel
	for _, volume := range volumes {

		// Filter out internal volumes
		if repos_regex.MatchString(volume.Label) {
			continue
		}

		// Filter out volumes without the prefix (pass all if prefix is empty)
		if !strings.HasPrefix(volume.Label, prefix) {
			continue
		}

		pool, ok := poolMap[volume.VolumeGroupRef]
		if !ok {
			log.WithField("volume", volume.Label).Warning("Pool not found for volume.")
			continue
		}

		channel <- &storage.VolumeExternalWrapper{d.getVolumeExternal(&volume, &pool), nil}
	}
}

// getExternalVolume is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *EseriesStorageDriver) getVolumeExternal(
	volumeAttrs *eseries.VolumeEx, poolAttrs *eseries.VolumeGroupEx) *storage.VolumeExternal {

	internalName := volumeAttrs.Label
	name := internalName[len(*d.Config.StoragePrefix):]

	volumeConfig := &storage.VolumeConfig{
		Version:         config.OrchestratorAPIVersion,
		Name:            name,
		InternalName:    internalName,
		Size:            volumeAttrs.VolumeSize,
		Protocol:        config.Block,
		SnapshotPolicy:  "",
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
		Pool:   poolAttrs.Label,
	}
}
