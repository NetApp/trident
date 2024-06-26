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

// NewRoleGetParams creates a new RoleGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewRoleGetParams() *RoleGetParams {
	return &RoleGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewRoleGetParamsWithTimeout creates a new RoleGetParams object
// with the ability to set a timeout on a request.
func NewRoleGetParamsWithTimeout(timeout time.Duration) *RoleGetParams {
	return &RoleGetParams{
		timeout: timeout,
	}
}

// NewRoleGetParamsWithContext creates a new RoleGetParams object
// with the ability to set a context for a request.
func NewRoleGetParamsWithContext(ctx context.Context) *RoleGetParams {
	return &RoleGetParams{
		Context: ctx,
	}
}

// NewRoleGetParamsWithHTTPClient creates a new RoleGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewRoleGetParamsWithHTTPClient(client *http.Client) *RoleGetParams {
	return &RoleGetParams{
		HTTPClient: client,
	}
}

/*
RoleGetParams contains all the parameters to send to the API endpoint

	for the role get operation.

	Typically these are written to a http.Request.
*/
type RoleGetParams struct {

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* Name.

	   Role name
	*/
	Name string

	/* OwnerUUID.

	   Role owner UUID
	*/
	OwnerUUID string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the role get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *RoleGetParams) WithDefaults() *RoleGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the role get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *RoleGetParams) SetDefaults() {
	// no default values defined for this parameter
}

// WithTimeout adds the timeout to the role get params
func (o *RoleGetParams) WithTimeout(timeout time.Duration) *RoleGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the role get params
func (o *RoleGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the role get params
func (o *RoleGetParams) WithContext(ctx context.Context) *RoleGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the role get params
func (o *RoleGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the role get params
func (o *RoleGetParams) WithHTTPClient(client *http.Client) *RoleGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the role get params
func (o *RoleGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithFields adds the fields to the role get params
func (o *RoleGetParams) WithFields(fields []string) *RoleGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the role get params
func (o *RoleGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithName adds the name to the role get params
func (o *RoleGetParams) WithName(name string) *RoleGetParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the role get params
func (o *RoleGetParams) SetName(name string) {
	o.Name = name
}

// WithOwnerUUID adds the ownerUUID to the role get params
func (o *RoleGetParams) WithOwnerUUID(ownerUUID string) *RoleGetParams {
	o.SetOwnerUUID(ownerUUID)
	return o
}

// SetOwnerUUID adds the ownerUuid to the role get params
func (o *RoleGetParams) SetOwnerUUID(ownerUUID string) {
	o.OwnerUUID = ownerUUID
}

// WriteToRequest writes these params to a swagger request
func (o *RoleGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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

	// path param name
	if err := r.SetPathParam("name", o.Name); err != nil {
		return err
	}

	// path param owner.uuid
	if err := r.SetPathParam("owner.uuid", o.OwnerUUID); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamRoleGet binds the parameter fields
func (o *RoleGetParams) bindParamFields(formats strfmt.Registry) []string {
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
