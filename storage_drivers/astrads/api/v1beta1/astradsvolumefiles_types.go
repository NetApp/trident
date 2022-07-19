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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AstraDSVolumeFilesSpec defines the desired state of AstraDSVolumeFiles
type AstraDSVolumeFilesSpec struct {
	Files []VolumeFileSpec `json:"files"`
	Sync  *bool            `json:"sync,omitempty"`
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
	// The desired path name of the file. Will always be an
	// absolute path to the root of the volume
	// +kubebuilder:validation:MaxLength=1023
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`
	// Optional path to sepcify for rename.
	// When attempting to rename a file 'from' one path 'to' another,
	// this field would be the path to rename 'from'.
	// +kubebuilder:validation:MaxLength=1023
	// +kubebuilder:validation:MinLength=1
	FromPath        *string          `json:"fromPath,omitempty"`
	FileRestoreSpec *FileRestoreSpec `json:"fileRestoreSpec,omitempty"`
	// Permissions for the file
	// +kubebuilder:validation:Pattern=`^[0][0-7]{3}$`
	UnixPermissions string `json:"unixPermissions,omitempty"`
	// The size of the file. Can be specified for a resize.
	Size *resource.Quantity `json:"size,omitempty"`
	// Specify if the file will display metdata fields in the status
	Metadata bool `json:"metadata,omitempty"`
	// The contents of the file
	Contents *string `json:"contents,omitempty"`
}

// FileRestoreSpec represents the spec to achieve file level restore within a volume
type FileRestoreSpec struct {
	FromPath     string `json:"fromPath"`
	SnapshotName string `json:"snapshotName"`
}

func (r *FileRestoreSpec) Equal(status *FileRestoreStatus) bool {
	return status != nil &&
		r.SnapshotName == status.SnapshotName &&
		r.FromPath == status.FromPath
}

// AstraDSVolumeFilesConditionType indicates the type of condition occurring on a AstraDSVolumeFiles
type AstraDSVolumeFilesConditionType string

// Valid conditions for AstraDSVolumeFiles
const (
	FileOperationsHealthy AstraDSVolumeFilesConditionType = "FileOperationsSuccess"
	SyncOperationSuccess  AstraDSVolumeFilesConditionType = "SyncOperationSuccess"
)

// AstraDSVolumeFilesCondition contains the condition information for AstraDSVolumeFiles
type AstraDSVolumeFilesCondition struct {
	// Type of AstraDSVolumeFiles condition.
	Type AstraDSVolumeFilesConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// Last time the condition transit from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// ObservedGeneration represents the .metadata.generation that the condition was set based upon.
	// For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date
	// with respect to the current state of the instance.
	// +kubebuilder:validation:Minimum=0
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
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
	// If a client attempts to write file contents to a directory
	DirectoryCannotHaveContents VolumeFileErrorReason = "DirectoryCannotHaveContents"
	// If a resize is attempted that would truncate the file, we will get into a continuous resize
	// rewrite loop. We can avoid this by not allowing a downsize of a file. Clients should
	// truncate the contents to handle a resize of this nature.
	DesiredSizeLessThanFileContents VolumeFileErrorReason = "DesiredSizeLessThanFileContents"
	// If the FileRestoreSpec field is missing. This error is unlikley to show up
	FileRestoreSpecMissing VolumeFileErrorReason = "FileRestoreSpecMissing"
	// For a successful rename, clients must reference a fromPath field in the status
	// IF this is not the case, emit error
	FromPathMissingInStatus VolumeFileErrorReason = "FromPathMissingInStatus"
	// For complex renames, the parent directory entries must be renamed along with the child entries
	FromPathMissingInParent VolumeFileErrorReason = "FromPathMissingInParent"
	// For complex renames, the child directory entries must be renamed along with the parent entries
	FromPathMissingInChild VolumeFileErrorReason = "FromPathMissingInChild"
	// A file entry must not have a fromPath field on create since there is no reason to perform a
	// rename. This error case is also to help block against invalid renames when the user references
	// a fromPath not in the status along with changing the path to a new name
	FromPathOnCreateError VolumeFileErrorReason = "FromPathOnCreateError"
	// The DMS API returned an error. Inspect the code/message on how to handle these specific errors.
	ADSInternalAPIError VolumeFileErrorReason = "ADSInternalAPIError"
	// The volume modify command returned a not found error when attempting to restore. Either
	// the snapshot does not exist or the file does not exist within a valid snapshot.
	SnapshotOrFileNotFound VolumeFileErrorReason = "SnapshotOrFileNotFound"
	// The volume modify command returned a not found error when attempting to restore. Either
	// the snapshot does not exist or the file does not exist within a valid snapshot.
	SnapshotRestoreFromExistingFile VolumeFileErrorReason = "SnapshotRestoreFromExistingFile"
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
	Reason VolumeFileErrorReason `json:"reason"`
	// The message from the component that failed
	Message string `json:"message"`
	// Any error codes that may be associated with the error
	Code *int32 `json:"code,omitempty"`
}

func (e *VolumeFileError) Error() string {
	if e.Code != nil {
		return fmt.Sprintf("File '%v' of type %v encountered an error [code: %v, reason: %v]: %v", e.Path, e.Type, *e.Code, e.Reason, e.Message)
	}
	return fmt.Sprintf("File '%v' of type %v encountered an error [reason: %v]: %v", e.Path, e.Type, e.Reason, e.Message)
}

// VolumeFile represents a file within a volume
type VolumeFileStatus struct {
	// The absolute path of the file relative to volume root
	// +kubebuilder:validation:MaxLength=1023
	// +kubebuilder:validation:MinLength=1
	Path              string             `json:"path"`
	FileRestoreStatus *FileRestoreStatus `json:"fileRestoreStatus,omitempty"`
	// Name of the file
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// The type of the file
	Type FileType `json:"type"`
	// The Size of the file
	// Currently, directories dont have a size
	Size *resource.Quantity `json:"size,omitempty"`
	// The number of bytes written to the file from the control plane.
	// This can differ from the size.
	BytesWritten *resource.Quantity `json:"bytesWritten,omitempty"`
	// Metadata for the volume file
	FileMetadata *VolumeFileMetadata `json:"fileMetadata,omitempty"`
}

// FileRestoreStatus represents the status of a file level restore within a volume
type FileRestoreStatus struct {
	FromPath     string `json:"fromPath"`
	SnapshotName string `json:"snapshotName"`
}

type VolumeFileMetadata struct {
	// Creation time of the file
	CreationTime metav1.Time `json:"creationTime"`
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
	// The contents of the file
	Contents string `json:"contents,omitempty"`
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
// +kubebuilder:storageversion
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
