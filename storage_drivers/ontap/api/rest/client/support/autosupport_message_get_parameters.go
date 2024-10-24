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

// NewAutosupportMessageGetParams creates a new AutosupportMessageGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewAutosupportMessageGetParams() *AutosupportMessageGetParams {
	return &AutosupportMessageGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewAutosupportMessageGetParamsWithTimeout creates a new AutosupportMessageGetParams object
// with the ability to set a timeout on a request.
func NewAutosupportMessageGetParamsWithTimeout(timeout time.Duration) *AutosupportMessageGetParams {
	return &AutosupportMessageGetParams{
		timeout: timeout,
	}
}

// NewAutosupportMessageGetParamsWithContext creates a new AutosupportMessageGetParams object
// with the ability to set a context for a request.
func NewAutosupportMessageGetParamsWithContext(ctx context.Context) *AutosupportMessageGetParams {
	return &AutosupportMessageGetParams{
		Context: ctx,
	}
}

// NewAutosupportMessageGetParamsWithHTTPClient creates a new AutosupportMessageGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewAutosupportMessageGetParamsWithHTTPClient(client *http.Client) *AutosupportMessageGetParams {
	return &AutosupportMessageGetParams{
		HTTPClient: client,
	}
}

/*
AutosupportMessageGetParams contains all the parameters to send to the API endpoint

	for the autosupport message get operation.

	Typically these are written to a http.Request.
*/
type AutosupportMessageGetParams struct {

	/* Destination.

	   The destination for this AutoSupport message.
	*/
	Destination string

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* Index.

	   AutoSupport message sequence number
	*/
	Index string

	/* NodeUUID.

	   Node UUID
	*/
	NodeUUID string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the autosupport message get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *AutosupportMessageGetParams) WithDefaults() *AutosupportMessageGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the autosupport message get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *AutosupportMessageGetParams) SetDefaults() {
	// no default values defined for this parameter
}

// WithTimeout adds the timeout to the autosupport message get params
func (o *AutosupportMessageGetParams) WithTimeout(timeout time.Duration) *AutosupportMessageGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the autosupport message get params
func (o *AutosupportMessageGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the autosupport message get params
func (o *AutosupportMessageGetParams) WithContext(ctx context.Context) *AutosupportMessageGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the autosupport message get params
func (o *AutosupportMessageGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the autosupport message get params
func (o *AutosupportMessageGetParams) WithHTTPClient(client *http.Client) *AutosupportMessageGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the autosupport message get params
func (o *AutosupportMessageGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithDestination adds the destination to the autosupport message get params
func (o *AutosupportMessageGetParams) WithDestination(destination string) *AutosupportMessageGetParams {
	o.SetDestination(destination)
	return o
}

// SetDestination adds the destination to the autosupport message get params
func (o *AutosupportMessageGetParams) SetDestination(destination string) {
	o.Destination = destination
}

// WithFields adds the fields to the autosupport message get params
func (o *AutosupportMessageGetParams) WithFields(fields []string) *AutosupportMessageGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the autosupport message get params
func (o *AutosupportMessageGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithIndex adds the index to the autosupport message get params
func (o *AutosupportMessageGetParams) WithIndex(index string) *AutosupportMessageGetParams {
	o.SetIndex(index)
	return o
}

// SetIndex adds the index to the autosupport message get params
func (o *AutosupportMessageGetParams) SetIndex(index string) {
	o.Index = index
}

// WithNodeUUID adds the nodeUUID to the autosupport message get params
func (o *AutosupportMessageGetParams) WithNodeUUID(nodeUUID string) *AutosupportMessageGetParams {
	o.SetNodeUUID(nodeUUID)
	return o
}

// SetNodeUUID adds the nodeUuid to the autosupport message get params
func (o *AutosupportMessageGetParams) SetNodeUUID(nodeUUID string) {
	o.NodeUUID = nodeUUID
}

// WriteToRequest writes these params to a swagger request
func (o *AutosupportMessageGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	// path param destination
	if err := r.SetPathParam("destination", o.Destination); err != nil {
		return err
	}

	if o.Fields != nil {

		// binding items for fields
		joinedFields := o.bindParamFields(reg)

		// query array param fields
		if err := r.SetQueryParam("fields", joinedFields...); err != nil {
			return err
		}
	}

	// path param index
	if err := r.SetPathParam("index", o.Index); err != nil {
		return err
	}

	// path param node.uuid
	if err := r.SetPathParam("node.uuid", o.NodeUUID); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamAutosupportMessageGet binds the parameter fields
func (o *AutosupportMessageGetParams) bindParamFields(formats strfmt.Registry) []string {
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
