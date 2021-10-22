/*
Copyright 2021 NetApp Inc..

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AstraDSNfsOptionSpec defines the desired state of AstraDSNfsOption
type AstraDSNfsOptionSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	NfsAccess         bool   `json:"nfsAccess,omitempty"`
	NfsV3Enable       bool   `json:"nfsV3Enable,omitempty"`
	NfsV40Enable      bool   `json:"nfsV40Enable,omitempty"`
	UdpEnable         bool   `json:"udpEnable,omitempty"`
	TcpEnable         bool   `json:"tcpEnable,omitempty"`
	NfsV41Enable      bool   `json:"nfsV41Enable,omitempty"`
	MountrootOnly     bool   `json:"mountrootOnly,omitempty"`
	ShowMount         bool   `json:"showMount,omitempty"`
	ShowMountRootOnly bool   `json:"showMountRootOnly,omitempty"`
	Cluster           string `json:"cluster"`
}

// AstraDSNfsOptionStatus defines the observed state of AstraDSNfsOption
type AstraDSNfsOptionStatus struct {
	Cluster string `json:"cluster"`
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=adsnf,categories={ads,all}
// AstraDSNfsOption is the Schema for the astradsnfsoptions API
type AstraDSNfsOption struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AstraDSNfsOptionSpec   `json:"spec,omitempty"`
	Status AstraDSNfsOptionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AstraDSNfsOptionList contains a list of AstraDSNfsOption
type AstraDSNfsOptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AstraDSNfsOption `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AstraDSNfsOption{}, &AstraDSNfsOptionList{})
}
