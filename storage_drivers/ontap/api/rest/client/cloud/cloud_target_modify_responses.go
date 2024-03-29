// Code generated by go-swagger; DO NOT EDIT.

package cloud

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"
	"io"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// CloudTargetModifyReader is a Reader for the CloudTargetModify structure.
type CloudTargetModifyReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *CloudTargetModifyReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 202:
		result := NewCloudTargetModifyAccepted()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewCloudTargetModifyDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewCloudTargetModifyAccepted creates a CloudTargetModifyAccepted with default headers values
func NewCloudTargetModifyAccepted() *CloudTargetModifyAccepted {
	return &CloudTargetModifyAccepted{}
}

/*
CloudTargetModifyAccepted describes a response with status code 202, with default header values.

Accepted
*/
type CloudTargetModifyAccepted struct {
	Payload *models.JobLinkResponse
}

// IsSuccess returns true when this cloud target modify accepted response has a 2xx status code
func (o *CloudTargetModifyAccepted) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this cloud target modify accepted response has a 3xx status code
func (o *CloudTargetModifyAccepted) IsRedirect() bool {
	return false
}

// IsClientError returns true when this cloud target modify accepted response has a 4xx status code
func (o *CloudTargetModifyAccepted) IsClientError() bool {
	return false
}

// IsServerError returns true when this cloud target modify accepted response has a 5xx status code
func (o *CloudTargetModifyAccepted) IsServerError() bool {
	return false
}

// IsCode returns true when this cloud target modify accepted response a status code equal to that given
func (o *CloudTargetModifyAccepted) IsCode(code int) bool {
	return code == 202
}

func (o *CloudTargetModifyAccepted) Error() string {
	return fmt.Sprintf("[PATCH /cloud/targets/{uuid}][%d] cloudTargetModifyAccepted  %+v", 202, o.Payload)
}

func (o *CloudTargetModifyAccepted) String() string {
	return fmt.Sprintf("[PATCH /cloud/targets/{uuid}][%d] cloudTargetModifyAccepted  %+v", 202, o.Payload)
}

func (o *CloudTargetModifyAccepted) GetPayload() *models.JobLinkResponse {
	return o.Payload
}

func (o *CloudTargetModifyAccepted) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.JobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewCloudTargetModifyDefault creates a CloudTargetModifyDefault with default headers values
func NewCloudTargetModifyDefault(code int) *CloudTargetModifyDefault {
	return &CloudTargetModifyDefault{
		_statusCode: code,
	}
}

/*
CloudTargetModifyDefault describes a response with status code -1, with default header values.

Error
*/
type CloudTargetModifyDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// Code gets the status code for the cloud target modify default response
func (o *CloudTargetModifyDefault) Code() int {
	return o._statusCode
}

// IsSuccess returns true when this cloud target modify default response has a 2xx status code
func (o *CloudTargetModifyDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this cloud target modify default response has a 3xx status code
func (o *CloudTargetModifyDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this cloud target modify default response has a 4xx status code
func (o *CloudTargetModifyDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this cloud target modify default response has a 5xx status code
func (o *CloudTargetModifyDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this cloud target modify default response a status code equal to that given
func (o *CloudTargetModifyDefault) IsCode(code int) bool {
	return o._statusCode == code
}

func (o *CloudTargetModifyDefault) Error() string {
	return fmt.Sprintf("[PATCH /cloud/targets/{uuid}][%d] cloud_target_modify default  %+v", o._statusCode, o.Payload)
}

func (o *CloudTargetModifyDefault) String() string {
	return fmt.Sprintf("[PATCH /cloud/targets/{uuid}][%d] cloud_target_modify default  %+v", o._statusCode, o.Payload)
}

func (o *CloudTargetModifyDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *CloudTargetModifyDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
