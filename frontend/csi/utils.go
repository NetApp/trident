// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Copyright 2017 The Kubernetes Authors.

package csi

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"

	"github.com/netapp/trident/config"
	controllerAPI "github.com/netapp/trident/frontend/csi/controller_api"
	"github.com/netapp/trident/internal/crypto"
	. "github.com/netapp/trident/logging"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils/devices/luks"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

const (
	publishInfoKeyIscsiTargetPortalCount       = "iscsiTargetPortalCount"
	publishInfoKeyIscsiTargetPortal            = "p1"
	publishInfoKeyIscsiEncryptedUsername       = "encryptedIscsiUsername"
	publishInfoKeyIscsiEncryptedSecret         = "encryptedIscsiInitiatorSecret"
	publishInfoKeyIscsiEncryptedTargetUsername = "encryptedIscsiTargetUsername"
	publishInfoKeyIscsiEncryptedTargetSecret   = "encryptedIscsiTargetSecret"
)

var (
	aesKeySingleton []byte
	aesKeyError     error
	readAESKeysOnce sync.Once
)

// ReadAESKey initializes a singleton for AES keys.
func ReadAESKey(ctx context.Context, aesKeyFile string) ([]byte, error) {
	readAESKeysOnce.Do(func() {
		if "" != aesKeyFile {
			aesKeySingleton, aesKeyError = os.ReadFile(aesKeyFile)
		} else {
			Logc(ctx).Warn("AES encryption key not provided!")
		}
	})
	return aesKeySingleton, aesKeyError
}

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

func NewGroupControllerServiceCapability(
	cap csi.GroupControllerServiceCapability_RPC_Type,
) *csi.GroupControllerServiceCapability {
	return &csi.GroupControllerServiceCapability{
		Type: &csi.GroupControllerServiceCapability_Rpc{
			Rpc: &csi.GroupControllerServiceCapability_RPC{
				Type: cap,
			},
		},
	}
}

func setEncryptedCHAPPublishInfo(publishInfo map[string]string, accessInfo *models.VolumeAccessInfo) {
	if publishInfo == nil {
		return
	}
	if accessInfo == nil {
		return
	}
	publishInfo[publishInfoKeyIscsiEncryptedUsername] = accessInfo.IscsiUsername
	publishInfo[publishInfoKeyIscsiEncryptedSecret] = accessInfo.IscsiInitiatorSecret
	publishInfo[publishInfoKeyIscsiEncryptedTargetUsername] = accessInfo.IscsiTargetUsername
	publishInfo[publishInfoKeyIscsiEncryptedTargetSecret] = accessInfo.IscsiTargetSecret
}

func SetEncryptedCHAPPublishInfo(publishInfo map[string]string, accessInfo *models.VolumeAccessInfo) {
	if accessInfo == nil {
		return
	}
	if !accessInfo.UseCHAP {
		return
	}
	setEncryptedCHAPPublishInfo(publishInfo, accessInfo)
	return
}

// encryptCHAPAccessInfo encrypts CHAP credentials within an access info in-place.
func encryptCHAPAccessInfo(
	ctx context.Context, accessInfo *models.VolumeAccessInfo, aesKey []byte,
) error {
	if accessInfo == nil {
		return nil
	}
	if !accessInfo.UseCHAP {
		return nil
	}
	if len(aesKey) == 0 {
		aesKey = aesKeySingleton
	}

	initiatorUser, err := crypto.EncryptStringWithAES(accessInfo.IscsiUsername, aesKey)
	if err != nil {
		Logc(ctx).Errorf("Error encrypting iSCSI username; %v", err)
		return errors.New("error encrypting iscsi username")
	}
	initiatorPass, err := crypto.EncryptStringWithAES(accessInfo.IscsiInitiatorSecret, aesKey)
	if err != nil {
		Logc(ctx).Errorf("Error encrypting initiator secret; %v", err)
		return errors.New("error encrypting initiator secret")
	}
	targetUser, err := crypto.EncryptStringWithAES(accessInfo.IscsiTargetUsername, aesKey)
	if err != nil {
		Logc(ctx).Errorf("Error encrypting target username; %v", err)
		return errors.New("error encrypting target username")
	}
	targetPass, err := crypto.EncryptStringWithAES(accessInfo.IscsiTargetSecret, aesKey)
	if err != nil {
		Logc(ctx).Errorf("Error encrypting target secret; %v", err)
		return errors.New("error encrypting target secret")
	}

	accessInfo.IscsiUsername = initiatorUser
	accessInfo.IscsiInitiatorSecret = initiatorPass
	accessInfo.IscsiTargetUsername = targetUser
	accessInfo.IscsiTargetSecret = targetPass

	return nil
}

