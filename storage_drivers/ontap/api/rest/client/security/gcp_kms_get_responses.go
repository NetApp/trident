// Code generated by go-swagger; DO NOT EDIT.

package security

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// GcpKmsGetReader is a Reader for the GcpKmsGet structure.
type GcpKmsGetReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *GcpKmsGetReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewGcpKmsGetOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewGcpKmsGetDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewGcpKmsGetOK creates a GcpKmsGetOK with default headers values
func NewGcpKmsGetOK() *GcpKmsGetOK {
	return &GcpKmsGetOK{}
}

/*
GcpKmsGetOK describes a response with status code 200, with default header values.

OK
*/
type GcpKmsGetOK struct {
	Payload *models.GcpKms
}

// IsSuccess returns true when this gcp kms get o k response has a 2xx status code
func (o *GcpKmsGetOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this gcp kms get o k response has a 3xx status code
func (o *GcpKmsGetOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this gcp kms get o k response has a 4xx status code
func (o *GcpKmsGetOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this gcp kms get o k response has a 5xx status code
func (o *GcpKmsGetOK) IsServerError() bool {
	return false
}

// IsCode returns true when this gcp kms get o k response a status code equal to that given
func (o *GcpKmsGetOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the gcp kms get o k response
func (o *GcpKmsGetOK) Code() int {
	return 200
}

func (o *GcpKmsGetOK) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /security/gcp-kms/{uuid}][%d] gcpKmsGetOK %s", 200, payload)
}

func (o *GcpKmsGetOK) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /security/gcp-kms/{uuid}][%d] gcpKmsGetOK %s", 200, payload)
}

func (o *GcpKmsGetOK) GetPayload() *models.GcpKms {
	return o.Payload
}

func (o *GcpKmsGetOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.GcpKms)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewGcpKmsGetDefault creates a GcpKmsGetDefault with default headers values
func NewGcpKmsGetDefault(code int) *GcpKmsGetDefault {
	return &GcpKmsGetDefault{
		_statusCode: code,
	}
}

/*
	GcpKmsGetDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 65537551 | Top-level internal key protection key (KEK) unavailable on one or more nodes. |
| 65537552 | Embedded KMIP server status not available. |
| 65537730 | The Google Cloud Key Management Service is unreachable from one or more nodes. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type GcpKmsGetDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this gcp kms get default response has a 2xx status code
func (o *GcpKmsGetDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this gcp kms get default response has a 3xx status code
func (o *GcpKmsGetDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this gcp kms get default response has a 4xx status code
func (o *GcpKmsGetDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this gcp kms get default response has a 5xx status code
func (o *GcpKmsGetDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this gcp kms get default response a status code equal to that given
func (o *GcpKmsGetDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the gcp kms get default response
func (o *GcpKmsGetDefault) Code() int {
	return o._statusCode
}

func (o *GcpKmsGetDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /security/gcp-kms/{uuid}][%d] gcp_kms_get default %s", o._statusCode, payload)
}

func (o *GcpKmsGetDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /security/gcp-kms/{uuid}][%d] gcp_kms_get default %s", o._statusCode, payload)
}

func (o *GcpKmsGetDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *GcpKmsGetDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
