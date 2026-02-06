// Copyright 2025 NetApp, Inc. All Rights Reserved.

// Package api provides a high-level interface to the Google Cloud NetApp Volumes SDK
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
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
	"github.com/netapp/trident/pkg/convert"
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

	// TODO(GCNV API): Remove FallbackIQNFormat when GCNV API provides the target IQN.
	// FallbackIQNFormat is a placeholder IQN format (serial-based) used when the authoritative target IQN
	// is not provided by the backend and must be discovered via iSCSI SendTargets discovery.
	FallbackIQNFormat = "iqn.2008-11.com.netapp.gcnv:%s"

	// GiBBytes is the number of bytes in a gibibyte (1024^3), used for converting between
	// GCNV's GiB-based capacity and Trident's byte-based sizes.
	GiBBytes = int64(1024 * 1024 * 1024)
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

	// GCP Workload Identity Pool credential configuration for GCNV API authentication
	WIPCredentialConfig *drivers.GCPWIPCredential

	// GCP CVS API authentication parameters
	APIKey *drivers.GCPPrivateKey

	// GCP region
	Location string

	// URL for accessing the API via an HTTP/HTTPS proxy
	ProxyURL string

	// APIEndpoint allows overriding the GCNV API endpoint for internal testing (autopush/staging).
	APIEndpoint string

	// Options
	DebugTraceFlags map[string]bool
	SDKTimeout      time.Duration // Timeout applied to all calls to the GCNV SDK
	MaxCacheAge     time.Duration // The oldest data we should expect in the cached resources
}

type GCNVClient struct {
	gcnv      *netapp.Client
	compute   *compute.RegionZonesClient
	resources *GCNVResources
}

// Client encapsulates connection details.
type Client struct {
	config    *ClientConfig
	sdkClient *GCNVClient
}

// NewDriver is a factory method for creating a new SDK interface.
func NewDriver(ctx context.Context, config *ClientConfig) (GCNV, error) {
	var credentials *google.Credentials
	var err error
	if config.WIPCredentialConfig != nil {
		Logc(ctx).Debug("Using GCP Workload Identity pool credentials from backend config")

		if err := validateWIPCredentialConfig(config.WIPCredentialConfig); err != nil {
			return nil, fmt.Errorf("invalid WIP credential configuration: %v", err)
		}

		credBytes, jsonErr := json.Marshal(config.WIPCredentialConfig)
		if jsonErr != nil {
			return nil, fmt.Errorf("failed to marshal WIP credential config: %v", jsonErr)
		}

		credentials, err = google.CredentialsFromJSON(ctx, credBytes, netapp.DefaultAuthScopes()...)
		if err != nil {
			return nil, fmt.Errorf("failed to create credentials from WIP credential config: %v", err)
		}
	} else if reflect.ValueOf(*config.APIKey).IsZero() {
		credentials, err = google.FindDefaultCredentials(ctx)
		if err != nil {
			return nil, err
		}
	} else if config.APIKey != nil {
		keyBytes, jsonErr := json.Marshal(config.APIKey)
		if jsonErr != nil {
			return nil, jsonErr
		}
		credentials, err = google.CredentialsFromJSON(ctx, keyBytes, netapp.DefaultAuthScopes()...)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("apiKey in config must be specified")
	}

	computeClient, computeErr := compute.NewRegionZonesRESTClient(ctx, option.WithCredentials(credentials))
	if computeErr != nil {
		return nil, computeErr
	}
	// Create GCNV client with optional endpoint override for testing
	clientOptions := []option.ClientOption{option.WithCredentials(credentials)}
	if config.APIEndpoint != "" {
		clientOptions = append(clientOptions, option.WithEndpoint(config.APIEndpoint))
	}
	gcnvClient, err := netapp.NewClient(ctx, clientOptions...)
	if err != nil {
		return nil, err
	}

	// NOTE: This package uses the NetApp v1 SDK (cloud.google.com/go/netapp v1.12.0+).
	sdkClient := &GCNVClient{
		gcnv:      gcnvClient,
		compute:   computeClient,
		resources: newGCNVResources(),
	}

	return Client{
		config:    config,
		sdkClient: sdkClient,
	}, nil
}

func validateWIPCredentialConfig(config *drivers.GCPWIPCredential) error {
	var missingFields []string

	if config.Type == "" {
		missingFields = append(missingFields, "type")
	}
	if config.Audience == "" {
		missingFields = append(missingFields, "audience")
	}
	if config.SubjectTokenType == "" {
		missingFields = append(missingFields, "subject_token_type")
	}
	if config.TokenURL == "" {
		missingFields = append(missingFields, "token_url")
	}
	if config.ServiceAccountImpersonationURL == "" {
		missingFields = append(missingFields, "service_account_impersonation_url")
	}
	if config.CredentialSource == nil {
		missingFields = append(missingFields, "credential_source")
	} else if config.CredentialSource.File == "" {
		missingFields = append(missingFields, "credential_source.file")
	}

	if len(missingFields) > 0 {
		return fmt.Errorf("missing required WIP credential fields: %s", strings.Join(missingFields, ", "))
	}

	return nil
}

// Init runs startup logic after allocating the driver resources.
func (c Client) Init(ctx context.Context, pools map[string]storage.Pool) error {
	// Map vpools to backend
	c.registerStoragePools(pools)

	// Find out what we have to work with in GCNV
	return c.RefreshGCNVResources(ctx)
}

// registerStoragePools makes a note of pools defined by the driver for later mapping.
func (c Client) registerStoragePools(sPools map[string]storage.Pool) {
	m := make(map[string]storage.Pool)

	for _, sPool := range sPools {
		m[sPool.Name()] = sPool
	}

	c.sdkClient.resources.SetStoragePools(m)
}

// ///////////////////////////////////////////////////////////////////////////////
// Functions to create & parse GCNV resource names
// ///////////////////////////////////////////////////////////////////////////////

// createBaseID creates the base GCNV-style ID for a project & location.
func (c Client) createBaseID(location string) string {
	return fmt.Sprintf("projects/%s/locations/%s", c.config.ProjectNumber, location)
}

