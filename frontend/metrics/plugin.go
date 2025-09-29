// Copyright 2025 NetApp, Inc. All Rights Reserved.

package metrics

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
)

type Server struct {
	server *http.Server
}

// NewMetricsServer see also: https://godoc.org/github.com/prometheus/client_golang/prometheus/promauto
func NewMetricsServer(address, port string) *Server {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginCreate, LogLayerMetricsFrontend)

	metricsServer := &Server{
		server: &http.Server{
			Addr:         fmt.Sprintf("%s:%s", address, port),
			Handler:      promhttp.Handler(),
			ReadTimeout:  config.HTTPTimeout,
			WriteTimeout: config.HTTPTimeout,
		},
	}

	Logc(ctx).WithField("address", metricsServer.server.Addr).Info("Initializing metrics frontend.")

	return metricsServer
}

func (s *Server) Activate() error {
	go func() {
		ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginActivate, LogLayerMetricsFrontend)

		Logc(ctx).WithField("address", s.server.Addr).Info("Activating metrics frontend.")

		http.Handle("/metrics", s.server.Handler)
		err := s.server.ListenAndServe()
		if err == http.ErrServerClosed {
			Logc(ctx).WithField("address", s.server.Addr).Info("Metrics frontend server has closed.")
		} else if err != nil {
			Logc(ctx).Fatal(err)
		}
	}()
	return nil
}

func (s *Server) Deactivate() error {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginDeactivate, LogLayerMetricsFrontend)

	Logc(ctx).WithField("address", s.server.Addr).Info("Deactivating metrics frontend.")
	ctx, cancel := context.WithTimeout(ctx, config.HTTPTimeout)
	defer cancel()
	return s.server.Shutdown(ctx)
}

func (s *Server) GetName() string {
	return "metrics"
}

func (s *Server) Version() string {
	return config.OrchestratorAPIVersion
}

// HTTPSServer represents an HTTPS metrics server
type HTTPSServer struct {
	server         *http.Server
	caCertFile     string
	serverCertFile string
	serverKeyFile  string
}

// NewHTTPSMetricsServer creates a new HTTPS metrics server with TLS configuration
func NewHTTPSMetricsServer(address, port, caCertFile, serverCertFile, serverKeyFile string, enableMutualTLS bool, writeTimeout time.Duration) (*HTTPSServer, error) {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginCreate, LogLayerMetricsFrontend)

	httpsServer := &HTTPSServer{
		server: &http.Server{
			Addr:    fmt.Sprintf("%s:%s", address, port),
			Handler: &metricsAuthHandler{handler: promhttp.Handler()},
			TLSConfig: &tls.Config{
				ClientAuth: tls.RequireAndVerifyClientCert,
				MinVersion: config.MinServerTLSVersion,
			},
			ReadTimeout:  config.HTTPTimeout,
			WriteTimeout: writeTimeout,
		},
		caCertFile:     caCertFile,
		serverCertFile: serverCertFile,
		serverKeyFile:  serverKeyFile,
	}

	// Configure for non-mutual TLS if needed
	if !enableMutualTLS {
		httpsServer.server.Handler = promhttp.Handler()
		httpsServer.server.TLSConfig.ClientAuth = tls.NoClientCert
	}

	// Load CA certificate if provided
	if caCertFile != "" {
		caCert, err := os.ReadFile(caCertFile)
		if err != nil {
			return nil, fmt.Errorf("could not read CA certificate file: %v", err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		httpsServer.server.TLSConfig.ClientCAs = caCertPool
	}

	Logc(ctx).WithField("address", httpsServer.server.Addr).Info("Initializing HTTPS metrics frontend.")

	return httpsServer, nil
}

func (s *HTTPSServer) Activate() error {
	go func() {
		ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginActivate, LogLayerMetricsFrontend)

		Logc(ctx).WithField("address", s.server.Addr).Info("Activating HTTPS metrics frontend.")

		err := s.server.ListenAndServeTLS(s.serverCertFile, s.serverKeyFile)
		if err == http.ErrServerClosed {
			Logc(ctx).WithField("address", s.server.Addr).Info("HTTPS metrics frontend server has closed.")
		} else if err != nil {
			Logc(ctx).Fatal(err)
		}
	}()
	return nil
}

func (s *HTTPSServer) Deactivate() error {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginDeactivate, LogLayerMetricsFrontend)

	Logc(ctx).WithField("address", s.server.Addr).Info("Deactivating HTTPS metrics frontend.")
	ctx, cancel := context.WithTimeout(ctx, config.HTTPTimeout)
	defer cancel()
	return s.server.Shutdown(ctx)
}

func (s *HTTPSServer) GetName() string {
	return "HTTPS metrics"
}

func (s *HTTPSServer) Version() string {
	return config.OrchestratorAPIVersion
}

// metricsAuthHandler handles TLS authentication for metrics endpoints
type metricsAuthHandler struct {
	handler http.Handler
}

func (h *metricsAuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Service requests from Trident nodes with a valid client certificate
	if len(r.TLS.PeerCertificates) > 0 && r.TLS.PeerCertificates[0].Subject.CommonName == config.ClientCertName {
		ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginActivate, LogLayerMetricsFrontend)
		Logc(ctx).WithField("peerCert", config.ClientCertName).Debug("Authenticated by HTTPS metrics frontend.")
		h.handler.ServeHTTP(w, r)
	} else {
		w.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=\"%s\"", config.OrchestratorName))
		w.WriteHeader(http.StatusUnauthorized)
	}
}
