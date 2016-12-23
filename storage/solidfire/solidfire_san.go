// Copyright 2016 NetApp, Inc. All Rights Reserved.

package solidfire

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/netapp/netappdvp/apis/sfapi"
	dvp "github.com/netapp/netappdvp/storage_drivers"
	"github.com/netapp/netappdvp/utils"

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
		vc := storage.NewStoragePool(backend, volType.Type)

		vc.Attributes[sa.Media] = sa.NewStringOffer(sa.SSD)
		vc.Attributes[sa.IOPS] = sa.NewIntOffer(int(volType.QOS.MinIOPS),
			int(volType.QOS.MaxIOPS))
		vc.Attributes[sa.Snapshots] = sa.NewBoolOffer(true)
		vc.Attributes[sa.ProvisioningType] = sa.NewStringOffer("thin")
		vc.Attributes[sa.BackendType] = sa.NewStringOffer(d.Name())
		backend.AddStoragePool(vc)
	}

	return nil
}

func (d *SolidfireSANStorageDriver) GetInternalVolumeName(name string) string {
	internalName := storage.GetCommonInternalVolumeName(
		&d.Config.CommonStorageDriverConfig, name)
	return strings.Replace(internalName, "_", "-", -1)
}

func (d *SolidfireSANStorageDriver) CreatePrepare(
	volConfig *storage.VolumeConfig,
) bool {
	// 1. Sanitize the volume name
	volConfig.InternalName = d.GetInternalVolumeName(volConfig.Name)
	return true
}

func (d *SolidfireSANStorageDriver) CreateFollowup(volConfig *storage.VolumeConfig) error {
	return d.mapSolidfireLun(volConfig)
}

func (d *SolidfireSANStorageDriver) mapSolidfireLun(volConfig *storage.VolumeConfig) error {
	// Add the newly created volume to the default VAG
	name := volConfig.InternalName
	v, err := d.Client.GetVolumeByName(name, d.TenantID)
	if err != nil {
		return fmt.Errorf("Could not find SolidFire volume %s: %s", name, err.Error())
	}
	volumeIDList := []int64{v.VolumeID}
	err = d.Client.AddVolumeToAccessGroup(d.VagID, volumeIDList)
	if err != nil {
		return fmt.Errorf("Could not map SolidFire volume %s to the VAG: %s", name, err.Error())
	}

	volConfig.AccessInfo.IscsiTargetPortal = d.Config.SVIP
	volConfig.AccessInfo.IscsiTargetIQN = v.Iqn
	volConfig.AccessInfo.IscsiLunNumber = 0
	volConfig.AccessInfo.IscsiInterface = d.Config.InitiatorIFace
	volConfig.AccessInfo.IscsiVAG = d.VagID
	log.WithFields(log.Fields{
		"volume":          volConfig.Name,
		"volume_internal": volConfig.InternalName,
		"targetIQN":       volConfig.AccessInfo.IscsiTargetIQN,
		"lunNumber":       volConfig.AccessInfo.IscsiLunNumber,
		"interface":       volConfig.AccessInfo.IscsiInterface,
		"VAG":             volConfig.AccessInfo.IscsiVAG,
	}).Debug("Successfully mapped SolidFire LUN.")

	return nil
}

func (d *SolidfireSANStorageDriver) GetVolumeOpts(
	volConfig *storage.VolumeConfig,
	pool *storage.StoragePool,
	requests map[string]sa.Request,
) (map[string]string, error) {
	opts := make(map[string]string)
	opts["type"] = pool.Name

	// Set the size, since the SolidFire driver takes storage in GB, rather
	// than bytes.
	requestedSize, err := utils.ConvertSizeToBytes64(volConfig.Size)
	if err != nil {
		return nil, fmt.Errorf("Couldn't convert volume size %s: %s",
			volConfig.Size, err.Error())
	}
	volSize, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%v is an invalid volume size: %s",
			volConfig.Size, err.Error())
	}
	opts["size"] = fmt.Sprintf("%d", int(
		math.Ceil(float64(volSize)/(1024*1024*1024))))
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
		&d.Config.CommonStorageDriverConfig)
	b.SolidfireConfig = &d.Config
}

func (d *SolidfireSANStorageDriver) GetExternalConfig() interface{} {
	endpointHalves := strings.Split(d.Config.EndPoint, "@")
	return &SolidfireStorageDriverConfigExternal{
		CommonStorageDriverConfigExternal: storage.GetCommonStorageDriverConfigExternal(
			&d.Config.CommonStorageDriverConfig),
		TenantName:     d.Config.TenantName,
		EndPoint:       fmt.Sprintf("https://%s", endpointHalves[1]),
		SVIP:           d.Config.SVIP,
		InitiatorIFace: d.Config.InitiatorIFace,
		Types:          d.Config.Types,
	}
}
