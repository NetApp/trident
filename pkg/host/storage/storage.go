// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package storage defines common volume attachment APIs.
// All storage attachment drivers must implement this.
package storage

//go:generate mockgen -destination=../../../mocks/mock_pkg/mock_host/mock_storage/mock_storage.go github.com/netapp/trident/pkg/host/storage Protocol

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netapp/trident/utils/models"
)

// ErrTransportNotLoaded is returned, wrapped, by PreChecks when the host has the protocol's tools but its
// transport is not loaded, so the protocol is not active on the host.
var ErrTransportNotLoaded = errors.New("transport is not loaded on the host")

type Protocol interface {
	// PreChecks reports whether the host can run the protocol.
	PreChecks(ctx context.Context) error

	// HostID returns the host's identity for the protocol, such as the NVMe host NQN.
	HostID(ctx context.Context) (string, error)

	// Attach establishes connectivity to a storage volume and makes it
	// usable on the host. This includes session/path setup, device
	// discovery, optional LUKS encryption, and optional filesystem
	// formatting.
	//
	// Each phase is idempotent: repeated calls converge to the same
	// result without side effects.
	//
	// The returned state captures the host-side device
	// identity produced by the attachment.
	Attach(ctx context.Context, volumeID string, spec *models.VolumeAttachSpec) (*models.VolumeAttachState, error)

	// Detach tears down the attachment described by state: unmounts
	// filesystems, closes LUKS mappings, logs out sessions, and
	// cleans up multipath maps.
	//
	// Like Attach, each step is idempotent.
	// If force is true, the detachment will be forced even if there are active mounts.
	Detach(ctx context.Context, volumeID string, state *models.VolumeAttachState, spec *models.VolumeDetachSpec) error

	// Mount mounts the volume at the given target path.
	// volumeID is the ID of the volume to mount.
	// spec is the desired specification for the mount.
	Mount(ctx context.Context, volumeID string, state *models.VolumeAttachState, spec *models.VolumeMountSpec) error

	// Unmount unmounts the volume from the given target path.
	// volumeID is the ID of the volume to unmount.
	// spec is the current state of the volume mount.
	Unmount(ctx context.Context, volumeID string, spec *models.VolumeUnmountSpec) error

	// Expand performs an online expansion of an existing attachment.
	// volumeID is the ID of the volume to expand.
	// state is the current state of the volume attachment.
	// spec is the desired specification for the expansion.
	Expand(ctx context.Context, volumeID string, state *models.VolumeAttachState, spec *models.VolumeExpandSpec) error

	// Discover observes existing host state for a previously-attached volume
	// and returns the current attachment state without creating or modifying anything.
	// Returns nil state if the device is not currently live on the host.
	Discover(ctx context.Context, publishInfo *models.VolumePublishInfo) (*models.VolumeAttachState, error)

	// Graft and Prune take state as the attachment as known, and the request's access info as the desired one:
	// the targets come from the request, and an identity in the request that differs from state's is rejected.
	//
	// Graft connects the attachment to the request's targets that it is not yet connected to, and returns the
	// refreshed state.
	Graft(
		ctx context.Context, state *models.VolumeAttachState, req *models.GraftAttachmentRequest,
	) (*models.VolumeAttachState, error)

	// Prune removes the attachment's paths to targets that are not in the request.
	Prune(ctx context.Context, state *models.VolumeAttachState, req *models.PruneAttachmentRequest) error

	Healer
}

type HealConfig struct {
	Interval time.Duration
	Delay    time.Duration
}

type Healer interface {
	// Start spawns a background thread for self-healing that the Protocol driver owns.
	Start(ctx context.Context, cfg *HealConfig) error
	// Stop may be unnecessary.
	Stop()
	// Register records an attached volume, read from its tracking info at startup, in the self-healing state.
	// Attach and detach keep that state up to date afterwards.
	Register(ctx context.Context, volumeID string, info *models.VolumeTrackingInfo)
}

// CountingRWMutex is a sync.RWMutex that counts the readers waiting for it, so that a long-running writer can
// see waiting readers and yield to them. The zero value is unlocked.
type CountingRWMutex struct {
	mu      sync.RWMutex
	waiting atomic.Int32
}

func NewCountingRWMutex() *CountingRWMutex {
	return &CountingRWMutex{mu: sync.RWMutex{}, waiting: atomic.Int32{}}
}

func (m *CountingRWMutex) RLock() {
	m.waiting.Add(1)
	m.mu.RLock()
	m.waiting.Add(-1)
}

func (m *CountingRWMutex) RUnlock() { m.mu.RUnlock() }
func (m *CountingRWMutex) Lock()    { m.mu.Lock() }
func (m *CountingRWMutex) Unlock()  { m.mu.Unlock() }

// Waiting returns the number of readers blocked in RLock.
func (m *CountingRWMutex) Waiting() int32 { return m.waiting.Load() }
