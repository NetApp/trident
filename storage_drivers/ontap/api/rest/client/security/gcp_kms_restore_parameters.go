// Code generated by go-swagger; DO NOT EDIT.

package security

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

// NewGcpKmsRestoreParams creates a new GcpKmsRestoreParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewGcpKmsRestoreParams() *GcpKmsRestoreParams {
	return &GcpKmsRestoreParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewGcpKmsRestoreParamsWithTimeout creates a new GcpKmsRestoreParams object
// with the ability to set a timeout on a request.
func NewGcpKmsRestoreParamsWithTimeout(timeout time.Duration) *GcpKmsRestoreParams {
	return &GcpKmsRestoreParams{
		timeout: timeout,
	}
}

// NewGcpKmsRestoreParamsWithContext creates a new GcpKmsRestoreParams object
// with the ability to set a context for a request.
func NewGcpKmsRestoreParamsWithContext(ctx context.Context) *GcpKmsRestoreParams {
	return &GcpKmsRestoreParams{
		Context: ctx,
	}
}

// NewGcpKmsRestoreParamsWithHTTPClient creates a new GcpKmsRestoreParams object
// with the ability to set a custom HTTPClient for a request.
func NewGcpKmsRestoreParamsWithHTTPClient(client *http.Client) *GcpKmsRestoreParams {
	return &GcpKmsRestoreParams{
		HTTPClient: client,
	}
}

/*
GcpKmsRestoreParams contains all the parameters to send to the API endpoint

	for the gcp kms restore operation.

	Typically these are written to a http.Request.
*/
type GcpKmsRestoreParams struct {

	/* ReturnRecords.

	   The default is false.  If set to true, the records are returned.
	*/
	ReturnRecords *bool

	/* ReturnTimeout.

	   The number of seconds to allow the call to execute before returning. When doing a POST, PATCH, or DELETE operation on a single record, the default is 0 seconds.  This means that if an asynchronous operation is started, the server immediately returns HTTP code 202 (Accepted) along with a link to the job.  If a non-zero value is specified for POST, PATCH, or DELETE operations, ONTAP waits that length of time to see if the job completes so it can return something other than 202.
	*/
	ReturnTimeout *int64

	/* UUID.

	   UUID of the existing Google Cloud KMS configuration.
	*/
	UUID string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the gcp kms restore params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *GcpKmsRestoreParams) WithDefaults() *GcpKmsRestoreParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the gcp kms restore params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *GcpKmsRestoreParams) SetDefaults() {
	var (
		returnRecordsDefault = bool(false)

		returnTimeoutDefault = int64(0)
	)

	val := GcpKmsRestoreParams{
		ReturnRecords: &returnRecordsDefault,
		ReturnTimeout: &returnTimeoutDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the gcp kms restore params
func (o *GcpKmsRestoreParams) WithTimeout(timeout time.Duration) *GcpKmsRestoreParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the gcp kms restore params
func (o *GcpKmsRestoreParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the gcp kms restore params
func (o *GcpKmsRestoreParams) WithContext(ctx context.Context) *GcpKmsRestoreParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the gcp kms restore params
func (o *GcpKmsRestoreParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the gcp kms restore params
func (o *GcpKmsRestoreParams) WithHTTPClient(client *http.Client) *GcpKmsRestoreParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the gcp kms restore params
func (o *GcpKmsRestoreParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithReturnRecords adds the returnRecords to the gcp kms restore params
func (o *GcpKmsRestoreParams) WithReturnRecords(returnRecords *bool) *GcpKmsRestoreParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the gcp kms restore params
func (o *GcpKmsRestoreParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the gcp kms restore params
func (o *GcpKmsRestoreParams) WithReturnTimeout(returnTimeout *int64) *GcpKmsRestoreParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the gcp kms restore params
func (o *GcpKmsRestoreParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithUUID adds the uuid to the gcp kms restore params
func (o *GcpKmsRestoreParams) WithUUID(uuid string) *GcpKmsRestoreParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the gcp kms restore params
func (o *GcpKmsRestoreParams) SetUUID(uuid string) {
	o.UUID = uuid
}

// WriteToRequest writes these params to a swagger request
func (o *GcpKmsRestoreParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.ReturnRecords != nil {

		// query param return_records
		var qrReturnRecords bool

		if o.ReturnRecords != nil {
			qrReturnRecords = *o.ReturnRecords
		}
		qReturnRecords := swag.FormatBool(qrReturnRecords)
		if qReturnRecords != "" {

			if err := r.SetQueryParam("return_records", qReturnRecords); err != nil {
				return err
			}
		}
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

	// path param uuid
	if err := r.SetPathParam("uuid", o.UUID); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
