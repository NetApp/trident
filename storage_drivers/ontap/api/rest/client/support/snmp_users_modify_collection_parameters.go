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

// NewSnmpUsersModifyCollectionParams creates a new SnmpUsersModifyCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewSnmpUsersModifyCollectionParams() *SnmpUsersModifyCollectionParams {
	return &SnmpUsersModifyCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewSnmpUsersModifyCollectionParamsWithTimeout creates a new SnmpUsersModifyCollectionParams object
// with the ability to set a timeout on a request.
func NewSnmpUsersModifyCollectionParamsWithTimeout(timeout time.Duration) *SnmpUsersModifyCollectionParams {
	return &SnmpUsersModifyCollectionParams{
		timeout: timeout,
	}
}

// NewSnmpUsersModifyCollectionParamsWithContext creates a new SnmpUsersModifyCollectionParams object
// with the ability to set a context for a request.
func NewSnmpUsersModifyCollectionParamsWithContext(ctx context.Context) *SnmpUsersModifyCollectionParams {
	return &SnmpUsersModifyCollectionParams{
		Context: ctx,
	}
}

// NewSnmpUsersModifyCollectionParamsWithHTTPClient creates a new SnmpUsersModifyCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewSnmpUsersModifyCollectionParamsWithHTTPClient(client *http.Client) *SnmpUsersModifyCollectionParams {
	return &SnmpUsersModifyCollectionParams{
		HTTPClient: client,
	}
}

/*
SnmpUsersModifyCollectionParams contains all the parameters to send to the API endpoint

	for the snmp users modify collection operation.

	Typically these are written to a http.Request.
*/
type SnmpUsersModifyCollectionParams struct {

	/* AuthenticationMethod.

	   Filter by authentication_method
	*/
	AuthenticationMethod *string

	/* Comment.

	   Filter by comment
	*/
	Comment *string

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* EngineID.

	   Filter by engine_id
	*/
	EngineID *string

	/* Info.

	   Info specification
	*/
	Info SnmpUsersModifyCollectionBody

	/* Name.

	   Filter by name
	*/
	Name *string

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

	/* Snmpv3AuthenticationProtocol.

	   Filter by snmpv3.authentication_protocol
	*/
	Snmpv3AuthenticationProtocol *string

	/* Snmpv3PrivacyProtocol.

	   Filter by snmpv3.privacy_protocol
	*/
	Snmpv3PrivacyProtocol *string

	/* SwitchAddress.

	   Filter by switch_address
	*/
	SwitchAddress *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the snmp users modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *SnmpUsersModifyCollectionParams) WithDefaults() *SnmpUsersModifyCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the snmp users modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *SnmpUsersModifyCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := SnmpUsersModifyCollectionParams{
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

// WithTimeout adds the timeout to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) WithTimeout(timeout time.Duration) *SnmpUsersModifyCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) WithContext(ctx context.Context) *SnmpUsersModifyCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) WithHTTPClient(client *http.Client) *SnmpUsersModifyCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithAuthenticationMethod adds the authenticationMethod to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) WithAuthenticationMethod(authenticationMethod *string) *SnmpUsersModifyCollectionParams {
	o.SetAuthenticationMethod(authenticationMethod)
	return o
}

// SetAuthenticationMethod adds the authenticationMethod to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) SetAuthenticationMethod(authenticationMethod *string) {
	o.AuthenticationMethod = authenticationMethod
}

// WithComment adds the comment to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) WithComment(comment *string) *SnmpUsersModifyCollectionParams {
	o.SetComment(comment)
	return o
}

// SetComment adds the comment to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) SetComment(comment *string) {
	o.Comment = comment
}

// WithContinueOnFailure adds the continueOnFailure to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *SnmpUsersModifyCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithEngineID adds the engineID to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) WithEngineID(engineID *string) *SnmpUsersModifyCollectionParams {
	o.SetEngineID(engineID)
	return o
}

// SetEngineID adds the engineId to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) SetEngineID(engineID *string) {
	o.EngineID = engineID
}

// WithInfo adds the info to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) WithInfo(info SnmpUsersModifyCollectionBody) *SnmpUsersModifyCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) SetInfo(info SnmpUsersModifyCollectionBody) {
	o.Info = info
}

// WithName adds the name to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) WithName(name *string) *SnmpUsersModifyCollectionParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) SetName(name *string) {
	o.Name = name
}

// WithOwnerName adds the ownerName to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) WithOwnerName(ownerName *string) *SnmpUsersModifyCollectionParams {
	o.SetOwnerName(ownerName)
	return o
}

// SetOwnerName adds the ownerName to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) SetOwnerName(ownerName *string) {
	o.OwnerName = ownerName
}

// WithOwnerUUID adds the ownerUUID to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) WithOwnerUUID(ownerUUID *string) *SnmpUsersModifyCollectionParams {
	o.SetOwnerUUID(ownerUUID)
	return o
}

// SetOwnerUUID adds the ownerUuid to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) SetOwnerUUID(ownerUUID *string) {
	o.OwnerUUID = ownerUUID
}

