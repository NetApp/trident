// Code generated by go-swagger; DO NOT EDIT.

package object_store

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

// S3BucketLifecycleRuleCollectionGetReader is a Reader for the S3BucketLifecycleRuleCollectionGet structure.
type S3BucketLifecycleRuleCollectionGetReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *S3BucketLifecycleRuleCollectionGetReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewS3BucketLifecycleRuleCollectionGetOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewS3BucketLifecycleRuleCollectionGetDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewS3BucketLifecycleRuleCollectionGetOK creates a S3BucketLifecycleRuleCollectionGetOK with default headers values
func NewS3BucketLifecycleRuleCollectionGetOK() *S3BucketLifecycleRuleCollectionGetOK {
	return &S3BucketLifecycleRuleCollectionGetOK{}
}

/*
S3BucketLifecycleRuleCollectionGetOK describes a response with status code 200, with default header values.

OK
*/
type S3BucketLifecycleRuleCollectionGetOK struct {
	Payload *models.S3BucketLifecycleRuleResponse
}

// IsSuccess returns true when this s3 bucket lifecycle rule collection get o k response has a 2xx status code
func (o *S3BucketLifecycleRuleCollectionGetOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this s3 bucket lifecycle rule collection get o k response has a 3xx status code
func (o *S3BucketLifecycleRuleCollectionGetOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this s3 bucket lifecycle rule collection get o k response has a 4xx status code
func (o *S3BucketLifecycleRuleCollectionGetOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this s3 bucket lifecycle rule collection get o k response has a 5xx status code
func (o *S3BucketLifecycleRuleCollectionGetOK) IsServerError() bool {
	return false
}

// IsCode returns true when this s3 bucket lifecycle rule collection get o k response a status code equal to that given
func (o *S3BucketLifecycleRuleCollectionGetOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the s3 bucket lifecycle rule collection get o k response
func (o *S3BucketLifecycleRuleCollectionGetOK) Code() int {
	return 200
}

func (o *S3BucketLifecycleRuleCollectionGetOK) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /protocols/s3/services/{svm.uuid}/buckets/{s3_bucket.uuid}/rules][%d] s3BucketLifecycleRuleCollectionGetOK %s", 200, payload)
}

func (o *S3BucketLifecycleRuleCollectionGetOK) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /protocols/s3/services/{svm.uuid}/buckets/{s3_bucket.uuid}/rules][%d] s3BucketLifecycleRuleCollectionGetOK %s", 200, payload)
}

func (o *S3BucketLifecycleRuleCollectionGetOK) GetPayload() *models.S3BucketLifecycleRuleResponse {
	return o.Payload
}

func (o *S3BucketLifecycleRuleCollectionGetOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.S3BucketLifecycleRuleResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewS3BucketLifecycleRuleCollectionGetDefault creates a S3BucketLifecycleRuleCollectionGetDefault with default headers values
func NewS3BucketLifecycleRuleCollectionGetDefault(code int) *S3BucketLifecycleRuleCollectionGetDefault {
	return &S3BucketLifecycleRuleCollectionGetDefault{
		_statusCode: code,
	}
}

/*
S3BucketLifecycleRuleCollectionGetDefault describes a response with status code -1, with default header values.

Error
*/
type S3BucketLifecycleRuleCollectionGetDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this s3 bucket lifecycle rule collection get default response has a 2xx status code
func (o *S3BucketLifecycleRuleCollectionGetDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this s3 bucket lifecycle rule collection get default response has a 3xx status code
func (o *S3BucketLifecycleRuleCollectionGetDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this s3 bucket lifecycle rule collection get default response has a 4xx status code
func (o *S3BucketLifecycleRuleCollectionGetDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this s3 bucket lifecycle rule collection get default response has a 5xx status code
func (o *S3BucketLifecycleRuleCollectionGetDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this s3 bucket lifecycle rule collection get default response a status code equal to that given
func (o *S3BucketLifecycleRuleCollectionGetDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the s3 bucket lifecycle rule collection get default response
func (o *S3BucketLifecycleRuleCollectionGetDefault) Code() int {
	return o._statusCode
}

func (o *S3BucketLifecycleRuleCollectionGetDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /protocols/s3/services/{svm.uuid}/buckets/{s3_bucket.uuid}/rules][%d] s3_bucket_lifecycle_rule_collection_get default %s", o._statusCode, payload)
}

func (o *S3BucketLifecycleRuleCollectionGetDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /protocols/s3/services/{svm.uuid}/buckets/{s3_bucket.uuid}/rules][%d] s3_bucket_lifecycle_rule_collection_get default %s", o._statusCode, payload)
}

func (o *S3BucketLifecycleRuleCollectionGetDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *S3BucketLifecycleRuleCollectionGetDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}