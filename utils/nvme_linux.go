// Copyright 2024 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/netapp/trident/logging"
	sa "github.com/netapp/trident/storage_attribute"
)

var transport = "tcp"

// GetHostNqn returns the Nqn string of the k8s node.
func GetHostNqn(ctx context.Context) (string, error) {
	Logc(ctx).Debug(">>>> nvme_linux.GetHostNqn")
	defer Logc(ctx).Debug("<<<< nvme_linux.GetHostNqn")

	out, err := command.Execute(ctx, "cat", "/etc/nvme/hostnqn")
	if err != nil {
		Logc(ctx).WithField("Error", err).Warn("Could not read hostnqn; perhaps NVMe is not installed?")
		return "", fmt.Errorf("failed to get hostnqn: %v", err)
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
		Logc(ctx).WithField("Error", err).Warn("Could not read nvme cli version; perhaps NVMe cli is not installed?")
		return false, fmt.Errorf("failed to get hostnqn: %v", err)
	}

	out, err := command.ExecuteWithTimeout(ctx, "lsmod", NVMeListCmdTimeoutInSeconds*time.Second, false)
	if err != nil {
		Logc(ctx).WithField("Error", err).Warn("Could not read the modules loaded on the host")
		return false, fmt.Errorf("failed to get nvme driver info")
	}
	newout := strings.Split(string(out), "\n")
	for _, s := range newout {
		if strings.Contains(s, fmt.Sprintf("%s_%s", sa.NVMe, transport)) {
			return true, nil
		}
	}
	return false, fmt.Errorf("NVMe driver is not loaded on the host")
}

// GetNVMeSubsystemList returns the list of subsystems connected to the k8s node.
func GetNVMeSubsystemList(ctx context.Context) (Subsystems, error) {
	Logc(ctx).Debug(">>>> nvme_linux.GetNVMeSubsystemList")
	defer Logc(ctx).Debug("<<<< nvme_linux.GetNVMeSubsystemList")

	var subs Subsystems

	out, err := command.ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "list-subsys", "-o", "json")
	if err != nil {
		Logc(ctx).WithField("Error", err).Errorf("Failed to list subsystem: %v", err)
		return subs, fmt.Errorf("failed to list subsys %v", err)
	}

	// For RHEL, the output is present in array for this command.
	if string(out)[0] == '[' {
		var rhelSubs []Subsystems
		if err = json.Unmarshal([]byte(out), &rhelSubs); err != nil {
			Logc(ctx).WithField("Error", err).Errorf("Failed to unmarshal ontap nvme devices: %v", err)
			return subs, fmt.Errorf("failed to unmarshal ontap nvme devices: %v", err)
		}

		if len(rhelSubs) > 0 {
			return rhelSubs[0], nil
		}
		// No subsystems are present.
		return subs, nil
	}

	if err = json.Unmarshal(out, &subs); err != nil {
		Logc(ctx).WithField("Error", err).Errorf("Failed to unmarshal subsystem: %v", err)
		return subs, fmt.Errorf("failed to unmarshal subsys %v", err)
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
		Logc(ctx).WithField("Error", err).Errorf("Failed to connect subsystem to host: %v", err)
		return fmt.Errorf("failed to connect subsystem %s to %s: %v", subsNqn, IP, err)
	}

	return nil
}

// DisconnectSubsystemFromHost removes the subsystem from the k8s node.
func DisconnectSubsystemFromHost(ctx context.Context, subsysNqn string) error {
	Logc(ctx).Debug(">>>> nvme_linux.DisconnectSubsystemFromHost")
	defer Logc(ctx).Debug("<<<< nvme_linux.DisconnectSubsystemFromHost")

	_, err := command.Execute(ctx, "nvme", "disconnect", "-n", subsysNqn)
	if err != nil {
		Logc(ctx).WithField("Error", err).Errorf("Failed to disconnect subsystem %s: %v", subsysNqn, err)
		return fmt.Errorf("failed to disconnect subsystem %s: %v", subsysNqn, err)
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
		Logc(ctx).WithField("Error", err).Errorf("Failed to get namespace count: %v", err)
		return 0, fmt.Errorf("failed to get namespace count: %v", err)
	}

	return strings.Count(string(out), "["), nil
}

// GetNVMeDeviceList returns the list of NVMe devices present on the k8s node.
func GetNVMeDeviceList(ctx context.Context) (NVMeDevices, error) {
	Logc(ctx).Debug(">>>> nvme_linux.GetNVMeDeviceList")
	defer Logc(ctx).Debug("<<<< nvme_linux.GetNVMeDeviceList")

	var ontapDevs NVMeDevices

	out, err := command.ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "netapp", "ontapdevices", "-o", "json")
	if err != nil {
		Logc(ctx).WithField("Error", err).Errorf("Failed to exec list nvme ontap devices: %v", err)
		return ontapDevs, fmt.Errorf("failed to exec list nvme ontap devices: %v", err)
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
			Logc(ctx).WithField("Error", err).Errorf("Failed to unmarshal ontap nvme devices: %v", err)
			return ontapDevs, fmt.Errorf("failed to unmarshal ontap nvme devices: %v", err)
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
		Logc(ctx).Error("Error while flushing the device %s, %v", device, err)
		return fmt.Errorf("error while flushing the device %s, %v", device, err)
	}

	return nil
}
