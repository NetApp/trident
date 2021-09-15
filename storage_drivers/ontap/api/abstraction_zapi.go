// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"

	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
)

func (d OntapAPIZAPI) ValidateAPIVersion(ctx context.Context) error {

	// Make sure we're using a valid ONTAP version
	ontapVersion, err := d.APIVersion(ctx)
	if err != nil {
		return fmt.Errorf("could not determine Data ONTAP API version: %v", err)
	}

	if !d.SupportsFeature(ctx, MinimumONTAPIVersion) {
		return errors.New("ONTAP 9.3 or later is required")
	}

	Logc(ctx).WithField("Ontapi", ontapVersion).Debug("ONTAP API version.")

	return nil
}

func (d OntapAPIZAPI) VolumeCreate(ctx context.Context, volume Volume) error {

	if d.API.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "VolumeCreate",
			"Type":   "OntapAPIZAPI",
			"spec":   volume,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> VolumeCreate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< VolumeCreate")
	}

	volCreateResponse, err := d.API.VolumeCreate(ctx, volume.Name, volume.Aggregates[0], volume.Size, volume.SpaceReserve,
		volume.SnapshotPolicy, volume.UnixPermissions, volume.ExportPolicy, volume.SecurityStyle, volume.TieringPolicy,
		volume.Comment, volume.Qos, volume.Encrypt, volume.SnapshotReserve, volume.DPVolume)
	if err != nil {
		return fmt.Errorf("error creating volume: %v", err)
	}

	if volCreateResponse == nil {
		return fmt.Errorf("missing volume create response")
	}

	if err = GetError(ctx, volCreateResponse, err); err != nil {
		if zerr, ok := err.(ZapiError); ok {
			// Handle case where the Create is passed to every Docker Swarm node
			if zerr.Code() == azgo.EAPIERROR && strings.HasSuffix(strings.TrimSpace(zerr.Reason()), "Job exists") {
				Logc(ctx).WithField("volume", volume.Name).Warn("Volume create job already exists, " +
					"skipping volume create on this node.")
				err = VolumeCreateJobExistsError(fmt.Sprintf("volume create job already exists, %s",
					volume.Name))
			}
		}
	}

	return err
}

func (d OntapAPIZAPI) VolumeDestroy(ctx context.Context, name string, force bool) error {

	volDestroyResponse, err := d.API.VolumeDestroy(name, force)
	if err != nil {
		return fmt.Errorf("error destroying volume %v: %v", name, err)
	}

	if zerr := NewZapiError(volDestroyResponse); !zerr.IsPassed() {

		// It's not an error if the volume no longer exists
		if zerr.Code() == azgo.EVOLUMEDOESNOTEXIST {
			Logc(ctx).WithField("volume", name).Warn("Volume already deleted.")
		} else {
			return fmt.Errorf("error destroying volume %v: %v", name, zerr)
		}
	}

	return nil
}

func (d OntapAPIZAPI) VolumeInfo(ctx context.Context, name string) (*Volume, error) {

	if d.API.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "VolumeInfo",
			"Type":   "OntapAPIZAPI",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> VolumeInfo")
		defer Logc(ctx).WithFields(fields).Debug("<<<< VolumeInfo")
	}

	// Get Flexvol by name
	volumeGetResponse, err := d.API.VolumeGet(name)

	Logc(ctx).WithFields(log.Fields{
		"volumeGetResponse": volumeGetResponse,
		"name":              name,
		"err":               err,
	}).Debug("d.API.VolumeGet")

	if err != nil {
		Logc(ctx).Errorf("Could not find volume with name: %v; error: %v", name, err.Error())
		return nil, VolumeReadError(fmt.Sprintf("could not find volume with name %s", name))
	}

	volumeInfo, err := VolumeInfoFromZapiAttrsHelper(volumeGetResponse)
	if err != nil {
		return nil, err
	}

	return volumeInfo, nil
}

