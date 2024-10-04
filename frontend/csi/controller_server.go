// Copyright 2022 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"

	tridentconfig "github.com/netapp/trident/config"
	controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

func (p *Plugin) CreateVolume(
	ctx context.Context, req *csi.CreateVolumeRequest,
) (*csi.CreateVolumeResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowVolumeCreate)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{"Method": "CreateVolume", "Type": "CSI_Controller", "name": req.Name}
	Logc(ctx).WithFields(fields).Debug(">>>> CreateVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< CreateVolume")

	if _, ok := p.opCache.Load(req.Name); ok {
		Logc(ctx).WithFields(fields).Debug("Create already in progress, returning DeadlineExceeded.")
		return nil, status.Error(codes.DeadlineExceeded, "create already in progress")
	} else {
		p.opCache.Store(req.Name, true)
		defer p.opCache.Delete(req.Name)
	}

	// Check arguments
	if len(req.GetName()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume name missing in request")
	}
	if req.GetVolumeCapabilities() == nil {
		return nil, status.Error(codes.InvalidArgument, "volume capabilities missing in request")
	}

	// Check for pre-existing volume with the same name
	existingVolume, err := p.orchestrator.GetVolume(ctx, req.Name)
	if err != nil && !errors.IsNotFoundError(err) {
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	// If pre-existing volume found, check for the requested capacity and already allocated capacity
	if existingVolume != nil {

		// Check if the size of existing volume is compatible with the new request
		existingSize, _ := strconv.ParseInt(existingVolume.Config.Size, 10, 64)
		if existingSize < req.GetCapacityRange().GetRequiredBytes() {
			return nil, status.Error(codes.AlreadyExists,
				fmt.Sprintf("volume %s (with different size) already exists", req.GetName()))
		}

		// If CSI asked for a clone, check that the existing volume matches the request
		if req.VolumeContentSource != nil {
			switch contentSource := req.VolumeContentSource.Type.(type) {

			case *csi.VolumeContentSource_Volume:
				sourceVolumeID := contentSource.Volume.VolumeId
				if sourceVolumeID == "" {
					return nil, status.Error(codes.InvalidArgument, "content source volume ID missing in request")
				}
				if sourceVolumeID != existingVolume.Config.CloneSourceVolume {
					return nil, status.Error(codes.AlreadyExists,
						fmt.Sprintf("volume %s (with different volume source) already exists", req.GetName()))
				}

			case *csi.VolumeContentSource_Snapshot:
				sourceSnapshotID := contentSource.Snapshot.SnapshotId
				if sourceSnapshotID == "" {
					return nil, status.Error(codes.InvalidArgument, "content source snapshot ID missing in request")
				}
				cloneSourceVolume, cloneSourceSnapshot, err := storage.ParseSnapshotID(sourceSnapshotID)
				if err != nil {
					return nil, status.Error(codes.InvalidArgument, "invalid snapshot ID")
				}
				if cloneSourceVolume != existingVolume.Config.CloneSourceVolume {
					return nil, status.Error(codes.AlreadyExists,
						fmt.Sprintf("volume %s (with different volume source) already exists", req.GetName()))
				}
				if cloneSourceSnapshot != existingVolume.Config.CloneSourceSnapshot {
					return nil, status.Error(codes.AlreadyExists,
						fmt.Sprintf("volume %s (with different snapshot source) already exists", req.GetName()))
				}

			default:
				return nil, status.Error(codes.InvalidArgument, "unsupported volume content source")
			}
		}

		// Request matches existing volume, so just return it
		csiVolume, err := p.getCSIVolumeFromTridentVolume(ctx, existingVolume)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		csiVolume.ContentSource = req.VolumeContentSource
		return &csi.CreateVolumeResponse{Volume: csiVolume}, nil
	}

	// Note: Some of the terms mentioned here might be confusing, I hope below explanation helps:
	// AccessType (VolumeMode): It can take values - `Block` for raw block device or `Filesystem` to use a filesystem
	// Protocol (trident.netapp.io/protocol): It can take values - `block` for iSCSI protocol or `file` for NFS protocol
	// AccessMode: It can take values - Read Write Once (RWO), Read Only Many (ROX) or Read Write Many (RWX)

	// Check for matching volume capabilities
	Logc(ctx).Debugf("Volume capabilities (%d): %v", len(req.GetVolumeCapabilities()), req.GetVolumeCapabilities())
	var accessModes []tridentconfig.AccessMode
	var csiAccessModes []csi.VolumeCapability_AccessMode_Mode
	var isRawBlockAccessType, isFileMountAccessType bool
	var isBlockProtocol, isFileProtocol bool
	var fsType string
	volumeMode := tridentconfig.Filesystem
	protocol := tridentconfig.ProtocolAny
	// var mountFlags []string

	if req.GetVolumeCapabilities() != nil {
		for _, capability := range req.GetVolumeCapabilities() {

			// Make a list of all the accessModes and translate that into a single protocol
			csiAccessModes = append(csiAccessModes, capability.GetAccessMode().Mode)

			// Is AccessType(VolumeMode) = block (raw block)?
			if block := capability.GetBlock(); block != nil {
				isRawBlockAccessType = true
			}

			// Is AccessType(VolumeMode) = Mount (file mount)? If so, see if fsType has been specified
			if mount := capability.GetMount(); mount != nil {
				isFileMountAccessType = true
				fsType = mount.GetFsType()
				// mountFlags = mount.GetMountFlags()
			}
		}
	}

	// AccessType (VolumeMode) can either be block (raw-block volume) or mount (file mount)
	if isRawBlockAccessType && isFileMountAccessType {
		return nil, status.Error(codes.InvalidArgument, "mixed block and mount capabilities")
	} else if isRawBlockAccessType {
		if !p.controllerHelper.SupportsFeature(ctx, CSIBlockVolumes) {
			Logc(ctx).WithFields(fields).Error("Raw block volumes are not supported for this container orchestrator.")
			return nil, status.Error(codes.FailedPrecondition,
				"raw block volumes are not supported for this container orchestrator")
		}
		volumeMode = tridentconfig.RawBlock
		fsType = tridentconfig.FsRaw
	}

	for _, csiAccessMode := range csiAccessModes {
		accessModes = append(accessModes, p.getAccessForCSIAccessMode(csiAccessMode))
		protocolLocal := p.getProtocolForCSIAccessMode(csiAccessMode, volumeMode)

		if protocolLocal == tridentconfig.Block {
			isBlockProtocol = true
		} else if protocolLocal == tridentconfig.File {
			isFileProtocol = true
		}
	}

	// Cannot have both the protocols block (iSCSI) and file (NFS) together
	if isBlockProtocol && isFileProtocol {
		return nil, status.Error(codes.InvalidArgument,
			"specified access-modes: %s, translates to both both file and block protocols")
	} else if isBlockProtocol {
		protocol = tridentconfig.Block
	} else if isFileProtocol {
		protocol = tridentconfig.File
	}

	if !p.hasBackendForProtocol(ctx, protocol) {
		return nil, status.Errorf(codes.InvalidArgument, "no available storage for access modes: %s", accessModes)
	}

	var sizeBytes int64
	if req.CapacityRange != nil {
		sizeBytes = req.CapacityRange.RequiredBytes
	}

	// Get topology requirements
	requisiteTopologies := make([]map[string]string, 0)
	for _, topReq := range req.GetAccessibilityRequirements().GetRequisite() {
		requirement := make(map[string]string)
		for k, v := range topReq.GetSegments() {
			requirement[k] = v
		}
		requisiteTopologies = append(requisiteTopologies, requirement)
	}

	preferredTopologies := make([]map[string]string, 0)
	for _, topReq := range req.GetAccessibilityRequirements().GetPreferred() {
		preference := make(map[string]string)
		for k, v := range topReq.GetSegments() {
			preference[k] = v
		}
		preferredTopologies = append(preferredTopologies, preference)
	}

	// Convert volume creation options into a Trident volume config
	volConfig, err := p.controllerHelper.GetVolumeConfig(ctx, req.Name, sizeBytes, req.Parameters, protocol, accessModes,
		volumeMode, fsType, requisiteTopologies, preferredTopologies, nil)
	if err != nil {
		p.controllerHelper.RecordVolumeEvent(ctx, req.Name, controllerhelpers.EventTypeNormal, "ProvisioningFailed", err.Error())
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	// Check if CSI asked for a clone (overrides trident.netapp.io/cloneFromPVC PVC annotation, if present)
	if req.VolumeContentSource != nil {
		switch contentSource := req.VolumeContentSource.Type.(type) {

		case *csi.VolumeContentSource_Volume:
			volumeID := contentSource.Volume.VolumeId
			if volumeID == "" {
				return nil, status.Error(codes.InvalidArgument, "content source volume ID missing in request")
			}
			volConfig.CloneSourceVolume = volumeID

		case *csi.VolumeContentSource_Snapshot:
			snapshotID := contentSource.Snapshot.SnapshotId
			if snapshotID == "" {
				return nil, status.Error(codes.InvalidArgument, "content source snapshot ID missing in request")
			}
			if cloneSourceVolume, cloneSourceSnapshot, err := storage.ParseSnapshotID(snapshotID); err != nil {
				Logc(ctx).WithFields(LogFields{
					"volumeName": req.Name,
					"snapshotID": contentSource.Snapshot.SnapshotId,
				}).Error("Cannot create clone, invalid snapshot ID.")
				return nil, status.Error(codes.NotFound, "invalid snapshot ID")
			} else {
				volConfig.CloneSourceVolume = cloneSourceVolume
				volConfig.CloneSourceSnapshot = cloneSourceSnapshot
			}

		default:
			return nil, status.Error(codes.InvalidArgument, "unsupported volume content source")
		}
	}

	// Invoke the orchestrator to create or clone the new volume
	var newVolume *storage.VolumeExternal
	if volConfig.CloneSourceVolume != "" {
		newVolume, err = p.orchestrator.CloneVolume(ctx, volConfig)
	} else if volConfig.ImportOriginalName != "" {
		newVolume, err = p.orchestrator.ImportVolume(ctx, volConfig)
	} else {
		newVolume, err = p.orchestrator.AddVolume(ctx, volConfig)
	}

	if err != nil {
		return nil, p.getCSIErrorForOrchestratorError(err)
	} else {
		p.controllerHelper.RecordVolumeEvent(ctx, req.Name, v1.EventTypeNormal, "ProvisioningSuccess", "provisioned a volume")
	}

	csiVolume, err := p.getCSIVolumeFromTridentVolume(ctx, newVolume)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	csiVolume.ContentSource = req.VolumeContentSource
	return &csi.CreateVolumeResponse{Volume: csiVolume}, nil
}

func (p *Plugin) DeleteVolume(
	ctx context.Context, req *csi.DeleteVolumeRequest,
) (*csi.DeleteVolumeResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowVolumeDelete)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{"Method": "DeleteVolume", "Type": "CSI_Controller"}
	Logc(ctx).WithFields(fields).Debug(">>>> DeleteVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< DeleteVolume")

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "no volume ID provided")
	}

	if err := p.orchestrator.DeleteVolume(ctx, req.VolumeId); err != nil {

		Logc(ctx).WithFields(LogFields{
			"volumeName": req.VolumeId,
			"error":      err,
		}).Debugf("Could not delete volume.")

		// In CSI, delete is idempotent, so don't return an error if the volume doesn't exist
		if !errors.IsNotFoundError(err) {
			return nil, p.getCSIErrorForOrchestratorError(err)
		}
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func stashIscsiTargetPortals(publishInfo map[string]string, volumePublishInfo *models.VolumePublishInfo) {
	count := 1 + len(volumePublishInfo.IscsiPortals)
	publishInfo["iscsiTargetPortalCount"] = strconv.Itoa(count)
	publishInfo["p1"] = volumePublishInfo.IscsiTargetPortal
	for i, p := range volumePublishInfo.IscsiPortals {
		key := fmt.Sprintf("p%d", i+2)
		publishInfo[key] = p
	}
}

func (p *Plugin) ControllerPublishVolume(
	ctx context.Context, req *csi.ControllerPublishVolumeRequest,
) (*csi.ControllerPublishVolumeResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowControllerPublish)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{"Method": "ControllerPublishVolume", "Type": "CSI_Controller"}
	Logc(ctx).WithFields(fields).Debug(">>>> ControllerPublishVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< ControllerPublishVolume")

	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "no volume ID provided")
	}

	nodeID := req.GetNodeId()
	if nodeID == "" {
		return nil, status.Error(codes.InvalidArgument, "no node ID provided")
	}

	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "no volume capabilities provided")
	}

	// Make sure volume exists
	volume, err := p.orchestrator.GetVolume(ctx, volumeID)
	if err != nil {
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	// Get node attributes from the node ID
	nodeInfo, err := p.orchestrator.GetNode(ctx, nodeID)
	if err != nil {
		Logc(ctx).WithField("node", nodeID).Error("Node info not found.")
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Set up volume publish info with what we know about the node
	volumePublishInfo := &models.VolumePublishInfo{
		Localhost:      false,
		HostIQN:        []string{nodeInfo.IQN},
		HostNQN:        nodeInfo.NQN,
		HostIP:         nodeInfo.IPs,
		HostName:       nodeInfo.Name,
		Unmanaged:      volume.Config.ImportNotManaged,
		LUKSEncryption: volume.Config.LUKSEncryption,
	}
	populatePublishInfoFromCSIPublishRequest(volumePublishInfo, req)

	// Update NFS export rules (?), add node IQN to igroup, etc.
	err = p.orchestrator.PublishVolume(ctx, volume.Config.Name, volumePublishInfo)
	if err != nil {
		Logc(ctx).WithFields(fields).Error(err)
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	// If any mount options are passed in via CSI (e.g. from a StorageClass), then any mount options
	// that were specified in the storage driver's backend configuration and passed here in the
	// VolumePublishInfo struct are completely discarded and replaced by the CSI-supplied values.
	mount := req.VolumeCapability.GetMount()
	if mount != nil && len(mount.MountFlags) > 0 {
		volumePublishInfo.MountOptions = strings.Join(mount.MountFlags, ",")
	}

	// Build CSI controller publish info from volume publish info
	publishInfo := map[string]string{
		"protocol": string(volume.Config.Protocol),
	}

	publishInfo["mountOptions"] = volumePublishInfo.MountOptions
	publishInfo["formatOptions"] = volumePublishInfo.FormatOptions
	publishInfo["filesystemType"] = volumePublishInfo.FilesystemType
	switch volume.Config.Protocol {
	case tridentconfig.File:
		if volumePublishInfo.FilesystemType == "smb" {
			publishInfo["smbServer"] = volumePublishInfo.SMBServer
			publishInfo["smbPath"] = volumePublishInfo.SMBPath
		} else {
			publishInfo["nfsServerIp"] = volumePublishInfo.NfsServerIP
			publishInfo["nfsPath"] = volumePublishInfo.NfsPath
		}
	case tridentconfig.Block:
		publishInfo["LUKSEncryption"] = volumePublishInfo.LUKSEncryption
		publishInfo["sharedTarget"] = strconv.FormatBool(volumePublishInfo.SharedTarget)

		if volumePublishInfo.SANType == sa.NVMe {
			// fill in only NVMe specific fields in publishInfo
			publishInfo["nvmeSubsystemNqn"] = volumePublishInfo.NVMeSubsystemNQN
			publishInfo["nvmeNamespaceUUID"] = volumePublishInfo.NVMeNamespaceUUID
			publishInfo["nvmeTargetIPs"] = strings.Join(volumePublishInfo.NVMeTargetIPs, ",")
			publishInfo["SANType"] = sa.NVMe
		} else {
			// fill in only iSCSI specific fields in publishInfo
			stashIscsiTargetPortals(publishInfo, volumePublishInfo)
			publishInfo["iscsiTargetIqn"] = volumePublishInfo.IscsiTargetIQN
			publishInfo["iscsiLunNumber"] = strconv.Itoa(int(volumePublishInfo.IscsiLunNumber))
			publishInfo["iscsiInterface"] = volumePublishInfo.IscsiInterface
			publishInfo["iscsiLunSerial"] = volumePublishInfo.IscsiLunSerial
			publishInfo["iscsiIgroup"] = volumePublishInfo.IscsiIgroup
			publishInfo["useCHAP"] = strconv.FormatBool(volumePublishInfo.UseCHAP)
			publishInfo["SANType"] = sa.ISCSI

			// Encrypt and add CHAP credentials if they're needed
			if volumePublishInfo.UseCHAP {
				if p.aesKey != nil {
					if err := encryptCHAPPublishInfo(ctx, publishInfo, volumePublishInfo, p.aesKey); err != nil {
						return nil, status.Error(codes.Internal, err.Error())
					}
				} else {
					msg := "encryption key not set; cannot encrypt CHAP credentials for transit"
					Logc(ctx).Error(msg)
					return nil, status.Error(codes.Internal, msg)
				}
			}
		}
	}

	return &csi.ControllerPublishVolumeResponse{PublishContext: publishInfo}, nil
}

func (p *Plugin) verifyVolumePublicationIsNew(ctx context.Context, vp *models.VolumePublication) error {
	existingPub, err := p.orchestrator.GetVolumePublication(ctx, vp.VolumeName, vp.NodeName)
	if err != nil {
		// Volume publication was not found or an error occurred
		return err
	}
	if reflect.DeepEqual(existingPub, vp) {
		// Volume publication already exists with the current values
		return nil
	} else {
		// Volume publication already exists with different values
		return errors.FoundError("this volume is already published to this node with different options")
	}
}

func populatePublishInfoFromCSIPublishRequest(info *models.VolumePublishInfo, req *csi.ControllerPublishVolumeRequest) {
	info.ReadOnly = req.GetReadonly()
	if req.VolumeCapability != nil {
		if req.VolumeCapability.GetAccessMode() != nil {
			info.AccessMode = int32(req.VolumeCapability.GetAccessMode().GetMode())
		}
	}
}

func (p *Plugin) ControllerUnpublishVolume(
	ctx context.Context, req *csi.ControllerUnpublishVolumeRequest,
) (*csi.ControllerUnpublishVolumeResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowControllerUnpublish)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{"Method": "ControllerUnpublishVolume", "Type": "CSI_Controller"}
	Logc(ctx).WithFields(fields).Debug(">>>> ControllerUnpublishVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< ControllerUnpublishVolume")

	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "no volume ID provided")
	}

	nodeID := req.GetNodeId()
	if nodeID == "" {
		return nil, status.Error(codes.InvalidArgument, "no node ID provided")
	}

	// Check whether the node shall be considered unsafe to publish volumes. Ideally CSI gives us this
	// extra context, but for now we get it just-in-time here.
	nodePublicationState, err := p.controllerHelper.GetNodePublicationState(ctx, nodeID)
	if err != nil {
		msg := "Could not check if node is safe to publish volumes"
		if !errors.IsNotFoundError(err) {
			Logc(ctx).WithField("node", nodeID).WithError(err).Errorf("%s.", msg)
			return nil, status.Error(codes.Internal, err.Error())
		}
		Logc(ctx).WithField("node", nodeID).WithError(err).Debugf(
			"%s; node not found in Kubernetes, continuing.", msg)
	}
	err = p.orchestrator.UpdateNode(ctx, nodeID, nodePublicationState)
	if err != nil {
		msg := "Could not update core with node status"
		if !errors.IsNotFoundError(err) {
			Logc(ctx).WithField("node", nodeID).WithError(err).Errorf("%s.", msg)
			return nil, status.Error(codes.Internal, err.Error())
		}
		Logc(ctx).WithField("node", nodeID).WithError(err).Debugf(
			"%s; node not found in Trident core, continuing.", msg)
	}

	logFields := LogFields{"volume": req.GetVolumeId(), "node": req.GetNodeId()}

	// Unpublish the volume by updating NFS export rules, removing node IQN from igroup, etc.
	if err = p.orchestrator.UnpublishVolume(ctx, volumeID, nodeID); err != nil {
		if !errors.IsNotFoundError(err) {
			Logc(ctx).WithFields(logFields).WithError(err).Error("Could not unpublish volume.")
			return nil, status.Error(codes.Internal, err.Error())
		}
		Logc(ctx).WithFields(logFields).WithError(err).Warning("Unpublish targets not found, continuing.")
	}

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (p *Plugin) ValidateVolumeCapabilities(
	ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest,
) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowVolumeGetCapabilities)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "no volume ID provided")
	}
	if req.GetVolumeCapabilities() == nil {
		return nil, status.Error(codes.InvalidArgument, "no volume capabilities provided")
	}

	volume, err := p.orchestrator.GetVolume(ctx, volumeID)
	if err != nil {
		return nil, status.Error(codes.NotFound, "volume not found")
	}

	resp := &csi.ValidateVolumeCapabilitiesResponse{}

	for _, v := range req.GetVolumeCapabilities() {
		if volume.Config.AccessMode != p.getAccessForCSIAccessMode(v.GetAccessMode().Mode) {
			resp.Message = "Could not satisfy one or more access modes."
			return resp, nil
		}
		if block := v.GetBlock(); block != nil {
			if volume.Config.Protocol != tridentconfig.Block {
				resp.Message = "Could not satisfy block protocol."
				return resp, nil
			}
		} else {
			if volume.Config.Protocol != tridentconfig.File {
				resp.Message = "Could not satisfy file protocol."
				return resp, nil
			}
		}
	}

	confirmed := &csi.ValidateVolumeCapabilitiesResponse_Confirmed{}
	confirmed.VolumeCapabilities = req.GetVolumeCapabilities()

	resp.Confirmed = confirmed

	return resp, nil
}

