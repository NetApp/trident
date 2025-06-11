// Copyright 2025 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	xrv "github.com/mattermost/xml-roundtrip-validator"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/utils/errors"
)

type ZAPIRequest interface {
	ToXML() (string, error)
}

type ZAPIResponseIterable interface {
	NextTag() string
}

type ZapiRunner struct {
	ManagementLIF        string
	svm                  string
	Username             string
	Password             string
	ClientPrivateKey     string
	ClientCertificate    string
	TrustedCACertificate string
	Secure               bool
	ontapApiVersion      string
	DebugTraceFlags      map[string]bool // Example: {"api":false, "method":true}
	m                    *sync.RWMutex
}

func NewZapiRunner(managementLIF, svm, username, password, clientPrivateKey, clientCertificate,
	clientCACert string, secure bool, ontapApiVersion string, debugTraceFlags map[string]bool,
) *ZapiRunner {
	return &ZapiRunner{
		ManagementLIF:        managementLIF,
		svm:                  svm,
		Username:             username,
		Password:             password,
		ClientPrivateKey:     clientPrivateKey,
		ClientCertificate:    clientCertificate,
		TrustedCACertificate: clientCACert,
		Secure:               secure,
		ontapApiVersion:      ontapApiVersion,
		DebugTraceFlags:      debugTraceFlags,
		m:                    &sync.RWMutex{},
	}
}

// CopyForNontunneledZapiRunner returns a clone of the ZapiRunner configured on this driver with the SVM field cleared
// so ZAPI calls made with the resulting runner aren't tunneled.  Note that the calls could still go directly to either
// a cluster or vserver management LIF.
func (o *ZapiRunner) CopyForNontunneledZapiRunner() *ZapiRunner {
	o.m.RLock()
	defer o.m.RUnlock()
	clone := *o
	clone.m = &sync.RWMutex{}
	clone.svm = "" // Clear the SVM to prevent tunneling
	return &clone
}

// GetZAPIName returns the name of the ZAPI request; it must parse the XML because ZAPIRequest is an interface
//
//	See also: https://play.golang.org/p/IqHhgVB3Q7x
func GetZAPIName(zr ZAPIRequest) (string, error) {
	zapiXML, err := zr.ToXML()
	if err != nil {
		return "", err
	}

	decoder := xml.NewDecoder(strings.NewReader(zapiXML))
	for {
		token, _ := decoder.Token()
		if token == nil {
			break
		}
		switch startElement := token.(type) {
		case xml.StartElement:
			return startElement.Name.Local, nil
		}
	}
	return "", fmt.Errorf("could not find start tag for ZAPI: %v", zapiXML)
}

func (o *ZapiRunner) GetSVM() string {
	o.m.RLock()
	defer o.m.RUnlock()
	return o.svm
}

func (o *ZapiRunner) SetOntapApiVersion(version string) {
	o.m.Lock()
	defer o.m.Unlock()
	o.ontapApiVersion = version
}

func (o *ZapiRunner) GetOntapApiVersion() string {
	o.m.RLock()
	defer o.m.RUnlock()
	return o.ontapApiVersion
}

