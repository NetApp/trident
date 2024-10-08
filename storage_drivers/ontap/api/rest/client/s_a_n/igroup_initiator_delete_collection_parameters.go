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

// NewIgroupInitiatorDeleteCollectionParams creates a new IgroupInitiatorDeleteCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewIgroupInitiatorDeleteCollectionParams() *IgroupInitiatorDeleteCollectionParams {
	return &IgroupInitiatorDeleteCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewIgroupInitiatorDeleteCollectionParamsWithTimeout creates a new IgroupInitiatorDeleteCollectionParams object
// with the ability to set a timeout on a request.
func NewIgroupInitiatorDeleteCollectionParamsWithTimeout(timeout time.Duration) *IgroupInitiatorDeleteCollectionParams {
	return &IgroupInitiatorDeleteCollectionParams{
		timeout: timeout,
	}
}

// NewIgroupInitiatorDeleteCollectionParamsWithContext creates a new IgroupInitiatorDeleteCollectionParams object
// with the ability to set a context for a request.
func NewIgroupInitiatorDeleteCollectionParamsWithContext(ctx context.Context) *IgroupInitiatorDeleteCollectionParams {
	return &IgroupInitiatorDeleteCollectionParams{
		Context: ctx,
	}
}

// NewIgroupInitiatorDeleteCollectionParamsWithHTTPClient creates a new IgroupInitiatorDeleteCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewIgroupInitiatorDeleteCollectionParamsWithHTTPClient(client *http.Client) *IgroupInitiatorDeleteCollectionParams {
	return &IgroupInitiatorDeleteCollectionParams{
		HTTPClient: client,
	}
}

/*
IgroupInitiatorDeleteCollectionParams contains all the parameters to send to the API endpoint

	for the igroup initiator delete collection operation.

	Typically these are written to a http.Request.
*/
type IgroupInitiatorDeleteCollectionParams struct {

	/* AllowDeleteWhileMapped.

	     Allows the deletion of an initiator from of a mapped initiator group.<br/>
	Deleting an initiator from a mapped initiator group makes the LUNs to which the initiator group is mapped no longer available to the initiator. This might cause a disruption in the availability of data.<br/>
	<b>This parameter should be used with caution.</b>

	*/
	AllowDeleteWhileMapped *bool

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* IgroupUUID.

	   The unique identifier of the initiator group.

	*/
	IgroupUUID string

	/* Info.

	   Info specification
	*/
	Info IgroupInitiatorDeleteCollectionBody

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

// WithDefaults hydrates default values in the igroup initiator delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *IgroupInitiatorDeleteCollectionParams) WithDefaults() *IgroupInitiatorDeleteCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the igroup initiator delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *IgroupInitiatorDeleteCollectionParams) SetDefaults() {
	var (
		allowDeleteWhileMappedDefault = bool(false)

		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := IgroupInitiatorDeleteCollectionParams{
		AllowDeleteWhileMapped: &allowDeleteWhileMappedDefault,
		ContinueOnFailure:      &continueOnFailureDefault,
		ReturnRecords:          &returnRecordsDefault,
		ReturnTimeout:          &returnTimeoutDefault,
		SerialRecords:          &serialRecordsDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) WithTimeout(timeout time.Duration) *IgroupInitiatorDeleteCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) WithContext(ctx context.Context) *IgroupInitiatorDeleteCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) WithHTTPClient(client *http.Client) *IgroupInitiatorDeleteCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithAllowDeleteWhileMapped adds the allowDeleteWhileMapped to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) WithAllowDeleteWhileMapped(allowDeleteWhileMapped *bool) *IgroupInitiatorDeleteCollectionParams {
	o.SetAllowDeleteWhileMapped(allowDeleteWhileMapped)
	return o
}

// SetAllowDeleteWhileMapped adds the allowDeleteWhileMapped to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) SetAllowDeleteWhileMapped(allowDeleteWhileMapped *bool) {
	o.AllowDeleteWhileMapped = allowDeleteWhileMapped
}

// WithContinueOnFailure adds the continueOnFailure to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *IgroupInitiatorDeleteCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithIgroupUUID adds the igroupUUID to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) WithIgroupUUID(igroupUUID string) *IgroupInitiatorDeleteCollectionParams {
	o.SetIgroupUUID(igroupUUID)
	return o
}

// SetIgroupUUID adds the igroupUuid to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) SetIgroupUUID(igroupUUID string) {
	o.IgroupUUID = igroupUUID
}

// WithInfo adds the info to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) WithInfo(info IgroupInitiatorDeleteCollectionBody) *IgroupInitiatorDeleteCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) SetInfo(info IgroupInitiatorDeleteCollectionBody) {
	o.Info = info
}

// WithReturnRecords adds the returnRecords to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) WithReturnRecords(returnRecords *bool) *IgroupInitiatorDeleteCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) WithReturnTimeout(returnTimeout *int64) *IgroupInitiatorDeleteCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithSerialRecords adds the serialRecords to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) WithSerialRecords(serialRecords *bool) *IgroupInitiatorDeleteCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the igroup initiator delete collection params
func (o *IgroupInitiatorDeleteCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WriteToRequest writes these params to a swagger request
func (o *IgroupInitiatorDeleteCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.AllowDeleteWhileMapped != nil {

		// query param allow_delete_while_mapped
		var qrAllowDeleteWhileMapped bool

		if o.AllowDeleteWhileMapped != nil {
			qrAllowDeleteWhileMapped = *o.AllowDeleteWhileMapped
		}
		qAllowDeleteWhileMapped := swag.FormatBool(qrAllowDeleteWhileMapped)
		if qAllowDeleteWhileMapped != "" {

			if err := r.SetQueryParam("allow_delete_while_mapped", qAllowDeleteWhileMapped); err != nil {
				return err
			}
		}
	}

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

	// path param igroup.uuid
	if err := r.SetPathParam("igroup.uuid", o.IgroupUUID); err != nil {
		return err
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
