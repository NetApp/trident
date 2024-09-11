// Code generated by go-swagger; DO NOT EDIT.

package networking

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

// HTTPProxyModifyCollectionReader is a Reader for the HTTPProxyModifyCollection structure.
type HTTPProxyModifyCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *HTTPProxyModifyCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewHTTPProxyModifyCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewHTTPProxyModifyCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewHTTPProxyModifyCollectionOK creates a HTTPProxyModifyCollectionOK with default headers values
func NewHTTPProxyModifyCollectionOK() *HTTPProxyModifyCollectionOK {
	return &HTTPProxyModifyCollectionOK{}
}

/*
HTTPProxyModifyCollectionOK describes a response with status code 200, with default header values.

OK
*/
type HTTPProxyModifyCollectionOK struct {
}

// IsSuccess returns true when this http proxy modify collection o k response has a 2xx status code
func (o *HTTPProxyModifyCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this http proxy modify collection o k response has a 3xx status code
func (o *HTTPProxyModifyCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this http proxy modify collection o k response has a 4xx status code
func (o *HTTPProxyModifyCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this http proxy modify collection o k response has a 5xx status code
func (o *HTTPProxyModifyCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this http proxy modify collection o k response a status code equal to that given
func (o *HTTPProxyModifyCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the http proxy modify collection o k response
func (o *HTTPProxyModifyCollectionOK) Code() int {
	return 200
}

func (o *HTTPProxyModifyCollectionOK) Error() string {
	return fmt.Sprintf("[PATCH /network/http-proxy][%d] httpProxyModifyCollectionOK", 200)
}

func (o *HTTPProxyModifyCollectionOK) String() string {
	return fmt.Sprintf("[PATCH /network/http-proxy][%d] httpProxyModifyCollectionOK", 200)
}

func (o *HTTPProxyModifyCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewHTTPProxyModifyCollectionDefault creates a HTTPProxyModifyCollectionDefault with default headers values
func NewHTTPProxyModifyCollectionDefault(code int) *HTTPProxyModifyCollectionDefault {
	return &HTTPProxyModifyCollectionDefault{
		_statusCode: code,
	}
}

/*
	HTTPProxyModifyCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 26214473   | The HTTP proxy configuration is not valid. |
| 23724130   | Cannot use an IPv6 name server address because there are no IPv6 interfaces. |
| 26214481   | Username and password cannot be empty when HTTP proxy authentication is enabled. |
| 26214482   | Username and password cannot be specified when HTTP proxy authentication is disabled. |
*/
type HTTPProxyModifyCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this http proxy modify collection default response has a 2xx status code
func (o *HTTPProxyModifyCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this http proxy modify collection default response has a 3xx status code
func (o *HTTPProxyModifyCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this http proxy modify collection default response has a 4xx status code
func (o *HTTPProxyModifyCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this http proxy modify collection default response has a 5xx status code
func (o *HTTPProxyModifyCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this http proxy modify collection default response a status code equal to that given
func (o *HTTPProxyModifyCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the http proxy modify collection default response
func (o *HTTPProxyModifyCollectionDefault) Code() int {
	return o._statusCode
}

func (o *HTTPProxyModifyCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /network/http-proxy][%d] http_proxy_modify_collection default %s", o._statusCode, payload)
}

func (o *HTTPProxyModifyCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /network/http-proxy][%d] http_proxy_modify_collection default %s", o._statusCode, payload)
}

func (o *HTTPProxyModifyCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *HTTPProxyModifyCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
HTTPProxyModifyCollectionBody HTTP proxy modify collection body
swagger:model HTTPProxyModifyCollectionBody
*/
type HTTPProxyModifyCollectionBody struct {

	// links
	Links *models.NetworkHTTPProxyInlineLinks `json:"_links,omitempty"`

	// Specifies whether or not authentication with the HTTP proxy server is enabled.
	//
	AuthenticationEnabled *bool `json:"authentication_enabled,omitempty"`

	// ipspace
	Ipspace *models.NetworkHTTPProxyInlineIpspace `json:"ipspace,omitempty"`

	// network http proxy response inline records
	NetworkHTTPProxyResponseInlineRecords []*models.NetworkHTTPProxy `json:"records,omitempty"`

	// Password to authenticate with the HTTP proxy server when authentication_enabled is set to "true".
	//
	Password *string `json:"password,omitempty"`

	// The port number on which the HTTP proxy service is configured on the
	// proxy server.
	//
	// Example: 3128
	// Maximum: 65535
	// Minimum: 1
	Port *int64 `json:"port,omitempty"`

	// Set to “svm” for HTTP proxy owned by an SVM. Otherwise, set to "cluster".
	//
	// Read Only: true
	// Enum: ["svm","cluster"]
	Scope *string `json:"scope,omitempty"`

	// Fully qualified domain name (FQDN) or IP address of the HTTP proxy server.
	//
	Server *string `json:"server,omitempty"`

	// svm
	Svm *models.NetworkHTTPProxyInlineSvm `json:"svm,omitempty"`

	// Username to authenticate with the HTTP proxy server when authentication_enabled is set to "true".
	//
	Username *string `json:"username,omitempty"`

	// The UUID that uniquely identifies the HTTP proxy.
	//
	// Read Only: true
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this HTTP proxy modify collection body
func (o *HTTPProxyModifyCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateIpspace(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateNetworkHTTPProxyResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validatePort(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateScope(formats); err != nil {
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

func (o *HTTPProxyModifyCollectionBody) validateLinks(formats strfmt.Registry) error {
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

func (o *HTTPProxyModifyCollectionBody) validateIpspace(formats strfmt.Registry) error {
	if swag.IsZero(o.Ipspace) { // not required
		return nil
	}

	if o.Ipspace != nil {
		if err := o.Ipspace.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "ipspace")
			}
			return err
		}
	}

	return nil
}

func (o *HTTPProxyModifyCollectionBody) validateNetworkHTTPProxyResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.NetworkHTTPProxyResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.NetworkHTTPProxyResponseInlineRecords); i++ {
		if swag.IsZero(o.NetworkHTTPProxyResponseInlineRecords[i]) { // not required
			continue
		}

		if o.NetworkHTTPProxyResponseInlineRecords[i] != nil {
			if err := o.NetworkHTTPProxyResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *HTTPProxyModifyCollectionBody) validatePort(formats strfmt.Registry) error {
	if swag.IsZero(o.Port) { // not required
		return nil
	}

	if err := validate.MinimumInt("info"+"."+"port", "body", *o.Port, 1, false); err != nil {
		return err
	}

	if err := validate.MaximumInt("info"+"."+"port", "body", *o.Port, 65535, false); err != nil {
		return err
	}

	return nil
}

var httpProxyModifyCollectionBodyTypeScopePropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["svm","cluster"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		httpProxyModifyCollectionBodyTypeScopePropEnum = append(httpProxyModifyCollectionBodyTypeScopePropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// HTTPProxyModifyCollectionBody
	// HTTPProxyModifyCollectionBody
	// scope
	// Scope
	// svm
	// END DEBUGGING
	// HTTPProxyModifyCollectionBodyScopeSvm captures enum value "svm"
	HTTPProxyModifyCollectionBodyScopeSvm string = "svm"

	// BEGIN DEBUGGING
	// HTTPProxyModifyCollectionBody
	// HTTPProxyModifyCollectionBody
	// scope
	// Scope
	// cluster
	// END DEBUGGING
	// HTTPProxyModifyCollectionBodyScopeCluster captures enum value "cluster"
	HTTPProxyModifyCollectionBodyScopeCluster string = "cluster"
)

// prop value enum
func (o *HTTPProxyModifyCollectionBody) validateScopeEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, httpProxyModifyCollectionBodyTypeScopePropEnum, true); err != nil {
		return err
	}
	return nil
}

func (o *HTTPProxyModifyCollectionBody) validateScope(formats strfmt.Registry) error {
	if swag.IsZero(o.Scope) { // not required
		return nil
	}

	// value enum
	if err := o.validateScopeEnum("info"+"."+"scope", "body", *o.Scope); err != nil {
		return err
	}

	return nil
}

func (o *HTTPProxyModifyCollectionBody) validateSvm(formats strfmt.Registry) error {
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

// ContextValidate validate this HTTP proxy modify collection body based on the context it is used
func (o *HTTPProxyModifyCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateIpspace(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateNetworkHTTPProxyResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateScope(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateSvm(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateUUID(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *HTTPProxyModifyCollectionBody) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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

func (o *HTTPProxyModifyCollectionBody) contextValidateIpspace(ctx context.Context, formats strfmt.Registry) error {

	if o.Ipspace != nil {
		if err := o.Ipspace.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "ipspace")
			}
			return err
		}
	}

	return nil
}

func (o *HTTPProxyModifyCollectionBody) contextValidateNetworkHTTPProxyResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.NetworkHTTPProxyResponseInlineRecords); i++ {

		if o.NetworkHTTPProxyResponseInlineRecords[i] != nil {
			if err := o.NetworkHTTPProxyResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *HTTPProxyModifyCollectionBody) contextValidateScope(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "info"+"."+"scope", "body", o.Scope); err != nil {
		return err
	}

	return nil
}

func (o *HTTPProxyModifyCollectionBody) contextValidateSvm(ctx context.Context, formats strfmt.Registry) error {

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

func (o *HTTPProxyModifyCollectionBody) contextValidateUUID(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "info"+"."+"uuid", "body", o.UUID); err != nil {
		return err
	}

	return nil
}

// MarshalBinary interface implementation
func (o *HTTPProxyModifyCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *HTTPProxyModifyCollectionBody) UnmarshalBinary(b []byte) error {
	var res HTTPProxyModifyCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
NetworkHTTPProxyInlineLinks network http proxy inline links
swagger:model network_http_proxy_inline__links
*/
type NetworkHTTPProxyInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this network http proxy inline links
func (o *NetworkHTTPProxyInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NetworkHTTPProxyInlineLinks) validateSelf(formats strfmt.Registry) error {
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

// ContextValidate validate this network http proxy inline links based on the context it is used
func (o *NetworkHTTPProxyInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NetworkHTTPProxyInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

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
func (o *NetworkHTTPProxyInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *NetworkHTTPProxyInlineLinks) UnmarshalBinary(b []byte) error {
	var res NetworkHTTPProxyInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
NetworkHTTPProxyInlineIpspace Applies to both SVM and cluster-scoped objects. Either the UUID or name is supplied on input. This is mutually exclusive with SVM during POST and PATCH.
//
swagger:model network_http_proxy_inline_ipspace
*/
type NetworkHTTPProxyInlineIpspace struct {

	// links
	Links *models.NetworkHTTPProxyInlineIpspaceInlineLinks `json:"_links,omitempty"`

	// IPspace name
	// Example: Default
	Name *string `json:"name,omitempty"`

	// IPspace UUID
	// Example: 1cd8a442-86d1-11e0-ae1c-123478563412
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this network http proxy inline ipspace
func (o *NetworkHTTPProxyInlineIpspace) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NetworkHTTPProxyInlineIpspace) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(o.Links) { // not required
		return nil
	}

	if o.Links != nil {
		if err := o.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "ipspace" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this network http proxy inline ipspace based on the context it is used
func (o *NetworkHTTPProxyInlineIpspace) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NetworkHTTPProxyInlineIpspace) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if o.Links != nil {
		if err := o.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "ipspace" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *NetworkHTTPProxyInlineIpspace) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *NetworkHTTPProxyInlineIpspace) UnmarshalBinary(b []byte) error {
	var res NetworkHTTPProxyInlineIpspace
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
NetworkHTTPProxyInlineIpspaceInlineLinks network http proxy inline ipspace inline links
swagger:model network_http_proxy_inline_ipspace_inline__links
*/
type NetworkHTTPProxyInlineIpspaceInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this network http proxy inline ipspace inline links
func (o *NetworkHTTPProxyInlineIpspaceInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NetworkHTTPProxyInlineIpspaceInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(o.Self) { // not required
		return nil
	}

	if o.Self != nil {
		if err := o.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "ipspace" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this network http proxy inline ipspace inline links based on the context it is used
func (o *NetworkHTTPProxyInlineIpspaceInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NetworkHTTPProxyInlineIpspaceInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if o.Self != nil {
		if err := o.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "ipspace" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *NetworkHTTPProxyInlineIpspaceInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *NetworkHTTPProxyInlineIpspaceInlineLinks) UnmarshalBinary(b []byte) error {
	var res NetworkHTTPProxyInlineIpspaceInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
NetworkHTTPProxyInlineSvm This is mutually exclusive with IPspace during POST and PATCH.
//
swagger:model network_http_proxy_inline_svm
*/
type NetworkHTTPProxyInlineSvm struct {

	// links
	Links *models.NetworkHTTPProxyInlineSvmInlineLinks `json:"_links,omitempty"`

	// The name of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: svm1
	Name *string `json:"name,omitempty"`

	// The unique identifier of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: 02c9e252-41be-11e9-81d5-00a0986138f7
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this network http proxy inline svm
func (o *NetworkHTTPProxyInlineSvm) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NetworkHTTPProxyInlineSvm) validateLinks(formats strfmt.Registry) error {
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

// ContextValidate validate this network http proxy inline svm based on the context it is used
func (o *NetworkHTTPProxyInlineSvm) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NetworkHTTPProxyInlineSvm) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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
func (o *NetworkHTTPProxyInlineSvm) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *NetworkHTTPProxyInlineSvm) UnmarshalBinary(b []byte) error {
	var res NetworkHTTPProxyInlineSvm
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
NetworkHTTPProxyInlineSvmInlineLinks network http proxy inline svm inline links
swagger:model network_http_proxy_inline_svm_inline__links
*/
type NetworkHTTPProxyInlineSvmInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this network http proxy inline svm inline links
func (o *NetworkHTTPProxyInlineSvmInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NetworkHTTPProxyInlineSvmInlineLinks) validateSelf(formats strfmt.Registry) error {
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

// ContextValidate validate this network http proxy inline svm inline links based on the context it is used
func (o *NetworkHTTPProxyInlineSvmInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NetworkHTTPProxyInlineSvmInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

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
func (o *NetworkHTTPProxyInlineSvmInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *NetworkHTTPProxyInlineSvmInlineLinks) UnmarshalBinary(b []byte) error {
	var res NetworkHTTPProxyInlineSvmInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}