func VolumeInfoFromZapiAttrsHelper(volumeGetResponse *azgo.VolumeAttributesType) (*Volume, error) {
	var responseAccessType string
	var responseAggregates []string
	var responseComment string
	var responseExportPolicy string
	var responseJunctionPath string
	var responseName string
	var responseSize string
	var responseSnapdirAccessEnabled bool
	var responseSnapshotPolicy string
	var responseSnapshotReserveInt int
	var responseSpaceReserve string
	var responseUnixPermissions string

	if volumeGetResponse.VolumeIdAttributesPtr != nil {
		volumeIdAttributes := volumeGetResponse.VolumeIdAttributes()

		if volumeIdAttributes.ContainingAggregateNamePtr != nil {
			responseAggregates = []string{
				volumeIdAttributes.ContainingAggregateName(),
			}
		}

		if volumeGetResponse.VolumeIdAttributesPtr.TypePtr != nil {
			responseAccessType = volumeIdAttributes.Type()
		}

		if volumeGetResponse.VolumeIdAttributesPtr.CommentPtr != nil {
			responseComment = volumeIdAttributes.Comment()
		}

		if volumeGetResponse.VolumeIdAttributesPtr.NamePtr != nil {
			responseName = volumeIdAttributes.Name()
		}

		if volumeGetResponse.VolumeIdAttributesPtr.JunctionPathPtr != nil {
			responseJunctionPath = volumeIdAttributes.JunctionPath()
		}
	} else {
		return nil, VolumeIdAttributesReadError(fmt.Sprintf("error reading id attributes for volume %s",
			responseName))
	}

	if volumeGetResponse.VolumeSpaceAttributesPtr != nil {
		volumeSpaceAttrs := volumeGetResponse.VolumeSpaceAttributes()
		if volumeSpaceAttrs.PercentageSnapshotReservePtr != nil {
			responseSnapshotReserveInt = volumeSpaceAttrs.PercentageSnapshotReserve()
		}

		if volumeGetResponse.VolumeSpaceAttributesPtr.SpaceGuaranteePtr != nil {
			responseSpaceReserve = volumeGetResponse.VolumeSpaceAttributesPtr.SpaceGuarantee()
		}
		if volumeGetResponse.VolumeSpaceAttributesPtr.SizePtr != nil {
			responseSize = strconv.FormatInt(int64(volumeGetResponse.VolumeSpaceAttributesPtr.Size()), 10)
		} else {
			return nil, VolumeSpaceAttributesReadError(fmt.Sprintf("volume %s size not available", responseName))
		}
	} else {
		return nil, VolumeSpaceAttributesReadError(fmt.Sprintf("error reading space attributes for volume %s",
			responseName))
	}

	if volumeGetResponse.VolumeExportAttributesPtr != nil {
		responseExportAttrs := volumeGetResponse.VolumeExportAttributes()
		if responseExportAttrs.PolicyPtr != nil {
			responseExportPolicy = responseExportAttrs.Policy()
		}
	}

	if volumeGetResponse.VolumeSecurityAttributesPtr != nil {
		responseSecurityAttrs := volumeGetResponse.VolumeSecurityAttributesPtr
		if responseSecurityAttrs.VolumeSecurityUnixAttributesPtr != nil {
			responseSecurityUnixAttrs := responseSecurityAttrs.VolumeSecurityUnixAttributes()
			if responseSecurityUnixAttrs.PermissionsPtr != nil {
				responseUnixPermissions = responseSecurityUnixAttrs.Permissions()
			}
		}
	}

	if volumeGetResponse.VolumeSnapshotAttributesPtr != nil {
		if volumeGetResponse.VolumeSnapshotAttributesPtr.SnapshotPolicyPtr != nil {
			responseSnapshotPolicy = volumeGetResponse.VolumeSnapshotAttributesPtr.SnapshotPolicy()
		}

		if volumeGetResponse.VolumeSnapshotAttributesPtr.SnapdirAccessEnabledPtr != nil {
			responseSnapdirAccessEnabled = volumeGetResponse.VolumeSnapshotAttributesPtr.SnapdirAccessEnabled()
		}
	}

	volumeInfo := &Volume{
		AccessType:      responseAccessType,
		Aggregates:      responseAggregates,
		Comment:         responseComment,
		ExportPolicy:    responseExportPolicy,
		JunctionPath:    responseJunctionPath,
		Name:            responseName,
		Size:            responseSize,
		SnapshotDir:     responseSnapdirAccessEnabled,
		SnapshotPolicy:  responseSnapshotPolicy,
		SnapshotReserve: responseSnapshotReserveInt,
		SpaceReserve:    responseSpaceReserve,
		UnixPermissions: responseUnixPermissions,
	}
	return volumeInfo, nil
}

func (d OntapAPIZAPI) APIVersion(ctx context.Context) (string, error) {
	return d.API.SystemGetOntapiVersion(ctx)
}

func (d OntapAPIZAPI) SupportsFeature(ctx context.Context, feature feature) bool {
	return d.API.SupportsFeature(ctx, feature)
}

func (d OntapAPIZAPI) NodeListSerialNumbers(ctx context.Context) ([]string, error) {
	return d.API.NodeListSerialNumbers(ctx)
}

type OntapAPIZAPI struct {
	API *Client
}

func NewOntapAPIZAPI(zapiClient *Client) (OntapAPIZAPI, error) {
	result := OntapAPIZAPI{
		API: zapiClient,
	}
	return result, nil
}

// NewZAPIClient is a factory method for creating a new instance
func NewZAPIClient(config ClientConfig) *Client {
	d := &Client{
		config: config,
		zr: &azgo.ZapiRunner{
			ManagementLIF:        config.ManagementLIF,
			SVM:                  config.SVM,
			Username:             config.Username,
			Password:             config.Password,
			ClientPrivateKey:     config.ClientPrivateKey,
			ClientCertificate:    config.ClientCertificate,
			TrustedCACertificate: config.TrustedCACertificate,
			Secure:               true,
			DebugTraceFlags:      config.DebugTraceFlags,
		},
		m: &sync.Mutex{},
	}
	return d
}

