// Copyright 2023 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"strings"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/awsapi"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

// SetSvmCredentials Pull SVM credentials out of AWS secret store and enter them into the config.
func SetSvmCredentials(ctx context.Context, secretARN string, api awsapi.AWSAPI, config *drivers.OntapStorageDriverConfig) (err error) {
	secret, secretErr := api.GetSecret(ctx, secretARN)
	secretMap := secret.SecretMap
	if secretErr != nil {
		return fmt.Errorf("could not retrieve credentials from AWS Secrets Manager; %w", secretErr)
	}

	if username, ok := secretMap["username"]; !ok {
		return fmt.Errorf("%s driver must include username in the secret referenced by Credentials",
			config.StorageDriverName)
	} else {
		config.Username = username
	}

	if password, ok := secretMap["password"]; !ok {
		return fmt.Errorf("%s driver must include password in the secret referenced by Credentials",
			config.StorageDriverName)
	} else {
		config.Password = password
	}

	return nil
}

// initializeAWSDriver returns an AWS SDK client.  It does all the other initialization needed for
// AWS (FSxN or CVO), such that this is the only method the drivers have to call.
func initializeAWSDriver(
	ctx context.Context, config *drivers.OntapStorageDriverConfig,
) (api awsapi.AWSAPI, err error) {
	// No AWS config is normal for on-prem ONTAP, so just return
	if config.AWSConfig == nil {
		return nil, nil
	}

	// Create the AWS API client and read ONTAP credentials if so configured
	api, err = initializeAWSAPI(ctx, config)
	if err != nil {
		return nil, err
	}

	// Validate FSxN filesystem if so configured
	if config.AWSConfig.FSxFilesystemID != "" {
		err = validateFSxFilesystem(ctx, api, config)
		if err != nil {
			return nil, fmt.Errorf("error validating FSx filesystem; %v", err)
		}
	}

	return
}

// initializeAWSAPI returns an AWS SDK client.  If configured, it detects and pulls ONTAP credentials from
// AWS Secrets Manager.  All discovered values are written back to the supplied OntapStorageDriverConfig.
func initializeAWSAPI(
	ctx context.Context, config *drivers.OntapStorageDriverConfig,
) (awsapi.AWSAPI, error) {
	fields := LogFields{"Method": "initializeAWSAPI", "Type": "ontap_common"}
	Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace(
		">>>> initializeAWSAPI")
	defer Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace(
		"<<<< initializeAWSAPI")

	secretManagerRegion := config.AWSConfig.APIRegion
	// Check if the configured credentials refer to a secret in AWS Secrets Manager
	secretARN, secretErr := getAWSSecretsManagerARNFromConfig(ctx, config)
	if secretErr != nil && !errors.IsNotFoundError(secretErr) {
		// We find ARN, but we get an error.
		return nil, secretErr
	}
	if secretARN != "" {
		var err error
		secretManagerRegion, _, _, err = awsapi.ParseSecretARN(secretARN)
		if err != nil {
			return nil, err
		}
	}

	api, err := awsapi.NewClient(ctx, awsapi.ClientConfig{
		APIRegion:           config.AWSConfig.APIRegion,
		APIKey:              config.AWSConfig.APIKey,
		SecretKey:           config.AWSConfig.SecretKey,
		FilesystemID:        config.AWSConfig.FSxFilesystemID,
		DebugTraceFlags:     config.DebugTraceFlags,
		SecretManagerRegion: secretManagerRegion,
	})
	if err != nil {
		return nil, err
	}

	if secretErr != nil && errors.IsNotFoundError(secretErr) {
		// No ARN found, so continue with any configured explicit credentials
		return api, nil
	}

	err = SetSvmCredentials(ctx, secretARN, api, config)
	if err != nil {
		return nil, err
	}

	return api, nil
}

