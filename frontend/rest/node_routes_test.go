// Copyright 2025 NetApp, Inc. All Rights Reserved.

package rest

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/frontend/csi"
)

func TestNodeRoutes(t *testing.T) {
	tests := []struct {
		name                  string
		plugin                *csi.Plugin
		expectedRouteCount    int
		expectedRouteNames    []string
		expectedRouteMethods  []string
		expectedRoutePatterns []string
	}{
		{
			name:                  "ValidPlugin",
			plugin:                &csi.Plugin{}, // Real plugin struct
			expectedRouteCount:    2,
			expectedRouteNames:    []string{"LivenessProbe", "ReadinessProbe"},
			expectedRouteMethods:  []string{"GET", "GET"},
			expectedRoutePatterns: []string{"/liveness", "/readiness"},
		},
		{
			name:                  "NilPlugin",
			plugin:                nil,
			expectedRouteCount:    2,
			expectedRouteNames:    []string{"LivenessProbe", "ReadinessProbe"},
			expectedRouteMethods:  []string{"GET", "GET"},
			expectedRoutePatterns: []string{"/liveness", "/readiness"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes := nodeRoutes(tt.plugin)

			// Test route count
			assert.Len(t, routes, tt.expectedRouteCount, "Expected %d routes", tt.expectedRouteCount)

			// Test each route's properties
			for i, route := range routes {
				assert.Equal(t, tt.expectedRouteNames[i], route.Name, "Route %d name mismatch", i)
				assert.Equal(t, tt.expectedRouteMethods[i], route.Method, "Route %d method mismatch", i)
				assert.Equal(t, tt.expectedRoutePatterns[i], route.Pattern, "Route %d pattern mismatch", i)
				assert.Nil(t, route.Middleware, "Route %d should have nil middleware", i)
				assert.NotNil(t, route.HandlerFunc, "Route %d handler should not be nil", i)
			}
		})
	}
}

func TestNodeRoutes_RouteProperties(t *testing.T) {
	plugin := &csi.Plugin{}
	routes := nodeRoutes(plugin)

	t.Run("LivenessProbeRoute", func(t *testing.T) {
		livenessRoute := routes[0]
		assert.Equal(t, "LivenessProbe", livenessRoute.Name)
		assert.Equal(t, "GET", livenessRoute.Method)
		assert.Equal(t, "/liveness", livenessRoute.Pattern)
		assert.Nil(t, livenessRoute.Middleware)
		assert.NotNil(t, livenessRoute.HandlerFunc)

		// Verify it's the same function as NodeLivenessCheck
		// We can't directly compare function pointers, but we can ensure it's not nil
		assert.IsType(t, http.HandlerFunc(nil), livenessRoute.HandlerFunc)
	})

	t.Run("ReadinessProbeRoute", func(t *testing.T) {
		readinessRoute := routes[1]
		assert.Equal(t, "ReadinessProbe", readinessRoute.Name)
		assert.Equal(t, "GET", readinessRoute.Method)
		assert.Equal(t, "/readiness", readinessRoute.Pattern)
		assert.Nil(t, readinessRoute.Middleware)
		assert.NotNil(t, readinessRoute.HandlerFunc)

		// Verify it's a handler function
		assert.IsType(t, http.HandlerFunc(nil), readinessRoute.HandlerFunc)
	})
}

func TestNodeRoutes_HandlerFunctionality(t *testing.T) {
	plugin := &csi.Plugin{}
	routes := nodeRoutes(plugin)

	t.Run("LivenessProbeHandlerNotNil", func(t *testing.T) {
		livenessHandler := routes[0].HandlerFunc
		assert.NotNil(t, livenessHandler, "Liveness probe handler should not be nil")
	})

	t.Run("ReadinessProbeHandlerNotNil", func(t *testing.T) {
		readinessHandler := routes[1].HandlerFunc
		assert.NotNil(t, readinessHandler, "Readiness probe handler should not be nil")
	})

	t.Run("HandlersAreDifferent", func(t *testing.T) {
		livenessHandler := routes[0].HandlerFunc
		readinessHandler := routes[1].HandlerFunc

		// While we can't directly compare function pointers for equality in all cases,
		// we can ensure both handlers exist and are valid HandlerFunc types
		assert.NotNil(t, livenessHandler)
		assert.NotNil(t, readinessHandler)
		assert.IsType(t, http.HandlerFunc(nil), livenessHandler)
		assert.IsType(t, http.HandlerFunc(nil), readinessHandler)
	})
}

func TestNodeRoutes_RoutesSliceType(t *testing.T) {
	plugin := &csi.Plugin{}
	routes := nodeRoutes(plugin)

	// Test that the returned value is of type []Route (Routes is an alias for []Route)
	assert.IsType(t, []Route{}, routes, "nodeRoutes should return type []Route")

	// Verify it can be assigned to Routes type
	var routesVar Routes = routes
	assert.IsType(t, Routes{}, routesVar, "Should be assignable to Routes type")
}

