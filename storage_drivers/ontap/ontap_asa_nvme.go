// Copyright 2025 NetApp, Inc. All Rights Reserved.
package ontap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/RoaringBitmap/roaring/v2"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/capacity"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/utils/errors"
	tridenterrors "github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/nvme"
)

// ASANVMeStorageDriver is for NVMe/TCP storage provisioning on ASAr2 systems
type ASANVMeStorageDriver struct {
	initialized bool
	Config      drivers.OntapStorageDriverConfig
	ips         []string
	API         api.OntapAPI
	telemetry   *Telemetry

	physicalPools map[string]storage.Pool
	virtualPools  map[string]storage.Pool

	cloneSplitTimers *sync.Map
}

const (
	asaNVMeSubsystemPrefix = "an"
)

func (d *ASANVMeStorageDriver) GetConfig() drivers.DriverConfig {
	return &d.Config
}

func (d *ASANVMeStorageDriver) GetOntapConfig() *drivers.OntapStorageDriverConfig {
	return &d.Config
}

func (d *ASANVMeStorageDriver) GetAPI() api.OntapAPI {
	return d.API
}

func (d *ASANVMeStorageDriver) GetTelemetry() *Telemetry {
	return d.telemetry
}

// Name is for returning the name of this driver
func (d ASANVMeStorageDriver) Name() string {
	return tridentconfig.OntapSANStorageDriverName
}

// BackendName returns the name of the backend managed by this driver instance
func (d *ASANVMeStorageDriver) BackendName() string {
	if d.Config.BackendName == "" {
		// Use the old naming scheme if no name is specified
		lif0 := "noLIFs"
		if len(d.ips) > 0 {
			lif0 = d.ips[0]
		}
		return CleanBackendName("ontapasanvme_" + lif0)
	} else {
		return d.Config.BackendName
	}
}

