// Copyright 2016 NetApp, Inc. All Rights Reserved.

package rest

import (
	"net/http"
	"strings"

	"github.com/netapp/trident/config"
)

type Route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
}

type Routes []Route

var routes = Routes{
	Route{
		"GetVersion",
		"GET",
		strings.Replace(config.VersionURL,
			"/v"+config.OrchestratorMajorVersion, "", 1),
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
}
