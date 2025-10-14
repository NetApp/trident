// Copyright 2025 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-openapi/strfmt"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/pkg/convert"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/n_a_s"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/s_a_n"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/storage"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/svm"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/netapp/trident/utils/errors"
	versionutils "github.com/netapp/trident/utils/version"
)

var ctx = context.Background()

func newRestClient(ip string, httpClient *http.Client) *RestClient {
	clientConfig := ClientConfig{
		DebugTraceFlags:                map[string]bool{"method": true},
		ManagementLIF:                  ip,
		Username:                       "username",
		Password:                       "password",
		unitTestTransportConfigSchemes: "http",
	}
	rs, _ := NewRestClient(context.Background(), clientConfig, "svm0", "ontap-nas")

	if rs != nil {
		rs.OntapVersion = "9.12.1"
		rs.svmUUID = "1234"
	}

	return rs
}

func TestPayload(t *testing.T) {
	// //////////////////
	// negative tests //
	// //////////////////

	// pass a nil
	err := ValidatePayloadExists(ctx, nil)
	assert.Equal(t, "result was nil", err.Error(), "Strings not equal")

	// pass an invalid object type (no Payload field on struct)
	err = ValidatePayloadExists(ctx, azgo.ZapiError{})
	assert.Equal(t, "no payload field exists for type 'ZapiError'", err.Error(), "Strings not equal")

	// pass an invalid pointer object type (no Payload field on struct)
	err = ValidatePayloadExists(ctx, &azgo.ZapiError{})
	assert.Equal(t, "no payload field exists for type '*ZapiError'", err.Error(), "Strings not equal")

	// pass a nil pointer
	var svmCollectionGetOK *svm.SvmCollectionGetOK
	err = ValidatePayloadExists(ctx, svmCollectionGetOK)
	assert.Equal(t, "result was nil", err.Error(), "Strings not equal")

	// pass a valid pointer, with a nil Payload field
	svmCollectionGetOK = &svm.SvmCollectionGetOK{}
	err = ValidatePayloadExists(ctx, svmCollectionGetOK)
	assert.Equal(t, "result payload was nil", err.Error(), "Strings not equal")

	// pass a valid instance, with a nil Payload field
	err = ValidatePayloadExists(ctx, svm.SvmCollectionGetOK{})
	assert.Equal(t, "result payload was nil", err.Error(), "Strings not equal")

	// //////////////////
	// positive tests //
	// //////////////////

	// pass a valid pointer, with a minimal Payload
	svmCollectionGetOK = &svm.SvmCollectionGetOK{Payload: &models.SvmResponse{}}
	err = ValidatePayloadExists(ctx, svmCollectionGetOK)
	assert.Nil(t, err)

	// pass an instance, with a minimal Payload
	err = ValidatePayloadExists(ctx, svm.SvmCollectionGetOK{Payload: &models.SvmResponse{}})
	assert.Nil(t, err)
}

func TestMinimumONTAPVersionForREST(t *testing.T) {
	expectedMinimumONTAPVersion := versionutils.MustParseSemantic("9.12.1")
	assert.Equal(t, MinimumONTAPVersion, expectedMinimumONTAPVersion, "Unexpected minimum ONTAP version")
}

func TestExtractErrorResponse(t *testing.T) {
	// //////////////////
	// negative tests //
	// //////////////////

	var eeResponse *models.ErrorResponse

	// pass a nil
	eeResponse, err := ExtractErrorResponse(ctx, nil)
	assert.Nil(t, eeResponse)
	assert.Equal(t, "rest error was nil", err.Error(), "Strings not equal")

	// pass an invalid object type (no Payload field on struct)
	eeResponse, err = ExtractErrorResponse(ctx, azgo.ZapiError{})
	assert.Nil(t, eeResponse)
	assert.Equal(t, "no error payload field exists for type 'ZapiError'", err.Error(), "Strings not equal")

	// pass an invalid pointer object type (no Payload field on struct)
	eeResponse, err = ExtractErrorResponse(ctx, &azgo.ZapiError{})
	assert.Nil(t, eeResponse)
	assert.Equal(t, "no error payload field exists for type '*ZapiError'", err.Error(), "Strings not equal")

	// pass a nil pointer
	var lunMapReportingNodeCollectionGetOK *s_a_n.LunMapReportingNodeCollectionGetOK
	eeResponse, err = ExtractErrorResponse(ctx, lunMapReportingNodeCollectionGetOK)
	assert.Nil(t, eeResponse)
	assert.Equal(t, "rest error was nil", err.Error(), "Strings not equal")

	// pass a valid pointer, with a nil Payload field
	lunMapReportingNodeCollectionGetOK = &s_a_n.LunMapReportingNodeCollectionGetOK{}
	eeResponse, err = ExtractErrorResponse(ctx, lunMapReportingNodeCollectionGetOK)
	assert.Nil(t, eeResponse)
	assert.Equal(t, "no error payload field exists for type '*LunMapReportingNodeResponse'", err.Error(), "Strings not equal")

	// pass a valid instance, with a nil Payload field
	eeResponse, err = ExtractErrorResponse(ctx, s_a_n.LunMapReportingNodeCollectionGetOK{})
	assert.Nil(t, eeResponse)
	assert.Equal(t, "no error payload field exists for type '*LunMapReportingNodeResponse'", err.Error(), "Strings not equal")

	// //////////////////
	// positive tests //
	// //////////////////

	// pass a LunModifyDefault instance, with no error response (this is the success case usually from a REST call)
	lunModifyDefaultResponse := s_a_n.LunModifyDefault{}
	eeResponse, err = ExtractErrorResponse(ctx, lunModifyDefaultResponse)
	assert.Nil(t, err)
	assert.Nil(t, eeResponse)

	// pass a LunModifyDefault instance, with a populated error response
	lunModifyDefaultResponse = s_a_n.LunModifyDefault{
		Payload: &models.ErrorResponse{
			Error: &models.ReturnedError{
				Code:    convert.ToPtr("42"),
				Message: convert.ToPtr("error 42"),
			},
		},
	}
	eeResponse, err = ExtractErrorResponse(ctx, lunModifyDefaultResponse)
	assert.Nil(t, err)
	assert.NotNil(t, eeResponse)
	assert.Equal(t, *eeResponse.Error.Code, "42", "Unexpected code")
	assert.Equal(t, *eeResponse.Error.Message, "error 42", "Unexpected message")
}

func TestVolumeEncryption(t *testing.T) {
	// negative case:  if nil, should not be set
	veMarshall := models.VolumeInlineEncryption{}
	bytes, _ := json.MarshalIndent(veMarshall, "", "  ")
	assert.Equal(t, `{}`, string(bytes))
	volumeEncrytion := models.VolumeInlineEncryption{}
	json.Unmarshal(bytes, &volumeEncrytion)
	assert.Nil(t, volumeEncrytion.Enabled)

	// positive case:  if set to false, should be sent as false (not omitted)
	veMarshall = models.VolumeInlineEncryption{Enabled: convert.ToPtr(false)}
	bytes, _ = json.MarshalIndent(veMarshall, "", "  ")
	assert.Equal(t,
		`{
  "enabled": false
}`,
		string(bytes))
	volumeEncrytion = models.VolumeInlineEncryption{}
	json.Unmarshal(bytes, &volumeEncrytion)
	assert.False(t, *volumeEncrytion.Enabled)

	// positive case:  if set to true, should be sent as true
	veMarshall = models.VolumeInlineEncryption{Enabled: convert.ToPtr(true)}
	bytes, _ = json.MarshalIndent(veMarshall, "", "  ")
	assert.Equal(t,
		`{
  "enabled": true
}`,
		string(bytes))
	volumeEncrytion = models.VolumeInlineEncryption{}
	json.Unmarshal(bytes, &volumeEncrytion)
	assert.True(t, *volumeEncrytion.Enabled)
}

func TestSnapmirrorErrorCode(t *testing.T) {
	// ensure the error code remains a *string in the swagger definition (it was incorrectly a number)
	messageCode := convert.ToPtr("42")
	smErr := &models.SnapmirrorError{
		Code:    messageCode,
		Message: messageCode,
	}
	assert.Equal(t, messageCode, smErr.Code)
	assert.Equal(t, messageCode, smErr.Message)
}

func setHTTPResponseHeader(w http.ResponseWriter, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
}

func mockResourceNotFound(w http.ResponseWriter, r *http.Request) {
	setHTTPResponseHeader(w, http.StatusNotFound)
	json.NewEncoder(w).Encode("")
}

func getIscsiCredentials() *models.IscsiCredentialsResponse {
	svmName := "fake-svm"
	chapUser := "admin"
	chapPassword := "********"
	initiator := "iqn.1998-01.com.corp.iscsi:name1"
	authType := "chap"
	numRecords := int64(1)

	svm := models.IscsiCredentialsInlineSvm{Name: &svmName}
	inbound := models.IscsiCredentialsInlineChapInlineInbound{User: &chapUser, Password: &chapPassword}
	outbound := models.IscsiCredentialsInlineChapInlineOutbound{User: &chapUser, Password: &chapPassword}
	chap := models.IscsiCredentialsInlineChap{Inbound: &inbound, Outbound: &outbound}
	iscsiCred := models.IscsiCredentials{Chap: &chap, Initiator: &initiator, AuthenticationType: &authType, Svm: &svm}

	return &models.IscsiCredentialsResponse{
		IscsiCredentialsResponseInlineRecords: []*models.
			IscsiCredentials{&iscsiCred},
		NumRecords: &numRecords,
	}
}

func mockIscsiCredentials(w http.ResponseWriter, r *http.Request) {
	iscsiCred := getIscsiCredentials()
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(*iscsiCred)
}

func TestOntapREST_IscsiInitiatorGetDefaultAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(mockIscsiCredentials))
	rs := newRestClient(server.Listener.Addr().String(), server.Client())
	assert.NotNil(t, rs)

	_, err := rs.IscsiInitiatorGetDefaultAuth(ctx, []string{""})
	assert.NoError(t, err, "could not get the iscsi initiator default auth")

	server.Close()
}

func mockIscsiServiceResponse(w http.ResponseWriter, r *http.Request) {
	svmName := "fake-svm"
	enabled := true
	targetName := "iqn.1992-08.com.netapp:sn.574caf71890911e8a6b7005056b4ea79"
	svm := models.IscsiServiceInlineSvm{Name: &svmName}
	iscsiService := models.IscsiService{
		Enabled: &enabled, Target: &models.IscsiServiceInlineTarget{Name: &targetName},
		Svm: &svm,
	}
	iscsiServiceResponse := models.IscsiServiceResponse{
		IscsiServiceResponseInlineRecords: []*models.
			IscsiService{&iscsiService},
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(iscsiServiceResponse)
}

func TestOntapREST_IscsiInterfaceGet(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockIscsiServiceResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			iscsi, err := rs.IscsiInterfaceGet(ctx, []string{""})
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the iscsi interface")
				assert.Equal(t, "fake-svm",
					*iscsi.Payload.IscsiServiceResponseInlineRecords[0].Svm.Name,
					"svm name does not match")
			} else {
				assert.Error(t, err, "get the iscsi interface")
			}
			server.Close()
		})
	}
}

func mockIscsiCredentialsNumRecordsNil(w http.ResponseWriter, r *http.Request) {
	iscsiCred := getIscsiCredentials()
	iscsiCred.NumRecords = nil
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(iscsiCred)
}

func mockIscsiCredentialsFailure(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/svm/svms/1234" {
		mockSVM(w, r)
	} else {
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode("invalidResponse")
	}
}

func mockIscsiCredentialsNumRecordsMoreThanTwo(w http.ResponseWriter, r *http.Request) {
	numRecords := int64(3)
	iscsiCred := getIscsiCredentials()
	iscsiCred.NumRecords = &numRecords
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(iscsiCred)
}

func TestOntapREST_IscsiInitiatorSetDefaultAuth(t *testing.T) {
	chapUser := "admin"
	chapPassword := "********"
	authType := "chap"

	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"ModifyIscsiCredentials", mockIscsiCredentials, false},
		{"ModifyFail", mockIscsiCredentialsFailure, true},
		{"NumRecordsNilInResponse", mockIscsiCredentialsNumRecordsNil, true},
		{"NumRecordsMoreThanTwoInResponse", mockIscsiCredentialsNumRecordsMoreThanTwo, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.IscsiInitiatorSetDefaultAuth(ctx, authType, chapUser, chapPassword, chapUser, chapPassword)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not update the default auth")
			} else {
				assert.Error(t, err, "updated the default auth")
			}
			server.Close()
		})
	}
}

func mockSVMUUIDNil(w http.ResponseWriter, r *http.Request) {
	svm := models.Svm{
		Name:  convert.ToPtr("svm0"),
		State: convert.ToPtr("running"),
		SvmInlineAggregates: []*models.SvmInlineAggregatesInlineArrayItem{
			{
				Name: convert.ToPtr("aggr1"),
				UUID: nil,
			},
		},
	}

	if r.URL.Path == "/api/svm/svms/1234" {
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(svm)
	}

	if r.URL.Path == "/api/svm/svms" {
		svmResponse := models.SvmResponse{
			SvmResponseInlineRecords: []*models.Svm{&svm},
			NumRecords:               convert.ToPtr(int64(1)),
		}
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(svmResponse)
	}
}

func mockIscsiService(w http.ResponseWriter, r *http.Request) {
	svmName := "fake-svm"
	enabled := true
	targetName := "iqn.1992-08.com.netapp:sn.574caf71890911e8a6b7005056b4ea79"
	svm := models.IscsiServiceInlineSvm{Name: &svmName}
	iscsiService := models.IscsiService{
		Enabled: &enabled, Target: &models.IscsiServiceInlineTarget{Name: &targetName},
		Svm: &svm,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(iscsiService)
}

func mockIscsiNodeGetName(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/svm/svms/1234" {
		mockSVM(w, r)
	}

	if r.URL.Path == "/api/protocols/san/iscsi/services/1234" {
		mockIscsiService(w, r)
	}
}

func TestOntapREST_IscsiNodeGetName(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"successTest", mockIscsiNodeGetName, false},
		{"getSVM_fail", mockResourceNotFound, true},
		{"SVMUUID_fail", mockSVMUUIDNil, true},
		{"getIscsiNode_fail", mockIscsiCredentialsFailure, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			iscsi, err := rs.IscsiNodeGetName(ctx, []string{""})

			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the iscsi name")
				assert.Equal(t, "fake-svm", *iscsi.Payload.Svm.Name, "svm name does not match")
			} else {
				assert.Error(t, err, "get the iscsi name")
			}
			server.Close()
		})
	}
}

func getIgroup() *models.IgroupResponse {
	igroupName := "igroup1"
	subsysUUID := "fakeUUID"
	igroup := models.Igroup{
		Name: &igroupName,
		UUID: &subsysUUID,
	}
	IgroupList := []*models.Igroup{&igroup}
	numRecords := int64(1)
	return &models.IgroupResponse{
		IgroupResponseInlineRecords: IgroupList,
		NumRecords:                  &numRecords,
	}
}

func mockIgroup(w http.ResponseWriter, r *http.Request) {
	igroupResponse := getIgroup()

	switch r.Method {
	case "GET":
		setHTTPResponseHeader(w, http.StatusOK)
		if r.URL.Path == "/api/protocols/san/igroups/igroup" {
			json.NewEncoder(w).Encode(igroupResponse.IgroupResponseInlineRecords[0])
		} else if r.URL.Path == "/api/protocols/san/igroups" {
			json.NewEncoder(w).Encode(igroupResponse)
		}
	case "POST":
		setHTTPResponseHeader(w, http.StatusCreated)
		json.NewEncoder(w).Encode(igroupResponse)
	case "DELETE":
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(nil)
	}
}

func mockIgroupNumRecordsNil(w http.ResponseWriter, r *http.Request) {
	igroupResponse := getIgroup()
	igroupResponse.IgroupResponseInlineRecords[0].UUID = nil
	igroupResponse.NumRecords = nil

	if r.Method == "GET" {
		setHTTPResponseHeader(w, http.StatusOK)
	} else {
		setHTTPResponseHeader(w, http.StatusCreated)
	}
	json.NewEncoder(w).Encode(igroupResponse)
}

func mockIgroupNumRecordsMoreThanOne(w http.ResponseWriter, r *http.Request) {
	numRec := int64(2)
	igroupResponse := getIgroup()
	igroupResponse.IgroupResponseInlineRecords[0].UUID = nil
	igroupResponse.NumRecords = &numRec
	setHTTPResponseHeader(w, http.StatusCreated)
	json.NewEncoder(w).Encode(igroupResponse)
}

func TestOntapREST_IgroupCreate(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"IgroupCreate", mockIgroup, false},
		{"BackendReturnError", mockResourceNotFound, true},
		{"NumRecordsNilInResponse", mockIgroupNumRecordsNil, true},
		{"NumRecordsMorethanOneInResponse", mockIgroupNumRecordsMoreThanOne, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.IgroupCreate(ctx, "igroup1", "fake_igroupType", "linux")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not create igroup")
			} else {
				assert.Error(t, err, "igroup created")
			}
			server.Close()
		})
	}
}

func TestOntapREST_IgroupList(t *testing.T) {
	tests := []struct {
		name            string
		pattern         string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"InputPatternHasValue", "igroup", mockIgroup, false},
		{"InputPatternEmpty", "", mockIgroup, false},
		{"BackendReturnError", "igroup", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			igroup, err := rs.IgroupList(ctx, test.pattern, []string{""})

			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get igroup")
				assert.Equal(t, "igroup1",
					*igroup.Payload.IgroupResponseInlineRecords[0].Name,
					"igroup name does not match")
			} else {
				assert.Error(t, err, "get the igroup")
			}
			server.Close()
		})
	}
}

func getHttpServer(
	isNegativeTest bool, mockFunction func(
		hasNextLink bool, w http.ResponseWriter,
		r *http.Request,
	),
) *httptest.Server {
	hasNextLink := true
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hasNextLink && isNegativeTest {
			mockResourceNotFound(w, r)
		} else {
			mockFunction(hasNextLink, w, r)
			hasNextLink = false
		}
	}))
}

func mockIgroupHrefLinkInternalError(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	igroupName := "igroup1"
	subsysUUID := "fakeUUID"
	igroup := models.Igroup{
		Name: &igroupName,
		UUID: &subsysUUID,
	}
	IgroupList := []*models.Igroup{&igroup}
	numRecords := int64(1)

	url := "/api/protocols/san/igroups/igroup"
	var hrefLink *models.IgroupResponseInlineLinks
	sc := http.StatusInternalServerError
	if hasNextLink {
		hrefLink = &models.IgroupResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		sc = http.StatusOK
	}

	igroupResponse := &models.IgroupResponse{
		IgroupResponseInlineRecords: IgroupList,
		NumRecords:                  &numRecords,
		Links:                       hrefLink,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(sc)
	json.NewEncoder(w).Encode(igroupResponse)
}

func mockInternalServerError(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	setHTTPResponseHeader(w, http.StatusInternalServerError)
	json.NewEncoder(w).Encode(nil)
}

func mockIgroupHrefLink(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	igroupName := "igroup1"
	subsysUUID := "fakeUUID"
	igroup := models.Igroup{
		Name: &igroupName,
		UUID: &subsysUUID,
	}
	IgroupList := []*models.Igroup{&igroup}
	numRecords := int64(1)

	url := "/api/protocols/san/igroups/igroup"
	var hrefLink *models.IgroupResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.IgroupResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		hasNextLink = false
	}

	igroupResponse := &models.IgroupResponse{
		IgroupResponseInlineRecords: IgroupList,
		NumRecords:                  &numRecords,
		Links:                       hrefLink,
	}

	if r.Method == "GET" {
		setHTTPResponseHeader(w, http.StatusOK)
		if r.URL.Path == "/api/protocols/san/igroups/igroup" {
			json.NewEncoder(w).Encode(igroup)
		} else if r.URL.Path == "/api/protocols/san/igroups" {
			json.NewEncoder(w).Encode(igroupResponse)
		}
	}
}

func mockIgroupHrefLinkNumRecordsNil(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	igroupName := "igroup1"
	subsysUUID := "fakeUUID"
	igroup := models.Igroup{
		Name: &igroupName,
		UUID: &subsysUUID,
	}
	IgroupList := []*models.Igroup{&igroup}

	url := "/api/protocols/san/igroups/igroup"
	var hrefLink *models.IgroupResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.IgroupResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
	}

	igroupResponse := &models.IgroupResponse{
		IgroupResponseInlineRecords: IgroupList,
		Links:                       hrefLink,
	}

	if r.Method == "GET" {
		setHTTPResponseHeader(w, http.StatusOK)
		if r.URL.Path == "/api/protocols/san/igroups/igroup" {
			json.NewEncoder(w).Encode(igroup)
		} else if r.URL.Path == "/api/protocols/san/igroups" {
			json.NewEncoder(w).Encode(igroupResponse)
		}
	}
}

func TestOntapREST_IgroupListHref(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(hasNextLink bool, w http.ResponseWriter, r *http.Request)
		isNegativeTest  bool
		isErrorExpected bool
	}{
		{"PositiveTestCase", mockIgroupHrefLink, false, false},
		{"NumRecordsNilInResponse", mockIgroupHrefLinkNumRecordsNil, false, false},
		{"SecondGetRequestFailed", mockIgroupHrefLinkInternalError, false, true},
		{"BackendReturnError", mockInternalServerError, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := getHttpServer(test.isNegativeTest, test.mockFunction)
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			igroup, err := rs.IgroupList(ctx, "igroup", []string{""})
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get igroup")
				assert.Equal(t, "igroup1",
					*igroup.Payload.IgroupResponseInlineRecords[0].Name,
					"igroup name does not match")
			} else {
				assert.Error(t, err, "get the igroup")
			}
			server.Close()
		})
	}
}

func TestOntapREST_IgroupGet(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockIgroup, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			igroup, err := rs.IgroupGet(ctx, "igroup")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get igroup")
				assert.Equal(t, "igroup1", *igroup.Payload.Name, "igroup name does not match")
			} else {
				assert.Error(t, err, "get the igroup")
			}
			server.Close()
		})
	}
}

func TestOntapREST_IgroupGetByName(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockIgroup, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			igroup, err := rs.IgroupGetByName(ctx, "igroup", []string{""})
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get igroup")
				assert.Equal(t, "igroup1", *igroup.Name, "igroup name does not match")
			} else {
				assert.Error(t, err, "get the igroup")
			}
			server.Close()
		})
	}
}

func mockIgroupUUIDNil(w http.ResponseWriter, r *http.Request) {
	igroupResponse := getIgroup()
	igroupResponse.IgroupResponseInlineRecords[0].UUID = nil
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(igroupResponse)
}

func mockIgroupCreateFail(w http.ResponseWriter, r *http.Request) {
	igroupResponse := getIgroup()

	if r.Method == "GET" {
		setHTTPResponseHeader(w, http.StatusOK)
	} else {
		setHTTPResponseHeader(w, http.StatusInternalServerError)
	}
	json.NewEncoder(w).Encode(igroupResponse)
}

func TestOntapREST_IgroupAdd(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockIgroup, false},
		{"BackendReturnError", mockResourceNotFound, true},
		{"UUIDNil", mockIgroupUUIDNil, true},
		{"IgroupCreateFail", mockIgroupCreateFail, true},
		{"NumRecordsNilInResponse", mockIgroupNumRecordsNil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.IgroupAdd(ctx, "igroup", "iqn.1993-08.org.debian:01:9031309bbebd")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not add igroup")
			} else {
				assert.Error(t, err, "igroup added")
			}
			server.Close()
		})
	}
}

func TestOntapREST_IgroupRemove(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockIgroup, false},
		{"BackendReturnError", mockResourceNotFound, true},
		{"UUIDNil", mockIgroupUUIDNil, true},
		{"IgroupCreateFail", mockIgroupCreateFail, true},
		{"NumRecordsNilInResponse", mockIgroupNumRecordsNil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.IgroupRemove(ctx, "igroup", "iqn.1993-08.org.debian:01:9031309bbebd")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not remove igroup")
			} else {
				assert.Error(t, err, "igroup removed")
			}
			server.Close()
		})
	}
}

func TestOntapREST_IgroupDestroy(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockIgroup, false},
		{"BackendReturnError", mockResourceNotFound, true},
		{"UUIDNil", mockIgroupUUIDNil, false},
		{"IgroupCreateFail", mockIgroupCreateFail, true},
		{"NumRecordsNilInResponse", mockIgroupNumRecordsNil, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.IgroupDestroy(ctx, "igroup")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not delete igroup")
			} else {
				assert.Error(t, err, "igroup deleted")
			}
			server.Close()
		})
	}
}

func mockLunListResponse(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	lunInfo := getLunInfo(convert.ToPtr("lunAttr"))
	numRecords := int64(1)

	url := "/api/storage/luns"
	var hrefLink *models.LunResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.LunResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		hasNextLink = false
	}

	lunResponse := models.LunResponse{
		LunResponseInlineRecords: []*models.Lun{lunInfo},
		NumRecords:               &numRecords,
		Links:                    hrefLink,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	if r.URL.Path == "/api/storage/luns/fake-lunName" {
		json.NewEncoder(w).Encode(lunInfo)
	} else {
		json.NewEncoder(w).Encode(lunResponse)
	}
}

func mockLunListResponseInternalError(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	lunInfo := getLunInfo(convert.ToPtr("lunAttr"))
	numRecords := int64(1)

	url := "/api/storage/luns"
	var hrefLink *models.LunResponseInlineLinks
	sc := http.StatusInternalServerError
	if hasNextLink {
		hrefLink = &models.LunResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		sc = http.StatusOK
	}

	lunResponse := models.LunResponse{
		LunResponseInlineRecords: []*models.Lun{lunInfo},
		NumRecords:               &numRecords,
		Links:                    hrefLink,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(sc)
	json.NewEncoder(w).Encode(lunResponse)
}

func mockLunListResponseNumRecordsNil(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	lunInfo := getLunInfo(convert.ToPtr("lunAttr"))

	url := "/api/storage/luns"
	var hrefLink *models.LunResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.LunResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
	}

	lunResponse := models.LunResponse{
		LunResponseInlineRecords: []*models.Lun{lunInfo},
		Links:                    hrefLink,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	if r.URL.Path == "/api/storage/luns/fake-lunName" {
		json.NewEncoder(w).Encode(lunInfo)
	} else {
		json.NewEncoder(w).Encode(lunResponse)
	}
}

func TestOntapREST_LunListByPattern(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(hasNextLink bool, w http.ResponseWriter, r *http.Request)
		isNegativeTest  bool
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunListResponse, false, false},
		{"NumRecordsNilInResponse", mockLunListResponseNumRecordsNil, false, false},
		{"SecondGetRequestFail", mockLunListResponseInternalError, false, true},
		{"BackendReturnError", mockInternalServerError, false, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := getHttpServer(test.isNegativeTest, test.mockFunction)
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			lunResponse, err := rs.LunList(ctx, "*", []string{""})
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get LUN")
				assert.Equal(t, "fake-lunName",
					*lunResponse.Payload.LunResponseInlineRecords[0].Name, "lun name does not match")
			} else {
				assert.Error(t, err, "get the LUN by name")
			}
			server.Close()
		})
	}
}

