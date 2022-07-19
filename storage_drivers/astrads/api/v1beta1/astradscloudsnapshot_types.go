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

// +kubebuilder:validation:Enum=Azure_Cloud;GoogleCloud;AWS_S3;SGWS;ONTAPS3
type AstraDSCloudSnapshotProviderType string

// Valid provider types for cloud
const (
	AzureCloud  AstraDSCloudSnapshotProviderType = "Azure_Cloud"
	GoogleCloud AstraDSCloudSnapshotProviderType = "GoogleCloud"
	AWSS3       AstraDSCloudSnapshotProviderType = "AWS_S3"
	SGWS        AstraDSCloudSnapshotProviderType = "SGWS"
	ONTAPS3     AstraDSCloudSnapshotProviderType = "ONTAPS3"
)

// AstraDSCloudSnapshotSpec defines the desired state of AstraDSCloudSnapshot
type AstraDSCloudSnapshotSpec struct {
	DisplayName  string                           `json:"displayName,omitempty"`
	Imported     bool                             `json:"imported,omitempty"`
	Container    string                           `json:"container"`
	Server       string                           `json:"server"`
	Port         int32                            `json:"port"`
	ProviderType AstraDSCloudSnapshotProviderType `json:"providerType"`
	SecretRef    v1.SecretReference               `json:"secretsRef"`
}

// AstraDSCloudSnapshotConditionType indicates the type of condition occurred on an AstraDSCloudSnapshot
type AstraDSCloudSnapshotConditionType string

// Valid conditions for a AstraDSVolumeCloudSnapshot
const (
	CloudSnapshotReady    AstraDSCloudSnapshotConditionType = "CloudSnapshotReady"
	CloudSnapshotCreating AstraDSCloudSnapshotConditionType = "CloudSnapshotCreating"
	CloudSnapshotDeleting AstraDSCloudSnapshotConditionType = "CloudSnapshotDeleting"
	CloudSnapshotError    AstraDSCloudSnapshotConditionType = "CloudSnapshotError"
)

// AstraDSCloudSnapshotCondition contains the condition information for an AstraDSCloudSnapshot
type AstraDSCloudSnapshotCondition struct {
	// Type of AstraDSVolumeCloudSnapshot condition.
	Type AstraDSCloudSnapshotConditionType `json:"type"`
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

// AstraDSCloudSnapshotStatus defines the observed state of AstraDSCloudSnapshot
type AstraDSCloudSnapshotStatus struct {
	Conditions        []AstraDSCloudSnapshotCondition `json:"conditions"`
	EndpointUUID      string                          `json:"endpointUUID,omitempty"`
	BytesTransferred  int64                           `json:"bytesTransferred"`
	LogicalSize       int64                           `json:"logicalSize,omitempty"`
	CompletionPercent int64                           `json:"completionPercent"`
	CreationTime      metav1.Time                     `json:"creationTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:shortName=adscs,categories={ads,all}

// AstraDSCloudSnapshot is the Schema for the astradscloudsnapshots API
type AstraDSCloudSnapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AstraDSCloudSnapshotSpec   `json:"spec,omitempty"`
	Status AstraDSCloudSnapshotStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AstraDSCloudSnapshotList contains a list of AstraDSCloudSnapshot
type AstraDSCloudSnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AstraDSCloudSnapshot `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AstraDSCloudSnapshot{}, &AstraDSCloudSnapshotList{})
}
