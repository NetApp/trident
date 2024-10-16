// Copyright 2024 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/spf13/afero"
	"go.uber.org/multierr"

	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

const (
	// Filesystem types
	fsXfs  = "xfs"
	fsExt3 = "ext3"
	fsExt4 = "ext4"
	fsRaw  = "raw"
)

const (
	// filesystem check (fsck) error codes
	fsckNoErrors                = 0
	fsckFsErrorsCorrected       = 1
	fsckFsErrorsCorrectedReboot = 2
	fsckFsErrorsUncorrected     = 4
	fsckOperationalError        = 8
	fsckSyntaxError             = 16
	fsckCancelledByUser         = 32
	fsckSharedLibError          = 128
)

const FormatOptionsSeparator = " "

var (
	osFs             = afero.NewOsFs()
	JsonReaderWriter = NewJSONReaderWriter()

	duringFormatVolume = fiji.Register("duringFormatVolume", "filesystem")
	duringRepairVolume = fiji.Register("duringRepairVolume", "filesystem")
)

type jsonReaderWriter struct{}

func NewJSONReaderWriter() models.JSONReaderWriter {
	return &jsonReaderWriter{}
}

// DFInfo data structure for wrapping the parsed output from the 'df' command
type DFInfo struct {
	Target string
	Source string
}

// GetDFOutput returns parsed DF output
func GetDFOutput(ctx context.Context) ([]DFInfo, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> filesystem.GetDFOutput")
	defer Logc(ctx).Debug("<<<< filesystem.GetDFOutput")

	var result []DFInfo
	out, err := command.Execute(ctx, "df", "--output=target,source")
	if err != nil {
		// df returns an error if there's a stale file handle that we can
		// safely ignore. There may be other reasons. Consider it a warning if
		// it printed anything to stdout.
		if len(out) == 0 {
			Logc(ctx).Error("Error encountered gathering df output.")
			return nil, err
		}
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, l := range lines {

		a := strings.Fields(l)
		if len(a) > 1 {
			result = append(result, DFInfo{
				Target: a[0],
				Source: a[1],
			})
		}
	}
	if len(result) > 1 {
		return result[1:], nil
	}
	return result, nil
}

// formatVolume creates a filesystem for the supplied device of the supplied type.
func formatVolume(ctx context.Context, device, fstype, options string) error {
	logFields := LogFields{"device": device, "fsType": fstype}
	Logc(ctx).WithFields(logFields).Debug(">>>> filesystem.formatVolume")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< filesystem.formatVolume")

	var err error
	var out []byte

	if err = duringFormatVolume.Inject(); err != nil {
		return err
	}

	var optionList []string
	if options != "" {
		optionList = strings.Split(options, FormatOptionsSeparator)
	}

	switch fstype {
	case fsXfs:
		optionList = append(optionList, "-f", device)
		out, err = command.Execute(ctx, "mkfs.xfs", optionList...)
	case fsExt3:
		optionList = append(optionList, "-F", device)
		out, err = command.Execute(ctx, "mkfs.ext3", optionList...)
	case fsExt4:
		optionList = append(optionList, "-F", device)
		out, err = command.Execute(ctx, "mkfs.ext4", optionList...)
	default:
		return fmt.Errorf("unsupported file system type: %s", fstype)
	}

	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"output": string(out),
			"device": device,
		}).Error("Formatting Failed.")
		return fmt.Errorf("error formatting device %v: %v", device, string(out))
	}

	return nil
}

// formatVolume creates a filesystem for the supplied device of the supplied type.
func formatVolumeRetry(ctx context.Context, device, fstype, options string) error {
	logFields := LogFields{"device": device, "fsType": fstype}
	Logc(ctx).WithFields(logFields).Debug(">>>> filesystem.formatVolumeRetry")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< filesystem.formatVolumeRetry")

	maxDuration := 30 * time.Second

	formatVolume := func() error {
		return formatVolume(ctx, device, fstype, options)
	}

	formatNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Format failed, retrying.")
	}

	formatBackoff := backoff.NewExponentialBackOff()
	formatBackoff.InitialInterval = 2 * time.Second
	formatBackoff.Multiplier = 2
	formatBackoff.RandomizationFactor = 0.1
	formatBackoff.MaxElapsedTime = maxDuration

	// Run the check/scan using an exponential backoff
	if err := backoff.RetryNotify(formatVolume, formatBackoff, formatNotify); err != nil {
		Logc(ctx).Warnf("Could not format device after %3.2f seconds.", maxDuration.Seconds())
		return err
	}

	Logc(ctx).WithFields(logFields).Info("Device formatted.")
	return nil
}

// repairVolume runs fsck on a volume. This is best-effort, does not return error.
func repairVolume(ctx context.Context, device, fstype string) {
	logFields := LogFields{"device": device, "fsType": fstype}
	Logc(ctx).WithFields(logFields).Debug(">>>> filesystem.repairVolume")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< filesystem.repairVolume")

	var err error

	switch fstype {
	case "xfs":
		break // fsck.xfs does nothing
	case "ext3":
		_, err = command.Execute(ctx, "fsck.ext3", "-p", device)
	case "ext4":
		_, err = command.Execute(ctx, "fsck.ext4", "-p", device)
	default:
		Logc(ctx).WithFields(logFields).Errorf("Unsupported file system type: %s.", fstype)
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {

			logFields["exitCode"] = exitErr.ExitCode()

			switch exitErr.ExitCode() {
			case fsckNoErrors:
				Logc(ctx).WithFields(logFields).Debug("No filesystem errors.")
			case fsckFsErrorsCorrected, fsckFsErrorsCorrectedReboot:
				Logc(ctx).WithFields(logFields).Info("Fixed filesystem errors.")
			case fsckOperationalError, fsckCancelledByUser, fsckSharedLibError:
				Logc(ctx).WithFields(logFields).Debug("Filesystem check errors.")
			case fsckFsErrorsUncorrected, fsckSyntaxError:
				Logc(ctx).WithError(err).WithFields(logFields).Error("Failed to repair filesystem errors.")
			}
		} else {
			Logc(ctx).WithError(err).WithFields(logFields).Error("Error executing fsck command.")
		}
	}
}

