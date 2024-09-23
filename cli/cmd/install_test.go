// Copyright 2023 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	mockK8sClient "github.com/netapp/trident/mocks/mock_cli/mock_k8s_client"
	"github.com/netapp/trident/utils/errors"
)

var k8sClientError = errors.New("k8s error")

func newMockKubeClient(t *testing.T) *mockK8sClient.MockKubernetesClient {
	mockCtrl := gomock.NewController(t)
	mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)
	return mockKubeClient
}

func TestGetCRDMapFromBundle(t *testing.T) {
	CRDBundle := createCRDBundle(CRDnames)

	crdMap := getCRDMapFromBundle(CRDBundle)

	assert.Equal(t, len(CRDnames), len(crdMap), "Mismatch between number of CRD names ("+
		"count %v) and CRDs in the map (count %v)", len(CRDnames), len(crdMap))

	var missingCRDs []string
	for _, crdName := range CRDnames {
		if _, ok := crdMap[crdName]; !ok {
			missingCRDs = append(missingCRDs, crdName)
		}
	}

	if len(missingCRDs) > 0 {
		assert.Fail(t, "CRD map missing CRDs: %v", missingCRDs)
	}
}

func TestValidateCRDsPass(t *testing.T) {
	CRDBundle := createCRDBundle(CRDnames)

	crdMap := getCRDMapFromBundle(CRDBundle)

	assert.Nil(t, validateCRDs(crdMap), "CRD validation failed")
}

func TestValidateCRDsMissingCRDsFail(t *testing.T) {
	CRDBundle := createCRDBundle(CRDnames[1 : len(CRDnames)-1])

	crdMap := getCRDMapFromBundle(CRDBundle)

	assert.NotNil(t, validateCRDs(crdMap), "CRD validation should fail")
}

func TestValidateCRDsExtraCRDsFail(t *testing.T) {
	newCRDNames := append(CRDnames, "xyz.trident.netapp.io")

	CRDBundle := createCRDBundle(newCRDNames)

	crdMap := getCRDMapFromBundle(CRDBundle)

	assert.NotNil(t, validateCRDs(crdMap), "CRD validation should fail")
}

func createCRDBundle(crdNames []string) string {
	var crdBundle string
	for i, crdName := range crdNames {
		crdBundle = crdBundle + fmt.Sprintf(CRDTemplate, crdName)

		if i != len(crdNames)-1 {
			crdBundle = crdBundle + "\n---"
		}
	}

	return crdBundle
}

func TestCreateCRD(t *testing.T) {
	mockKubeClient := newMockKubeClient(t)
	client = mockKubeClient
	k8sTimeout = 100 * time.Millisecond
	name := VersionCRDName
	yaml := k8sclient.GetVersionCRDYAML()
	crd := &v1.CustomResourceDefinition{
		Status: v1.CustomResourceDefinitionStatus{
			Conditions: []v1.CustomResourceDefinitionCondition{
				{
					Status: v1.ConditionTrue,
					Type:   v1.Established,
				},
			},
		},
	}

	mockKubeClient.EXPECT().CreateObjectByYAML(yaml).Return(k8sClientError)
	expectedErr := createCRD(name, yaml)
	assert.NotNil(t, expectedErr, "expected non-nil error")

	// CRD returned from GetCRD has the appropriate fields and returns safely
	mockKubeClient.EXPECT().CreateObjectByYAML(yaml).Return(nil)
	mockKubeClient.EXPECT().GetCRD(name).Return(crd, nil).AnyTimes()
	expectedErr = createCRD(name, yaml)
	assert.Nil(t, expectedErr, "expected nil error")

	// Set the crd status conditions false to force a failure in ensureCRDEstablished
	crd.Status.Conditions[0].Status = v1.ConditionFalse
	mockKubeClient.EXPECT().CreateObjectByYAML(yaml).Return(nil)
	mockKubeClient.EXPECT().GetCRD(name).Return(crd, k8sClientError).AnyTimes()
	mockKubeClient.EXPECT().DeleteObjectByYAML(yaml, true).Return(nil)

	expectedErr = createCRD(name, yaml)
	assert.NotNil(t, expectedErr, "expected non-nil error")

	// Set the crd status conditions false to force a failure in ensureCRDEstablished
	crd.Status.Conditions[0].Status = v1.ConditionFalse
	mockKubeClient.EXPECT().CreateObjectByYAML(yaml).Return(nil)
	mockKubeClient.EXPECT().GetCRD(name).Return(crd, k8sClientError).AnyTimes()
	mockKubeClient.EXPECT().DeleteObjectByYAML(yaml, true).Return(k8sClientError)

	expectedErr = createCRD(name, yaml)
	assert.NotNil(t, expectedErr, "expected non-nil error")

	client = nil // reset the client to nil
}

