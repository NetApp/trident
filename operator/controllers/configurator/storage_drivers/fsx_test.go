// Copyright 2025 NetApp, Inc. All Rights Reserved.

package storage_drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	mockConfClients "github.com/netapp/trident/mocks/mock_operator/mock_controllers/mock_configurator/mock_clients"
	confClients "github.com/netapp/trident/operator/controllers/configurator/clients"
	operatorV1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
	tridentV1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	sa "github.com/netapp/trident/storage_attribute"
)

var ctxBg = context.Background()

const (
	// Test identifiers
	testFsxnID        = "fs-1234567890abcdef0"
	testSvmName       = "test-svm"
	testSecretARN     = "arn:aws:secretsmanager:us-east-1:123456789012:secret:test-secret-AbCdEf"
	testManagementLIF = "10.0.0.100"
	testAWSRegion     = "us-east-1"

	// Test names and prefixes
	testFSxConfiguratorName = "test-fsx-configurator"
	testTridentNamespace    = "trident"
	testBackendPrefix       = "trident-"
	testStorageDriverName   = "ontap-nas"
	testSecretName          = "test-secret"
	testUsername            = "testuser"
	testPassword            = "testpass"

	// Error messages
	errEmptyTorcCR            = "empty torc CR"
	errEmptyAWSFSxNConfig     = "empty AWS FSxN configurator CR"
	errInvalidClient          = "invalid client"
	errAWSRegionNotSet        = "AWS_REGION is not set"
	errInvalidCharacter       = "invalid character"
	errFailedToCreateBackend  = "failed to create backend"
	errFailedToCreateSC       = "failed to create storage class"
	errFailedToCreateSnapshot = "failed to create snapshot class"
	errFailedToDeleteBackend  = "failed to delete backend"
	errFailedToDeleteSC       = "failed to delete storage class"
	errCreateOrPatchObject    = "error creating or patching object"
	errDeleteBackend          = "error occurred while deleting backend"
	errDeleteStorageClass     = "error occurred while deleting storage class"
	errListTBCObjects         = "error occurred while listing TBC objects"

	// Test data values
	testPasswordLength      = 10
	testSecretDataSize      = 2
	testBackendCount        = 2
	testStorageClassCount   = 2
	testProtocolsCount      = 2
	testSingleProtocolCount = 1
	testEmptyListSize       = 0
	testSingleItemSize      = 1

	// JSON test data
	invalidJSONData = "invalid json"
	validJSONPrefix = `{"storageDriverName":"ontap-nas","svms":[`
	validJSONSuffix = `]}`
)

func getTestTorcCR() *operatorV1.TridentOrchestrator {
	return &operatorV1.TridentOrchestrator{
		Spec: operatorV1.TridentOrchestratorSpec{
			Namespace: testTridentNamespace,
		},
	}
}

func getTestTconfCR(svms []SVM) *operatorV1.TridentConfigurator {
	awsConfig := AwsConfig{
		StorageDriverName: testStorageDriverName,
		SVMs:              svms,
	}
	configBytes, _ := json.Marshal(awsConfig)

	return &operatorV1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{
			Name: testFSxConfiguratorName,
		},
		Spec: operatorV1.TridentConfiguratorSpec{
			RawExtension: runtime.RawExtension{
				Raw: configBytes,
			},
		},
	}
}

// Test NewFSxNInstance
func TestNewFSxNInstance_Success(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	torcCR := getTestTorcCR()
	svms := []SVM{{FsxnID: testFsxnID, Protocols: []string{sa.NFS}}}
	tconfCR := getTestTconfCR(svms)

	aws, err := NewFSxNInstance(torcCR, tconfCR, mockClient)

	assert.NoError(t, err)
	assert.NotNil(t, aws)
	assert.Equal(t, testFSxConfiguratorName, aws.TBCNamePrefix)
	assert.Equal(t, testTridentNamespace, aws.TridentNamespace)
	assert.Equal(t, testStorageDriverName, aws.StorageDriverName)
	assert.Len(t, aws.SVMs, 1)
	assert.Equal(t, testFsxnID, aws.SVMs[0].FsxnID)
}

