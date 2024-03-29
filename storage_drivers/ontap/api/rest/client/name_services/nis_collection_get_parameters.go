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

// NewNisCollectionGetParams creates a new NisCollectionGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewNisCollectionGetParams() *NisCollectionGetParams {
	return &NisCollectionGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewNisCollectionGetParamsWithTimeout creates a new NisCollectionGetParams object
// with the ability to set a timeout on a request.
func NewNisCollectionGetParamsWithTimeout(timeout time.Duration) *NisCollectionGetParams {
	return &NisCollectionGetParams{
		timeout: timeout,
	}
}

// NewNisCollectionGetParamsWithContext creates a new NisCollectionGetParams object
// with the ability to set a context for a request.
func NewNisCollectionGetParamsWithContext(ctx context.Context) *NisCollectionGetParams {
	return &NisCollectionGetParams{
		Context: ctx,
	}
}

// NewNisCollectionGetParamsWithHTTPClient creates a new NisCollectionGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewNisCollectionGetParamsWithHTTPClient(client *http.Client) *NisCollectionGetParams {
	return &NisCollectionGetParams{
		HTTPClient: client,
	}
}

/*
NisCollectionGetParams contains all the parameters to send to the API endpoint

	for the nis collection get operation.

	Typically these are written to a http.Request.
*/
type NisCollectionGetParams struct {

	/* BindingDetailsServer.

	   Filter by binding_details.server
	*/
	BindingDetailsServer *string

	/* BindingDetailsStatusCode.

	   Filter by binding_details.status.code
	*/
	BindingDetailsStatusCode *string

	/* BindingDetailsStatusMessage.

	   Filter by binding_details.status.message
	*/
	BindingDetailsStatusMessage *string

	/* BoundServers.

	   Filter by bound_servers
	*/
	BoundServers *string

	/* Domain.

	   Filter by domain
	*/
	Domain *string

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

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

	/* Servers.

	   Filter by servers
	*/
	Servers *string

	/* SvmName.

	   Filter by svm.name
	*/
	SvmName *string

	/* SvmUUID.

	   Filter by svm.uuid
	*/
	SvmUUID *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the nis collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *NisCollectionGetParams) WithDefaults() *NisCollectionGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the nis collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *NisCollectionGetParams) SetDefaults() {
	var (
		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)
	)

	val := NisCollectionGetParams{
		ReturnRecords: &returnRecordsDefault,
		ReturnTimeout: &returnTimeoutDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the nis collection get params
func (o *NisCollectionGetParams) WithTimeout(timeout time.Duration) *NisCollectionGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the nis collection get params
func (o *NisCollectionGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the nis collection get params
func (o *NisCollectionGetParams) WithContext(ctx context.Context) *NisCollectionGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the nis collection get params
func (o *NisCollectionGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the nis collection get params
func (o *NisCollectionGetParams) WithHTTPClient(client *http.Client) *NisCollectionGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the nis collection get params
func (o *NisCollectionGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithBindingDetailsServer adds the bindingDetailsServer to the nis collection get params
func (o *NisCollectionGetParams) WithBindingDetailsServer(bindingDetailsServer *string) *NisCollectionGetParams {
	o.SetBindingDetailsServer(bindingDetailsServer)
	return o
}

// SetBindingDetailsServer adds the bindingDetailsServer to the nis collection get params
func (o *NisCollectionGetParams) SetBindingDetailsServer(bindingDetailsServer *string) {
	o.BindingDetailsServer = bindingDetailsServer
}

// WithBindingDetailsStatusCode adds the bindingDetailsStatusCode to the nis collection get params
func (o *NisCollectionGetParams) WithBindingDetailsStatusCode(bindingDetailsStatusCode *string) *NisCollectionGetParams {
	o.SetBindingDetailsStatusCode(bindingDetailsStatusCode)
	return o
}

// SetBindingDetailsStatusCode adds the bindingDetailsStatusCode to the nis collection get params
func (o *NisCollectionGetParams) SetBindingDetailsStatusCode(bindingDetailsStatusCode *string) {
	o.BindingDetailsStatusCode = bindingDetailsStatusCode
}

// WithBindingDetailsStatusMessage adds the bindingDetailsStatusMessage to the nis collection get params
func (o *NisCollectionGetParams) WithBindingDetailsStatusMessage(bindingDetailsStatusMessage *string) *NisCollectionGetParams {
	o.SetBindingDetailsStatusMessage(bindingDetailsStatusMessage)
	return o
}

// SetBindingDetailsStatusMessage adds the bindingDetailsStatusMessage to the nis collection get params
func (o *NisCollectionGetParams) SetBindingDetailsStatusMessage(bindingDetailsStatusMessage *string) {
	o.BindingDetailsStatusMessage = bindingDetailsStatusMessage
}

// WithBoundServers adds the boundServers to the nis collection get params
func (o *NisCollectionGetParams) WithBoundServers(boundServers *string) *NisCollectionGetParams {
	o.SetBoundServers(boundServers)
	return o
}

// SetBoundServers adds the boundServers to the nis collection get params
func (o *NisCollectionGetParams) SetBoundServers(boundServers *string) {
	o.BoundServers = boundServers
}

// WithDomain adds the domain to the nis collection get params
func (o *NisCollectionGetParams) WithDomain(domain *string) *NisCollectionGetParams {
	o.SetDomain(domain)
	return o
}

// SetDomain adds the domain to the nis collection get params
func (o *NisCollectionGetParams) SetDomain(domain *string) {
	o.Domain = domain
}

// WithFields adds the fields to the nis collection get params
func (o *NisCollectionGetParams) WithFields(fields []string) *NisCollectionGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the nis collection get params
func (o *NisCollectionGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithMaxRecords adds the maxRecords to the nis collection get params
func (o *NisCollectionGetParams) WithMaxRecords(maxRecords *int64) *NisCollectionGetParams {
	o.SetMaxRecords(maxRecords)
	return o
}

// SetMaxRecords adds the maxRecords to the nis collection get params
func (o *NisCollectionGetParams) SetMaxRecords(maxRecords *int64) {
	o.MaxRecords = maxRecords
}

// WithOrderBy adds the orderBy to the nis collection get params
func (o *NisCollectionGetParams) WithOrderBy(orderBy []string) *NisCollectionGetParams {
	o.SetOrderBy(orderBy)
	return o
}

// SetOrderBy adds the orderBy to the nis collection get params
func (o *NisCollectionGetParams) SetOrderBy(orderBy []string) {
	o.OrderBy = orderBy
}

// WithReturnRecords adds the returnRecords to the nis collection get params
func (o *NisCollectionGetParams) WithReturnRecords(returnRecords *bool) *NisCollectionGetParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the nis collection get params
func (o *NisCollectionGetParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the nis collection get params
func (o *NisCollectionGetParams) WithReturnTimeout(returnTimeout *int64) *NisCollectionGetParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the nis collection get params
func (o *NisCollectionGetParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithServers adds the servers to the nis collection get params
func (o *NisCollectionGetParams) WithServers(servers *string) *NisCollectionGetParams {
	o.SetServers(servers)
	return o
}

// SetServers adds the servers to the nis collection get params
func (o *NisCollectionGetParams) SetServers(servers *string) {
	o.Servers = servers
}

// WithSvmName adds the svmName to the nis collection get params
func (o *NisCollectionGetParams) WithSvmName(svmName *string) *NisCollectionGetParams {
	o.SetSvmName(svmName)
	return o
}

// SetSvmName adds the svmName to the nis collection get params
func (o *NisCollectionGetParams) SetSvmName(svmName *string) {
	o.SvmName = svmName
}

// WithSvmUUID adds the svmUUID to the nis collection get params
func (o *NisCollectionGetParams) WithSvmUUID(svmUUID *string) *NisCollectionGetParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the nis collection get params
func (o *NisCollectionGetParams) SetSvmUUID(svmUUID *string) {
	o.SvmUUID = svmUUID
}

// WriteToRequest writes these params to a swagger request
func (o *NisCollectionGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.BindingDetailsServer != nil {

		// query param binding_details.server
		var qrBindingDetailsServer string

		if o.BindingDetailsServer != nil {
			qrBindingDetailsServer = *o.BindingDetailsServer
		}
		qBindingDetailsServer := qrBindingDetailsServer
		if qBindingDetailsServer != "" {

			if err := r.SetQueryParam("binding_details.server", qBindingDetailsServer); err != nil {
				return err
			}
		}
	}

	if o.BindingDetailsStatusCode != nil {

		// query param binding_details.status.code
		var qrBindingDetailsStatusCode string

		if o.BindingDetailsStatusCode != nil {
			qrBindingDetailsStatusCode = *o.BindingDetailsStatusCode
		}
		qBindingDetailsStatusCode := qrBindingDetailsStatusCode
		if qBindingDetailsStatusCode != "" {

			if err := r.SetQueryParam("binding_details.status.code", qBindingDetailsStatusCode); err != nil {
				return err
			}
		}
	}

	if o.BindingDetailsStatusMessage != nil {

		// query param binding_details.status.message
		var qrBindingDetailsStatusMessage string

		if o.BindingDetailsStatusMessage != nil {
			qrBindingDetailsStatusMessage = *o.BindingDetailsStatusMessage
		}
		qBindingDetailsStatusMessage := qrBindingDetailsStatusMessage
		if qBindingDetailsStatusMessage != "" {

			if err := r.SetQueryParam("binding_details.status.message", qBindingDetailsStatusMessage); err != nil {
				return err
			}
		}
	}

	if o.BoundServers != nil {

		// query param bound_servers
		var qrBoundServers string

		if o.BoundServers != nil {
			qrBoundServers = *o.BoundServers
		}
		qBoundServers := qrBoundServers
		if qBoundServers != "" {

			if err := r.SetQueryParam("bound_servers", qBoundServers); err != nil {
				return err
			}
		}
	}

	if o.Domain != nil {

		// query param domain
		var qrDomain string

		if o.Domain != nil {
			qrDomain = *o.Domain
		}
		qDomain := qrDomain
		if qDomain != "" {

			if err := r.SetQueryParam("domain", qDomain); err != nil {
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

	if o.Servers != nil {

		// query param servers
		var qrServers string

		if o.Servers != nil {
			qrServers = *o.Servers
		}
		qServers := qrServers
		if qServers != "" {

			if err := r.SetQueryParam("servers", qServers); err != nil {
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

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamNisCollectionGet binds the parameter fields
func (o *NisCollectionGetParams) bindParamFields(formats strfmt.Registry) []string {
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

// bindParamNisCollectionGet binds the parameter order_by
func (o *NisCollectionGetParams) bindParamOrderBy(formats strfmt.Registry) []string {
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
