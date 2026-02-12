// Copyright 2025 NetApp, Inc. All Rights Reserved.

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/netapp/trident/utils/models"
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
	// AutogrowPolicy describes the Autogrow policy for the volume
	AutogrowPolicy string `json:"autogrowPolicy,omitempty"`
	// AutogrowIneligible indicates if volume is ineligible for autogrow monitoring
	AutogrowIneligible bool `json:"autogrowIneligible,omitempty"`
	// StorageClass is the name of the StorageClass used by this volume
	StorageClass string `json:"storageClass,omitempty"`
	// BackendUUID is the UUID of the backend hosting this volume
	BackendUUID string `json:"backendUUID,omitempty"`
	// Pool is the storage pool hosting this volume
	Pool string `json:"pool,omitempty"`
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
	TridentNodeRemediatingState  = "Remediating"
	NodeRecoveryPending          = "NodeRecoveryPending"
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

// TridentNodeRemediation defines a Trident node remediation
// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentNodeRemediation struct {
	metav1.TypeMeta `json:",inline"`
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TridentNodeRemediationSpec   `json:"spec,omitempty"`
	Status            TridentNodeRemediationStatus `json:"status,omitempty"`
}

// TridentNodeRemediationSpec defines the desired state of TridentNodeRemediation
type TridentNodeRemediationSpec struct{}

// TridentNodeRemediationStatus defines the observed state of TridentNodeRemediation
type TridentNodeRemediationStatus struct {
	State             string            `json:"state,omitempty"`
	CompletionTime    *metav1.Time      `json:"completionTime,omitempty"`
	Message           string            `json:"message,omitempty"`
	VolumeAttachments map[string]string `json:"volumeAttachments,omitempty"`
}

// TridentNodeRemediationList is a list of TridentNodeRemediation objects
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentNodeRemediationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []*TridentNodeRemediation `json:"items"`
}

// TridentNodeRemediationTemplate defines a Trident node remediation template
// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentNodeRemediationTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TridentNodeRemediationTemplateSpec   `json:"spec,omitempty"`
	Status            TridentNodeRemediationTemplateStatus `json:"status,omitempty"`
}

// TridentNodeRemediationTemplateSpec defines the desired state of TridentNodeRemediationTemplate
type TridentNodeRemediationTemplateSpec struct {
	Test     string               `json:"test,omitempty"`
	Template runtime.RawExtension `json:"template,omitempty"`
}

// TridentNodeRemediationTemplateStatus defines the observed state (can be empty if unused)
type TridentNodeRemediationTemplateStatus struct{}

// TridentNodeRemediationTemplateList is a list of TridentNodeRemediationTemplate objects
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentNodeRemediationTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TridentNodeRemediationTemplate `json:"items"`
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
	// AutogrowStatus tracks the autogrow feature state and history for this volume
	AutogrowStatus *models.VolumeAutogrowStatus `json:"autogrowStatus,omitempty"`
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
	// HostWWPNMap is the map of host WWPNs
	HostWWPNMap map[string][]string `json:"hostWWPNMap,omitempty"`
	// NodePrep is the current status of node preparation for this node
	NodePrep runtime.RawExtension `json:"nodePrep,omitempty"`
	// HostInfo contains information about the node's host machine
	HostInfo runtime.RawExtension `json:"hostInfo,omitempty"`
	// Deleted indicates that Trident received an event that the node has been removed
	Deleted bool `json:"deleted"`
	// PublicationState indicates whether the node is safe for volume publications
	PublicationState string `json:"publicationState"`
	// LogLevel indicates the log level which is currently set for the node
	LogLevel string `json:"logLevel"`
	// LogWorkflows indicates the selected workflows which are currently set for the node
	LogWorkflows string `json:"logWorkflows"`
	// LogLayers indicates the selected log layers which are currently set for the node
	LogLayers string `json:"logLayers"`
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
	// State records the TridentSnapshot's state
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

// TridentGroupSnapshot defines a Trident group snapshot.
// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentGroupSnapshot struct {
	metav1.TypeMeta `json:",inline"`
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of the group snapshot
	Spec runtime.RawExtension `json:"spec"`
	// List of Snapshot IDs in the group.
	Snapshots []string `json:"snapshotIDs"`
	// The UTC time that the group snapshot was created at in RFC3339 format
	Created string `json:"dateCreated,omitempty"`
}

