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
	tridentDeviceInfoPath      = "/var/lib/trident/tracking"
	fsRaw                      = "raw"
	lockID                     = "csi_node_server"
	volumePublishInfoFilename  = "volumePublishInfo.json"
	nodePrepBreadcrumbFilename = "nodePrepInfo.json"
	umountMaxDuration          = 30 * time.Second
)

var (
	topologyLabels = make(map[string]string)
)

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
	default:
		return nil, status.Error(codes.InvalidArgument, "unknown protocol")
	}
}

func (p *Plugin) NodeUnstageVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest,
) (response *csi.NodeUnstageVolumeResponse, responseErr error) {

	lockContext := "NodeUnstageVolume-" + req.GetVolumeId()
	utils.Lock(ctx, lockContext, lockID)
	defer utils.Unlock(ctx, lockContext, lockID)

	fields := log.Fields{"Method": "NodeUnstageVolume", "Type": "CSI_Node"}
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
		return p.nodeUnstageNFSVolume(ctx, req)
	case tridentconfig.Block:
		return p.nodeUnstageISCSIVolume(ctx, req, publishInfo)
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
		notMountPoint, err = utils.IsLikelyNotMountPoint(targetPath)
	} else {
		var mounted bool
		mounted, err = utils.IsMounted(ctx, "", targetPath)
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
		if err = utils.WaitForUmount(ctx, targetPath, umountMaxDuration); err != nil {
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

		isRawBlock = publishInfo.FilesystemType == fsRaw
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

	lunID := int(publishInfo.IscsiLunNumber)

	Logc(ctx).WithFields(log.Fields{
		"targetIQN":      publishInfo.IscsiTargetIQN,
		"lunID":          lunID,
		"devicePath":     publishInfo.DevicePath,
		"mountOptions":   publishInfo.MountOptions,
		"filesystemType": publishInfo.FilesystemType,
	}).Debug("PublishInfo for device to expand.")

	// Make sure device is ready
	if utils.IsAlreadyAttached(ctx, lunID, publishInfo.IscsiTargetIQN) {

		// Rescan device to detect increased size
		if err = utils.ISCSIRescanDevices(
			ctx, publishInfo.IscsiTargetIQN, publishInfo.IscsiLunNumber, requiredBytes); err != nil {
			Logc(ctx).WithFields(log.Fields{
				"device": publishInfo.DevicePath,
				"error":  err,
			}).Error("Unable to scan device.")
			return nil, status.Error(codes.Internal, err.Error())
		}

		// Expand filesystem
		if publishInfo.FilesystemType != fsRaw {
			filesystemSize, err := utils.ExpandISCSIFilesystem(ctx, publishInfo, stagingTargetPath)
			if err != nil {
				Logc(ctx).WithFields(log.Fields{
					"device":         publishInfo.DevicePath,
					"filesystemType": publishInfo.FilesystemType,
					"error":          err,
				}).Error("Unable to expand filesystem.")
				return nil, status.Error(codes.Internal, err.Error())
			}
			Logc(ctx).WithFields(log.Fields{
				"filesystemSize": filesystemSize,
				"requiredBytes":  requiredBytes,
				"limitBytes":     limitBytes,
			}).Debug("Filesystem size after expand.")
		}
	} else {
		Logc(ctx).WithField("devicePath", publishInfo.DevicePath).Error(
			"Unable to expand volume as device is not attached.")
		err = fmt.Errorf("device %s to expand is not attached", publishInfo.DevicePath)
		return nil, status.Error(codes.Internal, err.Error())
	}

	Logc(ctx).WithFields(log.Fields{
		"volumePath": volumePath,
		"volumeId":   volumeId,
	}).Debug("Filesystem expansion completed.")
	return &csi.NodeExpandVolumeResponse{}, nil
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

	// Only get the host system info if node prep is enabled and we don't have the info yet.
	if p.nodePrep.Enabled && p.hostInfo == nil {
		// Discover host OS info
		host, err := utils.GetHostSystemInfo(ctx)
		if err != nil {
			p.hostInfo = nil
			if p.nodePrep.NFS != utils.PrepCompleted {
				p.nodePrep.NFS = utils.PrepFailed
				p.nodePrep.NFSStatusMessage = "Unable to get host system information."
			}
			Logc(ctx).WithError(err).Warn("Unable to get host system information; worker prep will be skipped.")
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

	node := &utils.Node{
		Name:     p.nodeName,
		IQN:      iscsiWWN,
		IPs:      ips,
		NodePrep: p.nodePrep,
		HostInfo: p.hostInfo,
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

	if p.nodePrep.Enabled {
		p.nodePrepForNFS(ctx)
	}

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

func (p *Plugin) nodePrepForNFS(ctx context.Context) {

	switch p.nodePrep.NFS {
	case utils.PrepCompleted:
		Logc(ctx).Debug("Node preparation for NFS has already completed; continuing.")
		return
	case utils.PrepFailed:
		Logc(ctx).WithField("FailureReason", p.nodePrep.NFSStatusMessage).Warn(
			"Node preparation for NFS failed; NFS mounts to this node may fail.")
		return
	case utils.PrepRunning:
		Logc(ctx).Warn("Node preparation for NFS is incomplete; NFS mounts to this node may fail.")
		return
	case utils.PrepOutdated:
		Logc(ctx).Debug("Previously completed node preparation was from a different version of Trident; will rerun.")
		fallthrough
	case utils.PrepPending:
		Logc(ctx).Debug("Preparing node for NFS...")
	default:
		Logc(ctx).WithField("nfsStatus", p.nodePrep.NFS).Error(
			"Node preparation for NFS is in an unknown state; NFS mounts to this node may fail")
		return
	}

	// Set NFS prep to running
	t := time.Now()
	p.nodePrep.NFS = utils.PrepRunning
	p.nodePrep.NFSStatusMessage = fmt.Sprintf("Started at %s", t.Format("2006-01-02 15:04:05"))

	// Update the controller with the new status
	p.nodeRegisterWithController(ctx, 30*time.Second)
	defer p.nodeRegisterWithController(ctx, 30*time.Second)

	// Install required packages
	err := utils.PrepareNFSPackagesOnHost(ctx, *p.hostInfo)
	if err != nil {
		err = fmt.Errorf("error preparing NFS packages on the host; %+v", err)
		Logc(ctx).Warn(err)
		p.nodePrep.NFS = utils.PrepFailed
		p.nodePrep.NFSStatusMessage = fmt.Sprint(err)
		return
	}

	// Enable required services
	err = utils.PrepareNFSServicesOnHost(ctx)
	if err != nil {
		err = fmt.Errorf("error preparing NFS services on the host; %+v", err)
		Logc(ctx).Warn(err)
		p.nodePrep.NFS = utils.PrepFailed
		p.nodePrep.NFSStatusMessage = fmt.Sprint(err)
		return
	}

	// Set NFS prep to completed
	t = time.Now()
	p.nodePrep.NFS = utils.PrepCompleted
	p.nodePrep.NFSStatusMessage = fmt.Sprintf("Completed at %s", t.Format("2006-01-02 15:04:05"))
	Logc(ctx).Info("Successfully completed Worker Node Prep for NFS.")

	// Write the breadcrumb file to persist completed status beyond restarts
	err = p.writeNodePrepBreadcrumbFile(ctx)
	if err != nil {
		Logc(ctx).WithError(err).Warn(
			"Could not write node prep status file; node prep may run again if node restarts.")
	}
}

func (p *Plugin) nodePrepForISCSI(ctx context.Context) {

	outdated := false

	switch p.nodePrep.ISCSI {
	case utils.PrepCompleted:
		Logc(ctx).Debug("Node preparation for iSCSI has already completed; continuing.")
		return
	case utils.PrepFailed:
		Logc(ctx).WithField("FailureReason", p.nodePrep.ISCSIStatusMessage).Warn(
			"Node preparation for iSCSI failed; iSCSI mounts to this node may fail.")
		return
	case utils.PrepRunning:
		Logc(ctx).Warn("Node preparation for iSCSI is incomplete; iSCSI mounts to this node may fail.")
		return
	case utils.PrepPreConfigured:
		// We do not record a preconfigured status in the breadcrumb file.
		// This is so that it will retry on pod restarts as the user may not have been aware that iscsi was partially
		// implemented on their nodes already.
		Logc(ctx).Warn("iSCSI was preconfigured; iSCSI mounts to this node may fail.")
		return
	case utils.PrepOutdated:
		Logc(ctx).Debug("Previously completed node preparation was from a different version of Trident; will rerun.")
		outdated = true
		fallthrough
	case utils.PrepPending:
		Logc(ctx).Debug("Preparing node for iSCSI...")
	default:
		Logc(ctx).WithField("iscsiStatus", p.nodePrep.ISCSI).Warn(
			"Node preparation for iSCSI is in an unknown state; iSCSI mounts to this node may fail")
		return
	}

	// Set iSCSI prep to running
	t := time.Now()
	p.nodePrep.ISCSI = utils.PrepRunning
	p.nodePrep.ISCSIStatusMessage = fmt.Sprintf("Started at %s", t.Format("2006-01-02 15:04:05"))

	// Update the controller with the new status
	p.nodeRegisterWithController(ctx, 30*time.Second)
	defer p.nodeRegisterWithController(ctx, 30*time.Second)

	var iscsiPreconfigured bool
	var err error

	// If this is an outdated Trident prep, then iscsi is expected to be preconfigured, so skip this check.
	if !outdated {
		// Check if iSCSI is currently running as that indicates it may be in use and we should not adjust iSCSI or
		// multipath settings/packages.
		iscsiPreconfigured, err = utils.ISCSIActiveOnHost(ctx, *p.hostInfo)
		if err != nil {
			err = fmt.Errorf("error checking status of iscsi daemon on host; %+v", err)
			Logc(ctx).Warn(err)
			p.nodePrep.ISCSI = utils.PrepFailed
			p.nodePrep.ISCSIStatusMessage = fmt.Sprint(err)
			return
		}
	}

	// Install required packages, skipping iscsi/multipath if iscsi already is running
	err = utils.PrepareISCSIPackagesOnHost(ctx, *p.hostInfo, iscsiPreconfigured)
	if err != nil {
		err = fmt.Errorf("error preparing iSCSI packages on the host; %+v", err)
		Logc(ctx).Warn(err)
		p.nodePrep.ISCSI = utils.PrepFailed
		p.nodePrep.ISCSIStatusMessage = fmt.Sprint(err)
		return
	}

	if iscsiPreconfigured {
		Logc(ctx).Warn("iSCSI service is already running; skipping iSCSI and multipath configuration; iSCSI " +
			"mounts to this node may fail.")
		p.nodePrep.ISCSI = utils.PrepPreConfigured
		p.nodePrep.ISCSIStatusMessage = "iSCSI service was already running on the host; " +
			"iSCSI and multipath configuration skipped; iSCSI mounts to this node may fail."
		return
	}

	// Enable required services
	err = utils.PrepareISCSIServicesOnHost(ctx, *p.hostInfo)
	if err != nil {
		err = fmt.Errorf("error preparing iSCSI services on the host; %+v", err)
		Logc(ctx).Warn(err)
		p.nodePrep.ISCSI = utils.PrepFailed
		p.nodePrep.ISCSIStatusMessage = fmt.Sprint(err)
		return
	}

	// Set iSCSI prep to completed
	t = time.Now()
	p.nodePrep.ISCSI = utils.PrepCompleted
	p.nodePrep.ISCSIStatusMessage = fmt.Sprintf("Completed at %s", t.Format("2006-01-02 15:04:05"))
	Logc(ctx).Info("Successfully completed node prep for iSCSI.")

	// Write the breadcrumb file to persist completed status beyond restarts
	err = p.writeNodePrepBreadcrumbFile(ctx)
	if err != nil {
		Logc(ctx).WithError(err).Warn(
			"Could not write node prep status file; node prep may run again if node restarts.")
	}
}

func (p *Plugin) writeNodePrepBreadcrumbFile(ctx context.Context) error {

	// We should only write statuses that have succeeded
	breadcrumbs := utils.NodePrepBreadcrumb{TridentVersion: tridentconfig.OrchestratorVersion.String()}
	if p.nodePrep.NFS == utils.PrepCompleted {
		breadcrumbs.NFS = p.nodePrep.NFSStatusMessage
	}
	if p.nodePrep.ISCSI == utils.PrepCompleted {
		breadcrumbs.ISCSI = p.nodePrep.ISCSIStatusMessage
	}

	// Convert struct to json
	breadcrumbBytes, err := json.Marshal(breadcrumbs)
	if err != nil {
		return err
	}

	// Write the file; piggyback on deviceInfoPath as that is already being created on the host
	breadcrumbFile := path.Join(tridentDeviceInfoPath, nodePrepBreadcrumbFilename)
	err = ioutil.WriteFile(breadcrumbFile, breadcrumbBytes, 0600)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"file":  breadcrumbFile,
			"error": err,
		}).Error("Could not write file.")
		return err
	}

	Logc(ctx).Debug("Node prep status file written.")
	return nil
}

// readNodePrepBreadcrumbFile returns the contents of the node prep status file, if it exists.
// Returns a status.Error if an error is returned.
func (p *Plugin) readNodePrepBreadcrumbFile(ctx context.Context) (*utils.NodePrepBreadcrumb, error) {

	// File may not exist so caller needs to handle not found error
	breadcrumbFile := path.Join(tridentDeviceInfoPath, nodePrepBreadcrumbFilename)
	if _, err := os.Stat(breadcrumbFile); err != nil {
		Logc(ctx).WithFields(log.Fields{
			"file":  breadcrumbFile,
			"error": err.Error(),
		}).Debug("Unable to find node prep status file.")
		return nil, utils.NotFoundError("node prep status file not found")
	}

	breadcrumb := utils.NodePrepBreadcrumb{}

	breadcrumbBytes, err := ioutil.ReadFile(breadcrumbFile)
	if err != nil {
		return nil, fmt.Errorf(err.Error())
	}

	err = json.Unmarshal(breadcrumbBytes, &breadcrumb)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"file":  breadcrumb,
			"raw":   string(breadcrumbBytes),
			"error": err.Error(),
		}).Error("Could not parse node prep status.")
		return nil, err
	}

	Logc(ctx).WithField("nodePrepStatus", breadcrumb).Debug("Node prep status found.")
	return &breadcrumb, nil
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
	notMnt, err := utils.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(targetPath, 0750); err != nil {
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
		mountOptions := strings.Split(publishInfo.MountOptions, ",")
		mountOptions = append(mountOptions, "ro")
		publishInfo.MountOptions = strings.Join(mountOptions, ",")
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

	if p.nodePrep.Enabled {
		p.nodePrepForISCSI(ctx)
	}

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

	if fstype == fsRaw && mountCapability != nil {
		return nil, status.Error(codes.InvalidArgument, "mount capability requested with raw blocks")
	} else if fstype != fsRaw && blockCapability != nil {
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

	lunID, err := strconv.Atoi(req.PublishContext["iscsiLunNumber"])
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	publishInfo := &utils.VolumePublishInfo{
		Localhost:      true,
		FilesystemType: fstype,
		SharedTarget:   sharedTarget,
	}
	publishInfo.UseCHAP = useCHAP

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
				return nil, status.Error(codes.Internal, err.Error())
			}
		} else if encryptedCHAP {
			// Only error if the key is not set and the publishContext includes encrypted fields
			msg := "encryption key not set; cannot decrypt CHAP credentials"
			Logc(ctx).Error(msg)
			return nil, status.Error(codes.Internal, msg)
		}
	}

	// Perform the login/rescan/discovery/(optionally)format, mount & get the device back in the publish info
	if err := utils.AttachISCSIVolume(ctx, req.VolumeContext["internalName"], "", publishInfo); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
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

func (p *Plugin) nodeUnstageISCSIVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest, publishInfo *utils.VolumePublishInfo,
) (*csi.NodeUnstageVolumeResponse, error) {

	// Delete the device from the host
	err := utils.PrepareDeviceForRemoval(ctx, int(publishInfo.IscsiLunNumber), publishInfo.IscsiTargetIQN,
		p.unsafeDetach)
	if nil != err && !p.unsafeDetach {
		return nil, status.Error(codes.Internal, err.Error())
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
		return nil, err
	}

	// Delete the device info we saved to the staging path so unstage can succeed
	if err := p.clearStagedDeviceInfo(ctx, stagingTargetPath, volumeId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Ensure that the temporary mount point created during a filesystem expand operation is removed.
	if err := utils.UmountAndRemoveTemporaryMountPoint(ctx, stagingTargetPath); err != nil {
		Logc(ctx).WithField("stagingTargetPath", stagingTargetPath).Errorf(
			"Failed to remove directory in staging target path; %s", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to remove temporary directory "+
			"in staging target path %s; %s", stagingTargetPath, err))
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
		mountOptions := strings.Split(publishInfo.MountOptions, ",")
		mountOptions = append(mountOptions, "ro")
		publishInfo.MountOptions = strings.Join(mountOptions, ",")
	}

	isRawBlock := publishInfo.FilesystemType == fsRaw
	if isRawBlock {

		if len(publishInfo.MountOptions) > 0 {
			mountOptions := strings.Split(publishInfo.MountOptions, ",")
			mountOptions = append(mountOptions, "bind")
			publishInfo.MountOptions = strings.Join(mountOptions, ",")
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

// writeStagedDeviceInfo writes the volume publish info to the staging target path.
func (p *Plugin) writeStagedDeviceInfo(
	ctx context.Context, stagingTargetPath string, publishInfo *utils.VolumePublishInfo, volumeId string,
) error {

	publishInfoBytes, err := json.Marshal(publishInfo)
	if err != nil {
		return err
	}

	filename := path.Join(stagingTargetPath, volumePublishInfoFilename)

	if err := ioutil.WriteFile(filename, publishInfoBytes, 0600); err != nil {
		return err
	}

	if err := p.writeStagedTrackingFile(ctx, volumeId, stagingTargetPath); err != nil {
		return status.Error(codes.Internal, err.Error())
	}

	return nil
}

// writeStagedDeviceInfo reads the volume publish info from the staging target path.
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
func (p *Plugin) clearStagedDeviceInfo(ctx context.Context, stagingTargetPath string, volumeId string) error {

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
func (p *Plugin) writeStagedTrackingFile(ctx context.Context, volumeId string, stagingTargetPath string) error {

	volumeTrackingPublishInfo := &utils.VolumeTrackingPublishInfo{
		StagingTargetPath: stagingTargetPath,
	}

	volumeTrackingPublishInfoBytes, err := json.Marshal(volumeTrackingPublishInfo)
	if err != nil {
		return err
	}

	trackingFile := volumeId + ".json"
	trackingFilename := path.Join(tridentDeviceInfoPath, trackingFile)

	if err := ioutil.WriteFile(trackingFilename, volumeTrackingPublishInfoBytes, 0600); err != nil {
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
// the protocol type from the volume (File or Block).
func (p *Plugin) getVolumeProtocolFromPublishInfo(
	publishInfo *utils.VolumePublishInfo,
) (tridentconfig.Protocol, error) {

	if publishInfo.VolumeAccessInfo.NfsServerIP != "" && publishInfo.VolumeAccessInfo.IscsiTargetIQN == "" {
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
