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

	"github.com/netapp/trident/utils"

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

	if d.api.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "VolumeCreate",
			"Type":   "OntapAPIZAPI",
			"spec":   volume,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> VolumeCreate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< VolumeCreate")
	}

	volCreateResponse, err := d.api.VolumeCreate(ctx, volume.Name, volume.Aggregates[0], volume.Size,
		volume.SpaceReserve, volume.SnapshotPolicy, volume.UnixPermissions, volume.ExportPolicy,
		volume.SecurityStyle, volume.TieringPolicy, volume.Comment, volume.Qos, volume.Encrypt,
		volume.SnapshotReserve, volume.DPVolume)
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
				Logc(ctx).WithField("volume", volume.Name).Warn(
					"Volume create job already exists, skipping volume create on this node.",
				)
				err = VolumeCreateJobExistsError(fmt.Sprintf("volume create job already exists, %s", volume.Name))
			}
		}
	}

	return err
}

func (d OntapAPIZAPI) VolumeDestroy(ctx context.Context, name string, force bool) error {

	volDestroyResponse, err := d.api.VolumeDestroy(name, force)
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

	if d.api.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "VolumeInfo",
			"Type":   "OntapAPIZAPI",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> VolumeInfo")
		defer Logc(ctx).WithFields(fields).Debug("<<<< VolumeInfo")
	}

	// Get Flexvol by name
	volumeGetResponse, err := d.api.VolumeGet(name)

	Logc(ctx).WithFields(log.Fields{
		"volumeGetResponse": volumeGetResponse,
		"name":              name,
		"err":               err,
	}).Debug("d.api.VolumeGet")

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
	var responseSnapshotSpaceUsed int
	var responseSpaceReserve string
	var responseUnixPermissions string

	if volumeGetResponse.VolumeIdAttributesPtr != nil {
		volumeIdAttributes := volumeGetResponse.VolumeIdAttributes()

		responseAggregates = []string{}
		if volumeIdAttributes.ContainingAggregateNamePtr != nil {
			responseAggregates = append(responseAggregates, volumeIdAttributes.ContainingAggregateName())
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
		return nil, VolumeIdAttributesReadError(
			fmt.Sprintf("error reading id attributes for volume %s", responseName),
		)
	}

	if volumeGetResponse.VolumeSpaceAttributesPtr != nil {
		volumeSpaceAttrs := volumeGetResponse.VolumeSpaceAttributes()
		if volumeSpaceAttrs.PercentageSnapshotReservePtr != nil {
			responseSnapshotReserveInt = volumeSpaceAttrs.PercentageSnapshotReserve()
		}
		if volumeSpaceAttrs.SizeUsedBySnapshotsPtr != nil {
			responseSnapshotSpaceUsed = volumeSpaceAttrs.SizeUsedBySnapshots()
		}
		if volumeGetResponse.VolumeSpaceAttributesPtr.SpaceGuaranteePtr != nil {
			responseSpaceReserve = volumeGetResponse.VolumeSpaceAttributesPtr.SpaceGuarantee()
		}
		if volumeGetResponse.VolumeSpaceAttributesPtr.SizePtr != nil {
			responseSize = strconv.FormatInt(int64(volumeGetResponse.VolumeSpaceAttributesPtr.Size()), 10)
		} else {
			return nil, VolumeSpaceAttributesReadError(fmt.Sprintf("volume %s size not available", responseName))
		}
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
		AccessType:        responseAccessType,
		Aggregates:        responseAggregates,
		Comment:           responseComment,
		ExportPolicy:      responseExportPolicy,
		JunctionPath:      responseJunctionPath,
		Name:              responseName,
		Size:              responseSize,
		SnapshotDir:       responseSnapdirAccessEnabled,
		SnapshotPolicy:    responseSnapshotPolicy,
		SnapshotReserve:   responseSnapshotReserveInt,
		SnapshotSpaceUsed: responseSnapshotSpaceUsed,
		SpaceReserve:      responseSpaceReserve,
		UnixPermissions:   responseUnixPermissions,
		DPVolume:          responseAccessType == "dp",
	}
	return volumeInfo, nil
}

func (d OntapAPIZAPI) APIVersion(ctx context.Context) (string, error) {
	return d.api.SystemGetOntapiVersion(ctx)
}

func (d OntapAPIZAPI) SupportsFeature(ctx context.Context, feature Feature) bool {
	return d.api.SupportsFeature(ctx, feature)
}

func (d OntapAPIZAPI) NodeListSerialNumbers(ctx context.Context) ([]string, error) {
	return d.api.NodeListSerialNumbers(ctx)
}

type OntapAPIZAPI struct {
	api *Client
}

func (d OntapAPIZAPI) ParseLunComment(ctx context.Context, commentJSON string) (map[string]string, error) {
	// ParseLunComment shouldn't be called as it isn't needed for ZAPI
	Logc(ctx).Info("Not implemented")
	return nil, nil
}

func (d OntapAPIZAPI) LunList(ctx context.Context, pattern string) (Luns, error) {
	lunResponse, err := d.api.LunGetAll(pattern)
	if err = GetError(ctx, lunResponse, err); err != nil {
		return nil, err
	}

	luns := Luns{}

	if lunResponse.Result.AttributesListPtr != nil {
		for _, lun := range lunResponse.Result.AttributesListPtr.LunInfoPtr {
			lunInfo, err := lunInfoFromZapiAttrsHelper(lun)
			if err != nil {
				return nil, err
			}
			luns = append(luns, *lunInfo)
		}
	}

	return luns, nil
}

func (d OntapAPIZAPI) LunCreate(ctx context.Context, lun Lun) error {
	if d.api.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "LunCreate",
			"Type":   "OntapAPIZAPI",
			"spec":   lun,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> LunCreate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< LunCreate")
	}

	sizeBytesStr, _ := utils.ConvertSizeToBytes(lun.Size)
	sizeBytes, _ := strconv.ParseUint(sizeBytesStr, 10, 64)

	lunCreateResponse, err := d.api.LunCreate(lun.Name, int(sizeBytes), lun.OsType, lun.Qos, *lun.SpaceReserved,
		*lun.SpaceAllocated)

	if err != nil {
		return fmt.Errorf("error creating LUN: %v", err)
	}

	if lunCreateResponse == nil {
		return fmt.Errorf("missing LUN create response")
	}

	if err = GetError(ctx, lunCreateResponse, err); err != nil {
		if zerr, ok := err.(ZapiError); ok {
			// Handle case where the Create is passed to every Docker Swarm node
			if zerr.Code() == azgo.EAPIERROR && strings.HasSuffix(strings.TrimSpace(zerr.Reason()), "Job exists") {
				Logc(ctx).WithField("LUN", lun.Name).Warn("LUN create job already exists, " +
					"skipping LUN create on this node.")
				err = VolumeCreateJobExistsError(fmt.Sprintf("LUN create job already exists, %s", lun.Name))
			}
		}
	}

	return err
}

