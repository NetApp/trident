// Copyright 2016 NetApp, Inc. All Rights Reserved.

package fake

import (
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/drivers/fake"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
)

type FakeStorageDriver struct {
	fake.FakeStorageDriver
}

func (m *FakeStorageDriver) GetStorageBackendSpecs(
	backend *storage.StorageBackend,
) error {
	backend.Name = m.Config.InstanceName
	for name, pool := range m.Config.Pools {
		vc := &storage.StoragePool{
			Name:           name,
			StorageClasses: make([]string, 0),
			Volumes:        make(map[string]*storage.Volume, 0),
			Backend:        backend,
			Attributes:     pool.Attrs,
		}
		vc.Attributes[sa.BackendType] = sa.NewStringOffer(m.Name())
		backend.AddStoragePool(vc)
	}
	return nil
}

func (m *FakeStorageDriver) GetVolumeOpts(
	volConfig *storage.VolumeConfig,
	pool *storage.StoragePool,
	requests map[string]sa.Request,
) (map[string]string, error) {
	opts := make(map[string]string)
	opts[fake.FakePoolAttribute] = pool.Name
	return opts, nil
}

func (m *FakeStorageDriver) GetInternalVolumeName(name string) string {
	return storage.GetCommonInternalVolumeName(
		&m.Config.CommonStorageDriverConfig, name)
}

func (m *FakeStorageDriver) CreatePrepare(
	volConfig *storage.VolumeConfig,
) bool {
	volConfig.InternalName = m.GetInternalVolumeName(volConfig.Name)
	return true
}

func (m *FakeStorageDriver) CreateFollowup(
	volConfig *storage.VolumeConfig,
) error {
	return nil
}

func (d *FakeStorageDriver) GetProtocol() config.Protocol {
	return d.Config.Protocol
}

func (d *FakeStorageDriver) GetDriverName() string {
	return d.Config.StorageDriverName
}

func (d *FakeStorageDriver) StoreConfig(
	b *storage.PersistentStorageBackendConfig,
) {
	storage.SanitizeCommonStorageDriverConfig(
		&d.Config.CommonStorageDriverConfig)
	b.FakeStorageDriverConfig = &d.Config
}

func (d *FakeStorageDriver) GetExternalConfig() interface{} {
	// It's fake, so by definition, there's nothing sensitive
	return &d.Config
}
