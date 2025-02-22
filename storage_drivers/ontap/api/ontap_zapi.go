// Copyright 2025 NetApp, Inc. All Rights Reserved.

package api

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/utils/errors"
	versionutils "github.com/netapp/trident/utils/version"
)

// //////////////////////////////////////////////////////////////////////////////////////////////////////
// ZAPI layer
// //////////////////////////////////////////////////////////////////////////////////////////////////////

const (
	DefaultZapiRecords   = 100
	MaxZapiRecords       = 0xfffffffe
	NumericalValueNotSet = -1
	maxFlexGroupWait     = 120 * time.Second

	MaxNASLabelLength = 1023
	MaxSANLabelLength = 254

	// errors
	ESNAPSHOTBUSY_REST = "1638555"

	SVMSubtypeSyncSource      = "sync_source"
	SVMSubtypeSyncDestination = "sync_destination"
)

var (
	// must be string pointers, but cannot take address of a const, so don't modify these at runtime!
	LifOperationalStatusUp   = "up"
	LifOperationalStatusDown = "down"
)

// ClientConfig holds the configuration data for Client objects
type ClientConfig struct {
	ManagementLIF                  string
	Username                       string
	Password                       string
	ClientPrivateKey               string
	ClientCertificate              string
	TrustedCACertificate           string
	DriverContext                  tridentconfig.DriverContext
	ContextBasedZapiRecords        int
	DebugTraceFlags                map[string]bool
	unitTestTransportConfigSchemes string
}

// Client is the object to use for interacting with ONTAP controllers
type Client struct {
	config     ClientConfig
	zr         *azgo.ZapiRunner
	m          *sync.Mutex
	driverName string
	svmUUID    string
	svmMCC     bool
}

// NewClient is a factory method for creating a new instance
func NewClient(config ClientConfig, SVM, driverName string) *Client {
	// When running in Docker context we want to request MAX number of records from ZAPI for Volume, LUNs and Qtrees
	config.ContextBasedZapiRecords = DefaultZapiRecords
	if config.DriverContext == tridentconfig.ContextDocker {
		config.ContextBasedZapiRecords = MaxZapiRecords
	}

	d := &Client{
		driverName: driverName,
		config:     config,
		zr: &azgo.ZapiRunner{
			ManagementLIF:        config.ManagementLIF,
			SVM:                  SVM,
			Username:             config.Username,
			Password:             config.Password,
			ClientPrivateKey:     config.ClientPrivateKey,
			ClientCertificate:    config.ClientCertificate,
			TrustedCACertificate: config.TrustedCACertificate,
			Secure:               true,
			DebugTraceFlags:      config.DebugTraceFlags,
		},
		m: &sync.Mutex{},
	}
	return d
}

func (c *Client) ClientConfig() ClientConfig {
	return c.config
}

func (c *Client) SetSVMUUID(svmUUID string) {
	c.svmUUID = svmUUID
}

func (c *Client) SVMUUID() string {
	return c.svmUUID
}

func (c *Client) SetSVMMCC(mcc bool) {
	c.svmMCC = mcc
}

func (c *Client) SVMMCC() bool {
	return c.svmMCC
}

func (c *Client) SVMName() string {
	return c.zr.SVM
}

// GetClonedZapiRunner returns a clone of the ZapiRunner configured on this driver.
func (c Client) GetClonedZapiRunner() *azgo.ZapiRunner {
	clone := new(azgo.ZapiRunner)
	*clone = *c.zr
	return clone
}

// GetNontunneledZapiRunner returns a clone of the ZapiRunner configured on this driver with the SVM field cleared so ZAPI calls
// made with the resulting runner aren't tunneled.  Note that the calls could still go directly to either a cluster or
// vserver management LIF.
func (c Client) GetNontunneledZapiRunner() *azgo.ZapiRunner {
	clone := new(azgo.ZapiRunner)
	*clone = *c.zr
	clone.SVM = ""
	return clone
}

type QosPolicyGroupKindType int

const (
	InvalidQosPolicyGroupKind QosPolicyGroupKindType = iota
	QosPolicyGroupKind
	QosAdaptivePolicyGroupKind
)

type QosPolicyGroup struct {
	Name string
	Kind QosPolicyGroupKindType
}

func NewQosPolicyGroup(qosPolicy, adaptiveQosPolicy string) (QosPolicyGroup, error) {
	switch {
	case qosPolicy != "" && adaptiveQosPolicy != "":
		return QosPolicyGroup{}, fmt.Errorf("only one kind of QoS policy group may be defined")
	case qosPolicy != "":
		return QosPolicyGroup{
			Name: qosPolicy,
			Kind: QosPolicyGroupKind,
		}, nil
	case adaptiveQosPolicy != "":
		return QosPolicyGroup{
			Name: adaptiveQosPolicy,
			Kind: QosAdaptivePolicyGroupKind,
		}, nil
	default:
		return QosPolicyGroup{
			Kind: InvalidQosPolicyGroupKind,
		}, nil
	}
}

// ///////////////////////////////////////////////////////////////////////////
// API feature operations BEGIN

// API functions are named in a NounVerb pattern. This reflects how the azgo
// functions are also named. (i.e. VolumeGet instead of GetVolume)

type Feature string

// Define new version-specific feature constants here
const (
	MinimumONTAPIVersion      Feature = "MINIMUM_ONTAPI_VERSION"
	NetAppFlexGroups          Feature = "NETAPP_FLEXGROUPS"
	NetAppFlexGroupsClone     Feature = "NETAPP_FLEXGROUPS_CLONE_ONTAPI_MINIMUM"
	NetAppFabricPoolFlexVol   Feature = "NETAPP_FABRICPOOL_FLEXVOL"
	NetAppFabricPoolFlexGroup Feature = "NETAPP_FABRICPOOL_FLEXGROUP"
	LunGeometrySkip           Feature = "LUN_GEOMETRY_SKIP"
	FabricPoolForSVMDR        Feature = "FABRICPOOL_FOR_SVMDR"
	QosPolicies               Feature = "QOS_POLICIES"
	LIFServices               Feature = "LIF_SERVICES"
	NVMeProtocol              Feature = "NVME_PROTOCOL"
)

// Indicate the minimum Ontapi version for each feature here
var features = map[Feature]*versionutils.Version{
	MinimumONTAPIVersion:      versionutils.MustParseSemantic("1.150.0"), // cDOT 9.5.0
	NetAppFlexGroups:          versionutils.MustParseSemantic("1.120.0"), // cDOT 9.2.0
	NetAppFlexGroupsClone:     versionutils.MustParseSemantic("1.170.0"), // cDOT 9.7.0
	NetAppFabricPoolFlexVol:   versionutils.MustParseSemantic("1.120.0"), // cDOT 9.2.0
	NetAppFabricPoolFlexGroup: versionutils.MustParseSemantic("1.150.0"), // cDOT 9.5.0
	FabricPoolForSVMDR:        versionutils.MustParseSemantic("1.150.0"), // cDOT 9.5.0
	QosPolicies:               versionutils.MustParseSemantic("1.180.0"), // cDOT 9.8.0
	LIFServices:               versionutils.MustParseSemantic("1.160.0"), // cDOT 9.6.0
	// TODO(sphadnis): Check if all the Zapi calls work on this version once Zapi APIs for NVMe are implemented.
	NVMeProtocol: versionutils.MustParseSemantic("1.201.0"), // cDOT 9.10.1
}

// Indicate the minimum Ontap version for each feature here (non-API specific)
var featuresByVersion = map[Feature]*versionutils.Version{
	MinimumONTAPIVersion:      versionutils.MustParseSemantic("9.5.0"),
	NetAppFlexGroups:          versionutils.MustParseSemantic("9.2.0"),
	NetAppFlexGroupsClone:     versionutils.MustParseSemantic("9.7.0"),
	NetAppFabricPoolFlexVol:   versionutils.MustParseSemantic("9.2.0"),
	NetAppFabricPoolFlexGroup: versionutils.MustParseSemantic("9.5.0"),
	FabricPoolForSVMDR:        versionutils.MustParseSemantic("9.5.0"),
	QosPolicies:               versionutils.MustParseSemantic("9.8.0"),
	LIFServices:               versionutils.MustParseSemantic("9.6.0"),
	NVMeProtocol:              versionutils.MustParseSemantic("9.10.1"),
}

var MaximumONTAPIVersion = versionutils.MustParseMajorMinorVersion("9.99")

// SupportsFeature returns true if the Ontapi version supports the supplied feature
func (c Client) SupportsFeature(ctx context.Context, feature Feature) bool {
	ontapiVersion, err := c.SystemGetOntapiVersion(ctx, true)
	if err != nil {
		return false
	}

	ontapiSemVer, err := versionutils.ParseSemantic(fmt.Sprintf("%s.0", ontapiVersion))
	if err != nil {
		return false
	}

	if minVersion, ok := features[feature]; ok {
		return ontapiSemVer.AtLeast(minVersion)
	} else {
		return false
	}
}

func IsZAPISupported(version string) (bool, error) {
	ontapSemVer, err := versionutils.ParseSemantic(version)
	if err != nil {
		return false, err
	}

	if ontapSemVer.ToMajorMinorVersion().GreaterThan(MaximumONTAPIVersion) {
		return false, nil
	}

	return true, nil
}

// API feature operations END
// ///////////////////////////////////////////////////////////////////////////

// ///////////////////////////////////////////////////////////////////////////
// IGROUP operations BEGIN

// IgroupCreate creates the specified initiator group
// equivalent to filer::> igroup create docker -vserver iscsi_vs -protocol iscsi -ostype linux
func (c Client) IgroupCreate(
	initiatorGroupName, initiatorGroupType, osType string,
) (*azgo.IgroupCreateResponse, error) {
	response, err := azgo.NewIgroupCreateRequest().
		SetInitiatorGroupName(initiatorGroupName).
		SetInitiatorGroupType(initiatorGroupType).
		SetOsType(osType).
		ExecuteUsing(c.zr)
	return response, err
}

// IgroupAdd adds an initiator to an initiator group
// equivalent to filer::> igroup add -vserver iscsi_vs -igroup docker -initiator iqn.1993-08.org.debian:01:9031309bbebd
func (c Client) IgroupAdd(initiatorGroupName, initiator string) (*azgo.IgroupAddResponse, error) {
	response, err := azgo.NewIgroupAddRequest().
		SetInitiatorGroupName(initiatorGroupName).
		SetInitiator(initiator).
		ExecuteUsing(c.zr)
	return response, err
}

// IgroupRemove removes an initiator from an initiator group
func (c Client) IgroupRemove(initiatorGroupName, initiator string, force bool) (*azgo.IgroupRemoveResponse, error) {
	response, err := azgo.NewIgroupRemoveRequest().
		SetInitiatorGroupName(initiatorGroupName).
		SetInitiator(initiator).
		SetForce(force).
		ExecuteUsing(c.zr)
	return response, err
}

// IgroupDestroy destroys an initiator group
func (c Client) IgroupDestroy(initiatorGroupName string) (*azgo.IgroupDestroyResponse, error) {
	response, err := azgo.NewIgroupDestroyRequest().
		SetInitiatorGroupName(initiatorGroupName).
		ExecuteUsing(c.zr)
	return response, err
}

// IgroupList lists initiator groups
func (c Client) IgroupList() (*azgo.IgroupGetIterResponse, error) {
	response, err := azgo.NewIgroupGetIterRequest().
		SetMaxRecords(DefaultZapiRecords).
		ExecuteUsing(c.zr)
	return response, err
}

// IgroupGet gets a specified initiator group
func (c Client) IgroupGet(initiatorGroupName string) (*azgo.InitiatorGroupInfoType, error) {
	query := &azgo.IgroupGetIterRequestQuery{}
	iGroupInfo := azgo.NewInitiatorGroupInfoType().
		SetInitiatorGroupName(initiatorGroupName)
	query.SetInitiatorGroupInfo(*iGroupInfo)

	response, err := azgo.NewIgroupGetIterRequest().
		SetQuery(*query).
		ExecuteUsing(c.zr)
	if err != nil {
		return &azgo.InitiatorGroupInfoType{}, err
	} else if response.Result.NumRecords() == 0 {
		return &azgo.InitiatorGroupInfoType{}, fmt.Errorf("igroup %s not found", initiatorGroupName)
	} else if response.Result.NumRecords() > 1 {
		return &azgo.InitiatorGroupInfoType{}, fmt.Errorf("more than one igroup %s found", initiatorGroupName)
	} else if response.Result.AttributesListPtr == nil {
		return &azgo.InitiatorGroupInfoType{}, fmt.Errorf("igroup %s not found", initiatorGroupName)
	} else if response.Result.AttributesListPtr.InitiatorGroupInfoPtr != nil {
		return &response.Result.AttributesListPtr.InitiatorGroupInfoPtr[0], nil
	}
	return &azgo.InitiatorGroupInfoType{}, fmt.Errorf("igroup %s not found", initiatorGroupName)
}

// IGROUP operations END
// ///////////////////////////////////////////////////////////////////////////

// ///////////////////////////////////////////////////////////////////////////
// LUN operations BEGIN

// LunCreate creates a lun with the specified attributes
// equivalent to filer::> lun create -vserver iscsi_vs -path /vol/v/lun1 -size 1g -ostype linux -space-reserve disabled -space-allocation enabled
func (c Client) LunCreate(
	lunPath string, sizeInBytes int, osType string, qosPolicyGroup QosPolicyGroup, spaceReserved bool,
	spaceAllocated bool,
) (*azgo.LunCreateBySizeResponse, error) {
	if strings.Contains(lunPath, failureLUNCreate) {
		return nil, errors.New("injected error")
	}

	request := azgo.NewLunCreateBySizeRequest().
		SetPath(lunPath).
		SetSize(sizeInBytes).
		SetOstype(osType).
		SetSpaceReservationEnabled(spaceReserved).
		SetSpaceAllocationEnabled(spaceAllocated)

	switch qosPolicyGroup.Kind {
	case QosPolicyGroupKind:
		request.SetQosPolicyGroup(qosPolicyGroup.Name)
	case QosAdaptivePolicyGroupKind:
		request.SetQosAdaptivePolicyGroup(qosPolicyGroup.Name)
	}

	response, err := request.ExecuteUsing(c.zr)
	return response, err
}

// LunCloneCreate clones a LUN from a snapshot
func (c Client) LunCloneCreate(
	volumeName, sourceLun, destinationLun string, qosPolicyGroup QosPolicyGroup,
) (*azgo.CloneCreateResponse, error) {
	request := azgo.NewCloneCreateRequest().
		SetVolume(volumeName).
		SetSourcePath(sourceLun).
		SetDestinationPath(destinationLun)

	switch qosPolicyGroup.Kind {
	case QosAdaptivePolicyGroupKind:
		request.SetQosAdaptivePolicyGroupName(qosPolicyGroup.Name)
	case QosPolicyGroupKind:
		request.SetQosPolicyGroupName(qosPolicyGroup.Name)
	}

	response, err := request.ExecuteUsing(c.zr)
	return response, err
}

// LunSetQosPolicyGroup sets the qos policy group or adaptive qos policy group on a lun; does not unset policy groups
func (c Client) LunSetQosPolicyGroup(
	lunPath string, qosPolicyGroup QosPolicyGroup,
) (*azgo.LunSetQosPolicyGroupResponse, error) {
	request := azgo.NewLunSetQosPolicyGroupRequest().
		SetPath(lunPath)

	switch qosPolicyGroup.Kind {
	case QosAdaptivePolicyGroupKind:
		request.SetQosAdaptivePolicyGroup(qosPolicyGroup.Name)
	case QosPolicyGroupKind:
		request.SetQosPolicyGroup(qosPolicyGroup.Name)
	}

	response, err := request.ExecuteUsing(c.zr)
	return response, err
}

// LunGetSerialNumber returns the serial# for a lun
func (c Client) LunGetSerialNumber(lunPath string) (*azgo.LunGetSerialNumberResponse, error) {
	response, err := azgo.NewLunGetSerialNumberRequest().
		SetPath(lunPath).
		ExecuteUsing(c.zr)
	return response, err
}

// LunMapsGetByLun returns a list of LUN map details for a given LUN path
// equivalent to filer::> lun mapping show -vserver iscsi_vs -path /vol/v/lun0
func (c Client) LunMapsGetByLun(lunPath string) (*azgo.LunMapGetIterResponse, error) {
	lunMapInfo := azgo.NewLunMapInfoType().
		SetPath(lunPath)

	query := azgo.LunMapGetIterRequestQuery{LunMapInfoPtr: lunMapInfo}

	response, err := azgo.NewLunMapGetIterRequest().
		SetQuery(query).
		ExecuteUsing(c.zr)
	return response, err
}

// LunMapsGetByIgroup returns a list of LUN map details for a given igroup
// equivalent to filer::> lun mapping show -vserver iscsi_vs -igroup trident
func (c Client) LunMapsGetByIgroup(initiatorGroupName string) (*azgo.LunMapGetIterResponse, error) {
	lunMapInfo := azgo.NewLunMapInfoType().
		SetInitiatorGroup(initiatorGroupName)

	query := azgo.LunMapGetIterRequestQuery{LunMapInfoPtr: lunMapInfo}

	response, err := azgo.NewLunMapGetIterRequest().
		SetQuery(query).
		ExecuteUsing(c.zr)
	return response, err
}

