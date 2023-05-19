// Copyright 2023 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"go.uber.org/multierr"

	. "github.com/netapp/trident/logging"
)

// getNVMeSubsystem returns the NVMe subsystem details.
func getNVMeSubsystem(ctx context.Context, subsysNqn string) (*NVMeSubsystem, error) {
	Logc(ctx).Debug(">>>> nvme.getNVMeSubsystem")
	defer Logc(ctx).Debug("<<<< nvme.getNVMeSubsystem")

	subsys, err := GetNVMeSubsystemList(ctx)
	if err != nil {
		Logc(ctx).WithField("Error", err).Errorf("Failed to get subsystem list: %v", err)
		return nil, err
	}

	// Getting current subsystem details.
	for _, sub := range subsys.Subsystems {
		if sub.NQN == subsysNqn {
			return &sub, nil
		}
	}

	return nil, fmt.Errorf("couldn't find subsystem %s", subsysNqn)
}

// updatePaths updates the paths with the current state of the subsystem on the k8s node.
func (s *NVMeSubsystem) updatePaths(ctx context.Context) error {
	// Getting current state of subsystem on the k8s node.
	sub, err := getNVMeSubsystem(ctx, s.NQN)
	if err != nil {
		Logc(ctx).WithField("Error", err).Errorf("Failed to update subsystem paths: %v", err)
		return fmt.Errorf("failed to update subsystem paths: %v", err)
	}

	s.Paths = sub.Paths

	return nil
}

// GetConnectionStatus checks if subsystem is connected to the k8s node.
func (s *NVMeSubsystem) GetConnectionStatus() NVMeSubsystemConnectionStatus {
	if len(s.Paths) == MaxSessionsPerSubsystem {
		return NVMeSubsystemConnected
	}

	if len(s.Paths) > 0 {
		return NVMeSubsystemPartiallyConnected
	}

	return NVMeSubsystemDisconnected
}

// isLIFPresent checks if there is a path present in the subsystem corresponding to the LIF.
func (s *NVMeSubsystem) isLIFPresent(ip string) bool {
	for _, path := range s.Paths {
		if extractNVMeLIF(path.Address) == ip {
			return true
		}
	}

	return false
}

// Connect creates paths corresponding to all the targetIPs for the subsystem
// and updates the in-memory subsystem path details.
func (s *NVMeSubsystem) Connect(ctx context.Context, nvmeTargetIps []string) error {
	updatePaths := false
	var errors error

	for _, LIF := range nvmeTargetIps {
		if !s.isLIFPresent(LIF) {
			if err := ConnectSubsystemToHost(ctx, s.NQN, LIF); err != nil {
				errors = multierr.Append(errors, err)
			} else {
				updatePaths = true
			}
		}
	}

	// updatePaths == true indicates that at least one path/connection was created for the subsystem.
	// So we ignore the errors we encountered while creating the other error paths.
	if updatePaths {
		// We are here because one of the `nvme connect` command succeeded. If updating the subsystem fails
		// for some reason, we return error as all the subsequent operations are dependent on this update.
		if err := s.updatePaths(ctx); err != nil {
			return err
		}
	} else {
		return errors
	}

	return nil
}

// Disconnect removes the subsystem and its corresponding paths/sessions from the k8s node.
func (s *NVMeSubsystem) Disconnect(ctx context.Context) error {
	return DisconnectSubsystemFromHost(ctx, s.NQN)
}

// GetNamespaceCount returns the number of namespaces mapped to the subsystem.
func (s *NVMeSubsystem) GetNamespaceCount(ctx context.Context) (int, error) {
	for _, path := range s.Paths {
		count, err := GetNamespaceCountForSubsDevice(ctx, "/dev/"+path.Name)
		if err != nil {
			Logc(ctx).WithField("Error", err).Errorf("Failed to get namespace count: %v", err)
			return 0, fmt.Errorf("failed to get namespace count: %v", err)
		}

		return count, nil
	}

	// Couldn't find any sessions, so no namespaces are attached to this subsystem
	return 0, nil
}