// WithReturnRecords adds the returnRecords to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) WithReturnRecords(returnRecords *bool) *SnmpUsersModifyCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) WithReturnTimeout(returnTimeout *int64) *SnmpUsersModifyCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithScope adds the scope to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) WithScope(scope *string) *SnmpUsersModifyCollectionParams {
	o.SetScope(scope)
	return o
}

// SetScope adds the scope to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) SetScope(scope *string) {
	o.Scope = scope
}

// WithSerialRecords adds the serialRecords to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) WithSerialRecords(serialRecords *bool) *SnmpUsersModifyCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WithSnmpv3AuthenticationProtocol adds the snmpv3AuthenticationProtocol to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) WithSnmpv3AuthenticationProtocol(snmpv3AuthenticationProtocol *string) *SnmpUsersModifyCollectionParams {
	o.SetSnmpv3AuthenticationProtocol(snmpv3AuthenticationProtocol)
	return o
}

// SetSnmpv3AuthenticationProtocol adds the snmpv3AuthenticationProtocol to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) SetSnmpv3AuthenticationProtocol(snmpv3AuthenticationProtocol *string) {
	o.Snmpv3AuthenticationProtocol = snmpv3AuthenticationProtocol
}

// WithSnmpv3PrivacyProtocol adds the snmpv3PrivacyProtocol to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) WithSnmpv3PrivacyProtocol(snmpv3PrivacyProtocol *string) *SnmpUsersModifyCollectionParams {
	o.SetSnmpv3PrivacyProtocol(snmpv3PrivacyProtocol)
	return o
}

// SetSnmpv3PrivacyProtocol adds the snmpv3PrivacyProtocol to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) SetSnmpv3PrivacyProtocol(snmpv3PrivacyProtocol *string) {
	o.Snmpv3PrivacyProtocol = snmpv3PrivacyProtocol
}

// WithSwitchAddress adds the switchAddress to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) WithSwitchAddress(switchAddress *string) *SnmpUsersModifyCollectionParams {
	o.SetSwitchAddress(switchAddress)
	return o
}

// SetSwitchAddress adds the switchAddress to the snmp users modify collection params
func (o *SnmpUsersModifyCollectionParams) SetSwitchAddress(switchAddress *string) {
	o.SwitchAddress = switchAddress
}

// WriteToRequest writes these params to a swagger request
func (o *SnmpUsersModifyCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.AuthenticationMethod != nil {

		// query param authentication_method
		var qrAuthenticationMethod string

		if o.AuthenticationMethod != nil {
			qrAuthenticationMethod = *o.AuthenticationMethod
		}
		qAuthenticationMethod := qrAuthenticationMethod
		if qAuthenticationMethod != "" {

			if err := r.SetQueryParam("authentication_method", qAuthenticationMethod); err != nil {
				return err
			}
		}
	}

	if o.Comment != nil {

		// query param comment
		var qrComment string

		if o.Comment != nil {
			qrComment = *o.Comment
		}
		qComment := qrComment
		if qComment != "" {

			if err := r.SetQueryParam("comment", qComment); err != nil {
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

	if o.EngineID != nil {

		// query param engine_id
		var qrEngineID string

		if o.EngineID != nil {
			qrEngineID = *o.EngineID
		}
		qEngineID := qrEngineID
		if qEngineID != "" {

			if err := r.SetQueryParam("engine_id", qEngineID); err != nil {
				return err
			}
		}
	}
	if err := r.SetBodyParam(o.Info); err != nil {
		return err
	}

	if o.Name != nil {

		// query param name
		var qrName string

		if o.Name != nil {
			qrName = *o.Name
		}
		qName := qrName
		if qName != "" {

			if err := r.SetQueryParam("name", qName); err != nil {
				return err
			}
		}
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

	if o.Snmpv3AuthenticationProtocol != nil {

		// query param snmpv3.authentication_protocol
		var qrSnmpv3AuthenticationProtocol string

		if o.Snmpv3AuthenticationProtocol != nil {
			qrSnmpv3AuthenticationProtocol = *o.Snmpv3AuthenticationProtocol
		}
		qSnmpv3AuthenticationProtocol := qrSnmpv3AuthenticationProtocol
		if qSnmpv3AuthenticationProtocol != "" {

			if err := r.SetQueryParam("snmpv3.authentication_protocol", qSnmpv3AuthenticationProtocol); err != nil {
				return err
			}
		}
	}

	if o.Snmpv3PrivacyProtocol != nil {

		// query param snmpv3.privacy_protocol
		var qrSnmpv3PrivacyProtocol string

		if o.Snmpv3PrivacyProtocol != nil {
			qrSnmpv3PrivacyProtocol = *o.Snmpv3PrivacyProtocol
		}
		qSnmpv3PrivacyProtocol := qrSnmpv3PrivacyProtocol
		if qSnmpv3PrivacyProtocol != "" {

			if err := r.SetQueryParam("snmpv3.privacy_protocol", qSnmpv3PrivacyProtocol); err != nil {
				return err
			}
		}
	}

	if o.SwitchAddress != nil {

		// query param switch_address
		var qrSwitchAddress string

		if o.SwitchAddress != nil {
			qrSwitchAddress = *o.SwitchAddress
		}
		qSwitchAddress := qrSwitchAddress
		if qSwitchAddress != "" {

			if err := r.SetQueryParam("switch_address", qSwitchAddress); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
