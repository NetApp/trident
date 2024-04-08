// Copyright 2024 NetApp, Inc. All Rights Reserved.

package rest

import (
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/operator/clients"
)

type APIServerHTTP struct {
	server *http.Server
}

const (
	httpReadTimeout  = 90 * time.Second
	httpWriteTimeout = 5 * time.Second
	httpTimeout      = 90 * time.Second
	apiVersion       = "1"
)

func NewHTTPServer(address, port string, clientFactory *clients.Clients) *APIServerHTTP {
	apiServer := &APIServerHTTP{
		server: &http.Server{
			Addr:         fmt.Sprintf("%s:%s", address, port),
			Handler:      NewRouter(clientFactory),
			ReadTimeout:  httpReadTimeout,
			WriteTimeout: httpWriteTimeout,
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
	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()

	return s.server.Shutdown(ctx)
}

func (s *APIServerHTTP) GetName() string {
	return "Operator HTTP REST Server"
}

func (s *APIServerHTTP) Version() string {
	return apiVersion
}
