// Copyright 2024 NetApp, Inc. All Rights Reserved.

package mount

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/net/context"

	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils/errors"
	tridentexec "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/mount/filepathwrapper"
	"github.com/netapp/trident/utils/mount/oswrapper"
)

// Part of the published path for raw devices
const (
	rawDevicePublishPath = "plugins/kubernetes.io/csi/volumeDevices/publish/pvc-"
)

var (
	// Regex to identify published path for mounted devices
	pvMountpointRegex = regexp.MustCompile(`^(.*pods)(.*volumes)(.*pvc-).*$`)

	beforeMount   = fiji.Register("beforeMount", "mount_linux")
	beforeUnmount = fiji.Register("beforeUnmount", "mount_linux")
)

type LinuxClient struct {
	os       oswrapper.OS
	filepath filepathwrapper.FilePath
	command  tridentexec.Command
}

var _ mountOS = &LinuxClient{}

func newOsSpecificClient() (*LinuxClient, error) {
	return newOsSpecificClientDetailed(oswrapper.New(), filepathwrapper.New(), tridentexec.NewCommand()), nil
}

func newOsSpecificClientDetailed(os oswrapper.OS, filepath filepathwrapper.FilePath,
	command tridentexec.Command,
) *LinuxClient {
	return &LinuxClient{
		os:       os,
		filepath: filepath,
		command:  command,
	}
}

// IsLikelyNotMountPoint uses heuristics to determine if a directory is not a mountpoint.
// It should return ErrNotExist when the directory does not exist.
// IsLikelyNotMountPoint does NOT properly detect all mountpoint types
// most notably Linux bind mounts and symbolic links. For callers that do not
// care about such situations, this is a faster alternative to scanning the list of mounts.
// A return value of false means the directory is definitely a mount point.
// A return value of true means it's not a mount this function knows how to find,
// but it could still be a mount point.
func (client *LinuxClient) IsLikelyNotMountPoint(ctx context.Context, mountpoint string) (bool, error) {
	fields := LogFields{"mountpoint": mountpoint}
	Logc(ctx).WithFields(fields).Debug(">>>> mount_linux.IsLikelyNotMountPoint")
	defer Logc(ctx).WithFields(fields).Debug("<<<< mount_linux.IsLikelyNotMountPoint")

	stat, err := client.os.Stat(mountpoint)
	if err != nil {
		return true, err
	}

	rootStat, err := client.os.Lstat(client.filepath.Dir(strings.TrimSuffix(mountpoint, "/")))
	if err != nil {
		return true, err
	}
	// If the directory has a different device as parent, then it is a mountpoint.
	if stat.Sys().(*syscall.Stat_t).Dev != rootStat.Sys().(*syscall.Stat_t).Dev {
		Logc(ctx).WithFields(fields).Debug("Path is a mountpoint.")
		return false, nil
	}

	Logc(ctx).WithFields(fields).Debug("Path is likely not a mountpoint.")
	return true, nil
}

// IsMounted verifies if the supplied device is attached at the supplied location.
// If no source device is specified, any existing mount with the specified mountpoint returns true.
// If no mountpoint is specified, any existing mount with the specified device returns true.
func (client *LinuxClient) IsMounted(ctx context.Context, sourceDevice, mountpoint, mountOptions string) (bool, error) {
	logFields := LogFields{"source": sourceDevice, "target": mountpoint}
	Logc(ctx).WithFields(logFields).Debug(">>>> mount_linux.IsMounted")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< mount_linux.IsMounted")

	tempSourceDevice := strings.TrimPrefix(sourceDevice, "/dev/")

	// Ensure at least one arg was specified
	if tempSourceDevice == "" && mountpoint == "" {
		return false, errors.New("no device or mountpoint specified")
	}

	// Get device path if source is already linked
	devicePath, err := client.filepath.EvalSymlinks(sourceDevice)
	if err != nil {
		Logc(ctx).WithError(err).WithFields(logFields).Debug("Could not resolve device symlink")
		// Unset device path to try and find it from mountinfo
		devicePath = ""
	}

	devicePath = strings.TrimPrefix(devicePath, "/dev/")
	sourceDevice = tempSourceDevice

	mountInfo, err := client.ReadMountProcInfo(ctx, mountpoint, sourceDevice, devicePath)
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Debug("Checking mounts failed.")
		return false, err
	}

	if mountInfo == nil {
		Logc(ctx).WithFields(logFields).Debug("Mount information not found.")
		return false, nil
	}

	if err = checkMountOptions(ctx, mountInfo, mountOptions); err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Warning("Checking mount options failed.")
	}

	Logc(ctx).WithFields(logFields).Debug("Mount information found.")
	return true, nil
}

