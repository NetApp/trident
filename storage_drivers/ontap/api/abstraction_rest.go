// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"

	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/netapp/trident/utils"
)

// RestError encapsulates the status, reason, and errno values from a REST invocation, and it provides helper methods for detecting
// common error conditions.
type RestError struct {
	// 	"uuid": "2453aafc-a9a6-11eb-9fc7-080027c8f2a7",
	// 	"description": "DELETE /api/storage/volumes/60018ffd-a9a3-11eb-9fc7-080027c8f2a7/snapshots/6365e696-a9a3-11eb-9fc7-080027c8f2a7",
	// 	"state": "failure",
	// 	"message": "Snapshot copy \"snapshot-60f627c7-576b-42a5-863e-9ea174856f2f\" of volume \"rippy_pvc_e8f1cc49_7949_403c_9f83_786d1480af38\" on Vserver \"nfs_vs\" has not expired or is locked. Use the \"snapshot show -fields owners, expiry-time\" command to view the expiry and lock status of the Snapshot copy.",
	// 	"code": 1638555,
	// 	"start_time": "2021-04-30T07:21:00-04:00",
	// 	"end_time": "2021-04-30T07:21:10-04:00",
	uuid        string
	description string
	state       string
	message     string
	code        string
	start_time  string
	end_time    string
}

func NewRestErrorFromPayload(payload *models.Job) RestError {
	return RestError{
		uuid:        payload.UUID.String(),
		description: payload.Description,
		state:       payload.State,
		message:     payload.Message,
		code:        fmt.Sprint(payload.Code),
		start_time:  payload.StartTime.String(),
		end_time:    payload.EndTime.String(),
	}
}

func (e RestError) IsSuccess() bool {
	return e.state == models.JobStateSuccess
}

func (e RestError) IsFailure() bool {
	return e.state == models.JobStateFailure
}

func (e RestError) Error() string {
	if e.IsSuccess() {
		return "API status: success"
	}
	return fmt.Sprintf("API State: %s, Message: %s, Code: %s", e.state, e.message, e.code)
}

func (e RestError) IsSnapshotBusy() bool {
	return e.code == ESNAPSHOTBUSY_REST
}

func (e RestError) State() string {
	return e.state
}

func (e RestError) Message() string {
	return e.message
}

func (e RestError) Code() string {
	return e.code
}

type OntapAPIREST struct {
	api RestClientInterface
}

func NewOntapAPIREST(restClient *RestClient) (OntapAPIREST, error) {
	result := OntapAPIREST{
		api: restClient,
	}

	return result, nil
}

// NewOntapAPIRESTFromRestClientInterface added for testing
func NewOntapAPIRESTFromRestClientInterface(restClient RestClientInterface) (OntapAPIREST, error) {
	result := OntapAPIREST{
		api: restClient,
	}

	return result, nil
}

func (d OntapAPIREST) ValidateAPIVersion(ctx context.Context) error {

	// Make sure we're using a valid ONTAP version
	ontapVersion, err := d.APIVersion(ctx)
	if err != nil {
		return fmt.Errorf("could not determine Data ONTAP version: %v", err)
	}
	Logc(ctx).WithField("ontapVersion", ontapVersion).Debug("ONTAP version.")

	ontapSemVer, err := utils.ParseSemantic(ontapVersion)
	if err != nil {
		return err
	}
	if !ontapSemVer.AtLeast(MinimumONTAPVersion) {
		return fmt.Errorf("ONTAP %s or later is required, found %v", MinimumONTAPVersion.String(), ontapVersion)
	}
	return nil
}

func (d OntapAPIREST) VolumeCreate(ctx context.Context, volume Volume) error {

	if d.api.ClientConfig().DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "VolumeCreate",
			"Type":   "OntapAPIREST",
			"spec":   volume,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> VolumeCreate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< VolumeCreate")
	}

	creationErr := d.api.VolumeCreate(ctx, volume.Name, volume.Aggregates[0], volume.Size, volume.SpaceReserve,
		volume.SnapshotPolicy, volume.UnixPermissions, volume.ExportPolicy, volume.SecurityStyle,
		volume.TieringPolicy, volume.Comment, volume.Qos, volume.Encrypt, volume.SnapshotReserve)
	if creationErr != nil {
		return fmt.Errorf("error creating volume: %v", creationErr)
	}

	return nil
}

func (d OntapAPIREST) VolumeDestroy(ctx context.Context, name string, force bool) error {

	deletionErr := d.api.VolumeDestroy(ctx, name)
	if deletionErr != nil {
		return fmt.Errorf("error destroying volume %v: %v", name, deletionErr)
	}
	return nil
}

func (d OntapAPIREST) VolumeInfo(ctx context.Context, name string) (*Volume, error) {

	volumeGetResponse, err := d.api.VolumeGetByName(ctx, name)
	if err != nil {
		Logc(ctx).Errorf("Could not find volume with name: %v, error: %v", name, err.Error())
		return nil, err
	}

	if volumeGetResponse == nil {
		Logc(ctx).Errorf("Could not find volume with name: %v", name)
		return nil, VolumeReadError(fmt.Sprintf("could not find volume with name %s", name))
	}

	volumeInfo, err := VolumeInfoFromRestAttrsHelper(volumeGetResponse)
	if err != nil {
		return nil, err
	}

	return volumeInfo, nil
}

func VolumeInfoFromRestAttrsHelper(volumeGetResponse *models.Volume) (*Volume, error) {
	var responseAccessType string
	var responseAggregates []string
	var responseComment string
	var responseExportPolicy string
	var responseJunctionPath string
	var responseSize string
	// var responseSnapdirAccessEnabled bool
	var responseSnapshotPolicy string
	var responseSnapshotReserveInt int
	var responseSnapshotSpaceUsed int
	var responseSpaceReserve string
	var responseUnixPermissions string

	if volumeGetResponse.Type != nil {
		responseAccessType = *volumeGetResponse.Type
	}

	if volumeGetResponse.Aggregates != nil && len(volumeGetResponse.Aggregates) > 0 {
		responseAggregates = []string{
			volumeGetResponse.Aggregates[0].Name,
		}
	} else {
		return nil, VolumeIdAttributesReadError(fmt.Sprintf("error reading aggregates for volume %s",
			volumeGetResponse.Name))
	}

	if volumeGetResponse.Comment != nil {
		responseComment = *volumeGetResponse.Comment
	}

	if volumeGetResponse.Nas != nil {
		if volumeGetResponse.Nas.Path != nil {
			responseJunctionPath = *volumeGetResponse.Nas.Path
		}

		if volumeGetResponse.Nas.ExportPolicy != nil {
			responseExportPolicy = volumeGetResponse.Nas.ExportPolicy.Name
		}

		responseUnixPermissions = strconv.FormatInt(volumeGetResponse.Nas.UnixPermissions, 8)
	}

	responseSize = strconv.FormatInt(volumeGetResponse.Size, 10)

	if volumeGetResponse.Guarantee != nil {
		responseSpaceReserve = volumeGetResponse.Guarantee.Type
	}

	if volumeGetResponse.SnapshotPolicy != nil {
		responseSnapshotPolicy = volumeGetResponse.SnapshotPolicy.Name
	}

	if volumeGetResponse.Space != nil {
		if volumeGetResponse.Space.Snapshot != nil {
			if volumeGetResponse.Space.Snapshot.ReservePercent != nil {
				responseSnapshotReserveInt = int(*volumeGetResponse.Space.Snapshot.ReservePercent)
			}
			responseSnapshotSpaceUsed = int(volumeGetResponse.Space.Snapshot.Used)
		}
	}

	volumeInfo := &Volume{
		AccessType:   responseAccessType,
		Aggregates:   responseAggregates,
		Comment:      responseComment,
		ExportPolicy: responseExportPolicy,
		JunctionPath: responseJunctionPath,
		Name:         volumeGetResponse.Name,
		Size:         responseSize,
		// SnapshotDir:     false, // TODO fix this, figure it out for real
		SnapshotPolicy:    responseSnapshotPolicy,
		SnapshotReserve:   responseSnapshotReserveInt,
		SnapshotSpaceUsed: responseSnapshotSpaceUsed,
		SpaceReserve:      responseSpaceReserve,
		UnixPermissions:   responseUnixPermissions,
		UUID:              volumeGetResponse.UUID,
		DPVolume:          responseAccessType == "dp",
	}
	return volumeInfo, nil
}

