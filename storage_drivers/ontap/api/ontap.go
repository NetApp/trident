// Copyright 2018 NetApp, Inc. All Rights Reserved.

package api

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/utils"
)

const (
	defaultZapiRecords   = 100
	NumericalValueNotSet = -1
	maxFlexGroupWait     = 30 * time.Second
)

// ClientConfig holds the configuration data for Client objects
type ClientConfig struct {
	ManagementLIF   string
	SVM             string
	Username        string
	Password        string
	DebugTraceFlags map[string]bool
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

	val := NewZapiResultValue(zapiResult)
	if reflect.TypeOf(zapiResult).Kind() == reflect.Ptr {
		val = reflect.Indirect(val)
	}

	err = ZapiError{
		val.FieldByName("ResultStatusAttr").String(),
		val.FieldByName("ResultReasonAttr").String(),
		val.FieldByName("ResultErrnoAttr").String(),
	}

	return err
}

// NewZapiAsyncResult accepts the Response value from any AZGO Async Request, extracts the status, jobId, and
// errorCode values and returns a ZapiAsyncResult.
func NewZapiAsyncResult(zapiResult interface{}) (result ZapiAsyncResult, err error) {
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

	if s := val.FieldByName("ResultStatusPtr"); !s.IsNil() {
		status = s.Elem().String()
	}
	if j := val.FieldByName("ResultJobidPtr"); !j.IsNil() {
		jobId = j.Elem().Int()
	}
	if e := val.FieldByName("ResultErrorCodePtr"); !e.IsNil() {
		errorCode = e.Elem().Int()
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
func GetError(zapiResult interface{}, errorIn error) (errorOut error) {

	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Panic in ontap#GetError. %v", zapiResult)
			errorOut = ZapiError{}
		}
	}()

	// A ZAPI Result struct works as-is, but a ZAPI Response struct must have its
	// embedded Result struct extracted via reflection.
	val := reflect.ValueOf(zapiResult)
	if reflect.TypeOf(zapiResult).Kind() == reflect.Ptr {
		val = reflect.Indirect(val)
	}
	if testResult := val.FieldByName("Result"); testResult.IsValid() {
		zapiResult = testResult.Interface()
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
	MinimumONTAPIVersion   feature = "MINIMUM_ONTAPI_VERSION"
	VServerShowAggr        feature = "VSERVER_SHOW_AGGR"
	FlexGroupsFilter       feature = "FLEX_GROUPS_FILTER"
	NetAppVolumeEncryption feature = "NETAPP_VOLUME_ENCRYPTION"
	NetAppFlexGroups       feature = "NETAPP_FLEX_GROUPS"
	LunGeometrySkip        feature = "LUN_GEOMETRY_SKIP"
)

// Indicate the minimum Ontapi version for each feature here
var features = map[feature]*utils.Version{
	MinimumONTAPIVersion:   utils.MustParseSemantic("1.30.0"),  // cDOT 8.3.0
	VServerShowAggr:        utils.MustParseSemantic("1.100.0"), // cDOT 9.0.0
	FlexGroupsFilter:       utils.MustParseSemantic("1.100.0"), // cDOT 9.0.0
	NetAppVolumeEncryption: utils.MustParseSemantic("1.110.0"), // cDOT 9.1.0
	NetAppFlexGroups:       utils.MustParseSemantic("1.120.0"), // cDOT 9.2.0
	LunGeometrySkip:        utils.MustParseSemantic("1.150.0"), // cDOT 9.5.0
}

// SupportsFeature returns true if the Ontapi version supports the supplied feature
func (d Client) SupportsFeature(feature feature) bool {

	ontapiVersion, err := d.SystemGetOntapiVersion()
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
func (d Client) IgroupCreate(initiatorGroupName, initiatorGroupType, osType string) (azgo.IgroupCreateResponse, error) {
	response, err := azgo.NewIgroupCreateRequest().
		SetInitiatorGroupName(initiatorGroupName).
		SetInitiatorGroupType(initiatorGroupType).
		SetOsType(osType).
		ExecuteUsing(d.zr)
	return *response, err
}

// IgroupAdd adds an initiator to an initiator group
// equivalent to filer::> igroup add -vserver iscsi_vs -igroup docker -initiator iqn.1993-08.org.debian:01:9031309bbebd
func (d Client) IgroupAdd(initiatorGroupName, initiator string) (azgo.IgroupAddResponse, error) {
	response, err := azgo.NewIgroupAddRequest().
		SetInitiatorGroupName(initiatorGroupName).
		SetInitiator(initiator).
		ExecuteUsing(d.zr)
	return *response, err
}

// IgroupRemove removes an initiator from an initiator group
func (d Client) IgroupRemove(initiatorGroupName, initiator string, force bool) (azgo.IgroupRemoveResponse, error) {
	response, err := azgo.NewIgroupRemoveRequest().
		SetInitiatorGroupName(initiatorGroupName).
		SetInitiator(initiator).
		SetForce(force).
		ExecuteUsing(d.zr)
	return *response, err
}

// IgroupDestroy destroys an initiator group
func (d Client) IgroupDestroy(initiatorGroupName string) (azgo.IgroupDestroyResponse, error) {
	response, err := azgo.NewIgroupDestroyRequest().
		SetInitiatorGroupName(initiatorGroupName).
		ExecuteUsing(d.zr)
	return *response, err
}

// IgroupList lists initiator groups
func (d Client) IgroupList() (azgo.IgroupGetIterResponse, error) {
	response, err := azgo.NewIgroupGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		ExecuteUsing(d.zr)
	return *response, err
}

// IGROUP operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// LUN operations BEGIN

// LunCreate creates a lun with the specified attributes
// equivalent to filer::> lun create -vserver iscsi_vs -path /vol/v/lun1 -size 1g -ostype linux -space-reserve disabled
func (d Client) LunCreate(lunPath string, sizeInBytes int, osType string, spaceReserved bool) (azgo.LunCreateBySizeResponse, error) {
	response, err := azgo.NewLunCreateBySizeRequest().
		SetPath(lunPath).
		SetSize(sizeInBytes).
		SetOstype(osType).
		SetSpaceReservationEnabled(spaceReserved).
		ExecuteUsing(d.zr)
	return *response, err
}

// LunGetSerialNumber returns the serial# for a lun
func (d Client) LunGetSerialNumber(lunPath string) (azgo.LunGetSerialNumberResponse, error) {
	response, err := azgo.NewLunGetSerialNumberRequest().
		SetPath(lunPath).
		ExecuteUsing(d.zr)
	return *response, err
}

// LunMap maps a lun to an id in an initiator group
// equivalent to filer::> lun map -vserver iscsi_vs -path /vol/v/lun1 -igroup docker -lun-id 0
func (d Client) LunMap(initiatorGroupName, lunPath string, lunID int) (azgo.LunMapResponse, error) {
	response, err := azgo.NewLunMapRequest().
		SetInitiatorGroup(initiatorGroupName).
		SetPath(lunPath).
		SetLunId(lunID).
		ExecuteUsing(d.zr)
	return *response, err
}

// LunMapAutoID maps a LUN in an initiator group, allowing ONTAP to choose an available LUN ID
// equivalent to filer::> lun map -vserver iscsi_vs -path /vol/v/lun1 -igroup docker
func (d Client) LunMapAutoID(initiatorGroupName, lunPath string) (azgo.LunMapResponse, error) {
	response, err := azgo.NewLunMapRequest().
		SetInitiatorGroup(initiatorGroupName).
		SetPath(lunPath).
		ExecuteUsing(d.zr)
	return *response, err
}

func (d Client) LunMapIfNotMapped(initiatorGroupName, lunPath string) (int, error) {

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
			if igroup.InitiatorGroupName() == initiatorGroupName {

				lunID = igroup.LunId()
				alreadyMapped = true

				log.WithFields(log.Fields{
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

		log.WithFields(log.Fields{
			"lun":    lunPath,
			"igroup": initiatorGroupName,
			"id":     lunID,
		}).Debug("LUN mapped.")
	}

	return lunID, nil
}

// LunMapListInfo returns lun mapping information for the specified lun
// equivalent to filer::> lun mapped show -vserver iscsi_vs -path /vol/v/lun0
func (d Client) LunMapListInfo(lunPath string) (azgo.LunMapListInfoResponse, error) {
	response, err := azgo.NewLunMapListInfoRequest().
		SetPath(lunPath).
		ExecuteUsing(d.zr)
	return *response, err
}

// LunOffline offlines a lun
// equivalent to filer::> lun offline -vserver iscsi_vs -path /vol/v/lun0
func (d Client) LunOffline(lunPath string) (azgo.LunOfflineResponse, error) {
	response, err := azgo.NewLunOfflineRequest().
		SetPath(lunPath).
		ExecuteUsing(d.zr)
	return *response, err
}

// LunOnline onlines a lun
// equivalent to filer::> lun online -vserver iscsi_vs -path /vol/v/lun0
func (d Client) LunOnline(lunPath string) (azgo.LunOnlineResponse, error) {
	response, err := azgo.NewLunOnlineRequest().
		SetPath(lunPath).
		ExecuteUsing(d.zr)
	return *response, err
}

// LunDestroy destroys a lun
// equivalent to filer::> lun destroy -vserver iscsi_vs -path /vol/v/lun0
func (d Client) LunDestroy(lunPath string) (azgo.LunDestroyResponse, error) {
	response, err := azgo.NewLunDestroyRequest().
		SetPath(lunPath).
		ExecuteUsing(d.zr)
	return *response, err
}

// LunSetAttribute sets a named attribute for a given LUN.
func (d Client) LunSetAttribute(lunPath, name, value string) (azgo.LunSetAttributeResponse, error) {
	response, err := azgo.NewLunSetAttributeRequest().
		SetPath(lunPath).
		SetName(name).
		SetValue(value).
		ExecuteUsing(d.zr)
	return *response, err
}

// LunGetAttribute gets a named attribute for a given LUN.
func (d Client) LunGetAttribute(lunPath, name string) (azgo.LunGetAttributeResponse, error) {
	response, err := azgo.NewLunGetAttributeRequest().
		SetPath(lunPath).
		SetName(name).
		ExecuteUsing(d.zr)
	return *response, err
}

// LunGet returns all relevant details for a single LUN
// equivalent to filer::> lun show
func (d Client) LunGet(path string) (azgo.LunInfoType, error) {

	// Limit the LUNs to the one matching the path
	//query := azgo.NewLunInfoType().SetPath(path)
	query := &azgo.LunGetIterRequestQuery{}
	lunInfo := azgo.NewLunInfoType().
		SetPath(path)
	query.SetLunInfo(*lunInfo)

	// Limit the returned data to only the data relevant to containers
	desiredAttributes := &azgo.LunGetIterRequestDesiredAttributes{}
	lunInfo = azgo.NewLunInfoType().
		SetPath("").
		SetVolume("").
		SetSize(0)
	desiredAttributes.SetLunInfo(*lunInfo)

	response, err := azgo.NewLunGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)

	if err != nil {
		return azgo.LunInfoType{}, err
	} else if response.Result.NumRecords() == 0 {
		return azgo.LunInfoType{}, fmt.Errorf("LUN %s not found", path)
	} else if response.Result.NumRecords() > 1 {
		return azgo.LunInfoType{}, fmt.Errorf("more than one LUN %s found", path)
	} else if response.Result.AttributesListPtr == nil {
		return azgo.LunInfoType{}, fmt.Errorf("LUN %s not found", path)
	}

	return response.Result.AttributesListPtr.LunInfoPtr[0], nil
}

func (d Client) lunGetAllCommon(query *azgo.LunGetIterRequestQuery) (azgo.LunGetIterResponse, error) {
	// Limit the returned data to only the data relevant to containers
	desiredAttributes := &azgo.LunGetIterRequestDesiredAttributes{}
	lunInfo := azgo.NewLunInfoType().
		SetPath("").
		SetVolume("").
		SetSize(0)
	desiredAttributes.SetLunInfo(*lunInfo)

	response, err := azgo.NewLunGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)
	return *response, err
}

func (d Client) LunGetGeometry(path string) (azgo.LunGetGeometryResponse, error) {
	response, err := azgo.NewLunGetGeometryRequest().
		SetPath(path).
		ExecuteUsing(d.zr)
	return *response, err
}

func (d Client) LunResize(path string, sizeBytes int) (azgo.LunResizeResponse, error) {
	response, err := azgo.NewLunResizeRequest().
		SetPath(path).
		SetSize(sizeBytes).
		ExecuteUsing(d.zr)
	return *response, err
}

// LunGetAll returns all relevant details for all LUNs whose paths match the supplied pattern
// equivalent to filer::> lun show -path /vol/trident_*/lun0
func (d Client) LunGetAll(pathPattern string) (azgo.LunGetIterResponse, error) {

	// Limit LUNs to those matching the pathPattern; ex, "/vol/trident_*/lun0"
	query := &azgo.LunGetIterRequestQuery{}
	lunInfo := azgo.NewLunInfoType().
		SetPath(pathPattern)
	query.SetLunInfo(*lunInfo)

	return d.lunGetAllCommon(query)
}

// LunGetAllForVolume returns all relevant details for all LUNs in the supplied Volume
// equivalent to filer::> lun show -volume trident_CEwDWXQRPz
func (d Client) LunGetAllForVolume(volumeName string) (azgo.LunGetIterResponse, error) {

	// Limit LUNs to those owned by the volumeName; ex, "trident_trident"
	query := &azgo.LunGetIterRequestQuery{}
	lunInfo := azgo.NewLunInfoType().
		SetVolume(volumeName)
	query.SetLunInfo(*lunInfo)

	return d.lunGetAllCommon(query)
}

// LUN operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// FlexGroup operations BEGIN

// FlexGroupCreate creates a FlexGroup with the specified options
// equivalent to filer::> volume create -vserver svm_name -volume fg_vol_name –auto-provision-as flexgroup -size fg_size  -state online -type RW -policy default -unix-permissions ---rwxr-xr-x -space-guarantee none -snapshot-policy none -security-style unix -encrypt false
func (d Client) FlexGroupCreate(name string, size int, aggrs []azgo.AggrNameType, spaceReserve, snapshotPolicy, unixPermissions,
	exportPolicy, securityStyle string, encrypt *bool, snapshotReserve int) (azgo.VolumeCreateAsyncResponse, error) {
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
		SetEncrypt(*encrypt).
		SetAggrList(aggrList).
		SetJunctionPath(junctionPath)

	if snapshotReserve != NumericalValueNotSet {
		request.SetPercentageSnapshotReserve(snapshotReserve)
	}

	response, err := request.ExecuteUsing(d.zr)
	if zerr := GetError(*response, err); zerr != nil {
		return *response, zerr
	}

	err = d.waitForAsyncResponse(*response, time.Duration(maxFlexGroupWait))
	if err != nil {
		return *response, fmt.Errorf("error waiting for response: %v", err)
	}

	return *response, err
}

// FlexGroupDestroy destroys a FlexGroup
func (d Client) FlexGroupDestroy(name string, force bool) (azgo.VolumeDestroyAsyncResponse, error) {
	response, err := azgo.NewVolumeDestroyAsyncRequest().
		SetVolumeName(name).
		SetUnmountAndOffline(force).
		ExecuteUsing(d.zr)

	if zerr := NewZapiError(*response); !zerr.IsPassed() {
		// It's not an error if the volume no longer exists
		if zerr.Code() == azgo.EVOLUMEDOESNOTEXIST {
			log.WithField("volume", name).Warn("FlexGroup already deleted.")
			return *response, nil
		}
	}

	if gerr := GetError(response, err); gerr != nil {
		return *response, gerr
	}

	err = d.waitForAsyncResponse(*response, time.Duration(maxFlexGroupWait))
	if err != nil {
		return *response, fmt.Errorf("error waiting for response: %v", err)
	}

	return *response, err
}

// FlexGroupExists tests for the existence of a FlexGroup
func (d Client) FlexGroupExists(name string) (bool, error) {
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

	if gerr := GetError(response, err); gerr != nil {
		return false, gerr
	}

	// Wait for Async Job to complete
	err = d.waitForAsyncResponse(response, time.Duration(maxFlexGroupWait))
	if err != nil {
		return false, fmt.Errorf("error waiting for response: %v", err)
	}

	return true, nil
}

// FlexGroupVolumeSize retrieves the size of the specified volume
func (d Client) FlexGroupSize(name string) (int, error) {
	volAttrs, err := d.FlexGroupGet(name)
	if err != nil {
		return 0, err
	}

	volSpaceAttrs := volAttrs.VolumeSpaceAttributes()
	return volSpaceAttrs.Size(), nil
}

// FlexGroupSetSize sets the size of the specified FlexGroup
func (d Client) FlexGroupSetSize(name, newSize string) (azgo.VolumeSizeAsyncResponse, error) {
	response, err := azgo.NewVolumeSizeAsyncRequest().
		SetVolumeName(name).
		SetNewSize(newSize).
		ExecuteUsing(d.zr)

	if zerr := GetError(*response, err); zerr != nil {
		return *response, zerr
	}

	err = d.waitForAsyncResponse(*response, time.Duration(maxFlexGroupWait))
	if err != nil {
		return *response, fmt.Errorf("error waiting for response: %v", err)
	}

	return *response, err
}

// FlexGroupVolumeDisableSnapshotDirectoryAccess disables access to the ".snapshot" directory
// Disable '.snapshot' to allow official mysql container's chmod-in-init to work
func (d Client) FlexGroupVolumeDisableSnapshotDirectoryAccess(name string) (azgo.VolumeModifyIterAsyncResponse, error) {

	volattr := &azgo.VolumeModifyIterAsyncRequestAttributes{}
	ssattr := azgo.NewVolumeSnapshotAttributesType().SetSnapdirAccessEnabled(false)
	volSnapshotAttrs := azgo.NewVolumeAttributesType().SetVolumeSnapshotAttributes(*ssattr)
	volattr.SetVolumeAttributes(*volSnapshotAttrs)

	queryattr := &azgo.VolumeModifyIterAsyncRequestQuery{}
	volidattr := azgo.NewVolumeIdAttributesType().SetName(azgo.VolumeNameType(name))
	volIdAttrs := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*volidattr)
	queryattr.SetVolumeAttributes(*volIdAttrs)

	response, err := azgo.NewVolumeModifyIterAsyncRequest().
		SetQuery(*queryattr).
		SetAttributes(*volattr).
		ExecuteUsing(d.zr)

	if zerr := GetError(*response, err); zerr != nil {
		return *response, zerr
	}

	err = d.waitForAsyncResponse(*response, time.Duration(maxFlexGroupWait))
	if err != nil {
		return *response, fmt.Errorf("error waiting for response: %v", err)
	}

	return *response, err
}

// FlexGroupGet returns all relevant details for a single FlexGroup
func (d Client) FlexGroupGet(name string) (azgo.VolumeAttributesType, error) {
	// Limit the FlexGroups to the one matching the name
	queryVolIDAttrs := azgo.NewVolumeIdAttributesType().SetName(azgo.VolumeNameType(name))
	queryVolIDAttrs.SetStyleExtended("flexgroup")
	return d.volumeGetIterCommon(name, queryVolIDAttrs)
}

// FlexGroupGetAll returns all relevant details for all FlexGroups whose names match the supplied prefix
func (d Client) FlexGroupGetAll(prefix string) (azgo.VolumeGetIterResponse, error) {
	// Limit the FlexGroups to those matching the name prefix
	queryVolIDAttrs := azgo.NewVolumeIdAttributesType().SetName(azgo.VolumeNameType(prefix + "*"))
	queryVolStateAttrs := azgo.NewVolumeStateAttributesType().SetState("online")
	queryVolIDAttrs.SetStyleExtended("flexgroup")
	return d.volumeGetIterAll(prefix, queryVolIDAttrs, queryVolStateAttrs)
}

// waitForAsyncResponse handles waiting for an AsyncResponse to return successfully or return an error.
func (d Client) waitForAsyncResponse(zapiResult interface{}, maxWaitTime time.Duration) error {

	asyncResult, err := NewZapiAsyncResult(zapiResult)
	if err != nil {
		return err
	}

	// Possible values: "succeeded", "in_progress", "failed". Returns nil if succeeded
	if asyncResult.status == "in_progress" {
		// handle zapi response
		jobId := int(asyncResult.jobId)
		if asyncResponseError := d.checkForJobCompletion(jobId, maxWaitTime); asyncResponseError != nil {
			return asyncResponseError
		}
	} else if asyncResult.status == "failed" {
		return fmt.Errorf("result status is failed with errorCode %d", asyncResult.errorCode)
	}

	return nil
}

// checkForJobCompletion polls for the ONTAP job status success with backoff retry logic
func (d *Client) checkForJobCompletion(jobId int, maxWaitTime time.Duration) error {

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
		log.WithFields(log.Fields{
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
		log.WithField("duration", duration).
			Debug("Job not yet completed, waiting.")
	}

	inProgressBackoff := asyncResponseBackoff(maxWaitTime)

	// Run the job completion check using an exponential backoff
	if err := backoff.RetryNotify(checkJobFinished, inProgressBackoff, jobCompletedNotify); err != nil {
		log.Warnf("Job not completed after %v seconds.", inProgressBackoff.MaxElapsedTime.Seconds())
		return fmt.Errorf("job Id %d failed to complete successfully", jobId)
	} else {
		//log.WithField("volume", name).Debug("Volume found.")
		log.WithField("jobId", jobId).Debug("Job completed successfully.")
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
func (d Client) JobGetIterStatus(jobId int) (azgo.JobGetIterResponse, error) {
	jobInfo := azgo.NewJobInfoType().SetJobId(jobId)
	queryAttr := &azgo.JobGetIterRequestQuery{}
	queryAttr.SetJobInfo(*jobInfo)

	response, err := azgo.NewJobGetIterRequest().
		SetQuery(*queryAttr).
		ExecuteUsing(d.GetNontunneledZapiRunner())
	return *response, err
}

// FlexGroup operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// VOLUME operations BEGIN

// VolumeCreate creates a volume with the specified options
// equivalent to filer::> volume create -vserver iscsi_vs -volume v -aggregate aggr1 -size 1g -state online -type RW -policy default -unix-permissions ---rwxr-xr-x -space-guarantee none -snapshot-policy none -security-style unix -encrypt false
func (d Client) VolumeCreate(name, aggregateName, size, spaceReserve, snapshotPolicy, unixPermissions,
	exportPolicy, securityStyle string, encrypt *bool, snapshotReserve int,
) (azgo.VolumeCreateResponse, error) {
	request := azgo.NewVolumeCreateRequest().
		SetVolume(name).
		SetContainingAggrName(aggregateName).
		SetSize(size).
		SetSpaceReserve(spaceReserve).
		SetSnapshotPolicy(snapshotPolicy).
		SetUnixPermissions(unixPermissions).
		SetExportPolicy(exportPolicy).
		SetVolumeSecurityStyle(securityStyle)

	// Don't send 'encrypt' unless needed, as pre-9.1 ONTAP won't accept it.
	if encrypt != nil {
		request.SetEncrypt(*encrypt)
	}

	if snapshotReserve != NumericalValueNotSet {
		request.SetPercentageSnapshotReserve(snapshotReserve)
	}

	response, err := request.ExecuteUsing(d.zr)
	return *response, err
}

// VolumeCloneCreate clones a volume from a snapshot
func (d Client) VolumeCloneCreate(name, source, snapshot string) (azgo.VolumeCloneCreateResponse, error) {
	response, err := azgo.NewVolumeCloneCreateRequest().
		SetVolume(name).
		SetParentVolume(source).
		SetParentSnapshot(snapshot).
		ExecuteUsing(d.zr)
	return *response, err
}

// VolumeCloneSplitStart splits a cloned volume from its parent
func (d Client) VolumeCloneSplitStart(name string) (azgo.VolumeCloneSplitStartResponse, error) {
	response, err := azgo.NewVolumeCloneSplitStartRequest().
		SetVolume(name).
		ExecuteUsing(d.zr)
	return *response, err
}

// VolumeDisableSnapshotDirectoryAccess disables access to the ".snapshot" directory
// Disable '.snapshot' to allow official mysql container's chmod-in-init to work
func (d Client) VolumeDisableSnapshotDirectoryAccess(name string) (azgo.VolumeModifyIterResponse, error) {
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
	return *response, err
}

// VolumeExists tests for the existence of a Flexvol
func (d Client) VolumeExists(name string) (bool, error) {
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
func (d Client) VolumeSetSize(name, newSize string) (azgo.VolumeSizeResponse, error) {
	response, err := azgo.NewVolumeSizeRequest().
		SetVolume(name).
		SetNewSize(newSize).
		ExecuteUsing(d.zr)
	return *response, err
}

// VolumeMount mounts a volume at the specified junction
func (d Client) VolumeMount(name, junctionPath string) (azgo.VolumeMountResponse, error) {
	response, err := azgo.NewVolumeMountRequest().
		SetVolumeName(name).
		SetJunctionPath(junctionPath).
		ExecuteUsing(d.zr)
	return *response, err
}

// VolumeUnmount unmounts a volume from the specified junction
func (d Client) VolumeUnmount(name string, force bool) (azgo.VolumeUnmountResponse, error) {
	response, err := azgo.NewVolumeUnmountRequest().
		SetVolumeName(name).
		SetForce(force).
		ExecuteUsing(d.zr)
	return *response, err
}

// VolumeOffline offlines a volume
func (d Client) VolumeOffline(name string) (azgo.VolumeOfflineResponse, error) {
	response, err := azgo.NewVolumeOfflineRequest().
		SetName(name).
		ExecuteUsing(d.zr)
	return *response, err
}

// VolumeDestroy destroys a volume
func (d Client) VolumeDestroy(name string, force bool) (azgo.VolumeDestroyResponse, error) {
	response, err := azgo.NewVolumeDestroyRequest().
		SetName(name).
		SetUnmountAndOffline(force).
		ExecuteUsing(d.zr)
	return *response, err
}

// VolumeGet returns all relevant details for a single Flexvol
// equivalent to filer::> volume show
func (d Client) VolumeGet(name string) (azgo.VolumeAttributesType, error) {

	// Limit the Flexvols to the one matching the name
	queryVolIDAttrs := azgo.NewVolumeIdAttributesType().SetName(azgo.VolumeNameType(name))
	if d.SupportsFeature(FlexGroupsFilter) {
		queryVolIDAttrs.SetStyleExtended("flexvol")
	}
	return d.volumeGetIterCommon(name, queryVolIDAttrs)
}

func (d Client) volumeGetIterCommon(name string,
	queryVolIDAttrs *azgo.VolumeIdAttributesType) (azgo.VolumeAttributesType, error) {

	queryVolStateAttrs := azgo.NewVolumeStateAttributesType().SetState("online")

	query := &azgo.VolumeGetIterRequestQuery{}
	volAttrs := azgo.NewVolumeAttributesType().
		SetVolumeIdAttributes(*queryVolIDAttrs).
		SetVolumeStateAttributes(*queryVolStateAttrs)
	query.SetVolumeAttributes(*volAttrs)

	response, err := azgo.NewVolumeGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		ExecuteUsing(d.zr)

	if err != nil {
		return azgo.VolumeAttributesType{}, err
	} else if response.Result.NumRecords() == 0 {
		return azgo.VolumeAttributesType{}, fmt.Errorf("flexvol %s not found", name)
	} else if response.Result.NumRecords() > 1 {
		return azgo.VolumeAttributesType{}, fmt.Errorf("more than one Flexvol %s found", name)
	} else if response.Result.AttributesListPtr == nil {
		return azgo.VolumeAttributesType{}, fmt.Errorf("flexvol %s not found", name)
	}

	return response.Result.AttributesListPtr.VolumeAttributesPtr[0], nil
}

// VolumeGetAll returns all relevant details for all FlexVols whose names match the supplied prefix
// equivalent to filer::> volume show
func (d Client) VolumeGetAll(prefix string) (response azgo.VolumeGetIterResponse, err error) {

	// Limit the Flexvols to those matching the name prefix
	queryVolIDAttrs := azgo.NewVolumeIdAttributesType().SetName(azgo.VolumeNameType(prefix + "*"))
	queryVolStateAttrs := azgo.NewVolumeStateAttributesType().SetState("online")
	if d.SupportsFeature(FlexGroupsFilter) {
		queryVolIDAttrs.SetStyleExtended("flexvol")
	}

	return d.volumeGetIterAll(prefix, queryVolIDAttrs, queryVolStateAttrs)
}

func (d Client) volumeGetIterAll(prefix string, queryVolIDAttrs *azgo.VolumeIdAttributesType,
	queryVolStateAttrs *azgo.VolumeStateAttributesType) (azgo.VolumeGetIterResponse, error) {

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
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)
	return *response, err
}

// VolumeList returns the names of all Flexvols whose names match the supplied prefix
func (d Client) VolumeList(prefix string) (azgo.VolumeGetIterResponse, error) {

	// Limit the Flexvols to those matching the name prefix
	query := &azgo.VolumeGetIterRequestQuery{}
	queryVolIDAttrs := azgo.NewVolumeIdAttributesType().SetName(azgo.VolumeNameType(prefix + "*"))
	if d.SupportsFeature(FlexGroupsFilter) {
		queryVolIDAttrs.SetStyleExtended("flexvol")
	}
	queryVolStateAttrs := azgo.NewVolumeStateAttributesType().SetState("online")
	volumeAttributes := azgo.NewVolumeAttributesType().
		SetVolumeIdAttributes(*queryVolIDAttrs).
		SetVolumeStateAttributes(*queryVolStateAttrs)
	query.SetVolumeAttributes(*volumeAttributes)

	// Limit the returned data to only the Flexvol names
	desiredAttributes := &azgo.VolumeGetIterRequestDesiredAttributes{}
	desiredVolIDAttrs := azgo.NewVolumeIdAttributesType().SetName("")
	desiredVolumeAttributes := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*desiredVolIDAttrs)
	desiredAttributes.SetVolumeAttributes(*desiredVolumeAttributes)

	response, err := azgo.NewVolumeGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)
	return *response, err
}

// VolumeListByAttrs returns the names of all Flexvols matching the specified attributes
func (d Client) VolumeListByAttrs(
	prefix, aggregate, spaceReserve, snapshotPolicy string, snapshotDir bool, encrypt *bool,
) (azgo.VolumeGetIterResponse, error) {

	// Limit the Flexvols to those matching the specified attributes
	query := &azgo.VolumeGetIterRequestQuery{}
	queryVolIDAttrs := azgo.NewVolumeIdAttributesType().
		SetName(azgo.VolumeNameType(prefix + "*")).
		SetContainingAggregateName(aggregate)
	if d.SupportsFeature(FlexGroupsFilter) {
		queryVolIDAttrs.SetStyleExtended("flexvol")
	}
	queryVolSpaceAttrs := azgo.NewVolumeSpaceAttributesType().
		SetSpaceGuarantee(spaceReserve)
	queryVolSnapshotAttrs := azgo.NewVolumeSnapshotAttributesType().
		SetSnapshotPolicy(snapshotPolicy).
		SetSnapdirAccessEnabled(snapshotDir)
	queryVolStateAttrs := azgo.NewVolumeStateAttributesType().
		SetState("online")
	volumeAttributes := azgo.NewVolumeAttributesType().
		SetVolumeIdAttributes(*queryVolIDAttrs).
		SetVolumeSpaceAttributes(*queryVolSpaceAttrs).
		SetVolumeSnapshotAttributes(*queryVolSnapshotAttrs).
		SetVolumeStateAttributes(*queryVolStateAttrs)
	query.SetVolumeAttributes(*volumeAttributes)

	if encrypt != nil {
		volumeAttributes.SetEncrypt(*encrypt)
	}

	// Limit the returned data to only the Flexvol names
	desiredAttributes := &azgo.VolumeGetIterRequestDesiredAttributes{}
	desiredVolIDAttrs := azgo.NewVolumeIdAttributesType().SetName("")
	desiredVolumeAttributes := azgo.NewVolumeAttributesType().SetVolumeIdAttributes(*desiredVolIDAttrs)
	desiredAttributes.SetVolumeAttributes(*desiredVolumeAttributes)

	response, err := azgo.NewVolumeGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)
	return *response, err
}

// VolumeGetRootName gets the name of the root volume of a vserver
func (d Client) VolumeGetRootName() (azgo.VolumeGetRootNameResponse, error) {
	response, err := azgo.NewVolumeGetRootNameRequest().
		ExecuteUsing(d.zr)
	return *response, err
}

// VOLUME operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// QTREE operations BEGIN

// QtreeCreate creates a qtree with the specified options
// equivalent to filer::> qtree create -vserver ndvp_vs -volume v -qtree q -export-policy default -unix-permissions ---rwxr-xr-x -security-style unix
func (d Client) QtreeCreate(name, volumeName, unixPermissions, exportPolicy,
	securityStyle string) (azgo.QtreeCreateResponse, error) {
	response, err := azgo.NewQtreeCreateRequest().
		SetQtree(name).
		SetVolume(volumeName).
		SetMode(unixPermissions).
		SetSecurityStyle(securityStyle).
		SetExportPolicy(exportPolicy).
		ExecuteUsing(d.zr)
	return *response, err
}

// QtreeRename renames a qtree
// equivalent to filer::> volume qtree rename
func (d Client) QtreeRename(path, newPath string) (azgo.QtreeRenameResponse, error) {
	response, err := azgo.NewQtreeRenameRequest().
		SetQtree(path).
		SetNewQtreeName(newPath).
		ExecuteUsing(d.zr)
	return *response, err
}

// QtreeDestroyAsync destroys a qtree in the background
// equivalent to filer::> volume qtree delete -foreground false
func (d Client) QtreeDestroyAsync(path string, force bool) (azgo.QtreeDeleteAsyncResponse, error) {
	response, err := azgo.NewQtreeDeleteAsyncRequest().
		SetQtree(path).
		SetForce(force).
		ExecuteUsing(d.zr)
	return *response, err
}

// QtreeList returns the names of all Qtrees whose names match the supplied prefix
// equivalent to filer::> volume qtree show
func (d Client) QtreeList(prefix, volumePrefix string) (azgo.QtreeListIterResponse, error) {

	// Limit the qtrees to those matching the Flexvol and Qtree name prefixes
	query := &azgo.QtreeListIterRequestQuery{}
	queryInfo := azgo.NewQtreeInfoType().SetVolume(volumePrefix + "*").SetQtree(prefix + "*")
	query.SetQtreeInfo(*queryInfo)

	// Limit the returned data to only the Flexvol and Qtree names
	desiredAttributes := &azgo.QtreeListIterRequestDesiredAttributes{}
	desiredInfo := azgo.NewQtreeInfoType().SetVolume("").SetQtree("")
	desiredAttributes.SetQtreeInfo(*desiredInfo)

	response, err := azgo.NewQtreeListIterRequest().
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)
	return *response, err
}