func (p *Plugin) ListVolumes(
	ctx context.Context, req *csi.ListVolumesRequest,
) (*csi.ListVolumesResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowVolumeList)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{"Method": "ListVolumes", "Type": "CSI_Controller"}
	Logc(ctx).WithFields(fields).Trace(">>>> ListVolumes")
	defer Logc(ctx).WithFields(fields).Trace("<<<< ListVolumes")

	// Verify volume named same as starting-token exists or not.
	if req.StartingToken != "" {
		existingVolume, err := p.orchestrator.GetVolume(ctx, req.StartingToken)
		if err != nil && !errors.IsNotFoundError(err) {
			return nil, p.getCSIErrorForOrchestratorError(err)
		}

		if existingVolume == nil {
			return nil, status.Errorf(codes.Aborted, "starting_token '%s' is not valid", req.StartingToken)
		}
	}

	volumes, err := p.orchestrator.ListVolumes(ctx)
	if err != nil {
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	entries := make([]*csi.ListVolumesResponse_Entry, 0)
	maxPageEntries := int(req.MaxEntries)
	encounteredStartingToken := req.StartingToken == ""
	nextToken := ""

	if maxPageEntries < 0 {
		return nil, status.Errorf(codes.InvalidArgument, "max_entries value '%d' is not valid", maxPageEntries)
	} else if maxPageEntries == 0 {
		maxPageEntries = math.MaxInt16
	}

	// NextToken might get set only when requested number of entries < number of volumes returned by the core,
	// it also depends on the condition whether the starting token (if any) has been found or not.
	for _, volume := range volumes {
		if !encounteredStartingToken {
			if volume.Config.Name == req.StartingToken {
				encounteredStartingToken = true
			} else {
				continue
			}
		}

		if len(entries) < maxPageEntries {
			if csiVolume, err := p.getCSIVolumeFromTridentVolume(ctx, volume); err == nil {
				entry := &csi.ListVolumesResponse_Entry{Volume: csiVolume}
				// We must always include the volume status when we report LIST_VOLUMES_PUBLISHED_NODES capability
				entry.Status = &csi.ListVolumesResponse_VolumeStatus{
					PublishedNodeIds: []string{},
				}
				// Find all the nodes to which this volume has been published
				publications, err := p.orchestrator.ListVolumePublicationsForVolume(ctx, csiVolume.VolumeId)
				if err != nil {
					msg := fmt.Sprintf("error listing volume publications for volume %s", csiVolume.VolumeId)
					Logc(ctx).WithError(err).Error(msg)
					return nil, status.Error(codes.Internal, msg)
				}
				for _, publication := range publications {
					entry.Status.PublishedNodeIds = append(entry.Status.PublishedNodeIds, publication.NodeName)
				}
				entries = append(entries, entry)
			}
		} else {
			nextToken = volume.Config.Name
			break
		}
	}

	return &csi.ListVolumesResponse{Entries: entries, NextToken: nextToken}, nil
}