func lunInfoFromRestAttrsHelper(lunGetResponse *models.Lun) (*Lun, error) {
	var responseComment string
	var responseLunMaps []LunMap
	var responseQos string
	var responseSize string
	var responseMapped bool
	var responseVolName string

	if lunGetResponse == nil {
		return nil, fmt.Errorf("lun response is nil")
	}
	if lunGetResponse.Comment != nil {
		responseComment = *lunGetResponse.Comment
	}

	var lunMap LunMap
	if lunGetResponse.LunMaps != nil {
		for _, record := range lunGetResponse.LunMaps {
			lunMap.IgroupName = record.Igroup.Name
			lunMap.LunID = int(record.LogicalUnitNumber)
			responseLunMaps = append(responseLunMaps, lunMap)
		}
	}

	if lunGetResponse.Space != nil {
		responseSize = strconv.FormatInt(lunGetResponse.Space.Size, 10)
	}

	if lunGetResponse.Comment != nil {
		responseComment = *lunGetResponse.Comment
	}

	if lunGetResponse.QosPolicy != nil {
		responseQos = lunGetResponse.QosPolicy.Name
	}

	if lunGetResponse.Status.Mapped != nil {
		responseMapped = *lunGetResponse.Status.Mapped
	}

	if lunGetResponse.Location != nil {
		if lunGetResponse.Location.Volume != nil {
			responseVolName = lunGetResponse.Location.Volume.Name
		}
	}

	lunInfo := &Lun{
		Comment:      responseComment,
		Enabled:      lunGetResponse.Enabled,
		LunMaps:      responseLunMaps,
		Name:         lunGetResponse.Name,
		Qos:          QosPolicyGroup{Name: responseQos},
		Size:         responseSize,
		Mapped:       responseMapped,
		UUID:         lunGetResponse.UUID,
		SerialNumber: lunGetResponse.SerialNumber,
		State:        lunGetResponse.Status.State,
		VolumeName:   responseVolName,
	}
	return lunInfo, nil
}

func (d OntapAPIREST) APIVersion(ctx context.Context) (string, error) {
	return d.api.SystemGetOntapVersion(ctx)
}

func (d OntapAPIREST) NodeListSerialNumbers(ctx context.Context) ([]string, error) {
	return d.api.NodeListSerialNumbers(ctx)
}

func (d OntapAPIREST) SupportsFeature(ctx context.Context, feature Feature) bool {
	return d.api.SupportsFeature(ctx, feature)
}

func (d OntapAPIREST) NetInterfaceGetDataLIFs(ctx context.Context, protocol string) ([]string, error) {
	return d.api.NetInterfaceGetDataLIFs(ctx, protocol)
}

func (d OntapAPIREST) GetSVMAggregateNames(ctx context.Context) ([]string, error) {
	return d.api.SVMGetAggregateNames(ctx)
}

func (d OntapAPIREST) EmsAutosupportLog(
	ctx context.Context,
	driverName string,
	appVersion string,
	autoSupport bool,
	category string,
	computerName string,
	eventDescription string,
	eventID int,
	eventSource string,
	logLevel int,
) {

	d.api.EmsAutosupportLog(ctx, appVersion, autoSupport, category, computerName, eventDescription, eventID,
		eventSource, logLevel)
}

func (d OntapAPIREST) FlexgroupExists(ctx context.Context, volumeName string) (bool, error) {
	return d.api.FlexGroupExists(ctx, volumeName)
}

func (d OntapAPIREST) FlexgroupCreate(ctx context.Context, volume Volume) error {
	if d.api.ClientConfig().DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "FlexgroupCreate",
			"Type":   "OntapAPIREST",
			"spec":   volume,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> FlexgroupCreate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< FlexgroupCreate")
	}

	volumeSize, _ := strconv.ParseUint(volume.Size, 10, 64)

	creationErr := d.api.FlexGroupCreate(ctx, volume.Name, int(volumeSize), volume.Aggregates, volume.SpaceReserve,
		volume.SnapshotPolicy, volume.UnixPermissions, volume.ExportPolicy, volume.SecurityStyle, volume.TieringPolicy,
		volume.Comment, volume.Qos, volume.Encrypt, volume.SnapshotReserve)
	if creationErr != nil {
		return fmt.Errorf("error creating volume: %v", creationErr)
	}

	return nil
}

func (d OntapAPIREST) FlexgroupCloneSplitStart(ctx context.Context, cloneName string) error {
	if err := d.api.FlexgroupCloneSplitStart(ctx, cloneName); err != nil {
		return fmt.Errorf("error splitting clone: %v", err)
	}
	return nil
}

func (d OntapAPIREST) FlexgroupDisableSnapshotDirectoryAccess(ctx context.Context, volumeName string) error {

	if err := d.api.FlexGroupVolumeDisableSnapshotDirectoryAccess(ctx, volumeName); err != nil {
		return fmt.Errorf("error disabling snapshot directory access: %v", err)
	}

	return nil
}

func (d OntapAPIREST) FlexgroupInfo(ctx context.Context, volumeName string) (*Volume, error) {

	volumeGetResponse, err := d.api.FlexGroupGetByName(ctx, volumeName)
	if err != nil {
		Logc(ctx).Errorf("Could not find volume with name: %v, error: %v", volumeName, err.Error())
		return nil, err
	}

	if volumeGetResponse == nil {
		Logc(ctx).Errorf("Could not find volume with name: %v", volumeName)
		return nil, VolumeReadError(fmt.Sprintf("could not find volume with name %s", volumeName))
	}

	volumeInfo, err := VolumeInfoFromRestAttrsHelper(volumeGetResponse)
	if err != nil {
		return nil, err
	}

	return volumeInfo, nil
}

func (d OntapAPIREST) FlexgroupSetComment(
	ctx context.Context, volumeNameInternal, volumeNameExternal, comment string,
) error {
	if err := d.api.FlexGroupSetComment(ctx, volumeNameInternal, comment); err != nil {
		Logc(ctx).WithField("originalName", volumeNameExternal).Errorf("Modifying comment failed: %v", err)
		return fmt.Errorf("volume %s modify failed: %v", volumeNameExternal, err)
	}
	return nil
}

func (d OntapAPIREST) FlexgroupSetQosPolicyGroupName(ctx context.Context, name string, qos QosPolicyGroup) error {
	if err := d.api.FlexgroupSetQosPolicyGroupName(ctx, name, qos); err != nil {
		return fmt.Errorf("error setting quality of service policy: %v", err)
	}

	return nil
}

func (d OntapAPIREST) FlexgroupSnapshotCreate(ctx context.Context, snapshotName, sourceVolume string) error {
	volume, err := d.FlexgroupInfo(ctx, sourceVolume)
	if err != nil {
		return fmt.Errorf("error looking up source volume %v: %v", sourceVolume, err)
	}
	if volume == nil {
		return fmt.Errorf("error looking up source volume: %v", sourceVolume)
	}

	if err = d.api.SnapshotCreateAndWait(ctx, volume.UUID, snapshotName); err != nil {
		return fmt.Errorf("could not create snapshot: %v", err)
	}
	return nil
}

func (d OntapAPIREST) FlexgroupSnapshotList(ctx context.Context, sourceVolume string) (Snapshots, error) {

	volume, err := d.FlexgroupInfo(ctx, sourceVolume)
	if err != nil {
		return nil, fmt.Errorf("error looking up source volume: %v", err)
	}
	if volume == nil {
		return nil, fmt.Errorf("error looking up source volume: %v", sourceVolume)
	}

	snapListResponse, err := d.api.SnapshotList(ctx, volume.UUID)
	if err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %v", err)
	}
	if snapListResponse == nil {
		return nil, fmt.Errorf("error enumerating snapshots")
	}
	snapshots := Snapshots{}

	for _, snap := range snapListResponse.Payload.Records {
		snapshots = append(snapshots, Snapshot{
			CreateTime: snap.CreateTime.String(), // TODO do we need to format this?
			Name:       snap.Name,
		})
	}

	Logc(ctx).Debugf("Returned %v snapshots.", snapListResponse.Payload.NumRecords)

	return snapshots, nil
}

func (d OntapAPIREST) FlexgroupModifyUnixPermissions(
	ctx context.Context, volumeNameInternal, volumeNameExternal, unixPermissions string,
) error {
	err := d.api.FlexGroupModifyUnixPermissions(ctx, volumeNameInternal, unixPermissions)
	if err != nil {
		Logc(ctx).WithField("originalName", volumeNameExternal).Errorf("Could not import volume, "+
			"modifying unix permissions failed: %v", err)
		return fmt.Errorf("volume %s modify failed: %v", volumeNameExternal, err)
	}

	return nil
}

