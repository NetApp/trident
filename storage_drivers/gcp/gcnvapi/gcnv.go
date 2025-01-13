// Copyright 2025 NetApp, Inc. All Rights Reserved.

// Package gcnvapi provides a high-level interface to the Google Cloud NetApp Volumes SDK
package gcnvapi

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"time"

	netapp "cloud.google.com/go/netapp/apiv1"
	"cloud.google.com/go/netapp/apiv1/netapppb"
	"github.com/cenkalti/backoff/v4"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/storage"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/utils/errors"
)

const (
	VolumeCreateTimeout = 10 * time.Second
	SnapshotTimeout     = 240 * time.Second // Snapshotter sidecar has a timeout of 5 minutes.  Stay under that!
	DefaultTimeout      = 120 * time.Second
	MaxLabelLength      = 63
	MaxLabelCount       = 64
	DefaultSDKTimeout   = 30 * time.Second
	PaginationLimit     = 100
)

var (
	capacityPoolNameRegex = regexp.MustCompile(`^projects/(?P<projectNumber>[^/]+)/locations/(?P<location>[^/]+)/storagePools/(?P<capacityPool>[^/]+)$`)
	volumeNameRegex       = regexp.MustCompile(`^projects/(?P<projectNumber>[^/]+)/locations/(?P<location>[^/]+)/volumes/(?P<volume>[^/]+)$`)
	snapshotNameRegex     = regexp.MustCompile(`^projects/(?P<projectNumber>[^/]+)/locations/(?P<location>[^/]+)/volumes/(?P<volume>[^/]+)/snapshots/(?P<snapshot>[^/]+)$`)
	networkNameRegex      = regexp.MustCompile(`^projects/(?P<projectNumber>[^/]+)/global/networks/(?P<network>[^/]+)$`)
)

// ClientConfig holds configuration data for the API driver object.
type ClientConfig struct {
	StorageDriverName string

	// GCP project number
	ProjectNumber string

	// GCP CVS API authentication parameters
	APIKey *drivers.GCPPrivateKey

	// GCP region
	Location string

	// URL for accessing the API via an HTTP/HTTPS proxy
	ProxyURL string

	// Options
	DebugTraceFlags map[string]bool
	SDKTimeout      time.Duration // Timeout applied to all calls to the GCNV SDK
	MaxCacheAge     time.Duration // The oldest data we should expect in the cached resources
}

type GCNVClient struct {
	gcnv *netapp.Client
	GCNVResources
}

// Client encapsulates connection details.
type Client struct {
	config    *ClientConfig
	sdkClient *GCNVClient
}

func createGCNVClient(ctx context.Context, config *ClientConfig) (*netapp.Client, error) {
	// Check if the config is empty
	if reflect.ValueOf(*config.APIKey).IsZero() {
		credentials, err := google.FindDefaultCredentials(ctx)
		if err != nil {
			return nil, err
		}
		return netapp.NewClient(ctx, option.WithCredentials(credentials))
	} else if config.APIKey != nil {
		keyBytes, jsonErr := json.Marshal(config.APIKey)
		if jsonErr != nil {
			return nil, jsonErr
		}
		creds, credsErr := google.CredentialsFromJSON(ctx, keyBytes, netapp.DefaultAuthScopes()...)
		if credsErr != nil {
			return nil, credsErr
		}
		return netapp.NewClient(ctx, option.WithCredentials(creds))
	} else {
		return nil, errors.New("apiKey in config must be specified")
	}
}

// NewDriver is a factory method for creating a new SDK interface.
func NewDriver(ctx context.Context, config *ClientConfig) (GCNV, error) {
	gcnvClient, err := createGCNVClient(ctx, config)
	if err != nil {
		return nil, err
	}

	return Client{
		config: config,
		sdkClient: &GCNVClient{
			gcnv: gcnvClient,
		},
	}, nil
}

// Init runs startup logic after allocating the driver resources.
func (c Client) Init(ctx context.Context, pools map[string]storage.Pool) error {
	// Map vpools to backend
	c.registerStoragePools(pools)

	// Find out what we have to work with in GCNV
	return c.RefreshGCNVResources(ctx)
}

// RegisterStoragePool makes a note of pools defined by the driver for later mapping.
func (c Client) registerStoragePools(sPools map[string]storage.Pool) {
	c.sdkClient.GCNVResources.StoragePoolMap = make(map[string]storage.Pool)

	for _, sPool := range sPools {
		c.sdkClient.GCNVResources.StoragePoolMap[sPool.Name()] = sPool
	}
}

// ///////////////////////////////////////////////////////////////////////////////
// Functions to create & parse GCNV resource names
// ///////////////////////////////////////////////////////////////////////////////

// createBaseID creates the base GCNV-style ID for a project & location.
func (c Client) createBaseID() string {
	return fmt.Sprintf("projects/%s/locations/%s", c.config.ProjectNumber, c.config.Location)
}

// createCapacityPoolID creates the GCNV-style ID for a capacity pool.
func (c Client) createCapacityPoolID(capacityPool string) string {
	return fmt.Sprintf("projects/%s/locations/%s/storagePools/%s",
		c.config.ProjectNumber, c.config.Location, capacityPool)
}

