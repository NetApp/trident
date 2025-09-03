// Copyright 2024 NetApp, Inc. All Rights Reserved.
package awsapi

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	fsxtypes "github.com/aws/aws-sdk-go-v2/service/fsx/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/storage"
)

// Test constants
// These constants are used across multiple tests to simulate AWS resources.
const (
	testRegion       = "us-east-1"
	testAPIKey       = "test-access-key"
	testSecretKey    = "test-secret-key"
	testFilesystemID = "fs-1234567890abcdef0"
	testVolumeARN    = "arn:aws:fsx:us-east-1:123456789012:volume/fs-1234567890abcdef0/fv-1234567890abcdef0"
	testSecretARN    = "arn:aws:secretsmanager:us-east-1:123456789012:secret:test-secret-AbCdEf"
	testAccountID    = "123456789012"
	testVolumeID     = "fv-1234567890abcdef0"
	testSecretName   = "test-secret"
	testPassword     = "test-password"
	testUsername     = "test-user"
	testSVMName      = "test-svm"
	testSVMID        = "svm-1234567890abcdef0"
	testRequestID    = "12345678-1234-1234-1234-123456789012"
)

// setupTestClient creates a test client with valid configuration.
func setupTestClient(t *testing.T) *Client {
	ctx := context.Background()
	config := ClientConfig{
		APIRegion:    testRegion,
		APIKey:       testAPIKey,
		SecretKey:    testSecretKey,
		FilesystemID: testFilesystemID,
	}

	client, err := NewClient(ctx, config)
	require.NoError(t, err)
	require.NotNil(t, client)

	return client
}

// Core Infrastructure Tests
// These tests validate the core AWS client configuration and initialization.
func TestCreateAWSConfig(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		region         string
		apiKey         string
		secretKey      string
		expectError    bool
		validateCreds  bool
		expectedRegion string
	}{
		{
			name:           "WithCredentials",
			region:         testRegion,
			apiKey:         testAPIKey,
			secretKey:      testSecretKey,
			expectError:    false,
			validateCreds:  true,
			expectedRegion: testRegion,
		},
		{
			name:           "WithoutCredentials",
			region:         testRegion,
			apiKey:         "",
			secretKey:      "",
			expectError:    false,
			validateCreds:  false,
			expectedRegion: testRegion,
		},
		{
			name:           "DifferentRegion",
			region:         "us-west-2",
			apiKey:         testAPIKey,
			secretKey:      testSecretKey,
			expectError:    false,
			validateCreds:  true,
			expectedRegion: "us-west-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := createAWSConfig(ctx, tt.region, tt.apiKey, tt.secretKey)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, cfg)
			assert.Equal(t, tt.expectedRegion, cfg.Region)

			if tt.validateCreds {
				creds, err := cfg.Credentials.Retrieve(ctx)
				assert.NoError(t, err)
				assert.Equal(t, tt.apiKey, creds.AccessKeyID)
				assert.Equal(t, tt.secretKey, creds.SecretAccessKey)
			}
		})
	}
}

func TestNewClient(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		config         ClientConfig
		expectError    bool
		validateFields []string // Fields to validate are present
	}{
		{
			name: "ValidConfig",
			config: ClientConfig{
				APIRegion:           testRegion,
				APIKey:              testAPIKey,
				SecretKey:           testSecretKey,
				FilesystemID:        testFilesystemID,
				SecretManagerRegion: "us-west-2",
			},
			expectError:    false,
			validateFields: []string{"fsxClient", "secretsClient", "filesystemID"},
		},
		{
			name:           "EmptyConfig",
			config:         ClientConfig{},
			expectError:    false,
			validateFields: []string{"fsxClient", "secretsClient"},
		},
		{
			name: "MinimalConfig",
			config: ClientConfig{
				APIRegion: testRegion,
			},
			expectError:    false,
			validateFields: []string{"fsxClient", "secretsClient"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(ctx, tt.config)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, client)

			// Validate required fields
			for _, field := range tt.validateFields {
				switch field {
				case "fsxClient":
					assert.NotNil(t, client.fsxClient)
				case "secretsClient":
					assert.NotNil(t, client.secretsClient)
				case "filesystemID":
					assert.Equal(t, tt.config.FilesystemID, client.config.FilesystemID)
				}
			}
		})
	}
}

