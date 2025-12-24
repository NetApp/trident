// Copyright 2024 NetApp, Inc. All Rights Reserved.

package clients

import (
	"fmt"
	"time"

	v1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
)

// TODO(sphadnis): Combine this Extended client with orchestrator extended client

type K8sClient struct {
	k8sclient.KubernetesClient
	KubeClient kubernetes.Interface
}

func NewExtendedK8sClient(
	config *rest.Config, namespace string, timeout int, client kubernetes.Interface,
) (ExtendedK8sClientInterface, error) {
	if client == nil {
		return nil, fmt.Errorf("invalid k8s clientset")
	}

	if timeout <= 0 {
		k8sTimeout = DefaultK8sClientTimeout
	}
	k8sTimeout = time.Duration(timeout) * time.Second

	// Create the Kubernetes client
	kubeClient, err := k8sclient.NewKubeClient(config, namespace, k8sTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Kubernetes client; %v", err)
	}

	return &K8sClient{
		KubernetesClient: kubeClient,
		KubeClient:       client,
	}, nil
}

func (kc *K8sClient) CheckStorageClassExists(name string) (bool, error) {
	if _, err := kc.GetStorageClass(name); err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (kc *K8sClient) GetStorageClass(name string) (*v1.StorageClass, error) {
	return kc.KubeClient.StorageV1().StorageClasses().Get(ctx, name, getOpts)
}

func (kc *K8sClient) PatchStorageClass(name string, patchBytes []byte, patchType types.PatchType) error {
	_, err := kc.KubeClient.StorageV1().StorageClasses().Patch(ctx, name, patchType, patchBytes, patchOpts)
	return err
}

func (kc *K8sClient) DeleteStorageClass(name string) error {
	return kc.KubeClient.StorageV1().StorageClasses().Delete(ctx, name, deleteOpts)
}

func (kc *K8sClient) ListStorageClassesByLabel(labelKey, labelValue string) (*v1.StorageClassList, error) {
	labelSelector := fmt.Sprintf("%s=%s", labelKey, labelValue)
	return kc.KubeClient.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
}
