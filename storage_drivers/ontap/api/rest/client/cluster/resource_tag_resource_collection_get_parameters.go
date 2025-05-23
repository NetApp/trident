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

// NewResourceTagResourceCollectionGetParams creates a new ResourceTagResourceCollectionGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewResourceTagResourceCollectionGetParams() *ResourceTagResourceCollectionGetParams {
	return &ResourceTagResourceCollectionGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewResourceTagResourceCollectionGetParamsWithTimeout creates a new ResourceTagResourceCollectionGetParams object
// with the ability to set a timeout on a request.
func NewResourceTagResourceCollectionGetParamsWithTimeout(timeout time.Duration) *ResourceTagResourceCollectionGetParams {
	return &ResourceTagResourceCollectionGetParams{
		timeout: timeout,
	}
}

// NewResourceTagResourceCollectionGetParamsWithContext creates a new ResourceTagResourceCollectionGetParams object
// with the ability to set a context for a request.
func NewResourceTagResourceCollectionGetParamsWithContext(ctx context.Context) *ResourceTagResourceCollectionGetParams {
	return &ResourceTagResourceCollectionGetParams{
		Context: ctx,
	}
}

// NewResourceTagResourceCollectionGetParamsWithHTTPClient creates a new ResourceTagResourceCollectionGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewResourceTagResourceCollectionGetParamsWithHTTPClient(client *http.Client) *ResourceTagResourceCollectionGetParams {
	return &ResourceTagResourceCollectionGetParams{
		HTTPClient: client,
	}
}

/*
ResourceTagResourceCollectionGetParams contains all the parameters to send to the API endpoint

	for the resource tag resource collection get operation.

	Typically these are written to a http.Request.
*/
type ResourceTagResourceCollectionGetParams struct {

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* Href.

	   Filter by href
	*/
	Href *string

	/* Label.

	   Filter by label
	*/
	Label *string

	/* MaxRecords.

	   Limit the number of records returned.
	*/
	MaxRecords *int64

	/* OrderBy.

	   Order results by specified fields and optional [asc|desc] direction. Default direction is 'asc' for ascending.
	*/
	OrderBy []string

	// ResourceTagValue.
	//
	// Format: key:value
	ResourceTagValue string

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

	/* SvmName.

	   Filter by svm.name
	*/
	SvmName *string

	/* SvmUUID.

	   Filter by svm.uuid
	*/
	SvmUUID *string

	/* Value.

	   Filter by value
	*/
	Value *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the resource tag resource collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *ResourceTagResourceCollectionGetParams) WithDefaults() *ResourceTagResourceCollectionGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the resource tag resource collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *ResourceTagResourceCollectionGetParams) SetDefaults() {
	var (
		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)
	)

	val := ResourceTagResourceCollectionGetParams{
		ReturnRecords: &returnRecordsDefault,
		ReturnTimeout: &returnTimeoutDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) WithTimeout(timeout time.Duration) *ResourceTagResourceCollectionGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) WithContext(ctx context.Context) *ResourceTagResourceCollectionGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) WithHTTPClient(client *http.Client) *ResourceTagResourceCollectionGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithFields adds the fields to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) WithFields(fields []string) *ResourceTagResourceCollectionGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithHref adds the href to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) WithHref(href *string) *ResourceTagResourceCollectionGetParams {
	o.SetHref(href)
	return o
}

// SetHref adds the href to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) SetHref(href *string) {
	o.Href = href
}

// WithLabel adds the label to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) WithLabel(label *string) *ResourceTagResourceCollectionGetParams {
	o.SetLabel(label)
	return o
}

// SetLabel adds the label to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) SetLabel(label *string) {
	o.Label = label
}

// WithMaxRecords adds the maxRecords to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) WithMaxRecords(maxRecords *int64) *ResourceTagResourceCollectionGetParams {
	o.SetMaxRecords(maxRecords)
	return o
}

// SetMaxRecords adds the maxRecords to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) SetMaxRecords(maxRecords *int64) {
	o.MaxRecords = maxRecords
}

// WithOrderBy adds the orderBy to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) WithOrderBy(orderBy []string) *ResourceTagResourceCollectionGetParams {
	o.SetOrderBy(orderBy)
	return o
}

// SetOrderBy adds the orderBy to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) SetOrderBy(orderBy []string) {
	o.OrderBy = orderBy
}

// WithResourceTagValue adds the resourceTagValue to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) WithResourceTagValue(resourceTagValue string) *ResourceTagResourceCollectionGetParams {
	o.SetResourceTagValue(resourceTagValue)
	return o
}

// SetResourceTagValue adds the resourceTagValue to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) SetResourceTagValue(resourceTagValue string) {
	o.ResourceTagValue = resourceTagValue
}

// WithReturnRecords adds the returnRecords to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) WithReturnRecords(returnRecords *bool) *ResourceTagResourceCollectionGetParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) WithReturnTimeout(returnTimeout *int64) *ResourceTagResourceCollectionGetParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithSvmName adds the svmName to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) WithSvmName(svmName *string) *ResourceTagResourceCollectionGetParams {
	o.SetSvmName(svmName)
	return o
}

// SetSvmName adds the svmName to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) SetSvmName(svmName *string) {
	o.SvmName = svmName
}

// WithSvmUUID adds the svmUUID to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) WithSvmUUID(svmUUID *string) *ResourceTagResourceCollectionGetParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) SetSvmUUID(svmUUID *string) {
	o.SvmUUID = svmUUID
}

// WithValue adds the value to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) WithValue(value *string) *ResourceTagResourceCollectionGetParams {
	o.SetValue(value)
	return o
}

// SetValue adds the value to the resource tag resource collection get params
func (o *ResourceTagResourceCollectionGetParams) SetValue(value *string) {
	o.Value = value
}

// WriteToRequest writes these params to a swagger request
func (o *ResourceTagResourceCollectionGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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

	if o.Href != nil {

		// query param href
		var qrHref string

		if o.Href != nil {
			qrHref = *o.Href
		}
		qHref := qrHref
		if qHref != "" {

			if err := r.SetQueryParam("href", qHref); err != nil {
				return err
			}
		}
	}

	if o.Label != nil {

		// query param label
		var qrLabel string

		if o.Label != nil {
			qrLabel = *o.Label
		}
		qLabel := qrLabel
		if qLabel != "" {

			if err := r.SetQueryParam("label", qLabel); err != nil {
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

	// path param resource_tag.value
	if err := r.SetPathParam("resource_tag.value", o.ResourceTagValue); err != nil {
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

	if o.Value != nil {

		// query param value
		var qrValue string

		if o.Value != nil {
			qrValue = *o.Value
		}
		qValue := qrValue
		if qValue != "" {

			if err := r.SetQueryParam("value", qValue); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamResourceTagResourceCollectionGet binds the parameter fields
func (o *ResourceTagResourceCollectionGetParams) bindParamFields(formats strfmt.Registry) []string {
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

// bindParamResourceTagResourceCollectionGet binds the parameter order_by
func (o *ResourceTagResourceCollectionGetParams) bindParamOrderBy(formats strfmt.Registry) []string {
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
