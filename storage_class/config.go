// Copyright 2016 NetApp, Inc. All Rights Reserved.

package storage_class

import (
	"encoding/json"

	"github.com/netapp/trident/storage_attribute"
)

func (c *Config) UnmarshalJSON(data []byte) error {
	var tmp struct {
		Version             string              `json:"version"`
		Name                string              `json:"name"`
		Attributes          json.RawMessage     `json:"attributes,omitempty"`
		BackendStoragePools map[string][]string `json:"requiredStorage,omitempty"`
	}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	c.Version = tmp.Version
	c.Name = tmp.Name
	c.Attributes, err = storage_attribute.UnmarshalRequestMap(tmp.Attributes)
	c.BackendStoragePools = tmp.BackendStoragePools
	return err
}

func (c *Config) MarshalJSON() ([]byte, error) {
	var tmp struct {
		Version             string              `json:"version"`
		Name                string              `json:"name"`
		Attributes          json.RawMessage     `json:"attributes,omitempty"`
		BackendStoragePools map[string][]string `json:"requiredStorage,omitempty"`
	}
	tmp.Version = c.Version
	tmp.Name = c.Name
	tmp.BackendStoragePools = c.BackendStoragePools
	attrs, err := storage_attribute.MarshalRequestMap(c.Attributes)
	if err != nil {
		return nil, err
	}
	tmp.Attributes = attrs
	return json.Marshal(&tmp)
}
