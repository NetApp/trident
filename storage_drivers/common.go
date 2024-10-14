// Copyright 2023 NetApp, Inc. All Rights Reserved.

package storagedrivers

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	trident "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
	tridentmodels "github.com/netapp/trident/utils/models"
)

var ontapConfigRedactList = [...]string{
	"Username", "Password", "ChapUsername", "ChapInitiatorSecret",
	"ChapTargetUsername", "ChapTargetInitiatorSecret", "ClientPrivateKey",
}

func GetOntapConfigRedactList() []string {
	clone := ontapConfigRedactList
	return clone[:]
}

// ValidateCommonSettings attempts to "partially" decode the JSON into just the settings in CommonStorageDriverConfig
func ValidateCommonSettings(ctx context.Context, configJSON string) (*CommonStorageDriverConfig, error) {
	config := &CommonStorageDriverConfig{}

	// Decode configJSON into config object
	err := json.Unmarshal([]byte(configJSON), &config)
	if err != nil {
		return nil, fmt.Errorf("could not parse JSON configuration: %v", err)
	}

	// Load storage drivers and validate the one specified actually exists
	if config.StorageDriverName == "" {
		return nil, errors.New("missing storage driver name in configuration file")
	}

	// Validate config file version information
	if config.Version != ConfigVersion {
		return nil, fmt.Errorf("unexpected config file version; found %d, expected %d", config.Version, ConfigVersion)
	}

	// Warn about ignored fields in common config if any are set
	if config.DisableDelete {
		Logc(ctx).WithFields(LogFields{
			"driverName": config.StorageDriverName,
		}).Warn("disableDelete set in backend config.  This will be ignored.")
	}
	if config.Debug {
		Logc(ctx).Warnf("The debug setting in the configuration file is now ignored; " +
			"use the command line --debug switch instead.")
	}

	var parsedStoragePrefix *string
	if parsedStoragePrefix, err = parseRawStoragePrefix(ctx, config.StoragePrefixRaw); err != nil {
		return nil, fmt.Errorf("unable to parse storage prefix: %v", err)
	}
	config.StoragePrefix = parsedStoragePrefix

	// Validate volume size limit (if set)
	if config.LimitVolumeSize != "" {
		if _, err = utils.ConvertSizeToBytes(config.LimitVolumeSize); err != nil {
			return nil, fmt.Errorf("invalid value for limitVolumeSize: %v", config.LimitVolumeSize)
		}
	}

	if config.Credentials != nil {
		Logc(ctx).Debug("Credentials field not empty.")

		if _, _, err := config.GetCredentials(); err != nil {
			return nil, err
		}
	}

	Logc(ctx).Debugf("Parsed commonConfig: %+v", *config)

	return config, nil
}

// parseRawStoragePrefix parses a raw storage prefix and returns a pointer to a parsed prefix.
func parseRawStoragePrefix(ctx context.Context, storagePrefixRaw json.RawMessage) (*string, error) {
	// The storage prefix may have three states: nil (no prefix specified, drivers will use
	// a default prefix), "" (specified as an empty string, drivers will use no prefix), and
	// "<value>" (a prefix specified in the backend config file).  For historical reasons,
	// the value is serialized as a raw JSON string (a byte array), and it may take multiple
	// forms.  An empty byte array, or an array with the ASCII values {} or null, is interpreted
	// as nil (no prefix specified).  A byte array containing two double-quote characters ("")
	// is an empty string.  A byte array containing characters enclosed in double quotes is
	// a specified prefix.  Anything else is rejected as invalid.  The storage prefix is exposed
	// to the rest of the code in StoragePrefix; only serialization code such as this should
	// be concerned with StoragePrefixRaw.
	var storagePrefix *string

	if len(storagePrefixRaw) > 0 {
		rawPrefix := string(storagePrefixRaw)
		if rawPrefix == "{}" || rawPrefix == "null" {
			storagePrefix = nil
			Logc(ctx).Debugf("Storage prefix is %s, will use default prefix.", rawPrefix)
		} else if rawPrefix == "\"\"" {
			empty := ""
			storagePrefix = &empty
			Logc(ctx).Debug("Storage prefix is empty, will use no prefix.")
		} else if strings.HasPrefix(rawPrefix, "\"") && strings.HasSuffix(rawPrefix, "\"") {
			prefix := string(storagePrefixRaw[1 : len(storagePrefixRaw)-1])
			storagePrefix = &prefix
			Logc(ctx).WithField("storagePrefix", prefix).Debug("Parsed storage prefix.")
		} else {
			return nil, fmt.Errorf("invalid value for storage prefix: %v", storagePrefixRaw)
		}
	} else {
		storagePrefix = nil
		Logc(ctx).Debug("Storage prefix is absent, will use default prefix.")
	}

	return storagePrefix, nil
}

