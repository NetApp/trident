// Code generated by go-swagger; DO NOT EDIT.

package ndmp

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

// NdmpSvmModifyCollectionReader is a Reader for the NdmpSvmModifyCollection structure.
type NdmpSvmModifyCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *NdmpSvmModifyCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewNdmpSvmModifyCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewNdmpSvmModifyCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewNdmpSvmModifyCollectionOK creates a NdmpSvmModifyCollectionOK with default headers values
func NewNdmpSvmModifyCollectionOK() *NdmpSvmModifyCollectionOK {
	return &NdmpSvmModifyCollectionOK{}
}

/*
NdmpSvmModifyCollectionOK describes a response with status code 200, with default header values.

OK
*/
type NdmpSvmModifyCollectionOK struct {
	Payload *models.NdmpSvm
}

// IsSuccess returns true when this ndmp svm modify collection o k response has a 2xx status code
func (o *NdmpSvmModifyCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this ndmp svm modify collection o k response has a 3xx status code
func (o *NdmpSvmModifyCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this ndmp svm modify collection o k response has a 4xx status code
func (o *NdmpSvmModifyCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this ndmp svm modify collection o k response has a 5xx status code
func (o *NdmpSvmModifyCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this ndmp svm modify collection o k response a status code equal to that given
func (o *NdmpSvmModifyCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the ndmp svm modify collection o k response
func (o *NdmpSvmModifyCollectionOK) Code() int {
	return 200
}

func (o *NdmpSvmModifyCollectionOK) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /protocols/ndmp/svms][%d] ndmpSvmModifyCollectionOK %s", 200, payload)
}

func (o *NdmpSvmModifyCollectionOK) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /protocols/ndmp/svms][%d] ndmpSvmModifyCollectionOK %s", 200, payload)
}

func (o *NdmpSvmModifyCollectionOK) GetPayload() *models.NdmpSvm {
	return o.Payload
}

func (o *NdmpSvmModifyCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.NdmpSvm)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewNdmpSvmModifyCollectionDefault creates a NdmpSvmModifyCollectionDefault with default headers values
func NewNdmpSvmModifyCollectionDefault(code int) *NdmpSvmModifyCollectionDefault {
	return &NdmpSvmModifyCollectionDefault{
		_statusCode: code,
	}
}

/*
	NdmpSvmModifyCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response codes

| Error code  |  Description |
|-------------|--------------|
| 65601536    | The operation is not supported because NDMP SVM-aware mode is disabled.|
| 65601551    | Authentication type \"plaintext_sso\" cannot be combined with other authentication types.|
*/
type NdmpSvmModifyCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this ndmp svm modify collection default response has a 2xx status code
func (o *NdmpSvmModifyCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this ndmp svm modify collection default response has a 3xx status code
func (o *NdmpSvmModifyCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this ndmp svm modify collection default response has a 4xx status code
func (o *NdmpSvmModifyCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this ndmp svm modify collection default response has a 5xx status code
func (o *NdmpSvmModifyCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this ndmp svm modify collection default response a status code equal to that given
func (o *NdmpSvmModifyCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the ndmp svm modify collection default response
func (o *NdmpSvmModifyCollectionDefault) Code() int {
	return o._statusCode
}

func (o *NdmpSvmModifyCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /protocols/ndmp/svms][%d] ndmp_svm_modify_collection default %s", o._statusCode, payload)
}

func (o *NdmpSvmModifyCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /protocols/ndmp/svms][%d] ndmp_svm_modify_collection default %s", o._statusCode, payload)
}

func (o *NdmpSvmModifyCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *NdmpSvmModifyCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
NdmpSvmModifyCollectionBody ndmp svm modify collection body
swagger:model NdmpSvmModifyCollectionBody
*/
type NdmpSvmModifyCollectionBody struct {

	// links
	Links *models.NdmpSvmInlineLinks `json:"_links,omitempty"`

	// Is the NDMP service enabled on the SVM?
	// Example: true
	Enabled *bool `json:"enabled,omitempty"`

	// NDMP authentication types.
	// Example: ["plaintext","challenge"]
	NdmpSvmInlineAuthenticationTypes []*models.NdmpAuthType `json:"authentication_types,omitempty"`

	// ndmp svm response inline records
	NdmpSvmResponseInlineRecords []*models.NdmpSvm `json:"records,omitempty"`

	// svm
	Svm *models.NdmpSvmInlineSvm `json:"svm,omitempty"`
}

// Validate validates this ndmp svm modify collection body
func (o *NdmpSvmModifyCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateNdmpSvmInlineAuthenticationTypes(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateNdmpSvmResponseInlineRecords(formats); err != nil {
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

func (o *NdmpSvmModifyCollectionBody) validateLinks(formats strfmt.Registry) error {
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

func (o *NdmpSvmModifyCollectionBody) validateNdmpSvmInlineAuthenticationTypes(formats strfmt.Registry) error {
	if swag.IsZero(o.NdmpSvmInlineAuthenticationTypes) { // not required
		return nil
	}

	for i := 0; i < len(o.NdmpSvmInlineAuthenticationTypes); i++ {
		if swag.IsZero(o.NdmpSvmInlineAuthenticationTypes[i]) { // not required
			continue
		}

		if o.NdmpSvmInlineAuthenticationTypes[i] != nil {
			if err := o.NdmpSvmInlineAuthenticationTypes[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "authentication_types" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *NdmpSvmModifyCollectionBody) validateNdmpSvmResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.NdmpSvmResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.NdmpSvmResponseInlineRecords); i++ {
		if swag.IsZero(o.NdmpSvmResponseInlineRecords[i]) { // not required
			continue
		}

		if o.NdmpSvmResponseInlineRecords[i] != nil {
			if err := o.NdmpSvmResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *NdmpSvmModifyCollectionBody) validateSvm(formats strfmt.Registry) error {
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

// ContextValidate validate this ndmp svm modify collection body based on the context it is used
func (o *NdmpSvmModifyCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateNdmpSvmInlineAuthenticationTypes(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateNdmpSvmResponseInlineRecords(ctx, formats); err != nil {
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

func (o *NdmpSvmModifyCollectionBody) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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

func (o *NdmpSvmModifyCollectionBody) contextValidateNdmpSvmInlineAuthenticationTypes(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.NdmpSvmInlineAuthenticationTypes); i++ {

		if o.NdmpSvmInlineAuthenticationTypes[i] != nil {
			if err := o.NdmpSvmInlineAuthenticationTypes[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "authentication_types" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *NdmpSvmModifyCollectionBody) contextValidateNdmpSvmResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.NdmpSvmResponseInlineRecords); i++ {

		if o.NdmpSvmResponseInlineRecords[i] != nil {
			if err := o.NdmpSvmResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *NdmpSvmModifyCollectionBody) contextValidateSvm(ctx context.Context, formats strfmt.Registry) error {

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
func (o *NdmpSvmModifyCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *NdmpSvmModifyCollectionBody) UnmarshalBinary(b []byte) error {
	var res NdmpSvmModifyCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
NdmpSvmInlineLinks ndmp svm inline links
swagger:model ndmp_svm_inline__links
*/
type NdmpSvmInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this ndmp svm inline links
func (o *NdmpSvmInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NdmpSvmInlineLinks) validateSelf(formats strfmt.Registry) error {
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

// ContextValidate validate this ndmp svm inline links based on the context it is used
func (o *NdmpSvmInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NdmpSvmInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

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
func (o *NdmpSvmInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *NdmpSvmInlineLinks) UnmarshalBinary(b []byte) error {
	var res NdmpSvmInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
NdmpSvmInlineSvm SVM, applies only to SVM-scoped objects.
swagger:model ndmp_svm_inline_svm
*/
type NdmpSvmInlineSvm struct {

	// links
	Links *models.NdmpSvmInlineSvmInlineLinks `json:"_links,omitempty"`

	// The name of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: svm1
	Name *string `json:"name,omitempty"`

	// The unique identifier of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: 02c9e252-41be-11e9-81d5-00a0986138f7
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this ndmp svm inline svm
func (o *NdmpSvmInlineSvm) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NdmpSvmInlineSvm) validateLinks(formats strfmt.Registry) error {
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

// ContextValidate validate this ndmp svm inline svm based on the context it is used
func (o *NdmpSvmInlineSvm) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NdmpSvmInlineSvm) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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
func (o *NdmpSvmInlineSvm) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *NdmpSvmInlineSvm) UnmarshalBinary(b []byte) error {
	var res NdmpSvmInlineSvm
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
NdmpSvmInlineSvmInlineLinks ndmp svm inline svm inline links
swagger:model ndmp_svm_inline_svm_inline__links
*/
type NdmpSvmInlineSvmInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this ndmp svm inline svm inline links
func (o *NdmpSvmInlineSvmInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NdmpSvmInlineSvmInlineLinks) validateSelf(formats strfmt.Registry) error {
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

// ContextValidate validate this ndmp svm inline svm inline links based on the context it is used
func (o *NdmpSvmInlineSvmInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NdmpSvmInlineSvmInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

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
func (o *NdmpSvmInlineSvmInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *NdmpSvmInlineSvmInlineLinks) UnmarshalBinary(b []byte) error {
	var res NdmpSvmInlineSvmInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