// Initialize from the provided config
func (d *ASANVMeStorageDriver) Initialize(
	ctx context.Context, driverContext tridentconfig.DriverContext, configJSON string,
	commonConfig *drivers.CommonStorageDriverConfig, backendSecret map[string]string, backendUUID string,
) error {
	fields := LogFields{"Method": "Initialize", "Type": "ASANVMeStorageDriver"}
	Logd(ctx, commonConfig.StorageDriverName,
		commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Initialize")
	defer Logd(ctx, commonConfig.StorageDriverName,
		commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Initialize")

	var err error

	if d.Config.CommonStorageDriverConfig == nil {

		// Initialize the driver's CommonStorageDriverConfig
		d.Config.CommonStorageDriverConfig = commonConfig

		// Parse the config
		config, err := InitializeOntapConfig(ctx, driverContext, configJSON, commonConfig, backendSecret)
		if err != nil {
			return fmt.Errorf("error initializing %s driver: %w", d.Name(), err)
		}

		d.Config = *config
	}

	// Initialize the ONTAP API.
	// Unit tests mock the API layer, so we only use the real API interface if it doesn't already exist.
	if d.API == nil {
		if d.API, err = InitializeOntapDriver(ctx, &d.Config); err != nil {
			return fmt.Errorf("error initializing %s driver: %w", d.Name(), err)
		}
	}

	// Load default config parameters
	if err = PopulateASAConfigurationDefaults(ctx, &d.Config); err != nil {
		return fmt.Errorf("could not populate configuration defaults: %w", err)
	}

	// Check NVMe feature support
	if !d.API.SupportsFeature(ctx, api.NVMeProtocol) {
		return fmt.Errorf("error initializing %s driver: ONTAP doesn't support NVMe", d.Name())
	}

	d.ips, err = d.API.NetInterfaceGetDataLIFs(ctx, sa.NVMeTransport)
	if err != nil {
		return err
	}

	if len(d.ips) == 0 {
		return fmt.Errorf("no NVMe data LIFs found on SVM %s", d.API.SVMName())
	} else {
		Logc(ctx).WithField("dataLIFs", d.ips).Debug("Found iSCSI LIFs.")
	}

	d.physicalPools, d.virtualPools, err = InitializeManagedStoragePoolsCommon(ctx, d,
		d.getStoragePoolAttributes(ctx), d.BackendName())
	if err != nil {
		return fmt.Errorf("could not configure storage pools: %w", err)
	}

	if err = d.validate(ctx); err != nil {
		return fmt.Errorf("error validating %s driver: %w", d.Name(), err)
	}

	// Identify non-overlapping storage backend pools on the driver backend.
	pools, err := drivers.EncodeStorageBackendPools(ctx, commonConfig, d.getStorageBackendPools(ctx))
	if err != nil {
		return fmt.Errorf("failed to encode storage backend pools: %w", err)
	}
	d.Config.BackendPools = pools

	// Set up the autosupport heartbeat
	d.telemetry = NewOntapTelemetry(ctx, d)
	d.telemetry.Telemetry = tridentconfig.OrchestratorTelemetry
	d.telemetry.TridentBackendUUID = backendUUID
	d.telemetry.Start(ctx)

	// Set up the clone split timers
	d.cloneSplitTimers = &sync.Map{}

	Logc(ctx).Debug("Initialized All-SAN Array NVMe/TCP backend.")

	d.initialized = true

	return nil
}

func (d *ASANVMeStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *ASANVMeStorageDriver) Terminate(ctx context.Context, _ string) {
	fields := LogFields{"Method": "Terminate", "Type": "ASANVMeStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Terminate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Terminate")

	if d.telemetry != nil {
		d.telemetry.Stop()
	}
	d.API.Terminate()
	d.initialized = false
}

// Validate the driver configuration and execution environment
func (d *ASANVMeStorageDriver) validate(ctx context.Context) error {
	fields := LogFields{"Method": "validate", "Type": "ASANVMeStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> validate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< validate")

	if err := ValidateSANDriver(ctx, &d.Config, d.ips, nil); err != nil {
		return fmt.Errorf("driver validation failed: %v", err)
	}

	if err := ValidateStoragePrefix(*d.Config.StoragePrefix); err != nil {
		return err
	}

	if err := ValidateASAStoragePools(ctx, d.physicalPools, d.virtualPools, d, api.MaxSANLabelLength); err != nil {
		return fmt.Errorf("storage pool validation failed: %w", err)
	}

	return nil
}

// Create a Namespace with the specified options
func (d *ASANVMeStorageDriver) Create(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool storage.Pool, volAttributes map[string]sa.Request,
) error {
	name := volConfig.InternalName

	var fstype string

	fields := LogFields{
		"Method": "Create",
		"Type":   "ASANVMeStorageDriver",
		"name":   name,
		"attrs":  volAttributes,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Create")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Create")

	// Early exit if Namespace already exists.
	namespaceExists, err := d.API.NVMeNamespaceExists(ctx, name)
	if err != nil {
		return fmt.Errorf("failure checking for existence of NVMe namespace %v: %w", name, err)
	}

	if namespaceExists {
		return drivers.NewVolumeExistsError(name)
	}

	// Get options
	opts := d.GetVolumeOpts(ctx, volConfig, volAttributes)

	// Get options with default fallback values
	// see also: ontap_common.go#PopulateConfigurationDefaults
	var (
		spaceAllocation   = collection.GetV(opts, "spaceAllocation", storagePool.InternalAttributes()[SpaceAllocation])
		spaceReserve      = collection.GetV(opts, "spaceReserve", storagePool.InternalAttributes()[SpaceReserve])
		snapshotPolicy    = collection.GetV(opts, "snapshotPolicy", storagePool.InternalAttributes()[SnapshotPolicy])
		snapshotReserve   = collection.GetV(opts, "snapshotReserve", storagePool.InternalAttributes()[SnapshotReserve])
		snapshotDir       = "false"
		unixPermissions   = collection.GetV(opts, "unixPermissions", storagePool.InternalAttributes()[UnixPermissions])
		exportPolicy      = collection.GetV(opts, "exportPolicy", storagePool.InternalAttributes()[ExportPolicy])
		securityStyle     = collection.GetV(opts, "securityStyle", storagePool.InternalAttributes()[SecurityStyle])
		encryption        = collection.GetV(opts, "encryption", storagePool.InternalAttributes()[Encryption])
		tieringPolicy     = collection.GetV(opts, "tieringPolicy", storagePool.InternalAttributes()[TieringPolicy])
		skipRecoveryQueue = collection.GetV(opts, "skipRecoveryQueue", storagePool.InternalAttributes()[SkipRecoveryQueue])
		formatOptions     = collection.GetV(opts, "formatOptions", storagePool.InternalAttributes()[FormatOptions])
		qosPolicy         = storagePool.InternalAttributes()[QosPolicy]
		adaptiveQosPolicy = storagePool.InternalAttributes()[AdaptiveQosPolicy]
		luksEncryption    = storagePool.InternalAttributes()[LUKSEncryption]
	)

	// Validate overridden values from the volume config
	if spaceAllocation != DefaultSpaceAllocation {
		return fmt.Errorf("spaceAllocation must be set to %s", DefaultSpaceAllocation)
	}
	if spaceReserve != DefaultSpaceReserve {
		return fmt.Errorf("spaceReserve must be set to %s", DefaultSpaceReserve)
	}
	if snapshotPolicy != DefaultSnapshotPolicy {
		return fmt.Errorf("snapshotPolicy must be set to %s", DefaultSnapshotPolicy)
	}
	if snapshotReserve != "" {
		return errors.New("snapshotReserve must not be set")
	}
	if unixPermissions != "" {
		return errors.New("unixPermissions must not be set")
	}
	if exportPolicy != "" {
		return errors.New("exportPolicy must not be set")
	}
	if securityStyle != "" {
		return errors.New("securityStyle must not be set")
	}
	if encryption != DefaultASAEncryption {
		return fmt.Errorf("encryption must be set to %s", DefaultASAEncryption)
	}
	if tieringPolicy != DefaultTieringPolicy {
		return errors.New("tieringPolicy must not be set")
	}
	if luksEncryption != DefaultLuksEncryption {
		return fmt.Errorf("luksEncryption must be set to %s", DefaultLuksEncryption)
	}

	if _, err = strconv.ParseBool(skipRecoveryQueue); skipRecoveryQueue != "" && err != nil {
		return fmt.Errorf("invalid boolean value for skipRecoveryQueue: %w", err)
	}

	// Determine namespace size in bytes
	requestedSize, err := capacity.ToBytes(volConfig.Size)
	if err != nil {
		return fmt.Errorf("could not convert size %s: %v", volConfig.Size, err)
	}
	requestedSizeBytes, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid size: %v", volConfig.Size, err)
	}

	// Apply minimum size rule
	namespaceSizeBytes := GetVolumeSize(requestedSizeBytes, storagePool.InternalAttributes()[Size])
	namespaceSize := strconv.FormatUint(namespaceSizeBytes, 10)

	// Apply maximum size rule
	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, namespaceSizeBytes, d.Config.CommonStorageDriverConfig,
	); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	fstype, err = drivers.CheckSupportedFilesystem(
		ctx, collection.GetV(opts, "fstype|fileSystemType", storagePool.InternalAttributes()[FileSystemType]), name)
	if err != nil {
		return err
	}

	if tieringPolicy == "" {
		tieringPolicy = d.API.TieringPolicyValue(ctx)
	}

	qosPolicyGroup, err := api.NewQosPolicyGroup(qosPolicy, adaptiveQosPolicy)
	if err != nil {
		return err
	}

	// Update config to reflect values used to create volume
	volConfig.Size = strconv.FormatUint(namespaceSizeBytes, 10)
	volConfig.SpaceReserve = spaceReserve
	volConfig.SnapshotPolicy = snapshotPolicy
	volConfig.SnapshotReserve = snapshotReserve
	volConfig.SnapshotDir = snapshotDir
	volConfig.UnixPermissions = unixPermissions
	volConfig.ExportPolicy = exportPolicy
	volConfig.SecurityStyle = securityStyle
	volConfig.Encryption = encryption
	volConfig.SkipRecoveryQueue = skipRecoveryQueue
	volConfig.FormatOptions = formatOptions
	volConfig.QosPolicy = qosPolicy
	volConfig.AdaptiveQosPolicy = adaptiveQosPolicy
	volConfig.LUKSEncryption = luksEncryption
	volConfig.FileSystem = fstype

	// Make comment field from labels
	nsCommentString, err := d.createNSCommentWithMetadata(ctx, volConfig, storagePool)
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"name":              name,
		"namespaceSize":     namespaceSize,
		"spaceAllocation":   spaceAllocation,
		"spaceReserve":      spaceReserve,
		"snapshotPolicy":    snapshotPolicy,
		"snapshotDir":       snapshotDir,
		"unixPermissions":   unixPermissions,
		"exportPolicy":      exportPolicy,
		"securityStyle":     securityStyle,
		"LUKSEncryption":    luksEncryption,
		"encryption":        encryption,
		"tieringPolicy":     tieringPolicy,
		"skipRecoveryQueue": skipRecoveryQueue,
		"formatOptions":     formatOptions,
		"qosPolicy":         qosPolicy,
		"adaptiveQosPolicy": adaptiveQosPolicy,
		"qosPolicyGroup":    qosPolicyGroup,
		"nsComments":        nsCommentString,
	}).Debug("Creating ASA NVMe namespace.")

	err = d.API.NVMeNamespaceCreate(
		ctx, api.NVMeNamespace{
			Name:      name,
			Size:      namespaceSize,
			QosPolicy: qosPolicyGroup,
			OsType:    "linux",
			BlockSize: defaultNamespaceBlockSize,
			Comment:   nsCommentString,
		})
	if err != nil {
		errMessage := fmt.Sprintf("ONTAP-NVMe pool %s; error creating NVMe Namespace %s: %v", storagePool.Name(), name, err)
		Logc(ctx).Error(errMessage)
		return err
	}

	// Get the newly created namespace and save the UUID
	newNamespace, err := d.API.NVMeNamespaceGetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("failure fetching newly created NVMe namespace %v: %w", name, err)
	}

	if newNamespace == nil {
		return fmt.Errorf("newly create NVMe namespace %s not found", name)
	}

	// Store the Namespace UUID and Namespace Path for future operations.
	volConfig.AccessInfo.NVMeNamespaceUUID = newNamespace.UUID
	volConfig.InternalID = d.CreateASANVMeNamespaceInternalID(d.Config.SVM, name)

	Logc(ctx).WithFields(LogFields{
		"name":          name,
		"namespaceUUID": volConfig.AccessInfo.NVMeNamespaceUUID,
		"internalName":  volConfig.InternalName,
		"internalID":    volConfig.InternalID,
	}).Debug("Created ASA NVMe namespace.")

	return nil
}