// createCapacityPoolID creates the GCNV-style ID for a capacity pool.
func (c Client) createCapacityPoolID(location, capacityPool string) string {
	return fmt.Sprintf("projects/%s/locations/%s/storagePools/%s",
		c.config.ProjectNumber, location, capacityPool)
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
func (c Client) createVolumeID(location, volume string) string {
	return fmt.Sprintf("projects/%s/locations/%s/volumes/%s", c.config.ProjectNumber, location, volume)
}

// createHostGroupID creates the GCNV-style ID for a host group.
func (c Client) createHostGroupID(location, hostGroup string) string {
	return fmt.Sprintf("projects/%s/locations/%s/hostGroups/%s", c.config.ProjectNumber, location, hostGroup)
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
func (c Client) createSnapshotID(location, volume, snapshot string) string {
	return fmt.Sprintf("projects/%s/locations/%s/volumes/%s/snapshots/%s",
		c.config.ProjectNumber, location, volume, snapshot)
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

// flexPoolCount returns the number of Flex pools in the StoragePoolMap.
func (c Client) flexPoolCount() int {
	flexPoolsCount := 0
	c.sdkClient.resources.GetStoragePools().Range(func(_ string, storagePool storage.Pool) bool {
		if storagePool.InternalAttributes()[serviceLevel] == ServiceLevelFlex {
			flexPoolsCount++
		}
		return true
	})
	return flexPoolsCount
}

// findAllLocationsFromCapacityPool returns map of locations of all the CapacityPools
// that trident backend recognises.
func (c Client) findAllLocationsFromCapacityPool(flexPoolsCount int) map[string]struct{} {
	locations := make(map[string]struct{})
	if flexPoolsCount != 0 {
		c.sdkClient.resources.GetCapacityPools().Range(func(_ string, cPool *CapacityPool) bool {
			locations[cPool.Location] = struct{}{}
			return true
		})
	} else {
		locations[c.config.Location] = struct{}{}
	}
	return locations
}

// getLocationFromCapacityPools returns the location from the first available capacity pool.
// Returns an error if no capacity pools are available.
func (c Client) getLocationFromCapacityPools() (string, error) {
	pools := c.CapacityPools()
	if pools == nil || len(*pools) == 0 {
		return "", fmt.Errorf("no capacity pools available")
	}
	return (*pools)[0].Location, nil
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

	// Extract tiering information from the protobuf volume.
	tieringPolicy := drivers.TieringPolicyNone
	var tieringMinimumCoolingDays *int32

	if volume.TieringPolicy != nil {
		// Map GCNV TieringPolicy_ENABLED to Trident's "auto" policy.
		// For any other TierAction value (PAUSED, UNSPECIFIED) or when TieringPolicy is nil,
		// the tieringPolicy remains at the default TieringPolicyNone.
		switch volume.TieringPolicy.GetTierAction() {
		case netapppb.TieringPolicy_ENABLED:
			tieringPolicy = drivers.TieringPolicyAuto
			if volume.TieringPolicy.CoolingThresholdDays != nil {
				tieringMinimumCoolingDays = volume.TieringPolicy.CoolingThresholdDays
			}
		case netapppb.TieringPolicy_PAUSED, netapppb.TieringPolicy_TIER_ACTION_UNSPECIFIED:
			// Both PAUSED and UNSPECIFIED map to Trident's "none" policy.
			// PAUSED: Explicitly disabled tiering (user/admin set tieringPolicy=none).
			// UNSPECIFIED: Tiering action not specified by the API; Trident conservatively
			// treats this the same as PAUSED and does not enable auto-tiering.
			tieringPolicy = drivers.TieringPolicyNone
		default:
			// Future-proof: any new/unknown tier action conservatively maps to "none".
			tieringPolicy = drivers.TieringPolicyNone
		}
	}

	result := &Volume{
		Name:                      volumeName,
		CreationToken:             volume.ShareName,
		FullName:                  volume.Name,
		Location:                  location,
		State:                     VolumeStateFromGCNVState(volume.State),
		CapacityPool:              volume.StoragePool,
		NetworkName:               network,
		NetworkFullName:           volume.Network,
		ServiceLevel:              ServiceLevelFromCapacityPool(c.capacityPool(volume.StoragePool)),
		SizeBytes:                 volume.CapacityGib * GiBBytes,
		ExportPolicy:              c.exportPolicyImport(volume.ExportPolicy),
		ProtocolTypes:             protocolTypes,
		MountTargets:              c.getMountTargetsFromVolume(ctx, volume),
		UnixPermissions:           volume.UnixPermissions,
		Labels:                    volume.Labels,
		SnapshotReserve:           int64(volume.SnapReserve),
		SnapshotDirectory:         volume.SnapshotDirectory,
		SecurityStyle:             VolumeSecurityStyleFromGCNVSecurityStyle(volume.SecurityStyle),
		TieringPolicy:             tieringPolicy,
		TieringMinimumCoolingDays: tieringMinimumCoolingDays,
	}

	// If present, extract block device info and iSCSI connection details.
	if len(volume.BlockDevices) > 0 {
		for _, bd := range volume.BlockDevices {
			result.BlockDevices = append(result.BlockDevices, BlockDevice{
				HostGroups: bd.GetHostGroups(),
				Identifier: bd.GetIdentifier(),
				OSType:     osTypeToString(bd.GetOsType()),
				Name:       bd.GetName(),
			})
		}
		if len(result.BlockDevices) > 0 {
			result.SerialNumber = result.BlockDevices[0].Identifier
		}
		// Ensure ISCSI is present in ProtocolTypes for block volumes, even if the API didn't include it.
		hasISCSI := false
		for _, p := range result.ProtocolTypes {
			if p == ProtocolTypeISCSI {
				hasISCSI = true
				break
			}
		}
		if !hasISCSI {
			result.ProtocolTypes = append([]string{ProtocolTypeISCSI}, result.ProtocolTypes...)
		}
	}

	// Extract iSCSI portal details from mount options when present.
	for _, mo := range volume.MountOptions {
		if mo.GetProtocol() != netapppb.Protocols_ISCSI {
			continue
		}
		if mo.GetIpAddress() != "" {
			ipAddresses := strings.Split(mo.GetIpAddress(), ",")
			for _, ip := range ipAddresses {
				ip = strings.TrimSpace(ip)
				if ip == "" {
					continue
				}
				result.ISCSIPortals = append(result.ISCSIPortals, fmt.Sprintf("%s:3260", ip))
			}
			if len(result.ISCSIPortals) > 0 {
				result.ISCSITargetPortal = result.ISCSIPortals[0]
			}
		}

		// For GCNV, the real target IQN may need to be obtained via iSCSI discovery.
		// We set a placeholder IQN from the volume's LUN serial so something is populated.
		if result.SerialNumber != "" && result.ISCSITargetIQN == "" {
			result.ISCSITargetIQN = fmt.Sprintf(FallbackIQNFormat, result.SerialNumber)
		}
	}

	return result, nil
}

// convertProtoHostGroupToHostGroup converts a protobuf HostGroup to our HostGroup struct.
func (c Client) convertProtoHostGroupToHostGroup(protoHG *netapppb.HostGroup) *HostGroup {
	if protoHG == nil {
		return nil
	}

	hg := &HostGroup{
		Name:        protoHG.Name,
		Type:        hostGroupTypeFromProtoType(protoHG.Type),
		State:       hostGroupStateFromProtoState(protoHG.State),
		Hosts:       protoHG.Hosts,
		OSType:      osTypeFromProtoOSType(protoHG.OsType),
		Description: protoHG.Description,
		Labels:      protoHG.Labels,
	}

	if protoHG.CreateTime != nil {
		hg.CreateTime = protoHG.CreateTime.AsTime().Format(time.RFC3339)
	}

	return hg
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

	flexPoolsCount := c.flexPoolCount()
	locations := c.findAllLocationsFromCapacityPool(flexPoolsCount)

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()
	for location := range locations {

		req := &netapppb.ListVolumesRequest{
			Parent:   c.createBaseID(location),
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
	}

	return &volumes, nil
}

// Volume uses a volume config record to fetch a volume by the most efficient means.
func (c Client) Volume(ctx context.Context, volConfig *storage.VolumeConfig) (*Volume, error) {
	// When we know the internal ID, use that as it is vastly more efficient
	if volConfig.InternalID != "" {
		return c.VolumeByID(ctx, volConfig.InternalID)
	}

	// Fall back to the name
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

	flexPoolsCount := c.flexPoolCount()
	locations := c.findAllLocationsFromCapacityPool(flexPoolsCount)

	gcnvVolumes := make(map[string]*netapppb.Volume)
	var gcnvVolume *netapppb.Volume
	var err error
	// We iterate over a list of region and zones to check if the volume is in that region or zone.
	// We use this logic to reduce the number of API calls to the GCNV API
	for location := range locations {
		req := &netapppb.GetVolumeRequest{
			Name: c.createVolumeID(location, name),
		}
		gcnvVolume, err = c.sdkClient.gcnv.GetVolume(sdkCtx, req)
		if gcnvVolume != nil {
			gcnvVolumes[gcnvVolume.Name] = gcnvVolume
		} else if err != nil {
			if IsGCNVNotFoundError(err) {
				Logc(ctx).WithFields(logFields).Debugf("Volume not found in '%s' location.", location)
				continue
			}

			Logc(ctx).WithFields(logFields).WithError(err).Error("Error fetching volume.")
		}
	}

	if len(gcnvVolumes) == 0 {
		if IsGCNVNotFoundError(err) {
			return nil, errors.WrapWithNotFoundError(err, "volume '%s' not found", name)
		}
		return nil, err
	} else if len(gcnvVolumes) > 1 {
		return nil, errors.New("found multiple volumes with the same name in the given location")
	}
	Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["api"]).
		WithFields(logFields).Debug("Found volume by name.")
	for _, volume := range gcnvVolumes {
		gcnvVolume = volume
	}

	return c.newVolumeFromGCNVVolume(ctx, gcnvVolume)
}

// VolumeExistsByName checks whether a volume exists using its volume name as a key.
func (c Client) VolumeExistsByName(ctx context.Context, name string) (bool, *Volume, error) {
	volume, err := c.VolumeByName(ctx, name)
	if err != nil {
		if IsGCNVNotFoundError(err) {
			return false, nil, nil
		} else {
			return false, nil, err
		}
	}
	return true, volume, nil
}

// VolumeExists uses a volume config record to look for a Filesystem by the most efficient means.
func (c Client) VolumeExists(ctx context.Context, volConfig *storage.VolumeConfig) (bool, *Volume, error) {
	// When we know the internal ID, use that as it is vastly more efficient
	if volConfig.InternalID != "" {
		return c.VolumeExistsByID(ctx, volConfig.InternalID)
	}
	// Fall back to the creation token
	return c.VolumeExistsByName(ctx, volConfig.InternalName)
}

// VolumeByID returns a volume based on its full GCNV-style resource ID.
func (c Client) VolumeByID(ctx context.Context, id string) (*Volume, error) {
	logFields := LogFields{
		"API":    "GCNV.GetVolume",
		"volume": id,
	}

	Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["api"]).
		WithFields(logFields).Trace("Fetching volume by volumeID.")

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()
	req := &netapppb.GetVolumeRequest{
		Name: id,
	}
	gcnvVolume, err := c.sdkClient.gcnv.GetVolume(sdkCtx, req)
	if err != nil {
		if IsGCNVNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Debug("Volume not found.")
			return nil, errors.WrapWithNotFoundError(err, "volume '%s' not found", id)
		}

		Logc(ctx).WithFields(logFields).WithError(err).Error("Error fetching volume.")
		return nil, err
	}

	Logd(ctx, c.config.StorageDriverName, c.config.DebugTraceFlags["api"]).
		WithFields(logFields).Debug("Found volume by id.")

	return c.newVolumeFromGCNVVolume(ctx, gcnvVolume)
}

// VolumeExistsByID checks whether a volume exists using its volume id as a key.
func (c Client) VolumeExistsByID(ctx context.Context, id string) (bool, *Volume, error) {
	volume, err := c.VolumeByID(ctx, id)
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
		v, err := c.VolumeByID(ctx, volume.FullName)
		if err != nil {

			// There is no 'Deleted' state in GCNV -- the volume just vanishes.  If we failed to query
			// the volume info, and we're trying to transition to StateDeleted, and we get back a 404,
			// then return success.  Otherwise, log the error as usual.
			if desiredState == VolumeStateDeleted && errors.IsNotFoundError(err) {
				Logc(ctx).Debugf("Implied deletion for volume %s.", volume.Name)
				volumeState = VolumeStateDeleted
				return nil
			}
			if IsGCNVTimeoutError(err) {
				Logc(ctx).WithError(err).Debugf("Timed out while waiting for volume %s state.", volume.Name)
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

// CreateVolume creates a new volume (NAS or block).
// For NAS volumes (NFS/SMB), it sets export policy, unix permissions, snapshot directory, etc.
// For block volumes (iSCSI), it sets block devices with OS type.
func (c Client) CreateVolume(ctx context.Context, request *VolumeCreateRequest) (*Volume, error) {
	var protocols []netapppb.Protocols
	isBlockVolume := false
	for _, protocolType := range request.ProtocolTypes {
		protocols = append(protocols, GCNVProtocolFromVolumeProtocol(protocolType))
		if protocolType == ProtocolTypeISCSI {
			isBlockVolume = true
		}
	}

	cPool := c.capacityPool(request.CapacityPool)
	if cPool == nil {
		return nil, fmt.Errorf("pool %s not found", request.CapacityPool)
	}

	newVol := &netapppb.Volume{
		Name:        request.Name,
		StoragePool: request.CapacityPool,
		CapacityGib: request.SizeBytes / GiBBytes,
		Protocols:   protocols,
		Labels:      request.Labels,
	}

	// Add auto-tiering configuration using TieringPolicy
	switch request.TieringPolicy {
	case drivers.TieringPolicyAuto:
		// Only set tiering if the pool supports it
		if !cPool.AutoTiering {
			return nil, fmt.Errorf("cannot enable auto-tiering: storage pool %s does not support auto-tiering", request.CapacityPool)
		}

		// If cooling days is unset, omit it and let the service apply its default.
		tierAction := netapppb.TieringPolicy_ENABLED
		newVol.TieringPolicy = &netapppb.TieringPolicy{
			TierAction:           &tierAction,
			CoolingThresholdDays: request.TieringMinimumCoolingDays,
		}

		Logc(ctx).WithFields(LogFields{
			"volume":                    request.Name,
			"tieringPolicy":             request.TieringPolicy,
			"tieringMinimumCoolingDays": convert.PtrToString(request.TieringMinimumCoolingDays),
		}).Debug("Auto-tiering enabled for volume creation")
	case drivers.TieringPolicyNone:
		if cPool.AutoTiering {
			tierAction := netapppb.TieringPolicy_PAUSED
			newVol.TieringPolicy = &netapppb.TieringPolicy{
				TierAction: &tierAction,
			}
		}
	}

	if request.ExportPolicy != nil {
		newVol.ExportPolicy = exportPolicyExport(request.ExportPolicy)
	}
	if request.SnapshotReserve != nil {
		newVol.SnapReserve = float64(*request.SnapshotReserve)
	}

	// Set NAS vs block specific fields.
	if isBlockVolume {
		newVol.BlockDevices = []*netapppb.BlockDevice{
			{
				OsType:     stringToOsType(request.OSType),
				HostGroups: []string{}, // Start with empty mappings for per-node architecture
			},
		}
	} else {
		newVol.ShareName = request.CreationToken
		newVol.UnixPermissions = request.UnixPermissions
		newVol.SnapshotDirectory = request.SnapshotDirectory
		newVol.SecurityStyle = GCNVSecurityStyleFromVolumeSecurityStyle(request.SecurityStyle)
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
		"isBlock":       isBlockVolume,
	}).Debug("Issuing create request.")

	logFields := LogFields{
		"API":    "GCNV.CreateVolume",
		"volume": request.Name,
	}

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()
	req := &netapppb.CreateVolumeRequest{
		Parent:   c.createBaseID(cPool.Location),
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
	}
	// The volume doesn't exist yet, so forge the name & network IDs to enable conversion to a Volume struct
	newVol.Name = c.createVolumeID(cPool.Location, request.Name)
	newVol.Network = cPool.NetworkFullName
	return c.newVolumeFromGCNVVolume(ctx, newVol)
}

// UpdateNASVolume updates NAS-specific attributes of a volume: labels, unix permissions, and snapshot directory access.
// This is only applicable to NFS/SMB volumes. For block volumes, use UpdateSANVolume.
func (c Client) UpdateNASVolume(
	ctx context.Context, volume *Volume, labels map[string]string, unixPermissions *string,
	snapshotDirAccess *bool, _ *ExportRule,
) error {
	logFields := LogFields{
		"API":    "GCNV.UpdateVolumeAttributes",
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

	Logc(ctx).WithFields(logFields).Debug("Updating volume attributes.")

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()
	req := &netapppb.UpdateVolumeRequest{
		Volume:     newVolume,
		UpdateMask: updateMask,
	}
	poller, err := c.sdkClient.gcnv.UpdateVolume(sdkCtx, req)
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error updating volume attributes.")
		return err
	}

	Logc(ctx).WithFields(logFields).Info("Volume attribute update request issued.")

	waitCtx, waitCancel := context.WithTimeout(ctx, DefaultTimeout)
	defer waitCancel()
	if _, pollErr := poller.Wait(waitCtx); pollErr != nil {
		Logc(ctx).WithFields(logFields).WithError(pollErr).Error("Error polling for volume attribute update result.")
		return pollErr
	}

	Logc(ctx).WithFields(logFields).Debug("Volume attributes updated.")

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
		CapacityGib: newSizeBytes / GiBBytes,
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
	if _, waitErr := poller.Wait(waitCtx); waitErr != nil {
		Logc(ctx).WithFields(logFields).WithError(waitErr).Error("Error polling for volume resize result.")
		return waitErr
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
		Name:  c.createVolumeID(volume.Location, name),
		Force: true,
	}
	poller, err := c.sdkClient.gcnv.DeleteVolume(sdkCtx, req)
	if err != nil {
		if IsGCNVNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Info("Volume already deleted.")
			return nil
		}

		Logc(ctx).WithFields(logFields).WithError(err).Error("Error deleting volume.")
		return err
	}

	Logc(ctx).WithFields(logFields).Debug("Volume delete request issued.")

	if pollErr := poller.Poll(sdkCtx); pollErr != nil {
		return pollErr
	}

	return nil
}

// VolumeByNameOrID retrieves a gcnv volume for import by volumeName or volumeID
func (c Client) VolumeByNameOrID(ctx context.Context, volumeID string) (*Volume, error) {
	var volume *Volume
	var err error
	match := volumeNameRegex.FindStringSubmatch(volumeID)
	if match == nil {
		volume, err = c.VolumeByName(ctx, volumeID)
	} else {
		volume, err = c.VolumeByID(ctx, volumeID)
	}
	if err != nil {
		return nil, err
	}
	return volume, nil
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
		Parent:   c.createVolumeID(volume.Location, volume.Name),
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
		Name: c.createSnapshotID(volume.Location, volume.Name, snapshotName),
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

// CreateSnapshot creates a new snapshot.
func (c Client) CreateSnapshot(
	ctx context.Context, volume *Volume, snapshotName string, waitDuration time.Duration,
) (*Snapshot, error) {
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

	waitCtx, waitCancel := context.WithTimeout(ctx, waitDuration)
	defer waitCancel()
	if snapshot, pollErr := poller.Wait(waitCtx); pollErr != nil {
		Logc(ctx).WithFields(logFields).WithError(pollErr).Error("Error polling for create snapshot result.")
		return nil, pollErr
	} else {
		return c.newSnapshotFromGCNVSnapshot(ctx, snapshot)
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
func (c Client) DeleteSnapshot(
	ctx context.Context, volume *Volume, snapshot *Snapshot, waitDuration time.Duration,
) error {
	logFields := LogFields{
		"API":      "GCNV.DeleteSnapshot",
		"volume":   volume.Name,
		"snapshot": snapshot.Name,
	}

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()
	req := &netapppb.DeleteSnapshotRequest{
		Name: c.createSnapshotID(volume.Location, volume.Name, snapshot.Name),
	}
	poller, err := c.sdkClient.gcnv.DeleteSnapshot(sdkCtx, req)
	if err != nil {
		if IsGCNVNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Info("Snapshot already deleted.")
			return nil
		}

		Logc(ctx).WithFields(logFields).WithError(err).Error("Error deleting snapshot.")
		return err
	}

	Logc(ctx).WithFields(logFields).Debug("Snapshot delete request issued.")

	waitCtx, waitCancel := context.WithTimeout(ctx, waitDuration)
	defer waitCancel()
	if pollErr := poller.Wait(waitCtx); pollErr != nil {
		Logc(ctx).WithFields(logFields).WithError(pollErr).Error("Error waiting for delete snapshot result.")
		return pollErr
	}

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
	case netapppb.Protocols_NFSV3:
		return ProtocolTypeNFSv3
	case netapppb.Protocols_NFSV4:
		return ProtocolTypeNFSv41
	case netapppb.Protocols_SMB:
		return ProtocolTypeSMB
	case netapppb.Protocols_ISCSI:
		return ProtocolTypeISCSI
	default:
		return ProtocolTypeUnknown
	}
}

// GCNVProtocolFromVolumeProtocol converts string to GCNV protocol type.
func GCNVProtocolFromVolumeProtocol(protocol string) netapppb.Protocols {
	switch protocol {
	case ProtocolTypeNFSv3:
		return netapppb.Protocols_NFSV3
	case ProtocolTypeNFSv41:
		return netapppb.Protocols_NFSV4
	case ProtocolTypeSMB:
		return netapppb.Protocols_SMB
	case ProtocolTypeISCSI:
		return netapppb.Protocols_ISCSI
	default:
		return netapppb.Protocols_PROTOCOLS_UNSPECIFIED
	}
}

func osTypeToString(osType netapppb.OsType) string {
	switch osType {
	case netapppb.OsType_LINUX:
		return OSTypeLinux
	case netapppb.OsType_WINDOWS:
		return OSTypeWindows
	default:
		return ""
	}
}

func stringToOsType(osType string) netapppb.OsType {
	switch strings.ToUpper(osType) {
	case OSTypeLinux:
		return netapppb.OsType_LINUX
	case OSTypeWindows:
		return netapppb.OsType_WINDOWS
	default:
		return netapppb.OsType_OS_TYPE_UNSPECIFIED
	}
}

// hostGroupTypeFromProtoType converts protobuf HostGroup_Type to string
func hostGroupTypeFromProtoType(t netapppb.HostGroup_Type) string {
	switch t {
	case netapppb.HostGroup_ISCSI_INITIATOR:
		return HostGroupTypeISCSIInitiator
	default:
		return "TYPE_UNSPECIFIED"
	}
}

// hostGroupStateFromProtoState converts protobuf HostGroup_State to string
func hostGroupStateFromProtoState(s netapppb.HostGroup_State) string {
	switch s {
	case netapppb.HostGroup_READY:
		return HostGroupStateReady
	case netapppb.HostGroup_CREATING:
		return HostGroupStateCreating
	case netapppb.HostGroup_UPDATING:
		return HostGroupStateUpdating
	case netapppb.HostGroup_DELETING:
		return HostGroupStateDeleting
	default:
		return HostGroupStateUnspecified
	}
}

// osTypeFromProtoOSType converts protobuf OsType to string
func osTypeFromProtoOSType(osType netapppb.OsType) string {
	switch osType {
	case netapppb.OsType_LINUX:
		return OSTypeLinux
	case netapppb.OsType_WINDOWS:
		return OSTypeWindows
	default:
		return ""
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

// ///////////////////////////////////////////////////////////////////////////////
// Block Storage (iSCSI LUN) API Methods - v1 protobuf API (v1.12.0+)
// ///////////////////////////////////////////////////////////////////////////////

// UpdateSANVolume updates SAN/block volume properties (labels, block devices) using v1 protobuf SDK.
// This is only applicable to iSCSI volumes. For NAS volumes, use UpdateNASVolume.
func (c Client) UpdateSANVolume(ctx context.Context, volume *Volume, request *VolumeUpdateRequest) (*Volume, error) {
	logFields := LogFields{
		"API":       "GCNV.UpdateVolume",
		"volume":    volume.FullName,
		"fieldMask": request.FieldMask,
	}

	Logc(ctx).WithFields(logFields).Debug("Updating volume via NetApp v1 SDK.")

	// Build the volume update with only the fields specified in FieldMask
	updatedVolume := &netapppb.Volume{
		Name: volume.FullName,
	}

	// Add fields based on FieldMask
	for _, field := range request.FieldMask {
		switch field {
		case "labels":
			if request.Labels != nil {
				updatedVolume.Labels = request.Labels
			}
		case "block_devices":
			// BlockDevices updates are handled by AddHostGroupToVolume/RemoveHostGroupFromVolume
			// This case is here for completeness but not currently used
			if request.BlockDevices != nil {
				Logc(ctx).WithFields(logFields).Debug("BlockDevices update requested but should use AddHostGroupToVolume/RemoveHostGroupFromVolume instead.")
			}
		default:
			Logc(ctx).WithFields(logFields).WithField("unknownField", field).Debug("Unknown field in FieldMask, ignoring.")
		}
	}

	updateMask := &fieldmaskpb.FieldMask{
		Paths: request.FieldMask,
	}

	req := &netapppb.UpdateVolumeRequest{
		Volume:     updatedVolume,
		UpdateMask: updateMask,
	}

	opCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	op, err := c.sdkClient.gcnv.UpdateVolume(opCtx, req)
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error updating volume.")
		return nil, err
	}

	// Wait for the update operation to complete
	gcnvVol, err := op.Wait(opCtx)
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error waiting for volume update operation.")
		return nil, err
	}

	Logc(ctx).WithFields(logFields).Debug("Volume updated successfully.")
	return c.newVolumeFromGCNVVolume(ctx, gcnvVol)
}

// VolumeMappedHostGroups retrieves the list of host groups mapped to a volume.
func (c Client) VolumeMappedHostGroups(ctx context.Context, volumeID string) ([]string, error) {
	volume, err := c.VolumeByID(ctx, volumeID)
	if err != nil {
		return nil, err
	}

	if len(volume.BlockDevices) > 0 {
		return volume.BlockDevices[0].HostGroups, nil
	}

	return []string{}, nil
}

// AddHostGroupToVolume adds a host group to the volume's blockDevices[0].hostGroups array.
func (c Client) AddHostGroupToVolume(ctx context.Context, volumeID, hostGroupID string) error {
	logFields := LogFields{
		"API":         "GCNV.AddHostGroupToVolume",
		"volume":      volumeID,
		"hostGroupID": hostGroupID,
	}

	Logc(ctx).WithFields(logFields).Debug("Adding host group to volume via NetApp v1 SDK.")

	sdkCtx, cancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer cancel()
	gcnvVol, err := c.sdkClient.gcnv.GetVolume(sdkCtx, &netapppb.GetVolumeRequest{Name: volumeID})
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error fetching volume.")
		return err
	}
	if len(gcnvVol.BlockDevices) == 0 {
		return fmt.Errorf("volume %s has no block devices", volumeID)
	}

	current := gcnvVol.BlockDevices[0].GetHostGroups()
	for _, hg := range current {
		if hg == hostGroupID {
			return nil
		}
	}
	updatedHostGroups := append(current, hostGroupID)

	// Create a new BlockDevice with only the fields we're updating (HostGroups).
	// Do not include read-only fields like SizeGib, which the API rejects.
	// Name is required by the API.
	blockDeviceName := gcnvVol.BlockDevices[0].Name
	if blockDeviceName != nil {
		Logc(ctx).WithFields(logFields).WithField("blockDeviceName", *blockDeviceName).Info("Retrieved block device name for host group update.")
	} else {
		Logc(ctx).WithFields(logFields).Debug("Block device name is nil.")
	}

	updatedBlockDevice := &netapppb.BlockDevice{
		Name:       blockDeviceName, // Required field (pointer to string)
		HostGroups: updatedHostGroups,
		OsType:     gcnvVol.BlockDevices[0].GetOsType(), // Preserve OS type
	}

	updateReq := &netapppb.UpdateVolumeRequest{
		Volume: &netapppb.Volume{
			Name:         gcnvVol.Name,
			BlockDevices: []*netapppb.BlockDevice{updatedBlockDevice},
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"block_devices"}},
	}

	opCtx, opCancel := context.WithTimeout(ctx, DefaultTimeout)
	defer opCancel()
	op, err := c.sdkClient.gcnv.UpdateVolume(opCtx, updateReq)
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error adding host group to volume.")
		return err
	}
	// Use a detached context for waiting - the operation should complete regardless of caller timeout
	waitCtx, waitCancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer waitCancel()
	if _, err := op.Wait(waitCtx); err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error adding host group to volume.")
		return err
	}

	Logc(ctx).WithFields(logFields).Info("Host group added to volume.")
	return nil
}