// LunMapGet returns a list of LUN map details
// equivalent to filer::> lun mapping show -vserver iscsi_vs -path /vol/v/lun0 -igroup trident
func (c Client) LunMapGet(initiatorGroupName, lunPath string) (*azgo.LunMapGetIterResponse, error) {
	lunMapInfo := *azgo.NewLunMapInfoType().
		SetInitiatorGroup(initiatorGroupName).
		SetPath(lunPath)

	response, err := azgo.NewLunMapGetIterRequest().
		SetQuery(azgo.LunMapGetIterRequestQuery{LunMapInfoPtr: &lunMapInfo}).
		ExecuteUsing(c.zr)
	return response, err
}

// LunMap maps a lun to an id in an initiator group
// equivalent to filer::> lun map -vserver iscsi_vs -path /vol/v/lun1 -igroup docker -lun-id 0
func (c Client) LunMap(initiatorGroupName, lunPath string, lunID int) (*azgo.LunMapResponse, error) {
	response, err := azgo.NewLunMapRequest().
		SetInitiatorGroup(initiatorGroupName).
		SetPath(lunPath).
		SetLunId(lunID).
		ExecuteUsing(c.zr)
	return response, err
}

// LunMapAutoID maps a LUN in an initiator group, allowing ONTAP to choose an available LUN ID
// equivalent to filer::> lun map -vserver iscsi_vs -path /vol/v/lun1 -igroup docker
func (c Client) LunMapAutoID(initiatorGroupName, lunPath string) (*azgo.LunMapResponse, error) {
	response, err := azgo.NewLunMapRequest().
		SetInitiatorGroup(initiatorGroupName).
		SetPath(lunPath).
		ExecuteUsing(c.zr)
	return response, err
}

func (c Client) LunMapIfNotMapped(ctx context.Context, initiatorGroupName, lunPath string) (int, error) {
	// Read LUN maps to see if the LUN is already mapped to the igroup
	lunMapListResponse, err := c.LunMapListInfo(lunPath)
	if err != nil {
		return -1, fmt.Errorf("problem reading maps for LUN %s: %v", lunPath, err)
	} else if lunMapListResponse.Result.ResultStatusAttr != "passed" {
		return -1, fmt.Errorf("problem reading maps for LUN %s: %+v", lunPath, lunMapListResponse.Result)
	}

	Logc(ctx).WithFields(
		LogFields{
			"lun":    lunPath,
			"igroup": initiatorGroupName,
		},
	).Debug("Checking if LUN is mapped to igroup.")

	lunID := 0
	alreadyMapped := false
	if lunMapListResponse.Result.InitiatorGroupsPtr != nil {
		for _, igroup := range lunMapListResponse.Result.InitiatorGroupsPtr.InitiatorGroupInfoPtr {
			if igroup.InitiatorGroupName() != initiatorGroupName {
				Logc(ctx).Debugf("LUN %s is mapped to igroup %s.", lunPath, igroup.InitiatorGroupName())
			}
			if igroup.InitiatorGroupName() == initiatorGroupName {

				lunID = igroup.LunId()
				alreadyMapped = true

				Logc(ctx).WithFields(LogFields{
					"lun":    lunPath,
					"igroup": initiatorGroupName,
					"id":     lunID,
				}).Debug("LUN already mapped.")

				break
			}
		}
	}

	// Map IFF not already mapped
	if !alreadyMapped {
		lunMapResponse, err := c.LunMapAutoID(initiatorGroupName, lunPath)
		if err != nil {
			return -1, fmt.Errorf("problem mapping LUN %s: %v", lunPath, err)
		} else if lunMapResponse.Result.ResultStatusAttr != "passed" {
			return -1, fmt.Errorf("problem mapping LUN %s: %+v", lunPath, lunMapResponse.Result)
		}

		lunID = lunMapResponse.Result.LunIdAssigned()

		Logc(ctx).WithFields(LogFields{
			"lun":    lunPath,
			"igroup": initiatorGroupName,
			"id":     lunID,
		}).Debug("LUN mapped.")
	}

	return lunID, nil
}

// LunMapListInfo returns lun mapping information for the specified lun
// equivalent to filer::> lun mapped show -vserver iscsi_vs -path /vol/v/lun0
func (c Client) LunMapListInfo(lunPath string) (*azgo.LunMapListInfoResponse, error) {
	response, err := azgo.NewLunMapListInfoRequest().
		SetPath(lunPath).
		ExecuteUsing(c.zr)
	return response, err
}

// LunOffline offlines a lun
// equivalent to filer::> lun offline -vserver iscsi_vs -path /vol/v/lun0
func (c Client) LunOffline(lunPath string) (*azgo.LunOfflineResponse, error) {
	response, err := azgo.NewLunOfflineRequest().
		SetPath(lunPath).
		ExecuteUsing(c.zr)
	return response, err
}

// LunOnline onlines a lun
// equivalent to filer::> lun online -vserver iscsi_vs -path /vol/v/lun0
func (c Client) LunOnline(lunPath string) (*azgo.LunOnlineResponse, error) {
	response, err := azgo.NewLunOnlineRequest().
		SetPath(lunPath).
		ExecuteUsing(c.zr)
	return response, err
}

// LunDestroy destroys a LUN
// equivalent to filer::> lun destroy -vserver iscsi_vs -path /vol/v/lun0
func (c Client) LunDestroy(lunPath string) (*azgo.LunDestroyResponse, error) {
	response, err := azgo.NewLunDestroyRequest().
		SetPath(lunPath).
		ExecuteUsing(c.zr)
	return response, err
}

// LunSetAttribute sets a named attribute for a given LUN.
func (c Client) LunSetAttribute(lunPath, name, value string) (*azgo.LunSetAttributeResponse, error) {
	if strings.Contains(lunPath, failureLUNSetAttr) {
		return nil, errors.New("injected error")
	}

	response, err := azgo.NewLunSetAttributeRequest().
		SetPath(lunPath).
		SetName(name).
		SetValue(value).
		ExecuteUsing(c.zr)
	return response, err
}

// LunGetAttribute gets a named attribute for a given LUN.
func (c Client) LunGetAttribute(ctx context.Context, lunPath, name string) (string, error) {
	response, err := azgo.NewLunGetAttributeRequest().
		SetPath(lunPath).
		SetName(name).
		ExecuteUsing(c.zr)

	if err = azgo.GetError(ctx, response, err); err != nil {
		return "", err
	}
	return response.Result.Value(), err
}

// LunGetComment gets the comment for a given LUN.
func (c Client) LunGetComment(ctx context.Context, lunPath string) (string, error) {
	lun, err := c.LunGet(lunPath)
	if err != nil {
		return "", err
	}
	if lun == nil {
		return "", fmt.Errorf("could not find LUN with name %v", lunPath)
	}
	if lun.CommentPtr == nil {
		return "", fmt.Errorf("LUN did not have a comment")
	}

	return *lun.CommentPtr, nil
}

// LunGet returns all relevant details for a single LUN
// equivalent to filer::> lun show
func (c Client) LunGet(path string) (*azgo.LunInfoType, error) {
	// Limit the LUNs to the one matching the path
	query := &azgo.LunGetIterRequestQuery{}
	lunInfo := azgo.NewLunInfoType().
		SetPath(path)
	query.SetLunInfo(*lunInfo)

	// Limit the returned data to only the data relevant to containers
	desiredAttributes := &azgo.LunGetIterRequestDesiredAttributes{}
	lunInfo = azgo.NewLunInfoType().
		SetPath("").
		SetVolume("").
		SetSize(0).
		SetCreationTimestamp(0).
		SetOnline(false).
		SetMapped(false).
		SetSerialNumber("")
	desiredAttributes.SetLunInfo(*lunInfo)

	response, err := azgo.NewLunGetIterRequest().
		SetMaxRecords(c.config.ContextBasedZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(c.zr)

	if err != nil {
		return &azgo.LunInfoType{}, err
	} else if response.Result.NumRecords() == 0 {
		return &azgo.LunInfoType{}, errors.NotFoundError(fmt.Sprintf("LUN %s not found", path))
	} else if response.Result.NumRecords() > 1 {
		return &azgo.LunInfoType{}, fmt.Errorf("more than one LUN %s found", path)
	} else if response.Result.AttributesListPtr == nil {
		return &azgo.LunInfoType{}, errors.NotFoundError(fmt.Sprintf("LUN %s not found", path))
	} else if response.Result.AttributesListPtr.LunInfoPtr != nil {
		return &response.Result.AttributesListPtr.LunInfoPtr[0], nil
	}
	return &azgo.LunInfoType{}, errors.NotFoundError(fmt.Sprintf("LUN %s not found", path))
}

func (c Client) lunGetAllCommon(query *azgo.LunGetIterRequestQuery) (*azgo.LunGetIterResponse, error) {
	// Limit the returned data to only the data relevant to containers
	desiredAttributes := &azgo.LunGetIterRequestDesiredAttributes{}
	lunInfo := azgo.NewLunInfoType().
		SetPath("").
		SetVolume("").
		SetSize(0).
		SetCreationTimestamp(0)
	desiredAttributes.SetLunInfo(*lunInfo)

	response, err := azgo.NewLunGetIterRequest().
		SetMaxRecords(c.config.ContextBasedZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(c.zr)
	return response, err
}

func (c Client) LunGetGeometry(path string) (*azgo.LunGetGeometryResponse, error) {
	response, err := azgo.NewLunGetGeometryRequest().
		SetPath(path).
		ExecuteUsing(c.zr)
	return response, err
}

func (c Client) LunResize(path string, sizeBytes int) (uint64, error) {
	response, err := azgo.NewLunResizeRequest().
		SetPath(path).
		SetSize(sizeBytes).
		ExecuteUsing(c.zr)

	var errSize uint64 = 0
	if err != nil {
		return errSize, err
	}

	if zerr := azgo.NewZapiError(response); !zerr.IsPassed() {
		return errSize, zerr
	}

	result := azgo.NewZapiResultValue(response)
	if sizePtr := result.FieldByName("ActualSizePtr"); !sizePtr.IsNil() {
		size := sizePtr.Elem().Int()
		if size < 0 {
			return errSize, fmt.Errorf("lun resize operation return an invalid size")
		} else {
			return uint64(size), nil
		}
	} else {
		return errSize, fmt.Errorf("error parsing result size")
	}
}

// LunGetAll returns all relevant details for all LUNs whose paths match the supplied pattern
// equivalent to filer::> lun show -path /vol/trident_*/*
func (c Client) LunGetAll(pathPattern string) (*azgo.LunGetIterResponse, error) {
	// Limit LUNs to those matching the pathPattern; ex, "/vol/trident_*/*"
	query := &azgo.LunGetIterRequestQuery{}
	lunInfo := azgo.NewLunInfoType().
		SetPath(pathPattern)
	query.SetLunInfo(*lunInfo)

	return c.lunGetAllCommon(query)
}

// LunGetAllForVolume returns all relevant details for all LUNs in the supplied Volume
// equivalent to filer::> lun show -volume trident_CEwDWXQRPz
func (c Client) LunGetAllForVolume(volumeName string) (*azgo.LunGetIterResponse, error) {
	// Limit LUNs to those owned by the volumeName; ex, "trident_trident"
	query := &azgo.LunGetIterRequestQuery{}
	lunInfo := azgo.NewLunInfoType().
		SetVolume(volumeName)
	query.SetLunInfo(*lunInfo)

	return c.lunGetAllCommon(query)
}

// LunGetAllForVserver returns all relevant details for all LUNs in the supplied SVM
// equivalent to filer::> lun show -vserver trident_CEwDWXQRPz
func (c Client) LunGetAllForVserver(vserverName string) (*azgo.LunGetIterResponse, error) {
	// Limit LUNs to those owned by the SVM with the supplied vserverName
	query := &azgo.LunGetIterRequestQuery{}
	lunInfo := azgo.NewLunInfoType().
		SetVserver(vserverName)
	query.SetLunInfo(*lunInfo)

	return c.lunGetAllCommon(query)
}

// LunCount returns the number of LUNs that exist in a given volume
func (c Client) LunCount(ctx context.Context, volume string) (int, error) {
	// Limit the LUNs to those in the specified Flexvol
	query := &azgo.LunGetIterRequestQuery{}
	lunInfo := azgo.NewLunInfoType().SetVolume(volume)
	query.SetLunInfo(*lunInfo)

	// Limit the returned data to only the Flexvol and LUN names
	desiredAttributes := &azgo.LunGetIterRequestDesiredAttributes{}
	desiredInfo := azgo.NewLunInfoType().SetPath("").SetVolume("")
	desiredAttributes.SetLunInfo(*desiredInfo)

	response, err := azgo.NewLunGetIterRequest().
		SetMaxRecords(DefaultZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(c.zr)
	if err = azgo.GetError(ctx, response, err); err != nil {
		return 0, err
	}

	return response.Result.NumRecords(), nil
}

// LunRename changes the name of a LUN
func (c Client) LunRename(path, newPath string) (*azgo.LunMoveResponse, error) {
	response, err := azgo.NewLunMoveRequest().
		SetPath(path).
		SetNewPath(newPath).
		ExecuteUsing(c.zr)
	return response, err
}

// LunUnmap deletes the lun mapping for the given LUN path and igroup
// equivalent to filer::> lun mapping delete -vserver iscsi_vs -path /vol/v/lun0 -igroup group
func (c Client) LunUnmap(initiatorGroupName, lunPath string) (*azgo.LunUnmapResponse, error) {
	response, err := azgo.NewLunUnmapRequest().
		SetInitiatorGroup(initiatorGroupName).
		SetPath(lunPath).
		ExecuteUsing(c.zr)
	return response, err
}

// LunSize retrieves the size of the specified volume, does not work with economy driver
func (c Client) LunSize(lunPath string) (int, error) {
	lunAttrs, err := c.LunGet(lunPath)
	if err != nil {
		return 0, err
	}

	return lunAttrs.Size(), nil
}

// LUN operations END
// ///////////////////////////////////////////////////////////////////////////

// ///////////////////////////////////////////////////////////////////////////
// FlexGroup operations BEGIN

// FlexGroupCreate creates a FlexGroup with the specified options
// equivalent to filer::> volume create -vserver svm_name -volume fg_vol_name –auto-provision-as flexgroup -size fg_size  -state online -type RW -policy default -unix-permissions ---rwxr-xr-x -space-guarantee none -snapshot-policy none -security-style unix -encrypt false
func (c Client) FlexGroupCreate(
	ctx context.Context, name string, size int, aggrs []azgo.AggrNameType, spaceReserve, snapshotPolicy,
	unixPermissions, exportPolicy, securityStyle, tieringPolicy, comment string, qosPolicyGroup QosPolicyGroup,
	encrypt *bool, snapshotReserve int,
) (*azgo.VolumeCreateAsyncResponse, error) {
	junctionPath := fmt.Sprintf("/%s", name)

	aggrList := azgo.VolumeCreateAsyncRequestAggrList{}
	aggrList.SetAggrName(aggrs)

	request := azgo.NewVolumeCreateAsyncRequest().
		SetVolumeName(name).
		SetSize(size).
		SetSnapshotPolicy(snapshotPolicy).
		SetSpaceReserve(spaceReserve).
		SetExportPolicy(exportPolicy).
		SetVolumeSecurityStyle(securityStyle).
		SetAggrList(aggrList).
		SetJunctionPath(junctionPath).
		SetVolumeComment(comment)

	// Set Unix permission for NFS volume only.
	if unixPermissions != "" {
		request.SetUnixPermissions(unixPermissions)
	}
	// For encrypt == nil - we don't explicitely set the encrypt argument.
	// If destination aggregate is NAE enabled, new volume will be aggregate encrypted
	// else it will be volume encrypted as per Ontap's default behaviour.
	if encrypt != nil {
		request.SetEncrypt(*encrypt)
	}

	switch qosPolicyGroup.Kind {
	case QosPolicyGroupKind:
		request.SetQosPolicyGroupName(qosPolicyGroup.Name)
	case QosAdaptivePolicyGroupKind:
		request.SetQosAdaptivePolicyGroupName(qosPolicyGroup.Name)
	}

	if snapshotReserve != NumericalValueNotSet {
		request.SetPercentageSnapshotReserve(snapshotReserve)
	}

	// Allowed ONTAP tiering Policy values
	//
	// =================================================================================
	// SVM-DR - Value applicable to source SVM (and destination cluster during failover)
	// =================================================================================
	// ONTAP DRIVER	            ONTAP 9.3                   ONTAP 9.4                   ONTAP 9.5
	// ONTAP-FlexGroups         NA                          NA                          NA
	//
	//
	// ==========
	// Non-SVM-DR
	// ==========
	// ONTAP DRIVER             ONTAP 9.3                   ONTAP 9.4                   ONTAP 9.5
	// ONTAP-FlexGroups         all-values/pass             all-values/pass             other-values(backup)/pass(fail)
	//

	if c.SupportsFeature(ctx, NetAppFabricPoolFlexGroup) {
		request.SetTieringPolicy(tieringPolicy)
	}

	response, err := request.ExecuteUsing(c.zr)
	if zerr := azgo.GetError(ctx, *response, err); zerr != nil {
		apiError, message, code := ExtractError(zerr)
		if apiError == "failed" && code == azgo.EINVALIDINPUTERROR {
			return response, errors.InvalidInputError(message)
		}

		return response, zerr
	}

	err = c.WaitForAsyncResponse(ctx, *response, maxFlexGroupWait)
	if err != nil {
		return response, fmt.Errorf("error waiting for response: %v", err)
	}

	return response, err
}

// FlexGroupDestroy destroys a FlexGroup
func (c Client) FlexGroupDestroy(
	ctx context.Context, name string, force bool,
) (*azgo.VolumeDestroyAsyncResponse, error) {
	response, err := azgo.NewVolumeDestroyAsyncRequest().
		SetVolumeName(name).
		ExecuteUsing(c.zr)

	if zerr := azgo.NewZapiError(*response); !zerr.IsPassed() {
		// It's not an error if the volume no longer exists
		if zerr.Code() == azgo.EVOLUMEDOESNOTEXIST {
			Logc(ctx).WithField("volume", name).Warn("FlexGroup already deleted.")
			return response, nil
		}
	}

	if gerr := azgo.GetError(ctx, response, err); gerr != nil {
		return response, gerr
	}

	err = c.WaitForAsyncResponse(ctx, *response, maxFlexGroupWait)
	if err != nil {
		return response, fmt.Errorf("error waiting for response: %v", err)
	}

	return response, err
}

// FlexGroupExists tests for the existence of a FlexGroup
func (c Client) FlexGroupExists(ctx context.Context, name string) (bool, error) {
	response, err := azgo.NewVolumeSizeAsyncRequest().
		SetVolumeName(name).
		ExecuteUsing(c.zr)

	if zerr := azgo.NewZapiError(response); !zerr.IsPassed() {
		switch zerr.Code() {
		case azgo.EOBJECTNOTFOUND, azgo.EVOLUMEDOESNOTEXIST:
			return false, nil
		default:
			return false, zerr
		}
	}

	if gerr := azgo.GetError(ctx, response, err); gerr != nil {
		return false, gerr
	}

	// Wait for Async Job to complete
	err = c.WaitForAsyncResponse(ctx, response, maxFlexGroupWait)
	if err != nil {
		return false, fmt.Errorf("error waiting for response: %v", err)
	}

	return true, nil
}

// FlexGroupUsedSize retrieves the used space of the specified volume
func (c Client) FlexGroupUsedSize(name string) (int, error) {
	volAttrs, err := c.FlexGroupGet(name)
	if err != nil {
		return 0, err
	}
	if volAttrs == nil {
		return 0, fmt.Errorf("error getting used space for FlexGroup: %v", name)
	}

	volSpaceAttrs := volAttrs.VolumeSpaceAttributes()
	return volSpaceAttrs.SizeUsed(), nil
}

// FlexGroupSize retrieves the size of the specified volume
func (c Client) FlexGroupSize(name string) (int, error) {
	volAttrs, err := c.FlexGroupGet(name)
	if err != nil {
		return 0, err
	}
	if volAttrs == nil {
		return 0, fmt.Errorf("error getting size for FlexGroup: %v", name)
	}

	volSpaceAttrs := volAttrs.VolumeSpaceAttributes()
	return volSpaceAttrs.Size(), nil
}

// FlexGroupSetSize sets the size of the specified FlexGroup
func (c Client) FlexGroupSetSize(ctx context.Context, name, newSize string) (*azgo.VolumeSizeAsyncResponse, error) {
	response, err := azgo.NewVolumeSizeAsyncRequest().
		SetVolumeName(name).
		SetNewSize(newSize).
		ExecuteUsing(c.zr)

	if zerr := azgo.GetError(ctx, *response, err); zerr != nil {
		return response, zerr
	}

	err = c.WaitForAsyncResponse(ctx, *response, maxFlexGroupWait)
	if err != nil {
		return response, fmt.Errorf("error waiting for response: %v", err)
	}

	return response, err
}

// FlexGroupVolumeModifySnapshotDirectoryAccess modifies access to the ".snapshot" directory
func (c Client) FlexGroupVolumeModifySnapshotDirectoryAccess(
	ctx context.Context, name string, enable bool,
) (*azgo.VolumeModifyIterAsyncResponse, error) {
	volattr := &azgo.VolumeModifyIterAsyncRequestAttributes{}
	ssattr := azgo.NewVolumeSnapshotAttributesType().SetSnapdirAccessEnabled(enable)
	volSnapshotAttrs := azgo.NewVolumeAttributesType().SetVolumeSnapshotAttributes(*ssattr)
	volattr.SetVolumeAttributes(*volSnapshotAttrs)

	queryattr := &azgo.VolumeModifyIterAsyncRequestQuery{}
	volidattr := azgo.NewVolumeIdAttributesType().SetName(name)
	volIdAttrs := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*volidattr)
	queryattr.SetVolumeAttributes(*volIdAttrs)

	response, err := azgo.NewVolumeModifyIterAsyncRequest().
		SetQuery(*queryattr).
		SetAttributes(*volattr).
		ExecuteUsing(c.zr)

	if zerr := azgo.GetError(ctx, response, err); zerr != nil {
		return response, zerr
	}

	err = c.WaitForAsyncResponse(ctx, *response, maxFlexGroupWait)
	if err != nil {
		return response, fmt.Errorf("error waiting for response: %v", err)
	}

	return response, err
}

func (c Client) FlexGroupModifyUnixPermissions(
	ctx context.Context, volumeName, unixPermissions string,
) (*azgo.VolumeModifyIterAsyncResponse, error) {
	volAttr := &azgo.VolumeModifyIterAsyncRequestAttributes{}
	volSecurityUnixAttrs := azgo.NewVolumeSecurityUnixAttributesType()
	// Set Unix permission for NFS volume only.
	if unixPermissions != "" {
		volSecurityUnixAttrs.SetPermissions(unixPermissions)
	}
	volSecurityAttrs := azgo.NewVolumeSecurityAttributesType().SetVolumeSecurityUnixAttributes(*volSecurityUnixAttrs)
	securityAttributes := azgo.NewVolumeAttributesType().SetVolumeSecurityAttributes(*volSecurityAttrs)
	volAttr.SetVolumeAttributes(*securityAttributes)

	queryAttr := &azgo.VolumeModifyIterAsyncRequestQuery{}
	volIDAttr := azgo.NewVolumeIdAttributesType().SetName(volumeName)
	volIDAttrs := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*volIDAttr)
	queryAttr.SetVolumeAttributes(*volIDAttrs)

	response, err := azgo.NewVolumeModifyIterAsyncRequest().
		SetQuery(*queryAttr).
		SetAttributes(*volAttr).
		ExecuteUsing(c.zr)

	if zerr := azgo.GetError(ctx, response, err); zerr != nil {
		return response, zerr
	}

	err = c.WaitForAsyncResponse(ctx, *response, maxFlexGroupWait)
	if err != nil {
		return response, fmt.Errorf("error waiting for response: %v", err)
	}

	return response, err
}

// FlexGroupSetComment sets a flexgroup's comment to the supplied value
func (c Client) FlexGroupSetComment(
	ctx context.Context, volumeName, newVolumeComment string,
) (*azgo.VolumeModifyIterAsyncResponse, error) {
	volattr := &azgo.VolumeModifyIterAsyncRequestAttributes{}
	idattr := azgo.NewVolumeIdAttributesType().SetComment(newVolumeComment)
	volidattr := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*idattr)
	volattr.SetVolumeAttributes(*volidattr)

	queryAttr := &azgo.VolumeModifyIterAsyncRequestQuery{}
	volIDAttr := azgo.NewVolumeIdAttributesType().SetName(volumeName)
	volIDAttrs := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*volIDAttr)
	queryAttr.SetVolumeAttributes(*volIDAttrs)

	response, err := azgo.NewVolumeModifyIterAsyncRequest().
		SetQuery(*queryAttr).
		SetAttributes(*volattr).
		ExecuteUsing(c.zr)

	if zerr := azgo.GetError(ctx, response, err); zerr != nil {
		return response, zerr
	}

	err = c.WaitForAsyncResponse(ctx, *response, maxFlexGroupWait)
	if err != nil {
		return response, fmt.Errorf("error waiting for response: %v", err)
	}

	return response, err
}