// CreateClone creates a volume clone
func (d *ASANVMeStorageDriver) CreateClone(
	ctx context.Context, _, cloneVolConfig *storage.VolumeConfig, storagePool storage.Pool,
) error {
	name := cloneVolConfig.InternalName
	source := cloneVolConfig.CloneSourceVolumeInternal
	snapshot := cloneVolConfig.CloneSourceSnapshotInternal

	fields := LogFields{
		"Method":      "CreateClone",
		"Type":        "ASANVMeStorageDriver",
		"name":        name,
		"source":      source,
		"snapshot":    snapshot,
		"storagePool": storagePool,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateClone")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateClone")

	opts := d.GetVolumeOpts(ctx, cloneVolConfig, make(map[string]sa.Request))

	// How "splitOnClone" value gets set:
	// In the Core we first check clone's VolumeConfig for splitOnClone value
	// If it is not set then (again in Core) we check source PV's VolumeConfig for splitOnClone value
	// If we still don't have splitOnClone value then HERE we check for value in the source PV's Storage/Virtual Pool
	// If the value for "splitOnClone" is still empty then HERE we set it to backend config's SplitOnClone value

	// Attempt to get splitOnClone value based on storagePool (source Volume's StoragePool)
	var storagePoolSplitOnCloneVal string

	// Ensure that the source NVMe namespace exists
	sourceNamespace, err := d.API.NVMeNamespaceGetByName(ctx, cloneVolConfig.CloneSourceVolumeInternal)
	if err != nil {
		return err
	} else if sourceNamespace == nil {
		return tridenterrors.NotFoundError("source NVMe namespace %s not found", cloneVolConfig.CloneSourceVolumeInternal)
	}

	// Set the value for split on clone based on storagePool
	if !storage.IsStoragePoolUnset(storagePool) {
		storagePoolSplitOnCloneVal = storagePool.InternalAttributes()[SplitOnClone]
	}

	// If storagePoolSplitOnCloneVal is still unknown, set it to backend's default value
	if storagePoolSplitOnCloneVal == "" {
		storagePoolSplitOnCloneVal = d.Config.SplitOnClone
	}

	split, err := strconv.ParseBool(collection.GetV(opts, "splitOnClone", storagePoolSplitOnCloneVal))
	if err != nil {
		return fmt.Errorf("invalid boolean value for splitOnClone: %v", err)
	}

	qosPolicy := collection.GetV(opts, "qosPolicy", "")
	adaptiveQosPolicy := collection.GetV(opts, "adaptiveQosPolicy", "")
	qosPolicyGroup, err := api.NewQosPolicyGroup(qosPolicy, adaptiveQosPolicy)
	if err != nil {
		return err
	}

	// Construct comment including both metadata and labels for the clone based on the cloneVolConfig
	labels, labelErr := d.createNSCommentWithMetadata(ctx, cloneVolConfig, storagePool)
	if labelErr != nil {
		return fmt.Errorf("failed to create clone ASA NVMe namespace %v: %w", name, err)
	}

	// Clone the ASA NVMe namespace
	Logc(ctx).WithField("splitOnClone", split).Debug("Creating ASA NVMe namespace clone.")
	if err = cloneASAvol(ctx, cloneVolConfig, split, &d.Config, d.API); err != nil {
		return err
	}

	// Set labels if present
	if labels != "" {
		err = d.API.NVMeNamespaceSetComment(ctx, name, labels)
		if err != nil {
			return fmt.Errorf("error setting labels on the ASA NVMe namespace: %v", err)
		}
	}

	// Set QoS policy group if present
	if qosPolicyGroup.Kind != api.InvalidQosPolicyGroupKind {
		err = d.API.NVMeNamespaceSetQosPolicyGroup(ctx, name, qosPolicyGroup)
		if err != nil {
			return fmt.Errorf("error setting QoS policy group: %v", err)
		}
	}

	// Get the cloned namespace
	ns, err := d.API.NVMeNamespaceGetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("error fetching namespace %v; error:%v", name, err)
	}

	// Populate access info in the cloneVolConfig
	cloneVolConfig.AccessInfo.NVMeNamespaceUUID = ns.UUID
	cloneVolConfig.InternalID = d.CreateASANVMeNamespaceInternalID(d.Config.SVM, ns.Name)

	return nil
}