func (p *Plugin) GetCapacity(_ context.Context, _ *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	// Trident doesn't report pool capacities
	return nil, status.Error(codes.Unimplemented, "")
}

func (p *Plugin) ControllerGetCapabilities(
	ctx context.Context, _ *csi.ControllerGetCapabilitiesRequest,
) (*csi.ControllerGetCapabilitiesResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowControllerGetCapabilities)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{"Method": "ControllerGetCapabilities", "Type": "CSI_Controller"}
	Logc(ctx).WithFields(fields).Trace(">>>> ControllerGetCapabilities")
	defer Logc(ctx).WithFields(fields).Trace("<<<< ControllerGetCapabilities")

	return &csi.ControllerGetCapabilitiesResponse{Capabilities: p.csCap}, nil
}

func (p *Plugin) CreateSnapshot(
	ctx context.Context, req *csi.CreateSnapshotRequest,
) (*csi.CreateSnapshotResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowSnapshotCreate)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{"Method": "CreateSnapshot", "Type": "CSI_Controller"}
	Logc(ctx).WithFields(fields).Debug(">>>> CreateSnapshot")
	defer Logc(ctx).WithFields(fields).Debug("<<<< CreateSnapshot")

	volumeName := req.GetSourceVolumeId()
	if volumeName == "" {
		return nil, status.Error(codes.InvalidArgument, "no volume ID provided")
	}

	snapshotName := req.GetName()
	if snapshotName == "" {
		return nil, status.Error(codes.InvalidArgument, "no snapshot name provided")
	}

	// Check for pre-existing snapshot with the same name on the same volume
	existingSnapshot, err := p.orchestrator.GetSnapshot(ctx, volumeName, snapshotName)
	if err != nil && !errors.IsNotFoundError(err) {
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	// If pre-existing snapshot found, just return it
	if existingSnapshot != nil {
		if csiSnapshot, err := p.getCSISnapshotFromTridentSnapshot(ctx, existingSnapshot); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		} else {
			return &csi.CreateSnapshotResponse{Snapshot: csiSnapshot}, nil
		}
	}

	// Check for pre-existing snapshot with the same name on a different volume
	if existingSnapshots, err := p.orchestrator.ListSnapshotsByName(ctx, snapshotName); err != nil {
		return nil, p.getCSIErrorForOrchestratorError(err)
	} else if len(existingSnapshots) > 0 {
		for _, s := range existingSnapshots {
			Logc(ctx).Errorf("Found existing snapshot %s in another volume %s.", s.Config.Name, s.Config.VolumeName)
		}
		// We already handled the same name / same volume case, so getting here has to mean a different volume
		return nil, status.Error(codes.AlreadyExists, "snapshot exists on a different volume")
	} else {
		Logc(ctx).Debugf("Found no existing snapshot %s in other volumes.", snapshotName)
	}

	// Convert snapshot creation options into a Trident snapshot config
	snapshotConfig, err := p.controllerHelper.GetSnapshotConfigForCreate(volumeName, snapshotName)
	if err != nil {
		p.controllerHelper.RecordVolumeEvent(ctx, req.Name, controllerhelpers.EventTypeNormal, "ProvisioningFailed", err.Error())
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	// Create the snapshot
	newSnapshot, err := p.orchestrator.CreateSnapshot(ctx, snapshotConfig)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		} else if errors.IsUnsupportedError(err) {
			// CSI snapshotter has no exponential backoff for retries, so slow it down here
			time.Sleep(10 * time.Second)
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		} else if errors.IsMaxLimitReachedError(err) {
			// CSI snapshotter has no exponential backoff for retries, so slow it down here
			time.Sleep(10 * time.Second)
			return nil, status.Error(codes.ResourceExhausted, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	if csiSnapshot, err := p.getCSISnapshotFromTridentSnapshot(ctx, newSnapshot); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	} else {
		return &csi.CreateSnapshotResponse{Snapshot: csiSnapshot}, nil
	}
}

