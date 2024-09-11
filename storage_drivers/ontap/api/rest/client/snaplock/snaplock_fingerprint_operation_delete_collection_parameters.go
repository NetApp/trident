// Code generated by go-swagger; DO NOT EDIT.

package snaplock

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

// NewSnaplockFingerprintOperationDeleteCollectionParams creates a new SnaplockFingerprintOperationDeleteCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewSnaplockFingerprintOperationDeleteCollectionParams() *SnaplockFingerprintOperationDeleteCollectionParams {
	return &SnaplockFingerprintOperationDeleteCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewSnaplockFingerprintOperationDeleteCollectionParamsWithTimeout creates a new SnaplockFingerprintOperationDeleteCollectionParams object
// with the ability to set a timeout on a request.
func NewSnaplockFingerprintOperationDeleteCollectionParamsWithTimeout(timeout time.Duration) *SnaplockFingerprintOperationDeleteCollectionParams {
	return &SnaplockFingerprintOperationDeleteCollectionParams{
		timeout: timeout,
	}
}

// NewSnaplockFingerprintOperationDeleteCollectionParamsWithContext creates a new SnaplockFingerprintOperationDeleteCollectionParams object
// with the ability to set a context for a request.
func NewSnaplockFingerprintOperationDeleteCollectionParamsWithContext(ctx context.Context) *SnaplockFingerprintOperationDeleteCollectionParams {
	return &SnaplockFingerprintOperationDeleteCollectionParams{
		Context: ctx,
	}
}

// NewSnaplockFingerprintOperationDeleteCollectionParamsWithHTTPClient creates a new SnaplockFingerprintOperationDeleteCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewSnaplockFingerprintOperationDeleteCollectionParamsWithHTTPClient(client *http.Client) *SnaplockFingerprintOperationDeleteCollectionParams {
	return &SnaplockFingerprintOperationDeleteCollectionParams{
		HTTPClient: client,
	}
}

/*
SnaplockFingerprintOperationDeleteCollectionParams contains all the parameters to send to the API endpoint

	for the snaplock fingerprint operation delete collection operation.

	Typically these are written to a http.Request.
*/
type SnaplockFingerprintOperationDeleteCollectionParams struct {

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* Info.

	   Info specification
	*/
	Info SnaplockFingerprintOperationDeleteCollectionBody

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

	/* SvmUUID.

	   SVM UUID
	*/
	SvmUUID string

	/* VolumeUUID.

	   Volume UUID
	*/
	VolumeUUID string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the snaplock fingerprint operation delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *SnaplockFingerprintOperationDeleteCollectionParams) WithDefaults() *SnaplockFingerprintOperationDeleteCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the snaplock fingerprint operation delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *SnaplockFingerprintOperationDeleteCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := SnaplockFingerprintOperationDeleteCollectionParams{
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

// WithTimeout adds the timeout to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) WithTimeout(timeout time.Duration) *SnaplockFingerprintOperationDeleteCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) WithContext(ctx context.Context) *SnaplockFingerprintOperationDeleteCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) WithHTTPClient(client *http.Client) *SnaplockFingerprintOperationDeleteCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithContinueOnFailure adds the continueOnFailure to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *SnaplockFingerprintOperationDeleteCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithInfo adds the info to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) WithInfo(info SnaplockFingerprintOperationDeleteCollectionBody) *SnaplockFingerprintOperationDeleteCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) SetInfo(info SnaplockFingerprintOperationDeleteCollectionBody) {
	o.Info = info
}

// WithReturnRecords adds the returnRecords to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) WithReturnRecords(returnRecords *bool) *SnaplockFingerprintOperationDeleteCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) WithReturnTimeout(returnTimeout *int64) *SnaplockFingerprintOperationDeleteCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithSerialRecords adds the serialRecords to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) WithSerialRecords(serialRecords *bool) *SnaplockFingerprintOperationDeleteCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WithSvmUUID adds the svmUUID to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) WithSvmUUID(svmUUID string) *SnaplockFingerprintOperationDeleteCollectionParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) SetSvmUUID(svmUUID string) {
	o.SvmUUID = svmUUID
}

// WithVolumeUUID adds the volumeUUID to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) WithVolumeUUID(volumeUUID string) *SnaplockFingerprintOperationDeleteCollectionParams {
	o.SetVolumeUUID(volumeUUID)
	return o
}

// SetVolumeUUID adds the volumeUuid to the snaplock fingerprint operation delete collection params
func (o *SnaplockFingerprintOperationDeleteCollectionParams) SetVolumeUUID(volumeUUID string) {
	o.VolumeUUID = volumeUUID
}

// WriteToRequest writes these params to a swagger request
func (o *SnaplockFingerprintOperationDeleteCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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

	// query param svm.uuid
	qrSvmUUID := o.SvmUUID
	qSvmUUID := qrSvmUUID
	if qSvmUUID != "" {

		if err := r.SetQueryParam("svm.uuid", qSvmUUID); err != nil {
			return err
		}
	}

	// query param volume.uuid
	qrVolumeUUID := o.VolumeUUID
	qVolumeUUID := qrVolumeUUID
	if qVolumeUUID != "" {

		if err := r.SetQueryParam("volume.uuid", qVolumeUUID); err != nil {
			return err
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}