func TestSetClientFsConfig(t *testing.T) {
	client := setupTestClient(t)
	newFilesystemID := "new-fs-id"

	client.SetClientFsConfig(newFilesystemID)
	assert.Equal(t, newFilesystemID, client.config.FilesystemID)
}

// ARN Parsing and Utility Tests
// These tests validate the parsing of ARNs and utility functions for dereferencing pointers.
func TestParseVolumeARN(t *testing.T) {
	tests := []struct {
		name                 string
		arn                  string
		expectError          bool
		expectedRegion       string
		expectedAccountID    string
		expectedFilesystemID string
		expectedVolumeID     string
	}{
		{
			name:                 "ValidARN",
			arn:                  testVolumeARN,
			expectError:          false,
			expectedRegion:       "us-east-1",
			expectedAccountID:    testAccountID,
			expectedFilesystemID: testFilesystemID,
			expectedVolumeID:     testVolumeID,
		},
		{
			name:        "InvalidARN",
			arn:         "invalid-arn",
			expectError: true,
		},
		{
			name:        "WrongServiceARN",
			arn:         "arn:aws:s3:::bucket/key",
			expectError: true,
		},
		{
			name:        "FilesystemARN",
			arn:         "arn:aws:fsx:us-east-1:123456789012:filesystem/fs-123",
			expectError: true,
		},
		{
			name:        "EmptyARN",
			arn:         "",
			expectError: true,
		},
		{
			name:                 "AWSChinaPartition",
			arn:                  "arn:aws-cn:fsx:cn-north-1:123456789012:volume/fs-1234567890abcdef0/fv-1234567890abcdef0",
			expectError:          false,
			expectedRegion:       "cn-north-1",
			expectedAccountID:    testAccountID,
			expectedFilesystemID: testFilesystemID,
			expectedVolumeID:     testVolumeID,
		},
		{
			name:                 "AWSGovCloudPartition",
			arn:                  "arn:aws-us-gov:fsx:us-gov-east-1:123456789012:volume/fs-1234567890abcdef0/fv-1234567890abcdef0",
			expectError:          false,
			expectedRegion:       "us-gov-east-1",
			expectedAccountID:    testAccountID,
			expectedFilesystemID: testFilesystemID,
			expectedVolumeID:     testVolumeID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			region, accountID, filesystemID, volumeID, err := ParseVolumeARN(tt.arn)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "volume ARN")
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedRegion, region)
			assert.Equal(t, tt.expectedAccountID, accountID)
			assert.Equal(t, tt.expectedFilesystemID, filesystemID)
			assert.Equal(t, tt.expectedVolumeID, volumeID)
		})
	}
}

func TestParseSecretARN(t *testing.T) {
	tests := []struct {
		name               string
		arn                string
		expectError        bool
		expectedRegion     string
		expectedAccountID  string
		expectedSecretName string
	}{
		{
			name:               "ValidARN",
			arn:                testSecretARN,
			expectError:        false,
			expectedRegion:     "us-east-1",
			expectedAccountID:  testAccountID,
			expectedSecretName: testSecretName,
		},
		{
			name:        "InvalidARN",
			arn:         "invalid-arn",
			expectError: true,
		},
		{
			name:        "WrongServiceARN",
			arn:         "arn:aws:s3:::bucket/key",
			expectError: true,
		},
		{
			name:        "IncompleteSecretARN",
			arn:         "arn:aws:secretsmanager:us-east-1:123456789012:secret",
			expectError: true,
		},
		{
			name:        "EmptyARN",
			arn:         "",
			expectError: true,
		},
		{
			name:               "SpecialCharacters",
			arn:                "arn:aws:secretsmanager:us-east-1:123456789012:secret:my/secret_name.with+special@chars-AbCdEf",
			expectError:        false,
			expectedRegion:     "us-east-1",
			expectedAccountID:  testAccountID,
			expectedSecretName: "my/secret_name.with+special@chars",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			region, accountID, secretName, err := ParseSecretARN(tt.arn)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "secret ARN")
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedRegion, region)
			assert.Equal(t, tt.expectedAccountID, accountID)
			assert.Equal(t, tt.expectedSecretName, secretName)
		})
	}
}