func NewZAPIClientFromOntapConfig(ctx context.Context, ontapConfig *drivers.OntapStorageDriverConfig,
	numRecords int) (OntapAPI, error) {

	client := NewZAPIClient(ClientConfig{
		ManagementLIF:           ontapConfig.ManagementLIF,
		SVM:                     ontapConfig.SVM,
		Username:                ontapConfig.Username,
		Password:                ontapConfig.Password,
		ClientCertificate:       ontapConfig.ClientCertificate,
		ClientPrivateKey:        ontapConfig.ClientPrivateKey,
		ContextBasedZapiRecords: numRecords,
		TrustedCACertificate:    ontapConfig.TrustedCACertificate,
		DebugTraceFlags:         ontapConfig.DebugTraceFlags,
	})
	if ontapConfig.SVM != "" {

		vserverResponse, err := client.VserverGetRequest()
		if err = GetError(ctx, vserverResponse, err); err != nil {
			return nil, fmt.Errorf("error reading SVM details: %v", err)
		}

		client.SVMUUID = vserverResponse.Result.AttributesPtr.VserverInfoPtr.Uuid()

		Logc(ctx).WithField("SVM", ontapConfig.SVM).Debug("Using specified SVM.")

	} else {

		// Use VserverGetIterRequest to populate config.SVM if it wasn't specified and we can derive it
		vserverResponse, err := client.VserverGetIterRequest()
		if err = GetError(ctx, vserverResponse, err); err != nil {
			return nil, fmt.Errorf("error enumerating SVMs: %v", err)
		}

		if vserverResponse.Result.NumRecords() != 1 {
			return nil, errors.New("cannot derive SVM to use; please specify SVM in config file")
		}

		// Update everything to use our derived SVM
		ontapConfig.SVM = vserverResponse.Result.AttributesListPtr.VserverInfoPtr[0].VserverName()
		svmUUID := vserverResponse.Result.AttributesListPtr.VserverInfoPtr[0].Uuid()

		client = NewZAPIClient(ClientConfig{
			ManagementLIF:        ontapConfig.ManagementLIF,
			SVM:                  ontapConfig.SVM,
			Username:             ontapConfig.Username,
			Password:             ontapConfig.Password,
			ClientCertificate:    ontapConfig.ClientCertificate,
			ClientPrivateKey:     ontapConfig.ClientPrivateKey,
			TrustedCACertificate: ontapConfig.TrustedCACertificate,
			DebugTraceFlags:      ontapConfig.DebugTraceFlags,
		})
		client.SVMUUID = svmUUID
	}

	apiZAPI, err := NewOntapAPIZAPI(client)
	if err != nil {
		return nil, fmt.Errorf("unable to get zapi client for ontap: %v", err)
	}

	return apiZAPI, nil
}

func (d OntapAPIZAPI) NetInterfaceGetDataLIFs(ctx context.Context, protocol string) ([]string, error) {
	return d.API.NetInterfaceGetDataLIFs(ctx, protocol)
}

func (d OntapAPIZAPI) GetSVMAggregateNames(_ context.Context) ([]string, error) {
	return d.API.SVMGetAggregateNames()
}

// EmsAutosupportLog generates an auto support message with the supplied parameters
func (d OntapAPIZAPI) EmsAutosupportLog(
	ctx context.Context,
	driverName string,
	appVersion string,
	autoSupport bool,
	category string,
	computerName string,
	eventDescription string,
	eventID int,
	eventSource string,
	logLevel int) {

	emsResponse, err := d.API.EmsAutosupportLog(appVersion, autoSupport, category, computerName, eventDescription, eventID,
		eventSource, logLevel)

	if err = GetError(ctx, emsResponse, err); err != nil {
		Logc(ctx).WithFields(log.Fields{
			"driver": driverName,
			"error":  err,
		}).Error("Error logging EMS message.")
	} else {
		Logc(ctx).WithField("driver", driverName).Debug("Logged EMS message.")
	}
}

func (d OntapAPIZAPI) FlexgroupExists(ctx context.Context, volumeName string) (bool, error) {
	volExists, err := d.API.FlexGroupExists(ctx, volumeName)
	if err != nil {
		err = fmt.Errorf("error checking for existing FlexGroup: %v", err)
	}
	return volExists, err
}

func (d OntapAPIZAPI) FlexgroupCreate(ctx context.Context, volume Volume) error {
	if d.API.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "FlexgroupCreate",
			"Type":   "OntapAPIZAPI",
			"spec":   volume,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> FlexgroupCreate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< FlexgroupCreate")
	}

	sizeBytes, err := strconv.ParseUint(volume.Size, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid volume size: %v", volume.Size, err)
	}

	flexgroupCreateResponse, err := d.API.FlexGroupCreate(ctx, volume.Name, int(sizeBytes), volume.Aggregates, volume.SpaceReserve,
		volume.SnapshotPolicy, volume.UnixPermissions, volume.ExportPolicy, volume.SecurityStyle, volume.TieringPolicy,
		volume.Comment, volume.Qos, volume.Encrypt, volume.SnapshotReserve)
	if err != nil {
		return fmt.Errorf("error creating volume: %v", err)
	}

	if flexgroupCreateResponse == nil {
		return fmt.Errorf("missing volume create response")
	}

	if err = GetError(ctx, flexgroupCreateResponse, err); err != nil {
		if zerr, ok := err.(ZapiError); ok {
			// Handle case where the Create is passed to every Docker Swarm node
			if zerr.Code() == azgo.EAPIERROR && strings.HasSuffix(strings.TrimSpace(zerr.Reason()), "Job exists") {
				Logc(ctx).WithField("volume", volume.Name).Warn("Volume create job already exists, " +
					"skipping volume create on this node.")
				err = VolumeCreateJobExistsError(fmt.Sprintf("volume create job already exists, %s",
					volume.Name))
			}
		}
	}

	return err
}

func (d OntapAPIZAPI) FlexgroupCloneSplitStart(ctx context.Context, cloneName string) error {
	return d.VolumeCloneSplitStart(ctx, cloneName)
}

func (d OntapAPIZAPI) FlexgroupDisableSnapshotDirectoryAccess(ctx context.Context, volumeName string) error {
	snapDirResponse, err := d.API.FlexGroupVolumeDisableSnapshotDirectoryAccess(ctx, volumeName)
	if err = GetError(ctx, snapDirResponse, err); err != nil {
		return fmt.Errorf("error disabling snapshot directory access: %v", err)
	}

	return nil
}