func (d OntapAPIZAPI) LunDestroy(ctx context.Context, lunPath string) error {
	_, err := d.api.LunDestroy(lunPath)
	return err
}

func (d OntapAPIZAPI) LunGetComment(ctx context.Context, lunPath string) (string, bool, error) {
	// TODO: refactor, this is specifically for getting the fstype
	var fstype string
	parse := false
	LUNAttributeFSType := "com.netapp.ndvp.fstype"
	attrResponse, err := d.api.LunGetAttribute(lunPath, LUNAttributeFSType)
	if err = GetError(ctx, attrResponse, err); err != nil {
		return "", parse, err
	} else {
		fstype = attrResponse.Result.Value()
		Logc(ctx).WithFields(log.Fields{"LUN": lunPath, "fstype": fstype}).Debug("Found LUN attribute fstype.")
	}
	return fstype, parse, nil
}

func (d OntapAPIZAPI) LunSetAttribute(ctx context.Context, lunPath, attribute, fstype, context string) error {
	var attrResponse interface{}
	var err error
	if fstype != "" {
		attrResponse, err = d.api.LunSetAttribute(lunPath, attribute, fstype)
	}
	if err = GetError(ctx, attrResponse, err); err != nil {
		return err
	}

	if context != "" {
		attrResponse, err = d.api.LunSetAttribute(lunPath, "context", context)
	}
	if err = GetError(ctx, attrResponse, err); err != nil {
		Logc(ctx).WithField("LUN", lunPath).Warning("Failed to save the driver context attribute for new LUN.")
	}

	return nil
}

func (d OntapAPIZAPI) LunSetQosPolicyGroup(ctx context.Context, lunPath string, qosPolicyGroup QosPolicyGroup) error {
	qosResponse, err := d.api.LunSetQosPolicyGroup(lunPath, qosPolicyGroup)
	if err = GetError(ctx, qosResponse, err); err != nil {
		return err
	}
	return nil
}

func (d OntapAPIZAPI) LunGetByName(ctx context.Context, name string) (*Lun, error) {
	if d.api.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":  "LunGetByName",
			"Type":    "OntapAPIZAPI",
			"LunPath": name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> LunGetByName")
		defer Logc(ctx).WithFields(fields).Debug("<<<< LunGetByName")
	}
	lunResponse, err := d.api.LunGet(name)
	if err != nil || lunResponse == nil {
		return nil, fmt.Errorf("could not get LUN by name %v, error: %v", name, err)
	}

	lun, err := lunInfoFromZapiAttrsHelper(*lunResponse)
	if err != nil {
		return nil, err
	}
	return lun, nil
}

func lunInfoFromZapiAttrsHelper(lunResponse azgo.LunInfoType) (*Lun, error) {
	var responseComment string
	var responseName string
	var responseQos string
	var responseSize string
	var responseMapped bool
	var responseUUID string
	var responseSerial string
	var responseState string
	var responseVolumeName string
	var responseSpaceReserved bool
	var responseSpaceAllocated bool

	if lunResponse.CommentPtr != nil {
		responseComment = lunResponse.Comment()
	}

	if lunResponse.PathPtr != nil {
		responseName = lunResponse.Path()
	}

	if lunResponse.QosPolicyGroupPtr != nil {
		responseQos = lunResponse.QosPolicyGroup()
	}

	if lunResponse.SizePtr != nil {
		responseSize = strconv.FormatUint(uint64(lunResponse.Size()), 10)
	}

	if lunResponse.MappedPtr != nil {
		responseMapped = lunResponse.Mapped()
	}

	if lunResponse.UuidPtr != nil {
		responseUUID = lunResponse.Uuid()
	}

	if lunResponse.SerialNumberPtr != nil {
		responseSerial = lunResponse.SerialNumber()
	}

	if lunResponse.OnlinePtr != nil {
		if lunResponse.Online() {
			responseState = "online"
		} else {
			responseState = "offline"
		}
	}

	if lunResponse.VolumePtr != nil {
		responseVolumeName = lunResponse.Volume()
	}

	if lunResponse.IsSpaceReservationEnabledPtr != nil {
		responseSpaceReserved = lunResponse.IsSpaceReservationEnabled()
	}

	if lunResponse.IsSpaceAllocEnabledPtr != nil {
		responseSpaceAllocated = lunResponse.IsSpaceAllocEnabled()
	}

	lunInfo := &Lun{
		Comment:        responseComment,
		Name:           responseName,
		Qos:            QosPolicyGroup{Name: responseQos},
		Size:           responseSize,
		Mapped:         responseMapped,
		UUID:           responseUUID,
		SerialNumber:   responseSerial,
		State:          responseState,
		VolumeName:     responseVolumeName,
		SpaceReserved:  ToBoolPointer(responseSpaceReserved),
		SpaceAllocated: ToBoolPointer(responseSpaceAllocated),
	}
	return lunInfo, nil
}

func (d OntapAPIZAPI) LunRename(ctx context.Context, lunPath, newLunPath string) error {
	renameResponse, err := d.api.LunRename(lunPath, newLunPath)
	if err = GetError(ctx, renameResponse, err); err != nil {
		Logc(ctx).WithField("originalName", lunPath).Errorf("renaming LUN failed: %v", err)
		return fmt.Errorf("LUN %s rename failed: %v", lunPath, err)
	}

	return nil
}

func (d OntapAPIZAPI) LunMapInfo(ctx context.Context, initiatorGroupName, lunPath string) (int, error) {
	lunID := -1
	lunMapResponse, err := d.api.LunMapListInfo(lunPath)
	if err = GetError(ctx, lunMapResponse, err); err != nil {
		return lunID, err
	}
	if lunMapResponse.Result.InitiatorGroupsPtr != nil {
		for _, lunMapResponse := range lunMapResponse.Result.InitiatorGroupsPtr.InitiatorGroupInfoPtr {
			if lunMapResponse.InitiatorGroupName() == initiatorGroupName {
				lunID = lunMapResponse.LunId()
			}
		}
	}
	return lunID, nil
}

