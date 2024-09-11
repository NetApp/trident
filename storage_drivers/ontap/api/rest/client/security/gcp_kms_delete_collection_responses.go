// Code generated by go-swagger; DO NOT EDIT.

package security

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// GcpKmsDeleteCollectionReader is a Reader for the GcpKmsDeleteCollection structure.
type GcpKmsDeleteCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *GcpKmsDeleteCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewGcpKmsDeleteCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	case 202:
		result := NewGcpKmsDeleteCollectionAccepted()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewGcpKmsDeleteCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewGcpKmsDeleteCollectionOK creates a GcpKmsDeleteCollectionOK with default headers values
func NewGcpKmsDeleteCollectionOK() *GcpKmsDeleteCollectionOK {
	return &GcpKmsDeleteCollectionOK{}
}

/*
GcpKmsDeleteCollectionOK describes a response with status code 200, with default header values.

OK
*/
type GcpKmsDeleteCollectionOK struct {
}

// IsSuccess returns true when this gcp kms delete collection o k response has a 2xx status code
func (o *GcpKmsDeleteCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this gcp kms delete collection o k response has a 3xx status code
func (o *GcpKmsDeleteCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this gcp kms delete collection o k response has a 4xx status code
func (o *GcpKmsDeleteCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this gcp kms delete collection o k response has a 5xx status code
func (o *GcpKmsDeleteCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this gcp kms delete collection o k response a status code equal to that given
func (o *GcpKmsDeleteCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the gcp kms delete collection o k response
func (o *GcpKmsDeleteCollectionOK) Code() int {
	return 200
}

func (o *GcpKmsDeleteCollectionOK) Error() string {
	return fmt.Sprintf("[DELETE /security/gcp-kms][%d] gcpKmsDeleteCollectionOK", 200)
}

func (o *GcpKmsDeleteCollectionOK) String() string {
	return fmt.Sprintf("[DELETE /security/gcp-kms][%d] gcpKmsDeleteCollectionOK", 200)
}

func (o *GcpKmsDeleteCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewGcpKmsDeleteCollectionAccepted creates a GcpKmsDeleteCollectionAccepted with default headers values
func NewGcpKmsDeleteCollectionAccepted() *GcpKmsDeleteCollectionAccepted {
	return &GcpKmsDeleteCollectionAccepted{}
}

/*
GcpKmsDeleteCollectionAccepted describes a response with status code 202, with default header values.

Accepted
*/
type GcpKmsDeleteCollectionAccepted struct {
	Payload *models.GcpKmsJobLinkResponse
}

// IsSuccess returns true when this gcp kms delete collection accepted response has a 2xx status code
func (o *GcpKmsDeleteCollectionAccepted) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this gcp kms delete collection accepted response has a 3xx status code
func (o *GcpKmsDeleteCollectionAccepted) IsRedirect() bool {
	return false
}

// IsClientError returns true when this gcp kms delete collection accepted response has a 4xx status code
func (o *GcpKmsDeleteCollectionAccepted) IsClientError() bool {
	return false
}

// IsServerError returns true when this gcp kms delete collection accepted response has a 5xx status code
func (o *GcpKmsDeleteCollectionAccepted) IsServerError() bool {
	return false
}

// IsCode returns true when this gcp kms delete collection accepted response a status code equal to that given
func (o *GcpKmsDeleteCollectionAccepted) IsCode(code int) bool {
	return code == 202
}

// Code gets the status code for the gcp kms delete collection accepted response
func (o *GcpKmsDeleteCollectionAccepted) Code() int {
	return 202
}

func (o *GcpKmsDeleteCollectionAccepted) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /security/gcp-kms][%d] gcpKmsDeleteCollectionAccepted %s", 202, payload)
}

func (o *GcpKmsDeleteCollectionAccepted) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /security/gcp-kms][%d] gcpKmsDeleteCollectionAccepted %s", 202, payload)
}

func (o *GcpKmsDeleteCollectionAccepted) GetPayload() *models.GcpKmsJobLinkResponse {
	return o.Payload
}

func (o *GcpKmsDeleteCollectionAccepted) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.GcpKmsJobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewGcpKmsDeleteCollectionDefault creates a GcpKmsDeleteCollectionDefault with default headers values
func NewGcpKmsDeleteCollectionDefault(code int) *GcpKmsDeleteCollectionDefault {
	return &GcpKmsDeleteCollectionDefault{
		_statusCode: code,
	}
}

/*
	GcpKmsDeleteCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 65536817 | Internal error. Failed to determine if it is safe to disable key manager. |
| 65536827 | Internal error. Failed to determine if the given SVM has any encrypted volumes. |
| 65536834 | Internal error. Failed to get existing key-server details for the given SVM. |
| 65536867 | Volume encryption keys (VEK) for one or more encrypted volumes are stored on the key manager configured for the given SVM. |
| 65536883 | Internal error. Volume encryption key is missing for a volume. |
| 65536884 | Internal error. Volume encryption key is invalid for a volume. |
| 65537721 | The Google Cloud Key Management Service is not configured for the SVM. |
| 196608080 | One or more nodes in the cluster have the root volume encrypted using NVE (NetApp Volume Encryption). |
| 196608301 | Internal error. Failed to get encryption type. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type GcpKmsDeleteCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this gcp kms delete collection default response has a 2xx status code
func (o *GcpKmsDeleteCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this gcp kms delete collection default response has a 3xx status code
func (o *GcpKmsDeleteCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this gcp kms delete collection default response has a 4xx status code
func (o *GcpKmsDeleteCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this gcp kms delete collection default response has a 5xx status code
func (o *GcpKmsDeleteCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this gcp kms delete collection default response a status code equal to that given
func (o *GcpKmsDeleteCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the gcp kms delete collection default response
func (o *GcpKmsDeleteCollectionDefault) Code() int {
	return o._statusCode
}

func (o *GcpKmsDeleteCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /security/gcp-kms][%d] gcp_kms_delete_collection default %s", o._statusCode, payload)
}

func (o *GcpKmsDeleteCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /security/gcp-kms][%d] gcp_kms_delete_collection default %s", o._statusCode, payload)
}

func (o *GcpKmsDeleteCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *GcpKmsDeleteCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
GcpKmsDeleteCollectionBody gcp kms delete collection body
swagger:model GcpKmsDeleteCollectionBody
*/
type GcpKmsDeleteCollectionBody struct {

	// gcp kms response inline records
	GcpKmsResponseInlineRecords []*models.GcpKms `json:"records,omitempty"`
}

// Validate validates this gcp kms delete collection body
func (o *GcpKmsDeleteCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateGcpKmsResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *GcpKmsDeleteCollectionBody) validateGcpKmsResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.GcpKmsResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.GcpKmsResponseInlineRecords); i++ {
		if swag.IsZero(o.GcpKmsResponseInlineRecords[i]) { // not required
			continue
		}

		if o.GcpKmsResponseInlineRecords[i] != nil {
			if err := o.GcpKmsResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this gcp kms delete collection body based on the context it is used
func (o *GcpKmsDeleteCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateGcpKmsResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *GcpKmsDeleteCollectionBody) contextValidateGcpKmsResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.GcpKmsResponseInlineRecords); i++ {

		if o.GcpKmsResponseInlineRecords[i] != nil {
			if err := o.GcpKmsResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// MarshalBinary interface implementation
func (o *GcpKmsDeleteCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *GcpKmsDeleteCollectionBody) UnmarshalBinary(b []byte) error {
	var res GcpKmsDeleteCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}