// FlexGroupGet returns all relevant details for a single FlexGroup
func (c Client) FlexGroupGet(name string) (*azgo.VolumeAttributesType, error) {
	// Limit the FlexGroups to the one matching the name
	queryVolIDAttrs := azgo.NewVolumeIdAttributesType().SetName(name)
	queryVolIDAttrs.SetStyleExtended("flexgroup")
	return c.volumeGetIterCommon(name, queryVolIDAttrs)
}

// FlexGroupGetAll returns all relevant details for all FlexGroups whose names match the supplied prefix
func (c Client) FlexGroupGetAll(prefix string) (*azgo.VolumeGetIterResponse, error) {
	// Limit the FlexGroups to those matching the name prefix
	queryVolIDAttrs := azgo.NewVolumeIdAttributesType().SetName(prefix + "*")
	queryVolStateAttrs := azgo.NewVolumeStateAttributesType().SetState("online")
	queryVolIDAttrs.SetStyleExtended("flexgroup")
	return c.volumeGetIterAll(prefix, queryVolIDAttrs, queryVolStateAttrs)
}

// WaitForAsyncResponse handles waiting for an AsyncResponse to return successfully or return an error.
func (c Client) WaitForAsyncResponse(ctx context.Context, zapiResult interface{}, maxWaitTime time.Duration) error {
	asyncResult, err := azgo.NewZapiAsyncResult(ctx, zapiResult)
	if err != nil {
		return err
	}

	// Possible values: "succeeded", "in_progress", "failed". Returns nil if succeeded
	if asyncResult.Status() == "in_progress" {
		// handle zapi response
		jobId := int(asyncResult.JobId())
		if asyncResponseError := c.checkForJobCompletion(ctx, jobId, maxWaitTime); asyncResponseError != nil {
			return asyncResponseError
		}
	} else if asyncResult.Status() == "failed" {
		return fmt.Errorf("result status is failed with errorCode %d", asyncResult.ErrorCode())
	}

	return nil
}

// checkForJobCompletion polls for the ONTAP job status success with backoff retry logic
func (c *Client) checkForJobCompletion(ctx context.Context, jobId int, maxWaitTime time.Duration) error {
	checkJobFinished := func() error {
		jobResponse, err := c.JobGetIterStatus(jobId)
		if err != nil {
			return fmt.Errorf("error occurred getting job status for job ID %d: %v", jobId, jobResponse.Result)
		}
		if jobResponse.Result.ResultStatusAttr != "passed" {
			return fmt.Errorf("failed to get job status for job ID %d: %v ", jobId, jobResponse.Result)
		}

		if jobResponse.Result.AttributesListPtr == nil {
			return fmt.Errorf("failed to get job status for job ID %d: %v ", jobId, jobResponse.Result)
		}

		jobState := jobResponse.Result.AttributesListPtr.JobInfoPtr[0].JobState()
		Logc(ctx).WithFields(LogFields{
			"jobId":    jobId,
			"jobState": jobState,
		}).Debug("Job status for job ID")
		// Check for an error with the job. If found return Permanent error to halt backoff.
		if jobState == "failure" || jobState == "error" || jobState == "quit" || jobState == "dead" {
			err = fmt.Errorf("job %d failed to complete. job state: %v", jobId, jobState)
			return backoff.Permanent(err)
		}
		if jobState != "success" {
			return fmt.Errorf("job %d is not yet completed. job state: %v", jobId, jobState)
		}
		return nil
	}

	jobCompletedNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("duration", duration).
			Debug("Job not yet completed, waiting.")
	}

	inProgressBackoff := asyncResponseBackoff(maxWaitTime)

	// Run the job completion check using an exponential backoff
	if err := backoff.RetryNotify(checkJobFinished, inProgressBackoff, jobCompletedNotify); err != nil {
		Logc(ctx).Warnf("Job not completed after %v seconds.", inProgressBackoff.MaxElapsedTime.Seconds())
		return fmt.Errorf("job Id %d failed to complete successfully", jobId)
	} else {
		Logc(ctx).WithField("jobId", jobId).Debug("Job completed successfully.")
		return nil
	}
}

func asyncResponseBackoff(maxWaitTime time.Duration) *backoff.ExponentialBackOff {
	inProgressBackoff := backoff.NewExponentialBackOff()
	inProgressBackoff.InitialInterval = 1 * time.Second
	inProgressBackoff.Multiplier = 2
	inProgressBackoff.RandomizationFactor = 0.1

	inProgressBackoff.MaxElapsedTime = maxWaitTime
	return inProgressBackoff
}

// JobGetIterStatus returns the current job status for Async requests.
func (c Client) JobGetIterStatus(jobId int) (*azgo.JobGetIterResponse, error) {
	jobInfo := azgo.NewJobInfoType().SetJobId(jobId)
	queryAttr := &azgo.JobGetIterRequestQuery{}
	queryAttr.SetJobInfo(*jobInfo)

	response, err := azgo.NewJobGetIterRequest().
		SetQuery(*queryAttr).
		ExecuteUsing(c.GetNontunneledZapiRunner())
	return response, err
}

// FlexGroup operations END
// ///////////////////////////////////////////////////////////////////////////

// ///////////////////////////////////////////////////////////////////////////
// VOLUME operations BEGIN

// VolumeCreate creates a volume with the specified options
// equivalent to filer::> volume create -vserver iscsi_vs -volume v -aggregate aggr1 -size 1g -state online -type RW -policy default -unix-permissions ---rwxr-xr-x -space-guarantee none -snapshot-policy none -security-style unix -encrypt false
func (c Client) VolumeCreate(
	ctx context.Context, name, aggregateName, size, spaceReserve, snapshotPolicy, unixPermissions,
	exportPolicy, securityStyle, tieringPolicy, comment string, qosPolicyGroup QosPolicyGroup, encrypt *bool,
	snapshotReserve int, dpVolume bool,
) (*azgo.VolumeCreateResponse, error) {
	request := azgo.NewVolumeCreateRequest().
		SetVolume(name).
		SetContainingAggrName(aggregateName).
		SetSize(size).
		SetSpaceReserve(spaceReserve).
		SetSnapshotPolicy(snapshotPolicy).
		SetExportPolicy(exportPolicy).
		SetVolumeSecurityStyle(securityStyle).
		SetVolumeComment(comment)

	// For encrypt == nil - we don't explicitely set the encrypt argument.
	// If destination aggregate is NAE enabled, new volume will be aggregate encrypted
	// else it will be volume encrypted as per Ontap's default behaviour.
	if encrypt != nil {
		request.SetEncrypt(*encrypt)
	}

	if snapshotReserve != NumericalValueNotSet {
		request.SetPercentageSnapshotReserve(snapshotReserve)
	}

	if dpVolume {
		request.SetVolumeType("DP")
	} else if unixPermissions != "" {
		request.SetUnixPermissions(unixPermissions)
	}

	// Allowed ONTAP tiering Policy values
	//
	// =================================================================================
	// SVM-DR - Value applicable to source SVM (and destination cluster during failover)
	// =================================================================================
	// ONTAP DRIVER	            ONTAP 9.3                           ONTAP 9.4                           ONTAP 9.5
	// ONTAP-NAS                snapshot-only/pass                  snapshot-only/pass                  none/pass
	// ONTAP-NAS-ECO            snapshot-only/pass                  snapshot-only/pass                  none/pass
	//
	//
	// ==========
	// Non-SVM-DR
	// ==========
	// ONTAP DRIVER             ONTAP 9.3                           ONTAP 9.4                           ONTAP 9.5
	// ONTAP-NAS                other-values(backup)/pass(fail)     other-values(backup)/pass(fail)     other-values(
	// backup)/pass(fail)
	// ONTAP-NAS-ECO            other-values(backup)/pass(fail)     other-values(backup)/pass(fail)     other-values(
	// backup)/pass(fail)
	//
	// PLEASE NOTE:
	// 1. 'backup' tiering policy is for dp-volumes only.
	//

	if c.SupportsFeature(ctx, NetAppFabricPoolFlexVol) {
		request.SetTieringPolicy(tieringPolicy)
	}

	switch qosPolicyGroup.Kind {
	case QosPolicyGroupKind:
		request.SetQosPolicyGroupName(qosPolicyGroup.Name)
	case QosAdaptivePolicyGroupKind:
		request.SetQosAdaptivePolicyGroupName(qosPolicyGroup.Name)
	}

	response, err := request.ExecuteUsing(c.zr)
	return response, err
}

func (c Client) VolumeModifyExportPolicy(volumeName, exportPolicyName string) (*azgo.VolumeModifyIterResponse, error) {
	volAttr := &azgo.VolumeModifyIterRequestAttributes{}
	exportAttributes := azgo.NewVolumeExportAttributesType().SetPolicy(exportPolicyName)
	volExportAttrs := azgo.NewVolumeAttributesType().SetVolumeExportAttributes(*exportAttributes)
	volAttr.SetVolumeAttributes(*volExportAttrs)

	queryAttr := &azgo.VolumeModifyIterRequestQuery{}
	volIDAttr := azgo.NewVolumeIdAttributesType().SetName(volumeName)
	volIDAttrs := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*volIDAttr)
	queryAttr.SetVolumeAttributes(*volIDAttrs)

	response, err := azgo.NewVolumeModifyIterRequest().
		SetQuery(*queryAttr).
		SetAttributes(*volAttr).
		ExecuteUsing(c.zr)
	return response, err
}

