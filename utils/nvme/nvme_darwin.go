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
	Logc(ctx).Debug(">>>> nvme_darwin.NVMeActiveOnHost")
	defer Logc(ctx).Debug("<<<< nvme_darwin.NVMeActiveOnHost")
	return false, errors.UnsupportedError("NVMeActiveOnHost is not supported for darwin")
}

// GetHostNqn returns the Nqn string of the k8s node.
func (nh *NVMeHandler) GetHostNqn(ctx context.Context) (string, error) {
	Logc(ctx).Debug(">>>> nvme_darwin.GetHostNqn")
	defer Logc(ctx).Debug("<<<< nvme_darwin.GetHostNqn")
	return "", errors.UnsupportedError("GetHostNqn is not supported for darwin")
}

// listSubsystemsFromSysFs returns the list of subsystems connected to the k8s node.
func (nh *NVMeHandler) listSubsystemsFromSysFs(ctx context.Context) (Subsystems, error) {
	Logc(ctx).Debug(">>>> nvme_darwin.listSubsystemsFromSysFs")
	defer Logc(ctx).Debug("<<<< nvme_darwin.listSubsystemsFromSysFs")
	return Subsystems{}, errors.UnsupportedError("listSubsystemsFromSysFs is not supported for darwin")
}

// ConnectSubsystemToHost creates a path (or session) from the ONTAP subsystem to the k8s node using svmDataLIF.
func (s *NVMeSubsystem) ConnectSubsystemToHost(ctx context.Context, svmDataLIF string) error {
	Logc(ctx).Debug(">>>> nvme_darwin.ConnectSubsystemToHost")
	defer Logc(ctx).Debug("<<<< nvme_darwin.ConnectSubsystemToHost")
	return errors.UnsupportedError("ConnectSubsystemToHost is not supported for darwin")
}

// DisconnectSubsystemFromHost removes the subsystem from the k8s node.
func (s *NVMeSubsystem) DisconnectSubsystemFromHost(ctx context.Context) error {
	Logc(ctx).Debug(">>>> nvme_darwin.DisconnectSubsystemFromHost")
	defer Logc(ctx).Debug("<<<< nvme_darwin.DisconnectSubsystemFromHost")
	return errors.UnsupportedError("DisconnectSubsystemFromHost is not supported for darwin")
}

func (nh *NVMeHandler) GetNVMeSubsystem(ctx context.Context, nqn string) (*NVMeSubsystem,
	error,
) {
	Logc(ctx).Debug(">>>> nvme_darwin.GetNVMeSubsystem")
	defer Logc(ctx).Debug("<<<< nvme_darwin.GetNVMeSubsystem")
	return &NVMeSubsystem{}, errors.UnsupportedError("GetNVMeSubsystem is not supported for darwin")
}

func GetNVMeSubsystemPaths(ctx context.Context, fs afero.Fs, subsystemDirPath string) ([]Path, error) {
	Logc(ctx).Debug(">>>> nvme_darwin.GetNVMeSubsystemPaths")
	defer Logc(ctx).Debug("<<<< nvme_darwin.GetNVMeSubsystemPaths")
	return []Path{}, errors.UnsupportedError("GetNVMeSubsystemPaths is not supported for darwin")
}

func InitializeNVMeSubsystemPath(ctx context.Context, path *Path) error {
	Logc(ctx).Debug(">>>> nvme_darwin.InitializeNVMeSubsystemPath")
	defer Logc(ctx).Debug("<<<< nvme_darwin.InitializeNVMeSubsystemPath")
	return errors.UnsupportedError("InitializeNVMeSubsystemPath is not supported for darwin")
}

func (s *NVMeSubsystem) GetNVMeDeviceCountAt(ctx context.Context, path string) (int, error) {
	Logc(ctx).Debug(">>>> nvme_darwin.GetNVMeDeviceCountAt")
	defer Logc(ctx).Debug("<<<< nvme_darwin.GetNVMeDeviceCountAt")
	return 0, errors.UnsupportedError("GetNVMeDeviceCountAt is not supported for darwin")
}

func (s *NVMeSubsystem) GetNVMeDeviceAt(ctx context.Context, nsUUID string) (*NVMeDevice, error) {
	Logc(ctx).Debug(">>>> nvme_darwin.GetNVMeDeviceAt")
	defer Logc(ctx).Debug("<<<< nvme_darwin.GetNVMeDeviceAt")
	return nil, errors.UnsupportedError("GetNVMeDeviceAt is not supported for darwin")
}

// GetNamespaceCountForSubsDevice returns the number of namespaces present in a given subsystem device.
func (s *NVMeSubsystem) GetNamespaceCountForSubsDevice(ctx context.Context) (int, error) {
	Logc(ctx).Debug(">>>> nvme_darwin.GetNamespaceCount")
	defer Logc(ctx).Debug("<<<< nvme_darwin.GetNamespaceCount")
	return 0, errors.UnsupportedError("GetNamespaceCountForSubsDevice is not supported for darwin")
}

// FlushNVMeDevice flushes any ongoing IOs present on the NVMe device.
func (d *NVMeDevice) FlushNVMeDevice(ctx context.Context) error {
	Logc(ctx).Debug(">>>> nvme_darwin.FlushNVMeDevice")
	defer Logc(ctx).Debug("<<<< nvme_darwin.FlushNVMeDevice")
	return errors.UnsupportedError("FlushNVMeDevice is not supported for darwin")
}