// PVMountpointMappings identifies devices corresponding to published paths
func (client *LinuxClient) PVMountpointMappings(ctx context.Context) (map[string]string, error) {
	Logc(ctx).Debug(">>>> mount_linux.PVMountpointMappings")
	defer Logc(ctx).Debug("<<<< mount_linux.PVMountpointMappings")

	mappings := make(map[string]string)

	// Read the system mounts
	procSelfMountinfo, err := client.ListProcMountinfo()
	if err != nil {
		Logc(ctx).Errorf("checking mounts failed; %s", err)
		return nil, fmt.Errorf("checking mounts failed; %s", err)
	}

	// Check each mount for K8s-based mounts
	for _, procMount := range procSelfMountinfo {
		if pvMountpointRegex.MatchString(procMount.MountPoint) ||
			strings.Contains(procMount.MountPoint, rawDevicePublishPath) {

			// In case of raw block volumes device is at the `procMount.Root`
			procSourceDevice := strings.TrimPrefix(procMount.Root, "/")

			if strings.HasPrefix(procMount.MountSource, "/dev/") {
				procSourceDevice, err = client.filepath.EvalSymlinks(procMount.MountSource)
				if err != nil {
					Logc(ctx).Error(err)
					continue
				}
				procSourceDevice = strings.TrimPrefix(procSourceDevice, "/dev/")
			}

			if procSourceDevice != "" {
				mappings[procMount.MountPoint] = "/dev/" + procSourceDevice
			}
		}
	}

	return mappings, nil
}

// MountNFSPath attaches the supplied NFS share at the supplied location with options.
func (client *LinuxClient) MountNFSPath(ctx context.Context, exportPath, mountpoint, options string) error {
	Logc(ctx).WithFields(LogFields{
		"exportPath": exportPath,
		"mountpoint": mountpoint,
		"options":    options,
	}).Debug(">>>> mount_linux.mountNFSPath")
	defer Logc(ctx).Debug("<<<< mount_linux.mountNFSPath")

	mountCommand := "mount.nfs"
	nfsVersRegex := regexp.MustCompile(`nfsvers\s*=\s*4(\.\d+)?`)
	if nfsVersRegex.MatchString(options) {
		mountCommand = "mount.nfs4"
	}

	args := []string{exportPath, mountpoint}
	if len(options) > 0 {
		sanitizedOptions := strings.TrimPrefix(options, "-o ")
		args = []string{"-o", sanitizedOptions, exportPath, mountpoint}
	}

	// Create the mount point dir if necessary
	if _, err := client.command.Execute(ctx, "mkdir", "-p", mountpoint); err != nil {
		Logc(ctx).WithField("error", err).Warning("Mkdir failed.")
	}

	if out, err := client.command.Execute(ctx, mountCommand, args...); err != nil {
		Logc(ctx).WithField("output", string(out)).Debug("Mount failed.")
		return fmt.Errorf("error mounting NFS volume %v on mountpoint %v: %v", exportPath, mountpoint, err)
	}

	return nil
}

// UmountAndRemoveTemporaryMountPoint unmounts and removes the temporaryMountDir
func (client *LinuxClient) UmountAndRemoveTemporaryMountPoint(ctx context.Context, mountPath string) error {
	Logc(ctx).Debug(">>>> mount_linux.UmountAndRemoveTemporaryMountPoint")
	defer Logc(ctx).Debug("<<<< mount_linux.UmountAndRemoveTemporaryMountPoint")

	return client.UmountAndRemoveMountPoint(ctx, path.Join(mountPath, temporaryMountDir))
}

