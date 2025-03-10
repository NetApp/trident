// Code generated by go-swagger; DO NOT EDIT.

package n_a_s

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

// GroupPoliciesToBeAppliedModifyReader is a Reader for the GroupPoliciesToBeAppliedModify structure.
type GroupPoliciesToBeAppliedModifyReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *GroupPoliciesToBeAppliedModifyReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewGroupPoliciesToBeAppliedModifyOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewGroupPoliciesToBeAppliedModifyDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewGroupPoliciesToBeAppliedModifyOK creates a GroupPoliciesToBeAppliedModifyOK with default headers values
func NewGroupPoliciesToBeAppliedModifyOK() *GroupPoliciesToBeAppliedModifyOK {
	return &GroupPoliciesToBeAppliedModifyOK{}
}

/*
GroupPoliciesToBeAppliedModifyOK describes a response with status code 200, with default header values.

OK
*/
type GroupPoliciesToBeAppliedModifyOK struct {
}

// IsSuccess returns true when this group policies to be applied modify o k response has a 2xx status code
func (o *GroupPoliciesToBeAppliedModifyOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this group policies to be applied modify o k response has a 3xx status code
func (o *GroupPoliciesToBeAppliedModifyOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this group policies to be applied modify o k response has a 4xx status code
func (o *GroupPoliciesToBeAppliedModifyOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this group policies to be applied modify o k response has a 5xx status code
func (o *GroupPoliciesToBeAppliedModifyOK) IsServerError() bool {
	return false
}

// IsCode returns true when this group policies to be applied modify o k response a status code equal to that given
func (o *GroupPoliciesToBeAppliedModifyOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the group policies to be applied modify o k response
func (o *GroupPoliciesToBeAppliedModifyOK) Code() int {
	return 200
}

func (o *GroupPoliciesToBeAppliedModifyOK) Error() string {
	return fmt.Sprintf("[PATCH /protocols/cifs/group-policies/{svm.uuid}][%d] groupPoliciesToBeAppliedModifyOK", 200)
}

func (o *GroupPoliciesToBeAppliedModifyOK) String() string {
	return fmt.Sprintf("[PATCH /protocols/cifs/group-policies/{svm.uuid}][%d] groupPoliciesToBeAppliedModifyOK", 200)
}

func (o *GroupPoliciesToBeAppliedModifyOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewGroupPoliciesToBeAppliedModifyDefault creates a GroupPoliciesToBeAppliedModifyDefault with default headers values
func NewGroupPoliciesToBeAppliedModifyDefault(code int) *GroupPoliciesToBeAppliedModifyDefault {
	return &GroupPoliciesToBeAppliedModifyDefault{
		_statusCode: code,
	}
}

/*
	GroupPoliciesToBeAppliedModifyDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 4456649 | Unable to get Group Policy information for SVM. |
| 4456650 | Group Policy is disabled for SVM. |
| 4456651 | Unable to get CIFS server information for SVM. |
| 4456652 | CIFS server is down for SVM. |
| 4456851 | An update is already in progress for specified SVM. Wait a few minutes, then try the command again. |
| 4456852 | Failed to set forced update for specified SVM. Reason: Internal error. Starting normal update. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type GroupPoliciesToBeAppliedModifyDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this group policies to be applied modify default response has a 2xx status code
func (o *GroupPoliciesToBeAppliedModifyDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this group policies to be applied modify default response has a 3xx status code
func (o *GroupPoliciesToBeAppliedModifyDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this group policies to be applied modify default response has a 4xx status code
func (o *GroupPoliciesToBeAppliedModifyDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this group policies to be applied modify default response has a 5xx status code
func (o *GroupPoliciesToBeAppliedModifyDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this group policies to be applied modify default response a status code equal to that given
func (o *GroupPoliciesToBeAppliedModifyDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the group policies to be applied modify default response
func (o *GroupPoliciesToBeAppliedModifyDefault) Code() int {
	return o._statusCode
}

func (o *GroupPoliciesToBeAppliedModifyDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /protocols/cifs/group-policies/{svm.uuid}][%d] group_policies_to_be_applied_modify default %s", o._statusCode, payload)
}

func (o *GroupPoliciesToBeAppliedModifyDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /protocols/cifs/group-policies/{svm.uuid}][%d] group_policies_to_be_applied_modify default %s", o._statusCode, payload)
}

func (o *GroupPoliciesToBeAppliedModifyDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *GroupPoliciesToBeAppliedModifyDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
