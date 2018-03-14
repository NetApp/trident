// Copyright 2018 NetApp, Inc. All Rights Reserved.

package solidfire

import (
	"strings"
	"testing"

	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/solidfire/api"
)

func TestGetExternalConfig(t *testing.T) {
	driver := SANStorageDriver{
		Config: drivers.SolidfireStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				Version:           1,
				StorageDriverName: "solidfire-san",
			},
			SolidfireStorageDriverConfigDefaults: drivers.SolidfireStorageDriverConfigDefaults{
				CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
					Size: "100GiB",
				},
			},
			TenantName:     "test",
			EndPoint:       "https://admin:solidfire@10.63.171.151/json-rpc/7.0",
			SVIP:           "10.63.171.153:3260",
			InitiatorIFace: "default",
			Types: &[]api.VolType{
				{
					Type: "Bronze", QOS: api.QoS{
						MinIOPS:   1000,
						MaxIOPS:   2000,
						BurstIOPS: 4000,
					}},
				{
					Type: "Silver",
					QOS: api.QoS{
						MinIOPS:   4000,
						MaxIOPS:   6000,
						BurstIOPS: 8000,
					},
				},
				{
					Type: "Gold",
					QOS: api.QoS{
						MinIOPS:   6000,
						MaxIOPS:   8000,
						BurstIOPS: 10000,
					},
				},
			},
		},
	}
	newConfig := driver.GetExternalConfig().(*StorageDriverConfigExternal)
	if newConfig.EndPoint == driver.Config.EndPoint {
		t.Error("EndPoints are equal; expected different.  Got:  ",
			newConfig.EndPoint)
	}
	if strings.Contains(newConfig.EndPoint, "admin") {
		t.Error("Username not removed from external config endpoint.")
	}
	if strings.Contains(newConfig.EndPoint, "solidfire") {
		t.Error("Password not removed from external config endpoint.")
	}
	t.Log("External config endpoint:  ", newConfig.EndPoint)
	if !strings.Contains(driver.Config.EndPoint, "admin") {
		t.Error("Username removed from main config endpoint.")
	}
	if !strings.Contains(driver.Config.EndPoint, "solidfire") {
		t.Errorf("Password removed from main config endpoint.")
	}
	t.Log("Main config endpoint:  ", driver.Config.EndPoint)
}
