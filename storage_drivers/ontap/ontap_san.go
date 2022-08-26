// Copyright 2022 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/RoaringBitmap/roaring"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/utils"
)

func lunPath(name string) string {
	return fmt.Sprintf("/vol/%v/lun0", name)
}

// SANStorageDriver is for iSCSI storage provisioning
type SANStorageDriver struct {
	initialized bool
	Config      drivers.OntapStorageDriverConfig
	ips         []string
	API         api.OntapAPI
	telemetry   *TelemetryAbstraction

	physicalPools map[string]storage.Pool
	virtualPools  map[string]storage.Pool
}

func (d *SANStorageDriver) GetConfig() *drivers.OntapStorageDriverConfig {
	return &d.Config
}

func (d *SANStorageDriver) GetAPI() api.OntapAPI {
	return d.API
}

func (d *SANStorageDriver) GetTelemetry() *TelemetryAbstraction {
	return d.telemetry
}

// Name is for returning the name of this driver
func (d SANStorageDriver) Name() string {
	return drivers.OntapSANStorageDriverName
}

// BackendName returns the name of the backend managed by this driver instance
func (d *SANStorageDriver) BackendName() string {
	if d.Config.BackendName == "" {
		// Use the old naming scheme if no name is specified
		lif0 := "noLIFs"
		if len(d.ips) > 0 {
			lif0 = d.ips[0]
		}
		return CleanBackendName("ontapsan_" + lif0)
	} else {
		return d.Config.BackendName
	}
}

// Initialize from the provided config
func (d *SANStorageDriver) Initialize(
	ctx context.Context, driverContext tridentconfig.DriverContext, configJSON string,
	commonConfig *drivers.CommonStorageDriverConfig, backendSecret map[string]string, backendUUID string,
) error {
	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Initialize", "Type": "SANStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Initialize")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Initialize")
	}

	// Initialize the driver's CommonStorageDriverConfig
	d.Config.CommonStorageDriverConfig = commonConfig

	// Parse the config
	config, err := InitializeOntapConfig(ctx, driverContext, configJSON, commonConfig, backendSecret)
	if err != nil {
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
	}
	d.Config = *config

	d.API, err = InitializeOntapDriverAbstraction(ctx, config)
	if err != nil {
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
	}
	d.Config = *config

	d.ips, err = d.API.NetInterfaceGetDataLIFs(ctx, "iscsi")
	if err != nil {
		return err
	}

	if len(d.ips) == 0 {
		return fmt.Errorf("no iSCSI data LIFs found on SVM %s", d.API.SVMName())
	} else {
		Logc(ctx).WithField("dataLIFs", d.ips).Debug("Found iSCSI LIFs.")
	}

	d.physicalPools, d.virtualPools, err = InitializeStoragePoolsCommonAbstraction(ctx, d,
		d.getStoragePoolAttributes(ctx), d.BackendName())
	if err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

	err = InitializeSANDriverAbstraction(ctx, driverContext, d.API, &d.Config, d.validate, backendUUID)

	// clean up igroup for failed driver
	if err != nil {
		if d.Config.DriverContext == tridentconfig.ContextCSI {
			err := d.API.IgroupDestroy(ctx, d.Config.IgroupName)
			if err != nil {
				Logc(ctx).WithError(err).WithField("igroup", d.Config.IgroupName).Warn("Error deleting igroup.")
			}
		}
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
	}

	// Set up the autosupport heartbeat
	d.telemetry = NewOntapTelemetryAbstraction(ctx, d)
	d.telemetry.Telemetry = tridentconfig.OrchestratorTelemetry
	d.telemetry.TridentBackendUUID = backendUUID
	d.telemetry.Start(ctx)

	d.initialized = true
	return nil
}

func (d *SANStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *SANStorageDriver) Terminate(ctx context.Context, _ string) {
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Terminate", "Type": "SANStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Terminate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Terminate")
	}

	if d.Config.DriverContext == tridentconfig.ContextCSI {
		// clean up igroup for terminated driver
		err := d.API.IgroupDestroy(ctx, d.Config.IgroupName)
		if err != nil {
			Logc(ctx).WithError(err).Warn("Error deleting igroup.")
		}
	}

	if d.telemetry != nil {
		d.telemetry.Stop()
	}
	d.initialized = false
}

// Validate the driver configuration and execution environment
func (d *SANStorageDriver) validate(ctx context.Context) error {
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "validate", "Type": "SANStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> validate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< validate")
	}

	if err := validateReplicationConfig(ctx, d.Config.ReplicationPolicy, d.Config.ReplicationSchedule,
		d.API); err != nil {
		return fmt.Errorf("replication validation failed: %v", err)
	}

	if err := ValidateSANDriver(ctx, &d.Config, d.ips); err != nil {
		return fmt.Errorf("driver validation failed: %v", err)
	}

	if err := ValidateStoragePrefix(*d.Config.StoragePrefix); err != nil {
		return err
	}

	if err := ValidateStoragePoolsAbstraction(ctx, d.physicalPools, d.virtualPools, d,
		api.MaxSANLabelLength); err != nil {
		return fmt.Errorf("storage pool validation failed: %v", err)
	}

	return nil
}

