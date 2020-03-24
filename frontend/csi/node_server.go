// Copyright 2020 NetApp, Inc. All Rights Reserved.

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
	"github.com/netapp/trident/utils"
)

const (
	tridentDeviceInfoPath     = "/var/lib/trident/tracking"
	fsRaw                     = "raw"
	lockID                    = "csi_node_server"
	volumePublishInfoFilename = "volumePublishInfo.json"
)

func (p *Plugin) NodeStageVolume(
	ctx context.Context, req *csi.NodeStageVolumeRequest,
) (*csi.NodeStageVolumeResponse, error) {

	lockContext := "NodeStageVolume-" + req.GetVolumeId()
	utils.Lock(lockContext, lockID)
	defer utils.Unlock(lockContext, lockID)

	fields := log.Fields{"Method": "NodeStageVolume", "Type": "CSI_Node"}
	log.WithFields(fields).Debug(">>>> NodeStageVolume")
	defer log.WithFields(fields).Debug("<<<< NodeStageVolume")

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
) (*csi.NodeUnstageVolumeResponse, error) {

	lockContext := "NodeUnstageVolume-" + req.GetVolumeId()
	utils.Lock(lockContext, lockID)
	defer utils.Unlock(lockContext, lockID)

	fields := log.Fields{"Method": "NodeUnstageVolume", "Type": "CSI_Node"}
	log.WithFields(fields).Debug(">>>> NodeUnstageVolume")
	defer log.WithFields(fields).Debug("<<<< NodeUnstageVolume")

	_, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return nil, err
	}

	// Read the device info from the staging path
	publishInfo, err := p.readStagedDeviceInfo(stagingTargetPath)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "unable to read the staging target %s; %s", stagingTargetPath, err)
	}
	protocol, err := p.getVolumeProtocolFromPublishInfo(publishInfo)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "unable to read protocol info from publish info; %s", err)
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
	utils.Lock(lockContext, lockID)
	defer utils.Unlock(lockContext, lockID)

	fields := log.Fields{"Method": "NodePublishVolume", "Type": "CSI_Node"}
	log.WithFields(fields).Debug(">>>> NodePublishVolume")
	defer log.WithFields(fields).Debug("<<<< NodePublishVolume")

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
	utils.Lock(lockContext, lockID)
	defer utils.Unlock(lockContext, lockID)

	fields := log.Fields{"Method": "NodeUnpublishVolume", "Type": "CSI_Node"}
	log.WithFields(fields).Debug(">>>> NodeUnpublishVolume")
	defer log.WithFields(fields).Debug("<<<< NodeUnpublishVolume")

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
			return nil, status.Errorf(codes.NotFound,
				"target path (%s) not found; could not check if the target path is a directory.", targetPath)
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
		mounted, err = utils.IsMounted("", targetPath)
		notMountPoint = !mounted
	}

	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, "target path not found")
		} else {
			return nil, status.Errorf(codes.Internal, "unable to check if targetPath (%s) is mounted or not; %s",
				targetPath, err)
		}
	}

	if notMountPoint {
		return nil, status.Error(codes.NotFound, "volume not mounted")
	}

	if err = utils.Umount(targetPath); err != nil {
		log.WithFields(log.Fields{"path": targetPath, "error": err}).Error("unable to unmount volume.")
		return nil, status.Errorf(codes.InvalidArgument, "unable to unmount volume; %s", err)
	}

	// As per the CSI spec SP i.e. Trident is responsible for deleting the target path,
	// however today Kubernetes performs this deletion. Here we are making best efforts
	// to delete the resource at target path. Sometimes this fails resulting CSI calling
	// NodeUnpublishVolume again and usually deletion goes through in the second attempt.
	// As a viable solution making it run as a goroutine
	go func() {
		if err = utils.DeleteResourceAtPath(targetPath); err != nil {
			log.Debugf("Unable to delete resource at target path: %s; %s", targetPath, err)
		}
	}()

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (p *Plugin) NodeGetVolumeStats(
	ctx context.Context, req *csi.NodeGetVolumeStatsRequest,
) (*csi.NodeGetVolumeStatsResponse, error) {

	// Trident doesn't support GET_VOLUME_STATS capability
	return nil, status.Error(codes.Unimplemented, "")
}

