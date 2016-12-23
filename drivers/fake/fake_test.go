// Copyright 2016 NetApp, Inc. All Rights Reserved.

package fake

import (
	"fmt"
	"testing"

	"github.com/netapp/trident/config"
	sa "github.com/netapp/trident/storage_attribute"
)

func getPools(count int) map[string]*FakeStoragePool {
	ret := make(map[string]*FakeStoragePool, count)
	for i := 0; i < count; i++ {
		ret[fmt.Sprintf("pool-%d", i)] = &FakeStoragePool{
			Bytes: 100 * 1024 * 1024 * 1024,
			Attrs: map[string]sa.Offer{
				sa.IOPS:             sa.NewIntOffer(0, 100),
				sa.Snapshots:        sa.NewBoolOffer(false),
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
