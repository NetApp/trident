/*
Copyright 2021 NetApp Inc.

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

// AutoSupportSpec defines the desired state of AutoSupport
type AutoSupportSpec struct {
	AutoSupportSetting `json:",inline"`

	// Trigger is the trigger type of the AutoSupport instance. <br> (___default:___ `manual`)
	// +kubebuilder:validation:Enum=firetapEvent;coreEvent;k8sEvent;periodic;manual;userTrigger
	// +kubebuilder:default:=manual
	// +optional
	Trigger AutoSupportTrigger `json:"trigger"`
	// Name of the ADS Cluster.
	Cluster string `json:"cluster"`
	// Name of the ADS Node for Storage AutoSupport.
	// +kubebuilder:validation:Optional
	NodeName string `json:"nodeName"`
	// Case Number associated with the ASUP.
	// +kubebuilder:validation:Optional
	CaseNumber int64 `json:"caseNumber"`
}

// AutoSupportSetting defines the fields required to collect AutoSupport bundle
type AutoSupportSetting struct {
	// Component is the AutoSupport component against which the callhome event is to be invoked
	Component AutoSupportComponent `json:"component"`

	// LocalCollection is a flag to prevent transfer of AutoSupport bundle to the backend.
	// When set to true, the collected AutoSupport bundle is not uploaded.
	// When set to false, the collected AutoSupport bundle is uploaded.
	// For a one-off upload of the Support bundle, this flag can be overridden by the ForceUpload flag
	// +kubebuilder:default:=false
	LocalCollection bool `json:"localCollection,omitempty"`

	// UserMessage is the additional message to include in the AutoSupport subject header
	UserMessage string `json:"userMessage,omitempty"`

	// Priority is the priority set on the AutoSupport bundle to be sent to the backend.
	// Priority is appended to the subject header of the AutoSupport bundle
	// (default: `notice`)
	// +kubebuilder:validation:Enum=notice;debug;warning;error;emergency;alert;info
	// +kubebuilder:default:=notice
	Priority AutoSupportPriority `json:"priority,omitempty"`

	// Retry forces the controller to retry the operation before the AutoSupport transitioned to an error state.
	// When set to true, the operation is retried (with exponential backoff) till the operation is successful.
	// When set to false, the AutoSupport transitions to an error state on any error
	// +kubebuilder:default:=false
	Retry bool `json:"retry,omitempty"`

	// ForceUpload is a flag to force a one-off AutoSupport bundle transfer to the backend.
	// Overrides autoUpload flag in the cluster wide AutoSupport configuration and localCollection flag in this AutoSupport specification
	// +kubebuilder:default:=false
	ForceUpload bool `json:"forceUpload,omitempty"`

	// StartTime is the starting time from when the AutoSupport is supposed to collect the data.
	// +kubebuilder:validation:Format:=date-time
	StartTime string `json:"startTime,omitempty"`

	// Duration determines the duration in hours from the StartTime for the AutoSupport
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:MultipleOf=1
	Duration int64 `json:"duration,omitempty"`
}

// AutoSupportComponent defines the callhome event for the component
type AutoSupportComponent struct {
	// Name is the AutoSupport component associated with the callhome event
	// +kubebuilder:validation:Enum=storage;controlplane;vasaprovider
	Name AutoSupportComponentName `json:"name"`
	// Event is the Callhome event to be invoked for the component
	CallhomeEvent string `json:"event"`
}

// AutoSupportComponentName is used to identify all components supported by AutoSupport controller for bundle collection
type AutoSupportComponentName string

const (
	// StorageComponent represents the support bundle collection component comprising the firetap container and the node
	StorageComponent AutoSupportComponentName = "storage"
	// ControlplaneComponent represents the support bundle collection component comprising the Kubernetes control plane
	ControlplaneComponent AutoSupportComponentName = "controlplane"
	// VASAProviderComponent represents the support bundle collection component comprising the Kubernetes control plane
	VASAProviderComponent AutoSupportComponentName = "vasaprovider"
)

// AutoSupportTrigger represents the possible triggers which can cause an AutoSupport bundle collection
type AutoSupportTrigger string

// AutoSupport triggers supported
// firetapEvent: Represents an AutoSupport collection triggered by an event in the Firetap.
// coreEvent: Represents an AutoSupport collection triggered by a coredump.
// k8sEvent: Represents an AutoSupport collection triggered by an event in the Kubernetes.
// periodic: Represents an AutoSupport collection periodically triggered in the system
// manual: Represents an AutoSupport collection triggered by an user
// userTrigger: Represents an AutoSupport Collection triggered by a user from CLI.
const (
	FiretapEvent AutoSupportTrigger = "firetapEvent"
	CoreEvent    AutoSupportTrigger = "coreEvent"
	K8sEvent     AutoSupportTrigger = "k8sEvent"
	Periodic     AutoSupportTrigger = "periodic"
	Manual       AutoSupportTrigger = "manual"
	UserTrigger  AutoSupportTrigger = "userTrigger"
)

// AutoSupportPriority is used to denote the priority that the collected support bundle would be associated within the bundle header
type AutoSupportPriority string

const (
	// PriNotice is used to set the priority of the AutoSupport bundle sent to backend as "notice"
	PriNotice AutoSupportPriority = "notice"
	// PriDebug is used to set the priority of the AutoSupport bundle sent to backend as "debug"
	PriDebug AutoSupportPriority = "debug"
	// PriWarning is used to set the priority of the AutoSupport bundle sent to backend as "warning"
	PriWarning AutoSupportPriority = "warning"
	// PriError is used to set the priority of the AutoSupport bundle sent to backend as "error"
	PriError AutoSupportPriority = "error"
	// PriEmergency is used to set the priority of the AutoSupport bundle sent to backend as "emergency"
	PriEmergency AutoSupportPriority = "emergency"
	// PriAlert is used to set the priority of the AutoSupport bundle sent to backend as "alert"
	PriAlert AutoSupportPriority = "alert"
	// PriInfo is used to set the priority of the AutoSupport bundle sent to backend as "info"
	PriInfo AutoSupportPriority = "info"
)

// AutoSupportStatus defines the observed state of AutoSupport
type AutoSupportStatus struct {
	// SequenceNumber is the Id of the AutoSupport
	SequenceNumber int64 `json:"sequenceNumber,omitempty"`

	// Size is the total size of the AutoSupport bundle
	Size resource.Quantity `json:"size,omitempty"`

	// LocalPath is the location of the AutoSupport bundle in the local system
	LocalPath string `json:"localPath,omitempty"`

	// State is the current state of collecting an AutoSupport bundle
	// +kubebuilder:validation:Enum=initializing;collecting;collected;compressed;uploadReady;uploading;uploaded;error
	State AutoSupportStateType `json:"state,omitempty"`

	// Condition is the latest observation of the AutoSupport state
	Conditions []AutoSupportCondition `json:"conditions,omitempty"`
	Cluster    string                 `json:"cluster,omitempty"`
	// Case Number of Asup.
	CaseNumber int64 `json:"caseNumber,omitempty"`
}

// AutoSupportCondition defines the condition information for an AutoSupport
type AutoSupportCondition struct {
	// Type of AutoSupport condition
	Type AutoSupportStateType `json:"type"`

	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status"`

	// LastHeartbeatTime is the timestamp of when the AutoSupport condition was last probed
	LastHeartbeatTime metav1.Time `json:"lastHeartbeatTime,omitempty"`

	// LastTransitionTime is the timestamp for when the AutoSupport last transitioned from one status to another
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// Reason is the (brief) reason for the condition's last transition
	Reason string `json:"reason,omitempty"`

	// Message is the Human-readable message indicating details about last transition
	Message string `json:"message,omitempty"`
}

// AutoSupportStateType represents all the states that a particular AutoSupport representing a bundle could be in
type AutoSupportStateType string

// Valid states for an AutoSupport are:
// Collecting: AutoSupport bundle is being collected
// Collected: AutoSupport bundle is collected
// Compressed: AutoSupport bundle is compressed
// UploadReady: AutoSupport bundle is ready for upload - a flag set by upload scheduler to begin upload
// Uploading: AutoSupport bundle is being transferred to the backend
// Uploaded: AutoSupport bundle transfer completed
// Error: Non-recoverable error from one of the other states
const (
	AutoSupportInitializing AutoSupportStateType = "initializing"
	AutoSupportCollecting   AutoSupportStateType = "collecting"
	AutoSupportCollected    AutoSupportStateType = "collected"
	AutoSupportCompressed   AutoSupportStateType = "compressed"
	AutoSupportUploadReady  AutoSupportStateType = "uploadReady"
	AutoSupportUploading    AutoSupportStateType = "uploading"
	AutoSupportUploaded     AutoSupportStateType = "uploaded"
	AutoSupportError        AutoSupportStateType = "error"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Sequence",type="string",JSONPath=".status.sequenceNumber",description="Sequence number of the AutoSupport"
// +kubebuilder:printcolumn:name="Component",type="string",JSONPath=".spec.component.name",description="AutoSupport component associated with the event"
// +kubebuilder:printcolumn:name="Event",type="string",JSONPath=".spec.component.event", description="Call home event for the AutoSupport"
// +kubebuilder:printcolumn:name="Trigger",type="string",JSONPath=".spec.trigger", description="Trigger type of the AutoSupport"
// +kubebuilder:printcolumn:name="Priority",type="string",JSONPath=".spec.priority", description="Priority of the AutoSupport"
// +kubebuilder:printcolumn:name="Size",type="string",JSONPath=".status.size", description="Size of the AutoSupport bundle collected"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state", description="Current state of the AutoSupport"
// +kubebuilder:printcolumn:name="Local Collection",type="boolean",JSONPath=".spec.localCollection", description="Flag to indicate if the AutoSupport is to be uploaded or is a local collection",priority=1
// +kubebuilder:printcolumn:name="Local Path",type="string",JSONPath=".status.localPath", description="Location of the AutoSupport bundle in the local system", priority=1
// +kubebuilder:resource:shortName=adsas,categories={ads,all}

// AstraDSAutoSupport is the Schema for the astradsautosupports API
type AstraDSAutoSupport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AutoSupportSpec   `json:"spec,omitempty"`
	Status AutoSupportStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AstraDSAutoSupportList contains a list of AstraDSAutoSupport
type AstraDSAutoSupportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AstraDSAutoSupport `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AstraDSAutoSupport{}, &AstraDSAutoSupportList{})
}