func getLunInfo(lunAttr *string) *models.Lun {
	comment := "LUN for flexvol"
	igroup1 := "igroup1"
	logicalUnitNumber := int64(12345)
	size := int64(2147483648)
	qosPolicyName := "fake-qosPolicy"
	mapStatus := true
	volumeName := "fake-volume"
	createTime1 := strfmt.NewDateTime()
	enabled := false
	lunName := "fake-lunName"
	lunUUID := "fake-lunUUID"
	lunSerialNumber := "fake-serialNumber"
	lunState := "online"

	igroup := models.LunInlineLunMapsInlineArrayItemInlineIgroup{Name: &igroup1}
	lunMap := []*models.LunInlineLunMapsInlineArrayItem{
		{Igroup: &igroup, LogicalUnitNumber: &logicalUnitNumber},
	}

	lunAttrList := []*models.LunInlineAttributesInlineArrayItem{
		{Name: lunAttr},
	}
	space := models.LunInlineSpace{Size: &size}
	qosPolicy := models.LunInlineQosPolicy{Name: &qosPolicyName}
	status := models.LunInlineStatus{Mapped: &mapStatus, State: &lunState}
	location := &models.LunInlineLocation{
		Volume: &models.LunInlineLocationInlineVolume{
			Name: &volumeName,
		},
	}

	lun := models.Lun{
		Name:                &lunName,
		UUID:                &lunUUID,
		SerialNumber:        &lunSerialNumber,
		Status:              &status,
		Enabled:             &enabled,
		Comment:             &comment,
		Space:               &space,
		CreateTime:          &createTime1,
		Location:            location,
		QosPolicy:           &qosPolicy,
		LunInlineLunMaps:    lunMap,
		LunInlineAttributes: lunAttrList,
	}
	return &lun
}

func mockLunMapResponse(w http.ResponseWriter, r *http.Request) {
	initiatorGroup := "initiatorGroup"

	lunMapResponse := &models.LunMapResponse{
		NumRecords: convert.ToPtr(int64(1)),
		LunMapResponseInlineRecords: []*models.LunMap{
			{
				LogicalUnitNumber: nil,
				Igroup: &models.LunMapInlineIgroup{
					Name: convert.ToPtr(initiatorGroup),
					UUID: convert.ToPtr("fake-igroupUUID"),
				},
			},
		},
	}

	if r.Method == "POST" {
		setHTTPResponseHeader(w, http.StatusCreated)
		json.NewEncoder(w).Encode(lunMapResponse)
	} else {
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(lunMapResponse)
	}
}

func mockLunResponse(w http.ResponseWriter, r *http.Request) {
	lunInfo := getLunInfo(convert.ToPtr("lunAttr"))
	numRecords := int64(1)
	lunResponse := models.LunResponse{LunResponseInlineRecords: []*models.Lun{lunInfo}, NumRecords: &numRecords}
	switch r.Method {
	case "GET":
		if r.URL.Path == "/api/protocols/san/lun-maps" {
			mockLunMapResponse(w, r)
		} else {
			setHTTPResponseHeader(w, http.StatusOK)
			if r.URL.Path == "/api/storage/luns/fake-lunName" {
				json.NewEncoder(w).Encode(lunInfo)
			} else {
				json.NewEncoder(w).Encode(lunResponse)
			}
		}
	case "POST":
		if r.URL.Path == "/api/protocols/san/lun-maps" {
			mockLunMapResponse(w, r)
		} else {
			setHTTPResponseHeader(w, http.StatusCreated)
			json.NewEncoder(w).Encode(lunResponse)
		}
	case "DELETE", "PATCH":
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(nil)
	}
}

func mockLunResponseNumRecordsNil(w http.ResponseWriter, r *http.Request) {
	lunInfo := getLunInfo(convert.ToPtr("lunAttr"))
	lunResponse := models.LunResponse{LunResponseInlineRecords: []*models.Lun{lunInfo}}
	if r.Method == "GET" {
		if r.URL.Path == "/api/protocols/san/lun-maps" {
			mockLunMapResponse(w, r)
		} else {
			setHTTPResponseHeader(w, http.StatusOK)
			if r.URL.Path == "/api/storage/luns/fake-lunName" {
				json.NewEncoder(w).Encode(lunInfo)
			} else {
				json.NewEncoder(w).Encode(lunResponse)
			}
		}
	}
}

func mockLunResponseNumRecordsZero(w http.ResponseWriter, r *http.Request) {
	numRecords := int64(0)
	lunResponse := models.LunResponse{
		LunResponseInlineRecords: []*models.Lun{},
		NumRecords:               &numRecords,
	}
	if r.Method == "GET" {
		if r.URL.Path == "/api/protocols/san/lun-maps" {
			mockLunMapResponse(w, r)
		} else {
			setHTTPResponseHeader(w, http.StatusOK)
			if r.URL.Path == "/api/storage/luns/fake-lunName" {
				// LunGetByName calls LunList
				json.NewEncoder(w).Encode(lunResponse)
			} else {
				json.NewEncoder(w).Encode(lunResponse)
			}
		}
	}
}

func TestOntapREST_LunGetByName(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isResponseNil   bool
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunResponse, false, false},
		{"NumRecordsNilInResponse", mockLunResponseNumRecordsNil, true, true},
		{"NumRecordsZeroInResponse", mockLunResponseNumRecordsZero, false, true},
		{"BackendReturnError", mockResourceNotFound, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			lunResponse, err := rs.LunGetByName(ctx, "fake-lunName", []string{""})
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get LUN by name")
				if !test.isResponseNil {
					assert.Equal(t, "fake-lunName", *lunResponse.Name, "lun name does not match")
				}
			} else {
				assert.Error(t, err, "get the LUN by name")
			}
			server.Close()
		})
	}
}

func TestOntapREST_LunGet(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			lunResponse, err := rs.LunGet(ctx, "fake-lunName")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get LUN")
				assert.Equal(t, "fake-lunName", *lunResponse.Payload.Name, "lun name does not match")
			} else {
				assert.Error(t, err, "get the LUN")
			}
			server.Close()
		})
	}
}

func TestOntapREST_LunCloneCreate(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		lunPath         string
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunResponse, "/fake-lunName", false},
		{"BackendReturnError", mockResourceNotFound, "/fake-lunName", true},
		{"BackendReturnInvalidLunIQN", mockResourceNotFound, "failure_65dc2f4b_adbe_4ed3_8b73_6c61d5eac054", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.LunCloneCreate(ctx, test.lunPath, "/fake-lunName", int64(2147483648), "linux",
				QosPolicyGroup{Name: "qosPolicy", Kind: QosPolicyGroupKind})
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not create clone of the LUN")
			} else {
				assert.Error(t, err, "clone created")
			}
			server.Close()
		})
	}
}

func TestOntapREST_LunCreate(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		lunPath         string
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunResponse, "/fake-lunName", false},
		{"BackendReturnError", mockResourceNotFound, "/fake-lunName", true},
		{"BackendReturnInvalidLunIQN", mockResourceNotFound, "failure_65dc2f4b_adbe_4ed3_8b73_6c61d5eac054", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.LunCreate(ctx, test.lunPath, int64(2147483648), "linux",
				QosPolicyGroup{Name: "qosPolicy", Kind: QosPolicyGroupKind}, convert.ToPtr(false), convert.ToPtr(false))
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not create LUN")
			} else {
				assert.Error(t, err, "LUN created")
			}
			server.Close()
		})
	}
}

func TestOntapREST_LunDelete(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.LunDelete(ctx, "fake-lunName")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not delete LUN")
			} else {
				assert.Error(t, err, "LUN deleted")
			}
			server.Close()
		})
	}
}

func TestOntapREST_LunGetComment(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			comment, err := rs.LunGetComment(ctx, "fake-lunName")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get LUN comment")
				assert.Equal(t, "LUN for flexvol", comment, "lun comment does not match")
			} else {
				assert.Error(t, err, "get LUN comment")
			}
			server.Close()
		})
	}
}

func mockLunMapResponseCreateFailure(w http.ResponseWriter, r *http.Request) {
	setHTTPResponseHeader(w, http.StatusInternalServerError)
	json.NewEncoder(w).Encode(nil)
}

func mockLunResponseFailure(w http.ResponseWriter, r *http.Request) {
	lunInfo := getLunInfo(convert.ToPtr("lunAttr"))
	numRecords := int64(1)
	lunResponse := models.LunResponse{LunResponseInlineRecords: []*models.Lun{lunInfo}, NumRecords: &numRecords}
	switch r.Method {
	case "GET":
		if r.URL.Path == "/api/protocols/san/lun-maps" {
			mockLunMapResponse(w, r)
		} else {
			setHTTPResponseHeader(w, http.StatusOK)
			if r.URL.Path == "/api/storage/luns/fake-lunName" {
				json.NewEncoder(w).Encode(lunInfo)
			} else {
				json.NewEncoder(w).Encode(lunResponse)
			}
		}
	case "POST":
		if r.URL.Path == "/api/protocols/san/lun-maps" {
			mockLunMapResponseCreateFailure(w, r)
		} else {
			setHTTPResponseHeader(w, http.StatusCreated)
			json.NewEncoder(w).Encode(lunResponse)
		}
	case "DELETE", "PATCH":
		mockLunMapResponseCreateFailure(w, r)
	}
}

func mockLunResponseUUIDNil(w http.ResponseWriter, r *http.Request) {
	numRecords := int64(1)
	lunInfo := getLunInfo(convert.ToPtr("lunAttr"))
	lunInfo.UUID = nil
	lunInfo.Name = nil
	lunResponse := models.LunResponse{LunResponseInlineRecords: []*models.Lun{lunInfo}, NumRecords: &numRecords}
	if r.Method == "GET" {
		if r.URL.Path == "/api/protocols/san/lun-maps" {
			mockLunMapResponse(w, r)
		} else {
			setHTTPResponseHeader(w, http.StatusOK)
			if r.URL.Path == "/api/storage/luns/fake-lunName" {
				json.NewEncoder(w).Encode(lunInfo)
			} else {
				json.NewEncoder(w).Encode(lunResponse)
			}
		}
	}
}

func TestOntapREST_LunSetComment(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunResponse, false},
		{"NumRecordsNilInResponse", mockLunResponseNumRecordsNil, true},
		{"UUIDNilInResponse", mockLunResponseUUIDNil, true},
		{"BackendReturnError", mockResourceNotFound, true},
		{"modifyFailed", mockLunResponseFailure, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.LunSetComment(ctx, "/fake-lunName", "new_comment")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not update the LUN comment")
			} else {
				assert.Error(t, err, "update the LUN comment")
			}
			server.Close()
		})
	}
}

func mockLunResponseLunInlineAttributesNil(w http.ResponseWriter, r *http.Request) {
	numRecords := int64(1)
	lunInfo := getLunInfo(convert.ToPtr("lunAttr"))
	lunInfo.UUID = nil
	lunInfo.Name = nil
	lunInfo.LunInlineAttributes = nil
	lunResponse := models.LunResponse{LunResponseInlineRecords: []*models.Lun{lunInfo}, NumRecords: &numRecords}
	if r.Method == "GET" {
		if r.URL.Path == "/api/protocols/san/lun-maps" {
			mockLunMapResponse(w, r)
		} else {
			setHTTPResponseHeader(w, http.StatusOK)
			if r.URL.Path == "/api/storage/luns/fake-lunName" {
				json.NewEncoder(w).Encode(lunInfo)
			} else {
				json.NewEncoder(w).Encode(lunResponse)
			}
		}
	}
}

func TestOntapREST_LunGetAttribute(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
		{"NumRecordsNilInResponse", mockLunResponseNumRecordsNil, true},
		{"UUIDNilInResponse", mockLunResponseUUIDNil, false},
		{"InlineAttributesNil", mockLunResponseLunInlineAttributesNil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			attr, err := rs.LunGetAttribute(ctx, "/fake-lunName", "comment")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get LUN attribute")
				assert.Equal(t, "", attr, "lun attribute name is not empty")
			} else {
				assert.Error(t, err, "get LUN attribute")
			}
			server.Close()
		})
	}
}

func mockLunAttrNotExistsResponse(w http.ResponseWriter, r *http.Request) {
	lunInfo := getLunInfo(nil)
	numRecords := int64(1)
	lunResponse := models.LunResponse{LunResponseInlineRecords: []*models.Lun{lunInfo}, NumRecords: &numRecords}
	switch r.Method {
	case "GET":
		setHTTPResponseHeader(w, http.StatusOK)
		if r.URL.Path == "/api/storage/luns/fake-lunName" {
			json.NewEncoder(w).Encode(lunInfo)
		} else {
			json.NewEncoder(w).Encode(lunResponse)
		}
	case "POST":
		setHTTPResponseHeader(w, http.StatusCreated)
		json.NewEncoder(w).Encode(nil)
	}
}

func TestOntapREST_LunAttributeModify(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunResponse, false},
		{"LunAttrNotFound", mockLunAttrNotExistsResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
		{"UUIDNilInResponse", mockLunResponseUUIDNil, true},
		{"NumRecordsNilInResponse", mockLunResponseNumRecordsNil, true},
		{"modifyFailed", mockLunResponseFailure, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.LunSetAttribute(ctx, "/fake-lunName", "lunAttr", "lunAttr1")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not update LUN attribute")
			} else {
				assert.Error(t, err, "updated LUN attribute")
			}
			server.Close()
		})
	}
}

func TestOntapREST_LunSetQosPolicyGroup(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
		{"UUIDNilInResponse", mockLunResponseUUIDNil, true},
		{"NumRecordsNilInResponse", mockLunResponseNumRecordsNil, true},
		{"modifyFailed", mockLunResponseFailure, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.LunSetQosPolicyGroup(ctx, "/fake-lunName", "fake-qosPolicy")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not update the qos policy on LUN")
			} else {
				assert.Error(t, err, "qos policy updated on LUN")
			}
			server.Close()
		})
	}
}

func TestOntapREST_LunRename(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
		{"UUIDNilInResponse", mockLunResponseUUIDNil, true},
		{"NumRecordsNilInResponse", mockLunResponseNumRecordsNil, true},
		{"modifyFailed", mockLunResponseFailure, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.LunRename(ctx, "/fake-lunName", "/fake-NewlunName")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not rename the LUN")
			} else {
				assert.Error(t, err, "LUN renamed")
			}
			server.Close()
		})
	}
}

func mockLunResponseSuccessAsync(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/cluster/jobs/1234" {
		mockJobResponse(w, r)
	} else if r.Method == "PATCH" {
		mockRequestAccepted(w, r)
	} else {
		lunInfo := getLunInfo(convert.ToPtr("lunAttr"))
		numRecords := int64(1)
		lunResponse := models.LunResponse{
			LunResponseInlineRecords: []*models.Lun{lunInfo},
			NumRecords:               &numRecords,
		}

		r.Host = "127.0.0.1"
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(lunResponse)
	}
}

func mockJobResponseBadRequest(w http.ResponseWriter, r *http.Request) {
	jobId := strfmt.UUID("1234")
	jobStatus := models.JobStateSuccess
	jobLink := models.Job{
		UUID:  &jobId,
		State: &jobStatus,
		Error: &models.JobInlineError{
			Code:    convert.ToPtr("5376461"),
			Message: convert.ToPtr("mock error which resulted in job failure"),
		},
	}
	setHTTPResponseHeader(w, http.StatusBadRequest)
	json.NewEncoder(w).Encode(jobLink)
}

func TestOntapREST_LunRename_Async(t *testing.T) {
	tests := []struct {
		name            string
		newName         string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", "fake_valid_new_name", mockLunResponseSuccessAsync, false},
		{"NegativeTest", "fake-%invalid-new-name", mockJobResponseBadRequest, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.LunRename(ctx, "fake-oldLunName", test.newName)
			if !test.isErrorExpected {
				assert.NoError(t, err, "expected no error in LUN rename, but got one.")
			} else {
				assert.Error(t, err, "expected error in LUN rename, but got none.")
			}
			server.Close()
		})
	}
}

func TestOntapREST_LunMapInfo(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			lunMapInfo, err := rs.LunMapInfo(ctx, "fake-initiatorGroupName", "/fake-lunName")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the LUN map")
				assert.Equal(t, "initiatorGroup",
					*lunMapInfo.Payload.LunMapResponseInlineRecords[0].Igroup.Name,
					"initiator group name does not match")
			} else {
				assert.Error(t, err, "get the LUN map info")
			}
			server.Close()
		})
	}
}

func mockLunResponseInternalError(w http.ResponseWriter, r *http.Request) {
	lunInfo := getLunInfo(convert.ToPtr("lunAttr"))
	lunResponse := models.LunResponse{LunResponseInlineRecords: []*models.Lun{lunInfo}}
	if r.Method == "GET" {
		if r.URL.Path == "/api/protocols/san/lun-maps" {
			mockLunMapResponse(w, r)
		} else {
			setHTTPResponseHeader(w, http.StatusInternalServerError)
			if r.URL.Path == "/api/storage/luns/fake-lunName" {
				json.NewEncoder(w).Encode(lunInfo)
			} else {
				json.NewEncoder(w).Encode(lunResponse)
			}
		}
	}
}

func mockLunMapResponseNumRecordsNil(w http.ResponseWriter, r *http.Request) {
	initiatorGroup := "initiatorGroup"

	lunMapResponse := &models.LunMapResponse{
		LunMapResponseInlineRecords: []*models.LunMap{
			{
				LogicalUnitNumber: nil,
				Igroup: &models.LunMapInlineIgroup{
					Name: convert.ToPtr(initiatorGroup),
					UUID: convert.ToPtr("fake-igroupUUID"),
				},
			},
		},
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(lunMapResponse)
}

func mockLunMapResponseNumRecordsZero(w http.ResponseWriter, r *http.Request) {
	initiatorGroup := "initiatorGroup"

	lunMapResponse := &models.LunMapResponse{
		LunMapResponseInlineRecords: []*models.LunMap{
			{
				LogicalUnitNumber: nil,
				Igroup: &models.LunMapInlineIgroup{
					Name: convert.ToPtr(initiatorGroup),
					UUID: convert.ToPtr("fake-igroupUUID"),
				},
			},
		},
		NumRecords: convert.ToPtr(int64(0)),
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(lunMapResponse)
}

func mockLunMapResponseIgroupUUIDNil(w http.ResponseWriter, r *http.Request) {
	initiatorGroup := "initiatorGroup"

	lunMapResponse := &models.LunMapResponse{
		LunMapResponseInlineRecords: []*models.LunMap{
			{
				LogicalUnitNumber: nil,
				Igroup: &models.LunMapInlineIgroup{
					Name: convert.ToPtr(initiatorGroup),
					UUID: nil,
				},
			},
		},
		NumRecords: convert.ToPtr(int64(1)),
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(lunMapResponse)
}

func TestOntapREST_LunUnmap(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
		{"LunMapNumRecordsNilInResponse", mockLunMapResponseNumRecordsNil, true},
		{"LunMapNumRecordsZero", mockLunMapResponseNumRecordsZero, false},
		{"IgroupUUIDNil", mockLunMapResponseIgroupUUIDNil, true},
		{"LunNumRecordsNilInResponse", mockLunResponseNumRecordsNil, true},
		{"UUIDNilInResponse", mockLunResponseUUIDNil, true},
		{"UnmapFailed", mockLunResponseFailure, true},
		{"BackendCouldNotUnmapLun", mockLunResponseInternalError, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.LunUnmap(ctx, "fake-initiatorGroupName", "/fake-lunName")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not unmap LUN")
			} else {
				assert.Error(t, err, "unmap the LUN")
			}
			server.Close()
		})
	}
}

func TestOntapREST_LunMap(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
		{"NumRecordsNilInResponse", mockLunResponseNumRecordsNil, true},
		{"LunMapFailed", mockLunResponseFailure, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			lumMapResponse, err := rs.LunMap(ctx, "fake-initiatorGroupName", "/fake-lunName", 0)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the LUN map")
				assert.Equal(t, "initiatorGroup",
					*lumMapResponse.Payload.LunMapResponseInlineRecords[0].Igroup.Name,
					"initiator group name does not match")
			} else {
				assert.Error(t, err, "get the LUN map")
			}
			server.Close()
		})
	}
}

func TestOntapREST_LunMapList(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			lumMapResponse, err := rs.LunMapList(ctx, "fake-initiatorGroupName", "/dev/sda", []string{""})
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the LUN map")
				assert.Equal(t, "initiatorGroup",
					*lumMapResponse.Payload.LunMapResponseInlineRecords[0].Igroup.Name,
					"initiator group name does not match")
			} else {
				assert.Error(t, err, "get the LUN map list")
			}
			server.Close()
		})
	}
}

func mockLunMapReportingNode(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/storage/luns" {
		mockLunResponse(w, r)
	} else if r.URL.Path == "/api/protocols/san/lun-maps/fake-lunUUID/fakeUUID/reporting-nodes" {
		lunMapResp := models.LunMapReportingNodeResponse{
			LunMapReportingNodeResponseInlineRecords: []*models.LunMapReportingNode{
				{Name: convert.ToPtr("fake-lunMap")},
			},
		}
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(lunMapResp)
	} else {
		mockIgroup(w, r)
	}
}

func mockLunMapReportingNodeFailure(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/storage/luns" {
		mockLunResponse(w, r)
	} else if r.URL.Path == "/api/protocols/san/lun-maps/fake-lunUUID/fakeUUID/reporting-nodes" {
		lunMapResp := models.LunMapReportingNodeResponse{
			LunMapReportingNodeResponseInlineRecords: []*models.LunMapReportingNode{
				{Name: convert.ToPtr("fake-lunMap")},
			},
		}
		setHTTPResponseHeader(w, http.StatusInternalServerError)
		json.NewEncoder(w).Encode(lunMapResp)
	} else {
		mockIgroup(w, r)
	}
}

func mockIgroupInternalError(w http.ResponseWriter, r *http.Request) {
	igroupResponse := getIgroup()
	igroupResponse.IgroupResponseInlineRecords[0].UUID = nil
	setHTTPResponseHeader(w, http.StatusInternalServerError)
	json.NewEncoder(w).Encode(igroupResponse)
}

func mockLunMapReportingNodeIgroupFailure(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/storage/luns" {
		mockLunResponse(w, r)
	} else {
		mockIgroupInternalError(w, r)
	}
}

func mockLunMapReportingNodeIgroupUUIDNil(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/storage/luns" {
		mockLunResponse(w, r)
	} else {
		mockIgroupUUIDNil(w, r)
	}
}

func mockLunMapReportingNodeIgroupNumRecordsNil(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/storage/luns" {
		mockLunResponse(w, r)
	} else {
		mockIgroupNumRecordsNil(w, r)
	}
}

func TestOntapREST_GetLunMapReportingNodes(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunMapReportingNode, false},
		{"BackendReturnError", mockResourceNotFound, true},
		{"LunNumRecordsNilInResponse", mockLunResponseNumRecordsNil, true},
		{"UUIDNilInResponse", mockLunResponseUUIDNil, true},
		{"LunMapReportingNodeFailure", mockLunMapReportingNodeFailure, true},
		{"LunMapReportingNodeIgroupFailure", mockLunMapReportingNodeIgroupFailure, true},
		{"LunMapReportingNodeIgroupNumRecordsNil", mockLunMapReportingNodeIgroupNumRecordsNil, true},
		{"LunMapReportingNodeIgroupUUIDNil", mockLunMapReportingNodeIgroupUUIDNil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			lumMapResponse, err := rs.LunMapGetReportingNodes(ctx, "fake-initiatorGroupName", "/dev/sda")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the reporting node")
				assert.Equal(t, 1, len(lumMapResponse), "mapped lun count is not equal to one")
			} else {
				assert.Error(t, err, "get the reporting node")
			}
			server.Close()
		})
	}
}

func mockLunResponseSizeNil(w http.ResponseWriter, r *http.Request) {
	lunInfo := getLunInfo(convert.ToPtr("lunAttr"))
	lunInfo.Space.Size = nil
	lunResponse := models.LunResponse{LunResponseInlineRecords: []*models.Lun{lunInfo}, NumRecords: convert.ToPtr(int64(1))}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(lunResponse)
}

func mockLunResponseSpaceNil(w http.ResponseWriter, r *http.Request) {
	lunInfo := getLunInfo(convert.ToPtr("lunAttr"))
	lunInfo.Space = nil
	lunResponse := models.LunResponse{LunResponseInlineRecords: []*models.Lun{lunInfo}, NumRecords: convert.ToPtr(int64(1))}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(lunResponse)
}

func TestOntapREST_GetLunSize(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
		{"NumRecordsNilInResponse", mockLunResponseNumRecordsNil, true},
		{"LunResponseSpaceNil", mockLunResponseSpaceNil, true},
		{"LunResponseSizeNil", mockLunResponseSizeNil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			lunSize, err := rs.LunSize(ctx, "fake-lunName")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the LUN size")
				assert.Equal(t, 2147483648, lunSize, "lun size does not match")
			} else {
				assert.Error(t, err, "get the LUN size")
			}
			server.Close()
		})
	}
}

func TestOntapREST_SetLunSize(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
		{"NumRecordsNilInResponse", mockLunResponseNumRecordsNil, true},
		{"UUIDNilInResponse", mockLunResponseUUIDNil, true},
		{"SetLunSizeFailed", mockLunResponseFailure, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			lunSize, err := rs.LunSetSize(ctx, "fake-lunName", "3147483648")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not update LUN size")
				assert.Equal(t, uint64(3147483648), lunSize, "lun size does not match")
			} else {
				assert.Error(t, err, "update the LUN size")
			}
			server.Close()
		})
	}
}

func getNetworkIpInterface(hasNextLink bool) *models.IPInterfaceResponse {
	ipAddress := models.IPAddress("1.1.1.1")
	ipFamily := models.IPAddressFamily("ipv4")

	node := models.IPInterfaceInlineLocationInlineNode{
		Name: convert.ToPtr("node1"),
	}

	url := ""
	var hrefLink *models.IPInterfaceResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.IPInterfaceResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		hasNextLink = false
	}

	return &models.IPInterfaceResponse{
		IPInterfaceResponseInlineRecords: []*models.IPInterface{
			{
				Location: &models.IPInterfaceInlineLocation{Node: &node},
				IP:       &models.IPInfo{Address: &ipAddress, Family: &ipFamily},
				State:    convert.ToPtr("up"),
			},
		},
		Links: hrefLink,
	}
}

func mockNetworkIpInterfaceList(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	NwIpInterface := getNetworkIpInterface(hasNextLink)
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(NwIpInterface)
}

