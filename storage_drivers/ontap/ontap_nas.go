// Copyright 2021 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/RoaringBitmap/roaring"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/v21/config"
	. "github.com/netapp/trident/v21/logger"
	netappv1 "github.com/netapp/trident/v21/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/v21/storage"
	sa "github.com/netapp/trident/v21/storage_attribute"
	drivers "github.com/netapp/trident/v21/storage_drivers"
	"github.com/netapp/trident/v21/storage_drivers/ontap/api"
	"github.com/netapp/trident/v21/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/v21/utils"
)

// NASStorageDriver is for NFS storage provisioning
type NASStorageDriver struct {
	initialized bool
	Config      drivers.OntapStorageDriverConfig
	API         *api.Client
	Telemetry   *Telemetry

	physicalPools map[string]*storage.Pool
	virtualPools  map[string]*storage.Pool
}

func (d *NASStorageDriver) GetConfig() *drivers.OntapStorageDriverConfig {
	return &d.Config
}

func (d *NASStorageDriver) GetAPI() *api.Client {
	return d.API
}

func (d *NASStorageDriver) GetTelemetry() *Telemetry {
	d.Telemetry.Telemetry = tridentconfig.OrchestratorTelemetry
	return d.Telemetry
}

// Name is for returning the name of this driver
func (d *NASStorageDriver) Name() string {
	return drivers.OntapNASStorageDriverName
}

// BackendName returns the name of the backend managed by this driver instance
func (d *NASStorageDriver) BackendName() string {
	if d.Config.BackendName == "" {
		// Use the old naming scheme if no name is specified
		return CleanBackendName("ontapnas_" + d.Config.DataLIF)
	} else {
		return d.Config.BackendName
	}
}

// Initialize from the provided config
func (d *NASStorageDriver) Initialize(
	ctx context.Context, driverContext tridentconfig.DriverContext, configJSON string,
	commonConfig *drivers.CommonStorageDriverConfig, backendSecret map[string]string, _ string,
) error {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Initialize", "Type": "NASStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Initialize")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Initialize")
	}

	// Parse the config
	config, err := InitializeOntapConfig(ctx, driverContext, configJSON, commonConfig, backendSecret)
	if err != nil {
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
	}
	d.Config = *config

	d.API, err = InitializeOntapDriver(ctx, config)
	if err != nil {
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
	}
	d.Config = *config

	d.physicalPools, d.virtualPools, err = InitializeStoragePoolsCommon(ctx, d, d.getStoragePoolAttributes(ctx),
		d.BackendName())
	if err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

	// Validate the none, true/false values
	err = d.validate(ctx)
	if err != nil {
		return fmt.Errorf("error validating %s driver: %v", d.Name(), err)
	}

	// Set up the autosupport heartbeat
	d.Telemetry = NewOntapTelemetry(ctx, d)
	d.Telemetry.Start(ctx)

	d.initialized = true
	return nil
}

func (d *NASStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *NASStorageDriver) Terminate(ctx context.Context, backendUUID string) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Terminate", "Type": "NASStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Terminate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Terminate")
	}
	if d.Config.AutoExportPolicy {
		policyName := getExportPolicyName(backendUUID)
		if err := deleteExportPolicy(ctx, policyName, d.API); err != nil {
			Logc(ctx).Warn(err)
		}
	}
	if d.Telemetry != nil {
		d.Telemetry.Stop()
	}
	d.initialized = false
}

// Validate the driver configuration and execution environment
func (d *NASStorageDriver) validate(ctx context.Context) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "validate", "Type": "NASStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> validate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< validate")
	}

	if err := ValidateReplicationPolicy(ctx, d.Config.ReplicationPolicy, d.API); err != nil {
		return fmt.Errorf("replication policy validation error: %v", err)
	}
	if err := ValidateNASDriver(ctx, d.API, &d.Config); err != nil {
		return fmt.Errorf("driver validation failed: %v", err)
	}

	if err := ValidateStoragePrefix(*d.Config.StoragePrefix); err != nil {
		return err
	}
	if err := ValidateStoragePools(ctx, d.physicalPools, d.virtualPools, d, api.MaxNASLabelLength); err != nil {
		return fmt.Errorf("storage pool validation failed: %v", err)
	}

	return nil
}