// QtreeCount returns the number of Qtrees in the specified Flexvol, not including the Flexvol itself
func (d Client) QtreeCount(volume string) (int, error) {

	// Limit the qtrees to those in the specified Flexvol
	query := &azgo.QtreeListIterRequestQuery{}
	queryInfo := azgo.NewQtreeInfoType().SetVolume(volume)
	query.SetQtreeInfo(*queryInfo)

	// Limit the returned data to only the Flexvol and Qtree names
	desiredAttributes := &azgo.QtreeListIterRequestDesiredAttributes{}
	desiredInfo := azgo.NewQtreeInfoType().SetVolume("").SetQtree("")
	desiredAttributes.SetQtreeInfo(*desiredInfo)

	response, err := azgo.NewQtreeListIterRequest().
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)

	if err = GetError(response, err); err != nil {
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
func (d Client) QtreeExists(name, volumePrefix string) (bool, string, error) {

	// Limit the qtrees to those matching the Flexvol and Qtree name prefixes
	query := &azgo.QtreeListIterRequestQuery{}
	queryInfo := azgo.NewQtreeInfoType().SetVolume(volumePrefix + "*").SetQtree(name)
	query.SetQtreeInfo(*queryInfo)

	// Limit the returned data to only the Flexvol and Qtree names
	desiredAttributes := &azgo.QtreeListIterRequestDesiredAttributes{}
	desiredInfo := azgo.NewQtreeInfoType().SetVolume("").SetQtree("")
	desiredAttributes.SetQtreeInfo(*desiredInfo)

	response, err := azgo.NewQtreeListIterRequest().
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)

	// Ensure the API call succeeded
	if err = GetError(response, err); err != nil {
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
func (d Client) QtreeGet(name, volumePrefix string) (azgo.QtreeInfoType, error) {

	// Limit the qtrees to those matching the Flexvol and Qtree name prefixes
	query := &azgo.QtreeListIterRequestQuery{}
	info := azgo.NewQtreeInfoType().SetVolume(volumePrefix + "*").SetQtree(name)
	query.SetQtreeInfo(*info)

	response, err := azgo.NewQtreeListIterRequest().
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		ExecuteUsing(d.zr)

	if err != nil {
		return azgo.QtreeInfoType{}, err
	} else if response.Result.NumRecords() == 0 {
		return azgo.QtreeInfoType{}, fmt.Errorf("qtree %s not found", name)
	} else if response.Result.NumRecords() > 1 {
		return azgo.QtreeInfoType{}, fmt.Errorf("more than one qtree %s found", name)
	} else if response.Result.AttributesListPtr == nil {
		return azgo.QtreeInfoType{}, fmt.Errorf("qtree %s not found", name)
	}

	return response.Result.AttributesListPtr.QtreeInfoPtr[0], nil
}

// QtreeGetAll returns all relevant details for all qtrees whose Flexvol names match the supplied prefix
// equivalent to filer::> volume qtree show
func (d Client) QtreeGetAll(volumePrefix string) (azgo.QtreeListIterResponse, error) {

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
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)
	return *response, err
}

// QuotaOn enables quotas on a Flexvol
// equivalent to filer::> volume quota on
func (d Client) QuotaOn(volume string) (azgo.QuotaOnResponse, error) {
	response, err := azgo.NewQuotaOnRequest().
		SetVolume(volume).
		ExecuteUsing(d.zr)
	return *response, err
}

// QuotaOff disables quotas on a Flexvol
// equivalent to filer::> volume quota off
func (d Client) QuotaOff(volume string) (azgo.QuotaOffResponse, error) {
	response, err := azgo.NewQuotaOffRequest().
		SetVolume(volume).
		ExecuteUsing(d.zr)
	return *response, err
}

// QuotaResize resizes quotas on a Flexvol
// equivalent to filer::> volume quota resize
func (d Client) QuotaResize(volume string) (azgo.QuotaResizeResponse, error) {
	response, err := azgo.NewQuotaResizeRequest().
		SetVolume(volume).
		ExecuteUsing(d.zr)
	return *response, err
}

// QuotaStatus returns the quota status for a Flexvol
// equivalent to filer::> volume quota show
func (d Client) QuotaStatus(volume string) (azgo.QuotaStatusResponse, error) {
	response, err := azgo.NewQuotaStatusRequest().
		SetVolume(volume).
		ExecuteUsing(d.zr)
	return *response, err
}

// QuotaSetEntry creates a new quota rule with an optional hard disk limit
// equivalent to filer::> volume quota policy rule create
func (d Client) QuotaSetEntry(qtreeName, volumeName, quotaTarget, quotaType, diskLimit string) (azgo.QuotaSetEntryResponse, error) {

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
	return *response, err
}

// QuotaEntryGet returns the disk limit for a single qtree
// equivalent to filer::> volume quota policy rule show
func (d Client) QuotaGetEntry(target string) (azgo.QuotaEntryType, error) {

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
		return azgo.QuotaEntryType{}, err
	} else if response.Result.NumRecords() == 0 {
		return azgo.QuotaEntryType{}, fmt.Errorf("tree quota for %s not found", target)
	} else if response.Result.NumRecords() > 1 {
		return azgo.QuotaEntryType{}, fmt.Errorf("more than one tree quota for %s found", target)
	} else if response.Result.AttributesListPtr == nil {
		return azgo.QuotaEntryType{}, fmt.Errorf("tree quota for %s not found", target)
	}

	return response.Result.AttributesListPtr.QuotaEntryPtr[0], nil
}

