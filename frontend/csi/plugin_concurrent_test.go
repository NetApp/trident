// Copyright 2026 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tridentconfig "github.com/netapp/trident/config"
)

func TestPlugin_NodeRegistrationGate_ReproducedWithAndWithoutConcurrentMode(t *testing.T) {
	modes := []struct {
		name       string
		concurrent bool
	}{
		{name: "without_concurrency", concurrent: false},
		{name: "with_concurrency", concurrent: true},
	}

	gatedMethods := []string{
		csi.Node_NodeStageVolume_FullMethodName,
		csi.Node_NodePublishVolume_FullMethodName,
		csi.Node_NodeExpandVolume_FullMethodName,
		csi.Node_NodeGetVolumeStats_FullMethodName,
	}

	allowedNodeMethods := []string{
		csi.Node_NodeGetInfo_FullMethodName,
		csi.Node_NodeGetCapabilities_FullMethodName,
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			previousConcurrent := tridentconfig.IsConcurrent
			tridentconfig.IsConcurrent = mode.concurrent
			t.Cleanup(func() {
				tridentconfig.IsConcurrent = previousConcurrent
			})

			plugin := &Plugin{
				role:        CSINode,
				nodeName:    "test-node",
				endpoint:    "unix:///tmp/test.sock",
				nodeReadyCh: make(chan struct{}),
			}

			for _, method := range gatedMethods {
				t.Run(method+"_blocked_before_registration", func(t *testing.T) {
					called := false
					handler := func(ctx context.Context, req interface{}) (interface{}, error) {
						called = true
						return method + "-ok", nil
					}

					resp, err := nodeRegistrationInterceptor(context.Background(), nil, serverInfo(plugin, method), handler)

					require.Error(t, err)
					assert.Nil(t, resp)
					assert.False(t, called, "handler should not run before registration completes")
					assert.Equal(t, codes.Unavailable, status.Code(err))
				})
			}

			for _, method := range allowedNodeMethods {
				t.Run(method+"_allowed_before_registration", func(t *testing.T) {
					called := false
					handler := func(ctx context.Context, req interface{}) (interface{}, error) {
						called = true
						return method + "-ok", nil
					}

					resp, err := nodeRegistrationInterceptor(context.Background(), nil, serverInfo(plugin, method), handler)

					require.NoError(t, err)
					assert.Equal(t, method+"-ok", resp)
					assert.True(t, called, "handler should run for informational node methods")
				})
			}

			plugin.markNodeReady()

			for _, method := range gatedMethods {
				t.Run(method+"_allowed_after_registration", func(t *testing.T) {
					called := false
					handler := func(ctx context.Context, req interface{}) (interface{}, error) {
						called = true
						return method + "-ok", nil
					}

					resp, err := nodeRegistrationInterceptor(context.Background(), nil, serverInfo(plugin, method), handler)

					require.NoError(t, err)
					assert.Equal(t, method+"-ok", resp)
					assert.True(t, called, "handler should run after registration completes")
				})
			}

			aioPlugin := &Plugin{role: CSIAllInOne, nodeName: "aio-node", nodeReadyCh: make(chan struct{})}
			for _, method := range gatedMethods {
				t.Run("all_in_one_"+method+"_blocked_before_registration", func(t *testing.T) {
					called := false
					handler := func(ctx context.Context, req interface{}) (interface{}, error) {
						called = true
						return method + "-ok", nil
					}

					resp, err := nodeRegistrationInterceptor(context.Background(), nil, serverInfo(aioPlugin, method), handler)

					require.Error(t, err)
					assert.Nil(t, resp)
					assert.False(t, called, "all-in-one node data-path method should also be blocked before registration")
					assert.Equal(t, codes.Unavailable, status.Code(err))
				})
			}

			t.Run("controller_method_still_allowed", func(t *testing.T) {
				controllerPlugin := &Plugin{role: CSIController, nodeReadyCh: make(chan struct{})}
				called := false
				handler := func(ctx context.Context, req interface{}) (interface{}, error) {
					called = true
					return "controller-ok", nil
				}

				resp, err := nodeRegistrationInterceptor(
					context.Background(), nil, serverInfo(controllerPlugin, csi.Controller_CreateVolume_FullMethodName), handler,
				)

				require.NoError(t, err)
				assert.Equal(t, "controller-ok", resp)
				assert.True(t, called, "controller methods should not be gated by node registration")
			})

			t.Run("non_plugin_server_passthrough", func(t *testing.T) {
				called := false
				handler := func(ctx context.Context, req interface{}) (interface{}, error) {
					called = true
					return "passthrough-ok", nil
				}

				resp, err := nodeRegistrationInterceptor(
					context.Background(), nil, serverInfo("not-a-plugin", csi.Node_NodeStageVolume_FullMethodName), handler,
				)

				require.NoError(t, err)
				assert.Equal(t, "passthrough-ok", resp)
				assert.True(t, called, "non-plugin servers should not be gated")
			})
		})
	}
}

// TestPlugin_NodeGetInfoAllowedDuringRegistration verifies that informational RPCs
// (like NodeGetInfo) are allowed even before registration completes. This ensures
// node-driver-registrar can still get node info for its bookkeeping.
func TestPlugin_NodeGetInfoAllowedDuringRegistration(t *testing.T) {
	plugin := &Plugin{
		role:        CSINode,
		nodeName:    "test-node",
		nodeReadyCh: make(chan struct{}), // Not registered yet
	}

	infoHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "node-info-ok", nil
	}

	infoInfo := serverInfo(plugin, csi.Node_NodeGetInfo_FullMethodName)

	// NodeGetInfo should be allowed even before registration
	resp, err := nodeRegistrationInterceptor(context.Background(), nil, infoInfo, infoHandler)
	require.NoError(t, err)
	assert.Equal(t, "node-info-ok", resp)
}

// TestPlugin_ControllerRPCsUnaffectedByNodeRegistration ensures that controller-side
// RPCs are not gated by node registration status. Only node data-path RPCs should be blocked.
func TestPlugin_ControllerRPCsUnaffectedByNodeRegistration(t *testing.T) {
	plugin := &Plugin{
		role:        CSIController,
		nodeReadyCh: make(chan struct{}), // Not registered (irrelevant for controller)
	}

	ctrlHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "controller-ok", nil
	}

	ctrlInfo := serverInfo(plugin, csi.Controller_CreateVolume_FullMethodName)

	// Controller RPCs should pass through unaffected
	resp, err := nodeRegistrationInterceptor(context.Background(), nil, ctrlInfo, ctrlHandler)
	require.NoError(t, err)
	assert.Equal(t, "controller-ok", resp)
}