// EncryptCHAPAccessInfo encrypts CHAP credentials within an access info in-place.
func EncryptCHAPAccessInfo(
	ctx context.Context, accessInfo *models.VolumeAccessInfo,
) error {
	return encryptCHAPAccessInfo(ctx, accessInfo, aesKeySingleton)
}

// encryptCHAPPublishInfo will encrypt the CHAP credentials from volumePublish and add them to publishInfo
func encryptCHAPPublishInfo(
	ctx context.Context, publishInfo map[string]string, volumePublishInfo *models.VolumePublishInfo, aesKey []byte,
) error {
	if err := encryptCHAPAccessInfo(ctx, &volumePublishInfo.VolumeAccessInfo, aesKey); err != nil {
		return err
	}
	setEncryptedCHAPPublishInfo(publishInfo, &volumePublishInfo.VolumeAccessInfo)
	return nil
}

// decryptCHAPAccessInfo decrypts CHAP credentials within an access info in-place.
func decryptCHAPAccessInfo(
	ctx context.Context, accessInfo *models.VolumeAccessInfo, aesKey []byte,
) error {
	if accessInfo == nil {
		return nil
	}
	if !accessInfo.UseCHAP {
		return nil
	}
	if len(aesKey) == 0 {
		aesKey = aesKeySingleton
	}

	initiatorUser, err := crypto.DecryptStringWithAES(accessInfo.IscsiUsername, aesKey)
	if err != nil {
		Logc(ctx).Errorf("Error decrypting iSCSI username; %v", err)
		return errors.New("error decrypting iscsi username")
	}
	initiatorPass, err := crypto.DecryptStringWithAES(accessInfo.IscsiInitiatorSecret, aesKey)
	if err != nil {
		Logc(ctx).Errorf("Error decrypting initiator secret; %v", err)
		return errors.New("error decrypting initiator secret")
	}
	targetUser, err := crypto.DecryptStringWithAES(accessInfo.IscsiTargetUsername, aesKey)
	if err != nil {
		Logc(ctx).Errorf("Error decrypting target username; %v", err)
		return errors.New("error decrypting target username")
	}
	targetPass, err := crypto.DecryptStringWithAES(accessInfo.IscsiTargetSecret, aesKey)
	if err != nil {
		Logc(ctx).Errorf("Error decrypting target secret; %v", err)
		return errors.New("error decrypting target secret")
	}

	accessInfo.IscsiUsername = initiatorUser
	accessInfo.IscsiInitiatorSecret = initiatorPass
	accessInfo.IscsiTargetUsername = targetUser
	accessInfo.IscsiTargetSecret = targetPass

	return nil
}

// DecryptCHAPAccessInfo decrypts CHAP credentials within an access info in-place.
func DecryptCHAPAccessInfo(
	ctx context.Context, accessInfo *models.VolumeAccessInfo,
) error {
	return decryptCHAPAccessInfo(ctx, accessInfo, aesKeySingleton)
}

