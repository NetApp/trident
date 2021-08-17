// Copyright 2019 NetApp, Inc. All Rights Reserved.

package metrics

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/v21/config"
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

	log.WithField("address", metricsServer.server.Addr).Info("Initializing metrics frontend.")

	return metricsServer
}

func (s *Server) Activate() error {
	go func() {
		log.WithField("address", s.server.Addr).Info("Activating metrics frontend.")
		http.Handle("/metrics", s.server.Handler)
		if err := s.server.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()
	return nil
}

func (s *Server) Deactivate() error {
	log.WithField("address", s.server.Addr).Info("Deactivating metrics frontend.")
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