func (d OntapAPIZAPI) FlexgroupInfo(ctx context.Context, volumeName string) (*Volume, error) {
	if d.API.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "FlexgroupInfo",
			"Type":   "OntapAPIZAPI",
			"name":   volumeName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> FlexgroupInfo")
		defer Logc(ctx).WithFields(fields).Debug("<<<< FlexgroupInfo")
	}

	// Get Flexvol by name
	flexGroupGetResponse, err := d.API.FlexGroupGet(volumeName)

	Logc(ctx).WithFields(log.Fields{
		"flexGroupGetResponse": flexGroupGetResponse,
		"name":                 volumeName,
		"err":                  err,
	}).Debug("d.API.FlexGroupGet")

	if err != nil {
		Logc(ctx).Errorf("Could not find volume with name: %v; error: %v", volumeName, err.Error())
		return nil, VolumeReadError(fmt.Sprintf("could not find volume with name %s", volumeName))
	}

	flexgroupInfo, err := VolumeInfoFromZapiAttrsHelper(flexGroupGetResponse)
	if err != nil {
		return nil, err
	}

	return flexgroupInfo, nil
}

func (d OntapAPIZAPI) FlexgroupSetQosPolicyGroupName(ctx context.Context, name string, qos QosPolicyGroup) error {
	return d.VolumeSetQosPolicyGroupName(ctx, name, qos)
}

func (d OntapAPIZAPI) FlexgroupModifyExportPolicy(ctx context.Context, volumeName, policyName string) error {
	return d.VolumeModifyExportPolicy(ctx, volumeName, policyName)
}

func (d OntapAPIZAPI) FlexgroupSetComment(ctx context.Context, volumeNameInternal, volumeNameExternal, comment string) error {
	modifyCommentResponse, err := d.API.FlexGroupSetComment(ctx, volumeNameInternal, comment)
	if err = GetError(ctx, modifyCommentResponse, err); err != nil {
		Logc(ctx).WithField("originalName", volumeNameExternal).Errorf("Modifying comment failed: %v", err)
		return fmt.Errorf("volume %s modify failed: %v", volumeNameExternal, err)
	}

	return nil
}

func (d OntapAPIZAPI) FlexgroupSnapshotCreate(ctx context.Context, snapshotName, sourceVolume string) error {
	return d.VolumeSnapshotCreate(ctx, snapshotName, sourceVolume)
}

func (d OntapAPIZAPI) FlexgroupSnapshotList(ctx context.Context, sourceVolume string) (Snapshots, error) {
	return d.VolumeSnapshotList(ctx, sourceVolume)
}

func (d OntapAPIZAPI) FlexgroupModifyUnixPermissions(ctx context.Context, volumeNameInternal, volumeNameExternal,
	unixPermissions string) error {
	modifyUnixPermResponse, err := d.API.FlexGroupModifyUnixPermissions(ctx, volumeNameInternal, unixPermissions)
	if err = GetError(ctx, modifyUnixPermResponse, err); err != nil {
		Logc(ctx).WithField("originalName", volumeNameExternal).Errorf("Could not import volume, "+
			"modifying unix permissions failed: %v", err)
		return fmt.Errorf("volume %s modify failed: %v", volumeNameExternal, err)
	}

	return nil
}

func (d OntapAPIZAPI) FlexgroupMount(ctx context.Context, name, junctionPath string) error {
	// Mount the volume at the specified junction
	return d.VolumeMount(ctx, name, junctionPath)
}

func (d OntapAPIZAPI) FlexgroupDestroy(ctx context.Context, volumeName string, force bool) error {
	// TODO: If this is the parent of one or more clones, those clones have to split from this
	// volume before it can be deleted, which means separate copies of those volumes.
	// If there are a lot of clones on this volume, that could seriously balloon the amount of
	// utilized space. Is that what we want? Or should we just deny the delete, and force the
	// user to keep the volume around until all of the clones are gone? If we do that, need a
	// way to list the clones. Maybe volume inspect.
	if err := d.FlexgroupUnmount(ctx, volumeName, true); err != nil {
		return err
	}

	// This call is sync, but not idempotent, so we check if it is already offline
	offlineResp, offErr := d.API.VolumeOffline(volumeName)
	if offErr != nil {
		return fmt.Errorf("error taking Volume %v offline: %v", volumeName, offErr)
	}

	if zerr := NewZapiError(offlineResp); !zerr.IsPassed() {
		if zerr.Code() == azgo.EVOLUMEOFFLINE {
			Logc(ctx).WithField("volume", volumeName).Warn("Volume already offline.")
		} else if zerr.Code() == azgo.EVOLUMEDOESNOTEXIST {
			Logc(ctx).WithField("volume", volumeName).Debug("Volume already deleted, skipping destroy.")
			return nil
		} else {
			return fmt.Errorf("error taking Volume %v offline: %v", volumeName, zerr)
		}
	}

	volDestroyResponse, err := d.API.FlexGroupDestroy(ctx, volumeName, force)
	if err != nil {
		return fmt.Errorf("error destroying volume %v: %v", volumeName, err)
	}

	if zerr := NewZapiError(volDestroyResponse); !zerr.IsPassed() {

		// It's not an error if the volume no longer exists
		if zerr.Code() == azgo.EVOLUMEDOESNOTEXIST {
			Logc(ctx).WithField("volume", volumeName).Warn("Volume already deleted.")
		} else {
			return fmt.Errorf("error destroying volume %v: %v", volumeName, zerr)
		}
	}

	return nil
}

