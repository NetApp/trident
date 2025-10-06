// Copyright 2025 NetApp, Inc. All Rights Reserved.

package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-openapi/runtime"
	runtime_client "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/capacity"
	"github.com/netapp/trident/pkg/convert"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/application"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/cluster"
	nas "github.com/netapp/trident/storage_drivers/ontap/api/rest/client/n_a_s"
	nvme "github.com/netapp/trident/storage_drivers/ontap/api/rest/client/n_v_me"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/networking"
	san "github.com/netapp/trident/storage_drivers/ontap/api/rest/client/s_a_n"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/snapmirror"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/storage"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/support"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/svm"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/netapp/trident/utils/errors"
	versionutils "github.com/netapp/trident/utils/version"
)

// //////////////////////////////////////////////////////////////////////////////////////////////////////
// REST layer
// //////////////////////////////////////////////////////////////////////////////////////////////////////

var returnTimeout = convert.ToPtr(int64(2)) // seconds

// RestClient is the object to use for interacting with ONTAP controllers via the REST API
type RestClient struct {
	config        ClientConfig
	tr            *http.Transport
	httpClient    *http.Client
	api           *client.ONTAPRESTAPIOnlineReference
	authInfo      runtime.ClientAuthInfoWriter
	OntapVersion  string
	driverName    string
	svmUUID       string
	svmName       string
	clusterUUID   string
	sanOptimized  bool
	disaggregated bool
	m             *sync.RWMutex
}

func (c *RestClient) ClientConfig() ClientConfig {
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

// IsSANOptimized returns whether this client has detected a SAN-optimized ONTAP cluster.  This
// property is discovered in SystemGetOntapVersion().
func (c *RestClient) IsSANOptimized() bool {
	c.m.RLock()
	defer c.m.RUnlock()
	return c.sanOptimized
}

func (c *RestClient) SetSANOptimized(sanOptimized bool) {
	c.m.Lock()
	defer c.m.Unlock()
	c.sanOptimized = sanOptimized
}

// IsDisaggregated returns whether this client has detected a disaggregated ONTAP cluster.  This
// property is discovered in SystemGetOntapVersion().
func (c *RestClient) IsDisaggregated() bool {
	return c.disaggregated
}

func (c *RestClient) SetDisaggregated(disaggregated bool) {
	c.m.Lock()
	defer c.m.Unlock()
	c.disaggregated = disaggregated
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
		m:          &sync.RWMutex{},
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
	if config.unitTestTransportConfigSchemes != "" {
		transportConfig.Schemes = []string{config.unitTestTransportConfigSchemes}
	}

	result.api = client.NewHTTPClientWithConfig(formats, transportConfig)

	if config.Username != "" && config.Password != "" {
		result.authInfo = runtime_client.BasicAuth(config.Username, config.Password)
	}

	if rClient, ok := result.api.Transport.(*runtime_client.Runtime); ok {
		apiLogger := &log.Logger{
			Out:       os.Stdout,
			Formatter: &Redactor{BaseFormatter: new(log.TextFormatter)},
			Level:     log.DebugLevel,
		}
		rClient.SetLogger(apiLogger)
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
		if vserver.UUID != nil {
			restClient.SetSVMUUID(*vserver.UUID)
		}

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

		if result.Payload.SvmResponseInlineRecords == nil || result.Payload.NumRecords == nil || *result.Payload.NumRecords != 1 {
			// if NumRecords has more than 1 result, not going to guess
			return errors.New(errorMsg)
		}

		// Use our derived SVM
		derivedSVM := result.Payload.SvmResponseInlineRecords[0]
		if derivedSVM.Name != nil {
			ontapConfig.SVM = *derivedSVM.Name
			restClient.SetSVMName(*derivedSVM.Name)
		} else {
			// derivedSVM.Name is nil
			return errors.New(errorMsg)
		}

		svmUUID := derivedSVM.UUID
		if svmUUID != nil {
			restClient.SetSVMUUID(*svmUUID)
		} else {
			// derivedSVM.UUID is nil
			return errors.New(errorMsg)
		}

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

	if _, err = restClient.SystemGetOntapVersion(ctx, false); err != nil {
		return nil, err
	}

	if err = EnsureSVMWithRest(ctx, ontapConfig, restClient); err != nil {
		return nil, err
	}

	apiREST, err := NewOntapAPIREST(restClient, ontapConfig.StorageDriverName)
	if err != nil {
		return nil, fmt.Errorf("unable to get REST API client for ontap; %v", err)
	}

	return apiREST, nil
}

var (
	MinimumONTAPVersion                             = versionutils.MustParseSemantic("9.12.1")
	MinimumONTAPVersionDefault                      = versionutils.MustParseSemantic("9.15.1")
	MinimumASAR2Version                             = versionutils.MustParseSemantic("9.16.1")
	MinimumDisaggregatedTieringPolicyRemovedVersion = versionutils.MustParseSemantic("9.17.0")
)

func IsRESTSupported(version string) (bool, error) {
	ontapSemVer, err := versionutils.ParseSemantic(version)
	if err != nil {
		return false, err
	}

	if !ontapSemVer.AtLeast(MinimumONTAPVersion) {
		return false, nil
	}

	return true, nil
}

func IsRESTSupportedDefault(version string) (bool, error) {
	ontapSemVer, err := versionutils.ParseSemantic(version)
	if err != nil {
		return false, err
	}

	if !ontapSemVer.AtLeast(MinimumONTAPVersionDefault) {
		return false, nil
	}

	return true, nil
}

// SupportsFeature returns true if the Ontap version supports the supplied feature
func (c *RestClient) SupportsFeature(ctx context.Context, feature Feature) bool {
	ontapVersion, err := c.SystemGetOntapVersion(ctx, true)
	if err != nil {
		return false
	}

	ontapSemVer, err := versionutils.ParseSemantic(ontapVersion)
	if err != nil {
		return false
	}

	if minVersion, ok := featuresByVersion[feature]; ok {
		return ontapSemVer.AtLeast(minVersion)
	} else {
		return false
	}
}

// QueryWrapper wraps a Param instance and overrides query parameters
type QueryWrapper struct {
	originalParams runtime.ClientRequestWriter
	queryParams    map[string]string
}

// WriteToRequest adds custom query parameters to the request
func (q *QueryWrapper) WriteToRequest(req runtime.ClientRequest, reg strfmt.Registry) error {
	// Call the original Params function if it exists
	if q.originalParams != nil {
		if err := q.originalParams.WriteToRequest(req, reg); err != nil {
			return err
		}
	}
	// Add the custom query parameters
	for k, v := range q.queryParams {
		if err := req.SetQueryParam(k, v); err != nil {
			return err
		}
	}
	return nil
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

	if o.next == nil || o.next.Href == nil || *o.next.Href == "" {
		// nothing to do
		return nil
	}

	// now, override query parameters values as needed. see also:
	//   -  https://play.golang.org/p/mjRu2iYod9N
	u, parseErr := url.Parse(*o.next.Href)
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

func (c *RestClient) getAllVolumePayloadRecords(
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
			if resultNext == nil || resultNext.Payload == nil || resultNext.Payload.NumRecords == nil {
				break
			}

			if payload.NumRecords == nil {
				payload.NumRecords = convert.ToPtr(int64(0))
			}
			payload.NumRecords = convert.ToPtr(*payload.NumRecords + *resultNext.Payload.NumRecords)
			payload.VolumeResponseInlineRecords = append(payload.VolumeResponseInlineRecords,
				resultNext.Payload.VolumeResponseInlineRecords...)

			if !HasNextLink(resultNext.Payload) {
				break
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return payload, nil
}

// getAllVolumesByPatternStyleAndState returns all relevant details for all volumes of the style specified whose names match the supplied prefix
func (c *RestClient) getAllVolumesByPatternStyleAndState(
	ctx context.Context, pattern, style, state string, fields []string,
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

	// params.MaxRecords = utils.ToPtr(int64(1)) // use for testing, forces pagination

	params.SvmUUID = &c.svmUUID
	params.SetName(convert.ToPtr(pattern))
	if state != "" {
		params.SetState(convert.ToPtr(state))
	}
	params.SetStyle(convert.ToPtr(style))
	params.SetFields(fields)

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
func (c *RestClient) checkVolumeExistsByNameAndStyle(ctx context.Context, volumeName, style string) (bool, error) {
	if volumeName == "" {
		return false, nil
	}
	fields := []string{""}
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style, fields)
	if err != nil {
		return false, err
	}
	if volume == nil {
		return false, err
	}
	return true, nil
}

// getVolumeByNameAndStyle gets the volume of the style and name specified
func (c *RestClient) getVolumeByNameAndStyle(
	ctx context.Context,
	volumeName string,
	style string,
	fields []string,
) (*models.Volume, error) {
	result, err := c.getAllVolumesByPatternStyleAndState(ctx, volumeName, style, models.VolumeStateOnline, fields)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Payload == nil || result.Payload.NumRecords == nil || *result.Payload.NumRecords == 0 {
		return nil, nil
	}
	if *result.Payload.NumRecords == 1 && result.Payload.VolumeResponseInlineRecords != nil {
		return result.Payload.VolumeResponseInlineRecords[0], nil
	}
	return nil, fmt.Errorf("could not find unique volume with name '%v'; found %d matching volumes",
		volumeName, result.Payload.NumRecords)
}

// getVolumeInAnyStateByNameAndStyle gets the volume of the style and name specified
func (c *RestClient) getVolumeInAnyStateByNameAndStyle(
	ctx context.Context,
	volumeName string,
	style string,
	fields []string,
) (*models.Volume, error) {
	result, err := c.getAllVolumesByPatternStyleAndState(ctx, volumeName, style, "", fields)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Payload == nil || result.Payload.NumRecords == nil || *result.Payload.NumRecords == 0 {
		return nil, nil
	}
	if *result.Payload.NumRecords == 1 && result.Payload.VolumeResponseInlineRecords != nil {
		return result.Payload.VolumeResponseInlineRecords[0], nil
	}
	return nil, fmt.Errorf("could not find unique volume with name '%v'; found %d matching volumes", volumeName,
		result.Payload.NumRecords)
}

// getVolumeSizeByNameAndStyle retrieves the size of the volume of the style and name specified
func (c *RestClient) getVolumeSizeByNameAndStyle(ctx context.Context, volumeName, style string) (uint64, error) {
	fields := []string{"size"}
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style, fields)
	if err != nil {
		return 0, err
	}
	if volume == nil {
		return 0, fmt.Errorf("could not find volume with name %v", volumeName)
	}
	if volume.Size == nil {
		return 0, fmt.Errorf("could not find size for volume with name %v", volumeName)
	}

	return uint64(*volume.Size), nil
}

// getVolumeUsedSizeByNameAndStyle retrieves the used bytes of the the volume of the style and name specified
func (c *RestClient) getVolumeUsedSizeByNameAndStyle(ctx context.Context, volumeName, style string) (int, error) {
	fields := []string{"space.logical_space.used"}
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style, fields)
	if err != nil {
		return 0, err
	}
	if volume == nil {
		return 0, errors.NotFoundError("could not find volume with name %v", volumeName)
	}

	if volume.Space == nil {
		return 0, fmt.Errorf("could not find space attributes for volume %v", volumeName)
	}

	if volume.Space.LogicalSpace == nil {
		return 0, fmt.Errorf("could not find logical space attributes for volume %v", volumeName)
	}

	if volume.Space.LogicalSpace.Used == nil {
		return 0, fmt.Errorf("could not find logical space attributes for volume %v", volumeName)
	}

	return int(*volume.Space.LogicalSpace.Used), nil
}

// setVolumeSizeByNameAndStyle sets the size of the specified volume of given style
func (c *RestClient) setVolumeSizeByNameAndStyle(ctx context.Context, volumeName, newSize, style string) error {
	fields := []string{""}
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style, fields)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}
	if volume.UUID == nil {
		return fmt.Errorf("could not find volume uuid with name %v", volumeName)
	}

	uuid := *volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = uuid
	params.SetReturnTimeout(returnTimeout)

	sizeBytesStr, _ := capacity.ToBytes(newSize)
	sizeBytes, err := convert.ToPositiveInt64(sizeBytesStr)
	if err != nil {
		Logc(ctx).WithField("sizeInBytes", sizeBytes).WithError(err).Error("Invalid volume size.")
		return fmt.Errorf("invalid volume size: %v", err)
	}

	volumeInfo := &models.Volume{
		Size: convert.ToPtr(sizeBytes),
	}

	params.SetInfo(volumeInfo)

	volumeModifyOK, volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	} else if volumeModifyOK != nil {
		Logc(ctx).WithField("name", volumeName).Debug("Volume resized synchronously.")
		return nil
	} else if volumeModifyAccepted != nil {
		jobLink := getGenericJobLinkFromVolumeJobLink(volumeModifyAccepted.Payload)
		return c.PollJobStatus(ctx, jobLink)
	}
	return fmt.Errorf("unexpected response from volume modify")
}

// mountVolumeByNameAndStyle mounts a volume at the specified junction
func (c *RestClient) mountVolumeByNameAndStyle(ctx context.Context, volumeName, junctionPath, style string) error {
	fields := []string{"nas.path"}
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style, fields)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}
	if volume.UUID == nil {
		return fmt.Errorf("could not find volume uuid with name %v", volumeName)
	}

	if volume.Nas != nil && volume.Nas.Path != nil {
		if *volume.Nas.Path == junctionPath {
			Logc(ctx).Debug("already mounted to the correct junction path, nothing to do")
			return nil
		}
	}

	uuid := *volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = uuid

	volumeInfo := &models.Volume{
		Nas: &models.VolumeInlineNas{Path: convert.ToPtr(junctionPath)},
	}
	params.SetInfo(volumeInfo)

	_, volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	jobLink := getGenericJobLinkFromVolumeJobLink(volumeModifyAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

// unmountVolumeByNameAndStyle umounts a volume
func (c *RestClient) unmountVolumeByNameAndStyle(
	ctx context.Context,
	volumeName, style string,
) error {
	fields := []string{"nas.path"}
	volume, err := c.getVolumeInAnyStateByNameAndStyle(ctx, volumeName, style, fields)
	if err != nil {
		return err
	}

	if volume == nil {
		Logc(ctx).WithField("volume", volumeName).Warn("Volume does not exist.")
		return err
	}
	if volume.UUID == nil {
		Logc(ctx).WithField("volume", volumeName).Warn("Volume UUID does not exist.")
		return err
	}

	if volume.Nas != nil && volume.Nas.Path != nil {
		if *volume.Nas.Path == "" {
			Logc(ctx).Debug("already unmounted, nothing to do")
			return nil
		}
	}

	uuid := *volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = uuid

	volumeInfo := &models.Volume{
		Nas: &models.VolumeInlineNas{Path: convert.ToPtr("")},
	}
	params.SetInfo(volumeInfo)

	_, volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	jobLink := getGenericJobLinkFromVolumeJobLink(volumeModifyAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

// RenameVolumeByNameAndStyle changes the name of a FlexVol (but not a FlexGroup!)
func (c *RestClient) renameVolumeByNameAndStyle(ctx context.Context, volumeName, newVolumeName, style string) error {
	fields := []string{""}
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style, fields)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}
	if volume == nil {
		return fmt.Errorf("could not find volume uuid with name %v", volumeName)
	}

	uuid := *volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = uuid

	volumeInfo := &models.Volume{
		Name: convert.ToPtr(newVolumeName),
	}

	params.SetInfo(volumeInfo)

	_, volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	jobLink := getGenericJobLinkFromVolumeJobLink(volumeModifyAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

// destroyVolumeByNameAndStyle destroys a volume
func (c *RestClient) destroyVolumeByNameAndStyle(ctx context.Context, name, style string, force bool) error {
	fields := []string{""}
	volume, err := c.getVolumeByNameAndStyle(ctx, name, style, fields)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume: %v", name)
	}
	if volume.UUID == nil {
		return fmt.Errorf("could not find volume uuid: %v", name)
	}

	params := storage.NewVolumeDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = *volume.UUID
	params.Force = &force

	_, volumeDeleteAccepted, err := c.api.Storage.VolumeDelete(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeDeleteAccepted == nil {
		return fmt.Errorf("unexpected response from volume create")
	}

	jobLink := getGenericJobLinkFromVolumeJobLink(volumeDeleteAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

func (c *RestClient) modifyVolumeExportPolicyByNameAndStyle(
	ctx context.Context, volumeName, exportPolicyName, style string,
) error {
	fields := []string{""}
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style, fields)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}
	if volume.UUID == nil {
		return fmt.Errorf("could not find volume uuid with name %v", volumeName)
	}

	uuid := *volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = uuid

	exportPolicy := &models.VolumeInlineNasInlineExportPolicy{Name: convert.ToPtr(exportPolicyName)}
	nasInfo := &models.VolumeInlineNas{ExportPolicy: exportPolicy}
	volumeInfo := &models.Volume{Nas: nasInfo}
	params.SetInfo(volumeInfo)

	_, volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	jobLink := getGenericJobLinkFromVolumeJobLink(volumeModifyAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

func (c *RestClient) modifyVolumeUnixPermissionsByNameAndStyle(
	ctx context.Context,
	volumeName, unixPermissions, style string,
) error {
	fields := []string{""}
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style, fields)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}
	if volume.UUID == nil {
		return fmt.Errorf("could not find volume uuid with name %v", volumeName)
	}

	uuid := *volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = uuid

	// handle NAS options
	volumeNas := &models.VolumeInlineNas{}
	volumeInfo := &models.Volume{}

	if unixPermissions != "" {
		unixPermissions = convertUnixPermissions(unixPermissions)
		volumePermissions, parseErr := strconv.ParseInt(unixPermissions, 10, 64)
		if parseErr != nil {
			return fmt.Errorf("cannot process unix permissions value %v", unixPermissions)
		}
		volumeNas.UnixPermissions = convert.ToPtr(volumePermissions)
	}

	volumeInfo.Nas = volumeNas
	params.SetInfo(volumeInfo)

	_, volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	jobLink := getGenericJobLinkFromVolumeJobLink(volumeModifyAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

// setVolumeCommentByNameAndStyle sets a volume's comment to the supplied value
// equivalent to filer::> volume modify -vserver iscsi_vs -volume v -comment newVolumeComment
func (c *RestClient) setVolumeCommentByNameAndStyle(
	ctx context.Context,
	volumeName, newVolumeComment, style string,
) error {
	fields := []string{""}
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style, fields)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}
	if volume.UUID == nil {
		return fmt.Errorf("could not find volume uuid with name %v", volumeName)
	}

	uuid := *volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = uuid

	volumeInfo := &models.Volume{
		Comment: convert.ToPtr(newVolumeComment),
	}

	params.SetInfo(volumeInfo)

	_, volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	jobLink := getGenericJobLinkFromVolumeJobLink(volumeModifyAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
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
func (c *RestClient) setVolumeQosPolicyGroupNameByNameAndStyle(
	ctx context.Context, volumeName string, qosPolicyGroup QosPolicyGroup, style string,
) error {
	fields := []string{""}
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style, fields)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}
	if volume == nil {
		return fmt.Errorf("could not find volume uuid with name %v", volumeName)
	}

	uuid := *volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = uuid

	volumeInfo := &models.Volume{}
	if qosPolicyGroup.Kind != InvalidQosPolicyGroupKind {
		if qosPolicyGroup.Name != "" {
			volumeInfo.Qos = &models.VolumeInlineQos{
				Policy: &models.VolumeInlineQosInlinePolicy{Name: convert.ToPtr(qosPolicyGroup.Name)},
			}
		} else {
			return fmt.Errorf("missing QoS policy group name")
		}
	} else {
		return fmt.Errorf("invalid QoS policy group")
	}
	params.SetInfo(volumeInfo)

	_, volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	jobLink := getGenericJobLinkFromVolumeJobLink(volumeModifyAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

// startCloneSplitByNameAndStyle starts splitting the clone
func (c *RestClient) startCloneSplitByNameAndStyle(ctx context.Context, volumeName, style string) error {
	fields := []string{""}
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style, fields)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}
	if volume.UUID == nil {
		return fmt.Errorf("could not find volume uuid with name %v", volumeName)
	}

	uuid := *volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = uuid

	volumeInfo := &models.Volume{
		Clone: &models.VolumeInlineClone{SplitInitiated: convert.ToPtr(true)},
	}

	params.SetInfo(volumeInfo)

	volumeModifyOk, volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}

	if volumeModifyOk == nil && volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	// If there is explicit error, return the error
	if volumeModifyOk != nil && !volumeModifyOk.IsSuccess() {
		return fmt.Errorf("failed to start clone split; %v", volumeModifyOk.Error())
	}

	if volumeModifyAccepted != nil && !volumeModifyAccepted.IsSuccess() {
		return fmt.Errorf("failed to start clone split; %v", volumeModifyAccepted.Error())
	}

	// At this point, it is clear that no error occurred while trying to start the split.
	// Since clone split usually takes time, we do not wait for its completion.
	// Hence, do not get the jobLink and poll for its status. Assume, it is successful.
	Logc(ctx).WithField("volume", volumeName).Debug(
		"Clone split initiated successfully. This is an asynchronous operation, and its completion is not monitored.")
	return nil
}

// restoreSnapshotByNameAndStyle restores a volume to a snapshot as a non-blocking operation
func (c *RestClient) restoreSnapshotByNameAndStyle(
	ctx context.Context,
	snapshotName, volumeName, style string,
) error {
	fields := []string{""}
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, style, fields)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}
	if volume.UUID == nil {
		return fmt.Errorf("could not find volume uuid with name %v", volumeName)
	}

	uuid := *volume.UUID

	// restore
	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = uuid
	params.RestoreToSnapshotName = &snapshotName

	_, volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	jobLink := getGenericJobLinkFromVolumeJobLink(volumeModifyAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

func (c *RestClient) createCloneNAS(
	ctx context.Context,
	cloneName, sourceVolumeName, snapshotName string,
) (*storage.VolumeCreateAccepted, error) {
	params := storage.NewVolumeCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.ReturnRecords = convert.ToPtr(true)

	cloneInfo := &models.VolumeInlineClone{
		ParentVolume: &models.VolumeInlineCloneInlineParentVolume{
			Name: convert.ToPtr(sourceVolumeName),
		},
		IsFlexclone: convert.ToPtr(true),
	}
	if snapshotName != "" {
		cloneInfo.ParentSnapshot = &models.SnapshotReference{Name: convert.ToPtr(snapshotName)}
	}

	volumeInfo := &models.Volume{
		Name:  convert.ToPtr(cloneName),
		Clone: cloneInfo,
	}
	volumeInfo.Svm = &models.VolumeInlineSvm{UUID: convert.ToPtr(c.svmUUID)}

	params.SetInfo(volumeInfo)

	_, volumeCreateAccepted, err := c.api.Storage.VolumeCreate(params, c.authInfo)
	return volumeCreateAccepted, err
}

// listAllVolumeNamesBackedBySnapshot returns the names of all volumes backed by the specified snapshot
func (c *RestClient) listAllVolumeNamesBackedBySnapshot(ctx context.Context, volumeName, snapshotName string) (
	[]string, error,
) {
	params := storage.NewVolumeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SvmUUID = &c.svmUUID
	params.SetFields([]string{"name"})

	params.SetCloneParentVolumeName(convert.ToPtr(volumeName))
	params.SetCloneParentSnapshotName(convert.ToPtr(snapshotName))

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
	for _, vol := range result.Payload.VolumeResponseInlineRecords {
		if vol == nil || vol.Name == nil {
			continue
		}
		if vol.Clone != nil {
			if vol.Clone.ParentSnapshot != nil && vol.Clone.ParentSnapshot.Name != nil && *vol.Clone.ParentSnapshot.Name == snapshotName &&
				vol.Clone.ParentVolume != nil && *vol.Clone.ParentVolume.Name == volumeName {
				volumeNames = append(volumeNames, *vol.Name)
			}
		}
	}
	return volumeNames, nil
}

// createVolumeByStyle creates a volume with the specified options
// equivalent to filer::> volume create -vserver iscsi_vs -volume v -aggregate aggr1 -size 1g -state online -type RW
// -policy default -unix-permissions ---rwxr-xr-x -space-guarantee none -snapshot-policy none -security-style unix
// -encrypt false
func (c *RestClient) createVolumeByStyle(
	ctx context.Context, name string, sizeInBytes int64, aggrs []string,
	spaceReserve, snapshotPolicy, unixPermissions, exportPolicy, securityStyle, tieringPolicy, comment string,
	qosPolicyGroup QosPolicyGroup, encrypt *bool, snapshotReserve int, style string, dpVolume bool,
) error {
	params := storage.NewVolumeCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	volumeInfo := &models.Volume{
		Name:           convert.ToPtr(name),
		Size:           convert.ToPtr(sizeInBytes),
		Guarantee:      &models.VolumeInlineGuarantee{Type: convert.ToPtr(spaceReserve)},
		SnapshotPolicy: &models.VolumeInlineSnapshotPolicy{Name: convert.ToPtr(snapshotPolicy)},
		Comment:        convert.ToPtr(comment),
		State:          convert.ToPtr(models.VolumeStateOnline),
		Style:          convert.ToPtr(style),
	}

	volumeInfoAggregates := ToSliceVolumeAggregatesItems(aggrs)
	if len(volumeInfoAggregates) > 0 {
		volumeInfo.VolumeInlineAggregates = volumeInfoAggregates
	}

	if snapshotReserve != NumericalValueNotSet {
		volumeInfo.Space = &models.VolumeInlineSpace{
			Snapshot: &models.VolumeInlineSpaceInlineSnapshot{
				ReservePercent: convert.ToPtr(int64(snapshotReserve)),
			},
		}
	}

	volumeInfo.Svm = &models.VolumeInlineSvm{UUID: convert.ToPtr(c.svmUUID)}

	// For encrypt == nil - we don't explicitely set the encrypt argument.
	// If destination aggregate is NAE enabled, new volume will be aggregate encrypted
	// else it will be volume encrypted as per Ontap's default behaviour.
	if encrypt != nil {
		volumeInfo.Encryption = &models.VolumeInlineEncryption{Enabled: encrypt}
	}

	if qosPolicyGroup.Kind != InvalidQosPolicyGroupKind {
		if qosPolicyGroup.Name != "" {
			volumeInfo.Qos = &models.VolumeInlineQos{
				Policy: &models.VolumeInlineQosInlinePolicy{Name: convert.ToPtr(qosPolicyGroup.Name)},
			}
		}
	}
	if tieringPolicy != "" {
		volumeInfo.Tiering = &models.VolumeInlineTiering{Policy: convert.ToPtr(tieringPolicy)}
	}

	// handle NAS options
	volumeNas := &models.VolumeInlineNas{}
	if securityStyle != "" {
		volumeNas.SecurityStyle = convert.ToPtr(securityStyle)
		volumeInfo.Nas = volumeNas
	}
	if dpVolume {
		volumeInfo.Type = convert.ToPtr("DP")
	} else if unixPermissions != "" {
		unixPermissions = convertUnixPermissions(unixPermissions)
		volumePermissions, parseErr := strconv.ParseInt(unixPermissions, 10, 64)
		if parseErr != nil {
			return fmt.Errorf("cannot process unix permissions value %v", unixPermissions)
		}
		volumeNas.UnixPermissions = &volumePermissions
		volumeInfo.Nas = volumeNas
	}
	if exportPolicy != "" {
		volumeNas.ExportPolicy = &models.VolumeInlineNasInlineExportPolicy{Name: &exportPolicy}
		volumeInfo.Nas = volumeNas
	}

	params.SetInfo(volumeInfo)

	_, volumeCreateAccepted, err := c.api.Storage.VolumeCreate(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeCreateAccepted == nil {
		return fmt.Errorf("unexpected response from volume create")
	}

	// Sample ZAPI error: API status: failed, Reason: Size \"1GB\" (\"1073741824B\") is too small.Minimum size is \"400GB\" (\"429496729600B\").,Code: 13115
	// Sample REST error: API State: failure, Message: Size \"1GB\" (\"1073741824B\") is too small.Minimum size is \"400GB\" (\"429496729600B\").,Code: 917534

	jobLink := getGenericJobLinkFromVolumeJobLink(volumeCreateAccepted.Payload)
	if pollErr := c.PollJobStatus(ctx, jobLink); pollErr != nil {
		apiError, message, code := ExtractError(pollErr)
		if apiError == "failure" && code == FLEXGROUP_VOLUME_SIZE_ERROR_REST {
			return errors.InvalidInputError(message)
		}
		return pollErr
	}

	switch style {
	case models.VolumeStyleFlexgroup:
		return c.waitForFlexgroup(ctx, name)
	default:
		return c.waitForVolume(ctx, name)
	}
}

// waitForVolume polls for the ONTAP volume to exist, with backoff retry logic
func (c *RestClient) waitForVolume(ctx context.Context, volumeName string) error {
	checkStatus := func() error {
		exists, err := c.VolumeExists(ctx, volumeName)
		if !exists {
			return fmt.Errorf("volume '%v' does not exit, will continue checking", volumeName)
		}
		return err
	}
	statusNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Volume not found, waiting.")
	}
	statusBackoff := backoff.NewExponentialBackOff()
	statusBackoff.InitialInterval = 1 * time.Second
	statusBackoff.MaxInterval = 2 * time.Second
	statusBackoff.Multiplier = 1.414
	statusBackoff.RandomizationFactor = 0.1
	statusBackoff.MaxElapsedTime = 1 * time.Minute

	// Run the existence check using an exponential backoff
	if err := backoff.RetryNotify(checkStatus, statusBackoff, statusNotify); err != nil {
		Logc(ctx).WithField("name", volumeName).Warnf("Volume not found after %3.2f seconds.",
			statusBackoff.MaxElapsedTime.Seconds())
		return err
	}

	Logc(ctx).WithField("Name", volumeName).Debugf("Flexvol created after %3.2f seconds.",
		statusBackoff.GetElapsedTime().Seconds())

	return nil
}

// waitForFlexgroup polls for the ONTAP flexgroup to exist, with backoff retry logic
func (c *RestClient) waitForFlexgroup(ctx context.Context, volumeName string) error {
	checkStatus := func() error {
		exists, err := c.FlexGroupExists(ctx, volumeName)
		if !exists {
			return fmt.Errorf("FlexGroup '%v' does not exit, will continue checking", volumeName)
		}
		return err
	}
	statusNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("FlexGroup not found, waiting.")
	}
	statusBackoff := backoff.NewExponentialBackOff()
	statusBackoff.InitialInterval = 1 * time.Second
	statusBackoff.MaxInterval = 5 * time.Second
	statusBackoff.Multiplier = 2
	statusBackoff.RandomizationFactor = 0.1
	statusBackoff.MaxElapsedTime = 2 * time.Minute

	// Run the existence check using an exponential backoff
	if err := backoff.RetryNotify(checkStatus, statusBackoff, statusNotify); err != nil {
		Logc(ctx).WithField("name", volumeName).Warnf("FlexGroup not found after %3.2f seconds.",
			statusBackoff.MaxElapsedTime.Seconds())
		return err
	}

	Logc(ctx).WithField("Name", volumeName).Debugf("FlexGroup created after %3.2f seconds.",
		statusBackoff.GetElapsedTime().Seconds())

	return nil
}

// ////////////////////////////////////////////////////////////////////////////
// NAS VOLUME by style operations end
// ////////////////////////////////////////////////////////////////////////////

// //////////////////////////////////////////////////////////////////////////
// VOLUME operations
// ////////////////////////////////////////////////////////////////////////////

// VolumeList returns the names of all Flexvols whose names match the supplied pattern
func (c *RestClient) VolumeList(
	ctx context.Context, pattern string, fields []string,
) (*storage.VolumeCollectionGetOK, error) {
	return c.getAllVolumesByPatternStyleAndState(ctx, pattern, models.VolumeStyleFlexvol, models.VolumeStateOnline,
		fields)
}

// VolumeListByAttrs is used to find bucket volumes for nas-eco and san-eco
func (c *RestClient) VolumeListByAttrs(
	ctx context.Context, volumeAttrs *Volume, fields []string,
) (*storage.VolumeCollectionGetOK, error) {
	params := storage.NewVolumeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	// params.MaxRecords = utils.ToPtr(int64(1)) // use for testing, forces pagination

	params.SvmUUID = &c.svmUUID

	style := models.VolumeStyleFlexvol // or models.VolumeStyleFlexgroup ??
	state := models.VolumeStateOnline

	wildcard := convert.ToPtr("*")

	if volumeAttrs.Name != "" {
		params.SetName(convert.ToPtr(volumeAttrs.Name))
	} else {
		params.SetName(wildcard)
	}

	if len(volumeAttrs.Aggregates) > 0 {
		aggrs := strings.Join(volumeAttrs.Aggregates, "|")
		params.SetAggregatesName(convert.ToPtr(aggrs))
	} else {
		params.SetAggregatesName(wildcard)
	}

	if volumeAttrs.TieringPolicy != "" {
		params.SetTieringPolicy(convert.ToPtr(volumeAttrs.TieringPolicy))
	} else {
		params.SetTieringPolicy(wildcard)
	}

	if volumeAttrs.SnapshotPolicy != "" {
		params.SetSnapshotPolicyName(convert.ToPtr(volumeAttrs.SnapshotPolicy))
	} else {
		params.SetSnapshotPolicyName(wildcard)
	}

	if volumeAttrs.SpaceReserve != "" {
		params.SetGuaranteeType(convert.ToPtr(volumeAttrs.SpaceReserve))
	} else {
		params.SetGuaranteeType(wildcard)
	}

	params.SetSpaceSnapshotReservePercent(convert.ToPtr(int64(volumeAttrs.SnapshotReserve)))
	params.SetSnapshotDirectoryAccessEnabled(volumeAttrs.SnapshotDir)
	params.SetEncryptionEnabled(volumeAttrs.Encrypt)

	params.SetState(convert.ToPtr(state))
	params.SetStyle(convert.ToPtr(style))
	params.SetFields(fields)

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

	return result, nil
}

// VolumeCreate creates a volume with the specified options
// equivalent to filer::> volume create -vserver iscsi_vs -volume v -aggregate aggr1 -size 1g -state online -type RW
// -policy default -unix-permissions ---rwxr-xr-x -space-guarantee none -snapshot-policy none -security-style unix
// -encrypt false
func (c *RestClient) VolumeCreate(
	ctx context.Context,
	name, aggregateName, size, spaceReserve, snapshotPolicy, unixPermissions, exportPolicy, securityStyle, tieringPolicy, comment string,
	qosPolicyGroup QosPolicyGroup, encrypt *bool, snapshotReserve int, dpVolume bool,
) error {
	sizeBytesStr, _ := capacity.ToBytes(size)
	sizeInBytes, _ := strconv.ParseInt(sizeBytesStr, 10, 64)

	return c.createVolumeByStyle(ctx, name, sizeInBytes, []string{aggregateName}, spaceReserve, snapshotPolicy,
		unixPermissions, exportPolicy, securityStyle, tieringPolicy, comment, qosPolicyGroup, encrypt, snapshotReserve,
		models.VolumeStyleFlexvol, dpVolume)
}

// VolumeExists tests for the existence of a flexvol
func (c *RestClient) VolumeExists(ctx context.Context, volumeName string) (bool, error) {
	return c.checkVolumeExistsByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexvol)
}