func TestCreateCustomResourceDefinition(t *testing.T) {
	mockKubeClient := newMockKubeClient(t)
	client = mockKubeClient
	name := VersionCRDName
	yaml := k8sclient.GetVersionCRDYAML()

	mockKubeClient.EXPECT().CreateObjectByYAML(yaml).Return(k8sClientError)
	expectedErr := createCustomResourceDefinition(name, yaml)
	assert.NotNil(t, expectedErr, "expected non-nil error")

	mockKubeClient.EXPECT().CreateObjectByYAML(yaml).Return(nil)
	expectedErr = createCustomResourceDefinition(name, yaml)
	assert.Nil(t, expectedErr, "expected nil error")

	client = nil // reset the client to nil
}

func TestPatchCRD(t *testing.T) {
	mockKubeClient := newMockKubeClient(t)
	client = mockKubeClient
	k8sTimeout = 100 * time.Millisecond
	name := VersionCRDName
	yaml := k8sclient.GetVersionCRDYAML()
	crd := &v1.CustomResourceDefinition{
		Status: v1.CustomResourceDefinitionStatus{
			Conditions: []v1.CustomResourceDefinitionCondition{
				{
					Status: v1.ConditionTrue,
					Type:   v1.Established,
				},
			},
		},
	}

	// Fail at GetCRD
	mockKubeClient.EXPECT().GetCRD(name).Return(crd, k8sClientError)
	expectedErr := patchCRD(name, yaml)
	assert.NotNil(t, expectedErr, "expected non-nil error")

	// Fail at PatchCRD
	mockKubeClient.EXPECT().GetCRD(name).Return(crd, nil)
	mockKubeClient.EXPECT().PatchCRD(name, gomock.Any(), types.MergePatchType).Return(k8sClientError)
	expectedErr = patchCRD(name, yaml)
	assert.NotNil(t, expectedErr, "expected non-nil error")

	// Succeed at PatchCRD
	mockKubeClient.EXPECT().GetCRD(name).Return(crd, nil)
	mockKubeClient.EXPECT().PatchCRD(name, gomock.Any(), types.MergePatchType).Return(nil)
	expectedErr = patchCRD(name, yaml)
	assert.Nil(t, expectedErr, "expected nil error")

	client = nil // reset the client to nil
}

