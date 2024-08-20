package rest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRoutes_makeRoutes(t *testing.T) {
	routes := makeRoutes(nil)
	assert.NotEmpty(t, routes, "expected routes to be non-empty")

	for _, route := range routes {
		assert.NotNil(t, route.HandlerFunc, "route handlers should never be nil")
	}
}