func (p *Plugin) DeleteSnapshot(
	ctx context.Context, req *csi.DeleteSnapshotRequest,
) (*csi.DeleteSnapshotResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowSnapshotDelete)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{"Method": "DeleteSnapshot", "Type": "CSI_Controller"}
	Logc(ctx).WithFields(fields).Debug(">>>> DeleteSnapshot")
	defer Logc(ctx).WithFields(fields).Debug("<<<< DeleteSnapshot")

	snapshotID := req.GetSnapshotId()
	if snapshotID == "" {
		return nil, status.Error(codes.InvalidArgument, "no snapshot ID provided")
	}

	volumeName, snapshotName, err := storage.ParseSnapshotID(snapshotID)
	if err != nil {
		// An invalid ID is treated an a non-existent snapshot, so we log the error and return success
		Logc(ctx).Error(err)
		return &csi.DeleteSnapshotResponse{}, nil
	}

	// Delete the snapshot
	if err = p.orchestrator.DeleteSnapshot(ctx, volumeName, snapshotName); err != nil {

		Logc(ctx).WithFields(LogFields{
			"volumeName":   volumeName,
			"snapshotName": snapshotName,
			"error":        err,
		}).Debugf("Could not delete snapshot.")

		// In CSI, delete is idempotent, so don't return an error if the snapshot doesn't exist
		if !errors.IsNotFoundError(err) {
			return nil, p.getCSIErrorForOrchestratorError(err)
		}
	}

	return &csi.DeleteSnapshotResponse{}, nil
}

