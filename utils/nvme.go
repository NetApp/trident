// Copyright 2023 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"go.uber.org/multierr"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

const NVMeAttachTimeout = 20 * time.Second

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

// IsNetworkPathPresent checks if there is a path present in the subsystem corresponding to the LIF.
func (s *NVMeSubsystem) IsNetworkPathPresent(ip string) bool {
	for _, path := range s.Paths {
		if extractIPFromNVMeAddress(path.Address) == ip {
			return true
		}
	}

	return false
}

// Connect creates paths corresponding to all the targetIPs for the subsystem
// and updates the in-memory subsystem path details.
func (s *NVMeSubsystem) Connect(ctx context.Context, nvmeTargetIps []string, connectOnly bool) error {
	updatePaths := false
	var connectErrors error

	for _, LIF := range nvmeTargetIps {
		if !s.IsNetworkPathPresent(LIF) {
			if err := ConnectSubsystemToHost(ctx, s.NQN, LIF); err != nil {
				connectErrors = multierr.Append(connectErrors, err)
			} else {
				updatePaths = true
			}
		}
	}

	// We set connectOnly true only when we connect in NVMe self-healing thread. We don't want to update any in-memory
	// structures in that case.
	if connectOnly {
		return connectErrors
	}

	// updatePaths == true indicates that at least one path/connection was created for the subsystem.
	// So we ignore the errors we encountered while creating the other error paths.
	if updatePaths {
		// We are here because one of the `nvme connect` command succeeded. If updating the subsystem fails
		// for some reason, we return error as all the subsequent operations are dependent on this update.
		if err := s.updatePaths(ctx); err != nil {
			return err
		}

		return nil
	}

	if s.GetConnectionStatus() != NVMeSubsystemDisconnected {
		// We can arrive in this case for the below use case -
		// LIF1 is up and LIF2 is down. After creating first pod, we connect a path for subsystem using
		// LIF1 successfully while for LIF2 it fails. So updatePaths will update the new path for the
		// subsystem. When we create the second pod, we don't create a path for LIF1 as it is already
		// present while we try to create for LIF2 which fails again. So updatePaths will remain false.
		// Since at least one path is present, we should not return error.
		return nil
	}

	return connectErrors
}

// Disconnect removes the subsystem and its corresponding paths/sessions from the k8s node.
func (s *NVMeSubsystem) Disconnect(ctx context.Context) error {
	return DisconnectSubsystemFromHost(ctx, s.NQN)
}

// GetNamespaceCount returns the number of namespaces mapped to the subsystem.
func (s *NVMeSubsystem) GetNamespaceCount(ctx context.Context) (int, error) {
	var combinedError error

	for _, path := range s.Paths {
		count, err := GetNamespaceCountForSubsDevice(ctx, "/dev/"+path.Name)
		if err != nil {
			Logc(ctx).WithField("Error", err).Warnf("Failed to get namespace count: %v", err)
			combinedError = multierr.Append(combinedError, err)
			continue
		}

		return count, nil
	}

	// Couldn't find any sessions, so no namespaces are attached to this subsystem.
	// But if there was error getting the number of namespaces from all the paths, return error.
	return 0, combinedError
}

// getNVMeDevice returns the NVMe device corresponding to nsPath namespace.
func getNVMeDevice(ctx context.Context, nsUUID string) (*NVMeDevice, error) {
	Logc(ctx).Debug(">>>> nvme.getNVMeDevice")
	defer Logc(ctx).Debug("<<<< nvme.getNVMeDevice")

	dList, err := GetNVMeDeviceList(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %v", err)
	}

	for _, dev := range dList.Devices {
		if dev.UUID == nsUUID {
			Logc(ctx).Debugf("Device found: %v.", dev)
			return &dev, nil
		}
	}

	Logc(ctx).WithField("nsUUID", nsUUID).Debug("No device found for this Namespace.")
	return nil, errors.NotFoundError("no device found for the given namespace %v", nsUUID)
}

