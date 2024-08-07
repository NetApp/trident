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

// NewAwsKmsRekeyInternalParams creates a new AwsKmsRekeyInternalParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewAwsKmsRekeyInternalParams() *AwsKmsRekeyInternalParams {
	return &AwsKmsRekeyInternalParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewAwsKmsRekeyInternalParamsWithTimeout creates a new AwsKmsRekeyInternalParams object
// with the ability to set a timeout on a request.
func NewAwsKmsRekeyInternalParamsWithTimeout(timeout time.Duration) *AwsKmsRekeyInternalParams {
	return &AwsKmsRekeyInternalParams{
		timeout: timeout,
	}
}

// NewAwsKmsRekeyInternalParamsWithContext creates a new AwsKmsRekeyInternalParams object
// with the ability to set a context for a request.
func NewAwsKmsRekeyInternalParamsWithContext(ctx context.Context) *AwsKmsRekeyInternalParams {
	return &AwsKmsRekeyInternalParams{
		Context: ctx,
	}
}

// NewAwsKmsRekeyInternalParamsWithHTTPClient creates a new AwsKmsRekeyInternalParams object
// with the ability to set a custom HTTPClient for a request.
func NewAwsKmsRekeyInternalParamsWithHTTPClient(client *http.Client) *AwsKmsRekeyInternalParams {
	return &AwsKmsRekeyInternalParams{
		HTTPClient: client,
	}
}

/*
AwsKmsRekeyInternalParams contains all the parameters to send to the API endpoint

	for the aws kms rekey internal operation.

	Typically these are written to a http.Request.
*/
type AwsKmsRekeyInternalParams struct {

	/* AwsKmsUUID.

	   UUID of the existing AWS KMS configuration.
	*/
	AwsKmsUUID string

	/* ReturnRecords.

	   The default is false.  If set to true, the records are returned.
	*/
	ReturnRecords *bool

	/* ReturnTimeout.

	   The number of seconds to allow the call to execute before returning. When doing a POST, PATCH, or DELETE operation on a single record, the default is 0 seconds.  This means that if an asynchronous operation is started, the server immediately returns HTTP code 202 (Accepted) along with a link to the job.  If a non-zero value is specified for POST, PATCH, or DELETE operations, ONTAP waits that length of time to see if the job completes so it can return something other than 202.
	*/
	ReturnTimeout *int64

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the aws kms rekey internal params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *AwsKmsRekeyInternalParams) WithDefaults() *AwsKmsRekeyInternalParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the aws kms rekey internal params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *AwsKmsRekeyInternalParams) SetDefaults() {
	var (
		returnRecordsDefault = bool(false)

		returnTimeoutDefault = int64(0)
	)

	val := AwsKmsRekeyInternalParams{
		ReturnRecords: &returnRecordsDefault,
		ReturnTimeout: &returnTimeoutDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the aws kms rekey internal params
func (o *AwsKmsRekeyInternalParams) WithTimeout(timeout time.Duration) *AwsKmsRekeyInternalParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the aws kms rekey internal params
func (o *AwsKmsRekeyInternalParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the aws kms rekey internal params
func (o *AwsKmsRekeyInternalParams) WithContext(ctx context.Context) *AwsKmsRekeyInternalParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the aws kms rekey internal params
func (o *AwsKmsRekeyInternalParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the aws kms rekey internal params
func (o *AwsKmsRekeyInternalParams) WithHTTPClient(client *http.Client) *AwsKmsRekeyInternalParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the aws kms rekey internal params
func (o *AwsKmsRekeyInternalParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithAwsKmsUUID adds the awsKmsUUID to the aws kms rekey internal params
func (o *AwsKmsRekeyInternalParams) WithAwsKmsUUID(awsKmsUUID string) *AwsKmsRekeyInternalParams {
	o.SetAwsKmsUUID(awsKmsUUID)
	return o
}

// SetAwsKmsUUID adds the awsKmsUuid to the aws kms rekey internal params
func (o *AwsKmsRekeyInternalParams) SetAwsKmsUUID(awsKmsUUID string) {
	o.AwsKmsUUID = awsKmsUUID
}

// WithReturnRecords adds the returnRecords to the aws kms rekey internal params
func (o *AwsKmsRekeyInternalParams) WithReturnRecords(returnRecords *bool) *AwsKmsRekeyInternalParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the aws kms rekey internal params
func (o *AwsKmsRekeyInternalParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the aws kms rekey internal params
func (o *AwsKmsRekeyInternalParams) WithReturnTimeout(returnTimeout *int64) *AwsKmsRekeyInternalParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the aws kms rekey internal params
func (o *AwsKmsRekeyInternalParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WriteToRequest writes these params to a swagger request
func (o *AwsKmsRekeyInternalParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	// path param aws_kms.uuid
	if err := r.SetPathParam("aws_kms.uuid", o.AwsKmsUUID); err != nil {
		return err
	}

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

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
