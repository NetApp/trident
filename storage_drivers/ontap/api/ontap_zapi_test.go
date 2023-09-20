// Copyright 2019 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
)

func TestGetError(t *testing.T) {
	e := azgo.GetError(context.Background(), nil, nil)

	assert.Equal(t, "failed", e.(azgo.ZapiError).Status(), "Strings not equal")

	assert.Equal(t, azgo.EINTERNALERROR, e.(azgo.ZapiError).Code(), "Strings not equal")

	assert.Equal(t, "unexpected nil ZAPI result", e.(azgo.ZapiError).Reason(), "Strings not equal")
}

func TestVolumeExists_EmptyVolumeName(t *testing.T) {
	ctx := context.Background()

	zapiClient := Client{}
	volumeExists, err := zapiClient.VolumeExists(ctx, "")

	assert.NoError(t, err, "VolumeExists should not have errored with a missing volume name")
	assert.False(t, volumeExists)
}
