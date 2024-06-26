// Code generated by go-swagger; DO NOT EDIT.

package storage

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"
	"io"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// QtreeDeleteReader is a Reader for the QtreeDelete structure.
type QtreeDeleteReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *QtreeDeleteReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 202:
		result := NewQtreeDeleteAccepted()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewQtreeDeleteDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewQtreeDeleteAccepted creates a QtreeDeleteAccepted with default headers values
func NewQtreeDeleteAccepted() *QtreeDeleteAccepted {
	return &QtreeDeleteAccepted{}
}

/*
QtreeDeleteAccepted describes a response with status code 202, with default header values.

Accepted
*/
type QtreeDeleteAccepted struct {
	Payload *models.JobLinkResponse
}

// IsSuccess returns true when this qtree delete accepted response has a 2xx status code
func (o *QtreeDeleteAccepted) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this qtree delete accepted response has a 3xx status code
func (o *QtreeDeleteAccepted) IsRedirect() bool {
	return false
}

// IsClientError returns true when this qtree delete accepted response has a 4xx status code
func (o *QtreeDeleteAccepted) IsClientError() bool {
	return false
}

// IsServerError returns true when this qtree delete accepted response has a 5xx status code
func (o *QtreeDeleteAccepted) IsServerError() bool {
	return false
}

// IsCode returns true when this qtree delete accepted response a status code equal to that given
func (o *QtreeDeleteAccepted) IsCode(code int) bool {
	return code == 202
}

func (o *QtreeDeleteAccepted) Error() string {
	return fmt.Sprintf("[DELETE /storage/qtrees/{volume.uuid}/{id}][%d] qtreeDeleteAccepted  %+v", 202, o.Payload)
}

func (o *QtreeDeleteAccepted) String() string {
	return fmt.Sprintf("[DELETE /storage/qtrees/{volume.uuid}/{id}][%d] qtreeDeleteAccepted  %+v", 202, o.Payload)
}

func (o *QtreeDeleteAccepted) GetPayload() *models.JobLinkResponse {
	return o.Payload
}

func (o *QtreeDeleteAccepted) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.JobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewQtreeDeleteDefault creates a QtreeDeleteDefault with default headers values
func NewQtreeDeleteDefault(code int) *QtreeDeleteDefault {
	return &QtreeDeleteDefault{
		_statusCode: code,
	}
}

/*
	QtreeDeleteDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 918235 | A volume with UUID was not found. |
| 5242925 | The limit for the number of concurrent delete jobs has been reached. |
| 5242955 | The UUID of the volume is required. |
| 5242957 | Failed to delete qtree with ID in the volume and SVM. |
*/
type QtreeDeleteDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// Code gets the status code for the qtree delete default response
func (o *QtreeDeleteDefault) Code() int {
	return o._statusCode
}

// IsSuccess returns true when this qtree delete default response has a 2xx status code
func (o *QtreeDeleteDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this qtree delete default response has a 3xx status code
func (o *QtreeDeleteDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this qtree delete default response has a 4xx status code
func (o *QtreeDeleteDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this qtree delete default response has a 5xx status code
func (o *QtreeDeleteDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this qtree delete default response a status code equal to that given
func (o *QtreeDeleteDefault) IsCode(code int) bool {
	return o._statusCode == code
}

func (o *QtreeDeleteDefault) Error() string {
	return fmt.Sprintf("[DELETE /storage/qtrees/{volume.uuid}/{id}][%d] qtree_delete default  %+v", o._statusCode, o.Payload)
}

func (o *QtreeDeleteDefault) String() string {
	return fmt.Sprintf("[DELETE /storage/qtrees/{volume.uuid}/{id}][%d] qtree_delete default  %+v", o._statusCode, o.Payload)
}

func (o *QtreeDeleteDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *QtreeDeleteDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