func (d OntapAPIZAPI) FlexgroupListByPrefix(ctx context.Context, prefix string) (Volumes, error) {

	// Get all volumes matching the storage prefix
	volumesResponse, err := d.API.FlexGroupGetAll(prefix)
	if err = GetError(ctx, volumesResponse, err); err != nil {
		return nil, err
	}

	volumes := Volumes{}

	// Convert all volumes to VolumeExternal and write them to the channel
	if volumesResponse.Result.AttributesListPtr != nil {
		for _, volume := range volumesResponse.Result.AttributesListPtr.VolumeAttributesPtr {
			volumeInfo, err := VolumeInfoFromZapiAttrsHelper(&volume)
			if err != nil {
				return nil, err
			}
			volumes = append(volumes, *volumeInfo)
		}
	}

	return volumes, nil
}

func (d OntapAPIZAPI) FlexgroupSetSize(ctx context.Context, name, newSize string) error {
	_, err := d.API.FlexGroupSetSize(ctx, name, newSize)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("FlexGroup resize failed.")
		return fmt.Errorf("flexgroup resize failed %v: %v", name, err)
	}
	return nil
}

func (d OntapAPIZAPI) FlexgroupSize(_ context.Context, volumeName string) (uint64, error) {
	size, err := d.API.FlexGroupSize(volumeName)
	return uint64(size), err
}

func (d OntapAPIZAPI) FlexgroupUsedSize(_ context.Context, volumeName string) (int, error) {
	return d.API.FlexGroupUsedSize(volumeName)
}

func (d OntapAPIZAPI) FlexgroupUnmount(ctx context.Context, name string, force bool) error {
	// This call is sync and idempotent
	umountResp, err := d.API.VolumeUnmount(name, force)
	if err != nil {
		return fmt.Errorf("error unmounting Volume %v: %v", name, err)
	}

	if zerr := NewZapiError(umountResp); !zerr.IsPassed() {
		if zerr.Code() == azgo.EOBJECTNOTFOUND {
			Logc(ctx).WithField("volume", name).Warn("Volume does not exist.")
			return nil
		} else {
			return fmt.Errorf("error unmounting Volume %v: %v", name, zerr)
		}
	}

	return nil
}

func (d OntapAPIZAPI) GetSVMAggregateAttributes(_ context.Context) (aggrList map[string]string, err error) {
	// Handle panics from the API layer
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("unable to inspect ONTAP backend: %v\nStack trace:\n%s", r, debug.Stack())
		}
	}()

	result, err := d.API.VserverShowAggrGetIterRequest()
	if err != nil {
		return
	}

	if zerr := NewZapiError(result.Result); !zerr.IsPassed() {
		err = zerr
		return
	}

	aggrList = make(map[string]string)

	if result.Result.AttributesListPtr != nil {
		for _, aggr := range result.Result.AttributesListPtr.ShowAggregatesPtr {
			aggrList[aggr.AggregateName()] = aggr.AggregateType()
		}
	}

	return aggrList, nil
}

func (d OntapAPIZAPI) ExportPolicyDestroy(ctx context.Context, policy string) error {
	response, err := d.API.ExportPolicyDestroy(policy)
	if err = GetError(ctx, response, err); err != nil {
		err = fmt.Errorf("error deleting export policy: %v", err)
	}
	return err
}

func (d OntapAPIZAPI) VolumeExists(ctx context.Context, volumeName string) (bool, error) {
	return d.API.VolumeExists(ctx, volumeName)
}

func (d OntapAPIZAPI) TieringPolicyValue(ctx context.Context) string {
	return d.API.TieringPolicyValue(ctx)
}

func (d OntapAPIZAPI) GetSVMAggregateSpace(ctx context.Context, aggregate string) ([]SVMAggregateSpace, error) {
	// lookup aggregate
	aggrSpaceResponse, aggrSpaceErr := d.API.AggrSpaceGetIterRequest(aggregate)
	if aggrSpaceErr != nil {
		return nil, aggrSpaceErr
	}

	var svmAggregateSpaceList []SVMAggregateSpace

	// iterate over results
	if aggrSpaceResponse.Result.AttributesListPtr != nil {

		for _, aggrSpace := range aggrSpaceResponse.Result.AttributesListPtr.SpaceInformationPtr {
			aggrName := aggrSpace.Aggregate()
			if aggregate != aggrName {
				Logc(ctx).Debugf("Skipping " + aggrName)
				continue
			}

			Logc(ctx).WithFields(log.Fields{
				"aggrName":                            aggrName,
				"size":                                aggrSpace.AggregateSize(),
				"volumeFootprints":                    aggrSpace.VolumeFootprints(),
				"volumeFootprintsPercent":             aggrSpace.VolumeFootprintsPercent(),
				"usedIncludingSnapshotReserve":        aggrSpace.UsedIncludingSnapshotReserve(),
				"usedIncludingSnapshotReservePercent": aggrSpace.UsedIncludingSnapshotReservePercent(),
			}).Info("Dumping aggregate space")

			svmAggregateSpace := SVMAggregateSpace{
				size:      int64(aggrSpace.AggregateSize()),
				used:      int64(aggrSpace.UsedIncludingSnapshotReserve()),
				footprint: int64(aggrSpace.VolumeFootprints()),
			}

			svmAggregateSpaceList = append(svmAggregateSpaceList, svmAggregateSpace)
		}
	}

	return svmAggregateSpaceList, nil
}

