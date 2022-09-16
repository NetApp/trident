// Copyright 2022 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/utils"
)

const (
	tridentDeviceInfoPath         = "/var/lib/trident/tracking"
	lockID                        = "csi_node_server"
	volumePublishInfoFilename     = "volumePublishInfo.json"
	nodePrepBreadcrumbFilename    = "nodePrepInfo.json"
	AttachISCSIVolumeTimeoutShort = 15 * time.Second
	iSCSINodeUnstageMaxDuration   = 15 * time.Second
)

var topologyLabels = make(map[string]string)

func (p *Plugin) NodeStageVolume(
	ctx context.Context, req *csi.NodeStageVolumeRequest,
) (*csi.NodeStageVolumeResponse, error) {
	lockContext := "NodeStageVolume-" + req.GetVolumeId()
	utils.Lock(ctx, lockContext, lockID)
	defer utils.Unlock(ctx, lockContext, lockID)

	fields := log.Fields{"Method": "NodeStageVolume", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Debug(">>>> NodeStageVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NodeStageVolume")

	switch req.PublishContext["protocol"] {
	case string(tridentconfig.File):
		return p.nodeStageNFSVolume(ctx, req)
	case string(tridentconfig.Block):
		return p.nodeStageISCSIVolume(ctx, req)
	case string(tridentconfig.BlockOnFile):
		return p.nodeStageNFSBlockVolume(ctx, req)
	default:
		return nil, status.Error(codes.InvalidArgument, "unknown protocol")
	}
}

func (p *Plugin) NodeUnstageVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest,
) (*csi.NodeUnstageVolumeResponse, error) {
	return p.nodeUnstageVolume(ctx, req, false)
}

