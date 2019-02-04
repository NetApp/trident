// Copyright 2018 NetApp, Inc. All Rights Reserved.

package testutils

import (
	"fmt"

	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/fake"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
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

func (p *PoolMatch) Matches(pool *storage.Pool) bool {
	return pool.Name == p.Pool && pool.Backend.Name == p.Backend
}

func (p *PoolMatch) String() string {
	return fmt.Sprintf("%s:%s", p.Backend, p.Pool)
}

func getFakeVirtualPool(size, region, zone string, labels map[string]string) drivers.FakeStorageDriverPool {

	commonConfigDefaults := drivers.CommonStorageDriverConfigDefaults{Size: size}
	fakeConfigDefaults := drivers.FakeStorageDriverConfigDefaults{
		CommonStorageDriverConfigDefaults: commonConfigDefaults,
	}

	return drivers.FakeStorageDriverPool{
		Labels: labels,
		Region: region,
		Zone:   zone,
		FakeStorageDriverConfigDefaults: fakeConfigDefaults,
	}
}

func GetFakeVirtualPools() (drivers.FakeStorageDriverPool, []drivers.FakeStorageDriverPool) {

	pool := getFakeVirtualPool("10G", "us-east", "", map[string]string{"cloud": "aws"})

	return pool, []drivers.FakeStorageDriverPool{
		getFakeVirtualPool("1G", "", "1", map[string]string{"performance": "gold", "cost": "3"}),
		getFakeVirtualPool("", "", "1", map[string]string{"performance": "silver", "cost": "2"}),
		getFakeVirtualPool("", "", "1", map[string]string{"performance": "bronze", "cost": "1"}),
		getFakeVirtualPool("1G", "", "2", map[string]string{"performance": "gold", "cost": "3"}),
		getFakeVirtualPool("", "", "2", map[string]string{"performance": "silver", "cost": "2"}),
		getFakeVirtualPool("", "", "2", map[string]string{"performance": "bronze", "cost": "1"}),
	}
}

func GetFakePools() map[string]*fake.StoragePool {
	return map[string]*fake.StoragePool{
		SlowNoSnapshots: {
			Bytes: 50 * 1024 * 1024 * 1024,
			Attrs: map[string]sa.Offer{
				sa.IOPS:             sa.NewIntOffer(0, 100),
				sa.Snapshots:        sa.NewBoolOffer(false),
				sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
				sa.Labels:           sa.NewLabelOffer(map[string]string{"cloud": "aws", "performance": "bronze"}),
			},
		},
		SlowSnapshots: {
			Bytes: 50 * 1024 * 1024 * 1024,
			Attrs: map[string]sa.Offer{
				sa.IOPS:             sa.NewIntOffer(0, 100),
				sa.Snapshots:        sa.NewBoolOffer(true),
				sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
				sa.Labels:           sa.NewLabelOffer(map[string]string{"cloud": "aws", "performance": "bronze"}),
			},
		},
		FastSmall: {
			Bytes: 25 * 1024 * 1024 * 1024,
			Attrs: map[string]sa.Offer{
				sa.IOPS:             sa.NewIntOffer(1000, 10000),
				sa.Snapshots:        sa.NewBoolOffer(true),
				sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
				sa.Labels:           sa.NewLabelOffer(map[string]string{"cloud": "aws", "performance": "gold"}),
			},
		},
		FastThinOnly: {
			Bytes: 50 * 1024 * 1024 * 1024,
			Attrs: map[string]sa.Offer{
				sa.IOPS:             sa.NewIntOffer(1000, 10000),
				sa.Snapshots:        sa.NewBoolOffer(true),
				sa.ProvisioningType: sa.NewStringOffer("thin"),
				sa.Labels:           sa.NewLabelOffer(map[string]string{"cloud": "azure", "performance": "gold"}),
			},
		},
		FastUniqueAttr: {
			Bytes: 50 * 1024 * 1024 * 1024,
			Attrs: map[string]sa.Offer{
				sa.IOPS:             sa.NewIntOffer(1000, 10000),
				sa.Snapshots:        sa.NewBoolOffer(true),
				sa.ProvisioningType: sa.NewStringOffer("thin", "thick"),
				sa.Labels:           sa.NewLabelOffer(map[string]string{"cloud": "azure", "performance": "gold"}),
				"uniqueOptions":     sa.NewStringOffer("foo", "bar", "baz"),
			},
		},
		MediumOverlap: {
			Bytes: 100 * 1024 * 1024 * 1024,
			Attrs: map[string]sa.Offer{
				sa.IOPS:             sa.NewIntOffer(500, 1000),
				sa.Snapshots:        sa.NewBoolOffer(true),
				sa.ProvisioningType: sa.NewStringOffer("thin"),
				sa.Labels:           sa.NewLabelOffer(map[string]string{"cloud": "azure", "performance": "silver"}),
			},
		},
	}
}
