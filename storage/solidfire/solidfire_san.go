// Copyright 2016 NetApp, Inc. All Rights Reserved.

package solidfire

import (
	"fmt"
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
		vc := storage.NewStoragePool(backend, volType.Type)

		vc.Attributes[sa.Media] = sa.NewStringOffer(sa.SSD)
		vc.Attributes[sa.IOPS] = sa.NewIntOffer(int(volType.QOS.MinIOPS),
			int(volType.QOS.MaxIOPS))
		vc.Attributes[sa.Snapshots] = sa.NewBoolOffer(true)
		vc.Attributes[sa.Encryption] = sa.NewBoolOffer(false)
		vc.Attributes[sa.ProvisioningType] = sa.NewStringOffer("thin")
		vc.Attributes[sa.BackendType] = sa.NewStringOffer(d.Name())
		backend.AddStoragePool(vc)
	}

	return nil
}

func (d *SolidfireSANStorageDriver) GetInternalVolumeName(name string) string {
	s1 := storage.GetCommonInternalVolumeName(d.Config.CommonStorageDriverConfig, name)
	s2 := strings.Replace(s1, "_", "-", -1)
	s3 := strings.Replace(s2, ".", "-", -1)
	return s3
}

func (d *SolidfireSANStorageDriver) CreatePrepare(
	volConfig *storage.VolumeConfig,
) bool {
	// 1. Sanitize the volume name
	volConfig.InternalName = d.GetInternalVolumeName(volConfig.Name)

	if volConfig.SourceName != "" {
		volConfig.SourceInternalName = d.GetInternalVolumeName(volConfig.SourceName)
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
	opts["type"] = pool.Name
	if volConfig.BlockSize != "" {
		opts["blocksize"] = volConfig.BlockSize
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

func (d *SolidfireSANStorageDriver) GetExternalVolume(name string) (*storage.VolumeExternal, error) {

	volumeConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         name,
		InternalName: name,
	}

	volume := &storage.VolumeExternal{
		Config:  volumeConfig,
		Backend: d.Name(),
		Pool:    "",
	}

	return volume, nil
}
