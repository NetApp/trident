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
	// Config of the trident backend
	Config runtime.RawExtension `json:"config"`
	// Name is the real name of the backend (metadata has restrictions)
	Name string `json:"name"`
	// Version is the version of the backend
	Version string `json:"version"`
	// Online defines if the backend is online
	Online bool `json:"online"`
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
	// Config is the Volumes Config
	Config runtime.RawExtension `json:"config"`
	// Backend is the name of the volumes backend
	Backend string `json:"backend"`
	// Pool is the volumes pool
	Pool string `json:"pool"`
	// Orphaned defines if the backend is orphaned
	Orphaned bool `json:"orphaned"`
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
	// Operation is the volume transaction operation
	Operation string `json:"operation"`
	// Config is the volume config
	Config runtime.RawExtension `json:"config"`
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
