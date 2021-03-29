// Copyright 2020 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	runtime "github.com/go-openapi/runtime"
	runtime_client "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	log "github.com/sirupsen/logrus"

	. "github.com/netapp/trident/logger"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/cluster"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/networking"
	san "github.com/netapp/trident/storage_drivers/ontap/api/rest/client/s_a_n"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/storage"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/svm"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/netapp/trident/utils"
)

// TODO fix all params.Context = context.TODO()

////////////////////////////////////////////////////////////////////////////////////////////
/// https://confluence.ngage.netapp.com/display/TRID/ONTAP+REST+abstraction+layer
////////////////////////////////////////////////////////////////////////////////////////////

type IgroupSpec struct {
	Name  string
	Group string
	OS    string
}

type IgroupAPI interface {
	IgroupCreate(IgroupSpec) (Response, error)
	IgroupAdd(IgroupSpec, string) (Response, error)
	IgroupRemove(IgroupSpec, string, bool) (Response, error)
	IgroupDestroy(IgroupSpec) (Response, error)
	IgroupInfo(int, string) (Iterator, error)
}

type LunSpec struct {
	Path      string
	Size      int
	OS        string
	Qos       QosPolicyGroup
	Reserved  bool
	Allocated bool
}

type ByCriteria int

const (
	ByName ByCriteria = iota + 1
	ByPattern
	ByVolume
	ByVserver
	BySnapshot
	ByAttrs
)

type LunAPI interface {
	LunCreate(LunSpec) (Response, error)
	LunClone(string, string, string, QosPolicyGroup) (Response, error)
	LunSerialNumber(string) (Response, error)
	LunMap(string, string) (Iterator, error)
	LunMapAutoID(string, string) (Iterator, error)
	LunMapIfNotMapped(context.Context, string, string, bool) (int, error)
	LunMapInfo(Response, error)
	LunOffline(string) (Response, error)
	LunOnline(string) (Response, error)
	LunDestroy(string) (Response, error)
	LunSetAttribute(string, string, string) (Response, error)
	LunGetAttribute(string, string) (Response, error)
	LunGeometry(string) (Response, error)
	LunResize(string, int) (Response, error)
	LunInfo(int, string) (Response, error)
	LunCount(context.Context, string) (int, error)
	LunRename(string, string) (Response, error)
	LunUnmap(string, string) (Response, error)
}

type FlexGroupSpec struct {
	Name            string
	Size            int
	Aggrs           []string
	SpaceReserve    string
	SnapshotPolicy  string
	UnixPermissions string
	ExportPolicy    string
	SecurityStyle   string
	TieringPolicy   string
	Comment         string
	Qos             QosPolicyGroup
	Encrypt         bool
	SnapshotReserve int
}

type FlexGroupAPI interface {
	FlexGroupCreate(context.Context, FlexGroupSpec) (Response, error)
	FlexGroupDestroy(context.Context, string, bool) (Response, error)
	FlexGroupExists(context.Context, string) (bool, error)
	FlexGroupSize(string) (Response, error)
	FlexGroupSetSize(context.Context, string, string) (Response, error)
	FlexGroupVolumeDisableSnapshotDirectoryAccess(context.Context, string, string) (Response, error)
	FlexGroupModifyUnixPermissions(context.Context, string, string) (Response, error)
	FlexGroupSetComment(context.Context, string, string) (Response, error)
	FlexGroupInfo(int, string) (Iterator, error)
}

type VolumeSpec struct {
	Name            string
	AggregateName   string
	Size            string
	SpaceReserve    string
	SnapshotPolicy  string
	UnixPermissions string
	ExportPolicy    string
	SecurityStyle   string
	TieringPolicy   string
	Comment         string
	Qos             QosPolicyGroup
	Encrypt         bool
	SnapshotReserve int
}

type VolumeAPI interface {
	VolumeCreate(context.Context, VolumeSpec) (Response, error)
	VolumeModifyExportPolicy(string, string) (Response, error)
	VolumeModifyUnixPermissions(string, string) (Response, error)
	VolumeClone(string, string, string) (Response, error)
	VolumeCloneAsync(string, string, string) (Response, error)
	VolumeCloneSplit(string) (Response, error)
	VolumeDisableSnapshotDirAccess(string) (Response, error)
	VolumeSetQos(string, QosPolicyGroup) (Response, error)
	VolumeExists(string) (bool, error)
	VolumeSize(string) (int, error)
	VolumeSetSize(string, string) (Response, error)
	VolumeMount(string, string) (Response, error)
	VolumeUnmount(string, bool) (Response, error)
	VolumeOffline(string) (Response, error)
	VolumeDestroy(string, bool) (Response, error)
	VolumeInfo(int, string) (VolumeIterator, error)
	VolumeResize(string, string) (Response, error)
	VolumeSetComment(context.Context, string, string) (Response, error)
}

type QtreeSpec struct {
	Name            string
	VolumeName      string
	UnixPermissions string
	ExportPolicy    string
	SecurityStyle   string
	QosPolicy       string
}