func (d OntapAPIREST) FlexgroupMount(ctx context.Context, name, junctionPath string) error {
	// Mount the volume at the specified junction
	if err := d.api.FlexGroupMount(ctx, name, junctionPath); err != nil {
		return fmt.Errorf("error mounting volume %v to junction %v: %v", name, junctionPath, err)
	}

	return nil
}

func (d OntapAPIREST) FlexgroupDestroy(ctx context.Context, volumeName string, force bool) error {
	if err := d.FlexgroupUnmount(ctx, volumeName, true); err != nil {
		return fmt.Errorf("error unmounting volume %v: %v", volumeName, err)
	}

	// TODO: If this is the parent of one or more clones, those clones have to split from this
	// volume before it can be deleted, which means separate copies of those volumes.
	// If there are a lot of clones on this volume, that could seriously balloon the amount of
	// utilized space. Is that what we want? Or should we just deny the delete, and force the
	// user to keep the volume around until all of the clones are gone? If we do that, need a
	// way to list the clones. Maybe volume inspect.
	deletionErr := d.api.FlexGroupDestroy(ctx, volumeName)
	if deletionErr != nil {
		return fmt.Errorf("error destroying volume %v: %v", volumeName, deletionErr)
	}
	return nil
}

func (d OntapAPIREST) FlexgroupListByPrefix(ctx context.Context, prefix string) (Volumes, error) {

	// TODO handle this higher? or just leave this here? i think here is OK
	if !strings.HasSuffix(prefix, "*") {
		// append the "*" to our prefix if it's missing
		prefix += "*"
	}

	volumesResponse, err := d.api.FlexGroupGetAll(ctx, prefix)
	if err != nil {
		return nil, err
	}

	volumes := Volumes{}

	if volumesResponse.Payload.Records != nil {
		for _, volume := range volumesResponse.Payload.Records {
			volumeInfo, err := VolumeInfoFromRestAttrsHelper(volume)
			if err != nil {
				return nil, err
			}
			volumes = append(volumes, volumeInfo)
		}
	}

	return volumes, nil

}

func (d OntapAPIREST) FlexgroupSetSize(ctx context.Context, name, newSize string) error {
	if err := d.api.FlexGroupSetSize(ctx, name, newSize); err != nil {
		Logc(ctx).WithField("error", err).Error("Volume resize failed.")
		return fmt.Errorf("volume resize failed")
	}

	return nil
}

func (d OntapAPIREST) FlexgroupSize(ctx context.Context, volumeName string) (uint64, error) {
	return d.api.FlexGroupSize(ctx, volumeName)
}

func (d OntapAPIREST) FlexgroupUsedSize(ctx context.Context, volumeName string) (int, error) {
	return d.api.FlexGroupUsedSize(ctx, volumeName)
}

func (d OntapAPIREST) FlexgroupModifyExportPolicy(ctx context.Context, volumeName, policyName string) error {
	err := d.api.FlexgroupModifyExportPolicy(ctx, volumeName, policyName)
	if err != nil {
		err = fmt.Errorf("error updating export policy on volume %s: %v", volumeName, err)
		Logc(ctx).Error(err)
		return err
	}

	return nil
}

func (d OntapAPIREST) FlexgroupUnmount(ctx context.Context, name string, _ bool) error {
	// Setting an empty path should deactivate and unmount the volume
	if err := d.api.FlexgroupUnmount(ctx, name); err != nil {
		return fmt.Errorf("error unmounting volume %v: %v", name, err)
	}

	return nil
}

func (d OntapAPIREST) GetSVMAggregateAttributes(ctx context.Context) (aggrList map[string]string, err error) {

	// Handle panics from the API layer
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("unable to inspect ONTAP backend: %v\nStack trace:\n%s", r, debug.Stack())
		}
	}()

	result, err := d.api.AggregateList(ctx, "*")
	if result == nil || result.Payload.NumRecords == 0 || result.Payload.Records == nil {
		return nil, fmt.Errorf("could not retrieve aggregate information")
	}

	aggrList = make(map[string]string)

	for _, aggr := range result.Payload.Records {
		aggrList[aggr.Name] = aggr.BlockStorage.Primary.DiskType // TODO validate this is right
	}

	return aggrList, nil
}

func (d OntapAPIREST) ExportPolicyDestroy(ctx context.Context, policy string) error {
	exportPolicyDestroyResult, err := d.api.ExportPolicyDestroy(ctx, policy)
	if err != nil {
		return fmt.Errorf("error deleting export policy: %v", err)
	}
	if exportPolicyDestroyResult == nil {
		return fmt.Errorf("error deleting export policy")
	}

	return err
}

func (d OntapAPIREST) VolumeExists(ctx context.Context, volumeName string) (bool, error) {
	return d.api.VolumeExists(ctx, volumeName)
}

func (d OntapAPIREST) TieringPolicyValue(ctx context.Context) string {
	return d.api.TieringPolicyValue(ctx)
}

func hasRestAggrSpaceInformation(ctx context.Context, aggrSpace *models.AggregateSpace) bool {
	if aggrSpace == nil {
		return false
	}
	if aggrSpace.BlockStorage == nil {
		return false
	}
	return true
}

func (d OntapAPIREST) GetSVMAggregateSpace(ctx context.Context, aggregate string) ([]SVMAggregateSpace, error) {

	response, aggrSpaceErr := d.api.AggregateList(ctx, aggregate)
	if aggrSpaceErr != nil {
		return nil, aggrSpaceErr
	}
	if response == nil {
		return nil, fmt.Errorf("error looking up aggregate: %v", aggregate)
	}

	if response.Payload == nil {
		return nil, fmt.Errorf("error looking up aggregate: %v", aggregate)
	}

	var svmAggregateSpaceList []SVMAggregateSpace

	for _, aggr := range response.Payload.Records {

		if aggr == nil {
			Logc(ctx).Debugf("Skipping empty record")
			continue
		}

		aggrName := aggr.Name
		if aggregate != aggrName {
			Logc(ctx).Debugf("Skipping " + aggrName)
			continue
		}

		aggrSpace := aggr.Space
		if !hasRestAggrSpaceInformation(ctx, aggrSpace) {
			Logc(ctx).Debugf("Skipping entry with missing aggregate space information")
			continue
		}

		Logc(ctx).WithFields(log.Fields{
			"aggrName": aggrName,
			"size":     aggrSpace.BlockStorage.Size,
		}).Info("Dumping aggregate space")

		svmAggregateSpace := SVMAggregateSpace{
			size:      aggrSpace.BlockStorage.Size,
			used:      aggrSpace.BlockStorage.Used,
			footprint: aggrSpace.Footprint,
		}

		svmAggregateSpaceList = append(svmAggregateSpaceList, svmAggregateSpace)

	}

	return svmAggregateSpaceList, nil
}

func (d OntapAPIREST) VolumeDisableSnapshotDirectoryAccess(ctx context.Context, name string) error {

	if err := d.api.VolumeDisableSnapshotDirectoryAccess(ctx, name); err != nil {
		return fmt.Errorf("error disabling snapshot directory access: %v", err)
	}

	return nil
}

func (d OntapAPIREST) VolumeMount(ctx context.Context, name, junctionPath string) error {
	// Mount the volume at the specified junction
	if err := d.api.VolumeMount(ctx, name, junctionPath); err != nil {
		return fmt.Errorf("error mounting volume %v to junction %v: %v", name, junctionPath, err)
	}

	return nil
}

func (d OntapAPIREST) VolumeRename(ctx context.Context, originalName, newName string) error {
	return d.api.VolumeRename(ctx, originalName, newName)
}

func (d OntapAPIREST) VolumeSetComment(
	ctx context.Context, volumeNameInternal, volumeNameExternal, comment string,
) error {
	if err := d.api.VolumeSetComment(ctx, volumeNameInternal, comment); err != nil {
		Logc(ctx).WithField("originalName", volumeNameExternal).Errorf("Modifying comment failed: %v", err)
		return fmt.Errorf("volume %s modify failed: %v", volumeNameExternal, err)
	}
	return nil
}

func (d OntapAPIREST) ExportPolicyCreate(ctx context.Context, policy string) error {

	// TODO use isExportPolicyExistsRest ?
	exportPolicy, err := d.api.ExportPolicyGetByName(ctx, policy)
	if err != nil {
		Logc(ctx).Errorf("error checking for existing export policy %s: %v", policy, err)
		return err
	}
	if exportPolicy != nil {
		// specified export policy already exists
		return nil
	}

	// could not find the specified export policy, create it
	policyCreateResponse, err := d.api.ExportPolicyCreate(ctx, policy)
	if err != nil {
		err = fmt.Errorf("error creating export policy %s: %v", policy, err)
	} else if policyCreateResponse == nil {
		err = fmt.Errorf("error creating export policy %s", policy)
	}

	return err
}