// getNVMeDevice returns the NVMe device corresponding to nsPath namespace.
func getNVMeDevice(ctx context.Context, nsPath string) (*NVMeDevice, error) {
	Logc(ctx).Debug(">>>> nvme.getNVMeDevice")
	defer Logc(ctx).Debug("<<<< nvme.getNVMeDevice")

	dList, err := GetNVMeDeviceList(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %v", err)
	}

	for _, dev := range dList.Devices {
		if dev.NamespacePath == nsPath {
			return &dev, nil
		}
	}

	return nil, fmt.Errorf("no device found for the given namespace")
}

// GetPath returns the device path where we mount the filesystem in NodePublish.
func (d *NVMeDevice) GetPath() string {
	return d.Device
}

// FlushDevice flushes any ongoing IOs on the device.
func (d *NVMeDevice) FlushDevice(ctx context.Context) error {
	return FlushNVMeDevice(ctx, d.Device)
}

// NewNVMeHandler returns the interface to handle NVMe related utils operations.
func NewNVMeHandler() NVMeInterface {
	return &NVMeHandler{}
}

// NewNVMeDevice returns new NVMe device
func (nh *NVMeHandler) NewNVMeDevice(ctx context.Context, nsPath string) (NVMeDeviceInterface, error) {
	return getNVMeDevice(ctx, nsPath)
}

// NewNVMeSubsystem returns NVMe subsystem object. Even if a subsystem is not connected to the k8s node,
// this function returns a minimal NVMe subsystem object.
func (nh *NVMeHandler) NewNVMeSubsystem(ctx context.Context, subsNqn string) NVMeSubsystemInterface {
	sub, err := getNVMeSubsystem(ctx, subsNqn)
	if err != nil {
		Logc(ctx).WithField("Error", err).Warnf("Failed to get subsystem: %v; returning minimal subsystem", err)
		return &NVMeSubsystem{NQN: subsNqn}
	}
	return sub
}

// GetHostNqn returns the NQN of the k8s node.
func (nh *NVMeHandler) GetHostNqn(ctx context.Context) (string, error) {
	return GetHostNqn(ctx)
}

func (nh *NVMeHandler) NVMeActiveOnHost(ctx context.Context) (bool, error) {
	return NVMeActiveOnHost(ctx)
}

// extractNVMeLIF extracts IP address from NVMeSubsystem's Path.Address field.
func extractNVMeLIF(a string) string {
	after := strings.TrimPrefix(a, TransportAddressEqualTo)
	// Address contents in Rhel - "traddr=10.193.108.74,trsvcid=4420".
	before, _, _ := strings.Cut(after, ",")
	// Address contents in Ubuntu - "traddr=10.193.108.74 trsvcid=4420".
	before, _, _ = strings.Cut(before, " ")

	return before
}

func AttachNVMeVolumeRetry(
	ctx context.Context, name, mountpoint string, publishInfo *VolumePublishInfo, secrets map[string]string, timeout time.Duration,
) error {
	Logc(ctx).Debug(">>>> nvme.AttachNVMeVolumeRetry")
	defer Logc(ctx).Debug("<<<< nvme.AttachNVMeVolumeRetry")
	var err error

	checkAttachNVMeVolume := func() error {
		return AttachNVMeVolume(ctx, name, mountpoint, publishInfo, secrets)
	}

	attachNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"increment": duration,
			"error":     err,
		}).Debug("Attach NVMe volume is not complete, waiting.")
	}

	attachBackoff := backoff.NewExponentialBackOff()
	attachBackoff.InitialInterval = 1 * time.Second
	attachBackoff.Multiplier = 1.414 // approx sqrt(2)
	attachBackoff.RandomizationFactor = 0.1
	attachBackoff.MaxElapsedTime = timeout

	err = backoff.RetryNotify(checkAttachNVMeVolume, attachBackoff, attachNotify)
	return err
}

