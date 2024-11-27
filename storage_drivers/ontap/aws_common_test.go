package ontap

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/awsapi"
)

const (
	SECRET_MANAGER_ARN = "arn:aws:secretsmanager:eu-west-3:111111111111:secret:secret-name-mlNvrF"
)

func TestFSxFilesystemValidation_Error(t *testing.T) {
	fsxId := FSX_ID
	svmName := "SVM1"
	svm := &awsapi.SVM{
		FSxObject: awsapi.FSxObject{
			Name: svmName,
		},
	}
	mockCtrl := gomock.NewController(t)
	mockAWSAPI := mockapi.NewMockAWSAPI(mockCtrl)
	CommonStorageDriverConfig := &drivers.CommonStorageDriverConfig{}
	CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	CommonStorageDriverConfig.DebugTraceFlags["method"] = true
	CommonStorageDriverConfig.StorageDriverName = "ontap-nas"
	tests := []struct {
		name              string
		fsxId             string
		fileSystemIdError error
		svmError          error
		svm               *[]*awsapi.SVM
		config            *drivers.OntapStorageDriverConfig
		error             string
	}{
		{
			"FSx id is is empty",
			"",
			nil,
			nil,
			nil,
			&drivers.OntapStorageDriverConfig{
				CommonStorageDriverConfig: CommonStorageDriverConfig,
				AWSConfig:                 &drivers.AWSConfig{},
			},
			"filesystem ID in config must be specified",
		},
		{
			"FSx id is api error",
			fsxId, api.ApiError("not found"),
			nil,
			nil,
			&drivers.OntapStorageDriverConfig{
				CommonStorageDriverConfig: CommonStorageDriverConfig,
				AWSConfig: &drivers.AWSConfig{
					FSxFilesystemID: fsxId,
				},
			},
			fmt.Sprintf("filesystem with ID %s not found", fsxId),
		},
		{
			"Get svm error",
			fsxId,
			nil,
			api.ApiError("not found"),
			nil,
			&drivers.OntapStorageDriverConfig{
				CommonStorageDriverConfig: CommonStorageDriverConfig,
				AWSConfig: &drivers.AWSConfig{
					FSxFilesystemID: FSX_ID,
				},
			},
			"could not retrieve FSxN SVMs",
		},
		{
			"SVM does not exist in filesystem",
			fsxId,
			nil,
			nil,
			&[]*awsapi.SVM{
				svm,
			},
			&drivers.OntapStorageDriverConfig{
				CommonStorageDriverConfig: CommonStorageDriverConfig,
				SVM:                       "SVM2",
				AWSConfig: &drivers.AWSConfig{
					FSxFilesystemID: FSX_ID,
				},
			},
			"SVM SVM2 does not exist in filesystem " + fsxId,
		},
		{
			"multiple SVMs exist in filesystem",
			fsxId,
			nil,
			nil,
			&[]*awsapi.SVM{
				svm,
				svm,
			},
			&drivers.OntapStorageDriverConfig{
				CommonStorageDriverConfig: CommonStorageDriverConfig,
				AWSConfig: &drivers.AWSConfig{
					FSxFilesystemID: FSX_ID,
				},
			},
			"no SVM specified and multiple SVMs exist in filesystem " + fsxId,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.fsxId != "" {
				mockAWSAPI.EXPECT().GetFilesystemByID(ctx, test.fsxId).Return(nil, test.fileSystemIdError)
			}

			if test.svm != nil || test.svmError != nil {
				mockAWSAPI.EXPECT().GetSVMs(ctx).Return(test.svm, test.svmError)
			}
			err := validateFSxFilesystem(ctx, mockAWSAPI, test.config)

			assert.Contains(t, err.Error(), test.error)
		})
	}
}

