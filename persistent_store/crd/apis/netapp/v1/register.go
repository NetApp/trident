// Copyright 2025 NetApp, Inc. All Rights Reserved.

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	GroupName    = "trident.netapp.io"
	GroupVersion = "v1"
)

// SchemeGroupVersion is group version used to register these objects
var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: GroupVersion}

// Kind takes an unqualified kind and returns a Group qualified GroupKind
func Kind(kind string) schema.GroupKind {
	return SchemeGroupVersion.WithKind(kind).GroupKind()
}

// Resource takes an unqualified resource and returns back a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

// Adds the list of known types to the given scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&TridentBackend{},
		&TridentBackendList{},
		&TridentMirrorRelationship{},
		&TridentMirrorRelationshipList{},
		&TridentActionMirrorUpdate{},
		&TridentActionMirrorUpdateList{},
		&TridentSnapshotInfo{},
		&TridentSnapshotInfoList{},
		&TridentBackendConfig{},
		&TridentBackendConfigList{},
		&TridentVolume{},
		&TridentVolumeList{},
		&TridentVolumePublication{},
		&TridentVolumePublicationList{},
		&TridentStorageClass{},
		&TridentStorageClassList{},
		&TridentTransaction{},
		&TridentTransactionList{},
		&TridentNode{},
		&TridentNodeList{},
		&TridentNodeRemediation{},
		&TridentNodeRemediationList{},
		&TridentNodeRemediationTemplate{},
		&TridentNodeRemediationTemplateList{},
		&TridentVersion{},
		&TridentVersionList{},
		&TridentSnapshot{},
		&TridentSnapshotList{},
		&TridentGroupSnapshot{},
		&TridentGroupSnapshotList{},
		&TridentVolumeReference{},
		&TridentVolumeReferenceList{},
		&TridentActionSnapshotRestore{},
		&TridentActionSnapshotRestoreList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
