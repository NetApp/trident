// Copyright 2021 NetApp, Inc. All Rights Reserved.

// Copyright 2017 The Kubernetes Authors.

package csi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/utils"
)

func ParseEndpoint(ep string) (string, string, error) {
	if strings.HasPrefix(strings.ToLower(ep), "unix://") || strings.HasPrefix(strings.ToLower(ep), "tcp://") {
		s := strings.SplitN(ep, "://", 2)
		if s[1] != "" {
			return s[0], s[1], nil
		}
	}
	return "", "", fmt.Errorf("invalid endpoint: %v", ep)
}

func NewVolumeCapabilityAccessMode(mode csi.VolumeCapability_AccessMode_Mode) *csi.VolumeCapability_AccessMode {
	return &csi.VolumeCapability_AccessMode{Mode: mode}
}

func NewControllerServiceCapability(cap csi.ControllerServiceCapability_RPC_Type) *csi.ControllerServiceCapability {
	return &csi.ControllerServiceCapability{
		Type: &csi.ControllerServiceCapability_Rpc{
			Rpc: &csi.ControllerServiceCapability_RPC{
				Type: cap,
			},
		},
	}
}

func NewNodeServiceCapability(cap csi.NodeServiceCapability_RPC_Type) *csi.NodeServiceCapability {
	return &csi.NodeServiceCapability{
		Type: &csi.NodeServiceCapability_Rpc{
			Rpc: &csi.NodeServiceCapability_RPC{
				Type: cap,
			},
		},
	}
}

func logGRPC(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{},
	error,
) {
	ctx = GenerateRequestContext(ctx, "", ContextSourceCSI)
	Logc(ctx).Debugf("GRPC call: %s", info.FullMethod)
	Logc(ctx).Debugf("GRPC request: %+v", req)
	resp, err := handler(ctx, req)
	if err != nil {
		Logc(ctx).Errorf("GRPC error: %v", err)
	} else {
		Logc(ctx).Debugf("GRPC response: %+v", resp)
	}
	return resp, err
}

// encryptCHAPPublishInfo will encrypt the CHAP credentials from volumePublish and add them to publishInfo
func encryptCHAPPublishInfo(
	ctx context.Context, publishInfo map[string]string, volumePublishInfo *utils.VolumePublishInfo, aesKey []byte,
) error {
	var err error
	if publishInfo["encryptedIscsiUsername"], err = utils.EncryptStringWithAES(
		volumePublishInfo.IscsiUsername, aesKey); err != nil {
		Logc(ctx).Errorf("Error encrypting iSCSI username; %v", err)
		return errors.New("error encrypting iscsi username")
	}
	if publishInfo["encryptedIscsiInitiatorSecret"], err = utils.EncryptStringWithAES(
		volumePublishInfo.IscsiInitiatorSecret, aesKey); err != nil {
		Logc(ctx).Errorf("Error encrypting initiator secret; %v", err)
		return errors.New("error encrypting initiator secret")
	}
	if publishInfo["encryptedIscsiTargetUsername"], err = utils.EncryptStringWithAES(
		volumePublishInfo.IscsiTargetUsername, aesKey); err != nil {
		Logc(ctx).Errorf("Error encrypting target username; %v", err)
		return errors.New("error encrypting target username")
	}
	if publishInfo["encryptedIscsiTargetSecret"], err = utils.EncryptStringWithAES(
		volumePublishInfo.IscsiTargetSecret, aesKey); err != nil {
		Logc(ctx).Errorf("Error encrypting target secret; %v", err)
		return errors.New("error encrypting target secret")
	}
	return nil
}

// decryptCHAPPublishInfo will decrypt the CHAP credentials from req and replace empty plaintext credential fields in
// publishInfo with their decrypted counterparts
func decryptCHAPPublishInfo(
	ctx context.Context, publishInfo *utils.VolumePublishInfo, publishContext map[string]string, aesKey []byte,
) error {
	var err error

	if publishInfo.IscsiUsername == "" && publishContext["encryptedIscsiUsername"] != "" {
		if publishInfo.IscsiUsername, err = utils.DecryptStringWithAES(publishContext["encryptedIscsiUsername"],
			aesKey); err != nil {
			Logc(ctx).Errorf("error decrypting iscsi username; %v", err)
			return errors.New("error decrypting iscsi username")
		}
	}

	if publishInfo.IscsiInitiatorSecret == "" && publishContext["encryptedIscsiInitiatorSecret"] != "" {
		if publishInfo.IscsiInitiatorSecret, err = utils.DecryptStringWithAES(
			publishContext["encryptedIscsiInitiatorSecret"], aesKey); err != nil {
			Logc(ctx).Errorf("error decrypting initiator secret; %v", err)
			return errors.New("error decrypting initiator secret")
		}
	}

	if publishInfo.IscsiTargetUsername == "" && publishContext["encryptedIscsiTargetUsername"] != "" {
		if publishInfo.IscsiTargetUsername, err = utils.DecryptStringWithAES(
			publishContext["encryptedIscsiTargetUsername"], aesKey); err != nil {
			Logc(ctx).Errorf("error decrypting target username; %v", err)
			return errors.New("error decrypting target username")
		}
	}

	if publishInfo.IscsiTargetSecret == "" && publishContext["encryptedIscsiTargetSecret"] != "" {
		if publishInfo.IscsiTargetSecret, err = utils.DecryptStringWithAES(publishContext["encryptedIscsiTargetSecret"],
			aesKey); err != nil {
			Logc(ctx).Errorf("error decrypting target secret; %v", err)
			return errors.New("error decrypting target secret")
		}
	}
	return nil
}

