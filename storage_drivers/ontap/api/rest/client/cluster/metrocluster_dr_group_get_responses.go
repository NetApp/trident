// Code generated by go-swagger; DO NOT EDIT.

package cluster

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

// MetroclusterDrGroupGetReader is a Reader for the MetroclusterDrGroupGet structure.
type MetroclusterDrGroupGetReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *MetroclusterDrGroupGetReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewMetroclusterDrGroupGetOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewMetroclusterDrGroupGetDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewMetroclusterDrGroupGetOK creates a MetroclusterDrGroupGetOK with default headers values
func NewMetroclusterDrGroupGetOK() *MetroclusterDrGroupGetOK {
	return &MetroclusterDrGroupGetOK{}
}

/*
MetroclusterDrGroupGetOK describes a response with status code 200, with default header values.

OK
*/
type MetroclusterDrGroupGetOK struct {
	Payload *models.MetroclusterDrGroup
}

// IsSuccess returns true when this metrocluster dr group get o k response has a 2xx status code
func (o *MetroclusterDrGroupGetOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this metrocluster dr group get o k response has a 3xx status code
func (o *MetroclusterDrGroupGetOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this metrocluster dr group get o k response has a 4xx status code
func (o *MetroclusterDrGroupGetOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this metrocluster dr group get o k response has a 5xx status code
func (o *MetroclusterDrGroupGetOK) IsServerError() bool {
	return false
}

// IsCode returns true when this metrocluster dr group get o k response a status code equal to that given
func (o *MetroclusterDrGroupGetOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the metrocluster dr group get o k response
func (o *MetroclusterDrGroupGetOK) Code() int {
	return 200
}

func (o *MetroclusterDrGroupGetOK) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /cluster/metrocluster/dr-groups/{id}][%d] metroclusterDrGroupGetOK %s", 200, payload)
}

func (o *MetroclusterDrGroupGetOK) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /cluster/metrocluster/dr-groups/{id}][%d] metroclusterDrGroupGetOK %s", 200, payload)
}

func (o *MetroclusterDrGroupGetOK) GetPayload() *models.MetroclusterDrGroup {
	return o.Payload
}

func (o *MetroclusterDrGroupGetOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.MetroclusterDrGroup)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewMetroclusterDrGroupGetDefault creates a MetroclusterDrGroupGetDefault with default headers values
func NewMetroclusterDrGroupGetDefault(code int) *MetroclusterDrGroupGetDefault {
	return &MetroclusterDrGroupGetDefault{
		_statusCode: code,
	}
}

/*
MetroclusterDrGroupGetDefault describes a response with status code -1, with default header values.

Error
*/
type MetroclusterDrGroupGetDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this metrocluster dr group get default response has a 2xx status code
func (o *MetroclusterDrGroupGetDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this metrocluster dr group get default response has a 3xx status code
func (o *MetroclusterDrGroupGetDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this metrocluster dr group get default response has a 4xx status code
func (o *MetroclusterDrGroupGetDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this metrocluster dr group get default response has a 5xx status code
func (o *MetroclusterDrGroupGetDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this metrocluster dr group get default response a status code equal to that given
func (o *MetroclusterDrGroupGetDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the metrocluster dr group get default response
func (o *MetroclusterDrGroupGetDefault) Code() int {
	return o._statusCode
}

func (o *MetroclusterDrGroupGetDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /cluster/metrocluster/dr-groups/{id}][%d] metrocluster_dr_group_get default %s", o._statusCode, payload)
}

func (o *MetroclusterDrGroupGetDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /cluster/metrocluster/dr-groups/{id}][%d] metrocluster_dr_group_get default %s", o._statusCode, payload)
}

func (o *MetroclusterDrGroupGetDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *MetroclusterDrGroupGetDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
