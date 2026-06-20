// Copyright 2026 NetApp, Inc. All Rights Reserved.

package vaindexer

import (
	"context"
	"fmt"

	k8sstoragev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	. "github.com/netapp/trident/logging"
)

// Indexer keys
const (
	vaByNodeIndex   = "vaByNode"
	vaByVolumeIndex = "vaByVolume"
)

type VaIndexer struct {
	// Volume Attachment Indexer
	vaIndexer            cache.Indexer
	vaController         cache.SharedIndexInformer
	vaControllerStopChan chan struct{}
	vaSource             cache.ListerWatcher
	vaSynced             cache.InformerSynced
}

func NewVolumeAttachmentIndexer(kubeClient kubernetes.Interface) *VaIndexer {
	va := &VaIndexer{}

	// Init stop channels
	va.vaControllerStopChan = make(chan struct{})

	// Set up a watch for VolumeAttachments
	va.vaSource = cache.NewListWatchFromClient(
		kubeClient.StorageV1().RESTClient(),
		"volumeattachments",
		metav1.NamespaceAll,
		fields.Everything(),
	)

	// Set cacheSyncPeriod to 0 to disable resync. This is to avoid scaling concerns with large number of VAs.
	const cacheSyncPeriod = 0

	// Set up the VolumeAttachments indexing controller
	va.vaController = cache.NewSharedIndexInformer(
		va.vaSource,
		&k8sstoragev1.VolumeAttachment{},
		cacheSyncPeriod,
		cache.Indexers{
			vaByNodeIndex:   volumeAttachmentsByNodeKeyFunc,
			vaByVolumeIndex: volumeAttachmentsByVolumeKeyFunc,
		},
	)
	va.vaIndexer = va.vaController.GetIndexer()
	va.vaSynced = va.vaController.HasSynced
	return va
}

func (v *VaIndexer) Activate() {
	go v.vaController.Run(v.vaControllerStopChan)
}

func (v *VaIndexer) Deactivate() {
	close(v.vaControllerStopChan)
}

func (v *VaIndexer) WaitForCacheSync(ctx context.Context) bool {
	var ok bool
	if ok = cache.WaitForCacheSync(v.vaControllerStopChan, v.vaSynced); !ok {
		Logc(ctx).Errorf("failed to wait for vaController cache to sync")
	}
	return ok
}

// GetCachedVolumeAttachmentsByNode returns a VA list for a node from the client's cache.
func (v *VaIndexer) GetCachedVolumeAttachmentsByNode(
	ctx context.Context, nodeName string,
) ([]*k8sstoragev1.VolumeAttachment, error) {
	logFields := LogFields{"nodeName": nodeName}
	Logc(ctx).WithFields(logFields).Trace(">>>> GetCachedVolumeAttachmentsByNode")
	defer Logc(ctx).Trace("<<<< GetCachedVolumeAttachmentsByNode")

	items, err := v.vaIndexer.ByIndex(vaByNodeIndex, nodeName)
	if err != nil {
		Logc(ctx).WithFields(logFields).Error("Could not search cache for volume attachments.")
		return nil, fmt.Errorf("could not search cache for volume attachments on node %s: %v", nodeName, err)
	}
	if len(items) == 0 {
		Logc(ctx).WithFields(logFields).Debugf("No volume attachments found in cache for node %s.", nodeName)
	} else {
		Logc(ctx).WithFields(logFields).Debug("Found cached volume attachments.")
	}

	attachments := []*k8sstoragev1.VolumeAttachment{}
	for _, item := range items {
		va, ok := item.(*k8sstoragev1.VolumeAttachment)
		if !ok {
			Logc(ctx).WithFields(logFields).Error("Non-volume-reference cached object found.")
			return nil, fmt.Errorf("non-volume-reference object %s found in cache", nodeName)
		}
		attachments = append(attachments, va)
	}
	return attachments, nil
}

// volumeAttachmentsByNodeKeyFunc is a indexer KeyFunc which knows how to make keys for VolumeAttachment by node.
// The key is the node name.
func volumeAttachmentsByNodeKeyFunc(obj interface{}) ([]string, error) {
	Logc(context.Background()).Trace(">>>> volumeAttachmentsByNodeKeyFunc")
	defer Logc(context.Background()).Trace("<<<< volumeAttachmentsByNodeKeyFunc")

	va, ok := obj.(*k8sstoragev1.VolumeAttachment)
	if !ok {
		return nil, fmt.Errorf("object is not a VolumeAttachment CR")
	}
	if va.Spec.NodeName != "" {
		Log().WithFields(LogFields{
			"nodeName": va.Spec.NodeName,
			"vaSource": va.Spec.Source,
		}).Trace("Volume attached to node.")
		return []string{va.Spec.NodeName}, nil
	}
	return nil, nil
}

// GetCachedVolumeAttachmentsByVolume returns a VA list for a volume from the client's cache.
func (v *VaIndexer) GetCachedVolumeAttachmentsByVolume(
	ctx context.Context, pvName string,
) ([]*k8sstoragev1.VolumeAttachment, error) {
	logFields := LogFields{"pvName": pvName}
	Logc(ctx).WithFields(logFields).Trace(">>>> GetCachedVolumeAttachmentsByVolume")
	defer Logc(ctx).Trace("<<<< GetCachedVolumeAttachmentsByVolume")

	items, err := v.vaIndexer.ByIndex(vaByVolumeIndex, pvName)
	if err != nil {
		Logc(ctx).WithFields(logFields).Error("Could not search cache for volume attachments.")
		return nil, fmt.Errorf("could not search cache for volume attachments for volume %s: %v", pvName, err)
	}
	if len(items) == 0 {
		Logc(ctx).WithFields(logFields).Debugf("No volume attachments found in cache for volume %s.", pvName)
	} else {
		Logc(ctx).WithFields(logFields).Debug("Found cached volume attachments.")
	}

	attachments := make([]*k8sstoragev1.VolumeAttachment, 0, len(items))
	for _, item := range items {
		va, ok := item.(*k8sstoragev1.VolumeAttachment)
		if !ok {
			Logc(ctx).WithFields(logFields).Error("Non-volume-reference cached object found.")
			return nil, fmt.Errorf("non-volume-reference object %s found in cache", pvName)
		}
		attachments = append(attachments, va)
	}
	return attachments, nil
}

// volumeAttachmentsByVolumeKeyFunc is an indexer KeyFunc which knows how to make keys for
// VolumeAttachment by volume. The key is the persistent volume name.
func volumeAttachmentsByVolumeKeyFunc(obj interface{}) ([]string, error) {
	Logc(context.Background()).Trace(">>>> volumeAttachmentsByVolumeKeyFunc")
	defer Logc(context.Background()).Trace("<<<< volumeAttachmentsByVolumeKeyFunc")

	va, ok := obj.(*k8sstoragev1.VolumeAttachment)
	if !ok {
		return nil, fmt.Errorf("object is not a VolumeAttachment CR")
	}
	if va.Spec.Source.PersistentVolumeName == nil || *va.Spec.Source.PersistentVolumeName == "" {
		return nil, nil
	}

	Log().WithFields(LogFields{
		"nodeName": va.Spec.NodeName,
		"vaSource": va.Spec.Source,
	}).Trace("Volume attached to node.")
	return []string{*va.Spec.Source.PersistentVolumeName}, nil
}
