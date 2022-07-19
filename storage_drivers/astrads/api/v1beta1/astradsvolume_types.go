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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AstraDSVolumeType indicates the type of Volume(RW, DP)
type AstraDSVolumeType string

// AstraDSVolumeConditionType indicates the type of condition occurred on a volume
type AstraDSVolumeConditionType string

const (
	AstraDSVolumeTypeReadWrite     AstraDSVolumeType = "ReadWrite"
	AstraDSVolumeTypDataProtection AstraDSVolumeType = "DataProtection"

	AstraDSVolumeCloudSnapshotPreparing    AstraDSVolumeConditionType = "CloudSnapshotPreparing"
	AstraDSVolumeCloudSnapshotRestoring    AstraDSVolumeConditionType = "CloudSnapshotRestoring"
	AstraDSVolumeCloudSnapshotRestored     AstraDSVolumeConditionType = "CloudSnapshotRestored"
	AstraDSVolumeCloudSnapshotRestoreError AstraDSVolumeConditionType = "CloudSnapshotRestoreError"
	// AstraDSVolumeCreated represents a condition detailing the status of a volume being created
	AstraDSVolumeCreated AstraDSVolumeConditionType = "AstraDSVolumeCreated"
	// AstraDSVolumeOnline represents a condition detailing the status of a volume being online
	AstraDSVolumeOnline AstraDSVolumeConditionType = "AstraDSVolumeOnline"
	// AstraDSVolumeDeleted represents a condition detailing the status of a volume being deleted
	AstraDSVolumeDeleted         AstraDSVolumeConditionType = "AstraDSVolumeDeleted"
	NetAppVolumeSnapshotRestored AstraDSVolumeConditionType = "VolumeSnapshotRestored"
	// AstraDSVolumeModifyError represents a condition detailing the status of properties of a volume being modified
	AstraDSVolumeModifyError AstraDSVolumeConditionType = "ModifyError"
)

// AstraDSVolumeSpec defines the desired state of AstraDSVolume
type AstraDSVolumeSpec struct {
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern=`^(([0-9]+(\.[0-9]*)?)|(\.[0-9]+))(([KMGTPE]i)|[numkMGTPE])?$`
	// Size is the size of the volume
	Size resource.Quantity `json:"size"`
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern=`^(ReadWrite|DataProtection)$`
	// +optional
	Type AstraDSVolumeType `json:"type,omitempty"`
	// Export policy is the name of the AstraDSExportPolicy object to associate with this volume
	ExportPolicy   string `json:"exportPolicy,omitempty"`
	SnapshotPolicy string `json:"snapshotPolicy,omitempty"`
	// +kubebuilder:default:="bronze"
	// QosPolicy is the name of the AstraDSQosPolicy to associate with this volume <br> (___default:___ `bronze`)
	// +optional
	QosPolicy string `json:"qosPolicy,omitempty"`
	// +kubebuilder:validation:Pattern=`^\/.*$|^$`
	// VolumePath defines the exposed NFS mount path for this volume
	VolumePath string `json:"volumePath"`
	// +kubebuilder:validation:Pattern=`^(online|offline)$`
	// State sets the state of a volume. <br> (___Example:___ `online` or `offline`)
	State string `json:"state,omitempty"`
	// DisplayName defines an additional name field for this volume that will be displayed in the ACC UI
	// +optional
	DisplayName string `json:"displayName,omitempty"`
	// +kubebuilder:validation:Pattern=`^[0-7]{0,1}[0-7]{3}$`
	// Permissions defines the unix permissions for this volume <br> (___Example:___ `0777`)
	Permissions string `json:"permissions,omitempty"`
	// NoSnapDir defines a setting to disable the default .snap directory at the root of this volume that allows access of all completed snapshots <br> (___default:___ `false`)
	// +optional
	NoSnapDir bool `json:"noSnapDir,omitempty"`
	// SnapshotReservePercent defines the percent of the volume to reserve for snapshot storage <br> (___default:___ `5`)
	// +kubebuilder:validation:Type=integer
	// +kubebuilder:validation:Maximum:=90
	// +kubebuilder:validation:Minimum:=0
	// +kubebuilder:default:=5
	// +optional
	SnapshotReservePercent *int32 `json:"snapshotReservePercent,omitempty"`
	CloudSnapshot          string `json:"cloudSnapshot,omitempty"`
	// CloneVolume defines a AstraDSVolume to clone from when creating this volume
	// +optional
	CloneVolume     string `json:"cloneVolume,omitempty"`
	RestoreSnapshot string `json:"restoreSnapshot,omitempty"`
	Cluster         string `json:"cluster"`
	// CloneSnapshot defines a AstraDSVolumeSnapshot to clone from when creating this volume
	// +optional
	CloneSnapshot string `json:"cloneSnapshot,omitempty"`
	// Define the custom labels on volume. These labels will be then added
	// to the "labels" in volume metrics.
	MetricsLabels *MetricsLabels `json:"metricsLabels,omitempty"`
}

