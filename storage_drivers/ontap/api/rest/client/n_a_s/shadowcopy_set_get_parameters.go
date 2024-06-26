// Code generated by go-swagger; DO NOT EDIT.

package n_a_s

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

// NewShadowcopySetGetParams creates a new ShadowcopySetGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewShadowcopySetGetParams() *ShadowcopySetGetParams {
	return &ShadowcopySetGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewShadowcopySetGetParamsWithTimeout creates a new ShadowcopySetGetParams object
// with the ability to set a timeout on a request.
func NewShadowcopySetGetParamsWithTimeout(timeout time.Duration) *ShadowcopySetGetParams {
	return &ShadowcopySetGetParams{
		timeout: timeout,
	}
}

// NewShadowcopySetGetParamsWithContext creates a new ShadowcopySetGetParams object
// with the ability to set a context for a request.
func NewShadowcopySetGetParamsWithContext(ctx context.Context) *ShadowcopySetGetParams {
	return &ShadowcopySetGetParams{
		Context: ctx,
	}
}

// NewShadowcopySetGetParamsWithHTTPClient creates a new ShadowcopySetGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewShadowcopySetGetParamsWithHTTPClient(client *http.Client) *ShadowcopySetGetParams {
	return &ShadowcopySetGetParams{
		HTTPClient: client,
	}
}

/*
ShadowcopySetGetParams contains all the parameters to send to the API endpoint

	for the shadowcopy set get operation.

	Typically these are written to a http.Request.
*/
type ShadowcopySetGetParams struct {

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* UUID.

	   Storage shadowcopy set ID.
	*/
	UUID string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the shadowcopy set get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *ShadowcopySetGetParams) WithDefaults() *ShadowcopySetGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the shadowcopy set get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *ShadowcopySetGetParams) SetDefaults() {
	// no default values defined for this parameter
}

// WithTimeout adds the timeout to the shadowcopy set get params
func (o *ShadowcopySetGetParams) WithTimeout(timeout time.Duration) *ShadowcopySetGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the shadowcopy set get params
func (o *ShadowcopySetGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the shadowcopy set get params
func (o *ShadowcopySetGetParams) WithContext(ctx context.Context) *ShadowcopySetGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the shadowcopy set get params
func (o *ShadowcopySetGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the shadowcopy set get params
func (o *ShadowcopySetGetParams) WithHTTPClient(client *http.Client) *ShadowcopySetGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the shadowcopy set get params
func (o *ShadowcopySetGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithFields adds the fields to the shadowcopy set get params
func (o *ShadowcopySetGetParams) WithFields(fields []string) *ShadowcopySetGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the shadowcopy set get params
func (o *ShadowcopySetGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithUUID adds the uuid to the shadowcopy set get params
func (o *ShadowcopySetGetParams) WithUUID(uuid string) *ShadowcopySetGetParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the shadowcopy set get params
func (o *ShadowcopySetGetParams) SetUUID(uuid string) {
	o.UUID = uuid
}

// WriteToRequest writes these params to a swagger request
func (o *ShadowcopySetGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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

// bindParamShadowcopySetGet binds the parameter fields
func (o *ShadowcopySetGetParams) bindParamFields(formats strfmt.Registry) []string {
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
