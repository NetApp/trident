// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-openapi/strfmt"
	"github.com/stretchr/testify/assert"

	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/s_a_n"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/svm"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/netapp/trident/utils"
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
	lunModifyDefaultResponse = s_a_n.LunModifyDefault{Payload: &models.ErrorResponse{
		Error: &models.Error{
			Code:    utils.Ptr("42"),
			Message: utils.Ptr("error 42"),
		},
	}}
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
	veMarshall = models.VolumeInlineEncryption{Enabled: utils.Ptr(false)}
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
	veMarshall = models.VolumeInlineEncryption{Enabled: utils.Ptr(true)}
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
	messageCode := utils.Ptr("42")
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

	_, err := rs.IscsiInitiatorGetDefaultAuth(ctx)
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
	iscsiServiceResponse := models.IscsiServiceResponse{IscsiServiceResponseInlineRecords: []*models.
		IscsiService{&iscsiService}}

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

			iscsi, err := rs.IscsiInterfaceGet(ctx)
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
		Name:  utils.Ptr("svm0"),
		State: utils.Ptr("running"),
		SvmInlineAggregates: []*models.SvmInlineAggregatesInlineArrayItem{
			{
				Name: utils.Ptr("aggr1"),
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
			NumRecords:               utils.Ptr(int64(1)),
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

			iscsi, err := rs.IscsiNodeGetName(ctx)

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

			igroup, err := rs.IgroupList(ctx, test.pattern)

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

func getHttpServer(isNegativeTest bool, mockFunction func(hasNextLink bool, w http.ResponseWriter,
	r *http.Request),
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

			igroup, err := rs.IgroupList(ctx, "igroup")
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

			igroup, err := rs.IgroupGetByName(ctx, "igroup")
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
	lunInfo := getLunInfo(utils.Ptr("lunAttr"))
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
	lunInfo := getLunInfo(utils.Ptr("lunAttr"))
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
	lunInfo := getLunInfo(utils.Ptr("lunAttr"))

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

			lunResponse, err := rs.LunList(ctx, "*")
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
		NumRecords: utils.Ptr(int64(1)),
		LunMapResponseInlineRecords: []*models.LunMap{
			{
				LogicalUnitNumber: nil,
				Igroup: &models.LunMapInlineIgroup{
					Name: utils.Ptr(initiatorGroup),
					UUID: utils.Ptr("fake-igroupUUID"),
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
	lunInfo := getLunInfo(utils.Ptr("lunAttr"))
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
	lunInfo := getLunInfo(utils.Ptr("lunAttr"))
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

func TestOntapREST_LunGetByName(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isResponseNil   bool
		isErrorExpected bool
	}{
		{"PositiveTest", mockLunResponse, false, false},
		{"NumRecordsNilInResponse", mockLunResponseNumRecordsNil, true, false},
		{"BackendReturnError", mockResourceNotFound, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			lunResponse, err := rs.LunGetByName(ctx, "fake-lunName")
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

func TestOntapREST_PollLunCreate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(mockLunResponse))
	rs := newRestClient(server.Listener.Addr().String(), server.Client())
	assert.NotNil(t, rs)

	err := rs.pollLunCreate(ctx, "/fake-lunName")
	assert.NoError(t, err, "could not poll LUN create job")
	server.Close()
}

func mockInvalidResponse(w http.ResponseWriter, r *http.Request) {
	setHTTPResponseHeader(w, http.StatusNotFound)
	json.NewEncoder(w).Encode("invalidResponse")
}

func mockUnauthorizedError(w http.ResponseWriter, r *http.Request) {
	setHTTPResponseHeader(w, http.StatusUnauthorized)
	json.NewEncoder(w).Encode(fmt.Errorf("error"))
}

func mockIOUtilError(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Length", "50")
	w.Write([]byte("500 - Something bad happened!"))
}

func TestOntapREST_LunOptions(t *testing.T) {
	tests := []struct {
		name            string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{"getUnauthorizedError", mockUnauthorizedError, true},
		{"getIOUtilError", mockIOUtilError, true},
		{"getInvalidResponse", mockInvalidResponse, true},
		{"getHttpServerNotRespondError", mockInvalidResponse, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(test.mockFunction))
			rs := newRestClient(server.Listener.Addr().String(), server.Client())
			assert.NotNil(t, rs)

			if test.name == "getHttpServerNotRespondError" {
				server.Close()
			}
			_, err := rs.LunOptions(ctx)

			if !test.isErrorExpected {
				assert.NoError(t, err, "could not create clone of the LUN")
			} else {
				assert.Error(t, err, "could not create clone of the LUN")
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
				QosPolicyGroup{Name: "qosPolicy", Kind: QosPolicyGroupKind}, false, false)
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
	lunInfo := getLunInfo(utils.Ptr("lunAttr"))
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
	lunInfo := getLunInfo(utils.Ptr("lunAttr"))
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
	lunInfo := getLunInfo(utils.Ptr("lunAttr"))
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
	lunInfo := getLunInfo(utils.Ptr("lunAttr"))
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
					Name: utils.Ptr(initiatorGroup),
					UUID: utils.Ptr("fake-igroupUUID"),
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
					Name: utils.Ptr(initiatorGroup),
					UUID: utils.Ptr("fake-igroupUUID"),
				},
			},
		},
		NumRecords: utils.Ptr(int64(0)),
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
					Name: utils.Ptr(initiatorGroup),
					UUID: nil,
				},
			},
		},
		NumRecords: utils.Ptr(int64(1)),
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

			lumMapResponse, err := rs.LunMapList(ctx, "fake-initiatorGroupName", "/dev/sda")
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
				{Name: utils.Ptr("fake-lunMap")},
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
				{Name: utils.Ptr("fake-lunMap")},
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
	lunInfo := getLunInfo(utils.Ptr("lunAttr"))
	lunInfo.Space.Size = nil
	lunResponse := models.LunResponse{LunResponseInlineRecords: []*models.Lun{lunInfo}, NumRecords: utils.Ptr(int64(1))}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(lunResponse)
}

func mockLunResponseSpaceNil(w http.ResponseWriter, r *http.Request) {
	lunInfo := getLunInfo(utils.Ptr("lunAttr"))
	lunInfo.Space = nil
	lunResponse := models.LunResponse{LunResponseInlineRecords: []*models.Lun{lunInfo}, NumRecords: utils.Ptr(int64(1))}

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
		Name: utils.Ptr("node1"),
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
				State:    utils.Ptr("up"),
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
					Svm: &models.SvmPeerInlinePeerInlineSvm{Name: utils.Ptr("svm1")},
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
					Path: utils.Ptr("svm0:"),
					Svm: &models.SnapmirrorEndpointInlineSvm{
						Name: utils.Ptr("svm0"),
					},
				},
				Source: &models.SnapmirrorEndpoint{
					Path: utils.Ptr("svm1:"),
					Svm: &models.SnapmirrorEndpointInlineSvm{
						Name: utils.Ptr("svm1"),
					},
				},
				UUID: utils.Ptr(strfmt.UUID("1")),
			},
			{Source: &models.
				SnapmirrorEndpoint{Svm: &models.SnapmirrorEndpointInlineSvm{}}},
			{Source: &models.
				SnapmirrorEndpoint{Path: utils.Ptr("svm0:")}},
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
		State: utils.Ptr("running"),
		SvmInlineAggregates: []*models.SvmInlineAggregatesInlineArrayItem{
			{Name: utils.Ptr("aggr1")},
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
				UUID: utils.Ptr("fakeUUID"),
				Name: utils.Ptr("fakeName"),
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
	jobLink := models.Job{UUID: &jobId, State: utils.Ptr(models.JobStateFailure)}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(jobLink)
}