// UmountAndRemoveMountPoint unmounts and removes the mountPoint
func (client *LinuxClient) UmountAndRemoveMountPoint(ctx context.Context, mountPoint string) error {
	Logc(ctx).Debug(">>>> mount_linux.UmountAndRemoveMountPoint")
	defer Logc(ctx).Debug("<<<< mount_linux.UmountAndRemoveMountPoint")

	// Delete the mount point if it exists.
	if _, err := client.os.Stat(mountPoint); err == nil {
		if err = client.RemoveMountPoint(ctx, mountPoint); err != nil {
			return fmt.Errorf("failed to remove directory %s; %s", mountPoint, err)
		}
	} else if !os.IsNotExist(err) {
		Logc(ctx).WithField("mountPoint", mountPoint).Errorf("Can't determine if mount point dir path exists; %s",
			err)
		return fmt.Errorf("can't determine if mount point dir path %s exists; %s", mountPoint, err)
	}

	return nil
}

func (client *LinuxClient) IsNFSShareMounted(ctx context.Context, exportPath, mountpoint string) (bool, error) {
	fields := LogFields{
		"exportPath": exportPath,
		"target":     mountpoint,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> mount_linux.IsNFSShareMounted")
	defer Logc(ctx).WithFields(fields).Debug("<<<< mount_linux.IsNFSShareMounted")

	mounts, err := client.GetSelfMountInfo(ctx)
	if err != nil {
		return false, err
	}

	for _, mount := range mounts {

		Logc(ctx).Tracef("Mount: %+v", mount)

		if mount.MountSource == exportPath && mount.MountPoint == mountpoint {
			Logc(ctx).Debug("NFS Share is mounted.")
			return true, nil
		}
	}

	Logc(ctx).Debug("NFS Share is not mounted.")
	return false, nil
}

// GetSelfMountInfo returns the list of mounts found in /proc/self/mountinfo
func (client *LinuxClient) GetSelfMountInfo(ctx context.Context) ([]models.MountInfo, error) {
	Logc(ctx).Debug(">>>> mount_linux.GetSelfMountInfo")
	defer Logc(ctx).Debug("<<<< mount_linux.GetSelfMountInfo")

	return client.ListProcMountinfo()
}

// GetHostMountInfo returns the list of mounts found in /proc/1/mountinfo
func (client *LinuxClient) GetHostMountInfo(ctx context.Context) ([]models.MountInfo, error) {
	Logc(ctx).Debug(">>>> mount.GetHostMountInfo")
	defer Logc(ctx).Debug("<<<< mount.GetHostMountInfo")

	return client.ListProcHostMountinfo()
}

// MountDevice attaches the supplied device at the supplied location.  Use this for iSCSI devices.
func (client *LinuxClient) MountDevice(ctx context.Context, device, mountpoint, options string,
	isMountPointFile bool,
) error {
	Logc(ctx).WithFields(LogFields{
		"device":     device,
		"mountpoint": mountpoint,
		"options":    options,
	}).Debug(">>>> mount_linux.MountDevice")
	defer Logc(ctx).Debug("<<<< mount_linux.MountDevice")

	// Build the command
	var args []string
	if len(options) > 0 {
		sanitizedOptions := strings.TrimPrefix(options, "-o ")
		args = []string{"-o", sanitizedOptions, device, mountpoint}
	} else {
		args = []string{device, mountpoint}
	}

	mounted, _ := client.IsMounted(ctx, device, mountpoint, options)
	mountPointExists := client.PathExists(mountpoint)

	Logc(ctx).Debugf("Already mounted: %v, mount point exists: %v", mounted, mountPointExists)

	if !mountPointExists {
		client.createMountPoint(ctx, mountpoint, isMountPointFile)
	}

	if err := beforeMount.Inject(); err != nil {
		return err
	}

	if !mounted {
		if out, err := client.command.Execute(ctx, "mount", args...); err != nil {
			Logc(ctx).WithError(fmt.Errorf("exit error: %v, mount command output: %s", err, string(out))).Error("Mount failed.")
			return fmt.Errorf("mount failed: %v, output: %s", err, out)
		}
	}

	return nil
}

func (client *LinuxClient) createMountPoint(ctx context.Context, mountPoint string, isMountPointFile bool) {
	if isMountPointFile {
		if err := client.EnsureFileExists(ctx, mountPoint); err != nil {
			Logc(ctx).WithField("error", err).Warning("File check failed.")
		}
		return
	}

	if err := client.EnsureDirExists(ctx, mountPoint); err != nil {
		Logc(ctx).WithField("error", err).Warning("Mkdir failed.")
	}
}

func (client *LinuxClient) PathExists(path string) bool {
	_, err := client.os.Stat(path)
	return err == nil
}

func (client *LinuxClient) EnsureFileExists(ctx context.Context, path string) error {
	fields := LogFields{"path": path}

	if info, err := client.os.Stat(path); err == nil {
		if info.IsDir() {
			Logc(ctx).WithFields(fields).Error("Path exists but is a directory")
			return errors.New("path exists but is a directory")
		}
		return nil

	} else if !os.IsNotExist(err) {
		Logc(ctx).WithFields(fields).Errorf("Can't determine if file exists; %s", err)
		return fmt.Errorf("can't determine if file %s exists; %s", path, err)
	}

	file, err := client.os.OpenFile(path, os.O_CREATE|os.O_TRUNC, 0o600)
	if nil != err {
		Logc(ctx).WithFields(fields).Errorf("OpenFile failed; %s", err)
		return fmt.Errorf("failed to create file %s; %s", path, err)
	}

	defer func() {
		_ = file.Close()
	}()

	return nil
}

func (client *LinuxClient) EnsureDirExists(ctx context.Context, path string) error {
	fields := LogFields{"path": path}

	Logc(ctx).WithFields(fields).Debug(">>>> EnsureDirExists")
	defer Logc(ctx).WithFields(fields).Debug("<<<< EnsureDirExists")

	if info, err := client.os.Stat(path); err == nil {
		if !info.IsDir() {
			Logc(ctx).WithFields(fields).Error("Path exists but is not a directory")
			return fmt.Errorf("path exists but is not a directory: %s", path)
		}
		return nil

	} else if !os.IsNotExist(err) {
		Logc(ctx).WithFields(fields).Errorf("Can't determine if directory exists; %s", err)
		return fmt.Errorf("can't determine if directory %s exists; %s", path, err)
	}

	if err := client.os.MkdirAll(path, 0o755); err != nil {
		Logc(ctx).WithFields(fields).Errorf("Mkdir failed; %s", err)
		return fmt.Errorf("failed to mkdir %s; %s", path, err)
	}

	return nil
}

// RemountDevice remounts the mountpoint with supplied mount options.
func (client *LinuxClient) RemountDevice(ctx context.Context, mountpoint, options string) error {
	Logc(ctx).WithFields(LogFields{
		"mountpoint": mountpoint,
		"options":    options,
	}).Debug(">>>> mount_linux.RemountDevice")
	defer Logc(ctx).Debug("<<<< mount_linux.RemountDevice")

	// Build the command
	var args []string
	if len(options) > 0 {
		sanitizedOptions := strings.TrimPrefix(options, "-o ")
		args = []string{"-o", sanitizedOptions, mountpoint}
	} else {
		args = []string{mountpoint}
	}

	if _, err := client.command.Execute(ctx, "mount", args...); err != nil {
		Logc(ctx).WithField("error", err).Error("Remounting failed.")
	}

	return nil
}

// Umount detaches from the supplied location.
func (client *LinuxClient) Umount(ctx context.Context, mountpoint string) error {
	Logc(ctx).WithField("mountpoint", mountpoint).Debug(">>>> mount_linux.Umount")
	defer Logc(ctx).Debug("<<<< mount_linux.Umount")

	if err := beforeUnmount.Inject(); err != nil {
		return err
	}

	out, err := client.command.ExecuteWithTimeout(ctx, "umount", umountTimeout, true, mountpoint)
	if err == nil || strings.Contains(string(out), umountNotMounted) {
		return nil
	}

	if !errors.IsTimeoutError(err) {
		return err
	}

	Logc(ctx).WithField("error", err).Error("Umount failed, attempting to force umount")

	out, err = client.command.ExecuteWithTimeout(ctx, "umount", umountTimeout, true, mountpoint, "-f")
	if strings.Contains(string(out), umountNotMounted) {
		return nil
	}

	return err
}

// RemoveMountPoint attempts to unmount and remove the directory of the mountPointPath.  This method should
// be idempotent and safe to call again if it fails the first time.
func (client *LinuxClient) RemoveMountPoint(ctx context.Context, mountPointPath string) error {
	Logc(ctx).Debug(">>>> mount_linux.RemoveMountPoint")
	defer Logc(ctx).Debug("<<<< mount_linux.RemoveMountPoint")

	// If the directory does not exist, return nil.
	if _, err := client.os.Stat(mountPointPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("could not check for mount path %s; %v", mountPointPath, err)
	}

	// Unmount the path.  Umount() returns nil if the path exists but is not a mount.
	if err := client.Umount(ctx, mountPointPath); err != nil {
		Logc(ctx).WithField("mountPointPath", mountPointPath).Errorf("Umount failed; %s", err)
		return err
	}

	// Delete the mount path after it is unmounted (or confirmed to not be a mount point).
	if err := client.os.Remove(mountPointPath); err != nil {
		Logc(ctx).WithField("mountPointPath", mountPointPath).Errorf("Remove dir failed; %s", err)
		return fmt.Errorf("failed to remove dir %s; %s", mountPointPath, err)
	}

	return nil
}

// MountSMBPath is a dummy added for compilation on non-windows platform.
func (client *LinuxClient) MountSMBPath(ctx context.Context, exportPath, mountpoint, username, password string) error {
	Logc(ctx).Debug(">>>> mount_linux.MountSMBPath")
	defer Logc(ctx).Debug("<<<< mount_linux.MountSMBPath")
	return errors.UnsupportedError("mountSMBPath is not supported on non-windows platform")
}

// UmountSMBPath is a dummy added for compilation on non-windows platform.
func (client *LinuxClient) UmountSMBPath(ctx context.Context, mappingPath, target string) (err error) {
	Logc(ctx).Debug(">>>> mount_linux.UmountSMBPath")
	defer Logc(ctx).Debug("<<<< mount_linux.UmountSMBPath")
	return errors.UnsupportedError("UmountSMBPath is not supported on non-windows platform")
}

// WindowsBindMount is a dummy added for compilation on non-windows platform.
func (client *LinuxClient) WindowsBindMount(ctx context.Context, source, target string, options []string) (err error) {
	Logc(ctx).Debug(">>>> mount_linux.WindowsBindMount")
	defer Logc(ctx).Debug("<<<< mount_linux.WindowsBindMount")
	return errors.UnsupportedError("WindowsBindMount is not supported on non-windows platform")
}

// IsCompatible checks for compatibility of protocol and platform
func (client *LinuxClient) IsCompatible(ctx context.Context, protocol string) error {
	Logc(ctx).Debug(">>>> mount_linux.IsCompatible")
	defer Logc(ctx).Debug("<<<< mount_linux.IsCompatible")

	if protocol != sa.NFS {
		return fmt.Errorf("mounting %s volume is not supported on linux", protocol)
	} else {
		return nil
	}
}

// ListProcMountinfo (Available since Linux 2.6.26) lists information about mount points
// in the process's mount namespace. Ref: http://man7.org/linux/man-pages/man5/proc.5.html
// for /proc/[pid]/mountinfo
func (client *LinuxClient) ListProcMountinfo() ([]models.MountInfo, error) {
	return client.ListMountinfo("/proc/self/mountinfo")
}

func (client *LinuxClient) ListProcHostMountinfo() ([]models.MountInfo, error) {
	return client.ListMountinfo("/proc/1/mountinfo")
}

func (client *LinuxClient) ListMountinfo(mountFilePath string) ([]models.MountInfo, error) {
	content, err := client.ConsistentRead(mountFilePath, maxListTries)
	if err != nil {
		return nil, err
	}
	return parseProcMountInfo(content)
}

// ConsistentRead repeatedly reads a file until it gets the same content twice.
// This is useful when reading files in /proc that are larger than page size
// and kernel may modify them between individual read() syscalls.
func (client *LinuxClient) ConsistentRead(filename string, attempts int) ([]byte, error) {
	oldContent, err := client.os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	for i := 0; i < attempts; i++ {
		newContent, err := client.os.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		if bytes.Equal(oldContent, newContent) {
			return newContent, nil
		}
		// Files are different, continue reading
		oldContent = newContent
	}
	return nil, fmt.Errorf("could not get consistent content of %s after %d attempts", filename, attempts)
}

// parseProcMountInfo parses the output of /proc/self/mountinfo file into a slice of MountInfo struct
func parseProcMountInfo(content []byte) ([]models.MountInfo, error) {
	out := make([]models.MountInfo, 0)
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if line == "" {
			// The last split() item is empty string following the last \n
			continue
		}

		mp, err := parseProcMount(line)
		if err != nil {
			return nil, err
		}

		if mp == nil {
			// For the case, where err == nil && mp == nil, this can happen when root is marked as deleted.
			continue
		}

		out = append(out, *mp)
	}
	return out, nil
}