// GetPath returns the device path where we mount the filesystem in NodePublish.
func (d *NVMeDevice) GetPath() string {
	if d == nil {
		return ""
	}
	return d.Device
}

// FlushDevice flushes any ongoing IOs on the device.
func (d *NVMeDevice) FlushDevice(ctx context.Context) error {
	return FlushNVMeDevice(ctx, d.GetPath())
}

// IsNil returns true if Device and NamespacePath are not set.
func (d *NVMeDevice) IsNil() bool {
	if d == nil || (d.Device == "" && d.NamespacePath == "") {
		return true
	}
	return false
}

// NewNVMeHandler returns the interface to handle NVMe related utils operations.
func NewNVMeHandler() NVMeInterface {
	return &NVMeHandler{}
}

// NewNVMeDevice returns new NVMe device
func (nh *NVMeHandler) NewNVMeDevice(ctx context.Context, nsUUID string) (NVMeDeviceInterface, error) {
	return getNVMeDevice(ctx, nsUUID)
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

// extractIPFromNVMeAddress extracts IP address from NVMeSubsystem's Path.Address field.
func extractIPFromNVMeAddress(a string) string {
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
		if err := nvmeSubsys.Connect(ctx, publishInfo.NVMeTargetIPs, false); err != nil {
			return err
		}
	}

	nvmeDev, err := nvmeHandler.NewNVMeDevice(ctx, publishInfo.NVMeNamespaceUUID)
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
		err = repairVolume(ctx, devicePath, publishInfo.FilesystemType)
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				logFields := LogFields{
					"volume": name,
					"fstype": publishInfo.FilesystemType,
					"device": devicePath,
				}

				if exitErr.ExitCode() == 1 {
					Logc(ctx).WithFields(logFields).Info("Fixed filesystem errors")
				} else {
					logFields["exitCode"] = exitErr.ExitCode()
					Logc(ctx).WithError(err).WithFields(logFields).Error("Failed to repair filesystem errors.")
				}
			}
		}
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

// Self-healing functions

// NewNVMeSessionData returns NVMeSessionData object with the specified subsystem, targetIPs and default values.
func NewNVMeSessionData(subsystem NVMeSubsystem, targetIPs []string) *NVMeSessionData {
	return &NVMeSessionData{
		Subsystem:      subsystem,
		NVMeTargetIPs:  targetIPs,
		LastAccessTime: time.Now(),
		Remediation:    NoOp,
	}
}

// SetRemediation updates the remediation value.
func (sd *NVMeSessionData) SetRemediation(op NVMeOperation) {
	sd.Remediation = op
}

func (sd *NVMeSessionData) IsTargetIPPresent(ip string) bool {
	for _, existingIP := range sd.NVMeTargetIPs {
		if existingIP == ip {
			return true
		}
	}
	return false
}

func (sd *NVMeSessionData) AddTargetIP(ip string) {
	sd.NVMeTargetIPs = append(sd.NVMeTargetIPs, ip)
}

// NewNVMeSessions initializes and returns empty NVMeSession object pointer.
func NewNVMeSessions() *NVMeSessions {
	return &NVMeSessions{Info: make(map[string]*NVMeSessionData)}
}

// AddNVMeSession adds a new NVMeSession to the session map.
func (s *NVMeSessions) AddNVMeSession(subsystem NVMeSubsystem, targetIPs []string) {
	if s.Info == nil {
		s.Info = make(map[string]*NVMeSessionData)
	}

	sd, ok := s.Info[subsystem.NQN]
	if !ok {
		s.Info[subsystem.NQN] = NewNVMeSessionData(subsystem, targetIPs)
		return
	}

	// NVMeSession Already present. We won't reach here in case of PopulateCurrentNVMeSessions as all the sessions are
	// unique in the case; unlike AddPublishedNVMeSession. Check if NVMeTargetIPs present in the existing session data
	// match with the targetIPs sent in this function and update any missing IP.
	for _, ip := range targetIPs {
		if !sd.IsTargetIPPresent(ip) {
			sd.AddTargetIP(ip)
		}
	}
}

