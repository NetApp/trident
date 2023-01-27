// Copyright 2019 NetApp, Inc. All Rights Reserved.

// Copyright 2017 The Kubernetes Authors.

package csi

import (
	"net"
	"os"
	"runtime"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
)

// NonBlockingGRPCServer Defines Non blocking GRPC server interfaces
type NonBlockingGRPCServer interface {
	// Start services at the endpoint
	Start(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer)
	// GracefulStop Stops the service gracefully
	GracefulStop()
	// Stops the service forcefully
	Stop()
}

func NewNonBlockingGRPCServer() NonBlockingGRPCServer {
	return &nonBlockingGRPCServer{}
}

// NonBlocking server
type nonBlockingGRPCServer struct {
	server *grpc.Server
}

func (s *nonBlockingGRPCServer) Start(
	endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer,
) {
	go s.serve(endpoint, ids, cs, ns)
}

func (s *nonBlockingGRPCServer) GracefulStop() {
	s.server.GracefulStop()
}

func (s *nonBlockingGRPCServer) Stop() {
	s.server.Stop()
}

func (s *nonBlockingGRPCServer) serve(
	endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer,
) {
	proto, addr, err := ParseEndpoint(endpoint)
	if err != nil {
		Log().Fatal(err.Error())
	}

	if proto == "unix" {
		if runtime.GOOS != "windows" {
			addr = "/" + addr
		}
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			Log().Fatalf("Failed to remove %s, error: %s", addr, err.Error())
		}
	}

	listener, err := net.Listen(proto, addr)
	if err != nil {
		Log().Fatalf("Failed to listen: %v", err)
	}

	addrString := listener.Addr().String()

	Log().WithFields(LogFields{
		"name": addrString,
		"net":  listener.Addr().Network(),
	}).Info("Listening for GRPC connections.")

	if listener.Addr().Network() == "unix" {
		pluginDir := strings.ReplaceAll(addrString, "csi.sock", "")
		// Plugins directory only needs to be accessed by Container Orchestrator components or Trident, so set to 600.
		if err := os.Chmod(pluginDir, config.CSISocketDirPermissions); err != nil {
			Log().Fatal(err)
		}

		// CSI socket file only needs read+write access to Container Orchestrator components or Trident, so set to 600.
		if err := os.Chmod(addrString, config.CSIUnixSocketPermissions); err != nil {
			Log().Fatal(err)
		}
	}

	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(logGRPC),
	}
	server := grpc.NewServer(opts...)
	s.server = server

	if ids != nil {
		csi.RegisterIdentityServer(server, ids)
		Log().Debug("Registered CSI identity server.")
	}
	if cs != nil {
		csi.RegisterControllerServer(server, cs)
		Log().Debug("Registered CSI controller server.")
	}
	if ns != nil {
		csi.RegisterNodeServer(server, ns)
		Log().Debug("Registered CSI node server.")
	}

	if err := server.Serve(listener); err != nil {
		Log().Fatal(err)
	}
}
