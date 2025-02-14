// Copyright 2025 NetApp, Inc. All Rights Reserved.

// Package awsapi provides a high-level interface to the AWS FSx for NetApp ONTAP API.
package awsapi

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/middleware"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/fsx"
	fsxtypes "github.com/aws/aws-sdk-go-v2/service/fsx/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	secretmanagertypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/cenkalti/backoff/v4"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

const (
	RetryDeleteTimeout   = 1 * time.Second
	DefaultDeleteTimeout = 60 * time.Second
	PaginationLimit      = 100
)

var (
	volumeARNRegex = regexp.MustCompile(`^arn:(?P<partition>aws|aws-cn|aws-us-gov){1}:fsx:(?P<region>[^:]+):(?P<accountID>\d{12}):volume/(?P<filesystemID>[A-z0-9-]+)/(?P<volumeID>[A-z0-9-]+)$`)
	secretARNRegex = regexp.MustCompile(`^arn:(?P<partition>aws|aws-cn|aws-us-gov){1}:secretsmanager:(?P<region>[^:]+):(?P<accountID>\d{12}):secret:(?P<secretName>[A-z0-9/_+=.@-]+)-[A-z0-9/_+=.@-]{6}$`)
)

// ClientConfig holds configuration data for the API driver object.
type ClientConfig struct {
	// Optional AWS SDK authentication parameters
	APIRegion string
	APIKey    string
	SecretKey string

	// FSx Filesystem
	FilesystemID string
	// Secret Manager Region
	SecretManagerRegion string

	// Options
	DebugTraceFlags map[string]bool
}

type Client struct {
	config        *ClientConfig
	fsxClient     *fsx.Client
	secretsClient *secretsmanager.Client
}

func createAWSConfig(ctx context.Context, region, apiKey, secretKey string) (aws.Config, error) {
	var cfg aws.Config
	var err error

	if apiKey != "" {
		// Explicit credentials
		if region != "" {
			cfg, err = awsconfig.LoadDefaultConfig(ctx,
				awsconfig.WithCredentialsProvider(
					credentials.NewStaticCredentialsProvider(apiKey, secretKey, ""),
				),
				awsconfig.WithRegion(region),
			)
		} else {
			cfg, err = awsconfig.LoadDefaultConfig(ctx,
				awsconfig.WithCredentialsProvider(
					credentials.NewStaticCredentialsProvider(apiKey, secretKey, ""),
				),
			)
		}
	} else {
		// Implicit credentials
		if region != "" {
			cfg, err = awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
		} else {
			cfg, err = awsconfig.LoadDefaultConfig(ctx)
		}
	}

	return cfg, err
}

// NewClient is a factory method for creating a new instance.
func NewClient(ctx context.Context, config ClientConfig) (*Client, error) {
	fsxCfg, err := createAWSConfig(ctx, config.APIRegion, config.APIKey, config.SecretKey)
	if err != nil {
		return nil, err
	}

	secMngCfg, err := createAWSConfig(ctx, config.SecretManagerRegion, config.APIKey, config.SecretKey)
	if err != nil {
		return nil, err
	}

	client := &Client{
		config:        &config,
		fsxClient:     fsx.NewFromConfig(fsxCfg),
		secretsClient: secretsmanager.NewFromConfig(secMngCfg),
	}

	return client, nil
}

func (d *Client) SetClientFsConfig(fileSystemId string) {
	d.config.FilesystemID = fileSystemId
}

func (d *Client) CreateSecret(ctx context.Context, request *SecretCreateRequest) (*Secret, error) {
	logFields := LogFields{
		"API":         "CreateSecret",
		"SecretName":  request.Name,
		"Description": request.Description,
	}

	secretBytes, err := json.Marshal(request.SecretData)
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Could not marshal secret data.")
		return nil, fmt.Errorf("error marshaling secret data; %w", err)
	}

	// Add tags to the secret request if they are provided
	tags := make([]secretmanagertypes.Tag, 0)
	for k, v := range request.Tags {
		tags = append(tags, secretmanagertypes.Tag{
			Key:   convert.ToPtr(k),
			Value: convert.ToPtr(v),
		})
	}

	input := &secretsmanager.CreateSecretInput{
		Name:         convert.ToPtr(request.Name),
		Description:  convert.ToPtr(request.Description),
		SecretString: convert.ToPtr(string(secretBytes)),
		Tags:         tags,
	}

	createSecretOutput, err := d.secretsClient.CreateSecret(ctx, input)
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Could not create secret.")
		return nil, fmt.Errorf("error creating secret; %w", err)
	}

	logFields["requestID"], _ = middleware.GetRequestIDMetadata(createSecretOutput.ResultMetadata)
	Logc(ctx).WithFields(logFields).Info("Secret created.")

	return &Secret{
		SecretARN: DerefString(createSecretOutput.ARN),
	}, nil
}

