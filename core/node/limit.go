// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"fmt"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/limiter"
)

const (
	attachNFSVolumeKey   = "AttachNFSVolume"
	attachSMBVolumeKey   = "AttachSMBVolume"
	attachISCSIVolumeKey = "AttachISCSIVolume"
	attachFCPVolumeKey   = "AttachFCPVolume"
	attachNVMeVolumeKey  = "AttachNVMeVolume"

	maxAttachNFSVolumeOperations   = 10
	maxAttachSMBVolumeOperations   = 10
	maxAttachISCSIVolumeOperations = 5
	maxAttachFCPVolumeOperations   = 5
	maxAttachNVMeVolumeOperations  = 5

	detachNFSVolumeKey   = "DetachNFSVolume"
	detachSMBVolumeKey   = "DetachSMBVolume"
	detachISCSIVolumeKey = "DetachISCSIVolume"
	detachFCPVolumeKey   = "DetachFCPVolume"
	detachNVMeVolumeKey  = "DetachNVMeVolume"

	maxDetachNFSVolumeOperations   = 10
	maxDetachSMBVolumeOperations   = 10
	maxDetachISCSIVolumeOperations = 10
	maxDetachFCPVolumeOperations   = 10
	maxDetachNVMeVolumeOperations  = 10

	mountNFSVolumeKey   = "MountNFSVolume"
	mountSMBVolumeKey   = "MountSMBVolume"
	mountISCSIVolumeKey = "MountISCSIVolume"
	mountFCPVolumeKey   = "MountFCPVolume"
	mountNVMeVolumeKey  = "MountNVMeVolume"

	maxMountNFSVolumeOperations   = 10
	maxMountSMBVolumeOperations   = 10
	maxMountISCSIVolumeOperations = 10
	maxMountFCPVolumeOperations   = 10
	maxMountNVMeVolumeOperations  = 10

	unmountVolumeKey = "UnmountVolume"

	maxUnmountVolumeOperations = 10

	expandVolumeKey = "ExpandVolume"

	maxExpandVolumeOperations = 10

	graftISCSIAttachmentKey = "GraftISCSIAttachment"
	pruneISCSIAttachmentKey = "PruneISCSIAttachment"

	maxGraftISCSIAttachmentOperations = 10
	maxPruneISCSIAttachmentOperations = 10
)

func (c *Core) initializeLimiters(ctx context.Context) error {
	Logc(ctx).Debug("Initializing node limiters.")
	defer Logc(ctx).Debug("Node limiters initialized.")

	limiterSizes := map[string]int{
		attachNFSVolumeKey:   maxAttachNFSVolumeOperations,
		attachSMBVolumeKey:   maxAttachSMBVolumeOperations,
		attachISCSIVolumeKey: maxAttachISCSIVolumeOperations,
		attachFCPVolumeKey:   maxAttachFCPVolumeOperations,
		attachNVMeVolumeKey:  maxAttachNVMeVolumeOperations,

		detachNFSVolumeKey:   maxDetachNFSVolumeOperations,
		detachSMBVolumeKey:   maxDetachSMBVolumeOperations,
		detachISCSIVolumeKey: maxDetachISCSIVolumeOperations,
		detachFCPVolumeKey:   maxDetachFCPVolumeOperations,
		detachNVMeVolumeKey:  maxDetachNVMeVolumeOperations,

		mountNFSVolumeKey:   maxMountNFSVolumeOperations,
		mountSMBVolumeKey:   maxMountSMBVolumeOperations,
		mountISCSIVolumeKey: maxMountISCSIVolumeOperations,
		mountFCPVolumeKey:   maxMountFCPVolumeOperations,
		mountNVMeVolumeKey:  maxMountNVMeVolumeOperations,

		unmountVolumeKey: maxUnmountVolumeOperations,
		expandVolumeKey:  maxExpandVolumeOperations,

		graftISCSIAttachmentKey: maxGraftISCSIAttachmentOperations,
		pruneISCSIAttachmentKey: maxPruneISCSIAttachmentOperations,
	}

	sharedMap := make(map[string]limiter.Limiter, len(limiterSizes))
	for name, size := range limiterSizes {
		l, err := limiter.New(ctx, name, limiter.TypeSemaphoreN, limiter.WithSemaphoreNSize(ctx, size))
		if err != nil {
			return fmt.Errorf("failed to initialize limiter for %s: %w", name, err)
		}
		sharedMap[name] = l
	}
	c.protocolLimiters = sharedMap

	return nil
}

func (c *Core) acquireLimiter(ctx context.Context, key string) (release func(), err error) {
	l, ok := c.protocolLimiters[key]
	if !ok || l == nil {
		return func() {}, nil
	}
	if err = l.Wait(ctx); err != nil {
		return func() {}, err
	}
	return func() { l.Release(ctx) }, nil
}