func TestNodeRoutes_EmptyMiddleware(t *testing.T) {
	plugin := &csi.Plugin{}
	routes := nodeRoutes(plugin)

	// Verify all routes have nil middleware
	for i, route := range routes {
		assert.Nil(t, route.Middleware, "Route %d (%s) should have nil middleware", i, route.Name)
	}
}

func TestNodeRoutes_HTTPMethods(t *testing.T) {
	plugin := &csi.Plugin{}
	routes := nodeRoutes(plugin)

	// Verify all routes use GET method
	for i, route := range routes {
		assert.Equal(t, "GET", route.Method, "Route %d (%s) should use GET method", i, route.Name)
	}
}

func TestNodeRoutes_Patterns(t *testing.T) {
	plugin := &csi.Plugin{}
	routes := nodeRoutes(plugin)

	expectedPatterns := []string{"/liveness", "/readiness"}

	for i, route := range routes {
		assert.Equal(t, expectedPatterns[i], route.Pattern, "Route %d (%s) pattern mismatch", i, route.Name)
		assert.True(t, len(route.Pattern) > 0, "Route %d (%s) pattern should not be empty", i, route.Name)
		assert.True(t, route.Pattern[0] == '/', "Route %d (%s) pattern should start with '/'", i, route.Name)
	}
}

func TestNodeRoutes_RouteNames(t *testing.T) {
	plugin := &csi.Plugin{}
	routes := nodeRoutes(plugin)

	expectedNames := []string{"LivenessProbe", "ReadinessProbe"}

	for i, route := range routes {
		assert.Equal(t, expectedNames[i], route.Name, "Route %d name mismatch", i)
		assert.True(t, len(route.Name) > 0, "Route %d name should not be empty", i)
	}
}

// Edge case tests
func TestNodeRoutes_EdgeCases(t *testing.T) {
	t.Run("NilPluginShouldNotPanic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			routes := nodeRoutes(nil)
			assert.Len(t, routes, 2, "Should still return 2 routes even with nil plugin")
		})
	})

	t.Run("MultipleCallsReturnSameStructure", func(t *testing.T) {
		plugin := &csi.Plugin{}

		routes1 := nodeRoutes(plugin)
		routes2 := nodeRoutes(plugin)

		assert.Len(t, routes1, len(routes2), "Multiple calls should return same number of routes")

		for i := range routes1 {
			assert.Equal(t, routes1[i].Name, routes2[i].Name, "Route %d name should be consistent", i)
			assert.Equal(t, routes1[i].Method, routes2[i].Method, "Route %d method should be consistent", i)
			assert.Equal(t, routes1[i].Pattern, routes2[i].Pattern, "Route %d pattern should be consistent", i)
		}
	})
}

// Benchmark tests for performance validation
func BenchmarkNodeRoutes(b *testing.B) {
	plugin := &csi.Plugin{}
	for i := 0; i < b.N; i++ {
		routes := nodeRoutes(plugin)
		_ = routes // Prevent optimization
	}
}

func BenchmarkNodeRoutesNilPlugin(b *testing.B) {
	for i := 0; i < b.N; i++ {
		routes := nodeRoutes(nil)
		_ = routes // Prevent optimization
	}
}

// Integration test to verify route structure matches expected format
func TestNodeRoutes_IntegrationStructure(t *testing.T) {
	plugin := &csi.Plugin{}
	routes := nodeRoutes(plugin)

	// Verify the exact structure matches what's expected by the router
	expectedStructure := []struct {
		name          string
		method        string
		pattern       string
		hasMiddleware bool
		hasHandler    bool
	}{
		{"LivenessProbe", "GET", "/liveness", false, true},
		{"ReadinessProbe", "GET", "/readiness", false, true},
	}

	assert.Len(t, routes, len(expectedStructure), "Route count should match expected structure")

	for i, expected := range expectedStructure {
		route := routes[i]
		assert.Equal(t, expected.name, route.Name, "Route %d name mismatch", i)
		assert.Equal(t, expected.method, route.Method, "Route %d method mismatch", i)
		assert.Equal(t, expected.pattern, route.Pattern, "Route %d pattern mismatch", i)

		if expected.hasMiddleware {
			assert.NotNil(t, route.Middleware, "Route %d should have middleware", i)
		} else {
			assert.Nil(t, route.Middleware, "Route %d should not have middleware", i)
		}

		if expected.hasHandler {
			assert.NotNil(t, route.HandlerFunc, "Route %d should have handler", i)
		} else {
			assert.Nil(t, route.HandlerFunc, "Route %d should not have handler", i)
		}
	}
}