func (d OntapAPIREST) VolumeSize(ctx context.Context, volumeName string) (uint64, error) {
	return d.api.VolumeSize(ctx, volumeName)
}

func (d OntapAPIREST) VolumeUsedSize(ctx context.Context, volumeName string) (int, error) {
	return d.api.VolumeUsedSize(ctx, volumeName)
}

func (d OntapAPIREST) VolumeSetSize(ctx context.Context, name, newSize string) error {
	if err := d.api.VolumeSetSize(ctx, name, newSize); err != nil {
		Logc(ctx).WithField("error", err).Error("Volume resize failed.")
		return fmt.Errorf("volume resize failed")
	}

	return nil
}

func (d OntapAPIREST) VolumeModifyUnixPermissions(
	ctx context.Context, volumeNameInternal, volumeNameExternal, unixPermissions string,
) error {
	err := d.api.VolumeModifyUnixPermissions(ctx, volumeNameInternal, unixPermissions)
	if err != nil {
		Logc(ctx).WithField("originalName", volumeNameExternal).Errorf(
			"Could not import volume, modifying unix permissions failed: %v", err)
		return fmt.Errorf("volume %s modify failed: %v", volumeNameExternal, err)
	}

	return nil
}

func (d OntapAPIREST) VolumeListByPrefix(ctx context.Context, prefix string) (Volumes, error) {

	// TODO handle this higher? or just leave this here? i think here is OK
	if !strings.HasSuffix(prefix, "*") {
		// append the "*" to our prefix if it's missing
		prefix += "*"
	}

	volumesResponse, err := d.api.VolumeList(ctx, prefix)
	if err != nil {
		return nil, err
	}

	volumes := Volumes{}

	if volumesResponse.Payload.Records != nil {
		for _, volume := range volumesResponse.Payload.Records {
			volumeInfo, err := VolumeInfoFromRestAttrsHelper(volume)
			if err != nil {
				return nil, err
			}
			volumes = append(volumes, volumeInfo)
		}
	}

	return volumes, nil
}

func (d OntapAPIREST) VolumeListByAttrs(ctx context.Context, volumeAttrs *Volume) (Volumes, error) {
	return d.api.VolumeListByAttrs(ctx, volumeAttrs)
}

func (d OntapAPIREST) ExportRuleCreate(ctx context.Context, policyName, desiredPolicyRules string) error {
	if d.api.ClientConfig().DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":             "ExportRuleCreate",
			"Type":               "OntapAPIREST",
			"policyName":         policyName,
			"desiredPolicyRules": desiredPolicyRules,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> ExportRuleCreate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< ExportRuleCreate")
	}

	// unlike the ZAPI version of this function, we must create them 1 at a time here in REST
	for _, desiredPolicyRule := range strings.Split(desiredPolicyRules, ",") {
		Logc(ctx).Debugf("processing desiredPolicyRule: '%v'", desiredPolicyRule)
		ruleResponse, err := d.api.ExportRuleCreate(ctx, policyName, desiredPolicyRule,
			[]string{"nfs"}, []string{"any"}, []string{"any"}, []string{"any"})
		if err != nil {
			err = fmt.Errorf("error creating export rule: %v", err)
			Logc(ctx).WithFields(log.Fields{
				"ExportPolicy": policyName,
				"ClientMatch":  desiredPolicyRule,
			}).Error(err)
			return err
		}
		if ruleResponse == nil {
			return fmt.Errorf("unexpected response")
		}
	}

	return nil
}

func (d OntapAPIREST) ExportRuleDestroy(ctx context.Context, policyName string, ruleIndex int) error {
	ruleDestroyResponse, err := d.api.ExportRuleDestroy(ctx, policyName, ruleIndex)
	if err != nil {
		err = fmt.Errorf("error deleting export rule on policy %s at index %d; %v", policyName, ruleIndex, err)
		Logc(ctx).WithFields(log.Fields{
			"ExportPolicy": policyName,
			"RuleIndex":    ruleIndex,
		}).Error(err)
	}

	if ruleDestroyResponse == nil {
		return fmt.Errorf("unexpected response")
	}
	return nil
}

func (d OntapAPIREST) VolumeModifyExportPolicy(ctx context.Context, volumeName, policyName string) error {
	err := d.api.VolumeModifyExportPolicy(ctx, volumeName, policyName)
	if err != nil {
		err = fmt.Errorf("error updating export policy on volume %s: %v", volumeName, err)
		Logc(ctx).Error(err)
		return err
	}

	return nil
}

func (d OntapAPIREST) ExportPolicyExists(ctx context.Context, policyName string) (bool, error) {
	policyGetResponse, err := d.api.ExportPolicyGetByName(ctx, policyName)
	if err != nil {
		err = fmt.Errorf("error getting export policy; %v", err)
		Logc(ctx).WithField("exportPolicy", policyName).Error(err)
		return false, err
	}
	if policyGetResponse == nil {
		return false, nil
	}
	return true, nil
}

func (d OntapAPIREST) ExportRuleList(ctx context.Context, policyName string) (map[string]int, error) {
	ruleListResponse, err := d.api.ExportRuleList(ctx, policyName)
	if err != nil {
		return nil, fmt.Errorf("error listing export policy rules: %v", err)
	}

	rules := make(map[string]int)
	if ruleListResponse != nil && ruleListResponse.Payload.NumRecords > 0 {
		exportRuleList := ruleListResponse.Payload.Records
		for _, rule := range exportRuleList {
			for _, client := range rule.Clients {
				rules[client.Match] = int(rule.Index)
			}
		}
	}

	return rules, nil
}

func (d OntapAPIREST) QtreeExists(ctx context.Context, name, volumePrefix string) (bool, string, error) {
	return d.api.QtreeExists(ctx, name, volumePrefix)
}

func (d OntapAPIREST) QtreeCreate(
	ctx context.Context, name, volumeName, unixPermissions, exportPolicy, securityStyle, qosPolicy string,
) error {
	return d.api.QtreeCreate(ctx, name, volumeName, unixPermissions, exportPolicy, securityStyle, qosPolicy)
}

func (d OntapAPIREST) QtreeDestroyAsync(ctx context.Context, path string, force bool) error {
	// REST interface doesn't use the /vol prefix in the paths
	path = strings.TrimPrefix(path, "/vol")
	return d.api.QtreeDestroyAsync(ctx, path, force)
}

func (d OntapAPIREST) QtreeRename(ctx context.Context, path, newPath string) error {
	// REST interface doesn't use the /vol prefix in the paths
	path = strings.TrimPrefix(path, "/vol")
	newPath = strings.TrimPrefix(newPath, "/vol")
	return d.api.QtreeRename(ctx, path, newPath)
}

func (d OntapAPIREST) QtreeModifyExportPolicy(ctx context.Context, name, volumeName, newExportPolicyName string) error {
	return d.api.QtreeModifyExportPolicy(ctx, name, volumeName, newExportPolicyName)
}

func (d OntapAPIREST) QtreeCount(ctx context.Context, volumeName string) (int, error) {
	return d.api.QtreeCount(ctx, volumeName)
}

func (d OntapAPIREST) QtreeListByPrefix(ctx context.Context, prefix, volumePrefix string) (Qtrees, error) {
	qtreeList, err := d.api.QtreeList(ctx, prefix, volumePrefix)
	if err != nil {
		msg := fmt.Sprintf("Error listing qtrees. %v", err)
		Logc(ctx).Errorf(msg)
		return nil, fmt.Errorf(msg)
	}
	qtrees := Qtrees{}
	for _, qtree := range qtreeList.GetPayload().Records {
		newQtree := d.convertQtree(qtree)
		qtrees = append(qtrees, newQtree)
	}

	return qtrees, nil
}

func (d OntapAPIREST) convertQtree(qtree *models.Qtree) *Qtree {
	newQtree := &Qtree{
		Name:            qtree.Name,
		SecurityStyle:   string(qtree.SecurityStyle),
		UnixPermissions: strconv.FormatInt(qtree.UnixPermissions, 10),
	}
	if qtree.ExportPolicy != nil {
		newQtree.ExportPolicy = qtree.ExportPolicy.Name
	}
	if qtree.Volume != nil {
		newQtree.Volume = qtree.Volume.Name
	}
	if qtree.Svm != nil {
		newQtree.Vserver = qtree.Svm.Name
	}
	return newQtree
}

