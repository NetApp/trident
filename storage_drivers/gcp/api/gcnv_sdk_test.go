// Copyright 2026 NetApp, Inc. All Rights Reserved.

package api

import (
	"testing"

	"cloud.google.com/go/netapp/apiv1/netapppb"
	"github.com/stretchr/testify/assert"
)

func TestVolumeProtocolFromGCNVProtocol_IncludesISCSI(t *testing.T) {
	assert.Equal(t, ProtocolTypeISCSI, VolumeProtocolFromGCNVProtocol(netapppb.Protocols_ISCSI))
}

func TestNewVolumeFromGCNVVolume_ISCSIBlockDevicesAndPortals(t *testing.T) {
	sdk := getFakeSDK()

	volPB := &netapppb.Volume{
		Name:        "projects/123456789/locations/fake-location/volumes/test-volume",
		Network:     "projects/123456789/global/networks/default",
		StoragePool: "projects/123456789/locations/fake-location/storagePools/CP1",
		CapacityGib: 100,
		Protocols:   []netapppb.Protocols{netapppb.Protocols_ISCSI},
		BlockDevices: []*netapppb.BlockDevice{
			{
				Identifier: "lun-serial-123",
				OsType:     netapppb.OsType_LINUX,
				HostGroups: []string{"projects/123456789/locations/fake-location/hostGroups/hg1"},
			},
		},
		MountOptions: []*netapppb.MountOption{
			{
				Protocol:  netapppb.Protocols_ISCSI,
				IpAddress: "10.0.0.1,10.0.0.2",
			},
		},
	}

	vol, err := sdk.newVolumeFromGCNVVolume(ctx, volPB)
	assert.NoError(t, err)
	assert.NotNil(t, vol)

	assert.Equal(t, "test-volume", vol.Name)
	assert.Equal(t, int64(100)*GiBBytes, vol.SizeBytes)
	assert.Equal(t, "lun-serial-123", vol.SerialNumber)
	assert.Contains(t, vol.ProtocolTypes, ProtocolTypeISCSI)
	assert.Equal(t, "10.0.0.1:3260", vol.ISCSITargetPortal)
	assert.Equal(t, []string{"10.0.0.1:3260", "10.0.0.2:3260"}, vol.ISCSIPortals)
	assert.NotEmpty(t, vol.ISCSITargetIQN)
	assert.Len(t, vol.BlockDevices, 1)
	assert.Equal(t, 0, vol.LunID)
}

func TestNewHostGroupFromGCNVHostGroup_Basic(t *testing.T) {
	sdk := getFakeSDK()
	hgPB := &netapppb.HostGroup{
		Name:   "projects/123456789/locations/fake-location/hostGroups/hg1",
		Type:   netapppb.HostGroup_ISCSI_INITIATOR,
		State:  netapppb.HostGroup_READY,
		Hosts:  []string{"iqn.1993-08.org.debian:01:abc"},
		OsType: netapppb.OsType_LINUX,
		Labels: map[string]string{"k": "v"},
	}

	hg := sdk.convertProtoHostGroupToHostGroup(hgPB)
	assert.NotNil(t, hg)
	assert.Equal(t, hgPB.Name, hg.Name)
	assert.Equal(t, HostGroupTypeISCSIInitiator, hg.Type)
	assert.Equal(t, HostGroupStateReady, hg.State)
	assert.Equal(t, OSTypeLinux, hg.OSType)
	assert.Equal(t, hgPB.Hosts, hg.Hosts)
	assert.Equal(t, hgPB.Labels, hg.Labels)
}
