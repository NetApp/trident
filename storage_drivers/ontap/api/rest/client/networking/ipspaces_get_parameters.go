// Code generated by go-swagger; DO NOT EDIT.

package networking

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

// NewIpspacesGetParams creates a new IpspacesGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewIpspacesGetParams() *IpspacesGetParams {
	return &IpspacesGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewIpspacesGetParamsWithTimeout creates a new IpspacesGetParams object
// with the ability to set a timeout on a request.
func NewIpspacesGetParamsWithTimeout(timeout time.Duration) *IpspacesGetParams {
	return &IpspacesGetParams{
		timeout: timeout,
	}
}

// NewIpspacesGetParamsWithContext creates a new IpspacesGetParams object
// with the ability to set a context for a request.
func NewIpspacesGetParamsWithContext(ctx context.Context) *IpspacesGetParams {
	return &IpspacesGetParams{
		Context: ctx,
	}
}

// NewIpspacesGetParamsWithHTTPClient creates a new IpspacesGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewIpspacesGetParamsWithHTTPClient(client *http.Client) *IpspacesGetParams {
	return &IpspacesGetParams{
		HTTPClient: client,
	}
}

/*
IpspacesGetParams contains all the parameters to send to the API endpoint

	for the ipspaces get operation.

	Typically these are written to a http.Request.
*/
type IpspacesGetParams struct {

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

	/* UUID.

	   Filter by uuid
	*/
	UUID *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the ipspaces get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *IpspacesGetParams) WithDefaults() *IpspacesGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the ipspaces get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *IpspacesGetParams) SetDefaults() {
	var (
		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)
	)

	val := IpspacesGetParams{
		ReturnRecords: &returnRecordsDefault,
		ReturnTimeout: &returnTimeoutDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the ipspaces get params
func (o *IpspacesGetParams) WithTimeout(timeout time.Duration) *IpspacesGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the ipspaces get params
func (o *IpspacesGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the ipspaces get params
func (o *IpspacesGetParams) WithContext(ctx context.Context) *IpspacesGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the ipspaces get params
func (o *IpspacesGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the ipspaces get params
func (o *IpspacesGetParams) WithHTTPClient(client *http.Client) *IpspacesGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the ipspaces get params
func (o *IpspacesGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithFields adds the fields to the ipspaces get params
func (o *IpspacesGetParams) WithFields(fields []string) *IpspacesGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the ipspaces get params
func (o *IpspacesGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithMaxRecords adds the maxRecords to the ipspaces get params
func (o *IpspacesGetParams) WithMaxRecords(maxRecords *int64) *IpspacesGetParams {
	o.SetMaxRecords(maxRecords)
	return o
}

// SetMaxRecords adds the maxRecords to the ipspaces get params
func (o *IpspacesGetParams) SetMaxRecords(maxRecords *int64) {
	o.MaxRecords = maxRecords
}

// WithName adds the name to the ipspaces get params
func (o *IpspacesGetParams) WithName(name *string) *IpspacesGetParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the ipspaces get params
func (o *IpspacesGetParams) SetName(name *string) {
	o.Name = name
}

// WithOrderBy adds the orderBy to the ipspaces get params
func (o *IpspacesGetParams) WithOrderBy(orderBy []string) *IpspacesGetParams {
	o.SetOrderBy(orderBy)
	return o
}

// SetOrderBy adds the orderBy to the ipspaces get params
func (o *IpspacesGetParams) SetOrderBy(orderBy []string) {
	o.OrderBy = orderBy
}

// WithReturnRecords adds the returnRecords to the ipspaces get params
func (o *IpspacesGetParams) WithReturnRecords(returnRecords *bool) *IpspacesGetParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the ipspaces get params
func (o *IpspacesGetParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the ipspaces get params
func (o *IpspacesGetParams) WithReturnTimeout(returnTimeout *int64) *IpspacesGetParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the ipspaces get params
func (o *IpspacesGetParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithUUID adds the uuid to the ipspaces get params
func (o *IpspacesGetParams) WithUUID(uuid *string) *IpspacesGetParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the ipspaces get params
func (o *IpspacesGetParams) SetUUID(uuid *string) {
	o.UUID = uuid
}

// WriteToRequest writes these params to a swagger request
func (o *IpspacesGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

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

// bindParamIpspacesGet binds the parameter fields
func (o *IpspacesGetParams) bindParamFields(formats strfmt.Registry) []string {
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

// bindParamIpspacesGet binds the parameter order_by
func (o *IpspacesGetParams) bindParamOrderBy(formats strfmt.Registry) []string {
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
