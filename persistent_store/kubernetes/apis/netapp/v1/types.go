// Copyright 2018 NetApp, Inc. All Rights Reserved.

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Backend defines a netapp backend
// +genclient
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Backend struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object’s metadata. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#metadata
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of the NetApp backend. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#spec-and-status
	Spec runtime.RawExtension `json:"spec"`
	// Status of the NetApp backend. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#spec-and-status
	Status BackendStatus `json:"status"`
}

// BackendList is a list of Backends.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type BackendList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata
	// More info: https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#metadata
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of Backends
	Items []*Backend `json:"items"`
}

// BackendStatus is the status of the NetApp backend.
// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#spec-and-status
type BackendStatus struct {
	Version string `json:"version"`
	Online  bool   `json:"online"`
}

// Volume defines a netapp volume
// +genclient
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Volume struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object’s metadata. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#metadata
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of the NetApp volume. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#spec-and-status
	Spec runtime.RawExtension `json:"spec"`

	// Status of the NetApp volume. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#spec-and-status
	Status VolumeStatus `json:"status"`
}

// VolumeList is a list of Volume.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VolumeList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata
	// More info: https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#metadata
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of Volumes
	Items []*Volume `json:"items"`
}

// VolumeStatus is the status of the NetApp volume. More info:
// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#spec-and-status
type VolumeStatus struct {
	Backend  string `json:"backend"`
	Pool     string `json:"pool"`
	Orphaned bool   `json:"orphaned"`
}

// StorageClass defines a netapp storage class
// +genclient
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type StorageClass struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object’s metadata. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#metadata
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of the NetApp volume. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#spec-and-status
	Spec runtime.RawExtension `json:"spec"`
}

// StorageClassList is a list of StorageClass.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type StorageClassList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata
	// More info: https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#metadata
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of StorageClasses
	Items []*StorageClass `json:"items"`
}

// VolumeTransaction defines a netapp volume transtaion
// +genclient
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VolumeTransaction struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object’s metadata. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#metadata
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Transaction information
	Transaction VolumeTransactionSpec `json:"transaction"`
}

// VolumeTransactionList is a list of VolumeTransaction.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VolumeTransactionList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata
	// More info: https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#metadata
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of VolumeTransactiones
	Items []*VolumeTransaction `json:"items"`
}

// VolumeTransactionSpec is the specification of the NetApp volume transation. More info:
// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#spec-and-status
type VolumeTransactionSpec struct {
	Operation string               `json:"operation"`
	Config    runtime.RawExtension `json:"config"`
}
