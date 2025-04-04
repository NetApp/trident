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
	"github.com/go-openapi/swag"
)

// NewLocalHostDeleteCollectionParams creates a new LocalHostDeleteCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewLocalHostDeleteCollectionParams() *LocalHostDeleteCollectionParams {
	return &LocalHostDeleteCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewLocalHostDeleteCollectionParamsWithTimeout creates a new LocalHostDeleteCollectionParams object
// with the ability to set a timeout on a request.
func NewLocalHostDeleteCollectionParamsWithTimeout(timeout time.Duration) *LocalHostDeleteCollectionParams {
	return &LocalHostDeleteCollectionParams{
		timeout: timeout,
	}
}

// NewLocalHostDeleteCollectionParamsWithContext creates a new LocalHostDeleteCollectionParams object
// with the ability to set a context for a request.
func NewLocalHostDeleteCollectionParamsWithContext(ctx context.Context) *LocalHostDeleteCollectionParams {
	return &LocalHostDeleteCollectionParams{
		Context: ctx,
	}
}

// NewLocalHostDeleteCollectionParamsWithHTTPClient creates a new LocalHostDeleteCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewLocalHostDeleteCollectionParamsWithHTTPClient(client *http.Client) *LocalHostDeleteCollectionParams {
	return &LocalHostDeleteCollectionParams{
		HTTPClient: client,
	}
}

/*
LocalHostDeleteCollectionParams contains all the parameters to send to the API endpoint

	for the local host delete collection operation.

	Typically these are written to a http.Request.
*/
type LocalHostDeleteCollectionParams struct {

	/* Address.

	   Filter by address
	*/
	Address *string

	/* Aliases.

	   Filter by aliases
	*/
	Aliases *string

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* Hostname.

	   Filter by hostname
	*/
	Hostname *string

	/* Info.

	   Info specification
	*/
	Info LocalHostDeleteCollectionBody

	/* OwnerName.

	   Filter by owner.name
	*/
	OwnerName *string

	/* OwnerUUID.

	   Filter by owner.uuid
	*/
	OwnerUUID *string

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

	/* Scope.

	   Filter by scope
	*/
	Scope *string

	/* SerialRecords.

	   Perform the operation on the records synchronously.
	*/
	SerialRecords *bool

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the local host delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *LocalHostDeleteCollectionParams) WithDefaults() *LocalHostDeleteCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the local host delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *LocalHostDeleteCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := LocalHostDeleteCollectionParams{
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

// WithTimeout adds the timeout to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) WithTimeout(timeout time.Duration) *LocalHostDeleteCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) WithContext(ctx context.Context) *LocalHostDeleteCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) WithHTTPClient(client *http.Client) *LocalHostDeleteCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithAddress adds the address to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) WithAddress(address *string) *LocalHostDeleteCollectionParams {
	o.SetAddress(address)
	return o
}

// SetAddress adds the address to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) SetAddress(address *string) {
	o.Address = address
}

// WithAliases adds the aliases to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) WithAliases(aliases *string) *LocalHostDeleteCollectionParams {
	o.SetAliases(aliases)
	return o
}

// SetAliases adds the aliases to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) SetAliases(aliases *string) {
	o.Aliases = aliases
}

// WithContinueOnFailure adds the continueOnFailure to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *LocalHostDeleteCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithHostname adds the hostname to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) WithHostname(hostname *string) *LocalHostDeleteCollectionParams {
	o.SetHostname(hostname)
	return o
}

// SetHostname adds the hostname to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) SetHostname(hostname *string) {
	o.Hostname = hostname
}

// WithInfo adds the info to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) WithInfo(info LocalHostDeleteCollectionBody) *LocalHostDeleteCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) SetInfo(info LocalHostDeleteCollectionBody) {
	o.Info = info
}

// WithOwnerName adds the ownerName to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) WithOwnerName(ownerName *string) *LocalHostDeleteCollectionParams {
	o.SetOwnerName(ownerName)
	return o
}

// SetOwnerName adds the ownerName to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) SetOwnerName(ownerName *string) {
	o.OwnerName = ownerName
}

// WithOwnerUUID adds the ownerUUID to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) WithOwnerUUID(ownerUUID *string) *LocalHostDeleteCollectionParams {
	o.SetOwnerUUID(ownerUUID)
	return o
}

// SetOwnerUUID adds the ownerUuid to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) SetOwnerUUID(ownerUUID *string) {
	o.OwnerUUID = ownerUUID
}

// WithReturnRecords adds the returnRecords to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) WithReturnRecords(returnRecords *bool) *LocalHostDeleteCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) WithReturnTimeout(returnTimeout *int64) *LocalHostDeleteCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithScope adds the scope to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) WithScope(scope *string) *LocalHostDeleteCollectionParams {
	o.SetScope(scope)
	return o
}

// SetScope adds the scope to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) SetScope(scope *string) {
	o.Scope = scope
}

// WithSerialRecords adds the serialRecords to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) WithSerialRecords(serialRecords *bool) *LocalHostDeleteCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the local host delete collection params
func (o *LocalHostDeleteCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WriteToRequest writes these params to a swagger request
func (o *LocalHostDeleteCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.Address != nil {

		// query param address
		var qrAddress string

		if o.Address != nil {
			qrAddress = *o.Address
		}
		qAddress := qrAddress
		if qAddress != "" {

			if err := r.SetQueryParam("address", qAddress); err != nil {
				return err
			}
		}
	}

	if o.Aliases != nil {

		// query param aliases
		var qrAliases string

		if o.Aliases != nil {
			qrAliases = *o.Aliases
		}
		qAliases := qrAliases
		if qAliases != "" {

			if err := r.SetQueryParam("aliases", qAliases); err != nil {
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

	if o.Hostname != nil {

		// query param hostname
		var qrHostname string

		if o.Hostname != nil {
			qrHostname = *o.Hostname
		}
		qHostname := qrHostname
		if qHostname != "" {

			if err := r.SetQueryParam("hostname", qHostname); err != nil {
				return err
			}
		}
	}
	if err := r.SetBodyParam(o.Info); err != nil {
		return err
	}

	if o.OwnerName != nil {

		// query param owner.name
		var qrOwnerName string

		if o.OwnerName != nil {
			qrOwnerName = *o.OwnerName
		}
		qOwnerName := qrOwnerName
		if qOwnerName != "" {

			if err := r.SetQueryParam("owner.name", qOwnerName); err != nil {
				return err
			}
		}
	}

	if o.OwnerUUID != nil {

		// query param owner.uuid
		var qrOwnerUUID string

		if o.OwnerUUID != nil {
			qrOwnerUUID = *o.OwnerUUID
		}
		qOwnerUUID := qrOwnerUUID
		if qOwnerUUID != "" {

			if err := r.SetQueryParam("owner.uuid", qOwnerUUID); err != nil {
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

	if o.Scope != nil {

		// query param scope
		var qrScope string

		if o.Scope != nil {
			qrScope = *o.Scope
		}
		qScope := qrScope
		if qScope != "" {

			if err := r.SetQueryParam("scope", qScope); err != nil {
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