// RemoveHostGroupFromVolume removes a host group from the volume's blockDevices[0].hostGroups array.
func (c Client) RemoveHostGroupFromVolume(ctx context.Context, volumeID, hostGroupID string) error {
	logFields := LogFields{
		"API":         "GCNV.RemoveHostGroupFromVolume",
		"volume":      volumeID,
		"hostGroupID": hostGroupID,
	}

	Logc(ctx).WithFields(logFields).Debug("Removing host group from volume via NetApp v1 SDK.")

	sdkCtx, cancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer cancel()
	gcnvVol, err := c.sdkClient.gcnv.GetVolume(sdkCtx, &netapppb.GetVolumeRequest{Name: volumeID})
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error fetching volume.")
		return err
	}
	if len(gcnvVol.BlockDevices) == 0 {
		return fmt.Errorf("volume %s has no block devices", volumeID)
	}

	current := gcnvVol.BlockDevices[0].GetHostGroups()
	updated := make([]string, 0, len(current))
	found := false
	for _, hg := range current {
		if hg == hostGroupID {
			found = true
			continue
		}
		updated = append(updated, hg)
	}
	if !found {
		return nil
	}

	// Create a new BlockDevice with only the fields we're updating (HostGroups).
	// Do not include read-only fields like SizeGib, which the API rejects.
	// Name is required by the API.
	blockDeviceName := gcnvVol.BlockDevices[0].Name
	if blockDeviceName != nil {
		Logc(ctx).WithFields(logFields).WithField("blockDeviceName", *blockDeviceName).Info("Retrieved block device name for host group removal.")
	} else {
		Logc(ctx).WithFields(logFields).Debug("Block device name is nil.")
	}

	updatedBlockDevice := &netapppb.BlockDevice{
		Name:       blockDeviceName, // Required field (pointer to string)
		HostGroups: updated,
		OsType:     gcnvVol.BlockDevices[0].GetOsType(), // Preserve OS type
	}

	updateReq := &netapppb.UpdateVolumeRequest{
		Volume: &netapppb.Volume{
			Name:         gcnvVol.Name,
			BlockDevices: []*netapppb.BlockDevice{updatedBlockDevice},
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"block_devices"}},
	}

	opCtx, opCancel := context.WithTimeout(ctx, DefaultTimeout)
	defer opCancel()
	op, err := c.sdkClient.gcnv.UpdateVolume(opCtx, updateReq)
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error removing host group from volume.")
		return err
	}
	// Use a detached context for waiting - the operation should complete regardless of caller timeout
	waitCtx, waitCancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer waitCancel()
	if _, err := op.Wait(waitCtx); err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error removing host group from volume.")
		return err
	}

	Logc(ctx).WithFields(logFields).Info("Host group removed from volume.")
	return nil
}

