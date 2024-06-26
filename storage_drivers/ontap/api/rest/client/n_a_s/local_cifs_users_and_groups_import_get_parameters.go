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

// NewLocalCifsUsersAndGroupsImportGetParams creates a new LocalCifsUsersAndGroupsImportGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewLocalCifsUsersAndGroupsImportGetParams() *LocalCifsUsersAndGroupsImportGetParams {
	return &LocalCifsUsersAndGroupsImportGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewLocalCifsUsersAndGroupsImportGetParamsWithTimeout creates a new LocalCifsUsersAndGroupsImportGetParams object
// with the ability to set a timeout on a request.
func NewLocalCifsUsersAndGroupsImportGetParamsWithTimeout(timeout time.Duration) *LocalCifsUsersAndGroupsImportGetParams {
	return &LocalCifsUsersAndGroupsImportGetParams{
		timeout: timeout,
	}
}

// NewLocalCifsUsersAndGroupsImportGetParamsWithContext creates a new LocalCifsUsersAndGroupsImportGetParams object
// with the ability to set a context for a request.
func NewLocalCifsUsersAndGroupsImportGetParamsWithContext(ctx context.Context) *LocalCifsUsersAndGroupsImportGetParams {
	return &LocalCifsUsersAndGroupsImportGetParams{
		Context: ctx,
	}
}

// NewLocalCifsUsersAndGroupsImportGetParamsWithHTTPClient creates a new LocalCifsUsersAndGroupsImportGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewLocalCifsUsersAndGroupsImportGetParamsWithHTTPClient(client *http.Client) *LocalCifsUsersAndGroupsImportGetParams {
	return &LocalCifsUsersAndGroupsImportGetParams{
		HTTPClient: client,
	}
}

/*
LocalCifsUsersAndGroupsImportGetParams contains all the parameters to send to the API endpoint

	for the local cifs users and groups import get operation.

	Typically these are written to a http.Request.
*/
type LocalCifsUsersAndGroupsImportGetParams struct {

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* SvmUUID.

	   UUID of the SVM to which this object belongs.
	*/
	SvmUUID string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the local cifs users and groups import get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *LocalCifsUsersAndGroupsImportGetParams) WithDefaults() *LocalCifsUsersAndGroupsImportGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the local cifs users and groups import get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *LocalCifsUsersAndGroupsImportGetParams) SetDefaults() {
	// no default values defined for this parameter
}

// WithTimeout adds the timeout to the local cifs users and groups import get params
func (o *LocalCifsUsersAndGroupsImportGetParams) WithTimeout(timeout time.Duration) *LocalCifsUsersAndGroupsImportGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the local cifs users and groups import get params
func (o *LocalCifsUsersAndGroupsImportGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the local cifs users and groups import get params
func (o *LocalCifsUsersAndGroupsImportGetParams) WithContext(ctx context.Context) *LocalCifsUsersAndGroupsImportGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the local cifs users and groups import get params
func (o *LocalCifsUsersAndGroupsImportGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the local cifs users and groups import get params
func (o *LocalCifsUsersAndGroupsImportGetParams) WithHTTPClient(client *http.Client) *LocalCifsUsersAndGroupsImportGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the local cifs users and groups import get params
func (o *LocalCifsUsersAndGroupsImportGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithFields adds the fields to the local cifs users and groups import get params
func (o *LocalCifsUsersAndGroupsImportGetParams) WithFields(fields []string) *LocalCifsUsersAndGroupsImportGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the local cifs users and groups import get params
func (o *LocalCifsUsersAndGroupsImportGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithSvmUUID adds the svmUUID to the local cifs users and groups import get params
func (o *LocalCifsUsersAndGroupsImportGetParams) WithSvmUUID(svmUUID string) *LocalCifsUsersAndGroupsImportGetParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the local cifs users and groups import get params
func (o *LocalCifsUsersAndGroupsImportGetParams) SetSvmUUID(svmUUID string) {
	o.SvmUUID = svmUUID
}

// WriteToRequest writes these params to a swagger request
func (o *LocalCifsUsersAndGroupsImportGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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

	// path param svm.uuid
	if err := r.SetPathParam("svm.uuid", o.SvmUUID); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamLocalCifsUsersAndGroupsImportGet binds the parameter fields
func (o *LocalCifsUsersAndGroupsImportGetParams) bindParamFields(formats strfmt.Registry) []string {
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
