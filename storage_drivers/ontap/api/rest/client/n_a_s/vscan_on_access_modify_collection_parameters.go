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

// NewVscanOnAccessModifyCollectionParams creates a new VscanOnAccessModifyCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewVscanOnAccessModifyCollectionParams() *VscanOnAccessModifyCollectionParams {
	return &VscanOnAccessModifyCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewVscanOnAccessModifyCollectionParamsWithTimeout creates a new VscanOnAccessModifyCollectionParams object
// with the ability to set a timeout on a request.
func NewVscanOnAccessModifyCollectionParamsWithTimeout(timeout time.Duration) *VscanOnAccessModifyCollectionParams {
	return &VscanOnAccessModifyCollectionParams{
		timeout: timeout,
	}
}

// NewVscanOnAccessModifyCollectionParamsWithContext creates a new VscanOnAccessModifyCollectionParams object
// with the ability to set a context for a request.
func NewVscanOnAccessModifyCollectionParamsWithContext(ctx context.Context) *VscanOnAccessModifyCollectionParams {
	return &VscanOnAccessModifyCollectionParams{
		Context: ctx,
	}
}

// NewVscanOnAccessModifyCollectionParamsWithHTTPClient creates a new VscanOnAccessModifyCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewVscanOnAccessModifyCollectionParamsWithHTTPClient(client *http.Client) *VscanOnAccessModifyCollectionParams {
	return &VscanOnAccessModifyCollectionParams{
		HTTPClient: client,
	}
}

/*
VscanOnAccessModifyCollectionParams contains all the parameters to send to the API endpoint

	for the vscan on access modify collection operation.

	Typically these are written to a http.Request.
*/
type VscanOnAccessModifyCollectionParams struct {

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* Enabled.

	   Filter by enabled
	*/
	Enabled *bool

	/* Info.

	   Info specification
	*/
	Info VscanOnAccessModifyCollectionBody

	/* Mandatory.

	   Filter by mandatory
	*/
	Mandatory *bool

	/* Name.

	   Filter by name
	*/
	Name *string

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

	/* ScopeExcludeExtensions.

	   Filter by scope.exclude_extensions
	*/
	ScopeExcludeExtensions *string

	/* ScopeExcludePaths.

	   Filter by scope.exclude_paths
	*/
	ScopeExcludePaths *string

	/* ScopeIncludeExtensions.

	   Filter by scope.include_extensions
	*/
	ScopeIncludeExtensions *string

	/* ScopeMaxFileSize.

	   Filter by scope.max_file_size
	*/
	ScopeMaxFileSize *int64

	/* ScopeOnlyExecuteAccess.

	   Filter by scope.only_execute_access
	*/
	ScopeOnlyExecuteAccess *bool

	/* ScopeScanReadonlyVolumes.

	   Filter by scope.scan_readonly_volumes
	*/
	ScopeScanReadonlyVolumes *bool

	/* ScopeScanWithoutExtension.

	   Filter by scope.scan_without_extension
	*/
	ScopeScanWithoutExtension *bool

	/* SerialRecords.

	   Perform the operation on the records synchronously.
	*/
	SerialRecords *bool

	/* SvmName.

	   Filter by svm.name
	*/
	SvmName *string

	/* SvmUUID.

	   UUID of the SVM to which this object belongs.
	*/
	SvmUUID string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the vscan on access modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *VscanOnAccessModifyCollectionParams) WithDefaults() *VscanOnAccessModifyCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the vscan on access modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *VscanOnAccessModifyCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := VscanOnAccessModifyCollectionParams{
		ContinueOnFailure: &continueOnFailureDefault,
		ReturnRecords:     &returnRecordsDefault,
		ReturnTimeout:     &returnTimeoutDefault,
		SerialRecords:     &serialRecordsDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithTimeout(timeout time.Duration) *VscanOnAccessModifyCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithContext(ctx context.Context) *VscanOnAccessModifyCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithHTTPClient(client *http.Client) *VscanOnAccessModifyCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithContinueOnFailure adds the continueOnFailure to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *VscanOnAccessModifyCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithEnabled adds the enabled to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithEnabled(enabled *bool) *VscanOnAccessModifyCollectionParams {
	o.SetEnabled(enabled)
	return o
}

// SetEnabled adds the enabled to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetEnabled(enabled *bool) {
	o.Enabled = enabled
}

// WithInfo adds the info to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithInfo(info VscanOnAccessModifyCollectionBody) *VscanOnAccessModifyCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetInfo(info VscanOnAccessModifyCollectionBody) {
	o.Info = info
}

// WithMandatory adds the mandatory to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithMandatory(mandatory *bool) *VscanOnAccessModifyCollectionParams {
	o.SetMandatory(mandatory)
	return o
}

// SetMandatory adds the mandatory to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetMandatory(mandatory *bool) {
	o.Mandatory = mandatory
}

// WithName adds the name to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithName(name *string) *VscanOnAccessModifyCollectionParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetName(name *string) {
	o.Name = name
}

