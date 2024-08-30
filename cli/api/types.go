// Copyright 2019 NetApp, Inc. All Rights Reserved.

package api

import (
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/models"
)

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

type MultipleVolumePublicationResponse struct {
	Items []models.VolumePublicationExternal `json:"items"`
}

type MultipleNodeResponse struct {
	Items []models.NodeExternal `json:"items"`
}

type MultipleSnapshotResponse struct {
	Items []storage.SnapshotExternal `json:"items"`
}

type Version struct {
	Version       string `json:"version"`
	MajorVersion  uint   `json:"majorVersion"`
	MinorVersion  uint   `json:"minorVersion"`
	PatchVersion  uint   `json:"patchVersion"`
	PreRelease    string `json:"preRelease"`
	BuildMetadata string `json:"buildMetadata"`
	APIVersion    string `json:"apiVersion"`
	GoVersion     string `json:"goVersion"`
}

type VersionResponse struct {
	Server    *Version `json:"server"`
	Client    *Version `json:"client"`
	ACPServer *Version `json:"acpServer,omitempty"`
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

type CRStatus struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type OperatorPhaseStatus string

const (
	OperatorPhaseDone       OperatorPhaseStatus = "Done"
	OperatorPhaseProcessing OperatorPhaseStatus = "Processing"
	OperatorPhaseUnknown    OperatorPhaseStatus = "Unknown"
	OperatorPhaseFailed     OperatorPhaseStatus = "Failed"
	OperatorPhaseError      OperatorPhaseStatus = "Error"
)

type OperatorStatus struct {
	ErrorMessage string              `json:"errorMessage"`
	Status       string              `json:"operatorStatus"`
	TorcStatus   map[string]CRStatus `json:"torcStatus"`
	TconfStatus  map[string]CRStatus `json:"tconfStatus"`
}