func (d OntapAPIREST) QtreeGetByName(ctx context.Context, name, volumePrefix string) (*Qtree, error) {
	qtree, err := d.api.QtreeGet(ctx, name, volumePrefix)
	if err != nil {
		msg := "error getting qtree"
		Logc(ctx).WithError(err).Errorf(msg)
		return nil, fmt.Errorf(msg)
	}
	return d.convertQtree(qtree), nil
}

func (d OntapAPIREST) QuotaEntryList(ctx context.Context, volumeName string) (QuotaEntries, error) {
	response, err := d.api.QuotaEntryList(ctx, volumeName)
	if err != nil {
		return nil, err
	}
	entries := QuotaEntries{}
	for _, entry := range response.Payload.Records {
		entries = append(entries, d.convertQuota(entry))
	}
	return entries, nil
}

func (d OntapAPIREST) QuotaOff(ctx context.Context, volumeName string) error {
	err := d.api.QuotaOff(ctx, volumeName)
	if err != nil {
		msg := "error disabling quota"
		Logc(ctx).WithError(err).WithField("volume", volumeName).Error(msg)
		return err
	}
	return nil
}

func (d OntapAPIREST) QuotaOn(ctx context.Context, volumeName string) error {
	err := d.api.QuotaOn(ctx, volumeName)
	if err != nil {
		msg := "error enabling quota"
		Logc(ctx).WithError(err).WithField("volume", volumeName).Error(msg)
		return err
	}
	return nil
}

func (d OntapAPIREST) QuotaResize(context.Context, string) error {
	// With REST Changes to quota rule limits ("space.hard_limit", "space.soft_limit", "files.hard_limit",
	// and "files.soft_limit") are applied automatically without requiring a quota resize operation.
	return nil
}

func (d OntapAPIREST) QuotaStatus(ctx context.Context, volumeName string) (string, error) {
	volume, err := d.api.VolumeGetByName(ctx, volumeName)
	if err != nil {
		return "", fmt.Errorf("error getting quota status for Flexvol %s: %v", volumeName, err)
	}

	return volume.Quota.State, nil
}

func (d OntapAPIREST) QuotaSetEntry(ctx context.Context, qtreeName, volumeName, quotaType, diskLimit string) error {
	if diskLimit == "" {
		diskLimit = "-1"
	} else {
		hardLimit, parseErr := strconv.ParseInt(diskLimit, 10, 64)
		if parseErr != nil {
			return fmt.Errorf("cannot process disk limit value %v", diskLimit)
		}
		hardLimit *= 1024 // REST takes the limit in bytes, not KB
		diskLimit = strconv.FormatInt(hardLimit, 10)
	}
	return d.api.QuotaSetEntry(ctx, qtreeName, volumeName, quotaType, diskLimit)
}

func (d OntapAPIREST) QuotaGetEntry(ctx context.Context, volumeName, qtreeName, quotaType string) (*QuotaEntry, error) {
	quota, err := d.api.QuotaGetEntry(ctx, volumeName, qtreeName, quotaType)
	if err != nil {
		Logc(ctx).WithError(err).Error("error getting quota rule")
	}
	quotaEntry := d.convertQuota(quota)
	return quotaEntry, nil
}

func (d OntapAPIREST) convertQuota(quota *models.QuotaRule) *QuotaEntry {
	diskLimit := int64(-1)
	if quota.Space != nil {
		diskLimit = quota.Space.HardLimit
	}
	quotaEntry := &QuotaEntry{
		DiskLimitBytes: diskLimit,
		Target:         fmt.Sprintf("/vol/%s", quota.Volume.Name),
	}
	if quota.Qtree != nil && quota.Qtree.Name != "" {
		quotaEntry.Target += fmt.Sprintf("/%s", quota.Qtree.Name)
	}
	return quotaEntry
}

func (d OntapAPIREST) VolumeSnapshotCreate(ctx context.Context, snapshotName, sourceVolume string) error {

	volume, err := d.VolumeInfo(ctx, sourceVolume)
	if err != nil {
		return fmt.Errorf("error looking up source volume %v: %v", sourceVolume, err)
	}
	if volume == nil {
		return fmt.Errorf("error looking up source volume: %v", sourceVolume)
	}

	if err = d.api.SnapshotCreateAndWait(ctx, volume.UUID, snapshotName); err != nil {
		return fmt.Errorf("could not create snapshot: %v", err)
	}
	return nil
}

// pollVolumeExistence polls for the volume, with backoff retry logic
func (c OntapAPIREST) pollVolumeExistence(ctx context.Context, volumeName string) error {

	checkVolumeStatus := func() error {
		volume, err := c.VolumeInfo(ctx, volumeName)
		if err != nil {
			return err
		}
		if volume == nil {
			return fmt.Errorf("could not find Volume with name %v", volumeName)
		}
		return nil
	}
	volumeStatusNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Volume not found, waiting.")
	}
	volumeStatusBackoff := backoff.NewExponentialBackOff()
	volumeStatusBackoff.InitialInterval = 1 * time.Second
	volumeStatusBackoff.Multiplier = 2
	volumeStatusBackoff.RandomizationFactor = 0.1
	volumeStatusBackoff.MaxElapsedTime = 2 * time.Minute

	// Run the check using an exponential backoff
	if err := backoff.RetryNotify(checkVolumeStatus, volumeStatusBackoff, volumeStatusNotify); err != nil {
		Logc(ctx).WithField("Volume", volumeName).Warnf("Volume not found after %3.2f seconds.",
			volumeStatusBackoff.MaxElapsedTime.Seconds())
		return err
	}

	return nil
}

func (d OntapAPIREST) VolumeCloneCreate(ctx context.Context, cloneName, sourceName, snapshot string, async bool) error {
	err := d.api.VolumeCloneCreateAsync(ctx, cloneName, sourceName, snapshot)
	if err != nil {
		return fmt.Errorf("error creating clone: %v", err)
	}

	return d.pollVolumeExistence(ctx, cloneName)
}

func (d OntapAPIREST) VolumeSnapshotList(ctx context.Context, sourceVolume string) (Snapshots, error) {

	volume, err := d.VolumeInfo(ctx, sourceVolume)
	if err != nil {
		return nil, fmt.Errorf("error looking up source volume: %v", err)
	}
	if volume == nil {
		return nil, fmt.Errorf("error looking up source volume: %v", sourceVolume)
	}

	snapListResponse, err := d.api.SnapshotList(ctx, volume.UUID)
	if err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %v", err)
	}
	if snapListResponse == nil {
		return nil, fmt.Errorf("error enumerating snapshots")
	}
	snapshots := Snapshots{}

	for _, snap := range snapListResponse.Payload.Records {
		snapshots = append(snapshots, Snapshot{
			CreateTime: snap.CreateTime.String(), // TODO do we need to format this?
			Name:       snap.Name,
		})
	}

	Logc(ctx).Debugf("Returned %v snapshots.", snapListResponse.Payload.NumRecords)

	return snapshots, nil
}

func (d OntapAPIREST) VolumeSetQosPolicyGroupName(ctx context.Context, name string, qos QosPolicyGroup) error {

	if err := d.api.VolumeSetQosPolicyGroupName(ctx, name, qos); err != nil {
		return fmt.Errorf("error setting quality of service policy: %v", err)
	}

	return nil
}

func (d OntapAPIREST) VolumeCloneSplitStart(ctx context.Context, cloneName string) error {
	if err := d.api.VolumeCloneSplitStart(ctx, cloneName); err != nil {
		return fmt.Errorf("error splitting clone: %v", err)
	}
	return nil
}

func (d OntapAPIREST) SnapshotRestoreVolume(
	ctx context.Context, snapshotName, sourceVolume string,
) error {
	if err := d.api.SnapshotRestoreVolume(ctx, snapshotName, sourceVolume); err != nil {
		return fmt.Errorf("error restoring snapshot: %v", err)
	}

	return nil
}

func (d OntapAPIREST) SnapshotRestoreFlexgroup(ctx context.Context, snapshotName, sourceVolume string) error {
	if err := d.api.SnapshotRestoreFlexgroup(ctx, snapshotName, sourceVolume); err != nil {
		return fmt.Errorf("error restoring snapshot: %v", err)
	}

	return nil
}

