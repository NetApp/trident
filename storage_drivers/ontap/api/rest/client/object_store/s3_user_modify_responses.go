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

// S3UserModifyReader is a Reader for the S3UserModify structure.
type S3UserModifyReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *S3UserModifyReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewS3UserModifyOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewS3UserModifyDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewS3UserModifyOK creates a S3UserModifyOK with default headers values
func NewS3UserModifyOK() *S3UserModifyOK {
	return &S3UserModifyOK{}
}

/*
S3UserModifyOK describes a response with status code 200, with default header values.

OK
*/
type S3UserModifyOK struct {
	Payload *models.S3UserPostPatchResponse
}

// IsSuccess returns true when this s3 user modify o k response has a 2xx status code
func (o *S3UserModifyOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this s3 user modify o k response has a 3xx status code
func (o *S3UserModifyOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this s3 user modify o k response has a 4xx status code
func (o *S3UserModifyOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this s3 user modify o k response has a 5xx status code
func (o *S3UserModifyOK) IsServerError() bool {
	return false
}

// IsCode returns true when this s3 user modify o k response a status code equal to that given
func (o *S3UserModifyOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the s3 user modify o k response
func (o *S3UserModifyOK) Code() int {
	return 200
}

func (o *S3UserModifyOK) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /protocols/s3/services/{svm.uuid}/users/{name}][%d] s3UserModifyOK %s", 200, payload)
}

func (o *S3UserModifyOK) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /protocols/s3/services/{svm.uuid}/users/{name}][%d] s3UserModifyOK %s", 200, payload)
}

func (o *S3UserModifyOK) GetPayload() *models.S3UserPostPatchResponse {
	return o.Payload
}

func (o *S3UserModifyOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.S3UserPostPatchResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewS3UserModifyDefault creates a S3UserModifyDefault with default headers values
func NewS3UserModifyDefault(code int) *S3UserModifyDefault {
	return &S3UserModifyDefault{
		_statusCode: code,
	}
}

/*
	S3UserModifyDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 92405792   | Failed to regenerate access-key and secret-key for user. |
| 92406082   | Cannot perform \"regenerate_keys\" and \"delete_keys\" operations simultaneously on an S3 user. |
| 92406081   | The \"regenerate_keys\" operation on S3 User \"user-2\" in SVM \"vs1\" succeeded. However, modifying all of the other S3 user properties failed. Reason: resource limit exceeded. Retry the operation again without specifying the \"regenerate_keys\" parameter. |
| 92406080   | Cannot delete root user keys because there exists at least one S3 SnapMirror relationship that is using these keys. |
| 92406083   | The maximum supported value for user key expiry configuration is \"1095\" days. |
| 92406088   | The \"key_time_to_live\" parameter can only be used when the \"regenerate_keys\" operation is performed. |
| 92406096   | The user does not have permission to access the requested resource \\\"{0}\\\". |
| 92406097   | Internal error. The operation configuration is not correct. |

| 92406196   | The specified value for the \"key_time_to_live\" field cannot be greater than the maximum limit specified for the \"max_key_time_to_live\" field in the object store server. |
| 92406197   | Object store user \"user-2\" must have a non-zero value for the \"key_time_to_live\" field because the maximum limit specified for the \"max_key_time_to_live\" field in the object store server is not zero.
| 92406200   | An object store user with the same access-key already exists. |
| 92406201   | Missing access-key or secret-key. Either provide both of the keys or none. If not provided, keys are generated automatically. |
| 92406202   | The \"delete_keys\" operation must be performed without specifying the user keys. |
| 92406205   | The object store user access key contains invalid characters. Valid characters are 0-9 and A-Z. |
*/
type S3UserModifyDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this s3 user modify default response has a 2xx status code
func (o *S3UserModifyDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this s3 user modify default response has a 3xx status code
func (o *S3UserModifyDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this s3 user modify default response has a 4xx status code
func (o *S3UserModifyDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this s3 user modify default response has a 5xx status code
func (o *S3UserModifyDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this s3 user modify default response a status code equal to that given
func (o *S3UserModifyDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the s3 user modify default response
func (o *S3UserModifyDefault) Code() int {
	return o._statusCode
}

func (o *S3UserModifyDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /protocols/s3/services/{svm.uuid}/users/{name}][%d] s3_user_modify default %s", o._statusCode, payload)
}

func (o *S3UserModifyDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /protocols/s3/services/{svm.uuid}/users/{name}][%d] s3_user_modify default %s", o._statusCode, payload)
}

func (o *S3UserModifyDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *S3UserModifyDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
