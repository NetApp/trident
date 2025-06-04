// Copyright 2025 NetApp, Inc. All Rights Reserved.

package filesystem

//go:generate mockgen -destination=../../mocks/mock_utils/mock_filesystem/mock_mount_client.go github.com/netapp/trident/utils/filesystem Mount
//go:generate mockgen -destination=../../mocks/mock_utils/mock_filesystem/mock_filesystem_client.go github.com/netapp/trident/utils/filesystem Filesystem

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/spf13/afero"
	"go.uber.org/multierr"

	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/convert"
	tridentexec "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/models"
)

const (
	// Filesystem types
	Xfs           = "xfs"
	Ext3          = "ext3"
	Ext4          = "ext4"
	Raw           = "raw"
	UnknownFstype = "<unknown>"
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

	FormatOptionsSeparator = " "
)

var (
	formatVolumeMaxRetryDuration = 30 * time.Second

	duringFormatVolume = fiji.Register("duringFormatVolume", "filesystem")
	duringRepairVolume = fiji.Register("duringRepairVolume", "filesystem")
)

type Filesystem interface {
	GetDFOutput(ctx context.Context) ([]models.DFInfo, error)
	FormatVolume(ctx context.Context, device, fstype, options string) error
	RepairVolume(ctx context.Context, device, fstype string)
	ExpandFilesystemOnNode(
		ctx context.Context, publishInfo *models.VolumePublishInfo, devicePath, stagedTargetPath, fsType, mountOptions string,
	) (int64, error)
	DeleteFile(ctx context.Context, filepath, fileDescription string) (string, error)
	GetFilesystemStats(
		ctx context.Context, path string,
	) (available, capacity, usage, inodes, inodesFree, inodesUsed int64, err error)
	GetUnmountPath(ctx context.Context, trackingInfo *models.VolumeTrackingInfo) (string, error)
	ScanFile(filename string) ([]byte, error)
	ScanDir(path string) ([]os.FileInfo, error)
}

type Mount interface {
	MountFilesystemForResize(ctx context.Context, devicePath, stagedTargetPath, mountOptions string) (string, error)
	RemoveMountPoint(ctx context.Context, mountPointPath string) error
}

type FSClient struct {
	command     tridentexec.Command
	osFs        afero.Fs
	mountClient Mount
}

func New(mount Mount) *FSClient {
	return NewDetailed(tridentexec.NewCommand(), afero.NewOsFs(), mount)
}

func NewDetailed(command tridentexec.Command, osFs afero.Fs, mount Mount) *FSClient {
	return &FSClient{
		command:     command,
		osFs:        osFs,
		mountClient: mount,
	}
}