// ISCSITargetInfo extracts iSCSI connection details from a volume.
func (c Client) ISCSITargetInfo(ctx context.Context, volume *Volume) (*ISCSITargetInfo, error) {
	if volume == nil {
		return nil, fmt.Errorf("volume cannot be nil")
	}

	// Check if volume is an iSCSI volume by checking for BlockDevices or ISCSI protocol
	isISCSI := len(volume.BlockDevices) > 0
	if !isISCSI && len(volume.ProtocolTypes) > 0 {
		for _, proto := range volume.ProtocolTypes {
			if proto == ProtocolTypeISCSI {
				isISCSI = true
				break
			}
		}
	}

	if !isISCSI {
		return nil, fmt.Errorf("volume %s is not an iSCSI volume", volume.Name)
	}

	info := &ISCSITargetInfo{
		TargetIQN:    volume.ISCSITargetIQN,
		TargetPortal: volume.ISCSITargetPortal,
		Portals:      volume.ISCSIPortals,
		LunID:        volume.LunID,
		NetworkPath:  volume.NetworkFullName,
	}

	return info, nil
}

// ///////////////////////////////////////////////////////////////////////////////
// Host Group API Methods - v1 protobuf SDK
// ///////////////////////////////////////////////////////////////////////////////

// HostGroups retrieves all host groups in the configured location using v1 protobuf SDK.
func (c Client) HostGroups(ctx context.Context) ([]*HostGroup, error) {
	logFields := LogFields{
		"API": "GCNV.HostGroups",
	}

	Logc(ctx).WithFields(logFields).Debug("Listing host groups via v1 protobuf SDK.")

	location, err := c.getLocationFromCapacityPools()
	if err != nil {
		return []*HostGroup{}, nil
	}

	sdkCtx, cancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer cancel()
	req := &netapppb.ListHostGroupsRequest{
		Parent:   c.createBaseID(location),
		PageSize: PaginationLimit,
	}
	it := c.sdkClient.gcnv.ListHostGroups(sdkCtx, req)

	hostGroups := make([]*HostGroup, 0)
	for {
		hg, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			Logc(ctx).WithFields(logFields).WithError(err).Error("Error listing host groups.")
			return nil, err
		}
		hostGroups = append(hostGroups, c.convertProtoHostGroupToHostGroup(hg))
	}

	Logc(ctx).WithFields(logFields).WithField("count", len(hostGroups)).Debug("Listed host groups.")
	return hostGroups, nil
}

