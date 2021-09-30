// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-openapi/runtime"
	runtime_client "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/cluster"
	nas "github.com/netapp/trident/storage_drivers/ontap/api/rest/client/n_a_s"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/networking"
	san "github.com/netapp/trident/storage_drivers/ontap/api/rest/client/s_a_n"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/storage"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/svm"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/netapp/trident/utils"
)

////////////////////////////////////////////////////////////////////////////////////////////////////////
// REST layer
////////////////////////////////////////////////////////////////////////////////////////////////////////

// ToStringPointer returns a pointer to the supplied string
func ToStringPointer(s string) *string {
	return &s
}

// ToSliceStringPointer returns a slice of strings converted into a slice of string pointers
func ToSliceStringPointer(slice []string) []*string {
	var result []*string
	for _, s := range slice {
		result = append(result, ToStringPointer(s))
	}
	return result
}

// ToBoolPointer returns a pointer to the supplied bool
func ToBoolPointer(b bool) *bool {
	return &b
}

// ToInt64Pointer returns an int64 pointer to the supplied int
func ToInt64Pointer(i int) *int64 {
	result := int64(i)
	return &result
}

func init() {
	// TODO move this around, rethink this
	os.Setenv("SWAGGER_DEBUG", "true")
	//os.Setenv("DEBUG", "true")
}

// RestClient is the object to use for interacting with ONTAP controllers via the REST API
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

	var cert tls.Certificate
	caCertPool := x509.NewCertPool()
	skipVerify := true

	clientCertificate := config.ClientCertificate
	clientPrivateKey := config.ClientPrivateKey
	if clientCertificate != "" && clientPrivateKey != "" {
		certDecode, err := base64.StdEncoding.DecodeString(clientCertificate)
		if err != nil {
			Logc(ctx).Debugf("error: %v", err)
			return nil, errors.New("failed to decode client certificate from base64")
		}
		keyDecode, err := base64.StdEncoding.DecodeString(clientPrivateKey)
		if err != nil {
			Logc(ctx).Debugf("error: %v", err)
			return nil, errors.New("failed to decode private key from base64")
		}
		cert, err = tls.X509KeyPair(certDecode, keyDecode)
		if err != nil {
			Logc(ctx).Debugf("error: %v", err)
			return nil, errors.New("cannot load certificate and key")
		}
	}

	trustedCACertificate := config.TrustedCACertificate
	if trustedCACertificate != "" {
		trustedCACert, err := base64.StdEncoding.DecodeString(trustedCACertificate)
		if err != nil {
			Logc(ctx).Debugf("error: %v", err)
			return nil, errors.New("failed to decode trusted CA certificate from base64")
		}
		skipVerify = false
		caCertPool.AppendCertsFromPEM(trustedCACert)
	}

	result := &RestClient{
		config: config,
	}

	result.tr = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: skipVerify,
			MinVersion:         tridentconfig.MinTLSVersion,
			Certificates:       []tls.Certificate{cert},
			RootCAs:            caCertPool},
	}

	result.httpClient = &http.Client{
		Transport: result.tr,
		Timeout:   time.Duration(60 * time.Second),
	}

	formats := strfmt.Default

	transportConfig := client.DefaultTransportConfig()
	transportConfig.Host = config.ManagementLIF

	result.api = client.NewHTTPClientWithConfig(formats, transportConfig)

	if config.Username != "" && config.Password != "" {
		result.authInfo = runtime_client.BasicAuth(config.Username, config.Password)
	}

	return result, nil
}

