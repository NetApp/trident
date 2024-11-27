// Copyright 2024 NetApp, Inc. All Rights Reserved.

package storage_drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	confClients "github.com/netapp/trident/operator/controllers/configurator/clients"
	operatorV1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/storage_drivers/ontap/awsapi"
	"github.com/netapp/trident/utils"
)

const (
	SvmStateCreated = "CREATED"
	AWSRegion       = "AWS_REGION"

	TridentSecretPattern    = "trident-%s"
	SvmNamePattern          = "trident-%s"
	StorageClassNamePattern = "trident-%s-%s"
	BackendNamePattern      = "trident-%s-%s"
	VsAdmin                 = "vsadmin"
	Description             = "Trident secret for FsxN for ONTAP"

	// Tags for the secret
	FileSystemId = "file-system-id"
)

type AWS struct {
	AwsConfig
	ConfClient       confClients.ConfiguratorClientInterface
	AwsClient        *awsapi.Client
	ManagementLif    string
	TBCNamePrefix    string
	TridentNamespace string
}

type AwsConfig struct {
	StorageDriverName string `json:"storageDriverName"`
	SVMs              []SVM  `json:"svms"`
}

type SVM struct {
	FsxnID        string   `json:"fsxnID"`
	Protocols     []string `json:"protocols"`
	AuthType      string   `json:"authType"`
	SvmName       string   `json:"svmName"`
	SecretARNName string   `json:"secretARNName"`
	ManagementLIF string   `json:"managementLIF"`
}

// NewFsxnInstance creates a new instance of the AWS struct and populates it with the provided CRs and client
func NewFsxnInstance(
	torcCR *operatorV1.TridentOrchestrator, configuratorCR *operatorV1.TridentConfigurator,
	client confClients.ConfiguratorClientInterface,
) (*AWS, error) {
	if torcCR == nil {
		return nil, fmt.Errorf("empty torc CR")
	}

	if configuratorCR == nil {
		return nil, fmt.Errorf("empty AWSFsxN configurator CR")
	}

	if client == nil {
		return nil, fmt.Errorf("invalid client")
	}

	awsConfig := AwsConfig{}
	if err := json.Unmarshal(configuratorCR.Spec.Raw, &awsConfig); err != nil {
		Log().Errorf("Error occured while unmarshalling configurator CR: %v", err)
		return nil, err
	}
	return &AWS{
		AwsConfig:        awsConfig,
		ConfClient:       client,
		TBCNamePrefix:    configuratorCR.Name,
		TridentNamespace: torcCR.Spec.Namespace,
	}, nil
}

// Validate validates the AWS configuration and prepares each FSxN instance for the auto-backend configuration.
func (aws *AWS) Validate() error {
	var (
		awsRegion string
		apiKey    string
		secretKey string
	)
	awsRegion = os.Getenv(AWSRegion)
	if awsRegion == "" {
		return fmt.Errorf("%s is not set", AWSRegion)
	}
	// If key and secret are set, then add to the config
	apiKey = os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")

	api, err := awsapi.NewClient(context.Background(), awsapi.ClientConfig{
		APIRegion: awsRegion,
		APIKey:    apiKey,
		SecretKey: secretKey,
	})
	if err != nil {
		return fmt.Errorf("error occurred while creating AWS client: %w", err)
	}

	aws.AwsClient = api

	for key, fsxnInstance := range aws.SVMs {
		if err := aws.processFsxnInstance(context.Background(), key, fsxnInstance); err != nil {
			return err
		}
	}

	return nil
}

