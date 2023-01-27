// Copyright 2022 NetApp, Inc. All Rights Reserved.

package rest

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	. "github.com/netapp/trident/logging"
)

type APIServerHTTPS struct {
	server         *http.Server
	caCertFile     string
	serverCertFile string
	serverKeyFile  string
}

func NewHTTPSServer(
	p core.Orchestrator, address, port, caCertFile, serverCertFile, serverKeyFile string, enableMutualTLS bool,
	handler http.Handler, writeTimeout time.Duration,
) (*APIServerHTTPS, error) {
	orchestrator = p

	apiServer := &APIServerHTTPS{
		server: &http.Server{
			Addr:    fmt.Sprintf("%s:%s", address, port),
			Handler: &tlsAuthHandler{handler: handler},
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

	if !enableMutualTLS {
		apiServer.server.Handler = handler
		apiServer.server.TLSConfig.ClientAuth = tls.NoClientCert
	}

	if caCertFile != "" {
		caCert, err := ioutil.ReadFile(caCertFile)
		if err != nil {
			return nil, fmt.Errorf("could not read CA certificate file: %v", err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		apiServer.server.TLSConfig.ClientCAs = caCertPool
	}

	Log().WithField("address", apiServer.server.Addr).Info("Initializing HTTPS REST frontend.")

	return apiServer, nil
}

func (s *APIServerHTTPS) Activate() error {
	go func() {
		Log().WithField("address", s.server.Addr).Infof("Activating HTTPS REST frontend.")

		err := s.server.ListenAndServeTLS(s.serverCertFile, s.serverKeyFile)
		if err == http.ErrServerClosed {
			Log().WithField("address", s.server.Addr).Info("HTTPS REST frontend server has closed.")
		} else if err != nil {
			Log().Fatal(err)
		}
	}()
	return nil
}

func (s *APIServerHTTPS) Deactivate() error {
	Log().WithField("address", s.server.Addr).Infof("Deactivating HTTPS REST frontend.")
	ctx, cancel := context.WithTimeout(context.Background(), config.HTTPTimeout)
	defer cancel()
	return s.server.Shutdown(ctx)
}

func (s *APIServerHTTPS) GetName() string {
	return "HTTPS REST"
}

func (s *APIServerHTTPS) Version() string {
	return config.OrchestratorAPIVersion
}

type tlsAuthHandler struct {
	handler http.Handler
}

func (h *tlsAuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Service requests from Trident nodes with a valid client certificate
	if len(r.TLS.PeerCertificates) > 0 && r.TLS.PeerCertificates[0].Subject.CommonName == config.ClientCertName {
		Log().WithField("peerCert", config.ClientCertName).Debug("Authenticated by HTTPS REST frontend.")
		h.handler.ServeHTTP(w, r)
	} else {
		w.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=\"%s\"", config.OrchestratorName))
		w.WriteHeader(http.StatusUnauthorized)
	}
}