func (client *LinuxClient) ListProcMounts(mountFilePath string) ([]models.MountPoint, error) {
	content, err := client.ConsistentRead(mountFilePath, maxListTries)
	if err != nil {
		return nil, err
	}
	return parseProcMounts(content)
}

func parseProcMounts(content []byte) ([]models.MountPoint, error) {
	out := make([]models.MountPoint, 0)
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if line == "" {
			// the last split() item is empty string following the last \n
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != expectedNumProcMntFieldsPerLine {
			return nil, fmt.Errorf("wrong number of fields (expected %d, got %d): %s",
				expectedNumProcMntFieldsPerLine, len(fields), line)
		}

		mp := models.MountPoint{
			Device: fields[0],
			Path:   fields[1],
			Type:   fields[2],
			Opts:   strings.Split(fields[3], ","),
		}

		freq, err := strconv.Atoi(fields[4])
		if err != nil {
			return nil, err
		}
		mp.Freq = freq

		pass, err := strconv.Atoi(fields[5])
		if err != nil {
			return nil, err
		}
		mp.Pass = pass

		out = append(out, mp)
	}
	return out, nil
}

// ReadMountProcInfo tries the consistent read for the given mount information from the "/proc/self/mountinfo"
func (client *LinuxClient) ReadMountProcInfo(ctx context.Context, mountpoint, sourceDevice, devicePath string) (*models.MountInfo, error) {
	return client.ConsistentReadMount(ctx, "/proc/self/mountinfo", mountpoint, sourceDevice, devicePath, maxListTries)
}

