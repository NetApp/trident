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
