// Copyright 2024 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	. "github.com/netapp/trident/logging"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
)

var (
	transport    = "tcp"
	nvmeNQNRegex = regexp.MustCompile(`^nvme([0-9]+)n([0-9]+)$`)
	nvmeRegex    = regexp.MustCompile(`^nvme([0-9]+)$`)
)

const (
	NVME_PATH = "/sys/class/nvme-subsystem"
	SUBSYSNQN = "/subsysnqn"
)

func ReadFile(fs filesystem.FSClient, filename string) ([]byte, error) {
	data, err := fs.ScanFile(filename)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func ReadDir(fs filesystem.FSClient, path string) ([]os.FileInfo, error) {
	dir, err := fs.ScanDir(path)
	if err != nil {
		return nil, err
	}
	return dir, nil
}

// GetHostNqn returns the Nqn string of the k8s node.
func GetHostNqn(ctx context.Context) (string, error) {
	Logc(ctx).Debug(">>>> nvme_linux.GetHostNqn")
	defer Logc(ctx).Debug("<<<< nvme_linux.GetHostNqn")

	out, err := command.Execute(ctx, "cat", "/etc/nvme/hostnqn")
	if err != nil {
		Logc(ctx).WithError(err).Warn("Could not read hostnqn; perhaps NVMe is not installed?")
		return "", fmt.Errorf("failed to get hostnqn; %v", err)
	}

	newout := strings.Split(string(out), "\n")
	return newout[0], nil
}

func NVMeActiveOnHost(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> nvme_linux.NVMeActiveOnHost")
	defer Logc(ctx).Debug("<<<< nvme_linux.NVMeActiveOnHost")

	_, err := command.ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "version")
	if err != nil {
		Logc(ctx).WithError(err).Warn("Could not read NVMe CLI version; perhaps NVMe CLI is not installed?")
		return false, fmt.Errorf("failed to get hostnqn: %v", err)
	}

	out, err := command.ExecuteWithTimeout(ctx, "lsmod", NVMeListCmdTimeoutInSeconds*time.Second, false)
	if err != nil {
		Logc(ctx).WithError(err).Warn("Could not read the modules loaded on the host.")
		return false, fmt.Errorf("failed to get NVMe driver info; %v", err)
	}

	newout := strings.Split(string(out), "\n")
	for _, s := range newout {
		if strings.Contains(s, fmt.Sprintf("%s_%s", sa.NVMe, transport)) {
			return true, nil
		}
	}

	return false, fmt.Errorf("NVMe driver is not loaded on the host")
}

func listSubsystemsFromSysFs(fs filesystem.FSClient, ctx context.Context) (Subsystems, error) {
	Logc(ctx).Trace(">>>> nvme_linux.listSubsystemsFromSysFs")
	defer Logc(ctx).Trace("<<<< nvme_linux.listSubsystemsFromSysFs")

	var subsystems Subsystems
	subsystemDirs, err := ReadDir(fs, NVME_PATH)
	if err != nil {
		return subsystems, fmt.Errorf("failed to open nvme subsystems directory: %v", err)
	}

	for _, subsystemDir := range subsystemDirs {
		subsystemDirPath := NVME_PATH + "/" + subsystemDir.Name()
		subsystemNqnPath := subsystemDirPath + SUBSYSNQN
		fileBytes, err := ReadFile(fs, subsystemNqnPath)
		if err != nil {
			return subsystems, fmt.Errorf("failed to read subsystem nqn: %v", err)
		}

		fileContent := strings.TrimSpace(string(fileBytes))

		sub := NVMeSubsystem{NQN: fileContent, Name: subsystemDirPath}
		paths, err := GetNVMeSubsystemPaths(ctx, fs, subsystemDirPath)
		if err != nil {
			return subsystems, err
		}
		sub.Paths = paths
		subsystems.Subsystems = append(subsystems.Subsystems, sub)
	}

	return subsystems, nil
}

