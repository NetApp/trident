// Copyright 2021 NetApp, Inc. All Rights Reserved.

package v1

import (
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
	Debug                   bool     `json:"debug"`
	Namespace               string   `json:"namespace"`
	IPv6                    bool     `json:"IPv6,omitempty"`
	K8sTimeout              int      `json:"k8sTimeout,omitempty"`
	HTTPRequestTimeout      string   `json:"httpRequestTimeout,omitempty"`
	SilenceAutosupport      bool     `json:"silenceAutosupport,omitempty"`
	AutosupportImage        string   `json:"autosupportImage,omitempty"`
	AutosupportProxy        string   `json:"autosupportProxy,omitempty"`
	AutosupportSerialNumber string   `json:"autosupportSerialNumber,omitempty"`
	AutosupportHostname     string   `json:"autosupportHostname,omitempty"`
	Uninstall               bool     `json:"uninstall,omitempty"`
	LogFormat               string   `json:"logFormat,omitempty"`
	ProbePort               *int64   `json:"probePort,omitempty"`
	TridentImage            string   `json:"tridentImage,omitempty"`
	ImageRegistry           string   `json:"imageRegistry,omitempty"`
	KubeletDir              string   `json:"kubeletDir,omitempty"`
	Wipeout                 []string `json:"wipeout,omitempty"`
	ImagePullSecrets        []string `json:"imagePullSecrets,omitempty"`
	EnableNodePrep          bool     `json:"enableNodePrep,omitempty"`
}

// TridentOrchestratorStatus defines the observed state of TridentOrchestrator
type TridentOrchestratorStatus struct {
	Message                   string                        `json:"message"`
	Status                    string                        `json:"status"`
	Version                   string                        `json:"version"`
	Namespace                 string                        `json:"namespace"`
	CurrentInstallationParams TridentOrchestratorSpecValues `json:"currentInstallationParams"`
}

type TridentOrchestratorSpecValues struct {
	Debug                   string   `json:"debug"`
	IPv6                    string   `json:"IPv6"`
	SilenceAutosupport      string   `json:"silenceAutosupport"`
	AutosupportImage        string   `json:"autosupportImage"`
	AutosupportProxy        string   `json:"autosupportProxy"`
	AutosupportSerialNumber string   `json:"autosupportSerialNumber"`
	AutosupportHostname     string   `json:"autosupportHostname"`
	K8sTimeout              string   `json:"k8sTimeout"`
	HTTPRequestTimeout      string   `json:"httpRequestTimeout"`
	LogFormat               string   `json:"logFormat"`
	ProbePort               string   `json:"probePort"`
	TridentImage            string   `json:"tridentImage"`
	ImageRegistry           string   `json:"imageRegistry"`
	KubeletDir              string   `json:"kubeletDir"`
	ImagePullSecrets        []string `json:"imagePullSecrets"`
	EnableNodePrep          string   `json:"enableNodePrep"`
}
