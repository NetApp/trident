// Copyright 2019 NetApp, Inc. All Rights Reserved.

package storageclass

import (
	"encoding/json"

	storageattribute "github.com/netapp/trident/storage_attribute"
)

// UnmarshalJSON parses a JSON-formatted byte array into a storage class config struct.
func (c *Config) UnmarshalJSON(data []byte) error {
	var tmp struct {
		Version         string              `json:"version"`
		Name            string              `json:"name"`
		Attributes      json.RawMessage     `json:"attributes,omitempty"`
		Pools           map[string][]string `json:"storagePools,omitempty"`
		RequiredStorage map[string][]string `json:"requiredStorage,omitempty"`
		AdditionalPools map[string][]string `json:"additionalStoragePools,omitempty"`
		ExcludePools    map[string][]string `json:"excludeStoragePools,omitempty"`
		AutogrowPolicy  string              `json:"autogrowPolicy,omitempty"`
	}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	c.Version = tmp.Version
	c.Name = tmp.Name
	c.Attributes, err = storageattribute.UnmarshalRequestMap(tmp.Attributes)
	c.Pools = tmp.Pools

	// Handle the renaming of "requiredStorage" to "additionalStoragePools"
	if tmp.RequiredStorage != nil && tmp.AdditionalPools == nil {
		c.AdditionalPools = tmp.RequiredStorage
	} else {
		c.AdditionalPools = tmp.AdditionalPools
	}

	c.ExcludePools = tmp.ExcludePools
	c.AutogrowPolicy = tmp.AutogrowPolicy

	return err
}

// MarshalJSON emits a storage class config struct as a JSON-formatted byte array.
func (c *Config) MarshalJSON() ([]byte, error) {
	var tmp struct {
		Version         string              `json:"version"`
		Name            string              `json:"name"`
		Attributes      json.RawMessage     `json:"attributes,omitempty"`
		Pools           map[string][]string `json:"storagePools,omitempty"`
		AdditionalPools map[string][]string `json:"additionalStoragePools,omitempty"`
		ExcludePools    map[string][]string `json:"excludeStoragePools,omitempty"`
		AutogrowPolicy  string              `json:"autogrowPolicy,omitempty"`
	}
	tmp.Version = c.Version
	tmp.Name = c.Name
	tmp.Pools = c.Pools
	tmp.AdditionalPools = c.AdditionalPools
	tmp.ExcludePools = c.ExcludePools
	tmp.AutogrowPolicy = c.AutogrowPolicy
	// TODO (agagan): The below function MarshalRequestMap always return a positive response.
	//  The negative use case is not covered in the unit test.
	attrs, err := storageattribute.MarshalRequestMap(c.Attributes)
	if err != nil {
		return nil, err
	}
	tmp.Attributes = attrs
	return json.Marshal(&tmp)
}
