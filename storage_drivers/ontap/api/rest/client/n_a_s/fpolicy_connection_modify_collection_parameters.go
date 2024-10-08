// Code generated by go-swagger; DO NOT EDIT.

package n_a_s

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

// NewFpolicyConnectionModifyCollectionParams creates a new FpolicyConnectionModifyCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewFpolicyConnectionModifyCollectionParams() *FpolicyConnectionModifyCollectionParams {
	return &FpolicyConnectionModifyCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewFpolicyConnectionModifyCollectionParamsWithTimeout creates a new FpolicyConnectionModifyCollectionParams object
// with the ability to set a timeout on a request.
func NewFpolicyConnectionModifyCollectionParamsWithTimeout(timeout time.Duration) *FpolicyConnectionModifyCollectionParams {
	return &FpolicyConnectionModifyCollectionParams{
		timeout: timeout,
	}
}

// NewFpolicyConnectionModifyCollectionParamsWithContext creates a new FpolicyConnectionModifyCollectionParams object
// with the ability to set a context for a request.
func NewFpolicyConnectionModifyCollectionParamsWithContext(ctx context.Context) *FpolicyConnectionModifyCollectionParams {
	return &FpolicyConnectionModifyCollectionParams{
		Context: ctx,
	}
}

// NewFpolicyConnectionModifyCollectionParamsWithHTTPClient creates a new FpolicyConnectionModifyCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewFpolicyConnectionModifyCollectionParamsWithHTTPClient(client *http.Client) *FpolicyConnectionModifyCollectionParams {
	return &FpolicyConnectionModifyCollectionParams{
		HTTPClient: client,
	}
}

/*
FpolicyConnectionModifyCollectionParams contains all the parameters to send to the API endpoint

	for the fpolicy connection modify collection operation.

	Typically these are written to a http.Request.
*/
type FpolicyConnectionModifyCollectionParams struct {

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* DisconnectedReasonCode.

	   Filter by disconnected_reason.code
	*/
	DisconnectedReasonCode *int64

	/* DisconnectedReasonMessage.

	   Filter by disconnected_reason.message
	*/
	DisconnectedReasonMessage *string

	/* Info.

	   Info specification
	*/
	Info FpolicyConnectionModifyCollectionBody

	/* NodeName.

	   Filter by node.name
	*/
	NodeName *string

	/* NodeUUID.

	   Filter by node.uuid
	*/
	NodeUUID *string

	/* PassthroughRead.

	   Whether to view only passthrough-read connections
	*/
	PassthroughRead *bool

	/* PolicyName.

	   Filter by policy.name
	*/
	PolicyName *string

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

	/* Server.

	   Filter by server
	*/
	Server *string

	/* SessionUUID.

	   Filter by session_uuid
	*/
	SessionUUID *string

	/* State.

	   Filter by state
	*/
	State *string

	/* SvmName.

	   Filter by svm.name
	*/
	SvmName *string

	/* SvmUUID.

	   UUID of the SVM to which this object belongs.
	*/
	SvmUUID string

	/* Type.

	   Filter by type
	*/
	Type *string

	/* UpdateTime.

	   Filter by update_time
	*/
	UpdateTime *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the fpolicy connection modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *FpolicyConnectionModifyCollectionParams) WithDefaults() *FpolicyConnectionModifyCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the fpolicy connection modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *FpolicyConnectionModifyCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := FpolicyConnectionModifyCollectionParams{
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

// WithTimeout adds the timeout to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithTimeout(timeout time.Duration) *FpolicyConnectionModifyCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithContext(ctx context.Context) *FpolicyConnectionModifyCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithHTTPClient(client *http.Client) *FpolicyConnectionModifyCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithContinueOnFailure adds the continueOnFailure to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *FpolicyConnectionModifyCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithDisconnectedReasonCode adds the disconnectedReasonCode to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithDisconnectedReasonCode(disconnectedReasonCode *int64) *FpolicyConnectionModifyCollectionParams {
	o.SetDisconnectedReasonCode(disconnectedReasonCode)
	return o
}

