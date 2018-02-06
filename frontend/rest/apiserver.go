/*
 * Copyright 2018 NetApp, Inc. All Rights Reserved.
 */

package rest

import (
	"context"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
)

const httpTimeout = 30 * time.Second

var orchestrator core.Orchestrator

type APIServer struct {
	server *http.Server
}

func NewAPIServer(p core.Orchestrator, address, port string) *APIServer {

	orchestrator = p

	addressPort := address + ":" + port
	log.Infof("Starting REST interface on %s", addressPort)

	return &APIServer{
		server: &http.Server{
			Addr:         addressPort,
			Handler:      NewRouter(),
			ReadTimeout:  httpTimeout,
			WriteTimeout: httpTimeout,
		},
	}
}

func (s *APIServer) Activate() error {
	go func() {
		err := s.server.ListenAndServe()
		if err != nil {
			log.Fatal(err)
		}
	}()
	return nil
}

func (s *APIServer) Deactivate() error {
	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()
	return s.server.Shutdown(ctx)
}

func (s *APIServer) GetName() string {
	return "REST"
}

func (s *APIServer) Version() string {
	return config.OrchestratorAPIVersion
}