type QtreeAPI interface {
	QtreeCreate(QtreeSpec) (Response, error)
	QtreeRename(string, string) (Response, error)
	QtreeDestroyAsync(string bool) (Response, error)
	QtreeCount(context.Context, string) (int, error)
	QtreeExists(context.Context, string, string) (bool, error)
	QtreeInfo(int, string) (Iterator, error)
	QtreeSetExportPolicy(string, string, string) (Response, error)
}

type OntapAPI interface {
	// TODO PUT BACK VV
	// SupportsFeature(context.Context, string) bool
	// NewQosPolicyGroup(string, string) (QosPolicyGroup, error)
	// IgroupAPI
	// LunAPI
	// FlexGroupAPI
	// VolumeAPI
	// QtreeAPI
	// TODO PUT BACK ^^
	VolumeCreate(context.Context, VolumeSpec) (*VolumeResponse, error)
	VolumeInfo(context.Context, int, string) (*VolumeIterator, error)
	VolumeDestroy(string, bool) (*VolumeResponse, error)
}

type Response interface {
	Name() string
	Version() string
	Status() string
	Reason() string
	Errno() string
	Data() []interface{}
}

type VolumeResponse struct {
	name    string
	version string
	status  string
	reason  string
	errno   string
	data    []interface{}
}

func (o VolumeResponse) Name() string {
	return o.name
}

func (o VolumeResponse) Version() string {
	return o.version
}

func (o VolumeResponse) Status() string {
	return o.status
}

func (o VolumeResponse) Reason() string {
	return o.reason
}

func (o VolumeResponse) Errno() string {
	return o.errno
}

func (o VolumeResponse) Data() []interface{} {
	return o.data
}

type Iterator interface {
	Response
	Next() Iterator
}

type VolumeIterator interface {
	Iterator
}

////////////////////////////////////////////////////////////////////////////////////////////////////////
// REST layer
////////////////////////////////////////////////////////////////////////////////////////////////////////

// ToStringPointer TBD
func ToStringPointer(s string) *string {
	return &s
}

// ToBoolPointer TBD
func ToBoolPointer(b bool) *bool {
	return &b
}

func init() {
	// TODO move this around, rethink this
	//os.Setenv("SWAGGER_DEBUG", "true")
	//os.Setenv("DEBUG", "true")
}

// RestClient TBD
type RestClient struct {
	config       ClientConfig
	tr           *http.Transport
	httpClient   *http.Client
	api          *client.ONTAPRESTAPI
	authInfo     runtime.ClientAuthInfoWriter
	OntapVersion string
}

// NewRestClient is a factory method for creating a new instance
func NewRestClient(ctx context.Context, config ClientConfig) (*RestClient, error) {
	result := &RestClient{
		config: config,
	}

	result.tr = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	result.httpClient = &http.Client{
		Transport: result.tr,
		Timeout:   time.Duration(60 * time.Second),
	}

	formats := strfmt.Default

	transportConfig := client.DefaultTransportConfig()
	transportConfig.Host = config.ManagementLIF

	result.api = client.NewHTTPClientWithConfig(formats, transportConfig)

	result.authInfo = runtime_client.BasicAuth(config.Username, config.Password)

	return result, nil
}

// NewRestClient is a factory method for creating a new instance
func NewRestClientFromOntapConfig(ctx context.Context, ontapConfig *drivers.OntapStorageDriverConfig) (*RestClient, error) {

	config := ClientConfig{
		ManagementLIF:        ontapConfig.ManagementLIF,
		SVM:                  ontapConfig.SVM,
		Username:             ontapConfig.Username,
		Password:             ontapConfig.Password,
		ClientPrivateKey:     ontapConfig.ClientPrivateKey,
		ClientCertificate:    ontapConfig.ClientCertificate,
		TrustedCACertificate: ontapConfig.TrustedCACertificate,
		DriverContext:        ontapConfig.DriverContext,
		DebugTraceFlags:      ontapConfig.DebugTraceFlags,
	}
	return NewRestClient(ctx, config)
}

//////////////////////////////////////////////////////////////////////////////
// VOLUME operations
//////////////////////////////////////////////////////////////////////////////

// VolumeList returns the names of all Flexvols whose names match the supplied pattern
func (d *RestClient) VolumeList(pattern string) (*storage.VolumeCollectionGetOK, error) {
	params := storage.NewVolumeCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = context.TODO()
	params.HTTPClient = d.httpClient
	params.ReturnRecords = ToBoolPointer(true)

	params.SVMNameQueryParameter = &d.config.SVM
	params.SetNameQueryParameter(ToStringPointer(pattern))
	params.SetFields([]string{"**"}) // TODO trim these down to just what we need

	return d.api.Storage.VolumeCollectionGet(params, d.authInfo)
}

