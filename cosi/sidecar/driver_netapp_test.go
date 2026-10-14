// Copyright 2026 NetApp, Inc. All Rights Reserved.

// This file is NETAPP LOCAL. It is not vendored from upstream and is absent from
// cosi/upstream.manifest, so resync.sh ignores it. It covers NewDriverInfo, the one
// deliberate divergence in cosi/sidecar/driver.go.
//
// The package is reconciler_test rather than reconciler on purpose: the tests reach
// back into cosi/adapter and cosi/provisioner/fake, and an external test package
// keeps that from ever becoming an import cycle if the adapter later needs
// something from the vendored reconcilers.

package reconciler_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cosiproto "sigs.k8s.io/container-object-storage-interface/proto"

	"github.com/netapp/trident/cosi/adapter"
	"github.com/netapp/trident/cosi/provisioner/fake"
	reconciler "github.com/netapp/trident/cosi/sidecar"
)

// TestNewDriverInfoInProcess is the test that matters: it walks the path the COSI
// frontend will take at startup, with no gRPC connection anywhere. A fake backend is
// wrapped in the adapter, the adapter is asked to describe itself, and the answer is
// validated into the DriverInfo the reconcilers consume.
func TestNewDriverInfoInProcess(t *testing.T) {
	ctx := context.Background()
	client := adapter.New(fake.New())

	reported, err := client.DriverGetInfo(ctx, &cosiproto.DriverGetInfoRequest{})
	require.NoError(t, err)

	driverInfo, err := reconciler.NewDriverInfo(reported, client)
	require.NoError(t, err)

	assert.Equal(t, fake.DriverName, driverInfo.Name)
	assert.Equal(t, []cosiproto.ObjectProtocol_Type{cosiproto.ObjectProtocol_S3}, driverInfo.SupportedProtocols)

	// The client must be carried through untouched. If NewDriverInfo ever rebuilt it
	// from a connection the way upstream does, this is what would break.
	assert.Same(t, client, driverInfo.ProvisionerClient)

	// And it must be callable. A reconciler holding this DriverInfo provisions against
	// the fake backend directly, in-process.
	created, err := driverInfo.ProvisionerClient.DriverCreateBucket(ctx, &cosiproto.DriverCreateBucketRequest{
		Name:      "bucket-in-process",
		Protocols: []*cosiproto.ObjectProtocol{{Type: cosiproto.ObjectProtocol_S3}},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, created.GetBucketId())
}

// TestNewDriverInfoValidation confirms the divergence kept upstream's validation
// rather than skipping it. These checks are not cosmetic: a driver name that fails
// them leaves the reconcilers matching no objects at all, and they would silently do
// nothing.
func TestNewDriverInfoValidation(t *testing.T) {
	client := adapter.New(fake.New())

	tests := []struct {
		name        string
		reported    *cosiproto.DriverGetInfoResponse
		wantErrPart string
	}{
		{
			name: "driver name starting with a digit",
			reported: &cosiproto.DriverGetInfoResponse{
				Name:               "7.of.9",
				SupportedProtocols: []*cosiproto.ObjectProtocol{{Type: cosiproto.ObjectProtocol_S3}},
			},
			wantErrPart: "driver name is invalid",
		},
		{
			name: "no protocols reported",
			reported: &cosiproto.DriverGetInfoResponse{
				Name:               "seven.of.nine",
				SupportedProtocols: []*cosiproto.ObjectProtocol{},
			},
			wantErrPart: "supported protocols list is invalid",
		},
		{
			name: "duplicate protocol",
			reported: &cosiproto.DriverGetInfoResponse{
				Name: "seven.of.nine",
				SupportedProtocols: []*cosiproto.ObjectProtocol{
					{Type: cosiproto.ObjectProtocol_S3},
					{Type: cosiproto.ObjectProtocol_S3},
				},
			},
			wantErrPart: "supported protocols list is invalid",
		},
		{
			name: "unknown protocol",
			reported: &cosiproto.DriverGetInfoResponse{
				Name:               "seven.of.nine",
				SupportedProtocols: []*cosiproto.ObjectProtocol{{Type: cosiproto.ObjectProtocol_UNKNOWN}},
			},
			wantErrPart: "supported protocols list is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driverInfo, err := reconciler.NewDriverInfo(tt.reported, client)
			assert.ErrorContains(t, err, tt.wantErrPart)
			assert.Nil(t, driverInfo)
		})
	}
}

// TestValidateAndSetDriverConnectionInfoStillDelegates guards the shape of the
// divergence, not just its behaviour. The upstream entry point was reduced to a
// wrapper so the two constructors cannot drift; if someone re-inlines the validation
// into it, the wrapper and NewDriverInfo stop agreeing and this notices.
func TestValidateAndSetDriverConnectionInfoStillDelegates(t *testing.T) {
	reported := &cosiproto.DriverGetInfoResponse{
		Name:               "seven.of.nine",
		SupportedProtocols: []*cosiproto.ObjectProtocol{{Type: cosiproto.ObjectProtocol_S3}},
	}

	// A nil ClientConn is fine here: NewProvisionerClient only stores it, and nothing
	// in this test issues an RPC.
	viaConn, connErr := reconciler.ValidateAndSetDriverConnectionInfo(reported, nil)
	viaClient, clientErr := reconciler.NewDriverInfo(reported, adapter.New(fake.New()))

	require.NoError(t, connErr)
	require.NoError(t, clientErr)
	assert.Equal(t, viaClient.Name, viaConn.Name)
	assert.Equal(t, viaClient.SupportedProtocols, viaConn.SupportedProtocols)
}
