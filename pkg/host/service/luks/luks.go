// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package luks provides a protocol agnostic LUKS device service layer.
// It composes multiple host adapters to manage the lifecycle of LUKS2
// devices.
//
// Higher transport protocols such as iSCSI, NVMe, and FCP drivers may
// consume this package indiscriminately.
package luks

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/host/adapter/block"
	"github.com/netapp/trident/pkg/host/adapter/cryptsetup"
	"github.com/netapp/trident/utils/errors"
)

const (
	MetadataSize = 18874368

	defaultMapperNamePrefix = "luks-"

	waitForDevicePollInterval  = 1 * time.Second
	waitForDevicePollTimeout   = 10 * time.Second
	waitForRemovalPollInterval = 1 * time.Second
	waitForRemovalPollTimeout  = 10 * time.Second
)

var (
	// ErrEmptyPassphrase is returned when the current LUKS passphrase is empty.
	ErrEmptyPassphrase = errors.New("LUKS passphrase cannot be empty")
	// ErrEmptyPassphraseName is returned when the current LUKS passphrase name is empty.
	ErrEmptyPassphraseName = errors.New("LUKS passphrase name cannot be empty")
)

type (
	FormatOptions = cryptsetup.FormatOptions
	OpenOptions   = cryptsetup.OpenOptions
)

// Secret holds the name and passphrase for a LUKS secret.
type Secret struct {
	Name       string
	Passphrase string
}

func (s *Secret) PassphraseName() string {
	return s.Name
}

func (s *Secret) PassphraseValue() string {
	return s.Passphrase
}

// Well-known key names used in the CSI secrets map.
const (
	SecretKeyPassphraseName         = "luks-passphrase-name"          //nolint:gosec // G101: map keys, not secret material
	SecretKeyPassphrase             = "luks-passphrase"               //nolint:gosec // G101: map keys, not secret material
	SecretKeyPreviousPassphraseName = "previous-luks-passphrase-name" //nolint:gosec // G101: map keys, not secret material
	SecretKeyPreviousPassphrase     = "previous-luks-passphrase"      //nolint:gosec // G101: map keys, not secret material
)

type EnsureDeviceOptions struct {
	FormatOptions
	OpenOptions
	// VolumeID is the CSI volume ID (the Trident volume name, e.g. "pvc-..."). The passphrase name is recorded
	// against it (via Helper) before a first format. The mapper is named from the internal ID instead.
	VolumeID string
}

func (e *EnsureDeviceOptions) Validate() error {
	if e == nil {
		return fmt.Errorf("options must not be nil")
	}
	if e.FormatOptions == (FormatOptions{}) {
		e.FormatOptions = cryptsetup.DefaultFormatOptions()
	}
	if e.OpenOptions == (OpenOptions{}) {
		e.OpenOptions = cryptsetup.DefaultOpenOptions()
	}
	return nil
}

func DefaultEnsureDeviceOptions() *EnsureDeviceOptions {
	return &EnsureDeviceOptions{
		FormatOptions: cryptsetup.DefaultFormatOptions(),
		OpenOptions:   cryptsetup.DefaultOpenOptions(),
	}
}

type EnsurePassphraseOptions struct {
	ForceUpdate bool
	// VolumeID is the CSI volume ID (the Trident volume name, e.g. "pvc-..."). The passphrase names are recorded
	// against it (via Helper). If empty, the internal ID is used.
	VolumeID string
}

func DefaultEnsurePassphraseOptions() *EnsurePassphraseOptions {
	return &EnsurePassphraseOptions{
		ForceUpdate: false,
	}
}

// Helper updates the LUKS passphrase names recorded in the Trident controller over the node's
// back-channel REST API after a passphrase rotation.
//
// TODO: Decide whether services should call the back-channel REST API at all, or whether the frontend
// should drive the rotation through the driver. Revisit when the Trident node talks to the K8s API directly.
type Helper interface {
	UpdateVolumeLUKSPassphraseNames(ctx context.Context, volume string, passphraseNames []string) error
}

// helper is safe default behavior.
type helper struct{}

func (h *helper) UpdateVolumeLUKSPassphraseNames(_ context.Context, _ string, _ []string) error {
	return nil
}