// HostGroupByName retrieves a specific host group by name using v1 protobuf SDK.
func (c Client) HostGroupByName(ctx context.Context, name string) (*HostGroup, error) {
	logFields := LogFields{
		"API":           "GCNV.HostGroupByName",
		"hostGroupName": name,
	}

	Logc(ctx).WithFields(logFields).Debug("Fetching host group by name via v1 protobuf SDK.")

	location, err := c.getLocationFromCapacityPools()
	if err != nil {
		return nil, err
	}

	fullName := c.createHostGroupID(location, name)

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()

	req := &netapppb.GetHostGroupRequest{
		Name: fullName,
	}

	gcnvHostGroup, err := c.sdkClient.gcnv.GetHostGroup(sdkCtx, req)
	if err != nil {
		if IsGCNVNotFoundError(err) {
			return nil, errors.NotFoundError("host group %s not found", name)
		}
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error fetching host group.")
		return nil, err
	}

	hostGroup := c.convertProtoHostGroupToHostGroup(gcnvHostGroup)

	Logc(ctx).WithFields(logFields).Debug("Fetched host group.")
	return hostGroup, nil
}

// CreateHostGroup creates a new host group with the specified initiators.
func (c Client) CreateHostGroup(ctx context.Context, request *HostGroupCreateRequest) (*HostGroup, error) {
	logFields := LogFields{
		"API":           "GCNV.CreateHostGroup",
		"hostGroupName": request.ResourceID,
	}

	Logc(ctx).WithFields(logFields).Debug("Creating host group via v1 protobuf SDK.")

	location, err := c.getLocationFromCapacityPools()
	if err != nil {
		return nil, err
	}

	// Validate and convert host group type
	hostGroupType := netapppb.HostGroup_ISCSI_INITIATOR
	if request.Type != "" {
		if request.Type != HostGroupTypeISCSIInitiator {
			return nil, fmt.Errorf("unsupported host group type: %s (only %s is supported)", request.Type, HostGroupTypeISCSIInitiator)
		}
	}

	// Validate and convert OS type
	osType := netapppb.OsType_LINUX
	if request.OSType != "" {
		osType = stringToOsType(request.OSType)
		if osType == netapppb.OsType_OS_TYPE_UNSPECIFIED {
			return nil, fmt.Errorf("invalid OS type: %s (must be %s or %s)", request.OSType, OSTypeLinux, OSTypeWindows)
		}
	}

	newHostGroup := &netapppb.HostGroup{
		Type:        hostGroupType,
		Hosts:       request.Hosts,
		OsType:      osType,
		Description: request.Description,
		Labels:      make(map[string]string),
	}

	req := &netapppb.CreateHostGroupRequest{
		Parent:      c.createBaseID(location),
		HostGroupId: request.ResourceID,
		HostGroup:   newHostGroup,
	}

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()

	poller, err := c.sdkClient.gcnv.CreateHostGroup(sdkCtx, req)
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error creating host group.")
		return nil, err
	}

	Logc(ctx).WithFields(logFields).Info("Host group create request issued.")

	// Wait for operation to complete
	waitCtx, waitCancel := context.WithTimeout(ctx, DefaultTimeout)
	defer waitCancel()
	gcnvHostGroup, pollErr := poller.Wait(waitCtx)
	if pollErr != nil {
		Logc(ctx).WithFields(logFields).WithError(pollErr).Error("Error polling for host group creation result.")
		return nil, pollErr
	}

	// Convert protobuf response to our HostGroup struct
	hostGroup := c.convertProtoHostGroupToHostGroup(gcnvHostGroup)

	Logc(ctx).WithFields(logFields).Info("Host group created successfully.")
	return hostGroup, nil
}

