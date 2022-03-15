// Copyright 2021 NetApp, Inc. All Rights Reserved.

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
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend/csi/helpers"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
)

func (p *Plugin) CreateVolume(
	ctx context.Context, req *csi.CreateVolumeRequest,
) (*csi.CreateVolumeResponse, error) {

	fields := log.Fields{"Method": "CreateVolume", "Type": "CSI_Controller", "name": req.Name}
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
	if err != nil && !utils.IsNotFoundError(err) {
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
		if !p.helper.SupportsFeature(ctx, CSIBlockVolumes) {
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
	var requisiteTopologies = make([]map[string]string, 0)
	for _, topReq := range req.GetAccessibilityRequirements().GetRequisite() {
		requirement := make(map[string]string)
		for k, v := range topReq.GetSegments() {
			requirement[k] = v
		}
		requisiteTopologies = append(requisiteTopologies, requirement)
	}

	var preferredTopologies = make([]map[string]string, 0)
	for _, topReq := range req.GetAccessibilityRequirements().GetPreferred() {
		preference := make(map[string]string)
		for k, v := range topReq.GetSegments() {
			preference[k] = v
		}
		preferredTopologies = append(preferredTopologies, preference)
	}

	// Convert volume creation options into a Trident volume config
	volConfig, err := p.helper.GetVolumeConfig(ctx, req.Name, sizeBytes, req.Parameters, protocol, accessModes,
		volumeMode, fsType, requisiteTopologies, preferredTopologies, nil)
	if err != nil {
		p.helper.RecordVolumeEvent(ctx, req.Name, helpers.EventTypeNormal, "ProvisioningFailed", err.Error())
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
				Logc(ctx).WithFields(log.Fields{
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
		p.helper.RecordVolumeEvent(ctx, req.Name, helpers.EventTypeNormal, "ProvisioningFailed", err.Error())
		return nil, p.getCSIErrorForOrchestratorError(err)
	} else {
		p.helper.RecordVolumeEvent(ctx, req.Name, v1.EventTypeNormal, "ProvisioningSuccess", "provisioned a volume")
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

	fields := log.Fields{"Method": "DeleteVolume", "Type": "CSI_Controller"}
	Logc(ctx).WithFields(fields).Debug(">>>> DeleteVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< DeleteVolume")

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "no volume ID provided")
	}

	if err := p.orchestrator.DeleteVolume(ctx, req.VolumeId); err != nil {

		Logc(ctx).WithFields(log.Fields{
			"volumeName": req.VolumeId,
			"error":      err,
		}).Debugf("Could not delete volume.")

		// In CSI, delete is idempotent, so don't return an error if the volume doesn't exist
		if !utils.IsNotFoundError(err) {
			return nil, p.getCSIErrorForOrchestratorError(err)
		}
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func stashIscsiTargetPortals(publishInfo map[string]string, volumePublishInfo *utils.VolumePublishInfo) {

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

	fields := log.Fields{"Method": "ControllerPublishVolume", "Type": "CSI_Controller"}
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

	// TODO (akerr): Discover all hosts currently mounting this volume

	// Get node attributes from the node ID
	nodeInfo, err := p.orchestrator.GetNode(ctx, nodeID)
	if err != nil {
		Logc(ctx).WithField("node", nodeID).Error("Node info not found.")
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Verify the publication is new or the same as the existing publication
	vp := generateVolumePublicationFromCSIPublishRequest(req)
	err = p.verifyVolumePublicationIsNew(ctx, vp)
	// If err == nil then the volume publication already exists with the requested values, so we do not need to do
	// anything
	if err != nil {
		if utils.IsNotFoundError(err) {
			// Volume publication does not exist, add it
			if err := p.orchestrator.AddVolumePublication(ctx, vp); err != nil {
				msg := "error recording volume publication"
				Logc(ctx).WithError(err).Error(msg)
				return nil, status.Error(codes.Internal, msg)
			}
		} else if utils.IsFoundError(err) {
			// Volume publication exists, but with different values, we cannot handle this
			return nil, status.Error(codes.AlreadyExists, err.Error())
		} else {
			// Something else went wrong
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	// Set up volume publish info with what we know about the node
	volumePublishInfo := &utils.VolumePublishInfo{
		Localhost: false,
		HostIQN:   []string{nodeInfo.IQN},
		HostIP:    nodeInfo.IPs,
		HostName:  nodeInfo.Name,
		Unmanaged: volume.Config.ImportNotManaged,
	}

	// Update NFS export rules (?), add node IQN to igroup, etc.
	err = p.orchestrator.PublishVolume(ctx, volume.Config.Name, volumePublishInfo)
	if err != nil {
		Logc(ctx).Error(err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	// If any mount options are passed in via CSI (e.g. from a StorageClass), then any mount options
	// that were specified in the storage driver's backend configuration and passed here in the
	// VolumePublishInfo struct are completely discarded and replaced by the CSI-supplied values.
	mount := req.VolumeCapability.GetMount()
	if mount != nil && len(mount.MountFlags) > 0 {
		if volume.Config.Protocol == tridentconfig.BlockOnFile {
			mount.MountFlags = utils.RemoveStringFromSlice(mount.MountFlags, "ro")
			volumePublishInfo.SubvolumeMountOptions = strings.Join(mount.MountFlags, ",")
		} else {
			volumePublishInfo.MountOptions = strings.Join(mount.MountFlags, ",")
		}
	}

	// Build CSI controller publish info from volume publish info
	publishInfo := map[string]string{
		"protocol": string(volume.Config.Protocol),
	}

	publishInfo["mountOptions"] = volumePublishInfo.MountOptions
	if volume.Config.Protocol == tridentconfig.File {
		publishInfo["nfsServerIp"] = volume.Config.AccessInfo.NfsServerIP
		publishInfo["nfsPath"] = volume.Config.AccessInfo.NfsPath
	} else if volume.Config.Protocol == tridentconfig.Block {
		stashIscsiTargetPortals(publishInfo, volumePublishInfo)
		publishInfo["iscsiTargetIqn"] = volume.Config.AccessInfo.IscsiTargetIQN
		publishInfo["iscsiLunNumber"] = strconv.Itoa(int(volume.Config.AccessInfo.IscsiLunNumber))
		publishInfo["iscsiInterface"] = volume.Config.AccessInfo.IscsiInterface
		publishInfo["iscsiLunSerial"] = volume.Config.AccessInfo.IscsiLunSerial
		publishInfo["iscsiIgroup"] = volume.Config.AccessInfo.IscsiIgroup
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
		publishInfo["filesystemType"] = volumePublishInfo.FilesystemType
		publishInfo["useCHAP"] = strconv.FormatBool(volumePublishInfo.UseCHAP)
		publishInfo["sharedTarget"] = strconv.FormatBool(volumePublishInfo.SharedTarget)
	} else if volume.Config.Protocol == tridentconfig.BlockOnFile {
		publishInfo["subvolumeMountOptions"] = volumePublishInfo.SubvolumeMountOptions
		publishInfo["nfsServerIp"] = volume.Config.AccessInfo.NfsServerIP
		publishInfo["nfsPath"] = volume.Config.AccessInfo.NfsPath
		publishInfo["nfsUniqueID"] = volume.Config.AccessInfo.NfsUniqueID
		publishInfo["subvolumeName"] = volume.Config.AccessInfo.SubvolumeName
		publishInfo["filesystemType"] = volumePublishInfo.FilesystemType
		publishInfo["backendUUID"] = volumePublishInfo.BackendUUID
	}

	return &csi.ControllerPublishVolumeResponse{PublishContext: publishInfo}, nil
}

func (p *Plugin) verifyVolumePublicationIsNew(ctx context.Context, vp *utils.VolumePublication) error {
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
		return utils.FoundError("this volume is already published to this node with different options")
	}
}

func generateVolumePublicationFromCSIPublishRequest(req *csi.ControllerPublishVolumeRequest) *utils.VolumePublication {
	vp := &utils.VolumePublication{
		Name:       utils.GenerateVolumePublishName(req.GetVolumeId(), req.GetNodeId()),
		VolumeName: req.GetVolumeId(),
		NodeName:   req.GetNodeId(),
		ReadOnly:   req.GetReadonly(),
	}
	if req.VolumeCapability != nil {
		if req.VolumeCapability.GetAccessMode() != nil {
			vp.AccessMode = int32(req.VolumeCapability.GetAccessMode().GetMode())
		}
	}
	return vp
}

func (p *Plugin) ControllerUnpublishVolume(
	ctx context.Context, req *csi.ControllerUnpublishVolumeRequest,
) (*csi.ControllerUnpublishVolumeResponse, error) {

	fields := log.Fields{"Method": "ControllerUnpublishVolume", "Type": "CSI_Controller"}
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

	logFields := log.Fields{"volume": req.GetVolumeId(), "node": req.GetNodeId()}

	// Unpublish the volume by updating NFS export rules, removing node IQN from igroup, etc.
	if err := p.orchestrator.UnpublishVolume(ctx, volumeID, nodeID); err != nil {
		if !utils.IsNotFoundError(err) {
			Logc(ctx).WithFields(logFields).WithError(err).Error("Could not unpublish volume.")
			return nil, status.Error(codes.Internal, err.Error())
		}
		Logc(ctx).WithFields(logFields).WithError(err).Warning("Unpublish targets not found, continuing.")
	}

	// Remove the stateful publish tracking record.
	if err := p.orchestrator.DeleteVolumePublication(ctx, req.GetVolumeId(), req.GetNodeId()); err != nil {
		if !utils.IsNotFoundError(err) {
			Logc(ctx).WithFields(logFields).WithError(err).Error("Could not remove volume publication record.")
			return nil, status.Error(codes.Internal, err.Error())
		}
		Logc(ctx).WithFields(logFields).Warning("Volume publication record not found, returning success.")
	}

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (p *Plugin) ValidateVolumeCapabilities(
	ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest,
) (*csi.ValidateVolumeCapabilitiesResponse, error) {

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

	fields := log.Fields{"Method": "ListVolumes", "Type": "CSI_Controller"}
	Logc(ctx).WithFields(fields).Debug(">>>> ListVolumes")
	defer Logc(ctx).WithFields(fields).Debug("<<<< ListVolumes")

	// Verify volume named same as starting-token exists or not.
	if req.StartingToken != "" {
		existingVolume, err := p.orchestrator.GetVolume(ctx, req.StartingToken)
		if err != nil && !utils.IsNotFoundError(err) {
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

	fields := log.Fields{"Method": "ControllerGetCapabilities", "Type": "CSI_Controller"}
	Logc(ctx).WithFields(fields).Debug(">>>> ControllerGetCapabilities")
	defer Logc(ctx).WithFields(fields).Debug("<<<< ControllerGetCapabilities")

	return &csi.ControllerGetCapabilitiesResponse{Capabilities: p.csCap}, nil
}

func (p *Plugin) CreateSnapshot(
	ctx context.Context, req *csi.CreateSnapshotRequest,
) (*csi.CreateSnapshotResponse, error) {

	fields := log.Fields{"Method": "CreateSnapshot", "Type": "CSI_Controller"}
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
	if err != nil && !utils.IsNotFoundError(err) {
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
	snapshotConfig, err := p.helper.GetSnapshotConfig(volumeName, snapshotName)
	if err != nil {
		p.helper.RecordVolumeEvent(ctx, req.Name, helpers.EventTypeNormal, "ProvisioningFailed", err.Error())
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	// Create the snapshot
	newSnapshot, err := p.orchestrator.CreateSnapshot(ctx, snapshotConfig)
	if err != nil {
		if utils.IsNotFoundError(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		} else if utils.IsUnsupportedError(err) {
			// CSI snapshotter has no exponential backoff for retries, so slow it down here
			time.Sleep(10 * time.Second)
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		} else if utils.IsMaxLimitReachedError(err) {
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

	fields := log.Fields{"Method": "DeleteSnapshot", "Type": "CSI_Controller"}
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

		Logc(ctx).WithFields(log.Fields{
			"volumeName":   volumeName,
			"snapshotName": snapshotName,
			"error":        err,
		}).Debugf("Could not delete snapshot.")

		// In CSI, delete is idempotent, so don't return an error if the snapshot doesn't exist
		if !utils.IsNotFoundError(err) {
			return nil, p.getCSIErrorForOrchestratorError(err)
		}
	}

	return &csi.DeleteSnapshotResponse{}, nil
}

func (p *Plugin) ListSnapshots(
	ctx context.Context, req *csi.ListSnapshotsRequest,
) (*csi.ListSnapshotsResponse, error) {

	fields := log.Fields{"Method": "ListSnapshots", "Type": "CSI_Controller"}
	Logc(ctx).WithFields(fields).Debug(">>>> ListSnapshots")
	defer Logc(ctx).WithFields(fields).Debug("<<<< ListSnapshots")

	entries := make([]*csi.ListSnapshotsResponse_Entry, 0)

	snapshotID := req.GetSnapshotId()
	// If snapshotID is not set then a list of snapshots is expected.
	if snapshotID == "" {
		var snapshots []*storage.SnapshotExternal
		var err error

		sourceVolume := req.GetSourceVolumeId()
		if sourceVolume != "" {
			snapshots, err = p.orchestrator.ListSnapshotsForVolume(ctx, sourceVolume)
			// CSI spec expects empty return if source volume is not found
			if err != nil && utils.IsNotFoundError(err) {
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

	volumeName, snapshotName, err := storage.ParseSnapshotID(snapshotID)
	if err != nil {
		Logc(ctx).WithFields(fields).Warnf("Snapshot %s not found.", snapshotID)
		// CSI spec calls for empty return if snapshot is not found
		return &csi.ListSnapshotsResponse{}, nil
	}

	// Get the snapshot
	snapshot, err := p.orchestrator.GetSnapshot(ctx, volumeName, snapshotName)
	if err != nil {

		Logc(ctx).WithFields(log.Fields{
			"volumeName":   volumeName,
			"snapshotName": snapshotName,
			"error":        err,
		}).Debugf("Could not find snapshot.")

		// CSI spec calls for empty return if snapshot is not found
		return &csi.ListSnapshotsResponse{}, nil
	}

	if csiSnapshot, err := p.getCSISnapshotFromTridentSnapshot(ctx, snapshot); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	} else {
		entries = append(entries, &csi.ListSnapshotsResponse_Entry{Snapshot: csiSnapshot})
	}

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

	fields := log.Fields{"Method": "ControllerExpandVolume", "Type": "CSI_Controller"}
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

	Logc(ctx).WithFields(log.Fields{
		"volumeId":         volumeId,
		"capRequiredBytes": newSize,
		"capLimitBytes":    minSize,
	}).Debug("ControllerExpandVolume")

	volume, err := p.orchestrator.GetVolume(ctx, volumeId)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"volumeId":          volumeId,
			"requestedCapacity": newSize,
		}).Error("Could not find volume.")
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	nodeExpansionRequired := volume.Config.Protocol == tridentconfig.Block || volume.Config.
		Protocol == tridentconfig.BlockOnFile

	// Return success if the volume is already at least as large as required
	if volumeSize, err := strconv.ParseInt(volume.Config.Size, 10, 64); err != nil {
		Logc(ctx).WithFields(log.Fields{
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
		Logc(ctx).WithFields(log.Fields{
			"volumeId":          volumeId,
			"requestedCapacity": newSize,
			"error":             err,
		}).Error("Could not resize volume.")
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	// Get the volume again to get the real volume size
	resizedVolume, err := p.orchestrator.GetVolume(ctx, volumeId)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"volumeId": volumeId,
		}).Error("Could not find resized volume.")
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	responseSize, err := strconv.ParseInt(resizedVolume.Config.Size, 10, 64)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
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
		Logc(ctx).WithFields(log.Fields{
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
		Logc(ctx).WithFields(log.Fields{
			"volume": snapshot.Config.VolumeName,
		}).Error("Could not find volume.")
		return nil, err
	}
	volCapacityString, err := utils.ConvertSizeToBytes(volume.Config.Size)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"volume": volume.Config.InternalName,
			"size":   volume.Config.Size,
		}).Error("Could not parse volume size.")
		return nil, err
	}
	volCapacity, err := strconv.ParseInt(volCapacityString, 10, 64)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
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
