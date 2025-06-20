// Copyright 2025 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/capacity"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/pkg/convert"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/utils/errors"
)

func (d OntapAPIZAPI) SVMName() string {
	return d.api.SVMName()
}

func (d OntapAPIZAPI) IsSANOptimized() bool {
	// No SAN optimized version of ONTAP uses ZAPI
	return false
}

func (d OntapAPIZAPI) IsDisaggregated() bool {
	// No disaggregated version of ONTAP uses ZAPI
	return false
}

func (d OntapAPIZAPI) ValidateAPIVersion(ctx context.Context) error {
	// Make sure we're using a valid ONTAP version
	ontapVersion, err := d.APIVersion(ctx, true)
	if err != nil {
		return fmt.Errorf("could not determine Data ONTAP API version: %v", err)
	}

	if !d.SupportsFeature(ctx, MinimumONTAPIVersion) {
		return errors.New("ONTAP 9.5 or later is required")
	}

	Logc(ctx).WithField("Ontapi", ontapVersion).Debug("ONTAP API version.")

	return nil
}

func (d OntapAPIZAPI) VolumeCreate(ctx context.Context, volume Volume) error {
	fields := LogFields{
		"Method": "VolumeCreate",
		"Type":   "OntapAPIZAPI",
		"spec":   volume,
	}
	Logd(ctx, d.driverName,
		d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> VolumeCreate")
	defer Logd(ctx, d.driverName,
		d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< VolumeCreate")

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
	if err = azgo.GetError(ctx, volCreateResponse, err); err != nil {
		if zerr, ok := err.(azgo.ZapiError); ok {
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

func (d OntapAPIZAPI) VolumeDestroy(ctx context.Context, name string, force, skipRecoveryQueue bool) error {
	volDestroyResponse, err := d.api.VolumeDestroy(name, force)
	if err != nil {
		return fmt.Errorf("error destroying volume %v: %v", name, err)
	}

	if zerr := azgo.NewZapiError(volDestroyResponse); !zerr.IsPassed() {
		// It's not an error if the volume no longer exists
		if zerr.Code() == azgo.EVOLUMEDOESNOTEXIST {
			Logc(ctx).WithField("volume", name).Warn("Volume already deleted.")
			return nil
		}
		return fmt.Errorf("error destroying volume %v: %v", name, zerr)
	}

	if !skipRecoveryQueue {
		return nil
	}

	recoveryQueueVolumeName, err := d.VolumeRecoveryQueueGetName(ctx, name)
	if err != nil {
		Logc(ctx).WithField("volume", name).Warn("Volume not found in recovery queue.")
		return nil
	}
	_ = d.VolumeRecoveryQueuePurge(ctx, recoveryQueueVolumeName)

	return nil
}

func (d OntapAPIZAPI) VolumeRecoveryQueuePurge(ctx context.Context, recoveryQueueVolumeName string) error {
	volRecoveryQueuePurgeResponse, err := d.api.VolumeRecoveryQueuePurge(recoveryQueueVolumeName)
	if err != nil {
		Logc(ctx).WithField("volume", recoveryQueueVolumeName).WithError(err).Error(
			"Failed to purge volume from recovery queue.")
		return nil
	}

	if zerr := azgo.NewZapiError(volRecoveryQueuePurgeResponse); !zerr.IsPassed() {
		// It's not an error if the volume no longer exists
		if zerr.Code() == azgo.EOBJECTNOTFOUND {
			Logc(ctx).WithField("volume", recoveryQueueVolumeName).Warn("Volume already purged from the recovery queue.")
			return nil
		}
		if zerr.Code() == azgo.EVSERVER_OP_NOT_ALLOWED {
			Logc(ctx).WithField("volume", recoveryQueueVolumeName).Warn("Purging volume from recovery queue not allowed.")
			return nil
		}

		// Log the error and continue with the volume deletion, we will not reach this logic on retry,
		// this is best-effort
		Logc(ctx).WithField("volume", recoveryQueueVolumeName).WithError(zerr).Error(
			"Failed to purge volume from recovery queue.")
	}
	return nil
}

func (d OntapAPIZAPI) VolumeRecoveryQueueGetName(ctx context.Context, name string) (string, error) {
	volRecoveryQueueGetResponse, err := d.api.VolumeRecoveryQueueGetIter(fmt.Sprintf("%s*", name))
	if err != nil {
		Logc(ctx).WithField("volume", name).WithError(err).Error(
			"Failed to get volume from recovery queue.")
		return "", err
	}

	if zerr := azgo.NewZapiError(volRecoveryQueueGetResponse); !zerr.IsPassed() {
		// It's not an error if the volume no longer exists
		if zerr.Code() == azgo.EOBJECTNOTFOUND {
			Logc(ctx).WithField("volume", name).Warn("Volume already purged from the recovery queue.")
			return "", errors.NotFoundError("volume not found in recovery queue: %v", zerr)
		}

		Logc(ctx).WithField("volume", name).WithError(zerr).Error(
			"Failed to get volume from recovery queue.")
		return "", zerr
	}

	if volRecoveryQueueGetResponse.Result.AttributesListPtr == nil {
		return "", errors.NotFoundError("volume not found in recovery queue")
	}

	volRecoveryQueueInfo := volRecoveryQueueGetResponse.Result.AttributesListPtr.VolumeRecoveryQueueInfo()
	if len(volRecoveryQueueInfo) == 0 {
		return "", errors.NotFoundError("volume not found in recovery queue")
	}
	if len(volRecoveryQueueInfo) > 1 {
		return "", fmt.Errorf("multiple volumes found in recovery queue")
	}

	return volRecoveryQueueInfo[0].VolumeName(), nil
}

func (d OntapAPIZAPI) VolumeInfo(ctx context.Context, name string) (*Volume, error) {
	fields := LogFields{
		"Method": "VolumeInfo",
		"Type":   "OntapAPIZAPI",
		"name":   name,
	}
	Logd(ctx, d.driverName, d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> VolumeInfo")
	defer Logd(ctx, d.driverName,
		d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< VolumeInfo")

	// Get Flexvol by name
	volumeGetResponse, err := d.api.VolumeGet(name)

	Logc(ctx).WithFields(LogFields{
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
	var (
		responseAccessType           string
		responseAggregates           []string
		responseComment              string
		responseExportPolicy         string
		responseJunctionPath         string
		responseName                 string
		responseSize                 string
		responseSnapdirAccessEnabled *bool
		responseSnapshotPolicy       string
		responseSnapshotReserveInt   int
		responseSnapshotSpaceUsed    int
		responseSpaceReserve         string
		responseUnixPermissions      string
	)

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

		responseSnapdirAccessEnabled = volumeGetResponse.VolumeSnapshotAttributesPtr.SnapdirAccessEnabledPtr
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

func (d OntapAPIZAPI) APIVersion(ctx context.Context, cached bool) (string, error) {
	return d.api.SystemGetOntapiVersion(ctx, cached)
}

func (d OntapAPIZAPI) SupportsFeature(ctx context.Context, feature Feature) bool {
	return d.api.SupportsFeature(ctx, feature)
}

func (d OntapAPIZAPI) NodeListSerialNumbers(ctx context.Context) ([]string, error) {
	return d.api.NodeListSerialNumbers(ctx)
}

type OntapAPIZAPI struct {
	api        ZapiClientInterface
	driverName string
}

func (d OntapAPIZAPI) FcpInterfaceGet(ctx context.Context, svm string) ([]string, error) {
	var fcpInterfaces []string
	interfaceResponse, err := d.api.FcpInterfaceGetIterRequest()
	if err = azgo.GetError(ctx, interfaceResponse, err); err != nil {
		Logc(ctx).Debugf("could not get SVM FCP interfaces: %v", err)
		return nil, err
	}
	if interfaceResponse.Result.AttributesListPtr != nil {
		for _, fcpAttrs := range interfaceResponse.Result.AttributesListPtr.FcpInterfaceInfoPtr {
			fcpInterface := fmt.Sprintf("%s:%s", fcpAttrs.NodeName(), fcpAttrs.PortName())
			fcpInterfaces = append(fcpInterfaces, fcpInterface)
		}
	}
	if len(fcpInterfaces) == 0 {
		return nil, fmt.Errorf("SVM %s has no active FCP interfaces", svm)
	}
	return fcpInterfaces, nil
}

func (d OntapAPIZAPI) FcpNodeGetNameRequest(ctx context.Context) (string, error) {
	nodeNameResponse, err := d.api.FcpNodeGetNameRequest()
	if err != nil {
		return "", err
	}
	return nodeNameResponse.Result.NodeName(), nil
}

func (d OntapAPIZAPI) LunGetGeometry(ctx context.Context, lunPath string) (uint64, error) {
	// Check LUN geometry and verify LUN max size.
	var lunMaxSize uint64 = 0
	lunGeometry, err := d.api.LunGetGeometry(lunPath)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("LUN resize failed.")
		return lunMaxSize, fmt.Errorf("volume resize failed")
	}

	if lunGeometry != nil && lunGeometry.Result.MaxResizeSizePtr != nil {
		lunMaxSize = uint64(lunGeometry.Result.MaxResizeSize())
	}
	return lunMaxSize, nil
}

func (d OntapAPIZAPI) LunCloneCreate(
	ctx context.Context, flexvol, source, lunName string, qosPolicyGroup QosPolicyGroup,
) error {
	cloneResponse, err := d.api.LunCloneCreate(flexvol, source, lunName, qosPolicyGroup)
	if err != nil {
		return fmt.Errorf("error creating clone: %v", err)
	}
	if zerr := azgo.NewZapiError(cloneResponse); !zerr.IsPassed() {
		if zerr.Code() == azgo.EOBJECTNOTFOUND {
			return fmt.Errorf("snapshot does not exist in volume %s", source)
		} else if zerr.IsFailedToLoadJobError() {
			fields := LogFields{
				"zerr": zerr,
			}
			Logc(ctx).WithFields(fields).Warn(
				"Problem encountered during the clone create operation, " +
					"attempting to verify the clone was actually created",
			)
			if volumeLookupError := d.probeForVolume(ctx, lunName); volumeLookupError != nil {
				return volumeLookupError
			}
		} else {
			return fmt.Errorf("error creating clone: %v", zerr)
		}
	}
	return nil
}

func (d OntapAPIZAPI) LunList(ctx context.Context, pattern string) (Luns, error) {
	lunResponse, err := d.api.LunGetAll(pattern)
	if err = azgo.GetError(ctx, lunResponse, err); err != nil {
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
	} // TODO: For snapshot economy we currently return an error if AttributesListPtr is nil
	// "error snapshot attribute pointer nil"
	// Determine if that is an error case in general, OK to ignore
	// r needs to be returned only for SAN eco specifically

	return luns, nil
}

func (d OntapAPIZAPI) LunCreate(ctx context.Context, lun Lun) error {
	fields := LogFields{
		"Method": "LunCreate",
		"Type":   "OntapAPIZAPI",
		"spec":   lun,
	}
	Logd(ctx, d.driverName, d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> LunCreate")
	defer Logd(ctx, d.driverName,
		d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< LunCreate")

	sizeBytesStr, _ := capacity.ToBytes(lun.Size)
	sizeBytes, err := convert.ToPositiveInt(sizeBytesStr)
	if err != nil {
		return fmt.Errorf("%v is an invalid volume size: %v", sizeBytes, err)
	}

	lunCreateResponse, err := d.api.LunCreate(lun.Name, sizeBytes, lun.OsType, lun.Qos, *lun.SpaceReserved,
		*lun.SpaceAllocated)
	if err != nil {
		return fmt.Errorf("error creating LUN: %v", err)
	}

	if lunCreateResponse == nil {
		return fmt.Errorf("missing LUN create response")
	}

	if err = azgo.GetError(ctx, lunCreateResponse, err); err != nil {
		if zerr, ok := err.(azgo.ZapiError); ok {
			// Handle case where the Create is passed to every Docker Swarm node
			if zerr.Code() == azgo.EAPIERROR {
				if strings.HasSuffix(strings.TrimSpace(zerr.Reason()), "Job exists") {
					Logc(ctx).WithField("LUN", lun.Name).Warn("LUN create job already exists, " +
						"skipping LUN create on this node.")
					err = VolumeCreateJobExistsError(fmt.Sprintf("LUN create job already exists, %s", lun.Name))
				} else if strings.HasSuffix(strings.TrimSpace(zerr.Reason()), "Job exists") {
					// The originally chosen flexvol has hit the ONTAP hard limit of LUNs per flexvol.
					// This limit is model dependent therefore we must handle the error after-the-fact.
					// Add the full flexvol to the ignored list and find/create a new one
					Logc(ctx).WithError(err).Warn("ONTAP limit for LUNs/Flexvol reached; finding a new Flexvol")
					err = TooManyLunsError(fmt.Sprintf("ONTAP limit for LUNs/Flexvol reached; finding a new Flexvol, %s",
						lun.Name))
				}
			}
		}
	}

	return err
}

func (d OntapAPIZAPI) LunDestroy(ctx context.Context, lunPath string) error {
	offlineResponse, err := d.api.LunOffline(lunPath)
	if err != nil {
		fields := LogFields{"LUN": lunPath, "offlineResponse": offlineResponse}
		Logc(ctx).WithFields(fields).Errorf("Error LUN offline failed: %v", err)
		return err
	}
	_, err = d.api.LunDestroy(lunPath)
	return err
}

func (d OntapAPIZAPI) LunGetFSType(ctx context.Context, lunPath string) (string, error) {
	// Get the fstype from LUN Attribute
	LUNAttributeFSType := "com.netapp.ndvp.fstype"
	fstype, err := d.api.LunGetAttribute(ctx, lunPath, LUNAttributeFSType)
	if err != nil {
		// If not found, extract the fstype from LUN Comment
		comment, err := d.api.LunGetComment(ctx, lunPath)
		if err != nil {
			return "", err
		}

		// Parse the comment to get fstype value
		var lunComment map[string]map[string]string
		err = json.Unmarshal([]byte(comment), &lunComment)
		if err != nil {
			return "", err
		}
		lunAttrs := lunComment["lunAttributes"]
		if err == nil && lunAttrs != nil {
			fstype = lunAttrs["fstype"]
		} else {
			return "", fmt.Errorf("lunAttributes field not found in LUN comment")
		}
	}

	Logc(ctx).WithFields(LogFields{"LUN": lunPath, "fstype": fstype}).Debug("Found LUN attribute fstype.")
	return fstype, nil
}

func (d OntapAPIZAPI) LunGetAttribute(ctx context.Context, lunPath, attributeName string) (string, error) {
	attributeValue, err := d.api.LunGetAttribute(ctx, lunPath, attributeName)
	if err != nil {
		return "", fmt.Errorf("LUN attribute %s not found: %v", attributeName, err)
	}

	Logc(ctx).WithFields(LogFields{
		"LUN":         lunPath,
		attributeName: attributeValue,
	}).Debug("Found LUN attribute.")

	return attributeValue, nil
}

func (d OntapAPIZAPI) LunSetAttribute(
	ctx context.Context, lunPath, attribute, fstype, context, luks, formatOptions string,
) error {
	var attrResponse interface{}
	var err error

	if fstype != "" {
		attrResponse, err = d.api.LunSetAttribute(lunPath, attribute, fstype)
		if err = azgo.GetError(ctx, attrResponse, err); err != nil {
			return err
		}
	}

	if context != "" {
		attrResponse, err = d.api.LunSetAttribute(lunPath, "context", context)
		if err = azgo.GetError(ctx, attrResponse, err); err != nil {
			Logc(ctx).WithField("LUN", lunPath).Warning("Failed to save the driver context attribute for new LUN.")
		}
	}

	if luks != "" {
		attrResponse, err = d.api.LunSetAttribute(lunPath, "LUKS", luks)
		if err = azgo.GetError(ctx, attrResponse, err); err != nil {
			Logc(ctx).WithField("LUN", lunPath).Warning("Failed to save the LUKS attribute for new LUN.")
		}
	}

	// An example of how formatOption may look like:
	// "-E stride=256,stripe_width=16 -F -b 2435965"
	if formatOptions != "" {
		attrResponse, err = d.api.LunSetAttribute(lunPath, "formatOptions", formatOptions)
		if err = azgo.GetError(ctx, attrResponse, err); err != nil {
			Logc(ctx).WithField("LUN", lunPath).Warning("Failed to save the format options attribute for new LUN.")
			return fmt.Errorf("failed to save the formatOptions attribute for new LUN: %w", err)
		}
	}

	return nil
}

func (d OntapAPIZAPI) LunSetComment(
	_ context.Context, _, _ string,
) error {
	return errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) LunSetQosPolicyGroup(ctx context.Context, lunPath string, qosPolicyGroup QosPolicyGroup) error {
	qosResponse, err := d.api.LunSetQosPolicyGroup(lunPath, qosPolicyGroup)
	if err = azgo.GetError(ctx, qosResponse, err); err != nil {
		return err
	}
	return nil
}

func (d OntapAPIZAPI) LunGetByName(ctx context.Context, name string) (*Lun, error) {
	fields := LogFields{
		"Method":  "LunGetByName",
		"Type":    "OntapAPIZAPI",
		"LunPath": name,
	}
	Logd(ctx, d.driverName,
		d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> LunGetByName")
	defer Logd(ctx, d.driverName,
		d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< LunGetByName")

	lunResponse, err := d.api.LunGet(name)
	if err != nil {
		return nil, err
	}

	lun, err := lunInfoFromZapiAttrsHelper(*lunResponse)
	if err != nil {
		return nil, err
	}
	return lun, nil
}

func (d OntapAPIZAPI) LunExists(ctx context.Context, name string) (bool, error) {
	fields := LogFields{
		"Method":  "LunExists",
		"Type":    "OntapAPIZAPI",
		"LunPath": name,
	}

	Logd(ctx, d.driverName,
		d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> LunExists")
	defer Logd(ctx, d.driverName,
		d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< LunExists")

	lunResponse, err := d.api.LunGet(name)
	if err != nil {
		if errors.IsNotFoundError(err) {
			Logc(ctx).WithField("LUN", name).Debug("LUN does not exist.")
			return false, nil
		}

		return false, err
	}

	if lunResponse != nil {
		return true, nil
	}

	return false, nil
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
	var responseCreateTime string
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

	if lunResponse.CreationTimestampPtr != nil {
		responseCreateTime = time.Unix(int64(lunResponse.CreationTimestamp()), 0).UTC().Format(convert.TimestampFormat)
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
		SpaceReserved:  convert.ToPtr(responseSpaceReserved),
		SpaceAllocated: convert.ToPtr(responseSpaceAllocated),
		CreateTime:     responseCreateTime,
	}
	return lunInfo, nil
}

func (d OntapAPIZAPI) LunRename(ctx context.Context, lunPath, newLunPath string) error {
	renameResponse, err := d.api.LunRename(lunPath, newLunPath)
	if err = azgo.GetError(ctx, renameResponse, err); err != nil {
		Logc(ctx).WithField("originalName", lunPath).Errorf("renaming LUN failed: %v", err)
		return fmt.Errorf("LUN %s rename failed: %v", lunPath, err)
	}

	return nil
}

func (d OntapAPIZAPI) LunMapInfo(ctx context.Context, initiatorGroupName, lunPath string) (int, error) {
	lunID := -1
	lunMapResponse, err := d.api.LunMapListInfo(lunPath)
	if err = azgo.GetError(ctx, lunMapResponse, err); err != nil {
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

func (d OntapAPIZAPI) EnsureLunMapped(ctx context.Context, initiatorGroupName, lunPath string) (int, error) {
	return d.api.LunMapIfNotMapped(ctx, initiatorGroupName, lunPath)
}

func (d OntapAPIZAPI) LunSize(ctx context.Context, lunPath string) (int, error) {
	// lunPath is either an actual LUN name or a path in the form of /vol/<flexvol>/<lun>.
	// The caller is responsible for ensuring that the path is in the correct format.
	size, err := d.api.LunSize(lunPath)
	if err != nil {
		return 0, err
	}
	return size, nil
}

func (d OntapAPIZAPI) LunSetSize(ctx context.Context, lunPath, newSize string) (uint64, error) {
	sizeBytesStr, err := capacity.ToBytes(newSize)
	if err != nil {
		return 0, err
	}
	sizeBytes, err := convert.ToPositiveInt(sizeBytesStr)
	if err != nil {
		return 0, err
	}
	return d.api.LunResize(lunPath, sizeBytes)
}

// LunListIgroupsMapped returns a list of igroups the LUN is currently mapped to.
func (d OntapAPIZAPI) LunListIgroupsMapped(ctx context.Context, lunPath string) ([]string, error) {
	var results []string
	lunMapGetResponse, err := d.api.LunMapsGetByLun(lunPath)
	if err != nil {
		msg := "error getting LUN maps"
		Logc(ctx).WithError(err).Error(msg)
		return nil, fmt.Errorf(msg)
	}
	attributesList := lunMapGetResponse.Result.AttributesList()
	for _, lunMapInfo := range attributesList.LunMapInfo() {
		results = append(results, lunMapInfo.InitiatorGroup())
	}

	return results, nil
}

// IgroupListLUNsMapped returns a list of LUNs currently mapped to the given Igroup.
func (d OntapAPIZAPI) IgroupListLUNsMapped(ctx context.Context, initiatorGroupName string) ([]string, error) {
	var results []string
	lunMapGetResponse, err := d.api.LunMapsGetByIgroup(initiatorGroupName)
	if err != nil {
		msg := "error getting LUN maps"
		Logc(ctx).WithError(err).Error(msg)
		return nil, fmt.Errorf(msg)
	}
	attributesList := lunMapGetResponse.Result.AttributesList()
	for _, lunMapInfo := range attributesList.LunMapInfo() {
		results = append(results, lunMapInfo.Path())
	}

	return results, nil
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
		for _, lunMapInfo := range lunMapGetResponse.Result.AttributesListPtr.LunMapInfo() {
			if lunMapInfo.ReportingNodesPtr != nil {
				for _, reportingNode := range lunMapInfo.ReportingNodesPtr.NodeName() {
					results = append(results, reportingNode)
				}
			}
		}
	}

	return results, nil
}

func (d OntapAPIZAPI) LunUnmap(ctx context.Context, initiatorGroupName, lunPath string) error {
	fields := LogFields{
		"LUN":    lunPath,
		"igroup": initiatorGroupName,
	}
	Logd(ctx, d.driverName,
		d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Debug(">>>> LunUnmap.")
	defer Logd(ctx, d.driverName,
		d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< LunUnmap.")

	apiResponse, err := d.api.LunUnmap(initiatorGroupName, lunPath)
	err = azgo.GetError(ctx, apiResponse, err)
	if zerr := azgo.NewZapiError(apiResponse); zerr.Code() == azgo.EVDISK_ERROR_NO_SUCH_LUNMAP {
		// IGroup is not mapped to LUN, return success.
		Logc(ctx).WithFields(fields).Debug("Igroup is not mapped to LUN; returning success.")
		return nil
	}
	if err != nil {
		msg := "error unmapping LUN"
		Logc(ctx).WithFields(fields).WithError(err).Error(msg)
		return fmt.Errorf(msg)
	}
	return nil
}

func (d OntapAPIZAPI) IscsiInitiatorGetDefaultAuth(ctx context.Context) (IscsiInitiatorAuth, error) {
	authInfo := IscsiInitiatorAuth{}
	apiResponse, err := d.api.IscsiInitiatorGetDefaultAuth()
	err = azgo.GetError(ctx, apiResponse, err)
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
	err = azgo.GetError(ctx, response, err)
	if err != nil {
		return err
	}
	return nil
}

func (d OntapAPIZAPI) IscsiInterfaceGet(ctx context.Context, svm string) ([]string, error) {
	var iSCSIInterfaces []string
	interfaceResponse, err := d.api.IscsiInterfaceGetIterRequest()
	err = azgo.GetError(ctx, interfaceResponse, err)
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
	err = azgo.GetError(ctx, response, err)
	zerr, zerrOK := err.(azgo.ZapiError)
	if err == nil || (zerrOK && zerr.Code() == azgo.EVDISK_ERROR_NO_SUCH_INITGROUP) {
		Logc(ctx).WithField("igroup", initiatorGroupName).Debug("No such initiator group (igroup).")
	} else if zerrOK && zerr.Code() == azgo.EVDISK_ERROR_INITGROUP_EXISTS {
		Logc(ctx).WithField("igroup", initiatorGroupName).Info("Initiator group (igroup) already exists.")
	} else {
		Logc(ctx).WithFields(LogFields{
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
	err = azgo.GetError(ctx, response, err)
	zerr, zerrOK := err.(azgo.ZapiError)
	if err == nil || (zerrOK && zerr.Code() == azgo.EVDISK_ERROR_NO_SUCH_INITGROUP) {
		Logc(ctx).WithField("igroup", initiatorGroupName).Debug("No such initiator group (igroup).")
	} else if zerrOK && zerr.Code() == azgo.EVDISK_ERROR_INITGROUP_MAPS_EXIST {
		Logc(ctx).WithField("igroup", initiatorGroupName).Info("Initiator group (igroup) currently in use.")
	} else {
		Logc(ctx).WithFields(LogFields{
			"igroup": initiatorGroupName,
			"error":  err.Error(),
		}).Error("Initiator group (igroup) could not be deleted.")
		return err
	}
	return nil
}

func (d OntapAPIZAPI) EnsureIgroupAdded(ctx context.Context, initiatorGroupName, initiator string) error {
	response, err := d.api.IgroupAdd(initiatorGroupName, initiator)
	err = azgo.GetError(ctx, response, err)
	zerr, zerrOK := err.(azgo.ZapiError)
	if err == nil || (zerrOK && zerr.Code() == azgo.EVDISK_ERROR_INITGROUP_HAS_NODE) {
		Logc(ctx).WithFields(LogFields{
			"IQN":    initiator,
			"igroup": initiatorGroupName,
		}).Debug("Host IQN already in igroup.")
	} else {
		return fmt.Errorf("error adding IQN %v to igroup %v: %v", initiator, initiatorGroupName, err)
	}
	return nil
}

func (d OntapAPIZAPI) IgroupList(ctx context.Context) ([]string, error) {
	response, err := d.api.IgroupList()
	err = azgo.GetError(ctx, response, err)
	if err != nil {
		return nil, fmt.Errorf("error listing igroups: %v", err)
	}
	if response.Result.NumRecords() == 0 {
		return nil, nil
	}
	igroups := make([]string, 0, response.Result.NumRecords())
	responseIgroups := response.Result.AttributesList().InitiatorGroupInfoPtr
	for _, i := range responseIgroups {
		igroups = append(igroups, i.InitiatorGroupName())
	}

	return igroups, nil
}

func (d OntapAPIZAPI) IgroupRemove(ctx context.Context, initiatorGroupName, initiator string, force bool) error {
	response, err := d.api.IgroupRemove(initiatorGroupName, initiator, force)
	err = azgo.GetError(ctx, response, err)
	zerr, zerrOK := err.(azgo.ZapiError)
	if err == nil || (zerrOK && zerr.Code() == azgo.EVDISK_ERROR_NODE_NOT_IN_INITGROUP) {
		Logc(ctx).WithFields(LogFields{
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

// GetSLMDataLifs returns IP addresses whose node name matches reporting node names
func (d OntapAPIZAPI) GetSLMDataLifs(ctx context.Context, ips, reportingNodeNames []string) ([]string, error) {
	var reportedDataLIFs []string

	if len(ips) == 0 || len(reportingNodeNames) == 0 {
		return nil, nil
	}

	response, err := d.api.NetInterfaceGet()
	if err != nil {
		return nil, fmt.Errorf("error checking network interfaces: %v", err)
	}

	if response == nil {
		Logc(ctx).Debug("Net interface returned a empty response.")
		return nil, nil
	}

	if response.Result.AttributesListPtr != nil && response.Result.AttributesListPtr.
		NetInterfaceInfoPtr != nil {
		for _, netInterface := range response.Result.AttributesListPtr.NetInterfaceInfo() {
			if netInterface.CurrentNodePtr != nil && netInterface.AddressPtr != nil {
				nodeName := netInterface.CurrentNode()
				ipAddress := netInterface.Address()

				if nodeName != "" && ipAddress != "" {
					if collection.ContainsString(ips, ipAddress) &&
						collection.ContainsString(reportingNodeNames, nodeName) {
						reportedDataLIFs = append(reportedDataLIFs, ipAddress)
					}
				}
			}
		}
	}

	return reportedDataLIFs, nil
}

func NewOntapAPIZAPI(zapiClient *Client) (OntapAPIZAPI, error) {
	result := OntapAPIZAPI{
		api: zapiClient,
	}
	return result, nil
}

// NewOntapAPIZAPIFromZapiClientInterface added for testing
func NewOntapAPIZAPIFromZapiClientInterface(zapiClient ZapiClientInterface) (OntapAPIZAPI, error) {
	result := OntapAPIZAPI{
		api: zapiClient,
	}
	return result, nil
}

// NewZAPIClient is a factory method for creating a new instance
func NewZAPIClient(config ClientConfig, SVM, driverName string) *Client {
	d := &Client{
		driverName: driverName,
		config:     config,
		zr: azgo.NewZapiRunner(
			config.ManagementLIF,
			SVM,
			config.Username,
			config.Password,
			config.ClientPrivateKey,
			config.ClientCertificate,
			config.TrustedCACertificate,
			true,
			"",
			config.DebugTraceFlags,
		),
		m: &sync.Mutex{},
	}
	return d
}

func NewZAPIClientFromOntapConfig(
	ctx context.Context, ontapConfig *drivers.OntapStorageDriverConfig, numRecords int,
) (OntapAPI, error) {
	client := NewZAPIClient(ClientConfig{
		ManagementLIF:           ontapConfig.ManagementLIF,
		Username:                ontapConfig.Username,
		Password:                ontapConfig.Password,
		ClientCertificate:       ontapConfig.ClientCertificate,
		ClientPrivateKey:        ontapConfig.ClientPrivateKey,
		ContextBasedZapiRecords: numRecords,
		TrustedCACertificate:    ontapConfig.TrustedCACertificate,
		DebugTraceFlags:         ontapConfig.DebugTraceFlags,
	}, ontapConfig.SVM, ontapConfig.StorageDriverName)

	if ontapConfig.SVM != "" {

		// Try the SVM as given
		vserverResponse, err := client.VserverGetRequest()
		if err = azgo.GetError(ctx, vserverResponse, err); err != nil {
			return nil, fmt.Errorf("error reading SVM details: %v", err)
		}

		// Update everything to use our derived SVM
		ontapConfig.SVM = vserverResponse.Result.AttributesPtr.VserverInfoPtr.VserverName()
		svmUUID := vserverResponse.Result.AttributesPtr.VserverInfoPtr.Uuid()

		// Detect MCC
		var svmSubtype string
		if vserverResponse.Result.AttributesPtr.VserverInfoPtr.VserverSubtypePtr != nil {
			svmSubtype = vserverResponse.Result.AttributesPtr.VserverInfoPtr.VserverSubtype()
		}
		mcc := svmSubtype == SVMSubtypeSyncSource || svmSubtype == SVMSubtypeSyncDestination

		// Create a new client based on the SVM we discovered
		client = NewZAPIClient(ClientConfig{
			ManagementLIF:           ontapConfig.ManagementLIF,
			Username:                ontapConfig.Username,
			Password:                ontapConfig.Password,
			ClientCertificate:       ontapConfig.ClientCertificate,
			ClientPrivateKey:        ontapConfig.ClientPrivateKey,
			ContextBasedZapiRecords: numRecords,
			TrustedCACertificate:    ontapConfig.TrustedCACertificate,
			DebugTraceFlags:         ontapConfig.DebugTraceFlags,
		}, ontapConfig.SVM, ontapConfig.StorageDriverName)
		client.SetSVMUUID(svmUUID)
		client.SetSVMMCC(mcc)

		Logc(ctx).WithFields(LogFields{
			"SVM":  ontapConfig.SVM,
			"UUID": client.SVMUUID(),
			"MCC":  client.SVMMCC(),
		}).Debug("Using specified SVM.")

	} else {

		// Use VserverGetIterRequest to populate config.SVM if it wasn't specified and we can derive it
		vserverResponse, err := client.VserverGetIterRequest()
		if err = azgo.GetError(ctx, vserverResponse, err); err != nil {
			return nil, fmt.Errorf("error enumerating SVMs: %v", err)
		}

		if vserverResponse.Result.NumRecords() != 1 {
			return nil, errors.New("cannot derive SVM to use; please specify SVM in config file")
		}

		// Update everything to use our derived SVM
		ontapConfig.SVM = vserverResponse.Result.AttributesListPtr.VserverInfoPtr[0].VserverName()
		svmUUID := vserverResponse.Result.AttributesListPtr.VserverInfoPtr[0].Uuid()

		// Detect MCC
		var svmSubtype string
		if vserverResponse.Result.AttributesListPtr.VserverInfoPtr[0].VserverSubtypePtr != nil {
			svmSubtype = vserverResponse.Result.AttributesListPtr.VserverInfoPtr[0].VserverSubtype()
		}
		mcc := svmSubtype == SVMSubtypeSyncSource || svmSubtype == SVMSubtypeSyncDestination

		// Create a new client based on the SVM we discovered
		client = NewZAPIClient(ClientConfig{
			ManagementLIF:           ontapConfig.ManagementLIF,
			Username:                ontapConfig.Username,
			Password:                ontapConfig.Password,
			ClientCertificate:       ontapConfig.ClientCertificate,
			ClientPrivateKey:        ontapConfig.ClientPrivateKey,
			ContextBasedZapiRecords: numRecords,
			TrustedCACertificate:    ontapConfig.TrustedCACertificate,
			DebugTraceFlags:         ontapConfig.DebugTraceFlags,
		}, ontapConfig.SVM, ontapConfig.StorageDriverName)
		client.SetSVMUUID(svmUUID)
		client.SetSVMMCC(mcc)

		Logc(ctx).WithFields(LogFields{
			"SVM":  ontapConfig.SVM,
			"UUID": client.SVMUUID(),
			"MCC":  client.SVMMCC(),
		}).Debug("Using derived SVM.")
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

func (d OntapAPIZAPI) NetFcpInterfaceGetDataLIFs(ctx context.Context, protocol string) ([]string, error) {
	return d.api.NetFcpInterfaceGetDataLIFs(ctx, protocol)
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

	if err = azgo.GetError(ctx, emsResponse, err); err != nil {
		Logc(ctx).WithFields(LogFields{
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
	fields := LogFields{
		"Method": "FlexgroupCreate",
		"Type":   "OntapAPIZAPI",
		"spec":   volume,
	}
	Logd(ctx, d.driverName,
		d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> FlexgroupCreate")
	defer Logd(ctx, d.driverName,
		d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< FlexgroupCreate")

	sizeBytes, err := convert.ToPositiveInt(volume.Size)
	if err != nil {
		return fmt.Errorf("%v is an invalid volume size: %v", volume.Size, err)
	}

	flexgroupCreateResponse, err := d.api.FlexGroupCreate(ctx, volume.Name, sizeBytes, volume.Aggregates,
		volume.SpaceReserve, volume.SnapshotPolicy, volume.UnixPermissions, volume.ExportPolicy, volume.SecurityStyle,
		volume.TieringPolicy, volume.Comment, volume.Qos, volume.Encrypt, volume.SnapshotReserve)
	if err != nil {
		return fmt.Errorf("error creating volume: %v", err)
	}

	if flexgroupCreateResponse == nil {
		return fmt.Errorf("missing volume create response")
	}

	if err = azgo.GetError(ctx, flexgroupCreateResponse, err); err != nil {
		if zerr, ok := err.(azgo.ZapiError); ok {
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

func (d OntapAPIZAPI) FlexgroupModifySnapshotDirectoryAccess(ctx context.Context, volumeName string, enable bool) error {
	snapDirResponse, err := d.api.FlexGroupVolumeModifySnapshotDirectoryAccess(ctx, volumeName, enable)
	if err = azgo.GetError(ctx, snapDirResponse, err); err != nil {
		return fmt.Errorf("error modifying snapshot directory access: %v", err)
	}

	return nil
}

func (d OntapAPIZAPI) FlexgroupInfo(ctx context.Context, volumeName string) (*Volume, error) {
	fields := LogFields{
		"Method": "FlexgroupInfo",
		"Type":   "OntapAPIZAPI",
		"name":   volumeName,
	}
	Logd(ctx, d.driverName,
		d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> FlexgroupInfo")
	defer Logd(ctx, d.driverName,
		d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< FlexgroupInfo")

	// Get Flexvol by name
	flexGroupGetResponse, err := d.api.FlexGroupGet(volumeName)

	Logc(ctx).WithFields(LogFields{
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
	if err = azgo.GetError(ctx, modifyCommentResponse, err); err != nil {
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
	if err = azgo.GetError(ctx, modifyUnixPermResponse, err); err != nil {
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

func (d OntapAPIZAPI) FlexgroupDestroy(ctx context.Context, volumeName string, force, skipRecoveryQueue bool) error {
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

	if zerr := azgo.NewZapiError(offlineResp); !zerr.IsPassed() {
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

	if zerr := azgo.NewZapiError(volDestroyResponse); !zerr.IsPassed() {
		// It's not an error if the volume no longer exists
		if zerr.Code() == azgo.EVOLUMEDOESNOTEXIST {
			Logc(ctx).WithField("volume", volumeName).Warn("Volume already deleted.")
		} else {
			return fmt.Errorf("error destroying volume %v: %v", volumeName, zerr)
		}
	}

	if !skipRecoveryQueue {
		return nil
	}

	recoveryQueueVolumeName, err := d.VolumeRecoveryQueueGetName(ctx, volumeName)
	if err != nil {
		Logc(ctx).WithField("volume", volumeName).Warn("Volume not found in recovery queue.")
		return nil
	}

	_ = d.VolumeRecoveryQueuePurge(ctx, recoveryQueueVolumeName)

	return nil
}

func (d OntapAPIZAPI) FlexgroupListByPrefix(ctx context.Context, prefix string) (Volumes, error) {
	// Get all volumes matching the storage prefix
	volumesResponse, err := d.api.FlexGroupGetAll(prefix)
	if err = azgo.GetError(ctx, volumesResponse, err); err != nil {
		return nil, err
	}

	volumes := Volumes{}

	// Convert all volumes to VolumeExternal and write them to the channel
	if volumesResponse.Result.AttributesListPtr != nil {
		for idx := range volumesResponse.Result.AttributesListPtr.VolumeAttributesPtr {
			volumeInfo, err := VolumeInfoFromZapiAttrsHelper(&volumesResponse.Result.AttributesListPtr.VolumeAttributesPtr[idx])
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

	if zerr := azgo.NewZapiError(umountResp); !zerr.IsPassed() {
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

	if zerr := azgo.NewZapiError(result.Result); !zerr.IsPassed() {
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
	if err = azgo.GetError(ctx, response, err); err != nil {
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

func hasZapiAggrSpaceInformation(ctx context.Context, aggrSpace azgo.SpaceInformationType) bool {
	if aggrSpace.AggregatePtr == nil {
		return false
	}
	if aggrSpace.AggregateSizePtr == nil {
		return false
	}
	if aggrSpace.VolumeFootprintsPtr == nil {
		return false
	}
	if aggrSpace.VolumeFootprintsPercentPtr == nil {
		return false
	}
	if aggrSpace.UsedIncludingSnapshotReservePtr == nil {
		return false
	}
	if aggrSpace.UsedIncludingSnapshotReservePercentPtr == nil {
		return false
	}
	return true
}

func (d OntapAPIZAPI) GetSVMAggregateSpace(ctx context.Context, aggregate string) ([]SVMAggregateSpace, error) {
	// lookup aggregate
	aggrSpaceResponse, aggrSpaceErr := d.api.AggrSpaceGetIterRequest(aggregate)
	if aggrSpaceErr != nil {
		return nil, aggrSpaceErr
	}

	if aggrSpaceResponse == nil {
		return nil, errors.New("could not determine aggregate space, cannot check aggregate provisioning limits for " + aggregate)
	}

	var svmAggregateSpaceList []SVMAggregateSpace

	// iterate over results
	if aggrSpaceResponse.Result.AttributesListPtr != nil {
		for _, aggrSpace := range aggrSpaceResponse.Result.AttributesListPtr.SpaceInformationPtr {

			if !hasZapiAggrSpaceInformation(ctx, aggrSpace) {
				Logc(ctx).Debugf("Skipping entry with missing aggregate space information")
				continue
			}

			aggrName := aggrSpace.Aggregate()
			if aggregate != aggrName {
				Logc(ctx).Debugf("Skipping " + aggrName)
				continue
			}

			Logc(ctx).WithFields(LogFields{
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

func (d OntapAPIZAPI) VolumeModifySnapshotDirectoryAccess(ctx context.Context, name string, enable bool) error {
	snapDirResponse, err := d.api.VolumeModifySnapshotDirectoryAccess(name, enable)
	if err = azgo.GetError(ctx, snapDirResponse, err); err != nil {
		return fmt.Errorf("error modifying snapshot directory access: %v", err)
	}

	return nil
}

func (d OntapAPIZAPI) VolumeMount(ctx context.Context, name, junctionPath string) error {
	mountResponse, err := d.api.VolumeMount(name, junctionPath)
	if err = azgo.GetError(ctx, mountResponse, err); err != nil {
		if zerr, ok := err.(azgo.ZapiError); ok {
			if zerr.Code() == azgo.EAPIERROR {
				return ApiError(fmt.Sprintf("%v", err))
			}
		}
		return fmt.Errorf("error mounting volume to junction: %v", err)
	}

	return nil
}

func (d OntapAPIZAPI) VolumeRename(ctx context.Context, originalName, newName string) error {
	renameResponse, err := d.api.VolumeRename(originalName, newName)
	if err = azgo.GetError(ctx, renameResponse, err); err != nil {
		Logc(ctx).WithField("originalName", originalName).Errorf("renaming volume failed: %v", err)
		return fmt.Errorf("volume %s rename failed: %v", originalName, err)
	}

	return nil
}

func (d OntapAPIZAPI) VolumeSetComment(
	ctx context.Context, volumeNameInternal, volumeNameExternal, comment string,
) error {
	modifyCommentResponse, err := d.api.VolumeSetComment(ctx, volumeNameInternal, comment)
	if err = azgo.GetError(ctx, modifyCommentResponse, err); err != nil {
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
	if zerr := azgo.NewZapiError(policyCreateResponse); !zerr.IsPassed() {
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

	if err = azgo.GetError(ctx, volumeSetSizeResponse.Result, err); err != nil {
		Logc(ctx).WithField("error", err).Error("Volume resize failed.")
		return fmt.Errorf("volume resize failed")
	}

	return nil
}

func (d OntapAPIZAPI) VolumeModifyUnixPermissions(
	ctx context.Context, volumeNameInternal, volumeNameExternal, unixPermissions string,
) error {
	modifyUnixPermResponse, err := d.api.VolumeModifyUnixPermissions(volumeNameInternal, unixPermissions)
	if err = azgo.GetError(ctx, modifyUnixPermResponse, err); err != nil {
		Logc(ctx).WithField("originalName", volumeNameExternal).Errorf(
			"Could not import volume, modifying unix permissions failed: %v", err)
		return fmt.Errorf("volume %s modify failed: %v", volumeNameExternal, err)
	}

	return nil
}

func (d OntapAPIZAPI) VolumeListByPrefix(ctx context.Context, prefix string) (Volumes, error) {
	// Get all volumes matching the storage prefix
	volumesResponse, err := d.api.VolumeGetAll(prefix)
	if err = azgo.GetError(ctx, volumesResponse, err); err != nil {
		return nil, err
	}

	volumes := Volumes{}

	// Convert all volumes to VolumeExternal and write them to the channel
	if volumesResponse.Result.AttributesListPtr != nil {
		for idx := range volumesResponse.Result.AttributesListPtr.VolumeAttributesPtr {
			volumeInfo, err := VolumeInfoFromZapiAttrsHelper(&volumesResponse.Result.AttributesListPtr.VolumeAttributesPtr[idx])
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
	if err = azgo.GetError(ctx, response, err); err != nil {
		return nil, err
	}

	volumes := Volumes{}
	if response.Result.AttributesListPtr != nil {
		for idx := range response.Result.AttributesListPtr.VolumeAttributesPtr {
			volumeInfo, err := VolumeInfoFromZapiAttrsHelper(&response.Result.AttributesListPtr.VolumeAttributesPtr[idx])
			if err != nil {
				return nil, err
			}
			volumes = append(volumes, volumeInfo)
		}
	}
	return volumes, nil
}

func (d OntapAPIZAPI) ExportRuleCreate(ctx context.Context, policyName, desiredPolicyRule, nasProtocol string) error {
	var ruleResponse *azgo.ExportRuleCreateResponse
	var err error
	if nasProtocol == sa.SMB {
		ruleResponse, err = d.api.ExportRuleCreate(policyName, desiredPolicyRule,
			[]string{"cifs"}, []string{"any"}, []string{"any"}, []string{"any"})
	} else {
		ruleResponse, err = d.api.ExportRuleCreate(policyName, desiredPolicyRule,
			[]string{"nfs"}, []string{"any"}, []string{"any"}, []string{"any"})
	}
	if zerr := azgo.GetError(ctx, ruleResponse, err); zerr != nil {
		apiStatus, message, code := ExtractError(zerr)
		if apiStatus == "failed" && code == azgo.EAPIERROR && strings.Contains(message, "policy already contains a rule") {
			return errors.AlreadyExistsError(message)
		}

		err = fmt.Errorf("error creating export rule: %v", zerr)
		Logc(ctx).WithFields(LogFields{
			"ExportPolicy": policyName,
			"ClientMatch":  desiredPolicyRule,
		}).Error(err)
		return err
	}

	if ruleResponse == nil {
		return fmt.Errorf("unexpected response")
	}

	return nil
}

func (d OntapAPIZAPI) ExportRuleDestroy(ctx context.Context, policyName string, ruleIndex int) error {
	ruleResponse, err := d.api.ExportRuleDestroy(policyName, ruleIndex)
	if err = azgo.GetError(ctx, ruleResponse, err); err != nil {
		err = fmt.Errorf("error deleting export rule on policy %s at index %d; %v", policyName, ruleIndex, err)
		Logc(ctx).WithFields(LogFields{
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
	if err = azgo.GetError(ctx, volumeModifyResponse, err); err != nil {
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
	if zerr := azgo.NewZapiError(policyGetResponse); !zerr.IsPassed() {
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

func (d OntapAPIZAPI) ExportRuleList(ctx context.Context, policyName string) (map[int]string, error) {
	ruleListResponse, err := d.api.ExportRuleGetIterRequest(policyName)
	if err = azgo.GetError(ctx, ruleListResponse, err); err != nil {
		return nil, fmt.Errorf("error listing export policy rules: %v", err)
	}
	rules := make(map[int]string)

	if ruleListResponse.Result.NumRecords() > 0 {
		rulesAttrList := ruleListResponse.Result.AttributesList()
		exportRuleList := rulesAttrList.ExportRuleInfo()
		for _, rule := range exportRuleList {
			rules[rule.RuleIndex()] = rule.ClientMatch()
		}
	}

	return rules, nil
}

func (d OntapAPIZAPI) QtreeExists(ctx context.Context, name, volumePattern string) (bool, string, error) {
	return d.api.QtreeExists(ctx, name, volumePattern)
}

func (d OntapAPIZAPI) QtreeCreate(
	ctx context.Context, name, volumeName, unixPermissions, exportPolicy, securityStyle, qosPolicy string,
) error {
	response, err := d.api.QtreeCreate(name, volumeName, unixPermissions, exportPolicy, securityStyle, qosPolicy)
	return azgo.GetError(ctx, response, err)
}

func (d OntapAPIZAPI) QtreeDestroyAsync(ctx context.Context, path string, force bool) error {
	response, err := d.api.QtreeDestroyAsync(path, force)
	return azgo.GetError(ctx, response, err)
}

func (d OntapAPIZAPI) QtreeRename(ctx context.Context, path, newPath string) error {
	response, err := d.api.QtreeRename(path, newPath)
	return azgo.GetError(ctx, response, err)
}

func (d OntapAPIZAPI) QtreeModifyExportPolicy(ctx context.Context, name, volumeName, newExportPolicyName string) error {
	response, err := d.api.QtreeModifyExportPolicy(name, volumeName, newExportPolicyName)
	if zerr := azgo.GetError(ctx, *response, err); zerr != nil {
		apiError, message, code := ExtractError(zerr)
		if apiError == "failed" && code == azgo.EAPIERROR {
			return errors.NotFoundError(message)
		}

		return zerr
	}
	return nil
}

func (d OntapAPIZAPI) QtreeCount(ctx context.Context, volumeName string) (int, error) {
	return d.api.QtreeCount(ctx, volumeName)
}

func (d OntapAPIZAPI) QuotaEntryList(ctx context.Context, volumeName string) (QuotaEntries, error) {
	response, err := d.api.QuotaEntryList(volumeName)
	err = azgo.GetError(ctx, response, err)
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
	if err = azgo.GetError(ctx, response, err); err != nil {
		msg := "error setting quota"
		Logc(ctx).WithError(err).Error(msg)
		return fmt.Errorf(msg)
	}
	return nil
}

func (d OntapAPIZAPI) QuotaStatus(ctx context.Context, volumeName string) (string, error) {
	statusResponse, err := d.api.QuotaStatus(volumeName)
	if err = azgo.GetError(ctx, statusResponse, err); err != nil {
		return "", fmt.Errorf("error getting quota status for Flexvol %s: %v", volumeName, err)
	}

	return statusResponse.Result.Status(), nil
}

func (d OntapAPIZAPI) QuotaOff(ctx context.Context, volumeName string) error {
	response, err := d.api.QuotaOff(volumeName)
	if err = azgo.GetError(ctx, response, err); err != nil {
		msg := "error disabling quota"
		Logc(ctx).WithError(err).WithField("volume", volumeName).Error(msg)
		return err
	}
	return nil
}

func (d OntapAPIZAPI) QuotaOn(ctx context.Context, volumeName string) error {
	response, err := d.api.QuotaOn(volumeName)
	if err = azgo.GetError(ctx, response, err); err != nil {
		msg := "error enabling quota"
		Logc(ctx).WithError(err).WithField("volume", volumeName).Error(msg)
		return err
	}
	return nil
}

func (d OntapAPIZAPI) QuotaResize(ctx context.Context, volumeName string) error {
	response, err := d.api.QuotaResize(volumeName)
	err = azgo.GetError(ctx, response, err)
	if zerr, ok := err.(azgo.ZapiError); ok {
		if zerr.Code() == azgo.EVOLUMEDOESNOTEXIST {
			return errors.NotFoundError(zerr.Error())
		}
	}
	return err
}

func (d OntapAPIZAPI) QtreeListByPrefix(ctx context.Context, prefix, volumePrefix string) (Qtrees, error) {
	listResponse, err := d.api.QtreeList(prefix, volumePrefix)
	if err = azgo.GetError(ctx, listResponse, err); err != nil {
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
	if err = azgo.GetError(ctx, snapResponse, err); err != nil {
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
		if zerr := azgo.NewZapiError(cloneResponse); !zerr.IsPassed() {
			if zerr.Code() == azgo.EOBJECTNOTFOUND {
				return fmt.Errorf("snapshot %s does not exist in volume %s", snapshot, sourceName)
			} else if zerr.IsFailedToLoadJobError() {
				fields := LogFields{
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

func (d OntapAPIZAPI) VolumeSnapshotInfo(ctx context.Context, snapshotName, sourceVolume string) (Snapshot, error) {
	emptyResult := Snapshot{}

	snapListResponse, err := d.api.SnapshotInfo(snapshotName, sourceVolume)
	if err = azgo.GetError(ctx, snapListResponse, err); err != nil {
		return emptyResult, fmt.Errorf("error getting snapshot %v for volume %v: %v", snapshotName, sourceVolume, err)
	}

	if snapListResponse == nil {
		return emptyResult, fmt.Errorf("unexpected error getting snapshot %v for volume %v", snapshotName, sourceVolume)
	}

	if snapListResponse.Result.NumRecords() == 0 {
		return emptyResult, errors.NotFoundError(fmt.Sprintf("snapshot %v not found for volume %v", snapshotName, sourceVolume))
	} else if snapListResponse.Result.NumRecords() > 1 {
		return emptyResult, fmt.Errorf("should have exactly 1 record, not: %v", snapListResponse.Result.NumRecords())
	}

	if snapListResponse.Result.AttributesListPtr == nil || snapListResponse.Result.AttributesListPtr.SnapshotInfoPtr == nil {
		return emptyResult, fmt.Errorf("unexpected error getting snapshot %v for volume %v", snapshotName, sourceVolume)
	}

	snap := snapListResponse.Result.AttributesListPtr.SnapshotInfoPtr[0]
	result := Snapshot{
		CreateTime: time.Unix(int64(snap.AccessTime()), 0).UTC().Format(convert.TimestampFormat),
		Name:       snap.Name(),
	}

	return result, nil
}

func (d OntapAPIZAPI) VolumeSnapshotList(ctx context.Context, sourceVolume string) (Snapshots, error) {
	snapListResponse, err := d.api.SnapshotList(sourceVolume)
	if err = azgo.GetError(ctx, snapListResponse, err); err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %v", err)
	}

	snapshots := Snapshots{}

	if snapListResponse.Result.AttributesListPtr != nil {
		for _, snap := range snapListResponse.Result.AttributesListPtr.SnapshotInfoPtr {
			snapshots = append(snapshots, Snapshot{
				CreateTime: time.Unix(int64(snap.AccessTime()), 0).UTC().Format(convert.TimestampFormat),
				Name:       snap.Name(),
			})
		}
	}

	Logc(ctx).Debugf("Returned %v snapshots.", len(snapshots))

	return snapshots, nil
}

func (d OntapAPIZAPI) VolumeSetQosPolicyGroupName(ctx context.Context, name string, qos QosPolicyGroup) error {
	qosResponse, err := d.api.VolumeSetQosPolicyGroupName(name, qos)
	if err = azgo.GetError(ctx, qosResponse, err); err != nil {
		return fmt.Errorf("error setting quality of service policy: %v", err)
	}
	return nil
}

func (d OntapAPIZAPI) VolumeCloneSplitStart(ctx context.Context, cloneName string) error {
	splitResponse, err := d.api.VolumeCloneSplitStart(cloneName)
	if err = azgo.GetError(ctx, splitResponse, err); err != nil {
		return fmt.Errorf("error splitting clone: %v", err)
	}

	return nil
}

func (d OntapAPIZAPI) SnapshotRestoreVolume(
	ctx context.Context, snapshotName, sourceVolume string,
) error {
	snapResponse, err := d.api.SnapshotRestoreVolume(snapshotName, sourceVolume)

	if err = azgo.GetError(ctx, snapResponse, err); err != nil {
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
	if zerr := azgo.NewZapiError(snapResponse); !zerr.IsPassed() {
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
		Logc(ctx).WithFields(LogFields{
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

func (d OntapAPIZAPI) SnapmirrorDeleteViaDestination(
	ctx context.Context, localInternalVolumeName, localSVMName string,
) error {
	snapDeleteResponse, err := d.api.SnapmirrorDeleteViaDestination(localInternalVolumeName, localSVMName)
	if snapDeleteResponse != nil {
		if snapDeleteResponse.Result.ResultErrnoAttr != "" && snapDeleteResponse.Result.ResultErrnoAttr != azgo.
			EOBJECTNOTFOUND {
			return fmt.Errorf("error deleting snapmirror info for volume %v: %v", localInternalVolumeName, err)
		}
	}

	// Ensure no leftover snapmirror metadata
	releaseResponse, err := d.api.SnapmirrorDestinationRelease(localInternalVolumeName)
	if releaseResponse != nil {
		if releaseResponse.Result.ResultErrnoAttr == azgo.EAPIERROR {
			return errors.ReconcileIncompleteError("operation failed, retrying, err: %v", err)
		}
		if releaseResponse.Result.ResultErrnoAttr != "" && releaseResponse.Result.ResultErrnoAttr != azgo.
			SOURCEINFONOTFOUND {
			return fmt.Errorf("error releasing snapmirror info for volume %v: %v", localInternalVolumeName,
				releaseResponse.Result.ResultReasonAttr)
		}
	}

	return nil
}

func (d OntapAPIZAPI) SnapmirrorRelease(ctx context.Context, sourceFlexvolName, sourceSVMName string) error {
	// Ensure no leftover snapmirror metadata
	err := d.api.SnapmirrorRelease(sourceFlexvolName, sourceSVMName)
	if err != nil {
		if !errors.IsNotFoundError(err) {
			return fmt.Errorf("error releasing snapmirror info for volume %v: %v", sourceFlexvolName, err)
		}
	}

	return nil
}

func (d OntapAPIZAPI) IsSVMDRCapable(ctx context.Context) (bool, error) {
	return d.api.IsVserverDRCapable(ctx)
}

func (d OntapAPIZAPI) GetSVMUUID() string {
	return d.api.SVMUUID()
}

func (d OntapAPIZAPI) SetSVMUUID(svmUUID string) {
	d.api.SetSVMUUID(svmUUID)
}

func (d OntapAPIZAPI) SVMUUID() string {
	return d.api.SVMUUID()
}

func (d OntapAPIZAPI) GetSVMState(ctx context.Context) (string, error) {
	return d.api.GetSVMState(ctx)
}

func (d OntapAPIZAPI) SnapmirrorCreate(
	ctx context.Context, localInternalVolumeName, localSVMName, remoteFlexvolName,
	remoteSVMName, replicationPolicy, replicationSchedule string,
) error {
	snapCreate, err := d.api.SnapmirrorCreate(localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName,
		replicationPolicy, replicationSchedule)
	if err = azgo.GetError(ctx, snapCreate, err); err != nil {
		if zerr, ok := err.(azgo.ZapiError); !ok || zerr.Code() != azgo.EDUPLICATEENTRY {
			Logc(ctx).WithError(err).Error("Error on snapmirror create")
			return err
		}
	}
	return nil
}

func (d OntapAPIZAPI) SnapmirrorGet(
	ctx context.Context, localInternalVolumeName, localSVMName, remoteFlexvolName,
	remoteSVMName string,
) (*Snapmirror, error) {
	snapmirrorResponse, err := d.api.SnapmirrorGet(localInternalVolumeName, localSVMName, remoteFlexvolName,
		remoteSVMName)
	if err = azgo.GetError(ctx, snapmirrorResponse, err); err != nil {
		if zerr, ok := err.(azgo.ZapiError); ok {
			if zerr.Code() == azgo.EOBJECTNOTFOUND {
				return nil, NotFoundError("Error on snapmirror get")
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

	if info.LastTransferEndTimestampPtr != nil {
		transferUnix := int64(uint32(info.LastTransferEndTimestamp()))
		transferTime := time.Unix(transferUnix, 0)
		transferTime = transferTime.UTC()
		snapmirror.EndTransferTime = &transferTime
	}

	if info.IsHealthyPtr != nil {
		snapmirror.IsHealthy = info.IsHealthy()

		if info.UnhealthyReasonPtr != nil {
			snapmirror.UnhealthyReason = info.UnhealthyReason()
		}
	}

	if info.PolicyPtr != nil {
		snapmirror.ReplicationPolicy = info.Policy()
	}

	if info.SchedulePtr != nil {
		snapmirror.ReplicationSchedule = info.Schedule()
	}

	return snapmirror, nil
}

func (d OntapAPIZAPI) SnapmirrorInitialize(
	ctx context.Context, localInternalVolumeName, localSVMName, remoteFlexvolName,
	remoteSVMName string,
) error {
	_, err := d.api.SnapmirrorInitialize(localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err != nil {
		if zerr, ok := err.(azgo.ZapiError); ok {
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

func (d OntapAPIZAPI) SnapmirrorDelete(
	ctx context.Context, localInternalVolumeName, localSVMName, remoteFlexvolName,
	remoteSVMName string,
) error {
	snapDelete, deleteErr := d.api.SnapmirrorDelete(localInternalVolumeName, localSVMName, remoteFlexvolName,
		remoteSVMName)
	if deleteErr = azgo.GetError(ctx, snapDelete, deleteErr); deleteErr != nil {
		Logc(ctx).WithError(deleteErr).Warn("Error on snapmirror delete")
	}
	return deleteErr
}

func (d OntapAPIZAPI) SnapmirrorResync(
	ctx context.Context, localInternalVolumeName, localSVMName, remoteFlexvolName,
	remoteSVMName string,
) error {
	snapResync, err := d.api.SnapmirrorResync(localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = azgo.GetError(ctx, snapResync, err); err != nil {
		if zerr, ok := err.(azgo.ZapiError); !ok || zerr.Code() != azgo.ETRANSFERINPROGRESS {
			Logc(ctx).WithError(err).Error("Error on snapmirror resync")
			// If we fail on the resync, we need to cleanup the snapmirror
			// it will be recreated in a future TMR reconcile loop through this function
			if delError := d.SnapmirrorDelete(ctx, localInternalVolumeName, localSVMName, remoteFlexvolName,
				remoteSVMName); delError != nil {
				Logc(ctx).WithError(delError).Error("Error on snapmirror delete following a resync failure")
			}
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

	snapmirrorPolicy.CopyAllSnapshots = false
	for rule := range rules {
		if rule == SnapmirrorPolicyRuleAll {
			snapmirrorPolicy.CopyAllSnapshots = true
			break
		}
	}

	return snapmirrorPolicy, nil
}

func (d OntapAPIZAPI) SnapmirrorQuiesce(
	ctx context.Context, localInternalVolumeName, localSVMName, remoteFlexvolName,
	remoteSVMName string,
) error {
	snapQuiesce, err := d.api.SnapmirrorQuiesce(localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = azgo.GetError(ctx, snapQuiesce, err); err != nil {
		Logc(ctx).WithError(err).Error("Error on snapmirror quiesce")
		if zerr, ok := err.(azgo.ZapiError); ok {
			if zerr.Code() == azgo.EAPIERROR {
				err = NotReadyError(fmt.Sprintf("Snapmirror quiesce failed: %v", err.Error()))
			}
		}
		return err
	}
	return nil
}

func (d OntapAPIZAPI) SnapmirrorAbort(
	ctx context.Context, localInternalVolumeName, localSVMName, remoteFlexvolName,
	remoteSVMName string,
) error {
	snapAbort, err := d.api.SnapmirrorAbort(localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName)
	if err = azgo.GetError(ctx, snapAbort, err); err != nil {
		zerr, ok := err.(azgo.ZapiError)
		if !ok || zerr.Code() != azgo.ENOTRANSFERINPROGRESS {
			if zerr.Code() == azgo.EOBJECTNOTFOUND {
				return nil
			}
			return NotReadyError(fmt.Sprintf("Snapmirror abort failed, still aborting: %v", err.Error()))
		}
	}

	return err
}

func (d OntapAPIZAPI) SnapmirrorBreak(
	ctx context.Context, localInternalVolumeName, localSVMName, remoteFlexvolName,
	remoteSVMName, snapshotName string,
) error {
	snapBreak, err := d.api.SnapmirrorBreak(localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName,
		snapshotName)
	if err = azgo.GetError(ctx, snapBreak, err); err != nil {
		zerr, ok := err.(azgo.ZapiError)
		if ok && zerr.Code() == azgo.EAPIERROR {
			return NotReadyError(fmt.Sprintf("Snapmirror break failed: %v", err.Error()))
		}
		if !ok || zerr.Code() != azgo.EDEST_ISNOT_DP_VOLUME {
			Logc(ctx).WithError(err).Info("Error on snapmirror break")
			return err
		}
	}
	return nil
}

func (d OntapAPIZAPI) SnapmirrorUpdate(ctx context.Context, localInternalVolumeName, snapshotName string) error {
	mirrorUpdate, err := d.api.SnapmirrorUpdate(localInternalVolumeName, snapshotName)
	return azgo.GetError(ctx, mirrorUpdate, err)
}

func (d OntapAPIZAPI) JobScheduleExists(ctx context.Context, replicationSchedule string) (bool, error) {
	exists, err := d.api.JobScheduleExists(ctx, replicationSchedule)
	if err != nil {
		return false, fmt.Errorf("failed to list job schedules: %v", err)
	} else if !exists {
		return false, fmt.Errorf("specified replicationSchedule %v does not exist", replicationSchedule)
	}
	return true, nil
}

func (d OntapAPIZAPI) GetSVMPeers(ctx context.Context) ([]string, error) {
	return d.api.GetPeeredVservers(ctx)
}

func (d OntapAPIZAPI) SMBShareCreate(ctx context.Context, shareName, path string) error {
	shareCreateResponse, err := d.api.SMBShareCreate(shareName, path)
	if err = azgo.GetError(ctx, shareCreateResponse, err); err != nil {
		if zerr, ok := err.(azgo.ZapiError); ok {
			if zerr.Code() == azgo.EAPIERROR {
				return ApiError(fmt.Sprintf("%v", err))
			}
		}
		return fmt.Errorf("error while creating SMB share %v: %v", shareName, err)
	}
	return nil
}

func (d OntapAPIZAPI) SMBShareExists(ctx context.Context, shareName string) (bool, error) {
	response, err := d.api.SMBShareExists(shareName)
	if err != nil {
		return false, fmt.Errorf("error while checking SMB share %v : %v", shareName, err)
	}

	return response, nil
}

func (d OntapAPIZAPI) SMBShareDestroy(ctx context.Context, shareName string) error {
	shareDestroyResponse, err := d.api.SMBShareDestroy(shareName)
	if err = azgo.GetError(ctx, shareDestroyResponse, err); err != nil {
		if zerr, ok := err.(azgo.ZapiError); ok {
			if zerr.Code() == azgo.EAPIERROR {
				return ApiError(fmt.Sprintf("%v", err))
			}
		}
		return fmt.Errorf("error while deleting SMB share %v: %v", shareName, err)
	}
	return nil
}

func (d OntapAPIZAPI) SMBShareAccessControlCreate(ctx context.Context, shareName string, smbShareACL map[string]string) error {
	shareAccessControlResponse, err := d.api.SMBShareAccessControlCreate(shareName, smbShareACL)
	if err != nil {
		if err = azgo.GetError(ctx, shareAccessControlResponse, err); err != nil {
			if zerr, ok := err.(azgo.ZapiError); ok {
				if zerr.Code() == azgo.EAPIERROR {
					return ApiError(fmt.Sprintf("%v", err))
				}
			}
			return fmt.Errorf("error while creating SMB share access control %v: %v", shareName, err)
		}
	}

	return nil
}

func (d OntapAPIZAPI) SMBShareAccessControlDelete(ctx context.Context, shareName string,
	smbShareACL map[string]string,
) error {
	shareAccessControlDeleteResponse, err := d.api.SMBShareAccessControlDelete(shareName, smbShareACL)
	if err != nil {
		if err = azgo.GetError(ctx, shareAccessControlDeleteResponse, err); err != nil {
			if zerr, ok := err.(azgo.ZapiError); ok {
				if zerr.Code() == azgo.EAPIERROR {
					return ApiError(fmt.Sprintf("%v", err))
				}
			}
			return fmt.Errorf("error while deleting SMB share access control %v: %v", shareName, err)
		}
	}

	return nil
}

// NVMeNamespaceCreate creates NVMe namespace.
func (d OntapAPIZAPI) NVMeNamespaceCreate(ctx context.Context, ns NVMeNamespace) error {
	return errors.UnsupportedError("ZAPI call is not supported yet")
}

// NVMeNamespaceSetSize updates the namespace size to newSize.
func (d OntapAPIZAPI) NVMeNamespaceSetSize(ctx context.Context, nsUUID string, newSize int64) error {
	return errors.UnsupportedError("ZAPI call is not supported yet")
}

// NVMeNamespaceSetComment sets comments on a namespace.
func (d OntapAPIZAPI) NVMeNamespaceSetComment(ctx context.Context, name, comment string) error {
	return errors.UnsupportedError("ZAPI call is not supported yet")
}

// NVMeNamespaceSetQosPolicyGroup sets QoS policy on a namespace.
func (d OntapAPIZAPI) NVMeNamespaceSetQosPolicyGroup(
	ctx context.Context, name string, qosPolicyGroup QosPolicyGroup,
) error {
	return errors.UnsupportedError("ZAPI call is not supported yet")
}

// NVMeNamespaceRename renames the namespace to newName.
func (d OntapAPIZAPI) NVMeNamespaceRename(ctx context.Context, nsUUID, newName string) error {
	return errors.UnsupportedError("ZAPI call is not supported yet")
}

// NVMeNamespaceExists returns if an NVMe namespace exists.
func (d OntapAPIZAPI) NVMeNamespaceExists(ctx context.Context, name string) (bool, error) {
	return false, errors.UnsupportedError("ZAPI call is not supported yet")
}

// NVMeNamespaceGetByName returns NVMe namespace with the specified name.
func (d OntapAPIZAPI) NVMeNamespaceGetByName(ctx context.Context, name string) (*NVMeNamespace, error) {
	return nil, errors.UnsupportedError("ZAPI call is not supported yet")
}

// NVMeNamespaceList returns the list of NVMe namespaces with the specified pattern.
func (d OntapAPIZAPI) NVMeNamespaceList(ctx context.Context, pattern string) (NVMeNamespaces, error) {
	return nil, errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) NamespaceSize(ctx context.Context, subsystemName string) (int, error) {
	return 0, errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) NVMeNamespaceDelete(ctx context.Context, name string) error {
	return errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) NVMeSubsystemCreate(ctx context.Context, subsystemName, comment string) (*NVMeSubsystem, error) {
	return nil, errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) NVMeSubsystemAddNamespace(ctx context.Context, subsystemUUID, nsUUID string) error {
	return errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) NVMeSubsystemRemoveNamespace(ctx context.Context, subsysUUID, nsUUID string) error {
	return errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) NVMeSubsystemGetNamespaceCount(ctx context.Context, subsysUUID string) (int64, error) {
	return 0, errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) NVMeSubsystemDelete(ctx context.Context, subsysUUID string) error {
	return errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) NVMeAddHostToSubsystem(ctx context.Context, hostNQN, subsUUID string) error {
	return errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) NVMeRemoveHostFromSubsystem(ctx context.Context, hostNQN, subsUUID string) error {
	return errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) NVMeIsNamespaceMapped(ctx context.Context, subsysUUID, nsUUID string) (bool, error) {
	return false, errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) NVMeEnsureNamespaceMapped(ctx context.Context, subsystemUUID, nsUUID string) error {
	return errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) NVMeEnsureNamespaceUnmapped(ctx context.Context, hostNQN, subsystemUUID, namespaceUUID string) (bool, error) {
	return false, errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) NVMeNamespaceGetSize(ctx context.Context, subsystemName string) (int, error) {
	return 0, errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) VolumeWaitForStates(
	ctx context.Context, volumeName string, desiredStates,
	abortStates []string, maxElapsedTime time.Duration,
) (string, error) {
	fields := LogFields{
		"method":        "VolumeWaitForStates",
		"type":          "OntapAPIZAPI",
		"volume":        volumeName,
		"desiredStates": desiredStates,
		"abortStates":   abortStates,
	}
	Logd(ctx, d.driverName, d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> VolumeWaitForStates")
	defer Logd(ctx, d.driverName, d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< VolumeWaitForStates")

	var volumeState string

	checkVolumeState := func() error {
		vol, err := d.api.VolumeGet(volumeName)
		if err != nil {
			volumeState = ""
			return fmt.Errorf("error getting volume %v; %v", volumeName, err)
		}

		if vol == nil {
			return fmt.Errorf("volume %v not found", volumeName)
		}
		if vol.VolumeStateAttributesPtr == nil || vol.VolumeStateAttributesPtr.StatePtr == nil {
			return fmt.Errorf("volume %v state not found", volumeName)
		}

		volumeState := *vol.VolumeStateAttributesPtr.StatePtr
		Logc(ctx).Debugf("Volume %v is in state:%v", volumeName, volumeState)

		if collection.ContainsString(desiredStates, volumeState) {
			Logc(ctx).Debugf("Found volume in the desired state %v", desiredStates)
			return nil
		}

		Logc(ctx).Debugf("Volume is not in desired states. Current State: %v, Desired States: %v", volumeState, desiredStates)

		// Return a permanent error to stop retrying if we reached one of the abort states
		for _, abortState := range abortStates {
			if volumeState == abortState {
				Logc(ctx).Debugf("Volume is in abort state %v. Permanently backing off", volumeState)
				return backoff.Permanent(TerminalState(fmt.Errorf("volume is in abort state")))
			} else {
				return fmt.Errorf("volume is neither in desired state nor in abort state")
			}
		}

		return fmt.Errorf("volume is in unknown state")
	}

	stateNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"increment": duration,
			"message":   err.Error(),
		}).Debugf("Waiting for volume state.")
	}
	stateBackoff := backoff.NewExponentialBackOff()
	stateBackoff.MaxElapsedTime = maxElapsedTime
	stateBackoff.MaxInterval = 2 * time.Second
	stateBackoff.RandomizationFactor = 0.1
	stateBackoff.InitialInterval = backoff.DefaultInitialInterval
	stateBackoff.Multiplier = 1.414

	Logc(ctx).WithField("desiredStates", desiredStates).Info("Waiting for volume state.")

	if err := backoff.RetryNotify(checkVolumeState, stateBackoff, stateNotify); err != nil {
		if terminalStateErr, ok := err.(*TerminalStateError); ok {
			Logc(ctx).Errorf("Volume reached terminal state: %v", terminalStateErr)
		} else {
			Logc(ctx).Errorf("Volume state was not any of %s after %3.2f seconds.",
				desiredStates, stateBackoff.MaxElapsedTime.Seconds())
		}
		return volumeState, err
	}

	Logc(ctx).WithField("desiredStates", desiredStates).Debug("Desired volume state reached.")
	return volumeState, nil
}

func (d OntapAPIZAPI) StorageUnitExists(_ context.Context, _ string) (bool, error) {
	return false, errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) StorageUnitSnapshotCreate(
	_ context.Context, _, _ string,
) error {
	return errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) StorageUnitSnapshotInfo(
	_ context.Context, _, _ string,
) (*Snapshot, error) {
	return nil, errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) StorageUnitSnapshotList(
	_ context.Context, _ string,
) (*Snapshots, error) {
	return nil, errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) StorageUnitSnapshotRestore(
	_ context.Context, _, _ string,
) error {
	return errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) StorageUnitSnapshotDelete(
	_ context.Context, _, _ string,
) error {
	return errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) StorageUnitCloneCreate(
	_ context.Context, _, _, _ string,
) error {
	return errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) StorageUnitCloneSplitStart(
	_ context.Context, _ string,
) error {
	return errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) StorageUnitListBySnapshotParent(
	_ context.Context, _, _ string,
) (VolumeNameList, error) {
	return nil, errors.UnsupportedError("ZAPI call is not supported yet")
}

func (d OntapAPIZAPI) ConsistencyGroupSnapshot(ctx context.Context, snapshotName string, volumes []string) error {
	fields := LogFields{
		"Method":        "ConsistencyGroupSnapshot",
		"Type":          "OntapAPIZAPI",
		"Snapshot Name": snapshotName,
		"FlexVols":      volumes,
	}
	Logd(ctx, d.driverName,
		d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ConsistencyGroupSnapshot")
	defer Logd(ctx, d.driverName,
		d.api.ClientConfig().DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ConsistencyGroupSnapshot")

	cgStartResponse, err := d.api.ConsistencyGroupStart(snapshotName, volumes)
	if err = azgo.GetError(ctx, cgStartResponse, err); err != nil {
		return err
	}

	if cgStartResponse != nil && cgStartResponse.Result.CgIdPtr != nil {
		cgID := cgStartResponse.Result.CgId()
		_, err := d.api.ConsistencyGroupCommit(cgID)
		if err = azgo.GetError(ctx, cgStartResponse, err); err != nil {
			return err
		}
	}

	return err
}
