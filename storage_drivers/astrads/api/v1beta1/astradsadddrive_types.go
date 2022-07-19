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

package v1beta1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AstraDSAddDriveState is the current state of an add drive request
type AstraDSAddDriveState string

const (
	// AstraDSAddDriveStateNew occurs when a AddDrive is new and has input that is not yet validated
	AstraDSAddDriveStateNew = AstraDSAddDriveState("New")
	// AstraDSAddDriveStateProgress occurs when an AddDrive is in the process of adding disks to the cluster
	AstraDSAddDriveStateProgress = AstraDSAddDriveState("InProgress")
	// AstraDSAddDriveStateDone occurs when the AddDrive is has succeeded and is complete
	AstraDSAddDriveStateDone = AstraDSAddDriveState("Done")
	// AstraDSAddDriveStateError occurs when the AddDrive has failed, due invalid input or a system error
	AstraDSAddDriveStateError = AstraDSAddDriveState("Error")
)

// AstraDSAddDriveSpec defines the desired state of AstraDSAddDrive
type AstraDSAddDriveSpec struct {
	// ExecuteAdd is set to true to begin the add drive process
	ExecuteAdd bool `json:"executeAdd,omitempty"`
	// Drives contain an array of drives to add to nodes in the cluster
	Drives []AddDriveData `json:"drives,omitempty"`
	// Cluster is the cluster name that AddDrive will act upon
	Cluster string `json:"cluster"`
}

// AddDriveData is a set of properties to uniquely identify a disk on a node
type AddDriveData struct {
	// NodeName is the firetap node name that a drive resides on
	NodeName string `json:"nodeName"`
	// Serial is the unique serial number of a drive
	Serial string `json:"serial,omitempty"`
	// DevName is the disk name on a node e.g. "sda"
	DevName string `json:"devName,omitempty"`
}

// AddDriveDataStatus is the current status of a single drive being added to the cluster
type AddDriveDataStatus struct {
	AddDriveData `json:",inline"`
	// Name is the device name of the drive
	Name string `json:"name"`
	// SizeBytes is the size of the drive
	SizeBytes int64 `json:"sizeBytes"`
	// ByIDPath is the by-id mapping of a drive on its node
	ByIDPath string `json:"byIDPath"`
	// State is the current state of a single drive being added
	State AstraDSAddDriveState `json:"state"`
}

// AstraDSAddDriveStatus defines the observed state of AstraDSAddDrive
type AstraDSAddDriveStatus struct {
	Conditions []AstraDSAddDriveCondition `json:"conditions,omitempty"`
	// Drives contains all of the statuses of drives being added for this request
	Drives []AddDriveDataStatus `json:"drives,omitempty"`
	// Cluster is the ADS cluster this object is for
	Cluster string `json:"cluster,omitempty"`
	// State is the overall state of the add drive request
	State AstraDSAddDriveState `json:"state,omitempty"`
}

// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:shortName=adsad,categories={ads,all}
// +kubebuilder:object:root=true
// AstraDSAddDrive is the Schema for the astradsadddrives API
type AstraDSAddDrive struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AstraDSAddDriveSpec   `json:"spec,omitempty"`
	Status AstraDSAddDriveStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// AstraDSAddDriveList contains a list of AstraDSAddDrive
type AstraDSAddDriveList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AstraDSAddDrive `json:"items"`
}
type AstraDSAddDriveConditionType string

const (
	// AstraDSAddDriveDone represents a drive add that is completely finished
	AstraDSAddDriveDone AstraDSAddDriveConditionType = "Done"
	// AstraDSAddDriveError represents a drive add that ran into an issue, due to invalid user input or a system error
	AstraDSAddDriveError AstraDSAddDriveConditionType = "Error"
)

// AutoSupportCondition defines the condition information for an AutoSupport
type AstraDSAddDriveCondition struct {
	// Type of AutoSupport condition
	Type AstraDSAddDriveConditionType `json:"type"`

	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status"`

	// LastHeartbeatTime is the timestamp of when the AutoSupport condition was last probed
	LastHeartbeatTime metav1.Time `json:"lastHeartbeatTime,omitempty"`

	// LastTransitionTime is the timestamp for when the AutoSupport last transitioned from one status to another
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// Reason is the (brief) reason for the condition's last transition
	Reason string `json:"reason,omitempty"`

	// Message is the Human-readable message indicating details about last transition
	Message string `json:"message,omitempty"`
}

func init() {
	SchemeBuilder.Register(&AstraDSAddDrive{}, &AstraDSAddDriveList{})
}
