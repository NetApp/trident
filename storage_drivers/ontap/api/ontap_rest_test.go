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

func mockSVM(w http.ResponseWriter, r *http.Request) {
	svm := models.Svm{
		UUID:  utils.Ptr("1234"),
		Name:  utils.Ptr("svm0"),
		State: utils.Ptr("running"),
		SvmInlineAggregates: []*models.SvmInlineAggregatesInlineArrayItem{
			{Name: utils.Ptr("aggr1")},
		},
	}

	setHTTPResponseHeader(w, http.StatusOK)
	json.NewEncoder(w).Encode(svm)
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
	} else if r.URL.Path == "/api/svm/svms" {
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
	} else if r.URL.Path == "/api/protocols/san/iscsi/services/1234" {
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