func TestNewFSxNInstance_ErrorCases(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	tests := []struct {
		name        string
		torcCR      *operatorV1.TridentOrchestrator
		tconfCR     *operatorV1.TridentConfigurator
		client      interface{}
		expectedErr string
	}{
		{
			name:        "nil TridentOrchestrator",
			torcCR:      nil,
			tconfCR:     getTestTconfCR([]SVM{}),
			client:      mockClient,
			expectedErr: errEmptyTorcCR,
		},
		{
			name:        "nil TridentConfigurator",
			torcCR:      getTestTorcCR(),
			tconfCR:     nil,
			client:      mockClient,
			expectedErr: errEmptyAWSFSxNConfig,
		},
		{
			name:        "nil client",
			torcCR:      getTestTorcCR(),
			tconfCR:     getTestTconfCR([]SVM{}),
			client:      nil,
			expectedErr: errInvalidClient,
		},
		{
			name:   "invalid JSON in configurator CR",
			torcCR: getTestTorcCR(),
			tconfCR: &operatorV1.TridentConfigurator{
				ObjectMeta: metav1.ObjectMeta{
					Name: testFSxConfiguratorName,
				},
				Spec: operatorV1.TridentConfiguratorSpec{
					RawExtension: runtime.RawExtension{
						Raw: []byte(invalidJSONData),
					},
				},
			},
			client:      mockClient,
			expectedErr: errInvalidCharacter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var client confClients.ConfiguratorClientInterface
			if tt.client != nil {
				client = tt.client.(*mockConfClients.MockConfiguratorClientInterface)
			}
			aws, err := NewFSxNInstance(tt.torcCR, tt.tconfCR, client)

			assert.Error(t, err)
			if err != nil {
				assert.Contains(t, err.Error(), tt.expectedErr)
			}
			assert.Nil(t, aws)
		})
	}
}

// Test Validate method - only error cases that don't require AWS client
func TestAWS_Validate_MissingAWSRegion(t *testing.T) {
	os.Unsetenv(AWSRegion) // Ensure it's not set

	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		AwsConfig: AwsConfig{
			StorageDriverName: testStorageDriverName,
			SVMs: []SVM{
				{FsxnID: testFsxnID, Protocols: []string{sa.NFS}},
			},
		},
		ConfClient:       mockClient,
		TBCNamePrefix:    testFSxConfiguratorName,
		TridentNamespace: testTridentNamespace,
	}

	err := aws.Validate()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), errAWSRegionNotSet)
}

// Test Create method
func TestAWS_Create_Success(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		AwsConfig: AwsConfig{
			StorageDriverName: testStorageDriverName,
			SVMs: []SVM{
				{
					FsxnID:        testFsxnID,
					Protocols:     []string{sa.NFS, sa.ISCSI},
					ManagementLIF: testManagementLIF,
					SecretARNName: testSecretARN,
				},
			},
		},
		ConfClient:       mockClient,
		TBCNamePrefix:    testFSxConfiguratorName,
		TridentNamespace: testTridentNamespace,
	}

	// Mock backend creation for both protocols
	mockClient.EXPECT().CreateOrPatchObject(gomock.Any(), testBackendPrefix+testFsxnID+"-nfs", testTridentNamespace, gomock.Any()).Return(nil)
	mockClient.EXPECT().CreateOrPatchObject(gomock.Any(), testBackendPrefix+testFsxnID+"-iscsi", testTridentNamespace, gomock.Any()).Return(nil)

	backends, err := aws.Create()

	assert.NoError(t, err)
	assert.Len(t, backends, testBackendCount)
	assert.Contains(t, backends, testBackendPrefix+testFsxnID+"-nfs")
	assert.Contains(t, backends, testBackendPrefix+testFsxnID+"-iscsi")
}