// VolumeCreate creates a volume with the specified options
// equivalent to filer::> volume create -vserver iscsi_vs -volume v -aggregate aggr1 -size 1g -state online -type RW -policy default -unix-permissions ---rwxr-xr-x -space-guarantee none -snapshot-policy none -security-style unix -encrypt false
func (d *RestClient) VolumeCreate(
	ctx context.Context, name, aggregateName, size, spaceReserve, snapshotPolicy, unixPermissions,
	exportPolicy, securityStyle, tieringPolicy, comment string, qosPolicyGroup QosPolicyGroup, encrypt bool,
	snapshotReserve int,
) (*storage.VolumeCreateAccepted, error) {

	// TODO handle these
	// spaceReserve := spec.SpaceReserve
	// unixPermissions := spec.UnixPermissions
	// exportPolicy := spec.ExportPolicy
	// securityStyle := spec.SecurityStyle
	// tieringPolicy := spec.TieringPolicy
	// qosPolicyGroup := spec.Qos
	// encrypt := spec.Encrypt
	// snapshotReserve := spec.SnapshotReserve

	params := storage.NewVolumeCreateParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.ReturnRecords = ToBoolPointer(true)

	sizeBytesStr, _ := utils.ConvertSizeToBytes(size)
	sizeBytes, _ := strconv.ParseUint(sizeBytesStr, 10, 64)

	volumeInfo := &models.Volume{
		Name: name,
		Aggregates: []*models.VolumeAggregatesItems0{
			{Name: aggregateName},
		},
		Size: int64(sizeBytes),
		//		Guarantee:      &models.VolumeGuarantee{Honored: ToBoolPointer(false)},
		SnapshotPolicy: &models.VolumeSnapshotPolicy{Name: snapshotPolicy},
		Comment:        ToStringPointer(comment),
	}
	volumeInfo.Svm = &models.VolumeSvm{Name: d.config.SVM}

	params.SetInfo(volumeInfo)

	return d.api.Storage.VolumeCreate(params, d.authInfo)
}

// VolumeDelete deletes the volume with the specified uuid
func (d RestClient) VolumeDelete(
	ctx context.Context, uuid string,
) (*storage.VolumeDeleteAccepted, error) {

	params := storage.NewVolumeDeleteParamsWithTimeout(d.httpClient.Timeout)
	params.Context = context.TODO()
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = uuid

	return d.api.Storage.VolumeDelete(params, d.authInfo)
}

// VolumeGet gets the volume with the specified uuid
func (d RestClient) VolumeGet(
	ctx context.Context, uuid string,
) (*storage.VolumeGetOK, error) {

	params := storage.NewVolumeGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = context.TODO()
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = uuid

	return d.api.Storage.VolumeGet(params, d.authInfo)
}

// VolumeExists tests for the existence of a Flexvol
func (d RestClient) VolumeExists(ctx context.Context, volumeName string) (bool, error) {
	// TODO validate/improve this logic?
	volume, err := d.VolumeGetByName(ctx, volumeName)
	if err != nil {
		return false, err
	}
	if volume == nil {
		return false, err
	}
	return true, nil
}

// VolumeGetByName gets the volume with the specified uuid
func (d RestClient) VolumeGetByName(
	ctx context.Context, volumeName string,
) (*models.Volume, error) {

	// TODO validate/improve this logic?
	result, err := d.VolumeList(volumeName)
	if result.Payload.NumRecords == 1 && result.Payload.Records != nil {
		return result.Payload.Records[0], nil
	}
	return nil, err
}

// VolumeMount mounts a volume at the specified junction
func (d RestClient) VolumeMount(ctx context.Context, volumeName, junctionPath string) error {

	volume, err := d.VolumeGetByName(ctx, volumeName)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}

	uuid := volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = uuid

	volumeInfo := &models.Volume{
		Nas: &models.VolumeNas{Path: junctionPath},
	}
	params.SetInfo(volumeInfo)

	// TODO validate/improve this logic? we get back a job and should check its status
	volumeModifyAccepted, err := d.api.Storage.VolumeModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	// check job status
	isDone := false
	for !isDone {
		isDone, err = d.IsJobFinished(volumeModifyAccepted.Payload)
		if err != nil {
			isDone = true
			return fmt.Errorf("error mounting: %v", err)
		}
		if !isDone {
			Logc(ctx).Debug("Sleeping")
			time.Sleep(5 * time.Second) // TODO use backoff retry? make a helper function?
		}
	}

	return nil
}

// VolumeRename changes the name of a FlexVol (but not a FlexGroup!)
func (d RestClient) VolumeRename(ctx context.Context, volumeName, newVolumeName string) error {

	volume, err := d.VolumeGetByName(ctx, volumeName)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}

	uuid := volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = uuid

	volumeInfo := &models.Volume{
		Name: newVolumeName,
	}

	params.SetInfo(volumeInfo)

	// TODO validate/improve this logic?
	volumeModifyAccepted, err := d.api.Storage.VolumeModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	// check job status
	isDone := false
	for !isDone {
		isDone, err = d.IsJobFinished(volumeModifyAccepted.Payload)
		if err != nil {
			isDone = true
			return fmt.Errorf("error renaming: %v", err)
		}
		if !isDone {
			Logc(ctx).Debug("Sleeping")
			time.Sleep(5 * time.Second) // TODO use backoff retry? make a helper function?
		}
	}

	return nil
}

