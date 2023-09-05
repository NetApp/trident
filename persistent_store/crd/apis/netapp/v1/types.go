// Copyright 2023 NetApp, Inc. All Rights Reserved.

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
	GetKind() string
	GetFinalizers() []string
	HasTridentFinalizers() bool
	RemoveTridentFinalizers()
}

// TridentVolumePublication tracks volumes the Trident controller has been asked to publish.
// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentVolumePublication struct {
	metav1.TypeMeta `json:",inline"`
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// VolumeID is the ID of the volume as given by the CO
	VolumeID string `json:"volumeID"`
	// NodeID is the ID of the node as given by the CO
	NodeID string `json:"nodeID"`
	// ReadOnly indicates that the ControllerPublishVolume request had this flag value
	ReadOnly bool `json:"readOnly"`
	// AccessMode describes how the CO intends to use the volume
	AccessMode int32 `json:"accessMode,omitempty"`
}

// TridentVolumePublicationList is a list of TridentVolumePublication objects.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentVolumePublicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of TridentVolumePublication objects
	Items []*TridentVolumePublication `json:"items"`
}

// TridentSnapshotInfo maps a k8s snapshot to the Trident internal snapshot.
// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentSnapshotInfo struct {
	metav1.TypeMeta `json:",inline"`
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Input spec for the Trident Snapshot
	Spec   TridentSnapshotInfoSpec   `json:"spec"`
	Status TridentSnapshotInfoStatus `json:"status"`
}

// TridentSnapshotInfoList is a list of TridentSnapshotInfo objects.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentSnapshotInfoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// List of TridentSnapshotInfo objects
	Items []*TridentSnapshotInfo `json:"items"`
}

// TridentSnapshotInfoSpec defines the desired state of TridentSnapshotInfo
type TridentSnapshotInfoSpec struct {
	SnapshotName string `json:"snapshotName"`
}

// TridentSnapshotInfoStatus defines the observed state of TridentSnapshotInfo
type TridentSnapshotInfoStatus struct {
	SnapshotHandle     string `json:"snapshotHandle"`
	LastTransitionTime string `json:"lastTransitionTime"`
	ObservedGeneration int    `json:"observedGeneration"`
}

// TridentMirrorRelationship defines a Trident Mirror relationship.
// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentMirrorRelationship struct {
	metav1.TypeMeta `json:",inline"`
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Input spec for the Trident mirror relationship
	Spec   TridentMirrorRelationshipSpec   `json:"spec"`
	Status TridentMirrorRelationshipStatus `json:"status"`
}

// TridentMirrorRelationshipList is a list of TridentBackend objects.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentMirrorRelationshipList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// List of TridentMirrorRelationship objects
	Items []*TridentMirrorRelationship `json:"items"`
}

// TridentMirrorRelationshipSpec defines the desired state of TridentMirrorRelationship
type TridentMirrorRelationshipSpec struct {
	MirrorState         string                                    `json:"state"`
	ReplicationPolicy   string                                    `json:"replicationPolicy"`
	ReplicationSchedule string                                    `json:"replicationSchedule"`
	VolumeMappings      []*TridentMirrorRelationshipVolumeMapping `json:"volumeMappings"`
}
type TridentMirrorRelationshipVolumeMapping struct {
	RemoteVolumeHandle     string `json:"remoteVolumeHandle"`
	LocalPVCName           string `json:"localPVCName"`
	PromotedSnapshotHandle string `json:"promotedSnapshotHandle"`
}
type TridentMirrorRelationshipCondition struct {
	MirrorState         string `json:"state"`
	Message             string `json:"message"`
	LastTransitionTime  string `json:"lastTransitionTime"`
	ObservedGeneration  int    `json:"observedGeneration"`
	LocalVolumeHandle   string `json:"localVolumeHandle"`
	LocalPVCName        string `json:"localPVCName"`
	RemoteVolumeHandle  string `json:"remoteVolumeHandle"`
	ReplicationPolicy   string `json:"replicationPolicy"`
	ReplicationSchedule string `json:"replicationSchedule"`
}

// TridentMirrorRelationshipStatus defines the observed state of TridentMirrorRelationship
type TridentMirrorRelationshipStatus struct {
	Conditions []*TridentMirrorRelationshipCondition `json:"conditions"`
}

const (
	TridentActionStateSucceeded  = "Succeeded"
	TridentActionStateInProgress = "In progress"
	TridentActionStateFailed     = "Failed"
)

// TridentActionMirrorUpdate defines an imperative action to update a mirror relationship
// on demand for a specified snapshot.
// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentActionMirrorUpdate struct {
	metav1.TypeMeta `json:",inline"`
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Input spec for TridentActionMirrorUpdate
	Spec TridentActionMirrorUpdateSpec `json:"spec"`

	// Completion status for TridentActionMirrorUpdate
	Status TridentActionMirrorUpdateStatus `json:"status"`
}

// TridentActionMirrorUpdateSpec defines the arguments of TridentActionMirrorUpdate
type TridentActionMirrorUpdateSpec struct {
	TMRName        string `json:"tridentMirrorRelationshipName"`
	SnapshotHandle string `json:"snapshotHandle"`
}

