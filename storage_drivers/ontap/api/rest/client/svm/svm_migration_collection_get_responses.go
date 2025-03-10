// Code generated by go-swagger; DO NOT EDIT.

package svm

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

// SvmMigrationCollectionGetReader is a Reader for the SvmMigrationCollectionGet structure.
type SvmMigrationCollectionGetReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *SvmMigrationCollectionGetReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewSvmMigrationCollectionGetOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewSvmMigrationCollectionGetDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewSvmMigrationCollectionGetOK creates a SvmMigrationCollectionGetOK with default headers values
func NewSvmMigrationCollectionGetOK() *SvmMigrationCollectionGetOK {
	return &SvmMigrationCollectionGetOK{}
}

/*
SvmMigrationCollectionGetOK describes a response with status code 200, with default header values.

OK
*/
type SvmMigrationCollectionGetOK struct {
	Payload *models.SvmMigrationResponse
}

// IsSuccess returns true when this svm migration collection get o k response has a 2xx status code
func (o *SvmMigrationCollectionGetOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this svm migration collection get o k response has a 3xx status code
func (o *SvmMigrationCollectionGetOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this svm migration collection get o k response has a 4xx status code
func (o *SvmMigrationCollectionGetOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this svm migration collection get o k response has a 5xx status code
func (o *SvmMigrationCollectionGetOK) IsServerError() bool {
	return false
}

// IsCode returns true when this svm migration collection get o k response a status code equal to that given
func (o *SvmMigrationCollectionGetOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the svm migration collection get o k response
func (o *SvmMigrationCollectionGetOK) Code() int {
	return 200
}

func (o *SvmMigrationCollectionGetOK) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /svm/migrations][%d] svmMigrationCollectionGetOK %s", 200, payload)
}

func (o *SvmMigrationCollectionGetOK) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /svm/migrations][%d] svmMigrationCollectionGetOK %s", 200, payload)
}

func (o *SvmMigrationCollectionGetOK) GetPayload() *models.SvmMigrationResponse {
	return o.Payload
}

func (o *SvmMigrationCollectionGetOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.SvmMigrationResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewSvmMigrationCollectionGetDefault creates a SvmMigrationCollectionGetDefault with default headers values
func NewSvmMigrationCollectionGetDefault(code int) *SvmMigrationCollectionGetDefault {
	return &SvmMigrationCollectionGetDefault{
		_statusCode: code,
	}
}

/*
	SvmMigrationCollectionGetDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 13172783 | Migrate RDB lookup failed |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type SvmMigrationCollectionGetDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this svm migration collection get default response has a 2xx status code
func (o *SvmMigrationCollectionGetDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this svm migration collection get default response has a 3xx status code
func (o *SvmMigrationCollectionGetDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this svm migration collection get default response has a 4xx status code
func (o *SvmMigrationCollectionGetDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this svm migration collection get default response has a 5xx status code
func (o *SvmMigrationCollectionGetDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this svm migration collection get default response a status code equal to that given
func (o *SvmMigrationCollectionGetDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the svm migration collection get default response
func (o *SvmMigrationCollectionGetDefault) Code() int {
	return o._statusCode
}

func (o *SvmMigrationCollectionGetDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /svm/migrations][%d] svm_migration_collection_get default %s", o._statusCode, payload)
}

func (o *SvmMigrationCollectionGetDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /svm/migrations][%d] svm_migration_collection_get default %s", o._statusCode, payload)
}

func (o *SvmMigrationCollectionGetDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *SvmMigrationCollectionGetDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
