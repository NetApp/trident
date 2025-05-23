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

// GcpKmsRestoreReader is a Reader for the GcpKmsRestore structure.
type GcpKmsRestoreReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *GcpKmsRestoreReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 201:
		result := NewGcpKmsRestoreCreated()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	case 202:
		result := NewGcpKmsRestoreAccepted()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewGcpKmsRestoreDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewGcpKmsRestoreCreated creates a GcpKmsRestoreCreated with default headers values
func NewGcpKmsRestoreCreated() *GcpKmsRestoreCreated {
	return &GcpKmsRestoreCreated{}
}

/*
GcpKmsRestoreCreated describes a response with status code 201, with default header values.

Created
*/
type GcpKmsRestoreCreated struct {

	/* Useful for tracking the resource location
	 */
	Location string

	Payload *models.GcpKmsJobLinkResponse
}

// IsSuccess returns true when this gcp kms restore created response has a 2xx status code
func (o *GcpKmsRestoreCreated) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this gcp kms restore created response has a 3xx status code
func (o *GcpKmsRestoreCreated) IsRedirect() bool {
	return false
}

// IsClientError returns true when this gcp kms restore created response has a 4xx status code
func (o *GcpKmsRestoreCreated) IsClientError() bool {
	return false
}

// IsServerError returns true when this gcp kms restore created response has a 5xx status code
func (o *GcpKmsRestoreCreated) IsServerError() bool {
	return false
}

// IsCode returns true when this gcp kms restore created response a status code equal to that given
func (o *GcpKmsRestoreCreated) IsCode(code int) bool {
	return code == 201
}

// Code gets the status code for the gcp kms restore created response
func (o *GcpKmsRestoreCreated) Code() int {
	return 201
}

func (o *GcpKmsRestoreCreated) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[POST /security/gcp-kms/{uuid}/restore][%d] gcpKmsRestoreCreated %s", 201, payload)
}

func (o *GcpKmsRestoreCreated) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[POST /security/gcp-kms/{uuid}/restore][%d] gcpKmsRestoreCreated %s", 201, payload)
}

func (o *GcpKmsRestoreCreated) GetPayload() *models.GcpKmsJobLinkResponse {
	return o.Payload
}

func (o *GcpKmsRestoreCreated) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	// hydrates response header Location
	hdrLocation := response.GetHeader("Location")

	if hdrLocation != "" {
		o.Location = hdrLocation
	}

	o.Payload = new(models.GcpKmsJobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewGcpKmsRestoreAccepted creates a GcpKmsRestoreAccepted with default headers values
func NewGcpKmsRestoreAccepted() *GcpKmsRestoreAccepted {
	return &GcpKmsRestoreAccepted{}
}

/*
GcpKmsRestoreAccepted describes a response with status code 202, with default header values.

Accepted
*/
type GcpKmsRestoreAccepted struct {

	/* Useful for tracking the resource location
	 */
	Location string

	Payload *models.GcpKmsJobLinkResponse
}

// IsSuccess returns true when this gcp kms restore accepted response has a 2xx status code
func (o *GcpKmsRestoreAccepted) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this gcp kms restore accepted response has a 3xx status code
func (o *GcpKmsRestoreAccepted) IsRedirect() bool {
	return false
}

// IsClientError returns true when this gcp kms restore accepted response has a 4xx status code
func (o *GcpKmsRestoreAccepted) IsClientError() bool {
	return false
}

// IsServerError returns true when this gcp kms restore accepted response has a 5xx status code
func (o *GcpKmsRestoreAccepted) IsServerError() bool {
	return false
}

// IsCode returns true when this gcp kms restore accepted response a status code equal to that given
func (o *GcpKmsRestoreAccepted) IsCode(code int) bool {
	return code == 202
}

// Code gets the status code for the gcp kms restore accepted response
func (o *GcpKmsRestoreAccepted) Code() int {
	return 202
}

func (o *GcpKmsRestoreAccepted) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[POST /security/gcp-kms/{uuid}/restore][%d] gcpKmsRestoreAccepted %s", 202, payload)
}

func (o *GcpKmsRestoreAccepted) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[POST /security/gcp-kms/{uuid}/restore][%d] gcpKmsRestoreAccepted %s", 202, payload)
}

func (o *GcpKmsRestoreAccepted) GetPayload() *models.GcpKmsJobLinkResponse {
	return o.Payload
}

func (o *GcpKmsRestoreAccepted) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	// hydrates response header Location
	hdrLocation := response.GetHeader("Location")

	if hdrLocation != "" {
		o.Location = hdrLocation
	}

	o.Payload = new(models.GcpKmsJobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewGcpKmsRestoreDefault creates a GcpKmsRestoreDefault with default headers values
func NewGcpKmsRestoreDefault(code int) *GcpKmsRestoreDefault {
	return &GcpKmsRestoreDefault{
		_statusCode: code,
	}
}

/*
	GcpKmsRestoreDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 65537544 | Missing wrapped top-level internal key protection key (KEK) from internal database. |
| 65537721 | The Google Cloud Key Management Service is not configured for the given SVM. |
| 65537722 | Failed to restore keys on the following nodes. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type GcpKmsRestoreDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this gcp kms restore default response has a 2xx status code
func (o *GcpKmsRestoreDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this gcp kms restore default response has a 3xx status code
func (o *GcpKmsRestoreDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this gcp kms restore default response has a 4xx status code
func (o *GcpKmsRestoreDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this gcp kms restore default response has a 5xx status code
func (o *GcpKmsRestoreDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this gcp kms restore default response a status code equal to that given
func (o *GcpKmsRestoreDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the gcp kms restore default response
func (o *GcpKmsRestoreDefault) Code() int {
	return o._statusCode
}

func (o *GcpKmsRestoreDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[POST /security/gcp-kms/{uuid}/restore][%d] gcp_kms_restore default %s", o._statusCode, payload)
}

func (o *GcpKmsRestoreDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[POST /security/gcp-kms/{uuid}/restore][%d] gcp_kms_restore default %s", o._statusCode, payload)
}

func (o *GcpKmsRestoreDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *GcpKmsRestoreDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
