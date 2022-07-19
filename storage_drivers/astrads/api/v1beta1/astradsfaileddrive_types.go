/*
Copyright 2020 NetApp Inc..

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

type AstraDSFailedDriveState string

const (
	// New is a state that represents a new failed drive, that is still in use and not ready to replace yet
	AstraDSFailedDriveStateNew = AstraDSFailedDriveState("New")
	// ReadyToReplace is a state that represents a failed drive that is ready for replacement
	AstraDSFailedDriveStateReady = AstraDSFailedDriveState("ReadyToReplace")
	// InProgress is a state that represents a failed drive undergoing replacement
	AstraDSFailedDriveStateProgress = AstraDSFailedDriveState("InProgress")
	// Replaced is a state that represents a finished and resolved failed drive
	AstraDSFailedDriveStateDone = AstraDSFailedDriveState("Replaced")
)

// AstraDSFailedDriveConditionType indicates the type of condition occurred on a volume
type AstraDSFailedDriveConditionType string

const (
	// Done is a condition that represents if a failed drive replacement is done
	AstraDSFailedDriveDone AstraDSFailedDriveConditionType = "Done"
)

type AstraDSFailedDriveCondition struct {
	// Type of AstraDSFailedDrive condition.
	Type AstraDSFailedDriveConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status"`
	// Last time we got an update on a given condition.
	// +optional
	LastHeartbeatTime metav1.Time `json:"lastHeartbeatTime,omitempty"`
	// Last time the condition transit from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// (brief) reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// AstraDSFailedDrivespec defines the desired state of AstraDSFailedDrive
type AstraDSFailedDriveSpec struct {
	// ExecuteReplace is set to true to start drive replacement
	ExecuteReplace bool `json:"executeReplace"`
	// ReplaceWith is the serial of the drive to replace the failed drive with
	ReplaceWith string `json:"replaceWith"`
}

// AstraDSFailedDrivestatus defines the observed state of AstraDSFailedDrive
type AstraDSFailedDriveStatus struct {
	// FailedDriveInfo contains information about the current status of the drive that failed
	FailedDriveInfo FailedDriveInfo `json:"failedDriveInfo"`
	// Cluster is the current ADS cluster of the failed drive
	Cluster string `json:"cluster"`
	// State is the current state of the failed drive
	State AstraDSFailedDriveState `json:"state"`
	// Condition are the latest observation of the FailedDrive state
	Conditions []AstraDSFailedDriveCondition `json:"conditions,omitempty"`
}

// FailedDriveInfo lists all of the attributes of the drive that has failed
type FailedDriveInfo struct {
	// InUse defines if the failed drive is still being used for IO by the cluster and can't be removed yet
	InUse bool `json:"inUse"`
	// Present defines if the failed drive is still inserted into its respective node in the cluster
	Present bool `json:"present"`
	// Name is the failed drive's device name
	Name string `json:"name"`
	// FiretapUUID is the internal UUID of the failed drive
	FiretapUUID string `json:"firetapUUID"`
	// Serial is the serial number of the failed drive
	Serial string `json:"serial"`
	// Path is the "by-id" path of the failed drive
	Path string `json:"path"`
	// FailureReason is the reported reason this drive failed
	FailureReason string `json:"failureReason"`
	// Node is the node name of the node this drive exists on
	Node string `json:"node"`
	// SizeBytes is the size of the failed drive
	SizeBytes int `json:"sizeBytes"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:shortName=adsfd,categories={ads,all}

// AstraDSFailedDrive is the Schema for the astradsfaileddrives API
type AstraDSFailedDrive struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AstraDSFailedDriveSpec   `json:"spec,omitempty"`
	Status AstraDSFailedDriveStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AstraDSFailedDriveList contains a list of AstraDSFailedDrive
type AstraDSFailedDriveList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AstraDSFailedDrive `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AstraDSFailedDrive{}, &AstraDSFailedDriveList{})
}