func (p *Plugin) ListSnapshots(
	ctx context.Context, req *csi.ListSnapshotsRequest,
) (*csi.ListSnapshotsResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowSnapshotList)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{"Method": "ListSnapshots", "Type": "CSI_Controller"}
	Logc(ctx).WithFields(fields).Trace(">>>> ListSnapshots")
	defer Logc(ctx).WithFields(fields).Trace("<<<< ListSnapshots")

	entries := make([]*csi.ListSnapshotsResponse_Entry, 0)

	snapshotID := req.GetSnapshotId()
	// If snapshotID is not set then a list of snapshots is expected.
	if snapshotID == "" {
		var snapshots []*storage.SnapshotExternal
		var err error

		sourceVolume := req.GetSourceVolumeId()
		if sourceVolume != "" {
			snapshots, err = p.orchestrator.ListSnapshotsForVolume(ctx, sourceVolume)
			// CSI spec expects empty return if source volume is not found.
			if err != nil && errors.IsNotFoundError(err) {
				err = nil
				snapshots = make([]*storage.SnapshotExternal, 0)
			}
		} else {
			snapshots, err = p.orchestrator.ListSnapshots(ctx)
		}
		if err != nil {
			return nil, p.getCSIErrorForOrchestratorError(err)
		}
		return p.getListSnapshots(ctx, req, snapshots)
	}

	volumeName, snapName, err := storage.ParseSnapshotID(snapshotID)
	if err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Errorf("Could not parse snapshot ID: %s.", snapshotID)

		// CSI spec calls for empty return if snapshot is not found; invalid ID implies the snapshot won't be found.
		return &csi.ListSnapshotsResponse{}, nil
	}

	fields = LogFields{
		"snapshotID": snapshotID,
		"volumeName": volumeName,
		"snapName":   snapName,
	}

	// Ensure the components of the snapshot ID are valid for the CO.
	if !p.controllerHelper.IsValidResourceName(volumeName) || !p.controllerHelper.IsValidResourceName(snapName) {
		Logc(ctx).WithFields(fields).WithError(err).Errorf("Invalid values in snapshot ID: %s.", snapshotID)

		// CSI spec calls for empty return if snapshot is not found; invalid ID implies the snapshot won't be found.
		return &csi.ListSnapshotsResponse{}, nil
	}

	// Check if Trident is already tracking this snapshot.
	snapshot, err := p.orchestrator.GetSnapshot(ctx, volumeName, snapName)
	if err != nil && !errors.IsNotFoundError(err) {
		Logc(ctx).WithFields(fields).WithError(err).Error("Could not get snapshot.")
		return &csi.ListSnapshotsResponse{}, p.getCSIErrorForOrchestratorError(err)
	}

	// If no snapshot can be found by this point, try snapshot import.
	if snapshot == nil {
		snapshotConfig, err := p.controllerHelper.GetSnapshotConfigForImport(ctx, volumeName, snapName)
		if err != nil {
			Logc(ctx).WithFields(fields).WithError(err).Error("Could not expand snapshot config.")
			return &csi.ListSnapshotsResponse{}, p.getCSIErrorForOrchestratorError(err)
		}

		snapshot, err = p.orchestrator.ImportSnapshot(ctx, snapshotConfig)
		if err != nil || snapshot == nil {
			Logc(ctx).WithFields(fields).WithError(err).Error("Could not import snapshot.")

			// CSI spec calls for empty return if snapshot is not found.
			if errors.IsNotFoundError(err) {
				return &csi.ListSnapshotsResponse{}, nil
			}
			return &csi.ListSnapshotsResponse{}, p.getCSIErrorForOrchestratorError(err)
		}
	}

	csiSnapshot, err := p.getCSISnapshotFromTridentSnapshot(ctx, snapshot)
	if err != nil {
		return &csi.ListSnapshotsResponse{}, status.Error(codes.Internal, err.Error())
	}
	entries = append(entries, &csi.ListSnapshotsResponse_Entry{Snapshot: csiSnapshot})

	return &csi.ListSnapshotsResponse{Entries: entries}, nil
}