func TestAWS_Create_BackendCreateError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		AwsConfig: AwsConfig{
			StorageDriverName: testStorageDriverName,
			SVMs: []SVM{
				{
					FsxnID:        testFsxnID,
					Protocols:     []string{sa.NFS},
					ManagementLIF: testManagementLIF,
					SecretARNName: testSecretARN,
				},
			},
		},
		ConfClient:       mockClient,
		TBCNamePrefix:    testFSxConfiguratorName,
		TridentNamespace: testTridentNamespace,
	}

	// Mock first backend creation failure
	mockClient.EXPECT().CreateOrPatchObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(fmt.Errorf(errFailedToCreateBackend))

	backends, err := aws.Create()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), errCreateOrPatchObject)
	assert.Nil(t, backends)
}

// Test CreateStorageClass method
func TestAWS_CreateStorageClass_Success(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		AwsConfig: AwsConfig{
			StorageDriverName: testStorageDriverName,
			SVMs: []SVM{
				{
					FsxnID:    testFsxnID,
					Protocols: []string{sa.NFS, sa.ISCSI},
				},
			},
		},
		ConfClient:       mockClient,
		TBCNamePrefix:    testFSxConfiguratorName,
		TridentNamespace: testTridentNamespace,
	}

	// Mock storage class creation for both protocols
	mockClient.EXPECT().CreateOrPatchObject(gomock.Any(), testBackendPrefix+testFsxnID+"-nfs", testTridentNamespace, gomock.Any()).Return(nil)
	mockClient.EXPECT().CreateOrPatchObject(gomock.Any(), testBackendPrefix+testFsxnID+"-iscsi", testTridentNamespace, gomock.Any()).Return(nil)

	err := aws.CreateStorageClass()

	assert.NoError(t, err)
}

func TestAWS_CreateStorageClass_Error(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		AwsConfig: AwsConfig{
			StorageDriverName: testStorageDriverName,
			SVMs: []SVM{
				{
					FsxnID:    testFsxnID,
					Protocols: []string{sa.NFS},
				},
			},
		},
		ConfClient:       mockClient,
		TBCNamePrefix:    testFSxConfiguratorName,
		TridentNamespace: testTridentNamespace,
	}

	// Mock storage class creation failure
	mockClient.EXPECT().CreateOrPatchObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(fmt.Errorf(errFailedToCreateSC))

	err := aws.CreateStorageClass()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), errCreateOrPatchObject)
}

// Test CreateSnapshotClass method
func TestAWS_CreateSnapshotClass_Success(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		ConfClient: mockClient,
	}

	mockClient.EXPECT().CreateOrPatchObject(gomock.Any(), NetAppSnapshotClassName, "", gomock.Any()).Return(nil)

	err := aws.CreateSnapshotClass()

	assert.NoError(t, err)
}

func TestAWS_CreateSnapshotClass_Error(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		ConfClient: mockClient,
	}

	mockClient.EXPECT().CreateOrPatchObject(gomock.Any(), NetAppSnapshotClassName, "", gomock.Any()).
		Return(fmt.Errorf(errFailedToCreateSnapshot))

	err := aws.CreateSnapshotClass()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), errFailedToCreateSnapshot)
}

// Test GetCloudProvider method
func TestAWS_GetCloudProvider(t *testing.T) {
	aws := &AWS{}

	cloudProvider := aws.GetCloudProvider()

	assert.Equal(t, k8sclient.CloudProviderAWS, cloudProvider)
}

// Test DeleteBackend method
func TestAWS_DeleteBackend_Success(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		ConfClient:       mockClient,
		TridentNamespace: testTridentNamespace,
	}

	request := map[string]interface{}{
		"FSxNID":    testFsxnID,
		"protocols": []string{sa.NFS, sa.ISCSI},
	}

	mockClient.EXPECT().DeleteObject(gomock.Any(), testBackendPrefix+testFsxnID+"-nfs", testTridentNamespace).Return(nil)
	mockClient.EXPECT().DeleteObject(gomock.Any(), testBackendPrefix+testFsxnID+"-iscsi", testTridentNamespace).Return(nil)

	err := aws.DeleteBackend(request)

	assert.NoError(t, err)
}

