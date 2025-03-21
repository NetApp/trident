// Code generated by go-swagger; DO NOT EDIT.

package ndmp

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

// NdmpNodeSessionGetReader is a Reader for the NdmpNodeSessionGet structure.
type NdmpNodeSessionGetReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *NdmpNodeSessionGetReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewNdmpNodeSessionGetOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewNdmpNodeSessionGetDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewNdmpNodeSessionGetOK creates a NdmpNodeSessionGetOK with default headers values
func NewNdmpNodeSessionGetOK() *NdmpNodeSessionGetOK {
	return &NdmpNodeSessionGetOK{}
}

/*
NdmpNodeSessionGetOK describes a response with status code 200, with default header values.

OK
*/
type NdmpNodeSessionGetOK struct {
	Payload *models.NdmpSession
}

// IsSuccess returns true when this ndmp node session get o k response has a 2xx status code
func (o *NdmpNodeSessionGetOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this ndmp node session get o k response has a 3xx status code
func (o *NdmpNodeSessionGetOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this ndmp node session get o k response has a 4xx status code
func (o *NdmpNodeSessionGetOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this ndmp node session get o k response has a 5xx status code
func (o *NdmpNodeSessionGetOK) IsServerError() bool {
	return false
}

// IsCode returns true when this ndmp node session get o k response a status code equal to that given
func (o *NdmpNodeSessionGetOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the ndmp node session get o k response
func (o *NdmpNodeSessionGetOK) Code() int {
	return 200
}

func (o *NdmpNodeSessionGetOK) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /protocols/ndmp/sessions/{owner.uuid}/{session.id}][%d] ndmpNodeSessionGetOK %s", 200, payload)
}

func (o *NdmpNodeSessionGetOK) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /protocols/ndmp/sessions/{owner.uuid}/{session.id}][%d] ndmpNodeSessionGetOK %s", 200, payload)
}

func (o *NdmpNodeSessionGetOK) GetPayload() *models.NdmpSession {
	return o.Payload
}

func (o *NdmpNodeSessionGetOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.NdmpSession)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewNdmpNodeSessionGetDefault creates a NdmpNodeSessionGetDefault with default headers values
func NewNdmpNodeSessionGetDefault(code int) *NdmpNodeSessionGetDefault {
	return &NdmpNodeSessionGetDefault{
		_statusCode: code,
	}
}

/*
	NdmpNodeSessionGetDefault describes a response with status code -1, with default header values.

	ONTAP Error Response codes

| Error code  |  Description |
|-------------|--------------|
| 68812802    | The UUID is not valid.|
*/
type NdmpNodeSessionGetDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this ndmp node session get default response has a 2xx status code
func (o *NdmpNodeSessionGetDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this ndmp node session get default response has a 3xx status code
func (o *NdmpNodeSessionGetDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this ndmp node session get default response has a 4xx status code
func (o *NdmpNodeSessionGetDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this ndmp node session get default response has a 5xx status code
func (o *NdmpNodeSessionGetDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this ndmp node session get default response a status code equal to that given
func (o *NdmpNodeSessionGetDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the ndmp node session get default response
func (o *NdmpNodeSessionGetDefault) Code() int {
	return o._statusCode
}

func (o *NdmpNodeSessionGetDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /protocols/ndmp/sessions/{owner.uuid}/{session.id}][%d] ndmp_node_session_get default %s", o._statusCode, payload)
}

func (o *NdmpNodeSessionGetDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /protocols/ndmp/sessions/{owner.uuid}/{session.id}][%d] ndmp_node_session_get default %s", o._statusCode, payload)
}

func (o *NdmpNodeSessionGetDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *NdmpNodeSessionGetDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
