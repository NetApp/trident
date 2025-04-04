// Copyright 2023 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"

	"github.com/spf13/afero"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

// NVMeActiveOnHost checks if NVMe is active on host
func (nh *NVMeHandler) NVMeActiveOnHost(ctx context.Context) (bool, error) {
	Logc(ctx).Debug(">>>> nvme_windows.NVMeActiveOnHost")
	defer Logc(ctx).Debug("<<<< nvme_windows.NVMeActiveOnHost")
	return false, errors.UnsupportedError("NVMeActiveOnHost is not supported for windows")
}

// GetHostNqn returns the Nqn string of the k8s node.
func (nh *NVMeHandler) GetHostNqn(ctx context.Context) (string, error) {
	Logc(ctx).Debug(">>>> nvme_windows.GetHostNqn")
	defer Logc(ctx).Debug("<<<< nvme_windows.GetHostNqn")
	return "", errors.UnsupportedError("GetHostNqn is not supported for windows")
}

// listSubsystemsFromSysFs returns the list of subsystems connected to the k8s node.
func (nh *NVMeHandler) listSubsystemsFromSysFs(ctx context.Context) (Subsystems, error) {
	Logc(ctx).Debug(">>>> nvme_windows.listSubsystemsFromSysFs")
	defer Logc(ctx).Debug("<<<< nvme_windows.listSubsystemsFromSysFs")
	return Subsystems{}, errors.UnsupportedError("ListSubsystemsFromSysFs is not supported for windows")
}

// ConnectSubsystemToHost creates a path (or session) from the ONTAP subsystem to the k8s node using svmDataLIF.
func (s *NVMeSubsystem) ConnectSubsystemToHost(ctx context.Context, svmDataLIF string) error {
	Logc(ctx).Debug(">>>> nvme_windows.ConnectSubsystemToHost")
	defer Logc(ctx).Debug("<<<< nvme_windows.ConnectSubsystemToHost")
	return errors.UnsupportedError("ConnectSubsystemToHost is not supported for windows")
}

// DisconnectSubsystemFromHost removes the subsystem from the k8s node.
func (s *NVMeSubsystem) DisconnectSubsystemFromHost(ctx context.Context) error {
	Logc(ctx).Debug(">>>> nvme_windows.DisconnectSubsystemFromHost")
	defer Logc(ctx).Debug("<<<< nvme_windows.DisconnectSubsystemFromHost")
	return errors.UnsupportedError("DisconnectSubsystemFromHost is not supported for windows")
}

func (nh *NVMeHandler) GetNVMeSubsystem(ctx context.Context, nqn string) (*NVMeSubsystem, error) {
	Logc(ctx).Debug(">>>> nvme_windows.GetNVMeSubsystem")
	defer Logc(ctx).Debug("<<<< nvme_windows.GetNVMeSubsystem")
	return &NVMeSubsystem{}, errors.UnsupportedError("GetNVMeSubsystem is not supported for windows")
}

func GetNVMeSubsystemPaths(ctx context.Context, fs afero.Fs, subsystemDirPath string) ([]Path, error) {
	Logc(ctx).Debug(">>>> nvme_windows.GetNVMeSubsystemPaths")
	defer Logc(ctx).Debug("<<<< nvme_windows.GetNVMeSubsystemPaths")
	return []Path{}, errors.UnsupportedError("GetNVMeSubsystemPaths is not supported for windows")
}

func InitializeNVMeSubsystemPath(ctx context.Context, path *Path) error {
	Logc(ctx).Debug(">>>> nvme_windows.InitializeNVMeSubsystemPath")
	defer Logc(ctx).Debug("<<<< nvme_windows.InitializeNVMeSubsystemPath")
	return errors.UnsupportedError("InitializeNVMeSubsystemPath is not supported for windows")
}

func (s *NVMeSubsystem) GetNVMeDeviceCountAt(ctx context.Context, path string) (int, error) {
	Logc(ctx).Debug(">>>> nvme_windows.GetNVMeDeviceCountAt")
	defer Logc(ctx).Debug("<<<< nvme_windows.GetNVMeDeviceCountAt")
	return 0, errors.UnsupportedError("GetNVMeDeviceCountAt is not supported for windows")
}

func (s *NVMeSubsystem) GetNVMeDeviceAt(ctx context.Context, nsUUID string) (*NVMeDevice, error) {
	Logc(ctx).Debug(">>>> nvme_windows.GetNVMeDeviceAt")
	defer Logc(ctx).Debug("<<<< nvme_windows.GetNVMeDeviceAt")
	return nil, errors.UnsupportedError("GetNVMeDeviceAt is not supported for windows")
}

// GetNamespaceCountForSubsDevice returns the number of namespaces present in a given subsystem device.
func (s *NVMeSubsystem) GetNamespaceCountForSubsDevice(ctx context.Context) (int, error) {
	Logc(ctx).Debug(">>>> nvme_windows.GetNamespaceCount")
	defer Logc(ctx).Debug("<<<< nvme_windows.GetNamespaceCount")
	return 0, errors.UnsupportedError("GetNamespaceCountForSubsDevice is not supported for windows")
}

// GetNVMeDeviceList returns the list of NVMe devices present on the k8s node.
func (nh *NVMeHandler) GetNVMeDeviceList(ctx context.Context) (NVMeDevices, error) {
	Logc(ctx).Debug(">>>> nvme_windows.GetNVMeDeviceList")
	defer Logc(ctx).Debug("<<<< nvme_windows.GetNVMeDeviceList")
	return NVMeDevices{}, errors.UnsupportedError("GetNVMeDeviceList is not supported for windows")
}

// FlushNVMeDevice flushes any ongoing IOs present on the NVMe device.
func (d *NVMeDevice) FlushNVMeDevice(ctx context.Context) error {
	Logc(ctx).Debug(">>>> nvme_windows.FlushNVMeDevice")
	defer Logc(ctx).Debug("<<<< nvme_windows.FlushNVMeDevice")
	return errors.UnsupportedError("FlushNVMeDevice is not supported for windows")
}
