// Copyright 2022 NetApp, Inc. All Rights Reserved.

package api

import (
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
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-openapi/runtime"
	runtime_client "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/cluster"
	nas "github.com/netapp/trident/storage_drivers/ontap/api/rest/client/n_a_s"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/networking"
	san "github.com/netapp/trident/storage_drivers/ontap/api/rest/client/s_a_n"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/storage"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/support"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/svm"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/netapp/trident/utils"
	versionutils "github.com/netapp/trident/utils/version"
)

// //////////////////////////////////////////////////////////////////////////////////////////////////////
// REST layer
// //////////////////////////////////////////////////////////////////////////////////////////////////////

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

// RestClient is the object to use for interacting with ONTAP controllers via the REST API
type RestClient struct {
	config       ClientConfig
	tr           *http.Transport
	httpClient   *http.Client
	api          *client.ONTAPRESTAPIOnlineReference
	authInfo     runtime.ClientAuthInfoWriter
	OntapVersion string
	driverName   string
	svmUUID      string
	svmName      string
}

func (c RestClient) ClientConfig() ClientConfig {
	return c.config
}

func (c *RestClient) SetSVMUUID(svmUUID string) {
	c.svmUUID = svmUUID
}

func (c *RestClient) SVMUUID() string {
	return c.svmUUID
}

func (c *RestClient) SetSVMName(svmName string) {
	c.svmName = svmName
}

func (c *RestClient) SVMName() string {
	return c.svmName
}

// NewRestClient is a factory method for creating a new instance
func NewRestClient(ctx context.Context, config ClientConfig, SVM, driverName string) (*RestClient, error) {
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
		config:     config,
		svmName:    SVM,
		driverName: driverName,
	}

	result.tr = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: skipVerify,
			MinVersion:         tridentconfig.MinClientTLSVersion,
			Certificates:       []tls.Certificate{cert},
			RootCAs:            caCertPool,
		},
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

	if rClient, ok := result.api.Transport.(*runtime_client.Runtime); ok {
		rClient.SetLogger(log.New())
		rClient.SetDebug(config.DebugTraceFlags["api"])
	}

	return result, nil
}

// EnsureSVMWithRest uses the supplied SVM or attempts to derive one if no SVM is supplied
func EnsureSVMWithRest(
	ctx context.Context, ontapConfig *drivers.OntapStorageDriverConfig, restClient RestClientInterface,
) error {
	if ontapConfig.SVM != "" {
		// Attempt to use the specified SVM
		vserver, err := restClient.SvmGetByName(ctx, ontapConfig.SVM)
		if err != nil {
			return fmt.Errorf("unable to get details for SVM %v; %v", ontapConfig.SVM, err)
		}
		restClient.SetSVMUUID(vserver.UUID)

		Logc(ctx).WithFields(
			LogFields{
				"SVM":  ontapConfig.SVM,
				"UUID": vserver.UUID,
			},
		).Debug("Using specified SVM.")
		return nil

	} else {
		// Attempt to derive the SVM
		result, err := restClient.SvmList(ctx, "*")
		if err != nil {
			return err
		}

		errorMsg := "cannot derive SVM to use; please specify SVM in config file"
		if validationErr := ValidatePayloadExists(ctx, result); validationErr != nil {
			return fmt.Errorf("%s; %v", errorMsg, validationErr)
		}

		if result.Payload.Records == nil || result.Payload.NumRecords != 1 {
			// more than result, not going to try and guess
			return errors.New(errorMsg)
		}

		// Use our derived SVM
		derivedSVM := result.Payload.Records[0]
		ontapConfig.SVM = derivedSVM.Name
		restClient.SetSVMName(derivedSVM.Name)
		svmUUID := derivedSVM.UUID
		restClient.SetSVMUUID(svmUUID)

		Logc(ctx).WithFields(
			LogFields{
				"SVM":  ontapConfig.SVM,
				"UUID": svmUUID,
			},
		).Debug("Using derived SVM.")
		return nil
	}
}

// NewRestClientFromOntapConfig is a factory method for creating a new Ontap API instance with a REST client
func NewRestClientFromOntapConfig(
	ctx context.Context, ontapConfig *drivers.OntapStorageDriverConfig,
) (OntapAPI, error) {
	restClient, err := NewRestClient(ctx, ClientConfig{
		ManagementLIF:        ontapConfig.ManagementLIF,
		Username:             ontapConfig.Username,
		Password:             ontapConfig.Password,
		ClientPrivateKey:     ontapConfig.ClientPrivateKey,
		ClientCertificate:    ontapConfig.ClientCertificate,
		TrustedCACertificate: ontapConfig.TrustedCACertificate,
		DebugTraceFlags:      ontapConfig.DebugTraceFlags,
	}, ontapConfig.SVM, ontapConfig.StorageDriverName)
	if err != nil {
		return nil, fmt.Errorf("could not instantiate REST client; %s", err.Error())
	}

	if restClient == nil {
		return nil, fmt.Errorf("could not instantiate REST client")
	}

	if err := EnsureSVMWithRest(ctx, ontapConfig, restClient); err != nil {
		return nil, err
	}

	apiREST, err := NewOntapAPIREST(restClient, ontapConfig.StorageDriverName)
	if err != nil {
		return nil, fmt.Errorf("unable to get REST API client for ontap: %v", err)
	}

	return apiREST, nil
}

var MinimumONTAPVersion = versionutils.MustParseSemantic("9.11.1")

