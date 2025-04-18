// Code generated by go-swagger; DO NOT EDIT.

package n_v_me

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

// NewNvmeSubsystemCollectionGetParams creates a new NvmeSubsystemCollectionGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewNvmeSubsystemCollectionGetParams() *NvmeSubsystemCollectionGetParams {
	return &NvmeSubsystemCollectionGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewNvmeSubsystemCollectionGetParamsWithTimeout creates a new NvmeSubsystemCollectionGetParams object
// with the ability to set a timeout on a request.
func NewNvmeSubsystemCollectionGetParamsWithTimeout(timeout time.Duration) *NvmeSubsystemCollectionGetParams {
	return &NvmeSubsystemCollectionGetParams{
		timeout: timeout,
	}
}

// NewNvmeSubsystemCollectionGetParamsWithContext creates a new NvmeSubsystemCollectionGetParams object
// with the ability to set a context for a request.
func NewNvmeSubsystemCollectionGetParamsWithContext(ctx context.Context) *NvmeSubsystemCollectionGetParams {
	return &NvmeSubsystemCollectionGetParams{
		Context: ctx,
	}
}

// NewNvmeSubsystemCollectionGetParamsWithHTTPClient creates a new NvmeSubsystemCollectionGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewNvmeSubsystemCollectionGetParamsWithHTTPClient(client *http.Client) *NvmeSubsystemCollectionGetParams {
	return &NvmeSubsystemCollectionGetParams{
		HTTPClient: client,
	}
}

/*
NvmeSubsystemCollectionGetParams contains all the parameters to send to the API endpoint

	for the nvme subsystem collection get operation.

	Typically these are written to a http.Request.
*/
type NvmeSubsystemCollectionGetParams struct {

	/* Comment.

	   Filter by comment
	*/
	Comment *string

	/* DeleteOnUnmap.

	   Filter by delete_on_unmap
	*/
	DeleteOnUnmap *bool

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* HostsDhHmacChapGroupSize.

	   Filter by hosts.dh_hmac_chap.group_size
	*/
	HostsDhHmacChapGroupSize *string

	/* HostsDhHmacChapHashFunction.

	   Filter by hosts.dh_hmac_chap.hash_function
	*/
	HostsDhHmacChapHashFunction *string

	/* HostsDhHmacChapMode.

	   Filter by hosts.dh_hmac_chap.mode
	*/
	HostsDhHmacChapMode *string

	/* HostsNqn.

	   Filter by hosts.nqn
	*/
	HostsNqn *string

	/* HostsPriority.

	   Filter by hosts.priority
	*/
	HostsPriority *string

	/* HostsTLSKeyType.

	   Filter by hosts.tls.key_type
	*/
	HostsTLSKeyType *string

	/* IoQueueDefaultCount.

	   Filter by io_queue.default.count
	*/
	IoQueueDefaultCount *int64

	/* IoQueueDefaultDepth.

	   Filter by io_queue.default.depth
	*/
	IoQueueDefaultDepth *int64

	/* MaxRecords.

	   Limit the number of records returned.
	*/
	MaxRecords *int64

	/* Name.

	   Filter by name
	*/
	Name *string

	/* OrderBy.

	   Order results by specified fields and optional [asc|desc] direction. Default direction is 'asc' for ascending.
	*/
	OrderBy []string

	/* OsType.

	   Filter by os_type
	*/
	OsType *string

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

	/* SerialNumber.

	   Filter by serial_number
	*/
	SerialNumber *string

	/* SubsystemMapsAnagrpid.

	   Filter by subsystem_maps.anagrpid
	*/
	SubsystemMapsAnagrpid *string

	/* SubsystemMapsNamespaceName.

	   Filter by subsystem_maps.namespace.name
	*/
	SubsystemMapsNamespaceName *string

	/* SubsystemMapsNamespaceUUID.

	   Filter by subsystem_maps.namespace.uuid
	*/
	SubsystemMapsNamespaceUUID *string

	/* SubsystemMapsNsid.

	   Filter by subsystem_maps.nsid
	*/
	SubsystemMapsNsid *string

	/* SvmName.

	   Filter by svm.name
	*/
	SvmName *string

	/* SvmUUID.

	   Filter by svm.uuid
	*/
	SvmUUID *string

	/* TargetNqn.

	   Filter by target_nqn
	*/
	TargetNqn *string

	/* UUID.

	   Filter by uuid
	*/
	UUID *string

	/* VendorUuids.

	   Filter by vendor_uuids
	*/
	VendorUuids *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the nvme subsystem collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *NvmeSubsystemCollectionGetParams) WithDefaults() *NvmeSubsystemCollectionGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the nvme subsystem collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *NvmeSubsystemCollectionGetParams) SetDefaults() {
	var (
		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)
	)

	val := NvmeSubsystemCollectionGetParams{
		ReturnRecords: &returnRecordsDefault,
		ReturnTimeout: &returnTimeoutDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithTimeout(timeout time.Duration) *NvmeSubsystemCollectionGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithContext(ctx context.Context) *NvmeSubsystemCollectionGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithHTTPClient(client *http.Client) *NvmeSubsystemCollectionGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithComment adds the comment to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithComment(comment *string) *NvmeSubsystemCollectionGetParams {
	o.SetComment(comment)
	return o
}

// SetComment adds the comment to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetComment(comment *string) {
	o.Comment = comment
}