func (d OntapAPIZAPI) EnsureLunMapped(
	ctx context.Context, initiatorGroupName, lunPath string, importNotManaged bool,
) (int, error) {
	return d.api.LunMapIfNotMapped(ctx, initiatorGroupName, lunPath, importNotManaged)
}

func (d OntapAPIZAPI) LunSize(ctx context.Context, flexvolName string) (int, error) {
	size, err := d.api.LunSize(flexvolName)
	if err != nil {
		return 0, err
	}
	return size, nil
}

func (d OntapAPIZAPI) LunSetSize(ctx context.Context, lunPath, newSize string) (uint64, error) {
	sizeBytesStr, err := utils.ConvertSizeToBytes(newSize)
	if err != nil {
		return 0, err
	}
	sizeBytes, err := strconv.ParseUint(sizeBytesStr, 10, 64)
	if err != nil {
		return 0, err
	}
	return d.api.LunResize(lunPath, int(sizeBytes))
}

func (d OntapAPIZAPI) LunMapGetReportingNodes(ctx context.Context, initiatorGroupName, lunPath string) (
	[]string, error,
) {
	var results []string
	lunMapGetResponse, err := d.api.LunMapGet(initiatorGroupName, lunPath)
	if err != nil {
		return nil, fmt.Errorf("could not get iSCSI reported nodes: %v", err)
	}
	if lunMapGetResponse.Result.AttributesListPtr != nil {
		for _, lunMapInfo := range lunMapGetResponse.Result.AttributesListPtr {
			for _, reportingNode := range lunMapInfo.ReportingNodes() {
				results = append(results, reportingNode)
			}
		}
	}

	return results, nil
}

func (d OntapAPIZAPI) IscsiInitiatorGetDefaultAuth(ctx context.Context) (IscsiInitiatorAuth, error) {
	authInfo := IscsiInitiatorAuth{}
	apiResponse, err := d.api.IscsiInitiatorGetDefaultAuth()
	err = GetError(ctx, apiResponse, err)
	if err != nil {
		return authInfo, err
	}

	if apiResponse.Result.AuthTypePtr != nil {
		authInfo.AuthType = apiResponse.Result.AuthType()
	}

	if apiResponse.Result.UserNamePtr != nil {
		authInfo.ChapUser = apiResponse.Result.UserName()
	}

	if apiResponse.Result.OutboundUserNamePtr != nil {
		authInfo.ChapOutboundUser = apiResponse.Result.OutboundUserName()
	}

	return authInfo, nil
}

func (d OntapAPIZAPI) IscsiInitiatorSetDefaultAuth(
	ctx context.Context, authType, userName, passphrase, outbountUserName, outboundPassphrase string,
) error {
	response, err := d.api.IscsiInitiatorSetDefaultAuth(authType, userName, passphrase, outbountUserName,
		outboundPassphrase)
	err = GetError(ctx, response, err)
	if err != nil {
		return err
	}
	return nil
}

func (d OntapAPIZAPI) IscsiInterfaceGet(ctx context.Context, svm string) ([]string, error) {
	var iSCSIInterfaces []string
	interfaceResponse, err := d.api.IscsiInterfaceGetIterRequest()
	err = GetError(ctx, interfaceResponse, err)
	if err != nil {
		Logc(ctx).Debugf("could not get SVM iSCSI interfaces: %v", err)
		return nil, err
	}
	if interfaceResponse.Result.AttributesListPtr != nil {
		for _, iscsiAttrs := range interfaceResponse.Result.AttributesListPtr.IscsiInterfaceListEntryInfoPtr {
			if !iscsiAttrs.IsInterfaceEnabled() {
				continue
			}
			iSCSIInterface := fmt.Sprintf("%s:%d", iscsiAttrs.IpAddress(), iscsiAttrs.IpPort())
			iSCSIInterfaces = append(iSCSIInterfaces, iSCSIInterface)
		}
	}
	if len(iSCSIInterfaces) == 0 {
		return nil, fmt.Errorf("SVM %s has no active iSCSI interfaces", svm)
	}
	return iSCSIInterfaces, nil
}

func (d OntapAPIZAPI) IscsiNodeGetNameRequest(ctx context.Context) (string, error) {
	nodeNameResponse, err := d.api.IscsiNodeGetNameRequest()
	if err != nil {
		return "", err
	}
	return nodeNameResponse.Result.NodeName(), nil
}

func (d OntapAPIZAPI) IgroupCreate(ctx context.Context, initiatorGroupName, initiatorGroupType, osType string) error {
	response, err := d.api.IgroupCreate(initiatorGroupName, initiatorGroupType, osType)
	err = GetError(ctx, response, err)
	zerr, zerrOK := err.(ZapiError)
	if err == nil || (zerrOK && zerr.Code() == azgo.EVDISK_ERROR_NO_SUCH_INITGROUP) {
		Logc(ctx).WithField("igroup", initiatorGroupName).Debug("No such initiator group (igroup).")
	} else if zerr.Code() == azgo.EVDISK_ERROR_INITGROUP_EXISTS {
		Logc(ctx).WithField("igroup", initiatorGroupName).Info("Initiator group (igroup) already exists.")
	} else {
		Logc(ctx).WithFields(log.Fields{
			"igroup": initiatorGroupName,
			"error":  err.Error(),
		}).Error("Initiator group (igroup) could not be created.")
		return err
	}
	return nil
}

// Same functionality as cleanIgroups from ontap_common.go
func (d OntapAPIZAPI) IgroupDestroy(ctx context.Context, initiatorGroupName string) error {
	response, err := d.api.IgroupDestroy(initiatorGroupName)
	err = GetError(ctx, response, err)
	zerr, zerrOK := err.(ZapiError)
	if err == nil || (zerrOK && zerr.Code() == azgo.EVDISK_ERROR_NO_SUCH_INITGROUP) {
		Logc(ctx).WithField("igroup", initiatorGroupName).Debug("No such initiator group (igroup).")
	} else if zerr.Code() == azgo.EVDISK_ERROR_INITGROUP_MAPS_EXIST {
		Logc(ctx).WithField("igroup", initiatorGroupName).Info("Initiator group (igroup) currently in use.")
	} else {
		Logc(ctx).WithFields(log.Fields{
			"igroup": initiatorGroupName,
			"error":  err.Error(),
		}).Error("Initiator group (igroup) could not be deleted.")
		return err
	}
	return nil
}

