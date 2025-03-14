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

// NewEmsRoleConfigDeleteCollectionParams creates a new EmsRoleConfigDeleteCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewEmsRoleConfigDeleteCollectionParams() *EmsRoleConfigDeleteCollectionParams {
	return &EmsRoleConfigDeleteCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewEmsRoleConfigDeleteCollectionParamsWithTimeout creates a new EmsRoleConfigDeleteCollectionParams object
// with the ability to set a timeout on a request.
func NewEmsRoleConfigDeleteCollectionParamsWithTimeout(timeout time.Duration) *EmsRoleConfigDeleteCollectionParams {
	return &EmsRoleConfigDeleteCollectionParams{
		timeout: timeout,
	}
}

// NewEmsRoleConfigDeleteCollectionParamsWithContext creates a new EmsRoleConfigDeleteCollectionParams object
// with the ability to set a context for a request.
func NewEmsRoleConfigDeleteCollectionParamsWithContext(ctx context.Context) *EmsRoleConfigDeleteCollectionParams {
	return &EmsRoleConfigDeleteCollectionParams{
		Context: ctx,
	}
}

// NewEmsRoleConfigDeleteCollectionParamsWithHTTPClient creates a new EmsRoleConfigDeleteCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewEmsRoleConfigDeleteCollectionParamsWithHTTPClient(client *http.Client) *EmsRoleConfigDeleteCollectionParams {
	return &EmsRoleConfigDeleteCollectionParams{
		HTTPClient: client,
	}
}

/*
EmsRoleConfigDeleteCollectionParams contains all the parameters to send to the API endpoint

	for the ems role config delete collection operation.

	Typically these are written to a http.Request.
*/
type EmsRoleConfigDeleteCollectionParams struct {

	/* AccessControlRoleName.

	   Filter by access_control_role.name
	*/
	AccessControlRoleName *string

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* EventFilterName.

	   Filter by event_filter.name
	*/
	EventFilterName *string

	/* Info.

	   Info specification
	*/
	Info EmsRoleConfigDeleteCollectionBody

	/* LimitAccessToGlobalConfigs.

	   Filter by limit_access_to_global_configs
	*/
	LimitAccessToGlobalConfigs *bool

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

// WithDefaults hydrates default values in the ems role config delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *EmsRoleConfigDeleteCollectionParams) WithDefaults() *EmsRoleConfigDeleteCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the ems role config delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *EmsRoleConfigDeleteCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := EmsRoleConfigDeleteCollectionParams{
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

// WithTimeout adds the timeout to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) WithTimeout(timeout time.Duration) *EmsRoleConfigDeleteCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) WithContext(ctx context.Context) *EmsRoleConfigDeleteCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) WithHTTPClient(client *http.Client) *EmsRoleConfigDeleteCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithAccessControlRoleName adds the accessControlRoleName to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) WithAccessControlRoleName(accessControlRoleName *string) *EmsRoleConfigDeleteCollectionParams {
	o.SetAccessControlRoleName(accessControlRoleName)
	return o
}

// SetAccessControlRoleName adds the accessControlRoleName to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) SetAccessControlRoleName(accessControlRoleName *string) {
	o.AccessControlRoleName = accessControlRoleName
}

// WithContinueOnFailure adds the continueOnFailure to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *EmsRoleConfigDeleteCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithEventFilterName adds the eventFilterName to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) WithEventFilterName(eventFilterName *string) *EmsRoleConfigDeleteCollectionParams {
	o.SetEventFilterName(eventFilterName)
	return o
}

// SetEventFilterName adds the eventFilterName to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) SetEventFilterName(eventFilterName *string) {
	o.EventFilterName = eventFilterName
}

// WithInfo adds the info to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) WithInfo(info EmsRoleConfigDeleteCollectionBody) *EmsRoleConfigDeleteCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) SetInfo(info EmsRoleConfigDeleteCollectionBody) {
	o.Info = info
}

// WithLimitAccessToGlobalConfigs adds the limitAccessToGlobalConfigs to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) WithLimitAccessToGlobalConfigs(limitAccessToGlobalConfigs *bool) *EmsRoleConfigDeleteCollectionParams {
	o.SetLimitAccessToGlobalConfigs(limitAccessToGlobalConfigs)
	return o
}

// SetLimitAccessToGlobalConfigs adds the limitAccessToGlobalConfigs to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) SetLimitAccessToGlobalConfigs(limitAccessToGlobalConfigs *bool) {
	o.LimitAccessToGlobalConfigs = limitAccessToGlobalConfigs
}

// WithReturnRecords adds the returnRecords to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) WithReturnRecords(returnRecords *bool) *EmsRoleConfigDeleteCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) WithReturnTimeout(returnTimeout *int64) *EmsRoleConfigDeleteCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithSerialRecords adds the serialRecords to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) WithSerialRecords(serialRecords *bool) *EmsRoleConfigDeleteCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the ems role config delete collection params
func (o *EmsRoleConfigDeleteCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WriteToRequest writes these params to a swagger request
func (o *EmsRoleConfigDeleteCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.AccessControlRoleName != nil {

		// query param access_control_role.name
		var qrAccessControlRoleName string

		if o.AccessControlRoleName != nil {
			qrAccessControlRoleName = *o.AccessControlRoleName
		}
		qAccessControlRoleName := qrAccessControlRoleName
		if qAccessControlRoleName != "" {

			if err := r.SetQueryParam("access_control_role.name", qAccessControlRoleName); err != nil {
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

	if o.EventFilterName != nil {

		// query param event_filter.name
		var qrEventFilterName string

		if o.EventFilterName != nil {
			qrEventFilterName = *o.EventFilterName
		}
		qEventFilterName := qrEventFilterName
		if qEventFilterName != "" {

			if err := r.SetQueryParam("event_filter.name", qEventFilterName); err != nil {
				return err
			}
		}
	}
	if err := r.SetBodyParam(o.Info); err != nil {
		return err
	}

	if o.LimitAccessToGlobalConfigs != nil {

		// query param limit_access_to_global_configs
		var qrLimitAccessToGlobalConfigs bool

		if o.LimitAccessToGlobalConfigs != nil {
			qrLimitAccessToGlobalConfigs = *o.LimitAccessToGlobalConfigs
		}
		qLimitAccessToGlobalConfigs := swag.FormatBool(qrLimitAccessToGlobalConfigs)
		if qLimitAccessToGlobalConfigs != "" {

			if err := r.SetQueryParam("limit_access_to_global_configs", qLimitAccessToGlobalConfigs); err != nil {
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

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
