// Copyright 2025 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/spf13/afero"
	"go.uber.org/multierr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/pkg/locks"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/devices"
	"github.com/netapp/trident/utils/devices/luks"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/fcp"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/nvme"
	"github.com/netapp/trident/utils/osutils"
	"github.com/netapp/trident/utils/smb"
)

const (
	tridentDeviceInfoPath           = "/var/lib/trident/tracking"
	nodeLockID                      = "csi_node_server"
	AttachISCSIVolumeTimeoutShort   = 20 * time.Second
	AttachFCPVolumeTimeoutShort     = 20 * time.Second
	iSCSINodeUnstageMaxDuration     = 15 * time.Second
	iSCSILoginTimeout               = 10 * time.Second
	iSCSISelfHealingLockContext     = "ISCSISelfHealingThread"
	iSCSISelfHealingTimeout         = 60 * time.Second
	fcpNodeUnstageMaxDuration       = 15 * time.Second
	nvmeSelfHealingLockContext      = "NVMeSelfHealingThread"
	defaultNodeReconciliationPeriod = 1 * time.Minute
	maximumNodeReconciliationJitter = 5000 * time.Millisecond
	nvmeMaxFlushWaitDuration        = 6 * time.Minute
	fsUnavailableTimeout            = 5 * time.Second

	// Node Scalability constants.
	maxNodeStageNFSVolumeOperations     = 10
	maxNodeStageSMBVolumeOperations     = 10
	maxNodeUnstageNFSVolumeOperations   = 10
	maxNodeUnstageSMBVolumeOperations   = 10
	maxNodePublishNFSVolumeOperations   = 10
	maxNodePublishSMBVolumeOperations   = 10
	maxNodeUnpublishVolumeOperations    = 10
	maxNodeStageISCSIVolumeOperations   = 5
	maxNodeUnstageISCSIVolumeOperations = 10
	maxNodePublishISCSIVolumeOperations = 10
	maxNodeStageFCPVolumeOperations     = 5
	maxNodeUnstageFCPVolumeOperations   = 10
	maxNodePublishFCPVolumeOperations   = 10
	maxNodeExpandVolumeOperations       = 10

	NodeStageNFSVolume   = "NodeStageNFSVolume"
	NodeStageSMBVolume   = "NodeStageSMBVolume"
	NodeUnstageNFSVolume = "NodeUnstageNFSVolume"
	NodeUnstageSMBVolume = "NodeUnstageSMBVolume"
	NodePublishNFSVolume = "NodePublishNFSVolume"
	NodePublishSMBVolume = "NodePublishSMBVolume"

	// iSCSI Constants
	NodeStageISCSIVolume   = "NodeStageISCSIVolume"
	NodeUnstageISCSIVolume = "NodeUnstageISCSIVolume"
	NodePublishISCSIVolume = "NodePublishISCSIVolume"

	// FCP Constants
	NodeStageFCPVolume   = "NodeStageFCPVolume"
	NodeUnstageFCPVolume = "NodeUnstageFCPVolume"
	NodePublishFCPVolume = "NodePublishFCPVolume"

	NodeUnpublishVolume = "NodeUnpublishVolume"
	NodeExpandVolume    = "NodeExpandVolume"

	// LockID Constant for the self-healing global lock.
	iSCSISelfHealingSessionLock = "iSCSISelfHealingSessionLock"
)

var (
	// TODO (pshashan): Unify both csiNodeLockTimeout and csiKubeletTimeout
	csiNodeLockTimeout = 60 * time.Second
	csiKubeletTimeout  = 110 * time.Second

	topologyLabels = make(map[string]string)
	iscsiUtils     = iscsi.IscsiUtils
	fcpUtils       = utils.FcpUtils

	publishedISCSISessions, currentISCSISessions *models.ISCSISessions
	publishedNVMeSessions, currentNVMeSessions   nvme.NVMeSessions

	// NVMeNamespacesFlushRetry - Non-persistent map of Namespaces to maintain the flush errors if any.
	// During NodeUnstageVolume, Trident shall return success after specific wait time (nvmeMaxFlushWaitDuration).
	NVMeNamespacesFlushRetry = make(map[string]time.Time)

	betweenAttachAndLUKSPassphrase = fiji.Register("betweenAttachAndLUKSPassphrase", "node_server")
	duringIscsiLogout              = fiji.Register("duringIscsiLogout", "node_server")
	afterInitialTrackingInfoWrite  = fiji.Register("afterInitialTrackingInfoWrite", "node_server")
	afterNvmeLuksDeviceClosed      = fiji.Register("afterNvmeLuksDeviceClosed", "node_server")
	afterNvmeDisconnect            = fiji.Register("afterNvmeDisconnect", "node_server")
	beforeTrackingInfoWrite        = fiji.Register("beforeTrackingInfoWrite", "node_server")

	// TODO(pshashan): Once the locks package revamp PR is merged, remove both  of these var below.
	iSCSISelfHealingLock           = sync.RWMutex{}
	iSCSINodeOperationWaitingCount atomic.Int32
)

const (
	removeMultipathDeviceMappingRetries    = 4
	removeMultipathDeviceMappingRetryDelay = 500 * time.Millisecond
)

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

func (p *Plugin) NodeStageVolume(
	ctx context.Context, req *csi.NodeStageVolumeRequest,
) (*csi.NodeStageVolumeResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowNodeStage)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)
	ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
	defer cancel()

	fields := LogFields{"Method": "NodeStageVolume", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Debug(">>>> NodeStageVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NodeStageVolume")

	lockContext := "NodeStageVolume"
	defer locks.Unlock(ctx, lockContext, req.GetVolumeId())
	if !attemptLock(ctx, lockContext, req.GetVolumeId(), csiNodeLockTimeout) {
		return nil, status.Error(codes.Aborted, "request waited too long for the lock")
	}

	var resp *csi.NodeStageVolumeResponse
	var err error

	switch req.PublishContext["protocol"] {
	case string(tridentconfig.File):
		if req.PublishContext["filesystemType"] == smb.SMB {
			resp, err = p.nodeStageSMBVolume(ctx, req)
		} else {
			resp, err = p.nodeStageNFSVolume(ctx, req)
		}
	case string(tridentconfig.Block):
		return p.nodeStageSANVolume(ctx, req)
	default:
		return nil, status.Error(codes.InvalidArgument, "unknown protocol")
	}

	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, status.Error(codes.DeadlineExceeded, "nodeStageVolume timed out")
		}
		return nil, err
	}
	return resp, nil
}

func (p *Plugin) NodeUnstageVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest,
) (*csi.NodeUnstageVolumeResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowNodeUnstage)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)
	ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
	defer cancel()

	return p.nodeUnstageVolume(ctx, req, false)
}

// nodeUnstageVolume detaches a volume from a node. Setting force=true may cause data loss.
func (p *Plugin) nodeUnstageVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest, force bool,
) (*csi.NodeUnstageVolumeResponse, error) {
	fields := LogFields{
		"Method": "NodeUnstageVolume",
		"Type":   "CSI_Node",
		"Force":  force,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> NodeUnstageVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NodeUnstageVolume")

	lockContext := "NodeUnstageVolume"
	defer locks.Unlock(ctx, lockContext, req.GetVolumeId())
	if !attemptLock(ctx, lockContext, req.GetVolumeId(), csiNodeLockTimeout) {
		return nil, status.Error(codes.Aborted, "request waited too long for the lock")
	}

	// Empty strings for either of these arguments are required by CSI Sanity to return an error.
	if req.VolumeId == "" {
		msg := "nodeUnstageVolume was called, but no VolumeID was provided in the request"
		return nil, status.Error(codes.InvalidArgument, msg)
	}

	if req.StagingTargetPath == "" {
		msg := "nodeUnstageVolume was called, but no StagingTargetPath was provided in the request"
		return nil, status.Error(codes.InvalidArgument, msg)
	}

	trackingInfo, err := p.nodeHelper.ReadTrackingInfo(ctx, req.VolumeId)
	if err != nil {
		if errors.IsNotFoundError(err) {
			// If the file doesn't exist, there is nothing we can do and a retry is unlikely to help, so return success.
			Logc(ctx).WithFields(fields).Warning("Volume tracking info file not found, returning success.")
			return &csi.NodeUnstageVolumeResponse{}, nil
		} else {
			file := path.Join(tridentconfig.VolumeTrackingInfoPath, req.VolumeId+".json")
			if errors.IsInvalidJSONError(err) {
				errMsgTemplate := "The volume tracking file is not readable because it was not valid JSON: %s ."
				Logc(ctx).WithFields(fields).WithError(err).Errorf(errMsgTemplate, file)
			} else {
				Logc(ctx).WithFields(fields).WithError(err).Errorf("Unable to read the volume tracking file %s", file)
			}
			return nil, status.Errorf(codes.Internal, "unable to read the volume tracking file %s; %s", file, err)
		}
	}

	publishInfo := &trackingInfo.VolumePublishInfo

	protocol, err := getVolumeProtocolFromPublishInfo(publishInfo)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "unable to read protocol info from publish info; %s", err)
	}

	var resp *csi.NodeUnstageVolumeResponse
	switch protocol {
	case tridentconfig.File:
		if publishInfo.FilesystemType == smb.SMB {
			if force {
				Logc(ctx).WithFields(fields).WithField("protocol", tridentconfig.File).
					Warning("forced unstage not supported for this protocol")
			}
			resp, err = p.nodeUnstageSMBVolume(ctx, req)
		} else {
			resp, err = p.nodeUnstageNFSVolume(ctx, req)
		}

	case tridentconfig.Block:
		if publishInfo.SANType == sa.NVMe {
			return p.nodeUnstageNVMeVolume(ctx, req, publishInfo, force)
		} else if publishInfo.SANType == sa.FCP {
			return p.nodeUnstageFCPVolumeRetry(ctx, req, publishInfo, force)
		}
		return p.nodeUnstageISCSIVolumeRetry(ctx, req, publishInfo, force)
	default:
		return nil, status.Error(codes.InvalidArgument, "unknown protocol")
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, status.Error(codes.DeadlineExceeded, "nodeUnstageVolume timed out")
		}
		return nil, err
	}
	return resp, nil
}

func (p *Plugin) NodePublishVolume(
	ctx context.Context, req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowNodePublish)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)
	ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
	defer cancel()

	fields := LogFields{"Method": "NodePublishVolume", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Debug(">>>> NodePublishVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NodePublishVolume")

	lockContext := "NodePublishVolume"
	defer locks.Unlock(ctx, lockContext, req.GetVolumeId())
	if !attemptLock(ctx, lockContext, req.GetVolumeId(), csiNodeLockTimeout) {
		return nil, status.Error(codes.Aborted, "request waited too long for the lock")
	}

	var resp *csi.NodePublishVolumeResponse
	var err error

	switch req.PublishContext["protocol"] {
	case string(tridentconfig.File):
		var trackingInfo *models.VolumeTrackingInfo
		trackingInfo, err = p.nodeHelper.ReadTrackingInfo(ctx, req.VolumeId)
		if err != nil {
			if errors.IsNotFoundError(err) {
				return nil, status.Error(codes.FailedPrecondition, err.Error())
			} else {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}
		if trackingInfo.VolumePublishInfo.FilesystemType == smb.SMB {
			resp, err = p.nodePublishSMBVolume(ctx, req)
		} else {
			resp, err = p.nodePublishNFSVolume(ctx, req)
		}
	case string(tridentconfig.Block):
		if req.PublishContext["SANType"] == sa.NVMe {
			return p.nodePublishNVMeVolume(ctx, req)
		} else if req.PublishContext["SANType"] == sa.FCP {
			return p.nodePublishFCPVolume(ctx, req)
		}
		return p.nodePublishISCSIVolume(ctx, req)
	default:
		return nil, status.Error(codes.InvalidArgument, "unknown protocol")
	}

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, status.Error(codes.DeadlineExceeded, "nodePublishVolume timed out")
		}
		return nil, err
	}
	return resp, nil
}

func (p *Plugin) NodeUnpublishVolume(
	ctx context.Context, req *csi.NodeUnpublishVolumeRequest,
) (*csi.NodeUnpublishVolumeResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowNodeUnpublish)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)
	ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
	defer cancel()

	fields := LogFields{"Method": "NodeUnpublishVolume", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Debug(">>>> NodeUnpublishVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NodeUnpublishVolume")

	lockContext := "NodeUnpublishVolume"
	defer locks.Unlock(ctx, lockContext, req.GetVolumeId())
	if !attemptLock(ctx, lockContext, req.GetVolumeId(), csiNodeLockTimeout) {
		return nil, status.Error(codes.Aborted, "request waited too long for the lock")
	}

	if err := p.limiterSharedMap[NodeUnpublishVolume].Wait(ctx); err != nil {
		return nil, err
	}
	defer p.limiterSharedMap[NodeUnpublishVolume].Release(ctx)

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "no volume ID provided")
	}

	targetPath := req.GetTargetPath()

	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "no target path provided")
	}

	isDir, err := p.osutils.IsLikelyDir(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			Logc(ctx).WithFields(fields).Infof("target path (%s) not found; volume is not mounted.", targetPath)
			return &csi.NodeUnpublishVolumeResponse{}, nil
		} else {
			return nil, status.Errorf(codes.Internal,
				"could not check if the target path (%s) is a directory; %s", targetPath, err)
		}
	}

	var notMountPoint bool
	if isDir {
		notMountPoint, err = p.mount.IsLikelyNotMountPoint(ctx, targetPath)
	} else {
		var mounted bool
		mounted, err = p.mount.IsMounted(ctx, "", targetPath, "")
		notMountPoint = !mounted
	}

	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, "target path not found")
		} else {
			return nil, status.Errorf(codes.Internal, "unable to check if targetPath (%s) is mounted; %s",
				targetPath, err)
		}
	}

	if notMountPoint {
		Logc(ctx).Debug("Volume not mounted, proceeding to unpublish volume")
	} else {
		if err = p.mount.Umount(ctx, targetPath); err != nil {
			Logc(ctx).WithFields(LogFields{"path": targetPath, "error": err}).Error("unable to unmount volume.")
			return nil, status.Errorf(codes.InvalidArgument, "unable to unmount volume; %s", err)
		}
	}

	// As per the CSI spec SP i.e. Trident is responsible for deleting the target path,
	// however today Kubernetes performs this deletion. Here we are making best efforts
	// to delete the resource at target path. Sometimes this fails resulting CSI calling
	// NodeUnpublishVolume again and usually deletion goes through in the second attempt.
	if err = p.osutils.DeleteResourceAtPath(ctx, targetPath); err != nil {
		Logc(ctx).Debugf("Unable to delete resource at target path: %s; %s", targetPath, err)
	}

	err = p.nodeHelper.RemovePublishedPath(ctx, req.VolumeId, targetPath)
	if err != nil {
		fmtStr := "could not remove published path (%s) from volume tracking file for volume %s: %s"

		if errors.IsNotFoundError(err) {
			warnStr := fmt.Sprintf(fmtStr, targetPath, req.VolumeId, err.Error())
			Logc(ctx).WithFields(LogFields{"targetPath": targetPath, "volumeId": req.VolumeId}).Warning(warnStr)
			return &csi.NodeUnpublishVolumeResponse{}, nil
		}

		return nil, status.Errorf(codes.Internal, fmtStr, targetPath, req.VolumeId, err)
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (p *Plugin) NodeGetVolumeStats(
	ctx context.Context, req *csi.NodeGetVolumeStatsRequest,
) (*csi.NodeGetVolumeStatsResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowVolumeGetStats)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume id provided")
	}

	if req.GetVolumePath() == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume path provided")
	}

	// Ensure volume is published at path
	exists, err := p.osutils.PathExistsWithTimeout(ctx, req.GetVolumePath(), fsUnavailableTimeout)
	if !exists || err != nil {
		return nil, status.Error(codes.NotFound,
			fmt.Sprintf("could not find volume mount at path: %s; %v", req.GetVolumePath(), err))
	}

	// If raw block volume, don't return usage.
	isRawBlock := false
	if req.StagingTargetPath != "" {
		trackingInfo, err := p.nodeHelper.ReadTrackingInfo(ctx, req.VolumeId)
		if err != nil {
			if errors.IsNotFoundError(err) {
				return nil, status.Error(codes.FailedPrecondition, err.Error())
			} else {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}
		publishInfo := &trackingInfo.VolumePublishInfo

		isRawBlock = publishInfo.FilesystemType == filesystem.Raw
	}
	if isRawBlock {
		// Return no capacity info for raw block volumes, we cannot reliably determine the capacity.
		return &csi.NodeGetVolumeStatsResponse{}, nil
	} else {
		// If filesystem, return usage reported by FS.
		available, capacity, usage, inodes, inodesFree, inodesUsed, err := p.fs.GetFilesystemStats(
			ctx, req.GetVolumePath())
		if err != nil {
			Logc(ctx).Errorf("unable to get filesystem stats at path: %s; %v", req.GetVolumePath(), err)
			return nil, status.Error(codes.Unknown, "Failed to get filesystem stats")
		}
		return &csi.NodeGetVolumeStatsResponse{
			Usage: []*csi.VolumeUsage{
				{
					Unit:      csi.VolumeUsage_BYTES,
					Available: available,
					Total:     capacity,
					Used:      usage,
				},
				{
					Unit:      csi.VolumeUsage_INODES,
					Available: inodesFree,
					Total:     inodes,
					Used:      inodesUsed,
				},
			},
		}, nil
	}
}

