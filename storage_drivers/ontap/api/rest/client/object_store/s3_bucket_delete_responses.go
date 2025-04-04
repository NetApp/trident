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

// S3BucketDeleteReader is a Reader for the S3BucketDelete structure.
type S3BucketDeleteReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *S3BucketDeleteReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewS3BucketDeleteOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	case 202:
		result := NewS3BucketDeleteAccepted()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewS3BucketDeleteDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewS3BucketDeleteOK creates a S3BucketDeleteOK with default headers values
func NewS3BucketDeleteOK() *S3BucketDeleteOK {
	return &S3BucketDeleteOK{}
}

/*
S3BucketDeleteOK describes a response with status code 200, with default header values.

OK
*/
type S3BucketDeleteOK struct {
	Payload *models.S3BucketJobLinkResponse
}

// IsSuccess returns true when this s3 bucket delete o k response has a 2xx status code
func (o *S3BucketDeleteOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this s3 bucket delete o k response has a 3xx status code
func (o *S3BucketDeleteOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this s3 bucket delete o k response has a 4xx status code
func (o *S3BucketDeleteOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this s3 bucket delete o k response has a 5xx status code
func (o *S3BucketDeleteOK) IsServerError() bool {
	return false
}

// IsCode returns true when this s3 bucket delete o k response a status code equal to that given
func (o *S3BucketDeleteOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the s3 bucket delete o k response
func (o *S3BucketDeleteOK) Code() int {
	return 200
}

func (o *S3BucketDeleteOK) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/s3/buckets/{svm.uuid}/{uuid}][%d] s3BucketDeleteOK %s", 200, payload)
}

func (o *S3BucketDeleteOK) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/s3/buckets/{svm.uuid}/{uuid}][%d] s3BucketDeleteOK %s", 200, payload)
}

func (o *S3BucketDeleteOK) GetPayload() *models.S3BucketJobLinkResponse {
	return o.Payload
}

func (o *S3BucketDeleteOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.S3BucketJobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewS3BucketDeleteAccepted creates a S3BucketDeleteAccepted with default headers values
func NewS3BucketDeleteAccepted() *S3BucketDeleteAccepted {
	return &S3BucketDeleteAccepted{}
}

/*
S3BucketDeleteAccepted describes a response with status code 202, with default header values.

Accepted
*/
type S3BucketDeleteAccepted struct {
	Payload *models.S3BucketJobLinkResponse
}

// IsSuccess returns true when this s3 bucket delete accepted response has a 2xx status code
func (o *S3BucketDeleteAccepted) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this s3 bucket delete accepted response has a 3xx status code
func (o *S3BucketDeleteAccepted) IsRedirect() bool {
	return false
}

// IsClientError returns true when this s3 bucket delete accepted response has a 4xx status code
func (o *S3BucketDeleteAccepted) IsClientError() bool {
	return false
}

// IsServerError returns true when this s3 bucket delete accepted response has a 5xx status code
func (o *S3BucketDeleteAccepted) IsServerError() bool {
	return false
}

// IsCode returns true when this s3 bucket delete accepted response a status code equal to that given
func (o *S3BucketDeleteAccepted) IsCode(code int) bool {
	return code == 202
}

// Code gets the status code for the s3 bucket delete accepted response
func (o *S3BucketDeleteAccepted) Code() int {
	return 202
}

func (o *S3BucketDeleteAccepted) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/s3/buckets/{svm.uuid}/{uuid}][%d] s3BucketDeleteAccepted %s", 202, payload)
}

func (o *S3BucketDeleteAccepted) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/s3/buckets/{svm.uuid}/{uuid}][%d] s3BucketDeleteAccepted %s", 202, payload)
}

func (o *S3BucketDeleteAccepted) GetPayload() *models.S3BucketJobLinkResponse {
	return o.Payload
}

func (o *S3BucketDeleteAccepted) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.S3BucketJobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewS3BucketDeleteDefault creates a S3BucketDeleteDefault with default headers values
func NewS3BucketDeleteDefault(code int) *S3BucketDeleteDefault {
	return &S3BucketDeleteDefault{
		_statusCode: code,
	}
}

/*
	S3BucketDeleteDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error code | Message |
| ---------- | ------- |
| 92405811   | "Failed to delete bucket \\\"{bucket name}\\\" for SVM \\\"{svm.name}\\\". Wait a few minutes and try the operation again.";
| 92405858   | "Failed to \\\"delete\\\" the \\\"bucket\\\" because the operation is only supported on data SVMs.";
| 92405861   | "The specified SVM UUID or bucket UUID does not exist.";
| 92405779   | "Failed to remove bucket \\\"{bucket name}\\\" for SVM \\\"{svm.name}\\\". Reason: {Reason for failure}. ";
| 92405813   | "Failed to delete the object store volume. Reason: {Reason for failure}.";
| 92405864   | "An error occurred when deleting an access policy. The reason for failure is detailed in the error message.";
*/
type S3BucketDeleteDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this s3 bucket delete default response has a 2xx status code
func (o *S3BucketDeleteDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this s3 bucket delete default response has a 3xx status code
func (o *S3BucketDeleteDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this s3 bucket delete default response has a 4xx status code
func (o *S3BucketDeleteDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this s3 bucket delete default response has a 5xx status code
func (o *S3BucketDeleteDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this s3 bucket delete default response a status code equal to that given
func (o *S3BucketDeleteDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the s3 bucket delete default response
func (o *S3BucketDeleteDefault) Code() int {
	return o._statusCode
}

func (o *S3BucketDeleteDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/s3/buckets/{svm.uuid}/{uuid}][%d] s3_bucket_delete default %s", o._statusCode, payload)
}

func (o *S3BucketDeleteDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/s3/buckets/{svm.uuid}/{uuid}][%d] s3_bucket_delete default %s", o._statusCode, payload)
}

func (o *S3BucketDeleteDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *S3BucketDeleteDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
