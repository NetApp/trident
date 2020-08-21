// Copyright 2020 NetApp, Inc. All Rights Reserved.

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TridentProvisioner is the Schema for the tridentprovisioners API
type TridentProvisioner struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TridentProvisionerSpec   `json:"spec,omitempty"`
	Status TridentProvisionerStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TridentProvisionerList contains a list of TridentProvisioner
type TridentProvisionerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []TridentProvisioner `json:"items"`
}

// TridentProvisionerSpec defines the desired state of TridentProvisioner
type TridentProvisionerSpec struct {
	Debug                     bool     `json:"debug"`
	IPv6                      bool     `json:"IPv6,omitempty"`
	K8sTimeout                int      `json:"k8sTimeout,omitempty"`
	SilenceAutosupport        bool     `json:"silenceAutosupport,omitempty"`
	AutosupportImage          string   `json:"autosupportImage,omitempty"`
	AutosupportProxy          string   `json:"autosupportProxy,omitempty"`
	AutosupportSerialNumber   string   `json:"autosupportSerialNumber,omitempty"`
	AutosupportHostname       string   `json:"autosupportHostname,omitempty"`
	Uninstall                 bool     `json:"uninstall,omitempty"`
	LogFormat                 string   `json:"logFormat,omitempty"`
	TridentImage              string   `json:"tridentImage,omitempty"`
	ImageRegistry             string   `json:"imageRegistry,omitempty"`
	KubeletDir                string   `json:"kubeletDir,omitempty"`
	Wipeout                   []string `json:"wipeout,omitempty"`
	ImagePullSecrets          []string `json:"imagePullSecrets,omitempty"`
}

// TridentProvisionerStatus defines the observed state of TridentProvisioner
type TridentProvisionerStatus struct {
	Message                   string                       `json:"message"`
	Status                    string                       `json:"status"`
	Version                   string                       `json:"version"`
	CurrentInstallationParams TridentProvisionerSpecValues `json:"currentInstallationParams"`
}

type TridentProvisionerSpecValues struct {
	Debug                     string   `json:"debug"`
	IPv6                      string   `json:"IPv6"`
	SilenceAutosupport        string   `json:"silenceAutosupport"`
	AutosupportImage          string   `json:"autosupportImage"`
	AutosupportProxy          string   `json:"autosupportProxy"`
	AutosupportSerialNumber   string   `json:"autosupportSerialNumber"`
	AutosupportHostname       string   `json:"autosupportHostname"`
	K8sTimeout                string   `json:"k8sTimeout"`
	LogFormat                 string   `json:"logFormat"`
	TridentImage              string   `json:"tridentImage"`
	ImageRegistry             string   `json:"imageRegistry"`
	KubeletDir                string   `json:"kubeletDir"`
	ImagePullSecrets          []string `json:"imagePullSecrets"`
}
