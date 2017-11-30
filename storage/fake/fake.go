// Copyright 2016 NetApp, Inc. All Rights Reserved.

package fake

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"strconv"

	dvp "github.com/netapp/netappdvp/storage_drivers"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/drivers/fake"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
)

type FakeStorageDriver struct {
	fake.FakeStorageDriver
}

func (d *FakeStorageDriver) GetStorageBackendSpecs(backend *storage.StorageBackend) error {
	backend.Name = d.Config.InstanceName
	for name, pool := range d.Config.Pools {
		vc := &storage.StoragePool{
			Name:           name,
			StorageClasses: make([]string, 0),
			Volumes:        make(map[string]*storage.Volume, 0),
			Backend:        backend,
			Attributes:     pool.Attrs,
		}
		vc.Attributes[sa.BackendType] = sa.NewStringOffer(d.Name())
		backend.AddStoragePool(vc)
	}
	return nil
}

func (d *FakeStorageDriver) GetVolumeOpts(
	volConfig *storage.VolumeConfig,
	pool *storage.StoragePool,
	requests map[string]sa.Request,
) (map[string]string, error) {
	opts := make(map[string]string)
	opts[fake.FakePoolAttribute] = pool.Name
	return opts, nil
}

func (d *FakeStorageDriver) GetInternalVolumeName(name string) string {
	return storage.GetCommonInternalVolumeName(
		d.Config.CommonStorageDriverConfig, name)
}

func (d *FakeStorageDriver) CreatePrepare(volConfig *storage.VolumeConfig) bool {
	volConfig.InternalName = d.GetInternalVolumeName(volConfig.Name)
	if volConfig.CloneSourceVolume != "" {
		volConfig.CloneSourceVolumeInternal =
			d.GetInternalVolumeName(volConfig.CloneSourceVolume)
	}
	return true
}

func (d *FakeStorageDriver) CreateFollowup(volConfig *storage.VolumeConfig) error {

	switch d.Config.Protocol {
	case config.File:
		volConfig.AccessInfo.NfsServerIP = "192.0.2.1" // unrouteable test address, see RFC 5737
		volConfig.AccessInfo.NfsPath = "/" + volConfig.InternalName
	case config.Block:
		volConfig.AccessInfo.IscsiTargetPortal = "192.0.2.1"
		volConfig.AccessInfo.IscsiTargetIQN = "iqn.2017-06.com.netapp:fake"
		volConfig.AccessInfo.IscsiLunNumber = 0
	}
	return nil
}

func (d *FakeStorageDriver) GetProtocol() config.Protocol {
	return d.Config.Protocol
}

func (d *FakeStorageDriver) GetDriverName() string {
	return d.Config.StorageDriverName
}

func (d *FakeStorageDriver) StoreConfig(b *storage.PersistentStorageBackendConfig) {
	storage.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)

	// Clone the config so we don't alter the original
	var cloneCommonConfig dvp.CommonStorageDriverConfig
	Clone(d.Config.CommonStorageDriverConfig, &cloneCommonConfig)
	cloneCommonConfig.SerialNumbers = nil

	b.FakeStorageDriverConfig = &fake.FakeStorageDriverConfig{
		CommonStorageDriverConfig: &cloneCommonConfig,
		Protocol:                  d.Config.Protocol,
		Pools:                     d.Config.Pools,
		InstanceName:              d.Config.InstanceName,
	}
}

func (d *FakeStorageDriver) GetExternalConfig() interface{} {

	storage.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)

	return &struct {
		*storage.CommonStorageDriverConfigExternal
		Protocol     config.Protocol                  `json:"protocol"`
		Pools        map[string]*fake.FakeStoragePool `json:"pools"`
		InstanceName string
	}{
		storage.GetCommonStorageDriverConfigExternal(
			d.Config.CommonStorageDriverConfig),
		d.Config.Protocol,
		d.Config.Pools,
		d.Config.InstanceName,
	}
}

func (d *FakeStorageDriver) GetVolumeExternal(name string) (*storage.VolumeExternal, error) {

	volume, ok := d.Volumes[name]
	if !ok {
		return nil, fmt.Errorf("Fake volume %s not found.", name)
	}

	return d.getVolumeExternal(volume), nil
}

func (d *FakeStorageDriver) GetVolumeExternalWrappers(
	channel chan *storage.VolumeExternalWrapper) {

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Convert all volumes to VolumeExternal and write them to the channel
	for _, volume := range d.Volumes {
		channel <- &storage.VolumeExternalWrapper{d.getVolumeExternal(volume), nil}
	}
}

func (d *FakeStorageDriver) getVolumeExternal(volume fake.FakeVolume) *storage.VolumeExternal {

	volumeConfig := &storage.VolumeConfig{
		Version:      config.OrchestratorAPIVersion,
		Name:         volume.Name,
		InternalName: volume.Name,
		Size:         strconv.FormatUint(volume.SizeBytes, 10),
	}

	volumeExternal := &storage.VolumeExternal{
		Config:  volumeConfig,
		Backend: d.Name(),
		Pool:    volume.PoolName,
	}

	return volumeExternal
}

func Clone(a, b interface{}) {
	buff := new(bytes.Buffer)
	enc := gob.NewEncoder(buff)
	dec := gob.NewDecoder(buff)
	enc.Encode(a)
	dec.Decode(b)
}