// Import adds non managed ONTAP volume to trident.
func (d *ASANVMeStorageDriver) Import(ctx context.Context, volConfig *storage.VolumeConfig, originalName string) error {
	fields := LogFields{
		"Method":       "Import",
		"Type":         "ASANVMeStorageDriver",
		"originalName": originalName,
		"newName":      volConfig.InternalName,
		"notManaged":   volConfig.ImportNotManaged,
		"noRename":     volConfig.ImportNoRename,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Import")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Import")

	// Ensure that the Namespace exists
	nsInfo, err := d.API.NVMeNamespaceGetByName(ctx, originalName)
	if err != nil {
		return err
	} else if nsInfo == nil {
		return tridenterrors.NotFoundError("ASA NVMe namespace %s not found", originalName)
	}

	// Get flexvol corresponding to the Namespace and ensure it is "rw" volume
	flexvol, err := d.API.VolumeInfo(ctx, originalName)
	if err != nil {
		return err
	} else if flexvol == nil {
		return tridenterrors.NotFoundError("ASA NVMe volume %s not found", originalName)
	}

	if flexvol.AccessType != "rw" {
		Logc(ctx).WithField("originalName", originalName).Error("Could not import ASA NVMe namespace, type is not rw.")
		return fmt.Errorf("NVMe namepsace %s type is %s, not rw", originalName, flexvol.AccessType)
	}

	// Set the volume's LUKS encryption from backend's default
	if volConfig.LUKSEncryption == "" {
		volConfig.LUKSEncryption = d.Config.LUKSEncryption
	}

	// Set the filesystem type to backend's, if it hasn't been set via annotation
	// in the provided pvc during import.
	if volConfig.FileSystem == "" {
		volConfig.FileSystem = d.Config.FileSystemType
	}

	// The Namespace should be online
	if nsInfo.State != "online" {
		return fmt.Errorf("ASA NVMe namespace %s is not online", nsInfo.Name)
	}

	// The Namespace should not be mapped to any subsystem
	nsMapped, err := d.API.NVMeIsNamespaceMapped(ctx, "", nsInfo.UUID)
	if err != nil {
		return err
	} else if nsMapped {
		return fmt.Errorf("ASA NVMe namespace %s is mapped to a subsystem", nsInfo.Name)
	}

	// Use the Namespace size
	volConfig.Size = nsInfo.Size

	// Rename the Namespace if Trident will manage its lifecycle and the names are different
	if !volConfig.ImportNotManaged && !volConfig.ImportNoRename {
		// Rename the namespace
		err = d.API.NVMeNamespaceRename(ctx, nsInfo.UUID, volConfig.InternalName)
		if err != nil {
			Logc(ctx).WithField("originalName", originalName).Errorf(
				"Could not import ASA NVMe namespace, rename of ASA NVMe namespace failed: %w.", err)
			return fmt.Errorf("ASA NVMe namespace %s rename failed: %w", originalName, err)
		}
	}

	// Update namespace comments if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		// Create comment for the namespace based on the source Namespace
		nsCommentString, commentErr := d.createNSCommentBasedOnSourceNS(ctx, volConfig, nsInfo, nil)
		if commentErr != nil {
			Logc(ctx).WithFields(fields).WithError(err).Error("Failed to import ASA NVMe namespace as failed to generate comment.")
			return fmt.Errorf("ASA NVMe namespace %s import failed: %w", originalName, commentErr)
		}

		// Set comment on the namespace if needed
		if nsCommentString != "" {
			err = d.API.NVMeNamespaceSetComment(ctx, volConfig.InternalName, nsCommentString)
			if err != nil {
				Logc(ctx).WithFields(fields).WithError(err).Error("Failed to import ASA NVMe namespace as failed to modify comment.")
				return fmt.Errorf("ASA NVMe namespace %s import failed: %w", originalName, err)
			}
		}
	}

	volConfig.InternalID = d.CreateASANVMeNamespaceInternalID(d.Config.SVM, nsInfo.Name)
	volConfig.AccessInfo.NVMeNamespaceUUID = nsInfo.UUID

	return nil
}

// createNSCommentBasedOnSourceNS creates a comment string for ASA NVMe namespace based on comments on source namespace
// It checks if the source namespace's comment has been set by Trident. If so, it creates a new comment by combining
// metadata with new values provided in the config. If the comment is not set by Trident in the source namespace, it
// does not create new comments and returns empty string.
// Sample comment set by Trident (metadata + label):
// {"nsAttribute":{"LUKS":"false","com.netapp.ndvp.fstype":"ext4","driverContext":"csi",
// "formatOptions":"-E nodiscard","nsLabels":"{\"provisioning\":{\"Cluster\":\"my_cluster_new\",\"team\":\"abc\"}}"}}
func (d *ASANVMeStorageDriver) createNSCommentBasedOnSourceNS(
	ctx context.Context, volConfig *storage.VolumeConfig,
	sourceNs *api.NVMeNamespace, storagePool storage.Pool,
) (string, error) {
	fields := LogFields{
		"Method":           "createNSCommentBasedOnSourceNS",
		"Type":             "ASANVMeStorageDriver",
		"name":             volConfig.InternalName,
		"sourceNSName":     sourceNs.Name,
		"sourceNSComments": sourceNs.Comment,
		"storgePool":       storagePool,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> createNSCommentBasedOnSourceNS")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< createNSCommentBasedOnSourceNS")

	commentFields := LogFields{
		"name":            volConfig.InternalName,
		"sourceNSName":    sourceNs.Name,
		"sourceNSComment": sourceNs.Comment,
	}

	// If the source namespace comment is not empty, check if it is of format as that set by Trident.
	// If yes, then we can overwrite it with new values provided in the config.
	if sourceNs.Comment != "" {
		poolLabelOverwrite := false

		// Parse the source namespace comment
		nsAttrs, err := d.ParseNVMeNamespaceCommentString(ctx, sourceNs.Comment)
		if err != nil {
			// Existing comment seems not to be set by Trident. Ignore it.
			Logc(ctx).WithFields(commentFields).Warnf(
				"Failed to parse ASA NVMe namespace comment; it doesn't seem to be set by Trident, thus ignoring it: %w.", err)
			return "", nil
		} else {
			// Existing comment is set by Trident. Check if we can overwrite the labels.
			poolLabelOverwrite = storage.AllowPoolLabelOverwrite(storage.ProvisioningLabelTag, nsAttrs[nsLabels])
			Logc(ctx).WithFields(commentFields).Infof("Decided to overwrite pool label based on source ASA NVMe Namespace: %v.", poolLabelOverwrite)
		}

		// If we cannot overwrite pool label ('provisioning' key is missing from comment json), then ignore the existing comment
		if !poolLabelOverwrite {
			Logc(ctx).WithFields(commentFields).Debug("Using existing ASA NVMe namespace comment.")
			return "", nil
		}
	}

	// If the source namespace does not have any existing comment, or we can overwrite pool label, then recreate the labels
	nsCommentString, err := d.createNSCommentWithMetadata(ctx, volConfig, storagePool)
	if err != nil {
		Logc(ctx).WithFields(commentFields).WithError(err).Error("Failed to create ASA NVMe namespace comment from source ASA NVMe namespace.")
		return "", fmt.Errorf("failed to create ASA NVMe namespace comment: %w", err)
	}

	return nsCommentString, nil
}

// createNSCommentWithMetadata creates a comment string combining metadata and labels
func (d *ASANVMeStorageDriver) createNSCommentWithMetadata(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool storage.Pool,
) (string, error) {
	fields := LogFields{
		"Method":     "createNSCommentWithMetadata",
		"Type":       "ASANVMeStorageDriver",
		"name":       volConfig.InternalName,
		"storgePool": storagePool,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> createNSCommentWithMetadata")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< createNSCommentWithMetadata")

	var labels string
	var labelErr error

	if storage.IsStoragePoolUnset(storagePool) {
		// Create a temporary storage pool
		storagePoolTemp := ConstructPoolForLabels(d.Config.NameTemplate, d.Config.Labels)

		// Create the base label
		if labels, labelErr = ConstructLabelsFromConfigs(ctx, storagePoolTemp, volConfig,
			d.Config.CommonStorageDriverConfig, api.MaxSANLabelLength); labelErr != nil {
			return "", labelErr
		}
	} else {
		// Create the base label

		if labels, labelErr = ConstructLabelsFromConfigs(ctx, storagePool, volConfig,
			d.Config.CommonStorageDriverConfig, api.MaxSANLabelLength); labelErr != nil {
			return "", labelErr
		}
	}

	// Create the namespace comment combining the metadata and labels
	nsComment := map[string]string{
		nsAttributeFSType:    volConfig.FileSystem,
		nsAttributeLUKS:      volConfig.LUKSEncryption,
		nsAttributeDriverCtx: string(d.Config.DriverContext),
		FormatOptions:        volConfig.FormatOptions,
		nsLabels:             labels,
	}

	comment, err := d.createNVMeNamespaceCommentString(ctx, nsComment, nsMaxCommentLength)

	commentFields := LogFields{
		"name":    volConfig.InternalName,
		"comment": comment,
	}
	Logc(ctx).WithFields(commentFields).Debug("Successfully created ASA NVMe namespace comment with both metadata and labels.")

	return comment, err
}

