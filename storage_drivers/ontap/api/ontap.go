// Copyright 2020 NetApp, Inc. All Rights Reserved.

package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/utils"
)

const (
	defaultZapiRecords   = 100
	maxZapiRecords       = 0xfffffffe
	NumericalValueNotSet = -1
	maxFlexGroupWait     = 30 * time.Second

	failureLUNCreate  = "failure_65dc2f4b_adbe_4ed3_8b73_6c61d5eac054"
	failureLUNSetAttr = "failure_7c3a89e2_7d83_457b_9e29_bfdb082c1d8b"

	MaxNASLabelLength = 1023
)

// ClientConfig holds the configuration data for Client objects
type ClientConfig struct {
	ManagementLIF           string
	SVM                     string
	Username                string
	Password                string
	DriverContext           tridentconfig.DriverContext
	ContextBasedZapiRecords int
	DebugTraceFlags         map[string]bool
}

// Client is the object to use for interacting with ONTAP controllers
type Client struct {
	config  ClientConfig
	zr      *azgo.ZapiRunner
	m       *sync.Mutex
	SVMUUID string
}

// NewClient is a factory method for creating a new instance
func NewClient(config ClientConfig) *Client {

	// When running in Docker context we want to request MAX number of records from ZAPI for Volume, LUNs and Qtrees
	config.ContextBasedZapiRecords = defaultZapiRecords
	if config.DriverContext == tridentconfig.ContextDocker {
		config.ContextBasedZapiRecords = maxZapiRecords
	}

	d := &Client{
		config: config,
		zr: &azgo.ZapiRunner{
			ManagementLIF:   config.ManagementLIF,
			SVM:             config.SVM,
			Username:        config.Username,
			Password:        config.Password,
			Secure:          true,
			DebugTraceFlags: config.DebugTraceFlags,
		},
		m: &sync.Mutex{},
	}
	return d
}

// GetClonedZapiRunner returns a clone of the ZapiRunner configured on this driver.
func (d Client) GetClonedZapiRunner() *azgo.ZapiRunner {
	clone := new(azgo.ZapiRunner)
	*clone = *d.zr
	return clone
}

// GetNontunneledZapiRunner returns a clone of the ZapiRunner configured on this driver with the SVM field cleared so ZAPI calls
// made with the resulting runner aren't tunneled.  Note that the calls could still go directly to either a cluster or
// vserver management LIF.
func (d Client) GetNontunneledZapiRunner() *azgo.ZapiRunner {
	clone := new(azgo.ZapiRunner)
	*clone = *d.zr
	clone.SVM = ""
	return clone
}

// NewZapiError accepts the Response value from any AZGO call, extracts the status, reason, and errno values,
// and returns a ZapiError.  The interface passed in may either be a Response object, or the always-embedded
// Result object where the error info exists.
func NewZapiError(zapiResult interface{}) (err ZapiError) {
	defer func() {
		if r := recover(); r != nil {
			err = ZapiError{}
		}
	}()

	if zapiResult != nil {
		val := NewZapiResultValue(zapiResult)
		if reflect.TypeOf(zapiResult).Kind() == reflect.Ptr {
			val = reflect.Indirect(val)
		}

		err = ZapiError{
			val.FieldByName("ResultStatusAttr").String(),
			val.FieldByName("ResultReasonAttr").String(),
			val.FieldByName("ResultErrnoAttr").String(),
		}
	} else {
		err = ZapiError{}
		err.code = azgo.EINTERNALERROR
		err.reason = "unexpected nil ZAPI result"
		err.status = "failed"
	}

	return err
}

// NewZapiAsyncResult accepts the Response value from any AZGO Async Request, extracts the status, jobId, and
// errorCode values and returns a ZapiAsyncResult.
func NewZapiAsyncResult(ctx context.Context, zapiResult interface{}) (result ZapiAsyncResult, err error) {

	defer func() {
		if r := recover(); r != nil {
			err = ZapiError{}
		}
	}()

	var jobId int64
	var status string
	var errorCode int64

	val := NewZapiResultValue(zapiResult)
	if reflect.TypeOf(zapiResult).Kind() == reflect.Ptr {
		val = reflect.Indirect(val)
	}

	switch obj := zapiResult.(type) {
	case azgo.VolumeModifyIterAsyncResponse:
		Logc(ctx).Debugf("NewZapiAsyncResult - processing VolumeModifyIterAsyncResponse: %v", obj)
		// Handle ZAPI result for response object that contains a list of one item with the needed job information.
		volModifyResult := val.Interface().(azgo.VolumeModifyIterAsyncResponseResult)
		if volModifyResult.NumSucceededPtr != nil && *volModifyResult.NumSucceededPtr > 0 {
			if volModifyResult.SuccessListPtr != nil && volModifyResult.SuccessListPtr.VolumeModifyIterAsyncInfoPtr != nil {
				volInfoType := volModifyResult.SuccessListPtr.VolumeModifyIterAsyncInfoPtr[0]
				if volInfoType.JobidPtr != nil {
					jobId = int64(*volInfoType.JobidPtr)
				}
				if volInfoType.StatusPtr != nil {
					status = *volInfoType.StatusPtr
				}
				if volInfoType.ErrorCodePtr != nil {
					errorCode = int64(*volInfoType.ErrorCodePtr)
				}
			}
		}
	default:
		if s := val.FieldByName("ResultStatusPtr"); !s.IsNil() {
			status = s.Elem().String()
		}
		if j := val.FieldByName("ResultJobidPtr"); !j.IsNil() {
			jobId = j.Elem().Int()
		}
		if e := val.FieldByName("ResultErrorCodePtr"); !e.IsNil() {
			errorCode = e.Elem().Int()
		}
	}

	result = ZapiAsyncResult{
		int(jobId),
		status,
		int(errorCode),
	}

	return result, err
}

// NewZapiResultValue obtains the Result from an AZGO Response object and returns the Result
func NewZapiResultValue(zapiResult interface{}) reflect.Value {
	// A ZAPI Result struct works as-is, but a ZAPI Response struct must have its
	// embedded Result struct extracted via reflection.
	val := reflect.ValueOf(zapiResult)
	if reflect.TypeOf(zapiResult).Kind() == reflect.Ptr {
		val = reflect.Indirect(val)
	}
	if testResult := val.FieldByName("Result"); testResult.IsValid() {
		zapiResult = testResult.Interface()
		val = reflect.ValueOf(zapiResult)
	}
	return val
}

// ZapiAsyncResult encap
type ZapiAsyncResult struct {
	jobId     int
	status    string
	errorCode int
}

// ZapiError encapsulates the status, reason, and errno values from a ZAPI invocation, and it provides helper methods for detecting
// common error conditions.
type ZapiError struct {
	status string
	reason string
	code   string
}

func (e ZapiError) IsPassed() bool {
	return e.status == "passed"
}
func (e ZapiError) Error() string {
	if e.IsPassed() {
		return "API status: passed"
	}
	return fmt.Sprintf("API status: %s, Reason: %s, Code: %s", e.status, e.reason, e.code)
}
func (e ZapiError) IsPrivilegeError() bool {
	return e.code == azgo.EAPIPRIVILEGE
}
func (e ZapiError) IsScopeError() bool {
	return e.code == azgo.EAPIPRIVILEGE || e.code == azgo.EAPINOTFOUND
}
func (e ZapiError) IsFailedToLoadJobError() bool {
	return e.code == azgo.EINTERNALERROR && strings.Contains(e.reason, "Failed to load job")
}
func (e ZapiError) Status() string {
	return e.status
}
func (e ZapiError) Reason() string {
	return e.reason
}
func (e ZapiError) Code() string {
	return e.code
}

// GetError accepts both an error and the Response value from an AZGO invocation.
// If error is non-nil, it is returned as is.  Otherwise, the Response value is
// probed for an error returned by ZAPI; if one is found, a ZapiError error object
// is returned.  If no failures are detected, the method returns nil.  The interface
// passed in may either be a Response object, or the always-embedded Result object
// where the error info exists.
func GetError(ctx context.Context, zapiResult interface{}, errorIn error) (errorOut error) {

	defer func() {
		if r := recover(); r != nil {
			Logc(ctx).Errorf("Panic in ontap#GetError. %v\nStack Trace: %v", zapiResult, string(debug.Stack()))
			errorOut = ZapiError{}
		}
	}()

	// A ZAPI Result struct works as-is, but a ZAPI Response struct must have its
	// embedded Result struct extracted via reflection.
	if zapiResult != nil {
		val := reflect.ValueOf(zapiResult)
		if reflect.TypeOf(zapiResult).Kind() == reflect.Ptr {
			val = reflect.Indirect(val)
			if val.IsValid() {
				if testResult := val.FieldByName("Result"); testResult.IsValid() {
					zapiResult = testResult.Interface()
				}
			}
		}
	}

	errorOut = nil

	if errorIn != nil {
		errorOut = errorIn
	} else if zerr := NewZapiError(zapiResult); !zerr.IsPassed() {
		errorOut = zerr
	}

	return
}

/////////////////////////////////////////////////////////////////////////////
// API feature operations BEGIN

// API functions are named in a NounVerb pattern. This reflects how the azgo
// functions are also named. (i.e. VolumeGet instead of GetVolume)

type feature string

// Define new version-specific feature constants here
const (
	MinimumONTAPIVersion      feature = "MINIMUM_ONTAPI_VERSION"
	NetAppFlexGroups          feature = "NETAPP_FLEXGROUPS"
	NetAppFlexGroupsClone     feature = "NETAPP_FLEXGROUPS_CLONE_ONTAPI_MINIMUM"
	NetAppFabricPoolFlexVol   feature = "NETAPP_FABRICPOOL_FLEXVOL"
	NetAppFabricPoolFlexGroup feature = "NETAPP_FABRICPOOL_FLEXGROUP"
	LunGeometrySkip           feature = "LUN_GEOMETRY_SKIP"
	FabricPoolForSVMDR        feature = "FABRICPOOL_FOR_SVMDR"
)

// Indicate the minimum Ontapi version for each feature here
var features = map[feature]*utils.Version{
	MinimumONTAPIVersion:      utils.MustParseSemantic("1.110.0"), // cDOT 9.1.0
	NetAppFlexGroups:          utils.MustParseSemantic("1.120.0"), // cDOT 9.2.0
	NetAppFlexGroupsClone:     utils.MustParseSemantic("1.170.0"), // cDOT 9.7.0
	NetAppFabricPoolFlexVol:   utils.MustParseSemantic("1.120.0"), // cDOT 9.2.0
	NetAppFabricPoolFlexGroup: utils.MustParseSemantic("1.150.0"), // cDOT 9.5.0
	LunGeometrySkip:           utils.MustParseSemantic("1.150.0"), // cDOT 9.5.0
	FabricPoolForSVMDR:        utils.MustParseSemantic("1.150.0"), // cDOT 9.5.0
}