func (p *Plugin) getListSnapshots(
	ctx context.Context, req *csi.ListSnapshotsRequest, snapshots []*storage.SnapshotExternal,
) (*csi.ListSnapshotsResponse, error) {
	entries := make([]*csi.ListSnapshotsResponse_Entry, 0)
	maxPageEntries := int(req.MaxEntries)
	encounteredStartingToken := req.StartingToken == ""
	nextToken := ""

	if maxPageEntries < 0 {
		return nil, status.Errorf(codes.InvalidArgument, "max_entries value '%d' is not valid", maxPageEntries)
	} else if maxPageEntries == 0 {
		maxPageEntries = math.MaxInt16
	}

	// NextToken might get set only when requested number of entries < number of snapshots returned by the core,
	// it also depends on the condition whether the starting token (if any) has been found or not.
	for _, snapshot := range snapshots {
		snapshotID := storage.MakeSnapshotID(snapshot.Config.VolumeName, snapshot.Config.Name)
		if !encounteredStartingToken {
			if snapshotID == req.StartingToken {
				encounteredStartingToken = true
			} else {
				continue
			}
		}

		if len(entries) < maxPageEntries {
			if csiSnapshot, err := p.getCSISnapshotFromTridentSnapshot(ctx, snapshot); err == nil {
				entries = append(entries, &csi.ListSnapshotsResponse_Entry{Snapshot: csiSnapshot})
			}
		} else {
			nextToken = snapshotID
			break
		}
	}
	return &csi.ListSnapshotsResponse{Entries: entries, NextToken: nextToken}, nil
}

