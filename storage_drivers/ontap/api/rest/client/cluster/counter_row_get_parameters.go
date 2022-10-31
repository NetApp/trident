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

// NewCounterRowGetParams creates a new CounterRowGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewCounterRowGetParams() *CounterRowGetParams {
	return &CounterRowGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewCounterRowGetParamsWithTimeout creates a new CounterRowGetParams object
// with the ability to set a timeout on a request.
func NewCounterRowGetParamsWithTimeout(timeout time.Duration) *CounterRowGetParams {
	return &CounterRowGetParams{
		timeout: timeout,
	}
}

// NewCounterRowGetParamsWithContext creates a new CounterRowGetParams object
// with the ability to set a context for a request.
func NewCounterRowGetParamsWithContext(ctx context.Context) *CounterRowGetParams {
	return &CounterRowGetParams{
		Context: ctx,
	}
}

// NewCounterRowGetParamsWithHTTPClient creates a new CounterRowGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewCounterRowGetParamsWithHTTPClient(client *http.Client) *CounterRowGetParams {
	return &CounterRowGetParams{
		HTTPClient: client,
	}
}

/*
CounterRowGetParams contains all the parameters to send to the API endpoint

	for the counter row get operation.

	Typically these are written to a http.Request.
*/
type CounterRowGetParams struct {

	/* CounterTableName.

	   Counter table name.
	*/
	CounterTableNamePathParameter string

	/* Fields.

	   Specify the fields to return.
	*/
	FieldsQueryParameter []string

	/* ID.

	   Unique row identifier.
	*/
	IDPathParameter string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the counter row get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *CounterRowGetParams) WithDefaults() *CounterRowGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the counter row get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *CounterRowGetParams) SetDefaults() {
	// no default values defined for this parameter
}

// WithTimeout adds the timeout to the counter row get params
func (o *CounterRowGetParams) WithTimeout(timeout time.Duration) *CounterRowGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the counter row get params
func (o *CounterRowGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the counter row get params
func (o *CounterRowGetParams) WithContext(ctx context.Context) *CounterRowGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the counter row get params
func (o *CounterRowGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the counter row get params
func (o *CounterRowGetParams) WithHTTPClient(client *http.Client) *CounterRowGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the counter row get params
func (o *CounterRowGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithCounterTableNamePathParameter adds the counterTableName to the counter row get params
func (o *CounterRowGetParams) WithCounterTableNamePathParameter(counterTableName string) *CounterRowGetParams {
	o.SetCounterTableNamePathParameter(counterTableName)
	return o
}

// SetCounterTableNamePathParameter adds the counterTableName to the counter row get params
func (o *CounterRowGetParams) SetCounterTableNamePathParameter(counterTableName string) {
	o.CounterTableNamePathParameter = counterTableName
}

// WithFieldsQueryParameter adds the fields to the counter row get params
func (o *CounterRowGetParams) WithFieldsQueryParameter(fields []string) *CounterRowGetParams {
	o.SetFieldsQueryParameter(fields)
	return o
}

// SetFieldsQueryParameter adds the fields to the counter row get params
func (o *CounterRowGetParams) SetFieldsQueryParameter(fields []string) {
	o.FieldsQueryParameter = fields
}

// WithIDPathParameter adds the id to the counter row get params
func (o *CounterRowGetParams) WithIDPathParameter(id string) *CounterRowGetParams {
	o.SetIDPathParameter(id)
	return o
}

// SetIDPathParameter adds the id to the counter row get params
func (o *CounterRowGetParams) SetIDPathParameter(id string) {
	o.IDPathParameter = id
}

// WriteToRequest writes these params to a swagger request
func (o *CounterRowGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	// path param counter_table.name
	if err := r.SetPathParam("counter_table.name", o.CounterTableNamePathParameter); err != nil {
		return err
	}

	if o.FieldsQueryParameter != nil {

		// binding items for fields
		joinedFields := o.bindParamFields(reg)

		// query array param fields
		if err := r.SetQueryParam("fields", joinedFields...); err != nil {
			return err
		}
	}

	// path param id
	if err := r.SetPathParam("id", o.IDPathParameter); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamCounterRowGet binds the parameter fields
func (o *CounterRowGetParams) bindParamFields(formats strfmt.Registry) []string {
	fieldsIR := o.FieldsQueryParameter

	var fieldsIC []string
	for _, fieldsIIR := range fieldsIR { // explode []string

		fieldsIIV := fieldsIIR // string as string
		fieldsIC = append(fieldsIC, fieldsIIV)
	}

	// items.CollectionFormat: "csv"
	fieldsIS := swag.JoinByFormat(fieldsIC, "csv")

	return fieldsIS
}