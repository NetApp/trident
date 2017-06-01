package api

import "github.com/netapp/trident/storage"

type Backend struct {
	Name   string `json:"name"`
	Config struct {
		Version           int    `json:"version"`
		StorageDriverName string `json:"storageDriverName"`
		//StoragePrefix     struct {
		//} `json:"storagePrefix"`
	} `json:"config"`
	Storage interface{} `json:"storage"`
	Online  bool        `json:"online"`
	Volumes []string    `json:"volumes"`
}

type GetBackendResponse struct {
	Backend Backend `json:"backend"`
	Error   string  `json:"error"`
}

type MultipleBackendResponse struct {
	Items []Backend `json:"items"`
}

type StorageClass struct {
	Config struct {
		Version    string `json:"version"`
		Name       string `json:"name"`
		Attributes struct {
			BackendType interface{} `json:"backendType"`
		} `json:"attributes"`
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

type VersionResponse struct {
	Server struct {
		Version       string `json:"version"`
		MajorVersion  uint   `json:"majorVersion"`
		MinorVersion  uint   `json:"minorVersion"`
		PatchVersion  uint   `json:"patchVersion"`
		PreRelease    string `json:"preRelease"`
		BuildMetadata string `json:"buildMetadata"`
		APIVersion    string `json:"apiVersion"`
	} `json:"server"`
	Client struct {
		Version       string `json:"version"`
		MajorVersion  uint   `json:"majorVersion"`
		MinorVersion  uint   `json:"minorVersion"`
		PatchVersion  uint   `json:"patchVersion"`
		PreRelease    string `json:"preRelease"`
		BuildMetadata string `json:"buildMetadata"`
		APIVersion    string `json:"apiVersion"`
	} `json:"client"`
}