// ConsistentReadMount verifies whether a specific mountpoint and source device consistently appear in the mount file.
// It reads the file up to maxAttempts times, returning parsedEntry if the same matching entry is found in two consecutive reads.
// Returns nil and an error if either the entry was present but not consistently found within the allowed attempts or there was an error.
// Returns nil and nil if the entry was never found.
func (client *LinuxClient) ConsistentReadMount(ctx context.Context, filename, mountpoint, sourceDevice, devicePath string, maxAttempts int) (*models.MountInfo, error) {
	logFields := LogFields{
		"filename":    filename,
		"mountpoint":  mountpoint,
		"maxAttempts": maxAttempts,
	}

	Logc(ctx).WithFields(logFields).Debug(">>>> ConsistentReadMount")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< ConsistentReadMount")

	if maxAttempts < 1 {
		Logc(ctx).WithFields(logFields).Errorf("maxAttempts has to be equal or greater than 1, currently set to :%d", maxAttempts)
		return nil, fmt.Errorf("maxAttempts has to be equal or greater than 1, currently set to :%d", maxAttempts)
	}

	var (
		previousMountEntry string
		entryFound         = false
	)

	// First iteration is to actually read the file for the first time.
	// And maxAttempts are made for the comparison thereafter.
	// Hence, +1
	for i := 0; i < (maxAttempts + 1); i++ {
		select {
		case <-ctx.Done():
			Logc(ctx).WithError(ctx.Err()).Error("Exiting early as the context has been cancelled or timed out.")
			return nil, ctx.Err()
		default:
		}

		var (
			mountFound   bool
			currentMount string
			parsedEntry  *models.MountInfo
		)

		content, err := client.os.ReadFile(filename)
		if err != nil {
			Logc(ctx).WithError(err).Error("Failed to read mount file.")
			return nil, err
		}

		// Ranging over each entry.
		lines := strings.Split(string(content), "\n")
		for _, currentMount = range lines {
			if currentMount == "" {
				// The last split() item is empty string following the last \n
				continue
			}

			// Parsing the line that we retrieved to models.MountInfo struct
			parsedEntry, err = parseProcMount(currentMount)
			if err != nil || parsedEntry == nil {
				// We'll keep continuing ranging over remaining lines.
				Logc(ctx).WithField("line", currentMount).WithError(err).Warn("Unable to parse the entry.")
				continue
			}

			// Comparing the parsedEntry with the information that we have.
			mountFound, err = client.compareMount(ctx, mountpoint, sourceDevice, devicePath, parsedEntry)
			if err != nil {
				Logc(ctx).WithField("line", currentMount).WithError(err).Warn("Unable to compare the mount.")
				continue
			}

			if mountFound {
				break
			}
		}

		if mountFound {
			entryFound = true
			if previousMountEntry == currentMount {
				return parsedEntry, nil
			}
		}

		// Changing previousMountEntry every iteration ensures, that when we're comparing,
		// it is between consecutive reads.
		previousMountEntry = currentMount
	}

	if entryFound {
		return nil, fmt.Errorf("could not find consistent mount entry in %s for mountpoint %s after %d attempts", filename, mountpoint, maxAttempts)
	}

	return nil, nil
}

