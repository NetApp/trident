// Copyright 2022 NetApp, Inc. All Rights Reserved.

package v1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TridentOrchestrator is the Schema for the tridentorchestrators API
type TridentOrchestrator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TridentOrchestratorSpec   `json:"spec,omitempty"`
	Status TridentOrchestratorStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TridentOrchestratorList contains a list of TridentOrchestrator
type TridentOrchestratorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []TridentOrchestrator `json:"items"`
}

// TridentOrchestratorSpec defines the desired state of TridentOrchestrator
type TridentOrchestratorSpec struct {
	EnableForceDetach            bool              `json:"enableForceDetach"`
	DisableAuditLog              *bool             `json:"disableAuditLog"`
	Namespace                    string            `json:"namespace"`
	IPv6                         bool              `json:"IPv6,omitempty"`
	K8sTimeout                   int               `json:"k8sTimeout,omitempty"`
	HTTPRequestTimeout           string            `json:"httpRequestTimeout,omitempty"`
	SilenceAutosupport           bool              `json:"silenceAutosupport,omitempty"`
	AutosupportImage             string            `json:"autosupportImage,omitempty"`
	AutosupportProxy             string            `json:"autosupportProxy,omitempty"`
	AutosupportInsecure          bool              `json:"autosupportInsecure,omitempty"`
	AutosupportSerialNumber      string            `json:"autosupportSerialNumber,omitempty"`
	AutosupportHostname          string            `json:"autosupportHostname,omitempty"`
	Uninstall                    bool              `json:"uninstall,omitempty"`
	LogFormat                    string            `json:"logFormat,omitempty"`
	LogLevel                     string            `json:"logLevel,omitempty"`
	Debug                        bool              `json:"debug,omitempty"`
	LogWorkflows                 string            `json:"logWorkflows,omitempty"`
	LogLayers                    string            `json:"logLayers,omitempty"`
	ProbePort                    *int64            `json:"probePort,omitempty"`
	TridentImage                 string            `json:"tridentImage,omitempty"`
	ImageRegistry                string            `json:"imageRegistry,omitempty"`
	KubeletDir                   string            `json:"kubeletDir,omitempty"`
	Wipeout                      []string          `json:"wipeout,omitempty"`
	ImagePullSecrets             []string          `json:"imagePullSecrets,omitempty"`
	ControllerPluginNodeSelector map[string]string `json:"controllerPluginNodeSelector,omitempty"`
	ControllerPluginTolerations  []Toleration      `json:"controllerPluginTolerations,omitempty"`
	NodePluginNodeSelector       map[string]string `json:"nodePluginNodeSelector,omitempty"`
	NodePluginTolerations        []Toleration      `json:"nodePluginTolerations,omitempty"`
	Windows                      bool              `json:"windows,omitempty"`
	ImagePullPolicy              string            `json:"imagePullPolicy,omitempty"`
	CloudProvider                string            `json:"cloudProvider,omitempty"`
	EnableACP                    bool              `json:"enableACP,omitempty"`
	ACPImage                     string            `json:"acpImage,omitempty"`
}

// Toleration
type Toleration struct {
	Effect            string `json:"effect,omitempty"`
	Key               string `json:"key,omitempty"`
	Value             string `json:"value,omitempty"`
	Operator          string `json:"operator,omitempty"`
	TolerationSeconds int    `json:"tolerationSeconds,omitempty"`
}

func (t *Toleration) GetMap() map[string]string {
	toleration := map[string]string{}

	if t.Key != "" {
		toleration["key"] = t.Key
	}

	if t.Effect != "" {
		toleration["effect"] = t.Effect
	}

	if t.Value != "" {
		toleration["value"] = t.Value
	}

	if t.Operator != "" {
		toleration["operator"] = t.Operator
	}

	if t.TolerationSeconds != 0 {
		toleration["tolerationSeconds"] = fmt.Sprintf("%d", t.TolerationSeconds)
	}

	return toleration
}

// TridentOrchestratorStatus defines the observed state of TridentOrchestrator
type TridentOrchestratorStatus struct {
	Message                   string                        `json:"message"`
	Status                    string                        `json:"status"`
	Version                   string                        `json:"version"`
	Namespace                 string                        `json:"namespace"`
	CurrentInstallationParams TridentOrchestratorSpecValues `json:"currentInstallationParams"`
	ACPVersion                string                        `json:"acpVersion"`
}

type TridentOrchestratorSpecValues struct {
	EnableForceDetach       string            `json:"enableForceDetach"`
	DisableAuditLog         string            `json:"disableAuditLog"`
	IPv6                    string            `json:"IPv6"`
	SilenceAutosupport      string            `json:"silenceAutosupport"`
	AutosupportImage        string            `json:"autosupportImage"`
	AutosupportProxy        string            `json:"autosupportProxy"`
	AutosupportInsecure     bool              `json:"autosupportInsecure"`
	AutosupportSerialNumber string            `json:"autosupportSerialNumber"`
	AutosupportHostname     string            `json:"autosupportHostname"`
	K8sTimeout              string            `json:"k8sTimeout"`
	HTTPRequestTimeout      string            `json:"httpRequestTimeout"`
	LogFormat               string            `json:"logFormat"`
	LogLevel                string            `json:"logLevel"`
	Debug                   string            `json:"debug"`
	LogWorkflows            string            `json:"logWorkflows"`
	LogLayers               string            `json:"logLayers"`
	ProbePort               string            `json:"probePort"`
	TridentImage            string            `json:"tridentImage"`
	ImageRegistry           string            `json:"imageRegistry"`
	KubeletDir              string            `json:"kubeletDir"`
	ImagePullSecrets        []string          `json:"imagePullSecrets"`
	NodePluginNodeSelector  map[string]string `json:"nodePluginNodeSelector,omitempty"`
	NodePluginTolerations   []Toleration      `json:"nodePluginTolerations,omitempty"`
	ImagePullPolicy         string            `json:"imagePullPolicy"`
	EnableACP               string            `json:"enableACP"`
	ACPImage                string            `json:"acpImage"`
}