func (d OntapAPIZAPI) VolumeDisableSnapshotDirectoryAccess(ctx context.Context, name string) error {
	snapDirResponse, err := d.API.VolumeDisableSnapshotDirectoryAccess(name)
	if err = GetError(ctx, snapDirResponse, err); err != nil {
		return fmt.Errorf("error disabling snapshot directory access: %v", err)
	}

	return nil
}

func (d OntapAPIZAPI) VolumeMount(ctx context.Context, name, junctionPath string) error {

	mountResponse, err := d.API.VolumeMount(name, junctionPath)
	if err = GetError(ctx, mountResponse, err); err != nil {
		return fmt.Errorf("error mounting volume to junction: %v", err)
	}

	return nil
}

func (d OntapAPIZAPI) VolumeRename(ctx context.Context, originalName, newName string) error {
	renameResponse, err := d.API.VolumeRename(originalName, newName)
	if err = GetError(ctx, renameResponse, err); err != nil {
		Logc(ctx).WithField("originalName", originalName).Errorf("renaming volume failed: %v", err)
		return fmt.Errorf("volume %s rename failed: %v", originalName, err)
	}

	return nil
}

func (d OntapAPIZAPI) VolumeSetComment(ctx context.Context, volumeNameInternal, volumeNameExternal, comment string) error {
	modifyCommentResponse, err := d.API.VolumeSetComment(ctx, volumeNameInternal, comment)
	if err = GetError(ctx, modifyCommentResponse, err); err != nil {
		Logc(ctx).WithField("originalName", volumeNameExternal).Errorf("Modifying comment failed: %v", err)
		return fmt.Errorf("volume %s modify failed: %v", volumeNameExternal, err)
	}

	return nil
}

func (d OntapAPIZAPI) ExportPolicyCreate(ctx context.Context, policy string) error {
	policyCreateResponse, err := d.API.ExportPolicyCreate(policy)
	if err != nil {
		err = fmt.Errorf("error creating export policy %s: %v", policy, err)
	}
	if zerr := NewZapiError(policyCreateResponse); !zerr.IsPassed() {
		if zerr.Code() == azgo.EDUPLICATEENTRY {
			Logc(ctx).WithField("exportPolicy", policy).Debug("Export policy already exists.")
		} else {
			err = fmt.Errorf("error creating export policy %s: %v", policy, zerr)
		}
	}
	return err
}

func (d OntapAPIZAPI) VolumeSize(_ context.Context, volumeName string) (uint64, error) {
	size, err := d.API.VolumeSize(volumeName)
	return uint64(size), err
}

func (d OntapAPIZAPI) VolumeUsedSize(_ context.Context, volumeName string) (int, error) {
	return d.API.VolumeUsedSize(volumeName)
}

func (d OntapAPIZAPI) VolumeSetSize(ctx context.Context, name, newSize string) error {
	volumeSetSizeResponse, err := d.API.VolumeSetSize(name, newSize)

	if volumeSetSizeResponse == nil {
		if err != nil {
			err = fmt.Errorf("volume resize failed for volume %s: %v", name, err)
		} else {
			err = fmt.Errorf("volume resize failed for volume %s: unexpected error", name)
		}
		return err
	}

	if err = GetError(ctx, volumeSetSizeResponse.Result, err); err != nil {
		Logc(ctx).WithField("error", err).Error("Volume resize failed.")
		return fmt.Errorf("volume resize failed")
	}

	return nil
}

func (d OntapAPIZAPI) VolumeModifyUnixPermissions(ctx context.Context, volumeNameInternal, volumeNameExternal,
	unixPermissions string) error {
	modifyUnixPermResponse, err := d.API.VolumeModifyUnixPermissions(volumeNameInternal, unixPermissions)
	if err = GetError(ctx, modifyUnixPermResponse, err); err != nil {
		Logc(ctx).WithField("originalName", volumeNameExternal).Errorf("Could not import volume, "+
			"modifying unix permissions failed: %v", err)
		return fmt.Errorf("volume %s modify failed: %v", volumeNameExternal, err)
	}

	return nil
}

func (d OntapAPIZAPI) VolumeListByPrefix(ctx context.Context, prefix string) (Volumes, error) {
	// Get all volumes matching the storage prefix
	volumesResponse, err := d.API.VolumeGetAll(prefix)
	if err = GetError(ctx, volumesResponse, err); err != nil {
		return nil, err
	}

	volumes := Volumes{}

	// Convert all volumes to VolumeExternal and write them to the channel
	if volumesResponse.Result.AttributesListPtr != nil {
		for _, volume := range volumesResponse.Result.AttributesListPtr.VolumeAttributesPtr {
			volumeInfo, err := VolumeInfoFromZapiAttrsHelper(&volume)
			if err != nil {
				return nil, err
			}
			volumes = append(volumes, *volumeInfo)
		}
	}

	return volumes, nil
}

func (d OntapAPIZAPI) ExportRuleCreate(ctx context.Context, policyName, desiredPolicyRule string) error {
	ruleResponse, err := d.API.ExportRuleCreate(policyName, desiredPolicyRule,
		[]string{"nfs"}, []string{"any"}, []string{"any"}, []string{"any"})
	if err = GetError(ctx, ruleResponse, err); err != nil {
		err = fmt.Errorf("error creating export rule: %v", err)
		Logc(ctx).WithFields(log.Fields{
			"ExportPolicy": policyName,
			"ClientMatch":  desiredPolicyRule,
		}).Error(err)
	}

	if ruleResponse == nil {
		return fmt.Errorf("unexpected response")
	}

	return nil
}