func (d OntapAPIZAPI) EnsureIgroupAdded(ctx context.Context, initiatorGroupName, initiator string) error {
	response, err := d.api.IgroupAdd(initiatorGroupName, initiator)
	err = GetError(ctx, response, err)
	zerr, zerrOK := err.(ZapiError)
	if err == nil || (zerrOK && zerr.Code() == azgo.EVDISK_ERROR_INITGROUP_HAS_NODE) {
		Logc(ctx).WithFields(log.Fields{
			"IQN":    initiator,
			"igroup": initiatorGroupName,
		}).Debug("Host IQN already in igroup.")
	} else {
		return fmt.Errorf("error adding IQN %v to igroup %v: %v", initiator, initiatorGroupName, err)
	}
	return nil
}

func (d OntapAPIZAPI) IgroupRemove(ctx context.Context, initiatorGroupName, initiator string, force bool) error {
	response, err := d.api.IgroupRemove(initiatorGroupName, initiator, force)
	err = GetError(ctx, response, err)
	zerr, zerrOK := err.(ZapiError)
	if err == nil || (zerrOK && zerr.Code() == azgo.EVDISK_ERROR_NODE_NOT_IN_INITGROUP) {
		Logc(ctx).WithFields(log.Fields{
			"IQN":    initiator,
			"igroup": initiatorGroupName,
		}).Debug("Host IQN not in igroup.")
	} else {
		return fmt.Errorf("error removing IQN %v from igroup %v: %v", initiator, initiatorGroupName, err)
	}

	return nil
}

func (d OntapAPIZAPI) IgroupGetByName(ctx context.Context, initiatorGroupName string) (
	map[string]bool, error,
) {
	response, err := d.api.IgroupGet(initiatorGroupName)
	if err != nil {
		return nil, err
	}
	mappedIQNs := make(map[string]bool)
	if response.InitiatorsPtr != nil {
		if response.InitiatorsPtr.InitiatorInfoPtr != nil {
			initiators := response.InitiatorsPtr.InitiatorInfo()
			for _, initiator := range initiators {
				if initiator.InitiatorNamePtr != nil {
					mappedIQNs[initiator.InitiatorName()] = true
				}
			}
		}
	}
	return mappedIQNs, nil
}

func (d OntapAPIZAPI) GetReportedDataLifs(ctx context.Context) (string, []string, error) {
	var reportedDataLIFs []string
	var currentNode string
	response, err := d.api.NetInterfaceGet()
	if err != nil {
		return currentNode, nil, nil
	}
	if response.Result.AttributesListPtr != nil && response.Result.AttributesListPtr.NetInterfaceInfoPtr != nil {
		for _, netInterface := range response.Result.AttributesListPtr.NetInterfaceInfo() {
			if netInterface.CurrentNodePtr != nil {
				currentNode = netInterface.CurrentNode()
			}
			reportedDataLIFs = append(reportedDataLIFs, netInterface.Address())
		}
	}

	return currentNode, reportedDataLIFs, nil
}

