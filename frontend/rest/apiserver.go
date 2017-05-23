// Copyright 2016 NetApp, Inc. All Rights Reserved.

package rest

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/tylerb/graceful"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
)

const httpTimeout = 10 * time.Second

func init() {

}

var orchestrator core.Orchestrator

type APIServer struct {
	router *mux.Router
	port   string
	server *graceful.Server
}

func NewAPIServer(p core.Orchestrator, port string) *APIServer {
	orchestrator = p
	router := NewRouter()
	return &APIServer{
		router: router,
		port:   port,
		server: &graceful.Server{
			Timeout: httpTimeout,
			Server: &http.Server{
				Addr:    ":" + port,
				Handler: router,
			},
		},
	}
}

func (server *APIServer) Activate() error {
	go func() {
		err := server.server.ListenAndServe()
		if err != nil {
			log.Fatal(err)
		}
	}()
	return nil
}

func (server *APIServer) Deactivate() error {
	server.server.Stop(httpTimeout)
	return nil
}

func (server *APIServer) GetName() string {
	return "REST"
}

func (server *APIServer) Version() string {
	return config.OrchestratorAPIVersion
}