// Create a volume+LUN with the specified options
func (d *SANStorageDriver) Create(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool storage.Pool, volAttributes map[string]sa.Request,
) error {
	name := volConfig.InternalName

	var fstype string

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Create",
			"Type":   "SANStorageDriver",
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
		if err = checkSVMPeeredAbstraction(ctx, volConfig, d.API.SVMName(), d.API); err != nil {
			return err
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

	// Get options with default fallback values
	// see also: ontap_common.go#PopulateConfigurationDefaults

	spaceAllocation, _ := strconv.ParseBool(
		utils.GetV(opts, "spaceAllocation", storagePool.InternalAttributes()[SpaceAllocation]))
	var (
		spaceReserve      = utils.GetV(opts, "spaceReserve", storagePool.InternalAttributes()[SpaceReserve])
		snapshotPolicy    = utils.GetV(opts, "snapshotPolicy", storagePool.InternalAttributes()[SnapshotPolicy])
		snapshotReserve   = utils.GetV(opts, "snapshotReserve", storagePool.InternalAttributes()[SnapshotReserve])
		unixPermissions   = utils.GetV(opts, "unixPermissions", storagePool.InternalAttributes()[UnixPermissions])
		snapshotDir       = "false"
		exportPolicy      = utils.GetV(opts, "exportPolicy", storagePool.InternalAttributes()[ExportPolicy])
		securityStyle     = utils.GetV(opts, "securityStyle", storagePool.InternalAttributes()[SecurityStyle])
		encryption        = utils.GetV(opts, "encryption", storagePool.InternalAttributes()[Encryption])
		tieringPolicy     = utils.GetV(opts, "tieringPolicy", storagePool.InternalAttributes()[TieringPolicy])
		qosPolicy         = storagePool.InternalAttributes()[QosPolicy]
		adaptiveQosPolicy = storagePool.InternalAttributes()[AdaptiveQosPolicy]
	)

	snapshotReserveInt, err := GetSnapshotReserve(snapshotPolicy, snapshotReserve)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotReserve: %v", err)
	}

	// Determine volume size in bytes
	requestedSize, err := utils.ConvertSizeToBytes(volConfig.Size)
	if err != nil {
		return fmt.Errorf("could not convert volume size %s: %v", volConfig.Size, err)
	}
	requestedSizeBytes, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid volume size: %v", volConfig.Size, err)
	}
	lunSizeBytes, err := GetVolumeSize(requestedSizeBytes, storagePool.InternalAttributes()[Size])
	if err != nil {
		return err
	}
	lunSize := strconv.FormatUint(lunSizeBytes, 10)
	// Get the flexvol size based on the snapshot reserve
	flexvolSize := calculateFlexvolSizeBytes(ctx, name, lunSizeBytes, snapshotReserveInt)
	// Add extra 10% to the Flexvol to account for LUN metadata
	flexvolBufferSize := uint64(LUNMetadataBufferMultiplier * float64(flexvolSize))

	volumeSize := strconv.FormatUint(flexvolBufferSize, 10)

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, lunSizeBytes, d.Config.CommonStorageDriverConfig,
	); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	enableEncryption, err := GetEncryptionValue(encryption)
	if err != nil {
		return fmt.Errorf("invalid boolean value for encryption: %v", err)
	}

	fstype, err = drivers.CheckSupportedFilesystem(
		ctx, utils.GetV(opts, "fstype|fileSystemType", storagePool.InternalAttributes()[FileSystemType]), name)
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
	volConfig.QosPolicy = qosPolicy
	volConfig.AdaptiveQosPolicy = adaptiveQosPolicy

	Logc(ctx).WithFields(log.Fields{
		"name":              name,
		"lunSize":           lunSize,
		"flexvolSize":       flexvolBufferSize,
		"spaceAllocation":   spaceAllocation,
		"spaceReserve":      spaceReserve,
		"snapshotPolicy":    snapshotPolicy,
		"snapshotReserve":   snapshotReserveInt,
		"unixPermissions":   unixPermissions,
		"snapshotDir":       snapshotDir,
		"exportPolicy":      exportPolicy,
		"securityStyle":     securityStyle,
		"encryption":        utils.GetPrintableBoolPtrValue(enableEncryption),
		"qosPolicy":         qosPolicy,
		"adaptiveQosPolicy": adaptiveQosPolicy,
	}).Debug("Creating Flexvol.")

	createErrors := make([]error, 0)
	physicalPoolNames := make([]string, 0)

	for _, physicalPool := range physicalPools {
		aggregate := physicalPool.Name()
		physicalPoolNames = append(physicalPoolNames, aggregate)

		if aggrLimitsErr := checkAggregateLimitsAbstraction(
			ctx, aggregate, spaceReserve, flexvolBufferSize, d.Config, d.GetAPI(),
		); aggrLimitsErr != nil {
			errMessage := fmt.Sprintf("ONTAP-SAN pool %s/%s; error: %v", storagePool.Name(), aggregate, aggrLimitsErr)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))

			// Move on to the next pool
			continue
		}

		labels, err := storagePool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, api.MaxSANLabelLength)
		if err != nil {
			return err
		}

		// Create the volume
		err = d.API.VolumeCreate(
			ctx, api.Volume{
				AccessType: "",
				Aggregates: []string{
					aggregate,
				},
				Comment:         labels,
				Encrypt:         enableEncryption,
				ExportPolicy:    exportPolicy,
				JunctionPath:    "",
				Name:            name,
				Qos:             api.QosPolicyGroup{},
				SecurityStyle:   securityStyle,
				Size:            volumeSize,
				SnapshotDir:     false,
				SnapshotPolicy:  snapshotPolicy,
				SnapshotReserve: snapshotReserveInt,
				SpaceReserve:    spaceReserve,
				TieringPolicy:   tieringPolicy,
				UnixPermissions: unixPermissions,
				UUID:            "",
				DPVolume:        volConfig.IsMirrorDestination,
			})

		if err != nil {
			if api.IsVolumeCreateJobExistsError(err) {
				return nil
			}

			errMessage := fmt.Sprintf(
				"ONTAP-SAN pool %s/%s; error creating volume %s: %v", storagePool.Name(),
				aggregate, name, err,
			)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, fmt.Errorf(errMessage))

			// Move on to the next pool
			continue
		}

		lunPath := lunPath(name)
		osType := "linux"

		// If a DP volume, do not create the LUN, it will be copied over by snapmirror
		if !volConfig.IsMirrorDestination {
			// Create the LUN.  If this fails, clean up and move on to the next pool.
			// QoS policy is set at the LUN layer
			err = d.API.LunCreate(
				ctx, api.Lun{
					Name:           lunPath,
					Qos:            qosPolicyGroup,
					Size:           lunSize,
					OsType:         osType,
					SpaceReserved:  api.ToBoolPointer(false),
					SpaceAllocated: api.ToBoolPointer(spaceAllocation),
				})

			if err != nil {
				errMessage := fmt.Sprintf(
					"ONTAP-SAN pool %s/%s; error creating LUN %s: %v", storagePool.Name(),
					aggregate, name, err,
				)
				Logc(ctx).Error(errMessage)
				createErrors = append(createErrors, fmt.Errorf(errMessage))

				// Don't leave the new Flexvol around
				if err := d.API.VolumeDestroy(ctx, name, true); err != nil {
					Logc(ctx).WithField("volume", name).Errorf("Could not clean up volume; %v", err)
				} else {
					Logc(ctx).WithField("volume", name).Debugf("Cleaned up volume after LUN create error.")
				}

				// Move on to the next pool
				continue
			}

			// Save the fstype in a LUN attribute so we know what to do in Attach.  If this fails, clean up and
			// move on to the next pool.
			// TODO: Change this for LUN Attributes when available, not planning on shipping like this
			// Save the context and fstype in LUN comment
			err = d.API.LunSetAttribute(ctx, lunPath, LUNAttributeFSType, fstype, string(d.Config.DriverContext))
			if err != nil {

				errMessage := fmt.Sprintf("ONTAP-SAN pool %s/%s; error saving file system type for LUN %s: %v",
					storagePool.Name(), aggregate, name, err)
				Logc(ctx).Error(errMessage)
				createErrors = append(createErrors, fmt.Errorf(errMessage))

				// Don't leave the new LUN around
				if err := d.API.LunDestroy(ctx, lunPath); err != nil {
					Logc(ctx).WithField("LUN", lunPath).Errorf("Could not clean up LUN; %v", err)
				} else {
					Logc(ctx).WithField("volume", name).Debugf("Cleaned up LUN after set attribute error.")
				}

				// Don't leave the new Flexvol around
				if err := d.API.VolumeDestroy(ctx, name, true); err != nil {
					Logc(ctx).WithField("volume", name).Errorf("Could not clean up volume; %v", err)
				} else {
					Logc(ctx).WithField("volume", name).Debugf("Cleaned up volume after set attribute error.")
				}

				// Move on to the next pool
				continue
			}
		}
		return nil
	}

	// All physical pools that were eligible ultimately failed, so don't try this backend again
	return drivers.NewBackendIneligibleError(name, createErrors, physicalPoolNames)
}

