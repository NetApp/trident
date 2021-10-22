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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Pattern=`^(nfs4|nfs3)$`
type Protocol string

// +kubebuilder:validation:Pattern=`^(any|sys|none|never)$`
type RoRule string

// +kubebuilder:validation:Pattern=`^(any|sys|none|never)$`
type RwRule string

// +kubebuilder:validation:Pattern=`^(any|sys|none)$`
type SuperUser string

type AstraDSExportPolicyRule struct {
	// list of Client IP addresses
	Clients []string `json:"clients"`
	// list of access protocols
	Protocols []Protocol `json:"protocols"`
	// +kubebuilder:validation:Type=integer
	// +kubebuilder:validation:Minimum= 1
	// +kubebuilder:validation:Maximum=1024
	// rule index number
	RuleIndex uint64 `json:"ruleIndex"`
	// security styles for read-only client access
	RoRules []RoRule `json:"roRules"`
	// security styles for read-write client access
	RwRules []RwRule `json:"rwRules"`
	// security styles for root user access
	SuperUser []SuperUser `json:"superUser"`
	// sets the uid for the anon user.
	// +kubebuilder:validation:Type=integer
	// +kubebuilder:validation:Minimum= 0
	// +kubebuilder:validation:Maximum=4294967295
	AnonUser int `json:"anonUser,omitempty"`
}

type AstraDSExportPolicyRules []AstraDSExportPolicyRule

// AstraDSExportPolicySpec defines the desired state of AstraDSExportPolicy
type AstraDSExportPolicySpec struct {
	Rules   AstraDSExportPolicyRules `json:"rules"`
	Cluster string                   `json:"cluster"`
}

// AstraDSExportPolicyStatus defines the observed state of AstraDSExportPolicy
type AstraDSExportPolicyStatus struct {
	RSID       uint64                        `json:"rsid,omitempty"`
	Cluster    string                        `json:"cluster,omitempty"`
	Conditions []NetAppExportPolicyCondition `json:"conditions,omitempty"`
}

//NetAppExportPolicyConditionType indicates the type of condition occurred on a export policy
type NetAppExportPolicyConditionType string

const (
	NetAppExportPolicyDeleted NetAppExportPolicyConditionType = "Deleted"
	NetAppExportPolicyCreated NetAppExportPolicyConditionType = "Created"
)

// NetAppExportPolicyCondition contains the condition information for a NetAppExportPolicy
type NetAppExportPolicyCondition struct {
	// Type of NetAppExportPolicy condition.
	Type NetAppExportPolicyConditionType `json:"type"`
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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=adsep,categories={ads,all}
// AstraDSExportPolicy is the Schema for the astradsexportpolicies API
type AstraDSExportPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AstraDSExportPolicySpec   `json:"spec,omitempty"`
	Status AstraDSExportPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AstraDSExportPolicyList contains a list of AstraDSExportPolicy
type AstraDSExportPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AstraDSExportPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AstraDSExportPolicy{}, &AstraDSExportPolicyList{})
}
