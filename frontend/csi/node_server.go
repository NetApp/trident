// Copyright 2019 NetApp, Inc. All Rights Reserved.

package csi

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/netapp/trident/frontend/rest"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/utils"
)

const volumePublishInfoFilename = "volumePublishInfo.json"

func (p *Plugin) NodeStageVolume(
	ctx context.Context, req *csi.NodeStageVolumeRequest,
) (*csi.NodeStageVolumeResponse, error) {

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

	fields := log.Fields{"Method": "NodeUnstageVolume", "Type": "CSI_Node"}
	log.WithFields(fields).Debug(">>>> NodeUnstageVolume")
	defer log.WithFields(fields).Debug("<<<< NodeUnstageVolume")

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "no volume ID provided")
	}

	targetPath := req.GetStagingTargetPath()

	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "no staging target path provided")
	}

	// Read the device info from the staging path
	publishInfo, err := p.readStagedDeviceInfo(targetPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	protocol, err := p.getVolumeProtocolFromPublishInfo(publishInfo)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	switch protocol {
	case tridentconfig.File:
		return p.nodeUnstageNFSVolume(ctx, req, publishInfo)
	case tridentconfig.Block:
		return p.nodeUnstageISCSIVolume(ctx, req, publishInfo)
	default:
		return nil, status.Error(codes.InvalidArgument, "unknown protocol")
	}
}

func (p *Plugin) NodePublishVolume(
	ctx context.Context, req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {

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

	notMnt, err := utils.IsLikelyNotMountPoint(targetPath)

	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, "target path not found")
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	if notMnt {
		return nil, status.Error(codes.NotFound, "volume not mounted")
	}

	if err := utils.Umount(targetPath); err != nil {
		log.WithFields(log.Fields{"path": targetPath, "error": err}).Error("unable to unmount volume.")
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (p *Plugin) NodeGetVolumeStats(
	ctx context.Context, req *csi.NodeGetVolumeStatsRequest,
) (*csi.NodeGetVolumeStatsResponse, error) {

	// Trident doesn't support GET_VOLUME_STATS capability
	return nil, status.Error(codes.Unimplemented, "")
}

func (p *Plugin) NodeExpandVolume(
	ctx context.Context, in *csi.NodeExpandVolumeRequest,
) (*csi.NodeExpandVolumeResponse, error) {

	// Trident doesn't support expansion via CSI
	return nil, status.Error(codes.Unimplemented, "")
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
		log.WithField("error", err).Error("Could not get iSCSI initiator name.")
	} else if iscsiWWNs == nil || len(iscsiWWNs) == 0 {
		log.Warn("Could not get iSCSI initiator name.")
	} else {
		iscsiWWN = iscsiWWNs[0]
	}

	// TODO (akerr): add IP discovery here as well

	node := &utils.Node{
		Name: p.nodeName,
		IQN:  iscsiWWN,
	}
	return node
}

func (p *Plugin) nodeRegisterWithController() error {
	node := p.nodeGetInfo()
	// The controller may not be fully initialized by the time the node is ready to register,
	// so wait here until it is ready.
	checkBackChannelReady := func() error {
		_, err := p.restClient.GetNodes()
		return err
	}
	backChannelReadyNotify := func(err error, duration time.Duration) {
		log.WithField("increment", duration).Debug("Controller not yet ready, waiting.")
	}
	backChannelBackoff := backoff.NewExponentialBackOff()
	backChannelBackoff.InitialInterval = 1 * time.Second
	backChannelBackoff.Multiplier = 2
	backChannelBackoff.RandomizationFactor = 0.1
	backChannelBackoff.MaxElapsedTime = rest.HTTPTimeout

	// Run the check using an exponential backoff
	if err := backoff.RetryNotify(checkBackChannelReady, backChannelBackoff, backChannelReadyNotify); err != nil {
		log.WithField("node", p.nodeName).Errorf("Could not communicate with controller after %3.2f seconds; %v",
			backChannelBackoff.MaxElapsedTime.Seconds(), err)
	} else {
		log.WithField("node", p.nodeName).Debug("Communication with controller established.")
	}
	err := p.restClient.CreateNode(node)
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) nodeDeregisterWithController() error {
	err := p.restClient.DeleteNode(p.nodeName)
	if err != nil {
		return err
	}
	return nil
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

	// Save the device info to the staging path for use in the publish & unstage calls
	if err := p.writeStagedDeviceInfo(req.StagingTargetPath, publishInfo); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.NodeStageVolumeResponse{}, nil
}

func (p *Plugin) nodeUnstageNFSVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest, publishInfo *utils.VolumePublishInfo,
) (*csi.NodeUnstageVolumeResponse, error) {

	// Delete the device info we saved to the staging path so unstage can succeed
	if err := p.clearStagedDeviceInfo(req.StagingTargetPath); err != nil {
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

	fstype := "ext4"
	mountCapability := req.GetVolumeCapability().GetMount()
	if mountCapability != nil {
		if mountCapability.GetFsType() != "" {
			fstype = mountCapability.GetFsType()
		}
	}

	if fstype == "" {
		fstype = req.PublishContext["filesystemType"]
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

	// Perform the login/rescan/discovery/format & get the device back in the publish info
	if err := utils.AttachISCSIVolume(req.VolumeContext["internalName"], "", publishInfo); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Save the device info to the staging path for use in the publish & unstage calls
	if err := p.writeStagedDeviceInfo(req.StagingTargetPath, publishInfo); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (p *Plugin) nodeUnstageISCSIVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest, publishInfo *utils.VolumePublishInfo,
) (*csi.NodeUnstageVolumeResponse, error) {

	// Delete the device from the host
	utils.PrepareDeviceForRemoval(int(publishInfo.IscsiLunNumber), publishInfo.IscsiTargetIQN)

	// Logout of the iSCSI session if appropriate
	logout := false
	if !publishInfo.SharedTarget {
		// Always log out of a non-shared target
		logout = true
	} else {
		// Log out of a shared target if no mounts to that target remain
		anyMounts, err := utils.ISCSITargetHasMountedDevice(publishInfo.IscsiTargetIQN)
		logout = (err == nil) && !anyMounts
	}
	if logout {
		utils.ISCSIDisableDelete(publishInfo.IscsiTargetIQN, publishInfo.IscsiTargetPortal)
		for _, portal := range publishInfo.IscsiPortals {
			utils.ISCSIDisableDelete(publishInfo.IscsiTargetIQN, portal)
		}
	}

	// Delete the device info we saved to the staging path so unstage can succeed
	if err := p.clearStagedDeviceInfo(req.StagingTargetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
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

	// Mount the device
	err = utils.MountDevice(publishInfo.DevicePath, req.TargetPath, publishInfo.MountOptions)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (p *Plugin) writeStagedDeviceInfo(stagingTargetPath string, publishInfo *utils.VolumePublishInfo) error {

	publishInfoBytes, err := json.Marshal(publishInfo)
	if err != nil {
		return err
	}

	filename := path.Join(stagingTargetPath, volumePublishInfoFilename)

	if err := ioutil.WriteFile(filename, publishInfoBytes, 0600); err != nil {
		return err
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

func (p *Plugin) clearStagedDeviceInfo(stagingTargetPath string) error {
	filename := path.Join(stagingTargetPath, volumePublishInfoFilename)
	return os.Remove(filename)
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
