// Code generated by go-swagger; DO NOT EDIT.

package security

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"
	"io"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// GcpKmsDeleteReader is a Reader for the GcpKmsDelete structure.
type GcpKmsDeleteReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *GcpKmsDeleteReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewGcpKmsDeleteOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewGcpKmsDeleteDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewGcpKmsDeleteOK creates a GcpKmsDeleteOK with default headers values
func NewGcpKmsDeleteOK() *GcpKmsDeleteOK {
	return &GcpKmsDeleteOK{}
}

/*
GcpKmsDeleteOK describes a response with status code 200, with default header values.

OK
*/
type GcpKmsDeleteOK struct {
}

// IsSuccess returns true when this gcp kms delete o k response has a 2xx status code
func (o *GcpKmsDeleteOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this gcp kms delete o k response has a 3xx status code
func (o *GcpKmsDeleteOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this gcp kms delete o k response has a 4xx status code
func (o *GcpKmsDeleteOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this gcp kms delete o k response has a 5xx status code
func (o *GcpKmsDeleteOK) IsServerError() bool {
	return false
}

// IsCode returns true when this gcp kms delete o k response a status code equal to that given
func (o *GcpKmsDeleteOK) IsCode(code int) bool {
	return code == 200
}

func (o *GcpKmsDeleteOK) Error() string {
	return fmt.Sprintf("[DELETE /security/gcp-kms/{uuid}][%d] gcpKmsDeleteOK ", 200)
}

func (o *GcpKmsDeleteOK) String() string {
	return fmt.Sprintf("[DELETE /security/gcp-kms/{uuid}][%d] gcpKmsDeleteOK ", 200)
}

func (o *GcpKmsDeleteOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewGcpKmsDeleteDefault creates a GcpKmsDeleteDefault with default headers values
func NewGcpKmsDeleteDefault(code int) *GcpKmsDeleteDefault {
	return &GcpKmsDeleteDefault{
		_statusCode: code,
	}
}

/*
	GcpKmsDeleteDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 65536242 | One or more self-encrypting drives are assigned an authentication key. |
| 65536243 | Cannot determine authentication key presence on one or more self-encrypting drives. |
| 65536817 | Internal error. Failed to determine if it is safe to disable key manager. |
| 65536827 | Internal error. Failed to determine if the given SVM has any encrypted volumes. |
| 65536834 | Internal error. Failed to get existing key-server details for the given SVM. |
| 65536867 | Volume encryption keys (VEK) for one or more encrypted volumes are stored on the key manager configured for the given SVM. |
| 65536883 | Internal error. Volume encryption key is missing for a volume. |
| 65536884 | Internal error. Volume encryption key is invalid for a volume. |
| 65536924 | Cannot remove key manager that still contains one or more authentication keys for self-encrypting drives. |
| 65537721 | The Google Cloud Key Management Service is not configured for the SVM. |
| 196608080 | One or more nodes in the cluster have the root volume encrypted using NVE (NetApp Volume Encryption). |
| 196608301 | Internal error. Failed to get encryption type. |
| 196608305 | NAE aggregates found in the cluster. |
*/
type GcpKmsDeleteDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// Code gets the status code for the gcp kms delete default response
func (o *GcpKmsDeleteDefault) Code() int {
	return o._statusCode
}

// IsSuccess returns true when this gcp kms delete default response has a 2xx status code
func (o *GcpKmsDeleteDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this gcp kms delete default response has a 3xx status code
func (o *GcpKmsDeleteDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this gcp kms delete default response has a 4xx status code
func (o *GcpKmsDeleteDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this gcp kms delete default response has a 5xx status code
func (o *GcpKmsDeleteDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this gcp kms delete default response a status code equal to that given
func (o *GcpKmsDeleteDefault) IsCode(code int) bool {
	return o._statusCode == code
}

func (o *GcpKmsDeleteDefault) Error() string {
	return fmt.Sprintf("[DELETE /security/gcp-kms/{uuid}][%d] gcp_kms_delete default  %+v", o._statusCode, o.Payload)
}

func (o *GcpKmsDeleteDefault) String() string {
	return fmt.Sprintf("[DELETE /security/gcp-kms/{uuid}][%d] gcp_kms_delete default  %+v", o._statusCode, o.Payload)
}

func (o *GcpKmsDeleteDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *GcpKmsDeleteDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