// SendZapi sends the provided ZAPIRequest to the Ontap system
func (o *ZapiRunner) SendZapi(r ZAPIRequest) (*http.Response, error) {
	startTime := time.Now()

	if o.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "SendZapi", "Type": "ZapiRunner"}
		log.WithFields(fields).Debug(">>>> SendZapi")
		defer log.WithFields(fields).Debug("<<<< SendZapi")
	}

	zapiCommand, err := r.ToXML()
	if err != nil {
		return nil, err
	}

	zapiName, zapiNameErr := GetZAPIName(r)
	if zapiNameErr == nil {
		zapiOpsTotal.WithLabelValues(o.svm, zapiName).Inc()
		defer func() {
			endTime := float64(time.Since(startTime).Milliseconds())
			zapiOpsDurationInMsBySVMSummary.WithLabelValues(o.svm, zapiName).Observe(endTime)
		}()
	}

	s := ""
	redactedRequest := ""
	if o.svm == "" {
		s = fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
          <netapp xmlns="http://www.netapp.com/filer/admin" version="1.21">
            %s
          </netapp>`, zapiCommand)
	} else {
		s = fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
		  <netapp xmlns="http://www.netapp.com/filer/admin" version="1.21" %s>
            %s
          </netapp>`, "vfiler=\""+o.svm+"\"", zapiCommand)
	}
	if o.DebugTraceFlags["api"] {
		secretFields := []string{"outbound-passphrase", "outbound-user-name", "passphrase", "user-name"}
		secrets := make(map[string]string)
		for _, f := range secretFields {
			fmtString := "<%s>%s</%s>"
			secrets[fmt.Sprintf(fmtString, f, ".*", f)] = fmt.Sprintf(fmtString, f, tridentconfig.REDACTED, f)
		}
		redactedRequest = convert.RedactSecretsFromString(s, secrets, true)
		log.Debugf("sending to '%s' xml: \n%s", o.ManagementLIF, redactedRequest)
	}

	url := "http://" + o.ManagementLIF + "/servlets/netapp.servlets.admin.XMLrequest_filer"
	if o.Secure {
		url = "https://" + o.ManagementLIF + "/servlets/netapp.servlets.admin.XMLrequest_filer"
	}
	if o.DebugTraceFlags["api"] {
		log.Debugf("URL:> %s", url)
	}

	b := []byte(s)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/xml")

	// Check to use cert/key and load the cert pair
	var cert tls.Certificate
	caCertPool := x509.NewCertPool()
	skipVerify := true
	if o.ClientCertificate != "" && o.ClientPrivateKey != "" {
		certDecode, err := base64.StdEncoding.DecodeString(o.ClientCertificate)
		if err != nil {
			return nil, errors.New("failed to decode client certificate from base64")
		}
		keyDecode, err := base64.StdEncoding.DecodeString(o.ClientPrivateKey)
		if err != nil {
			return nil, errors.New("failed to decode private key from base64")
		}
		cert, err = tls.X509KeyPair(certDecode, keyDecode)
		if err != nil {
			log.Debugf("error: %v", err)
			return nil, errors.New("cannot load certificate and key")
		}
	} else {
		req.SetBasicAuth(o.Username, o.Password)
	}

	// Check to use trustedCACertificate to use InsecureSkipVerify or not
	if o.TrustedCACertificate != "" {
		trustedCACert, err := base64.StdEncoding.DecodeString(o.TrustedCACertificate)
		if err != nil {
			return nil, errors.New("failed to decode trusted CA certificate from base64")
		}
		skipVerify = false
		caCertPool.AppendCertsFromPEM(trustedCACert)
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: skipVerify, MinVersion: tridentconfig.MinClientTLSVersion,
			Certificates: []tls.Certificate{cert}, RootCAs: caCertPool,
		},
	}

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

	if o.DebugTraceFlags["api"] {
		log.Debugf("response Status: %s", response.Status)
		log.Debugf("response Headers: %s", response.Header)
	}

	return ValidateZAPIResponse(response)
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *ZapiRunner) ExecuteUsing(z ZAPIRequest, requestType string, v interface{}) (interface{}, error) {
	// Copy the v interface, in case we need a clean version for a retry
	o.m.RLock()
	var vCopy interface{}
	if reflect.TypeOf(v).Kind() == reflect.Ptr {
		vCopy = reflect.New(reflect.ValueOf(v).Elem().Type()).Interface()
	} else {
		vCopy = reflect.New(reflect.TypeOf(v)).Elem().Interface()
	}

	svm := o.svm

	// Try API call as-is first
	response, err := o.executeWithoutIteration(z, requestType, v)
	o.m.RUnlock()
	if err != nil {
		// Always return an error if the call itself failed
		return response, err
	}
	// Check for non-existent SVM error in case this is MetroCluster
	zapiError := NewZapiError(response)
	if !zapiError.IsVserverNotFoundError() {
		// This is some other error, or the call succeeded, so return what we got
		return response, err
	}

	// The call failed for a non-existent SVM, so retry with MCC alternate name.
	// Modifying the SVM name in the ZapiRunner prevents retries until the MCC switches again.
	o.m.Lock()
	defer o.m.Unlock()
	if strings.HasSuffix(svm, "-mc") {
		o.svm = strings.TrimSuffix(svm, "-mc")
	} else {
		o.svm += "-mc"
	}

	return o.executeWithoutIteration(z, requestType, vCopy)
}

