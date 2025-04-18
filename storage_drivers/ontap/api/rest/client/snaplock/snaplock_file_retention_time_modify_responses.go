// Code generated by go-swagger; DO NOT EDIT.

package snaplock

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

// SnaplockFileRetentionTimeModifyReader is a Reader for the SnaplockFileRetentionTimeModify structure.
type SnaplockFileRetentionTimeModifyReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *SnaplockFileRetentionTimeModifyReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewSnaplockFileRetentionTimeModifyOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewSnaplockFileRetentionTimeModifyDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewSnaplockFileRetentionTimeModifyOK creates a SnaplockFileRetentionTimeModifyOK with default headers values
func NewSnaplockFileRetentionTimeModifyOK() *SnaplockFileRetentionTimeModifyOK {
	return &SnaplockFileRetentionTimeModifyOK{}
}

/*
SnaplockFileRetentionTimeModifyOK describes a response with status code 200, with default header values.

OK
*/
type SnaplockFileRetentionTimeModifyOK struct {
}

// IsSuccess returns true when this snaplock file retention time modify o k response has a 2xx status code
func (o *SnaplockFileRetentionTimeModifyOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this snaplock file retention time modify o k response has a 3xx status code
func (o *SnaplockFileRetentionTimeModifyOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this snaplock file retention time modify o k response has a 4xx status code
func (o *SnaplockFileRetentionTimeModifyOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this snaplock file retention time modify o k response has a 5xx status code
func (o *SnaplockFileRetentionTimeModifyOK) IsServerError() bool {
	return false
}

// IsCode returns true when this snaplock file retention time modify o k response a status code equal to that given
func (o *SnaplockFileRetentionTimeModifyOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the snaplock file retention time modify o k response
func (o *SnaplockFileRetentionTimeModifyOK) Code() int {
	return 200
}

func (o *SnaplockFileRetentionTimeModifyOK) Error() string {
	return fmt.Sprintf("[PATCH /storage/snaplock/file/{volume.uuid}/{path}][%d] snaplockFileRetentionTimeModifyOK", 200)
}

func (o *SnaplockFileRetentionTimeModifyOK) String() string {
	return fmt.Sprintf("[PATCH /storage/snaplock/file/{volume.uuid}/{path}][%d] snaplockFileRetentionTimeModifyOK", 200)
}

func (o *SnaplockFileRetentionTimeModifyOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewSnaplockFileRetentionTimeModifyDefault creates a SnaplockFileRetentionTimeModifyDefault with default headers values
func NewSnaplockFileRetentionTimeModifyDefault(code int) *SnaplockFileRetentionTimeModifyDefault {
	return &SnaplockFileRetentionTimeModifyDefault{
		_statusCode: code,
	}
}

/*
	SnaplockFileRetentionTimeModifyDefault describes a response with status code -1, with default header values.

	ONTAP Error Response codes

| Error code  |  Description |
|-------------|--------------|
| 262179      | Unexpected argument \"<file_name>\"  |
| 262186      | Field \"expiry_time\" cannot be used with field \"retention_period\"  |
| 6691623     | User is not authorized  |
| 13763279    | The resulting expiry time due to the specified retention period is earlier than the current expiry time  |
| 14090348    | Invalid Expiry time  |
| 14090347    | File path must be in the format \"\/\<dir\>\/\<file path\>\"  |
| 917804      | Path should be given in the format \"\/\vol\/\<volume name>\/\<file path>\".
*/
type SnaplockFileRetentionTimeModifyDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this snaplock file retention time modify default response has a 2xx status code
func (o *SnaplockFileRetentionTimeModifyDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this snaplock file retention time modify default response has a 3xx status code
func (o *SnaplockFileRetentionTimeModifyDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this snaplock file retention time modify default response has a 4xx status code
func (o *SnaplockFileRetentionTimeModifyDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this snaplock file retention time modify default response has a 5xx status code
func (o *SnaplockFileRetentionTimeModifyDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this snaplock file retention time modify default response a status code equal to that given
func (o *SnaplockFileRetentionTimeModifyDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the snaplock file retention time modify default response
func (o *SnaplockFileRetentionTimeModifyDefault) Code() int {
	return o._statusCode
}

func (o *SnaplockFileRetentionTimeModifyDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /storage/snaplock/file/{volume.uuid}/{path}][%d] snaplock_file_retention_time_modify default %s", o._statusCode, payload)
}

func (o *SnaplockFileRetentionTimeModifyDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /storage/snaplock/file/{volume.uuid}/{path}][%d] snaplock_file_retention_time_modify default %s", o._statusCode, payload)
}

func (o *SnaplockFileRetentionTimeModifyDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *SnaplockFileRetentionTimeModifyDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
