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

// AstraDSKeyProviderSpec defines the desired state of AstraDSKeyProvider
type AstraDSKeyProviderSpec struct {
	// The name of the ads cluster.
	Cluster string `json:"cluster"`
	// Kmip Server config for external key provider.
	KmipServer KmipServerConfig `json:"kmipServer,omitempty"`
}
type KmipServerConfig struct {
	// The name of the secret that holds the certificates
	SecretRef string `json:"secretRef"`
	// Array of the hostnames or IP addresses associated with this Key Server. Multiple hostnames or IP addresses must only be provided if the key servers are in a clustered configuration.
	Hostnames []string `json:"hostnames"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=5696
	Port int64 `json:"port"`
}

// AstraDSKeyProviderStatus defines the observed state of AstraDSKeyProvider
type AstraDSKeyProviderStatus struct {
	// UUID of key provider in ADS.
	KeyProviderUUID string `json:"keyProviderUUID,omitempty"`
	// Status of Kmip Server.
	KmipServer KmipServerStatus `json:"kmipServerStatus,omitempty"`
}
type KmipServerStatus struct {
	// UUID of key server in ADS.
	KeyServerUUID string `json:"keyServerUUID,omitempty"`
	// Capabilites of the KMIP Server
	Capabilities string `json:"capabilities,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=adskp,categories={ads,all}

// AstraDSKeyProvider is the Schema for the astradskeyproviders API
type AstraDSKeyProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AstraDSKeyProviderSpec   `json:"spec,omitempty"`
	Status AstraDSKeyProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AstraDSKeyProviderList contains a list of AstraDSKeyProvider
type AstraDSKeyProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AstraDSKeyProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AstraDSKeyProvider{}, &AstraDSKeyProviderList{})
}