func TestUpdateCustomResourceDefinition(t *testing.T) {
	mockKubeClient := newMockKubeClient(t)
	client = mockKubeClient
	name := VersionCRDName
	yaml := k8sclient.GetVersionCRDYAML()
	crd := &v1.CustomResourceDefinition{
		Status: v1.CustomResourceDefinitionStatus{
			Conditions: []v1.CustomResourceDefinitionCondition{
				{
					Status: v1.ConditionTrue,
					Type:   v1.Established,
				},
			},
		},
	}
	patchType := types.MergePatchType

	mockKubeClient.EXPECT().GetCRD(name).Return(crd, k8sClientError)
	expectedErr := patchCRD(name, yaml)
	assert.NotNil(t, expectedErr, "expected non-nil error")

	mockKubeClient.EXPECT().GetCRD(name).Return(crd, nil)
	mockKubeClient.EXPECT().PatchCRD(name, gomock.Any(), patchType).Return(k8sClientError)
	expectedErr = patchCRD(name, yaml)
	assert.NotNil(t, expectedErr, "expected non-nil error")

	mockKubeClient.EXPECT().GetCRD(name).Return(crd, nil)
	mockKubeClient.EXPECT().PatchCRD(name, gomock.Any(), patchType).Return(nil)
	expectedErr = patchCRD(name, yaml)
	assert.Nil(t, expectedErr, "expected nil error")

	client = nil // reset the client to nil
}

var CRDTemplate = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: %v
spec:
  group: trident.netapp.io`

func TestValidateInstallationArguments(t *testing.T) {
	tests := []struct {
		name                string
		tridentPodNamespace string
		logFormat           string
		imagePullPolicy     string
		cloudProvider       string
		cloudIdentity       string
		nodePrep            []string
		assertValid         assert.ErrorAssertionFunc
	}{
		// Valid arguments
		{"default namespace", "default", "text", "IfNotPresent", "", "", nil, assert.NoError},
		{"test namespace", "test-namespace", "text", "IfNotPresent", "", "", []string{}, assert.NoError},
		{"image pull never", "test-namespace", "json", "Never", "", "", []string{"iscsi"}, assert.NoError},
		{"cloud provider azure", "test-namespace", "json", "Never", k8sclient.CloudProviderAzure, "", []string{"iscsi"}, assert.NoError},
		{"azure cloud identity key", "test", "text", "Always", k8sclient.CloudProviderAzure, k8sclient.AzureCloudIdentityKey + " a8rry78r8-7733-49bd-6656582", []string{}, assert.NoError},
		{"image pull always", "test", "text", "Always", k8sclient.CloudProviderAzure, "", []string{}, assert.NoError},

		// Invalid arguments
		{"invalid namespace", "", "", "", "", "", []string{}, assert.Error},
		{"invalid log format", "test", "html", "", "", "", []string{}, assert.Error},
		{"invalid image pull policy", "test", "text", "Anyways", "", "", []string{}, assert.Error},
		{"invalid cloud provider", "test", "json", "Never", "Docker", "", []string{}, assert.Error},
		{"invalid cloud provider with azure key", "test", "json", "Never", "Docker", k8sclient.AzureCloudIdentityKey + " a8rry78r8-7733-49bd-6656582", []string{}, assert.Error},
		{"invalid blank cloud identity", "test", "text", "IfNotPresent", k8sclient.CloudProviderAWS, "", []string{}, assert.Error},
		{"invalid blank cloud provider", "test", "text", "IfNotPresent", "", k8sclient.AzureCloudIdentityKey + " a8rry78r8-7733-49bd-6656582", []string{}, assert.Error},
		{"invalid cloud identity", "test", "text", "Always", k8sclient.CloudProviderAzure, "a8rry78r8-7733-49bd-6656582", []string{}, assert.Error},
		{"invalid blank node prep", "default", "text", "IfNotPresent", "", "", []string{""}, assert.Error},
		{"invalid only node prep", "default", "text", "IfNotPresent", "", "", []string{"NVME"}, assert.Error},
		{"invalid node prep in list", "default", "text", "IfNotPresent", "", "", []string{"iscsi", "nvme"}, assert.Error},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// set global variables that are used in validateInstallationArguments
			TridentPodNamespace = test.tridentPodNamespace
			logFormat = test.logFormat
			imagePullPolicy = test.imagePullPolicy
			cloudProvider = test.cloudProvider
			cloudIdentity = test.cloudIdentity
			nodePrep = test.nodePrep
			err := validateInstallationArguments()
			test.assertValid(t, err)
		})
	}
}
