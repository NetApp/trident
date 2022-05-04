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

// AstraDSNodeInfoSpec defines the desired state of AstraDSNodeInfo
type AstraDSNodeInfoSpec struct{}

// AstraDSNodeInfoStatus defines the observed state of AstraDSNodeInfo
type AstraDSNodeInfoStatus struct {
	// Number of logical CPUs
	CPUCount int64       `json:"cpuCount"`
	Hostname string      `json:"hostName,omitempty"`
	Drives   []DriveInfo `json:"drives"`
	// Is the host a virtual machine
	IsVirtual bool `json:"isVirtual,omitempty"`
	// Memory capacity in bytes
	MemoryBytes int64 `json:"memoryBytes"`
	// List of logical NICs present on the host
	Nics []NicInfo `json:"nics,omitempty"`
	// List of export addresses discovered on the host
	ExportAddresses []string `json:"exportAddresses,omitempty"`
}

// DriveInfo describes a single drive with identifying information to uniquely
// identify each drive. Drive partitions are listed in the Partitions field.
type DriveInfo struct {
	Model      string           `json:"model,omitempty"`
	Name       string           `json:"name"`
	ByIDPath   string           `json:"byIDPath,omitempty"`
	ByUUIDPath string           `json:"byUUIDPath,omitempty"`
	Rotational bool             `json:"rotational,omitempty"`
	Serial     string           `json:"serial,omitempty"`
	SizeBytes  int64            `json:"sizeBytes"`
	Vendor     string           `json:"vendor,omitempty"`
	BusType    string           `json:"busType,omitempty"`
	Partitions []DrivePartition `json:"partitions,omitempty"`
}

// NIC defines a single network interface.
type NicInfo struct {
	Addresses []string `json:"addresses,omitempty"`
	MTU       int      `json:"mtu"`
	Name      string   `json:"name"`
	SpeedMbit int64    `json:"speedmbit,omitempty"`
	Type      string   `json:"type"`
}

// DrivePartition describes a single partition of a drive.
type DrivePartition struct {
	PartitionName string `json:"partitionName"`
	Mounted       bool   `json:"mounted"`
	ByIDPath      string `json:"byIDPath,omitempty"`
	ByUUIDPath    string `json:"byUUIDPath,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=adsni,categories={ads,all}

// AstraDSNodeInfo is the Schema for the astradsnodeinfoes API
type AstraDSNodeInfo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AstraDSNodeInfoSpec   `json:"spec,omitempty"`
	Status AstraDSNodeInfoStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AstraDSNodeInfoList contains a list of AstraDSNodeInfo
type AstraDSNodeInfoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AstraDSNodeInfo `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AstraDSNodeInfo{}, &AstraDSNodeInfoList{})
}