func (c Client) VolumeModifyUnixPermissions(
	volumeName, unixPermissions string,
) (*azgo.VolumeModifyIterResponse, error) {
	volAttr := &azgo.VolumeModifyIterRequestAttributes{}
	volSecurityUnixAttrs := azgo.NewVolumeSecurityUnixAttributesType()
	// Set Unix permission for NFS volume only.
	if unixPermissions != "" {
		volSecurityUnixAttrs.SetPermissions(unixPermissions)
	}

	volSecurityAttrs := azgo.NewVolumeSecurityAttributesType().SetVolumeSecurityUnixAttributes(*volSecurityUnixAttrs)
	securityAttributes := azgo.NewVolumeAttributesType().SetVolumeSecurityAttributes(*volSecurityAttrs)
	volAttr.SetVolumeAttributes(*securityAttributes)

	queryAttr := &azgo.VolumeModifyIterRequestQuery{}
	volIDAttr := azgo.NewVolumeIdAttributesType().SetName(azgo.VolumeNameType(volumeName))
	volIDAttrs := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*volIDAttr)
	queryAttr.SetVolumeAttributes(*volIDAttrs)

	response, err := azgo.NewVolumeModifyIterRequest().
		SetQuery(*queryAttr).
		SetAttributes(*volAttr).
		ExecuteUsing(c.zr)
	return response, err
}

// VolumeCloneCreate clones a volume from a snapshot
func (c Client) VolumeCloneCreate(name, source, snapshot string) (*azgo.VolumeCloneCreateResponse, error) {
	response, err := azgo.NewVolumeCloneCreateRequest().
		SetVolume(name).
		SetParentVolume(source).
		SetParentSnapshot(snapshot).
		ExecuteUsing(c.zr)
	return response, err
}

// VolumeCloneCreateAsync clones a volume from a snapshot
func (c Client) VolumeCloneCreateAsync(name, source, snapshot string) (*azgo.VolumeCloneCreateAsyncResponse, error) {
	response, err := azgo.NewVolumeCloneCreateAsyncRequest().
		SetVolume(name).
		SetParentVolume(source).
		SetParentSnapshot(snapshot).
		ExecuteUsing(c.zr)
	return response, err
}

// VolumeCloneSplitStart splits a cloned volume from its parent
func (c Client) VolumeCloneSplitStart(name string) (*azgo.VolumeCloneSplitStartResponse, error) {
	response, err := azgo.NewVolumeCloneSplitStartRequest().
		SetVolume(name).
		ExecuteUsing(c.zr)
	return response, err
}

// VolumeModifySnapshotDirectoryAccess modifies access to the ".snapshot" directory
func (c Client) VolumeModifySnapshotDirectoryAccess(name string, enable bool) (*azgo.VolumeModifyIterResponse, error) {
	volattr := &azgo.VolumeModifyIterRequestAttributes{}
	ssattr := azgo.NewVolumeSnapshotAttributesType().SetSnapdirAccessEnabled(enable)
	volSnapshotAttrs := azgo.NewVolumeAttributesType().SetVolumeSnapshotAttributes(*ssattr)
	volattr.SetVolumeAttributes(*volSnapshotAttrs)

	queryattr := &azgo.VolumeModifyIterRequestQuery{}
	volidattr := azgo.NewVolumeIdAttributesType().SetName(azgo.VolumeNameType(name))
	volIdAttrs := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*volidattr)
	queryattr.SetVolumeAttributes(*volIdAttrs)

	response, err := azgo.NewVolumeModifyIterRequest().
		SetQuery(*queryattr).
		SetAttributes(*volattr).
		ExecuteUsing(c.zr)
	return response, err
}

// Use this to set the QoS Policy Group for volume clones since
// we can't set adaptive policy groups directly during volume clone creation.
func (c Client) VolumeSetQosPolicyGroupName(
	name string, qosPolicyGroup QosPolicyGroup,
) (*azgo.VolumeModifyIterResponse, error) {
	volModAttr := &azgo.VolumeModifyIterRequestAttributes{}
	volQosAttr := azgo.NewVolumeQosAttributesType()

	switch qosPolicyGroup.Kind {
	case QosPolicyGroupKind:
		volQosAttr.SetPolicyGroupName(qosPolicyGroup.Name)
	case QosAdaptivePolicyGroupKind:
		volQosAttr.SetAdaptivePolicyGroupName(qosPolicyGroup.Name)
	}

	volAttrs := azgo.NewVolumeAttributesType().SetVolumeQosAttributes(*volQosAttr)
	volModAttr.SetVolumeAttributes(*volAttrs)

	queryattr := &azgo.VolumeModifyIterRequestQuery{}
	volidattr := azgo.NewVolumeIdAttributesType().SetName(name)
	volIdAttrs := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*volidattr)
	queryattr.SetVolumeAttributes(*volIdAttrs)

	response, err := azgo.NewVolumeModifyIterRequest().
		SetQuery(*queryattr).
		SetAttributes(*volModAttr).
		ExecuteUsing(c.zr)
	return response, err
}

// VolumeExists tests for the existence of a Flexvol
func (c Client) VolumeExists(ctx context.Context, name string) (bool, error) {
	if name == "" {
		return false, nil
	}

	response, err := azgo.NewVolumeSizeRequest().
		SetVolume(name).
		ExecuteUsing(c.zr)
	if err != nil {
		return false, err
	}

	if zerr := azgo.NewZapiError(response); !zerr.IsPassed() {
		switch zerr.Code() {
		case azgo.EOBJECTNOTFOUND, azgo.EVOLUMEDOESNOTEXIST:
			return false, nil
		default:
			return false, zerr
		}
	}

	return true, nil
}

// VolumeUsedSize retrieves the used bytes of the specified volume
func (c Client) VolumeUsedSize(name string) (int, error) {
	volAttrs, err := c.VolumeGet(name)
	if err != nil {
		return 0, err
	}
	volSpaceAttrs := volAttrs.VolumeSpaceAttributes()

	return volSpaceAttrs.SizeUsed(), nil
}

// VolumeSize retrieves the size of the specified volume
func (c Client) VolumeSize(name string) (int, error) {
	volAttrs, err := c.VolumeGet(name)
	if err != nil {
		return 0, err
	}
	volSpaceAttrs := volAttrs.VolumeSpaceAttributes()

	return volSpaceAttrs.Size(), nil
}

// VolumeSetSize sets the size of the specified volume
func (c Client) VolumeSetSize(name, newSize string) (*azgo.VolumeSizeResponse, error) {
	response, err := azgo.NewVolumeSizeRequest().
		SetVolume(name).
		SetNewSize(newSize).
		ExecuteUsing(c.zr)
	return response, err
}

// VolumeMount mounts a volume at the specified junction
func (c Client) VolumeMount(name, junctionPath string) (*azgo.VolumeMountResponse, error) {
	response, err := azgo.NewVolumeMountRequest().
		SetVolumeName(name).
		SetJunctionPath(junctionPath).
		ExecuteUsing(c.zr)
	return response, err
}

// VolumeUnmount unmounts a volume from the specified junction
func (c Client) VolumeUnmount(name string, force bool) (*azgo.VolumeUnmountResponse, error) {
	response, err := azgo.NewVolumeUnmountRequest().
		SetVolumeName(name).
		SetForce(force).
		ExecuteUsing(c.zr)
	return response, err
}

// VolumeOffline offlines a volume
func (c Client) VolumeOffline(name string) (*azgo.VolumeOfflineResponse, error) {
	response, err := azgo.NewVolumeOfflineRequest().
		SetName(name).
		ExecuteUsing(c.zr)
	return response, err
}

// VolumeDestroy destroys a volume
func (c Client) VolumeDestroy(name string, force bool) (*azgo.VolumeDestroyResponse, error) {
	response, err := azgo.NewVolumeDestroyRequest().
		SetName(name).
		SetUnmountAndOffline(force).
		ExecuteUsing(c.zr)
	return response, err
}

// VolumeGet returns all relevant details for a single Flexvol
// equivalent to filer::> volume show
func (c Client) VolumeGet(name string) (*azgo.VolumeAttributesType, error) {
	// Limit the Flexvols to the one matching the name
	queryVolIDAttrs := azgo.NewVolumeIdAttributesType().
		SetName(azgo.VolumeNameType(name)).
		SetStyleExtended("flexvol")
	return c.volumeGetIterCommon(name, queryVolIDAttrs)
}

// VolumeGetType returns the volume type such as RW or DP
func (c Client) VolumeGetType(name string) (string, error) {
	// Limit the Flexvols to the one matching the name
	queryVolIDAttrs := azgo.NewVolumeIdAttributesType().
		SetName(azgo.VolumeNameType(name)).
		SetStyleExtended("flexvol")
	volGet, err := c.volumeGetIterCommon(name, queryVolIDAttrs)
	if err != nil {
		return "", err
	}

	attributes := volGet.VolumeIdAttributes()
	return attributes.Type(), nil
}

func (c Client) volumeGetIterCommon(
	name string, queryVolIDAttrs *azgo.VolumeIdAttributesType,
) (*azgo.VolumeAttributesType, error) {
	queryVolStateAttrs := azgo.NewVolumeStateAttributesType().SetState("online")

	query := &azgo.VolumeGetIterRequestQuery{}
	volAttrs := azgo.NewVolumeAttributesType().
		SetVolumeIdAttributes(*queryVolIDAttrs).
		SetVolumeStateAttributes(*queryVolStateAttrs)
	query.SetVolumeAttributes(*volAttrs)

	response, err := azgo.NewVolumeGetIterRequest().
		SetMaxRecords(c.config.ContextBasedZapiRecords).
		SetQuery(*query).
		ExecuteUsing(c.zr)

	if err != nil {
		return &azgo.VolumeAttributesType{}, err
	} else if response.Result.NumRecords() == 0 {
		return &azgo.VolumeAttributesType{}, errors.NotFoundError(fmt.Sprintf("flexvol %s not found", name))
	} else if response.Result.NumRecords() > 1 {
		return &azgo.VolumeAttributesType{}, fmt.Errorf("more than one Flexvol %s found", name)
	} else if response.Result.AttributesListPtr == nil {
		return &azgo.VolumeAttributesType{}, errors.NotFoundError(fmt.Sprintf("flexvol %s not found", name))
	} else if response.Result.AttributesListPtr.VolumeAttributesPtr != nil {
		return &response.Result.AttributesListPtr.VolumeAttributesPtr[0], nil
	}
	return &azgo.VolumeAttributesType{}, errors.NotFoundError(fmt.Sprintf("flexvol %s not found", name))
}

// VolumeGetAll returns all relevant details for all FlexVols whose names match the supplied prefix
// equivalent to filer::> volume show
func (c Client) VolumeGetAll(prefix string) (response *azgo.VolumeGetIterResponse, err error) {
	// Limit the Flexvols to those matching the name prefix
	queryVolIDAttrs := azgo.NewVolumeIdAttributesType().
		SetName(azgo.VolumeNameType(prefix + "*")).
		SetStyleExtended("flexvol")
	queryVolStateAttrs := azgo.NewVolumeStateAttributesType().SetState("online")

	return c.volumeGetIterAll(prefix, queryVolIDAttrs, queryVolStateAttrs)
}

func (c Client) volumeGetIterAll(
	prefix string, queryVolIDAttrs *azgo.VolumeIdAttributesType, queryVolStateAttrs *azgo.VolumeStateAttributesType,
) (*azgo.VolumeGetIterResponse, error) {
	query := &azgo.VolumeGetIterRequestQuery{}
	volumeAttributes := azgo.NewVolumeAttributesType().
		SetVolumeIdAttributes(*queryVolIDAttrs).
		SetVolumeStateAttributes(*queryVolStateAttrs)
	query.SetVolumeAttributes(*volumeAttributes)

	// Limit the returned data to only the data relevant to containers
	desiredVolExportAttrs := azgo.NewVolumeExportAttributesType().
		SetPolicy("")
	desiredVolIDAttrs := azgo.NewVolumeIdAttributesType().
		SetName("").
		SetComment("").
		SetContainingAggregateName("")
	desiredVolSecurityUnixAttrs := azgo.NewVolumeSecurityUnixAttributesType().
		SetPermissions("")
	desiredVolSecurityAttrs := azgo.NewVolumeSecurityAttributesType().
		SetVolumeSecurityUnixAttributes(*desiredVolSecurityUnixAttrs)
	desiredVolSpaceAttrs := azgo.NewVolumeSpaceAttributesType().
		SetSize(0).
		SetSpaceGuarantee("")
	desiredVolSnapshotAttrs := azgo.NewVolumeSnapshotAttributesType().
		SetSnapdirAccessEnabled(true).
		SetSnapshotPolicy("")

	desiredAttributes := &azgo.VolumeGetIterRequestDesiredAttributes{}
	desiredVolumeAttributes := azgo.NewVolumeAttributesType().
		SetVolumeExportAttributes(*desiredVolExportAttrs).
		SetVolumeIdAttributes(*desiredVolIDAttrs).
		SetVolumeSecurityAttributes(*desiredVolSecurityAttrs).
		SetVolumeSpaceAttributes(*desiredVolSpaceAttrs).
		SetVolumeSnapshotAttributes(*desiredVolSnapshotAttrs)
	desiredAttributes.SetVolumeAttributes(*desiredVolumeAttributes)

	response, err := azgo.NewVolumeGetIterRequest().
		SetMaxRecords(c.config.ContextBasedZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(c.zr)
	return response, err
}

// VolumeList returns the names of all Flexvols whose names match the supplied prefix
func (c Client) VolumeList(prefix string) (*azgo.VolumeGetIterResponse, error) {
	// Limit the Flexvols to those matching the name prefix
	query := &azgo.VolumeGetIterRequestQuery{}
	queryVolIDAttrs := azgo.NewVolumeIdAttributesType().
		SetName(azgo.VolumeNameType(prefix + "*")).
		SetStyleExtended("flexvol")
	queryVolStateAttrs := azgo.NewVolumeStateAttributesType().SetState("online")
	volumeAttributes := azgo.NewVolumeAttributesType().
		SetVolumeIdAttributes(*queryVolIDAttrs).
		SetVolumeStateAttributes(*queryVolStateAttrs)
	query.SetVolumeAttributes(*volumeAttributes)

	// Limit the returned Flexvol data to names
	desiredAttributes := &azgo.VolumeGetIterRequestDesiredAttributes{}
	desiredVolIDAttrs := azgo.NewVolumeIdAttributesType().SetName("")
	desiredVolumeAttributes := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*desiredVolIDAttrs)
	desiredAttributes.SetVolumeAttributes(*desiredVolumeAttributes)

	response, err := azgo.NewVolumeGetIterRequest().
		SetMaxRecords(c.config.ContextBasedZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(c.zr)
	return response, err
}

// VolumeListByAttrs returns the names of all Flexvols matching the specified attributes
func (c Client) VolumeListByAttrs(
	prefix, aggregate, spaceReserve, snapshotPolicy, tieringPolicy string, snapshotDir, encrypt *bool,
	snapReserve int,
) (*azgo.VolumeGetIterResponse, error) {
	// Limit the Flexvols to those matching the specified attributes
	query := &azgo.VolumeGetIterRequestQuery{}
	queryVolIDAttrs := azgo.NewVolumeIdAttributesType().
		SetName(prefix).
		SetContainingAggregateName(aggregate).
		SetStyleExtended("flexvol")
	queryVolSpaceAttrs := azgo.NewVolumeSpaceAttributesType().
		SetSpaceGuarantee(spaceReserve)
	if snapReserve >= 0 {
		queryVolSpaceAttrs.SetPercentageSnapshotReserve(snapReserve)
	}
	queryVolSnapshotAttrs := azgo.NewVolumeSnapshotAttributesType().SetSnapshotPolicy(snapshotPolicy)
	if snapshotDir != nil {
		queryVolSnapshotAttrs.SetSnapdirAccessEnabled(*snapshotDir)
	}

	queryVolStateAttrs := azgo.NewVolumeStateAttributesType().
		SetState("online")
	queryVolCompAggrAttrs := azgo.NewVolumeCompAggrAttributesType().
		SetTieringPolicy(tieringPolicy)
	volumeAttributes := azgo.NewVolumeAttributesType().
		SetVolumeCompAggrAttributes(*queryVolCompAggrAttrs).
		SetVolumeIdAttributes(*queryVolIDAttrs).
		SetVolumeSpaceAttributes(*queryVolSpaceAttrs).
		SetVolumeSnapshotAttributes(*queryVolSnapshotAttrs).
		SetVolumeStateAttributes(*queryVolStateAttrs)

	if encrypt != nil {
		volumeAttributes.SetEncrypt(*encrypt)
	}

	query.SetVolumeAttributes(*volumeAttributes)

	// Limit the returned data to only the Flexvol names
	desiredAttributes := &azgo.VolumeGetIterRequestDesiredAttributes{}
	desiredVolIDAttrs := azgo.NewVolumeIdAttributesType().SetName("")
	desiredVolumeAttributes := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*desiredVolIDAttrs)
	desiredAttributes.SetVolumeAttributes(*desiredVolumeAttributes)

	response, err := azgo.NewVolumeGetIterRequest().
		SetMaxRecords(c.config.ContextBasedZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(c.zr)
	return response, err
}

// VolumeListAllBackedBySnapshot returns the names of all FlexVols backed by the specified snapshot
func (c Client) VolumeListAllBackedBySnapshot(ctx context.Context, volumeName, snapshotName string) ([]string, error) {
	// Limit the Flexvols to those matching the specified attributes
	query := &azgo.VolumeGetIterRequestQuery{}
	queryVolCloneParentAttrs := azgo.NewVolumeCloneParentAttributesType().
		SetName(volumeName).
		SetSnapshotName(snapshotName)
	queryVolCloneAttrs := azgo.NewVolumeCloneAttributesType().
		SetVolumeCloneParentAttributes(*queryVolCloneParentAttrs)
	volumeAttributes := azgo.NewVolumeAttributesType().
		SetVolumeCloneAttributes(*queryVolCloneAttrs)
	query.SetVolumeAttributes(*volumeAttributes)

	// Limit the returned data to only the Flexvol names
	desiredAttributes := &azgo.VolumeGetIterRequestDesiredAttributes{}
	desiredVolIDAttrs := azgo.NewVolumeIdAttributesType().SetName("")
	desiredVolumeAttributes := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*desiredVolIDAttrs)
	desiredAttributes.SetVolumeAttributes(*desiredVolumeAttributes)

	response, err := azgo.NewVolumeGetIterRequest().
		SetMaxRecords(DefaultZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(c.zr)

	if err = azgo.GetError(ctx, response, err); err != nil {
		return nil, fmt.Errorf("error enumerating volumes backed by snapshot: %v", err)
	}

	volumeNames := make([]string, 0)

	if response.Result.AttributesListPtr != nil {
		for _, volAttrs := range response.Result.AttributesListPtr.VolumeAttributesPtr {
			volIDAttrs := volAttrs.VolumeIdAttributes()
			volumeNames = append(volumeNames, string(volIDAttrs.Name()))
		}
	}

	return volumeNames, nil
}

// VolumeRename changes the name of a FlexVol (but not a FlexGroup!)
func (c Client) VolumeRename(volumeName, newVolumeName string) (*azgo.VolumeRenameResponse, error) {
	response, err := azgo.NewVolumeRenameRequest().
		SetVolume(volumeName).
		SetNewVolumeName(newVolumeName).
		ExecuteUsing(c.zr)
	return response, err
}

// VolumeSetComment sets a volume's comment to the supplied value
// equivalent to filer::> volume modify -vserver iscsi_vs -volume v -comment newVolumeComment
func (c Client) VolumeSetComment(ctx context.Context, volumeName, newVolumeComment string) (
	*azgo.VolumeModifyIterResponse, error,
) {
	volattr := &azgo.VolumeModifyIterRequestAttributes{}
	idattr := azgo.NewVolumeIdAttributesType().SetComment(newVolumeComment)
	volidattr := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*idattr)
	volattr.SetVolumeAttributes(*volidattr)

	queryAttr := &azgo.VolumeModifyIterRequestQuery{}
	volIDAttr := azgo.NewVolumeIdAttributesType().SetName(volumeName)
	volIDAttrs := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*volIDAttr)
	queryAttr.SetVolumeAttributes(*volIDAttrs)

	response, err := azgo.NewVolumeModifyIterRequest().
		SetQuery(*queryAttr).
		SetAttributes(*volattr).
		ExecuteUsing(c.zr)
	return response, err
}