func (d OntapAPIREST) SnapshotDeleteByNameAndStyle(
	ctx context.Context, snapshotName, sourceVolume, sourceVolumeUUID string,
) error {
	// GET the snapshot by name
	snapshot, err := d.api.SnapshotGetByName(ctx, sourceVolumeUUID, snapshotName)
	if err != nil {
		return fmt.Errorf("error checking for snapshot: %v", err)
	}
	if snapshot == nil {
		return fmt.Errorf("error looking up snapshot: %v", snapshotName)
	}
	snapshotUUID := snapshot.UUID

	// DELETE the snapshot
	snapshotDeleteResult, err := d.api.SnapshotDelete(ctx, sourceVolumeUUID, snapshotUUID)
	if err != nil {
		return fmt.Errorf("error while deleting snapshot: %v", err)
	}
	if snapshotDeleteResult == nil {
		return fmt.Errorf("error while deleting snapshot: %v", snapshotName)
	}

	// check snapshot delete job status
	snapshotDeleteErr := d.api.PollJobStatus(ctx, snapshotDeleteResult.Payload)
	// if err := client.PollJobStatus(ctx, snapshotDeleteResult.Payload); err != nil {
	if snapshotDeleteErr != nil {
		Logc(ctx).Debugf("Could not delete the snapshot, going to check if it's busy; error was: %v", snapshotDeleteErr)
		if restErr, ok := snapshotDeleteErr.(RestError); ok {
			Logc(ctx).Debugf("restErr: %v", restErr)
			Logc(ctx).Debugf("restErr.Code(): %v", restErr.Code())
			if restErr.IsSnapshotBusy() {
				Logc(ctx).Debug("Snapshot was busy, going to split it")
				// Start a split here before returning the error so a subsequent delete attempt may succeed.
				return SnapshotBusyError(fmt.Sprintf("snapshot %s backing volume %s is busy", snapshotName,
					sourceVolume))
			}
		}
		return snapshotDeleteErr
	}

	return nil
}

func (d OntapAPIREST) FlexgroupSnapshotDelete(ctx context.Context, snapshotName, sourceVolume string) error {
	volume, err := d.FlexgroupInfo(ctx, sourceVolume)
	if err != nil {
		return fmt.Errorf("error looking up source volume: %v", err)
	}
	if volume == nil {
		return fmt.Errorf("error looking up source volume: %v", sourceVolume)
	}
	volumeUUID := volume.UUID

	return d.SnapshotDeleteByNameAndStyle(ctx, snapshotName, sourceVolume, volumeUUID)
}

func (d OntapAPIREST) VolumeSnapshotDelete(ctx context.Context, snapshotName, sourceVolume string) error {
	volume, err := d.VolumeInfo(ctx, sourceVolume)
	if err != nil {
		return fmt.Errorf("error looking up source volume: %v", err)
	}
	if volume == nil {
		return fmt.Errorf("error looking up source volume: %v", sourceVolume)
	}
	volumeUUID := volume.UUID

	return d.SnapshotDeleteByNameAndStyle(ctx, snapshotName, sourceVolume, volumeUUID)
}

func (d OntapAPIREST) VolumeListBySnapshotParent(
	ctx context.Context, snapshotName, sourceVolume string,
) (VolumeNameList, error) {

	childVolumes, err := d.api.VolumeListAllBackedBySnapshot(ctx, sourceVolume, snapshotName)
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

func (d OntapAPIREST) SnapmirrorDeleteViaDestination(_, _ string) error {
	// TODO implement
	return nil
}

func (d OntapAPIREST) IsSVMDRCapable(_ context.Context) (bool, error) {
	// TODO implement
	return false, fmt.Errorf("not implemented")
}

func (d OntapAPIREST) SnapmirrorCreate(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName,
	remoteSVMName, replicationPolicy, replicationSchedule string) error {
	// TODO implement
	return fmt.Errorf("not implemented for REST")
}

func (d OntapAPIREST) SnapmirrorGet(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName,
	remoteSVMName string) (*Snapmirror, error) {
	// TODO implement
	return nil, fmt.Errorf("not implemented for REST")
}

func (d OntapAPIREST) SnapmirrorDelete(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName,
	remoteSVMName string) error {
	// TODO implement
	return fmt.Errorf("not implemented for REST")
}

func (d OntapAPIREST) SnapmirrorInitialize(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName,
	remoteSVMName string) error {
	// TODO implement
	return fmt.Errorf("not implemented for REST")
}

func (d OntapAPIREST) SnapmirrorResync(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName,
	remoteSVMName string) error {
	// TODO implement
	return fmt.Errorf("not implemented for REST")
}

func (d OntapAPIREST) SnapmirrorPolicyGet(ctx context.Context, replicationPolicy string) (*SnapmirrorPolicy, error) {
	// TODO implement
	return nil, fmt.Errorf("not implemented for REST")
}

func (d OntapAPIREST) SnapmirrorQuiesce(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName,
	remoteSVMName string) error {
	// TODO implement
	return fmt.Errorf("not implemented for REST")
}

func (d OntapAPIREST) SnapmirrorAbort(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName,
	remoteSVMName string) error {
	// TODO implement
	return fmt.Errorf("not implemented for REST")
}

func (d OntapAPIREST) SnapmirrorBreak(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName,
	remoteSVMName string) error {
	return fmt.Errorf("not implemented for REST")
}

func (d OntapAPIREST) JobScheduleExists(ctx context.Context, replicationSchedule string) error {
	// TODO implement
	return fmt.Errorf("not implemented for REST")
}
func (d OntapAPIREST) GetSVMPeers(ctx context.Context) ([]string, error) {
	// TODO implement
	return nil, fmt.Errorf("not implemented for REST")
}

func (d OntapAPIREST) LunList(ctx context.Context, pattern string) (Luns, error) {
	lunsResponse, err := d.api.LunList(ctx, pattern)
	if err != nil {
		return nil, err
	}

	luns := Luns{}

	if lunsResponse.Payload.Records != nil {
		for _, lun := range lunsResponse.Payload.Records {
			lunInfo, err := lunInfoFromRestAttrsHelper(lun)
			if err != nil {
				return nil, err
			}
			luns = append(luns, *lunInfo)
		}
	}

	return luns, nil
}

func (d OntapAPIREST) LunCreate(ctx context.Context, lun Lun) error {

	if d.api.ClientConfig().DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "LunCreate",
			"Type":   "OntapAPIREST",
			"spec":   lun,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> LunCreate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< LunCreate")
	}

	sizeBytesStr, _ := utils.ConvertSizeToBytes(lun.Size)
	sizeBytes, _ := strconv.ParseUint(sizeBytesStr, 10, 64)
	creationErr := d.api.LunCreate(ctx, lun.Name, int64(sizeBytes), lun.OsType, lun.Qos)
	if creationErr != nil {
		return fmt.Errorf("error creating LUN %v: %v", lun.Name, creationErr)
	}

	return nil
}

func (d OntapAPIREST) LunDestroy(ctx context.Context, lunPath string) error {
	if d.api.ClientConfig().DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "LunDestroy",
			"Type":   "OntapAPIREST",
			"Name":   lunPath,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> LunDestroy")
		defer Logc(ctx).WithFields(fields).Debug("<<<< LunDestroy")
	}

	lun, err := d.api.LunGetByName(ctx, lunPath)
	if err != nil {
		return fmt.Errorf("error getting LUN: %v", lunPath)
	}

	err = d.api.LunDelete(ctx, lun.UUID)
	if err != nil {
		return fmt.Errorf("error deleting LUN: %v", lunPath)
	}

	return nil
}

// TODO: Change this for LUN Attributes when available
func (d OntapAPIREST) LunSetAttribute(ctx context.Context, lunPath, attribute, fstype, context string) error {
	if strings.Contains(lunPath, failureLUNSetAttr) {
		return errors.New("injected error")
	}
	return d.LunSetComments(ctx, lunPath, fstype, context)
}

// TODO: Change this for LUN Attributes when available
func (d OntapAPIREST) LunGetComment(ctx context.Context, lunPath string) (string, bool, error) {
	parse := true
	comment, err := d.api.LunGetComment(ctx, lunPath)
	return comment, parse, err
}

// TODO: Change this for LUN Attributes when available
func (d OntapAPIREST) LunSetComments(ctx context.Context, lunPath, fstype, context string) error {

	setComment, err := d.GetCommentJSON(ctx, fstype, context, 254)
	if err != nil {
		return err
	}
	return d.api.LunSetComment(ctx, lunPath, setComment)
}

