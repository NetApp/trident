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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=None;Node;Upgrade;Recovery
type MaintenanceVariant string

/*
Use like:

var variant MaintenanceVariant
err := variant.InitFromString("upgrade")
if err != nil {
        // handle
}
*/
func (mv *MaintenanceVariant) InitFromString(str string) error {
	switch strings.Title(strings.ToLower(str)) {
	case string(None):
		*mv = MaintenanceVariant("None")
	case string(Node):
		*mv = MaintenanceVariant("Node")
	case string(Upgrade):
		*mv = MaintenanceVariant("Upgrade")
	case string(Recovery):
		*mv = MaintenanceVariant("Recovery")
	default:
		return fmt.Errorf("invalid variant string: %v", str)
	}
	return nil
}

func ValidMaintenanceVariants() string {
	return strings.Join([]string{string(None), string(Node), string(Upgrade), string(Recovery)}, ",")
}

const (
	None     MaintenanceVariant = "None"
	Node     MaintenanceVariant = "Node"
	Upgrade  MaintenanceVariant = "Upgrade"
	Recovery MaintenanceVariant = "Recovery"
)

// AstraDSNodeManagementSpec defines the desired state of AstraDSNodeManagement
type AstraDSNodeManagementSpec struct {
	// +kubebuilder:validation:Required
	NodeName string `json:"nodeName"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default:="Upgrade"
	Variant MaintenanceVariant `json:"variant"`
}

// AstraDSNodeManagementStatus defines the observed state of AstraDSNodeManagement
type AstraDSNodeManagementStatus struct {
	// InMaintenance signifies whether the node is in maintenance mode or not
	InMaintenance bool `json:"inMaintenance"`

	// +kubebuilder:default:="None"
	Variant string `json:"variant"`

	Conditions []ADSNodeManagementCondition `json:"conditions,omitempty"`

	// Possible values: Disabled;FailedToRecover;Unexpected;RecoveringFromMaintenance;PreparingForMaintenance;ReadyForMaintenance
	State string `json:"state,omitempty"`

	JobUUID string `json:"jobUUID,omitempty"`
}

type ADSNodeManagementConditionType string

const (
	ADSNodeManagementConditionTypeNodeInMaintenance ADSNodeManagementConditionType = "NodeInMaintenance"
)

// ADSNodeManagementCondition
type ADSNodeManagementCondition struct {
	// Type of management mode condition.
	Type ADSNodeManagementConditionType `json:"type"`
	// Status of the condition, one of True, False, InProgress.
	Status ADSNodeManagementConditionStatus `json:"status,omitempty"`
	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

type ADSNodeManagementConditionStatus string

const (
	ADSNodeManagementConditionTrue       ADSNodeManagementConditionStatus = "True"
	ADSNodeManagementConditionFalse      ADSNodeManagementConditionStatus = "False"
	ADSNodeManagementConditionPreparing  ADSNodeManagementConditionStatus = "PreparingForMaintenance"
	ADSNodeManagementConditionRecovering ADSNodeManagementConditionStatus = "RecoveringFromMaintenance"
	ADSNodeManagementConditionFailed     ADSNodeManagementConditionStatus = "Failed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Variant",type="string",JSONPath=".status.variant",description="The node maintenance mode variant"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".spec.state",description="The state of node maintenance action"
// +kubebuilder:resource:shortName=adsnm,categories={ads,all}

// AstraDSNodeManagement is the Schema for the astradsnodemanagements API
type AstraDSNodeManagement struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AstraDSNodeManagementSpec   `json:"spec,omitempty"`
	Status AstraDSNodeManagementStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// AstraDSNodeManagementList contains a list of AstraDSNodeManagement
type AstraDSNodeManagementList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AstraDSNodeManagement `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AstraDSNodeManagement{}, &AstraDSNodeManagementList{})
}