// CreateClone creates a volume clone
func (d *SANStorageDriver) CreateClone(
	ctx context.Context, _, cloneVolConfig *storage.VolumeConfig, storagePool storage.Pool,
) error {
	name := cloneVolConfig.InternalName
	source := cloneVolConfig.CloneSourceVolumeInternal
	snapshot := cloneVolConfig.CloneSourceSnapshot

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":      "CreateClone",
			"Type":        "SANStorageDriver",
			"name":        name,
			"source":      source,
			"snapshot":    snapshot,
			"storagePool": storagePool,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateClone")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateClone")
	}

	opts, err := d.GetVolumeOpts(ctx, cloneVolConfig, make(map[string]sa.Request))
	if err != nil {
		return err
	}

	// How "splitOnClone" value gets set:
	// In the Core we first check clone's VolumeConfig for splitOnClone value
	// If it is not set then (again in Core) we check source PV's VolumeConfig for splitOnClone value
	// If we still don't have splitOnClone value then HERE we check for value in the source PV's Storage/Virtual Pool
	// If the value for "splitOnClone" is still empty then HERE we set it to backend config's SplitOnClone value

	// Attempt to get splitOnClone value based on storagePool (source Volume's StoragePool)
	var storagePoolSplitOnCloneVal string
	labels := ""
	if storage.IsStoragePoolUnset(storagePool) {
		// Set the base label
		storagePoolTemp := &storage.StoragePool{}
		storagePoolTemp.SetAttributes(map[string]sa.Offer{
			sa.Labels: sa.NewLabelOffer(d.GetConfig().Labels),
		})
		labels, err = storagePoolTemp.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, api.MaxSANLabelLength)
		if err != nil {
			return err
		}

	} else {
		storagePoolSplitOnCloneVal = storagePool.InternalAttributes()[SplitOnClone]

		// Ensure the volume exists
		flexvol, err := d.API.VolumeInfo(ctx, cloneVolConfig.CloneSourceVolumeInternal)
		if err != nil {
			return err
		} else if flexvol == nil {
			return fmt.Errorf("volume %s not found", cloneVolConfig.CloneSourceVolumeInternal)
		}

		// Get the source volume's label
		if flexvol.Comment != "" {
			labels = flexvol.Comment
		}
	}

	// If storagePoolSplitOnCloneVal is still unknown, set it to backend's default value
	if storagePoolSplitOnCloneVal == "" {
		storagePoolSplitOnCloneVal = d.Config.SplitOnClone
	}

	split, err := strconv.ParseBool(utils.GetV(opts, "splitOnClone", storagePoolSplitOnCloneVal))
	if err != nil {
		return fmt.Errorf("invalid boolean value for splitOnClone: %v", err)
	}

	qosPolicy := utils.GetV(opts, "qosPolicy", "")
	adaptiveQosPolicy := utils.GetV(opts, "adaptiveQosPolicy", "")
	qosPolicyGroup, err := api.NewQosPolicyGroup(qosPolicy, adaptiveQosPolicy)
	if err != nil {
		return err
	}

	Logc(ctx).WithField("splitOnClone", split).Debug("Creating volume clone.")
	if err = cloneFlexvol(
		ctx, name, source, snapshot, labels, split, &d.Config, d.API, api.QosPolicyGroup{},
	); err != nil {
		return err
	}

	if qosPolicyGroup.Kind != api.InvalidQosPolicyGroupKind {
		err = d.API.LunSetQosPolicyGroup(ctx, lunPath(name), qosPolicyGroup)
		if err != nil {
			return fmt.Errorf("error setting QoS policy group: %v", err)
		}
	}

	return nil
}

