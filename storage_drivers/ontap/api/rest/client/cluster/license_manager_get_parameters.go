// Code generated by go-swagger; DO NOT EDIT.

package cluster

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

// NewLicenseManagerGetParams creates a new LicenseManagerGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewLicenseManagerGetParams() *LicenseManagerGetParams {
	return &LicenseManagerGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewLicenseManagerGetParamsWithTimeout creates a new LicenseManagerGetParams object
// with the ability to set a timeout on a request.
func NewLicenseManagerGetParamsWithTimeout(timeout time.Duration) *LicenseManagerGetParams {
	return &LicenseManagerGetParams{
		timeout: timeout,
	}
}

// NewLicenseManagerGetParamsWithContext creates a new LicenseManagerGetParams object
// with the ability to set a context for a request.
func NewLicenseManagerGetParamsWithContext(ctx context.Context) *LicenseManagerGetParams {
	return &LicenseManagerGetParams{
		Context: ctx,
	}
}

// NewLicenseManagerGetParamsWithHTTPClient creates a new LicenseManagerGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewLicenseManagerGetParamsWithHTTPClient(client *http.Client) *LicenseManagerGetParams {
	return &LicenseManagerGetParams{
		HTTPClient: client,
	}
}

/*
LicenseManagerGetParams contains all the parameters to send to the API endpoint

	for the license manager get operation.

	Typically these are written to a http.Request.
*/
type LicenseManagerGetParams struct {

	/* Default.

	   Filter by default
	*/
	Default *bool

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* URIHost.

	   Filter by uri.host
	*/
	URIHost *string

	// UUID.
	UUID string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the license manager get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *LicenseManagerGetParams) WithDefaults() *LicenseManagerGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the license manager get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *LicenseManagerGetParams) SetDefaults() {
	// no default values defined for this parameter
}

// WithTimeout adds the timeout to the license manager get params
func (o *LicenseManagerGetParams) WithTimeout(timeout time.Duration) *LicenseManagerGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the license manager get params
func (o *LicenseManagerGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the license manager get params
func (o *LicenseManagerGetParams) WithContext(ctx context.Context) *LicenseManagerGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the license manager get params
func (o *LicenseManagerGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the license manager get params
func (o *LicenseManagerGetParams) WithHTTPClient(client *http.Client) *LicenseManagerGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the license manager get params
func (o *LicenseManagerGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithDefault adds the defaultVar to the license manager get params
func (o *LicenseManagerGetParams) WithDefault(defaultVar *bool) *LicenseManagerGetParams {
	o.SetDefault(defaultVar)
	return o
}

// SetDefault adds the default to the license manager get params
func (o *LicenseManagerGetParams) SetDefault(defaultVar *bool) {
	o.Default = defaultVar
}

// WithFields adds the fields to the license manager get params
func (o *LicenseManagerGetParams) WithFields(fields []string) *LicenseManagerGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the license manager get params
func (o *LicenseManagerGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithURIHost adds the uRIHost to the license manager get params
func (o *LicenseManagerGetParams) WithURIHost(uRIHost *string) *LicenseManagerGetParams {
	o.SetURIHost(uRIHost)
	return o
}

// SetURIHost adds the uriHost to the license manager get params
func (o *LicenseManagerGetParams) SetURIHost(uRIHost *string) {
	o.URIHost = uRIHost
}

// WithUUID adds the uuid to the license manager get params
func (o *LicenseManagerGetParams) WithUUID(uuid string) *LicenseManagerGetParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the license manager get params
func (o *LicenseManagerGetParams) SetUUID(uuid string) {
	o.UUID = uuid
}

// WriteToRequest writes these params to a swagger request
func (o *LicenseManagerGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.Default != nil {

		// query param default
		var qrDefault bool

		if o.Default != nil {
			qrDefault = *o.Default
		}
		qDefault := swag.FormatBool(qrDefault)
		if qDefault != "" {

			if err := r.SetQueryParam("default", qDefault); err != nil {
				return err
			}
		}
	}

	if o.Fields != nil {

		// binding items for fields
		joinedFields := o.bindParamFields(reg)

		// query array param fields
		if err := r.SetQueryParam("fields", joinedFields...); err != nil {
			return err
		}
	}

	if o.URIHost != nil {

		// query param uri.host
		var qrURIHost string

		if o.URIHost != nil {
			qrURIHost = *o.URIHost
		}
		qURIHost := qrURIHost
		if qURIHost != "" {

			if err := r.SetQueryParam("uri.host", qURIHost); err != nil {
				return err
			}
		}
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

// bindParamLicenseManagerGet binds the parameter fields
func (o *LicenseManagerGetParams) bindParamFields(formats strfmt.Registry) []string {
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
