// Code generated by go-swagger; DO NOT EDIT.

package storage

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

// NewStorageAvailabilityZoneGetParams creates a new StorageAvailabilityZoneGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewStorageAvailabilityZoneGetParams() *StorageAvailabilityZoneGetParams {
	return &StorageAvailabilityZoneGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewStorageAvailabilityZoneGetParamsWithTimeout creates a new StorageAvailabilityZoneGetParams object
// with the ability to set a timeout on a request.
func NewStorageAvailabilityZoneGetParamsWithTimeout(timeout time.Duration) *StorageAvailabilityZoneGetParams {
	return &StorageAvailabilityZoneGetParams{
		timeout: timeout,
	}
}

// NewStorageAvailabilityZoneGetParamsWithContext creates a new StorageAvailabilityZoneGetParams object
// with the ability to set a context for a request.
func NewStorageAvailabilityZoneGetParamsWithContext(ctx context.Context) *StorageAvailabilityZoneGetParams {
	return &StorageAvailabilityZoneGetParams{
		Context: ctx,
	}
}

// NewStorageAvailabilityZoneGetParamsWithHTTPClient creates a new StorageAvailabilityZoneGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewStorageAvailabilityZoneGetParamsWithHTTPClient(client *http.Client) *StorageAvailabilityZoneGetParams {
	return &StorageAvailabilityZoneGetParams{
		HTTPClient: client,
	}
}

/*
StorageAvailabilityZoneGetParams contains all the parameters to send to the API endpoint

	for the storage availability zone get operation.

	Typically these are written to a http.Request.
*/
type StorageAvailabilityZoneGetParams struct {

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* UUID.

	   Availability zone UUID
	*/
	UUID string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the storage availability zone get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *StorageAvailabilityZoneGetParams) WithDefaults() *StorageAvailabilityZoneGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the storage availability zone get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *StorageAvailabilityZoneGetParams) SetDefaults() {
	// no default values defined for this parameter
}

// WithTimeout adds the timeout to the storage availability zone get params
func (o *StorageAvailabilityZoneGetParams) WithTimeout(timeout time.Duration) *StorageAvailabilityZoneGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the storage availability zone get params
func (o *StorageAvailabilityZoneGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the storage availability zone get params
func (o *StorageAvailabilityZoneGetParams) WithContext(ctx context.Context) *StorageAvailabilityZoneGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the storage availability zone get params
func (o *StorageAvailabilityZoneGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the storage availability zone get params
func (o *StorageAvailabilityZoneGetParams) WithHTTPClient(client *http.Client) *StorageAvailabilityZoneGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the storage availability zone get params
func (o *StorageAvailabilityZoneGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithFields adds the fields to the storage availability zone get params
func (o *StorageAvailabilityZoneGetParams) WithFields(fields []string) *StorageAvailabilityZoneGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the storage availability zone get params
func (o *StorageAvailabilityZoneGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithUUID adds the uuid to the storage availability zone get params
func (o *StorageAvailabilityZoneGetParams) WithUUID(uuid string) *StorageAvailabilityZoneGetParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the storage availability zone get params
func (o *StorageAvailabilityZoneGetParams) SetUUID(uuid string) {
	o.UUID = uuid
}

// WriteToRequest writes these params to a swagger request
func (o *StorageAvailabilityZoneGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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

	// path param uuid
	if err := r.SetPathParam("uuid", o.UUID); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamStorageAvailabilityZoneGet binds the parameter fields
func (o *StorageAvailabilityZoneGetParams) bindParamFields(formats strfmt.Registry) []string {
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