func (d *ASANVMeStorageDriver) Rename(ctx context.Context, name, newName string) error {
	fields := LogFields{
		"Method":  "Rename",
		"Type":    "ASANVMeStorageDriver",
		"name":    name,
		"newName": newName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Rename")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Rename")

	// Get the NVMe namespace
	ns, err := d.API.NVMeNamespaceGetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("error getting ASA NVMe namespace %s: %w", name, err)
	}

	err = d.API.NVMeNamespaceRename(ctx, ns.UUID, newName)
	if err != nil {
		Logc(ctx).WithField("name", name).WithError(err).Error("Could not rename ASA NVMe namespace")
		return fmt.Errorf("could not rename ASA NVMe namespace %s: %w", name, err)
	}

	return nil
}

// Destroy the requested NVMe namespace
func (d *ASANVMeStorageDriver) Destroy(ctx context.Context, volConfig *storage.VolumeConfig) error {
	name := volConfig.InternalName

	fields := LogFields{
		"Method": "Destroy",
		"Type":   "ASANVMeStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Destroy")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Destroy")

	// Validate NVMe namespace exists before trying to destroy
	volExists, err := d.API.NVMeNamespaceExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing ASA NVMe namespace %v: %w", name, err)
	}
	if !volExists {
		Logc(ctx).WithField("name", name).Debug("ASA NVMe namespace already deleted, skipping destroy.")
		return nil
	}

	defer func() {
		deleteAutomaticASASnapshot(ctx, &d.Config, d.API, volConfig)
	}()

	if d.Config.DriverContext == tridentconfig.ContextDocker {
		Logc(ctx).Debug("No actions for Destroy for Docker.")
	}

	// skipRecoveryQueue is not supported yet. Log it and continue.
	if skipRecoveryQueueValue, err := strconv.ParseBool(volConfig.SkipRecoveryQueue); err == nil {
		Logc(ctx).WithField("skipRecoveryQueue", skipRecoveryQueueValue).Warn(
			"SkipRecoveryQueue is not supported. It will be ignored when deleting ASA NVMe namespace.")
	}

	// Delete the Namespace
	err = d.API.NVMeNamespaceDelete(ctx, name)
	if err != nil {
		return fmt.Errorf("error destroying ASA NVMe namespace %v: %w", name, err)
	}

	return nil
}

// Publish prepares the volume to attach/mount it to the pod.
func (d *ASANVMeStorageDriver) Publish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *models.VolumePublishInfo,
) error {
	// InternalName is of the format <storagePrefix><pvc-UUID>
	name := volConfig.InternalName
	// volConfig.Name is same as <pvc-UUID>
	pvName := volConfig.Name

	fields := LogFields{
		"method": "Publish",
		"type":   "ASANVMeStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Publish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Publish")

	// Check if the volume is DP or RW and don't publish if DP
	volIsRW, err := isFlexvolRW(ctx, d.GetAPI(), name)
	if err != nil {
		return err
	}
	if !volIsRW {
		return errors.UnsupportedError(fmt.Sprintf("the volume %v is not enabled for read or writes", name))
	}

	if publishInfo.Localhost {
		// Get its HostNQN and populate it in publishInfo.
		nvmeHandler := nvme.NewNVMeHandler()
		nqn, err := nvmeHandler.GetHostNqn(ctx)
		if err != nil {
			return err
		}
		publishInfo.HostNQN = nqn
	}

	// For Docker context fetch the metadata fields from the NVMe namespace comment
	if d.Config.DriverContext == tridentconfig.ContextDocker {
		ns, err := d.API.NVMeNamespaceGetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("problem fetching ASA NVMe namespace %v; %w", name, err)
		}
		if ns == nil {
			return fmt.Errorf("ASA NVMe namespace %v not found", name)
		}
		nsAttrs, err := d.ParseNVMeNamespaceCommentString(ctx, ns.Comment)
		if err != nil {
			return fmt.Errorf("failed to parse ASA NVMe namespace %v comment %v; %w", name, ns.Comment, err)
		}
		publishInfo.FilesystemType = nsAttrs[nsAttributeFSType]
		publishInfo.LUKSEncryption = nsAttrs[nsAttributeLUKS]
		publishInfo.FormatOptions = nsAttrs[FormatOptions]
	} else {
		publishInfo.FilesystemType = volConfig.FileSystem
		publishInfo.LUKSEncryption = volConfig.LUKSEncryption
		publishInfo.FormatOptions = volConfig.FormatOptions
	}

	// Get host nqn
	if publishInfo.HostNQN == "" {
		Logc(ctx).Error("Host NQN is empty")
		return fmt.Errorf("hostNQN not found")
	} else {
		Logc(ctx).Debug("Host NQN is ", publishInfo.HostNQN)
	}

	// When FS type is RAW, we create a new subsystem per namespace,
	// else we use the subsystem created for that particular node
	var ssName string
	var completeSSName string

	if volConfig.FileSystem == filesystem.Raw {
		ssName = getNamespaceSpecificSubsystemName(name, pvName)
		completeSSName = ssName
	} else {
		// Create a unique node specific subsystem name. Use prefix to indicate ASA r2 NVMe.
		completeSSName, ssName, err = getUniqueNodeSpecificSubsystemName(
			publishInfo.HostName, publishInfo.TridentUUID, asaNVMeSubsystemPrefix, maximumSubsystemNameLength)
		if err != nil {
			return fmt.Errorf("failed to create node specific subsystem name: %w", err)
		}
	}

	logFields := LogFields{
		"filesystemType":        volConfig.FileSystem,
		"hostName":              publishInfo.HostName,
		"subsystemName":         ssName,
		"completeSubsystemName": completeSSName,
	}
	Logc(ctx).WithFields(logFields).Debug("Successfully generated subsystem name for publish.")

	// Checks if subsystem exists and creates a new one if not
	subsystem, err := d.API.NVMeSubsystemCreate(ctx, ssName, completeSSName)
	if err != nil {
		Logc(ctx).Errorf("subsystem create failed, %w", err)
		return err
	}

	if subsystem == nil {
		return fmt.Errorf("no subsystem returned after subsystem create")
	}

	// Fill important info in publishInfo
	nsUUID := volConfig.AccessInfo.NVMeNamespaceUUID
	publishInfo.NVMeSubsystemNQN = subsystem.NQN
	publishInfo.NVMeSubsystemUUID = subsystem.UUID
	publishInfo.NVMeNamespaceUUID = nsUUID
	publishInfo.SANType = d.Config.SANType

	// Add HostNQN to the subsystem using api call
	if err := d.API.NVMeAddHostToSubsystem(ctx, publishInfo.HostNQN, subsystem.UUID); err != nil {
		Logc(ctx).Errorf("Add host to subsystem failed; %w.", err)
		return err
	}

	if err := d.API.NVMeEnsureNamespaceMapped(ctx, subsystem.UUID, nsUUID); err != nil {
		return err
	}

	publishInfo.VolumeAccessInfo.NVMeTargetIPs = d.ips

	// xfs volumes are always mounted with '-o nouuid' to allow clones to be mounted to the same node as the source
	if publishInfo.FilesystemType == filesystem.Xfs {
		publishInfo.MountOptions = drivers.EnsureMountOption(publishInfo.MountOptions, drivers.MountOptionNoUUID)
	}

	// Fill in the volume config fields as well
	volConfig.AccessInfo = publishInfo.VolumeAccessInfo

	return nil
}