func containsEncryptedCHAP(input map[string]string) bool {
	encryptedCHAPFields := []string{
		"encryptedIscsiUsername",
		"encryptedIscsiInitiatorSecret",
		"encryptedIscsiTargetUsername",
		"encryptedIscsiTargetSecret",
	}
	for _, field := range encryptedCHAPFields {
		if _, found := input[field]; found {
			return true
		}
	}
	return false
}

// getVolumeProtocolFromPublishInfo examines the publish info read from the staging target path and determines
// the protocol type from the volume (File or Block or Block-on-File).
func getVolumeProtocolFromPublishInfo(publishInfo *utils.VolumePublishInfo) (config.Protocol, error) {
	nfsIP := publishInfo.VolumeAccessInfo.NfsServerIP
	iqn := publishInfo.VolumeAccessInfo.IscsiTargetIQN
	subvolName := publishInfo.VolumeAccessInfo.SubvolumeName
	smbPath := publishInfo.SMBPath

	nfsSet := nfsIP != ""
	iqnSet := iqn != ""
	subvolSet := subvolName != ""
	smbSet := smbPath != ""

	isSmb := smbSet && !nfsSet && !iqnSet
	isNfs := nfsSet && !iqnSet && !smbSet
	isBof := isNfs && subvolSet
	isIscsi := iqnSet && !nfsSet && !smbSet

	if isSmb || (isNfs && !isBof) {
		return config.File, nil
	} else if isBof {
		return config.BlockOnFile, nil
	} else if isIscsi {
		return config.Block, nil
	}

	fields := log.Fields{
		"SMBPath":        smbPath,
		"SubvolumeName":  subvolName,
		"IscsiTargetIQN": iqn,
		"NfsServerIP":    nfsIP,
	}

	errMsg := "unable to infer volume protocol"
	Logc(context.Background()).WithFields(fields).Error(FormatMessageForLog(errMsg))

	return "", fmt.Errorf(errMsg)
}

// performProtocolSpecificReconciliation checks the protocol-specific conditions that signify whether a volume exists.
// Nothing is done for NFS because NodeUnstageVolume for NFS only checks for the staging path. The ISCSI and Block on
// File conditions are the same conditions that are checked in NodeUnstageVolume.
func performProtocolSpecificReconciliation(ctx context.Context, trackingInfo *utils.VolumeTrackingInfo) (bool, error) {
	Logc(ctx).Debug(">>>> performProtocolSpecificReconciliation")
	defer Logc(ctx).Debug("<<<< performProtocolSpecificReconciliation")

	atLeastOneConditionMet := false
	protocol, err := getVolumeProtocolFromPublishInfo(&trackingInfo.VolumePublishInfo)
	if err != nil {
		// If we are unable to determine the protocol from the publish info then something is very wrong and we consider
		// this an invalid tracking file.
		errMsg := fmt.Sprintf("unable to determine protocol info from publish info; %v", err)
		return false, InvalidTrackingFileError(errMsg)
	}

	// Nothing more than checking the staging path needs to be done for NFS, so ignore that case.
	switch protocol {
	case config.Block:
		atLeastOneConditionMet, err = iscsiUtils.ReconcileISCSIVolumeInfo(ctx, trackingInfo)
		if err != nil {
			return false, fmt.Errorf("unable to reconcile ISCSI volume info: %v", err)
		}
		return atLeastOneConditionMet, nil
	case config.BlockOnFile:
		atLeastOneConditionMet, err = bofUtils.ReconcileBlockOnFileVolumeInfo(ctx, trackingInfo)
		if err != nil {
			return false, fmt.Errorf("unable to reconcile Block-on-file volume info: %v", err)
		}
		return atLeastOneConditionMet, nil
	}

	return false, nil
}
