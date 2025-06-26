// Copyright 2022 NetApp, Inc. All Rights Reserved.

package storageclass

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	sa "github.com/netapp/trident/storage_attribute"
)

func TestUnmarshalJSON(t *testing.T) {
	conf := &Config{
		Name:            "bronze",
		Attributes:      make(map[string]sa.Request),
		AdditionalPools: make(map[string][]string),
		Pools:           map[string][]string{},
	}
	var storageClassConfig struct {
		Version         string              `json:"version"`
		Name            string              `json:"name"`
		RequiredStorage map[string][]string `json:"requiredStorage,omitempty"`
		AdditionalPools map[string][]string `json:"additionalStoragePools,omitempty"`
	}
	requiredStorage := make(map[string][]string)
	requiredStorage["Backend1"] = []string{"storagePool"}
	additionalPool := make(map[string][]string)
	additionalPool["Backend1"] = []string{"storagePool1"}
	storageClassConfig.Version = "v1"
	storageClassConfig.Name = "bronze"
	storageClassConfig.RequiredStorage = requiredStorage
	storageClassConfigByteArray, _ := json.Marshal(storageClassConfig)

	type unmarshalJSONTest struct {
		ByteArray       []byte
		isErrorExpected bool
	}

	tests := map[string]unmarshalJSONTest{
		"SuccessfullyUnmarshalJSON": {ByteArray: storageClassConfigByteArray, isErrorExpected: false},
		"FailedToUnmarshalJSON":     {ByteArray: []byte("any random string"), isErrorExpected: true},
	}

	for i, test := range tests {
		t.Run(i+":", func(t *testing.T) {
			err := conf.UnmarshalJSON(test.ByteArray)
			if test.isErrorExpected {
				assert.Error(t, err, "Error is nil")
			} else {
				assert.NoError(t, err, "Error object does not nil")
			}
		})
	}
}

func TestMarshalJSON(t *testing.T) {
	conf := &Config{
		Name:            "bronze",
		Attributes:      make(map[string]sa.Request),
		AdditionalPools: make(map[string][]string),
		Pools:           map[string][]string{},
		Version:         "v1",
	}
	response, err := conf.MarshalJSON()
	var jsonMap map[string]interface{}
	json.Unmarshal(response, &jsonMap)

	assert.NoError(t, err, "found error")
	assert.Equal(t, "bronze", jsonMap["name"], "config name does not match")
	assert.Empty(t, jsonMap["attributes"], "config attribute is not empty")
	assert.Equal(t, "v1", jsonMap["version"], "config version does not match")
}