// QuotaEntryList returns the disk limit quotas for a Flexvol
// equivalent to filer::> volume quota policy rule show
func (d Client) QuotaEntryList(volume string) (azgo.QuotaListEntriesIterResponse, error) {
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
	return *response, err
}

// QTREE operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// EXPORT POLICY operations BEGIN

// ExportPolicyCreate creates an export policy
// equivalent to filer::> vserver export-policy create
func (d Client) ExportPolicyCreate(policy string) (azgo.ExportPolicyCreateResponse, error) {
	response, err := azgo.NewExportPolicyCreateRequest().
		SetPolicyName(azgo.ExportPolicyNameType(policy)).
		ExecuteUsing(d.zr)
	return *response, err
}

// ExportRuleCreate creates a rule in an export policy
// equivalent to filer::> vserver export-policy rule create
func (d Client) ExportRuleCreate(
	policy, clientMatch string,
	protocols, roSecFlavors, rwSecFlavors, suSecFlavors []string,
) (azgo.ExportRuleCreateResponse, error) {

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
	return *response, err
}

// ExportRuleGetIterRequest returns the export rules in an export policy
// equivalent to filer::> vserver export-policy rule show
func (d Client) ExportRuleGetIterRequest(policy string) (azgo.ExportRuleGetIterResponse, error) {

	// Limit the qtrees to those matching the Flexvol and Qtree name prefixes
	query := &azgo.ExportRuleGetIterRequestQuery{}
	exportRuleInfo := azgo.NewExportRuleInfoType().SetPolicyName(azgo.ExportPolicyNameType(policy))
	query.SetExportRuleInfo(*exportRuleInfo)

	response, err := azgo.NewExportRuleGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		ExecuteUsing(d.zr)
	return *response, err
}