// SupportsFeature returns true if the Ontap version supports the supplied feature
func (c RestClient) SupportsFeature(ctx context.Context, feature Feature) bool {
	ontapVersion, err := c.SystemGetOntapVersion(ctx)
	if err != nil {
		return false
	}

	ontapSemVer, err := versionutils.ParseSemantic(ontapVersion)
	if err != nil {
		return false
	}

	if feature == LunGeometrySkip {
		return !ontapSemVer.AtLeast(versionutils.MustParseSemantic("9.11.0"))
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

// ////////////////////////////////////////////////////////////////////////////
// NAS VOLUME operations by style (flexgroup and flexvol)
// ////////////////////////////////////////////////////////////////////////////

func (c RestClient) getAllVolumePayloadRecords(
	payload *models.VolumeResponse,
	params *storage.VolumeCollectionGetParams,
) (*models.VolumeResponse, error) {
	if HasNextLink(payload) {
		nextLink := payload.Links.Next

		for {
			resultNext, errNext := c.api.Storage.VolumeCollectionGet(params, c.authInfo, WithNextLink(nextLink))
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

// getAllVolumesByPatternAndStyle returns all relevant details for all volumes of the style specified whose names match the supplied prefix
func (c RestClient) getAllVolumesByPatternStyleAndState(
	ctx context.Context, pattern, style, state string,
) (*storage.VolumeCollectionGetOK, error) {
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

	params := storage.NewVolumeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	// params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SVMNameQueryParameter = &c.svmName
	params.SetNameQueryParameter(ToStringPointer(pattern))
	if state != "" {
		params.SetStateQueryParameter(ToStringPointer(state))
	}
	params.SetStyleQueryParameter(ToStringPointer(style))
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := c.api.Storage.VolumeCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	result.Payload, err = c.getAllVolumePayloadRecords(result.Payload, params)
	if err != nil {
		return result, err
	}

	return result, nil
}

// checkVolumeExistsByNameAndStyle tests for the existence of a volume of the style and name specified
func (c RestClient) checkVolumeExistsByNameAndStyle(ctx context.Context, volumeName, style string) (bool, error) {
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style)
	if err != nil {
		return false, err
	}
	if volume == nil {
		return false, err
	}
	return true, nil
}

// getVolumeByNameAndStyle gets the volume of the style and name specified
func (c RestClient) getVolumeByNameAndStyle(
	ctx context.Context,
	volumeName string,
	style string,
) (*models.Volume, error) {
	result, err := c.getAllVolumesByPatternStyleAndState(ctx, volumeName, style, models.VolumeStateOnline)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Payload == nil || result.Payload.NumRecords == 0 {
		return nil, nil
	}
	if result.Payload.NumRecords == 1 && result.Payload.Records != nil {
		return result.Payload.Records[0], nil
	}
	return nil, fmt.Errorf("could not find unique volume with name '%v'; found %d matching volumes",
		volumeName, result.Payload.NumRecords)
}

// getVolumeInAnyStateByNameAndStyle gets the volume of the style and name specified
func (c RestClient) getVolumeInAnyStateByNameAndStyle(
	ctx context.Context,
	volumeName string,
	style string,
) (*models.Volume, error) {
	result, err := c.getAllVolumesByPatternStyleAndState(ctx, volumeName, style, "")
	if err != nil {
		return nil, err
	}
	if result == nil || result.Payload == nil || result.Payload.NumRecords == 0 {
		return nil, nil
	}
	if result.Payload.NumRecords == 1 && result.Payload.Records != nil {
		return result.Payload.Records[0], nil
	}
	return nil, fmt.Errorf("could not find unique volume with name '%v'; found %d matching volumes", volumeName,
		result.Payload.NumRecords)
}

// getVolumeSizeByNameAndStyle retrieves the size of the volume of the style and name specified
func (c RestClient) getVolumeSizeByNameAndStyle(ctx context.Context, volumeName, style string) (uint64, error) {
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style)
	if err != nil {
		return 0, err
	}
	if volume == nil {
		return 0, fmt.Errorf("could not find volume with name %v", volumeName)
	}

	return uint64(volume.Size), nil
}

// getVolumeUsedSizeByNameAndStyle retrieves the used bytes of the the volume of the style and name specified
func (c RestClient) getVolumeUsedSizeByNameAndStyle(ctx context.Context, volumeName, style string) (int, error) {
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style)
	if err != nil {
		return 0, err
	}
	if volume == nil {
		return 0, fmt.Errorf("could not find volume with name %v", volumeName)
	}

	if volume.Space == nil {
		return 0, fmt.Errorf("could not find space attributes for volume %v", volumeName)
	}

	if volume.Space.LogicalSpace == nil {
		return 0, fmt.Errorf("could not find logical space attributes for volume %v", volumeName)
	}

	return int(volume.Space.LogicalSpace.Used), nil
}

// setVolumeSizeByNameAndStyle sets the size of the specified volume of given style
func (c RestClient) setVolumeSizeByNameAndStyle(ctx context.Context, volumeName, newSize, style string) error {
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}

	uuid := volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = uuid

	sizeBytesStr, _ := utils.ConvertSizeToBytes(newSize)
	sizeBytes, _ := strconv.ParseUint(sizeBytesStr, 10, 64)

	volumeInfo := &models.Volume{
		Size: int64(sizeBytes),
	}

	params.SetInfo(volumeInfo)

	volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return c.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

// mountVolumeByNameAndStyle mounts a volume at the specified junction
func (c RestClient) mountVolumeByNameAndStyle(ctx context.Context, volumeName, junctionPath, style string) error {
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style)
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

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = uuid

	volumeInfo := &models.Volume{
		Nas: &models.VolumeNas{Path: ToStringPointer(junctionPath)},
	}
	params.SetInfo(volumeInfo)

	volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return c.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

// unmountVolumeByNameAndStyle umounts a volume
func (c RestClient) unmountVolumeByNameAndStyle(
	ctx context.Context,
	volumeName, style string,
) error {
	volume, err := c.getVolumeInAnyStateByNameAndStyle(ctx, volumeName, style)
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

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = uuid

	volumeInfo := &models.Volume{
		Nas: &models.VolumeNas{Path: ToStringPointer("")},
	}
	params.SetInfo(volumeInfo)

	volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return c.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

// RenameVolumeByNameAndStyle changes the name of a FlexVol (but not a FlexGroup!)
func (c RestClient) renameVolumeByNameAndStyle(ctx context.Context, volumeName, newVolumeName, style string) error {
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}

	uuid := volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = uuid

	volumeInfo := &models.Volume{
		Name: newVolumeName,
	}

	params.SetInfo(volumeInfo)

	volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return c.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

// destroyVolumeByNameAndStyle destroys a volume
func (c RestClient) destroyVolumeByNameAndStyle(ctx context.Context, name, style string) error {
	volume, err := c.getVolumeByNameAndStyle(ctx, name, style)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume: %v", name)
	}

	params := storage.NewVolumeDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = volume.UUID

	volumeDeleteAccepted, err := c.api.Storage.VolumeDelete(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeDeleteAccepted == nil {
		return fmt.Errorf("unexpected response from volume create")
	}

	return c.PollJobStatus(ctx, volumeDeleteAccepted.Payload)
}

func (c RestClient) modifyVolumeExportPolicyByNameAndStyle(
	ctx context.Context, volumeName, exportPolicyName, style string,
) error {
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}

	uuid := volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = uuid

	exportPolicy := &models.VolumeNasExportPolicy{Name: exportPolicyName}
	nasInfo := &models.VolumeNas{ExportPolicy: exportPolicy}
	volumeInfo := &models.Volume{Nas: nasInfo}
	params.SetInfo(volumeInfo)

	volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return c.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

func (c RestClient) modifyVolumeUnixPermissionsByNameAndStyle(
	ctx context.Context,
	volumeName, unixPermissions, style string,
) error {
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}

	uuid := volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = uuid

	// handle NAS options
	volumeNas := &models.VolumeNas{}
	volumeInfo := &models.Volume{}

	if unixPermissions != "" {
		unixPermissions = convertUnixPermissions(unixPermissions)
		volumePermissions, parseErr := strconv.ParseInt(unixPermissions, 10, 64)
		if parseErr != nil {
			return fmt.Errorf("cannot process unix permissions value %v", unixPermissions)
		}
		volumeNas.UnixPermissions = volumePermissions
	}

	volumeInfo.Nas = volumeNas
	params.SetInfo(volumeInfo)

	volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return c.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

// setVolumeCommentByNameAndStyle sets a volume's comment to the supplied value
// equivalent to filer::> volume modify -vserver iscsi_vs -volume v -comment newVolumeComment
func (c RestClient) setVolumeCommentByNameAndStyle(
	ctx context.Context,
	volumeName, newVolumeComment, style string,
) error {
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}

	uuid := volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = uuid

	volumeInfo := &models.Volume{
		Comment: ToStringPointer(newVolumeComment),
	}

	params.SetInfo(volumeInfo)

	volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return c.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

// convertUnixPermissions turns "rwx" into "7" and so on, if possible, otherwise returns the string
func convertUnixPermissions(s string) string {
	s = strings.TrimPrefix(s, "---")
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
func (c RestClient) setVolumeQosPolicyGroupNameByNameAndStyle(
	ctx context.Context, volumeName string, qosPolicyGroup QosPolicyGroup, style string,
) error {
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}

	uuid := volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
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

	volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return c.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

// startCloneSplitByNameAndStyle starts splitting the clone
func (c RestClient) startCloneSplitByNameAndStyle(ctx context.Context, volumeName, style string) error {
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}

	uuid := volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = uuid

	volumeInfo := &models.Volume{
		Clone: &models.VolumeClone{SplitInitiated: true},
	}
	// volumeInfo.Svm = &models.VolumeSvm{Name: d.config.SVM}

	params.SetInfo(volumeInfo)

	volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return c.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

// restoreSnapshotByNameAndStyle restores a volume to a snapshot as a non-blocking operation
func (c RestClient) restoreSnapshotByNameAndStyle(
	ctx context.Context,
	snapshotName, volumeName, style string,
) error {
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}

	// restore
	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = volume.UUID
	params.RestoreToSnapshotNameQueryParameter = &snapshotName

	volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return c.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

func (c RestClient) createCloneNAS(
	ctx context.Context,
	cloneName, sourceVolumeName, snapshotName string,
) (*storage.VolumeCreateAccepted, error) {
	params := storage.NewVolumeCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.ReturnRecordsQueryParameter = ToBoolPointer(true)

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
	volumeInfo.Svm = &models.VolumeSvm{Name: c.svmName}

	params.SetInfo(volumeInfo)

	return c.api.Storage.VolumeCreate(params, c.authInfo)
}

// listAllVolumeNamesBackedBySnapshot returns the names of all volumes backed by the specified snapshot
func (c RestClient) listAllVolumeNamesBackedBySnapshot(ctx context.Context, volumeName, snapshotName string) (
	[]string, error,
) {
	params := storage.NewVolumeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SVMNameQueryParameter = &c.svmName
	params.SetFieldsQueryParameter([]string{"name"})

	params.SetCloneParentVolumeNameQueryParameter(ToStringPointer(volumeName))
	params.SetCloneParentSnapshotNameQueryParameter(ToStringPointer(snapshotName))

	result, err := c.api.Storage.VolumeCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	result.Payload, err = c.getAllVolumePayloadRecords(result.Payload, params)
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
// equivalent to filer::> volume create -vserver iscsi_vs -volume v -aggregate aggr1 -size 1g -state online -type RW
// -policy default -unix-permissions ---rwxr-xr-x -space-guarantee none -snapshot-policy none -security-style unix
// -encrypt false
func (c RestClient) createVolumeByStyle(
	ctx context.Context,
	name string, sizeInBytes int64, aggrs []string, spaceReserve, snapshotPolicy, unixPermissions,
	exportPolicy, securityStyle, tieringPolicy, comment string, qosPolicyGroup QosPolicyGroup, encrypt *bool,
	snapshotReserve int, style string,
) error {
	params := storage.NewVolumeCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

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

	volumeInfo.Svm = &models.VolumeSvm{Name: c.svmName}

	// For encrypt == nil - we don't explicitely set the encrypt argument.
	// If destination aggregate is NAE enabled, new volume will be aggregate encrypted
	// else it will be volume encrypted as per Ontap's default behaviour.
	if encrypt != nil {
		volumeInfo.Encryption = &models.VolumeEncryption{Enabled: encrypt}
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

	volumeCreateAccepted, err := c.api.Storage.VolumeCreate(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeCreateAccepted == nil {
		return fmt.Errorf("unexpected response from volume create")
	}

	return c.PollJobStatus(ctx, volumeCreateAccepted.Payload)
}

// ////////////////////////////////////////////////////////////////////////////
// NAS VOLUME by style operations end
// ////////////////////////////////////////////////////////////////////////////

// //////////////////////////////////////////////////////////////////////////
// VOLUME operations
// ////////////////////////////////////////////////////////////////////////////

// VolumeList returns the names of all Flexvols whose names match the supplied pattern
func (c RestClient) VolumeList(ctx context.Context, pattern string) (*storage.VolumeCollectionGetOK, error) {
	return c.getAllVolumesByPatternStyleAndState(ctx, pattern, models.VolumeStyleFlexvol, models.VolumeStateOnline)
}

// VolumeListByAttrs is used to find bucket volumes for nas-eco and san-eco
func (c RestClient) VolumeListByAttrs(ctx context.Context, volumeAttrs *Volume) (Volumes, error) {
	params := storage.NewVolumeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	// params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SVMNameQueryParameter = &c.svmName

	style := models.VolumeStyleFlexvol // or models.VolumeStyleFlexgroup ??
	state := models.VolumeStateOnline

	wildcard := ToStringPointer("*")

	if volumeAttrs.Name != "" {
		params.SetNameQueryParameter(ToStringPointer(volumeAttrs.Name))
	} else {
		params.SetNameQueryParameter(wildcard)
	}

	if len(volumeAttrs.Aggregates) > 0 {
		aggrs := strings.Join(volumeAttrs.Aggregates, "|")
		params.SetAggregatesNameQueryParameter(ToStringPointer(aggrs))
	} else {
		params.SetAggregatesNameQueryParameter(wildcard)
	}

	if volumeAttrs.TieringPolicy != "" {
		params.SetTieringPolicyQueryParameter(ToStringPointer(volumeAttrs.TieringPolicy))
	} else {
		params.SetTieringPolicyQueryParameter(wildcard)
	}

	if volumeAttrs.SnapshotPolicy != "" {
		params.SetSnapshotPolicyNameQueryParameter(ToStringPointer(volumeAttrs.SnapshotPolicy))
	} else {
		params.SetSnapshotPolicyNameQueryParameter(wildcard)
	}

	if volumeAttrs.SpaceReserve != "" {
		params.SetGuaranteeTypeQueryParameter(ToStringPointer(volumeAttrs.SpaceReserve))
	} else {
		params.SetGuaranteeTypeQueryParameter(wildcard)
	}

	params.SetSpaceSnapshotReservePercentQueryParameter(ToInt64Pointer(volumeAttrs.SnapshotReserve))
	params.SetSnapshotDirectoryAccessEnabledQueryParameter(ToBoolPointer(volumeAttrs.SnapshotDir))
	params.SetEncryptionEnabledQueryParameter(volumeAttrs.Encrypt)

	if state != "" {
		params.SetStateQueryParameter(ToStringPointer(state))
	}
	params.SetStyleQueryParameter(ToStringPointer(style))
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := c.api.Storage.VolumeCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	result.Payload, err = c.getAllVolumePayloadRecords(result.Payload, params)
	if err != nil {
		return nil, err
	}

	volumes := Volumes{}
	for _, volume := range result.Payload.Records {

		aggregates := []string{}
		for _, aggr := range volume.Aggregates {
			aggregates = append(aggregates, aggr.Name)
		}

		snapshotDirAccessEnabled := false
		if volume.SnapshotDirectoryAccessEnabled != nil {
			snapshotDirAccessEnabled = *volume.SnapshotDirectoryAccessEnabled
		}

		tieringPolicy := models.VolumeTieringPolicyNone
		if volume.Movement != nil {
			tieringPolicy = volume.Movement.TieringPolicy
		}

		volumes = append(volumes, &Volume{
			Name:           volume.Name,
			Aggregates:     aggregates,
			Encrypt:        volume.Encryption.Enabled,
			TieringPolicy:  tieringPolicy,
			SnapshotDir:    snapshotDirAccessEnabled,
			SpaceReserve:   volume.Guarantee.Type,
			SnapshotPolicy: volume.SnapshotPolicy.Name,
		})
	}

	return volumes, nil
}

// VolumeCreate creates a volume with the specified options
// equivalent to filer::> volume create -vserver iscsi_vs -volume v -aggregate aggr1 -size 1g -state online -type RW
// -policy default -unix-permissions ---rwxr-xr-x -space-guarantee none -snapshot-policy none -security-style unix
// -encrypt false
func (c RestClient) VolumeCreate(
	ctx context.Context, name, aggregateName, size, spaceReserve, snapshotPolicy, unixPermissions,
	exportPolicy, securityStyle, tieringPolicy, comment string, qosPolicyGroup QosPolicyGroup, encrypt *bool,
	snapshotReserve int,
) error {
	sizeBytesStr, _ := utils.ConvertSizeToBytes(size)
	sizeInBytes, _ := strconv.ParseInt(sizeBytesStr, 10, 64)

	return c.createVolumeByStyle(ctx, name, sizeInBytes, []string{aggregateName}, spaceReserve, snapshotPolicy,
		unixPermissions, exportPolicy, securityStyle, tieringPolicy, comment, qosPolicyGroup, encrypt,
		snapshotReserve, models.VolumeStyleFlexvol)
}

// VolumeExists tests for the existence of a flexvol
func (c RestClient) VolumeExists(ctx context.Context, volumeName string) (bool, error) {
	return c.checkVolumeExistsByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexvol)
}

// VolumeGetByName gets the flexvol with the specified name
func (c RestClient) VolumeGetByName(ctx context.Context, volumeName string) (*models.Volume, error) {
	return c.getVolumeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexvol)
}

// VolumeMount mounts a flexvol at the specified junction
func (c RestClient) VolumeMount(
	ctx context.Context,
	volumeName, junctionPath string,
) error {
	return c.mountVolumeByNameAndStyle(ctx, volumeName, junctionPath, models.VolumeStyleFlexvol)
}

// VolumeRename changes the name of a flexvol
func (c RestClient) VolumeRename(
	ctx context.Context,
	volumeName, newVolumeName string,
) error {
	return c.renameVolumeByNameAndStyle(ctx, volumeName, newVolumeName, models.VolumeStyleFlexvol)
}

func (c RestClient) VolumeModifyExportPolicy(
	ctx context.Context,
	volumeName, exportPolicyName string,
) error {
	return c.modifyVolumeExportPolicyByNameAndStyle(ctx, volumeName, exportPolicyName, models.VolumeStyleFlexvol)
}

// VolumeSize retrieves the size of the specified flexvol
func (c RestClient) VolumeSize(
	ctx context.Context, volumeName string,
) (uint64, error) {
	return c.getVolumeSizeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexvol)
}

// VolumeUsedSize retrieves the used bytes of the specified volume
func (c RestClient) VolumeUsedSize(ctx context.Context, volumeName string) (int, error) {
	return c.getVolumeUsedSizeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexvol)
}

// VolumeSetSize sets the size of the specified flexvol
func (c RestClient) VolumeSetSize(ctx context.Context, volumeName, newSize string) error {
	return c.setVolumeSizeByNameAndStyle(ctx, volumeName, newSize, models.VolumeStyleFlexvol)
}

func (c RestClient) VolumeModifyUnixPermissions(ctx context.Context, volumeName, unixPermissions string) error {
	return c.modifyVolumeUnixPermissionsByNameAndStyle(ctx, volumeName, unixPermissions, models.VolumeStyleFlexvol)
}

// VolumeSetComment sets a flexvol's comment to the supplied value
// equivalent to filer::> volume modify -vserver iscsi_vs -volume v -comment newVolumeComment
func (c RestClient) VolumeSetComment(ctx context.Context, volumeName, newVolumeComment string) error {
	return c.setVolumeCommentByNameAndStyle(ctx, volumeName, newVolumeComment, models.VolumeStyleFlexvol)
}

// VolumeSetQosPolicyGroupName sets the QoS Policy Group for volume clones since
// we can't set adaptive policy groups directly during volume clone creation.
func (c RestClient) VolumeSetQosPolicyGroupName(
	ctx context.Context, volumeName string, qosPolicyGroup QosPolicyGroup,
) error {
	return c.setVolumeQosPolicyGroupNameByNameAndStyle(ctx, volumeName, qosPolicyGroup, models.VolumeStyleFlexvol)
}

// VolumeCloneSplitStart starts splitting theflexvol clone
func (c RestClient) VolumeCloneSplitStart(ctx context.Context, volumeName string) error {
	return c.startCloneSplitByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexvol)
}

// VolumeDestroy destroys a flexvol
func (c RestClient) VolumeDestroy(ctx context.Context, name string) error {
	return c.destroyVolumeByNameAndStyle(ctx, name, models.VolumeStyleFlexvol)
}

// ////////////////////////////////////////////////////////////////////////////
// SNAPSHOT operations
// ////////////////////////////////////////////////////////////////////////////

// SnapshotCreate creates a snapshot
func (c RestClient) SnapshotCreate(
	ctx context.Context, volumeUUID, snapshotName string,
) (*storage.SnapshotCreateAccepted, error) {
	params := storage.NewSnapshotCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.VolumeUUIDPathParameter = volumeUUID

	snapshotInfo := &models.Snapshot{
		Name: snapshotName,
	}

	snapshotInfo.Svm = &models.SnapshotSvm{Name: c.svmName}

	params.SetInfo(snapshotInfo)

	return c.api.Storage.SnapshotCreate(params, c.authInfo)
}

// SnapshotCreateAndWait creates a snapshot and waits on the job to complete
func (c RestClient) SnapshotCreateAndWait(ctx context.Context, volumeUUID, snapshotName string) error {
	snapshotCreateResult, err := c.SnapshotCreate(ctx, volumeUUID, snapshotName)
	if err != nil {
		return fmt.Errorf("could not create snapshot: %v", err)
	}
	if snapshotCreateResult == nil {
		return fmt.Errorf("could not create snapshot: %v", "unexpected result")
	}

	return c.PollJobStatus(ctx, snapshotCreateResult.Payload)
}

// SnapshotList lists snapshots
func (c RestClient) SnapshotList(ctx context.Context, volumeUUID string) (*storage.SnapshotCollectionGetOK, error) {
	params := storage.NewSnapshotCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.VolumeUUIDPathParameter = volumeUUID

	params.SVMNameQueryParameter = ToStringPointer(c.svmName)
	params.SetFieldsQueryParameter([]string{"name", "create_time"})

	result, err := c.api.Storage.SnapshotCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NextLoop:
		for !done {
			resultNext, errNext := c.api.Storage.SnapshotCollectionGet(params, c.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NextLoop
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NextLoop
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

// SnapshotListByName lists snapshots by name
func (c RestClient) SnapshotListByName(ctx context.Context, volumeUUID, snapshotName string) (
	*storage.SnapshotCollectionGetOK, error,
) {
	params := storage.NewSnapshotCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.VolumeUUIDPathParameter = volumeUUID
	params.NameQueryParameter = ToStringPointer(snapshotName)

	params.SVMNameQueryParameter = ToStringPointer(c.svmName)
	params.SetFieldsQueryParameter([]string{"name", "create_time"})

	return c.api.Storage.SnapshotCollectionGet(params, c.authInfo)
}

// SnapshotGet returns info on the snapshot
func (c RestClient) SnapshotGet(ctx context.Context, volumeUUID, snapshotUUID string) (*storage.SnapshotGetOK, error) {
	params := storage.NewSnapshotGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.VolumeUUIDPathParameter = volumeUUID
	params.UUIDPathParameter = snapshotUUID

	return c.api.Storage.SnapshotGet(params, c.authInfo)
}

// SnapshotGetByName finds the snapshot by name
func (c RestClient) SnapshotGetByName(ctx context.Context, volumeUUID, snapshotName string) (*models.Snapshot, error) {
	result, err := c.SnapshotListByName(ctx, volumeUUID, snapshotName)
	if result.Payload.NumRecords == 1 && result.Payload.Records != nil {
		return result.Payload.Records[0], nil
	}
	return nil, err
}

// SnapshotDelete deletes a snapshot
func (c RestClient) SnapshotDelete(
	ctx context.Context,
	volumeUUID, snapshotUUID string,
) (*storage.SnapshotDeleteAccepted, error) {
	params := storage.NewSnapshotDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.VolumeUUIDPathParameter = volumeUUID
	params.UUIDPathParameter = snapshotUUID

	return c.api.Storage.SnapshotDelete(params, c.authInfo)
}

// SnapshotRestoreVolume restores a volume to a snapshot as a non-blocking operation
func (c RestClient) SnapshotRestoreVolume(ctx context.Context, snapshotName, volumeName string) error {
	return c.restoreSnapshotByNameAndStyle(ctx, snapshotName, volumeName, models.VolumeStyleFlexvol)
}

// SnapshotRestoreFlexgroup restores a volume to a snapshot as a non-blocking operation
func (c RestClient) SnapshotRestoreFlexgroup(ctx context.Context, snapshotName, volumeName string) error {
	return c.restoreSnapshotByNameAndStyle(ctx, snapshotName, volumeName, models.VolumeStyleFlexgroup)
}

// VolumeDisableSnapshotDirectoryAccess disables access to the ".snapshot" directory
// Disable '.snapshot' to allow official mysql container's chmod-in-init to work
func (c RestClient) VolumeDisableSnapshotDirectoryAccess(ctx context.Context, volumeName string) error {
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexvol)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}

	uuid := volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = uuid

	volumeInfo := &models.Volume{}
	volumeInfo.SnapshotDirectoryAccessEnabled = ToBoolPointer(false)
	params.SetInfo(volumeInfo)

	volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return c.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

// VolumeListAllBackedBySnapshot returns the names of all FlexVols backed by the specified snapshot
func (c RestClient) VolumeListAllBackedBySnapshot(ctx context.Context, volumeName, snapshotName string) ([]string,
	error,
) {
	return c.listAllVolumeNamesBackedBySnapshot(ctx, volumeName, snapshotName)
}

// ////////////////////////////////////////////////////////////////////////////
// CLONE operations
// ////////////////////////////////////////////////////////////////////////////

// VolumeCloneCreate creates a clone
// see also: https://library.netapp.com/ecmdocs/ECMLP2858435/html/resources/volume.html#creating-a-flexclone-and-specifying-its-properties-using-post
func (c RestClient) VolumeCloneCreate(ctx context.Context, cloneName, sourceVolumeName, snapshotName string) (
	*storage.VolumeCreateAccepted, error,
) {
	return c.createCloneNAS(ctx, cloneName, sourceVolumeName, snapshotName)
}

// VolumeCloneCreateAsync clones a volume from a snapshot
func (c RestClient) VolumeCloneCreateAsync(ctx context.Context, cloneName, sourceVolumeName, snapshot string) error {
	cloneCreateResult, err := c.createCloneNAS(ctx, cloneName, sourceVolumeName, snapshot)
	if err != nil {
		return fmt.Errorf("could not create clone: %v", err)
	}
	if cloneCreateResult == nil {
		return fmt.Errorf("could not create clone: %v", "unexpected result")
	}

	return c.PollJobStatus(ctx, cloneCreateResult.Payload)
}

// ///////////////////////////////////////////////////////////////////////////
// iSCSI initiator operations
// ///////////////////////////////////////////////////////////////////////////

// IscsiInitiatorGetDefaultAuth returns the authorization details for the default initiator
// equivalent to filer::> vserver iscsi security show -vserver SVM -initiator-name default
func (c RestClient) IscsiInitiatorGetDefaultAuth(ctx context.Context) (*san.IscsiCredentialsCollectionGetOK, error) {
	params := san.NewIscsiCredentialsCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.ReturnRecordsQueryParameter = ToBoolPointer(true)

	// params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SVMNameQueryParameter = ToStringPointer(c.svmName)
	params.InitiatorQueryParameter = ToStringPointer("default")

	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := c.api.San.IscsiCredentialsCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result.Payload == nil {
		return nil, nil
	}

	return result, nil
}

// IscsiInterfaceGet returns information about the vserver's  iSCSI interfaces
func (c RestClient) IscsiInterfaceGet(ctx context.Context, svm string) (*san.IscsiServiceCollectionGetOK,
	error,
) {
	params := san.NewIscsiServiceCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.ReturnRecordsQueryParameter = ToBoolPointer(true)
	params.SVMNameQueryParameter = &svm

	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := c.api.San.IscsiServiceCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	return result, nil
}

// IscsiInitiatorSetDefaultAuth sets the authorization details for the default initiator
//
//	equivalent to filer::> vserver iscsi security modify -vserver SVM -initiator-name default \
//	                          -auth-type CHAP -user-name outboundUserName -outbound-user-name outboundPassphrase
func (c RestClient) IscsiInitiatorSetDefaultAuth(
	ctx context.Context, authType, userName, passphrase,
	outbountUserName, outboundPassphrase string,
) error {
	getDefaultAuthResponse, err := c.IscsiInitiatorGetDefaultAuth(ctx)
	if err != nil {
		return err
	}
	if getDefaultAuthResponse == nil {
		return fmt.Errorf("could not get the default iscsi initiator")
	}
	if getDefaultAuthResponse.Payload.NumRecords != 1 {
		return fmt.Errorf("should only be one default iscsi initiator")
	}

	params := san.NewIscsiCredentialsModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

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

	_, err = c.api.San.IscsiCredentialsModify(params, c.authInfo)

	return err
}

// IscsiNodeGetName returns information about the vserver's iSCSI node name
func (c RestClient) IscsiNodeGetName(ctx context.Context) (*san.IscsiServiceGetOK,
	error,
) {
	svm, err := c.SvmGetByName(ctx, c.svmName)
	if err != nil {
		return nil, err
	}
	if svm == nil {
		return nil, fmt.Errorf("could not find SVM %s", c.svmName)
	}

	params := san.NewIscsiServiceGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SVMUUIDPathParameter = svm.UUID

	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := c.api.San.IscsiServiceGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	return result, nil
}

// ///////////////////////////////////////////////////////////////////////////
// IGROUP operations
// ///////////////////////////////////////////////////////////////////////////

// IgroupCreate creates the specified initiator group
// equivalent to filer::> igroup create docker -vserver iscsi_vs -protocol iscsi -ostype linux
func (c RestClient) IgroupCreate(ctx context.Context, initiatorGroupName, initiatorGroupType, osType string) error {
	params := san.NewIgroupCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.ReturnRecordsQueryParameter = ToBoolPointer(true)

	igroupInfo := &models.Igroup{
		Name:     initiatorGroupName,
		Protocol: ToStringPointer(initiatorGroupType),
		OsType:   osType,
	}

	igroupInfo.Svm = &models.IgroupSvm{Name: c.svmName}

	params.SetInfo(igroupInfo)

	igroupCreateAccepted, err := c.api.San.IgroupCreate(params, c.authInfo)
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
func (c RestClient) IgroupAdd(ctx context.Context, initiatorGroupName, initiator string) error {
	igroup, err := c.IgroupGetByName(ctx, initiatorGroupName)
	if err != nil {
		return err
	}
	if igroup == nil {
		return fmt.Errorf("unexpected response from igroup lookup, igroup was nil")
	}
	igroupUUID := igroup.UUID

	params := san.NewIgroupInitiatorCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.IgroupUUIDPathParameter = igroupUUID

	igroupInitiator := &models.IgroupInitiator{
		Name: initiator,
	}

	params.SetInfo(igroupInitiator)

	_, err = c.api.San.IgroupInitiatorCreate(params, c.authInfo)
	if err != nil {
		return err
	}

	return nil
}

// IgroupRemove removes an initiator from an initiator group
func (c RestClient) IgroupRemove(ctx context.Context, initiatorGroupName, initiator string) error {
	igroup, err := c.IgroupGetByName(ctx, initiatorGroupName)
	if err != nil {
		return err
	}
	if igroup == nil {
		return fmt.Errorf("unexpected response from igroup lookup, igroup was nil")
	}
	igroupUUID := igroup.UUID

	params := san.NewIgroupInitiatorDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.IgroupUUIDPathParameter = igroupUUID
	params.NamePathParameter = initiator

	deleteAccepted, err := c.api.San.IgroupInitiatorDelete(params, c.authInfo)
	if err != nil {
		return err
	}
	if deleteAccepted == nil {
		return fmt.Errorf("unexpected response from igroup remove")
	}

	return nil
}

// IgroupDestroy destroys an initiator group
func (c RestClient) IgroupDestroy(ctx context.Context, initiatorGroupName string) error {
	igroup, err := c.IgroupGetByName(ctx, initiatorGroupName)
	if err != nil {
		return err
	}
	if igroup == nil {
		// Initiator group not found. Log a message and return nil.
		Logc(ctx).WithField("igroup", initiatorGroupName).Debug("No such initiator group (igroup).")
		return nil
	}
	igroupUUID := igroup.UUID

	params := san.NewIgroupDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = igroupUUID

	lunDeleteResult, err := c.api.San.IgroupDelete(params, c.authInfo)
	if err != nil {
		return fmt.Errorf("could not delete igroup: %v", err)
	}
	if lunDeleteResult == nil {
		return fmt.Errorf("could not delete igroup: %v", "unexpected result")
	}

	return nil
}

// IgroupList lists initiator groups
func (c RestClient) IgroupList(ctx context.Context, pattern string) (*san.IgroupCollectionGetOK, error) {
	if pattern == "" {
		pattern = "*"
	}

	params := san.NewIgroupCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SVMNameQueryParameter = &c.svmName
	params.SetNameQueryParameter(ToStringPointer(pattern))
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := c.api.San.IgroupCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NextLoop:
		for !done {
			resultNext, errNext := c.api.San.IgroupCollectionGet(params, c.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NextLoop
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NextLoop
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

// IgroupGet gets the igroup with the specified uuid
func (c RestClient) IgroupGet(ctx context.Context, uuid string) (*san.IgroupGetOK, error) {
	params := san.NewIgroupGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = uuid

	return c.api.San.IgroupGet(params, c.authInfo)
}

// IgroupGetByName gets the igroup with the specified name
func (c RestClient) IgroupGetByName(ctx context.Context, initiatorGroupName string) (*models.Igroup, error) {
	result, err := c.IgroupList(ctx, initiatorGroupName)
	if err != nil {
		return nil, err
	}
	if result.Payload.NumRecords == 1 && result.Payload.Records != nil {
		return result.Payload.Records[0], nil
	}
	return nil, err
}

//

// //////////////////////////////////////////////////////////////////////////
// LUN operations
// ////////////////////////////////////////////////////////////////////////////

type LunOptionsResult struct {
	RecordSchema struct {
		Space struct {
			Size struct {
				OpenAPIType string `json:"open_api_type"`
				Range       struct {
					Min int   `json:"min"`
					Max int64 `json:"max"`
				} `json:"range"`
			} `json:"size,omitempty"`
		} `json:"space,omitempty"`
	} `json:"record_schema"`
}

// LunOptions gets the LUN options
func (d RestClient) LunOptions(
	ctx context.Context,
) (*LunOptionsResult, error) {
	url := fmt.Sprintf(
		`https://%v/api/v1/storage/luns?return_schema=POST&fields=space.size`,
		d.config.ManagementLIF,
	)

	Logc(ctx).WithFields(LogFields{
		"url": url,
	}).Debug("LunOptions request")

	req, _ := http.NewRequestWithContext(ctx, "OPTIONS", url, nil)
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

	Logc(ctx).WithFields(LogFields{
		"body": string(body),
	}).Debug("LunOptions")

	result := &LunOptionsResult{}
	unmarshalErr := json.Unmarshal(body, result)
	if unmarshalErr != nil {
		Log().WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		return nil, unmarshalErr
	}

	return result, nil
}

// pollLunCreate polls for the created LUN to appear, with backoff retry logic
func (c RestClient) pollLunCreate(ctx context.Context, lunPath string) error {
	checkCreateStatus := func() error {
		lun, err := c.LunGetByName(ctx, lunPath)
		if err != nil {
			return err
		}
		if lun == nil {
			return fmt.Errorf("could not find LUN with name %v", lunPath)
		}
		return nil
	}
	createStatusNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("LUN creation not finished, waiting.")
	}
	createStatusBackoff := backoff.NewExponentialBackOff()
	createStatusBackoff.InitialInterval = 1 * time.Second
	createStatusBackoff.Multiplier = 2
	createStatusBackoff.RandomizationFactor = 0.1
	createStatusBackoff.MaxElapsedTime = 2 * time.Minute

	// Run the creation check using an exponential backoff
	if err := backoff.RetryNotify(checkCreateStatus, createStatusBackoff, createStatusNotify); err != nil {
		Logc(ctx).WithField("LUN", lunPath).Warnf("LUN not found after %3.2f seconds.",
			createStatusBackoff.MaxElapsedTime.Seconds())
		return err
	}

	return nil
}

// LunCloneCreate creates a LUN clone
func (c RestClient) LunCloneCreate(
	ctx context.Context, lunPath, sourcePath string, sizeInBytes int64, osType string, qosPolicyGroup QosPolicyGroup,
) error {
	fields := LogFields{
		"Method":         "LunCloneCreate",
		"Type":           "ontap_rest",
		"lunPath":        lunPath,
		"sourcePath":     sourcePath,
		"sizeInBytes":    sizeInBytes,
		"osType":         osType,
		"qosPolicyGroup": qosPolicyGroup,
	}
	Logd(ctx, c.driverName, c.config.DebugTraceFlags["method"]).WithFields(fields).
		Trace(">>>> LunCloneCreate")
	defer Logd(ctx, c.driverName, c.config.DebugTraceFlags["method"]).WithFields(fields).
		Trace("<<<< LunCloneCreate")

	if strings.Contains(lunPath, failureLUNCreate) {
		return errors.New("injected error")
	}

	params := san.NewLunCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.ReturnRecordsQueryParameter = ToBoolPointer(true)

	lunInfo := &models.Lun{
		Clone: &models.LunClone{
			Source: &models.LunCloneSource{
				Name: sourcePath,
			},
		},
		Name:   lunPath, // example:  /vol/myVolume/myLun1
		OsType: osType,
		Space: &models.LunSpace{
			Size: sizeInBytes,
		},
		QosPolicy: &models.LunQosPolicy{
			Name: qosPolicyGroup.Name,
		},
	}
	lunInfo.Svm = &models.LunSvm{Name: c.SVMName()}

	params.SetInfo(lunInfo)

	lunCreateAccepted, err := c.api.San.LunCreate(params, c.authInfo)
	if err != nil {
		return err
	}
	if lunCreateAccepted == nil {
		return fmt.Errorf("unexpected response from lun create")
	}

	// verify the created LUN can be found
	return c.pollLunCreate(ctx, lunPath)
}

// LunCreate creates a LUN
func (c RestClient) LunCreate(
	ctx context.Context, lunPath string, sizeInBytes int64, osType string, qosPolicyGroup QosPolicyGroup,
	spaceReserved, spaceAllocated bool,
) error {
	if strings.Contains(lunPath, failureLUNCreate) {
		return errors.New("injected error")
	}

	params := san.NewLunCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.ReturnRecordsQueryParameter = ToBoolPointer(true)

	lunInfo := &models.Lun{
		Name:   lunPath, // example:  /vol/myVolume/myLun1
		OsType: osType,
		Space: &models.LunSpace{
			Size: sizeInBytes,
			Guarantee: &models.LunSpaceGuarantee{
				Requested: ToBoolPointer(spaceReserved),
			},
			ScsiThinProvisioningSupportEnabled: ToBoolPointer(spaceAllocated),
		},
		QosPolicy: &models.LunQosPolicy{
			Name: qosPolicyGroup.Name,
		},
	}
	lunInfo.Svm = &models.LunSvm{Name: c.svmName}

	params.SetInfo(lunInfo)

	lunCreateAccepted, err := c.api.San.LunCreate(params, c.authInfo)
	if err != nil {
		return err
	}
	if lunCreateAccepted == nil {
		return fmt.Errorf("unexpected response from lun create")
	}

	// verify the created LUN can be found
	return c.pollLunCreate(ctx, lunPath)
}

// LunGet gets the LUN with the specified uuid
func (c RestClient) LunGet(ctx context.Context, uuid string) (*san.LunGetOK, error) {
	params := san.NewLunGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = uuid

	return c.api.San.LunGet(params, c.authInfo)
}

// LunGetByName gets the LUN with the specified name
func (c RestClient) LunGetByName(ctx context.Context, name string) (*models.Lun, error) {
	result, err := c.LunList(ctx, name)
	if err != nil || result.Payload == nil {
		return nil, err
	}
	if result.Payload.NumRecords == 1 && result.Payload.Records != nil {
		return result.Payload.Records[0], nil
	}
	return nil, err
}

// LunList finds LUNs with the specified pattern
func (c RestClient) LunList(ctx context.Context, pattern string) (*san.LunCollectionGetOK, error) {
	params := san.NewLunCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SVMNameQueryParameter = &c.svmName
	params.SetNameQueryParameter(ToStringPointer(pattern))
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := c.api.San.LunCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NextLoop:
		for !done {
			resultNext, errNext := c.api.San.LunCollectionGet(params, c.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NextLoop
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NextLoop
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

// LunDelete deletes a LUN
func (c RestClient) LunDelete(
	ctx context.Context,
	lunUUID string,
) error {
	params := san.NewLunDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = lunUUID

	lunDeleteResult, err := c.api.San.LunDelete(params, c.authInfo)
	if err != nil {
		return fmt.Errorf("could not delete lun: %v", err)
	}
	if lunDeleteResult == nil {
		return fmt.Errorf("could not delete lun: %v", "unexpected result")
	}

	return nil
}

// LunGetComment gets the comment for a given LUN.
func (c RestClient) LunGetComment(
	ctx context.Context,
	lunPath string,
) (string, error) {
	lun, err := c.LunGetByName(ctx, lunPath)
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
func (c RestClient) LunSetComment(
	ctx context.Context,
	lunPath, comment string,
) error {
	lun, err := c.LunGetByName(ctx, lunPath)
	if err != nil {
		return err
	}
	if lun == nil {
		return fmt.Errorf("could not find lun with name %v", lunPath)
	}

	uuid := lun.UUID

	params := san.NewLunModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = uuid

	lunInfo := &models.Lun{
		Comment: &comment,
	}

	params.SetInfo(lunInfo)

	lunModifyOK, err := c.api.San.LunModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if lunModifyOK == nil {
		return fmt.Errorf("unexpected response from LUN modify")
	}

	return nil
}

// LunGetAttribute gets an attribute by name for a given LUN.
func (c RestClient) LunGetAttribute(
	ctx context.Context,
	lunPath, attributeName string,
) (string, error) {
	lun, err := c.LunGetByName(ctx, lunPath)
	if err != nil {
		return "", err
	}
	if lun == nil {
		return "", fmt.Errorf("could not find LUN with name %v", lunPath)
	}
	if lun.Attributes == nil {
		return "", fmt.Errorf("LUN did not have any attributes")
	}
	for _, attr := range lun.Attributes {
		if attributeName == attr.Name {
			return attr.Value, nil
		}
	}

	// LUN has no value for the specified attribute
	return "", nil
}

// LunSetAttribute sets the attribute to the provided value for a given LUN.
func (c RestClient) LunSetAttribute(
	ctx context.Context,
	lunPath, attributeName, attributeValue string,
) error {
	lun, err := c.LunGetByName(ctx, lunPath)
	if err != nil {
		return err
	}
	if lun == nil {
		return fmt.Errorf("could not find LUN with name %v", lunPath)
	}

	uuid := lun.UUID

	attributeExists := false
	for _, attrs := range lun.Attributes {
		if attributeName == attrs.Name {
			attributeExists = true
		}
	}

	if !attributeExists {

		params := san.NewLunAttributeCreateParamsWithTimeout(c.httpClient.Timeout)
		params.Context = ctx
		params.HTTPClient = c.httpClient
		params.LunUUIDPathParameter = uuid

		attrInfo := &models.LunAttribute{
			// in a create, the attribute name is specified here
			Name:  attributeName,
			Value: attributeValue,
		}
		params.Info = attrInfo

		lunAttrCreateOK, err := c.api.San.LunAttributeCreate(params, c.authInfo)
		if err != nil {
			return err
		}
		if lunAttrCreateOK == nil {
			return fmt.Errorf("unexpected response from LUN attribute create")
		}
		return nil

	} else {

		params := san.NewLunAttributeModifyParamsWithTimeout(c.httpClient.Timeout)
		params.Context = ctx
		params.HTTPClient = c.httpClient
		params.LunUUIDPathParameter = uuid
		params.NamePathParameter = attributeName

		attrInfo := &models.LunAttribute{
			// in a modify, the attribute name is specified in the path as params.NamePathParameter
			Value: attributeValue,
		}
		params.Info = attrInfo

		lunAttrModifyOK, err := c.api.San.LunAttributeModify(params, c.authInfo)
		if err != nil {
			return err
		}
		if lunAttrModifyOK == nil {
			return fmt.Errorf("unexpected response from LUN attribute modify")
		}
		return nil
	}
}

// LunSetComment sets the comment for a given LUN.
func (c RestClient) LunSetQosPolicyGroup(
	ctx context.Context,
	lunPath, qosPolicyGroup string,
) error {
	lun, err := c.LunGetByName(ctx, lunPath)
	if err != nil {
		return err
	}
	if lun == nil {
		return fmt.Errorf("could not find lun with name %v", lunPath)
	}

	uuid := lun.UUID

	params := san.NewLunModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = uuid

	qosPolicy := &models.LunQosPolicy{
		Name: qosPolicyGroup,
	}
	lunInfo := &models.Lun{
		QosPolicy: qosPolicy,
	}

	params.SetInfo(lunInfo)

	lunModifyOK, err := c.api.San.LunModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if lunModifyOK == nil {
		return fmt.Errorf("unexpected response from LUN modify")
	}

	return nil
}

// LunRename changes the name of a LUN
func (c RestClient) LunRename(
	ctx context.Context,
	lunPath, newLunPath string,
) error {
	lun, err := c.LunGetByName(ctx, lunPath)
	if err != nil {
		return err
	}
	if lun == nil {
		return fmt.Errorf("could not find lun with name %v", lunPath)
	}

	uuid := lun.UUID

	params := san.NewLunModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = uuid

	lunInfo := &models.Lun{
		Name: newLunPath,
	}

	params.SetInfo(lunInfo)

	lunModifyOK, err := c.api.San.LunModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if lunModifyOK == nil {
		return fmt.Errorf("unexpected response from LUN modify")
	}

	return nil
}

// LunMapInfo gets the LUN maping information for the specified LUN
func (c RestClient) LunMapInfo(
	ctx context.Context,
	initiatorGroupName, lunPath string,
) (*san.LunMapCollectionGetOK, error) {
	params := san.NewLunMapCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.LunNameQueryParameter = &lunPath
	if initiatorGroupName != "" {
		params.IgroupNameQueryParameter = &initiatorGroupName
	}
	params.FieldsQueryParameter = []string{"svm", "lun", "igroup", "logical-unit-number"}

	return c.api.San.LunMapCollectionGet(params, c.authInfo)
}

// LunUnmap deletes the lun mapping for the given LUN path and igroup
// equivalent to filer::> lun mapping delete -vserver iscsi_vs -path /vol/v/lun0 -igroup group
func (c RestClient) LunUnmap(
	ctx context.Context,
	initiatorGroupName, lunPath string,
) error {
	lunMapResponse, err := c.LunMapInfo(ctx, initiatorGroupName, lunPath)
	if err != nil {
		return fmt.Errorf("problem reading maps for LUN %s: %v", lunPath, err)
	} else if lunMapResponse.Payload == nil {
		return fmt.Errorf("problem reading maps for LUN %s", lunPath)
	}
	igroupUUID := lunMapResponse.Payload.Records[0].Igroup.UUID

	lun, err := c.LunGetByName(ctx, lunPath)
	if err != nil {
		return err
	}
	if lun == nil {
		return fmt.Errorf("could not find lun with name %v", lunPath)
	}
	lunUUID := lun.UUID

	params := san.NewLunMapDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.IgroupUUIDPathParameter = igroupUUID
	params.LunUUIDPathParameter = lunUUID

	_, err = c.api.San.LunMapDelete(params, c.authInfo)
	if err != nil {
		return err
	}
	return nil
}

// LunMap maps a LUN to an id in an initiator group
// equivalent to filer::> lun map -vserver iscsi_vs -path /vol/v/lun1 -igroup docker -lun-id 0
func (c RestClient) LunMap(
	ctx context.Context,
	initiatorGroupName, lunPath string,
	lunID int,
) (*san.LunMapCreateCreated, error) {
	lun, err := c.LunGetByName(ctx, lunPath)
	if err != nil {
		return nil, err
	}
	if lun == nil {
		return nil, fmt.Errorf("could not find lun with name %v", lunPath)
	}
	uuid := lun.UUID

	params := san.NewLunMapCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.ReturnRecordsQueryParameter = ToBoolPointer(true)

	igroupInfo := &models.LunMapIgroup{
		Name: initiatorGroupName,
	}
	lunInfo := &models.LunMapLun{
		Name: lunPath,
		UUID: uuid,
	}
	lunSVM := &models.LunMapSvm{
		Name: c.svmName,
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

	result, err := c.api.San.LunMapCreate(params, c.authInfo)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// LunMapList equivalent to the following
// filer::> lun mapping show -vserver iscsi_vs -path /vol/v/lun0 -igroup trident
// filer::> lun mapping show -vserver iscsi_vs -path /vol/v/lun0 -igroup *
// filer::> lun mapping show -vserver iscsi_vs -path *           -igroup trident
func (c RestClient) LunMapList(
	ctx context.Context,
	initiatorGroupName, lunPath string,
) (*san.LunMapCollectionGetOK, error) {
	params := san.NewLunMapCollectionGetParams()
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	params.SetIgroupNameQueryParameter(ToStringPointer(initiatorGroupName))
	params.SetLunNameQueryParameter(ToStringPointer(lunPath))

	return c.api.San.LunMapCollectionGet(params, c.authInfo)
}

// LunMapGetReportingNodes
// equivalent to filer::> lun mapping show -vserver iscsi_vs -path /vol/v/lun0 -igroup trident
func (c RestClient) LunMapGetReportingNodes(
	ctx context.Context,
	initiatorGroupName, lunPath string,
) ([]string, error) {
	lun, lunGetErr := c.LunGetByName(ctx, lunPath)
	if lunGetErr != nil {
		return nil, lunGetErr
	}
	if lun == nil {
		return nil, fmt.Errorf("could not find lun with name %v", lunPath)
	}
	lunUUID := lun.UUID

	igroup, igroupGetErr := c.IgroupGetByName(ctx, initiatorGroupName)
	if igroupGetErr != nil {
		return nil, igroupGetErr
	}
	if igroup == nil {
		return nil, fmt.Errorf("could not find igroup with name %v", initiatorGroupName)
	}
	igroupUUID := igroup.UUID

	params := san.NewLunMapReportingNodeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SetLunUUIDPathParameter(lunUUID)
	params.SetIgroupUUIDPathParameter(igroupUUID)

	result, err := c.api.San.LunMapReportingNodeCollectionGet(params, c.authInfo)
	if err != nil {
		// use reflection to access any underlying REST error response and check the code
		errorResponse, extractErr := ExtractErrorResponse(ctx, err)
		if extractErr != nil {
			return nil, err
		}
		if errorResponse != nil && errorResponse.Error != nil {
			errorCode := errorResponse.Error.Code
			if errorCode == "5374922" {
				// the specified LUN map does not exist
				return []string{}, nil
			}
		}
		return nil, err
	}

	names := []string{}
	for _, records := range result.Payload.Records {
		names = append(names, records.Name)
	}
	return names, nil
}

// LunSize gets the size for a given LUN.
func (c RestClient) LunSize(
	ctx context.Context,
	lunPath string,
) (int, error) {
	lun, err := c.LunGetByName(ctx, lunPath)
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
func (c RestClient) LunSetSize(
	ctx context.Context,
	lunPath, newSize string,
) (uint64, error) {
	lun, err := c.LunGetByName(ctx, lunPath)
	if err != nil {
		return 0, err
	}
	if lun == nil {
		return 0, fmt.Errorf("could not find lun with name %v", lunPath)
	}

	uuid := lun.UUID

	params := san.NewLunModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = uuid

	sizeBytesStr, _ := utils.ConvertSizeToBytes(newSize)
	sizeBytes, _ := strconv.ParseUint(sizeBytesStr, 10, 64)
	spaceInfo := &models.LunSpace{
		Size: int64(sizeBytes),
	}
	lunInfo := &models.Lun{
		Space: spaceInfo,
	}

	params.SetInfo(lunInfo)

	lunModifyOK, err := c.api.San.LunModify(params, c.authInfo)
	if err != nil {
		return 0, err
	}
	if lunModifyOK == nil {
		return 0, fmt.Errorf("unexpected response from LUN modify")
	}

	return sizeBytes, nil
}

// ////////////////////////////////////////////////////////////////////////////
// NETWORK operations
// ////////////////////////////////////////////////////////////////////////////

// NetworkIPInterfacesList lists all IP interfaces
func (c RestClient) NetworkIPInterfacesList(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
	params := networking.NewNetworkIPInterfacesGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SVMNameQueryParameter = &c.svmName
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := c.api.Networking.NetworkIPInterfacesGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NextLoop:
		for !done {
			resultNext, errNext := c.api.Networking.NetworkIPInterfacesGet(params, c.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NextLoop
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NextLoop
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

func (c RestClient) NetInterfaceGetDataLIFs(ctx context.Context, protocol string) ([]string, error) {
	if protocol == "" {
		return nil, fmt.Errorf("missing protocol specification")
	}

	params := networking.NewNetworkIPInterfacesGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.ServicesQueryParameter = ToStringPointer(fmt.Sprintf("data_%v", protocol))

	params.SVMNameQueryParameter = &c.svmName
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	lifResponse, err := c.api.Networking.NetworkIPInterfacesGet(params, c.authInfo)
	if err != nil {
		return nil, fmt.Errorf("error checking network interfaces: %v", err)
	}
	if lifResponse == nil {
		return nil, fmt.Errorf("unexpected error checking network interfaces")
	}

	dataLIFs := make([]string, 0)
	for _, record := range lifResponse.Payload.Records {
		if record.IP != nil && record.State == models.IPInterfaceStateUp {
			dataLIFs = append(dataLIFs, string(record.IP.Address))
		}
	}

	Logc(ctx).WithField("dataLIFs", dataLIFs).Debug("Data LIFs")
	return dataLIFs, nil
}

// ////////////////////////////////////////////////////////////////////////////
// JOB operations
// ////////////////////////////////////////////////////////////////////////////

// JobGet returns the job by ID
func (c RestClient) JobGet(ctx context.Context, jobUUID string) (*cluster.JobGetOK, error) {
	params := cluster.NewJobGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = jobUUID

	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need, this forces ALL fields

	return c.api.Cluster.JobGet(params, c.authInfo)
}

// IsJobFinished lookus up the supplied JobLinkResponse's UUID to see if it's reached a terminal state
func (c RestClient) IsJobFinished(ctx context.Context, payload *models.JobLinkResponse) (bool, error) {
	if payload == nil {
		return false, fmt.Errorf("payload is nil")
	}

	if payload.Job == nil {
		return false, fmt.Errorf("payload's Job is nil")
	}

	job := payload.Job
	jobUUID := job.UUID

	Logc(ctx).WithFields(LogFields{
		"payload": payload,
		"job":     payload.Job,
		"jobUUID": jobUUID,
	}).Debug("IsJobFinished")

	jobResult, err := c.JobGet(ctx, string(jobUUID))
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
func (c RestClient) PollJobStatus(ctx context.Context, payload *models.JobLinkResponse) error {
	job := payload.Job
	jobUUID := job.UUID

	checkJobStatus := func() error {
		isDone, err := c.IsJobFinished(ctx, payload)
		if err != nil {
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
		Logc(ctx).WithField("UUID", jobUUID).Warnf("Job not completed after %3.2f seconds.",
			jobStatusBackoff.MaxElapsedTime.Seconds())
		return err
	}

	Logc(ctx).WithField("UUID", jobUUID).Debug("Job completed.")
	jobResult, err := c.JobGet(ctx, string(jobUUID))
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

	Logc(ctx).WithFields(LogFields{
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
		return NewRestErrorFromPayload(jobResult.Payload)
	default:
		return fmt.Errorf("unexpected job state %v", jobState)
	}
}

// ////////////////////////////////////////////////////////////////////////////
// Aggregrate operations
// ////////////////////////////////////////////////////////////////////////////

// AggregateList returns the names of all Aggregates whose names match the supplied pattern
func (c RestClient) AggregateList(ctx context.Context, pattern string) (*storage.AggregateCollectionGetOK, error) {
	params := storage.NewAggregateCollectionGetParamsWithTimeout(c.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SetNameQueryParameter(ToStringPointer(pattern))
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := c.api.Storage.AggregateCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NextLoop:
		for !done {
			resultNext, errNext := c.api.Storage.AggregateCollectionGet(params, c.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NextLoop
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NextLoop
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

// ////////////////////////////////////////////////////////////////////////////
// SVM/Vserver operations
// ////////////////////////////////////////////////////////////////////////////

// SvmGet gets the volume with the specified uuid
func (c RestClient) SvmGet(ctx context.Context, uuid string) (*svm.SvmGetOK, error) {
	params := svm.NewSvmGetParamsWithTimeout(c.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = uuid

	return c.api.Svm.SvmGet(params, c.authInfo)
}

// SvmList returns the names of all SVMs whose names match the supplied pattern
func (c RestClient) SvmList(ctx context.Context, pattern string) (*svm.SvmCollectionGetOK, error) {
	params := svm.NewSvmCollectionGetParamsWithTimeout(c.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SetNameQueryParameter(ToStringPointer(pattern))
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := c.api.Svm.SvmCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NextLoop:
		for !done {
			resultNext, errNext := c.api.Svm.SvmCollectionGet(params, c.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NextLoop
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NextLoop
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

// ValidatePayloadExists returns an error if the Payload field is missing from the supplied restResult
func ValidatePayloadExists(ctx context.Context, restResult interface{}) (errorOut error) {
	defer func() {
		if r := recover(); r != nil {
			Logc(ctx).Errorf("Panic in ontap_rest#ValidatePayloadExists. %v\nStack Trace: %v",
				restResult, string(debug.Stack()))
			errorOut = fmt.Errorf("recovered from panic")
		}
	}()

	if restResult == nil {
		return fmt.Errorf("result was nil")
	}

	val := reflect.ValueOf(restResult)
	if reflect.TypeOf(restResult).Kind() == reflect.Ptr {
		// handle being passed a pointer
		val = reflect.Indirect(val)
		if !val.IsValid() {
			return fmt.Errorf("result was nil")
		}
	}

	// safely check to see if we have restResult.Payload
	if testPayload := val.FieldByName("Payload"); testPayload.IsValid() {
		restResult = testPayload.Interface()
		val = reflect.ValueOf(restResult)
		if reflect.TypeOf(restResult).Kind() == reflect.Ptr {
			val = reflect.Indirect(val)
			if !val.IsValid() {
				return fmt.Errorf("result payload was nil")
			}
		}
		return nil
	}

	return fmt.Errorf("no payload field exists for type '%v'", getType(restResult))
}

// ExtractErrorResponse returns any underlying *models.ErrorResponse from the supplied restError
func ExtractErrorResponse(ctx context.Context, restError interface{}) (errorResponse *models.ErrorResponse, errorOut error) {
	// for an example, see s_a_n.LunMapReportingNodeCollectionGetDefault
	defer func() {
		if r := recover(); r != nil {
			Logc(ctx).Errorf("Panic in ontap_rest#ExtractErrorResponse. %v\nStack Trace: %v",
				restError, string(debug.Stack()))
			errorOut = fmt.Errorf("recovered from panic")
		}
	}()

	if restError == nil {
		return nil, fmt.Errorf("rest error was nil")
	}

	val := reflect.ValueOf(restError)
	if reflect.TypeOf(restError).Kind() == reflect.Ptr {
		// handle being passed a pointer
		val = reflect.Indirect(val)
		if !val.IsValid() {
			return nil, fmt.Errorf("rest error was nil")
		}
	}

	// safely check to see if we have restResult.Payload
	if testPayload := val.FieldByName("Payload"); testPayload.IsValid() {
		restError = testPayload.Interface()
		val = reflect.ValueOf(restError)

		if apiError, ok := val.Interface().(*models.ErrorResponse); ok {
			return apiError, nil
		}
	}

	return nil, fmt.Errorf("no error payload field exists for type '%v'", getType(restError))
}

func getType(i interface{}) string {
	if t := reflect.TypeOf(i); t.Kind() == reflect.Ptr {
		return "*" + t.Elem().Name()
	} else {
		return t.Name()
	}
}

// SvmGetByName gets the volume with the specified name
func (c RestClient) SvmGetByName(ctx context.Context, svmName string) (*models.Svm, error) {
	result, err := c.SvmList(ctx, svmName)
	if err != nil {
		return nil, err
	}

	if validationErr := ValidatePayloadExists(ctx, result); validationErr != nil {
		return nil, validationErr
	}

	if result.Payload.NumRecords == 1 && result.Payload.Records != nil {
		return result.Payload.Records[0], nil
	}
	return nil, fmt.Errorf("unexpected result")
}

func (c RestClient) SVMGetAggregateNames(
	ctx context.Context,
) ([]string, error) {
	svm, err := c.SvmGetByName(ctx, c.svmName)
	if err != nil {
		return nil, err
	}
	if svm == nil {
		return nil, fmt.Errorf("could not find SVM %s", c.svmName)
	}

	aggrNames := make([]string, 0, 10)
	for _, aggr := range svm.Aggregates {
		aggrNames = append(aggrNames, string(aggr.Name))
	}

	return aggrNames, nil
}

// ////////////////////////////////////////////////////////////////////////////
// Misc operations
// ////////////////////////////////////////////////////////////////////////////

// ClusterInfo returns information about the cluster
func (c RestClient) ClusterInfo(
	ctx context.Context,
) (*cluster.ClusterGetOK, error) {
	params := cluster.NewClusterGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	return c.api.Cluster.ClusterGet(params, c.authInfo)
}

// SystemGetOntapVersion gets the ONTAP version using the credentials, and caches & returns the result.
func (c RestClient) SystemGetOntapVersion(
	ctx context.Context,
) (string, error) {
	if c.OntapVersion != "" {
		// return cached version
		return c.OntapVersion, nil
	}

	// it wasn't cached, look it up and cache it
	clusterInfoResult, err := c.ClusterInfo(ctx)
	if err != nil {
		return "unknown", err
	}
	if clusterInfoResult == nil {
		return "unknown", fmt.Errorf("could not determine cluster version")
	}

	version := clusterInfoResult.Payload.Version
	// version.Full // "NetApp Release 9.8X29: Sun Sep 27 12:15:48 UTC 2020"
	c.OntapVersion = fmt.Sprintf("%d.%d.%d", version.Generation, version.Major, version.Minor) // 9.8.0
	return c.OntapVersion, nil
}

// ClusterInfo returns information about the cluster
func (c RestClient) NodeList(ctx context.Context, pattern string) (*cluster.NodesGetOK, error) {
	params := cluster.NewNodesGetParamsWithTimeout(c.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SetNameQueryParameter(ToStringPointer(pattern))
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := c.api.Cluster.NodesGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NextLoop:
		for !done {
			resultNext, errNext := c.api.Cluster.NodesGet(params, c.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NextLoop
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NextLoop
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

func (c RestClient) NodeListSerialNumbers(ctx context.Context) ([]string, error) {
	serialNumbers := make([]string, 0)

	nodeListResult, err := c.NodeList(ctx, "*")
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

	Logc(ctx).WithFields(LogFields{
		"Count":         len(serialNumbers),
		"SerialNumbers": strings.Join(serialNumbers, ","),
	}).Debug("Read serial numbers.")

	return serialNumbers, nil
}

// EmsAutosupportLog generates an auto support message with the supplied parameters
func (c RestClient) EmsAutosupportLog(
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
	params := support.NewEmsApplicationLogsCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.ReturnRecordsQueryParameter = ToBoolPointer(true)

	emsApplicationLog := &models.EmsApplicationLog{
		AppVersion:          appVersion,
		AutosupportRequired: autoSupport,
		Category:            category,
		ComputerName:        computerName,
		EventDescription:    eventDescription,
		EventID:             int64(eventID),
		EventSource:         eventSource,
		Severity:            models.EmsApplicationLogSeverityNotice,
	}
	params.SetInfo(emsApplicationLog)

	_, err := c.api.Support.EmsApplicationLogsCreate(params, c.authInfo)
	if err != nil {
		if apiError, ok := err.(*runtime.APIError); ok {
			if apiError.Code == 200 {
				return nil // Skipping non-error caused by a result of just "{}""
			}
		}
	}
	return err
}

func (c RestClient) TieringPolicyValue(
	ctx context.Context,
) string {
	// Becase the REST API is always > ONTAP 9.5, just default to "none"
	tieringPolicy := "none"
	return tieringPolicy
}

// ///////////////////////////////////////////////////////////////////////////
// EXPORT POLICY operations BEGIN

// ExportPolicyCreate creates an export policy
// equivalent to filer::> vserver export-policy create
func (c RestClient) ExportPolicyCreate(ctx context.Context, policy string) (*nas.ExportPolicyCreateCreated, error) {
	params := nas.NewExportPolicyCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	exportPolicyInfo := &models.ExportPolicy{
		Name: policy,
		Svm: &models.ExportPolicySvm{
			Name: c.svmName,
		},
	}
	params.SetInfo(exportPolicyInfo)

	return c.api.Nas.ExportPolicyCreate(params, c.authInfo)
}

// ExportPolicyGet gets the export policy with the specified uuid
func (c RestClient) ExportPolicyGet(ctx context.Context, id int64) (*nas.ExportPolicyGetOK, error) {
	params := nas.NewExportPolicyGetParamsWithTimeout(c.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.IDPathParameter = id

	return c.api.Nas.ExportPolicyGet(params, c.authInfo)
}

// ExportPolicyList returns the names of all export polices whose names match the supplied pattern
func (c RestClient) ExportPolicyList(ctx context.Context, pattern string) (*nas.ExportPolicyCollectionGetOK, error) {
	params := nas.NewExportPolicyCollectionGetParamsWithTimeout(c.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SVMNameQueryParameter = &c.svmName

	params.SetNameQueryParameter(ToStringPointer(pattern))
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := c.api.Nas.ExportPolicyCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NextLoop:
		for !done {
			resultNext, errNext := c.api.Nas.ExportPolicyCollectionGet(params, c.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NextLoop
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NextLoop
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

// ExportPolicyGetByName gets the volume with the specified name
func (c RestClient) ExportPolicyGetByName(ctx context.Context, exportPolicyName string) (*models.ExportPolicy, error) {
	result, err := c.ExportPolicyList(ctx, exportPolicyName)
	if result.Payload.NumRecords == 1 && result.Payload.Records != nil {
		return result.Payload.Records[0], nil
	}
	return nil, err
}

func (c RestClient) ExportPolicyDestroy(ctx context.Context, policy string) (*nas.ExportPolicyDeleteOK, error) {
	exportPolicy, err := c.ExportPolicyGetByName(ctx, policy)
	if err != nil {
		return nil, err
	}
	if exportPolicy == nil {
		return nil, fmt.Errorf("could not get export policy %v", policy)
	}

	params := nas.NewExportPolicyDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.IDPathParameter = exportPolicy.ID

	return c.api.Nas.ExportPolicyDelete(params, c.authInfo)
}

// ExportRuleList returns the export rules in an export policy
// equivalent to filer::> vserver export-policy rule show
func (c RestClient) ExportRuleList(ctx context.Context, policy string) (*nas.ExportRuleCollectionGetOK, error) {
	exportPolicy, err := c.ExportPolicyGetByName(ctx, policy)
	if err != nil {
		return nil, err
	}
	if exportPolicy == nil {
		return nil, fmt.Errorf("could not get export policy %v", policy)
	}

	params := nas.NewExportRuleCollectionGetParamsWithTimeout(c.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.PolicyIDPathParameter = exportPolicy.ID

	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := c.api.Nas.ExportRuleCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NextLoop:
		for !done {
			resultNext, errNext := c.api.Nas.ExportRuleCollectionGet(params, c.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NextLoop
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NextLoop
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

// ExportRuleCreate creates a rule in an export policy
// equivalent to filer::> vserver export-policy rule create
func (c RestClient) ExportRuleCreate(
	ctx context.Context, policy, clientMatch string, protocols, roSecFlavors, rwSecFlavors, suSecFlavors []string,
) (*nas.ExportRuleCreateCreated, error) {
	exportPolicy, err := c.ExportPolicyGetByName(ctx, policy)
	if err != nil {
		return nil, err
	}
	if exportPolicy == nil {
		return nil, fmt.Errorf("could not get export policy %v", policy)
	}

	params := nas.NewExportRuleCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.PolicyIDPathParameter = exportPolicy.ID

	info := &models.ExportRule{}

	var clients []*models.ExportClients
	for _, match := range strings.Split(clientMatch, ",") {
		clients = append(clients, &models.ExportClients{Match: match})
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

	return c.api.Nas.ExportRuleCreate(params, c.authInfo)
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
func (c RestClient) ExportRuleDestroy(
	ctx context.Context, policy string, ruleIndex int,
) (*nas.ExportRuleDeleteOK, error) {
	exportPolicy, err := c.ExportPolicyGetByName(ctx, policy)
	if err != nil {
		return nil, err
	}
	if exportPolicy == nil {
		return nil, fmt.Errorf("could not get export policy %v", policy)
	}

	params := nas.NewExportRuleDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.PolicyIDPathParameter = exportPolicy.ID
	params.IndexPathParameter = int64(ruleIndex)

	return c.api.Nas.ExportRuleDelete(params, c.authInfo)
}

// ///////////////////////////////////////////////////////////////////////////
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
func (c RestClient) FlexGroupCreate(
	ctx context.Context, name string, size int, aggrs []string, spaceReserve, snapshotPolicy, unixPermissions,
	exportPolicy, securityStyle, tieringPolicy, comment string, qosPolicyGroup QosPolicyGroup, encrypt *bool,
	snapshotReserve int,
) error {
	return c.createVolumeByStyle(ctx, name, int64(size), aggrs, spaceReserve, snapshotPolicy, unixPermissions,
		exportPolicy, securityStyle, tieringPolicy, comment, qosPolicyGroup, encrypt,
		snapshotReserve, models.VolumeStyleFlexgroup)
}

// FlexgroupCloneSplitStart starts splitting the flexgroup clone
func (c RestClient) FlexgroupCloneSplitStart(ctx context.Context, volumeName string) error {
	return c.startCloneSplitByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexgroup)
}

// FlexGroupDestroy destroys a FlexGroup
func (c RestClient) FlexGroupDestroy(ctx context.Context, name string) error {
	volume, err := c.FlexGroupGetByName(ctx, name)
	if err != nil {
		return err
	}
	params := storage.NewVolumeDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = volume.UUID

	volumeDeleteAccepted, err := c.api.Storage.VolumeDelete(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeDeleteAccepted == nil {
		return fmt.Errorf("unexpected response from volume delete")
	}

	return c.PollJobStatus(ctx, volumeDeleteAccepted.Payload)
}

// FlexGroupExists tests for the existence of a FlexGroup
func (c RestClient) FlexGroupExists(ctx context.Context, volumeName string) (bool, error) {
	return c.checkVolumeExistsByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexgroup)
}

// FlexGroupSize retrieves the size of the specified flexgroup
func (c RestClient) FlexGroupSize(ctx context.Context, volumeName string) (uint64, error) {
	return c.getVolumeSizeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexgroup)
}

// FlexGroupUsedSize retrieves the used space of the specified volume
func (c RestClient) FlexGroupUsedSize(ctx context.Context, volumeName string) (int, error) {
	return c.getVolumeUsedSizeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexgroup)
}

// FlexGroupSetSize sets the size of the specified FlexGroup
func (c RestClient) FlexGroupSetSize(ctx context.Context, volumeName, newSize string) error {
	return c.setVolumeSizeByNameAndStyle(ctx, volumeName, newSize, models.VolumeStyleFlexgroup)
}

// FlexgroupSetQosPolicyGroupName note: we can't set adaptive policy groups directly during volume clone creation.
func (c RestClient) FlexgroupSetQosPolicyGroupName(
	ctx context.Context, volumeName string, qosPolicyGroup QosPolicyGroup,
) error {
	return c.setVolumeQosPolicyGroupNameByNameAndStyle(ctx, volumeName, qosPolicyGroup, models.VolumeStyleFlexgroup)
}

// FlexGroupVolumeDisableSnapshotDirectoryAccess disables access to the ".snapshot" directory
// Disable '.snapshot' to allow official mysql container's chmod-in-init to work
func (c RestClient) FlexGroupVolumeDisableSnapshotDirectoryAccess(ctx context.Context, flexGroupVolumeName string) error {
	volume, err := c.getVolumeByNameAndStyle(ctx, flexGroupVolumeName, models.VolumeStyleFlexgroup)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find flexgroup volume with name %v", flexGroupVolumeName)
	}

	uuid := volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUIDPathParameter = uuid

	volumeInfo := &models.Volume{}
	volumeInfo.SnapshotDirectoryAccessEnabled = ToBoolPointer(false)
	params.SetInfo(volumeInfo)

	volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return c.PollJobStatus(ctx, volumeModifyAccepted.Payload)
}

func (c RestClient) FlexGroupModifyUnixPermissions(ctx context.Context, volumeName, unixPermissions string) error {
	return c.modifyVolumeUnixPermissionsByNameAndStyle(ctx, volumeName, unixPermissions, models.VolumeStyleFlexgroup)
}

// FlexGroupSetComment sets a flexgroup's comment to the supplied value
func (c RestClient) FlexGroupSetComment(ctx context.Context, volumeName, newVolumeComment string) error {
	return c.setVolumeCommentByNameAndStyle(ctx, volumeName, newVolumeComment, models.VolumeStyleFlexgroup)
}

// FlexGroupGetByName gets the flexgroup with the specified name
func (c RestClient) FlexGroupGetByName(ctx context.Context, volumeName string) (*models.Volume, error) {
	return c.getVolumeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexgroup)
}

// FlexGroupGetAll returns all relevant details for all FlexGroups whose names match the supplied prefix
func (c RestClient) FlexGroupGetAll(ctx context.Context, pattern string) (*storage.VolumeCollectionGetOK, error) {
	return c.getAllVolumesByPatternStyleAndState(ctx, pattern, models.VolumeStyleFlexgroup, models.VolumeStateOnline)
}

// FlexGroupMount mounts a flexgroup at the specified junction
func (c RestClient) FlexGroupMount(ctx context.Context, volumeName, junctionPath string) error {
	return c.mountVolumeByNameAndStyle(ctx, volumeName, junctionPath, models.VolumeStyleFlexgroup)
}

// FlexgroupUnmount unmounts the flexgroup
func (c RestClient) FlexgroupUnmount(ctx context.Context, volumeName string) error {
	return c.unmountVolumeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexgroup)
}

func (c RestClient) FlexgroupModifyExportPolicy(ctx context.Context, volumeName, exportPolicyName string) error {
	return c.modifyVolumeExportPolicyByNameAndStyle(ctx, volumeName, exportPolicyName, models.VolumeStyleFlexgroup)
}

// ///////////////////////////////////////////////////////////////////////////
// FlexGroup operations END
// ///////////////////////////////////////////////////////////////////////////

// ///////////////////////////////////////////////////////////////////////////
// QTREE operations BEGIN

// QtreeCreate creates a qtree with the specified options
// equivalent to filer::> qtree create -vserver ndvp_vs -volume v -qtree q -export-policy default -unix-permissions ---rwxr-xr-x -security-style unix
func (c RestClient) QtreeCreate(
	ctx context.Context, name, volumeName, unixPermissions, exportPolicy, securityStyle, qosPolicy string,
) error {
	params := storage.NewQtreeCreateParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	qtreeInfo := &models.Qtree{
		Name:   name,
		Volume: &models.QtreeVolume{Name: volumeName},
		Svm:    &models.QtreeSvm{Name: c.svmName},
	}

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
		qtreeInfo.ExportPolicy = &models.QtreeExportPolicy{Name: exportPolicy}
	}
	if securityStyle != "" {
		qtreeInfo.SecurityStyle = models.SecurityStyle(securityStyle)
	}
	if qosPolicy != "" {
		qtreeInfo.QosPolicy = &models.QtreeQosPolicy{Name: qosPolicy}
	}

	params.SetInfo(qtreeInfo)

	createAccepted, err := c.api.Storage.QtreeCreate(params, c.authInfo)
	if err != nil {
		return err
	}
	if createAccepted == nil {
		return fmt.Errorf("unexpected response from qtree create")
	}

	return c.PollJobStatus(ctx, createAccepted.Payload)
}

// QtreeRename renames a qtree
// equivalent to filer::> volume qtree rename
func (c RestClient) QtreeRename(ctx context.Context, path, newPath string) error {
	qtree, err := c.QtreeGetByPath(ctx, path)
	if err != nil {
		return err
	}
	if qtree == nil {
		return fmt.Errorf("could not find qtree with path %v", path)
	}

	params := storage.NewQtreeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetIDPathParameter(strconv.FormatInt(int64(qtree.ID), 10))
	params.SetVolumeUUIDPathParameter(qtree.Volume.UUID)

	qtreeInfo := &models.Qtree{
		Name: strings.TrimPrefix(newPath, "/"+qtree.Volume.Name+"/"),
	}

	params.SetInfo(qtreeInfo)

	modifyAccepted, err := c.api.Storage.QtreeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if modifyAccepted == nil {
		return fmt.Errorf("unexpected response from qtree modify")
	}

	return c.PollJobStatus(ctx, modifyAccepted.Payload)
}

// QtreeDestroyAsync destroys a qtree in the background
// equivalent to filer::> volume qtree delete -foreground false
func (c RestClient) QtreeDestroyAsync(ctx context.Context, path string, force bool) error {
	// note, force isn't used
	qtree, err := c.QtreeGetByPath(ctx, path)
	if err != nil {
		return err
	}
	if qtree == nil {
		return fmt.Errorf("unexpected response from qtree lookup by path")
	}
	if qtree.Volume == nil {
		return fmt.Errorf("unexpected response from qtree lookup by path, missing volume information")
	}

	params := storage.NewQtreeDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetIDPathParameter(strconv.FormatInt(int64(qtree.ID), 10))
	params.SetVolumeUUIDPathParameter(qtree.Volume.UUID)

	deleteAccepted, err := c.api.Storage.QtreeDelete(params, c.authInfo)
	if err != nil {
		return err
	}
	if deleteAccepted == nil {
		return fmt.Errorf("unexpected response from quota delete")
	}

	return c.PollJobStatus(ctx, deleteAccepted.Payload)
}

// QtreeList returns the names of all Qtrees whose names match the supplied prefix
// equivalent to filer::> volume qtree show
func (c RestClient) QtreeList(ctx context.Context, prefix, volumePrefix string) (*storage.QtreeCollectionGetOK, error) {
	namePattern := "*"
	if prefix != "" {
		namePattern = prefix + "*"
	}

	volumePattern := "*"
	if volumePrefix != "" {
		volumePattern = volumePrefix + "*"
	}

	// Limit the qtrees to those matching the Flexvol name prefix
	params := storage.NewQtreeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	// params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SetSVMNameQueryParameter(ToStringPointer(c.svmName))
	params.SetNameQueryParameter(ToStringPointer(namePattern))         // Qtree name prefix
	params.SetVolumeNameQueryParameter(ToStringPointer(volumePattern)) // Flexvol name prefix
	params.SetFieldsQueryParameter([]string{"**"})                     // TODO trim these down to just what we need

	result, err := c.api.Storage.QtreeCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NextLoop:
		for !done {
			resultNext, errNext := c.api.Storage.QtreeCollectionGet(params, c.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NextLoop
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NextLoop
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

// QtreeGetByPath gets the qtree with the specified path
func (c RestClient) QtreeGetByPath(ctx context.Context, path string) (*models.Qtree, error) {
	// Limit the qtrees to those specified path
	params := storage.NewQtreeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	// params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SetSVMNameQueryParameter(ToStringPointer(c.svmName))
	params.SetPathQueryParameter(ToStringPointer(path))
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := c.api.Storage.QtreeCollectionGet(params, c.authInfo)
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
func (c RestClient) QtreeGetByName(ctx context.Context, name, volumeName string) (*models.Qtree, error) {
	// Limit to the single qtree /volumeName/name
	params := storage.NewQtreeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	// params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SVMNameQueryParameter = &c.svmName
	params.SetNameQueryParameter(ToStringPointer(name))
	params.SetVolumeNameQueryParameter(ToStringPointer(volumeName))
	params.SetFieldsQueryParameter([]string{"**"}) // TODO trim these down to just what we need

	result, err := c.api.Storage.QtreeCollectionGet(params, c.authInfo)
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
func (c RestClient) QtreeCount(ctx context.Context, volumeName string) (int, error) {
	// Limit the qtrees to those in the specified Flexvol name
	params := storage.NewQtreeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	// params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SetSVMNameQueryParameter(ToStringPointer(c.svmName))
	params.SetVolumeNameQueryParameter(ToStringPointer(volumeName)) // Flexvol name
	params.SetFieldsQueryParameter([]string{"**"})                  // TODO trim these down to just what we need

	result, err := c.api.Storage.QtreeCollectionGet(params, c.authInfo)
	if err != nil {
		return 0, err
	}
	if result == nil {
		return 0, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NextLoop:
		for !done {
			resultNext, errNext := c.api.Storage.QtreeCollectionGet(params, c.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return 0, errNext
			}
			if resultNext == nil {
				done = true
				continue NextLoop
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NextLoop
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}

	// There will always be one qtree for the Flexvol, so decrement by 1
	n := int(result.Payload.NumRecords)
	switch n {
	case 0, 1:
		return 0, nil
	default:
		return n - 1, nil
	}
}

// QtreeExists returns true if the named Qtree exists (and is unique in the matching Flexvols)
func (c RestClient) QtreeExists(ctx context.Context, name, volumePattern string) (bool, string, error) {
	// Limit the qtrees to those matching the Flexvol and Qtree name prefixes
	params := storage.NewQtreeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	// params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SetSVMNameQueryParameter(ToStringPointer(c.svmName))
	params.SetNameQueryParameter(ToStringPointer(name))                // Qtree name
	params.SetVolumeNameQueryParameter(ToStringPointer(volumePattern)) // Flexvol name prefix
	params.SetFieldsQueryParameter([]string{"**"})                     // TODO trim these down to just what we need

	result, err := c.api.Storage.QtreeCollectionGet(params, c.authInfo)
	if err != nil {
		return false, "", err
	}
	if result == nil {
		return false, "", nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NextLoop:
		for !done {
			resultNext, errNext := c.api.Storage.QtreeCollectionGet(params, c.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return false, "", errNext
			}
			if resultNext == nil {
				done = true
				continue NextLoop
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NextLoop
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
func (c RestClient) QtreeGet(ctx context.Context, name, volumePrefix string) (*models.Qtree, error) {
	pattern := "*"
	if volumePrefix != "" {
		pattern = volumePrefix + "*"
	}

	// Limit the qtrees to those matching the Flexvol name prefix
	params := storage.NewQtreeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	// params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SetSVMNameQueryParameter(ToStringPointer(c.svmName))
	params.SetNameQueryParameter(ToStringPointer(name))          // qtree name
	params.SetVolumeNameQueryParameter(ToStringPointer(pattern)) // Flexvol name prefix
	params.SetFieldsQueryParameter([]string{"**"})               // TODO trim these down to just what we need

	result, err := c.api.Storage.QtreeCollectionGet(params, c.authInfo)
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
func (c RestClient) QtreeGetAll(ctx context.Context, volumePrefix string) (*storage.QtreeCollectionGetOK, error) {
	pattern := "*"
	if volumePrefix != "" {
		pattern = volumePrefix + "*"
	}

	// Limit the qtrees to those matching the Flexvol name prefix
	params := storage.NewQtreeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	// params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SVMNameQueryParameter = &c.svmName
	params.SetVolumeNameQueryParameter(ToStringPointer(pattern)) // Flexvol name prefix
	params.SetFieldsQueryParameter([]string{"**"})               // TODO trim these down to just what we need

	result, err := c.api.Storage.QtreeCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NextLoop:
		for !done {
			resultNext, errNext := c.api.Storage.QtreeCollectionGet(params, c.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NextLoop
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NextLoop
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return result, nil
}

// QtreeModifyExportPolicy modifies the export policy for the qtree
func (c RestClient) QtreeModifyExportPolicy(ctx context.Context, name, volumeName, newExportPolicyName string) error {
	qtree, err := c.QtreeGetByName(ctx, name, volumeName)
	if err != nil {
		return err
	}
	if qtree == nil {
		return fmt.Errorf("could not find qtree %v", name)
	}

	params := storage.NewQtreeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetIDPathParameter(strconv.FormatInt(int64(qtree.ID), 10))
	params.SetVolumeUUIDPathParameter(qtree.Volume.UUID)

	qtreeInfo := &models.Qtree{
		ExportPolicy: &models.QtreeExportPolicy{
			Name: newExportPolicyName,
		},
	}

	params.SetInfo(qtreeInfo)

	modifyAccepted, err := c.api.Storage.QtreeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if modifyAccepted == nil {
		return fmt.Errorf("unexpected response from qtree modify")
	}

	return c.PollJobStatus(ctx, modifyAccepted.Payload)
}

// QuotaOn enables quotas on a Flexvol
// equivalent to filer::> volume quota on
func (c RestClient) QuotaOn(ctx context.Context, volumeName string) error {
	return c.quotaModify(ctx, volumeName, true)
}

// QuotaOff disables quotas on a Flexvol
// equivalent to filer::> volume quota off
func (c RestClient) QuotaOff(ctx context.Context, volumeName string) error {
	return c.quotaModify(ctx, volumeName, false)
}

// quotaModify enables/disables quotas on a Flexvol
func (c RestClient) quotaModify(ctx context.Context, volumeName string, quotaEnabled bool) error {
	volume, err := c.VolumeGetByName(ctx, volumeName)
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

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetUUIDPathParameter(volume.UUID)

	volumeInfo := &models.Volume{
		Quota: &models.VolumeQuota{
			Enabled: quotaEnabled,
		},
	}

	params.SetInfo(volumeInfo)

	modifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if modifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	return c.PollJobStatus(ctx, modifyAccepted.Payload)
}

// QuotaSetEntry updates (or creates) a quota rule with an optional hard disk limit
// equivalent to filer::> volume quota policy rule modify
func (c RestClient) QuotaSetEntry(ctx context.Context, qtreeName, volumeName, quotaType, diskLimit string) error {
	// We can only modify existing rules, so we must check if this rule exists first
	quotaRule, err := c.QuotaGetEntry(ctx, volumeName, qtreeName, quotaType)
	if err != nil && !utils.IsNotFoundError(err) {
		// Error looking up quota rule
		Logc(ctx).Error(err)
		return err
	} else if err != nil && utils.IsNotFoundError(err) {
		// Quota rule doesn't exist; add it instead
		return c.QuotaAddEntry(ctx, volumeName, qtreeName, quotaType, diskLimit)
	}
	// Quota rule exists; modify it
	params := storage.NewQuotaRuleModifyParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetUUIDPathParameter(quotaRule.UUID)

	quotaRuleInfo := &models.QuotaRule{
		Qtree: &models.QuotaRuleQtree{
			Name: qtreeName,
		},
		Volume: &models.QuotaRuleVolume{
			Name: volumeName,
		},
		Type: quotaType,
	}

	quotaRuleInfo.Svm = &models.QuotaRuleSvm{Name: c.svmName}
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
	params.SetInfo(quotaRuleInfo)

	modifyAccepted, err := c.api.Storage.QuotaRuleModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if modifyAccepted == nil {
		return fmt.Errorf("unexpected response from quota rule modify")
	}

	return c.PollJobStatus(ctx, modifyAccepted.Payload)
}

// QuotaAddEntry creates a quota rule with an optional hard disk limit
// equivalent to filer::> volume quota policy rule create
func (c RestClient) QuotaAddEntry(ctx context.Context, volumeName, qtreeName, quotaType, diskLimit string) error {
	params := storage.NewQuotaRuleCreateParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	quotaRuleInfo := &models.QuotaRule{
		Qtree: &models.QuotaRuleQtree{
			Name: qtreeName,
		},
		Volume: &models.QuotaRuleVolume{
			Name: volumeName,
		},
		Type: quotaType,
	}

	quotaRuleInfo.Svm = &models.QuotaRuleSvm{Name: c.svmName}

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

	params.SetInfo(quotaRuleInfo)

	createAccepted, err := c.api.Storage.QuotaRuleCreate(params, c.authInfo)
	if err != nil {
		return err
	}
	if createAccepted == nil {
		return fmt.Errorf("unexpected response from quota rule create")
	}

	return c.PollJobStatus(ctx, createAccepted.Payload)
}

// QuotaGetEntry returns the disk limit for a single qtree
// equivalent to filer::> volume quota policy rule show
func (c RestClient) QuotaGetEntry(
	ctx context.Context, volumeName, qtreeName, quotaType string,
) (*models.QuotaRule, error) {
	params := storage.NewQuotaRuleCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	// params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SetTypeQueryParameter(ToStringPointer(quotaType))
	params.SetSVMNameQueryParameter(ToStringPointer(c.svmName))
	params.SetQtreeNameQueryParameter(ToStringPointer(qtreeName))
	params.SetVolumeNameQueryParameter(ToStringPointer(volumeName))

	params.SetFieldsQueryParameter([]string{"uuid", "space.hard_limit"})

	result, err := c.api.Storage.QuotaRuleCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NextLoop:
		for !done {
			resultNext, errNext := c.api.Storage.QuotaRuleCollectionGet(params, c.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NextLoop
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NextLoop
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}

	target := volumeName
	if qtreeName != "" {
		target += fmt.Sprintf("/%s", qtreeName)
	}
	if result.Payload == nil {
		return nil, utils.NotFoundError(fmt.Sprintf("quota rule entries for %s not found", target))
	} else if len(result.Payload.Records) > 1 {
		return nil, fmt.Errorf("more than one quota rule entry for %s found", target)
	} else if len(result.Payload.Records) == 1 {
		return result.Payload.Records[0], nil
	} else if HasNextLink(result.Payload) {
		return nil, fmt.Errorf("more than one quota rule entry for %s found", target)
	}

	return nil, utils.NotFoundError(fmt.Sprintf("no entries for %s", target))
}

// QuotaEntryList returns the disk limit quotas for a Flexvol
// equivalent to filer::> volume quota policy rule show
func (c RestClient) QuotaEntryList(ctx context.Context, volumeName string) (*storage.QuotaRuleCollectionGetOK, error) {
	params := storage.NewQuotaRuleCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	// params.MaxRecords = ToInt64Pointer(1) // use for testing, forces pagination

	params.SVMNameQueryParameter = &c.svmName
	params.VolumeNameQueryParameter = ToStringPointer(volumeName)
	params.TypeQueryParameter = ToStringPointer("tree")

	params.SetFieldsQueryParameter([]string{"space.hard_limit", "uuid", "qtree.name", "volume.name"})

	result, err := c.api.Storage.QuotaRuleCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NextLoop:
		for !done {
			resultNext, errNext := c.api.Storage.QuotaRuleCollectionGet(params, c.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NextLoop
			}

			result.Payload.NumRecords += resultNext.Payload.NumRecords
			result.Payload.Records = append(result.Payload.Records, resultNext.Payload.Records...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NextLoop
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}

	return result, nil
}

// QTREE operations END
// ///////////////////////////////////////////////////////////////////////////