// UpdateHostGroup updates an existing host group (e.g., modify initiators).
func (c Client) UpdateHostGroup(ctx context.Context, hostGroup *HostGroup, newHosts []string) error {
	logFields := LogFields{
		"API":           "GCNV.UpdateHostGroup",
		"hostGroupName": hostGroup.Name,
	}

	Logc(ctx).WithFields(logFields).Debug("Updating host group via v1 protobuf SDK.")

	updatedHostGroup := &netapppb.HostGroup{
		Name:  hostGroup.Name,
		Hosts: newHosts,
	}
	updateMask := &fieldmaskpb.FieldMask{
		Paths: []string{"hosts"},
	}

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()

	req := &netapppb.UpdateHostGroupRequest{
		HostGroup:  updatedHostGroup,
		UpdateMask: updateMask,
	}

	poller, err := c.sdkClient.gcnv.UpdateHostGroup(sdkCtx, req)
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error updating host group.")
		return err
	}

	Logc(ctx).WithFields(logFields).Info("Host group update request issued.")

	// Wait for operation to complete
	waitCtx, waitCancel := context.WithTimeout(ctx, DefaultTimeout)
	defer waitCancel()
	if _, waitErr := poller.Wait(waitCtx); waitErr != nil {
		Logc(ctx).WithFields(logFields).WithError(waitErr).Error("Error waiting for host group update to complete.")
		return waitErr
	}

	Logc(ctx).WithFields(logFields).Info("Host group updated successfully.")
	return nil
}

