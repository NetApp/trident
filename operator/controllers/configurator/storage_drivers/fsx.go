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
	"github.com/netapp/trident/internal/crypto"
	. "github.com/netapp/trident/logging"
	confClients "github.com/netapp/trident/operator/controllers/configurator/clients"
	operatorV1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
	tridentV1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/storage_drivers/ontap/awsapi"
	"github.com/netapp/trident/utils/errors"
)

const (
	SvmStateCreated = "CREATED"
	AWSRegion       = "AWS_REGION"

	TridentSecretPattern        = "trident-%s"
	SvmNamePattern              = "trident-%s"
	StorageClassNamePattern     = "trident-%s-%s"
	BackendNamePattern          = "trident-%s-%s"
	SCManagedBackendNamePattern = "trident-%s-%s-%s"
	VsAdmin                     = "vsadmin"
	Description                 = "Trident secret for FsxN for ONTAP"

	// Tags for the secret
	FileSystemId = "file-system-id"
)

type AWS struct {
	AwsConfig
	ConfClient       confClients.ConfiguratorClientInterface
	AwsClient        *awsapi.Client
	TBCNamePrefix    string
	TridentNamespace string
	SCManagedTConf   bool
	SecretARNName    string
	TConfSpec        map[string]interface{}
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

// NewFSxNInstance creates a new instance of the AWS struct and populates it with the provided CRs and client
func NewFSxNInstance(
	torcCR *operatorV1.TridentOrchestrator, configuratorCR *operatorV1.TridentConfigurator,
	client confClients.ConfiguratorClientInterface, scManagedTconf bool,
) (*AWS, error) {
	if torcCR == nil {
		return nil, fmt.Errorf("empty torc CR")
	}

	if configuratorCR == nil {
		return nil, fmt.Errorf("empty AWS FSxN configurator CR")
	}

	if client == nil {
		return nil, fmt.Errorf("invalid client")
	}

	awsConfig := AwsConfig{}
	if err := json.Unmarshal(configuratorCR.Spec.Raw, &awsConfig); err != nil {
		Log().Errorf("Error occured while unmarshalling configurator CR: %v", err)
		return nil, err
	}

	var tConfSpec map[string]interface{}
	if err := json.Unmarshal(configuratorCR.Spec.Raw, &tConfSpec); err != nil {
		Log().Errorf("Error occurred while unmarshalling configurator CR: %v", err)
		return nil, err
	}

	awsSecretARNName := ""
	if scManagedTconf {
		if credentialsName, ok := tConfSpec["credentialsName"].(string); ok {
			awsSecretARNName = credentialsName
		}
	}
	return &AWS{
		AwsConfig:        awsConfig,
		ConfClient:       client,
		TBCNamePrefix:    configuratorCR.Name,
		TridentNamespace: torcCR.Spec.Namespace,
		SCManagedTConf:   scManagedTconf,
		SecretARNName:    awsSecretARNName,
		TConfSpec:        tConfSpec,
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
		if err := aws.processFSxNInstance(context.Background(), key, fsxnInstance); err != nil {
			return err
		}
	}

	return nil
}

// processFSxNInstance processes the auto-backend configuration for the FSxN instance.
// It creates the SVM if it does not exist and creates the secret for the SVM.
func (aws *AWS) processFSxNInstance(ctx context.Context, key int, svm SVM) error {
	var (
		svmName       string
		secretName    string
		secretARN     string
		svmExists     bool
		ErrStatusCode = "StatusCode: 400"
	)
	svmName = svm.SvmName
	if svmName == "" && !aws.SCManagedTConf {
		svmName = fmt.Sprintf(SvmNamePattern, svm.FsxnID)
		svm.SvmName = svmName
	}
	_, err := aws.AwsClient.GetFilesystemByID(ctx, svm.FsxnID)
	if errors.IsNotFoundError(err) || (err != nil && strings.Contains(err.Error(), ErrStatusCode)) {
		err = aws.cleanUpFSxNRelatedObjects(svm, svm.Protocols)
		if err != nil {
			Log().Error(err)
		}
		return fmt.Errorf("error occurred while getting fsxn id: %v : %v", svm.FsxnID, err)
	}
	Log().Debugf("Filesystem ID: %s exists", svm.FsxnID)
	if !aws.SCManagedTConf {
		secretName = getAWSSecretName(svmName)
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
					"password": crypto.GenerateRandomPassword(ctx, 10, true, true, true, true),
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
	} else {
		// If the tconf is managed by the resource monitor, use the secret ARN name from the tconf
		aws.SVMs[key].SecretARNName = aws.SecretARNName
	}

	aws.AwsClient.SetClientFsConfig(svm.FsxnID)

	if !aws.SCManagedTConf {
		svmList, err := aws.AwsClient.GetSVMs(ctx)
		if err != nil {
			return fmt.Errorf("error occurred while getting SVMs: %w", err)
		}
		svmExists, err = aws.findAndSetSVM(ctx, svmList, svmName, key)
		if err != nil {
			return err
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
	} else {
		svmList, err := aws.AwsClient.GetSVMs(ctx)
		if err != nil {
			return fmt.Errorf("error occurred while getting SVMs: %w", err)
		}

		svmExists, err = aws.findAndSetSVM(ctx, svmList, svmName, key)
		if err != nil {
			return err
		}

		// If svmName was specified but not found, fail
		if !svmExists && svmName != "" {
			return fmt.Errorf("specified SVM %q not found on filesystem %s", svmName, svm.FsxnID)
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
			backendName = getFSxNBackendName(aws, svm.FsxnID, protocol)
			backendYAML := getFsxnTBCYaml(svm, aws.TridentNamespace, backendName, protocol, aws.TBCNamePrefix, aws.SCManagedTConf, aws.TConfSpec)
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
	// If the storage class is managed by the resource monitor, do not create a storage class
	if aws.SCManagedTConf {
		return nil
	}
	for _, svm := range aws.SVMs {
		for _, protocol := range svm.Protocols {
			name := getFSxNStorageClassName(svm.FsxnID, protocol)
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

// DeleteBackend deletes the backend if the FSxN instance is deleted
func (aws *AWS) DeleteBackend(request map[string]interface{}) error {
	protocols := request["protocols"].([]string)
	fsxnId := request["FSxNID"].(string)
	for _, protocol := range protocols {
		backendName := getFSxNBackendName(aws, fsxnId, protocol)
		if err := aws.ConfClient.DeleteObject(confClients.OBackend, backendName, aws.TridentNamespace); err != nil {
			return fmt.Errorf("error occurred while deleting backend: %w", err)
		}
	}
	return nil
}

// DeleteStorageClass deletes the storage class if the FSxN instance is deleted
func (aws *AWS) DeleteStorageClass(request map[string]interface{}) error {
	protocols := request["protocols"].([]string)
	fsxnId := request["FSxNID"].(string)
	for _, protocol := range protocols {
		name := getFSxNStorageClassName(fsxnId, protocol)
		if err := aws.ConfClient.DeleteObject(confClients.OStorageClass, name, aws.TridentNamespace); err != nil {
			return fmt.Errorf("error occurred while deleting storage class: %w", err)
		}
	}
	return nil
}

// DeleteSnapshotClass deletes the snapshot class if the FSxN instance is deleted
func (aws *AWS) DeleteSnapshotClass() error {
	tbcList, err := aws.ConfClient.ListObjects(confClients.OBackend, "")
	if err != nil {
		return fmt.Errorf("error occurred while listing TBC objects: %w", err)
	}
	if len(tbcList.(*tridentV1.TridentBackendList).Items) == 0 {
		if err := aws.ConfClient.DeleteObject(confClients.OSnapshotClass, NetAppSnapshotClassName, ""); err != nil {
			return fmt.Errorf("error occurred while deleting snapshot class: %w", err)
		}
	}

	return nil
}

// findAndSetSVM locates an SVM in the provided list (using the first available if svmName is empty,
// or matching by name if specified) and sets its name and management LIF.
// Returns true if found and set, false if not found, or an error if no SVMs are available.
func (aws *AWS) findAndSetSVM(ctx context.Context, svmList *[]*awsapi.SVM, svmName string, key int) (bool, error) {
	// If svmName is empty, pick the first available SVM
	if svmName == "" {
		if len(*svmList) == 0 {
			return false, fmt.Errorf("no SVMs available on filesystem")
		}
		firstSVM := (*svmList)[0]
		Log().Debugf("No SVM name specified, using first available SVM: %v for fsxnId: %v",
			firstSVM.Name, firstSVM.FilesystemID)
		aws.SVMs[key].SvmName = firstSVM.Name
		aws.SVMs[key].ManagementLIF = firstSVM.MgtEndpoint.IPAddresses[0]
		return true, nil
	}

	// If svmName is specified, find exact match
	for _, storageVirtualMachine := range *svmList {
		if storageVirtualMachine.Name == svmName {
			Log().Debugf("SVM already exists: %v for fsxnId: %v",
				storageVirtualMachine.Name, storageVirtualMachine.FilesystemID)
			aws.SVMs[key].SvmName = storageVirtualMachine.Name
			aws.SVMs[key].ManagementLIF = storageVirtualMachine.MgtEndpoint.IPAddresses[0]
			return true, nil
		}
	}

	return false, nil
}

// cleanUpFSxNRelatedObjects cleans up the FSxN instance related objects like storage class, backend, and AWS secret
func (aws *AWS) cleanUpFSxNRelatedObjects(svm SVM, protocols []string) (err error) {
	cleanupRequest := map[string]interface{}{
		"FSxNID":    svm.FsxnID,
		"protocols": protocols,
	}

	// Delete the storage class
	if deleteErr := aws.DeleteStorageClass(cleanupRequest); err != nil {
		err = fmt.Errorf("failed to delete VolumeSnapshotClass: %w", deleteErr)
	}

	// Delete the backend
	if deleteErr := aws.DeleteBackend(cleanupRequest); err != nil {
		err = fmt.Errorf("failed to delete backend: %w", deleteErr)
	}

	// Delete the AWS secret
	if deleteErr := deleteSecret(context.Background(), aws, getAWSSecretName(svm.SvmName)); err != nil {
		err = fmt.Errorf("failed to delete AWS secret: %w", deleteErr)
	}

	// Delete the VolumeSnapshotClass if no backend is present
	if deleteErr := aws.DeleteSnapshotClass(); err != nil {
		err = fmt.Errorf("failed to delete VolumeSnapshotClass: %w", deleteErr)
	}

	return
}

func deleteSecret(ctx context.Context, aws *AWS, secretName string) error {
	if err := aws.AwsClient.DeleteSecret(ctx, secretName); err != nil {
		return fmt.Errorf("error occurred while deleting secret: %w", err)
	}
	return nil
}

// getFSxNBackendName returns the FsxN Trident backend config name
func getFSxNBackendName(aws *AWS, fsxnId, protocolType string) string {
	if aws.SCManagedTConf {
		scName := strings.TrimPrefix(aws.TBCNamePrefix, "tconf-")
		return fmt.Sprintf(SCManagedBackendNamePattern, scName, fsxnId, protocolType)
	}
	return fmt.Sprintf(BackendNamePattern, fsxnId, protocolType)
}

// getFSxNStorageClassName returns the storage class name for the FSxN backend
func getFSxNStorageClassName(fsxnId, protocolType string) string {
	return fmt.Sprintf(StorageClassNamePattern, fsxnId, protocolType)
}

// getAWSSecretName returns the AWS secret name for the FSxN backend
func getAWSSecretName(svmName string) string {
	return fmt.Sprintf(TridentSecretPattern, strings.TrimPrefix(svmName, "trident-"))
}