// TridentGroupSnapshotList defines a list of Trident group snapshots.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentGroupSnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of TridentGroupSnapshot objects
	Items []*TridentGroupSnapshot `json:"items"`
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

/************************
* Trident Autogrow Policy
************************/

// +genclient
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// TridentAutogrowPolicy is the Schema for the tridentautogrowpolicies API
type TridentAutogrowPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TridentAutogrowPolicySpec   `json:"spec"`
	Status TridentAutogrowPolicyStatus `json:"status,omitempty"`
}

// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// TridentAutogrowPolicyList contains a list of TridentAutogrowPolicy
type TridentAutogrowPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []TridentAutogrowPolicy `json:"items"`
}

type TridentAutogrowPolicySpec struct {
	// UsedThreshold specifies when to trigger Autogrow.
	// Can be either:
	// - Percentage format: "80%" (triggers when volume reaches 80% usage)
	// - Absolute format: "50Gi" (triggers when volume has used 50Gi)
	UsedThreshold string `json:"usedThreshold"`

	// GrowthAmount specifies how much to grow the volume.
	// Can be either:
	// - Percentage format: "10%" (grow by 10% of current size)
	// - Absolute format: "5Gi" (grow by exactly 5Gi)
	// Default: "10%" if not specified
	GrowthAmount string `json:"growthAmount,omitempty"`

	// MaxSize specifies the maximum size the volume can grow to.
	// Must be a valid Kubernetes resource.Quantity (e.g., "100Gi", "1Ti")
	// Optional: if not specified, volume can grow indefinitely (subject to backend limits)
	MaxSize string `json:"maxSize,omitempty"`
}

// TridentAutogrowPolicyStatus defines the observed state of TridentAutogrowPolicy
type TridentAutogrowPolicyStatus struct {
	// State represents the current state of the policy.
	// Valid values: "Success", "Failed", "Deleting"
	State string `json:"state,omitempty"`

	// Message provides detailed information about the policy status.
	// This field contains error messages for failed validation or
	// information about volumes blocking deletion.
	Message string `json:"message,omitempty"`
}

/********************************
* Trident Autogrow Request Internal
********************************/

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// TridentAutogrowRequestInternal is the Schema for the tridentautogrowrequestinternals API
// This is a namespaced, ephemeral CRD that acts as a signaling mechanism between Trident node pods and controller pod
type TridentAutogrowRequestInternal struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TridentAutogrowRequestInternalSpec   `json:"spec"`
	Status TridentAutogrowRequestInternalStatus `json:"status,omitempty"`
}

// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// TridentAutogrowRequestInternalList contains a list of TridentAutogrowRequestInternal
type TridentAutogrowRequestInternalList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []TridentAutogrowRequestInternal `json:"items"`
}

type TridentAutogrowRequestInternalSpec struct {
	// Volume is the name of the PersistentVolume to be expanded
	Volume string `json:"volume"`

	// AutogrowPolicyRef identifies the TridentAutogrowPolicy governing this request
	AutogrowPolicyRef TridentAutogrowRequestInternalPolicyRef `json:"autogrowPolicyRef"`

	ObservedUsedPercent   float32 `json:"observedUsedPercent,omitempty"`   // Percent used that triggered this request (observability)
	ObservedUsedBytes     string  `json:"observedUsedBytes,omitempty"`     // Used bytes at request time (observability; optional)
	ObservedCapacityBytes string  `json:"observedCapacityBytes,omitempty"` // Total size host sees at request time; required, must be valid resource.Quantity > 0

	NodeName  string      `json:"nodeName,omitempty"`  // Node that created this request
	Timestamp metav1.Time `json:"timestamp,omitempty"` // When the threshold was breached (observability)
}

type TridentAutogrowRequestInternalPolicyRef struct {
	Name       string `json:"name"`
	Generation int64  `json:"generation"`
}

type TridentAutogrowRequestInternalStatus struct {
	Phase              string       `json:"phase,omitempty"`
	Message            string       `json:"message,omitempty"`
	FinalCapacityBytes string       `json:"finalCapacityBytes,omitempty"`
	ProcessedAt        *metav1.Time `json:"processedAt,omitempty"` // Time when TAGRI moved to terminal phase (Completed/Failed/Rejected)
	RetryCount         int          `json:"retryCount,omitempty"`
}
