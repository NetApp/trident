// Code generated by go-swagger; DO NOT EDIT.

package s_a_n

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

// NewLunAttributeGetParams creates a new LunAttributeGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewLunAttributeGetParams() *LunAttributeGetParams {
	return &LunAttributeGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewLunAttributeGetParamsWithTimeout creates a new LunAttributeGetParams object
// with the ability to set a timeout on a request.
func NewLunAttributeGetParamsWithTimeout(timeout time.Duration) *LunAttributeGetParams {
	return &LunAttributeGetParams{
		timeout: timeout,
	}
}

// NewLunAttributeGetParamsWithContext creates a new LunAttributeGetParams object
// with the ability to set a context for a request.
func NewLunAttributeGetParamsWithContext(ctx context.Context) *LunAttributeGetParams {
	return &LunAttributeGetParams{
		Context: ctx,
	}
}

// NewLunAttributeGetParamsWithHTTPClient creates a new LunAttributeGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewLunAttributeGetParamsWithHTTPClient(client *http.Client) *LunAttributeGetParams {
	return &LunAttributeGetParams{
		HTTPClient: client,
	}
}

/*
LunAttributeGetParams contains all the parameters to send to the API endpoint

	for the lun attribute get operation.

	Typically these are written to a http.Request.
*/
type LunAttributeGetParams struct {

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* LunUUID.

	   The unique identifier of the LUN.

	*/
	LunUUID string

	/* Name.

	   The name of the attribute.

	*/
	Name string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the lun attribute get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *LunAttributeGetParams) WithDefaults() *LunAttributeGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the lun attribute get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *LunAttributeGetParams) SetDefaults() {
	// no default values defined for this parameter
}

// WithTimeout adds the timeout to the lun attribute get params
func (o *LunAttributeGetParams) WithTimeout(timeout time.Duration) *LunAttributeGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the lun attribute get params
func (o *LunAttributeGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the lun attribute get params
func (o *LunAttributeGetParams) WithContext(ctx context.Context) *LunAttributeGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the lun attribute get params
func (o *LunAttributeGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the lun attribute get params
func (o *LunAttributeGetParams) WithHTTPClient(client *http.Client) *LunAttributeGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the lun attribute get params
func (o *LunAttributeGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithFields adds the fields to the lun attribute get params
func (o *LunAttributeGetParams) WithFields(fields []string) *LunAttributeGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the lun attribute get params
func (o *LunAttributeGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithLunUUID adds the lunUUID to the lun attribute get params
func (o *LunAttributeGetParams) WithLunUUID(lunUUID string) *LunAttributeGetParams {
	o.SetLunUUID(lunUUID)
	return o
}

// SetLunUUID adds the lunUuid to the lun attribute get params
func (o *LunAttributeGetParams) SetLunUUID(lunUUID string) {
	o.LunUUID = lunUUID
}

// WithName adds the name to the lun attribute get params
func (o *LunAttributeGetParams) WithName(name string) *LunAttributeGetParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the lun attribute get params
func (o *LunAttributeGetParams) SetName(name string) {
	o.Name = name
}

// WriteToRequest writes these params to a swagger request
func (o *LunAttributeGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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

	// path param lun.uuid
	if err := r.SetPathParam("lun.uuid", o.LunUUID); err != nil {
		return err
	}

	// path param name
	if err := r.SetPathParam("name", o.Name); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamLunAttributeGet binds the parameter fields
func (o *LunAttributeGetParams) bindParamFields(formats strfmt.Registry) []string {
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
