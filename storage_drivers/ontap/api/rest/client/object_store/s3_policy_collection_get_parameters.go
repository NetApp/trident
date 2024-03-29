// Code generated by go-swagger; DO NOT EDIT.

package object_store

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

// NewS3PolicyCollectionGetParams creates a new S3PolicyCollectionGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewS3PolicyCollectionGetParams() *S3PolicyCollectionGetParams {
	return &S3PolicyCollectionGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewS3PolicyCollectionGetParamsWithTimeout creates a new S3PolicyCollectionGetParams object
// with the ability to set a timeout on a request.
func NewS3PolicyCollectionGetParamsWithTimeout(timeout time.Duration) *S3PolicyCollectionGetParams {
	return &S3PolicyCollectionGetParams{
		timeout: timeout,
	}
}

// NewS3PolicyCollectionGetParamsWithContext creates a new S3PolicyCollectionGetParams object
// with the ability to set a context for a request.
func NewS3PolicyCollectionGetParamsWithContext(ctx context.Context) *S3PolicyCollectionGetParams {
	return &S3PolicyCollectionGetParams{
		Context: ctx,
	}
}

// NewS3PolicyCollectionGetParamsWithHTTPClient creates a new S3PolicyCollectionGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewS3PolicyCollectionGetParamsWithHTTPClient(client *http.Client) *S3PolicyCollectionGetParams {
	return &S3PolicyCollectionGetParams{
		HTTPClient: client,
	}
}

/*
S3PolicyCollectionGetParams contains all the parameters to send to the API endpoint

	for the s3 policy collection get operation.

	Typically these are written to a http.Request.
*/
type S3PolicyCollectionGetParams struct {

	/* Comment.

	   Filter by comment
	*/
	Comment *string

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* MaxRecords.

	   Limit the number of records returned.
	*/
	MaxRecords *int64

	/* Name.

	   Filter by name
	*/
	Name *string

	/* OrderBy.

	   Order results by specified fields and optional [asc|desc] direction. Default direction is 'asc' for ascending.
	*/
	OrderBy []string

	/* ReadOnly.

	   Filter by read-only
	*/
	ReadOnly *bool

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

	/* StatementsActions.

	   Filter by statements.actions
	*/
	StatementsActions *string

	/* StatementsEffect.

	   Filter by statements.effect
	*/
	StatementsEffect *string

	/* StatementsIndex.

	   Filter by statements.index
	*/
	StatementsIndex *int64

	/* StatementsResources.

	   Filter by statements.resources
	*/
	StatementsResources *string

	/* StatementsSid.

	   Filter by statements.sid
	*/
	StatementsSid *string

	/* SvmName.

	   Filter by svm.name
	*/
	SvmName *string

	/* SvmUUID.

	   UUID of the SVM to which this object belongs.
	*/
	SvmUUID string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the s3 policy collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *S3PolicyCollectionGetParams) WithDefaults() *S3PolicyCollectionGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the s3 policy collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *S3PolicyCollectionGetParams) SetDefaults() {
	var (
		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)
	)

	val := S3PolicyCollectionGetParams{
		ReturnRecords: &returnRecordsDefault,
		ReturnTimeout: &returnTimeoutDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) WithTimeout(timeout time.Duration) *S3PolicyCollectionGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) WithContext(ctx context.Context) *S3PolicyCollectionGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) WithHTTPClient(client *http.Client) *S3PolicyCollectionGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithComment adds the comment to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) WithComment(comment *string) *S3PolicyCollectionGetParams {
	o.SetComment(comment)
	return o
}

// SetComment adds the comment to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) SetComment(comment *string) {
	o.Comment = comment
}

// WithFields adds the fields to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) WithFields(fields []string) *S3PolicyCollectionGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithMaxRecords adds the maxRecords to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) WithMaxRecords(maxRecords *int64) *S3PolicyCollectionGetParams {
	o.SetMaxRecords(maxRecords)
	return o
}