func mockJobResponseJobStatePaused(w http.ResponseWriter, r *http.Request) {
	jobId := strfmt.UUID("1234")
	jobLink := models.Job{UUID: &jobId, State: utils.Ptr(models.JobStatePaused)}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(jobLink)
}

func mockJobResponseJobStateRunning(w http.ResponseWriter, r *http.Request) {
	jobId := strfmt.UUID("1234")
	jobLink := models.Job{UUID: &jobId, State: utils.Ptr(models.JobStateRunning)}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(jobLink)
}

func mockJobResponseJobStateQueued(w http.ResponseWriter, r *http.Request) {
	jobId := strfmt.UUID("1234")
	jobLink := models.Job{UUID: &jobId, State: utils.Ptr(models.JobStateQueued)}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(jobLink)
}

func mockJobResponseInvalidState(w http.ResponseWriter, r *http.Request) {
	jobId := strfmt.UUID("1234")
	jobLink := models.Job{UUID: &jobId, State: utils.Ptr("InvalidState")}
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
		Name: utils.Ptr("aggr1"),
		BlockStorage: &models.AggregateInlineBlockStorage{
			Primary: &models.AggregateInlineBlockStorageInlinePrimary{DiskType: utils.Ptr("fc")},
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
		NumRecords:                     utils.Ptr(int64(1)),
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

			aggrList, err := rs.AggregateList(ctx, "aggr1")
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

	svm := models.Svm{
		UUID:  &svmUUID,
		Name:  &svmName,
		State: utils.Ptr("running"),
		SvmInlineAggregates: []*models.SvmInlineAggregatesInlineArrayItem{
			{Name: utils.Ptr("aggr1")},
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
		NumRecords:               utils.Ptr(int64(1)),
		Links:                    hrefLink,
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(svmResponse)
}

func mockSVMListInternalError(hasNextLink bool, w http.ResponseWriter, r *http.Request) {
	svmUUID := "1234"
	svmName := "svm0"

	svm := models.Svm{
		UUID:  &svmUUID,
		Name:  &svmName,
		State: utils.Ptr("running"),
		SvmInlineAggregates: []*models.SvmInlineAggregatesInlineArrayItem{
			{Name: utils.Ptr("aggr1")},
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
		SvmResponseInlineRecords: []*models.Svm{&svm},
		NumRecords:               utils.Ptr(int64(1)),
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

			svm, err := rs.SvmList(ctx, "svm0")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get svm")
				assert.Equal(t, "svm0", *svm.Payload.SvmResponseInlineRecords[0].Name)
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

			svm, err := rs.SvmGetByName(ctx, "svm0")
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not get svm by name")
				assert.Equal(t, "svm0", *svm.Name)
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

	svm := models.Svm{
		UUID: &svmUUID,
		Name: &svmName,
		SvmInlineAggregates: []*models.SvmInlineAggregatesInlineArrayItem{
			{Name: utils.Ptr("aggr1")},
		},
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(svm)
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
			{SerialNumber: utils.Ptr("4048820-60-9")},
		},
		NumRecords: utils.Ptr(int64(1)),
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
			{SerialNumber: utils.Ptr("4048820-60-9")},
		},
		NumRecords: utils.Ptr(int64(1)),
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
			{SerialNumber: utils.Ptr("4048820-60-9")},
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
			{SerialNumber: utils.Ptr("4048820-60-9")},
		},
		NumRecords: utils.Ptr(int64(1)),
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(nodeResponse)
}

func mockNodeResponseSerialNumberNil(w http.ResponseWriter, r *http.Request) {
	nodeResponse := models.NodeResponse{
		NodeResponseInlineRecords: []*models.NodeResponseInlineRecordsInlineArrayItem{
			{},
		},
		NumRecords: utils.Ptr(int64(1)),
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(nodeResponse)
}

func mockNodeResponseNumRecordsNil(w http.ResponseWriter, r *http.Request) {
	nodeResponse := models.NodeResponse{
		NodeResponseInlineRecords: []*models.NodeResponseInlineRecordsInlineArrayItem{
			{SerialNumber: utils.Ptr("4048820-60-9")},
		},
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(nodeResponse)
}

func mockNodeResponseNumRecordsZero(w http.ResponseWriter, r *http.Request) {
	nodeResponse := models.NodeResponse{
		NodeResponseInlineRecords: []*models.NodeResponseInlineRecordsInlineArrayItem{
			{SerialNumber: utils.Ptr("4048820-60-9")},
		},
		NumRecords: utils.Ptr(int64(0)),
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
				Name:  utils.Ptr("namespace1"),
				UUID:  utils.Ptr("1cd8a442-86d1-11e0-ae1c-123478563412"),
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
				Name:  utils.Ptr("namespace1"),
				UUID:  utils.Ptr("1cd8a442-86d1-11e0-ae1c-123478563412"),
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
				Name:  utils.Ptr("namespace1"),
				UUID:  utils.Ptr("1cd8a442-86d1-11e0-ae1c-123478563412"),
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
				Name: utils.Ptr("subsystemName"),
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
				Name: utils.Ptr("subsystemName"),
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
				Name:  utils.Ptr("namespace1"),
				UUID:  utils.Ptr("1cd8a442-86d1-11e0-ae1c-123478563412"),
				Space: &models.NvmeNamespaceInlineSpace{Size: &size},
			},
		},
		NumRecords: &numRecords,
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

func mockNvmeNamespaceResponseNumRecordsNil(w http.ResponseWriter, r *http.Request) {
	size := int64(1073741824)

	nvmeNamespaceResponse := models.NvmeNamespaceResponse{
		NvmeNamespaceResponseInlineRecords: []*models.NvmeNamespace{
			{
				Name:  utils.Ptr("namespace1"),
				UUID:  utils.Ptr("1cd8a442-86d1-11e0-ae1c-123478563412"),
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
		NumRecords: utils.Ptr(int64(1)),
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
					UUID: utils.Ptr("1cd8a442-86d1-11e0-ae1c-123478563412"),
				},
			},
		},
		NumRecords: &numRecords,
	}

	nvmeSubsystemResponse := models.NvmeSubsystemResponse{
		NvmeSubsystemResponseInlineRecords: []*models.NvmeSubsystem{
			{
				Name: utils.Ptr("subsystemName"),
			},
		},
		NumRecords: &numRecords,
	}

	nvmeSubsystemHostResponse := models.NvmeSubsystemHostResponse{
		NvmeSubsystemHostResponseInlineRecords: []*models.NvmeSubsystemHost{
			{
				Subsystem: &models.NvmeSubsystemHostInlineSubsystem{
					Name: utils.Ptr("nvmeSubsystemName"),
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
			uuid, err := rs.NVMeNamespaceCreate(ctx, ns)
			if !test.isErrorExpected {
				assert.NoError(t, err, "could not create the NVMe namespace")
				assert.Equal(t, "1cd8a442-86d1-11e0-ae1c-123478563412", uuid)
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

			nvmeResponse, err := rs.NVMeNamespaceList(ctx, "namespace1")
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

			nvmeResponse, err := rs.NVMeNamespaceGetByName(ctx, "namespace1")
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

			_, err := rs.NVMeSubsystemList(ctx, test.pattern)
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

			nvmeSubsystemList, err := rs.NVMeSubsystemGetByName(ctx, "subsystemUUID")
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

			nvmeSubsystem, err := rs.NVMeSubsystemCreate(ctx, "subsystemName")
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

	svm := models.Svm{
		UUID:  &svmUUID,
		Name:  &svmName,
		State: utils.Ptr("running"),
		SvmInlineAggregates: []*models.SvmInlineAggregatesInlineArrayItem{
			{Name: utils.Ptr("aggr1")},
		},
	}
	if r.URL.Path == "/api/svm/svms/1234" {
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(svm)
	}

	if r.URL.Path == "/api/svm/svms" {
		svmResponse := models.SvmResponse{
			SvmResponseInlineRecords: []*models.Svm{&svm},
			NumRecords:               utils.Ptr(int64(1)),
		}
		setHTTPResponseHeader(w, http.StatusOK)
		json.NewEncoder(w).Encode(svmResponse)
	}
}

func mockSVMNameNil(w http.ResponseWriter, r *http.Request) {
	svm := models.Svm{
		UUID:  utils.Ptr("fake-uuid"),
		State: utils.Ptr("running"),
		SvmInlineAggregates: []*models.SvmInlineAggregatesInlineArrayItem{
			{UUID: utils.Ptr("fake-uuid")},
		},
	}

	svmResponse := models.SvmResponse{
		SvmResponseInlineRecords: []*models.Svm{&svm},
		NumRecords:               utils.Ptr(int64(1)),
	}
	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(svmResponse)
}

func mockSVMNumRecordsNil(w http.ResponseWriter, r *http.Request) {
	svm := models.Svm{
		Name: utils.Ptr("svm0"),
		SvmInlineAggregates: []*models.SvmInlineAggregatesInlineArrayItem{
			{Name: utils.Ptr("aggr1")},
		},
	}
	svmResponse := models.SvmResponse{
		SvmResponseInlineRecords: []*models.Svm{&svm},
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
