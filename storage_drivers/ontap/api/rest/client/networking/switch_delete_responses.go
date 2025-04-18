// Code generated by go-swagger; DO NOT EDIT.

package networking

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

// SwitchDeleteReader is a Reader for the SwitchDelete structure.
type SwitchDeleteReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *SwitchDeleteReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewSwitchDeleteOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewSwitchDeleteDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewSwitchDeleteOK creates a SwitchDeleteOK with default headers values
func NewSwitchDeleteOK() *SwitchDeleteOK {
	return &SwitchDeleteOK{}
}

/*
SwitchDeleteOK describes a response with status code 200, with default header values.

OK
*/
type SwitchDeleteOK struct {
}

// IsSuccess returns true when this switch delete o k response has a 2xx status code
func (o *SwitchDeleteOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this switch delete o k response has a 3xx status code
func (o *SwitchDeleteOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this switch delete o k response has a 4xx status code
func (o *SwitchDeleteOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this switch delete o k response has a 5xx status code
func (o *SwitchDeleteOK) IsServerError() bool {
	return false
}

// IsCode returns true when this switch delete o k response a status code equal to that given
func (o *SwitchDeleteOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the switch delete o k response
func (o *SwitchDeleteOK) Code() int {
	return 200
}

func (o *SwitchDeleteOK) Error() string {
	return fmt.Sprintf("[DELETE /network/ethernet/switches/{name}][%d] switchDeleteOK", 200)
}

func (o *SwitchDeleteOK) String() string {
	return fmt.Sprintf("[DELETE /network/ethernet/switches/{name}][%d] switchDeleteOK", 200)
}

func (o *SwitchDeleteOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewSwitchDeleteDefault creates a SwitchDeleteDefault with default headers values
func NewSwitchDeleteDefault(code int) *SwitchDeleteDefault {
	return &SwitchDeleteDefault{
		_statusCode: code,
	}
}

/*
SwitchDeleteDefault describes a response with status code -1, with default header values.

Error
*/
type SwitchDeleteDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this switch delete default response has a 2xx status code
func (o *SwitchDeleteDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this switch delete default response has a 3xx status code
func (o *SwitchDeleteDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this switch delete default response has a 4xx status code
func (o *SwitchDeleteDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this switch delete default response has a 5xx status code
func (o *SwitchDeleteDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this switch delete default response a status code equal to that given
func (o *SwitchDeleteDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the switch delete default response
func (o *SwitchDeleteDefault) Code() int {
	return o._statusCode
}

func (o *SwitchDeleteDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /network/ethernet/switches/{name}][%d] switch_delete default %s", o._statusCode, payload)
}

func (o *SwitchDeleteDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /network/ethernet/switches/{name}][%d] switch_delete default %s", o._statusCode, payload)
}

func (o *SwitchDeleteDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *SwitchDeleteDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
