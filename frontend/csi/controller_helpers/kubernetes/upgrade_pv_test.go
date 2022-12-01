// Copyright 2022 NetApp, Inc. All Rights Reserved.
package kubernetes

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
)

func TestHandleFailedPVUpgrades(t *testing.T) {
	volConfig1 := storage.VolumeConfig{
		Name: "fake1",
	}

	v1 := storage.VolumeExternal{
		Config: &volConfig1,
		State:  "upgrading",
	}

	volExt := []*storage.VolumeExternal{&v1}

	ctx := GenerateRequestContext(nil, "", ContextSourceInternal)
	m, helper := newMockPlugin(t)
	m.EXPECT().ListVolumes(ctx).Return(volExt, nil)

	volTxn := &storage.VolumeTransaction{
		Config: v1.Config,
	}
	volTxnOut := &storage.VolumeTransaction{
		Config: v1.Config,
		Op:     "upgradeVolume",
	}

	m.EXPECT().GetVolumeTransaction(ctx, volTxn).Return(volTxnOut, nil)
	m.EXPECT().GetVolume(ctx, "fake1").Return(nil, fmt.Errorf("volume not found"))
	assert.NotNil(t, helper.handleFailedPVUpgrades(ctx))
}

func TestHandleFailedPVUpgradesNoVolume(t *testing.T) {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal)
	m, helper := newMockPlugin(t)

	tests := []struct {
		Value string
	}{
		{""},
		{"no volume found"},
	}

	for _, test := range tests {
		t.Run(test.Value, func(t *testing.T) {
			if test.Value == "" {
				m.EXPECT().ListVolumes(ctx).Return(nil, nil)
				assert.Nil(t, helper.handleFailedPVUpgrades(ctx))
			} else {
				m.EXPECT().ListVolumes(ctx).Return(nil, fmt.Errorf(test.Value))
				assert.NotNil(t, helper.handleFailedPVUpgrades(ctx))
			}
		})
	}
}

func TestHandleFailedPVUpgradesGetVolumeTransaction(t *testing.T) {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal)
	m, helper := newMockPlugin(t)

	volConfig1 := storage.VolumeConfig{
		Name: "fake1",
	}

	v1 := storage.VolumeExternal{
		Config: &volConfig1,
		State:  "upgrading",
	}

	volExt := []*storage.VolumeExternal{&v1}

	volTxn := &storage.VolumeTransaction{
		Config: v1.Config,
	}

	tests := []struct {
		Value string
	}{
		{""},
		{"error parsing transaction"},
	}

	for _, test := range tests {
		m.EXPECT().ListVolumes(ctx).Return(volExt, nil).Times(1)
		t.Run(test.Value, func(t *testing.T) {
			if test.Value == "" {
				m.EXPECT().GetVolumeTransaction(ctx, volTxn).Return(nil, nil)
			} else {
				m.EXPECT().GetVolumeTransaction(ctx, volTxn).Return(nil, fmt.Errorf(test.Value))
			}
			assert.NotNil(t, helper.handleFailedPVUpgrades(ctx))
		})
	}
}