// DeleteHostGroup deletes a host group (only if no volumes are mapped).
func (c Client) DeleteHostGroup(ctx context.Context, hostGroup *HostGroup) error {
	logFields := LogFields{
		"API":           "GCNV.DeleteHostGroup",
		"hostGroupName": hostGroup.Name,
	}

	Logc(ctx).WithFields(logFields).Debug("Deleting host group via v1 protobuf SDK.")

	sdkCtx, sdkCancel := context.WithTimeout(ctx, c.config.SDKTimeout)
	defer sdkCancel()

	req := &netapppb.DeleteHostGroupRequest{
		Name: hostGroup.Name,
	}

	poller, err := c.sdkClient.gcnv.DeleteHostGroup(sdkCtx, req)
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Error deleting host group.")
		return err
	}

	Logc(ctx).WithFields(logFields).Info("Host group delete request issued.")

	// Use a detached context for waiting - the operation should complete regardless of caller timeout
	waitCtx, waitCancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer waitCancel()
	if waitErr := poller.Wait(waitCtx); waitErr != nil {
		Logc(ctx).WithFields(logFields).WithError(waitErr).Error("Error waiting for host group deletion to complete.")
		return waitErr
	}

	Logc(ctx).WithFields(logFields).Info("Host group deleted successfully.")
	return nil
}