// GetNVMeSubsystemList returns the list of subsystems connected to the k8s node.
func GetNVMeSubsystemList(ctx context.Context) (Subsystems, error) {
	Logc(ctx).Debug(">>>> nvme_linux.GetNVMeSubsystemList")
	defer Logc(ctx).Debug("<<<< nvme_linux.GetNVMeSubsystemList")

	var subs Subsystems

	out, err := command.ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "list-subsys", "-o", "json")
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to list subsystem.")
		return subs, fmt.Errorf("failed to list subsys %v", err)
	}

	// For RHEL, the output is present in array for this command.
	if string(out)[0] == '[' {
		var rhelSubs []Subsystems
		if err = json.Unmarshal(out, &rhelSubs); err != nil {
			Logc(ctx).WithError(err).Error("Failed to unmarshal ONTAP NVMe devices.")
			return subs, fmt.Errorf("failed to unmarshal ONTAP NVMe devices: %v", err)
		}

		if len(rhelSubs) > 0 {
			return rhelSubs[0], nil
		}
		// No subsystems are present.
		return subs, nil
	}

	if err = json.Unmarshal(out, &subs); err != nil {
		Logc(ctx).WithError(err).Error("Failed to unmarshal subsystem.")
		return subs, fmt.Errorf("failed to unmarshal subsystems; %v", err)
	}

	return subs, nil
}

// ConnectSubsystemToHost creates a path (or session) from the subsystem to the k8s node for the provided IP.
func ConnectSubsystemToHost(ctx context.Context, subsNqn, IP string) error {
	Logc(ctx).Debug(">>>> nvme_linux.ConnectSubsystemToHost")
	defer Logc(ctx).Debug("<<<< nvme_linux.ConnectSubsystemToHost")

	// Specifying value of "l" (ctrl-loss-tmo) to -1 makes the NVMe session undroppable even if the IP goes down for infinity.
	_, err := command.Execute(ctx, "nvme", "connect", "-t", "tcp", "-n", subsNqn, "-a", IP,
		"-s", "4420", "-l", "-1")
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Failed to connect subsystem %s to host.", subsNqn)
		return fmt.Errorf("failed to connect subsystem %s to %s; %v", subsNqn, IP, err)
	}

	return nil
}

// DisconnectSubsystemFromHost removes the subsystem from the k8s node.
func DisconnectSubsystemFromHost(ctx context.Context, subsysNqn string) error {
	Logc(ctx).Debug(">>>> nvme_linux.DisconnectSubsystemFromHost")
	defer Logc(ctx).Debug("<<<< nvme_linux.DisconnectSubsystemFromHost")

	_, err := command.Execute(ctx, "nvme", "disconnect", "-n", subsysNqn)
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Failed to disconnect subsystem %s.", subsysNqn)
		return fmt.Errorf("failed to disconnect subsystem %s; %v", subsysNqn, err)
	}

	return nil
}

// GetNamespaceCountForSubsDevice returns the number of namespaces present in a given subsystem device.
func GetNamespaceCountForSubsDevice(ctx context.Context, subsDevice string) (int, error) {
	Logc(ctx).Debug(">>>> nvme_linux.GetNamespaceCount")
	defer Logc(ctx).Debug("<<<< nvme_linux.GetNamespaceCount")

	out, err := command.ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "list-ns", subsDevice)
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to get namespace count.")
		return 0, fmt.Errorf("failed to get namespace count; %v", err)
	}

	return strings.Count(string(out), "["), nil
}

