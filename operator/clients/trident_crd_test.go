// Copyright 2024 NetApp, Inc. All Rights Reserved.

package clients

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8stesting "k8s.io/client-go/testing"

	tridentV1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	fakeTridentClient "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
)

func getMockTridentClient() (TridentCRDClientInterface, *fakeTridentClient.Clientset) {
	fakeClient := fakeTridentClient.NewSimpleClientset()
	tridentCRDClient := NewTridentCRDClient(fakeClient)
	return tridentCRDClient, fakeClient
}

func TestNewTridentCRDClient_Success(t *testing.T) {
	client := fakeTridentClient.NewSimpleClientset()
	tridentCRDClient := NewTridentCRDClient(client)

	assert.NotNil(t, tridentCRDClient, "expected TridentCRDClient, got nil")
}

func TestTridentCRDClient_CheckTridentBackendConfigExists_Success(t *testing.T) {
	client, fakeClient := getMockTridentClient()

	fakeClient.PrependReactor("get", "tridentbackendconfigs", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &tridentV1.TridentBackendConfig{}, nil
	})

	exists, err := client.CheckTridentBackendConfigExists("test", "test-namespace")

	assert.NoError(t, err, "failed to check if backend config exists")
	assert.True(t, exists, "backend config doesn't exist")
}

func TestTridentCRDClient_CheckTridentBackendConfigExists_NotFound(t *testing.T) {
	client, fakeClient := getMockTridentClient()

	fakeClient.PrependReactor("get", "tridentbackendconfigs", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: "trident.netapp.io", Resource: "tridentbackendconfigs"}, "test")
	})

	exists, err := client.CheckTridentBackendConfigExists("test", "test-namespace")

	assert.NoError(t, err, "failed to check if backend config exists")
	assert.False(t, exists, "backend config exists")
}

func TestTridentCRDClient_CheckTridentBackendConfigExists_Error(t *testing.T) {
	client, fakeClient := getMockTridentClient()

	fakeClient.PrependReactor("get", "tridentbackendconfigs", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("random error")
	})

	exists, err := client.CheckTridentBackendConfigExists("test", "test-namespace")

	assert.Error(t, err, "check backend exists did not fail")
	assert.False(t, exists, "backend config exists")
}

func TestTridentCRDClient_GetTridentBackendConfig_Success(t *testing.T) {
	client, fakeClient := getMockTridentClient()

	fakeClient.PrependReactor("get", "tridentbackendconfigs", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &tridentV1.TridentBackendConfig{}, nil
	})

	backendConfig, err := client.GetTridentBackendConfig("test", "test-namespace")

	assert.NoError(t, err, "failed to get backend config")
	assert.NotNil(t, backendConfig, "expected TridentBackendConfig, got nil")
}

func TestTridentCRDClient_GetTridentBackendConfig_NotFound(t *testing.T) {
	client, fakeClient := getMockTridentClient()

	fakeClient.PrependReactor("get", "tridentbackendconfigs", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: "trident.netapp.io", Resource: "tridentbackendconfigs"}, "test")
	})

	backendConfig, err := client.GetTridentBackendConfig("test", "test-namespace")

	assert.Error(t, err, "found backend config")
	assert.Nil(t, backendConfig, "expected nil, got TridentBackendConfig")
}

func TestTridentCRDClient_PatchTridentBackendConfig_Success(t *testing.T) {
	client, fakeClient := getMockTridentClient()

	fakeClient.PrependReactor("patch", "tridentbackendconfigs", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &tridentV1.TridentBackendConfig{}, nil
	})

	err := client.PatchTridentBackendConfig("test", "test-namespace", []byte{}, types.MergePatchType)

	assert.NoError(t, err, "failed to patch backend config")
}

func TestTridentCRDClient_PatchTridentBackendConfig_Error(t *testing.T) {
	client, fakeClient := getMockTridentClient()

	fakeClient.PrependReactor("patch", "tridentbackendconfigs", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("random error")
	})

	err := client.PatchTridentBackendConfig("test", "test-namespace", []byte{}, types.MergePatchType)

	assert.Error(t, err, "patch backend config did not fail")
}

func TestTridentCRDClient_DeleteTridentBackendConfig_Success(t *testing.T) {
	client, fakeClient := getMockTridentClient()

	fakeClient.PrependReactor("delete", "tridentbackendconfigs", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, nil
	})

	err := client.DeleteTridentBackendConfig("test", "test-namespace")

	assert.NoError(t, err, "failed to delete backend config")
}

func TestTridentCRDClient_DeleteTridentBackendConfig_Error(t *testing.T) {
	client, fakeClient := getMockTridentClient()

	fakeClient.PrependReactor("delete", "tridentbackendconfigs", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("random error")
	})

	err := client.DeleteTridentBackendConfig("test", "test-namespace")

	assert.Error(t, err, "delete backend config did not fail")
}

func TestTridentCRDClient_ListTridentBackend_Success(t *testing.T) {
	client, fakeClient := getMockTridentClient()

	fakeClient.PrependReactor("list", "tridentbackends", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &tridentV1.TridentBackendList{}, nil
	})

	backendList, err := client.ListTridentBackend("test-namespace")

	assert.NoError(t, err, "failed to list trident backends")
	assert.NotNil(t, backendList, "expected TridentBackendList, got nil")
}

func TestTridentCRDClient_ListTridentBackend_Error(t *testing.T) {
	client, fakeClient := getMockTridentClient()

	fakeClient.PrependReactor("list", "tridentbackends", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("random error")
	})

	backendList, err := client.ListTridentBackend("test-namespace")

	assert.Error(t, err, "list trident backends did not fail")
	assert.Nil(t, backendList, "expected nil, got TridentBackendList")
}