// executeWithoutIteration does not attempt to perform any nextTag style iteration
func (o *ZapiRunner) executeWithoutIteration(z ZAPIRequest, requestType string, v interface{}) (interface{}, error) {
	if o.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": requestType}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := o.SendZapi(z)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return nil, err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return nil, readErr
	}
	if o.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	// unmarshalErr := xml.Unmarshal(body, &v)
	unmarshalErr := xml.Unmarshal(body, v)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
	}
	if o.DebugTraceFlags["api"] {
		log.Debugf("%s result:\n%v", requestType, v)
	}

	return v, nil
}

// ToString implements a String() function via reflection
func ToString(val reflect.Value) string {
	if reflect.TypeOf(val).Kind() == reflect.Ptr {
		val = reflect.Indirect(val)
	}

	var buffer bytes.Buffer
	if reflect.ValueOf(val).Kind() == reflect.Struct {
		for i := 0; i < val.Type().NumField(); i++ {
			fieldName := val.Type().Field(i).Name
			fieldType := val.Type().Field(i)
			fieldTag := fieldType.Tag
			fieldValue := val.Field(i)

			switch val.Field(i).Kind() {
			case reflect.Ptr:
				fieldValue = reflect.Indirect(val.Field(i))
			default:
				fieldValue = val.Field(i)
			}

			if fieldTag != "" {
				xmlTag := fieldTag.Get("xml")
				if xmlTag != "" {
					fieldName = xmlTag
				}
			}

			if fieldValue.IsValid() {
				buffer.WriteString(fmt.Sprintf("%s: %v\n", fieldName, fieldValue))
			} else {
				buffer.WriteString(fmt.Sprintf("%s: %v\n", fieldName, "nil"))
			}
		}
	}

	return buffer.String()
}

func ValidateZAPIResponse(response *http.Response) (*http.Response, error) {
	resp, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	respString := string(resp)
	// Remove newlines
	sanitizedString := strings.ReplaceAll(respString, "\n", "")

	if errs := xrv.ValidateAll(strings.NewReader(sanitizedString)); len(errs) > 0 {
		for verr := range errs {
			log.Errorf("validation of ZAPI XML caused an error: %v", verr)
		}
		return nil, errors.New("zapiCommand XML response validation failed")
	}

	// Create a manual http response using the already read body, while using all the other fields as-is; this should
	// prevent us from having to change the return type to []byte, which would likely require changes to a large number
	// of azgo files. Creating a response manually with only the Body would not be seen as valid by consumers of the
	// response
	return &http.Response{
		Status:        response.Status,
		StatusCode:    response.StatusCode,
		Proto:         response.Proto,
		ProtoMajor:    response.ProtoMajor,
		ProtoMinor:    response.ProtoMinor,
		Body:          io.NopCloser(bytes.NewBufferString(respString)),
		ContentLength: int64(len(respString)),
		Request:       response.Request,
		Header:        response.Header,
	}, nil
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
		err.code = EINTERNALERROR
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
	case VolumeModifyIterAsyncResponse:
		Logc(ctx).Debugf("NewZapiAsyncResult - processing VolumeModifyIterAsyncResponse: %v", obj)
		// Handle ZAPI result for response object that contains a list of one item with the needed job information.
		volModifyResult, ok := val.Interface().(VolumeModifyIterAsyncResponseResult)
		if !ok {
			return ZapiAsyncResult{}, errors.TypeAssertionError("val.Interface().(azgo.VolumeModifyIterAsyncResponseResult)")
		}
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

// ZapiAsyncResult encapsulates the status, reason, and errno values from a ZAPI invocation, and it provides
// helper methods for detecting common error conditions.
type ZapiAsyncResult struct {
	jobId     int
	status    string
	errorCode int
}

func (z ZapiAsyncResult) JobId() int {
	return z.jobId
}

func (z ZapiAsyncResult) Status() string {
	return z.status
}

func (z ZapiAsyncResult) ErrorCode() int {
	return z.errorCode
}

// ZapiError encapsulates the status, reason, and errno values from a ZAPI invocation, and it provides helper
// methods for detecting common error conditions.
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

func (e ZapiError) IsVserverNotFoundError() bool {
	return e.code == EVSERVERNOTFOUND
}

func (e ZapiError) IsPrivilegeError() bool {
	return e.code == EAPIPRIVILEGE
}

func (e ZapiError) IsScopeError() bool {
	return e.code == EAPIPRIVILEGE || e.code == EAPINOTFOUND
}

func (e ZapiError) IsFailedToLoadJobError() bool {
	return e.code == EINTERNALERROR && strings.Contains(e.reason, "Failed to load job")
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
