/*
Copyright 2022 NetApp Inc..

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AstraDSSEARKeyRotateRequestSpec defines the current state of AstraDSSEARKeyRotateRequest
type AstraDSSEARKeyRotateRequestSpec struct {
	Cluster string `json:"cluster"`
}

// AstraDSSEARKeyRotateRequestStatus defines the desired state of AstraDSSEARKeyRotateRequest
type AstraDSSEARKeyRotateRequestStatus struct {
	ADSKeyProviderUUID string      `json:"adsKeyProviderUUID,omitempty"`
	NewKeyUUID         string      `json:"newKeyUUID,omitempty"`
	ActiveTime         metav1.Time `json:"activeTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories={ads,all}

// AstraDSSEARKeyRotateRequest is the Schema for the AstraDSSEARKeyRotateRequest API
type AstraDSSEARKeyRotateRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AstraDSSEARKeyRotateRequestSpec   `json:"spec"`
	Status AstraDSSEARKeyRotateRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AstraDSSEARKeyRotateRequestList contains a list of AstraDSSEARKeyRotateRequest
type AstraDSSEARKeyRotateRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AstraDSSEARKeyRotateRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AstraDSSEARKeyRotateRequest{}, &AstraDSSEARKeyRotateRequestList{})
}
