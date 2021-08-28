// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"fmt"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/netapp/trident/utils"
)

//RestError encapsulates the status, reason, and errno values from a REST invocation, and it provides helper methods for detecting
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
	API RestClient
}

func NewOntapAPIREST(restClient RestClient) (OntapAPIREST, error) {
	result := OntapAPIREST{
		API: restClient,
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

func (d OntapAPIREST) VolumeCreate(ctx context.Context, volume Volume) (*APIResponse, error) {

	if d.API.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "VolumeCreate",
			"Type":   "OntapAPIREST",
			"spec":   volume,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> VolumeCreate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< VolumeCreate")
	}

	creationErr := d.API.VolumeCreate(ctx, volume.Name, volume.Aggregates[0], volume.Size, volume.SpaceReserve,
		volume.SnapshotPolicy, volume.UnixPermissions, volume.ExportPolicy, volume.SecurityStyle, volume.TieringPolicy,
		volume.Comment, volume.Qos, volume.Encrypt, volume.SnapshotReserve)
	if creationErr != nil {
		return nil, fmt.Errorf("error creating volume: %v", creationErr)
	}

	return APIResponsePassed, nil
}

func (d OntapAPIREST) VolumeDestroy(ctx context.Context, name string, _ bool) (*APIResponse, error) {

	volume, _, err := d.VolumeInfo(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("error looking up source volume: %v", err)
	}
	if volume == nil {
		return nil, fmt.Errorf("error looking up source volume: %v", name)
	}

	deletionErr := d.API.VolumeDelete(ctx, volume.UUID)
	if deletionErr != nil {
		return nil, fmt.Errorf("error destroying volume %v: %v", name, deletionErr)
	}
	return APIResponsePassed, nil
}

func (d OntapAPIREST) VolumeInfo(ctx context.Context, name string) (*Volume, *APIResponse, error) {

	volumeGetResponse, err := d.API.VolumeGetByName(ctx, name)
	if err != nil {
		Logc(ctx).Errorf("Could not find volume with name: %v, error: %v", name, err.Error())
		return nil, nil, err
	}

	if volumeGetResponse == nil {
		Logc(ctx).Errorf("Could not find volume with name: %v", name)
		return nil, nil, VolumeReadError(fmt.Sprintf("could not find volume with name %s", name))
	}

	volumeInfo, err := VolumeInfoFromRestAttrsHelper(volumeGetResponse)
	if err != nil {
		return nil, nil, err
	}

	return volumeInfo, APIResponsePassed, nil
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
	} else {
		return nil, VolumeSpaceAttributesReadError(fmt.Sprintf("error reading space attributes for volume %s",
			volumeGetResponse.Name))
	}

	if volumeGetResponse.SnapshotPolicy != nil {
		responseSnapshotPolicy = volumeGetResponse.SnapshotPolicy.Name
	}

	if volumeGetResponse.Space != nil {
		if volumeGetResponse.Space.Snapshot != nil {
			if volumeGetResponse.Space.Snapshot.ReservePercent != nil {
				responseSnapshotReserveInt = int(*volumeGetResponse.Space.Snapshot.ReservePercent)
			}
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
		SnapshotPolicy:  responseSnapshotPolicy,
		SnapshotReserve: responseSnapshotReserveInt,
		SpaceReserve:    responseSpaceReserve,
		UnixPermissions: responseUnixPermissions,
		UUID:            volumeGetResponse.UUID,
	}
	return volumeInfo, nil
}

func (d OntapAPIREST) APIVersion(ctx context.Context) (string, error) {
	return d.API.SystemGetOntapVersion(ctx)
}

func (d OntapAPIREST) NodeListSerialNumbers(ctx context.Context) ([]string, error) {
	return d.API.NodeListSerialNumbers(ctx)
}

func (d OntapAPIREST) SupportsFeature(ctx context.Context, feature feature) bool {
	return d.API.SupportsFeature(ctx, feature)
}

func (d OntapAPIREST) NetInterfaceGetDataLIFs(ctx context.Context, protocol string) ([]string, error) {
	return d.API.NetInterfaceGetDataLIFs(ctx, protocol)
}

func (d OntapAPIREST) GetSVMAggregateNames(ctx context.Context) ([]string, error) {
	return d.API.SVMGetAggregateNames(ctx)
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
	logLevel int) {

	// TODO
}

func (d OntapAPIREST) GetSVMAggregateAttributes(ctx context.Context) (aggrList map[string]string, err error) {

	// Handle panics from the API layer
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("unable to inspect ONTAP backend: %v\nStack trace:\n%s", r, debug.Stack())
		}
	}()

	result, err := d.API.AggregateList(ctx, "*")
	if result == nil || result.Payload.NumRecords == 0 || result.Payload.Records == nil {
		return nil, fmt.Errorf("could not retrieve aggregate information")
	}

	aggrList = make(map[string]string)

	for _, aggr := range result.Payload.Records {
		aggrList[aggr.Name] = aggr.BlockStorage.Primary.DiskType // TODO validate this is right
	}

	return aggrList, nil
}

