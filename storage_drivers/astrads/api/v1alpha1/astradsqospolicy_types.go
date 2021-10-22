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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AstraDSQosPolicySpec defines the desired state of AstraDSQosPolicy
type AstraDSQosPolicySpec struct {
	MinIOPS     int32  `json:"minIOPS"`
	MaxIOPS     int32  `json:"maxIOPS"`
	BurstIOPS   int32  `json:"burstIOPS"`
	Description string `json:"description"`
	Cluster     string `json:"cluster"`
}

// AstraDSQosPolicySpec defines the desired state of AstraDSQosPolicy
type AstraDSQosPolicyStatus struct {
	MinIOPS   int32  `json:"minIOPS"`
	MaxIOPS   int32  `json:"maxIOPS"`
	BurstIOPS int32  `json:"burstIOPS"`
	Cluster   string `json:"cluster"`
	Uuid      string `json:"uuid"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=adsqp,categories={ads,all}

// AstraDSQosPolicy is the Schema for the astradsqospolicies API
type AstraDSQosPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AstraDSQosPolicySpec   `json:"spec"`
	Status AstraDSQosPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AstraDSQosPolicyList contains a list of AstraDSQosPolicy
type AstraDSQosPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AstraDSQosPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AstraDSQosPolicy{}, &AstraDSQosPolicyList{})
}