func GetDefaultStoragePrefix(context trident.DriverContext) string {
	switch context {
	default:
		return ""
	case trident.ContextCSI:
		return DefaultTridentStoragePrefix
	case trident.ContextDocker:
		return DefaultDockerStoragePrefix
	}
}

func GetDefaultIgroupName(context trident.DriverContext) string {
	switch context {
	default:
		fallthrough
	case trident.ContextCSI:
		return DefaultTridentIgroupName
	case trident.ContextDocker:
		return DefaultDockerIgroupName
	}
}

func SanitizeCommonStorageDriverConfig(c *CommonStorageDriverConfig) {
	if c != nil && c.StoragePrefixRaw == nil {
		c.StoragePrefixRaw = json.RawMessage("{}")
	}
}

func GetCommonInternalVolumeName(c *CommonStorageDriverConfig, name string) string {
	prefixToUse := trident.OrchestratorName

	// If a prefix was specified in the configuration, use that.
	if c.StoragePrefix != nil {
		prefixToUse = *c.StoragePrefix
	}

	// Special case an empty prefix so that we don't get a delimiter in front.
	if prefixToUse == "" {
		return name
	}

	return fmt.Sprintf("%s-%s", prefixToUse, name)
}

// CheckVolumeSizeLimits if a limit has been set, ensures the requestedSize is under it.
func CheckVolumeSizeLimits(
	ctx context.Context, requestedSizeInt uint64, config *CommonStorageDriverConfig,
) (bool, uint64, error) {
	requestedSize := float64(requestedSizeInt)
	// If the user specified a limit for volume size, parse and enforce it
	limitVolumeSize := config.LimitVolumeSize
	Logc(ctx).WithFields(LogFields{
		"limitVolumeSize": limitVolumeSize,
	}).Debugf("Limits")
	if limitVolumeSize == "" {
		Logc(ctx).Debugf("No limits specified, not limiting volume size")
		return false, 0, nil
	}

	volumeSizeLimit := uint64(0)
	volumeSizeLimitStr, parseErr := utils.ConvertSizeToBytes(limitVolumeSize)
	if parseErr != nil {
		return false, 0, fmt.Errorf("error parsing limitVolumeSize: %v", parseErr)
	}
	volumeSizeLimit, _ = strconv.ParseUint(volumeSizeLimitStr, 10, 64)

	Logc(ctx).WithFields(LogFields{
		"limitVolumeSize":    limitVolumeSize,
		"volumeSizeLimit":    volumeSizeLimit,
		"requestedSizeBytes": requestedSize,
	}).Debugf("Comparing limits")

	if requestedSize > float64(volumeSizeLimit) {
		return true, volumeSizeLimit, errors.UnsupportedCapacityRangeError(fmt.Errorf(
			"requested size: %1.f > the size limit: %d", requestedSize, volumeSizeLimit))
	}

	return true, volumeSizeLimit, nil
}