// VolumeRecoveryQueuePurge purges the volume from the recovery queue
func (c Client) VolumeRecoveryQueuePurge(volumeName string) (*azgo.VolumeRecoveryQueuePurgeResponse, error) {
	return azgo.NewVolumeRecoveryQueuePurgeRequest().
		SetVolumeName(volumeName).
		ExecuteUsing(c.zr)
}

func (c Client) VolumeRecoveryQueueGetIter(volumeName string) (*azgo.VolumeRecoveryQueueGetIterResponse, error) {
	queryAtters := &azgo.VolumeRecoveryQueueGetIterRequestQuery{}
	queryAtters.SetVolumeRecoveryQueueInfo(*azgo.NewVolumeRecoveryQueueInfoType().SetVolumeName(volumeName).
		SetVserverName(c.SVMName()))

	return azgo.NewVolumeRecoveryQueueGetIterRequest().SetQuery(*queryAtters).ExecuteUsing(c.zr)
}

// VOLUME operations END
// ///////////////////////////////////////////////////////////////////////////

// ///////////////////////////////////////////////////////////////////////////
// QTREE operations BEGIN

// QtreeCreate creates a qtree with the specified options
// equivalent to filer::> qtree create -vserver ndvp_vs -volume v -qtree q -export-policy default -unix-permissions ---rwxr-xr-x -security-style unix
func (c Client) QtreeCreate(
	name, volumeName, unixPermissions, exportPolicy, securityStyle, qosPolicy string,
) (*azgo.QtreeCreateResponse, error) {
	request := azgo.NewQtreeCreateRequest().
		SetQtree(name).
		SetVolume(volumeName).
		SetSecurityStyle(securityStyle).
		SetExportPolicy(exportPolicy)

	// Set Unix permission for NFS volume only.
	if unixPermissions != "" {
		request.SetMode(unixPermissions)
	}

	if qosPolicy != "" {
		request.SetQosPolicyGroup(qosPolicy)
	}

	response, err := request.ExecuteUsing(c.zr)
	return response, err
}

// QtreeRename renames a qtree
// equivalent to filer::> volume qtree rename
func (c Client) QtreeRename(path, newPath string) (*azgo.QtreeRenameResponse, error) {
	response, err := azgo.NewQtreeRenameRequest().
		SetQtree(path).
		SetNewQtreeName(newPath).
		ExecuteUsing(c.zr)
	return response, err
}

// QtreeDestroyAsync destroys a qtree in the background
// equivalent to filer::> volume qtree delete -foreground false
func (c Client) QtreeDestroyAsync(path string, force bool) (*azgo.QtreeDeleteAsyncResponse, error) {
	response, err := azgo.NewQtreeDeleteAsyncRequest().
		SetQtree(path).
		SetForce(force).
		ExecuteUsing(c.zr)
	return response, err
}

