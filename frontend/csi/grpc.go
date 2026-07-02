// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Copyright 2017 The Kubernetes Authors.

package csi

import (
	"errors"
	"net"
	"os"
	"path"
	"runtime"
	"sync"

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

// NewNonBlockingGRPCServer creates a gRPC server with optional extra interceptors inserted between the timeout
// and metrics interceptors. For CSINode and CSIAllInOne roles the caller should pass nodeRegistrationInterceptor;
// for CSIController it should be omitted so the interceptor is never in the chain.
func NewNonBlockingGRPCServer(extraInterceptors ...grpc.UnaryServerInterceptor) NonBlockingGRPCServer {
	return &nonBlockingGRPCServer{extraInterceptors: extraInterceptors}
}

// NonBlocking server
type nonBlockingGRPCServer struct {
	server            *grpc.Server
	extraInterceptors []grpc.UnaryServerInterceptor
	mu                sync.RWMutex
	stopRequested     bool
	gracefulStop      bool
}

func (s *nonBlockingGRPCServer) Start(
	endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer, gcs csi.GroupControllerServer,
) {
	go s.serve(endpoint, ids, cs, ns, gcs)
}

func (s *nonBlockingGRPCServer) GracefulStop() {
	if server, shouldStop := s.markStopRequested(true); shouldStop {
		server.GracefulStop()
	}
}

func (s *nonBlockingGRPCServer) Stop() {
	if server, shouldStop := s.markStopRequested(false); shouldStop {
		server.Stop()
	}
}

// buildInterceptorChain constructs the gRPC unary interceptor chain. The log and timeout interceptors are always
// present. Any extra interceptors (e.g. nodeRegistrationInterceptor for Node/AllInOne roles) are inserted between
// the timeout and metrics interceptors so they only run for the roles that need them.
func (s *nonBlockingGRPCServer) buildInterceptorChain() []grpc.UnaryServerInterceptor {
	chain := []grpc.UnaryServerInterceptor{
		logGRPCInterceptor,
		timeoutInterceptor,
	}
	chain = append(chain, s.extraInterceptors...)
	chain = append(chain, incomingRequestMetricsInterceptor)
	return chain
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
		// The first interceptor is always the outermost.
		// When CSI calls come in, the outermost interceptor is hit first.
		// The log gRPC and timeout interceptors should always be the first in the chain.
		grpc.ChainUnaryInterceptor(s.buildInterceptorChain()...),
	}
	server := grpc.NewServer(opts...)
	s.setServer(server)
	defer s.setServer(nil)

	if shouldStop, graceful := s.consumePendingStop(); shouldStop {
		if graceful {
			server.GracefulStop()
		} else {
			server.Stop()
		}
		_ = listener.Close()
		return
	}

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

	if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		Log().Fatal(err)
	}
}

func (s *nonBlockingGRPCServer) setServer(server *grpc.Server) {
	s.mu.Lock()
	s.server = server
	s.mu.Unlock()
}

func (s *nonBlockingGRPCServer) getServer() *grpc.Server {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.server
}

func (s *nonBlockingGRPCServer) markStopRequested(graceful bool) (*grpc.Server, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stopRequested = true
	if graceful {
		s.gracefulStop = true
	}

	if s.server == nil {
		return nil, false
	}
	return s.server, true
}

func (s *nonBlockingGRPCServer) consumePendingStop() (bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.stopRequested {
		return false, false
	}
	return true, s.gracefulStop
}