// Create a volume with the specified options
func (d *NASStorageDriver) Create(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool *storage.Pool, volAttributes map[string]sa.Request,
) error {

	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Create",
			"Type":   "NASStorageDriver",
			"name":   name,
			"attrs":  volAttributes,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Create")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Create")
	}

	// If the volume already exists, bail out
	volExists, err := d.API.VolumeExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if volExists {
		return drivers.NewVolumeExistsError(name)
	}

	// If volume shall be mirrored, check that the SVM is peered with the other side
	if volConfig.PeerVolumeHandle != "" {
		remoteSVM, _, err := parseVolumeHandle(volConfig.PeerVolumeHandle)
		if err != nil {
			err = fmt.Errorf("could not determine required peer SVM; %v", err)
			return drivers.NewBackendIneligibleError(volConfig.InternalName, []error{err}, []string{})
		}
		peeredVservers, _ := d.API.GetPeeredVservers(ctx)
		if !utils.SliceContainsString(peeredVservers, remoteSVM) {
			err = fmt.Errorf("backend SVM %v is not peered with required SVM %v", d.Config.SVM, remoteSVM)
			return drivers.NewBackendIneligibleError(volConfig.InternalName, []error{err}, []string{})
		}
	}

	// Get candidate physical pools
	physicalPools, err := getPoolsForCreate(ctx, volConfig, storagePool, volAttributes, d.physicalPools, d.virtualPools)
	if err != nil {
		return err
	}

	// Get options
	opts, err := d.GetVolumeOpts(ctx, volConfig, volAttributes)
	if err != nil {
		return err
	}

	// get options with default fallback values
	// see also: ontap_common.go#PopulateConfigurationDefaults
	spaceReserve := utils.GetV(opts, "spaceReserve", storagePool.InternalAttributes[SpaceReserve])
	snapshotPolicy := utils.GetV(opts, "snapshotPolicy", storagePool.InternalAttributes[SnapshotPolicy])
	snapshotReserve := utils.GetV(opts, "snapshotReserve", storagePool.InternalAttributes[SnapshotReserve])
	unixPermissions := utils.GetV(opts, "unixPermissions", storagePool.InternalAttributes[UnixPermissions])
	snapshotDir := utils.GetV(opts, "snapshotDir", storagePool.InternalAttributes[SnapshotDir])
	exportPolicy := utils.GetV(opts, "exportPolicy", storagePool.InternalAttributes[ExportPolicy])
	securityStyle := utils.GetV(opts, "securityStyle", storagePool.InternalAttributes[SecurityStyle])
	encryption := utils.GetV(opts, "encryption", storagePool.InternalAttributes[Encryption])
	tieringPolicy := utils.GetV(opts, "tieringPolicy", storagePool.InternalAttributes[TieringPolicy])
	qosPolicy := storagePool.InternalAttributes[QosPolicy]
	adaptiveQosPolicy := storagePool.InternalAttributes[AdaptiveQosPolicy]

	snapshotReserveInt, err := GetSnapshotReserve(snapshotPolicy, snapshotReserve)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotReserve: %v", err)
	}

	// Determine volume size in bytes
	requestedSize, err := utils.ConvertSizeToBytes(volConfig.Size)
	if err != nil {
		return fmt.Errorf("could not convert volume size %s: %v", volConfig.Size, err)
	}
	sizeBytes, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid volume size: %v", volConfig.Size, err)
	}
	sizeBytes, err = GetVolumeSize(sizeBytes, storagePool.InternalAttributes[Size])

	// Get the flexvol size based on the snapshot reserve
	flexvolSize := calculateFlexvolSize(ctx, name, sizeBytes, snapshotReserveInt)
	if err != nil {
		return err
	}
	size := strconv.FormatUint(flexvolSize, 10)

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, sizeBytes, d.Config.CommonStorageDriverConfig); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	enableSnapshotDir, err := strconv.ParseBool(snapshotDir)
	if err != nil {
		return fmt.Errorf("invalid boolean value for snapshotDir: %v", err)
	}

	enableEncryption, err := strconv.ParseBool(encryption)
	if err != nil {
		return fmt.Errorf("invalid boolean value for encryption: %v", err)
	}

	if tieringPolicy == "" {
		tieringPolicy = d.API.TieringPolicyValue(ctx)
	}

	if d.Config.AutoExportPolicy {
		exportPolicy = getExportPolicyName(storagePool.Backend.BackendUUID())
	}

	qosPolicyGroup, err := api.NewQosPolicyGroup(qosPolicy, adaptiveQosPolicy)
	if err != nil {
		return err
	}
	volConfig.QosPolicy = qosPolicy
	volConfig.AdaptiveQosPolicy = adaptiveQosPolicy

	Logc(ctx).WithFields(log.Fields{
		"name":              name,
		"size":              size,
		"spaceReserve":      spaceReserve,
		"snapshotPolicy":    snapshotPolicy,
		"snapshotReserve":   snapshotReserveInt,
		"unixPermissions":   unixPermissions,
		"snapshotDir":       enableSnapshotDir,
		"exportPolicy":      exportPolicy,
		"securityStyle":     securityStyle,
		"encryption":        enableEncryption,
		"tieringPolicy":     tieringPolicy,
		"qosPolicy":         qosPolicy,
		"adaptiveQosPolicy": adaptiveQosPolicy,
	}).Debug("Creating Flexvol.")

	createErrors := make([]error, 0)
	physicalPoolNames := make([]string, 0)

	for _, physicalPool := range physicalPools {
		aggregate := physicalPool.Name
		physicalPoolNames = append(physicalPoolNames, aggregate)

		if aggrLimitsErr := checkAggregateLimits(
			ctx, aggregate, spaceReserve, flexvolSize, d.Config, d.GetAPI()); aggrLimitsErr != nil {
			errMessage := fmt.Sprintf("ONTAP-NAS pool %s/%s; error: %v", storagePool.Name, aggregate, aggrLimitsErr)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))
			continue
		}

		labels, err := storagePool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, api.MaxNASLabelLength)
		if err != nil {
			return err
		}

		// Create the volume
		volCreateResponse, err := d.API.VolumeCreate(
			ctx, name, aggregate, size, spaceReserve, snapshotPolicy, unixPermissions, exportPolicy, securityStyle,
			tieringPolicy, labels, qosPolicyGroup, enableEncryption, snapshotReserveInt, volConfig.IsMirrorDestination)

		if err = api.GetError(ctx, volCreateResponse, err); err != nil {
			if zerr, ok := err.(api.ZapiError); ok {
				// Handle case where the Create is passed to every Docker Swarm node
				if zerr.Code() == azgo.EAPIERROR && strings.HasSuffix(strings.TrimSpace(zerr.Reason()), "Job exists") {
					Logc(ctx).WithField("volume", name).Warn("Volume create job already exists, " +
						"skipping volume create on this node.")
					return nil
				}
			}

			errMessage := fmt.Sprintf("ONTAP-NAS pool %s/%s; error creating volume %s: %v", storagePool.Name,
				aggregate, name, err)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))
			continue
		}

		// Disable '.snapshot' to allow official mysql container's chmod-in-init to work
		if !enableSnapshotDir {
			snapDirResponse, err := d.API.VolumeDisableSnapshotDirectoryAccess(name)
			if err = api.GetError(ctx, snapDirResponse, err); err != nil {
				return fmt.Errorf("error disabling snapshot directory access: %v", err)
			}
		}
		// If a DP volume, skip mounting the volume
		if volConfig.IsMirrorDestination {
			return nil
		}

		// Mount the volume at the specified junction
		mountResponse, err := d.API.VolumeMount(name, "/"+name)
		if err = api.GetError(ctx, mountResponse, err); err != nil {
			return fmt.Errorf("error mounting volume to junction: %v", err)
		}

		return nil
	}

	// All physical pools that were eligible ultimately failed, so don't try this backend again
	return drivers.NewBackendIneligibleError(name, createErrors, physicalPoolNames)
}