// Service defines the LUKS device lifecycle API.
//
// The service owns the full LUKS state machine (format → open → resolve)
// so that every protocol driver (iSCSI, NVMe, FCP) gets identical
// behaviour from a single EnsureDevice call.
type Service interface {
	// PreChecks verifies that cryptsetup is installed and reachable.
	// This allows Trident to fail attachments before any attaching anything for the volume.
	//
	// Returns an error if cryptsetup is not installed or reachable.
	PreChecks(ctx context.Context) error

	// DeviceByUUID returns a view of a LUKS device.
	// This Device is not a handle for operations on the host.
	DeviceByUUID(ctx context.Context, uuid string) (*Device, error)

	// DeviceByInternalID returns a view of the open LUKS device for a volume, found by the mapper name
	// derived from its internal volume ID. It returns a NotFoundError if no such mapper is open.
	// This Device is not a handle for operations on the host.
	DeviceByInternalID(ctx context.Context, internalVolumeID string) (*Device, error)

	// EnsureDevice ensures a LUKS device is formatted, open, and
	// resolvable on the host.
	// It is an idempotent pipeline: format → open → resolve.
	// The internalID parameter should be an internal ID for the volume.
	//
	// The returned Device reports IsFreshFormat when this call ran luksFormat, so the caller
	// can skip probing the new mapper, whose contents are random, for a filesystem.
	//
	// Returns a *Device embedding the block.Device plus LUKS-specific metadata.
	//
	// Partial-format cleanup is handled internally; callers never see
	// PartialFormatError.
	EnsureDevice(
		ctx context.Context, internalVolumeID, devicePath string, current, previous Secret, opts *EnsureDeviceOptions,
	) (*Device, error)

	// EnsureClose ensures a LUKS device is closed on the host.
	EnsureClose(ctx context.Context, dmUUID string, force bool) error

	// EnsureResize ensures the LUKS device is resized to the desired size.
	//
	// Returns an error if the LUKS device cannot be found, resized or the passphrase is invalid.
	EnsureResize(ctx context.Context, dmUUID string, secret Secret) error

	ExtractSecrets(secrets map[string]string) (current, previous Secret)

	EnsurePassphrase(ctx context.Context, volume, rawDevicePath string, current, previous Secret, opts *EnsurePassphraseOptions) error
}

type service struct {
	block  block.Adapter
	crypt  cryptsetup.Adapter
	helper Helper
}

type Option func(*service)

func WithBlockAdapter(b block.Adapter) Option      { return func(s *service) { s.block = b } }
func WithCryptAdapter(c cryptsetup.Adapter) Option { return func(s *service) { s.crypt = c } }
func WithHelper(h Helper) Option                   { return func(s *service) { s.helper = h } }

