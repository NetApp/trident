// Copyright 2018 NetApp, Inc. All Rights Reserved.

package csi

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi/v0"
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

	switch req.PublishInfo["protocol"] {
	case string(tridentconfig.File):
		return &csi.NodeStageVolumeResponse{}, nil // No need to stage NFS
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

	_, protocol, err := p.parseVolumeID(req.VolumeId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	switch protocol {
	case tridentconfig.File:
		return &csi.NodeUnstageVolumeResponse{}, nil // No need to unstage NFS
	case tridentconfig.Block:
		return p.nodeUnstageISCSIVolume(ctx, req)
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

	switch req.PublishInfo["protocol"] {
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

	targetPath := req.GetTargetPath()
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

func (p *Plugin) NodeGetId(
	ctx context.Context, req *csi.NodeGetIdRequest,
) (*csi.NodeGetIdResponse, error) {

	fields := log.Fields{"Method": "NodeGetId", "Type": "CSI_Node"}
	log.WithFields(fields).Debug(">>>> NodeGetId")
	defer log.WithFields(fields).Debug("<<<< NodeGetId")

	iscsiWWN := ""
	iscsiWWNs, err := utils.GetInitiatorIqns()
	if err != nil {
		log.WithField("error", err).Error("Could not get iSCSI initiator name.")
	} else if iscsiWWNs == nil || len(iscsiWWNs) == 0 {
		log.Warn("Could not get iSCSI initiator name.")
	} else {
		iscsiWWN = iscsiWWNs[0]
	}

	// Encode node info as JSON and return as the opaque node ID
	nodeID := &TridentNodeID{
		Name: p.nodeName,
		IQN:  iscsiWWN,
	}
	nodeIDbytes, err := json.Marshal(nodeID)
	if err != nil {
		log.WithFields(log.Fields{
			"nodeID": nodeID,
			"error":  err,
		}).Error("Could not marshal node ID struct.")
		return nil, status.Error(codes.Internal, "could not marshal node ID struct")
	}

	return &csi.NodeGetIdResponse{NodeId: string(nodeIDbytes)}, nil
}

func (p *Plugin) NodeGetCapabilities(
	ctx context.Context, req *csi.NodeGetCapabilitiesRequest,
) (*csi.NodeGetCapabilitiesResponse, error) {

	fields := log.Fields{"Method": "NodeGetCapabilities", "Type": "CSI_Node"}
	log.WithFields(fields).Debug(">>>> NodeGetCapabilities")
	defer log.WithFields(fields).Debug("<<<< NodeGetCapabilities")

	return &csi.NodeGetCapabilitiesResponse{Capabilities: p.nsCap}, nil
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

	mountOptions := make([]string, 0)
	mountCapability := req.GetVolumeCapability().GetMount()
	if mountCapability != nil && mountCapability.GetMountFlags() != nil {
		mountOptions = mountCapability.GetMountFlags()
	}
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	publishInfo := &utils.VolumePublishInfo{
		Localhost:      true,
		FilesystemType: "nfs",
		MountOptions:   strings.Join(mountOptions, ","),
	}

	publishInfo.NfsServerIP = req.PublishInfo["nfsServerIp"]
	publishInfo.NfsPath = req.PublishInfo["nfsPath"]

	err = utils.AttachNFSVolume(req.VolumeAttributes["internalName"], req.TargetPath, publishInfo)
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
		fstype = req.PublishInfo["filesystemType"]
	}

	useCHAP, err := strconv.ParseBool(req.PublishInfo["useCHAP"])
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	sharedTarget, err := strconv.ParseBool(req.PublishInfo["sharedTarget"])
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	lunID, err := strconv.Atoi(req.PublishInfo["iscsiLunNumber"])
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	publishInfo := &utils.VolumePublishInfo{
		Localhost:      true,
		FilesystemType: fstype,
		UseCHAP:        useCHAP,
		SharedTarget:   sharedTarget,
	}

	publishInfo.IscsiTargetPortal = req.PublishInfo["iscsiTargetPortal"]
	publishInfo.IscsiTargetIQN = req.PublishInfo["iscsiTargetIqn"]
	publishInfo.IscsiLunNumber = int32(lunID)
	publishInfo.IscsiInterface = req.PublishInfo["iscsiInterface"]
	publishInfo.IscsiIgroup = req.PublishInfo["iscsiIgroup"]
	publishInfo.IscsiUsername = req.PublishInfo["iscsiUsername"]
	publishInfo.IscsiInitiatorSecret = req.PublishInfo["iscsiInitiatorSecret"]
	publishInfo.IscsiTargetSecret = req.PublishInfo["iscsiTargetSecret"]

	// Perform the login/rescan/discovery/format & get the device back in the publish info
	if err := utils.AttachISCSIVolume(req.VolumeAttributes["internalName"], "", publishInfo); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Save the device info to the staging path for use in the publish & unstage calls
	if err := p.writeStagedDeviceInfo(req.StagingTargetPath, publishInfo); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (p *Plugin) nodeUnstageISCSIVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest,
) (*csi.NodeUnstageVolumeResponse, error) {

	// Read the device info from the staging path
	publishInfo, err := p.readStagedDeviceInfo(req.StagingTargetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

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
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (p *Plugin) nodePublishISCSIVolume(
	ctx context.Context, req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {

	var err error

	mountOptions := make([]string, 0)
	mountCapability := req.GetVolumeCapability().GetMount()
	if mountCapability != nil {
		if mountCapability.GetMountFlags() != nil {
			mountOptions = mountCapability.GetMountFlags()
		}
	}
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	// Read the device info from the staging path
	publishInfo, err := p.readStagedDeviceInfo(req.StagingTargetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Mount the device
	err = utils.MountDevice(publishInfo.DevicePath, req.TargetPath, strings.Join(mountOptions, ","))
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

	return &publishInfo, nil
}
