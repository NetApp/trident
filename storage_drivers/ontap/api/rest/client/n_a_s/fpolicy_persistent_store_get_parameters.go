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

// NewFpolicyPersistentStoreGetParams creates a new FpolicyPersistentStoreGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewFpolicyPersistentStoreGetParams() *FpolicyPersistentStoreGetParams {
	return &FpolicyPersistentStoreGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewFpolicyPersistentStoreGetParamsWithTimeout creates a new FpolicyPersistentStoreGetParams object
// with the ability to set a timeout on a request.
func NewFpolicyPersistentStoreGetParamsWithTimeout(timeout time.Duration) *FpolicyPersistentStoreGetParams {
	return &FpolicyPersistentStoreGetParams{
		timeout: timeout,
	}
}

// NewFpolicyPersistentStoreGetParamsWithContext creates a new FpolicyPersistentStoreGetParams object
// with the ability to set a context for a request.
func NewFpolicyPersistentStoreGetParamsWithContext(ctx context.Context) *FpolicyPersistentStoreGetParams {
	return &FpolicyPersistentStoreGetParams{
		Context: ctx,
	}
}

// NewFpolicyPersistentStoreGetParamsWithHTTPClient creates a new FpolicyPersistentStoreGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewFpolicyPersistentStoreGetParamsWithHTTPClient(client *http.Client) *FpolicyPersistentStoreGetParams {
	return &FpolicyPersistentStoreGetParams{
		HTTPClient: client,
	}
}

/*
FpolicyPersistentStoreGetParams contains all the parameters to send to the API endpoint

	for the fpolicy persistent store get operation.

	Typically these are written to a http.Request.
*/
type FpolicyPersistentStoreGetParams struct {

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	// Name.
	Name string

	/* SvmUUID.

	   UUID of the SVM to which this object belongs.
	*/
	SvmUUID string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the fpolicy persistent store get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *FpolicyPersistentStoreGetParams) WithDefaults() *FpolicyPersistentStoreGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the fpolicy persistent store get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *FpolicyPersistentStoreGetParams) SetDefaults() {
	// no default values defined for this parameter
}

// WithTimeout adds the timeout to the fpolicy persistent store get params
func (o *FpolicyPersistentStoreGetParams) WithTimeout(timeout time.Duration) *FpolicyPersistentStoreGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the fpolicy persistent store get params
func (o *FpolicyPersistentStoreGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the fpolicy persistent store get params
func (o *FpolicyPersistentStoreGetParams) WithContext(ctx context.Context) *FpolicyPersistentStoreGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the fpolicy persistent store get params
func (o *FpolicyPersistentStoreGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the fpolicy persistent store get params
func (o *FpolicyPersistentStoreGetParams) WithHTTPClient(client *http.Client) *FpolicyPersistentStoreGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the fpolicy persistent store get params
func (o *FpolicyPersistentStoreGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithFields adds the fields to the fpolicy persistent store get params
func (o *FpolicyPersistentStoreGetParams) WithFields(fields []string) *FpolicyPersistentStoreGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the fpolicy persistent store get params
func (o *FpolicyPersistentStoreGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithName adds the name to the fpolicy persistent store get params
func (o *FpolicyPersistentStoreGetParams) WithName(name string) *FpolicyPersistentStoreGetParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the fpolicy persistent store get params
func (o *FpolicyPersistentStoreGetParams) SetName(name string) {
	o.Name = name
}

// WithSvmUUID adds the svmUUID to the fpolicy persistent store get params
func (o *FpolicyPersistentStoreGetParams) WithSvmUUID(svmUUID string) *FpolicyPersistentStoreGetParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the fpolicy persistent store get params
func (o *FpolicyPersistentStoreGetParams) SetSvmUUID(svmUUID string) {
	o.SvmUUID = svmUUID
}

// WriteToRequest writes these params to a swagger request
func (o *FpolicyPersistentStoreGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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

	// path param svm.uuid
	if err := r.SetPathParam("svm.uuid", o.SvmUUID); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamFpolicyPersistentStoreGet binds the parameter fields
func (o *FpolicyPersistentStoreGetParams) bindParamFields(formats strfmt.Registry) []string {
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