// decryptCHAPPublishInfo will decrypt the CHAP credentials from req and replace empty plaintext credential fields in
// publishInfo with their decrypted counterparts
func decryptCHAPPublishInfo(
	ctx context.Context, publishInfo *models.VolumePublishInfo, publishContext map[string]string, aesKey []byte,
) error {
	var err error

	if publishInfo.IscsiUsername == "" && publishContext[publishInfoKeyIscsiEncryptedUsername] != "" {
		if publishInfo.IscsiUsername, err = crypto.DecryptStringWithAES(
			publishContext[publishInfoKeyIscsiEncryptedUsername], aesKey); err != nil {
			Logc(ctx).Errorf("error decrypting iscsi username; %v", err)
			return errors.New("error decrypting iscsi username")
		}
	}

	if publishInfo.IscsiInitiatorSecret == "" && publishContext[publishInfoKeyIscsiEncryptedSecret] != "" {
		if publishInfo.IscsiInitiatorSecret, err = crypto.DecryptStringWithAES(
			publishContext[publishInfoKeyIscsiEncryptedSecret], aesKey); err != nil {
			Logc(ctx).Errorf("error decrypting initiator secret; %v", err)
			return errors.New("error decrypting initiator secret")
		}
	}

	if publishInfo.IscsiTargetUsername == "" && publishContext[publishInfoKeyIscsiEncryptedTargetUsername] != "" {
		if publishInfo.IscsiTargetUsername, err = crypto.DecryptStringWithAES(
			publishContext[publishInfoKeyIscsiEncryptedTargetUsername], aesKey); err != nil {
			Logc(ctx).Errorf("error decrypting target username; %v", err)
			return errors.New("error decrypting target username")
		}
	}

	if publishInfo.IscsiTargetSecret == "" && publishContext[publishInfoKeyIscsiEncryptedTargetSecret] != "" {
		if publishInfo.IscsiTargetSecret, err = crypto.DecryptStringWithAES(
			publishContext[publishInfoKeyIscsiEncryptedTargetSecret], aesKey); err != nil {
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
// the protocol type from the volume (File or Block).
func getVolumeProtocolFromPublishInfo(publishInfo *models.VolumePublishInfo) (config.Protocol, error) {
	nfsIP := publishInfo.VolumeAccessInfo.NfsServerIP
	iqn := publishInfo.VolumeAccessInfo.IscsiTargetIQN
	smbPath := publishInfo.SMBPath
	nqn := publishInfo.VolumeAccessInfo.NVMeSubsystemNQN
	fcp := publishInfo.VolumeAccessInfo.FCTargetWWNN

	nfsSet := nfsIP != ""
	iqnSet := iqn != ""
	smbSet := smbPath != ""
	nqnSet := nqn != ""
	fcpSet := fcp != ""

	isSmb := smbSet && !nfsSet && !iqnSet
	isNfs := nfsSet && !iqnSet && !smbSet
	isIscsi := iqnSet && !nfsSet && !smbSet
	isNVMe := nqnSet && !nfsSet && !smbSet && !iqnSet
	isFCP := fcpSet && !nfsSet && !smbSet

	switch {
	case isSmb, isNfs:
		return config.File, nil
	case isIscsi, isNVMe, isFCP:
		return config.Block, nil
	}

	fields := LogFields{
		"SMBPath":          smbPath,
		"IscsiTargetIQN":   iqn,
		"NfsServerIP":      nfsIP,
		"NVMeSubsystemNQN": nqn,
		"FCTargetWWNN":     fcp,
	}

	errMsg := "unable to infer volume protocol"
	Logc(context.Background()).WithFields(fields).Error(FormatMessageForLog(errMsg))

	return "", errors.New(errMsg)
}

// performProtocolSpecificReconciliation checks the protocol-specific conditions that signify whether a volume exists.
// Nothing is done for NFS because NodeUnstageVolume for NFS only checks for the staging path. The ISCSI and Block on
// File conditions are the same conditions that are checked in NodeUnstageVolume.
func performProtocolSpecificReconciliation(ctx context.Context, trackingInfo *models.VolumeTrackingInfo) (bool, error) {
	Logc(ctx).Trace(">>>> performProtocolSpecificReconciliation")
	defer Logc(ctx).Trace("<<<< performProtocolSpecificReconciliation")

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
		if trackingInfo.SANType == sa.ISCSI {
			atLeastOneConditionMet, err = iscsiUtils.ReconcileISCSIVolumeInfo(ctx, trackingInfo)
			if err != nil {
				return false, fmt.Errorf("unable to reconcile ISCSI volume info: %v", err)
			}
			return atLeastOneConditionMet, nil
		} else if trackingInfo.SANType == sa.FCP {
			atLeastOneConditionMet, err = fcpUtils.ReconcileFCPVolumeInfo(ctx, trackingInfo)
			if err != nil {
				return false, fmt.Errorf("unable to reconcile FCP volume info: %v", err)
			}
			return atLeastOneConditionMet, nil
		}
	}

	return false, nil
}

// ensureLUKSVolumePassphrase ensures the LUKS device has the most recent passphrase and notifies the Trident controller
// of any possibly in use passphrases. If forceUpdate is true, the Trident controller will be notified of the current
// passphrase name, regardless of a rotation.
func ensureLUKSVolumePassphrase(
	ctx context.Context, _ controllerAPI.TridentController, luksDevice luks.Device,
	volumeId string, secrets map[string]string, _ bool,
) error {
	luksPassphraseName, luksPassphrase, previousLUKSPassphraseName,
		previousLUKSPassphrase := luks.GetLUKSPassphrasesFromSecretMap(secrets)
	if luksPassphrase == "" {
		return fmt.Errorf("LUKS passphrase cannot be empty")
	}
	if luksPassphraseName == "" {
		return fmt.Errorf("LUKS passphrase name cannot be empty")
	}

	// Check if passphrase is already up-to-date
	current, err := luksDevice.CheckPassphrase(ctx, luksPassphrase)
	if err != nil {
		return fmt.Errorf("could not verify passphrase %s; %v", luksPassphraseName, err)
	}
	if current {
		Logc(ctx).WithFields(LogFields{
			"volume": volumeId,
		}).Debugf("Current LUKS passphrase name '%s'.", luksPassphraseName)
		// Disabled in all supported versions until 26.06.0. Users must track LUKS passphrases for volumes.
		// if forceUpdate {
		// 	luksPassphraseNames := []string{luksPassphraseName}
		// 	err = restClient.UpdateVolumeLUKSPassphraseNames(ctx, volumeId, luksPassphraseNames)
		// 	if err != nil {
		// 		return fmt.Errorf("could not update current passphrase name for LUKS volume; %v", err)
		// 	}
		// }
		return nil
	}

	// Check if previous passphrase is set, otherwise we can't rotate
	var previous bool
	if previousLUKSPassphrase != "" {
		if previousLUKSPassphraseName == "" {
			return fmt.Errorf("previous LUKS passphrase name cannot be empty if previous LUKS passphrase is also specified")
		}
		previous, err = luksDevice.CheckPassphrase(ctx, previousLUKSPassphrase)
		if err != nil {
			return fmt.Errorf("could not verify passphrase %s; %v", luksPassphraseName, err)
		}
	}
	if !previous {
		return fmt.Errorf("no working passphrase provided")
	}
	Logc(ctx).WithFields(LogFields{
		"volume": volumeId,
	}).Debugf("Current LUKS passphrase name '%s'.", previousLUKSPassphraseName)

	// Disabled in all supported versions until 26.06.0. Users must track LUKS passphrases for volumes.
	// Send up current and previous passphrase names, if rotation fails
	// luksPassphraseNames := []string{luksPassphraseName, previousLUKSPassphraseName}
	// err = restClient.UpdateVolumeLUKSPassphraseNames(ctx, volumeId, luksPassphraseNames)
	// if err != nil {
	// 	return fmt.Errorf("could not update passphrase names for LUKS volume, skipping passphrase rotation; %v", err)
	// }

	// Rotate
	Logc(ctx).WithFields(LogFields{
		"volume":                       volumeId,
		"current-luks-passphrase-name": previousLUKSPassphraseName,
		"new-luks-passphrase-name":     luksPassphraseName,
	}).Info("Rotating LUKS passphrase.")
	err = luksDevice.RotatePassphrase(ctx, volumeId, previousLUKSPassphrase, luksPassphrase)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"volume":                       volumeId,
			"current-luks-passphrase-name": previousLUKSPassphraseName,
			"new-luks-passphrase-name":     luksPassphraseName,
		}).WithError(err).Errorf("Failed to rotate LUKS passphrase.")
		return fmt.Errorf("failed to rotate LUKS passphrase; %w", err)
	}
	Logc(ctx).Infof("Rotated LUKS passphrase")

	// isCurrent, err := luksDevice.CheckPassphrase(ctx, luksPassphrase)
	// if err != nil {
	// 	return fmt.Errorf("could not check current passphrase for LUKS volume; %v", err)
	// Disabled in all supported versions until 26.06.0. Users must track LUKS passphrases for volumes.
	// } else if isCurrent {
	// 	// Send only current passphrase up
	// 	luksPassphraseNames = []string{luksPassphraseName}
	// 	err = restClient.UpdateVolumeLUKSPassphraseNames(ctx, volumeId, luksPassphraseNames)
	// 	if err != nil {
	// 		return fmt.Errorf("could not update passphrase names for LUKS volume after rotation; %v", err)
	// 	}
	// }
	// }
	return nil
}