func TestFSxFilesystemValidation_NoError(t *testing.T) {
	fsxId := FSX_ID
	svmName := "SVM1"
	mockCtrl := gomock.NewController(t)
	mockAWSAPI := mockapi.NewMockAWSAPI(mockCtrl)
	CommonStorageDriverConfig := &drivers.CommonStorageDriverConfig{}
	CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	CommonStorageDriverConfig.DebugTraceFlags["method"] = true
	CommonStorageDriverConfig.StorageDriverName = "ontap-nas"
	tests := []struct {
		name   string
		svm    *[]*awsapi.SVM
		config *drivers.OntapStorageDriverConfig
	}{
		{
			"FSX filesystem validation without one IPAddresses and no Svm",
			&[]*awsapi.SVM{
				{
					FSxObject: awsapi.FSxObject{
						Name: svmName,
					},
					MgtEndpoint: &awsapi.Endpoint{
						IPAddresses: []string{"1.1.1.1"},
					},
				},
			},
			&drivers.OntapStorageDriverConfig{
				CommonStorageDriverConfig: CommonStorageDriverConfig,
				AWSConfig: &drivers.AWSConfig{
					FSxFilesystemID: fsxId,
				},
			},
		},
		{
			"FSX filesystem validation with ManagementLIF and Svm",
			&[]*awsapi.SVM{
				{
					FSxObject: awsapi.FSxObject{
						Name: svmName,
					},
				},
			},
			&drivers.OntapStorageDriverConfig{
				CommonStorageDriverConfig: CommonStorageDriverConfig,
				SVM:                       svmName,
				ManagementLIF:             "1.1.1.1",
				AWSConfig: &drivers.AWSConfig{
					FSxFilesystemID: fsxId,
				},
			},
		},
		{
			"FSX filesystem validation with DNSName and no ManagementLIF ",
			&[]*awsapi.SVM{
				{
					FSxObject: awsapi.FSxObject{
						Name: svmName,
					},
					MgtEndpoint: &awsapi.Endpoint{
						IPAddresses: []string{},
						DNSName:     "dns",
					},
				},
			},
			&drivers.OntapStorageDriverConfig{
				CommonStorageDriverConfig: CommonStorageDriverConfig,
				SVM:                       svmName,
				AWSConfig: &drivers.AWSConfig{
					FSxFilesystemID: fsxId,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockAWSAPI.EXPECT().GetFilesystemByID(ctx, fsxId).Return(nil, nil)

			if test.svm != nil {
				mockAWSAPI.EXPECT().GetSVMs(ctx).Return(test.svm, nil)
			}
			err := validateFSxFilesystem(ctx, mockAWSAPI, test.config)

			assert.NoError(t, err, nil)
		})
	}
}

func TestInitializeAWSDriver(t *testing.T) {
	fsxId := ""
	secretArn := SECRET_MANAGER_ARN
	config := &drivers.OntapStorageDriverConfig{}
	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true
	config.StorageDriverName = "ontap-nas"
	config.ManagementLIF = "1.1.1.1"
	config.AWSConfig = &drivers.AWSConfig{}
	config.AWSConfig.FSxFilesystemID = fsxId
	tests := []struct {
		name       string
		secretName string
		secretType string
		userName   string
		error      string
	}{
		{"Invalid secret ARN value", "secret-manager-arn-value", "awsarn", "", "secret ARN secret-manager-arn-value is invalid"},
		{"Invalid secret ARN value - use username and password", "", "awsarn", "arn:aws:secretsmanager:region", "secret ARN arn:aws:secretsmanager:region is invalid"},
		{"Invalid awsarn secret", secretArn, "awsarn", "", "could not retrieve credentials from AWS Secrets Manager"},
		{"valid aws secret", "", "secret", "", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config.Username = test.userName
			if test.secretName == "" {
				config.Credentials = map[string]string{}
			} else {
				config.Credentials = map[string]string{
					"type": test.secretType,
					"name": test.secretName,
				}
			}

			_, err := initializeAWSDriver(ctx, config)

			if test.error == "" {
				assert.NoError(t, err, nil)
			} else {
				assert.Contains(t, err.Error(), test.error)
			}
		})
	}
}

func TestSvmCredentials(t *testing.T) {
	secretArn := "secret-arn"
	mockCtrl := gomock.NewController(t)
	mockAWSAPI := mockapi.NewMockAWSAPI(mockCtrl)
	config := &drivers.OntapStorageDriverConfig{}
	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true
	config.StorageDriverName = "ontap-nas"
	tests := []struct {
		name   string
		secret *awsapi.Secret
		error  string
	}{
		{"Both username and password key is missing", &awsapi.Secret{
			SecretMap: map[string]string{},
		}, "ontap-nas driver must include username in the secret referenced by Credentials"},
		{"The password key is missing", &awsapi.Secret{
			SecretMap: map[string]string{"username": "username"},
		}, "ontap-nas driver must include password in the secret referenced by Credentials"},
		{"Both username and password key is present", &awsapi.Secret{
			SecretMap: map[string]string{"username": "username", "password": "password"},
		}, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockAWSAPI.EXPECT().GetSecret(ctx, secretArn).Return(test.secret, nil)
			err := SetSvmCredentials(ctx, secretArn, mockAWSAPI, config)
			if test.error == "" {
				assert.NoError(t, err, nil)
			} else {
				assert.Contains(t, err.Error(), test.error)
			}
		})
	}
}