func (p *Plugin) ControllerExpandVolume(
	ctx context.Context, req *csi.ControllerExpandVolumeRequest,
) (*csi.ControllerExpandVolumeResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowVolumeResize)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{"Method": "ControllerExpandVolume", "Type": "CSI_Controller"}
	Logc(ctx).WithFields(fields).Debug(">>>> ControllerExpandVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< ControllerExpandVolume")

	volumeId := req.GetVolumeId()
	if volumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "no volume ID provided")
	}

	if req.GetCapacityRange() == nil {
		return nil, status.Error(codes.InvalidArgument, "no capacity range provided")
	}

	minSize := req.GetCapacityRange().GetRequiredBytes()
	maxSize := req.GetCapacityRange().GetLimitBytes()
	if 0 < maxSize && maxSize < minSize {
		return nil, status.Error(codes.InvalidArgument, "limitBytes are smaller than requiredBytes")
	}
	newSize := strconv.FormatInt(minSize, 10)

	Logc(ctx).WithFields(LogFields{
		"volumeId":         volumeId,
		"capRequiredBytes": newSize,
		"capLimitBytes":    minSize,
	}).Debug("ControllerExpandVolume")

	volume, err := p.orchestrator.GetVolume(ctx, volumeId)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"volumeId":          volumeId,
			"requestedCapacity": newSize,
		}).Error("Could not find volume.")
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	nodeExpansionRequired := volume.Config.Protocol == tridentconfig.Block

	// Return success if the volume is already at least as large as required
	if volumeSize, err := strconv.ParseInt(volume.Config.Size, 10, 64); err != nil {
		Logc(ctx).WithFields(LogFields{
			"volumeId":          volumeId,
			"requestedCapacity": newSize,
		}).Error("Could not parse existing volume size.")
		return nil, p.getCSIErrorForOrchestratorError(err)
	} else if volumeSize >= minSize {
		return &csi.ControllerExpandVolumeResponse{
			CapacityBytes:         volumeSize,
			NodeExpansionRequired: nodeExpansionRequired,
		}, nil
	}

	if err = p.orchestrator.ResizeVolume(ctx, volume.Config.Name, newSize); err != nil {
		Logc(ctx).WithFields(LogFields{
			"volumeId":          volumeId,
			"requestedCapacity": newSize,
			"error":             err,
		}).Error("Could not resize volume.")
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	// Get the volume again to get the real volume size
	resizedVolume, err := p.orchestrator.GetVolume(ctx, volumeId)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"volumeId": volumeId,
		}).Error("Could not find resized volume.")
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	responseSize, err := strconv.ParseInt(resizedVolume.Config.Size, 10, 64)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"volumeId":          volumeId,
			"requestedCapacity": resizedVolume.Config.Size,
			"error":             err,
		}).Error("Invalid type conversion.")
	}

	response := csi.ControllerExpandVolumeResponse{
		CapacityBytes:         responseSize,
		NodeExpansionRequired: nodeExpansionRequired,
	}

	return &response, nil
}

func (p *Plugin) ControllerGetVolume(
	_ context.Context, _ *csi.ControllerGetVolumeRequest,
) (*csi.ControllerGetVolumeResponse, error) {
	// Trident doesn't support ControllerGetVolume
	return nil, status.Error(codes.Unimplemented, "")
}

