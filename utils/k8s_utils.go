// Copyright 2017 The Kubernetes Authors.
// Copyright 2018 NetApp, Inc. All Rights Reserved.

package utils

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"

	"os"
	"syscall"

	log "github.com/sirupsen/logrus"
)

const (
	// How many times to retry for a consistent read of /proc/mounts.
	maxListTries = 3
	// Number of fields per line in /proc/mounts as per the fstab man page.
	expectedNumProcMntFieldsPerLine = 6
	// Number of fields per line in /proc/self/mountinfo as per the fstab man page.
	expectedNumProcSelfMntInfoFieldsPerLine = 11
	// Location of the mount file to use
	procMountsPath = "/proc/mounts"
	// Location of the mount file to use
	procSelfMountinfoPath = "/proc/self/mountinfo"
	// Location of the mount file to use
	udevSource     = "udev"
	devTmpFsSource = "devTmpFsSource"
)

// This represents a single line in /proc/mounts or /etc/fstab.
type MountPoint struct {
	Device string
	Path   string
	Type   string
	Opts   []string
	Freq   int
	Pass   int
}

// This represents a single line in /proc/self/mountinfo.
type MountInfo struct {
	MountId        int
	ParentId       int
	DeviceId       string
	Root           string
	MountPoint     string
	MountOptions   []string
	OptionalFields []string
	FsType         string
	MountSource    string
	SuperOptions   []string
}

// IsLikelyDir determines if mountpoint is a directory
func IsLikelyDir(mountpoint string) (bool, error) {
	stat, err := os.Stat(mountpoint)
	if err != nil {
		return false, err
	}

	return stat.IsDir(), nil
}

// IsLikelyNotMountPoint determines if a directory is not a mountpoint.
func IsLikelyNotMountPoint(mountpoint string) (bool, error) {
	stat, err := os.Stat(mountpoint)
	if err != nil {
		return true, err
	}
	rootStat, err := os.Lstat(filepath.Dir(strings.TrimSuffix(mountpoint, "/")))
	if err != nil {
		return true, err
	}
	// If the directory has a different device as parent, then it is a mountpoint.
	if stat.Sys().(*syscall.Stat_t).Dev != rootStat.Sys().(*syscall.Stat_t).Dev {
		return false, nil
	}

	return true, nil
}

func GetDeviceNameFromMount(mountpath string) (string, int, error) {

	fields := log.Fields{"mountpath": mountpath}
	log.WithFields(fields).Debug(">>>> k8s_utils.GetDeviceNameFromMount")
	defer log.WithFields(fields).Debug("<<<< k8s_utils.GetDeviceNameFromMount")

	mps, err := listProcMounts(procMountsPath)
	if err != nil {
		return "", 0, err
	}

	// Find the device name.
	// FIXME if multiple devices mounted on the same mount path, only the first one is returned
	device := ""
	// If mountPath is symlink, need get its target path.
	slTarget, err := filepath.EvalSymlinks(mountpath)
	if err != nil {
		slTarget = mountpath
	}
	for i := range mps {
		if mps[i].Path == slTarget {
			device = mps[i].Device
			break
		}
	}

	// Find all references to the device.
	refCount := 0
	for i := range mps {
		if mps[i].Device == device {
			refCount++
		}
	}

	log.WithFields(log.Fields{
		"mountpath": mountpath,
		"device":    device,
		"refCount":  refCount,
	}).Debug("Found device from mountpath.")

	return device, refCount, nil
}

// listProcSelfMountinfo (Available since Linux 2.6.26) lists information about mount points
// in the process's mount namespace. Ref: http://man7.org/linux/man-pages/man5/proc.5.html
func listProcSelfMountinfo(mountFilePath string) ([]MountInfo, error) {
	content, err := ConsistentRead(mountFilePath, maxListTries)
	if err != nil {
		return nil, err
	}
	return parseProcSelfMountinfo(content)
}

// parseProcSelfMountinfo parses the output of /proc/self/mountinfo file into a slice of MountInfo struct
func parseProcSelfMountinfo(content []byte) ([]MountInfo, error) {
	out := make([]MountInfo, 0)
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if line == "" {
			// The last split() item is empty string following the last \n
			continue
		}
		fields := strings.Fields(line)
		expectedFieldsPerLine := expectedNumProcSelfMntInfoFieldsPerLine
		if len(fields) > expectedFieldsPerLine || len(fields) < (expectedFieldsPerLine-1) {
			return nil, fmt.Errorf("wrong number of fields (expected %d or %d, got %d): %s", expectedFieldsPerLine,
				(expectedFieldsPerLine - 1), len(fields), line)
		}

		mp := MountInfo{
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

		if fields[6] != "-" {
			mp.OptionalFields = strings.Split(fields[6], ",")
			mp.FsType = fields[8]
			mp.MountSource = fields[9]
			mp.SuperOptions = strings.Split(fields[10], ",")
		} else {
			mp.OptionalFields = nil
			mp.FsType = fields[7]
			mp.MountSource = fields[8]
			mp.SuperOptions = strings.Split(fields[9], ",")
		}

		out = append(out, mp)
	}
	return out, nil
}

func listProcMounts(mountFilePath string) ([]MountPoint, error) {
	content, err := ConsistentRead(mountFilePath, maxListTries)
	if err != nil {
		return nil, err
	}
	return parseProcMounts(content)
}

func parseProcMounts(content []byte) ([]MountPoint, error) {
	out := make([]MountPoint, 0)
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

		mp := MountPoint{
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

// ConsistentRead repeatedly reads a file until it gets the same content twice.
// This is useful when reading files in /proc that are larger than page size
// and kernel may modify them between individual read() syscalls.
func ConsistentRead(filename string, attempts int) ([]byte, error) {
	oldContent, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	for i := 0; i < attempts; i++ {
		newContent, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		if bytes.Compare(oldContent, newContent) == 0 {
			return newContent, nil
		}
		// Files are different, continue reading
		oldContent = newContent
	}
	return nil, fmt.Errorf("could not get consistent content of %s after %d attempts", filename, attempts)
}