func mockNetworkIpInterfaceListInternalError(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	NwIpInterface := getNetworkIpInterface(hasNextLink)
	sc := http.StatusInternalServerError

	if hasNextLink {
		sc = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(sc)
	json.NewEncoder(w).Encode(NwIpInterface)
}

func mockNetworkIpInterfaceListNumRecordsNil(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	nwIpInterface := getNetworkIpInterface(hasNextLink)
	nwIpInterface.NumRecords = nil
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(nwIpInterface)
}

func TestOntapREST_NetworkIPInterfacesList(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(hasNextLink bool, w http.ResponseWriter, r *http.Request)
		isNegativeTest  bool
		isErrorExpected bool
	}{
		{"PositiveTest", mockNetworkIpInterfaceList, false, false},
		{"NumRecordsNilInResponse", mockNetworkIpInterfaceListNumRecordsNil, false, false},
		{"SecondGetRequestFail", mockNetworkIpInterfaceListInternalError, false, true},
		{"BackendReturnError", mockInternalServerError, false, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := getHttpServer(test.isNegativeTest, test.mockFunction)
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			networkIPInterfaces, err := rs.NetworkIPInterfacesList(ctx)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get ip interface list")
				assert.Equal(t, "node1",
					*networkIPInterfaces.Payload.IPInterfaceResponseInlineRecords[0].Location.Node.Name,
					"node name does not match")
			} else {
				assert.Error(t, err, "get the ip interface list")
			}
			server.Close()
		})
	}
}

func mockNetworkIpInterface(w http.ResponseWriter, r *http.Request) {
	NwIpInterface := getNetworkIpInterface(false)
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(NwIpInterface)
}

func TestOntapREST_NetworkInterfaceGetDataLIFs(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		protocol        string
		isErrorExpected bool
	}{
		{"ProtocolTCP", mockNetworkIpInterface, "tcp", false},
		{"ProtocolEmpty", mockNetworkIpInterface, "", true},
		{"BackendReturnError", mockResourceNotFound, "tcp", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			dataLifs, err := rs.NetInterfaceGetDataLIFs(ctx, test.protocol)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get data lifs")
				assert.Equal(t, "1.1.1.1", dataLifs[0], "data lifs does not match")
			} else {
				assert.Error(t, err, "get the data lifs")
			}
			server.Close()
		})
	}
}

func mockJobResponse(w http.ResponseWriter, r *http.Request) {
	jobId := strfmt.UUID("1234")
	jobStatus := models.JobStateSuccess
	jobLink := models.Job{UUID: &jobId, State: &jobStatus}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(jobLink)
}

func mockSvmPeerResponse(w http.ResponseWriter, r *http.Request) {
	svmPeerResponse := models.SvmPeerResponse{
		SvmPeerResponseInlineRecords: []*models.SvmPeer{
			{
				Peer: &models.SvmPeerInlinePeer{
					Svm: &models.SvmPeerInlinePeerInlineSvm{Name: convert.ToPtr("svm1")},
				},
			},
		},
	}
	if r.URL.Path == "/api/cluster/jobs/1234" {
		mockJobResponse(w, r)
	} else {
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(svmPeerResponse)
	}
}

func TestOntapRest_GetPeeredVservers(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"GetPeerVserver_Success", mockSvmPeerResponse, false},
		{"GetPeerVserver_Fail", mockGetSVMError, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			svmPeerName, err := rs.GetPeeredVservers(ctx)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get peer vserver")
				assert.Equal(t, "svm1", svmPeerName[0])
			} else {
				assert.Error(t, err, "get the peer vserver")
			}
			server.Close()
		})
	}
}

func mockIsVserverInSVMDR(w http.ResponseWriter, r *http.Request) {
	snapMirrorRelationshipResponse := &models.SnapmirrorRelationshipResponse{
		SnapmirrorRelationshipResponseInlineRecords: []*models.SnapmirrorRelationship{
			{
				Destination: &models.SnapmirrorEndpoint{
					Path: convert.ToPtr("svm0:"),
					Svm: &models.SnapmirrorEndpointInlineSvm{
						Name: convert.ToPtr("svm0"),
					},
				},
				Source: &models.SnapmirrorSourceEndpoint{
					Path: convert.ToPtr("svm1:"),
					Svm: &models.SnapmirrorSourceEndpointInlineSvm{
						Name: convert.ToPtr("svm1"),
					},
				},
				UUID: convert.ToPtr(strfmt.UUID("1")),
			},
			{
				Source: &models.
					SnapmirrorSourceEndpoint{Svm: &models.SnapmirrorSourceEndpointInlineSvm{}},
			},
			{
				Source: &models.
					SnapmirrorSourceEndpoint{Path: convert.ToPtr("svm0:")},
			},
			{},
		},
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(snapMirrorRelationshipResponse)
}

func TestOntapRest_IsVserverDRDestination(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"CheckVserverDestination_Success", mockIsVserverInSVMDR, false},
		{"CheckVserverDestination_Fail", mockGetSVMError, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			isVserverDRDestination, err := rs.IsVserverDRDestination(ctx)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not verify svm on DR destination")
				assert.Equal(t, true, isVserverDRDestination)
			} else {
				assert.Error(t, err, "verified svm on DR destination")
			}
			server.Close()
		})
	}
}

func TestOntapRest_IsVserverDRSource(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"CheckVserverInSource_Success", mockIsVserverInSVMDR, false},
		{"CheckVserverInSource_Fail", mockGetSVMError, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			isVserverDRSource, err := rs.IsVserverDRSource(ctx)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not verify svm on DR destination")
				assert.Equal(t, true, isVserverDRSource)
			} else {
				assert.Error(t, err, "verified svm on DR destination")
			}
			server.Close()
		})
	}
}

func TestOntapRest_IsVserverInSVMDR(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"IsInSVMDR_Success", mockIsVserverInSVMDR, false},
		{"IsInSVMDR_Fail", mockGetSVMError, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			isVserverInSVMDR := rs.IsVserverInSVMDR(ctx)
			if !test.isErrorExpected {
				assert.Equal(t, true, isVserverInSVMDR)
			} else {
				assert.Equal(t, false, isVserverInSVMDR)
			}
			server.Close()
		})
	}
}

func mockSVMListNumRecordsNil(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	svmUUID := "1234"
	svmName := "svm0"

	svm := models.Svm{
		UUID:  &svmUUID,
		Name:  &svmName,
		State: convert.ToPtr("running"),
		SvmInlineAggregates: []*models.SvmInlineAggregatesInlineArrayItem{
			{Name: convert.ToPtr("aggr1")},
		},
	}

	url := "/api/svm/svms"
	var hrefLink *models.SvmResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.SvmResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
	}

	svmResponse := models.SvmResponse{
		SvmResponseInlineRecords: []*models.Svm{&svm},
		Links:                    hrefLink,
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(svmResponse)
}

func mockSvmPeerResponseNumRecordsNil(w http.ResponseWriter, r *http.Request) {
	svmPeerResponse := models.SvmPeerResponse{}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(svmPeerResponse)
}

func TestOntapRest_IsVserverDRCapable(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		response        bool
		isErrorExpected bool
	}{
		{"IsVserverDRCapable_Success", mockSvmPeerResponse, true, false},
		{"NumberOfRecordsFieldNil", mockSvmPeerResponseNumRecordsNil, false, false},
		{"GettingErrorFromBackend", mockGetSVMError, false, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			isVserverDRCapable, err := rs.IsVserverDRCapable(ctx)
			if !test.isErrorExpected {
				assert.NoError(t, err, "vserver is not DR capable")
				assert.Equal(t, test.response, isVserverDRCapable)
			} else {
				assert.Error(t, err, "vserver is DR capable")
			}
			server.Close()
		})
	}
}

func mockRequestAccepted(w http.ResponseWriter, r *http.Request) {
	jobId := strfmt.UUID("1234")
	jobLink := models.JobLink{UUID: &jobId}
	jobResponse := models.JobLinkResponse{Job: &jobLink}
	setHTTPResponseHeader(w, http.StatusAccepted)
	json.NewEncoder(w).Encode(jobResponse)
}

func mockSnapshotCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		mockRequestAccepted(w, r)
	} else if r.URL.Path == "/api/cluster/jobs/1234" {
		mockJobResponse(w, r)
	}
}

func mockSnapshotResourceNotFound(w http.ResponseWriter, r *http.Request) {
	setHTTPResponseHeader(w, http.StatusNotFound)
	json.NewEncoder(w).Encode("")
}

func TestSnapshotCreateAndWait(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockSnapshotCreate, false},
		{"CreationFailedOnBackend", mockSnapshotResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.SnapshotCreateAndWait(ctx, "fakeUUID", "fakeSnapshot")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not create the snapshot")
			} else {
				assert.Error(t, err, "snapshot created")
			}
			server.Close()
		})
	}
}

func getSnapshot() *models.Snapshot {
	snapshotName := "fake-snapshot"
	snapshotUUID := "fake-snapshotUUID"
	createTime1 := strfmt.NewDateTime()

	snapshot := models.Snapshot{Name: &snapshotName, CreateTime: &createTime1, UUID: &snapshotUUID}
	return &snapshot
}

func mockSnapshot(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	url := "/api/storage/volume"
	var hrefLink *models.SnapshotResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.SnapshotResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		hasNextLink = false
	}

	snapShot := getSnapshot()
	numRecords := int64(1)
	volumeResponse := models.SnapshotResponse{
		SnapshotResponseInlineRecords: []*models.Snapshot{snapShot},
		NumRecords:                    &numRecords,
		Links:                         hrefLink,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(volumeResponse)
}

func mockSnapshotNumRecordsNil(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	url := "/api/storage/volume"
	var hrefLink *models.SnapshotResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.SnapshotResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		hasNextLink = false
	}

	snapShot := getSnapshot()
	volumeResponse := models.SnapshotResponse{
		SnapshotResponseInlineRecords: []*models.Snapshot{snapShot},
		Links:                         hrefLink,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(volumeResponse)
}

func mockSnapshotInternalError(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	url := "/api/storage/volume"
	var hrefLink *models.SnapshotResponseInlineLinks
	sc := http.StatusInternalServerError
	if hasNextLink {
		hrefLink = &models.SnapshotResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		sc = http.StatusOK
	}

	snapShot := getSnapshot()
	numRecords := int64(1)
	volumeResponse := models.SnapshotResponse{
		SnapshotResponseInlineRecords: []*models.Snapshot{snapShot},
		NumRecords:                    &numRecords,
		Links:                         hrefLink,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(sc)
	json.NewEncoder(w).Encode(volumeResponse)
}

func TestOntapREST_SnapshotList(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(hasNextLink bool, w http.ResponseWriter, r *http.Request)
		isNegativeTest  bool
		isErrorExpected bool
	}{
		{"PositiveTest", mockSnapshot, false, false},
		{"ErrorInFetchingHrefLink", mockSnapshotInternalError, false, true},
		{"NumRecordsFieldInResponseIsNil", mockSnapshotNumRecordsNil, false, false},
		{"backendReturnError", mockInternalServerError, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := getHttpServer(test.isNegativeTest, test.mockFunction)
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			snapshot, err := rs.SnapshotList(ctx, "fakeUUID")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the snapshot")
				assert.Equal(t, "fake-snapshot", *snapshot.Payload.SnapshotResponseInlineRecords[0].Name)
			} else {
				assert.Error(t, err, "get the snapshot")
			}
			server.Close()
		})
	}
}

func mockSnapshotList(w http.ResponseWriter, r *http.Request) {
	snapShot := getSnapshot()
	numRecords := int64(1)
	snapshotResponse := models.SnapshotResponse{
		SnapshotResponseInlineRecords: []*models.Snapshot{snapShot},
		NumRecords:                    &numRecords,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(snapshotResponse)
}

func TestOntapREST_SnapshotListByName(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockSnapshotList, false},
		{"backendReturnError", mockSnapshotResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			snapshot, err := rs.SnapshotListByName(ctx, "fakeUUID", "fake-snapshot")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the snapshot")
				assert.Equal(t, "fake-snapshot", *snapshot.Payload.SnapshotResponseInlineRecords[0].Name)
			} else {
				assert.Error(t, err, "get the snapshot")
			}
			server.Close()
		})
	}
}

func mockSnapshotGet(w http.ResponseWriter, r *http.Request) {
	snapShot := getSnapshot()

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(snapShot)
}

func mockGetVolumeResponseAccepted(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/cluster/jobs/1234" {
		mockJobResponse(w, r)
	} else {
		switch r.Method {
		case "PATCH", "DELETE":
			mockRequestAccepted(w, r)
		default:
			volume := &models.Volume{
				UUID: convert.ToPtr("fakeUUID"),
				Name: convert.ToPtr("fakeName"),
			}
			numRecords := int64(1)
			volumeResponse := models.VolumeResponse{
				VolumeResponseInlineRecords: []*models.Volume{volume},
				NumRecords:                  &numRecords,
			}

			setHTTPResponseHeader(w, http.StatusOK)
			json.NewEncoder(w).Encode(volumeResponse)
		}
	}
}

func TestOntapREST_SnapshotGet(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockSnapshotGet, false},
		{"BackendReturnError", mockSnapshotResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			snapshot, err := rs.SnapshotGet(ctx, "fakeUUID", "fake-snapshotUUID")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the snapshot")
				assert.Equal(t, "fake-snapshot", *snapshot.Payload.Name)
			} else {
				assert.Error(t, err, "get the snapshot")
			}
			server.Close()
		})
	}
}

func mockSnapshotListInvalidNumRecords(w http.ResponseWriter, r *http.Request) {
	snapShot := getSnapshot()
	snapshotResponse := models.SnapshotResponse{
		SnapshotResponseInlineRecords: []*models.Snapshot{snapShot},
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(snapshotResponse)
}

func TestOntapREST_SnapshotGetByName(t *testing.T) {
	tests := []struct {
		name          string
		mockFunction  func(w http.ResponseWriter, r *http.Request)
		isResponseNil bool
	}{
		{"PositiveTest", mockSnapshotList, false},
		{"BackendReturnError", mockSnapshotListInvalidNumRecords, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			snapshot, err := rs.SnapshotGetByName(ctx, "fakeUUID", "fake-snapshot")
			assert.NoError(t, err, "could not get the snapshot by name")
			if !test.isResponseNil {
				assert.Equal(t, "fake-snapshot", *snapshot.Name)
			}
			server.Close()
		})
	}
}

func TestOntapREST_SnapshotDelete(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockSnapshotResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			snapshot, err := rs.SnapshotDelete(ctx, "fakeUUID", "fake-snapshot")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not delete snapshot")
				assert.Equal(t, strfmt.UUID("1234"), *snapshot.Payload.Job.UUID)
			} else {
				assert.Error(t, err, "snapshot deleted")
			}
			server.Close()
		})
	}
}

func TestOntapREST_SnapshotRestoreVolume(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockSnapshotResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.SnapshotRestoreVolume(ctx, "fakeSnapshot", "fakeVolume")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not restore snapshot")
			} else {
				assert.Error(t, err, "snapshot restored")
			}
			server.Close()
		})
	}
}

func TestOntapREST_SnapshotRestoreFlexgroup(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockSnapshotResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.SnapshotRestoreFlexgroup(ctx, "fakeSnapshot", "fakeVolume")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not restore snapshot")
			} else {
				assert.Error(t, err, "snapshot restored")
			}
			server.Close()
		})
	}
}

func TestOntapREST_VolumeListAllBackedBySnapshot(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			_, err := rs.VolumeListAllBackedBySnapshot(ctx, "fakeVolume", "fakeSnapshot")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get volume backend by snapshot")
			} else {
				assert.Error(t, err, "get the volume backend by snapshot")
			}
			server.Close()
		})
	}
}

func TestOntapREST_VolumeCloneCreate(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockRequestAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			volume, err := rs.VolumeCloneCreate(ctx, "fakeClone", "fakeVolume", "fakeSnapshot")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not create clone of a volume")
				assert.Equal(t, strfmt.UUID("1234"), *volume.Payload.Job.UUID)
			} else {
				assert.Error(t, err, "clone created")
			}
			server.Close()
		})
	}
}

func mockRequestAcceptedJobFailed(w http.ResponseWriter, r *http.Request) {
	jobResponse := models.JobLinkResponse{}
	setHTTPResponseHeader(w, http.StatusAccepted)
	json.NewEncoder(w).Encode(jobResponse)
}

func TestOntapREST_VolumeCloneCreateAsync(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockRequestAcceptedJobFailed, true},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.VolumeCloneCreateAsync(ctx, "fakeClone", "fakeVolume", "fakeSnapshot")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not create clone of a volume")
			} else {
				assert.Error(t, err, "clone created")
			}
			server.Close()
		})
	}
}

func mockJobResponseStateNil(w http.ResponseWriter, r *http.Request) {
	jobId := strfmt.UUID("1234")
	jobLink := models.Job{UUID: &jobId}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(jobLink)
}

func mockJobResponseInternalError(w http.ResponseWriter, r *http.Request) {
	jobId := strfmt.UUID("1234")
	jobLink := models.Job{UUID: &jobId}
	setHTTPResponseHeader(w, http.StatusInternalServerError)
	json.NewEncoder(w).Encode(jobLink)
}

func mockJobResponseJobStateFailure(w http.ResponseWriter, r *http.Request) {
	jobId := strfmt.UUID("1234")
	jobLink := models.Job{UUID: &jobId, State: convert.ToPtr(models.JobStateFailure)}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(jobLink)
}

func mockJobResponseJobStatePaused(w http.ResponseWriter, r *http.Request) {
	jobId := strfmt.UUID("1234")
	jobLink := models.Job{UUID: &jobId, State: convert.ToPtr(models.JobStatePaused)}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(jobLink)
}

func mockJobResponseJobStateRunning(w http.ResponseWriter, r *http.Request) {
	jobId := strfmt.UUID("1234")
	jobLink := models.Job{UUID: &jobId, State: convert.ToPtr(models.JobStateRunning)}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(jobLink)
}

func mockJobResponseJobStateQueued(w http.ResponseWriter, r *http.Request) {
	jobId := strfmt.UUID("1234")
	jobLink := models.Job{UUID: &jobId, State: convert.ToPtr(models.JobStateQueued)}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(jobLink)
}

func mockJobResponseInvalidState(w http.ResponseWriter, r *http.Request) {
	jobId := strfmt.UUID("1234")
	jobLink := models.Job{UUID: &jobId, State: convert.ToPtr("InvalidState")}
	sc := http.StatusOK
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(sc)
	json.NewEncoder(w).Encode(jobLink)
}

func TestOntapREST_IsJobFinished(t *testing.T) {
	payload1 := &models.JobLinkResponse{}
	payload2 := &models.JobLinkResponse{Job: &models.JobLink{}}

	jobId := strfmt.UUID("1234")
	jobLink := models.JobLink{UUID: &jobId}
	jobResponse := models.JobLinkResponse{Job: &jobLink}

	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		payload         *models.JobLinkResponse
		isErrorExpected bool
		response        bool
	}{
		{"ResponsePayloadNil", mockJobResponse, nil, true, false},
		{"ResponsePayloadEmpty", mockJobResponse, payload1, true, false},
		{"ResponsePayloadJobLinkEmpty", mockJobResponse, payload2, true, false},
		{"ResponseStatusNil", mockJobResponseStateNil, &jobResponse, true, false},
		{"PositiveTest", mockJobResponse, &jobResponse, false, true},
		{"ResponseStatusFailure", mockJobResponseJobStateFailure, &jobResponse, false, true},
		{"ResponseStatusQueued", mockJobResponseJobStateQueued, &jobResponse, false, false},
		{"ResponseStatusPaused", mockJobResponseJobStatePaused, &jobResponse, false, false},
		{"ResponseStatusRunning", mockJobResponseJobStateRunning, &jobResponse, false, false},
		{"ResponseStatusInternalError", mockJobResponseInternalError, &jobResponse, true, false},
		{"ResponseStatusInvalidState", mockJobResponseInvalidState, &jobResponse, true, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			isJobFinish, err := rs.IsJobFinished(ctx, test.payload)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get job status")
				assert.Equal(t, test.response, isJobFinish)
			} else {
				assert.Error(t, err, "get the job status")
			}
			server.Close()
		})
	}
}

func TestOntapREST_PollJobStatus(t *testing.T) {
	payload1 := &models.JobLinkResponse{}
	payload2 := &models.JobLinkResponse{Job: &models.JobLink{}}

	jobId := strfmt.UUID("1234")
	jobLink := models.JobLink{UUID: &jobId}
	jobResponse := models.JobLinkResponse{Job: &jobLink}

	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		payload         *models.JobLinkResponse
		isErrorExpected bool
	}{
		{"PositiveTest", mockJobResponse, &jobResponse, false},
		{"PayloadEmpty", mockJobResponse, payload1, true},
		{"JobObjectEmptyInPayload", mockJobResponse, payload2, true},
		{"JabResponseFailure", mockJobResponseJobStateFailure, &jobResponse, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.PollJobStatus(ctx, test.payload)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get job status")
			} else {
				assert.Error(t, err, "get the job status")
			}
			server.Close()
		})
	}
}

func mockAggregateListResponse(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	aggrResponse := getAggregateResponse(hasNextLink)
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(aggrResponse)
}

func mockAggregateListResponseInternalError(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	aggrResponse := getAggregateResponse(hasNextLink)
	sc := http.StatusInternalServerError
	if hasNextLink {
		sc = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(sc)
	json.NewEncoder(w).Encode(aggrResponse)
}

func getAggregateResponse(hasNextLink bool) *models.AggregateResponse {
	size := int64(3221225472)
	usedSize := int64(2147483648)

	aggrSpace := models.AggregateInlineSpace{
		BlockStorage: &models.AggregateInlineSpaceInlineBlockStorage{
			Size: &size,
			Used: &usedSize,
		},
	}
	aggregate := models.Aggregate{
		Name: convert.ToPtr("aggr1"),
		BlockStorage: &models.AggregateInlineBlockStorage{
			Primary: &models.AggregateInlineBlockStorageInlinePrimary{DiskType: convert.ToPtr("fc")},
		},
		Space: &aggrSpace,
	}
	url := ""
	var hrefLink *models.AggregateResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.AggregateResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		hasNextLink = false
	}

	return &models.AggregateResponse{
		AggregateResponseInlineRecords: []*models.Aggregate{&aggregate},
		NumRecords:                     convert.ToPtr(int64(1)),
		Links:                          hrefLink,
	}
}

func mockAggregateListResponseNumRecordsNil(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	aggrResponse := getAggregateResponse(hasNextLink)
	aggrResponse.NumRecords = nil
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(aggrResponse)
}

func TestOntapREST_AggregateList(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(hasNextLink bool, w http.ResponseWriter, r *http.Request)
		isNegativeTest  bool
		isErrorExpected bool
	}{
		{"PositiveTest", mockAggregateListResponse, false, false},
		{"NumOfRecordsFieldIsNil", mockAggregateListResponseNumRecordsNil, false, false},
		{"BackendReturnErrorForSecondHrefLink", mockAggregateListResponseInternalError, false, true},
		{"BackendReturnError", mockInternalServerError, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := getHttpServer(test.isNegativeTest, test.mockFunction)
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			aggrList, err := rs.AggregateList(ctx, "aggr1", []string{""})

			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the aggregate list")
				assert.Equal(t, "aggr1", *aggrList.Payload.AggregateResponseInlineRecords[0].Name)
			} else {
				assert.Error(t, err, "get the svm")
			}
			server.Close()
		})
	}
}

func mockSVMList(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	svmUUID := "1234"
	svmName := "svm0"

	newSvm := models.Svm{
		UUID:  &svmUUID,
		Name:  &svmName,
		State: convert.ToPtr("running"),
		SvmInlineAggregates: []*models.SvmInlineAggregatesInlineArrayItem{
			{Name: convert.ToPtr("aggr1")},
		},
	}

	url := "/api/svm/svms"
	var hrefLink *models.SvmResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.SvmResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
	}

	svmResponse := models.SvmResponse{
		SvmResponseInlineRecords: []*models.Svm{&newSvm},
		NumRecords:               convert.ToPtr(int64(1)),
		Links:                    hrefLink,
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(svmResponse)
}

func mockSVMListInternalError(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	svmUUID := "1234"
	svmName := "svm0"

	newsvm := models.Svm{
		UUID:  &svmUUID,
		Name:  &svmName,
		State: convert.ToPtr("running"),
		SvmInlineAggregates: []*models.SvmInlineAggregatesInlineArrayItem{
			{Name: convert.ToPtr("aggr1")},
		},
	}

	url := "/api/svm/svms"
	var hrefLink *models.SvmResponseInlineLinks
	sc := http.StatusInternalServerError
	if hasNextLink {
		hrefLink = &models.SvmResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		sc = http.StatusOK
	}

	svmResponse := models.SvmResponse{
		SvmResponseInlineRecords: []*models.Svm{&newsvm},
		NumRecords:               convert.ToPtr(int64(1)),
		Links:                    hrefLink,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(sc)
	json.NewEncoder(w).Encode(svmResponse)
}

func TestOntapREST_SvmList(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(hasNextLink bool, w http.ResponseWriter, r *http.Request)
		isNegativeTest  bool
		isErrorExpected bool
	}{
		{"PositiveTest", mockSVMList, false, false},
		{"NumOfRecordsFieldIsNil", mockSVMListNumRecordsNil, false, false},
		{"BackendReturnErrorForSecondHrefLink", mockSVMListInternalError, false, true},
		{"BackendReturnError", mockInternalServerError, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := getHttpServer(test.isNegativeTest, test.mockFunction)
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			svmres, err := rs.SvmList(ctx, "svm0")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get svm")
				assert.Equal(t, "svm0", *svmres.Payload.SvmResponseInlineRecords[0].Name)
			} else {
				assert.Error(t, err, "get the svm")
			}
			server.Close()
		})
	}
}

func TestOntapREST_SvmGetByName(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockSVM, false},
		{"BackendReturnError", mockGetSVMError, true},
		{"NumOfRecordsFieldIsNil", mockSVMNumRecordsNil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			svmname, err := rs.SvmGetByName(ctx, "svm0")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get svm by name")
				assert.Equal(t, "svm0", *svmname.Name)
			} else {
				assert.Error(t, err, "get the svm by name")
			}
			server.Close()
		})
	}
}

func mockSVMSvmStateNil(w http.ResponseWriter, r *http.Request) {
	svmUUID := "1234"
	svmName := "svm0"

	newsvm := models.Svm{
		UUID: &svmUUID,
		Name: &svmName,
		SvmInlineAggregates: []*models.SvmInlineAggregatesInlineArrayItem{
			{Name: convert.ToPtr("aggr1")},
		},
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(newsvm)
}

func TestOntapREST_GetSVMState(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockSVM, false},
		{"NumOfRecordsFieldIsNil", mockSVMNumRecordsNil, true},
		{"SvmUUIDFieldIsNil", mockSVMUUIDNil, true},
		{"SvmStateFieldIsNil", mockSVMSvmStateNil, true},
		{"BackendReturnError", mockGetSVMError, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			svmState, err := rs.GetSVMState(ctx)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get svm state")
				assert.Equal(t, "running", svmState)
			} else {
				assert.Error(t, err, "get the svm state")
			}
			server.Close()
		})
	}
}

func TestOntapREST_SVMGetAggregateNames(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockSVM, false},
		{"BackendReturnError", mockGetSVMError, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			aggr, err := rs.SVMGetAggregateNames(ctx)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the system aggregates name")
				assert.Equal(t, "aggr1", aggr[0])
			} else {
				assert.Error(t, err, "get the system aggregates name")
			}
			server.Close()
		})
	}
}