// CreateClone creates a volume clone
func (d *NASStorageDriver) CreateClone(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool *storage.Pool,
) error {

	sourceLabel := ""

	// Ensure the volume exists
	flexvol, err := d.API.VolumeGet(volConfig.CloneSourceVolumeInternal)
	if err != nil {
		return err
	} else if flexvol == nil {
		return fmt.Errorf("volume %s not found", volConfig.CloneSourceVolumeInternal)
	}

	// Get the source volume's label
	if flexvol.VolumeIdAttributesPtr != nil {
		volumeIdAttrs := flexvol.VolumeIdAttributes()
		sourceLabel = volumeIdAttrs.Comment()
	}

	return CreateCloneNAS(ctx, d, volConfig, storagePool, sourceLabel, api.MaxNASLabelLength, false)
}

// Destroy the volume
func (d *NASStorageDriver) Destroy(ctx context.Context, name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Destroy",
			"Type":   "NASStorageDriver",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Destroy")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Destroy")
	}

	// TODO: If this is the parent of one or more clones, those clones have to split from this
	// volume before it can be deleted, which means separate copies of those volumes.
	// If there are a lot of clones on this volume, that could seriously balloon the amount of
	// utilized space. Is that what we want? Or should we just deny the delete, and force the
	// user to keep the volume around until all of the clones are gone? If we do that, need a
	// way to list the clones. Maybe volume inspect.

	// If flexvol has been a snapmirror destination
	snapDeleteResponse, err := d.API.SnapmirrorDeleteViaDestination(name, d.Config.SVM)
	err = api.GetError(ctx, snapDeleteResponse, err)
	if err != nil && snapDeleteResponse.Result.ResultErrnoAttr != azgo.EOBJECTNOTFOUND {
		return fmt.Errorf("error deleting snapmirror info for volume %v: %v", name, err)
	}

	// Ensure no leftover snapmirror metadata
	err = d.API.SnapmirrorRelease(name, d.Config.SVM)
	if err != nil {
		return fmt.Errorf("error releasing snapmirror info for volume %v: %v", name, err)
	}

	volDestroyResponse, err := d.API.VolumeDestroy(name, true)
	if err != nil {
		return fmt.Errorf("error destroying volume %v: %v", name, err)
	}
	if zerr := api.NewZapiError(volDestroyResponse); !zerr.IsPassed() {

		// It's not an error if the volume no longer exists
		if zerr.Code() == azgo.EVOLUMEDOESNOTEXIST {
			Logc(ctx).WithField("volume", name).Warn("Volume already deleted.")
		} else {
			return fmt.Errorf("error destroying volume %v: %v", name, zerr)
		}
	}

	return nil
}

func (d *NASStorageDriver) Import(ctx context.Context, volConfig *storage.VolumeConfig, originalName string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "Import",
			"Type":         "NASStorageDriver",
			"originalName": originalName,
			"newName":      volConfig.InternalName,
			"notManaged":   volConfig.ImportNotManaged,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Import")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Import")
	}

	// Ensure the volume exists
	flexvol, err := d.API.VolumeGet(originalName)
	if err != nil {
		return err
	} else if flexvol == nil {
		return fmt.Errorf("volume %s not found", originalName)
	}

	// Validate the volume is what it should be
	if flexvol.VolumeIdAttributesPtr != nil {
		volumeIdAttrs := flexvol.VolumeIdAttributes()
		if volumeIdAttrs.TypePtr != nil && volumeIdAttrs.Type() != "rw" {
			Logc(ctx).WithField("originalName", originalName).Error("Could not import volume, type is not rw.")
			return fmt.Errorf("volume %s type is %s, not rw", originalName, volumeIdAttrs.Type())
		}
	}

	// Get the volume size
	if flexvol.VolumeSpaceAttributesPtr == nil || flexvol.VolumeSpaceAttributesPtr.SizePtr == nil {
		Logc(ctx).WithField("originalName", originalName).Errorf("Could not import volume, size not available")
		return fmt.Errorf("volume %s size not available", originalName)
	}
	volConfig.Size = strconv.FormatInt(int64(flexvol.VolumeSpaceAttributesPtr.Size()), 10)

	// Rename the volume if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		renameResponse, err := d.API.VolumeRename(originalName, volConfig.InternalName)
		if err = api.GetError(ctx, renameResponse, err); err != nil {
			Logc(ctx).WithField("originalName", originalName).Errorf("Could not import volume, rename failed: %v", err)
			return fmt.Errorf("volume %s rename failed: %v", originalName, err)
		}
	}

	// Update the volume labels if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		volumeIdAttrs := flexvol.VolumeIdAttributes()
		if storage.AllowPoolLabelOverwrite(storage.ProvisioningLabelTag, volumeIdAttrs.Comment()) {
			modifyCommentResponse, err := d.API.VolumeSetComment(ctx, volConfig.InternalName, "")
			if err = api.GetError(ctx, modifyCommentResponse, err); err != nil {
				Logc(ctx).WithField("originalName", originalName).Errorf("Modifying comment failed: %v", err)
				return fmt.Errorf("volume %s modify failed: %v", originalName, err)
			}
		}
	}

	// Modify unix-permissions of the volume if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		// unixPermissions specified in PVC annotation takes precedence over backend's unixPermissions config
		unixPerms := volConfig.UnixPermissions
		if unixPerms == "" {
			unixPerms = d.Config.UnixPermissions
		}
		modifyUnixPermResponse, err := d.API.VolumeModifyUnixPermissions(volConfig.InternalName, unixPerms)
		if err = api.GetError(ctx, modifyUnixPermResponse, err); err != nil {
			Logc(ctx).WithField("originalName", originalName).Errorf("Could not import volume, "+
				"modifying unix permissions failed: %v", err)
			return fmt.Errorf("volume %s modify failed: %v", originalName, err)
		}
	}

	// Make sure we're not importing a volume without a junction path when not managed
	if volConfig.ImportNotManaged {
		if flexvol.VolumeIdAttributesPtr == nil {
			return fmt.Errorf("unable to read volume id attributes of volume %s", originalName)
		} else if flexvol.VolumeIdAttributesPtr.JunctionPathPtr == nil || flexvol.VolumeIdAttributesPtr.JunctionPath() == "" {
			return fmt.Errorf("junction path is not set for volume %s", originalName)
		}
	}

	return nil
}