func TestAWS_DeleteBackend_Error(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		ConfClient:       mockClient,
		TridentNamespace: testTridentNamespace,
	}

	request := map[string]interface{}{
		"FSxNID":    testFsxnID,
		"protocols": []string{sa.NFS},
	}

	mockClient.EXPECT().DeleteObject(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(fmt.Errorf(errFailedToDeleteBackend))

	err := aws.DeleteBackend(request)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), errDeleteBackend)
}

// Test DeleteStorageClass method
func TestAWS_DeleteStorageClass_Success(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		ConfClient:       mockClient,
		TridentNamespace: testTridentNamespace,
	}

	request := map[string]interface{}{
		"FSxNID":    testFsxnID,
		"protocols": []string{sa.NFS, sa.ISCSI},
	}

	mockClient.EXPECT().DeleteObject(gomock.Any(), testBackendPrefix+testFsxnID+"-nfs", testTridentNamespace).Return(nil)
	mockClient.EXPECT().DeleteObject(gomock.Any(), testBackendPrefix+testFsxnID+"-iscsi", testTridentNamespace).Return(nil)

	err := aws.DeleteStorageClass(request)

	assert.NoError(t, err)
}

func TestAWS_DeleteStorageClass_Error(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		ConfClient:       mockClient,
		TridentNamespace: testTridentNamespace,
	}

	request := map[string]interface{}{
		"FSxNID":    testFsxnID,
		"protocols": []string{sa.NFS},
	}

	mockClient.EXPECT().DeleteObject(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(fmt.Errorf(errFailedToDeleteSC))

	err := aws.DeleteStorageClass(request)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), errDeleteStorageClass)
}

// Test DeleteSnapshotClass method
func TestAWS_DeleteSnapshotClass_Success(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		ConfClient: mockClient,
	}

	// Mock empty backend list (should delete snapshot class)
	mockClient.EXPECT().ListObjects(gomock.Any(), "").Return(&tridentV1.TridentBackendList{}, nil)
	mockClient.EXPECT().DeleteObject(gomock.Any(), NetAppSnapshotClassName, "").Return(nil)

	err := aws.DeleteSnapshotClass()

	assert.NoError(t, err)
}

func TestAWS_DeleteSnapshotClass_BackendsExist(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		ConfClient: mockClient,
	}

	// Mock non-empty backend list (should not delete snapshot class)
	backendList := &tridentV1.TridentBackendList{
		Items: []*tridentV1.TridentBackend{{}}, // Non-empty list
	}
	mockClient.EXPECT().ListObjects(gomock.Any(), "").Return(backendList, nil)

	err := aws.DeleteSnapshotClass()

	assert.NoError(t, err)
}

func TestAWS_DeleteSnapshotClass_ListError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		ConfClient: mockClient,
	}

	mockClient.EXPECT().ListObjects(gomock.Any(), "").Return(nil, fmt.Errorf("failed to list backends"))

	err := aws.DeleteSnapshotClass()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), errListTBCObjects)
}