// WithDeleteOnUnmap adds the deleteOnUnmap to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithDeleteOnUnmap(deleteOnUnmap *bool) *NvmeSubsystemCollectionGetParams {
	o.SetDeleteOnUnmap(deleteOnUnmap)
	return o
}

// SetDeleteOnUnmap adds the deleteOnUnmap to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetDeleteOnUnmap(deleteOnUnmap *bool) {
	o.DeleteOnUnmap = deleteOnUnmap
}

// WithFields adds the fields to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithFields(fields []string) *NvmeSubsystemCollectionGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithHostsDhHmacChapGroupSize adds the hostsDhHmacChapGroupSize to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithHostsDhHmacChapGroupSize(hostsDhHmacChapGroupSize *string) *NvmeSubsystemCollectionGetParams {
	o.SetHostsDhHmacChapGroupSize(hostsDhHmacChapGroupSize)
	return o
}

// SetHostsDhHmacChapGroupSize adds the hostsDhHmacChapGroupSize to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetHostsDhHmacChapGroupSize(hostsDhHmacChapGroupSize *string) {
	o.HostsDhHmacChapGroupSize = hostsDhHmacChapGroupSize
}

// WithHostsDhHmacChapHashFunction adds the hostsDhHmacChapHashFunction to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithHostsDhHmacChapHashFunction(hostsDhHmacChapHashFunction *string) *NvmeSubsystemCollectionGetParams {
	o.SetHostsDhHmacChapHashFunction(hostsDhHmacChapHashFunction)
	return o
}

// SetHostsDhHmacChapHashFunction adds the hostsDhHmacChapHashFunction to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetHostsDhHmacChapHashFunction(hostsDhHmacChapHashFunction *string) {
	o.HostsDhHmacChapHashFunction = hostsDhHmacChapHashFunction
}

// WithHostsDhHmacChapMode adds the hostsDhHmacChapMode to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithHostsDhHmacChapMode(hostsDhHmacChapMode *string) *NvmeSubsystemCollectionGetParams {
	o.SetHostsDhHmacChapMode(hostsDhHmacChapMode)
	return o
}

// SetHostsDhHmacChapMode adds the hostsDhHmacChapMode to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetHostsDhHmacChapMode(hostsDhHmacChapMode *string) {
	o.HostsDhHmacChapMode = hostsDhHmacChapMode
}

// WithHostsNqn adds the hostsNqn to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithHostsNqn(hostsNqn *string) *NvmeSubsystemCollectionGetParams {
	o.SetHostsNqn(hostsNqn)
	return o
}

// SetHostsNqn adds the hostsNqn to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetHostsNqn(hostsNqn *string) {
	o.HostsNqn = hostsNqn
}

// WithHostsPriority adds the hostsPriority to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithHostsPriority(hostsPriority *string) *NvmeSubsystemCollectionGetParams {
	o.SetHostsPriority(hostsPriority)
	return o
}

// SetHostsPriority adds the hostsPriority to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetHostsPriority(hostsPriority *string) {
	o.HostsPriority = hostsPriority
}