// parseCapacityPoolID parses the GCNV-style full name for a capacity pool.
func parseCapacityPoolID(fullName string) (projectNumber, location, capacityPool string, err error) {
	match := capacityPoolNameRegex.FindStringSubmatch(fullName)

	if match == nil {
		err = fmt.Errorf("capacity pool name %s is invalid", fullName)
		return
	}

	paramsMap := make(map[string]string)
	for i, name := range capacityPoolNameRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	projectNumber = paramsMap["projectNumber"]
	location = paramsMap["location"]
	capacityPool = paramsMap["capacityPool"]

	return
}

// createVolumeID creates the GCNV-style ID for a volume.
func (c Client) createVolumeID(volume string) string {
	return fmt.Sprintf("projects/%s/locations/%s/volumes/%s", c.config.ProjectNumber, c.config.Location, volume)
}

// parseVolumeID parses the GCNV-style full name for a volume.
func parseVolumeID(fullName string) (projectNumber, location, volume string, err error) {
	match := volumeNameRegex.FindStringSubmatch(fullName)

	if match == nil {
		err = fmt.Errorf("volume name %s is invalid", fullName)
		return
	}

	paramsMap := make(map[string]string)
	for i, name := range volumeNameRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	projectNumber = paramsMap["projectNumber"]
	location = paramsMap["location"]
	volume = paramsMap["volume"]

	return
}

// createSnapshotID creates the GCNV-style ID for a snapshot.
func (c Client) createSnapshotID(volume, snapshot string) string {
	return fmt.Sprintf("projects/%s/locations/%s/volumes/%s/snapshots/%s",
		c.config.ProjectNumber, c.config.Location, volume, snapshot)
}

// parseSnapshotID parses the GCNV-style full name for a snapshot.
func parseSnapshotID(fullName string) (projectNumber, location, volume, snapshot string, err error) {
	match := snapshotNameRegex.FindStringSubmatch(fullName)

	if match == nil {
		err = fmt.Errorf("snapshot name %s is invalid", fullName)
		return
	}

	paramsMap := make(map[string]string)
	for i, name := range snapshotNameRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	projectNumber = paramsMap["projectNumber"]
	location = paramsMap["location"]
	volume = paramsMap["volume"]
	snapshot = paramsMap["snapshot"]

	return
}

// createNetworkID creates the GCNV-style ID for a network.
func (c Client) createNetworkID(network string) string {
	return fmt.Sprintf("projects/%s/global/networks/%s", c.config.ProjectNumber, network)
}

// parseNetworkID parses the GCNV-style full name for a network.
func parseNetworkID(fullName string) (projectNumber, network string, err error) {
	match := networkNameRegex.FindStringSubmatch(fullName)

	if match == nil {
		err = fmt.Errorf("network name %s is invalid", fullName)
		return
	}

	paramsMap := make(map[string]string)
	for i, name := range networkNameRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	projectNumber = paramsMap["projectNumber"]
	network = paramsMap["network"]

	return
}

// ///////////////////////////////////////////////////////////////////////////////
// Functions to convert between GCNV SDK & internal volume structs
// ///////////////////////////////////////////////////////////////////////////////

// exportPolicyExport turns an internal ExportPolicy into something consumable by the SDK.
func exportPolicyExport(exportPolicy *ExportPolicy) *netapppb.ExportPolicy {
	gcnvRules := make([]*netapppb.SimpleExportPolicyRule, 0)

	for _, rule := range exportPolicy.Rules {

		allowedClients := rule.AllowedClients
		accessType := GCNVAccessTypeFromVolumeAccessType(rule.AccessType)
		nfsv3 := rule.Nfsv3
		nfsv4 := rule.Nfsv4

		gcnvRule := netapppb.SimpleExportPolicyRule{
			AllowedClients: &allowedClients,
			AccessType:     &accessType,
			Nfsv3:          &nfsv3,
			Nfsv4:          &nfsv4,
		}

		gcnvRules = append(gcnvRules, &gcnvRule)
	}

	return &netapppb.ExportPolicy{
		Rules: gcnvRules,
	}
}

// exportPolicyImport turns an SDK ExportPolicy into an internal one.
func (c Client) exportPolicyImport(gcnvExportPolicy *netapppb.ExportPolicy) *ExportPolicy {
	rules := make([]ExportRule, 0)

	if gcnvExportPolicy == nil || len(gcnvExportPolicy.Rules) == 0 {
		return &ExportPolicy{Rules: rules}
	}

	for index, gcnvRule := range gcnvExportPolicy.Rules {
		rules = append(rules, ExportRule{
			AllowedClients: DerefString(gcnvRule.AllowedClients),
			Nfsv3:          DerefBool(gcnvRule.Nfsv3),
			Nfsv4:          DerefBool(gcnvRule.Nfsv4),
			RuleIndex:      int32(index),
			AccessType:     VolumeAccessTypeFromGCNVAccessType(DerefAccessType(gcnvRule.AccessType)),
		})
	}

	return &ExportPolicy{Rules: rules}
}