// EXPORT POLICY operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// SNAPSHOT operations BEGIN

// SnapshotCreate creates a snapshot of a volume
func (d Client) SnapshotCreate(name, volumeName string) (azgo.SnapshotCreateResponse, error) {
	response, err := azgo.NewSnapshotCreateRequest().
		SetSnapshot(name).
		SetVolume(volumeName).
		ExecuteUsing(d.zr)
	return *response, err
}

// SnapshotGetByVolume returns the list of snapshots associated with a volume
func (d Client) SnapshotGetByVolume(volumeName string) (azgo.SnapshotGetIterResponse, error) {
	query := &azgo.SnapshotGetIterRequestQuery{}
	snapshotInfo := azgo.NewSnapshotInfoType().SetVolume(volumeName)
	query.SetSnapshotInfo(*snapshotInfo)

	response, err := azgo.NewSnapshotGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		SetQuery(*query).
		ExecuteUsing(d.zr)
	return *response, err
}

// SNAPSHOT operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// ISCSI operations BEGIN

// IscsiServiceGetIterRequest returns information about an iSCSI target
func (d Client) IscsiServiceGetIterRequest() (azgo.IscsiServiceGetIterResponse, error) {
	response, err := azgo.NewIscsiServiceGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		ExecuteUsing(d.zr)
	return *response, err
}