// processFsxnInstance processes the auto-backend configuration for the FSxN instance.
// It creates the SVM if it does not exist and creates the secret for the SVM.
func (aws *AWS) processFsxnInstance(ctx context.Context, key int, svm SVM) error {
	var (
		svmName    string
		secretName string
		secretARN  string
		svmExists  bool
	)
	svmName = svm.SvmName
	if svmName == "" {
		svmName = fmt.Sprintf(SvmNamePattern, svm.FsxnID)
	}
	_, err := aws.AwsClient.GetFilesystemByID(ctx, svm.FsxnID)
	if err != nil {
		return fmt.Errorf("error occurred while getting fsxn id: %v : %v", svm.FsxnID, err)
	}
	Log().Debugf("Filesystem ID: %s exists", svm.FsxnID)
	secretName = fmt.Sprintf(TridentSecretPattern, strings.TrimPrefix(svmName, "trident-"))
	// Get the secret if it already exists or create a new one
	secret, _ := aws.AwsClient.GetSecret(ctx, secretName)
	if secret != nil {
		Log().Debugf("Secret %s already exists, reusing the same for auto-backend config.", secretName)
		secretARN = secret.SecretARN
	} else {
		Log().Debugf("Creating secret %s for auto-backend config.", secretName)
		resSecret, err := aws.AwsClient.CreateSecret(ctx, &awsapi.SecretCreateRequest{
			Name:        secretName,
			Description: Description,
			SecretData: map[string]string{
				"username": VsAdmin,
				"password": utils.GenerateRandomPassword(ctx, 10, true, true, true, true),
			},
			Tags: map[string]string{
				FileSystemId: svm.FsxnID,
			},
		})
		if err != nil {
			return fmt.Errorf("error occurred while creating secret: %w ", err)
		}
		secretARN = resSecret.SecretARN
	}

	aws.SVMs[key].SecretARNName = secretARN
	aws.AwsClient.SetClientFsConfig(svm.FsxnID)

	svmList, err := aws.AwsClient.GetSVMs(ctx)
	if err != nil {
		return fmt.Errorf("error occurred while getting SVMs: %w", err)
	}
	for _, storageVirtualMachine := range *svmList {
		if storageVirtualMachine.Name == svmName {
			// SVM already exists against the filesystem. Reuse the same for auto-backend config.
			Log().Debugf("SVM already exists: %v for fsxnId: %v", storageVirtualMachine.Name,
				storageVirtualMachine.FilesystemID)
			aws.SVMs[key].SvmName = storageVirtualMachine.Name
			aws.SVMs[key].ManagementLIF = storageVirtualMachine.MgtEndpoint.IPAddresses[0]
			svmExists = true
			break
		}
	}
	if !svmExists {
		// Create SVM if it does not exist against the filesystem
		svmCreateRequest := &awsapi.SVMCreateRequest{
			Name:      svmName,
			SecretARN: secretARN,
		}

		_, err = aws.AwsClient.CreateSVM(ctx, svmCreateRequest)
		if err != nil {
			return fmt.Errorf("error occurred while creating SVM: %w", err)
		}
		svmStatus := func() error {
			svm, err := aws.AwsClient.GetSVMs(ctx)
			if err != nil {
				return fmt.Errorf("error occurred while getting SVM: %w", err)
			}

			for _, svm := range *svm {
				if svm.Name == svmCreateRequest.Name && svm.State == SvmStateCreated {
					// SVM is in running state now and ready for auto-backend config
					Log().Infof("SVM %v is in running state", svm.Name)
					aws.SVMs[key].SvmName = svm.Name
					aws.SVMs[key].ManagementLIF = svm.MgtEndpoint.IPAddresses[0]
					return nil
				}
			}

			return fmt.Errorf("SVM is still not in running state")
		}
		checkSvmCreatedNotify := func(err error, duration time.Duration) {
			Log().WithFields(LogFields{
				"svmName": svmName,
				"err":     err,
			}).Debug("Svm not yet created, waiting.")
		}
		checkSVMStatusBackoff := backoff.NewExponentialBackOff()
		checkSVMStatusBackoff.InitialInterval = 1 * time.Second
		checkSVMStatusBackoff.RandomizationFactor = 0.1
		checkSVMStatusBackoff.Multiplier = 1.414
		checkSVMStatusBackoff.MaxInterval = 10 * time.Second
		checkSVMStatusBackoff.MaxElapsedTime = 5 * time.Minute

		// Retry to check the SVM status until it is in running state
		if err := backoff.RetryNotify(svmStatus, checkSVMStatusBackoff, checkSvmCreatedNotify); err != nil {
			Log().Errorf("Svm not in running state after %3.2f minutes", checkSVMStatusBackoff.MaxElapsedTime.Minutes())
			return err
		}
	}
	return nil
}

// Create creates the Trident backend against the storage drivers
func (aws *AWS) Create() ([]string, error) {
	var (
		backendList []string
		backendName string
	)
	for _, svm := range aws.SVMs {
		for _, protocol := range svm.Protocols {
			backendName = getFsxnBackendName(svm.FsxnID, protocol)
			backendYAML := getFsxnTBCYaml(svm, aws.TridentNamespace, backendName, protocol)
			if err := aws.ConfClient.CreateOrPatchObject(confClients.OBackend, backendName,
				aws.TridentNamespace, backendYAML); err != nil {
				return nil, fmt.Errorf("error creating or patching object: %w", err)
			}
			backendList = append(backendList, backendName)
		}
	}
	return backendList, nil
}

// CreateStorageClass creates a storage class for the storage driver
func (aws *AWS) CreateStorageClass() error {
	var driver string
	for _, svm := range aws.SVMs {
		for _, protocol := range svm.Protocols {
			name := getFsxnStorageClassName(svm.FsxnID, protocol)
			if protocol == sa.NFS {
				driver = config.OntapNASStorageDriverName
			} else if protocol == sa.ISCSI {
				driver = config.OntapSANStorageDriverName
			}
			storageClassYaml := getFsxnStorageClassYaml(name, driver)
			if err := aws.ConfClient.CreateOrPatchObject(confClients.OStorageClass, name,
				aws.TridentNamespace, storageClassYaml); err != nil {
				return fmt.Errorf("error creating or patching object: %w", err)
			}
		}
	}
	return nil
}

// CreateSnapshotClass creates a snapshot class for the storage driver
func (aws *AWS) CreateSnapshotClass() error {
	fsxnSnapClassYAML := GetVolumeSnapshotClassYAML(NetAppSnapshotClassName)
	return aws.ConfClient.CreateOrPatchObject(confClients.OSnapshotClass, NetAppSnapshotClassName,
		"", fsxnSnapClassYAML)
}

// GetCloudProvider returns the cloud provider for the storage driver
func (aws *AWS) GetCloudProvider() string {
	return k8sclient.CloudProviderAWS
}

// getFsxnTBCYaml returns the FsxN Trident backend config name
func getFsxnBackendName(fsxnId, protocolType string) string {
	return fmt.Sprintf(BackendNamePattern, fsxnId, protocolType)
}

// getFsxnStorageClassName returns the storage class name for the FSxN backend
func getFsxnStorageClassName(fsxnId, protocolType string) string {
	return fmt.Sprintf(StorageClassNamePattern, fsxnId, protocolType)
}