func mockNodeListResponse(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	url := "/api/cluster/nodes"
	var hrefLink *models.NodeResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.NodeResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
	}

	nodeResponse := models.NodeResponse{
		NodeResponseInlineRecords: []*models.NodeResponseInlineRecordsInlineArrayItem{
			{SerialNumber: convert.ToPtr("4048820-60-9")},
		},
		NumRecords: convert.ToPtr(int64(1)),
		Links:      hrefLink,
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(nodeResponse)
}

func mockNodeListResponseInternalError(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	url := "/api/cluster/nodes"
	var hrefLink *models.NodeResponseInlineLinks
	sc := http.StatusInternalServerError
	if hasNextLink {
		hrefLink = &models.NodeResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		sc = http.StatusOK
	}
	nodeResponse := models.NodeResponse{
		NodeResponseInlineRecords: []*models.NodeResponseInlineRecordsInlineArrayItem{
			{SerialNumber: convert.ToPtr("4048820-60-9")},
		},
		NumRecords: convert.ToPtr(int64(1)),
		Links:      hrefLink,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(sc)
	json.NewEncoder(w).Encode(nodeResponse)
}

func mockNodeListResponseNumRecordsNil(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	url := "/api/cluster/nodes"
	var hrefLink *models.NodeResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.NodeResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		hasNextLink = false
	}
	nodeResponse := models.NodeResponse{
		NodeResponseInlineRecords: []*models.NodeResponseInlineRecordsInlineArrayItem{
			{SerialNumber: convert.ToPtr("4048820-60-9")},
		},
		Links: hrefLink,
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(nodeResponse)
}

func TestOntapREST_NodeList(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(hasNextLink bool, w http.ResponseWriter, r *http.Request)
		isNegativeTest  bool
		isErrorExpected bool
	}{
		{"PositiveTest", mockNodeListResponse, false, false},
		{"ResponseNumOfRecordsFieldIsNil", mockNodeListResponseNumRecordsNil, false, false},
		{"BackendReturnErrorForSecondHrefLink", mockNodeListResponseInternalError, false, true},
		{"BackendReturnError", mockInternalServerError, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := getHttpServer(test.isNegativeTest, test.mockFunction)
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			nodeResponses, err := rs.NodeList(ctx, "node1")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the node serial number")
				assert.Equal(t, "4048820-60-9", *nodeResponses.Payload.NodeResponseInlineRecords[0].SerialNumber)
			} else {
				assert.Error(t, err, "get the node serial number")
			}
			server.Close()
		})
	}
}

func mockNodeResponse(w http.ResponseWriter, r *http.Request) {
	nodeResponse := models.NodeResponse{
		NodeResponseInlineRecords: []*models.NodeResponseInlineRecordsInlineArrayItem{
			{SerialNumber: convert.ToPtr("4048820-60-9")},
		},
		NumRecords: convert.ToPtr(int64(1)),
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(nodeResponse)
}

func mockNodeResponseSerialNumberNil(w http.ResponseWriter, r *http.Request) {
	nodeResponse := models.NodeResponse{
		NodeResponseInlineRecords: []*models.NodeResponseInlineRecordsInlineArrayItem{
			{},
		},
		NumRecords: convert.ToPtr(int64(1)),
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(nodeResponse)
}

func mockNodeResponseNumRecordsNil(w http.ResponseWriter, r *http.Request) {
	nodeResponse := models.NodeResponse{
		NodeResponseInlineRecords: []*models.NodeResponseInlineRecordsInlineArrayItem{
			{SerialNumber: convert.ToPtr("4048820-60-9")},
		},
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(nodeResponse)
}

func mockNodeResponseNumRecordsZero(w http.ResponseWriter, r *http.Request) {
	nodeResponse := models.NodeResponse{
		NodeResponseInlineRecords: []*models.NodeResponseInlineRecordsInlineArrayItem{
			{SerialNumber: convert.ToPtr("4048820-60-9")},
		},
		NumRecords: convert.ToPtr(int64(0)),
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(nodeResponse)
}

func TestOntapREST_GetNodeSerialNumbers(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockNodeResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
		{"ResponseNumOfRecordsFieldIsNil", mockNodeResponseNumRecordsNil, true},
		{"ResponseNumOfRecordsZero", mockNodeResponseNumRecordsZero, true},
		{"ResponseSerialNumberFieldNil", mockNodeResponseSerialNumberNil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			serialNumber, err := rs.NodeListSerialNumbers(ctx)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not the node serial number")
				assert.Equal(t, "4048820-60-9", serialNumber[0])
			} else {
				assert.Error(t, err, "get the node serial number")
			}
			server.Close()
		})
	}
}

func TestOntapREST_EmsAutosupportLog(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockNodeResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.EmsAutosupportLog(ctx, "", true, "", "", "", 0, "", 0)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not generate a log message")
			} else {
				assert.Error(t, err, "log message generated")
			}
			server.Close()
		})
	}
}

func TestOntapREST_TieringPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(mockResourceNotFound))
	rs := newRestClient(server.Listener.Addr().String(), server.Client())
	assert.NotNil(t, rs)
	tieringPolicy := rs.TieringPolicyValue(ctx)
	assert.Equal(t, "none", tieringPolicy)
	server.Close()
}

func TestOntapREST_TieringPolicy_Disaggregated_916(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(mockResourceNotFound))
	rs := newRestClient(server.Listener.Addr().String(), server.Client())
	rs.disaggregated = true
	rs.OntapVersion = "9.16.1"
	assert.NotNil(t, rs)
	tieringPolicy := rs.TieringPolicyValue(ctx)
	assert.Equal(t, "none", tieringPolicy)
	server.Close()
}

func TestOntapREST_TieringPolicy_Disaggregated_917(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(mockResourceNotFound))
	rs := newRestClient(server.Listener.Addr().String(), server.Client())
	rs.disaggregated = true
	rs.OntapVersion = "9.17.0"
	assert.NotNil(t, rs)
	tieringPolicy := rs.TieringPolicyValue(ctx)
	assert.Equal(t, "", tieringPolicy)
	server.Close()
}

func TestOntapREST_TieringPolicy_InvalidVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(mockResourceNotFound))
	rs := newRestClient(server.Listener.Addr().String(), server.Client())
	rs.OntapVersion = "invalid"
	assert.NotNil(t, rs)
	tieringPolicy := rs.TieringPolicyValue(ctx)
	assert.Equal(t, "none", tieringPolicy)
	server.Close()
}

func mockNvmeNamespaceListResponse(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	numRecords := int64(1)
	size := int64(1073741824)

	url := "/api/storage/qtrees"

	var hrefLink *models.NvmeNamespaceResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.NvmeNamespaceResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		hasNextLink = false
	}

	nvmeNamespaceResponse := models.NvmeNamespaceResponse{
		NvmeNamespaceResponseInlineRecords: []*models.NvmeNamespace{
			{
				Name:  convert.ToPtr("namespace1"),
				UUID:  convert.ToPtr("1cd8a442-86d1-11e0-ae1c-123478563412"),
				Space: &models.NvmeNamespaceInlineSpace{Size: &size},
			},
		},
		NumRecords: &numRecords,
		Links:      hrefLink,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(nvmeNamespaceResponse)
}

func mockNvmeNamespaceListResponseInternalError(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	numRecords := int64(1)
	size := int64(1073741824)

	url := "/api/storage/qtrees"

	var hrefLink *models.NvmeNamespaceResponseInlineLinks
	sc := http.StatusInternalServerError
	if hasNextLink {
		hrefLink = &models.NvmeNamespaceResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		sc = http.StatusOK
	}

	nvmeNamespaceResponse := models.NvmeNamespaceResponse{
		NvmeNamespaceResponseInlineRecords: []*models.NvmeNamespace{
			{
				Name:  convert.ToPtr("namespace1"),
				UUID:  convert.ToPtr("1cd8a442-86d1-11e0-ae1c-123478563412"),
				Space: &models.NvmeNamespaceInlineSpace{Size: &size},
			},
		},
		NumRecords: &numRecords,
		Links:      hrefLink,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(sc)
	json.NewEncoder(w).Encode(nvmeNamespaceResponse)
}

func mockNvmeNamespaceListResponseNumRecordsNil(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	size := int64(1073741824)
	url := "/api/storage/qtrees"

	var hrefLink *models.NvmeNamespaceResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.NvmeNamespaceResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
	}

	nvmeNamespaceResponse := models.NvmeNamespaceResponse{
		NvmeNamespaceResponseInlineRecords: []*models.NvmeNamespace{
			{
				Name:  convert.ToPtr("namespace1"),
				UUID:  convert.ToPtr("1cd8a442-86d1-11e0-ae1c-123478563412"),
				Space: &models.NvmeNamespaceInlineSpace{Size: &size},
			},
		},
		Links: hrefLink,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(nvmeNamespaceResponse)
}

func mockNvmeSubsystemListResponse(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	numRecords := int64(1)
	url := "/api/fake"

	var hrefLink *models.NvmeSubsystemResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.NvmeSubsystemResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		hasNextLink = false
	}

	nvmeSubsystemResponse := models.NvmeSubsystemResponse{
		NvmeSubsystemResponseInlineRecords: []*models.NvmeSubsystem{
			{
				Name: convert.ToPtr("subsystemName"),
			},
		},
		NumRecords: &numRecords,
		Links:      hrefLink,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(nvmeSubsystemResponse)
}

func mockNvmeSubsystemListResponseInternalError(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	numRecords := int64(1)
	url := "/api/fake"

	var hrefLink *models.NvmeSubsystemResponseInlineLinks
	sc := http.StatusInternalServerError
	if hasNextLink {
		hrefLink = &models.NvmeSubsystemResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		sc = http.StatusOK
	}

	nvmeSubsystemResponse := models.NvmeSubsystemResponse{
		NvmeSubsystemResponseInlineRecords: []*models.NvmeSubsystem{
			{
				Name: convert.ToPtr("subsystemName"),
			},
		},
		NumRecords: &numRecords,
		Links:      hrefLink,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(sc)
	json.NewEncoder(w).Encode(nvmeSubsystemResponse)
}

func mockNvmeNamespaceResponse(w http.ResponseWriter, r *http.Request) {
	numRecords := int64(1)
	size := int64(1073741824)

	nvmeNamespaceResponse := models.NvmeNamespaceResponse{
		NvmeNamespaceResponseInlineRecords: []*models.NvmeNamespace{
			{
				Name:  convert.ToPtr("namespace1"),
				UUID:  convert.ToPtr("1cd8a442-86d1-11e0-ae1c-123478563412"),
				Space: &models.NvmeNamespaceInlineSpace{Size: &size},
			},
		},
		NumRecords: &numRecords,
	}

	switch r.Method {
	case "POST":
		setHTTPResponseHeader(w, http.StatusCreated)
		json.NewEncoder(w).Encode(nvmeNamespaceResponse)
	case "DELETE":
		setHTTPResponseHeader(w, http.StatusOK)
	default:
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(nvmeNamespaceResponse)
	}
}

func mockNvmeNamespaceResponseNumRecordsNil(w http.ResponseWriter, r *http.Request) {
	size := int64(1073741824)

	nvmeNamespaceResponse := models.NvmeNamespaceResponse{
		NvmeNamespaceResponseInlineRecords: []*models.NvmeNamespace{
			{
				Name:  convert.ToPtr("namespace1"),
				UUID:  convert.ToPtr("1cd8a442-86d1-11e0-ae1c-123478563412"),
				Space: &models.NvmeNamespaceInlineSpace{Size: &size},
			},
		},
	}

	switch r.Method {
	case "POST":
		setHTTPResponseHeader(w, http.StatusCreated)
		json.NewEncoder(w).Encode(nvmeNamespaceResponse)
	default:
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(nvmeNamespaceResponse)
	}
}

func mockNvmeNamespaceResponseNil(w http.ResponseWriter, r *http.Request) {
	nvmeNamespaceResponse := models.NvmeNamespaceResponse{
		NvmeNamespaceResponseInlineRecords: []*models.NvmeNamespace{
			nil,
		},
		NumRecords: convert.ToPtr(int64(1)),
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(nvmeNamespaceResponse)
}

func mockNvmeSubsystemMapResponse(w http.ResponseWriter, r *http.Request) {
	numRecords := int64(1)

	nvmeSubsystemMapResponse := models.NvmeSubsystemMapResponse{
		NvmeSubsystemMapResponseInlineRecords: []*models.NvmeSubsystemMap{
			{
				Namespace: &models.NvmeSubsystemMapInlineNamespace{
					UUID: convert.ToPtr("1cd8a442-86d1-11e0-ae1c-123478563412"),
				},
			},
		},
		NumRecords: &numRecords,
	}

	nvmeSubsystemResponse := models.NvmeSubsystemResponse{
		NvmeSubsystemResponseInlineRecords: []*models.NvmeSubsystem{
			{
				Name: convert.ToPtr("subsystemName"),
			},
		},
		NumRecords: &numRecords,
	}

	nvmeSubsystemHostResponse := models.NvmeSubsystemHostResponse{
		NvmeSubsystemHostResponseInlineRecords: []*models.NvmeSubsystemHost{
			{
				Subsystem: &models.NvmeSubsystemHostInlineSubsystem{
					Name: convert.ToPtr("nvmeSubsystemName"),
				},
			},
		},
	}

	switch r.Method {
	case "POST":
		switch r.URL.Path {
		case "/api/protocols/nvme/subsystems":
			setHTTPResponseHeader(w, http.StatusCreated)
			json.NewEncoder(w).Encode(nvmeSubsystemResponse)
		case "/api/protocols/nvme/subsystems/subsystemName/hosts":
			setHTTPResponseHeader(w, http.StatusCreated)
			json.NewEncoder(w).Encode(nvmeSubsystemHostResponse)
		default:
			setHTTPResponseHeader(w, http.StatusCreated)
			json.NewEncoder(w).Encode(nvmeSubsystemMapResponse)
		}
	case "DELETE":
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(nil)
	case "GET":
		switch r.URL.Path {
		case "/api/protocols/nvme/subsystems/subsystemName/hosts":
			setHTTPResponseHeader(w, http.StatusOK)
			json.NewEncoder(w).Encode(nvmeSubsystemHostResponse)
		case "/api/protocols/nvme/subsystem-maps":
			setHTTPResponseHeader(w, http.StatusOK)
			json.NewEncoder(w).Encode(nvmeSubsystemMapResponse)
		default:
			setHTTPResponseHeader(w, http.StatusOK)
			json.NewEncoder(w).Encode(nvmeSubsystemResponse)
		}
	}
}

func mockNvmeResourceNotFound(w http.ResponseWriter, r *http.Request) {
	setHTTPResponseHeader(w, http.StatusNotFound)
	json.NewEncoder(w).Encode("")
}

func TestOntapRestNVMeNamespaceCreate(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockNvmeNamespaceResponse, false},
		{"NumRecordsNilInResponse", mockNvmeNamespaceResponseNumRecordsNil, true},
		{"BackendReturnError", mockNvmeResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			ns := NVMeNamespace{
				Name:      "namespace1",
				UUID:      "1cd8a442-86d1-11e0-ae1c-123478563412",
				OsType:    "linux",
				Size:      "99999",
				BlockSize: 4096,
				State:     "online",
			}
			err := rs.NVMeNamespaceCreate(ctx, ns)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not create the NVMe namespace")
			} else {
				assert.Error(t, err, "NVMe namespace list created")
			}
			server.Close()
		})
	}
}

func TestOntapRestNVMeNamespaceSetSize(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockNvmeNamespaceResponse, false},
		{"BackendReturnError", mockNvmeResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			ns := NVMeNamespace{
				Name:      "namespace1",
				UUID:      "1cd8a442-86d1-11e0-ae1c-123478563412",
				OsType:    "linux",
				Size:      "99999",
				BlockSize: 4096,
				State:     "online",
			}
			err := rs.NVMeNamespaceSetSize(ctx, ns.UUID, int64(100000000))
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the NVMe namespace size")
			} else {
				assert.Error(t, err, "get the NVMe namespace size")
			}
			server.Close()
		})
	}
}

func TestOntapRest_NVMeNamespaceList(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(hasNextLink bool, w http.ResponseWriter, r *http.Request)
		isNegativeTest  bool
		isErrorExpected bool
	}{
		{"PositiveTest", mockNvmeNamespaceListResponse, false, false},
		{"ResponseNumOfRecordsFieldIsNil", mockNvmeNamespaceListResponseNumRecordsNil, false, false},
		{"BackendReturnErrorForSecondHrefLink", mockNvmeNamespaceListResponseInternalError, false, true},
		{"BackendReturnError", mockNvmeNamespaceListResponse, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := getHttpServer(test.isNegativeTest, test.mockFunction)
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			nvmeResponse, err := rs.NVMeNamespaceList(ctx, "namespace1", []string{""})
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the NVMe namespace list")
				assert.Equal(t, "namespace1", *nvmeResponse.Payload.NvmeNamespaceResponseInlineRecords[0].Name)
			} else {
				assert.Error(t, err, "get the NVMe namespace list")
			}
			server.Close()
		})
	}
}

func TestOntapRest_NVMeNamespaceGetByName(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockNvmeNamespaceResponse, false},
		{"ResponseNumOfRecordsFieldIsNil", mockNvmeNamespaceResponseNumRecordsNil, true},
		{"BackendReturnError", mockNvmeResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			defer server.Close()
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			nvmeResponse, err := rs.NVMeNamespaceGetByName(ctx, "namespace1", []string{""})
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the NVMe namespace by name")
				assert.Equal(t, "namespace1", *nvmeResponse.Name)
			} else {
				assert.Error(t, err, "get the NVMe namespace by name")
			}
			server.Close()
		})
	}
}

func mockNvmeNamespaceDeleteAsync(w http.ResponseWriter, r *http.Request) {
	if r.Method == "DELETE" {
		mockRequestAccepted(w, r)
	} else if r.URL.Path == "/api/cluster/jobs/1234" {
		mockJobResponse(w, r)
	}
}

func mockNvmeNamespaceDeleteSync(w http.ResponseWriter, r *http.Request) {
	if r.Method == "DELETE" {
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(nil)
	}
}

func TestOntapRest_NVMeNamespaceDelete(t *testing.T) {
	uuid := "1cd8a442-86d1-11e0-ae1c-123478563412"
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		uuid            string
		isErrorExpected bool
	}{
		{"PositiveTest with jobID", mockNvmeNamespaceDeleteAsync, uuid, false},
		{"PositiveTest with no jobID", mockNvmeNamespaceDeleteSync, uuid, false},
		{"NegativeTest Namespace UUID not provided", nil, "", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			defer server.Close()
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.NVMeNamespaceDelete(ctx, test.uuid)
			if !test.isErrorExpected {
				assert.NoError(t, err, "expected no error when deleting thenamespace")
			} else {
				assert.Error(t, err, "expected error when deleting namespace")
			}
			server.Close()
		})
	}
}

func TestOntapRest_NVMeSubsystemAddNamespace(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockNvmeSubsystemMapResponse, false},
		{"BackendReturnError", mockNvmeResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			defer server.Close()
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.NVMeSubsystemAddNamespace(ctx, "subsystemUUID", "nsUUID")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not add subsystem in NVMe namespace")
			} else {
				assert.Error(t, err, "subsystem is added in NVMe namespace")
			}
			server.Close()
		})
	}
}

func TestOntapRest_NVMeSubsystemRemoveNamespace(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockNvmeSubsystemMapResponse, false},
		{"BackendReturnError", mockNvmeResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.NVMeSubsystemRemoveNamespace(ctx, "subsystemUUID", "nsUUID")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not delete NVMe namespace")
			} else {
				assert.Error(t, err, "NVMe namespace deleted")
			}
			server.Close()
		})
	}
}

func TestOntapRest_NVMeIsNamespaceMapped(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockNvmeSubsystemMapResponse, false},
		{"BackendReturnError", mockNvmeResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			isMapped, err := rs.NVMeIsNamespaceMapped(ctx, "subsystemUUID", "1cd8a442-86d1-11e0-ae1c-123478563412")
			if !test.isErrorExpected {
				assert.NoError(t, err, "NVMe namespace is not mapped")
				assert.Equal(t, true, isMapped)
			} else {
				assert.Error(t, err, "NVMe namespace is mapped")
			}
			server.Close()
		})
	}
}

func TestOntapRest_NVMeNamespaceCount(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockNvmeSubsystemMapResponse, false},
		{"BackendReturnError", mockNvmeResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			count, err := rs.NVMeNamespaceCount(ctx, "subsystemUUID")
			if !test.isErrorExpected {
				assert.NoError(t, err)
				assert.Equal(t, int64(1), count, "could not nvme subsystem count")
			} else {
				assert.Error(t, err, "get the nvme subsystem count")
			}
			server.Close()
		})
	}
}

func TestOntapRest_NVMeSubsystemList(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(hasNextLink bool, w http.ResponseWriter, r *http.Request)
		pattern         string
		isNegativeTest  bool
		isErrorExpected bool
	}{
		{"PositiveTest", mockNvmeSubsystemListResponse, "subsystemUUID", false, false},
		{"BackendReturnErrorForSecondHrefLink", mockNvmeSubsystemListResponseInternalError, "subsystemUUID", false, true},
		{"BackendReturnError", mockInternalServerError, "", true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := getHttpServer(test.isNegativeTest, test.mockFunction)
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			_, err := rs.NVMeSubsystemList(ctx, test.pattern, []string{""})
			if !test.isErrorExpected {
				assert.NoError(t, err, "failed to get NVMe subsystem list")
			} else {
				assert.Error(t, err, "get the NVMe subsystem")
			}
			server.Close()
		})
	}
}

func TestOntapRest_NVMeSubsystemGetByName(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockNvmeSubsystemMapResponse, false},
		{"BackendReturnError", mockNvmeResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			nvmeSubsystemList, err := rs.NVMeSubsystemGetByName(ctx, "subsystemUUID", []string{""})
			if !test.isErrorExpected {
				assert.NoError(t, err, "failed to get NVMe subsystem by name")
				assert.Equal(t, "subsystemName", *nvmeSubsystemList.Name)
			} else {
				assert.Error(t, err, "get the NVMe subsystem")
			}
			server.Close()
		})
	}
}

func TestOntapRest_NVMeSubsystemCreate(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockNvmeSubsystemMapResponse, false},
		{"BackendReturnError", mockNvmeResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			nvmeSubsystem, err := rs.NVMeSubsystemCreate(ctx, "subsystemName", "comment to be set on subsystem")
			if !test.isErrorExpected {
				assert.NoError(t, err, "issue while creating subsystem")
				assert.Equal(t, "subsystemName", *nvmeSubsystem.Name)
			} else {
				assert.Error(t, err, "subsystem is created")
			}
			server.Close()
		})
	}
}

func TestOntapRestNVMeSubsystemDelete(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockNvmeSubsystemMapResponse, false},
		{"BackendReturnError", mockNvmeResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.NVMeSubsystemDelete(ctx, "subsystemName")
			if !test.isErrorExpected {
				assert.NoError(t, err, "issue while deleting subsystem")
			} else {
				assert.Error(t, err, "subsystem is deleted")
			}
			server.Close()
		})
	}
}

func TestOntapRestNVMeAddHostNqnToSubsystem(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockNvmeSubsystemMapResponse, false},
		{"BackendReturnError", mockNvmeResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.NVMeAddHostNqnToSubsystem(ctx, "hostiqn", "subsystemName")
			if !test.isErrorExpected {
				assert.NoError(t, err, "issue while adding host to subsystem")
			} else {
				assert.Error(t, err, "host added to subsystem")
			}
			server.Close()
		})
	}
}

func TestOntapRestNVMeGetHostsOfSubsystem(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockNvmeSubsystemMapResponse, false},
		{"BackendReturnError", mockNvmeResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			nvmeGetHostsOfSubsystem, err := rs.NVMeGetHostsOfSubsystem(ctx, "subsystemName")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the host of NVMe subsystem")
				assert.Equal(t, "nvmeSubsystemName", *nvmeGetHostsOfSubsystem[0].Subsystem.Name)
			} else {
				assert.Error(t, err, "get the host of NVMe subsystem")
			}

			server.Close()
		})
	}
}

func TestOntapRestNVMeNamespaceSize(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockNvmeNamespaceResponse, false},
		{"BackendReturnError", mockNvmeResourceNotFound, true},
		{"BackendReturnNilResponse", mockNvmeNamespaceResponseNil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			size, err := rs.NVMeNamespaceSize(ctx, "namespace1")
			if !test.isErrorExpected {
				assert.NoError(t, err, "failed to get NVMe namespace size")
				assert.Equal(t, 1073741824, size)
			} else {
				assert.Error(t, err, "get the NVMe namespace size")
			}
			server.Close()
		})
	}
}

func mockSVM(w http.ResponseWriter, r *http.Request) {
	svmUUID := "1234"
	svmName := "svm0"

	mocksvm := models.Svm{
		UUID:  &svmUUID,
		Name:  &svmName,
		State: convert.ToPtr("running"),
		SvmInlineAggregates: []*models.SvmInlineAggregatesInlineArrayItem{
			{Name: convert.ToPtr("aggr1")},
		},
	}
	if r.URL.Path == "/api/svm/svms/1234" {
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(mocksvm)
	}

	if r.URL.Path == "/api/svm/svms" {
		svmResponse := models.SvmResponse{
			SvmResponseInlineRecords: []*models.Svm{&mocksvm},
			NumRecords:               convert.ToPtr(int64(1)),
		}
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(svmResponse)
	}
}

func mockSVMNameNil(w http.ResponseWriter, r *http.Request) {
	mocksvm := models.Svm{
		UUID:  convert.ToPtr("fake-uuid"),
		State: convert.ToPtr("running"),
		SvmInlineAggregates: []*models.SvmInlineAggregatesInlineArrayItem{
			{UUID: convert.ToPtr("fake-uuid")},
		},
	}

	svmResponse := models.SvmResponse{
		SvmResponseInlineRecords: []*models.Svm{&mocksvm},
		NumRecords:               convert.ToPtr(int64(1)),
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(svmResponse)
}

func mockSVMNumRecordsNil(w http.ResponseWriter, r *http.Request) {
	mocksvm := models.Svm{
		Name: convert.ToPtr("svm0"),
		SvmInlineAggregates: []*models.SvmInlineAggregatesInlineArrayItem{
			{Name: convert.ToPtr("aggr1")},
		},
	}
	svmResponse := models.SvmResponse{
		SvmResponseInlineRecords: []*models.Svm{&mocksvm},
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(svmResponse)
}

func mockGetSVMError(w http.ResponseWriter, r *http.Request) {
	setHTTPResponseHeader(w, http.StatusNotFound)
	json.NewEncoder(w).Encode("")
}

func TestOntapRest_EnsureSVMWithRest(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		svm             string
		isErrorExpected bool
	}{
		{"PositiveTest", mockSVM, "svm0", false},
		{"PositiveTest_SVMNameEmptyInOntapConfig", mockSVM, "", false},
		{"SVMNameEmptyInOntapConfig_NumRecordsFieldNilInResponse", mockSVMNumRecordsNil, "", true},
		{"SVMNameEmptyInOntapConfig_UUIDNilInResponse", mockSVMUUIDNil, "", true},
		{"SVMNameEmptyInOntapConfig_SVMNameNilInResponse", mockSVMNameNil, "", true},
		{"SVMNameEmptyInOntapConfig_BackendReturnError", mockGetSVMError, "", true},
		{"BackendReturnError", mockGetSVMError, "svm0", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			storageDriverConfig := drivers.OntapStorageDriverConfig{SVM: test.svm}
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := EnsureSVMWithRest(ctx, &storageDriverConfig, rs)
			if !test.isErrorExpected {
				assert.NoError(t, err, "failed to get svm")
			} else {
				assert.Error(t, err, "get the svm")
			}
			server.Close()
		})
	}
}

