// Copyright 2022 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"go.uber.org/multierr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

const (
	tridentDeviceInfoPath           = "/var/lib/trident/tracking"
	lockID                          = "csi_node_server"
	AttachISCSIVolumeTimeoutShort   = 20 * time.Second
	iSCSINodeUnstageMaxDuration     = 15 * time.Second
	iSCSISelfHealingLockContext     = "ISCSISelfHealingThread"
	nvmeSelfHealingLockContext      = "NVMeSelfHealingThread"
	defaultNodeReconciliationPeriod = 1 * time.Minute
	maximumNodeReconciliationJitter = 5000 * time.Millisecond
	nvmeMaxFlushWaitDuration        = 6 * time.Minute
	csiNodeLockTimeout              = 60 * time.Second
)

var (
	topologyLabels = make(map[string]string)
	iscsiUtils     = utils.IscsiUtils
	bofUtils       = utils.BofUtils

	publishedISCSISessions, currentISCSISessions utils.ISCSISessions
	publishedNVMeSessions, currentNVMeSessions   utils.NVMeSessions
	// NVMeNamespacesFlushRetry - Non-persistent map of Namespaces to maintain the flush errors if any.
	// During NodeUnstageVolume, Trident shall return success after specific wait time (nvmeMaxFlushWaitDuration).
	NVMeNamespacesFlushRetry = make(map[string]time.Time)
)

func attemptLock(ctx context.Context, lockContext string, lockTimeout time.Duration) bool {
	startTime := time.Now()
	utils.Lock(ctx, lockContext, lockID)
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

	fields := LogFields{"Method": "NodeStageVolume", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Debug(">>>> NodeStageVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NodeStageVolume")

	lockContext := "NodeStageVolume-" + req.GetVolumeId()
	defer utils.Unlock(ctx, lockContext, lockID)

	if !attemptLock(ctx, lockContext, csiNodeLockTimeout) {
		return nil, status.Error(codes.Aborted, "request waited too long for the lock")
	}
	switch req.PublishContext["protocol"] {
	case string(tridentconfig.File):
		if req.PublishContext["filesystemType"] == utils.SMB {
			return p.nodeStageSMBVolume(ctx, req)
		} else {
			return p.nodeStageNFSVolume(ctx, req)
		}
	case string(tridentconfig.Block):
		return p.nodeStageSANVolume(ctx, req)
	case string(tridentconfig.BlockOnFile):
		return p.nodeStageNFSBlockVolume(ctx, req)
	default:
		return nil, status.Error(codes.InvalidArgument, "unknown protocol")
	}
}

func (p *Plugin) NodeUnstageVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest,
) (*csi.NodeUnstageVolumeResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowNodeUnstage)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

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

	lockContext := "NodeUnstageVolume-" + req.GetVolumeId()
	defer utils.Unlock(ctx, lockContext, lockID)

	if !attemptLock(ctx, lockContext, csiNodeLockTimeout) {
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

	switch protocol {
	case tridentconfig.File:
		if publishInfo.FilesystemType == utils.SMB {
			if force {
				Logc(ctx).WithFields(fields).WithField("protocol", tridentconfig.File).
					Warning("forced unstage not supported for this protocol")
			}
			return p.nodeUnstageSMBVolume(ctx, req)
		} else {
			if force {
				Logc(ctx).WithFields(fields).WithField("protocol", tridentconfig.File).
					Warning("forced unstage not supported for this protocol")
			}
			return p.nodeUnstageNFSVolume(ctx, req)
		}
	case tridentconfig.Block:
		if publishInfo.SANType == sa.NVMe {
			return p.nodeUnstageNVMeVolume(ctx, req, publishInfo, force)
		}
		return p.nodeUnstageISCSIVolumeRetry(ctx, req, publishInfo, force)
	case tridentconfig.BlockOnFile:
		if force {
			Logc(ctx).WithFields(fields).WithField("protocol", tridentconfig.BlockOnFile).
				Warning("forced unstage not supported for this protocol")
		}
		return p.nodeUnstageNFSBlockVolume(ctx, req, publishInfo)
	default:
		return nil, status.Error(codes.InvalidArgument, "unknown protocol")
	}
}

