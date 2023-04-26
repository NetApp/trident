// Copyright 2019 NetApp, Inc. All Rights Reserved.

package metrics

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
)

type Server struct {
	server *http.Server
}

// NewMetricsServer see also: https://godoc.org/github.com/prometheus/client_golang/prometheus/promauto
func NewMetricsServer(address, port string) *Server {
	metricsServer := &Server{
		server: &http.Server{
			Addr:         fmt.Sprintf("%s:%s", address, port),
			Handler:      promhttp.Handler(),
			ReadTimeout:  config.HTTPTimeout,
			WriteTimeout: config.HTTPTimeout,
		},
	}

	Log().WithField("address", metricsServer.server.Addr).Info("Initializing metrics frontend.")

	return metricsServer
}

func (s *Server) Activate() error {
	go func() {
		Log().WithField("address", s.server.Addr).Info("Activating metrics frontend.")
		http.Handle("/metrics", s.server.Handler)

		err := s.server.ListenAndServe()
		if err == http.ErrServerClosed {
			Log().WithField("address", s.server.Addr).Info("Metrics frontend server has closed.")
		} else if err != nil {
			Log().Fatal(err)
		}
	}()
	return nil
}

func (s *Server) Deactivate() error {
	Log().WithField("address", s.server.Addr).Info("Deactivating metrics frontend.")
	ctx, cancel := context.WithTimeout(context.Background(), config.HTTPTimeout)
	defer cancel()
	return s.server.Shutdown(ctx)
}

func (s *Server) GetName() string {
	return "metrics"
}

func (s *Server) Version() string {
	return config.OrchestratorAPIVersion
}
