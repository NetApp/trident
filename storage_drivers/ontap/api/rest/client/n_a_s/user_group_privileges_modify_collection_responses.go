// Code generated by go-swagger; DO NOT EDIT.

package n_a_s

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
	"github.com/go-openapi/validate"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// UserGroupPrivilegesModifyCollectionReader is a Reader for the UserGroupPrivilegesModifyCollection structure.
type UserGroupPrivilegesModifyCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *UserGroupPrivilegesModifyCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewUserGroupPrivilegesModifyCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewUserGroupPrivilegesModifyCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewUserGroupPrivilegesModifyCollectionOK creates a UserGroupPrivilegesModifyCollectionOK with default headers values
func NewUserGroupPrivilegesModifyCollectionOK() *UserGroupPrivilegesModifyCollectionOK {
	return &UserGroupPrivilegesModifyCollectionOK{}
}

/*
UserGroupPrivilegesModifyCollectionOK describes a response with status code 200, with default header values.

OK
*/
type UserGroupPrivilegesModifyCollectionOK struct {
}

// IsSuccess returns true when this user group privileges modify collection o k response has a 2xx status code
func (o *UserGroupPrivilegesModifyCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this user group privileges modify collection o k response has a 3xx status code
func (o *UserGroupPrivilegesModifyCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this user group privileges modify collection o k response has a 4xx status code
func (o *UserGroupPrivilegesModifyCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this user group privileges modify collection o k response has a 5xx status code
func (o *UserGroupPrivilegesModifyCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this user group privileges modify collection o k response a status code equal to that given
func (o *UserGroupPrivilegesModifyCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the user group privileges modify collection o k response
func (o *UserGroupPrivilegesModifyCollectionOK) Code() int {
	return 200
}

func (o *UserGroupPrivilegesModifyCollectionOK) Error() string {
	return fmt.Sprintf("[PATCH /protocols/cifs/users-and-groups/privileges][%d] userGroupPrivilegesModifyCollectionOK", 200)
}

func (o *UserGroupPrivilegesModifyCollectionOK) String() string {
	return fmt.Sprintf("[PATCH /protocols/cifs/users-and-groups/privileges][%d] userGroupPrivilegesModifyCollectionOK", 200)
}

func (o *UserGroupPrivilegesModifyCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewUserGroupPrivilegesModifyCollectionDefault creates a UserGroupPrivilegesModifyCollectionDefault with default headers values
func NewUserGroupPrivilegesModifyCollectionDefault(code int) *UserGroupPrivilegesModifyCollectionDefault {
	return &UserGroupPrivilegesModifyCollectionDefault{
		_statusCode: code,
	}
}

/*
	UserGroupPrivilegesModifyCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 262196     | Field 'svm.name' is not supported in the body of PATCH request. |
| 262203     | Field 'svm.uuid' is not supported in the body of PATCH request. |
| 655673     | Failed to resolve the user or group. |
| 655730     | The specified local user to which privileges are to be associated to does not exist. |
*/
type UserGroupPrivilegesModifyCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this user group privileges modify collection default response has a 2xx status code
func (o *UserGroupPrivilegesModifyCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this user group privileges modify collection default response has a 3xx status code
func (o *UserGroupPrivilegesModifyCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this user group privileges modify collection default response has a 4xx status code
func (o *UserGroupPrivilegesModifyCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this user group privileges modify collection default response has a 5xx status code
func (o *UserGroupPrivilegesModifyCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this user group privileges modify collection default response a status code equal to that given
func (o *UserGroupPrivilegesModifyCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the user group privileges modify collection default response
func (o *UserGroupPrivilegesModifyCollectionDefault) Code() int {
	return o._statusCode
}

func (o *UserGroupPrivilegesModifyCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /protocols/cifs/users-and-groups/privileges][%d] user_group_privileges_modify_collection default %s", o._statusCode, payload)
}

func (o *UserGroupPrivilegesModifyCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /protocols/cifs/users-and-groups/privileges][%d] user_group_privileges_modify_collection default %s", o._statusCode, payload)
}

func (o *UserGroupPrivilegesModifyCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *UserGroupPrivilegesModifyCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
UserGroupPrivilegesModifyCollectionBody user group privileges modify collection body
swagger:model UserGroupPrivilegesModifyCollectionBody
*/
type UserGroupPrivilegesModifyCollectionBody struct {

	// links
	Links *models.UserGroupPrivilegesInlineLinks `json:"_links,omitempty"`

	// Local or Active Directory user or group name.
	//
	// Example: user1
	Name *string `json:"name,omitempty"`

	// An array of privileges associated with the local or Active Directory user or group.
	// The available values are:
	// * SeTcbPrivilege              - Allows user to act as part of the operating system
	// * SeBackupPrivilege           - Allows user to back up files and directories, overriding any ACLs
	// * SeRestorePrivilege          - Allows user to restore files and directories, overriding any ACLs
	// * SeTakeOwnershipPrivilege    - Allows user to take ownership of files or other objects
	// * SeSecurityPrivilege         - Allows user to manage auditing and viewing/dumping/clearing the security log
	// * SeChangeNotifyPrivilege     - Allows user to bypass traverse checking
	//
	Privileges []*string `json:"privileges,omitempty"`

	// svm
	Svm *models.UserGroupPrivilegesInlineSvm `json:"svm,omitempty"`

	// user group privileges response inline records
	UserGroupPrivilegesResponseInlineRecords []*models.UserGroupPrivileges `json:"records,omitempty"`
}

// Validate validates this user group privileges modify collection body
func (o *UserGroupPrivilegesModifyCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validatePrivileges(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateSvm(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateUserGroupPrivilegesResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *UserGroupPrivilegesModifyCollectionBody) validateLinks(formats strfmt.Registry) error {
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

var userGroupPrivilegesModifyCollectionBodyPrivilegesItemsEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["setcbprivilege","sebackupprivilege","serestoreprivilege","setakeownershipprivilege","sesecurityprivilege","sechangenotifyprivilege"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		userGroupPrivilegesModifyCollectionBodyPrivilegesItemsEnum = append(userGroupPrivilegesModifyCollectionBodyPrivilegesItemsEnum, v)
	}
}

func (o *UserGroupPrivilegesModifyCollectionBody) validatePrivilegesItemsEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, userGroupPrivilegesModifyCollectionBodyPrivilegesItemsEnum, true); err != nil {
		return err
	}
	return nil
}

func (o *UserGroupPrivilegesModifyCollectionBody) validatePrivileges(formats strfmt.Registry) error {
	if swag.IsZero(o.Privileges) { // not required
		return nil
	}

	for i := 0; i < len(o.Privileges); i++ {
		if swag.IsZero(o.Privileges[i]) { // not required
			continue
		}

		// value enum
		if err := o.validatePrivilegesItemsEnum("info"+"."+"privileges"+"."+strconv.Itoa(i), "body", *o.Privileges[i]); err != nil {
			return err
		}

	}

	return nil
}

func (o *UserGroupPrivilegesModifyCollectionBody) validateSvm(formats strfmt.Registry) error {
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

func (o *UserGroupPrivilegesModifyCollectionBody) validateUserGroupPrivilegesResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.UserGroupPrivilegesResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.UserGroupPrivilegesResponseInlineRecords); i++ {
		if swag.IsZero(o.UserGroupPrivilegesResponseInlineRecords[i]) { // not required
			continue
		}

		if o.UserGroupPrivilegesResponseInlineRecords[i] != nil {
			if err := o.UserGroupPrivilegesResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this user group privileges modify collection body based on the context it is used
func (o *UserGroupPrivilegesModifyCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateSvm(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateUserGroupPrivilegesResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *UserGroupPrivilegesModifyCollectionBody) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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

func (o *UserGroupPrivilegesModifyCollectionBody) contextValidateSvm(ctx context.Context, formats strfmt.Registry) error {

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

func (o *UserGroupPrivilegesModifyCollectionBody) contextValidateUserGroupPrivilegesResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.UserGroupPrivilegesResponseInlineRecords); i++ {

		if o.UserGroupPrivilegesResponseInlineRecords[i] != nil {
			if err := o.UserGroupPrivilegesResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
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
func (o *UserGroupPrivilegesModifyCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *UserGroupPrivilegesModifyCollectionBody) UnmarshalBinary(b []byte) error {
	var res UserGroupPrivilegesModifyCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
UserGroupPrivilegesInlineLinks user group privileges inline links
swagger:model user_group_privileges_inline__links
*/
type UserGroupPrivilegesInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this user group privileges inline links
func (o *UserGroupPrivilegesInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *UserGroupPrivilegesInlineLinks) validateSelf(formats strfmt.Registry) error {
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

// ContextValidate validate this user group privileges inline links based on the context it is used
func (o *UserGroupPrivilegesInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *UserGroupPrivilegesInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

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
func (o *UserGroupPrivilegesInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *UserGroupPrivilegesInlineLinks) UnmarshalBinary(b []byte) error {
	var res UserGroupPrivilegesInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
UserGroupPrivilegesInlineSvm SVM, applies only to SVM-scoped objects.
swagger:model user_group_privileges_inline_svm
*/
type UserGroupPrivilegesInlineSvm struct {

	// links
	Links *models.UserGroupPrivilegesInlineSvmInlineLinks `json:"_links,omitempty"`

	// The name of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: svm1
	Name *string `json:"name,omitempty"`

	// The unique identifier of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: 02c9e252-41be-11e9-81d5-00a0986138f7
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this user group privileges inline svm
func (o *UserGroupPrivilegesInlineSvm) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *UserGroupPrivilegesInlineSvm) validateLinks(formats strfmt.Registry) error {
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

// ContextValidate validate this user group privileges inline svm based on the context it is used
func (o *UserGroupPrivilegesInlineSvm) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *UserGroupPrivilegesInlineSvm) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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
func (o *UserGroupPrivilegesInlineSvm) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *UserGroupPrivilegesInlineSvm) UnmarshalBinary(b []byte) error {
	var res UserGroupPrivilegesInlineSvm
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
UserGroupPrivilegesInlineSvmInlineLinks user group privileges inline svm inline links
swagger:model user_group_privileges_inline_svm_inline__links
*/
type UserGroupPrivilegesInlineSvmInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this user group privileges inline svm inline links
func (o *UserGroupPrivilegesInlineSvmInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *UserGroupPrivilegesInlineSvmInlineLinks) validateSelf(formats strfmt.Registry) error {
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

// ContextValidate validate this user group privileges inline svm inline links based on the context it is used
func (o *UserGroupPrivilegesInlineSvmInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *UserGroupPrivilegesInlineSvmInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

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
func (o *UserGroupPrivilegesInlineSvmInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *UserGroupPrivilegesInlineSvmInlineLinks) UnmarshalBinary(b []byte) error {
	var res UserGroupPrivilegesInlineSvmInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