// AddNamespaceToSession adds the Namespace UUID to the list of Namespaces for session.
func (s *NVMeSessions) AddNamespaceToSession(subNQN, nsUUID string) bool {
	if s == nil || s.IsEmpty() {
		return false
	}

	session, ok := s.Info[subNQN]
	if !ok {
		// No session found for given subsystem.
		return false
	}
	// add namespace to the session
	if session.Namespaces == nil {
		session.Namespaces = make(map[string]bool)
	}
	session.Namespaces[nsUUID] = true
	return true
}

// RemoveNamespaceFromSession Removes the Namespace UUID from the list of Namespaces for session.
func (s *NVMeSessions) RemoveNamespaceFromSession(subNQN, nsUUID string) {
	if s == nil || s.IsEmpty() {
		return
	}

	session, ok := s.Info[subNQN]
	if !ok {
		// No session found for given subsystem.
		return
	}
	// Remove namespace from the list to the session
	delete(session.Namespaces, nsUUID)
	return
}

// GetNamespaceCountForSession Gets the number of Namespaces associated with the given session.
func (s *NVMeSessions) GetNamespaceCountForSession(subNQN string) int {
	if s == nil || s.IsEmpty() {
		return 0
	}

	session, ok := s.Info[subNQN]
	if !ok {
		// No session found for given subsystem.
		return 0
	}

	return len(session.Namespaces)
}

// RemoveNVMeSession deletes NVMeSession corresponding to a given subsystem NQN.
func (s *NVMeSessions) RemoveNVMeSession(subNQN string) {
	if !s.IsEmpty() {
		delete(s.Info, subNQN)
	}
}

// CheckNVMeSessionExists queries if a particular NVMeSession is present using the subsystem NQN.
func (s *NVMeSessions) CheckNVMeSessionExists(subNQN string) bool {
	if s.IsEmpty() {
		return false
	}

	_, ok := s.Info[subNQN]
	return ok
}

// IsEmpty checks if the subsystem map is empty.
func (s *NVMeSessions) IsEmpty() bool {
	if s.Info == nil || len(s.Info) == 0 {
		return true
	}

	return false
}

// ResetRemediationForAll updates the remediation for all the NVMeSessions to NoOp.
func (s *NVMeSessions) ResetRemediationForAll() {
	for _, sData := range s.Info {
		sData.Remediation = NoOp
	}
}

// AddPublishedNVMeSession adds the published NVMeSession to the given session map.
func (nh *NVMeHandler) AddPublishedNVMeSession(pubSessions *NVMeSessions, publishInfo *VolumePublishInfo) {
	if pubSessions == nil {
		return
	}

	pubSessions.AddNVMeSession(NVMeSubsystem{NQN: publishInfo.NVMeSubsystemNQN}, publishInfo.NVMeTargetIPs)
	pubSessions.AddNamespaceToSession(publishInfo.NVMeSubsystemNQN, publishInfo.NVMeNamespaceUUID)
}

// RemovePublishedNVMeSession deletes the  published NVMeSession from the given session map.
func (nh *NVMeHandler) RemovePublishedNVMeSession(pubSessions *NVMeSessions, subNQN, nsUUID string) {
	if pubSessions == nil {
		return
	}

	pubSessions.RemoveNamespaceFromSession(subNQN, nsUUID)
	if pubSessions.GetNamespaceCountForSession(subNQN) < 1 {
		pubSessions.RemoveNVMeSession(subNQN)
	}
}

// PopulateCurrentNVMeSessions populates the given session map with the current session present on the k8s node. This
// is done by listing the existing NVMe namespaces using the nvme cli command.
func (nh *NVMeHandler) PopulateCurrentNVMeSessions(ctx context.Context, currSessions *NVMeSessions) error {
	if currSessions == nil {
		return fmt.Errorf("current NVMeSessions not initialized")
	}

	// Get the list of the subsystems currently present on the k8s node.
	subs, err := GetNVMeSubsystemList(ctx)
	if err != nil {
		Logc(ctx).WithField("Error", err).Errorf("Failed to get subsystem list: %v", err)
		return err
	}

	Logc(ctx).Debug("Populating current NVMe sessions.")
	for index := range subs.Subsystems {
		currSessions.AddNVMeSession(subs.Subsystems[index], []string{})
	}

	return nil
}

