package api

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestZapiClientInterface confirms ZapiClient implements the ZapiClientInterface
func TestZapiClientInterface(t *testing.T) {
	// use reflection to lookup the interface for ZapiClientInterface
	iface := reflect.TypeOf((*ZapiClientInterface)(nil)).Elem()

	// validate zapiClient implements the ZapiClientInterface
	zapiClient := NewClient(ClientConfig{}, "", "")
	assert.True(t, reflect.TypeOf(zapiClient).Implements(iface))
}