// CheckMinVolumeSize returns UnsupportedCapacityRangeError if the requested volume size is less than the minimum
// volume size
func CheckMinVolumeSize(requestedSizeBytes, minVolumeSizeBytes uint64) error {
	if requestedSizeBytes < minVolumeSizeBytes {
		return errors.UnsupportedCapacityRangeError(fmt.Errorf("requested volume size ("+
			"%d bytes) is too small; the minimum volume size is %d bytes",
			requestedSizeBytes, minVolumeSizeBytes))
	}
	return nil
}

// CalculateVolumeSizeBytes calculates the size of a volume taking into account the snapshot reserve
func CalculateVolumeSizeBytes(
	ctx context.Context, volume string, requestedSizeBytes uint64, snapshotReserve int,
) uint64 {
	snapReserveDivisor := 1.0 - (float64(snapshotReserve) / 100.0)

	sizeWithSnapReserve := float64(requestedSizeBytes) / snapReserveDivisor

	volumeSizeBytes := uint64(sizeWithSnapReserve)

	Logc(ctx).WithFields(LogFields{
		"volume":              volume,
		"snapReserveDivisor":  snapReserveDivisor,
		"requestedSize":       requestedSizeBytes,
		"sizeWithSnapReserve": sizeWithSnapReserve,
		"volumeSizeBytes":     volumeSizeBytes,
	}).Debug("Calculated optimal size for volume with snapshot reserve.")

	return volumeSizeBytes
}

// Clone will create a copy of the source object and store it into the destination object (which must be a pointer)
func Clone(ctx context.Context, source, destination interface{}) {
	if reflect.TypeOf(destination).Kind() != reflect.Ptr {
		Logc(ctx).Error("storage_drivers.Clone, destination parameter must be a pointer")
	}

	buff := new(bytes.Buffer)
	enc := gob.NewEncoder(buff)
	dec := gob.NewDecoder(buff)
	if err := enc.Encode(source); err != nil {
		Logc(ctx).Error(err)
	}
	if err := dec.Decode(destination); err != nil {
		Logc(ctx).Error(err)
	}
}

// CheckSupportedFilesystem checks for a supported file system type
func CheckSupportedFilesystem(ctx context.Context, fs, volumeInternalName string) (string, error) {
	fsType, err := utils.VerifyFilesystemSupport(fs)
	if err == nil {
		Logc(ctx).WithFields(LogFields{
			"fileSystemType": fsType,
			"name":           volumeInternalName,
		}).Debug("Filesystem format.")
	}

	return fsType, err
}

func AreSameCredentials(credentials1, credentials2 map[string]string) bool {
	secretName1, secretStore1, err := getCredentialNameAndType(credentials1)
	if err != nil {
		return false
	}

	secretName2, secretStore2, err := getCredentialNameAndType(credentials2)
	if err != nil {
		return false
	}

	return secretName1 == secretName2 && secretStore1 == secretStore2
}

// EnsureMountOption ensures option is present in mount options; option is appended to mountOptions if not present
func EnsureMountOption(mountOptions, option string) string {
	return ensureJoinedStringContainsElem(mountOptions, option, ",")
}

// ensureJoinedStringContainsElem adds elem to joined string if not present; requires that elem is not part of a longer
// element
func ensureJoinedStringContainsElem(joined, elem, sep string) string {
	if strings.Contains(joined, elem) {
		return joined
	}
	if joined == "" {
		return elem
	}
	return joined + sep + elem
}

