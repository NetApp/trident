// Copyright 2025 NetApp, Inc. All Rights Reserved.
package indexers

//go:generate mockgen -destination=../../../../mocks/mock_frontend/crd/controller/indexers/indexer/mock_vaindexer.go -package=mock_indexer github.com/netapp/trident/frontend/crd/controller/indexers VolumeAttachmentIndexer
//go:generate mockgen -destination=../../../../mocks/mock_frontend/crd/controller/indexers/mock_indexers.go github.com/netapp/trident/frontend/crd/controller/indexers Indexers
//go:generate mockgen -destination=../../../../mocks/mock_cache/mock_indexer.go -package=mock_cache k8s.io/client-go/tools/cache Indexer

import (
	"context"

	k8sstoragev1 "k8s.io/api/storage/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/netapp/trident/frontend/crd/controller/indexers/vaindexer"
)

// Indexers interface provides access to all the different indexers
type Indexers interface {
	Activate()
	Deactivate()
	VolumeAttachmentIndexer() VolumeAttachmentIndexer
}

// Generic indexer interface that all specific indexers implement
type Indexer interface {
	Activate()
	Deactivate()
	WaitForCacheSync(ctx context.Context) bool
}

// VolumeAttachmentIndexer provides access to the VolumeAttachment indexer
type VolumeAttachmentIndexer interface {
	Indexer
	GetCachedVolumeAttachmentsByNode(ctx context.Context, nodeName string) ([]*k8sstoragev1.VolumeAttachment, error)
}

type k8sIndexers struct {
	Indexer
	vaIndexer VolumeAttachmentIndexer
}

func NewIndexers(kubeClient kubernetes.Interface) *k8sIndexers {
	indexers := &k8sIndexers{}
	indexers.vaIndexer = vaindexer.NewVolumeAttachmentIndexer(kubeClient)
	return indexers
}

// Activate starts all the indexers
func (k *k8sIndexers) Activate() {
	k.vaIndexer.Activate()
}

// Deactivate stops all the indexers
func (k *k8sIndexers) Deactivate() {
	k.vaIndexer.Deactivate()
}

func (k *k8sIndexers) VolumeAttachmentIndexer() VolumeAttachmentIndexer {
	return k.vaIndexer
}
