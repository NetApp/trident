// Copyright 2021 NetApp, Inc. All Rights Reserved.

package config

import (
	"io/ioutil"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	log.SetOutput(ioutil.Discard)
	os.Exit(m.Run())
}

func TestPlatformAtLeast(t *testing.T) {
	tests := []struct {
		platformName string
		version      string
		result       bool
	}{
		{platformName: "docker", version: "v1.6.0", result: false},
		{platformName: "kubernetes", version: "v1.7.0", result: true},
		{platformName: "kubernetes", version: "v1.6.0", result: true},
		{platformName: "kubernetes", version: "v1.9.0", result: true},
		{platformName: "kubernetes", version: "v1.12.0", result: false},
		{platformName: "kubernetes", version: "x123", result: false},
	}

	OrchestratorTelemetry.Platform = "kubernetes"
	OrchestratorTelemetry.PlatformVersion = "v1.9.0"
	for _, test := range tests {
		result := PlatformAtLeast(test.platformName, test.version)
		if result != test.result {
			t.Errorf("Failed platform test. %s %s result: %v", test.platformName, test.version, result)
		}
	}
}