func (p *Plugin) NodePublishVolume(
	ctx context.Context, req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowNodePublish)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{"Method": "NodePublishVolume", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Debug(">>>> NodePublishVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NodePublishVolume")

	lockContext := "NodePublishVolume-" + req.GetVolumeId()
	defer utils.Unlock(ctx, lockContext, lockID)

	if !attemptLock(ctx, lockContext, csiNodeLockTimeout) {
		return nil, status.Error(codes.Aborted, "request waited too long for the lock")
	}
	switch req.PublishContext["protocol"] {
	case string(tridentconfig.File):
		trackingInfo, err := p.nodeHelper.ReadTrackingInfo(ctx, req.VolumeId)
		if err != nil {
			if errors.IsNotFoundError(err) {
				return nil, status.Error(codes.FailedPrecondition, err.Error())
			} else {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}
		if trackingInfo.VolumePublishInfo.FilesystemType == utils.SMB {
			return p.nodePublishSMBVolume(ctx, req)
		} else {
			return p.nodePublishNFSVolume(ctx, req)
		}
	case string(tridentconfig.Block):
		if req.PublishContext["SANType"] == sa.NVMe {
			return p.nodePublishNVMeVolume(ctx, req)
		}
		return p.nodePublishISCSIVolume(ctx, req)
	case string(tridentconfig.BlockOnFile):
		return p.nodePublishNFSBlockVolume(ctx, req)
	default:
		return nil, status.Error(codes.InvalidArgument, "unknown protocol")
	}
}

func (p *Plugin) NodeUnpublishVolume(
	ctx context.Context, req *csi.NodeUnpublishVolumeRequest,
) (*csi.NodeUnpublishVolumeResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowNodeUnpublish)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{"Method": "NodeUnpublishVolume", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Debug(">>>> NodeUnpublishVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NodeUnpublishVolume")

	lockContext := "NodeUnpublishVolume-" + req.GetVolumeId()
	defer utils.Unlock(ctx, lockContext, lockID)

	if !attemptLock(ctx, lockContext, csiNodeLockTimeout) {
		return nil, status.Error(codes.Aborted, "request waited too long for the lock")
	}

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "no volume ID provided")
	}

	targetPath := req.GetTargetPath()

	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "no target path provided")
	}

	isDir, err := utils.IsLikelyDir(targetPath)
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
		notMountPoint, err = utils.IsLikelyNotMountPoint(ctx, targetPath)
	} else {
		var mounted bool
		mounted, err = utils.IsMounted(ctx, "", targetPath, "")
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
		if err = utils.Umount(ctx, targetPath); err != nil {
			Logc(ctx).WithFields(LogFields{"path": targetPath, "error": err}).Error("unable to unmount volume.")
			return nil, status.Errorf(codes.InvalidArgument, "unable to unmount volume; %s", err)
		}
	}

	// As per the CSI spec SP i.e. Trident is responsible for deleting the target path,
	// however today Kubernetes performs this deletion. Here we are making best efforts
	// to delete the resource at target path. Sometimes this fails resulting CSI calling
	// NodeUnpublishVolume again and usually deletion goes through in the second attempt.
	if err = utils.DeleteResourceAtPath(ctx, targetPath); err != nil {
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
	exists, err := utils.PathExists(req.GetVolumePath())
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

		isRawBlock = publishInfo.FilesystemType == tridentconfig.FsRaw
	}
	if isRawBlock {
		// Return no capacity info for raw block volumes, we cannot reliably determine the capacity.
		return &csi.NodeGetVolumeStatsResponse{}, nil
	} else {
		// If filesystem, return usage reported by FS.
		available, capacity, usage, inodes, inodesFree, inodesUsed, err := utils.GetFilesystemStats(
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
	ctx = SetContextWorkflow(ctx, WorkflowVolumeResize)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

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

	// VolumePublishInfo is stored in the volume tracking file as of Trident 23.01.
	trackingInfo, err := p.nodeHelper.ReadTrackingInfo(ctx, volumeId)
	if err != nil {
		if errors.IsNotFoundError(err) {
			msg := fmt.Sprintf("unable to find tracking file for volume: %s ; needed it for resize", volumeId)
			return nil, status.Error(codes.NotFound, msg)
		}

		return nil, status.Errorf(codes.Internal, err.Error())
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

	err = p.nodeExpandVolume(ctx, &trackingInfo.VolumePublishInfo, requiredBytes, stagingTargetPath, volumeId,
		req.GetSecrets())
	if err != nil {
		return nil, err
	}
	return &csi.NodeExpandVolumeResponse{}, nil
}

func (p *Plugin) nodeExpandVolume(
	ctx context.Context, publishInfo *utils.VolumePublishInfo, requiredBytes int64, stagingTargetPath string,
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
		if fsType, err = utils.VerifyFilesystemSupport(publishInfo.FilesystemType); err != nil {
			break
		}
		// We don't need to rescan mount devices for NVMe protocol backend. Automatic namespace rescanning happens
		// everytime the NVMe controller is reset, or if the controller posts an asynchronous event indicating
		// namespace attributes have changed.
		if publishInfo.SANType != sa.NVMe {
			err = nodePrepareISCSIVolumeForExpansion(ctx, publishInfo, requiredBytes)
		}
		mountOptions = publishInfo.MountOptions
	case tridentconfig.BlockOnFile:
		if fsType, err = utils.GetVerifiedBlockFsType(publishInfo.FilesystemType); err != nil {
			break
		}
		err = nodePrepareBlockOnFileVolumeForExpansion(ctx, publishInfo, requiredBytes)
		mountOptions = publishInfo.SubvolumeMountOptions
	default:
		return status.Error(codes.InvalidArgument, "unknown protocol")
	}

	if err != nil {
		Logc(ctx).WithField("devicePath", publishInfo.DevicePath).WithError(err).Error(
			"Unable to expand volume")
		return err
	}

	isLUKS, err := strconv.ParseBool(publishInfo.LUKSEncryption)
	if err != nil {
		Logc(ctx).WithField("volumeId", volumeId).Warnf("Could not parse LUKSEncryption into a bool, got %v.",
			publishInfo.LUKSEncryption)
	}
	if isLUKS {
		Logc(ctx).WithField("volumeId", volumeId).Info("Resizing the LUKS mapping.")
		// Refresh the luks device
		// cryptsetup resize <luks-device-path> << <passphrase>
		passphrase, ok := secrets["luks-passphrase"]
		if !ok {
			return status.Error(codes.InvalidArgument, "cannot expand LUKS encrypted volume; no passphrase provided")
		} else if passphrase == "" {
			return status.Error(codes.InvalidArgument, "cannot expand LUKS encrypted volume; empty passphrase provided")
		}
		err := utils.ResizeLUKSDevice(ctx, publishInfo.DevicePath, passphrase)
		if err != nil {
			if errors.IsIncorrectLUKSPassphraseError(err) {
				return status.Error(codes.InvalidArgument, err.Error())
			}
			return status.Error(codes.Internal, err.Error())
		}
	}

	// Expand filesystem.
	if fsType != tridentconfig.FsRaw {
		filesystemSize, err := utils.ExpandFilesystemOnNode(ctx, publishInfo, stagingTargetPath, fsType,
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

// nodePrepareISCSIVolumeForExpansion readies volume expansion for Block (i.e. iSCSI) volumes
func nodePrepareISCSIVolumeForExpansion(
	ctx context.Context, publishInfo *utils.VolumePublishInfo, requiredBytes int64,
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
	if utils.IsAlreadyAttached(ctx, lunID, publishInfo.IscsiTargetIQN) {
		// Rescan device to detect increased size.
		if err = utils.ISCSIRescanDevices(
			ctx, publishInfo.IscsiTargetIQN, publishInfo.IscsiLunNumber, requiredBytes); err != nil {
			Logc(ctx).WithFields(LogFields{
				"device": publishInfo.DevicePath,
				"error":  err,
			}).Error("Unable to scan device.")
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

// nodePrepareBlockOnFileVolumeForExpansion readies volume expansion for BlockOnFile volumes
func nodePrepareBlockOnFileVolumeForExpansion(
	ctx context.Context, publishInfo *utils.VolumePublishInfo, requiredBytes int64,
) error {
	Logc(ctx).WithFields(LogFields{
		"nfsUniqueID":   publishInfo.NfsUniqueID,
		"nfsPath":       publishInfo.NfsPath,
		"subvolumeName": publishInfo.SubvolumeName,
		"devicePath":    publishInfo.DevicePath,
	}).Trace("PublishInfo for device to expand.")

	nfsMountpoint := publishInfo.NFSMountpoint
	loopFile := path.Join(nfsMountpoint, publishInfo.SubvolumeName)

	var err error = nil

	loopDeviceAttached, err := utils.IsLoopDeviceAttachedToFile(ctx, publishInfo.DevicePath, loopFile)
	if err != nil {
		return fmt.Errorf("unable to identify if loop device '%s' is attached to file '%s': %v",
			publishInfo.DevicePath, loopFile, err)
	}

	// Make sure device is ready.
	if loopDeviceAttached {
		if err = utils.ResizeLoopDevice(ctx, publishInfo.DevicePath, loopFile, requiredBytes); err != nil {
			Logc(ctx).WithField("device", publishInfo.DevicePath).WithError(err).Error(
				"Unable to resize loop device.")
			err = status.Error(codes.Internal, err.Error())
		}
	} else {
		Logc(ctx).WithField("devicePath", publishInfo.DevicePath).Error(
			"Unable to expand volume as device is not attached.")
		err = fmt.Errorf("device %s to expand is not attached", publishInfo.DevicePath)
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

func (p *Plugin) nodeGetInfo(ctx context.Context) *utils.Node {
	// Only get the host system info if we don't have the info yet.
	if p.hostInfo == nil {
		host, err := utils.GetHostSystemInfo(ctx)
		if err != nil {
			p.hostInfo = &utils.HostSystem{}
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
	iscsiWWNs, err := utils.GetInitiatorIqns(ctx)
	if err != nil {
		Logc(ctx).WithError(err).Warn("Problem getting iSCSI initiator name.")
	} else if len(iscsiWWNs) == 0 {
		Logc(ctx).Warn("Could not find iSCSI initiator name.")
	} else {
		iscsiWWN = iscsiWWNs[0]
		Logc(ctx).WithField("IQN", iscsiWWN).Info("Discovered iSCSI initiator name.")
	}

	ips, err := utils.GetIPAddresses(ctx)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Could not get IP addresses.")
	} else if len(ips) == 0 {
		Logc(ctx).Warn("Could not find any usable IP addresses.")
	} else {
		Logc(ctx).WithField("IP Addresses", ips).Info("Discovered IP addresses.")
	}

	// Discover active protocol services on the host.
	var services []string
	nfsActive, err := utils.NFSActiveOnHost(ctx)
	if err != nil {
		Logc(ctx).WithError(err).Warn("Error discovering NFS service on host.")
	}
	if nfsActive {
		services = append(services, "NFS")
	}

	smbActive, err := utils.SMBActiveOnHost(ctx)
	if err != nil {
		Logc(ctx).WithError(err).Warn("Error discovering SMB service on host.")
	}
	if smbActive {
		services = append(services, "SMB")
	}

	iscsiActive, err := utils.ISCSIActiveOnHost(ctx, *p.hostInfo)
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
	node := &utils.Node{
		Name:     p.nodeName,
		IQN:      iscsiWWN,
		NQN:      nvmeNQN,
		IPs:      ips,
		NodePrep: nil,
		HostInfo: p.hostInfo,
		Deleted:  false,
		// If the node is already known to exist Trident CSI Controllers persistence layer,
		// that state will be used instead. Otherwise, node state defaults to clean.
		PublicationState: utils.NodeClean,
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
	publishInfo := &utils.VolumePublishInfo{
		Localhost:      true,
		FilesystemType: "nfs",
	}

	if err := utils.IsCompatible(ctx, publishInfo.FilesystemType); err != nil {
		return nil, err
	}

	publishInfo.MountOptions = req.PublishContext["mountOptions"]
	publishInfo.NfsServerIP = req.PublishContext["nfsServerIp"]
	publishInfo.NfsPath = req.PublishContext["nfsPath"]

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return nil, err
	}

	volTrackingInfo := &utils.VolumeTrackingInfo{
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
	targetPath := req.GetTargetPath()
	notMnt, err := utils.IsLikelyNotMountPoint(ctx, targetPath)
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
		publishInfo.MountOptions = utils.AppendToStringList(publishInfo.MountOptions, "ro", ",")
	}

	err = utils.AttachNFSVolume(ctx, req.VolumeContext["internalName"], req.TargetPath, publishInfo)
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
	publishInfo := &utils.VolumePublishInfo{
		Localhost:      true,
		FilesystemType: utils.SMB,
	}

	if err := utils.IsCompatible(ctx, publishInfo.FilesystemType); err != nil {
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

	volTrackingInfo := &utils.VolumeTrackingInfo{
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
	err = utils.AttachSMBVolume(ctx, volumeId, stagingTargetPath, username, password, publishInfo)
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
	var mappingPath string

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return nil, err
	}

	trackingInfo, err := p.nodeHelper.ReadTrackingInfo(ctx, volumeId)
	if err != nil {
		return &csi.NodeUnstageVolumeResponse{}, err
	}

	mappingPath, err = utils.GetUnmountPath(ctx, trackingInfo)
	if err != nil {
		return nil, err
	}

	err = utils.UmountSMBPath(ctx, mappingPath, stagingTargetPath)

	// Delete the device info we saved to the volume tracking info path so unstage can succeed.
	if err := p.nodeHelper.DeleteTrackingInfo(ctx, volumeId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnstageVolumeResponse{}, err
}

func (p *Plugin) nodePublishSMBVolume(
	ctx context.Context, req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	source := req.GetStagingTargetPath()
	if len(source) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target not provided")
	}
	notMnt, err := utils.IsLikelyNotMountPoint(ctx, targetPath)
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
	err = utils.WindowsBindMount(ctx, source, targetPath, mountOptions)
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

func unstashIscsiTargetPortals(publishInfo *utils.VolumePublishInfo, reqPublishInfo map[string]string) error {
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
	volumeIDs := utils.GetAllVolumeIDs(ctx, tridentDeviceInfoPath)
	for _, volumeID := range volumeIDs {
		trackingInfo, err := p.nodeHelper.ReadTrackingInfo(ctx, volumeID)
		if err != nil || trackingInfo == nil {
			Logc(ctx).WithFields(LogFields{
				"volumeID": volumeID,
				"error":    err.Error(),
				"isEmpty":  trackingInfo == nil,
			}).Error("Volume tracking file info not found or is empty.")

			continue
		}

		publishInfo := &trackingInfo.VolumePublishInfo

		if publishInfo.SANType != sa.NVMe {
			newCtx := context.WithValue(ctx, utils.SessionInfoSource, utils.SessionSourceTrackingInfo)
			utils.AddISCSISession(newCtx, &publishedISCSISessions, publishInfo, volumeID, "", utils.NotInvalid)
		} else {
			p.nvmeHandler.AddPublishedNVMeSession(&publishedNVMeSessions, publishInfo)
		}
	}
}

func (p *Plugin) readAllTrackingFiles(ctx context.Context) []utils.VolumePublishInfo {
	publishInfos := make([]utils.VolumePublishInfo, 0)
	volumeIDs := utils.GetAllVolumeIDs(ctx, tridentDeviceInfoPath)
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

func (p *Plugin) nodeStageISCSIVolume(
	ctx context.Context, req *csi.NodeStageVolumeRequest, publishInfo *utils.VolumePublishInfo,
) error {
	useCHAP, err := strconv.ParseBool(req.PublishContext["useCHAP"])
	if err != nil {
		return err
	}
	publishInfo.UseCHAP = useCHAP

	lunID, err := strconv.ParseInt(req.PublishContext["iscsiLunNumber"], 10, 0)
	if err != nil {
		return err
	}

	var isLUKS bool
	if req.PublishContext["LUKSEncryption"] != "" {
		isLUKS, err = strconv.ParseBool(req.PublishContext["LUKSEncryption"])
		if err != nil {
			return fmt.Errorf("could not parse LUKSEncryption into a bool, got %v", publishInfo.LUKSEncryption)
		}
	}
	publishInfo.LUKSEncryption = strconv.FormatBool(isLUKS)

	err = unstashIscsiTargetPortals(publishInfo, req.PublishContext)
	if nil != err {
		return err
	}

	publishInfo.MountOptions = req.PublishContext["mountOptions"]
	publishInfo.IscsiTargetIQN = req.PublishContext["iscsiTargetIqn"]
	publishInfo.IscsiLunNumber = int32(lunID)
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

	mpathSize, err := p.ensureAttachISCSIVolume(ctx, req, "", publishInfo, AttachISCSIVolumeTimeoutShort)
	if err != nil {
		return err
	}

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return err
	}
	if isLUKS {
		luksDevice, err := utils.NewLUKSDeviceFromMappingPath(ctx, publishInfo.DevicePath,
			req.VolumeContext["internalName"])
		if err != nil {
			return err
		}
		// Ensure we update the passphrase incase it has never been set before
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

	volTrackingInfo := &utils.VolumeTrackingInfo{
		VolumePublishInfo: *publishInfo,
		StagingTargetPath: stagingTargetPath,
		PublishedPaths:    map[string]struct{}{},
	}
	// Save the device info to the volume tracking info path for use in the publish & unstage calls.
	if err := p.nodeHelper.WriteTrackingInfo(ctx, volumeId, volTrackingInfo); err != nil {
		return err
	}
	// Update in-mem map used for self-healing; do it after a volume has been staged.
	// Beyond here if there is a problem with the session or there are missing LUNs
	// then self-healing should be able to fix those issues.
	newCtx := context.WithValue(ctx, utils.SessionInfoSource, utils.SessionSourceNodeStage)
	utils.AddISCSISession(newCtx, &publishedISCSISessions, publishInfo, req.GetVolumeId(), "", utils.NotInvalid)
	return nil
}

// ensureAttachISCSIVolume attempts to attach the volume to the local host
// with a retry logic based on the pubish information passed in.
func (p *Plugin) ensureAttachISCSIVolume(
	ctx context.Context, req *csi.NodeStageVolumeRequest, mountpoint string,
	publishInfo *utils.VolumePublishInfo, attachTimeout time.Duration,
) (int64, error) {
	var err error
	var mpathSize int64

	// Perform the login/rescan/discovery/(optionally)format, mount & get the device back in the publish info
	if mpathSize, err = utils.AttachISCSIVolumeRetry(ctx, req.VolumeContext["internalName"], mountpoint, publishInfo,
		req.GetSecrets(), attachTimeout); err != nil {
		// Did we fail to log in?
		if errors.IsAuthError(err) {
			// Update CHAP info from the controller and try one more time.
			Logc(ctx).Warn("iSCSI login failed; will retrieve CHAP credentials from Trident controller and try again.")
			if err = p.updateChapInfoFromController(ctx, req, publishInfo); err != nil {
				return mpathSize, status.Error(codes.Internal, err.Error())
			}
			if mpathSize, err = utils.AttachISCSIVolumeRetry(ctx, req.VolumeContext["internalName"], mountpoint,
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

func (p *Plugin) updateChapInfoFromController(
	ctx context.Context, req *csi.NodeStageVolumeRequest, publishInfo *utils.VolumePublishInfo,
) error {
	chapInfo, err := p.restClient.GetChap(ctx, req.GetVolumeId(), p.nodeName)
	if err != nil {
		msg := "could not retrieve CHAP credentials from Trident controller"
		Logc(ctx).WithError(err).Error(msg)
		return fmt.Errorf(msg)
	}
	publishInfo.UseCHAP = chapInfo.UseCHAP
	publishInfo.IscsiUsername = chapInfo.IscsiUsername
	publishInfo.IscsiInitiatorSecret = chapInfo.IscsiInitiatorSecret
	publishInfo.IscsiTargetUsername = chapInfo.IscsiTargetUsername
	publishInfo.IscsiTargetSecret = chapInfo.IscsiTargetSecret
	return nil
}

func (p *Plugin) nodeUnstageISCSIVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest, publishInfo *utils.VolumePublishInfo, force bool,
) error {
	// Remove Portal/LUN entries in self-healing map.
	utils.RemoveLUNFromSessions(ctx, publishInfo, &publishedISCSISessions)

	if publishInfo.LUKSEncryption != "" {
		isLUKS, err := strconv.ParseBool(publishInfo.LUKSEncryption)
		if err != nil {
			return fmt.Errorf("could not parse LUKSEncryption into a bool, got %v", publishInfo.LUKSEncryption)
		}
		if isLUKS {
			// Before closing the device, get the corresponding DM device.
			publishedLUKsDevice, err := utils.GetUnderlyingDevicePathForLUKSDevice(ctx, publishInfo.DevicePath)
			if err != nil {
				// No need to return an error
				Logc(ctx).WithFields(LogFields{
					"devicePath": publishInfo.DevicePath,
					"lun":        publishInfo.IscsiLunNumber,
					"err":        err,
				}).Error("Failed to verify the multipath device, could not determine" +
					" underlying device for LUKS mapping.")
			}

			err = utils.EnsureLUKSDeviceClosed(ctx, publishInfo.DevicePath)
			if err != nil {
				return err
			}

			// For the future steps LUKs device path is not really useful, either it should be
			// DM device or empty.
			publishInfo.DevicePath = publishedLUKsDevice
		}
	}

	// Delete the device from the host.
	unmappedMpathDevice, err := utils.PrepareDeviceForRemoval(ctx, publishInfo, nil, p.unsafeDetach, force)
	if err != nil {
		if errors.IsISCSISameLunNumberError(err) {
			// There is a need to pass all the publish infos this time
			unmappedMpathDevice, err = utils.PrepareDeviceForRemoval(ctx, publishInfo, p.readAllTrackingFiles(ctx),
				p.unsafeDetach, force)
		}

		if err != nil && !p.unsafeDetach {
			return status.Error(codes.Internal, err.Error())
		}
	}

	// Get map of hosts and sessions for given Target IQN.
	hostSessionMap := iscsiUtils.GetISCSIHostSessionMapForTarget(ctx, publishInfo.IscsiTargetIQN)
	if len(hostSessionMap) == 0 {
		Logc(ctx).Warnf("no iSCSI hosts found for target %s", publishInfo.IscsiTargetIQN)
	}

	// Logout of the iSCSI session if appropriate for each applicable host
	logout := false
	for hostNumber, sessionNumber := range hostSessionMap {
		if !publishInfo.SharedTarget {
			// Always log out of a non-shared target.
			logout = true
			break
		} else {
			// Log out of a shared target if no mounts to that target remain.
			anyMounts, err := utils.ISCSITargetHasMountedDevice(ctx, publishInfo.IscsiTargetIQN)
			if logout = (err == nil) && !anyMounts && utils.SafeToLogOut(ctx, hostNumber, sessionNumber); logout {
				break
			}
		}
	}

	if logout {
		Logc(ctx).Debug("Safe to log out")

		// Remove portal entries from the self-healing map.
		utils.RemovePortalsFromSession(ctx, publishInfo, &publishedISCSISessions)

		if err := utils.ISCSILogout(ctx, publishInfo.IscsiTargetIQN, publishInfo.IscsiTargetPortal); err != nil {
			Logc(ctx).Error(err)
		}
		for _, portal := range publishInfo.IscsiPortals {
			if err := utils.ISCSILogout(ctx, publishInfo.IscsiTargetIQN, portal); err != nil {
				Logc(ctx).Error(err)
			}
		}
	}

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return err
	}

	// Ensure that the temporary mount point created during a filesystem expand operation is removed.
	if err := utils.UmountAndRemoveTemporaryMountPoint(ctx, stagingTargetPath); err != nil {
		Logc(ctx).WithField("stagingTargetPath", stagingTargetPath).Errorf(
			"Failed to remove directory in staging target path; %s", err)
		errStr := fmt.Sprintf("failed to remove temporary directory in staging target path %s; %s",
			stagingTargetPath, err)
		return status.Error(codes.Internal, errStr)
	}

	// Delete the device info we saved to the volume tracking info path so unstage can succeed.
	if err := p.nodeHelper.DeleteTrackingInfo(ctx, volumeId); err != nil {
		return status.Error(codes.Internal, err.Error())
	}

	// If there is multipath device, flush(remove) mappings
	if unmappedMpathDevice != "" {
		utils.RemoveMultipathDeviceMapping(ctx, unmappedMpathDevice)
	}

	return nil
}

func (p *Plugin) nodeUnstageISCSIVolumeRetry(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest, publishInfo *utils.VolumePublishInfo, force bool,
) (*csi.NodeUnstageVolumeResponse, error) {
	nodeUnstageISCSIVolumeNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Failed to unstage the volume, retrying.")
	}

	nodeUnstageISCSIVolumeAttempt := func() error {
		err := p.nodeUnstageISCSIVolume(ctx, req, publishInfo, force)
		return err
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
		publishInfo.MountOptions = utils.AppendToStringList(publishInfo.MountOptions, "ro", ",")
	}
	if publishInfo.LUKSEncryption != "" {
		isLUKS, err := strconv.ParseBool(publishInfo.LUKSEncryption)
		if err != nil {
			return nil, fmt.Errorf("could not parse LUKSEncryption into a bool, got %v", publishInfo.LUKSEncryption)
		}
		if isLUKS {
			// Rotate the LUKS passphrase if needed, on failure, log and continue to publish
			luksDevice, err := utils.NewLUKSDeviceFromMappingPath(ctx, publishInfo.DevicePath,
				req.VolumeContext["internalName"])
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			err = ensureLUKSVolumePassphrase(ctx, p.restClient, luksDevice, req.GetVolumeId(), req.GetSecrets(), false)
			if err != nil {
				Logc(ctx).WithError(err).Error("Failed to ensure current LUKS passphrase.")
			}
		}
	}
	isRawBlock := publishInfo.FilesystemType == tridentconfig.FsRaw
	if isRawBlock {

		if len(publishInfo.MountOptions) > 0 {
			publishInfo.MountOptions = utils.AppendToStringList(publishInfo.MountOptions, "bind", ",")
		} else {
			publishInfo.MountOptions = "bind"
		}

		// Place the block device at the target path for the raw-block.
		err = utils.MountDevice(ctx, publishInfo.DevicePath, req.TargetPath, publishInfo.MountOptions, true)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "unable to bind mount raw device; %s", err)
		}
	} else {
		// Mount the device.
		err = utils.MountDevice(ctx, publishInfo.DevicePath, req.TargetPath, publishInfo.MountOptions, false)
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

func (p *Plugin) nodeStageNFSBlockVolume(
	ctx context.Context, req *csi.NodeStageVolumeRequest,
) (*csi.NodeStageVolumeResponse, error) {
	// Get VolumeID and Staging Path.
	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return nil, err
	}

	publishInfo := &utils.VolumePublishInfo{
		Localhost: true,
	}

	publishInfo.MountOptions = utils.SanitizeMountOptions(req.PublishContext["mountOptions"], []string{"ro"})
	publishInfo.NfsServerIP = req.PublishContext["nfsServerIp"]
	publishInfo.NfsPath = req.PublishContext["nfsPath"]
	publishInfo.NfsUniqueID = req.PublishContext["nfsUniqueID"]
	publishInfo.SubvolumeName = req.PublishContext["subvolumeName"]
	publishInfo.FilesystemType = req.PublishContext["filesystemType"]
	publishInfo.SubvolumeMountOptions = utils.SanitizeMountOptions(req.PublishContext["subvolumeMountOptions"],
		[]string{"ro"})

	// The NFS mount path should be same for all the Subvolumes belonging to the same NFS volumes
	// thus use NFS volume's Unique ID. This also means the subvolumes from different Virtual Pools,
	// different backends but from the same NFS volume would be available under the same NFS mount
	// path on the Node.
	// NOTE: Store this value in the publishInfo so that other CSI plugin functions do not have to guess this value
	//       in case location of the NFS mountpoint changes in the future.
	publishInfo.NFSMountpoint = path.Join(tridentconfig.VolumeTrackingInfoPath, publishInfo.NfsUniqueID,
		publishInfo.NfsPath)

	loopFile := path.Join(publishInfo.NFSMountpoint, publishInfo.SubvolumeName)
	loopDeviceName, stagingMountpoint, err := utils.AttachBlockOnFileVolume(ctx, stagingTargetPath, publishInfo)
	if err != nil {
		return nil, err
	}
	publishInfo.DevicePath = loopDeviceName
	publishInfo.StagingMountpoint = stagingMountpoint

	loopFileInfo, err := os.Stat(loopFile)
	if err != nil {
		Logc(ctx).WithField("loopFile", loopFile).WithError(err).Error("Failed to get loop file size")
		return nil, fmt.Errorf("failed to get loop file size: %s, %s", loopFile, err.Error())
	}

	loopFileSize := loopFileInfo.Size()

	err = p.nodeExpandVolume(ctx, publishInfo, loopFileSize, stagingTargetPath, volumeId, map[string]string{})
	if err != nil {
		return nil, err
	}

	volTrackingInfo := &utils.VolumeTrackingInfo{
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

func (p *Plugin) nodeUnstageNFSBlockVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest, publishInfo *utils.VolumePublishInfo,
) (*csi.NodeUnstageVolumeResponse, error) {
	volumeId, _, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return nil, err
	}

	// Ensure that the staging mount point is removed.
	if err := utils.UmountAndRemoveMountPoint(ctx, publishInfo.StagingMountpoint); err != nil {
		Logc(ctx).WithField("stagingMountPoint", publishInfo.StagingMountpoint).Errorf(
			"Failed to remove the staging mount point directory; %s", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to remove staging mount point directory %s; %s",
			publishInfo.StagingMountpoint, err))
	}

	nfsMountpoint := publishInfo.NFSMountpoint
	loopFile := path.Join(nfsMountpoint, publishInfo.SubvolumeName)

	// Get the loop device name and see if it is attached or not.
	isLoopDeviceAttached, loopDevice, err := bofUtils.GetLoopDeviceAttachedToFile(ctx, loopFile)
	if err != nil {
		return nil, status.Error(codes.Internal,
			fmt.Sprintf("failed to get loop device for loop file '%s': %s", loopFile, err.Error()))
	}

	if isLoopDeviceAttached {
		// Check if it is mounted or not (ideally it should never be mounted).
		isLoopDeviceMounted, err := utils.IsLoopDeviceMounted(ctx, loopDevice.Name)
		if err != nil {
			return nil, status.Error(codes.Internal,
				fmt.Sprintf("unable to identify if loop device '%s' is mounted; %v", loopDevice.Name, err))
		}
		// If not mounted anymore, proceed to remove the device and/or remove the NFS mount point.
		if !isLoopDeviceMounted {
			// Detach the loop device from the NFS mountpoint.
			if err = utils.DetachBlockOnFileVolume(ctx, loopDevice.Name, loopFile); err != nil {
				return nil, status.Error(codes.Internal, fmt.Sprintf("failed to detach loop device '%s': %s",
					loopDevice.Name, err.Error()))
			}

			// Wait for the loop device to disappear.
			if err = utils.WaitForLoopDeviceDetach(ctx, nfsMountpoint, loopFile, 10*time.Second); err != nil {
				return nil, status.Error(codes.Internal, fmt.Sprintf("unable to detach loop device '%s': %s",
					loopDevice.Name, err.Error()))
			}
		} else {
			// We shall never be in this state, so error out.
			Logc(ctx).WithFields(LogFields{
				"loopFile":   loopFile,
				"loopDevice": loopDevice.Name,
			}).Error("Loop device is still mounted, skipping detach.")
			return nil, status.Error(codes.FailedPrecondition,
				fmt.Sprintf("unable to detach loop device '%s'; it is still mounted", loopDevice.Name))
		}
	}

	// Identify if the NFS mount point has any subvolumes in use by loop devices, if not then it is safe to remove it.
	if utils.SafeToRemoveNFSMount(ctx, nfsMountpoint) {
		if err = utils.RemoveMountPointRetry(ctx, nfsMountpoint); err != nil {
			Logc(ctx).Errorf("Failed to remove NFS mountpoint '%s': %s", nfsMountpoint, err.Error())
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	// Delete the device info we saved to the volume tracking info path so unstage can succeed.
	if err = p.nodeHelper.DeleteTrackingInfo(ctx, volumeId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (p *Plugin) nodePublishNFSBlockVolume(
	ctx context.Context, req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {
	// Ensure AccessMode is RWO or RWOP.
	csiAccessMode := p.getAccessForCSIAccessMode(req.GetVolumeCapability().GetAccessMode().Mode)
	if p.containsMultiNodeAccessMode([]tridentconfig.AccessMode{csiAccessMode}) {
		return nil, status.Errorf(codes.InvalidArgument, "volume cannot be mounted using multi-node access mode")
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

	publishInfo.SubvolumeMountOptions = getSubvolumeMountOptions(false, publishInfo.SubvolumeMountOptions)

	err = utils.MountDevice(ctx, publishInfo.StagingMountpoint, req.TargetPath, publishInfo.SubvolumeMountOptions,
		false)
	if err != nil {
		if os.IsPermission(err) {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
		if strings.Contains(err.Error(), "invalid argument") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// For read only behavior need to remount the bind mount for utils-linux < v2.27
	// TODO (arorar): Verify if using `-o bind,ro` during initial mount is acceptable instead of a remount,
	//                based on below commit it is now supported in util-linux >= 2.27:
	// https://git.kernel.org/pub/scm/utils/util-linux/util-linux.git/commit/?id=9ac77b8a78452eab0612523d27fee52159f5016a
	if req.GetReadonly() {
		publishInfo.SubvolumeMountOptions = getSubvolumeMountOptions(true, publishInfo.SubvolumeMountOptions)

		err = utils.RemountDevice(ctx, req.TargetPath, publishInfo.SubvolumeMountOptions)
		if err != nil {
			if os.IsPermission(err) {
				return nil, status.Error(codes.PermissionDenied, err.Error())
			}
			if strings.Contains(err.Error(), "invalid argument") {
				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	err = p.nodeHelper.AddPublishedPath(ctx, req.VolumeId, req.TargetPath)
	if err != nil {
		return nil, err
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func getSubvolumeMountOptions(readOnly bool, subvolumeMountOptions string) string {
	if !utils.AreMountOptionsInList(subvolumeMountOptions, []string{"bind"}) {
		subvolumeMountOptions = utils.AppendToStringList(subvolumeMountOptions, "bind", ",")
	}

	if readOnly && !utils.AreMountOptionsInList(subvolumeMountOptions, []string{"ro", "remount"}) {
		subvolumeMountOptions = utils.AppendToStringList(subvolumeMountOptions, "remount", ",")
		subvolumeMountOptions = utils.AppendToStringList(subvolumeMountOptions, "ro", ",")
	}

	return subvolumeMountOptions
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
	if node.PublicationState == utils.NodeClean {
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
func (p *Plugin) discoverDesiredPublicationState(ctx context.Context) (map[string]*utils.VolumePublicationExternal, error) {
	Logc(ctx).Debug("Discovering desired publication state.")

	publications, err := p.restClient.ListVolumePublicationsForNode(ctx, p.nodeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get desired publication state")
	}

	desiredPublicationState := make(map[string]*utils.VolumePublicationExternal, len(publications))
	for _, pub := range publications {
		desiredPublicationState[pub.VolumeName] = pub
	}

	return desiredPublicationState, nil
}

// discoverActualPublicationState discovers the actual state of published volumes on the node and returns
// a mapping of volumeID -> tracking information.
func (p *Plugin) discoverActualPublicationState(ctx context.Context) (map[string]*utils.VolumeTrackingInfo, error) {
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
	actualPublicationState map[string]*utils.VolumeTrackingInfo,
	desiredPublicationState map[string]*utils.VolumePublicationExternal,
) map[string]*utils.VolumeTrackingInfo {
	Logc(ctx).Debug("Discovering stale volume publications.")

	// Track the delta between actual (node-side) and desired (controller-side) publication state.
	stalePublications := make(map[string]*utils.VolumeTrackingInfo, 0)

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
func (p *Plugin) cleanStalePublications(ctx context.Context, stalePublications map[string]*utils.VolumeTrackingInfo) error {
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

func (p *Plugin) updateNodePublicationState(ctx context.Context, nodeState utils.NodePublicationState) error {
	// Check if node is cleanable in the first place and bail if Trident shouldn't mark it as clean.
	if nodeState != utils.NodeCleanable {
		Logc(ctx).Debugf("Controller node state is not cleanable; state was: [%s]", nodeState)
		return nil
	}

	Logc(ctx).Debug("Updating node publication state.")
	nodeStateFlags := &utils.NodePublicationStateFlags{
		ProvisionerReady: utils.Ptr(true),
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
func (p *Plugin) selfHealingRectifySession(ctx context.Context, portal string, action utils.ISCSIAction) error {
	var err error
	var volumeID string
	var publishInfo utils.VolumePublishInfo
	iSCSILoginTimeout := 10 * time.Second

	if portal == "" {
		return fmt.Errorf("portal value is empty")
	}
	Logc(ctx).WithFields(LogFields{
		"portal": portal,
		"action": action,
	}).Debug("ISCSI self-healing rectify session is invoked.")

	if publishInfo, err = publishedISCSISessions.GeneratePublishInfo(portal); err != nil {
		return err
	}

	switch action {
	case utils.LogoutLoginScan:
		if err = utils.ISCSILogout(ctx, publishInfo.IscsiTargetIQN, portal); err != nil {
			return fmt.Errorf("error while logging out of target %s", publishInfo.IscsiTargetIQN)
		} else {
			Logc(ctx).Debug("Logout is successful.")
		}
		// Logout is successful, fallthrough to perform login.
		fallthrough
	case utils.LoginScan:
		// Set FilesystemType to "raw" so that we only heal the session connectivity and not perform the mount and
		// filesystem related operations.
		publishInfo.FilesystemType = tridentconfig.FsRaw

		if volumeID, err = publishedISCSISessions.VolumeIDForPortal(portal); err != nil {
			return err
		}
		volContext := map[string]string{"internalName": volumeID}

		req := &csi.NodeStageVolumeRequest{
			VolumeId:      volumeID,
			VolumeContext: volContext,
			Secrets:       map[string]string{},
		}

		publishedCHAPCredentials := publishInfo.IscsiChapInfo

		if _, err = p.ensureAttachISCSIVolume(ctx, req, "", &publishInfo, iSCSILoginTimeout); err != nil {
			return fmt.Errorf("failed to login to the target")
		}

		if publishedCHAPCredentials != publishInfo.IscsiChapInfo {
			updateErr := publishedISCSISessions.UpdateCHAPForPortal(portal, publishInfo.IscsiChapInfo)
			if updateErr != nil {
				Logc(ctx).Warn("Failed to update published CHAP information.")
			}

			Logc(ctx).Debug("Updated published CHAP information.")
		}

		Logc(ctx).Debug("Login to target is successful.")
		// Login is successful, fallthrough to perform scan
		fallthrough
	case utils.Scan:
		if err = utils.InitiateScanForAllLUNs(ctx, publishInfo.IscsiTargetIQN); err != nil {
			Logc(ctx).Debug("Error while rescanning of LUNs.")
		}

		Logc(ctx).Debug("Scanning of LUNs is successful.")
	default:
		Logc(ctx).Debug("No valid action to be taken in iSCSI self-healing.")
	}

	return nil
}

// performISCSISelfHealing inspects the desired state of the iSCSI sessions with the current state and accordingly
// identifies candidate sessions that require remediation. This function is invoked periodically.
func (p *Plugin) performISCSISelfHealing(ctx context.Context) {
	utils.Lock(ctx, iSCSISelfHealingLockContext, lockID)
	defer utils.Unlock(ctx, iSCSISelfHealingLockContext, lockID)

	defer func() {
		if r := recover(); r != nil {
			Logc(ctx).Errorf("Panic in iSCSISelfHealing. \nStack Trace: %v", string(debug.Stack()))
			return
		}
	}()

	// After this time self-healing may be stopped
	stopSelfHealingAt := time.Now().Add(60 * time.Second)

	// If there are not iSCSI volumes expected on the host skip self-healing
	if publishedISCSISessions.IsEmpty() {
		Logc(ctx).Debug("Skipping iSCSI self-heal cycle; no iSCSI volumes published on the host.")
		return
	}

	if err := utils.ISCSIPreChecks(ctx); err != nil {
		Logc(ctx).Errorf("Skipping iSCSI self-heal cycle; pre-checks failed: %v.", err)
	}

	// Reset current sessions
	currentISCSISessions = utils.ISCSISessions{}

	// Reset published iSCSI session remediation information
	if err := publishedISCSISessions.ResetAllRemediationValues(); err != nil {
		Logc(ctx).WithError(err).Error("Failed to reset remediation value(s) for published iSCSI sessions. ")
	}

	if err := utils.PopulateCurrentSessions(ctx, &currentISCSISessions); err != nil {
		Logc(ctx).WithError(err).Error("Failed to get current state of iSCSI Sessions LUN mappings; skipping iSCSI self-heal cycle.")
		return
	}

	if currentISCSISessions.IsEmpty() {
		Logc(ctx).Debug("No iSCSI sessions LUN mappings found.")
	}

	Logc(ctx).Debugf("\nPublished iSCSI Sessions: %v", publishedISCSISessions)
	Logc(ctx).Debugf("\n\nCurrent iSCSI Sessions: %v", currentISCSISessions)

	// The problems self-heal aims to resolve can be divided into two buckets. A stale portal bucket
	// and a non-stale portal bucket. Stale portals are the sessions that were healthy at some point
	// in time, then due to temporary network connectivity issue along with CHAP rotation made them
	// un-recoverable by themselves and thus require intervention.
	// Issues in non-stale portal bucket would include sessions that were never established due
	// to temporary networking issue or logged-out sessions or LUNs that were never scanned for some sessions.

	// SELF-HEAL STEP 1: Identify all sorted candidate stale portals and sorted candidate non-stale portals.
	staleISCSIPortals, nonStaleISCSIPortals := utils.InspectAllISCSISessions(ctx, &publishedISCSISessions,
		&currentISCSISessions, p.iSCSISelfHealingWaitTime)

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

	for idx, portal := range portals {

		// First get the fix action
		fixAction := publishedISCSISessions.Info[portal].Remediation
		isNonStaleSessionFix := fixAction != utils.LogoutLoginScan

		// Check if there is a need to stop the loop from running
		// NOTE: The loop should run at least once for all portal types.
		if idx > 0 && utils.WaitQueueSize(lockID) > 0 {
			// Check to see if some other operation(s) requires node lock, if not then continue to resolve
			// non-stale iSCSI portal issues else break out of this loop.
			if isNonStaleSessionFix {
				Logc(ctx).Debug("Identified other node operations waiting for the node lock; preempting non-stale" +
					" iSCSI session self-healing.")
				break
			} else if time.Now().After(stopAt) {
				Logc(ctx).Debug("Self-healing has exceeded maximum runtime; preempting iSCSI session self-healing.")
				break
			}
		}

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
	publishInfo *utils.VolumePublishInfo,
) error {
	var isLUKS bool
	var err error

	if req.PublishContext["LUKSEncryption"] != "" {
		isLUKS, err = strconv.ParseBool(req.PublishContext["LUKSEncryption"])
		if err != nil {
			return fmt.Errorf("could not parse LUKSEncryption into a bool, got %v", publishInfo.LUKSEncryption)
		}
	}

	publishInfo.LUKSEncryption = strconv.FormatBool(isLUKS)
	publishInfo.MountOptions = req.PublishContext["mountOptions"]
	publishInfo.NVMeSubsystemNQN = req.PublishContext["nvmeSubsystemNqn"]
	publishInfo.NVMeNamespaceUUID = req.PublishContext["nvmeNamespaceUUID"]
	publishInfo.NVMeTargetIPs = strings.Split(req.PublishContext["nvmeTargetIPs"], ",")
	publishInfo.SANType = req.PublishContext["SANType"]

	if err := utils.AttachNVMeVolumeRetry(ctx, req.VolumeContext["internalName"], "", publishInfo, nil,
		utils.NVMeAttachTimeout); err != nil {
		return err
	}

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return err
	}

	if isLUKS {
		luksDevice, err := utils.NewLUKSDeviceFromMappingPath(ctx, publishInfo.DevicePath,
			req.VolumeContext["internalName"])
		if err != nil {
			return err
		}
		// Ensure we update the passphrase in case it has never been set before
		err = ensureLUKSVolumePassphrase(ctx, p.restClient, luksDevice, volumeId, req.GetSecrets(), true)
		if err != nil {
			return fmt.Errorf("could not set LUKS volume passphrase")
		}
	}

	volTrackingInfo := &utils.VolumeTrackingInfo{
		VolumePublishInfo: *publishInfo,
		StagingTargetPath: stagingTargetPath,
		PublishedPaths:    map[string]struct{}{},
	}

	// Save the device info to the volume tracking info path for use in the publish & unstage calls.
	if err := p.nodeHelper.WriteTrackingInfo(ctx, volumeId, volTrackingInfo); err != nil {
		return err
	}

	p.nvmeHandler.AddPublishedNVMeSession(&publishedNVMeSessions, publishInfo)

	return nil
}

// nodeUnstageNVMEVolume unstages volume for nvme driver.
// - Get the device from tracking info OR from "nvme netapp ontapdevices".
// - Flush the device using "nvme flush /dev/<nvme-device>"
// - If the subsystem has <= 1 Namespaces connected to it, then disconnect.
// - Remove the tracking file for the volume/Namespace.
func (p *Plugin) nodeUnstageNVMeVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest, publishInfo *utils.VolumePublishInfo, force bool,
) (*csi.NodeUnstageVolumeResponse, error) {
	disconnect := p.nvmeHandler.RemovePublishedNVMeSession(&publishedNVMeSessions, publishInfo.NVMeSubsystemNQN,
		publishInfo.NVMeNamespaceUUID)

	// Get the device using 'nvme-cli' commands. Flush the device IOs.
	nvmeDev, err := p.nvmeHandler.NewNVMeDevice(ctx, publishInfo.NVMeNamespaceUUID)
	// Proceed further with Unstage flow, if 'device is not found'.
	if err != nil && !errors.IsNotFoundError(err) {
		return nil, fmt.Errorf("error while getting NVMe device, %v", err)
	}

	if !nvmeDev.IsNil() {
		// If device is found, proceed to flush and clean up.
		err := nvmeDev.FlushDevice(ctx, p.unsafeDetach, force)
		// If flush fails, give a grace period of 6 minutes (nvmeMaxFlushWaitDuration) before giving up.
		if err != nil {
			if NVMeNamespacesFlushRetry[publishInfo.NVMeNamespaceUUID].IsZero() {
				NVMeNamespacesFlushRetry[publishInfo.NVMeNamespaceUUID] = time.Now()
				return nil, fmt.Errorf("error while flushing NVMe device, %v", err)
			} else {
				elapsed := time.Since(NVMeNamespacesFlushRetry[publishInfo.NVMeNamespaceUUID])
				if elapsed > nvmeMaxFlushWaitDuration {
					// Waited enough, log it and proceed with next step in detach flow.
					Logc(ctx).WithFields(
						LogFields{
							"namespace": publishInfo.NVMeNamespaceUUID,
							"elapsed":   elapsed,
							"maxWait":   nvmeMaxFlushWaitDuration,
						}).Debug("Volume is not safe to be detached. But, waited enough time.")
					// Done with this, remove entry from exceptions list.
					delete(NVMeNamespacesFlushRetry, publishInfo.NVMeNamespaceUUID)
				} else {
					// Allowing to wait for some more time. Let the kubelet retry.
					Logc(ctx).WithFields(
						LogFields{
							"namespace": publishInfo.NVMeNamespaceUUID,
							"elapsed":   elapsed,
						}).Debug("Waiting for some more time.")
					return nil, fmt.Errorf("error while flushing NVMe device, %v", err)
				}
			}
		} else {
			// No error in 'flush', remove entry from exceptions list in case it was added earlier.
			delete(NVMeNamespacesFlushRetry, publishInfo.NVMeNamespaceUUID)
		}
	}

	// Get the number of namespaces associated with the subsystem
	nvmeSubsys := p.nvmeHandler.NewNVMeSubsystem(ctx, publishInfo.NVMeSubsystemNQN)
	numNs, err := nvmeSubsys.GetNamespaceCount(ctx)
	if err != nil {
		Logc(ctx).WithFields(
			LogFields{
				"subsystem": publishInfo.NVMeSubsystemNQN,
				"error":     err,
			}).Debug("Error getting Namespace count.")
	}

	// If number of namespaces is more than 1, don't disconnect the subsystem. If we get any issues while getting the
	// number of namespaces through the CLI, we can rely on the disconnect flag from NVMe self-healing sessions (if
	// NVMe self-healing is enabled), which keeps track of namespaces associated with the subsystem.
	if (err == nil && numNs <= 1) || (p.nvmeSelfHealingInterval > 0 && err != nil && disconnect) {
		if err := nvmeSubsys.Disconnect(ctx); err != nil {
			Logc(ctx).WithFields(
				LogFields{
					"subsystem": publishInfo.NVMeSubsystemNQN,
					"error":     err,
				}).Debug("Error disconnecting subsystem.")
		}
	}

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return nil, err
	}

	// Ensure that the temporary mount point created during a filesystem expand operation is removed.
	if err := utils.UmountAndRemoveTemporaryMountPoint(ctx, stagingTargetPath); err != nil {
		Logc(ctx).WithField("stagingTargetPath", stagingTargetPath).Errorf(
			"Failed to remove directory in staging target path; %s", err)
		errStr := fmt.Sprintf("failed to remove temporary directory in staging target path %s; %s",
			stagingTargetPath, err)
		return nil, status.Error(codes.Internal, errStr)
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
		publishInfo.MountOptions = utils.AppendToStringList(publishInfo.MountOptions, "ro", ",")
	}
	if publishInfo.LUKSEncryption != "" {
		isLUKS, err := strconv.ParseBool(publishInfo.LUKSEncryption)
		if err != nil {
			return nil, fmt.Errorf("could not parse LUKSEncryption into a bool, got %v", publishInfo.LUKSEncryption)
		}
		if isLUKS {
			// Rotate the LUKS passphrase if needed, on failure, log and continue to publish
			luksDevice, err := utils.NewLUKSDeviceFromMappingPath(ctx, publishInfo.DevicePath,
				req.VolumeContext["internalName"])
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			err = ensureLUKSVolumePassphrase(ctx, p.restClient, luksDevice, req.GetVolumeId(), req.GetSecrets(), false)
			if err != nil {
				Logc(ctx).WithError(err).Error("Failed to ensure current LUKS passphrase.")
			}
		}
	}
	isRawBlock := publishInfo.FilesystemType == tridentconfig.FsRaw
	if isRawBlock {

		if len(publishInfo.MountOptions) > 0 {
			publishInfo.MountOptions = utils.AppendToStringList(publishInfo.MountOptions, "bind", ",")
		} else {
			publishInfo.MountOptions = "bind"
		}

		// Place the block device at the target path for the raw-block.
		err = utils.MountDevice(ctx, publishInfo.DevicePath, req.TargetPath, publishInfo.MountOptions, true)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "unable to bind mount raw device; %s", err)
		}
	} else {
		// Mount the device.
		err = utils.MountDevice(ctx, publishInfo.DevicePath, req.TargetPath, publishInfo.MountOptions, false)
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

func (p *Plugin) nodeStageSANVolume(ctx context.Context, req *csi.NodeStageVolumeRequest,
) (*csi.NodeStageVolumeResponse, error) {
	var err error
	var fstype string

	mountCapability := req.GetVolumeCapability().GetMount()
	blockCapability := req.GetVolumeCapability().GetBlock()

	if mountCapability == nil && blockCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "mount or block capability required")
	} else if mountCapability != nil && blockCapability != nil {
		return nil, status.Error(codes.InvalidArgument, "mixed block and mount capabilities")
	}

	if mountCapability != nil && mountCapability.GetFsType() != "" {
		fstype = mountCapability.GetFsType()
	}

	if fstype == "" {
		fstype = req.PublishContext["filesystemType"]
	}

	if fstype == tridentconfig.FsRaw && mountCapability != nil {
		return nil, status.Error(codes.InvalidArgument, "mount capability requested with raw blocks")
	} else if fstype != tridentconfig.FsRaw && blockCapability != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("block capability requested with %s", fstype))
	}

	sharedTarget, err := strconv.ParseBool(req.PublishContext["sharedTarget"])
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	publishInfo := &utils.VolumePublishInfo{
		Localhost:      true,
		FilesystemType: fstype,
		SharedTarget:   sharedTarget,
	}

	if req.PublishContext["SANType"] == sa.NVMe {
		if err := p.nodeStageNVMeVolume(ctx, req, publishInfo); err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("failed to stage volume: %s", err.Error()))
		}
	} else {
		if err = p.nodeStageISCSIVolume(ctx, req, publishInfo); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

// performNVMeSelfHealing inspects the desired state of the NVMe sessions with the current state and accordingly
// identifies candidate sessions that require remediation. This function is invoked periodically.
func (p *Plugin) performNVMeSelfHealing(ctx context.Context) {
	utils.Lock(ctx, nvmeSelfHealingLockContext, lockID)
	defer utils.Unlock(ctx, nvmeSelfHealingLockContext, lockID)

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
	currentNVMeSessions = utils.NVMeSessions{}

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

func (p *Plugin) fixNVMeSessions(ctx context.Context, stopAt time.Time, subsystems []utils.NVMeSubsystem) {
	for index, sub := range subsystems {
		// If the subsystem is not present in the published NVMe sessions, we don't need to rectify its sessions.
		if !publishedNVMeSessions.CheckNVMeSessionExists(sub.NQN) {
			continue
		}

		// 1. We should fix at least one subsystem in a single self-healing thread.
		// 2. If there's another thread waiting for the node lock and if we have exceeded our 60 secs lock, we should
		//    stop NVMe self-healing.
		if index > 0 && utils.WaitQueueSize(lockID) > 0 && time.Now().After(stopAt) {
			Logc(ctx).Info("Self-healing has exceeded maximum runtime; preempting NVMe session self-healing.")
			break
		}

		p.nvmeHandler.RectifyNVMeSession(ctx, sub, &publishedNVMeSessions)
	}
}
