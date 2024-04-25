// Copyright 2024 NetApp, Inc. All Rights Reserved.

package clients

import (
	"fmt"
	"testing"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	fakeSnapshotClient "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8stesting "k8s.io/client-go/testing"
)

func getMockSnapshotClient() (SnapshotCRDClientInterface, *fakeSnapshotClient.Clientset) {
	fakeClient := fakeSnapshotClient.NewSimpleClientset()
	snapshotClient := NewSnapshotCRDClient(fakeClient)
	return snapshotClient, fakeClient
}

func TestNewSnapshotCRDClient_Success(t *testing.T) {
	client := fakeSnapshotClient.NewSimpleClientset()
	snapshotClient := NewSnapshotCRDClient(client)

	assert.NotNil(t, snapshotClient, "expected SnapshotCRDClient, got nil")
}

func TestSnapshotCRDClient_CheckVolumeSnapshotClassExists_Success(t *testing.T) {
	client, fakeClient := getMockSnapshotClient()

	fakeClient.PrependReactor("get", "volumesnapshotclasses", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &snapshotv1.VolumeSnapshotClass{}, nil
	})

	exists, err := client.CheckVolumeSnapshotClassExists("test")

	assert.NoError(t, err, "failed to check if volume snapshots class exist")
	assert.True(t, exists, "volume snapshot class doesn't exist")
}

func TestSnapshotCRDClient_CheckVolumeSnapshotClassExists_NotFound(t *testing.T) {
	client, fakeClient := getMockSnapshotClient()

	fakeClient.PrependReactor("get", "volumesnapshotclasses", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: "snapshot.storage.k8s.io", Resource: "volumesnapshotclasses"}, "test")
	})

	exists, err := client.CheckVolumeSnapshotClassExists("test")

	assert.NoError(t, err, "failed to check if volume snapshots class exist")
	assert.False(t, exists, "volume snapshot class exists")
}

func TestSnapshotCRDClient_CheckVolumeSnapshotClassExists_Error(t *testing.T) {
	client, fakeClient := getMockSnapshotClient()

	fakeClient.PrependReactor("get", "volumesnapshotclasses", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("random error")
	})

	exists, err := client.CheckVolumeSnapshotClassExists("test")

	assert.Error(t, err, "no failure seen while checking if volume snapshot class exists")
	assert.False(t, exists, "volume snapshot class exists")
}

func TestSnapshotCRDClient_GetVolumeSnapshotClass_Success(t *testing.T) {
	client, fakeClient := getMockSnapshotClient()

	fakeClient.PrependReactor("get", "volumesnapshotclasses", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &snapshotv1.VolumeSnapshotClass{}, nil
	})

	snapshotClass, err := client.GetVolumeSnapshotClass("test")

	assert.NoError(t, err, "failed to get VolumeSnapshotClass")
	assert.NotNil(t, snapshotClass, "expected VolumeSnapshotClass, got nil")
}

func TestSnapshotCRDClient_GetVolumeSnapshotClass_NotFound(t *testing.T) {
	client, fakeClient := getMockSnapshotClient()

	fakeClient.PrependReactor("get", "volumesnapshotclasses", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: "snapshot.storage.k8s.io", Resource: "volumesnapshotclasses"}, "test")
	})

	snapshotClass, err := client.GetVolumeSnapshotClass("test")

	assert.Error(t, err, "no failure seen while getting VolumeSnapshotClass")
	assert.Nil(t, snapshotClass, "expected nil, got VolumeSnapshotClass")
}

func TestSnapshotCRDClient_PatchVolumeSnapshotClass_Success(t *testing.T) {
	client, fakeClient := getMockSnapshotClient()

	fakeClient.PrependReactor("patch", "volumesnapshotclasses", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &snapshotv1.VolumeSnapshotClass{}, nil
	})

	err := client.PatchVolumeSnapshotClass("test", []byte{}, types.MergePatchType)

	assert.NoError(t, err, "failed to patch VolumeSnapshotClass")
}

func TestSnapshotCRDClient_PatchVolumeSnapshotClass_Error(t *testing.T) {
	client, fakeClient := getMockSnapshotClient()

	fakeClient.PrependReactor("patch", "volumesnapshotclasses", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("random error")
	})

	err := client.PatchVolumeSnapshotClass("test", []byte{}, types.MergePatchType)

	assert.Error(t, err, "no failure seen while patching VolumeSnapshotClass")
}

func TestSnapshotCRDClient_DeleteVolumeSnapshotClass_Success(t *testing.T) {
	client, fakeClient := getMockSnapshotClient()

	fakeClient.PrependReactor("delete", "volumesnapshotclasses", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, nil
	})

	err := client.DeleteVolumeSnapshotClass("test")

	assert.NoError(t, err, "failed to delete VolumeSnapshotClass")
}

func TestSnapshotCRDClient_DeleteVolumeSnapshotClass_Error(t *testing.T) {
	client, fakeClient := getMockSnapshotClient()

	fakeClient.PrependReactor("delete", "volumesnapshotclasses", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("random error")
	})

	err := client.DeleteVolumeSnapshotClass("test")

	assert.Error(t, err, "no failure seen while deleting VolumeSnapshotClass")
}
