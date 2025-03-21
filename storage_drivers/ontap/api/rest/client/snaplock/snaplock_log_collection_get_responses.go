// Code generated by go-swagger; DO NOT EDIT.

package snaplock

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

// SnaplockLogCollectionGetReader is a Reader for the SnaplockLogCollectionGet structure.
type SnaplockLogCollectionGetReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *SnaplockLogCollectionGetReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewSnaplockLogCollectionGetOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewSnaplockLogCollectionGetDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewSnaplockLogCollectionGetOK creates a SnaplockLogCollectionGetOK with default headers values
func NewSnaplockLogCollectionGetOK() *SnaplockLogCollectionGetOK {
	return &SnaplockLogCollectionGetOK{}
}

/*
SnaplockLogCollectionGetOK describes a response with status code 200, with default header values.

OK
*/
type SnaplockLogCollectionGetOK struct {
	Payload *models.SnaplockLogResponse
}

// IsSuccess returns true when this snaplock log collection get o k response has a 2xx status code
func (o *SnaplockLogCollectionGetOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this snaplock log collection get o k response has a 3xx status code
func (o *SnaplockLogCollectionGetOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this snaplock log collection get o k response has a 4xx status code
func (o *SnaplockLogCollectionGetOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this snaplock log collection get o k response has a 5xx status code
func (o *SnaplockLogCollectionGetOK) IsServerError() bool {
	return false
}

// IsCode returns true when this snaplock log collection get o k response a status code equal to that given
func (o *SnaplockLogCollectionGetOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the snaplock log collection get o k response
func (o *SnaplockLogCollectionGetOK) Code() int {
	return 200
}

func (o *SnaplockLogCollectionGetOK) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /storage/snaplock/audit-logs][%d] snaplockLogCollectionGetOK %s", 200, payload)
}

func (o *SnaplockLogCollectionGetOK) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /storage/snaplock/audit-logs][%d] snaplockLogCollectionGetOK %s", 200, payload)
}

func (o *SnaplockLogCollectionGetOK) GetPayload() *models.SnaplockLogResponse {
	return o.Payload
}

func (o *SnaplockLogCollectionGetOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.SnaplockLogResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewSnaplockLogCollectionGetDefault creates a SnaplockLogCollectionGetDefault with default headers values
func NewSnaplockLogCollectionGetDefault(code int) *SnaplockLogCollectionGetDefault {
	return &SnaplockLogCollectionGetDefault{
		_statusCode: code,
	}
}

/*
SnaplockLogCollectionGetDefault describes a response with status code -1, with default header values.

Error
*/
type SnaplockLogCollectionGetDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this snaplock log collection get default response has a 2xx status code
func (o *SnaplockLogCollectionGetDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this snaplock log collection get default response has a 3xx status code
func (o *SnaplockLogCollectionGetDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this snaplock log collection get default response has a 4xx status code
func (o *SnaplockLogCollectionGetDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this snaplock log collection get default response has a 5xx status code
func (o *SnaplockLogCollectionGetDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this snaplock log collection get default response a status code equal to that given
func (o *SnaplockLogCollectionGetDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the snaplock log collection get default response
func (o *SnaplockLogCollectionGetDefault) Code() int {
	return o._statusCode
}

func (o *SnaplockLogCollectionGetDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /storage/snaplock/audit-logs][%d] snaplock_log_collection_get default %s", o._statusCode, payload)
}

func (o *SnaplockLogCollectionGetDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /storage/snaplock/audit-logs][%d] snaplock_log_collection_get default %s", o._statusCode, payload)
}

func (o *SnaplockLogCollectionGetDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *SnaplockLogCollectionGetDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