// SetMaxRecords adds the maxRecords to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) SetMaxRecords(maxRecords *int64) {
	o.MaxRecords = maxRecords
}

// WithName adds the name to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) WithName(name *string) *S3PolicyCollectionGetParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) SetName(name *string) {
	o.Name = name
}

// WithOrderBy adds the orderBy to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) WithOrderBy(orderBy []string) *S3PolicyCollectionGetParams {
	o.SetOrderBy(orderBy)
	return o
}

// SetOrderBy adds the orderBy to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) SetOrderBy(orderBy []string) {
	o.OrderBy = orderBy
}

// WithReadOnly adds the readOnly to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) WithReadOnly(readOnly *bool) *S3PolicyCollectionGetParams {
	o.SetReadOnly(readOnly)
	return o
}

// SetReadOnly adds the readOnly to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) SetReadOnly(readOnly *bool) {
	o.ReadOnly = readOnly
}

// WithReturnRecords adds the returnRecords to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) WithReturnRecords(returnRecords *bool) *S3PolicyCollectionGetParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) WithReturnTimeout(returnTimeout *int64) *S3PolicyCollectionGetParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithStatementsActions adds the statementsActions to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) WithStatementsActions(statementsActions *string) *S3PolicyCollectionGetParams {
	o.SetStatementsActions(statementsActions)
	return o
}

// SetStatementsActions adds the statementsActions to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) SetStatementsActions(statementsActions *string) {
	o.StatementsActions = statementsActions
}

// WithStatementsEffect adds the statementsEffect to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) WithStatementsEffect(statementsEffect *string) *S3PolicyCollectionGetParams {
	o.SetStatementsEffect(statementsEffect)
	return o
}

// SetStatementsEffect adds the statementsEffect to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) SetStatementsEffect(statementsEffect *string) {
	o.StatementsEffect = statementsEffect
}

// WithStatementsIndex adds the statementsIndex to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) WithStatementsIndex(statementsIndex *int64) *S3PolicyCollectionGetParams {
	o.SetStatementsIndex(statementsIndex)
	return o
}

// SetStatementsIndex adds the statementsIndex to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) SetStatementsIndex(statementsIndex *int64) {
	o.StatementsIndex = statementsIndex
}

// WithStatementsResources adds the statementsResources to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) WithStatementsResources(statementsResources *string) *S3PolicyCollectionGetParams {
	o.SetStatementsResources(statementsResources)
	return o
}

// SetStatementsResources adds the statementsResources to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) SetStatementsResources(statementsResources *string) {
	o.StatementsResources = statementsResources
}

// WithStatementsSid adds the statementsSid to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) WithStatementsSid(statementsSid *string) *S3PolicyCollectionGetParams {
	o.SetStatementsSid(statementsSid)
	return o
}

// SetStatementsSid adds the statementsSid to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) SetStatementsSid(statementsSid *string) {
	o.StatementsSid = statementsSid
}

// WithSvmName adds the svmName to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) WithSvmName(svmName *string) *S3PolicyCollectionGetParams {
	o.SetSvmName(svmName)
	return o
}

// SetSvmName adds the svmName to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) SetSvmName(svmName *string) {
	o.SvmName = svmName
}

// WithSvmUUID adds the svmUUID to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) WithSvmUUID(svmUUID string) *S3PolicyCollectionGetParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the s3 policy collection get params
func (o *S3PolicyCollectionGetParams) SetSvmUUID(svmUUID string) {
	o.SvmUUID = svmUUID
}

