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

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AstraDSFailedDriveState string

const (
	AstraDSFailedDriveStateNew      = AstraDSFailedDriveState("New")
	AstraDSFailedDriveStateReady    = AstraDSFailedDriveState("ReadyToReplace")
	AstraDSFailedDriveStateProgress = AstraDSFailedDriveState("InProgress")
	AstraDSFailedDriveStateDone     = AstraDSFailedDriveState("Replaced")
)

// AstraDSFailedDriveConditionType indicates the type of condition occurred on a volume
type AstraDSFailedDriveConditionType string

const (
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
	ExecuteReplace bool   `json:"executeReplace"`
	ReplaceWith    string `json:"replaceWith"`
}

// AstraDSFailedDrivestatus defines the observed state of AstraDSFailedDrive
type AstraDSFailedDriveStatus struct {
	FailedDriveInfo FailedDriveInfo               `json:"failedDriveInfo"`
	Cluster         string                        `json:"cluster"`
	State           AstraDSFailedDriveState       `json:"state"`
	Conditions      []AstraDSFailedDriveCondition `json:"conditions,omitempty"`
}

type FailedDriveInfo struct {
	InUse         bool   `json:"inUse"`
	Present       bool   `json:"present"`
	Name          string `json:"name"`
	FiretapUUID   string `json:"firetapUUID"`
	Serial        string `json:"serial"`
	Path          string `json:"path"`
	FailureReason string `json:"failureReason"`
	Node          string `json:"node"`
	SizeBytes     int    `json:"sizeBytes"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
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
