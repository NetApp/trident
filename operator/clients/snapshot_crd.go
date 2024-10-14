// Copyright 2024 NetApp, Inc. All Rights Reserved.

package clients

import (
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	snapshotClient "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type SnapshotCRDClient struct {
	client snapshotClient.Interface
}

func NewSnapshotCRDClient(client snapshotClient.Interface) SnapshotCRDClientInterface {
	return &SnapshotCRDClient{client: client}
}

func (sc *SnapshotCRDClient) CheckVolumeSnapshotClassExists(name string) (bool, error) {
	if _, err := sc.GetVolumeSnapshotClass(name); err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (sc *SnapshotCRDClient) GetVolumeSnapshotClass(name string) (*snapshotv1.VolumeSnapshotClass, error) {
	return sc.client.SnapshotV1().VolumeSnapshotClasses().Get(ctx, name, getOpts)
}

func (sc *SnapshotCRDClient) PatchVolumeSnapshotClass(name string, patchBytes []byte, patchType types.PatchType) error {
	_, err := sc.client.SnapshotV1().VolumeSnapshotClasses().Patch(ctx, name, patchType, patchBytes, patchOpts)
	return err
}

func (sc *SnapshotCRDClient) DeleteVolumeSnapshotClass(name string) error {
	return sc.client.SnapshotV1().VolumeSnapshotClasses().Delete(ctx, name, deleteOpts)
}
