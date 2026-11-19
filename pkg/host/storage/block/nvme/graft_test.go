// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/pkg/host/service/nvme"
	tridenterrors "github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

type graftNVMe struct {
	nvme.Service
	connected []string
	deviceErr error
	pruned    []string
	pruneErr  error
}

func (n *graftNVMe) EnsureConnections(_ context.Context, _ string, ips []string) error {
	n.connected = ips
	return nil
}

func (n *graftNVMe) EnsureDevice(context.Context, string, string) (*nvme.Device, error) {
	return nvme.NewDeviceForTest("nqn.sub", "ns-1", "nvme1n1"), n.deviceErr
}

func (n *graftNVMe) PrunePaths(_ context.Context, _ string, ips []string) error {
	n.pruned = ips
	return n.pruneErr
}

func graftState() *models.VolumeAttachState {
	state := &models.VolumeAttachState{}
	state.NVMeSubsystemNQN = "nqn.sub"
	state.NVMeNamespaceUUID = "ns-1"
	return state
}

func TestGraft(t *testing.T) {
	ctx := context.Background()
	namespace := models.BlockDeviceInfo{ID: "ns-1", Type: models.BlockDeviceDisk, Name: "nvme1n1"}

	t.Run("connects and waits for the namespace", func(t *testing.T) {
		n := &graftNVMe{}
		d := New(WithNVMe(n))

		grafted, err := d.Graft(ctx, graftState(), graftRequest([]string{"1.1.1.1", "2.2.2.2"}))

		require.NoError(t, err)
		assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, n.connected)
		assert.Equal(t, "/dev/nvme1n1", grafted.DevicePath)
		assert.Equal(t, models.BlockDeviceStack{namespace}, grafted.BlockAttachInfo.Devices)
		assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, d.sessionStore.Info["nqn.sub"].NVMeTargetIPs)
	})

	t.Run("refreshes a renamed namespace under the mapper", func(t *testing.T) {
		d := New(WithNVMe(&graftNVMe{}))
		crypt := models.BlockDeviceInfo{ID: "CRYPT-x", Type: models.BlockDeviceCrypt, Base: "ns-1"}
		state := graftState()
		state.DevicePath = "/dev/mapper/luks-x"
		state.BlockAttachInfo.Devices = models.BlockDeviceStack{
			{ID: "ns-1", Type: models.BlockDeviceDisk, Name: "nvme0n1"}, crypt,
		}

		grafted, err := d.Graft(ctx, state, graftRequest([]string{"1.1.1.1"}))

		require.NoError(t, err)
		assert.Equal(t, "/dev/mapper/luks-x", grafted.DevicePath)
		assert.Equal(t, models.BlockDeviceStack{namespace, crypt}, grafted.BlockAttachInfo.Devices)
		assert.Equal(t, "nvme0n1", state.BlockAttachInfo.Devices[0].Name, "the input state is not changed")
	})

	t.Run("namespace not visible", func(t *testing.T) {
		deviceErr := tridenterrors.NotFoundError("namespace not found")
		d := New(WithNVMe(&graftNVMe{deviceErr: deviceErr}))

		grafted, err := d.Graft(ctx, graftState(), graftRequest([]string{"1.1.1.1"}))

		assert.ErrorIs(t, err, deviceErr)
		assert.Nil(t, grafted)
		assert.False(t, d.sessionStore.SessionExists("nqn.sub"))
	})

	t.Run("rejects a request it cannot apply", func(t *testing.T) {
		otherNQN := graftRequest([]string{"1.1.1.1"})
		otherNQN.NVMeSubsystemNQN = "nqn.other"
		otherNamespace := graftRequest([]string{"1.1.1.1"})
		otherNamespace.NVMeNamespaceUUID = "ns-2"

		for name, req := range map[string]*models.GraftAttachmentRequest{
			"no NVMe target IPs specified": graftRequest(nil),
			"NVMe subsystem NQN mismatch":  otherNQN,
			"NVMe namespace UUID mismatch": otherNamespace,
		} {
			n := &graftNVMe{}
			_, err := New(WithNVMe(n)).Graft(ctx, graftState(), req)

			assert.True(t, tridenterrors.IsTerminalReconciliationError(err), name)
			assert.EqualError(t, err, name)
			assert.Nil(t, n.connected)
		}
	})
}

func TestPrune(t *testing.T) {
	ctx := context.Background()
	published := func(d *Driver) []string { return d.sessionStore.Info["nqn.sub"].NVMeTargetIPs }

	t.Run("prunes to the desired IPs", func(t *testing.T) {
		n := &graftNVMe{}
		d := New(WithNVMe(n))
		d.Register(ctx, "vol1", trackingInfo("nqn.sub", "ns-1", "1.1.1.1", "2.2.2.2"))

		require.NoError(t, d.Prune(ctx, graftState(), pruneRequest([]string{"1.1.1.1"})))
		assert.Equal(t, []string{"1.1.1.1"}, n.pruned)
		assert.Equal(t, []string{"1.1.1.1"}, published(d))
	})

	t.Run("subsystem without paths", func(t *testing.T) {
		d := New(WithNVMe(&graftNVMe{pruneErr: tridenterrors.NotFoundError("no paths")}))
		d.Register(ctx, "vol1", trackingInfo("nqn.sub", "ns-1", "1.1.1.1", "2.2.2.2"))

		assert.NoError(t, d.Prune(ctx, graftState(), pruneRequest([]string{"1.1.1.1"})))
		assert.Equal(t, []string{"1.1.1.1"}, published(d))
	})

	t.Run("other errors are returned", func(t *testing.T) {
		pruneErr := errors.New("sysfs read failed")
		d := New(WithNVMe(&graftNVMe{pruneErr: pruneErr}))
		d.Register(ctx, "vol1", trackingInfo("nqn.sub", "ns-1", "1.1.1.1", "2.2.2.2"))

		assert.ErrorIs(t, d.Prune(ctx, graftState(), pruneRequest([]string{"1.1.1.1"})), pruneErr)
		assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, published(d))
	})

	t.Run("rejects another volume's identity", func(t *testing.T) {
		n := &graftNVMe{}
		req := pruneRequest([]string{"1.1.1.1"})
		req.NVMeNamespaceUUID = "ns-2"

		err := New(WithNVMe(n)).Prune(ctx, graftState(), req)

		assert.True(t, tridenterrors.IsTerminalReconciliationError(err))
		assert.Nil(t, n.pruned)
	})
}

func graftRequest(targetIPs []string) *models.GraftAttachmentRequest {
	return &models.GraftAttachmentRequest{
		VolumeAccessInfo: models.VolumeAccessInfo{NVMeAccessInfo: models.NVMeAccessInfo{NVMeTargetIPs: targetIPs}},
	}
}

func pruneRequest(targetIPs []string) *models.PruneAttachmentRequest {
	return &models.PruneAttachmentRequest{
		VolumeAccessInfo: models.VolumeAccessInfo{NVMeAccessInfo: models.NVMeAccessInfo{NVMeTargetIPs: targetIPs}},
	}
}