// newVolumeFromGCNVVolume creates a new internal Volume struct from a GCNV volume.
func (c Client) newVolumeFromGCNVVolume(ctx context.Context, volume *netapppb.Volume) (*Volume, error) {
	if volume == nil {
		return nil, errors.New("nil volume")
	}

	_, location, volumeName, err := parseVolumeID(volume.Name)
	if err != nil {
		return nil, err
	}

	_, network, err := parseNetworkID(volume.Network)
	if err != nil {
		return nil, err
	}

	var protocolTypes []string
	for _, gcnvProtocolType := range volume.Protocols {
		protocolTypes = append(protocolTypes, VolumeProtocolFromGCNVProtocol(gcnvProtocolType))
	}

	return &Volume{
		Name:              volumeName,
		CreationToken:     volume.ShareName,
		FullName:          volume.Name,
		Location:          location,
		State:             VolumeStateFromGCNVState(volume.State),
		CapacityPool:      volume.StoragePool,
		NetworkName:       network,
		NetworkFullName:   volume.Network,
		ServiceLevel:      ServiceLevelFromCapacityPool(c.capacityPool(volume.StoragePool)),
		SizeBytes:         volume.CapacityGib * int64(1073741824),
		ExportPolicy:      c.exportPolicyImport(volume.ExportPolicy),
		ProtocolTypes:     protocolTypes,
		MountTargets:      c.getMountTargetsFromVolume(ctx, volume),
		UnixPermissions:   volume.UnixPermissions,
		Labels:            volume.Labels,
		SnapshotReserve:   int64(volume.SnapReserve),
		SnapshotDirectory: volume.SnapshotDirectory,
		SecurityStyle:     VolumeSecurityStyleFromGCNVSecurityStyle(volume.SecurityStyle),
	}, nil
}

// getMountTargetsFromVolume extracts the mount targets from a GCNV volume.
func (c Client) getMountTargetsFromVolume(ctx context.Context, volume *netapppb.Volume) []MountTarget {
	mounts := make([]MountTarget, 0)

	if len(volume.MountOptions) == 0 {
		Logc(ctx).Tracef("Volume %s has no mount targets.", volume.Name)
		return mounts
	}

	for _, gcnvMountTarget := range volume.MountOptions {
		mounts = append(mounts, MountTarget{
			Export:     gcnvMountTarget.Export,
			ExportPath: gcnvMountTarget.ExportFull,
			Protocol:   VolumeProtocolFromGCNVProtocol(gcnvMountTarget.Protocol),
		})
	}

	return mounts
}

// ///////////////////////////////////////////////////////////////////////////////
// Functions to retrieve and manage volumes
// ///////////////////////////////////////////////////////////////////////////////

// Volumes queries GCNV SDK for all volumes in the current location.
func (c Client) Volumes(ctx context.Context) (*[]*Volume, error) {
	logFields := LogFields{
		"API": "GCNV.ListVolumes",
	}

	var volumes []*Volume

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()
	req := &netapppb.ListVolumesRequest{
		Parent:   c.createBaseID(),
		PageSize: PaginationLimit,
	}
	it := c.sdkClient.gcnv.ListVolumes(sdkCtx, req)
	for {
		gcnvVolume, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["api"]).
				WithFields(logFields).WithError(err).Error("Could not read volumes.")
			return nil, err
		}
		Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["api"]).
			WithFields(logFields).WithError(err).Debug("Volume: %v.", gcnvVolume)

		volume, err := c.newVolumeFromGCNVVolume(ctx, gcnvVolume)
		if err != nil {
			Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["api"]).
				WithError(err).Warning("Skipping volume.")
			continue
		}
		volumes = append(volumes, volume)
	}

	return &volumes, nil
}

// Volume uses a volume config record to fetch a volume by the most efficient means.
func (c Client) Volume(ctx context.Context, volConfig *storage.VolumeConfig) (*Volume, error) {
	return c.VolumeByName(ctx, volConfig.InternalName)
}

func (c Client) VolumeByName(ctx context.Context, name string) (*Volume, error) {
	logFields := LogFields{
		"API":    "GCNV.GetVolume",
		"volume": name,
	}

	Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["api"]).
		WithFields(logFields).Trace("Fetching volume by name.")

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()
	req := &netapppb.GetVolumeRequest{
		Name: c.createVolumeID(name),
	}
	gcnvVolume, err := c.sdkClient.gcnv.GetVolume(sdkCtx, req)
	if err != nil {
		if IsGCNVNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Debug("Volume not found.")
			return nil, errors.WrapWithNotFoundError(err, "volume '%s' not found", name)
		}

		Logc(ctx).WithFields(logFields).WithError(err).Error("Error fetching volume.")
		return nil, err
	}

	Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["api"]).
		WithFields(logFields).Debug("Found volume by name.")

	return c.newVolumeFromGCNVVolume(ctx, gcnvVolume)
}

// VolumeExists uses a volume config record to look for a Filesystem by the most efficient means.
func (c Client) VolumeExists(ctx context.Context, volConfig *storage.VolumeConfig) (bool, *Volume, error) {
	volume, err := c.Volume(ctx, volConfig)
	if err != nil {
		if IsGCNVNotFoundError(err) {
			return false, nil, nil
		} else {
			return false, nil, err
		}
	}
	return true, volume, nil
}

