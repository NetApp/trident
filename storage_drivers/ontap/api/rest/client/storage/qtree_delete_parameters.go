// Code generated by go-swagger; DO NOT EDIT.

package storage

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

// NewQtreeDeleteParams creates a new QtreeDeleteParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewQtreeDeleteParams() *QtreeDeleteParams {
	return &QtreeDeleteParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewQtreeDeleteParamsWithTimeout creates a new QtreeDeleteParams object
// with the ability to set a timeout on a request.
func NewQtreeDeleteParamsWithTimeout(timeout time.Duration) *QtreeDeleteParams {
	return &QtreeDeleteParams{
		timeout: timeout,
	}
}

// NewQtreeDeleteParamsWithContext creates a new QtreeDeleteParams object
// with the ability to set a context for a request.
func NewQtreeDeleteParamsWithContext(ctx context.Context) *QtreeDeleteParams {
	return &QtreeDeleteParams{
		Context: ctx,
	}
}

// NewQtreeDeleteParamsWithHTTPClient creates a new QtreeDeleteParams object
// with the ability to set a custom HTTPClient for a request.
func NewQtreeDeleteParamsWithHTTPClient(client *http.Client) *QtreeDeleteParams {
	return &QtreeDeleteParams{
		HTTPClient: client,
	}
}

/*
QtreeDeleteParams contains all the parameters to send to the API endpoint

	for the qtree delete operation.

	Typically these are written to a http.Request.
*/
type QtreeDeleteParams struct {

	/* ID.

	   Qtree ID
	*/
	ID string

	/* ReturnTimeout.

	   The number of seconds to allow the call to execute before returning. When doing a POST, PATCH, or DELETE operation on a single record, the default is 0 seconds.  This means that if an asynchronous operation is started, the server immediately returns HTTP code 202 (Accepted) along with a link to the job.  If a non-zero value is specified for POST, PATCH, or DELETE operations, ONTAP waits that length of time to see if the job completes so it can return something other than 202.
	*/
	ReturnTimeout *int64

	/* VolumeUUID.

	   Volume UUID
	*/
	VolumeUUID string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the qtree delete params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *QtreeDeleteParams) WithDefaults() *QtreeDeleteParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the qtree delete params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *QtreeDeleteParams) SetDefaults() {
	var (
		returnTimeoutDefault = int64(0)
	)

	val := QtreeDeleteParams{
		ReturnTimeout: &returnTimeoutDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the qtree delete params
func (o *QtreeDeleteParams) WithTimeout(timeout time.Duration) *QtreeDeleteParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the qtree delete params
func (o *QtreeDeleteParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the qtree delete params
func (o *QtreeDeleteParams) WithContext(ctx context.Context) *QtreeDeleteParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the qtree delete params
func (o *QtreeDeleteParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the qtree delete params
func (o *QtreeDeleteParams) WithHTTPClient(client *http.Client) *QtreeDeleteParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the qtree delete params
func (o *QtreeDeleteParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithID adds the id to the qtree delete params
func (o *QtreeDeleteParams) WithID(id string) *QtreeDeleteParams {
	o.SetID(id)
	return o
}

// SetID adds the id to the qtree delete params
func (o *QtreeDeleteParams) SetID(id string) {
	o.ID = id
}

// WithReturnTimeout adds the returnTimeout to the qtree delete params
func (o *QtreeDeleteParams) WithReturnTimeout(returnTimeout *int64) *QtreeDeleteParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the qtree delete params
func (o *QtreeDeleteParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithVolumeUUID adds the volumeUUID to the qtree delete params
func (o *QtreeDeleteParams) WithVolumeUUID(volumeUUID string) *QtreeDeleteParams {
	o.SetVolumeUUID(volumeUUID)
	return o
}

// SetVolumeUUID adds the volumeUuid to the qtree delete params
func (o *QtreeDeleteParams) SetVolumeUUID(volumeUUID string) {
	o.VolumeUUID = volumeUUID
}

// WriteToRequest writes these params to a swagger request
func (o *QtreeDeleteParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	// path param id
	if err := r.SetPathParam("id", o.ID); err != nil {
		return err
	}

	if o.ReturnTimeout != nil {

		// query param return_timeout
		var qrReturnTimeout int64

		if o.ReturnTimeout != nil {
			qrReturnTimeout = *o.ReturnTimeout
		}
		qReturnTimeout := swag.FormatInt64(qrReturnTimeout)
		if qReturnTimeout != "" {

			if err := r.SetQueryParam("return_timeout", qReturnTimeout); err != nil {
				return err
			}
		}
	}

	// path param volume.uuid
	if err := r.SetPathParam("volume.uuid", o.VolumeUUID); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
