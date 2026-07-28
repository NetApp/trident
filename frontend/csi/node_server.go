// Copyright 2026 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tridentconfig "github.com/netapp/trident/config"
	nodecore "github.com/netapp/trident/core/node"
	nodehelpers "github.com/netapp/trident/frontend/csi/node_helpers"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/pkg/locks"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/smb"
)

const (
	nodeLockID = "csi_node_server"
)

var (
	csiNodeLockTimeout = 60 * time.Second

	// topologyLabels: nil = not yet loaded; non-nil (possibly empty) = loaded.
	// Cache writes store {} instead of nil so != nil acts as the initialized sentinel.
	topologyLabels     map[string]string
	topologyLabelsLock sync.RWMutex
	iscsiUtils         = iscsi.IscsiUtils
	fcpUtils           = utils.FcpUtils
)

// getTopologyLabels lazily loads topology segments once. Return paths keep maps.Clone results as-is:
// nil and {} are equivalent for read-only CSI Segments, and not normalizing on error preserves retry.
func (p *Plugin) getTopologyLabels(ctx context.Context) map[string]string {
	topologyLabelsLock.RLock()
	if topologyLabels != nil || p.controllerHelper == nil {
		labels := maps.Clone(topologyLabels)
		topologyLabelsLock.RUnlock()
		return labels
	}
	topologyLabelsLock.RUnlock()

	labels, err := p.controllerHelper.GetNodeTopologyLabels(ctx, p.nodeName)
	if err != nil {
		Logc(ctx).WithError(err).Debug("Unable to get topology labels from node; falling back to registered labels.")
		topologyLabelsLock.RLock()
		defer topologyLabelsLock.RUnlock()
		return maps.Clone(topologyLabels)
	}

	topologyLabelsLock.Lock()
	defer topologyLabelsLock.Unlock()

	if topologyLabels != nil {
		return maps.Clone(topologyLabels)
	}

	topologyLabels = maps.Clone(labels)
	if topologyLabels == nil {
		topologyLabels = map[string]string{} // mark cache initialized so we do not re-fetch K8s
	}
	if len(labels) > 0 {
		val, ok := labels[K8sTopologyRegionLabel]
		if ok && val != "" {
			p.topologyInUse.Store(true)
		}
	}

	return maps.Clone(labels)
}

func attemptLock(ctx context.Context, lockContext, lockID string, lockTimeout time.Duration) bool {
	startTime := time.Now()
	locks.Lock(ctx, lockContext, lockID)
	// Fail if the gRPC call came in a long time ago to avoid kubelet 120s timeout
	if time.Since(startTime) > lockTimeout {
		Logc(ctx).Debugf("Request spent more than %v in the queue and timed out", csiNodeLockTimeout)
		return false
	}
	return true
}

