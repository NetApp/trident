// Copyright 2024 NetApp, Inc. All Rights Reserved.

package clients

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fakeK8sClient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"

	mockK8sClient "github.com/netapp/trident/mocks/mock_cli/mock_k8s_client"
)

func getMockK8sClient(t *testing.T) (ExtendedK8sClientInterface, *fakeK8sClient.Clientset) {
	kubeClient := fakeK8sClient.NewSimpleClientset()
	ctrl := gomock.NewController(t)
	tridentKubernetesClient := mockK8sClient.NewMockKubernetesClient(ctrl)
	extendedClient := &K8sClient{
		KubernetesClient: tridentKubernetesClient,
		KubeClient:       kubeClient,
	}

	return extendedClient, kubeClient
}

func TestNewExtendedK8sClient_WithNilClient(t *testing.T) {
	_, err := NewExtendedK8sClient(&rest.Config{}, "default", 30, nil)
	assert.Error(t, err, "created extended k8s client with nil client")
}

func TestNewExtendedK8sClient_WithFailedKubeClientCreation(t *testing.T) {
	client := fakeK8sClient.NewSimpleClientset()
	_, err := NewExtendedK8sClient(&rest.Config{}, "default", -1, client)
	assert.Error(t, err, "created extended k8s client with invalid timeout")
}

func TestK8sClient_CheckStorageClassExists_StorageClassExists(t *testing.T) {
	extendedClient, kubeClient := getMockK8sClient(t)

	_, _ = kubeClient.StorageV1().StorageClasses().
		Create(ctx, &v1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "test"}}, metav1.CreateOptions{})

	exists, _ := extendedClient.CheckStorageClassExists("test")

	assert.True(t, exists, "storage class doesn't exist")
}

func TestK8sClient_CheckStorageClassExists_StorageClassDoesNotExist(t *testing.T) {
	extendedClient, _ := getMockK8sClient(t)
	exists, _ := extendedClient.CheckStorageClassExists("test")
	assert.False(t, exists, "storage class exists when it shouldn't")
}

func TestK8sClient_CheckStorageClassExists_Error(t *testing.T) {
	extendedClient, kubeClient := getMockK8sClient(t)

	kubeClient.PrependReactor("get", "storageclasses", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("forced error")
	})

	_, err := extendedClient.CheckStorageClassExists("test")

	assert.Error(t, err, "check storage class exists did not fail")
}

func TestK8sClient_PatchStorageClass_Success(t *testing.T) {
	extendedClient, kubeClient := getMockK8sClient(t)

	_, _ = kubeClient.StorageV1().StorageClasses().Create(ctx, &v1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "test"}}, metav1.CreateOptions{})

	err := extendedClient.PatchStorageClass("test", []byte(`{"metadata":{"annotations":{"foo":"bar"}}}`), types.MergePatchType)

	assert.NoError(t, err, "patch storage class failed")
}

func TestK8sClient_PatchStorageClass_Error(t *testing.T) {
	extendedClient, kubeClient := getMockK8sClient(t)

	kubeClient.PrependReactor("patch", "storageclasses", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("forced error")
	})

	err := extendedClient.PatchStorageClass("test", []byte(`{"metadata":{"annotations":{"foo":"bar"}}}`), types.MergePatchType)

	assert.Error(t, err, "patch storage class did not fail")
}

func TestK8sClient_DeleteStorageClass_Success(t *testing.T) {
	extendedClient, kubeClient := getMockK8sClient(t)

	_, _ = kubeClient.StorageV1().StorageClasses().Create(ctx, &v1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "test"}}, metav1.CreateOptions{})

	err := extendedClient.DeleteStorageClass("test")

	assert.NoError(t, err, "delete storage class failed")
}

func TestK8sClient_DeleteStorageClass_Error(t *testing.T) {
	extendedClient, kubeClient := getMockK8sClient(t)

	kubeClient.PrependReactor("delete", "storageclasses", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("forced error")
	})

	err := extendedClient.DeleteStorageClass("test")

	assert.Error(t, err, "delete storage class did not fail")
}