// nodeUnstageVolume detaches a volume from a node. Setting force=true may cause data loss.
func (p *Plugin) nodeUnstageVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest, force bool,
) (response *csi.NodeUnstageVolumeResponse, responseErr error) {
	lockContext := "NodeUnstageVolume-" + req.GetVolumeId()
	utils.Lock(ctx, lockContext, lockID)
	defer utils.Unlock(ctx, lockContext, lockID)

	fields := log.Fields{
		"Method": "NodeUnstageVolume",
		"Type":   "CSI_Node",
		"Force":  force,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> NodeUnstageVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NodeUnstageVolume")

	defer func() {
		// If the unstage operation succeeded, also remove the tracking file from Trident's volume tracking location.
		if responseErr == nil {
			_ = p.clearStagedTrackingFile(ctx, req.VolumeId)
		}
	}()

	_, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return nil, err
	}

	// Read the device info from the staging path.
	publishInfo, err := p.readStagedDeviceInfo(ctx, stagingTargetPath)
	if err != nil {
		if utils.IsNotFoundError(err) {
			// If the file doesn't exist, there is nothing we can do and a retry is unlikely to help, so return success.
			Logc(ctx).WithFields(fields).Warning("Staged device info file not found, returning success.")
			return &csi.NodeUnstageVolumeResponse{}, nil
		} else {
			Logc(ctx).WithFields(fields).WithError(err).Errorf("Unable to read the staging path %s", stagingTargetPath)
			return nil, status.Errorf(codes.Internal, "unable to read the staging path %s; %s", stagingTargetPath, err)
		}
	}

	protocol, err := p.getVolumeProtocolFromPublishInfo(publishInfo)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "unable to read protocol info from publish info; %s", err)
	}

	switch protocol {
	case tridentconfig.File:
		if force {
			Logc(ctx).WithFields(fields).WithField("protocol", tridentconfig.File).
				Warning("forced unstage not supported for this protocol")
		}
		return p.nodeUnstageNFSVolume(ctx, req)
	case tridentconfig.Block:
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
	lockContext := "NodePublishVolume-" + req.GetVolumeId()
	utils.Lock(ctx, lockContext, lockID)
	defer utils.Unlock(ctx, lockContext, lockID)

	fields := log.Fields{"Method": "NodePublishVolume", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Debug(">>>> NodePublishVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NodePublishVolume")

	switch req.PublishContext["protocol"] {
	case string(tridentconfig.File):
		return p.nodePublishNFSVolume(ctx, req)
	case string(tridentconfig.Block):
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
	lockContext := "NodeUnpublishVolume-" + req.GetVolumeId()
	utils.Lock(ctx, lockContext, lockID)
	defer utils.Unlock(ctx, lockContext, lockID)

	fields := log.Fields{"Method": "NodeUnpublishVolume", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Debug(">>>> NodeUnpublishVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NodeUnpublishVolume")

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
			Logc(ctx).WithFields(log.Fields{"path": targetPath, "error": err}).Error("unable to unmount volume.")
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

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (p *Plugin) NodeGetVolumeStats(
	ctx context.Context, req *csi.NodeGetVolumeStatsRequest,
) (*csi.NodeGetVolumeStatsResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume id provided")
	}

	if req.GetVolumePath() == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume path provided")
	}

	// Ensure volume is published at path
	_, err := os.Stat(req.GetVolumePath())
	if err != nil {
		return nil, status.Error(codes.NotFound,
			fmt.Sprintf("could not find volume mount at path: %s; %v ", req.GetVolumePath(), err))
	}

	// If raw block volume, dont return usage
	isRawBlock := false
	if req.StagingTargetPath != "" {
		publishInfo, err := p.readStagedDeviceInfo(ctx, req.StagingTargetPath)
		if err != nil {
			if utils.IsNotFoundError(err) {
				return nil, status.Error(codes.FailedPrecondition, err.Error())
			} else {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}

		isRawBlock = publishInfo.FilesystemType == tridentconfig.FsRaw
	}
	if isRawBlock {
		// Return no capacity info for raw block volumes, we cannot reliably determine the capacity
		return &csi.NodeGetVolumeStatsResponse{}, nil
	} else {
		// If filesystem, return usage reported by FS
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
	fields := log.Fields{"Method": "NodeExpandVolume", "Type": "CSI_Node"}
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

	requiredBytes := req.GetCapacityRange().GetRequiredBytes()
	limitBytes := req.GetCapacityRange().GetLimitBytes()

	Logc(ctx).WithFields(log.Fields{
		"volumeId":      volumeId,
		"volumePath":    volumePath,
		"requiredBytes": requiredBytes,
		"limitBytes":    limitBytes,
	}).Debug("NodeExpandVolumeRequest values")

	// The CSI spec does not specify what the CO may pass as the staging_target_path. This is why the tracking file
	// exists req order to locate the VolumePublishInfo file.
	// If the PV was staged with a Trident version prior to 19.10 then the tracking file will not exist. The
	// stagingTargetPath is likely to be the directory where the VolumePublishInfo file is located so we will attempt
	// to set the stagingTargetPath to this location if the tracking file does not exist.
	stagingTargetPath, err := p.readStagedTrackingFile(ctx, volumeId)
	if err != nil {
		if utils.IsNotFoundError(err) {
			// Verify the volumePath is the stagingTargetPath
			filePath := path.Join(volumePath, volumePublishInfoFilename)
			if _, err = os.Stat(filePath); !os.IsNotExist(err) {
				stagingTargetPath = volumePath
			} else {
				Logc(ctx).WithField("filePath", filePath).Errorf("Unable to find volumePublishInfo.")
				return nil, status.Errorf(codes.NotFound, "unable to find volume publish info needed for resize")
			}
		} else {
			return nil, status.Errorf(codes.Internal, err.Error())
		}
	}

	// Current K8S behavior is to send the volumePath as the stagingTargetPath. Log what is received if the
	// two variables don't match.
	if stagingTargetPath != volumePath {
		Logc(ctx).WithFields(log.Fields{
			"stagingTargetPath": stagingTargetPath,
			"volumePath":        volumePath,
			"volumeId":          volumeId,
		}).Info("Received something other than the expected stagingTargetPath.")
	}

	publishInfo, err := p.readStagedDeviceInfo(ctx, stagingTargetPath)
	if err != nil {
		if utils.IsNotFoundError(err) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	err = p.nodeExpandVolume(ctx, publishInfo, requiredBytes, stagingTargetPath, volumeId)
	if err != nil {
		return nil, err
	}
	return &csi.NodeExpandVolumeResponse{}, nil
}

func (p *Plugin) nodeExpandVolume(
	ctx context.Context, publishInfo *utils.VolumePublishInfo, requiredBytes int64, stagingTargetPath string,
	volumeId string,
) error {
	protocol, err := p.getVolumeProtocolFromPublishInfo(publishInfo)
	if err != nil {
		return status.Errorf(codes.FailedPrecondition, "unable to read protocol info from publish info; %s", err)
	}

	var fsType, mountOptions string

	switch protocol {
	case tridentconfig.File:
		return status.Error(codes.InvalidArgument, "NFS does not need to explicitly expand the filesystem")
	case tridentconfig.Block:
		if fsType, err = utils.VerifyFilesystemSupport(publishInfo.FilesystemType); err != nil {
			break
		}
		err = nodePrepareISCSIVolumeForExpansion(ctx, publishInfo, requiredBytes)
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

	// Expand filesystem
	if fsType != tridentconfig.FsRaw {
		filesystemSize, err := utils.ExpandFilesystemOnNode(ctx, publishInfo, stagingTargetPath, fsType,
			mountOptions)
		if err != nil {
			Logc(ctx).WithFields(log.Fields{
				"device":         publishInfo.DevicePath,
				"filesystemType": fsType,
			}).WithError(err).Error("Unable to expand filesystem.")
			return status.Error(codes.Internal, err.Error())
		}
		Logc(ctx).WithFields(log.Fields{
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

	Logc(ctx).WithFields(log.Fields{
		"targetIQN":      publishInfo.IscsiTargetIQN,
		"lunID":          lunID,
		"devicePath":     publishInfo.DevicePath,
		"mountOptions":   publishInfo.MountOptions,
		"filesystemType": publishInfo.FilesystemType,
	}).Debug("PublishInfo for block device to expand.")

	var err error

	// Make sure device is ready
	if utils.IsAlreadyAttached(ctx, lunID, publishInfo.IscsiTargetIQN) {
		// Rescan device to detect increased size
		if err = utils.ISCSIRescanDevices(
			ctx, publishInfo.IscsiTargetIQN, publishInfo.IscsiLunNumber, requiredBytes); err != nil {
			Logc(ctx).WithFields(log.Fields{
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
	Logc(ctx).WithFields(log.Fields{
		"nfsUniqueID":   publishInfo.NfsUniqueID,
		"nfsPath":       publishInfo.NfsPath,
		"subvolumeName": publishInfo.SubvolumeName,
		"devicePath":    publishInfo.DevicePath,
	}).Debug("PublishInfo for device to expand.")

	nfsMountpoint := publishInfo.NFSMountpoint
	loopFile := path.Join(nfsMountpoint, publishInfo.SubvolumeName)

	var err error = nil

	loopDeviceAttached, err := utils.IsLoopDeviceAttachedToFile(ctx, publishInfo.DevicePath, loopFile)
	if err != nil {
		return fmt.Errorf("unable to identify if loop device '%s' is attached to file '%s': %v",
			publishInfo.DevicePath, loopFile, err)
	}

	// Make sure device is ready
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
	fields := log.Fields{"Method": "NodeGetCapabilities", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Debug(">>>> NodeGetCapabilities")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NodeGetCapabilities")

	return &csi.NodeGetCapabilitiesResponse{Capabilities: p.nsCap}, nil
}

func (p *Plugin) NodeGetInfo(
	ctx context.Context, _ *csi.NodeGetInfoRequest,
) (*csi.NodeGetInfoResponse, error) {
	fields := log.Fields{"Method": "NodeGetInfo", "Type": "CSI_Node"}
	Logc(ctx).WithFields(fields).Debug(">>>> NodeGetInfo")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NodeGetInfo")

	topology := &csi.Topology{
		Segments: topologyLabels,
	}

	return &csi.NodeGetInfoResponse{NodeId: p.nodeName, AccessibleTopology: topology}, nil
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
				log.Fields{
					"distro":  host.OS.Distro,
					"version": host.OS.Version,
				}).Debug("Discovered host info.")
		}
	}

	iscsiWWN := ""
	iscsiWWNs, err := utils.GetInitiatorIqns(ctx)
	if err != nil {
		Logc(ctx).WithField("error", err).Warn("Problem getting iSCSI initiator name.")
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

	// Discover active protocol services on the host
	var services []string
	nfsActive, err := utils.NFSActiveOnHost(ctx)
	if err != nil {
		Logc(ctx).WithError(err).Warn("Error discovering NFS service on host.")
	}
	if nfsActive {
		services = append(services, "NFS")
	}

	iscsiActive, err := utils.ISCSIActiveOnHost(ctx, *p.hostInfo)
	if err != nil {
		Logc(ctx).WithError(err).Warn("Error discovering iSCSI service on host.")
	}
	if iscsiActive {
		services = append(services, "iSCSI")
	}
	p.hostInfo.Services = services

	// Generate node object
	node := &utils.Node{
		Name:     p.nodeName,
		IQN:      iscsiWWN,
		IPs:      ips,
		NodePrep: nil,
		HostInfo: p.hostInfo,
		Deleted:  false,
	}
	return node
}

func (p *Plugin) nodeRegisterWithController(ctx context.Context, timeout time.Duration) {
	// Assemble the node details that we will register with the controller
	node := p.nodeGetInfo(ctx)

	// The controller may not be fully initialized by the time the node is ready to register,
	// so retry until it is responding on the back channel and we have registered the node.
	registerNode := func() error {
		nodeDetails, err := p.restClient.CreateNode(ctx, node)
		if err == nil {
			topologyLabels = nodeDetails.TopologyLabels
			node.TopologyLabels = nodeDetails.TopologyLabels
		}
		return err
	}

	registerNodeNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(log.Fields{
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

	// Retry using an exponential backoff that never ends until it succeeds
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

	publishInfo.MountOptions = req.PublishContext["mountOptions"]
	publishInfo.NfsServerIP = req.PublishContext["nfsServerIp"]
	publishInfo.NfsPath = req.PublishContext["nfsPath"]

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return nil, err
	}

	// Save the device info to the staging path for use in the publish & unstage calls
	if err := p.writeStagedDeviceInfo(ctx, stagingTargetPath, publishInfo, volumeId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (p *Plugin) nodeUnstageNFSVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest,
) (*csi.NodeUnstageVolumeResponse, error) {
	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return nil, err
	}

	// Delete the device info we saved to the staging path so unstage can succeed
	if err := p.clearStagedDeviceInfo(ctx, stagingTargetPath, volumeId); err != nil {
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

	publishInfo, err := p.readStagedDeviceInfo(ctx, req.StagingTargetPath)
	if err != nil {
		if utils.IsNotFoundError(err) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

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

func (p *Plugin) nodeStageISCSIVolume(
	ctx context.Context, req *csi.NodeStageVolumeRequest,
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

	useCHAP, err := strconv.ParseBool(req.PublishContext["useCHAP"])
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	sharedTarget, err := strconv.ParseBool(req.PublishContext["sharedTarget"])
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	lunID, err := strconv.ParseInt(req.PublishContext["iscsiLunNumber"], 10, 0)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	publishInfo := &utils.VolumePublishInfo{
		Localhost:      true,
		FilesystemType: fstype,
		SharedTarget:   sharedTarget,
	}
	publishInfo.UseCHAP = useCHAP
	luks, ok := req.PublishContext["LUKSEncryption"]
	if ok && luks != "" {
		publishInfo.LUKSEncryption = luks
		// TODO(ameade): when luks volume publishing is supported, remove this
		luksBool, err := strconv.ParseBool(luks)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "could not parse LUKSEncryption into a bool, got %v", luks)
		}
		if luksBool {
			return nil, status.Error(codes.InvalidArgument, "cannot stage LUKS encrypted volumes")
		}
	}

	err = unstashIscsiTargetPortals(publishInfo, req.PublishContext)
	if nil != err {
		return nil, status.Error(codes.Internal, err.Error())
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
					return nil, status.Error(codes.Internal, err.Error())
				}
			}
		} else if encryptedCHAP {
			// Only error if the key is not set and the publishContext includes encrypted fields
			Logc(ctx).Warn(
				"Encryption key not set; cannot decrypt CHAP credentials; will retrieve from Trident controller.")
			if err = p.updateChapInfoFromController(ctx, req, publishInfo); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}
	}

	// Perform the login/rescan/discovery/(optionally)format, mount & get the device back in the publish info
	if err := utils.AttachISCSIVolumeRetry(ctx, req.VolumeContext["internalName"], "", publishInfo,
		AttachISCSIVolumeTimeoutShort); err != nil {
		// Did we fail to log in?
		if utils.IsAuthError(err) {
			// Update CHAP info from the controller and try one more time
			Logc(ctx).Warn("iSCSI login failed; will retrieve CHAP credentials from Trident controller and try again.")
			if err = p.updateChapInfoFromController(ctx, req, publishInfo); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			if err = utils.AttachISCSIVolumeRetry(ctx, req.VolumeContext["internalName"], "", publishInfo,
				AttachISCSIVolumeTimeoutShort); err != nil {
				// Bail out no matter what as we've now tried with updated credentials
				return nil, status.Error(codes.Internal, err.Error())
			}
		} else {
			return nil, status.Error(codes.Internal, fmt.Sprintf("failed to stage volume: %v", err))
		}
	}

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return nil, err
	}

	// Save the device info to the staging path for use in the publish & unstage calls
	if err := p.writeStagedDeviceInfo(ctx, stagingTargetPath, publishInfo, volumeId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeStageVolumeResponse{}, nil
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
	// Delete the device from the host
	err := utils.PrepareDeviceForRemoval(ctx, int(publishInfo.IscsiLunNumber), publishInfo.IscsiTargetIQN,
		p.unsafeDetach, force)
	if nil != err && !p.unsafeDetach {
		return status.Error(codes.Internal, err.Error())
	}

	// Get map of hosts and sessions for given Target IQN
	hostSessionMap := utils.GetISCSIHostSessionMapForTarget(ctx, publishInfo.IscsiTargetIQN)
	if len(hostSessionMap) == 0 {
		Logc(ctx).Warnf("no iSCSI hosts found for target %s", publishInfo.IscsiTargetIQN)
	}

	// Logout of the iSCSI session if appropriate for each applicable host
	logout := false
	for hostNumber, sessionNumber := range hostSessionMap {
		if !publishInfo.SharedTarget {
			// Always log out of a non-shared target
			logout = true
			break
		} else {
			// Log out of a shared target if no mounts to that target remain
			anyMounts, err := utils.ISCSITargetHasMountedDevice(ctx, publishInfo.IscsiTargetIQN)
			if logout = (err == nil) && !anyMounts && utils.SafeToLogOut(ctx, hostNumber, sessionNumber); logout {
				break
			}
		}
	}

	if logout {
		Logc(ctx).Debug("Safe to log out")

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

	// Delete the device info we saved to the staging path so unstage can succeed
	if err := p.clearStagedDeviceInfo(ctx, stagingTargetPath, volumeId); err != nil {
		return status.Error(codes.Internal, err.Error())
	}

	// Ensure that the temporary mount point created during a filesystem expand operation is removed.
	if err := utils.UmountAndRemoveTemporaryMountPoint(ctx, stagingTargetPath); err != nil {
		Logc(ctx).WithField("stagingTargetPath", stagingTargetPath).Errorf(
			"Failed to remove directory in staging target path; %s", err)
		return status.Error(codes.Internal, fmt.Sprintf("failed to remove temporary directory "+
			"in staging target path %s; %s", stagingTargetPath, err))
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
	var err error

	// Read the device info from the staging path
	publishInfo, err := p.readStagedDeviceInfo(ctx, req.StagingTargetPath)
	if err != nil {
		if utils.IsNotFoundError(err) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if req.GetReadonly() {
		publishInfo.MountOptions = utils.AppendToStringList(publishInfo.MountOptions, "ro", ",")
	}

	isRawBlock := publishInfo.FilesystemType == tridentconfig.FsRaw
	if isRawBlock {

		if len(publishInfo.MountOptions) > 0 {
			publishInfo.MountOptions = utils.AppendToStringList(publishInfo.MountOptions, "bind", ",")
		} else {
			publishInfo.MountOptions = "bind"
		}

		// Place the block device at the target path for the raw-block
		err = utils.MountDevice(ctx, publishInfo.DevicePath, req.TargetPath, publishInfo.MountOptions, true)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "unable to bind mount raw device; %s", err)
		}
	} else {
		// Mount the device
		err = utils.MountDevice(ctx, publishInfo.DevicePath, req.TargetPath, publishInfo.MountOptions, false)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "unable to mount device; %s", err)
		}
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (p *Plugin) nodeStageNFSBlockVolume(ctx context.Context, req *csi.NodeStageVolumeRequest,
) (*csi.NodeStageVolumeResponse, error) {
	// Get VolumeID and Staging Path
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
	publishInfo.NFSMountpoint = path.Join(tridentDeviceInfoPath, publishInfo.NfsUniqueID, publishInfo.NfsPath)

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
	err = p.nodeExpandVolume(ctx, publishInfo, loopFileSize, stagingTargetPath, volumeId)
	if err != nil {
		return nil, err
	}

	// Save the device info to the staging path for use in the publish & unstage calls
	if err := p.writeStagedDeviceInfo(ctx, stagingTargetPath, publishInfo, volumeId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (p *Plugin) nodeUnstageNFSBlockVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest, publishInfo *utils.VolumePublishInfo,
) (*csi.NodeUnstageVolumeResponse, error) {
	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
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

	// Get the loop device name and see if it is attached or not
	isLoopDeviceAttached, loopDevice, err := utils.GetLoopDeviceAttachedToFile(ctx, loopFile)
	if err != nil {
		return nil, status.Error(codes.Internal,
			fmt.Sprintf("failed to get loop device for loop file '%s': %s", loopFile, err.Error()))
	}

	if isLoopDeviceAttached {
		// Check if it is mounted or not (ideally it should never be mounted)
		isLoopDeviceMounted, err := utils.IsLoopDeviceMounted(ctx, loopDevice.Name)
		if err != nil {
			return nil, status.Error(codes.Internal,
				fmt.Sprintf("unable to identify if loop device '%s' is mounted: %s",
					loopDevice.Name, err.Error()))
		}
		// If not mounted any more proceed to remove the device and/or remove the NFS mount point
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
			// We shall never be in this state, so error out
			Logc(ctx).WithFields(log.Fields{
				"loopFile":   loopFile,
				"loopDevice": loopDevice.Name,
			}).Error("Loop device is still mounted, skipping detach.")
			return nil, status.Error(codes.FailedPrecondition,
				fmt.Sprintf("unable to detach loop device '%s'; it is still mounted", loopDevice.Name))
		}
	}

	// Identify if the NFS mount point has any subvolumes in use by loop devices, if not then it is safe to remove it
	if utils.SafeToRemoveNFSMount(ctx, nfsMountpoint) {
		if err = utils.RemoveMountPointRetry(ctx, nfsMountpoint); err != nil {
			Logc(ctx).Errorf("Failed to remove NFS mountpoint '%s': %s", nfsMountpoint, err.Error())
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	// Delete the device info we saved to the staging path so unstage can succeed
	if err = p.clearStagedDeviceInfo(ctx, stagingTargetPath, volumeId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (p *Plugin) nodePublishNFSBlockVolume(
	ctx context.Context, req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {
	// Ensure AccessMode is RWO or RWOP
	csiAccessMode := p.getAccessForCSIAccessMode(req.GetVolumeCapability().GetAccessMode().Mode)
	if p.containsMultiNodeAccessMode([]tridentconfig.AccessMode{csiAccessMode}) {
		return nil, status.Errorf(codes.InvalidArgument, "volume cannot be mounted using multi-node access mode")
	}

	publishInfo, err := p.readStagedDeviceInfo(ctx, req.StagingTargetPath)
	if err != nil {
		if utils.IsNotFoundError(err) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

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

// writeStagedDeviceInfo writes the volume publish info to the staging target path.
func (p *Plugin) writeStagedDeviceInfo(
	ctx context.Context, stagingTargetPath string, publishInfo *utils.VolumePublishInfo, volumeId string,
) error {
	publishInfoBytes, err := json.Marshal(publishInfo)
	if err != nil {
		return err
	}

	filename := path.Join(stagingTargetPath, volumePublishInfoFilename)

	if err := ioutil.WriteFile(filename, publishInfoBytes, 0o600); err != nil {
		return err
	}

	if err := p.writeStagedTrackingFile(ctx, volumeId, stagingTargetPath); err != nil {
		return status.Error(codes.Internal, err.Error())
	}

	return nil
}

// readStagedDeviceInfo reads the volume publish info from the staging target path.
func (p *Plugin) readStagedDeviceInfo(
	ctx context.Context, stagingTargetPath string,
) (*utils.VolumePublishInfo, error) {
	var publishInfo utils.VolumePublishInfo
	filename := path.Join(stagingTargetPath, volumePublishInfoFilename)

	// File may not exist so caller needs to handle not found error
	if _, err := os.Stat(filename); err != nil {

		Logc(ctx).WithFields(log.Fields{
			"stagingTargetPath": filename,
			"error":             err.Error(),
		}).Error("Unable to find staged device info.")

		if os.IsNotExist(err) {
			return nil, utils.NotFoundError("staged device info file not found")
		} else {
			return nil, err
		}
	}

	publishInfoBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(publishInfoBytes, &publishInfo)
	if err != nil {
		return nil, err
	}

	Logc(ctx).Debug("Publish Info found")
	return &publishInfo, nil
}

// clearStagedDeviceInfo removes the volume info at the staging target path.  This method is idempotent,
// in that if the file doesn't exist, no error is generated.
func (p *Plugin) clearStagedDeviceInfo(ctx context.Context, stagingTargetPath, volumeId string) error {
	fields := log.Fields{"stagingTargetPath": stagingTargetPath, "volumeId": volumeId}
	Logc(ctx).WithFields(fields).Debug(">>>> clearStagedDeviceInfo")
	defer Logc(ctx).WithFields(fields).Debug("<<<< clearStagedDeviceInfo")

	stagingFilename := path.Join(stagingTargetPath, volumePublishInfoFilename)

	if err := os.Remove(stagingFilename); err != nil {

		logFields := log.Fields{"stagingFilename": stagingFilename, "error": err}

		if os.IsNotExist(err) {
			Logc(ctx).WithFields(logFields).Warning("Staging file does not exist.")
			return nil
		} else {
			Logc(ctx).WithFields(logFields).Error("Removing staging file failed.")
			return err
		}
	}

	Logc(ctx).WithField("stagingFilename", stagingFilename).Debug("Removed staging file.")
	return nil
}

// writeStagedTrackingFile writes the serialized staging target path for a volume to Trident's own
// volume tracking location (/var/lib/trident/tracking) as a backup to the staging target path.
func (p *Plugin) writeStagedTrackingFile(ctx context.Context, volumeId, stagingTargetPath string) error {
	volumeTrackingPublishInfo := &utils.VolumeTrackingPublishInfo{
		StagingTargetPath: stagingTargetPath,
	}

	volumeTrackingPublishInfoBytes, err := json.Marshal(volumeTrackingPublishInfo)
	if err != nil {
		return err
	}

	trackingFile := volumeId + ".json"
	trackingFilename := path.Join(tridentDeviceInfoPath, trackingFile)

	if err := ioutil.WriteFile(trackingFilename, volumeTrackingPublishInfoBytes, 0o600); err != nil {
		Logc(ctx).WithFields(log.Fields{
			"volumeId": volumeId,
			"error":    err.Error(),
		}).Error("Unable to write tracking file.")
		return err
	}

	return nil
}

// readStagedTrackingFile returns the staged_target_path given the volumeId for a staged device.
// Devices that were created with 19.07 and prior will not have a tracking file.
func (p *Plugin) readStagedTrackingFile(ctx context.Context, volumeId string) (string, error) {
	var publishInfoLocation utils.VolumeTrackingPublishInfo
	trackingFile := volumeId + ".json"
	trackingFilename := path.Join(tridentDeviceInfoPath, trackingFile)

	// File may not exist so caller needs to handle not found error
	if _, err := os.Stat(trackingFilename); err != nil {

		Logc(ctx).WithFields(log.Fields{
			"volumeId": volumeId,
			"error":    err.Error(),
		}).Error("Unable to find tracking file matching volumeId.")

		if os.IsNotExist(err) {
			return "", utils.NotFoundError("tracking file not found")
		} else {
			return "", err
		}
	}

	publishInfoLocationBytes, err := ioutil.ReadFile(trackingFilename)
	if err != nil {
		return "", err
	}

	err = json.Unmarshal(publishInfoLocationBytes, &publishInfoLocation)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"volumeId": volumeId,
			"error":    err.Error(),
		}).Error("Could not parse publish info location.")
		return "", err
	}

	Logc(ctx).WithField("publishInfoLocation", publishInfoLocation).Debug("Publish info location found.")
	return publishInfoLocation.StagingTargetPath, nil
}

// clearStagedTrackingFile deletes the staging target path info for a volume from Trident's own
// volume tracking location (/var/lib/trident/tracking).  This method is idempotent, in that if
// the file doesn't exist, no error is generated.
func (p *Plugin) clearStagedTrackingFile(ctx context.Context, volumeId string) error {
	fields := log.Fields{"volumeId": volumeId}
	Logc(ctx).WithFields(fields).Debug(">>>> clearStagedTrackingFile")
	defer Logc(ctx).WithFields(fields).Debug("<<<< clearStagedTrackingFile")

	trackingFile := volumeId + ".json"
	trackingFilename := path.Join(tridentDeviceInfoPath, trackingFile)

	if err := os.Remove(trackingFilename); err != nil {

		logFields := log.Fields{"trackingFilename": trackingFilename, "error": err}

		if os.IsNotExist(err) {
			Logc(ctx).WithFields(logFields).Warning("Tracking file does not exist.")
			return nil
		} else {
			Logc(ctx).WithFields(logFields).Error("Removing tracking file failed.")
			return err
		}
	}

	Logc(ctx).WithField("trackingFilename", trackingFilename).Debug("Removed tracking file.")
	return nil
}

// getVolumeProtocolFromPublishInfo examines the publish info read from the staging target path and determines
// the protocol type from the volume (File or Block or Block-on-File).
func (p *Plugin) getVolumeProtocolFromPublishInfo(
	publishInfo *utils.VolumePublishInfo,
) (tridentconfig.Protocol, error) {
	if publishInfo.VolumeAccessInfo.NfsServerIP != "" && publishInfo.VolumeAccessInfo.IscsiTargetIQN == "" {
		if publishInfo.VolumeAccessInfo.SubvolumeName != "" {
			return tridentconfig.BlockOnFile, nil
		}
		return tridentconfig.File, nil
	} else if publishInfo.VolumeAccessInfo.IscsiTargetIQN != "" && publishInfo.VolumeAccessInfo.NfsServerIP == "" {
		return tridentconfig.Block, nil
	} else {
		return "", fmt.Errorf("unable to infer volume protocol")
	}
}

type RequestHandler interface {
	GetVolumeId() string
	GetStagingTargetPath() string
}

// getVolumeIdAndStagingPath is a helper method to reduce code duplication
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
