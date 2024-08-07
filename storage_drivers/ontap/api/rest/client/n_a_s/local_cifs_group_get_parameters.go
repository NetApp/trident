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

// NewLocalCifsGroupGetParams creates a new LocalCifsGroupGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewLocalCifsGroupGetParams() *LocalCifsGroupGetParams {
	return &LocalCifsGroupGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewLocalCifsGroupGetParamsWithTimeout creates a new LocalCifsGroupGetParams object
// with the ability to set a timeout on a request.
func NewLocalCifsGroupGetParamsWithTimeout(timeout time.Duration) *LocalCifsGroupGetParams {
	return &LocalCifsGroupGetParams{
		timeout: timeout,
	}
}

// NewLocalCifsGroupGetParamsWithContext creates a new LocalCifsGroupGetParams object
// with the ability to set a context for a request.
func NewLocalCifsGroupGetParamsWithContext(ctx context.Context) *LocalCifsGroupGetParams {
	return &LocalCifsGroupGetParams{
		Context: ctx,
	}
}

// NewLocalCifsGroupGetParamsWithHTTPClient creates a new LocalCifsGroupGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewLocalCifsGroupGetParamsWithHTTPClient(client *http.Client) *LocalCifsGroupGetParams {
	return &LocalCifsGroupGetParams{
		HTTPClient: client,
	}
}

/*
LocalCifsGroupGetParams contains all the parameters to send to the API endpoint

	for the local cifs group get operation.

	Typically these are written to a http.Request.
*/
type LocalCifsGroupGetParams struct {

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* Sid.

	   Local group SID
	*/
	Sid string

	/* SvmUUID.

	   UUID of the SVM to which this object belongs.
	*/
	SvmUUID string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the local cifs group get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *LocalCifsGroupGetParams) WithDefaults() *LocalCifsGroupGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the local cifs group get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *LocalCifsGroupGetParams) SetDefaults() {
	// no default values defined for this parameter
}

// WithTimeout adds the timeout to the local cifs group get params
func (o *LocalCifsGroupGetParams) WithTimeout(timeout time.Duration) *LocalCifsGroupGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the local cifs group get params
func (o *LocalCifsGroupGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the local cifs group get params
func (o *LocalCifsGroupGetParams) WithContext(ctx context.Context) *LocalCifsGroupGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the local cifs group get params
func (o *LocalCifsGroupGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the local cifs group get params
func (o *LocalCifsGroupGetParams) WithHTTPClient(client *http.Client) *LocalCifsGroupGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the local cifs group get params
func (o *LocalCifsGroupGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithFields adds the fields to the local cifs group get params
func (o *LocalCifsGroupGetParams) WithFields(fields []string) *LocalCifsGroupGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the local cifs group get params
func (o *LocalCifsGroupGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithSid adds the sid to the local cifs group get params
func (o *LocalCifsGroupGetParams) WithSid(sid string) *LocalCifsGroupGetParams {
	o.SetSid(sid)
	return o
}

// SetSid adds the sid to the local cifs group get params
func (o *LocalCifsGroupGetParams) SetSid(sid string) {
	o.Sid = sid
}

// WithSvmUUID adds the svmUUID to the local cifs group get params
func (o *LocalCifsGroupGetParams) WithSvmUUID(svmUUID string) *LocalCifsGroupGetParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the local cifs group get params
func (o *LocalCifsGroupGetParams) SetSvmUUID(svmUUID string) {
	o.SvmUUID = svmUUID
}

// WriteToRequest writes these params to a swagger request
func (o *LocalCifsGroupGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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

	// path param sid
	if err := r.SetPathParam("sid", o.Sid); err != nil {
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

// bindParamLocalCifsGroupGet binds the parameter fields
func (o *LocalCifsGroupGetParams) bindParamFields(formats strfmt.Registry) []string {
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