// NodeExpandVolume handles volume expansion for Block (i.e. iSCSI) volumes.  The CO only calls NodeExpandVolume
// for the Block protocol as the filesystem has to be mounted to perform the resize. This is enforced in our
// ControllerExpandVolume method where we return true for nodeExpansionRequired when the protocol is Block and
// return false when the protocol is file.
func (p *Plugin) NodeExpandVolume(
	ctx context.Context, req *csi.NodeExpandVolumeRequest,
) (*csi.NodeExpandVolumeResponse, error) {
	fields := LogFields{"Method": "NodeExpandVolume", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Debug(">>>> NodeExpandVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NodeExpandVolume")

	ctx = SetContextWorkflow(ctx, WorkflowVolumeResize)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)
	ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
	defer cancel()

	if err := p.limiterSharedMap[NodeExpandVolume].Wait(ctx); err != nil {
		return nil, err
	}
	defer p.limiterSharedMap[NodeExpandVolume].Release(ctx)

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

	// VolumePublishInfo is stored in the volume tracking file as of Trident 23.01.
	trackingInfo, err := p.nodeHelper.ReadTrackingInfo(ctx, volumeId)
	if err != nil {
		if errors.IsNotFoundError(err) {
			msg := fmt.Sprintf("unable to find tracking file for volume: %s ; needed it for resize", volumeId)
			return nil, status.Error(codes.NotFound, msg)
		}

		return nil, status.Error(codes.Internal, err.Error())
	}

	stagingTargetPath := trackingInfo.StagingTargetPath

	// Current K8S behavior is to send the volumePath as the stagingTargetPath. Log what is received if the
	// two variables don't match.
	if stagingTargetPath != volumePath {
		Logc(ctx).WithFields(LogFields{
			"stagingTargetPath": stagingTargetPath,
			"volumePath":        volumePath,
			"volumeId":          volumeId,
		}).Warn("Received something other than the expected stagingTargetPath.")
	}

	{
		// TODO(pshashan): Remove this POC once the NVMe protocol is parallelized.
		// It currently enables parallelization for the iSCSI and FCP protocol while keeping NVMe serialized.
		protocol, err := getVolumeProtocolFromPublishInfo(&trackingInfo.VolumePublishInfo)
		if err != nil {
			return nil, status.Errorf(codes.FailedPrecondition, "unable to read protocol info from publish info; %s", err)
		}
		lockID := req.GetVolumeId()
		lockContext := "NodeExpandVolume"
		if protocol == tridentconfig.Block {
			switch trackingInfo.VolumePublishInfo.SANType {
			case sa.NVMe:
				lockID = nodeLockID
				lockContext = "NodeExpandVolume-" + req.GetVolumeId()
			}
		}
		defer locks.Unlock(ctx, lockContext, lockID)
		if !attemptLock(ctx, lockContext, lockID, csiNodeLockTimeout) {
			return nil, status.Error(codes.Aborted, "request waited too long for the lock")
		}
	}

	err = p.nodeExpandVolume(ctx, &trackingInfo.VolumePublishInfo, requiredBytes, stagingTargetPath, volumeId,
		req.GetSecrets())
	if err != nil {
		return nil, err
	}
	return &csi.NodeExpandVolumeResponse{}, nil
}

func (p *Plugin) nodeExpandVolume(
	ctx context.Context, publishInfo *models.VolumePublishInfo, requiredBytes int64, stagingTargetPath string,
	volumeId string, secrets map[string]string,
) error {
	protocol, err := getVolumeProtocolFromPublishInfo(publishInfo)
	if err != nil {
		return status.Errorf(codes.FailedPrecondition, "unable to read protocol info from publish info; %s", err)
	}

	var fsType, mountOptions string

	switch protocol {
	case tridentconfig.File:
		Logc(ctx).WithField("volumeId", volumeId).Info("Filesystem expansion check is not required.")
		return nil
	case tridentconfig.Block:
		if fsType, err = filesystem.VerifyFilesystemSupport(publishInfo.FilesystemType); err != nil {
			break
		}
		// We don't need to rescan mount devices for NVMe protocol backend. Automatic namespace rescanning happens
		// everytime the NVMe controller is reset, or if the controller posts an asynchronous event indicating
		// namespace attributes have changed.
		switch publishInfo.SANType {
		case sa.NVMe:
			Logc(ctx).WithField("volumeId", volumeId).Info("NVMe volume expansion check is not required.")
		case sa.FCP:
			err = p.nodePrepareFCPVolumeForExpansion(ctx, publishInfo, requiredBytes)
		default:
			err = p.nodePrepareISCSIVolumeForExpansion(ctx, publishInfo, requiredBytes)
		}

		mountOptions = publishInfo.MountOptions
	default:
		return status.Error(codes.InvalidArgument, "unknown protocol")
	}

	if err != nil {
		Logc(ctx).WithField("devicePath", publishInfo.DevicePath).WithError(err).Error(
			"Unable to expand volume")
		return err
	}

	devicePath := publishInfo.DevicePath
	if convert.ToBool(publishInfo.LUKSEncryption) {
		if !luks.IsLegacyDevicePath(devicePath) {
			devicePath, err = p.devices.GetLUKSDevicePathForVolume(ctx, volumeId)
			if err != nil {
				Logc(ctx).WithFields(LogFields{
					"volumeId":      volumeId,
					"publishedPath": publishInfo.DevicePath,
				}).WithError(err).Error("Failed to get LUKS device path for volume.")
				return status.Error(codes.Internal, err.Error())
			}
		}
		Logc(ctx).WithField("volumeId", volumeId).Info("Resizing the LUKS mapping.")

		// Refresh the LUKS device.
		// "cryptsetup resize <luks-device-path> << <passphrase>"
		passphrase, ok := secrets["luks-passphrase"]
		if !ok {
			return status.Error(codes.InvalidArgument, "cannot expand LUKS encrypted volume; no passphrase provided")
		} else if passphrase == "" {
			return status.Error(codes.InvalidArgument, "cannot expand LUKS encrypted volume; empty passphrase provided")
		}

		luksDevice := luks.NewDetailed("", filepath.Base(devicePath), p.command, p.devices, afero.NewOsFs())
		if err := luksDevice.Resize(ctx, passphrase); err != nil {
			if errors.IsIncorrectLUKSPassphraseError(err) {
				return status.Error(codes.InvalidArgument, err.Error())
			}
			return status.Error(codes.Internal, err.Error())
		}
	}

	// Expand filesystem.
	if fsType != filesystem.Raw {
		filesystemSize, err := p.fs.ExpandFilesystemOnNode(ctx, publishInfo, devicePath, stagingTargetPath, fsType,
			mountOptions)
		if err != nil {
			Logc(ctx).WithFields(LogFields{
				"device":         publishInfo.DevicePath,
				"filesystemType": fsType,
			}).WithError(err).Error("Unable to expand filesystem.")
			return status.Error(codes.Internal, err.Error())
		}
		Logc(ctx).WithFields(LogFields{
			"filesystemSize": filesystemSize,
			"requiredBytes":  requiredBytes,
		}).Debug("Filesystem size after expand.")
	}

	Logc(ctx).WithField("volumeId", volumeId).Info("Filesystem expansion completed.")
	return nil
}

// nodePrepareFCPVolumeForExpansion readies volume expansion for FCP volumes
func (p *Plugin) nodePrepareFCPVolumeForExpansion(
	ctx context.Context, publishInfo *models.VolumePublishInfo, requiredBytes int64,
) error {
	lunID := int(publishInfo.FCPLunNumber)

	Logc(ctx).WithFields(LogFields{
		"targetWWNN":     publishInfo.FCTargetWWNN,
		"lunID":          lunID,
		"devicePath":     publishInfo.DevicePath,
		"mountOptions":   publishInfo.MountOptions,
		"filesystemType": publishInfo.FilesystemType,
	}).Debug("PublishInfo for block device to expand.")

	var err error

	// Make sure device is ready.
	if p.fcp.IsAlreadyAttached(ctx, lunID, publishInfo.FCTargetWWNN) {
		// Rescan device to detect increased size.
		if err = p.fcp.RescanDevices(
			ctx, publishInfo.FCTargetWWNN, publishInfo.FCPLunNumber, requiredBytes); err != nil {
			Logc(ctx).WithField("device", publishInfo.DevicePath).WithError(err).
				Error("Unable to scan device.")
			err = status.Error(codes.Internal, err.Error())
		}
	} else {
		err = fmt.Errorf("device %s to expand is not attached", publishInfo.DevicePath)
		Logc(ctx).WithField("devicePath", publishInfo.DevicePath).WithError(err).Error(
			"Unable to expand volume.")
		return status.Error(codes.Internal, err.Error())
	}
	return err
}

// nodePrepareISCSIVolumeForExpansion readies volume expansion for Block (i.e. iSCSI) volumes
func (p *Plugin) nodePrepareISCSIVolumeForExpansion(
	ctx context.Context, publishInfo *models.VolumePublishInfo, requiredBytes int64,
) error {
	lunID := int(publishInfo.IscsiLunNumber)

	Logc(ctx).WithFields(LogFields{
		"targetIQN":      publishInfo.IscsiTargetIQN,
		"lunID":          lunID,
		"devicePath":     publishInfo.DevicePath,
		"mountOptions":   publishInfo.MountOptions,
		"filesystemType": publishInfo.FilesystemType,
	}).Debug("PublishInfo for block device to expand.")

	var err error

	// Make sure device is ready.
	if p.iscsi.IsAlreadyAttached(ctx, lunID, publishInfo.IscsiTargetIQN) {
		// Rescan device to detect increased size.
		if err = p.iscsi.RescanDevices(
			ctx, publishInfo.IscsiTargetIQN, publishInfo.IscsiLunNumber, requiredBytes); err != nil {
			Logc(ctx).WithField("device", publishInfo.DevicePath).WithError(err).
				Error("Unable to scan device.")
			err = status.Error(codes.Internal, err.Error())
		}
	} else {
		err = fmt.Errorf("device %s to expand is not attached", publishInfo.DevicePath)
		Logc(ctx).WithField("devicePath", publishInfo.DevicePath).WithError(err).Error(
			"Unable to expand volume.")
		return status.Error(codes.Internal, err.Error())
	}
	return err
}

func (p *Plugin) NodeGetCapabilities(
	ctx context.Context, _ *csi.NodeGetCapabilitiesRequest,
) (*csi.NodeGetCapabilitiesResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowNodeGetCapabilities)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{"Method": "NodeGetCapabilities", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Trace(">>>> NodeGetCapabilities")
	defer Logc(ctx).WithFields(fields).Trace("<<<< NodeGetCapabilities")

	return &csi.NodeGetCapabilitiesResponse{Capabilities: p.nsCap}, nil
}

func (p *Plugin) NodeGetInfo(
	ctx context.Context, _ *csi.NodeGetInfoRequest,
) (*csi.NodeGetInfoResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowNodeGetInfo)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{"Method": "NodeGetInfo", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Trace(">>>> NodeGetInfo")
	defer Logc(ctx).WithFields(fields).Trace("<<<< NodeGetInfo")

	return &csi.NodeGetInfoResponse{
		NodeId: p.nodeName,
		AccessibleTopology: &csi.Topology{
			Segments: topologyLabels,
		},
	}, nil
}

func (p *Plugin) nodeGetInfo(ctx context.Context) *models.Node {
	// Only get the host system info if we don't have the info yet.
	if p.hostInfo == nil {
		host, err := p.osutils.GetHostSystemInfo(ctx)
		if err != nil {
			p.hostInfo = &models.HostSystem{}
			Logc(ctx).WithError(err).Warn("Unable to get host system information.")
		} else {
			p.hostInfo = host
			Logc(ctx).WithFields(
				LogFields{
					"distro":  host.OS.Distro,
					"version": host.OS.Version,
				}).Debug("Discovered host info.")
		}
	}

	iscsiWWN := ""
	iscsiWWNs, err := iscsi.GetInitiatorIqns(ctx)
	if err != nil {
		Logc(ctx).WithError(err).Warn("Problem getting iSCSI initiator name.")
	} else if len(iscsiWWNs) == 0 {
		Logc(ctx).Warn("Could not find iSCSI initiator name.")
	} else {
		iscsiWWN = iscsiWWNs[0]
		Logc(ctx).WithField("IQN", iscsiWWN).Info("Discovered iSCSI initiator name.")
	}

	ips, err := p.osutils.GetIPAddresses(ctx)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Could not get IP addresses.")
	} else if len(ips) == 0 {
		Logc(ctx).Warn("Could not find any usable IP addresses.")
	} else {
		Logc(ctx).WithField("IP Addresses", ips).Info("Discovered IP addresses.")
	}

	var hostWWPNMap map[string][]string
	if hostWWPNMap, err = fcp.GetFCPInitiatorTargetMap(ctx); err != nil {
		Logc(ctx).WithError(err).Warn("Problem getting FCP host node port name association.")
	}

	// Discover active protocol services on the host.
	var services []string
	nfsActive, err := p.osutils.NFSActiveOnHost(ctx)
	if err != nil {
		Logc(ctx).WithError(err).Warn("Error discovering NFS service on host.")
	}
	if nfsActive {
		services = append(services, "NFS")
	}

	smbActive, err := osutils.SMBActiveOnHost(ctx)
	if err != nil {
		Logc(ctx).WithError(err).Warn("Error discovering SMB service on host.")
	}
	if smbActive {
		services = append(services, "SMB")
	}

	iscsiActive, err := p.iscsi.ISCSIActiveOnHost(ctx, *p.hostInfo)
	if err != nil {
		Logc(ctx).WithError(err).Warn("Error discovering iSCSI service on host.")
	}
	if iscsiActive {
		services = append(services, "iSCSI")
	}

	var nvmeNQN string
	isNVMeActive, err := p.nvmeHandler.NVMeActiveOnHost(ctx)
	if err != nil {
		Logc(ctx).WithError(err).Warn("Error discovering NVMe service on host.")
	}

	if isNVMeActive {
		services = append(services, "nvme")
		nvmeNQN, err = p.nvmeHandler.GetHostNqn(ctx)
		if err != nil {
			Logc(ctx).WithError(err).Warn("Problem getting Host NQN.")
		} else {
			Logc(ctx).WithField("NQN", nvmeNQN).Debug("Discovered NQN.")
		}
	} else {
		Logc(ctx).Info("NVMe is not active on this host.")
	}

	p.hostInfo.Services = services

	// Generate node object.
	node := &models.Node{
		Name:        p.nodeName,
		IQN:         iscsiWWN,
		NQN:         nvmeNQN,
		HostWWPNMap: hostWWPNMap,
		IPs:         ips,
		NodePrep:    nil,
		HostInfo:    p.hostInfo,
		Deleted:     false,
		// If the node is already known to exist Trident CSI Controllers persistence layer,
		// that state will be used instead. Otherwise, node state defaults to clean.
		PublicationState: models.NodeClean,
	}
	return node
}