func mockJobScheduleResponse(w http.ResponseWriter, r *http.Request) {
	numRecords := int64(1)
	scheduleResponse := &models.ScheduleResponse{
		ScheduleResponseInlineRecords: []*models.Schedule{{}},
		NumRecords:                    &numRecords,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(scheduleResponse)
}

func mockJobScheduleResponseRecordNil(w http.ResponseWriter, r *http.Request) {
	scheduleResponse := &models.ScheduleResponse{}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(scheduleResponse)
}

func mockJobScheduleResponseNumRecordsNil(w http.ResponseWriter, r *http.Request) {
	scheduleResponse := &models.ScheduleResponse{
		ScheduleResponseInlineRecords: []*models.Schedule{{}},
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(scheduleResponse)
}

func mockJobScheduleResponseNumRecordsZero(w http.ResponseWriter, r *http.Request) {
	numRecords := int64(0)
	scheduleResponse := &models.ScheduleResponse{
		ScheduleResponseInlineRecords: []*models.Schedule{{}},
		NumRecords:                    &numRecords,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(scheduleResponse)
}

func mockJobScheduleResponseNumRecordsGrt1(w http.ResponseWriter, r *http.Request) {
	numRecords := int64(2)
	scheduleResponse := &models.ScheduleResponse{
		ScheduleResponseInlineRecords: []*models.Schedule{{}},
		NumRecords:                    &numRecords,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(scheduleResponse)
}

func TestOntapRest_JobScheduleExists(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockJobScheduleResponse, false},
		{"JobIsNilInResponse", mockJobScheduleResponseRecordNil, true},
		{"NumRecordFieldNil", mockJobScheduleResponseNumRecordsNil, true},
		{"NumRecordFieldZero", mockJobScheduleResponseNumRecordsZero, true},
		{"NumRecordMoreThanOne", mockJobScheduleResponseNumRecordsGrt1, true},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			jobExists, err := rs.JobScheduleExists(ctx, "fake-job")
			if !test.isErrorExpected {
				assert.Equal(t, true, jobExists)
				assert.NoError(t, err, "schedule job does not exists")
			} else {
				assert.Error(t, err, "schedule job exists")
			}
			server.Close()
		})
	}
}

func TestOntapRest_VolumeExists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(mockResourceNotFound))
	defer server.Close()

	rs := newRestClient(server.Listener.Addr().String(), server.Client())
	assert.NotNil(t, rs)

	volumeExists, err := rs.VolumeExists(ctx, "")
	assert.NoError(t, err, "could not check if the volume exists")
	assert.False(t, volumeExists)
}

func getVolumeInfo() *models.Volume {
	volumeName := "fakeVolume"
	volumeUUID := "fakeUUID"
	volumeType := "rw"
	aggregates := "fakeAggr"
	comment := ""
	path := "/fakeVolume"
	unixPermission := int64(777)
	exportPolicy := "fakeExportPolicy"
	size := int64(1073741824)
	guarantee := "none"
	snapshotPolicy := "fakeSnapshotPolicy"
	snapshotReservePercent := int64(20)
	snapshotUsed := int64(1073741810)
	snapshotDir := false
	quotaState := "on"
	quotaEnabled := true
	snapshotName := "fakeSnapshot"
	encryptionEnabled := true

	volumeGuarantee := models.VolumeInlineGuarantee{Type: &guarantee}

	volumeInlineAggr := models.VolumeInlineAggregatesInlineArrayItem{Name: &aggregates}
	volumeInlineAggrList := []*models.VolumeInlineAggregatesInlineArrayItem{&volumeInlineAggr}

	volumeInlineExportPolicy := models.VolumeInlineNasInlineExportPolicy{Name: &exportPolicy}
	VolumeInlineNas := models.VolumeInlineNas{
		Path: &path, UnixPermissions: &unixPermission,
		ExportPolicy: &volumeInlineExportPolicy,
	}

	volumeSnapshotPolicy := models.VolumeInlineSnapshotPolicy{Name: &snapshotPolicy}
	volumeSpaceSnapshot := models.VolumeInlineSpaceInlineSnapshot{
		ReservePercent: &snapshotReservePercent,
		Used:           &snapshotUsed,
	}
	volumeSpace := models.VolumeInlineSpace{Snapshot: &volumeSpaceSnapshot, LogicalSpace: &models.VolumeInlineSpaceInlineLogicalSpace{Used: &size}}
	volumeQuota := models.VolumeInlineQuota{State: &quotaState, Enabled: &quotaEnabled}

	clone := models.VolumeInlineClone{
		ParentSnapshot: &models.SnapshotReference{Name: &snapshotName},
		ParentVolume:   &models.VolumeInlineCloneInlineParentVolume{Name: &volumeName},
	}

	encryption := models.VolumeInlineEncryption{Enabled: &encryptionEnabled}
	movement := models.VolumeInlineMovement{TieringPolicy: convert.ToPtr("tieringPolicy")}
	volume := models.Volume{
		Name:                           &volumeName,
		UUID:                           &volumeUUID,
		Type:                           &volumeType,
		VolumeInlineAggregates:         volumeInlineAggrList,
		Comment:                        &comment,
		Nas:                            &VolumeInlineNas,
		Size:                           &size,
		Guarantee:                      &volumeGuarantee,
		SnapshotPolicy:                 &volumeSnapshotPolicy,
		Space:                          &volumeSpace,
		SnapshotDirectoryAccessEnabled: &snapshotDir,
		Quota:                          &volumeQuota,
		Clone:                          &clone,
		Encryption:                     &encryption,
		Movement:                       &movement,
	}
	return &volume
}

func mockGetVolumeResponseNumRecordsMoreThanTwo(w http.ResponseWriter, r *http.Request) {
	volume := getVolumeInfo()
	volume.Size = nil
	numRecords := int64(2)
	volumeResponse := models.VolumeResponse{
		VolumeResponseInlineRecords: []*models.Volume{volume, nil},
		NumRecords:                  &numRecords,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(volumeResponse)
}

func mockGetVolumeResponseSizeNil(w http.ResponseWriter, r *http.Request) {
	volume := getVolumeInfo()
	volume.Size = nil
	numRecords := int64(1)
	volumeResponse := models.VolumeResponse{
		VolumeResponseInlineRecords: []*models.Volume{volume, nil},
		NumRecords:                  &numRecords,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(volumeResponse)
}

func mockGetVolumeResponseUUIDNil(w http.ResponseWriter, r *http.Request) {
	volume := getVolumeInfo()
	// Test case: Volume name and UUId is Nil
	volume.UUID = nil
	volume.Name = nil
	numRecords := int64(1)
	volumeResponse := models.VolumeResponse{
		VolumeResponseInlineRecords: []*models.Volume{volume, nil},
		NumRecords:                  &numRecords,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(volumeResponse)
}

func mockGetVolumeResponseNasPathNil(w http.ResponseWriter, r *http.Request) {
	nasPath := ""
	volume := getVolumeInfo()
	volume.Nas.Path = &nasPath
	numRecords := int64(1)
	volumeResponse := models.VolumeResponse{
		VolumeResponseInlineRecords: []*models.Volume{volume, nil},
		NumRecords:                  &numRecords,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(volumeResponse)
}

func mockVolume(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	url := "/api/storage/volumes"
	var hrefLink *models.VolumeResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.VolumeResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		hasNextLink = false
	}

	volume := getVolumeInfo()
	numRecords := int64(1)
	volumeResponse := models.VolumeResponse{
		VolumeResponseInlineRecords: []*models.Volume{volume},
		NumRecords:                  &numRecords,
		Links:                       hrefLink,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(volumeResponse)
}

func TestGetAllVolumePayloadRecords(t *testing.T) {
	href := "/api/storage/volumes"

	server := getHttpServer(false, mockVolume)
	rs := newRestClient(server.Listener.Addr().String(), server.Client())
	assert.NotNil(t, rs)

	volumeResponse := models.VolumeResponse{
		Links: &models.VolumeResponseInlineLinks{
			Next: &models.Href{Href: &href},
		},
	}

	volumeParam := &storage.VolumeCollectionGetParams{}
	volumeParam.Context = ctx
	volume, err := rs.getAllVolumePayloadRecords(&volumeResponse, volumeParam)
	assert.NoError(t, err)
	assert.Equal(t, "fakeVolume", *volume.VolumeResponseInlineRecords[0].Name)
	assert.Equal(t, "fakeUUID", *volume.VolumeResponseInlineRecords[0].UUID)
	assert.Equal(t, int64(1073741824), *volume.VolumeResponseInlineRecords[0].Size)
	assert.Equal(t, "rw", *volume.VolumeResponseInlineRecords[0].Type)
	assert.Equal(t, int64(777), *volume.VolumeResponseInlineRecords[0].Nas.UnixPermissions)
	assert.Equal(t, "fakeAggr", *volume.VolumeResponseInlineRecords[0].VolumeInlineAggregates[0].Name)
	server.Close()
}

func mockGetVolumeResponseNumRecordsNil(w http.ResponseWriter, r *http.Request) {
	href := "/api/storage/volumes"
	volume := getVolumeInfo()
	volumeResponse := models.VolumeResponse{
		VolumeResponseInlineRecords: []*models.Volume{volume, nil},
		Links: &models.VolumeResponseInlineLinks{
			Next: &models.Href{Href: &href},
		},
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(volumeResponse)
}

func TestGetAllVolumePayloadRecordsFail(t *testing.T) {
	href := "/api/storage/volumes"

	volumeResponse := models.VolumeResponse{
		Links: &models.VolumeResponseInlineLinks{
			Next: &models.Href{Href: &href},
		},
	}

	volumeParam := &storage.VolumeCollectionGetParams{}
	volumeParam.Context = ctx

	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		IsErrorExpected bool
	}{
		{mockFunction: mockResourceNotFound, IsErrorExpected: true, name: "BackendReturnError"},
		{
			mockFunction: mockGetVolumeResponseNumRecordsNil, IsErrorExpected: false,
			name: "GetVolumeResponseNumRecordsNil",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			_, err := rs.getAllVolumePayloadRecords(&volumeResponse, volumeParam)
			if !test.IsErrorExpected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
			server.Close()
		})
	}
}

func mockGetVolumeResponse(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/cluster/jobs/1234" {
		mockJobResponse(w, r)
	} else if r.Method == "PATCH" {
		mockRequestAccepted(w, r)
	} else {
		volume := getVolumeInfo()
		numRecords := int64(1)
		volumeResponse := models.VolumeResponse{
			VolumeResponseInlineRecords: []*models.Volume{volume},
			NumRecords:                  &numRecords,
		}

		r.Host = "127.0.0.1"
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(volumeResponse)
	}
}

func TestGetAllVolumesByPatternStyleAndState_failure(t *testing.T) {
	tests := []struct {
		style           string
		state           string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"InvalidStyle", "InvalidState", mockGetVolumeResponse, true},
		{models.VolumeStyleFlexvol, "InvalidState", mockGetVolumeResponse, true},
		{models.VolumeStyleFlexvol, models.VolumeStateOnline, mockGetVolumeResponseNumRecordsNil, false},
	}

	for _, test := range tests {
		t.Run(test.style, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))

			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			volumeParam := &storage.VolumeCollectionGetParams{}
			volumeParam.Context = ctx

			fields := []string{"size", "type", "nas.unix_permissions", "aggregates"}
			volume, err := rs.getAllVolumesByPatternStyleAndState(ctx, "trident", test.style, test.state, fields)

			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the volume info")
				assert.Equal(t, "fakeVolume", *volume.Payload.VolumeResponseInlineRecords[0].Name)
				assert.Equal(t, "fakeUUID", *volume.Payload.VolumeResponseInlineRecords[0].UUID)
				assert.Equal(t, int64(1073741824), *volume.Payload.VolumeResponseInlineRecords[0].Size)
				assert.Equal(t, "rw", *volume.Payload.VolumeResponseInlineRecords[0].Type)
				assert.Equal(t, int64(777), *volume.Payload.VolumeResponseInlineRecords[0].Nas.UnixPermissions)
				assert.Equal(t, "fakeAggr", *volume.Payload.VolumeResponseInlineRecords[0].
					VolumeInlineAggregates[0].Name)
			} else {
				assert.Error(t, err, "get the volume info")
			}
			server.Close()
		})
	}
}

func TestOntapRestGetVolumeByNameAndStyle(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"GetVolume", mockGetVolumeResponse, false},
		{"GetVolumeNumRecordMoreThanOne", mockGetVolumeResponseNumRecordsMoreThanTwo, true},
		{"GetVolumeNumRecordsNil", mockGetVolumeResponseNumRecordsNil, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))

			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			volumeParam := &storage.VolumeCollectionGetParams{}
			volumeParam.Context = ctx
			_, err := rs.getVolumeByNameAndStyle(ctx, "trident", models.VolumeStyleFlexgroup, []string{""})
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the volume info")
			} else {
				assert.Error(t, err, "get the volume info")
			}
			server.Close()
		})
	}
}

func TestOntapRestGetVolumeSizeByNameAndStyleFailure(t *testing.T) {
	tests := []struct {
		name         string
		mockFunction func(w http.ResponseWriter, r *http.Request)
	}{
		{"GetVolumeNumRecordMoreThanOne", mockGetVolumeResponseNumRecordsMoreThanTwo},
		{"GetVolumeNumRecordsNil", mockGetVolumeResponseNumRecordsNil},
		{"GetVolumeSizeNil", mockGetVolumeResponseSizeNil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))

			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			volumeParam := &storage.VolumeCollectionGetParams{}
			volumeParam.Context = ctx
			_, err := rs.getVolumeSizeByNameAndStyle(ctx, "fakeVolume", models.VolumeStyleFlexvol)
			assert.Error(t, err, "could not get volume info")
			server.Close()
		})
	}
}

func mockGetVolumeResponseSpaceNil(w http.ResponseWriter, r *http.Request) {
	volume := getVolumeInfo()
	volume.Space = nil
	numRecords := int64(1)
	volumeResponse := models.VolumeResponse{
		VolumeResponseInlineRecords: []*models.Volume{volume, nil},
		NumRecords:                  &numRecords,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(volumeResponse)
}

func mockGetVolumeResponseLogicalSpaceNil(w http.ResponseWriter, r *http.Request) {
	volume := getVolumeInfo()
	volume.Space.LogicalSpace = nil
	numRecords := int64(1)
	volumeResponse := models.VolumeResponse{
		VolumeResponseInlineRecords: []*models.Volume{volume, nil},
		NumRecords:                  &numRecords,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(volumeResponse)
}

func mockGetVolumeResponseLogicalSpaceUsedNil(w http.ResponseWriter, r *http.Request) {
	volume := getVolumeInfo()
	volume.Space.LogicalSpace.Used = nil
	numRecords := int64(1)
	volumeResponse := models.VolumeResponse{
		VolumeResponseInlineRecords: []*models.Volume{volume, nil},
		NumRecords:                  &numRecords,
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(volumeResponse)
}

func TestOntapRestGetVolumeUsedSizeByNameAndStyle_failure(t *testing.T) {
	tests := []struct {
		name         string
		mockFunction func(w http.ResponseWriter, r *http.Request)
	}{
		{"GetVolumeSpaceNil", mockGetVolumeResponseSpaceNil},
		{"GetVolumeLogicalSpaceNil", mockGetVolumeResponseLogicalSpaceNil},
		{"GetVolumeLogicalSpaceUsedNil", mockGetVolumeResponseLogicalSpaceUsedNil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))

			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			_, err := rs.getVolumeUsedSizeByNameAndStyle(ctx, "fakeVolume", models.VolumeStyleFlexvol)
			assert.Error(t, err, "could not get the volume info")
			server.Close()
		})
	}
}

func mockModifyFailed(w http.ResponseWriter, r *http.Request) {
	if r.Method == "PATCH" || r.Method == "DELETE" {
		mockResourceNotFound(w, r)
	} else if r.URL.Path == "/api/cluster/jobs/1234" {
		mockJobResponse(w, r)
	} else {
		volume := getVolumeInfo()
		numRecords := int64(1)
		volumeResponse := models.VolumeResponse{
			VolumeResponseInlineRecords: []*models.Volume{volume},
			NumRecords:                  &numRecords,
		}

		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(volumeResponse)
	}
}

func TestOntapRestSetVolumeSizeByNameAndStyle_failure(t *testing.T) {
	tests := []struct {
		name         string
		mockFunction func(w http.ResponseWriter, r *http.Request)
	}{
		{"GetVolumeNumRecordMoreThanOne", mockGetVolumeResponseNumRecordsMoreThanTwo},
		{"GetVolumeNumRecordsNil", mockGetVolumeResponseNumRecordsNil},
		{"GetVolumeUUIDNil", mockGetVolumeResponseUUIDNil},
		{"ModifySizeFail", mockModifyFailed},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))

			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			volumeParam := &storage.VolumeCollectionGetParams{}
			volumeParam.Context = ctx
			err := rs.setVolumeSizeByNameAndStyle(ctx, "fakeVolume", "1073741824", models.VolumeStyleFlexvol)
			assert.Error(t, err, "could not set the volume info")
			server.Close()
		})
	}
}

func TestOntapRestMountVolumeByNameAndStyle(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"GetVolume", mockGetVolumeResponseAccepted, false},
		{"MountFail", mockModifyFailed, true},
		{"GetVolumeNumRecordMoreThanOne", mockGetVolumeResponseNumRecordsMoreThanTwo, true},
		{"GetVolumeNumRecordsNil", mockGetVolumeResponseNumRecordsNil, true},
		{"GetVolumeUUIDNil", mockGetVolumeResponseUUIDNil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.mountVolumeByNameAndStyle(ctx, "fakeVolume", "/fakeVolume_junctionPath", models.VolumeStyleFlexvol)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get a volume")
			} else {
				assert.Error(t, err, "get the volume")
			}
			server.Close()
		})
	}
}

func TestOntapRestUnmountVolumeByNameAndStyle(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"GetVolume", mockGetVolumeResponseAccepted, false},
		{"UnMountFail", mockModifyFailed, true},
		{"GetVolumeNumRecordMoreThanOne", mockGetVolumeResponseNumRecordsMoreThanTwo, true},
		{"GetVolumeNumRecordsNil", mockGetVolumeResponseNumRecordsNil, false},
		{"GetVolumeUUIDNil", mockGetVolumeResponseUUIDNil, false},
		{"GetVolumeNasPathNil", mockGetVolumeResponseNasPathNil, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.unmountVolumeByNameAndStyle(ctx, "fakeVolume", models.VolumeStyleFlexvol)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get a volume")
			} else {
				assert.Error(t, err, "get the volume")
			}
			server.Close()
		})
	}
}

func TestOntapRestRenameVolumeByNameAndStyle(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"GetVolume", mockGetVolumeResponseAccepted, false},
		{"VolumeRenameFailed", mockModifyFailed, true},
		{"GetVolumeNumRecordsNil", mockGetVolumeResponseNumRecordsNil, true},
		{"GetVolumeNumRecordMoreThanOne", mockGetVolumeResponseNumRecordsMoreThanTwo, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.renameVolumeByNameAndStyle(ctx, "fakeVolume", "newVolumeName", models.VolumeStyleFlexvol)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get a volume")
			} else {
				assert.Error(t, err, "get the volume")
			}
			server.Close()
		})
	}
}

func TestOntapRestDestroyVolumeByNameAndStyle(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"GetVolume", mockGetVolumeResponseAccepted, false},
		{"VolumeDeleteFail", mockModifyFailed, true},
		{"GetVolumeNumRecordsNil", mockGetVolumeResponseNumRecordsNil, true},
		{"GetVolumeNumRecordMoreThanOne", mockGetVolumeResponseNumRecordsMoreThanTwo, true},
		{"GetVolumeUUIDNil", mockGetVolumeResponseUUIDNil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.destroyVolumeByNameAndStyle(ctx, "fakeVolume", models.VolumeStyleFlexvol, false)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not delete a volume")
			} else {
				assert.Error(t, err, "volume deleted")
			}
			server.Close()
		})
	}
}

func TestOntapRestModifyVolumeExportPolicyByNameAndStyle(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"VolumeExportFail", mockModifyFailed, true},
		{"GetVolumeNumRecordsNil", mockGetVolumeResponseNumRecordsNil, true},
		{"GetVolumeUUIDNil", mockGetVolumeResponseUUIDNil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.modifyVolumeExportPolicyByNameAndStyle(ctx, "fakeVolume",
				"fake-exportPolicy", models.VolumeStyleFlexvol)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not modify export policy on volume")
			} else {
				assert.Error(t, err, "export policy modified on volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_ModifyVolumeUnixPermissionsByNameAndStyle(t *testing.T) {
	tests := []struct {
		name         string
		mockFunction func(w http.ResponseWriter, r *http.Request)
	}{
		{"modified_fail", mockModifyFailed},
		{"getVolume_fail", mockGetVolumeResponseNumRecordsNil},
		{"UUID_nil", mockGetVolumeResponseUUIDNil},
		{"invalidUnixPermission", mockModifyFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			if test.name != "invalidUnixPermission" {
				err := rs.modifyVolumeUnixPermissionsByNameAndStyle(ctx, "fakeVolume",
					"---rwxrwxrwx", models.VolumeStyleFlexvol)
				assert.Error(t, err, "unix permission modified on volume")
			} else {
				err := rs.modifyVolumeUnixPermissionsByNameAndStyle(ctx, "fakeVolume",
					"invalidUnixPermission", models.VolumeStyleFlexvol)
				assert.Error(t, err, "unix permission modified on volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_SetVolumeCommentByNameAndStyle(t *testing.T) {
	tests := []struct {
		name         string
		mockFunction func(w http.ResponseWriter, r *http.Request)
	}{
		{"modified_fail", mockModifyFailed},
		{"getVolume_fail", mockGetVolumeResponseNumRecordsNil},
		{"UUID_nil", mockGetVolumeResponseUUIDNil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.setVolumeCommentByNameAndStyle(ctx, "fakeVolume", "newVolumeComment", models.VolumeStyleFlexvol)
			assert.Error(t, err, "comment is updated on volume")

			server.Close()
		})
	}
}

func TestOntapREST_convertUnixPermissions(t *testing.T) {
	tests := []struct {
		permission string
		expected   string
	}{
		{"---rwxrwxrwx", "777"},
		{"---rwxrwxrw", "rwxrwxrw"},
		{"---xwrxwrxwr", "xwrxwrxwr"},
		{"---rwrxwrxwr", "rwrxwrxwr"},
		{"---rrrxwrxwr", "rrrxwrxwr"},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("convertUnixPermissions %d", i), func(t *testing.T) {
			s := convertUnixPermissions(test.permission)
			assert.Equal(t, test.expected, s)
		})
	}
}

func TestOntapRestSetVolumeQosPolicyGroupNameByNameAndStyle_failure(t *testing.T) {
	tests := []struct {
		name          string
		mockFunction  func(w http.ResponseWriter, r *http.Request)
		qosPolicyName string
		qosPolicyKind QosPolicyGroupKindType
	}{
		{"ModifyQoSFailed", mockModifyFailed, "qosPolicy", QosPolicyGroupKind},
		{"GetVolumeNumRecordsNil", mockGetVolumeResponseNumRecordsNil, "qosPolicy", QosPolicyGroupKind},
		{"qosPolicyNameEmpty", mockModifyFailed, "", QosPolicyGroupKind},
		{"InvalidQosPolicy", mockModifyFailed, "qosPolicy", InvalidQosPolicyGroupKind},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.setVolumeQosPolicyGroupNameByNameAndStyle(ctx, "fakeVolume", QosPolicyGroup{
				Name: test.qosPolicyName,
				Kind: test.qosPolicyKind,
			}, models.VolumeStyleFlexvol)
			assert.Error(t, err, "qos policy is updated on volume")

			server.Close()
		})
	}
}

func TestOntapRestStartCloneSplitByNameAndStyle(t *testing.T) {
	tests := []struct {
		name         string
		mockFunction func(w http.ResponseWriter, r *http.Request)
	}{
		{"SplitCloneFail", mockModifyFailed},
		{"GetVolumeNumRecordsNil", mockGetVolumeResponseNumRecordsNil},
		{"GetVolumeUUIDNil", mockGetVolumeResponseUUIDNil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.startCloneSplitByNameAndStyle(ctx, "fakeVolume", models.VolumeStyleFlexvol)
			assert.Error(t, err, "split clone updated on volume")

			server.Close()
		})
	}
}

func TestOntapREST_RestoreSnapshotByNameAndStyle(t *testing.T) {
	tests := []struct {
		name         string
		mockFunction func(w http.ResponseWriter, r *http.Request)
	}{
		{"RestoreSnapshot", mockModifyFailed},
		{"NumRecordsFieldNilInResponse", mockGetVolumeResponseNumRecordsNil},
		{"UUIDNilInResponse", mockGetVolumeResponseUUIDNil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.restoreSnapshotByNameAndStyle(ctx, "fakeSnapshot", "fakeVolume", models.VolumeStyleFlexvol)
			assert.Error(t, err, "split clone updated on volume")

			server.Close()
		})
	}
}

func TestOntapREST_VolumeDisableSnapshotDirectoryAccess(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTestCase", mockGetVolumeResponseAccepted, false},
		{"NumRecordsMoreThanTwo", mockGetVolumeResponseNumRecordsMoreThanTwo, true},
		{"NumRecordsFieldsNil", mockGetVolumeResponseNumRecordsNil, true},
		{"UUIDNil", mockGetVolumeResponseUUIDNil, true},
		{"VolumeDisableSnapshotDirectoryAccess_Fail", mockModifyFailed, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.VolumeModifySnapshotDirectoryAccess(ctx, "fakeVolume", false)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the volume info")
			} else {
				assert.Error(t, err, "get the volume info")
			}
			server.Close()
		})
	}
}

func TestOntapRestListAllVolumeNamesBackedBySnapshot(t *testing.T) {
	tests := []struct {
		name         string
		mockFunction func(w http.ResponseWriter, r *http.Request)
	}{
		{"GetVolumeNumRecordsNil", mockGetVolumeResponseNumRecordsNil},
		{"GetVolumeUUIDNil", mockGetVolumeResponseUUIDNil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			_, err := rs.listAllVolumeNamesBackedBySnapshot(ctx, "fakeVolume", "fakeSnapshot")
			assert.NoError(t, err, "could not get the volumes")

			server.Close()
		})
	}
}

