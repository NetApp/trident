// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
)

type AstraDSVolumeType string

const (
	NetappVolumeTypeReadWrite AstraDSVolumeType = "ReadWrite"
)

type Cluster struct {
	Name      string
	Namespace string
	UUID      string
	Status    string
	Version   string
}

type Volume struct {
	Name              string
	Namespace         string
	ResourceVersion   string
	Annotations       map[string]string
	Labels            map[string]string
	DeletionTimestamp *metav1.Time
	Finalizers        []string

	Type            AstraDSVolumeType
	VolumePath      string
	Permissions     string
	DisplayName     string
	Created         bool
	State           string
	RequestedSize   resource.Quantity
	ActualSize      resource.Quantity
	VolumeUUID      string
	ExportAddress   string
	ExportPolicy    string
	SnapshotReserve int32
	NoSnapDir       bool
	QoSPolicy       string
	RestorePercent  int64
	CloneVolume     string
	CloneSnapshot   string

	CreateError error
	OnlineError error
	ModifyError error
	DeleteError error
}

// IsReady checks several values to ensure a volume is ready for use.
func (v *Volume) IsReady(ctx context.Context) bool {

	// Volume must report as having been created
	if !v.Created || v.CreateError != nil {
		return false
	}

	// Volume must report its state as online
	if v.State != VolumeStateOnline || v.OnlineError != nil {
		return false
	}

	// Volume must not be deleting
	if v.DeletionTimestamp != nil {
		return false
	}

	// Volume must have an export address
	if v.ExportAddress == "" {
		return false
	}

	// Volume must have a non-zero size
	if sizeBytes, err := v.GetActualSize(ctx); err != nil {
		return false
	} else if sizeBytes == 0 {
		return false
	}

	return true
}

func (v *Volume) GetRequestedSize(ctx context.Context) (int64, error) {

	var size string
	var err error
	if size, err = utils.ConvertSizeToBytes(v.RequestedSize.String()); err != nil {
		Logc(ctx).Errorf("Invalid volume size (%s); %v", v.RequestedSize.String(), err)
		return 0, err
	}

	var sizeBytes int64
	if sizeBytes, err = strconv.ParseInt(size, 10, 64); err != nil {
		Logc(ctx).Errorf("Invalid volume size (%s); %v", size, err)
		return 0, err
	}

	return sizeBytes, nil
}

func (v *Volume) GetActualSize(ctx context.Context) (int64, error) {

	var size string
	var err error
	if size, err = utils.ConvertSizeToBytes(v.ActualSize.String()); err != nil {
		Logc(ctx).Errorf("Invalid volume size (%s); %v", v.ActualSize.String(), err)
		return 0, err
	}

	var sizeBytes int64
	if sizeBytes, err = strconv.ParseInt(size, 10, 64); err != nil {
		Logc(ctx).Errorf("Invalid volume size (%s); %v", size, err)
		return 0, err
	}

	return sizeBytes, nil
}

// GetCreationError returns any error indicating a volume creation error, or nil if the volume is created and online.
func (v *Volume) GetCreationError() error {

	if IsVolumeCreateError(v.CreateError) {
		return v.CreateError
	}
	if IsVolumeOnlineError(v.OnlineError) {
		return v.OnlineError
	}

	return nil
}

type ExportPolicy struct {
	Name              string
	Namespace         string
	ResourceVersion   string
	DeletionTimestamp *metav1.Time
	Annotations       map[string]string
	Labels            map[string]string
	Finalizers        []string
	Rules             []ExportPolicyRule

	CreateError error
}

type ExportPolicyRule struct {
	Clients   []string
	Protocols []string
	RuleIndex uint64
	RoRules   []string
	RwRules   []string
	SuperUser []string
	AnonUser  int
}

// IsReady checks several values to ensure an export policy is ready for use.
func (e *ExportPolicy) IsReady(_ context.Context) bool {

	// Export policy must report as having been created
	if e.CreateError != nil {
		return false
	}

	// Export policy must not be deleting
	if e.DeletionTimestamp != nil {
		return false
	}

	return true
}