func (d *Client) GetSecret(ctx context.Context, secretARN string) (*Secret, error) {
	input := &secretsmanager.GetSecretValueInput{
		SecretId:     convert.ToPtr(secretARN),
		VersionStage: convert.ToPtr("AWSCURRENT"),
	}

	secretData, err := d.secretsClient.GetSecretValue(ctx, input)
	if err != nil {
		return nil, err
	}
	secret := &Secret{
		FSxObject: FSxObject{
			ARN:  DerefString(secretData.ARN),
			ID:   DerefString(secretData.VersionId),
			Name: DerefString(secretData.Name),
		},
	}
	var secretMap map[string]string
	if err = json.Unmarshal([]byte(DerefString(secretData.SecretString)), &secretMap); err != nil {
		return nil, err
	}
	secret.SecretMap = secretMap
	secret.SecretARN = *secretData.ARN
	return secret, nil
}

// DeleteSecret deletes a secret by its ARN/name.
func (d *Client) DeleteSecret(ctx context.Context, secretARN string) error {
	logFields := LogFields{
		"API":       "DeleteSecret",
		"secretARN": secretARN,
	}
	deleteSecretInput := &secretsmanager.DeleteSecretInput{
		SecretId:                   convert.ToPtr(secretARN),
		ForceDeleteWithoutRecovery: convert.ToPtr(true),
	}
	if _, err := d.secretsClient.DeleteSecret(ctx, deleteSecretInput); err != nil {
		logFields["requestID"] = GetRequestIDFromError(err)
		Logc(ctx).WithFields(logFields).WithError(err).Error("Could not delete secret.")
		return fmt.Errorf("error deleting secret; %w", err)
	}
	Logc(ctx).WithFields(logFields).Info("Secret deleted.")
	return nil
}

// ParseVolumeARN parses the AWS-style ARN for a volume.
func ParseVolumeARN(volumeARN string) (region, accountID, filesystemID, volumeID string, err error) {
	match := volumeARNRegex.FindStringSubmatch(volumeARN)

	if match == nil {
		err = fmt.Errorf("volume ARN %s is invalid", volumeARN)
		return
	}

	paramsMap := make(map[string]string)
	for i, name := range volumeARNRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	region = paramsMap["region"]
	accountID = paramsMap["accountID"]
	filesystemID = paramsMap["filesystemID"]
	volumeID = paramsMap["volumeID"]

	return
}

// ParseSecretARN parses the AWS-style ARN for a secret.
func ParseSecretARN(secretARN string) (region, accountID, secretName string, err error) {
	match := secretARNRegex.FindStringSubmatch(secretARN)

	if match == nil {
		err = fmt.Errorf("secret ARN %s is invalid", secretARN)
		return
	}

	paramsMap := make(map[string]string)
	for i, name := range secretARNRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	region = paramsMap["region"]
	accountID = paramsMap["accountID"]
	secretName = paramsMap["secretName"]

	return
}

func (d *Client) GetFilesystems(ctx context.Context) (*[]*Filesystem, error) {
	logFields := LogFields{"API": "DescribeFileSystemsPaginator.NextPage"}

	input := &fsx.DescribeFileSystemsInput{}

	paginator := fsx.NewDescribeFileSystemsPaginator(d.fsxClient, input,
		func(o *fsx.DescribeFileSystemsPaginatorOptions) { o.Limit = PaginationLimit })

	var filesystems []*Filesystem

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			logFields["requestID"] = GetRequestIDFromError(err)
			Logc(ctx).WithFields(logFields).WithError(err).Error("Could not iterate filesystems.")
			return nil, fmt.Errorf("error iterating filesystems; %w", err)
		}

		logFields["requestID"], _ = middleware.GetRequestIDMetadata(output.ResultMetadata)

		for _, f := range output.FileSystems {
			filesystems = append(filesystems, d.getFilesystemFromFSxFilesystem(f))
		}
	}

	logFields["count"] = len(filesystems)
	Logc(ctx).WithFields(logFields).Debug("Read FSx filesystems.")

	return &filesystems, nil
}