// WithHostsTLSKeyType adds the hostsTLSKeyType to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithHostsTLSKeyType(hostsTLSKeyType *string) *NvmeSubsystemCollectionGetParams {
	o.SetHostsTLSKeyType(hostsTLSKeyType)
	return o
}

// SetHostsTLSKeyType adds the hostsTlsKeyType to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetHostsTLSKeyType(hostsTLSKeyType *string) {
	o.HostsTLSKeyType = hostsTLSKeyType
}

// WithIoQueueDefaultCount adds the ioQueueDefaultCount to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithIoQueueDefaultCount(ioQueueDefaultCount *int64) *NvmeSubsystemCollectionGetParams {
	o.SetIoQueueDefaultCount(ioQueueDefaultCount)
	return o
}

// SetIoQueueDefaultCount adds the ioQueueDefaultCount to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetIoQueueDefaultCount(ioQueueDefaultCount *int64) {
	o.IoQueueDefaultCount = ioQueueDefaultCount
}

// WithIoQueueDefaultDepth adds the ioQueueDefaultDepth to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithIoQueueDefaultDepth(ioQueueDefaultDepth *int64) *NvmeSubsystemCollectionGetParams {
	o.SetIoQueueDefaultDepth(ioQueueDefaultDepth)
	return o
}

// SetIoQueueDefaultDepth adds the ioQueueDefaultDepth to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetIoQueueDefaultDepth(ioQueueDefaultDepth *int64) {
	o.IoQueueDefaultDepth = ioQueueDefaultDepth
}

// WithMaxRecords adds the maxRecords to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithMaxRecords(maxRecords *int64) *NvmeSubsystemCollectionGetParams {
	o.SetMaxRecords(maxRecords)
	return o
}

// SetMaxRecords adds the maxRecords to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetMaxRecords(maxRecords *int64) {
	o.MaxRecords = maxRecords
}

// WithName adds the name to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithName(name *string) *NvmeSubsystemCollectionGetParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetName(name *string) {
	o.Name = name
}

// WithOrderBy adds the orderBy to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithOrderBy(orderBy []string) *NvmeSubsystemCollectionGetParams {
	o.SetOrderBy(orderBy)
	return o
}

// SetOrderBy adds the orderBy to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetOrderBy(orderBy []string) {
	o.OrderBy = orderBy
}

// WithOsType adds the osType to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithOsType(osType *string) *NvmeSubsystemCollectionGetParams {
	o.SetOsType(osType)
	return o
}

// SetOsType adds the osType to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetOsType(osType *string) {
	o.OsType = osType
}

// WithReturnRecords adds the returnRecords to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithReturnRecords(returnRecords *bool) *NvmeSubsystemCollectionGetParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithReturnTimeout(returnTimeout *int64) *NvmeSubsystemCollectionGetParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithSerialNumber adds the serialNumber to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithSerialNumber(serialNumber *string) *NvmeSubsystemCollectionGetParams {
	o.SetSerialNumber(serialNumber)
	return o
}

// SetSerialNumber adds the serialNumber to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetSerialNumber(serialNumber *string) {
	o.SerialNumber = serialNumber
}

// WithSubsystemMapsAnagrpid adds the subsystemMapsAnagrpid to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithSubsystemMapsAnagrpid(subsystemMapsAnagrpid *string) *NvmeSubsystemCollectionGetParams {
	o.SetSubsystemMapsAnagrpid(subsystemMapsAnagrpid)
	return o
}

// SetSubsystemMapsAnagrpid adds the subsystemMapsAnagrpid to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetSubsystemMapsAnagrpid(subsystemMapsAnagrpid *string) {
	o.SubsystemMapsAnagrpid = subsystemMapsAnagrpid
}

// WithSubsystemMapsNamespaceName adds the subsystemMapsNamespaceName to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithSubsystemMapsNamespaceName(subsystemMapsNamespaceName *string) *NvmeSubsystemCollectionGetParams {
	o.SetSubsystemMapsNamespaceName(subsystemMapsNamespaceName)
	return o
}

// SetSubsystemMapsNamespaceName adds the subsystemMapsNamespaceName to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetSubsystemMapsNamespaceName(subsystemMapsNamespaceName *string) {
	o.SubsystemMapsNamespaceName = subsystemMapsNamespaceName
}

