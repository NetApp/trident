// Code generated by go-swagger; DO NOT EDIT.

package s_a_n

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

// PortsetGetReader is a Reader for the PortsetGet structure.
type PortsetGetReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *PortsetGetReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewPortsetGetOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewPortsetGetDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewPortsetGetOK creates a PortsetGetOK with default headers values
func NewPortsetGetOK() *PortsetGetOK {
	return &PortsetGetOK{}
}

/*
PortsetGetOK describes a response with status code 200, with default header values.

OK
*/
type PortsetGetOK struct {
	Payload *models.Portset
}

// IsSuccess returns true when this portset get o k response has a 2xx status code
func (o *PortsetGetOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this portset get o k response has a 3xx status code
func (o *PortsetGetOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this portset get o k response has a 4xx status code
func (o *PortsetGetOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this portset get o k response has a 5xx status code
func (o *PortsetGetOK) IsServerError() bool {
	return false
}

// IsCode returns true when this portset get o k response a status code equal to that given
func (o *PortsetGetOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the portset get o k response
func (o *PortsetGetOK) Code() int {
	return 200
}

func (o *PortsetGetOK) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /protocols/san/portsets/{uuid}][%d] portsetGetOK %s", 200, payload)
}

func (o *PortsetGetOK) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /protocols/san/portsets/{uuid}][%d] portsetGetOK %s", 200, payload)
}

func (o *PortsetGetOK) GetPayload() *models.Portset {
	return o.Payload
}

func (o *PortsetGetOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Portset)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewPortsetGetDefault creates a PortsetGetDefault with default headers values
func NewPortsetGetDefault(code int) *PortsetGetDefault {
	return &PortsetGetDefault{
		_statusCode: code,
	}
}

/*
	PortsetGetDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 4 | The portset does not exist. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type PortsetGetDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this portset get default response has a 2xx status code
func (o *PortsetGetDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this portset get default response has a 3xx status code
func (o *PortsetGetDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this portset get default response has a 4xx status code
func (o *PortsetGetDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this portset get default response has a 5xx status code
func (o *PortsetGetDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this portset get default response a status code equal to that given
func (o *PortsetGetDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the portset get default response
func (o *PortsetGetDefault) Code() int {
	return o._statusCode
}

func (o *PortsetGetDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /protocols/san/portsets/{uuid}][%d] portset_get default %s", o._statusCode, payload)
}

func (o *PortsetGetDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /protocols/san/portsets/{uuid}][%d] portset_get default %s", o._statusCode, payload)
}

func (o *PortsetGetDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *PortsetGetDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