func (d *SANStorageDriver) Import(ctx context.Context, volConfig *storage.VolumeConfig, originalName string) error {
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "Import",
			"Type":         "SANStorageDriver",
			"originalName": originalName,
			"newName":      volConfig.InternalName,
			"notManaged":   volConfig.ImportNotManaged,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Import")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Import")
	}

	// Ensure the volume exists
	flexvol, err := d.API.VolumeInfo(ctx, originalName)
	if err != nil {
		return err
	} else if flexvol == nil {
		return fmt.Errorf("volume %s not found", originalName)
	}

	// Validate the volume is what it should be
	if !api.IsVolumeIdAttributesReadError(err) {
		if flexvol.AccessType != "" && flexvol.AccessType != "rw" {
			Logc(ctx).WithField("originalName", originalName).Error("Could not import volume, type is not rw.")
			return fmt.Errorf("volume %s type is %s, not rw", originalName, flexvol.AccessType)
		}
	}

	// Ensure the volume has only one LUN
	lunInfo, err := d.API.LunGetByName(ctx, "/vol/"+originalName+"/*")
	if err != nil {
		return err
	} else if lunInfo == nil {
		return fmt.Errorf("lun not found in volume %s", originalName)
	}
	targetPath := "/vol/" + originalName + "/lun0"

	// The LUN should be online
	if lunInfo.State != "online" {
		return fmt.Errorf("LUN %s is not online", lunInfo.Name)
	}

	// Use the LUN size
	volConfig.Size = lunInfo.Size

	// Rename the volume or LUN if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		if lunInfo.Name != targetPath {
			err = d.API.LunRename(ctx, lunInfo.Name, targetPath)
			if err != nil {
				Logc(ctx).WithField("path", lunInfo.Name).Errorf("Could not import volume, rename LUN failed: %v", err)
				return fmt.Errorf("LUN path %s rename failed: %v", lunInfo.Name, err)
			}
		}

		err = d.API.VolumeRename(ctx, originalName, volConfig.InternalName)
		if err != nil {
			Logc(ctx).WithField("originalName", originalName).Errorf(
				"Could not import volume, rename volume failed: %v", err)
			return fmt.Errorf("volume %s rename failed: %v", originalName, err)
		}
		if storage.AllowPoolLabelOverwrite(storage.ProvisioningLabelTag, flexvol.Comment) {
			err = d.API.VolumeSetComment(ctx, volConfig.InternalName, originalName, "")
			if err != nil {
				Logc(ctx).WithField("originalName", originalName).Warnf("Modifying comment failed: %v", err)
				return fmt.Errorf("volume %s modify failed: %v", originalName, err)
			}
		}
	} else {
		// Volume import is not managed by Trident
		if lunInfo.Name != targetPath {
			return fmt.Errorf("could not import volume, LUN is nammed incorrectly: %s", lunInfo.Name)
		}
		if !lunInfo.Mapped {
			return fmt.Errorf("could not import volume, LUN is not mapped: %s", lunInfo.Name)
		}
	}

	return nil
}

func (d *SANStorageDriver) Rename(ctx context.Context, name, newName string) error {
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":  "Rename",
			"Type":    "SANStorageDriver",
			"name":    name,
			"newName": newName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Rename")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Rename")
	}

	err := d.API.VolumeRename(ctx, name, newName)
	if err != nil {
		Logc(ctx).WithField("name", name).Warnf("Could not rename volume: %v", err)
		return fmt.Errorf("could not rename volume %s: %v", name, err)
	}

	return nil
}

// Destroy the requested (volume,lun) storage tuple
func (d *SANStorageDriver) Destroy(ctx context.Context, volConfig *storage.VolumeConfig) error {
	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Destroy",
			"Type":   "SANStorageDriver",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Destroy")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Destroy")
	}

	var (
		err           error
		iSCSINodeName string
		lunID         int
	)

	// Validate Flexvol exists before trying to destroy
	volExists, err := d.API.VolumeExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if !volExists {
		Logc(ctx).WithField("volume", name).Debug("Volume already deleted, skipping destroy.")
		return nil
	}

	if d.Config.DriverContext == tridentconfig.ContextDocker {

		// Get target info
		iSCSINodeName, _, err = GetISCSITargetInfoAbstraction(ctx, d.API, &d.Config)
		if err != nil {
			Logc(ctx).WithField("error", err).Error("Could not get target info.")
			return err
		}

		// Get the LUN ID
		lunPath := lunPath(name)
		lunID, err = d.API.LunMapInfo(ctx, d.Config.IgroupName, lunPath)
		if err != nil {
			return fmt.Errorf("error reading LUN maps for volume %s: %v", name, err)
		}
		if lunID >= 0 {
			// Inform the host about the device removal
			if err := utils.PrepareDeviceForRemoval(ctx, lunID, iSCSINodeName, true, false); err != nil {
				Logc(ctx).Error(err)
			}
		}
	}

	// If flexvol has been a snapmirror destination
	if err := d.API.SnapmirrorDeleteViaDestination(name, d.API.SVMName()); err != nil {
		if !api.IsNotFoundError(err) {
			return err
		}
	}

	// Delete the Flexvol & LUN
	err = d.API.VolumeDestroy(ctx, name, true)
	if err != nil {
		return fmt.Errorf("error destroying volume %v: %v", name, err)
	}

	return nil
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *SANStorageDriver) Publish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *utils.VolumePublishInfo,
) error {
	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Publish",
			"Type":   "SANStorageDriver",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Publish")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Publish")
	}

	// Check if the volume is DP or RW and don't publish if DP
	volIsRW, err := isFlexvolRWAbstraction(ctx, d.GetAPI(), name)
	if err != nil {
		return err
	}
	if !volIsRW {
		return fmt.Errorf("volume is not read-write")
	}

	lunPath := lunPath(name)
	igroupName := d.Config.IgroupName
	// Use the node specific igroup if publish enforcement is enabled
	if volConfig.AccessInfo.PublishEnforcement && tridentconfig.CurrentDriverContext == tridentconfig.ContextCSI {
		igroupName = getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
		err = ensureIGroupExistsAbstraction(ctx, d.GetAPI(), igroupName)
	}

	// Get target info
	iSCSINodeName, _, err := GetISCSITargetInfoAbstraction(ctx, d.API, &d.Config)
	if err != nil {
		return err
	}

	err = PublishLUNAbstraction(ctx, d.API, &d.Config, d.ips, publishInfo, lunPath, igroupName, iSCSINodeName)
	if err != nil {
		return fmt.Errorf("error publishing %s driver: %v", d.Name(), err)
	}
	// Fill in the volume config fields as well
	volConfig.AccessInfo = publishInfo.VolumeAccessInfo

	return nil
}