// WaitForVolumeState watches for a desired volume state and returns when that state is achieved.
func (c Client) WaitForVolumeState(
	ctx context.Context, volume *Volume, desiredState string, abortStates []string,
	maxElapsedTime time.Duration,
) (string, error) {
	volumeState := ""

	checkVolumeState := func() error {
		v, err := c.VolumeByName(ctx, volume.Name)
		if err != nil {

			// There is no 'Deleted' state in GCNV -- the volume just vanishes.  If we failed to query
			// the volume info, and we're trying to transition to StateDeleted, and we get back a 404,
			// then return success.  Otherwise, log the error as usual.
			if desiredState == VolumeStateDeleted && errors.IsNotFoundError(err) {
				Logc(ctx).Debugf("Implied deletion for volume %s.", volume.Name)
				volumeState = VolumeStateDeleted
				return nil
			}
			if errors.Is(err, context.Canceled) {
				return backoff.Permanent(err)
			}
			volumeState = ""
			return fmt.Errorf("could not get volume status; %v", err)
		}

		volumeState = v.State

		if v.State == desiredState {
			return nil
		}

		errMsg := fmt.Sprintf("volume state is %s, not %s", v.State, desiredState)
		if desiredState == VolumeStateDeleted && v.State == VolumeStateDeleting {
			err = errors.VolumeDeletingError(errMsg)
		} else {
			err = errors.New(errMsg)
		}

		// Return a permanent error to stop retrying if we reached one of the abort states
		if collection.ContainsString(abortStates, v.State) {
			return backoff.Permanent(TerminalState(err))
		}

		return err
	}

	stateNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
		}).Debugf("Waiting for volume state.")
	}

	stateBackoff := backoff.NewExponentialBackOff()
	stateBackoff.MaxElapsedTime = maxElapsedTime
	stateBackoff.MaxInterval = 5 * time.Second
	stateBackoff.RandomizationFactor = 0.1
	stateBackoff.InitialInterval = 3 * time.Second
	stateBackoff.Multiplier = 1.414

	Logc(ctx).WithField("desiredState", desiredState).Info("Waiting for volume state.")

	if err := backoff.RetryNotify(checkVolumeState, stateBackoff, stateNotify); err != nil {
		if IsTerminalStateError(err) {
			Logc(ctx).WithError(err).Error("Volume reached terminal state.")
		} else {
			Logc(ctx).Warningf("Volume state was not %s after %3.2f seconds.",
				desiredState, stateBackoff.MaxElapsedTime.Seconds())
		}
		return volumeState, err
	}

	Logc(ctx).WithField("desiredState", desiredState).Debug("Desired volume state reached.")

	return volumeState, nil
}

// CreateVolume creates a new volume.
func (c Client) CreateVolume(ctx context.Context, request *VolumeCreateRequest) (*Volume, error) {
	var protocols []netapppb.Protocols
	for _, protocolType := range request.ProtocolTypes {
		protocols = append(protocols, GCNVProtocolFromVolumeProtocol(protocolType))
	}

	cPool := c.capacityPool(request.CapacityPool)
	if cPool == nil {
		return nil, fmt.Errorf("pool %s not found", request.CapacityPool)
	}

	newVol := &netapppb.Volume{
		Name:              request.Name,
		ShareName:         request.CreationToken,
		StoragePool:       request.CapacityPool,
		CapacityGib:       request.SizeBytes / 1073741824,
		Protocols:         protocols,
		UnixPermissions:   request.UnixPermissions,
		Labels:            request.Labels,
		SnapshotDirectory: request.SnapshotDirectory,
		SecurityStyle:     GCNVSecurityStyleFromVolumeSecurityStyle(request.SecurityStyle),
	}

	if request.ExportPolicy != nil {
		newVol.ExportPolicy = exportPolicyExport(request.ExportPolicy)
	}
	if request.SnapshotReserve != nil {
		newVol.SnapReserve = float64(*request.SnapshotReserve)
	}

	// Only set the snapshot ID if we are cloning
	if request.SnapshotID != "" {
		newVol.RestoreParameters = &netapppb.RestoreParameters{
			Source: &netapppb.RestoreParameters_SourceSnapshot{
				SourceSnapshot: request.SnapshotID,
			},
		}
	}

	Logc(ctx).WithFields(LogFields{
		"name":          request.Name,
		"creationToken": request.CreationToken,
		"capacityPool":  request.CapacityPool,
	}).Debug("Issuing create request.")

	logFields := LogFields{
		"API":    "GCNV.CreateVolume",
		"volume": request.Name,
	}

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()
	req := &netapppb.CreateVolumeRequest{
		Parent:   c.createBaseID(),
		VolumeId: request.Name,
		Volume:   newVol,
	}
	poller, err := c.sdkClient.gcnv.CreateVolume(sdkCtx, req)
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error creating volume.")
		return nil, err
	}

	Logc(ctx).WithFields(logFields).Info("Volume create request issued.")

	if _, pollErr := poller.Poll(sdkCtx); pollErr != nil {
		return nil, pollErr
	} else {
		// The volume doesn't exist yet, so forge the name & network IDs to enable conversion to a Volume struct
		newVol.Name = c.createVolumeID(request.Name)
		newVol.Network = cPool.NetworkFullName
		return c.newVolumeFromGCNVVolume(ctx, newVol)
	}
}

