// Code generated by go-swagger; DO NOT EDIT.

package name_services

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
)

// NewUnixGroupDeleteParams creates a new UnixGroupDeleteParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewUnixGroupDeleteParams() *UnixGroupDeleteParams {
	return &UnixGroupDeleteParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewUnixGroupDeleteParamsWithTimeout creates a new UnixGroupDeleteParams object
// with the ability to set a timeout on a request.
func NewUnixGroupDeleteParamsWithTimeout(timeout time.Duration) *UnixGroupDeleteParams {
	return &UnixGroupDeleteParams{
		timeout: timeout,
	}
}

// NewUnixGroupDeleteParamsWithContext creates a new UnixGroupDeleteParams object
// with the ability to set a context for a request.
func NewUnixGroupDeleteParamsWithContext(ctx context.Context) *UnixGroupDeleteParams {
	return &UnixGroupDeleteParams{
		Context: ctx,
	}
}

// NewUnixGroupDeleteParamsWithHTTPClient creates a new UnixGroupDeleteParams object
// with the ability to set a custom HTTPClient for a request.
func NewUnixGroupDeleteParamsWithHTTPClient(client *http.Client) *UnixGroupDeleteParams {
	return &UnixGroupDeleteParams{
		HTTPClient: client,
	}
}

/*
UnixGroupDeleteParams contains all the parameters to send to the API endpoint

	for the unix group delete operation.

	Typically these are written to a http.Request.
*/
type UnixGroupDeleteParams struct {

	/* Name.

	   UNIX group name.
	*/
	Name string

	/* SvmUUID.

	   UUID of the SVM to which this object belongs.
	*/
	SvmUUID string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the unix group delete params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *UnixGroupDeleteParams) WithDefaults() *UnixGroupDeleteParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the unix group delete params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *UnixGroupDeleteParams) SetDefaults() {
	// no default values defined for this parameter
}

// WithTimeout adds the timeout to the unix group delete params
func (o *UnixGroupDeleteParams) WithTimeout(timeout time.Duration) *UnixGroupDeleteParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the unix group delete params
func (o *UnixGroupDeleteParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the unix group delete params
func (o *UnixGroupDeleteParams) WithContext(ctx context.Context) *UnixGroupDeleteParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the unix group delete params
func (o *UnixGroupDeleteParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the unix group delete params
func (o *UnixGroupDeleteParams) WithHTTPClient(client *http.Client) *UnixGroupDeleteParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the unix group delete params
func (o *UnixGroupDeleteParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithName adds the name to the unix group delete params
func (o *UnixGroupDeleteParams) WithName(name string) *UnixGroupDeleteParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the unix group delete params
func (o *UnixGroupDeleteParams) SetName(name string) {
	o.Name = name
}

// WithSvmUUID adds the svmUUID to the unix group delete params
func (o *UnixGroupDeleteParams) WithSvmUUID(svmUUID string) *UnixGroupDeleteParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the unix group delete params
func (o *UnixGroupDeleteParams) SetSvmUUID(svmUUID string) {
	o.SvmUUID = svmUUID
}

// WriteToRequest writes these params to a swagger request
func (o *UnixGroupDeleteParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

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