// Unpublish the volume from the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *SANStorageDriver) Unpublish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *utils.VolumePublishInfo,
) error {
	name := volConfig.InternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Unpublish",
			"Type":   "SANStorageDriver",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Unpublish")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Unpublish")
	}

	if !volConfig.AccessInfo.PublishEnforcement || tridentconfig.CurrentDriverContext != tridentconfig.ContextCSI {
		// Nothing to do if publish enforcement is not enabled
		return nil
	}

	// Unmap the LUN from the node's igroup
	igroupName := getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
	lunPath := lunPath(name)
	lunID, err := d.API.LunMapInfo(ctx, igroupName, lunPath)
	if err != nil {
		msg := fmt.Sprintf("error reading LUN maps for volume %s", name)
		Logc(ctx).WithError(err).Error(msg)
		return fmt.Errorf(msg)
	}
	if lunID >= 0 {
		err := d.API.LunUnmap(ctx, igroupName, lunPath)
		if err != nil {
			msg := "error unmapping LUN"
			Logc(ctx).WithError(err).Error(msg)
			return fmt.Errorf(msg)
		}
	}

	// Remove igroup if no LUNs are mapped to it anymore
	luns, err := d.API.IgroupListLUNsMapped(ctx, igroupName)
	if err != nil {
		msg := fmt.Sprintf("error listing LUNs mapped to igroup %s", igroupName)
		Logc(ctx).WithError(err).Error(msg)
		return fmt.Errorf(msg)
	}
	if len(luns) == 0 {
		err = d.API.IgroupDestroy(ctx, igroupName)
		if err != nil {
			msg := fmt.Sprintf("error deleting igroup %s", igroupName)
			Logc(ctx).WithError(err).Error(msg)
			return fmt.Errorf(msg)
		}
	}
	return nil
}

// CanSnapshot determines whether a snapshot as specified in the provided snapshot config may be taken.
func (d *SANStorageDriver) CanSnapshot(_ context.Context, _ *storage.SnapshotConfig, _ *storage.VolumeConfig) error {
	return nil
}

// GetSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func (d *SANStorageDriver) GetSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "GetSnapshot",
			"Type":         "SANStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetSnapshot")
	}

	return getVolumeSnapshot(ctx, snapConfig, &d.Config, d.API, d.API.LunSize)
}

// GetSnapshots returns the list of snapshots associated with the specified volume
func (d *SANStorageDriver) GetSnapshots(ctx context.Context, volConfig *storage.VolumeConfig) (
	[]*storage.Snapshot, error,
) {
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "GetSnapshots",
			"Type":       "SANStorageDriver",
			"volumeName": volConfig.InternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetSnapshots")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetSnapshots")
	}

	return getVolumeSnapshotList(ctx, volConfig, &d.Config, d.API, d.API.LunSize)
}

// CreateSnapshot creates a snapshot for the given volume
func (d *SANStorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "CreateSnapshot",
			"Type":         "SANStorageDriver",
			"snapshotName": internalSnapName,
			"sourceVolume": internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateSnapshot")
	}

	return createFlexvolSnapshot(ctx, snapConfig, &d.Config, d.API, d.API.LunSize)
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *SANStorageDriver) RestoreSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "RestoreSnapshot",
			"Type":         "SANStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> RestoreSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< RestoreSnapshot")
	}

	return RestoreSnapshotAbstraction(ctx, snapConfig, &d.Config, d.API)
}

// DeleteSnapshot creates a snapshot of a volume.
func (d *SANStorageDriver) DeleteSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "DeleteSnapshot",
			"Type":         "SANStorageDriver",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> DeleteSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< DeleteSnapshot")
	}

	err := d.API.VolumeSnapshotDelete(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName)
	if err != nil {
		if api.IsSnapshotBusyError(err) {
			// Start a split here before returning the error so a subsequent delete attempt may succeed.
			_ = SplitVolumeFromBusySnapshotAbstraction(ctx, snapConfig, &d.Config, d.API, d.API.VolumeCloneSplitStart)
		}
		// we must return the err, even if we started a split, so the snapshot delete is retried
		return err
	}

	Logc(ctx).WithField("snapshotName", snapConfig.InternalName).Debug("Deleted snapshot.")
	return nil
}

// Get tests for the existence of a volume
func (d *SANStorageDriver) Get(ctx context.Context, name string) error {
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Get", "Type": "SANStorageDriver"}
		Logc(ctx).WithFields(fields).Debug(">>>> Get")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Get")
	}

	volExists, err := d.API.VolumeExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if !volExists {
		Logc(ctx).WithField("Flexvol", name).Debug("Flexvol not found.")
		return fmt.Errorf("volume %s does not exist", name)
	}

	return nil
}