// Rename changes the name of a volume
func (d *NASStorageDriver) Rename(ctx context.Context, name, newName string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":  "Rename",
			"Type":    "NASStorageDriver",
			"name":    name,
			"newName": newName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Rename")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Rename")
	}

	renameResponse, err := d.API.VolumeRename(name, newName)
	if err = api.GetError(ctx, renameResponse, err); err != nil {
		Logc(ctx).WithField("name", name).Warnf("Could not rename volume: %v", err)
		return fmt.Errorf("could not rename volume %s: %v", name, err)
	}

	return nil
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *NASStorageDriver) Publish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *utils.VolumePublishInfo,
) error {

	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":  "Publish",
			"DataLIF": d.Config.DataLIF,
			"Type":    "NASStorageDriver",
			"name":    name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Publish")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Publish")
	}

	// Determine mount options (volume config wins, followed by backend config)
	mountOptions := d.Config.NfsMountOptions
	if volConfig.MountOptions != "" {
		mountOptions = volConfig.MountOptions
	}

	// Add fields needed by Attach
	publishInfo.NfsPath = fmt.Sprintf("/%s", name)
	publishInfo.NfsServerIP = d.Config.DataLIF
	publishInfo.FilesystemType = "nfs"
	publishInfo.MountOptions = mountOptions

	return publishFlexVolShare(ctx, d.API, &d.Config, publishInfo, name)
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (d *NASStorageDriver) CanSnapshot(_ context.Context, _ *storage.SnapshotConfig) error {
	return nil
}

// GetSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func (d *NASStorageDriver) GetSnapshot(ctx context.Context, snapConfig *storage.SnapshotConfig) (
	*storage.Snapshot, error,
) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "GetSnapshot",
			"Type":         "NASStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetSnapshot")
	}

	return GetSnapshot(ctx, snapConfig, &d.Config, d.API, d.API.VolumeSize)
}

// GetSnapshots returns the list of snapshots associated with the specified volume
func (d *NASStorageDriver) GetSnapshots(ctx context.Context, volConfig *storage.VolumeConfig) (
	[]*storage.Snapshot, error,
) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "GetSnapshots",
			"Type":       "NASStorageDriver",
			"volumeName": volConfig.InternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetSnapshots")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetSnapshots")
	}

	return GetSnapshots(ctx, volConfig, &d.Config, d.API, d.API.VolumeSize)
}

// CreateSnapshot creates a snapshot for the given volume
func (d *NASStorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig,
) (*storage.Snapshot, error) {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "CreateSnapshot",
			"Type":         "NASStorageDriver",
			"snapshotName": internalSnapName,
			"sourceVolume": internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateSnapshot")
	}

	return CreateSnapshot(ctx, snapConfig, &d.Config, d.API, d.API.VolumeSize)
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *NASStorageDriver) RestoreSnapshot(ctx context.Context, snapConfig *storage.SnapshotConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "RestoreSnapshot",
			"Type":         "NASStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> RestoreSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< RestoreSnapshot")
	}

	return RestoreSnapshot(ctx, snapConfig, &d.Config, d.API)
}

// DeleteSnapshot creates a snapshot of a volume.
func (d *NASStorageDriver) DeleteSnapshot(ctx context.Context, snapConfig *storage.SnapshotConfig) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "DeleteSnapshot",
			"Type":         "NASStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> DeleteSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< DeleteSnapshot")
	}

	return DeleteSnapshot(ctx, snapConfig, &d.Config, d.API)
}

// Get tests for the existence of a volume
func (d *NASStorageDriver) Get(ctx context.Context, name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Get", "Type": "NASStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Get")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Get")
	}

	return GetVolume(ctx, name, d.API, &d.Config)
}

// GetStorageBackendSpecs retrieves storage backend capabilities
func (d *NASStorageDriver) GetStorageBackendSpecs(_ context.Context, backend storage.Backend) error {
	return getStorageBackendSpecsCommon(backend, d.physicalPools, d.virtualPools, d.BackendName())
}

// GetStorageBackendPhysicalPoolNames retrieves storage backend physical pools
func (d *NASStorageDriver) GetStorageBackendPhysicalPoolNames(context.Context) []string {
	return getStorageBackendPhysicalPoolNamesCommon(d.physicalPools)
}