// ExpandFilesystemOnNode will expand the filesystem of an already expanded volume.
func ExpandFilesystemOnNode(
	ctx context.Context, publishInfo *models.VolumePublishInfo, devicePath, stagedTargetPath, fsType, mountOptions string,
) (int64, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	var err error
	expansionMountPoint := publishInfo.StagingMountpoint

	logFields := LogFields{
		"rawDevicePath":     devicePath,
		"stagedTargetPath":  stagedTargetPath,
		"mountOptions":      mountOptions,
		"filesystemType":    fsType,
		"stagingMountpoint": expansionMountPoint,
	}
	Logc(ctx).WithFields(logFields).Debug(">>>> filesystem.ExpandFilesystemOnNode")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< filesystem.ExpandFilesystemOnNode")

	if expansionMountPoint == "" {
		expansionMountPoint, err = mountFilesystemForResize(ctx, devicePath, stagedTargetPath, mountOptions)
		if err != nil {
			return 0, err
		}
		defer func() {
			err = multierr.Append(err, RemoveMountPoint(ctx, expansionMountPoint))
		}()
	}

	// Don't need to verify the filesystem type as the resize utilities will throw an error if the filesystem
	// is not the correct type.
	var size int64
	switch fsType {
	case "xfs":
		size, err = expandFilesystem(ctx, "xfs_growfs", expansionMountPoint, expansionMountPoint)
	case "ext3", "ext4":
		size, err = expandFilesystem(ctx, "resize2fs", devicePath, expansionMountPoint)
	default:
		err = fmt.Errorf("unsupported file system type: %s", fsType)
	}
	if err != nil {
		return 0, err
	}
	return size, err
}

func expandFilesystem(ctx context.Context, cmd, cmdArguments, tmpMountPoint string) (int64, error) {
	logFields := LogFields{
		"cmd":           cmd,
		"cmdArguments":  cmdArguments,
		"tmpMountPoint": tmpMountPoint,
	}
	Logc(ctx).WithFields(logFields).Debug(">>>> filesystem.expandFilesystem")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< filesystem.expandFilesystem")

	preExpandSize, err := getFilesystemSize(ctx, tmpMountPoint)
	if err != nil {
		return 0, err
	}
	_, err = command.Execute(ctx, cmd, cmdArguments)
	if err != nil {
		Logc(ctx).Errorf("Expanding filesystem failed; %s", err)
		return 0, err
	}

	postExpandSize, err := getFilesystemSize(ctx, tmpMountPoint)
	if err != nil {
		return 0, err
	}

	if postExpandSize == preExpandSize {
		Logc(ctx).Warnf("Failed to expand filesystem; size=%d", postExpandSize)
	}

	return postExpandSize, nil
}

// WriteJSONFile writes the contents of any type of struct to a file, with logging.
func (j jsonReaderWriter) WriteJSONFile(
	ctx context.Context, fileContents interface{}, filepath, fileDescription string,
) error {
	file, err := osFs.OpenFile(filepath, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	if err = json.NewEncoder(file).Encode(fileContents); err != nil {
		Logc(ctx).WithFields(LogFields{
			"filename": filepath,
			"error":    err.Error(),
		}).Error(fmt.Sprintf("Unable to write %s file.", fileDescription))
		return err
	}

	return nil
}

// ReadJSONFile reads a file at the specified path and deserializes its contents into the provided fileContents var.
// fileContents must be a pointer to a struct, not a pointer type!
func (j *jsonReaderWriter) ReadJSONFile(
	ctx context.Context, fileContents interface{}, filepath, fileDescription string,
) error {
	file, err := osFs.Open(filepath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			Logc(ctx).WithFields(LogFields{
				"filepath": filepath,
				"error":    err.Error(),
			}).Warningf("Could not find JSON file: %s.", filepath)
			return errors.NotFoundError(err.Error())
		}
		return err
	}
	defer func() { _ = file.Close() }()

	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}
	// We do not consider an empty file valid JSON.
	if fileInfo.Size() == 0 {
		return errors.InvalidJSONError("file was empty, which is not considered valid JSON")
	}

	err = json.NewDecoder(file).Decode(fileContents)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"filename": filepath,
			"error":    err.Error(),
		}).Error(fmt.Sprintf("Could not parse %s file.", fileDescription))

		e, _ := errors.AsInvalidJSONError(err)
		return e
	}

	return nil
}

// DeleteFile deletes the file at the provided path, and provides additional logging.
func DeleteFile(ctx context.Context, filepath, fileDescription string) (string, error) {
	if err := osFs.Remove(filepath); err != nil {
		logFields := LogFields{strings.ReplaceAll(fileDescription, " ", ""): filepath, "error": err}

		if os.IsNotExist(err) {
			Logc(ctx).WithFields(logFields).Warning(fmt.Sprintf("%s file does not exist.", Title(fileDescription)))
			return "", nil
		} else {
			Logc(ctx).WithFields(logFields).Error(fmt.Sprintf("Removing %s file failed.", fileDescription))
			return "", err
		}
	}

	return filepath, nil
}