func (d OntapAPIZAPI) ExportRuleDestroy(ctx context.Context, policyName string, ruleIndex int) error {
	ruleResponse, err := d.API.ExportRuleDestroy(policyName, ruleIndex)
	if err = GetError(ctx, ruleResponse, err); err != nil {
		err = fmt.Errorf("error deleting export rule on policy %s at index %d; %v",
			policyName, ruleIndex, err)
		Logc(ctx).WithFields(log.Fields{
			"ExportPolicy": policyName,
			"RuleIndex":    ruleIndex,
		}).Error(err)
	}

	if ruleResponse == nil {
		return fmt.Errorf("unexpected response")
	}

	return nil
}

func (d OntapAPIZAPI) VolumeModifyExportPolicy(ctx context.Context, volumeName, policyName string) error {
	volumeModifyResponse, err := d.API.VolumeModifyExportPolicy(volumeName, policyName)
	if err = GetError(ctx, volumeModifyResponse, err); err != nil {
		err = fmt.Errorf("error updating export policy on volume %s: %v", volumeName, err)
		Logc(ctx).Error(err)
		return err
	}

	if volumeModifyResponse == nil {
		return fmt.Errorf("unexpected response")
	}

	return nil
}

func (d OntapAPIZAPI) ExportPolicyExists(ctx context.Context, policyName string) (bool, error) {
	policyGetResponse, err := d.API.ExportPolicyGet(policyName)
	if err != nil {
		err = fmt.Errorf("error getting export policy; %v", err)
		Logc(ctx).WithField("exportPolicy", policyName).Error(err)
		return false, err
	}
	if zerr := NewZapiError(policyGetResponse); !zerr.IsPassed() {
		if zerr.Code() == azgo.EOBJECTNOTFOUND {
			Logc(ctx).WithField("exportPolicy", policyName).Debug("Export policy not found.")
			return false, nil
		} else {
			err = fmt.Errorf("error getting export policy; %v", zerr)
			Logc(ctx).WithField("exportPolicy", policyName).Error(err)
			return false, err
		}
	}
	return true, nil
}

func (d OntapAPIZAPI) ExportRuleList(ctx context.Context, policyName string) (map[string]int, error) {

	ruleListResponse, err := d.API.ExportRuleGetIterRequest(policyName)
	if err = GetError(ctx, ruleListResponse, err); err != nil {
		return nil, fmt.Errorf("error listing export policy rules: %v", err)
	}
	rules := make(map[string]int)

	if ruleListResponse.Result.NumRecords() > 0 {
		rulesAttrList := ruleListResponse.Result.AttributesList()
		exportRuleList := rulesAttrList.ExportRuleInfo()
		for _, rule := range exportRuleList {
			rules[rule.ClientMatch()] = rule.RuleIndex()
		}
	}

	return rules, nil
}

func (d OntapAPIZAPI) VolumeSnapshotCreate(ctx context.Context, snapshotName, sourceVolume string) error {
	snapResponse, err := d.API.SnapshotCreate(snapshotName, sourceVolume)
	if err = GetError(ctx, snapResponse, err); err != nil {
		return fmt.Errorf("error creating snapshot: %v", err)
	}
	return nil
}

// probeForVolume polls for the ONTAP volume to appear, with backoff retry logic
func (d OntapAPIZAPI) probeForVolume(ctx context.Context, name string) error {

	checkVolumeExists := func() error {
		volExists, err := d.VolumeExists(ctx, name)
		if err != nil {
			return err
		}
		if !volExists {
			return fmt.Errorf("volume %v does not yet exist", name)
		}
		return nil
	}
	volumeExistsNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Volume not yet present, waiting.")
	}
	volumeBackoff := backoff.NewExponentialBackOff()
	volumeBackoff.InitialInterval = 1 * time.Second
	volumeBackoff.Multiplier = 2
	volumeBackoff.RandomizationFactor = 0.1
	volumeBackoff.MaxElapsedTime = 30 * time.Second

	// Run the volume check using an exponential backoff
	if err := backoff.RetryNotify(checkVolumeExists, volumeBackoff, volumeExistsNotify); err != nil {
		Logc(ctx).WithField("volume", name).Warnf("Could not find volume after %3.2f seconds.", volumeBackoff.MaxElapsedTime.Seconds())
		return fmt.Errorf("volume %v does not exist", name)
	} else {
		Logc(ctx).WithField("volume", name).Debug("Volume found.")
		return nil
	}
}

func (d OntapAPIZAPI) VolumeCloneCreate(ctx context.Context, cloneName, sourceName, snapshot string, async bool) error {

	if async {
		cloneResponse, err := d.API.VolumeCloneCreateAsync(cloneName, sourceName, snapshot)
		err = d.API.WaitForAsyncResponse(ctx, cloneResponse, 120*time.Second)
		if err != nil {
			return errors.New("waiting for async response failed")
		}
	} else {
		cloneResponse, err := d.API.VolumeCloneCreate(cloneName, sourceName, snapshot)
		if err != nil {
			return fmt.Errorf("error creating clone: %v", err)
		}
		if zerr := NewZapiError(cloneResponse); !zerr.IsPassed() {

			if zerr.Code() == azgo.EOBJECTNOTFOUND {
				return fmt.Errorf("snapshot %s does not exist in volume %s", snapshot, sourceName)
			} else if zerr.IsFailedToLoadJobError() {
				fields := log.Fields{
					"zerr": zerr,
				}
				Logc(ctx).WithFields(fields).Warn(
					"Problem encountered during the clone create operation, " +
						"attempting to verify the clone was actually created")
				if volumeLookupError := d.probeForVolume(ctx, sourceName); volumeLookupError != nil {
					return volumeLookupError
				}
			} else {
				return fmt.Errorf("error creating clone: %v", zerr)
			}
		}
	}

	return nil
}

