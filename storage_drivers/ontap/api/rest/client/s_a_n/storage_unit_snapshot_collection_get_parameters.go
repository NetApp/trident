// Code generated by go-swagger; DO NOT EDIT.

package s_a_n

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

// NewStorageUnitSnapshotCollectionGetParams creates a new StorageUnitSnapshotCollectionGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewStorageUnitSnapshotCollectionGetParams() *StorageUnitSnapshotCollectionGetParams {
	return &StorageUnitSnapshotCollectionGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewStorageUnitSnapshotCollectionGetParamsWithTimeout creates a new StorageUnitSnapshotCollectionGetParams object
// with the ability to set a timeout on a request.
func NewStorageUnitSnapshotCollectionGetParamsWithTimeout(timeout time.Duration) *StorageUnitSnapshotCollectionGetParams {
	return &StorageUnitSnapshotCollectionGetParams{
		timeout: timeout,
	}
}

// NewStorageUnitSnapshotCollectionGetParamsWithContext creates a new StorageUnitSnapshotCollectionGetParams object
// with the ability to set a context for a request.
func NewStorageUnitSnapshotCollectionGetParamsWithContext(ctx context.Context) *StorageUnitSnapshotCollectionGetParams {
	return &StorageUnitSnapshotCollectionGetParams{
		Context: ctx,
	}
}

// NewStorageUnitSnapshotCollectionGetParamsWithHTTPClient creates a new StorageUnitSnapshotCollectionGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewStorageUnitSnapshotCollectionGetParamsWithHTTPClient(client *http.Client) *StorageUnitSnapshotCollectionGetParams {
	return &StorageUnitSnapshotCollectionGetParams{
		HTTPClient: client,
	}
}

/*
StorageUnitSnapshotCollectionGetParams contains all the parameters to send to the API endpoint

	for the storage unit snapshot collection get operation.

	Typically these are written to a http.Request.
*/
type StorageUnitSnapshotCollectionGetParams struct {

	/* Comment.

	   Filter by comment
	*/
	Comment *string

	/* CreateTime.

	   Filter by create_time
	*/
	CreateTime *string

	/* DeltaSizeConsumed.

	   Filter by delta.size_consumed
	*/
	DeltaSizeConsumed *int64

	/* DeltaTimeElapsed.

	   Filter by delta.time_elapsed
	*/
	DeltaTimeElapsed *string

	/* ExpiryTime.

	   Filter by expiry_time
	*/
	ExpiryTime *string

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* LogicalSize.

	   Filter by logical_size
	*/
	LogicalSize *int64

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

	/* Owners.

	   Filter by owners
	*/
	Owners *string

	/* ReclaimableSpace.

	   Filter by reclaimable_space
	*/
	ReclaimableSpace *int64

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

	/* Size.

	   Filter by size
	*/
	Size *int64

	/* SnaplockExpired.

	   Filter by snaplock.expired
	*/
	SnaplockExpired *bool

	/* SnaplockExpiryTime.

	   Filter by snaplock.expiry_time
	*/
	SnaplockExpiryTime *string

	/* SnaplockTimeUntilExpiry.

	   Filter by snaplock.time_until_expiry
	*/
	SnaplockTimeUntilExpiry *string

	/* SnapmirrorLabel.

	   Filter by snapmirror_label
	*/
	SnapmirrorLabel *string

	/* State.

	   Filter by state
	*/
	State *string

	/* StorageUnitName.

	   Filter by storage_unit.name
	*/
	StorageUnitName *string

	/* StorageUnitUUID.

	   Storage Unit UUID
	*/
	StorageUnitUUID string

	/* SvmName.

	   Filter by svm.name
	*/
	SvmName *string

	/* SvmUUID.

	   Filter by svm.uuid
	*/
	SvmUUID *string

	/* UUID.

	   Filter by uuid
	*/
	UUID *string

	/* VersionUUID.

	   Filter by version_uuid
	*/
	VersionUUID *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the storage unit snapshot collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *StorageUnitSnapshotCollectionGetParams) WithDefaults() *StorageUnitSnapshotCollectionGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the storage unit snapshot collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *StorageUnitSnapshotCollectionGetParams) SetDefaults() {
	var (
		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)
	)

	val := StorageUnitSnapshotCollectionGetParams{
		ReturnRecords: &returnRecordsDefault,
		ReturnTimeout: &returnTimeoutDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithTimeout(timeout time.Duration) *StorageUnitSnapshotCollectionGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithContext(ctx context.Context) *StorageUnitSnapshotCollectionGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithHTTPClient(client *http.Client) *StorageUnitSnapshotCollectionGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithComment adds the comment to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithComment(comment *string) *StorageUnitSnapshotCollectionGetParams {
	o.SetComment(comment)
	return o
}

// SetComment adds the comment to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetComment(comment *string) {
	o.Comment = comment
}

