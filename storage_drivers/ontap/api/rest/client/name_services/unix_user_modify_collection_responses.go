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

// UnixUserModifyCollectionReader is a Reader for the UnixUserModifyCollection structure.
type UnixUserModifyCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *UnixUserModifyCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewUnixUserModifyCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewUnixUserModifyCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewUnixUserModifyCollectionOK creates a UnixUserModifyCollectionOK with default headers values
func NewUnixUserModifyCollectionOK() *UnixUserModifyCollectionOK {
	return &UnixUserModifyCollectionOK{}
}

/*
UnixUserModifyCollectionOK describes a response with status code 200, with default header values.

OK
*/
type UnixUserModifyCollectionOK struct {
}

// IsSuccess returns true when this unix user modify collection o k response has a 2xx status code
func (o *UnixUserModifyCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this unix user modify collection o k response has a 3xx status code
func (o *UnixUserModifyCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this unix user modify collection o k response has a 4xx status code
func (o *UnixUserModifyCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this unix user modify collection o k response has a 5xx status code
func (o *UnixUserModifyCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this unix user modify collection o k response a status code equal to that given
func (o *UnixUserModifyCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the unix user modify collection o k response
func (o *UnixUserModifyCollectionOK) Code() int {
	return 200
}

func (o *UnixUserModifyCollectionOK) Error() string {
	return fmt.Sprintf("[PATCH /name-services/unix-users][%d] unixUserModifyCollectionOK", 200)
}

func (o *UnixUserModifyCollectionOK) String() string {
	return fmt.Sprintf("[PATCH /name-services/unix-users][%d] unixUserModifyCollectionOK", 200)
}

func (o *UnixUserModifyCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewUnixUserModifyCollectionDefault creates a UnixUserModifyCollectionDefault with default headers values
func NewUnixUserModifyCollectionDefault(code int) *UnixUserModifyCollectionDefault {
	return &UnixUserModifyCollectionDefault{
		_statusCode: code,
	}
}

/*
	UnixUserModifyCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 23724128   | The specified Unix user full-name contains invalid character ':' |
| 23724089   | The specified UNIX user full-name is too long. Maximum supported length is 256 characters. |
| 23724055   | Internal error. Failed to modify the UNIX user for the SVM. Verify that the cluster is healthy, then try the command again. |
| 23724090   | Configuring individual entries is not supported because file-only configuration is enabled. |
*/
type UnixUserModifyCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this unix user modify collection default response has a 2xx status code
func (o *UnixUserModifyCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this unix user modify collection default response has a 3xx status code
func (o *UnixUserModifyCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this unix user modify collection default response has a 4xx status code
func (o *UnixUserModifyCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this unix user modify collection default response has a 5xx status code
func (o *UnixUserModifyCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this unix user modify collection default response a status code equal to that given
func (o *UnixUserModifyCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the unix user modify collection default response
func (o *UnixUserModifyCollectionDefault) Code() int {
	return o._statusCode
}

func (o *UnixUserModifyCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /name-services/unix-users][%d] unix_user_modify_collection default %s", o._statusCode, payload)
}

func (o *UnixUserModifyCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /name-services/unix-users][%d] unix_user_modify_collection default %s", o._statusCode, payload)
}

func (o *UnixUserModifyCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *UnixUserModifyCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
UnixUserModifyCollectionBody unix user modify collection body
swagger:model UnixUserModifyCollectionBody
*/
type UnixUserModifyCollectionBody struct {

	// links
	Links *models.UnixUserInlineLinks `json:"_links,omitempty"`

	// User's full name.
	//
	// Example: Full User Name for user1
	FullName *string `json:"full_name,omitempty"`

	// UNIX user ID of the specified user.
	//
	ID *int64 `json:"id,omitempty"`

	// UNIX user name to be added to the local database.
	//
	// Example: user1
	Name *string `json:"name,omitempty"`

	// Primary group ID to which the user belongs.
	//
	PrimaryGid *int64 `json:"primary_gid,omitempty"`

	// Indicates whether or not the validation for the specified UNIX user name is disabled.
	SkipNameValidation *bool `json:"skip_name_validation,omitempty"`

	// svm
	Svm *models.UnixUserInlineSvm `json:"svm,omitempty"`

	// unix user response inline records
	UnixUserResponseInlineRecords []*models.UnixUser `json:"records,omitempty"`
}

// Validate validates this unix user modify collection body
func (o *UnixUserModifyCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateSvm(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateUnixUserResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *UnixUserModifyCollectionBody) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(o.Links) { // not required
		return nil
	}

	if o.Links != nil {
		if err := o.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

func (o *UnixUserModifyCollectionBody) validateSvm(formats strfmt.Registry) error {
	if swag.IsZero(o.Svm) { // not required
		return nil
	}

	if o.Svm != nil {
		if err := o.Svm.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "svm")
			}
			return err
		}
	}

	return nil
}

func (o *UnixUserModifyCollectionBody) validateUnixUserResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.UnixUserResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.UnixUserResponseInlineRecords); i++ {
		if swag.IsZero(o.UnixUserResponseInlineRecords[i]) { // not required
			continue
		}

		if o.UnixUserResponseInlineRecords[i] != nil {
			if err := o.UnixUserResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this unix user modify collection body based on the context it is used
func (o *UnixUserModifyCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateSvm(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateUnixUserResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *UnixUserModifyCollectionBody) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if o.Links != nil {
		if err := o.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

func (o *UnixUserModifyCollectionBody) contextValidateSvm(ctx context.Context, formats strfmt.Registry) error {

	if o.Svm != nil {
		if err := o.Svm.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "svm")
			}
			return err
		}
	}

	return nil
}

func (o *UnixUserModifyCollectionBody) contextValidateUnixUserResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.UnixUserResponseInlineRecords); i++ {

		if o.UnixUserResponseInlineRecords[i] != nil {
			if err := o.UnixUserResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
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
func (o *UnixUserModifyCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *UnixUserModifyCollectionBody) UnmarshalBinary(b []byte) error {
	var res UnixUserModifyCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
UnixUserInlineLinks unix user inline links
swagger:model unix_user_inline__links
*/
type UnixUserInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this unix user inline links
func (o *UnixUserInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *UnixUserInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(o.Self) { // not required
		return nil
	}

	if o.Self != nil {
		if err := o.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this unix user inline links based on the context it is used
func (o *UnixUserInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *UnixUserInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if o.Self != nil {
		if err := o.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *UnixUserInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *UnixUserInlineLinks) UnmarshalBinary(b []byte) error {
	var res UnixUserInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
UnixUserInlineSvm SVM, applies only to SVM-scoped objects.
swagger:model unix_user_inline_svm
*/
type UnixUserInlineSvm struct {

	// links
	Links *models.UnixUserInlineSvmInlineLinks `json:"_links,omitempty"`

	// The name of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: svm1
	Name *string `json:"name,omitempty"`

	// The unique identifier of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: 02c9e252-41be-11e9-81d5-00a0986138f7
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this unix user inline svm
func (o *UnixUserInlineSvm) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *UnixUserInlineSvm) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(o.Links) { // not required
		return nil
	}

	if o.Links != nil {
		if err := o.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "svm" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this unix user inline svm based on the context it is used
func (o *UnixUserInlineSvm) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *UnixUserInlineSvm) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if o.Links != nil {
		if err := o.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "svm" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *UnixUserInlineSvm) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *UnixUserInlineSvm) UnmarshalBinary(b []byte) error {
	var res UnixUserInlineSvm
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
UnixUserInlineSvmInlineLinks unix user inline svm inline links
swagger:model unix_user_inline_svm_inline__links
*/
type UnixUserInlineSvmInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this unix user inline svm inline links
func (o *UnixUserInlineSvmInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *UnixUserInlineSvmInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(o.Self) { // not required
		return nil
	}

	if o.Self != nil {
		if err := o.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "svm" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this unix user inline svm inline links based on the context it is used
func (o *UnixUserInlineSvmInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *UnixUserInlineSvmInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if o.Self != nil {
		if err := o.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "svm" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *UnixUserInlineSvmInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *UnixUserInlineSvmInlineLinks) UnmarshalBinary(b []byte) error {
	var res UnixUserInlineSvmInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}