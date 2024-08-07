// Code generated by go-swagger; DO NOT EDIT.

package n_a_s

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

// NewEffectivePermissionGetParams creates a new EffectivePermissionGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewEffectivePermissionGetParams() *EffectivePermissionGetParams {
	return &EffectivePermissionGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewEffectivePermissionGetParamsWithTimeout creates a new EffectivePermissionGetParams object
// with the ability to set a timeout on a request.
func NewEffectivePermissionGetParamsWithTimeout(timeout time.Duration) *EffectivePermissionGetParams {
	return &EffectivePermissionGetParams{
		timeout: timeout,
	}
}

// NewEffectivePermissionGetParamsWithContext creates a new EffectivePermissionGetParams object
// with the ability to set a context for a request.
func NewEffectivePermissionGetParamsWithContext(ctx context.Context) *EffectivePermissionGetParams {
	return &EffectivePermissionGetParams{
		Context: ctx,
	}
}

// NewEffectivePermissionGetParamsWithHTTPClient creates a new EffectivePermissionGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewEffectivePermissionGetParamsWithHTTPClient(client *http.Client) *EffectivePermissionGetParams {
	return &EffectivePermissionGetParams{
		HTTPClient: client,
	}
}

/*
EffectivePermissionGetParams contains all the parameters to send to the API endpoint

	for the effective permission get operation.

	Typically these are written to a http.Request.
*/
type EffectivePermissionGetParams struct {

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* Path.

	   File Path
	*/
	Path string

	/* ShareName.

	   Share Name
	*/
	ShareName *string

	/* SvmUUID.

	   UUID of the SVM to which this object belongs.
	*/
	SvmUUID string

	/* Type.

	   User Type
	*/
	Type *string

	/* User.

	   User_Name
	*/
	User string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the effective permission get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *EffectivePermissionGetParams) WithDefaults() *EffectivePermissionGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the effective permission get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *EffectivePermissionGetParams) SetDefaults() {
	// no default values defined for this parameter
}

// WithTimeout adds the timeout to the effective permission get params
func (o *EffectivePermissionGetParams) WithTimeout(timeout time.Duration) *EffectivePermissionGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the effective permission get params
func (o *EffectivePermissionGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the effective permission get params
func (o *EffectivePermissionGetParams) WithContext(ctx context.Context) *EffectivePermissionGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the effective permission get params
func (o *EffectivePermissionGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the effective permission get params
func (o *EffectivePermissionGetParams) WithHTTPClient(client *http.Client) *EffectivePermissionGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the effective permission get params
func (o *EffectivePermissionGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithFields adds the fields to the effective permission get params
func (o *EffectivePermissionGetParams) WithFields(fields []string) *EffectivePermissionGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the effective permission get params
func (o *EffectivePermissionGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithPath adds the path to the effective permission get params
func (o *EffectivePermissionGetParams) WithPath(path string) *EffectivePermissionGetParams {
	o.SetPath(path)
	return o
}

// SetPath adds the path to the effective permission get params
func (o *EffectivePermissionGetParams) SetPath(path string) {
	o.Path = path
}

// WithShareName adds the shareName to the effective permission get params
func (o *EffectivePermissionGetParams) WithShareName(shareName *string) *EffectivePermissionGetParams {
	o.SetShareName(shareName)
	return o
}

// SetShareName adds the shareName to the effective permission get params
func (o *EffectivePermissionGetParams) SetShareName(shareName *string) {
	o.ShareName = shareName
}

// WithSvmUUID adds the svmUUID to the effective permission get params
func (o *EffectivePermissionGetParams) WithSvmUUID(svmUUID string) *EffectivePermissionGetParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the effective permission get params
func (o *EffectivePermissionGetParams) SetSvmUUID(svmUUID string) {
	o.SvmUUID = svmUUID
}

// WithType adds the typeVar to the effective permission get params
func (o *EffectivePermissionGetParams) WithType(typeVar *string) *EffectivePermissionGetParams {
	o.SetType(typeVar)
	return o
}

// SetType adds the type to the effective permission get params
func (o *EffectivePermissionGetParams) SetType(typeVar *string) {
	o.Type = typeVar
}

// WithUser adds the user to the effective permission get params
func (o *EffectivePermissionGetParams) WithUser(user string) *EffectivePermissionGetParams {
	o.SetUser(user)
	return o
}

// SetUser adds the user to the effective permission get params
func (o *EffectivePermissionGetParams) SetUser(user string) {
	o.User = user
}

// WriteToRequest writes these params to a swagger request
func (o *EffectivePermissionGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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

	// path param path
	if err := r.SetPathParam("path", o.Path); err != nil {
		return err
	}

	if o.ShareName != nil {

		// query param share.name
		var qrShareName string

		if o.ShareName != nil {
			qrShareName = *o.ShareName
		}
		qShareName := qrShareName
		if qShareName != "" {

			if err := r.SetQueryParam("share.name", qShareName); err != nil {
				return err
			}
		}
	}

	// path param svm.uuid
	if err := r.SetPathParam("svm.uuid", o.SvmUUID); err != nil {
		return err
	}

	if o.Type != nil {

		// query param type
		var qrType string

		if o.Type != nil {
			qrType = *o.Type
		}
		qType := qrType
		if qType != "" {

			if err := r.SetQueryParam("type", qType); err != nil {
				return err
			}
		}
	}

	// query param user
	qrUser := o.User
	qUser := qrUser
	if qUser != "" {

		if err := r.SetQueryParam("user", qUser); err != nil {
			return err
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamEffectivePermissionGet binds the parameter fields
func (o *EffectivePermissionGetParams) bindParamFields(formats strfmt.Registry) []string {
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