func NewOntapAPIZAPI(zapiClient *Client) (OntapAPIZAPI, error) {
	result := OntapAPIZAPI{
		api: zapiClient,
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

func NewZAPIClientFromOntapConfig(
	ctx context.Context, ontapConfig *drivers.OntapStorageDriverConfig, numRecords int,
) (OntapAPI, error) {

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

		client.svmUUID = vserverResponse.Result.AttributesPtr.VserverInfoPtr.Uuid()

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
		client.svmUUID = svmUUID
	}

	apiZAPI, err := NewOntapAPIZAPI(client)
	if err != nil {
		return nil, fmt.Errorf("unable to get zapi client for ontap: %v", err)
	}

	return apiZAPI, nil
}

func (d OntapAPIZAPI) NetInterfaceGetDataLIFs(ctx context.Context, protocol string) ([]string, error) {
	return d.api.NetInterfaceGetDataLIFs(ctx, protocol)
}

func (d OntapAPIZAPI) GetSVMAggregateNames(_ context.Context) ([]string, error) {
	return d.api.SVMGetAggregateNames()
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
	logLevel int,
) {

	emsResponse, err := d.api.EmsAutosupportLog(
		appVersion, autoSupport, category, computerName, eventDescription, eventID, eventSource, logLevel,
	)

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
	volExists, err := d.api.FlexGroupExists(ctx, volumeName)
	if err != nil {
		err = fmt.Errorf("error checking for existing FlexGroup: %v", err)
	}
	return volExists, err
}

func (d OntapAPIZAPI) FlexgroupCreate(ctx context.Context, volume Volume) error {
	if d.api.config.DebugTraceFlags["method"] {
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

	flexgroupCreateResponse, err := d.api.FlexGroupCreate(ctx, volume.Name, int(sizeBytes), volume.Aggregates,
		volume.SpaceReserve,
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
	snapDirResponse, err := d.api.FlexGroupVolumeDisableSnapshotDirectoryAccess(ctx, volumeName)
	if err = GetError(ctx, snapDirResponse, err); err != nil {
		return fmt.Errorf("error disabling snapshot directory access: %v", err)
	}

	return nil
}

func (d OntapAPIZAPI) FlexgroupInfo(ctx context.Context, volumeName string) (*Volume, error) {
	if d.api.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "FlexgroupInfo",
			"Type":   "OntapAPIZAPI",
			"name":   volumeName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> FlexgroupInfo")
		defer Logc(ctx).WithFields(fields).Debug("<<<< FlexgroupInfo")
	}

	// Get Flexvol by name
	flexGroupGetResponse, err := d.api.FlexGroupGet(volumeName)

	Logc(ctx).WithFields(log.Fields{
		"flexGroupGetResponse": flexGroupGetResponse,
		"name":                 volumeName,
		"err":                  err,
	}).Debug("d.api.FlexGroupGet")

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

func (d OntapAPIZAPI) FlexgroupSetComment(
	ctx context.Context, volumeNameInternal, volumeNameExternal, comment string,
) error {
	modifyCommentResponse, err := d.api.FlexGroupSetComment(ctx, volumeNameInternal, comment)
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

func (d OntapAPIZAPI) FlexgroupModifyUnixPermissions(
	ctx context.Context, volumeNameInternal, volumeNameExternal, unixPermissions string,
) error {
	modifyUnixPermResponse, err := d.api.FlexGroupModifyUnixPermissions(ctx, volumeNameInternal, unixPermissions)
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
	offlineResp, offErr := d.api.VolumeOffline(volumeName)
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

	volDestroyResponse, err := d.api.FlexGroupDestroy(ctx, volumeName, force)
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
	volumesResponse, err := d.api.FlexGroupGetAll(prefix)
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
			volumes = append(volumes, volumeInfo)
		}
	}

	return volumes, nil
}

func (d OntapAPIZAPI) FlexgroupSetSize(ctx context.Context, name, newSize string) error {
	_, err := d.api.FlexGroupSetSize(ctx, name, newSize)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("FlexGroup resize failed.")
		return fmt.Errorf("flexgroup resize failed %v: %v", name, err)
	}
	return nil
}

func (d OntapAPIZAPI) FlexgroupSize(_ context.Context, volumeName string) (uint64, error) {
	size, err := d.api.FlexGroupSize(volumeName)
	return uint64(size), err
}

func (d OntapAPIZAPI) FlexgroupUsedSize(_ context.Context, volumeName string) (int, error) {
	return d.api.FlexGroupUsedSize(volumeName)
}

func (d OntapAPIZAPI) FlexgroupUnmount(ctx context.Context, name string, force bool) error {
	// This call is sync and idempotent
	umountResp, err := d.api.VolumeUnmount(name, force)
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

	result, err := d.api.VserverShowAggrGetIterRequest()
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
	response, err := d.api.ExportPolicyDestroy(policy)
	if err = GetError(ctx, response, err); err != nil {
		err = fmt.Errorf("error deleting export policy: %v", err)
	}
	return err
}

func (d OntapAPIZAPI) VolumeExists(ctx context.Context, volumeName string) (bool, error) {
	return d.api.VolumeExists(ctx, volumeName)
}

func (d OntapAPIZAPI) TieringPolicyValue(ctx context.Context) string {
	return d.api.TieringPolicyValue(ctx)
}

func (d OntapAPIZAPI) GetSVMAggregateSpace(ctx context.Context, aggregate string) ([]SVMAggregateSpace, error) {
	// lookup aggregate
	aggrSpaceResponse, aggrSpaceErr := d.api.AggrSpaceGetIterRequest(aggregate)
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
	snapDirResponse, err := d.api.VolumeDisableSnapshotDirectoryAccess(name)
	if err = GetError(ctx, snapDirResponse, err); err != nil {
		return fmt.Errorf("error disabling snapshot directory access: %v", err)
	}

	return nil
}

func (d OntapAPIZAPI) VolumeMount(ctx context.Context, name, junctionPath string) error {

	mountResponse, err := d.api.VolumeMount(name, junctionPath)
	if err = GetError(ctx, mountResponse, err); err != nil {
		if err.(ZapiError).Code() == azgo.EAPIERROR {
			return ApiError(fmt.Sprintf("%v", err))
		}
		return fmt.Errorf("error mounting volume to junction: %v", err)
	}

	return nil
}

func (d OntapAPIZAPI) VolumeRename(ctx context.Context, originalName, newName string) error {
	renameResponse, err := d.api.VolumeRename(originalName, newName)
	if err = GetError(ctx, renameResponse, err); err != nil {
		Logc(ctx).WithField("originalName", originalName).Errorf("renaming volume failed: %v", err)
		return fmt.Errorf("volume %s rename failed: %v", originalName, err)
	}

	return nil
}

func (d OntapAPIZAPI) VolumeSetComment(
	ctx context.Context, volumeNameInternal, volumeNameExternal, comment string,
) error {
	modifyCommentResponse, err := d.api.VolumeSetComment(ctx, volumeNameInternal, comment)
	if err = GetError(ctx, modifyCommentResponse, err); err != nil {
		Logc(ctx).WithField("originalName", volumeNameExternal).Errorf("Modifying comment failed: %v", err)
		return fmt.Errorf("volume %s modify failed: %v", volumeNameExternal, err)
	}

	return nil
}

func (d OntapAPIZAPI) ExportPolicyCreate(ctx context.Context, policy string) error {
	policyCreateResponse, err := d.api.ExportPolicyCreate(policy)
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
	size, err := d.api.VolumeSize(volumeName)
	return uint64(size), err
}

func (d OntapAPIZAPI) VolumeUsedSize(_ context.Context, volumeName string) (int, error) {
	return d.api.VolumeUsedSize(volumeName)
}

func (d OntapAPIZAPI) VolumeSetSize(ctx context.Context, name, newSize string) error {
	volumeSetSizeResponse, err := d.api.VolumeSetSize(name, newSize)

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

func (d OntapAPIZAPI) VolumeModifyUnixPermissions(
	ctx context.Context, volumeNameInternal, volumeNameExternal, unixPermissions string,
) error {
	modifyUnixPermResponse, err := d.api.VolumeModifyUnixPermissions(volumeNameInternal, unixPermissions)
	if err = GetError(ctx, modifyUnixPermResponse, err); err != nil {
		Logc(ctx).WithField("originalName", volumeNameExternal).Errorf(
			"Could not import volume, modifying unix permissions failed: %v", err)
		return fmt.Errorf("volume %s modify failed: %v", volumeNameExternal, err)
	}

	return nil
}

func (d OntapAPIZAPI) VolumeListByPrefix(ctx context.Context, prefix string) (Volumes, error) {
	// Get all volumes matching the storage prefix
	volumesResponse, err := d.api.VolumeGetAll(prefix)
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
			volumes = append(volumes, volumeInfo)
		}
	}

	return volumes, nil
}
func (d OntapAPIZAPI) VolumeListByAttrs(ctx context.Context, volumeAttrs *Volume) (Volumes, error) {
	aggrs := strings.Join(volumeAttrs.Aggregates, "|")
	response, err := d.api.VolumeListByAttrs(volumeAttrs.Name, aggrs, volumeAttrs.SpaceReserve,
		volumeAttrs.SnapshotPolicy, volumeAttrs.TieringPolicy, volumeAttrs.SnapshotDir, volumeAttrs.Encrypt,
		volumeAttrs.SnapshotReserve)
	if err = GetError(ctx, response, err); err != nil {
		return nil, err
	}

	volumes := Volumes{}
	if response.Result.AttributesListPtr != nil {
		for _, volume := range response.Result.AttributesListPtr.VolumeAttributesPtr {
			volumeInfo, err := VolumeInfoFromZapiAttrsHelper(&volume)
			if err != nil {
				return nil, err
			}
			volumes = append(volumes, volumeInfo)
		}
	}
	return volumes, nil
}

