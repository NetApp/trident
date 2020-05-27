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
	Debug            bool     `json:"debug"`
	IPv6             bool     `json:"IPv6,omitempty"`
	Uninstall        bool     `json:"uninstall,omitempty"`
	LogFormat        string   `json:"logFormat,omitempty"`
	TridentImage     string   `json:"tridentImage,omitempty"`
	ImageRegistry    string   `json:"imageRegistry,omitempty"`
	KubeletDir       string   `json:"kubeletDir,omitempty"`
	Wipeout          []string `json:"wipeout,omitempty"`
	ImagePullSecrets []string `json:"imagePullSecrets,omitempty"`
}

// TridentProvisionerStatus defines the observed state of TridentProvisioner
type TridentProvisionerStatus struct {
	Message string `json:"message"`
	Status  string `json:"status"`
	Version string `json:"version"`
}

