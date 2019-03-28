// Copyright 2019 NetApp, Inc. All Rights Reserved.

package rest

import (
	"context"
	"fmt"
	"net/http"

	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
)

var orchestrator core.Orchestrator

type APIServerHTTP struct {
	server *http.Server
}

func NewHTTPServer(p core.Orchestrator, address, port string) *APIServerHTTP {

	orchestrator = p

	apiServer := &APIServerHTTP{
		server: &http.Server{
			Addr:         fmt.Sprintf("%s:%s", address, port),
			Handler:      NewRouter(),
			ReadTimeout:  HTTPTimeout,
			WriteTimeout: HTTPTimeout,
		},
	}

	log.WithField("address", apiServer.server.Addr).Info("Initializing HTTP REST frontend.")

	return apiServer
}

func (s *APIServerHTTP) Activate() error {
	go func() {
		log.WithField("address", s.server.Addr).Info("Activating HTTP REST frontend.")
		err := s.server.ListenAndServe()
		if err != nil {
			log.Fatal(err)
		}
	}()
	return nil
}

func (s *APIServerHTTP) Deactivate() error {
	log.WithField("address", s.server.Addr).Info("Deactivating HTTP REST frontend.")
	ctx, cancel := context.WithTimeout(context.Background(), HTTPTimeout)
	defer cancel()
	return s.server.Shutdown(ctx)
}

func (s *APIServerHTTP) GetName() string {
	return "HTTP REST"
}

func (s *APIServerHTTP) Version() string {
	return config.OrchestratorAPIVersion
}
