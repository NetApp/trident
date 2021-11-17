package k8sclient

import (
	"github.com/netapp/trident/utils"
)

type DeploymentYAMLArguments struct {
	DeploymentName          string            `json:"deploymentName"`
	TridentImage            string            `json:"tridentImage"`
	AutosupportImage        string            `json:"autosupportImage"`
	AutosupportProxy        string            `json:"autosupportProxy"`
	AutosupportCustomURL    string            `json:"autosupportCustomURL"`
	AutosupportSerialNumber string            `json:"autosupportSerialNumber"`
	AutosupportHostname     string            `json:"autosupportHostname"`
	ImageRegistry           string            `json:"imageRegistry"`
	LogFormat               string            `json:"logFormat"`
	SnapshotCRDVersion      string            `json:"snapshotCRDVersion"`
	ImagePullSecrets        []string          `json:"imagePullSecrets"`
	Labels                  map[string]string `json:"labels"`
	ControllingCRDetails    map[string]string `json:"controllingCRDetails"`
	Debug                   bool              `json:"debug"`
	UseIPv6                 bool              `json:"useIPv6"`
	SilenceAutosupport      bool              `json:"silenceAutosupport"`
	Version                 *utils.Version    `json:"version"`
	TopologyEnabled		    bool              `json:"topologyEnabled"`
	HTTPRequestTimeout      string            `json:"httpRequestTimeout"`
}

type DaemonsetYAMLArguments struct {
	DaemonsetName           string            `json:"daemonsetName"`
	TridentImage            string            `json:"tridentImage"`
	ImageRegistry           string            `json:"imageRegistry"`
	KubeletDir              string            `json:"kubeletDir"`
	LogFormat               string            `json:"logFormat"`
	ProbePort               string            `json:"probePort"`
	ImagePullSecrets        []string          `json:"imagePullSecrets"`
	Labels                  map[string]string `json:"labels"`
	ControllingCRDetails    map[string]string `json:"controllingCRDetails"`
	Debug                   bool              `json:"debug"`
	NodePrep                bool              `json:"nodePrep"`
	Version                 *utils.Version    `json:"version"`
	HTTPRequestTimeout      string            `json:"httpRequestTimeout"`
}