func AttachNVMeVolume(ctx context.Context, name, mountpoint string, publishInfo *VolumePublishInfo, secrets map[string]string) error {
	nvmeHandler := NewNVMeHandler()
	nvmeSubsys := nvmeHandler.NewNVMeSubsystem(ctx, publishInfo.NVMeSubsystemNQN)
	connectionStatus := nvmeSubsys.GetConnectionStatus()

	if connectionStatus != NVMeSubsystemConnected {
		// connect to the subsystem from this host -> nvme connect call
		if err := nvmeSubsys.Connect(ctx, publishInfo.NVMeTargetIPs); err != nil {
			return err
		}
	}

	nvmeDev, err := nvmeHandler.NewNVMeDevice(ctx, publishInfo.NVMeNamespacePath)
	if err != nil {
		return err
	}
	devPath := nvmeDev.GetPath()
	publishInfo.DevicePath = devPath

	if err = NVMeMountVolume(ctx, name, mountpoint, publishInfo); err != nil {
		return err
	}

	return nil
}

func NVMeMountVolume(ctx context.Context, name, mountpoint string, publishInfo *VolumePublishInfo) error {
	if publishInfo.FilesystemType == fsRaw {
		return nil
	}
	devicePath := publishInfo.DevicePath

	existingFstype, err := getDeviceFSType(ctx, devicePath)
	if err != nil {
		return err
	}
	if existingFstype == "" {
		if unformatted, err := isDeviceUnformatted(ctx, devicePath); err != nil {
			Logc(ctx).WithField("device",
				devicePath).Errorf("Unable to identify if the device is not formatted; err: %v", err)
			return err
		} else if !unformatted {
			Logc(ctx).WithField("device", devicePath).Errorf("Device is not not formatted; err: %v", err)
			return fmt.Errorf("device %v is not unformatted", devicePath)
		}
		Logc(ctx).WithFields(LogFields{"volume": name, "fstype": publishInfo.FilesystemType}).Debug("Formatting LUN.")
		err := formatVolume(ctx, devicePath, publishInfo.FilesystemType)
		if err != nil {
			return fmt.Errorf("error formatting Namespace %s, device %s: %v", name, devicePath, err)
		}
	} else if existingFstype != unknownFstype && existingFstype != publishInfo.FilesystemType {
		Logc(ctx).WithFields(LogFields{
			"volume":          name,
			"existingFstype":  existingFstype,
			"requestedFstype": publishInfo.FilesystemType,
		}).Error("Namespace already formatted with a different file system type.")
		return fmt.Errorf("namespace %s, device %s already formatted with other filesystem: %s",
			name, devicePath, existingFstype)
	} else {
		Logc(ctx).WithFields(LogFields{
			"volume": name,
			"fstype": existingFstype,
		}).Debug("Namespace already formatted.")
	}

	// Attempt to resolve any filesystem inconsistencies that might be due to dirty node shutdowns, cloning
	// in-use volumes, or creating volumes from snapshots taken from in-use volumes.  This is only safe to do
	// if a device is not mounted.  The fsck command returns a non-zero exit code if filesystem errors are found,
	// even if they are completely and automatically fixed, so we don't return any error here.
	mounted, err := IsMounted(ctx, devicePath, "", "")
	if err != nil {
		return err
	}
	if !mounted {
		_ = repairVolume(ctx, devicePath, publishInfo.FilesystemType)
	}

	// Optionally mount the device
	if mountpoint != "" {
		if err := MountDevice(ctx, devicePath, mountpoint, publishInfo.MountOptions, false); err != nil {
			return fmt.Errorf("error mounting Namespace %v, device %v, mountpoint %v; %s",
				name, devicePath, mountpoint, err)
		}
	}

	return nil
}