// SupportsFeature returns true if the Ontapi version supports the supplied feature
func (d Client) SupportsFeature(ctx context.Context, feature feature) bool {

	ontapiVersion, err := d.SystemGetOntapiVersion(ctx)
	if err != nil {
		return false
	}

	ontapiSemVer, err := utils.ParseSemantic(fmt.Sprintf("%s.0", ontapiVersion))
	if err != nil {
		return false
	}

	if minVersion, ok := features[feature]; ok {
		return ontapiSemVer.AtLeast(minVersion)
	} else {
		return false
	}
}

// API feature operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// IGROUP operations BEGIN

// IgroupCreate creates the specified initiator group
// equivalent to filer::> igroup create docker -vserver iscsi_vs -protocol iscsi -ostype linux
func (d Client) IgroupCreate(initiatorGroupName, initiatorGroupType, osType string) (*azgo.IgroupCreateResponse, error) {
	response, err := azgo.NewIgroupCreateRequest().
		SetInitiatorGroupName(initiatorGroupName).
		SetInitiatorGroupType(initiatorGroupType).
		SetOsType(osType).
		ExecuteUsing(d.zr)
	return response, err
}

// IgroupAdd adds an initiator to an initiator group
// equivalent to filer::> igroup add -vserver iscsi_vs -igroup docker -initiator iqn.1993-08.org.debian:01:9031309bbebd
func (d Client) IgroupAdd(initiatorGroupName, initiator string) (*azgo.IgroupAddResponse, error) {
	response, err := azgo.NewIgroupAddRequest().
		SetInitiatorGroupName(initiatorGroupName).
		SetInitiator(initiator).
		ExecuteUsing(d.zr)
	return response, err
}

// IgroupRemove removes an initiator from an initiator group
func (d Client) IgroupRemove(initiatorGroupName, initiator string, force bool) (*azgo.IgroupRemoveResponse, error) {
	response, err := azgo.NewIgroupRemoveRequest().
		SetInitiatorGroupName(initiatorGroupName).
		SetInitiator(initiator).
		SetForce(force).
		ExecuteUsing(d.zr)
	return response, err
}

// IgroupDestroy destroys an initiator group
func (d Client) IgroupDestroy(initiatorGroupName string) (*azgo.IgroupDestroyResponse, error) {
	response, err := azgo.NewIgroupDestroyRequest().
		SetInitiatorGroupName(initiatorGroupName).
		ExecuteUsing(d.zr)
	return response, err
}

// IgroupList lists initiator groups
func (d Client) IgroupList() (*azgo.IgroupGetIterResponse, error) {
	response, err := azgo.NewIgroupGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		ExecuteUsing(d.zr)
	return response, err
}

