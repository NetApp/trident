package rest

import (
	"errors"
	"net/http"
	"time"

	. "github.com/netapp/trident/logging"
)

const (
	apiName     = "fiji"
	apiVersion  = "1"
	httpTimeout = 90 * time.Second
)

type Server struct {
	server *http.Server
}

func NewHTTPServer(address string, handler http.Handler) *Server {
	return &Server{
		server: &http.Server{
			Addr:              address,
			Handler:           handler,
			ReadHeaderTimeout: httpTimeout,
		},
	}
}

func (s *Server) Activate() error {
	go func() {
		Log().WithField("address", s.server.Addr).Info("Activating FIJI API server.")
		if err := s.server.ListenAndServe(); err != nil {
			// If the server closes gracefully, log and exit the routine.
			if errors.Is(err, http.ErrServerClosed) {
				return
			}
		}
	}()
	return nil
}

func (s *Server) Deactivate() error {
	Log().WithField("address", s.server.Addr).Info("Deactivating FIJI API server.")
	return s.server.Shutdown(nil)
}

func (s *Server) GetName() string {
	return apiName
}

func (s *Server) Version() string {
	return apiVersion
}