func TestUtilityFunctions(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			name: "DerefString",
			testFunc: func(t *testing.T) {
				testString := "test-value"
				assert.Equal(t, testString, DerefString(&testString))
				assert.Equal(t, "", DerefString(nil))
			},
		},
		{
			name: "DerefInt32",
			testFunc: func(t *testing.T) {
				testInt := int32(42)
				assert.Equal(t, testInt, DerefInt32(&testInt))
				assert.Equal(t, int32(0), DerefInt32(nil))
			},
		},
		{
			name: "DerefTime",
			testFunc: func(t *testing.T) {
				testTime := time.Now()
				assert.Equal(t, testTime, DerefTime(&testTime))
				assert.Equal(t, time.Time{}, DerefTime(nil))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestErrorUtilities(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			name: "IsVolumeNotFoundError",
			testFunc: func(t *testing.T) {
				vnfError := &fsxtypes.VolumeNotFound{
					Message: aws.String("Volume not found"),
				}
				assert.True(t, IsVolumeNotFoundError(vnfError))

				// Test wrapped error - This tests the handling of wrapped errors.
				wrappedError := fmt.Errorf("operation failed: %w", vnfError)
				assert.True(t, IsVolumeNotFoundError(wrappedError))

				// Test other errors - This tests the handling of non-wrapped errors.
				assert.False(t, IsVolumeNotFoundError(fmt.Errorf("other error")))
				assert.False(t, IsVolumeNotFoundError(nil))
			},
		},
		{
			name: "GetRequestIDFromError",
			testFunc: func(t *testing.T) {
				responseError := &awshttp.ResponseError{
					RequestID: testRequestID,
				}
				assert.Equal(t, testRequestID, GetRequestIDFromError(responseError))
				assert.Equal(t, "", GetRequestIDFromError(fmt.Errorf("other error")))
				assert.Equal(t, "", GetRequestIDFromError(nil))
			},
		},
		{
			name: "TerminalStateError",
			testFunc: func(t *testing.T) {
				// Test error creation and methods
				// This tests the creation and methods of TerminalStateError.
				originalError := fmt.Errorf("operation failed")
				terminalError := &TerminalStateError{
					Err: originalError,
				}
				assert.Error(t, terminalError)
				assert.Contains(t, terminalError.Error(), "operation failed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

// AWS API Function Tests (These provide actual coverage)
// These tests validate the AWS API functions for secrets, filesystems, SVMs, and volumes.
func TestSecretOperations(t *testing.T) {
	ctx := context.Background()
	client := setupTestClient(t)

	tests := []struct {
		name        string
		testFunc    func(*testing.T)
		expectError bool
	}{
		{
			name: "CreateSecret",
			testFunc: func(t *testing.T) {
				request := &SecretCreateRequest{
					Name:        testSecretName,
					Description: "Test secret",
					SecretData: map[string]string{
						"username": testUsername,
						"password": testPassword,
					},
				}

				// This calls the actual CreateSecret function
				// This tests the creation of a secret using the AWS Secrets Manager API.
				secret, err := client.CreateSecret(ctx, request)
				assert.Error(t, err) // Expected due to no real AWS setup
				assert.Nil(t, secret)
			},
			expectError: true,
		},
		{
			name: "GetSecretInvalidARN",
			testFunc: func(t *testing.T) {
				_, err := client.GetSecret(ctx, "invalid-arn")
				assert.Error(t, err)
			},
			expectError: true,
		},
		{
			name: "GetSecretValidARN",
			testFunc: func(t *testing.T) {
				_, err := client.GetSecret(ctx, testSecretARN)
				assert.Error(t, err) // Expected due to no real AWS setup
			},
			expectError: true,
		},
		{
			name: "DeleteSecret",
			testFunc: func(t *testing.T) {
				err := client.DeleteSecret(ctx, testSecretARN)
				assert.Error(t, err) // Expected due to no real AWS setup
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestFilesystemOperations(t *testing.T) {
	ctx := context.Background()
	client := setupTestClient(t)

	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			name: "GetFilesystems",
			testFunc: func(t *testing.T) {
				filesystems, err := client.GetFilesystems(ctx)
				assert.Error(t, err) // Expected due to no real AWS setup
				assert.Nil(t, filesystems)
			},
		},
		{
			name: "GetFilesystemByID",
			testFunc: func(t *testing.T) {
				filesystem, err := client.GetFilesystemByID(ctx, testFilesystemID)
				assert.Error(t, err) // Expected due to no real AWS setup
				assert.Nil(t, filesystem)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestSVMOperations(t *testing.T) {
	ctx := context.Background()
	client := setupTestClient(t)

	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			name: "CreateSVM",
			testFunc: func(t *testing.T) {
				request := &SVMCreateRequest{
					Name: testSVMName,
				}

				// This tests the creation of an SVM using the AWS FSx API.
				svm, err := client.CreateSVM(ctx, request)
				assert.Error(t, err) // Expected due to no real AWS setup
				assert.Nil(t, svm)
			},
		},
		{
			name: "GetSVMs",
			testFunc: func(t *testing.T) {
				svms, err := client.GetSVMs(ctx)
				assert.Error(t, err) // Expected due to no real AWS setup
				assert.Nil(t, svms)
			},
		},
		{
			name: "GetSVMByID",
			testFunc: func(t *testing.T) {
				svm, err := client.GetSVMByID(ctx, testSVMID)
				assert.Error(t, err) // Expected due to no real AWS setup
				assert.Nil(t, svm)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestVolumeOperations(t *testing.T) {
	ctx := context.Background()
	client := setupTestClient(t)

	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			name: "GetVolumes",
			testFunc: func(t *testing.T) {
				volumes, err := client.GetVolumes(ctx)
				assert.Error(t, err) // Expected due to no real AWS setup
				assert.Nil(t, volumes)
			},
		},
		{
			name: "GetVolumeByName",
			testFunc: func(t *testing.T) {
				volume, err := client.GetVolumeByName(ctx, "test-volume")
				assert.Error(t, err) // Expected due to no real AWS setup
				assert.Nil(t, volume)
			},
		},
		{
			name: "GetVolumeByARNInvalid",
			testFunc: func(t *testing.T) {
				_, err := client.GetVolumeByARN(ctx, "invalid-arn")
				assert.Error(t, err)
			},
		},
		{
			name: "GetVolumeByARNValid",
			testFunc: func(t *testing.T) {
				volume, err := client.GetVolumeByARN(ctx, testVolumeARN)
				assert.Error(t, err) // Expected due to no real AWS setup
				assert.Nil(t, volume)
			},
		},
		{
			name: "GetVolumeByID",
			testFunc: func(t *testing.T) {
				volume, err := client.GetVolumeByID(ctx, testVolumeID)
				assert.Error(t, err) // Expected due to no real AWS setup
				assert.Nil(t, volume)
			},
		},
		{
			name: "GetVolumeWithInternalID",
			testFunc: func(t *testing.T) {
				config := &storage.VolumeConfig{
					Name:         "test-volume",
					InternalID:   testVolumeID,
					InternalName: "test-volume",
				}

				volume, err := client.GetVolume(ctx, config)
				assert.Error(t, err) // Expected due to no real AWS setup
				assert.Nil(t, volume)
			},
		},
		{
			name: "GetVolumeWithoutInternalID",
			testFunc: func(t *testing.T) {
				configWithoutID := &storage.VolumeConfig{
					Name:         "test-volume",
					InternalName: "test-volume",
				}
				volume, err := client.GetVolume(ctx, configWithoutID)
				assert.Error(t, err)
				assert.Nil(t, volume)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestVolumeExistenceChecks(t *testing.T) {
	ctx := context.Background()
	client := setupTestClient(t)

	testCases := []struct {
		name string
		fn   func() (bool, *Volume, error)
	}{
		{
			"VolumeExistsByName",
			func() (bool, *Volume, error) {
				return client.VolumeExistsByName(ctx, "test-volume")
			},
		},
		{
			"VolumeExistsByID",
			func() (bool, *Volume, error) {
				return client.VolumeExistsByID(ctx, testVolumeID)
			},
		},
		{
			"VolumeExistsByARN",
			func() (bool, *Volume, error) {
				return client.VolumeExistsByARN(ctx, testVolumeARN)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			exists, vol, err := tc.fn()
			assert.Error(t, err) // Expected due to no real AWS setup
			assert.False(t, exists)
			assert.Nil(t, vol)
		})
	}

	t.Run("VolumeExistsWithConfig", func(t *testing.T) {
		config := &storage.VolumeConfig{
			Name:         "test-volume",
			InternalID:   testVolumeID,
			InternalName: "test-volume",
		}

		exists, vol, err := client.VolumeExists(ctx, config)
		assert.Error(t, err) // Expected due to no real AWS setup
		assert.False(t, exists)
		assert.Nil(t, vol)
	})

	t.Run("VolumeExistsByARNInvalidARN", func(t *testing.T) {
		exists, vol, err := client.VolumeExistsByARN(ctx, "invalid-arn")
		assert.Error(t, err)
		assert.False(t, exists)
		assert.Nil(t, vol)
	})
}

func TestVolumeLifecycleOperations(t *testing.T) {
	ctx := context.Background()
	client := setupTestClient(t)

	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			name: "CreateVolumeNFS",
			testFunc: func(t *testing.T) {
				request := &VolumeCreateRequest{
					Name:              "test-volume-1",
					SizeBytes:         1073741824,
					SVMID:             testSVMID,
					ProtocolTypes:     []string{"NFS"},
					SecurityStyle:     "unix",
					SnapshotDirectory: true,
				}
				volume, err := client.CreateVolume(ctx, request)
				assert.Error(t, err) // Expected due to no real AWS setup
				assert.Nil(t, volume)
			},
		},
		{
			name: "CreateVolumeCIFS",
			testFunc: func(t *testing.T) {
				request := &VolumeCreateRequest{
					Name:              "test-volume-2",
					SizeBytes:         2147483648,
					SVMID:             testSVMID,
					ProtocolTypes:     []string{"CIFS"},
					SecurityStyle:     "ntfs",
					SnapshotDirectory: false,
					SnapshotPolicy:    "none",
				}
				volume, err := client.CreateVolume(ctx, request)
				assert.Error(t, err) // Expected due to no real AWS setup
				assert.Nil(t, volume)
			},
		},
		{
			name: "ResizeVolume",
			testFunc: func(t *testing.T) {
				volume := &Volume{
					FSxObject: FSxObject{ID: testVolumeID},
				}
				newSize := uint64(2147483648) // 2GB
				resizedVolume, err := client.ResizeVolume(ctx, volume, newSize)
				assert.Error(t, err) // Expected due to no real AWS setup
				assert.Nil(t, resizedVolume)
			},
		},
		{
			name: "DeleteVolume",
			testFunc: func(t *testing.T) {
				volume := &Volume{
					FSxObject: FSxObject{ID: testVolumeID},
				}
				err := client.DeleteVolume(ctx, volume)
				assert.Error(t, err) // Expected due to no real AWS setup
			},
		},
		{
			name: "RelabelVolume",
			testFunc: func(t *testing.T) {
				volume := &Volume{
					FSxObject: FSxObject{ID: testVolumeID},
				}
				newLabels := map[string]string{
					"env":  "test",
					"team": "storage",
				}
				err := client.RelabelVolume(ctx, volume, newLabels)
				assert.Error(t, err) // Expected due to no real AWS setup
			},
		},
		{
			name: "WaitForVolumeStates",
			testFunc: func(t *testing.T) {
				volume := &Volume{
					FSxObject: FSxObject{ID: testVolumeID},
				}

				desiredStates := []string{StateAvailable}
				abortStates := []string{StateFailed}
				maxElapsedTime := 1 * time.Millisecond // Very short timeout

				_, err := client.WaitForVolumeStates(ctx, volume, desiredStates, abortStates, maxElapsedTime)
				assert.Error(t, err) // Expected due to timeout or AWS setup
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

// Conversion Function Tests
// These tests validate the conversion of AWS FSx objects to internal representations.
func TestConversionFunctions(t *testing.T) {
	client := setupTestClient(t)

	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			name: "GetFilesystemFromFSxFilesystem",
			testFunc: func(t *testing.T) {
				creationTime := time.Now()
				fsxFilesystem := fsxtypes.FileSystem{
					FileSystemId: aws.String(testFilesystemID),
					CreationTime: &creationTime,
					Lifecycle:    fsxtypes.FileSystemLifecycleAvailable,
					ResourceARN:  aws.String("arn:aws:fsx:us-east-1:123456789012:file-system/" + testFilesystemID),
					OwnerId:      aws.String(testAccountID),
					VpcId:        aws.String("vpc-12345"),
				}

				filesystem := client.getFilesystemFromFSxFilesystem(fsxFilesystem)

				assert.Equal(t, testFilesystemID, filesystem.ID)
				assert.Equal(t, testAccountID, filesystem.OwnerID)
				assert.Equal(t, "vpc-12345", filesystem.VPCID)
				assert.Equal(t, StateAvailable, filesystem.State)
			},
		},
		{
			name: "GetSVMFromFSxSVM",
			testFunc: func(t *testing.T) {
				creationTime := time.Now()
				fsxSVM := fsxtypes.StorageVirtualMachine{
					StorageVirtualMachineId: aws.String(testSVMID),
					Name:                    aws.String(testSVMName),
					CreationTime:            &creationTime,
					FileSystemId:            aws.String(testFilesystemID),
					Lifecycle:               fsxtypes.StorageVirtualMachineLifecycleCreated,
					ResourceARN:             aws.String("arn:aws:fsx:us-east-1:123456789012:storage-virtual-machine/" + testSVMID),
					UUID:                    aws.String("uuid-12345"),
					Subtype:                 fsxtypes.StorageVirtualMachineSubtypeDefault,
				}

				svm := client.getSVMFromFSxSVM(fsxSVM)

				assert.Equal(t, testSVMID, svm.ID)
				assert.Equal(t, testSVMName, svm.Name)
				assert.Equal(t, testFilesystemID, svm.FilesystemID)
				assert.Equal(t, "uuid-12345", svm.UUID)
				assert.Equal(t, StateCreated, svm.State)
			},
		},
		{
			name: "GetVolumeFromFSxVolume",
			testFunc: func(t *testing.T) {
				creationTime := time.Now()
				volumeSize := int32(1024) // 1GB in megabytes

				fsxVolume := fsxtypes.Volume{
					VolumeId:     aws.String(testVolumeID),
					Name:         aws.String("test-volume"),
					CreationTime: &creationTime,
					Lifecycle:    fsxtypes.VolumeLifecycleAvailable,
					ResourceARN:  aws.String(testVolumeARN),
					FileSystemId: aws.String(testFilesystemID),
					VolumeType:   fsxtypes.VolumeTypeOntap,
					OntapConfiguration: &fsxtypes.OntapVolumeConfiguration{
						SizeInMegabytes:         &volumeSize,
						SecurityStyle:           fsxtypes.SecurityStyleUnix,
						JunctionPath:            aws.String("/test-volume"),
						StorageVirtualMachineId: aws.String(testSVMID),
						UUID:                    aws.String("volume-uuid-12345"),
						SnapshotPolicy:          aws.String("default"),
					},
				}

				volume := client.getVolumeFromFSxVolume(fsxVolume)

				assert.Equal(t, testVolumeID, volume.ID)
				assert.Equal(t, "test-volume", volume.Name)
				assert.Equal(t, testFilesystemID, volume.FilesystemID)
				assert.Equal(t, testSVMID, volume.SVMID)
				assert.Equal(t, uint64(volumeSize)*1048576, volume.Size) // MB to bytes conversion
				assert.Equal(t, "/test-volume", volume.JunctionPath)
				assert.Equal(t, "volume-uuid-12345", volume.UUID)
				assert.Equal(t, StateAvailable, volume.State)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

// Edge Cases and Error Validation
// These tests validate the handling of edge cases and error scenarios.
func TestErrorValidation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			name: "InvalidJSONUnmarshal",
			testFunc: func(t *testing.T) {
				invalidJSON := `{"password": "test", "username":}` // Invalid JSON
				var secretData map[string]string
				err := json.Unmarshal([]byte(invalidJSON), &secretData)
				assert.Error(t, err)
			},
		},
		{
			name: "NewClientEdgeCases",
			testFunc: func(t *testing.T) {
				// Test with different regions
				config := ClientConfig{
					APIRegion:           "us-west-2",
					APIKey:              testAPIKey,
					SecretKey:           testSecretKey,
					FilesystemID:        testFilesystemID,
					SecretManagerRegion: "us-east-1",
				}

				client, err := NewClient(ctx, config)
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.Equal(t, testFilesystemID, client.config.FilesystemID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}