// Unpublish removes the attach publication of the volume.
func (d *ASANVMeStorageDriver) Unpublish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *models.VolumePublishInfo,
) error {
	name := volConfig.InternalName

	fields := LogFields{
		"method":            "Unpublish",
		"type":              "ASANVMeStorageDriver",
		"name":              name,
		"NVMeNamespaceUUID": volConfig.AccessInfo.NVMeNamespaceUUID,
		"NVMeSubsystemUUID": volConfig.AccessInfo.NVMeSubsystemUUID,
		"hostNQN":           publishInfo.HostNQN,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Unpublish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Unpublish")

	subsystemUUID := volConfig.AccessInfo.NVMeSubsystemUUID
	namespaceUUID := volConfig.AccessInfo.NVMeNamespaceUUID

	removePublishInfo, err := d.API.NVMeEnsureNamespaceUnmapped(ctx, publishInfo.HostNQN, subsystemUUID, namespaceUUID)
	if removePublishInfo {
		volConfig.AccessInfo.NVMeTargetIPs = []string{}
		volConfig.AccessInfo.NVMeSubsystemNQN = ""
		volConfig.AccessInfo.NVMeSubsystemUUID = ""
	}
	return err
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (d *ASANVMeStorageDriver) CanSnapshot(_ context.Context, _ *storage.SnapshotConfig, _ *storage.VolumeConfig) error {
	return nil
}

// GetSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func (d *ASANVMeStorageDriver) GetSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	fields := LogFields{
		"Method":       "GetSnapshot",
		"Type":         "ASANVMeStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshot")

	return getASASnapshot(ctx, snapConfig, &d.Config, d.API)
}

// GetSnapshots returns the list of snapshots associated with the specified volume
func (d *ASANVMeStorageDriver) GetSnapshots(
	ctx context.Context, volConfig *storage.VolumeConfig,
) ([]*storage.Snapshot, error) {
	fields := LogFields{
		"Method":     "GetSnapshots",
		"Type":       "ASANVMeStorageDriver",
		"volumeName": volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshots")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshots")

	return getASASnapshotList(ctx, volConfig, &d.Config, d.API)
}

// CreateSnapshot creates a snapshot for the given volume
func (d *ASANVMeStorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "CreateSnapshot",
		"Type":         "ASANVMeStorageDriver",
		"snapshotName": internalSnapName,
		"sourceVolume": internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateSnapshot")

	return createASASnapshot(ctx, snapConfig, &d.Config, d.API)
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *ASANVMeStorageDriver) RestoreSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	fields := LogFields{
		"Method":       "RestoreSnapshot",
		"Type":         "ASANVMeStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> RestoreSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< RestoreSnapshot")

	return restoreASASnapshot(ctx, snapConfig, &d.Config, d.API)
}

// DeleteSnapshot deletes a snapshot of a volume.
func (d *ASANVMeStorageDriver) DeleteSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	fields := LogFields{
		"Method":       "DeleteSnapshot",
		"Type":         "ASANVMeStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> DeleteSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< DeleteSnapshot")

	err := deleteASASnapshot(ctx, snapConfig, &d.Config, d.API, d.cloneSplitTimers)
	if err != nil {
		Logc(ctx).WithField("snapshotName", snapConfig.InternalName).WithError(err).Error("Error deleting snapshot.")
		return err
	}

	Logc(ctx).WithField("snapshotName", snapConfig.InternalName).Debug("Deleted snapshot.")
	return nil
}

// Get tests for the existence of a volume
func (d *ASANVMeStorageDriver) Get(ctx context.Context, name string) error {
	fields := LogFields{"Method": "Get", "Type": "ASANVMeStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Get")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Get")

	exists, err := d.API.NVMeNamespaceExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing ASA NVMe namespace %v: %w", name, err)
	}
	if !exists {
		Logc(ctx).WithField("name", name).Debug("ASA NVMe namespace not found.")
		return tridenterrors.NotFoundError("ASA NVMe namespace %s does not exist", name)
	}

	return nil
}

// GetStorageBackendSpecs retrieves storage backend capabilities
func (d *ASANVMeStorageDriver) GetStorageBackendSpecs(_ context.Context, backend storage.Backend) error {
	return getStorageBackendSpecsCommon(backend, d.physicalPools, d.virtualPools, d.BackendName())
}

// GetStorageBackendPhysicalPoolNames retrieves storage backend physical pools
func (d *ASANVMeStorageDriver) GetStorageBackendPhysicalPoolNames(context.Context) []string {
	return getStorageBackendPhysicalPoolNamesCommon(d.physicalPools)
}

// getStorageBackendPools determines any non-overlapping, discrete storage pools present on a driver's storage backend.
func (d *ASANVMeStorageDriver) getStorageBackendPools(ctx context.Context) []drivers.OntapStorageBackendPool {
	fields := LogFields{"Method": "getStorageBackendPools", "Type": "ASANVMeStorageDriver"}
	Logc(ctx).WithFields(fields).Debug(">>>> getStorageBackendPools")
	defer Logc(ctx).WithFields(fields).Debug("<<<< getStorageBackendPools")

	// For this driver, a discrete storage pool is composed of the following:
	// 1. SVM UUID
	// ASA backends will only report 1 storage pool.
	svmUUID := d.GetAPI().GetSVMUUID()
	return []drivers.OntapStorageBackendPool{{SvmUUID: svmUUID}}
}

func (d *ASANVMeStorageDriver) getStoragePoolAttributes(_ context.Context) map[string]sa.Offer {
	return map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(d.Name()),
		sa.Snapshots:        sa.NewBoolOffer(true),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.Encryption:       sa.NewBoolOffer(true),
		sa.Replication:      sa.NewBoolOffer(false),
		sa.ProvisioningType: sa.NewStringOffer("thin"),
		sa.Media:            sa.NewStringOffer(sa.SSD),
	}
}