// NodeStageVolume is responsible for handling a CSI stage volume request.
// It must parse the request and construct a *models.VolumePublishInfo.
func (p *Plugin) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume requires a non-nil request")
	}

	volumeID, targetPath := req.GetVolumeId(), req.GetStagingTargetPath()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume requires a non-empty VolumeID")
	}
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume requires a non-empty TargetPath")
	}

	fields := LogFields{"Method": "NodeStageVolume", "Type": "CSI_Node", "VolumeID": volumeID, "TargetPath": targetPath}
	Logc(ctx).WithFields(fields).Debug(">>>> NodeStageVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NodeStageVolume")

	secrets := req.GetSecrets()
	volumeContext := req.GetVolumeContext()
	publishContext := req.GetPublishContext()

	// Seed the publish info with values common to all protocols.
	publishInfo := &models.VolumePublishInfo{}
	publishInfo.InternalID = volumeContext["internalName"]
	publishInfo.Localhost = true
	publishInfo.GlobalMount = targetPath
	publishInfo.VolumeMode = publishContext["volumeMode"]
	publishInfo.MountOptions = publishContext["mountOptions"]
	publishInfo.FilesystemType = publishContext["filesystemType"]
	publishInfo.Secrets = secrets

	switch tridentconfig.Protocol(publishContext["protocol"]) {
	case tridentconfig.File:
		switch publishInfo.FilesystemType {
		case smb.SMB:
			publishInfo.StorageProtocol = models.SMB
			publishInfo.FilesystemType = smb.SMB
			publishInfo.SMBPath = req.PublishContext["smbPath"]
			publishInfo.SMBServer = req.PublishContext["smbServer"]
			publishInfo.SMBADUser = secrets["username"]
			publishInfo.SMBADPass = secrets["password"]
		default:
			publishInfo.StorageProtocol = models.NFS // Yes. This is a duplicate to FilesystemType for File protocols.
			publishInfo.FilesystemType = "nfs"
			publishInfo.NfsPath = req.PublishContext["nfsPath"]
			publishInfo.NfsServerIP = req.PublishContext["nfsServerIp"]
		}

	case tridentconfig.Block:
		if err := validateStageSANVolumeCapability(req); err != nil {
			return nil, err
		}

		if mountCapability := req.GetVolumeCapability().GetMount(); mountCapability != nil && mountCapability.GetFsType() != "" {
			publishInfo.FilesystemType = mountCapability.GetFsType()
		}

		publishInfo.SANType = req.PublishContext["SANType"]
		publishInfo.FormatOptions = req.PublishContext["formatOptions"]
		publishInfo.LUKSEncryption = strconv.FormatBool(convert.ToBool(req.PublishContext["LUKSEncryption"]))

		switch publishContext["SANType"] {
		case sa.ISCSI:
			publishInfo.StorageProtocol = models.ISCSI
			var lunID int32
			lunID, err := convert.ToPositiveInt32(req.PublishContext["iscsiLunNumber"])
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			publishInfo.IscsiLunNumber = lunID

			if err = unstashIscsiTargetPortals(publishInfo, req.PublishContext); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}

			useCHAP, err := strconv.ParseBool(req.PublishContext["useCHAP"])
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			if useCHAP {
				publishInfo.IscsiUsername = req.PublishContext["iscsiUsername"]
				publishInfo.IscsiInitiatorSecret = req.PublishContext["iscsiInitiatorSecret"]
				publishInfo.IscsiTargetUsername = req.PublishContext["iscsiTargetUsername"]
				publishInfo.IscsiTargetSecret = req.PublishContext["iscsiTargetSecret"]

				if containsEncryptedCHAP(publishContext) {
					if err := decryptCHAPPublishInfo(ctx, publishInfo, publishContext, p.aesKey); err != nil {
						Logc(ctx).Warn("Could not decrypt CHAP credentials; will retrieve from the Trident controller later.")
						// Do not make an API call here.
						// Let attach workflow in the node core fetch the credentials instead.
					}
				}
			}
			publishInfo.UseCHAP = useCHAP

			publishInfo.IscsiIgroup = req.PublishContext["iscsiIgroup"]
			publishInfo.IscsiTargetIQN = req.PublishContext["iscsiTargetIqn"]
			publishInfo.IscsiLunSerial = req.PublishContext["iscsiLunSerial"]
			publishInfo.IscsiInterface = req.PublishContext["iscsiInterface"]
		case sa.FCP:
			publishInfo.StorageProtocol = models.FCP
			var lunID int32
			lunID, err := convert.ToPositiveInt32(req.PublishContext["fcpLunNumber"])
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			publishInfo.FCPLunNumber = lunID
			publishInfo.FCPIgroup = req.PublishContext["fcpIgroup"]
			publishInfo.FCPLunSerial = req.PublishContext["fcpLunSerial"]
			publishInfo.FCTargetWWNN = req.PublishContext["fcTargetWWNN"]
		case sa.NVMe:
			publishInfo.StorageProtocol = models.NVMe
			publishInfo.NVMeSubsystemNQN = req.PublishContext["nvmeSubsystemNqn"]
			publishInfo.NVMeNamespaceUUID = req.PublishContext["nvmeNamespaceUUID"]
			publishInfo.NVMeTargetIPs = strings.Split(req.PublishContext["nvmeTargetIPs"], ",")
		default:
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("unsupported SAN_TYPE %q", publishContext["SANType"]))
		}

	default:
		return nil, status.Error(codes.InvalidArgument, "unknown protocol")
	}

	attachReq := nodecore.AttachRequest{PublishInfo: publishInfo}
	if tridentconfig.Protocol(publishContext["protocol"]) == tridentconfig.Block {
		sharedTarget, parseErr := strconv.ParseBool(publishContext["sharedTarget"])
		if parseErr != nil {
			return nil, status.Error(codes.Internal, parseErr.Error())
		}
		attachReq.SharedTarget = sharedTarget
	}

	if err := p.nodeOrchestrator.Attach(ctx, volumeID, attachReq); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, status.Error(codes.DeadlineExceeded, "NodeStageVolume timed out")
		}
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

// NodeUnstageVolume detaches a volume from a node. All protocol-specific detachment logic and
// admission control (per-protocol concurrency limiting) live in the node core; the frontend only
// validates the request and translates errors to gRPC status codes.
func (p *Plugin) NodeUnstageVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest,
) (res *csi.NodeUnstageVolumeResponse, err error) {
	fields := LogFields{"Method": "NodeUnstageVolume", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Debug(">>>> NodeUnstageVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NodeUnstageVolume")

	// Empty strings for either of these arguments are required by CSI Sanity to return an error.
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "no volume ID provided")
	}
	if req.GetStagingTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "no staging target path provided")
	}

	if err := p.nodeOrchestrator.Detach(ctx, req.GetVolumeId(), nodecore.DetachRequest{}); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, status.Error(codes.DeadlineExceeded, "NodeUnstageVolume timed out")
		}
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

