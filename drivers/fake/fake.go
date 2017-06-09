// Copyright 2016 NetApp, Inc. All Rights Reserved.

package fake

import (
	"encoding/json"
	"errors"
	"fmt"

	log "github.com/Sirupsen/logrus"
	dvp "github.com/netapp/netappdvp/storage_drivers"
	"github.com/netapp/trident/config"
	sa "github.com/netapp/trident/storage_attribute"
)

const (
	FakeStorageDriverName = "fake"
	FakePoolAttribute     = "pool"
)

type FakeStoragePool struct {
	Attrs map[string]sa.Offer `json:"attributes"`
	Bytes uint64              `json:"sizeBytes"`
}

// UnmarshalJSON implements json.Unmarshaler and allows FakeStoragePool
// to be unmarshaled with the Attrs map correctly defined.
func (p *FakeStoragePool) UnmarshalJSON(data []byte) error {
	var tmp struct {
		Attrs json.RawMessage `json:"attributes"`
		Bytes uint64          `json:"sizeBytes"`
	}

	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	p.Attrs, err = sa.UnmarshalOfferMap(tmp.Attrs)
	if err != nil {
		return err
	}
	p.Bytes = tmp.Bytes
	return nil
}

type FakeStorageDriverConfig struct {
	dvp.CommonStorageDriverConfig
	Protocol config.Protocol `json:"protocol"`
	// pools represents the possible buckets into which a given volume should go
	Pools        map[string]*FakeStoragePool `json:"pools"`
	InstanceName string                      `json:"instanceName"`
}

type FakeVolume struct {
	name      string
	poolName  string
	sizeBytes uint64
}

type FakeStorageDriver struct {
	Config FakeStorageDriverConfig

	// volumes saves info about volumes created on this driver
	volumes map[string]FakeVolume

	// DestroyedVolumes is here so that tests can check whether destroy
	// has been called on a volume during or after bootstrapping, since
	// different driver instances with the same config won't actually share
	// state.
	DestroyedVolumes map[string]bool
}

func newFakeStorageDriverConfigJSON(
	name string,
	protocol config.Protocol,
	pools map[string]*FakeStoragePool,
	destroyIgnoreNotPresent bool,
) (string, error) {
	prefix := ""
	json, err := json.Marshal(
		&FakeStorageDriverConfig{
			CommonStorageDriverConfig: dvp.CommonStorageDriverConfig{
				Version:           1,
				StorageDriverName: FakeStorageDriverName,
				StoragePrefixRaw:  json.RawMessage("{}"),
				StoragePrefix:     &prefix,
			},
			Protocol:     protocol,
			Pools:        pools,
			InstanceName: name,
		},
	)
	if err != nil {
		return "", err
	}
	return string(json), nil
}

func NewFakeStorageDriverConfigJSON(
	name string,
	protocol config.Protocol,
	pools map[string]*FakeStoragePool,
) (string, error) {
	return newFakeStorageDriverConfigJSON(name, protocol, pools, false)
}

func (d *FakeStorageDriver) Name() string {
	return FakeStorageDriverName
}

func (d *FakeStorageDriver) Initialize(configJSON string, commonConfig *dvp.CommonStorageDriverConfig) error {

	err := json.Unmarshal([]byte(configJSON), &d.Config)
	if err != nil {
		return fmt.Errorf("Unable to initialize fake driver: %v", err)
	}

	d.volumes = make(map[string]FakeVolume)
	d.DestroyedVolumes = make(map[string]bool)

	s, err := json.Marshal(d.Config)
	log.Debugf("FakeStorageDriverConfig: %s", string(s))

	return nil
}

func (d *FakeStorageDriver) Validate() error {
	return nil
}

func (d *FakeStorageDriver) Create(name string, sizeBytes uint64, opts map[string]string) error {

	poolName, ok := opts[FakePoolAttribute]
	if !ok {
		return fmt.Errorf("No pool specified.  Expected %s in opts map", FakePoolAttribute)
	}

	pool, ok := d.Config.Pools[poolName]
	if !ok {
		return fmt.Errorf("Could not find pool %s.", pool)
	}

	if _, ok = d.volumes[name]; ok {
		return fmt.Errorf("Volume %s already exists", name)
	}

	if sizeBytes > pool.Bytes {
		return fmt.Errorf("Requested volume is too large.  Requested %d bytes; "+
			"have %d available in pool %s.", sizeBytes, pool.Bytes, poolName)
	}

	d.volumes[name] = FakeVolume{
		name:      name,
		poolName:  poolName,
		sizeBytes: sizeBytes,
	}
	d.DestroyedVolumes[name] = false
	pool.Bytes -= sizeBytes

	log.WithFields(log.Fields{
		"backend":   d.Config.InstanceName,
		"name":      name,
		"poolName":  poolName,
		"sizeBytes": sizeBytes,
	}).Debug("Created fake volume.")

	return nil
}

func (d *FakeStorageDriver) CreateClone(name, source, snapshot string, opts map[string]string) error {
	return errors.New("Fake driver does not support CreateClone")
}

func (d *FakeStorageDriver) Destroy(name string) error {

	d.DestroyedVolumes[name] = true

	volume, ok := d.volumes[name]
	if !ok {
		return nil
	}

	pool, ok := d.Config.Pools[volume.poolName]
	if !ok {
		return fmt.Errorf("Could not find pool %s.", volume.poolName)
	}

	pool.Bytes += volume.sizeBytes
	delete(d.volumes, name)

	log.WithFields(log.Fields{
		"backend":   d.Config.InstanceName,
		"name":      name,
		"poolName":  volume.poolName,
		"sizeBytes": volume.sizeBytes,
	}).Debug("Deleted fake volume.")

	return nil
}

func (d *FakeStorageDriver) Attach(name, mountpoint string, opts map[string]string) error {
	return errors.New("Fake driver does not support attaching.")
}

func (d *FakeStorageDriver) Detach(name, mountpoint string) error {
	return errors.New("Fake driver does not support detaching.")
}

func (d *FakeStorageDriver) SnapshotList(name string) ([]dvp.CommonSnapshot, error) {
	return nil, errors.New("Fake driver does not support SnapshotList")
}

func (d *FakeStorageDriver) List() ([]string, error) {
	vols := []string{}
	for vol := range d.volumes {
		vols = append(vols, vol)
	}
	return vols, nil
}

func (d *FakeStorageDriver) Get(name string) error {

	_, ok := d.volumes[name]
	if !ok {
		return fmt.Errorf("Could not find volume %s.", name)
	}

	return nil
}