func (d *Client) GetFilesystemByID(ctx context.Context, filesystemID string) (*Filesystem, error) {
	logFields := LogFields{
		"API":          "DescribeFileSystems",
		"filesystemID": filesystemID,
	}

	input := &fsx.DescribeFileSystemsInput{
		FileSystemIds: []string{filesystemID},
	}

	output, err := d.fsxClient.DescribeFileSystems(ctx, input)
	if err != nil {
		logFields["requestID"] = GetRequestIDFromError(err)
		Logc(ctx).WithFields(logFields).WithError(err).Error("Could not iterate filesystems.")
		return nil, fmt.Errorf("error iterating filesystems; %w", err)
	}
	if len(output.FileSystems) == 0 {
		return nil, errors.NotFoundError(fmt.Sprintf("filesystem %s not found", filesystemID))
	}
	if len(output.FileSystems) > 1 {
		return nil, errors.NotFoundError(fmt.Sprintf("multiple filesystems with ID '%s' found", filesystemID))
	}

	logFields["requestID"], _ = middleware.GetRequestIDMetadata(output.ResultMetadata)
	Logc(ctx).WithFields(logFields).Debug("Found FSx filesystem by ID.")

	return d.getFilesystemFromFSxFilesystem(output.FileSystems[0]), nil
}

func (d *Client) getFilesystemFromFSxFilesystem(f fsxtypes.FileSystem) *Filesystem {
	filesystem := &Filesystem{
		FSxObject: FSxObject{
			ARN:  DerefString(f.ResourceARN),
			ID:   DerefString(f.FileSystemId),
			Name: DerefString(f.DNSName),
		},
		Created: DerefTime(f.CreationTime),
		OwnerID: DerefString(f.OwnerId),
		State:   string(f.Lifecycle),
		VPCID:   DerefString(f.VpcId),
	}

	return filesystem
}

func (d *Client) CreateSVM(ctx context.Context, request *SVMCreateRequest) (*SVM, error) {
	logFields := LogFields{
		"API":        "CreateStorageVirtualMachine",
		"svmName":    request.Name,
		"filesystem": d.config.FilesystemID,
	}

	secret, err := d.GetSecret(ctx, request.SecretARN)
	if err != nil {
		logFields["requestID"] = GetRequestIDFromError(err)
		Logc(ctx).WithFields(logFields).WithError(err).Error("Could not get secret.")
		return nil, fmt.Errorf("error getting secret; %w", err)
	}
	input := &fsx.CreateStorageVirtualMachineInput{
		FileSystemId:     convert.ToPtr(d.config.FilesystemID),
		Name:             convert.ToPtr(request.Name),
		SvmAdminPassword: convert.ToPtr(secret.SecretMap["password"]),
	}

	output, err := d.fsxClient.CreateStorageVirtualMachine(ctx, input)
	if err != nil {
		logFields["requestID"] = GetRequestIDFromError(err)
		Logc(ctx).WithFields(logFields).WithError(err).Error("Could not create SVM.")
		return nil, fmt.Errorf("error creating SVM; %w", err)
	}

	newSVM := d.getSVMFromFSxSVM(*output.StorageVirtualMachine)

	logFields["requestID"], _ = middleware.GetRequestIDMetadata(output.ResultMetadata)
	logFields["svmID"] = newSVM.ID
	Logc(ctx).WithFields(logFields).Info("SVM create request issued.")

	return newSVM, nil
}