// ModifyVolume updates attributes of a volume.
func (c Client) ModifyVolume(
	ctx context.Context, volume *Volume, labels map[string]string, unixPermissions *string,
	snapshotDirAccess *bool, _ *ExportRule,
) error {
	logFields := LogFields{
		"API":    "GCNV.UpdateVolume",
		"volume": volume.Name,
	}

	newVolume := &netapppb.Volume{
		Name:   volume.FullName,
		Labels: labels,
	}
	updateMask := &fieldmaskpb.FieldMask{
		Paths: []string{"labels"},
	}

	if unixPermissions != nil {
		newVolume.UnixPermissions = *unixPermissions
		updateMask.Paths = append(updateMask.Paths, "unix_permissions")
	}

	if snapshotDirAccess != nil {
		newVolume.SnapshotDirectory = *snapshotDirAccess
		updateMask.Paths = append(updateMask.Paths, "snapshot_directory")
	}

	Logc(ctx).WithFields(logFields).Debug("Modifying volume.")

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()
	req := &netapppb.UpdateVolumeRequest{
		Volume:     newVolume,
		UpdateMask: updateMask,
	}
	poller, err := c.sdkClient.gcnv.UpdateVolume(sdkCtx, req)
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error modifying volume.")
		return err
	}

	Logc(ctx).WithFields(logFields).Info("Volume modify request issued.")

	waitCtx, waitCancel := context.WithTimeout(ctx, DefaultTimeout)
	defer waitCancel()
	if _, pollErr := poller.Wait(waitCtx); pollErr != nil {
		Logc(ctx).WithFields(logFields).WithError(pollErr).Error("Error polling for volume modify result.")
		return pollErr
	}

	Logc(ctx).WithFields(logFields).Debug("Volume modified.")

	return nil
}

// ResizeVolume sends a VolumePatch to update a volume's quota.
func (c Client) ResizeVolume(ctx context.Context, volume *Volume, newSizeBytes int64) error {
	logFields := LogFields{
		"API":    "GCNV.UpdateVolume",
		"volume": volume.Name,
	}

	newVolume := &netapppb.Volume{
		Name:        volume.FullName,
		CapacityGib: newSizeBytes / 1073741824,
	}
	updateMask := &fieldmaskpb.FieldMask{
		Paths: []string{"capacity_gib"},
	}

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()
	req := &netapppb.UpdateVolumeRequest{
		Volume:     newVolume,
		UpdateMask: updateMask,
	}
	poller, err := c.sdkClient.gcnv.UpdateVolume(sdkCtx, req)
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error resizing volume.")
		return err
	}

	Logc(ctx).WithFields(logFields).Info("Volume resize request issued.")

	waitCtx, waitCancel := context.WithTimeout(ctx, DefaultTimeout)
	defer waitCancel()
	if _, pollErr := poller.Wait(waitCtx); pollErr != nil {
		Logc(ctx).WithFields(logFields).WithError(pollErr).Error("Error polling for volume resize result.")
		return pollErr
	}

	Logc(ctx).WithFields(logFields).Debug("Volume resize complete.")

	return nil
}

// DeleteVolume deletes a volume.
func (c Client) DeleteVolume(ctx context.Context, volume *Volume) error {
	name := volume.Name
	logFields := LogFields{
		"API":    "GCNV.DeleteVolume",
		"volume": name,
	}

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()
	req := &netapppb.DeleteVolumeRequest{
		Name:  c.createVolumeID(name),
		Force: true,
	}
	_, err := c.sdkClient.gcnv.DeleteVolume(sdkCtx, req)
	if err != nil {
		if IsGCNVNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Info("Volume already deleted.")
			return nil
		}

		Logc(ctx).WithFields(logFields).WithError(err).Error("Error deleting volume.")
		return err
	}

	Logc(ctx).WithFields(logFields).Debug("Volume deleted.")

	return nil
}

// ///////////////////////////////////////////////////////////////////////////////
// Functions to retrieve and manage snapshots
// ///////////////////////////////////////////////////////////////////////////////

// newSnapshotFromGCNVSnapshot creates a new internal Snapshot struct from a GCNV snapshot.
func (c Client) newSnapshotFromGCNVSnapshot(_ context.Context, gcnvSnapshot *netapppb.Snapshot) (*Snapshot, error) {
	_, location, volumeName, snapshotName, err := parseSnapshotID(gcnvSnapshot.Name)
	if err != nil {
		return nil, err
	}

	snapshot := &Snapshot{
		Name:     snapshotName,
		FullName: gcnvSnapshot.Name,
		Volume:   volumeName,
		Location: location,
		State:    SnapshotStateFromGCNVState(gcnvSnapshot.State),
		Labels:   gcnvSnapshot.Labels,
	}

	if gcnvSnapshot.CreateTime != nil {
		snapshot.Created = gcnvSnapshot.CreateTime.AsTime()
	}

	return snapshot, nil
}

// SnapshotsForVolume returns a list of snapshots on a volume.
func (c Client) SnapshotsForVolume(ctx context.Context, volume *Volume) (*[]*Snapshot, error) {
	logFields := LogFields{
		"API":    "GCNV.ListSnapshots",
		"volume": volume.Name,
	}

	var snapshots []*Snapshot

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()
	req := &netapppb.ListSnapshotsRequest{
		Parent:   c.createVolumeID(volume.Name),
		PageSize: PaginationLimit,
	}
	it := c.sdkClient.gcnv.ListSnapshots(sdkCtx, req)
	for {
		gcnvSnapshot, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["api"]).
				WithFields(logFields).WithError(err).Error("Could not read snapshots.")
			return nil, err
		}
		Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["api"]).
			WithFields(logFields).WithError(err).Debug("Snapshot: %v.", gcnvSnapshot)

		snapshot, err := c.newSnapshotFromGCNVSnapshot(ctx, gcnvSnapshot)
		if err != nil {
			Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["api"]).
				WithError(err).Warning("Skipping snapshot.")
			continue
		}
		snapshots = append(snapshots, snapshot)
	}

	Logc(ctx).WithFields(logFields).Debug("Read snapshots from volume.")

	return &snapshots, nil
}

