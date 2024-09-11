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
	"github.com/go-openapi/validate"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// LdapSchemaModifyCollectionReader is a Reader for the LdapSchemaModifyCollection structure.
type LdapSchemaModifyCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *LdapSchemaModifyCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewLdapSchemaModifyCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewLdapSchemaModifyCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewLdapSchemaModifyCollectionOK creates a LdapSchemaModifyCollectionOK with default headers values
func NewLdapSchemaModifyCollectionOK() *LdapSchemaModifyCollectionOK {
	return &LdapSchemaModifyCollectionOK{}
}

/*
LdapSchemaModifyCollectionOK describes a response with status code 200, with default header values.

OK
*/
type LdapSchemaModifyCollectionOK struct {
}

// IsSuccess returns true when this ldap schema modify collection o k response has a 2xx status code
func (o *LdapSchemaModifyCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this ldap schema modify collection o k response has a 3xx status code
func (o *LdapSchemaModifyCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this ldap schema modify collection o k response has a 4xx status code
func (o *LdapSchemaModifyCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this ldap schema modify collection o k response has a 5xx status code
func (o *LdapSchemaModifyCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this ldap schema modify collection o k response a status code equal to that given
func (o *LdapSchemaModifyCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the ldap schema modify collection o k response
func (o *LdapSchemaModifyCollectionOK) Code() int {
	return 200
}

func (o *LdapSchemaModifyCollectionOK) Error() string {
	return fmt.Sprintf("[PATCH /name-services/ldap-schemas][%d] ldapSchemaModifyCollectionOK", 200)
}

func (o *LdapSchemaModifyCollectionOK) String() string {
	return fmt.Sprintf("[PATCH /name-services/ldap-schemas][%d] ldapSchemaModifyCollectionOK", 200)
}

func (o *LdapSchemaModifyCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewLdapSchemaModifyCollectionDefault creates a LdapSchemaModifyCollectionDefault with default headers values
func NewLdapSchemaModifyCollectionDefault(code int) *LdapSchemaModifyCollectionDefault {
	return &LdapSchemaModifyCollectionDefault{
		_statusCode: code,
	}
}

/*
	LdapSchemaModifyCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 4915205    | The LDAP schema is a default schema and cannot be modified or deleted. |
| 4915217    | LDAP schema is owned by the admin SVM. |
| 4915223    | LDAP schema does not belong to the admin SVM. |
*/
type LdapSchemaModifyCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this ldap schema modify collection default response has a 2xx status code
func (o *LdapSchemaModifyCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this ldap schema modify collection default response has a 3xx status code
func (o *LdapSchemaModifyCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this ldap schema modify collection default response has a 4xx status code
func (o *LdapSchemaModifyCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this ldap schema modify collection default response has a 5xx status code
func (o *LdapSchemaModifyCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this ldap schema modify collection default response a status code equal to that given
func (o *LdapSchemaModifyCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the ldap schema modify collection default response
func (o *LdapSchemaModifyCollectionDefault) Code() int {
	return o._statusCode
}

func (o *LdapSchemaModifyCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /name-services/ldap-schemas][%d] ldap_schema_modify_collection default %s", o._statusCode, payload)
}

func (o *LdapSchemaModifyCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /name-services/ldap-schemas][%d] ldap_schema_modify_collection default %s", o._statusCode, payload)
}

func (o *LdapSchemaModifyCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *LdapSchemaModifyCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
LdapSchemaModifyCollectionBody ldap schema modify collection body
swagger:model LdapSchemaModifyCollectionBody
*/
type LdapSchemaModifyCollectionBody struct {

	// links
	Links *models.LdapSchemaInlineLinks `json:"_links,omitempty"`

	// Comment to associate with the schema.
	// Example: Schema based on Active Directory Services for UNIX (read-only).
	Comment *string `json:"comment,omitempty"`

	// A global schema that can be used by all the SVMs.
	// Example: true
	// Read Only: true
	GlobalSchema *bool `json:"global_schema,omitempty"`

	// ldap schema response inline records
	LdapSchemaResponseInlineRecords []*models.LdapSchema `json:"records,omitempty"`

	// The name of the schema being created, modified or deleted.
	// Example: AD-SFU-v1
	// Max Length: 32
	// Min Length: 1
	Name *string `json:"name,omitempty"`

	// name mapping
	NameMapping *models.LdapSchemaNameMapping `json:"name_mapping,omitempty"`

	// owner
	Owner *models.LdapSchemaInlineOwner `json:"owner,omitempty"`

	// rfc2307
	Rfc2307 *models.Rfc2307 `json:"rfc2307,omitempty"`

	// rfc2307bis
	Rfc2307bis *models.Rfc2307bis `json:"rfc2307bis,omitempty"`

	// Scope of the entity. Set to "cluster" for cluster owned objects and to "svm" for SVM owned objects.
	// Read Only: true
	// Enum: ["cluster","svm"]
	Scope *string `json:"scope,omitempty"`

	// template
	Template *models.LdapSchemaInlineTemplate `json:"template,omitempty"`
}

// Validate validates this ldap schema modify collection body
func (o *LdapSchemaModifyCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateLdapSchemaResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateName(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateNameMapping(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateOwner(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateRfc2307(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateRfc2307bis(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateScope(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateTemplate(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LdapSchemaModifyCollectionBody) validateLinks(formats strfmt.Registry) error {
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

func (o *LdapSchemaModifyCollectionBody) validateLdapSchemaResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.LdapSchemaResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.LdapSchemaResponseInlineRecords); i++ {
		if swag.IsZero(o.LdapSchemaResponseInlineRecords[i]) { // not required
			continue
		}

		if o.LdapSchemaResponseInlineRecords[i] != nil {
			if err := o.LdapSchemaResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *LdapSchemaModifyCollectionBody) validateName(formats strfmt.Registry) error {
	if swag.IsZero(o.Name) { // not required
		return nil
	}

	if err := validate.MinLength("info"+"."+"name", "body", *o.Name, 1); err != nil {
		return err
	}

	if err := validate.MaxLength("info"+"."+"name", "body", *o.Name, 32); err != nil {
		return err
	}

	return nil
}

func (o *LdapSchemaModifyCollectionBody) validateNameMapping(formats strfmt.Registry) error {
	if swag.IsZero(o.NameMapping) { // not required
		return nil
	}

	if o.NameMapping != nil {
		if err := o.NameMapping.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "name_mapping")
			}
			return err
		}
	}

	return nil
}

func (o *LdapSchemaModifyCollectionBody) validateOwner(formats strfmt.Registry) error {
	if swag.IsZero(o.Owner) { // not required
		return nil
	}

	if o.Owner != nil {
		if err := o.Owner.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "owner")
			}
			return err
		}
	}

	return nil
}

func (o *LdapSchemaModifyCollectionBody) validateRfc2307(formats strfmt.Registry) error {
	if swag.IsZero(o.Rfc2307) { // not required
		return nil
	}

	if o.Rfc2307 != nil {
		if err := o.Rfc2307.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "rfc2307")
			}
			return err
		}
	}

	return nil
}

func (o *LdapSchemaModifyCollectionBody) validateRfc2307bis(formats strfmt.Registry) error {
	if swag.IsZero(o.Rfc2307bis) { // not required
		return nil
	}

	if o.Rfc2307bis != nil {
		if err := o.Rfc2307bis.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "rfc2307bis")
			}
			return err
		}
	}

	return nil
}

var ldapSchemaModifyCollectionBodyTypeScopePropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["cluster","svm"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		ldapSchemaModifyCollectionBodyTypeScopePropEnum = append(ldapSchemaModifyCollectionBodyTypeScopePropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// LdapSchemaModifyCollectionBody
	// LdapSchemaModifyCollectionBody
	// scope
	// Scope
	// cluster
	// END DEBUGGING
	// LdapSchemaModifyCollectionBodyScopeCluster captures enum value "cluster"
	LdapSchemaModifyCollectionBodyScopeCluster string = "cluster"

	// BEGIN DEBUGGING
	// LdapSchemaModifyCollectionBody
	// LdapSchemaModifyCollectionBody
	// scope
	// Scope
	// svm
	// END DEBUGGING
	// LdapSchemaModifyCollectionBodyScopeSvm captures enum value "svm"
	LdapSchemaModifyCollectionBodyScopeSvm string = "svm"
)

// prop value enum
func (o *LdapSchemaModifyCollectionBody) validateScopeEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, ldapSchemaModifyCollectionBodyTypeScopePropEnum, true); err != nil {
		return err
	}
	return nil
}

func (o *LdapSchemaModifyCollectionBody) validateScope(formats strfmt.Registry) error {
	if swag.IsZero(o.Scope) { // not required
		return nil
	}

	// value enum
	if err := o.validateScopeEnum("info"+"."+"scope", "body", *o.Scope); err != nil {
		return err
	}

	return nil
}

func (o *LdapSchemaModifyCollectionBody) validateTemplate(formats strfmt.Registry) error {
	if swag.IsZero(o.Template) { // not required
		return nil
	}

	if o.Template != nil {
		if err := o.Template.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "template")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this ldap schema modify collection body based on the context it is used
func (o *LdapSchemaModifyCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateGlobalSchema(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateLdapSchemaResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateNameMapping(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateOwner(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateRfc2307(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateRfc2307bis(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateScope(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateTemplate(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LdapSchemaModifyCollectionBody) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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

func (o *LdapSchemaModifyCollectionBody) contextValidateGlobalSchema(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "info"+"."+"global_schema", "body", o.GlobalSchema); err != nil {
		return err
	}

	return nil
}

func (o *LdapSchemaModifyCollectionBody) contextValidateLdapSchemaResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.LdapSchemaResponseInlineRecords); i++ {

		if o.LdapSchemaResponseInlineRecords[i] != nil {
			if err := o.LdapSchemaResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *LdapSchemaModifyCollectionBody) contextValidateNameMapping(ctx context.Context, formats strfmt.Registry) error {

	if o.NameMapping != nil {
		if err := o.NameMapping.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "name_mapping")
			}
			return err
		}
	}

	return nil
}

func (o *LdapSchemaModifyCollectionBody) contextValidateOwner(ctx context.Context, formats strfmt.Registry) error {

	if o.Owner != nil {
		if err := o.Owner.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "owner")
			}
			return err
		}
	}

	return nil
}

func (o *LdapSchemaModifyCollectionBody) contextValidateRfc2307(ctx context.Context, formats strfmt.Registry) error {

	if o.Rfc2307 != nil {
		if err := o.Rfc2307.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "rfc2307")
			}
			return err
		}
	}

	return nil
}

func (o *LdapSchemaModifyCollectionBody) contextValidateRfc2307bis(ctx context.Context, formats strfmt.Registry) error {

	if o.Rfc2307bis != nil {
		if err := o.Rfc2307bis.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "rfc2307bis")
			}
			return err
		}
	}

	return nil
}