func (d *Client) GetSVMs(ctx context.Context) (*[]*SVM, error) {
	logFields := LogFields{"API": "DescribeStorageVirtualMachinesPaginator.NextPage"}

	input := &fsx.DescribeStorageVirtualMachinesInput{
		Filters: []fsxtypes.StorageVirtualMachineFilter{
			{
				Name:   fsxtypes.StorageVirtualMachineFilterNameFileSystemId,
				Values: []string{d.config.FilesystemID},
			},
		},
	}

	paginator := fsx.NewDescribeStorageVirtualMachinesPaginator(d.fsxClient, input,
		func(o *fsx.DescribeStorageVirtualMachinesPaginatorOptions) { o.Limit = PaginationLimit })

	var svms []*SVM

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			logFields["requestID"] = GetRequestIDFromError(err)
			Logc(ctx).WithFields(logFields).WithError(err).Error("Could not iterate SVMs.")
			return nil, fmt.Errorf("error iterating SVMs; %w", err)
		}

		logFields["requestID"], _ = middleware.GetRequestIDMetadata(output.ResultMetadata)

		for _, svm := range output.StorageVirtualMachines {
			svms = append(svms, d.getSVMFromFSxSVM(svm))
		}
	}

	logFields["count"] = len(svms)
	Logc(ctx).WithFields(logFields).Debug("Read FSx SVMs.")

	return &svms, nil
}

func (d *Client) GetSVMByID(ctx context.Context, svmID string) (*SVM, error) {
	logFields := LogFields{
		"API":   "DescribeStorageVirtualMachines",
		"svmID": svmID,
	}

	input := &fsx.DescribeStorageVirtualMachinesInput{
		Filters: []fsxtypes.StorageVirtualMachineFilter{
			{
				Name:   fsxtypes.StorageVirtualMachineFilterNameFileSystemId,
				Values: []string{d.config.FilesystemID},
			},
		},
		StorageVirtualMachineIds: []string{svmID},
	}

	output, err := d.fsxClient.DescribeStorageVirtualMachines(ctx, input)
	if err != nil {
		logFields["requestID"] = GetRequestIDFromError(err)
		Logc(ctx).WithFields(logFields).WithError(err).Error("Could not iterate SVMs.")
		return nil, fmt.Errorf("error iterating SVMs; %w", err)
	}
	if len(output.StorageVirtualMachines) == 0 {
		return nil, errors.NotFoundError(fmt.Sprintf("SVM %s not found", svmID))
	}
	if len(output.StorageVirtualMachines) > 1 {
		return nil, errors.NotFoundError(fmt.Sprintf("multiple SVMs with ID '%s' found", svmID))
	}

	logFields["requestID"], _ = middleware.GetRequestIDMetadata(output.ResultMetadata)
	Logc(ctx).WithFields(logFields).Debug("Found FSx SVM by ID.")

	return d.getSVMFromFSxSVM(output.StorageVirtualMachines[0]), nil
}

func (d *Client) getSVMFromFSxSVM(s fsxtypes.StorageVirtualMachine) *SVM {
	svm := &SVM{
		FSxObject: FSxObject{
			ARN:  DerefString(s.ResourceARN),
			ID:   DerefString(s.StorageVirtualMachineId),
			Name: DerefString(s.Name),
		},
		Created:      DerefTime(s.CreationTime),
		FilesystemID: DerefString(s.FileSystemId),
		State:        string(s.Lifecycle),
		Subtype:      string(s.Subtype),
		UUID:         DerefString(s.UUID),
	}

	if s.Endpoints != nil {
		if s.Endpoints.Iscsi != nil {
			svm.IscsiEndpoint = &Endpoint{
				DNSName:     DerefString(s.Endpoints.Iscsi.DNSName),
				IPAddresses: s.Endpoints.Iscsi.IpAddresses,
			}
		}
		if s.Endpoints.Management != nil {
			svm.MgtEndpoint = &Endpoint{
				DNSName:     DerefString(s.Endpoints.Management.DNSName),
				IPAddresses: s.Endpoints.Management.IpAddresses,
			}
		}
		if s.Endpoints.Nfs != nil {
			svm.NFSEndpoint = &Endpoint{
				DNSName:     DerefString(s.Endpoints.Nfs.DNSName),
				IPAddresses: s.Endpoints.Nfs.IpAddresses,
			}
		}
		if s.Endpoints.Smb != nil {
			svm.SMBEndpoint = &Endpoint{
				DNSName:     DerefString(s.Endpoints.Smb.DNSName),
				IPAddresses: s.Endpoints.Smb.IpAddresses,
			}
		}
	}

	return svm
}