// VolumeSize retrieves the size of the specified volume
func (d RestClient) VolumeSize(volumeName string) (int, error) {
	ctx := context.TODO()

	volume, err := d.VolumeGetByName(ctx, volumeName)
	if err != nil {
		return 0, err
	}
	if volume == nil {
		return 0, fmt.Errorf("could not find volume with name %v", volumeName)
	}

	// TODO validate/improve this logic? int64 vs int
	return int(volume.Size), nil
}

// VolumeSetSize sets the size of the specified volume
func (d RestClient) VolumeSetSize(ctx context.Context, volumeName, newSize string) error {

	volume, err := d.VolumeGetByName(ctx, volumeName)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}

	uuid := volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = uuid

	sizeBytesStr, _ := utils.ConvertSizeToBytes(newSize)
	sizeBytes, _ := strconv.ParseUint(sizeBytesStr, 10, 64)

	volumeInfo := &models.Volume{
		Size: int64(sizeBytes),
	}

	params.SetInfo(volumeInfo)

	// TODO validate/improve this logic?
	volumeModifyAccepted, err := d.api.Storage.VolumeModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	// check job status
	isDone := false
	for !isDone {
		isDone, err = d.IsJobFinished(volumeModifyAccepted.Payload)
		if err != nil {
			isDone = true
			return fmt.Errorf("error setting size: %v", err)
		}
		if !isDone {
			Logc(ctx).Debug("Sleeping")
			time.Sleep(5 * time.Second) // TODO use backoff retry? make a helper function?
		}
	}

	return nil
}

// VolumeSetComment sets a volume's comment to the supplied value
// equivalent to filer::> volume modify -vserver iscsi_vs -volume v -comment newVolumeComment
func (d RestClient) VolumeSetComment(ctx context.Context, volumeName, newVolumeComment string) error {

	volume, err := d.VolumeGetByName(ctx, volumeName)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}

	uuid := volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = uuid

	volumeInfo := &models.Volume{
		Comment: ToStringPointer(newVolumeComment),
	}

	params.SetInfo(volumeInfo)

	// TODO validate/improve this logic?
	volumeModifyAccepted, err := d.api.Storage.VolumeModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	// check job status
	isDone := false
	for !isDone {
		isDone, err = d.IsJobFinished(volumeModifyAccepted.Payload)
		if err != nil {
			isDone = true
			return fmt.Errorf("error setting comment: %v", err)
		}
		if !isDone {
			Logc(ctx).Debug("Sleeping")
			time.Sleep(5 * time.Second) // TODO use backoff retry? make a helper function?
		}
	}

	return nil
}

//////////////////////////////////////////////////////////////////////////////
// SNAPSHOT operations
//////////////////////////////////////////////////////////////////////////////

// SnapshotCreate creates a snapshot
//func (d *Client) SnapshotCreate(volumeUUID, name, comment string) (*storage.SnapshotCreateAccepted, error) {
func (d *RestClient) SnapshotCreate(volumeUUID, snapshotName string) (*storage.SnapshotCreateAccepted, error) {
	// TODO this one and the ZAPI one are different in the parameter list, need to resolve

	params := storage.NewSnapshotCreateParamsWithTimeout(d.httpClient.Timeout)
	params.Context = context.TODO()
	params.HTTPClient = d.httpClient
	params.ReturnRecords = ToBoolPointer(true)

	params.VolumeUUIDPathParameter = volumeUUID

	snapshotInfo := &models.Snapshot{
		Name: snapshotName,
		//Comment: comment,
	}

	snapshotInfo.Svm = &models.SnapshotSvm{Name: d.config.SVM}

	params.SetInfo(snapshotInfo)

	return d.api.Storage.SnapshotCreate(params, d.authInfo)
}

// SnapshotCreateAndWait creates a snapshot and waits on the job to complete
//func (d *Client) SnapshotCreate(volumeUUID, name, comment string) (*storage.SnapshotCreateAccepted, error) {
func (d *RestClient) SnapshotCreateAndWait(ctx context.Context, volumeUUID, snapshotName string) error {
	snapshotCreateResult, err := d.SnapshotCreate(volumeUUID, snapshotName)
	if err != nil {
		return fmt.Errorf("could not create snapshot: %v", err)
	}
	if snapshotCreateResult == nil {
		return fmt.Errorf("could not create snapshot: %v", "unexpected result")
	}

	isDone := false
	for !isDone {
		isDone, err := d.IsJobFinished(snapshotCreateResult.Payload)
		if err != nil {
			isDone = true
			return fmt.Errorf("could not create snapshot: %v", err)
		}
		if !isDone {
			Logc(ctx).Debug("Sleeping")
			time.Sleep(5 * time.Second) // TODO use backoff retry? make a helper function?
		}
	}
	return nil
}