// WithSubsystemMapsNamespaceUUID adds the subsystemMapsNamespaceUUID to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithSubsystemMapsNamespaceUUID(subsystemMapsNamespaceUUID *string) *NvmeSubsystemCollectionGetParams {
	o.SetSubsystemMapsNamespaceUUID(subsystemMapsNamespaceUUID)
	return o
}

// SetSubsystemMapsNamespaceUUID adds the subsystemMapsNamespaceUuid to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetSubsystemMapsNamespaceUUID(subsystemMapsNamespaceUUID *string) {
	o.SubsystemMapsNamespaceUUID = subsystemMapsNamespaceUUID
}

// WithSubsystemMapsNsid adds the subsystemMapsNsid to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithSubsystemMapsNsid(subsystemMapsNsid *string) *NvmeSubsystemCollectionGetParams {
	o.SetSubsystemMapsNsid(subsystemMapsNsid)
	return o
}

// SetSubsystemMapsNsid adds the subsystemMapsNsid to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetSubsystemMapsNsid(subsystemMapsNsid *string) {
	o.SubsystemMapsNsid = subsystemMapsNsid
}

// WithSvmName adds the svmName to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithSvmName(svmName *string) *NvmeSubsystemCollectionGetParams {
	o.SetSvmName(svmName)
	return o
}

// SetSvmName adds the svmName to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetSvmName(svmName *string) {
	o.SvmName = svmName
}

// WithSvmUUID adds the svmUUID to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithSvmUUID(svmUUID *string) *NvmeSubsystemCollectionGetParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetSvmUUID(svmUUID *string) {
	o.SvmUUID = svmUUID
}

// WithTargetNqn adds the targetNqn to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithTargetNqn(targetNqn *string) *NvmeSubsystemCollectionGetParams {
	o.SetTargetNqn(targetNqn)
	return o
}

// SetTargetNqn adds the targetNqn to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetTargetNqn(targetNqn *string) {
	o.TargetNqn = targetNqn
}

// WithUUID adds the uuid to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithUUID(uuid *string) *NvmeSubsystemCollectionGetParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetUUID(uuid *string) {
	o.UUID = uuid
}

// WithVendorUuids adds the vendorUuids to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) WithVendorUuids(vendorUuids *string) *NvmeSubsystemCollectionGetParams {
	o.SetVendorUuids(vendorUuids)
	return o
}

// SetVendorUuids adds the vendorUuids to the nvme subsystem collection get params
func (o *NvmeSubsystemCollectionGetParams) SetVendorUuids(vendorUuids *string) {
	o.VendorUuids = vendorUuids
}