// TODO: Change this for LUN Attributes when available
// GetCommentJSON returns a JSON-formatted string containing the labels on this LUN.
// This is a temporary solution until we are able to implement LUN attributes in REST
// For example: {"lunAttributes":{"fstype":"xfs","driverContext":"csi"}}
func (d OntapAPIREST) GetCommentJSON(ctx context.Context, fstype, context string, commentLimit int) (string,
	error) {

	lunCommentMap := make(map[string]map[string]string)
	newcommentMap := make(map[string]string)
	newcommentMap["fstype"] = fstype
	newcommentMap["driverContext"] = context
	lunCommentMap["lunAttributes"] = newcommentMap

	lunCommentJSON, err := json.Marshal(lunCommentMap)
	if err != nil {
		Logc(ctx).Errorf("Failed to marshal lun comments: %+v", lunCommentMap)
		return "", err
	}

	commentsJsonBytes := new(bytes.Buffer)
	err = json.Compact(commentsJsonBytes, lunCommentJSON)
	if err != nil {
		Logc(ctx).Errorf("Failed to compact lun comments: %s", string(lunCommentJSON))
		return "", err
	}

	commentsJSON := commentsJsonBytes.String()

	if commentLimit != 0 && len(commentsJSON) > commentLimit {
		Logc(ctx).WithFields(log.Fields{
			"commentsJSON":       commentsJSON,
			"commentsJSONLength": len(commentsJSON),
			"maxCommentLength":   commentLimit,
		}).Error("comment length exceeds the character limit")
		return "", fmt.Errorf("comment length %v exceeds the character limit of %v characters", len(commentsJSON),
			commentLimit)
	}

	return commentsJSON, nil
}

func (d OntapAPIREST) ParseLunComment(ctx context.Context, commentJSON string) (map[string]string, error) {
	var lunComment map[string]map[string]string
	err := json.Unmarshal([]byte(commentJSON), &lunComment)
	if err != nil {
		return nil, err
	}
	lunAttrs := lunComment["lunAttributes"]

	return lunAttrs, nil
}

func (d OntapAPIREST) LunSetQosPolicyGroup(ctx context.Context, lunPath string, qosPolicyGroup QosPolicyGroup) error {
	return d.api.LunSetQosPolicyGroup(ctx, lunPath, qosPolicyGroup.Name)
}

func (d OntapAPIREST) LunGetByName(ctx context.Context, name string) (*Lun, error) {
	if d.api.ClientConfig().DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":  "LunGetByName",
			"Type":    "OntapAPIREST",
			"LunPath": name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> LunGetByName")
		defer Logc(ctx).WithFields(fields).Debug("<<<< LunGetByName")
	}

	lunResponse, err := d.api.LunGetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	lun, err := lunInfoFromRestAttrsHelper(lunResponse)
	if err != nil {
		return nil, err
	}
	return lun, nil
}

func (d OntapAPIREST) LunRename(ctx context.Context, lunPath, newLunPath string) error {
	if d.api.ClientConfig().DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "LunRename",
			"Type":       "OntapAPIREST",
			"OldLunName": lunPath,
			"NewLunName": newLunPath,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> LunRename")
		defer Logc(ctx).WithFields(fields).Debug("<<<< LunRename")
	}
	return d.api.LunRename(ctx, lunPath, newLunPath)
}

func (d OntapAPIREST) LunMapInfo(ctx context.Context, initiatorGroupName, lunPath string) (int, error) {
	lunID := -1
	info, err := d.api.LunMapInfo(ctx, initiatorGroupName, lunPath)
	if err != nil {
		return lunID, fmt.Errorf("error reading LUN maps for volume %s: %v", lunPath, err)
	}

	if info.Payload != nil {
		for _, lunMapResponse := range info.Payload.Records {
			if lunMapResponse.Igroup.Name == initiatorGroupName {
				if lunMapResponse.LogicalUnitNumber != nil {
					lunID = int(*lunMapResponse.LogicalUnitNumber)
				}
			}
		}
	}
	return lunID, nil
}

func (d OntapAPIREST) isLunMapped(
	ctx context.Context, lunPath, initiatorGroupName string, importNotManaged bool,
) (bool, int, error) {
	alreadyMapped := false
	lunID := -1

	lunMapResponse, err := d.api.LunMapInfo(ctx, "", lunPath)
	if err != nil {
		return alreadyMapped, lunID, fmt.Errorf("problem reading maps for LUN %s: %v", lunPath, err)
	}
	if lunMapResponse == nil || lunMapResponse.Payload == nil || lunMapResponse.Payload.Records == nil {
		return alreadyMapped, lunID, fmt.Errorf("problem reading maps for LUN %s", lunPath)
	}

	for _, record := range lunMapResponse.Payload.Records {
		if record.Igroup != nil {
			if record.Igroup.Name != initiatorGroupName && !importNotManaged {
				Logc(ctx).Debugf("deleting existing LUN mapping")
				err = d.api.LunUnmap(ctx, record.Igroup.Name, lunPath)
				if err != nil {
					return alreadyMapped, lunID, fmt.Errorf("problem deleting map for LUN %s", lunPath)
				}
			}
			if record.Igroup.Name == initiatorGroupName || importNotManaged {
				if record.LogicalUnitNumber != nil {
					lunID = int(*record.LogicalUnitNumber)
					alreadyMapped = true

					Logc(ctx).WithFields(
						log.Fields{
							"lun":    lunPath,
							"igroup": initiatorGroupName,
							"id":     lunID,
						},
					).Debug("LUN already mapped.")
					break
				} else {
					lun, err := d.api.LunGetByName(ctx, lunPath)
					if err != nil {
						return alreadyMapped, lunID, err
					}
					if lun != nil || lun.LunMaps != nil {
						lunID = int(lun.LunMaps[0].LogicalUnitNumber)
						alreadyMapped = true

						Logc(ctx).WithFields(
							log.Fields{
								"lun":    lunPath,
								"igroup": initiatorGroupName,
								"id":     lunID,
							},
						).Debug("LUN already mapped when checking LUN.")
						break
					}
				}
			}
		}
	}
	return alreadyMapped, lunID, nil
}

func (d OntapAPIREST) EnsureLunMapped(
	ctx context.Context, initiatorGroupName, lunPath string, importNotManaged bool,
) (int, error) {
	alreadyMapped, lunID, err := d.isLunMapped(ctx, lunPath, initiatorGroupName, importNotManaged)
	if err != nil {
		return -1, err
	}

	// Map IFF not already mapped
	if !alreadyMapped {
		lunMapResponse, err := d.api.LunMap(ctx, initiatorGroupName, lunPath, lunID)
		if err != nil {
			return -1, fmt.Errorf("err not nil, problem mapping LUN %s: %s", lunPath, err.Error())
		}
		if lunMapResponse == nil || lunMapResponse.Payload == nil || lunMapResponse.Payload.Records == nil {
			return -1, fmt.Errorf("response nil, problem mapping LUN %s: %v", lunPath, err)
		}
		if lunMapResponse.Payload.NumRecords == 1 {
			if lunMapResponse.Payload.Records[0].LogicalUnitNumber != nil {
				lunID = int(*lunMapResponse.Payload.Records[0].LogicalUnitNumber)
			}
		}

		Logc(ctx).WithFields(log.Fields{
			"lun":    lunPath,
			"igroup": initiatorGroupName,
			"id":     lunID,
		}).Debug("LUN mapped.")
	}

	return lunID, nil
}

// LunMapGetReportingNodes returns a list of LUN map details
// equivalent to filer::> lun mapping show -vserver iscsi_vs -path /vol/v/lun0 -igroup trident
func (d OntapAPIREST) LunMapGetReportingNodes(ctx context.Context, initiatorGroupName, lunPath string) (
	[]string, error,
) {
	cliPassthroughResult, err := d.api.CliPassthroughLunMappingGet(ctx, initiatorGroupName, lunPath)
	fmt.Printf("cliPassthroughResult: %v \n", cliPassthroughResult)
	fmt.Printf("err: %v \n", err)

	var results []string
	for _, rawJson := range cliPassthroughResult.Records {

		result := &LunMappingGetCliPassthroughResult{}
		unmarshalErr := json.Unmarshal(rawJson, result)
		if unmarshalErr != nil {
			log.WithField("body", string(rawJson)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
			return nil, unmarshalErr
		}

		results = append(results, result.ReportingNodes...)
	}
	return results, err
}

func (d OntapAPIREST) LunSize(ctx context.Context, flexvolName string) (int, error) {
	lunPath := fmt.Sprintf("/vol/%v/lun0", flexvolName)
	return d.api.LunSize(ctx, lunPath)
}

func (d OntapAPIREST) LunSetSize(ctx context.Context, lunPath, newSize string) (uint64, error) {
	if d.api.ClientConfig().DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":  "LunSetSize",
			"Type":    "OntapAPIREST",
			"Name":    lunPath,
			"NewSize": newSize,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> LunSetSize")
		defer Logc(ctx).WithFields(fields).Debug("<<<< LunSetSize")
	}
	return d.api.LunSetSize(ctx, lunPath, newSize)
}

