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

// NewTokenDeleteCollectionParams creates a new TokenDeleteCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewTokenDeleteCollectionParams() *TokenDeleteCollectionParams {
	return &TokenDeleteCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewTokenDeleteCollectionParamsWithTimeout creates a new TokenDeleteCollectionParams object
// with the ability to set a timeout on a request.
func NewTokenDeleteCollectionParamsWithTimeout(timeout time.Duration) *TokenDeleteCollectionParams {
	return &TokenDeleteCollectionParams{
		timeout: timeout,
	}
}

// NewTokenDeleteCollectionParamsWithContext creates a new TokenDeleteCollectionParams object
// with the ability to set a context for a request.
func NewTokenDeleteCollectionParamsWithContext(ctx context.Context) *TokenDeleteCollectionParams {
	return &TokenDeleteCollectionParams{
		Context: ctx,
	}
}

// NewTokenDeleteCollectionParamsWithHTTPClient creates a new TokenDeleteCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewTokenDeleteCollectionParamsWithHTTPClient(client *http.Client) *TokenDeleteCollectionParams {
	return &TokenDeleteCollectionParams{
		HTTPClient: client,
	}
}

/*
TokenDeleteCollectionParams contains all the parameters to send to the API endpoint

	for the token delete collection operation.

	Typically these are written to a http.Request.
*/
type TokenDeleteCollectionParams struct {

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* ExpiryTimeLeft.

	   Filter by expiry_time.left
	*/
	ExpiryTimeLeft *string

	/* ExpiryTimeLimit.

	   Filter by expiry_time.limit
	*/
	ExpiryTimeLimit *string

	/* Info.

	   Info specification
	*/
	Info TokenDeleteCollectionBody

	/* NodeName.

	   Filter by node.name
	*/
	NodeName *string

	/* NodeUUID.

	   Filter by node.uuid
	*/
	NodeUUID *string

	/* ReserveSize.

	   Filter by reserve_size
	*/
	ReserveSize *int64

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

	/* UUID.

	   Filter by uuid
	*/
	UUID *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the token delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *TokenDeleteCollectionParams) WithDefaults() *TokenDeleteCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the token delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *TokenDeleteCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := TokenDeleteCollectionParams{
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

// WithTimeout adds the timeout to the token delete collection params
func (o *TokenDeleteCollectionParams) WithTimeout(timeout time.Duration) *TokenDeleteCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the token delete collection params
func (o *TokenDeleteCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the token delete collection params
func (o *TokenDeleteCollectionParams) WithContext(ctx context.Context) *TokenDeleteCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the token delete collection params
func (o *TokenDeleteCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the token delete collection params
func (o *TokenDeleteCollectionParams) WithHTTPClient(client *http.Client) *TokenDeleteCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the token delete collection params
func (o *TokenDeleteCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithContinueOnFailure adds the continueOnFailure to the token delete collection params
func (o *TokenDeleteCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *TokenDeleteCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the token delete collection params
func (o *TokenDeleteCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithExpiryTimeLeft adds the expiryTimeLeft to the token delete collection params
func (o *TokenDeleteCollectionParams) WithExpiryTimeLeft(expiryTimeLeft *string) *TokenDeleteCollectionParams {
	o.SetExpiryTimeLeft(expiryTimeLeft)
	return o
}

// SetExpiryTimeLeft adds the expiryTimeLeft to the token delete collection params
func (o *TokenDeleteCollectionParams) SetExpiryTimeLeft(expiryTimeLeft *string) {
	o.ExpiryTimeLeft = expiryTimeLeft
}

// WithExpiryTimeLimit adds the expiryTimeLimit to the token delete collection params
func (o *TokenDeleteCollectionParams) WithExpiryTimeLimit(expiryTimeLimit *string) *TokenDeleteCollectionParams {
	o.SetExpiryTimeLimit(expiryTimeLimit)
	return o
}

// SetExpiryTimeLimit adds the expiryTimeLimit to the token delete collection params
func (o *TokenDeleteCollectionParams) SetExpiryTimeLimit(expiryTimeLimit *string) {
	o.ExpiryTimeLimit = expiryTimeLimit
}

// WithInfo adds the info to the token delete collection params
func (o *TokenDeleteCollectionParams) WithInfo(info TokenDeleteCollectionBody) *TokenDeleteCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the token delete collection params
func (o *TokenDeleteCollectionParams) SetInfo(info TokenDeleteCollectionBody) {
	o.Info = info
}

// WithNodeName adds the nodeName to the token delete collection params
func (o *TokenDeleteCollectionParams) WithNodeName(nodeName *string) *TokenDeleteCollectionParams {
	o.SetNodeName(nodeName)
	return o
}

// SetNodeName adds the nodeName to the token delete collection params
func (o *TokenDeleteCollectionParams) SetNodeName(nodeName *string) {
	o.NodeName = nodeName
}

// WithNodeUUID adds the nodeUUID to the token delete collection params
func (o *TokenDeleteCollectionParams) WithNodeUUID(nodeUUID *string) *TokenDeleteCollectionParams {
	o.SetNodeUUID(nodeUUID)
	return o
}

// SetNodeUUID adds the nodeUuid to the token delete collection params
func (o *TokenDeleteCollectionParams) SetNodeUUID(nodeUUID *string) {
	o.NodeUUID = nodeUUID
}

// WithReserveSize adds the reserveSize to the token delete collection params
func (o *TokenDeleteCollectionParams) WithReserveSize(reserveSize *int64) *TokenDeleteCollectionParams {
	o.SetReserveSize(reserveSize)
	return o
}

// SetReserveSize adds the reserveSize to the token delete collection params
func (o *TokenDeleteCollectionParams) SetReserveSize(reserveSize *int64) {
	o.ReserveSize = reserveSize
}

// WithReturnRecords adds the returnRecords to the token delete collection params
func (o *TokenDeleteCollectionParams) WithReturnRecords(returnRecords *bool) *TokenDeleteCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the token delete collection params
func (o *TokenDeleteCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the token delete collection params
func (o *TokenDeleteCollectionParams) WithReturnTimeout(returnTimeout *int64) *TokenDeleteCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the token delete collection params
func (o *TokenDeleteCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithSerialRecords adds the serialRecords to the token delete collection params
func (o *TokenDeleteCollectionParams) WithSerialRecords(serialRecords *bool) *TokenDeleteCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the token delete collection params
func (o *TokenDeleteCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WithUUID adds the uuid to the token delete collection params
func (o *TokenDeleteCollectionParams) WithUUID(uuid *string) *TokenDeleteCollectionParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the token delete collection params
func (o *TokenDeleteCollectionParams) SetUUID(uuid *string) {
	o.UUID = uuid
}

// WriteToRequest writes these params to a swagger request
func (o *TokenDeleteCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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

	if o.ExpiryTimeLeft != nil {

		// query param expiry_time.left
		var qrExpiryTimeLeft string

		if o.ExpiryTimeLeft != nil {
			qrExpiryTimeLeft = *o.ExpiryTimeLeft
		}
		qExpiryTimeLeft := qrExpiryTimeLeft
		if qExpiryTimeLeft != "" {

			if err := r.SetQueryParam("expiry_time.left", qExpiryTimeLeft); err != nil {
				return err
			}
		}
	}

	if o.ExpiryTimeLimit != nil {

		// query param expiry_time.limit
		var qrExpiryTimeLimit string

		if o.ExpiryTimeLimit != nil {
			qrExpiryTimeLimit = *o.ExpiryTimeLimit
		}
		qExpiryTimeLimit := qrExpiryTimeLimit
		if qExpiryTimeLimit != "" {

			if err := r.SetQueryParam("expiry_time.limit", qExpiryTimeLimit); err != nil {
				return err
			}
		}
	}
	if err := r.SetBodyParam(o.Info); err != nil {
		return err
	}

	if o.NodeName != nil {

		// query param node.name
		var qrNodeName string

		if o.NodeName != nil {
			qrNodeName = *o.NodeName
		}
		qNodeName := qrNodeName
		if qNodeName != "" {

			if err := r.SetQueryParam("node.name", qNodeName); err != nil {
				return err
			}
		}
	}

	if o.NodeUUID != nil {

		// query param node.uuid
		var qrNodeUUID string

		if o.NodeUUID != nil {
			qrNodeUUID = *o.NodeUUID
		}
		qNodeUUID := qrNodeUUID
		if qNodeUUID != "" {

			if err := r.SetQueryParam("node.uuid", qNodeUUID); err != nil {
				return err
			}
		}
	}

	if o.ReserveSize != nil {

		// query param reserve_size
		var qrReserveSize int64

		if o.ReserveSize != nil {
			qrReserveSize = *o.ReserveSize
		}
		qReserveSize := swag.FormatInt64(qrReserveSize)
		if qReserveSize != "" {

			if err := r.SetQueryParam("reserve_size", qReserveSize); err != nil {
				return err
			}
		}
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

	if o.UUID != nil {

		// query param uuid
		var qrUUID string

		if o.UUID != nil {
			qrUUID = *o.UUID
		}
		qUUID := qrUUID
		if qUUID != "" {

			if err := r.SetQueryParam("uuid", qUUID); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}