// The CO only calls NodeExpandVolume for the Block protocol as the filesystem has to be mounted to perform
// the resize. This is enforced in our ControllerExpandVolume method where we return true for nodeExpansionRequired
// when the protocol is Block and return false when the protocol is file.
func (p *Plugin) NodeExpandVolume(
	ctx context.Context, req *csi.NodeExpandVolumeRequest,
) (*csi.NodeExpandVolumeResponse, error) {

	fields := log.Fields{"Method": "NodeExpandVolume", "Type": "CSI_Node"}
	log.WithFields(fields).Debug(">>>> NodeExpandVolume")
	defer log.WithFields(fields).Debug("<<<< NodeExpandVolume")

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

	log.WithFields(log.Fields{
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
	stagingTargetPath, err := p.readStagedTrackingFile(volumeId)
	if err != nil {
		if utils.IsNotFoundError(err) {
			// Verify the volumePath is the stagingTargetPath
			filePath := path.Join(volumePath, volumePublishInfoFilename)
			if _, err = os.Stat(filePath); !os.IsNotExist(err) {
				stagingTargetPath = volumePath
			} else {
				log.WithField("filePath", filePath).Errorf("Unable to find volumePublishInfo.")
				return nil, status.Errorf(codes.Internal, "unable to find volume publish info needed for resize")
			}
		} else {
			return nil, status.Errorf(codes.Internal, err.Error())
		}
	}

	// Current K8S behavior is to send the volumePath as the stagingTargetPath. Log what is received if the
	// two variables don't match.
	if stagingTargetPath != volumePath {
		log.WithFields(log.Fields{
			"stagingTargetPath": stagingTargetPath,
			"volumePath":        volumePath,
			"volumeId":          volumeId,
		}).Info("Received something other than the expected stagingTargetPath.")
	}

	publishInfo, err := p.readStagedDeviceInfo(stagingTargetPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	lunID := int(publishInfo.IscsiLunNumber)

	log.WithFields(log.Fields{
		"targetIQN":      publishInfo.IscsiTargetIQN,
		"lunID":          lunID,
		"devicePath":     publishInfo.DevicePath,
		"mountOptions":   publishInfo.MountOptions,
		"filesystemType": publishInfo.FilesystemType,
	}).Debug("PublishInfo for device to expand.")

	// Make sure device is ready
	if utils.IsAlreadyAttached(lunID, publishInfo.IscsiTargetIQN) {

		// Rescan device to detect increased size
		if err = utils.ISCSIRescanDevices(publishInfo.IscsiTargetIQN, publishInfo.IscsiLunNumber, requiredBytes); err != nil {
			log.WithFields(log.Fields{
				"device": publishInfo.DevicePath,
				"error":  err,
			}).Error("Unable to scan device.")
			return nil, status.Error(codes.Internal, err.Error())
		}

		// Expand filesystem
		if publishInfo.FilesystemType != fsRaw {
			filesystemSize, err := utils.ExpandISCSIFilesystem(publishInfo, stagingTargetPath)
			if err != nil {
				log.WithFields(log.Fields{
					"device":         publishInfo.DevicePath,
					"filesystemType": publishInfo.FilesystemType,
					"error":          err,
				}).Error("Unable to expand filesystem.")
				return nil, status.Error(codes.Internal, err.Error())
			}
			log.WithFields(log.Fields{
				"filesystemSize": filesystemSize,
				"requiredBytes":  requiredBytes,
				"limitBytes":     limitBytes,
			}).Debug("Filesystem size after expand.")
		}
	} else {
		log.WithField("devicePath", publishInfo.DevicePath).Error("Unable to expand volume as device is not attached.")
		err = fmt.Errorf("device %s to expand is not attached", publishInfo.DevicePath)
		return nil, status.Error(codes.Internal, err.Error())
	}

	log.WithFields(log.Fields{
		"volumePath": volumePath,
		"volumeId":   volumeId,
	}).Debug("Filesystem expansion completed.")
	return &csi.NodeExpandVolumeResponse{}, nil
}

func (p *Plugin) NodeGetCapabilities(
	ctx context.Context, req *csi.NodeGetCapabilitiesRequest,
) (*csi.NodeGetCapabilitiesResponse, error) {

	fields := log.Fields{"Method": "NodeGetCapabilities", "Type": "CSI_Node"}
	log.WithFields(fields).Debug(">>>> NodeGetCapabilities")
	defer log.WithFields(fields).Debug("<<<< NodeGetCapabilities")

	return &csi.NodeGetCapabilitiesResponse{Capabilities: p.nsCap}, nil
}

func (p *Plugin) NodeGetInfo(
	ctx context.Context, req *csi.NodeGetInfoRequest,
) (*csi.NodeGetInfoResponse, error) {

	fields := log.Fields{"Method": "NodeGetInfo", "Type": "CSI_Node"}
	log.WithFields(fields).Debug(">>>> NodeGetInfo")
	defer log.WithFields(fields).Debug("<<<< NodeGetInfo")

	return &csi.NodeGetInfoResponse{NodeId: p.nodeName}, nil
}

func (p *Plugin) nodeGetInfo() *utils.Node {
	iscsiWWN := ""
	iscsiWWNs, err := utils.GetInitiatorIqns()
	if err != nil {
		log.WithField("error", err).Warn("Problem getting iSCSI initiator name.")
	} else if iscsiWWNs == nil || len(iscsiWWNs) == 0 {
		log.Warn("Could not find iSCSI initiator name.")
	} else {
		iscsiWWN = iscsiWWNs[0]
	}

	ips, err := utils.GetIPAddresses()
	if err != nil {
		log.WithField("error", err).Error("Could not get IP addresses.")
	} else if ips == nil || len(ips) == 0 {
		log.Warn("Could not find any usable IP addresses.")
	} else {
		log.WithField("IP Addresses", ips).Info("Discovered IP addresses.")
	}

	node := &utils.Node{
		Name: p.nodeName,
		IQN:  iscsiWWN,
		IPs:  ips,
	}
	return node
}

func (p *Plugin) nodeRegisterWithController() {

	// Assemble the node details that we will register with the controller
	node := p.nodeGetInfo()

	// The controller may not be fully initialized by the time the node is ready to register,
	// so retry until it is responding on the back channel and we have registered the node.
	registerNode := func() error {
		return p.restClient.CreateNode(node)
	}

	registerNodeNotify := func(err error, duration time.Duration) {
		log.WithFields(log.Fields{
			"increment": duration,
			"error":     err,
		}).Debug("Controller not yet ready, waiting.")
	}

	registerNodeBackoff := backoff.NewExponentialBackOff()
	registerNodeBackoff.InitialInterval = 1 * time.Second
	registerNodeBackoff.Multiplier = 2
	registerNodeBackoff.MaxInterval = 5 * time.Second
	registerNodeBackoff.RandomizationFactor = 0.1
	registerNodeBackoff.MaxElapsedTime = 0 // Indefinite

	// Retry using an exponential backoff that never ends until it succeeds
	backoff.RetryNotify(registerNode, registerNodeBackoff, registerNodeNotify)

	log.WithField("node", p.nodeName).Debug("Communication with controller established, node registered.")
}

func (p *Plugin) nodeDeregisterWithController() error {
	return p.restClient.DeleteNode(p.nodeName)
}

func (p *Plugin) nodeStageNFSVolume(ctx context.Context, req *csi.NodeStageVolumeRequest,
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
	if err := p.writeStagedDeviceInfo(stagingTargetPath, publishInfo, volumeId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (p *Plugin) nodeUnstageNFSVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return nil, err
	}

	// Delete the device info we saved to the staging path so unstage can succeed
	if err := p.clearStagedDeviceInfo(stagingTargetPath, volumeId); err != nil {
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

	publishInfo, err := p.readStagedDeviceInfo(req.StagingTargetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if req.GetReadonly() {
		mountOptions := strings.Split(publishInfo.MountOptions, ",")
		mountOptions = append(mountOptions, "ro")
		publishInfo.MountOptions = strings.Join(mountOptions, ",")
	}

	err = utils.AttachNFSVolume(req.VolumeContext["internalName"], req.TargetPath, publishInfo)
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
		UseCHAP:        useCHAP,
		SharedTarget:   sharedTarget,
	}

	err = unstashIscsiTargetPortals(publishInfo, req.PublishContext)
	if nil != err {
		return nil, status.Error(codes.Internal, err.Error())
	}
	publishInfo.MountOptions = req.PublishContext["mountOptions"]
	publishInfo.IscsiTargetIQN = req.PublishContext["iscsiTargetIqn"]
	publishInfo.IscsiLunNumber = int32(lunID)
	publishInfo.IscsiInterface = req.PublishContext["iscsiInterface"]
	publishInfo.IscsiIgroup = req.PublishContext["iscsiIgroup"]
	publishInfo.IscsiUsername = req.PublishContext["iscsiUsername"]
	publishInfo.IscsiInitiatorSecret = req.PublishContext["iscsiInitiatorSecret"]
	publishInfo.IscsiTargetSecret = req.PublishContext["iscsiTargetSecret"]

	// Perform the login/rescan/discovery/(optionally)format, mount & get the device back in the publish info
	if err := utils.AttachISCSIVolume(req.VolumeContext["internalName"], "", publishInfo); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return nil, err
	}

	// Save the device info to the staging path for use in the publish & unstage calls
	if err := p.writeStagedDeviceInfo(stagingTargetPath, publishInfo, volumeId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (p *Plugin) nodeUnstageISCSIVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest, publishInfo *utils.VolumePublishInfo,
) (*csi.NodeUnstageVolumeResponse, error) {

	// Delete the device from the host
	utils.PrepareDeviceForRemoval(int(publishInfo.IscsiLunNumber), publishInfo.IscsiTargetIQN)

	// Get map of hosts and sessions for given Target IQN
	hostSessionMap := utils.GetISCSIHostSessionMapForTarget(publishInfo.IscsiTargetIQN)
	if len(hostSessionMap) == 0 {
		log.Warnf("no iSCSI hosts found for target %s", publishInfo.IscsiTargetIQN)
	}

	// Logout of the iSCSI session if appropriate for each applicable host
	for hostNumber, sessionNumber := range hostSessionMap {
		logout := false
		if !publishInfo.SharedTarget {
			// Always log out of a non-shared target
			logout = true
		} else {
			// Log out of a shared target if no mounts to that target remain
			anyMounts, err := utils.ISCSITargetHasMountedDevice(publishInfo.IscsiTargetIQN)
			logout = (err == nil) && !anyMounts && utils.SafeToLogOut(hostNumber, sessionNumber)
		}

		if logout {
			log.Debug("Safe to log out")
			utils.ISCSIDisableDelete(publishInfo.IscsiTargetIQN, publishInfo.IscsiTargetPortal)
			for _, portal := range publishInfo.IscsiPortals {
				utils.ISCSIDisableDelete(publishInfo.IscsiTargetIQN, portal)
			}
		}
	}

	volumeId, stagingTargetPath, err := p.getVolumeIdAndStagingPath(req)
	if err != nil {
		return nil, err
	}

	// Delete the device info we saved to the staging path so unstage can succeed
	if err := p.clearStagedDeviceInfo(stagingTargetPath, volumeId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Ensure that the temporary mount point created during a filesystem expand operation is removed.
	if err := utils.UmountAndRemoveTemporaryMountPoint(stagingTargetPath); err != nil {
		log.WithField("stagingTargetPath", stagingTargetPath).Errorf(
			"Failed to remove directory in staging target path; %s", err)
		return nil, fmt.Errorf("failed to remove temporary directory in staging target path %s; %s",
			stagingTargetPath, err)
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (p *Plugin) nodePublishISCSIVolume(
	ctx context.Context, req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {

	var err error

	// Read the device info from the staging path
	publishInfo, err := p.readStagedDeviceInfo(req.StagingTargetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
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
		err = utils.MountDevice(publishInfo.DevicePath, req.TargetPath, publishInfo.MountOptions, true)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "unable to bind mount raw device; %s", err)
		}
	} else {
		// Mount the device
		err = utils.MountDevice(publishInfo.DevicePath, req.TargetPath, publishInfo.MountOptions, false)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "unable to mount device; %s", err)
		}
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (p *Plugin) writeStagedDeviceInfo(
	stagingTargetPath string, publishInfo *utils.VolumePublishInfo, volumeId string,
) error {

	publishInfoBytes, err := json.Marshal(publishInfo)
	if err != nil {
		return err
	}

	filename := path.Join(stagingTargetPath, volumePublishInfoFilename)

	if err := ioutil.WriteFile(filename, publishInfoBytes, 0600); err != nil {
		return err
	}

	if err := p.writeStagedTrackingFile(volumeId, stagingTargetPath); err != nil {
		return status.Error(codes.Internal, err.Error())
	}

	return nil
}

func (p *Plugin) readStagedDeviceInfo(stagingTargetPath string) (*utils.VolumePublishInfo, error) {

	var publishInfo utils.VolumePublishInfo
	filename := path.Join(stagingTargetPath, volumePublishInfoFilename)

	publishInfoBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(publishInfoBytes, &publishInfo)
	if err != nil {
		return nil, err
	}

	log.Debug("Publish Info found")
	return &publishInfo, nil
}

func (p *Plugin) clearStagedDeviceInfo(stagingTargetPath string, volumeId string) error {
	fields := log.Fields{"stagingTargetPath": stagingTargetPath, "volumeId": volumeId}
	log.WithFields(fields).Debug(">>>> clearStagedDeviceInfo")
	defer log.WithFields(fields).Debug("<<<< clearStagedDeviceInfo")

	filename := path.Join(stagingTargetPath, volumePublishInfoFilename)

	if _, err := os.Stat(filename); err == nil {
		if err = os.Remove(filename); err != nil {
			log.WithField("filename", filename).Errorf("Failed to remove staging target path; %s", err)
			return fmt.Errorf("failed to remove staging target path %s; %s", filename, err)
		}
	} else if !os.IsNotExist(err) {
		log.WithFields(fields).Errorf("Can't determine if staging target path exists; %s", err)
		return fmt.Errorf("can't determine if staging target path %s exists; %s", filename, err)
	}

	if err := p.clearStagedTrackingFile(volumeId); err != nil {
		log.WithField("volumeId", volumeId).Errorf("Failed to remove tracking file: %s", err)
	}

	return nil
}

// writeStagedTrackingFile writes the serialized staged_target_path for a volumeId.
func (p *Plugin) writeStagedTrackingFile(volumeId string, stagingTargetPath string) error {

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
		log.WithFields(log.Fields{
			"volumeId": volumeId,
			"error":    err.Error(),
		}).Error("Unable to write tracking file.")
		return err
	}

	return nil
}

// readStagedTrackingFile returns the staged_target_path given the volumeId for a staged device.
// Devices that were created with 19.07 and prior will not have a tracking file.
// Returns a status.Error if an error is returned.
func (p *Plugin) readStagedTrackingFile(volumeId string) (string, error) {

	var publishInfoLocation utils.VolumeTrackingPublishInfo
	trackingFile := volumeId + ".json"
	trackingFilename := path.Join(tridentDeviceInfoPath, trackingFile)

	// File may not exist so caller needs to handle not found error
	if _, err := os.Stat(trackingFilename); err != nil {
		log.WithFields(log.Fields{
			"volumeId": volumeId,
			"error":    err.Error(),
		}).Error("Unable to find tracking file matching volumeId.")
		return "", utils.NotFoundError("tracking file not found")
	}

	publishInfoLocationBytes, err := ioutil.ReadFile(trackingFilename)
	if err != nil {
		return "", fmt.Errorf(err.Error())
	}

	err = json.Unmarshal(publishInfoLocationBytes, &publishInfoLocation)
	if err != nil {
		log.WithFields(log.Fields{
			"volumeId": volumeId,
			"error":    err.Error(),
		})
		return "", err
	}

	log.WithField("publishInfoLocation", publishInfoLocation).Debug("Publish info location found")
	return publishInfoLocation.StagingTargetPath, nil
}

func (p *Plugin) clearStagedTrackingFile(volumeId string) error {
	fields := log.Fields{"volumeId": volumeId}
	log.WithFields(fields).Debug(">>>> clearStagedTrackingFile")
	defer log.WithFields(fields).Debug("<<<< clearTrackingFile")

	trackingFile := volumeId + ".json"
	trackingFilename := path.Join(tridentDeviceInfoPath, trackingFile)

	err := os.Remove(trackingFilename)
	if err != nil {
		log.WithFields(log.Fields{
			"trackingFilename": trackingFilename,
			"error":            err,
		}).Error("Removing tracking file failed.")
	}
	return err
}

func (p *Plugin) getVolumeProtocolFromPublishInfo(publishInfo *utils.VolumePublishInfo) (tridentconfig.Protocol, error) {
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
