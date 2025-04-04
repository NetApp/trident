// Code generated by go-swagger; DO NOT EDIT.

package storage

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

// PortCollectionGetReader is a Reader for the PortCollectionGet structure.
type PortCollectionGetReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *PortCollectionGetReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewPortCollectionGetOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewPortCollectionGetDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewPortCollectionGetOK creates a PortCollectionGetOK with default headers values
func NewPortCollectionGetOK() *PortCollectionGetOK {
	return &PortCollectionGetOK{}
}

/*
PortCollectionGetOK describes a response with status code 200, with default header values.

OK
*/
type PortCollectionGetOK struct {
	Payload *models.StoragePortResponse
}

// IsSuccess returns true when this port collection get o k response has a 2xx status code
func (o *PortCollectionGetOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this port collection get o k response has a 3xx status code
func (o *PortCollectionGetOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this port collection get o k response has a 4xx status code
func (o *PortCollectionGetOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this port collection get o k response has a 5xx status code
func (o *PortCollectionGetOK) IsServerError() bool {
	return false
}

// IsCode returns true when this port collection get o k response a status code equal to that given
func (o *PortCollectionGetOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the port collection get o k response
func (o *PortCollectionGetOK) Code() int {
	return 200
}

func (o *PortCollectionGetOK) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /storage/ports][%d] portCollectionGetOK %s", 200, payload)
}

func (o *PortCollectionGetOK) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /storage/ports][%d] portCollectionGetOK %s", 200, payload)
}

func (o *PortCollectionGetOK) GetPayload() *models.StoragePortResponse {
	return o.Payload
}

func (o *PortCollectionGetOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.StoragePortResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewPortCollectionGetDefault creates a PortCollectionGetDefault with default headers values
func NewPortCollectionGetDefault(code int) *PortCollectionGetDefault {
	return &PortCollectionGetDefault{
		_statusCode: code,
	}
}

/*
PortCollectionGetDefault describes a response with status code -1, with default header values.

Error
*/
type PortCollectionGetDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this port collection get default response has a 2xx status code
func (o *PortCollectionGetDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this port collection get default response has a 3xx status code
func (o *PortCollectionGetDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this port collection get default response has a 4xx status code
func (o *PortCollectionGetDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this port collection get default response has a 5xx status code
func (o *PortCollectionGetDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this port collection get default response a status code equal to that given
func (o *PortCollectionGetDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the port collection get default response
func (o *PortCollectionGetDefault) Code() int {
	return o._statusCode
}

func (o *PortCollectionGetDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /storage/ports][%d] port_collection_get default %s", o._statusCode, payload)
}

func (o *PortCollectionGetDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /storage/ports][%d] port_collection_get default %s", o._statusCode, payload)
}

func (o *PortCollectionGetDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *PortCollectionGetDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
