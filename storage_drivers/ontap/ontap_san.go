// Copyright 2026 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

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
	"github.com/netapp/trident/storage_drivers/ontap/awsapi"
	"github.com/netapp/trident/utils/devices/luks"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
)

func lunPath(name string) string {
	return fmt.Sprintf("/vol/%v/lun0", name)
}

func lunSizeGetterFromFlexvol(
	baseGetter func(context.Context, string) (int, error),
) func(context.Context, string) (int, error) {
	return func(ctx context.Context, volumeName string) (int, error) {
		return baseGetter(ctx, lunPath(volumeName))
	}
}

// SANStorageDriver is for iSCSI storage provisioning
type SANStorageDriver struct {
	initialized bool
	Config      drivers.OntapStorageDriverConfig
	ips         []string
	wwpns       []string
	API         api.OntapAPI
	AWSAPI      awsapi.AWSAPI
	telemetry   *Telemetry
	iscsi       iscsi.ISCSI

	physicalPools map[string]storage.Pool
	virtualPools  map[string]storage.Pool

	cloneSplitTimers *sync.Map
}

func (d *SANStorageDriver) GetConfig() drivers.DriverConfig {
	return &d.Config
}

func (d *SANStorageDriver) GetOntapConfig() *drivers.OntapStorageDriverConfig {
	return &d.Config
}

func (d *SANStorageDriver) GetAPI() api.OntapAPI {
	return d.API
}

func (d *SANStorageDriver) GetTelemetry() *Telemetry {
	return d.telemetry
}

// Name is for returning the name of this driver
func (d SANStorageDriver) Name() string {
	return tridentconfig.OntapSANStorageDriverName
}

// GetResizeDeltaBytes returns the size delta (bytes) below which this driver treats resize as a no-op.
func (d *SANStorageDriver) GetResizeDeltaBytes() int64 {
	return int64(tridentconfig.SANResizeDelta)
}

