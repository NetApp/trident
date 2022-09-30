// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/s_a_n"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/svm"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/netapp/trident/utils"
)

var ctx = context.Background()

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
	expectedMinimumONTAPVersion := utils.MustParseSemantic("9.11.1")
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
			Code:    "42",
			Message: "error 42",
		},
	}}
	eeResponse, err = ExtractErrorResponse(ctx, lunModifyDefaultResponse)
	assert.Nil(t, err)
	assert.NotNil(t, eeResponse)
	assert.Equal(t, eeResponse.Error.Code, "42", "Unexpected code")
	assert.Equal(t, eeResponse.Error.Message, "error 42", "Unexpected message")
}

func TestVolumeEncryption(t *testing.T) {
	// negative case:  if nil, should not be set
	veMarshall := models.VolumeEncryption{}
	bytes, _ := json.MarshalIndent(veMarshall, "", "  ")
	assert.Equal(t, `{}`, string(bytes))
	volumeEncrytion := models.VolumeEncryption{}
	json.Unmarshal(bytes, &volumeEncrytion)
	assert.Nil(t, volumeEncrytion.Enabled)

	// positive case:  if set to false, should be sent as false (not omitted)
	veMarshall = models.VolumeEncryption{Enabled: ToBoolPointer(false)}
	bytes, _ = json.MarshalIndent(veMarshall, "", "  ")
	assert.Equal(t,
		`{
  "enabled": false
}`,
		string(bytes))
	volumeEncrytion = models.VolumeEncryption{}
	json.Unmarshal(bytes, &volumeEncrytion)
	assert.False(t, *volumeEncrytion.Enabled)

	// positive case:  if set to true, should be sent as true
	veMarshall = models.VolumeEncryption{Enabled: ToBoolPointer(true)}
	bytes, _ = json.MarshalIndent(veMarshall, "", "  ")
	assert.Equal(t,
		`{
  "enabled": true
}`,
		string(bytes))
	volumeEncrytion = models.VolumeEncryption{}
	json.Unmarshal(bytes, &volumeEncrytion)
	assert.True(t, *volumeEncrytion.Enabled)
}
