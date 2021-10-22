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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//AstraDSVolumeType indicates the type of Volume(RW, DP)
type AstraDSVolumeType string

//AstraDSVolumeConditionType indicates the type of condition occurred on a volume
type AstraDSVolumeConditionType string

const (
	AstraDSVolumeTypeReadWrite     AstraDSVolumeType = "ReadWrite"
	AstraDSVolumeTypDataProtection AstraDSVolumeType = "DataProtection"

	AstraDSVolumeCloudSnapshotPreparing    AstraDSVolumeConditionType = "CloudSnapshotPreparing"
	AstraDSVolumeCloudSnapshotRestoring    AstraDSVolumeConditionType = "CloudSnapshotRestoring"
	AstraDSVolumeCloudSnapshotRestored     AstraDSVolumeConditionType = "CloudSnapshotRestored"
	AstraDSVolumeCloudSnapshotRestoreError AstraDSVolumeConditionType = "CloudSnapshotRestoreError"
	AstraDSVolumeCreated                   AstraDSVolumeConditionType = "AstraDSVolumeCreated"
	AstraDSVolumeOnline                    AstraDSVolumeConditionType = "AstraDSVolumeOnline"
	AstraDSVolumeDeleted                   AstraDSVolumeConditionType = "AstraDSVolumeDeleted"
	NetAppVolumeSnapshotRestored           AstraDSVolumeConditionType = "VolumeSnapshotRestored"
	AstraDSVolumeModifyError               AstraDSVolumeConditionType = "ModifyError"
)

// AstraDSVolumeSpec defines the desired state of AstraDSVolume
type AstraDSVolumeSpec struct {

	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern=`^(([0-9]+(\.[0-9]*)?)|(\.[0-9]+))(([KMGTPE]i)|[numkMGTPE])?$`
	Size resource.Quantity `json:"size"`
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern=`^(ReadWrite|DataProtection)$`
	Type           AstraDSVolumeType `json:"type,omitempty"`
	ExportPolicy   string            `json:"exportPolicy,omitempty"`
	SnapshotPolicy string            `json:"snapshotPolicy,omitempty"`
	QosPolicy      string            `json:"qosPolicy,omitempty"`
	// +kubebuilder:validation:Pattern=`^\/.*$`
	VolumePath  string `json:"volumePath"`
	DisplayName string `json:"displayName,omitempty"`
	// +kubebuilder:validation:Pattern=`^[0][0-7]{3}$`
	Permissions string `json:"permissions,omitempty"`
	NoSnapDir   bool   `json:"noSnapDir,omitempty"`
	// +kubebuilder:validation:Type=integer
	// +kubebuilder:validation:Maximum:=100
	// +kubebuilder:validation:ExclusiveMaximum:=true
	// +kubebuilder:validation:Minimum:=0
	// +kubebuilder:validation:default:=5
	SnapshotReservePercent *int32 `json:"snapshotReservePercent,omitempty"`
	CloudSnapshot          string `json:"cloudSnapshot,omitempty"`
	CloneVolume            string `json:"cloneVolume,omitempty"`
	RestoreSnapshot        string `json:"restoreSnapshot,omitempty"`
	Cluster                string `json:"cluster"`
	CloneSnapshot          string `json:"cloneSnapshot,omitempty"`
}

// AstraDSVolumeCondition contains the condition information for a AstraDSVolume
type AstraDSVolumeCondition struct {
	// Type of AstraDSVolume condition.
	Type AstraDSVolumeConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status"`
	// Last time we got an update on a given condition.
	// +optional
	LastHeartbeatTime *metav1.Time `json:"lastHeartbeatTime,omitempty"`
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

// AstraDSVolumeStatus defines the observed state of AstraDSVolume
type AstraDSVolumeStatus struct {
	InternalName string            `json:"internalName,omitempty"`
	DisplayName  string            `json:"displayName,omitempty"`
	Created      bool              `json:"created,omitempty"`
	Cluster      string            `json:"cluster,omitempty"`
	MIP          string            `json:"mip,omitempty"`
	State        string            `json:"state,omitempty"`
	QosPolicy    string            `json:"qosPolicy,omitempty"`
	Size         resource.Quantity `json:"size,omitempty"`
	// RequestedSize keeps track of reconciling spec.Size since firetap can return a slightly different one
	RequestedSize          resource.Quantity        `json:"requestedSize,omitempty"`
	VolumeUUID             string                   `json:"volumeUUID,omitempty"`
	ExportAddress          string                   `json:"exportAddress,omitempty"`
	ExportPolicy           string                   `json:"exportPolicy,omitempty"`
	SnapshotPolicy         string                   `json:"snapshotPolicy,omitempty"`
	RestoreSnapshot        string                   `json:"restoreSnapshot,omitempty"`
	NoSnapDir              bool                     `json:"noSnapDir,omitempty"`
	SnapshotReservePercent int32                    `json:"snapshotReservePercent,omitempty"`
	CloneVolume            string                   `json:"cloneVolume,omitempty"`
	CloneSnapshot          string                   `json:"cloneSnapshot,omitempty"`
	RestoreCacheSize       resource.Quantity        `json:"restoreCacheSize,omitempty"`
	RestorePercent         int64                    `json:"restorePercent,omitempty"`
	Conditions             []AstraDSVolumeCondition `json:"conditions,omitempty"`
	VolumePath             string                   `json:"volumePath,omitempty"`
	Permissions            string                   `json:"permissions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="SIZE",type="string",JSONPath=".status.size",description="The size of the volume (default unit in bytes)"
// +kubebuilder:printcolumn:name="IP",type="string",JSONPath=".status.exportAddress",description="The IP that the volume can be mounted from"
// +kubebuilder:printcolumn:name="CLUSTER",type="string",JSONPath=".status.cluster",description="The cluster that the volume exists in"
// +kubebuilder:printcolumn:name="EXPORTPOLICY",type="string",JSONPath=".status.exportPolicy",description="Export policy for the volume",priority=1
// +kubebuilder:printcolumn:name="CREATED",type="string",JSONPath=".status.created",description="Is the underlying firetap volume created"
// +kubebuilder:resource:shortName=adsvo,categories={ads,all}

// AstraDSVolume is the Schema for the astradsvolumes API
type AstraDSVolume struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AstraDSVolumeSpec   `json:"spec,omitempty"`
	Status AstraDSVolumeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AstraDSVolumeList contains a list of AstraDSVolume
type AstraDSVolumeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AstraDSVolume `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AstraDSVolume{}, &AstraDSVolumeList{})
}