func GetNVMeSubsystem(ctx context.Context, fs filesystem.FSClient, nqn string) (NVMeSubsystem, error) {
	Logc(ctx).Trace(">>>> nvme_linux.GetNVMeSubsystem")
	defer Logc(ctx).Trace("<<<< nvme_linux.GetNVMeSubsystem")

	sub := NVMeSubsystem{NQN: nqn}
	subsystemDirs, err := ReadDir(fs, NVME_PATH)
	if err != nil {
		return sub, fmt.Errorf("failed to open nvme subsystems directory: %v", err)
	}

	subsystemDirPath := ""
	for _, subsystemDir := range subsystemDirs {
		subsystemDirPath = NVME_PATH + "/" + subsystemDir.Name()
		subsystemNqnPath := subsystemDirPath + SUBSYSNQN

		// Example of subsysnqn file : nqn.1992-08.com.netapp:sn.6628417f7bec11ef9bf2005056b3e634:subsystem.scspa3014048001-b06e4d9a-6817-446b-8dc6-e819c100f935
		fileBytes, err := ReadFile(fs, subsystemNqnPath)
		if err != nil {
			return sub, fmt.Errorf("failed to read subsystem nqn: %v", err)
		}

		fileContent := strings.TrimSpace(string(fileBytes))
		// Ignore this subsystem because it doesn't have the right NQN.
		if nqn != fileContent {
			continue
		}

		// Gather the subsystem paths.
		sub.Name = subsystemDirPath
		paths, err := GetNVMeSubsystemPaths(ctx, fs, subsystemDirPath)
		if err != nil {
			return sub, err
		}
		sub.Paths = paths
	}

	if len(sub.Paths) == 0 {
		return sub, errors.NotFoundError("no subsystem paths found")
	}

	return sub, nil
}

func GetNVMeSubsystemPaths(ctx context.Context, fs filesystem.FSClient, subsystemDirPath string) ([]Path, error) {
	Logc(ctx).Trace(">>>> nvme_linux.GetNVMeSubsystemPaths")
	defer Logc(ctx).Trace("<<<< nvme_linux.GetNVMeSubsystemPaths")

	var paths []Path

	subsystemDirContents, err := ReadDir(fs, subsystemDirPath)
	if err != nil {
		return paths, fmt.Errorf("failed to read subsystem directory contents, %v", err)
	}

	for _, subsystemDirContent := range subsystemDirContents {
		if nvmeRegex.MatchString(subsystemDirContent.Name()) {
			path := Path{Name: subsystemDirPath + "/" + subsystemDirContent.Name()}
			if err := updateNVMeSubsystemPathAttributes(ctx, fs, &path); err != nil {
				return paths, fmt.Errorf("failed to get path, %v", err)
			}

			paths = append(paths, path)
		}
	}

	return paths, nil
}

func updateNVMeSubsystemPathAttributes(ctx context.Context, fs filesystem.FSClient, path *Path) error {
	Logc(ctx).Trace(">>>> nvme_linux.updateNVMeSubsystemPathAttributes")
	defer Logc(ctx).Trace("<<<< nvme_linux.updateNVMeSubsystemPathAttributes")

	if path == nil {
		return fmt.Errorf("path is nil")
	}
	var err error
	// Example of state: live
	if path.State, err = getSessionFileContent("state", path.Name, fs); err != nil {
		Logc(ctx).WithError(err).Error("state is nil")
		return err
	}
	// Example of address: traddr=fd20:8b1e:b258:2014:9c83:2d91:44a:b618,trsvcid=4420
	if path.Address, err = getSessionFileContent("address", path.Name, fs); err != nil {
		Logc(ctx).WithError(err).Error("address is nil")
		return err
	}
	// Example of transport: tcp
	if path.Transport, err = getSessionFileContent("transport", path.Name, fs); err != nil {
		Logc(ctx).WithError(err).Error("transport is nil")
		return err
	}
	return nil
}

func getSessionFileContent(sessionFileName, pathName string, fs filesystem.FSClient) (string, error) {
	fileBytes, err := ReadFile(fs, pathName+"/"+sessionFileName)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(fileBytes)), nil
}

