// Copyright 2016 NetApp, Inc. All Rights Reserved.

package test_utils

import (
	"fmt"

	"github.com/netapp/trident/drivers/fake"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
)

const (
	SlowNoSnapshots = "slow-no-snapshots"
	SlowSnapshots   = "slow-snapshots"
	FastSmall       = "fast-small"
	FastThinOnly    = "fast-thin-only"
	FastUniqueAttr  = "fast-unique-attr"
	MediumOverlap   = "medium-overlap"
)

type PoolMatch struct {
	Backend string
	Pool    string
}

func (p *PoolMatch) Matches(pool *storage.StoragePool) bool {
	return pool.Name == p.Pool && pool.Backend.Name == p.Backend
}

func (p *PoolMatch) String() string {
	return fmt.Sprintf("%s:%s", p.Backend, p.Pool)
}

func GetFakePools() map[string]*fake.FakeStoragePool {
	return map[string]*fake.FakeStoragePool{
		SlowNoSnapshots: &fake.FakeStoragePool{
			Bytes: 50 * 1024 * 1024 * 1024,
			Attrs: map[string]sa.Offer{
				sa.IOPS:             sa.NewIntOffer(0, 100),
				sa.Snapshots:        sa.NewBoolOffer(false),
				sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
			},
		},
		SlowSnapshots: &fake.FakeStoragePool{
			Bytes: 50 * 1024 * 1024 * 1024,
			Attrs: map[string]sa.Offer{
				sa.IOPS:             sa.NewIntOffer(0, 100),
				sa.Snapshots:        sa.NewBoolOffer(true),
				sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
			},
		},
		FastSmall: &fake.FakeStoragePool{
			Bytes: 25 * 1024 * 1024 * 1024,
			Attrs: map[string]sa.Offer{
				sa.IOPS:             sa.NewIntOffer(1000, 10000),
				sa.Snapshots:        sa.NewBoolOffer(true),
				sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
			},
		},
		FastThinOnly: &fake.FakeStoragePool{
			Bytes: 50 * 1024 * 1024 * 1024,
			Attrs: map[string]sa.Offer{
				sa.IOPS:             sa.NewIntOffer(1000, 10000),
				sa.Snapshots:        sa.NewBoolOffer(true),
				sa.ProvisioningType: sa.NewStringOffer("thin"),
			},
		},
		FastUniqueAttr: &fake.FakeStoragePool{
			Bytes: 50 * 1024 * 1024 * 1024,
			Attrs: map[string]sa.Offer{
				sa.IOPS:             sa.NewIntOffer(1000, 10000),
				sa.Snapshots:        sa.NewBoolOffer(true),
				sa.ProvisioningType: sa.NewStringOffer("thin", "thick"),
				"uniqueOptions":     sa.NewStringOffer("foo", "bar", "baz"),
			},
		},
		MediumOverlap: &fake.FakeStoragePool{
			Bytes: 100 * 1024 * 1024 * 1024,
			Attrs: map[string]sa.Offer{
				sa.IOPS:             sa.NewIntOffer(500, 1000),
				sa.Snapshots:        sa.NewBoolOffer(true),
				sa.ProvisioningType: sa.NewStringOffer("thin"),
			},
		},
	}
}