// SnapshotForVolume fetches a specific snapshot on a volume by its name.
func (c Client) SnapshotForVolume(
	ctx context.Context, volume *Volume, snapshotName string,
) (*Snapshot, error) {
	logFields := LogFields{
		"API":      "GCNV.GetSnapshot",
		"volume":   volume.Name,
		"snapshot": snapshotName,
	}

	Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["api"]).
		WithFields(logFields).Trace("Fetching snapshot by name.")

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()
	req := &netapppb.GetSnapshotRequest{
		Name: c.createSnapshotID(volume.Name, snapshotName),
	}
	gcnvSnapshot, err := c.sdkClient.gcnv.GetSnapshot(sdkCtx, req)
	if err != nil {
		if IsGCNVNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Debug("Snapshot not found.")
			return nil, errors.WrapWithNotFoundError(err, "snapshot '%s' not found", snapshotName)
		}

		Logc(ctx).WithFields(logFields).WithError(err).Error("Error fetching snapshot.")
		return nil, err
	}

	Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["api"]).
		WithFields(logFields).Debug("Found snapshot by name.")

	return c.newSnapshotFromGCNVSnapshot(ctx, gcnvSnapshot)
}

// WaitForSnapshotState waits for a desired snapshot state and returns once that state is achieved.
func (c Client) WaitForSnapshotState(
	ctx context.Context, snapshot *Snapshot, volume *Volume, desiredState string, abortStates []string,
	maxElapsedTime time.Duration,
) error {
	checkSnapshotState := func() error {
		s, err := c.SnapshotForVolume(ctx, volume, snapshot.Name)
		if err != nil {

			// There is no 'Deleted' state in GCNV -- the snapshot just vanishes.  If we failed to query
			// the snapshot info, and we're trying to transition to StateDeleted, and we get back a 404,
			// then return success.  Otherwise, log the error as usual.
			if desiredState == SnapshotStateDeleted && errors.IsNotFoundError(err) {
				Logc(ctx).Debugf("Implied deletion for snapshot %s.", snapshot.Name)
				return nil
			}
			if errors.Is(err, context.Canceled) {
				return backoff.Permanent(err)
			}
			return fmt.Errorf("could not get snapshot status; %v", err)
		}

		if s.State == desiredState {
			return nil
		}

		err = fmt.Errorf("snapshot state is %s, not %s", s.State, desiredState)

		// Return a permanent error to stop retrying if we reached one of the abort states
		if collection.ContainsString(abortStates, s.State) {
			return backoff.Permanent(TerminalState(err))
		}

		return err
	}

	stateNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"increment": duration.Truncate(10 * time.Millisecond),
			"message":   err.Error(),
		}).Debugf("Waiting for snapshot state.")
	}

	stateBackoff := backoff.NewExponentialBackOff()
	stateBackoff.MaxElapsedTime = maxElapsedTime
	stateBackoff.MaxInterval = 5 * time.Second
	stateBackoff.RandomizationFactor = 0.1
	stateBackoff.InitialInterval = 3 * time.Second
	stateBackoff.Multiplier = 1.414

	Logc(ctx).WithField("desiredState", desiredState).Info("Waiting for snapshot state.")

	if err := backoff.RetryNotify(checkSnapshotState, stateBackoff, stateNotify); err != nil {
		if IsTerminalStateError(err) {
			Logc(ctx).WithError(err).Error("Snapshot reached terminal state.")
		} else {
			Logc(ctx).Warningf("Snapshot state was not %s after %3.2f seconds.",
				desiredState, stateBackoff.MaxElapsedTime.Seconds())
		}
		return err
	}

	Logc(ctx).WithField("desiredState", desiredState).Debugf("Desired snapshot state reached.")

	return nil
}

// CreateSnapshot creates a new snapshot.
func (c Client) CreateSnapshot(ctx context.Context, volume *Volume, snapshotName string) (*Snapshot, error) {
	newSnapshot := &netapppb.Snapshot{}

	logFields := LogFields{
		"API":      "GCNV.CreateSnapshot",
		"volume":   volume.Name,
		"snapshot": snapshotName,
	}

	Logc(ctx).WithFields(logFields).Debug("Issuing snapshot create request.")

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()
	req := &netapppb.CreateSnapshotRequest{
		Parent:     volume.FullName,
		Snapshot:   newSnapshot,
		SnapshotId: snapshotName,
	}
	poller, err := c.sdkClient.gcnv.CreateSnapshot(sdkCtx, req)
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error creating snapshot.")
		return nil, err
	}

	Logc(ctx).WithFields(logFields).Info("Snapshot create request issued.")

	if _, pollErr := poller.Poll(sdkCtx); pollErr != nil {
		return nil, pollErr
	} else {
		// The snapshot doesn't exist yet, so forge the name ID to enable conversion to a Snapshot struct
		newSnapshot.Name = c.createSnapshotID(volume.Name, snapshotName)
		return c.newSnapshotFromGCNVSnapshot(ctx, newSnapshot)
	}
}

