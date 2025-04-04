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

// NewGroupRoleMappingsDeleteCollectionParams creates a new GroupRoleMappingsDeleteCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewGroupRoleMappingsDeleteCollectionParams() *GroupRoleMappingsDeleteCollectionParams {
	return &GroupRoleMappingsDeleteCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewGroupRoleMappingsDeleteCollectionParamsWithTimeout creates a new GroupRoleMappingsDeleteCollectionParams object
// with the ability to set a timeout on a request.
func NewGroupRoleMappingsDeleteCollectionParamsWithTimeout(timeout time.Duration) *GroupRoleMappingsDeleteCollectionParams {
	return &GroupRoleMappingsDeleteCollectionParams{
		timeout: timeout,
	}
}

// NewGroupRoleMappingsDeleteCollectionParamsWithContext creates a new GroupRoleMappingsDeleteCollectionParams object
// with the ability to set a context for a request.
func NewGroupRoleMappingsDeleteCollectionParamsWithContext(ctx context.Context) *GroupRoleMappingsDeleteCollectionParams {
	return &GroupRoleMappingsDeleteCollectionParams{
		Context: ctx,
	}
}

// NewGroupRoleMappingsDeleteCollectionParamsWithHTTPClient creates a new GroupRoleMappingsDeleteCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewGroupRoleMappingsDeleteCollectionParamsWithHTTPClient(client *http.Client) *GroupRoleMappingsDeleteCollectionParams {
	return &GroupRoleMappingsDeleteCollectionParams{
		HTTPClient: client,
	}
}

/*
GroupRoleMappingsDeleteCollectionParams contains all the parameters to send to the API endpoint

	for the group role mappings delete collection operation.

	Typically these are written to a http.Request.
*/
type GroupRoleMappingsDeleteCollectionParams struct {

	/* Comment.

	   Filter by comment
	*/
	Comment *string

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* GroupID.

	   Filter by group_id
	*/
	GroupID *int64

	/* Info.

	   Info specification
	*/
	Info GroupRoleMappingsDeleteCollectionBody

	/* OntapRoleName.

	   Filter by ontap_role.name
	*/
	OntapRoleName *string

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

// WithDefaults hydrates default values in the group role mappings delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *GroupRoleMappingsDeleteCollectionParams) WithDefaults() *GroupRoleMappingsDeleteCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the group role mappings delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *GroupRoleMappingsDeleteCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := GroupRoleMappingsDeleteCollectionParams{
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

// WithTimeout adds the timeout to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) WithTimeout(timeout time.Duration) *GroupRoleMappingsDeleteCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) WithContext(ctx context.Context) *GroupRoleMappingsDeleteCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) WithHTTPClient(client *http.Client) *GroupRoleMappingsDeleteCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithComment adds the comment to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) WithComment(comment *string) *GroupRoleMappingsDeleteCollectionParams {
	o.SetComment(comment)
	return o
}

// SetComment adds the comment to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) SetComment(comment *string) {
	o.Comment = comment
}

// WithContinueOnFailure adds the continueOnFailure to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *GroupRoleMappingsDeleteCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithGroupID adds the groupID to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) WithGroupID(groupID *int64) *GroupRoleMappingsDeleteCollectionParams {
	o.SetGroupID(groupID)
	return o
}

// SetGroupID adds the groupId to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) SetGroupID(groupID *int64) {
	o.GroupID = groupID
}

// WithInfo adds the info to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) WithInfo(info GroupRoleMappingsDeleteCollectionBody) *GroupRoleMappingsDeleteCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) SetInfo(info GroupRoleMappingsDeleteCollectionBody) {
	o.Info = info
}

// WithOntapRoleName adds the ontapRoleName to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) WithOntapRoleName(ontapRoleName *string) *GroupRoleMappingsDeleteCollectionParams {
	o.SetOntapRoleName(ontapRoleName)
	return o
}

// SetOntapRoleName adds the ontapRoleName to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) SetOntapRoleName(ontapRoleName *string) {
	o.OntapRoleName = ontapRoleName
}

// WithReturnRecords adds the returnRecords to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) WithReturnRecords(returnRecords *bool) *GroupRoleMappingsDeleteCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) WithReturnTimeout(returnTimeout *int64) *GroupRoleMappingsDeleteCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithScope adds the scope to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) WithScope(scope *string) *GroupRoleMappingsDeleteCollectionParams {
	o.SetScope(scope)
	return o
}

// SetScope adds the scope to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) SetScope(scope *string) {
	o.Scope = scope
}

// WithSerialRecords adds the serialRecords to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) WithSerialRecords(serialRecords *bool) *GroupRoleMappingsDeleteCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the group role mappings delete collection params
func (o *GroupRoleMappingsDeleteCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WriteToRequest writes these params to a swagger request
func (o *GroupRoleMappingsDeleteCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

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

	if o.GroupID != nil {

		// query param group_id
		var qrGroupID int64

		if o.GroupID != nil {
			qrGroupID = *o.GroupID
		}
		qGroupID := swag.FormatInt64(qrGroupID)
		if qGroupID != "" {

			if err := r.SetQueryParam("group_id", qGroupID); err != nil {
				return err
			}
		}
	}
	if err := r.SetBodyParam(o.Info); err != nil {
		return err
	}

	if o.OntapRoleName != nil {

		// query param ontap_role.name
		var qrOntapRoleName string

		if o.OntapRoleName != nil {
			qrOntapRoleName = *o.OntapRoleName
		}
		qOntapRoleName := qrOntapRoleName
		if qOntapRoleName != "" {

			if err := r.SetQueryParam("ontap_role.name", qOntapRoleName); err != nil {
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
