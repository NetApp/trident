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

// NewVscanOnDemandModifyCollectionParams creates a new VscanOnDemandModifyCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewVscanOnDemandModifyCollectionParams() *VscanOnDemandModifyCollectionParams {
	return &VscanOnDemandModifyCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewVscanOnDemandModifyCollectionParamsWithTimeout creates a new VscanOnDemandModifyCollectionParams object
// with the ability to set a timeout on a request.
func NewVscanOnDemandModifyCollectionParamsWithTimeout(timeout time.Duration) *VscanOnDemandModifyCollectionParams {
	return &VscanOnDemandModifyCollectionParams{
		timeout: timeout,
	}
}

// NewVscanOnDemandModifyCollectionParamsWithContext creates a new VscanOnDemandModifyCollectionParams object
// with the ability to set a context for a request.
func NewVscanOnDemandModifyCollectionParamsWithContext(ctx context.Context) *VscanOnDemandModifyCollectionParams {
	return &VscanOnDemandModifyCollectionParams{
		Context: ctx,
	}
}

// NewVscanOnDemandModifyCollectionParamsWithHTTPClient creates a new VscanOnDemandModifyCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewVscanOnDemandModifyCollectionParamsWithHTTPClient(client *http.Client) *VscanOnDemandModifyCollectionParams {
	return &VscanOnDemandModifyCollectionParams{
		HTTPClient: client,
	}
}

/*
VscanOnDemandModifyCollectionParams contains all the parameters to send to the API endpoint

	for the vscan on demand modify collection operation.

	Typically these are written to a http.Request.
*/
type VscanOnDemandModifyCollectionParams struct {

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* Info.

	   Info specification
	*/
	Info VscanOnDemandModifyCollectionBody

	/* LogPath.

	   Filter by log_path
	*/
	LogPath *string

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

	/* ScanPaths.

	   Filter by scan_paths
	*/
	ScanPaths *string

	/* ScheduleName.

	   Filter by schedule.name
	*/
	ScheduleName *string

	/* ScheduleUUID.

	   Filter by schedule.uuid
	*/
	ScheduleUUID *string

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

// WithDefaults hydrates default values in the vscan on demand modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *VscanOnDemandModifyCollectionParams) WithDefaults() *VscanOnDemandModifyCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the vscan on demand modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *VscanOnDemandModifyCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := VscanOnDemandModifyCollectionParams{
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

// WithTimeout adds the timeout to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithTimeout(timeout time.Duration) *VscanOnDemandModifyCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithContext(ctx context.Context) *VscanOnDemandModifyCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithHTTPClient(client *http.Client) *VscanOnDemandModifyCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithContinueOnFailure adds the continueOnFailure to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *VscanOnDemandModifyCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithInfo adds the info to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithInfo(info VscanOnDemandModifyCollectionBody) *VscanOnDemandModifyCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetInfo(info VscanOnDemandModifyCollectionBody) {
	o.Info = info
}

// WithLogPath adds the logPath to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithLogPath(logPath *string) *VscanOnDemandModifyCollectionParams {
	o.SetLogPath(logPath)
	return o
}

// SetLogPath adds the logPath to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetLogPath(logPath *string) {
	o.LogPath = logPath
}

// WithName adds the name to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithName(name *string) *VscanOnDemandModifyCollectionParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetName(name *string) {
	o.Name = name
}

// WithReturnRecords adds the returnRecords to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithReturnRecords(returnRecords *bool) *VscanOnDemandModifyCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithReturnTimeout(returnTimeout *int64) *VscanOnDemandModifyCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithScanPaths adds the scanPaths to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithScanPaths(scanPaths *string) *VscanOnDemandModifyCollectionParams {
	o.SetScanPaths(scanPaths)
	return o
}

// SetScanPaths adds the scanPaths to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetScanPaths(scanPaths *string) {
	o.ScanPaths = scanPaths
}

// WithScheduleName adds the scheduleName to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithScheduleName(scheduleName *string) *VscanOnDemandModifyCollectionParams {
	o.SetScheduleName(scheduleName)
	return o
}

// SetScheduleName adds the scheduleName to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetScheduleName(scheduleName *string) {
	o.ScheduleName = scheduleName
}

// WithScheduleUUID adds the scheduleUUID to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithScheduleUUID(scheduleUUID *string) *VscanOnDemandModifyCollectionParams {
	o.SetScheduleUUID(scheduleUUID)
	return o
}

// SetScheduleUUID adds the scheduleUuid to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetScheduleUUID(scheduleUUID *string) {
	o.ScheduleUUID = scheduleUUID
}