// AddInitiatorsToHostGroup adds IQNs to an existing host group.
func (c Client) AddInitiatorsToHostGroup(ctx context.Context, hostGroup *HostGroup, iqns []string) error {
	logFields := LogFields{
		"API":           "GCNV.AddInitiatorsToHostGroup",
		"hostGroupName": hostGroup.Name,
		"iqns":          iqns,
	}

	Logc(ctx).WithFields(logFields).Debug("Adding initiators to host group.")

	// Add new IQNs (deduplicate)
	existingIQNs := make(map[string]bool)
	for _, iqn := range hostGroup.Hosts {
		existingIQNs[iqn] = true
	}

	updatedHosts := hostGroup.Hosts
	for _, iqn := range iqns {
		if !existingIQNs[iqn] {
			updatedHosts = append(updatedHosts, iqn)
		}
	}

	// Only update if there are changes
	if len(updatedHosts) == len(hostGroup.Hosts) {
		Logc(ctx).WithFields(logFields).Debug("No new initiators to add.")
		return nil
	}

	return c.UpdateHostGroup(ctx, hostGroup, updatedHosts)
}

// RemoveInitiatorsFromHostGroup removes IQNs from an existing host group.
func (c Client) RemoveInitiatorsFromHostGroup(ctx context.Context, hostGroup *HostGroup, iqns []string) error {
	logFields := LogFields{
		"API":           "GCNV.RemoveInitiatorsFromHostGroup",
		"hostGroupName": hostGroup.Name,
		"iqns":          iqns,
	}

	Logc(ctx).WithFields(logFields).Debug("Removing initiators from host group.")

	// Remove IQNs
	toRemove := make(map[string]bool)
	for _, iqn := range iqns {
		toRemove[iqn] = true
	}

	updatedHosts := []string{}
	for _, iqn := range hostGroup.Hosts {
		if !toRemove[iqn] {
			updatedHosts = append(updatedHosts, iqn)
		}
	}

	// Only update if there are changes
	if len(updatedHosts) == len(hostGroup.Hosts) {
		Logc(ctx).WithFields(logFields).Debug("No initiators to remove.")
		return nil
	}

	return c.UpdateHostGroup(ctx, hostGroup, updatedHosts)
}

// HostGroupVolumes retrieves all volumes mapped to a specific host group.
func (c Client) HostGroupVolumes(ctx context.Context, hostGroupID string) ([]string, error) {
	logFields := LogFields{
		"API":         "GCNV.HostGroupVolumes",
		"hostGroupID": hostGroupID,
	}

	Logc(ctx).WithFields(logFields).Debug("Fetching volumes for host group.")

	// Use a detached context - this query should complete regardless of caller timeout
	queryCtx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	// Get all volumes and filter by host group mapping
	volumes, err := c.Volumes(queryCtx)
	if err != nil {
		return nil, err
	}

	mappedVolumes := []string{}
	for _, vol := range *volumes {
		if len(vol.BlockDevices) > 0 {
			for _, hgID := range vol.BlockDevices[0].HostGroups {
				if hgID == hostGroupID {
					mappedVolumes = append(mappedVolumes, vol.FullName)
					break
				}
			}
		}
	}

	Logc(ctx).WithFields(logFields).WithField("count", len(mappedVolumes)).Debug("Found mapped volumes.")
	return mappedVolumes, nil
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

// IsGCNVDeadlineExceededError checks whether an error returned from the GCNV indicates the deadline was exceeded.
func IsGCNVDeadlineExceededError(err error) bool {
	if err == nil {
		return false
	}

	if s, ok := status.FromError(err); ok && s.Code() == codes.DeadlineExceeded {
		return true
	}

	return false
}

// IsGCNVCanceledError checks whether an error returned from the GCNV indicates the request was canceled.
func IsGCNVCanceledError(err error) bool {
	if err == nil {
		return false
	}

	if s, ok := status.FromError(err); ok && s.Code() == codes.Canceled {
		return true
	}

	return false
}

// IsGCNVTimeoutError checks whether an error returned from the GCNV indicates a timeout, deadline exceeded, or
// context cancellation.  If this function returns true, the caller should not retry the operation.
func IsGCNVTimeoutError(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		IsGCNVDeadlineExceededError(err) ||
		IsGCNVCanceledError(err)
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