// NodePublishVolume publishes an already-staged volume to its pod-visible target path. All
// protocol-specific mount logic and admission control (per-protocol concurrency limiting) live
// in the node core; the frontend only validates the request and translates errors to gRPC
// status codes.
func (p *Plugin) NodePublishVolume(
	ctx context.Context, req *csi.NodePublishVolumeRequest,
) (res *csi.NodePublishVolumeResponse, err error) {
	fields := LogFields{"Method": "NodePublishVolume", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Debug(">>>> NodePublishVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NodePublishVolume")

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "no volume ID provided")
	}
	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "no target path provided")
	}
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "no volume capability provided")
	}

	switch req.PublishContext["protocol"] {
	case string(tridentconfig.File), string(tridentconfig.Block):
	default:
		return nil, status.Error(codes.InvalidArgument, "unknown protocol")
	}

	if err := p.nodeOrchestrator.Mount(ctx, req.GetVolumeId(), nodecore.MountRequest{
		TargetPath:             req.GetTargetPath(),
		ReadOnly:               req.GetReadonly(),
		Secrets:                req.GetSecrets(),
		SingleNodeSingleWriter: hasSingleNodeSingleWriterAccessMode(req),
	}); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, status.Error(codes.DeadlineExceeded, "NodePublishVolume timed out")
		}
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume removes a volume's pod-visible mount. All protocol-specific unmount
// logic and admission control live in the node core; the frontend only validates the request
// and translates errors to gRPC status codes.
func (p *Plugin) NodeUnpublishVolume(
	ctx context.Context, req *csi.NodeUnpublishVolumeRequest,
) (res *csi.NodeUnpublishVolumeResponse, err error) {
	fields := LogFields{"Method": "NodeUnpublishVolume", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Debug(">>>> NodeUnpublishVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NodeUnpublishVolume")

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "no volume ID provided")
	}
	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "no target path provided")
	}

	if err := p.nodeOrchestrator.Unmount(ctx, req.GetVolumeId(), nodecore.UnmountRequest{
		TargetPath: req.GetTargetPath(),
	}); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, status.Error(codes.DeadlineExceeded, "NodeUnpublishVolume timed out")
		}
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (p *Plugin) NodeGetVolumeStats(
	ctx context.Context, req *csi.NodeGetVolumeStatsRequest,
) (res *csi.NodeGetVolumeStatsResponse, err error) {
	Logc(ctx).WithFields(LogFields{
		"volumeId":          req.VolumeId,
		"volumePath":        req.GetVolumePath(),
		"stagingTargetPath": req.StagingTargetPath,
	}).Debug("NodeGetVolumeStats request received")

	// Validate request
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume id provided")
	}
	if req.GetVolumePath() == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume path provided")
	}

	// Verify volume exists at path
	if err := p.nodeHelper.VerifyVolumePath(ctx, req.GetVolumePath()); err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Read tracking info to determine volume type
	trackingInfo, err := p.nodeHelper.ReadTrackingInfo(ctx, req.VolumeId)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("error reading the tracking file: %v", err.Error()))
	}

	// Determine if this is a raw block or filesystem volume
	isRawBlock := p.nodeHelper.IsRawBlockVolume(trackingInfo)

	var stats *nodehelpers.VolumeStats
	if isRawBlock {
		// Get stats for raw block volume
		stats, err = p.nodeHelper.GetBlockDeviceStatsByID(ctx, req.VolumeId, trackingInfo)
		if err != nil {
			// For block devices, log warning but return empty response (not an error)
			Logc(ctx).WithError(err).Warn("Failed to get block device volume statistics, returning empty response")
			return &csi.NodeGetVolumeStatsResponse{}, nil
		}
	} else {
		// Get stats for filesystem volume
		stats, err = p.nodeHelper.GetFilesystemStatsByID(ctx, req.GetVolumePath())
		if err != nil {
			Logc(ctx).Errorf("unable to get filesystem stats at path: %s; %v", req.GetVolumePath(), err)
			return nil, status.Error(codes.Unknown, "Failed to get filesystem stats")
		}
	}

	// Build CSI response from VolumeStats
	if isRawBlock {
		return &csi.NodeGetVolumeStatsResponse{
			Usage: []*csi.VolumeUsage{
				{
					Unit:      csi.VolumeUsage_BYTES,
					Total:     stats.Total,
					Used:      stats.Used,
					Available: stats.Available,
				},
			},
		}, nil
	}

	// Filesystem volumes include inode information
	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Unit:      csi.VolumeUsage_BYTES,
				Total:     stats.Total,
				Used:      stats.Used,
				Available: stats.Available,
			},
			{
				Unit:      csi.VolumeUsage_INODES,
				Total:     stats.Inodes,
				Used:      stats.InodesUsed,
				Available: stats.InodesFree,
			},
		},
	}, nil
}

