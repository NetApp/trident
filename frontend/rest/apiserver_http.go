// Copyright 2022 NetApp, Inc. All Rights Reserved.

package rest

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	. "github.com/netapp/trident/logging"
)

var orchestrator core.Orchestrator

type APIServerHTTP struct {
	server *http.Server
}

func NewHTTPServer(p core.Orchestrator, address, port string, writeTimeout time.Duration) *APIServerHTTP {
	orchestrator = p

	apiServer := &APIServerHTTP{
		server: &http.Server{
			Addr:         fmt.Sprintf("%s:%s", address, port),
			Handler:      NewRouter(false),
			ReadTimeout:  config.HTTPTimeout,
			WriteTimeout: writeTimeout,
		},
	}

	Log().WithField("address", apiServer.server.Addr).Info("Initializing HTTP REST frontend.")

	return apiServer
}

func (s *APIServerHTTP) Activate() error {
	go func() {
		Log().WithField("address", s.server.Addr).Info("Activating HTTP REST frontend.")

		err := s.server.ListenAndServe()
		if err == http.ErrServerClosed {
			Log().WithField("address", s.server.Addr).Info("HTTP REST frontend server has closed.")
		} else if err != nil {
			Log().Fatal(err)
		}
	}()
	return nil
}

func (s *APIServerHTTP) Deactivate() error {
	Log().WithField("address", s.server.Addr).Info("Deactivating HTTP REST frontend.")
	ctx, cancel := context.WithTimeout(context.Background(), config.HTTPTimeout)
	defer cancel()
	return s.server.Shutdown(ctx)
}

func (s *APIServerHTTP) GetName() string {
	return "HTTP REST"
}

func (s *APIServerHTTP) Version() string {
	return config.OrchestratorAPIVersion
}