// EncodeStorageBackendPools serializes and base64 encodes backend storage pools within the driver's backend;
// it is shared by all storage drivers.
func EncodeStorageBackendPools[P StorageBackendPool](
	ctx context.Context, config *CommonStorageDriverConfig, backendPools []P,
) ([]string, error) {
	fields := LogFields{"Method": "EncodeStorageBackendPools", "Type": config.StorageDriverName}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Debug(">>>> EncodeStorageBackendPools")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Debug("<<<< EncodeStorageBackendPools")

	if len(backendPools) == 0 {
		return nil, fmt.Errorf("failed to encode backend pools; no storage backend pools supplied")
	}

	encodedPools := make([]string, 0)
	for _, pool := range backendPools {
		encodedPool, err := utils.EncodeObjectToBase64String(pool)
		if err != nil {
			return nil, err
		}
		encodedPools = append(encodedPools, encodedPool)
	}
	return encodedPools, nil
}

// DecodeStorageBackendPools deserializes and decodes base64 encoded pools into driver-specific backend storage pools.
func DecodeStorageBackendPools[P StorageBackendPool](
	ctx context.Context, config *CommonStorageDriverConfig, encodedPools []string,
) ([]P, error) {
	fields := LogFields{"Method": "DecodeStorageBackendPools", "Type": config.StorageDriverName}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Debug(">>>> DecodeStorageBackendPools")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Debug("<<<< DecodeStorageBackendPools")

	if len(encodedPools) == 0 {
		return nil, fmt.Errorf("failed to decode backend pools; no encoded backend pools supplied")
	}

	backendPools := make([]P, 0)
	for _, pool := range encodedPools {
		var backendPool P
		err := utils.DecodeBase64StringToObject(pool, &backendPool)
		if err != nil {
			return nil, err
		}
		backendPools = append(backendPools, backendPool)
	}
	return backendPools, nil
}

// TODO (vhs): Extract the common bits and write two different functions for iSCSI and FC
func RemoveSCSIDeviceByPublishInfo(ctx context.Context, publishInfo *tridentmodels.VolumePublishInfo) {
	if publishInfo.SANType == sa.ISCSI {
		hostSessionMap := utils.IscsiUtils.GetISCSIHostSessionMapForTarget(ctx, publishInfo.IscsiTargetIQN)
		fields := LogFields{"targetIQN": publishInfo.IscsiTargetIQN}
		if len(hostSessionMap) == 0 {
			Logc(ctx).WithFields(fields).Error("Could not find host session for target IQN.")
			return
		}

		deviceInfo, err := utils.GetDeviceInfoForLUN(ctx, hostSessionMap, int(publishInfo.IscsiLunNumber), publishInfo.IscsiTargetIQN, false)
		if err != nil {
			Logc(ctx).WithError(err).WithFields(fields).Error("Error getting device info.")
		} else if deviceInfo == nil {
			Logc(ctx).WithFields(fields).Error("No device info found.")
		} else {
			// Inform the host about the device removal
			if _, err := utils.PrepareDeviceForRemoval(ctx, deviceInfo, publishInfo, nil, true, false); err != nil {
				Logc(ctx).WithError(err).WithFields(fields).Error("Error removing device.")
			}
		}
	} else if publishInfo.SANType == sa.FCP {
		hostSessionMap := utils.FcpUtils.GetFCPHostSessionMapForTarget(ctx, publishInfo.FCTargetWWNN)
		fields := LogFields{"targetIQN": publishInfo.FCTargetWWNN}
		if len(hostSessionMap) == 0 {
			Logc(ctx).WithFields(fields).Error("Could not find host session for target WWNN.")
			return
		}

		deviceInfo, err := utils.GetDeviceInfoForFCPLUN(ctx, nil, int(publishInfo.FCPLunNumber),
			publishInfo.FCTargetWWNN, false)
		if err != nil {
			Logc(ctx).WithError(err).WithFields(fields).Error("Error getting device info.")
		} else if deviceInfo == nil {
			Logc(ctx).WithFields(fields).Error("No device info found.")
		} else {
			// Inform the host about the device removal
			if _, err := utils.PrepareDeviceForRemoval(ctx, deviceInfo, publishInfo, nil, true, false); err != nil {
				Logc(ctx).WithError(err).WithFields(fields).Error("Error removing device.")
			}
		}
	}
}
