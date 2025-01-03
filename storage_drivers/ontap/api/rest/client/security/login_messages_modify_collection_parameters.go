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

// NewLoginMessagesModifyCollectionParams creates a new LoginMessagesModifyCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewLoginMessagesModifyCollectionParams() *LoginMessagesModifyCollectionParams {
	return &LoginMessagesModifyCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewLoginMessagesModifyCollectionParamsWithTimeout creates a new LoginMessagesModifyCollectionParams object
// with the ability to set a timeout on a request.
func NewLoginMessagesModifyCollectionParamsWithTimeout(timeout time.Duration) *LoginMessagesModifyCollectionParams {
	return &LoginMessagesModifyCollectionParams{
		timeout: timeout,
	}
}

// NewLoginMessagesModifyCollectionParamsWithContext creates a new LoginMessagesModifyCollectionParams object
// with the ability to set a context for a request.
func NewLoginMessagesModifyCollectionParamsWithContext(ctx context.Context) *LoginMessagesModifyCollectionParams {
	return &LoginMessagesModifyCollectionParams{
		Context: ctx,
	}
}

// NewLoginMessagesModifyCollectionParamsWithHTTPClient creates a new LoginMessagesModifyCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewLoginMessagesModifyCollectionParamsWithHTTPClient(client *http.Client) *LoginMessagesModifyCollectionParams {
	return &LoginMessagesModifyCollectionParams{
		HTTPClient: client,
	}
}

/*
LoginMessagesModifyCollectionParams contains all the parameters to send to the API endpoint

	for the login messages modify collection operation.

	Typically these are written to a http.Request.
*/
type LoginMessagesModifyCollectionParams struct {

	/* Banner.

	   Filter by banner
	*/
	Banner *string

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* Info.

	   Info specification
	*/
	Info LoginMessagesModifyCollectionBody

	/* Message.

	   Filter by message
	*/
	Message *string

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

	/* ShowClusterMessage.

	   Filter by show_cluster_message
	*/
	ShowClusterMessage *bool

	/* SvmName.

	   Filter by svm.name
	*/
	SvmName *string

	/* SvmUUID.

	   Filter by svm.uuid
	*/
	SvmUUID *string

	/* UUID.

	   Filter by uuid
	*/
	UUID *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the login messages modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *LoginMessagesModifyCollectionParams) WithDefaults() *LoginMessagesModifyCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the login messages modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *LoginMessagesModifyCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := LoginMessagesModifyCollectionParams{
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

// WithTimeout adds the timeout to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) WithTimeout(timeout time.Duration) *LoginMessagesModifyCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) WithContext(ctx context.Context) *LoginMessagesModifyCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) WithHTTPClient(client *http.Client) *LoginMessagesModifyCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithBanner adds the banner to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) WithBanner(banner *string) *LoginMessagesModifyCollectionParams {
	o.SetBanner(banner)
	return o
}

// SetBanner adds the banner to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) SetBanner(banner *string) {
	o.Banner = banner
}

// WithContinueOnFailure adds the continueOnFailure to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *LoginMessagesModifyCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithInfo adds the info to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) WithInfo(info LoginMessagesModifyCollectionBody) *LoginMessagesModifyCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) SetInfo(info LoginMessagesModifyCollectionBody) {
	o.Info = info
}

// WithMessage adds the message to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) WithMessage(message *string) *LoginMessagesModifyCollectionParams {
	o.SetMessage(message)
	return o
}

// SetMessage adds the message to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) SetMessage(message *string) {
	o.Message = message
}

// WithReturnRecords adds the returnRecords to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) WithReturnRecords(returnRecords *bool) *LoginMessagesModifyCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) WithReturnTimeout(returnTimeout *int64) *LoginMessagesModifyCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithScope adds the scope to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) WithScope(scope *string) *LoginMessagesModifyCollectionParams {
	o.SetScope(scope)
	return o
}

// SetScope adds the scope to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) SetScope(scope *string) {
	o.Scope = scope
}

// WithSerialRecords adds the serialRecords to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) WithSerialRecords(serialRecords *bool) *LoginMessagesModifyCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WithShowClusterMessage adds the showClusterMessage to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) WithShowClusterMessage(showClusterMessage *bool) *LoginMessagesModifyCollectionParams {
	o.SetShowClusterMessage(showClusterMessage)
	return o
}

// SetShowClusterMessage adds the showClusterMessage to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) SetShowClusterMessage(showClusterMessage *bool) {
	o.ShowClusterMessage = showClusterMessage
}

// WithSvmName adds the svmName to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) WithSvmName(svmName *string) *LoginMessagesModifyCollectionParams {
	o.SetSvmName(svmName)
	return o
}

// SetSvmName adds the svmName to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) SetSvmName(svmName *string) {
	o.SvmName = svmName
}

// WithSvmUUID adds the svmUUID to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) WithSvmUUID(svmUUID *string) *LoginMessagesModifyCollectionParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) SetSvmUUID(svmUUID *string) {
	o.SvmUUID = svmUUID
}

// WithUUID adds the uuid to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) WithUUID(uuid *string) *LoginMessagesModifyCollectionParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the login messages modify collection params
func (o *LoginMessagesModifyCollectionParams) SetUUID(uuid *string) {
	o.UUID = uuid
}

// WriteToRequest writes these params to a swagger request
func (o *LoginMessagesModifyCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.Banner != nil {

		// query param banner
		var qrBanner string

		if o.Banner != nil {
			qrBanner = *o.Banner
		}
		qBanner := qrBanner
		if qBanner != "" {

			if err := r.SetQueryParam("banner", qBanner); err != nil {
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
	if err := r.SetBodyParam(o.Info); err != nil {
		return err
	}

	if o.Message != nil {

		// query param message
		var qrMessage string

		if o.Message != nil {
			qrMessage = *o.Message
		}
		qMessage := qrMessage
		if qMessage != "" {

			if err := r.SetQueryParam("message", qMessage); err != nil {
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

	if o.ShowClusterMessage != nil {

		// query param show_cluster_message
		var qrShowClusterMessage bool

		if o.ShowClusterMessage != nil {
			qrShowClusterMessage = *o.ShowClusterMessage
		}
		qShowClusterMessage := swag.FormatBool(qrShowClusterMessage)
		if qShowClusterMessage != "" {

			if err := r.SetQueryParam("show_cluster_message", qShowClusterMessage); err != nil {
				return err
			}
		}
	}

	if o.SvmName != nil {

		// query param svm.name
		var qrSvmName string

		if o.SvmName != nil {
			qrSvmName = *o.SvmName
		}
		qSvmName := qrSvmName
		if qSvmName != "" {

			if err := r.SetQueryParam("svm.name", qSvmName); err != nil {
				return err
			}
		}
	}

	if o.SvmUUID != nil {

		// query param svm.uuid
		var qrSvmUUID string

		if o.SvmUUID != nil {
			qrSvmUUID = *o.SvmUUID
		}
		qSvmUUID := qrSvmUUID
		if qSvmUUID != "" {

			if err := r.SetQueryParam("svm.uuid", qSvmUUID); err != nil {
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