// WithCreateTime adds the createTime to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithCreateTime(createTime *string) *StorageUnitSnapshotCollectionGetParams {
	o.SetCreateTime(createTime)
	return o
}

// SetCreateTime adds the createTime to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetCreateTime(createTime *string) {
	o.CreateTime = createTime
}

// WithDeltaSizeConsumed adds the deltaSizeConsumed to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithDeltaSizeConsumed(deltaSizeConsumed *int64) *StorageUnitSnapshotCollectionGetParams {
	o.SetDeltaSizeConsumed(deltaSizeConsumed)
	return o
}

// SetDeltaSizeConsumed adds the deltaSizeConsumed to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetDeltaSizeConsumed(deltaSizeConsumed *int64) {
	o.DeltaSizeConsumed = deltaSizeConsumed
}

// WithDeltaTimeElapsed adds the deltaTimeElapsed to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithDeltaTimeElapsed(deltaTimeElapsed *string) *StorageUnitSnapshotCollectionGetParams {
	o.SetDeltaTimeElapsed(deltaTimeElapsed)
	return o
}

// SetDeltaTimeElapsed adds the deltaTimeElapsed to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetDeltaTimeElapsed(deltaTimeElapsed *string) {
	o.DeltaTimeElapsed = deltaTimeElapsed
}

// WithExpiryTime adds the expiryTime to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithExpiryTime(expiryTime *string) *StorageUnitSnapshotCollectionGetParams {
	o.SetExpiryTime(expiryTime)
	return o
}

// SetExpiryTime adds the expiryTime to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetExpiryTime(expiryTime *string) {
	o.ExpiryTime = expiryTime
}

// WithFields adds the fields to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithFields(fields []string) *StorageUnitSnapshotCollectionGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithLogicalSize adds the logicalSize to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithLogicalSize(logicalSize *int64) *StorageUnitSnapshotCollectionGetParams {
	o.SetLogicalSize(logicalSize)
	return o
}

// SetLogicalSize adds the logicalSize to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetLogicalSize(logicalSize *int64) {
	o.LogicalSize = logicalSize
}

// WithMaxRecords adds the maxRecords to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithMaxRecords(maxRecords *int64) *StorageUnitSnapshotCollectionGetParams {
	o.SetMaxRecords(maxRecords)
	return o
}

// SetMaxRecords adds the maxRecords to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetMaxRecords(maxRecords *int64) {
	o.MaxRecords = maxRecords
}

// WithName adds the name to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithName(name *string) *StorageUnitSnapshotCollectionGetParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetName(name *string) {
	o.Name = name
}

// WithOrderBy adds the orderBy to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithOrderBy(orderBy []string) *StorageUnitSnapshotCollectionGetParams {
	o.SetOrderBy(orderBy)
	return o
}

// SetOrderBy adds the orderBy to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetOrderBy(orderBy []string) {
	o.OrderBy = orderBy
}

// WithOwners adds the owners to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithOwners(owners *string) *StorageUnitSnapshotCollectionGetParams {
	o.SetOwners(owners)
	return o
}

// SetOwners adds the owners to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetOwners(owners *string) {
	o.Owners = owners
}