// NodeExpandVolume handles volume expansion for Block (i.e. iSCSI) volumes.  The CO only calls NodeExpandVolume
// for the Block protocol as the filesystem has to be mounted to perform the resize. This is enforced in our
// ControllerExpandVolume method where we return true for nodeExpansionRequired when the protocol is Block and
// return false when the protocol is file.
func (p *Plugin) NodeExpandVolume(
	ctx context.Context, req *csi.NodeExpandVolumeRequest,
) (res *csi.NodeExpandVolumeResponse, err error) {
	fields := LogFields{"Method": "NodeExpandVolume", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Debug(">>>> NodeExpandVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NodeExpandVolume")

	volumeId := req.GetVolumeId()
	if volumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "no volume ID provided")
	}

	volumePath := req.GetVolumePath()
	if volumePath == "" {
		return nil, status.Error(codes.InvalidArgument, "no volume path provided")
	}

	ctx = SetContextWorkflow(ctx, WorkflowVolumeResize)

	requiredBytes := req.GetCapacityRange().GetRequiredBytes()
	limitBytes := req.GetCapacityRange().GetLimitBytes()

	Logc(ctx).WithFields(LogFields{
		"volumeId":      volumeId,
		"volumePath":    volumePath,
		"requiredBytes": requiredBytes,
		"limitBytes":    limitBytes,
	}).Trace("NodeExpandVolumeRequest values")

	if err := p.nodeOrchestrator.Expand(ctx, volumeId, nodecore.ExpandRequest{
		MountPath:     volumePath,
		RequiredBytes: requiredBytes,
		Secrets:       req.GetSecrets(),
	}); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, status.Error(codes.DeadlineExceeded, "NodeExpandVolume timed out")
		}
		return nil, p.getCSIErrorForOrchestratorError(err)
	}

	return &csi.NodeExpandVolumeResponse{}, nil
}

func (p *Plugin) NodeGetCapabilities(
	ctx context.Context, _ *csi.NodeGetCapabilitiesRequest,
) (res *csi.NodeGetCapabilitiesResponse, err error) {
	fields := LogFields{"Method": "NodeGetCapabilities", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Trace(">>>> NodeGetCapabilities")
	defer Logc(ctx).WithFields(fields).Trace("<<<< NodeGetCapabilities")

	return &csi.NodeGetCapabilitiesResponse{Capabilities: p.nsCap}, nil
}

func (p *Plugin) NodeGetInfo(
	ctx context.Context, _ *csi.NodeGetInfoRequest,
) (res *csi.NodeGetInfoResponse, err error) {
	fields := LogFields{"Method": "NodeGetInfo", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Trace(">>>> NodeGetInfo")
	defer Logc(ctx).WithFields(fields).Trace("<<<< NodeGetInfo")

	return &csi.NodeGetInfoResponse{
		NodeId: p.nodeName,
		AccessibleTopology: &csi.Topology{
			Segments: p.getTopologyLabels(ctx),
		},
	}, nil
}

func validateStageSANVolumeCapability(req *csi.NodeStageVolumeRequest) error {
	mountCapability := req.GetVolumeCapability().GetMount()
	blockCapability := req.GetVolumeCapability().GetBlock()

	if mountCapability == nil && blockCapability == nil {
		return status.Error(codes.InvalidArgument, "mount or block capability required")
	}

	fsType := req.PublishContext["filesystemType"]
	if mountCapability != nil && mountCapability.GetFsType() != "" {
		fsType = mountCapability.GetFsType()
	}

	if fsType == filesystem.Raw && mountCapability != nil {
		return status.Error(codes.InvalidArgument, "mount capability requested with raw blocks")
	}

	if fsType != filesystem.Raw && blockCapability != nil {
		return status.Error(codes.InvalidArgument, fmt.Sprintf("block capability requested with %s", fsType))
	}

	return nil
}

func hasSingleNodeSingleWriterAccessMode(req *csi.NodePublishVolumeRequest) bool {
	accessMode := req.GetVolumeCapability().GetAccessMode()
	return accessMode != nil &&
		accessMode.GetMode() == csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER
}
