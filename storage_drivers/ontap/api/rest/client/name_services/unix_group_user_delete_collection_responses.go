// Code generated by go-swagger; DO NOT EDIT.

package name_services

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// UnixGroupUserDeleteCollectionReader is a Reader for the UnixGroupUserDeleteCollection structure.
type UnixGroupUserDeleteCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *UnixGroupUserDeleteCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewUnixGroupUserDeleteCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewUnixGroupUserDeleteCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewUnixGroupUserDeleteCollectionOK creates a UnixGroupUserDeleteCollectionOK with default headers values
func NewUnixGroupUserDeleteCollectionOK() *UnixGroupUserDeleteCollectionOK {
	return &UnixGroupUserDeleteCollectionOK{}
}

/*
UnixGroupUserDeleteCollectionOK describes a response with status code 200, with default header values.

OK
*/
type UnixGroupUserDeleteCollectionOK struct {
}

// IsSuccess returns true when this unix group user delete collection o k response has a 2xx status code
func (o *UnixGroupUserDeleteCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this unix group user delete collection o k response has a 3xx status code
func (o *UnixGroupUserDeleteCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this unix group user delete collection o k response has a 4xx status code
func (o *UnixGroupUserDeleteCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this unix group user delete collection o k response has a 5xx status code
func (o *UnixGroupUserDeleteCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this unix group user delete collection o k response a status code equal to that given
func (o *UnixGroupUserDeleteCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the unix group user delete collection o k response
func (o *UnixGroupUserDeleteCollectionOK) Code() int {
	return 200
}

func (o *UnixGroupUserDeleteCollectionOK) Error() string {
	return fmt.Sprintf("[DELETE /name-services/unix-groups/{svm.uuid}/{unix_group.name}/users][%d] unixGroupUserDeleteCollectionOK", 200)
}

func (o *UnixGroupUserDeleteCollectionOK) String() string {
	return fmt.Sprintf("[DELETE /name-services/unix-groups/{svm.uuid}/{unix_group.name}/users][%d] unixGroupUserDeleteCollectionOK", 200)
}

func (o *UnixGroupUserDeleteCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewUnixGroupUserDeleteCollectionDefault creates a UnixGroupUserDeleteCollectionDefault with default headers values
func NewUnixGroupUserDeleteCollectionDefault(code int) *UnixGroupUserDeleteCollectionDefault {
	return &UnixGroupUserDeleteCollectionDefault{
		_statusCode: code,
	}
}

/*
	UnixGroupUserDeleteCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 3276897    | The specified UNIX group does not exist in the SVM. |
*/
type UnixGroupUserDeleteCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this unix group user delete collection default response has a 2xx status code
func (o *UnixGroupUserDeleteCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this unix group user delete collection default response has a 3xx status code
func (o *UnixGroupUserDeleteCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this unix group user delete collection default response has a 4xx status code
func (o *UnixGroupUserDeleteCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this unix group user delete collection default response has a 5xx status code
func (o *UnixGroupUserDeleteCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this unix group user delete collection default response a status code equal to that given
func (o *UnixGroupUserDeleteCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the unix group user delete collection default response
func (o *UnixGroupUserDeleteCollectionDefault) Code() int {
	return o._statusCode
}

func (o *UnixGroupUserDeleteCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /name-services/unix-groups/{svm.uuid}/{unix_group.name}/users][%d] unix_group_user_delete_collection default %s", o._statusCode, payload)
}

func (o *UnixGroupUserDeleteCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /name-services/unix-groups/{svm.uuid}/{unix_group.name}/users][%d] unix_group_user_delete_collection default %s", o._statusCode, payload)
}

func (o *UnixGroupUserDeleteCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *UnixGroupUserDeleteCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
UnixGroupUserDeleteCollectionBody unix group user delete collection body
swagger:model UnixGroupUserDeleteCollectionBody
*/
type UnixGroupUserDeleteCollectionBody struct {

	// unix group users response inline records
	UnixGroupUsersResponseInlineRecords []*models.UnixGroupUsers `json:"records,omitempty"`
}

// Validate validates this unix group user delete collection body
func (o *UnixGroupUserDeleteCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateUnixGroupUsersResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *UnixGroupUserDeleteCollectionBody) validateUnixGroupUsersResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.UnixGroupUsersResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.UnixGroupUsersResponseInlineRecords); i++ {
		if swag.IsZero(o.UnixGroupUsersResponseInlineRecords[i]) { // not required
			continue
		}

		if o.UnixGroupUsersResponseInlineRecords[i] != nil {
			if err := o.UnixGroupUsersResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this unix group user delete collection body based on the context it is used
func (o *UnixGroupUserDeleteCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateUnixGroupUsersResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *UnixGroupUserDeleteCollectionBody) contextValidateUnixGroupUsersResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.UnixGroupUsersResponseInlineRecords); i++ {

		if o.UnixGroupUsersResponseInlineRecords[i] != nil {
			if err := o.UnixGroupUsersResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// MarshalBinary interface implementation
func (o *UnixGroupUserDeleteCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *UnixGroupUserDeleteCollectionBody) UnmarshalBinary(b []byte) error {
	var res UnixGroupUserDeleteCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}