// WithReclaimableSpace adds the reclaimableSpace to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithReclaimableSpace(reclaimableSpace *int64) *StorageUnitSnapshotCollectionGetParams {
	o.SetReclaimableSpace(reclaimableSpace)
	return o
}

// SetReclaimableSpace adds the reclaimableSpace to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetReclaimableSpace(reclaimableSpace *int64) {
	o.ReclaimableSpace = reclaimableSpace
}

// WithReturnRecords adds the returnRecords to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithReturnRecords(returnRecords *bool) *StorageUnitSnapshotCollectionGetParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithReturnTimeout(returnTimeout *int64) *StorageUnitSnapshotCollectionGetParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithSize adds the size to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithSize(size *int64) *StorageUnitSnapshotCollectionGetParams {
	o.SetSize(size)
	return o
}

// SetSize adds the size to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetSize(size *int64) {
	o.Size = size
}

// WithSnaplockExpired adds the snaplockExpired to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithSnaplockExpired(snaplockExpired *bool) *StorageUnitSnapshotCollectionGetParams {
	o.SetSnaplockExpired(snaplockExpired)
	return o
}

// SetSnaplockExpired adds the snaplockExpired to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetSnaplockExpired(snaplockExpired *bool) {
	o.SnaplockExpired = snaplockExpired
}

// WithSnaplockExpiryTime adds the snaplockExpiryTime to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithSnaplockExpiryTime(snaplockExpiryTime *string) *StorageUnitSnapshotCollectionGetParams {
	o.SetSnaplockExpiryTime(snaplockExpiryTime)
	return o
}

// SetSnaplockExpiryTime adds the snaplockExpiryTime to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetSnaplockExpiryTime(snaplockExpiryTime *string) {
	o.SnaplockExpiryTime = snaplockExpiryTime
}

// WithSnaplockTimeUntilExpiry adds the snaplockTimeUntilExpiry to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithSnaplockTimeUntilExpiry(snaplockTimeUntilExpiry *string) *StorageUnitSnapshotCollectionGetParams {
	o.SetSnaplockTimeUntilExpiry(snaplockTimeUntilExpiry)
	return o
}

// SetSnaplockTimeUntilExpiry adds the snaplockTimeUntilExpiry to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetSnaplockTimeUntilExpiry(snaplockTimeUntilExpiry *string) {
	o.SnaplockTimeUntilExpiry = snaplockTimeUntilExpiry
}

// WithSnapmirrorLabel adds the snapmirrorLabel to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithSnapmirrorLabel(snapmirrorLabel *string) *StorageUnitSnapshotCollectionGetParams {
	o.SetSnapmirrorLabel(snapmirrorLabel)
	return o
}

// SetSnapmirrorLabel adds the snapmirrorLabel to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetSnapmirrorLabel(snapmirrorLabel *string) {
	o.SnapmirrorLabel = snapmirrorLabel
}

// WithState adds the state to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithState(state *string) *StorageUnitSnapshotCollectionGetParams {
	o.SetState(state)
	return o
}

// SetState adds the state to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetState(state *string) {
	o.State = state
}

// WithStorageUnitName adds the storageUnitName to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithStorageUnitName(storageUnitName *string) *StorageUnitSnapshotCollectionGetParams {
	o.SetStorageUnitName(storageUnitName)
	return o
}

// SetStorageUnitName adds the storageUnitName to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetStorageUnitName(storageUnitName *string) {
	o.StorageUnitName = storageUnitName
}

// WithStorageUnitUUID adds the storageUnitUUID to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithStorageUnitUUID(storageUnitUUID string) *StorageUnitSnapshotCollectionGetParams {
	o.SetStorageUnitUUID(storageUnitUUID)
	return o
}

// SetStorageUnitUUID adds the storageUnitUuid to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetStorageUnitUUID(storageUnitUUID string) {
	o.StorageUnitUUID = storageUnitUUID
}

// WithSvmName adds the svmName to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithSvmName(svmName *string) *StorageUnitSnapshotCollectionGetParams {
	o.SetSvmName(svmName)
	return o
}

