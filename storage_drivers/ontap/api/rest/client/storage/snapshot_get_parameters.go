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

// NewSnapshotGetParams creates a new SnapshotGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewSnapshotGetParams() *SnapshotGetParams {
	return &SnapshotGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewSnapshotGetParamsWithTimeout creates a new SnapshotGetParams object
// with the ability to set a timeout on a request.
func NewSnapshotGetParamsWithTimeout(timeout time.Duration) *SnapshotGetParams {
	return &SnapshotGetParams{
		timeout: timeout,
	}
}

// NewSnapshotGetParamsWithContext creates a new SnapshotGetParams object
// with the ability to set a context for a request.
func NewSnapshotGetParamsWithContext(ctx context.Context) *SnapshotGetParams {
	return &SnapshotGetParams{
		Context: ctx,
	}
}

// NewSnapshotGetParamsWithHTTPClient creates a new SnapshotGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewSnapshotGetParamsWithHTTPClient(client *http.Client) *SnapshotGetParams {
	return &SnapshotGetParams{
		HTTPClient: client,
	}
}

/*
SnapshotGetParams contains all the parameters to send to the API endpoint

	for the snapshot get operation.

	Typically these are written to a http.Request.
*/
type SnapshotGetParams struct {

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* UUID.

	   Snapshot UUID
	*/
	UUID string

	/* VolumeUUID.

	   Volume UUID
	*/
	VolumeUUID string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the snapshot get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *SnapshotGetParams) WithDefaults() *SnapshotGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the snapshot get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *SnapshotGetParams) SetDefaults() {
	// no default values defined for this parameter
}

// WithTimeout adds the timeout to the snapshot get params
func (o *SnapshotGetParams) WithTimeout(timeout time.Duration) *SnapshotGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the snapshot get params
func (o *SnapshotGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the snapshot get params
func (o *SnapshotGetParams) WithContext(ctx context.Context) *SnapshotGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the snapshot get params
func (o *SnapshotGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the snapshot get params
func (o *SnapshotGetParams) WithHTTPClient(client *http.Client) *SnapshotGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the snapshot get params
func (o *SnapshotGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithFields adds the fields to the snapshot get params
func (o *SnapshotGetParams) WithFields(fields []string) *SnapshotGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the snapshot get params
func (o *SnapshotGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithUUID adds the uuid to the snapshot get params
func (o *SnapshotGetParams) WithUUID(uuid string) *SnapshotGetParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the snapshot get params
func (o *SnapshotGetParams) SetUUID(uuid string) {
	o.UUID = uuid
}

// WithVolumeUUID adds the volumeUUID to the snapshot get params
func (o *SnapshotGetParams) WithVolumeUUID(volumeUUID string) *SnapshotGetParams {
	o.SetVolumeUUID(volumeUUID)
	return o
}

// SetVolumeUUID adds the volumeUuid to the snapshot get params
func (o *SnapshotGetParams) SetVolumeUUID(volumeUUID string) {
	o.VolumeUUID = volumeUUID
}

// WriteToRequest writes these params to a swagger request
func (o *SnapshotGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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

	// path param volume.uuid
	if err := r.SetPathParam("volume.uuid", o.VolumeUUID); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamSnapshotGet binds the parameter fields
func (o *SnapshotGetParams) bindParamFields(formats strfmt.Registry) []string {
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