// RestoreSnapshot restores a volume to a snapshot.
func (c Client) RestoreSnapshot(ctx context.Context, volume *Volume, snapshot *Snapshot) error {
	logFields := LogFields{
		"API":      "GCNV.RevertVolume",
		"volume":   volume.Name,
		"snapshot": snapshot.Name,
	}

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()
	req := &netapppb.RevertVolumeRequest{
		Name:       volume.FullName,
		SnapshotId: snapshot.Name,
	}
	poller, err := c.sdkClient.gcnv.RevertVolume(sdkCtx, req)
	if err != nil {
		if IsGCNVNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Info("Volume or snapshot not found.")
			return err
		}

		Logc(ctx).WithFields(logFields).WithError(err).Error("Error reverting volume to snapshot.")
		return err
	}

	waitCtx, waitCancel := context.WithTimeout(ctx, DefaultTimeout)
	defer waitCancel()
	if _, pollErr := poller.Wait(waitCtx); pollErr != nil {
		Logc(ctx).WithFields(logFields).WithError(pollErr).Error("Error polling for volume revert to snapshot result.")
		return pollErr
	}

	Logc(ctx).WithFields(logFields).Debug("Volume reverted to snapshot.")

	return nil
}

// DeleteSnapshot deletes a snapshot.
func (c Client) DeleteSnapshot(ctx context.Context, volume *Volume, snapshot *Snapshot) error {
	logFields := LogFields{
		"API":      "GCNV.DeleteSnapshot",
		"volume":   volume.Name,
		"snapshot": snapshot.Name,
	}

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()
	req := &netapppb.DeleteSnapshotRequest{
		Name: c.createSnapshotID(volume.Name, snapshot.Name),
	}
	_, err := c.sdkClient.gcnv.DeleteSnapshot(sdkCtx, req)
	if err != nil {
		if IsGCNVNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Info("Snapshot already deleted.")
			return nil
		}

		Logc(ctx).WithFields(logFields).WithError(err).Error("Error deleting snapshot.")
		return err
	}

	Logc(ctx).WithFields(logFields).Debug("Snapshot deleted.")

	return nil
}

// ///////////////////////////////////////////////////////////////////////////////
// Miscellaneous utility functions and error types
// ///////////////////////////////////////////////////////////////////////////////

func ServiceLevelFromCapacityPool(capacityPool *CapacityPool) string {
	if capacityPool == nil {
		return ServiceLevelUnspecified
	}
	return capacityPool.ServiceLevel
}

// ServiceLevelFromGCNVServiceLevel converts GCNV service level to string
func ServiceLevelFromGCNVServiceLevel(serviceLevel netapppb.ServiceLevel) string {
	switch serviceLevel {
	default:
		fallthrough
	case netapppb.ServiceLevel_SERVICE_LEVEL_UNSPECIFIED:
		return ServiceLevelUnspecified
	case netapppb.ServiceLevel_FLEX:
		return ServiceLevelFlex
	case netapppb.ServiceLevel_STANDARD:
		return ServiceLevelStandard
	case netapppb.ServiceLevel_PREMIUM:
		return ServiceLevelPremium
	case netapppb.ServiceLevel_EXTREME:
		return ServiceLevelExtreme
	}
}

// StoragePoolStateFromGCNVState converts GCNV storage pool state to string
func StoragePoolStateFromGCNVState(state netapppb.StoragePool_State) string {
	switch state {
	default:
		fallthrough
	case netapppb.StoragePool_STATE_UNSPECIFIED:
		return StoragePoolStateUnspecified
	case netapppb.StoragePool_READY:
		return StoragePoolStateReady
	case netapppb.StoragePool_CREATING:
		return StoragePoolStateCreating
	case netapppb.StoragePool_DELETING:
		return StoragePoolStateDeleting
	case netapppb.StoragePool_UPDATING:
		return StoragePoolStateUpdating
	case netapppb.StoragePool_RESTORING:
		return StoragePoolStateRestoring
	case netapppb.StoragePool_DISABLED:
		return StoragePoolStateDisabled
	case netapppb.StoragePool_ERROR:
		return StoragePoolStateError
	}
}

// VolumeStateFromGCNVState converts GCNV volume state to string
func VolumeStateFromGCNVState(state netapppb.Volume_State) string {
	switch state {
	default:
		fallthrough
	case netapppb.Volume_STATE_UNSPECIFIED:
		return VolumeStateUnspecified
	case netapppb.Volume_READY:
		return VolumeStateReady
	case netapppb.Volume_CREATING:
		return VolumeStateCreating
	case netapppb.Volume_DELETING:
		return VolumeStateDeleting
	case netapppb.Volume_UPDATING:
		return VolumeStateUpdating
	case netapppb.Volume_RESTORING:
		return VolumeStateRestoring
	case netapppb.Volume_DISABLED:
		return VolumeStateDisabled
	case netapppb.Volume_ERROR:
		return VolumeStateError
	}
}

// VolumeSecurityStyleFromGCNVSecurityStyle converts GCNV volume security style to string
func VolumeSecurityStyleFromGCNVSecurityStyle(state netapppb.SecurityStyle) string {
	switch state {
	default:
		fallthrough
	case netapppb.SecurityStyle_SECURITY_STYLE_UNSPECIFIED:
		return SecurityStyleUnspecified
	case netapppb.SecurityStyle_NTFS:
		return SecurityStyleNTFS
	case netapppb.SecurityStyle_UNIX:
		return SecurityStyleUnix
	}
}

