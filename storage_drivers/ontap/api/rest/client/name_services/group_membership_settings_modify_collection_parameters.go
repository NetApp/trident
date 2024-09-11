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

// NewGroupMembershipSettingsModifyCollectionParams creates a new GroupMembershipSettingsModifyCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewGroupMembershipSettingsModifyCollectionParams() *GroupMembershipSettingsModifyCollectionParams {
	return &GroupMembershipSettingsModifyCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewGroupMembershipSettingsModifyCollectionParamsWithTimeout creates a new GroupMembershipSettingsModifyCollectionParams object
// with the ability to set a timeout on a request.
func NewGroupMembershipSettingsModifyCollectionParamsWithTimeout(timeout time.Duration) *GroupMembershipSettingsModifyCollectionParams {
	return &GroupMembershipSettingsModifyCollectionParams{
		timeout: timeout,
	}
}

// NewGroupMembershipSettingsModifyCollectionParamsWithContext creates a new GroupMembershipSettingsModifyCollectionParams object
// with the ability to set a context for a request.
func NewGroupMembershipSettingsModifyCollectionParamsWithContext(ctx context.Context) *GroupMembershipSettingsModifyCollectionParams {
	return &GroupMembershipSettingsModifyCollectionParams{
		Context: ctx,
	}
}

// NewGroupMembershipSettingsModifyCollectionParamsWithHTTPClient creates a new GroupMembershipSettingsModifyCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewGroupMembershipSettingsModifyCollectionParamsWithHTTPClient(client *http.Client) *GroupMembershipSettingsModifyCollectionParams {
	return &GroupMembershipSettingsModifyCollectionParams{
		HTTPClient: client,
	}
}

/*
GroupMembershipSettingsModifyCollectionParams contains all the parameters to send to the API endpoint

	for the group membership settings modify collection operation.

	Typically these are written to a http.Request.
*/
type GroupMembershipSettingsModifyCollectionParams struct {

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* Enabled.

	   Filter by enabled
	*/
	Enabled *bool

	/* Info.

	   Info specification
	*/
	Info GroupMembershipSettingsModifyCollectionBody

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

	/* SvmName.

	   Filter by svm.name
	*/
	SvmName *string

	/* SvmUUID.

	   Filter by svm.uuid
	*/
	SvmUUID *string

	/* TTL.

	   Filter by ttl
	*/
	TTL *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the group membership settings modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *GroupMembershipSettingsModifyCollectionParams) WithDefaults() *GroupMembershipSettingsModifyCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the group membership settings modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *GroupMembershipSettingsModifyCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := GroupMembershipSettingsModifyCollectionParams{
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

// WithTimeout adds the timeout to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) WithTimeout(timeout time.Duration) *GroupMembershipSettingsModifyCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) WithContext(ctx context.Context) *GroupMembershipSettingsModifyCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) WithHTTPClient(client *http.Client) *GroupMembershipSettingsModifyCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithContinueOnFailure adds the continueOnFailure to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *GroupMembershipSettingsModifyCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithEnabled adds the enabled to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) WithEnabled(enabled *bool) *GroupMembershipSettingsModifyCollectionParams {
	o.SetEnabled(enabled)
	return o
}

// SetEnabled adds the enabled to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) SetEnabled(enabled *bool) {
	o.Enabled = enabled
}

// WithInfo adds the info to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) WithInfo(info GroupMembershipSettingsModifyCollectionBody) *GroupMembershipSettingsModifyCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) SetInfo(info GroupMembershipSettingsModifyCollectionBody) {
	o.Info = info
}

// WithReturnRecords adds the returnRecords to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) WithReturnRecords(returnRecords *bool) *GroupMembershipSettingsModifyCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) WithReturnTimeout(returnTimeout *int64) *GroupMembershipSettingsModifyCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithSerialRecords adds the serialRecords to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) WithSerialRecords(serialRecords *bool) *GroupMembershipSettingsModifyCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WithSvmName adds the svmName to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) WithSvmName(svmName *string) *GroupMembershipSettingsModifyCollectionParams {
	o.SetSvmName(svmName)
	return o
}

// SetSvmName adds the svmName to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) SetSvmName(svmName *string) {
	o.SvmName = svmName
}

// WithSvmUUID adds the svmUUID to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) WithSvmUUID(svmUUID *string) *GroupMembershipSettingsModifyCollectionParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) SetSvmUUID(svmUUID *string) {
	o.SvmUUID = svmUUID
}

// WithTTL adds the ttl to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) WithTTL(ttl *string) *GroupMembershipSettingsModifyCollectionParams {
	o.SetTTL(ttl)
	return o
}

// SetTTL adds the ttl to the group membership settings modify collection params
func (o *GroupMembershipSettingsModifyCollectionParams) SetTTL(ttl *string) {
	o.TTL = ttl
}

// WriteToRequest writes these params to a swagger request
func (o *GroupMembershipSettingsModifyCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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

	if o.Enabled != nil {

		// query param enabled
		var qrEnabled bool

		if o.Enabled != nil {
			qrEnabled = *o.Enabled
		}
		qEnabled := swag.FormatBool(qrEnabled)
		if qEnabled != "" {

			if err := r.SetQueryParam("enabled", qEnabled); err != nil {
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

	if o.TTL != nil {

		// query param ttl
		var qrTTL string

		if o.TTL != nil {
			qrTTL = *o.TTL
		}
		qTTL := qrTTL
		if qTTL != "" {

			if err := r.SetQueryParam("ttl", qTTL); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}