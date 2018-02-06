// Copyright 2018 NetApp, Inc. All Rights Reserved.

package api

const HTTPUnprocessableEntity = 422 // Not defined in net/http package

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

// Add array to Web Services Proxy
type MsgConnect struct {
	ControllerAddresses []string `json:"controllerAddresses"`
	Password            string   `json:"password,omitempty"`
}

type MsgConnectResponse struct {
	ArrayID       string `json:"id"`
	AlreadyExists bool   `json:"alreadyExists"`
}

type Controller struct {
	Active                   bool   `json:"active"`
	Quiesced                 bool   `json:"quiesced"`
	Status                   string `json:"status"`
	ControllerRef            string `json:"controllerRef"`
	Manufacturer             string `json:"manufacturer"`
	ManufacturerDate         string `json:"manufacturerDate"`
	AppVersion               string `json:"appVersion"`
	BootVersion              string `json:"bootVersion"`
	ProductID                string `json:"productID"`
	ProductRevLevel          string `json:"productRevLevel"`
	SerialNumber             string `json:"serialNumber"`
	BoardID                  string `json:"boardID"`
	CacheMemorySize          int    `json:"cacheMemorySize"`
	ProcessorMemorySize      int    `json:"processorMemorySize"`
	Reserved1                string `json:"reserved1"`
	Reserved2                string `json:"reserved2"`
	HostBoardID              string `json:"hostBoardID"`
	PhysicalCacheMemorySize  int    `json:"physicalCacheMemorySize"`
	ReadyToRemove            bool   `json:"readyToRemove"`
	BoardSubmodelID          string `json:"boardSubmodelID"`
	SubmodelSupported        bool   `json:"submodelSupported"`
	OemPartNumber            string `json:"oemPartNumber"`
	PartNumber               string `json:"partNumber"`
	BootTime                 string `json:"bootTime"`
	ModelName                string `json:"modelName"`
	FlashCacheMemorySize     int    `json:"flashCacheMemorySize"`
	LocateInProgress         bool   `json:"locateInProgress"`
	HasTrayIdentityIndicator bool   `json:"hasTrayIdentityIndicator"`
	ControllerErrorMode      string `json:"controllerErrorMode"`
	ID                       string `json:"id"`
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

type VolumeTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
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
	InitiatorRef string             `json:"initiatorRef"`
	NodeName     HostExScsiNodeName `json:"nodeName"`
	Label        string             `json:"label"`
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
