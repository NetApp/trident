// Copyright 2016 NetApp, Inc. All Rights Reserved.

package solidfire

import (
	"strings"
	"testing"

	"github.com/netapp/netappdvp/apis/sfapi"
	dvp "github.com/netapp/netappdvp/storage_drivers"
)

func TestGetExternalConfig(t *testing.T) {
	driver := SolidfireSANStorageDriver{
		SolidfireSANStorageDriver: dvp.SolidfireSANStorageDriver{
			Config: dvp.SolidfireStorageDriverConfig{
				CommonStorageDriverConfig: dvp.CommonStorageDriverConfig{
					Version:           1,
					StorageDriverName: "solidfire-san",
				},
				TenantName:     "test",
				EndPoint:       "https://admin:solidfire@10.63.171.151/json-rpc/7.0",
				DefaultVolSz:   100 * 1024 * 1024 * 1024,
				SVIP:           "10.63.171.153:3260",
				InitiatorIFace: "default",
				Types: &[]sfapi.VolType{
					sfapi.VolType{
						Type: "Bronze", QOS: sfapi.QoS{
							MinIOPS:   1000,
							MaxIOPS:   2000,
							BurstIOPS: 4000,
						}},
					sfapi.VolType{
						Type: "Silver",
						QOS: sfapi.QoS{
							MinIOPS:   4000,
							MaxIOPS:   6000,
							BurstIOPS: 8000,
						},
					},
					sfapi.VolType{
						Type: "Gold",
						QOS: sfapi.QoS{
							MinIOPS:   6000,
							MaxIOPS:   8000,
							BurstIOPS: 10000,
						},
					},
				},
			},
		},
	}
	newConfig := driver.GetExternalConfig().(*SolidfireStorageDriverConfigExternal)
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