func (d OntapAPIREST) ExportPolicyDestroy(ctx context.Context, policy string) (*APIResponse, error) {
	exportPolicyDestroyResult, err := d.API.ExportPolicyDestroy(ctx, policy)
	if err != nil {
		return nil, fmt.Errorf("error deleting export policy: %v", err)
	}
	if exportPolicyDestroyResult == nil {
		return nil, fmt.Errorf("error deleting export policy")
	}

	return APIResponsePassed, err
}

func (d OntapAPIREST) VolumeExists(ctx context.Context, volumeName string) (bool, error) {
	return d.API.VolumeExists(ctx, volumeName)
}

func (d OntapAPIREST) TieringPolicyValue(ctx context.Context) string {
	return d.API.TieringPolicyValue(ctx)
}

func (d OntapAPIREST) GetSVMAggregateSpace(ctx context.Context, aggregate string) ([]SVMAggregateSpace, error) {

	response, aggrSpaceErr := d.API.AggregateList(ctx, aggregate)
	if aggrSpaceErr != nil {
		return nil, aggrSpaceErr
	}
	if response == nil {
		return nil, fmt.Errorf("error looking up aggregate: %v", aggregate)
	}

	var svmAggregateSpaceList []SVMAggregateSpace

	for _, aggr := range response.Payload.Records {
		aggrName := aggr.Name
		if aggregate != aggrName {
			Logc(ctx).Debugf("Skipping " + aggrName)
			continue
		}

		aggrSpace := aggr.Space

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

func (d OntapAPIREST) VolumeDisableSnapshotDirectoryAccess(ctx context.Context, name string) (*APIResponse, error) {

	if err := d.API.VolumeDisableSnapshotDirectoryAccess(ctx, name); err != nil {
		return nil, fmt.Errorf("error disabling snapshot directory access: %v", err)
	}

	return APIResponsePassed, nil
}

func (d OntapAPIREST) VolumeMount(ctx context.Context, name, junctionPath string) (*APIResponse, error) {
	// Mount the volume at the specified junction
	if err := d.API.VolumeMount(ctx, name, junctionPath); err != nil {
		return nil, fmt.Errorf("error mounting volume %v to junction %v: %v", name, junctionPath, err)
	}

	return APIResponsePassed, nil
}

func (d OntapAPIREST) VolumeRename(ctx context.Context, originalName, newName string) error {
	return d.API.VolumeRename(ctx, originalName, newName)
}

func (d OntapAPIREST) VolumeSetComment(ctx context.Context, volumeNameInternal, volumeNameExternal, comment string) error {
	if err := d.API.VolumeSetComment(ctx, volumeNameInternal, comment); err != nil {
		Logc(ctx).WithField("originalName", volumeNameExternal).Errorf("Modifying comment failed: %v", err)
		return fmt.Errorf("volume %s modify failed: %v", volumeNameExternal, err)
	}
	return nil
}

func (d OntapAPIREST) ExportPolicyCreate(ctx context.Context, policy string) error {

	// TODO use isExportPolicyExistsRest ?
	exportPolicy, err := d.API.ExportPolicyGetByName(ctx, policy)
	if err != nil {
		err = fmt.Errorf("error checking for existing export policy %s: %v", policy, err)
	}
	if exportPolicy != nil {
		// specified export policy already exists
		return nil
	}

	// could not find the specified export policy, create it
	policyCreateResponse, err := d.API.ExportPolicyCreate(ctx, policy)
	if err != nil {
		err = fmt.Errorf("error creating export policy %s: %v", policy, err)
	} else if policyCreateResponse == nil {
		err = fmt.Errorf("error creating export policy %s", policy)
	}

	return err
}

func (d OntapAPIREST) VolumeSize(ctx context.Context, volumeName string) (int, error) {
	return d.API.VolumeSize(ctx, volumeName)
}

func (d OntapAPIREST) VolumeUsedSize(ctx context.Context, volumeName string) (int, error) {
	return d.API.VolumeUsedSize(ctx, volumeName)
}

func (d OntapAPIREST) VolumeSetSize(ctx context.Context, name, newSize string) (*APIResponse, error) {
	if err := d.API.VolumeSetSize(ctx, name, newSize); err != nil {
		Logc(ctx).WithField("error", err).Error("Volume resize failed.")
		return nil, fmt.Errorf("volume resize failed")
	}

	return APIResponsePassed, nil
}

func (d OntapAPIREST) VolumeModifyUnixPermissions(ctx context.Context, volumeNameInternal, volumeNameExternal,
	unixPermissions string) (*APIResponse, error) {
	err := d.API.VolumeModifyUnixPermissions(ctx, volumeNameInternal, unixPermissions)
	if err != nil {
		Logc(ctx).WithField("originalName", volumeNameExternal).Errorf("Could not import volume, "+
			"modifying unix permissions failed: %v", err)
		return nil, fmt.Errorf("volume %s modify failed: %v", volumeNameExternal, err)
	}

	return APIResponsePassed, nil
}

func (d OntapAPIREST) VolumeListByPrefix(ctx context.Context, prefix string) (Volumes, *APIResponse, error) {

	// TODO handle this higher? or just leave this here? i think here is OK
	if !strings.HasSuffix(prefix, "*") {
		// append the "*" to our prefix if it's missing
		prefix += "*"
	}

	volumesResponse, err := d.API.VolumeList(ctx, prefix)
	if err != nil {
		return nil, nil, err
	}

	volumes := Volumes{}

	if volumesResponse.Payload.Records != nil {
		for _, volume := range volumesResponse.Payload.Records {
			volumeInfo, err := VolumeInfoFromRestAttrsHelper(volume)
			if err != nil {
				return nil, nil, err
			}
			volumes = append(volumes, *volumeInfo)
		}
	}

	return volumes, APIResponsePassed, nil
}

func (d OntapAPIREST) ExportRuleCreate(ctx context.Context, policyName, desiredPolicyRules string) (*APIResponse, error) {
	if d.API.config.DebugTraceFlags["method"] {
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
		ruleResponse, err := d.API.ExportRuleCreate(ctx, policyName, desiredPolicyRule,
			[]string{"nfs"}, []string{"any"}, []string{"any"}, []string{"any"})
		if err != nil {
			err = fmt.Errorf("error creating export rule: %v", err)
			Logc(ctx).WithFields(log.Fields{
				"ExportPolicy": policyName,
				"ClientMatch":  desiredPolicyRule,
			}).Error(err)
			return nil, err
		}
		if ruleResponse == nil {
			return nil, fmt.Errorf("unexpected response")
		}
	}

	return APIResponsePassed, nil
}

func (d OntapAPIREST) ExportRuleDestroy(ctx context.Context, policyName string, ruleIndex int) (*APIResponse, error) {
	ruleDestroyResponse, err := d.API.ExportRuleDestroy(ctx, policyName, ruleIndex)
	if err != nil {
		err = fmt.Errorf("error deleting export rule on policy %s at index %d; %v",
			policyName, ruleIndex, err)
		Logc(ctx).WithFields(log.Fields{
			"ExportPolicy": policyName,
			"RuleIndex":    ruleIndex,
		}).Error(err)
	}

	if ruleDestroyResponse == nil {
		return nil, fmt.Errorf("unexpected response")
	}
	return APIResponsePassed, nil
}

func (d OntapAPIREST) VolumeModifyExportPolicy(ctx context.Context, volumeName, policyName string) (*APIResponse, error) {
	err := d.API.VolumeModifyExportPolicy(ctx, volumeName, policyName)
	if err != nil {
		err = fmt.Errorf("error updating export policy on volume %s: %v", volumeName, err)
		Logc(ctx).Error(err)
		return nil, err
	}

	return APIResponsePassed, nil
}

func (d OntapAPIREST) ExportPolicyExists(ctx context.Context, policyName string) (bool, error) {
	policyGetResponse, err := d.API.ExportPolicyGetByName(ctx, policyName)
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
	ruleListResponse, err := d.API.ExportRuleList(ctx, policyName)
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

func (d OntapAPIREST) SnapshotCreate(ctx context.Context, snapshotName, sourceVolume string) (*APIResponse, error) {

	volume, _, err := d.VolumeInfo(ctx, sourceVolume)
	if err != nil {
		return nil, fmt.Errorf("error looking up source volume %v: %v", sourceVolume, err)
	}
	if volume == nil {
		return nil, fmt.Errorf("error looking up source volume: %v", sourceVolume)
	}

	if err = d.API.SnapshotCreateAndWait(ctx, volume.UUID, snapshotName); err != nil {
		return nil, fmt.Errorf("could not create snapshot: %v", err)
	}
	return APIResponsePassed, nil
}

func (d OntapAPIREST) VolumeCloneCreate(ctx context.Context, cloneName, sourceName, snapshot string, _ bool) (
	*APIResponse, error) {
	err := d.API.VolumeCloneCreateAsync(ctx, cloneName, sourceName, snapshot)
	if err != nil {
		return nil, fmt.Errorf("error creating clone: %v", err)
	}
	return APIResponsePassed, nil
}

func (d OntapAPIREST) SnapshotList(ctx context.Context, sourceVolume string) (Snapshots, *APIResponse, error) {

	volume, _, err := d.VolumeInfo(ctx, sourceVolume)
	if err != nil {
		return nil, nil, fmt.Errorf("error looking up source volume: %v", err)
	}
	if volume == nil {
		return nil, nil, fmt.Errorf("error looking up source volume: %v", sourceVolume)
	}

	snapListResponse, err := d.API.SnapshotList(ctx, volume.UUID)
	if err != nil {
		return nil, nil, fmt.Errorf("error enumerating snapshots: %v", err)
	}
	if snapListResponse == nil {
		return nil, nil, fmt.Errorf("error enumerating snapshots")
	}
	snapshots := Snapshots{}

	for _, snap := range snapListResponse.Payload.Records {
		snapshots = append(snapshots, Snapshot{
			CreateTime: snap.CreateTime.String(), // TODO do we need to format this?
			Name:       snap.Name,
		})
	}

	Logc(ctx).Debugf("Returned %v snapshots.", snapListResponse.Payload.NumRecords)

	return snapshots, APIResponsePassed, nil
}

func (d OntapAPIREST) VolumeSetQosPolicyGroupName(ctx context.Context, name string, qos QosPolicyGroup) (*APIResponse,
	error) {

	if err := d.API.VolumeSetQosPolicyGroupName(ctx, name, qos); err != nil {
		return nil, fmt.Errorf("error setting quality of service policy: %v", err)
	}

	return APIResponsePassed, nil
}

func (d OntapAPIREST) VolumeCloneSplitStart(ctx context.Context, cloneName string) (*APIResponse, error) {
	if err := d.API.VolumeCloneSplitStart(ctx, cloneName); err != nil {
		return nil, fmt.Errorf("error splitting clone: %v", err)
	}
	return APIResponsePassed, nil
}

func (d OntapAPIREST) SnapshotRestoreVolume(ctx context.Context, snapshotName, sourceVolume string) (*APIResponse, error) {
	if err := d.API.SnapshotRestoreVolume(ctx, snapshotName, sourceVolume); err != nil {
		return nil, fmt.Errorf("error restoring snapshot: %v", err)
	}

	return APIResponsePassed, nil
}
func (d OntapAPIREST) SnapshotDelete(ctx context.Context, snapshotName, sourceVolume string) (*APIResponse, error) {

	volume, _, err := d.VolumeInfo(ctx, sourceVolume)
	if err != nil {
		return nil, fmt.Errorf("error looking up source volume: %v", err)
	}
	if volume == nil {
		return nil, fmt.Errorf("error looking up source volume: %v", sourceVolume)
	}
	volumeUUID := volume.UUID

	// GET the snapshot by name
	snapshot, err := d.API.SnapshotGetByName(ctx, volumeUUID, snapshotName)
	if err != nil {
		return nil, fmt.Errorf("error checking for snapshot: %v", err)
	}
	if snapshot == nil {
		return nil, fmt.Errorf("error looking up snapshot: %v", snapshotName)
	}
	snapshotUUID := snapshot.UUID

	// DELETE the snapshot
	snapshotDeleteResult, err := d.API.SnapshotDelete(ctx, volumeUUID, snapshotUUID)
	if err != nil {
		return nil, fmt.Errorf("error while deleting snapshot: %v", err)
	}
	if snapshotDeleteResult == nil {
		return nil, fmt.Errorf("error while deleting snapshot: %v", snapshotName)
	}

	// check snapshot delete job status
	snapshotDeleteErr := d.API.PollJobStatus(ctx, snapshotDeleteResult.Payload)
	// if err := client.PollJobStatus(ctx, snapshotDeleteResult.Payload); err != nil {
	if snapshotDeleteErr != nil {
		Logc(ctx).Debugf("Could not delete the snapshot, going to check if it's busy; error was: %v", snapshotDeleteErr)
		if restErr, ok := snapshotDeleteErr.(RestError); ok {
			Logc(ctx).Debugf("restErr: %v", restErr)
			Logc(ctx).Debugf("restErr.Code(): %v", restErr.Code())
			if restErr.IsSnapshotBusy() {
				Logc(ctx).Debug("Snapshot was busy, going to split it")
				// Start a split here before returning the error so a subsequent delete attempt may succeed.
				return nil, SnapshotBusyError(fmt.Sprintf("snapshot %s backing volume %s is busy", snapshotName,
					sourceVolume))
			}
		}
		return nil, snapshotDeleteErr
	}

	return APIResponsePassed, nil
}

func (d OntapAPIREST) VolumeListBySnapshotParent(ctx context.Context, snapshotName, sourceVolume string) (
	VolumeNameList, *APIResponse, error) {

	childVolumes, err := d.API.VolumeListAllBackedBySnapshot(ctx, sourceVolume, snapshotName)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"snapshotName":     snapshotName,
			"parentVolumeName": sourceVolume,
			"error":            err,
		}).Error("Could not list volumes backed by snapshot.")
		return nil, nil, err
	} else if len(childVolumes) == 0 {
		return nil, nil, nil
	}

	// We're going to start a single split operation, but there could be multiple children, so we
	// sort the volumes by name to not have more than one split operation running at a time.
	sort.Strings(childVolumes)

	return childVolumes, APIResponsePassed, nil
}

func (d OntapAPIREST) SnapmirrorDeleteViaDestination(_, _ string) (*APIResponse, error) {
	// TODO implement
	return nil, nil // nothing to do yet for REST

	// TODO @ameade/ @rippy
	// return &APIResponse{
	// 	apiName: apiName,
	// 	status:  "failed",
	// 	reason:  "not implemented",
	// 	errno:   "",
	// }, nil
}

func (d OntapAPIREST) IsSVMDRCapable(_ context.Context) (bool, error) {

	// TODO @ameade/ @rippy
	return false, fmt.Errorf("not implemented")
}