// IscsiNodeGetNameRequest gets the IQN of the vserver
func (d Client) IscsiNodeGetNameRequest() (azgo.IscsiNodeGetNameResponse, error) {
	response, err := azgo.NewIscsiNodeGetNameRequest().ExecuteUsing(d.zr)
	return *response, err
}

// IscsiInterfaceGetIterRequest returns information about the vserver's iSCSI interfaces
func (d Client) IscsiInterfaceGetIterRequest() (azgo.IscsiInterfaceGetIterResponse, error) {
	response, err := azgo.NewIscsiInterfaceGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		ExecuteUsing(d.zr)
	return *response, err
}

// ISCSI operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// VSERVER operations BEGIN

// VserverGetIterRequest returns the vservers on the system
// equivalent to filer::> vserver show
func (d Client) VserverGetIterRequest() (azgo.VserverGetIterResponse, error) {
	response, err := azgo.NewVserverGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		ExecuteUsing(d.zr)
	return *response, err
}

// VserverGetIterAdminRequest returns vservers of type "admin" on the system.
// equivalent to filer::> vserver show -type admin
func (d Client) VserverGetIterAdminRequest() (azgo.VserverGetIterResponse, error) {
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
	return *response, err
}

// VserverGetRequest returns vserver to which it is sent
// equivalent to filer::> vserver show
func (d Client) VserverGetRequest() (azgo.VserverGetResponse, error) {
	response, err := azgo.NewVserverGetRequest().ExecuteUsing(d.zr)
	return *response, err
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
			aggrList := vserver.VserverAggrInfoList().VserverAggrInfoPtr
			for _, aggr := range aggrList {
				aggrNames = append(aggrNames, string(aggr.AggrName()))
			}
		}
	}

	return aggrNames, nil
}