func GetNVMeDeviceCountAt(ctx context.Context, fs filesystem.FSClient, path string) (int, error) {
	Logc(ctx).Trace(">>>> nvme_linux.GetNVMeDeviceCountAt")
	defer Logc(ctx).Trace("<<<< nvme_linux.GetNVMeDeviceCountAt")

	count := 0

	pathDirContents, err := ReadDir(fs, path)
	if err != nil {
		return count, fmt.Errorf("failed to open %s directory, %v", path, err)
	}

	for _, pathDirContent := range pathDirContents {
		if nvmeNQNRegex.MatchString(pathDirContent.Name()) {
			count++
		}
	}

	return count, nil
}

func GetNVMeDeviceAt(ctx context.Context, path, nsUUID string) (NVMeDeviceInterface, error) {
	Logc(ctx).Trace(">>>> nvme_linux.GetNVMeDeviceAt")
	defer Logc(ctx).Trace("<<<< nvme_linux.GetNVMeDeviceAt")

	pathContents, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s directory, %v", path, err)
	}

	for _, pathContent := range pathContents {
		if nvmeNQNRegex.MatchString(pathContent.Name()) {
			uuidPath := path + "/" + pathContent.Name() + "/uuid"
			fileBytes, err := os.ReadFile(uuidPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read uuid, %v", err)
			}

			fileContent := strings.TrimSpace(string(fileBytes))

			if nsUUID == fileContent {
				return &NVMeDevice{UUID: nsUUID, Device: "/dev/" + pathContent.Name()}, nil
			}
		}
	}

	return nil, fmt.Errorf("nvme device not found")
}

// GetNVMeDeviceList returns the list of NVMe devices present on the k8s node.
func GetNVMeDeviceList(ctx context.Context) (NVMeDevices, error) {
	Logc(ctx).Debug(">>>> nvme_linux.GetNVMeDeviceList")
	defer Logc(ctx).Debug("<<<< nvme_linux.GetNVMeDeviceList")

	var ontapDevs NVMeDevices

	out, err := command.ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "netapp", "ontapdevices", "-o", "json")
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to list NVMe ONTAP devices.")
		return ontapDevs, fmt.Errorf("failed to list NVMe ONTAP devices; %v", err)
	}

	// There are 2 use cases where we may need to format the output before unmarshalling it.
	// 1. When no namespaces are associated with any subsystem, the output of this command is not a json.
	//    It is usually a string with this value - "No NVMe devices detected."
	// 2. When any device is unreachable, we get the list of those devices stating the error reason and the available
	//    list of devices is appended after that. For example -
	//    # nvme netapp ontapdevices -o json
	//    Identify Controller failed to /dev/nvme0n2 (Operation not permitted)
	//    {
	//       "ONTAPdevices":[ { ...,...} ]
	//    }
	//    In this case, we need to remove everything before valid json string starts from the string.
	if !json.Valid(out) {
		_, afterBrace, found := strings.Cut(string(out), "{")
		if found {
			afterBrace = "{" + afterBrace
		}
		out = []byte(afterBrace)
	}

	if string(out) != "" {
		// "out" would be empty string if there are no devices
		if err = json.Unmarshal(out, &ontapDevs); err != nil {
			Logc(ctx).WithError(err).Error("Failed to unmarshal NVMe ONTAP devices.")
			return ontapDevs, fmt.Errorf("failed to unmarshal NVMe ONTAP devices; %v", err)
		}
	}

	return ontapDevs, nil
}

// FlushNVMeDevice flushes any ongoing IOs present on the NVMe device.
func FlushNVMeDevice(ctx context.Context, device string) error {
	Logc(ctx).Debug(">>>> nvme_linux.FlushNVMeDevice")
	defer Logc(ctx).Debug("<<<< nvme_linux.FlushNVMeDevice")

	_, err := command.Execute(ctx, "nvme", "flush", device)
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Failed to flush device: %s", device)
		return fmt.Errorf("failed to flush device: %s; %v", device, err)
	}

	return nil
}
