// Code generated by go-swagger; DO NOT EDIT.

package support

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

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// NewEmsRoleConfigModifyParams creates a new EmsRoleConfigModifyParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewEmsRoleConfigModifyParams() *EmsRoleConfigModifyParams {
	return &EmsRoleConfigModifyParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewEmsRoleConfigModifyParamsWithTimeout creates a new EmsRoleConfigModifyParams object
// with the ability to set a timeout on a request.
func NewEmsRoleConfigModifyParamsWithTimeout(timeout time.Duration) *EmsRoleConfigModifyParams {
	return &EmsRoleConfigModifyParams{
		timeout: timeout,
	}
}

// NewEmsRoleConfigModifyParamsWithContext creates a new EmsRoleConfigModifyParams object
// with the ability to set a context for a request.
func NewEmsRoleConfigModifyParamsWithContext(ctx context.Context) *EmsRoleConfigModifyParams {
	return &EmsRoleConfigModifyParams{
		Context: ctx,
	}
}

// NewEmsRoleConfigModifyParamsWithHTTPClient creates a new EmsRoleConfigModifyParams object
// with the ability to set a custom HTTPClient for a request.
func NewEmsRoleConfigModifyParamsWithHTTPClient(client *http.Client) *EmsRoleConfigModifyParams {
	return &EmsRoleConfigModifyParams{
		HTTPClient: client,
	}
}

/*
EmsRoleConfigModifyParams contains all the parameters to send to the API endpoint

	for the ems role config modify operation.

	Typically these are written to a http.Request.
*/
type EmsRoleConfigModifyParams struct {

	/* AccessControlRoleName.

	   Access control role name
	*/
	AccessControlRoleName string

	/* Info.

	   Information specification
	*/
	Info *models.EmsRoleConfig

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the ems role config modify params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *EmsRoleConfigModifyParams) WithDefaults() *EmsRoleConfigModifyParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the ems role config modify params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *EmsRoleConfigModifyParams) SetDefaults() {
	// no default values defined for this parameter
}

// WithTimeout adds the timeout to the ems role config modify params
func (o *EmsRoleConfigModifyParams) WithTimeout(timeout time.Duration) *EmsRoleConfigModifyParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the ems role config modify params
func (o *EmsRoleConfigModifyParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the ems role config modify params
func (o *EmsRoleConfigModifyParams) WithContext(ctx context.Context) *EmsRoleConfigModifyParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the ems role config modify params
func (o *EmsRoleConfigModifyParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the ems role config modify params
func (o *EmsRoleConfigModifyParams) WithHTTPClient(client *http.Client) *EmsRoleConfigModifyParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the ems role config modify params
func (o *EmsRoleConfigModifyParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithAccessControlRoleName adds the accessControlRoleName to the ems role config modify params
func (o *EmsRoleConfigModifyParams) WithAccessControlRoleName(accessControlRoleName string) *EmsRoleConfigModifyParams {
	o.SetAccessControlRoleName(accessControlRoleName)
	return o
}

// SetAccessControlRoleName adds the accessControlRoleName to the ems role config modify params
func (o *EmsRoleConfigModifyParams) SetAccessControlRoleName(accessControlRoleName string) {
	o.AccessControlRoleName = accessControlRoleName
}

// WithInfo adds the info to the ems role config modify params
func (o *EmsRoleConfigModifyParams) WithInfo(info *models.EmsRoleConfig) *EmsRoleConfigModifyParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the ems role config modify params
func (o *EmsRoleConfigModifyParams) SetInfo(info *models.EmsRoleConfig) {
	o.Info = info
}

// WriteToRequest writes these params to a swagger request
func (o *EmsRoleConfigModifyParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	// path param access_control_role.name
	if err := r.SetPathParam("access_control_role.name", o.AccessControlRoleName); err != nil {
		return err
	}
	if o.Info != nil {
		if err := r.SetBodyParam(o.Info); err != nil {
			return err
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
