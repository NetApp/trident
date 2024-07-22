// Copyright 2022 NetApp, Inc. All Rights Reserved.

package main

import (
	"context"
	"io"
	"math"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"

	"github.com/netapp/trident/config"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

func TestMain_processCommandLineArgs_QPS(t *testing.T) {
	type parameters struct {
		k8sApiQPS         float64
		expectedK8sApiQPS float32
	}

	tests := map[string]parameters{
		"QPS is set to 100.0": {
			k8sApiQPS:         100.0,
			expectedK8sApiQPS: 100.0,
		},
		"QPS is set to maxFloat32": {
			k8sApiQPS:         math.MaxFloat32,
			expectedK8sApiQPS: math.MaxFloat32,
		},
		"QPS is set to maxFloat64 + 1": {
			k8sApiQPS:         math.MaxFloat64 + 1,
			expectedK8sApiQPS: config.DefaultK8sAPIQPS,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			csiEndpoint = pointer.String("true")
			useInMemory = pointer.Bool(true)
			k8sApiQPS = &params.k8sApiQPS
			processCmdLineArgs(context.TODO())

			assert.Equal(t, params.expectedK8sApiQPS, config.K8sAPIQPS)
		})
	}
}