// WriteToRequest writes these params to a swagger request
func (o *NvmeSubsystemCollectionGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.Comment != nil {

		// query param comment
		var qrComment string

		if o.Comment != nil {
			qrComment = *o.Comment
		}
		qComment := qrComment
		if qComment != "" {

			if err := r.SetQueryParam("comment", qComment); err != nil {
				return err
			}
		}
	}

	if o.DeleteOnUnmap != nil {

		// query param delete_on_unmap
		var qrDeleteOnUnmap bool

		if o.DeleteOnUnmap != nil {
			qrDeleteOnUnmap = *o.DeleteOnUnmap
		}
		qDeleteOnUnmap := swag.FormatBool(qrDeleteOnUnmap)
		if qDeleteOnUnmap != "" {

			if err := r.SetQueryParam("delete_on_unmap", qDeleteOnUnmap); err != nil {
				return err
			}
		}
	}

	if o.Fields != nil {

		// binding items for fields
		joinedFields := o.bindParamFields(reg)

		// query array param fields
		if err := r.SetQueryParam("fields", joinedFields...); err != nil {
			return err
		}
	}

	if o.HostsDhHmacChapGroupSize != nil {

		// query param hosts.dh_hmac_chap.group_size
		var qrHostsDhHmacChapGroupSize string

		if o.HostsDhHmacChapGroupSize != nil {
			qrHostsDhHmacChapGroupSize = *o.HostsDhHmacChapGroupSize
		}
		qHostsDhHmacChapGroupSize := qrHostsDhHmacChapGroupSize
		if qHostsDhHmacChapGroupSize != "" {

			if err := r.SetQueryParam("hosts.dh_hmac_chap.group_size", qHostsDhHmacChapGroupSize); err != nil {
				return err
			}
		}
	}

	if o.HostsDhHmacChapHashFunction != nil {

		// query param hosts.dh_hmac_chap.hash_function
		var qrHostsDhHmacChapHashFunction string

		if o.HostsDhHmacChapHashFunction != nil {
			qrHostsDhHmacChapHashFunction = *o.HostsDhHmacChapHashFunction
		}
		qHostsDhHmacChapHashFunction := qrHostsDhHmacChapHashFunction
		if qHostsDhHmacChapHashFunction != "" {

			if err := r.SetQueryParam("hosts.dh_hmac_chap.hash_function", qHostsDhHmacChapHashFunction); err != nil {
				return err
			}
		}
	}

	if o.HostsDhHmacChapMode != nil {

		// query param hosts.dh_hmac_chap.mode
		var qrHostsDhHmacChapMode string

		if o.HostsDhHmacChapMode != nil {
			qrHostsDhHmacChapMode = *o.HostsDhHmacChapMode
		}
		qHostsDhHmacChapMode := qrHostsDhHmacChapMode
		if qHostsDhHmacChapMode != "" {

			if err := r.SetQueryParam("hosts.dh_hmac_chap.mode", qHostsDhHmacChapMode); err != nil {
				return err
			}
		}
	}

	if o.HostsNqn != nil {

		// query param hosts.nqn
		var qrHostsNqn string

		if o.HostsNqn != nil {
			qrHostsNqn = *o.HostsNqn
		}
		qHostsNqn := qrHostsNqn
		if qHostsNqn != "" {

			if err := r.SetQueryParam("hosts.nqn", qHostsNqn); err != nil {
				return err
			}
		}
	}

	if o.HostsPriority != nil {

		// query param hosts.priority
		var qrHostsPriority string

		if o.HostsPriority != nil {
			qrHostsPriority = *o.HostsPriority
		}
		qHostsPriority := qrHostsPriority
		if qHostsPriority != "" {

			if err := r.SetQueryParam("hosts.priority", qHostsPriority); err != nil {
				return err
			}
		}
	}

	if o.HostsTLSKeyType != nil {

		// query param hosts.tls.key_type
		var qrHostsTLSKeyType string

		if o.HostsTLSKeyType != nil {
			qrHostsTLSKeyType = *o.HostsTLSKeyType
		}
		qHostsTLSKeyType := qrHostsTLSKeyType
		if qHostsTLSKeyType != "" {

			if err := r.SetQueryParam("hosts.tls.key_type", qHostsTLSKeyType); err != nil {
				return err
			}
		}
	}

	if o.IoQueueDefaultCount != nil {

		// query param io_queue.default.count
		var qrIoQueueDefaultCount int64

		if o.IoQueueDefaultCount != nil {
			qrIoQueueDefaultCount = *o.IoQueueDefaultCount
		}
		qIoQueueDefaultCount := swag.FormatInt64(qrIoQueueDefaultCount)
		if qIoQueueDefaultCount != "" {

			if err := r.SetQueryParam("io_queue.default.count", qIoQueueDefaultCount); err != nil {
				return err
			}
		}
	}

	if o.IoQueueDefaultDepth != nil {

		// query param io_queue.default.depth
		var qrIoQueueDefaultDepth int64

		if o.IoQueueDefaultDepth != nil {
			qrIoQueueDefaultDepth = *o.IoQueueDefaultDepth
		}
		qIoQueueDefaultDepth := swag.FormatInt64(qrIoQueueDefaultDepth)
		if qIoQueueDefaultDepth != "" {

			if err := r.SetQueryParam("io_queue.default.depth", qIoQueueDefaultDepth); err != nil {
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

	if o.OrderBy != nil {

		// binding items for order_by
		joinedOrderBy := o.bindParamOrderBy(reg)

		// query array param order_by
		if err := r.SetQueryParam("order_by", joinedOrderBy...); err != nil {
			return err
		}
	}

	if o.OsType != nil {

		// query param os_type
		var qrOsType string

		if o.OsType != nil {
			qrOsType = *o.OsType
		}
		qOsType := qrOsType
		if qOsType != "" {

			if err := r.SetQueryParam("os_type", qOsType); err != nil {
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

	if o.SerialNumber != nil {

		// query param serial_number
		var qrSerialNumber string

		if o.SerialNumber != nil {
			qrSerialNumber = *o.SerialNumber
		}
		qSerialNumber := qrSerialNumber
		if qSerialNumber != "" {

			if err := r.SetQueryParam("serial_number", qSerialNumber); err != nil {
				return err
			}
		}
	}

	if o.SubsystemMapsAnagrpid != nil {

		// query param subsystem_maps.anagrpid
		var qrSubsystemMapsAnagrpid string

		if o.SubsystemMapsAnagrpid != nil {
			qrSubsystemMapsAnagrpid = *o.SubsystemMapsAnagrpid
		}
		qSubsystemMapsAnagrpid := qrSubsystemMapsAnagrpid
		if qSubsystemMapsAnagrpid != "" {

			if err := r.SetQueryParam("subsystem_maps.anagrpid", qSubsystemMapsAnagrpid); err != nil {
				return err
			}
		}
	}

	if o.SubsystemMapsNamespaceName != nil {

		// query param subsystem_maps.namespace.name
		var qrSubsystemMapsNamespaceName string

		if o.SubsystemMapsNamespaceName != nil {
			qrSubsystemMapsNamespaceName = *o.SubsystemMapsNamespaceName
		}
		qSubsystemMapsNamespaceName := qrSubsystemMapsNamespaceName
		if qSubsystemMapsNamespaceName != "" {

			if err := r.SetQueryParam("subsystem_maps.namespace.name", qSubsystemMapsNamespaceName); err != nil {
				return err
			}
		}
	}

	if o.SubsystemMapsNamespaceUUID != nil {

		// query param subsystem_maps.namespace.uuid
		var qrSubsystemMapsNamespaceUUID string

		if o.SubsystemMapsNamespaceUUID != nil {
			qrSubsystemMapsNamespaceUUID = *o.SubsystemMapsNamespaceUUID
		}
		qSubsystemMapsNamespaceUUID := qrSubsystemMapsNamespaceUUID
		if qSubsystemMapsNamespaceUUID != "" {

			if err := r.SetQueryParam("subsystem_maps.namespace.uuid", qSubsystemMapsNamespaceUUID); err != nil {
				return err
			}
		}
	}

	if o.SubsystemMapsNsid != nil {

		// query param subsystem_maps.nsid
		var qrSubsystemMapsNsid string

		if o.SubsystemMapsNsid != nil {
			qrSubsystemMapsNsid = *o.SubsystemMapsNsid
		}
		qSubsystemMapsNsid := qrSubsystemMapsNsid
		if qSubsystemMapsNsid != "" {

			if err := r.SetQueryParam("subsystem_maps.nsid", qSubsystemMapsNsid); err != nil {
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

	if o.TargetNqn != nil {

		// query param target_nqn
		var qrTargetNqn string

		if o.TargetNqn != nil {
			qrTargetNqn = *o.TargetNqn
		}
		qTargetNqn := qrTargetNqn
		if qTargetNqn != "" {

			if err := r.SetQueryParam("target_nqn", qTargetNqn); err != nil {
				return err
			}
		}
	}

	if o.UUID != nil {

		// query param uuid
		var qrUUID string

		if o.UUID != nil {
			qrUUID = *o.UUID
		}
		qUUID := qrUUID
		if qUUID != "" {

			if err := r.SetQueryParam("uuid", qUUID); err != nil {
				return err
			}
		}
	}

	if o.VendorUuids != nil {

		// query param vendor_uuids
		var qrVendorUuids string

		if o.VendorUuids != nil {
			qrVendorUuids = *o.VendorUuids
		}
		qVendorUuids := qrVendorUuids
		if qVendorUuids != "" {

			if err := r.SetQueryParam("vendor_uuids", qVendorUuids); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamNvmeSubsystemCollectionGet binds the parameter fields
func (o *NvmeSubsystemCollectionGetParams) bindParamFields(formats strfmt.Registry) []string {
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

// bindParamNvmeSubsystemCollectionGet binds the parameter order_by
func (o *NvmeSubsystemCollectionGetParams) bindParamOrderBy(formats strfmt.Registry) []string {
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
