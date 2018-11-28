// Copyright 2018 NetApp, Inc. All Rights Reserved.

package api

import "github.com/netapp/trident/storage"

type ErrorResponse struct {
	Error string `json:"error"`
}

type GetBackendResponse struct {
	Backend storage.BackendExternal `json:"backend"`
	Error   string                  `json:"error"`
}

type MultipleBackendResponse struct {
	Items []storage.BackendExternal `json:"items"`
}

type StorageClass struct {
	Config struct {
		Version         string              `json:"version"`
		Name            string              `json:"name"`
		Attributes      interface{}         `json:"attributes"`
		Pools           map[string][]string `json:"storagePools"`
		AdditionalPools map[string][]string `json:"additionalStoragePools"`
	} `json:"Config"`
	Storage interface{} `json:"storage"`
}

type GetStorageClassResponse struct {
	StorageClass `json:"storageClass"`
	Error        string `json:"error"`
}

type MultipleStorageClassResponse struct {
	Items []StorageClass `json:"items"`
}

type MultipleVolumeResponse struct {
	Items []storage.VolumeExternal `json:"items"`
}

type Version struct {
	Version       string `json:"version"`
	MajorVersion  uint   `json:"majorVersion"`
	MinorVersion  uint   `json:"minorVersion"`
	PatchVersion  uint   `json:"patchVersion"`
	PreRelease    string `json:"preRelease"`
	BuildMetadata string `json:"buildMetadata"`
	APIVersion    string `json:"apiVersion"`
}

type VersionResponse struct {
	Server Version `json:"server"`
	Client Version `json:"client"`
}

type ClientVersionResponse struct {
	Client Version `json:"client"`
}

type Metadata struct {
	Name string `json:"name,omitempty"`
}

type KubernetesNamespace struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Metadata   Metadata `json:"metadata"`
}
