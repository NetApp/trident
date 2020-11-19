// Copyright 2019 NetApp, Inc. All Rights Reserved.

// Copyright 2017 The Kubernetes Authors.

package csi

import (
	"net"
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
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

func (s *nonBlockingGRPCServer) Start(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {
	go s.serve(endpoint, ids, cs, ns)
}

func (s *nonBlockingGRPCServer) GracefulStop() {
	s.server.GracefulStop()
}

func (s *nonBlockingGRPCServer) Stop() {
	s.server.Stop()
}

func (s *nonBlockingGRPCServer) serve(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {

	proto, addr, err := ParseEndpoint(endpoint)
	if err != nil {
		log.Fatal(err.Error())
	}

	if proto == "unix" {
		addr = "/" + addr
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			log.Fatalf("Failed to remove %s, error: %s", addr, err.Error())
		}
	}

	listener, err := net.Listen(proto, addr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	log.WithFields(log.Fields{
		"name": listener.Addr().String(),
		"net":  listener.Addr().Network(),
	}).Info("Listening for GRPC connections.")

	if listener.Addr().Network() == "unix" {
		if err := os.Chmod(listener.Addr().String(), 0777); err != nil {
			log.Fatal(err)
		}
	}

	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(logGRPC),
	}
	server := grpc.NewServer(opts...)
	s.server = server

	if ids != nil {
		csi.RegisterIdentityServer(server, ids)
		log.Debug("Registered CSI identity server.")
	}
	if cs != nil {
		csi.RegisterControllerServer(server, cs)
		log.Debug("Registered CSI controller server.")
	}
	if ns != nil {
		csi.RegisterNodeServer(server, ns)
		log.Debug("Registered CSI node server.")
	}

	if err := server.Serve(listener); err != nil {
		log.Fatal(err)
	}
}