func (p *Plugin) nodeRegisterWithController(ctx context.Context, timeout time.Duration) {
	// Assemble the node details that we will register with the controller.
	node := p.nodeGetInfo(ctx)

	// The controller may not be fully initialized by the time the node is ready to register,
	// so retry until it is responding on the back channel and we have registered the node.
	registerNode := func() error {
		nodeDetails, err := p.restClient.CreateNode(ctx, node)
		if err == nil {
			topologyLabels = nodeDetails.TopologyLabels
			node.TopologyLabels = nodeDetails.TopologyLabels

			// Assume topology to be in use only if there exists a topology label with key "topology.kubernetes.io/region" on the node.
			if len(topologyLabels) > 0 {
				val, ok := topologyLabels[K8sTopologyRegionLabel]
				fields := LogFields{"topologyLabelKey": K8sTopologyRegionLabel, "value": val}
				if ok && val != "" {
					Logc(ctx).WithFields(fields).Infof("%s label found on node. Assuming topology to be in use.", K8sTopologyRegionLabel)
					p.topologyInUse = true
				} else {
					Logc(ctx).WithFields(fields).Infof("%s label not found on node. Assuming topology not in use.", K8sTopologyRegionLabel)
					p.topologyInUse = false
				}
			}

			// Setting log level, log workflows and log layers on the node same as to what is set on the controller.
			if err = p.orchestrator.SetLogLevel(ctx, nodeDetails.LogLevel); err != nil {
				Logc(ctx).WithError(err).Error("Unable to set log level.")
			}
			if err = p.orchestrator.SetLoggingWorkflows(ctx, nodeDetails.LogWorkflows); err != nil {
				Logc(ctx).WithError(err).Error("Unable to set logging workflows.")
			}
			if err = p.orchestrator.SetLogLayers(ctx, nodeDetails.LogLayers); err != nil {
				Logc(ctx).WithError(err).Error("Unable to set logging layers.")
			}
		}
		return err
	}

	registerNodeNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"increment": duration,
			"error":     err,
		}).Warn("Could not update Trident controller with node registration, will retry.")
	}

	registerNodeBackoff := backoff.NewExponentialBackOff()
	registerNodeBackoff.InitialInterval = 10 * time.Second
	registerNodeBackoff.Multiplier = 2
	registerNodeBackoff.MaxInterval = 120 * time.Second
	registerNodeBackoff.RandomizationFactor = 0.1
	registerNodeBackoff.MaxElapsedTime = timeout

	// Retry using an exponential backoff that never ends until it succeeds.
	err := backoff.RetryNotify(registerNode, registerNodeBackoff, registerNodeNotify)
	if err != nil {
		Logc(ctx).WithError(err).Error("Unable to update Trident controller with node registration.")
	} else {
		Logc(ctx).WithField("node", p.nodeName).Info("Updated Trident controller with node registration.")
		Logc(ctx).WithField("node", p.nodeName).Debug("Topology labels found for node: ", topologyLabels)
	}

	p.nodeIsRegistered = true
}