func (p *Plugin) getCSIVolumeFromTridentVolume(
	ctx context.Context, volume *storage.VolumeExternal,
) (*csi.Volume, error) {
	capacity, err := strconv.ParseInt(volume.Config.Size, 10, 64)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"volume": volume.Config.InternalName,
			"size":   volume.Config.Size,
		}).Error("Could not parse volume size.")
		capacity = 0
	}

	attributes := map[string]string{
		"backendUUID":  volume.BackendUUID,
		"name":         volume.Config.Name,
		"internalName": volume.Config.InternalName,
		"protocol":     string(volume.Config.Protocol),
	}

	if volume.Config.InternalID != "" {
		attributes["internalID"] = volume.Config.InternalID
	}

	accessibleTopologies := make([]*csi.Topology, 0)
	if volume.Config.AllowedTopologies != nil {
		for _, segment := range volume.Config.AllowedTopologies {
			accessibleTopologies = append(accessibleTopologies, &csi.Topology{Segments: segment})
		}
	}

	return &csi.Volume{
		CapacityBytes:      capacity,
		VolumeId:           volume.Config.Name,
		VolumeContext:      attributes,
		AccessibleTopology: accessibleTopologies,
	}, nil
}

func (p *Plugin) getCSISnapshotFromTridentSnapshot(
	ctx context.Context, snapshot *storage.SnapshotExternal,
) (*csi.Snapshot, error) {
	createdSeconds, err := time.Parse(time.RFC3339, snapshot.Created)
	if err != nil {
		Logc(ctx).WithField("time", snapshot.Created).Error("Could not parse RFC3339 snapshot time.")
		createdSeconds = time.Now()
	}
	volume, err := p.orchestrator.GetVolume(ctx, snapshot.Config.VolumeName)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"volume": snapshot.Config.VolumeName,
		}).Error("Could not find volume.")
		return nil, err
	}
	volCapacityString, err := utils.ConvertSizeToBytes(volume.Config.Size)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"volume": volume.Config.InternalName,
			"size":   volume.Config.Size,
		}).Error("Could not parse volume size.")
		return nil, err
	}
	volCapacity, err := strconv.ParseInt(volCapacityString, 10, 64)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"volume": volume.Config.InternalName,
			"size":   volume.Config.Size,
		}).Error("Could not parse volume size.")
		return nil, err
	}
	size := utils.MinInt64(snapshot.SizeBytes, volCapacity)
	if size <= 0 {
		size = volCapacity
	}

	return &csi.Snapshot{
		SizeBytes:      size,
		SnapshotId:     storage.MakeSnapshotID(snapshot.Config.VolumeName, snapshot.Config.Name),
		SourceVolumeId: snapshot.Config.VolumeName,
		CreationTime:   &timestamp.Timestamp{Seconds: createdSeconds.Unix()},
		ReadyToUse:     snapshot.State == storage.SnapshotStateOnline,
	}, nil
}

func (p *Plugin) getAccessForCSIAccessMode(accessMode csi.VolumeCapability_AccessMode_Mode) tridentconfig.AccessMode {
	switch accessMode {
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER:
		return tridentconfig.ReadWriteOncePod
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER:
		return tridentconfig.ReadWriteOnce
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER:
		return tridentconfig.ReadWriteOnce
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY:
		return tridentconfig.ReadWriteOnce
	case csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY:
		return tridentconfig.ReadOnlyMany
	case csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER:
		return tridentconfig.ReadWriteMany
	case csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER:
		return tridentconfig.ReadWriteMany
	default:
		return tridentconfig.ModeAny
	}
}

func (p *Plugin) getProtocolForCSIAccessMode(
	accessMode csi.VolumeCapability_AccessMode_Mode, volumeMode tridentconfig.VolumeMode,
) tridentconfig.Protocol {
	protocol := tridentconfig.ProtocolAny

	// Mapping of AccessMode to equivalent an protocol:

	// AccessMode                       AccessType              Result: Protocol
	// SINGLE_NODE_WRITER               Any                     Any
	// SINGLE_NODE_READER_ONLY          Any                     Any
	// MULTI_NODE_READER_ONLY           Any                     Any
	// MULTI_NODE_SINGLE_WRITER         Block                   block
	// MULTI_NODE_SINGLE_WRITER         Any/Filesystem          file
	// MULTI_NODE_MULTI_WRITER          Block                   block
	// MULTI_NODE_MULTI_WRITER          Any/Filesystem          file

	if accessMode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER ||
		accessMode == csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER {
		if volumeMode == tridentconfig.RawBlock {
			protocol = tridentconfig.Block
		} else {
			protocol = tridentconfig.File
		}
	}

	return protocol
}

// hasBackendForProtocol identifies if Trident has a backend with the matching protocol.
func (p *Plugin) hasBackendForProtocol(ctx context.Context, protocol tridentconfig.Protocol) bool {
	backends, err := p.orchestrator.ListBackends(ctx)
	if err != nil || backends == nil || len(backends) == 0 {
		return false
	}

	if protocol == tridentconfig.ProtocolAny {
		return true
	}

	for _, b := range backends {
		if b.Protocol == tridentconfig.ProtocolAny || b.Protocol == protocol {
			return true
		}
	}

	return false
}

// TODO: Move this to utils once config (github.com/netapp/trident/config) has no dependency on utils.
// containsMultiNodeAccessMode identifies if list of AccessModes contains any of the multi-node access mode type
func (p *Plugin) containsMultiNodeAccessMode(accessModes []tridentconfig.AccessMode) bool {
	_, hasMultiNodeAccessMode := utils.SliceContainsElements(accessModes, tridentconfig.MultiNodeAccessModes)

	return hasMultiNodeAccessMode
}