func (d *NASStorageDriver) getStoragePoolAttributes(ctx context.Context) map[string]sa.Offer {

	client := d.GetAPI()
	mirroring, _ := client.IsVserverDRCapable(ctx)
	return map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(d.Name()),
		sa.Snapshots:        sa.NewBoolOffer(true),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.Encryption:       sa.NewBoolOffer(true),
		sa.Replication:      sa.NewBoolOffer(mirroring),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}
}

func (d *NASStorageDriver) GetVolumeOpts(
	ctx context.Context, volConfig *storage.VolumeConfig, requests map[string]sa.Request,
) (map[string]string, error) {
	return getVolumeOptsCommon(ctx, volConfig, requests), nil
}

func (d *NASStorageDriver) GetInternalVolumeName(_ context.Context, name string) string {
	return getInternalVolumeNameCommon(d.Config.CommonStorageDriverConfig, name)
}

func (d *NASStorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig) {
	createPrepareCommon(ctx, d, volConfig)
}

func (d *NASStorageDriver) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {

	volConfig.MirrorHandle = d.Config.SVM + ":" + volConfig.InternalName
	volConfig.AccessInfo.NfsServerIP = d.Config.DataLIF
	volConfig.AccessInfo.MountOptions = strings.TrimPrefix(d.Config.NfsMountOptions, "-o ")
	volConfig.FileSystem = ""

	// Set correct junction path
	flexvol, err := d.API.VolumeGet(volConfig.InternalName)
	if err != nil {
		return err
	} else if flexvol == nil {
		return fmt.Errorf("volume %s not found", volConfig.InternalName)
	}

	if flexvol.VolumeIdAttributesPtr == nil {
		return fmt.Errorf("error reading volume id attributes for volume %s", volConfig.InternalName)
	}
	if flexvol.VolumeIdAttributesPtr.JunctionPathPtr == nil || flexvol.VolumeIdAttributesPtr.JunctionPath() == "" {
		// Flexvol is not mounted, we need to mount it
		volConfig.AccessInfo.NfsPath = "/" + volConfig.InternalName
		mountResponse, err := d.API.VolumeMount(volConfig.InternalName, volConfig.AccessInfo.NfsPath)
		// An API error is returned if we attempt to mount a DP volume that has not yet been snapmirrored,
		// we expect this to be the case.
		if err = api.GetError(ctx, mountResponse, err); err != nil {
			attributes := flexvol.VolumeMirrorAttributes()
			if err.(api.ZapiError).Code() == azgo.EAPIERROR && attributes.IsDataProtectionMirror() {
				Logc(ctx).Debugf("Received expected API error when mounting DP volume to junction; %v", err)
			} else {
				return fmt.Errorf("error mounting volume to junction %s; %v", volConfig.AccessInfo.NfsPath, err)
			}
		}
	} else {
		volConfig.AccessInfo.NfsPath = flexvol.VolumeIdAttributesPtr.JunctionPath()
	}
	return nil
}

func (d *NASStorageDriver) GetProtocol(context.Context) tridentconfig.Protocol {
	return tridentconfig.File
}

func (d *NASStorageDriver) StoreConfig(_ context.Context, b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.OntapConfig = &d.Config
}

func (d *NASStorageDriver) GetExternalConfig(ctx context.Context) interface{} {
	return getExternalConfig(ctx, d.Config)
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *NASStorageDriver) GetVolumeExternal(_ context.Context, name string) (*storage.VolumeExternal, error) {

	volumeAttributes, err := d.API.VolumeGet(name)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(volumeAttributes), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *NASStorageDriver) GetVolumeExternalWrappers(ctx context.Context, channel chan *storage.VolumeExternalWrapper) {

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes matching the storage prefix
	volumesResponse, err := d.API.VolumeGetAll(*d.Config.StoragePrefix)
	if err = api.GetError(ctx, volumesResponse, err); err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Convert all volumes to VolumeExternal and write them to the channel
	if volumesResponse.Result.AttributesListPtr != nil {
		for _, volume := range volumesResponse.Result.AttributesListPtr.VolumeAttributesPtr {
			channel <- &storage.VolumeExternalWrapper{Volume: d.getVolumeExternal(&volume), Error: nil}
		}
	}
}

// getVolumeExternal is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *NASStorageDriver) getVolumeExternal(
	volumeAttrs *azgo.VolumeAttributesType) *storage.VolumeExternal {

	volumeExportAttrs := volumeAttrs.VolumeExportAttributesPtr
	volumeIDAttrs := volumeAttrs.VolumeIdAttributesPtr
	volumeSecurityAttrs := volumeAttrs.VolumeSecurityAttributesPtr
	volumeSecurityUnixAttrs := volumeSecurityAttrs.VolumeSecurityUnixAttributesPtr
	volumeSpaceAttrs := volumeAttrs.VolumeSpaceAttributesPtr
	volumeSnapshotAttrs := volumeAttrs.VolumeSnapshotAttributesPtr

	internalName := volumeIDAttrs.Name()
	name := internalName
	if strings.HasPrefix(internalName, *d.Config.StoragePrefix) {
		name = internalName[len(*d.Config.StoragePrefix):]
	}

	volumeConfig := &storage.VolumeConfig{
		Version:         tridentconfig.OrchestratorAPIVersion,
		Name:            name,
		InternalName:    internalName,
		Size:            strconv.FormatInt(int64(volumeSpaceAttrs.Size()), 10),
		Protocol:        tridentconfig.File,
		SnapshotPolicy:  volumeSnapshotAttrs.SnapshotPolicy(),
		ExportPolicy:    volumeExportAttrs.Policy(),
		SnapshotDir:     strconv.FormatBool(volumeSnapshotAttrs.SnapdirAccessEnabled()),
		UnixPermissions: volumeSecurityUnixAttrs.Permissions(),
		StorageClass:    "",
		AccessMode:      tridentconfig.ReadWriteMany,
		AccessInfo:      utils.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
	}

	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   volumeIDAttrs.ContainingAggregateName(),
	}
}