// SnapshotList lists snapshots
func (d *RestClient) SnapshotList(volumeUUID string) (*storage.SnapshotCollectionGetOK, error) {

	params := storage.NewSnapshotCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = context.TODO()
	params.HTTPClient = d.httpClient
	params.ReturnRecords = ToBoolPointer(true)

	params.VolumeUUIDPathParameter = volumeUUID

	params.SVMNameQueryParameter = ToStringPointer(d.config.SVM)
	params.SetFields([]string{"**"}) // TODO trim these down to just what we need

	return d.api.Storage.SnapshotCollectionGet(params, d.authInfo)
}

// SnapshotListByName lists snapshots by name
func (d *RestClient) SnapshotListByName(volumeUUID, snapshotName string) (*storage.SnapshotCollectionGetOK, error) {

	params := storage.NewSnapshotCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = context.TODO()
	params.HTTPClient = d.httpClient
	params.ReturnRecords = ToBoolPointer(true)

	params.VolumeUUIDPathParameter = volumeUUID
	params.NameQueryParameter = ToStringPointer(snapshotName)

	params.SVMNameQueryParameter = ToStringPointer(d.config.SVM)
	params.SetFields([]string{"**"}) // TODO trim these down to just what we need

	return d.api.Storage.SnapshotCollectionGet(params, d.authInfo)
}

// SnapshotGet returns info on the snapshot
func (d *RestClient) SnapshotGet(volumeUUID, snapshotUUID string) (*storage.SnapshotGetOK, error) {

	params := storage.NewSnapshotGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = context.TODO()
	params.HTTPClient = d.httpClient

	params.VolumeUUIDPathParameter = volumeUUID
	params.UUIDPathParameter = snapshotUUID

	return d.api.Storage.SnapshotGet(params, d.authInfo)
}

// SnapshotGetByName finds the snapshot by name
func (d RestClient) SnapshotGetByName(
	volumeUUID, snapshotName string,
) (*models.Snapshot, error) {

	// TODO improve this
	result, err := d.SnapshotListByName(volumeUUID, snapshotName)
	if result.Payload.NumRecords == 1 && result.Payload.Records != nil {
		return result.Payload.Records[0], nil
	}
	return nil, err
}

// SnapshotDelete deletes a snapshot
func (d *RestClient) SnapshotDelete(volumeUUID, snapshotUUID string) (*storage.SnapshotDeleteAccepted, error) {

	params := storage.NewSnapshotDeleteParamsWithTimeout(d.httpClient.Timeout)
	params.Context = context.TODO()
	params.HTTPClient = d.httpClient

	params.VolumeUUIDPathParameter = volumeUUID
	params.UUIDPathParameter = snapshotUUID

	return d.api.Storage.SnapshotDelete(params, d.authInfo)
}

//////////////////////////////////////////////////////////////////////////////
// CLONE operations
//////////////////////////////////////////////////////////////////////////////

// VolumeCloneCreate creates a clone
// see also: https://library.netapp.com/ecmdocs/ECMLP2858435/html/resources/volume.html#creating-a-flexclone-and-specifying-its-properties-using-post
func (d RestClient) VolumeCloneCreate(
	ctx context.Context, sourceVolumeName, cloneName string,
) (*storage.VolumeCreateAccepted, error) {

	params := storage.NewVolumeCreateParamsWithTimeout(d.httpClient.Timeout)
	params.Context = context.TODO()
	params.HTTPClient = d.httpClient
	params.ReturnRecords = ToBoolPointer(true)

	cloneInfo := &models.VolumeClone{
		ParentVolume: &models.VolumeCloneParentVolume{
			Name: sourceVolumeName,
		},
		IsFlexclone: true,
	}

	volumeInfo := &models.Volume{
		Name:  cloneName,
		Clone: cloneInfo,
	}
	volumeInfo.Svm = &models.VolumeSvm{Name: d.config.SVM}

	params.SetInfo(volumeInfo)

	return d.api.Storage.VolumeCreate(params, d.authInfo)
}

//////////////////////////////////////////////////////////////////////////////
// LUN operations
//////////////////////////////////////////////////////////////////////////////

// LunCreate creates a LUN
func (d RestClient) LunCreate(
	ctx context.Context, lunName, size string,
) (*san.LunCreateCreated, error) {

	params := san.NewLunCreateParamsWithTimeout(d.httpClient.Timeout)
	params.Context = context.TODO()
	params.HTTPClient = d.httpClient
	params.ReturnRecords = ToBoolPointer(true)

	// TODO call the real size helper in utils
	sizeBytesStr, _ := utils.ConvertSizeToBytes(size)
	sizeBytes, _ := strconv.ParseUint(sizeBytesStr, 10, 64)

	lunInfo := &models.Lun{
		Name:   lunName,
		OsType: models.LunOsTypeLinux,
		Space: &models.LunSpace{
			Size: int64(sizeBytes),
		},
	}
	lunInfo.Svm = &models.LunSvm{Name: d.config.SVM}

	params.SetInfo(lunInfo)

	return d.api.San.LunCreate(params, d.authInfo)
}

// LunGet gets the LUN with the specified uuid
func (d RestClient) LunGet(
	ctx context.Context, uuid string,
) (*san.LunGetOK, error) {

	params := san.NewLunGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = context.TODO()
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = uuid

	return d.api.San.LunGet(params, d.authInfo)
}