func (d *ASANVMeStorageDriver) GetVolumeOpts(
	ctx context.Context, volConfig *storage.VolumeConfig, requests map[string]sa.Request,
) map[string]string {
	return getVolumeOptsCommon(ctx, volConfig, requests)
}

func (d *ASANVMeStorageDriver) GetInternalVolumeName(
	ctx context.Context, volConfig *storage.VolumeConfig, pool storage.Pool,
) string {
	return getInternalVolumeNameCommon(ctx, &d.Config, volConfig, pool)
}

func (d *ASANVMeStorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig, pool storage.Pool) {
	if !volConfig.ImportNotManaged && tridentconfig.CurrentDriverContext == tridentconfig.ContextCSI {
		// All new CSI ONTAP SAN volumes start with publish enforcement on, unless they're unmanaged imports
		volConfig.AccessInfo.PublishEnforcement = true
	}

	// If no pool is specified, a new pool is created and assigned a name template and label from the common configuration.
	// The process of generating a custom volume name necessitates a name template and label.
	if storage.IsStoragePoolUnset(pool) {
		pool = ConstructPoolForLabels(d.Config.NameTemplate, d.Config.Labels)
	}
	createPrepareCommon(ctx, d, volConfig, pool)
}

func (d *ASANVMeStorageDriver) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {
	fields := LogFields{
		"Method":       "CreateFollowup",
		"Type":         "ASANVMeStorageDriver",
		"name":         volConfig.Name,
		"internalName": volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateFollowup")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateFollowup")
	Logc(ctx).Debug("No follow-up create actions for ASA NVMe namespace.")

	return nil
}

func (d *ASANVMeStorageDriver) GetProtocol(context.Context) tridentconfig.Protocol {
	return tridentconfig.Block
}

func (d *ASANVMeStorageDriver) StoreConfig(_ context.Context, b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.OntapConfig = &d.Config
}

func (d *ASANVMeStorageDriver) GetExternalConfig(ctx context.Context) interface{} {
	return getExternalConfig(ctx, d.Config)
}

// GetVolumeForImport queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.  For this driver, volumeID is the name of the
// NVMe namespace (which also is name of the container Flexvol) on the storage system.
func (d *ASANVMeStorageDriver) GetVolumeForImport(ctx context.Context, volumeID string) (*storage.VolumeExternal, error) {
	// TODO (aparna0508): Replace Volume API with Consistency Group APIs when available.
	volumeAttrs, err := d.API.VolumeInfo(ctx, volumeID)
	if err != nil {
		return nil, err
	}

	nsName := volumeID
	nsAttrs, err := d.API.NVMeNamespaceGetByName(ctx, nsName)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(nsAttrs, volumeAttrs), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *ASANVMeStorageDriver) GetVolumeExternalWrappers(ctx context.Context, channel chan *storage.VolumeExternalWrapper) {
	// Let the caller know we're done by closing the channel
	defer close(channel)

	// TODO (aparna0508): Replace volume API with Consistency Group API when available to fetch Snapshot Policy
	// Get all volumes matching the storage prefix
	volumes, err := d.API.VolumeListByPrefix(ctx, *d.Config.StoragePrefix)
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Get all namespaces matching the storage prefix.
	nsPathPattern := *d.Config.StoragePrefix + "*"
	namespaces, err := d.API.NVMeNamespaceList(ctx, nsPathPattern)
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Make a map of volumes for faster correlation with namespaces.
	volumeMap := make(map[string]api.Volume)
	if volumes != nil {
		for _, volumeAttrs := range volumes {
			internalName := volumeAttrs.Name
			volumeMap[internalName] = *volumeAttrs
		}
	}

	// Convert all namespaces to VolumeExternal and write them to the channel.
	if namespaces != nil {
		for idx := range namespaces {
			ns := namespaces[idx]
			volume, ok := volumeMap[ns.VolumeName]
			if !ok {
				Logc(ctx).WithField("path", ns.Name).Warning("FlexVol not found for namespace.")
				continue
			}

			channel <- &storage.VolumeExternalWrapper{Volume: d.getVolumeExternal(ns, &volume), Error: nil}
		}
	}
}

// getVolumeExternal is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *ASANVMeStorageDriver) getVolumeExternal(
	ns *api.NVMeNamespace, volume *api.Volume,
) *storage.VolumeExternal {
	internalName := volume.Name
	name := internalName
	if strings.HasPrefix(internalName, *d.Config.StoragePrefix) {
		name = internalName[len(*d.Config.StoragePrefix):]
	}

	volumeConfig := &storage.VolumeConfig{
		Version:         tridentconfig.OrchestratorAPIVersion,
		Name:            name,
		InternalName:    internalName,
		Size:            ns.Size,
		Protocol:        tridentconfig.Block,
		SnapshotPolicy:  volume.SnapshotPolicy,
		ExportPolicy:    "",
		SnapshotDir:     "false",
		UnixPermissions: "",
		StorageClass:    "",
		AccessMode:      tridentconfig.ReadWriteOnce,
		AccessInfo:      models.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
	}

	volumeConfig.AccessInfo.NVMeAccessInfo.NVMeNamespaceUUID = ns.UUID
	volumeConfig.InternalID = d.CreateASANVMeNamespaceInternalID(d.Config.SVM, ns.Name)
	pool := drivers.UnsetPool
	if len(volume.Aggregates) > 0 {
		pool = volume.Aggregates[0]
	}
	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   pool,
	}
}

// GetUpdateType returns a bitmap populated with updates to the driver
func (d *ASANVMeStorageDriver) GetUpdateType(_ context.Context, driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*ASANVMeStorageDriver)
	if !ok {
		bitmap.Add(storage.InvalidUpdate)
		return bitmap
	}

	if d.Config.Password != dOrig.Config.Password {
		bitmap.Add(storage.PasswordChange)
	}

	if d.Config.Username != dOrig.Config.Username {
		bitmap.Add(storage.UsernameChange)
	}

	if !drivers.AreSameCredentials(d.Config.Credentials, dOrig.Config.Credentials) {
		bitmap.Add(storage.CredentialsChange)
	}

	if !reflect.DeepEqual(d.Config.StoragePrefix, dOrig.Config.StoragePrefix) {
		bitmap.Add(storage.PrefixChange)
	}

	return bitmap
}

