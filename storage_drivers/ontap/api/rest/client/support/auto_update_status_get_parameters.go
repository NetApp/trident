// Code generated by go-swagger; DO NOT EDIT.

package support

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

// NewAutoUpdateStatusGetParams creates a new AutoUpdateStatusGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewAutoUpdateStatusGetParams() *AutoUpdateStatusGetParams {
	return &AutoUpdateStatusGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewAutoUpdateStatusGetParamsWithTimeout creates a new AutoUpdateStatusGetParams object
// with the ability to set a timeout on a request.
func NewAutoUpdateStatusGetParamsWithTimeout(timeout time.Duration) *AutoUpdateStatusGetParams {
	return &AutoUpdateStatusGetParams{
		timeout: timeout,
	}
}

// NewAutoUpdateStatusGetParamsWithContext creates a new AutoUpdateStatusGetParams object
// with the ability to set a context for a request.
func NewAutoUpdateStatusGetParamsWithContext(ctx context.Context) *AutoUpdateStatusGetParams {
	return &AutoUpdateStatusGetParams{
		Context: ctx,
	}
}

// NewAutoUpdateStatusGetParamsWithHTTPClient creates a new AutoUpdateStatusGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewAutoUpdateStatusGetParamsWithHTTPClient(client *http.Client) *AutoUpdateStatusGetParams {
	return &AutoUpdateStatusGetParams{
		HTTPClient: client,
	}
}

/*
AutoUpdateStatusGetParams contains all the parameters to send to the API endpoint

	for the auto update status get operation.

	Typically these are written to a http.Request.
*/
type AutoUpdateStatusGetParams struct {

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* UUID.

	   Update identifier
	*/
	UUID string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the auto update status get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *AutoUpdateStatusGetParams) WithDefaults() *AutoUpdateStatusGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the auto update status get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *AutoUpdateStatusGetParams) SetDefaults() {
	// no default values defined for this parameter
}

// WithTimeout adds the timeout to the auto update status get params
func (o *AutoUpdateStatusGetParams) WithTimeout(timeout time.Duration) *AutoUpdateStatusGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the auto update status get params
func (o *AutoUpdateStatusGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the auto update status get params
func (o *AutoUpdateStatusGetParams) WithContext(ctx context.Context) *AutoUpdateStatusGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the auto update status get params
func (o *AutoUpdateStatusGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the auto update status get params
func (o *AutoUpdateStatusGetParams) WithHTTPClient(client *http.Client) *AutoUpdateStatusGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the auto update status get params
func (o *AutoUpdateStatusGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithFields adds the fields to the auto update status get params
func (o *AutoUpdateStatusGetParams) WithFields(fields []string) *AutoUpdateStatusGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the auto update status get params
func (o *AutoUpdateStatusGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithUUID adds the uuid to the auto update status get params
func (o *AutoUpdateStatusGetParams) WithUUID(uuid string) *AutoUpdateStatusGetParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the auto update status get params
func (o *AutoUpdateStatusGetParams) SetUUID(uuid string) {
	o.UUID = uuid
}

// WriteToRequest writes these params to a swagger request
func (o *AutoUpdateStatusGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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

	// path param uuid
	if err := r.SetPathParam("uuid", o.UUID); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamAutoUpdateStatusGet binds the parameter fields
func (o *AutoUpdateStatusGetParams) bindParamFields(formats strfmt.Registry) []string {
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