// Test utility functions
func TestGetFSxNBackendName(t *testing.T) {
	tests := []struct {
		fsxnId       string
		protocolType string
		expected     string
	}{
		{testFsxnID, sa.NFS, testBackendPrefix + testFsxnID + "-nfs"},
		{testFsxnID, sa.ISCSI, testBackendPrefix + testFsxnID + "-iscsi"},
		{"fs-test", "custom", testBackendPrefix + "fs-test-custom"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.fsxnId, tt.protocolType), func(t *testing.T) {
			result := getFSxNBackendName(tt.fsxnId, tt.protocolType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetFSxNStorageClassName(t *testing.T) {
	tests := []struct {
		fsxnId       string
		protocolType string
		expected     string
	}{
		{testFsxnID, sa.NFS, testBackendPrefix + testFsxnID + "-nfs"},
		{testFsxnID, sa.ISCSI, testBackendPrefix + testFsxnID + "-iscsi"},
		{"fs-test", "custom", testBackendPrefix + "fs-test-custom"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.fsxnId, tt.protocolType), func(t *testing.T) {
			result := getFSxNStorageClassName(tt.fsxnId, tt.protocolType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetAWSSecretName(t *testing.T) {
	tests := []struct {
		svmName  string
		expected string
	}{
		{testBackendPrefix + testFsxnID, testBackendPrefix + testFsxnID},
		{testBackendPrefix + testSvmName, testBackendPrefix + testSvmName},
		{"custom-svm", testBackendPrefix + "custom-svm"},
	}

	for _, tt := range tests {
		t.Run(tt.svmName, func(t *testing.T) {
			result := getAWSSecretName(tt.svmName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test cleanUpFSxNRelatedObjects - this method is complex and requires AWS client mocking
// For now, we test it indirectly through its component methods

// Test that the main struct embeds the AWS config correctly
func TestAWS_ConfigEmbedding(t *testing.T) {
	aws := &AWS{
		AwsConfig: AwsConfig{
			StorageDriverName: testStorageDriverName,
			SVMs: []SVM{
				{
					FsxnID:    testFsxnID,
					Protocols: []string{sa.NFS},
					SvmName:   testSvmName,
				},
			},
		},
		TBCNamePrefix:    testFSxConfiguratorName,
		TridentNamespace: testTridentNamespace,
	}

	// Test that embedded fields are accessible
	assert.Equal(t, testStorageDriverName, aws.StorageDriverName)
	assert.Len(t, aws.SVMs, testSingleItemSize)
	assert.Equal(t, testFsxnID, aws.SVMs[0].FsxnID)
	assert.Equal(t, testSvmName, aws.SVMs[0].SvmName)
	assert.Equal(t, testFSxConfiguratorName, aws.TBCNamePrefix)
	assert.Equal(t, testTridentNamespace, aws.TridentNamespace)
}

// Test Validate method - empty SVMs list
func TestAWS_Validate_EmptySVMsList(t *testing.T) {
	os.Setenv(AWSRegion, testAWSRegion)
	defer os.Unsetenv(AWSRegion)

	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		AwsConfig: AwsConfig{
			StorageDriverName: testStorageDriverName,
			SVMs:              []SVM{}, // Empty SVMs list
		},
		ConfClient:       mockClient,
		TBCNamePrefix:    testFSxConfiguratorName,
		TridentNamespace: testTridentNamespace,
	}

	err := aws.Validate()

	// Should not error with empty SVMs list, but let's verify behavior
	assert.NoError(t, err)
}

// Test DeleteBackend method - invalid request type for FSxNID
func TestAWS_DeleteBackend_InvalidFSxNIDType(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		ConfClient:       mockClient,
		TridentNamespace: testTridentNamespace,
	}

	request := map[string]interface{}{
		"FSxNID":    123, // Invalid type (should be string)
		"protocols": []string{sa.NFS},
	}

	// This should panic due to type assertion failure
	assert.Panics(t, func() {
		aws.DeleteBackend(request)
	})
}

// Test DeleteBackend method - invalid request type for protocols
func TestAWS_DeleteBackend_InvalidProtocolsType(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		ConfClient:       mockClient,
		TridentNamespace: testTridentNamespace,
	}

	request := map[string]interface{}{
		"FSxNID":    testFsxnID,
		"protocols": "invalid-type", // Invalid type (should be []string)
	}

	// This should panic due to type assertion failure
	assert.Panics(t, func() {
		aws.DeleteBackend(request)
	})
}

// Test DeleteStorageClass method - invalid request type for FSxNID
func TestAWS_DeleteStorageClass_InvalidFSxNIDType(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		ConfClient:       mockClient,
		TridentNamespace: testTridentNamespace,
	}

	request := map[string]interface{}{
		"FSxNID":    123, // Invalid type (should be string)
		"protocols": []string{sa.NFS},
	}

	// This should panic due to type assertion failure
	assert.Panics(t, func() {
		aws.DeleteStorageClass(request)
	})
}

// Test DeleteStorageClass method - invalid request type for protocols
func TestAWS_DeleteStorageClass_InvalidProtocolsType(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		ConfClient:       mockClient,
		TridentNamespace: testTridentNamespace,
	}

	request := map[string]interface{}{
		"FSxNID":    testFsxnID,
		"protocols": "invalid-type", // Invalid type (should be []string)
	}

	// This should panic due to type assertion failure
	assert.Panics(t, func() {
		aws.DeleteStorageClass(request)
	})
}

// Test DeleteSnapshotClass method - delete error
func TestAWS_DeleteSnapshotClass_DeleteError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		ConfClient: mockClient,
	}

	// Mock empty backend list (should delete snapshot class)
	mockClient.EXPECT().ListObjects(gomock.Any(), "").Return(&tridentV1.TridentBackendList{}, nil)
	mockClient.EXPECT().DeleteObject(gomock.Any(), NetAppSnapshotClassName, "").Return(fmt.Errorf("delete error"))

	err := aws.DeleteSnapshotClass()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error occurred while deleting snapshot class")
}

// Test Create method - empty SVMs list
func TestAWS_Create_EmptySVMsList(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		AwsConfig: AwsConfig{
			StorageDriverName: testStorageDriverName,
			SVMs:              []SVM{}, // Empty SVMs list
		},
		ConfClient:       mockClient,
		TBCNamePrefix:    testFSxConfiguratorName,
		TridentNamespace: testTridentNamespace,
	}

	backends, err := aws.Create()

	assert.NoError(t, err)
	assert.Len(t, backends, testEmptyListSize)
}

// Test CreateStorageClass method - empty SVMs list
func TestAWS_CreateStorageClass_EmptySVMsList(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		AwsConfig: AwsConfig{
			StorageDriverName: testStorageDriverName,
			SVMs:              []SVM{}, // Empty SVMs list
		},
		ConfClient:       mockClient,
		TBCNamePrefix:    testFSxConfiguratorName,
		TridentNamespace: testTridentNamespace,
	}

	err := aws.CreateStorageClass()

	assert.NoError(t, err)
}

// Test Create method - SVM with empty protocols
func TestAWS_Create_SVMEmptyProtocols(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		AwsConfig: AwsConfig{
			StorageDriverName: testStorageDriverName,
			SVMs: []SVM{
				{
					FsxnID:        testFsxnID,
					Protocols:     []string{}, // Empty protocols
					ManagementLIF: testManagementLIF,
					SecretARNName: testSecretARN,
				},
			},
		},
		ConfClient:       mockClient,
		TBCNamePrefix:    testFSxConfiguratorName,
		TridentNamespace: testTridentNamespace,
	}

	backends, err := aws.Create()

	assert.NoError(t, err)
	assert.Len(t, backends, testEmptyListSize)
}

// Test CreateStorageClass method - SVM with empty protocols
func TestAWS_CreateStorageClass_SVMEmptyProtocols(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	aws := &AWS{
		AwsConfig: AwsConfig{
			StorageDriverName: testStorageDriverName,
			SVMs: []SVM{
				{
					FsxnID:    testFsxnID,
					Protocols: []string{}, // Empty protocols
				},
			},
		},
		ConfClient:       mockClient,
		TBCNamePrefix:    testFSxConfiguratorName,
		TridentNamespace: testTridentNamespace,
	}

	err := aws.CreateStorageClass()

	assert.NoError(t, err)
}

// Test getAWSSecretName with edge cases
func TestGetAWSSecretName_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		svmName  string
		expected string
	}{
		{
			name:     "empty string",
			svmName:  "",
			expected: testBackendPrefix,
		},
		{
			name:     "already has trident prefix",
			svmName:  testBackendPrefix + "already-prefixed",
			expected: testBackendPrefix + "already-prefixed",
		},
		{
			name:     "no trident prefix",
			svmName:  "no-prefix",
			expected: testBackendPrefix + "no-prefix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getAWSSecretName(tt.svmName)
			assert.Equal(t, tt.expected, result)
		})
	}
}