func (d *Client) GetVolumes(ctx context.Context) (*[]*Volume, error) {
	logFields := LogFields{"API": "DescribeVolumesPaginator.NextPage"}

	input := &fsx.DescribeVolumesInput{
		Filters: []fsxtypes.VolumeFilter{
			{
				Name:   fsxtypes.VolumeFilterNameFileSystemId,
				Values: []string{d.config.FilesystemID},
			},
		},
	}

	paginator := fsx.NewDescribeVolumesPaginator(d.fsxClient, input,
		func(o *fsx.DescribeVolumesPaginatorOptions) { o.Limit = PaginationLimit })

	var volumes []*Volume

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			logFields["requestID"] = GetRequestIDFromError(err)
			Logc(ctx).WithFields(logFields).WithError(err).Error("Could not iterate volumes.")
			return nil, fmt.Errorf("error iterating volumes; %w", err)
		}

		logFields["requestID"], _ = middleware.GetRequestIDMetadata(output.ResultMetadata)

		for _, v := range output.Volumes {
			volumes = append(volumes, d.getVolumeFromFSxVolume(v))
		}
	}

	logFields["count"] = len(volumes)
	Logc(ctx).WithFields(logFields).Debug("Read FSx volumes.")

	return &volumes, nil
}

func (d *Client) GetVolumeByName(ctx context.Context, name string) (*Volume, error) {
	logFields := LogFields{
		"API":        "DescribeVolumesPaginator.NextPage",
		"volumeName": name,
	}

	input := &fsx.DescribeVolumesInput{
		Filters: []fsxtypes.VolumeFilter{
			{
				Name:   fsxtypes.VolumeFilterNameFileSystemId,
				Values: []string{d.config.FilesystemID},
			},
		},
	}

	paginator := fsx.NewDescribeVolumesPaginator(d.fsxClient, input,
		func(o *fsx.DescribeVolumesPaginatorOptions) { o.Limit = PaginationLimit })

	matchingVolumes := make([]fsxtypes.Volume, 0)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			logFields["requestID"] = GetRequestIDFromError(err)
			Logc(ctx).WithFields(logFields).WithError(err).Error("Could not iterate volumes.")
			return nil, fmt.Errorf("error iterating volumes; %w", err)
		}

		logFields["requestID"], _ = middleware.GetRequestIDMetadata(output.ResultMetadata)

		for _, v := range output.Volumes {
			if DerefString(v.Name) == name {
				matchingVolumes = append(matchingVolumes, v)
			}
		}
	}

	if len(matchingVolumes) == 0 {
		return nil, errors.NotFoundError("volume with name %s not found", name)
	} else if len(matchingVolumes) > 1 {
		return nil, fmt.Errorf("multiple volumes with name %s found", name)
	}

	Logc(ctx).WithFields(logFields).Debug("Found FSx volume by name.")

	return d.getVolumeFromFSxVolume(matchingVolumes[0]), nil
}

func (d *Client) GetVolumeByARN(ctx context.Context, volumeARN string) (*Volume, error) {
	_, _, _, volumeID, err := ParseVolumeARN(volumeARN)
	if err != nil {
		return nil, err
	}

	return d.GetVolumeByID(ctx, volumeID)
}

func (d *Client) GetVolumeByID(ctx context.Context, volumeID string) (*Volume, error) {
	logFields := LogFields{
		"API":      "DescribeVolumes",
		"volumeID": volumeID,
	}

	input := &fsx.DescribeVolumesInput{
		Filters: []fsxtypes.VolumeFilter{
			{
				Name:   fsxtypes.VolumeFilterNameFileSystemId,
				Values: []string{d.config.FilesystemID},
			},
		},
		VolumeIds: []string{volumeID},
	}

	output, err := d.fsxClient.DescribeVolumes(ctx, input)
	if err != nil {
		logFields["requestID"] = GetRequestIDFromError(err)
		Logc(ctx).WithFields(logFields).WithError(err).Error("Could not iterate volumes.")
		return nil, fmt.Errorf("error iterating volumes; %w", err)
	}
	if len(output.Volumes) == 0 {
		return nil, errors.NotFoundError(fmt.Sprintf("volume %s not found", volumeID))
	}
	if len(output.Volumes) > 1 {
		return nil, errors.NotFoundError(fmt.Sprintf("multiple volumes with ID '%s' found", volumeID))
	}

	logFields["requestID"], _ = middleware.GetRequestIDMetadata(output.ResultMetadata)
	Logc(ctx).WithFields(logFields).Debug("Found FSx volume by ID.")

	return d.getVolumeFromFSxVolume(output.Volumes[0]), nil
}