// parseProcMount parses single line/entry of the /proc/self/mountinfo file.
func parseProcMount(line string) (*models.MountInfo, error) {
	// Ex: 26 29 0:5 / /dev rw,nosuid,relatime shared:2 - devtmpfs udev rw,size=4027860k,nr_inodes=1006965,mode=755,inode64

	fields := strings.Fields(line)
	numFields := len(fields)
	if numFields < minNumProcSelfMntInfoFieldsPerLine {
		return nil, fmt.Errorf("wrong number of fields (expected at least %d, got %d): %s",
			minNumProcSelfMntInfoFieldsPerLine, numFields, line)
	}

	// separator must be in the 4th position from the end for the line to contain fsType, mountSource, and
	//  superOptions
	if fields[numFields-4] != "-" {
		return nil, fmt.Errorf("malformed mountinfo (could not find separator): %s", line)
	}

	// If root value is marked deleted, skip the entry
	if strings.Contains(fields[3], "deleted") {
		return nil, nil
	}

	mp := models.MountInfo{
		DeviceId:     fields[2],
		Root:         fields[3],
		MountPoint:   fields[4],
		MountOptions: strings.Split(fields[5], ","),
	}

	mountId, err := strconv.Atoi(fields[0])
	if err != nil {
		return nil, err
	}
	mp.MountId = mountId

	parentId, err := strconv.Atoi(fields[1])
	if err != nil {
		return nil, err
	}
	mp.ParentId = parentId

	mp.FsType = fields[numFields-3]
	mp.MountSource = fields[numFields-2]
	mp.SuperOptions = strings.Split(fields[numFields-1], ",")

	return &mp, nil
}

