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

// S3BucketSvmDeleteReader is a Reader for the S3BucketSvmDelete structure.
type S3BucketSvmDeleteReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *S3BucketSvmDeleteReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewS3BucketSvmDeleteOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	case 202:
		result := NewS3BucketSvmDeleteAccepted()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewS3BucketSvmDeleteDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewS3BucketSvmDeleteOK creates a S3BucketSvmDeleteOK with default headers values
func NewS3BucketSvmDeleteOK() *S3BucketSvmDeleteOK {
	return &S3BucketSvmDeleteOK{}
}

/*
S3BucketSvmDeleteOK describes a response with status code 200, with default header values.

OK
*/
type S3BucketSvmDeleteOK struct {
	Payload *models.S3BucketSvmJobLinkResponse
}

// IsSuccess returns true when this s3 bucket svm delete o k response has a 2xx status code
func (o *S3BucketSvmDeleteOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this s3 bucket svm delete o k response has a 3xx status code
func (o *S3BucketSvmDeleteOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this s3 bucket svm delete o k response has a 4xx status code
func (o *S3BucketSvmDeleteOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this s3 bucket svm delete o k response has a 5xx status code
func (o *S3BucketSvmDeleteOK) IsServerError() bool {
	return false
}

// IsCode returns true when this s3 bucket svm delete o k response a status code equal to that given
func (o *S3BucketSvmDeleteOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the s3 bucket svm delete o k response
func (o *S3BucketSvmDeleteOK) Code() int {
	return 200
}

func (o *S3BucketSvmDeleteOK) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/s3/services/{svm.uuid}/buckets/{uuid}][%d] s3BucketSvmDeleteOK %s", 200, payload)
}

func (o *S3BucketSvmDeleteOK) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/s3/services/{svm.uuid}/buckets/{uuid}][%d] s3BucketSvmDeleteOK %s", 200, payload)
}

func (o *S3BucketSvmDeleteOK) GetPayload() *models.S3BucketSvmJobLinkResponse {
	return o.Payload
}

func (o *S3BucketSvmDeleteOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.S3BucketSvmJobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewS3BucketSvmDeleteAccepted creates a S3BucketSvmDeleteAccepted with default headers values
func NewS3BucketSvmDeleteAccepted() *S3BucketSvmDeleteAccepted {
	return &S3BucketSvmDeleteAccepted{}
}

/*
S3BucketSvmDeleteAccepted describes a response with status code 202, with default header values.

Accepted
*/
type S3BucketSvmDeleteAccepted struct {
	Payload *models.S3BucketSvmJobLinkResponse
}

// IsSuccess returns true when this s3 bucket svm delete accepted response has a 2xx status code
func (o *S3BucketSvmDeleteAccepted) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this s3 bucket svm delete accepted response has a 3xx status code
func (o *S3BucketSvmDeleteAccepted) IsRedirect() bool {
	return false
}

// IsClientError returns true when this s3 bucket svm delete accepted response has a 4xx status code
func (o *S3BucketSvmDeleteAccepted) IsClientError() bool {
	return false
}

// IsServerError returns true when this s3 bucket svm delete accepted response has a 5xx status code
func (o *S3BucketSvmDeleteAccepted) IsServerError() bool {
	return false
}

// IsCode returns true when this s3 bucket svm delete accepted response a status code equal to that given
func (o *S3BucketSvmDeleteAccepted) IsCode(code int) bool {
	return code == 202
}

// Code gets the status code for the s3 bucket svm delete accepted response
func (o *S3BucketSvmDeleteAccepted) Code() int {
	return 202
}

func (o *S3BucketSvmDeleteAccepted) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/s3/services/{svm.uuid}/buckets/{uuid}][%d] s3BucketSvmDeleteAccepted %s", 202, payload)
}

func (o *S3BucketSvmDeleteAccepted) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/s3/services/{svm.uuid}/buckets/{uuid}][%d] s3BucketSvmDeleteAccepted %s", 202, payload)
}

func (o *S3BucketSvmDeleteAccepted) GetPayload() *models.S3BucketSvmJobLinkResponse {
	return o.Payload
}

func (o *S3BucketSvmDeleteAccepted) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.S3BucketSvmJobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewS3BucketSvmDeleteDefault creates a S3BucketSvmDeleteDefault with default headers values
func NewS3BucketSvmDeleteDefault(code int) *S3BucketSvmDeleteDefault {
	return &S3BucketSvmDeleteDefault{
		_statusCode: code,
	}
}

/*
	S3BucketSvmDeleteDefault describes a response with status code -1, with default header values.

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
type S3BucketSvmDeleteDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this s3 bucket svm delete default response has a 2xx status code
func (o *S3BucketSvmDeleteDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this s3 bucket svm delete default response has a 3xx status code
func (o *S3BucketSvmDeleteDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this s3 bucket svm delete default response has a 4xx status code
func (o *S3BucketSvmDeleteDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this s3 bucket svm delete default response has a 5xx status code
func (o *S3BucketSvmDeleteDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this s3 bucket svm delete default response a status code equal to that given
func (o *S3BucketSvmDeleteDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the s3 bucket svm delete default response
func (o *S3BucketSvmDeleteDefault) Code() int {
	return o._statusCode
}

func (o *S3BucketSvmDeleteDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/s3/services/{svm.uuid}/buckets/{uuid}][%d] s3_bucket_svm_delete default %s", o._statusCode, payload)
}

func (o *S3BucketSvmDeleteDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/s3/services/{svm.uuid}/buckets/{uuid}][%d] s3_bucket_svm_delete default %s", o._statusCode, payload)
}

func (o *S3BucketSvmDeleteDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *S3BucketSvmDeleteDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
