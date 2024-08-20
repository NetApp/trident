package rest

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func makeTestHandler() http.HandlerFunc {
	return func(_ http.ResponseWriter, _ *http.Request) {}
}

func TestServer_NewHTTPServer(t *testing.T) {
	handler := makeTestHandler()
	server := NewHTTPServer(":50000", handler)
	assert.NoError(t, server.Activate(), "unexpected error")
	assert.Equal(t, apiName, server.GetName(), "expected names to match")
	assert.Equal(t, apiVersion, server.Version(), "expected versions to match")
	assert.NoError(t, server.Deactivate(), "unexepcted error")
}
