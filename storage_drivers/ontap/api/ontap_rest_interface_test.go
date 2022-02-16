package api

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRestClientInterface confirms RestClient implements the RestClientInterface
func TestRestClientInterface(t *testing.T) {
	// use reflection to lookup the interface for RestClientInterface
	iface := reflect.TypeOf((*RestClientInterface)(nil)).Elem()

	// validate restClient implements the RestClientInterface
	restClient, _ := NewRestClient(ctx, ClientConfig{})
	assert.True(t, reflect.TypeOf(restClient).Implements(iface))
}