// Resize expands the NVMe namespace size.
func (d *ASANVMeStorageDriver) Resize(
	ctx context.Context, volConfig *storage.VolumeConfig, requestedSizeBytes uint64,
) error {
	name := volConfig.InternalName
	fields := LogFields{
		"Method":             "Resize",
		"Type":               "ASANVMeStorageDriver",
		"name":               name,
		"requestedSizeBytes": requestedSizeBytes,
	}
	Logd(ctx, d.Config.StorageDriverName, d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Resize")
	defer Logd(ctx, d.Config.StorageDriverName,
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Resize")

	if requestedSizeBytes > math.MaxInt64 {
		Logc(ctx).WithFields(fields).Error(
			"Invalid volume size. Requested size is more than maximum integer limit %v", math.MaxInt64)
		return fmt.Errorf("invalid volume size")
	}

	// Get the NVMe namespace
	ns, err := d.API.NVMeNamespaceGetByName(ctx, name)
	if err != nil {
		Logc(ctx).WithField("name", name).WithError(err).Error("Error checking for existing ASA NVMe namespace.")
		return fmt.Errorf("error checking for existing ASA NVMe namespace %v: %w", name, err)
	}

	if ns == nil {
		return tridenterrors.NotFoundError("ASA NVMe namespace %s does not exist", name)
	}

	// Get current size
	nsSizeBytes, err := convert.ToPositiveInt64(ns.Size)
	if err != nil {
		return fmt.Errorf("error while parsing ASA NVMe namespace size, %w", err)
	}

	if int64(requestedSizeBytes) < nsSizeBytes {
		return fmt.Errorf("requested size %d is less than existing ASA NVMe namespace size %d", requestedSizeBytes, nsSizeBytes)
	}

	// Check if the requested size falls within volume size limits
	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, requestedSizeBytes, d.Config.CommonStorageDriverConfig,
	); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	// Resize Namespace
	if err = d.API.NVMeNamespaceSetSize(ctx, ns.UUID, int64(requestedSizeBytes)); err != nil {
		Logc(ctx).WithField("name", name).WithError(err).Error("ASA NVMe namespace resize failed.")
		return fmt.Errorf("ASA NVMe namespace %s resize failed: %w", name, err)
	}

	// Setting the new size in the volume config.
	volConfig.Size = strconv.FormatUint(requestedSizeBytes, 10)

	return nil
}

func (d *ASANVMeStorageDriver) ReconcileNodeAccess(ctx context.Context, nodes []*models.Node,
	backendUUID, tridentUUID string,
) error {
	nodeNames := make([]string, len(nodes))
	for _, n := range nodes {
		nodeNames = append(nodeNames, n.Name)
	}
	fields := LogFields{
		"Method": "ReconcileNodeAccess",
		"Type":   "ASANVMeStorageDriver",
		"Nodes":  nodeNames,
	}

	Logd(ctx, d.Config.StorageDriverName,
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileNodeAccess")
	defer Logd(ctx, d.Config.StorageDriverName,
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileNodeAccess")

	Logc(ctx).Debug("No reconcile node access action for ASA NVMe namespace.")

	return nil
}

func (d *ASANVMeStorageDriver) ReconcileVolumeNodeAccess(ctx context.Context, _ *storage.VolumeConfig, _ []*models.Node) error {
	fields := LogFields{
		"Method": "ReconcileVolumeNodeAccess",
		"Type":   "ASANVMeStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileVolumeNodeAccess")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileVolumeNodeAccess")

	Logc(ctx).Debug("No reconcile volume node access action for ASA NVMe namespace.")

	return nil
}

// GetBackendState returns the reason if SVM is offline, and a flag to indicate if there is change
// in physical pools list.
func (d *ASANVMeStorageDriver) GetBackendState(ctx context.Context) (string, *roaring.Bitmap) {
	Logc(ctx).Debug(">>>> GetBackendState")
	defer Logc(ctx).Debug("<<<< GetBackendState")

	return getSVMState(ctx, d.API, sa.NVMeTransport, d.GetStorageBackendPhysicalPoolNames(ctx), d.Config.Aggregate)
}

// String makes ASANVMeStorageDriver satisfy the Stringer interface.
func (d ASANVMeStorageDriver) String() string {
	return convert.ToStringRedacted(&d, GetOntapDriverRedactList(), d.GetExternalConfig(context.Background()))
}

// GoString makes ASANVMeStorageDriver satisfy the GoStringer interface.
func (d *ASANVMeStorageDriver) GoString() string {
	return d.String()
}

// GetCommonConfig returns driver's CommonConfig
func (d ASANVMeStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}

// CreateNVMeNamespaceCommentString returns the string that needs to be stored in namespace comment field.
func (d *ASANVMeStorageDriver) createNVMeNamespaceCommentString(ctx context.Context, nsAttributeMap map[string]string, maxCommentLength int) (string, error) {
	nsCommentMap := map[string]map[string]string{}
	nsCommentMap[nsAttribute] = nsAttributeMap

	nsCommentJSON, err := json.Marshal(nsCommentMap)
	if err != nil {
		Logc(ctx).Errorf("Failed to marshal namespace comments: %+v.", nsCommentMap)
		return "", err
	}

	commentsJSONBytes := new(bytes.Buffer)
	if err = json.Compact(commentsJSONBytes, nsCommentJSON); err != nil {
		Logc(ctx).Errorf("Failed to compact namespace comments: %s.", string(nsCommentJSON))
		return "", err
	}

	commentsJSONString := commentsJSONBytes.String()

	if maxCommentLength != 0 && len(commentsJSONString) > maxCommentLength {
		Logc(ctx).WithFields(LogFields{
			"commentsJSON":       commentsJSONString,
			"commentsJSONLength": len(commentsJSONString),
			"maxCommentLength":   maxCommentLength,
		}).Error("Comment length exceeds the character limit.")
		return "", fmt.Errorf("comment length %v exceeds the character limit of %v characters",
			len(commentsJSONString), maxCommentLength)
	}

	return commentsJSONString, nil
}

// ParseNVMeNamespaceCommentString returns the map of attributes that were stored in namespace comment field.
func (d *ASANVMeStorageDriver) ParseNVMeNamespaceCommentString(_ context.Context, comment string) (map[string]string, error) {
	// Parse the comment
	nsComment := map[string]map[string]string{}

	err := json.Unmarshal([]byte(comment), &nsComment)
	if err != nil {
		return nil, err
	}

	nsAttrs := nsComment[nsAttribute]
	if nsAttrs != nil {
		return nsAttrs, nil
	}
	return nil, fmt.Errorf("nsAttrs field not found in Namespace comment")
}

// EnablePublishEnforcement sets the publishEnforcement on older NVMe volumes.
func (d *ASANVMeStorageDriver) EnablePublishEnforcement(_ context.Context, volume *storage.Volume) error {
	volume.Config.AccessInfo.PublishEnforcement = true
	return nil
}

// CanEnablePublishEnforcement dictates if any NVMe volume will get published on a node, depending on the node state.
func (d *ASANVMeStorageDriver) CanEnablePublishEnforcement() bool {
	return true
}

func (d *ASANVMeStorageDriver) HealVolumePublishEnforcement(ctx context.Context, volume *storage.Volume) bool {
	return HealSANPublishEnforcement(ctx, d, volume)
}

// CreateASANVMeNamespaceInternalID creates a string in the format /svm/<svm_name>/<namespace_name>
func (d *ASANVMeStorageDriver) CreateASANVMeNamespaceInternalID(svm, name string) string {
	return fmt.Sprintf("/svm/%s/namespace/%s", svm, name)
}
