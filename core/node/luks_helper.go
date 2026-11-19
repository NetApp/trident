// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"

	"github.com/netapp/trident/config"
	hostluks "github.com/netapp/trident/pkg/host/service/luks"
	"github.com/netapp/trident/pkg/host/storage"
	hostnvme "github.com/netapp/trident/pkg/host/storage/block/nvme"
)

// luksHelper records LUKS passphrase names on the volume through the core's controller.
// It reads the controller at call time because SetController may run after NewCore.
type luksHelper struct {
	core *Core
}

var _ hostluks.Helper = (*luksHelper)(nil)

func (h *luksHelper) UpdateVolumeLUKSPassphraseNames(ctx context.Context, volume string, names []string) error {
	return updateLUKSPassphraseNames(ctx, h.core.controller, volume, names)
}

// newNVMeDriver builds the NVMe host storage stack with the core's LUKS helper.
func newNVMeDriver(c *Core) storage.Protocol {
	return hostnvme.New(
		hostnvme.WithDriverContext(config.ContextCSI),
		hostnvme.WithLUKS(hostluks.New(hostluks.WithHelper(&luksHelper{core: c}))),
	)
}
