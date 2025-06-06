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

// SecurityCertificateDeleteCollectionReader is a Reader for the SecurityCertificateDeleteCollection structure.
type SecurityCertificateDeleteCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *SecurityCertificateDeleteCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewSecurityCertificateDeleteCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewSecurityCertificateDeleteCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewSecurityCertificateDeleteCollectionOK creates a SecurityCertificateDeleteCollectionOK with default headers values
func NewSecurityCertificateDeleteCollectionOK() *SecurityCertificateDeleteCollectionOK {
	return &SecurityCertificateDeleteCollectionOK{}
}

/*
SecurityCertificateDeleteCollectionOK describes a response with status code 200, with default header values.

OK
*/
type SecurityCertificateDeleteCollectionOK struct {
}

// IsSuccess returns true when this security certificate delete collection o k response has a 2xx status code
func (o *SecurityCertificateDeleteCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this security certificate delete collection o k response has a 3xx status code
func (o *SecurityCertificateDeleteCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this security certificate delete collection o k response has a 4xx status code
func (o *SecurityCertificateDeleteCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this security certificate delete collection o k response has a 5xx status code
func (o *SecurityCertificateDeleteCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this security certificate delete collection o k response a status code equal to that given
func (o *SecurityCertificateDeleteCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the security certificate delete collection o k response
func (o *SecurityCertificateDeleteCollectionOK) Code() int {
	return 200
}

func (o *SecurityCertificateDeleteCollectionOK) Error() string {
	return fmt.Sprintf("[DELETE /security/certificates][%d] securityCertificateDeleteCollectionOK", 200)
}

func (o *SecurityCertificateDeleteCollectionOK) String() string {
	return fmt.Sprintf("[DELETE /security/certificates][%d] securityCertificateDeleteCollectionOK", 200)
}

func (o *SecurityCertificateDeleteCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewSecurityCertificateDeleteCollectionDefault creates a SecurityCertificateDeleteCollectionDefault with default headers values
func NewSecurityCertificateDeleteCollectionDefault(code int) *SecurityCertificateDeleteCollectionDefault {
	return &SecurityCertificateDeleteCollectionDefault{
		_statusCode: code,
	}
}

/*
	SecurityCertificateDeleteCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 3735644    | Cannot delete server-chain certificate. Reason: There is a corresponding server certificate for it. |
| 3735679    | Cannot delete pre-installed server_ca certificates through REST. Use CLI or ZAPI. |
| 3735650    | Deleting this client_ca certificate directly is not supported. Delete the corresponding root-ca certificate using type `root_ca` to delete the root, client, and server certificates. |
| 3735627    | Deleting this server_ca certificate directly is not supported. Delete the corresponding root-ca certificate using type `root_ca` to delete the root, client, and server certificates. |
| 3735589    | Cannot delete certificate. |
| 3735590    | Cannot delete certificate. Failed to remove SSL configuration for the certificate. |
| 3735683    | Cannot remove this certificate while external key manager is configured. |
| 3735681    | Cannot delete preinstalled `server-ca` certificates. Use the CLI to complete the operation. |
| 52560272   | The certificate could not be removed due to being in use by one or more subsystems. |
*/
type SecurityCertificateDeleteCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this security certificate delete collection default response has a 2xx status code
func (o *SecurityCertificateDeleteCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this security certificate delete collection default response has a 3xx status code
func (o *SecurityCertificateDeleteCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this security certificate delete collection default response has a 4xx status code
func (o *SecurityCertificateDeleteCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this security certificate delete collection default response has a 5xx status code
func (o *SecurityCertificateDeleteCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this security certificate delete collection default response a status code equal to that given
func (o *SecurityCertificateDeleteCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the security certificate delete collection default response
func (o *SecurityCertificateDeleteCollectionDefault) Code() int {
	return o._statusCode
}

func (o *SecurityCertificateDeleteCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /security/certificates][%d] security_certificate_delete_collection default %s", o._statusCode, payload)
}

func (o *SecurityCertificateDeleteCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /security/certificates][%d] security_certificate_delete_collection default %s", o._statusCode, payload)
}

func (o *SecurityCertificateDeleteCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *SecurityCertificateDeleteCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
SecurityCertificateDeleteCollectionBody security certificate delete collection body
swagger:model SecurityCertificateDeleteCollectionBody
*/
type SecurityCertificateDeleteCollectionBody struct {

	// security certificate response inline records
	SecurityCertificateResponseInlineRecords []*models.SecurityCertificate `json:"records,omitempty"`
}

// Validate validates this security certificate delete collection body
func (o *SecurityCertificateDeleteCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSecurityCertificateResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SecurityCertificateDeleteCollectionBody) validateSecurityCertificateResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.SecurityCertificateResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.SecurityCertificateResponseInlineRecords); i++ {
		if swag.IsZero(o.SecurityCertificateResponseInlineRecords[i]) { // not required
			continue
		}

		if o.SecurityCertificateResponseInlineRecords[i] != nil {
			if err := o.SecurityCertificateResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this security certificate delete collection body based on the context it is used
func (o *SecurityCertificateDeleteCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSecurityCertificateResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SecurityCertificateDeleteCollectionBody) contextValidateSecurityCertificateResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.SecurityCertificateResponseInlineRecords); i++ {

		if o.SecurityCertificateResponseInlineRecords[i] != nil {
			if err := o.SecurityCertificateResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
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
func (o *SecurityCertificateDeleteCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *SecurityCertificateDeleteCollectionBody) UnmarshalBinary(b []byte) error {
	var res SecurityCertificateDeleteCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
