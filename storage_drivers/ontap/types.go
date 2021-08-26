// Copyright 2021 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"time"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
)

type StorageDriver interface {
	GetConfig() *storagedrivers.OntapStorageDriverConfig
	GetAPI() *api.Client
	GetTelemetry() *Telemetry
	Name() string
}

type NASDriver interface {
	GetVolumeOpts(context.Context, *storage.VolumeConfig, map[string]storageattribute.Request) (
		map[string]string, error,
	)
	GetAPI() *api.Client
	GetConfig() *storagedrivers.OntapStorageDriverConfig
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