func New(opts ...Option) Service {
	s := &service{
		block:  block.New(),
		crypt:  cryptsetup.New(),
		helper: &helper{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// PreChecks verifies that cryptsetup is installed and reachable.
func (s *service) PreChecks(ctx context.Context) error {
	Logc(ctx).Debug(">>>> luks.PreChecks")
	defer Logc(ctx).Debug("<<<< luks.PreChecks")

	if _, err := s.crypt.Version(ctx); err != nil {
		return fmt.Errorf("cryptsetup not available: %w", err)
	}
	return nil
}

func (s *service) DeviceByUUID(
	ctx context.Context, uuid string,
) (*Device, error) {
	fields := LogFields{"deviceUUID": uuid}
	Logc(ctx).WithFields(fields).Debug(">>>> luks.DeviceByUUID")
	defer Logc(ctx).WithFields(fields).Debug("<<<< luks.DeviceByUUID")

	if uuid == "" {
		return nil, fmt.Errorf("device uuid is empty")
	}
	blockDevice, err := s.block.DeviceByDMUUID(ctx, uuid)
	if err != nil {
		if block.IsDeviceNotFoundError(err) {
			Logc(ctx).WithFields(fields).Info("LUKS dm device not found.")
			return nil, errors.NotFoundError(err.Error())
		}
		return nil, err
	}
	return newDevice(blockDevice, blockDevice.DMInfo()), nil
}

func (s *service) DeviceByInternalID(ctx context.Context, internalVolumeID string) (*Device, error) {
	mapperName := constructMapperName(internalVolumeID)
	fields := LogFields{"mapperName": mapperName, "internalVolumeID": internalVolumeID}
	Logc(ctx).WithFields(fields).Debug(">>>> luks.DeviceByInternalID")
	defer Logc(ctx).WithFields(fields).Debug("<<<< luks.DeviceByInternalID")

	if internalVolumeID == "" {
		return nil, fmt.Errorf("internal volume ID is empty")
	}
	blockDevice, err := s.block.DeviceByDMName(ctx, mapperName)
	if err != nil {
		if block.IsDeviceNotFoundError(err) {
			Logc(ctx).WithFields(fields).Info("LUKS mapper not found.")
			return nil, errors.NotFoundError(err.Error())
		}
		return nil, err
	}
	dmInfo := blockDevice.DMInfo()
	if dmInfo == nil || !dmInfo.IsCrypt() {
		return nil, fmt.Errorf("device with LUKS mapper name '%s' is not a crypt device", mapperName)
	}
	return newDevice(blockDevice, dmInfo), nil
}

// EnsureDevice ensures a LUKS device is formatted, open, and resolvable
// on the host.
//
// The method is a flat idempotent pipeline: format → open → resolve.
// Each step is individually idempotent, so the pipeline is safe to call
// at any point in the device lifecycle.
func (s *service) EnsureDevice(
	ctx context.Context, internalVolumeID, devicePath string,
	current, previous Secret, opts *EnsureDeviceOptions,
) (*Device, error) {
	mapperName := constructMapperName(internalVolumeID)
	devicePath = block.DevicePath(devicePath)
	fields := LogFields{
		"mapperName":       mapperName,
		"rawDevicePath":    devicePath,
		"internalVolumeID": internalVolumeID,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> luks.EnsureDevice")
	defer Logc(ctx).WithFields(fields).Debug("<<<< luks.EnsureDevice")

	if devicePath == "" {
		return nil, fmt.Errorf("raw device path is empty")
	}
	if opts == nil {
		opts = DefaultEnsureDeviceOptions()
	}
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}

	isLUKS, err := s.isDeviceLUKS(ctx, devicePath)
	if err != nil {
		return nil, fmt.Errorf("failed to check LUKS state of '%s': %w", devicePath, err)
	}
	if !isLUKS {
		Logc(ctx).WithFields(fields).Debug("Device is not LUKS formatted; formatting.")
		if err := s.ensureFormat(ctx, opts.VolumeID, devicePath, current, opts.FormatOptions); err != nil {
			return nil, err
		}
	}

	if err := s.ensureOpen(ctx, devicePath, mapperName, opts.OpenOptions, current, previous); err != nil {
		return nil, err
	}

	device, err := s.waitForDevice(ctx, devicePath, mapperName)
	if err != nil {
		return nil, err
	}
	device.freshFormat = !isLUKS

	Logc(ctx).WithFields(fields).Info("LUKS device formatted and open.")
	return device, nil
}

// EnsureResize ensures the LUKS device is resized to reflect the available space
// on the underlying device (multipath or NVMe).
//
// Returns an error if any of the input is malformed or the resize fails.
func (s *service) EnsureResize(ctx context.Context, dmUUID string, secret Secret) error {
	fields := LogFields{"dmUUID": dmUUID}
	Logc(ctx).WithFields(fields).Debug(">>>> luks.EnsureResize")
	defer Logc(ctx).WithFields(fields).Debug("<<<< luks.EnsureResize")

	if dmUUID == "" {
		return fmt.Errorf("no dm device uuid specified")
	}
	if secret.PassphraseValue() == "" {
		return fmt.Errorf("no passphrase provided for LUKS resize")
	}

	// The device should exist. If we can't find it by dm uuid, it doesn't exist.
	blockDev, err := s.block.DeviceByDMUUID(ctx, dmUUID)
	if err != nil {
		return err
	}
	Logc(ctx).WithField("device", blockDev).Debug("LUKS device found.")

	// Underlying cryptsetup adapter wants the mapper name, not the kernel device name.
	dmInfo := blockDev.DMInfo()
	if dmInfo == nil {
		return fmt.Errorf("DM device not found for dm UUID '%v'", dmUUID)
	}
	if !dmInfo.IsCrypt() {
		return fmt.Errorf("DM UUID '%s' points to a non-LUKS device", dmUUID)
	}
	mapperName := dmInfo.Name()

	// Resize the LUKS device.
	return s.resizeDevice(ctx, mapperName, secret.PassphraseValue())
}

// EnsureClose ensures that the given LUKS mapper is closed.
// If force is supplied, the BusyDeviceError's are ignored.
func (s *service) EnsureClose(ctx context.Context, dmUUID string, force bool) error {
	fields := LogFields{"dmUUID": dmUUID, "force": force}
	Logc(ctx).WithFields(fields).Debug(">>>> luks.EnsureClose")
	defer Logc(ctx).WithFields(fields).Debug("<<<< luks.EnsureClose")

	if dmUUID == "" {
		return fmt.Errorf("no dm device uuid specified")
	}

	dev, err := s.block.DeviceByDMUUID(ctx, dmUUID)
	if err != nil {
		if block.IsDeviceNotFoundError(err) {
			Logc(ctx).WithFields(fields).Info("LUKS dm device not found.")
			return nil
		}
		return fmt.Errorf("failed to resolve LUKS device with dm UUID %s: %w", dmUUID, err)
	}
	Logc(ctx).WithField("device", dev).Debug("LUKS device found.")

	// cryptsetup needs the mapper name, not the kernel device name.
	dmInfo := dev.DMInfo()
	if dmInfo == nil {
		return fmt.Errorf("DM device not found for dm UUID '%v'", dmUUID)
	}
	if !dmInfo.IsCrypt() {
		return fmt.Errorf("DM UUID '%s' points to a non-LUKS device", dmUUID)
	}
	mapperName := dmInfo.Name()

	// Close the LUKS device. It wants the mapper name (luks-pvc_123).
	if err := s.closeDevice(ctx, mapperName); err != nil {
		if force && errors.IsBusyDeviceError(err) {
			Logc(ctx).WithFields(fields).WithError(err).Warn("LUKS device busy on close; continuing under force.")
			return nil
		}
		Logc(ctx).WithFields(fields).WithError(err).Error("Failed to close LUKS device.")
		return fmt.Errorf("failed to close LUKS device '%s': %w", mapperName, err)
	}

	if force {
		Logc(ctx).WithFields(fields).Warn("Force detach; not waiting for LUKS device removal.")
		return nil
	}
	return s.waitForRemoval(ctx, dmUUID)
}

// ExtractSecrets extracts current and previous LUKS secrets from the raw CSI
// secrets map using the well-known key names.
func (s *service) ExtractSecrets(secrets map[string]string) (current, previous Secret) {
	current = Secret{
		Name:       secrets[SecretKeyPassphraseName],
		Passphrase: secrets[SecretKeyPassphrase],
	}
	previous = Secret{
		Name:       secrets[SecretKeyPreviousPassphraseName],
		Passphrase: secrets[SecretKeyPreviousPassphrase],
	}
	return current, previous
}

// EnsurePassphrase ensures the LUKS device accepts the 'current' passphrase.
// If it does not, it rotates from 'previous' to 'current'.
func (s *service) EnsurePassphrase(
	ctx context.Context, volumeID, devicePath string,
	current, previous Secret, opts *EnsurePassphraseOptions,
) error {
	mapperName := constructMapperName(volumeID)
	devicePath = block.DevicePath(devicePath)
	if opts != nil && opts.VolumeID != "" {
		volumeID = opts.VolumeID
	}

	fields := LogFields{
		"volumeID":      volumeID,
		"mapperName":    mapperName,
		"rawDevicePath": devicePath,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> luks.EnsurePassphrase")
	defer Logc(ctx).WithFields(fields).Debug("<<<< luks.EnsurePassphrase")

	currentName := current.PassphraseName()
	currentValue := current.PassphraseValue()
	if currentName == "" || currentValue == "" {
		return fmt.Errorf("current LUKS passphrase and name must be provided")
	}

	currentOK, err := s.testPassphrase(ctx, devicePath, mapperName, currentValue)
	if err != nil {
		return fmt.Errorf("could not verify current passphrase %s; %w", currentName, err)
	}
	if currentOK {
		if opts != nil && opts.ForceUpdate {
			if err = s.helper.UpdateVolumeLUKSPassphraseNames(ctx, volumeID, []string{currentName}); err != nil {
				return fmt.Errorf("error updating LUKS passphrase names; %w", err)
			}
		}
		return nil
	}

	prevName := previous.PassphraseName()
	prevVal := previous.PassphraseValue()
	if prevVal == "" || prevName == "" {
		return errors.AuthError("no working passphrase provided")
	}
	previousOK, err := s.testPassphrase(ctx, devicePath, mapperName, prevVal)
	if err != nil {
		return fmt.Errorf("could not verify previous passphrase %s; %w", prevName, err)
	}
	if !previousOK {
		return errors.AuthError("no working passphrase provided")
	}

	// Save both passphrase names to the volume.
	// This ensures that the passphrase name is present if we error or restart after a rotation,
	// and before we update the volume with the new passphrase name.
	if err = s.helper.UpdateVolumeLUKSPassphraseNames(ctx, volumeID, []string{currentName, prevName}); err != nil {
		return fmt.Errorf("error updating LUKS passphrase names; %w", err)
	}

	Logc(ctx).WithFields(LogFields{
		"current-luks-passphrase-name": prevName,
		"new-luks-passphrase-name":     currentName,
	}).Info("Rotating LUKS passphrase.")
	if err := s.crypt.ChangeKey(ctx, devicePath, volumeID, prevVal, currentValue); err != nil {
		return fmt.Errorf("failed to rotate LUKS passphrase: %w", translateError(err))
	}

	currentOK, err = s.testPassphrase(ctx, devicePath, mapperName, currentValue)
	if err != nil {
		return fmt.Errorf("could not check current passphrase for LUKS volume; %w", err)
	}
	if !currentOK {
		return errors.AuthError("current passphrase still invalid after rotation")
	}

	if err = s.helper.UpdateVolumeLUKSPassphraseNames(ctx, volumeID, []string{currentName}); err != nil {
		Logc(ctx).WithFields(LogFields{
			"volume":               volumeID,
			"luks-passphrase-name": currentName,
		}).WithError(err).Warn("Could not record current LUKS passphrase name after rotation.")
	}
	return nil
}

// ensureFormat ensures the devicePath has a LUKS header.
// It assumes the caller has already checked if the devicePath is already LUKS formatted.
//
// If the leading bytes of devicePath are not all zeros, it returns an error and never modifies the device.
func (s *service) ensureFormat(
	ctx context.Context, volumeID, devicePath string, current Secret, opts FormatOptions,
) error {
	fields := LogFields{"devicePath": devicePath}
	Logc(ctx).WithFields(fields).Trace(">>>> luks.ensureFormat")
	defer Logc(ctx).WithFields(fields).Trace("<<<< luks.ensureFormat")

	// A new device is always formatted with the current passphrase; the previous one is only
	// ever used to open or rotate an existing device.
	passphrase, passphraseName := current.PassphraseValue(), current.PassphraseName()
	if passphrase == "" {
		return ErrEmptyPassphrase
	}
	if passphraseName == "" {
		return ErrEmptyPassphraseName
	}

	if err := s.requireEmpty(ctx, devicePath); err != nil {
		return fmt.Errorf("device not safe for LUKS format: %w", err)
	}

	// Record the current passphrase name before formatting, so the recorded names include one that opens the device.
	if err := s.helper.UpdateVolumeLUKSPassphraseNames(ctx, volumeID, []string{passphraseName}); err != nil {
		return fmt.Errorf("could not record LUKS passphrase name before format; %w", err)
	}

	if _, err := s.formatDevice(ctx, devicePath, passphrase, opts); err != nil {
		Logc(ctx).WithField("devicePath", devicePath).Error("Failed to LUKS format device.")

		// requireEmpty proved the device was empty, so wipe any partial header to keep the next attempt possible.
		// Use a context that survives the caller's deadline so the cleanup can still run.
		cleanupCtx := context.WithoutCancel(ctx)
		if zeroErr := s.block.Zero(cleanupCtx, devicePath, block.DataOptions{
			BlockSize:  block.ProbeBlockSize,
			BlockCount: block.EmptyCheckBlockCount,
			Force:      true,
		}); zeroErr != nil {
			Logc(ctx).WithField("devicePath", devicePath).WithError(zeroErr).Warn(
				"Failed to clear partial LUKS header after format failure.")
		}
		return err
	}

	Logc(ctx).WithField("devicePath", devicePath).Info("LUKS formatted device.")
	return nil
}

// requireEmpty returns nil if the leading 2 MiB of devicePath are all zeros.
// Any non-zero byte (a filesystem, stale data, a foreign header) is refused;
// the device is never wiped here.
func (s *service) requireEmpty(ctx context.Context, devicePath string) error {
	buf, err := s.block.Read(ctx, devicePath, block.DataOptions{
		BlockSize:  block.ProbeBlockSize,
		BlockCount: block.EmptyCheckBlockCount,
	})
	if err != nil {
		return fmt.Errorf("failed to read device %s: %w", devicePath, err)
	}
	if len(buf) != block.EmptyCheckSize {
		return fmt.Errorf(
			"read %d bytes (expected %d) from %s; cannot verify it is empty",
			len(buf), block.EmptyCheckSize, devicePath,
		)
	}
	for _, b := range buf {
		if b != 0 {
			return fmt.Errorf("device %s is not empty", devicePath)
		}
	}
	return nil
}

// ensureOpen activates the LUKS mapper, trying each secret in order.
// Idempotent: no-ops if the mapper is already active.
// Non-auth errors abort immediately; auth errors advance to the next secret.
func (s *service) ensureOpen(
	ctx context.Context, devicePath, mapperName string, opts OpenOptions, secrets ...Secret,
) error {
	fields := LogFields{"devicePath": devicePath, "mapperName": mapperName}
	Logc(ctx).WithFields(fields).Trace(">>>> luks.ensureOpen")
	defer Logc(ctx).WithFields(fields).Trace("<<<< luks.ensureOpen")

	isOpen, err := s.isDeviceOpen(ctx, mapperName)
	if err != nil {
		return fmt.Errorf("failed to check if LUKS mapper '%s' is open: %w", mapperName, err)
	}
	if isOpen {
		Logc(ctx).WithFields(fields).Debug("LUKS device is already open.")
		return nil
	}
	Logc(ctx).WithFields(fields).Debug("LUKS device is not open.")

	if len(secrets) == 0 {
		return fmt.Errorf("no secrets provided to open LUKS device")
	}

	// Deduplicate secrets by passphrase content so we don't retry the same value.
	type checkSum [32]byte
	seen := make(map[checkSum]struct{})
	for _, secret := range secrets {
		name := secret.PassphraseName()
		phrase := secret.PassphraseValue()
		if phrase == "" {
			continue
		}
		sum := sha256.Sum256([]byte(phrase))
		if _, ok := seen[sum]; ok {
			continue
		}
		seen[sum] = struct{}{}

		if err := s.openDevice(ctx, devicePath, mapperName, phrase, opts); err != nil {
			if !errors.IsAuthError(err) {
				return fmt.Errorf("LUKS open of '%s' failed: %w", mapperName, err)
			}

			Logc(ctx).WithFields(fields).WithField(
				"luks-passphrase-name", name,
			).WithError(err).Warn("Passphrase rejected; trying next.")
			continue
		}
		Logc(ctx).WithFields(fields).Info("Opened encrypted volume.")
		return nil
	}

	return errors.AuthError("could not open LUKS device; no valid passphrase for LUKS mapper '%s'", mapperName)
}

// resolveDevice returns the *Device for the LUKS mapper named mapperName, or
// nil if no matching dm-* entry exists yet.
//
// We look the device up by mapper name (dm/name) rather than by walking the
// raw device's holders/ directory. udev populates holders/ lazily after
// cryptsetup open returns, so a holders walk can race the kernel and return
// an empty list for several seconds; dm/name is written atomically when the
// kernel creates the mapper, so this lookup is race-free.
//
// The returned device must be a CRYPT-LUKS2 dm device (defense in depth — we
// own the mapper namespace, so this should always be true when the lookup
// succeeds).
func (s *service) resolveDevice(ctx context.Context, rawDevicePath, mapperName string) *Device {
	fields := LogFields{"rawDevPath": rawDevicePath, "mapperName": mapperName}
	Logc(ctx).WithFields(fields).Trace(">>>> luks.resolveDevice")
	defer Logc(ctx).WithFields(fields).Trace("<<<< luks.resolveDevice")

	dev, err := s.block.DeviceByDMName(ctx, mapperName)
	if err != nil {
		if block.IsDeviceNotFoundError(err) {
			return nil
		}
		Logc(ctx).WithFields(fields).WithError(err).Warn("Failed to look up LUKS mapper.")
		return nil
	}

	dmInfo := dev.DMInfo()
	if dmInfo == nil || !dmInfo.IsCrypt() {
		Logc(ctx).WithFields(fields).Warn("Device with LUKS mapper name is not a crypt device.")
		return nil
	}

	Logc(ctx).WithFields(LogFields{
		"rawDevPath": rawDevicePath,
		"deviceName": dev.Name(),
		"devicePath": dev.Path(),
		"mapperName": dmInfo.Name(),
		"mapperPath": block.MapperPath(dmInfo.Name()),
	}).Info("Found LUKS device.")
	return newDevice(dev, dmInfo)
}

func (s *service) isDeviceLUKS(ctx context.Context, devicePath string) (bool, error) {
	if err := s.crypt.IsLUKS(ctx, devicePath); err != nil {
		if cryptsetup.IsNotLUKSDeviceError(err) {
			return false, nil
		}
		return false, fmt.Errorf("could not check LUKS status of %s: %w", devicePath, translateError(err))
	}
	return true, nil
}

func (s *service) isDeviceOpen(ctx context.Context, mapperName string) (bool, error) {
	if _, err := s.crypt.Status(ctx, mapperName); err != nil {
		if cryptsetup.IsDeviceNotActiveError(err) {
			return false, nil
		}
		return false, fmt.Errorf("could not check status of %s: %w", mapperName, translateError(err))
	}
	return true, nil
}

func (s *service) formatDevice(ctx context.Context, devicePath, passphrase string, opts cryptsetup.FormatOptions) (string, error) {
	Logc(ctx).WithField("rawDevicePath", devicePath).Trace(">>>> luks.formatDevice")
	defer Logc(ctx).WithField("rawDevicePath", devicePath).Trace("<<<< luks.formatDevice")

	out, err := s.crypt.Format(ctx, devicePath, passphrase, opts)
	if err != nil {
		return out, fmt.Errorf("LUKS format failed on %s: %w", devicePath, translateError(err))
	}
	return out, nil
}

func (s *service) testPassphrase(ctx context.Context, devicePath, mapperName string, passphrase string) (bool, error) {
	if err := s.crypt.TestPassphrase(ctx, devicePath, mapperName, passphrase); err != nil {
		if cryptsetup.IsIncorrectPassphraseError(err) {
			return false, nil
		}
		return false, fmt.Errorf("could not test passphrase on %s: %w", devicePath, translateError(err))
	}
	return true, nil
}

func (s *service) openDevice(ctx context.Context, devicePath, mapperName, passphrase string, opts cryptsetup.OpenOptions) error {
	if err := s.crypt.Open(ctx, devicePath, mapperName, passphrase, opts); err != nil {
		if cryptsetup.IsIncorrectPassphraseError(err) {
			return errors.AuthError("LUKS passphrase rejected for %s", mapperName)
		}

		// Idempotent: ensureOpen pre-checks isDeviceOpen, but a concurrent
		// caller (or udev) can win the race and create the mapper between
		// the check and our open. A busy error is success only if the mapper is now open.
		if cryptsetup.IsBusyDeviceError(err) {
			if isOpen, statusErr := s.isDeviceOpen(ctx, mapperName); statusErr == nil && isOpen {
				Logc(ctx).WithField("mapperName", mapperName).Debug(
					"LUKS mapper appeared between status check and open; treating as already open.")
				return nil
			}
		}
		return fmt.Errorf("failed to open LUKS device %s: %w", mapperName, translateError(err))
	}
	return nil
}

func (s *service) closeDevice(ctx context.Context, mapperName string) error {
	if err := s.crypt.Close(ctx, mapperName); err != nil {
		// Idempotent: a device that's already gone is a successful close.
		if cryptsetup.IsDeviceNotActiveError(err) {
			return nil
		}
		return fmt.Errorf("failed to close LUKS device %s: %w", mapperName, translateError(err))
	}
	return nil
}

func (s *service) resizeDevice(ctx context.Context, mapperName, passphrase string) error {
	if err := s.crypt.Resize(ctx, mapperName, passphrase); err != nil {
		if cryptsetup.IsIncorrectPassphraseError(err) {
			return errors.IncorrectLUKSPassphraseError("no key available with this passphrase; %v", err)
		}
		return fmt.Errorf("failed to resize LUKS device %s: %w", mapperName, translateError(err))
	}
	return nil
}

// waitForDevice waits for the host to populate the LUKS Device.
// Transient errors due to async propagation on the host are logged, but not returned.
// If no Device is found within the context deadline, an error is returned.
func (s *service) waitForDevice(ctx context.Context, devicePath, mapperName string) (*Device, error) {
	fields := LogFields{"devicePath": devicePath, "mapperName": mapperName}
	Logc(ctx).WithFields(fields).Trace(">>>> luks.waitForDevice")
	defer Logc(ctx).WithFields(fields).Trace("<<<< luks.waitForDevice")

	// First check immediately for the device. If we can find it now, return early.
	var dev *Device
	if dev = s.resolveDevice(ctx, devicePath, mapperName); dev != nil {
		return dev, nil
	}
	Logc(ctx).WithFields(fields).Debug("LUKS device not yet available; polling for the host to create it.")

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, waitForDevicePollTimeout)
		defer cancel()
	}
	ticker := time.NewTicker(waitForDevicePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled waiting for LUKS device: %w", ctx.Err())
		case <-ticker.C:
			if dev = s.resolveDevice(ctx, devicePath, mapperName); dev != nil {
				Logc(ctx).WithField("device", dev.Name()).Debug("LUKS device appeared after polling.")
				return dev, nil
			}
		}
	}
}

// waitForRemoval periodically (waitForRemovalPollInterval) polls the host to see if the
// LUKS dm device was removed. If the LUKS dm device is not found, it returns early.
// Otherwise, it will consume the entire context timeout budget until the LUKS dm device has disappeared.
func (s *service) waitForRemoval(ctx context.Context, dmUUID string) error {
	fields := LogFields{"dmUUID": dmUUID}
	Logc(ctx).WithFields(fields).Trace(">>>> luks.waitForRemoval")
	defer Logc(ctx).WithFields(fields).Trace("<<<< luks.waitForRemoval")

	// First check immediately for the device. If we cannot find it now, return early.
	dev, err := s.block.DeviceByDMUUID(ctx, dmUUID)
	if err != nil && block.IsDeviceNotFoundError(err) {
		Logc(ctx).WithFields(fields).Debug("LUKS device not present.")
		return nil
	}
	Logc(ctx).WithFields(fields).Debug("LUKS device still present; waiting for the host to remove it.")

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, waitForRemovalPollTimeout)
		defer cancel()
	}
	ticker := time.NewTicker(waitForRemovalPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled waiting for LUKS device {%v} removal: %w", dev, ctx.Err())
		case <-ticker.C:
			dev, err = s.block.DeviceByDMUUID(ctx, dmUUID)
			if err != nil && block.IsDeviceNotFoundError(err) {
				return nil
			}
			Logc(ctx).WithFields(fields).Debug("LUKS device still present; waiting for removal.")
		}
	}
}

// constructMapperName builds the LUKS mapper name from the supplied internal name and
// a constant luks device name prefix.
func constructMapperName(internalID string) string {
	return defaultMapperNamePrefix + internalID
}

// translateError translates cryptsetup adapter errors into Trident errors so
// upstream callers (drivers) can switch on stable Trident error types instead
// of cryptsetup adapter types.
func translateError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case cryptsetup.IsIncorrectPassphraseError(err):
		return errors.AuthError(err.Error())
	case cryptsetup.IsDeviceNotActiveError(err):
		return errors.NotFoundError(err.Error())
	case cryptsetup.IsBusyDeviceError(err):
		return errors.BusyDeviceError(err.Error())
	case cryptsetup.IsOOMError(err):
		return errors.ResourceExhaustedError(err)
	}
	return err
}
