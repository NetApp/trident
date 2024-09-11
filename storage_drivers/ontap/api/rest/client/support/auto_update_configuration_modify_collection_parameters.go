// Code generated by go-swagger; DO NOT EDIT.

package support

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

// NewAutoUpdateConfigurationModifyCollectionParams creates a new AutoUpdateConfigurationModifyCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewAutoUpdateConfigurationModifyCollectionParams() *AutoUpdateConfigurationModifyCollectionParams {
	return &AutoUpdateConfigurationModifyCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewAutoUpdateConfigurationModifyCollectionParamsWithTimeout creates a new AutoUpdateConfigurationModifyCollectionParams object
// with the ability to set a timeout on a request.
func NewAutoUpdateConfigurationModifyCollectionParamsWithTimeout(timeout time.Duration) *AutoUpdateConfigurationModifyCollectionParams {
	return &AutoUpdateConfigurationModifyCollectionParams{
		timeout: timeout,
	}
}

// NewAutoUpdateConfigurationModifyCollectionParamsWithContext creates a new AutoUpdateConfigurationModifyCollectionParams object
// with the ability to set a context for a request.
func NewAutoUpdateConfigurationModifyCollectionParamsWithContext(ctx context.Context) *AutoUpdateConfigurationModifyCollectionParams {
	return &AutoUpdateConfigurationModifyCollectionParams{
		Context: ctx,
	}
}

// NewAutoUpdateConfigurationModifyCollectionParamsWithHTTPClient creates a new AutoUpdateConfigurationModifyCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewAutoUpdateConfigurationModifyCollectionParamsWithHTTPClient(client *http.Client) *AutoUpdateConfigurationModifyCollectionParams {
	return &AutoUpdateConfigurationModifyCollectionParams{
		HTTPClient: client,
	}
}

/*
AutoUpdateConfigurationModifyCollectionParams contains all the parameters to send to the API endpoint

	for the auto update configuration modify collection operation.

	Typically these are written to a http.Request.
*/
type AutoUpdateConfigurationModifyCollectionParams struct {

	/* Action.

	   Filter by action
	*/
	Action *string

	/* Category.

	   Filter by category
	*/
	Category *string

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* DescriptionCode.

	   Filter by description.code
	*/
	DescriptionCode *string

	/* DescriptionMessage.

	   Filter by description.message
	*/
	DescriptionMessage *string

	/* Info.

	   Info specification
	*/
	Info AutoUpdateConfigurationModifyCollectionBody

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

// WithDefaults hydrates default values in the auto update configuration modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *AutoUpdateConfigurationModifyCollectionParams) WithDefaults() *AutoUpdateConfigurationModifyCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the auto update configuration modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *AutoUpdateConfigurationModifyCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := AutoUpdateConfigurationModifyCollectionParams{
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

// WithTimeout adds the timeout to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) WithTimeout(timeout time.Duration) *AutoUpdateConfigurationModifyCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) WithContext(ctx context.Context) *AutoUpdateConfigurationModifyCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) WithHTTPClient(client *http.Client) *AutoUpdateConfigurationModifyCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithAction adds the action to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) WithAction(action *string) *AutoUpdateConfigurationModifyCollectionParams {
	o.SetAction(action)
	return o
}

// SetAction adds the action to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) SetAction(action *string) {
	o.Action = action
}

// WithCategory adds the category to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) WithCategory(category *string) *AutoUpdateConfigurationModifyCollectionParams {
	o.SetCategory(category)
	return o
}

// SetCategory adds the category to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) SetCategory(category *string) {
	o.Category = category
}

// WithContinueOnFailure adds the continueOnFailure to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *AutoUpdateConfigurationModifyCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithDescriptionCode adds the descriptionCode to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) WithDescriptionCode(descriptionCode *string) *AutoUpdateConfigurationModifyCollectionParams {
	o.SetDescriptionCode(descriptionCode)
	return o
}

// SetDescriptionCode adds the descriptionCode to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) SetDescriptionCode(descriptionCode *string) {
	o.DescriptionCode = descriptionCode
}

// WithDescriptionMessage adds the descriptionMessage to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) WithDescriptionMessage(descriptionMessage *string) *AutoUpdateConfigurationModifyCollectionParams {
	o.SetDescriptionMessage(descriptionMessage)
	return o
}

// SetDescriptionMessage adds the descriptionMessage to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) SetDescriptionMessage(descriptionMessage *string) {
	o.DescriptionMessage = descriptionMessage
}

// WithInfo adds the info to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) WithInfo(info AutoUpdateConfigurationModifyCollectionBody) *AutoUpdateConfigurationModifyCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) SetInfo(info AutoUpdateConfigurationModifyCollectionBody) {
	o.Info = info
}

// WithReturnRecords adds the returnRecords to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) WithReturnRecords(returnRecords *bool) *AutoUpdateConfigurationModifyCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) WithReturnTimeout(returnTimeout *int64) *AutoUpdateConfigurationModifyCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithSerialRecords adds the serialRecords to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) WithSerialRecords(serialRecords *bool) *AutoUpdateConfigurationModifyCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WithUUID adds the uuid to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) WithUUID(uuid *string) *AutoUpdateConfigurationModifyCollectionParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the auto update configuration modify collection params
func (o *AutoUpdateConfigurationModifyCollectionParams) SetUUID(uuid *string) {
	o.UUID = uuid
}

// WriteToRequest writes these params to a swagger request
func (o *AutoUpdateConfigurationModifyCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.Action != nil {

		// query param action
		var qrAction string

		if o.Action != nil {
			qrAction = *o.Action
		}
		qAction := qrAction
		if qAction != "" {

			if err := r.SetQueryParam("action", qAction); err != nil {
				return err
			}
		}
	}

	if o.Category != nil {

		// query param category
		var qrCategory string

		if o.Category != nil {
			qrCategory = *o.Category
		}
		qCategory := qrCategory
		if qCategory != "" {

			if err := r.SetQueryParam("category", qCategory); err != nil {
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

	if o.DescriptionCode != nil {

		// query param description.code
		var qrDescriptionCode string

		if o.DescriptionCode != nil {
			qrDescriptionCode = *o.DescriptionCode
		}
		qDescriptionCode := qrDescriptionCode
		if qDescriptionCode != "" {

			if err := r.SetQueryParam("description.code", qDescriptionCode); err != nil {
				return err
			}
		}
	}

	if o.DescriptionMessage != nil {

		// query param description.message
		var qrDescriptionMessage string

		if o.DescriptionMessage != nil {
			qrDescriptionMessage = *o.DescriptionMessage
		}
		qDescriptionMessage := qrDescriptionMessage
		if qDescriptionMessage != "" {

			if err := r.SetQueryParam("description.message", qDescriptionMessage); err != nil {
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