func addAccessInfoPortalsToPublishInfo(publishInfo map[string]string, accessInfo models.VolumeAccessInfo) {
	count := 1 + len(accessInfo.IscsiPortals)
	publishInfo[publishInfoKeyIscsiTargetPortalCount] = strconv.Itoa(count)
	publishInfo[publishInfoKeyIscsiTargetPortal] = accessInfo.IscsiTargetPortal
	for i, p := range accessInfo.IscsiPortals {
		key := fmt.Sprintf("p%d", i+2)
		publishInfo[key] = p
	}
}

func stashIscsiTargetPortals(publishInfo map[string]string, volumePublishInfo *models.VolumePublishInfo) {
	addAccessInfoPortalsToPublishInfo(publishInfo, volumePublishInfo.VolumeAccessInfo)
}

// UpsertIscsiTargetPortals replaces the existing target portals in a publishInfo
// with the portals in the supplied accessInfo.
func UpsertIscsiTargetPortals(
	ctx context.Context, publishInfo map[string]string, accessInfo *models.VolumeAccessInfo,
) error {
	if len(publishInfo) == 0 {
		return fmt.Errorf("publishInfo is empty")
	}
	if accessInfo == nil {
		return fmt.Errorf("accessInfo is empty")
	}

	pMax, err := strconv.Atoi(publishInfo[publishInfoKeyIscsiTargetPortalCount])
	if err != nil {
		Logc(ctx).Error(err.Error())
		return err
	}
	for pNum := 0; pNum < pMax+1; pNum++ {
		delete(publishInfo, fmt.Sprintf("p%v", pNum))
	}
	delete(publishInfo, publishInfoKeyIscsiTargetPortal)

	addAccessInfoPortalsToPublishInfo(publishInfo, *accessInfo)
	return nil
}
