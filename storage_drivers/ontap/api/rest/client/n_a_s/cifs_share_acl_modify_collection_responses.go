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

// CifsShareACLModifyCollectionReader is a Reader for the CifsShareACLModifyCollection structure.
type CifsShareACLModifyCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *CifsShareACLModifyCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewCifsShareACLModifyCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewCifsShareACLModifyCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewCifsShareACLModifyCollectionOK creates a CifsShareACLModifyCollectionOK with default headers values
func NewCifsShareACLModifyCollectionOK() *CifsShareACLModifyCollectionOK {
	return &CifsShareACLModifyCollectionOK{}
}

/*
CifsShareACLModifyCollectionOK describes a response with status code 200, with default header values.

OK
*/
type CifsShareACLModifyCollectionOK struct {
}

// IsSuccess returns true when this cifs share Acl modify collection o k response has a 2xx status code
func (o *CifsShareACLModifyCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this cifs share Acl modify collection o k response has a 3xx status code
func (o *CifsShareACLModifyCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this cifs share Acl modify collection o k response has a 4xx status code
func (o *CifsShareACLModifyCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this cifs share Acl modify collection o k response has a 5xx status code
func (o *CifsShareACLModifyCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this cifs share Acl modify collection o k response a status code equal to that given
func (o *CifsShareACLModifyCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the cifs share Acl modify collection o k response
func (o *CifsShareACLModifyCollectionOK) Code() int {
	return 200
}

func (o *CifsShareACLModifyCollectionOK) Error() string {
	return fmt.Sprintf("[PATCH /protocols/cifs/shares/{svm.uuid}/{share}/acls][%d] cifsShareAclModifyCollectionOK", 200)
}

func (o *CifsShareACLModifyCollectionOK) String() string {
	return fmt.Sprintf("[PATCH /protocols/cifs/shares/{svm.uuid}/{share}/acls][%d] cifsShareAclModifyCollectionOK", 200)
}

func (o *CifsShareACLModifyCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewCifsShareACLModifyCollectionDefault creates a CifsShareACLModifyCollectionDefault with default headers values
func NewCifsShareACLModifyCollectionDefault(code int) *CifsShareACLModifyCollectionDefault {
	return &CifsShareACLModifyCollectionDefault{
		_statusCode: code,
	}
}

/*
	CifsShareACLModifyCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 655516     | The share ACL does not exist for given user and share |
*/
type CifsShareACLModifyCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this cifs share acl modify collection default response has a 2xx status code
func (o *CifsShareACLModifyCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this cifs share acl modify collection default response has a 3xx status code
func (o *CifsShareACLModifyCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this cifs share acl modify collection default response has a 4xx status code
func (o *CifsShareACLModifyCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this cifs share acl modify collection default response has a 5xx status code
func (o *CifsShareACLModifyCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this cifs share acl modify collection default response a status code equal to that given
func (o *CifsShareACLModifyCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the cifs share acl modify collection default response
func (o *CifsShareACLModifyCollectionDefault) Code() int {
	return o._statusCode
}

func (o *CifsShareACLModifyCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /protocols/cifs/shares/{svm.uuid}/{share}/acls][%d] cifs_share_acl_modify_collection default %s", o._statusCode, payload)
}

func (o *CifsShareACLModifyCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /protocols/cifs/shares/{svm.uuid}/{share}/acls][%d] cifs_share_acl_modify_collection default %s", o._statusCode, payload)
}

func (o *CifsShareACLModifyCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *CifsShareACLModifyCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
CifsShareACLModifyCollectionBody cifs share ACL modify collection body
swagger:model CifsShareACLModifyCollectionBody
*/
type CifsShareACLModifyCollectionBody struct {

	// links
	Links *models.CifsShareACLInlineLinks `json:"_links,omitempty"`

	// cifs share acl response inline records
	CifsShareACLResponseInlineRecords []*models.CifsShareACL `json:"records,omitempty"`

	// Specifies the access rights that a user or group has on the defined CIFS Share.
	// The following values are allowed:
	// * no_access    - User does not have CIFS share access
	// * read         - User has only read access
	// * change       - User has change access
	// * full_control - User has full_control access
	//
	// Enum: ["no_access","read","change","full_control"]
	Permission *string `json:"permission,omitempty"`

	// CIFS share name
	// Read Only: true
	Share *string `json:"share,omitempty"`

	// Specifies the user or group secure identifier (SID).
	// Example: S-1-5-21-256008430-3394229847-3930036330-1001
	// Read Only: true
	Sid *string `json:"sid,omitempty"`

	// svm
	Svm *models.CifsShareACLInlineSvm `json:"svm,omitempty"`

	// Specifies the type of the user or group to add to the access control
	// list of a CIFS share. The following values are allowed:
	// * windows    - Windows user or group
	// * unix_user  - UNIX user
	// * unix_group - UNIX group
	//
	// Enum: ["windows","unix_user","unix_group"]
	Type *string `json:"type,omitempty"`

	// Specifies the UNIX user or group identifier (UID/GID).
	// Example: 100
	// Read Only: true
	UnixID *int64 `json:"unix_id,omitempty"`

	// Specifies the user or group name to add to the access control list of a CIFS share.
	// Example: ENGDOMAIN\\ad_user
	UserOrGroup *string `json:"user_or_group,omitempty"`
}

// Validate validates this cifs share ACL modify collection body
func (o *CifsShareACLModifyCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateCifsShareACLResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validatePermission(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateSvm(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateType(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *CifsShareACLModifyCollectionBody) validateLinks(formats strfmt.Registry) error {
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

func (o *CifsShareACLModifyCollectionBody) validateCifsShareACLResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.CifsShareACLResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.CifsShareACLResponseInlineRecords); i++ {
		if swag.IsZero(o.CifsShareACLResponseInlineRecords[i]) { // not required
			continue
		}

		if o.CifsShareACLResponseInlineRecords[i] != nil {
			if err := o.CifsShareACLResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

var cifsShareAclModifyCollectionBodyTypePermissionPropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["no_access","read","change","full_control"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		cifsShareAclModifyCollectionBodyTypePermissionPropEnum = append(cifsShareAclModifyCollectionBodyTypePermissionPropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// CifsShareACLModifyCollectionBody
	// CifsShareACLModifyCollectionBody
	// permission
	// Permission
	// no_access
	// END DEBUGGING
	// CifsShareACLModifyCollectionBodyPermissionNoAccess captures enum value "no_access"
	CifsShareACLModifyCollectionBodyPermissionNoAccess string = "no_access"

	// BEGIN DEBUGGING
	// CifsShareACLModifyCollectionBody
	// CifsShareACLModifyCollectionBody
	// permission
	// Permission
	// read
	// END DEBUGGING
	// CifsShareACLModifyCollectionBodyPermissionRead captures enum value "read"
	CifsShareACLModifyCollectionBodyPermissionRead string = "read"

	// BEGIN DEBUGGING
	// CifsShareACLModifyCollectionBody
	// CifsShareACLModifyCollectionBody
	// permission
	// Permission
	// change
	// END DEBUGGING
	// CifsShareACLModifyCollectionBodyPermissionChange captures enum value "change"
	CifsShareACLModifyCollectionBodyPermissionChange string = "change"

	// BEGIN DEBUGGING
	// CifsShareACLModifyCollectionBody
	// CifsShareACLModifyCollectionBody
	// permission
	// Permission
	// full_control
	// END DEBUGGING
	// CifsShareACLModifyCollectionBodyPermissionFullControl captures enum value "full_control"
	CifsShareACLModifyCollectionBodyPermissionFullControl string = "full_control"
)

// prop value enum
func (o *CifsShareACLModifyCollectionBody) validatePermissionEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, cifsShareAclModifyCollectionBodyTypePermissionPropEnum, true); err != nil {
		return err
	}
	return nil
}

func (o *CifsShareACLModifyCollectionBody) validatePermission(formats strfmt.Registry) error {
	if swag.IsZero(o.Permission) { // not required
		return nil
	}

	// value enum
	if err := o.validatePermissionEnum("info"+"."+"permission", "body", *o.Permission); err != nil {
		return err
	}

	return nil
}

func (o *CifsShareACLModifyCollectionBody) validateSvm(formats strfmt.Registry) error {
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

var cifsShareAclModifyCollectionBodyTypeTypePropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["windows","unix_user","unix_group"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		cifsShareAclModifyCollectionBodyTypeTypePropEnum = append(cifsShareAclModifyCollectionBodyTypeTypePropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// CifsShareACLModifyCollectionBody
	// CifsShareACLModifyCollectionBody
	// type
	// Type
	// windows
	// END DEBUGGING
	// CifsShareACLModifyCollectionBodyTypeWindows captures enum value "windows"
	CifsShareACLModifyCollectionBodyTypeWindows string = "windows"

	// BEGIN DEBUGGING
	// CifsShareACLModifyCollectionBody
	// CifsShareACLModifyCollectionBody
	// type
	// Type
	// unix_user
	// END DEBUGGING
	// CifsShareACLModifyCollectionBodyTypeUnixUser captures enum value "unix_user"
	CifsShareACLModifyCollectionBodyTypeUnixUser string = "unix_user"

	// BEGIN DEBUGGING
	// CifsShareACLModifyCollectionBody
	// CifsShareACLModifyCollectionBody
	// type
	// Type
	// unix_group
	// END DEBUGGING
	// CifsShareACLModifyCollectionBodyTypeUnixGroup captures enum value "unix_group"
	CifsShareACLModifyCollectionBodyTypeUnixGroup string = "unix_group"
)

// prop value enum
func (o *CifsShareACLModifyCollectionBody) validateTypeEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, cifsShareAclModifyCollectionBodyTypeTypePropEnum, true); err != nil {
		return err
	}
	return nil
}

func (o *CifsShareACLModifyCollectionBody) validateType(formats strfmt.Registry) error {
	if swag.IsZero(o.Type) { // not required
		return nil
	}

	// value enum
	if err := o.validateTypeEnum("info"+"."+"type", "body", *o.Type); err != nil {
		return err
	}

	return nil
}

// ContextValidate validate this cifs share ACL modify collection body based on the context it is used
func (o *CifsShareACLModifyCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateCifsShareACLResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateShare(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateSid(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateSvm(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateUnixID(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *CifsShareACLModifyCollectionBody) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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

func (o *CifsShareACLModifyCollectionBody) contextValidateCifsShareACLResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.CifsShareACLResponseInlineRecords); i++ {

		if o.CifsShareACLResponseInlineRecords[i] != nil {
			if err := o.CifsShareACLResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *CifsShareACLModifyCollectionBody) contextValidateShare(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "info"+"."+"share", "body", o.Share); err != nil {
		return err
	}

	return nil
}

func (o *CifsShareACLModifyCollectionBody) contextValidateSid(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "info"+"."+"sid", "body", o.Sid); err != nil {
		return err
	}

	return nil
}

func (o *CifsShareACLModifyCollectionBody) contextValidateSvm(ctx context.Context, formats strfmt.Registry) error {

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

func (o *CifsShareACLModifyCollectionBody) contextValidateUnixID(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "info"+"."+"unix_id", "body", o.UnixID); err != nil {
		return err
	}

	return nil
}

// MarshalBinary interface implementation
func (o *CifsShareACLModifyCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *CifsShareACLModifyCollectionBody) UnmarshalBinary(b []byte) error {
	var res CifsShareACLModifyCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
CifsShareACLInlineLinks cifs share acl inline links
swagger:model cifs_share_acl_inline__links
*/
type CifsShareACLInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this cifs share acl inline links
func (o *CifsShareACLInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *CifsShareACLInlineLinks) validateSelf(formats strfmt.Registry) error {
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

// ContextValidate validate this cifs share acl inline links based on the context it is used
func (o *CifsShareACLInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *CifsShareACLInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

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
func (o *CifsShareACLInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *CifsShareACLInlineLinks) UnmarshalBinary(b []byte) error {
	var res CifsShareACLInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
CifsShareACLInlineSvm SVM, applies only to SVM-scoped objects.
swagger:model cifs_share_acl_inline_svm
*/
type CifsShareACLInlineSvm struct {

	// links
	Links *models.CifsShareACLInlineSvmInlineLinks `json:"_links,omitempty"`

	// The name of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: svm1
	Name *string `json:"name,omitempty"`

	// The unique identifier of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: 02c9e252-41be-11e9-81d5-00a0986138f7
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this cifs share acl inline svm
func (o *CifsShareACLInlineSvm) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *CifsShareACLInlineSvm) validateLinks(formats strfmt.Registry) error {
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

// ContextValidate validate this cifs share acl inline svm based on the context it is used
func (o *CifsShareACLInlineSvm) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *CifsShareACLInlineSvm) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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
func (o *CifsShareACLInlineSvm) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *CifsShareACLInlineSvm) UnmarshalBinary(b []byte) error {
	var res CifsShareACLInlineSvm
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
CifsShareACLInlineSvmInlineLinks cifs share acl inline svm inline links
swagger:model cifs_share_acl_inline_svm_inline__links
*/
type CifsShareACLInlineSvmInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this cifs share acl inline svm inline links
func (o *CifsShareACLInlineSvmInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *CifsShareACLInlineSvmInlineLinks) validateSelf(formats strfmt.Registry) error {
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

// ContextValidate validate this cifs share acl inline svm inline links based on the context it is used
func (o *CifsShareACLInlineSvmInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *CifsShareACLInlineSvmInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

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
func (o *CifsShareACLInlineSvmInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *CifsShareACLInlineSvmInlineLinks) UnmarshalBinary(b []byte) error {
	var res CifsShareACLInlineSvmInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}