// GetUpdateType returns a bitmap populated with updates to the driver
func (d *NASStorageDriver) GetUpdateType(ctx context.Context, driverOrig storage.Driver) *roaring.Bitmap {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "GetUpdateType",
			"Type":   "NASStorageDriver",
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetUpdateType")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetUpdateType")
	}

	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*NASStorageDriver)
	if !ok {
		bitmap.Add(storage.InvalidUpdate)
		return bitmap
	}

	if d.Config.DataLIF != dOrig.Config.DataLIF {
		bitmap.Add(storage.VolumeAccessInfoChange)
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

// Resize expands the volume size.
func (d *NASStorageDriver) Resize(ctx context.Context, volConfig *storage.VolumeConfig, requestedSizeBytes uint64) error {

	name := volConfig.InternalName
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":             "Resize",
			"Type":               "NASStorageDriver",
			"name":               name,
			"requestedSizeBytes": requestedSizeBytes,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Resize")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Resize")
	}

	flexvolSize, err := resizeValidation(ctx, name, requestedSizeBytes, d.API.VolumeExists, d.API.VolumeSize)
	if err != nil {
		return err
	}

	volConfig.Size = strconv.FormatUint(flexvolSize, 10)
	if flexvolSize == requestedSizeBytes {
		return nil
	}

	snapshotReserveInt, err := getSnapshotReserveFromOntap(ctx, name, d.API.VolumeGet)
	if err != nil {
		Logc(ctx).WithField("name", name).Errorf("Could not get the snapshot reserve percentage for volume")
	}

	newFlexvolSize := calculateFlexvolSize(ctx, name, requestedSizeBytes, snapshotReserveInt)

	if aggrLimitsErr := checkAggregateLimitsForFlexvol(
		ctx, name, newFlexvolSize, d.Config, d.GetAPI()); aggrLimitsErr != nil {
		return aggrLimitsErr
	}

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, requestedSizeBytes, d.Config.CommonStorageDriverConfig); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	response, err := d.API.VolumeSetSize(name, strconv.FormatUint(newFlexvolSize, 10))
	err = api.GetError(ctx, response, err)
	if err = api.GetError(ctx, response.Result, err); err != nil {
		Logc(ctx).WithField("error", err).Error("Volume resize failed.")
		return fmt.Errorf("volume resize failed")
	}

	volConfig.Size = strconv.FormatUint(requestedSizeBytes, 10)
	return nil
}

func (d *NASStorageDriver) ReconcileNodeAccess(ctx context.Context, nodes []*utils.Node, backendUUID string) error {

	nodeNames := make([]string, 0)
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "ReconcileNodeAccess",
			"Type":   "NASStorageDriver",
			"Nodes":  nodeNames,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> ReconcileNodeAccess")
		defer Logc(ctx).WithFields(fields).Debug("<<<< ReconcileNodeAccess")
	}

	policyName := getExportPolicyName(backendUUID)

	return reconcileNASNodeAccess(ctx, nodes, &d.Config, d.API, policyName)
}

// String makes NASStorageDriver satisfy the Stringer interface.
func (d NASStorageDriver) String() string {
	return drivers.ToString(&d, GetOntapDriverRedactList(), d.GetExternalConfig(context.Background()))
}

// GoString makes NASStorageDriver satisfy the GoStringer interface.
func (d NASStorageDriver) GoString() string {
	return d.String()
}

// GetCommonConfig returns driver's CommonConfig
func (d NASStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}