// VolumeGetByName gets the flexvol with the specified name
func (c *RestClient) VolumeGetByName(ctx context.Context, volumeName string, fields []string) (*models.Volume, error) {
	return c.getVolumeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexvol, fields)
}

// VolumeMount mounts a flexvol at the specified junction
func (c *RestClient) VolumeMount(
	ctx context.Context,
	volumeName, junctionPath string,
) error {
	return c.mountVolumeByNameAndStyle(ctx, volumeName, junctionPath, models.VolumeStyleFlexvol)
}

// VolumeRename changes the name of a flexvol
func (c *RestClient) VolumeRename(
	ctx context.Context,
	volumeName, newVolumeName string,
) error {
	return c.renameVolumeByNameAndStyle(ctx, volumeName, newVolumeName, models.VolumeStyleFlexvol)
}

func (c *RestClient) VolumeModifyExportPolicy(
	ctx context.Context,
	volumeName, exportPolicyName string,
) error {
	return c.modifyVolumeExportPolicyByNameAndStyle(ctx, volumeName, exportPolicyName, models.VolumeStyleFlexvol)
}

// VolumeSize retrieves the size of the specified flexvol
func (c *RestClient) VolumeSize(
	ctx context.Context, volumeName string,
) (uint64, error) {
	return c.getVolumeSizeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexvol)
}

// VolumeUsedSize retrieves the used bytes of the specified volume
func (c *RestClient) VolumeUsedSize(ctx context.Context, volumeName string) (int, error) {
	return c.getVolumeUsedSizeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexvol)
}

// VolumeSetSize sets the size of the specified flexvol
func (c *RestClient) VolumeSetSize(ctx context.Context, volumeName, newSize string) error {
	return c.setVolumeSizeByNameAndStyle(ctx, volumeName, newSize, models.VolumeStyleFlexvol)
}

func (c *RestClient) VolumeModifyUnixPermissions(ctx context.Context, volumeName, unixPermissions string) error {
	return c.modifyVolumeUnixPermissionsByNameAndStyle(ctx, volumeName, unixPermissions, models.VolumeStyleFlexvol)
}

// VolumeSetComment sets a flexvol's comment to the supplied value
// equivalent to filer::> volume modify -vserver iscsi_vs -volume v -comment newVolumeComment
func (c *RestClient) VolumeSetComment(ctx context.Context, volumeName, newVolumeComment string) error {
	return c.setVolumeCommentByNameAndStyle(ctx, volumeName, newVolumeComment, models.VolumeStyleFlexvol)
}

// VolumeSetQosPolicyGroupName sets the QoS Policy Group for volume clones since
// we can't set adaptive policy groups directly during volume clone creation.
func (c *RestClient) VolumeSetQosPolicyGroupName(
	ctx context.Context, volumeName string, qosPolicyGroup QosPolicyGroup,
) error {
	return c.setVolumeQosPolicyGroupNameByNameAndStyle(ctx, volumeName, qosPolicyGroup, models.VolumeStyleFlexvol)
}

// VolumeCloneSplitStart starts splitting theflexvol clone
func (c *RestClient) VolumeCloneSplitStart(ctx context.Context, volumeName string) error {
	return c.startCloneSplitByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexvol)
}

// VolumeDestroy destroys a flexvol
func (c *RestClient) VolumeDestroy(ctx context.Context, name string, force bool) error {
	return c.destroyVolumeByNameAndStyle(ctx, name, models.VolumeStyleFlexvol, force)
}

// VolumeRecoveryQueuePurge uses the cli passthrough REST API to purge a volume from the recovery queue.
// the cli does not support passing in the UUID of the verver instead of the name. This will not work with
// a failed over metro cluster. This is a workaround until the FSxN API supports a mechanism like the force option to
// directly purge the volume from the recovery queue.
func (c *RestClient) VolumeRecoveryQueuePurge(ctx context.Context, recoveryQueueVolumeName string) error {
	fields := LogFields{
		"Method":     "VolumeRecoveryQueuePurge",
		"Type":       "ontap_rest",
		"volumeName": recoveryQueueVolumeName,
	}
	Logd(ctx, c.driverName, c.config.DebugTraceFlags["method"]).WithFields(fields).
		Trace(">>>> VolumeRecoveryQueuePurge")
	defer Logd(ctx, c.driverName, c.config.DebugTraceFlags["method"]).WithFields(fields).
		Trace("<<<< VolumeRecoveryQueuePurge")

	requestUrl := fmt.Sprintf("https://%s/api/private/cli/volume/recovery-queue/purge", c.config.ManagementLIF)

	requestContent := map[string]string{
		"vserver": c.svmName,
		"volume":  recoveryQueueVolumeName,
	}
	requestBodyBytes, err := json.Marshal(requestContent)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, requestUrl, bytes.NewReader(requestBodyBytes))
	if err != nil {
		return err
	}

	response, err := c.sendPassThroughCliCommand(ctx, request)
	if err != nil {
		return err
	}

	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusAccepted {

		var responseBodyBytes []byte
		if response.Body != nil {
			responseBodyBytes, _ = io.ReadAll(response.Body)
		}

		Logc(ctx).WithField("statusCode", response.StatusCode).Error(
			"failed to purge volume from recovery queue: %s", string(responseBodyBytes))
		return fmt.Errorf("unexpected response status code: %v", response.StatusCode)
	}

	return nil
}

func (c *RestClient) VolumeRecoveryQueueGetName(ctx context.Context, name string) (string, error) {
	fields := LogFields{
		"Method":     "VolumeRecoveryQueueGetName",
		"Type":       "ontap_rest",
		"volumeName": name,
	}
	Logd(ctx, c.driverName, c.config.DebugTraceFlags["method"]).WithFields(fields).
		Trace(">>>> VolumeRecoveryQueueGetName")
	defer Logd(ctx, c.driverName, c.config.DebugTraceFlags["method"]).WithFields(fields).
		Trace("<<<< VolumeRecoveryQueueGetName")

	requestUrl := fmt.Sprintf("https://%s/api/private/cli/volume/recovery-queue", c.config.ManagementLIF)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestUrl, nil)
	if err != nil {
		return "", err
	}

	q := request.URL.Query()
	q.Add("vserver", c.svmName)
	q.Add("volume", fmt.Sprintf("%s*", name))
	request.URL.RawQuery = q.Encode()

	response, err := c.sendPassThroughCliCommand(ctx, request)
	if err != nil {
		return "", err
	}

	defer func() { _ = response.Body.Close() }()

	var responseBodyBytes []byte
	if response.Body != nil {
		if responseBodyBytes, err = io.ReadAll(response.Body); err != nil {
			return "", fmt.Errorf("failed to read response body: %v", err)
		}
	}

	if response.StatusCode != http.StatusOK {
		Logc(ctx).WithField("statusCode", response.StatusCode).Error(
			"failed to get recovery queue volume name: %s", string(responseBodyBytes))
		return "", fmt.Errorf("unexpected response status code: %v", response.StatusCode)
	}

	type recoveryQueueVolumeListResponse struct {
		Records []struct {
			Volume string
		}
	}

	responseObject := recoveryQueueVolumeListResponse{}
	if err = json.Unmarshal(responseBodyBytes, &responseObject); err != nil {
		return "", fmt.Errorf("failed to unmarshal response body: %v", err)
	}

	if len(responseObject.Records) == 0 {
		return "", errors.NotFoundError("unable to find volume '%s' in the recovery queue", name)
	}

	if len(responseObject.Records) > 1 {
		return "", fmt.Errorf("found multiple volumes matching '%s' in the recovery queue", name)
	}

	return responseObject.Records[0].Volume, nil
}

func (c *RestClient) sendPassThroughCliCommand(ctx context.Context, request *http.Request) (*http.Response, error) {
	request.Header.Set("Content-Type", "application/json")

	if c.config.Username != "" && c.config.Password != "" {
		request.SetBasicAuth(c.config.Username, c.config.Password)
	}

	httpClient := &http.Client{
		Transport: c.tr,
		Timeout:   tridentconfig.StorageAPITimeoutSeconds * time.Second,
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}

	responseBytes, _ := httputil.DumpResponse(response, true)
	Logd(ctx, c.driverName, c.config.DebugTraceFlags["api"]).Debugf("%s\n", string(responseBytes))

	return response, nil
}

// ////////////////////////////////////////////////////////////////////////////
// Consistency Group operations
// ////////////////////////////////////////////////////////////////////////////

