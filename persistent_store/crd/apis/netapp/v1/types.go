// Copyright 2019 NetApp, Inc. All Rights Reserved.

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	TridentFinalizerName = "trident.netapp.io"
)

func GetTridentFinalizers() []string {
	return []string{
		TridentFinalizerName,
	}
}

// TridentCRD should be implemented for our CRD objects
type TridentCRD interface {
	GetObjectMeta() metav1.ObjectMeta
	GetFinalizers() []string
	HasTridentFinalizers() bool
	RemoveTridentFinalizers()
}

// TridentBackend defines a Trident backend.
// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentBackend struct {
	metav1.TypeMeta `json:",inline"`
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Config of the Trident backend
	Config runtime.RawExtension `json:"config"`
	// BackendName is the real name of the backend (metadata has restrictions)
	BackendName string `json:"backendName"`
	// BackendUUID is a unique identifier for this backend
	BackendUUID string `json:"backendUUID"`
	// Version is the version of the backend
	Version string `json:"version"`
	// Online defines if the backend is online
	Online bool `json:"online"`
	// State records the TridentBackend's state
	State string `json:"state"`
}

// TridentBackendList is a list of TridentBackend objects.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentBackendList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of TridentBackend objects
	Items []*TridentBackend `json:"items"`
}

// TridentVolume defines a Trident volume.
// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentVolume struct {
	metav1.TypeMeta `json:",inline"`
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Config is the Volumes Config
	Config runtime.RawExtension `json:"config"`
	// BackendUUID is the UUID of the TridentBackend object
	BackendUUID string `json:"backendUUID"`
	// Pool is the volumes pool
	Pool string `json:"pool"`
	// Orphaned defines if the backend is orphaned
	Orphaned bool `json:"orphaned"`
	// State records the TridentVolume's state
	State string `json:"state"`
}

// TridentVolumeList is a list of TridentVolume objects.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentVolumeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of TridentVolume objects
	Items []*TridentVolume `json:"items"`
}

// TridentStorageClass defines a Trident storage class.
// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentStorageClass struct {
	metav1.TypeMeta `json:",inline"`
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of the storage class
	Spec runtime.RawExtension `json:"spec"`
}

// TridentStorageClassList is a list of TridentStorageClass objects.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentStorageClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of TridentStorageClass objects
	Items []*TridentStorageClass `json:"items"`
}

// TridentTransaction defines a Trident transaction.
// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentTransaction struct {
	metav1.TypeMeta `json:",inline"`
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Operation is the volume transaction operation
	Operation string `json:"operation"`
	// Config is the volume config
	Config runtime.RawExtension `json:"config"`
}

// TridentTransactionList is a list of TridentTransaction objects.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentTransactionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of TridentTransaction objects
	Items []*TridentTransaction `json:"items"`
}

// TridentNode defines a Trident CSI node object.
// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentNode struct {
	metav1.TypeMeta `json:",inline"`
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Name is the name of the node
	NodeName string `json:"name"`
	// IQN is the iqn of the node
	IQN string `json:"iqn,omitempty"`
	// IPs is a list of IP addresses for the TridentNode
	IPs []string `json:"ips,omitempty"`
}

// TridentNodeList is a list of TridentNode objects.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of TridentNode objects
	Items []*TridentNode `json:"items"`
}

// TridentVersion defines a Trident version object.
// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentVersion struct {
	metav1.TypeMeta `json:",inline"`
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// TridentVersion is the currently installed release version of Trident
	TridentVersion string `json:"trident_version,omitempty"`
	// PersistentStoreVersion is the Trident persistent store schema version
	PersistentStoreVersion string `json:"trident_store_version,omitempty"`
	// OrchestratorAPIVersion is the Trident API version
	OrchestratorAPIVersion string `json:"trident_api_version,omitempty"`
}

// TridentVersionList is a list of TridentVersion objects.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentVersionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of TridentVersion objects
	Items []*TridentVersion `json:"items"`
}

// TridentSnapshot defines a Trident snapshot.
// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentSnapshot struct {
	metav1.TypeMeta `json:",inline"`
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of the snapshot
	Spec runtime.RawExtension `json:"spec"`
	// The UTC time that the snapshot was created, in RFC3339 format
	Created string `json:"dateCreated"`
	// The size of the volume at the time the snapshot was created
	SizeBytes int64 `json:"size"`
}

// TridentSnapshotList is a list of TridentSnapshot objects.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentSnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of TridentSnapshot objects
	Items []*TridentSnapshot `json:"items"`
}