// LunGetByName gets the LUN with the specified name
func (d RestClient) LunGetByName(
	ctx context.Context, name string,
) (*models.Lun, error) {

	// TODO improve this
	result, err := d.LunList(ctx, name)
	if result.Payload.NumRecords == 1 && result.Payload.Records != nil {
		return result.Payload.Records[0], nil
	}
	return nil, err
}

// LunList finds LUNs with the specificed pattern
func (d RestClient) LunList(
	ctx context.Context, pattern string,
) (*san.LunCollectionGetOK, error) {

	params := san.NewLunCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = context.TODO()
	params.HTTPClient = d.httpClient

	params.ReturnRecords = ToBoolPointer(true)

	params.SVMNameQueryParameter = &d.config.SVM
	params.SetNameQueryParameter(ToStringPointer(pattern))
	params.SetFields([]string{"**"}) // TODO trim these down to just what we need

	return d.api.San.LunCollectionGet(params, d.authInfo)
}

// LunDelete deletes a LUN
func (d RestClient) LunDelete(
	ctx context.Context, lunUUID string,
) (*san.LunDeleteOK, error) {

	//params := san.NewLunCreateParamsWithTimeout(d.httpClient.Timeout)
	params := san.NewLunDeleteParamsWithTimeout(d.httpClient.Timeout)
	params.Context = context.TODO()
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = lunUUID

	return d.api.San.LunDelete(params, d.authInfo)
}

//////////////////////////////////////////////////////////////////////////////
// NETWORK operations
//////////////////////////////////////////////////////////////////////////////

// NetworkIPInterfacesList lists all IP interfaces
func (d RestClient) NetworkIPInterfacesList(
	ctx context.Context,
) (*networking.NetworkIPInterfacesGetOK, error) {

	params := networking.NewNetworkIPInterfacesGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = context.TODO()
	params.HTTPClient = d.httpClient

	params.ReturnRecords = ToBoolPointer(true)

	params.SVMNameQueryParameter = &d.config.SVM
	params.SetFields([]string{"**"}) // TODO trim these down to just what we need

	return d.api.Networking.NetworkIPInterfacesGet(params, d.authInfo)
}

//////////////////////////////////////////////////////////////////////////////
// JOB operations
//////////////////////////////////////////////////////////////////////////////

// JobGet returns the job by ID
func (d *RestClient) JobGet(jobUUID string) (*cluster.JobGetOK, error) {

	params := cluster.NewJobGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = context.TODO()
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = jobUUID

	params.SetFields([]string{"**"}) // TODO trim these down to just what we need, this forces ALL fields

	return d.api.Cluster.JobGet(params, d.authInfo)
}

// IsJobFinished lookus up the supplied JobLinkResponse's UUID to see if it's reached a terminal state
func (d *RestClient) IsJobFinished(payload *models.JobLinkResponse) (bool, error) {

	if payload == nil {
		return false, fmt.Errorf("payload is nil")
	}

	if payload.Job == nil {
		return false, fmt.Errorf("payload's Job is nil")
	}

	job := payload.Job
	jobUUID := job.UUID
	fmt.Printf("job %v\n", job)
	fmt.Printf("jobUUID %v\n", jobUUID)
	jobResult, err := d.JobGet(string(jobUUID))
	if err != nil {
		return false, err
	}

	switch jobState := jobResult.Payload.State; jobState {
	// BEGIN terminal states
	case models.JobStateSuccess:
		return true, nil
	case models.JobStateFailure:
		return true, nil
	// END terminal states
	// BEGIN non-terminal states
	case models.JobStatePaused:
		return false, nil
	case models.JobStateRunning:
		return false, nil
	case models.JobStateQueued:
		return false, nil
	// END non-terminal states
	default:
		return false, fmt.Errorf("unexpected job state %v", jobState)
	}
}

//////////////////////////////////////////////////////////////////////////////
// Aggregrate operations
//////////////////////////////////////////////////////////////////////////////

// AggregateList returns the names of all Aggregates whose names match the supplied pattern
func (d *RestClient) AggregateList(ctx context.Context, pattern string) (*storage.AggregateCollectionGetOK, error) {

	params := storage.NewAggregateCollectionGetParamsWithTimeout(d.httpClient.Timeout)

	params.Context = context.TODO()
	params.HTTPClient = d.httpClient
	params.ReturnRecords = ToBoolPointer(true)

	params.SetNameQueryParameter(ToStringPointer(pattern))
	params.SetFields([]string{"**"}) // TODO trim these down to just what we need

	return d.api.Storage.AggregateCollectionGet(params, d.authInfo)
}

//////////////////////////////////////////////////////////////////////////////
// SVM/Vserver operations
//////////////////////////////////////////////////////////////////////////////

