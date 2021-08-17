// Copyright 2019 NetApp, Inc. All Rights Reserved.

package rest

import (
	"net/http"
	"strings"

	"github.com/netapp/trident/v21/config"
)

type Route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
}

type Routes []Route

var controllerRoutes = Routes{
	Route{
		"GetVersion",
		"GET",
		strings.Replace(config.VersionURL, "/v"+config.OrchestratorAPIVersion, "", 1),
		GetVersion,
	},
	Route{
		"GetVersion",
		"GET",
		config.VersionURL,
		GetVersion,
	},
	Route{
		"AddBackend",
		"POST",
		config.BackendURL,
		AddBackend,
	},
	Route{
		"UpdateBackend",
		"POST",
		config.BackendURL + "/{backend}",
		UpdateBackend,
	},
	Route{
		"UpdateBackendState",
		"POST",
		config.BackendURL + "/{backend}" + "/state",
		UpdateBackendState,
	},
	Route{
		"GetBackend",
		"GET",
		config.BackendURL + "/{backend}",
		GetBackend,
	},
	Route{
		"ListBackends",
		"GET",
		config.BackendURL,
		ListBackends,
	},
	Route{
		"DeleteBackend",
		"DELETE",
		config.BackendURL + "/{backend}",
		DeleteBackend,
	},
	Route{
		"AddVolume",
		"POST",
		config.VolumeURL,
		AddVolume,
	},
	Route{
		"GetVolume",
		"GET",
		config.VolumeURL + "/{volume}",
		GetVolume,
	},
	Route{
		"ListVolumes",
		"GET",
		config.VolumeURL,
		ListVolumes,
	},
	Route{
		"DeleteVolume",
		"DELETE",
		config.VolumeURL + "/{volume}",
		DeleteVolume,
	},
	Route{
		"ImportVolume",
		"POST",
		config.VolumeURL + "/import",
		ImportVolume,
	},
	Route{
		"UpgradeVolume",
		"POST",
		config.VolumeURL + "/{volume}/upgrade",
		UpgradeVolume,
	},
	Route{
		"AddStorageClass",
		"POST",
		config.StorageClassURL,
		AddStorageClass,
	},
	Route{
		"GetStorageClass",
		"GET",
		config.StorageClassURL + "/{storageClass}",
		GetStorageClass,
	},
	Route{
		"ListStorageClasses",
		"GET",
		config.StorageClassURL,
		ListStorageClasses,
	},
	Route{
		"DeleteStorageClass",
		"DELETE",
		config.StorageClassURL + "/{storageClass}",
		DeleteStorageClass,
	},
	Route{
		"AddOrUpdateNode",
		"PUT",
		config.NodeURL + "/{node}",
		AddNode,
	},
	Route{
		"GetNode",
		"GET",
		config.NodeURL + "/{node}",
		GetNode,
	},
	Route{
		"ListNodes",
		"GET",
		config.NodeURL,
		ListNodes,
	},
	Route{
		"DeleteNode",
		"DELETE",
		config.NodeURL + "/{node}",
		DeleteNode,
	},
	Route{
		"ListSnapshots",
		"GET",
		config.SnapshotURL,
		ListSnapshots,
	},
	Route{
		"ListSnapshotsForVolume",
		"GET",
		config.VolumeURL + "/{volume}/snapshot",
		ListSnapshotsForVolume,
	},
	Route{
		"GetSnapshot",
		"GET",
		config.SnapshotURL + "/{volume}/{snapshot}",
		GetSnapshot,
	},
	Route{
		"AddSnapshot",
		"POST",
		config.SnapshotURL,
		AddSnapshot,
	},
	Route{
		"DeleteSnapshot",
		"DELETE",
		config.SnapshotURL + "/{volume}/{snapshot}",
		DeleteSnapshot,
	},
}
