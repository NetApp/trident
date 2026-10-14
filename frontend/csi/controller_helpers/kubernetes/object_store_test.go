// Copyright 2026 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/netapp/trident/object"
	objectfake "github.com/netapp/trident/object/drivers/fake"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	tridentfake "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
)

func fakeObjectStore(t *testing.T) object.Store {
	t.Helper()
	store, err := objectfake.NewObjectDriver(context.Background(),
		[]byte(`{"user":"alice","password":"secret"}`),
		[]byte(`{"objectStoreName":"store-a","endpoint":"https://localhost"}`),
	)
	require.NoError(t, err)
	return store
}

func TestAddObjectStoreRollsBackSecretWhenCRCreateFails(t *testing.T) {
	kubeClient := k8sfake.NewSimpleClientset()
	tridentClient := tridentfake.NewSimpleClientset()
	tridentClient.PrependReactor("create", "tridentobjectstores", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("create failed")
	})
	h := &helper{namespace: "trident", kubeClient: kubeClient, tridentClient: tridentClient}

	err := h.AddObjectStore(context.Background(), fakeObjectStore(t))
	require.Error(t, err)

	_, getErr := kubeClient.CoreV1().Secrets("trident").Get(
		context.Background(), "store-a-creds", metav1.GetOptions{},
	)
	assert.True(t, apierrors.IsNotFound(getErr))
}

func TestDeleteObjectStoreDeletesCreatedSecret(t *testing.T) {
	store := fakeObjectStore(t)
	kubeClient := k8sfake.NewSimpleClientset()
	tridentClient := tridentfake.NewSimpleClientset()
	h := &helper{namespace: "trident", kubeClient: kubeClient, tridentClient: tridentClient}

	require.NoError(t, h.AddObjectStore(context.Background(), store))
	_, err := kubeClient.CoreV1().Secrets("trident").Create(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "user-managed-secret", Namespace: "trident"},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	require.NoError(t, h.DeleteObjectStore(context.Background(), store.Name()))

	_, err = tridentClient.TridentV1().TridentObjectStores("trident").Get(
		context.Background(), store.Name(), metav1.GetOptions{},
	)
	assert.True(t, apierrors.IsNotFound(err))
	_, err = kubeClient.CoreV1().Secrets("trident").Get(context.Background(), "store-a-creds", metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
	_, err = kubeClient.CoreV1().Secrets("trident").Get(
		context.Background(), "user-managed-secret", metav1.GetOptions{},
	)
	require.NoError(t, err)
}

func TestDeleteObjectStoreLeavesUserSecret(t *testing.T) {
	kubeClient := k8sfake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "user-managed-secret", Namespace: "trident"},
	})
	tridentClient := tridentfake.NewSimpleClientset(&netappv1.TridentObjectStore{
		ObjectMeta: metav1.ObjectMeta{Name: "store-a", Namespace: "trident"},
		Spec: netappv1.TridentObjectStoreSpec{RawExtension: runtime.RawExtension{Raw: []byte(
			`{"objectStoreDriver":"fake","credentials":{"secretName":"user-managed-secret"}}`,
		)}},
	})
	h := &helper{namespace: "trident", kubeClient: kubeClient, tridentClient: tridentClient}

	require.NoError(t, h.DeleteObjectStore(context.Background(), "store-a"))

	_, err := kubeClient.CoreV1().Secrets("trident").Get(
		context.Background(), "user-managed-secret", metav1.GetOptions{},
	)
	require.NoError(t, err)
}
