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

// NewMultiAdminVerifyRuleDeleteCollectionParams creates a new MultiAdminVerifyRuleDeleteCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewMultiAdminVerifyRuleDeleteCollectionParams() *MultiAdminVerifyRuleDeleteCollectionParams {
	return &MultiAdminVerifyRuleDeleteCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewMultiAdminVerifyRuleDeleteCollectionParamsWithTimeout creates a new MultiAdminVerifyRuleDeleteCollectionParams object
// with the ability to set a timeout on a request.
func NewMultiAdminVerifyRuleDeleteCollectionParamsWithTimeout(timeout time.Duration) *MultiAdminVerifyRuleDeleteCollectionParams {
	return &MultiAdminVerifyRuleDeleteCollectionParams{
		timeout: timeout,
	}
}

// NewMultiAdminVerifyRuleDeleteCollectionParamsWithContext creates a new MultiAdminVerifyRuleDeleteCollectionParams object
// with the ability to set a context for a request.
func NewMultiAdminVerifyRuleDeleteCollectionParamsWithContext(ctx context.Context) *MultiAdminVerifyRuleDeleteCollectionParams {
	return &MultiAdminVerifyRuleDeleteCollectionParams{
		Context: ctx,
	}
}

// NewMultiAdminVerifyRuleDeleteCollectionParamsWithHTTPClient creates a new MultiAdminVerifyRuleDeleteCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewMultiAdminVerifyRuleDeleteCollectionParamsWithHTTPClient(client *http.Client) *MultiAdminVerifyRuleDeleteCollectionParams {
	return &MultiAdminVerifyRuleDeleteCollectionParams{
		HTTPClient: client,
	}
}

/*
MultiAdminVerifyRuleDeleteCollectionParams contains all the parameters to send to the API endpoint

	for the multi admin verify rule delete collection operation.

	Typically these are written to a http.Request.
*/
type MultiAdminVerifyRuleDeleteCollectionParams struct {

	/* ApprovalExpiry.

	   Filter by approval_expiry
	*/
	ApprovalExpiry *string

	/* ApprovalGroupsName.

	   Filter by approval_groups.name
	*/
	ApprovalGroupsName *string

	/* AutoRequestCreate.

	   Filter by auto_request_create
	*/
	AutoRequestCreate *bool

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* CreateTime.

	   Filter by create_time
	*/
	CreateTime *string

	/* ExecutionExpiry.

	   Filter by execution_expiry
	*/
	ExecutionExpiry *string

	/* Info.

	   Info specification
	*/
	Info MultiAdminVerifyRuleDeleteCollectionBody

	/* Operation.

	   Filter by operation
	*/
	Operation *string

	/* OwnerName.

	   Filter by owner.name
	*/
	OwnerName *string

	/* OwnerUUID.

	   Filter by owner.uuid
	*/
	OwnerUUID *string

	/* Query.

	   Filter by query
	*/
	Query *string

	/* RequiredApprovers.

	   Filter by required_approvers
	*/
	RequiredApprovers *int64

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

	/* SystemDefined.

	   Filter by system_defined
	*/
	SystemDefined *bool

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the multi admin verify rule delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithDefaults() *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the multi admin verify rule delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := MultiAdminVerifyRuleDeleteCollectionParams{
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

// WithTimeout adds the timeout to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithTimeout(timeout time.Duration) *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithContext(ctx context.Context) *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithHTTPClient(client *http.Client) *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithApprovalExpiry adds the approvalExpiry to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithApprovalExpiry(approvalExpiry *string) *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetApprovalExpiry(approvalExpiry)
	return o
}

// SetApprovalExpiry adds the approvalExpiry to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetApprovalExpiry(approvalExpiry *string) {
	o.ApprovalExpiry = approvalExpiry
}

// WithApprovalGroupsName adds the approvalGroupsName to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithApprovalGroupsName(approvalGroupsName *string) *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetApprovalGroupsName(approvalGroupsName)
	return o
}

// SetApprovalGroupsName adds the approvalGroupsName to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetApprovalGroupsName(approvalGroupsName *string) {
	o.ApprovalGroupsName = approvalGroupsName
}

// WithAutoRequestCreate adds the autoRequestCreate to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithAutoRequestCreate(autoRequestCreate *bool) *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetAutoRequestCreate(autoRequestCreate)
	return o
}

// SetAutoRequestCreate adds the autoRequestCreate to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetAutoRequestCreate(autoRequestCreate *bool) {
	o.AutoRequestCreate = autoRequestCreate
}

// WithContinueOnFailure adds the continueOnFailure to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithCreateTime adds the createTime to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithCreateTime(createTime *string) *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetCreateTime(createTime)
	return o
}

// SetCreateTime adds the createTime to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetCreateTime(createTime *string) {
	o.CreateTime = createTime
}

// WithExecutionExpiry adds the executionExpiry to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithExecutionExpiry(executionExpiry *string) *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetExecutionExpiry(executionExpiry)
	return o
}

// SetExecutionExpiry adds the executionExpiry to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetExecutionExpiry(executionExpiry *string) {
	o.ExecutionExpiry = executionExpiry
}

// WithInfo adds the info to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithInfo(info MultiAdminVerifyRuleDeleteCollectionBody) *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetInfo(info MultiAdminVerifyRuleDeleteCollectionBody) {
	o.Info = info
}

// WithOperation adds the operation to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithOperation(operation *string) *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetOperation(operation)
	return o
}

// SetOperation adds the operation to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetOperation(operation *string) {
	o.Operation = operation
}