// WithReturnRecords adds the returnRecords to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithReturnRecords(returnRecords *bool) *VscanOnAccessModifyCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithReturnTimeout(returnTimeout *int64) *VscanOnAccessModifyCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithScopeExcludeExtensions adds the scopeExcludeExtensions to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithScopeExcludeExtensions(scopeExcludeExtensions *string) *VscanOnAccessModifyCollectionParams {
	o.SetScopeExcludeExtensions(scopeExcludeExtensions)
	return o
}

// SetScopeExcludeExtensions adds the scopeExcludeExtensions to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetScopeExcludeExtensions(scopeExcludeExtensions *string) {
	o.ScopeExcludeExtensions = scopeExcludeExtensions
}

// WithScopeExcludePaths adds the scopeExcludePaths to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithScopeExcludePaths(scopeExcludePaths *string) *VscanOnAccessModifyCollectionParams {
	o.SetScopeExcludePaths(scopeExcludePaths)
	return o
}

// SetScopeExcludePaths adds the scopeExcludePaths to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetScopeExcludePaths(scopeExcludePaths *string) {
	o.ScopeExcludePaths = scopeExcludePaths
}

// WithScopeIncludeExtensions adds the scopeIncludeExtensions to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithScopeIncludeExtensions(scopeIncludeExtensions *string) *VscanOnAccessModifyCollectionParams {
	o.SetScopeIncludeExtensions(scopeIncludeExtensions)
	return o
}

// SetScopeIncludeExtensions adds the scopeIncludeExtensions to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetScopeIncludeExtensions(scopeIncludeExtensions *string) {
	o.ScopeIncludeExtensions = scopeIncludeExtensions
}

// WithScopeMaxFileSize adds the scopeMaxFileSize to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithScopeMaxFileSize(scopeMaxFileSize *int64) *VscanOnAccessModifyCollectionParams {
	o.SetScopeMaxFileSize(scopeMaxFileSize)
	return o
}

// SetScopeMaxFileSize adds the scopeMaxFileSize to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetScopeMaxFileSize(scopeMaxFileSize *int64) {
	o.ScopeMaxFileSize = scopeMaxFileSize
}

// WithScopeOnlyExecuteAccess adds the scopeOnlyExecuteAccess to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithScopeOnlyExecuteAccess(scopeOnlyExecuteAccess *bool) *VscanOnAccessModifyCollectionParams {
	o.SetScopeOnlyExecuteAccess(scopeOnlyExecuteAccess)
	return o
}

// SetScopeOnlyExecuteAccess adds the scopeOnlyExecuteAccess to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetScopeOnlyExecuteAccess(scopeOnlyExecuteAccess *bool) {
	o.ScopeOnlyExecuteAccess = scopeOnlyExecuteAccess
}

// WithScopeScanReadonlyVolumes adds the scopeScanReadonlyVolumes to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithScopeScanReadonlyVolumes(scopeScanReadonlyVolumes *bool) *VscanOnAccessModifyCollectionParams {
	o.SetScopeScanReadonlyVolumes(scopeScanReadonlyVolumes)
	return o
}

// SetScopeScanReadonlyVolumes adds the scopeScanReadonlyVolumes to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetScopeScanReadonlyVolumes(scopeScanReadonlyVolumes *bool) {
	o.ScopeScanReadonlyVolumes = scopeScanReadonlyVolumes
}

// WithScopeScanWithoutExtension adds the scopeScanWithoutExtension to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithScopeScanWithoutExtension(scopeScanWithoutExtension *bool) *VscanOnAccessModifyCollectionParams {
	o.SetScopeScanWithoutExtension(scopeScanWithoutExtension)
	return o
}

// SetScopeScanWithoutExtension adds the scopeScanWithoutExtension to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetScopeScanWithoutExtension(scopeScanWithoutExtension *bool) {
	o.ScopeScanWithoutExtension = scopeScanWithoutExtension
}

// WithSerialRecords adds the serialRecords to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithSerialRecords(serialRecords *bool) *VscanOnAccessModifyCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WithSvmName adds the svmName to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithSvmName(svmName *string) *VscanOnAccessModifyCollectionParams {
	o.SetSvmName(svmName)
	return o
}

// SetSvmName adds the svmName to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetSvmName(svmName *string) {
	o.SvmName = svmName
}

// WithSvmUUID adds the svmUUID to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) WithSvmUUID(svmUUID string) *VscanOnAccessModifyCollectionParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the vscan on access modify collection params
func (o *VscanOnAccessModifyCollectionParams) SetSvmUUID(svmUUID string) {
	o.SvmUUID = svmUUID
}