// GetStorageBackendSpecs retrieves storage backend capabilities
func (d *SANStorageDriver) GetStorageBackendSpecs(_ context.Context, backend storage.Backend) error {
	return getStorageBackendSpecsCommon(backend, d.physicalPools, d.virtualPools, d.BackendName())
}

// GetStorageBackendPhysicalPoolNames retrieves storage backend physical pools
func (d *SANStorageDriver) GetStorageBackendPhysicalPoolNames(context.Context) []string {
	return getStorageBackendPhysicalPoolNamesCommon(d.physicalPools)
}

func (d *SANStorageDriver) getStoragePoolAttributes(ctx context.Context) map[string]sa.Offer {
	client := d.GetAPI()
	mirroring, _ := client.IsSVMDRCapable(ctx)
	return map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(d.Name()),
		sa.Snapshots:        sa.NewBoolOffer(true),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.Encryption:       sa.NewBoolOffer(true),
		sa.Replication:      sa.NewBoolOffer(mirroring),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}
}

func (d *SANStorageDriver) GetVolumeOpts(
	ctx context.Context, volConfig *storage.VolumeConfig, requests map[string]sa.Request,
) (map[string]string, error) {
	return getVolumeOptsCommon(ctx, volConfig, requests), nil
}

func (d *SANStorageDriver) GetInternalVolumeName(_ context.Context, name string) string {
	return getInternalVolumeNameCommon(d.Config.CommonStorageDriverConfig, name)
}

func (d *SANStorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig) {
	if !volConfig.ImportNotManaged && tridentconfig.CurrentDriverContext == tridentconfig.ContextCSI {
		// All new CSI ONTAP SAN volumes start with publish enforcement on, unless they're unmanaged imports
		volConfig.AccessInfo.PublishEnforcement = true
	}
	createPrepareCommon(ctx, d, volConfig)
}

func (d *SANStorageDriver) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "CreateFollowup",
			"Type":         "SANStorageDriver",
			"name":         volConfig.Name,
			"internalName": volConfig.InternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateFollowup")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateFollowup")
	}

	if d.Config.DriverContext == tridentconfig.ContextDocker {
		Logc(ctx).Debug("No follow-up create actions for Docker.")
		return nil
	}

	volConfig.MirrorHandle = d.API.SVMName() + ":" + volConfig.InternalName

	// Check if the volume is RW and don't map the lun if not RW
	volIsRW, err := isFlexvolRWAbstraction(ctx, d.GetAPI(), volConfig.InternalName)
	if !volIsRW {
		return err
	}
	// Don't map at create time if publish enforcement is enabled
	if volConfig.AccessInfo.PublishEnforcement {
		return nil
	}
	return d.mapOntapSANLun(ctx, volConfig)
}

func (d *SANStorageDriver) mapOntapSANLun(ctx context.Context, volConfig *storage.VolumeConfig) error {
	// get the lunPath and lunID
	lunPath := fmt.Sprintf("/vol/%v/lun0", volConfig.InternalName)
	lunID, err := d.API.EnsureLunMapped(ctx, d.Config.IgroupName, lunPath, volConfig.ImportNotManaged)
	if err != nil {
		return err
	}

	err = PopulateOntapLunMappingAbstraction(ctx, d.API, &d.Config, d.ips, volConfig, lunID, lunPath,
		d.Config.IgroupName)
	if err != nil {
		return fmt.Errorf("error mapping LUN for %s driver: %v", d.Name(), err)
	}

	return nil
}

func (d *SANStorageDriver) GetProtocol(context.Context) tridentconfig.Protocol {
	return tridentconfig.Block
}

func (d *SANStorageDriver) StoreConfig(_ context.Context, b *storage.PersistentStorageBackendConfig) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.OntapConfig = &d.Config
}

func (d *SANStorageDriver) GetExternalConfig(ctx context.Context) interface{} {
	return getExternalConfig(ctx, d.Config)
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *SANStorageDriver) GetVolumeExternal(ctx context.Context, name string) (*storage.VolumeExternal, error) {
	volumeAttrs, err := d.API.VolumeInfo(ctx, name)
	if err != nil {
		return nil, err
	}

	lunPath := fmt.Sprintf("/vol/%v/*", name)
	lunAttrs, err := d.API.LunGetByName(ctx, lunPath)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(lunAttrs, volumeAttrs), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *SANStorageDriver) GetVolumeExternalWrappers(ctx context.Context, channel chan *storage.VolumeExternalWrapper) {
	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes matching the storage prefix
	volumes, err := d.API.VolumeListByPrefix(ctx, *d.Config.StoragePrefix)
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Get all LUNs named 'lun0' in volumes matching the storage prefix
	lunPathPattern := lunPath(*d.Config.StoragePrefix + "*")
	luns, err := d.API.LunList(ctx, lunPathPattern)
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{Volume: nil, Error: err}
		return
	}

	// Make a map of volumes for faster correlation with LUNs
	volumeMap := make(map[string]api.Volume)
	if volumes != nil {
		for _, volumeAttrs := range volumes {
			internalName := volumeAttrs.Name
			volumeMap[internalName] = *volumeAttrs
		}
	}

	// Convert all LUNs to VolumeExternal and write them to the channel
	if luns != nil {
		for idx := range luns {
			lun := &luns[idx]
			volume, ok := volumeMap[lun.VolumeName]
			if !ok {
				Logc(ctx).WithField("path", lun.Name).Warning("Flexvol not found for LUN.")
				continue
			}

			channel <- &storage.VolumeExternalWrapper{Volume: d.getVolumeExternal(lun, &volume), Error: nil}
		}
	}
}

