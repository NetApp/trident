// Copyright 2018 NetApp, Inc. All Rights Reserved.

package api

import "fmt"

var HostTypes = map[string]string{
	"linux_atto":        "LnxTPGSALUA",
	"linux_dm_mp":       "LnxALUA",
	"linux_mpp_rdac":    "LNX",
	"linux_pathmanager": "LnxTPGSALUA_PM",
	"linux_sf":          "LnxTPGSALUA_SF",
	"ontap":             "ONTAP_ALUA",
	"ontap_rdac":        "ONTAP_RDAC",
	"vmware":            "VmwTPGSALUA",
	"windows":           "W2KNETNCL",
	"windows_atto":      "WinTPGSALUA",
	"windows_clustered": "W2KNETCL",
}

// Error wrapper
type Error struct {
	Code    int
	Message string
}

func (e Error) Error() string {
	return fmt.Sprintf("device API error: %s, Status code: %d", e.Message, e.Code)
}

// Add array to Web Services Proxy
type MsgConnect struct {
	ControllerAddresses []string `json:"controllerAddresses"`
	Password            string   `json:"password,omitempty"`
}

type MsgConnectResponse struct {
	ArrayID       string `json:"id"`
	AlreadyExists bool   `json:"alreadyExists"`
}

// AboutResponse includes basic information about the system.
type AboutResponse struct {
	RunningAsProxy     bool   `json:"runningAsProxy"`     // "runningAsProxy": false,
	Version            string `json:"version"`            // "version": "03.20.9000.0004",
	SystemID           string `json:"systemId"`           // "systemId": "de679125-1c38-4356-b175-810c8b536d6a",
	ControllerPosition int    `json:"controllerPosition"` // "controllerPosition": 1,
	StartTimestamp     string `json:"startTimestamp"`     // "startTimestamp": "1573141561",
	SamlEnabled        bool   `json:"samlEnabled"`        // "samlEnabled": false
}

type StorageSystem struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Wwn             string   `json:"wwn"`
	PasswordStatus  string   `json:"passwordStatus"`
	PasswordSet     bool     `json:"passwordSet"`
	Status          string   `json:"status"`
	IP1             string   `json:"ip1"`
	IP2             string   `json:"ip2"`
	ManagementPaths []string `json:"managementPaths"`
	Controllers     []struct {
		ControllerID      string   `json:"controllerId"`
		IPAddresses       []string `json:"ipAddresses"`
		CertificateStatus string   `json:"certificateStatus"`
	} `json:"controllers"`
	DriveCount              int           `json:"driveCount"`
	TrayCount               int           `json:"trayCount"`
	TraceEnabled            bool          `json:"traceEnabled"`
	Types                   string        `json:"types"`
	Model                   string        `json:"model"`
	MetaTags                []interface{} `json:"metaTags"`
	HotSpareSize            string        `json:"hotSpareSize"`
	UsedPoolSpace           string        `json:"usedPoolSpace"`
	FreePoolSpace           string        `json:"freePoolSpace"`
	UnconfiguredSpace       string        `json:"unconfiguredSpace"`
	DriveTypes              []string      `json:"driveTypes"`
	HostSpareCountInStandby int           `json:"hostSpareCountInStandby"`
	HotSpareCount           int           `json:"hotSpareCount"`
	HostSparesUsed          int           `json:"hostSparesUsed"`
	BootTime                string        `json:"bootTime"`
	FwVersion               string        `json:"fwVersion"`
	AppVersion              string        `json:"appVersion"`
	BootVersion             string        `json:"bootVersion"`
	NvsramVersion           string        `json:"nvsramVersion"`
	ChassisSerialNumber     string        `json:"chassisSerialNumber"`
	AccessVolume            struct {
		Enabled               bool   `json:"enabled"`
		VolumeHandle          int    `json:"volumeHandle"`
		Capacity              string `json:"capacity"`
		AccessVolumeRef       string `json:"accessVolumeRef"`
		Reserved1             string `json:"reserved1"`
		ObjectType            string `json:"objectType"`
		Wwn                   string `json:"wwn"`
		PreferredControllerID string `json:"preferredControllerId"`
		TotalSizeInBytes      string `json:"totalSizeInBytes"`
		ListOfMappings        []struct {
			LunMappingRef string `json:"lunMappingRef"`
			Lun           int    `json:"lun"`
			Ssid          int    `json:"ssid"`
			Perms         int    `json:"perms"`
			VolumeRef     string `json:"volumeRef"`
			Type          string `json:"type"`
			MapRef        string `json:"mapRef"`
			ID            string `json:"id"`
		} `json:"listOfMappings"`
		Mapped              bool   `json:"mapped"`
		CurrentControllerID string `json:"currentControllerId"`
		Name                string `json:"name"`
		ID                  string `json:"id"`
	} `json:"accessVolume"`
	UnconfiguredSpaceByDriveType struct {
	} `json:"unconfiguredSpaceByDriveType"`
	MediaScanPeriod                  int         `json:"mediaScanPeriod"`
	DriveChannelPortDisabled         bool        `json:"driveChannelPortDisabled"`
	RecoveryModeEnabled              bool        `json:"recoveryModeEnabled"`
	AutoLoadBalancingEnabled         bool        `json:"autoLoadBalancingEnabled"`
	HostConnectivityReportingEnabled bool        `json:"hostConnectivityReportingEnabled"`
	RemoteMirroringEnabled           bool        `json:"remoteMirroringEnabled"`
	FcRemoteMirroringState           string      `json:"fcRemoteMirroringState"`
	AsupEnabled                      bool        `json:"asupEnabled"`
	SecurityKeyEnabled               bool        `json:"securityKeyEnabled"`
	ExternalKeyEnabled               bool        `json:"externalKeyEnabled"`
	LastContacted                    string      `json:"lastContacted"`
	DefinedPartitionCount            int         `json:"definedPartitionCount"`
	SimplexModeEnabled               bool        `json:"simplexModeEnabled"`
	EnabledManagementPorts           interface{} `json:"enabledManagementPorts"`
	FreePoolSpaceAsString            string      `json:"freePoolSpaceAsString"`
	HotSpareSizeAsString             string      `json:"hotSpareSizeAsString"`
	UnconfiguredSpaceAsStrings       string      `json:"unconfiguredSpaceAsStrings"`
	UsedPoolSpaceAsString            string      `json:"usedPoolSpaceAsString"`
}

