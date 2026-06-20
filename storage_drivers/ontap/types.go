// Copyright 2026 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"time"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
)

type StorageDriver interface {
	GetOntapConfig() *drivers.OntapStorageDriverConfig
	GetAPI() api.OntapAPI
	GetTelemetry() *Telemetry
	Name() string
}

type NASDriver interface {
	GetVolumeOpts(context.Context, *storage.VolumeConfig, map[string]sa.Request) map[string]string
	GetAPI() api.OntapAPI
	GetOntapConfig() *drivers.OntapStorageDriverConfig
}

type Telemetry struct {
	config.Telemetry
	Plugin        string        `json:"plugin"`
	SVM           string        `json:"svm"`
	StoragePrefix string        `json:"storagePrefix"`
	Driver        StorageDriver `json:"-"`
	done          chan struct{}
	ticker        *time.Ticker
	stopped       bool
}

// ChapCredentials holds the bidrectional chap settings
type ChapCredentials struct {
	ChapUsername              string
	ChapInitiatorSecret       string
	ChapTargetUsername        string
	ChapTargetInitiatorSecret string
}

type ontapPerformanceClass string

type VolumeMoveContext struct {
	JobID        string       `json:"jobID,omitempty"`
	JobState     api.JobState `json:"jobState,omitempty"`
	JobStartTime time.Time    `json:"jobStartTime,omitempty"`
	JobEndTime   time.Time    `json:"jobEndTime,omitempty"`
	SVMName      string       `json:"svmName,omitempty"`
	BackendName  string       `json:"backendName,omitempty"`
}
