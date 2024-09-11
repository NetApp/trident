// Code generated by go-swagger; DO NOT EDIT.

package security

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

// PublickeyModifyCollectionReader is a Reader for the PublickeyModifyCollection structure.
type PublickeyModifyCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *PublickeyModifyCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewPublickeyModifyCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewPublickeyModifyCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewPublickeyModifyCollectionOK creates a PublickeyModifyCollectionOK with default headers values
func NewPublickeyModifyCollectionOK() *PublickeyModifyCollectionOK {
	return &PublickeyModifyCollectionOK{}
}

/*
PublickeyModifyCollectionOK describes a response with status code 200, with default header values.

OK
*/
type PublickeyModifyCollectionOK struct {
}

// IsSuccess returns true when this publickey modify collection o k response has a 2xx status code
func (o *PublickeyModifyCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this publickey modify collection o k response has a 3xx status code
func (o *PublickeyModifyCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this publickey modify collection o k response has a 4xx status code
func (o *PublickeyModifyCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this publickey modify collection o k response has a 5xx status code
func (o *PublickeyModifyCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this publickey modify collection o k response a status code equal to that given
func (o *PublickeyModifyCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the publickey modify collection o k response
func (o *PublickeyModifyCollectionOK) Code() int {
	return 200
}

func (o *PublickeyModifyCollectionOK) Error() string {
	return fmt.Sprintf("[PATCH /security/authentication/publickeys][%d] publickeyModifyCollectionOK", 200)
}

func (o *PublickeyModifyCollectionOK) String() string {
	return fmt.Sprintf("[PATCH /security/authentication/publickeys][%d] publickeyModifyCollectionOK", 200)
}

func (o *PublickeyModifyCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPublickeyModifyCollectionDefault creates a PublickeyModifyCollectionDefault with default headers values
func NewPublickeyModifyCollectionDefault(code int) *PublickeyModifyCollectionDefault {
	return &PublickeyModifyCollectionDefault{
		_statusCode: code,
	}
}

/*
	PublickeyModifyCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 5832707 | Failed to generate fingerprint for the public key. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type PublickeyModifyCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this publickey modify collection default response has a 2xx status code
func (o *PublickeyModifyCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this publickey modify collection default response has a 3xx status code
func (o *PublickeyModifyCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this publickey modify collection default response has a 4xx status code
func (o *PublickeyModifyCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this publickey modify collection default response has a 5xx status code
func (o *PublickeyModifyCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this publickey modify collection default response a status code equal to that given
func (o *PublickeyModifyCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the publickey modify collection default response
func (o *PublickeyModifyCollectionDefault) Code() int {
	return o._statusCode
}

func (o *PublickeyModifyCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /security/authentication/publickeys][%d] publickey_modify_collection default %s", o._statusCode, payload)
}

func (o *PublickeyModifyCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /security/authentication/publickeys][%d] publickey_modify_collection default %s", o._statusCode, payload)
}

func (o *PublickeyModifyCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *PublickeyModifyCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
PublickeyModifyCollectionBody publickey modify collection body
swagger:model PublickeyModifyCollectionBody
*/
type PublickeyModifyCollectionBody struct {

	// links
	Links *models.PublickeyInlineLinks `json:"_links,omitempty"`

	// account
	Account *models.AccountReference `json:"account,omitempty"`

	// Optional certificate for the public key.
	Certificate *string `json:"certificate,omitempty"`

	// The details present in the certificate (READONLY).
	// Read Only: true
	CertificateDetails *string `json:"certificate_details,omitempty"`

	// The expiration details of the certificate (READONLY).
	// Read Only: true
	CertificateExpired *string `json:"certificate_expired,omitempty"`

	// The revocation details of the certificate (READONLY).
	// Read Only: true
	CertificateRevoked *string `json:"certificate_revoked,omitempty"`

	// Optional comment for the public key.
	Comment *string `json:"comment,omitempty"`

	// Index number for the public key (where there are multiple keys for the same account).
	// Maximum: 99
	// Minimum: 0
	Index *int64 `json:"index,omitempty"`

	// The obfuscated fingerprint for the public key (READONLY).
	// Read Only: true
	ObfuscatedFingerprint *string `json:"obfuscated_fingerprint,omitempty"`

	// owner
	Owner *models.PublickeyInlineOwner `json:"owner,omitempty"`

	// The public key
	PublicKey *string `json:"public_key,omitempty"`

	// publickey response inline records
	PublickeyResponseInlineRecords []*models.Publickey `json:"records,omitempty"`

	// Scope of the entity. Set to "cluster" for cluster owned objects and to "svm" for SVM owned objects.
	// Read Only: true
	// Enum: ["cluster","svm"]
	Scope *string `json:"scope,omitempty"`

	// The SHA fingerprint for the public key (READONLY).
	// Read Only: true
	ShaFingerprint *string `json:"sha_fingerprint,omitempty"`
}

// Validate validates this publickey modify collection body
func (o *PublickeyModifyCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateAccount(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateIndex(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateOwner(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validatePublickeyResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateScope(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *PublickeyModifyCollectionBody) validateLinks(formats strfmt.Registry) error {
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

func (o *PublickeyModifyCollectionBody) validateAccount(formats strfmt.Registry) error {
	if swag.IsZero(o.Account) { // not required
		return nil
	}

	if o.Account != nil {
		if err := o.Account.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "account")
			}
			return err
		}
	}

	return nil
}

func (o *PublickeyModifyCollectionBody) validateIndex(formats strfmt.Registry) error {
	if swag.IsZero(o.Index) { // not required
		return nil
	}

	if err := validate.MinimumInt("info"+"."+"index", "body", *o.Index, 0, false); err != nil {
		return err
	}

	if err := validate.MaximumInt("info"+"."+"index", "body", *o.Index, 99, false); err != nil {
		return err
	}

	return nil
}

func (o *PublickeyModifyCollectionBody) validateOwner(formats strfmt.Registry) error {
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

func (o *PublickeyModifyCollectionBody) validatePublickeyResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.PublickeyResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.PublickeyResponseInlineRecords); i++ {
		if swag.IsZero(o.PublickeyResponseInlineRecords[i]) { // not required
			continue
		}

		if o.PublickeyResponseInlineRecords[i] != nil {
			if err := o.PublickeyResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

var publickeyModifyCollectionBodyTypeScopePropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["cluster","svm"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		publickeyModifyCollectionBodyTypeScopePropEnum = append(publickeyModifyCollectionBodyTypeScopePropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// PublickeyModifyCollectionBody
	// PublickeyModifyCollectionBody
	// scope
	// Scope
	// cluster
	// END DEBUGGING
	// PublickeyModifyCollectionBodyScopeCluster captures enum value "cluster"
	PublickeyModifyCollectionBodyScopeCluster string = "cluster"

	// BEGIN DEBUGGING
	// PublickeyModifyCollectionBody
	// PublickeyModifyCollectionBody
	// scope
	// Scope
	// svm
	// END DEBUGGING
	// PublickeyModifyCollectionBodyScopeSvm captures enum value "svm"
	PublickeyModifyCollectionBodyScopeSvm string = "svm"
)

// prop value enum
func (o *PublickeyModifyCollectionBody) validateScopeEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, publickeyModifyCollectionBodyTypeScopePropEnum, true); err != nil {
		return err
	}
	return nil
}

func (o *PublickeyModifyCollectionBody) validateScope(formats strfmt.Registry) error {
	if swag.IsZero(o.Scope) { // not required
		return nil
	}

	// value enum
	if err := o.validateScopeEnum("info"+"."+"scope", "body", *o.Scope); err != nil {
		return err
	}

	return nil
}

// ContextValidate validate this publickey modify collection body based on the context it is used
func (o *PublickeyModifyCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateAccount(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateCertificateDetails(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateCertificateExpired(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateCertificateRevoked(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateObfuscatedFingerprint(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateOwner(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidatePublickeyResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateScope(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateShaFingerprint(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *PublickeyModifyCollectionBody) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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

func (o *PublickeyModifyCollectionBody) contextValidateAccount(ctx context.Context, formats strfmt.Registry) error {

	if o.Account != nil {
		if err := o.Account.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "account")
			}
			return err
		}
	}

	return nil
}

func (o *PublickeyModifyCollectionBody) contextValidateCertificateDetails(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "info"+"."+"certificate_details", "body", o.CertificateDetails); err != nil {
		return err
	}

	return nil
}

func (o *PublickeyModifyCollectionBody) contextValidateCertificateExpired(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "info"+"."+"certificate_expired", "body", o.CertificateExpired); err != nil {
		return err
	}

	return nil
}

func (o *PublickeyModifyCollectionBody) contextValidateCertificateRevoked(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "info"+"."+"certificate_revoked", "body", o.CertificateRevoked); err != nil {
		return err
	}

	return nil
}

func (o *PublickeyModifyCollectionBody) contextValidateObfuscatedFingerprint(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "info"+"."+"obfuscated_fingerprint", "body", o.ObfuscatedFingerprint); err != nil {
		return err
	}

	return nil
}

func (o *PublickeyModifyCollectionBody) contextValidateOwner(ctx context.Context, formats strfmt.Registry) error {

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

func (o *PublickeyModifyCollectionBody) contextValidatePublickeyResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.PublickeyResponseInlineRecords); i++ {

		if o.PublickeyResponseInlineRecords[i] != nil {
			if err := o.PublickeyResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *PublickeyModifyCollectionBody) contextValidateScope(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "info"+"."+"scope", "body", o.Scope); err != nil {
		return err
	}

	return nil
}

func (o *PublickeyModifyCollectionBody) contextValidateShaFingerprint(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "info"+"."+"sha_fingerprint", "body", o.ShaFingerprint); err != nil {
		return err
	}

	return nil
}

// MarshalBinary interface implementation
func (o *PublickeyModifyCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *PublickeyModifyCollectionBody) UnmarshalBinary(b []byte) error {
	var res PublickeyModifyCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
PublickeyInlineLinks publickey inline links
swagger:model publickey_inline__links
*/
type PublickeyInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this publickey inline links
func (o *PublickeyInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *PublickeyInlineLinks) validateSelf(formats strfmt.Registry) error {
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

// ContextValidate validate this publickey inline links based on the context it is used
func (o *PublickeyInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *PublickeyInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

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
func (o *PublickeyInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *PublickeyInlineLinks) UnmarshalBinary(b []byte) error {
	var res PublickeyInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
PublickeyInlineOwner Owner name and UUID that uniquely identifies the public key.
swagger:model publickey_inline_owner
*/
type PublickeyInlineOwner struct {

	// links
	Links *models.PublickeyInlineOwnerInlineLinks `json:"_links,omitempty"`

	// The name of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: svm1
	Name *string `json:"name,omitempty"`

	// The unique identifier of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: 02c9e252-41be-11e9-81d5-00a0986138f7
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this publickey inline owner
func (o *PublickeyInlineOwner) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *PublickeyInlineOwner) validateLinks(formats strfmt.Registry) error {
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

// ContextValidate validate this publickey inline owner based on the context it is used
func (o *PublickeyInlineOwner) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *PublickeyInlineOwner) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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
func (o *PublickeyInlineOwner) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *PublickeyInlineOwner) UnmarshalBinary(b []byte) error {
	var res PublickeyInlineOwner
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
PublickeyInlineOwnerInlineLinks publickey inline owner inline links
swagger:model publickey_inline_owner_inline__links
*/
type PublickeyInlineOwnerInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this publickey inline owner inline links
func (o *PublickeyInlineOwnerInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *PublickeyInlineOwnerInlineLinks) validateSelf(formats strfmt.Registry) error {
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

// ContextValidate validate this publickey inline owner inline links based on the context it is used
func (o *PublickeyInlineOwnerInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *PublickeyInlineOwnerInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

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
func (o *PublickeyInlineOwnerInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *PublickeyInlineOwnerInlineLinks) UnmarshalBinary(b []byte) error {
	var res PublickeyInlineOwnerInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}