type VolumeGroupEx struct {
	IsOffline      bool   `json:"offline"`
	WorldWideName  string `json:"worldWideName"`
	VolumeGroupRef string `json:"volumeGroupRef"`
	Label          string `json:"label"`
	FreeSpace      string `json:"freeSpace"`      // Documentation says this is an int but really it is a string!
	DriveMediaType string `json:"driveMediaType"` // 'hdd', 'ssd'
}

// Functions to allow sorting storage pools by free space
type ByFreeSpace []VolumeGroupEx

func (s ByFreeSpace) Len() int {
	return len(s)
}
func (s ByFreeSpace) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s ByFreeSpace) Less(i, j int) bool {
	return len(s[i].FreeSpace) < len(s[j].FreeSpace)
}

type VolumeCreateRequest struct {
	VolumeGroupRef   string      `json:"poolId"`
	Name             string      `json:"name"`
	SizeUnit         string      `json:"sizeUnit"` //bytes, b, kb, mb, gb, tb, pb, eb, zb, yb
	Size             int         `json:"size"`
	SegmentSize      int         `json:"segSize"`
	DataAssurance    bool        `json:"dataAssuranceEnabled,omitempty"`
	OwningController string      `json:"owningControllerId,omitempty"`
	VolumeTags       []VolumeTag `json:"metaTags,omitempty"`
}

// VolumeUpdateRequest is used to update a volume
type VolumeUpdateRequest struct {
	Name       string      `json:"name,omitempty"`     // the new name (to rename it)
	VolumeTags []VolumeTag `json:"metaTags,omitempty"` // Key/Value pair for volume meta data
}

type VolumeTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (t VolumeTag) Equals(otherTag VolumeTag) bool {
	return t.Key == otherTag.Key && t.Value == otherTag.Value
}

type VolumeEx struct {
	IsOffline      bool         `json:"offline"`
	Label          string       `json:"label"`
	VolumeSize     string       `json:"capacity"`
	SegmentSize    int          `json:"segmentSize"`
	VolumeRef      string       `json:"volumeRef"`
	VolumeGroupRef string       `json:"volumeGroupRef"`
	Mappings       []LUNMapping `json:"listOfMappings"`
	IsMapped       bool         `json:"mapped"`
	VolumeTags     []VolumeTag  `json:"metadata"`
}