// getVolumeExternal is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *SANStorageDriver) getVolumeExternal(
	lun *api.Lun, volume *api.Volume,
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
		Size:            lun.Size,
		Protocol:        tridentconfig.Block,
		SnapshotPolicy:  volume.SnapshotPolicy,
		ExportPolicy:    "",
		SnapshotDir:     "false",
		UnixPermissions: "",
		StorageClass:    "",
		AccessMode:      tridentconfig.ReadWriteOnce,
		AccessInfo:      utils.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
	}

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
func (d *SANStorageDriver) GetUpdateType(_ context.Context, driverOrig storage.Driver) *roaring.Bitmap {
	bitmap := roaring.New()
	dOrig, ok := driverOrig.(*SANStorageDriver)
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
func (d *SANStorageDriver) Resize(
	ctx context.Context, volConfig *storage.VolumeConfig, requestedSizeBytes uint64,
) error {
	name := volConfig.InternalName
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":             "Resize",
			"Type":               "SANStorageDriver",
			"name":               name,
			"requestedSizeBytes": requestedSizeBytes,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> Resize")
		defer Logc(ctx).WithFields(fields).Debug("<<<< Resize")
	}

	// Validation checks
	volExists, err := d.API.VolumeExists(ctx, name)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"error": err,
			"name":  name,
		}).Error("Error checking for existing volume.")
		return fmt.Errorf("error occurred checking for existing volume")
	}
	if !volExists {
		return fmt.Errorf("volume %s does not exist", name)
	}

	currentFlexvolSize, err := d.API.VolumeSize(ctx, name)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"error": err,
			"name":  name,
		}).Error("Error checking volume size.")
		return fmt.Errorf("error occurred when checking volume size")
	}

	currentLunSize, err := d.API.LunSize(ctx, name)
	if err != nil {
		return fmt.Errorf("error occurred when checking lun size")
	}

	lunSizeBytes := uint64(currentLunSize)
	if requestedSizeBytes < lunSizeBytes {
		return fmt.Errorf("requested size %d is less than existing volume size %d", requestedSizeBytes, lunSizeBytes)
	}

	snapshotReserveInt, err := getSnapshotReserveFromOntapAbstraction(ctx, name, d.API.VolumeInfo)
	if err != nil {
		Logc(ctx).WithField("name", name).Errorf("Could not get the snapshot reserve percentage for volume")
	}

	newFlexvolSize := calculateFlexvolSizeBytes(ctx, name, requestedSizeBytes, snapshotReserveInt)
	newFlexvolSize = uint64(LUNMetadataBufferMultiplier * float64(newFlexvolSize))

	sameLUNSize, err := utils.VolumeSizeWithinTolerance(int64(requestedSizeBytes), int64(currentLunSize),
		tridentconfig.SANResizeDelta)
	if err != nil {
		return err
	}

	sameFlexvolSize, err := utils.VolumeSizeWithinTolerance(int64(newFlexvolSize), int64(currentFlexvolSize),
		tridentconfig.SANResizeDelta)
	if err != nil {
		return err
	}

	if sameLUNSize && sameFlexvolSize {
		Logc(ctx).WithFields(log.Fields{
			"requestedSize":  requestedSizeBytes,
			"currentLUNSize": currentLunSize,
			"name":           name,
			"delta":          tridentconfig.SANResizeDelta,
		}).Info("Requested size and current LUN size are within the delta and therefore considered the same size" +
			" for SAN resize operations.")
		volConfig.Size = strconv.FormatUint(uint64(currentLunSize), 10)
		return nil
	}

	if aggrLimitsErr := checkAggregateLimitsForFlexvolAbstraction(
		ctx, name, newFlexvolSize, d.Config, d.GetAPI(),
	); aggrLimitsErr != nil {
		return aggrLimitsErr
	}

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, requestedSizeBytes, d.Config.CommonStorageDriverConfig,
	); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	// Resize FlexVol
	if !sameFlexvolSize {
		err := d.API.VolumeSetSize(ctx, name, strconv.FormatUint(newFlexvolSize, 10))
		if err != nil {
			Logc(ctx).WithField("error", err).Error("Volume resize failed.")
			return fmt.Errorf("volume resize failed")
		}
	}

	// Resize LUN0
	returnSize := lunSizeBytes
	if !sameLUNSize {
		returnSize, err = d.API.LunSetSize(ctx, lunPath(name), strconv.FormatUint(requestedSizeBytes, 10))
		if err != nil {
			Logc(ctx).WithField("error", err).Error("LUN resize failed.")
			return fmt.Errorf("volume resize failed")
		}
	}

	volConfig.Size = strconv.FormatUint(returnSize, 10)
	return nil
}

func (d *SANStorageDriver) ReconcileNodeAccess(ctx context.Context, nodes []*utils.Node, _ string) error {
	// Discover known nodes
	nodeNames := make([]string, 0)
	nodeIQNs := make([]string, 0)
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
		if node.IQN != "" {
			nodeIQNs = append(nodeIQNs, node.IQN)
		}
	}
	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "ReconcileNodeAccess",
			"Type":   "SANStorageDriver",
			"Nodes":  nodeNames,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> ReconcileNodeAccess")
		defer Logc(ctx).WithFields(fields).Debug("<<<< ReconcileNodeAccess")
	}

	return reconcileSANNodeAccessAbstraction(ctx, d.API, d.Config.IgroupName, nodeIQNs)
}

// String makes SANStorageDriver satisfy the Stringer interface.
func (d SANStorageDriver) String() string {
	return utils.ToStringRedacted(&d, GetOntapDriverRedactList(), d.GetExternalConfig(context.Background()))
}

// GoString makes SANStorageDriver satisfy the GoStringer interface.
func (d *SANStorageDriver) GoString() string {
	return d.String()
}

// GetCommonConfig returns driver's CommonConfig
func (d SANStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}

