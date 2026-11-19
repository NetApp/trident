// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package nvme provides the NVMe attachment Driver for the host storage stack.
// This driver should facilitate all network and device protocol attachment logic for NVMe.
// It orchestrates the full attach/detach lifecycle by composing transport-specific
// (NVMe) and transport-agnostic (LUKS, filesystem, device) services.
//
// The Driver itself contains no sysfs I/O or binary invocations.
// All host interactions flow through services -> adapters -> host.
//
// NVMe differs from iSCSI in the ways that matter to this package:
//   - There is no multipath dm layer. The kernel's native NVMe multipath presents one
//     namespace head device (/dev/nvmeXnY) for all paths, so the device stack is
//     [disk, crypt?] and path health is a transport concern, not a block concern.
//   - Subsystems are shared by many namespaces. Disconnecting a subsystem tears down
//     every namespace on it, so "is this the last namespace" is a safety decision.
//   - Paths are connected with ctrl-loss-tmo=-1, so a dead path is never reaped by the kernel.
//   - A LUKS mapper can outlive its namespace and paths (orphaned/stale mapper).
package nvme

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/pkg/host/service/block"
	"github.com/netapp/trident/pkg/host/service/filesystem"
	"github.com/netapp/trident/pkg/host/service/luks"
	"github.com/netapp/trident/pkg/host/service/nvme"
	"github.com/netapp/trident/pkg/host/storage"
	tridenterrors "github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/limiter"
	"github.com/netapp/trident/utils/models"
)

const (
	attachKey  = "attach"
	attachOps  = 5
	detachKey  = "detach"
	detachOps  = 10
	mountKey   = "mount"
	mountOps   = 10
	unmountKey = "unmount"
	unmountOps = 10
	expandKey  = "expand"
	expandOps  = 10
	graftKey   = "graft"
	graftOps   = 10
	pruneKey   = "prune"
	pruneOps   = 10
)

var _ storage.Protocol = (*Driver)(nil)

// Driver holds the host's published NVMe sessions and the self-healing loop over them, so one Driver must be
// shared by every caller on the host.
type Driver struct {
	/** Locks & limiters */
	limiters      map[string]limiter.Limiter
	healingLock   *storage.CountingRWMutex
	sessionLocker sync.RWMutex
	sessionStore  Sessions

	/** Self-healing control */
	healingStopCh chan any
	healingTicker *time.Ticker

	/** Detach retry windows: when a LUKS close (by dm UUID) and a flush (by namespace UUID) first failed. */
	failuresLock      sync.Mutex
	luksCloseFailedAt map[string]time.Time
	nvmeFlushFailedAt map[string]time.Time

	/** Transport & Device Domains */
	nvme       nvme.Service
	block      block.Service
	luks       luks.Service
	filesystem filesystem.Service

	// driverContext is the frontend the Driver serves. Only CSI publishes sessions and self-heals them.
	driverContext config.DriverContext
}

type Option func(*Driver)

func WithBlock(svc block.Service) Option           { return func(d *Driver) { d.block = svc } }
func WithNVMe(svc nvme.Service) Option             { return func(d *Driver) { d.nvme = svc } }
func WithLUKS(svc luks.Service) Option             { return func(d *Driver) { d.luks = svc } }
func WithFilesystem(svc filesystem.Service) Option { return func(d *Driver) { d.filesystem = svc } }

// WithDriverContext sets the frontend the Driver serves; the default is CSI.
// Docker has no self-healing: a Docker attach publishes no session and leaves a failed attach as it is, since
// without sessions the Driver cannot tell whether other volumes use the subsystem.
func WithDriverContext(c config.DriverContext) Option { return func(d *Driver) { d.driverContext = c } }

func New(opts ...Option) *Driver {
	d := &Driver{
		limiters:          make(map[string]limiter.Limiter),
		healingLock:       storage.NewCountingRWMutex(),
		healingStopCh:     make(chan any),
		healingTicker:     time.NewTicker(time.Minute),
		sessionLocker:     sync.RWMutex{},
		luksCloseFailedAt: make(map[string]time.Time),
		nvmeFlushFailedAt: make(map[string]time.Time),
		driverContext:     config.ContextCSI,
		block:             block.New(),
		nvme:              nvme.New(),
		luks:              luks.New(),
		filesystem:        filesystem.New(),
	}
	for _, opt := range opts {
		opt(d)
	}
	limiterSizes := map[string]int{
		attachKey:  attachOps,
		detachKey:  detachOps,
		mountKey:   mountOps,
		unmountKey: unmountOps,
		expandKey:  expandOps,
		graftKey:   graftOps,
		pruneKey:   pruneOps,
	}

	ctx := context.Background()
	sharedMap := make(map[string]limiter.Limiter, len(limiterSizes))
	for name, size := range limiterSizes {
		l, err := limiter.New(ctx, name, limiter.TypeSemaphoreN, limiter.WithSemaphoreNSize(ctx, size))
		if err != nil {
			panic(fmt.Sprintf("failed to initialize limiter for %s: %v", name, err))
		}
		sharedMap[name] = l
	}
	d.limiters = sharedMap
	return d
}

func (d *Driver) acquireLimiter(ctx context.Context, key string) (release func(), err error) {
	l, ok := d.limiters[key]
	if !ok || l == nil {
		return func() {}, nil
	}
	if err = l.Wait(ctx); err != nil {
		return func() {}, err
	}
	return func() { l.Release(ctx) }, nil
}

// PreChecks verifies that the host has nvme-cli and the NVMe/TCP transport. A missing transport is returned
// wrapping storage.ErrTransportNotLoaded.
func (d *Driver) PreChecks(ctx context.Context) error {
	if err := d.nvme.PreChecks(ctx); err != nil {
		if errors.Is(err, nvme.ErrTransportNotLoaded) {
			return fmt.Errorf("%w: %w", storage.ErrTransportNotLoaded, err)
		}
		return err
	}
	return nil
}

// HostID returns the host NQN.
func (d *Driver) HostID(ctx context.Context) (string, error) {
	return d.nvme.HostNQN(ctx)
}

// mapperByInternalID returns the crypt layer of the open LUKS mapper for a volume's internal ID, or nil
// if no such mapper is open. It finds a mapper that outlived its namespace, which Discover cannot reach.
func (d *Driver) mapperByInternalID(ctx context.Context, internalID string) (*models.BlockDeviceInfo, error) {
	dev, err := d.luks.DeviceByInternalID(ctx, internalID)
	if err != nil {
		if tridenterrors.IsNotFoundError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to look up LUKS mapper: %w", err)
	}
	return new(dev.Info()), nil
}

// ensurePassphrase verifies the LUKS passphrase on the raw device and rotates it from the previous one if needed.
// The mapper is named from internalID; the passphrase names are recorded against volumeID.
func (d *Driver) ensurePassphrase(
	ctx context.Context, volumeID, internalID, rawDevicePath string, secrets map[string]string,
) error {
	current, previous := d.luks.ExtractSecrets(secrets)
	opts := luks.DefaultEnsurePassphraseOptions()
	opts.VolumeID = volumeID
	return d.luks.EnsurePassphrase(ctx, internalID, rawDevicePath, current, previous, opts)
}