// compareMount compares the given mountpoint and sourceDevice with the retrieved entry from /proc/self/mountinfo
func (client *LinuxClient) compareMount(ctx context.Context, mountpoint, sourceDevice, devicePath string, procMount *models.MountInfo) (bool, error) {
	logFields := LogFields{
		"mountPoint": mountpoint,
		"procMount":  procMount,
	}

	var err error

	if mountpoint != "" {
		if !strings.Contains(procMount.MountPoint, mountpoint) {
			return false, nil
		}
		Logc(ctx).WithFields(logFields).Debugf("Mountpoint found: %v", procMount)
	}

	// If sourceDevice was specified and doesn't match proc mount, move on
	if sourceDevice != "" {

		procSourceDevice := strings.TrimPrefix(procMount.Root, "/")

		if strings.HasPrefix(procMount.MountSource, "/dev/") {
			procSourceDevice = strings.TrimPrefix(procMount.MountSource, "/dev/")
			if sourceDevice != procSourceDevice && devicePath != procSourceDevice {
				// Resolve any symlinks to get the real device, if device path has not already been found
				procSourceDevice, err = client.filepath.EvalSymlinks(procMount.MountSource)
				if err != nil {
					Logc(ctx).WithFields(logFields).WithError(err).Debug("Could not resolve device symlink")
					return false, err
				}
				procSourceDevice = strings.TrimPrefix(procSourceDevice, "/dev/")
			}
		}

		if sourceDevice != procSourceDevice && devicePath != procSourceDevice {
			return false, nil
		}

		Logc(ctx).WithFields(logFields).Debugf("Device found: %v", sourceDevice)
	}

	return true, nil
}
