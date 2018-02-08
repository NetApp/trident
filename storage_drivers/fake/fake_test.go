// Copyright 2016 NetApp, Inc. All Rights Reserved.

package fake

import (
	"fmt"
	"testing"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage/fake"
	sa "github.com/netapp/trident/storage_attribute"
)

func getPools(count int) map[string]*fake.StoragePool {
	ret := make(map[string]*fake.StoragePool, count)
	for i := 0; i < count; i++ {
		ret[fmt.Sprintf("pool-%d", i)] = &fake.StoragePool{
			Bytes: 100 * 1024 * 1024 * 1024,
			Attrs: map[string]sa.Offer{
				sa.IOPS:             sa.NewIntOffer(0, 100),
				sa.Snapshots:        sa.NewBoolOffer(false),
				sa.Encryption:       sa.NewBoolOffer(false),
				sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
			},
		}
	}
	return ret
}

// TestNewConfig tests that marshaling works properly.  This has broken
// in the past.
func TestNewConfig(t *testing.T) {
	_, err := NewFakeStorageDriverConfigJSON("test", config.File,
		getPools(2))
	if err != nil {
		t.Fatal("Unable to generate config JSON:  ", err)
	}
}
