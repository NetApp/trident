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

// Protocol is the IO protocol that this rule is configured to
// +kubebuilder:validation:Pattern=`^(nfs4|nfs3)$`
type Protocol string

// RoRule is the rule defining read access <br>
// `Any`: Always allow access to the exported data <br>
// `Sys`: Allow access if authenticated by NFS AUTH_SYS <br>
// `None`: clients with any security type are granted access as anonymous <br>
// `Never`: Never allow access, regardless of incoming security type. <br>
// +kubebuilder:validation:Pattern=`^(any|sys|none|never)$`
type RoRule string

// RwRule is the rule defining write access <br>
// `Any`: Always allow access to the exported data <br>
// `Sys`: Allow access if authenticated by NFS AUTH_SYS <br>
// `None`: clients with any security type are granted access as anonymous <br>
// `Never`: Never allow access, regardless of incoming security type. <br>
// +kubebuilder:validation:Pattern=`^(any|sys|none|never)$`
type RwRule string

// SuperUser defines how to handle clients presenting with user ID 0 depending on their security type. <br>
// `Any`: Always allow access to the exported data <br>
// `Sys`: Allow access if authenticated by NFS AUTH_SYS <br>
// `None`: clients with any security type are granted access as anonymous <br>
// +kubebuilder:validation:Pattern=`^(any|sys|none)$`
type SuperUser string

type AstraDSExportPolicyRule struct {
	// Clients defines the list of client IP addresses that this rule will apply to
	Clients []string `json:"clients"`
	// Protocols defines the list of protocols that this rule will apply to
	Protocols []Protocol `json:"protocols"`
	// +kubebuilder:validation:Type=integer
	// +kubebuilder:validation:Minimum= 1
	// +kubebuilder:validation:Maximum=1024
	// RuleIndex defines the rule index number/order
	RuleIndex uint64 `json:"ruleIndex"`
	// RoRules defines the security styles for read-only client access
	RoRules []RoRule `json:"roRules"`
	// RwRules defines the security styles for read-write client access
	RwRules []RwRule `json:"rwRules"`
	// SuperUser defines the security styles for root user access
	SuperUser []SuperUser `json:"superUser"`
	// AnonUser defines a UNIX user ID that the user credentials are mapped to
	// +kubebuilder:validation:Type=integer
	// +kubebuilder:validation:Minimum= 0
	// +kubebuilder:validation:Maximum=4294967295
	AnonUser int `json:"anonUser,omitempty"`
}

type AstraDSExportPolicyRules []AstraDSExportPolicyRule

// AstraDSExportPolicySpec defines the desired state of AstraDSExportPolicy
type AstraDSExportPolicySpec struct {
	// Rules defines the rules of an export policy
	Rules AstraDSExportPolicyRules `json:"rules"`
	// Cluster is the ADS cluster that this export policy is associated with
	Cluster string `json:"cluster"`
}

// AstraDSExportPolicyStatus defines the observed state of AstraDSExportPolicy
type AstraDSExportPolicyStatus struct {
	// RSID is the unique numerical identifier for this export policy
	RSID uint64 `json:"rsid,omitempty"`
	// Cluster is the ADS cluster that this export policy is associated with
	Cluster string `json:"cluster,omitempty"`
	// Conditions are the latest observations of the export policy's state
	Conditions []NetAppExportPolicyCondition `json:"conditions,omitempty"`
}

// NetAppExportPolicyConditionType indicates the type of condition occurred on a export policy
type NetAppExportPolicyConditionType string

const (
	// Deleted is a condition that represents the state of a delete operation
	NetAppExportPolicyDeleted NetAppExportPolicyConditionType = "Deleted"
	// Created is a condition that represents the state of a create operation
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
// +kubebuilder:storageversion
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
