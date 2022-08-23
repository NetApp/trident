// Copyright 2019 NetApp, Inc. All Rights Reserved.

package storagedrivers

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	trident "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/utils"
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
		Logc(ctx).WithFields(log.Fields{
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
	Logc(ctx).WithFields(log.Fields{
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

	Logc(ctx).WithFields(log.Fields{
		"limitVolumeSize":    limitVolumeSize,
		"volumeSizeLimit":    volumeSizeLimit,
		"requestedSizeBytes": requestedSize,
	}).Debugf("Comparing limits")

	if requestedSize > float64(volumeSizeLimit) {
		return true, volumeSizeLimit, utils.UnsupportedCapacityRangeError(fmt.Errorf(
			"requested size: %1.f > the size limit: %d", requestedSize, volumeSizeLimit))
	}

	return true, volumeSizeLimit, nil
}

// CheckMinVolumeSize returns UnsupportedCapacityRangeError if the requested volume size is less than the minimum
// volume size
func CheckMinVolumeSize(requestedSizeBytes, minVolumeSizeBytes uint64) error {
	if requestedSizeBytes < minVolumeSizeBytes {
		return utils.UnsupportedCapacityRangeError(fmt.Errorf("requested volume size ("+
			"%d bytes) is too small; the minimum volume size is %d bytes",
			requestedSizeBytes, minVolumeSizeBytes))
	}
	return nil
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
		Logc(ctx).WithFields(log.Fields{
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