// GetCreationError returns any error indicating a export policy creation error, or nil if the export policy is created.
func (e *ExportPolicy) GetCreationError() error {

	if IsExportPolicyCreateError(e.CreateError) {
		return e.CreateError
	}

	return nil
}

type Snapshot struct {
	Name              string
	Namespace         string
	ResourceVersion   string
	DeletionTimestamp *metav1.Time
	Annotations       map[string]string
	Labels            map[string]string
	Finalizers        []string

	VolumeName string

	CreationTime *metav1.Time
	VolumeUUID   string
	ReadyToUse   bool

	ReadyError error
}

// IsReady checks several values to ensure a snapshot is ready for use.
func (s *Snapshot) IsReady(_ context.Context) bool {

	// Snapshot must be ready to use
	if !s.ReadyToUse || s.ReadyError != nil {
		return false
	}

	// Snapshot must not be deleting
	if s.DeletionTimestamp != nil {
		return false
	}

	return true
}

// GetReadyError returns any error indicating a snapshot creation error, or nil if the snapshot is either ready
// or still being created.
func (s *Snapshot) GetReadyError() error {

	if IsSnapshotReadyError(s.ReadyError) {
		return s.ReadyError
	}

	return nil
}

type QosPolicy struct {
	Name      string
	Cluster   string
	MinIOPS   int32
	MaxIOPS   int32
	BurstIOPS int32
}

// FormatCreationTime returns the RFC3339-formatted creation time for a snapshot
func (s *Snapshot) FormatCreationTime() string {
	if s == nil || s.CreationTime == nil {
		return time.Time{}.UTC().Format(storage.SnapshotTimestampFormat)
	}
	return s.CreationTime.UTC().Format(storage.SnapshotTimestampFormat)
}

// //////////////////////////////////////////////////////////////////////////
// volumeCreateError
// //////////////////////////////////////////////////////////////////////////

type volumeCreateError struct {
	message string
}

func (e *volumeCreateError) Error() string { return e.message }

func VolumeCreateError(message string) error {
	return &volumeCreateError{message}
}

func IsVolumeCreateError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*volumeCreateError)
	return ok
}

// //////////////////////////////////////////////////////////////////////////
// volumeOnlineError
// //////////////////////////////////////////////////////////////////////////

type volumeOnlineError struct {
	message string
}

func (e *volumeOnlineError) Error() string { return e.message }

func VolumeOnlineError(message string) error {
	return &volumeOnlineError{message}
}

func IsVolumeOnlineError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*volumeOnlineError)
	return ok
}

// //////////////////////////////////////////////////////////////////////////
// volumeModifyError
// //////////////////////////////////////////////////////////////////////////

type volumeModifyError struct {
	message string
}

func (e *volumeModifyError) Error() string { return e.message }

func VolumeModifyError(message string) error {
	return &volumeModifyError{message}
}

func IsVolumeModifyError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*volumeModifyError)
	return ok
}

// //////////////////////////////////////////////////////////////////////////
// volumeDeleteError
// //////////////////////////////////////////////////////////////////////////

type volumeDeleteError struct {
	message string
}

func (e *volumeDeleteError) Error() string { return e.message }

func VolumeDeleteError(message string) error {
	return &volumeDeleteError{message}
}

func IsVolumeDeleteError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*volumeDeleteError)
	return ok
}

// //////////////////////////////////////////////////////////////////////////
// exportPolicyCreateError
// //////////////////////////////////////////////////////////////////////////

type exportPolicyCreateError struct {
	message string
}

func (e *exportPolicyCreateError) Error() string { return e.message }

func ExportPolicyCreateError(message string) error {
	return &exportPolicyCreateError{message}
}

func IsExportPolicyCreateError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*exportPolicyCreateError)
	return ok
}

// //////////////////////////////////////////////////////////////////////////
// snapshotReadyError
// //////////////////////////////////////////////////////////////////////////

type snapshotReadyError struct {
	message string
}

func (e *snapshotReadyError) Error() string { return e.message }

func SnapshotReadyError(message string) error {
	return &snapshotReadyError{message}
}

func IsSnapshotReadyError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*snapshotReadyError)
	return ok
}