// VserverShowAggrGetIterRequest returns the aggregates on the vserver.  Requires ONTAP 9 or later.
// equivalent to filer::> vserver show-aggregates
func (d Client) VserverShowAggrGetIterRequest() (azgo.VserverShowAggrGetIterResponse, error) {

	response, err := azgo.NewVserverShowAggrGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		ExecuteUsing(d.zr)
	return *response, err
}

// VSERVER operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// AGGREGATE operations BEGIN

// AggrGetIterRequest returns the aggregates on the system
// equivalent to filer::> storage aggregate show
func (d Client) AggrGetIterRequest() (azgo.AggrGetIterResponse, error) {

	// If we tunnel to an SVM, which is the default case, this API will never work.
	// It will still fail if the non-tunneled ZapiRunner addresses a vserver management LIF,
	// but that possibility must be handled by the caller.
	zr := d.GetNontunneledZapiRunner()

	response, err := azgo.NewAggrGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		ExecuteUsing(zr)
	return *response, err
}

// AggrSpaceGetIterRequest returns the aggregates on the system
// equivalent to filer::> storage aggregate show-space -aggregate-name aggregate
func (d Client) AggrSpaceGetIterRequest(aggregateName string) (azgo.AggrSpaceGetIterResponse, error) {
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
	return *responseAggrSpace, err
}