// ConsistencyGroupCreateAndWait creates a CG and waits on the job to complete
func (c *RestClient) ConsistencyGroupCreateAndWait(ctx context.Context, cgName string, flexVols []string) error {
	CGCreateResult, err := c.ConsistencyGroupCreate(ctx, cgName, flexVols)
	if err != nil {
		return fmt.Errorf("could not create consistency group; %v", err)
	}
	if CGCreateResult == nil {
		return fmt.Errorf("could not create consistency group: %v", "unexpected result")
	}

	jobLink := getGenericJobLinkFromCGJobLink(CGCreateResult.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

// ConsistencyGroupCreate creates a consistency group
func (c *RestClient) ConsistencyGroupCreate(
	ctx context.Context, cgName string, flexVols []string,
) (*application.ConsistencyGroupCreateAccepted, error) {
	params := application.NewConsistencyGroupCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	volumes := make([]*models.ConsistencyGroupInlineVolumesInlineArrayItem, 0)
	for _, vol := range flexVols {
		v := &models.ConsistencyGroupInlineVolumesInlineArrayItem{
			Name: convert.ToPtr(vol),
		}
		v.ProvisioningOptions = &models.ConsistencyGroupInlineVolumesInlineArrayItemInlineProvisioningOptions{
			Action: convert.ToPtr(models.
				ConsistencyGroupInlineVolumesInlineArrayItemInlineProvisioningOptionsActionAdd),
		}

		volumes = append(volumes, v)
	}

	cg := &models.ConsistencyGroup{
		Name:                          convert.ToPtr(cgName),
		ConsistencyGroupInlineVolumes: volumes,
		Svm:                           &models.ConsistencyGroupInlineSvm{UUID: convert.ToPtr(c.svmUUID)},
	}

	params.SetInfo(cg)

	_, cgCreateAccepted, err := c.api.Application.ConsistencyGroupCreate(params, c.authInfo)
	return cgCreateAccepted, err
}

// ConsistencyGroupGet returns the consistency group info
func (c *RestClient) ConsistencyGroupGet(ctx context.Context, cgName string) (*models.ConsistencyGroupResponseInlineRecordsInlineArrayItem, error) {
	params := application.NewConsistencyGroupCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.Name = convert.ToPtr(cgName)
	params.SvmUUID = convert.ToPtr(c.svmUUID)

	// result, err := c.api.Application.ConsistencyGroupGet(params, c.authInfo)
	result, err := c.api.Application.ConsistencyGroupCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Payload == nil || result.Payload.NumRecords == nil || *result.Payload.NumRecords == 0 {
		return nil, err
	}
	if *result.Payload.NumRecords == 1 && result.Payload.ConsistencyGroupResponseInlineRecords != nil {
		return result.Payload.ConsistencyGroupResponseInlineRecords[0], nil
	}

	return nil, fmt.Errorf("could not find unique consistency group with name '%v'; found %d matching groups",
		cgName, result.Payload.NumRecords)
}

// ConsistencyGroupDelete deletes the consistency group
func (c *RestClient) ConsistencyGroupDelete(ctx context.Context, cgName string) error {
	params := application.NewConsistencyGroupDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	cg, err := c.ConsistencyGroupGet(ctx, cgName)
	if err != nil {
		return err
	}
	if cg == nil || cg.UUID == nil {
		return fmt.Errorf("consistency group UUID is empty")
	}
	params.SetUUID(*cg.UUID)

	cgDeleteOK, cgDeleteAccepted, err := c.api.Application.ConsistencyGroupDelete(params, c.authInfo)
	if err != nil {
		// may need to check for code "53411842" for CG DNE
		return err
	}
	if cgDeleteOK != nil {
		if cgDeleteOK.IsSuccess() {
			return nil
		}
	}
	if cgDeleteAccepted == nil {
		return fmt.Errorf("unexpected response from consistency group delete")
	}

	jobLink := getGenericJobLinkFromCGJobLink(cgDeleteAccepted.Payload)

	return c.PollJobStatus(ctx, jobLink)
}

// ConsistencyGroupSnapshotAndWait creates a CG snapshot and waits on the job to complete
func (c *RestClient) ConsistencyGroupSnapshotAndWait(ctx context.Context, cgName, snapName string) error {
	CGSnapshotCreated, CGSnapshotAccepted, err := c.ConsistencyGroupSnapshot(ctx, cgName, snapName)
	if err != nil {
		return fmt.Errorf("could not create consistency group snapshot; %v", err)
	}
	if CGSnapshotCreated != nil {
		return nil
	}
	if CGSnapshotAccepted == nil {
		return fmt.Errorf("could not create consistency group snapshot: %v", "unexpected result")
	}

	jobLink := getGenericJobLinkFromCGSnapshotJobLink(CGSnapshotAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

// ConsistencyGroupSnapshot creates a snapshot of the consistency group
func (c *RestClient) ConsistencyGroupSnapshot(ctx context.Context, cgName, snapName string) (
	*application.ConsistencyGroupSnapshotCreateCreated, *application.ConsistencyGroupSnapshotCreateAccepted, error,
) {
	params := application.NewConsistencyGroupSnapshotCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	cg, err := c.ConsistencyGroupGet(ctx, cgName)
	if err != nil {
		return nil, nil, err
	}
	if cg.UUID == nil {
		return nil, nil, fmt.Errorf("consistency group UUID is empty")
	}
	params.SetConsistencyGroupUUID(*cg.UUID)

	snapshotInfo := &models.ConsistencyGroupSnapshot{
		Name: convert.ToPtr(snapName),
	}

	params.SetInfo(snapshotInfo)

	return c.api.Application.ConsistencyGroupSnapshotCreate(params, c.authInfo)
}

// ////////////////////////////////////////////////////////////////////////////
// SNAPSHOT operations
// ////////////////////////////////////////////////////////////////////////////

// SnapshotCreate creates a snapshot
func (c *RestClient) SnapshotCreate(
	ctx context.Context, volumeUUID, snapshotName string,
) (*storage.SnapshotCreateAccepted, error) {
	params := storage.NewSnapshotCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.VolumeUUID = volumeUUID

	snapshotInfo := &models.Snapshot{
		Name: convert.ToPtr(snapshotName),
	}

	snapshotInfo.Svm = &models.SnapshotInlineSvm{UUID: convert.ToPtr(c.svmUUID)}

	params.SetInfo(snapshotInfo)

	_, snapshotCreateAccepted, err := c.api.Storage.SnapshotCreate(params, c.authInfo)
	return snapshotCreateAccepted, err
}

// SnapshotCreateAndWait creates a snapshot and waits on the job to complete
func (c *RestClient) SnapshotCreateAndWait(ctx context.Context, volumeUUID, snapshotName string) error {
	snapshotCreateResult, err := c.SnapshotCreate(ctx, volumeUUID, snapshotName)
	if err != nil {
		return fmt.Errorf("could not create snapshot; %v", err)
	}
	if snapshotCreateResult == nil {
		return fmt.Errorf("could not create snapshot: %v", "unexpected result")
	}

	jobLink := getGenericJobLinkFromSnapshotJobLink(snapshotCreateResult.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

// SnapshotList lists snapshots
func (c *RestClient) SnapshotList(ctx context.Context, volumeUUID string) (*storage.SnapshotCollectionGetOK, error) {
	params := storage.NewSnapshotCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.VolumeUUID = volumeUUID

	params.SvmUUID = convert.ToPtr(c.svmUUID)
	params.SetFields([]string{"name", "create_time"})

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
			if resultNext == nil || resultNext.Payload == nil || resultNext.Payload.NumRecords == nil {
				done = true
				continue NextLoop
			}

			if result.Payload.NumRecords == nil {
				result.Payload.NumRecords = convert.ToPtr(int64(0))
			}
			result.Payload.NumRecords = convert.ToPtr(*result.Payload.NumRecords + *resultNext.Payload.NumRecords)
			result.Payload.SnapshotResponseInlineRecords = append(result.Payload.SnapshotResponseInlineRecords,
				resultNext.Payload.SnapshotResponseInlineRecords...)

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
func (c *RestClient) SnapshotListByName(ctx context.Context, volumeUUID, snapshotName string) (
	*storage.SnapshotCollectionGetOK, error,
) {
	params := storage.NewSnapshotCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.VolumeUUID = volumeUUID
	params.Name = convert.ToPtr(snapshotName)

	params.SvmUUID = convert.ToPtr(c.svmUUID)
	params.SetFields([]string{"name", "create_time"})

	return c.api.Storage.SnapshotCollectionGet(params, c.authInfo)
}

// SnapshotGet returns info on the snapshot
func (c *RestClient) SnapshotGet(ctx context.Context, volumeUUID, snapshotUUID string) (*storage.SnapshotGetOK, error) {
	params := storage.NewSnapshotGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.VolumeUUID = volumeUUID
	params.UUID = snapshotUUID

	return c.api.Storage.SnapshotGet(params, c.authInfo)
}

// SnapshotGetByName finds the snapshot by name
func (c *RestClient) SnapshotGetByName(ctx context.Context, volumeUUID, snapshotName string) (*models.Snapshot, error) {
	result, err := c.SnapshotListByName(ctx, volumeUUID, snapshotName)
	if result != nil && result.Payload != nil && result.Payload.NumRecords != nil && *result.Payload.NumRecords == 1 &&
		result.Payload.SnapshotResponseInlineRecords != nil {
		return result.Payload.SnapshotResponseInlineRecords[0], nil
	}
	return nil, err
}

// SnapshotDelete deletes a snapshot
func (c *RestClient) SnapshotDelete(
	ctx context.Context,
	volumeUUID, snapshotUUID string,
) (*storage.SnapshotDeleteAccepted, error) {
	params := storage.NewSnapshotDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.VolumeUUID = volumeUUID
	params.UUID = snapshotUUID

	_, snapshotDeleteAccepted, err := c.api.Storage.SnapshotDelete(params, c.authInfo)
	return snapshotDeleteAccepted, err
}

// SnapshotRestoreVolume restores a volume to a snapshot as a non-blocking operation
func (c *RestClient) SnapshotRestoreVolume(ctx context.Context, snapshotName, volumeName string) error {
	return c.restoreSnapshotByNameAndStyle(ctx, snapshotName, volumeName, models.VolumeStyleFlexvol)
}

// SnapshotRestoreFlexgroup restores a volume to a snapshot as a non-blocking operation
func (c *RestClient) SnapshotRestoreFlexgroup(ctx context.Context, snapshotName, volumeName string) error {
	return c.restoreSnapshotByNameAndStyle(ctx, snapshotName, volumeName, models.VolumeStyleFlexgroup)
}

// VolumeModifySnapshotDirectoryAccess modifies access to the ".snapshot" directory
func (c *RestClient) VolumeModifySnapshotDirectoryAccess(ctx context.Context, volumeName string, enable bool) error {
	fields := []string{""}
	volume, err := c.getVolumeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexvol, fields)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}
	if volume.UUID == nil {
		return fmt.Errorf("could not find volume uuid with name %v", volumeName)
	}

	uuid := *volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = uuid

	volumeInfo := &models.Volume{}
	volumeInfo.SnapshotDirectoryAccessEnabled = convert.ToPtr(enable)
	params.SetInfo(volumeInfo)

	_, volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	jobLink := getGenericJobLinkFromVolumeJobLink(volumeModifyAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

// VolumeListAllBackedBySnapshot returns the names of all FlexVols backed by the specified snapshot
func (c *RestClient) VolumeListAllBackedBySnapshot(ctx context.Context, volumeName, snapshotName string) ([]string,
	error,
) {
	return c.listAllVolumeNamesBackedBySnapshot(ctx, volumeName, snapshotName)
}

// ////////////////////////////////////////////////////////////////////////////
// CLONE operations
// ////////////////////////////////////////////////////////////////////////////

// VolumeCloneCreate creates a clone
// see also: https://library.netapp.com/ecmdocs/ECMLP2858435/html/resources/volume.html#creating-a-flexclone-and-specifying-its-properties-using-post
func (c *RestClient) VolumeCloneCreate(ctx context.Context, cloneName, sourceVolumeName, snapshotName string) (
	*storage.VolumeCreateAccepted, error,
) {
	return c.createCloneNAS(ctx, cloneName, sourceVolumeName, snapshotName)
}

// VolumeCloneCreateAsync clones a volume from a snapshot
func (c *RestClient) VolumeCloneCreateAsync(ctx context.Context, cloneName, sourceVolumeName, snapshot string) error {
	cloneCreateResult, err := c.createCloneNAS(ctx, cloneName, sourceVolumeName, snapshot)
	if err != nil {
		return fmt.Errorf("could not create clone; %v", err)
	}
	if cloneCreateResult == nil {
		return fmt.Errorf("could not create clone: %v", "unexpected result")
	}

	// NOTE the callers of this function should perform their own existence checks based on type (vol or flexgroup)
	jobLink := getGenericJobLinkFromVolumeJobLink(cloneCreateResult.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

// ///////////////////////////////////////////////////////////////////////////
// iSCSI initiator operations
// ///////////////////////////////////////////////////////////////////////////

// IscsiInitiatorGetDefaultAuth returns the authorization details for the default initiator
// equivalent to filer::> vserver iscsi security show -vserver SVM -initiator-name default
func (c *RestClient) IscsiInitiatorGetDefaultAuth(
	ctx context.Context, fields []string,
) (*san.IscsiCredentialsCollectionGetOK, error) {
	params := san.NewIscsiCredentialsCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.ReturnRecords = convert.ToPtr(true)

	// params.MaxRecords = utils.ToPtr(int64(1)) // use for testing, forces pagination

	params.SvmUUID = convert.ToPtr(c.svmUUID)
	params.Initiator = convert.ToPtr("default")

	params.SetFields(fields)
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
func (c *RestClient) IscsiInterfaceGet(ctx context.Context, fields []string) (*san.IscsiServiceCollectionGetOK,
	error,
) {
	params := san.NewIscsiServiceCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.ReturnRecords = convert.ToPtr(true)
	params.SvmUUID = convert.ToPtr(c.svmUUID)

	params.SetFields(fields)

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
func (c *RestClient) IscsiInitiatorSetDefaultAuth(
	ctx context.Context, authType, userName, passphrase,
	outbountUserName, outboundPassphrase string,
) error {
	fields := []string{"initiator"}
	getDefaultAuthResponse, err := c.IscsiInitiatorGetDefaultAuth(ctx, fields)
	if err != nil {
		return err
	}
	if getDefaultAuthResponse == nil {
		return fmt.Errorf("could not get the default iscsi initiator")
	}
	if getDefaultAuthResponse.Payload == nil {
		return fmt.Errorf("could not get the default iscsi initiator")
	}
	if getDefaultAuthResponse.Payload.NumRecords == nil {
		return fmt.Errorf("could not get the default iscsi initiator")
	}
	if *getDefaultAuthResponse.Payload.NumRecords != 1 {
		return fmt.Errorf("should only be one default iscsi initiator")
	}
	if getDefaultAuthResponse.Payload.IscsiCredentialsResponseInlineRecords[0] == nil {
		return fmt.Errorf("could not get the default iscsi initiator")
	}
	if getDefaultAuthResponse.Payload.IscsiCredentialsResponseInlineRecords[0].Initiator == nil {
		return fmt.Errorf("could not get the default iscsi initiator")
	}

	params := san.NewIscsiCredentialsModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SvmUUID = c.svmUUID
	params.Initiator = *getDefaultAuthResponse.Payload.IscsiCredentialsResponseInlineRecords[0].Initiator

	outboundInfo := &models.IscsiCredentialsInlineChapInlineOutbound{}
	if outbountUserName != "" && outboundPassphrase != "" {
		outboundInfo.Password = convert.ToPtr(outboundPassphrase)
		outboundInfo.User = convert.ToPtr(outbountUserName)
	}
	inboundInfo := &models.IscsiCredentialsInlineChapInlineInbound{
		Password: convert.ToPtr(passphrase),
		User:     convert.ToPtr(userName),
	}
	chapInfo := &models.IscsiCredentialsInlineChap{
		Inbound:  inboundInfo,
		Outbound: outboundInfo,
	}
	authInfo := &models.IscsiCredentials{
		AuthenticationType: convert.ToPtr(authType),
		Chap:               chapInfo,
	}

	params.SetInfo(authInfo)

	_, err = c.api.San.IscsiCredentialsModify(params, c.authInfo)

	return err
}

// IscsiNodeGetName returns information about the vserver's iSCSI node name
func (c *RestClient) IscsiNodeGetName(ctx context.Context, fields []string) (*san.IscsiServiceGetOK,
	error,
) {
	svmResult, err := c.SvmGet(ctx, c.svmUUID)
	if err != nil {
		return nil, err
	}
	if svmResult == nil || svmResult.Payload == nil || svmResult.Payload.UUID == nil {
		return nil, fmt.Errorf("could not find SVM %s (%s)", c.svmName, c.svmUUID)
	}

	svmInfo := svmResult.Payload

	params := san.NewIscsiServiceGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SvmUUID = *svmInfo.UUID

	params.SetFields(fields)

	result, err := c.api.San.IscsiServiceGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	return result, nil
}

// FcpNodeGetName returns information about the vserver's FCP target name
//
//	Equivalent to filer::> vserver fcp show -vserver svm1
func (c *RestClient) FcpNodeGetName(ctx context.Context, fields []string) (*san.FcpServiceGetOK,
	error,
) {
	svmResult, err := c.SvmGet(ctx, c.svmUUID)
	if err != nil {
		return nil, err
	}
	if svmResult == nil || svmResult.Payload == nil || svmResult.Payload.UUID == nil {
		return nil, fmt.Errorf("could not find SVM %s (%s)", c.svmName, c.svmUUID)
	}

	svmInfo := svmResult.Payload

	params := san.NewFcpServiceGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SvmUUID = *svmInfo.UUID

	params.SetFields(fields)

	result, err := c.api.San.FcpServiceGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	return result, nil
}

// FcpInterfaceGet returns information about the vserver's  FCP interfaces
func (c *RestClient) FcpInterfaceGet(ctx context.Context, fields []string) (*san.FcpServiceCollectionGetOK,
	error,
) {
	params := san.NewFcpServiceCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.ReturnRecords = convert.ToPtr(true)
	params.SvmUUID = convert.ToPtr(c.svmUUID)

	params.SetFields(fields)

	result, err := c.api.San.FcpServiceCollectionGet(params, c.authInfo)
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
func (c *RestClient) IgroupCreate(ctx context.Context, initiatorGroupName, initiatorGroupType, osType string) error {
	params := san.NewIgroupCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.ReturnRecords = convert.ToPtr(true)

	igroupInfo := &models.Igroup{
		Name:     convert.ToPtr(initiatorGroupName),
		Protocol: convert.ToPtr(initiatorGroupType),
		OsType:   convert.ToPtr(osType),
	}

	igroupInfo.Svm = &models.IgroupInlineSvm{UUID: convert.ToPtr(c.svmUUID)}

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
	} else if igroupCreateAccepted.Payload.NumRecords == nil {
		return fmt.Errorf("unexpected response from igroup create, payload numRecords was nil")
	} else {
		numRecords := *igroupCreateAccepted.Payload.NumRecords
		if numRecords != 1 {
			return fmt.Errorf("unexpected response from igroup create, created %v igroups", numRecords)
		}
	}

	return nil
}

// IgroupAdd adds an initiator to an initiator group
// equivalent to filer::> lun igroup add -vserver iscsi_vs -igroup docker -initiator iqn.1993-08.org.
// debian:01:9031309bbebd
func (c *RestClient) IgroupAdd(ctx context.Context, initiatorGroupName, initiator string) error {
	fields := []string{""}
	igroup, err := c.IgroupGetByName(ctx, initiatorGroupName, fields)
	if err != nil {
		return err
	}
	if igroup == nil {
		return fmt.Errorf("unexpected response from igroup lookup, igroup was nil")
	}
	if igroup.UUID == nil {
		return fmt.Errorf("unexpected response from igroup lookup, igroup uuid was nil")
	}
	igroupUUID := *igroup.UUID

	params := san.NewIgroupInitiatorCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.IgroupUUID = igroupUUID

	igroupInitiator := &models.IgroupInitiator{
		Name: convert.ToPtr(initiator),
	}

	params.SetInfo(igroupInitiator)

	_, err = c.api.San.IgroupInitiatorCreate(params, c.authInfo)
	if err != nil {
		return err
	}

	return nil
}

// IgroupRemove removes an initiator from an initiator group
func (c *RestClient) IgroupRemove(ctx context.Context, initiatorGroupName, initiator string) error {
	fields := []string{""}
	igroup, err := c.IgroupGetByName(ctx, initiatorGroupName, fields)
	if err != nil {
		return err
	}
	if igroup == nil {
		return fmt.Errorf("unexpected response from igroup lookup, igroup was nil")
	}
	if igroup.UUID == nil {
		return fmt.Errorf("unexpected response from igroup lookup, igroup uuid was nil")
	}
	igroupUUID := *igroup.UUID

	params := san.NewIgroupInitiatorDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.IgroupUUID = igroupUUID
	params.Name = initiator

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
func (c *RestClient) IgroupDestroy(ctx context.Context, initiatorGroupName string) error {
	fields := []string{""}
	igroup, err := c.IgroupGetByName(ctx, initiatorGroupName, fields)
	if err != nil {
		return err
	}
	if igroup == nil {
		// Initiator group not found. Log a message and return nil.
		Logc(ctx).WithField("igroup", initiatorGroupName).Debug("No such initiator group (igroup).")
		return nil
	}
	if igroup.UUID == nil {
		// Initiator group not found. Log a message and return nil.
		Logc(ctx).WithField("igroup", initiatorGroupName).Debug("Initiator group is missing its UUID.")
		return nil
	}
	igroupUUID := *igroup.UUID

	params := san.NewIgroupDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = igroupUUID

	lunDeleteResult, err := c.api.San.IgroupDelete(params, c.authInfo)
	if err != nil {
		return fmt.Errorf("could not delete igroup; %v", err)
	}
	if lunDeleteResult == nil {
		return fmt.Errorf("could not delete igroup: %v", "unexpected result")
	}

	return nil
}

// IgroupList lists initiator groups
func (c *RestClient) IgroupList(ctx context.Context, pattern string, fields []string) (*san.IgroupCollectionGetOK,
	error,
) {
	if pattern == "" {
		pattern = "*"
	}

	params := san.NewIgroupCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SvmUUID = convert.ToPtr(c.svmUUID)
	params.SetName(convert.ToPtr(pattern))
	params.SetFields(fields)

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
			if resultNext == nil || resultNext.Payload == nil || resultNext.Payload.NumRecords == nil {
				done = true
				continue NextLoop
			}

			if result.Payload.NumRecords == nil {
				result.Payload.NumRecords = convert.ToPtr(int64(0))
			}
			result.Payload.NumRecords = convert.ToPtr(*result.Payload.NumRecords + *resultNext.Payload.NumRecords)
			result.Payload.IgroupResponseInlineRecords = append(result.Payload.IgroupResponseInlineRecords,
				resultNext.Payload.IgroupResponseInlineRecords...)

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
func (c *RestClient) IgroupGet(ctx context.Context, uuid string) (*san.IgroupGetOK, error) {
	params := san.NewIgroupGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = uuid

	return c.api.San.IgroupGet(params, c.authInfo)
}

// IgroupGetByName gets the igroup with the specified name
func (c *RestClient) IgroupGetByName(ctx context.Context, initiatorGroupName string, fields []string) (*models.Igroup,
	error,
) {
	result, err := c.IgroupList(ctx, initiatorGroupName, fields)
	if err != nil {
		return nil, err
	}
	if result != nil && result.Payload != nil && result.Payload.NumRecords != nil && *result.Payload.NumRecords == 1 && result.Payload.IgroupResponseInlineRecords != nil {
		return result.Payload.IgroupResponseInlineRecords[0], nil
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
func (c *RestClient) LunOptions(
	ctx context.Context,
) (*LunOptionsResult, error) {
	url := fmt.Sprintf(
		`https://%v/api/v1/storage/luns?return_schema=POST&fields=space.size`,
		c.config.ManagementLIF,
	)

	Logc(ctx).WithFields(LogFields{
		"url": url,
	}).Debug("LunOptions request")

	req, _ := http.NewRequestWithContext(ctx, "OPTIONS", url, nil)
	req.Header.Set("Content-Type", "application/json")
	if c.config.Username != "" && c.config.Password != "" {
		req.SetBasicAuth(c.config.Username, c.config.Password)
	}

	// certs will have been parsed and configured already, if needed, as part of the RestClient init
	tr := c.tr

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

	body, err := io.ReadAll(response.Body)
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

// LunCloneCreate creates a LUN clone
func (c *RestClient) LunCloneCreate(
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
	params.ReturnRecords = convert.ToPtr(true)

	// sizeInBytes isn't required for LUN cloning from a snapshot.
	// If the value is not positive, ignore it.
	inlineSpace := &models.LunInlineSpace{}
	if sizeInBytes > 0 {
		inlineSpace = &models.LunInlineSpace{
			Size: convert.ToPtr(sizeInBytes),
		}
	}

	inlineQoSPolicy := &models.LunInlineQosPolicy{}
	if qosPolicyGroup.Name != "" || qosPolicyGroup.Kind != 0 {
		inlineQoSPolicy = &models.LunInlineQosPolicy{
			Name: convert.ToPtr(qosPolicyGroup.Name),
		}
	}

	lunInfo := &models.Lun{
		Clone: &models.LunInlineClone{
			Source: &models.LunInlineCloneInlineSource{
				Name: convert.ToPtr(sourcePath),
			},
		},
		Name: convert.ToPtr(lunPath), // example:  /vol/myVolume/myLun1
		// OsType is not supported for POST when creating a LUN clone
		Space:     inlineSpace,
		QosPolicy: inlineQoSPolicy,
	}
	lunInfo.Svm = &models.LunInlineSvm{UUID: convert.ToPtr(c.svmUUID)}

	params.SetInfo(lunInfo)

	return c.lunCreate(ctx, params, "failed to create lun clone")
}

// LunCreate creates a LUN
func (c *RestClient) LunCreate(
	ctx context.Context, lunPath string, sizeInBytes int64, osType string, qosPolicyGroup QosPolicyGroup,
	spaceReserved, spaceAllocated *bool,
) error {
	if strings.Contains(lunPath, failureLUNCreate) {
		return errors.New("injected error")
	}

	params := san.NewLunCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.ReturnRecords = convert.ToPtr(true)

	lunInfo := &models.Lun{
		Name:   convert.ToPtr(lunPath), // example:  /vol/myVolume/myLun1
		OsType: convert.ToPtr(osType),
		Space: &models.LunInlineSpace{
			Size: convert.ToPtr(sizeInBytes),
		},
	}

	// Set spaceReserved if present.
	if spaceReserved != nil {
		lunInfo.Space.Guarantee = &models.LunInlineSpaceInlineGuarantee{
			Requested: spaceReserved,
		}
	}

	// Set spaceAllocated if present.
	if spaceAllocated != nil {
		lunInfo.Space.ScsiThinProvisioningSupportEnabled = spaceAllocated
	}

	// Set QosPolicy name if present.
	if qosPolicyGroup.Name != "" {
		lunInfo.QosPolicy = &models.LunInlineQosPolicy{
			Name: convert.ToPtr(qosPolicyGroup.Name),
		}
	}

	lunInfo.Svm = &models.LunInlineSvm{UUID: convert.ToPtr(c.svmUUID)}

	params.SetInfo(lunInfo)

	return c.lunCreate(ctx, params, "failed to create lun")
}

func (c *RestClient) lunCreate(ctx context.Context, params *san.LunCreateParams, customErrMsg string) error {
	lunCreateOk, lunCreateAccepted, err := c.api.San.LunCreate(params, c.authInfo)
	if err != nil {
		return err
	}

	// If both response parameter is nil, then it is unexpected.
	if lunCreateOk == nil && lunCreateAccepted == nil {
		return fmt.Errorf("unexpected response from LUN create")
	}

	// Check for synchronous response and return status based on that
	if lunCreateOk != nil {
		if lunCreateOk.IsSuccess() {
			return nil
		}
		return fmt.Errorf("%s: %v", customErrMsg, lunCreateOk.Error())
	}

	// Else, wait for job to finish
	jobLink := getGenericJobLinkFromLunJobLink(lunCreateAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

// LunGet gets the LUN with the specified uuid
func (c *RestClient) LunGet(ctx context.Context, uuid string) (*san.LunGetOK, error) {
	params := san.NewLunGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = uuid

	return c.api.San.LunGet(params, c.authInfo)
}

// LunGetByName gets the LUN with the specified name
func (c *RestClient) LunGetByName(ctx context.Context, name string, fields []string) (*models.Lun, error) {
	result, err := c.LunList(ctx, name, fields)
	if err != nil {
		return nil, err
	}

	if result == nil || result.Payload == nil || result.Payload.NumRecords == nil || *result.Payload.NumRecords == 0 {
		return nil, errors.NotFoundError("could not get LUN by name %v", name)
	}

	if *result.Payload.NumRecords == 1 && result.Payload.LunResponseInlineRecords != nil {
		return result.Payload.LunResponseInlineRecords[0], nil
	}
	return nil, err
}

// LunList finds LUNs with the specified pattern
func (c *RestClient) LunList(ctx context.Context, pattern string, fields []string) (*san.LunCollectionGetOK, error) {
	params := san.NewLunCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SvmUUID = convert.ToPtr(c.svmUUID)
	params.SetName(convert.ToPtr(pattern))

	params.SetFields(fields)

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
			if resultNext == nil || resultNext.Payload == nil || resultNext.Payload.NumRecords == nil {
				done = true
				continue NextLoop
			}

			if result.Payload.NumRecords == nil {
				result.Payload.NumRecords = convert.ToPtr(int64(0))
			}
			result.Payload.NumRecords = convert.ToPtr(*result.Payload.NumRecords + *resultNext.Payload.NumRecords)
			result.Payload.LunResponseInlineRecords = append(result.Payload.LunResponseInlineRecords,
				resultNext.Payload.LunResponseInlineRecords...)

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
func (c *RestClient) LunDelete(
	ctx context.Context,
	lunUUID string,
) error {
	params := san.NewLunDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = lunUUID

	lunDeleteResult, lunDeleteAccepted, err := c.api.San.LunDelete(params, c.authInfo)
	if err != nil {
		return fmt.Errorf("could not delete lun; %v", err)
	}

	// If both of the response parameters are nil, then it is unexpected.
	if lunDeleteResult == nil && lunDeleteAccepted == nil {
		return fmt.Errorf("could not delete lun: %v", "unexpected result")
	}

	return nil
}

// LunGetComment gets the comment for a given LUN.
func (c *RestClient) LunGetComment(
	ctx context.Context,
	lunPath string,
) (string, error) {
	fields := []string{"comment"}
	lun, err := c.LunGetByName(ctx, lunPath, fields)
	if err != nil {
		return "", err
	}
	if lun == nil {
		return "", fmt.Errorf("could not find LUN with name %v", lunPath)
	}
	if lun.Comment == nil {
		return "", fmt.Errorf("LUN did not have a comment")
	}

	return *lun.Comment, nil
}

// LunSetComment sets the comment for a given LUN.
func (c *RestClient) LunSetComment(
	ctx context.Context,
	lunPath, comment string,
) error {
	fields := []string{""}
	lun, err := c.LunGetByName(ctx, lunPath, fields)
	if err != nil {
		return err
	}
	if lun == nil {
		return fmt.Errorf("could not find LUN with name %v", lunPath)
	}
	if lun.UUID == nil {
		return fmt.Errorf("could not find LUN UUID with name %v", lunPath)
	}

	uuid := *lun.UUID

	params := san.NewLunModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = uuid

	lunInfo := &models.Lun{
		Comment: &comment,
	}

	params.SetInfo(lunInfo)

	lunModifyOK, lunModifyAccepted, err := c.api.San.LunModify(params, c.authInfo)
	if err != nil {
		return err
	}

	// If both of the response parameters are nil, then it is unexpected.
	if lunModifyOK == nil && lunModifyAccepted == nil {
		return fmt.Errorf("unexpected response from LUN modify")
	}

	return nil
}

// LunGetAttribute gets an attribute by name for a given LUN.
func (c *RestClient) LunGetAttribute(
	ctx context.Context,
	lunPath, attributeName string,
) (string, error) {
	fields := []string{"attributes.name", "attributes.value"}
	lun, err := c.LunGetByName(ctx, lunPath, fields)
	if err != nil {
		return "", err
	}
	if lun == nil {
		return "", fmt.Errorf("could not find LUN with name %v", lunPath)
	}
	if lun.LunInlineAttributes == nil {
		return "", fmt.Errorf("LUN did not have any attributes")
	}
	for _, attr := range lun.LunInlineAttributes {
		if attr.Name != nil && attributeName == *attr.Name {
			if attr.Value != nil {
				return *attr.Value, nil
			}
		}
	}

	// LUN has no value for the specified attribute
	return "", nil
}

// LunSetAttribute sets the attribute to the provided value for a given LUN.
func (c *RestClient) LunSetAttribute(
	ctx context.Context,
	lunPath, attributeName, attributeValue string,
) error {
	fields := []string{"attributes.name"}
	lun, err := c.LunGetByName(ctx, lunPath, fields)
	if err != nil {
		return err
	}
	if lun == nil {
		return fmt.Errorf("could not find LUN with name %v", lunPath)
	}
	if lun.UUID == nil {
		return fmt.Errorf("could not find LUN UUID with name %v", lunPath)
	}

	uuid := *lun.UUID

	attributeExists := false
	for _, attrs := range lun.LunInlineAttributes {
		if attrs.Name != nil && attributeName == *attrs.Name {
			attributeExists = true
		}
	}

	if !attributeExists {

		params := san.NewLunAttributeCreateParamsWithTimeout(c.httpClient.Timeout)
		params.Context = ctx
		params.HTTPClient = c.httpClient
		params.LunUUID = uuid

		attrInfo := &models.LunAttribute{
			// in a create, the attribute name is specified here
			Name:  convert.ToPtr(attributeName),
			Value: convert.ToPtr(attributeValue),
		}
		params.Info = attrInfo

		lunAttrCreateOK, lunAttrCreateAccepted, err := c.api.San.LunAttributeCreate(params, c.authInfo)
		if err != nil {
			return err
		}

		// If both the response parameters are nil, then it is unexpected.
		if lunAttrCreateOK == nil && lunAttrCreateAccepted == nil {
			return fmt.Errorf("unexpected response from LUN attribute create")
		}
		return nil

	} else {

		params := san.NewLunAttributeModifyParamsWithTimeout(c.httpClient.Timeout)
		params.Context = ctx
		params.HTTPClient = c.httpClient
		params.LunUUID = uuid
		params.Name = attributeName

		attrInfo := &models.LunAttribute{
			// in a modify, the attribute name is specified in the path as params.NamePathParameter
			Value: convert.ToPtr(attributeValue),
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

// LunSetQosPolicyGroup sets the QoS policy for a given LUN.
func (c *RestClient) LunSetQosPolicyGroup(
	ctx context.Context,
	lunPath, qosPolicyGroup string,
) error {
	fields := []string{""}
	lun, err := c.LunGetByName(ctx, lunPath, fields)
	if err != nil {
		return err
	}
	if lun == nil {
		return fmt.Errorf("could not find LUN with name %v", lunPath)
	}
	if lun.UUID == nil {
		return fmt.Errorf("could not find LUN uuid with name %v", lunPath)
	}

	uuid := *lun.UUID

	params := san.NewLunModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = uuid

	qosPolicy := &models.LunInlineQosPolicy{
		Name: convert.ToPtr(qosPolicyGroup),
	}
	lunInfo := &models.Lun{
		QosPolicy: qosPolicy,
	}

	params.SetInfo(lunInfo)

	lunModifyOK, lunModifyAccepted, err := c.api.San.LunModify(params, c.authInfo)
	if err != nil {
		return err
	}

	// If both the response parameters are nil, then it is unexpected.
	if lunModifyOK == nil && lunModifyAccepted == nil {
		return fmt.Errorf("unexpected response from LUN modify")
	}

	return nil
}

// LunRename changes the name of a LUN
func (c *RestClient) LunRename(
	ctx context.Context,
	lunPath, newLunPath string,
) error {
	fields := []string{""}
	lun, err := c.LunGetByName(ctx, lunPath, fields)
	if err != nil {
		return err
	}
	if lun == nil {
		return fmt.Errorf("could not find LUN with name %v", lunPath)
	}
	if lun.UUID == nil {
		return fmt.Errorf("could not find LUN uuid with name %v", lunPath)
	}

	uuid := *lun.UUID

	params := san.NewLunModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = uuid

	lunInfo := &models.Lun{
		Name: convert.ToPtr(newLunPath),
	}

	params.SetInfo(lunInfo)

	lunModifyOK, lunModifyAccepted, err := c.api.San.LunModify(params, c.authInfo)
	if err != nil {
		return err
	}

	// If both the response parameters are nil, then it is unexpected.
	if lunModifyOK == nil && lunModifyAccepted == nil {
		return fmt.Errorf("unexpected response from LUN modify")
	}

	// If the API is async, then poll and wait for the job to complete
	if lunModifyAccepted != nil {
		jobLink := getGenericJobLinkFromLunJobLink(lunModifyAccepted.Payload)
		return c.PollJobStatus(ctx, jobLink)
	}

	return nil
}

// LunMapInfo gets the LUN maping information for the specified LUN
func (c *RestClient) LunMapInfo(
	ctx context.Context,
	initiatorGroupName, lunPath string,
) (*san.LunMapCollectionGetOK, error) {
	params := san.NewLunMapCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.LunName = &lunPath
	if initiatorGroupName != "" {
		params.IgroupName = &initiatorGroupName
	}
	params.Fields = []string{"svm", "lun", "igroup", "logical-unit-number"}

	return c.api.San.LunMapCollectionGet(params, c.authInfo)
}

// LunUnmap deletes the lun mapping for the given LUN path and igroup
// equivalent to filer::> lun mapping delete -vserver iscsi_vs -path /vol/v/lun0 -igroup group
func (c *RestClient) LunUnmap(
	ctx context.Context,
	initiatorGroupName, lunPath string,
) error {
	lunMapResponse, err := c.LunMapInfo(ctx, initiatorGroupName, lunPath)
	if err != nil {
		return fmt.Errorf("problem reading maps for LUN %s: %v", lunPath, err)
	} else if lunMapResponse.Payload == nil || lunMapResponse.Payload.NumRecords == nil {
		return fmt.Errorf("problem reading maps for LUN %s", lunPath)
	} else if *lunMapResponse.Payload.NumRecords == 0 {
		return nil
	}

	if lunMapResponse.Payload == nil ||
		lunMapResponse.Payload.LunMapResponseInlineRecords == nil ||
		lunMapResponse.Payload.LunMapResponseInlineRecords[0] == nil ||
		lunMapResponse.Payload.LunMapResponseInlineRecords[0].Igroup == nil ||
		lunMapResponse.Payload.LunMapResponseInlineRecords[0].Igroup.UUID == nil {
		return fmt.Errorf("problem reading maps for LUN %s", lunPath)
	}

	igroupUUID := *lunMapResponse.Payload.LunMapResponseInlineRecords[0].Igroup.UUID

	fields := []string{""}
	lun, err := c.LunGetByName(ctx, lunPath, fields)
	if err != nil {
		return err
	}
	if lun == nil {
		return fmt.Errorf("could not find LUN with name %v", lunPath)
	}
	if lun.UUID == nil {
		return fmt.Errorf("could not find LUN uuid with name %v", lunPath)
	}
	lunUUID := *lun.UUID

	params := san.NewLunMapDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.IgroupUUID = igroupUUID
	params.LunUUID = lunUUID

	_, err = c.api.San.LunMapDelete(params, c.authInfo)
	if err != nil {
		return err
	}
	return nil
}

// LunMap maps a LUN to an id in an initiator group
// equivalent to filer::> lun map -vserver iscsi_vs -path /vol/v/lun1 -igroup docker -lun-id 0
func (c *RestClient) LunMap(
	ctx context.Context,
	initiatorGroupName, lunPath string,
	lunID int,
) (*san.LunMapCreateCreated, error) {
	fields := []string{""}
	lun, err := c.LunGetByName(ctx, lunPath, fields)
	if err != nil {
		return nil, err
	}
	if lun == nil {
		return nil, fmt.Errorf("could not find LUN with name %v", lunPath)
	}
	uuid := lun.UUID

	params := san.NewLunMapCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.ReturnRecords = convert.ToPtr(true)

	igroupInfo := &models.LunMapInlineIgroup{
		Name: convert.ToPtr(initiatorGroupName),
	}
	lunInfo := &models.LunMapInlineLun{
		Name: convert.ToPtr(lunPath),
		UUID: uuid,
	}
	lunSVM := &models.LunMapInlineSvm{
		UUID: convert.ToPtr(c.svmUUID),
	}
	lunMapInfo := &models.LunMap{
		Igroup: igroupInfo,
		Lun:    lunInfo,
		Svm:    lunSVM,
	}
	if lunID != -1 {
		lunMapInfo.LogicalUnitNumber = convert.ToPtr(int64(lunID))
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
func (c *RestClient) LunMapList(
	ctx context.Context,
	initiatorGroupName, lunPath string,
	fields []string,
) (*san.LunMapCollectionGetOK, error) {
	params := san.NewLunMapCollectionGetParams()
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SetFields(fields)
	params.SetIgroupName(convert.ToPtr(initiatorGroupName))
	params.SetLunName(convert.ToPtr(lunPath))

	return c.api.San.LunMapCollectionGet(params, c.authInfo)
}

// LunMapGetReportingNodes
// equivalent to filer::> lun mapping show -vserver iscsi_vs -path /vol/v/lun0 -igroup trident
func (c *RestClient) LunMapGetReportingNodes(
	ctx context.Context,
	initiatorGroupName, lunPath string,
) ([]string, error) {
	lunFields := []string{""}
	lun, lunGetErr := c.LunGetByName(ctx, lunPath, lunFields)
	if lunGetErr != nil {
		return nil, lunGetErr
	}
	if lun == nil {
		return nil, fmt.Errorf("could not find LUN with name %v", lunPath)
	}
	if lun.UUID == nil {
		return nil, fmt.Errorf("could not find LUN uuid with name %v", lunPath)
	}
	lunUUID := *lun.UUID

	igroupFields := []string{""}
	igroup, igroupGetErr := c.IgroupGetByName(ctx, initiatorGroupName, igroupFields)
	if igroupGetErr != nil {
		return nil, igroupGetErr
	}
	if igroup == nil {
		return nil, fmt.Errorf("could not find igroup with name %v", initiatorGroupName)
	}
	if igroup.UUID == nil {
		return nil, fmt.Errorf("could not find igroup uuid with name %v", initiatorGroupName)
	}
	igroupUUID := *igroup.UUID

	params := san.NewLunMapReportingNodeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SetLunUUID(lunUUID)
	params.SetIgroupUUID(igroupUUID)

	result, err := c.api.San.LunMapReportingNodeCollectionGet(params, c.authInfo)
	if err != nil {
		// use reflection to access any underlying REST error response and check the code
		errorResponse, extractErr := ExtractErrorResponse(ctx, err)
		if extractErr != nil {
			return nil, err
		}
		if errorResponse != nil && errorResponse.Error != nil {
			errorCode := errorResponse.Error.Code
			if errorCode != nil && *errorCode == LUN_MAP_EXIST_ERROR {
				// the specified LUN map does not exist
				return []string{}, nil
			}
		}
		return nil, err
	}

	names := []string{}
	for _, records := range result.Payload.LunMapReportingNodeResponseInlineRecords {
		if records.Name != nil {
			names = append(names, *records.Name)
		}
	}
	return names, nil
}

// LunSize gets the size for a given LUN.
func (c *RestClient) LunSize(
	ctx context.Context,
	lunPath string,
) (int, error) {
	fields := []string{"space.size"}
	lun, err := c.LunGetByName(ctx, lunPath, fields)
	if err != nil {
		return 0, err
	}
	if lun == nil {
		return 0, errors.NotFoundError("could not find LUN with name %v", lunPath)
	}
	if lun.Space == nil {
		return 0, fmt.Errorf("could not find LUN space with name %v", lunPath)
	}
	if lun.Space.Size == nil {
		return 0, fmt.Errorf("could not find LUN size with name %v", lunPath)
	}

	// TODO validate/improve this logic? int64 vs int
	return int(*lun.Space.Size), nil
}

// LunSetSize sets the size for a given LUN.
func (c *RestClient) LunSetSize(
	ctx context.Context,
	lunPath, newSize string,
) (uint64, error) {
	fields := []string{""}
	lun, err := c.LunGetByName(ctx, lunPath, fields)
	if err != nil {
		return 0, err
	}
	if lun == nil {
		return 0, fmt.Errorf("could not find LUN with name %v", lunPath)
	}
	if lun.UUID == nil {
		return 0, fmt.Errorf("could not find LUN uuid with name %v", lunPath)
	}

	uuid := *lun.UUID

	params := san.NewLunModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = uuid

	sizeBytesStr, _ := capacity.ToBytes(newSize)
	sizeBytes, err := convert.ToPositiveInt64(sizeBytesStr)
	if err != nil {
		Logc(ctx).WithField("sizeInBytes", sizeBytes).WithError(err).Error("Invalid volume size.")
		return 0, fmt.Errorf("invalid volume size: %v", err)
	}

	spaceInfo := &models.LunInlineSpace{
		Size: convert.ToPtr(sizeBytes),
	}
	lunInfo := &models.Lun{
		Space: spaceInfo,
	}

	params.SetInfo(lunInfo)

	lunModifyOK, lunModifyAccepted, err := c.api.San.LunModify(params, c.authInfo)
	if err != nil {
		return 0, err
	}

	// If both the response parameters are nil, then it is unexpected.
	if lunModifyOK == nil && lunModifyAccepted == nil {
		return 0, fmt.Errorf("unexpected response from LUN modify")
	}

	return uint64(sizeBytes), nil
}

// ////////////////////////////////////////////////////////////////////////////
// NETWORK operations
// ////////////////////////////////////////////////////////////////////////////

// NetworkIPInterfacesList lists all IP interfaces
func (c *RestClient) NetworkIPInterfacesList(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error) {
	params := networking.NewNetworkIPInterfacesGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SvmUUID = convert.ToPtr(c.svmUUID)

	fields := []string{"location.node.name", "ip.address"}
	params.SetFields(fields)

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
			if resultNext == nil || resultNext.Payload == nil || resultNext.Payload.NumRecords == nil {
				done = true
				continue NextLoop
			}

			if result.Payload.NumRecords == nil {
				result.Payload.NumRecords = convert.ToPtr(int64(0))
			}
			result.Payload.NumRecords = convert.ToPtr(*result.Payload.NumRecords + *resultNext.Payload.NumRecords)
			result.Payload.IPInterfaceResponseInlineRecords = append(result.Payload.IPInterfaceResponseInlineRecords,
				resultNext.Payload.IPInterfaceResponseInlineRecords...)

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

func (c *RestClient) NetInterfaceGetDataLIFs(ctx context.Context, protocol string) ([]string, error) {
	if protocol == "" {
		return nil, fmt.Errorf("missing protocol specification")
	}

	params := networking.NewNetworkIPInterfacesGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.Services = convert.ToPtr(fmt.Sprintf("data_%v", protocol))
	params.SvmUUID = convert.ToPtr(c.svmUUID)
	fields := []string{"ip.address", "state"}
	params.SetFields(fields)

	lifResponse, err := c.api.Networking.NetworkIPInterfacesGet(params, c.authInfo)
	if err != nil {
		return nil, fmt.Errorf("error checking network interfaces; %v", err)
	}
	if lifResponse == nil {
		return nil, fmt.Errorf("unexpected error checking network interfaces")
	}

	dataLIFs := make([]string, 0)
	for _, record := range lifResponse.Payload.IPInterfaceResponseInlineRecords {
		if record.IP != nil && record.State != nil && *record.State == models.IPInterfaceStateUp {
			if record.IP.Address != nil {
				dataLIFs = append(dataLIFs, string(*record.IP.Address))
			}
		}
	}

	Logc(ctx).WithField("dataLIFs", dataLIFs).Debug("Data LIFs")
	return dataLIFs, nil
}

func (c *RestClient) NetFcpInterfaceGetDataLIFs(ctx context.Context, protocol string) ([]string, error) {
	if protocol == "" {
		return nil, fmt.Errorf("missing protocol specification")
	}

	params := networking.NewFcInterfaceCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SvmUUID = convert.ToPtr(c.svmUUID)
	fields := []string{"wwpn", "state"}
	params.SetFields(fields)

	lifResponse, err := c.api.Networking.FcInterfaceCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, fmt.Errorf("error checking network interfaces; %v", err)
	}
	if lifResponse == nil {
		return nil, fmt.Errorf("unexpected error checking network interfaces")
	}

	dataLIFs := make([]string, 0)
	for _, record := range lifResponse.Payload.FcInterfaceResponseInlineRecords {
		if record.Wwpn != nil && record.State != nil && *record.State == models.IPInterfaceStateUp {
			if record.Wwpn != nil {
				dataLIFs = append(dataLIFs, *record.Wwpn)
			}
		}
	}

	Logc(ctx).WithField("dataLIFs", dataLIFs).Debug("Data LIFs")
	return dataLIFs, nil
}

// ////////////////////////////////////////////////////////////////////////////
// JOB operations
// ////////////////////////////////////////////////////////////////////////////

// JobGet returns the job by ID
func (c *RestClient) JobGet(ctx context.Context, jobUUID string, fields []string) (*cluster.JobGetOK, error) {
	params := cluster.NewJobGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = jobUUID

	params.SetFields(fields)

	return c.api.Cluster.JobGet(params, c.authInfo)
}

// IsJobFinished lookus up the supplied JobLinkResponse's UUID to see if it's reached a terminal state
func (c *RestClient) IsJobFinished(ctx context.Context, payload *models.JobLinkResponse) (bool, error) {
	if payload == nil {
		return false, fmt.Errorf("payload is nil")
	}

	if payload.Job == nil {
		return false, fmt.Errorf("payload's Job is nil")
	}
	if payload.Job.UUID == nil {
		return false, fmt.Errorf("payload's Job uuid is nil")
	}

	job := payload.Job
	jobUUID := job.UUID

	Logc(ctx).WithFields(LogFields{
		"payload": payload,
		"job":     payload.Job,
		"jobUUID": jobUUID,
	}).Debug("IsJobFinished")

	fields := []string{"state"}
	jobResult, err := c.JobGet(ctx, string(*jobUUID), fields)
	if err != nil {
		return false, err
	}

	jobState := jobResult.Payload.State
	if jobState == nil {
		return false, fmt.Errorf("unexpected nil job state ")
	}

	switch *jobState {
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
func (c *RestClient) PollJobStatus(ctx context.Context, payload *models.JobLinkResponse) error {
	job := payload.Job
	if job == nil {
		return fmt.Errorf("missing job result")
	}
	if job.UUID == nil {
		return fmt.Errorf("missing job uuid for result")
	}
	jobUUID := *job.UUID

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
	jobStatusBackoff.MaxInterval = 2 * time.Second
	jobStatusBackoff.Multiplier = 1.414
	jobStatusBackoff.RandomizationFactor = 0.1
	jobStatusBackoff.MaxElapsedTime = 2 * time.Minute

	// Run the job status check using an exponential backoff
	if err := backoff.RetryNotify(checkJobStatus, jobStatusBackoff, jobStatusNotify); err != nil {
		Logc(ctx).WithField("UUID", jobUUID).Warnf("Job not completed after %3.2f seconds.",
			jobStatusBackoff.MaxElapsedTime.Seconds())
		return err
	}

	Logc(ctx).WithField("UUID", jobUUID).Debugf("Job completed after %3.2f seconds.",
		jobStatusBackoff.GetElapsedTime().Seconds())

	fields := []string{"**"} // we need to get all available fields
	jobResult, err := c.JobGet(ctx, string(jobUUID), fields)
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

	jobState := jobResult.Payload.State
	if jobState == nil {
		return fmt.Errorf("unexpected nil job state ")
	}

	switch *jobState {
	case models.JobStateSuccess:
		return nil
	case models.JobStateFailure:
		return NewRestErrorFromPayload(jobResult.Payload)
	default:
		return fmt.Errorf("unexpected job state %v", jobState)
	}
}

func getGenericJobLinkFromVolumeJobLink(volJobLink *models.VolumeJobLinkResponse) *models.JobLinkResponse {
	jobLink := &models.JobLinkResponse{}
	if volJobLink != nil {
		jobLink.Job = volJobLink.Job
	}
	return jobLink
}

func getGenericJobLinkFromSnapshotJobLink(snapshotJobLink *models.SnapshotJobLinkResponse) *models.JobLinkResponse {
	jobLink := &models.JobLinkResponse{}
	if snapshotJobLink != nil {
		jobLink.Job = snapshotJobLink.Job
	}
	return jobLink
}

func getGenericJobLinkFromQtreeJobLink(qtreeJobLink *models.QtreeJobLinkResponse) *models.JobLinkResponse {
	jobLink := &models.JobLinkResponse{}
	if qtreeJobLink != nil {
		jobLink.Job = qtreeJobLink.Job
	}
	return jobLink
}

func getGenericJobLinkFromQuotaRuleJobLink(quotaRuleJobLink *models.QuotaRuleJobLinkResponse) *models.JobLinkResponse {
	jobLink := &models.JobLinkResponse{}
	if quotaRuleJobLink != nil {
		jobLink.Job = quotaRuleJobLink.Job
	}
	return jobLink
}

func getGenericJobLinkFromSMRJobLink(smrJobLink *models.SnapmirrorRelationshipJobLinkResponse) *models.JobLinkResponse {
	jobLink := &models.JobLinkResponse{}
	if smrJobLink != nil {
		jobLink.Job = smrJobLink.Job
	}
	return jobLink
}

func getGenericJobLinkFromLunJobLink(lunJobLink *models.LunJobLinkResponse) *models.JobLinkResponse {
	jobLink := &models.JobLinkResponse{}
	if lunJobLink != nil {
		jobLink.Job = lunJobLink.Job
	}
	return jobLink
}

func getGenericJobLinkFromStorageUnitJobLink(suJobLink *models.StorageUnitJobLinkResponse) *models.JobLinkResponse {
	jobLink := &models.JobLinkResponse{}
	if suJobLink != nil {
		jobLink.Job = suJobLink.Job
	}
	return jobLink
}

func getGenericJobLinkFromStorageUnitSnapshotJobLink(suSnapshotJobLink *models.StorageUnitSnapshotJobLinkResponse) *models.JobLinkResponse {
	jobLink := &models.JobLinkResponse{}
	if suSnapshotJobLink != nil {
		jobLink.Job = suSnapshotJobLink.Job
	}
	return jobLink
}

func getGenericJobLinkFromCGJobLink(cgJobLink *models.ConsistencyGroupJobLinkResponse) *models.JobLinkResponse {
	jobLink := &models.JobLinkResponse{}
	if cgJobLink != nil {
		jobLink.Job = cgJobLink.Job
	}
	return jobLink
}

func getGenericJobLinkFromCGSnapshotJobLink(cgJobLink *models.ConsistencyGroupSnapshotJobLinkResponse) *models.JobLinkResponse {
	jobLink := &models.JobLinkResponse{}
	if cgJobLink != nil {
		jobLink.Job = cgJobLink.Job
	}
	return jobLink
}

func getGenericJobLinkFromNvmeNamespaceJobLink(nvmeNamespaceJobLink *models.NvmeNamespaceJobLinkResponse) *models.JobLinkResponse {
	jobLink := &models.JobLinkResponse{}
	if nvmeNamespaceJobLink != nil {
		jobLink.Job = nvmeNamespaceJobLink.Job
	}
	return jobLink
}

// ////////////////////////////////////////////////////////////////////////////
// Aggregrate operations
// ////////////////////////////////////////////////////////////////////////////

// AggregateList returns the names of all Aggregates whose names match the supplied pattern
func (c *RestClient) AggregateList(
	ctx context.Context, pattern string, fields []string,
) (*storage.AggregateCollectionGetOK, error) {
	params := storage.NewAggregateCollectionGetParamsWithTimeout(c.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SetName(convert.ToPtr(pattern))
	params.SetFields(fields)

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
			if resultNext == nil || resultNext.Payload == nil || resultNext.Payload.NumRecords == nil {
				done = true
				continue NextLoop
			}

			if result.Payload.NumRecords == nil {
				result.Payload.NumRecords = convert.ToPtr(int64(0))
			}
			result.Payload.NumRecords = convert.ToPtr(*result.Payload.NumRecords + *resultNext.Payload.NumRecords)
			result.Payload.AggregateResponseInlineRecords = append(result.Payload.AggregateResponseInlineRecords,
				resultNext.Payload.AggregateResponseInlineRecords...)

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
func (c *RestClient) SvmGet(ctx context.Context, uuid string) (*svm.SvmGetOK, error) {
	params := svm.NewSvmGetParamsWithTimeout(c.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = uuid

	return c.api.Svm.SvmGet(params, c.authInfo)
}

// SvmList returns the names of all SVMs whose names match the supplied pattern
func (c *RestClient) SvmList(ctx context.Context, pattern string) (*svm.SvmCollectionGetOK, error) {
	params := svm.NewSvmCollectionGetParamsWithTimeout(c.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SetName(convert.ToPtr(pattern))
	fields := []string{""}
	params.SetFields(fields)

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
			if resultNext == nil || resultNext.Payload == nil || resultNext.Payload.NumRecords == nil {
				done = true
				continue NextLoop
			}

			if result.Payload.NumRecords == nil {
				result.Payload.NumRecords = convert.ToPtr(int64(0))
			}
			result.Payload.NumRecords = convert.ToPtr(*result.Payload.NumRecords + *resultNext.Payload.NumRecords)
			result.Payload.SvmResponseInlineRecords = append(result.Payload.SvmResponseInlineRecords,
				resultNext.Payload.SvmResponseInlineRecords...)

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
func ExtractErrorResponse(ctx context.Context, restError interface{}) (errorResponse *models.ErrorResponse,
	errorOut error,
) {
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

func ExtractError(errResponse error) (string, string, string) {
	// Split the error response to get API state, message and code
	err := strings.Split(errResponse.Error(), ",")
	var apiState, message, code string

	// Size error will be of below format
	// Sample ZAPI error: API status: failed, Reason: Size \"1GB\" (\"1073741824B\") is too small.Minimum size is \"400GB\" (\"429496729600B\").,Code: 13115
	// Sample REST error: API State: failure, Message: Size \"1GB\" (\"1073741824B\") is too small.Minimum size is \"400GB\" (\"429496729600B\").,Code: 917534
	if len(err) == 3 {
		apiState = strings.TrimSpace(strings.Split(err[0], ":")[1])
		message = strings.TrimSpace(strings.Split(err[1], ":")[1])
		code = strings.TrimSpace(strings.Split(err[2], ":")[1])
	}

	return apiState, message, code
}

func getType(i interface{}) string {
	if t := reflect.TypeOf(i); t.Kind() == reflect.Ptr {
		return "*" + t.Elem().Name()
	} else {
		return t.Name()
	}
}

// SvmGetByName gets the SVM with the specified name
func (c *RestClient) SvmGetByName(ctx context.Context, svmName string) (*models.Svm, error) {
	result, err := c.SvmList(ctx, svmName)
	if err != nil {
		return nil, err
	}

	if validationErr := ValidatePayloadExists(ctx, result); validationErr != nil {
		return nil, validationErr
	}

	if result != nil && result.Payload != nil && result.Payload.NumRecords != nil && *result.Payload.NumRecords == 1 && result.Payload.SvmResponseInlineRecords != nil {
		return result.Payload.SvmResponseInlineRecords[0], nil
	}
	return nil, fmt.Errorf("unexpected result")
}

func (c *RestClient) SVMGetAggregateNames(
	ctx context.Context,
) ([]string, error) {
	result, err := c.SvmGet(ctx, c.svmUUID)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Payload == nil {
		return nil, fmt.Errorf("could not find SVM %s (%s)", c.svmName, c.svmUUID)
	}

	svmInfo := result.Payload
	aggrNames := make([]string, 0, 10)
	for _, aggr := range svmInfo.SvmInlineAggregates {
		if aggr != nil && aggr.Name != nil {
			aggrNames = append(aggrNames, string(*aggr.Name))
		}
	}

	return aggrNames, nil
}

// ////////////////////////////////////////////////////////////////////////////
// Misc operations
// ////////////////////////////////////////////////////////////////////////////

// ClusterInfo returns information about the cluster
func (c *RestClient) ClusterInfo(
	ctx context.Context, fields []string, ignoreUnknownFields bool,
) (*cluster.ClusterGetOK, error) {
	params := cluster.NewClusterGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	if len(fields) == 0 {
		fields = []string{"**"}
	}
	params.SetFields(fields)

	if ignoreUnknownFields {
		// Create a custom ClientOption to add the ignore_unknown_fields query parameter
		addQueryParam := func(op *runtime.ClientOperation) {
			op.Params = &QueryWrapper{
				originalParams: op.Params,
				queryParams:    map[string]string{"ignore_unknown_fields": "true"},
			}
		}
		return c.api.Cluster.ClusterGet(params, c.authInfo, addQueryParam)
	} else {
		return c.api.Cluster.ClusterGet(params, c.authInfo)
	}
}

func (c *RestClient) SetOntapVersion(version string) {
	c.m.Lock()
	defer c.m.Unlock()
	c.OntapVersion = version
}

func (c *RestClient) GetOntapVersion() string {
	c.m.RLock()
	defer c.m.RUnlock()
	return c.OntapVersion
}

// SystemGetOntapVersion gets the ONTAP version using the credentials, and caches & returns the result.
func (c *RestClient) SystemGetOntapVersion(
	ctx context.Context, cached bool,
) (string, error) {
	ontapApiVersion := c.GetOntapVersion()
	if ontapApiVersion != "" && cached {
		// return cached version
		return ontapApiVersion, nil
	}

	// it wasn't cached, look it up and cache it
	clusterInfoResult, err := c.ClusterInfo(ctx, nil, false)
	if err != nil {
		return "unknown", err
	}
	if clusterInfoResult == nil || clusterInfoResult.Payload == nil {
		return "unknown", fmt.Errorf("could not determine cluster version")
	}

	version := clusterInfoResult.Payload.Version
	if version == nil || version.Generation == nil || version.Major == nil || version.Minor == nil {
		return "unknown", fmt.Errorf("could not determine cluster version")
	}

	// version.Full // "NetApp Release 9.8X29: Sun Sep 27 12:15:48 UTC 2020"
	ontapApiVersion = fmt.Sprintf("%d.%d.%d", *version.Generation, *version.Major, *version.Minor) // 9.8.0
	c.SetOntapVersion(ontapApiVersion)

	parsedVersion, err := versionutils.ParseSemantic(ontapApiVersion)
	if err != nil {
		return "unknown", fmt.Errorf("could not determine cluster version %s; %v", ontapApiVersion, err)
	}

	// Determine ONTAP personality by invoking ClusterInfo again and requesting fields that ONTAP doesn't
	// return with vsadmin credentials even if fields="**".
	if parsedVersion.AtLeast(MinimumASAR2Version) {

		clusterInfoResult, err = c.ClusterInfo(ctx, []string{"san_optimized", "disaggregated"}, true)
		if err != nil {
			return "unknown", err
		}
		if clusterInfoResult == nil || clusterInfoResult.Payload == nil {
			return "unknown", fmt.Errorf("could not determine cluster personality")
		}

		sanOptimized := clusterInfoResult.Payload.SanOptimized
		if sanOptimized != nil {
			c.SetSANOptimized(*sanOptimized)
		}

		disaggregated := clusterInfoResult.Payload.Disaggregated
		if disaggregated != nil {
			c.SetDisaggregated(*disaggregated)
		}
	}

	return ontapApiVersion, nil
}

// NodeList returns information about nodes
func (c *RestClient) NodeList(ctx context.Context, pattern string) (*cluster.NodesGetOK, error) {
	params := cluster.NewNodesGetParamsWithTimeout(c.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SetName(convert.ToPtr(pattern))
	fields := []string{"serial_number"}
	params.SetFields(fields)

	params.SetFields(fields)
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
			if resultNext == nil || resultNext.Payload == nil || resultNext.Payload.NumRecords == nil {
				done = true
				continue NextLoop
			}

			if result.Payload.NumRecords == nil {
				result.Payload.NumRecords = convert.ToPtr(int64(0))
			}
			result.Payload.NumRecords = convert.ToPtr(*result.Payload.NumRecords + *resultNext.Payload.NumRecords)
			result.Payload.NodeResponseInlineRecords = append(result.Payload.NodeResponseInlineRecords,
				resultNext.Payload.NodeResponseInlineRecords...)

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

func (c *RestClient) NodeListSerialNumbers(ctx context.Context) ([]string, error) {
	serialNumbers := make([]string, 0)

	nodeListResult, err := c.NodeList(ctx, "*")
	if err != nil {
		return serialNumbers, err
	}
	if nodeListResult == nil {
		return serialNumbers, errors.New("could not get node info")
	}
	if nodeListResult.Payload == nil || nodeListResult.Payload.NumRecords == nil {
		return serialNumbers, errors.New("could not get node info")
	}

	if *nodeListResult.Payload.NumRecords == 0 {
		return serialNumbers, errors.New("could not get node info")
	}

	// Get the serial numbers
	for _, node := range nodeListResult.Payload.NodeResponseInlineRecords {
		serialNumber := node.SerialNumber
		if serialNumber != nil && *serialNumber != "" {
			serialNumbers = append(serialNumbers, *serialNumber)
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
func (c *RestClient) EmsAutosupportLog(
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

	params.ReturnRecords = convert.ToPtr(true)

	emsApplicationLog := &models.EmsApplicationLog{
		AppVersion:          convert.ToPtr(appVersion),
		AutosupportRequired: convert.ToPtr(autoSupport),
		Category:            convert.ToPtr(category),
		ComputerName:        convert.ToPtr(computerName),
		EventDescription:    convert.ToPtr(eventDescription),
		EventID:             convert.ToPtr(int64(eventID)),
		EventSource:         convert.ToPtr(eventSource),
		Severity:            convert.ToPtr(models.EmsApplicationLogSeverityNotice),
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

func (c *RestClient) TieringPolicyValue(
	ctx context.Context,
) (tieringPolicy string) {
	// Use "none" for unified ONTAP, and "" for disaggregated ONTAP.
	tieringPolicy = "none"

	parsedVersion, err := versionutils.ParseSemantic(c.GetOntapVersion())
	if err != nil {
		return
	}

	// TODO (cknight): verify this also applies to ASA r2
	if c.disaggregated && parsedVersion.AtLeast(MinimumDisaggregatedTieringPolicyRemovedVersion) {
		tieringPolicy = "" // An empty string prevents sending tieringPolicy when creating volumes.
	}
	return tieringPolicy
}

// ///////////////////////////////////////////////////////////////////////////
// EXPORT POLICY operations BEGIN

// ExportPolicyCreate creates an export policy
// equivalent to filer::> vserver export-policy create
func (c *RestClient) ExportPolicyCreate(ctx context.Context, policy string) (*nas.ExportPolicyCreateCreated, error) {
	params := nas.NewExportPolicyCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	exportPolicyInfo := &models.ExportPolicy{
		Name: convert.ToPtr(policy),
		Svm: &models.ExportPolicyInlineSvm{
			UUID: convert.ToPtr(c.svmUUID),
		},
	}
	params.SetInfo(exportPolicyInfo)

	return c.api.Nas.ExportPolicyCreate(params, c.authInfo)
}

// ExportPolicyGet gets the export policy with the specified uuid
func (c *RestClient) ExportPolicyGet(ctx context.Context, id int64) (*nas.ExportPolicyGetOK, error) {
	params := nas.NewExportPolicyGetParamsWithTimeout(c.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.ID = id

	return c.api.Nas.ExportPolicyGet(params, c.authInfo)
}

// ExportPolicyList returns the names of all export polices whose names match the supplied pattern
func (c *RestClient) ExportPolicyList(ctx context.Context, pattern string) (*nas.ExportPolicyCollectionGetOK, error) {
	params := nas.NewExportPolicyCollectionGetParamsWithTimeout(c.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SvmUUID = convert.ToPtr(c.svmUUID)

	params.SetName(convert.ToPtr(pattern))
	fields := []string{""} // default 'none' gives ID
	params.SetFields(fields)

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
			if resultNext == nil || resultNext.Payload == nil || resultNext.Payload.NumRecords == nil {
				done = true
				continue NextLoop
			}

			if result.Payload.NumRecords == nil {
				result.Payload.NumRecords = convert.ToPtr(int64(0))
			}
			result.Payload.NumRecords = convert.ToPtr(*result.Payload.NumRecords + *resultNext.Payload.NumRecords)
			result.Payload.ExportPolicyResponseInlineRecords = append(result.Payload.ExportPolicyResponseInlineRecords,
				resultNext.Payload.ExportPolicyResponseInlineRecords...)

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
func (c *RestClient) ExportPolicyGetByName(ctx context.Context, exportPolicyName string) (*models.ExportPolicy, error) {
	result, err := c.ExportPolicyList(ctx, exportPolicyName)
	if err == nil && result != nil && result.Payload != nil && result.Payload.NumRecords != nil &&
		*result.Payload.NumRecords == 1 && result.Payload.ExportPolicyResponseInlineRecords != nil {
		return result.Payload.ExportPolicyResponseInlineRecords[0], nil
	}
	return nil, err
}

func (c *RestClient) ExportPolicyDestroy(ctx context.Context, policy string) (*nas.ExportPolicyDeleteOK, error) {
	exportPolicy, err := c.ExportPolicyGetByName(ctx, policy)
	if err != nil {
		return nil, err
	}
	if exportPolicy == nil {
		return nil, fmt.Errorf("could not get export policy %v", policy)
	}
	if exportPolicy.ID == nil {
		return nil, fmt.Errorf("could not get id for export policy %v", policy)
	}

	params := nas.NewExportPolicyDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.ID = *exportPolicy.ID

	return c.api.Nas.ExportPolicyDelete(params, c.authInfo)
}

// ExportRuleList returns the export rules in an export policy
// equivalent to filer::> vserver export-policy rule show
func (c *RestClient) ExportRuleList(ctx context.Context, policy string) (*nas.ExportRuleCollectionGetOK, error) {
	exportPolicy, err := c.ExportPolicyGetByName(ctx, policy)
	if err != nil {
		return nil, err
	}
	if exportPolicy == nil {
		return nil, fmt.Errorf("could not get export policy %v", policy)
	}
	if exportPolicy.ID == nil {
		return nil, fmt.Errorf("could not get id for export policy %v", policy)
	}

	params := nas.NewExportRuleCollectionGetParamsWithTimeout(c.httpClient.Timeout)

	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.PolicyID = *exportPolicy.ID

	fields := []string{"clients"}
	params.SetFields(fields)

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
			if resultNext == nil || resultNext.Payload == nil || resultNext.Payload.NumRecords == nil {
				done = true
				continue NextLoop
			}

			if result.Payload.NumRecords == nil {
				result.Payload.NumRecords = convert.ToPtr(int64(0))
			}
			result.Payload.NumRecords = convert.ToPtr(*result.Payload.NumRecords + *resultNext.Payload.NumRecords)
			result.Payload.ExportRuleResponseInlineRecords = append(result.Payload.ExportRuleResponseInlineRecords,
				resultNext.Payload.ExportRuleResponseInlineRecords...)

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
func (c *RestClient) ExportRuleCreate(
	ctx context.Context, policy, clientMatch string, protocols, roSecFlavors, rwSecFlavors, suSecFlavors []string,
) (*nas.ExportRuleCreateCreated, error) {
	exportPolicy, err := c.ExportPolicyGetByName(ctx, policy)
	if err != nil {
		return nil, err
	}
	if exportPolicy == nil {
		return nil, fmt.Errorf("could not get export policy %v", policy)
	}
	if exportPolicy.ID == nil {
		return nil, fmt.Errorf("could not get id for export policy %v", policy)
	}

	params := nas.NewExportRuleCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.PolicyID = *exportPolicy.ID

	info := &models.ExportRule{}

	var clients []*models.ExportClients
	for _, match := range strings.Split(clientMatch, ",") {
		clients = append(clients, &models.ExportClients{Match: convert.ToPtr(match)})
	}
	info.ExportRuleInlineClients = clients

	if len(protocols) > 0 {
		info.Protocols = convert.ToSlicePtrs(protocols)
	}
	if len(roSecFlavors) > 0 {
		info.ExportRuleInlineRoRule = ToExportAuthenticationFlavorSlice(roSecFlavors)
	}
	if len(rwSecFlavors) > 0 {
		info.ExportRuleInlineRwRule = ToExportAuthenticationFlavorSlice(rwSecFlavors)
	}
	if len(suSecFlavors) > 0 {
		info.ExportRuleInlineSuperuser = ToExportAuthenticationFlavorSlice(suSecFlavors)
	}
	params.SetInfo(info)

	return c.api.Nas.ExportRuleCreate(params, c.authInfo)
}

// ToExportAuthenticationFlavorSlice converts a slice of strings into a slice of ExportAuthenticationFlavor
func ToExportAuthenticationFlavorSlice(authFlavor []string) []*models.ExportAuthenticationFlavor {
	var result []*models.ExportAuthenticationFlavor
	for _, s := range authFlavor {
		v := models.ExportAuthenticationFlavor(s)
		switch v {
		case models.ExportAuthenticationFlavorAny:
			result = append(result, models.ExportAuthenticationFlavorAny.Pointer())
		case models.ExportAuthenticationFlavorNone:
			result = append(result, models.ExportAuthenticationFlavorNone.Pointer())
		case models.ExportAuthenticationFlavorNever:
			result = append(result, models.ExportAuthenticationFlavorNever.Pointer())
		case models.ExportAuthenticationFlavorKrb5:
			result = append(result, models.ExportAuthenticationFlavorKrb5.Pointer())
		case models.ExportAuthenticationFlavorKrb5i:
			result = append(result, models.ExportAuthenticationFlavorKrb5i.Pointer())
		case models.ExportAuthenticationFlavorKrb5p:
			result = append(result, models.ExportAuthenticationFlavorKrb5p.Pointer())
		case models.ExportAuthenticationFlavorNtlm:
			result = append(result, models.ExportAuthenticationFlavorNtlm.Pointer())
		case models.ExportAuthenticationFlavorSys:
			result = append(result, models.ExportAuthenticationFlavorSys.Pointer())
		}
	}
	return result
}

// ExportRuleDestroy deletes the rule at the given index in the given policy
func (c *RestClient) ExportRuleDestroy(
	ctx context.Context, policy string, ruleIndex int,
) (*nas.ExportRuleDeleteOK, error) {
	exportPolicy, err := c.ExportPolicyGetByName(ctx, policy)
	if err != nil {
		return nil, err
	}
	if exportPolicy == nil {
		return nil, fmt.Errorf("could not get export policy %v", policy)
	}
	if exportPolicy.ID == nil {
		return nil, fmt.Errorf("could not get id for export policy %v", policy)
	}
	params := nas.NewExportRuleDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.PolicyID = *exportPolicy.ID
	params.Index = int64(ruleIndex)

	ok, err := c.api.Nas.ExportRuleDelete(params, c.authInfo)
	if err != nil {
		if restErr, extractErr := ExtractErrorResponse(ctx, err); extractErr == nil {
			if restErr.Error != nil && restErr.Error.Code != nil && *restErr.Error.Code != ENTRY_DOESNT_EXIST &&
				restErr.Error.Message != nil {
				return ok, errors.New(*restErr.Error.Message)
			}
		} else {
			return ok, err
		}
	}
	return ok, nil
}

// ///////////////////////////////////////////////////////////////////////////
// FlexGroup operations BEGIN

func ToSliceVolumeAggregatesItems(aggrs []string) []*models.VolumeInlineAggregatesInlineArrayItem {
	var result []*models.VolumeInlineAggregatesInlineArrayItem
	for _, aggregateName := range aggrs {
		// Skip empty aggregate names
		if strings.TrimSpace(aggregateName) == "" {
			continue
		}
		item := &models.VolumeInlineAggregatesInlineArrayItem{
			Name: convert.ToPtr(aggregateName),
		}
		result = append(result, item)
	}
	return result
}

// FlexGroupCreate creates a FlexGroup with the specified options
// equivalent to filer::> volume create -vserver svm_name -volume fg_vol_name –auto-provision-as flexgroup -size fg_size
// -state online -type RW -policy default -unix-permissions ---rwxr-xr-x -space-guarantee none -snapshot-policy none
// -security-style unix -encrypt false
func (c *RestClient) FlexGroupCreate(
	ctx context.Context, name string, size int, aggrs []string, spaceReserve, snapshotPolicy, unixPermissions,
	exportPolicy, securityStyle, tieringPolicy, comment string, qosPolicyGroup QosPolicyGroup, encrypt *bool,
	snapshotReserve int,
) error {
	return c.createVolumeByStyle(ctx, name, int64(size), aggrs, spaceReserve, snapshotPolicy, unixPermissions,
		exportPolicy, securityStyle, tieringPolicy, comment, qosPolicyGroup, encrypt, snapshotReserve,
		models.VolumeStyleFlexgroup, false)
}

// FlexgroupCloneSplitStart starts splitting the flexgroup clone
func (c *RestClient) FlexgroupCloneSplitStart(ctx context.Context, volumeName string) error {
	return c.startCloneSplitByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexgroup)
}

// FlexGroupDestroy destroys a FlexGroup
func (c *RestClient) FlexGroupDestroy(ctx context.Context, name string, force bool) error {
	fields := []string{""}
	volume, err := c.FlexGroupGetByName(ctx, name, fields)
	if err != nil {
		return err
	}
	if volume == nil || volume.UUID == nil {
		Logc(ctx).Warnf("volume %s may already be deleted, unexpected response from volume lookup", name)
		return nil
	}
	params := storage.NewVolumeDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = *volume.UUID
	params.Force = &force

	_, volumeDeleteAccepted, err := c.api.Storage.VolumeDelete(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeDeleteAccepted == nil {
		return fmt.Errorf("unexpected response from volume delete")
	}

	jobLink := getGenericJobLinkFromVolumeJobLink(volumeDeleteAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

// FlexGroupExists tests for the existence of a FlexGroup
func (c *RestClient) FlexGroupExists(ctx context.Context, volumeName string) (bool, error) {
	return c.checkVolumeExistsByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexgroup)
}

// FlexGroupSize retrieves the size of the specified flexgroup
func (c *RestClient) FlexGroupSize(ctx context.Context, volumeName string) (uint64, error) {
	return c.getVolumeSizeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexgroup)
}

// FlexGroupUsedSize retrieves the used space of the specified volume
func (c *RestClient) FlexGroupUsedSize(ctx context.Context, volumeName string) (int, error) {
	return c.getVolumeUsedSizeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexgroup)
}

// FlexGroupSetSize sets the size of the specified FlexGroup
func (c *RestClient) FlexGroupSetSize(ctx context.Context, volumeName, newSize string) error {
	return c.setVolumeSizeByNameAndStyle(ctx, volumeName, newSize, models.VolumeStyleFlexgroup)
}

// FlexgroupSetQosPolicyGroupName note: we can't set adaptive policy groups directly during volume clone creation.
func (c *RestClient) FlexgroupSetQosPolicyGroupName(
	ctx context.Context, volumeName string, qosPolicyGroup QosPolicyGroup,
) error {
	return c.setVolumeQosPolicyGroupNameByNameAndStyle(ctx, volumeName, qosPolicyGroup, models.VolumeStyleFlexgroup)
}

// FlexGroupVolumeModifySnapshotDirectoryAccess modifies access to the ".snapshot" directory
func (c *RestClient) FlexGroupVolumeModifySnapshotDirectoryAccess(
	ctx context.Context, flexGroupVolumeName string, enable bool,
) error {
	fields := []string{""}
	volume, err := c.getVolumeByNameAndStyle(ctx, flexGroupVolumeName, models.VolumeStyleFlexgroup, fields)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find flexgroup volume with name %v", flexGroupVolumeName)
	}
	if volume.UUID == nil {
		return fmt.Errorf("could not find flexgroup volume uuid with name %v", flexGroupVolumeName)
	}

	uuid := *volume.UUID

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = uuid

	volumeInfo := &models.Volume{}
	volumeInfo.SnapshotDirectoryAccessEnabled = convert.ToPtr(enable)
	params.SetInfo(volumeInfo)

	_, volumeModifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if volumeModifyAccepted == nil {
		return fmt.Errorf("unexpected response from volume modify")
	}

	jobLink := getGenericJobLinkFromVolumeJobLink(volumeModifyAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

func (c *RestClient) FlexGroupModifyUnixPermissions(ctx context.Context, volumeName, unixPermissions string) error {
	return c.modifyVolumeUnixPermissionsByNameAndStyle(ctx, volumeName, unixPermissions, models.VolumeStyleFlexgroup)
}

// FlexGroupSetComment sets a flexgroup's comment to the supplied value
func (c *RestClient) FlexGroupSetComment(ctx context.Context, volumeName, newVolumeComment string) error {
	return c.setVolumeCommentByNameAndStyle(ctx, volumeName, newVolumeComment, models.VolumeStyleFlexgroup)
}

// FlexGroupGetByName gets the flexgroup with the specified name
func (c *RestClient) FlexGroupGetByName(
	ctx context.Context, volumeName string, fields []string,
) (*models.Volume, error) {
	return c.getVolumeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexgroup, fields)
}

// FlexGroupGetAll returns all relevant details for all FlexGroups whose names match the supplied prefix
func (c *RestClient) FlexGroupGetAll(
	ctx context.Context, pattern string, fields []string,
) (*storage.VolumeCollectionGetOK, error) {
	return c.getAllVolumesByPatternStyleAndState(ctx, pattern, models.VolumeStyleFlexgroup, models.VolumeStateOnline,
		fields)
}

// FlexGroupMount mounts a flexgroup at the specified junction
func (c *RestClient) FlexGroupMount(ctx context.Context, volumeName, junctionPath string) error {
	return c.mountVolumeByNameAndStyle(ctx, volumeName, junctionPath, models.VolumeStyleFlexgroup)
}

// FlexgroupUnmount unmounts the flexgroup
func (c *RestClient) FlexgroupUnmount(ctx context.Context, volumeName string) error {
	return c.unmountVolumeByNameAndStyle(ctx, volumeName, models.VolumeStyleFlexgroup)
}

func (c *RestClient) FlexgroupModifyExportPolicy(ctx context.Context, volumeName, exportPolicyName string) error {
	return c.modifyVolumeExportPolicyByNameAndStyle(ctx, volumeName, exportPolicyName, models.VolumeStyleFlexgroup)
}

// ///////////////////////////////////////////////////////////////////////////
// FlexGroup operations END
// ///////////////////////////////////////////////////////////////////////////

// ///////////////////////////////////////////////////////////////////////////
// QTREE operations BEGIN

// QtreeCreate creates a qtree with the specified options
// equivalent to filer::> qtree create -vserver ndvp_vs -volume v -qtree q -export-policy default -unix-permissions ---rwxr-xr-x -security-style unix
func (c *RestClient) QtreeCreate(
	ctx context.Context, name, volumeName, unixPermissions, exportPolicy, securityStyle, qosPolicy string,
) error {
	params := storage.NewQtreeCreateParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetReturnTimeout(returnTimeout)

	qtreeInfo := &models.Qtree{
		Name:   convert.ToPtr(name),
		Volume: &models.QtreeInlineVolume{Name: convert.ToPtr(volumeName)},
		Svm:    &models.QtreeInlineSvm{UUID: convert.ToPtr(c.svmUUID)},
	}

	// handle options
	if unixPermissions != "" {
		unixPermissions = convertUnixPermissions(unixPermissions)
		volumePermissions, parseErr := strconv.ParseInt(unixPermissions, 10, 64)
		if parseErr != nil {
			return fmt.Errorf("cannot process unix permissions value %v", unixPermissions)
		}
		qtreeInfo.UnixPermissions = convert.ToPtr(volumePermissions)
	}
	if exportPolicy != "" {
		qtreeInfo.ExportPolicy = &models.QtreeInlineExportPolicy{Name: convert.ToPtr(exportPolicy)}
	}
	if securityStyle != "" {
		qtreeInfo.SecurityStyle = models.SecurityStyle(securityStyle).Pointer()
	}
	if qosPolicy != "" {
		qtreeInfo.QosPolicy = &models.QtreeInlineQosPolicy{Name: convert.ToPtr(qosPolicy)}
	}

	params.SetInfo(qtreeInfo)

	created, accepted, err := c.api.Storage.QtreeCreate(params, c.authInfo)
	if err != nil {
		return err
	} else if created != nil {
		Logc(ctx).WithField("name", name).Debug("Qtree created synchronously.")
		return nil
	} else if accepted != nil {
		jobLink := getGenericJobLinkFromQtreeJobLink(accepted.Payload)
		if pollErr := c.PollJobStatus(ctx, jobLink); pollErr != nil {
			return pollErr
		}
		return c.waitForQtree(ctx, volumeName, name)
	}
	return fmt.Errorf("unexpected response from qtree create")
}

// waitForQtree polls for the ONTAP qtree to exist, with backoff retry logic
func (c *RestClient) waitForQtree(ctx context.Context, volumeName, qtreeName string) error {
	checkStatus := func() error {
		qtree, err := c.QtreeGetByName(ctx, qtreeName, volumeName)
		if qtree == nil {
			return fmt.Errorf("Qtree '%v' does not exit within volume '%v', will continue checking", qtreeName,
				volumeName)
		}
		return err
	}
	statusNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Qtree not found, waiting.")
	}
	statusBackoff := backoff.NewExponentialBackOff()
	statusBackoff.InitialInterval = 500 * time.Millisecond
	statusBackoff.MaxInterval = 2 * time.Second
	statusBackoff.Multiplier = 1.414
	statusBackoff.RandomizationFactor = 0.1
	statusBackoff.MaxElapsedTime = 1 * time.Minute

	// Run the existence check using an exponential backoff
	if err := backoff.RetryNotify(checkStatus, statusBackoff, statusNotify); err != nil {
		Logc(ctx).WithField("name", volumeName).Warnf("Qtree not found after %3.2f seconds.",
			statusBackoff.MaxElapsedTime.Seconds())
		return err
	}

	Logc(ctx).WithField("Name", qtreeName).Debugf("Qtree created after %3.2f seconds.",
		statusBackoff.GetElapsedTime().Seconds())

	return nil
}

// QtreeRename renames a qtree
// equivalent to filer::> volume qtree rename
func (c *RestClient) QtreeRename(ctx context.Context, path, newPath string) error {
	fields := []string{""}
	qtree, err := c.QtreeGetByPath(ctx, path, fields)
	if err != nil {
		return err
	}
	if qtree == nil {
		return fmt.Errorf("could not find qtree with path %v", path)
	}
	if qtree.ID == nil {
		return fmt.Errorf("could not find id for qtree with path %v", path)
	}
	if qtree.Volume == nil || qtree.Volume.UUID == nil || qtree.Volume.Name == nil {
		return fmt.Errorf("unexpected response from qtree lookup by path, missing volume information for qtree with path %v",
			path)
	}

	params := storage.NewQtreeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetID(strconv.FormatInt(*qtree.ID, 10))
	params.SetVolumeUUID(*qtree.Volume.UUID)
	params.SetReturnTimeout(returnTimeout)

	qtreeInfo := &models.Qtree{
		Name: convert.ToPtr(strings.TrimPrefix(newPath, "/"+*qtree.Volume.Name+"/")),
	}

	params.SetInfo(qtreeInfo)

	modifyOK, modifyAccepted, err := c.api.Storage.QtreeModify(params, c.authInfo)
	if err != nil {
		return err
	} else if modifyOK != nil {
		Logc(ctx).WithField("newPath", newPath).Debug("Qtree renamed synchronously.")
		return nil
	} else if modifyAccepted != nil {
		jobLink := getGenericJobLinkFromQtreeJobLink(modifyAccepted.Payload)
		return c.PollJobStatus(ctx, jobLink)
	}
	return fmt.Errorf("unexpected response from qtree modify")
}

// QtreeDestroyAsync destroys a qtree in the background
// equivalent to filer::> volume qtree delete -foreground false
func (c *RestClient) QtreeDestroyAsync(ctx context.Context, path string, force bool) error {
	// note, force isn't used
	fields := []string{""}
	qtree, err := c.QtreeGetByPath(ctx, path, fields)
	if err != nil {
		return err
	}
	if qtree == nil {
		return fmt.Errorf("unexpected response from qtree lookup by path")
	}
	if qtree.ID == nil {
		return fmt.Errorf("could not find id for qtree with path %v", path)
	}
	if qtree.Volume == nil || qtree.Volume.UUID == nil || qtree.Volume.Name == nil {
		return fmt.Errorf("unexpected response from qtree lookup by path, missing volume information")
	}

	params := storage.NewQtreeDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetID(strconv.FormatInt(*qtree.ID, 10))
	params.SetVolumeUUID(*qtree.Volume.UUID)
	params.SetReturnTimeout(returnTimeout)

	deleteOK, deleteAccepted, err := c.api.Storage.QtreeDelete(params, c.authInfo)
	if err != nil {
		return err
	} else if deleteOK != nil {
		Logc(ctx).WithField("path", path).Debug("Qtree deleted synchronously.")
		return nil
	} else if deleteAccepted != nil {
		jobLink := getGenericJobLinkFromQtreeJobLink(deleteAccepted.Payload)
		return c.PollJobStatus(ctx, jobLink)
	}
	return fmt.Errorf("unexpected response from qtree delete")
}

// QtreeList returns the names of all Qtrees whose names match the supplied prefix
// equivalent to filer::> volume qtree show
func (c *RestClient) QtreeList(
	ctx context.Context, prefix, volumePrefix string, fields []string,
) (*storage.QtreeCollectionGetOK, error) {
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

	// params.MaxRecords = utils.ToPtr(int64(1)) // use for testing, forces pagination

	params.SetSvmUUID(convert.ToPtr(c.svmUUID))
	params.SetName(convert.ToPtr(namePattern))         // Qtree name prefix
	params.SetVolumeName(convert.ToPtr(volumePattern)) // Flexvol name prefix
	params.SetFields(fields)

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
			if resultNext == nil || resultNext.Payload == nil || resultNext.Payload.NumRecords == nil {
				done = true
				continue NextLoop
			}

			if result.Payload.NumRecords == nil {
				result.Payload.NumRecords = convert.ToPtr(int64(0))
			}
			result.Payload.NumRecords = convert.ToPtr(*result.Payload.NumRecords + *resultNext.Payload.NumRecords)
			result.Payload.QtreeResponseInlineRecords = append(result.Payload.QtreeResponseInlineRecords,
				resultNext.Payload.QtreeResponseInlineRecords...)

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
func (c *RestClient) QtreeGetByPath(ctx context.Context, path string, fields []string) (*models.Qtree, error) {
	// Limit the qtrees to those specified path
	params := storage.NewQtreeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	// params.MaxRecords = utils.ToPtr(int64(1)) // use for testing, forces pagination

	params.SetSvmUUID(convert.ToPtr(c.svmUUID))
	params.SetPath(convert.ToPtr(path))
	params.SetFields(fields)

	result, err := c.api.Storage.QtreeCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if result.Payload == nil {
		return nil, fmt.Errorf("qtree path %s not found", path)
	} else if len(result.Payload.QtreeResponseInlineRecords) > 1 {
		return nil, fmt.Errorf("more than one qtree at path %s found", path)
	} else if len(result.Payload.QtreeResponseInlineRecords) == 1 {
		return result.Payload.QtreeResponseInlineRecords[0], nil
	} else if HasNextLink(result.Payload) {
		return nil, fmt.Errorf("more than one qtree at path %s found", path)
	}

	return nil, fmt.Errorf("qtree path %s not found", path)
}

// QtreeGetByName gets the qtree with the specified name in the specified volume
func (c *RestClient) QtreeGetByName(ctx context.Context, name, volumeName string) (*models.Qtree, error) {
	// Limit to the single qtree /volumeName/name
	params := storage.NewQtreeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	// params.MaxRecords = utils.ToPtr(int64(1)) // use for testing, forces pagination

	params.SvmUUID = convert.ToPtr(c.svmUUID)
	params.SetName(convert.ToPtr(name))
	params.SetVolumeName(convert.ToPtr(volumeName))
	fields := []string{""}
	params.SetFields(fields)

	result, err := c.api.Storage.QtreeCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if result.Payload == nil {
		return nil, fmt.Errorf("qtree %s not found", name)
	} else if len(result.Payload.QtreeResponseInlineRecords) > 1 {
		return nil, fmt.Errorf("more than one qtree %s found", name)
	} else if len(result.Payload.QtreeResponseInlineRecords) == 1 {
		return result.Payload.QtreeResponseInlineRecords[0], nil
	} else if HasNextLink(result.Payload) {
		return nil, fmt.Errorf("more than one qtree %s found", name)
	}

	return nil, fmt.Errorf("qtree %s not found", name)
}

// QtreeCount returns the number of Qtrees in the specified Flexvol, not including the Flexvol itself
func (c *RestClient) QtreeCount(ctx context.Context, volumeName string) (int, error) {
	// Limit the qtrees to those in the specified Flexvol name
	params := storage.NewQtreeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	// params.MaxRecords = utils.ToPtr(int64(1)) // use for testing, forces pagination

	params.SetSvmUUID(convert.ToPtr(c.svmUUID))
	params.SetVolumeName(convert.ToPtr(volumeName)) // Flexvol name
	fields := []string{""}
	params.SetFields(fields)

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
			if resultNext == nil || resultNext.Payload == nil || resultNext.Payload.NumRecords == nil {
				done = true
				continue NextLoop
			}

			if result.Payload.NumRecords == nil {
				result.Payload.NumRecords = convert.ToPtr(int64(0))
			}
			result.Payload.NumRecords = convert.ToPtr(*result.Payload.NumRecords + *resultNext.Payload.NumRecords)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NextLoop
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}

	if result.Payload == nil || result.Payload.NumRecords == nil {
		return 0, nil
	}

	// There will always be one qtree for the Flexvol, so decrement by 1
	n := int(*result.Payload.NumRecords)
	switch n {
	case 0, 1:
		return 0, nil
	default:
		return n - 1, nil
	}
}

// QtreeExists returns true if the named Qtree exists (and is unique in the matching Flexvols)
func (c *RestClient) QtreeExists(ctx context.Context, name, volumePattern string) (bool, string, error) {
	// Limit the qtrees to those matching the Flexvol and Qtree name prefixes
	params := storage.NewQtreeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	// params.MaxRecords = utils.ToPtr(int64(1)) // use for testing, forces pagination

	params.SetSvmUUID(convert.ToPtr(c.svmUUID))
	params.SetName(convert.ToPtr(name))                // Qtree name
	params.SetVolumeName(convert.ToPtr(volumePattern)) // Flexvol name prefix
	fields := []string{""}
	params.SetFields(fields)

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
			if resultNext == nil || resultNext.Payload == nil || resultNext.Payload.NumRecords == nil {
				done = true
				continue NextLoop
			}

			if result.Payload.NumRecords == nil {
				result.Payload.NumRecords = convert.ToPtr(int64(0))
			}
			result.Payload.NumRecords = convert.ToPtr(*result.Payload.NumRecords + *resultNext.Payload.NumRecords)
			result.Payload.QtreeResponseInlineRecords = append(result.Payload.QtreeResponseInlineRecords,
				resultNext.Payload.QtreeResponseInlineRecords...)

			if !HasNextLink(resultNext.Payload) {
				done = true
				continue NextLoop
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}

	if result.Payload == nil || result.Payload.NumRecords == nil {
		return false, "", nil
	}

	// Ensure qtree is unique
	n := *result.Payload.NumRecords
	if n != 1 {
		return false, "", nil
	}

	// Get containing Flexvol
	volume := result.Payload.QtreeResponseInlineRecords[0].Volume
	if volume == nil || volume.Name == nil {
		return false, "", nil
	}

	flexvol := volume.Name
	return true, *flexvol, nil
}

// QtreeGet returns all relevant details for a single qtree
// equivalent to filer::> volume qtree show
func (c *RestClient) QtreeGet(ctx context.Context, name, volumePrefix string) (*models.Qtree, error) {
	pattern := "*"
	if volumePrefix != "" {
		pattern = volumePrefix + "*"
	}

	// Limit the qtrees to those matching the Flexvol name prefix
	params := storage.NewQtreeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	// params.MaxRecords = utils.ToPtr(int64(1)) // use for testing, forces pagination

	params.SetSvmUUID(convert.ToPtr(c.svmUUID))
	params.SetName(convert.ToPtr(name))          // qtree name
	params.SetVolumeName(convert.ToPtr(pattern)) // Flexvol name prefix
	fields := []string{"export_policy.name"}
	params.SetFields(fields)

	result, err := c.api.Storage.QtreeCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	if result.Payload == nil {
		return nil, fmt.Errorf("qtree %s not found", name)
	} else if result.Payload.QtreeResponseInlineRecords == nil {
		return nil, fmt.Errorf("qtree %s records not found", name)
	} else if len(result.Payload.QtreeResponseInlineRecords) > 1 {
		return nil, fmt.Errorf("more than one qtree %s found", name)
	} else if len(result.Payload.QtreeResponseInlineRecords) == 1 {
		return result.Payload.QtreeResponseInlineRecords[0], nil
	} else if HasNextLink(result.Payload) {
		return nil, fmt.Errorf("more than one qtree %s found", name)
	}

	return nil, fmt.Errorf("qtree %s not found", name)
}

// QtreeGetAll returns all relevant details for all qtrees whose Flexvol names match the supplied prefix
// equivalent to filer::> volume qtree show
func (c *RestClient) QtreeGetAll(ctx context.Context, volumePrefix string) (*storage.QtreeCollectionGetOK, error) {
	pattern := "*"
	if volumePrefix != "" {
		pattern = volumePrefix + "*"
	}

	// Limit the qtrees to those matching the Flexvol name prefix
	params := storage.NewQtreeCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	// params.MaxRecords = utils.ToPtr(int64(1)) // use for testing, forces pagination

	params.SvmUUID = convert.ToPtr(c.svmUUID)
	params.SetVolumeName(convert.ToPtr(pattern)) // Flexvol name prefix
	fields := []string{""}
	params.SetFields(fields)

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
			if resultNext == nil || resultNext.Payload == nil || resultNext.Payload.NumRecords == nil {
				done = true
				continue NextLoop
			}

			if result.Payload.NumRecords == nil {
				result.Payload.NumRecords = convert.ToPtr(int64(0))
			}
			result.Payload.NumRecords = convert.ToPtr(*result.Payload.NumRecords + *resultNext.Payload.NumRecords)
			result.Payload.QtreeResponseInlineRecords = append(result.Payload.QtreeResponseInlineRecords,
				resultNext.Payload.QtreeResponseInlineRecords...)

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
func (c *RestClient) QtreeModifyExportPolicy(ctx context.Context, name, volumeName, newExportPolicyName string) error {
	qtree, err := c.QtreeGetByName(ctx, name, volumeName)
	if err != nil {
		return err
	}
	if qtree == nil {
		return fmt.Errorf("could not find qtree %v", name)
	}
	if qtree.ID == nil {
		return fmt.Errorf("could not find id for qtree with name %v", name)
	}
	if qtree.Volume == nil || qtree.Volume.UUID == nil || qtree.Volume.Name == nil {
		return fmt.Errorf("unexpected response from qtree lookup by name, missing volume information for qtree with name %v",
			name)
	}

	params := storage.NewQtreeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetID(strconv.FormatInt(*qtree.ID, 10))
	params.SetVolumeUUID(*qtree.Volume.UUID)
	params.SetReturnTimeout(returnTimeout)

	qtreeInfo := &models.Qtree{
		ExportPolicy: &models.QtreeInlineExportPolicy{
			Name: convert.ToPtr(newExportPolicyName),
		},
	}

	params.SetInfo(qtreeInfo)

	modifyOK, modifyAccepted, err := c.api.Storage.QtreeModify(params, c.authInfo)
	if err != nil {
		return err
	} else if modifyOK != nil {
		Logc(ctx).WithField("name", name).Debug("Qtree export policy modified synchronously.")
		return nil
	} else if modifyAccepted != nil {
		jobLink := getGenericJobLinkFromQtreeJobLink(modifyAccepted.Payload)
		if pollErr := c.PollJobStatus(ctx, jobLink); pollErr != nil {
			apiError, message, code := ExtractError(pollErr)
			if apiError == "failure" && code == EXPORT_POLICY_NOT_FOUND {
				return errors.NotFoundError(message)
			}
			return pollErr
		}
		return nil
	}
	return fmt.Errorf("unexpected response from qtree modify")
}

// QuotaOn enables quotas on a Flexvol
// equivalent to filer::> volume quota on
func (c *RestClient) QuotaOn(ctx context.Context, volumeName string) error {
	return c.quotaModify(ctx, volumeName, true)
}

// QuotaOff disables quotas on a Flexvol
// equivalent to filer::> volume quota off
func (c *RestClient) QuotaOff(ctx context.Context, volumeName string) error {
	return c.quotaModify(ctx, volumeName, false)
}

// quotaModify enables/disables quotas on a Flexvol
func (c *RestClient) quotaModify(ctx context.Context, volumeName string, quotaEnabled bool) error {
	fields := []string{"quota"}
	volume, err := c.VolumeGetByName(ctx, volumeName, fields)
	if err != nil {
		return err
	}
	if volume == nil {
		return fmt.Errorf("could not find volume with name %v", volumeName)
	}
	if volume.UUID == nil {
		return fmt.Errorf("could not find volume uuid with name %v", volumeName)
	}

	if volume.Quota != nil && volume.Quota.Enabled != nil && *volume.Quota.Enabled == quotaEnabled {
		// nothing to do, already the specified value
		return nil
	}

	params := storage.NewVolumeModifyParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetUUID(*volume.UUID)
	params.SetReturnTimeout(returnTimeout)

	volumeInfo := &models.Volume{
		Quota: &models.VolumeInlineQuota{
			Enabled: convert.ToPtr(quotaEnabled),
		},
	}

	params.SetInfo(volumeInfo)

	modifyOK, modifyAccepted, err := c.api.Storage.VolumeModify(params, c.authInfo)
	if err != nil {
		return err
	} else if modifyOK != nil {
		Logc(ctx).WithField("volume", volumeName).Debug("Volume quota modified synchronously.")
		return nil
	} else if modifyAccepted != nil {
		jobLink := getGenericJobLinkFromVolumeJobLink(modifyAccepted.Payload)
		return c.PollJobStatus(ctx, jobLink)
	}
	return fmt.Errorf("unexpected response from volume modify")
}

// QuotaSetEntry updates (or creates) a quota rule with an optional hard disk limit
// equivalent to filer::> volume quota policy rule modify
func (c *RestClient) QuotaSetEntry(ctx context.Context, qtreeName, volumeName, quotaType, diskLimit string) error {
	// We can only modify existing rules, so we must check if this rule exists first
	quotaRule, err := c.QuotaGetEntry(ctx, volumeName, qtreeName, quotaType)
	if err != nil && !errors.IsNotFoundError(err) {
		// Error looking up quota rule
		Logc(ctx).Error(err)
		return err
	} else if err != nil && errors.IsNotFoundError(err) {
		// Quota rule doesn't exist; add it instead
		return c.QuotaAddEntry(ctx, volumeName, qtreeName, quotaType, diskLimit)
	}

	if quotaRule.UUID == nil {
		return fmt.Errorf("unexpected response from quota entry lookup")
	}

	// Quota rule exists; modify it
	params := storage.NewQuotaRuleModifyParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetUUID(*quotaRule.UUID)
	params.SetReturnTimeout(returnTimeout)

	// determine the new hard disk limit value
	if diskLimit == "" {
		return fmt.Errorf("invalid hard disk limit value '%s' for quota modify", diskLimit)
	}
	hardLimit, parseErr := strconv.ParseInt(diskLimit, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("cannot process hard disk limit value %v", diskLimit)
	}

	quotaRuleInfo := &models.QuotaRule{
		Space: &models.QuotaRuleInlineSpace{
			HardLimit: convert.ToPtr(hardLimit),
		},
	}
	params.SetInfo(quotaRuleInfo)

	modifyOK, modifyAccepted, err := c.api.Storage.QuotaRuleModify(params, c.authInfo)
	if err != nil {
		return err
	} else if modifyOK != nil {
		Logc(ctx).WithField("qtree", qtreeName).Debug("Quota rule modified synchronously.")
		return nil
	} else if modifyAccepted != nil {
		jobLink := getGenericJobLinkFromQuotaRuleJobLink(modifyAccepted.Payload)
		return c.PollJobStatus(ctx, jobLink)
	}
	return fmt.Errorf("unexpected response from quota rule modify")
}

// QuotaAddEntry creates a quota rule with an optional hard disk limit
// equivalent to filer::> volume quota policy rule create
func (c *RestClient) QuotaAddEntry(ctx context.Context, volumeName, qtreeName, quotaType, diskLimit string) error {
	params := storage.NewQuotaRuleCreateParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetReturnTimeout(convert.ToPtr(int64(3)))

	quotaRuleInfo := &models.QuotaRule{
		Qtree: &models.QuotaRuleInlineQtree{
			Name: convert.ToPtr(qtreeName),
		},
		Volume: &models.QuotaRuleInlineVolume{
			Name: convert.ToPtr(volumeName),
		},
		Type: convert.ToPtr(quotaType),
	}

	quotaRuleInfo.Svm = &models.QuotaRuleInlineSvm{UUID: convert.ToPtr(c.svmUUID)}

	// handle options
	if diskLimit != "" {
		hardLimit, parseErr := strconv.ParseInt(diskLimit, 10, 64)
		if parseErr != nil {
			return fmt.Errorf("cannot process hard disk limit value %v", diskLimit)
		}
		quotaRuleInfo.Space = &models.QuotaRuleInlineSpace{
			HardLimit: convert.ToPtr(hardLimit),
		}
	}

	params.SetInfo(quotaRuleInfo)

	created, accepted, err := c.api.Storage.QuotaRuleCreate(params, c.authInfo)
	if err != nil {
		return err
	} else if created != nil {
		Logc(ctx).WithField("qtree", qtreeName).Debug("Quota rule created synchronously.")
		return nil
	} else if accepted != nil {
		jobLink := getGenericJobLinkFromQuotaRuleJobLink(accepted.Payload)
		return c.PollJobStatus(ctx, jobLink)
	}
	return fmt.Errorf("unexpected response from quota rule create")
}

// QuotaGetEntry returns the disk limit for a single qtree
// equivalent to filer::> volume quota policy rule show
func (c *RestClient) QuotaGetEntry(
	ctx context.Context, volumeName, qtreeName, quotaType string,
) (*models.QuotaRule, error) {
	params := storage.NewQuotaRuleCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	// params.MaxRecords = utils.ToPtr(int64(1)) // use for testing, forces pagination

	params.SetType(convert.ToPtr(quotaType))
	params.SetSvmUUID(convert.ToPtr(c.svmUUID))
	params.SetQtreeName(convert.ToPtr(qtreeName))
	params.SetVolumeName(convert.ToPtr(volumeName))

	params.SetFields([]string{"uuid", "space.hard_limit"})

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
			if resultNext == nil || resultNext.Payload == nil || resultNext.Payload.NumRecords == nil {
				done = true
				continue NextLoop
			}

			if result.Payload.NumRecords == nil {
				result.Payload.NumRecords = convert.ToPtr(int64(0))
			}
			result.Payload.NumRecords = convert.ToPtr(*result.Payload.NumRecords + *resultNext.Payload.NumRecords)
			result.Payload.QuotaRuleResponseInlineRecords = append(result.Payload.QuotaRuleResponseInlineRecords,
				resultNext.Payload.QuotaRuleResponseInlineRecords...)

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
		return nil, errors.NotFoundError("quota rule entries for %s not found", target)
	} else if len(result.Payload.QuotaRuleResponseInlineRecords) > 1 {
		return nil, fmt.Errorf("more than one quota rule entry for %s found", target)
	} else if len(result.Payload.QuotaRuleResponseInlineRecords) == 1 {
		return result.Payload.QuotaRuleResponseInlineRecords[0], nil
	} else if HasNextLink(result.Payload) {
		return nil, fmt.Errorf("more than one quota rule entry for %s found", target)
	}

	return nil, errors.NotFoundError("no entries for %s", target)
}

// QuotaEntryList returns the disk limit quotas for a Flexvol
// equivalent to filer::> volume quota policy rule show
func (c *RestClient) QuotaEntryList(ctx context.Context, volumeName string) (*storage.QuotaRuleCollectionGetOK, error) {
	params := storage.NewQuotaRuleCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	// params.MaxRecords = utils.ToPtr(int64(1)) // use for testing, forces pagination

	params.SvmUUID = convert.ToPtr(c.svmUUID)
	params.VolumeName = convert.ToPtr(volumeName)
	params.Type = convert.ToPtr("tree")

	params.SetFields([]string{"space.hard_limit", "uuid", "qtree.name", "volume.name"})

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
			if resultNext == nil || resultNext.Payload == nil || resultNext.Payload.NumRecords == nil {
				done = true
				continue NextLoop
			}

			if result.Payload.NumRecords == nil {
				result.Payload.NumRecords = convert.ToPtr(int64(0))
			}
			result.Payload.NumRecords = convert.ToPtr(*result.Payload.NumRecords + *resultNext.Payload.NumRecords)
			result.Payload.QuotaRuleResponseInlineRecords = append(result.Payload.QuotaRuleResponseInlineRecords,
				resultNext.Payload.QuotaRuleResponseInlineRecords...)

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

// ///////////////////////////////////////////////////////////////////////////
// SNAPMIRROR operations BEGIN

// GetPeeredVservers returns a list of vservers peered with the vserver for this backend
func (c *RestClient) GetPeeredVservers(ctx context.Context) ([]string, error) {
	peers := *new([]string)

	params := svm.NewSvmPeerCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	fields := []string{"peer.svm.name"}
	params.SetFields(fields)

	result, err := c.api.Svm.SvmPeerCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}

	if result != nil && result.Payload != nil {
		// with no results, we will return the empty peers list
		for _, peerInfo := range result.Payload.SvmPeerResponseInlineRecords {
			if peerInfo != nil && peerInfo.Peer != nil && peerInfo.Peer.Svm != nil && peerInfo.Peer.Svm.Name != nil {
				peers = append(peers, *peerInfo.Peer.Svm.Name)
			}
		}
	}

	return peers, err
}

func (c *RestClient) SnapmirrorRelationshipsList(ctx context.Context) (*snapmirror.SnapmirrorRelationshipsGetOK, error) {
	params := snapmirror.NewSnapmirrorRelationshipsGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	fields := []string{"soruce", "destination"}
	params.SetFields(fields)

	results, err := c.api.Snapmirror.SnapmirrorRelationshipsGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}

	return results, nil
}

// IsVserverDRDestination identifies if the Vserver is a destination vserver of Snapmirror relationship (SVM-DR) or not
func (c *RestClient) IsVserverDRDestination(ctx context.Context) (bool, error) {
	results, err := c.SnapmirrorRelationshipsList(ctx)
	if err != nil {
		return false, err
	}
	if results == nil || results.Payload == nil {
		return false, nil
	}

	isSVMDRDestination := false
	for _, relationship := range results.Payload.SnapmirrorRelationshipResponseInlineRecords {
		if relationship == nil || relationship.Source == nil {
			continue
		}
		if relationship.Source.Path == nil {
			continue
		}
		if relationship.Source.Svm == nil || relationship.Source.Svm.Name == nil {
			continue
		}
		destinationLocation := *relationship.Destination.Path
		destinationVserver := *relationship.Destination.Svm.Name
		if (destinationVserver + ":") == destinationLocation {
			isSVMDRDestination = true
		}
	}

	return isSVMDRDestination, nil
}

// IsVserverDRSource identifies if the Vserver is a source vserver of Snapmirror relationship (SVM-DR) or not
func (c *RestClient) IsVserverDRSource(ctx context.Context) (bool, error) {
	results, err := c.SnapmirrorRelationshipsList(ctx)
	if err != nil {
		return false, err
	}
	if results == nil || results.Payload == nil {
		return false, nil
	}

	isSVMDRSource := false
	for _, relationship := range results.Payload.SnapmirrorRelationshipResponseInlineRecords {
		if relationship == nil || relationship.Source == nil {
			continue
		}
		if relationship.Source.Path == nil {
			continue
		}
		if relationship.Source.Svm == nil || relationship.Source.Svm.Name == nil {
			continue
		}
		sourceLocation := *relationship.Source.Path
		sourceVserver := *relationship.Source.Svm.Name
		if (sourceVserver + ":") == sourceLocation {
			isSVMDRSource = true
		}
	}

	return isSVMDRSource, nil
}

// IsVserverInSVMDR identifies if the Vserver is in Snapmirror relationship (SVM-DR) or not
func (c *RestClient) IsVserverInSVMDR(ctx context.Context) bool {
	isSVMDRSource, _ := c.IsVserverDRSource(ctx)
	isSVMDRDestination, _ := c.IsVserverDRDestination(ctx)

	return isSVMDRSource || isSVMDRDestination
}

func (c *RestClient) SnapmirrorGet(
	ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName string, fields []string,
) (*models.SnapmirrorRelationship, error) {
	params := snapmirror.NewSnapmirrorRelationshipsGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	params.SetFields(fields)

	results, err := c.api.Snapmirror.SnapmirrorRelationshipsGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if results == nil || results.Payload == nil {
		return nil, NotFoundError("could not find relationship")
	}

	for _, relationship := range results.Payload.SnapmirrorRelationshipResponseInlineRecords {
		if relationship == nil || relationship.Destination == nil || relationship.Source == nil {
			continue
		}

		if localFlexvolName != "" {
			if relationship.Destination.Path == nil {
				continue
			}
			if *relationship.Destination.Path != fmt.Sprintf("%s:%s", c.SVMName(), localFlexvolName) {
				continue
			}
		}

		if remoteFlexvolName != "" {
			if relationship.Source.Path == nil {
				continue
			}
			if *relationship.Source.Path != fmt.Sprintf("%s:%s", remoteSVMName, remoteFlexvolName) {
				continue
			}
		}

		return relationship, nil
	}

	return nil, NotFoundError("could not find relationship")
}

func (c *RestClient) SnapmirrorListDestinations(
	ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName string,
) (*models.SnapmirrorRelationship, error) {
	params := snapmirror.NewSnapmirrorRelationshipsGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.WithListDestinationsOnly(convert.ToPtr(true))

	fields := []string{"source.path", "destination.path"}
	params.SetFields(fields)

	results, err := c.api.Snapmirror.SnapmirrorRelationshipsGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if results == nil || results.Payload == nil {
		return nil, NotFoundError("could not find relationship")
	}

	for _, relationship := range results.Payload.SnapmirrorRelationshipResponseInlineRecords {
		if relationship == nil || relationship.Destination == nil || relationship.Source == nil {
			continue
		}

		if relationship.Destination.Path == nil || relationship.Source.Path == nil {
			continue
		}

		if localFlexvolName != "" {
			if *relationship.Destination.Path != fmt.Sprintf("%s:%s", c.SVMName(), localFlexvolName) {
				continue
			}
		}

		if remoteFlexvolName != "" {
			if *relationship.Source.Path != fmt.Sprintf("%s:%s", remoteSVMName, remoteFlexvolName) {
				continue
			}
		}

		return relationship, nil
	}

	return nil, NotFoundError("could not find relationship")
}

func (c *RestClient) SnapmirrorCreate(
	ctx context.Context,
	localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName, repPolicy, repSchedule string,
) error {
	params := snapmirror.NewSnapmirrorRelationshipCreateParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	info := &models.SnapmirrorRelationship{
		Destination: &models.SnapmirrorEndpoint{
			Path: convert.ToPtr(fmt.Sprintf("%s:%s", c.SVMName(), localFlexvolName)),
		},
		Source: &models.SnapmirrorSourceEndpoint{
			Path: convert.ToPtr(fmt.Sprintf("%s:%s", remoteSVMName, remoteFlexvolName)),
		},
	}
	if repPolicy != "" {
		info.Policy = &models.SnapmirrorRelationshipInlinePolicy{
			Name: convert.ToPtr(repPolicy),
		}
	}
	if repSchedule != "" {
		info.TransferSchedule = &models.SnapmirrorRelationshipInlineTransferSchedule{
			Name: convert.ToPtr(repSchedule),
		}
	}

	params.SetInfo(info)

	_, snapmirrorRelationshipCreateAccepted, err := c.api.Snapmirror.SnapmirrorRelationshipCreate(params, c.authInfo)
	if err != nil {
		return err
	}

	if snapmirrorRelationshipCreateAccepted == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship create")
	}

	jobLink := getGenericJobLinkFromSMRJobLink(snapmirrorRelationshipCreateAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

func (c *RestClient) SnapmirrorInitialize(
	ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName string,
) error {
	// first, find the relationship so we can then use the UUID to modify it
	fields := []string{"destination.path", "source.path"}
	relationship, err := c.SnapmirrorGet(ctx, localFlexvolName, c.SVMName(), remoteFlexvolName, remoteSVMName, fields)
	if err != nil {
		return err
	}
	if relationship == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship lookup")
	}
	if relationship.UUID == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship lookup")
	}

	params := snapmirror.NewSnapmirrorRelationshipTransferCreateParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetRelationshipUUID(string(*relationship.UUID))

	params.Info = &models.SnapmirrorTransfer{}

	snapmirrorRelationshipTransferCreateAccepted, err := c.api.Snapmirror.SnapmirrorRelationshipTransferCreate(params,
		c.authInfo)
	if err != nil {
		return err
	}
	if snapmirrorRelationshipTransferCreateAccepted == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship transfer modify")
	}

	return nil
}

func (c *RestClient) SnapmirrorResync(
	ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName string,
) error {
	// first, find the relationship, then we can then use the UUID to modify it
	fields := []string{"destination.path", "source.path", "policy.type"}
	relationship, err := c.SnapmirrorGet(ctx, localFlexvolName, c.SVMName(), remoteFlexvolName, remoteSVMName, fields)
	if err != nil {
		return err
	}
	if relationship == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship lookup")
	}
	if relationship.UUID == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship lookup")
	}

	params := snapmirror.NewSnapmirrorRelationshipModifyParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetUUID(string(*relationship.UUID))

	if relationship.Policy != nil && relationship.Policy.Type != nil && *relationship.Policy.Type == "sync" {
		params.Info = &models.SnapmirrorRelationship{
			State: convert.ToPtr(models.SnapmirrorRelationshipStateInSync),
		}
	} else {
		params.Info = &models.SnapmirrorRelationship{
			State: convert.ToPtr(models.SnapmirrorRelationshipStateSnapmirrored),
		}
	}

	_, snapmirrorRelationshipModifyAccepted, err := c.api.Snapmirror.SnapmirrorRelationshipModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if snapmirrorRelationshipModifyAccepted == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship modify")
	}

	jobLink := getGenericJobLinkFromSMRJobLink(snapmirrorRelationshipModifyAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

func (c *RestClient) SnapmirrorBreak(
	ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName, snapshotName string,
) error {
	// first, find the relationship so we can then use the UUID to modify it
	fields := []string{"destination.path", "source.path"}
	relationship, err := c.SnapmirrorGet(ctx, localFlexvolName, c.SVMName(), remoteFlexvolName, remoteSVMName, fields)
	if err != nil {
		return err
	}
	if relationship == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship lookup")
	}
	if relationship.UUID == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship lookup")
	}

	params := snapmirror.NewSnapmirrorRelationshipModifyParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetUUID(string(*relationship.UUID))

	params.Info = &models.SnapmirrorRelationship{
		State: convert.ToPtr(models.SnapmirrorRelationshipStateBrokenOff),
	}

	if snapshotName != "" {
		params.Info.RestoreToSnapshot = convert.ToPtr(snapshotName)
	}

	_, snapmirrorRelationshipModifyAccepted, err := c.api.Snapmirror.SnapmirrorRelationshipModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if snapmirrorRelationshipModifyAccepted == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship modify")
	}

	jobLink := getGenericJobLinkFromSMRJobLink(snapmirrorRelationshipModifyAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

func (c *RestClient) SnapmirrorQuiesce(
	ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName string,
) error {
	// first, find the relationship so we can then use the UUID to modify it
	fields := []string{"destination.path", "source.path"}
	relationship, err := c.SnapmirrorGet(ctx, localFlexvolName, c.SVMName(), remoteFlexvolName, remoteSVMName, fields)
	if err != nil {
		return err
	}
	if relationship == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship lookup")
	}
	if relationship.UUID == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship lookup")
	}

	params := snapmirror.NewSnapmirrorRelationshipModifyParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetUUID(string(*relationship.UUID))

	params.Info = &models.SnapmirrorRelationship{
		State: convert.ToPtr(models.SnapmirrorRelationshipStatePaused),
	}

	_, snapmirrorRelationshipModifyAccepted, err := c.api.Snapmirror.SnapmirrorRelationshipModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if snapmirrorRelationshipModifyAccepted == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship modify")
	}

	jobLink := getGenericJobLinkFromSMRJobLink(snapmirrorRelationshipModifyAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

func (c *RestClient) SnapmirrorAbort(
	ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName string,
) error {
	// find the existing transfer in a relationship, so we can then use the UUID to abort the transfer.
	fields := []string{"destination.path", "source.path", "transfer.uuid"}
	relationship, err := c.SnapmirrorGet(ctx, localFlexvolName, c.SVMName(), remoteFlexvolName, remoteSVMName, fields)
	if err != nil {
		return err
	}
	if relationship == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship lookup")
	}
	if relationship.UUID == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship lookup")
	}
	if relationship.Transfer == nil || relationship.Transfer.UUID == nil {
		// no transfer in progress, nothing to abort
		return nil
	}

	// Abort the transfer in the relationship
	params := snapmirror.NewSnapmirrorRelationshipTransferModifyParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetUUID(string(*relationship.Transfer.UUID))
	params.WithRelationshipUUID(string(*relationship.UUID))

	params.Info = &models.SnapmirrorTransfer{
		State: convert.ToPtr(models.SnapmirrorRelationshipInlineTransferStateAborted),
	}

	if _, err := c.api.Snapmirror.SnapmirrorRelationshipTransferModify(params, c.authInfo); err != nil {
		if _, _, code := ExtractError(err); code == ENTRY_DOESNT_EXIST {
			// Transfer does not exist, ignore the error
			return nil
		}

		return err
	}

	return nil
}

// SnapmirrorRelease removes all local snapmirror relationship metadata from the source vserver
// Intended to be used on the source vserver
func (c *RestClient) SnapmirrorRelease(ctx context.Context, sourceFlexvolName, sourceSVMName string) error {
	// first, find the relationship so we can then use the UUID to delete it
	relationship, err := c.SnapmirrorListDestinations(ctx, "", "", sourceFlexvolName, sourceSVMName)
	if err != nil {
		return err
	}
	if relationship == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship lookup")
	}
	if relationship.UUID == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship lookup")
	}

	// now, delete the relationship via its UUID
	params := snapmirror.NewSnapmirrorRelationshipDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetUUID(string(*relationship.UUID))
	params.WithSourceOnly(convert.ToPtr(true))

	_, snapmirrorRelationshipDeleteAccepted, err := c.api.Snapmirror.SnapmirrorRelationshipDelete(params, c.authInfo)
	if err != nil {
		return err
	}
	if snapmirrorRelationshipDeleteAccepted == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship delete")
	}

	jobLink := getGenericJobLinkFromSMRJobLink(snapmirrorRelationshipDeleteAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

// Intended to be from the destination vserver
func (c *RestClient) SnapmirrorDeleteViaDestination(
	ctx context.Context, localFlexvolName, localSVMName string,
) error {
	// first, find the relationship so we can then use the UUID to delete it
	relationshipUUID := ""
	relationship, err := c.SnapmirrorListDestinations(ctx, localFlexvolName, localSVMName, "", "")
	if err != nil {
		if IsNotFoundError(err) {
			fields := []string{"destination.path", "source.path"}
			relationship, err = c.SnapmirrorGet(ctx, localFlexvolName, localSVMName, "", "", fields)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}
	if relationship == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship lookup")
	}
	if relationship.UUID == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship lookup")
	}

	relationshipUUID = string(*relationship.UUID)

	// now, delete the relationship via its UUID
	params := snapmirror.NewSnapmirrorRelationshipDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetUUID(relationshipUUID)
	params.WithDestinationOnly(convert.ToPtr(true))

	_, snapmirrorRelationshipDeleteAccepted, err := c.api.Snapmirror.SnapmirrorRelationshipDelete(params, c.authInfo)
	if err != nil {
		if restErr, extractErr := ExtractErrorResponse(ctx, err); extractErr == nil {
			if restErr.Error != nil && restErr.Error.Code != nil && *restErr.Error.Code != ENTRY_DOESNT_EXIST {
				return errors.New(*restErr.Error.Message)
			}
		} else {
			return err
		}
	}
	// now, delete the relationship via its UUID
	params2 := snapmirror.NewSnapmirrorRelationshipDeleteParamsWithTimeout(c.httpClient.Timeout)
	params2.SetContext(ctx)
	params2.SetHTTPClient(c.httpClient)
	params2.SetUUID(relationshipUUID)
	params2.WithSourceInfoOnly(convert.ToPtr(true))
	_, snapmirrorRelationshipDeleteAccepted, err = c.api.Snapmirror.SnapmirrorRelationshipDelete(params2, c.authInfo)
	if err != nil {
		if restErr, extractErr := ExtractErrorResponse(ctx, err); extractErr == nil {
			if restErr.Error != nil && restErr.Error.Code != nil && *restErr.Error.Code == ENTRY_DOESNT_EXIST {
				return nil
			}
		} else {
			return err
		}
	}
	if snapmirrorRelationshipDeleteAccepted == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship delete")
	}

	jobLink := getGenericJobLinkFromSMRJobLink(snapmirrorRelationshipDeleteAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

// Intended to be from the destination vserver
func (c *RestClient) SnapmirrorDelete(
	ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName string,
) error {
	// first, find the relationship so we can then use the UUID to delete it
	fields := []string{"destination.path", "source.path"}
	relationship, err := c.SnapmirrorGet(ctx, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName, fields)
	if err != nil {
		return err
	}
	if relationship == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship lookup")
	}
	if relationship.UUID == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship lookup")
	}

	// now, delete the relationship via its UUID
	params := snapmirror.NewSnapmirrorRelationshipDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	params.SetUUID(string(*relationship.UUID))
	params.WithDestinationOnly(convert.ToPtr(true))

	_, snapmirrorRelationshipDeleteAccepted, err := c.api.Snapmirror.SnapmirrorRelationshipDelete(params, c.authInfo)
	if err != nil {
		return err
	}
	if snapmirrorRelationshipDeleteAccepted == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship delete")
	}

	jobLink := getGenericJobLinkFromSMRJobLink(snapmirrorRelationshipDeleteAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

func (c *RestClient) IsVserverDRCapable(ctx context.Context) (bool, error) {
	params := svm.NewSvmPeerCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	params.SvmUUID = &c.svmUUID
	fields := []string{"peer.svm.name"}
	params.SetFields(fields)

	result, err := c.api.Svm.SvmPeerCollectionGet(params, c.authInfo)
	if err != nil {
		return false, err
	}
	if result == nil {
		return false, nil
	}
	if result.Payload == nil {
		return false, nil
	}
	if result.Payload.SvmPeerResponseInlineRecords == nil || len(result.Payload.SvmPeerResponseInlineRecords) < 1 {
		return false, nil
	}

	peerFound := false
	for _, peerInfo := range result.Payload.SvmPeerResponseInlineRecords {
		if peerInfo != nil && peerInfo.Peer != nil && peerInfo.Peer.Svm != nil {
			peerFound = true
		}
	}

	return peerFound, nil
}

func (c *RestClient) SnapmirrorPolicyExists(ctx context.Context, policyName string) (bool, error) {
	policy, err := c.SnapmirrorPolicyGet(ctx, policyName)
	if err != nil {
		return false, err
	}
	if policy == nil {
		return false, nil
	}
	return true, nil
}

func (c *RestClient) SnapmirrorPolicyGet(
	ctx context.Context, policyName string,
) (*snapmirror.SnapmirrorPoliciesGetOK, error) {
	// Policy is typically cluster scoped not SVM scoped, do not use SVM UUID
	params := snapmirror.NewSnapmirrorPoliciesGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	params.SetName(convert.ToPtr(policyName))
	params.SetFields([]string{"type", "sync_type", "copy_all_source_snapshots"})

	result, err := c.api.Snapmirror.SnapmirrorPoliciesGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (c *RestClient) JobScheduleExists(ctx context.Context, jobName string) (bool, error) {
	// Schedule is typically cluster scoped not SVM scoped, do not use SVM UUID
	params := cluster.NewScheduleCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	params.SetName(convert.ToPtr(jobName))

	result, err := c.api.Cluster.ScheduleCollectionGet(params, c.authInfo)
	if err != nil {
		return false, err
	}
	if result == nil || result.Payload == nil {
		return false, fmt.Errorf("nil result finding job with name: %v", jobName)
	}
	if result.Payload.ScheduleResponseInlineRecords == nil || result.Payload.NumRecords == nil || *result.Payload.NumRecords == 0 {
		return false, fmt.Errorf("could not find job with name: %v", jobName)
	}
	if result.Payload.NumRecords != nil && *result.Payload.NumRecords != 1 {
		return false, fmt.Errorf("more than one job found with name: %v", jobName)
	}

	return true, nil
}

// GetSVMState returns the SVM state from the backend storage.
func (c *RestClient) GetSVMState(ctx context.Context) (string, error) {
	svmResult, err := c.SvmGet(ctx, c.svmUUID)
	if err != nil {
		return "", err
	}
	if svmResult == nil || svmResult.Payload == nil || svmResult.Payload.UUID == nil {
		return "", fmt.Errorf("could not find SVM %s (%s)", c.svmName, c.svmUUID)
	}
	if svmResult.Payload.State == nil {
		return "", fmt.Errorf("could not find operational state of SVM %s", c.svmName)
	}

	return *svmResult.Payload.State, nil
}

func (c *RestClient) SnapmirrorUpdate(ctx context.Context, localInternalVolumeName, snapshotName string) error {
	// first, find the relationship so we can then use the UUID to update
	fields := []string{"destination.path", "source.path"}
	relationship, err := c.SnapmirrorGet(ctx, localInternalVolumeName, c.SVMName(), "", "", fields)
	if err != nil {
		return err
	}
	if relationship == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship lookup")
	}
	if relationship.UUID == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship lookup")
	}

	params := snapmirror.NewSnapmirrorRelationshipTransferCreateParamsWithTimeout(c.httpClient.Timeout)
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)
	params.SetRelationshipUUID(string(*relationship.UUID))

	params.Info = &models.SnapmirrorTransfer{}
	if snapshotName != "" {
		params.Info.SourceSnapshot = &snapshotName
	}

	snapmirrorRelationshipTransferCreateAccepted, err := c.api.Snapmirror.SnapmirrorRelationshipTransferCreate(params,
		c.authInfo)
	if err != nil {
		return err
	}
	if snapmirrorRelationshipTransferCreateAccepted == nil {
		return fmt.Errorf("unexpected response from snapmirror relationship transfer update")
	}
	return nil
}

// SNAPMIRROR operations END

// SMBShareCreate creates an SMB share with the specified name and path.
// Equivalent to filer::> vserver cifs share create -share-name <shareName> -path <path>
func (c *RestClient) SMBShareCreate(ctx context.Context, shareName, path string) error {
	params := nas.NewCifsShareCreateParams()
	params.Context = ctx
	params.HTTPClient = c.httpClient

	cifsShareInfo := &models.CifsShare{
		Name: &shareName,
		Path: &path,
	}
	cifsShareInfo.Svm = &models.CifsShareInlineSvm{Name: &c.svmName}
	params.SetInfo(cifsShareInfo)
	result, err := c.api.Nas.CifsShareCreate(params, c.authInfo)
	if err != nil {
		return err
	}
	if result == nil {
		return fmt.Errorf("unexpected response from SMB share create")
	}

	return nil
}

// getSMBShareByName gets an SMB share with the given name.
func (c *RestClient) getSMBShareByName(ctx context.Context, shareName string) (*models.CifsShare, error) {
	params := nas.NewCifsShareCollectionGetParams()
	params.SetContext(ctx)
	params.SetHTTPClient(c.httpClient)

	params.SetSvmName(&c.svmName)
	params.SetName(&shareName)

	result, err := c.api.Nas.CifsShareCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Payload == nil || result.Payload.NumRecords == nil || *result.Payload.NumRecords == 0 {
		return nil, nil
	}
	// The specified SMB share already exists
	if *result.Payload.NumRecords == 1 && result.Payload.CifsShareResponseInlineRecords != nil {
		return result.Payload.CifsShareResponseInlineRecords[0], nil
	}

	return nil, fmt.Errorf("SMB share %s not found", shareName)
}

// SMBShareExists checks for the existence of an SMB share with the given name.
// Equivalent to filer::> cifs share show <shareName>
func (c *RestClient) SMBShareExists(ctx context.Context, smbShareName string) (bool, error) {
	share, err := c.getSMBShareByName(ctx, smbShareName)
	if err != nil {
		return false, err
	}
	if share == nil {
		return false, err
	}

	return true, nil
}

// SMBShareDestroy destroys an SMB share.
// Equivalent to filer::> cifs share delete <shareName>
func (c *RestClient) SMBShareDestroy(ctx context.Context, shareName string) error {
	params := nas.NewCifsShareDeleteParams()
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.Name = shareName
	params.SvmUUID = c.svmUUID

	result, err := c.api.Nas.CifsShareDelete(params, c.authInfo)
	if err != nil {
		return err
	}
	if result == nil {
		return fmt.Errorf("unexpected nil response from SMB share delete")
	}

	return nil
}

// SMBShareAccessControlCreate creates an SMB share access control entry for the specified share.
// Equivalent to filer::> cifs share access-control create -share <shareName> -user-or-group <userOrGroup> -access-type <accessType>
func (c *RestClient) SMBShareAccessControlCreate(ctx context.Context, shareName string,
	smbShareACL map[string]string,
) error {
	params := nas.NewCifsShareACLCreateParams()
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SetShare(shareName)
	params.SetSvmUUID(c.svmUUID)

	for userOrGroup, permission := range smbShareACL {
		cifsShareInfo := &models.CifsShareACL{
			Permission:  &permission,
			UserOrGroup: &userOrGroup,
		}

		params.SetInfo(cifsShareInfo)
		result, err := c.api.Nas.CifsShareACLCreate(params, c.authInfo)
		if err != nil {
			if restErr, extractErr := ExtractErrorResponse(ctx, err); extractErr == nil {
				if restErr.Error != nil && restErr.Error.Code != nil {
					switch *restErr.Error.Code {
					case DUPLICATE_ENTRY:
						Logc(ctx).WithField("userOrGroup", userOrGroup).Debug("Duplicate SMB share ACL entry, ignoring.")
					case INVALID_ENTRY:
						Logc(ctx).WithField("userOrGroup", userOrGroup).Warn("Invalid user or group specified for SMB share access control")
					default:
						if restErr.Error.Message != nil {
							return errors.New(*restErr.Error.Message)
						}
						return fmt.Errorf("ONTAP REST API error code %v with no message", *restErr.Error.Code)
					}
				} else if restErr.Error != nil && restErr.Error.Message != nil {
					return errors.New(*restErr.Error.Message)
				} else {
					return errors.New("unknown ONTAP REST API error")
				}
			} else {
				return err
			}
		}
		if result == nil && err == nil {
			return fmt.Errorf("unexpected response from SMB share access control create")
		}
	}

	return nil
}

// SMBShareAccessControlDelete deletes an SMB share access control entry for the specified share.
// Equivalent to filer::> cifs share access-control delete -share <shareName> -user-or-group <userOrGroup>
func (c *RestClient) SMBShareAccessControlDelete(ctx context.Context, shareName string,
	smbShareACL map[string]string,
) error {
	params := nas.NewCifsShareACLDeleteParams()
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SetSvmUUID(c.svmUUID)
	params.SetShare(shareName)

	for userOrGroup, userOrGroupType := range smbShareACL {
		params.SetUserOrGroup(userOrGroup)
		params.SetType(userOrGroupType)

		result, err := c.api.Nas.CifsShareACLDelete(params, c.authInfo)
		if err != nil {
			if restErr, extractErr := ExtractErrorResponse(ctx, err); extractErr == nil {
				if restErr.Error != nil && restErr.Error.Code != nil && *restErr.Error.Code != ENTRY_DOESNT_EXIST &&
					restErr.Error.Message != nil {
					return errors.New(*restErr.Error.Message)
				}
			} else {
				return err
			}
		}
		if result == nil && err == nil {
			return fmt.Errorf("unexpected response from SMB share create")
		}
	}

	return nil
}

// NVMe Namespace operations
// NVMeNamespaceCreate creates NVMe namespace in the backend's SVM.
func (c *RestClient) NVMeNamespaceCreate(ctx context.Context, ns NVMeNamespace) error {
	params := nvme.NewNvmeNamespaceCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.ReturnRecords = convert.ToPtr(true)

	sizeBytesStr, _ := capacity.ToBytes(ns.Size)
	sizeInBytes, err := convert.ToPositiveInt64(sizeBytesStr)
	if err != nil {
		Logc(ctx).WithField("sizeInBytes", sizeInBytes).WithError(err).Error("Invalid volume size.")
		return fmt.Errorf("invalid volume size: %v", err)
	}

	nsInfo := &models.NvmeNamespace{
		Name:   &ns.Name,
		OsType: &ns.OsType,
		Space: &models.NvmeNamespaceInlineSpace{
			Size:      convert.ToPtr(sizeInBytes),
			BlockSize: convert.ToPtr(int64(ns.BlockSize)),
		},
		Comment: &ns.Comment,
		Svm:     &models.NvmeNamespaceInlineSvm{UUID: convert.ToPtr(c.svmUUID)},
	}

	// Set QosPolicy if present.
	if ns.QosPolicy.Name != "" {
		nsInfo.QosPolicy = &models.NvmeNamespaceInlineQosPolicy{
			Name: convert.ToPtr(ns.QosPolicy.Name),
		}
	}

	params.SetInfo(nsInfo)

	nsCreateCreated, nsCreateAccepted, err := c.api.NvMe.NvmeNamespaceCreate(params, c.authInfo)
	if err != nil {
		return err
	}

	if nsCreateCreated == nil && nsCreateAccepted == nil {
		return fmt.Errorf("unexpected response from NVMe namespace create")
	}

	// For unified ONTAP, we expect the call to be synchronous and response to contain the created namespace.
	if nsCreateCreated != nil {
		if nsCreateCreated.IsSuccess() {
			nsResponse := nsCreateCreated.GetPayload()
			// Verify that the created namespace is the same as the one we requested.
			if nsResponse != nil && nsResponse.NumRecords != nil && *nsResponse.NumRecords == 1 &&
				*nsResponse.NvmeNamespaceResponseInlineRecords[0].Name == ns.Name {
				return nil
			}
			return fmt.Errorf("namespace create call succeeded but newly created namespace not found")
		} else {
			return fmt.Errorf("namespace create failed with error %v", nsCreateCreated.Error())
		}
	}

	// For disaggregated (ASAr2) ONTAP personalities, we expect the call to be async and hence wait for job to complete.
	jobLink := getGenericJobLinkFromNvmeNamespaceJobLink(nsCreateAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

// NVMeNamespaceSetSize updates the namespace size to newSize.
func (c *RestClient) NVMeNamespaceSetSize(ctx context.Context, nsUUID string, newSize int64) error {
	params := nvme.NewNvmeNamespaceModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = nsUUID
	params.Info = &models.NvmeNamespace{
		Space: &models.NvmeNamespaceInlineSpace{
			Size: convert.ToPtr(newSize),
		},
	}

	return c.nvmeNamespaceModify(ctx, params, "namespace resize failed")
}

func (c *RestClient) nvmeNamespaceModify(
	ctx context.Context, params *nvme.NvmeNamespaceModifyParams,
	customErrorMsg string,
) error {
	// Namespace modify can be sync or async depending on the parameter which is modified.
	// Hence, handle the response generically.
	nsModifyOk, nsModifyAccepted, err := c.api.NvMe.NvmeNamespaceModify(params, c.authInfo)
	if err != nil {
		return fmt.Errorf("%s; %w", customErrorMsg, err)
	}

	Logc(ctx).Debugf("NVMe namespace modify response nsModifyOk: %v, nsModifyAccepted: %v", nsModifyOk, nsModifyAccepted)

	// When no error is returned, we expect at least one response to be returned.
	if nsModifyOk == nil && nsModifyAccepted == nil {
		return fmt.Errorf("%s; %s", customErrorMsg, "unexpected response from NVMe namespace modify")
	}

	// Prioritize the first response which is the case when call is synchronous.
	if nsModifyOk != nil {
		if nsModifyOk.IsSuccess() {
			return nil
		} else {
			return fmt.Errorf("%s; %v", customErrorMsg, nsModifyOk.Error())
		}
	}

	// If the response is async and returns job link then wait for job to finish.
	jobLink := getGenericJobLinkFromNvmeNamespaceJobLink(nsModifyAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

// NVMeNamespaceSetComment sets comment for the given namespace.
func (c *RestClient) NVMeNamespaceSetComment(ctx context.Context, nsUUID, comment string) error {
	params := nvme.NewNvmeNamespaceModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = nsUUID
	params.Info = &models.NvmeNamespace{
		Comment: convert.ToPtr(comment),
	}

	return c.nvmeNamespaceModify(ctx, params, "setting comment on namespace failed")
}

// NVMeNamespaceRename renames the namespace.
func (c *RestClient) NVMeNamespaceRename(ctx context.Context, nsUUID, newName string) error {
	// Prepare the parameters for the namespace rename request
	params := nvme.NewNvmeNamespaceModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = nsUUID
	params.Info = &models.NvmeNamespace{
		Name: convert.ToPtr(newName),
	}

	return c.nvmeNamespaceModify(ctx, params, "namespace rename failed")
}

func (c *RestClient) NVMeNamespaceSetQosPolicyGroup(
	ctx context.Context, nsUUID string, qosPolicyGroup QosPolicyGroup,
) error {
	params := nvme.NewNvmeNamespaceModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = nsUUID
	params.Info = &models.NvmeNamespace{
		QosPolicy: &models.NvmeNamespaceInlineQosPolicy{
			Name: convert.ToPtr(qosPolicyGroup.Name),
		},
	}

	return c.nvmeNamespaceModify(ctx, params, "setting QoS policy on namespace failed")
}

// NVMeNamespaceList finds Namespaces with the specified pattern.
func (c *RestClient) NVMeNamespaceList(
	ctx context.Context, pattern string, fields []string,
) (*nvme.NvmeNamespaceCollectionGetOK, error) {
	params := nvme.NewNvmeNamespaceCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SvmUUID = convert.ToPtr(c.svmUUID)
	params.SetName(convert.ToPtr(pattern))
	params.SetFields(fields)

	result, err := c.api.NvMe.NvmeNamespaceCollectionGet(params, c.authInfo)
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
			resultNext, errNext := c.api.NvMe.NvmeNamespaceCollectionGet(params, c.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil || resultNext.Payload == nil || resultNext.Payload.NumRecords == nil {
				done = true
				continue NextLoop
			}

			if result.Payload.NumRecords == nil {
				result.Payload.NumRecords = convert.ToPtr(int64(0))
			}
			result.Payload.NumRecords = convert.ToPtr(*result.Payload.NumRecords + *resultNext.Payload.NumRecords)
			result.Payload.NvmeNamespaceResponseInlineRecords = append(result.Payload.NvmeNamespaceResponseInlineRecords,
				resultNext.Payload.NvmeNamespaceResponseInlineRecords...)

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

// NVMeNamespaceGetByName gets the Namespace with the specified name.
func (c *RestClient) NVMeNamespaceGetByName(ctx context.Context, name string, fields []string) (*models.NvmeNamespace,
	error,
) {
	result, err := c.NVMeNamespaceList(ctx, name, fields)
	if err != nil {
		return nil, err
	}

	if result != nil && result.Payload != nil && result.Payload.NumRecords != nil &&
		*result.Payload.NumRecords == 1 && result.Payload.NvmeNamespaceResponseInlineRecords != nil {
		return result.Payload.NvmeNamespaceResponseInlineRecords[0], nil
	}
	return nil, errors.NotFoundError("could not find namespace with name %v", name)
}

func (c *RestClient) NVMeNamespaceDelete(ctx context.Context, nsUUID string) error {
	params := nvme.NewNvmeNamespaceDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	if nsUUID == "" {
		return fmt.Errorf("no namespace UUID provided")
	}

	params.UUID = nsUUID

	nsDeleteOK, nsDeleteAccepted, err := c.api.NvMe.NvmeNamespaceDelete(params, c.authInfo)
	if err != nil {
		return err
	}

	if nsDeleteOK == nil && nsDeleteAccepted == nil {
		return fmt.Errorf("unexpected response from NVMe namespace delete")
	}

	// Unified ONTAP has sync delete and returns 200 with empty body on success
	if nsDeleteOK != nil {
		if nsDeleteOK.IsSuccess() {
			return nil
		} else {
			return fmt.Errorf("namespace delete failed, %v", nsDeleteOK.Error())
		}
	}

	// ASAr2 personality has async delete and returns job link; thus wait for job to finish
	jobLink := getGenericJobLinkFromNvmeNamespaceJobLink(nsDeleteAccepted.Payload)
	return c.PollJobStatus(ctx, jobLink)
}

// NVMe Subsystem operations
// NVMeSubsystemAddNamespace adds namespace to subsystem-map
func (c *RestClient) NVMeSubsystemAddNamespace(ctx context.Context, subsystemUUID, nsUUID string) error {
	params := nvme.NewNvmeSubsystemMapCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	subsystemMap := &models.NvmeSubsystemMap{
		Namespace: &models.NvmeSubsystemMapInlineNamespace{UUID: convert.ToPtr(nsUUID)},
		Subsystem: &models.NvmeSubsystemMapInlineSubsystem{UUID: convert.ToPtr(subsystemUUID)},
		Svm:       &models.NvmeSubsystemMapInlineSvm{UUID: &c.svmUUID},
	}

	params.SetInfo(subsystemMap)

	_, err := c.api.NvMe.NvmeSubsystemMapCreate(params, c.authInfo)
	if err != nil {
		return err
	}

	return nil
}

// NVMeSubsystemRemoveNamespace removes a namespace from subsystem-map
func (c *RestClient) NVMeSubsystemRemoveNamespace(ctx context.Context, subsysUUID, nsUUID string) error {
	params := nvme.NewNvmeSubsystemMapDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SubsystemUUID = subsysUUID
	params.NamespaceUUID = nsUUID

	_, err := c.api.NvMe.NvmeSubsystemMapDelete(params, c.authInfo)
	if err != nil {
		return fmt.Errorf("error while deleting namespace from subsystem map; %v", err)
	}
	return nil
}

// NVMeIsNamespaceMapped retrives a namespace from subsystem-map
func (c *RestClient) NVMeIsNamespaceMapped(ctx context.Context, subsysUUID, namespaceUUID string) (bool, error) {
	params := nvme.NewNvmeSubsystemMapCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SubsystemUUID = &subsysUUID

	getSubsys, err := c.api.NvMe.NvmeSubsystemMapCollectionGet(params, c.authInfo)
	if err != nil {
		return false, err
	}
	if getSubsys == nil {
		return false, fmt.Errorf("unexpected response while getting subsystem map")
	}

	payload := getSubsys.GetPayload()

	if payload == nil {
		return false, fmt.Errorf("could not get subsystem map collection")
	}

	if *payload.NumRecords > 0 {
		for count := 0; count < int(*payload.NumRecords); count++ {
			record := payload.NvmeSubsystemMapResponseInlineRecords[count]
			if record != nil && record.Namespace != nil && record.Namespace.UUID != nil &&
				*record.Namespace.UUID == namespaceUUID {
				return true, nil
			}
		}
	}
	// No record returned. This means the subsystem is not even in the map. Return success in this case
	return false, nil
}

// NVMeNamespaceCount gets the number of namespaces mapped to a subsystem
func (c *RestClient) NVMeNamespaceCount(ctx context.Context, subsysUUID string) (int64, error) {
	params := nvme.NewNvmeSubsystemMapCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SubsystemUUID = &subsysUUID

	getSubsys, err := c.api.NvMe.NvmeSubsystemMapCollectionGet(params, c.authInfo)
	if err != nil {
		return 0, err
	}
	if getSubsys == nil {
		return 0, fmt.Errorf("unexpected response while getting subsystem map")
	}

	if getSubsys.IsSuccess() {
		payload := getSubsys.GetPayload()
		if payload != nil && payload.NumRecords != nil {
			return *payload.NumRecords, nil
		}
	}

	return 0, fmt.Errorf("failed to get subsystem map collection")
}

// Subsystem operations
// NVMeSubsystemList returns a list of subsystems seen by the host
func (c *RestClient) NVMeSubsystemList(
	ctx context.Context, pattern string, fields []string,
) (*nvme.NvmeSubsystemCollectionGetOK, error) {
	if pattern == "" {
		pattern = "*"
	}

	params := nvme.NewNvmeSubsystemCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SvmUUID = &c.svmUUID
	params.SetName(convert.ToPtr(pattern))
	params.SetFields(fields)

	result, err := c.api.NvMe.NvmeSubsystemCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		Logc(ctx).Debug("Result is empty")
		return nil, nil
	} else {
		Logc(ctx).Debug("Result is", result.Payload)
	}

	if HasNextLink(result.Payload) {
		nextLink := result.Payload.Links.Next
		done := false
	NextLoop:
		for !done {
			resultNext, errNext := c.api.NvMe.NvmeSubsystemCollectionGet(params, c.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil {
				done = true
				continue NextLoop
			}

			*result.Payload.NumRecords += *resultNext.Payload.NumRecords
			result.Payload.NvmeSubsystemResponseInlineRecords = append(result.Payload.NvmeSubsystemResponseInlineRecords,
				resultNext.Payload.NvmeSubsystemResponseInlineRecords...)

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

// NVMeSubsystemGetByName gets the subsystem with the specified name
func (c *RestClient) NVMeSubsystemGetByName(
	ctx context.Context, subsystemName string, fields []string,
) (*models.NvmeSubsystem, error) {
	result, err := c.NVMeSubsystemList(ctx, subsystemName, fields)
	if err != nil {
		return nil, err
	}

	if result != nil && result.Payload != nil {
		if *result.Payload.NumRecords == 1 && result.Payload.NvmeSubsystemResponseInlineRecords != nil {
			return result.Payload.NvmeSubsystemResponseInlineRecords[0], nil
		}
	}
	return nil, nil
}

// NVMeSubsystemCreate creates a new subsystem
func (c *RestClient) NVMeSubsystemCreate(ctx context.Context, subsystemName, comment string) (*models.NvmeSubsystem, error) {
	params := nvme.NewNvmeSubsystemCreateParamsWithTimeout(c.httpClient.Timeout)
	osType := "linux"
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.ReturnRecords = convert.ToPtr(true)
	params.Info = &models.NvmeSubsystem{
		Name:   &subsystemName,
		OsType: &osType,
		Svm:    &models.NvmeSubsystemInlineSvm{UUID: convert.ToPtr(c.svmUUID)},
	}

	// If comment is provided, set the comment
	if len(comment) > 0 {
		params.Info.Comment = convert.ToPtr(comment)
	}

	subsysCreated, err := c.api.NvMe.NvmeSubsystemCreate(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if subsysCreated == nil {
		return nil, fmt.Errorf("unexpected response from subsystem create")
	}

	if subsysCreated.IsSuccess() {
		subsPayload := subsysCreated.GetPayload()

		if subsPayload != nil && *subsPayload.NumRecords == 1 &&
			*subsPayload.NvmeSubsystemResponseInlineRecords[0].Name == subsystemName {
			return subsPayload.NvmeSubsystemResponseInlineRecords[0], nil
		}

		return nil, fmt.Errorf("subsystem create call succeeded but newly created subsystem not found")
	}

	return nil, fmt.Errorf("subsystem create failed with error %v", subsysCreated.Error())
}

// NVMeSubsystemDelete deletes a given subsystem
func (c *RestClient) NVMeSubsystemDelete(ctx context.Context, subsysUUID string) error {
	params := nvme.NewNvmeSubsystemDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.UUID = subsysUUID
	// Set this value so that we don't need to call extra api call to unmap the hosts
	params.AllowDeleteWithHosts = convert.ToPtr(true)

	subsysDeleted, err := c.api.NvMe.NvmeSubsystemDelete(params, c.authInfo)
	if err != nil {
		return fmt.Errorf("issue while deleting the subsystem; %v", err)
	}
	if subsysDeleted == nil {
		return fmt.Errorf("issue while deleting the subsystem")
	}

	if subsysDeleted.IsSuccess() {
		return nil
	}

	return fmt.Errorf("error while deleting subsystem")
}

// NVMeAddHostNqnToSubsystem adds the NQN of the host to the subsystem
func (c *RestClient) NVMeAddHostNqnToSubsystem(ctx context.Context, hostNQN, subsUUID string) error {
	params := nvme.NewNvmeSubsystemHostCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SubsystemUUID = subsUUID
	params.Info = &models.NvmeSubsystemHost{Nqn: &hostNQN}

	hostAdded, err := c.api.NvMe.NvmeSubsystemHostCreate(params, c.authInfo)
	if err != nil {
		return err
	}
	if hostAdded == nil {
		return fmt.Errorf("issue while adding host to subsystem")
	}

	if hostAdded.IsSuccess() {
		return nil
	}

	return fmt.Errorf("error while adding host to subsystem %v", hostAdded.Error())
}

// NVMeRemoveHostFromSubsystem remove the NQN of the host from the subsystem
func (c *RestClient) NVMeRemoveHostFromSubsystem(ctx context.Context, hostNQN, subsUUID string) error {
	params := nvme.NewNvmeSubsystemHostDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SubsystemUUID = subsUUID
	params.Nqn = hostNQN

	hostRemoved, err := c.api.NvMe.NvmeSubsystemHostDelete(params, c.authInfo)
	if err != nil {
		return fmt.Errorf("issue while removing host to subsystem; %v", err)
	}
	if hostRemoved.IsSuccess() {
		return nil
	}

	return fmt.Errorf("error while removing host from subsystem; %v", hostRemoved.Error())
}

// NVMeGetHostsOfSubsystem retuns all the hosts connected to a subsystem
func (c *RestClient) NVMeGetHostsOfSubsystem(ctx context.Context, subsUUID string) ([]*models.NvmeSubsystemHost, error) {
	params := nvme.NewNvmeSubsystemHostCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SubsystemUUID = subsUUID

	hostCollection, err := c.api.NvMe.NvmeSubsystemHostCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if hostCollection == nil {
		return nil, fmt.Errorf("issue while getting hosts of the subsystem")
	}

	if hostCollection.IsSuccess() {
		return hostCollection.GetPayload().NvmeSubsystemHostResponseInlineRecords, nil
	}

	return nil, fmt.Errorf("get hosts of a subsystem call failed")
}

// NVMeNamespaceSize returns the size of the namespace
func (c *RestClient) NVMeNamespaceSize(ctx context.Context, namespacePath string) (int, error) {
	fields := []string{"space.size"}
	namespace, err := c.NVMeNamespaceGetByName(ctx, namespacePath, fields)
	if err != nil {
		return 0, err
	}
	if namespace == nil {
		return 0, errors.NotFoundError("could not find namespace with name %v", namespace)
	}
	size := namespace.Space.Size
	return int(*size), nil
}

// ///////////////////////////////////////////////////////////////////////////

// ////////////////////////////////////////////////////////////////////////////
// ASA.Next operations
// ////////////////////////////////////////////////////////////////////////////

// StorageUnitGetByName gets the storage unit with the specified name
func (c *RestClient) StorageUnitGetByName(ctx context.Context, suName string) (*models.StorageUnit, error) {
	params := san.NewStorageUnitCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.Name = convert.ToPtr(suName)
	params.SvmUUID = convert.ToPtr(c.svmUUID)

	params.Fields = []string{
		"uuid",
		"name",
		"type",
		"space.size",
		"comment",
		"qos_policy.name",
		"create_time",
		"enabled",
		"serial_number",
		"status.state",
		"os_type",
	}

	result, err := c.api.San.StorageUnitCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}

	if result.Payload != nil && result.Payload.NumRecords != nil &&
		*result.Payload.NumRecords == 1 && result.Payload.StorageUnitResponseInlineRecords != nil &&
		result.Payload.StorageUnitResponseInlineRecords[0] != nil &&
		result.Payload.StorageUnitResponseInlineRecords[0].Name != nil &&
		*result.Payload.StorageUnitResponseInlineRecords[0].Name == suName {
		return result.Payload.StorageUnitResponseInlineRecords[0], nil
	}

	return nil, NotFoundError(fmt.Sprintf("could not find storage unit with name %v", suName))
}

// StorageUnitSnapshotCreateAndWait creates a snapshot and waits on the job to complete
func (c *RestClient) StorageUnitSnapshotCreateAndWait(ctx context.Context, suUUID, snapshotName string) error {
	jobLink, err := c.storageUnitSnapshotCreate(ctx, suUUID, snapshotName)
	if err != nil {
		return fmt.Errorf("could not create snapshot; %v", err)
	}
	if jobLink == nil {
		return fmt.Errorf("could not create snapshot: %v", "no job link found")
	}

	return c.PollJobStatus(ctx, jobLink)
}

func (c *RestClient) storageUnitSnapshotCreate(
	ctx context.Context, suUUID, snapshotName string,
) (*models.JobLinkResponse, error) {
	params := san.NewStorageUnitSnapshotCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.StorageUnitUUID = suUUID

	snapshotInfo := &models.StorageUnitSnapshot{
		Name: convert.ToPtr(snapshotName),
		Svm: &models.StorageUnitSnapshotInlineSvm{
			UUID: convert.ToPtr(c.svmUUID),
		},
	}
	params.SetInfo(snapshotInfo)

	snapshotCreateOk, snapshotCreateAccepted, err := c.api.San.StorageUnitSnapshotCreate(params, c.authInfo)
	if err != nil {
		return nil, err
	}

	// Job link can be returned in either of the responses.
	if snapshotCreateOk != nil {
		return getGenericJobLinkFromStorageUnitSnapshotJobLink(snapshotCreateOk.Payload), nil
	}

	if snapshotCreateAccepted != nil {
		return getGenericJobLinkFromStorageUnitSnapshotJobLink(snapshotCreateAccepted.Payload), nil
	}

	return nil, fmt.Errorf("unexpected response from storage unit snapshot create")
}

func (c *RestClient) StorageUnitSnapshotListByName(
	ctx context.Context, suUUID,
	snapshotName string,
) (*san.StorageUnitSnapshotCollectionGetOK, error) {
	// Get storage unit snapshot by name
	params := san.NewStorageUnitSnapshotCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.StorageUnitUUID = suUUID
	params.Name = convert.ToPtr(snapshotName)
	params.SvmUUID = convert.ToPtr(c.svmUUID)

	// TODO (cknight): pass in fields requested
	params.Fields = []string{"**"}

	result, err := c.api.San.StorageUnitSnapshotCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (c *RestClient) StorageUnitSnapshotList(
	ctx context.Context, suUUID string,
) (*san.StorageUnitSnapshotCollectionGetOK, error) {
	params := san.NewStorageUnitSnapshotCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.StorageUnitUUID = suUUID
	params.SvmUUID = convert.ToPtr(c.svmUUID)

	params.SetFields([]string{"name", "create_time"})

	result, err := c.api.San.StorageUnitSnapshotCollectionGet(params, c.authInfo)
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
			resultNext, errNext := c.api.San.StorageUnitSnapshotCollectionGet(params, c.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil || resultNext.Payload == nil || resultNext.Payload.NumRecords == nil {
				done = true
				continue NextLoop
			}

			if result.Payload.NumRecords == nil {
				result.Payload.NumRecords = convert.ToPtr(int64(0))
			}
			result.Payload.NumRecords = convert.ToPtr(*result.Payload.NumRecords + *resultNext.Payload.NumRecords)
			result.Payload.StorageUnitSnapshotResponseInlineRecords = append(result.Payload.StorageUnitSnapshotResponseInlineRecords,
				resultNext.Payload.StorageUnitSnapshotResponseInlineRecords...)

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

func (c *RestClient) StorageUnitSnapshotRestore(
	ctx context.Context, snapshotName, suUUID string,
) error {
	jobLink, err := c.restoreSUSnapshotByNameAndStyle(ctx, snapshotName, suUUID)
	if err != nil {
		return fmt.Errorf("could not restore storage unit from snapshot; %w", err)
	}
	if jobLink == nil {
		return fmt.Errorf("could not restore storage unit from snapshot: %v", "no job link found")
	}

	return c.PollJobStatus(ctx, jobLink)
}

func (c *RestClient) restoreSUSnapshotByNameAndStyle(
	ctx context.Context, snapshotName,
	suUUID string,
) (*models.JobLinkResponse, error) {
	params := san.NewStorageUnitModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.UUID = suUUID
	params.RestoreToSnapshotName = &snapshotName

	suModifyOk, suModifyAccepted, err := c.api.San.StorageUnitModify(params, c.authInfo)
	if err != nil {
		return nil, err
	}

	// Job link can be returned in either of the responses.
	if suModifyOk != nil {
		return getGenericJobLinkFromStorageUnitJobLink(suModifyOk.Payload), nil
	} else if suModifyAccepted != nil {
		return getGenericJobLinkFromStorageUnitJobLink(suModifyAccepted.Payload), nil
	}

	return nil, fmt.Errorf("unexpected response from storage unit modify")
}

func (c *RestClient) StorageUnitSnapshotGetByName(
	ctx context.Context, snapshotName, suUUID string,
) (*models.StorageUnitSnapshot, error) {
	result, err := c.StorageUnitSnapshotListByName(ctx, suUUID, snapshotName)
	if err != nil {
		return nil, err
	}

	if result != nil && result.Payload != nil && result.Payload.NumRecords != nil &&
		*result.Payload.NumRecords == 1 && result.Payload.StorageUnitSnapshotResponseInlineRecords != nil &&
		result.Payload.StorageUnitSnapshotResponseInlineRecords[0] != nil &&
		result.Payload.StorageUnitSnapshotResponseInlineRecords[0].Name != nil &&
		*result.Payload.StorageUnitSnapshotResponseInlineRecords[0].Name == snapshotName {
		return result.Payload.StorageUnitSnapshotResponseInlineRecords[0], nil
	}
	return nil, NotFoundError(fmt.Sprintf("snapshot %v not found", snapshotName))
}

func (c *RestClient) StorageUnitSnapshotDelete(
	ctx context.Context, suUUID, snapshotUUID string,
) (*models.JobLinkResponse, error) {
	params := san.NewStorageUnitSnapshotDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.StorageUnitUUID = suUUID
	params.UUID = snapshotUUID

	snapshotDeleteOk, snapshotDeleteAccepted, err := c.api.San.StorageUnitSnapshotDelete(params, c.authInfo)
	if err != nil {
		return nil, err
	}

	// Job link can be returned in either of the responses.
	if snapshotDeleteOk != nil {
		return getGenericJobLinkFromStorageUnitSnapshotJobLink(snapshotDeleteOk.Payload), nil
	} else if snapshotDeleteAccepted != nil {
		return getGenericJobLinkFromStorageUnitSnapshotJobLink(snapshotDeleteAccepted.Payload), nil
	}

	return nil, fmt.Errorf("unexpected response from storage unit snapshot delete")
}

func (c *RestClient) StorageUnitCloneCreate(
	ctx context.Context, suUUID, cloneName, snapshot string,
) error {
	jobLink, err := c.suCreateClone(ctx, suUUID, cloneName, snapshot)
	if err != nil {
		return fmt.Errorf("could not create clone %v; %w", cloneName, err)
	}
	if jobLink == nil {
		return fmt.Errorf("could not create clone %v: %v", cloneName, "no job link found")
	}

	return c.PollJobStatus(ctx, jobLink)
}

func (c *RestClient) suCreateClone(
	ctx context.Context, suUUID, cloneName, snapshotName string,
) (*models.JobLinkResponse, error) {
	params := san.NewStorageUnitCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.ReturnRecords = convert.ToPtr(true)

	// Create details of clone info
	var sourceSnapshot *models.StorageUnitCloneInlineSourceInlineSnapshot = nil
	if snapshotName != "" {
		sourceSnapshot = &models.StorageUnitCloneInlineSourceInlineSnapshot{
			Name: convert.ToPtr(snapshotName),
		}
	}
	cloneInfo := &models.StorageUnitClone{
		Source: &models.StorageUnitCloneInlineSource{
			Snapshot: sourceSnapshot,
			StorageUnit: &models.StorageUnitCloneInlineSourceInlineStorageUnit{
				UUID: convert.ToPtr(suUUID),
			},
			Svm: &models.StorageUnitCloneInlineSourceInlineSvm{
				UUID: convert.ToPtr(c.svmUUID),
			},
		},
		IsFlexclone: convert.ToPtr(true),
	}

	// Create a new storage unit with the above clone info
	suInfo := &models.StorageUnit{
		Name:  convert.ToPtr(cloneName),
		Clone: cloneInfo,
		Svm:   &models.StorageUnitInlineSvm{UUID: convert.ToPtr(c.svmUUID)},
	}
	params.SetInfo(suInfo)

	suCreateOk, suCreateAccepted, err := c.api.San.StorageUnitCreate(params, c.authInfo)
	if err != nil {
		return nil, err
	}

	// Job link can be returned in either of the responses.
	if suCreateOk != nil {
		return getGenericJobLinkFromStorageUnitJobLink(suCreateOk.Payload), nil
	} else if suCreateAccepted != nil {
		return getGenericJobLinkFromStorageUnitJobLink(suCreateAccepted.Payload), nil
	}

	return nil, fmt.Errorf("unexpected response from storage unit create")
}

func (c *RestClient) StorageUnitCloneSplitStart(
	ctx context.Context, suUUID string,
) error {
	params := san.NewStorageUnitModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.UUID = suUUID

	suInfo := &models.StorageUnit{
		Clone: &models.StorageUnitClone{SplitInitiated: convert.ToPtr(true)},
	}

	params.SetInfo(suInfo)

	suModifyOk, suModifyAccepted, err := c.api.San.StorageUnitModify(params, c.authInfo)
	if err != nil {
		return err
	}

	if suModifyOk == nil && suModifyAccepted == nil {
		return fmt.Errorf("unexpected response from storage unit modify")
	}

	// If there is explicit error, return the error
	if suModifyOk != nil && !suModifyOk.IsSuccess() {
		return fmt.Errorf("failed to start clone split; %v", suModifyOk.Error())
	}

	if suModifyAccepted != nil && !suModifyAccepted.IsSuccess() {
		return fmt.Errorf("failed to start clone split; %v", suModifyAccepted.Error())
	}

	// If there is explicit error, return the error
	if suModifyOk != nil && !suModifyOk.IsSuccess() {
		return fmt.Errorf("failed to start clone split; %v", suModifyOk.Error())
	}

	if suModifyAccepted != nil && !suModifyAccepted.IsSuccess() {
		return fmt.Errorf("failed to start clone split; %v", suModifyAccepted.Error())
	}

	// At this point, it is clear that no error occurred while trying to start the split.
	// Since clone split usually takes time, we do not wait for its completion.
	// Hence, do not get the jobLink and poll for its status. Assume, it is successful.
	Logc(ctx).WithField("storageUnitUUID", suUUID).Debug(
		"Clone split initiated successfully. This is an asynchronous operation, and its completion is not monitored.")
	return nil
}

func (c *RestClient) StorageUnitListAllBackedBySnapshot(ctx context.Context, suName, snapshotName string) ([]string,
	error,
) {
	return c.listAllSUNamesBackedBySnapshot(ctx, suName, snapshotName)
}

// listAllSUNamesBackedBySnapshot returns the names of all storage units backed by the specified snapshot
func (c *RestClient) listAllSUNamesBackedBySnapshot(ctx context.Context, volumeName, snapshotName string) (
	[]string, error,
) {
	params := san.NewStorageUnitCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient

	params.SvmUUID = &c.svmUUID
	params.SetFields([]string{"name"})

	params.SetCloneSourceStorageUnitName(convert.ToPtr(volumeName))
	params.SetCloneSourceSnapshotName(convert.ToPtr(snapshotName))

	result, err := c.api.San.StorageUnitCollectionGet(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	result.Payload, err = c.getAllStorageUnitPayloadRecords(result.Payload, params)
	if err != nil {
		return nil, err
	}

	suNames := make([]string, 0)
	for _, su := range result.Payload.StorageUnitResponseInlineRecords {
		if su == nil || su.Name == nil {
			continue
		}
		if su.Clone != nil && su.Clone.Source != nil {
			sourceSU := su.Clone.Source.StorageUnit
			sourceSnapshot := su.Clone.Source.Snapshot
			if sourceSU != nil && sourceSU.Name != nil && *sourceSU.Name == volumeName &&
				sourceSnapshot != nil && sourceSnapshot.Name != nil && *sourceSnapshot.Name == snapshotName {
				suNames = append(suNames, *su.Name)
			}
		}
	}
	return suNames, nil
}

func (c *RestClient) getAllStorageUnitPayloadRecords(
	payload *models.StorageUnitResponse,
	params *san.StorageUnitCollectionGetParams,
) (*models.StorageUnitResponse, error) {
	if HasNextLink(payload) {
		nextLink := payload.Links.Next

		for {
			resultNext, errNext := c.api.San.StorageUnitCollectionGet(params, c.authInfo, WithNextLink(nextLink))
			if errNext != nil {
				return nil, errNext
			}
			if resultNext == nil || resultNext.Payload == nil || resultNext.Payload.NumRecords == nil {
				break
			}

			if payload.NumRecords == nil {
				payload.NumRecords = convert.ToPtr(int64(0))
			}
			payload.NumRecords = convert.ToPtr(*payload.NumRecords + *resultNext.Payload.NumRecords)
			payload.StorageUnitResponseInlineRecords = append(payload.StorageUnitResponseInlineRecords,
				resultNext.Payload.StorageUnitResponseInlineRecords...)

			if !HasNextLink(resultNext.Payload) {
				break
			} else {
				nextLink = resultNext.Payload.Links.Next
			}
		}
	}
	return payload, nil
}