// WriteToRequest writes these params to a swagger request
func (o *VscanOnAccessModifyCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.ContinueOnFailure != nil {

		// query param continue_on_failure
		var qrContinueOnFailure bool

		if o.ContinueOnFailure != nil {
			qrContinueOnFailure = *o.ContinueOnFailure
		}
		qContinueOnFailure := swag.FormatBool(qrContinueOnFailure)
		if qContinueOnFailure != "" {

			if err := r.SetQueryParam("continue_on_failure", qContinueOnFailure); err != nil {
				return err
			}
		}
	}

	if o.Enabled != nil {

		// query param enabled
		var qrEnabled bool

		if o.Enabled != nil {
			qrEnabled = *o.Enabled
		}
		qEnabled := swag.FormatBool(qrEnabled)
		if qEnabled != "" {

			if err := r.SetQueryParam("enabled", qEnabled); err != nil {
				return err
			}
		}
	}
	if err := r.SetBodyParam(o.Info); err != nil {
		return err
	}

	if o.Mandatory != nil {

		// query param mandatory
		var qrMandatory bool

		if o.Mandatory != nil {
			qrMandatory = *o.Mandatory
		}
		qMandatory := swag.FormatBool(qrMandatory)
		if qMandatory != "" {

			if err := r.SetQueryParam("mandatory", qMandatory); err != nil {
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

	if o.ScopeExcludeExtensions != nil {

		// query param scope.exclude_extensions
		var qrScopeExcludeExtensions string

		if o.ScopeExcludeExtensions != nil {
			qrScopeExcludeExtensions = *o.ScopeExcludeExtensions
		}
		qScopeExcludeExtensions := qrScopeExcludeExtensions
		if qScopeExcludeExtensions != "" {

			if err := r.SetQueryParam("scope.exclude_extensions", qScopeExcludeExtensions); err != nil {
				return err
			}
		}
	}

	if o.ScopeExcludePaths != nil {

		// query param scope.exclude_paths
		var qrScopeExcludePaths string

		if o.ScopeExcludePaths != nil {
			qrScopeExcludePaths = *o.ScopeExcludePaths
		}
		qScopeExcludePaths := qrScopeExcludePaths
		if qScopeExcludePaths != "" {

			if err := r.SetQueryParam("scope.exclude_paths", qScopeExcludePaths); err != nil {
				return err
			}
		}
	}

	if o.ScopeIncludeExtensions != nil {

		// query param scope.include_extensions
		var qrScopeIncludeExtensions string

		if o.ScopeIncludeExtensions != nil {
			qrScopeIncludeExtensions = *o.ScopeIncludeExtensions
		}
		qScopeIncludeExtensions := qrScopeIncludeExtensions
		if qScopeIncludeExtensions != "" {

			if err := r.SetQueryParam("scope.include_extensions", qScopeIncludeExtensions); err != nil {
				return err
			}
		}
	}

	if o.ScopeMaxFileSize != nil {

		// query param scope.max_file_size
		var qrScopeMaxFileSize int64

		if o.ScopeMaxFileSize != nil {
			qrScopeMaxFileSize = *o.ScopeMaxFileSize
		}
		qScopeMaxFileSize := swag.FormatInt64(qrScopeMaxFileSize)
		if qScopeMaxFileSize != "" {

			if err := r.SetQueryParam("scope.max_file_size", qScopeMaxFileSize); err != nil {
				return err
			}
		}
	}

	if o.ScopeOnlyExecuteAccess != nil {

		// query param scope.only_execute_access
		var qrScopeOnlyExecuteAccess bool

		if o.ScopeOnlyExecuteAccess != nil {
			qrScopeOnlyExecuteAccess = *o.ScopeOnlyExecuteAccess
		}
		qScopeOnlyExecuteAccess := swag.FormatBool(qrScopeOnlyExecuteAccess)
		if qScopeOnlyExecuteAccess != "" {

			if err := r.SetQueryParam("scope.only_execute_access", qScopeOnlyExecuteAccess); err != nil {
				return err
			}
		}
	}

	if o.ScopeScanReadonlyVolumes != nil {

		// query param scope.scan_readonly_volumes
		var qrScopeScanReadonlyVolumes bool

		if o.ScopeScanReadonlyVolumes != nil {
			qrScopeScanReadonlyVolumes = *o.ScopeScanReadonlyVolumes
		}
		qScopeScanReadonlyVolumes := swag.FormatBool(qrScopeScanReadonlyVolumes)
		if qScopeScanReadonlyVolumes != "" {

			if err := r.SetQueryParam("scope.scan_readonly_volumes", qScopeScanReadonlyVolumes); err != nil {
				return err
			}
		}
	}

	if o.ScopeScanWithoutExtension != nil {

		// query param scope.scan_without_extension
		var qrScopeScanWithoutExtension bool

		if o.ScopeScanWithoutExtension != nil {
			qrScopeScanWithoutExtension = *o.ScopeScanWithoutExtension
		}
		qScopeScanWithoutExtension := swag.FormatBool(qrScopeScanWithoutExtension)
		if qScopeScanWithoutExtension != "" {

			if err := r.SetQueryParam("scope.scan_without_extension", qScopeScanWithoutExtension); err != nil {
				return err
			}
		}
	}

	if o.SerialRecords != nil {

		// query param serial_records
		var qrSerialRecords bool

		if o.SerialRecords != nil {
			qrSerialRecords = *o.SerialRecords
		}
		qSerialRecords := swag.FormatBool(qrSerialRecords)
		if qSerialRecords != "" {

			if err := r.SetQueryParam("serial_records", qSerialRecords); err != nil {
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

	// path param svm.uuid
	if err := r.SetPathParam("svm.uuid", o.SvmUUID); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}