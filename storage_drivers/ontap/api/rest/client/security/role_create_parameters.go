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

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// NewRoleCreateParams creates a new RoleCreateParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewRoleCreateParams() *RoleCreateParams {
	return &RoleCreateParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewRoleCreateParamsWithTimeout creates a new RoleCreateParams object
// with the ability to set a timeout on a request.
func NewRoleCreateParamsWithTimeout(timeout time.Duration) *RoleCreateParams {
	return &RoleCreateParams{
		timeout: timeout,
	}
}

// NewRoleCreateParamsWithContext creates a new RoleCreateParams object
// with the ability to set a context for a request.
func NewRoleCreateParamsWithContext(ctx context.Context) *RoleCreateParams {
	return &RoleCreateParams{
		Context: ctx,
	}
}

// NewRoleCreateParamsWithHTTPClient creates a new RoleCreateParams object
// with the ability to set a custom HTTPClient for a request.
func NewRoleCreateParamsWithHTTPClient(client *http.Client) *RoleCreateParams {
	return &RoleCreateParams{
		HTTPClient: client,
	}
}

/* RoleCreateParams contains all the parameters to send to the API endpoint
   for the role create operation.

   Typically these are written to a http.Request.
*/
type RoleCreateParams struct {

	/* Info.

	   Role specification
	*/
	Info *models.Role

	/* ReturnRecords.

	   The default is false.  If set to true, the records are returned.
	*/
	ReturnRecords *bool

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the role create params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *RoleCreateParams) WithDefaults() *RoleCreateParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the role create params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *RoleCreateParams) SetDefaults() {
	var (
		returnRecordsDefault = bool(false)
	)

	val := RoleCreateParams{
		ReturnRecords: &returnRecordsDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the role create params
func (o *RoleCreateParams) WithTimeout(timeout time.Duration) *RoleCreateParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the role create params
func (o *RoleCreateParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the role create params
func (o *RoleCreateParams) WithContext(ctx context.Context) *RoleCreateParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the role create params
func (o *RoleCreateParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the role create params
func (o *RoleCreateParams) WithHTTPClient(client *http.Client) *RoleCreateParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the role create params
func (o *RoleCreateParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithInfo adds the info to the role create params
func (o *RoleCreateParams) WithInfo(info *models.Role) *RoleCreateParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the role create params
func (o *RoleCreateParams) SetInfo(info *models.Role) {
	o.Info = info
}

// WithReturnRecords adds the returnRecords to the role create params
func (o *RoleCreateParams) WithReturnRecords(returnRecords *bool) *RoleCreateParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the role create params
func (o *RoleCreateParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WriteToRequest writes these params to a swagger request
func (o *RoleCreateParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error
	if o.Info != nil {
		if err := r.SetBodyParam(o.Info); err != nil {
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

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}