// SvmGet gets the volume with the specified uuid
func (d RestClient) SvmGet(
	ctx context.Context, uuid string,
) (*svm.SvmGetOK, error) {

	params := svm.NewSvmGetParamsWithTimeout(d.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = uuid

	return d.api.Svm.SvmGet(params, d.authInfo)
}

// SvmList returns the names of all SVMs whose names match the supplied pattern
func (d *RestClient) SvmList(ctx context.Context, pattern string) (*svm.SvmCollectionGetOK, error) {

	params := svm.NewSvmCollectionGetParamsWithTimeout(d.httpClient.Timeout)

	params.Context = context.TODO()
	params.HTTPClient = d.httpClient
	params.ReturnRecords = ToBoolPointer(true)

	params.SetNameQueryParameter(ToStringPointer(pattern))
	params.SetFields([]string{"**"}) // TODO trim these down to just what we need

	return d.api.Svm.SvmCollectionGet(params, d.authInfo)
}

// SvmGetByName gets the volume with the specified name
func (d *RestClient) SvmGetByName(
	ctx context.Context, svmName string,
) (*models.Svm, error) {

	// TODO validate/improve this logic?
	result, err := d.SvmList(ctx, svmName)
	if result.Payload.NumRecords == 1 && result.Payload.Records != nil {
		return result.Payload.Records[0], nil
	}
	return nil, err
}

// TODO fill this out
func (d *RestClient) VserverGetAggregateNames() ([]string, error) {

	svm, err := d.SvmGetByName(context.TODO(), d.config.SVM)
	if err != nil {
		return nil, err
	}
	if svm == nil {
		return nil, fmt.Errorf("could not find SVM %s", d.config.SVM)
	}

	aggrNames := make([]string, 0, 10)
	for _, aggr := range svm.Aggregates {
		aggrNames = append(aggrNames, string(aggr.Name))
	}

	return aggrNames, nil
}

//////////////////////////////////////////////////////////////////////////////
// Misc operations
//////////////////////////////////////////////////////////////////////////////

// ClusterInfo returns information about the cluster
func (d *RestClient) ClusterInfo(ctx context.Context) (*cluster.ClusterGetOK, error) {

	params := cluster.NewClusterGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.ReturnRecords = ToBoolPointer(true)

	params.SetFields([]string{"**"}) // TODO trim these down to just what we need

	return d.api.Cluster.ClusterGet(params, d.authInfo)
}

// SystemGetOntapVersion gets the ONTAP version using the credentials, and caches & returns the result.
func (d *RestClient) SystemGetOntapVersion(ctx context.Context) (string, error) {

	if d.OntapVersion != "" {
		// return cached version
		return d.OntapVersion, nil
	}

	// it wasn't cached, look it up and cache it
	clusterInfoResult, err := d.ClusterInfo(ctx)
	if err != nil {
		return "unknown", err
	}
	if clusterInfoResult == nil {
		return "unknown", fmt.Errorf("could not determine cluster version")
	}

	version := clusterInfoResult.Payload.Version
	// version.Full // "NetApp Release 9.8X29: Sun Sep 27 12:15:48 UTC 2020"
	//d.OntapVersion = fmt.Sprintf("%d.%d", version.Generation, version.Major) // 9.8
	d.OntapVersion = fmt.Sprintf("%d.%d.%d", version.Generation, version.Major, version.Minor) // 9.8.0
	return d.OntapVersion, nil
}

// ClusterInfo returns information about the cluster
func (d *RestClient) NodeList(ctx context.Context, pattern string) (*cluster.NodesGetOK, error) {

	params := cluster.NewNodesGetParamsWithTimeout(d.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.ReturnRecords = ToBoolPointer(true)

	params.SetNameQueryParameter(ToStringPointer(pattern))
	params.SetFields([]string{"**"}) // TODO trim these down to just what we need

	return d.api.Cluster.NodesGet(params, d.authInfo)
}

func (d *RestClient) NodeListSerialNumbers(ctx context.Context) ([]string, error) {

	serialNumbers := make([]string, 0)

	nodeListResult, err := d.NodeList(ctx, "*")
	if err != nil {
		return serialNumbers, err
	}
	if nodeListResult == nil {
		return serialNumbers, errors.New("could not get node info")
	}

	if nodeListResult.Payload.NumRecords == 0 {
		return serialNumbers, errors.New("could not get node info")
	}

	// Get the serial numbers
	for _, node := range nodeListResult.Payload.Records {
		serialNumber := node.SerialNumber
		if serialNumber != "" {
			serialNumbers = append(serialNumbers, serialNumber)
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

////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////
// ONTAPI integration layer
////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////

type OntapAPIREST struct {
	API RestClient
}

func NewOntapAPIREST(restClient RestClient) (OntapAPIREST, error) {
	result := OntapAPIREST{
		API: restClient,
	}

	return result, nil
}

func (d OntapAPIREST) VolumeCreate(ctx context.Context, spec VolumeSpec) (*VolumeResponse, error) {

	if d.API.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "VolumeCreate",
			"Type":   "OntapAPIREST",
			"spec":   spec,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> VolumeCreate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< VolumeCreate")
	}

	name := spec.Name
	aggregateName := spec.AggregateName
	size := spec.Size
	spaceReserve := spec.SpaceReserve
	snapshotPolicy := spec.SnapshotPolicy
	unixPermissions := spec.UnixPermissions
	exportPolicy := spec.ExportPolicy
	securityStyle := spec.SecurityStyle
	tieringPolicy := spec.TieringPolicy
	comment := spec.Comment
	qosPolicyGroup := spec.Qos
	encrypt := spec.Encrypt
	snapshotReserve := spec.SnapshotReserve

	//_, err := d.API.VolumeCreate(ctx,
	volCreateResponse, err := d.API.VolumeCreate(ctx,
		name, aggregateName, size, spaceReserve, snapshotPolicy, unixPermissions, exportPolicy, securityStyle, tieringPolicy, comment,
		qosPolicyGroup,
		encrypt,
		snapshotReserve)
	if err != nil {
		return nil, fmt.Errorf("error creating volume: %v", err)
	}

	if volCreateResponse == nil {
		return nil, fmt.Errorf("missing volume create response")
	}

	// Name() string
	// Version() string
	// Status() string
	// Reason() string
	// Errno() string

	result := &VolumeResponse{}
	//result.name = fmt.Sprintf("%t", volCreateResponse) // TBD?
	//result.version = // TBD?
	// result.status = volCreateResponse.Result.ResultStatusAttr
	// result.reason = volCreateResponse.Result.ResultReasonAttr
	// result.errno = volCreateResponse.Result.ResultErrnoAttr
	return result, nil

	// return nil, nil
}

func (d OntapAPIREST) VolumeDestroy(name string, force bool) (*VolumeResponse, error) {
	return nil, nil
}

func (d OntapAPIREST) VolumeInfo(ctx context.Context, where ByCriteria, volumeName string) (*VolumeIterator, error) {
	switch where {
	case ByName: // Get Flexvol by name
		result, err := d.API.VolumeList(volumeName)
		if err != nil {
			return nil, err
		}

		if result.Payload.NumRecords != 1 {
			return nil, fmt.Errorf("unexpected number of volume records %v", result.Payload.NumRecords)
		}

		if result.Payload.Records == nil {
			return nil, fmt.Errorf("missing records in result")
		}

	}
	return nil, nil
}

////////////////////////////////////////////////////////////////////////////////////////////////////////
// ZAPI integration layer
////////////////////////////////////////////////////////////////////////////////////////////////////////

type OntapAPIZAPI struct {
	API Client
}

func NewOntapAPIZAPI(zapiClient Client) (OntapAPIZAPI, error) {
	result := OntapAPIZAPI{
		API: zapiClient,
	}
	return result, nil
}

func (d OntapAPIZAPI) VolumeCreate(ctx context.Context, spec VolumeSpec) (*VolumeResponse, error) {

	if d.API.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "VolumeCreate",
			"Type":   "OntapAPIZAPI",
			"spec":   spec,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> VolumeCreate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< VolumeCreate")
	}

	name := spec.Name
	aggregateName := spec.AggregateName
	size := spec.Size
	spaceReserve := spec.SpaceReserve
	snapshotPolicy := spec.SnapshotPolicy
	unixPermissions := spec.UnixPermissions
	exportPolicy := spec.ExportPolicy
	securityStyle := spec.SecurityStyle
	tieringPolicy := spec.TieringPolicy
	comment := spec.Comment
	qosPolicyGroup := spec.Qos
	encrypt := spec.Encrypt
	snapshotReserve := spec.SnapshotReserve

	//_, err := d.API.VolumeCreate(ctx,
	volCreateResponse, err := d.API.VolumeCreate(ctx,
		name, aggregateName, size, spaceReserve, snapshotPolicy, unixPermissions, exportPolicy, securityStyle, tieringPolicy, comment,
		qosPolicyGroup,
		encrypt,
		snapshotReserve)
	if err != nil {
		return nil, fmt.Errorf("error creating volume: %v", err)
	}

	if volCreateResponse == nil {
		return nil, fmt.Errorf("missing volume create response")
	}

	// Name() string
	// Version() string
	// Status() string
	// Reason() string
	// Errno() string

	result := &VolumeResponse{}
	//result.name = fmt.Sprintf("%t", volCreateResponse) // TBD?
	//result.version = // TBD?
	result.status = volCreateResponse.Result.ResultStatusAttr
	result.reason = volCreateResponse.Result.ResultReasonAttr
	result.errno = volCreateResponse.Result.ResultErrnoAttr
	return result, nil
}

func (d OntapAPIZAPI) VolumeDestroy(name string, force bool) (*VolumeResponse, error) {
	return nil, nil
}

func (d OntapAPIZAPI) VolumeInfo(ctx context.Context, where ByCriteria, name string) (*VolumeIterator, error) {
	switch where {
	case ByName: // Get Flexvol by name
		volumeGetResponse, err := d.API.VolumeGet(name)
		if err = GetError(ctx, volumeGetResponse, err); err != nil {
			Logc(ctx).Errorf("Error listing Flexvols: %v", err)
			return nil, err
		}

	}

	return nil, nil
}
