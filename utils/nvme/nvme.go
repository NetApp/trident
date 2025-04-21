// Copyright 2025 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/spf13/afero"
	"go.uber.org/multierr"

	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/utils/devices"
	"github.com/netapp/trident/utils/devices/luks"
	"github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/mount"
)

const NVMeAttachTimeout = 20 * time.Second

var beforeNVMeFlushDevice = fiji.Register("beforeNVMeFlushDevice", "nvme")

func NewNVMeSubsystem(nqn string, command exec.Command, fs afero.Fs) *NVMeSubsystem {
	return NewNVMeSubsystemDetailed(nqn, "", []Path{}, command, fs)
}

func NewNVMeSubsystemDetailed(nqn, name string, paths []Path, command exec.Command,
	osFs afero.Fs,
) *NVMeSubsystem {
	return &NVMeSubsystem{
		NQN:     nqn,
		Name:    name,
		Paths:   paths,
		command: command,
		osFs:    osFs,
	}
}

// updatePaths updates the paths with the current state of the subsystem on the k8s node.
func (s *NVMeSubsystem) updatePaths(ctx context.Context) error {
	// Getting current state of subsystem on the k8s node.
	paths, err := GetNVMeSubsystemPaths(ctx, s.osFs, s.Name)
	if err != nil {
		Logc(ctx).WithField("Error", err).Errorf("Failed to update subsystem paths: %v", err)
		return fmt.Errorf("failed to update subsystem paths: %v", err)
	}

	s.Paths = paths

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
			if err := s.ConnectSubsystemToHost(ctx, LIF); err != nil {
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
	return s.DisconnectSubsystemFromHost(ctx)
}

// GetNamespaceCount returns the number of namespaces mapped to the subsystem.
func (s *NVMeSubsystem) GetNamespaceCount(ctx context.Context) (int, error) {
	credibility := false
	for _, path := range s.Paths {
		if path.State == "live" {
			credibility = true
			break
		}
	}

	if !credibility {
		return 0, fmt.Errorf("nvme paths are down, couldn't get the number of namespaces")
	}

	count, err := s.GetNVMeDeviceCountAt(ctx, s.Name)
	if err != nil {
		Logc(ctx).Errorf("Failed to get namespace count: %v", err)
		return 0, err
	}

	return count, nil
}

func (s *NVMeSubsystem) GetNVMeDevice(ctx context.Context, nsUUID string) (*NVMeDevice, error) {
	Logc(ctx).Debug(">>>> nvme.GetNVMeDevice")
	defer Logc(ctx).Debug("<<<< nvme.GetNVMeDevice")

	devInterface, err := s.GetNVMeDeviceAt(ctx, nsUUID)
	if err != nil {
		Logc(ctx).Errorf("Failed to get NVMe device, %v", err)
		return nil, err
	}

	return devInterface, nil
}

// GetPath returns the device path where we mount the filesystem in NodePublish.
func (d *NVMeDevice) GetPath() string {
	if d == nil {
		return ""
	}
	return d.Device
}

// FlushDevice flushes any ongoing IOs on the device.
func (d *NVMeDevice) FlushDevice(ctx context.Context, ignoreErrors, force bool) error {
	if err := beforeNVMeFlushDevice.Inject(); err != nil {
		return err
	}

	// Force is set in forced detach use case. We don't flush in that scenario and return success.
	if force {
		return nil
	}

	err := d.FlushNVMeDevice(ctx)

	// When ignoreErrors is set, we try to flush but ignore any errors encountered during the flush.
	if ignoreErrors {
		return nil
	}

	return err
}

// IsNil returns true if Device and NamespacePath are not set.
func (d *NVMeDevice) IsNil() bool {
	if d == nil || d.Device == "" {
		return true
	}
	return false
}

// NewNVMeHandler returns the interface to handle NVMe related utils operations.
func NewNVMeHandler() NVMeInterface {
	command := exec.NewCommand()
	devicesClient := devices.New()
	mountClient, _ := mount.New()
	fsClient := filesystem.New(mountClient)
	osFs := afero.NewOsFs()
	return NewNVMeHandlerDetailed(command, devicesClient, mountClient, fsClient, osFs)
}

func NewNVMeHandlerDetailed(command exec.Command, devicesClient devices.Devices,
	mountClient mount.Mount, fsClient filesystem.Filesystem, osFs afero.Fs,
) *NVMeHandler {
	return &NVMeHandler{
		command:       command,
		devicesClient: devicesClient,
		mountClient:   mountClient,
		fsClient:      fsClient,
		osFs:          osFs,
	}
}

// NewNVMeSubsystem returns NVMe subsystem object. Even if a subsystem is not connected to the k8s node,
// this function returns a minimal NVMe subsystem object.
func (nh *NVMeHandler) NewNVMeSubsystem(ctx context.Context, subsNqn string) NVMeSubsystemInterface {
	sub, err := nh.GetNVMeSubsystem(ctx, subsNqn)
	if err != nil {
		Logc(ctx).WithField("Error", err).Warnf("Failed to get subsystem: %v; returning minimal subsystem", err)
		return NewNVMeSubsystem(subsNqn, nh.command, nh.osFs)
	}
	return sub
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

func (nh *NVMeHandler) AttachNVMeVolumeRetry(
	ctx context.Context, name, mountpoint string, publishInfo *models.VolumePublishInfo, secrets map[string]string,
	timeout time.Duration,
) error {
	Logc(ctx).Debug(">>>> nvme.AttachNVMeVolumeRetry")
	defer Logc(ctx).Debug("<<<< nvme.AttachNVMeVolumeRetry")
	var err error

	checkAttachNVMeVolume := func() error {
		return nh.AttachNVMeVolume(ctx, name, mountpoint, publishInfo, secrets)
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

func (nh *NVMeHandler) AttachNVMeVolume(
	ctx context.Context, name, mountpoint string, publishInfo *models.VolumePublishInfo, secrets map[string]string,
) error {
	Logc(ctx).Debug(">>>> nvme.AttachNVMeVolume")
	defer Logc(ctx).Debug("<<<< nvme.AttachNVMeVolume")
	nvmeSubsys := nh.NewNVMeSubsystem(ctx, publishInfo.NVMeSubsystemNQN)
	connectionStatus := nvmeSubsys.GetConnectionStatus()

	if connectionStatus != NVMeSubsystemConnected {
		// connect to the subsystem from this host -> nvme connect call
		if err := nvmeSubsys.Connect(ctx, publishInfo.NVMeTargetIPs, false); err != nil {
			return err
		}
	}

	nvmeDev, err := nvmeSubsys.GetNVMeDevice(ctx, publishInfo.NVMeNamespaceUUID)
	if err != nil {
		return err
	}
	devPath := nvmeDev.GetPath()
	publishInfo.DevicePath = devPath

	if err = nh.NVMeMountVolume(ctx, name, mountpoint, publishInfo, secrets); err != nil {
		return err
	}

	return nil
}

func (nh *NVMeHandler) NVMeMountVolume(
	ctx context.Context, name, mountpoint string, publishInfo *models.VolumePublishInfo, secrets map[string]string,
) error {
	Logc(ctx).Debug(">>>> nvme.NVMeMountVolume")
	defer Logc(ctx).Debug("<<<< nvme.NVMeMountVolume")

	// Initially, the device path raw device path for this NVMe namespace.
	devicePath := publishInfo.DevicePath

	// Format and open a LUKS device if LUKS Encryption is set to true.
	var luksFormatted bool
	var err error
	isLUKSDevice := convert.ToBool(publishInfo.LUKSEncryption)
	if isLUKSDevice {
		luksDevice := luks.NewDevice(devicePath, name, nh.command)
		luksFormatted, err = luksDevice.EnsureDeviceMappedOnHost(ctx, name, secrets)
		if err != nil {
			return err
		}

		devicePath = luksDevice.MappedDevicePath()
	}

	// Fail fast if the device should be a LUKS device but is not LUKS formatted.
	if isLUKSDevice && !luksFormatted {
		Logc(ctx).WithFields(LogFields{
			"devicePath":      publishInfo.DevicePath,
			"luksMapperPath":  devicePath,
			"isLUKSFormatted": luksFormatted,
			"shouldBeLUKS":    isLUKSDevice,
		}).Error("Device should be a LUKS device but is not LUKS formatted.")
		return fmt.Errorf("device should be a LUKS device but is not LUKS formatted")
	}

	// No filesystem work is required for raw block; return early.
	if publishInfo.FilesystemType == filesystem.Raw {
		return nil
	}

	existingFstype, err := nh.devicesClient.GetDeviceFSType(ctx, devicePath)
	if err != nil {
		return err
	}
	if existingFstype == "" {
		if !isLUKSDevice {
			if unformatted, err := nh.devicesClient.IsDeviceUnformatted(ctx, devicePath); err != nil {
				Logc(ctx).WithField(
					"device", devicePath,
				).WithError(err).Error("Unable to identify if the device is not formatted.")
				return err
			} else if !unformatted {
				Logc(ctx).WithField(
					"device", devicePath,
				).WithError(err).Error("Device is not unformatted.")
				return fmt.Errorf("device %v is already formatted", devicePath)
			}
		}

		Logc(ctx).WithFields(LogFields{
			"volume":    name,
			"namespace": publishInfo.NVMeNamespaceUUID,
			"fstype":    publishInfo.FilesystemType,
		}).Debug("Formatting NVMe Namespace.")
		err := nh.fsClient.FormatVolume(ctx, devicePath, publishInfo.FilesystemType, publishInfo.FormatOptions)
		if err != nil {
			return fmt.Errorf("error formatting Namespace %s, device %s: %v", name, devicePath, err)
		}
	} else if existingFstype != filesystem.UnknownFstype && existingFstype != publishInfo.FilesystemType {
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
	mounted, err := nh.mountClient.IsMounted(ctx, devicePath, "", "")
	if err != nil {
		return err
	}
	if !mounted {
		nh.fsClient.RepairVolume(ctx, devicePath, publishInfo.FilesystemType)
	}

	// Optionally mount the device
	if mountpoint != "" {
		if err := nh.mountClient.MountDevice(ctx, devicePath, mountpoint, publishInfo.MountOptions, false); err != nil {
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
func (nh *NVMeHandler) AddPublishedNVMeSession(pubSessions *NVMeSessions, publishInfo *models.VolumePublishInfo) {
	if pubSessions == nil {
		return
	}

	pubSessions.AddNVMeSession(*NewNVMeSubsystem(publishInfo.NVMeSubsystemNQN, nh.command, nh.osFs),
		publishInfo.NVMeTargetIPs)
	pubSessions.AddNamespaceToSession(publishInfo.NVMeSubsystemNQN, publishInfo.NVMeNamespaceUUID)
}

// RemovePublishedNVMeSession deletes the namespace from the published NVMeSession. If the number of namespaces
// associated with a session comes down to 0, we delete that session and send a disconnect signal for that subsystem.
func (nh *NVMeHandler) RemovePublishedNVMeSession(pubSessions *NVMeSessions, subNQN, nsUUID string) bool {
	disconnect := false
	if pubSessions == nil {
		return disconnect
	}

	pubSessions.RemoveNamespaceFromSession(subNQN, nsUUID)
	if pubSessions.GetNamespaceCountForSession(subNQN) < 1 {
		disconnect = true
		pubSessions.RemoveNVMeSession(subNQN)
	}

	return disconnect
}

// PopulateCurrentNVMeSessions populates the given session map with the current session present on the k8s node. This
// is done by listing the existing NVMe namespaces using the nvme cli command.
func (nh *NVMeHandler) PopulateCurrentNVMeSessions(ctx context.Context, currSessions *NVMeSessions) error {
	if currSessions == nil {
		return fmt.Errorf("current NVMeSessions not initialized")
	}

	// Get the list of the subsystems currently present on the k8s node.
	subs, err := nh.listSubsystemsFromSysFs(ctx)
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
func (nh *NVMeHandler) InspectNVMeSessions(
	ctx context.Context, pubSessions, currSessions *NVMeSessions,
) []NVMeSubsystem {
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
func (nh *NVMeHandler) RectifyNVMeSession(
	ctx context.Context, subsystemToFix NVMeSubsystem, pubSessions *NVMeSessions,
) {
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
