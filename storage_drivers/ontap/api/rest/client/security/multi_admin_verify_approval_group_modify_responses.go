// Code generated by go-swagger; DO NOT EDIT.

package security

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"
	"io"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// MultiAdminVerifyApprovalGroupModifyReader is a Reader for the MultiAdminVerifyApprovalGroupModify structure.
type MultiAdminVerifyApprovalGroupModifyReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *MultiAdminVerifyApprovalGroupModifyReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewMultiAdminVerifyApprovalGroupModifyOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewMultiAdminVerifyApprovalGroupModifyDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewMultiAdminVerifyApprovalGroupModifyOK creates a MultiAdminVerifyApprovalGroupModifyOK with default headers values
func NewMultiAdminVerifyApprovalGroupModifyOK() *MultiAdminVerifyApprovalGroupModifyOK {
	return &MultiAdminVerifyApprovalGroupModifyOK{}
}

/*
MultiAdminVerifyApprovalGroupModifyOK describes a response with status code 200, with default header values.

OK
*/
type MultiAdminVerifyApprovalGroupModifyOK struct {
}

// IsSuccess returns true when this multi admin verify approval group modify o k response has a 2xx status code
func (o *MultiAdminVerifyApprovalGroupModifyOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this multi admin verify approval group modify o k response has a 3xx status code
func (o *MultiAdminVerifyApprovalGroupModifyOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this multi admin verify approval group modify o k response has a 4xx status code
func (o *MultiAdminVerifyApprovalGroupModifyOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this multi admin verify approval group modify o k response has a 5xx status code
func (o *MultiAdminVerifyApprovalGroupModifyOK) IsServerError() bool {
	return false
}

// IsCode returns true when this multi admin verify approval group modify o k response a status code equal to that given
func (o *MultiAdminVerifyApprovalGroupModifyOK) IsCode(code int) bool {
	return code == 200
}

func (o *MultiAdminVerifyApprovalGroupModifyOK) Error() string {
	return fmt.Sprintf("[PATCH /security/multi-admin-verify/approval-groups/{owner.uuid}/{name}][%d] multiAdminVerifyApprovalGroupModifyOK ", 200)
}

func (o *MultiAdminVerifyApprovalGroupModifyOK) String() string {
	return fmt.Sprintf("[PATCH /security/multi-admin-verify/approval-groups/{owner.uuid}/{name}][%d] multiAdminVerifyApprovalGroupModifyOK ", 200)
}

func (o *MultiAdminVerifyApprovalGroupModifyOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewMultiAdminVerifyApprovalGroupModifyDefault creates a MultiAdminVerifyApprovalGroupModifyDefault with default headers values
func NewMultiAdminVerifyApprovalGroupModifyDefault(code int) *MultiAdminVerifyApprovalGroupModifyDefault {
	return &MultiAdminVerifyApprovalGroupModifyDefault{
		_statusCode: code,
	}
}

/*
	MultiAdminVerifyApprovalGroupModifyDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 262331 | At least one approver is required. |
| 262332 | An add or remove list is required. |
*/
type MultiAdminVerifyApprovalGroupModifyDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// Code gets the status code for the multi admin verify approval group modify default response
func (o *MultiAdminVerifyApprovalGroupModifyDefault) Code() int {
	return o._statusCode
}

// IsSuccess returns true when this multi admin verify approval group modify default response has a 2xx status code
func (o *MultiAdminVerifyApprovalGroupModifyDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this multi admin verify approval group modify default response has a 3xx status code
func (o *MultiAdminVerifyApprovalGroupModifyDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this multi admin verify approval group modify default response has a 4xx status code
func (o *MultiAdminVerifyApprovalGroupModifyDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this multi admin verify approval group modify default response has a 5xx status code
func (o *MultiAdminVerifyApprovalGroupModifyDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this multi admin verify approval group modify default response a status code equal to that given
func (o *MultiAdminVerifyApprovalGroupModifyDefault) IsCode(code int) bool {
	return o._statusCode == code
}

func (o *MultiAdminVerifyApprovalGroupModifyDefault) Error() string {
	return fmt.Sprintf("[PATCH /security/multi-admin-verify/approval-groups/{owner.uuid}/{name}][%d] multi_admin_verify_approval_group_modify default  %+v", o._statusCode, o.Payload)
}

func (o *MultiAdminVerifyApprovalGroupModifyDefault) String() string {
	return fmt.Sprintf("[PATCH /security/multi-admin-verify/approval-groups/{owner.uuid}/{name}][%d] multi_admin_verify_approval_group_modify default  %+v", o._statusCode, o.Payload)
}

func (o *MultiAdminVerifyApprovalGroupModifyDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *MultiAdminVerifyApprovalGroupModifyDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