func (d OntapAPIZAPI) ExportRuleCreate(ctx context.Context, policyName, desiredPolicyRule string) error {
	ruleResponse, err := d.api.ExportRuleCreate(policyName, desiredPolicyRule,
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
	ruleResponse, err := d.api.ExportRuleDestroy(policyName, ruleIndex)
	if err = GetError(ctx, ruleResponse, err); err != nil {
		err = fmt.Errorf("error deleting export rule on policy %s at index %d; %v", policyName, ruleIndex, err)
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
	volumeModifyResponse, err := d.api.VolumeModifyExportPolicy(volumeName, policyName)
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
	policyGetResponse, err := d.api.ExportPolicyGet(policyName)
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

	ruleListResponse, err := d.api.ExportRuleGetIterRequest(policyName)
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

func (d OntapAPIZAPI) QtreeExists(ctx context.Context, name, volumePrefix string) (bool, string, error) {
	return d.api.QtreeExists(ctx, name, volumePrefix)
}

func (d OntapAPIZAPI) QtreeCreate(
	ctx context.Context, name, volumeName, unixPermissions, exportPolicy, securityStyle, qosPolicy string,
) error {
	response, err := d.api.QtreeCreate(name, volumeName, unixPermissions, exportPolicy, securityStyle, qosPolicy)
	return GetError(ctx, response, err)
}

func (d OntapAPIZAPI) QtreeDestroyAsync(ctx context.Context, path string, force bool) error {
	response, err := d.api.QtreeDestroyAsync(path, force)
	return GetError(ctx, response, err)
}

func (d OntapAPIZAPI) QtreeRename(ctx context.Context, path, newPath string) error {
	response, err := d.api.QtreeRename(path, newPath)
	return GetError(ctx, response, err)
}

func (d OntapAPIZAPI) QtreeModifyExportPolicy(ctx context.Context, name, volumeName, newExportPolicyName string) error {
	response, err := d.api.QtreeModifyExportPolicy(name, volumeName, newExportPolicyName)
	return GetError(ctx, response, err)
}

func (d OntapAPIZAPI) QtreeCount(ctx context.Context, volumeName string) (int, error) {
	return d.api.QtreeCount(ctx, volumeName)
}

func (d OntapAPIZAPI) QuotaEntryList(ctx context.Context, volumeName string) (QuotaEntries, error) {
	response, err := d.api.QuotaEntryList(volumeName)
	err = GetError(ctx, response, err)
	if err != nil {
		return nil, err
	}

	entries := QuotaEntries{}
	if response.Result.AttributesListPtr != nil {
		for _, rule := range response.Result.AttributesListPtr.QuotaEntryPtr {
			entry, err := d.convertQuota(ctx, rule)
			if err != nil {
				continue
			}
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func (d OntapAPIZAPI) QuotaSetEntry(ctx context.Context, qtreeName, volumeName, quotaType, diskLimit string) error {
	target := ""
	// Default quota for flexvol should have no limit and no target
	if quotaType == "tree" && diskLimit != "" {
		// For tree type quotas we must use the target and set the qtree to empty string
		target = fmt.Sprintf("/vol/%s/%s", volumeName, qtreeName)
		qtreeName = ""
	}

	response, err := d.api.QuotaSetEntry(qtreeName, volumeName, target, quotaType, diskLimit)
	if err = GetError(ctx, response, err); err != nil {
		msg := "error setting quota"
		Logc(ctx).WithError(err).Error(msg)
		return fmt.Errorf(msg)
	}
	return nil
}

func (d OntapAPIZAPI) QuotaStatus(ctx context.Context, volumeName string) (string, error) {
	statusResponse, err := d.api.QuotaStatus(volumeName)
	if err = GetError(ctx, statusResponse, err); err != nil {
		return "", fmt.Errorf("error getting quota status for Flexvol %s: %v", volumeName, err)
	}

	return statusResponse.Result.Status(), nil
}

func (d OntapAPIZAPI) QuotaOff(ctx context.Context, volumeName string) error {
	response, err := d.api.QuotaOff(volumeName)
	if err = GetError(ctx, response, err); err != nil {
		msg := "error disabling quota"
		Logc(ctx).WithError(err).WithField("volume", volumeName).Error(msg)
		return err
	}
	return nil
}

func (d OntapAPIZAPI) QuotaOn(ctx context.Context, volumeName string) error {
	response, err := d.api.QuotaOn(volumeName)
	if err = GetError(ctx, response, err); err != nil {
		msg := "error enabling quota"
		Logc(ctx).WithError(err).WithField("volume", volumeName).Error(msg)
		return err
	}
	return nil
}

func (d OntapAPIZAPI) QuotaResize(ctx context.Context, volumeName string) error {
	response, err := d.api.QuotaResize(volumeName)
	err = GetError(ctx, response, err)
	if zerr, ok := err.(ZapiError); ok {
		if zerr.Code() == azgo.EVOLUMEDOESNOTEXIST {
			return utils.NotFoundError(zerr.Error())
		}
	}
	return err
}

func (d OntapAPIZAPI) QtreeListByPrefix(ctx context.Context, prefix, volumePrefix string) (Qtrees, error) {
	listResponse, err := d.api.QtreeList(prefix, volumePrefix)
	if err = GetError(ctx, listResponse, err); err != nil {
		msg := fmt.Sprintf("Error listing qtrees. %v", err)
		Logc(ctx).Errorf(msg)
		return nil, fmt.Errorf(msg)
	}
	qtrees := Qtrees{}
	if listResponse.Result.AttributesListPtr != nil {
		for _, qtree := range listResponse.Result.AttributesListPtr.QtreeInfoPtr {
			qtrees = append(qtrees, d.convertQtree(qtree))
		}
	}
	return qtrees, nil
}

func (d OntapAPIZAPI) convertQtree(qtree azgo.QtreeInfoType) *Qtree {
	newQtree := &Qtree{}
	if qtree.QtreePtr != nil {
		newQtree.Name = qtree.Qtree()
	}
	if qtree.ExportPolicyPtr != nil {
		newQtree.ExportPolicy = qtree.ExportPolicy()
	}
	if qtree.SecurityStylePtr != nil {
		newQtree.SecurityStyle = qtree.SecurityStyle()
	}
	if qtree.VolumePtr != nil {
		newQtree.Volume = qtree.Volume()
	}
	if qtree.VserverPtr != nil {
		newQtree.Vserver = qtree.Vserver()
	}
	if qtree.ModePtr != nil {
		newQtree.UnixPermissions = qtree.Mode()
	}
	return newQtree
}

func (d OntapAPIZAPI) QtreeGetByName(ctx context.Context, name, volumePrefix string) (*Qtree, error) {
	qtree, err := d.api.QtreeGet(name, volumePrefix)
	if err != nil {
		msg := "error getting qtree"
		Logc(ctx).WithError(err).Error(msg)
		return nil, fmt.Errorf(msg)
	}
	return d.convertQtree(*qtree), nil
}

func (d OntapAPIZAPI) QuotaGetEntry(ctx context.Context, volumeName, qtreeName, quotaType string) (*QuotaEntry, error) {
	target := fmt.Sprintf("/vol/%s", volumeName)
	if qtreeName != "" {
		target += fmt.Sprintf("/%s", qtreeName)
	}

	quota, err := d.api.QuotaGetEntry(target, quotaType)
	if err != nil {
		Logc(ctx).WithError(err).Error("error getting quota rule")
	}

	return d.convertQuota(ctx, *quota)
}

func (d OntapAPIZAPI) convertQuota(ctx context.Context, quota azgo.QuotaEntryType) (*QuotaEntry, error) {
	var diskLimit int64
	var err error
	if quota.DiskLimitPtr != nil && quota.DiskLimit() != "-" && quota.DiskLimit() != "" {
		diskLimit, err = strconv.ParseInt(quota.DiskLimit(), 10, 64)
		if err != nil {
			msg := fmt.Sprintf("could not parse diskLimit %s", quota.DiskLimit())
			Logc(ctx).WithError(err).Error(msg)
			return nil, fmt.Errorf(msg)
		}
		diskLimit *= 1024 // Convert from KB to Bytes
	} else {
		// No limit
		diskLimit = -1
	}
	quotaEntry := &QuotaEntry{
		Target:         fmt.Sprintf("/vol/%s", quota.Volume()),
		DiskLimitBytes: diskLimit,
	}
	if quota.QuotaTargetPtr != nil && quota.QuotaTarget() != "" {
		quotaEntry.Target = quota.QuotaTarget()
	}
	return quotaEntry, nil
}

func (d OntapAPIZAPI) VolumeSnapshotCreate(ctx context.Context, snapshotName, sourceVolume string) error {
	snapResponse, err := d.api.SnapshotCreate(snapshotName, sourceVolume)
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
		Logc(ctx).WithField("volume", name).Warnf("Could not find volume after %3.2f seconds.",
			volumeBackoff.MaxElapsedTime.Seconds())
		return fmt.Errorf("volume %v does not exist", name)
	} else {
		Logc(ctx).WithField("volume", name).Debug("Volume found.")
		return nil
	}
}

func (d OntapAPIZAPI) VolumeCloneCreate(ctx context.Context, cloneName, sourceName, snapshot string, async bool) error {

	if async {
		cloneResponse, err := d.api.VolumeCloneCreateAsync(cloneName, sourceName, snapshot)
		if err != nil {
			msg := "error creating volume clone"
			Logc(ctx).WithError(err).Error(msg)
			return errors.New(msg)
		}
		err = d.api.WaitForAsyncResponse(ctx, cloneResponse, 120*time.Second)
		if err != nil {
			return errors.New("waiting for async response failed")
		}
	} else {
		cloneResponse, err := d.api.VolumeCloneCreate(cloneName, sourceName, snapshot)
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
	snapListResponse, err := d.api.SnapshotList(sourceVolume)
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
	qosResponse, err := d.api.VolumeSetQosPolicyGroupName(name, qos)
	if err = GetError(ctx, qosResponse, err); err != nil {
		return fmt.Errorf("error setting quality of service policy: %v", err)
	}
	return nil
}

func (d OntapAPIZAPI) VolumeCloneSplitStart(ctx context.Context, cloneName string) error {
	splitResponse, err := d.api.VolumeCloneSplitStart(cloneName)
	if err = GetError(ctx, splitResponse, err); err != nil {
		return fmt.Errorf("error splitting clone: %v", err)
	}

	return nil
}

func (d OntapAPIZAPI) SnapshotRestoreVolume(
	ctx context.Context, snapshotName, sourceVolume string,
) error {

	snapResponse, err := d.api.SnapshotRestoreVolume(snapshotName, sourceVolume)

	if err = GetError(ctx, snapResponse, err); err != nil {
		return fmt.Errorf("error restoring snapshot: %v", err)
	}
	return nil
}

func (d OntapAPIZAPI) SnapshotRestoreFlexgroup(ctx context.Context, snapshotName, sourceVolume string) error {
	return d.SnapshotRestoreVolume(ctx, snapshotName, sourceVolume)
}

func (d OntapAPIZAPI) VolumeSnapshotDelete(_ context.Context, snapshotName, sourceVolume string) error {
	snapResponse, err := d.api.SnapshotDelete(snapshotName, sourceVolume)

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

func (d OntapAPIZAPI) VolumeListBySnapshotParent(
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

func (d OntapAPIZAPI) SnapmirrorDeleteViaDestination(localFlexvolName, localSVMName string) error {
	snapDeleteResponse, err := d.api.SnapmirrorDeleteViaDestination(localFlexvolName, localSVMName)
	if err != nil && snapDeleteResponse != nil {
		if snapDeleteResponse.Result.ResultErrnoAttr == azgo.EOBJECTNOTFOUND {
			return NotFoundError(fmt.Sprintf("Error deleting snapmirror info for volume %v: %v", localFlexvolName, err))
		} else {
			return fmt.Errorf("error deleting snapmirror info for volume %v: %v", localFlexvolName, err)
		}
	}

	// Ensure no leftover snapmirror metadata
	err = d.api.SnapmirrorRelease(localFlexvolName, localSVMName)
	if err != nil {
		return fmt.Errorf("error releasing snapmirror info for volume %v: %v", localFlexvolName, err)
	}

	return nil
}

func (d OntapAPIZAPI) IsSVMDRCapable(ctx context.Context) (bool, error) {
	return d.api.IsVserverDRCapable(ctx)
}

func (d OntapAPIZAPI) GetSVMUUID() string {
	return d.api.SVMUUID()
}

func (d OntapAPIZAPI) SnapmirrorCreate(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName,
	remoteSVMName, replicationPolicy, replicationSchedule string) error {
	snapCreate, err := d.api.SnapmirrorCreate(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName,
		replicationPolicy, replicationSchedule)
	if err = GetError(ctx, snapCreate, err); err != nil {
		if zerr, ok := err.(ZapiError); !ok || zerr.Code() != azgo.EDUPLICATEENTRY {
			Logc(ctx).WithError(err).Error("Error on snapmirror create")
			return err
		}
	}
	return nil
}

func (d OntapAPIZAPI) SnapmirrorGet(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName,
	remoteSVMName string) (*Snapmirror, error) {
	snapmirrorResponse, err := d.api.SnapmirrorGet(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = GetError(ctx, snapmirrorResponse, err); err != nil {
		if zerr, ok := err.(ZapiError); ok {
			if zerr.Code() == azgo.EOBJECTNOTFOUND {
				return nil, NotFoundError(fmt.Sprintf("Error on snapmirror get"))
			}
			if !zerr.IsPassed() {
				Logc(ctx).WithError(err).Error("Error on snapmirror get")
				return nil, err
			}
		}
	}

	if snapmirrorResponse == nil {
		return nil, fmt.Errorf("unexpected error on snapmirror get")
	}

	if snapmirrorResponse.Result.AttributesPtr == nil {
		return nil, fmt.Errorf("error reading id attributes for snapmirror")
	}

	if snapmirrorResponse.Result.Attributes().SnapmirrorInfoPtr == nil {
		return nil, fmt.Errorf("error reading snapmirror info")
	}

	attributes := snapmirrorResponse.Result.Attributes()
	info := attributes.SnapmirrorInfo()

	if info.MirrorStatePtr == nil {
		return nil, fmt.Errorf("error reading snapmirror state")
	}

	if info.RelationshipStatusPtr == nil {
		return nil, fmt.Errorf("error reading snapmirror relationship status")
	}

	snapmirror := &Snapmirror{
		State:              SnapmirrorState(info.MirrorState()),
		RelationshipStatus: SnapmirrorStatus(info.RelationshipStatus()),
	}

	if info.LastTransferTypePtr != nil {
		snapmirror.LastTransferType = info.LastTransferType()
	}

	if info.IsHealthyPtr != nil {
		snapmirror.IsHealthy = info.IsHealthy()

		if info.UnhealthyReasonPtr != nil {
			snapmirror.UnhealthyReason = info.UnhealthyReason()
		}
	}

	return snapmirror, nil
}

func (d OntapAPIZAPI) SnapmirrorInitialize(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName,
	remoteSVMName string) error {
	_, err := d.api.SnapmirrorInitialize(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err != nil {
		if zerr, ok := err.(ZapiError); ok {
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

func (d OntapAPIZAPI) SnapmirrorDelete(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName,
	remoteSVMName string) error {
	snapDelete, deleteErr := d.api.SnapmirrorDelete(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if deleteErr = GetError(ctx, snapDelete, deleteErr); deleteErr != nil {
		Logc(ctx).WithError(deleteErr).Warn("Error on snapmirror delete")
	}
	return deleteErr
}

func (d OntapAPIZAPI) SnapmirrorResync(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName,
	remoteSVMName string) error {
	snapResync, err := d.api.SnapmirrorResync(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = GetError(ctx, snapResync, err); err != nil {
		if zerr, ok := err.(ZapiError); !ok || zerr.Code() != azgo.ETRANSFERINPROGRESS {
			Logc(ctx).WithError(err).Error("Error on snapmirror resync")
			// If we fail on the resync, we need to cleanup the snapmirror
			// it will be recreated in a future TMR reconcile loop through this function
			d.SnapmirrorDelete(ctx, localFlexvolName, localSVMName, remoteFlexvolName,
				remoteSVMName)
			return err
		}
	}
	return nil
}

func (d OntapAPIZAPI) SnapmirrorPolicyGet(ctx context.Context, replicationPolicy string) (*SnapmirrorPolicy, error) {
	snapmirrorPolicyResponse, err := d.api.SnapmirrorPolicyGet(ctx, replicationPolicy)
	if err != nil {
		return nil, err
	}

	if snapmirrorPolicyResponse == nil {
		return nil, fmt.Errorf("unexpected error on snapmirror policy get")
	}

	var snapmirrorPolicyTypeResponse SnapmirrorPolicyType

	if snapmirrorPolicyResponse.TypePtr != nil {
		snapmirrorPolicyTypeResponse = SnapmirrorPolicyType(snapmirrorPolicyResponse.Type())
	}

	snapmirrorPolicy := &SnapmirrorPolicy{
		Type: snapmirrorPolicyTypeResponse,
	}

	rules := map[string]struct{}{}

	if snapmirrorPolicyResponse.SnapmirrorPolicyRulesPtr != nil {
		rulesListResponse := snapmirrorPolicyResponse.SnapmirrorPolicyRules()

		if rulesListResponse.SnapmirrorPolicyRuleInfoPtr != nil {
			rulesInfoResponse := rulesListResponse.SnapmirrorPolicyRuleInfo()

			for _, rule := range rulesInfoResponse {
				if rule.SnapmirrorLabelPtr != nil {
					rules[rule.SnapmirrorLabel()] = struct{}{}
				}
			}
		}
	}

	snapmirrorPolicy.Rules = rules

	return snapmirrorPolicy, nil
}

func (d OntapAPIZAPI) SnapmirrorQuiesce(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName,
	remoteSVMName string) error {
	snapQuiesce, err := d.api.SnapmirrorQuiesce(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = GetError(ctx, snapQuiesce, err); err != nil {
		Logc(ctx).WithError(err).Error("Error on snapmirror quiesce")
		return err
	}
	return nil
}

func (d OntapAPIZAPI) SnapmirrorAbort(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName,
	remoteSVMName string) error {
	snapAbort, err := d.api.SnapmirrorAbort(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = GetError(ctx, snapAbort, err); err != nil {
		if zerr, ok := err.(ZapiError); !ok || zerr.Code() != azgo.ENOTRANSFERINPROGRESS {
			return SnapmirrorTransferInProgress(fmt.Sprintf("Snapmirror tranfer in progress"))
		}
	}

	return err
}

func (d OntapAPIZAPI) SnapmirrorBreak(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName,
	remoteSVMName string) error {
	snapBreak, err := d.api.SnapmirrorBreak(localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = GetError(ctx, snapBreak, err); err != nil {
		if zerr, ok := err.(ZapiError); !ok || zerr.Code() != azgo.EDEST_ISNOT_DP_VOLUME {
			Logc(ctx).WithError(err).Info("Error on snapmirror break")
			return err
		}
	}
	return nil
}

func (d OntapAPIZAPI) JobScheduleExists(ctx context.Context, replicationSchedule string) error {
	exists, err := d.api.JobScheduleExists(ctx, replicationSchedule)
	if err != nil {
		return fmt.Errorf("failed to list job schedules: %v", err)
	} else if !exists {
		return fmt.Errorf("specified replicationSchedule %v does not exist", replicationSchedule)
	}
	return nil
}

func (d OntapAPIZAPI) GetSVMPeers(ctx context.Context) ([]string, error) {
	return d.api.GetPeeredVservers(ctx)
}