type VolumeResizeRequest struct {
	ExpansionSize int    `json:"expansionSize"`
	SizeUnit      string `json:"sizeUnit"` //bytes, b, kb, mb, gb, tb, pb, eb, zb, yb
}

type VolumeResizeStatusResponse struct {
	PercentComplete  int    `json:"percentComplete"`
	TimeToCompletion int    `json:"timeToCompletion"`
	Action           string `json:"action"`
}

type HostCreateRequest struct {
	Name     string `json:"name"`
	HostType `json:"hostType"`
	GroupID  string     `json:"groupId,omitempty"`
	Ports    []HostPort `json:"ports"`
}

type HostType struct {
	Name  string `json:"name,omitempty"`
	Index int    `json:"index"`
	Code  string `json:"code,omitempty"`
}

type HostPort struct {
	Type  string `json:"type"`
	Port  string `json:"port"`
	Label string `json:"label"`
}

type HostEx struct {
	HostRef       string            `json:"hostRef"`
	ClusterRef    string            `json:"clusterRef"`
	Label         string            `json:"label"`
	HostTypeIndex int               `json:"hostTypeIndex"`
	Initiators    []HostExInitiator `json:"initiators"`
}

type HostExInitiator struct {
	InitiatorRef      string                  `json:"initiatorRef"`
	NodeName          HostExScsiNodeName      `json:"nodeName"`
	Label             string                  `json:"label"`
	InitiatorNodeName HostExInitiatorNodeName `json:"initiatorNodeName"`
}

type HostExInitiatorNodeName struct {
	NodeName      HostExScsiNodeName `json:"nodeName"`
	InterfaceType string             `json:"interfaceType"`
}

type HostExScsiNodeName struct {
	IoInterfaceType string `json:"ioInterfaceType"`
	IscsiNodeName   string `json:"iscsiNodeName"`
}

type HostGroupCreateRequest struct {
	Name  string   `json:"name"`
	Hosts []string `json:"hosts"`
}

type HostGroup struct {
	ClusterRef string `json:"clusterRef"`
	Label      string `json:"label"`
}

type VolumeMappingCreateRequest struct {
	MappableObjectID string `json:"mappableObjectId"`
	TargetID         string `json:"targetId"`
	LunNumber        int    `json:"lun,omitempty"`
}

type LUNMapping struct {
	LunMappingRef string `json:"lunMappingRef"`
	LunNumber     int    `json:"lun"`
	VolumeRef     string `json:"volumeRef"`
	MapRef        string `json:"mapRef"`
	Type          string `json:"type"`
}

// Used for errors on RESTful calls to return what went wrong
type CallResponseError struct {
	ErrorMsg     string `json:"errorMessage"`
	LocalizedMsg string `json:"localizedMessage"`
	ReturnCode   string `json:"retcode"`
	CodeType     string `json:"codeType"` //'symbol', 'webservice', 'systemerror', 'devicemgrerror'
}

type IscsiTargetSettings struct {
	TargetRef string `json:"targetRef"`
	NodeName  struct {
		IoInterfaceType string      `json:"ioInterfaceType"`
		IscsiNodeName   string      `json:"iscsiNodeName"`
		RemoteNodeWWN   interface{} `json:"remoteNodeWWN"`
	} `json:"nodeName"`
	Alias struct {
		IoInterfaceType string `json:"ioInterfaceType"`
		IscsiAlias      string `json:"iscsiAlias"`
	} `json:"alias"`
	ConfiguredAuthMethods struct {
		AuthMethodData []struct {
			AuthMethod string      `json:"authMethod"`
			ChapSecret interface{} `json:"chapSecret"`
		} `json:"authMethodData"`
	} `json:"configuredAuthMethods"`
	Portals []struct {
		GroupTag  int `json:"groupTag"`
		IPAddress struct {
			AddressType string      `json:"addressType"`
			Ipv4Address string      `json:"ipv4Address"`
			Ipv6Address interface{} `json:"ipv6Address"`
		} `json:"ipAddress"`
		TCPListenPort int `json:"tcpListenPort"`
	} `json:"portals"`
}