// GetVolume uses a volume config record to look for a Filesystem by the most efficient means.
func (d *Client) GetVolume(ctx context.Context, volConfig *storage.VolumeConfig) (*Volume, error) {
	// When we know the internal ID, use that as it is vastly more efficient
	if volConfig.InternalID != "" {
		return d.GetVolumeByARN(ctx, volConfig.InternalID)
	}

	// Fall back to the name
	return d.GetVolumeByName(ctx, volConfig.InternalName)
}

func (d *Client) getVolumeFromFSxVolume(v fsxtypes.Volume) *Volume {
	volume := &Volume{
		FSxObject: FSxObject{
			ARN:  DerefString(v.ResourceARN),
			ID:   DerefString(v.VolumeId),
			Name: DerefString(v.Name),
		},
		Created:      DerefTime(v.CreationTime),
		FilesystemID: DerefString(v.FileSystemId),
		Labels:       make(map[string]string),
		State:        string(v.Lifecycle),
	}

	if v.OntapConfiguration != nil {
		volume.JunctionPath = DerefString(v.OntapConfiguration.JunctionPath)
		volume.SecurityStyle = string(v.OntapConfiguration.SecurityStyle)
		volume.Size = uint64(DerefInt32(v.OntapConfiguration.SizeInMegabytes)) * 1048576
		volume.SnapshotPolicy = DerefString(v.OntapConfiguration.SnapshotPolicy)
		volume.SVMID = DerefString(v.OntapConfiguration.StorageVirtualMachineId)
		volume.UUID = DerefString(v.OntapConfiguration.UUID)
	}

	for _, tag := range v.Tags {
		volume.Labels[DerefString(tag.Key)] = DerefString(tag.Value)
	}

	return volume
}

func (d *Client) VolumeExistsByName(ctx context.Context, name string) (bool, *Volume, error) {
	volume, err := d.GetVolumeByName(ctx, name)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, volume, nil
}

func (d *Client) VolumeExistsByARN(ctx context.Context, volumeARN string) (bool, *Volume, error) {
	_, _, _, volumeID, err := ParseVolumeARN(volumeARN)
	if err != nil {
		return false, nil, err
	}

	return d.VolumeExistsByID(ctx, volumeID)
}