func (d OntapAPIREST) IscsiInitiatorGetDefaultAuth(ctx context.Context) (IscsiInitiatorAuth, error) {
	authInfo := IscsiInitiatorAuth{}
	response, err := d.api.IscsiInitiatorGetDefaultAuth(ctx)
	if err != nil {
		return authInfo, err
	}

	if response.Payload.Records == nil {
		return authInfo, fmt.Errorf("iSCSI initiator response is nil")
	}

	if response.Payload.NumRecords == 0 {
		return authInfo, fmt.Errorf("iSCSI initiator response has no records")
	}

	if response.Payload.NumRecords > 1 {
		return authInfo, fmt.Errorf("iSCSI initiator response has too many records")
	}

	record := response.Payload.Records[0]
	if record.Svm != nil {
		authInfo.SVMName = record.Svm.Name
	}

	if record.Chap != nil {
		if record.Chap.Inbound != nil {
			authInfo.ChapUser = record.Chap.Inbound.User
			authInfo.ChapPassphrase = record.Chap.Inbound.Password
		}

		if record.Chap.Outbound != nil {
			authInfo.ChapOutboundUser = record.Chap.Outbound.User
			authInfo.ChapOutboundPassphrase = record.Chap.Outbound.Password
		}
	}

	authInfo.Initiator = record.Initiator
	authInfo.AuthType = record.AuthenticationType

	return authInfo, nil
}

func (d OntapAPIREST) IscsiInitiatorSetDefaultAuth(
	ctx context.Context, authType, userName, passphrase, outbountUserName, outboundPassphrase string,
) error {
	return d.api.IscsiInitiatorSetDefaultAuth(ctx, authType, userName, passphrase, outbountUserName, outboundPassphrase)
}

func (d OntapAPIREST) IscsiInterfaceGet(ctx context.Context, svm string) ([]string, error) {

	var iSCSINodeNames []string
	interfaceResponse, err := d.api.IscsiInterfaceGet(ctx, svm)
	if err != nil {
		return nil, fmt.Errorf("could not get SVM iSCSI node name: %v", err)
	}
	if interfaceResponse.Payload == nil {
		return nil, nil
	}
	// Get the IQN and ensure it is enabled
	for _, record := range interfaceResponse.Payload.Records {
		if *record.Enabled {
			iSCSINodeNames = append(iSCSINodeNames, record.Target.Name)
		}
	}

	if len(iSCSINodeNames) == 0 {
		return nil, fmt.Errorf(
			"SVM %s has no active iSCSI interfaces", interfaceResponse.Payload.Records[0].Svm.Name)
	}

	return iSCSINodeNames, nil
}

func (d OntapAPIREST) IscsiNodeGetNameRequest(ctx context.Context) (string, error) {
	result, err := d.api.IscsiNodeGetName(ctx)
	if err != nil {
		return "", err
	}
	if result == nil {
		return "", fmt.Errorf("iSCSI node name response is empty")
	}
	if result.Payload == nil {
		return "", fmt.Errorf("iSCSI node name payload is empty")
	}
	if result.Payload.Target == nil {
		return "", fmt.Errorf("could not get iSCSI node name target")
	}
	return result.Payload.Target.Name, nil
}

func (d OntapAPIREST) IgroupCreate(ctx context.Context, initiatorGroupName, initiatorGroupType, osType string) error {
	if d.api.ClientConfig().DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":             "IgroupCreate",
			"Type":               "OntapAPIREST",
			"InitiatorGroupName": initiatorGroupName,
			"InitiatorGroupType": initiatorGroupType,
			"OsType":             osType,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> IgroupCreate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< IgroupCreate")
	}
	igroup, err := d.api.IgroupGetByName(ctx, initiatorGroupName)
	if err != nil {
		return err
	}
	if igroup == nil {
		Logc(ctx).Debugf("igroup %s does not exist, creating new igroup now", initiatorGroupName)
		err = d.api.IgroupCreate(ctx, initiatorGroupName, initiatorGroupType, osType)
		if err != nil {
			if strings.Contains(err.Error(), "[409]") {
				return nil
			}
			return err
		}
	}

	return nil
}

func (d OntapAPIREST) IgroupDestroy(ctx context.Context, initiatorGroupName string) error {
	if d.api.ClientConfig().DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":             "IgroupDestroy",
			"Type":               "OntapAPIREST",
			"InitiatorGroupName": initiatorGroupName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> IgroupDestroy")
		defer Logc(ctx).WithFields(fields).Debug("<<<< IgroupDestroy")
	}
	return d.api.IgroupDestroy(ctx, initiatorGroupName)
}

func (d OntapAPIREST) EnsureIgroupAdded(
	ctx context.Context, initiatorGroupName, initiator string,
) error {
	if d.api.ClientConfig().DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":             "EnsureIgroupAdded",
			"Type":               "OntapAPIREST",
			"InitiatorGroupName": initiatorGroupName,
			"IQN":                initiator,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> EnsureIgroupAdded")
		defer Logc(ctx).WithFields(fields).Debug("<<<< EnsureIgroupAdded")
	}
	alreadyAdded, err := d.isIgroupAdded(ctx, initiator, initiatorGroupName)
	if err != nil {
		return err
	}

	if !alreadyAdded {
		Logc(ctx).Debugf("IQN %s not in igroup %s, adding now.", initiator, initiatorGroupName)
		return d.api.IgroupAdd(ctx, initiatorGroupName, initiator)
	}

	return nil
}

func (d OntapAPIREST) isIgroupAdded(ctx context.Context, initiator, initiatorGroupName string) (bool, error) {
	alreadyAdded := false
	igroup, err := d.api.IgroupGetByName(ctx, initiatorGroupName)
	if err != nil {
		return alreadyAdded, err
	}
	if igroup != nil || igroup.Initiators != nil {
		for _, i := range igroup.Initiators {
			if i.Name == initiator {
				Logc(ctx).Debugf("Initiator %v already in Igroup %v", initiator, initiatorGroupName)
				alreadyAdded = true
				break
			}
		}
	}
	return alreadyAdded, nil
}

func (d OntapAPIREST) IgroupRemove(ctx context.Context, initiatorGroupName, initiator string, force bool) error {
	if d.api.ClientConfig().DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":             "IgroupRemove",
			"Type":               "OntapAPIREST",
			"InitiatorGroupName": initiatorGroupName,
			"IQN":                initiator,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> IgroupRemove")
		defer Logc(ctx).WithFields(fields).Debug("<<<< IgroupRemove")
	}
	return d.api.IgroupRemove(ctx, initiatorGroupName, initiator)
}

func (d OntapAPIREST) IgroupGetByName(ctx context.Context, initiatorGroupName string) (map[string]bool,
	error) {
	// Discover mapped initiators
	iGroupResponse, err := d.api.IgroupGetByName(ctx, initiatorGroupName)
	if err != nil {
		return nil, fmt.Errorf("failed to read igroup info; %v", err)
	}
	mappedIQNs := make(map[string]bool)
	if iGroupResponse != nil {
		initiators := iGroupResponse.Initiators
		for _, initiator := range initiators {
			mappedIQNs[initiator.Name] = true
		}
	}
	return mappedIQNs, nil
}

func (d OntapAPIREST) GetReportedDataLifs(ctx context.Context) (string, []string, error) {
	var reportedDataLIFs []string
	var currentNode string
	netInterfaces, err := d.api.NetworkIPInterfacesList(ctx)
	if err != nil {
		return currentNode, nil, err
	}
	if netInterfaces.Payload.Records != nil {
		for _, netInterface := range netInterfaces.Payload.Records {
			if netInterface.State == "up" {
				reportedDataLIFs = append(reportedDataLIFs, string(netInterface.IP.Address))
			}
			currentNode = netInterface.Location.Node.Name
		}
	}
	return currentNode, reportedDataLIFs, nil
}

func (d OntapAPIREST) GetSVMUUID() string {
	return d.api.SVMUUID()
}