// NewRestClientFromOntapConfig is a factory method for creating a new Ontap API instance with a REST client
func NewRestClientFromOntapConfig(ctx context.Context, ontapConfig *drivers.OntapStorageDriverConfig) (OntapAPI, error) {

	client, err := NewRestClient(ctx, ClientConfig{
		ManagementLIF:        ontapConfig.ManagementLIF,
		SVM:                  ontapConfig.SVM,
		Username:             ontapConfig.Username,
		Password:             ontapConfig.Password,
		ClientPrivateKey:     ontapConfig.ClientPrivateKey,
		ClientCertificate:    ontapConfig.ClientCertificate,
		TrustedCACertificate: ontapConfig.TrustedCACertificate,
		DebugTraceFlags:      ontapConfig.DebugTraceFlags,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to get REST client for ontap: %v", err)
	}

	if client == nil {
		return nil, fmt.Errorf("unable to get REST client for ontap: unexpected error")
	}

	apiREST, err := NewOntapAPIREST(*client)
	if err != nil {
		return nil, fmt.Errorf("unable to get REST API client for ontap: %v", err)
	}

	return apiREST, nil
}

var (
	MinimumONTAPVersion = utils.MustParseSemantic("9.8.0")
)

// SupportsFeature returns true if the Ontap version supports the supplied feature
func (d RestClient) SupportsFeature(ctx context.Context, feature feature) bool {

	ontapVersion, err := d.SystemGetOntapVersion(ctx)
	if err != nil {
		return false
	}

	ontapSemVer, err := utils.ParseSemantic(ontapVersion)
	if err != nil {
		return false
	}

	if minVersion, ok := featuresByVersion[feature]; ok {
		return ontapSemVer.AtLeast(minVersion)
	} else {
		return false
	}
}

// ParamWrapper wraps a Param instance and overrides request parameters
type ParamWrapper struct {
	originalParams runtime.ClientRequestWriter
	next           *models.Href
}

// NewParamWrapper is a factory function to create a new instance of ParamWrapper
func NewParamWrapper(
	originalParams runtime.ClientRequestWriter, next *models.Href,
) runtime.ClientRequestWriter {
	result := &ParamWrapper{
		originalParams: originalParams,
		next:           next,
	}
	return result
}

// WriteToRequest uses composition to achieve collection pagination traversals
// * first, apply the original (wrapped) Param
// * then, apply any request values specified in the next link
func (o *ParamWrapper) WriteToRequest(r runtime.ClientRequest, req strfmt.Registry) error {
	// first, apply all of the wrapped request parameters
	if err := o.originalParams.WriteToRequest(r, req); err != nil {
		return err
	}

	if o.next == nil || o.next.Href == "" {
		// nothing to do
		return nil
	}

	// now, override query parameters values as needed. see also:
	//   -  https://play.golang.org/p/mjRu2iYod9N
	u, parseErr := url.Parse(o.next.Href)
	if parseErr != nil {
		return parseErr
	}

	m, parseErr := url.ParseQuery(u.RawQuery)
	if parseErr != nil {
		return parseErr
	}
	for parameter, value := range m {
		if err := r.SetQueryParam(parameter, value...); err != nil {
			return err
		}
	}
	return nil
}

func WithNextLink(next *models.Href) func(*runtime.ClientOperation) {
	return func(op *runtime.ClientOperation) {
		if next == nil {
			// no next link, nothing to do
			return
		}
		// this pattern, as defined by go-swagger, allows us to modify the clientOperation
		op.Params = NewParamWrapper(op.Params, next)
	}
}

// HasNextLink checks if restResult.Links.Next exists using reflection
func HasNextLink(restResult interface{}) (result bool) {

	//
	// using reflection, detect if we must paginate
	// "num_records": 1,
	// "_links": {
	//   "self": { "href": "/api/storage/volumes?fields=%2A%2A&max_records=1&name=%2A&return_records=true&svm.name=SVM" },
	//   "next": { "href": "/api/storage/volumes?start.uuid=00c881eb-f36c-11e8-996b-00a0986e75a0&fields=%2A%2A&max_records=1&name=%2A&return_records=true&svm.name=SVM" }
	// }
	//

	defer func() {
		if r := recover(); r != nil {
			result = false
		}
	}()

	if restResult == nil {
		return false // we were passed a nil
	}
	val := reflect.ValueOf(restResult)
	if reflect.TypeOf(restResult).Kind() == reflect.Ptr {
		// handle being passed either a pointer
		val = reflect.Indirect(val)
	}

	// safely check to see if we have restResult.Links
	if testLinks := val.FieldByName("Links"); testLinks.IsValid() {
		restResult = testLinks.Interface()
		val = reflect.ValueOf(restResult)
		if reflect.TypeOf(restResult).Kind() == reflect.Ptr {
			val = reflect.Indirect(val)
		}
	} else {
		return false
	}

	// safely check to see if we have restResult.Links.Next
	if testNext := val.FieldByName("Next"); testNext.IsValid() {
		restResult = testNext.Interface()
		val = reflect.ValueOf(restResult)
		if reflect.TypeOf(restResult).Kind() == reflect.Ptr {
			val = reflect.Indirect(val)
		}

		if testHref := val.FieldByName("Href"); testHref.IsValid() {
			href := val.FieldByName("Href").String()
			return href != ""
		} else {
			return false
		}
	}

	return false
}

//////////////////////////////////////////////////////////////////////////////
// NAS VOLUME operations by style (flexgroup and flexvol)
//////////////////////////////////////////////////////////////////////////////

func (d RestClient) getAllVolumePayloadRecords(payload *models.VolumeResponse,
	params *storage.VolumeCollectionGetParams) (*models.VolumeResponse, error) {

	if HasNextLink(payload) {
		nextLink := payload.Links.Next

		for {
			resultNext, errNext := d.api.Storage.VolumeCollectionGet(params, d.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				break
			}

			payload.NumRecords += resultNext.Payload.NumRecords
			payload.Records = append(resultNext.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				break
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return payload, nil
}

//getAllVolumesByPatternAndStyle returns all relevant details for all volumes of the style specified whose names match the supplied prefix
func (d RestClient) getAllVolumesByPatternStyleAndState(ctx context.Context, pattern, style, state string) (*storage.VolumeCollectionGetOK, error) {

	if style != models.VolumeStyleFlexvol && style != models.VolumeStyleFlexgroup {
		return nil, fmt.Errorf("unknown volume style %s", style)
	}

	validStates := map[string]struct{}{
		models.VolumeStateOnline:  {},
		models.VolumeStateOffline: {},
		models.VolumeStateMixed:   {},
		models.VolumeStateError:   {},
		"":                        {},
	}

	if _, ok := validStates[state]; !ok {
		return nil, fmt.Errorf("unknown volume state %s", state)
	}

	params := storage.NewVolumeCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	//params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SVMNameQueryParameter = &d.config.SVM
	params.SetNameQueryParameter(ToStringPointer(pattern))
	if state != "" {
		params.SetStateQueryParameter(ToStringPointer(state))
	}
	params.SetStyleQueryParameter(ToStringPointer(style))
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := d.api.Storage.VolumeCollectionGet(params, d.authInfo)

	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	result.Payload, err = d.getAllVolumePayloadRecords(result.Payload, params)
	if err != nil {
		return result, err
	}

	return result, nil
}

// checkVolumeExistsByNameAndStyle tests for the existence of a volume of the style and name specified
func (d RestClient) checkVolumeExistsByNameAndStyle(ctx context.Context, volumeName, style string) (bool, error) {
	volume, err := d.getVolumeByNameAndStyle(ctx, volumeName, style)
	if err != nil {
		return false, err
	}
	if volume == nil {
		return false, err
	}
	return true, nil
}

// getVolumeByNameAndStyle gets the volume of the style and name specified
func (d RestClient) getVolumeByNameAndStyle(
	ctx context.Context,
	volumeName string,
	style string,
) (*models.Volume, error) {

	result, err := d.getAllVolumesByPatternStyleAndState(ctx, volumeName, style, models.VolumeStateOnline)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Payload == nil || result.Payload.NumRecords == 0 {
		return nil, nil
	}
	if result.Payload.NumRecords == 1 && result.Payload.Records != nil {
		return result.Payload.Records[0], nil
	}
	return nil, fmt.Errorf("could not find unique volume with name '%v'; found %d matching volumes", volumeName, result.Payload.NumRecords)
}

// getVolumeInAnyStateByNameAndStyle gets the volume of the style and name specified
func (d RestClient) getVolumeInAnyStateByNameAndStyle(
	ctx context.Context,
	volumeName string,
	style string,
) (*models.Volume, error) {

	result, err := d.getAllVolumesByPatternStyleAndState(ctx, volumeName, style, "")
	if err != nil {
		return nil, err
	}
	if result == nil || result.Payload == nil || result.Payload.NumRecords == 0 {
		return nil, nil
	}
	if result.Payload.NumRecords == 1 && result.Payload.Records != nil {
		return result.Payload.Records[0], nil
	}
	return nil, fmt.Errorf("could not find unique volume with name '%v'; found %d matching volumes", volumeName, result.Payload.NumRecords)
}

// getVolumeSizeByNameAndStyle retrieves the size of the volume of the style and name specified
func (d RestClient) getVolumeSizeByNameAndStyle(ctx context.Context, volumeName, style string) (uint64, error) {

	volume, err := d.getVolumeByNameAndStyle(ctx, volumeName, style)
	if err != nil {
		return 0, err
	}
	if volume == nil {
		return 0, fmt.Errorf("could not find volume with name %v", volumeName)
	}

	return uint64(volume.Size), nil
}

// getVolumeUsedSizeByNameAndStyle retrieves the used bytes of the the volume of the style and name specified
func (d RestClient) getVolumeUsedSizeByNameAndStyle(ctx context.Context, volumeName, style string) (int, error) {

	volume, err := d.getVolumeByNameAndStyle(ctx, volumeName, style)
	if err != nil {
		return 0, err
	}
	if volume == nil {
		return 0, fmt.Errorf("could not find volume with name %v", volumeName)
	}

	if volume.Space == nil {
		return 0, fmt.Errorf("could not find space attributes for volume %v", volumeName)
	}

	if volume.Space.Snapshot == nil {
		return 0, fmt.Errorf("could not find snapshot space attributes for volume %v", volumeName)
	}

	return int(volume.Space.Used - volume.Space.Snapshot.Used), nil
}

// setVolumeSizeByNameAndStyle sets the size of the specified volume of given style
func (d RestClient) setVolumeSizeByNameAndStyle(
	ctx context.Context,
	volumeName, newSize, style string,
) error {

	volume, err := d.getVolumeByNameAndStyle(ctx, volumeName, style)
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

	volumeModifyAccepted, err := d.api.Storage.VolumeModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return d.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

// mountVolumeByNameAndStyle mounts a volume at the specified junction
func (d RestClient) mountVolumeByNameAndStyle(
	ctx context.Context,
	volumeName, junctionPath, style string,
) error {

	volume, err := d.getVolumeByNameAndStyle(ctx, volumeName, style)

	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}

	if volume.Nas != nil && volume.Nas.Path != nil {
		if *volume.Nas.Path == junctionPath {
			Logc(ctx).Debug("already mounted to the correct junction path, nothing to do")
			return nil
		}
	}

	uuid := volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = uuid

	volumeInfo := &models.Volume{
		Nas: &models.VolumeNas{Path: ToStringPointer(junctionPath)},
	}
	params.SetInfo(volumeInfo)

	volumeModifyAccepted, err := d.api.Storage.VolumeModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return d.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

// unmountVolumeByNameAndStyle umounts a volume
func (d RestClient) unmountVolumeByNameAndStyle(
	ctx context.Context,
	volumeName, style string,
) error {

	volume, err := d.getVolumeInAnyStateByNameAndStyle(ctx, volumeName, style)

	if err != nil {
		return err
	}

	if volume == nil {
		Logc(ctx).WithField("volume", volumeName).Warn("Volume does not exist.")
		return err
	}

	if volume.Nas != nil && volume.Nas.Path != nil {
		if *volume.Nas.Path == "" {
			Logc(ctx).Debug("already unmounted, nothing to do")
			return nil
		}
	}

	uuid := volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = uuid

	volumeInfo := &models.Volume{
		Nas: &models.VolumeNas{Path: ToStringPointer("")},
	}
	params.SetInfo(volumeInfo)

	volumeModifyAccepted, err := d.api.Storage.VolumeModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return d.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

// RenameVolumeByNameAndStyle changes the name of a FlexVol (but not a FlexGroup!)
func (d RestClient) renameVolumeByNameAndStyle(
	ctx context.Context,
	volumeName, newVolumeName, style string,
) error {

	volume, err := d.getVolumeByNameAndStyle(ctx, volumeName, style)
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

	volumeModifyAccepted, err := d.api.Storage.VolumeModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return d.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

// destroyVolumeByNameAndStyle destroys a volume
func (d RestClient) destroyVolumeByNameAndStyle(
	ctx context.Context, name, style string,
) error {

	volume, err := d.getVolumeByNameAndStyle(ctx, name, style)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume: %v", name)
	}

	params := storage.NewVolumeDeleteParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = volume.UUID

	volumeDeleteAccepted, err := d.api.Storage.VolumeDelete(params, d.authInfo)
	if err != nil {
		return err
	}
	if volumeDeleteAccepted == nil {
		return fmt.Errorf("unexpected response from volume create")
	}

	return d.PollJobStatus(ctx, volumeDeleteAccepted.Payload)
}

func (d RestClient) modifyVolumeExportPolicyByNameAndStyle(
	ctx context.Context,
	volumeName, exportPolicyName, style string,
) error {

	volume, err := d.getVolumeByNameAndStyle(ctx, volumeName, style)
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

	exportPolicy := &models.VolumeNasExportPolicy{Name: exportPolicyName}
	nasInfo := &models.VolumeNas{ExportPolicy: exportPolicy}
	volumeInfo := &models.Volume{Nas: nasInfo}
	params.SetInfo(volumeInfo)

	volumeModifyAccepted, err := d.api.Storage.VolumeModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return d.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

func (d RestClient) modifyVolumeUnixPermissionsByNameAndStyle(
	ctx context.Context,
	volumeName, unixPermissions, style string,
) error {

	if unixPermissions == "" {
		return fmt.Errorf("missing new unix permissions value")
	}

	volume, err := d.getVolumeByNameAndStyle(ctx, volumeName, style)
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

	// handle NAS options
	volumeNas := &models.VolumeNas{}
	unixPermissions = convertUnixPermissions(unixPermissions)
	volumePermissions, parseErr := strconv.ParseInt(unixPermissions, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("cannot process unix permissions value %v", unixPermissions)
	}
	volumeNas.UnixPermissions = volumePermissions

	volumeInfo := &models.Volume{}
	volumeInfo.Nas = volumeNas
	params.SetInfo(volumeInfo)

	volumeModifyAccepted, err := d.api.Storage.VolumeModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return d.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

// setVolumeCommentByNameAndStyle sets a volume's comment to the supplied value
// equivalent to filer::> volume modify -vserver iscsi_vs -volume v -comment newVolumeComment
func (d RestClient) setVolumeCommentByNameAndStyle(
	ctx context.Context,
	volumeName, newVolumeComment, style string,
) error {

	volume, err := d.getVolumeByNameAndStyle(ctx, volumeName, style)
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

	volumeModifyAccepted, err := d.api.Storage.VolumeModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return d.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

// convertUnixPermissions turns "rwx" into "7" and so on, if possible, otherwise returns the string
func convertUnixPermissions(s string) string {
	if strings.HasPrefix(s, "---") {
		// trim off any prefix
		s = s[3:]
	}
	if len(s) != 9 {
		return s
	}

	// make sure each position has a proper value
	for i := 0; i <= 6; i += 3 {
		if (s[i+0] != 'r') && (s[i+0] != '-') {
			return s
		}
		if (s[i+1] != 'w') && (s[i+1] != '-') {
			return s
		}
		if (s[i+2] != 'x') && (s[i+2] != '-') {
			return s
		}
	}

	values := map[rune]int{
		'r': 4,
		'w': 2,
		'x': 1,
		'-': 0,
	}

	a := []string{}
	for _, s := range []string{s[0:3], s[3:6], s[6:9]} {
		i := 0
		for _, r := range s {
			i += values[r]
		}
		a = append(a, fmt.Sprint(i))
	}
	return strings.Join(a, "")
}

// setVolumeQosPolicyGroupNameByNameAndStyle sets the QoS Policy Group for volume clones since
// we can't set adaptive policy groups directly during volume clone creation.
func (d RestClient) setVolumeQosPolicyGroupNameByNameAndStyle(
	ctx context.Context,
	volumeName string,
	qosPolicyGroup QosPolicyGroup,
	style string,
) error {

	volume, err := d.getVolumeByNameAndStyle(ctx, volumeName, style)
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

	volumeInfo := &models.Volume{}
	if qosPolicyGroup.Kind != InvalidQosPolicyGroupKind {
		if qosPolicyGroup.Name != "" {
			volumeInfo.Qos = &models.VolumeQos{
				Policy: &models.VolumeQosPolicy{Name: qosPolicyGroup.Name},
			}
		} else {
			return fmt.Errorf("missing QoS policy group name")
		}
	} else {
		return fmt.Errorf("invalid QoS policy group")
	}
	params.SetInfo(volumeInfo)

	volumeModifyAccepted, err := d.api.Storage.VolumeModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return d.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

// startCloneSplitByNameAndStyle starts splitting the clone
func (d RestClient) startCloneSplitByNameAndStyle(
	ctx context.Context,
	volumeName, style string,
) error {

	volume, err := d.getVolumeByNameAndStyle(ctx, volumeName, style)
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
		Clone: &models.VolumeClone{SplitInitiated: true},
	}
	//volumeInfo.Svm = &models.VolumeSvm{Name: d.config.SVM}

	params.SetInfo(volumeInfo)

	volumeModifyAccepted, err := d.api.Storage.VolumeModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return d.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

// restoreSnapshotByNameAndStyle restores a volume to a snapshot as a non-blocking operation
func (d RestClient) restoreSnapshotByNameAndStyle(
	ctx context.Context,
	snapshotName, volumeName, style string,
) error {

	volume, err := d.getVolumeByNameAndStyle(ctx, volumeName, style)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}

	// restore
	params := storage.NewVolumeModifyParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = volume.UUID
	params.RestoreToSnapshotNameQueryParameter = &snapshotName

	volumeModifyAccepted, err := d.api.Storage.VolumeModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return d.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

// TODO: change name
func (d RestClient) createCloneNAS(
	ctx context.Context,
	cloneName, sourceVolumeName, snapshotName string,
) (*storage.VolumeCreateAccepted, error) {
	params := storage.NewVolumeCreateParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	cloneInfo := &models.VolumeClone{
		ParentVolume: &models.VolumeCloneParentVolume{
			Name: sourceVolumeName,
		},
		IsFlexclone: true,
	}
	if snapshotName != "" {
		cloneInfo.ParentSnapshot = &models.SnapshotReference{Name: snapshotName}
	}

	volumeInfo := &models.Volume{
		Name:  cloneName,
		Clone: cloneInfo,
	}
	volumeInfo.Svm = &models.VolumeSvm{Name: d.config.SVM}

	params.SetInfo(volumeInfo)

	return d.api.Storage.VolumeCreate(params, d.authInfo)
}

// listAllVolumeNamesBackedBySnapshot returns the names of all volumes backed by the specified snapshot
func (d RestClient) listAllVolumeNamesBackedBySnapshot(ctx context.Context, volumeName, snapshotName string) (
	[]string, error) {

	params := storage.NewVolumeCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	params.SVMNameQueryParameter = &d.config.SVM
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	params.SetCloneParentVolumeNameQueryParameter(ToStringPointer(volumeName))
	params.SetCloneParentSnapshotNameQueryParameter(ToStringPointer(snapshotName))

	result, err := d.api.Storage.VolumeCollectionGet(params, d.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	result.Payload, err = d.getAllVolumePayloadRecords(result.Payload, params)
	if err != nil {
		return nil, err
	}

	volumeNames := make([]string, 0)
	for _, vol := range result.Payload.Records {
		if vol.Clone != nil && vol.Clone.IsFlexclone {
			if vol.Clone.ParentSnapshot != nil && vol.Clone.ParentSnapshot.Name == snapshotName &&
				vol.Clone.ParentVolume != nil && vol.Clone.ParentVolume.Name == volumeName {
				volumeNames = append(volumeNames, vol.Name)
			}
		}
	}
	return volumeNames, nil
}

// createVolumeByStyle creates a volume with the specified options
// equivalent to filer::> volume create -vserver iscsi_vs -volume v -aggregate aggr1 -size 1g -state online -type RW -policy default -unix-permissions ---rwxr-xr-x -space-guarantee none -snapshot-policy none -security-style unix -encrypt false
func (d *RestClient) createVolumeByStyle(
	ctx context.Context,
	name string, sizeInBytes int64, aggrs []string, spaceReserve, snapshotPolicy, unixPermissions,
	exportPolicy, securityStyle, tieringPolicy, comment string, qosPolicyGroup QosPolicyGroup, encrypt bool,
	snapshotReserve int, style string,
) error {

	params := storage.NewVolumeCreateParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	volumeInfo := &models.Volume{
		Name:           name,
		Size:           sizeInBytes,
		Guarantee:      &models.VolumeGuarantee{Type: spaceReserve},
		SnapshotPolicy: &models.VolumeSnapshotPolicy{Name: snapshotPolicy},
		Comment:        ToStringPointer(comment),
		State:          models.VolumeStateOnline,
		Style:          style,
	}

	volumeInfoAggregates := ToSliceVolumeAggregatesItems(aggrs)
	if len(volumeInfoAggregates) > 0 {
		volumeInfo.Aggregates = volumeInfoAggregates
	}

	if snapshotReserve != NumericalValueNotSet {
		volumeInfo.Space = &models.VolumeSpace{
			Snapshot: &models.VolumeSpaceSnapshot{
				ReservePercent: ToInt64Pointer(snapshotReserve),
			},
		}
	}
	volumeInfo.Svm = &models.VolumeSvm{Name: d.config.SVM}

	if encrypt {
		volumeInfo.Encryption = &models.VolumeEncryption{Enabled: true}
	}
	if qosPolicyGroup.Kind != InvalidQosPolicyGroupKind {
		if qosPolicyGroup.Name != "" {
			volumeInfo.Qos = &models.VolumeQos{
				Policy: &models.VolumeQosPolicy{Name: qosPolicyGroup.Name},
			}
		}
	}
	if tieringPolicy != "" {
		volumeInfo.Tiering = &models.VolumeTiering{Policy: tieringPolicy}
	}

	// handle NAS options
	volumeNas := &models.VolumeNas{}
	if securityStyle != "" {
		volumeNas.SecurityStyle = ToStringPointer(securityStyle)
		volumeInfo.Nas = volumeNas
	}
	if unixPermissions != "" {
		unixPermissions = convertUnixPermissions(unixPermissions)
		volumePermissions, parseErr := strconv.ParseInt(unixPermissions, 10, 64)
		if parseErr != nil {
			return fmt.Errorf("cannot process unix permissions value %v", unixPermissions)
		}
		volumeNas.UnixPermissions = volumePermissions
		volumeInfo.Nas = volumeNas
	}
	if exportPolicy != "" {
		volumeNas.ExportPolicy = &models.VolumeNasExportPolicy{Name: exportPolicy}
		volumeInfo.Nas = volumeNas
	}

	params.SetInfo(volumeInfo)

	volumeCreateAccepted, err := d.api.Storage.VolumeCreate(params, d.authInfo)
	if err != nil {
		return err
	}
	if volumeCreateAccepted == nil {
		return fmt.Errorf("unexpected response from volume create")
	}

	return d.PollJobStatus(ctx, volumeCreateAccepted.Payload)
}

//////////////////////////////////////////////////////////////////////////////
// NAS VOLUME by style operations end
//////////////////////////////////////////////////////////////////////////////

////////////////////////////////////////////////////////////////////////////
// VOLUME operations
//////////////////////////////////////////////////////////////////////////////

// VolumeList returns the names of all Flexvols whose names match the supplied pattern
func (d *RestClient) VolumeList(
	ctx context.Context,
	pattern string,
) (*storage.VolumeCollectionGetOK, error) {

	return d.getAllVolumesByPatternStyleAndState(ctx, pattern, models.VolumeStyleFlexvol, models.VolumeStateOnline)
}

// VolumeCreate creates a volume with the specified options
// equivalent to filer::> volume create -vserver iscsi_vs -volume v -aggregate aggr1 -size 1g -state online -type RW -policy default -unix-permissions ---rwxr-xr-x -space-guarantee none -snapshot-policy none -security-style unix -encrypt false
func (d *RestClient) VolumeCreate(
	ctx context.Context,
	name, aggregateName, size, spaceReserve, snapshotPolicy, unixPermissions,
	exportPolicy, securityStyle, tieringPolicy, comment string, qosPolicyGroup QosPolicyGroup, encrypt bool,
	snapshotReserve int,
) error {

	sizeBytesStr, _ := utils.ConvertSizeToBytes(size)
	sizeInBytes, _ := strconv.ParseInt(sizeBytesStr, 10, 64)

	return d.createVolumeByStyle(ctx, name, sizeInBytes, []string{aggregateName}, spaceReserve, snapshotPolicy,
		unixPermissions, exportPolicy, securityStyle, tieringPolicy, comment, qosPolicyGroup, encrypt,
		snapshotReserve, models.VolumeStyleFlexvol)
}

// VolumeExists tests for the existence of a flexvol
func (d RestClient) VolumeExists(ctx context.Context, volumeName string) (bool, error) {
	return d.checkVolumeExistsByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexvol)
}

// VolumeGetByName gets the flexvol with the specified name
func (d RestClient) VolumeGetByName(ctx context.Context, volumeName string) (*models.Volume, error) {
	return d.getVolumeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexvol)
}

// VolumeMount mounts a flexvol at the specified junction
func (d RestClient) VolumeMount(
	ctx context.Context,
	volumeName, junctionPath string,
) error {
	return d.mountVolumeByNameAndStyle(ctx, volumeName, junctionPath, models.VolumeStyleFlexvol)
}

// VolumeRename changes the name of a flexvol
func (d RestClient) VolumeRename(
	ctx context.Context,
	volumeName, newVolumeName string,
) error {

	return d.renameVolumeByNameAndStyle(ctx, volumeName, newVolumeName, models.VolumeStyleFlexvol)
}

func (d RestClient) VolumeModifyExportPolicy(
	ctx context.Context,
	volumeName, exportPolicyName string,
) error {

	return d.modifyVolumeExportPolicyByNameAndStyle(ctx, volumeName, exportPolicyName, models.VolumeStyleFlexvol)
}

// VolumeSize retrieves the size of the specified flexvol
func (d RestClient) VolumeSize(ctx context.Context, volumeName string,
) (uint64, error) {
	return d.getVolumeSizeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexvol)
}

// VolumeUsedSize retrieves the used bytes of the specified volume
func (d RestClient) VolumeUsedSize(ctx context.Context, volumeName string) (int, error) {
	return d.getVolumeUsedSizeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexvol)
}

// VolumeSetSize sets the size of the specified flexvol
func (d RestClient) VolumeSetSize(ctx context.Context, volumeName, newSize string,
) error {
	return d.setVolumeSizeByNameAndStyle(ctx, volumeName, newSize, models.VolumeStyleFlexvol)
}

func (d RestClient) VolumeModifyUnixPermissions(
	ctx context.Context,
	volumeName, unixPermissions string,
) error {

	return d.modifyVolumeUnixPermissionsByNameAndStyle(ctx, volumeName, unixPermissions, models.VolumeStyleFlexvol)
}

// VolumeSetComment sets a flexvol's comment to the supplied value
// equivalent to filer::> volume modify -vserver iscsi_vs -volume v -comment newVolumeComment
func (d RestClient) VolumeSetComment(ctx context.Context, volumeName, newVolumeComment string,
) error {
	return d.setVolumeCommentByNameAndStyle(ctx, volumeName, newVolumeComment, models.VolumeStyleFlexvol)
}

// VolumeSetQosPolicyGroupName sets the QoS Policy Group for volume clones since
// we can't set adaptive policy groups directly during volume clone creation.
func (d RestClient) VolumeSetQosPolicyGroupName(ctx context.Context, volumeName string, qosPolicyGroup QosPolicyGroup,
) error {
	return d.setVolumeQosPolicyGroupNameByNameAndStyle(ctx, volumeName, qosPolicyGroup, models.VolumeStyleFlexvol)
}

// VolumeCloneSplitStart starts splitting theflexvol clone
func (d RestClient) VolumeCloneSplitStart(
	ctx context.Context,
	volumeName string,
) error {
	return d.startCloneSplitByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexvol)
}

// VolumeDestroy destroys a flexvol
func (d RestClient) VolumeDestroy(
	ctx context.Context, name string,
) error {
	return d.destroyVolumeByNameAndStyle(ctx, name, models.VolumeStyleFlexvol)
}

//////////////////////////////////////////////////////////////////////////////
// SNAPSHOT operations
//////////////////////////////////////////////////////////////////////////////

// SnapshotCreate creates a snapshot
func (d *RestClient) SnapshotCreate(ctx context.Context, volumeUUID, snapshotName string) (
	*storage.SnapshotCreateAccepted, error) {
	params := storage.NewSnapshotCreateParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	params.VolumeUUIDPathParameter = volumeUUID

	snapshotInfo := &models.Snapshot{
		Name: snapshotName,
	}

	snapshotInfo.Svm = &models.SnapshotSvm{Name: d.config.SVM}

	params.SetInfo(snapshotInfo)

	return d.api.Storage.SnapshotCreate(params, d.authInfo)
}

// SnapshotCreateAndWait creates a snapshot and waits on the job to complete
func (d *RestClient) SnapshotCreateAndWait(ctx context.Context, volumeUUID, snapshotName string) error {
	snapshotCreateResult, err := d.SnapshotCreate(ctx, volumeUUID, snapshotName)
	if err != nil {
		return fmt.Errorf("could not create snapshot: %v", err)
	}
	if snapshotCreateResult == nil {
		return fmt.Errorf("could not create snapshot: %v", "unexpected result")
	}

	return d.PollJobStatus(ctx, snapshotCreateResult.Payload)
}

// SnapshotList lists snapshots
func (d *RestClient) SnapshotList(ctx context.Context, volumeUUID string) (*storage.SnapshotCollectionGetOK, error) {
	params := storage.NewSnapshotCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	params.VolumeUUIDPathParameter = volumeUUID

	params.SVMNameQueryParameter = ToStringPointer(d.config.SVM)
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := d.api.Storage.SnapshotCollectionGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NEXT_LOOP:
		for !done {
			resultNext, errNext := d.api.Storage.SnapshotCollectionGet(params, d.authInfo,
				WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NEXT_LOOP
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NEXT_LOOP
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

// SnapshotListByName lists snapshots by name
func (d *RestClient) SnapshotListByName(ctx context.Context, volumeUUID, snapshotName string) (
	*storage.SnapshotCollectionGetOK, error) {
	params := storage.NewSnapshotCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	params.VolumeUUIDPathParameter = volumeUUID
	params.NameQueryParameter = ToStringPointer(snapshotName)

	params.SVMNameQueryParameter = ToStringPointer(d.config.SVM)
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	return d.api.Storage.SnapshotCollectionGet(params, d.authInfo)
}

// SnapshotGet returns info on the snapshot
func (d *RestClient) SnapshotGet(ctx context.Context, volumeUUID, snapshotUUID string) (*storage.SnapshotGetOK, error) {
	params := storage.NewSnapshotGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	params.VolumeUUIDPathParameter = volumeUUID
	params.UUIDPathParameter = snapshotUUID

	return d.api.Storage.SnapshotGet(params, d.authInfo)
}

// SnapshotGetByName finds the snapshot by name
func (d RestClient) SnapshotGetByName(ctx context.Context, volumeUUID, snapshotName string) (*models.Snapshot, error) {

	// TODO improve this
	result, err := d.SnapshotListByName(ctx, volumeUUID, snapshotName)
	if result.Payload.NumRecords == 1 && result.Payload.Records != nil {
		return result.Payload.Records[0], nil
	}
	return nil, err
}

// SnapshotDelete deletes a snapshot
func (d *RestClient) SnapshotDelete(
	ctx context.Context,
	volumeUUID, snapshotUUID string,
) (*storage.SnapshotDeleteAccepted, error) {

	params := storage.NewSnapshotDeleteParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	params.VolumeUUIDPathParameter = volumeUUID
	params.UUIDPathParameter = snapshotUUID

	return d.api.Storage.SnapshotDelete(params, d.authInfo)
}

// SnapshotRestoreVolume restores a volume to a snapshot as a non-blocking operation
func (d RestClient) SnapshotRestoreVolume(ctx context.Context, snapshotName, volumeName string) error {
	return d.restoreSnapshotByNameAndStyle(ctx, snapshotName, volumeName, models.VolumeStyleFlexvol)
}

// SnapshotRestoreFlexgroup restores a volume to a snapshot as a non-blocking operation
func (d RestClient) SnapshotRestoreFlexgroup(ctx context.Context, snapshotName, volumeName string) error {
	return d.restoreSnapshotByNameAndStyle(ctx, snapshotName, volumeName, models.VolumeStyleFlexgroup)
}

// VolumeDisableSnapshotDirectoryAccess disables access to the ".snapshot" directory
// Disable '.snapshot' to allow official mysql container's chmod-in-init to work
func (d RestClient) VolumeDisableSnapshotDirectoryAccess(ctx context.Context, volumeName string) error {
	result, err := d.CliPassthroughVolumePatch(ctx, volumeName, `{
		"snapdir-access": "false"
}`)
	if err != nil {
		return err
	}

	if result.Error != nil && result.Error.Message != "" {
		return fmt.Errorf("error while disabling .snapshot directory access: %v", result.Error.Message)
	}
	return nil
}

// VolumeListAllBackedBySnapshot returns the names of all FlexVols backed by the specified snapshot
func (d RestClient) VolumeListAllBackedBySnapshot(ctx context.Context, volumeName, snapshotName string) ([]string,
	error) {
	return d.listAllVolumeNamesBackedBySnapshot(ctx, volumeName, snapshotName)
}

//////////////////////////////////////////////////////////////////////////////
// CLONE operations
//////////////////////////////////////////////////////////////////////////////

// VolumeCloneCreate creates a clone
// see also: https://library.netapp.com/ecmdocs/ECMLP2858435/html/resources/volume.html#creating-a-flexclone-and-specifying-its-properties-using-post
func (d RestClient) VolumeCloneCreate(ctx context.Context, cloneName, sourceVolumeName, snapshotName string) (
	*storage.VolumeCreateAccepted, error) {
	return d.createCloneNAS(ctx, cloneName, sourceVolumeName, snapshotName)
}

// VolumeCloneCreateAsync clones a volume from a snapshot
func (d RestClient) VolumeCloneCreateAsync(ctx context.Context, cloneName, sourceVolumeName, snapshot string) error {
	cloneCreateResult, err := d.createCloneNAS(ctx, cloneName, sourceVolumeName, snapshot)
	if err != nil {
		return fmt.Errorf("could not create clone: %v", err)
	}
	if cloneCreateResult == nil {
		return fmt.Errorf("could not create clone: %v", "unexpected result")
	}

	return d.PollJobStatus(ctx, cloneCreateResult.Payload)
}

/////////////////////////////////////////////////////////////////////////////
// iSCSI initiator operations
/////////////////////////////////////////////////////////////////////////////

// IscsiInitiatorGetDefaultAuth returns the authorization details for the default initiator
// equivalent to filer::> vserver iscsi security show -vserver SVM -initiator-name default
func (d RestClient) IscsiInitiatorGetDefaultAuth(ctx context.Context) (*san.IscsiCredentialsCollectionGetOK, error) {

	params := san.NewIscsiCredentialsCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.ReturnRecordsQueryParameter = ToBoolPointer(true)

	//params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SVMNameQueryParameter = ToStringPointer(d.config.SVM)
	params.InitiatorQueryParameter = ToStringPointer("default") // TODO use a constant?

	// TODO Limit the returned data to only the disk limit
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := d.api.San.IscsiCredentialsCollectionGet(params, d.authInfo)

	if err != nil {
		return nil, err
	}
	if result.Payload == nil {
		return nil, nil
	}

	return result, nil
}

// IscsiInterfaceGet returns information about the vserver's  iSCSI interfaces
func (d RestClient) IscsiInterfaceGet(ctx context.Context, svm string) (*san.IscsiServiceCollectionGetOK,
	error) {

	params := san.NewIscsiServiceCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.ReturnRecordsQueryParameter = ToBoolPointer(true)
	params.SVMNameQueryParameter = &svm

	// TODO Limit the returned data to only the disk limit
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := d.api.San.IscsiServiceCollectionGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	return result, nil
}

// IscsiInitiatorSetDefaultAuth sets the authorization details for the default initiator
// equivalent to filer::> vserver iscsi security modify -vserver SVM -initiator-name default \
//                           -auth-type CHAP -user-name outboundUserName -outbound-user-name outboundPassphrase
func (d RestClient) IscsiInitiatorSetDefaultAuth(ctx context.Context, authType, userName, passphrase,
	outbountUserName, outboundPassphrase string,
	) error {

	getDefaultAuthResponse, err := d.IscsiInitiatorGetDefaultAuth(ctx)
	if err != nil{
		return err
	}
	if getDefaultAuthResponse == nil {
		return fmt.Errorf("could not get the default iscsi initiator")
	}
	if getDefaultAuthResponse.Payload.NumRecords != 1 {
		return fmt.Errorf("should only be one default iscsi initiator")
	}

	params := san.NewIscsiCredentialsModifyParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient


	outboundInfo := &models.IscsiCredentialsChapOutbound{}
	if outbountUserName != "" && outboundPassphrase != "" {
		outboundInfo.Password = outboundPassphrase
		outboundInfo.User = outbountUserName
	}
	inboundInfo := &models.IscsiCredentialsChapInbound{
		Password: passphrase,
		User:     userName,
	}
	chapInfo := &models.IscsiCredentialsChap{
		Inbound:  inboundInfo,
		Outbound: outboundInfo,
	}
	authInfo := &models.IscsiCredentials{
		AuthenticationType: authType,
		Chap:               chapInfo,
		Initiator:          getDefaultAuthResponse.Payload.Records[0].Initiator,
	}

	params.SetInfo(authInfo)

	_, err = d.api.San.IscsiCredentialsModify(params, d.authInfo)

	return err
}

// IscsiNodeGetName returns information about the vserver's iSCSI node name
func (d RestClient) IscsiNodeGetName(ctx context.Context) (*san.IscsiServiceGetOK,
	error) {

	svm, err := d.SvmGetByName(ctx, d.config.SVM)
	if err != nil {
		return nil, err
	}
	if svm == nil {
		return nil, fmt.Errorf("could not find SVM %s", d.config.SVM)
	}

	params := san.NewIscsiServiceGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.SVMUUIDPathParameter = svm.UUID

	// TODO Limit the returned data to only the disk limit
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := d.api.San.IscsiServiceGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	return result, nil
}

/////////////////////////////////////////////////////////////////////////////
// IGROUP operations
/////////////////////////////////////////////////////////////////////////////

// IgroupCreate creates the specified initiator group
// equivalent to filer::> igroup create docker -vserver iscsi_vs -protocol iscsi -ostype linux
func (d RestClient) IgroupCreate(ctx context.Context, initiatorGroupName, initiatorGroupType, osType string) error {
	params := san.NewIgroupCreateParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.ReturnRecordsQueryParameter = ToBoolPointer(true)

	igroupInfo := &models.Igroup{
		Name:     initiatorGroupName,
		Protocol: ToStringPointer(initiatorGroupType),
		OsType:   osType,
	}

	igroupInfo.Svm = &models.IgroupSvm{Name: d.config.SVM}

	params.SetInfo(igroupInfo)

	igroupCreateAccepted, err := d.api.San.IgroupCreate(params, d.authInfo)
	if err != nil {

		return err
	}
	if igroupCreateAccepted == nil {
		return fmt.Errorf("unexpected response from igroup create")
	}

	if igroupCreateAccepted.Payload == nil {
		return fmt.Errorf("unexpected response from igroup create, payload was nil")
	} else {
		if igroupCreateAccepted.Payload.NumRecords != 1 {
			return fmt.Errorf("unexpected response from igroup create, created %v igroups",
				igroupCreateAccepted.Payload.NumRecords)
		}
	}

	return nil
}

// IgroupAdd adds an initiator to an initiator group
// equivalent to filer::> lun igroup add -vserver iscsi_vs -igroup docker -initiator iqn.1993-08.org.
// debian:01:9031309bbebd
func (d RestClient) IgroupAdd(
	ctx context.Context,
	initiatorGroupName, initiator string) error {

	igroup, err := d.IgroupGetByName(ctx, initiatorGroupName)
	if err != nil {
		return err
	}
	if igroup == nil {
		return fmt.Errorf("unexpected response from igroup lookup, igroup was nil")
	}
	igroupUUID := igroup.UUID

	params := san.NewIgroupInitiatorCreateParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.IgroupUUIDPathParameter = igroupUUID

	igroupInitiator := &models.IgroupInitiator{
		Name:    initiator,
	}

	params.SetInfo(igroupInitiator)

	_, err = d.api.San.IgroupInitiatorCreate(params, d.authInfo)
	if err != nil {
		return err
	}

	return nil
}

// IgroupRemove removes an initiator from an initiator group
func (d RestClient) IgroupRemove(
	ctx context.Context,
	initiatorGroupName, initiator string) error {

	igroup, err := d.IgroupGetByName(ctx, initiatorGroupName)
	if err != nil {
		return err
	}
	if igroup == nil {
		return fmt.Errorf("unexpected response from igroup lookup, igroup was nil")
	}
	igroupUUID := igroup.UUID

	params := san.NewIgroupInitiatorDeleteParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.IgroupUUIDPathParameter = igroupUUID
	params.NamePathParameter = initiator

	deleteAccepted, err := d.api.San.IgroupInitiatorDelete(params, d.authInfo)
	if err != nil {
		return err
	}
	if deleteAccepted == nil {
		return fmt.Errorf("unexpected response from igroup remove")
	}

	return nil
}

// IgroupDestroy destroys an initiator group
func (d RestClient) IgroupDestroy(
	ctx context.Context,
	initiatorGroupName string) error {

	igroup, err := d.IgroupGetByName(ctx, initiatorGroupName)
	if err != nil {
		return err
	}
	if igroup == nil {
		return fmt.Errorf("unexpected response from igroup lookup, igroup was nil")
	}
	igroupUUID := igroup.UUID

	params := san.NewIgroupDeleteParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = igroupUUID

	lunDeleteResult, err := d.api.San.IgroupDelete(params, d.authInfo)
	if err != nil {
		return fmt.Errorf("could not delete igroup: %v", err)
	}
	if lunDeleteResult == nil {
		return fmt.Errorf("could not delete igroup: %v", "unexpected result")
	}

	return nil
}

// IgroupList lists initiator groups
func (d RestClient) IgroupList(ctx context.Context, pattern string) (*san.IgroupCollectionGetOK, error) {

	if pattern == "" {
		pattern = "*"
	}

	params := san.NewIgroupCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	params.SVMNameQueryParameter = &d.config.SVM
	params.SetNameQueryParameter(ToStringPointer(pattern))
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := d.api.San.IgroupCollectionGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NEXT_LOOP:
		for !done {
			resultNext, errNext := d.api.San.IgroupCollectionGet(params, d.authInfo,
				WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NEXT_LOOP
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NEXT_LOOP
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

// IgroupGet gets the igroup with the specified uuid
func (d RestClient) IgroupGet(
	ctx context.Context,
	uuid string,
) (*san.IgroupGetOK, error) {

	params := san.NewIgroupGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = uuid

	return d.api.San.IgroupGet(params, d.authInfo)
}

// IgroupGetByName gets the igroup with the specified name
func (d RestClient) IgroupGetByName(
	ctx context.Context,
	initiatorGroupName string,
) (*models.Igroup, error) {

	// TODO improve this
	result, err := d.IgroupList(ctx, initiatorGroupName)
	if err != nil {
		return nil, err
	}
	if result.Payload.NumRecords == 1 && result.Payload.Records != nil {
		return result.Payload.Records[0], nil
	}
	return nil, err
}

//////////////////////////////////////////////////////////////////////////////
// LUN operations
//////////////////////////////////////////////////////////////////////////////

// LunCreate creates a LUN
func (d RestClient) LunCreate(
	ctx context.Context,
	lunPath string, sizeInBytes int64, osType string, qosPolicyGroup QosPolicyGroup,
) error {

	if strings.Contains(lunPath, failureLUNCreate) {
		return errors.New("injected error")
	}

	params := san.NewLunCreateParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	lunInfo := &models.Lun{
		Name:   lunPath, // example:  /vol/myVolume/myLun1
		OsType: osType,
		Space:  &models.LunSpace{
			Size: sizeInBytes,
		},
		QosPolicy: &models.LunQosPolicy{
			Name:  qosPolicyGroup.Name,
		},
	}
	lunInfo.Svm = &models.LunSvm{Name: d.config.SVM}

	params.SetInfo(lunInfo)

	lunCreateAccepted, err := d.api.San.LunCreate(params, d.authInfo)
	if err != nil {
		return err
	}
	if lunCreateAccepted == nil {
		return fmt.Errorf("unexpected response from lun create")
	}

	return nil
}

// LunGet gets the LUN with the specified uuid
func (d RestClient) LunGet(
	ctx context.Context,
	uuid string,
) (*san.LunGetOK, error) {

	params := san.NewLunGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = uuid

	return d.api.San.LunGet(params, d.authInfo)
}

// LunGetByName gets the LUN with the specified name
func (d RestClient) LunGetByName(
	ctx context.Context,
	name string,
) (*models.Lun, error) {

	// TODO improve this
	result, err := d.LunList(ctx, name)
	if err != nil || result.Payload == nil {
		return nil, err
	}
	if result.Payload.NumRecords == 1 && result.Payload.Records != nil {
		return result.Payload.Records[0], nil
	}
	return nil, err
}

// LunList finds LUNs with the specificed pattern
func (d RestClient) LunList(
	ctx context.Context,
	pattern string,
) (*san.LunCollectionGetOK, error) {

	params := san.NewLunCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	params.SVMNameQueryParameter = &d.config.SVM
	params.SetNameQueryParameter(ToStringPointer(pattern))
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := d.api.San.LunCollectionGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NEXT_LOOP:
		for !done {
			resultNext, errNext := d.api.San.LunCollectionGet(params, d.authInfo,
				WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NEXT_LOOP
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NEXT_LOOP
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

// LunDelete deletes a LUN
func (d RestClient) LunDelete(
	ctx context.Context,
	lunUUID string,
) error {

	params := san.NewLunDeleteParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = lunUUID

	lunDeleteResult, err := d.api.San.LunDelete(params, d.authInfo)
	if err != nil {
		return fmt.Errorf("could not delete lun: %v", err)
	}
	if lunDeleteResult == nil {
		return fmt.Errorf("could not delete lun: %v", "unexpected result")
	}

	return nil
}

// TODO: Change this for LUN Attributes when available
// LunGetComment gets the comment for a given LUN.
// This is in place of the fstype and context attributes from ZAPI
func (d RestClient) LunGetComment(
	ctx context.Context,
	lunPath string,
) (string, error) {

	lun, err := d.LunGetByName(ctx, lunPath)
	if err != nil {
		return "", err
	}
	if lun == nil {
		return "", fmt.Errorf("could not find lun with name %v", lunPath)
	}
	if lun.Comment == nil {
		return "", fmt.Errorf("lun did not have a comment")
	}

	return *lun.Comment, nil
}

// LunSetComment sets the comment for a given LUN.
// This is in place of the fstype and context attributes from ZAPI
func (d RestClient) LunSetComment(
	ctx context.Context,
	lunPath, comment string,
) error {

	lun, err := d.LunGetByName(ctx, lunPath)
	if err != nil {
		return err
	}
	if lun == nil {
		return fmt.Errorf("could not find lun with name %v", lunPath)
	}

	uuid := lun.UUID

	params := san.NewLunModifyParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = uuid

	lunInfo := &models.Lun{
		Comment:      &comment,
	}

	params.SetInfo(lunInfo)

	lunModifyOK, err := d.api.San.LunModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if lunModifyOK == nil {
		return fmt.Errorf("unexpected response from LUN modify")
	}

	return nil
}

// LunSetComment sets the comment for a given LUN.
func (d RestClient) LunSetQosPolicyGroup(
	ctx context.Context,
	lunPath, qosPolicyGroup string,
	) error {

	lun, err := d.LunGetByName(ctx, lunPath)
	if err != nil {
		return err
	}
	if lun == nil {
		return fmt.Errorf("could not find lun with name %v", lunPath)
	}

	uuid := lun.UUID

	params := san.NewLunModifyParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = uuid

	qosPolicy := &models.LunQosPolicy{
		Name:  qosPolicyGroup,
	}
	lunInfo := &models.Lun{
		QosPolicy:    qosPolicy,
	}

	params.SetInfo(lunInfo)

	lunModifyOK, err := d.api.San.LunModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if lunModifyOK == nil {
		return fmt.Errorf("unexpected response from LUN modify")
	}

	return nil
}

// LunRename changes the name of a LUN
func (d RestClient) LunRename(
	ctx context.Context,
	lunPath, newLunPath string,
) error {

	lun, err := d.LunGetByName(ctx, lunPath)
	if err != nil {
		return err
	}
	if lun == nil {
		return fmt.Errorf("could not find lun with name %v", lunPath)
	}

	uuid := lun.UUID

	params := san.NewLunModifyParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = uuid

	lunInfo := &models.Lun{
		Name:         newLunPath,
	}

	params.SetInfo(lunInfo)

	lunModifyOK, err := d.api.San.LunModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if lunModifyOK == nil {
		return fmt.Errorf("unexpected response from LUN modify")
	}

	return nil
}

// LunMapInfo gets the LUN maping information for the specified LUN
func (d RestClient) LunMapInfo(
	ctx context.Context,
	initiatorGroupName, lunPath string,
) (*san.LunMapCollectionGetOK, error) {

	params := san.NewLunMapCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.LunNameQueryParameter = &lunPath
	if initiatorGroupName != "" {
		params.IgroupNameQueryParameter = &initiatorGroupName
	}

	return d.api.San.LunMapCollectionGet(params, d.authInfo)
}

// LunUnmap deletes the lun mapping for the given LUN path and igroup
// equivalent to filer::> lun mapping delete -vserver iscsi_vs -path /vol/v/lun0 -igroup group
func (d RestClient) LunUnmap(
	ctx context.Context,
	initiatorGroupName, lunPath string,
	) error {

	lunMapResponse, err := d.LunMapInfo(ctx, initiatorGroupName, lunPath)
	if err != nil {
		return fmt.Errorf("problem reading maps for LUN %s: %v", lunPath, err)
	} else if lunMapResponse.Payload == nil {
		return fmt.Errorf("problem reading maps for LUN %s", lunPath)
	}
	igroupUUID := lunMapResponse.Payload.Records[0].Igroup.UUID

	lun, err := d.LunGetByName(ctx, lunPath)
	if err != nil {
		return err
	}
	if lun == nil {
		return fmt.Errorf("could not find lun with name %v", lunPath)
	}
	lunUUID := lun.UUID

	params := san.NewLunMapDeleteParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.IgroupUUIDPathParameter = igroupUUID
	params.LunUUIDPathParameter = lunUUID

	_, err = d.api.San.LunMapDelete(params, d.authInfo)
	if err != nil {
		return err
	}
	return nil
}

// LunMap maps a LUN to an id in an initiator group
// equivalent to filer::> lun map -vserver iscsi_vs -path /vol/v/lun1 -igroup docker -lun-id 0
func (d RestClient) LunMap(
	ctx context.Context,
	initiatorGroupName, lunPath string,
	lunID int,
) (*san.LunMapCreateCreated, error) {

	lun, err := d.LunGetByName(ctx, lunPath)
	if err != nil {
		return nil, err
	}
	if lun == nil {
		return nil, fmt.Errorf("could not find lun with name %v", lunPath)
	}
	uuid := lun.UUID

	params := san.NewLunMapCreateParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.ReturnRecordsQueryParameter = ToBoolPointer(true)

	igroupInfo := &models.LunMapIgroup{
		Name:       initiatorGroupName,
	}
	lunInfo := &models.LunMapLun{
		Name:  lunPath,
		UUID:  uuid,
	}
	lunSVM := &models.LunMapSvm{
		Name:  d.config.SVM,
	}
	lunMapInfo := &models.LunMap{
		Igroup: igroupInfo,
		Lun:    lunInfo,
		Svm:    lunSVM,
	}
	if lunID != -1 {
		lunMapInfo.LogicalUnitNumber = ToInt64Pointer(lunID)
	}
	params.SetInfo(lunMapInfo)

	result, err := d.api.San.LunMapCreate(params, d.authInfo)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (d *RestClient) CliPassthroughLunMappingGet(
	ctx context.Context,
	initiatorGroupName, lunPath string,
) (*CliPassthroughResult, error) {
	// See also:
	//   https://docs.netapp.com/us-en/ontap-automation/accessing_the_ontap_cli_through_the_rest_api.html
	//   https://library.netapp.com/ecmdocs/ECMLP2858435/html/resources/cli.html

	url := fmt.Sprintf(
		`https://%v/api/private/cli/lun/mapping?vserver=%v&igroup=%v&path=%v&fields=%v`,
		d.config.ManagementLIF,
		d.config.SVM,
		initiatorGroupName,
		url.QueryEscape(lunPath), // lunPath
		"vserver,path,volume,qtree,lun,igroup,reporting-nodes",
	)

	fmt.Printf("GET %v\n", url)

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Content-Type", "application/json")
	if d.config.Username != "" && d.config.Password != "" {
		req.SetBasicAuth(d.config.Username, d.config.Password)
	}

	// certs will have been parsed and configured already, if needed, as part of the RestClient init
	tr := d.tr

	client := &http.Client{
		Transport: tr,
		Timeout:   time.Duration(tridentconfig.StorageAPITimeoutSeconds * time.Second),
	}

	response, err := client.Do(req)
	if err != nil {
		return nil, err
	} else if response.StatusCode == 401 {
		return nil, errors.New("response code 401 (Unauthorized): incorrect or missing credentials")
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	Logc(ctx).WithFields(log.Fields{
		"body": string(body),
	}).Debug("CliPassthroughResult")

	result := &CliPassthroughResult{}
	unmarshalErr := json.Unmarshal(body, result)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		return nil, unmarshalErr
	}

	return result, nil
}

type LunMappingGetCliPassthroughResult struct {
	Igroup         string   `json:"igroup"`
	Lun            string   `json:"lun"`
	Path           string   `json:"path"`
	Qtree          string   `json:"qtree"`
	ReportingNodes []string `json:"reporting_nodes"`
	Volume         string   `json:"volume"`
	Vserver        string   `json:"vserver"`
}

type LunGeometryCliPassthroughResult struct {
	// http://json2struct.mervine.net
	BytesPerSector     int64  `json:"bytes_per_sector"`
	CylSize            int64  `json:"cyl_size"`
	Cylinders          int64  `json:"cylinders"`
	DevSize            int64  `json:"dev_size"`
	Lun                string `json:"lun"`
	MaxResizeSize      int    `json:"max_resize_size"`
	Path               string `json:"path"`
	Qtree              string `json:"qtree"`
	Sectors            int64  `json:"sectors"`
	SectorsPerCylinder int64  `json:"sectors_per_cylinder"`
	SectorsPerTrack    int64  `json:"sectors_per_track"`
	Size               int64  `json:"size"`
	Volume             string `json:"volume"`
	Vserver            string `json:"vserver"`
}

// LunSize gets the size for a given LUN.
func (d RestClient) LunSize(
	ctx context.Context,
	lunPath string,
) (int, error) {

	lun, err := d.LunGetByName(ctx, lunPath)
	if err != nil {
		return 0, err
	}
	if lun == nil {
		return 0, fmt.Errorf("could not find lun with name %v", lunPath)
	}

	// TODO validate/improve this logic? int64 vs int
	return int(lun.Space.Size), nil
}

// LunSetSize sets the size for a given LUN.
func (d RestClient) LunSetSize(
	ctx context.Context,
	lunPath, newSize string,
) (uint64, error) {

	lun, err := d.LunGetByName(ctx, lunPath)
	if err != nil {
		return 0, err
	}
	if lun == nil {
		return 0, fmt.Errorf("could not find lun with name %v", lunPath)
	}

	uuid := lun.UUID

	params := san.NewLunModifyParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = uuid

	sizeBytesStr, _ := utils.ConvertSizeToBytes(newSize)
	sizeBytes, _ := strconv.ParseUint(sizeBytesStr, 10, 64)
	spaceInfo := &models.LunSpace{
		Size:      int64(sizeBytes),
	}
	lunInfo := &models.Lun{
		Space:        spaceInfo,
	}

	params.SetInfo(lunInfo)

	lunModifyOK, err := d.api.San.LunModify(params, d.authInfo)
	if err != nil {
		return 0, err
	}
	if lunModifyOK == nil {
		return 0, fmt.Errorf("unexpected response from LUN modify")
	}

	return sizeBytes, nil
}

//////////////////////////////////////////////////////////////////////////////
// NETWORK operations
//////////////////////////////////////////////////////////////////////////////

// NetworkIPInterfacesList lists all IP interfaces
func (d RestClient) NetworkIPInterfacesList(
	ctx context.Context,
) (*networking.NetworkIPInterfacesGetOK, error) {

	params := networking.NewNetworkIPInterfacesGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	params.SVMNameQueryParameter = &d.config.SVM
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := d.api.Networking.NetworkIPInterfacesGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NEXT_LOOP:
		for !done {
			resultNext, errNext := d.api.Networking.NetworkIPInterfacesGet(params, d.authInfo,
				WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NEXT_LOOP
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NEXT_LOOP
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

func (d RestClient) NetInterfaceGetDataLIFs(
	ctx context.Context,
	protocol string,
) ([]string, error) {

	if protocol == "" {
		return nil, fmt.Errorf("missing protocol specification")
	}

	params := networking.NewNetworkIPInterfacesGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	params.ServicesQueryParameter = ToStringPointer(fmt.Sprintf("data_%v", protocol))

	params.SVMNameQueryParameter = &d.config.SVM
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	lifResponse, err := d.api.Networking.NetworkIPInterfacesGet(params, d.authInfo)
	if err != nil {
		return nil, fmt.Errorf("error checking network interfaces: %v", err)
	}
	if lifResponse == nil {
		return nil, fmt.Errorf("unexpected error checking network interfaces")
	}

	dataLIFs := make([]string, 0)
	for _, record := range lifResponse.Payload.Records {
		if record.IP != nil {
			dataLIFs = append(dataLIFs, string(record.IP.Address))
		}
	}

	Logc(ctx).WithField("dataLIFs", dataLIFs).Debug("Data LIFs")
	return dataLIFs, nil
}

//////////////////////////////////////////////////////////////////////////////
// JOB operations
//////////////////////////////////////////////////////////////////////////////

// JobGet returns the job by ID
func (d *RestClient) JobGet(
	ctx context.Context,
	jobUUID string,
) (*cluster.JobGetOK, error) {

	params := cluster.NewJobGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = jobUUID

	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need, this forces ALL fields

	return d.api.Cluster.JobGet(params, d.authInfo)
}

// IsJobFinished lookus up the supplied JobLinkResponse's UUID to see if it's reached a terminal state
func (d *RestClient) IsJobFinished(
	ctx context.Context,
	payload *models.JobLinkResponse,
) (bool, error) {

	if payload == nil {
		return false, fmt.Errorf("payload is nil")
	}

	if payload.Job == nil {
		return false, fmt.Errorf("payload's Job is nil")
	}

	job := payload.Job
	jobUUID := job.UUID

	Logc(ctx).WithFields(log.Fields{
		"payload": payload,
		"job":     payload.Job,
		"jobUUID": jobUUID,
	}).Debug("IsJobFinished")

	jobResult, err := d.JobGet(ctx, string(jobUUID))
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

// PollJobStatus polls for the ONTAP job to complete, with backoff retry logic
func (d RestClient) PollJobStatus(
	ctx context.Context,
	payload *models.JobLinkResponse,
) error {

	job := payload.Job
	jobUUID := job.UUID

	checkJobStatus := func() error {
		isDone, err := d.IsJobFinished(ctx, payload)
		if err != nil {
			isDone = true
			return err
		}
		if !isDone {
			return fmt.Errorf("job %v not yet done", jobUUID)
		}
		return nil
	}
	jobStatusNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Job not yet done, waiting.")
	}
	jobStatusBackoff := backoff.NewExponentialBackOff()
	jobStatusBackoff.InitialInterval = 1 * time.Second
	jobStatusBackoff.Multiplier = 2
	jobStatusBackoff.RandomizationFactor = 0.1
	jobStatusBackoff.MaxElapsedTime = 2 * time.Minute

	// Run the job status check using an exponential backoff
	if err := backoff.RetryNotify(checkJobStatus, jobStatusBackoff, jobStatusNotify); err != nil {
		Logc(ctx).WithField("UUID", jobUUID).Warnf("Job not completed after %3.2f seconds.", jobStatusBackoff.MaxElapsedTime.Seconds())
		return err
	}

	Logc(ctx).WithField("UUID", jobUUID).Debug("Job completed.")
	jobResult, err := d.JobGet(ctx, string(jobUUID))
	if err != nil {
		return err
	}
	if jobResult == nil {
		return fmt.Errorf("missing job result for job UUID %v", jobUUID)
	}

	// EXAMPLE 1
	// 	"uuid": "493e64d1-99e2-11eb-9fc4-080027c8f2a7",
	// 	"description": "PATCH /api/storage/volumes/7d2fd988-8277-11eb-9fc4-080027c8f2a7",
	// 	"state": "failure",
	// 	"message": "entry doesn't exist",
	// 	"code": 4,
	// 	"start_time": "2021-04-10T09:51:13+00:00",
	// 	"end_time": "2021-04-10T09:51:13+00:00",

	// EXAMPLE 2
	// 	"uuid": "2453aafc-a9a6-11eb-9fc7-080027c8f2a7",
	// 	"description": "DELETE /api/storage/volumes/60018ffd-a9a3-11eb-9fc7-080027c8f2a7/snapshots/6365e696-a9a3-11eb-9fc7-080027c8f2a7",
	// 	"state": "failure",
	// 	"message": "Snapshot copy \"snapshot-60f627c7-576b-42a5-863e-9ea174856f2f\" of volume \"rippy_pvc_e8f1cc49_7949_403c_9f83_786d1480af38\" on Vserver \"nfs_vs\" has not expired or is locked. Use the \"snapshot show -fields owners, expiry-time\" command to view the expiry and lock status of the Snapshot copy.",
	// 	"code": 1638555,
	// 	"start_time": "2021-04-30T07:21:00-04:00",
	// 	"end_time": "2021-04-30T07:21:10-04:00",

	Logc(ctx).WithFields(log.Fields{
		"uuid":        job.UUID,
		"description": jobResult.Payload.Description,
		"state":       jobResult.Payload.State,
		"message":     jobResult.Payload.Message,
		"code":        jobResult.Payload.Code,
		"start_time":  jobResult.Payload.StartTime,
		"end_time":    jobResult.Payload.EndTime,
	}).Debug("Job completed.")

	switch jobState := jobResult.Payload.State; jobState {
	case models.JobStateSuccess:
		return nil
	case models.JobStateFailure:
		// TODO return a new error object that contains the message and code etc
		//return fmt.Errorf("%v", jobResult.Payload.Message)
		return NewRestErrorFromPayload(jobResult.Payload)
	default:
		return fmt.Errorf("unexpected job state %v", jobState)
	}
}

//////////////////////////////////////////////////////////////////////////////
// Aggregrate operations
//////////////////////////////////////////////////////////////////////////////

// AggregateList returns the names of all Aggregates whose names match the supplied pattern
func (d *RestClient) AggregateList(
	ctx context.Context,
	pattern string,
) (*storage.AggregateCollectionGetOK, error) {

	params := storage.NewAggregateCollectionGetParamsWithTimeout(d.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = d.httpClient

	params.SetNameQueryParameter(ToStringPointer(pattern))
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := d.api.Storage.AggregateCollectionGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NEXT_LOOP:
		for !done {
			resultNext, errNext := d.api.Storage.AggregateCollectionGet(params, d.authInfo,
				WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NEXT_LOOP
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NEXT_LOOP
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

//////////////////////////////////////////////////////////////////////////////
// SVM/Vserver operations
//////////////////////////////////////////////////////////////////////////////

// SvmGet gets the volume with the specified uuid
func (d RestClient) SvmGet(
	ctx context.Context,
	uuid string,
) (*svm.SvmGetOK, error) {

	params := svm.NewSvmGetParamsWithTimeout(d.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = uuid

	return d.api.Svm.SvmGet(params, d.authInfo)
}

// SvmList returns the names of all SVMs whose names match the supplied pattern
func (d *RestClient) SvmList(
	ctx context.Context,
	pattern string,
) (*svm.SvmCollectionGetOK, error) {

	params := svm.NewSvmCollectionGetParamsWithTimeout(d.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = d.httpClient

	params.SetNameQueryParameter(ToStringPointer(pattern))
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := d.api.Svm.SvmCollectionGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NEXT_LOOP:
		for !done {
			resultNext, errNext := d.api.Svm.SvmCollectionGet(params, d.authInfo,
				WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NEXT_LOOP
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NEXT_LOOP
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

// SvmGetByName gets the volume with the specified name
func (d *RestClient) SvmGetByName(
	ctx context.Context,
	svmName string,
) (*models.Svm, error) {

	// TODO validate/improve this logic?
	result, err := d.SvmList(ctx, svmName)
	if result.Payload.NumRecords == 1 && result.Payload.Records != nil {
		return result.Payload.Records[0], nil
	}
	return nil, err
}

func (d *RestClient) SVMGetAggregateNames(
	ctx context.Context,
) ([]string, error) {

	svm, err := d.SvmGetByName(ctx, d.config.SVM)
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
func (d *RestClient) ClusterInfo(
	ctx context.Context,
) (*cluster.ClusterGetOK, error) {

	params := cluster.NewClusterGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	return d.api.Cluster.ClusterGet(params, d.authInfo)
}

// SystemGetOntapVersion gets the ONTAP version using the credentials, and caches & returns the result.
func (d *RestClient) SystemGetOntapVersion(
	ctx context.Context,
) (string, error) {

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
func (d *RestClient) NodeList(
	ctx context.Context,
	pattern string,
) (*cluster.NodesGetOK, error) {

	params := cluster.NewNodesGetParamsWithTimeout(d.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = d.httpClient

	params.SetNameQueryParameter(ToStringPointer(pattern))
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := d.api.Cluster.NodesGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NEXT_LOOP:
		for !done {
			resultNext, errNext := d.api.Cluster.NodesGet(params, d.authInfo,
				WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NEXT_LOOP
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NEXT_LOOP
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

func (d *RestClient) NodeListSerialNumbers(
	ctx context.Context,
) ([]string, error) {

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

type CliPassthroughResult struct {
	CliOutput *string `json:"cli_output,omitempty"`
	Error     *struct {
		Code    string `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
	} `json:"error,omitempty"`
	Jobs []struct {
		Links struct {
			Self struct {
				Href string `json:"href,omitempty"`
			} `json:"self,omitempty"`
		} `json:"_links,omitempty"`
		UUID string `json:"uuid,omitempty"`
	} `json:"jobs,omitempty"`
	Links struct {
		Self struct {
			Href string `json:"href,omitempty"`
		} `json:"self,omitempty"`
		Next struct {
			Href string `json:"href,omitempty"`
		} `json:"next,omitempty"`
	} `json:"_links,omitempty"`
	//Records    map[string]json.RawMessage `json:"records,omitempty"`
	Records    []json.RawMessage `json:"records,omitempty"`
	NumRecords *int64            `json:"num_records,omitempty"`
}

func (d *RestClient) CliPassthroughVolumePatch(
	ctx context.Context,
	volumeName, jsonString string,
) (*CliPassthroughResult, error) {
	// See also:
	//   https://docs.netapp.com/us-en/ontap-automation/accessing_the_ontap_cli_through_the_rest_api.html
	//   https://library.netapp.com/ecmdocs/ECMLP2858435/html/resources/cli.html

	url := fmt.Sprintf(
		`https://%v/api/private/cli/volume?pretty=false&vserver=%v&volume=%v`,
		d.config.ManagementLIF,
		d.config.SVM,
		volumeName,
	)

	jsonBytes := []byte(jsonString)
	req, _ := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	if d.config.Username != "" && d.config.Password != "" {
		req.SetBasicAuth(d.config.Username, d.config.Password)
	}

	// certs will have been parsed and configured already, if needed, as part of the RestClient init
	tr := d.tr

	client := &http.Client{
		Transport: tr,
		Timeout:   time.Duration(tridentconfig.StorageAPITimeoutSeconds * time.Second),
	}

	response, err := client.Do(req)
	if err != nil {
		return nil, err
	} else if response.StatusCode == 401 {
		return nil, errors.New("response code 401 (Unauthorized): incorrect or missing credentials")
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	Logc(ctx).WithFields(log.Fields{
		"body": string(body),
	}).Debug("CliPassthroughResult")

	result := &CliPassthroughResult{}
	unmarshalErr := json.Unmarshal(body, result)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		return nil, unmarshalErr
	}

	return result, nil
}

type EMSEvent struct {
	MessageName string        `json:"message_name"`
	Values      []interface{} `json:"values"`
}

func (d *RestClient) CliPassthroughEventGeneratePost(
	ctx context.Context,
	appVersion string,
	autoSupport bool,
	category string,
	computerName string,
	eventDescription string,
	eventID int,
	eventSource string,
	logLevel int,
) (*CliPassthroughResult, error) {
	// See also:
	//   https://docs.netapp.com/us-en/ontap-automation/accessing_the_ontap_cli_through_the_rest_api.html
	//   https://library.netapp.com/ecmdocs/ECMLP2858435/html/resources/cli.html

	// 	jsonString := fmt.Sprintf(`{
	// 	"message_name": "app.log.notice",
	// 	"values": [
	// 		"trident-csi-767d8c9-8422p",
	// 		"trident",
	// 		"1",
	// 		1,
	// 		"heartbeat",
	// 		"{\"version\":\"21.04.0\",\"platform\":\"kubernetes\",\"platformVersion\":\"v1.17.8\",\"plugin\":\"ontap-san\",\"svm\":\"CXE\",\"storagePrefix\":\"trident_\"}"
	// 	]
	// }`)

	// computerName argument (type STRING) missing at position 1 (Client Computer connected to the Filer.)
	// eventSource argument (type STRING) missing at position 2 (Client application that generated this event.)
	// appVersion argument (type STRING) missing at position 3 (Client application version.)
	// eventID argument (type INT) missing at position 4 (Application eventID.)
	// category argument (type STRING) missing at position 5 (Event category.)
	// subject argument (type STRING) missing at position 6 (Event description.)

	// 	jsonString := fmt.Sprintf(`{
	// 	"message_name": "app.log.notice",
	// 	"values": [
	// 		"%v",
	// 		"%v",
	// 		"%v",
	// 		%v,
	// 		"%v",
	// 		"%v"
	// 	]
	// }`, computerName, eventSource, appVersion, eventID, category, eventDescription)
	// 	Logc(ctx).Debugf("jsonString: %v", jsonString)
	// 	jsonBytes := []byte(jsonString)
	// 	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))

	emsEvent := EMSEvent{
		MessageName: "app.log.notice",
		Values: []interface{}{
			computerName,
			eventSource,
			appVersion,
			eventID,
			category,
			eventDescription,
		},
	}

	payloadBuf := new(bytes.Buffer)
	json.NewEncoder(payloadBuf).Encode(emsEvent)

	Logc(ctx).Debugf("payloadBuf: %v", payloadBuf)

	url := fmt.Sprintf(
		`https://%v/api/private/cli/event/generate`,
		d.config.ManagementLIF,
	)
	req, _ := http.NewRequest("POST", url, payloadBuf)

	req.Header.Set("Content-Type", "application/json")
	if d.config.Username != "" && d.config.Password != "" {
		req.SetBasicAuth(d.config.Username, d.config.Password)
	}

	// certs will have been parsed and configured already, if needed, as part of the RestClient init
	tr := d.tr

	client := &http.Client{
		Transport: tr,
		Timeout:   time.Duration(tridentconfig.StorageAPITimeoutSeconds * time.Second),
	}

	response, err := client.Do(req)
	if err != nil {
		return nil, err
	} else if response.StatusCode == 401 {
		return nil, errors.New("response code 401 (Unauthorized): incorrect or missing credentials")
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	Logc(ctx).WithFields(log.Fields{
		"body": string(body),
	}).Debug("CliPassthroughResult")

	result := &CliPassthroughResult{}
	unmarshalErr := json.Unmarshal(body, result)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		return nil, unmarshalErr
	}

	return result, nil
}

// EmsAutosupportLog generates an auto support message with the supplied parameters
func (d *RestClient) EmsAutosupportLog(
	ctx context.Context,
	appVersion string,
	autoSupport bool,
	category string,
	computerName string,
	eventDescription string,
	eventID int,
	eventSource string,
	logLevel int,
) error {

	// TODO handle non-cluster-admin user error when trying to generate an EMS message
	_, err := d.CliPassthroughEventGeneratePost(ctx,
		appVersion,
		autoSupport,
		category,
		computerName,
		eventDescription,
		eventID,
		eventSource,
		logLevel,
	)

	return err
}

func (d RestClient) TieringPolicyValue(
	ctx context.Context,
) string {
	// The REST API is > ONTAP 9.5, just default to "none"
	tieringPolicy := "none"
	return tieringPolicy
}

/////////////////////////////////////////////////////////////////////////////
// EXPORT POLICY operations BEGIN

// ExportPolicyCreate creates an export policy
// equivalent to filer::> vserver export-policy create
func (d RestClient) ExportPolicyCreate(
	ctx context.Context,
	policy string,
) (*nas.ExportPolicyCreateCreated, error) {

	params := nas.NewExportPolicyCreateParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	exportPolicyInfo := &models.ExportPolicy{
		Name: policy,
		Svm: &models.ExportPolicySvm{
			Name: d.config.SVM,
		},
	}
	params.SetInfo(exportPolicyInfo)

	return d.api.Nas.ExportPolicyCreate(params, d.authInfo)
}

// ExportPolicyGet gets the export policy with the specified uuid
func (d RestClient) ExportPolicyGet(
	ctx context.Context,
	id int64,
) (*nas.ExportPolicyGetOK, error) {

	params := nas.NewExportPolicyGetParamsWithTimeout(d.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.IDPathParameter = id

	return d.api.Nas.ExportPolicyGet(params, d.authInfo)
}

// ExportPolicyList returns the names of all export polices whose names match the supplied pattern
func (d *RestClient) ExportPolicyList(
	ctx context.Context,
	pattern string,
) (*nas.ExportPolicyCollectionGetOK, error) {

	params := nas.NewExportPolicyCollectionGetParamsWithTimeout(d.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.SVMNameQueryParameter = &d.config.SVM

	params.SetNameQueryParameter(ToStringPointer(pattern))
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := d.api.Nas.ExportPolicyCollectionGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NEXT_LOOP:
		for !done {
			resultNext, errNext := d.api.Nas.ExportPolicyCollectionGet(params, d.authInfo,
				WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NEXT_LOOP
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NEXT_LOOP
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

// ExportPolicyGetByName gets the volume with the specified name
func (d *RestClient) ExportPolicyGetByName(
	ctx context.Context,
	exportPolicyName string,
) (*models.ExportPolicy, error) {

	// TODO validate/improve this logic?
	result, err := d.ExportPolicyList(ctx, exportPolicyName)
	if result.Payload.NumRecords == 1 && result.Payload.Records != nil {
		return result.Payload.Records[0], nil
	}
	return nil, err
}

func (d RestClient) ExportPolicyDestroy(
	ctx context.Context,
	policy string,
) (*nas.ExportPolicyDeleteOK, error) {

	exportPolicy, err := d.ExportPolicyGetByName(ctx, policy)
	if err != nil {
		return nil, err
	}
	if exportPolicy == nil {
		return nil, fmt.Errorf("could not get export policy %v", policy)
	}

	params := nas.NewExportPolicyDeleteParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.IDPathParameter = exportPolicy.ID

	return d.api.Nas.ExportPolicyDelete(params, d.authInfo)
}

// ExportRuleList returns the export rules in an export policy
// equivalent to filer::> vserver export-policy rule show
func (d RestClient) ExportRuleList(
	ctx context.Context,
	policy string,
) (*nas.ExportRuleCollectionGetOK, error) {

	exportPolicy, err := d.ExportPolicyGetByName(ctx, policy)
	if err != nil {
		return nil, err
	}
	if exportPolicy == nil {
		return nil, fmt.Errorf("could not get export policy %v", policy)
	}

	params := nas.NewExportRuleCollectionGetParamsWithTimeout(d.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.PolicyIDPathParameter = exportPolicy.ID

	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := d.api.Nas.ExportRuleCollectionGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NEXT_LOOP:
		for !done {
			resultNext, errNext := d.api.Nas.ExportRuleCollectionGet(params, d.authInfo,
				WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NEXT_LOOP
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NEXT_LOOP
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

// ExportRuleCreate creates a rule in an export policy
// equivalent to filer::> vserver export-policy rule create
func (d RestClient) ExportRuleCreate(
	ctx context.Context,
	policy, clientMatch string,
	protocols, roSecFlavors, rwSecFlavors, suSecFlavors []string,
) (*nas.ExportRuleCreateCreated, error) {

	exportPolicy, err := d.ExportPolicyGetByName(ctx, policy)
	if err != nil {
		return nil, err
	}
	if exportPolicy == nil {
		return nil, fmt.Errorf("could not get export policy %v", policy)
	}

	params := nas.NewExportRuleCreateParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.PolicyIDPathParameter = exportPolicy.ID

	info := &models.ExportRule{}

	var clients []*models.ExportClient
	for _, match := range strings.Split(clientMatch, ",") {
		clients = append(clients, &models.ExportClient{Match: match})
	}
	info.Clients = clients

	if len(protocols) > 0 {
		info.Protocols = ToSliceStringPointer(protocols)
	}
	if len(roSecFlavors) > 0 {
		info.RoRule = ToExportAuthenticationFlavorSlice(roSecFlavors)
	}
	if len(rwSecFlavors) > 0 {
		info.RwRule = ToExportAuthenticationFlavorSlice(rwSecFlavors)
	}
	if len(suSecFlavors) > 0 {
		info.Superuser = ToExportAuthenticationFlavorSlice(suSecFlavors)
	}
	params.SetInfo(info)

	return d.api.Nas.ExportRuleCreate(params, d.authInfo)
}

// ToExportAuthenticationFlavorSlice converts a slice of strings into a slice of ExportAuthenticationFlavor
func ToExportAuthenticationFlavorSlice(authFlavor []string) []models.ExportAuthenticationFlavor {
	var result []models.ExportAuthenticationFlavor
	for _, s := range authFlavor {
		v := models.ExportAuthenticationFlavor(s)
		switch v {
		case models.ExportAuthenticationFlavorAny:
			result = append(result, models.ExportAuthenticationFlavorAny)
		case models.ExportAuthenticationFlavorNone:
			result = append(result, models.ExportAuthenticationFlavorNone)
		case models.ExportAuthenticationFlavorNever:
			result = append(result, models.ExportAuthenticationFlavorNever)
		case models.ExportAuthenticationFlavorKrb5:
			result = append(result, models.ExportAuthenticationFlavorKrb5)
		case models.ExportAuthenticationFlavorKrb5i:
			result = append(result, models.ExportAuthenticationFlavorKrb5i)
		case models.ExportAuthenticationFlavorKrb5p:
			result = append(result, models.ExportAuthenticationFlavorKrb5p)
		case models.ExportAuthenticationFlavorNtlm:
			result = append(result, models.ExportAuthenticationFlavorNtlm)
		case models.ExportAuthenticationFlavorSys:
			result = append(result, models.ExportAuthenticationFlavorSys)
		}
	}
	return result
}

// ExportRuleDestroy deletes the rule at the given index in the given policy
func (d RestClient) ExportRuleDestroy(
	ctx context.Context,
	policy string, ruleIndex int,
) (*nas.ExportRuleDeleteOK, error) {

	exportPolicy, err := d.ExportPolicyGetByName(ctx, policy)
	if err != nil {
		return nil, err
	}
	if exportPolicy == nil {
		return nil, fmt.Errorf("could not get export policy %v", policy)
	}

	params := nas.NewExportRuleDeleteParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.PolicyIDPathParameter = exportPolicy.ID
	params.IndexPathParameter = int64(ruleIndex)

	return d.api.Nas.ExportRuleDelete(params, d.authInfo)
}

/////////////////////////////////////////////////////////////////////////////
// FlexGroup operations BEGIN

func ToSliceVolumeAggregatesItems(aggrs []string) []*models.VolumeAggregatesItems0 {
	var result []*models.VolumeAggregatesItems0
	for _, aggregateName := range aggrs {
		item := &models.VolumeAggregatesItems0{
			Name: aggregateName,
		}
		result = append(result, item)
	}
	return result
}

// FlexGroupCreate creates a FlexGroup with the specified options
// equivalent to filer::> volume create -vserver svm_name -volume fg_vol_name –auto-provision-as flexgroup -size fg_size
// -state online -type RW -policy default -unix-permissions ---rwxr-xr-x -space-guarantee none -snapshot-policy none
// -security-style unix -encrypt false
func (d RestClient) FlexGroupCreate(ctx context.Context, name string, size int, aggrs []string, spaceReserve,
	snapshotPolicy, unixPermissions, exportPolicy, securityStyle, tieringPolicy, comment string,
	qosPolicyGroup QosPolicyGroup, encrypt bool, snapshotReserve int) error {
	return d.createVolumeByStyle(ctx, name, int64(size), aggrs, spaceReserve, snapshotPolicy, unixPermissions,
		exportPolicy, securityStyle, tieringPolicy, comment, qosPolicyGroup, encrypt,
		snapshotReserve, models.VolumeStyleFlexgroup)
}

// FlexgroupCloneSplitStart starts splitting the flexgroup clone
func (d RestClient) FlexgroupCloneSplitStart(ctx context.Context, volumeName string) error {
	return d.startCloneSplitByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexgroup)
}

// FlexGroupDestroy destroys a FlexGroup
func (d RestClient) FlexGroupDestroy(ctx context.Context, name string) error {
	volume, err := d.FlexGroupGetByName(ctx, name)
	if err != nil {
		return err
	}
	params := storage.NewVolumeDeleteParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = volume.UUID

	volumeDeleteAccepted, err := d.api.Storage.VolumeDelete(params, d.authInfo)
	if err != nil {
		return err
	}
	if volumeDeleteAccepted == nil {
		return fmt.Errorf("unexpected response from volume delete")
	}

	return d.PollJobStatus(ctx, volumeDeleteAccepted.Payload)
}

// FlexGroupExists tests for the existence of a FlexGroup
func (d RestClient) FlexGroupExists(ctx context.Context, volumeName string) (bool, error) {
	return d.checkVolumeExistsByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexgroup)
}

// FlexGroupSize retrieves the size of the specified flexgroup
func (d RestClient) FlexGroupSize(ctx context.Context, volumeName string) (uint64, error) {
	return d.getVolumeSizeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexgroup)
}

// FlexGroupUsedSize retrieves the used space of the specified volume
func (d RestClient) FlexGroupUsedSize(ctx context.Context, volumeName string) (int, error) {
	return d.getVolumeUsedSizeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexgroup)
}

// FlexGroupSetSize sets the size of the specified FlexGroup
func (d RestClient) FlexGroupSetSize(ctx context.Context, volumeName, newSize string) error {
	return d.setVolumeSizeByNameAndStyle(ctx, volumeName, newSize, models.VolumeStyleFlexgroup)
}

// FlexgroupSetQosPolicyGroupName note: we can't set adaptive policy groups directly during volume clone creation.
func (d RestClient) FlexgroupSetQosPolicyGroupName(
	ctx context.Context, volumeName string, qosPolicyGroup QosPolicyGroup,
) error {
	return d.setVolumeQosPolicyGroupNameByNameAndStyle(ctx, volumeName, qosPolicyGroup, models.VolumeStyleFlexgroup)
}

// FlexGroupVolumeDisableSnapshotDirectoryAccess disables access to the ".snapshot" directory
// Disable '.snapshot' to allow official mysql container's chmod-in-init to work
func (d RestClient) FlexGroupVolumeDisableSnapshotDirectoryAccess(ctx context.Context, name string) error {
	return d.VolumeDisableSnapshotDirectoryAccess(ctx, name)
}

func (d RestClient) FlexGroupModifyUnixPermissions(ctx context.Context, volumeName, unixPermissions string) error {
	return d.modifyVolumeUnixPermissionsByNameAndStyle(ctx, volumeName, unixPermissions, models.VolumeStyleFlexgroup)
}

// FlexGroupSetComment sets a flexgroup's comment to the supplied value
func (d RestClient) FlexGroupSetComment(ctx context.Context, volumeName, newVolumeComment string) error {
	return d.setVolumeCommentByNameAndStyle(ctx, volumeName, newVolumeComment, models.VolumeStyleFlexgroup)
}

// FlexGroupGetByName gets the flexgroup with the specified name
func (d RestClient) FlexGroupGetByName(ctx context.Context, volumeName string) (*models.Volume, error) {
	return d.getVolumeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexgroup)
}

// FlexGroupGetAll returns all relevant details for all FlexGroups whose names match the supplied prefix
func (d RestClient) FlexGroupGetAll(ctx context.Context, pattern string) (*storage.VolumeCollectionGetOK, error) {
	return d.getAllVolumesByPatternStyleAndState(ctx, pattern, models.VolumeStyleFlexgroup, models.VolumeStateOnline)
}

// FlexGroupMount mounts a flexgroup at the specified junction
func (d RestClient) FlexGroupMount(ctx context.Context, volumeName, junctionPath string) error {
	return d.mountVolumeByNameAndStyle(ctx, volumeName, junctionPath, models.VolumeStyleFlexgroup)
}

// FlexgroupUnmount unmounts the flexgroup
func (d RestClient) FlexgroupUnmount(ctx context.Context, volumeName string) error {
	return d.unmountVolumeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexgroup)
}

func (d RestClient) FlexgroupModifyExportPolicy(ctx context.Context, volumeName, exportPolicyName string) error {
	return d.modifyVolumeExportPolicyByNameAndStyle(ctx, volumeName, exportPolicyName, models.VolumeStyleFlexgroup)
}

/////////////////////////////////////////////////////////////////////////////
// FlexGroup operations END
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// QTREE operations BEGIN

// QtreeCreate creates a qtree with the specified options
// equivalent to filer::> qtree create -vserver ndvp_vs -volume v -qtree q -export-policy default -unix-permissions ---rwxr-xr-x -security-style unix
func (d RestClient) QtreeCreate(
	ctx context.Context,
	name, volumeName, unixPermissions, exportPolicy, securityStyle, qosPolicy string) error {

	params := storage.NewQtreeCreateParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	qtreeInfo := &models.Qtree{
		Name: name,
		Volume: &models.QtreeVolume{
			Name: volumeName,
		},
	}

	qtreeInfo.Svm = &models.QtreeSvm{Name: d.config.SVM}

	// handle options
	if unixPermissions != "" {
		unixPermissions = convertUnixPermissions(unixPermissions)
		volumePermissions, parseErr := strconv.ParseInt(unixPermissions, 10, 64)
		if parseErr != nil {
			return fmt.Errorf("cannot process unix permissions value %v", unixPermissions)
		}
		qtreeInfo.UnixPermissions = volumePermissions
	}
	if exportPolicy != "" {
		qtreeInfo.ExportPolicy = &models.QtreeExportPolicy{
			Name: exportPolicy,
		}
	}
	if securityStyle != "" {
		qtreeInfo.SecurityStyle = models.SecurityStyle(securityStyle)
	}

	// TODO handle qosPolicy (it's missing from the generated bindings)

	params.SetInfo(qtreeInfo)

	createAccepted, err := d.api.Storage.QtreeCreate(params, d.authInfo)
	if err != nil {
		return err
	}
	if createAccepted == nil {
		return fmt.Errorf("unexpected response from qtree create")
	}

	return d.PollJobStatus(ctx, createAccepted.Payload)
}

// QtreeRename renames a qtree
// equivalent to filer::> volume qtree rename
func (d RestClient) QtreeRename(
	ctx context.Context,
	path, newPath string) error {

	qtree, err := d.QtreeGetByPath(ctx, path)
	if err != nil {
		return err
	}
	if qtree == nil {
		return fmt.Errorf("could not find qtree with path %v", path)
	}

	// TODO add sanity checks around qtree.ID and qtree.Volume?
	params := storage.NewQtreeModifyParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.IDPathParameter = strconv.FormatInt(int64(qtree.ID), 10)
	params.VolumeUUIDPathParameter = qtree.Volume.UUID

	qtreeInfo := &models.Qtree{
		Name: strings.TrimPrefix(newPath, "/"+qtree.Volume.Name+"/"),
	}

	params.SetInfo(qtreeInfo)

	modifyAccepted, err := d.api.Storage.QtreeModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if modifyAccepted == nil {
		return fmt.Errorf("unexpected response from qtree modify")
	}

	return d.PollJobStatus(ctx, modifyAccepted.Payload)
}

// QtreeDestroyAsync destroys a qtree in the background
// equivalent to filer::> volume qtree delete -foreground false
func (d RestClient) QtreeDestroyAsync(
	ctx context.Context,
	path string, force bool) error {

	// TODO force isn't used

	qtree, err := d.QtreeGetByPath(ctx, path)
	if err != nil {
		return err
	}
	if qtree == nil {
		return fmt.Errorf("unexpected response from qtree lookup by path")
	}
	if qtree.Volume == nil {
		return fmt.Errorf("unexpected response from qtree lookup by path, missing volume information")
	}

	params := storage.NewQtreeDeleteParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.IDPathParameter = strconv.FormatInt(int64(qtree.ID), 10)
	params.VolumeUUIDPathParameter = qtree.Volume.UUID

	deleteAccepted, err := d.api.Storage.QtreeDelete(params, d.authInfo)
	if err != nil {
		return err
	}
	if deleteAccepted == nil {
		return fmt.Errorf("unexpected response from quota delete")
	}

	return d.PollJobStatus(ctx, deleteAccepted.Payload)
}

// QtreeList returns the names of all Qtrees whose names match the supplied prefix
// equivalent to filer::> volume qtree show
func (d RestClient) QtreeList(
	ctx context.Context,
	prefix, volumePrefix string) (*storage.QtreeCollectionGetOK, error) {

	namePattern := "*"
	if prefix != "" {
		namePattern = prefix + "*"
	}

	volumePattern := "*"
	if volumePrefix != "" {
		volumePattern = volumePrefix + "*"
	}

	// Limit the qtrees to those matching the Flexvol name prefix
	params := storage.NewQtreeCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	//params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SVMNameQueryParameter = &d.config.SVM
	params.SetNameQueryParameter(ToStringPointer(namePattern))         // Qtree name prefix
	params.SetVolumeNameQueryParameter(ToStringPointer(volumePattern)) // Flexvol name prefix
	params.SetFieldsQueryParameter([]string{"**"})                     // TODO trim these down to just what we need

	result, err := d.api.Storage.QtreeCollectionGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NEXT_LOOP:
		for !done {
			resultNext, errNext := d.api.Storage.QtreeCollectionGet(params, d.authInfo,
				WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NEXT_LOOP
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NEXT_LOOP
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

// QtreeGetByPath gets the qtree with the specified path
func (d RestClient) QtreeGetByPath(
	ctx context.Context,
	path string,
) (*models.Qtree, error) {

	// Limit the qtrees to those specified path
	params := storage.NewQtreeCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	//params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SVMNameQueryParameter = &d.config.SVM
	params.SetPathQueryParameter(ToStringPointer(path))
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := d.api.Storage.QtreeCollectionGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if result.Payload == nil {
		return nil, fmt.Errorf("qtree path %s not found", path)
	} else if len(result.Payload.Records) > 1 {
		return nil, fmt.Errorf("more than one qtree at path %s found", path)
	} else if len(result.Payload.Records) == 1 {
		return result.Payload.Records[0], nil
	} else if HasNextLink(result.Payload) {
		return nil, fmt.Errorf("more than one qtree at path %s found", path)
	}

	return nil, fmt.Errorf("qtree path %s not found", path)
}

// QtreeGetByName gets the qtree with the specified name in the specified volume
func (d RestClient) QtreeGetByName(
	ctx context.Context,
	name, volumeName string,
) (*models.Qtree, error) {

	// Limit to the single qtree /volumeName/name
	params := storage.NewQtreeCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	//params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SVMNameQueryParameter = &d.config.SVM
	params.SetNameQueryParameter(ToStringPointer(name))
	params.SetVolumeNameQueryParameter(ToStringPointer(volumeName))
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := d.api.Storage.QtreeCollectionGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if result.Payload == nil {
		return nil, fmt.Errorf("qtree %s not found", name)
	} else if len(result.Payload.Records) > 1 {
		return nil, fmt.Errorf("more than one qtree %s found", name)
	} else if len(result.Payload.Records) == 1 {
		return result.Payload.Records[0], nil
	} else if HasNextLink(result.Payload) {
		return nil, fmt.Errorf("more than one qtree %s found", name)
	}

	return nil, fmt.Errorf("qtree %s not found", name)
}

// QtreeCount returns the number of Qtrees in the specified Flexvol, not including the Flexvol itself
func (d RestClient) QtreeCount(
	ctx context.Context,
	volume string) (int, error) {

	// Limit the qtrees to those in the specified Flexvol name
	params := storage.NewQtreeCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	//params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SVMNameQueryParameter = &d.config.SVM
	params.SetVolumeNameQueryParameter(ToStringPointer(volume)) // Flexvol name
	params.SetFieldsQueryParameter([]string{"**"})              // TODO trim these down to just what we need

	result, err := d.api.Storage.QtreeCollectionGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return 0, err
	}
	if result == nil {
		return 0, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NEXT_LOOP:
		for !done {
			resultNext, errNext := d.api.Storage.QtreeCollectionGet(params, d.authInfo,
				WithNextLink(nextLink))
			if errNext != nil {
				return 0, errNext
			}
			if resultNext == nil {
				done = true
				continue NEXT_LOOP
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NEXT_LOOP
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}

	// There will always be one qtree for the Flexvol, so decrement by 1
	n := len(result.Payload.Records)
	switch n {
	case 0:
		fallthrough
	case 1:
		return 0, nil
	default:
		return n - 1, nil
	}
}

// QtreeExists returns true if the named Qtree exists (and is unique in the matching Flexvols)
func (d RestClient) QtreeExists(
	ctx context.Context,
	name, volumePrefix string) (bool, string, error) {

	volumePattern := "*"
	if volumePrefix != "" {
		volumePattern = volumePrefix + "*"
	}

	// Limit the qtrees to those matching the Flexvol and Qtree name prefixes
	params := storage.NewQtreeCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	//params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SVMNameQueryParameter = &d.config.SVM
	params.SetNameQueryParameter(ToStringPointer(name))                // Qtree name
	params.SetVolumeNameQueryParameter(ToStringPointer(volumePattern)) // Flexvol name prefix
	params.SetFieldsQueryParameter([]string{"**"})                     // TODO trim these down to just what we need

	result, err := d.api.Storage.QtreeCollectionGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return false, "", err
	}
	if result == nil {
		return false, "", nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NEXT_LOOP:
		for !done {
			resultNext, errNext := d.api.Storage.QtreeCollectionGet(params, d.authInfo,
				WithNextLink(nextLink))
			if errNext != nil {
				return false, "", errNext
			}
			if resultNext == nil {
				done = true
				continue NEXT_LOOP
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NEXT_LOOP
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}

	if result.Payload == nil {
		return false, "", nil
	}

	// Ensure qtree is unique
	n := result.Payload.NumRecords
	if n != 1 {
		return false, "", nil
	}

	// Get containing Flexvol
	volume := result.Payload.Records[0].Volume
	if volume == nil {
		return false, "", nil
	}
	flexvol := volume.Name

	return true, flexvol, nil
}

// QtreeGet returns all relevant details for a single qtree
// equivalent to filer::> volume qtree show
func (d RestClient) QtreeGet(
	ctx context.Context,
	name, volumePrefix string) (*models.Qtree, error) {

	pattern := "*"
	if volumePrefix != "" {
		pattern = volumePrefix + "*"
	}

	// Limit the qtrees to those matching the Flexvol name prefix
	params := storage.NewQtreeCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	//params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SVMNameQueryParameter = &d.config.SVM
	params.SetNameQueryParameter(ToStringPointer(name))          // qtree name
	params.SetVolumeNameQueryParameter(ToStringPointer(pattern)) // Flexvol name prefix
	params.SetFieldsQueryParameter([]string{"**"})               // TODO trim these down to just what we need

	result, err := d.api.Storage.QtreeCollectionGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if result.Payload == nil {
		return nil, fmt.Errorf("qtree %s not found", name)
	} else if len(result.Payload.Records) > 1 {
		return nil, fmt.Errorf("more than one qtree %s found", name)
	} else if len(result.Payload.Records) == 1 {
		return result.Payload.Records[0], nil
	} else if HasNextLink(result.Payload) {
		return nil, fmt.Errorf("more than one qtree %s found", name)
	}

	return nil, fmt.Errorf("qtree %s not found", name)
}

// QtreeGetAll returns all relevant details for all qtrees whose Flexvol names match the supplied prefix
// equivalent to filer::> volume qtree show
func (d RestClient) QtreeGetAll(
	ctx context.Context,
	volumePrefix string) (*storage.QtreeCollectionGetOK, error) {

	pattern := "*"
	if volumePrefix != "" {
		pattern = volumePrefix + "*"
	}

	// Limit the qtrees to those matching the Flexvol name prefix
	params := storage.NewQtreeCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	//params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SVMNameQueryParameter = &d.config.SVM
	params.SetVolumeNameQueryParameter(ToStringPointer(pattern)) // Flexvol name prefix
	params.SetFieldsQueryParameter([]string{"**"})               // TODO trim these down to just what we need

	result, err := d.api.Storage.QtreeCollectionGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NEXT_LOOP:
		for !done {
			resultNext, errNext := d.api.Storage.QtreeCollectionGet(params, d.authInfo,
				WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NEXT_LOOP
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NEXT_LOOP
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

// QtreeModifyExportPolicy modifies the export policy for the qtree
func (d RestClient) QtreeModifyExportPolicy(
	ctx context.Context,
	name, volumeName, newExportPolicyName string) error {

	qtree, err := d.QtreeGetByName(ctx, name, volumeName)
	if err != nil {
		return err
	}
	if qtree == nil {
		return fmt.Errorf("could not find qtree %v", name)
	}

	// TODO add sanity checks around qtree.ID and qtree.Volume?
	params := storage.NewQtreeModifyParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.IDPathParameter = strconv.FormatInt(int64(qtree.ID), 10)
	params.VolumeUUIDPathParameter = qtree.Volume.UUID

	qtreeInfo := &models.Qtree{
		ExportPolicy: &models.QtreeExportPolicy{
			Name: newExportPolicyName,
		},
	}

	params.SetInfo(qtreeInfo)

	modifyAccepted, err := d.api.Storage.QtreeModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if modifyAccepted == nil {
		return fmt.Errorf("unexpected response from qtree modify")
	}

	return d.PollJobStatus(ctx, modifyAccepted.Payload)
}

// QuotaOn enables quotas on a Flexvol
// equivalent to filer::> volume quota on
func (d RestClient) QuotaOn(
	ctx context.Context,
	volumeName string) error {
	return d.quotaModify(ctx, volumeName, true)
}

// QuotaOff disables quotas on a Flexvol
// equivalent to filer::> volume quota off
func (d RestClient) QuotaOff(
	ctx context.Context,
	volumeName string) error {
	return d.quotaModify(ctx, volumeName, false)
}

// quotaModify enables/disables quotas on a Flexvol
func (d RestClient) quotaModify(
	ctx context.Context,
	volumeName string,
	quotaEnabled bool) error {

	volume, err := d.VolumeGetByName(ctx, volumeName)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}

	if volume.Quota.Enabled == quotaEnabled {
		// nothing to do, already the specified value
		return nil
	}

	uuid := volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient
	params.UUIDPathParameter = uuid

	volumeInfo := &models.Volume{
		Quota: &models.VolumeQuota{
			Enabled: quotaEnabled,
		},
	}

	params.SetInfo(volumeInfo)

	modifyAccepted, err := d.api.Storage.VolumeModify(params, d.authInfo)
	if err != nil {
		return err
	}
	if modifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return d.PollJobStatus(ctx, modifyAccepted.Payload)
}

// QuotaResize resizes quotas on a Flexvol
// equivalent to filer::> volume quota resize
// func (d RestClient) QuotaResize(
// 	ctx context.Context,
// 	volume string) (*azgo.QuotaResizeResponse, error) {
// 	response, err := azgo.NewQuotaResizeRequest().
// 		SetVolume(volume).
// 		ExecuteUsing(d.zr)
// 	return response, err
// }

// QuotaStatus returns the quota status for a Flexvol
// equivalent to filer::> volume quota show
// func (d RestClient) QuotaStatus(
// 	ctx context.Context,
// 	volume string) (*azgo.QuotaStatusResponse, error) {
// 	response, err := azgo.NewQuotaStatusRequest().
// 		SetVolume(volume).
// 		ExecuteUsing(d.zr)
// 	return response, err
// }

// QuotaSetEntry creates a new quota rule with an optional hard disk limit
// equivalent to filer::> volume quota policy rule create
func (d RestClient) QuotaSetEntry(
	ctx context.Context,
	qtreeName, volumeName, quotaTarget, quotaType, diskLimit string) error {

	params := storage.NewQuotaRuleCreateParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	quotaRuleInfo := &models.QuotaRule{
		Qtree: &models.QuotaRuleQtree{
			Name: qtreeName,
		},
		Volume: &models.QuotaRuleVolume{
			Name: volumeName,
		},
		Type: quotaType,
	}

	quotaRuleInfo.Svm = &models.QuotaRuleSvm{Name: d.config.SVM}

	// handle options
	if diskLimit != "" {
		hardLimit, parseErr := strconv.ParseInt(diskLimit, 10, 64)
		if parseErr != nil {
			return fmt.Errorf("cannot process hard disk limit value %v", diskLimit)
		}
		quotaRuleInfo.Space = &models.QuotaRuleSpace{
			HardLimit: hardLimit,
		}
	}
	// TODO handle quotaTarget

	params.SetInfo(quotaRuleInfo)

	createAccepted, err := d.api.Storage.QuotaRuleCreate(params, d.authInfo)
	if err != nil {
		return err
	}
	if createAccepted == nil {
		return fmt.Errorf("unexpected response from quota rule create")
	}

	return d.PollJobStatus(ctx, createAccepted.Payload)
}

// QuotaEntryGet returns the disk limit for a single qtree
// equivalent to filer::> volume quota policy rule show
func (d RestClient) QuotaGetEntry(
	ctx context.Context,
	target string) (*models.QuotaRule, error) {

	qtree, err := d.QtreeGetByPath(ctx, target)
	if err != nil {
		return nil, err
	}
	if qtree == nil {
		return nil, fmt.Errorf("could not find qtree with path %v", target)
	}

	params := storage.NewQuotaRuleCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	//params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	// TODO add sanity checks around qtree.ID and qtree.Volume?
	params.SVMNameQueryParameter = &d.config.SVM
	params.QtreeIDQueryParameter = &qtree.ID
	params.VolumeUUIDQueryParameter = &qtree.Volume.UUID

	// TODO Limit the returned data to only the disk limit
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := d.api.Storage.QuotaRuleCollectionGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NEXT_LOOP:
		for !done {
			resultNext, errNext := d.api.Storage.QuotaRuleCollectionGet(params, d.authInfo,
				WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NEXT_LOOP
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NEXT_LOOP
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}

	if result.Payload == nil {
		return nil, fmt.Errorf("quota rule entries for %s not found", target)
	} else if len(result.Payload.Records) > 1 {
		return nil, fmt.Errorf("more than one quota rule entry for %s found", target)
	} else if len(result.Payload.Records) == 1 {
		return result.Payload.Records[0], nil
	} else if HasNextLink(result.Payload) {
		return nil, fmt.Errorf("more than one quota rule entry for %s found", target)
	}

	return nil, fmt.Errorf("no entries for %s", target)
}

// QuotaEntryList returns the disk limit quotas for a Flexvol
// equivalent to filer::> volume quota policy rule show
func (d RestClient) QuotaEntryList(
	ctx context.Context,
	volumeName string) (*storage.QuotaRuleCollectionGetOK, error) {

	params := storage.NewQuotaRuleCollectionGetParamsWithTimeout(d.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = d.httpClient

	//params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SVMNameQueryParameter = &d.config.SVM
	params.VolumeNameQueryParameter = ToStringPointer(volumeName)

	// TODO Limit the returned data to only the disk limit
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := d.api.Storage.QuotaRuleCollectionGet(params, d.authInfo)

	// TODO refactor to remove duplication
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NEXT_LOOP:
		for !done {
			resultNext, errNext := d.api.Storage.QuotaRuleCollectionGet(params, d.authInfo,
				WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NEXT_LOOP
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NEXT_LOOP
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}

	return result, nil
}

// QTREE operations END
/////////////////////////////////////////////////////////////////////////////
