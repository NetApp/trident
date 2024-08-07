// Code generated by go-swagger; DO NOT EDIT.

package cluster

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

// NewClusterNtpServersCollectionGetParams creates a new ClusterNtpServersCollectionGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewClusterNtpServersCollectionGetParams() *ClusterNtpServersCollectionGetParams {
	return &ClusterNtpServersCollectionGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewClusterNtpServersCollectionGetParamsWithTimeout creates a new ClusterNtpServersCollectionGetParams object
// with the ability to set a timeout on a request.
func NewClusterNtpServersCollectionGetParamsWithTimeout(timeout time.Duration) *ClusterNtpServersCollectionGetParams {
	return &ClusterNtpServersCollectionGetParams{
		timeout: timeout,
	}
}

// NewClusterNtpServersCollectionGetParamsWithContext creates a new ClusterNtpServersCollectionGetParams object
// with the ability to set a context for a request.
func NewClusterNtpServersCollectionGetParamsWithContext(ctx context.Context) *ClusterNtpServersCollectionGetParams {
	return &ClusterNtpServersCollectionGetParams{
		Context: ctx,
	}
}

// NewClusterNtpServersCollectionGetParamsWithHTTPClient creates a new ClusterNtpServersCollectionGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewClusterNtpServersCollectionGetParamsWithHTTPClient(client *http.Client) *ClusterNtpServersCollectionGetParams {
	return &ClusterNtpServersCollectionGetParams{
		HTTPClient: client,
	}
}

/*
ClusterNtpServersCollectionGetParams contains all the parameters to send to the API endpoint

	for the cluster ntp servers collection get operation.

	Typically these are written to a http.Request.
*/
type ClusterNtpServersCollectionGetParams struct {

	/* AuthenticationEnabled.

	   Filter by authentication_enabled
	*/
	AuthenticationEnabled *bool

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* KeyID.

	   Filter by key.id
	*/
	KeyID *int64

	/* MaxRecords.

	   Limit the number of records returned.
	*/
	MaxRecords *int64

	/* OrderBy.

	   Order results by specified fields and optional [asc|desc] direction. Default direction is 'asc' for ascending.
	*/
	OrderBy []string

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

	/* Server.

	   Filter by server
	*/
	Server *string

	/* Version.

	   Filter by version
	*/
	Version *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the cluster ntp servers collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *ClusterNtpServersCollectionGetParams) WithDefaults() *ClusterNtpServersCollectionGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the cluster ntp servers collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *ClusterNtpServersCollectionGetParams) SetDefaults() {
	var (
		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)
	)

	val := ClusterNtpServersCollectionGetParams{
		ReturnRecords: &returnRecordsDefault,
		ReturnTimeout: &returnTimeoutDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) WithTimeout(timeout time.Duration) *ClusterNtpServersCollectionGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) WithContext(ctx context.Context) *ClusterNtpServersCollectionGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) WithHTTPClient(client *http.Client) *ClusterNtpServersCollectionGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithAuthenticationEnabled adds the authenticationEnabled to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) WithAuthenticationEnabled(authenticationEnabled *bool) *ClusterNtpServersCollectionGetParams {
	o.SetAuthenticationEnabled(authenticationEnabled)
	return o
}

// SetAuthenticationEnabled adds the authenticationEnabled to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) SetAuthenticationEnabled(authenticationEnabled *bool) {
	o.AuthenticationEnabled = authenticationEnabled
}

// WithFields adds the fields to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) WithFields(fields []string) *ClusterNtpServersCollectionGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithKeyID adds the keyID to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) WithKeyID(keyID *int64) *ClusterNtpServersCollectionGetParams {
	o.SetKeyID(keyID)
	return o
}

// SetKeyID adds the keyId to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) SetKeyID(keyID *int64) {
	o.KeyID = keyID
}

// WithMaxRecords adds the maxRecords to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) WithMaxRecords(maxRecords *int64) *ClusterNtpServersCollectionGetParams {
	o.SetMaxRecords(maxRecords)
	return o
}

// SetMaxRecords adds the maxRecords to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) SetMaxRecords(maxRecords *int64) {
	o.MaxRecords = maxRecords
}

// WithOrderBy adds the orderBy to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) WithOrderBy(orderBy []string) *ClusterNtpServersCollectionGetParams {
	o.SetOrderBy(orderBy)
	return o
}

// SetOrderBy adds the orderBy to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) SetOrderBy(orderBy []string) {
	o.OrderBy = orderBy
}

// WithReturnRecords adds the returnRecords to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) WithReturnRecords(returnRecords *bool) *ClusterNtpServersCollectionGetParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) WithReturnTimeout(returnTimeout *int64) *ClusterNtpServersCollectionGetParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithServer adds the server to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) WithServer(server *string) *ClusterNtpServersCollectionGetParams {
	o.SetServer(server)
	return o
}

// SetServer adds the server to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) SetServer(server *string) {
	o.Server = server
}

// WithVersion adds the version to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) WithVersion(version *string) *ClusterNtpServersCollectionGetParams {
	o.SetVersion(version)
	return o
}

// SetVersion adds the version to the cluster ntp servers collection get params
func (o *ClusterNtpServersCollectionGetParams) SetVersion(version *string) {
	o.Version = version
}

// WriteToRequest writes these params to a swagger request
func (o *ClusterNtpServersCollectionGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.AuthenticationEnabled != nil {

		// query param authentication_enabled
		var qrAuthenticationEnabled bool

		if o.AuthenticationEnabled != nil {
			qrAuthenticationEnabled = *o.AuthenticationEnabled
		}
		qAuthenticationEnabled := swag.FormatBool(qrAuthenticationEnabled)
		if qAuthenticationEnabled != "" {

			if err := r.SetQueryParam("authentication_enabled", qAuthenticationEnabled); err != nil {
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

	if o.KeyID != nil {

		// query param key.id
		var qrKeyID int64

		if o.KeyID != nil {
			qrKeyID = *o.KeyID
		}
		qKeyID := swag.FormatInt64(qrKeyID)
		if qKeyID != "" {

			if err := r.SetQueryParam("key.id", qKeyID); err != nil {
				return err
			}
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

	if o.OrderBy != nil {

		// binding items for order_by
		joinedOrderBy := o.bindParamOrderBy(reg)

		// query array param order_by
		if err := r.SetQueryParam("order_by", joinedOrderBy...); err != nil {
			return err
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

	if o.Version != nil {

		// query param version
		var qrVersion string

		if o.Version != nil {
			qrVersion = *o.Version
		}
		qVersion := qrVersion
		if qVersion != "" {

			if err := r.SetQueryParam("version", qVersion); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamClusterNtpServersCollectionGet binds the parameter fields
func (o *ClusterNtpServersCollectionGetParams) bindParamFields(formats strfmt.Registry) []string {
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

// bindParamClusterNtpServersCollectionGet binds the parameter order_by
func (o *ClusterNtpServersCollectionGetParams) bindParamOrderBy(formats strfmt.Registry) []string {
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