func TestOntapRestCreateVolumeByStyleInvalidUnixPermission(t *testing.T) {
	encrypt := true

	server := httptest.NewServer(http.HandlerFunc(mockRequestAccepted))
	rs := newRestClient(server.Listener.Addr().String(), server.Client())
	assert.NotNil(t, rs)

	err := rs.createVolumeByStyle(ctx, "fakeVolume", 1073741824, []string{"aggr1"}, "spaceReserve",
		"fakeSnapshotPolicy", "invalidUnixPermission", "fake-exportpolicy", "unix", "fake-tier",
		"comment", QosPolicyGroup{Name: "qosPolicy", Kind: QosPolicyGroupKind}, &encrypt, 0, models.VolumeStyleFlexvol,
		false)
	assert.Error(t, err, "volume created")
	server.Close()
}

func TestOntapRESTVolumeList(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			fields := []string{
				"type", "size", "comment", "aggregates", "nas", "guarantee",
				"snapshot_policy", "snapshot_directory_access_enabled",
				"space.snapshot.used", "space.snapshot.reserve_percent",
			}
			volume, err := rs.VolumeList(ctx, "fakeVolume", fields)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get a volume")
				assert.Equal(t, "fakeVolume", *volume.Payload.VolumeResponseInlineRecords[0].Name)
				assert.Equal(t, "fakeUUID", *volume.Payload.VolumeResponseInlineRecords[0].UUID)
				assert.Equal(t, int64(1073741824), *volume.Payload.VolumeResponseInlineRecords[0].Size)
				assert.Equal(t, "rw", *volume.Payload.VolumeResponseInlineRecords[0].Type)
				assert.Equal(t, int64(777), *volume.Payload.VolumeResponseInlineRecords[0].Nas.UnixPermissions)
				assert.Equal(t, "fakeAggr", *volume.Payload.VolumeResponseInlineRecords[0].
					VolumeInlineAggregates[0].Name)

			} else {
				assert.Error(t, err, "get the volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_VolumeExists(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			volumePresent, err := rs.VolumeExists(ctx, "fakeVolume")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get a volume")
				assert.Equal(t, true, volumePresent)
			} else {
				assert.Error(t, err, "get the volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_VolumeGetByName(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			fields := []string{"size", "type", "nas.unix_permissions", "aggregates"}
			volume, err := rs.VolumeGetByName(ctx, "fakeVolume", fields)

			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get a volume")
				assert.Equal(t, "fakeVolume", *volume.Name)
				assert.Equal(t, "fakeUUID", *volume.UUID)
				assert.Equal(t, int64(1073741824), *volume.Size)
				assert.Equal(t, "rw", *volume.Type)
				assert.Equal(t, int64(777), *volume.Nas.UnixPermissions)
				assert.Equal(t, "fakeAggr", *volume.VolumeInlineAggregates[0].Name)
			} else {
				assert.Error(t, err, "get the volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_VolumeMount(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.VolumeMount(ctx, "fakeVolume", "/fakeVolume")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not mount a volume")
			} else {
				assert.Error(t, err, "volume mounted")
			}
			server.Close()
		})
	}
}

func TestOntapREST_VolumeRename(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.VolumeRename(ctx, "fakeVolume", "fake-policy")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not rename a volume")
			} else {
				assert.Error(t, err, "volume renamed")
			}
			server.Close()
		})
	}
}

func TestOntapREST_VolumeModifyExportPolicy(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.VolumeModifyExportPolicy(ctx, "fakeVolume", "fake-policy")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not update export policy on volume")
			} else {
				assert.Error(t, err, "the the volume volume export policy")
			}
			server.Close()
		})
	}
}

func TestOntapREST_VolumeSize(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			size, err := rs.VolumeSize(ctx, "fakeVolume")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get size on volume")
				assert.Equal(t, uint64(1073741824), size)
			} else {
				assert.Error(t, err, "get the volume size")
			}
			server.Close()
		})
	}
}

func TestOntapRestVolumeUsedSize(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"GetVolume", mockGetVolumeResponse, false},
		{"GetVolumeBackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			size, err := rs.VolumeUsedSize(ctx, "fakeVolume")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get used size on volume")
				assert.Equal(t, 1073741824, size)
			} else {
				assert.Error(t, err, "get the volume used size")
			}
			server.Close()
		})
	}
}

func TestOntapREST_VolumeSetSize(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.VolumeSetSize(ctx, "fakeVolume", "1073741824")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not modify size on volume")
			} else {
				assert.Error(t, err, "size updated on volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_VolumeModifyUnixPermissions(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.VolumeModifyUnixPermissions(ctx, "fakeVolume", "---rwxr-xrwx")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not modify unix permission on volume")
			} else {
				assert.Error(t, err, "unix permission updated on volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_VolumeSetComment(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.VolumeSetComment(ctx, "fakeVolume", "newVolumeComment")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not modify comment on volume")
			} else {
				assert.Error(t, err, "comment updated on volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_VolumeSetQosPolicyGroupName(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.VolumeSetQosPolicyGroupName(ctx, "fakeVolume", QosPolicyGroup{Name: "qosPolicy", Kind: QosPolicyGroupKind})
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not modify qos policy on volume")
			} else {
				assert.Error(t, err, "qos policy updated")
			}
			server.Close()
		})
	}
}

func TestOntapREST_VolumeCloneSplitStart(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.VolumeCloneSplitStart(ctx, "fakeVolume")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not clone the volume")
			} else {
				assert.Error(t, err, "volume cloned")
			}
			server.Close()
		})
	}
}

func TestOntapREST_VolumeDestroy(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.VolumeDestroy(ctx, "fakeVolume", false)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not delete the volume")
			} else {
				assert.Error(t, err, "volume deleted")
			}
			server.Close()
		})
	}
}

func getHttpServerPollCreateVolumeJob(
	mockPostRequest func(w http.ResponseWriter, r *http.Request),
	mockGetRequest func(w http.ResponseWriter, r *http.Request),
) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/storage/volumes" {
			mockPostRequest(w, r)
		} else if r.Method == "GET" && r.URL.Path == "/api/cluster/jobs/1234" {
			mockJobResponse(w, r)
		} else if r.Method == "GET" && r.URL.Path == "/api/storage/volumes" {
			mockGetRequest(w, r)
		} else {
			mockResourceNotFound(w, r)
		}
	}))
}

func TestOntapREST_CreateFlexGroup(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockRequestAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encrypt := true
			server := getHttpServerPollCreateVolumeJob(test.mockFunction, mockGetVolumeResponse)
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			volumeParam := &storage.VolumeCollectionGetParams{}
			volumeParam.Context = ctx

			err := rs.FlexGroupCreate(ctx, "fakeVolume", 1073741824, []string{"aggr1"}, "spaceReserve",
				"fakeSnapshotPolicy", "---rwxr-xr-x", "fake-exportpolicy", "unix", "fake-tier",
				"comment", QosPolicyGroup{Name: "qosPolicy", Kind: QosPolicyGroupKind}, &encrypt, 0)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not create a flexgroup volume")
			} else {
				assert.Error(t, err, "flexgroup volume created")
			}
			server.Close()
		})
	}
}

func TestOntapREST_PollJobSuccess(t *testing.T) {
	server := getHttpServerPollCreateVolumeJob(mockRequestAccepted, mockGetVolumeResponse)
	rs := newRestClient(server.Listener.Addr().String(), server.Client())
	assert.NotNil(t, rs)

	jobId := strfmt.UUID("1234")
	jobLink := models.JobLink{UUID: &jobId}
	jobResponse := models.JobLinkResponse{Job: &jobLink}

	err := rs.PollJobStatus(ctx, &jobResponse)
	assert.NoError(t, err, "Poll job failed")
	server.Close()
}

func TestOntapREST_PollJobFailed(t *testing.T) {
	tests := []struct {
		name            string
		isErrorExpected bool
	}{
		{"JobIsEmpty", true},
		{"JobUUIDEmpty", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(mockJobResponse))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			jobResponse := models.JobLinkResponse{}
			if test.name == "JobUUIDEmpty" {
				jobResponse = models.JobLinkResponse{Job: &models.JobLink{}}
			}

			err := rs.PollJobStatus(ctx, &jobResponse)
			assert.Error(t, err, "job or job uuid is not empty")
			server.Close()
		})
	}
}

func TestOntapREST_FlexgroupCloneSplitStart(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.FlexgroupCloneSplitStart(ctx, "fakeVolume")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not split clone a flexgroup volume")
			} else {
				assert.Error(t, err, "split clone a flexgroup volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_FlexGroupDestroy(t *testing.T) {
	type parameters struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}
	tests := map[string]parameters{
		"happy path": {
			mockFunction:    mockGetVolumeResponseAccepted,
			isErrorExpected: false,
		},
		"backend returns error": {
			mockFunction:    mockResourceNotFound,
			isErrorExpected: true,
		},
		"numRecords Field is nil": {
			mockFunction:    mockGetVolumeResponseNumRecordsNil,
			isErrorExpected: false,
		},
		"error deleting flexgroup": {
			mockFunction:    mockModifyFailed,
			isErrorExpected: true,
		},
	}
	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(params.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.FlexGroupDestroy(ctx, "fakeVolume", false)
			if !params.isErrorExpected {
				assert.NoError(t, err, "could not delete a flexgroup volume")
			} else {
				assert.Error(t, err, "delete a flexgroup volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_FlexGroupExists(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			volumePresent, err := rs.FlexGroupExists(ctx, "fakeVolume")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get a flexgroup volume")
				assert.Equal(t, true, volumePresent)
			} else {
				assert.Error(t, err, "get a flexgroup volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_FlexGroupSize(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			size, err := rs.FlexGroupSize(ctx, "fakeVolume")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get size on flexgroup volume")
				assert.Equal(t, uint64(1073741824), size)
			} else {
				assert.Error(t, err, "get the size  on the flexgroup volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_FlexGroupUsedSize(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			size, err := rs.FlexGroupUsedSize(ctx, "fakeVolume")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get used size on flexgroup volume")
				assert.Equal(t, 1073741824, size)
			} else {
				assert.Error(t, err, "get the used size  on the flexgroup volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_FlexGroupSetSize(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.FlexGroupSetSize(ctx, "fakeVolume", "1073741824")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not update size on flexgroup volume")
			} else {
				assert.Error(t, err, "update the size on the flexgroup volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_FlexGroupQosPolicyGroupName(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.FlexgroupSetQosPolicyGroupName(ctx, "fakeVolume", QosPolicyGroup{Name: "qosPolicy", Kind: QosPolicyGroupKind})
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not update a qos policy on flexgroup volume")
			} else {
				assert.Error(t, err, "update a qos policy  on the flexgroup volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_FlexGroupVolumeDisableSnapshotDirectoryAccess(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
		{"NumRecordsFieldNil", mockGetVolumeResponseNumRecordsNil, true},
		{"UUIDNil", mockGetVolumeResponseUUIDNil, true},
		{"VolumeDisableSnapshotDirectoryAccess_Fail", mockModifyFailed, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.FlexGroupVolumeModifySnapshotDirectoryAccess(ctx, "fakeVolume", false)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not disable snapshot dir access on flexgroup volume")
			} else {
				assert.Error(t, err, "disable the snapshot dir access on the flexgroup volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_FlexGroupModifyUnixPermissions(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.FlexGroupModifyUnixPermissions(ctx, "fakeVolume", "---rwxr-xrwx")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not update the unix permission on flexgroup volume")
			} else {
				assert.Error(t, err, "update the unix permission on the flexgroup volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_FlexGroupSetComment(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.FlexGroupSetComment(ctx, "fakeVolume", "newVolumeComment")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not update the comment on flexgroup volume")
			} else {
				assert.Error(t, err, "update the comment on the flexgroup volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_FlexGroupGetByName(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			fields := []string{
				"type", "size", "comment", "aggregates", "nas", "guarantee", "snapshot_policy",
				"snapshot_directory_access_enabled", "space.snapshot.used", "space.snapshot.reserve_percent",
			}
			volume, err := rs.FlexGroupGetByName(ctx, "fakeVolume", fields)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the flexgroup volume")
				assert.Equal(t, "fakeVolume", *volume.Name)
				assert.Equal(t, "fakeUUID", *volume.UUID)
				assert.Equal(t, int64(1073741824), *volume.Size)
				assert.Equal(t, "rw", *volume.Type)
				assert.Equal(t, int64(777), *volume.Nas.UnixPermissions)
				assert.Equal(t, "fakeAggr", *volume.VolumeInlineAggregates[0].Name)
			} else {
				assert.Error(t, err, "get the flexgroup volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_FlexGroupGetAll(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponse, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			fields := []string{
				"type", "size", "comment", "aggregates", "nas", "guarantee",
				"snapshot_policy", "snapshot_directory_access_enabled",
				"space.snapshot.used", "space.snapshot.reserve_percent",
			}
			volume, err := rs.FlexGroupGetAll(ctx, "fakeVolume", fields)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the flexgroup volume")
				assert.Equal(t, "fakeVolume", *volume.Payload.VolumeResponseInlineRecords[0].Name)
				assert.Equal(t, "fakeUUID", *volume.Payload.VolumeResponseInlineRecords[0].UUID)
				assert.Equal(t, int64(1073741824), *volume.Payload.VolumeResponseInlineRecords[0].Size)
				assert.Equal(t, "rw", *volume.Payload.VolumeResponseInlineRecords[0].Type)
				assert.Equal(t, int64(777), *volume.Payload.VolumeResponseInlineRecords[0].Nas.UnixPermissions)
				assert.Equal(t, "fakeAggr", *volume.Payload.VolumeResponseInlineRecords[0].
					VolumeInlineAggregates[0].Name)
			} else {
				assert.Error(t, err, "get the flexgroup volume")
			}
			server.Close()
		})
	}
}

func TestOntapREST_FlexGroupMount(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.FlexGroupMount(ctx, "fakeVolume", "/fakeVolume")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not mount the flexgroup")
			} else {
				assert.Error(t, err, "mount the flexgroup")
			}
			server.Close()
		})
	}
}

func TestOntapREST_FlexGroupUnmount(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.FlexgroupUnmount(ctx, "fakeVolume")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not unmount the flexgroup")
			} else {
				assert.Error(t, err, "unmount the flexgroup")
			}
			server.Close()
		})
	}
}

func TestOntapREST_FlexGroupModifyExportPolicy(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockGetVolumeResponseAccepted, false},
		{"BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.FlexgroupModifyExportPolicy(ctx, "fakeVolume", "fake-policy")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not modify flexgroup")
			} else {
				assert.Error(t, err, "modified flexgroup")
			}
			server.Close()
		})
	}
}

func TestOntapREST_VolumeListByAttrs(t *testing.T) {
	volumeName := "fakeVolume"
	volumeUUID := "fakeUUID"
	volumeType := "rw"
	aggregates := "fakeAggr"
	comment := ""
	path := "/fakeVolume"
	unixPermission := "---rwxr-xr--"
	exportPolicy := "fakeExportPolicy"
	size := "1073741824"
	snapshotPolicy := "fakeSnapshotPolicy"
	snapshotDir := false
	encrypt := false

	server := httptest.NewServer(http.HandlerFunc(mockGetVolumeResponse))
	rs := newRestClient(server.Listener.Addr().String(), server.Client())
	assert.NotNil(t, rs)

	volume := Volume{
		AccessType:        volumeType,
		Aggregates:        []string{aggregates},
		Comment:           comment,
		Encrypt:           &encrypt,
		ExportPolicy:      exportPolicy,
		JunctionPath:      path,
		Name:              volumeName,
		Qos:               QosPolicyGroup{Name: "qosPolicy", Kind: QosPolicyGroupKind},
		SecurityStyle:     "unix",
		Size:              size,
		SnapshotDir:       convert.ToPtr(snapshotDir),
		SnapshotPolicy:    snapshotPolicy,
		SnapshotReserve:   0,
		SnapshotSpaceUsed: 1073741810,
		SpaceReserve:      "spaceReserve",
		TieringPolicy:     "fakeTieringPolicy",
		UnixPermissions:   unixPermission,
		UUID:              volumeUUID,
		DPVolume:          true,
	}

	volumeParam := &storage.VolumeCollectionGetParams{}
	volumeParam.Context = ctx
	volumes, err := rs.VolumeListByAttrs(ctx, &volume, []string{""})
	assert.NoError(t, err)
	assert.Equal(t, volumeName, *volumes.Payload.VolumeResponseInlineRecords[0].Name)

	volumes, err = rs.VolumeListByAttrs(ctx, &Volume{}, []string{""})
	assert.NoError(t, err)
	server.Close()
}

func getQtree() models.Qtree {
	id := int64(1)
	name := "qtree_vol1"
	securityStyle := models.SecurityStyleUnix
	unixPermission := int64(777)
	exportPolicy := "fake-export-policy"
	volumeName := "vol1"
	volumeUUID := "vol1UUID"
	svmname := "svm1"

	qtreeExportPolicy := models.QtreeInlineExportPolicy{Name: &exportPolicy}
	qtreeSVM := models.QtreeInlineSvm{Name: &svmname}
	qtreeVolume := models.QtreeInlineVolume{Name: &volumeName, UUID: &volumeUUID}

	return models.Qtree{
		ID:              &id,
		Name:            &name,
		SecurityStyle:   &securityStyle,
		UnixPermissions: &unixPermission,
		ExportPolicy:    &qtreeExportPolicy,
		Svm:             &qtreeSVM,
		Volume:          &qtreeVolume,
	}
}

func mockQtreeJobResponse(w http.ResponseWriter, r *http.Request) {
	jobId := strfmt.UUID("1234")
	jobStatus := models.JobStateSuccess
	jobLink := models.Job{UUID: &jobId, State: &jobStatus}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(jobLink)
}

func mockQtreeResourceNotFound(w http.ResponseWriter, r *http.Request) {
	setHTTPResponseHeader(w, http.StatusNotFound)
	json.NewEncoder(w).Encode("")
}

func mockQtreeResponse(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST", "DELETE", "PATCH":
		jobId := strfmt.UUID("1234")
		jobLink := models.JobLink{UUID: &jobId}
		jobResponse := models.JobLinkResponse{Job: &jobLink}
		setHTTPResponseHeader(w, http.StatusAccepted)
		json.NewEncoder(w).Encode(jobResponse)
	case "GET":
		if r.URL.Path == "/api/cluster/jobs/1234" {
			mockQtreeJobResponse(w, r)
		} else {
			qtree := getQtree()
			numRecords := int64(1)
			qtreeResponse := &models.QtreeResponse{
				QtreeResponseInlineRecords: []*models.Qtree{&qtree},
				NumRecords:                 &numRecords,
			}
			setHTTPResponseHeader(w, http.StatusOK)
			json.NewEncoder(w).Encode(qtreeResponse)
		}
	}
}

func mockQtreeOKResponse(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST", "DELETE", "PATCH":
		setHTTPResponseHeader(w, http.StatusOK)
	case "GET":
		if r.URL.Path == "/api/cluster/jobs/1234" {
			mockQtreeJobResponse(w, r)
		} else {
			qtree := getQtree()
			numRecords := int64(1)
			qtreeResponse := &models.QtreeResponse{
				QtreeResponseInlineRecords: []*models.Qtree{&qtree},
				NumRecords:                 &numRecords,
			}
			setHTTPResponseHeader(w, http.StatusOK)
			json.NewEncoder(w).Encode(qtreeResponse)
		}
	}
}

func mockQtreeCreatedResponse(w http.ResponseWriter, r *http.Request) {
	setHTTPResponseHeader(w, http.StatusCreated)
}

func mockJobResponseFailure(w http.ResponseWriter, r *http.Request) {
	jobId := strfmt.UUID("1234")
	jobLink := models.JobLink{UUID: &jobId}
	jobResponse := models.JobLinkResponse{Job: &jobLink}
	setHTTPResponseHeader(w, http.StatusInternalServerError)
	json.NewEncoder(w).Encode(jobResponse)
}

func mockQtreeResponseFailure(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "PATCH", "POST", "DELETE":
		mockJobResponseFailure(w, r)
	case "GET":
		qtree := getQtree()
		numRecords := int64(1)
		qtreeResponse := &models.QtreeResponse{
			QtreeResponseInlineRecords: []*models.Qtree{&qtree},
			NumRecords:                 &numRecords,
		}
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(qtreeResponse)
	}
}

func mockQtreeResponseNumRecordsMoreThanOne(w http.ResponseWriter, r *http.Request) {
	qtree := getQtree()
	qtreeResponse := &models.QtreeResponse{
		QtreeResponseInlineRecords: []*models.Qtree{&qtree, nil},
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(qtreeResponse)
}

func mockQtreeResponseNumRecordsNil(w http.ResponseWriter, r *http.Request) {
	qtreeResponse := &models.QtreeResponse{}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(qtreeResponse)
}

func mockQtreeResponseUUIDNil(w http.ResponseWriter, r *http.Request) {
	qtree := getQtree()
	qtree.ID = nil
	qtreeResponse := &models.QtreeResponse{
		QtreeResponseInlineRecords: []*models.Qtree{&qtree},
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(qtreeResponse)
}

func mockQtreeResponseVolumeUUIdNil(w http.ResponseWriter, r *http.Request) {
	qtree := getQtree()
	qtree.Volume.UUID = nil
	qtreeResponse := &models.QtreeResponse{
		QtreeResponseInlineRecords: []*models.Qtree{&qtree},
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(qtreeResponse)
}

func getQuotaRule() *models.QuotaRuleResponse {
	hardLimit := int64(1073741810)
	quotaVolume := models.QuotaRuleInlineVolume{Name: convert.ToPtr("quotaVolumeName")}
	quotaQtree := models.QuotaRuleInlineQtree{Name: convert.ToPtr("quotaQtree")}
	quotaSpace := models.QuotaRuleInlineSpace{HardLimit: &hardLimit}
	quotaRule := models.QuotaRule{
		Volume: &quotaVolume, Qtree: &quotaQtree, Space: &quotaSpace,
		UUID: convert.ToPtr("QuotaUUID"),
	}
	quotaRuleResponseInlineRecords := []*models.QuotaRule{&quotaRule}
	numRecords := int64(1)
	return &models.QuotaRuleResponse{
		QuotaRuleResponseInlineRecords: quotaRuleResponseInlineRecords,
		NumRecords:                     &numRecords,
	}
}

func mockQuotaRuleResponse(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/cluster/jobs/1234" {
		mockQtreeJobResponse(w, r)
	} else if r.Method == "PATCH" || r.Method == "POST" {
		jobId := strfmt.UUID("1234")
		jobLink := models.JobLink{UUID: &jobId}
		jobResponse := models.JobLinkResponse{Job: &jobLink}
		setHTTPResponseHeader(w, http.StatusAccepted)
		json.NewEncoder(w).Encode(jobResponse)
	} else {
		quotaRuleResponse := getQuotaRule()
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(quotaRuleResponse)
	}
}

func mockQuotaRuleOKResponse(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/cluster/jobs/1234" {
		mockQtreeJobResponse(w, r)
	} else if r.Method == "PATCH" || r.Method == "POST" {
		setHTTPResponseHeader(w, http.StatusOK)
	} else {
		quotaRuleResponse := getQuotaRule()
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(quotaRuleResponse)
	}
}

func mockQuotaRuleCreatedResponse(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/cluster/jobs/1234" {
		mockQtreeJobResponse(w, r)
	} else if r.Method == "PATCH" || r.Method == "POST" {
		setHTTPResponseHeader(w, http.StatusCreated)
	} else {
		quotaRuleResponse := getQuotaRule()
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(quotaRuleResponse)
	}
}

func mockQuotaRuleResponseFailure(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/cluster/jobs/1234" {
		mockQtreeJobResponse(w, r)
	} else {
		switch r.Method {
		case "PATCH", "POST":
			mockJobResponseFailure(w, r)
		default:
			quotaRuleResponse := getQuotaRule()
			setHTTPResponseHeader(w, http.StatusOK)
			json.NewEncoder(w).Encode(quotaRuleResponse)
		}
	}
}

func mockQuotaRuleResponseUUIDNil(w http.ResponseWriter, r *http.Request) {
	quotaRuleResponse := getQuotaRule()
	// Backend return UUID Nil in response.
	quotaRuleResponse.QuotaRuleResponseInlineRecords[0].UUID = nil
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(quotaRuleResponse)
}

func mockQtreeListResponse(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	qtree := getQtree()
	numRecords := int64(1)
	url := "/api/storage/qtrees"

	var hrefLink *models.QtreeResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.QtreeResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
	}

	qtreeResponse := &models.QtreeResponse{
		QtreeResponseInlineRecords: []*models.Qtree{&qtree},
		NumRecords:                 &numRecords,
		Links:                      hrefLink,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(qtreeResponse)
}

func mockQtreeListResponseQtreeExists(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	qtree := getQtree()
	numRecords := int64(1)
	url := "/api/storage/qtrees"

	var hrefLink *models.QtreeResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.QtreeResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
	}

	qtreeResponse := &models.QtreeResponse{
		QtreeResponseInlineRecords: []*models.Qtree{&qtree},
		NumRecords:                 &numRecords,
		Links:                      hrefLink,
	}

	if !hasNextLink {
		qtreeResponse.NumRecords = convert.ToPtr(int64(0))
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(qtreeResponse)
}

func mockQtreeListResponseVolumeNil(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	qtree := getQtree()
	qtree.Volume = nil
	url := "/api/storage/qtrees"

	var hrefLink *models.QtreeResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.QtreeResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
	}

	qtreeResponse := &models.QtreeResponse{
		QtreeResponseInlineRecords: []*models.Qtree{&qtree},
		NumRecords:                 convert.ToPtr(int64(1)),
		Links:                      hrefLink,
	}

	if !hasNextLink {
		qtreeResponse.NumRecords = convert.ToPtr(int64(0))
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(qtreeResponse)
}

func mockQtreeListResponseInternalError(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	qtree := getQtree()
	numRecords := int64(1)
	url := "/api/storage/qtrees"

	var hrefLink *models.QtreeResponseInlineLinks
	sc := http.StatusInternalServerError
	if hasNextLink {
		hrefLink = &models.QtreeResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		sc = http.StatusOK
	}

	qtreeResponse := &models.QtreeResponse{
		QtreeResponseInlineRecords: []*models.Qtree{&qtree},
		NumRecords:                 &numRecords,
		Links:                      hrefLink,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(sc)
	json.NewEncoder(w).Encode(qtreeResponse)
}

func mockQtreeListResponseNumRecordsNil(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	qtree := getQtree()
	url := "/api/storage/qtrees"

	var hrefLink *models.QtreeResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.QtreeResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
	}

	qtreeResponse := &models.QtreeResponse{
		QtreeResponseInlineRecords: []*models.Qtree{&qtree},
		Links:                      hrefLink,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(qtreeResponse)
}

func TestOntapREST_QtreeCreate(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		unixPermission  string
		isErrorExpected bool
	}{
		{"PositiveTest", mockQtreeResponse, "---rwxr--rwx", false},
		{"PositiveTestSync", mockQtreeCreatedResponse, "---rwxr--rwx", false},
		{"UnixPermissionValueInvalid", mockQtreeResponse, "invalidValue", true},
		{"NegativeTest_BackendReturnError", mockQtreeResourceNotFound, "---rwxr--rwx", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.QtreeCreate(ctx, "qtree_vol1", "vol1", test.unixPermission, "fake-export-policy", "unix",
				"qosPolicy")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not create qtree by name")
			} else {
				assert.Error(t, err, "Qtree created")
			}
			server.Close()
		})
	}
}

