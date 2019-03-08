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

const httpTimeout = 90 * time.Second

var orchestrator core.Orchestrator

type APIServer struct {
	server *http.Server
}

func NewAPIServer(p core.Orchestrator, address, port string) *APIServer {

	orchestrator = p

	log.WithFields(log.Fields{
		"address": address,
		"port":    port,
	}).Info("Initializing REST frontend.")

	return &APIServer{
		server: &http.Server{
			Addr:         address + ":" + port,
			Handler:      NewRouter(),
			ReadTimeout:  httpTimeout,
			WriteTimeout: httpTimeout,
		},
	}
}

func (s *APIServer) Activate() error {
	go func() {
		log.Info("Activating REST frontend.")
		err := s.server.ListenAndServe()
		if err != nil {
			log.Fatal(err)
		}
	}()
	return nil
}

func (s *APIServer) Deactivate() error {
	log.Info("Deactivating REST frontend.")
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
