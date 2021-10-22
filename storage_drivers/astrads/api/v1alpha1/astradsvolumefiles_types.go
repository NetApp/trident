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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AstraDSVolumeFilesSpec defines the desired state of AstraDSVolumeFiles
type AstraDSVolumeFilesSpec struct {
	Files   []VolumeFileSpec `json:"files,omitempty"`
	Cluster string           `json:"cluster"`
}

// +kubebuilder:validation:Enum=directory;file
type FileType string

const (
	Directory FileType = "directory"
	File      FileType = "file"
)

// VolumeFile represents a file within a volume
type VolumeFileSpec struct {
	// The type of the file
	Type FileType `json:"type"`
	// The absolute path of the file relative to volume root
	// +kubebuilder:validation:MaxLength=1023
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`
	// Permissions for the file
	// +kubebuilder:validation:Pattern=`^[0][0-7]{3}$`
	UnixPermissions string `json:"unixPermissions,omitempty"`
	// The size of the file. Can be specified for a resize.
	Size *resource.Quantity `json:"size,omitempty"`
	// Specify if the file will display metdata fields in the status
	Metadata bool `json:"metadata,omitempty"`
}

// AstraDSVolumeFilesConditionType indicates the type of condition occurring on a AstraDSVolumeFiles
type AstraDSVolumeFilesConditionType string

// Valid conditions for AstraDSVolumeFiles
const (
	FileOperationsHealthy AstraDSVolumeFilesConditionType = "FileOperationsSuccess"
)

// AstraDSVolumeFilesCondition contains the condition information for AstraDSVolumeFiles
type AstraDSVolumeFilesCondition struct {
	// Type of AstraDSVolumeFiles condition.
	Type AstraDSVolumeFilesConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
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

// AstraDSVolumeFilesConditionType indicates the type of condition occurring on a AstraDSVolumeFiles
type VolumeFileErrorReason string

// Valid conditions for AstraDSVolumeFiles
const (
	// If a directory is up for deletion, it must not contain any files. If it does, it cannot
	// be deleted.
	DirectoryContainsFiles VolumeFileErrorReason = "DirectoryContainsFiles"
)

// Represents a volume file that encountered an error during a specific operation.
type VolumeFileError struct {
	// The absolute path of the file relative to volume root
	// +kubebuilder:validation:MaxLength=1023
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`
	// The type of the file
	Type FileType `json:"type"`
	// The reason the file failed. Can be used by clients as an enum to
	// decide what to do.
	Reason VolumeFileErrorReason `json:"reason,omitempty"`
	// The message from the component that failed
	Message string `json:"message"`
	// Any error codes that may be associated with the error
	Code *int32 `json:"code,omitempty"`
}

func (e *VolumeFileError) Error() string {
	if e.Code != nil {
		return fmt.Sprintf("File '%v' of type %v encountered an error [code %v]: %v", e.Path, e.Type, e.Code, e.Message)
	}
	return fmt.Sprintf("File '%v' of type %v encountered an error: %v", e.Path, e.Type, e.Message)
}

// VolumeFile represents a file within a volume
type VolumeFileStatus struct {
	// The absolute path of the file relative to volume root
	// +kubebuilder:validation:MaxLength=1023
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`
	// Name of the file
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// The type of the file
	Type FileType `json:"type"`
	// The Size of the file
	Size *resource.Quantity `json:"size,omitempty"`
	// Metadata for the volume file
	FileMetadata *VolumeFileMetadata `json:"fileMetadata,omitempty"`
}

type VolumeFileMetadata struct {
	// Creation time of the file
	CreationTime metav1.Time `json:"CreationTime,omitempty"`
	// Last data modification time of the file
	ModificationTime metav1.Time `json:"modificationTime,omitempty"`
	// Last access time of the file
	AccessTime metav1.Time `json:"accessTime,omitempty"`
	// Unix Permissions of the file. UNIX permissions to be viewed as an octal number.
	// It consists of 4 digits derived by adding up bits 4 (read),
	// 2 (write) and 1 (execute). The first digit selects the set user ID(4),
	// set group ID (2) and sticky (1) attributes.
	// The second digit selects permission for the owner of the file;
	// the third selects permissions for other users in the same group;
	// the fourth for other users not in the group.
	// +kubebuilder:default:="0755"
	UnixPermissions string `json:"unixPermissions"`
	// Integer ID of the file owner
	// +kubebuilder:default:=0
	OwnerId int64 `json:"ownerId"`
	// Integer ID of the group of the file owner
	// +kubebuilder:default:=0
	GroupId int64 `json:"groupId"`
}

// AstraDSVolumeFilesStatus defines the observed state of AstraDSVolumeFiles
type AstraDSVolumeFilesStatus struct {
	// Volume file conditions
	Conditions []AstraDSVolumeFilesCondition `json:"conditions,omitempty"`
	// Volume files status collection
	Files []VolumeFileStatus `json:"files,omitempty"`
	// Volume files that have encountered an error
	FileErrors []VolumeFileError `json:"fileErrors,omitempty"`
	Cluster    string            `json:"cluster"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=adsvf,categories={ads,all}

// AstraDSVolumeFiles is the Schema for the astradsvolumesfiles API
type AstraDSVolumeFiles struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AstraDSVolumeFilesSpec   `json:"spec,omitempty"`
	Status AstraDSVolumeFilesStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AstraDSVolumeFilesList contains a list of AstraDSVolumeFiles
type AstraDSVolumeFilesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AstraDSVolumeFiles `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AstraDSVolumeFiles{}, &AstraDSVolumeFilesList{})
}