func TestOntapREST_QtreeGetByPath(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockQtreeResponse, false},
		{"NegativeTest_BackendReturnError", mockQtreeResourceNotFound, true},
		{"ResponseContainMoreThanOneRecord", mockQtreeResponseNumRecordsMoreThanOne, true},
		{"NumRecordsFieldNilInResponse", mockQtreeResponseNumRecordsNil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			qtree, err := rs.QtreeGetByPath(ctx, "/vol1/qtree_vol1", []string{""})
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get qtree by name")
				assert.Equal(t, "qtree_vol1", *qtree.Name)
			} else {
				assert.Error(t, err, "Get qtree by name")
			}
			server.Close()
		})
	}
}

func TestOntapREST_QtreeRename(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockQtreeResponse, false},
		{"PositiveTestSync", mockQtreeOKResponse, false},
		{"ResposeContainUUIDNil", mockQtreeResponseUUIDNil, true},
		{"ResposeContainVolumeIdNil", mockQtreeResponseVolumeUUIdNil, true},
		{"NegativeTest_BackendReturnError", mockQtreeResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.QtreeRename(ctx, "/vol1/qtree_vol1", "/vol2/qtree_vol2")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not delete the rename")
			} else {
				assert.Error(t, err, "qtree renamed")
			}
			server.Close()
		})
	}
}

func TestOntapQtree_DestroyAsync(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockQtreeResponse, false},
		{"PositiveTestSync", mockQtreeOKResponse, false},
		{"NegativeTest_BackendReturnError", mockQtreeResourceNotFound, true},
		{"ResposeContainUUIDNil", mockQtreeResponseUUIDNil, true},
		{"ResposeContainVolumeIdNil", mockQtreeResponseVolumeUUIdNil, true},
		{"DeleteFail", mockQtreeResponseFailure, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.QtreeDestroyAsync(ctx, "/vol1/qtree_vol1", false)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not delete the qtree")
			} else {
				assert.Error(t, err, "qtree deleted")
			}
			server.Close()
		})
	}
}

func TestOntapREST_QtreeList(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(hasNextLink bool, w http.ResponseWriter, r *http.Request)
		isNegativeTest  bool
		isErrorExpected bool
	}{
		{"PositiveTest", mockQtreeListResponse, false, false},
		{"NumRecordsFieldNil", mockQtreeListResponseNumRecordsNil, false, false},
		{"HrefSecondGetCallFail", mockQtreeListResponseInternalError, false, true},
		{"NegativeTest_BackendReturnError", mockInternalServerError, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := getHttpServer(test.isNegativeTest, test.mockFunction)
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			fields := []string{"name", "volume"}
			qtree, err := rs.QtreeList(ctx, "qtree_", "volume_", fields)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the qtree list")
				assert.Equal(t, "qtree_vol1", *qtree.Payload.QtreeResponseInlineRecords[0].Name)
			} else {
				assert.Error(t, err, "get the qtree list")
			}
			server.Close()
		})
	}
}

func TestOntapREST_QtreeGetByName(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockQtreeResponse, false},
		{"NegativeTest_BackendReturnError", mockQtreeResourceNotFound, true},
		{"NumRecordsFieldNil", mockQtreeResponseNumRecordsNil, true},
		{"NumRecordsMoreThanOne", mockQtreeResponseNumRecordsMoreThanOne, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			qtree, err := rs.QtreeGetByName(ctx, "qtree_", "volume_")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the qtree by name")
				assert.Equal(t, "qtree_vol1", *qtree.Name)
			} else {
				assert.Error(t, err, "get the qtree by name")
			}
			server.Close()
		})
	}
}

func TestOntapREST_QtreeCount(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(hasNextLink bool, w http.ResponseWriter, r *http.Request)
		isNegativeTest  bool
		isErrorExpected bool
		expectedCount   int
	}{
		{"PositiveTest", mockQtreeListResponse, false, false, 1},
		{"CheckQtreeExists", mockQtreeListResponseQtreeExists, false, false, 0},
		{"NumRecordsFieldNilInResponse", mockQtreeListResponseNumRecordsNil, false, false, 0},
		{"HrefSecondGetCallFail", mockQtreeListResponseInternalError, false, true, 0},
		{"NegativeTest_BackendReturnError", mockInternalServerError, true, true, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := getHttpServer(test.isNegativeTest, test.mockFunction)
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			qtreeCount, err := rs.QtreeCount(ctx, "qtree_")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the count of qtree")
				assert.Equal(t, test.expectedCount, qtreeCount)
			} else {
				assert.Error(t, err, "get the count of qtree")
			}
			server.Close()
		})
	}
}

func TestOntapREST_QtreeExists(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(hasNextLink bool, w http.ResponseWriter, r *http.Request)
		isNegativeTest  bool
		qtreeExists     bool
		isErrorExpected bool
	}{
		{"PositiveTest", mockQtreeListResponseQtreeExists, false, true, false},
		{"NumRecordsFieldNilInResponse", mockQtreeListResponseNumRecordsNil, false, false, false},
		{"HrefSecondGetCallFail", mockQtreeListResponseInternalError, false, false, true},
		{"ParentVolumeNil", mockQtreeListResponseVolumeNil, false, false, false},
		{"NegativeTest_BackendReturnError", mockInternalServerError, true, false, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := getHttpServer(test.isNegativeTest, test.mockFunction)
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			qtreeExist, _, err := rs.QtreeExists(ctx, "qtree_", "vol1")
			if !test.isErrorExpected {
				assert.NoError(t, err, "qtree does not exists")
				assert.Equal(t, test.qtreeExists, qtreeExist)
			} else {
				assert.Error(t, err, "qtree exists")
			}
			server.Close()
		})
	}
}

func TestOntapREST_QtreeGet(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockQtreeResponse, false},
		{"NegativeTest_BackendReturnError", mockQtreeResourceNotFound, true},
		{"NumRecordsFieldNilInResponse", mockQtreeResponseNumRecordsNil, true},
		{"NumRecordsMoreThanOne", mockQtreeResponseNumRecordsMoreThanOne, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			qtree, err := rs.QtreeGet(ctx, "qtree_", "vol1")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the qtree")
				assert.Equal(t, "qtree_vol1", *qtree.Name)
			} else {
				assert.Error(t, err, "get the qtree")
			}
			server.Close()
		})
	}
}

func TestOntapREST_QtreeGetAll(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(hasNextLink bool, w http.ResponseWriter, r *http.Request)
		isNegativeTest  bool
		isErrorExpected bool
	}{
		{"PositiveTest", mockQtreeListResponse, false, false},
		{"NumRecordsFieldNilInResponse", mockQtreeListResponseNumRecordsNil, false, false},
		{"HrefSecondGetCallFail", mockQtreeListResponseInternalError, false, true},
		{"NegativeTest_BackendReturnError", mockInternalServerError, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := getHttpServer(test.isNegativeTest, test.mockFunction)
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			qtree, err := rs.QtreeGetAll(ctx, "volume_")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get all qtree")
				assert.Equal(t, "qtree_vol1", *qtree.Payload.QtreeResponseInlineRecords[0].Name)
			} else {
				assert.Error(t, err, "get all the qtree")
			}
			server.Close()
		})
	}
}

func TestOntapREST_QtreeModifyExportPolicy(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockQtreeResponse, false},
		{"PositiveTestSync", mockQtreeOKResponse, false},
		{"NegativeTest_BackendReturnError", mockQtreeResourceNotFound, true},
		{"UUIDNilInResponse", mockQtreeResponseUUIDNil, true},
		{"ParentVolumeIdNil", mockQtreeResponseVolumeUUIdNil, true},
		{"ModifyOperationFail", mockQtreeResponseFailure, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.QtreeModifyExportPolicy(ctx, "qtree_vol1", "volume_", "fake-exportpolicy")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not modify export policy")
			} else {
				assert.Error(t, err, "modified export policy")
			}
			server.Close()
		})
	}
}

func mockQuotaResourceNotFound(w http.ResponseWriter, r *http.Request) {
	setHTTPResponseHeader(w, http.StatusNotFound)
	json.NewEncoder(w).Encode("")
}

func TestOntapRest_QuotaOn(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockQuotaRuleResponse, false},
		{"NegativeTest_BackendReturnError", mockQuotaResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.QuotaOn(ctx, "fakeVolume")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not enable quota")
			} else {
				assert.Error(t, err, "quota enabled")
			}
			server.Close()
		})
	}
}

func TestOntapRest_QuotaOff(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockQuotaRuleResponse, false},
		{"NegativeTest_BackendReturnError", mockQuotaResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.QuotaOff(ctx, "fakeVolume")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not disable quota")
			} else {
				assert.Error(t, err, "quota enabled")
			}
			server.Close()
		})
	}
}

func TestOntapREST_QuotaModify(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockQuotaRuleResponse, false},
		{"PositiveTestSync", mockQuotaRuleOKResponse, false},
		{"NegativeTest_BackendReturnError", mockQuotaResourceNotFound, true},
		{"QuotaModifyFailed", mockQuotaRuleResponseFailure, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.quotaModify(ctx, "qtree_vol1", true)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not modify Quota")
			} else {
				assert.Error(t, err, "modified Quota")
			}
			server.Close()
		})
	}
}

func TestOntapRest_QuotaSetEntry(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		diskLimit       string
		isErrorExpected bool
	}{
		{"PositiveTest", mockQuotaRuleResponse, "1073741810", false},
		{"PositiveTestSync", mockQuotaRuleOKResponse, "1073741810", false},
		{"NegativeTest_BackendReturnError", mockQuotaResourceNotFound, "1073741810", true},
		{"UUIDNilInResponse", mockQuotaRuleResponseUUIDNil, "1073741810", true},
		{"ModifyQuotaRuleFail_DiskLimitNotEmpty", mockQuotaRuleResponseFailure, "1073741810", true},
		{"ModifyQuotaRuleFail_DiskLimitEmpty", mockQuotaRuleResponseFailure, "", true},
		{"ModifyQuotaRuleFail_InvalidDiskLimit", mockQuotaRuleResponseFailure, "invalid_disk_limit", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.QuotaSetEntry(ctx, "qtree_vol1", "fakeVolume", "user", test.diskLimit)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not set the quota entry list")
			} else {
				assert.Error(t, err, "set the quota entry list")
			}
			server.Close()
		})
	}
}

func TestOntapRest_QuotaAddEntry(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		diskLimit       string
		isErrorExpected bool
	}{
		{"PositiveTest", mockQuotaRuleResponse, "1073741810", false},
		{"PositiveTestSync", mockQuotaRuleCreatedResponse, "1073741810", false},
		{"InvalidDiskLimit", mockQuotaRuleResponse, "invalidValue", true},
		{"NegativeTest_BackendReturnError", mockQuotaResourceNotFound, "1073741810", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.QuotaAddEntry(ctx, "qtree_vol1", "fakeVolume", "user", test.diskLimit)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not add the quota entry list")
			} else {
				assert.Error(t, err, "add the quota entry list")
			}
			server.Close()
		})
	}
}

func mockQuotaRuleListResponse(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	url := "/api/storage/quota/rules"

	var hrefLink *models.QuotaRuleResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.QuotaRuleResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
	}

	quotaRuleResponse := getQuotaRule()
	quotaRuleResponse.Links = hrefLink

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(quotaRuleResponse)
}

func mockQuotaRuleListResponseInternalError(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	url := "/api/storage/quota/rules"

	var hrefLink *models.QuotaRuleResponseInlineLinks
	sc := http.StatusInternalServerError
	if hasNextLink {
		hrefLink = &models.QuotaRuleResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		sc = http.StatusOK
	}

	quotaRuleResponse := getQuotaRule()
	quotaRuleResponse.Links = hrefLink

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(sc)
	json.NewEncoder(w).Encode(quotaRuleResponse)
}

func mockQuotaRuleListResponseNumRecordsNil(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	url := "/api/storage/quota/rules"

	var hrefLink *models.QuotaRuleResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.QuotaRuleResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
	}

	quotaRuleResponse := getQuotaRule()
	quotaRuleResponse.NumRecords = nil
	quotaRuleResponse.Links = hrefLink

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(quotaRuleResponse)
}

func TestOntapRest_QuotaEntryList(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(hasNextLink bool, w http.ResponseWriter, r *http.Request)
		isNegativeTest  bool
		isErrorExpected bool
	}{
		{"PositiveTest", mockQuotaRuleListResponse, false, false},
		{"NumRecordsFieldNilInResponse", mockQuotaRuleListResponseNumRecordsNil, false, false},
		{"HrefSecondGetCallFail", mockQuotaRuleListResponseInternalError, false, true},
		{"NegativeTest_BackendReturnError", mockInternalServerError, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := getHttpServer(test.isNegativeTest, test.mockFunction)
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			quotaRule, err := rs.QuotaEntryList(ctx, "qtree_vol1")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the quota entry list")
				assert.Equal(t, "quotaVolumeName", *quotaRule.Payload.QuotaRuleResponseInlineRecords[0].Volume.Name)
			} else {
				assert.Error(t, err, "get the quota entry list")
			}
			server.Close()
		})
	}
}

func TestOntapRest_QuotaGetEntry(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(hasNextLink bool, w http.ResponseWriter, r *http.Request)
		isNegativeTest  bool
		isErrorExpected bool
	}{
		{"PositiveTest", mockQuotaRuleListResponse, false, true},
		{"NumRecordsFieldNilInResponse", mockQuotaRuleListResponseNumRecordsNil, false, false},
		{"HrefSecondCallFail", mockQuotaRuleListResponseInternalError, false, true},
		{"NegativeTest_BackendReturnError", mockInternalServerError, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := getHttpServer(test.isNegativeTest, test.mockFunction)
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			quotaRule, err := rs.QuotaGetEntry(ctx, "quotaVolumeName", "qtree_vol1", "user")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the quota entry list")
				assert.Equal(t, "quotaVolumeName", *quotaRule.Volume.Name)
			} else {
				assert.Error(t, err, "get the quota entry list")
			}
			server.Close()
		})
	}
}

func mockSnapMirrorRelationshipResponse(w http.ResponseWriter, r *http.Request) {
	snapMirrorRelationshipResponse := &models.SnapmirrorRelationshipResponse{
		SnapmirrorRelationshipResponseInlineRecords: []*models.SnapmirrorRelationship{
			{
				Destination: &models.SnapmirrorEndpoint{
					Path: convert.ToPtr("svm0:vol1"),
					Svm: &models.SnapmirrorEndpointInlineSvm{
						Name: convert.ToPtr("svm0"),
					},
				},
				Source: &models.SnapmirrorSourceEndpoint{
					Path: convert.ToPtr("svm1:vol1"),
					Svm: &models.SnapmirrorSourceEndpointInlineSvm{
						Name: convert.ToPtr("svm0"),
					},
				},
				UUID: convert.ToPtr(strfmt.UUID("1")),
			},
			{},
			{
				Destination: &models.SnapmirrorEndpoint{},
				Source:      &models.SnapmirrorSourceEndpoint{},
			},
			{
				Destination: &models.SnapmirrorEndpoint{Path: convert.ToPtr("svm0")},
				Source:      &models.SnapmirrorSourceEndpoint{Path: convert.ToPtr("svm1:vol1")},
			},
		},
	}

	if r.URL.Path == "/api/cluster/jobs/1234" {
		mockQtreeJobResponse(w, r)
	} else {
		switch r.Method {
		case "PATCH", "DELETE":
			jobId := strfmt.UUID("1234")
			jobLink := models.JobLink{UUID: &jobId}
			jobResponse := models.JobLinkResponse{Job: &jobLink}
			setHTTPResponseHeader(w, http.StatusAccepted)
			json.NewEncoder(w).Encode(jobResponse)
		case "POST":
			if r.URL.Path == "/api/snapmirror/relationships" {
				jobId := strfmt.UUID("1234")
				jobLink := models.JobLink{UUID: &jobId}
				jobResponse := models.JobLinkResponse{Job: &jobLink}
				setHTTPResponseHeader(w, http.StatusAccepted)
				json.NewEncoder(w).Encode(jobResponse)
			} else {
				setHTTPResponseHeader(w, http.StatusCreated)
				json.NewEncoder(w).Encode(snapMirrorRelationshipResponse)
			}
		default:
			setHTTPResponseHeader(w, http.StatusOK)
			json.NewEncoder(w).Encode(snapMirrorRelationshipResponse)
		}
	}
}

func mockSnapMirrorRelationshipResponseFailure(w http.ResponseWriter, r *http.Request) {
	snapMirrorRelationshipResponse := &models.SnapmirrorRelationshipResponse{
		SnapmirrorRelationshipResponseInlineRecords: []*models.SnapmirrorRelationship{
			{
				Destination: &models.SnapmirrorEndpoint{
					Path: convert.ToPtr("svm0:vol1"),
					Svm: &models.SnapmirrorEndpointInlineSvm{
						Name: convert.ToPtr("svm0"),
					},
				},
				Source: &models.SnapmirrorSourceEndpoint{
					Path: convert.ToPtr("svm1:vol1"),
					Svm: &models.SnapmirrorSourceEndpointInlineSvm{
						Name: convert.ToPtr("svm0"),
					},
				},
				UUID: convert.ToPtr(strfmt.UUID("1")),
			},
			{},
			{
				Destination: &models.SnapmirrorEndpoint{},
				Source:      &models.SnapmirrorSourceEndpoint{},
			},
			{
				Destination: &models.SnapmirrorEndpoint{Path: convert.ToPtr("svm0")},
				Source:      &models.SnapmirrorSourceEndpoint{Path: convert.ToPtr("svm1:vol1")},
			},
		},
	}

	if r.URL.Path == "/api/cluster/jobs/1234" {
		mockQtreeJobResponse(w, r)
	} else {
		switch r.Method {
		case "PATCH", "DELETE":
			jobId := strfmt.UUID("1234")
			jobLink := models.JobLink{UUID: &jobId}
			jobResponse := models.JobLinkResponse{Job: &jobLink}
			setHTTPResponseHeader(w, http.StatusInternalServerError)
			json.NewEncoder(w).Encode(jobResponse)
		case "POST":
			if r.URL.Path == "/api/snapmirror/relationships" {
				jobId := strfmt.UUID("1234")
				jobLink := models.JobLink{UUID: &jobId}
				jobResponse := models.JobLinkResponse{Job: &jobLink}
				setHTTPResponseHeader(w, http.StatusInternalServerError)
				json.NewEncoder(w).Encode(jobResponse)
			} else {
				setHTTPResponseHeader(w, http.StatusInternalServerError)
				json.NewEncoder(w).Encode(snapMirrorRelationshipResponse)
			}
		default:
			setHTTPResponseHeader(w, http.StatusOK)
			json.NewEncoder(w).Encode(snapMirrorRelationshipResponse)
		}
	}
}

func mockSnapMirrorRelationshipTransferResponseFailure(w http.ResponseWriter, r *http.Request) {
	snapmirrorTransferResponse := &models.SnapmirrorTransferResponse{
		SnapmirrorTransferResponseInlineRecords: []*models.SnapmirrorTransfer{
			{
				UUID: convert.ToPtr(strfmt.UUID("1234567899")),
			},
		},
	}

	if r.URL.Path == "/api/cluster/jobs/1234" {
		mockQtreeJobResponse(w, r)
	} else {
		switch r.Method {
		case "PATCH", "DELETE":
			jobId := strfmt.UUID("1234")
			jobLink := models.JobLink{UUID: &jobId}
			jobResponse := models.JobLinkResponse{Job: &jobLink}
			setHTTPResponseHeader(w, http.StatusInternalServerError)
			json.NewEncoder(w).Encode(jobResponse)
		case "POST":
			if r.URL.Path == "/api/snapmirror/relationships/1234/transfer" {
				jobId := strfmt.UUID("1234")
				jobLink := models.JobLink{UUID: &jobId}
				jobResponse := models.JobLinkResponse{Job: &jobLink}
				setHTTPResponseHeader(w, http.StatusInternalServerError)
				json.NewEncoder(w).Encode(jobResponse)
			} else {
				setHTTPResponseHeader(w, http.StatusInternalServerError)
				json.NewEncoder(w).Encode(snapmirrorTransferResponse)
			}
		default:
			setHTTPResponseHeader(w, http.StatusOK)
			json.NewEncoder(w).Encode(snapmirrorTransferResponse)
		}
	}
}

func mockSnapMirrorRelationshipResponseEmptyValue(w http.ResponseWriter, r *http.Request) {
	snapMirrorRelationshipResponse := &models.SnapmirrorRelationshipResponse{
		SnapmirrorRelationshipResponseInlineRecords: []*models.SnapmirrorRelationship{
			{},
			{
				Destination: &models.SnapmirrorEndpoint{},
				Source:      &models.SnapmirrorSourceEndpoint{},
			},
			{
				Destination: &models.SnapmirrorEndpoint{Path: convert.ToPtr("svm0")},
				Source:      &models.SnapmirrorSourceEndpoint{Path: convert.ToPtr("svm1")},
			},
		},
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(snapMirrorRelationshipResponse)
}

func mockSnapMirrorRelationshipResponseUUIDNil(w http.ResponseWriter, r *http.Request) {
	snapMirrorRelationshipResponse := &models.SnapmirrorRelationshipResponse{
		SnapmirrorRelationshipResponseInlineRecords: []*models.SnapmirrorRelationship{
			{
				Destination: &models.SnapmirrorEndpoint{
					Path: convert.ToPtr("svm0:vol1"),
					Svm: &models.SnapmirrorEndpointInlineSvm{
						Name: convert.ToPtr("svm0"),
					},
				},
				Source: &models.SnapmirrorSourceEndpoint{
					Path: convert.ToPtr("svm1:vol1"),
					Svm: &models.SnapmirrorSourceEndpointInlineSvm{
						Name: convert.ToPtr("svm0"),
					},
				},
				UUID: nil, // Test the use case where backend return UUID nil.
			},
		},
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(snapMirrorRelationshipResponse)
}

func mockSnapMirrorRelationshipResponseSync(w http.ResponseWriter, r *http.Request) {
	snapMirrorRelationshipResponse := &models.SnapmirrorRelationshipResponse{
		SnapmirrorRelationshipResponseInlineRecords: []*models.SnapmirrorRelationship{
			{
				Destination: &models.SnapmirrorEndpoint{
					Path: convert.ToPtr("svm0:vol1"),
					Svm: &models.SnapmirrorEndpointInlineSvm{
						Name: convert.ToPtr("svm0"),
					},
				},
				Source: &models.SnapmirrorSourceEndpoint{
					Path: convert.ToPtr("svm1:vol1"),
					Svm: &models.SnapmirrorSourceEndpointInlineSvm{
						Name: convert.ToPtr("svm0"),
					},
				},
				Policy: &models.SnapmirrorRelationshipInlinePolicy{Type: convert.ToPtr("sync")},
				UUID:   convert.ToPtr(strfmt.UUID("1")),
			},
		},
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(snapMirrorRelationshipResponse)
}

func TestOntapRestSnapmirrorCreate(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockSnapMirrorRelationshipResponse, false},
		{"NegativeTest_BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.SnapmirrorCreate(ctx, "vol1", "svm0", "vol1", "svm1", "fake-repPolicy", "fake-repSchedule")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not create snap mirror")
			} else {
				assert.Error(t, err, "snap mirror created")
			}
			server.Close()
		})
	}
}

func TestOntapRest_SnapmirrorInitialize(t *testing.T) {
	tests := []struct {
		name             string
		mockFunction     func(w http.ResponseWriter, r *http.Request)
		localFlexVolName string
		isErrorExpected  bool
	}{
		{"PositiveTest", mockSnapMirrorRelationshipResponse, "vol1", false},
		{"EmptyResponse_volumeNamePassInInput", mockSnapMirrorRelationshipResponseEmptyValue, "vol1", true},
		{"EmptyResponse_volumeNameEmptyInInput", mockSnapMirrorRelationshipResponseEmptyValue, "", true},
		{"SnapmirrorInitializeFailed", mockSnapMirrorRelationshipResponseFailure, "vol1", true},
		{"UUIDNilInResponse", mockSnapMirrorRelationshipResponseUUIDNil, "vol1", true},
		{"NegativeTest_BackendReturnError", mockResourceNotFound, "vol1", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.SnapmirrorInitialize(ctx, test.localFlexVolName, "svm0", "vol1", "svm1")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not initialize snap mirror")
			} else {
				assert.Error(t, err, "snap mirror initialized")
			}
			server.Close()
		})
	}
}

func TestOntapRest_SnapmirrorResync(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockSnapMirrorRelationshipResponse, false},
		{"UUIDNilInResponse", mockSnapMirrorRelationshipResponseUUIDNil, true},
		{"SnapMirrorRelationshipFailed", mockSnapMirrorRelationshipResponseFailure, true},
		{"SnapMirrorRelationshipSyncFailed", mockSnapMirrorRelationshipResponseSync, true},
		{"NegativeTest_BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.SnapmirrorResync(ctx, "vol1", "svm0", "vol1", "svm1")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not resync snap mirror")
			} else {
				assert.Error(t, err, "snap mirror resynced")
			}
			server.Close()
		})
	}
}

func TestOntapRest_SnapmirrorBreak(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockSnapMirrorRelationshipResponse, false},
		{"UUIDNilInResponse", mockSnapMirrorRelationshipResponseUUIDNil, true},
		{"SnapMirrorRelationshipFailed", mockSnapMirrorRelationshipResponseFailure, true},
		{"NegativeTest_BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.SnapmirrorBreak(ctx, "vol1", "svm0", "vol1", "svm1", "snapshot0")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not break snap mirror")
			} else {
				assert.Error(t, err, "snap mirror broke successfully")
			}
			server.Close()
		})
	}
}

func TestOntapRest_SnapmirrorQuiesce(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockSnapMirrorRelationshipResponse, false},
		{"UUIDNilInResponse", mockSnapMirrorRelationshipResponseUUIDNil, true},
		{"SnapMirrorRelationshipFailed", mockSnapMirrorRelationshipResponseFailure, true},
		{"NegativeTest_BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.SnapmirrorQuiesce(ctx, "vol1", "svm0", "vol1", "svm1")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not quiesce snap mirror")
			} else {
				assert.Error(t, err, "snap mirror quiesced")
			}
			server.Close()
		})
	}
}

func TestOntapRest_SnapmirrorAbort(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockSnapMirrorRelationshipResponse, false},
		{"UUIDNilInResponse", mockSnapMirrorRelationshipResponseUUIDNil, true},
		{"SnapMirrorRelationshipTransferFailed", mockSnapMirrorRelationshipTransferResponseFailure, true},
		{"NegativeTest_BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.SnapmirrorAbort(ctx, "vol1", "svm0", "vol1", "svm1")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not abort snap mirror")
			} else {
				assert.Error(t, err, "snap mirror aborted")
			}
			server.Close()
		})
	}
}