// SetDisconnectedReasonCode adds the disconnectedReasonCode to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetDisconnectedReasonCode(disconnectedReasonCode *int64) {
	o.DisconnectedReasonCode = disconnectedReasonCode
}

// WithDisconnectedReasonMessage adds the disconnectedReasonMessage to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithDisconnectedReasonMessage(disconnectedReasonMessage *string) *FpolicyConnectionModifyCollectionParams {
	o.SetDisconnectedReasonMessage(disconnectedReasonMessage)
	return o
}

// SetDisconnectedReasonMessage adds the disconnectedReasonMessage to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetDisconnectedReasonMessage(disconnectedReasonMessage *string) {
	o.DisconnectedReasonMessage = disconnectedReasonMessage
}

// WithInfo adds the info to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithInfo(info FpolicyConnectionModifyCollectionBody) *FpolicyConnectionModifyCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetInfo(info FpolicyConnectionModifyCollectionBody) {
	o.Info = info
}

// WithNodeName adds the nodeName to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithNodeName(nodeName *string) *FpolicyConnectionModifyCollectionParams {
	o.SetNodeName(nodeName)
	return o
}

// SetNodeName adds the nodeName to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetNodeName(nodeName *string) {
	o.NodeName = nodeName
}

// WithNodeUUID adds the nodeUUID to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithNodeUUID(nodeUUID *string) *FpolicyConnectionModifyCollectionParams {
	o.SetNodeUUID(nodeUUID)
	return o
}

// SetNodeUUID adds the nodeUuid to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetNodeUUID(nodeUUID *string) {
	o.NodeUUID = nodeUUID
}

// WithPassthroughRead adds the passthroughRead to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithPassthroughRead(passthroughRead *bool) *FpolicyConnectionModifyCollectionParams {
	o.SetPassthroughRead(passthroughRead)
	return o
}

// SetPassthroughRead adds the passthroughRead to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetPassthroughRead(passthroughRead *bool) {
	o.PassthroughRead = passthroughRead
}

// WithPolicyName adds the policyName to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithPolicyName(policyName *string) *FpolicyConnectionModifyCollectionParams {
	o.SetPolicyName(policyName)
	return o
}

// SetPolicyName adds the policyName to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetPolicyName(policyName *string) {
	o.PolicyName = policyName
}

// WithReturnRecords adds the returnRecords to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithReturnRecords(returnRecords *bool) *FpolicyConnectionModifyCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithReturnTimeout(returnTimeout *int64) *FpolicyConnectionModifyCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithSerialRecords adds the serialRecords to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithSerialRecords(serialRecords *bool) *FpolicyConnectionModifyCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WithServer adds the server to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithServer(server *string) *FpolicyConnectionModifyCollectionParams {
	o.SetServer(server)
	return o
}

// SetServer adds the server to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetServer(server *string) {
	o.Server = server
}

// WithSessionUUID adds the sessionUUID to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithSessionUUID(sessionUUID *string) *FpolicyConnectionModifyCollectionParams {
	o.SetSessionUUID(sessionUUID)
	return o
}

// SetSessionUUID adds the sessionUuid to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetSessionUUID(sessionUUID *string) {
	o.SessionUUID = sessionUUID
}

// WithState adds the state to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithState(state *string) *FpolicyConnectionModifyCollectionParams {
	o.SetState(state)
	return o
}

// SetState adds the state to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetState(state *string) {
	o.State = state
}

// WithSvmName adds the svmName to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithSvmName(svmName *string) *FpolicyConnectionModifyCollectionParams {
	o.SetSvmName(svmName)
	return o
}

// SetSvmName adds the svmName to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetSvmName(svmName *string) {
	o.SvmName = svmName
}

// WithSvmUUID adds the svmUUID to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithSvmUUID(svmUUID string) *FpolicyConnectionModifyCollectionParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetSvmUUID(svmUUID string) {
	o.SvmUUID = svmUUID
}

// WithType adds the typeVar to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithType(typeVar *string) *FpolicyConnectionModifyCollectionParams {
	o.SetType(typeVar)
	return o
}

