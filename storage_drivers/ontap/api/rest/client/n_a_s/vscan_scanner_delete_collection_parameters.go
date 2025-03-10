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

// NewVscanScannerDeleteCollectionParams creates a new VscanScannerDeleteCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewVscanScannerDeleteCollectionParams() *VscanScannerDeleteCollectionParams {
	return &VscanScannerDeleteCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewVscanScannerDeleteCollectionParamsWithTimeout creates a new VscanScannerDeleteCollectionParams object
// with the ability to set a timeout on a request.
func NewVscanScannerDeleteCollectionParamsWithTimeout(timeout time.Duration) *VscanScannerDeleteCollectionParams {
	return &VscanScannerDeleteCollectionParams{
		timeout: timeout,
	}
}

// NewVscanScannerDeleteCollectionParamsWithContext creates a new VscanScannerDeleteCollectionParams object
// with the ability to set a context for a request.
func NewVscanScannerDeleteCollectionParamsWithContext(ctx context.Context) *VscanScannerDeleteCollectionParams {
	return &VscanScannerDeleteCollectionParams{
		Context: ctx,
	}
}

// NewVscanScannerDeleteCollectionParamsWithHTTPClient creates a new VscanScannerDeleteCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewVscanScannerDeleteCollectionParamsWithHTTPClient(client *http.Client) *VscanScannerDeleteCollectionParams {
	return &VscanScannerDeleteCollectionParams{
		HTTPClient: client,
	}
}

/*
VscanScannerDeleteCollectionParams contains all the parameters to send to the API endpoint

	for the vscan scanner delete collection operation.

	Typically these are written to a http.Request.
*/
type VscanScannerDeleteCollectionParams struct {

	/* ClusterName.

	   Filter by cluster.name
	*/
	ClusterName *string

	/* ClusterUUID.

	   Filter by cluster.uuid
	*/
	ClusterUUID *string

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* Info.

	   Info specification
	*/
	Info VscanScannerDeleteCollectionBody

	/* Name.

	   Filter by name
	*/
	Name *string

	/* PrivilegedUsers.

	   Filter by privileged_users
	*/
	PrivilegedUsers *string

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

	/* Role.

	   Filter by role
	*/
	Role *string

	/* SerialRecords.

	   Perform the operation on the records synchronously.
	*/
	SerialRecords *bool

	/* Servers.

	   Filter by servers
	*/
	Servers *string

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

// WithDefaults hydrates default values in the vscan scanner delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *VscanScannerDeleteCollectionParams) WithDefaults() *VscanScannerDeleteCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the vscan scanner delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *VscanScannerDeleteCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := VscanScannerDeleteCollectionParams{
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

// WithTimeout adds the timeout to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) WithTimeout(timeout time.Duration) *VscanScannerDeleteCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) WithContext(ctx context.Context) *VscanScannerDeleteCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) WithHTTPClient(client *http.Client) *VscanScannerDeleteCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithClusterName adds the clusterName to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) WithClusterName(clusterName *string) *VscanScannerDeleteCollectionParams {
	o.SetClusterName(clusterName)
	return o
}

// SetClusterName adds the clusterName to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) SetClusterName(clusterName *string) {
	o.ClusterName = clusterName
}

// WithClusterUUID adds the clusterUUID to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) WithClusterUUID(clusterUUID *string) *VscanScannerDeleteCollectionParams {
	o.SetClusterUUID(clusterUUID)
	return o
}

// SetClusterUUID adds the clusterUuid to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) SetClusterUUID(clusterUUID *string) {
	o.ClusterUUID = clusterUUID
}

// WithContinueOnFailure adds the continueOnFailure to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *VscanScannerDeleteCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithInfo adds the info to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) WithInfo(info VscanScannerDeleteCollectionBody) *VscanScannerDeleteCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) SetInfo(info VscanScannerDeleteCollectionBody) {
	o.Info = info
}

// WithName adds the name to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) WithName(name *string) *VscanScannerDeleteCollectionParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) SetName(name *string) {
	o.Name = name
}

// WithPrivilegedUsers adds the privilegedUsers to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) WithPrivilegedUsers(privilegedUsers *string) *VscanScannerDeleteCollectionParams {
	o.SetPrivilegedUsers(privilegedUsers)
	return o
}

// SetPrivilegedUsers adds the privilegedUsers to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) SetPrivilegedUsers(privilegedUsers *string) {
	o.PrivilegedUsers = privilegedUsers
}

// WithReturnRecords adds the returnRecords to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) WithReturnRecords(returnRecords *bool) *VscanScannerDeleteCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) WithReturnTimeout(returnTimeout *int64) *VscanScannerDeleteCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithRole adds the role to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) WithRole(role *string) *VscanScannerDeleteCollectionParams {
	o.SetRole(role)
	return o
}

// SetRole adds the role to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) SetRole(role *string) {
	o.Role = role
}

// WithSerialRecords adds the serialRecords to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) WithSerialRecords(serialRecords *bool) *VscanScannerDeleteCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WithServers adds the servers to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) WithServers(servers *string) *VscanScannerDeleteCollectionParams {
	o.SetServers(servers)
	return o
}

// SetServers adds the servers to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) SetServers(servers *string) {
	o.Servers = servers
}

// WithSvmName adds the svmName to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) WithSvmName(svmName *string) *VscanScannerDeleteCollectionParams {
	o.SetSvmName(svmName)
	return o
}

// SetSvmName adds the svmName to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) SetSvmName(svmName *string) {
	o.SvmName = svmName
}

// WithSvmUUID adds the svmUUID to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) WithSvmUUID(svmUUID string) *VscanScannerDeleteCollectionParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the vscan scanner delete collection params
func (o *VscanScannerDeleteCollectionParams) SetSvmUUID(svmUUID string) {
	o.SvmUUID = svmUUID
}

// WriteToRequest writes these params to a swagger request
func (o *VscanScannerDeleteCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.ClusterName != nil {

		// query param cluster.name
		var qrClusterName string

		if o.ClusterName != nil {
			qrClusterName = *o.ClusterName
		}
		qClusterName := qrClusterName
		if qClusterName != "" {

			if err := r.SetQueryParam("cluster.name", qClusterName); err != nil {
				return err
			}
		}
	}

	if o.ClusterUUID != nil {

		// query param cluster.uuid
		var qrClusterUUID string

		if o.ClusterUUID != nil {
			qrClusterUUID = *o.ClusterUUID
		}
		qClusterUUID := qrClusterUUID
		if qClusterUUID != "" {

			if err := r.SetQueryParam("cluster.uuid", qClusterUUID); err != nil {
				return err
			}
		}
	}

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

	if o.PrivilegedUsers != nil {

		// query param privileged_users
		var qrPrivilegedUsers string

		if o.PrivilegedUsers != nil {
			qrPrivilegedUsers = *o.PrivilegedUsers
		}
		qPrivilegedUsers := qrPrivilegedUsers
		if qPrivilegedUsers != "" {

			if err := r.SetQueryParam("privileged_users", qPrivilegedUsers); err != nil {
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

	if o.Role != nil {

		// query param role
		var qrRole string

		if o.Role != nil {
			qrRole = *o.Role
		}
		qRole := qrRole
		if qRole != "" {

			if err := r.SetQueryParam("role", qRole); err != nil {
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

	if o.Servers != nil {

		// query param servers
		var qrServers string

		if o.Servers != nil {
			qrServers = *o.Servers
		}
		qServers := qrServers
		if qServers != "" {

			if err := r.SetQueryParam("servers", qServers); err != nil {
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