// validateFSxFilesystem validates a configured FSx filesystem, validates or infers the SVM, and optionally
// determines the SVM management LIF.  All discovered values are written back to the supplied OntapStorageDriverConfig.
func validateFSxFilesystem(
	ctx context.Context, api awsapi.AWSAPI, config *drivers.OntapStorageDriverConfig,
) error {
	fields := LogFields{"Method": "validateFSxFilesystem", "Type": "ontap_common"}
	Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace(
		">>>> validateFSxFilesystem")
	defer Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace(
		"<<<< validateFSxFilesystem")

	// Ensure FSx filesystem exists
	if config.AWSConfig.FSxFilesystemID == "" {
		return errors.New("filesystem ID in config must be specified")
	}
	_, err := api.GetFilesystemByID(ctx, config.AWSConfig.FSxFilesystemID)
	if err != nil {
		return fmt.Errorf("filesystem with ID %s not found; %w", config.AWSConfig.FSxFilesystemID, err)
	}

	Logc(ctx).WithField("region", config.AWSConfig.APIRegion).Debug("FSxN SDK access OK.")

	// Build list of SVM names
	discoveredSVMs, err := api.GetSVMs(ctx)
	if err != nil {
		return fmt.Errorf("could not retrieve FSxN SVMs; %w", err)
	}
	discoveredSVMNames := make([]string, 0)
	for _, svm := range *discoveredSVMs {
		discoveredSVMNames = append(discoveredSVMNames, svm.Name)
	}

	// Validate or infer SVM name
	if config.SVM == "" {
		if len(discoveredSVMNames) == 1 {
			config.SVM = discoveredSVMNames[0]
		} else {
			return fmt.Errorf("no SVM specified and multiple SVMs exist in filesystem %s",
				config.AWSConfig.FSxFilesystemID)
		}
	} else {
		if !utils.SliceContainsString(discoveredSVMNames, config.SVM) {
			return fmt.Errorf("SVM %s does not exist in filesystem %s", config.SVM, config.AWSConfig.FSxFilesystemID)
		}
	}

	var svm *awsapi.SVM
	for _, discoveredSVM := range *discoveredSVMs {
		if discoveredSVM.Name == config.SVM {
			svm = discoveredSVM
			break
		}
	}

	// Infer management LIF
	if config.ManagementLIF == "" {
		if len(svm.MgtEndpoint.IPAddresses) > 0 {
			config.ManagementLIF = svm.MgtEndpoint.IPAddresses[0]
		} else {
			config.ManagementLIF = svm.MgtEndpoint.DNSName
		}
	}

	return err
}

// getAWSSecretsManagerARNFromConfig examines an OntapStorageDriverConfig to find the AWS ARN that identifies the
// secret containing a set of SVM credentials.
func getAWSSecretsManagerARNFromConfig(_ context.Context, config *drivers.OntapStorageDriverConfig) (string, error) {
	if config.Credentials != nil && config.Credentials[drivers.KeyType] == string(drivers.CredentialStoreAWSARN) {
		return config.Credentials[drivers.KeyName], nil
	}

	if strings.HasPrefix(config.Username, "arn:aws:secretsmanager:") {
		return config.Username, nil
	}

	return "", errors.NotFoundError("%s driver with FSxN personality must include Credentials of type %s "+
		"in the configuration", config.StorageDriverName, string(drivers.CredentialStoreAWSARN))
}

// destroyFSxVolume discovers and deletes a volume using the FSx SDK.  This is needed to delete a volume in the case
// that FSx has created a hidden SnapMirror relationship for the purpose of creating backups.  If the volume isn't
// found, it returns a NotFoundError so that the client may choose to delete the volume using the underlying ONTAP
// ZAPI/REST client.
func destroyFSxVolume(
	ctx context.Context, fsx awsapi.AWSAPI, volConfig *storage.VolumeConfig, config *drivers.OntapStorageDriverConfig,
) error {
	name := volConfig.InternalName
	fields := LogFields{
		"Method": "destroyFSxVolume",
		"Type":   "ontap_common",
		"name":   name,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> destroyFSxVolume")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< destroyFSxVolume")

	if config.AWSConfig == nil || fsx == nil {
		return errors.New("FSxN not configured")
	}

	// If volume doesn't exist, return success
	volumeExists, extantVolume, err := fsx.VolumeExists(ctx, volConfig)
	if err != nil {
		return fmt.Errorf("error checking for existing FSx volume: %v", err)
	}
	if !volumeExists {
		Logc(ctx).WithField("volume", name).Warn("FSx volume already deleted.")
		return errors.NotFoundError("volume %s not found", name)
	} else if extantVolume.State == awsapi.StateDeleting {
		// This is a retry, so give it more time before giving up again.
		_, err = fsx.WaitForVolumeStates(
			ctx, extantVolume, []string{awsapi.StateDeleted}, []string{awsapi.StateFailed}, awsapi.RetryDeleteTimeout)
		return err
	}

	// Delete the volume
	if err = fsx.DeleteVolume(ctx, extantVolume); err != nil {
		return err
	}

	// Wait for deletion to complete
	deleteTimeout := awsapi.RetryDeleteTimeout
	if tridentconfig.CurrentDriverContext == tridentconfig.ContextDocker {
		deleteTimeout = awsapi.DefaultDeleteTimeout
	}
	_, err = fsx.WaitForVolumeStates(
		ctx, extantVolume, []string{awsapi.StateDeleted}, []string{awsapi.StateFailed}, deleteTimeout)
	return err
}