func (p *Plugin) nodeStageNFSVolume(
	ctx context.Context, req *csi.NodeStageVolumeRequest,
) (*csi.NodeStageVolumeResponse, error) {
	Logc(ctx).Debug(">>>> nodeStageNFSVolume")
	defer Logc(ctx).Debug("<<<< nodeStageNFSVolume")

	if err := p.limiterSharedMap[NodeStageNFSVolume].Wait(ctx); err != nil {
		return nil, err
	}
	defer p.limiterSharedMap[NodeStageNFSVolume].Release(ctx)

	publishInfo := &models.VolumePublishInfo{
		Localhost:      true,
		FilesystemType: "nfs",
	}

	if err := p.mount.IsCompatible(ctx, publishInfo.FilesystemType); err != nil {
		return nil, err
	}

	publishInfo.MountOptions = req.PublishContext["mountOptions"]
	publishInfo.NfsServerIP = req.PublishContext["nfsServerIp"]
	publishInfo.NfsPath = req.PublishContext["nfsPath"]

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return nil, err
	}

	volTrackingInfo := &models.VolumeTrackingInfo{
		VolumePublishInfo: *publishInfo,
		StagingTargetPath: stagingTargetPath,
		PublishedPaths:    map[string]struct{}{},
	}

	// Save the device info to the volume tracking info path for use in the publish & unstage calls.
	if err := p.nodeHelper.WriteTrackingInfo(ctx, volumeId, volTrackingInfo); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (p *Plugin) nodeUnstageNFSVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest,
) (*csi.NodeUnstageVolumeResponse, error) {
	Logc(ctx).Debug(">>>> nodeUnstageNFSVolume")
	defer Logc(ctx).Debug("<<<< nodeUnstageNFSVolume")

	if err := p.limiterSharedMap[NodeUnstageNFSVolume].Wait(ctx); err != nil {
		return nil, err
	}
	defer p.limiterSharedMap[NodeUnstageNFSVolume].Release(ctx)

	volumeId, _, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return nil, err
	}

	// Delete the device info we saved to the volume tracking info path so unstage can succeed.
	if err := p.nodeHelper.DeleteTrackingInfo(ctx, volumeId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (p *Plugin) nodePublishNFSVolume(
	ctx context.Context, req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {
	Logc(ctx).Debug(">>>> nodePublishNFSVolume")
	defer Logc(ctx).Debug("<<<< nodePublishNFSVolume")

	if err := p.limiterSharedMap[NodePublishNFSVolume].Wait(ctx); err != nil {
		return nil, err
	}
	defer p.limiterSharedMap[NodePublishNFSVolume].Release(ctx)

	targetPath := req.GetTargetPath()
	notMnt, err := p.mount.IsLikelyNotMountPoint(ctx, targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(targetPath, 0o750); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	trackingInfo, err := p.nodeHelper.ReadTrackingInfo(ctx, req.VolumeId)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	publishInfo := &trackingInfo.VolumePublishInfo

	if req.GetReadonly() {
		publishInfo.MountOptions = collection.AppendToStringList(publishInfo.MountOptions, "ro", ",")
	}

	err = p.mount.AttachNFSVolume(ctx, req.VolumeContext["internalName"], req.TargetPath, publishInfo)
	if err != nil {
		if os.IsPermission(err) {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
		if strings.Contains(err.Error(), "invalid argument") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	err = p.nodeHelper.AddPublishedPath(ctx, req.VolumeId, req.TargetPath)
	if err != nil {
		return nil, err
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (p *Plugin) nodeStageSMBVolume(
	ctx context.Context, req *csi.NodeStageVolumeRequest,
) (*csi.NodeStageVolumeResponse, error) {
	Logc(ctx).Debug(">>>> nodeStageSMBVolume")
	defer Logc(ctx).Debug("<<<< nodeStageSMBVolume")

	if err := p.limiterSharedMap[NodeStageSMBVolume].Wait(ctx); err != nil {
		return nil, err
	}
	defer p.limiterSharedMap[NodeStageSMBVolume].Release(ctx)

	publishInfo := &models.VolumePublishInfo{
		Localhost:      true,
		FilesystemType: smb.SMB,
	}

	if err := p.mount.IsCompatible(ctx, publishInfo.FilesystemType); err != nil {
		return nil, err
	}

	// Have to check if it's windows
	publishInfo.MountOptions = req.PublishContext["mountOptions"]
	publishInfo.SMBServer = req.PublishContext["smbServer"]
	publishInfo.SMBPath = req.PublishContext["smbPath"]

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return nil, err
	}

	volTrackingInfo := &models.VolumeTrackingInfo{
		VolumePublishInfo: *publishInfo,
		StagingTargetPath: stagingTargetPath,
		PublishedPaths:    map[string]struct{}{},
	}
	// Save the device info to the volume tracking info path for use in the publish & unstage calls.
	if err := p.nodeHelper.WriteTrackingInfo(ctx, volumeId, volTrackingInfo); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Get Active directory username and password from secret.
	secrets := req.GetSecrets()
	username := secrets["username"]
	password := secrets["password"]

	// Remote map the SMB share to the staging path
	err = p.mount.AttachSMBVolume(ctx, volumeId, stagingTargetPath, username, password, publishInfo)
	if err != nil {
		if os.IsPermission(err) {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
		if strings.Contains(err.Error(), "invalid argument") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (p *Plugin) nodeUnstageSMBVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest,
) (*csi.NodeUnstageVolumeResponse, error) {
	Logc(ctx).Debug(">>>> nodeUnstageSMBVolume")
	defer Logc(ctx).Debug("<<<< nodeUnstageSMBVolume")

	if err := p.limiterSharedMap[NodeUnstageSMBVolume].Wait(ctx); err != nil {
		return nil, err
	}
	defer p.limiterSharedMap[NodeUnstageSMBVolume].Release(ctx)

	var mappingPath string

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return nil, err
	}

	trackingInfo, err := p.nodeHelper.ReadTrackingInfo(ctx, volumeId)
	if err != nil {
		return &csi.NodeUnstageVolumeResponse{}, err
	}

	mappingPath, err = p.fs.GetUnmountPath(ctx, trackingInfo)
	if err != nil {
		return nil, err
	}

	err = p.mount.UmountSMBPath(ctx, mappingPath, stagingTargetPath)

	// Delete the device info we saved to the volume tracking info path so unstage can succeed.
	if err := p.nodeHelper.DeleteTrackingInfo(ctx, volumeId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnstageVolumeResponse{}, err
}

func (p *Plugin) nodePublishSMBVolume(
	ctx context.Context, req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {
	Logc(ctx).Debug(">>>> nodePublishSMBVolume")
	defer Logc(ctx).Debug("<<<< nodePublishSMBVolume")

	if err := p.limiterSharedMap[NodePublishSMBVolume].Wait(ctx); err != nil {
		return nil, err
	}
	defer p.limiterSharedMap[NodePublishSMBVolume].Release(ctx)

	targetPath := req.GetTargetPath()
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	source := req.GetStagingTargetPath()
	if len(source) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target not provided")
	}
	notMnt, err := p.mount.IsLikelyNotMountPoint(ctx, targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(targetPath, 0o750); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}
	mountOptions := []string{"bind"}
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}
	err = p.mount.WindowsBindMount(ctx, source, targetPath, mountOptions)
	if err != nil {
		if os.IsPermission(err) {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
		if strings.Contains(err.Error(), "invalid argument") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func unstashIscsiTargetPortals(publishInfo *models.VolumePublishInfo, reqPublishInfo map[string]string) error {
	count, err := strconv.Atoi(reqPublishInfo["iscsiTargetPortalCount"])
	if nil != err {
		return err
	}
	if 1 > count {
		return fmt.Errorf("iscsiTargetPortalCount=%d may not be less than 1", count)
	}
	publishInfo.IscsiTargetPortal = reqPublishInfo["p1"]
	publishInfo.IscsiPortals = make([]string, count-1)
	for i := 1; i < count; i++ {
		key := fmt.Sprintf("p%d", i+1)
		value, ok := reqPublishInfo[key]
		if !ok {
			return fmt.Errorf("missing portal: %s", key)
		}
		publishInfo.IscsiPortals[i-1] = value
	}
	return nil
}

func (p *Plugin) populatePublishedSessions(ctx context.Context) {
	volumeIDs := iscsi.GetAllVolumeIDs(ctx, tridentDeviceInfoPath)
	for _, volumeID := range volumeIDs {
		trackingInfo, err := p.nodeHelper.ReadTrackingInfo(ctx, volumeID)
		if err != nil || trackingInfo == nil {
			Logc(ctx).WithFields(LogFields{
				"volumeID": volumeID,
				"error":    err,
				"isEmpty":  trackingInfo == nil,
			}).Error("Volume tracking file info not found or is empty.")

			continue
		}

		publishInfo := &trackingInfo.VolumePublishInfo
		if publishInfo.SANType != sa.NVMe {
			newCtx := context.WithValue(ctx, iscsi.SessionInfoSource, iscsi.SessionSourceTrackingInfo)
			p.iscsi.AddSession(newCtx, publishedISCSISessions, publishInfo, volumeID, "", models.NotInvalid)
		} else {
			p.nvmeHandler.AddPublishedNVMeSession(&publishedNVMeSessions, publishInfo)
		}

	}
}

func (p *Plugin) readAllTrackingFiles(ctx context.Context) []models.VolumePublishInfo {
	publishInfos := make([]models.VolumePublishInfo, 0)
	volumeIDs := iscsi.GetAllVolumeIDs(ctx, tridentDeviceInfoPath)
	for _, volumeID := range volumeIDs {
		trackingInfo, err := p.nodeHelper.ReadTrackingInfo(ctx, volumeID)
		if err != nil || trackingInfo == nil {
			Logc(ctx).WithError(err).WithFields(LogFields{
				"volumeID": volumeID,
				"isEmpty":  trackingInfo == nil,
			}).Error("Volume tracking file info not found or is empty.")

			continue
		}

		publishInfos = append(publishInfos, trackingInfo.VolumePublishInfo)
	}

	return publishInfos
}

func (p *Plugin) nodeStageFCPVolume(
	ctx context.Context, req *csi.NodeStageVolumeRequest, publishInfo *models.VolumePublishInfo,
) (err error) {
	Logc(ctx).Debug(">>>> nodeStageFCPVolume")
	defer Logc(ctx).Debug("<<<< nodeStageFCPVolume")

	if err = p.limiterSharedMap[NodeStageFCPVolume].Wait(ctx); err != nil {
		return err
	}
	defer p.limiterSharedMap[NodeStageFCPVolume].Release(ctx)

	var lunID int32
	lunID, err = convert.ToPositiveInt32(req.PublishContext["fcpLunNumber"])
	if err != nil {
		return err
	}

	isLUKS := convert.ToBool(req.PublishContext["LUKSEncryption"])
	publishInfo.LUKSEncryption = strconv.FormatBool(isLUKS)

	publishInfo.MountOptions = req.PublishContext["mountOptions"]
	publishInfo.FCTargetWWNN = req.PublishContext["fcTargetWWNN"]
	publishInfo.FormatOptions = req.PublishContext["formatOptions"]
	publishInfo.FCPLunNumber = lunID
	publishInfo.FCPLunSerial = req.PublishContext["fcpLunSerial"]
	publishInfo.FCPIgroup = req.PublishContext["fcpIgroup"]
	publishInfo.SANType = req.PublishContext["SANType"]

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return err
	}

	volTrackingInfo := &models.VolumeTrackingInfo{
		VolumePublishInfo: *publishInfo,
		StagingTargetPath: stagingTargetPath,
		PublishedPaths:    map[string]struct{}{},
	}
	if err := p.nodeHelper.WriteTrackingInfo(ctx, volumeId, volTrackingInfo); err != nil {
		Logc(ctx).WithFields(LogFields{
			"volumeID":          volumeId,
			"stagingTargetPath": stagingTargetPath,
		}).WithError(err).Error("Could not write tracking file.")
		return err
	}

	if err := afterInitialTrackingInfoWrite.Inject(); err != nil {
		return err
	}
	// In the case of a failed CSI NodeStageVolume call, CSI node clients may call NodeStageVolume or NodeUnstageVolume.
	// To ensure Trident can handle a subsequent CSI NodeUnstageVolume call, Trident always writes a tracking file.
	// This should result in Trident having all it needs to for CSI NodeUnstageVolume should an attachment fail.
	defer func() {
		// Always write a volume tracking info for use in node publish & unstage calls.
		volTrackingInfo := &models.VolumeTrackingInfo{
			VolumePublishInfo: *publishInfo,
			StagingTargetPath: stagingTargetPath,
			PublishedPaths:    map[string]struct{}{},
		}
		if fileErr := p.nodeHelper.WriteTrackingInfo(ctx, volumeId, volTrackingInfo); fileErr != nil {
			Logc(ctx).WithFields(LogFields{
				"volumeID":          volumeId,
				"stagingTargetPath": stagingTargetPath,
			}).WithError(fileErr).Error("Could not write tracking file.")

			// If an attachment error exists, then we should capture that failure along with a write file error.
			if err != nil {
				err = fmt.Errorf("attachment failed: %v; could not write tracking file: %v", err, fileErr)
			} else {
				err = fmt.Errorf("could not write tracking file: %v", fileErr)
			}
		}
	}()

	var mpathSize int64
	mpathSize, err = p.ensureAttachFCPVolume(ctx, req, "", publishInfo, AttachFCPVolumeTimeoutShort)
	if err != nil {
		return err
	}

	if isLUKS {
		if err := betweenAttachAndLUKSPassphrase.Inject(); err != nil {
			return err
		}

		var luksDevice luks.Device
		luksDevice = luks.NewDevice(publishInfo.DevicePath, req.VolumeContext["internalName"], p.command)

		// Ensure we update the passphrase in case it has never been set before
		err = ensureLUKSVolumePassphrase(ctx, p.restClient, luksDevice, volumeId, req.GetSecrets(), true)
		if err != nil {
			return fmt.Errorf("could not set LUKS volume passphrase")
		}
	}
	if mpathSize > 0 {
		Logc(ctx).Warn("Multipath device size may not be correct, performing gratuitous resize.")

		err = p.nodeExpandVolume(ctx, publishInfo, mpathSize, stagingTargetPath, volumeId, req.GetSecrets())
		if err != nil {
			Logc(ctx).WithFields(LogFields{
				"multipathDevice": publishInfo.DevicePath,
				"volumeID":        volumeId,
				"size":            mpathSize,
				"err":             err,
			}).Warn("Attempt to perform gratuitous resize failed.")
		}
	}

	return nil
}

// ensureAttachFCPVolume attempts to attach the volume to the local host
// with a retry logic based on the publish information passed in.
func (p *Plugin) ensureAttachFCPVolume(
	ctx context.Context, req *csi.NodeStageVolumeRequest, mountpoint string,
	publishInfo *models.VolumePublishInfo, attachTimeout time.Duration,
) (int64, error) {
	var err error
	var mpathSize int64

	Logc(ctx).Debug(">>>> ensureAttachFCPVolume")
	defer Logc(ctx).Debug("<<<< ensureAttachFCPVolume")

	// Perform the login/rescan/discovery/(optionally)format, mount & get the device back in the publish info
	if mpathSize, err = p.fcp.AttachVolumeRetry(ctx, req.VolumeContext["internalName"], mountpoint,
		publishInfo, req.GetSecrets(), attachTimeout); err != nil {
		return mpathSize, status.Error(codes.Internal, fmt.Sprintf("failed to stage volume: %v", err))
	}

	return mpathSize, nil
}

func (p *Plugin) nodeUnstageFCPVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest, publishInfo *models.VolumePublishInfo, force bool,
) error {
	hostSessionMap := fcpUtils.GetFCPHostSessionMapForTarget(ctx, publishInfo.FCTargetWWNN)
	paths := fcpUtils.GetSysfsBlockDirsForLUN(int(publishInfo.FCPLunNumber), hostSessionMap)
	if deviceNames, err := fcpUtils.GetDevicesForLUN(paths); err != nil {
		return fmt.Errorf("could not get devices for LUN: %v", err)
	} else if len(deviceNames) == 0 {
		// If we are in this block it likely means we have errored or had a pod restart
		// before the tracking file has been removed. We need to ensure the device was removed and remove the tracking
		// file, without going through the rest of the unstage process.
		if convert.ToBool(publishInfo.LUKSEncryption) {
			var err error
			var luksMapperPath string
			fields := LogFields{"device": publishInfo.DevicePath}
			// Set device path to dm device to correctly verify legacy volumes.
			if luks.IsLegacyDevicePath(publishInfo.DevicePath) {
				luksMapperPath = publishInfo.DevicePath
				dmPath, err := luks.GetDmDevicePathFromLUKSLegacyPath(ctx, p.command,
					publishInfo.DevicePath)
				if err != nil {
					Logc(ctx).WithFields(fields).WithError(err).Warn(
						"Could not determine dm device path from legacy LUKS device path. " +
							"Continuing with device removal.")
				} else {
					publishInfo.DevicePath = dmPath
				}
			} else {
				// If not using luks legacy device path we need to find the LUKS mapper device.
				luksMapperPath, err = p.devices.GetLUKSDevicePathForVolume(ctx, req.GetVolumeId())
				if err != nil {
					// If the LUKS device is not found, the functional difference is negligible to unstage.
					// But it may be useful to log at different levels for observability.
					log := Logc(ctx).WithFields(fields).WithError(err)
					if errors.IsNotFoundError(err) {
						log.Warn("Failed to get LUKS device path for volume.")
					} else {
						log.Debug("Could not determine LUKS device path for volume.")
					}
					log.Debug("Continuing with device removal.")
				}
			}
			err = p.devices.EnsureLUKSDeviceClosedWithMaxWaitLimit(ctx, luksMapperPath)
			if err != nil {
				Logc(ctx).WithError(err).Debug("Unable to remove LUKS device. Continuing with tracking file removal.")
			}
		}
		if err := p.devices.RemoveMultipathDeviceMappingWithRetries(ctx, publishInfo.DevicePath,
			removeMultipathDeviceMappingRetries, removeMultipathDeviceMappingRetryDelay); err != nil {
			Logc(ctx).Warn("Unable to remove multipath device. Continuing with tracking file removal.")
		}
		// Ensure the tracking file is removed.
		volumeId, _, err := p.getVolumeIdAndStagingPath(req)
		if err != nil {
			return err
		}
		if err := p.nodeHelper.DeleteTrackingInfo(ctx, volumeId); err != nil {
			return status.Error(codes.Internal, err.Error())
		}
		return nil
	}

	deviceInfo, err := p.fcp.GetDeviceInfoForLUN(ctx, hostSessionMap, int(publishInfo.FCPLunNumber),
		publishInfo.FCTargetWWNN, false)
	if err != nil {
		return fmt.Errorf("could not get device info: %v", err)
	}

	if deviceInfo == nil {
		Logc(ctx).Debug("Could not find devices, nothing to do.")
		return nil
	}

	var luksMapperPath string
	// If the multipath device is not present, the LUKS device should not exist.
	if convert.ToBool(publishInfo.LUKSEncryption) && deviceInfo.MultipathDevice != "" {
		fields := LogFields{
			"luksDevicePath":  publishInfo.DevicePath,
			"lunID":           publishInfo.FCPLunNumber,
			"multipathDevice": deviceInfo.MultipathDevice,
		}

		luksMapperPath, err = p.devices.GetLUKSDevicePathForVolume(ctx, req.GetVolumeId())
		if err != nil {
			if !errors.IsNotFoundError(err) {
				Logc(ctx).WithFields(fields).WithError(err).Error("Failed to get LUKS device path from multipath device.")
				return err
			}
			Logc(ctx).WithFields(fields).Info("No LUKS device path found from multipath device.")
		}

		// Ensure the LUKS device is closed if the luksMapperPath is set.
		if luksMapperPath != "" {

			fields["mapperPath"] = luksMapperPath
			err = p.devices.EnsureLUKSDeviceClosedWithMaxWaitLimit(ctx, luksMapperPath)
			if err != nil {
				if !errors.IsMaxWaitExceededError(err) {
					Logc(ctx).WithFields(fields).WithError(err).Error("Failed to close LUKS device.")
					return err
				}
				Logc(ctx).WithFields(fields).WithError(err).
					Debug("LUKS close wait time exceeded continuing with device removal.")
			}
		}

		// Set device path to dm device to correctly verify legacy volumes.
		if luks.IsLegacyDevicePath(publishInfo.DevicePath) {
			publishInfo.DevicePath = deviceInfo.MultipathDevice
		}
	}

	// Delete the device from the host.
	unmappedMpathDevice, err := p.fcp.PrepareDeviceForRemoval(ctx, deviceInfo, publishInfo, nil, p.unsafeDetach,
		force)
	if err != nil {
		if errors.IsFCPSameLunNumberError(err) {
			// There is a need to pass all the publish infos this time
			unmappedMpathDevice, err = p.fcp.PrepareDeviceForRemoval(ctx, deviceInfo, publishInfo,
				p.readAllTrackingFiles(ctx),
				p.unsafeDetach, force)
		}

		if err != nil && !p.unsafeDetach {
			return status.Error(codes.Internal, err.Error())
		}
	}

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return err
	}

	// Ensure that the temporary mount point created during a filesystem expand operation is removed.
	if err := p.mount.UmountAndRemoveTemporaryMountPoint(ctx, stagingTargetPath); err != nil {
		Logc(ctx).WithFields(LogFields{
			"volumeId":          volumeId,
			"stagingTargetPath": stagingTargetPath,
		}).WithError(err).Errorf("Failed to remove directory in staging target path.")
		errStr := fmt.Sprintf("failed to remove temporary directory in staging target path %s; %s",
			stagingTargetPath, err.Error())
		return status.Error(codes.Internal, errStr)
	}

	// If the LUKS device still exists, it means the device was unable to be closed prior to removing the block
	// device. This can happen if the LUN was deleted or offline. It should be removable by this point.
	// It needs to be removed prior to removing the 'unmappedMpathDevice' device below.
	if luksMapperPath != "" {
		// EnsureLUKSDeviceClosed will not return an error if the device is already closed or removed.
		if err = p.devices.EnsureLUKSDeviceClosed(ctx, luksMapperPath); err != nil {
			Logc(ctx).WithFields(LogFields{
				"devicePath": luksMapperPath,
			}).WithError(err).Warning("Unable to remove LUKS mapper device.")
		}
		// Clear the time duration for the LUKS device.
		devices.LuksCloseDurations.RemoveDurationTracking(luksMapperPath)
	}

	// If there is multipath device, flush(remove) mappings
	if err := p.devices.RemoveMultipathDeviceMappingWithRetries(ctx, unmappedMpathDevice,
		removeMultipathDeviceMappingRetries, removeMultipathDeviceMappingRetryDelay); err != nil {
		return err
	}

	// Delete the device info we saved to the volume tracking info path so unstage can succeed.
	if err := p.nodeHelper.DeleteTrackingInfo(ctx, volumeId); err != nil {
		return status.Error(codes.Internal, err.Error())
	}

	return nil
}

func (p *Plugin) nodeUnstageFCPVolumeRetry(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest, publishInfo *models.VolumePublishInfo, force bool,
) (*csi.NodeUnstageVolumeResponse, error) {
	Logc(ctx).Debug(">>>> nodeUnstageFCPVolumeRetry")
	defer Logc(ctx).Debug("<<<< nodeUnstageFCPVolumeRetry")

	if err := p.limiterSharedMap[NodeUnstageFCPVolume].Wait(ctx); err != nil {
		return nil, err
	}
	defer p.limiterSharedMap[NodeUnstageFCPVolume].Release(ctx)

	nodeUnstageFCPVolumeNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Failed to unstage the volume, retrying.")
	}

	nodeUnstageFCPVolumeAttempt := func() error {
		return p.nodeUnstageFCPVolume(ctx, req, publishInfo, force)
	}

	nodeUnstageFCPVolumeBackoff := backoff.NewExponentialBackOff()
	nodeUnstageFCPVolumeBackoff.InitialInterval = 1 * time.Second
	nodeUnstageFCPVolumeBackoff.Multiplier = 1.414 // approx sqrt(2)
	nodeUnstageFCPVolumeBackoff.RandomizationFactor = 0.1
	nodeUnstageFCPVolumeBackoff.MaxElapsedTime = fcpNodeUnstageMaxDuration

	// Run the un-staging using an exponential backoff
	if err := backoff.RetryNotify(nodeUnstageFCPVolumeAttempt, nodeUnstageFCPVolumeBackoff,
		nodeUnstageFCPVolumeNotify); err != nil {
		Logc(ctx).Error("failed to unstage volume")
		return nil, err
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (p *Plugin) nodePublishFCPVolume(
	ctx context.Context, req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {
	Logc(ctx).Debug(">>>> nodePublishFCPVolume")
	defer Logc(ctx).Debug("<<<< nodePublishFCPVolume")

	if err := p.limiterSharedMap[NodePublishFCPVolume].Wait(ctx); err != nil {
		return nil, err
	}
	defer p.limiterSharedMap[NodePublishFCPVolume].Release(ctx)

	var err error

	// Read the device info from the volume tracking info path, or if that doesn't exist yet, the staging path.
	trackingInfo, err := p.nodeHelper.ReadTrackingInfo(ctx, req.VolumeId)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	publishInfo := &trackingInfo.VolumePublishInfo

	if req.GetReadonly() {
		publishInfo.MountOptions = collection.AppendToStringList(publishInfo.MountOptions, "ro", ",")
	}

	devicePath := publishInfo.DevicePath
	if convert.ToBool(publishInfo.LUKSEncryption) {
		// Rotate the LUKS passphrase if needed, on failure, log and continue to publish

		var luksDevice luks.Device
		if luks.IsLegacyDevicePath(devicePath) {
			// Supports legacy volumes that store the LUKS device path
			luksDevice, err = luks.NewDeviceFromMappingPath(
				ctx, p.command, devicePath, req.VolumeContext["internalName"],
			)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		} else {
			luksDevice = luks.NewDevice(publishInfo.DevicePath, req.VolumeContext["internalName"], p.command)
		}

		err = ensureLUKSVolumePassphrase(ctx, p.restClient, luksDevice, req.GetVolumeId(), req.GetSecrets(), false)
		if err != nil {
			Logc(ctx).WithError(err).Error("Failed to ensure current LUKS passphrase.")
		}

		// Mount LUKS device instead of mpath.
		devicePath = luksDevice.MappedDevicePath()
	}

	if publishInfo.FilesystemType == filesystem.Raw {

		if len(publishInfo.MountOptions) > 0 {
			publishInfo.MountOptions = collection.AppendToStringList(publishInfo.MountOptions, "bind", ",")
		} else {
			publishInfo.MountOptions = "bind"
		}

		// Place the block device at the target path for the raw-block.
		err = p.mount.MountDevice(ctx, devicePath, req.TargetPath, publishInfo.MountOptions, true)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "unable to bind mount raw device; %s", err)
		}
	} else {
		// Mount the device.
		err = p.mount.MountDevice(ctx, devicePath, req.TargetPath, publishInfo.MountOptions, false)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "unable to mount device; %s", err)
		}
	}

	err = p.nodeHelper.AddPublishedPath(ctx, req.VolumeId, req.TargetPath)
	if err != nil {
		return nil, err
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (p *Plugin) nodeStageISCSIVolume(
	ctx context.Context, req *csi.NodeStageVolumeRequest, publishInfo *models.VolumePublishInfo,
) (err error) {
	Logc(ctx).Debug(">>>> nodeStageISCSIVolume")
	defer Logc(ctx).Debug("<<<< nodeStageISCSIVolume")

	if err = p.limiterSharedMap[NodeStageISCSIVolume].Wait(ctx); err != nil {
		return err
	}
	defer p.limiterSharedMap[NodeStageISCSIVolume].Release(ctx)

	// TODO(pshashan): Once the locks package revamp PR is merged, remove this atomic counter.
	iSCSINodeOperationWaitingCount.Add(1)
	iSCSISelfHealingLock.RLock()
	defer iSCSISelfHealingLock.RUnlock()
	iSCSINodeOperationWaitingCount.Add(-1)

	var useCHAP bool
	useCHAP, err = strconv.ParseBool(req.PublishContext["useCHAP"])
	if err != nil {
		return err
	}
	publishInfo.UseCHAP = useCHAP

	var lunID int32
	lunID, err = convert.ToPositiveInt32(req.PublishContext["iscsiLunNumber"])
	if err != nil {
		return err
	}

	isLUKS := convert.ToBool(req.PublishContext["LUKSEncryption"])
	publishInfo.LUKSEncryption = strconv.FormatBool(isLUKS)

	err = unstashIscsiTargetPortals(publishInfo, req.PublishContext)
	if nil != err {
		return err
	}

	publishInfo.MountOptions = req.PublishContext["mountOptions"]
	publishInfo.FormatOptions = req.PublishContext["formatOptions"]
	publishInfo.IscsiTargetIQN = req.PublishContext["iscsiTargetIqn"]
	publishInfo.IscsiLunNumber = lunID
	publishInfo.IscsiLunSerial = req.PublishContext["iscsiLunSerial"]
	publishInfo.IscsiInterface = req.PublishContext["iscsiInterface"]
	publishInfo.IscsiIgroup = req.PublishContext["iscsiIgroup"]

	if useCHAP {
		publishInfo.IscsiUsername = req.PublishContext["iscsiUsername"]
		publishInfo.IscsiInitiatorSecret = req.PublishContext["iscsiInitiatorSecret"]
		publishInfo.IscsiTargetUsername = req.PublishContext["iscsiTargetUsername"]
		publishInfo.IscsiTargetSecret = req.PublishContext["iscsiTargetSecret"]

		encryptedCHAP := containsEncryptedCHAP(req.PublishContext)

		if p.aesKey != nil && encryptedCHAP {
			if err = decryptCHAPPublishInfo(ctx, publishInfo, req.PublishContext, p.aesKey); err != nil {
				Logc(ctx).WithError(err).Warn(
					"Could not decrypt CHAP credentials; will retrieve from Trident controller.")
				if err = p.updateChapInfoFromController(ctx, req, publishInfo); err != nil {
					return err
				}
			}
		} else if encryptedCHAP {
			// Only error if the key is not set and the publishContext includes encrypted fields.
			Logc(ctx).Warn(
				"Encryption key not set; cannot decrypt CHAP credentials; will retrieve from Trident controller.")
			if err = p.updateChapInfoFromController(ctx, req, publishInfo); err != nil {
				return err
			}
		}
	}

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return err
	}

	volTrackingInfo := &models.VolumeTrackingInfo{
		VolumePublishInfo: *publishInfo,
		StagingTargetPath: stagingTargetPath,
		PublishedPaths:    map[string]struct{}{},
	}
	if err := p.nodeHelper.WriteTrackingInfo(ctx, volumeId, volTrackingInfo); err != nil {
		Logc(ctx).WithFields(LogFields{
			"volumeID":          volumeId,
			"stagingTargetPath": stagingTargetPath,
		}).WithError(err).Error("Could not write tracking file.")
		return err
	}

	if err := afterInitialTrackingInfoWrite.Inject(); err != nil {
		return err
	}

	// In the case of a failed CSI NodeStageVolume call, CSI node clients may call NodeStageVolume or NodeUnstageVolume.
	// To ensure Trident can handle a subsequent CSI NodeUnstageVolume call, Trident always writes a tracking file.
	// This should result in Trident having all it needs to for CSI NodeUnstageVolume should an attachment fail.
	defer func() {
		// Always write a volume tracking info for use in node publish & unstage calls.
		volTrackingInfo := &models.VolumeTrackingInfo{
			VolumePublishInfo: *publishInfo,
			StagingTargetPath: stagingTargetPath,
			PublishedPaths:    map[string]struct{}{},
		}
		if fileErr := p.nodeHelper.WriteTrackingInfo(ctx, volumeId, volTrackingInfo); fileErr != nil {
			Logc(ctx).WithFields(LogFields{
				"volumeID":          volumeId,
				"stagingTargetPath": stagingTargetPath,
			}).WithError(fileErr).Error("Could not write tracking file.")

			// If an attachment error exists, then we should capture that failure along with a write file error.
			if err != nil {
				err = fmt.Errorf("attachment failed: %v; could not write tracking file: %v", err, fileErr)
			} else {
				err = fmt.Errorf("could not write tracking file: %v", fileErr)
			}
		}
	}()

	var mpathSize int64
	mpathSize, err = p.ensureAttachISCSIVolume(ctx, req, "", publishInfo, AttachISCSIVolumeTimeoutShort)
	if err != nil {
		return err
	}

	if isLUKS {
		if err := betweenAttachAndLUKSPassphrase.Inject(); err != nil {
			return err
		}

		var luksDevice luks.Device
		luksDevice = luks.NewDevice(publishInfo.DevicePath, req.VolumeContext["internalName"], p.command)

		// Ensure we update the passphrase in case it has never been set before
		err = ensureLUKSVolumePassphrase(ctx, p.restClient, luksDevice, volumeId, req.GetSecrets(), true)
		if err != nil {
			return fmt.Errorf("could not set LUKS volume passphrase")
		}
	}

	if mpathSize > 0 {
		Logc(ctx).Warn("Multipath device size may not be correct, performing gratuitous resize.")

		err = p.nodeExpandVolume(ctx, publishInfo, mpathSize, stagingTargetPath, volumeId, req.GetSecrets())
		if err != nil {
			Logc(ctx).WithFields(LogFields{
				"multipathDevice": publishInfo.DevicePath,
				"volumeID":        volumeId,
				"size":            mpathSize,
				"err":             err,
			}).Warn("Attempt to perform gratuitous resize failed.")
		}
	}

	// Update in-mem map used for self-healing; do it after a volume has been staged.
	// Beyond here if there is a problem with the session or there are missing LUNs
	// then self-healing should be able to fix those issues.
	newCtx := context.WithValue(ctx, iscsi.SessionInfoSource, iscsi.SessionSourceNodeStage)

	// Acquiring the global self-healing session lock may impact parallelism,
	// but self-healing session operations are minimal and should complete quickly.
	// Therefore, a slight performance impact is acceptable to keep the code clean and maintainable.
	lockContext := "nodeStageISCSIVolume.AddSession"
	if !attemptLock(ctx, lockContext, iSCSISelfHealingSessionLock, csiNodeLockTimeout) {
		locks.Unlock(ctx, lockContext, iSCSISelfHealingSessionLock)
		return status.Error(codes.Aborted, "request waited too long for the lock")
	}
	p.iscsi.AddSession(newCtx, publishedISCSISessions, publishInfo, req.GetVolumeId(), "", models.NotInvalid)
	locks.Unlock(ctx, lockContext, iSCSISelfHealingSessionLock)
	return nil
}

// ensureAttachISCSIVolume attempts to attach the volume to the local host
// with a retry logic based on the publish information passed in.
func (p *Plugin) ensureAttachISCSIVolume(
	ctx context.Context, req *csi.NodeStageVolumeRequest, mountpoint string,
	publishInfo *models.VolumePublishInfo, attachTimeout time.Duration,
) (int64, error) {
	var err error
	var mpathSize int64

	// Perform the login/rescan/discovery/(optionally)format, mount & get the device back in the publish info
	if mpathSize, err = p.iscsi.AttachVolumeRetry(ctx, req.VolumeContext["internalName"], mountpoint,
		publishInfo, req.GetSecrets(), attachTimeout); err != nil {
		// Did we fail to log in?
		if errors.IsAuthError(err) {
			// Update CHAP info from the controller and try one more time.
			Logc(ctx).Warn("iSCSI login failed; will retrieve CHAP credentials from Trident controller and try again.")
			if err = p.updateChapInfoFromController(ctx, req, publishInfo); err != nil {
				return mpathSize, status.Error(codes.Internal, err.Error())
			}
			if mpathSize, err = p.iscsi.AttachVolumeRetry(ctx, req.VolumeContext["internalName"], mountpoint,
				publishInfo,
				req.GetSecrets(), attachTimeout); err != nil {
				// Bail out no matter what as we've now tried with updated credentials
				return mpathSize, status.Error(codes.Internal, err.Error())
			}
		} else {
			return mpathSize, status.Error(codes.Internal, fmt.Sprintf("failed to stage volume: %v", err))
		}
	}

	return mpathSize, nil
}

// getChapInfoFromController attempts to gather CHAP info from the controller in a timebound manner.
func (p *Plugin) getChapInfoFromController(
	ctx context.Context, volumeID, nodeName string,
) (*models.IscsiChapInfo, error) {
	chapInfo, err := p.restClient.GetChap(ctx, volumeID, nodeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get CHAP info from CSI controller for volume: '%s'; %w", volumeID, err)
	}
	return chapInfo, nil
}

func (p *Plugin) updateChapInfoFromController(
	ctx context.Context, req *csi.NodeStageVolumeRequest, publishInfo *models.VolumePublishInfo,
) error {
	chapInfo, err := p.restClient.GetChap(ctx, req.GetVolumeId(), p.nodeName)
	if err != nil {
		msg := "could not retrieve CHAP credentials from Trident controller"
		Logc(ctx).WithError(err).Error(msg)
		return errors.New(msg)
	}
	publishInfo.UseCHAP = chapInfo.UseCHAP
	publishInfo.IscsiUsername = chapInfo.IscsiUsername
	publishInfo.IscsiInitiatorSecret = chapInfo.IscsiInitiatorSecret
	publishInfo.IscsiTargetUsername = chapInfo.IscsiTargetUsername
	publishInfo.IscsiTargetSecret = chapInfo.IscsiTargetSecret
	return nil
}

func (p *Plugin) nodeUnstageISCSIVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest, publishInfo *models.VolumePublishInfo, force bool,
) error {
	// Default the device path to the value in the tracking file.
	devicePath := publishInfo.DevicePath

	// For some iSCSI backends, Trident cannot rely on the LUN serial.
	if publishInfo.IscsiLunSerial != "" {
		// Derive the device path using the LunSerial. The publishInfo.DevicePath may be incorrect due to Kernel actions.
		// Fallback to using the publishInfo.DevicePath if the multipath device cannot be derived.
		multipathDevice, err := p.devices.GetMultipathDeviceBySerial(ctx, hex.EncodeToString([]byte(publishInfo.IscsiLunSerial)))
		if err != nil {
			Logc(ctx).WithError(err).WithField("LunSerial", publishInfo.IscsiLunSerial).Debug(
				"Error finding multipath device by serial.")
		} else {
			Logc(ctx).WithFields(LogFields{
				"multipathDevice": multipathDevice,
				"LunSerial":       publishInfo.IscsiLunSerial,
			}).Debug("Found multipath device by serial.")
			devicePath = iscsi.DevPrefix + multipathDevice
		}
	}
	publishInfo.DevicePath = devicePath

	hostSessionMap := iscsiUtils.GetISCSIHostSessionMapForTarget(ctx, publishInfo.IscsiTargetIQN)
	// TODO: (jharrod) This is a temporary fix to handle the case where the hostSessionMap is empty. We need to
	// refactor nodeUnstageISCSIVolume to handle this case more gracefully and not rely on the hostSessionMap.
	if len(hostSessionMap) == 0 {
		// If we are in this block it likely means we have errored or had a pod restart after the iscsi logout and
		// before the tracking file has been removed. We need to ensure the device was removed and remove the tracking
		// file, without going through the rest of the unstage process.
		if convert.ToBool(publishInfo.LUKSEncryption) {
			var err error
			var luksMapperPath string
			fields := LogFields{"device": publishInfo.DevicePath, "volume": req.GetVolumeId()}
			// Set device path to dm device to correctly verify legacy volumes.
			if luks.IsLegacyDevicePath(publishInfo.DevicePath) {
				luksMapperPath = publishInfo.DevicePath
				dmPath, err := luks.GetDmDevicePathFromLUKSLegacyPath(ctx, p.command,
					publishInfo.DevicePath)
				if err != nil {
					Logc(ctx).WithFields(fields).WithError(err).Warn(
						"Could not determine dm device path from legacy LUKS device path. " +
							"Continuing with device removal.")
				} else {
					publishInfo.DevicePath = dmPath
				}
			} else {
				// Use the volume ID to get the LUKS mapper path.
				// This should always work if the mapper is still present.
				luksMapperPath, err = p.devices.GetLUKSDevicePathForVolume(ctx, req.GetVolumeId())
				if err != nil {
					// If the LUKS device is not found, the functional difference is negligible to unstage.
					// But it may be useful to log at different levels for observability.
					log := Logc(ctx).WithFields(fields).WithError(err)
					if errors.IsNotFoundError(err) {
						log.Warn("Failed to get LUKS device path for volume.")
					} else {
						log.Debug("Could not determine LUKS device path for volume.")
					}
					log.Debug("Continuing with device removal.")
				}
			}

			err = p.devices.EnsureLUKSDeviceClosedWithMaxWaitLimit(ctx, luksMapperPath)
			if err != nil {
				Logc(ctx).WithError(err).Debug("Unable to remove LUKS device. Continuing with tracking file removal.")
			}
		}
		if err := p.devices.RemoveMultipathDeviceMappingWithRetries(ctx, publishInfo.DevicePath,
			removeMultipathDeviceMappingRetries, removeMultipathDeviceMappingRetryDelay); err != nil {
			Logc(ctx).Warn("Unable to remove multipath device. Continuing with tracking file removal.")
		}
		// Ensure the tracking file is removed.
		volumeId, _, err := p.getVolumeIdAndStagingPath(req)
		if err != nil {
			return err
		}
		if err := p.nodeHelper.DeleteTrackingInfo(ctx, volumeId); err != nil {
			return status.Error(codes.Internal, err.Error())
		}
		return nil
	}

	deviceInfo, err := p.iscsi.GetDeviceInfoForLUN(ctx, hostSessionMap, int(publishInfo.IscsiLunNumber),
		publishInfo.IscsiTargetIQN, false)
	if err != nil {
		Logc(ctx).WithError(err).Debug("Could not find devices.")
		return fmt.Errorf("could not get device info: %v", err)
	} else if deviceInfo == nil {
		Logc(ctx).Debug("No devices found.")
	}

	// Acquiring the global self-healing session lock may impact parallelism,
	// but self-healing session operations are minimal and should complete quickly.
	// Therefore, a slight performance impact is acceptable to keep the code clean and maintainable.
	lockContext := "nodeUnstageISCSIVolume.RemoveLUNFromSessions"
	if !attemptLock(ctx, lockContext, iSCSISelfHealingSessionLock, csiNodeLockTimeout) {
		locks.Unlock(ctx, lockContext, iSCSISelfHealingSessionLock)
		return status.Error(codes.Aborted, "request waited too long for the lock")
	}
	// Remove Portal/LUN entries in self-healing map.
	p.iscsi.RemoveLUNFromSessions(ctx, publishInfo, publishedISCSISessions)
	locks.Unlock(ctx, lockContext, iSCSISelfHealingSessionLock)

	var luksMapperPath string
	// If the multipath device is not present, the LUKS device should not exist.
	if convert.ToBool(publishInfo.LUKSEncryption) && deviceInfo != nil && deviceInfo.MultipathDevice != "" {
		fields := LogFields{
			"lunID":           publishInfo.IscsiLunNumber,
			"publishedDevice": publishInfo.DevicePath,
			"multipathDevice": deviceInfo.MultipathDevice,
		}

		luksMapperPath, err = p.devices.GetLUKSDevicePathForVolume(ctx, req.GetVolumeId())
		if err != nil {
			if !errors.IsNotFoundError(err) {
				Logc(ctx).WithFields(fields).WithError(err).Error("Failed to get LUKS device path from multipath device.")
				return err
			}
			Logc(ctx).WithFields(fields).Info("No LUKS device path found from multipath device.")
		}

		// Ensure the LUKS device is closed if the luksMapperPath is set.
		if luksMapperPath != "" {
			fields["luksDevice"] = luksMapperPath
			err = p.devices.EnsureLUKSDeviceClosedWithMaxWaitLimit(ctx, luksMapperPath)
			if err != nil {
				if !errors.IsMaxWaitExceededError(err) {
					Logc(ctx).WithFields(fields).WithError(err).Error("Failed to close LUKS device.")
					return err
				}
				Logc(ctx).WithFields(fields).WithError(err).Debug("LUKS close wait time exceeded, continuing with device removal.")
			}
		}

		// Set device path to dm device to correctly verify legacy volumes.
		if luks.IsLegacyDevicePath(publishInfo.DevicePath) {
			publishInfo.DevicePath = deviceInfo.MultipathDevice
		}
	}

	// Delete the device from the host.
	var unmappedMpathDevice string
	if deviceInfo != nil {
		unmappedMpathDevice, err = p.iscsi.PrepareDeviceForRemoval(ctx, deviceInfo, publishInfo, nil, p.unsafeDetach,
			force)
		if err != nil {
			if errors.IsISCSISameLunNumberError(err) {
				// There is a need to pass all the publish infos this time
				unmappedMpathDevice, err = p.iscsi.PrepareDeviceForRemoval(ctx, deviceInfo, publishInfo,
					p.readAllTrackingFiles(ctx),
					p.unsafeDetach, force)
			}

			if err != nil && !p.unsafeDetach {
				return status.Error(codes.Internal, err.Error())
			}
		}
	}

	// Logout of the iSCSI session if appropriate for each applicable host
	logout := true
	if publishInfo.SharedTarget {
		// Check for any remaining mounts for this ISCSI target.
		anyMounts, err := p.iscsi.TargetHasMountedDevice(ctx, publishInfo.IscsiTargetIQN)
		// It's only safe to logout if there are no mounts and no error occurred when checking.
		logout = !anyMounts && err == nil

		// Since there are no mounts and no error occurred, we should check the hosts for any remaining devices.
		if logout {
			for hostNumber, sessionNumber := range hostSessionMap {
				if !p.iscsi.SafeToLogOut(ctx, hostNumber, sessionNumber) {
					// If even one host session is in use, we can't logout of the iSCSI sessions.
					logout = false
					break
				}
			}
		}
	}

	if logout {
		Logc(ctx).Debug("Safe to log out.")

		// Acquiring the global self-healing session lock may impact parallelism,
		// but self-healing session operations are minimal and should complete quickly.
		// Therefore, a slight performance impact is acceptable to keep the code clean and maintainable.
		lockContext = "nodeUnstageISCSIVolume.RemovePortalsFromSession"
		if !attemptLock(ctx, lockContext, iSCSISelfHealingSessionLock, csiNodeLockTimeout) {
			locks.Unlock(ctx, lockContext, iSCSISelfHealingSessionLock)
			return status.Error(codes.Aborted, "request waited too long for the lock")
		}
		// Remove portal entries from the self-healing map.
		p.iscsi.RemovePortalsFromSession(ctx, publishInfo, publishedISCSISessions)
		locks.Unlock(ctx, lockContext, iSCSISelfHealingSessionLock)

		if err := p.iscsi.Logout(ctx, publishInfo.IscsiTargetIQN, publishInfo.IscsiTargetPortal); err != nil {
			Logc(ctx).Error(err)
		}

		for _, portal := range publishInfo.IscsiPortals {

			if err := duringIscsiLogout.Inject(); err != nil {
				return err
			}

			if err := p.iscsi.Logout(ctx, publishInfo.IscsiTargetIQN, portal); err != nil {
				Logc(ctx).Error(err)
			}
		}
	}

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return err
	}

	// Ensure that the temporary mount point created during a filesystem expand operation is removed.
	if err := p.mount.UmountAndRemoveTemporaryMountPoint(ctx, stagingTargetPath); err != nil {
		Logc(ctx).WithFields(LogFields{
			"volumeId":          volumeId,
			"stagingTargetPath": stagingTargetPath,
		}).WithError(err).Errorf("Failed to remove directory in staging target path.")
		errStr := fmt.Sprintf("failed to remove temporary directory in staging target path %s; %s",
			stagingTargetPath, err.Error())
		return status.Error(codes.Internal, errStr)
	}

	// If the LUKS device still exists, it means the device was unable to be closed prior to removing the block
	// device. This can happen if the LUN was deleted or offline. It should be removable by this point.
	// It needs to be removed prior to removing the 'unmappedMpathDevice' device below.
	if luksMapperPath != "" {
		// EnsureLUKSDeviceClosed will not return an error if the device is already closed or removed.
		if err = p.devices.EnsureLUKSDeviceClosed(ctx, luksMapperPath); err != nil {
			Logc(ctx).WithFields(LogFields{
				"devicePath": luksMapperPath,
			}).WithError(err).Warning("Unable to remove LUKS mapper device.")
		}
		// Clear the time duration for the LUKS device.
		devices.LuksCloseDurations.RemoveDurationTracking(luksMapperPath)
	}

	// If there is multipath device, flush(remove) mappings
	if err := p.devices.RemoveMultipathDeviceMappingWithRetries(ctx, unmappedMpathDevice,
		removeMultipathDeviceMappingRetries, removeMultipathDeviceMappingRetryDelay); err != nil {
		return err
	}

	// Delete the device info we saved to the volume tracking info path so unstage can succeed.
	if err := p.nodeHelper.DeleteTrackingInfo(ctx, volumeId); err != nil {
		return status.Error(codes.Internal, err.Error())
	}

	return nil
}

func (p *Plugin) nodeUnstageISCSIVolumeRetry(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest, publishInfo *models.VolumePublishInfo, force bool,
) (*csi.NodeUnstageVolumeResponse, error) {
	Logc(ctx).Debug(">>>> nodeUnstageISCSIVolumeRetry")
	defer Logc(ctx).Debug("<<<< nodeUnstageISCSIVolumeRetry")

	if err := p.limiterSharedMap[NodeUnstageISCSIVolume].Wait(ctx); err != nil {
		return nil, err
	}
	defer p.limiterSharedMap[NodeUnstageISCSIVolume].Release(ctx)

	// TODO(pshashan): Once the locks package revamp PR is merged, remove this atomic counter.
	iSCSINodeOperationWaitingCount.Add(1)
	iSCSISelfHealingLock.RLock()
	defer iSCSISelfHealingLock.RUnlock()
	iSCSINodeOperationWaitingCount.Add(-1)

	nodeUnstageISCSIVolumeNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Failed to unstage the volume, retrying.")
	}

	nodeUnstageISCSIVolumeAttempt := func() error {
		return p.nodeUnstageISCSIVolume(ctx, req, publishInfo, force)
	}

	nodeUnstageISCSIVolumeBackoff := backoff.NewExponentialBackOff()
	nodeUnstageISCSIVolumeBackoff.InitialInterval = 1 * time.Second
	nodeUnstageISCSIVolumeBackoff.Multiplier = 1.414 // approx sqrt(2)
	nodeUnstageISCSIVolumeBackoff.RandomizationFactor = 0.1
	nodeUnstageISCSIVolumeBackoff.MaxElapsedTime = iSCSINodeUnstageMaxDuration

	// Run the un-staging using an exponential backoff
	if err := backoff.RetryNotify(nodeUnstageISCSIVolumeAttempt, nodeUnstageISCSIVolumeBackoff,
		nodeUnstageISCSIVolumeNotify); err != nil {
		Logc(ctx).Error("failed to unstage volume")
		return nil, err
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (p *Plugin) nodePublishISCSIVolume(
	ctx context.Context, req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {
	Logc(ctx).Debug(">>>> nodePublishISCSIVolume")
	defer Logc(ctx).Debug("<<<< nodePublishISCSIVolume")

	if err := p.limiterSharedMap[NodePublishISCSIVolume].Wait(ctx); err != nil {
		return nil, err
	}
	defer p.limiterSharedMap[NodePublishISCSIVolume].Release(ctx)

	var err error

	// Read the device info from the volume tracking info path, or if that doesn't exist yet, the staging path.
	trackingInfo, err := p.nodeHelper.ReadTrackingInfo(ctx, req.VolumeId)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	publishInfo := &trackingInfo.VolumePublishInfo

	if req.GetReadonly() {
		publishInfo.MountOptions = collection.AppendToStringList(publishInfo.MountOptions, "ro", ",")
	}

	devicePath := publishInfo.DevicePath
	if convert.ToBool(publishInfo.LUKSEncryption) {
		// Rotate the LUKS passphrase if needed, on failure, log and continue to publish
		var luksDevice luks.Device
		var err error
		if luks.IsLegacyDevicePath(devicePath) {
			// Supports legacy volumes that store the LUKS device path
			luksDevice, err = luks.NewDeviceFromMappingPath(ctx, p.command, devicePath,
				req.VolumeContext["internalName"])
		} else {
			luksDevice = luks.NewDevice(publishInfo.DevicePath, req.VolumeContext["internalName"], p.command)
		}

		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		err = ensureLUKSVolumePassphrase(ctx, p.restClient, luksDevice, req.GetVolumeId(), req.GetSecrets(), false)
		if err != nil {
			Logc(ctx).WithError(err).Error("Failed to ensure current LUKS passphrase.")
		}

		// Mount LUKS device instead of mpath.
		devicePath = luksDevice.MappedDevicePath()
	}
	isRawBlock := publishInfo.FilesystemType == filesystem.Raw
	if isRawBlock {

		if len(publishInfo.MountOptions) > 0 {
			publishInfo.MountOptions = collection.AppendToStringList(publishInfo.MountOptions, "bind", ",")
		} else {
			publishInfo.MountOptions = "bind"
		}

		// Place the block device at the target path for the raw-block.
		err = p.mount.MountDevice(ctx, devicePath, req.TargetPath, publishInfo.MountOptions, true)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "unable to bind mount raw device; %s", err)
		}
	} else {
		// Mount the device.
		err = p.mount.MountDevice(ctx, devicePath, req.TargetPath, publishInfo.MountOptions, false)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "unable to mount device; %s", err)
		}
	}

	err = p.nodeHelper.AddPublishedPath(ctx, req.VolumeId, req.TargetPath)
	if err != nil {
		return nil, err
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

type RequestHandler interface {
	GetVolumeId() string
	GetStagingTargetPath() string
}

// getVolumeIdAndStagingPath is a controllerHelper method to reduce code duplication
func (p *Plugin) getVolumeIdAndStagingPath(req RequestHandler) (string, string, error) {
	volumeId := req.GetVolumeId()
	if volumeId == "" {
		return "", "", status.Error(codes.InvalidArgument, "no volume ID provided")
	}

	stagingTargetPath := req.GetStagingTargetPath()
	if stagingTargetPath == "" {
		return "", "", status.Error(codes.InvalidArgument, "no staging target path provided")
	}

	return volumeId, stagingTargetPath, nil
}

// refreshTimerPeriod resets the time period between node cleanup executions.
// It introduces randomness (jitter) between reconciliation periods to avoid a thundering herd on the controller API.
func (p *Plugin) refreshTimerPeriod(ctx context.Context) time.Duration {
	Logc(ctx).Debug("Refreshing node publication reconcile timer")

	jitter := maximumNodeReconciliationJitter
	if n, err := rand.Int(rand.Reader, big.NewInt(int64(maximumNodeReconciliationJitter))); err == nil {
		jitter = time.Duration(n.Int64())
	}
	return defaultNodeReconciliationPeriod + jitter
}

// startReconcilingNodePublications starts an infinite background task to
// periodically reconcileNodePublicationState.
func (p *Plugin) startReconcilingNodePublications(ctx context.Context) {
	Logc(ctx).Info("Activating node publication reconciliation service.")
	p.nodePublicationTimer = time.NewTimer(defaultNodeReconciliationPeriod)

	go func() {
		ctx = GenerateRequestContext(nil, "", ContextSourcePeriodic, WorkflowNodeReconcilePubs, LogLayerCSIFrontend)

		for {
			select {
			case <-p.stopNodePublicationLoop:
				// Exit on shutdown signal
				return

			case <-p.nodePublicationTimer.C:
				Logc(ctx).Debug("Reconciling node publication state.")
				if err := p.reconcileNodePublicationState(ctx); err != nil {
					Logc(ctx).WithError(err).Debug("Failed to reconcile node publication state.")
					continue
				}
				Logc(ctx).Debug("Reconciled node publication state.")
			}
		}
	}()

	return
}

// stopReconcilingNodePublications gracefully stops the node publication reconciliation background task.
func (p *Plugin) stopReconcilingNodePublications(ctx context.Context) {
	Logc(ctx).Info("Stopping the node publication reconciliation service.")

	// Gracefully stop the reconciliation timer.
	if p.nodePublicationTimer != nil {
		if !p.nodePublicationTimer.Stop() {
			<-p.nodePublicationTimer.C
		}
	}

	// Gracefully stop the reconciliation loop.
	if p.stopNodePublicationLoop != nil {
		close(p.stopNodePublicationLoop)
	}
}

// reconcileNodePublicationState cleans any stale published path for volumes on the node by rectifying the actual state
// of publications (published paths on the node) against the desired state of publications from the CSI controller.
// If all published paths are cleaned successfully and the node is cleanable, it updates the Trident node CR via
// the CSI controller REST API.
// If a node is not in a cleanable state, it will not mark the node as clean.
func (p *Plugin) reconcileNodePublicationState(ctx context.Context) error {
	defer func() {
		// Reset the Timer only after the cleanup process is complete, regardless of if it fails or not.
		p.nodePublicationTimer.Reset(p.refreshTimerPeriod(ctx))
	}()

	// For force detach purposes, always get the node and check if it needs to be updated.
	node, err := p.restClient.GetNode(ctx, p.nodeName)
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to get node state from the CSI controller server.")
		return err
	}

	// For now, only cleanup the node iff the node is not clean.
	if node.PublicationState == models.NodeClean {
		Logc(ctx).Debug("Node is clean, nothing to do.")
		return nil
	}

	if err := p.performNodeCleanup(ctx); err != nil {
		Logc(ctx).WithError(err).Error("Failed to clean stale node publications.")
		return err
	}

	return p.updateNodePublicationState(ctx, node.PublicationState)
}

// performNodeCleanup will discover the difference between the volume tracking information stored on the node, and the
// publication records stored in the controller's persistence. It will then force unstage any stale volume attachments
// and remove their relevant tracking files. This is only intended to be called after the node has registered with
// the controller.
func (p *Plugin) performNodeCleanup(ctx context.Context) error {
	Logc(ctx).Debug("Performing node cleanup.")

	// Discover the desired publication state.
	desiredPublicationState, err := p.discoverDesiredPublicationState(ctx)
	if err != nil {
		return errors.WrapWithReconcileFailedError(err, "reconcile failed")
	}

	// Discover the actual publication state.
	actualPublicationState, err := p.discoverActualPublicationState(ctx)
	if err != nil {
		return errors.WrapWithReconcileFailedError(err, "reconcile failed")
	}

	// Check for stale publication records. If any exist, clean them.
	stalePublications := p.discoverStalePublications(ctx, actualPublicationState, desiredPublicationState)
	if len(stalePublications) != 0 {
		if err = p.cleanStalePublications(ctx, stalePublications); err != nil {
			return errors.WrapWithReconcileFailedError(err, "reconcile failed")
		}
	}

	return nil
}

// discoverDesiredPublicationState discovers the desired state of published volumes on the CSI controller and returns
// a mapping of volumeID -> publications.
func (p *Plugin) discoverDesiredPublicationState(ctx context.Context) (
	map[string]*models.VolumePublicationExternal, error,
) {
	Logc(ctx).Debug("Discovering desired publication state.")

	publications, err := p.restClient.ListVolumePublicationsForNode(ctx, p.nodeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get desired publication state")
	}

	desiredPublicationState := make(map[string]*models.VolumePublicationExternal, len(publications))
	for _, pub := range publications {
		desiredPublicationState[pub.VolumeName] = pub
	}

	return desiredPublicationState, nil
}

// discoverActualPublicationState discovers the actual state of published volumes on the node and returns
// a mapping of volumeID -> tracking information.
func (p *Plugin) discoverActualPublicationState(ctx context.Context) (map[string]*models.VolumeTrackingInfo, error) {
	Logc(ctx).Debug("Discovering actual publication state.")

	actualPublicationState, err := p.nodeHelper.ListVolumeTrackingInfo(ctx)
	if err != nil && !errors.IsNotFoundError(err) {
		return nil, fmt.Errorf("failed to get actual publication state")
	}

	return actualPublicationState, nil
}

// discoverStalePublications compares the actual state of publications with the desired state
// of publications in the controller and returns the delta between the two.
func (p *Plugin) discoverStalePublications(
	ctx context.Context,
	actualPublicationState map[string]*models.VolumeTrackingInfo,
	desiredPublicationState map[string]*models.VolumePublicationExternal,
) map[string]*models.VolumeTrackingInfo {
	Logc(ctx).Debug("Discovering stale volume publications.")

	// Track the delta between actual (node-side) and desired (controller-side) publication state.
	stalePublications := make(map[string]*models.VolumeTrackingInfo, 0)

	// Reconcile the actual state of publications to the desired state of publications.
	for volumeID, trackingInfo := range actualPublicationState {
		fields := LogFields{"volumeID": volumeID}

		// If we find the publication in the desired state, then we don't want to do anything.
		// Otherwise, remove the published paths and tracking info on the node.
		if _, ok := desiredPublicationState[volumeID]; !ok {
			Logc(ctx).WithFields(fields).Debug("Volume has no matching volume publication record.")
			stalePublications[volumeID] = trackingInfo
		}
	}

	return stalePublications
}

// cleanStalePublications cleans published paths on the host node for attached volumes with no matching publication
// object in the CSI controller. It should never publish volumes to the node.
func (p *Plugin) cleanStalePublications(
	ctx context.Context, stalePublications map[string]*models.VolumeTrackingInfo,
) error {
	Logc(ctx).Debug("Cleaning stale node publication state.")

	// Clean stale volume publication state.
	var err error
	for volumeID, trackingInfo := range stalePublications {
		var fields LogFields

		// If no published paths exist for a still staged volume, then it means CO / kubelet
		// died before it could finish CSI unpublish and unstage for this given volume.
		// These unpublish calls act as a best-effort to abide by and act within the CSI workflow.
		for targetPath := range trackingInfo.PublishedPaths {
			fields = LogFields{
				"volumeID":   volumeID,
				"targetPath": targetPath,
			}

			// Both VolumeID and TargetPath are required for NodeUnpublishVolume.
			unpublishReq := &csi.NodeUnpublishVolumeRequest{
				VolumeId:   volumeID,
				TargetPath: targetPath,
			}
			if _, unpublishErr := p.NodeUnpublishVolume(ctx, unpublishReq); unpublishErr != nil {
				Logc(ctx).WithFields(fields).WithError(unpublishErr).Debug("Failed to unpublish volume.")
				err = multierr.Combine(unpublishErr, fmt.Errorf("failed to unpublish volume; %v", unpublishErr))
			} else {
				Logc(ctx).WithFields(fields).Debug("Unpublished stale volume.")
			}
		}

		fields = LogFields{
			"volumeID":          volumeID,
			"stagingTargetPath": trackingInfo.StagingTargetPath,
		}

		// Both VolumeID and StagingTargetPath are required for nodeUnstageVolume.
		unstageReq := &csi.NodeUnstageVolumeRequest{
			VolumeId:          volumeID,
			StagingTargetPath: trackingInfo.StagingTargetPath,
		}
		if _, unstageErr := p.nodeUnstageVolume(ctx, unstageReq, true); unstageErr != nil {
			Logc(ctx).WithFields(fields).WithError(unstageErr).Debug("Failed to force unstage volume.")
			err = multierr.Combine(unstageErr, fmt.Errorf("failed to force unstage volume; %v", unstageErr))
		} else {
			Logc(ctx).WithFields(fields).Debug("Force detached stale volume attachment.")
		}
	}

	return err
}

func (p *Plugin) updateNodePublicationState(ctx context.Context, nodeState models.NodePublicationState) error {
	// Check if node is cleanable in the first place and bail if Trident shouldn't mark it as clean.
	if nodeState != models.NodeCleanable {
		Logc(ctx).Debugf("Controller node state is not cleanable; state was: [%s]", nodeState)
		return nil
	}

	Logc(ctx).Debug("Updating node publication state.")
	nodeStateFlags := &models.NodePublicationStateFlags{
		ProvisionerReady: convert.ToPtr(true),
	}
	if err := p.restClient.UpdateNode(ctx, p.nodeName, nodeStateFlags); err != nil {
		Logc(ctx).WithError(err).Error("Failed to update node publication state.")
		return err
	}
	Logc(ctx).Debug("Updated node publication state.")

	return nil
}

// selfHealingRectifySession rectifies a session which has been identified as ghost session.
// If it is determined that re-login is required, perform re login to the sessions and then scan for all the LUNs.
func (p *Plugin) selfHealingRectifySession(ctx context.Context, portal string, action models.ISCSIAction) error {
	Logc(ctx).WithFields(LogFields{
		"portal": portal,
		"action": action,
	}).Debug("ISCSI self-healing rectify session is invoked.")
	publishInfo, err := publishedISCSISessions.GeneratePublishInfo(portal)
	if err != nil {
		return fmt.Errorf("failed to get publish info for session on portal '%s'; %v", portal, err)
	}

	switch action {
	case models.LogoutLoginScan:
		// Do not log out if the portal is unresponsive as subsequent logins are likely to fail.
		if isAccessible, err := p.iscsi.IsPortalAccessible(ctx, portal); !isAccessible {
			Logc(ctx).WithError(err).Warnf("Cannot safely log out of unresponsive portal '%s'.", portal)
			return fmt.Errorf("cannot safely log out of unresponsive portal '%s'", portal)
		}

		if err = p.iscsi.Logout(ctx, publishInfo.IscsiTargetIQN, portal); err != nil {
			return fmt.Errorf("error while logging out of target %s", publishInfo.IscsiTargetIQN)
		}
		Logc(ctx).Debug("Logout is successful.")

		// Logout is successful, fallthrough to perform login.
		fallthrough
	case models.LoginScan:
		// Set FilesystemType to "raw" so that we only heal the session connectivity and not perform the mount and
		// filesystem related operations.
		publishInfo.FilesystemType = filesystem.Raw
		volumeID, err := publishedISCSISessions.VolumeIDForPortalAndLUN(portal, publishInfo.IscsiLunNumber)
		if err != nil {
			return fmt.Errorf("failed to get volume ID for lun ID; %v", err)
		}

		req := &csi.NodeStageVolumeRequest{
			VolumeId:      volumeID,
			VolumeContext: map[string]string{"internalName": volumeID},
			Secrets:       map[string]string{},
		}

		publishedCHAPCredentials := publishInfo.IscsiChapInfo
		if _, err = p.ensureAttachISCSIVolume(ctx, req, "", publishInfo, iSCSILoginTimeout); err != nil {
			return fmt.Errorf("failed to login to the target")
		}

		if publishedCHAPCredentials != publishInfo.IscsiChapInfo {
			fields := LogFields{
				"portal":    portal,
				"CHAPInUse": true,
			}

			// Update the CHAP info in the portal. This should never fail.
			updateErr := publishedISCSISessions.UpdateCHAPForPortal(portal, publishInfo.IscsiChapInfo)
			if updateErr != nil {
				Logc(ctx).WithFields(fields).Warn("Failed to update published CHAP information.")
			}
			Logc(ctx).WithFields(fields).Debug("Updated published CHAP information after successful login.")
		}

		Logc(ctx).Debug("Login to target is successful.")
		// Login is successful, fallthrough to perform scan
		fallthrough
	case models.Scan:
		// This detection may be useful for support in the future.
		// Retain this until there is a good reason to remove it.
		_ = p.deprecatedIgroupInUse(ctx)

		luns, err := publishedISCSISessions.LUNsForPortal(portal)
		if err != nil {
			return fmt.Errorf("failed to get LUNs for portal: %s; %w", portal, err)
		}

		if err = iscsi.InitiateScanForLuns(ctx, luns, publishInfo.IscsiTargetIQN); err != nil {
			Logc(ctx).WithError(err).Error("Could not initiate scan for some LUNs.")
			return fmt.Errorf("failed to initiate scan for LUNs in portal: %s; %w", portal, err)
		}

		Logc(ctx).WithFields(LogFields{
			"portal": portal,
			"luns":   luns,
			"target": publishInfo.IscsiTargetIQN,
		}).Debug("Successfully initiated iSCSI scan(s).")
	default:
		Logc(ctx).Debug("No valid action to be taken in iSCSI self-healing.")
	}

	return nil
}

// deprecatedIgroupInUse looks through the tracking files for deprecated igroups and reports if any are in use.
// NOTE: Precise LUN scanning removes the requirement for this logic, but this information may be useful for debugging
// and support cases. Additionally, this calculation is cheap so keep it in for now.
func (p *Plugin) deprecatedIgroupInUse(ctx context.Context) bool {
	volumeTrackingInfo, _ := p.nodeHelper.ListVolumeTrackingInfo(ctx)
	for id, info := range volumeTrackingInfo {
		if !iscsi.IsPerNodeIgroup(info.IscsiIgroup) {
			Logc(ctx).WithFields(LogFields{
				"volumeID": id,
				"lunID":    info.IscsiLunNumber,
				"igroup":   info.IscsiIgroup,
			}).Debug("Detected a deprecated igroup.")
			return true
		}
	}

	Logc(ctx).Debug("No deprecated igroups detected.")
	return false
}

// updateCHAPInfoForSessions provides a best attempt to populate up-to-date CHAP credentials within iSCSI self-healing's
// published sessions. It tracks CHAP credentials by unique IQN to reduce the number of calls to the controller.
func (p *Plugin) updateCHAPInfoForSessions(
	ctx context.Context, publishedSessions, currentSessions *models.ISCSISessions,
) error {
	if publishedSessions == nil || currentSessions == nil {
		return nil
	}

	// Timebox this operation to prevent it from running indefinitely and blocking everything in self-healing and
	// other operations in the node server.
	cancelCtx, cancel := context.WithTimeout(ctx, iSCSISelfHealingTimeout/3)
	defer cancel()

	// Store the CHAP credentials we've found for certain IQNs.
	// IQNs should be unique between SVMs and CHAP credentials are scoped at the SVM level.
	iqnToCHAP := make(map[string]*models.IscsiChapInfo, 0)
	var errs error

	for portal, publishedData := range publishedSessions.Info {
		// If the session isn't stale on the host, then we can ignore this portal or now.
		data, ok := currentSessions.Info[portal]
		if ok && !p.iscsi.IsSessionStale(cancelCtx, data.PortalInfo.SessionNumber) {
			continue
		} else if !publishedData.PortalInfo.CHAPInUse() || !publishedData.PortalInfo.HasTargetIQN() {
			continue
		}

		chapInfo, ok := iqnToCHAP[publishedData.PortalInfo.ISCSITargetIQN]
		if !ok {
			volumeID, err := publishedSessions.VolumeIDForPortal(portal)
			if err != nil {
				errs = errors.Join(errs, fmt.Errorf("failed to get volume ID for portal: '%s'; %w", portal, err))
				continue
			}

			chapInfo, err = p.getChapInfoFromController(cancelCtx, volumeID, p.nodeName)
			if err != nil {
				errs = errors.Join(errs, fmt.Errorf("failed to get CHAP info for portal: '%s'; %w", portal, err))
				continue
			}

			// Store what we've learned for future iterations.
			iqnToCHAP[publishedData.PortalInfo.ISCSITargetIQN] = chapInfo
		}

		publishedData.PortalInfo.UpdateCHAPCredentials(*chapInfo)
		Logc(cancelCtx).WithField("portal", portal).Debug("Updated CHAP info for portal.")
	}

	if errs != nil {
		Logc(cancelCtx).WithError(errs).Error("Failed to get updated CHAP info for portal(s).")
		return errs
	} else if len(iqnToCHAP) == 0 {
		Logc(cancelCtx).Debug("No outdated CHAP info found in published sessions.")
		return nil
	}

	Logc(cancelCtx).Debug("Updated CHAP info for published sessions.")
	return nil
}

// performISCSISelfHealing inspects the desired state of the iSCSI sessions with the current state and accordingly
// identifies candidate sessions that require remediation. This function is invoked periodically.
func (p *Plugin) performISCSISelfHealing(ctx context.Context) {
	iSCSISelfHealingLock.Lock()
	defer iSCSISelfHealingLock.Unlock()

	lockContext := "performISCSISelfHealing.SessionLock"
	defer locks.Unlock(ctx, lockContext, iSCSISelfHealingSessionLock)
	if !attemptLock(ctx, lockContext, iSCSISelfHealingSessionLock, csiNodeLockTimeout) {
		Logc(ctx).WithError(fmt.Errorf("request waited too long for the lock"))
		return
	}

	defer func() {
		if r := recover(); r != nil {
			Logc(ctx).Errorf("Panic in iSCSISelfHealing. \nStack Trace: %v", string(debug.Stack()))
			return
		}
	}()

	// After this time self-healing may be stopped
	stopSelfHealingAt := time.Now().Add(iSCSISelfHealingTimeout)
	// If there are not iSCSI volumes expected on the host skip self-healing
	if publishedISCSISessions.IsEmpty() {
		Logc(ctx).Debug("Skipping iSCSI self-heal cycle; no iSCSI volumes published on the host.")
		return
	}

	if err := p.iscsi.PreChecks(ctx); err != nil {
		Logc(ctx).Errorf("Skipping iSCSI self-heal cycle; pre-checks failed: %v.", err)
	}

	// Reset current sessions
	currentISCSISessions = models.NewISCSISessions()

	// Reset published iSCSI session remediation information
	if err := publishedISCSISessions.ResetAllRemediationValues(); err != nil {
		Logc(ctx).WithError(err).Error("Failed to reset remediation value(s) for published iSCSI sessions. ")
	}

	if err := p.iscsi.PopulateCurrentSessions(ctx, currentISCSISessions); err != nil {
		Logc(ctx).WithError(err).
			Error("Failed to get current state of iSCSI Sessions LUN mappings; skipping iSCSI self-heal cycle.")
		return
	}

	if currentISCSISessions.IsEmpty() {
		Logc(ctx).Debug("No iSCSI sessions LUN mappings found.")
	}

	// Update CHAP info for published sessions.
	if err := p.updateCHAPInfoForSessions(ctx, publishedISCSISessions, currentISCSISessions); err != nil {
		Logc(ctx).WithError(err).Error("Failed to update CHAP credentials for published iSCSI sessions.")
	}

	Logc(ctx).Debugf("Published iSCSI Sessions: %v", publishedISCSISessions)
	Logc(ctx).Debugf("Current iSCSI Sessions: %v", currentISCSISessions)

	// The problems self-heal aims to resolve can be divided into two buckets. A stale portal bucket
	// and a non-stale portal bucket. Stale portals are the sessions that were healthy at some point
	// in time, then due to temporary network connectivity issue along with CHAP rotation made them
	// un-recoverable by themselves and thus require intervention.
	// Issues in non-stale portal bucket would include sessions that were never established due
	// to temporary networking issue or logged-out sessions or LUNs that were never scanned for some sessions.

	// SELF-HEAL STEP 1: Identify all sorted candidate stale portals and sorted candidate non-stale portals.
	staleISCSIPortals, nonStaleISCSIPortals := p.iscsi.InspectAllISCSISessions(ctx, publishedISCSISessions,
		currentISCSISessions, p.iSCSISelfHealingWaitTime)

	// SELF-HEAL STEP 2: Attempt to fix all the stale portals.
	p.fixISCSISessions(ctx, staleISCSIPortals, "stale", stopSelfHealingAt)

	// SELF-HEAL STEP 3: Attempt to fix at-least one of the non-stale portals.
	p.fixISCSISessions(ctx, nonStaleISCSIPortals, "non-stale", stopSelfHealingAt)

	return
}

// fixISCSISessions iterates through all the portal inputs, identifies their respective remediation
// and calls another function to actually fix the issues.
func (p *Plugin) fixISCSISessions(ctx context.Context, portals []string, portalType string, stopAt time.Time) {
	if len(portals) == 0 {
		Logc(ctx).Debugf("No %s iSCSI portal found.", portalType)
		return
	}

	Logc(ctx).Debugf("Found %s portal(s) that require remediation.", portalType)

	for _, portal := range portals {
		// TODO(pshashan): Once the locks package revamp PR is merged, remove this atomic counter
		// and just query the wait count of that particular lock.
		if iSCSINodeOperationWaitingCount.Load() > 0 {
			if time.Now().After(stopAt) {
				Logc(ctx).Debug("Self-healing has exceeded maximum runtime; preempting iSCSI session self-healing.")
				break
			}
		}

		// First get the fix action
		fixAction := publishedISCSISessions.Info[portal].Remediation

		Logc(ctx).Debugf("Attempting to fix iSCSI portal %v it requires %s", portal, fixAction)

		// First thing to do is to update the lastAccessTime
		publishedISCSISessions.Info[portal].PortalInfo.LastAccessTime = time.Now()

		if err := p.selfHealingRectifySession(ctx, portal, fixAction); err != nil {
			Logc(ctx).WithError(err).Errorf("Encountered error while attempting to fix portal %v.", portal)
		} else {
			Logc(ctx).Debugf("Fixed portal %v it required %s", portal, fixAction)

			// On success ensure portal's FirstIdentifiedStaleAt value is reset
			publishedISCSISessions.Info[portal].PortalInfo.ResetFirstIdentifiedStaleAt()
		}
	}
}

/*TODO (bpresnel) Enable later, with rate limiting?
func (p *Plugin) startLoggingConfigReconcile() {
	ticker := time.NewTicker(LoggingConfigReconcilePeriod)
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				p.updateNodeLoggingConfig()
			}
		}
	}()
}

func (p *Plugin) updateNodeLoggingConfig() {
	bgCtx := context.Background()
	logLevel, loggingWorkflows, loggingLayers, err := p.restClient.GetLoggingConfig(bgCtx)
	if err != nil {
		warnMsg := "Could not retrieve the current log level, logging workflows and logging layers from the controller."
		Log().WithError(err).Warnf(warnMsg)
		return
	}

	levelNeededUpdate, workflowsNeededUpdate, layersNeededUpdate := false, false, false

	if GetDefaultLogLevel() != logLevel {
		levelNeededUpdate = true
		if err = SetDefaultLogLevel(logLevel); err != nil {
			msg := "Could not set node log level to the controller's level: %s"
			Log().Warnf(msg, logLevel)
			return
		}
	}

	if GetSelectedWorkFlows() != loggingWorkflows {
		if err = SetWorkflows(loggingWorkflows); err != nil {
			msg := "Could not set node logging workflows to the controller's selected workflows: %s\n"
			Log().Warnf(msg, loggingWorkflows)
			return
		}
		workflowsNeededUpdate = true
	}

	if GetSelectedLogLayers() != loggingLayers {
		if err = SetLogLayers(loggingLayers); err != nil {
			msg := "Could not set node logging layers to the controller's selected log layers: %s\n"
			Log().Warnf(msg, loggingWorkflows)
			return
		}
		layersNeededUpdate = true
	}

	levelFmt := `Log level updated to: "%s" based on the controller's current configuration.`
	workflowsFmt := `Selected logging workflows updated to: "%s" based on the controller's current configuration.'`
	layersFmt := `Selected logging layers updated to: "%s" based on the controller's current configuration.'`

	if levelNeededUpdate {
		Log().Infof(levelFmt, logLevel)
	}
	if workflowsNeededUpdate {
		Log().Infof(workflowsFmt, loggingWorkflows)
	}
	if layersNeededUpdate {
		Log().Infof(layersFmt, loggingLayers)
	}
}
*/

func (p *Plugin) nodeStageNVMeVolume(
	ctx context.Context, req *csi.NodeStageVolumeRequest,
	publishInfo *models.VolumePublishInfo,
) error {
	Logc(ctx).Debug(">>>> nodeStageNVMeVolume")
	defer Logc(ctx).Debug("<<<< nodeStageNVMeVolume")

	// Serializing all the parallel requests by relying on the constant var.
	lockContext := "NodeStageSANNVMeVolume-" + req.GetVolumeId()
	defer locks.Unlock(ctx, lockContext, nodeLockID)
	if !attemptLock(ctx, lockContext, nodeLockID, csiNodeLockTimeout) {
		return status.Error(codes.Aborted, "request waited too long for the lock")
	}

	isLUKS := convert.ToBool(req.PublishContext["LUKSEncryption"])
	publishInfo.LUKSEncryption = strconv.FormatBool(isLUKS)
	publishInfo.MountOptions = req.PublishContext["mountOptions"]
	publishInfo.NVMeSubsystemNQN = req.PublishContext["nvmeSubsystemNqn"]
	publishInfo.NVMeNamespaceUUID = req.PublishContext["nvmeNamespaceUUID"]
	publishInfo.NVMeTargetIPs = strings.Split(req.PublishContext["nvmeTargetIPs"], ",")
	publishInfo.SANType = req.PublishContext["SANType"]
	publishInfo.FormatOptions = req.PublishContext["formatOptions"]

	err := p.nvmeHandler.AttachNVMeVolumeRetry(
		ctx, req.VolumeContext["internalName"], "", publishInfo, req.GetSecrets(), nvme.NVMeAttachTimeout,
	)
	if err != nil {
		return err
	}

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return err
	}

	if isLUKS {
		if err := betweenAttachAndLUKSPassphrase.Inject(); err != nil {
			return err
		}

		luksDevice := luks.NewDevice(publishInfo.DevicePath, req.VolumeContext["internalName"], p.command)

		// Ensure we update the passphrase in case it has never been set before
		err = ensureLUKSVolumePassphrase(ctx, p.restClient, luksDevice, volumeId, req.GetSecrets(), true)
		if err != nil {
			return fmt.Errorf("could not set LUKS volume passphrase; %v", err)
		}
	}

	if err := beforeTrackingInfoWrite.Inject(); err != nil {
		return err
	}

	volTrackingInfo := &models.VolumeTrackingInfo{
		VolumePublishInfo: *publishInfo,
		StagingTargetPath: stagingTargetPath,
		PublishedPaths:    map[string]struct{}{},
	}

	// Save the device info to the volume tracking info path for use in future CSI node publish & unstage calls.
	if err := p.nodeHelper.WriteTrackingInfo(ctx, volumeId, volTrackingInfo); err != nil {
		return err
	}

	if err := afterInitialTrackingInfoWrite.Inject(); err != nil {
		return err
	}

	p.nvmeHandler.AddPublishedNVMeSession(&publishedNVMeSessions, publishInfo)
	return nil
}

// nodeUnstageNVMeVolume unstages volume for nvme driver.
// - Get the device from tracking info OR from "nvme netapp ontapdevices".
// - Flush the device using "nvme flush /dev/<nvme-device>"
// - If the subsystem has <= 1 Namespaces connected to it, then disconnect.
// - Remove the tracking file for the volume/Namespace.
func (p *Plugin) nodeUnstageNVMeVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest, publishInfo *models.VolumePublishInfo, force bool,
) (*csi.NodeUnstageVolumeResponse, error) {
	Logc(ctx).Debug(">>>> nodeUnstageNVMeVolume")
	defer Logc(ctx).Debug("<<<< nodeUnstageNVMeVolume")

	// Serializing all the parallel requests by relying on the constant var.
	lockContext := "NodeUnstageNVMeVolume-" + req.GetVolumeId()
	defer locks.Unlock(ctx, lockContext, nodeLockID)
	if !attemptLock(ctx, lockContext, nodeLockID, csiNodeLockTimeout) {
		return nil, status.Error(codes.Aborted, "request waited too long for the lock")
	}

	disconnect := p.nvmeHandler.RemovePublishedNVMeSession(&publishedNVMeSessions, publishInfo.NVMeSubsystemNQN,
		publishInfo.NVMeNamespaceUUID)

	nvmeSubsys := p.nvmeHandler.NewNVMeSubsystem(ctx, publishInfo.NVMeSubsystemNQN)
	// Get the device using 'nvme-cli' commands. Flush the device IOs.
	// Proceed further with unstage flow, if device is not found.
	nvmeDev, err := nvmeSubsys.GetNVMeDevice(ctx, publishInfo.NVMeNamespaceUUID)
	if err != nil && !errors.IsNotFoundError(err) {
		return nil, fmt.Errorf("failed to get NVMe device; %v", err)
	}

	devicePath := publishInfo.DevicePath
	if nvmeDev != nil {
		devicePath = nvmeDev.GetPath()
	}

	var luksMapperPath string
	if convert.ToBool(publishInfo.LUKSEncryption) && devicePath != "" {
		fields := LogFields{
			"namespace":     publishInfo.NVMeNamespaceUUID,
			"devicePath":    devicePath,
			"publishedPath": publishInfo.DevicePath,
		}

		// Use the volume ID to get the LUKS mapper path.
		// This should always work if the mapper is still present.
		luksMapperPath, err = p.devices.GetLUKSDevicePathForVolume(ctx, req.GetVolumeId())
		if err != nil {
			// If the LUKS device is not found, the functional difference is negligible to unstage.
			// But it may be useful to log at different levels for observability.
			log := Logc(ctx).WithFields(fields).WithError(err)
			if errors.IsNotFoundError(err) {
				log.Warn("Failed to get LUKS device path for volume.")
			} else {
				log.Debug("Could not determine LUKS device path for volume.")
			}
			log.Debug("Continuing with device removal.")
		}

		if luksMapperPath != "" {
			fields["luksMapperPath"] = luksMapperPath
			if err = p.devices.EnsureLUKSDeviceClosedWithMaxWaitLimit(ctx, luksMapperPath); err != nil {
				if !errors.IsMaxWaitExceededError(err) {
					Logc(ctx).WithFields(fields).WithError(err).Error("Failed to close LUKS device.")
					return &csi.NodeUnstageVolumeResponse{}, err
				}
				Logc(ctx).WithFields(fields).WithError(err).Debug("LUKS close wait time exceeded, continuing with device removal.")
			}
			if err := afterNvmeLuksDeviceClosed.Inject(); err != nil {
				return nil, err
			}
		}
	}

	// Attempt to flush the NVMe device.
	if !nvmeDev.IsNil() {
		// If flush fails, give a grace period of 6 minutes (nvmeMaxFlushWaitDuration) before giving up.
		if err := nvmeDev.FlushDevice(ctx, p.unsafeDetach, force); err != nil {
			if NVMeNamespacesFlushRetry[publishInfo.NVMeNamespaceUUID].IsZero() {
				NVMeNamespacesFlushRetry[publishInfo.NVMeNamespaceUUID] = time.Now()
				return nil, fmt.Errorf("failed to flush NVMe device; %v", err)
			}

			// If the max wait time for flush isn't hit yet, fail and let the CSI node agent call again.
			elapsed := time.Since(NVMeNamespacesFlushRetry[publishInfo.NVMeNamespaceUUID])
			if elapsed <= nvmeMaxFlushWaitDuration {
				Logc(ctx).WithFields(LogFields{
					"devicePath": devicePath,
					"namespace":  publishInfo.NVMeNamespaceUUID,
					"elapsed":    elapsed,
				}).WithError(err).Debug("Could not flush NVMe device.")
				return nil, fmt.Errorf("failed to flush NVMe device; %v", err)
			}

			Logc(ctx).WithFields(LogFields{
				"namespace": publishInfo.NVMeNamespaceUUID,
				"elapsed":   elapsed,
				"maxWait":   nvmeMaxFlushWaitDuration,
			}).Warn("Could not flush device within expected time period.")
		}

		delete(NVMeNamespacesFlushRetry, publishInfo.NVMeNamespaceUUID)
	}

	// Get the number of namespaces associated with the subsystem
	numNs, err := nvmeSubsys.GetNamespaceCount(ctx)
	if err != nil {
		Logc(ctx).WithField(
			"subsystem", publishInfo.NVMeSubsystemNQN,
		).WithError(err).Debug("Error getting Namespace count.")
	}

	// If number of namespaces is more than 1, don't disconnect the subsystem. If we get any issues while getting the
	// number of namespaces through the CLI, we can rely on the disconnect flag from NVMe self-healing sessions (if
	// NVMe self-healing is enabled), which keeps track of namespaces associated with the subsystem.
	if (err == nil && numNs <= 1) || (p.nvmeSelfHealingInterval > 0 && err != nil && disconnect) {
		if err := nvmeSubsys.Disconnect(ctx); err != nil {
			Logc(ctx).WithField(
				"subsystem", publishInfo.NVMeSubsystemNQN,
			).WithError(err).Debug("Error disconnecting subsystem.")
		}
	}
	if err := afterNvmeDisconnect.Inject(); err != nil {
		return nil, err
	}

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return nil, err
	}

	// Ensure that the temporary mount point created during a filesystem expand operation is removed.
	if err := p.mount.UmountAndRemoveTemporaryMountPoint(ctx, stagingTargetPath); err != nil {
		Logc(ctx).WithField("stagingTargetPath", stagingTargetPath).Errorf(
			"Failed to remove directory in staging target path; %s", err)
		errStr := fmt.Sprintf("failed to remove temporary directory in staging target path %s; %s",
			stagingTargetPath, err)
		return nil, status.Error(codes.Internal, errStr)
	}

	// If the LUKS device still exists, it means the device was unable to be closed prior to removing the block
	// device. This can happen if the LUN was deleted or offline. It should be removable by this point.
	// It needs to be removed prior to removing the 'unmappedMpathDevice' device below.
	if luksMapperPath != "" {
		// EnsureLUKSDeviceClosed will not return an error if the device is already closed or removed.
		if err = p.devices.EnsureLUKSDeviceClosed(ctx, luksMapperPath); err != nil {
			Logc(ctx).WithFields(LogFields{
				"devicePath": luksMapperPath,
			}).WithError(err).Warning("Unable to remove LUKS mapper device.")
		}
		// Clear the time duration for the LUKS device.
		devices.LuksCloseDurations.RemoveDurationTracking(luksMapperPath)
	}

	// Delete the device info we saved to the volume tracking info path so unstage can succeed.
	if err := p.nodeHelper.DeleteTrackingInfo(ctx, volumeId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (p *Plugin) nodePublishNVMeVolume(
	ctx context.Context, req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {
	Logc(ctx).Debug(">>>> nodePublishNVMeVolume")
	defer Logc(ctx).Debug("<<<< nodePublishNVMeVolume")

	// Serializing all the parallel requests by relying on the constant var.
	lockContext := "NodePublishNVMeVolume-" + req.GetVolumeId()
	defer locks.Unlock(ctx, lockContext, nodeLockID)
	if !attemptLock(ctx, lockContext, nodeLockID, csiNodeLockTimeout) {
		return nil, status.Error(codes.Aborted, "request waited too long for the lock")
	}

	var err error

	// Read the device info from the volume tracking info path, or if that doesn't exist yet, the staging path.
	trackingInfo, err := p.nodeHelper.ReadTrackingInfo(ctx, req.VolumeId)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	publishInfo := &trackingInfo.VolumePublishInfo

	if req.GetReadonly() {
		publishInfo.MountOptions = collection.AppendToStringList(publishInfo.MountOptions, "ro", ",")
	}

	devicePath := publishInfo.DevicePath
	if convert.ToBool(publishInfo.LUKSEncryption) {
		// Rotate the LUKS passphrase if needed, on failure, log and continue to publish
		luksDevice := luks.NewDevice(devicePath, req.VolumeContext["internalName"], p.command)

		err = ensureLUKSVolumePassphrase(ctx, p.restClient, luksDevice, req.GetVolumeId(), req.GetSecrets(), false)
		if err != nil {
			Logc(ctx).WithError(err).Error("Failed to ensure current LUKS passphrase.")
		}

		// At this point, we must reassign the device path to the LUKS mapper path for mounts to work.
		devicePath = luksDevice.MappedDevicePath()
	}

	isRawBlock := publishInfo.FilesystemType == filesystem.Raw
	if isRawBlock {
		if len(publishInfo.MountOptions) > 0 {
			publishInfo.MountOptions = collection.AppendToStringList(publishInfo.MountOptions, "bind", ",")
		} else {
			publishInfo.MountOptions = "bind"
		}

		// Place the block device at the target path for the raw-block.
		err = p.mount.MountDevice(ctx, devicePath, req.TargetPath, publishInfo.MountOptions, true)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "unable to bind mount raw device; %s", err)
		}
	} else {
		// Mount the device.
		err = p.mount.MountDevice(ctx, devicePath, req.TargetPath, publishInfo.MountOptions, false)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "unable to mount device; %s", err)
		}
	}

	err = p.nodeHelper.AddPublishedPath(ctx, req.VolumeId, req.TargetPath)
	if err != nil {
		return nil, err
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (p *Plugin) nodeStageSANVolume(
	ctx context.Context, req *csi.NodeStageVolumeRequest,
) (*csi.NodeStageVolumeResponse, error) {
	var err error

	mountCapability := req.GetVolumeCapability().GetMount()
	blockCapability := req.GetVolumeCapability().GetBlock()

	if mountCapability == nil && blockCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "mount or block capability required")
	}

	fsType := req.PublishContext["filesystemType"]
	if mountCapability != nil && mountCapability.GetFsType() != "" {
		fsType = mountCapability.GetFsType()
	}

	if fsType == filesystem.Raw && mountCapability != nil {
		return nil, status.Error(codes.InvalidArgument, "mount capability requested with raw blocks")
	}

	if fsType != filesystem.Raw && blockCapability != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("block capability requested with %s", fsType))
	}

	sharedTarget, err := strconv.ParseBool(req.PublishContext["sharedTarget"])
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	publishInfo := &models.VolumePublishInfo{
		Localhost:      true,
		FilesystemType: fsType,
		SharedTarget:   sharedTarget,
	}

	switch req.PublishContext["SANType"] {
	case sa.FCP:
		if err = p.nodeStageFCPVolume(ctx, req, publishInfo); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	case sa.NVMe:
		if err = p.nodeStageNVMeVolume(ctx, req, publishInfo); err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("failed to stage volume: %s", err.Error()))
		}
	default:
		if err = p.nodeStageISCSIVolume(ctx, req, publishInfo); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

