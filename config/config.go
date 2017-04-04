// Copyright 2016 NetApp, Inc. All Rights Reserved.

package config

import (
	"strings"
	"time"
)

type Protocol string
type AccessMode string
type VolumeType string

const (
	/* Misc. orchestrator constants */
	OrchestratorName       = "trident"
	OrchestratorVersion    = "17.07.0"
	OrchestratorAPIVersion = "1"
	PersistentStoreTimeout = 60 * time.Second
	MaxBootstrapAttempts   = 10

	/* Protocol constants */
	File                Protocol = "file"
	Block               Protocol = "block"
	ProtocolAny         Protocol = ""
	ProtocolUnsupported Protocol = "unsupported"

	/* Access mode constants */
	ReadWriteOnce AccessMode = "ReadWriteOnce"
	ReadOnlyMany  AccessMode = "ReadOnlyMany"
	ReadWriteMany AccessMode = "ReadWriteMany"
	ModeAny       AccessMode = ""

	/* Volume type constants */
	ONTAP_NFS         VolumeType = "ONTAP_NFS"
	ONTAP_iSCSI       VolumeType = "ONTAP_iSCSI"
	SolidFire_iSCSI   VolumeType = "SolidFire_iSCSI"
	Eseries_iSCSI     VolumeType = "Eseries_iSCSI"
	UnknownVolumeType VolumeType = ""

	/* Driver-related constants */
	DefaultOntapIgroup      = OrchestratorName
	DefaultSolidFireVAG     = OrchestratorName
	DefaultEseriesHostGroup = OrchestratorName
	UnknownDriver           = "UnknownDriver"

	/* REST frontend constants */
	MaxRESTRequestSize = 10240
)

var (
	validProtocols = map[Protocol]bool{
		File:        true,
		Block:       true,
		ProtocolAny: true,
	}
	/* API Server and persistent store variables */
	OrchestratorMajorVersion = getMajorVersion(OrchestratorVersion)
	VersionURL               = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/version"
	BackendURL               = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/backend"
	VolumeURL                = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/volume"
	TransactionURL           = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/txn"
	StorageClassURL          = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/storageclass"
	/* Orchestrator build info variables */
	OrchestratorBuildType    = "custom"
	OrchestratorBuildTypeRev = "0"
	OrchestratorBuildHash    = "unknown"
	OrchestratorFullVersion  = OrchestratorVersion
	OrchestratorBuildVersion = "unknown"
	OrchestratorBuildTime    = "unknown"
)

func IsValidProtocol(p Protocol) bool {
	_, ok := validProtocols[p]
	return ok
}

func GetValidProtocolNames() []string {
	ret := make([]string, len(validProtocols))
	for key, _ := range validProtocols {
		ret = append(ret, string(key))
	}
	return ret
}

func getMajorVersion(version string) string {
	tmp := strings.Split(version, ".")
	if len(tmp) == 1 {
		return version
	}
	return tmp[0]
}