func (d *Client) VolumeExistsByID(ctx context.Context, volumeID string) (bool, *Volume, error) {
	volume, err := d.GetVolumeByID(ctx, volumeID)
	if err != nil {
		if IsVolumeNotFoundError(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, volume, nil
}

// VolumeExists uses a volume config record to look for a Filesystem by the most efficient means.
func (d *Client) VolumeExists(ctx context.Context, volConfig *storage.VolumeConfig) (bool, *Volume, error) {
	// When we know the internal ID, use that as it is vastly more efficient
	if volConfig.InternalID != "" {
		return d.VolumeExistsByARN(ctx, volConfig.InternalID)
	}

	// Fall back to the name
	return d.VolumeExistsByName(ctx, volConfig.InternalName)
}

func (d *Client) WaitForVolumeStates(
	ctx context.Context, volume *Volume, desiredStates, abortStates []string, maxElapsedTime time.Duration,
) (string, error) {
	volumeState := ""

	checkVolumeState := func() error {
		v, err := d.GetVolumeByID(ctx, volume.ID)
		if err != nil {

			// There is no 'Deleted' state in FSx -- the volume just vanishes.  If we failed to query
			// the volume info, and we're trying to transition to StateDeleted, and we get back a 404,
			// then return success.  Otherwise, log the error as usual.
			if collection.ContainsString(desiredStates, StateDeleted) && IsVolumeNotFoundError(err) {
				Logc(ctx).Debugf("Implied deletion for volume %s.", volume.Name)
				volumeState = StateDeleted
				return nil
			}

			volumeState = ""
			return fmt.Errorf("could not get volume status; %w", err)
		}

		volumeState = v.State

		if collection.ContainsString(desiredStates, volumeState) {
			return nil
		}

		err = fmt.Errorf("volume state is %s, not any of %s", v.State, desiredStates)

		// Return a permanent error to stop retrying if we reached one of the abort states
		for _, abortState := range abortStates {
			if volumeState == abortState {
				return backoff.Permanent(TerminalState(err))
			}
		}

		// Override error for a deleting volume
		if collection.ContainsString(desiredStates, StateDeleted) && volumeState == StateDeleting {
			err = errors.VolumeDeletingError(err.Error())
		}

		return err
	}
	stateNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"increment": duration,
			"message":   err.Error(),
		}).Debugf("Waiting for volume state.")
	}
	stateBackoff := backoff.NewExponentialBackOff()
	stateBackoff.MaxElapsedTime = maxElapsedTime
	stateBackoff.MaxInterval = 5 * time.Second
	stateBackoff.RandomizationFactor = 0.1
	stateBackoff.InitialInterval = backoff.DefaultInitialInterval
	stateBackoff.Multiplier = 1.414

	Logc(ctx).WithField("desiredStates", desiredStates).Info("Waiting for volume state.")

	if err := backoff.RetryNotify(checkVolumeState, stateBackoff, stateNotify); err != nil {
		if terminalStateErr, ok := err.(*TerminalStateError); ok {
			Logc(ctx).Errorf("Volume reached terminal state; %w", terminalStateErr)
		} else {
			Logc(ctx).Errorf("Volume state was not any of %s after %3.2f seconds.",
				desiredStates, stateBackoff.MaxElapsedTime.Seconds())
		}
		return volumeState, err
	}

	Logc(ctx).WithField("desiredStates", desiredStates).Debug("Desired volume state reached.")

	return volumeState, nil
}

func (d *Client) CreateVolume(ctx context.Context, request *VolumeCreateRequest) (*Volume, error) {
	logFields := LogFields{
		"API":        "CreateVolume",
		"volumeName": request.Name,
	}

	ontapConfig := &fsxtypes.CreateOntapVolumeConfiguration{
		SizeInMegabytes:          convert.ToPtr(int32(request.SizeBytes / 1048576)),
		StorageVirtualMachineId:  convert.ToPtr(request.SVMID),
		JunctionPath:             convert.ToPtr("/" + request.Name),
		SecurityStyle:            fsxtypes.SecurityStyleUnix,
		SnapshotPolicy:           convert.ToPtr(request.SnapshotPolicy),
		StorageEfficiencyEnabled: convert.ToPtr(true),
		TieringPolicy: &fsxtypes.TieringPolicy{
			Name: fsxtypes.TieringPolicyNameNone,
		},
	}

	tags := make([]fsxtypes.Tag, 0)
	for k, v := range request.Labels {
		tags = append(tags, fsxtypes.Tag{
			Key:   convert.ToPtr(k),
			Value: convert.ToPtr(v),
		})
	}

	input := &fsx.CreateVolumeInput{
		Name:               convert.ToPtr(request.Name),
		VolumeType:         fsxtypes.VolumeTypeOntap,
		OntapConfiguration: ontapConfig,
		Tags:               tags,
	}

	output, err := d.fsxClient.CreateVolume(ctx, input)
	if err != nil {
		logFields["requestID"] = GetRequestIDFromError(err)
		Logc(ctx).WithFields(logFields).WithError(err).Error("Could not create volume.")
		return nil, fmt.Errorf("error creating volume; %w", err)
	}

	newVolume := d.getVolumeFromFSxVolume(*output.Volume)

	logFields["requestID"], _ = middleware.GetRequestIDMetadata(output.ResultMetadata)
	logFields["volumeID"] = newVolume.ID
	Logc(ctx).WithFields(logFields).Info("Volume create request issued.")

	return newVolume, nil
}

