// Code generated by go-swagger; DO NOT EDIT.

package support

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

// EmsFilterDeleteReader is a Reader for the EmsFilterDelete structure.
type EmsFilterDeleteReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *EmsFilterDeleteReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewEmsFilterDeleteOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewEmsFilterDeleteDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewEmsFilterDeleteOK creates a EmsFilterDeleteOK with default headers values
func NewEmsFilterDeleteOK() *EmsFilterDeleteOK {
	return &EmsFilterDeleteOK{}
}

/*
EmsFilterDeleteOK describes a response with status code 200, with default header values.

OK
*/
type EmsFilterDeleteOK struct {
}

// IsSuccess returns true when this ems filter delete o k response has a 2xx status code
func (o *EmsFilterDeleteOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this ems filter delete o k response has a 3xx status code
func (o *EmsFilterDeleteOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this ems filter delete o k response has a 4xx status code
func (o *EmsFilterDeleteOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this ems filter delete o k response has a 5xx status code
func (o *EmsFilterDeleteOK) IsServerError() bool {
	return false
}

// IsCode returns true when this ems filter delete o k response a status code equal to that given
func (o *EmsFilterDeleteOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the ems filter delete o k response
func (o *EmsFilterDeleteOK) Code() int {
	return 200
}

func (o *EmsFilterDeleteOK) Error() string {
	return fmt.Sprintf("[DELETE /support/ems/filters/{name}][%d] emsFilterDeleteOK", 200)
}

func (o *EmsFilterDeleteOK) String() string {
	return fmt.Sprintf("[DELETE /support/ems/filters/{name}][%d] emsFilterDeleteOK", 200)
}

func (o *EmsFilterDeleteOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewEmsFilterDeleteDefault creates a EmsFilterDeleteDefault with default headers values
func NewEmsFilterDeleteDefault(code int) *EmsFilterDeleteDefault {
	return &EmsFilterDeleteDefault{
		_statusCode: code,
	}
}

/*
	EmsFilterDeleteDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 983113 | Default filters cannot be modified or removed |
| 983124 | Filter is being referenced by a destination |
| 983204 | Filter is being used for role-based operations |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type EmsFilterDeleteDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this ems filter delete default response has a 2xx status code
func (o *EmsFilterDeleteDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this ems filter delete default response has a 3xx status code
func (o *EmsFilterDeleteDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this ems filter delete default response has a 4xx status code
func (o *EmsFilterDeleteDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this ems filter delete default response has a 5xx status code
func (o *EmsFilterDeleteDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this ems filter delete default response a status code equal to that given
func (o *EmsFilterDeleteDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the ems filter delete default response
func (o *EmsFilterDeleteDefault) Code() int {
	return o._statusCode
}

func (o *EmsFilterDeleteDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /support/ems/filters/{name}][%d] ems_filter_delete default %s", o._statusCode, payload)
}

func (o *EmsFilterDeleteDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /support/ems/filters/{name}][%d] ems_filter_delete default %s", o._statusCode, payload)
}

func (o *EmsFilterDeleteDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *EmsFilterDeleteDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