type MetricsLabels struct {
	// Label 1 for volume metric
	Custom1 string `json:"custom1,omitempty"`
	// Label 2 for volume metric
	Custom2 string `json:"custom2,omitempty"`
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
	// InternalName defines the mapped internal name for this volume inside Firetap
	InternalName string `json:"internalName,omitempty"`
	// DisplayName defines the current displayName of the volume
	DisplayName string `json:"displayName,omitempty"`
	// Created defines if this volume has been successfully created
	// +kubebuilder:default:=false
	Created bool `json:"created,omitempty"`
	// Cluster defines the current cluster of the volume
	Cluster string `json:"cluster,omitempty"`
	// MIP defines the management IP that this volume is currently on
	MIP string `json:"mip,omitempty"`
	// State defines the current state of the volume
	State string `json:"state,omitempty"`
	// QosPolicy defines the current QosPolicy of a volume
	QosPolicy string `json:"qosPolicy,omitempty"`
	// Size defines the current size of the volume
	Size resource.Quantity `json:"size,omitempty"`
	// RequestedSize keeps track of reconciling spec.Size since firetap can return a slightly different one
	RequestedSize resource.Quantity `json:"requestedSize,omitempty"`
	// VolumeUUID is the current UUID of this volume
	VolumeUUID string `json:"volumeUUID,omitempty"`
	// ExportAddress is the current IP used to access the volume
	ExportAddress string `json:"exportAddress,omitempty"`
	// ExportPolicy is the current export policy for this volume
	ExportPolicy    string `json:"exportPolicy,omitempty"`
	SnapshotPolicy  string `json:"snapshotPolicy,omitempty"`
	RestoreSnapshot string `json:"restoreSnapshot,omitempty"`
	// NoSnapDir is current state of the NoSnapDir setting
	// +kubebuilder:default:=false
	NoSnapDir bool `json:"noSnapDir,omitempty"`
	// SnapshotReservePercent is the current reserve percent for snapshots on this volume
	// +kubebuilder:default:=0
	SnapshotReservePercent int32 `json:"snapshotReservePercent,omitempty"`
	// CloneVolume is the volume that was cloned to create this volume, if applicable
	CloneVolume string `json:"cloneVolume,omitempty"`
	// CloneSnapshot is the snapshot that was cloned to create this volume, if applicable
	CloneSnapshot    string            `json:"cloneSnapshot,omitempty"`
	RestoreCacheSize resource.Quantity `json:"restoreCacheSize,omitempty"`
	RestorePercent   int64             `json:"restorePercent,omitempty"`
	// Condition are the latest observations of the AstraDSVolume state
	Conditions []AstraDSVolumeCondition `json:"conditions,omitempty"`
	// VolumePath is the current state of the volume path
	// +kubebuilder:default:=""
	VolumePath string `json:"volumePath,omitempty"`
	// Permissions are the current state of UNIX permissions for this volume
	Permissions string `json:"permissions,omitempty"`
	// NodenName is the current node the volume resides on
	NodeName string `json:"nodeName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
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