// WithOwnerName adds the ownerName to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithOwnerName(ownerName *string) *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetOwnerName(ownerName)
	return o
}

// SetOwnerName adds the ownerName to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetOwnerName(ownerName *string) {
	o.OwnerName = ownerName
}

// WithOwnerUUID adds the ownerUUID to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithOwnerUUID(ownerUUID *string) *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetOwnerUUID(ownerUUID)
	return o
}

// SetOwnerUUID adds the ownerUuid to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetOwnerUUID(ownerUUID *string) {
	o.OwnerUUID = ownerUUID
}

// WithQuery adds the query to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithQuery(query *string) *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetQuery(query)
	return o
}

// SetQuery adds the query to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetQuery(query *string) {
	o.Query = query
}

// WithRequiredApprovers adds the requiredApprovers to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithRequiredApprovers(requiredApprovers *int64) *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetRequiredApprovers(requiredApprovers)
	return o
}

// SetRequiredApprovers adds the requiredApprovers to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetRequiredApprovers(requiredApprovers *int64) {
	o.RequiredApprovers = requiredApprovers
}

// WithReturnRecords adds the returnRecords to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithReturnRecords(returnRecords *bool) *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithReturnTimeout(returnTimeout *int64) *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithSerialRecords adds the serialRecords to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithSerialRecords(serialRecords *bool) *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WithSystemDefined adds the systemDefined to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WithSystemDefined(systemDefined *bool) *MultiAdminVerifyRuleDeleteCollectionParams {
	o.SetSystemDefined(systemDefined)
	return o
}

// SetSystemDefined adds the systemDefined to the multi admin verify rule delete collection params
func (o *MultiAdminVerifyRuleDeleteCollectionParams) SetSystemDefined(systemDefined *bool) {
	o.SystemDefined = systemDefined
}

// WriteToRequest writes these params to a swagger request
func (o *MultiAdminVerifyRuleDeleteCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.ApprovalExpiry != nil {

		// query param approval_expiry
		var qrApprovalExpiry string

		if o.ApprovalExpiry != nil {
			qrApprovalExpiry = *o.ApprovalExpiry
		}
		qApprovalExpiry := qrApprovalExpiry
		if qApprovalExpiry != "" {

			if err := r.SetQueryParam("approval_expiry", qApprovalExpiry); err != nil {
				return err
			}
		}
	}

	if o.ApprovalGroupsName != nil {

		// query param approval_groups.name
		var qrApprovalGroupsName string

		if o.ApprovalGroupsName != nil {
			qrApprovalGroupsName = *o.ApprovalGroupsName
		}
		qApprovalGroupsName := qrApprovalGroupsName
		if qApprovalGroupsName != "" {

			if err := r.SetQueryParam("approval_groups.name", qApprovalGroupsName); err != nil {
				return err
			}
		}
	}

	if o.AutoRequestCreate != nil {

		// query param auto_request_create
		var qrAutoRequestCreate bool

		if o.AutoRequestCreate != nil {
			qrAutoRequestCreate = *o.AutoRequestCreate
		}
		qAutoRequestCreate := swag.FormatBool(qrAutoRequestCreate)
		if qAutoRequestCreate != "" {

			if err := r.SetQueryParam("auto_request_create", qAutoRequestCreate); err != nil {
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

	if o.CreateTime != nil {

		// query param create_time
		var qrCreateTime string

		if o.CreateTime != nil {
			qrCreateTime = *o.CreateTime
		}
		qCreateTime := qrCreateTime
		if qCreateTime != "" {

			if err := r.SetQueryParam("create_time", qCreateTime); err != nil {
				return err
			}
		}
	}

	if o.ExecutionExpiry != nil {

		// query param execution_expiry
		var qrExecutionExpiry string

		if o.ExecutionExpiry != nil {
			qrExecutionExpiry = *o.ExecutionExpiry
		}
		qExecutionExpiry := qrExecutionExpiry
		if qExecutionExpiry != "" {

			if err := r.SetQueryParam("execution_expiry", qExecutionExpiry); err != nil {
				return err
			}
		}
	}
	if err := r.SetBodyParam(o.Info); err != nil {
		return err
	}

	if o.Operation != nil {

		// query param operation
		var qrOperation string

		if o.Operation != nil {
			qrOperation = *o.Operation
		}
		qOperation := qrOperation
		if qOperation != "" {

			if err := r.SetQueryParam("operation", qOperation); err != nil {
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

	if o.Query != nil {

		// query param query
		var qrQuery string

		if o.Query != nil {
			qrQuery = *o.Query
		}
		qQuery := qrQuery
		if qQuery != "" {

			if err := r.SetQueryParam("query", qQuery); err != nil {
				return err
			}
		}
	}

	if o.RequiredApprovers != nil {

		// query param required_approvers
		var qrRequiredApprovers int64

		if o.RequiredApprovers != nil {
			qrRequiredApprovers = *o.RequiredApprovers
		}
		qRequiredApprovers := swag.FormatInt64(qrRequiredApprovers)
		if qRequiredApprovers != "" {

			if err := r.SetQueryParam("required_approvers", qRequiredApprovers); err != nil {
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

	if o.SystemDefined != nil {

		// query param system_defined
		var qrSystemDefined bool

		if o.SystemDefined != nil {
			qrSystemDefined = *o.SystemDefined
		}
		qSystemDefined := swag.FormatBool(qrSystemDefined)
		if qSystemDefined != "" {

			if err := r.SetQueryParam("system_defined", qSystemDefined); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