// WithScopeExcludeExtensions adds the scopeExcludeExtensions to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithScopeExcludeExtensions(scopeExcludeExtensions *string) *VscanOnDemandModifyCollectionParams {
	o.SetScopeExcludeExtensions(scopeExcludeExtensions)
	return o
}

// SetScopeExcludeExtensions adds the scopeExcludeExtensions to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetScopeExcludeExtensions(scopeExcludeExtensions *string) {
	o.ScopeExcludeExtensions = scopeExcludeExtensions
}

// WithScopeExcludePaths adds the scopeExcludePaths to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithScopeExcludePaths(scopeExcludePaths *string) *VscanOnDemandModifyCollectionParams {
	o.SetScopeExcludePaths(scopeExcludePaths)
	return o
}

// SetScopeExcludePaths adds the scopeExcludePaths to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetScopeExcludePaths(scopeExcludePaths *string) {
	o.ScopeExcludePaths = scopeExcludePaths
}

// WithScopeIncludeExtensions adds the scopeIncludeExtensions to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithScopeIncludeExtensions(scopeIncludeExtensions *string) *VscanOnDemandModifyCollectionParams {
	o.SetScopeIncludeExtensions(scopeIncludeExtensions)
	return o
}

// SetScopeIncludeExtensions adds the scopeIncludeExtensions to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetScopeIncludeExtensions(scopeIncludeExtensions *string) {
	o.ScopeIncludeExtensions = scopeIncludeExtensions
}

// WithScopeMaxFileSize adds the scopeMaxFileSize to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithScopeMaxFileSize(scopeMaxFileSize *int64) *VscanOnDemandModifyCollectionParams {
	o.SetScopeMaxFileSize(scopeMaxFileSize)
	return o
}

// SetScopeMaxFileSize adds the scopeMaxFileSize to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetScopeMaxFileSize(scopeMaxFileSize *int64) {
	o.ScopeMaxFileSize = scopeMaxFileSize
}

// WithScopeScanWithoutExtension adds the scopeScanWithoutExtension to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithScopeScanWithoutExtension(scopeScanWithoutExtension *bool) *VscanOnDemandModifyCollectionParams {
	o.SetScopeScanWithoutExtension(scopeScanWithoutExtension)
	return o
}

// SetScopeScanWithoutExtension adds the scopeScanWithoutExtension to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetScopeScanWithoutExtension(scopeScanWithoutExtension *bool) {
	o.ScopeScanWithoutExtension = scopeScanWithoutExtension
}

// WithSerialRecords adds the serialRecords to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithSerialRecords(serialRecords *bool) *VscanOnDemandModifyCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WithSvmName adds the svmName to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithSvmName(svmName *string) *VscanOnDemandModifyCollectionParams {
	o.SetSvmName(svmName)
	return o
}

// SetSvmName adds the svmName to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetSvmName(svmName *string) {
	o.SvmName = svmName
}

// WithSvmUUID adds the svmUUID to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) WithSvmUUID(svmUUID string) *VscanOnDemandModifyCollectionParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the vscan on demand modify collection params
func (o *VscanOnDemandModifyCollectionParams) SetSvmUUID(svmUUID string) {
	o.SvmUUID = svmUUID
}

// WriteToRequest writes these params to a swagger request
func (o *VscanOnDemandModifyCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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
	if err := r.SetBodyParam(o.Info); err != nil {
		return err
	}

	if o.LogPath != nil {

		// query param log_path
		var qrLogPath string

		if o.LogPath != nil {
			qrLogPath = *o.LogPath
		}
		qLogPath := qrLogPath
		if qLogPath != "" {

			if err := r.SetQueryParam("log_path", qLogPath); err != nil {
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

	if o.ScanPaths != nil {

		// query param scan_paths
		var qrScanPaths string

		if o.ScanPaths != nil {
			qrScanPaths = *o.ScanPaths
		}
		qScanPaths := qrScanPaths
		if qScanPaths != "" {

			if err := r.SetQueryParam("scan_paths", qScanPaths); err != nil {
				return err
			}
		}
	}

	if o.ScheduleName != nil {

		// query param schedule.name
		var qrScheduleName string

		if o.ScheduleName != nil {
			qrScheduleName = *o.ScheduleName
		}
		qScheduleName := qrScheduleName
		if qScheduleName != "" {

			if err := r.SetQueryParam("schedule.name", qScheduleName); err != nil {
				return err
			}
		}
	}

	if o.ScheduleUUID != nil {

		// query param schedule.uuid
		var qrScheduleUUID string

		if o.ScheduleUUID != nil {
			qrScheduleUUID = *o.ScheduleUUID
		}
		qScheduleUUID := qrScheduleUUID
		if qScheduleUUID != "" {

			if err := r.SetQueryParam("schedule.uuid", qScheduleUUID); err != nil {
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