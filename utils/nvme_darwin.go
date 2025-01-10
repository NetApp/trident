// Copyright 2023 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
)

// NVMeActiveOnHost checks if NVMe is active on host
func NVMeActiveOnHost(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> nvme_darwin.NVMeActiveOnHost")
	defer Logc(ctx).Debug("<<<< nvme_darwin.NVMeActiveOnHost")
	return false, errors.UnsupportedError("NVMeActiveOnHost is not supported for darwin")
}

// GetHostNqn returns the Nqn string of the k8s node.
func GetHostNqn(ctx context.Context) (string, error) {
	Logc(ctx).Debug(">>>> nvme_darwin.GetHostNqn")
	defer Logc(ctx).Debug("<<<< nvme_darwin.GetHostNqn")
	return "", errors.UnsupportedError("GetHostNqn is not supported for darwin")
}

// GetNVMeSubsystemList returns the list of subsystems connected to the k8s node.
func listSubsystemsFromSysFs(fs filesystem.FSClient, ctx context.Context) (Subsystems, error) {
	Logc(ctx).Debug(">>>> nvme_darwin.listSubsystemsFromSysFs")
	defer Logc(ctx).Debug("<<<< nvme_darwin.listSubsystemsFromSysFs")
	return Subsystems{}, errors.UnsupportedError("listSubsystemsFromSysFs is not supported for darwin")
}

// ConnectSubsystemToHost creates a path (or session) from the ONTAP subsystem to the k8s node using svmDataLIF.
func ConnectSubsystemToHost(ctx context.Context, subsNqn, svmDataLIF string) error {
	Logc(ctx).Debug(">>>> nvme_darwin.ConnectSubsystemToHost")
	defer Logc(ctx).Debug("<<<< nvme_darwin.ConnectSubsystemToHost")
	return errors.UnsupportedError("ConnectSubsystemToHost is not supported for darwin")
}

// DisconnectSubsystemFromHost removes the subsystem from the k8s node.
func DisconnectSubsystemFromHost(ctx context.Context, subsysNqn string) error {
	Logc(ctx).Debug(">>>> nvme_darwin.DisconnectSubsystemFromHost")
	defer Logc(ctx).Debug("<<<< nvme_darwin.DisconnectSubsystemFromHost")
	return errors.UnsupportedError("DisconnectSubsystemFromHost is not supported for darwin")
}

func GetNVMeSubsystem(ctx context.Context, fs filesystem.FSClient, nqn string) (NVMeSubsystem, error) {
	Logc(ctx).Debug(">>>> nvme_darwin.GetNVMeSubsystem")
	defer Logc(ctx).Debug("<<<< nvme_darwin.GetNVMeSubsystem")
	return NVMeSubsystem{}, errors.UnsupportedError("GetNVMeSubsystem is not supported for darwin")
}

func GetNVMeSubsystemPaths(ctx context.Context, fs filesystem.FSClient, subsystemDirPath string) ([]Path, error) {
	Logc(ctx).Debug(">>>> nvme_darwin.GetNVMeSubsystemPaths")
	defer Logc(ctx).Debug("<<<< nvme_darwin.GetNVMeSubsystemPaths")
	return []Path{}, errors.UnsupportedError("GetNVMeSubsystemPaths is not supported for darwin")
}

func InitializeNVMeSubsystemPath(ctx context.Context, path *Path) error {
	Logc(ctx).Debug(">>>> nvme_darwin.InitializeNVMeSubsystemPath")
	defer Logc(ctx).Debug("<<<< nvme_darwin.InitializeNVMeSubsystemPath")
	return errors.UnsupportedError("InitializeNVMeSubsystemPath is not supported for darwin")
}

func GetNVMeDeviceCountAt(ctx context.Context, fs filesystem.FSClient, path string) (int, error) {
	Logc(ctx).Debug(">>>> nvme_darwin.GetNVMeDeviceCountAt")
	defer Logc(ctx).Debug("<<<< nvme_darwin.GetNVMeDeviceCountAt")
	return 0, errors.UnsupportedError("GetNVMeDeviceCountAt is not supported for darwin")
}

func GetNVMeDeviceAt(ctx context.Context, path, nsUUID string) (NVMeDeviceInterface, error) {
	Logc(ctx).Debug(">>>> nvme_darwin.GetNVMeDeviceAt")
	defer Logc(ctx).Debug("<<<< nvme_darwin.GetNVMeDeviceAt")
	return nil, errors.UnsupportedError("GetNVMeDeviceAt is not supported for darwin")
}

// GetNamespaceCountForSubsDevice returns the number of namespaces present in a given subsystem device.
func GetNamespaceCountForSubsDevice(ctx context.Context, subsDevice string) (int, error) {
	Logc(ctx).Debug(">>>> nvme_darwin.GetNamespaceCount")
	defer Logc(ctx).Debug("<<<< nvme_darwin.GetNamespaceCount")
	return 0, errors.UnsupportedError("GetNamespaceCountForSubsDevice is not supported for darwin")
}

// GetNVMeDeviceList returns the list of NVMe devices present on the k8s node.
func GetNVMeDeviceList(ctx context.Context) (NVMeDevices, error) {
	Logc(ctx).Debug(">>>> nvme_darwin.GetNVMeDeviceList")
	defer Logc(ctx).Debug("<<<< nvme_darwin.GetNVMeDeviceList")
	return NVMeDevices{}, errors.UnsupportedError("GetNVMeDeviceList is not supported for darwin")
}

// FlushNVMeDevice flushes any ongoing IOs present on the NVMe device.
func FlushNVMeDevice(ctx context.Context, device string) error {
	Logc(ctx).Debug(">>>> nvme_darwin.FlushNVMeDevice")
	defer Logc(ctx).Debug("<<<< nvme_darwin.FlushNVMeDevice")
	return errors.UnsupportedError("FlushNVMeDevice is not supported for darwin")
}