func (d *Client) RelabelVolume(ctx context.Context, volume *Volume, labels map[string]string) error {
	logFields := LogFields{
		"API":        "TagResource",
		"volumeID":   volume.ID,
		"volumeName": volume.Name,
	}

	tags := make([]fsxtypes.Tag, 0)
	for k, v := range labels {
		tags = append(tags, fsxtypes.Tag{
			Key:   convert.ToPtr(k),
			Value: convert.ToPtr(v),
		})
	}

	input := &fsx.TagResourceInput{
		ResourceARN: convert.ToPtr(volume.ARN),
		Tags:        tags,
	}

	output, err := d.fsxClient.TagResource(ctx, input)
	if err != nil {
		logFields["requestID"] = GetRequestIDFromError(err)
		Logc(ctx).WithFields(logFields).WithError(err).Error("Could not relabel volume.")
		return fmt.Errorf("error relabeling volume; %w", err)
	}

	logFields["requestID"], _ = middleware.GetRequestIDMetadata(output.ResultMetadata)
	Logc(ctx).WithFields(logFields).Info("Volume relabeled.")

	return nil
}

func (d *Client) ResizeVolume(ctx context.Context, volume *Volume, newSizeBytes uint64) (*Volume, error) {
	logFields := LogFields{
		"API":        "UpdateVolume",
		"volumeID":   volume.ID,
		"volumeName": volume.Name,
	}

	ontapConfig := &fsxtypes.UpdateOntapVolumeConfiguration{
		SizeInMegabytes: convert.ToPtr(int32(newSizeBytes / 1048576)),
	}

	input := &fsx.UpdateVolumeInput{
		VolumeId:           convert.ToPtr(volume.ID),
		OntapConfiguration: ontapConfig,
	}

	output, err := d.fsxClient.UpdateVolume(ctx, input)
	if err != nil {
		logFields["requestID"] = GetRequestIDFromError(err)
		Logc(ctx).WithFields(logFields).WithError(err).Error("Could not resize volume.")
		return nil, fmt.Errorf("error resizing volume; %w", err)
	}

	logFields["newSize"] = newSizeBytes
	logFields["requestID"], _ = middleware.GetRequestIDMetadata(output.ResultMetadata)
	Logc(ctx).WithFields(logFields).Info("Volume resized.")

	return d.getVolumeFromFSxVolume(*output.Volume), nil
}

func (d *Client) DeleteVolume(ctx context.Context, volume *Volume) error {
	logFields := LogFields{
		"API":        "DeleteVolume",
		"volumeID":   volume.ID,
		"volumeName": volume.Name,
	}

	ontapConfig := &fsxtypes.DeleteVolumeOntapConfiguration{
		SkipFinalBackup: convert.ToPtr(true),
	}

	input := &fsx.DeleteVolumeInput{
		VolumeId:           convert.ToPtr(volume.ID),
		OntapConfiguration: ontapConfig,
	}

	_, err := d.fsxClient.DeleteVolume(ctx, input)
	if err != nil {
		logFields["requestID"] = GetRequestIDFromError(err)
		Logc(ctx).WithFields(logFields).WithError(err).Error("Could not delete volume.")
		return fmt.Errorf("error deleting volume; %w", err)
	}

	Logc(ctx).WithFields(logFields).Info("Volume deleted.")

	return nil
}

// ///////////////////////////////////////////////////////////////////////////////
// Miscellaneous utility functions and error types
// ///////////////////////////////////////////////////////////////////////////////

// IsVolumeNotFoundError checks whether an error returned from the AWS SDK contains a VolumeNotFound error.
func IsVolumeNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var vnf *fsxtypes.VolumeNotFound
	return errors.As(err, &vnf)
}

func GetRequestIDFromError(err error) (requestID string) {
	if err != nil {
		var re *awshttp.ResponseError
		if errors.As(err, &re) {
			requestID = re.ServiceRequestID()
		}
	}
	return
}

// DerefString accepts a string pointer and returns the value of the string, or "" if the pointer is nil.
func DerefString(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}

func DerefInt32(i *int32) int32 {
	if i != nil {
		return *i
	}
	return 0
}

// DerefTime accepts a time pointer and returns the value of the timestamp, or nil.
func DerefTime(t *time.Time) time.Time {
	if t != nil {
		return *t
	}
	return time.Time{}
}

// TerminalStateError signals that the object is in a terminal state.  This is used to stop waiting on
// an object to change state.
type TerminalStateError struct {
	Err error
}

func (e *TerminalStateError) Error() string {
	return e.Err.Error()
}

// TerminalState wraps the given err in a *TerminalStateError.
func TerminalState(err error) *TerminalStateError {
	return &TerminalStateError{
		Err: err,
	}
}