func (d Client) getAggregateSize(aggregateName string) (result int, err error) {
	// First, lookup the aggregate and it's space used
	aggregateSizeTotal := NumericalValueNotSet

	responseAggrSpace, error := d.AggrSpaceGetIterRequest(aggregateName)
	if error = GetError(responseAggrSpace, error); error != nil {
		return NumericalValueNotSet, fmt.Errorf("error getting size for Aggregate %v: %v", aggregateName, error)
	}

	if responseAggrSpace.Result.AttributesListPtr != nil {
		for _, aggrSpace := range responseAggrSpace.Result.AttributesListPtr.SpaceInformationPtr {
			aggregateSizeTotal = aggrSpace.AggregateSize()
			return aggregateSizeTotal, nil
		}
	}

	return aggregateSizeTotal, fmt.Errorf("error getting size for Aggregate %v", aggregateName)
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
func (d Client) AggregateCommitment(aggregate string) (result *AggregateCommitment, err error) {

	zr := d.GetNontunneledZapiRunner()

	// first, get the aggregate's size
	aggregateSize, error := d.getAggregateSize(aggregate)
	if error != nil {
		return nil, error
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
	if err = GetError(response, err); err != nil {
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

			log.WithFields(log.Fields{
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
			if lunsResponseErr = GetError(lunsResponse, lunsResponseErr); lunsResponseErr != nil {
				return nil, fmt.Errorf("error enumerating LUNs for volume %v: %v", volName, lunsResponseErr)
			}

			if lunsResponse.Result.AttributesListPtr != nil &&
				lunsResponse.Result.AttributesListPtr.LunInfoPtr != nil {
				for _, lun := range lunsResponse.Result.AttributesListPtr.LunInfoPtr {
					lunPath := lun.Path()
					lunSize := lun.Size()
					log.WithFields(log.Fields{
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

// SnapmirrorGetLoadSharingMirrors gets load-sharing SnapMirror relationships for a volume
// equivalent to filer::> snapmirror -type LS -source-volume
func (d Client) SnapmirrorGetLoadSharingMirrors(volume string) (azgo.SnapmirrorGetIterResponse, error) {

	// Limit the mirrors to load-sharing mirrors matching the source FlexVol
	query := &azgo.SnapmirrorGetIterRequestQuery{}
	info := azgo.NewSnapmirrorInfoType().SetSourceVolume(volume).SetRelationshipType("load_sharing")
	query.SetSnapmirrorInfo(*info)

	// Limit the returned data to only the source location
	desiredAttributes := &azgo.SnapmirrorGetIterRequestDesiredAttributes{}
	desiredInfo := azgo.NewSnapmirrorInfoType().SetSourceLocation("").SetRelationshipStatus("")
	desiredAttributes.SetSnapmirrorInfo(*desiredInfo)

	response, err := azgo.NewSnapmirrorGetIterRequest().
		SetQuery(*query).
		SetDesiredAttributes(*desiredAttributes).
		ExecuteUsing(d.zr)
	return *response, err
}

// SnapmirrorUpdateLoadSharingMirrors updates the destination volumes of a set of load-sharing mirrors
// equivalent to filer::> snapmirror update-ls-set -source-path
func (d Client) SnapmirrorUpdateLoadSharingMirrors(
	sourceLocation string,
) (azgo.SnapmirrorUpdateLsSetResponse, error) {

	response, err := azgo.NewSnapmirrorUpdateLsSetRequest().
		SetSourceLocation(sourceLocation).
		ExecuteUsing(d.zr)
	return *response, err
}

// SNAPMIRROR operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// MISC operations BEGIN

// NetInterfaceGet returns the list of network interfaces with associated metadata
// equivalent to filer::> net interface list
func (d Client) NetInterfaceGet() (azgo.NetInterfaceGetIterResponse, error) {
	response, err := azgo.NewNetInterfaceGetIterRequest().
		SetMaxRecords(defaultZapiRecords).
		ExecuteUsing(d.zr)
	return *response, err
}

func (d Client) NetInterfaceGetDataLIFs(protocol string) ([]string, error) {
	lifResponse, err := d.NetInterfaceGet()
	if err = GetError(lifResponse, err); err != nil {
		return nil, fmt.Errorf("error checking network interfaces: %v", err)
	}

	dataLIFs := make([]string, 0)
	if lifResponse.Result.AttributesListPtr != nil {
		for _, attrs := range lifResponse.Result.AttributesListPtr.NetInterfaceInfoPtr {
			for _, proto := range attrs.DataProtocols().DataProtocolPtr {
				if proto == azgo.DataProtocolType(protocol) {
					dataLIFs = append(dataLIFs, string(attrs.Address()))
				}
			}
		}
	}

	log.WithField("dataLIFs", dataLIFs).Debug("Data LIFs")
	return dataLIFs, nil
}

// SystemGetVersion returns the system version
// equivalent to filer::> version
func (d Client) SystemGetVersion() (azgo.SystemGetVersionResponse, error) {
	response, err := azgo.NewSystemGetVersionRequest().ExecuteUsing(d.zr)
	return *response, err
}

// SystemGetOntapiVersion gets the ONTAPI version using the credentials, and caches & returns the result.
func (d Client) SystemGetOntapiVersion() (string, error) {

	if d.zr.OntapiVersion == "" {
		result, err := azgo.NewSystemGetOntapiVersionRequest().ExecuteUsing(d.zr)
		if err = GetError(result, err); err != nil {
			return "", fmt.Errorf("could not read ONTAPI version: %v", err)
		}

		major := result.Result.MajorVersion()
		minor := result.Result.MinorVersion()
		d.zr.OntapiVersion = fmt.Sprintf("%d.%d", major, minor)
	}

	return d.zr.OntapiVersion, nil
}

func (d Client) NodeListSerialNumbers() ([]string, error) {

	serialNumbers := make([]string, 0, 0)
	zr := d.GetNontunneledZapiRunner()

	// Limit the returned data to only the serial numbers
	desiredAttributes := &azgo.SystemNodeGetIterRequestDesiredAttributes{}
	info := azgo.NewNodeDetailsInfoType().SetNodeSerialNumber("")
	desiredAttributes.SetNodeDetailsInfo(*info)

	response, err := azgo.NewSystemNodeGetIterRequest().
		SetDesiredAttributes(*desiredAttributes).
		SetMaxRecords(defaultZapiRecords).
		ExecuteUsing(zr)

	log.WithFields(log.Fields{
		"response":          response,
		"info":              info,
		"desiredAttributes": desiredAttributes,
		"err":               err,
	}).Debug("NodeListSerialNumbers")

	if err = GetError(response, err); err != nil {
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

	log.WithFields(log.Fields{
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
	logLevel int) (azgo.EmsAutosupportLogResponse, error) {

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
	return *response, err
}

// MISC operations END
/////////////////////////////////////////////////////////////////////////////