// QtreeList returns the names of all Qtrees whose names match the supplied prefix
// equivalent to filer::> volume qtree show
func (c Client) QtreeList(prefix, volumePrefix string) (*azgo.QtreeListIterResponse, error) {
	// Limit the qtrees to those matching the Flexvol and Qtree name prefixes
	query := &azgo.QtreeListIterRequestQuery{}
	queryInfo := azgo.NewQtreeInfoType().SetVolume(volumePrefix + "*").SetQtree(prefix + "*")
	query.SetQtreeInfo(*queryInfo)

	// Limit the returned data to only the Flexvol, Qtree name and ExportPolicy
	desiredAttributes := &azgo.QtreeListIterRequestDesiredAttributes{}
	desiredInfo := azgo.NewQtreeInfoType().SetVolume("").SetQtree("").SetExportPolicy("")
	desiredAttributes.SetQtreeInfo(*desiredInfo)

	response, err := azgo.NewQtreeListIterRequest().
		SetMaxRecords(c.config.ContextBasedZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(c.zr)
	return response, err
}

// QtreeCount returns the number of Qtrees in the specified Flexvol, not including the Flexvol itself
func (c Client) QtreeCount(ctx context.Context, volume string) (int, error) {
	// Limit the qtrees to those in the specified Flexvol
	query := &azgo.QtreeListIterRequestQuery{}
	queryInfo := azgo.NewQtreeInfoType().SetVolume(volume)
	query.SetQtreeInfo(*queryInfo)

	// Limit the returned data to only the Flexvol and Qtree names
	desiredAttributes := &azgo.QtreeListIterRequestDesiredAttributes{}
	desiredInfo := azgo.NewQtreeInfoType().SetVolume("").SetQtree("")
	desiredAttributes.SetQtreeInfo(*desiredInfo)

	response, err := azgo.NewQtreeListIterRequest().
		SetMaxRecords(c.config.ContextBasedZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(c.zr)

	if err = azgo.GetError(ctx, response, err); err != nil {
		return 0, err
	}

	// There will always be one qtree for the Flexvol, so decrement by 1
	n := response.Result.NumRecords()
	switch n {
	case 0, 1:
		return 0, nil
	default:
		return n - 1, nil
	}
}

// QtreeExists returns true if the named Qtree exists (and is unique in the matching Flexvols)
func (c Client) QtreeExists(ctx context.Context, name, volumePattern string) (bool, string, error) {
	// Limit the qtrees to those matching the Flexvol and Qtree name prefixes
	query := &azgo.QtreeListIterRequestQuery{}
	queryInfo := azgo.NewQtreeInfoType().SetVolume(volumePattern).SetQtree(name)
	query.SetQtreeInfo(*queryInfo)

	// Limit the returned data to only the Flexvol and Qtree names
	desiredAttributes := &azgo.QtreeListIterRequestDesiredAttributes{}
	desiredInfo := azgo.NewQtreeInfoType().SetVolume("").SetQtree("")
	desiredAttributes.SetQtreeInfo(*desiredInfo)

	response, err := azgo.NewQtreeListIterRequest().
		SetMaxRecords(c.config.ContextBasedZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(c.zr)

	// Ensure the API call succeeded
	if err = azgo.GetError(ctx, response, err); err != nil {
		return false, "", err
	}

	// Ensure qtree is unique
	if response.Result.NumRecords() != 1 {
		return false, "", nil
	}

	if response.Result.AttributesListPtr == nil {
		return false, "", nil
	}

	// Get containing Flexvol
	flexvol := response.Result.AttributesListPtr.QtreeInfoPtr[0].Volume()

	return true, flexvol, nil
}

// QtreeGet returns all relevant details for a single qtree
// equivalent to filer::> volume qtree show
func (c Client) QtreeGet(name, volumePrefix string) (*azgo.QtreeInfoType, error) {
	// Limit the qtrees to those matching the Flexvol and Qtree name prefixes
	query := &azgo.QtreeListIterRequestQuery{}
	info := azgo.NewQtreeInfoType().SetVolume(volumePrefix + "*").SetQtree(name)
	query.SetQtreeInfo(*info)

	response, err := azgo.NewQtreeListIterRequest().
		SetMaxRecords(c.config.ContextBasedZapiRecords).
		SetQuery(*query).
		ExecuteUsing(c.zr)

	if err != nil {
		return &azgo.QtreeInfoType{}, err
	} else if response.Result.NumRecords() == 0 {
		return &azgo.QtreeInfoType{}, fmt.Errorf("qtree %s not found", name)
	} else if response.Result.NumRecords() > 1 {
		return &azgo.QtreeInfoType{}, fmt.Errorf("more than one qtree %s found", name)
	} else if response.Result.AttributesListPtr == nil {
		return &azgo.QtreeInfoType{}, fmt.Errorf("qtree %s not found", name)
	} else if response.Result.AttributesListPtr.QtreeInfoPtr != nil {
		return &response.Result.AttributesListPtr.QtreeInfoPtr[0], nil
	}
	return &azgo.QtreeInfoType{}, fmt.Errorf("qtree %s not found", name)
}

// QtreeGetAll returns all relevant details for all qtrees whose Flexvol names match the supplied prefix
// equivalent to filer::> volume qtree show
func (c Client) QtreeGetAll(volumePrefix string) (*azgo.QtreeListIterResponse, error) {
	// Limit the qtrees to those matching the Flexvol name prefix
	query := &azgo.QtreeListIterRequestQuery{}
	info := azgo.NewQtreeInfoType().SetVolume(volumePrefix + "*")
	query.SetQtreeInfo(*info)

	// Limit the returned data to only the data relevant to containers
	desiredAttributes := &azgo.QtreeListIterRequestDesiredAttributes{}
	desiredInfo := azgo.NewQtreeInfoType().
		SetVolume("").
		SetQtree("").
		SetSecurityStyle("").
		SetMode("").
		SetExportPolicy("")
	desiredAttributes.SetQtreeInfo(*desiredInfo)

	response, err := azgo.NewQtreeListIterRequest().
		SetMaxRecords(c.config.ContextBasedZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(c.zr)
	return response, err
}

func (c Client) QtreeModifyExportPolicy(name, volumeName, exportPolicy string) (*azgo.QtreeModifyResponse, error) {
	return azgo.NewQtreeModifyRequest().
		SetQtree(name).
		SetVolume(volumeName).
		SetExportPolicy(exportPolicy).
		ExecuteUsing(c.zr)
}

// QuotaOn enables quotas on a Flexvol
// equivalent to filer::> volume quota on
func (c Client) QuotaOn(volume string) (*azgo.QuotaOnResponse, error) {
	response, err := azgo.NewQuotaOnRequest().
		SetVolume(volume).
		ExecuteUsing(c.zr)
	return response, err
}

// QuotaOff disables quotas on a Flexvol
// equivalent to filer::> volume quota off
func (c Client) QuotaOff(volume string) (*azgo.QuotaOffResponse, error) {
	response, err := azgo.NewQuotaOffRequest().
		SetVolume(volume).
		ExecuteUsing(c.zr)
	return response, err
}

// QuotaResize resizes quotas on a Flexvol
// equivalent to filer::> volume quota resize
func (c Client) QuotaResize(volume string) (*azgo.QuotaResizeResponse, error) {
	response, err := azgo.NewQuotaResizeRequest().
		SetVolume(volume).
		ExecuteUsing(c.zr)
	return response, err
}

// QuotaStatus returns the quota status for a Flexvol
// equivalent to filer::> volume quota show
func (c Client) QuotaStatus(volume string) (*azgo.QuotaStatusResponse, error) {
	response, err := azgo.NewQuotaStatusRequest().
		SetVolume(volume).
		ExecuteUsing(c.zr)
	return response, err
}

// QuotaSetEntry creates a new quota rule with an optional hard disk limit
// equivalent to filer::> volume quota policy rule create
func (c Client) QuotaSetEntry(
	qtreeName, volumeName, quotaTarget, quotaType, diskLimit string,
) (*azgo.QuotaSetEntryResponse, error) {
	request := azgo.NewQuotaSetEntryRequest().
		SetQtree(qtreeName).
		SetVolume(volumeName).
		SetQuotaTarget(quotaTarget).
		SetQuotaType(quotaType)

	// To create a default quota rule, pass an empty disk limit
	if diskLimit != "" {
		request.SetDiskLimit(diskLimit)
	}

	response, err := request.ExecuteUsing(c.zr)
	return response, err
}

// QuotaEntryGet returns the disk limit for a single qtree
// equivalent to filer::> volume quota policy rule show
func (c Client) QuotaGetEntry(target, quotaType string) (*azgo.QuotaEntryType, error) {
	query := &azgo.QuotaListEntriesIterRequestQuery{}
	quotaEntry := azgo.NewQuotaEntryType().SetQuotaType(quotaType).SetQuotaTarget(target)
	query.SetQuotaEntry(*quotaEntry)

	// Limit the returned data to only the disk limit
	desiredAttributes := &azgo.QuotaListEntriesIterRequestDesiredAttributes{}
	desiredQuotaEntryFields := azgo.NewQuotaEntryType().SetDiskLimit("").SetQuotaTarget("")
	desiredAttributes.SetQuotaEntry(*desiredQuotaEntryFields)

	response, err := azgo.NewQuotaListEntriesIterRequest().
		SetMaxRecords(DefaultZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(c.zr)

	if err != nil {
		return &azgo.QuotaEntryType{}, err
	} else if response.Result.NumRecords() == 0 {
		return &azgo.QuotaEntryType{}, fmt.Errorf("tree quota for %s not found", target)
	} else if response.Result.NumRecords() > 1 {
		return &azgo.QuotaEntryType{}, fmt.Errorf("more than one tree quota for %s found", target)
	} else if response.Result.AttributesListPtr == nil {
		return &azgo.QuotaEntryType{}, fmt.Errorf("tree quota for %s not found", target)
	} else if response.Result.AttributesListPtr.QuotaEntryPtr != nil {
		return &response.Result.AttributesListPtr.QuotaEntryPtr[0], nil
	}
	return &azgo.QuotaEntryType{}, fmt.Errorf("tree quota for %s not found", target)
}

// QuotaEntryList returns the disk limit quotas for a Flexvol
// equivalent to filer::> volume quota policy rule show
func (c Client) QuotaEntryList(volume string) (*azgo.QuotaListEntriesIterResponse, error) {
	query := &azgo.QuotaListEntriesIterRequestQuery{}
	quotaEntry := azgo.NewQuotaEntryType().SetVolume(volume).SetQuotaType("tree")
	query.SetQuotaEntry(*quotaEntry)

	// Limit the returned data to only the disk limit
	desiredAttributes := &azgo.QuotaListEntriesIterRequestDesiredAttributes{}
	desiredQuotaEntryFields := azgo.NewQuotaEntryType().SetDiskLimit("").SetQuotaTarget("")
	desiredAttributes.SetQuotaEntry(*desiredQuotaEntryFields)

	response, err := azgo.NewQuotaListEntriesIterRequest().
		SetMaxRecords(DefaultZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(c.zr)
	return response, err
}

// QTREE operations END
// ///////////////////////////////////////////////////////////////////////////

// ///////////////////////////////////////////////////////////////////////////
// EXPORT POLICY operations BEGIN

// ExportPolicyCreate creates an export policy
// equivalent to filer::> vserver export-policy create
func (c Client) ExportPolicyCreate(policy string) (*azgo.ExportPolicyCreateResponse, error) {
	response, err := azgo.NewExportPolicyCreateRequest().
		SetPolicyName(policy).
		ExecuteUsing(c.zr)
	return response, err
}

func (c Client) ExportPolicyGet(policy string) (*azgo.ExportPolicyGetResponse, error) {
	return azgo.NewExportPolicyGetRequest().
		SetPolicyName(policy).
		ExecuteUsing(c.zr)
}

func (c Client) ExportPolicyDestroy(policy string) (*azgo.ExportPolicyDestroyResponse, error) {
	return azgo.NewExportPolicyDestroyRequest().
		SetPolicyName(policy).
		ExecuteUsing(c.zr)
}

// ExportRuleCreate creates a rule in an export policy
// equivalent to filer::> vserver export-policy rule create
func (c Client) ExportRuleCreate(
	policy, clientMatch string,
	protocols, roSecFlavors, rwSecFlavors, suSecFlavors []string,
) (*azgo.ExportRuleCreateResponse, error) {
	protocolTypes := &azgo.ExportRuleCreateRequestProtocol{}
	var protocolTypesToUse []azgo.AccessProtocolType
	for _, p := range protocols {
		protocolTypesToUse = append(protocolTypesToUse, azgo.AccessProtocolType(p))
	}
	protocolTypes.AccessProtocolPtr = protocolTypesToUse

	roSecFlavorTypes := &azgo.ExportRuleCreateRequestRoRule{}
	var roSecFlavorTypesToUse []azgo.SecurityFlavorType
	for _, f := range roSecFlavors {
		roSecFlavorTypesToUse = append(roSecFlavorTypesToUse, azgo.SecurityFlavorType(f))
	}
	roSecFlavorTypes.SecurityFlavorPtr = roSecFlavorTypesToUse

	rwSecFlavorTypes := &azgo.ExportRuleCreateRequestRwRule{}
	var rwSecFlavorTypesToUse []azgo.SecurityFlavorType
	for _, f := range rwSecFlavors {
		rwSecFlavorTypesToUse = append(rwSecFlavorTypesToUse, azgo.SecurityFlavorType(f))
	}
	rwSecFlavorTypes.SecurityFlavorPtr = rwSecFlavorTypesToUse

	suSecFlavorTypes := &azgo.ExportRuleCreateRequestSuperUserSecurity{}
	var suSecFlavorTypesToUse []azgo.SecurityFlavorType
	for _, f := range suSecFlavors {
		suSecFlavorTypesToUse = append(suSecFlavorTypesToUse, azgo.SecurityFlavorType(f))
	}
	suSecFlavorTypes.SecurityFlavorPtr = suSecFlavorTypesToUse

	response, err := azgo.NewExportRuleCreateRequest().
		SetPolicyName(azgo.ExportPolicyNameType(policy)).
		SetClientMatch(clientMatch).
		SetProtocol(*protocolTypes).
		SetRoRule(*roSecFlavorTypes).
		SetRwRule(*rwSecFlavorTypes).
		SetSuperUserSecurity(*suSecFlavorTypes).
		ExecuteUsing(c.zr)
	return response, err
}

// ExportRuleGetIterRequest returns the export rules in an export policy
// equivalent to filer::> vserver export-policy rule show
func (c Client) ExportRuleGetIterRequest(policy string) (*azgo.ExportRuleGetIterResponse, error) {
	// Limit the qtrees to those matching the Flexvol and Qtree name prefixes
	query := &azgo.ExportRuleGetIterRequestQuery{}
	exportRuleInfo := azgo.NewExportRuleInfoType().SetPolicyName(azgo.ExportPolicyNameType(policy))
	query.SetExportRuleInfo(*exportRuleInfo)

	response, err := azgo.NewExportRuleGetIterRequest().
		SetMaxRecords(DefaultZapiRecords).
		SetQuery(*query).
		ExecuteUsing(c.zr)
	return response, err
}

// ExportRuleDestroy deletes the rule at the given index in the given policy
func (c Client) ExportRuleDestroy(policy string, ruleIndex int) (*azgo.ExportRuleDestroyResponse, error) {
	response, err := azgo.NewExportRuleDestroyRequest().
		SetPolicyName(policy).
		SetRuleIndex(ruleIndex).
		ExecuteUsing(c.zr)
	if zerr := azgo.NewZapiError(response); !zerr.IsPassed() {
		// It's not an error if the export rule doesn't exist
		if zerr.Code() == azgo.EOBJECTNOTFOUND {
			return response, nil
		}
	}
	return response, err
}

// EXPORT POLICY operations END
// ///////////////////////////////////////////////////////////////////////////

// ///////////////////////////////////////////////////////////////////////////
// SNAPSHOT operations BEGIN

// SnapshotCreate creates a snapshot of a volume
func (c Client) SnapshotCreate(snapshotName, volumeName string) (*azgo.SnapshotCreateResponse, error) {
	response, err := azgo.NewSnapshotCreateRequest().
		SetSnapshot(snapshotName).
		SetVolume(volumeName).
		ExecuteUsing(c.zr)
	return response, err
}

// SnapshotList returns the list of snapshots associated with a volume
func (c Client) SnapshotList(volumeName string) (*azgo.SnapshotGetIterResponse, error) {
	query := &azgo.SnapshotGetIterRequestQuery{}
	snapshotInfo := azgo.NewSnapshotInfoType().SetVolume(volumeName)
	query.SetSnapshotInfo(*snapshotInfo)

	response, err := azgo.NewSnapshotGetIterRequest().
		SetMaxRecords(DefaultZapiRecords).
		SetQuery(*query).
		ExecuteUsing(c.zr)
	return response, err
}

// SnapshotInfo returns a snapshot by name for a volume
func (c Client) SnapshotInfo(snapshotName, volumeName string) (*azgo.SnapshotGetIterResponse, error) {
	query := &azgo.SnapshotGetIterRequestQuery{}
	snapshotInfo := azgo.NewSnapshotInfoType().
		SetVolume(volumeName).
		SetName(snapshotName)
	query.SetSnapshotInfo(*snapshotInfo)

	response, err := azgo.NewSnapshotGetIterRequest().
		SetMaxRecords(DefaultZapiRecords).
		SetQuery(*query).
		ExecuteUsing(c.zr)
	return response, err
}

// SnapshotRestoreVolume restores a volume to a snapshot as a non-blocking operation
func (c Client) SnapshotRestoreVolume(snapshotName, volumeName string) (*azgo.SnapshotRestoreVolumeResponse, error) {
	response, err := azgo.NewSnapshotRestoreVolumeRequest().
		SetVolume(volumeName).
		SetSnapshot(snapshotName).
		SetPreserveLunIds(true).
		ExecuteUsing(c.zr)
	return response, err
}

// DeleteSnapshot deletes a snapshot of a volume
func (c Client) SnapshotDelete(snapshotName, volumeName string) (*azgo.SnapshotDeleteResponse, error) {
	response, err := azgo.NewSnapshotDeleteRequest().
		SetVolume(volumeName).
		SetSnapshot(snapshotName).
		ExecuteUsing(c.zr)
	return response, err
}

// SNAPSHOT operations END
// ///////////////////////////////////////////////////////////////////////////

// ///////////////////////////////////////////////////////////////////////////
// ISCSI operations BEGIN

// IscsiServiceGetIterRequest returns information about an iSCSI target
func (c Client) IscsiServiceGetIterRequest() (*azgo.IscsiServiceGetIterResponse, error) {
	response, err := azgo.NewIscsiServiceGetIterRequest().
		SetMaxRecords(DefaultZapiRecords).
		ExecuteUsing(c.zr)
	return response, err
}

// IscsiNodeGetNameRequest gets the IQN of the vserver
func (c Client) IscsiNodeGetNameRequest() (*azgo.IscsiNodeGetNameResponse, error) {
	response, err := azgo.NewIscsiNodeGetNameRequest().ExecuteUsing(c.zr)
	return response, err
}

// IscsiInterfaceGetIterRequest returns information about the vserver's iSCSI interfaces
func (c Client) IscsiInterfaceGetIterRequest() (*azgo.IscsiInterfaceGetIterResponse, error) {
	response, err := azgo.NewIscsiInterfaceGetIterRequest().
		SetMaxRecords(DefaultZapiRecords).
		ExecuteUsing(c.zr)
	return response, err
}

// ISCSI operations END
// ///////////////////////////////////////////////////////////////////////////

// FCP operations BEGIN
// ///////////////////////////////////////////////////////////////////////////

func (c Client) FcpNodeGetNameRequest() (*azgo.FcpNodeGetNameResponse, error) {
	response, err := azgo.NewFcpNodeGetNameRequest().ExecuteUsing(c.zr)
	return response, err
}

// FcpInterfaceGetIterRequest returns information about the vserver's FCP interfaces
func (c Client) FcpInterfaceGetIterRequest() (*azgo.FcpInterfaceGetIterResponse, error) {
	response, err := azgo.NewFcpInterfaceGetIterRequest().
		SetMaxRecords(DefaultZapiRecords).
		ExecuteUsing(c.zr)
	return response, err
}

// FCP operations END
// ///////////////////////////////////////////////////////////////////////////

// ///////////////////////////////////////////////////////////////////////////
// VSERVER operations BEGIN

// VserverGetIterRequest returns the vservers on the system
// equivalent to filer::> vserver show
func (c Client) VserverGetIterRequest() (*azgo.VserverGetIterResponse, error) {
	response, err := azgo.NewVserverGetIterRequest().
		SetMaxRecords(DefaultZapiRecords).
		ExecuteUsing(c.zr)
	return response, err
}

// VserverGetIterAdminRequest returns vservers of type "admin" on the system.
// equivalent to filer::> vserver show -type admin
func (c Client) VserverGetIterAdminRequest() (*azgo.VserverGetIterResponse, error) {
	query := &azgo.VserverGetIterRequestQuery{}
	info := azgo.NewVserverInfoType().SetVserverType("admin")
	query.SetVserverInfo(*info)

	desiredAttributes := &azgo.VserverGetIterRequestDesiredAttributes{}
	desiredInfo := azgo.NewVserverInfoType().
		SetVserverName("").
		SetVserverType("")
	desiredAttributes.SetVserverInfo(*desiredInfo)

	response, err := azgo.NewVserverGetIterRequest().
		SetMaxRecords(DefaultZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(c.GetNontunneledZapiRunner())
	return response, err
}

// VserverGetRequest returns vserver to which it is sent
// equivalent to filer::> vserver show
func (c Client) VserverGetRequest() (*azgo.VserverGetResponse, error) {
	response, err := azgo.NewVserverGetRequest().ExecuteUsing(c.zr)
	return response, err
}

// SVMGetAggregateNames returns an array of names of the aggregates assigned to the configured vserver.
// The vserver-get-iter API works with either cluster or vserver scope, so the ZAPI runner may or may not
// be configured for tunneling; using the query parameter ensures we address only the configured vserver.
func (c Client) SVMGetAggregateNames() ([]string, error) {
	// Get just the SVM of interest
	query := &azgo.VserverGetIterRequestQuery{}
	info := azgo.NewVserverInfoType().SetVserverName(c.SVMName())
	query.SetVserverInfo(*info)

	response, err := azgo.NewVserverGetIterRequest().
		SetMaxRecords(DefaultZapiRecords).
		SetQuery(*query).
		ExecuteUsing(c.zr)
	if err != nil {
		return nil, err
	}
	if response.Result.NumRecords() != 1 {
		return nil, fmt.Errorf("could not find SVM %s", c.SVMName())
	}

	// Get the aggregates assigned to the SVM
	aggrNames := make([]string, 0, 10)
	if response.Result.AttributesListPtr != nil {
		for _, vserver := range response.Result.AttributesListPtr.VserverInfoPtr {
			if vserver.VserverAggrInfoListPtr != nil {
				for _, aggr := range vserver.VserverAggrInfoList().VserverAggrInfoPtr {
					aggrNames = append(aggrNames, string(aggr.AggrName()))
				}
			}
		}
	}

	return aggrNames, nil
}

// VserverShowAggrGetIterRequest returns the aggregates on the vserver.  Requires ONTAP 9 or later.
// equivalent to filer::> vserver show-aggregates
func (c Client) VserverShowAggrGetIterRequest() (*azgo.VserverShowAggrGetIterResponse, error) {
	response, err := azgo.NewVserverShowAggrGetIterRequest().
		SetMaxRecords(MaxZapiRecords).
		ExecuteUsing(c.zr)
	return response, err
}

// GetSVMState returns the state of SVM on the vserver.
func (c Client) GetSVMState(ctx context.Context) (string, error) {
	// Get just the SVM of interest
	query := &azgo.VserverGetIterRequestQuery{}
	info := azgo.NewVserverInfoType().SetVserverName(c.SVMName())
	query.SetVserverInfo(*info)

	response, err := azgo.NewVserverGetIterRequest().
		SetMaxRecords(DefaultZapiRecords).
		SetQuery(*query).
		ExecuteUsing(c.zr)
	if err != nil {
		return "", err
	}

	if response.Result.NumRecords() != 1 ||
		response.Result.AttributesListPtr == nil ||
		response.Result.AttributesListPtr.VserverInfoPtr == nil {
		return "", fmt.Errorf("could not find SVM %s (%s)", c.SVMName(), c.svmUUID)
	}

	return response.Result.AttributesListPtr.VserverInfoPtr[0].State(), nil
}

// VSERVER operations END
// ///////////////////////////////////////////////////////////////////////////

// ///////////////////////////////////////////////////////////////////////////
// AGGREGATE operations BEGIN

// AggrSpaceGetIterRequest returns the aggregates on the system
// equivalent to filer::> storage aggregate show-space -aggregate-name aggregate
func (c Client) AggrSpaceGetIterRequest(aggregateName string) (*azgo.AggrSpaceGetIterResponse, error) {
	zr := c.GetNontunneledZapiRunner()

	query := &azgo.AggrSpaceGetIterRequestQuery{}
	querySpaceInformation := azgo.NewSpaceInformationType()
	if aggregateName != "" {
		querySpaceInformation.SetAggregate(aggregateName)
	}
	query.SetSpaceInformation(*querySpaceInformation)

	responseAggrSpace, err := azgo.NewAggrSpaceGetIterRequest().
		SetQuery(*query).
		ExecuteUsing(zr)
	return responseAggrSpace, err
}

func (c Client) getAggregateSize(ctx context.Context, aggregateName string) (int, error) {
	// First, lookup the aggregate and it's space used
	aggregateSizeTotal := NumericalValueNotSet

	responseAggrSpace, err := c.AggrSpaceGetIterRequest(aggregateName)
	if err = azgo.GetError(ctx, responseAggrSpace, err); err != nil {
		return NumericalValueNotSet, fmt.Errorf("error getting size for aggregate %v: %v", aggregateName, err)
	}

	if responseAggrSpace.Result.AttributesListPtr != nil {
		for _, aggrSpace := range responseAggrSpace.Result.AttributesListPtr.SpaceInformationPtr {
			aggregateSizeTotal = aggrSpace.AggregateSize()
			return aggregateSizeTotal, nil
		}
	}

	return aggregateSizeTotal, fmt.Errorf("error getting size for aggregate %v", aggregateName)
}

type AggregateCommitment struct {
	AggregateSize  float64
	TotalAllocated float64
}

func (o *AggregateCommitment) Percent() float64 {
	committedPercent := (o.TotalAllocated / float64(o.AggregateSize)) * 100.0
	return committedPercent
}

func (o *AggregateCommitment) PercentWithRequestedSize(requestedSize float64) float64 {
	committedPercent := ((o.TotalAllocated + requestedSize) / float64(o.AggregateSize)) * 100.0
	return committedPercent
}

func (o AggregateCommitment) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %1.f ", "AggregateSize", o.AggregateSize))
	buffer.WriteString(fmt.Sprintf("%s: %1.f ", "TotalAllocated", o.TotalAllocated))
	buffer.WriteString(fmt.Sprintf("%s: %.2f %%", "Percent", o.Percent()))
	return buffer.String()
}

// AggregateCommitmentPercentage returns the allocated capacity percentage for an aggregate
// See also;  https://practical-admin.com/blog/netapp-powershell-toolkit-aggregate-overcommitment-report/
func (c Client) AggregateCommitment(ctx context.Context, aggregate string) (*AggregateCommitment, error) {
	zr := c.GetNontunneledZapiRunner()

	// first, get the aggregate's size
	aggregateSize, err := c.getAggregateSize(ctx, aggregate)
	if err != nil {
		return nil, err
	}

	// now, get all of the aggregate's volumes
	query := &azgo.VolumeGetIterRequestQuery{}
	queryVolIDAttrs := azgo.NewVolumeIdAttributesType().
		SetContainingAggregateName(aggregate)
	queryVolSpaceAttrs := azgo.NewVolumeSpaceAttributesType()
	volumeAttributes := azgo.NewVolumeAttributesType().
		SetVolumeIdAttributes(*queryVolIDAttrs).
		SetVolumeSpaceAttributes(*queryVolSpaceAttrs)
	query.SetVolumeAttributes(*volumeAttributes)

	response, err := azgo.NewVolumeGetIterRequest().
		SetMaxRecords(DefaultZapiRecords).
		SetQuery(*query).
		ExecuteUsing(zr)
	if err != nil {
		return nil, err
	}
	if err = azgo.GetError(ctx, response, err); err != nil {
		return nil, fmt.Errorf("error enumerating Flexvols: %v", err)
	}

	totalAllocated := 0.0

	// for each of the aggregate's volumes, compute its potential storage usage
	if response.Result.AttributesListPtr != nil {
		for _, volAttrs := range response.Result.AttributesListPtr.VolumeAttributesPtr {
			volIDAttrs := volAttrs.VolumeIdAttributes()
			volName := string(volIDAttrs.Name())
			volSpaceAttrs := volAttrs.VolumeSpaceAttributes()
			volSisAttrs := volAttrs.VolumeSisAttributes()
			volAllocated := float64(volSpaceAttrs.SizeTotal())

			Logc(ctx).WithFields(LogFields{
				"volName":         volName,
				"SizeTotal":       volSpaceAttrs.SizeTotal(),
				"TotalSpaceSaved": volSisAttrs.TotalSpaceSaved(),
				"volAllocated":    volAllocated,
			}).Info("Dumping volume")

			lunAllocated := 0.0
			lunsResponse, lunsResponseErr := c.LunGetAllForVolume(volName)
			if lunsResponseErr != nil {
				return nil, lunsResponseErr
			}
			if lunsResponseErr = azgo.GetError(ctx, lunsResponse, lunsResponseErr); lunsResponseErr != nil {
				return nil, fmt.Errorf("error enumerating LUNs for volume %v: %v", volName, lunsResponseErr)
			}

			if lunsResponse.Result.AttributesListPtr != nil &&
				lunsResponse.Result.AttributesListPtr.LunInfoPtr != nil {
				for _, lun := range lunsResponse.Result.AttributesListPtr.LunInfoPtr {
					lunPath := lun.Path()
					lunSize := lun.Size()
					Logc(ctx).WithFields(LogFields{
						"lunPath": lunPath,
						"lunSize": lunSize,
					}).Info("Dumping LUN")
					lunAllocated += float64(lunSize)
				}
			}

			if lunAllocated > volAllocated {
				totalAllocated += float64(lunAllocated)
			} else {
				totalAllocated += float64(volAllocated)
			}
		}
	}

	ac := &AggregateCommitment{
		TotalAllocated: totalAllocated,
		AggregateSize:  float64(aggregateSize),
	}

	return ac, nil
}

// AGGREGATE operations END
// ///////////////////////////////////////////////////////////////////////////

// ///////////////////////////////////////////////////////////////////////////
// SNAPMIRROR operations BEGIN

// ToSnapmirrorLocation returns a string in the form "svmName:volumeName" for use in snapmirror calls
func ToSnapmirrorLocation(svmName, volumeName string) string {
	return fmt.Sprintf("%s:%s", svmName, volumeName)
}

// SnapmirrorGetIterRequest returns the snapmirror operations on the destination cluster
// equivalent to filer::> snapmirror show
func (c Client) SnapmirrorGetIterRequest(relGroupType string) (*azgo.SnapmirrorGetIterResponse, error) {
	// Limit list-destination to relationship-group-type matching passed relGroupType
	query := &azgo.SnapmirrorGetIterRequestQuery{}
	relationshipGroupType := azgo.NewSnapmirrorInfoType().
		SetRelationshipGroupType(relGroupType)
	query.SetSnapmirrorInfo(*relationshipGroupType)

	response, err := azgo.NewSnapmirrorGetIterRequest().
		SetQuery(*query).
		ExecuteUsing(c.zr)
	return response, err
}

// SnapmirrorGetDestinationIterRequest returns the snapmirror operations on the source cluster
// equivalent to filer::> snapmirror list-destinations
func (c Client) SnapmirrorGetDestinationIterRequest(
	relGroupType string,
) (*azgo.SnapmirrorGetDestinationIterResponse, error) {
	// Limit list-destination to relationship-group-type matching passed relGroupType
	query := &azgo.SnapmirrorGetDestinationIterRequestQuery{}
	relationshipGroupType := azgo.NewSnapmirrorDestinationInfoType().
		SetRelationshipGroupType(relGroupType)
	query.SetSnapmirrorDestinationInfo(*relationshipGroupType)

	response, err := azgo.NewSnapmirrorGetDestinationIterRequest().
		SetQuery(*query).
		ExecuteUsing(c.zr)
	return response, err
}

// GetPeeredVservers returns a list of vservers peered with the vserver for this backend
func (c Client) GetPeeredVservers(ctx context.Context) ([]string, error) {
	peers := *new([]string)

	query := &azgo.VserverPeerGetIterRequestQuery{}
	info := azgo.VserverPeerInfoType{}
	info.SetVserver(c.SVMName())
	query.SetVserverPeerInfo(info)

	response, err := azgo.NewVserverPeerGetIterRequest().
		SetQuery(*query).
		ExecuteUsing(c.zr)

	if response.Result.AttributesListPtr != nil {
		for _, peerInfo := range response.Result.AttributesListPtr.VserverPeerInfo() {
			peeredVserver := peerInfo.PeerVserver()
			peers = append(peers, peeredVserver)
		}
	}
	return peers, err
}

// IsVserverDRDestination identifies if the Vserver is a destination vserver of Snapmirror relationship (SVM-DR) or not
func (c Client) IsVserverDRDestination(ctx context.Context) (bool, error) {
	// first, get the snapmirror destination info using relationship-group-type=vserver in a snapmirror relationship
	relationshipGroupType := "vserver"
	response, err := c.SnapmirrorGetIterRequest(relationshipGroupType)
	isSVMDRDestination := false

	if err != nil {
		return isSVMDRDestination, err
	}
	if err = azgo.GetError(ctx, response, err); err != nil {
		return isSVMDRDestination, fmt.Errorf("error getting snapmirror info: %v", err)
	}

	// for each of the aggregate's volumes, compute its potential storage usage
	if response.Result.AttributesListPtr != nil {
		for _, volAttrs := range response.Result.AttributesListPtr.SnapmirrorInfoPtr {
			destinationLocation := volAttrs.DestinationLocation()
			destinationVserver := volAttrs.DestinationVserver()

			if (destinationVserver + ":") == destinationLocation {
				isSVMDRDestination = true
			}
		}
	}
	return isSVMDRDestination, err
}

// IsVserverDRSource identifies if the Vserver is a source vserver of Snapmirror relationship (SVM-DR) or not
func (c Client) IsVserverDRSource(ctx context.Context) (bool, error) {
	// first, get the snapmirror destination info using relationship-group-type=vserver in a snapmirror relationship
	relationshipGroupType := "vserver"
	response, err := c.SnapmirrorGetDestinationIterRequest(relationshipGroupType)
	isSVMDRSource := false

	if err != nil {
		return isSVMDRSource, err
	}
	if err = azgo.GetError(ctx, response, err); err != nil {
		return isSVMDRSource, fmt.Errorf("error getting snapmirror destination info: %v", err)
	}

	// for each of the aggregate's volumes, compute its potential storage usage
	// TODO: need to change to DR Source
	if response.Result.AttributesListPtr != nil {
		for _, volAttrs := range response.Result.AttributesListPtr.SnapmirrorDestinationInfoPtr {
			destinationLocation := volAttrs.DestinationLocation()
			destinationVserver := volAttrs.DestinationVserver()

			if (destinationVserver + ":") == destinationLocation {
				isSVMDRSource = true
			}
		}
	}
	return isSVMDRSource, err
}

// isVserverInSVMDR identifies if the Vserver is in Snapmirror relationship (SVM-DR) or not
func (c Client) isVserverInSVMDR(ctx context.Context) bool {
	isSVMDRSource, _ := c.IsVserverDRSource(ctx)
	isSVMDRDestination, _ := c.IsVserverDRDestination(ctx)

	return isSVMDRSource || isSVMDRDestination
}

func (c Client) SnapmirrorGet(
	localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName string,
) (*azgo.SnapmirrorGetResponse, error) {
	query := azgo.NewSnapmirrorGetRequest()
	query.SetDestinationLocation(ToSnapmirrorLocation(c.SVMName(), localInternalVolumeName))
	if remoteSVMName != "" && remoteFlexvolName != "" {
		query.SetSourceLocation(ToSnapmirrorLocation(remoteSVMName, remoteFlexvolName))
	}

	return query.ExecuteUsing(c.zr)
}

func (c Client) SnapmirrorCreate(
	localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName, repPolicy, repSchedule string,
) (*azgo.SnapmirrorCreateResponse, error) {
	query := azgo.NewSnapmirrorCreateRequest()
	query.SetDestinationLocation(ToSnapmirrorLocation(c.SVMName(), localInternalVolumeName))
	query.SetSourceLocation(ToSnapmirrorLocation(remoteSVMName, remoteFlexvolName))

	if repPolicy != "" {
		query.SetPolicy(repPolicy)
	}
	if repSchedule != "" {
		query.SetSchedule(repSchedule)
	}

	return query.ExecuteUsing(c.zr)
}

func (c Client) SnapmirrorInitialize(
	localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName string,
) (*azgo.SnapmirrorInitializeResponse, error) {
	query := azgo.NewSnapmirrorInitializeRequest()
	query.SetDestinationLocation(ToSnapmirrorLocation(c.SVMName(), localInternalVolumeName))
	query.SetSourceLocation(ToSnapmirrorLocation(remoteSVMName, remoteFlexvolName))

	return query.ExecuteUsing(c.zr)
}

func (c Client) SnapmirrorResync(
	localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName string,
) (*azgo.SnapmirrorResyncResponse, error) {
	query := azgo.NewSnapmirrorResyncRequest()
	query.SetDestinationLocation(ToSnapmirrorLocation(c.SVMName(), localInternalVolumeName))
	query.SetSourceLocation(ToSnapmirrorLocation(remoteSVMName, remoteFlexvolName))

	response, err := query.ExecuteUsing(c.zr)
	if err != nil {
		return response, err
	}
	err = c.WaitForAsyncResponse(GenerateRequestContext(context.Background(), "", "",
		WorkflowNone, LogLayer(c.driverName)), *response, 60)
	return response, err
}

func (c Client) SnapmirrorBreak(
	localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName, snapshotName string,
) (*azgo.SnapmirrorBreakResponse, error) {
	query := azgo.NewSnapmirrorBreakRequest()
	query.SetDestinationLocation(ToSnapmirrorLocation(c.SVMName(), localInternalVolumeName))
	query.SetSourceLocation(ToSnapmirrorLocation(remoteSVMName, remoteFlexvolName))

	if snapshotName != "" {
		query.SetRestoreDestinationToSnapshot(snapshotName)
	}

	return query.ExecuteUsing(c.zr)
}

func (c Client) SnapmirrorQuiesce(
	localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName string,
) (*azgo.SnapmirrorQuiesceResponse, error) {
	query := azgo.NewSnapmirrorQuiesceRequest()
	query.SetDestinationLocation(ToSnapmirrorLocation(c.SVMName(), localInternalVolumeName))
	query.SetSourceLocation(ToSnapmirrorLocation(remoteSVMName, remoteFlexvolName))

	return query.ExecuteUsing(c.zr)
}

func (c Client) SnapmirrorAbort(
	localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName string,
) (*azgo.SnapmirrorAbortResponse, error) {
	query := azgo.NewSnapmirrorAbortRequest()
	query.SetDestinationLocation(ToSnapmirrorLocation(c.SVMName(), localInternalVolumeName))
	query.SetSourceLocation(ToSnapmirrorLocation(remoteSVMName, remoteFlexvolName))

	return query.ExecuteUsing(c.zr)
}

// SnapmirrorRelease removes all local snapmirror relationship metadata from the source vserver
// Intended to be used on the source vserver
func (c Client) SnapmirrorRelease(sourceFlexvolName, sourceSVMName string) error {
	query := azgo.SnapmirrorGetDestinationIterRequestQuery{}
	params := azgo.NewSnapmirrorDestinationInfoType()
	params.SetSourceLocation(ToSnapmirrorLocation(sourceSVMName, sourceFlexvolName))
	query.SetSnapmirrorDestinationInfo(*params)
	request := azgo.NewSnapmirrorGetDestinationIterRequest()
	request.SetQuery(query)

	response, err := request.ExecuteUsing(c.zr)
	if err != nil {
		return err
	}

	list := response.Result.AttributesList()
	relationships := list.SnapmirrorDestinationInfo()

	for _, destinationInfo := range relationships {

		var destinationLocation *string
		destinationVserver := destinationInfo.DestinationVserverPtr
		destinationVolume := destinationInfo.DestinationVolumePtr
		if destinationInfo.DestinationLocationPtr != nil {
			destinationLocation = destinationInfo.DestinationLocationPtr
		} else if destinationVserver != nil && destinationVolume != nil {
			destinationLocation = convert.ToPtr(ToSnapmirrorLocation(*destinationVserver, *destinationVolume))
		} else {
			destinationLocation = nil
		}

		if destinationLocation != nil && *destinationLocation != "" {
			requestQuery := azgo.SnapmirrorReleaseIterRequestQuery{
				SnapmirrorDestinationInfoPtr: &azgo.SnapmirrorDestinationInfoType{
					DestinationLocationPtr: destinationLocation,
				},
			}
			releaseRequest := azgo.SnapmirrorReleaseIterRequest{QueryPtr: &requestQuery}
			_, err = releaseRequest.ExecuteUsing(c.zr)
			if err != nil {
				return err
			} else {
				return nil
			}
		} else {
			Logc(context.Background()).WithFields(LogFields{
				"destinationInfo": destinationInfo,
			}).Warn("Missing destination location.")
		}
	}
	return errors.NotFoundError("could not find snapmirror relationship to release")
}

// SnapmirrorDestinationRelease removes all local snapmirror relationship metadata of the destination volume
// Intended to be used on the destination vserver
func (c Client) SnapmirrorDestinationRelease(localInternalVolumeName string) (*azgo.SnapmirrorReleaseResponse, error) {
	request := azgo.NewSnapmirrorReleaseRequest()
	request.SetDestinationLocation(ToSnapmirrorLocation(c.SVMName(), localInternalVolumeName))
	request.SetRelationshipInfoOnly(true)

	return request.ExecuteUsing(c.zr)
}

// Intended to be from the destination vserver
func (c Client) SnapmirrorDeleteViaDestination(
	localInternalVolumeName, localSVMName string,
) (*azgo.SnapmirrorDestroyResponse, error) {
	query := azgo.NewSnapmirrorDestroyRequest()
	query.SetDestinationLocation(ToSnapmirrorLocation(c.SVMName(), localInternalVolumeName))

	return query.ExecuteUsing(c.zr)
}

// Intended to be from the destination vserver
func (c Client) SnapmirrorDelete(
	localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName string,
) (*azgo.SnapmirrorDestroyResponse, error) {
	query := azgo.NewSnapmirrorDestroyRequest()
	query.SetDestinationLocation(ToSnapmirrorLocation(c.SVMName(), localInternalVolumeName))
	query.SetSourceLocation(ToSnapmirrorLocation(remoteSVMName, remoteFlexvolName))

	return query.ExecuteUsing(c.zr)
}

func (c Client) IsVserverDRCapable(ctx context.Context) (bool, error) {
	query := &azgo.VserverPeerGetIterRequestQuery{}

	response, err := azgo.NewVserverPeerGetIterRequest().
		SetQuery(*query).
		ExecuteUsing(c.zr)
	if err != nil {
		return false, err
	}
	if err = azgo.GetError(ctx, response, err); err != nil {
		return false, fmt.Errorf("error getting peer info: %v", err)
	}

	peerFound := false
	if response.Result.AttributesListPtr != nil {
		for _, peerInfo := range response.Result.AttributesListPtr.VserverPeerInfo() {
			peeredVserver := peerInfo.Vserver()
			if peeredVserver == c.SVMName() {
				peerFound = true
			}
		}
	}

	return peerFound, err
}

func (c Client) SnapmirrorPolicyExists(ctx context.Context, policyName string) (bool, error) {
	result, err := azgo.NewSnapmirrorPolicyGetIterRequest().ExecuteUsing(c.zr)
	if err = azgo.GetError(ctx, result, err); err != nil {
		return false, fmt.Errorf("error listing snapmirror policies: %v", err)
	}

	list := result.Result.AttributesList()
	policies := list.SnapmirrorPolicyInfo()

	for _, policy := range policies {
		if policyName == policy.PolicyName() {
			return true, nil
		}
	}

	return false, nil
}

func (c Client) SnapmirrorPolicyGet(
	ctx context.Context, policyName string,
) (*azgo.SnapmirrorPolicyInfoType, error) {
	desiredPolicy := azgo.NewSnapmirrorPolicyInfoType()
	desiredPolicy.SetPolicyName(policyName)

	query := &azgo.SnapmirrorPolicyGetIterRequestQuery{}
	query.SetSnapmirrorPolicyInfo(*desiredPolicy)

	result, err := azgo.NewSnapmirrorPolicyGetIterRequest().SetQuery(*query).ExecuteUsing(c.zr)
	if err = azgo.GetError(ctx, result, err); err != nil {
		return nil, fmt.Errorf("error listing snapmirror policies: %v", err)
	}

	list := result.Result.AttributesList()
	policies := list.SnapmirrorPolicyInfo()

	switch len(policies) {
	case 0:
		return nil, errors.NotFoundError("could not find snapmirror policy %v", policyName)
	case 1:
		return &policies[0], nil
	default:
		return nil, fmt.Errorf("more than one snapmirror policy found with name %v", policyName)
	}
}

func (c Client) JobScheduleExists(ctx context.Context, jobName string) (bool, error) {
	jobScheduleInfo := azgo.NewJobScheduleInfoType()
	jobScheduleInfo.SetJobScheduleName(jobName)

	query := &azgo.JobScheduleGetIterRequestQuery{}
	query.SetJobScheduleInfo(*jobScheduleInfo)

	result, err := azgo.NewJobScheduleGetIterRequest().SetQuery(*query).ExecuteUsing(c.zr)
	if err = azgo.GetError(ctx, result, err); err != nil {
		return false, fmt.Errorf("error getting snapmirror policy %s: %v", jobName, err)
	}

	list := result.Result.AttributesList()
	schedules := list.JobScheduleInfo()

	for _, job := range schedules {
		if jobName == job.JobScheduleName() {
			return true, nil
		}
	}

	return false, nil
}

func (c Client) SnapmirrorUpdate(
	localInternalVolumeName, snapshotName string,
) (*azgo.SnapmirrorUpdateResponse, error) {
	query := azgo.NewSnapmirrorUpdateRequest()
	query.SetDestinationLocation(ToSnapmirrorLocation(c.SVMName(), localInternalVolumeName))

	if snapshotName != "" {
		query.SetSourceSnapshot(snapshotName)
	}

	return query.ExecuteUsing(c.zr)
}

// SNAPMIRROR operations END
// ///////////////////////////////////////////////////////////////////////////

// ///////////////////////////////////////////////////////////////////////////
// MISC operations BEGIN

// NetInterfaceGet returns the list of network interfaces with associated metadata
// equivalent to filer::> net interface list, but only those LIFs that are operational
func (c Client) NetInterfaceGet() (*azgo.NetInterfaceGetIterResponse, error) {
	response, err := azgo.NewNetInterfaceGetIterRequest().
		SetMaxRecords(DefaultZapiRecords).
		SetQuery(azgo.NetInterfaceGetIterRequestQuery{
			NetInterfaceInfoPtr: &azgo.NetInterfaceInfoType{},
		}).ExecuteUsing(c.zr)

	return response, err
}

func (c Client) NetInterfaceGetDataLIFs(ctx context.Context, protocol string) ([]string, error) {
	lifResponse, err := c.NetInterfaceGet()
	if err = azgo.GetError(ctx, lifResponse, err); err != nil {
		return nil, fmt.Errorf("error checking network interfaces: %v", err)
	}

	dataLIFs := make([]string, 0)
	if lifResponse.Result.AttributesListPtr != nil {
		for _, attrs := range lifResponse.Result.AttributesListPtr.NetInterfaceInfoPtr {
			if attrs.OperationalStatus() == LifOperationalStatusUp {
				for _, proto := range attrs.DataProtocols().DataProtocolPtr {
					if proto == protocol {
						dataLIFs = append(dataLIFs, attrs.Address())
					}
				}
			}
		}
	}

	if len(dataLIFs) < 1 {
		return []string{}, fmt.Errorf("no data LIFs meet the provided criteria (protocol: %s)", protocol)
	}

	Logc(ctx).WithField("dataLIFs", dataLIFs).Debug("Data LIFs")
	return dataLIFs, nil
}

func (c Client) NetFcpInterfaceGetDataLIFs(ctx context.Context, protocol string) ([]string, error) {
	lifResponse, err := c.NetInterfaceGet()
	if err = azgo.GetError(ctx, lifResponse, err); err != nil {
		return nil, fmt.Errorf("error checking network interfaces: %v", err)
	}

	dataLIFs := make([]string, 0)
	if lifResponse.Result.AttributesListPtr != nil {
		for _, attrs := range lifResponse.Result.AttributesListPtr.NetInterfaceInfoPtr {
			if attrs.OperationalStatus() == LifOperationalStatusUp {
				for _, proto := range attrs.DataProtocols().DataProtocolPtr {
					if proto == protocol {
						dataLIFs = append(dataLIFs, attrs.Wwpn())
					}
				}
			}
		}
	}

	if len(dataLIFs) < 1 {
		return []string{}, fmt.Errorf("no data LIFs meet the provided criteria (protocol: %s)", protocol)
	}

	Logc(ctx).WithField("dataLIFs", dataLIFs).Debug("Data LIFs")
	return dataLIFs, nil
}

// SystemGetVersion returns the system version
// equivalent to filer::> version
func (c Client) SystemGetVersion() (*azgo.SystemGetVersionResponse, error) {
	response, err := azgo.NewSystemGetVersionRequest().ExecuteUsing(c.zr)
	return response, err
}

// SystemGetOntapiVersion gets the ONTAPI version using the credentials, and caches & returns the result.
func (c Client) SystemGetOntapiVersion(ctx context.Context, cached bool) (string, error) {
	if c.zr.OntapiVersion == "" || !cached {
		result, err := azgo.NewSystemGetOntapiVersionRequest().ExecuteUsing(c.zr)
		if err = azgo.GetError(ctx, result, err); err != nil {
			return "", fmt.Errorf("could not read ONTAPI version: %v", err)
		}

		major := result.Result.MajorVersion()
		minor := result.Result.MinorVersion()
		c.zr.OntapiVersion = fmt.Sprintf("%d.%d", major, minor)
	}

	return c.zr.OntapiVersion, nil
}

func (c Client) NodeListSerialNumbers(ctx context.Context) ([]string, error) {
	serialNumbers := make([]string, 0)
	zr := c.GetNontunneledZapiRunner()

	// Limit the returned data to only the serial numbers
	desiredAttributes := &azgo.SystemNodeGetIterRequestDesiredAttributes{}
	info := azgo.NewNodeDetailsInfoType().SetNodeSerialNumber("")
	desiredAttributes.SetNodeDetailsInfo(*info)

	response, err := azgo.NewSystemNodeGetIterRequest().
		SetDesiredAttributes(*desiredAttributes).
		SetMaxRecords(DefaultZapiRecords).
		ExecuteUsing(zr)

	Logc(ctx).WithFields(LogFields{
		"response":          response,
		"info":              info,
		"desiredAttributes": desiredAttributes,
		"err":               err,
	}).Debug("NodeListSerialNumbers")

	if err = azgo.GetError(ctx, response, err); err != nil {
		return serialNumbers, err
	}

	if response.Result.NumRecords() == 0 {
		return serialNumbers, errors.New("could not get node info")
	}

	// Get the serial numbers
	if response.Result.AttributesListPtr != nil {
		for _, node := range response.Result.AttributesListPtr.NodeDetailsInfo() {
			if node.NodeSerialNumberPtr != nil {
				serialNumber := node.NodeSerialNumber()
				if serialNumber != "" {
					serialNumbers = append(serialNumbers, serialNumber)
				}
			}
		}
	}

	if len(serialNumbers) == 0 {
		return serialNumbers, errors.New("could not get node serial numbers")
	}

	Logc(ctx).WithFields(LogFields{
		"Count":         len(serialNumbers),
		"SerialNumbers": strings.Join(serialNumbers, ","),
	}).Debug("Read serial numbers.")

	return serialNumbers, nil
}

// EmsAutosupportLog generates an auto support message with the supplied parameters
func (c Client) EmsAutosupportLog(
	appVersion string,
	autoSupport bool,
	category string,
	computerName string,
	eventDescription string,
	eventID int,
	eventSource string,
	logLevel int,
) (*azgo.EmsAutosupportLogResponse, error) {
	response, err := azgo.NewEmsAutosupportLogRequest().
		SetAutoSupport(autoSupport).
		SetAppVersion(appVersion).
		SetCategory(category).
		SetComputerName(computerName).
		SetEventDescription(eventDescription).
		SetEventId(eventID).
		SetEventSource(eventSource).
		SetLogLevel(logLevel).
		ExecuteUsing(c.zr)
	return response, err
}

// ONTAP tiering Policy value is set based on below rules
//
// =================================================================================
// SVM-DR - Value applicable to source SVM (and destination cluster during failover)
// =================================================================================
// ONTAP DRIVER             ONTAP 9.3                           ONTAP 9.4                   ONTAP 9.5
// ONTAP-NAS                snapshot-only/pass                  snapshot-only/pass          none/pass
// ONTAP-NAS-ECO            snapshot-only/pass                  snapshot-only/pass          none/pass
// ONTAP-Flexgroup          NA                                  NA                          NA
//
//
// ==========
// Non-SVM-DR
// ==========
// ONTAP DRIVER             ONTAP 9.3                           ONTAP 9.4                   ONTAP 9.5
// ONTAP-NAS                none/pass                           none/pass                   none/pass
// ONTAP-NAS-ECO            none/pass                           none/pass                   none/pass
// ONTAP-Flexgroup          ONTAP-default(snapshot-only)/pass   ONTAP-default(none)/pass    none/pass
//
// PLEASE NOTE:
// 1. We try to set 'none' default tieirng policy when possible except when in SVM-DR relationship for ONTAP 9.4 and
// before only valid tiering policy value is 'snapshot-only'.
// 2. In SVM-DR relationship FlexGroups are not allowed.
//

func (c Client) TieringPolicyValue(ctx context.Context) string {
	tieringPolicy := "none"
	// If ONTAP version < 9.5
	if !c.SupportsFeature(ctx, FabricPoolForSVMDR) {
		if c.isVserverInSVMDR(ctx) {
			tieringPolicy = "snapshot-only"
		}
	}

	return tieringPolicy
}

// MISC operations END
// ///////////////////////////////////////////////////////////////////////////

// ///////////////////////////////////////////////////////////////////////////
// iSCSI initiator operations BEGIN

// IscsiInitiatorAddAuth creates and sets the authorization details for a single initiator
//
//	equivalent to filer::> vserver iscsi security create -vserver SVM -initiator-name iqn.1993-08.org.debian:01:9031309bbebd \
//	                         -auth-type CHAP -user-name outboundUserName -outbound-user-name outboundPassphrase
func (c Client) IscsiInitiatorAddAuth(
	initiator, authType, userName, passphrase, outboundUserName, outboundPassphrase string,
) (*azgo.IscsiInitiatorAddAuthResponse, error) {
	request := azgo.NewIscsiInitiatorAddAuthRequest().
		SetInitiator(initiator).
		SetAuthType(authType).
		SetUserName(userName).
		SetPassphrase(passphrase)
	if outboundUserName != "" && outboundPassphrase != "" {
		request.SetOutboundUserName(outboundUserName)
		request.SetOutboundPassphrase(outboundPassphrase)
	}
	response, err := request.ExecuteUsing(c.zr)
	return response, err
}

// IscsiInitiatorAuthGetIter returns the authorization details for all non-default initiators for the Client's SVM
// equivalent to filer::> vserver iscsi security show -vserver SVM
func (c Client) IscsiInitiatorAuthGetIter() ([]azgo.IscsiSecurityEntryInfoType, error) {
	response, err := azgo.NewIscsiInitiatorAuthGetIterRequest().
		ExecuteUsing(c.zr)

	if err != nil {
		return []azgo.IscsiSecurityEntryInfoType{}, err
	} else if response.Result.NumRecords() == 0 {
		return []azgo.IscsiSecurityEntryInfoType{}, fmt.Errorf("no iscsi security entries found")
	} else if response.Result.AttributesListPtr == nil {
		return []azgo.IscsiSecurityEntryInfoType{}, fmt.Errorf("no iscsi security entries found")
	} else if response.Result.AttributesListPtr.IscsiSecurityEntryInfoPtr != nil {
		return response.Result.AttributesListPtr.IscsiSecurityEntryInfoPtr, nil
	}
	return []azgo.IscsiSecurityEntryInfoType{}, fmt.Errorf("no iscsi security entries found")
}

// IscsiInitiatorDeleteAuth deletes the authorization details for a single initiator
// equivalent to filer::> vserver iscsi security delete -vserver SVM -initiator-name iqn.1993-08.org.debian:01:9031309bbebd
func (c Client) IscsiInitiatorDeleteAuth(initiator string) (*azgo.IscsiInitiatorDeleteAuthResponse, error) {
	response, err := azgo.NewIscsiInitiatorDeleteAuthRequest().
		SetInitiator(initiator).
		ExecuteUsing(c.zr)
	return response, err
}

// IscsiInitiatorGetAuth returns the authorization details for a single initiator
// equivalent to filer::> vserver iscsi security show -vserver SVM -initiator-name iqn.1993-08.org.debian:01:9031309bbebd
//
//	or filer::> vserver iscsi security show -vserver SVM -initiator-name default
func (c Client) IscsiInitiatorGetAuth(initiator string) (*azgo.IscsiInitiatorGetAuthResponse, error) {
	response, err := azgo.NewIscsiInitiatorGetAuthRequest().
		SetInitiator(initiator).
		ExecuteUsing(c.zr)
	return response, err
}

// IscsiInitiatorGetDefaultAuth returns the authorization details for the default initiator
// equivalent to filer::> vserver iscsi security show -vserver SVM -initiator-name default
func (c Client) IscsiInitiatorGetDefaultAuth() (*azgo.IscsiInitiatorGetDefaultAuthResponse, error) {
	response, err := azgo.NewIscsiInitiatorGetDefaultAuthRequest().
		ExecuteUsing(c.zr)
	return response, err
}

// IscsiInitiatorGetIter returns the initiator details for all non-default initiators for the Client's SVM
// equivalent to filer::> vserver iscsi initiator show -vserver SVM
func (c Client) IscsiInitiatorGetIter() ([]azgo.IscsiInitiatorListEntryInfoType, error) {
	response, err := azgo.NewIscsiInitiatorGetIterRequest().
		ExecuteUsing(c.zr)

	if err != nil {
		return []azgo.IscsiInitiatorListEntryInfoType{}, err
	} else if response.Result.NumRecords() == 0 {
		return []azgo.IscsiInitiatorListEntryInfoType{}, fmt.Errorf("no iscsi initiator entries found")
	} else if response.Result.AttributesListPtr == nil {
		return []azgo.IscsiInitiatorListEntryInfoType{}, fmt.Errorf("no iscsi initiator entries found")
	} else if response.Result.AttributesListPtr.IscsiInitiatorListEntryInfoPtr != nil {
		return response.Result.AttributesListPtr.IscsiInitiatorListEntryInfoPtr, nil
	}
	return []azgo.IscsiInitiatorListEntryInfoType{}, fmt.Errorf("no iscsi initiator entries found")
}

// IscsiInitiatorModifyCHAPParams modifies the authorization details for a single initiator
//
//	equivalent to filer::> vserver iscsi security modify -vserver SVM -initiator-name iqn.1993-08.org.debian:01:9031309bbebd \
//	                         -user-name outboundUserName -outbound-user-name outboundPassphrase
func (c Client) IscsiInitiatorModifyCHAPParams(
	initiator, userName, passphrase, outboundUserName, outboundPassphrase string,
) (*azgo.IscsiInitiatorModifyChapParamsResponse, error) {
	request := azgo.NewIscsiInitiatorModifyChapParamsRequest().
		SetInitiator(initiator).
		SetUserName(userName).
		SetPassphrase(passphrase)
	if outboundUserName != "" && outboundPassphrase != "" {
		request.SetOutboundUserName(outboundUserName)
		request.SetOutboundPassphrase(outboundPassphrase)
	}
	response, err := request.ExecuteUsing(c.zr)
	return response, err
}

// IscsiInitiatorSetDefaultAuth sets the authorization details for the default initiator
//
//	equivalent to filer::> vserver iscsi security modify -vserver SVM -initiator-name default \
//	                          -auth-type CHAP -user-name outboundUserName -outbound-user-name outboundPassphrase
func (c Client) IscsiInitiatorSetDefaultAuth(
	authType, userName, passphrase, outboundUserName, outboundPassphrase string,
) (*azgo.IscsiInitiatorSetDefaultAuthResponse, error) {
	request := azgo.NewIscsiInitiatorSetDefaultAuthRequest().
		SetAuthType(authType).
		SetUserName(userName).
		SetPassphrase(passphrase)
	if outboundUserName != "" && outboundPassphrase != "" {
		request.SetOutboundUserName(outboundUserName)
		request.SetOutboundPassphrase(outboundPassphrase)
	}
	response, err := request.ExecuteUsing(c.zr)
	return response, err
}

// iSCSI initiator operations END

// SMBShareCreate creates an SMB share with the specified name and path.
// Equivalent to filer::> vserver cifs share create -share-name <shareName> -path <path>
func (c Client) SMBShareCreate(shareName, path string) (*azgo.CifsShareCreateResponse, error) {
	response, err := azgo.NewCifsShareCreateRequest().
		SetShareName(shareName).
		SetPath(path).
		ExecuteUsing(c.zr)

	return response, err
}

// getSMBShareByName gets an SMB share with the given name.
func (c Client) getSMBShareByName(shareName string) (*azgo.CifsShareType, error) {
	query := &azgo.CifsShareGetIterRequestQuery{}
	shareAttrs := azgo.NewCifsShareType().
		SetShareName(shareName)
	query.SetCifsShare(*shareAttrs)

	response, err := azgo.NewCifsShareGetIterRequest().
		SetMaxRecords(c.config.ContextBasedZapiRecords).
		SetQuery(*query).
		ExecuteUsing(c.zr)

	if err != nil {
		return nil, err
	} else if response.Result.NumRecords() == 0 {
		return nil, nil
	} else if response.Result.AttributesListPtr != nil &&
		response.Result.AttributesListPtr.CifsSharePtr != nil {
		return &response.Result.AttributesListPtr.CifsSharePtr[0], nil
	}

	return nil, fmt.Errorf("SMB share %s not found", shareName)
}

// SMBShareExists checks for the existence of an SMB share with the given name.
// Equivalent to filer::> cifs share show <shareName>
func (c Client) SMBShareExists(shareName string) (bool, error) {
	share, err := c.getSMBShareByName(shareName)
	if err != nil {
		return false, err
	}
	if share == nil {
		return false, err
	}
	return true, nil
}

// SMBShareDestroy destroys an SMB share
// Equivalent to filer::> cifs share delete shareName
func (c Client) SMBShareDestroy(shareName string) (*azgo.CifsShareDeleteResponse, error) {
	response, err := azgo.NewCifsShareDeleteRequest().
		SetShareName(shareName).
		ExecuteUsing(c.zr)

	return response, err
}

// ///////////////////////////////////////////////////////////////////////////