// EstablishMirror will create a new snapmirror relationship between a RW and a DP volume that have not previously
// had a relationship
func (d *SANStorageDriver) EstablishMirror(
	ctx context.Context, localVolumeHandle, remoteVolumeHandle, replicationPolicy, replicationSchedule string,
) error {
	// If replication policy in TMR is empty use the backend policy
	if replicationPolicy == "" {
		replicationPolicy = d.GetConfig().ReplicationPolicy
	}

	// Validate replication policy, if it is invalid, use the backend policy
	isAsync, err := validateReplicationPolicyAbstraction(ctx, replicationPolicy, d.API)
	if err != nil {
		Logc(ctx).Debugf("Replication policy given in TMR %s is invalid, using policy %s from backend.",
			replicationPolicy, d.GetConfig().ReplicationPolicy)
		replicationPolicy = d.GetConfig().ReplicationPolicy
		isAsync, err = validateReplicationPolicyAbstraction(ctx, replicationPolicy, d.API)
		if err != nil {
			Logc(ctx).Debugf("Replication policy %s in backend should be valid.", replicationPolicy)
		}
	}

	// If replication policy is async type, validate the replication schedule from TMR or use backend schedule
	if isAsync {
		if replicationSchedule != "" {
			if err := validateReplicationSchedule(ctx, replicationSchedule, d.API); err != nil {
				Logc(ctx).Debugf("Replication schedule given in TMR %s is invalid, using schedule %s from backend.",
					replicationSchedule, d.GetConfig().ReplicationSchedule)
				replicationSchedule = d.GetConfig().ReplicationSchedule
			}
		} else {
			replicationSchedule = d.GetConfig().ReplicationSchedule
		}
	} else {
		replicationSchedule = ""
	}

	return establishMirror(ctx, localVolumeHandle, remoteVolumeHandle, replicationPolicy, replicationSchedule, d.API)
}

// ReestablishMirror will attempt to resync a snapmirror relationship,
// if and only if the relationship existed previously
func (d *SANStorageDriver) ReestablishMirror(
	ctx context.Context, localVolumeHandle, remoteVolumeHandle, replicationPolicy, replicationSchedule string,
) error {
	// If replication policy in TMR is empty use the backend policy
	if replicationPolicy == "" {
		replicationPolicy = d.GetConfig().ReplicationPolicy
	}

	// Validate replication policy, if it is invalid, use the backend policy
	isAsync, err := validateReplicationPolicyAbstraction(ctx, replicationPolicy, d.API)
	if err != nil {
		Logc(ctx).Debugf("Replication policy given in TMR %s is invalid, using policy %s from backend.",
			replicationPolicy, d.GetConfig().ReplicationPolicy)
		replicationPolicy = d.GetConfig().ReplicationPolicy
		isAsync, err = validateReplicationPolicyAbstraction(ctx, replicationPolicy, d.API)
		if err != nil {
			Logc(ctx).Debugf("Replication policy %s in backend should be valid.", replicationPolicy)
		}
	}

	// If replication policy is async type, validate the replication schedule from TMR or use backend schedule
	if isAsync {
		if replicationSchedule != "" {
			if err := validateReplicationSchedule(ctx, replicationSchedule, d.API); err != nil {
				Logc(ctx).Debugf("Replication schedule given in TMR %s is invalid, using schedule %s from backend.",
					replicationSchedule, d.GetConfig().ReplicationSchedule)
				replicationSchedule = d.GetConfig().ReplicationSchedule
			}
		} else {
			replicationSchedule = d.GetConfig().ReplicationSchedule
		}
	} else {
		replicationSchedule = ""
	}

	return reestablishMirror(ctx, localVolumeHandle, remoteVolumeHandle, replicationPolicy, replicationSchedule, d.API)
}

// PromoteMirror will break the snapmirror and make the destination volume RW,
// optionally after a given snapshot has synced
func (d *SANStorageDriver) PromoteMirror(
	ctx context.Context, localVolumeHandle, remoteVolumeHandle, snapshotName string,
) (bool, error) {
	return promoteMirror(ctx, localVolumeHandle, remoteVolumeHandle, snapshotName, d.GetConfig().ReplicationPolicy,
		d.API)
}

// GetMirrorStatus returns the current state of a snapmirror relationship
func (d *SANStorageDriver) GetMirrorStatus(
	ctx context.Context, localVolumeHandle, remoteVolumeHandle string,
) (string, error) {
	return getMirrorStatus(ctx, localVolumeHandle, remoteVolumeHandle, d.API)
}

// ReleaseMirror will release the snapmirror relationship data of the source volume
func (d *SANStorageDriver) ReleaseMirror(ctx context.Context, localVolumeHandle string) error {
	return releaseMirror(ctx, localVolumeHandle, d.API)
}

// GetReplicationDetails returns the replication policy and schedule of a snapmirror relationship
func (d *SANStorageDriver) GetReplicationDetails(
	ctx context.Context, localVolumeHandle, remoteVolumeHandle string,
) (string, string, error) {
	return getReplicationDetails(ctx, localVolumeHandle, remoteVolumeHandle, d.API)
}

func (d *SANStorageDriver) GetChapInfo(_ context.Context, _, _ string) (*utils.IscsiChapInfo, error) {
	return &utils.IscsiChapInfo{
		UseCHAP:              d.Config.UseCHAP,
		IscsiUsername:        d.Config.ChapUsername,
		IscsiInitiatorSecret: d.Config.ChapInitiatorSecret,
		IscsiTargetUsername:  d.Config.ChapTargetUsername,
		IscsiTargetSecret:    d.Config.ChapTargetInitiatorSecret,
	}, nil
}

func (d *SANStorageDriver) EnablePublishEnforcement(ctx context.Context, volume *storage.Volume) error {
	// Do not enable publish enforcement on unmanaged imports
	if volume.Config.ImportNotManaged {
		return nil
	}
	err := LunUnmapAllIgroups(ctx, d.GetAPI(), lunPath(volume.Config.Name))
	if err != nil {
		msg := "error removing all mappings from LUN"
		Logc(ctx).WithError(err).Error(msg)
		return fmt.Errorf(msg)
	}

	volume.Config.AccessInfo.IscsiLunNumber = -1
	volume.Config.AccessInfo.PublishEnforcement = true
	return nil
}