// performNVMeSelfHealing inspects the desired state of the NVMe sessions with the current state and accordingly
// identifies candidate sessions that require remediation. This function is invoked periodically.
func (p *Plugin) performNVMeSelfHealing(ctx context.Context) {
	locks.Lock(ctx, nvmeSelfHealingLockContext, nodeLockID)
	defer locks.Unlock(ctx, nvmeSelfHealingLockContext, nodeLockID)

	defer func() {
		if r := recover(); r != nil {
			Logc(ctx).Errorf("Panic in NVMeSelfHealing. \nStack Trace: %v", string(debug.Stack()))
			return
		}
	}()

	if publishedNVMeSessions.IsEmpty() {
		Logc(ctx).Debug("No NVMe volumes are published. Self healing is not required.")
		return
	}

	// After this time self-healing may be stopped
	stopSelfHealingAt := time.Now().Add(60 * time.Second)

	// Reset the remediations found in the previous run.
	publishedNVMeSessions.ResetRemediationForAll()

	// Reset current sessions
	currentNVMeSessions = nvme.NVMeSessions{}

	// Populate the current sessions
	if err := p.nvmeHandler.PopulateCurrentNVMeSessions(ctx, &currentNVMeSessions); err != nil {
		Logc(ctx).Errorf("Failed to populate current sessions %v.", err)
		return
	}

	Logc(ctx).Debugf("Published NVMe sessions %v.", publishedNVMeSessions)
	Logc(ctx).Debugf("Current NVMe sessions %v.", currentNVMeSessions)

	subsToFix := p.nvmeHandler.InspectNVMeSessions(ctx, &publishedNVMeSessions, &currentNVMeSessions)

	Logc(ctx).Debug("Start NVMe healing.")
	p.fixNVMeSessions(ctx, stopSelfHealingAt, subsToFix)
	Logc(ctx).Debug("NVMe healing finished.")
}

func (p *Plugin) fixNVMeSessions(ctx context.Context, stopAt time.Time, subsystems []nvme.NVMeSubsystem) {
	for index, sub := range subsystems {
		// If the subsystem is not present in the published NVMe sessions, we don't need to rectify its sessions.
		if !publishedNVMeSessions.CheckNVMeSessionExists(sub.NQN) {
			continue
		}

		// 1. We should fix at least one subsystem in a single self-healing thread.
		// 2. If there's another thread waiting for the node lock and if we have exceeded our 60 secs lock, we should
		//    stop NVMe self-healing.
		if index > 0 && locks.WaitQueueSize(nodeLockID) > 0 && time.Now().After(stopAt) {
			Logc(ctx).Info("Self-healing has exceeded maximum runtime; preempting NVMe session self-healing.")
			break
		}

		p.nvmeHandler.RectifyNVMeSession(ctx, sub, &publishedNVMeSessions)
	}
}
