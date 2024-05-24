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

func TestIsZAPISupported(t *testing.T) {
	// Define test cases
	tests := []struct {
		name              string
		version           string
		isSupported       bool
		isSupportedErrMsg string
		wantErr           bool
		wantErrMsg        string
	}{
		{
			name:              "Supported version",
			version:           "9.14.1",
			isSupported:       true,
			isSupportedErrMsg: "positive test, version 9.14.1 is supported, expected true but got false",
			wantErr:           false,
			wantErrMsg:        "9.14.1 is a correct semantics, error was not expected",
		},
		{
			name:              "Unsupported version",
			version:           "9.18.1",
			isSupported:       false,
			isSupportedErrMsg: "negative test, version 9.18.1 is not supported, expected false but got true",
			wantErr:           false,
			wantErrMsg:        "9.18.1 is a correct semantics, error was not expected",
		},
		{
			name:              "Patch version i.e. 9.17.x",
			version:           "9.17.2",
			isSupported:       true,
			isSupportedErrMsg: "positive test, version 9.17.2, expected true but got false",
			wantErr:           false,
			wantErrMsg:        "9.17.2 is a correct semantics, error was not expected",
		},
		{
			name:              "Invalid version",
			version:           "invalid",
			isSupported:       false,
			isSupportedErrMsg: "invalid version provided, expected false but got true",
			wantErr:           true,
			wantErrMsg:        "incorrect semantics, error was expected",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			isSupported, err := IsZAPISupported(tt.version)
			assert.Equal(t, tt.isSupported, isSupported, tt.isSupportedErrMsg)
			if tt.wantErr {
				assert.Errorf(t, err, tt.wantErrMsg)
			} else {
				assert.NoError(t, err, tt.wantErrMsg)
			}
		})
	}
}