// GCNVSecurityStyleFromVolumeSecurityStyle converts string to GCNV volume security style
func GCNVSecurityStyleFromVolumeSecurityStyle(state string) netapppb.SecurityStyle {
	switch state {
	default:
		fallthrough
	case SecurityStyleUnspecified:
		return netapppb.SecurityStyle_SECURITY_STYLE_UNSPECIFIED
	case SecurityStyleNTFS:
		return netapppb.SecurityStyle_NTFS
	case SecurityStyleUnix:
		return netapppb.SecurityStyle_UNIX
	}
}

// VolumeAccessTypeFromGCNVAccessType converts GCNV volume access type to string
func VolumeAccessTypeFromGCNVAccessType(accessType netapppb.AccessType) string {
	switch accessType {
	default:
		fallthrough
	case netapppb.AccessType_ACCESS_TYPE_UNSPECIFIED:
		return AccessTypeUnspecified
	case netapppb.AccessType_READ_ONLY:
		return AccessTypeReadOnly
	case netapppb.AccessType_READ_WRITE:
		return AccessTypeReadWrite
	case netapppb.AccessType_READ_NONE:
		return AccessTypeReadNone
	}
}

// GCNVAccessTypeFromVolumeAccessType converts string to GCNV volume access type
func GCNVAccessTypeFromVolumeAccessType(accessType string) netapppb.AccessType {
	switch accessType {
	default:
		fallthrough
	case AccessTypeUnspecified:
		return netapppb.AccessType_ACCESS_TYPE_UNSPECIFIED
	case AccessTypeReadOnly:
		return netapppb.AccessType_READ_ONLY
	case AccessTypeReadWrite:
		return netapppb.AccessType_READ_WRITE
	case AccessTypeReadNone:
		return netapppb.AccessType_READ_NONE
	}
}

// VolumeProtocolFromGCNVProtocol converts GCNV protocol type to string
func VolumeProtocolFromGCNVProtocol(protocol netapppb.Protocols) string {
	switch protocol {
	default:
		fallthrough
	case netapppb.Protocols_PROTOCOLS_UNSPECIFIED:
		return ProtocolTypeUnknown
	case netapppb.Protocols_NFSV3:
		return ProtocolTypeNFSv3
	case netapppb.Protocols_NFSV4:
		return ProtocolTypeNFSv41
	case netapppb.Protocols_SMB:
		return ProtocolTypeSMB
	}
}

// GCNVProtocolFromVolumeProtocol converts string to GCNV protocol type
func GCNVProtocolFromVolumeProtocol(protocol string) netapppb.Protocols {
	switch protocol {
	default:
		fallthrough
	case ProtocolTypeUnknown:
		return netapppb.Protocols_PROTOCOLS_UNSPECIFIED
	case ProtocolTypeNFSv3:
		return netapppb.Protocols_NFSV3
	case ProtocolTypeNFSv41:
		return netapppb.Protocols_NFSV4
	case ProtocolTypeSMB:
		return netapppb.Protocols_SMB
	}
}

// SnapshotStateFromGCNVState converts GCNV snapshot state to string
func SnapshotStateFromGCNVState(state netapppb.Snapshot_State) string {
	switch state {
	default:
		fallthrough
	case netapppb.Snapshot_STATE_UNSPECIFIED:
		return SnapshotStateUnspecified
	case netapppb.Snapshot_READY:
		return SnapshotStateReady
	case netapppb.Snapshot_CREATING:
		return SnapshotStateCreating
	case netapppb.Snapshot_DELETING:
		return SnapshotStateDeleting
	case netapppb.Snapshot_UPDATING:
		return SnapshotStateUpdating
	case netapppb.Snapshot_DISABLED:
		return SnapshotStateDisabled
	case netapppb.Snapshot_ERROR:
		return SnapshotStateError
	}
}

// IsGCNVNotFoundError checks whether an error returned from the GCNV SDK contains a 404 (Not Found) error.
func IsGCNVNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	if s, ok := status.FromError(err); ok && s.Code() == codes.NotFound {
		return true
	}

	return false
}

// IsGCNVTooManyRequestsError checks whether an error returned from the GCNV SDK contains a 429 (Too Many Requests) error.
func IsGCNVTooManyRequestsError(err error) bool {
	if err == nil {
		return false
	}

	if s, ok := status.FromError(err); ok && s.Code() == codes.ResourceExhausted {
		return true
	}

	return false
}

// DerefString accepts a string pointer and returns the value of the string, or "" if the pointer is nil.
func DerefString(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}

// DerefBool accepts a bool pointer and returns the value of the bool, or false if the pointer is nil.
func DerefBool(b *bool) bool {
	if b != nil {
		return *b
	}
	return false
}

func DerefAccessType(at *netapppb.AccessType) netapppb.AccessType {
	if at != nil {
		return *at
	}
	return netapppb.AccessType_ACCESS_TYPE_UNSPECIFIED
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

func IsTerminalStateError(err error) bool {
	if err == nil {
		return false
	}
	var terminalStateError *TerminalStateError
	ok := errors.As(err, &terminalStateError)
	return ok
}
