// Copyright 2016 NetApp, Inc. All Rights Reserved.

package fake

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	dvp "github.com/netapp/netappdvp/storage_drivers"
	"github.com/netapp/netappdvp/utils"

	"github.com/netapp/trident/config"
	sa "github.com/netapp/trident/storage_attribute"
)

const (
	FakeStorageDriverName = "fake"
	FakePoolAttribute     = "pool"
)

type FakeStoragePool struct {
	Attrs map[string]sa.Offer
	Bytes uint64
}

// UnmarshalJSON implements json.Unmarshaler and allows FakeStoragePool
// to be unmarshaled with the Attrs map correctly defined.
func (m *FakeStoragePool) UnmarshalJSON(data []byte) error {
	var tmp struct {
		Attrs json.RawMessage
		Bytes uint64
	}

	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	m.Attrs, err = sa.UnmarshalOfferMap(tmp.Attrs)
	if err != nil {
		return err
	}
	m.Bytes = tmp.Bytes
	return nil
}

type FakeStorageDriverConfig struct {
	dvp.CommonStorageDriverConfig
	Protocol config.Protocol
	// pools represents the possible buckets into which a given volume should go
	Pools        map[string]*FakeStoragePool
	InstanceName string
}

type FakeStorageDriver struct {
	Config FakeStorageDriverConfig
	// Volumes maps volumes to the name of the pool in which the volume should
	// be stored.
	Volumes      map[string]string // Maps
	VolumesAdded int
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
	json, err := json.Marshal(
		&FakeStorageDriverConfig{
			CommonStorageDriverConfig: dvp.CommonStorageDriverConfig{
				Version:           1,
				StorageDriverName: FakeStorageDriverName,
				StoragePrefixRaw:  json.RawMessage("{}"),
				SnapshotPrefixRaw: json.RawMessage("{}"),
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

func (m *FakeStorageDriver) Name() string {
	return FakeStorageDriverName
}

func (m *FakeStorageDriver) Initialize(configJSON string) error {
	err := json.Unmarshal([]byte(configJSON), &m.Config)
	if err != nil {
		return fmt.Errorf("Unable to initialize fake driver:  %v", err)
	}
	m.Volumes = make(map[string]string)
	m.VolumesAdded = 0
	m.DestroyedVolumes = make(map[string]bool)
	return nil
}

func (m *FakeStorageDriver) Validate() error {
	return nil
}

func (m *FakeStorageDriver) Create(name string, opts map[string]string) error {
	requestedSize, err := utils.ConvertSizeToBytes64(opts["size"])
	if err != nil {
		return fmt.Errorf("Unable to convert volume size %s: %s",
			opts["size"], err.Error())
	}
	volSize, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return fmt.Errorf("%s is an invalid volume size: %v", opts["size"], err)
	}

	poolName, ok := opts[FakePoolAttribute]
	if !ok {
		return fmt.Errorf("No pool specified.  Expected %s in opts map",
			FakePoolAttribute)
	}
	pool, ok := m.Config.Pools[poolName]
	if !ok {
		return fmt.Errorf("Invalid pool %s.", pool)
	}
	if _, ok = m.Volumes[name]; ok {
		return fmt.Errorf("Volume %s already exists", name)
	}
	if volSize > pool.Bytes {
		return fmt.Errorf("Requested volume is too large.  Requested %d bytes;"+
			" have %d available in pool %s.", volSize, pool.Bytes, poolName)
	}
	m.Volumes[name] = poolName
	m.VolumesAdded++
	pool.Bytes -= volSize
	return nil
}

func (m *FakeStorageDriver) Destroy(name string) error {
	m.DestroyedVolumes[name] = true
	if _, ok := m.Volumes[name]; !ok {
		// TODO:  return the standard volume not found error once that gets
		// added to the nDVP.
		return nil
	}
	delete(m.Volumes, name)
	return nil
}

func (m *FakeStorageDriver) Attach(name, mountpoint string, opts map[string]string) error {
	return errors.New("Fake driver does not support attaching.")
}

func (m *FakeStorageDriver) Detach(name, mountpoint string) error {
	return errors.New("Fake driver does not support detaching.")
}

func (m *FakeStorageDriver) DefaultStoragePrefix() string {
	return "fake"
}

func (d *FakeStorageDriver) CreateClone(
	name, source, snapshot, newSnapshotPrefix string,
) error {
	return errors.New("Fake driver does not support CreateClone")
}

func (d *FakeStorageDriver) DefaultSnapshotPrefix() string {
	return ""
}

func (d *FakeStorageDriver) SnapshotList(name string) ([]dvp.CommonSnapshot, error) {
	return nil, errors.New("Fake driver does not support SnapshotList")
}

func (m *FakeStorageDriver) List(prefix string) ([]string, error) {
	vols := []string{}
	for vol := range m.Volumes {
		vols = append(vols, vol)
	}
	return vols, nil
}

func (d *FakeStorageDriver) Get(name string) error {

	_, ok := d.Volumes[name]
	if !ok {
		return fmt.Errorf("Could not find volume %s.", name)
	}

	return nil
}