func TestOntapRest_SnapmirrorRelease(t *testing.T) {
	tests := []struct {
		name              string
		mockFunction      func(w http.ResponseWriter, r *http.Request)
		sourceFlexVolName string
		isErrorExpected   bool
	}{
		{"PositiveTest", mockSnapMirrorRelationshipResponse, "vol1", false},
		{"ResponseIsEmpty_VolumeNameisNotEmpty", mockSnapMirrorRelationshipResponseEmptyValue, "vol1", true},
		{"ResponseIsEmpty_VolumeNameisEmpty", mockSnapMirrorRelationshipResponseEmptyValue, "", true},
		{"SnapMirrorRelationshipFailed", mockSnapMirrorRelationshipResponseFailure, "vol1", true},
		{"UUIDNilInResponse", mockSnapMirrorRelationshipResponseUUIDNil, "vol1", true},
		{"NegativeTest_BackendReturnError", mockResourceNotFound, "vol1", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.SnapmirrorRelease(ctx, test.sourceFlexVolName, "svm1")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not release snap mirror")
			} else {
				assert.Error(t, err, "snap mirror released")
			}
			server.Close()
		})
	}
}

func TestOntapRest_SnapmirrorDeleteViaDestination(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockSnapMirrorRelationshipResponse, false},
		{"UUIDNilInResponse", mockSnapMirrorRelationshipResponseUUIDNil, true},
		{"SnapMirrorRelationshipFailed", mockSnapMirrorRelationshipResponseFailure, true},
		{"NegativeTest_BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.SnapmirrorDeleteViaDestination(ctx, "vol1", "svm1")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not delete snap mirror via destination")
			} else {
				assert.Error(t, err, "snap mirror deleted")
			}
			server.Close()
		})
	}
}

func TestOntapRest_SnapmirrorDelete(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockSnapMirrorRelationshipResponse, false},
		{"UUIDNilInResponse", mockSnapMirrorRelationshipResponseUUIDNil, true},
		{"SnapMirrorRelationshipFailed", mockSnapMirrorRelationshipResponseFailure, true},
		{"NegativeTest_BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.SnapmirrorDelete(ctx, "vol1", "svm0", "vol1", "svm1")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not delete snap mirror")
			} else {
				assert.Error(t, err, "snap mirror deleted")
			}
			server.Close()
		})
	}
}

func mockSnapMirrorPolicyResponse(w http.ResponseWriter, r *http.Request) {
	copyAllSourceSnapshots := false
	snapMirroePolicy := models.SnapmirrorPolicy{
		Name:                   convert.ToPtr("snapPolicy"),
		CopyAllSourceSnapshots: &copyAllSourceSnapshots,
		SyncType:               convert.ToPtr("sync_mirror"), Type: convert.ToPtr("sync"),
	}
	snapmirrorPolicyRecords := []*models.SnapmirrorPolicy{&snapMirroePolicy}
	snapmirrorPolicyResponse := models.SnapmirrorPolicyResponse{SnapmirrorPolicyResponseInlineRecords: snapmirrorPolicyRecords}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(snapmirrorPolicyResponse)
}

func TestOntapRest_SnapmirrorPolicyExists(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockSnapMirrorPolicyResponse, false},
		{"NegativeTest_BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			policyExists, err := rs.SnapmirrorPolicyExists(ctx, "snapPolicy")
			if !test.isErrorExpected {
				assert.Equal(t, true, policyExists)
				assert.NoError(t, err, "snapmirror policy does not exists")
			} else {
				assert.Error(t, err, "snapmirror policy exists")
			}
			server.Close()
		})
	}
}

func TestOntapRest_SnapmirrorUpdate(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockSnapMirrorRelationshipResponse, false},
		{"UUIDNilInResponse", mockSnapMirrorRelationshipResponseUUIDNil, true},
		{"SnapMirrorRelationshipFailed", mockSnapMirrorRelationshipResponseFailure, true},
		{"NegativeTest_BackendReturnError", mockResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.SnapmirrorUpdate(ctx, "vol1", "svm0")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not delete snap mirror")
			} else {
				assert.Error(t, err, "snap mirror deleted")
			}
			server.Close()
		})
	}
}

func mockSMBShareResponse(w http.ResponseWriter, r *http.Request) {
	numRecords := int64(1)
	switch r.Method {
	case "POST":
		smShareCreateResponse := n_a_s.CifsShareCreateCreated{Location: "/"}
		setHTTPResponseHeader(w, http.StatusCreated)
		json.NewEncoder(w).Encode(smShareCreateResponse)
	case "DELETE":
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(nil)
	default:
		smbShareResponse := models.CifsShareResponse{
			CifsShareResponseInlineRecords: []*models.CifsShare{
				{
					Name: convert.ToPtr("share"), Path: convert.ToPtr("/"),
				},
			},
			NumRecords: &numRecords,
		}
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(smbShareResponse)
	}
}

func mockSMBShareResponseNumRecordsNil(w http.ResponseWriter, r *http.Request) {
	smbShareResponse := models.CifsShareResponse{
		CifsShareResponseInlineRecords: []*models.CifsShare{
			{
				Name: convert.ToPtr("share"), Path: convert.ToPtr("/"),
			},
		},
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(smbShareResponse)
}

func mockSMBShareResourceNotFound(w http.ResponseWriter, r *http.Request) {
	setHTTPResponseHeader(w, http.StatusNotFound)
	json.NewEncoder(w).Encode("")
}

func mockSMBShareAccessControlResponse(w http.ResponseWriter, r *http.Request) {
	numRecords := int64(1)
	switch r.Method {
	case "POST":
		// Mock response for creating SMB Share Access Control
		accessControlCreateResponse := n_a_s.CifsShareACLCreateCreated{Location: "/"}
		setHTTPResponseHeader(w, http.StatusCreated)
		json.NewEncoder(w).Encode(accessControlCreateResponse)
	case "DELETE":
		// Mock response for deleting SMB Share Access Control
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(nil)
	default:
		// Mock response for listing SMB Share Access Control
		smbShareACLResponse := models.CifsShareACLResponse{
			CifsShareACLResponseInlineRecords: []*models.CifsShareACL{
				{
					UserOrGroup: convert.ToPtr("user1"),
					Permission:  convert.ToPtr("full_control"),
				},
			},
			NumRecords: &numRecords,
		}
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(smbShareACLResponse)
	}
}

func mockSMBShareAccessControlResourceNotFound(w http.ResponseWriter, r *http.Request) {
	setHTTPResponseHeader(w, http.StatusNotFound)
	json.NewEncoder(w).Encode("")
}

func TestOntapRest_SMBShareCreate(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"SMBShareCreate_success", mockSMBShareResponse, false},
		{"SMBShareCreate_failed", mockSMBShareResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.SMBShareCreate(ctx, "share", "/")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not create an SMB share")
			} else {
				assert.Error(t, err, "SMB share created")
			}
			server.Close()
		})
	}
}

func TestOntapRest_SMBShareExists(t *testing.T) {
	tests := []struct {
		name             string
		mockFunction     func(w http.ResponseWriter, r *http.Request)
		isSMBShareExists bool
		isErrorExpected  bool
	}{
		{"GetSMBShare_success", mockSMBShareResponse, true, false},
		{"NumRecordsFeildsNil", mockSMBShareResponseNumRecordsNil, false, false},
		{"GetSMBShare_failed", mockSMBShareResourceNotFound, false, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			smbShareExists, err := rs.SMBShareExists(ctx, "share")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not found an SMB share")
				assert.Equal(t, test.isSMBShareExists, smbShareExists)
			} else {
				assert.Error(t, err, "SMB share exists")
			}
			server.Close()
		})
	}
}

func TestOntapRest_SMBShareDestroy(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"DeleteSMBShare_success", mockSMBShareResponse, false},
		{"DeleteSMBShare_failed", mockSMBShareResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			err := rs.SMBShareDestroy(ctx, "share")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not delete an SMB share")
			} else {
				assert.Error(t, err, "SMB share deleted")
			}
			server.Close()
		})
	}
}

func TestOntapRest_SMBShareAccessControlCreate(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"AccessControlCreate_success", mockSMBShareAccessControlResponse, false},
		{"AccessControlCreate_failed", mockSMBShareAccessControlResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			smbShareAccessControl := make(map[string]string)
			smbShareAccessControl["user"] = "full_control"
			err := rs.SMBShareAccessControlCreate(ctx, "share", smbShareAccessControl)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not create SMB share access control")
			} else {
				assert.Error(t, err, "SMB share access control created")
			}
			server.Close()
		})
	}
}

func TestOntapRest_SMBShareAccessControlDelete(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"AccessControlDelete_success", mockSMBShareAccessControlResponse, false},
		{"AccessControlDelete_failed", mockSMBShareAccessControlResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			smbShareAccessControl := make(map[string]string)
			smbShareAccessControl["Everyone"] = "windows"
			err := rs.SMBShareAccessControlDelete(ctx, "share", smbShareAccessControl)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not delete SMB share access control")
			} else {
				assert.Error(t, err, "SMB share access control deleted")
			}
			server.Close()
		})
	}
}

func mockExportPolicyListResponse(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	exportPolicyName := "fake-exportPolicy"
	exportPolicyID := int64(1)
	exportPolicy := models.ExportPolicy{
		Name: &exportPolicyName, ID: &exportPolicyID,
	}

	url := "/api/protocols/nfs/export-policies"
	var hrefLink *models.ExportPolicyResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.ExportPolicyResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		hasNextLink = false
	}

	exportPolicyResponse := models.ExportPolicyResponse{
		ExportPolicyResponseInlineRecords: []*models.ExportPolicy{&exportPolicy},
		NumRecords:                        convert.ToPtr(int64(1)),
		Links:                             hrefLink,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(exportPolicyResponse)
}

func mockExportPolicyListResponseInternalError(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	exportPolicyName := "fake-exportPolicy"
	exportPolicyID := int64(1)
	exportPolicy := models.ExportPolicy{
		Name: &exportPolicyName, ID: &exportPolicyID,
	}

	url := "/api/protocols/nfs/export-policies"
	var hrefLink *models.ExportPolicyResponseInlineLinks
	sc := http.StatusInternalServerError
	if hasNextLink {
		hrefLink = &models.ExportPolicyResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
		sc = http.StatusOK
	}

	exportPolicyResponse := models.ExportPolicyResponse{
		ExportPolicyResponseInlineRecords: []*models.ExportPolicy{&exportPolicy},
		NumRecords:                        convert.ToPtr(int64(1)),
		Links:                             hrefLink,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(sc)
	json.NewEncoder(w).Encode(exportPolicyResponse)
}

func mockExportPolicyListResponseNumRecordsNil(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	exportPolicyName := "fake-exportPolicy"
	exportPolicyID := int64(1)
	exportPolicy := models.ExportPolicy{
		Name: &exportPolicyName, ID: &exportPolicyID,
	}

	url := "/api/protocols/nfs/export-policies"
	var hrefLink *models.ExportPolicyResponseInlineLinks
	if hasNextLink {
		hrefLink = &models.ExportPolicyResponseInlineLinks{
			Next: &models.Href{Href: &url},
		}
	}

	exportPolicyResponse := models.ExportPolicyResponse{
		ExportPolicyResponseInlineRecords: []*models.ExportPolicy{&exportPolicy},
		Links:                             hrefLink,
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(exportPolicyResponse)
}

func mockExportPolicyResponse(w http.ResponseWriter, r *http.Request) {
	exportPolicyName := "fake-exportPolicy"
	exportPolicyID := int64(1)
	exportPolicy := models.ExportPolicy{
		Name: &exportPolicyName, ID: &exportPolicyID,
	}
	exportPolicyResponse := models.ExportPolicyResponse{
		ExportPolicyResponseInlineRecords: []*models.ExportPolicy{&exportPolicy},
		NumRecords:                        convert.ToPtr(int64(1)),
	}

	switch r.Method {
	case "POST":
		switch r.URL.Path {
		case "/api/protocols/nfs/export-policies/1/rules":
			mockExportPolicyRule(w, r)
		default:
			setHTTPResponseHeader(w, http.StatusCreated)
			json.NewEncoder(w).Encode(exportPolicyResponse)
		}
	case "GET":
		switch r.URL.Path {
		case "/api/protocols/nfs/export-policies":
			setHTTPResponseHeader(w, http.StatusOK)
			json.NewEncoder(w).Encode(exportPolicyResponse)
		case "/api/protocols/nfs/export-policies/1/rules":
			mockExportPolicyRule(w, r)
		default:
			setHTTPResponseHeader(w, http.StatusOK)
			json.NewEncoder(w).Encode(exportPolicy)
		}
	case "DELETE":
		switch r.URL.Path {
		case "/api/protocols/nfs/export-policies/1/rules":
			mockExportPolicyRule(w, r)
		default:
			setHTTPResponseHeader(w, http.StatusOK)
			json.NewEncoder(w).Encode(exportPolicy)
		}
	}
}

func mockExportPolicyResponseNumRecordsNil(w http.ResponseWriter, r *http.Request) {
	exportPolicyName := "fake-exportPolicy"
	exportPolicyID := int64(1)
	exportPolicy := models.ExportPolicy{
		Name: &exportPolicyName, ID: &exportPolicyID,
	}
	exportPolicyResponse := models.ExportPolicyResponse{
		ExportPolicyResponseInlineRecords: []*models.ExportPolicy{&exportPolicy},
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(exportPolicyResponse)
}

func mockExportPolicyResponseUUIDNil(w http.ResponseWriter, r *http.Request) {
	exportPolicyName := "fake-exportPolicy"
	exportPolicy := models.ExportPolicy{
		Name: &exportPolicyName,
	}
	exportPolicyResponse := models.ExportPolicyResponse{
		ExportPolicyResponseInlineRecords: []*models.ExportPolicy{&exportPolicy},
		NumRecords:                        convert.ToPtr(int64(1)),
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(exportPolicyResponse)
}

func mockExportPolicyRule(w http.ResponseWriter, r *http.Request) {
	exportClient := ".example.com"
	ruleIndex := int64(1)
	numRecords := int64(1)
	exportRule := models.ExportRuleResponse{
		ExportRuleResponseInlineRecords: []*models.ExportRule{
			{
				ExportRuleInlineClients: []*models.ExportClients{{Match: &exportClient}},
				Index:                   &ruleIndex,
			},
		},
		NumRecords: &numRecords,
	}

	switch r.Method {
	case "POST":
		setHTTPResponseHeader(w, http.StatusCreated)
		json.NewEncoder(w).Encode(exportRule)
	case "GET":
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(exportRule)
	case "DELETE":
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode("")
	}
}

func mockExportPolicyResourceNotFound(w http.ResponseWriter, r *http.Request) {
	setHTTPResponseHeader(w, http.StatusNotFound)
	json.NewEncoder(w).Encode("")
}

func TestOntapREST_CreateExportPolicy(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"ExportPolicy_success", mockExportPolicyResponse, false},
		{"ExportPolicy_failed", mockExportPolicyResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			exportPolicy, err := rs.ExportPolicyCreate(ctx, "fake-exportPolicy")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not create the export policy")
				assert.Equal(t, "fake-exportPolicy", *exportPolicy.Payload.ExportPolicyResponseInlineRecords[0].Name)
			} else {
				assert.Error(t, err, "export policy created")
			}
			server.Close()
		})
	}
}

func TestOntapREST_GetExportPolicy(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"ExportPolicy_success", mockExportPolicyResponse, false},
		{"ExportPolicy_failed", mockExportPolicyResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			exportPolicy, err := rs.ExportPolicyGet(ctx, 0)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the export policy")
				assert.Equal(t, "fake-exportPolicy", *exportPolicy.Payload.Name)
			} else {
				assert.Error(t, err, "get the export policy")
			}
			server.Close()
		})
	}
}

func TestOntapREST_ExportPolicyList(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(hasNextLink bool, w http.ResponseWriter, r *http.Request)
		isNegativeTest  bool
		isErrorExpected bool
	}{
		{"ExportPolicy_success", mockExportPolicyListResponse, false, false},
		{"NumRecordsFieldNilInResponse", mockExportPolicyListResponseNumRecordsNil, false, false},
		{"HrefLinkSecondGetCallFailed", mockExportPolicyListResponseInternalError, false, true},
		{"ExportPolicy_failed", mockInternalServerError, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := getHttpServer(test.isNegativeTest, test.mockFunction)
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			exportPolicy, err := rs.ExportPolicyList(ctx, "fake-exportPolicy")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the export policy")
				assert.Equal(t, "fake-exportPolicy", *exportPolicy.Payload.ExportPolicyResponseInlineRecords[0].Name)
			} else {
				assert.Error(t, err, "get the export policy")
			}
			server.Close()
		})
	}
}

func TestOntapREST_ExportPolicyGetByName(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"ExportPolicy_success", mockExportPolicyResponse, false},
		{"ExportPolicy_failed", mockExportPolicyResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			exportPolicy, err := rs.ExportPolicyGetByName(ctx, "fake-exportPolicy")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the export policy")
				assert.Equal(t, "fake-exportPolicy", *exportPolicy.Name)
			} else {
				assert.Error(t, err, "get the export policy")
			}
			server.Close()
		})
	}
}

func TestOntapREST_DestroyExportPolicy(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"ExportPolicy_success", mockExportPolicyResponse, false},
		{"ExportPolicy_Failure", mockExportPolicyResourceNotFound, true},
		{"ExportPolicy_NumRecordsNil", mockExportPolicyResponseNumRecordsNil, true},
		{"ExportPolicy_UUIDNilInResponse", mockExportPolicyResponseUUIDNil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			exportPolicy, err := rs.ExportPolicyDestroy(ctx, "fake-exportPolicy")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not delete the export policy")
				assert.Equal(t, true, exportPolicy.IsSuccess())
			} else {
				assert.Error(t, err, "delete the export policy")
			}
			server.Close()
		})
	}
}

func TestOntapREST_GetExportRules(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"ExportPolicy_success", mockExportPolicyResponse, false},
		{"ExportPolicy_failed", mockExportPolicyResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			exportPolicyRule, err := rs.ExportRuleList(ctx, "fake-exportPolicy")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get the export rule")
				assert.Equal(t, ".example.com",
					*exportPolicyRule.Payload.ExportRuleResponseInlineRecords[0].ExportRuleInlineClients[0].Match)
			} else {
				assert.Error(t, err, "get the export rule")
			}
			server.Close()
		})
	}
}

func TestOntapREST_CreateExportRule(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"ExportPolicy_success", mockExportPolicyResponse, false},
		{"ExportPolicy_failure", mockExportPolicyResourceNotFound, true},
		{"ExportPolicy_NumRecordsNil", mockExportPolicyResponseNumRecordsNil, true},
		{"ExportPolicy_UUIDNilInResponse", mockExportPolicyResponseUUIDNil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			exportPolicyRule, err := rs.ExportRuleCreate(ctx, "fake-exportPolicy", ".example.com", []string{"any"},
				[]string{"krb5"}, []string{"krb5p"}, []string{"krb5i"})
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not create export rule")
				assert.Equal(t, ".example.com",
					*exportPolicyRule.Payload.ExportRuleResponseInlineRecords[0].ExportRuleInlineClients[0].Match)
			} else {
				assert.Error(t, err, "export rule created")
			}
			server.Close()
		})
	}
}

func TestOntapREST_ToExportAuthenticationFlavorSlice(t *testing.T) {
	tests := []struct {
		authFlavor                 string
		exportAuthenticationFlavor models.ExportAuthenticationFlavor
	}{
		{"any", models.ExportAuthenticationFlavorAny},
		{"none", models.ExportAuthenticationFlavorNone},
		{"never", models.ExportAuthenticationFlavorNever},
		{"krb5", models.ExportAuthenticationFlavorKrb5},
		{"krb5i", models.ExportAuthenticationFlavorKrb5i},
		{"krb5p", models.ExportAuthenticationFlavorKrb5p},
		{"ntlm", models.ExportAuthenticationFlavorNtlm},
		{"sys", models.ExportAuthenticationFlavorSys},
	}
	for _, test := range tests {
		t.Run(test.authFlavor, func(t *testing.T) {
			exportAuthenticationFlavor := []string{test.authFlavor}
			result := ToExportAuthenticationFlavorSlice(exportAuthenticationFlavor)
			assert.Equal(t, test.exportAuthenticationFlavor, *result[0])
		})
	}
}

func TestOntapREST_DeleteExportRule(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"ExportPolicy_success", mockExportPolicyResponse, false},
		{"ExportPolicy_Failure", mockExportPolicyResourceNotFound, true},
		{"ExportPolicy_NumRecordsNil", mockExportPolicyResponseNumRecordsNil, true},
		{"ExportPolicy_UUIDNilInResponse", mockExportPolicyResponseUUIDNil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			exportPolicyRule, err := rs.ExportRuleDestroy(ctx, "fake-exportPolicy", 1)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not destroyed export rule")
				assert.Equal(t, true, exportPolicyRule.IsSuccess())
			} else {
				assert.Error(t, err, "export rule destroyed")
			}
			server.Close()
		})
	}
}

func TestExtractError(t *testing.T) {
	tests := []struct {
		Name  string
		Value error
		Valid bool
	}{
		// Valid errors
		{
			"Valid",
			errors.New("API status: failed, Reason: Size \\\"1GB\\\" (\\\"1073741824B\\\") is too small.Minimum size is \\\"400GB\\\" (\\\"429496729600B\\\").,Code: 13115"),
			true,
		},
		{
			"Valid",
			errors.New("API State: failure, Message: Size \\\"1GB\\\" (\\\"1073741824B\\\") is too small.Minimum size is \\\"400GB\\\" (\\\"429496729600B\\\").,Code: 917534"),
			true,
		},

		// Invalid errors
		{"Invalid", errors.New("test-error, 113455"), false},
		{"Invalid", errors.New("test-error"), false},
		{"Invalid", errors.New(""), false},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			field1, field2, field3 := ExtractError(test.Value)
			if test.Valid {
				assert.NotEmpty(t, field1, "should be valid")
				assert.NotEmpty(t, field2, "should be valid")
				assert.NotEmpty(t, field3, "should be valid")
			} else {
				assert.Empty(t, field1, "should be invalid")
				assert.Empty(t, field2, "should be invalid")
				assert.Empty(t, field3, "should be invalid")
			}
		})
	}
}

func TestIsRESTSupported(t *testing.T) {
	// Define test cases
	tests := []struct {
		name              string
		version           string
		isSupported       bool
		isSupportedErrMsg string
		wantErr           bool
		wantErrMsg        string
	}{
		{
			name:              "Supported version",
			version:           "9.12.1",
			isSupported:       true,
			isSupportedErrMsg: "positive test, version 9.12.1 is supported, expected true but got false",
			wantErr:           false,
			wantErrMsg:        "9.12.1 is a correct semantics, error was not expected",
		},
		{
			name:              "Unsupported version",
			version:           "9.10.1",
			isSupported:       false,
			isSupportedErrMsg: "negative test, version 9.10.1 is not supported, expected false but got true",
			wantErr:           false,
			wantErrMsg:        "9.10.1 is a correct semantics, error was not expected",
		},
		{
			name:              "Invalid version",
			version:           "invalid",
			isSupported:       false,
			isSupportedErrMsg: "invalid version provided, expected false but got true",
			wantErr:           true,
			wantErrMsg:        "incorrect semantics, error was expected",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			isSupported, err := IsRESTSupported(tt.version)
			assert.Equal(t, tt.isSupported, isSupported, tt.isSupportedErrMsg)
			if tt.wantErr {
				assert.Errorf(t, err, tt.wantErrMsg)
			} else {
				assert.NoError(t, err, tt.wantErrMsg)
			}
		})
	}
}

func TestIsRESTSupportedDefault(t *testing.T) {
	// Define test cases
	tests := []struct {
		name              string
		version           string
		isSupported       bool
		isSupportedErrMsg string
		wantErr           bool
		wantErrMsg        string
	}{
		{
			name:              "Supported version",
			version:           "9.15.1",
			isSupported:       true,
			isSupportedErrMsg: "positive test, version 9.15.1 is supported, expected true but got false",
			wantErr:           false,
			wantErrMsg:        "9.15.1 is a correct semantics, error was not expected",
		},
		{
			name:              "Unsupported version",
			version:           "9.13.1",
			isSupported:       false,
			isSupportedErrMsg: "negative test, version 9.13.1 is not supported, expected false but got true",
			wantErr:           false,
			wantErrMsg:        "9.13.1 is a correct semantics, error was not expected",
		},
		{
			name:              "Invalid version",
			version:           "invalid",
			isSupported:       false,
			isSupportedErrMsg: "invalid version provided, expected false but got true",
			wantErr:           true,
			wantErrMsg:        "incorrect semantics, error was expected",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			isSupported, err := IsRESTSupportedDefault(tt.version)
			assert.Equal(t, tt.isSupported, isSupported, tt.isSupportedErrMsg)
			if tt.wantErr {
				assert.Errorf(t, err, tt.wantErrMsg)
			} else {
				assert.NoError(t, err, tt.wantErrMsg)
			}
		})
	}
}

func TestNVMeNamespaceSetComment(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockNvmeNamespaceResponse, false},
		{"BackendReturnError", mockNvmeResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			ns := NVMeNamespace{
				Name:      "namespace1",
				UUID:      "1cd8a442-86d1-11e0-ae1c-123478563412",
				OsType:    "linux",
				Size:      "99999",
				BlockSize: 4096,
				State:     "online",
			}
			err := rs.NVMeNamespaceSetComment(ctx, ns.UUID, "test comment")
			if !test.isErrorExpected {
				assert.NoError(t, err, "expected no error while setting namespace comment")
			} else {
				assert.Error(t, err, "expected error while setting namespace comment")
			}
			server.Close()
		})
	}
}

func TestNVMeNamespaceSetQosPolicyGroup(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"PositiveTest", mockNvmeNamespaceResponse, false},
		{"BackendReturnError", mockNvmeResourceNotFound, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			qos := QosPolicyGroup{
				Name: "mock-qospolicy",
				Kind: QosPolicyGroupKind,
			}

			ns := NVMeNamespace{
				Name:      "namespace1",
				UUID:      "1cd8a442-86d1-11e0-ae1c-123478563412",
				OsType:    "linux",
				Size:      "99999",
				BlockSize: 4096,
				State:     "online",
				QosPolicy: qos,
			}

			err := rs.NVMeNamespaceSetQosPolicyGroup(ctx, ns.UUID, qos)
			if !test.isErrorExpected {
				assert.NoError(t, err, "expected no error while setting namespace Qos")
			} else {
				assert.Error(t, err, "expected error while setting namespace QoS")
			}
			server.Close()
		})
	}
}

func TestSetOntapVersion(t *testing.T) {
	ontapVersion := "9.8"
	client := RestClient{
		m: &sync.RWMutex{},
	}
	// Set a new ONTAP version
	client.SetOntapVersion(ontapVersion)
	assert.Equal(t, ontapVersion, client.OntapVersion, "SetOntapVersion should update the ONTAP version correctly")
}

func TestGetOntapVersion(t *testing.T) {
	ontapVersion := "9.8"
	client := RestClient{
		m:            &sync.RWMutex{},
		OntapVersion: ontapVersion,
	}
	assert.Equal(t, ontapVersion, client.GetOntapVersion(), "SetOntapVersion should update the ONTAP version correctly")
}
