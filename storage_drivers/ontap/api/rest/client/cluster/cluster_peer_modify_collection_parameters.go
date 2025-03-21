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

// NewClusterPeerModifyCollectionParams creates a new ClusterPeerModifyCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewClusterPeerModifyCollectionParams() *ClusterPeerModifyCollectionParams {
	return &ClusterPeerModifyCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewClusterPeerModifyCollectionParamsWithTimeout creates a new ClusterPeerModifyCollectionParams object
// with the ability to set a timeout on a request.
func NewClusterPeerModifyCollectionParamsWithTimeout(timeout time.Duration) *ClusterPeerModifyCollectionParams {
	return &ClusterPeerModifyCollectionParams{
		timeout: timeout,
	}
}

// NewClusterPeerModifyCollectionParamsWithContext creates a new ClusterPeerModifyCollectionParams object
// with the ability to set a context for a request.
func NewClusterPeerModifyCollectionParamsWithContext(ctx context.Context) *ClusterPeerModifyCollectionParams {
	return &ClusterPeerModifyCollectionParams{
		Context: ctx,
	}
}

// NewClusterPeerModifyCollectionParamsWithHTTPClient creates a new ClusterPeerModifyCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewClusterPeerModifyCollectionParamsWithHTTPClient(client *http.Client) *ClusterPeerModifyCollectionParams {
	return &ClusterPeerModifyCollectionParams{
		HTTPClient: client,
	}
}

/*
ClusterPeerModifyCollectionParams contains all the parameters to send to the API endpoint

	for the cluster peer modify collection operation.

	Typically these are written to a http.Request.
*/
type ClusterPeerModifyCollectionParams struct {

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* Info.

	   Info specification
	*/
	Info ClusterPeerModifyCollectionBody

	/* ReturnRecords.

	   The default is true for GET calls.  When set to false, only the number of records is returned.

	   Default: true
	*/
	ReturnRecords *bool

	/* ReturnTimeout.

	   The number of seconds to allow the call to execute before returning.  When iterating over a collection, the default is 15 seconds.  ONTAP returns earlier if either max records or the end of the collection is reached.

	   Default: 15
	*/
	ReturnTimeout *int64

	/* SerialRecords.

	   Perform the operation on the records synchronously.
	*/
	SerialRecords *bool

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the cluster peer modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *ClusterPeerModifyCollectionParams) WithDefaults() *ClusterPeerModifyCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the cluster peer modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *ClusterPeerModifyCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := ClusterPeerModifyCollectionParams{
		ContinueOnFailure: &continueOnFailureDefault,
		ReturnRecords:     &returnRecordsDefault,
		ReturnTimeout:     &returnTimeoutDefault,
		SerialRecords:     &serialRecordsDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the cluster peer modify collection params
func (o *ClusterPeerModifyCollectionParams) WithTimeout(timeout time.Duration) *ClusterPeerModifyCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the cluster peer modify collection params
func (o *ClusterPeerModifyCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the cluster peer modify collection params
func (o *ClusterPeerModifyCollectionParams) WithContext(ctx context.Context) *ClusterPeerModifyCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the cluster peer modify collection params
func (o *ClusterPeerModifyCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the cluster peer modify collection params
func (o *ClusterPeerModifyCollectionParams) WithHTTPClient(client *http.Client) *ClusterPeerModifyCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the cluster peer modify collection params
func (o *ClusterPeerModifyCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithContinueOnFailure adds the continueOnFailure to the cluster peer modify collection params
func (o *ClusterPeerModifyCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *ClusterPeerModifyCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the cluster peer modify collection params
func (o *ClusterPeerModifyCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithInfo adds the info to the cluster peer modify collection params
func (o *ClusterPeerModifyCollectionParams) WithInfo(info ClusterPeerModifyCollectionBody) *ClusterPeerModifyCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the cluster peer modify collection params
func (o *ClusterPeerModifyCollectionParams) SetInfo(info ClusterPeerModifyCollectionBody) {
	o.Info = info
}

// WithReturnRecords adds the returnRecords to the cluster peer modify collection params
func (o *ClusterPeerModifyCollectionParams) WithReturnRecords(returnRecords *bool) *ClusterPeerModifyCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the cluster peer modify collection params
func (o *ClusterPeerModifyCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the cluster peer modify collection params
func (o *ClusterPeerModifyCollectionParams) WithReturnTimeout(returnTimeout *int64) *ClusterPeerModifyCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the cluster peer modify collection params
func (o *ClusterPeerModifyCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithSerialRecords adds the serialRecords to the cluster peer modify collection params
func (o *ClusterPeerModifyCollectionParams) WithSerialRecords(serialRecords *bool) *ClusterPeerModifyCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the cluster peer modify collection params
func (o *ClusterPeerModifyCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WriteToRequest writes these params to a swagger request
func (o *ClusterPeerModifyCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.ContinueOnFailure != nil {

		// query param continue_on_failure
		var qrContinueOnFailure bool

		if o.ContinueOnFailure != nil {
			qrContinueOnFailure = *o.ContinueOnFailure
		}
		qContinueOnFailure := swag.FormatBool(qrContinueOnFailure)
		if qContinueOnFailure != "" {

			if err := r.SetQueryParam("continue_on_failure", qContinueOnFailure); err != nil {
				return err
			}
		}
	}
	if err := r.SetBodyParam(o.Info); err != nil {
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

	if o.SerialRecords != nil {

		// query param serial_records
		var qrSerialRecords bool

		if o.SerialRecords != nil {
			qrSerialRecords = *o.SerialRecords
		}
		qSerialRecords := swag.FormatBool(qrSerialRecords)
		if qSerialRecords != "" {

			if err := r.SetQueryParam("serial_records", qSerialRecords); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