// SortSubsystemsUsingSessions sorts the subsystems in accordance to the LastAccessTime present in the NVMeSessionData.
func SortSubsystemsUsingSessions(subs []NVMeSubsystem, pubSessions *NVMeSessions) {
	if pubSessions == nil || pubSessions.IsEmpty() {
		return
	}

	if len(subs) > 1 {
		sort.Slice(subs, func(i, j int) bool {
			iSub := subs[i]
			jSub := subs[j]

			iLastAccessTime := pubSessions.Info[iSub.NQN].LastAccessTime
			jLastAccessTime := pubSessions.Info[jSub.NQN].LastAccessTime

			return iLastAccessTime.Before(jLastAccessTime)
		})
	}
}

// InspectNVMeSessions compares and checks if the current sessions are in-line with the published sessions. If any
// subsystem is missing any path in the current session (as compared with the published session), then it is added in
// the subsToFix array for healing.
func (nh *NVMeHandler) InspectNVMeSessions(ctx context.Context, pubSessions, currSessions *NVMeSessions) []NVMeSubsystem {
	var subsToFix []NVMeSubsystem
	if pubSessions == nil || pubSessions.IsEmpty() {
		return subsToFix
	}

	for pubNQN, pubSessionData := range pubSessions.Info {
		if pubSessionData == nil {
			continue
		}

		if currSessions == nil || !currSessions.CheckNVMeSessionExists(pubNQN) {
			Logc(ctx).Warnf("Published nqn %s not present in current sessions.", pubNQN)
			continue
		}

		// Published session equivalent is present in the list of current sessions.
		currSessionData := currSessions.Info[pubNQN]
		if currSessionData == nil {
			continue
		}

		switch currSessionData.Subsystem.GetConnectionStatus() {
		case NVMeSubsystemDisconnected:
			// We can reconnect with all the subsystem paths in this case, but that leads to change in the NVMe device
			// names which are already mounted. So, we don't do self-healing in this case. It is better that the Admin
			// takes care of this issue and then restart the node.
			Logc(ctx).Warnf("All the paths to %s subsystem are down. Please check the network connectivity to"+
				" these IPs %v.", pubNQN, pubSessionData.NVMeTargetIPs)
		case NVMeSubsystemPartiallyConnected:
			// At least one path of the subsystem is connected. We should try to connect with other remaining paths.
			pubSessionData.SetRemediation(ConnectOp)
			subsToFix = append(subsToFix, currSessionData.Subsystem)
			continue
		}

		// All/None of the paths are present for the subsystem
		pubSessionData.LastAccessTime = time.Now()
	}

	SortSubsystemsUsingSessions(subsToFix, pubSessions)
	return subsToFix
}

// RectifyNVMeSession applies the required remediation on the subsystemToFix to make it working again.
func (nh *NVMeHandler) RectifyNVMeSession(ctx context.Context, subsystemToFix NVMeSubsystem, pubSessions *NVMeSessions) {
	if pubSessions == nil || pubSessions.IsEmpty() {
		return
	}

	pubSessionData := pubSessions.Info[subsystemToFix.NQN]
	if pubSessionData == nil {
		return
	}

	// Updating the access time as we are trying to do some NVMeOperation on this subsystem.
	pubSessionData.LastAccessTime = time.Now()

	if pubSessionData.Remediation == ConnectOp {
		if err := subsystemToFix.Connect(ctx, pubSessionData.NVMeTargetIPs, true); err != nil {
			Logc(ctx).Errorf("NVMe Self healing failed for subsystem %s; %v", subsystemToFix.NQN, err)
		} else {
			Logc(ctx).Infof("NVMe Self healing succeeded for %s", subsystemToFix.NQN)
		}
	}
}