// SetType adds the type to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetType(typeVar *string) {
	o.Type = typeVar
}

// WithUpdateTime adds the updateTime to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) WithUpdateTime(updateTime *string) *FpolicyConnectionModifyCollectionParams {
	o.SetUpdateTime(updateTime)
	return o
}

// SetUpdateTime adds the updateTime to the fpolicy connection modify collection params
func (o *FpolicyConnectionModifyCollectionParams) SetUpdateTime(updateTime *string) {
	o.UpdateTime = updateTime
}

// WriteToRequest writes these params to a swagger request
func (o *FpolicyConnectionModifyCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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

	if o.DisconnectedReasonCode != nil {

		// query param disconnected_reason.code
		var qrDisconnectedReasonCode int64

		if o.DisconnectedReasonCode != nil {
			qrDisconnectedReasonCode = *o.DisconnectedReasonCode
		}
		qDisconnectedReasonCode := swag.FormatInt64(qrDisconnectedReasonCode)
		if qDisconnectedReasonCode != "" {

			if err := r.SetQueryParam("disconnected_reason.code", qDisconnectedReasonCode); err != nil {
				return err
			}
		}
	}

	if o.DisconnectedReasonMessage != nil {

		// query param disconnected_reason.message
		var qrDisconnectedReasonMessage string

		if o.DisconnectedReasonMessage != nil {
			qrDisconnectedReasonMessage = *o.DisconnectedReasonMessage
		}
		qDisconnectedReasonMessage := qrDisconnectedReasonMessage
		if qDisconnectedReasonMessage != "" {

			if err := r.SetQueryParam("disconnected_reason.message", qDisconnectedReasonMessage); err != nil {
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

	if o.PassthroughRead != nil {

		// query param passthrough_read
		var qrPassthroughRead bool

		if o.PassthroughRead != nil {
			qrPassthroughRead = *o.PassthroughRead
		}
		qPassthroughRead := swag.FormatBool(qrPassthroughRead)
		if qPassthroughRead != "" {

			if err := r.SetQueryParam("passthrough_read", qPassthroughRead); err != nil {
				return err
			}
		}
	}

	if o.PolicyName != nil {

		// query param policy.name
		var qrPolicyName string

		if o.PolicyName != nil {
			qrPolicyName = *o.PolicyName
		}
		qPolicyName := qrPolicyName
		if qPolicyName != "" {

			if err := r.SetQueryParam("policy.name", qPolicyName); err != nil {
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

	if o.Server != nil {

		// query param server
		var qrServer string

		if o.Server != nil {
			qrServer = *o.Server
		}
		qServer := qrServer
		if qServer != "" {

			if err := r.SetQueryParam("server", qServer); err != nil {
				return err
			}
		}
	}

	if o.SessionUUID != nil {

		// query param session_uuid
		var qrSessionUUID string

		if o.SessionUUID != nil {
			qrSessionUUID = *o.SessionUUID
		}
		qSessionUUID := qrSessionUUID
		if qSessionUUID != "" {

			if err := r.SetQueryParam("session_uuid", qSessionUUID); err != nil {
				return err
			}
		}
	}

	if o.State != nil {

		// query param state
		var qrState string

		if o.State != nil {
			qrState = *o.State
		}
		qState := qrState
		if qState != "" {

			if err := r.SetQueryParam("state", qState); err != nil {
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

	// path param svm.uuid
	if err := r.SetPathParam("svm.uuid", o.SvmUUID); err != nil {
		return err
	}

	if o.Type != nil {

		// query param type
		var qrType string

		if o.Type != nil {
			qrType = *o.Type
		}
		qType := qrType
		if qType != "" {

			if err := r.SetQueryParam("type", qType); err != nil {
				return err
			}
		}
	}

	if o.UpdateTime != nil {

		// query param update_time
		var qrUpdateTime string

		if o.UpdateTime != nil {
			qrUpdateTime = *o.UpdateTime
		}
		qUpdateTime := qrUpdateTime
		if qUpdateTime != "" {

			if err := r.SetQueryParam("update_time", qUpdateTime); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