// SetSvmName adds the svmName to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetSvmName(svmName *string) {
	o.SvmName = svmName
}

// WithSvmUUID adds the svmUUID to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithSvmUUID(svmUUID *string) *StorageUnitSnapshotCollectionGetParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetSvmUUID(svmUUID *string) {
	o.SvmUUID = svmUUID
}

// WithUUID adds the uuid to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithUUID(uuid *string) *StorageUnitSnapshotCollectionGetParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetUUID(uuid *string) {
	o.UUID = uuid
}

// WithVersionUUID adds the versionUUID to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) WithVersionUUID(versionUUID *string) *StorageUnitSnapshotCollectionGetParams {
	o.SetVersionUUID(versionUUID)
	return o
}

// SetVersionUUID adds the versionUuid to the storage unit snapshot collection get params
func (o *StorageUnitSnapshotCollectionGetParams) SetVersionUUID(versionUUID *string) {
	o.VersionUUID = versionUUID
}

// WriteToRequest writes these params to a swagger request
func (o *StorageUnitSnapshotCollectionGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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

	if o.CreateTime != nil {

		// query param create_time
		var qrCreateTime string

		if o.CreateTime != nil {
			qrCreateTime = *o.CreateTime
		}
		qCreateTime := qrCreateTime
		if qCreateTime != "" {

			if err := r.SetQueryParam("create_time", qCreateTime); err != nil {
				return err
			}
		}
	}

	if o.DeltaSizeConsumed != nil {

		// query param delta.size_consumed
		var qrDeltaSizeConsumed int64

		if o.DeltaSizeConsumed != nil {
			qrDeltaSizeConsumed = *o.DeltaSizeConsumed
		}
		qDeltaSizeConsumed := swag.FormatInt64(qrDeltaSizeConsumed)
		if qDeltaSizeConsumed != "" {

			if err := r.SetQueryParam("delta.size_consumed", qDeltaSizeConsumed); err != nil {
				return err
			}
		}
	}

	if o.DeltaTimeElapsed != nil {

		// query param delta.time_elapsed
		var qrDeltaTimeElapsed string

		if o.DeltaTimeElapsed != nil {
			qrDeltaTimeElapsed = *o.DeltaTimeElapsed
		}
		qDeltaTimeElapsed := qrDeltaTimeElapsed
		if qDeltaTimeElapsed != "" {

			if err := r.SetQueryParam("delta.time_elapsed", qDeltaTimeElapsed); err != nil {
				return err
			}
		}
	}

	if o.ExpiryTime != nil {

		// query param expiry_time
		var qrExpiryTime string

		if o.ExpiryTime != nil {
			qrExpiryTime = *o.ExpiryTime
		}
		qExpiryTime := qrExpiryTime
		if qExpiryTime != "" {

			if err := r.SetQueryParam("expiry_time", qExpiryTime); err != nil {
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

	if o.LogicalSize != nil {

		// query param logical_size
		var qrLogicalSize int64

		if o.LogicalSize != nil {
			qrLogicalSize = *o.LogicalSize
		}
		qLogicalSize := swag.FormatInt64(qrLogicalSize)
		if qLogicalSize != "" {

			if err := r.SetQueryParam("logical_size", qLogicalSize); err != nil {
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

	if o.Owners != nil {

		// query param owners
		var qrOwners string

		if o.Owners != nil {
			qrOwners = *o.Owners
		}
		qOwners := qrOwners
		if qOwners != "" {

			if err := r.SetQueryParam("owners", qOwners); err != nil {
				return err
			}
		}
	}

	if o.ReclaimableSpace != nil {

		// query param reclaimable_space
		var qrReclaimableSpace int64

		if o.ReclaimableSpace != nil {
			qrReclaimableSpace = *o.ReclaimableSpace
		}
		qReclaimableSpace := swag.FormatInt64(qrReclaimableSpace)
		if qReclaimableSpace != "" {

			if err := r.SetQueryParam("reclaimable_space", qReclaimableSpace); err != nil {
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

	if o.Size != nil {

		// query param size
		var qrSize int64

		if o.Size != nil {
			qrSize = *o.Size
		}
		qSize := swag.FormatInt64(qrSize)
		if qSize != "" {

			if err := r.SetQueryParam("size", qSize); err != nil {
				return err
			}
		}
	}

	if o.SnaplockExpired != nil {

		// query param snaplock.expired
		var qrSnaplockExpired bool

		if o.SnaplockExpired != nil {
			qrSnaplockExpired = *o.SnaplockExpired
		}
		qSnaplockExpired := swag.FormatBool(qrSnaplockExpired)
		if qSnaplockExpired != "" {

			if err := r.SetQueryParam("snaplock.expired", qSnaplockExpired); err != nil {
				return err
			}
		}
	}

	if o.SnaplockExpiryTime != nil {

		// query param snaplock.expiry_time
		var qrSnaplockExpiryTime string

		if o.SnaplockExpiryTime != nil {
			qrSnaplockExpiryTime = *o.SnaplockExpiryTime
		}
		qSnaplockExpiryTime := qrSnaplockExpiryTime
		if qSnaplockExpiryTime != "" {

			if err := r.SetQueryParam("snaplock.expiry_time", qSnaplockExpiryTime); err != nil {
				return err
			}
		}
	}

	if o.SnaplockTimeUntilExpiry != nil {

		// query param snaplock.time_until_expiry
		var qrSnaplockTimeUntilExpiry string

		if o.SnaplockTimeUntilExpiry != nil {
			qrSnaplockTimeUntilExpiry = *o.SnaplockTimeUntilExpiry
		}
		qSnaplockTimeUntilExpiry := qrSnaplockTimeUntilExpiry
		if qSnaplockTimeUntilExpiry != "" {

			if err := r.SetQueryParam("snaplock.time_until_expiry", qSnaplockTimeUntilExpiry); err != nil {
				return err
			}
		}
	}

	if o.SnapmirrorLabel != nil {

		// query param snapmirror_label
		var qrSnapmirrorLabel string

		if o.SnapmirrorLabel != nil {
			qrSnapmirrorLabel = *o.SnapmirrorLabel
		}
		qSnapmirrorLabel := qrSnapmirrorLabel
		if qSnapmirrorLabel != "" {

			if err := r.SetQueryParam("snapmirror_label", qSnapmirrorLabel); err != nil {
				return err
			}
		}
	}

	if o.State != nil {

		// query param state
		var qrState string

		if o.State != nil {
			qrState = *o.State
		}
		qState := qrState
		if qState != "" {

			if err := r.SetQueryParam("state", qState); err != nil {
				return err
			}
		}
	}

	if o.StorageUnitName != nil {

		// query param storage_unit.name
		var qrStorageUnitName string

		if o.StorageUnitName != nil {
			qrStorageUnitName = *o.StorageUnitName
		}
		qStorageUnitName := qrStorageUnitName
		if qStorageUnitName != "" {

			if err := r.SetQueryParam("storage_unit.name", qStorageUnitName); err != nil {
				return err
			}
		}
	}

	// path param storage_unit.uuid
	if err := r.SetPathParam("storage_unit.uuid", o.StorageUnitUUID); err != nil {
		return err
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

	if o.VersionUUID != nil {

		// query param version_uuid
		var qrVersionUUID string

		if o.VersionUUID != nil {
			qrVersionUUID = *o.VersionUUID
		}
		qVersionUUID := qrVersionUUID
		if qVersionUUID != "" {

			if err := r.SetQueryParam("version_uuid", qVersionUUID); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamStorageUnitSnapshotCollectionGet binds the parameter fields
func (o *StorageUnitSnapshotCollectionGetParams) bindParamFields(formats strfmt.Registry) []string {
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

// bindParamStorageUnitSnapshotCollectionGet binds the parameter order_by
func (o *StorageUnitSnapshotCollectionGetParams) bindParamOrderBy(formats strfmt.Registry) []string {
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