// EstablishMirror will create a new snapmirror relationship between a RW and a DP volume that have not previously
// had a relationship
func (d NASStorageDriver) EstablishMirror(ctx context.Context, localVolumeHandle, remoteVolumeHandle string) error {
	localSVMName, localFlexvolName, err := parseVolumeHandle(localVolumeHandle)
	if err != nil {
		return fmt.Errorf("could not parse localVolumeHandle '%v'; %v", localVolumeHandle, err)
	}
	remoteSVMName, remoteFlexvolName, err := parseVolumeHandle(remoteVolumeHandle)
	if err != nil {
		return fmt.Errorf("could not parse remoteVolumeHandle '%v'; %v", remoteVolumeHandle, err)
	}

	// Ensure the destination is a DP volume
	volType, err := d.API.VolumeGetType(localFlexvolName)
	if err != nil {
		return fmt.Errorf("could not determine volume type")
	}
	if volType != "dp" {
		return fmt.Errorf("mirrors can only be established with empty DP volumes as the destination")
	}

	// Check if a snapmirror relationship already exists
	snapmirror, err := d.API.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	relationshipNotFound := false
	initialized := false
	if err = api.GetError(ctx, snapmirror, err); err != nil {
		if zerr, ok := err.(api.ZapiError); ok {
			relationshipNotFound = zerr.Code() == azgo.EOBJECTNOTFOUND
			if !relationshipNotFound && !zerr.IsPassed() {
				Logc(ctx).WithError(err).Error("Error on snapmirror get")
				return err
			}
		}
	}
	if relationshipNotFound {
		_, err := d.API.SnapmirrorCreate(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
			d.Config.ReplicationPolicy, d.Config.ReplicationSchedule)
		if err != nil {
			if zerr, ok := err.(api.ZapiError); ok && zerr.Code() != azgo.EDUPLICATEENTRY {
				Logc(ctx).WithError(err).Error("Error on snapmirror create")
				return err
			}
		}
		// Get the snapmirror after creation
		snapmirror, err = d.API.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err = api.GetError(ctx, snapmirror, err); err != nil {
			Logc(ctx).WithError(err).Error("Error on snapmirror get")
			return err
		}
	}

	attributes := snapmirror.Result.Attributes()
	info := attributes.SnapmirrorInfo()
	initialized = info.MirrorState() != SnapmirrorStateUninitialized
	snapmirrorIdle := info.RelationshipStatus() == SnapmirrorStatusIdle

	// Initialize the snapmirror
	if !initialized && snapmirrorIdle {
		_, err := d.API.SnapmirrorInitialize(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if zerr, ok := err.(api.ZapiError); ok {
			if !zerr.IsPassed() {
				// Snapmirror is current initializing
				if zerr.Code() == azgo.ETRANSFERINPROGRESS {
					Logc(ctx).Debug("snapmirror transfer already in progress")
				} else {
					Logc(ctx).WithError(err).Error("Error on snapmirror initialize")
					return err
				}
			}
		}

	}
	return nil
}

// ReestablishMirror will attempt to resync a snapmirror relationship,
// if and only if the relationship existed previously
func (d NASStorageDriver) ReestablishMirror(ctx context.Context, localVolumeHandle, remoteVolumeHandle string) error {
	localSVMName, localFlexvolName, err := parseVolumeHandle(localVolumeHandle)
	if err != nil {
		return fmt.Errorf("could not parse localVolumeHandle '%v'; %v", localVolumeHandle, err)
	}
	remoteSVMName, remoteFlexvolName, err := parseVolumeHandle(remoteVolumeHandle)
	if err != nil {
		return fmt.Errorf("could not parse remoteVolumeHandle '%v'; %v", remoteVolumeHandle, err)
	}

	relationshipNotFound := false
	initialized := false
	snapmirrorIdle := true

	// Check if a snapmirror relationship already exists
	snapmirror, err := d.API.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = api.GetError(ctx, snapmirror, err); err != nil {
		if zerr, ok := err.(api.ZapiError); ok {
			relationshipNotFound = zerr.Code() == azgo.EOBJECTNOTFOUND
			if !relationshipNotFound && !zerr.IsPassed() {
				Logc(ctx).WithError(err).Error("Error on snapmirror get")
				return err
			}
		}
	}
	if snapmirror != nil && !relationshipNotFound {
		attributes := snapmirror.Result.Attributes()
		info := attributes.SnapmirrorInfo()
		initialized = info.MirrorState() != SnapmirrorStateUninitialized || info.LastTransferType() != ""
		snapmirrorIdle = info.RelationshipStatus() == SnapmirrorStatusIdle
	}

	// If the snapmirror is already established we have nothing to do
	if initialized && snapmirrorIdle {
		return nil
	}

	// Create the relationship if it doesn't exist
	if relationshipNotFound {
		snapCreate, err := d.API.SnapmirrorCreate(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
			d.Config.ReplicationPolicy, d.Config.ReplicationSchedule)
		if err = api.GetError(ctx, snapCreate, err); err != nil {
			if zerr, ok := err.(api.ZapiError); !ok || zerr.Code() != azgo.EDUPLICATEENTRY {
				Logc(ctx).WithError(err).Error("Error on snapmirror create")
				return err
			}
		}
	}

	// Resync the relationship
	snapResync, err := d.API.SnapmirrorResync(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = api.GetError(ctx, snapResync, err); err != nil {
		if zerr, ok := err.(api.ZapiError); !ok || zerr.Code() != azgo.ETRANSFERINPROGRESS {
			Logc(ctx).WithError(err).Error("Error on snapmirror resync")
			// If we fail on the resync, we need to cleanup the snapmirror
			// it will be recreated in a future TMR reconcile loop through this function
			snapDelete, deleteErr := d.API.SnapmirrorDelete(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
			if deleteErr = api.GetError(ctx, snapDelete, deleteErr); deleteErr != nil {
				log.WithError(deleteErr).Warn("Error on snapmirror delete")
			}
			return err
		}
	}

	// Verify the state of the relationship
	snapmirror, err = d.API.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = api.GetError(ctx, snapmirror, err); err != nil {
		if zerr, ok := err.(api.ZapiError); ok {
			// If the snapmirror does not exist yet, we need to check back later
			if zerr.Code() == azgo.EOBJECTNOTFOUND {
				return utils.ReconcileIncompleteError()
			}
		} else {
			Logc(ctx).WithError(err).Error("Error on snapmirror get")
			return err
		}
	}
	// Check if the snapmirror is healthy
	attributes := snapmirror.Result.Attributes()
	info := attributes.SnapmirrorInfo()
	if !info.IsHealthy() {
		err = fmt.Errorf(info.UnhealthyReason())
		Logc(ctx).WithError(err).Error("Error on snapmirror resync")
		snapDelete, deleteErr := d.API.SnapmirrorDelete(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if deleteErr = api.GetError(ctx, snapDelete, deleteErr); deleteErr != nil {
			log.WithError(deleteErr).Warn("Error on snapmirror delete")
		}
		return err
	}
	return nil
}

// PromoteMirror will break the snapmirror and make the destination volume RW,
// optionally after a given snapshot has synced
func (d NASStorageDriver) PromoteMirror(
	ctx context.Context, localVolumeHandle, remoteVolumeHandle, snapshotHandle string,
) (bool, error) {

	if remoteVolumeHandle == "" {
		return false, nil
	}

	localSVMName, localFlexvolName, err := parseVolumeHandle(localVolumeHandle)
	if err != nil {
		return false, fmt.Errorf("could not parse localVolumeHandle '%v'; %v", localVolumeHandle, err)
	}
	remoteSVMName, remoteFlexvolName, err := parseVolumeHandle(remoteVolumeHandle)
	if err != nil {
		return false, fmt.Errorf("could not parse remoteVolumeHandle '%v'; %v", remoteVolumeHandle, err)
	}

	relationshipNotFound := false
	snapmirror, err := d.API.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = api.GetError(ctx, snapmirror, err); err != nil {
		if zerr, ok := err.(api.ZapiError); ok {
			relationshipNotFound = zerr.Code() == azgo.EOBJECTNOTFOUND
			if !relationshipNotFound && !zerr.IsPassed() {
				Logc(ctx).WithError(err).Error("Error on snapmirror get")
				return false, err
			}
		}
	}

	if d.Config.ReplicationPolicy != "" {
		smPolicy, err := d.API.SnapmirrorPolicyGet(ctx, d.Config.ReplicationPolicy)
		if err != nil {
			return false, err
		}
		// If the policy is a synchronous type we shouldn't wait for a snapshot
		if smPolicy.Type() == SnapmirrorPolicyTypeSync {
			snapshotHandle = ""
		}
	}

	// Check for snapshot
	if snapshotHandle != "" {
		snapshotTokens := strings.Split(snapshotHandle, "/")
		if len(snapshotTokens) != 2 {
			return false, fmt.Errorf("invalid snapshot handle %v", snapshotHandle)
		}
		_, snapshotName, err := storage.ParseSnapshotID(snapshotHandle)
		if err != nil {
			return false, err
		}

		found := false
		snapshotResponse, _ := d.API.SnapshotList(localFlexvolName)
		snapshotList := snapshotResponse.Result.AttributesList()
		for _, snapshot := range snapshotList.SnapshotInfo() {
			if snapshot.Name() == snapshotName {
				found = true
				break
			}
		}

		if !found {
			Logc(ctx).WithField("snapshot", snapshotHandle).Info("Snapshot not yet present.")
			return true, nil
		}
	}

	if !relationshipNotFound {
		snapQuiesce, err := d.API.SnapmirrorQuiesce(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err = api.GetError(ctx, snapQuiesce, err); err != nil {
			log.WithError(err).Info("Error on snapmirror quiesce")
			return false, err
		}

		snapAbort, err := d.API.SnapmirrorAbort(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err = api.GetError(ctx, snapAbort, err); err != nil {
			if zerr, ok := err.(api.ZapiError); !ok || zerr.Code() != azgo.ENOTRANSFERINPROGRESS {
				log.WithError(err).Info("Error on snapmirror abort")
				return false, err
			}
		}

		snapmirror, err = d.API.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err = api.GetError(ctx, snapmirror, err); err != nil {
			if zerr, ok := err.(api.ZapiError); ok {
				if !zerr.IsPassed() {
					Logc(ctx).WithError(err).Error("Error on snapmirror get")
					return false, err
				}
			}
		}

		// Skip the break if snapmirror is uninitialized, otherwise it will fail saying the volume is not initialized
		snapmirrorAttributes := snapmirror.Result.Attributes()
		snapmirrorInfo := snapmirrorAttributes.SnapmirrorInfo()
		if snapmirrorInfo.MirrorState() != SnapmirrorStateUninitialized {
			snapBreak, err := d.API.SnapmirrorBreak(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
			if err = api.GetError(ctx, snapBreak, err); err != nil {
				if zerr, ok := err.(api.ZapiError); !ok || zerr.Code() != azgo.EDEST_ISNOT_DP_VOLUME {
					log.WithError(err).Info("Error on snapmirror break")
					return false, err
				}
			}
		}

		snapDelete, err := d.API.SnapmirrorDelete(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
		if err = api.GetError(ctx, snapDelete, err); err != nil {
			log.WithError(err).Info("Error on snapmirror delete")
			return false, err
		}
	}
	return false, nil
}

// GetMirrorStatus returns the current state of a snapmirror relationship
func (d NASStorageDriver) GetMirrorStatus(
	ctx context.Context, localVolumeHandle, remoteVolumeHandle string,
) (string, error) {

	// Empty remote means there is no mirror to check for
	if remoteVolumeHandle == "" {
		return "", nil
	}

	localSVMName, localFlexvolName, err := parseVolumeHandle(localVolumeHandle)
	if err != nil {
		return "", fmt.Errorf("could not parse localVolumeHandle '%v'; %v", localVolumeHandle, err)
	}
	remoteSVMName, remoteFlexvolName, err := parseVolumeHandle(remoteVolumeHandle)
	if err != nil {
		return "", fmt.Errorf("could not parse remoteVolumeHandle '%v'; %v", remoteVolumeHandle, err)
	}

	created := true
	snapmirror, err := d.API.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = api.GetError(ctx, snapmirror, err); err != nil {
		if zerr, ok := err.(api.ZapiError); !ok || zerr.Code() != azgo.EOBJECTNOTFOUND {
			// If snapmirror is gone then the volume is promoted
			return netappv1.MirrorStatePromoted, nil
		} else {
			Logc(ctx).WithError(err).Error("Error on snapmirror get")
			created = false
		}
	}

	// Translate the snapmirror status to a mirror status
	if created {
		attributes := snapmirror.Result.Attributes()
		info := attributes.SnapmirrorInfo()
		mirrorState := info.MirrorState()
		relationshipStatus := info.RelationshipStatus()
		switch relationshipStatus {
		case SnapmirrorStatusBreaking:
			return netappv1.MirrorStatePromoting, nil
		case SnapmirrorStatusQuiescing:
			return netappv1.MirrorStatePromoting, nil
		case SnapmirrorStatusAborting:
			return netappv1.MirrorStatePromoting, nil
		default:
			switch mirrorState {
			case SnapmirrorStateBroken:
				if relationshipStatus == SnapmirrorStatusTransferring {
					return netappv1.MirrorStateEstablishing, nil
				}
				return netappv1.MirrorStatePromoting, nil
			case SnapmirrorStateUninitialized:
				return netappv1.MirrorStateEstablishing, nil
			case SnapmirrorStateSnapmirrored:
				return netappv1.MirrorStateEstablished, nil
			}
		}
	}
	return "", nil
}
