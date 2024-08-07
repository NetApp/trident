// Code generated by go-swagger; DO NOT EDIT.

package security

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"net/http"
	"time"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	cr "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// NewSecurityKeyManagerKeyServersGetParams creates a new SecurityKeyManagerKeyServersGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewSecurityKeyManagerKeyServersGetParams() *SecurityKeyManagerKeyServersGetParams {
	return &SecurityKeyManagerKeyServersGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewSecurityKeyManagerKeyServersGetParamsWithTimeout creates a new SecurityKeyManagerKeyServersGetParams object
// with the ability to set a timeout on a request.
func NewSecurityKeyManagerKeyServersGetParamsWithTimeout(timeout time.Duration) *SecurityKeyManagerKeyServersGetParams {
	return &SecurityKeyManagerKeyServersGetParams{
		timeout: timeout,
	}
}

// NewSecurityKeyManagerKeyServersGetParamsWithContext creates a new SecurityKeyManagerKeyServersGetParams object
// with the ability to set a context for a request.
func NewSecurityKeyManagerKeyServersGetParamsWithContext(ctx context.Context) *SecurityKeyManagerKeyServersGetParams {
	return &SecurityKeyManagerKeyServersGetParams{
		Context: ctx,
	}
}

// NewSecurityKeyManagerKeyServersGetParamsWithHTTPClient creates a new SecurityKeyManagerKeyServersGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewSecurityKeyManagerKeyServersGetParamsWithHTTPClient(client *http.Client) *SecurityKeyManagerKeyServersGetParams {
	return &SecurityKeyManagerKeyServersGetParams{
		HTTPClient: client,
	}
}

/*
SecurityKeyManagerKeyServersGetParams contains all the parameters to send to the API endpoint

	for the security key manager key servers get operation.

	Typically these are written to a http.Request.
*/
type SecurityKeyManagerKeyServersGetParams struct {

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* Server.

	   Primary Key server configured in the key manager.
	*/
	Server string

	/* UUID.

	   External key manager UUID
	*/
	UUID string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the security key manager key servers get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *SecurityKeyManagerKeyServersGetParams) WithDefaults() *SecurityKeyManagerKeyServersGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the security key manager key servers get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *SecurityKeyManagerKeyServersGetParams) SetDefaults() {
	// no default values defined for this parameter
}

// WithTimeout adds the timeout to the security key manager key servers get params
func (o *SecurityKeyManagerKeyServersGetParams) WithTimeout(timeout time.Duration) *SecurityKeyManagerKeyServersGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the security key manager key servers get params
func (o *SecurityKeyManagerKeyServersGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the security key manager key servers get params
func (o *SecurityKeyManagerKeyServersGetParams) WithContext(ctx context.Context) *SecurityKeyManagerKeyServersGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the security key manager key servers get params
func (o *SecurityKeyManagerKeyServersGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the security key manager key servers get params
func (o *SecurityKeyManagerKeyServersGetParams) WithHTTPClient(client *http.Client) *SecurityKeyManagerKeyServersGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the security key manager key servers get params
func (o *SecurityKeyManagerKeyServersGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithFields adds the fields to the security key manager key servers get params
func (o *SecurityKeyManagerKeyServersGetParams) WithFields(fields []string) *SecurityKeyManagerKeyServersGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the security key manager key servers get params
func (o *SecurityKeyManagerKeyServersGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithServer adds the server to the security key manager key servers get params
func (o *SecurityKeyManagerKeyServersGetParams) WithServer(server string) *SecurityKeyManagerKeyServersGetParams {
	o.SetServer(server)
	return o
}

// SetServer adds the server to the security key manager key servers get params
func (o *SecurityKeyManagerKeyServersGetParams) SetServer(server string) {
	o.Server = server
}

// WithUUID adds the uuid to the security key manager key servers get params
func (o *SecurityKeyManagerKeyServersGetParams) WithUUID(uuid string) *SecurityKeyManagerKeyServersGetParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the security key manager key servers get params
func (o *SecurityKeyManagerKeyServersGetParams) SetUUID(uuid string) {
	o.UUID = uuid
}

// WriteToRequest writes these params to a swagger request
func (o *SecurityKeyManagerKeyServersGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.Fields != nil {

		// binding items for fields
		joinedFields := o.bindParamFields(reg)

		// query array param fields
		if err := r.SetQueryParam("fields", joinedFields...); err != nil {
			return err
		}
	}

	// path param server
	if err := r.SetPathParam("server", o.Server); err != nil {
		return err
	}

	// path param uuid
	if err := r.SetPathParam("uuid", o.UUID); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamSecurityKeyManagerKeyServersGet binds the parameter fields
func (o *SecurityKeyManagerKeyServersGetParams) bindParamFields(formats strfmt.Registry) []string {
	fieldsIR := o.Fields

	var fieldsIC []string
	for _, fieldsIIR := range fieldsIR { // explode []string

		fieldsIIV := fieldsIIR // string as string
		fieldsIC = append(fieldsIC, fieldsIIV)
	}

	// items.CollectionFormat: "csv"
	fieldsIS := swag.JoinByFormat(fieldsIC, "csv")

	return fieldsIS
}
