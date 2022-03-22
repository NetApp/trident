// Copyright 2018 NetApp, Inc. All Rights Reserved.

package docker

type Version struct {
	Client struct {
		Version           string `json:"Version"`
		APIVersion        string `json:"ApiVersion"`
		DefaultAPIVersion string `json:"DefaultAPIVersion"`
		GitCommit         string `json:"GitCommit"`
		GoVersion         string `json:"GoVersion"`
		Os                string `json:"Os"`
		Arch              string `json:"Arch"`
		BuildTime         string `json:"BuildTime"`
	} `json:"Client"`
	Server struct {
		Version       string `json:"Version"`
		APIVersion    string `json:"ApiVersion"`
		MinAPIVersion string `json:"MinAPIVersion"`
		GitCommit     string `json:"GitCommit"`
		GoVersion     string `json:"GoVersion"`
		Os            string `json:"Os"`
		Arch          string `json:"Arch"`
		KernelVersion string `json:"KernelVersion"`
		BuildTime     string `json:"BuildTime"`
	} `json:"Server"`
}

type Snapshot struct {
	Name    string `json:"name"`
	Created string `json:"dateCreated"` // The UTC time that the snapshot was created, in RFC3339 format
}

const (
	DaemonConfigFile      = "/etc/docker/daemon.json"
	DefaultDaemonDataRoot = "/var/lib/docker"
)

// DaemonConfig holds only our fields of interest from /etc/docker/daemon.json
type DaemonConfig struct {
	// See also:
	//   https://docs.docker.com/engine/reference/commandline/dockerd/#daemon-configuration-file
	//   https://docs.docker.com/config/daemon/#configure-the-docker-daemon
	DataRoot string `json:"data-root"`
}