//IgroupGet gets a specified initiator group
func (d Client) IgroupGet(initiatorGroupName string) (*azgo.InitiatorGroupInfoType, error) {
	query := &azgo.IgroupGetIterRequestQuery{}
	iGroupInfo := azgo.NewInitiatorGroupInfoType().
		SetInitiatorGroupName(initiatorGroupName)
	query.SetInitiatorGroupInfo(*iGroupInfo)

	response, err := azgo.NewIgroupGetIterRequest().
		SetQuery(*query).
		ExecuteUsing(d.zr)
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
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// LUN operations BEGIN

// LunCreate creates a lun with the specified attributes
// equivalent to filer::> lun create -vserver iscsi_vs -path /vol/v/lun1 -size 1g -ostype linux -space-reserve disabled -space-allocation enabled
func (d Client) LunCreate(
	lunPath string, sizeInBytes int, osType string, spaceReserved bool, spaceAllocated bool,
) (*azgo.LunCreateBySizeResponse, error) {

	if strings.Contains(lunPath, failureLUNCreate) {
		return nil, errors.New("injected error")
	}

	response, err := azgo.NewLunCreateBySizeRequest().
		SetPath(lunPath).
		SetSize(sizeInBytes).
		SetOstype(osType).
		SetSpaceReservationEnabled(spaceReserved).
		SetSpaceAllocationEnabled(spaceAllocated).
		ExecuteUsing(d.zr)
	return response, err
}

// LunCloneCreate clones a LUN from a snapshot
func (d Client) LunCloneCreate(volumeName, sourceLun, destinationLun string) (*azgo.CloneCreateResponse, error) {
	response, err := azgo.NewCloneCreateRequest().
		SetVolume(volumeName).
		SetSourcePath(sourceLun).
		SetDestinationPath(destinationLun).
		ExecuteUsing(d.zr)
	return response, err
}

// LunGetSerialNumber returns the serial# for a lun
func (d Client) LunGetSerialNumber(lunPath string) (*azgo.LunGetSerialNumberResponse, error) {
	response, err := azgo.NewLunGetSerialNumberRequest().
		SetPath(lunPath).
		ExecuteUsing(d.zr)
	return response, err
}

// LunMapGet returns a list of LUN map details
// equivalent to filer::> lun mapping show -vserver iscsi_vs -path /vol/v/lun0 -igroup trident
func (d Client) LunMapGet(initiatorGroupName, lunPath string) (*azgo.LunMapGetIterResponse, error) {

	lunMapInfo := *azgo.NewLunMapInfoType().
		SetInitiatorGroup(initiatorGroupName).
		SetPath(lunPath)

	response, err := azgo.NewLunMapGetIterRequest().
		SetQuery(lunMapInfo).
		ExecuteUsing(d.zr)
	return &response, err
}

// LunMap maps a lun to an id in an initiator group
// equivalent to filer::> lun map -vserver iscsi_vs -path /vol/v/lun1 -igroup docker -lun-id 0
func (d Client) LunMap(initiatorGroupName, lunPath string, lunID int) (*azgo.LunMapResponse, error) {
	response, err := azgo.NewLunMapRequest().
		SetInitiatorGroup(initiatorGroupName).
		SetPath(lunPath).
		SetLunId(lunID).
		ExecuteUsing(d.zr)
	return response, err
}

// LunMapAutoID maps a LUN in an initiator group, allowing ONTAP to choose an available LUN ID
// equivalent to filer::> lun map -vserver iscsi_vs -path /vol/v/lun1 -igroup docker
func (d Client) LunMapAutoID(initiatorGroupName, lunPath string) (*azgo.LunMapResponse, error) {
	response, err := azgo.NewLunMapRequest().
		SetInitiatorGroup(initiatorGroupName).
		SetPath(lunPath).
		ExecuteUsing(d.zr)
	return response, err
}

func (d Client) LunMapIfNotMapped(
	ctx context.Context, initiatorGroupName, lunPath string, importNotManaged bool,
) (int, error) {

	// Read LUN maps to see if the LUN is already mapped to the igroup
	lunMapListResponse, err := d.LunMapListInfo(lunPath)
	if err != nil {
		return -1, fmt.Errorf("problem reading maps for LUN %s: %v", lunPath, err)
	} else if lunMapListResponse.Result.ResultStatusAttr != "passed" {
		return -1, fmt.Errorf("problem reading maps for LUN %s: %+v", lunPath, lunMapListResponse.Result)
	}

	lunID := 0
	alreadyMapped := false
	if lunMapListResponse.Result.InitiatorGroupsPtr != nil {
		for _, igroup := range lunMapListResponse.Result.InitiatorGroupsPtr.InitiatorGroupInfoPtr {
			if igroup.InitiatorGroupName() != initiatorGroupName && !importNotManaged {
				Logc(ctx).Debugf("deleting existing LUN mapping")
				lunUnmapResponse, err := d.LunUnmap(igroup.InitiatorGroupName(), lunPath)
				if err != nil {
					return -1, fmt.Errorf("problem deleting map for LUN %s: %+v", lunPath, lunUnmapResponse.Result)
				}
			}
			if igroup.InitiatorGroupName() == initiatorGroupName || importNotManaged {

				lunID = igroup.LunId()
				alreadyMapped = true

				Logc(ctx).WithFields(log.Fields{
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
		lunMapResponse, err := d.LunMapAutoID(initiatorGroupName, lunPath)
		if err != nil {
			return -1, fmt.Errorf("problem mapping LUN %s: %v", lunPath, err)
		} else if lunMapResponse.Result.ResultStatusAttr != "passed" {
			return -1, fmt.Errorf("problem mapping LUN %s: %+v", lunPath, lunMapResponse.Result)
		}

		lunID = lunMapResponse.Result.LunIdAssigned()

		Logc(ctx).WithFields(log.Fields{
			"lun":    lunPath,
			"igroup": initiatorGroupName,
			"id":     lunID,
		}).Debug("LUN mapped.")
	}

	return lunID, nil
}

// LunMapListInfo returns lun mapping information for the specified lun
// equivalent to filer::> lun mapped show -vserver iscsi_vs -path /vol/v/lun0
func (d Client) LunMapListInfo(lunPath string) (*azgo.LunMapListInfoResponse, error) {
	response, err := azgo.NewLunMapListInfoRequest().
		SetPath(lunPath).
		ExecuteUsing(d.zr)
	return response, err
}

// LunOffline offlines a lun
// equivalent to filer::> lun offline -vserver iscsi_vs -path /vol/v/lun0
func (d Client) LunOffline(lunPath string) (*azgo.LunOfflineResponse, error) {
	response, err := azgo.NewLunOfflineRequest().
		SetPath(lunPath).
		ExecuteUsing(d.zr)
	return response, err
}

// LunOnline onlines a lun
// equivalent to filer::> lun online -vserver iscsi_vs -path /vol/v/lun0
func (d Client) LunOnline(lunPath string) (*azgo.LunOnlineResponse, error) {
	response, err := azgo.NewLunOnlineRequest().
		SetPath(lunPath).
		ExecuteUsing(d.zr)
	return response, err
}

// LunDestroy destroys a LUN
// equivalent to filer::> lun destroy -vserver iscsi_vs -path /vol/v/lun0
func (d Client) LunDestroy(lunPath string) (*azgo.LunDestroyResponse, error) {
	response, err := azgo.NewLunDestroyRequest().
		SetPath(lunPath).
		ExecuteUsing(d.zr)
	return response, err
}

// LunSetAttribute sets a named attribute for a given LUN.
func (d Client) LunSetAttribute(lunPath, name, value string) (*azgo.LunSetAttributeResponse, error) {

	if strings.Contains(lunPath, failureLUNSetAttr) {
		return nil, errors.New("injected error")
	}

	response, err := azgo.NewLunSetAttributeRequest().
		SetPath(lunPath).
		SetName(name).
		SetValue(value).
		ExecuteUsing(d.zr)
	return response, err
}

// LunGetAttribute gets a named attribute for a given LUN.
func (d Client) LunGetAttribute(lunPath, name string) (*azgo.LunGetAttributeResponse, error) {
	response, err := azgo.NewLunGetAttributeRequest().
		SetPath(lunPath).
		SetName(name).
		ExecuteUsing(d.zr)
	return response, err
}

// LunGet returns all relevant details for a single LUN
// equivalent to filer::> lun show
func (d Client) LunGet(path string) (*azgo.LunInfoType, error) {

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
		SetMapped(false)
	desiredAttributes.SetLunInfo(*lunInfo)

	response, err := azgo.NewLunGetIterRequest().
		SetMaxRecords(d.config.ContextBasedZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)

	if err != nil {
		return &azgo.LunInfoType{}, err
	} else if response.Result.NumRecords() == 0 {
		return &azgo.LunInfoType{}, fmt.Errorf("LUN %s not found", path)
	} else if response.Result.NumRecords() > 1 {
		return &azgo.LunInfoType{}, fmt.Errorf("more than one LUN %s found", path)
	} else if response.Result.AttributesListPtr == nil {
		return &azgo.LunInfoType{}, fmt.Errorf("LUN %s not found", path)
	} else if response.Result.AttributesListPtr.LunInfoPtr != nil {
		return &response.Result.AttributesListPtr.LunInfoPtr[0], nil
	}
	return &azgo.LunInfoType{}, fmt.Errorf("LUN %s not found", path)
}

func (d Client) lunGetAllCommon(query *azgo.LunGetIterRequestQuery) (*azgo.LunGetIterResponse, error) {
	// Limit the returned data to only the data relevant to containers
	desiredAttributes := &azgo.LunGetIterRequestDesiredAttributes{}
	lunInfo := azgo.NewLunInfoType().
		SetPath("").
		SetVolume("").
		SetSize(0).
		SetCreationTimestamp(0)
	desiredAttributes.SetLunInfo(*lunInfo)

	response, err := azgo.NewLunGetIterRequest().
		SetMaxRecords(d.config.ContextBasedZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)
	return response, err
}

func (d Client) LunGetGeometry(path string) (*azgo.LunGetGeometryResponse, error) {
	response, err := azgo.NewLunGetGeometryRequest().
		SetPath(path).
		ExecuteUsing(d.zr)
	return response, err
}

func (d Client) LunResize(path string, sizeBytes int) (uint64, error) {
	response, err := azgo.NewLunResizeRequest().
		SetPath(path).
		SetSize(sizeBytes).
		ExecuteUsing(d.zr)

	var errSize uint64 = 0
	if err != nil {
		return errSize, err
	}

	if zerr := NewZapiError(response); !zerr.IsPassed() {
		return errSize, zerr
	}

	result := NewZapiResultValue(response)
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
func (d Client) LunGetAll(pathPattern string) (*azgo.LunGetIterResponse, error) {

	// Limit LUNs to those matching the pathPattern; ex, "/vol/trident_*/*"
	query := &azgo.LunGetIterRequestQuery{}
	lunInfo := azgo.NewLunInfoType().
		SetPath(pathPattern)
	query.SetLunInfo(*lunInfo)

	return d.lunGetAllCommon(query)
}

// LunGetAllForVolume returns all relevant details for all LUNs in the supplied Volume
// equivalent to filer::> lun show -volume trident_CEwDWXQRPz
func (d Client) LunGetAllForVolume(volumeName string) (*azgo.LunGetIterResponse, error) {

	// Limit LUNs to those owned by the volumeName; ex, "trident_trident"
	query := &azgo.LunGetIterRequestQuery{}
	lunInfo := azgo.NewLunInfoType().
		SetVolume(volumeName)
	query.SetLunInfo(*lunInfo)

	return d.lunGetAllCommon(query)
}

// LunGetAllForVserver returns all relevant details for all LUNs in the supplied SVM
// equivalent to filer::> lun show -vserver trident_CEwDWXQRPz
func (d Client) LunGetAllForVserver(vserverName string) (*azgo.LunGetIterResponse, error) {

	// Limit LUNs to those owned by the SVM with the supplied vserverName
	query := &azgo.LunGetIterRequestQuery{}
	lunInfo := azgo.NewLunInfoType().
		SetVserver(vserverName)
	query.SetLunInfo(*lunInfo)

	return d.lunGetAllCommon(query)
}

// LunCount returns the number of LUNs that exist in a given volume
func (d Client) LunCount(ctx context.Context, volume string) (int, error) {

	// Limit the LUNs to those in the specified Flexvol
	query := &azgo.LunGetIterRequestQuery{}
	lunInfo := azgo.NewLunInfoType().SetVolume(volume)
	query.SetLunInfo(*lunInfo)

	// Limit the returned data to only the Flexvol and LUN names
	desiredAttributes := &azgo.LunGetIterRequestDesiredAttributes{}
	desiredInfo := azgo.NewLunInfoType().SetPath("").SetVolume("")
	desiredAttributes.SetLunInfo(*desiredInfo)

	response, err := azgo.NewLunGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)
	if err = GetError(ctx, response, err); err != nil {
		return 0, err
	}

	return response.Result.NumRecords(), nil
}

// LunRename changes the name of a LUN
func (d Client) LunRename(path, newPath string) (*azgo.LunMoveResponse, error) {
	response, err := azgo.NewLunMoveRequest().
		SetPath(path).
		SetNewPath(newPath).
		ExecuteUsing(d.zr)
	return response, err
}

// LunUnmap deletes the lun mapping for the given LUN path and igroup
// equivalent to filer::> lun mapping delete -vserver iscsi_vs -path /vol/v/lun0 -igroup group
func (d Client) LunUnmap(initiatorGroupName, lunPath string) (*azgo.LunUnmapResponse, error) {
	response, err := azgo.NewLunUnmapRequest().
		SetInitiatorGroup(initiatorGroupName).
		SetPath(lunPath).
		ExecuteUsing(d.zr)
	return response, err
}

// LUN operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// FlexGroup operations BEGIN

// FlexGroupCreate creates a FlexGroup with the specified options
// equivalent to filer::> volume create -vserver svm_name -volume fg_vol_name –auto-provision-as flexgroup -size fg_size  -state online -type RW -policy default -unix-permissions ---rwxr-xr-x -space-guarantee none -snapshot-policy none -security-style unix -encrypt false
func (d Client) FlexGroupCreate(
	ctx context.Context, name string, size int, aggrs []azgo.AggrNameType, spaceReserve, snapshotPolicy,
	unixPermissions, exportPolicy, securityStyle, tieringPolicy string, encrypt bool, snapshotReserve int,
) (*azgo.VolumeCreateAsyncResponse, error) {

	junctionPath := fmt.Sprintf("/%s", name)

	aggrList := azgo.VolumeCreateAsyncRequestAggrList{}
	aggrList.SetAggrName(aggrs)

	request := azgo.NewVolumeCreateAsyncRequest().
		SetVolumeName(name).
		SetSize(size).
		SetSnapshotPolicy(snapshotPolicy).
		SetSpaceReserve(spaceReserve).
		SetUnixPermissions(unixPermissions).
		SetExportPolicy(exportPolicy).
		SetVolumeSecurityStyle(securityStyle).
		SetEncrypt(encrypt).
		SetAggrList(aggrList).
		SetJunctionPath(junctionPath)

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

	if d.SupportsFeature(ctx, NetAppFabricPoolFlexGroup) {
		request.SetTieringPolicy(tieringPolicy)
	}

	response, err := request.ExecuteUsing(d.zr)
	if zerr := GetError(ctx, *response, err); zerr != nil {
		return response, zerr
	}

	err = d.WaitForAsyncResponse(ctx, *response, maxFlexGroupWait)
	if err != nil {
		return response, fmt.Errorf("error waiting for response: %v", err)
	}

	return response, err
}

// FlexGroupDestroy destroys a FlexGroup
func (d Client) FlexGroupDestroy(
	ctx context.Context, name string, force bool,
) (*azgo.VolumeDestroyAsyncResponse, error) {

	response, err := azgo.NewVolumeDestroyAsyncRequest().
		SetVolumeName(name).
		ExecuteUsing(d.zr)

	if zerr := NewZapiError(*response); !zerr.IsPassed() {
		// It's not an error if the volume no longer exists
		if zerr.Code() == azgo.EVOLUMEDOESNOTEXIST {
			Logc(ctx).WithField("volume", name).Warn("FlexGroup already deleted.")
			return response, nil
		}
	}

	if gerr := GetError(ctx, response, err); gerr != nil {
		return response, gerr
	}

	err = d.WaitForAsyncResponse(ctx, *response, maxFlexGroupWait)
	if err != nil {
		return response, fmt.Errorf("error waiting for response: %v", err)
	}

	return response, err
}

// FlexGroupExists tests for the existence of a FlexGroup
func (d Client) FlexGroupExists(ctx context.Context, name string) (bool, error) {
	response, err := azgo.NewVolumeSizeAsyncRequest().
		SetVolumeName(name).
		ExecuteUsing(d.zr)

	if zerr := NewZapiError(response); !zerr.IsPassed() {
		switch zerr.Code() {
		case azgo.EOBJECTNOTFOUND, azgo.EVOLUMEDOESNOTEXIST:
			return false, nil
		default:
			return false, zerr
		}
	}

	if gerr := GetError(ctx, response, err); gerr != nil {
		return false, gerr
	}

	// Wait for Async Job to complete
	err = d.WaitForAsyncResponse(ctx, response, maxFlexGroupWait)
	if err != nil {
		return false, fmt.Errorf("error waiting for response: %v", err)
	}

	return true, nil
}

// FlexGroupSize retrieves the size of the specified volume
func (d Client) FlexGroupSize(name string) (int, error) {
	volAttrs, err := d.FlexGroupGet(name)
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
func (d Client) FlexGroupSetSize(ctx context.Context, name, newSize string) (*azgo.VolumeSizeAsyncResponse, error) {
	response, err := azgo.NewVolumeSizeAsyncRequest().
		SetVolumeName(name).
		SetNewSize(newSize).
		ExecuteUsing(d.zr)

	if zerr := GetError(ctx, *response, err); zerr != nil {
		return response, zerr
	}

	err = d.WaitForAsyncResponse(ctx, *response, maxFlexGroupWait)
	if err != nil {
		return response, fmt.Errorf("error waiting for response: %v", err)
	}

	return response, err
}

// FlexGroupVolumeDisableSnapshotDirectoryAccess disables access to the ".snapshot" directory
// Disable '.snapshot' to allow official mysql container's chmod-in-init to work
func (d Client) FlexGroupVolumeDisableSnapshotDirectoryAccess(
	ctx context.Context, name string,
) (*azgo.VolumeModifyIterAsyncResponse, error) {

	volattr := &azgo.VolumeModifyIterAsyncRequestAttributes{}
	ssattr := azgo.NewVolumeSnapshotAttributesType().SetSnapdirAccessEnabled(false)
	volSnapshotAttrs := azgo.NewVolumeAttributesType().SetVolumeSnapshotAttributes(*ssattr)
	volattr.SetVolumeAttributes(*volSnapshotAttrs)

	queryattr := &azgo.VolumeModifyIterAsyncRequestQuery{}
	volidattr := azgo.NewVolumeIdAttributesType().SetName(name)
	volIdAttrs := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*volidattr)
	queryattr.SetVolumeAttributes(*volIdAttrs)

	response, err := azgo.NewVolumeModifyIterAsyncRequest().
		SetQuery(*queryattr).
		SetAttributes(*volattr).
		ExecuteUsing(d.zr)

	if zerr := GetError(ctx, response, err); zerr != nil {
		return response, zerr
	}

	err = d.WaitForAsyncResponse(ctx, *response, maxFlexGroupWait)
	if err != nil {
		return response, fmt.Errorf("error waiting for response: %v", err)
	}

	return response, err
}

func (d Client) FlexGroupModifyUnixPermissions(
	ctx context.Context, volumeName, unixPermissions string,
) (*azgo.VolumeModifyIterAsyncResponse, error) {

	volAttr := &azgo.VolumeModifyIterAsyncRequestAttributes{}
	volSecurityUnixAttrs := azgo.NewVolumeSecurityUnixAttributesType().SetPermissions(unixPermissions)
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
		ExecuteUsing(d.zr)

	if zerr := GetError(ctx, response, err); zerr != nil {
		return response, zerr
	}

	err = d.WaitForAsyncResponse(ctx, *response, maxFlexGroupWait)
	if err != nil {
		return response, fmt.Errorf("error waiting for response: %v", err)
	}

	return response, err
}

// FlexGroupGet returns all relevant details for a single FlexGroup
func (d Client) FlexGroupGet(name string) (*azgo.VolumeAttributesType, error) {
	// Limit the FlexGroups to the one matching the name
	queryVolIDAttrs := azgo.NewVolumeIdAttributesType().SetName(name)
	queryVolIDAttrs.SetStyleExtended("flexgroup")
	return d.volumeGetIterCommon(name, queryVolIDAttrs)
}

// FlexGroupGetAll returns all relevant details for all FlexGroups whose names match the supplied prefix
func (d Client) FlexGroupGetAll(prefix string) (*azgo.VolumeGetIterResponse, error) {
	// Limit the FlexGroups to those matching the name prefix
	queryVolIDAttrs := azgo.NewVolumeIdAttributesType().SetName(prefix + "*")
	queryVolStateAttrs := azgo.NewVolumeStateAttributesType().SetState("online")
	queryVolIDAttrs.SetStyleExtended("flexgroup")
	return d.volumeGetIterAll(prefix, queryVolIDAttrs, queryVolStateAttrs)
}

// WaitForAsyncResponse handles waiting for an AsyncResponse to return successfully or return an error.
func (d Client) WaitForAsyncResponse(ctx context.Context, zapiResult interface{}, maxWaitTime time.Duration) error {

	asyncResult, err := NewZapiAsyncResult(ctx, zapiResult)
	if err != nil {
		return err
	}

	// Possible values: "succeeded", "in_progress", "failed". Returns nil if succeeded
	if asyncResult.status == "in_progress" {
		// handle zapi response
		jobId := int(asyncResult.jobId)
		if asyncResponseError := d.checkForJobCompletion(ctx, jobId, maxWaitTime); asyncResponseError != nil {
			return asyncResponseError
		}
	} else if asyncResult.status == "failed" {
		return fmt.Errorf("result status is failed with errorCode %d", asyncResult.errorCode)
	}

	return nil
}

// checkForJobCompletion polls for the ONTAP job status success with backoff retry logic
func (d *Client) checkForJobCompletion(ctx context.Context, jobId int, maxWaitTime time.Duration) error {

	checkJobFinished := func() error {
		jobResponse, err := d.JobGetIterStatus(jobId)
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
		Logc(ctx).WithFields(log.Fields{
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
func (d Client) JobGetIterStatus(jobId int) (*azgo.JobGetIterResponse, error) {
	jobInfo := azgo.NewJobInfoType().SetJobId(jobId)
	queryAttr := &azgo.JobGetIterRequestQuery{}
	queryAttr.SetJobInfo(*jobInfo)

	response, err := azgo.NewJobGetIterRequest().
		SetQuery(*queryAttr).
		ExecuteUsing(d.GetNontunneledZapiRunner())
	return response, err
}

// FlexGroup operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// VOLUME operations BEGIN

// VolumeCreate creates a volume with the specified options
// equivalent to filer::> volume create -vserver iscsi_vs -volume v -aggregate aggr1 -size 1g -state online -type RW -policy default -unix-permissions ---rwxr-xr-x -space-guarantee none -snapshot-policy none -security-style unix -encrypt false
func (d Client) VolumeCreate(
	ctx context.Context, name, aggregateName, size, spaceReserve, snapshotPolicy, unixPermissions,
	exportPolicy, securityStyle, tieringPolicy, comment string, encrypt bool, snapshotReserve int,
) (*azgo.VolumeCreateResponse, error) {
	request := azgo.NewVolumeCreateRequest().
		SetVolume(name).
		SetContainingAggrName(aggregateName).
		SetSize(size).
		SetSpaceReserve(spaceReserve).
		SetSnapshotPolicy(snapshotPolicy).
		SetUnixPermissions(unixPermissions).
		SetExportPolicy(exportPolicy).
		SetVolumeSecurityStyle(securityStyle).
		SetEncrypt(encrypt).
		SetVolumeComment(comment)

	if snapshotReserve != NumericalValueNotSet {
		request.SetPercentageSnapshotReserve(snapshotReserve)
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
	//backup)/pass(fail)
	// ONTAP-NAS-ECO            other-values(backup)/pass(fail)     other-values(backup)/pass(fail)     other-values(
	//backup)/pass(fail)
	//
	// PLEASE NOTE:
	// 1. 'backup' tiering policy is for dp-volumes only.
	//

	if d.SupportsFeature(ctx, NetAppFabricPoolFlexVol) {
		request.SetTieringPolicy(tieringPolicy)
	}

	response, err := request.ExecuteUsing(d.zr)
	return response, err
}

func (d Client) VolumeModifyExportPolicy(volumeName, exportPolicyName string) (*azgo.VolumeModifyIterResponse, error) {
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
		ExecuteUsing(d.zr)
	return response, err
}

func (d Client) VolumeModifyUnixPermissions(volumeName, unixPermissions string) (*azgo.VolumeModifyIterResponse, error) {
	volAttr := &azgo.VolumeModifyIterRequestAttributes{}
	volSecurityUnixAttrs := azgo.NewVolumeSecurityUnixAttributesType().SetPermissions(unixPermissions)
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
		ExecuteUsing(d.zr)
	return response, err
}

// VolumeCloneCreate clones a volume from a snapshot
func (d Client) VolumeCloneCreate(name, source, snapshot string) (*azgo.VolumeCloneCreateResponse, error) {
	response, err := azgo.NewVolumeCloneCreateRequest().
		SetVolume(name).
		SetParentVolume(source).
		SetParentSnapshot(snapshot).
		ExecuteUsing(d.zr)
	return response, err
}

// VolumeCloneCreateAsync clones a volume from a snapshot
func (d Client) VolumeCloneCreateAsync(name, source, snapshot string) (*azgo.VolumeCloneCreateAsyncResponse, error) {
	response, err := azgo.NewVolumeCloneCreateAsyncRequest().
		SetVolume(name).
		SetParentVolume(source).
		SetParentSnapshot(snapshot).
		ExecuteUsing(d.zr)
	return response, err
}

// VolumeCloneSplitStart splits a cloned volume from its parent
func (d Client) VolumeCloneSplitStart(name string) (*azgo.VolumeCloneSplitStartResponse, error) {
	response, err := azgo.NewVolumeCloneSplitStartRequest().
		SetVolume(name).
		ExecuteUsing(d.zr)
	return response, err
}

// VolumeDisableSnapshotDirectoryAccess disables access to the ".snapshot" directory
// Disable '.snapshot' to allow official mysql container's chmod-in-init to work
func (d Client) VolumeDisableSnapshotDirectoryAccess(name string) (*azgo.VolumeModifyIterResponse, error) {
	volattr := &azgo.VolumeModifyIterRequestAttributes{}
	ssattr := azgo.NewVolumeSnapshotAttributesType().SetSnapdirAccessEnabled(false)
	volSnapshotAttrs := azgo.NewVolumeAttributesType().SetVolumeSnapshotAttributes(*ssattr)
	volattr.SetVolumeAttributes(*volSnapshotAttrs)

	queryattr := &azgo.VolumeModifyIterRequestQuery{}
	volidattr := azgo.NewVolumeIdAttributesType().SetName(azgo.VolumeNameType(name))
	volIdAttrs := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*volidattr)
	queryattr.SetVolumeAttributes(*volIdAttrs)

	response, err := azgo.NewVolumeModifyIterRequest().
		SetQuery(*queryattr).
		SetAttributes(*volattr).
		ExecuteUsing(d.zr)
	return response, err
}

// VolumeExists tests for the existence of a Flexvol
func (d Client) VolumeExists(ctx context.Context, name string) (bool, error) {
	response, err := azgo.NewVolumeSizeRequest().
		SetVolume(name).
		ExecuteUsing(d.zr)

	if err != nil {
		return false, err
	}

	if zerr := NewZapiError(response); !zerr.IsPassed() {
		switch zerr.Code() {
		case azgo.EOBJECTNOTFOUND, azgo.EVOLUMEDOESNOTEXIST:
			return false, nil
		default:
			return false, zerr
		}
	}

	return true, nil
}

// VolumeSize retrieves the size of the specified volume
func (d Client) VolumeSize(name string) (int, error) {

	volAttrs, err := d.VolumeGet(name)
	if err != nil {
		return 0, err
	}
	volSpaceAttrs := volAttrs.VolumeSpaceAttributes()

	return volSpaceAttrs.Size(), nil
}

// VolumeSetSize sets the size of the specified volume
func (d Client) VolumeSetSize(name, newSize string) (*azgo.VolumeSizeResponse, error) {
	response, err := azgo.NewVolumeSizeRequest().
		SetVolume(name).
		SetNewSize(newSize).
		ExecuteUsing(d.zr)
	return response, err
}

// VolumeMount mounts a volume at the specified junction
func (d Client) VolumeMount(name, junctionPath string) (*azgo.VolumeMountResponse, error) {
	response, err := azgo.NewVolumeMountRequest().
		SetVolumeName(name).
		SetJunctionPath(junctionPath).
		ExecuteUsing(d.zr)
	return response, err
}

// VolumeUnmount unmounts a volume from the specified junction
func (d Client) VolumeUnmount(name string, force bool) (*azgo.VolumeUnmountResponse, error) {
	response, err := azgo.NewVolumeUnmountRequest().
		SetVolumeName(name).
		SetForce(force).
		ExecuteUsing(d.zr)
	return response, err
}

// VolumeOffline offlines a volume
func (d Client) VolumeOffline(name string) (*azgo.VolumeOfflineResponse, error) {
	response, err := azgo.NewVolumeOfflineRequest().
		SetName(name).
		ExecuteUsing(d.zr)
	return response, err
}

// VolumeDestroy destroys a volume
func (d Client) VolumeDestroy(name string, force bool) (*azgo.VolumeDestroyResponse, error) {
	response, err := azgo.NewVolumeDestroyRequest().
		SetName(name).
		SetUnmountAndOffline(force).
		ExecuteUsing(d.zr)
	return response, err
}

// VolumeGet returns all relevant details for a single Flexvol
// equivalent to filer::> volume show
func (d Client) VolumeGet(name string) (*azgo.VolumeAttributesType, error) {

	// Limit the Flexvols to the one matching the name
	queryVolIDAttrs := azgo.NewVolumeIdAttributesType().
		SetName(azgo.VolumeNameType(name)).
		SetStyleExtended("flexvol")
	return d.volumeGetIterCommon(name, queryVolIDAttrs)
}

func (d Client) volumeGetIterCommon(name string,
	queryVolIDAttrs *azgo.VolumeIdAttributesType) (*azgo.VolumeAttributesType, error) {

	queryVolStateAttrs := azgo.NewVolumeStateAttributesType().SetState("online")

	query := &azgo.VolumeGetIterRequestQuery{}
	volAttrs := azgo.NewVolumeAttributesType().
		SetVolumeIdAttributes(*queryVolIDAttrs).
		SetVolumeStateAttributes(*queryVolStateAttrs)
	query.SetVolumeAttributes(*volAttrs)

	response, err := azgo.NewVolumeGetIterRequest().
		SetMaxRecords(d.config.ContextBasedZapiRecords).
		SetQuery(*query).
		ExecuteUsing(d.zr)

	if err != nil {
		return &azgo.VolumeAttributesType{}, err
	} else if response.Result.NumRecords() == 0 {
		return &azgo.VolumeAttributesType{}, fmt.Errorf("flexvol %s not found", name)
	} else if response.Result.NumRecords() > 1 {
		return &azgo.VolumeAttributesType{}, fmt.Errorf("more than one Flexvol %s found", name)
	} else if response.Result.AttributesListPtr == nil {
		return &azgo.VolumeAttributesType{}, fmt.Errorf("flexvol %s not found", name)
	} else if response.Result.AttributesListPtr.VolumeAttributesPtr != nil {
		return &response.Result.AttributesListPtr.VolumeAttributesPtr[0], nil
	}
	return &azgo.VolumeAttributesType{}, fmt.Errorf("flexvol %s not found", name)
}

// VolumeGetAll returns all relevant details for all FlexVols whose names match the supplied prefix
// equivalent to filer::> volume show
func (d Client) VolumeGetAll(prefix string) (response *azgo.VolumeGetIterResponse, err error) {

	// Limit the Flexvols to those matching the name prefix
	queryVolIDAttrs := azgo.NewVolumeIdAttributesType().
		SetName(azgo.VolumeNameType(prefix + "*")).
		SetStyleExtended("flexvol")
	queryVolStateAttrs := azgo.NewVolumeStateAttributesType().SetState("online")

	return d.volumeGetIterAll(prefix, queryVolIDAttrs, queryVolStateAttrs)
}

func (d Client) volumeGetIterAll(prefix string, queryVolIDAttrs *azgo.VolumeIdAttributesType,
	queryVolStateAttrs *azgo.VolumeStateAttributesType) (*azgo.VolumeGetIterResponse, error) {

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
		SetMaxRecords(d.config.ContextBasedZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)
	return response, err
}

// VolumeList returns the names of all Flexvols whose names match the supplied prefix
func (d Client) VolumeList(prefix string) (*azgo.VolumeGetIterResponse, error) {

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

	// Limit the returned data to only the Flexvol names
	desiredAttributes := &azgo.VolumeGetIterRequestDesiredAttributes{}
	desiredVolIDAttrs := azgo.NewVolumeIdAttributesType().SetName("").SetComment("")
	desiredVolumeAttributes := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*desiredVolIDAttrs)
	desiredAttributes.SetVolumeAttributes(*desiredVolumeAttributes)

	response, err := azgo.NewVolumeGetIterRequest().
		SetMaxRecords(d.config.ContextBasedZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)
	return response, err
}

// VolumeListByAttrs returns the names of all Flexvols matching the specified attributes
func (d Client) VolumeListByAttrs(
	prefix, aggregate, spaceReserve, snapshotPolicy, tieringPolicy string, snapshotDir bool, encrypt bool,
) (*azgo.VolumeGetIterResponse, error) {

	// Limit the Flexvols to those matching the specified attributes
	query := &azgo.VolumeGetIterRequestQuery{}
	queryVolIDAttrs := azgo.NewVolumeIdAttributesType().
		SetName(azgo.VolumeNameType(prefix + "*")).
		SetContainingAggregateName(aggregate).
		SetStyleExtended("flexvol")
	queryVolSpaceAttrs := azgo.NewVolumeSpaceAttributesType().
		SetSpaceGuarantee(spaceReserve)
	queryVolSnapshotAttrs := azgo.NewVolumeSnapshotAttributesType().
		SetSnapshotPolicy(snapshotPolicy).
		SetSnapdirAccessEnabled(snapshotDir)
	queryVolStateAttrs := azgo.NewVolumeStateAttributesType().
		SetState("online")
	queryVolCompAggrAttrs := azgo.NewVolumeCompAggrAttributesType().
		SetTieringPolicy(tieringPolicy)
	volumeAttributes := azgo.NewVolumeAttributesType().
		SetVolumeCompAggrAttributes(*queryVolCompAggrAttrs).
		SetVolumeIdAttributes(*queryVolIDAttrs).
		SetVolumeSpaceAttributes(*queryVolSpaceAttrs).
		SetVolumeSnapshotAttributes(*queryVolSnapshotAttrs).
		SetVolumeStateAttributes(*queryVolStateAttrs).
		SetEncrypt(encrypt)
	query.SetVolumeAttributes(*volumeAttributes)

	// Limit the returned data to only the Flexvol names
	desiredAttributes := &azgo.VolumeGetIterRequestDesiredAttributes{}
	desiredVolIDAttrs := azgo.NewVolumeIdAttributesType().SetName("").SetComment("")
	desiredVolumeAttributes := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*desiredVolIDAttrs)
	desiredAttributes.SetVolumeAttributes(*desiredVolumeAttributes)

	response, err := azgo.NewVolumeGetIterRequest().
		SetMaxRecords(d.config.ContextBasedZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)
	return response, err
}

// VolumeListAllBackedBySnapshot returns the names of all FlexVols backed by the specified snapshot
func (d Client) VolumeListAllBackedBySnapshot(ctx context.Context, volumeName, snapshotName string) ([]string, error) {

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
	desiredVolIDAttrs := azgo.NewVolumeIdAttributesType().SetName("").SetComment("")
	desiredVolumeAttributes := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*desiredVolIDAttrs)
	desiredAttributes.SetVolumeAttributes(*desiredVolumeAttributes)

	response, err := azgo.NewVolumeGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)

	if err = GetError(ctx, response, err); err != nil {
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
func (d Client) VolumeRename(volumeName, newVolumeName string) (*azgo.VolumeRenameResponse, error) {
	response, err := azgo.NewVolumeRenameRequest().
		SetVolume(volumeName).
		SetNewVolumeName(newVolumeName).
		ExecuteUsing(d.zr)
	return response, err
}

// VolumeSetComment sets a volume's comment to the supplied value
// equivalent to filer::> volume modify -vserver iscsi_vs -volume v -comment newVolumeComment
func (d Client) VolumeSetComment(ctx context.Context, volumeName, newVolumeComment string) (
	*azgo.VolumeModifyIterResponse, error) {

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
		ExecuteUsing(d.zr)
	return response, err
}

// VOLUME operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// QTREE operations BEGIN

// QtreeCreate creates a qtree with the specified options
// equivalent to filer::> qtree create -vserver ndvp_vs -volume v -qtree q -export-policy default -unix-permissions ---rwxr-xr-x -security-style unix
func (d Client) QtreeCreate(name, volumeName, unixPermissions, exportPolicy,
	securityStyle string) (*azgo.QtreeCreateResponse, error) {
	response, err := azgo.NewQtreeCreateRequest().
		SetQtree(name).
		SetVolume(volumeName).
		SetMode(unixPermissions).
		SetSecurityStyle(securityStyle).
		SetExportPolicy(exportPolicy).
		ExecuteUsing(d.zr)
	return response, err
}

// QtreeRename renames a qtree
// equivalent to filer::> volume qtree rename
func (d Client) QtreeRename(path, newPath string) (*azgo.QtreeRenameResponse, error) {
	response, err := azgo.NewQtreeRenameRequest().
		SetQtree(path).
		SetNewQtreeName(newPath).
		ExecuteUsing(d.zr)
	return response, err
}

// QtreeDestroyAsync destroys a qtree in the background
// equivalent to filer::> volume qtree delete -foreground false
func (d Client) QtreeDestroyAsync(path string, force bool) (*azgo.QtreeDeleteAsyncResponse, error) {
	response, err := azgo.NewQtreeDeleteAsyncRequest().
		SetQtree(path).
		SetForce(force).
		ExecuteUsing(d.zr)
	return response, err
}

// QtreeList returns the names of all Qtrees whose names match the supplied prefix
// equivalent to filer::> volume qtree show
func (d Client) QtreeList(prefix, volumePrefix string) (*azgo.QtreeListIterResponse, error) {

	// Limit the qtrees to those matching the Flexvol and Qtree name prefixes
	query := &azgo.QtreeListIterRequestQuery{}
	queryInfo := azgo.NewQtreeInfoType().SetVolume(volumePrefix + "*").SetQtree(prefix + "*")
	query.SetQtreeInfo(*queryInfo)

	// Limit the returned data to only the Flexvol and Qtree names
	desiredAttributes := &azgo.QtreeListIterRequestDesiredAttributes{}
	desiredInfo := azgo.NewQtreeInfoType().SetVolume("").SetQtree("")
	desiredAttributes.SetQtreeInfo(*desiredInfo)

	response, err := azgo.NewQtreeListIterRequest().
		SetMaxRecords(d.config.ContextBasedZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)
	return response, err
}

// QtreeCount returns the number of Qtrees in the specified Flexvol, not including the Flexvol itself
func (d Client) QtreeCount(ctx context.Context, volume string) (int, error) {

	// Limit the qtrees to those in the specified Flexvol
	query := &azgo.QtreeListIterRequestQuery{}
	queryInfo := azgo.NewQtreeInfoType().SetVolume(volume)
	query.SetQtreeInfo(*queryInfo)

	// Limit the returned data to only the Flexvol and Qtree names
	desiredAttributes := &azgo.QtreeListIterRequestDesiredAttributes{}
	desiredInfo := azgo.NewQtreeInfoType().SetVolume("").SetQtree("")
	desiredAttributes.SetQtreeInfo(*desiredInfo)

	response, err := azgo.NewQtreeListIterRequest().
		SetMaxRecords(d.config.ContextBasedZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)

	if err = GetError(ctx, response, err); err != nil {
		return 0, err
	}

	// There will always be one qtree for the Flexvol, so decrement by 1
	switch response.Result.NumRecords() {
	case 0:
		fallthrough
	case 1:
		return 0, nil
	default:
		return response.Result.NumRecords() - 1, nil
	}
}

// QtreeExists returns true if the named Qtree exists (and is unique in the matching Flexvols)
func (d Client) QtreeExists(ctx context.Context, name, volumePrefix string) (bool, string, error) {

	// Limit the qtrees to those matching the Flexvol and Qtree name prefixes
	query := &azgo.QtreeListIterRequestQuery{}
	queryInfo := azgo.NewQtreeInfoType().SetVolume(volumePrefix + "*").SetQtree(name)
	query.SetQtreeInfo(*queryInfo)

	// Limit the returned data to only the Flexvol and Qtree names
	desiredAttributes := &azgo.QtreeListIterRequestDesiredAttributes{}
	desiredInfo := azgo.NewQtreeInfoType().SetVolume("").SetQtree("")
	desiredAttributes.SetQtreeInfo(*desiredInfo)

	response, err := azgo.NewQtreeListIterRequest().
		SetMaxRecords(d.config.ContextBasedZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)

	// Ensure the API call succeeded
	if err = GetError(ctx, response, err); err != nil {
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
func (d Client) QtreeGet(name, volumePrefix string) (*azgo.QtreeInfoType, error) {

	// Limit the qtrees to those matching the Flexvol and Qtree name prefixes
	query := &azgo.QtreeListIterRequestQuery{}
	info := azgo.NewQtreeInfoType().SetVolume(volumePrefix + "*").SetQtree(name)
	query.SetQtreeInfo(*info)

	response, err := azgo.NewQtreeListIterRequest().
		SetMaxRecords(d.config.ContextBasedZapiRecords).
		SetQuery(*query).
		ExecuteUsing(d.zr)

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
func (d Client) QtreeGetAll(volumePrefix string) (*azgo.QtreeListIterResponse, error) {

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
		SetMaxRecords(d.config.ContextBasedZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)
	return response, err
}

func (d Client) QtreeModifyExportPolicy(name, volumeName, exportPolicy string) (*azgo.QtreeModifyResponse, error) {

	return azgo.NewQtreeModifyRequest().
		SetQtree(name).
		SetVolume(volumeName).
		SetExportPolicy(exportPolicy).
		ExecuteUsing(d.zr)
}

// QuotaOn enables quotas on a Flexvol
// equivalent to filer::> volume quota on
func (d Client) QuotaOn(volume string) (*azgo.QuotaOnResponse, error) {
	response, err := azgo.NewQuotaOnRequest().
		SetVolume(volume).
		ExecuteUsing(d.zr)
	return response, err
}

// QuotaOff disables quotas on a Flexvol
// equivalent to filer::> volume quota off
func (d Client) QuotaOff(volume string) (*azgo.QuotaOffResponse, error) {
	response, err := azgo.NewQuotaOffRequest().
		SetVolume(volume).
		ExecuteUsing(d.zr)
	return response, err
}

// QuotaResize resizes quotas on a Flexvol
// equivalent to filer::> volume quota resize
func (d Client) QuotaResize(volume string) (*azgo.QuotaResizeResponse, error) {
	response, err := azgo.NewQuotaResizeRequest().
		SetVolume(volume).
		ExecuteUsing(d.zr)
	return response, err
}

// QuotaStatus returns the quota status for a Flexvol
// equivalent to filer::> volume quota show
func (d Client) QuotaStatus(volume string) (*azgo.QuotaStatusResponse, error) {
	response, err := azgo.NewQuotaStatusRequest().
		SetVolume(volume).
		ExecuteUsing(d.zr)
	return response, err
}

// QuotaSetEntry creates a new quota rule with an optional hard disk limit
// equivalent to filer::> volume quota policy rule create
func (d Client) QuotaSetEntry(qtreeName, volumeName, quotaTarget, quotaType, diskLimit string) (*azgo.QuotaSetEntryResponse, error) {

	request := azgo.NewQuotaSetEntryRequest().
		SetQtree(qtreeName).
		SetVolume(volumeName).
		SetQuotaTarget(quotaTarget).
		SetQuotaType(quotaType)

	// To create a default quota rule, pass an empty disk limit
	if diskLimit != "" {
		request.SetDiskLimit(diskLimit)
	}

	response, err := request.ExecuteUsing(d.zr)
	return response, err
}

// QuotaEntryGet returns the disk limit for a single qtree
// equivalent to filer::> volume quota policy rule show
func (d Client) QuotaGetEntry(target string) (*azgo.QuotaEntryType, error) {

	query := &azgo.QuotaListEntriesIterRequestQuery{}
	quotaEntry := azgo.NewQuotaEntryType().SetQuotaType("tree").SetQuotaTarget(target)
	query.SetQuotaEntry(*quotaEntry)

	// Limit the returned data to only the disk limit
	desiredAttributes := &azgo.QuotaListEntriesIterRequestDesiredAttributes{}
	desiredQuotaEntryFields := azgo.NewQuotaEntryType().SetDiskLimit("").SetQuotaTarget("")
	desiredAttributes.SetQuotaEntry(*desiredQuotaEntryFields)

	response, err := azgo.NewQuotaListEntriesIterRequest().
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)

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
func (d Client) QuotaEntryList(volume string) (*azgo.QuotaListEntriesIterResponse, error) {
	query := &azgo.QuotaListEntriesIterRequestQuery{}
	quotaEntry := azgo.NewQuotaEntryType().SetVolume(volume).SetQuotaType("tree")
	query.SetQuotaEntry(*quotaEntry)

	// Limit the returned data to only the disk limit
	desiredAttributes := &azgo.QuotaListEntriesIterRequestDesiredAttributes{}
	desiredQuotaEntryFields := azgo.NewQuotaEntryType().SetDiskLimit("").SetQuotaTarget("")
	desiredAttributes.SetQuotaEntry(*desiredQuotaEntryFields)

	response, err := azgo.NewQuotaListEntriesIterRequest().
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)
	return response, err
}

// QTREE operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// EXPORT POLICY operations BEGIN

// ExportPolicyCreate creates an export policy
// equivalent to filer::> vserver export-policy create
func (d Client) ExportPolicyCreate(policy string) (*azgo.ExportPolicyCreateResponse, error) {
	response, err := azgo.NewExportPolicyCreateRequest().
		SetPolicyName(policy).
		ExecuteUsing(d.zr)
	return response, err
}

func (d Client) ExportPolicyGet(policy string) (*azgo.ExportPolicyGetResponse, error) {
	return azgo.NewExportPolicyGetRequest().
		SetPolicyName(policy).
		ExecuteUsing(d.zr)
}

func (d Client) ExportPolicyDestroy(policy string) (*azgo.ExportPolicyDestroyResponse, error) {
	return azgo.NewExportPolicyDestroyRequest().
		SetPolicyName(policy).
		ExecuteUsing(d.zr)
}

// ExportRuleCreate creates a rule in an export policy
// equivalent to filer::> vserver export-policy rule create
func (d Client) ExportRuleCreate(
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
		ExecuteUsing(d.zr)
	return response, err
}

// ExportRuleGetIterRequest returns the export rules in an export policy
// equivalent to filer::> vserver export-policy rule show
func (d Client) ExportRuleGetIterRequest(policy string) (*azgo.ExportRuleGetIterResponse, error) {

	// Limit the qtrees to those matching the Flexvol and Qtree name prefixes
	query := &azgo.ExportRuleGetIterRequestQuery{}
	exportRuleInfo := azgo.NewExportRuleInfoType().SetPolicyName(azgo.ExportPolicyNameType(policy))
	query.SetExportRuleInfo(*exportRuleInfo)

	response, err := azgo.NewExportRuleGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		ExecuteUsing(d.zr)
	return response, err
}

// ExportRuleDestroy deletes the rule at the given index in the given policy
func (d Client) ExportRuleDestroy(policy string, ruleIndex int) (*azgo.ExportRuleDestroyResponse, error) {
	response, err := azgo.NewExportRuleDestroyRequest().
		SetPolicyName(policy).
		SetRuleIndex(ruleIndex).
		ExecuteUsing(d.zr)
	return response, err
}

// EXPORT POLICY operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// SNAPSHOT operations BEGIN

// SnapshotCreate creates a snapshot of a volume
func (d Client) SnapshotCreate(snapshotName, volumeName string) (*azgo.SnapshotCreateResponse, error) {
	response, err := azgo.NewSnapshotCreateRequest().
		SetSnapshot(snapshotName).
		SetVolume(volumeName).
		ExecuteUsing(d.zr)
	return response, err
}

// SnapshotList returns the list of snapshots associated with a volume
func (d Client) SnapshotList(volumeName string) (*azgo.SnapshotGetIterResponse, error) {
	query := &azgo.SnapshotGetIterRequestQuery{}
	snapshotInfo := azgo.NewSnapshotInfoType().SetVolume(volumeName)
	query.SetSnapshotInfo(*snapshotInfo)

	response, err := azgo.NewSnapshotGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		ExecuteUsing(d.zr)
	return response, err
}

// SnapshotRestoreVolume restores a volume to a snapshot as a non-blocking operation
func (d Client) SnapshotRestoreVolume(snapshotName, volumeName string) (*azgo.SnapshotRestoreVolumeResponse, error) {
	response, err := azgo.NewSnapshotRestoreVolumeRequest().
		SetVolume(volumeName).
		SetSnapshot(snapshotName).
		SetPreserveLunIds(true).
		ExecuteUsing(d.zr)
	return response, err
}

// DeleteSnapshot deletes a snapshot of a volume
func (d Client) SnapshotDelete(snapshotName, volumeName string) (*azgo.SnapshotDeleteResponse, error) {
	response, err := azgo.NewSnapshotDeleteRequest().
		SetVolume(volumeName).
		SetSnapshot(snapshotName).
		SetIgnoreOwners(true).
		ExecuteUsing(d.zr)
	return response, err
}

// SNAPSHOT operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// ISCSI operations BEGIN

// IscsiServiceGetIterRequest returns information about an iSCSI target
func (d Client) IscsiServiceGetIterRequest() (*azgo.IscsiServiceGetIterResponse, error) {
	response, err := azgo.NewIscsiServiceGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		ExecuteUsing(d.zr)
	return response, err
}

// IscsiNodeGetNameRequest gets the IQN of the vserver
func (d Client) IscsiNodeGetNameRequest() (*azgo.IscsiNodeGetNameResponse, error) {
	response, err := azgo.NewIscsiNodeGetNameRequest().ExecuteUsing(d.zr)
	return response, err
}

// IscsiInterfaceGetIterRequest returns information about the vserver's iSCSI interfaces
func (d Client) IscsiInterfaceGetIterRequest() (*azgo.IscsiInterfaceGetIterResponse, error) {
	response, err := azgo.NewIscsiInterfaceGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		ExecuteUsing(d.zr)
	return response, err
}

// ISCSI operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// VSERVER operations BEGIN

// VserverGetIterRequest returns the vservers on the system
// equivalent to filer::> vserver show
func (d Client) VserverGetIterRequest() (*azgo.VserverGetIterResponse, error) {
	response, err := azgo.NewVserverGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		ExecuteUsing(d.zr)
	return response, err
}

// VserverGetIterAdminRequest returns vservers of type "admin" on the system.
// equivalent to filer::> vserver show -type admin
func (d Client) VserverGetIterAdminRequest() (*azgo.VserverGetIterResponse, error) {
	query := &azgo.VserverGetIterRequestQuery{}
	info := azgo.NewVserverInfoType().SetVserverType("admin")
	query.SetVserverInfo(*info)

	desiredAttributes := &azgo.VserverGetIterRequestDesiredAttributes{}
	desiredInfo := azgo.NewVserverInfoType().
		SetVserverName("").
		SetVserverType("")
	desiredAttributes.SetVserverInfo(*desiredInfo)

	response, err := azgo.NewVserverGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.GetNontunneledZapiRunner())
	return response, err
}

// VserverGetRequest returns vserver to which it is sent
// equivalent to filer::> vserver show
func (d Client) VserverGetRequest() (*azgo.VserverGetResponse, error) {
	response, err := azgo.NewVserverGetRequest().ExecuteUsing(d.zr)
	return response, err
}

// VserverGetAggregateNames returns an array of names of the aggregates assigned to the configured vserver.
// The vserver-get-iter API works with either cluster or vserver scope, so the ZAPI runner may or may not
// be configured for tunneling; using the query parameter ensures we address only the configured vserver.
func (d Client) VserverGetAggregateNames() ([]string, error) {

	// Get just the SVM of interest
	query := &azgo.VserverGetIterRequestQuery{}
	info := azgo.NewVserverInfoType().SetVserverName(d.config.SVM)
	query.SetVserverInfo(*info)

	response, err := azgo.NewVserverGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		ExecuteUsing(d.zr)

	if err != nil {
		return nil, err
	}
	if response.Result.NumRecords() != 1 {
		return nil, fmt.Errorf("could not find SVM %s", d.config.SVM)
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
func (d Client) VserverShowAggrGetIterRequest() (*azgo.VserverShowAggrGetIterResponse, error) {

	response, err := azgo.NewVserverShowAggrGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		ExecuteUsing(d.zr)
	return response, err
}

// VSERVER operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// AGGREGATE operations BEGIN

// AggrSpaceGetIterRequest returns the aggregates on the system
// equivalent to filer::> storage aggregate show-space -aggregate-name aggregate
func (d Client) AggrSpaceGetIterRequest(aggregateName string) (*azgo.AggrSpaceGetIterResponse, error) {
	zr := d.GetNontunneledZapiRunner()

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

func (d Client) getAggregateSize(ctx context.Context, aggregateName string) (int, error) {
	// First, lookup the aggregate and it's space used
	aggregateSizeTotal := NumericalValueNotSet

	responseAggrSpace, err := d.AggrSpaceGetIterRequest(aggregateName)
	if err = GetError(ctx, responseAggrSpace, err); err != nil {
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
func (d Client) AggregateCommitment(ctx context.Context, aggregate string) (*AggregateCommitment, error) {

	zr := d.GetNontunneledZapiRunner()

	// first, get the aggregate's size
	aggregateSize, err := d.getAggregateSize(ctx, aggregate)
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
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		ExecuteUsing(zr)

	if err != nil {
		return nil, err
	}
	if err = GetError(ctx, response, err); err != nil {
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

			Logc(ctx).WithFields(log.Fields{
				"volName":         volName,
				"SizeTotal":       volSpaceAttrs.SizeTotal(),
				"TotalSpaceSaved": volSisAttrs.TotalSpaceSaved(),
				"volAllocated":    volAllocated,
			}).Info("Dumping volume")

			lunAllocated := 0.0
			lunsResponse, lunsResponseErr := d.LunGetAllForVolume(volName)
			if lunsResponseErr != nil {
				return nil, lunsResponseErr
			}
			if lunsResponseErr = GetError(ctx, lunsResponse, lunsResponseErr); lunsResponseErr != nil {
				return nil, fmt.Errorf("error enumerating LUNs for volume %v: %v", volName, lunsResponseErr)
			}

			if lunsResponse.Result.AttributesListPtr != nil &&
				lunsResponse.Result.AttributesListPtr.LunInfoPtr != nil {
				for _, lun := range lunsResponse.Result.AttributesListPtr.LunInfoPtr {
					lunPath := lun.Path()
					lunSize := lun.Size()
					Logc(ctx).WithFields(log.Fields{
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
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// SNAPMIRROR operations BEGIN

// SnapmirrorGetIterRequest returns the snapmirror operations on the destination cluster
// equivalent to filer::> snapmirror show
func (d Client) SnapmirrorGetIterRequest(relGroupType string) (*azgo.SnapmirrorGetIterResponse, error) {
	// Limit list-destination to relationship-group-type matching passed relGroupType
	query := &azgo.SnapmirrorGetIterRequestQuery{}
	relationshipGroupType := azgo.NewSnapmirrorInfoType().
		SetRelationshipGroupType(relGroupType)
	query.SetSnapmirrorInfo(*relationshipGroupType)

	response, err := azgo.NewSnapmirrorGetIterRequest().
		SetQuery(*query).
		ExecuteUsing(d.zr)
	return response, err
}

// SnapmirrorGetDestinationIterRequest returns the snapmirror operations on the source cluster
// equivalent to filer::> snapmirror list-destinations
func (d Client) SnapmirrorGetDestinationIterRequest(relGroupType string) (*azgo.
	SnapmirrorGetDestinationIterResponse, error) {

	// Limit list-destination to relationship-group-type matching passed relGroupType
	query := &azgo.SnapmirrorGetDestinationIterRequestQuery{}
	relationshipGroupType := azgo.NewSnapmirrorDestinationInfoType().
		SetRelationshipGroupType(relGroupType)
	query.SetSnapmirrorDestinationInfo(*relationshipGroupType)

	response, err := azgo.NewSnapmirrorGetDestinationIterRequest().
		SetQuery(*query).
		ExecuteUsing(d.zr)
	return response, err
}

// IsVserverDRDestination identifies if the Vserver is a destination vserver of Snapmirror relationship (SVM-DR) or not
func (d Client) IsVserverDRDestination(ctx context.Context) (bool, error) {

	// first, get the snapmirror destination info using relationship-group-type=vserver in a snapmirror relationship
	relationshipGroupType := "vserver"
	response, err := d.SnapmirrorGetIterRequest(relationshipGroupType)
	isSVMDRDestination := false

	if err != nil {
		return isSVMDRDestination, err
	}
	if err = GetError(ctx, response, err); err != nil {
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
func (d Client) IsVserverDRSource(ctx context.Context) (bool, error) {

	// first, get the snapmirror destination info using relationship-group-type=vserver in a snapmirror relationship
	relationshipGroupType := "vserver"
	response, err := d.SnapmirrorGetDestinationIterRequest(relationshipGroupType)
	isSVMDRSource := false

	if err != nil {
		return isSVMDRSource, err
	}
	if err = GetError(ctx, response, err); err != nil {
		return isSVMDRSource, fmt.Errorf("error getting snapmirror destination info: %v", err)
	}

	// for each of the aggregate's volumes, compute its potential storage usage
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
func (d Client) isVserverInSVMDR(ctx context.Context) bool {
	isSVMDRSource, _ := d.IsVserverDRSource(ctx)
	isSVMDRDestination, _ := d.IsVserverDRDestination(ctx)

	return isSVMDRSource || isSVMDRDestination
}

// SNAPMIRROR operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// MISC operations BEGIN

// NetInterfaceGet returns the list of network interfaces with associated metadata
// equivalent to filer::> net interface list
func (d Client) NetInterfaceGet() (*azgo.NetInterfaceGetIterResponse, error) {
	response, err := azgo.NewNetInterfaceGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		ExecuteUsing(d.zr)
	return response, err
}

func (d Client) NetInterfaceGetDataLIFsNode(ctx context.Context, ip string) (string, error) {
	lifResponse, err := d.NetInterfaceGet()
	if err = GetError(ctx, lifResponse, err); err != nil {
		return "", fmt.Errorf("error checking network interfaces: %v", err)
	}
	var nodeName string
	if lifResponse.Result.AttributesListPtr != nil {
		for _, attrs := range lifResponse.Result.AttributesListPtr.NetInterfaceInfoPtr {
			if ip == attrs.Address() {
				nodeName = attrs.CurrentNode()
				break
			}
		}
	}

	return nodeName, nil
}

func (d Client) NetInterfaceGetDataLIFs(ctx context.Context, protocol string) ([]string, error) {

	lifResponse, err := d.NetInterfaceGet()
	if err = GetError(ctx, lifResponse, err); err != nil {
		return nil, fmt.Errorf("error checking network interfaces: %v", err)
	}

	dataLIFs := make([]string, 0)
	if lifResponse.Result.AttributesListPtr != nil {
		for _, attrs := range lifResponse.Result.AttributesListPtr.NetInterfaceInfoPtr {
			for _, proto := range attrs.DataProtocols().DataProtocolPtr {
				if proto == protocol {
					dataLIFs = append(dataLIFs, attrs.Address())
				}
			}
		}
	}

	Logc(ctx).WithField("dataLIFs", dataLIFs).Debug("Data LIFs")
	return dataLIFs, nil
}

// SystemGetVersion returns the system version
// equivalent to filer::> version
func (d Client) SystemGetVersion() (*azgo.SystemGetVersionResponse, error) {
	response, err := azgo.NewSystemGetVersionRequest().ExecuteUsing(d.zr)
	return response, err
}

// SystemGetOntapiVersion gets the ONTAPI version using the credentials, and caches & returns the result.
func (d Client) SystemGetOntapiVersion(ctx context.Context) (string, error) {

	if d.zr.OntapiVersion == "" {
		result, err := azgo.NewSystemGetOntapiVersionRequest().ExecuteUsing(d.zr)
		if err = GetError(ctx, result, err); err != nil {
			return "", fmt.Errorf("could not read ONTAPI version: %v", err)
		}

		major := result.Result.MajorVersion()
		minor := result.Result.MinorVersion()
		d.zr.OntapiVersion = fmt.Sprintf("%d.%d", major, minor)
	}

	return d.zr.OntapiVersion, nil
}

func (d Client) NodeListSerialNumbers(ctx context.Context) ([]string, error) {

	serialNumbers := make([]string, 0)
	zr := d.GetNontunneledZapiRunner()

	// Limit the returned data to only the serial numbers
	desiredAttributes := &azgo.SystemNodeGetIterRequestDesiredAttributes{}
	info := azgo.NewNodeDetailsInfoType().SetNodeSerialNumber("")
	desiredAttributes.SetNodeDetailsInfo(*info)

	response, err := azgo.NewSystemNodeGetIterRequest().
		SetDesiredAttributes(*desiredAttributes).
		SetMaxRecords(defaultZapiRecords).
		ExecuteUsing(zr)

	Logc(ctx).WithFields(log.Fields{
		"response":          response,
		"info":              info,
		"desiredAttributes": desiredAttributes,
		"err":               err,
	}).Debug("NodeListSerialNumbers")

	if err = GetError(ctx, response, err); err != nil {
		return serialNumbers, err
	}

	if response.Result.NumRecords() == 0 {
		return serialNumbers, errors.New("could not get node info")
	}

	// Get the serial numbers
	if response.Result.AttributesListPtr != nil {
		for _, node := range response.Result.AttributesListPtr.NodeDetailsInfo() {
			serialNumber := node.NodeSerialNumber()
			if serialNumber != "" {
				serialNumbers = append(serialNumbers, serialNumber)
			}
		}
	}

	if len(serialNumbers) == 0 {
		return serialNumbers, errors.New("could not get node serial numbers")
	}

	Logc(ctx).WithFields(log.Fields{
		"Count":         len(serialNumbers),
		"SerialNumbers": strings.Join(serialNumbers, ","),
	}).Debug("Read serial numbers.")

	return serialNumbers, nil
}

// EmsAutosupportLog generates an auto support message with the supplied parameters
func (d Client) EmsAutosupportLog(
	appVersion string,
	autoSupport bool,
	category string,
	computerName string,
	eventDescription string,
	eventID int,
	eventSource string,
	logLevel int) (*azgo.EmsAutosupportLogResponse, error) {

	response, err := azgo.NewEmsAutosupportLogRequest().
		SetAutoSupport(autoSupport).
		SetAppVersion(appVersion).
		SetCategory(category).
		SetComputerName(computerName).
		SetEventDescription(eventDescription).
		SetEventId(eventID).
		SetEventSource(eventSource).
		SetLogLevel(logLevel).
		ExecuteUsing(d.zr)
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

func (d Client) TieringPolicyValue(ctx context.Context) string {
	tieringPolicy := "none"
	// If ONTAP version < 9.5
	if !d.SupportsFeature(ctx, FabricPoolForSVMDR) {
		if d.isVserverInSVMDR(ctx) {
			tieringPolicy = "snapshot-only"
		}
	}

	return tieringPolicy
}

// MISC operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// iSCSI initiator operations BEGIN

// IscsiInitiatorAddAuth creates and sets the authorization details for a single initiator
// equivalent to filer::> vserver iscsi security create -vserver SVM -initiator-name iqn.1993-08.org.debian:01:9031309bbebd \
//                          -auth-type CHAP -user-name outboundUserName -outbound-user-name outboundPassphrase
func (d Client) IscsiInitiatorAddAuth(initiator, authType, userName, passphrase, outboundUserName, outboundPassphrase string) (*azgo.IscsiInitiatorAddAuthResponse, error) {
	request := azgo.NewIscsiInitiatorAddAuthRequest().
		SetInitiator(initiator).
		SetAuthType(authType).
		SetUserName(userName).
		SetPassphrase(passphrase)
	if outboundUserName != "" && outboundPassphrase != "" {
		request.SetOutboundUserName(outboundUserName)
		request.SetOutboundPassphrase(outboundPassphrase)
	}
	response, err := request.ExecuteUsing(d.zr)
	return response, err
}

// IscsiInitiatorAuthGetIter returns the authorization details for all non-default initiators for the Client's SVM
// equivalent to filer::> vserver iscsi security show -vserver SVM
func (d Client) IscsiInitiatorAuthGetIter() ([]azgo.IscsiSecurityEntryInfoType, error) {
	response, err := azgo.NewIscsiInitiatorAuthGetIterRequest().
		ExecuteUsing(d.zr)

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
func (d Client) IscsiInitiatorDeleteAuth(initiator string) (*azgo.IscsiInitiatorDeleteAuthResponse, error) {
	response, err := azgo.NewIscsiInitiatorDeleteAuthRequest().
		SetInitiator(initiator).
		ExecuteUsing(d.zr)
	return response, err
}

// IscsiInitiatorGetAuth returns the authorization details for a single initiator
// equivalent to filer::> vserver iscsi security show -vserver SVM -initiator-name iqn.1993-08.org.debian:01:9031309bbebd
//            or filer::> vserver iscsi security show -vserver SVM -initiator-name default
func (d Client) IscsiInitiatorGetAuth(initiator string) (*azgo.IscsiInitiatorGetAuthResponse, error) {
	response, err := azgo.NewIscsiInitiatorGetAuthRequest().
		SetInitiator(initiator).
		ExecuteUsing(d.zr)
	return response, err
}

// IscsiInitiatorGetDefaultAuth returns the authorization details for the default initiator
// equivalent to filer::> vserver iscsi security show -vserver SVM -initiator-name default
func (d Client) IscsiInitiatorGetDefaultAuth() (*azgo.IscsiInitiatorGetDefaultAuthResponse, error) {
	response, err := azgo.NewIscsiInitiatorGetDefaultAuthRequest().
		ExecuteUsing(d.zr)
	return response, err
}

// IscsiInitiatorGetIter returns the initiator details for all non-default initiators for the Client's SVM
// equivalent to filer::> vserver iscsi initiator show -vserver SVM
func (d Client) IscsiInitiatorGetIter() ([]azgo.IscsiInitiatorListEntryInfoType, error) {
	response, err := azgo.NewIscsiInitiatorGetIterRequest().
		ExecuteUsing(d.zr)

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
// equivalent to filer::> vserver iscsi security modify -vserver SVM -initiator-name iqn.1993-08.org.debian:01:9031309bbebd \
//                          -user-name outboundUserName -outbound-user-name outboundPassphrase
func (d Client) IscsiInitiatorModifyCHAPParams(initiator, userName, passphrase, outboundUserName, outboundPassphrase string) (*azgo.IscsiInitiatorModifyChapParamsResponse, error) {
	request := azgo.NewIscsiInitiatorModifyChapParamsRequest().
		SetInitiator(initiator).
		SetUserName(userName).
		SetPassphrase(passphrase)
	if outboundUserName != "" && outboundPassphrase != "" {
		request.SetOutboundUserName(outboundUserName)
		request.SetOutboundPassphrase(outboundPassphrase)
	}
	response, err := request.ExecuteUsing(d.zr)
	return response, err
}

// IscsiInitiatorSetDefaultAuth sets the authorization details for the default initiator
// equivalent to filer::> vserver iscsi security modify -vserver SVM -initiator-name default \
//                           -auth-type CHAP -user-name outboundUserName -outbound-user-name outboundPassphrase
func (d Client) IscsiInitiatorSetDefaultAuth(authType, userName, passphrase, outboundUserName, outboundPassphrase string) (*azgo.IscsiInitiatorSetDefaultAuthResponse, error) {
	request := azgo.NewIscsiInitiatorSetDefaultAuthRequest().
		SetAuthType(authType).
		SetUserName(userName).
		SetPassphrase(passphrase)
	if outboundUserName != "" && outboundPassphrase != "" {
		request.SetOutboundUserName(outboundUserName)
		request.SetOutboundPassphrase(outboundPassphrase)
	}
	response, err := request.ExecuteUsing(d.zr)
	return response, err
}

// iSCSI initiator operations END
/////////////////////////////////////////////////////////////////////////////