// GetDFOutput returns parsed DF output
func (f *FSClient) GetDFOutput(ctx context.Context) ([]models.DFInfo, error) {
	GenerateRequestContextForLayer(ctx, LogLayerUtils)

	Logc(ctx).Debug(">>>> filesystem.GetDFOutput")
	defer Logc(ctx).Debug("<<<< filesystem.GetDFOutput")

	var result []models.DFInfo
	out, err := f.command.Execute(ctx, "df", "--output=target,source")
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
			result = append(result, models.DFInfo{
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

// FormatVolume creates a filesystem for the supplied device of the supplied type.
func (f *FSClient) FormatVolume(ctx context.Context, device, fstype, options string) error {
	logFields := LogFields{"device": device, "fsType": fstype, "options": options}
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
	case Xfs:
		optionList = append(optionList, "-f", device)
		out, err = f.command.Execute(ctx, "mkfs.xfs", optionList...)
	case Ext3:
		optionList = append(optionList, "-F", device)
		out, err = f.command.Execute(ctx, "mkfs.ext3", optionList...)
	case Ext4:
		optionList = append(optionList, "-F", device)
		out, err = f.command.Execute(ctx, "mkfs.ext4", optionList...)
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
func (f *FSClient) formatVolumeRetry(ctx context.Context, device, fstype, options string) error {
	logFields := LogFields{"device": device, "fsType": fstype, "options": options}
	Logc(ctx).WithFields(logFields).Debug(">>>> filesystem.formatVolumeRetry")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< filesystem.formatVolumeRetry")

	formatVolume := func() error {
		return f.FormatVolume(ctx, device, fstype, options)
	}

	formatNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Format failed, retrying.")
	}

	maxDuration := formatVolumeMaxRetryDuration

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

// RepairVolume runs fsck on a volume. This is best-effort, does not return error.
func (f *FSClient) RepairVolume(ctx context.Context, device, fstype string) {
	logFields := LogFields{"device": device, "fsType": fstype}
	Logc(ctx).WithFields(logFields).Debug(">>>> filesystem.repairVolume")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< filesystem.repairVolume")

	// Note: FIJI error injection will have no effect due to no error return. Panic injection is still valid.
	if err := duringRepairVolume.Inject(); err != nil {
		Logc(ctx).WithError(err).Debug("FIJI error in repairVolume has no effect, no error return.")
	}

	var err error

	switch fstype {
	case "xfs":
		break // fsck.xfs does nothing
	case "ext3":
		_, err = f.command.Execute(ctx, "fsck.ext3", "-p", device)
	case "ext4":
		_, err = f.command.Execute(ctx, "fsck.ext4", "-p", device)
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
func (f *FSClient) ExpandFilesystemOnNode(
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
		expansionMountPoint, err = f.mountClient.MountFilesystemForResize(ctx, devicePath, stagedTargetPath,
			mountOptions)
		if err != nil {
			return 0, err
		}
		defer func() {
			err = multierr.Append(err, f.mountClient.RemoveMountPoint(ctx, expansionMountPoint))
		}()
	}

	// Don't need to verify the filesystem type as the resize utilities will throw an error if the filesystem
	// is not the correct type.
	var size int64
	switch fsType {
	case "xfs":
		size, err = f.expandFilesystem(ctx, "xfs_growfs", expansionMountPoint, expansionMountPoint)
	case "ext3", "ext4":
		size, err = f.expandFilesystem(ctx, "resize2fs", devicePath, expansionMountPoint)
	default:
		err = fmt.Errorf("unsupported file system type: %s", fsType)
	}
	if err != nil {
		return 0, err
	}
	return size, err
}

func (f *FSClient) expandFilesystem(ctx context.Context, cmd, cmdArguments, tmpMountPoint string) (int64, error) {
	logFields := LogFields{
		"cmd":           cmd,
		"cmdArguments":  cmdArguments,
		"tmpMountPoint": tmpMountPoint,
	}
	Logc(ctx).WithFields(logFields).Debug(">>>> filesystem.expandFilesystem")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< filesystem.expandFilesystem")

	preExpandSize, err := f.getFilesystemSize(ctx, tmpMountPoint)
	if err != nil {
		return 0, err
	}
	_, err = f.command.Execute(ctx, cmd, cmdArguments)
	if err != nil {
		Logc(ctx).Errorf("Expanding filesystem failed; %s", err)
		return 0, err
	}

	postExpandSize, err := f.getFilesystemSize(ctx, tmpMountPoint)
	if err != nil {
		return 0, err
	}

	if postExpandSize == preExpandSize {
		Logc(ctx).Warnf("Failed to expand filesystem; size=%d", postExpandSize)
	}

	return postExpandSize, nil
}

// DeleteFile deletes the file at the provided path, and provides additional logging.
func (f *FSClient) DeleteFile(ctx context.Context, filepath, fileDescription string) (string, error) {
	if err := f.osFs.Remove(filepath); err != nil {
		logFields := LogFields{strings.ReplaceAll(fileDescription, " ", ""): filepath, "error": err}

		title := convert.ToTitle(fileDescription)
		if os.IsNotExist(err) {
			Logc(ctx).WithFields(logFields).Warning(fmt.Sprintf("%s file does not exist.", title))
			return "", nil
		} else {
			Logc(ctx).WithFields(logFields).Error(fmt.Sprintf("Removing %s file failed.", title))
			return "", err
		}
	}

	return filepath, nil
}

func (f *FSClient) ScanFile(filename string) ([]byte, error) {
	file, err := f.osFs.Open(filename)
	if err != nil {
		fmt.Println("Failed to open file:", err)
		return nil, err
	}
	defer file.Close()

	data, err := afero.ReadAll(file)
	if err != nil {
		fmt.Println("Failed to read file:", err)
		return nil, err
	}
	return data, nil
}

func (f *FSClient) ScanDir(path string) ([]os.FileInfo, error) {
	dirEntries, err := afero.ReadDir(f.osFs, path)
	if err != nil {
		fmt.Println("Failed to read  directory:", err)
		return nil, err
	}
	return dirEntries, nil
}
