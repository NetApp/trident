// Copyright 2023 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

// NVMeActiveOnHost checks if NVMe is active on host
func NVMeActiveOnHost(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> nvme_windows.NVMeActiveOnHost")
	defer Logc(ctx).Debug("<<<< nvme_windows.NVMeActiveOnHost")
	return false, errors.UnsupportedError("NVMeActiveOnHost is not supported for windows")
}

// GetHostNqn returns the Nqn string of the k8s node.
func GetHostNqn(ctx context.Context) (string, error) {
	Logc(ctx).Debug(">>>> nvme_windows.GetHostNqn")
	defer Logc(ctx).Debug("<<<< nvme_windows.GetHostNqn")
	return "", errors.UnsupportedError("GetHostNqn is not supported for windows")
}

// GetNVMeSubsystemList returns the list of subsystems connected to the k8s node.
func GetNVMeSubsystemList(ctx context.Context) (Subsystems, error) {
	Logc(ctx).Debug(">>>> nvme_windows.GetNVMeSubsystemList")
	defer Logc(ctx).Debug("<<<< nvme_windows.GetNVMeSubsystemList")
	return Subsystems{}, errors.UnsupportedError("GetNVMeSubsystemList is not supported for windows")
}

// ConnectSubsystemToHost creates a path (or session) from the ONTAP subsystem to the k8s node using svmDataLIF.
func ConnectSubsystemToHost(ctx context.Context, subsNqn, svmDataLIF string) error {
	Logc(ctx).Debug(">>>> nvme_windows.ConnectSubsystemToHost")
	defer Logc(ctx).Debug("<<<< nvme_windows.ConnectSubsystemToHost")
	return errors.UnsupportedError("ConnectSubsystemToHost is not supported for windows")
}

// DisconnectSubsystemFromHost removes the subsystem from the k8s node.
func DisconnectSubsystemFromHost(ctx context.Context, subsysNqn string) error {
	Logc(ctx).Debug(">>>> nvme_windows.DisconnectSubsystemFromHost")
	defer Logc(ctx).Debug("<<<< nvme_windows.DisconnectSubsystemFromHost")
	return errors.UnsupportedError("DisconnectSubsystemFromHost is not supported for windows")
}

// GetNamespaceCountForSubsDevice returns the number of namespaces present in a given subsystem device.
func GetNamespaceCountForSubsDevice(ctx context.Context, subsDevice string) (int, error) {
	Logc(ctx).Debug(">>>> nvme_windows.GetNamespaceCount")
	defer Logc(ctx).Debug("<<<< nvme_windows.GetNamespaceCount")
	return 0, errors.UnsupportedError("GetNamespaceCountForSubsDevice is not supported for windows")
}

// GetNVMeDeviceList returns the list of NVMe devices present on the k8s node.
func GetNVMeDeviceList(ctx context.Context) (NVMeDevices, error) {
	Logc(ctx).Debug(">>>> nvme_windows.GetNVMeDeviceList")
	defer Logc(ctx).Debug("<<<< nvme_windows.GetNVMeDeviceList")
	return NVMeDevices{}, errors.UnsupportedError("GetNVMeDeviceList is not supported for windows")
}

// FlushNVMeDevice flushes any ongoing IOs present on the NVMe device.
func FlushNVMeDevice(ctx context.Context, device string) error {
	Logc(ctx).Debug(">>>> nvme_windows.FlushNVMeDevice")
	defer Logc(ctx).Debug("<<<< nvme_windows.FlushNVMeDevice")
	return errors.UnsupportedError("FlushNVMeDevice is not supported for windows")
}
