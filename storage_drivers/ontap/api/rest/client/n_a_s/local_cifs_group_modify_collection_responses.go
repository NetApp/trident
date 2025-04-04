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

// LocalCifsGroupModifyCollectionReader is a Reader for the LocalCifsGroupModifyCollection structure.
type LocalCifsGroupModifyCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *LocalCifsGroupModifyCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewLocalCifsGroupModifyCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewLocalCifsGroupModifyCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewLocalCifsGroupModifyCollectionOK creates a LocalCifsGroupModifyCollectionOK with default headers values
func NewLocalCifsGroupModifyCollectionOK() *LocalCifsGroupModifyCollectionOK {
	return &LocalCifsGroupModifyCollectionOK{}
}

/*
LocalCifsGroupModifyCollectionOK describes a response with status code 200, with default header values.

OK
*/
type LocalCifsGroupModifyCollectionOK struct {
}

// IsSuccess returns true when this local cifs group modify collection o k response has a 2xx status code
func (o *LocalCifsGroupModifyCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this local cifs group modify collection o k response has a 3xx status code
func (o *LocalCifsGroupModifyCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this local cifs group modify collection o k response has a 4xx status code
func (o *LocalCifsGroupModifyCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this local cifs group modify collection o k response has a 5xx status code
func (o *LocalCifsGroupModifyCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this local cifs group modify collection o k response a status code equal to that given
func (o *LocalCifsGroupModifyCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the local cifs group modify collection o k response
func (o *LocalCifsGroupModifyCollectionOK) Code() int {
	return 200
}

func (o *LocalCifsGroupModifyCollectionOK) Error() string {
	return fmt.Sprintf("[PATCH /protocols/cifs/local-groups][%d] localCifsGroupModifyCollectionOK", 200)
}

func (o *LocalCifsGroupModifyCollectionOK) String() string {
	return fmt.Sprintf("[PATCH /protocols/cifs/local-groups][%d] localCifsGroupModifyCollectionOK", 200)
}

func (o *LocalCifsGroupModifyCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewLocalCifsGroupModifyCollectionDefault creates a LocalCifsGroupModifyCollectionDefault with default headers values
func NewLocalCifsGroupModifyCollectionDefault(code int) *LocalCifsGroupModifyCollectionDefault {
	return &LocalCifsGroupModifyCollectionDefault{
		_statusCode: code,
	}
}

/*
	LocalCifsGroupModifyCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 655661     | The group name and description should not exceed 256 characters. |
| 655668     | The specified group name contains illegal characters. |
| 655675     | The local domain name specified in the group name does not exist. |
| 655682     | The group name cannot be blank. |
| 655712     | To rename an existing group, the local domain specified in name must match the local domain of the group to be renamed. |
| 655713     | Failed to rename a group. The error code returned details the failure along with the reason for the failure. Take corrective actions as per the specified reason. |
*/
type LocalCifsGroupModifyCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this local cifs group modify collection default response has a 2xx status code
func (o *LocalCifsGroupModifyCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this local cifs group modify collection default response has a 3xx status code
func (o *LocalCifsGroupModifyCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this local cifs group modify collection default response has a 4xx status code
func (o *LocalCifsGroupModifyCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this local cifs group modify collection default response has a 5xx status code
func (o *LocalCifsGroupModifyCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this local cifs group modify collection default response a status code equal to that given
func (o *LocalCifsGroupModifyCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the local cifs group modify collection default response
func (o *LocalCifsGroupModifyCollectionDefault) Code() int {
	return o._statusCode
}

func (o *LocalCifsGroupModifyCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /protocols/cifs/local-groups][%d] local_cifs_group_modify_collection default %s", o._statusCode, payload)
}

func (o *LocalCifsGroupModifyCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /protocols/cifs/local-groups][%d] local_cifs_group_modify_collection default %s", o._statusCode, payload)
}

func (o *LocalCifsGroupModifyCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *LocalCifsGroupModifyCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
LocalCifsGroupModifyCollectionBody local cifs group modify collection body
swagger:model LocalCifsGroupModifyCollectionBody
*/
type LocalCifsGroupModifyCollectionBody struct {

	// links
	Links *models.LocalCifsGroupInlineLinks `json:"_links,omitempty"`

	// Description for the local group.
	//
	// Example: This is a local group
	// Max Length: 256
	Description *string `json:"description,omitempty"`

	// local cifs group inline members
	// Read Only: true
	LocalCifsGroupInlineMembers []*models.LocalCifsGroupInlineMembersInlineArrayItem `json:"members,omitempty"`

	// local cifs group response inline records
	LocalCifsGroupResponseInlineRecords []*models.LocalCifsGroup `json:"records,omitempty"`

	// Local group name. The maximum supported length of a group name is 256 characters.
	//
	// Example: SMB_SERVER01\\group
	// Min Length: 1
	Name *string `json:"name,omitempty"`

	// The security ID of the local group which uniquely identifies the group. The group SID is automatically generated in POST and it is retrieved using the GET method.
	//
	// Example: S-1-5-21-256008430-3394229847-3930036330-1001
	// Read Only: true
	Sid *string `json:"sid,omitempty"`

	// svm
	Svm *models.LocalCifsGroupInlineSvm `json:"svm,omitempty"`
}

// Validate validates this local cifs group modify collection body
func (o *LocalCifsGroupModifyCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateDescription(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateLocalCifsGroupInlineMembers(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateLocalCifsGroupResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateName(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateSvm(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LocalCifsGroupModifyCollectionBody) validateLinks(formats strfmt.Registry) error {
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

func (o *LocalCifsGroupModifyCollectionBody) validateDescription(formats strfmt.Registry) error {
	if swag.IsZero(o.Description) { // not required
		return nil
	}

	if err := validate.MaxLength("info"+"."+"description", "body", *o.Description, 256); err != nil {
		return err
	}

	return nil
}

func (o *LocalCifsGroupModifyCollectionBody) validateLocalCifsGroupInlineMembers(formats strfmt.Registry) error {
	if swag.IsZero(o.LocalCifsGroupInlineMembers) { // not required
		return nil
	}

	for i := 0; i < len(o.LocalCifsGroupInlineMembers); i++ {
		if swag.IsZero(o.LocalCifsGroupInlineMembers[i]) { // not required
			continue
		}

		if o.LocalCifsGroupInlineMembers[i] != nil {
			if err := o.LocalCifsGroupInlineMembers[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "members" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *LocalCifsGroupModifyCollectionBody) validateLocalCifsGroupResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.LocalCifsGroupResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.LocalCifsGroupResponseInlineRecords); i++ {
		if swag.IsZero(o.LocalCifsGroupResponseInlineRecords[i]) { // not required
			continue
		}

		if o.LocalCifsGroupResponseInlineRecords[i] != nil {
			if err := o.LocalCifsGroupResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *LocalCifsGroupModifyCollectionBody) validateName(formats strfmt.Registry) error {
	if swag.IsZero(o.Name) { // not required
		return nil
	}

	if err := validate.MinLength("info"+"."+"name", "body", *o.Name, 1); err != nil {
		return err
	}

	return nil
}

func (o *LocalCifsGroupModifyCollectionBody) validateSvm(formats strfmt.Registry) error {
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

// ContextValidate validate this local cifs group modify collection body based on the context it is used
func (o *LocalCifsGroupModifyCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateLocalCifsGroupInlineMembers(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateLocalCifsGroupResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateSid(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateSvm(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LocalCifsGroupModifyCollectionBody) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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

func (o *LocalCifsGroupModifyCollectionBody) contextValidateLocalCifsGroupInlineMembers(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "info"+"."+"members", "body", []*models.LocalCifsGroupInlineMembersInlineArrayItem(o.LocalCifsGroupInlineMembers)); err != nil {
		return err
	}

	for i := 0; i < len(o.LocalCifsGroupInlineMembers); i++ {

		if o.LocalCifsGroupInlineMembers[i] != nil {
			if err := o.LocalCifsGroupInlineMembers[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "members" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *LocalCifsGroupModifyCollectionBody) contextValidateLocalCifsGroupResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.LocalCifsGroupResponseInlineRecords); i++ {

		if o.LocalCifsGroupResponseInlineRecords[i] != nil {
			if err := o.LocalCifsGroupResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *LocalCifsGroupModifyCollectionBody) contextValidateSid(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "info"+"."+"sid", "body", o.Sid); err != nil {
		return err
	}

	return nil
}

func (o *LocalCifsGroupModifyCollectionBody) contextValidateSvm(ctx context.Context, formats strfmt.Registry) error {

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

// MarshalBinary interface implementation
func (o *LocalCifsGroupModifyCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *LocalCifsGroupModifyCollectionBody) UnmarshalBinary(b []byte) error {
	var res LocalCifsGroupModifyCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
LocalCifsGroupInlineLinks local cifs group inline links
swagger:model local_cifs_group_inline__links
*/
type LocalCifsGroupInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this local cifs group inline links
func (o *LocalCifsGroupInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LocalCifsGroupInlineLinks) validateSelf(formats strfmt.Registry) error {
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

// ContextValidate validate this local cifs group inline links based on the context it is used
func (o *LocalCifsGroupInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LocalCifsGroupInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

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
func (o *LocalCifsGroupInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *LocalCifsGroupInlineLinks) UnmarshalBinary(b []byte) error {
	var res LocalCifsGroupInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
LocalCifsGroupInlineMembersInlineArrayItem local cifs group inline members inline array item
swagger:model local_cifs_group_inline_members_inline_array_item
*/
type LocalCifsGroupInlineMembersInlineArrayItem struct {

	// Local user, Active Directory user, or Active Directory group which is a member of the specified local group.
	//
	Name *string `json:"name,omitempty"`
}

// Validate validates this local cifs group inline members inline array item
func (o *LocalCifsGroupInlineMembersInlineArrayItem) Validate(formats strfmt.Registry) error {
	return nil
}

// ContextValidate validates this local cifs group inline members inline array item based on context it is used
func (o *LocalCifsGroupInlineMembersInlineArrayItem) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	return nil
}

// MarshalBinary interface implementation
func (o *LocalCifsGroupInlineMembersInlineArrayItem) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *LocalCifsGroupInlineMembersInlineArrayItem) UnmarshalBinary(b []byte) error {
	var res LocalCifsGroupInlineMembersInlineArrayItem
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
LocalCifsGroupInlineSvm SVM, applies only to SVM-scoped objects.
swagger:model local_cifs_group_inline_svm
*/
type LocalCifsGroupInlineSvm struct {

	// links
	Links *models.LocalCifsGroupInlineSvmInlineLinks `json:"_links,omitempty"`

	// The name of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: svm1
	Name *string `json:"name,omitempty"`

	// The unique identifier of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: 02c9e252-41be-11e9-81d5-00a0986138f7
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this local cifs group inline svm
func (o *LocalCifsGroupInlineSvm) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LocalCifsGroupInlineSvm) validateLinks(formats strfmt.Registry) error {
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

// ContextValidate validate this local cifs group inline svm based on the context it is used
func (o *LocalCifsGroupInlineSvm) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LocalCifsGroupInlineSvm) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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
func (o *LocalCifsGroupInlineSvm) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *LocalCifsGroupInlineSvm) UnmarshalBinary(b []byte) error {
	var res LocalCifsGroupInlineSvm
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
LocalCifsGroupInlineSvmInlineLinks local cifs group inline svm inline links
swagger:model local_cifs_group_inline_svm_inline__links
*/
type LocalCifsGroupInlineSvmInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this local cifs group inline svm inline links
func (o *LocalCifsGroupInlineSvmInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LocalCifsGroupInlineSvmInlineLinks) validateSelf(formats strfmt.Registry) error {
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

// ContextValidate validate this local cifs group inline svm inline links based on the context it is used
func (o *LocalCifsGroupInlineSvmInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LocalCifsGroupInlineSvmInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

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
func (o *LocalCifsGroupInlineSvmInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *LocalCifsGroupInlineSvmInlineLinks) UnmarshalBinary(b []byte) error {
	var res LocalCifsGroupInlineSvmInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