// BackendName returns the name of the backend managed by this driver instance
func (d *SANStorageDriver) BackendName() string {
	if d.Config.BackendName == "" {
		// Use the old naming scheme if no name is specified
		lif0 := "noLIFs"
		if len(d.ips) > 0 {
			lif0 = d.ips[0]
		} else if len(d.wwpns) > 0 {
			lif0 = strings.ReplaceAll(d.wwpns[0], ":", ".")
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
	fields := LogFields{"Method": "Initialize", "Type": "SANStorageDriver"}
	Logd(ctx, commonConfig.StorageDriverName,
		commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Initialize")
	defer Logd(ctx, commonConfig.StorageDriverName,
		commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Initialize")

	// Initialize the iSCSI client
	var err error
	d.iscsi, err = iscsi.New()
	if err != nil {
		return fmt.Errorf("could not initialize iSCSI client; %v", err)
	}

	if d.Config.CommonStorageDriverConfig == nil {

		// Initialize the driver's CommonStorageDriverConfig
		d.Config.CommonStorageDriverConfig = commonConfig

		// Parse the config
		config, err := InitializeOntapConfig(ctx, driverContext, configJSON, commonConfig, backendSecret)
		if err != nil {
			return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
		}

		d.Config = *config
	}

	// Initialize AWS API if applicable.
	// Unit tests mock the API layer, so we only use the real API interface if it doesn't already exist.
	if d.AWSAPI == nil {
		d.AWSAPI, err = initializeAWSDriver(ctx, &d.Config)
		if err != nil {
			return fmt.Errorf("error initializing %s AWS driver; %v", d.Name(), err)
		}
	}

	// Initialize the ONTAP API.
	// Unit tests mock the API layer, so we only use the real API interface if it doesn't already exist.
	if d.API == nil {
		if d.API, err = InitializeOntapDriver(ctx, &d.Config); err != nil {
			return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
		}
	}

	// Load default config parameters
	if err = PopulateConfigurationDefaults(ctx, &d.Config); err != nil {
		return fmt.Errorf("could not populate configuration defaults: %v", err)
	}

	if d.Config.SANType == sa.FCP {
		if d.wwpns, err = d.API.NetFcpInterfaceGetDataLIFs(ctx, d.Config.SANType); err != nil {
			return err
		}

		if len(d.wwpns) == 0 {
			return fmt.Errorf("no FC data LIFs found on SVM %s", d.API.SVMName())
		} else {
			Logc(ctx).WithField("dataLIFs", d.wwpns).Debug("Found FC LIFs.")
		}
	} else {
		if d.ips, err = d.API.NetInterfaceGetDataLIFs(ctx, d.Config.SANType); err != nil {
			return err
		}

		if len(d.ips) == 0 {
			return fmt.Errorf("no iSCSI data LIFs found on SVM %s", d.API.SVMName())
		} else {
			Logc(ctx).WithField("dataLIFs", d.ips).Debug("Found iSCSI LIFs.")
		}
	}

	d.physicalPools, d.virtualPools, err = InitializeStoragePoolsCommon(ctx, d,
		d.getStoragePoolAttributes(ctx), d.BackendName())
	if err != nil {
		return fmt.Errorf("could not configure storage pools: %v", err)
	}

	err = InitializeSANDriver(ctx, driverContext, d.API, &d.Config, d.validate, backendUUID)
	if err != nil {
		if d.Config.DriverContext == tridentconfig.ContextCSI {
			// Clean up igroup for failed driver.
			if igroupErr := d.API.IgroupDestroy(ctx, d.Config.IgroupName); igroupErr != nil {
				Logc(ctx).WithError(igroupErr).WithField("igroup", d.Config.IgroupName).Warn("Error deleting igroup.")
			}
		}
		return fmt.Errorf("error initializing %s driver: %v", d.Name(), err)
	}

	// Identify non-overlapping storage backend pools on the driver backend.
	pools, err := drivers.EncodeStorageBackendPools(ctx, commonConfig, d.getStorageBackendPools(ctx))
	if err != nil {
		return fmt.Errorf("failed to encode storage backend pools: %v", err)
	}
	d.Config.BackendPools = pools

	// Set up the autosupport heartbeat
	d.telemetry = NewOntapTelemetry(ctx, d)
	d.telemetry.Telemetry = tridentconfig.OrchestratorTelemetry
	d.telemetry.TridentBackendUUID = backendUUID
	d.telemetry.Start(ctx)

	// Set up the clone split timers
	d.cloneSplitTimers = &sync.Map{}

	d.initialized = true
	return nil
}

func (d *SANStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *SANStorageDriver) Terminate(ctx context.Context, _ string) {
	fields := LogFields{"Method": "Terminate", "Type": "SANStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Terminate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Terminate")

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
	d.API.Terminate()
	d.initialized = false
}

// Validate the driver configuration and execution environment
func (d *SANStorageDriver) validate(ctx context.Context) error {
	fields := LogFields{"Method": "validate", "Type": "SANStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> validate")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< validate")

	if err := validateReplicationConfig(ctx, d.Config.ReplicationPolicy, d.Config.ReplicationSchedule,
		d.API); err != nil {
		return fmt.Errorf("replication validation failed: %v", err)
	}

	if err := ValidateSANDriver(ctx, &d.Config, d.ips, d.iscsi); err != nil {
		return fmt.Errorf("driver validation failed: %v", err)
	}

	if err := ValidateStoragePrefix(*d.Config.StoragePrefix); err != nil {
		return err
	}

	if err := ValidateStoragePools(ctx, d.physicalPools, d.virtualPools, d,
		api.MaxSANLabelLength); err != nil {
		return fmt.Errorf("storage pool validation failed: %v", err)
	}

	return nil
}

// lunAttributes are the values Trident stamps onto a LUN once it exists. Attach reads them back to learn how
// to format and mount the LUN, so they have to survive a resumed create.
type lunAttributes struct {
	fsType         string
	driverContext  string
	luksEncryption string
	formatOptions  string
	poolName       string
}

// lunUUIDAPI addresses a LUN by UUID rather than by name. ONTAP's REST name index can lag behind a
// create, so the UUID is the reliable way to reach a LUN once ONTAP has reported one. Only the REST
// abstraction implements this; ZAPI remains path-based.
type lunUUIDAPI interface {
	LunDestroyByUUID(ctx context.Context, lunPath, lunUUID string) error
	LunGetAttributeByUUID(ctx context.Context, lunPath, lunUUID, attributeName string) (string, error)
	LunSetAttributeByUUID(
		ctx context.Context, lunPath, lunUUID, attribute, fstype, driverContext, luks, formatOptions, poolName string,
	) error
	LunSetQosPolicyGroupByUUID(
		ctx context.Context, lunPath, lunUUID string, qosPolicyGroup api.QosPolicyGroup,
	) error
	LunSetSizeByUUID(ctx context.Context, lunPath, lunUUID, newSize string) (uint64, error)
	EnsureLunMappedByUUID(ctx context.Context, initiatorGroupName, lunPath, lunUUID string) (int, error)
	LunUnmapByUUID(ctx context.Context, initiatorGroupName, lunPath, lunUUID string) error
}

// lunByVolumeUUIDAPI discovers the LUN in a FlexVol by the volume UUID. The REST abstraction implements
// this capability so SAN operations can avoid ONTAP's asynchronously updated LUN name index.
type lunByVolumeUUIDAPI interface {
	LunGetByVolumeUUID(ctx context.Context, volumeUUID, lunPath string) (*api.Lun, error)
}

// lunMapReportingNodesUUIDAPI gets reporting nodes for a LUN without resolving its name.
type lunMapReportingNodesUUIDAPI interface {
	LunMapGetReportingNodesByUUID(ctx context.Context, initiatorGroupName, lunUUID string) ([]string, error)
}

// volumeCloneUUIDAPI returns the clone UUID carried by the REST create response.
type volumeCloneUUIDAPI interface {
	VolumeCloneCreateWithUUID(ctx context.Context, cloneName, sourceName, snapshot string) (string, error)
}

var _ lunUUIDAPI = api.OntapAPIREST{}
var _ lunByVolumeUUIDAPI = api.OntapAPIREST{}
var _ lunMapReportingNodesUUIDAPI = api.OntapAPIREST{}
var _ volumeCloneUUIDAPI = api.OntapAPIREST{}

// supportsLUNLookupByVolumeUUID reports whether the backend identifies FlexVols by UUID. REST does, so it
// must report one for every volume it returns. ZAPI addresses volumes by name and never reports a UUID.
func supportsLUNLookupByVolumeUUID(ontapAPI api.OntapAPI) bool {
	_, ok := ontapAPI.(lunByVolumeUUIDAPI)
	return ok
}

// lookupBackendVolumeID reports the backend volume ID to record for a volume that already exists. A
// backend that addresses volumes by UUID must report one, because the UUID is ONTAP's key for the object
// and nothing else identifies the volume later. A backend that addresses volumes by name reports none,
// and its volumes stay reachable by name.
func lookupBackendVolumeID(ctx context.Context, ontapAPI api.OntapAPI, name string) (string, error) {
	volume, err := ontapAPI.VolumeInfo(ctx, name)
	if err != nil {
		return "", fmt.Errorf("get existing volume %s: %w", name, err)
	}
	if volume == nil {
		return "", fmt.Errorf("existing volume %s has no volume information", name)
	}
	if volume.UUID == "" && supportsLUNLookupByVolumeUUID(ontapAPI) {
		return "", fmt.Errorf("existing volume %s has no backend volume ID", name)
	}
	return volume.UUID, nil
}

// lunAPIForUUID reports the API for reaching a LUN by its own UUID, and whether that route is usable:
// the backend must address LUNs by UUID and the LUN's UUID must be known.
func lunAPIForUUID(ontapAPI api.OntapAPI, lunUUID string) (lunUUIDAPI, bool) {
	uuidAPI, ok := ontapAPI.(lunUUIDAPI)
	return uuidAPI, ok && lunUUID != ""
}

// lunAPIForVolumeUUID reports the API for reaching a LUN through its FlexVol's UUID, and whether that
// route is usable: the backend must address volumes by UUID and the volume's UUID must be known.
func lunAPIForVolumeUUID(ontapAPI api.OntapAPI, volumeUUID string) (lunByVolumeUUIDAPI, bool) {
	volumeAPI, ok := ontapAPI.(lunByVolumeUUIDAPI)
	return volumeAPI, ok && volumeUUID != ""
}

// lookupLUNForVolume finds the LUN at lunPath. A known volume UUID identifies the FlexVol exactly, so the
// LUNs that volume reports are authoritative and the name index is not consulted; lunPath only selects
// among them. A volume Trident cannot address by UUID is looked up through the name index.
func lookupLUNForVolume(
	ctx context.Context, ontapAPI api.OntapAPI, lunPath, volumeUUID string,
) (*api.Lun, error) {
	if volumeAPI, ok := lunAPIForVolumeUUID(ontapAPI, volumeUUID); ok {
		return volumeAPI.LunGetByVolumeUUID(ctx, volumeUUID, lunPath)
	}
	return ontapAPI.LunGetByName(ctx, lunPath)
}

func waitForLUNToExist(
	ctx context.Context, ontapAPI api.OntapAPI, lunPath, volumeUUID string,
) (*api.Lun, error) {
	if volumeAPI, ok := lunAPIForVolumeUUID(ontapAPI, volumeUUID); ok {
		return api.WaitForLunToExistByVolumeUUID(ctx, volumeAPI, volumeUUID, lunPath)
	}
	return api.WaitForLunToExist(ctx, ontapAPI, lunPath)
}

// updateBackendVolumeIDFromVolume records the live FlexVol UUID on volConfig when ONTAP
// returned one and it differs from what Trident already has stored.
//
// ONTAP keeps FlexVol names unique within an SVM, so a name resolves to at most one volume and a lagging
// REST name index reports no record rather than a different one. A UUID that differs from the stored one
// therefore means the volume Trident provisioned is gone and another now answers to its name. The internal
// name belongs to a single PVC, so whatever holds that name is that PVC's volume and is safe to adopt.
func updateBackendVolumeIDFromVolume(
	ctx context.Context, volConfig *storage.VolumeConfig, volume *api.Volume,
) {
	if volume == nil || volume.UUID == "" {
		return
	}
	if volConfig.BackendVolumeID == volume.UUID {
		return
	}
	Logc(ctx).WithFields(LogFields{
		"volume":     volConfig.InternalName,
		"recordedID": volConfig.BackendVolumeID,
		"liveID":     volume.UUID,
	}).Info("Refreshing stored backend volume ID from live ONTAP volume.")
	volConfig.BackendVolumeID = volume.UUID
}

// setLUNAttributes records the fstype, driver context, LUKS setting, format options and pool name on a LUN.
// The pool name is written last and marks the set as complete.
func setLUNAttributes(
	ctx context.Context, ontapAPI api.OntapAPI, lun *api.Lun, attrs lunAttributes,
) error {
	if uuidAPI, ok := ontapAPI.(lunUUIDAPI); ok {
		if lun.UUID == "" {
			return fmt.Errorf("LUN %s has no UUID", lun.Name)
		}
		return uuidAPI.LunSetAttributeByUUID(
			ctx, lun.Name, lun.UUID, LUNAttributeFSType, attrs.fsType, attrs.driverContext,
			attrs.luksEncryption, attrs.formatOptions, attrs.poolName,
		)
	}
	return ontapAPI.LunSetAttribute(
		ctx, lun.Name, LUNAttributeFSType, attrs.fsType, attrs.driverContext,
		attrs.luksEncryption, attrs.formatOptions, attrs.poolName,
	)
}

func setLUNQos(
	ctx context.Context, ontapAPI api.OntapAPI, lunPath, volumeUUID string, qosPolicyGroup api.QosPolicyGroup,
) error {
	uuidAPI, addressesByUUID := ontapAPI.(lunUUIDAPI)
	if !addressesByUUID {
		return ontapAPI.LunSetQosPolicyGroup(ctx, lunPath, qosPolicyGroup)
	}

	lun, err := lookupLUNForVolume(ctx, ontapAPI, lunPath, volumeUUID)
	if err != nil {
		return err
	}
	if lun == nil || lun.UUID == "" {
		return fmt.Errorf("LUN %s UUID not found", lunPath)
	}

	return uuidAPI.LunSetQosPolicyGroupByUUID(ctx, lunPath, lun.UUID, qosPolicyGroup)
}

// getLUNAttribute reads a single LUN attribute, addressing the LUN by UUID when the caller knows one and
// falling back to the LUN path otherwise.
func getLUNAttribute(
	ctx context.Context, ontapAPI api.OntapAPI, lunPath, lunUUID, attributeName string,
) (string, error) {
	if uuidAPI, ok := lunAPIForUUID(ontapAPI, lunUUID); ok {
		return uuidAPI.LunGetAttributeByUUID(ctx, lunPath, lunUUID, attributeName)
	}
	return ontapAPI.LunGetAttribute(ctx, lunPath, attributeName)
}

// getLUNFSType reports the filesystem type recorded on a LUN. Given a UUID it reads the fstype attribute by
// UUID and, for a LUN that records the type in its comment instead, reads it from lunComment, which the
// caller already holds from its LUN lookup. An empty attribute value is treated as absent so older LUNs that
// only stored fstype in the comment are still found. Neither read consults the LUN name index. Without a
// UUID it falls back to the name-addressed lookup.
func getLUNFSType(ctx context.Context, ontapAPI api.OntapAPI, lunPath, lunUUID, lunComment string) (string, error) {
	if uuidAPI, ok := lunAPIForUUID(ontapAPI, lunUUID); ok {
		fstype, attrErr := uuidAPI.LunGetAttributeByUUID(ctx, lunPath, lunUUID, LUNAttributeFSType)
		if attrErr == nil && fstype != "" {
			return fstype, nil
		}

		fstype, commentErr := api.FSTypeFromLunComment(lunComment)
		if commentErr != nil {
			if attrErr != nil {
				return "", attrErr
			}
			return "", commentErr
		}
		return fstype, nil
	}
	return ontapAPI.LunGetFSType(ctx, lunPath)
}

func destroyLUN(ctx context.Context, ontapAPI api.OntapAPI, lun *api.Lun) error {
	if uuidAPI, ok := lunAPIForUUID(ontapAPI, lun.UUID); ok {
		return uuidAPI.LunDestroyByUUID(ctx, lun.Name, lun.UUID)
	}
	return ontapAPI.LunDestroy(ctx, lun.Name)
}

// reconcileLUNCreateState drives a Flexvol/LUN pair left behind by an earlier Create to the state this
// request asks for, so that Create() is idempotent: it discovers what the earlier attempt left behind and
// performs the work that finishes it, repairing whatever ONTAP lets it repair rather than starting over. The
// Flexvol is known to exist and is known not to be a mirror destination.
//
// A Flexvol ONTAP has not brought online yet is still being built, so it is reported as still creating and
// left untouched: destroying it would restart a slow create on every retry, and creating its LUN now could
// fail and take the Flexvol down with it. A Flexvol or LUN that differs from this request in an attribute
// ONTAP only honors at create time is destroyed, and the returned error asks the caller to retry with a
// clean create.
//
// A nil error means the pair now matches this request. A VolumeExistsError means it already did, a
// VolumeCreatingError means ONTAP is still working and the core should retry, and any other error asks the
// caller to retry with a clean create.
func (d *SANStorageDriver) reconcileLUNCreateState(
	ctx context.Context, volConfig *storage.VolumeConfig, desiredVolume api.Volume, desiredLUN api.Lun,
	desiredAttrs lunAttributes, allowedAggregates []string,
) error {
	name := volConfig.InternalName
	poolName := desiredAttrs.poolName
	fields := LogFields{
		"Method":   "reconcileLUNCreateState",
		"Type":     "SANStorageDriver",
		"name":     name,
		"poolName": poolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> reconcileLUNCreateState")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< reconcileLUNCreateState")

	volumeUUID, err := reconcileExistingVolumeForCreate(ctx, d.API, desiredVolume, allowedAggregates)
	if err != nil {
		if errors.IsVolumeCreatingError(err) || errors.IsVolumeDeletingError(err) {
			return err
		}
		return errors.VolumeCreatingError("reconcile existing volume %s: %v", name, err)
	}
	// A backend that addresses volumes by UUID must report one for a volume it just returned; without it
	// the volume cannot be identified later. A name-addressed backend reports none and stays reachable
	// by name.
	if volumeUUID == "" && supportsLUNLookupByVolumeUUID(d.API) {
		Logc(ctx).WithField("volume", name).Warn(
			"Could not refresh backend volume ID for existing volume.",
		)
		return errors.VolumeCreatingError(
			"volume %s exists but its backend volume ID is unavailable", name,
		)
	}
	// Record the ID before any LUN work. A later step can fail with the Flexvol still in place, and
	// without the ID the cleanup or delete that follows would fall back to the name index.
	volConfig.BackendVolumeID = volumeUUID

	// Verify if LUN exists.
	newLUNPath := desiredLUN.Name
	extantLUN, err := lookupLUNForVolume(ctx, d.API, newLUNPath, volumeUUID)
	if err != nil && !errors.IsNotFoundError(err) {
		return errors.VolumeCreatingError("check for existing LUN %s: %v", newLUNPath, err)
	}

	if extantLUN == nil {
		// The volume exists without its LUN because an earlier LUN create never finished. Keep the volume,
		// now that its attributes match, and finish the job here.
		Logc(ctx).WithFields(LogFields{
			"volume": name,
			"LUN":    newLUNPath,
		}).Debug("Volume exists without its LUN; resuming LUN create on the existing volume.")

		if err = d.createLUN(ctx, name, volumeUUID, desiredLUN, desiredAttrs); err != nil {
			if api.IsVolumeCreateJobExistsError(err) {
				return errors.VolumeCreatingError("volume %s is busy: %v", name, err)
			}
			if errors.IsVolumeCreatingError(err) || errors.IsVolumeDeletingError(err) {
				return err
			}
			return errors.VolumeCreatingError("ONTAP-SAN pool %s; %v", poolName, err)
		}

		return nil
	}

	if err = reconcileExistingLun(ctx, d.API, extantLUN, desiredLUN); err != nil {
		if isUnusableVolumeError(err) {
			return destroyUnusableVolume(ctx, d.API, name, volumeUUID, err.Error())
		}
		return errors.VolumeCreatingError("reconcile existing LUN %s: %v", newLUNPath, err)
	}

	// The pool name is the last attribute LunSetAttribute writes, so a LUN carrying this pool's name also
	// carries the rest of the set from the same request.
	lunPoolName, attrErr := getLUNAttribute(ctx, d.API, newLUNPath, extantLUN.UUID, "poolName")
	if attrErr == nil && lunPoolName == poolName {
		return drivers.NewVolumeExistsError(name)
	}

	// The attributes are missing or were written for a different pool. They are only read when the LUN is
	// attached, and a LUN whose create never returned has never been attached, so rewriting them is both
	// safe and far cheaper than destroying the volume.
	Logc(ctx).WithFields(LogFields{
		"LUN":           newLUNPath,
		"lunPoolName":   lunPoolName,
		"inputPoolName": poolName,
		"error":         attrErr,
	}).Info("Pool name not found or mismatch detected. Restamping LUN attributes.")

	if err = setLUNAttributes(ctx, d.API, extantLUN, desiredAttrs); err != nil {
		return errors.VolumeCreatingError(
			"ONTAP-SAN pool %s; error saving attributes for LUN %s: %v", poolName, newLUNPath, err,
		)
	}

	return nil
}

// createLUN creates the LUN for a Flexvol and stamps the attributes Attach later reads back. Once ONTAP
// accepts the LUN create, later read-back failures leave both objects in place for the next create retry to
// reconcile. A hard LunCreate failure removes the Flexvol so the next attempt starts clean.
func (d *SANStorageDriver) createLUN(
	ctx context.Context, volumeName, volumeUUID string, lun api.Lun, attrs lunAttributes,
) error {
	if err := d.API.LunCreate(ctx, lun); err != nil {
		if api.IsVolumeCreateJobExistsError(err) {
			return err
		}
		return d.cleanupFailedLUN(ctx, volumeName, volumeUUID, nil,
			fmt.Sprintf("error creating LUN %s: %v", lun.Name, err))
	}

	createdLUN, err := waitForLUNToExist(ctx, d.API, lun.Name, volumeUUID)
	if err != nil {
		return errors.VolumeCreatingError(
			"LUN %s is not visible after ONTAP accepted its create; retrying on volume %s: %v",
			lun.Name, volumeName, err,
		)
	}

	if err := setLUNAttributes(ctx, d.API, createdLUN, attrs); err != nil {
		return d.cleanupFailedLUN(ctx, volumeName, volumeUUID, createdLUN,
			fmt.Sprintf("error saving attributes for LUN %s: %v", lun.Name, err))
	}

	return nil
}

// cleanupFailedLUN removes what a failed LUN create left behind and reports whether the Flexvol is gone or
// still being deleted, so Create does not race a new attempt against asynchronous ONTAP cleanup.
func (d *SANStorageDriver) cleanupFailedLUN(
	ctx context.Context, volumeName, volumeUUID string, lun *api.Lun, reason string,
) error {
	if lun != nil {
		if err := destroyLUN(ctx, d.API, lun); err != nil {
			Logc(ctx).WithField("LUN", lun.Name).Errorf("Could not clean up LUN; %v", err)
		} else {
			Logc(ctx).WithField("LUN", lun.Name).Debugf("Cleaned up LUN after %s.", reason)
		}
	}

	if volumeUUID == "" {
		resolvedVolumeUUID, infoErr := lookupBackendVolumeID(ctx, d.API, volumeName)
		if infoErr != nil {
			Logc(ctx).WithError(infoErr).WithField("volume", volumeName).Warn(
				"Could not resolve backend volume ID for incomplete volume cleanup; deleting by name.",
			)
		}
		volumeUUID = resolvedVolumeUUID
	}

	cleanupConfig := &storage.VolumeConfig{InternalName: volumeName, BackendVolumeID: volumeUUID}
	if err := destroyFlexvol(ctx, d.API, cleanupConfig, true, false); err != nil {
		if api.IsVolumeBusyError(err) {
			return errors.VolumeDeletingError(
				"volume %s left behind by %s is still busy deleting: %v", volumeName, reason, err)
		}
		return errors.VolumeCreatingError("could not clean up volume %s after %s: %v", volumeName, reason, err)
	}

	// ONTAP deletes a Flexvol asynchronously, so the destroy above only queued the work. Recreating the
	// same name while that delete is still running fails, so report the delete as in progress and let the
	// retry come once it finishes.
	if err := api.WaitForVolumeToBeDeleted(ctx, d.API, volumeName); err != nil {
		Logc(ctx).WithError(err).WithField("volume", volumeName).Warning(
			"Volume delete is still in progress; the create will be retried once it completes.")
		return errors.VolumeDeletingError(
			"cleaned up volume %s after %s, but its delete is still in progress; %v", volumeName, reason, err)
	}

	return errors.VolumeCreatingError("cleaned up volume %s after %s; retrying the create", volumeName, reason)
}

// Create a volume+LUN with the specified options
func (d *SANStorageDriver) Create(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool storage.Pool, volAttributes map[string]sa.Request,
) (err error) {
	ctx = NewContextBuilder(ctx).WithLayer(LogLayerOntapSANDriver).BuildContext()

	name := volConfig.InternalName
	var fstype string

	fields := LogFields{
		"Method": "Create",
		"Type":   "SANStorageDriver",
		"name":   name,
		"attrs":  volAttributes,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Create")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Create")

	// An earlier attempt may have left a Flexvol behind. Note it here so the create fails fast if the backend
	// cannot be reached; its attributes and its LUN are reconciled further down, once this request's own
	// attributes have been resolved.
	volExists, err := d.API.VolumeExists(ctx, name)
	if err != nil {
		return errors.VolumeCreatingError("check for existing volume %s: %v", name, err)
	}
	if volExists && volConfig.IsMirrorDestination {
		// A DP volume's contents arrive by snapmirror, so there is no LUN for Trident to create or check.
		volumeUUID, infoErr := lookupBackendVolumeID(ctx, d.API, name)
		if infoErr != nil {
			Logc(ctx).WithError(infoErr).WithField("volume", name).Warn(
				"Could not refresh backend volume ID for existing mirror destination.",
			)
			return errors.VolumeCreatingError(
				"volume %s exists but its backend volume ID is unavailable: %v", name, infoErr,
			)
		}
		volConfig.BackendVolumeID = volumeUUID
		return drivers.NewVolumeExistsError(name)
	}

	// If volume shall be mirrored, check that the SVM is peered with the other side
	if volConfig.PeerVolumeHandle != "" {
		if err = checkSVMPeered(ctx, volConfig, d.API.SVMName(), d.API); err != nil {
			return err
		}
	}

	// Get candidate physical pools
	physicalPools, err := getPoolsForCreate(ctx, volConfig, storagePool, volAttributes, d.physicalPools, d.virtualPools)
	if err != nil {
		return err
	}

	// Get options
	opts := d.GetVolumeOpts(ctx, volConfig, volAttributes)

	// Get options with default fallback values
	// see also: ontap_common.go#PopulateConfigurationDefaults

	spaceAllocation, _ := strconv.ParseBool(
		collection.GetV(opts, "spaceAllocation", storagePool.InternalAttributes()[SpaceAllocation]))
	var (
		spaceReserve      = collection.GetV(opts, "spaceReserve", storagePool.InternalAttributes()[SpaceReserve])
		snapshotPolicy    = collection.GetV(opts, "snapshotPolicy", storagePool.InternalAttributes()[SnapshotPolicy])
		snapshotReserve   = collection.GetV(opts, "snapshotReserve", storagePool.InternalAttributes()[SnapshotReserve])
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

	snapshotReserveInt, err := GetSnapshotReserve(snapshotPolicy, snapshotReserve)
	if err != nil {
		return fmt.Errorf("invalid value for snapshotReserve: %v", err)
	}

	// Determine volume size in bytes
	requestedSize, err := capacity.ToBytes(volConfig.Size)
	if err != nil {
		return fmt.Errorf("could not convert volume size %s: %v", volConfig.Size, err)
	}

	requestedSizeBytes, err := strconv.ParseUint(requestedSize, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid volume size: %v", volConfig.Size, err)
	}
	lunSizeBytes := GetVolumeSize(requestedSizeBytes, storagePool.InternalAttributes()[Size])

	// Add a constant overhead for LUKS volumes to account for LUKS metadata. This overhead is
	// part of the LUN but is not reported to the orchestrator.
	reportedSize := lunSizeBytes
	lunSizeBytes = incrementWithLUKSMetadataIfLUKSEnabled(ctx, lunSizeBytes, luksEncryption)
	lunSize := strconv.FormatUint(lunSizeBytes, 10)

	// Get the flexvol size based on the snapshot reserve
	flexvolSize := drivers.CalculateVolumeSizeBytes(ctx, name, lunSizeBytes, snapshotReserveInt)
	// Add extra 10% to the Flexvol to account for LUN metadata
	flexvolBufferSize := uint64(LUNMetadataBufferMultiplier * float64(flexvolSize))

	volumeSize := strconv.FormatUint(flexvolBufferSize, 10)

	if _, _, checkVolumeSizeLimitsError := drivers.CheckVolumeSizeLimits(
		ctx, lunSizeBytes, d.Config.CommonStorageDriverConfig,
	); checkVolumeSizeLimitsError != nil {
		return checkVolumeSizeLimitsError
	}

	enableEncryption, configEncryption, err := GetEncryptionValue(encryption)
	if err != nil {
		return fmt.Errorf("invalid boolean value for encryption: %v", err)
	}

	fstype, err = drivers.CheckSupportedFilesystem(
		ctx, collection.GetV(opts, "fstype|fileSystemType", storagePool.InternalAttributes()[FileSystemType]), name)
	if err != nil {
		return err
	}

	if tieringPolicy == "" {
		tieringPolicy = d.API.TieringPolicyValue(ctx)
	}

	if _, err = strconv.ParseBool(skipRecoveryQueue); skipRecoveryQueue != "" && err != nil {
		return fmt.Errorf("invalid boolean value for skipRecoveryQueue: %v", err)
	}

	qosPolicyGroup, err := api.NewQosPolicyGroup(qosPolicy, adaptiveQosPolicy)
	if err != nil {
		return err
	}

	// Update config to reflect values used to create volume
	volConfig.Size = strconv.FormatUint(reportedSize, 10)
	volConfig.SpaceReserve = spaceReserve
	volConfig.SnapshotPolicy = snapshotPolicy
	volConfig.SnapshotReserve = snapshotReserve
	volConfig.UnixPermissions = unixPermissions
	volConfig.ExportPolicy = exportPolicy
	volConfig.SecurityStyle = securityStyle
	volConfig.Encryption = configEncryption
	volConfig.SkipRecoveryQueue = skipRecoveryQueue
	volConfig.QosPolicy = qosPolicy
	volConfig.AdaptiveQosPolicy = adaptiveQosPolicy
	volConfig.LUKSEncryption = luksEncryption
	volConfig.FileSystem = fstype
	volConfig.FormatOptions = formatOptions

	Logc(ctx).WithFields(LogFields{
		"name":              name,
		"lunSize":           lunSize,
		"flexvolSize":       flexvolBufferSize,
		"spaceAllocation":   spaceAllocation,
		"spaceReserve":      spaceReserve,
		"snapshotPolicy":    snapshotPolicy,
		"snapshotReserve":   snapshotReserveInt,
		"unixPermissions":   unixPermissions,
		"exportPolicy":      exportPolicy,
		"securityStyle":     securityStyle,
		"LUKSEncryption":    luksEncryption,
		"encryption":        convert.ToPrintableBoolPtr(enableEncryption),
		"tieringPolicy":     tieringPolicy,
		"skipRecoveryQueue": skipRecoveryQueue,
		"qosPolicy":         qosPolicy,
		"adaptiveQosPolicy": adaptiveQosPolicy,
		"formatOptions":     formatOptions,
	}).Debug("Creating Flexvol.")

	// Make comment field from labels
	labels, err := ConstructLabelsFromConfigs(ctx, storagePool, volConfig,
		d.Config.CommonStorageDriverConfig, api.MaxSANLabelLength)
	if err != nil {
		return err
	}

	// The Flexvol and LUN this request asks for. The aggregate is filled in per candidate pool below; a
	// resumed create keeps the aggregate the existing Flexvol already sits on.
	desiredVolume := api.Volume{
		AccessType:      "",
		Comment:         labels,
		Encrypt:         enableEncryption,
		ExportPolicy:    exportPolicy,
		JunctionPath:    "",
		Name:            name,
		Qos:             api.QosPolicyGroup{},
		SecurityStyle:   securityStyle,
		Size:            volumeSize,
		SnapshotPolicy:  snapshotPolicy,
		SnapshotReserve: snapshotReserveInt,
		SpaceReserve:    spaceReserve,
		TieringPolicy:   tieringPolicy,
		UnixPermissions: unixPermissions,
		UUID:            "",
		DPVolume:        volConfig.IsMirrorDestination,
	}
	desiredLUN := api.Lun{
		Name:           lunPath(name),
		Qos:            qosPolicyGroup,
		Size:           lunSize,
		OsType:         "linux",
		SpaceReserved:  convert.ToPtr(false),
		SpaceAllocated: convert.ToPtr(spaceAllocation),
	}
	desiredAttrs := lunAttributes{
		fsType:         fstype,
		driverContext:  string(d.Config.DriverContext),
		luksEncryption: luksEncryption,
		formatOptions:  formatOptions,
		poolName:       storagePool.Name(),
	}

	physicalPoolNames := make([]string, 0, len(physicalPools))
	for _, physicalPool := range physicalPools {
		physicalPoolNames = append(physicalPoolNames, physicalPool.Name())
	}

	// Reconcile whatever the earlier attempt left behind against this request, repairing what ONTAP allows,
	// so a retry finishes the job instead of starting over.
	if volExists {
		return d.reconcileLUNCreateState(ctx, volConfig, desiredVolume, desiredLUN, desiredAttrs,
			physicalPoolNames)
	}

	createErrors := make([]error, 0)

	for _, physicalPool := range physicalPools {
		aggregate := physicalPool.Name()

		if aggrLimitsErr := checkAggregateLimits(
			ctx, aggregate, spaceReserve, flexvolBufferSize, d.Config, d.GetAPI(),
		); aggrLimitsErr != nil {
			errMessage := fmt.Sprintf("ONTAP-SAN pool %s/%s; error: %v", storagePool.Name(), aggregate, aggrLimitsErr)
			Logc(ctx).Error(errMessage)
			createErrors = append(createErrors, errors.New(errMessage))

			// Move on to the next pool
			continue
		}

		// Create the volume
		desiredVolume.Aggregates = []string{aggregate}
		volumeUUID, err := d.API.VolumeCreate(ctx, desiredVolume)
		if err != nil {
			if api.IsVolumeCreateJobExistsError(err) {
				Logc(ctx).WithField("volume", name).Debug("Volume create is already in progress.")
				volumeUUID, err = lookupBackendVolumeID(ctx, d.API, name)
				if err != nil {
					return errors.VolumeCreatingError(
						"could not resolve backend volume ID for volume %s after create job collision: %v", name, err,
					)
				}
			} else {
				errMessage := fmt.Sprintf(
					"ONTAP-SAN pool %s/%s; error creating volume %s: %v", storagePool.Name(),
					aggregate, name, err,
				)
				Logc(ctx).Error(errMessage)
				createErrors = append(createErrors, errors.New(errMessage))
				continue
			}
		}

		// A backend that addresses objects by UUID reports the UUID of the volume it just created,
		// because the UUID is ONTAP's key for the object. No UUID alongside a successful create means
		// the volume Trident just made cannot be identified, so stop rather than record an empty ID;
		// the retry adopts the volume through the paths that resolve an ID for a volume that exists.
		// A backend that addresses objects by name reports no UUID and stays reachable by name.
		if volumeUUID == "" && supportsLUNLookupByVolumeUUID(d.API) {
			return errors.VolumeCreatingError(
				"created volume %s but the backend reported no backend volume ID", name,
			)
		}

		// Record the ID as soon as ONTAP confirms the volume exists, before the LUN is built on top of
		// it. A LUN failure can leave the volume in place for a retry to finish, and the recorded ID is
		// what lets a later delete address that volume directly instead of through the name index.
		volConfig.BackendVolumeID = volumeUUID

		// If a DP volume, do not create the LUN, it will be copied over by snapmirror
		if !volConfig.IsMirrorDestination {
			if err = d.createLUN(ctx, name, volumeUUID, desiredLUN, desiredAttrs); err != nil {
				if api.IsVolumeCreateJobExistsError(err) {
					return errors.VolumeCreatingError("volume %s is busy: %v", name, err)
				}
				if errors.IsVolumeCreatingError(err) || errors.IsVolumeDeletingError(err) {
					return err
				}

				errMessage := fmt.Sprintf("ONTAP-SAN pool %s/%s; %v", storagePool.Name(), aggregate, err)
				Logc(ctx).Error(errMessage)
				createErrors = append(createErrors, errors.New(errMessage))
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
	snapshot := cloneVolConfig.CloneSourceSnapshotInternal

	fields := LogFields{
		"Method":      "CreateClone",
		"Type":        "SANStorageDriver",
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

	// Ensure the volume exists
	sourceFlexvol, err := d.API.VolumeInfo(ctx, cloneVolConfig.CloneSourceVolumeInternal)
	if err != nil {
		return err
	} else if sourceFlexvol == nil {
		return fmt.Errorf("volume %s not found", cloneVolConfig.CloneSourceVolumeInternal)
	}

	// Get the source volume's label
	labels := ""
	if sourceFlexvol.Comment != "" {
		labels = sourceFlexvol.Comment
	}

	var labelErr error
	if storage.IsStoragePoolUnset(storagePool) {
		// Set the base label
		storagePoolTemp := ConstructPoolForLabels(d.Config.NameTemplate, d.Config.Labels)

		if labels, labelErr = ConstructLabelsFromConfigs(ctx, storagePoolTemp, cloneVolConfig,
			d.Config.CommonStorageDriverConfig, api.MaxNASLabelLength); labelErr != nil {
			return labelErr
		}
	} else {
		storagePoolSplitOnCloneVal = storagePool.InternalAttributes()[SplitOnClone]

		if labels, labelErr = ConstructLabelsFromConfigs(ctx, storagePool, cloneVolConfig,
			d.Config.CommonStorageDriverConfig, api.MaxSANLabelLength); labelErr != nil {
			return labelErr
		}
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

	Logc(ctx).WithField("splitOnClone", split).Debug("Creating volume clone.")
	cloneErr := cloneFlexvol(
		ctx, cloneVolConfig, labels, split, &d.Config, d.API, api.QosPolicyGroup{}, true,
	)
	cloneExists := drivers.IsVolumeExistsError(cloneErr)
	if cloneErr != nil && !cloneExists {
		return cloneErr
	}

	// The clone is a new Flexvol, so record its own UUID the way Create records the UUID of a volume
	// it just created. Later deletes address the clone directly instead of through ONTAP's
	// asynchronous name index.
	if cloneVolConfig.BackendVolumeID == "" {
		cloneUUID, infoErr := lookupBackendVolumeID(ctx, d.API, name)
		if infoErr != nil {
			return errors.VolumeCreatingError(
				"clone volume %s exists but its backend volume ID is unavailable: %v", name, infoErr,
			)
		}
		cloneVolConfig.BackendVolumeID = cloneUUID
	}

	if qosPolicyGroup.Kind != api.InvalidQosPolicyGroupKind {
		err = setLUNQos(ctx, d.API, lunPath(name), cloneVolConfig.BackendVolumeID, qosPolicyGroup)
		if err != nil {
			return fmt.Errorf("error setting QoS policy group: %w", err)
		}
	}

	if cloneExists {
		return cloneErr
	}
	return nil
}

func (d *SANStorageDriver) Import(ctx context.Context, volConfig *storage.VolumeConfig, originalName string) error {
	fields := LogFields{
		"Method":       "Import",
		"Type":         "SANStorageDriver",
		"originalName": originalName,
		"newName":      volConfig.InternalName,
		"notManaged":   volConfig.ImportNotManaged,
		"noRename":     volConfig.ImportNoRename,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Import")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Import")

	// Ensure the volume exists
	flexvol, err := d.API.VolumeInfo(ctx, originalName)
	if err != nil {
		return err
	} else if flexvol == nil {
		return fmt.Errorf("volume %s not found", originalName)
	}
	updateBackendVolumeIDFromVolume(ctx, volConfig, flexvol)

	// Validate the volume is what it should be
	if !api.IsVolumeIdAttributesReadError(err) {
		if flexvol.AccessType != "" && flexvol.AccessType != "rw" {
			Logc(ctx).WithField("originalName", originalName).Error("Could not import volume, type is not rw.")
			return fmt.Errorf("volume %s type is %s, not rw", originalName, flexvol.AccessType)
		}
	}

	// Set the volume to LUKS if backend has LUKS true as default
	if volConfig.LUKSEncryption == "" {
		volConfig.LUKSEncryption = d.Config.LUKSEncryption
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

	if convert.ToBool(volConfig.LUKSEncryption) {
		newSize, err := subtractUintFromSizeString(volConfig.Size, luks.MetadataSize)
		if err != nil {
			return err
		}
		volConfig.Size = newSize
	}

	// Rename the volume or LUN if Trident will manage its lifecycle and the names are different
	if !volConfig.ImportNotManaged && !volConfig.ImportNoRename {
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
	}

	// Update volume labels if Trident will manage its lifecycle
	if !volConfig.ImportNotManaged {
		if storage.AllowPoolLabelOverwrite(storage.ProvisioningLabelTag, flexvol.Comment) {
			// Set the base label
			storagePoolTemp := ConstructPoolForLabels(d.Config.NameTemplate, d.Config.Labels)

			// Make comment field from labels
			labels, labelErr := ConstructLabelsFromConfigs(ctx, storagePoolTemp, volConfig,
				d.Config.CommonStorageDriverConfig, api.MaxNASLabelLength)
			if labelErr != nil {
				return labelErr
			}

			err = d.API.VolumeSetComment(ctx, volConfig.InternalName, originalName, labels)
			if err != nil {
				Logc(ctx).WithField("originalName", originalName).Warnf("Modifying comment failed: %v", err)
				return fmt.Errorf("volume %s modify failed: %v", originalName, err)
			}
		}
		err = LunUnmapAllIgroups(ctx, d.GetAPI(), lunPath(volConfig.InternalName))
		if err != nil {
			Logc(ctx).WithField("LUN", lunInfo.Name).Warnf("Unmapping of igroups failed: %v", err)
			return fmt.Errorf("failed to unmap igroups for LUN %s: %v", lunInfo.Name, err)
		}
	} else {
		// Volume import is not managed by Trident
		if lunInfo.Name != targetPath {
			return fmt.Errorf("could not import volume, LUN is nammed incorrectly: %s", lunInfo.Name)
		}
	}

	return nil
}

func (d *SANStorageDriver) Rename(ctx context.Context, name, newName string) error {
	fields := LogFields{
		"Method":  "Rename",
		"Type":    "SANStorageDriver",
		"name":    name,
		"newName": newName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Rename")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Rename")

	err := d.API.VolumeRename(ctx, name, newName)
	if err != nil {
		Logc(ctx).WithField("name", name).Warnf("Could not rename volume: %v", err)
		return fmt.Errorf("could not rename volume %s: %v", name, err)
	}

	return nil
}

// Destroy the requested (volume,lun) storage tuple
func (d *SANStorageDriver) Destroy(ctx context.Context, volConfig *storage.VolumeConfig) (err error) {
	ctx = NewContextBuilder(ctx).WithLayer(LogLayerOntapSANDriver).BuildContext()

	name := volConfig.InternalName

	fields := LogFields{
		"Method": "Destroy",
		"Type":   "SANStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Destroy")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Destroy")

	var (
		lunID         int
		iSCSINodeName string
	)

	defer func() {
		deleteAutomaticSnapshot(ctx, d, err, volConfig, d.API.VolumeSnapshotDelete)
	}()

	if d.Config.DriverContext == tridentconfig.ContextDocker {
		if d.Config.SANType == sa.ISCSI {
			// Get target info
			iSCSINodeName, _, err = GetISCSITargetInfo(ctx, d.API, &d.Config)
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
				publishInfo := models.VolumePublishInfo{
					DevicePath: "",
					VolumeAccessInfo: models.VolumeAccessInfo{
						IscsiAccessInfo: models.IscsiAccessInfo{
							IscsiTargetIQN: iSCSINodeName,
							IscsiLunNumber: int32(lunID),
						},
					},
				}
				drivers.RemoveSCSIDeviceByPublishInfo(ctx, &publishInfo, d.iscsi)

			}
		}
	}

	skipRecoveryQueue := false
	if skipRecoveryQueueValue, err := strconv.ParseBool(volConfig.SkipRecoveryQueue); err == nil {
		skipRecoveryQueue = skipRecoveryQueueValue
	}

	// If volume exists and this is FSx, try the FSx SDK first so that any backup mirror relationship
	// is cleaned up.  If the volume isn't found, then FSx may not know about it yet, so just try the
	// underlying ONTAP delete call.  Any race condition with FSx will be resolved on a retry.
	if d.AWSAPI != nil {
		err = destroyFSxVolume(ctx, d.AWSAPI, volConfig, &d.Config)
		if skipRecoveryQueue && (err == nil || errors.IsNotFoundError(err)) {
			purgeRecoveryQueueVolume(ctx, d.API, volConfig.InternalName)
		}
		if err == nil || !errors.IsNotFoundError(err) {
			return err
		}
	}

	// Volumes without a recorded ONTAP UUID (created before it was tracked, or on a backend that does
	// not report one) fall back to a name-based existence check so an out-of-band deletion is still
	// treated as success. Volumes with a UUID are deleted directly by UUID below, which tolerates a
	// not-yet-indexed or already-deleted volume without consulting the name index.
	if volConfig.BackendVolumeID == "" {
		volExists, volExistsErr := d.API.VolumeExists(ctx, name)
		if volExistsErr != nil {
			return fmt.Errorf("error checking for existing volume: %v", volExistsErr)
		}
		if !volExists {
			Logc(ctx).WithField("volume", name).Debug("Volume already deleted, skipping destroy.")
			return nil
		}
	}

	// If flexvol has been a snapmirror destination
	if err = d.API.SnapmirrorDeleteViaDestination(ctx, name, d.API.SVMName()); err != nil {
		if !api.IsNotFoundError(err) {
			return err
		}
	}

	// If flexvol has been a snapmirror source
	if err = d.API.SnapmirrorRelease(ctx, name, d.API.SVMName()); err != nil {
		if !api.IsNotFoundError(err) {
			return err
		}
	}

	// Delete the Flexvol & LUN, preferring delete-by-UUID so a volume ONTAP created but has not yet
	// indexed by name is still removed.
	if err = destroyFlexvol(ctx, d.API, volConfig, true, skipRecoveryQueue); err != nil {
		return fmt.Errorf("error destroying volume %v: %v", name, err)
	}

	return nil
}

// Publish the volume to the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *SANStorageDriver) Publish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *models.VolumePublishInfo,
) error {
	var nodeName string
	name := volConfig.InternalName

	fields := LogFields{
		"Method": "Publish",
		"Type":   "SANStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Publish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Publish")

	// Check if the volume is DP or RW and don't publish if DP
	flexvol, err := d.GetAPI().VolumeInfo(ctx, name)
	if err != nil {
		return fmt.Errorf("get volume %s: %w", name, err)
	}
	updateBackendVolumeIDFromVolume(ctx, volConfig, flexvol)
	// A backend that addresses volumes by UUID must report one. Publishing without it would resolve the
	// LUN through the name index and could map a LUN belonging to a different volume to the host.
	if volConfig.BackendVolumeID == "" && supportsLUNLookupByVolumeUUID(d.API) {
		return errors.VolumeCreatingError(
			"volume %s exists but its backend volume ID is unavailable", name,
		)
	}
	if flexvol.AccessType != VolTypeRW {
		return errors.New("volume is not read-write")
	}

	lunPath := lunPath(name)
	igroupName := d.Config.IgroupName

	// Use the node specific igroup if publish enforcement is enabled and this is for CSI.
	if tridentconfig.CurrentDriverContext == tridentconfig.ContextCSI {
		if d.Config.SANType == sa.FCP {
			igroupName = getNodeSpecificFCPIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
		} else {
			igroupName = getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
		}
		err = ensureIGroupExists(ctx, d.GetAPI(), igroupName, d.Config.SANType)
		if err != nil {
			return err
		}
	}

	if d.Config.SANType == sa.FCP {
		// Get FCP target info.
		FCPNodeName, _, err := GetFCPTargetInfo(ctx, d.API, &d.Config)
		if err != nil {
			return err
		}

		nodeName = FCPNodeName

	} else {
		// Get iSCSI target info.
		iSCSINodeName, _, err := GetISCSITargetInfo(ctx, d.API, &d.Config)
		if err != nil {
			return err
		}

		nodeName = iSCSINodeName
	}

	err = PublishLUN(ctx, d.API, &d.Config, d.ips, publishInfo, lunPath, igroupName, nodeName, volConfig)
	if err != nil {
		return fmt.Errorf("error publishing %s driver: %v", d.Name(), err)
	}
	// Fill in the volume access fields as well.
	volConfig.AccessInfo = publishInfo.VolumeAccessInfo

	return nil
}

// resolveLUNUnmapTarget reports the UUID of the LUN to unmap and whether an unmap is needed at all.
//
// A volume Trident cannot address by UUID reports no UUID and needs the unmap, which then reaches the LUN
// by path. A volume that can be addressed by UUID is authoritative about the LUNs it holds, so a LUN it no
// longer reports is genuinely gone and there is nothing to unmap; the LUN path must not be used to look for
// a stand-in, because anything still answering to it belongs to a different volume.
func resolveLUNUnmapTarget(
	ctx context.Context, ontapAPI api.OntapAPI, lunPath string, volConfig *storage.VolumeConfig,
) (lunUUID string, unmapNeeded bool, err error) {
	volumeAPI, ok := lunAPIForVolumeUUID(ontapAPI, volConfig.BackendVolumeID)
	if !ok {
		return "", true, nil
	}

	lun, err := volumeAPI.LunGetByVolumeUUID(ctx, volConfig.BackendVolumeID, lunPath)
	if err != nil {
		if errors.IsNotFoundError(err) {
			Logc(ctx).WithFields(LogFields{
				"LUN":        lunPath,
				"volumeUUID": volConfig.BackendVolumeID,
			}).Debug("Volume no longer holds this LUN; nothing to unmap.")
			return "", false, nil
		}
		return "", false, err
	}
	return lun.UUID, true, nil
}

// Unpublish the volume from the host specified in publishInfo.  This method may or may not be running on the host
// where the volume will be mounted, so it should limit itself to updating access rules, initiator groups, etc.
// that require some host identity (but not locality) as well as storage controller API access.
func (d *SANStorageDriver) Unpublish(
	ctx context.Context, volConfig *storage.VolumeConfig, publishInfo *models.VolumePublishInfo,
) error {
	name := volConfig.InternalName
	lunMutex.Lock(lunPath(name))
	defer lunMutex.Unlock(lunPath(name))

	fields := LogFields{
		"Method": "Unpublish",
		"Type":   "SANStorageDriver",
		"name":   name,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Unpublish")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Unpublish")

	if tridentconfig.CurrentDriverContext != tridentconfig.ContextCSI {
		return nil
	}

	// Address the LUN by UUID where the volume records one, so the unmap cannot act on a different LUN
	// that has taken over this path.
	lunUUID, unmapNeeded, err := resolveLUNUnmapTarget(ctx, d.API, lunPath(name), volConfig)
	if err != nil {
		return err
	}

	// Attempt to unmap the LUN from the per-node igroup.
	var igroupName string
	if d.Config.SANType == sa.FCP {
		igroupName = getNodeSpecificFCPIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
		igroupMutex.Lock(igroupName)
		defer igroupMutex.Unlock(igroupName)
		lunPath := lunPath(name)
		if unmapNeeded {
			if err := LunUnmapIgroup(ctx, d.API, igroupName, lunPath, lunUUID); err != nil {
				return fmt.Errorf("error unmapping LUN %s from igroup %s; %v", lunPath, igroupName, err)
			}
		}

		// Remove igroup from volume config's FCP access Info
		volConfig.AccessInfo.FCPIgroup = removeIgroupFromFCPIgroupList(volConfig.AccessInfo.FCPIgroup,
			igroupName)
	} else {
		igroupName = getNodeSpecificIgroupName(publishInfo.HostName, publishInfo.TridentUUID)
		igroupMutex.Lock(igroupName)
		defer igroupMutex.Unlock(igroupName)
		lunPath := lunPath(name)
		if unmapNeeded {
			if err := LunUnmapIgroup(ctx, d.API, igroupName, lunPath, lunUUID); err != nil {
				return fmt.Errorf("error unmapping LUN %s from igroup %s; %v", lunPath, igroupName, err)
			}
		}

		// Remove igroup from volume config's iscsi access Info
		volConfig.AccessInfo.IscsiIgroup = removeIgroupFromIscsiIgroupList(volConfig.AccessInfo.IscsiIgroup, igroupName)
	}

	// Remove igroup if no LUNs are mapped.
	if err := DestroyUnmappedIgroup(ctx, d.API, igroupName); err != nil {
		return fmt.Errorf("error removing empty igroup; %v", err)
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
	fields := LogFields{
		"Method":       "GetSnapshot",
		"Type":         "SANStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshot")

	return getVolumeSnapshot(ctx, snapConfig, &d.Config, d.API, lunSizeGetterFromFlexvol(d.API.LunSize))
}

// GetSnapshots returns the list of snapshots associated with the specified volume
func (d *SANStorageDriver) GetSnapshots(ctx context.Context, volConfig *storage.VolumeConfig) (
	[]*storage.Snapshot, error,
) {
	fields := LogFields{
		"Method":     "GetSnapshots",
		"Type":       "SANStorageDriver",
		"volumeName": volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshots")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshots")

	return getVolumeSnapshotList(ctx, volConfig, &d.Config, d.API, lunSizeGetterFromFlexvol(d.API.LunSize))
}

// CreateSnapshot creates a snapshot for the given volume
func (d *SANStorageDriver) CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "CreateSnapshot",
		"Type":         "SANStorageDriver",
		"snapshotName": internalSnapName,
		"sourceVolume": internalVolName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateSnapshot")

	return createFlexvolSnapshot(ctx, snapConfig, &d.Config, d.API, lunSizeGetterFromFlexvol(d.API.LunSize))
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func (d *SANStorageDriver) RestoreSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	fields := LogFields{
		"Method":       "RestoreSnapshot",
		"Type":         "SANStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> RestoreSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< RestoreSnapshot")

	return RestoreSnapshot(ctx, snapConfig, &d.Config, d.API)
}

// DeleteSnapshot creates a snapshot of a volume.
func (d *SANStorageDriver) DeleteSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, _ *storage.VolumeConfig,
) error {
	fields := LogFields{
		"Method":       "DeleteSnapshot",
		"Type":         "SANStorageDriver",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> DeleteSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< DeleteSnapshot")

	err := d.API.VolumeSnapshotDelete(ctx, snapConfig.InternalName, snapConfig.VolumeInternalName)
	if err != nil {
		if api.IsSnapshotBusyError(err) {
			// Start a split here before returning the error so a subsequent delete attempt may succeed.
			SplitVolumeFromBusySnapshotWithDelay(ctx, snapConfig, &d.Config, d.API,
				d.API.VolumeCloneSplitStart, d.cloneSplitTimers)
		}

		// We must return the error, even if we started a split, so the snapshot delete is retried.
		return err
	}

	// Clean up any split timer
	d.cloneSplitTimers.Delete(snapConfig.ID())

	Logc(ctx).WithField("snapshotName", snapConfig.InternalName).Debug("Deleted snapshot.")
	return nil
}

// GetGroupSnapshotTarget returns a set of information about the target of a group snapshot.
// This information is used to gather information in a consistent way across storage drivers.
func (d *SANStorageDriver) GetGroupSnapshotTarget(
	ctx context.Context, volConfigs []*storage.VolumeConfig,
) (*storage.GroupSnapshotTargetInfo, error) {
	fields := LogFields{
		"Method": "GetGroupSnapshotTarget",
		"Type":   "SANStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetGroupSnapshotTarget")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetGroupSnapshotTarget")

	return GetGroupSnapshotTarget(ctx, volConfigs, &d.Config, d.API)
}

func (d *SANStorageDriver) CreateGroupSnapshot(
	ctx context.Context, config *storage.GroupSnapshotConfig, target *storage.GroupSnapshotTargetInfo,
) error {
	fields := LogFields{
		"Method": "CreateGroupSnapshot",
		"Type":   "SANStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateGroupSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateGroupSnapshot")

	return CreateGroupSnapshot(ctx, config, target, &d.Config, d.API)
}

func (d *SANStorageDriver) ProcessGroupSnapshot(
	ctx context.Context, config *storage.GroupSnapshotConfig, volConfigs []*storage.VolumeConfig,
) ([]*storage.Snapshot, error) {
	fields := LogFields{
		"Method": "ProcessGroupSnapshot",
		"Type":   "SANStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ProcessGroupSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ProcessGroupSnapshot")

	sizeGetter := lunSizeGetterFromFlexvol(d.API.LunSize)
	return ProcessGroupSnapshot(ctx, config, volConfigs, &d.Config, d.API, sizeGetter)
}

func (d *SANStorageDriver) ConstructGroupSnapshot(
	ctx context.Context, config *storage.GroupSnapshotConfig, snapshots []*storage.Snapshot,
) (*storage.GroupSnapshot, error) {
	fields := LogFields{
		"Method": "ConstructGroupSnapshot",
		"Type":   "SANStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ConstructGroupSnapshot")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ConstructGroupSnapshot")

	return ConstructGroupSnapshot(ctx, config, snapshots, &d.Config)
}

// Get tests for the existence of a volume
func (d *SANStorageDriver) Get(ctx context.Context, volConfig *storage.VolumeConfig) error {
	name := volConfig.InternalName
	fields := LogFields{"Method": "Get", "Type": "SANStorageDriver"}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Get")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Get")

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

// getStorageBackendPools determines any non-overlapping, discrete storage pools present on a driver's storage backend.
func (d *SANStorageDriver) getStorageBackendPools(ctx context.Context) []drivers.OntapStorageBackendPool {
	fields := LogFields{"Method": "getStorageBackendPools", "Type": "SANStorageDriver"}
	Logc(ctx).WithFields(fields).Debug(">>>> getStorageBackendPools")
	defer Logc(ctx).WithFields(fields).Debug("<<<< getStorageBackendPools")

	// For this driver, a discrete storage pool is composed of the following:
	// 1. SVM UUID
	// 2. Aggregate (physical pool)
	svmUUID := d.GetAPI().GetSVMUUID()
	backendPools := make([]drivers.OntapStorageBackendPool, 0)
	for _, pool := range d.physicalPools {
		backendPool := drivers.OntapStorageBackendPool{
			SvmUUID:   svmUUID,
			Aggregate: pool.Name(),
		}
		backendPools = append(backendPools, backendPool)
	}

	return backendPools
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
) map[string]string {
	return getVolumeOptsCommon(ctx, volConfig, requests)
}

func (d *SANStorageDriver) GetInternalVolumeName(
	ctx context.Context, volConfig *storage.VolumeConfig, pool storage.Pool,
) string {
	return getInternalVolumeNameCommon(ctx, &d.Config, volConfig, pool)
}

func (d *SANStorageDriver) CreatePrepare(ctx context.Context, volConfig *storage.VolumeConfig, pool storage.Pool) {
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

func (d *SANStorageDriver) CreateFollowup(ctx context.Context, volConfig *storage.VolumeConfig) error {
	fields := LogFields{
		"Method":       "CreateFollowup",
		"Type":         "SANStorageDriver",
		"name":         volConfig.Name,
		"internalName": volConfig.InternalName,
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateFollowup")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateFollowup")
	Logc(ctx).Debug("No follow-up create actions for ontap-san volume.")

	return nil
}

func (d *SANStorageDriver) GetProtocol(context.Context) tridentconfig.Protocol {
	return tridentconfig.Block
}

func (d *SANStorageDriver) StoreConfig(_ context.Context, b *storage.PersistentStorageBackendConfig) {
	config := d.Config.SmartCopy()
	drivers.SanitizeCommonStorageDriverConfig(config.CommonStorageDriverConfig)
	b.OntapConfig = config
}

func (d *SANStorageDriver) GetExternalConfig(ctx context.Context) interface{} {
	return getExternalConfig(ctx, d.Config)
}

// GetVolumeForImport queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.  For this driver, volumeID is the name of the
// Flexvol on the storage system.
func (d *SANStorageDriver) GetVolumeForImport(ctx context.Context, volumeID string) (*storage.VolumeExternal, error) {
	volumeAttrs, err := d.API.VolumeInfo(ctx, volumeID)
	if err != nil {
		return nil, err
	}

	lunPath := fmt.Sprintf("/vol/%v/*", volumeID)
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
		UnixPermissions: "",
		StorageClass:    "",
		AccessMode:      tridentconfig.ReadWriteOnce,
		AccessInfo:      models.VolumeAccessInfo{},
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
	fields := LogFields{
		"Method":             "Resize",
		"Type":               "SANStorageDriver",
		"name":               name,
		"requestedSizeBytes": requestedSizeBytes,
	}
	Logd(ctx, d.Config.StorageDriverName, d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> Resize")
	defer Logd(ctx, d.Config.StorageDriverName,
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< Resize")

	if requestedSizeBytes > math.MaxInt64 {
		Logc(ctx).WithFields(fields).Error("Invalid volume size.")
		return errors.New("invalid volume size")
	}

	// Validation checks
	volExists, err := d.API.VolumeExists(ctx, name)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
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
		Logc(ctx).WithFields(LogFields{
			"error": err,
			"name":  name,
		}).Error("Error checking volume size.")
		return fmt.Errorf("error occurred when checking volume size")
	}

	currentLunSize, err := d.API.LunSize(ctx, lunPath(name))
	if err != nil {
		return fmt.Errorf("error occurred when checking lun size")
	}

	lunSizeBytes := uint64(currentLunSize)
	if requestedSizeBytes < lunSizeBytes {
		return fmt.Errorf("requested size %d is less than existing volume size %d", requestedSizeBytes, lunSizeBytes)
	}

	// Add a constant overhead for LUKS volumes to account for LUKS metadata.
	requestedSizeBytes = incrementWithLUKSMetadataIfLUKSEnabled(ctx, requestedSizeBytes, volConfig.LUKSEncryption)

	volumeInfo, err := d.API.VolumeInfo(ctx, name)
	if err != nil {
		return fmt.Errorf("get volume %s metadata for resize: %w", name, err)
	}
	updateBackendVolumeIDFromVolume(ctx, volConfig, volumeInfo)

	snapshotReserveInt, err := snapshotReserveFromVolume(volumeInfo)
	if err != nil {
		Logc(ctx).WithField("name", name).Errorf("Could not get the snapshot reserve percentage for volume")
	}

	newFlexvolSize := drivers.CalculateVolumeSizeBytes(ctx, name, requestedSizeBytes, snapshotReserveInt)

	newFlexvolSize = uint64(LUNMetadataBufferMultiplier * float64(newFlexvolSize))

	sameLUNSize := capacity.VolumeSizeWithinTolerance(int64(requestedSizeBytes), int64(currentLunSize),
		tridentconfig.SANResizeDelta)

	sameFlexvolSize := capacity.VolumeSizeWithinTolerance(int64(newFlexvolSize), int64(currentFlexvolSize),
		tridentconfig.SANResizeDelta)

	if sameLUNSize && sameFlexvolSize {
		Logc(ctx).WithFields(LogFields{
			"requestedSize":  requestedSizeBytes,
			"currentLUNSize": currentLunSize,
			"name":           name,
			"delta":          tridentconfig.SANResizeDelta,
		}).Info("Requested size and current LUN size are within the delta and therefore considered the same size" +
			" for SAN resize operations.")
		volConfig.Size = strconv.FormatUint(uint64(currentLunSize), 10)
		return nil
	}

	if !sameFlexvolSize && (currentFlexvolSize > newFlexvolSize) {
		Logc(ctx).WithFields(LogFields{
			"requestedSize":      requestedSizeBytes,
			"currentFlexvolSize": currentFlexvolSize,
			"name":               name,
		}).Info("Requested size is less than current FlexVol size. FlexVol will not be resized.")
	}

	if aggrLimitsErr := checkAggregateLimitsForFlexvol(
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
	if !sameFlexvolSize && (currentFlexvolSize < newFlexvolSize) {
		err := d.API.VolumeSetSize(ctx, name, strconv.FormatUint(newFlexvolSize, 10))
		if err != nil {
			Logc(ctx).WithField("error", err).Error("Volume resize failed.")
			return errors.New("volume resize failed")
		}
	}

	// Resize LUN0
	returnSize := lunSizeBytes
	if !sameLUNSize {
		returnSize, err = d.API.LunSetSize(ctx, lunPath(name), strconv.FormatUint(requestedSizeBytes, 10))
		if err != nil {
			Logc(ctx).WithField("error", err).Error("LUN resize failed.")
			return errors.New("volume resize failed")
		}
	}

	// LUKS metadata size is not reported so remove it from LUN size
	returnSize = decrementWithLUKSMetadataIfLUKSEnabled(ctx, returnSize, volConfig.LUKSEncryption)

	volConfig.Size = strconv.FormatUint(returnSize, 10)
	return nil
}

func (d *SANStorageDriver) ReconcileNodeAccess(
	ctx context.Context, nodes []*models.Node, backendUUID, tridentUUID string,
) error {
	nodeNames := make([]string, len(nodes))
	for _, n := range nodes {
		nodeNames = append(nodeNames, n.Name)
	}
	fields := LogFields{
		"Method": "ReconcileNodeAccess",
		"Type":   "SANStorageDriver",
		"Nodes":  nodeNames,
	}

	Logd(ctx, d.Config.StorageDriverName,
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileNodeAccess")
	defer Logd(ctx, d.Config.StorageDriverName,
		d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileNodeAccess")

	return reconcileSANNodeAccess(ctx, d.API, nodeNames, backendUUID, tridentUUID)
}

func (d *SANStorageDriver) ReconcileVolumeNodeAccess(
	ctx context.Context, _ *storage.VolumeConfig, _ []*models.Node,
) error {
	fields := LogFields{
		"Method": "ReconcileVolumeNodeAccess",
		"Type":   "SANStorageDriver",
	}
	Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ReconcileVolumeNodeAccess")
	defer Logd(ctx, d.Name(), d.Config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ReconcileVolumeNodeAccess")

	return nil
}

// GetBackendState returns the reason if SVM is offline, and a flag to indicate if there is change
// in physical pools list.
func (d *SANStorageDriver) GetBackendState(ctx context.Context) (string, *roaring.Bitmap) {
	Logc(ctx).Debug(">>>> GetBackendState")
	defer Logc(ctx).Debug("<<<< GetBackendState")

	return getSVMState(ctx, d.API, d.Config.SANType, d.GetStorageBackendPhysicalPoolNames(ctx), d.Config.Aggregate)
}

// String makes SANStorageDriver satisfy the Stringer interface.
func (d SANStorageDriver) String() string {
	return convert.ToStringRedacted(&d, GetOntapDriverRedactList(), d.GetExternalConfig(context.Background()))
}

// GoString makes SANStorageDriver satisfy the GoStringer interface.
func (d *SANStorageDriver) GoString() string {
	return d.String()
}

// GetCommonConfig returns driver's CommonConfig
func (d SANStorageDriver) GetCommonConfig(context.Context) *drivers.CommonStorageDriverConfig {
	return d.Config.CommonStorageDriverConfig
}

// EstablishMirror will create a new mirror relationship between a RW and a DP volume that have not previously
// had a relationship
func (d *SANStorageDriver) EstablishMirror(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle, replicationPolicy, replicationSchedule string,
) error {
	// If replication policy in TMR is empty use the backend policy
	if replicationPolicy == "" {
		replicationPolicy = d.Config.ReplicationPolicy
	}

	// Validate replication policy, if it is invalid, use the backend policy
	isAsync, err := validateReplicationPolicy(ctx, replicationPolicy, d.API)
	if err != nil {
		Logc(ctx).Debugf("Replication policy given in TMR %s is invalid, using policy %s from backend.",
			replicationPolicy, d.Config.ReplicationPolicy)
		replicationPolicy = d.Config.ReplicationPolicy
		isAsync, err = validateReplicationPolicy(ctx, replicationPolicy, d.API)
		if err != nil {
			Logc(ctx).Debugf("Replication policy %s in backend should be valid.", replicationPolicy)
		}
	}

	// If replication policy is async type, validate the replication schedule from TMR or use backend schedule
	if isAsync {
		if replicationSchedule != "" {
			if err := validateReplicationSchedule(ctx, replicationSchedule, d.API); err != nil {
				Logc(ctx).Debugf("Replication schedule given in TMR %s is invalid, using schedule %s from backend.",
					replicationSchedule, d.Config.ReplicationSchedule)
				replicationSchedule = d.Config.ReplicationSchedule
			}
		} else {
			replicationSchedule = d.Config.ReplicationSchedule
		}
	} else {
		replicationSchedule = ""
	}

	return establishMirror(ctx, localInternalVolumeName, remoteVolumeHandle, replicationPolicy, replicationSchedule,
		d.API)
}

// ReestablishMirror will attempt to resync a mirror relationship, if and only if the relationship existed previously
func (d *SANStorageDriver) ReestablishMirror(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle, replicationPolicy, replicationSchedule string,
) error {
	// If replication policy in TMR is empty use the backend policy
	if replicationPolicy == "" {
		replicationPolicy = d.Config.ReplicationPolicy
	}

	// Validate replication policy, if it is invalid, use the backend policy
	isAsync, err := validateReplicationPolicy(ctx, replicationPolicy, d.API)
	if err != nil {
		Logc(ctx).Debugf("Replication policy given in TMR %s is invalid, using policy %s from backend.",
			replicationPolicy, d.Config.ReplicationPolicy)
		replicationPolicy = d.Config.ReplicationPolicy
		isAsync, err = validateReplicationPolicy(ctx, replicationPolicy, d.API)
		if err != nil {
			Logc(ctx).Debugf("Replication policy %s in backend should be valid.", replicationPolicy)
		}
	}

	// If replication policy is async type, validate the replication schedule from TMR or use backend schedule
	if isAsync {
		if replicationSchedule != "" {
			if err := validateReplicationSchedule(ctx, replicationSchedule, d.API); err != nil {
				Logc(ctx).Debugf("Replication schedule given in TMR %s is invalid, using schedule %s from backend.",
					replicationSchedule, d.Config.ReplicationSchedule)
				replicationSchedule = d.Config.ReplicationSchedule
			}
		} else {
			replicationSchedule = d.Config.ReplicationSchedule
		}
	} else {
		replicationSchedule = ""
	}

	return reestablishMirror(ctx, localInternalVolumeName, remoteVolumeHandle, replicationPolicy, replicationSchedule,
		d.API)
}

// PromoteMirror will break the mirror relationship and make the destination volume RW,
// optionally after a given snapshot has synced
func (d *SANStorageDriver) PromoteMirror(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle, snapshotName string,
) (bool, error) {
	return promoteMirror(ctx, localInternalVolumeName, remoteVolumeHandle, snapshotName,
		d.Config.ReplicationPolicy, d.API)
}

// GetMirrorStatus returns the current state of a mirror relationship
func (d *SANStorageDriver) GetMirrorStatus(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle string,
) (string, error) {
	return getMirrorStatus(ctx, localInternalVolumeName, remoteVolumeHandle, d.API)
}

// ReleaseMirror will release the mirror relationship data of the source volume
func (d *SANStorageDriver) ReleaseMirror(ctx context.Context, localInternalVolumeName string) error {
	return releaseMirror(ctx, localInternalVolumeName, d.API)
}

// GetReplicationDetails returns the replication policy and schedule of a mirror relationship
func (d *SANStorageDriver) GetReplicationDetails(
	ctx context.Context, localInternalVolumeName, remoteVolumeHandle string,
) (string, string, string, error) {
	return getReplicationDetails(ctx, localInternalVolumeName, remoteVolumeHandle, d.API)
}

// UpdateMirror will attempt a mirror update for the given destination volume
func (d *SANStorageDriver) UpdateMirror(ctx context.Context, localInternalVolumeName, snapshotName string) error {
	return mirrorUpdate(ctx, localInternalVolumeName, snapshotName, d.API)
}

// CheckMirrorTransferState will look at the transfer state of the mirror relationship to determine if it is failed,
// succeeded or in progress
func (d *SANStorageDriver) CheckMirrorTransferState(
	ctx context.Context, localInternalVolumeName string,
) (*time.Time, error) {
	return checkMirrorTransferState(ctx, localInternalVolumeName, d.API)
}

// GetMirrorTransferTime will return the transfer time of the mirror relationship
func (d *SANStorageDriver) GetMirrorTransferTime(
	ctx context.Context, localInternalVolumeName string,
) (*time.Time, error) {
	return getMirrorTransferTime(ctx, localInternalVolumeName, d.API)
}

func (d *SANStorageDriver) GetChapInfo(_ context.Context, _, _ string) (*models.IscsiChapInfo, error) {
	return &models.IscsiChapInfo{
		UseCHAP:              d.Config.UseCHAP,
		IscsiUsername:        d.Config.ChapUsername,
		IscsiInitiatorSecret: d.Config.ChapInitiatorSecret,
		IscsiTargetUsername:  d.Config.ChapTargetUsername,
		IscsiTargetSecret:    d.Config.ChapTargetInitiatorSecret,
	}, nil
}

// EnablePublishEnforcement prepares a volume for per-node igroup mapping allowing greater access control.
func (d *SANStorageDriver) EnablePublishEnforcement(ctx context.Context, volume *storage.Volume) error {
	return EnableSANPublishEnforcement(ctx, d.GetAPI(), volume.Config, lunPath(volume.Config.InternalName))
}

func (d *SANStorageDriver) CanEnablePublishEnforcement() bool {
	return true
}

func (d *SANStorageDriver) HealVolumePublishEnforcement(ctx context.Context, volume *storage.Volume) bool {
	return HealSANPublishEnforcement(ctx, d, volume)
}