func (o *LdapSchemaModifyCollectionBody) contextValidateScope(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "info"+"."+"scope", "body", o.Scope); err != nil {
		return err
	}

	return nil
}

func (o *LdapSchemaModifyCollectionBody) contextValidateTemplate(ctx context.Context, formats strfmt.Registry) error {

	if o.Template != nil {
		if err := o.Template.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "template")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *LdapSchemaModifyCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *LdapSchemaModifyCollectionBody) UnmarshalBinary(b []byte) error {
	var res LdapSchemaModifyCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
LdapSchemaInlineLinks ldap schema inline links
swagger:model ldap_schema_inline__links
*/
type LdapSchemaInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this ldap schema inline links
func (o *LdapSchemaInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LdapSchemaInlineLinks) validateSelf(formats strfmt.Registry) error {
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

// ContextValidate validate this ldap schema inline links based on the context it is used
func (o *LdapSchemaInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LdapSchemaInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

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
func (o *LdapSchemaInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *LdapSchemaInlineLinks) UnmarshalBinary(b []byte) error {
	var res LdapSchemaInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
LdapSchemaInlineOwner SVM, applies only to SVM-scoped objects.
swagger:model ldap_schema_inline_owner
*/
type LdapSchemaInlineOwner struct {

	// links
	Links *models.LdapSchemaInlineOwnerInlineLinks `json:"_links,omitempty"`

	// The name of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: svm1
	Name *string `json:"name,omitempty"`

	// The unique identifier of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: 02c9e252-41be-11e9-81d5-00a0986138f7
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this ldap schema inline owner
func (o *LdapSchemaInlineOwner) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LdapSchemaInlineOwner) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(o.Links) { // not required
		return nil
	}

	if o.Links != nil {
		if err := o.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "owner" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this ldap schema inline owner based on the context it is used
func (o *LdapSchemaInlineOwner) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LdapSchemaInlineOwner) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if o.Links != nil {
		if err := o.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "owner" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *LdapSchemaInlineOwner) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *LdapSchemaInlineOwner) UnmarshalBinary(b []byte) error {
	var res LdapSchemaInlineOwner
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
LdapSchemaInlineOwnerInlineLinks ldap schema inline owner inline links
swagger:model ldap_schema_inline_owner_inline__links
*/
type LdapSchemaInlineOwnerInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this ldap schema inline owner inline links
func (o *LdapSchemaInlineOwnerInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LdapSchemaInlineOwnerInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(o.Self) { // not required
		return nil
	}

	if o.Self != nil {
		if err := o.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "owner" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this ldap schema inline owner inline links based on the context it is used
func (o *LdapSchemaInlineOwnerInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LdapSchemaInlineOwnerInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if o.Self != nil {
		if err := o.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "owner" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *LdapSchemaInlineOwnerInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *LdapSchemaInlineOwnerInlineLinks) UnmarshalBinary(b []byte) error {
	var res LdapSchemaInlineOwnerInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
LdapSchemaInlineTemplate The existing schema template you want to copy.
swagger:model ldap_schema_inline_template
*/
type LdapSchemaInlineTemplate struct {

	// links
	Links *models.LdapSchemaInlineTemplateInlineLinks `json:"_links,omitempty"`

	// The name of the schema.
	// Example: AD-SFU-v1
	// Max Length: 32
	// Min Length: 1
	Name *string `json:"name,omitempty"`
}

// Validate validates this ldap schema inline template
func (o *LdapSchemaInlineTemplate) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateName(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LdapSchemaInlineTemplate) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(o.Links) { // not required
		return nil
	}

	if o.Links != nil {
		if err := o.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "template" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

func (o *LdapSchemaInlineTemplate) validateName(formats strfmt.Registry) error {
	if swag.IsZero(o.Name) { // not required
		return nil
	}

	if err := validate.MinLength("info"+"."+"template"+"."+"name", "body", *o.Name, 1); err != nil {
		return err
	}

	if err := validate.MaxLength("info"+"."+"template"+"."+"name", "body", *o.Name, 32); err != nil {
		return err
	}

	return nil
}

// ContextValidate validate this ldap schema inline template based on the context it is used
func (o *LdapSchemaInlineTemplate) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LdapSchemaInlineTemplate) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if o.Links != nil {
		if err := o.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "template" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *LdapSchemaInlineTemplate) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *LdapSchemaInlineTemplate) UnmarshalBinary(b []byte) error {
	var res LdapSchemaInlineTemplate
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
LdapSchemaInlineTemplateInlineLinks ldap schema inline template inline links
swagger:model ldap_schema_inline_template_inline__links
*/
type LdapSchemaInlineTemplateInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this ldap schema inline template inline links
func (o *LdapSchemaInlineTemplateInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LdapSchemaInlineTemplateInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(o.Self) { // not required
		return nil
	}

	if o.Self != nil {
		if err := o.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "template" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this ldap schema inline template inline links based on the context it is used
func (o *LdapSchemaInlineTemplateInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *LdapSchemaInlineTemplateInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if o.Self != nil {
		if err := o.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "template" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *LdapSchemaInlineTemplateInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *LdapSchemaInlineTemplateInlineLinks) UnmarshalBinary(b []byte) error {
	var res LdapSchemaInlineTemplateInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}