// Copyright 2026 NetApp, Inc. All Rights Reserved.

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/netapp/trident/frontend/csi"
	storageattribute "github.com/netapp/trident/storage_attribute"
)

func TestIsAttachedTridentVolumeAttachment(t *testing.T) {
	volumeID := "vol-1"
	nodeID := "node-1"

	tests := map[string]struct {
		attachment *storagev1.VolumeAttachment
		want       bool
	}{
		"attached trident volume": {
			attachment: testVolumeAttachment(volumeID, nodeID, storageattribute.NVMe, "192.0.2.10"),
			want:       true,
		},
		"nil attachment": {},
		"wrong attacher": {
			attachment: func() *storagev1.VolumeAttachment {
				va := testVolumeAttachment(volumeID, nodeID, storageattribute.NVMe, "192.0.2.10")
				va.Spec.Attacher = "csi.other.io"
				return va
			}(),
		},
		"not attached": {
			attachment: func() *storagev1.VolumeAttachment {
				va := testVolumeAttachment(volumeID, nodeID, storageattribute.NVMe, "192.0.2.10")
				va.Status.Attached = false
				return va
			}(),
		},
		"empty metadata": {
			attachment: func() *storagev1.VolumeAttachment {
				va := testVolumeAttachment(volumeID, nodeID, storageattribute.NVMe, "192.0.2.10")
				va.Status.AttachmentMetadata = nil
				return va
			}(),
		},
		"wrong node": {
			attachment: testVolumeAttachment(volumeID, "other-node", storageattribute.NVMe, "192.0.2.10"),
		},
		"wrong volume": {
			attachment: testVolumeAttachment("other-vol", nodeID, storageattribute.NVMe, "192.0.2.10"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, isAttachedTridentVolumeAttachment(test.attachment, volumeID, nodeID))
		})
	}
}

func TestTargetIPHandlerForAttachment(t *testing.T) {
	volumeID := "vol-1"
	nodeID := "node-1"

	tests := map[string]struct {
		attachment  *storagev1.VolumeAttachment
		wantOK      bool
		wantSANType string
		wantKey     string
		wantCurrent string
		wantEncoded string
	}{
		"nvme is registered": {
			attachment:  testVolumeAttachment(volumeID, nodeID, storageattribute.NVMe, "192.0.2.10"),
			wantOK:      true,
			wantSANType: storageattribute.NVMe,
			wantKey:     nvmeTargetIPsMetadataKey,
			wantCurrent: "192.0.2.10",
			wantEncoded: "192.0.2.10,192.0.2.11",
		},
		"iscsi is not registered yet": {
			attachment: testVolumeAttachment(volumeID, nodeID, storageattribute.ISCSI, "192.0.2.10"),
		},
		"fcp is not registered yet": {
			attachment: testVolumeAttachment(volumeID, nodeID, storageattribute.FCP, "192.0.2.10"),
		},
		"nil attachment": {},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			handler, ok := targetIPHandlerForAttachment(test.attachment)
			assert.Equal(t, test.wantOK, ok)
			if !test.wantOK {
				return
			}
			assert.Equal(t, test.wantSANType, handler.sanType)
			assert.Equal(t, test.wantKey, handler.metadataKey)
			assert.Equal(t, test.wantCurrent, handler.current(test.attachment))
			assert.Equal(t, test.wantEncoded, handler.encode([]string{"192.0.2.10", "192.0.2.11"}))
		})
	}
}

func TestUpdateTridentVolumeAttachmentTargetIPs_NVMe(t *testing.T) {
	volumeID := "vol-1"
	nodeID := "node-1"
	attachment := testVolumeAttachment(volumeID, nodeID, storageattribute.NVMe, "192.0.2.10")
	controller := &TridentCrdController{
		kubeClientset: k8sfake.NewSimpleClientset(attachment),
	}

	err := controller.updateTridentVolumeAttachmentTargetIPs(
		context.Background(), attachment.Name, volumeID, nodeID, []string{"192.0.2.11", "192.0.2.12"},
	)

	require.NoError(t, err)
	updated, err := controller.kubeClientset.StorageV1().VolumeAttachments().Get(
		context.Background(), attachment.Name, metav1.GetOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, "192.0.2.11,192.0.2.12", updated.Status.AttachmentMetadata[nvmeTargetIPsMetadataKey])
}

func TestUpdateTridentVolumeAttachmentTargetIPs_UnregisteredProtocolSkipped(t *testing.T) {
	volumeID := "vol-1"
	nodeID := "node-1"
	attachment := testVolumeAttachment(volumeID, nodeID, storageattribute.ISCSI, "192.0.2.10")
	attachment.Status.AttachmentMetadata["iscsiTargetPortal"] = "192.0.2.10"
	controller := &TridentCrdController{
		kubeClientset: k8sfake.NewSimpleClientset(attachment),
	}

	err := controller.updateTridentVolumeAttachmentTargetIPs(
		context.Background(), attachment.Name, volumeID, nodeID, []string{"192.0.2.11"},
	)

	require.NoError(t, err)
	updated, err := controller.kubeClientset.StorageV1().VolumeAttachments().Get(
		context.Background(), attachment.Name, metav1.GetOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, "192.0.2.10", updated.Status.AttachmentMetadata["iscsiTargetPortal"])
	assert.Empty(t, updated.Status.AttachmentMetadata[nvmeTargetIPsMetadataKey])
}

func testVolumeAttachment(volumeID, nodeID, sanType, targetIPs string) *storagev1.VolumeAttachment {
	metadata := map[string]string{"SANType": sanType}
	if sanType == storageattribute.NVMe {
		metadata[nvmeTargetIPsMetadataKey] = targetIPs
	}

	return &storagev1.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: "va-" + volumeID + "-" + nodeID + "-" + sanType},
		Spec: storagev1.VolumeAttachmentSpec{
			Attacher: csi.Provisioner,
			NodeName: nodeID,
			Source:   storagev1.VolumeAttachmentSource{PersistentVolumeName: &volumeID},
		},
		Status: storagev1.VolumeAttachmentStatus{
			Attached:           true,
			AttachmentMetadata: metadata,
		},
	}
}