// TridentActionMirrorUpdateStatus defines the result of TridentActionMirrorUpdate
type TridentActionMirrorUpdateStatus struct {
	LocalVolumeHandle    string       `json:"localVolumeHandle"`
	RemoteVolumeHandle   string       `json:"remoteVolumeHandle"`
	SnapshotHandle       string       `json:"snapshotHandle,omitempty"`
	State                string       `json:"state,omitempty"`
	Message              string       `json:"message,omitempty"`
	CompletionTime       *metav1.Time `json:"completionTime,omitempty"`
	PreviousTransferTime *metav1.Time `json:"previousTransferTime,omitempty"`
}

// TridentActionMirrorUpdateList is a list of TridentActionMirrorUpdate objects.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentActionMirrorUpdateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// List of TridentActionMirrorUpdate objects
	Items []*TridentActionMirrorUpdate `json:"items"`
}

// TridentBackendConfig defines a Trident backend.
// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentBackendConfig struct {
	metav1.TypeMeta `json:",inline"`
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Input spec for the Trident Backend
	Spec   TridentBackendConfigSpec   `json:"spec"`
	Status TridentBackendConfigStatus `json:"status"`
}

// TridentBackendConfigList is a list of TridentBackend objects.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentBackendConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// List of TridentBackendConfig objects
	Items []*TridentBackendConfig `json:"items"`
}

// TridentBackendConfigSpec defines the desired state of TridentBackendConfig
type TridentBackendConfigSpec struct {
	runtime.RawExtension
}

// TridentBackendConfigStatus defines the observed state of TridentBackendConfig
type TridentBackendConfigStatus struct {
	Message             string                          `json:"message"`
	BackendInfo         TridentBackendConfigBackendInfo `json:"backendInfo"`
	DeletionPolicy      string                          `json:"deletionPolicy"`
	Phase               string                          `json:"phase"`
	LastOperationStatus string                          `json:"lastOperationStatus"`
}

type TridentBackendConfigBackendInfo struct {
	BackendName string `json:"backendName"`
	BackendUUID string `json:"backendUUID"`
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
	// UserState records the TridentBackend's user-defined state
	UserState string `json:"userState"`
	// StateReason records the reason if TridentBackend's state is offline
	StateReason string `json:"stateReason,omitempty"`
	// ConfigRef is a reference to the TridentBackendConfig object
	ConfigRef string `json:"configRef"`
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
	// Transaction is the transaction struct
	Transaction runtime.RawExtension `json:"transaction"`
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
	// NQN is the nqn of the node
	NQN string `json:"nqn,omitempty"`
	// IPs is a list of IP addresses for the TridentNode
	IPs []string `json:"ips,omitempty"`
	// NodePrep is the current status of node preparation for this node
	NodePrep runtime.RawExtension `json:"nodePrep,omitempty"`
	// HostInfo contains information about the node's host machine
	HostInfo runtime.RawExtension `json:"hostInfo,omitempty"`
	// Deleted indicates that Trident received an event that the node has been removed
	Deleted bool `json:"deleted"`
	// PublicationState indicates whether the node is safe for volume publications
	PublicationState string `json:"publicationState"`
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
	// PublicationsSynced indicates if Trident has done an initial syncing of the publication objects with the
	// container orchestrator
	PublicationsSynced bool `json:"publications_synced,omitempty"`
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
	// State records the TridentVolume's state
	State string `json:"state"`
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

// TridentVolumeReference defines a PVC whose backing volume Trident may share to other namespaces.
// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentVolumeReference struct {
	metav1.TypeMeta `json:",inline"`
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Input spec for the Trident Volume Reference
	Spec TridentVolumeReferenceSpec `json:"spec"`
}

// TridentVolumeReferenceSpec defines the desired state of TridentVolumeReference
type TridentVolumeReferenceSpec struct {
	// PVCName is the name of the referenced PVC
	PVCName string `json:"pvcName"`
	// PVCNamespace is the namespace containing the referenced PVC
	PVCNamespace string `json:"pvcNamespace"`
}

// TridentVolumeReferenceList is a list of TridentVolumeReference objects.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentVolumeReferenceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// List of TridentVolumeReference objects
	Items []*TridentVolumeReference `json:"items"`
}

// TridentActionSnapshotRestore defines an imperative action to restore a volume to a snapshot.
// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentActionSnapshotRestore struct {
	metav1.TypeMeta `json:",inline"`
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Input spec for TridentActionSnapshotRestore
	Spec TridentActionSnapshotRestoreSpec `json:"spec"`

	// Completion status for TridentActionSnapshotRestore
	Status TridentActionSnapshotRestoreStatus `json:"status"`
}

// TridentActionSnapshotRestoreSpec defines the arguments of TridentActionSnapshotRestore
type TridentActionSnapshotRestoreSpec struct {
	// PVCName is the name of the PVC (not the PV) whose bound volume is to be restored
	PVCName string `json:"pvcName"`
	// VolumeSnapshotName is the name of the volume snapshot (not the VSC) to restore
	VolumeSnapshotName string `json:"volumeSnapshotName"`
}

// TridentActionSnapshotRestoreStatus defines the result of TridentActionSnapshotRestore
type TridentActionSnapshotRestoreStatus struct {
	State          string       `json:"state,omitempty"`
	Message        string       `json:"message,omitempty"`
	StartTime      *metav1.Time `json:"startTime,omitempty"`
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

// TridentActionSnapshotRestoreList is a list of TridentActionSnapshotRestore objects.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentActionSnapshotRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// List of TridentActionSnapshotRestore objects
	Items []*TridentActionSnapshotRestore `json:"items"`
}