// WriteToRequest writes these params to a swagger request
func (o *S3PolicyCollectionGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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

	if o.Fields != nil {

		// binding items for fields
		joinedFields := o.bindParamFields(reg)

		// query array param fields
		if err := r.SetQueryParam("fields", joinedFields...); err != nil {
			return err
		}
	}

	if o.MaxRecords != nil {

		// query param max_records
		var qrMaxRecords int64

		if o.MaxRecords != nil {
			qrMaxRecords = *o.MaxRecords
		}
		qMaxRecords := swag.FormatInt64(qrMaxRecords)
		if qMaxRecords != "" {

			if err := r.SetQueryParam("max_records", qMaxRecords); err != nil {
				return err
			}
		}
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

	if o.OrderBy != nil {

		// binding items for order_by
		joinedOrderBy := o.bindParamOrderBy(reg)

		// query array param order_by
		if err := r.SetQueryParam("order_by", joinedOrderBy...); err != nil {
			return err
		}
	}

	if o.ReadOnly != nil {

		// query param read-only
		var qrReadOnly bool

		if o.ReadOnly != nil {
			qrReadOnly = *o.ReadOnly
		}
		qReadOnly := swag.FormatBool(qrReadOnly)
		if qReadOnly != "" {

			if err := r.SetQueryParam("read-only", qReadOnly); err != nil {
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

	if o.StatementsActions != nil {

		// query param statements.actions
		var qrStatementsActions string

		if o.StatementsActions != nil {
			qrStatementsActions = *o.StatementsActions
		}
		qStatementsActions := qrStatementsActions
		if qStatementsActions != "" {

			if err := r.SetQueryParam("statements.actions", qStatementsActions); err != nil {
				return err
			}
		}
	}

	if o.StatementsEffect != nil {

		// query param statements.effect
		var qrStatementsEffect string

		if o.StatementsEffect != nil {
			qrStatementsEffect = *o.StatementsEffect
		}
		qStatementsEffect := qrStatementsEffect
		if qStatementsEffect != "" {

			if err := r.SetQueryParam("statements.effect", qStatementsEffect); err != nil {
				return err
			}
		}
	}

	if o.StatementsIndex != nil {

		// query param statements.index
		var qrStatementsIndex int64

		if o.StatementsIndex != nil {
			qrStatementsIndex = *o.StatementsIndex
		}
		qStatementsIndex := swag.FormatInt64(qrStatementsIndex)
		if qStatementsIndex != "" {

			if err := r.SetQueryParam("statements.index", qStatementsIndex); err != nil {
				return err
			}
		}
	}

	if o.StatementsResources != nil {

		// query param statements.resources
		var qrStatementsResources string

		if o.StatementsResources != nil {
			qrStatementsResources = *o.StatementsResources
		}
		qStatementsResources := qrStatementsResources
		if qStatementsResources != "" {

			if err := r.SetQueryParam("statements.resources", qStatementsResources); err != nil {
				return err
			}
		}
	}

	if o.StatementsSid != nil {

		// query param statements.sid
		var qrStatementsSid string

		if o.StatementsSid != nil {
			qrStatementsSid = *o.StatementsSid
		}
		qStatementsSid := qrStatementsSid
		if qStatementsSid != "" {

			if err := r.SetQueryParam("statements.sid", qStatementsSid); err != nil {
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

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamS3PolicyCollectionGet binds the parameter fields
func (o *S3PolicyCollectionGetParams) bindParamFields(formats strfmt.Registry) []string {
	fieldsIR := o.Fields

	var fieldsIC []string
	for _, fieldsIIR := range fieldsIR { // explode []string

		fieldsIIV := fieldsIIR // string as string
		fieldsIC = append(fieldsIC, fieldsIIV)
	}

	// items.CollectionFormat: "csv"
	fieldsIS := swag.JoinByFormat(fieldsIC, "csv")

	return fieldsIS
}

// bindParamS3PolicyCollectionGet binds the parameter order_by
func (o *S3PolicyCollectionGetParams) bindParamOrderBy(formats strfmt.Registry) []string {
	orderByIR := o.OrderBy

	var orderByIC []string
	for _, orderByIIR := range orderByIR { // explode []string

		orderByIIV := orderByIIR // string as string
		orderByIC = append(orderByIC, orderByIIV)
	}

	// items.CollectionFormat: "csv"
	orderByIS := swag.JoinByFormat(orderByIC, "csv")

	return orderByIS
}
