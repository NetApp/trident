// Copyright 2025 NetApp, Inc. All Rights Reserved.

// Copyright 2017 The Kubernetes Authors.

package csi

import (
	"net"
	"os"
	"path"
	"runtime"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
)

// NonBlockingGRPCServer Defines Non blocking GRPC server interfaces
type NonBlockingGRPCServer interface {
	// Start services at the endpoint
	Start(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer,
		gcs csi.GroupControllerServer)

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
	endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer, gcs csi.GroupControllerServer,
) {
	go s.serve(endpoint, ids, cs, ns, gcs)
}

func (s *nonBlockingGRPCServer) GracefulStop() {
	s.server.GracefulStop()
}

func (s *nonBlockingGRPCServer) Stop() {
	s.server.Stop()
}

func (s *nonBlockingGRPCServer) serve(
	endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer, gcs csi.GroupControllerServer,
) {
	proto, addr, err := ParseEndpoint(endpoint)
	if err != nil {
		Log().Fatal(err.Error())
	}

	// Check whether we are in a container using a variable we always set.
	_, inCSIContainer := os.LookupEnv("CSI_ENDPOINT")

	if proto == "unix" {
		if runtime.GOOS != "windows" {
			addr = "/" + addr
		}
		if inCSIContainer {
			// Ensure socket dir exists
			socketDir := path.Dir(addr)
			if err := os.MkdirAll(socketDir, config.CSISocketDirPermissions); err != nil {
				Log().Fatalf("Failed to make socket dir %s, error: %v", socketDir, err)
			}
			// Ensure socket dir has minimum permissions if it already existed
			if err := os.Chmod(socketDir, config.CSISocketDirPermissions); err != nil {
				Log().Fatalf("Failed to chmod socket dir %s, error: %v", socketDir, err)
			}
		}
		// Remove existing socket
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			Log().Fatalf("Failed to remove %s, error: %v", addr, err)
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

	if listener.Addr().Network() == "unix" && inCSIContainer {
		// Ensure minimum permissions on socket
		if err := os.Chmod(addrString, config.CSIUnixSocketPermissions); err != nil {
			Log().Fatalf("Failed to chmod %s, error: %v", addrString, err)
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
	if gcs != nil {
		csi.RegisterGroupControllerServer(server, gcs)
		Log().Debug("Registered CSI group controller server.")
	}

	if err := server.Serve(listener); err != nil {
		Log().Fatal(err)
	}
}