func (d OntapAPIZAPI) VolumeSnapshotList(ctx context.Context, sourceVolume string) (Snapshots, error) {
	snapListResponse, err := d.API.SnapshotList(sourceVolume)
	if err = GetError(ctx, snapListResponse, err); err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %v", err)
	}

	snapshots := Snapshots{}

	if snapListResponse.Result.AttributesListPtr != nil {
		for _, snap := range snapListResponse.Result.AttributesListPtr.SnapshotInfoPtr {
			snapshots = append(snapshots, Snapshot{
				CreateTime: time.Unix(int64(snap.AccessTime()), 0).UTC().Format(storage.SnapshotTimestampFormat),
				Name:       snap.Name(),
			})
		}
	}

	Logc(ctx).Debugf("Returned %v snapshots.", len(snapshots))

	return snapshots, nil
}

func (d OntapAPIZAPI) VolumeSetQosPolicyGroupName(ctx context.Context, name string, qos QosPolicyGroup) error {
	qosResponse, err := d.API.VolumeSetQosPolicyGroupName(name, qos)
	if err = GetError(ctx, qosResponse, err); err != nil {
		return fmt.Errorf("error setting quality of service policy: %v", err)
	}
	return nil
}

func (d OntapAPIZAPI) VolumeCloneSplitStart(ctx context.Context, cloneName string) error {
	splitResponse, err := d.API.VolumeCloneSplitStart(cloneName)
	if err = GetError(ctx, splitResponse, err); err != nil {
		return fmt.Errorf("error splitting clone: %v", err)
	}

	return nil
}

func (d OntapAPIZAPI) SnapshotRestoreVolume(ctx context.Context, snapshotName, sourceVolume string) error {

	snapResponse, err := d.API.SnapshotRestoreVolume(snapshotName, sourceVolume)

	if err = GetError(ctx, snapResponse, err); err != nil {
		return fmt.Errorf("error restoring snapshot: %v", err)
	}
	return nil
}

func (d OntapAPIZAPI) SnapshotRestoreFlexgroup(ctx context.Context, snapshotName, sourceVolume string) error {
	return d.SnapshotRestoreVolume(ctx, snapshotName, sourceVolume)
}

func (d OntapAPIZAPI) VolumeSnapshotDelete(_ context.Context, snapshotName, sourceVolume string) error {
	snapResponse, err := d.API.SnapshotDelete(snapshotName, sourceVolume)

	if err != nil {
		return fmt.Errorf("error deleting snapshot: %v", err)
	}
	if zerr := NewZapiError(snapResponse); !zerr.IsPassed() {
		if zerr.Code() == azgo.ESNAPSHOTBUSY {
			// Start a split here before returning the error so a subsequent delete attempt may succeed.
			return SnapshotBusyError(fmt.Sprintf("snapshot %s backing volume %s is busy", snapshotName,
				sourceVolume))
		}
		return fmt.Errorf("error deleting snapshot: %v", zerr)
	}
	return nil
}

func (d OntapAPIZAPI) FlexgroupSnapshotDelete(ctx context.Context, snapshotName, sourceVolume string) error {
	return d.VolumeSnapshotDelete(ctx, snapshotName, sourceVolume)
}

func (d OntapAPIZAPI) VolumeListBySnapshotParent(ctx context.Context, snapshotName, sourceVolume string) (
	VolumeNameList, error) {
	childVolumes, err := d.API.VolumeListAllBackedBySnapshot(ctx, sourceVolume, snapshotName)

	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"snapshotName":     snapshotName,
			"parentVolumeName": sourceVolume,
			"error":            err,
		}).Error("Could not list volumes backed by snapshot.")
		return nil, err
	} else if len(childVolumes) == 0 {
		return nil, nil
	}

	// We're going to start a single split operation, but there could be multiple children, so we
	// sort the volumes by name to not have more than one split operation running at a time.
	sort.Strings(childVolumes)

	return childVolumes, nil
}

func (d OntapAPIZAPI) SnapmirrorDeleteViaDestination(localFlexvolName, localSVMName string) error {
	snapDeleteResponse, err := d.API.SnapmirrorDeleteViaDestination(localFlexvolName, localSVMName)
	if err != nil && snapDeleteResponse != nil && snapDeleteResponse.Result.ResultErrnoAttr != azgo.EOBJECTNOTFOUND {
		return fmt.Errorf("error deleting snapmirror info for volume %v: %v", localFlexvolName, err)
	}

	// Ensure no leftover snapmirror metadata
	err = d.API.SnapmirrorRelease(localFlexvolName, localSVMName)
	if err != nil {
		return fmt.Errorf("error releasing snapmirror info for volume %v: %v", localFlexvolName, err)
	}

	return nil
}

func (d OntapAPIZAPI) IsSVMDRCapable(ctx context.Context) (bool, error) {
	return d.API.IsVserverDRCapable(ctx)
}
