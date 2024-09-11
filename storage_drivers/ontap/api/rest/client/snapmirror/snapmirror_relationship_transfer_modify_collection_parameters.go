// Code generated by go-swagger; DO NOT EDIT.

package snapmirror

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

// NewSnapmirrorRelationshipTransferModifyCollectionParams creates a new SnapmirrorRelationshipTransferModifyCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewSnapmirrorRelationshipTransferModifyCollectionParams() *SnapmirrorRelationshipTransferModifyCollectionParams {
	return &SnapmirrorRelationshipTransferModifyCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewSnapmirrorRelationshipTransferModifyCollectionParamsWithTimeout creates a new SnapmirrorRelationshipTransferModifyCollectionParams object
// with the ability to set a timeout on a request.
func NewSnapmirrorRelationshipTransferModifyCollectionParamsWithTimeout(timeout time.Duration) *SnapmirrorRelationshipTransferModifyCollectionParams {
	return &SnapmirrorRelationshipTransferModifyCollectionParams{
		timeout: timeout,
	}
}

// NewSnapmirrorRelationshipTransferModifyCollectionParamsWithContext creates a new SnapmirrorRelationshipTransferModifyCollectionParams object
// with the ability to set a context for a request.
func NewSnapmirrorRelationshipTransferModifyCollectionParamsWithContext(ctx context.Context) *SnapmirrorRelationshipTransferModifyCollectionParams {
	return &SnapmirrorRelationshipTransferModifyCollectionParams{
		Context: ctx,
	}
}

// NewSnapmirrorRelationshipTransferModifyCollectionParamsWithHTTPClient creates a new SnapmirrorRelationshipTransferModifyCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewSnapmirrorRelationshipTransferModifyCollectionParamsWithHTTPClient(client *http.Client) *SnapmirrorRelationshipTransferModifyCollectionParams {
	return &SnapmirrorRelationshipTransferModifyCollectionParams{
		HTTPClient: client,
	}
}

/*
SnapmirrorRelationshipTransferModifyCollectionParams contains all the parameters to send to the API endpoint

	for the snapmirror relationship transfer modify collection operation.

	Typically these are written to a http.Request.
*/
type SnapmirrorRelationshipTransferModifyCollectionParams struct {

	/* BytesTransferred.

	   Filter by bytes_transferred
	*/
	BytesTransferred *int64

	/* CheckpointSize.

	   Filter by checkpoint_size
	*/
	CheckpointSize *int64

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* EndTime.

	   Filter by end_time
	*/
	EndTime *string

	/* ErrorInfoCode.

	   Filter by error_info.code
	*/
	ErrorInfoCode *int64

	/* ErrorInfoMessage.

	   Filter by error_info.message
	*/
	ErrorInfoMessage *string

	/* Info.

	   Info specification
	*/
	Info SnapmirrorRelationshipTransferModifyCollectionBody

	/* LastUpdatedTime.

	   Filter by last_updated_time
	*/
	LastUpdatedTime *string

	/* NetworkCompressionRatio.

	   Filter by network_compression_ratio
	*/
	NetworkCompressionRatio *string

	/* OnDemandAttrs.

	   Filter by on_demand_attrs
	*/
	OnDemandAttrs *string

	/* RelationshipDestinationClusterName.

	   Filter by relationship.destination.cluster.name
	*/
	RelationshipDestinationClusterName *string

	/* RelationshipDestinationClusterUUID.

	   Filter by relationship.destination.cluster.uuid
	*/
	RelationshipDestinationClusterUUID *string

	/* RelationshipDestinationConsistencyGroupVolumesName.

	   Filter by relationship.destination.consistency_group_volumes.name
	*/
	RelationshipDestinationConsistencyGroupVolumesName *string

	/* RelationshipDestinationLunsName.

	   Filter by relationship.destination.luns.name
	*/
	RelationshipDestinationLunsName *string

	/* RelationshipDestinationLunsUUID.

	   Filter by relationship.destination.luns.uuid
	*/
	RelationshipDestinationLunsUUID *string

	/* RelationshipDestinationPath.

	   Filter by relationship.destination.path
	*/
	RelationshipDestinationPath *string

	/* RelationshipDestinationSvmName.

	   Filter by relationship.destination.svm.name
	*/
	RelationshipDestinationSvmName *string

	/* RelationshipDestinationSvmUUID.

	   Filter by relationship.destination.svm.uuid
	*/
	RelationshipDestinationSvmUUID *string

	/* RelationshipRestore.

	   Filter by relationship.restore
	*/
	RelationshipRestore *bool

	/* RelationshipUUID.

	   SnapMirror relationship UUID
	*/
	RelationshipUUID string

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

	/* SerialRecords.

	   Perform the operation on the records synchronously.
	*/
	SerialRecords *bool

	/* Snapshot.

	   Filter by snapshot
	*/
	Snapshot *string

	/* State.

	   Filter by state
	*/
	State *string

	/* Throttle.

	   Filter by throttle
	*/
	Throttle *int64

	/* TotalDuration.

	   Filter by total_duration
	*/
	TotalDuration *string

	/* UUID.

	   Filter by uuid
	*/
	UUID *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the snapmirror relationship transfer modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithDefaults() *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the snapmirror relationship transfer modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := SnapmirrorRelationshipTransferModifyCollectionParams{
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

// WithTimeout adds the timeout to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithTimeout(timeout time.Duration) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithContext(ctx context.Context) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithHTTPClient(client *http.Client) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithBytesTransferred adds the bytesTransferred to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithBytesTransferred(bytesTransferred *int64) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetBytesTransferred(bytesTransferred)
	return o
}

// SetBytesTransferred adds the bytesTransferred to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetBytesTransferred(bytesTransferred *int64) {
	o.BytesTransferred = bytesTransferred
}

// WithCheckpointSize adds the checkpointSize to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithCheckpointSize(checkpointSize *int64) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetCheckpointSize(checkpointSize)
	return o
}

// SetCheckpointSize adds the checkpointSize to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetCheckpointSize(checkpointSize *int64) {
	o.CheckpointSize = checkpointSize
}

// WithContinueOnFailure adds the continueOnFailure to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithEndTime adds the endTime to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithEndTime(endTime *string) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetEndTime(endTime)
	return o
}

// SetEndTime adds the endTime to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetEndTime(endTime *string) {
	o.EndTime = endTime
}

// WithErrorInfoCode adds the errorInfoCode to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithErrorInfoCode(errorInfoCode *int64) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetErrorInfoCode(errorInfoCode)
	return o
}

// SetErrorInfoCode adds the errorInfoCode to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetErrorInfoCode(errorInfoCode *int64) {
	o.ErrorInfoCode = errorInfoCode
}

// WithErrorInfoMessage adds the errorInfoMessage to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithErrorInfoMessage(errorInfoMessage *string) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetErrorInfoMessage(errorInfoMessage)
	return o
}

// SetErrorInfoMessage adds the errorInfoMessage to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetErrorInfoMessage(errorInfoMessage *string) {
	o.ErrorInfoMessage = errorInfoMessage
}

// WithInfo adds the info to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithInfo(info SnapmirrorRelationshipTransferModifyCollectionBody) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetInfo(info SnapmirrorRelationshipTransferModifyCollectionBody) {
	o.Info = info
}

// WithLastUpdatedTime adds the lastUpdatedTime to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithLastUpdatedTime(lastUpdatedTime *string) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetLastUpdatedTime(lastUpdatedTime)
	return o
}

// SetLastUpdatedTime adds the lastUpdatedTime to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetLastUpdatedTime(lastUpdatedTime *string) {
	o.LastUpdatedTime = lastUpdatedTime
}

// WithNetworkCompressionRatio adds the networkCompressionRatio to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithNetworkCompressionRatio(networkCompressionRatio *string) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetNetworkCompressionRatio(networkCompressionRatio)
	return o
}

// SetNetworkCompressionRatio adds the networkCompressionRatio to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetNetworkCompressionRatio(networkCompressionRatio *string) {
	o.NetworkCompressionRatio = networkCompressionRatio
}

// WithOnDemandAttrs adds the onDemandAttrs to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithOnDemandAttrs(onDemandAttrs *string) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetOnDemandAttrs(onDemandAttrs)
	return o
}

// SetOnDemandAttrs adds the onDemandAttrs to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetOnDemandAttrs(onDemandAttrs *string) {
	o.OnDemandAttrs = onDemandAttrs
}

// WithRelationshipDestinationClusterName adds the relationshipDestinationClusterName to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithRelationshipDestinationClusterName(relationshipDestinationClusterName *string) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetRelationshipDestinationClusterName(relationshipDestinationClusterName)
	return o
}

// SetRelationshipDestinationClusterName adds the relationshipDestinationClusterName to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetRelationshipDestinationClusterName(relationshipDestinationClusterName *string) {
	o.RelationshipDestinationClusterName = relationshipDestinationClusterName
}

// WithRelationshipDestinationClusterUUID adds the relationshipDestinationClusterUUID to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithRelationshipDestinationClusterUUID(relationshipDestinationClusterUUID *string) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetRelationshipDestinationClusterUUID(relationshipDestinationClusterUUID)
	return o
}

// SetRelationshipDestinationClusterUUID adds the relationshipDestinationClusterUuid to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetRelationshipDestinationClusterUUID(relationshipDestinationClusterUUID *string) {
	o.RelationshipDestinationClusterUUID = relationshipDestinationClusterUUID
}

// WithRelationshipDestinationConsistencyGroupVolumesName adds the relationshipDestinationConsistencyGroupVolumesName to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithRelationshipDestinationConsistencyGroupVolumesName(relationshipDestinationConsistencyGroupVolumesName *string) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetRelationshipDestinationConsistencyGroupVolumesName(relationshipDestinationConsistencyGroupVolumesName)
	return o
}

// SetRelationshipDestinationConsistencyGroupVolumesName adds the relationshipDestinationConsistencyGroupVolumesName to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetRelationshipDestinationConsistencyGroupVolumesName(relationshipDestinationConsistencyGroupVolumesName *string) {
	o.RelationshipDestinationConsistencyGroupVolumesName = relationshipDestinationConsistencyGroupVolumesName
}

// WithRelationshipDestinationLunsName adds the relationshipDestinationLunsName to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithRelationshipDestinationLunsName(relationshipDestinationLunsName *string) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetRelationshipDestinationLunsName(relationshipDestinationLunsName)
	return o
}

// SetRelationshipDestinationLunsName adds the relationshipDestinationLunsName to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetRelationshipDestinationLunsName(relationshipDestinationLunsName *string) {
	o.RelationshipDestinationLunsName = relationshipDestinationLunsName
}

// WithRelationshipDestinationLunsUUID adds the relationshipDestinationLunsUUID to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithRelationshipDestinationLunsUUID(relationshipDestinationLunsUUID *string) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetRelationshipDestinationLunsUUID(relationshipDestinationLunsUUID)
	return o
}

// SetRelationshipDestinationLunsUUID adds the relationshipDestinationLunsUuid to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetRelationshipDestinationLunsUUID(relationshipDestinationLunsUUID *string) {
	o.RelationshipDestinationLunsUUID = relationshipDestinationLunsUUID
}

// WithRelationshipDestinationPath adds the relationshipDestinationPath to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithRelationshipDestinationPath(relationshipDestinationPath *string) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetRelationshipDestinationPath(relationshipDestinationPath)
	return o
}

// SetRelationshipDestinationPath adds the relationshipDestinationPath to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetRelationshipDestinationPath(relationshipDestinationPath *string) {
	o.RelationshipDestinationPath = relationshipDestinationPath
}

// WithRelationshipDestinationSvmName adds the relationshipDestinationSvmName to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithRelationshipDestinationSvmName(relationshipDestinationSvmName *string) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetRelationshipDestinationSvmName(relationshipDestinationSvmName)
	return o
}

// SetRelationshipDestinationSvmName adds the relationshipDestinationSvmName to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetRelationshipDestinationSvmName(relationshipDestinationSvmName *string) {
	o.RelationshipDestinationSvmName = relationshipDestinationSvmName
}

// WithRelationshipDestinationSvmUUID adds the relationshipDestinationSvmUUID to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithRelationshipDestinationSvmUUID(relationshipDestinationSvmUUID *string) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetRelationshipDestinationSvmUUID(relationshipDestinationSvmUUID)
	return o
}

// SetRelationshipDestinationSvmUUID adds the relationshipDestinationSvmUuid to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetRelationshipDestinationSvmUUID(relationshipDestinationSvmUUID *string) {
	o.RelationshipDestinationSvmUUID = relationshipDestinationSvmUUID
}

// WithRelationshipRestore adds the relationshipRestore to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithRelationshipRestore(relationshipRestore *bool) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetRelationshipRestore(relationshipRestore)
	return o
}

// SetRelationshipRestore adds the relationshipRestore to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetRelationshipRestore(relationshipRestore *bool) {
	o.RelationshipRestore = relationshipRestore
}

// WithRelationshipUUID adds the relationshipUUID to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithRelationshipUUID(relationshipUUID string) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetRelationshipUUID(relationshipUUID)
	return o
}

// SetRelationshipUUID adds the relationshipUuid to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetRelationshipUUID(relationshipUUID string) {
	o.RelationshipUUID = relationshipUUID
}

// WithReturnRecords adds the returnRecords to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithReturnRecords(returnRecords *bool) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithReturnTimeout(returnTimeout *int64) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithSerialRecords adds the serialRecords to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithSerialRecords(serialRecords *bool) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WithSnapshot adds the snapshot to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithSnapshot(snapshot *string) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetSnapshot(snapshot)
	return o
}

// SetSnapshot adds the snapshot to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetSnapshot(snapshot *string) {
	o.Snapshot = snapshot
}

// WithState adds the state to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithState(state *string) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetState(state)
	return o
}

// SetState adds the state to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetState(state *string) {
	o.State = state
}

// WithThrottle adds the throttle to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithThrottle(throttle *int64) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetThrottle(throttle)
	return o
}

// SetThrottle adds the throttle to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetThrottle(throttle *int64) {
	o.Throttle = throttle
}

// WithTotalDuration adds the totalDuration to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithTotalDuration(totalDuration *string) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetTotalDuration(totalDuration)
	return o
}

// SetTotalDuration adds the totalDuration to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetTotalDuration(totalDuration *string) {
	o.TotalDuration = totalDuration
}

// WithUUID adds the uuid to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WithUUID(uuid *string) *SnapmirrorRelationshipTransferModifyCollectionParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the snapmirror relationship transfer modify collection params
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) SetUUID(uuid *string) {
	o.UUID = uuid
}

// WriteToRequest writes these params to a swagger request
func (o *SnapmirrorRelationshipTransferModifyCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.BytesTransferred != nil {

		// query param bytes_transferred
		var qrBytesTransferred int64

		if o.BytesTransferred != nil {
			qrBytesTransferred = *o.BytesTransferred
		}
		qBytesTransferred := swag.FormatInt64(qrBytesTransferred)
		if qBytesTransferred != "" {

			if err := r.SetQueryParam("bytes_transferred", qBytesTransferred); err != nil {
				return err
			}
		}
	}

	if o.CheckpointSize != nil {

		// query param checkpoint_size
		var qrCheckpointSize int64

		if o.CheckpointSize != nil {
			qrCheckpointSize = *o.CheckpointSize
		}
		qCheckpointSize := swag.FormatInt64(qrCheckpointSize)
		if qCheckpointSize != "" {

			if err := r.SetQueryParam("checkpoint_size", qCheckpointSize); err != nil {
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

	if o.EndTime != nil {

		// query param end_time
		var qrEndTime string

		if o.EndTime != nil {
			qrEndTime = *o.EndTime
		}
		qEndTime := qrEndTime
		if qEndTime != "" {

			if err := r.SetQueryParam("end_time", qEndTime); err != nil {
				return err
			}
		}
	}

	if o.ErrorInfoCode != nil {

		// query param error_info.code
		var qrErrorInfoCode int64

		if o.ErrorInfoCode != nil {
			qrErrorInfoCode = *o.ErrorInfoCode
		}
		qErrorInfoCode := swag.FormatInt64(qrErrorInfoCode)
		if qErrorInfoCode != "" {

			if err := r.SetQueryParam("error_info.code", qErrorInfoCode); err != nil {
				return err
			}
		}
	}

	if o.ErrorInfoMessage != nil {

		// query param error_info.message
		var qrErrorInfoMessage string

		if o.ErrorInfoMessage != nil {
			qrErrorInfoMessage = *o.ErrorInfoMessage
		}
		qErrorInfoMessage := qrErrorInfoMessage
		if qErrorInfoMessage != "" {

			if err := r.SetQueryParam("error_info.message", qErrorInfoMessage); err != nil {
				return err
			}
		}
	}
	if err := r.SetBodyParam(o.Info); err != nil {
		return err
	}

	if o.LastUpdatedTime != nil {

		// query param last_updated_time
		var qrLastUpdatedTime string

		if o.LastUpdatedTime != nil {
			qrLastUpdatedTime = *o.LastUpdatedTime
		}
		qLastUpdatedTime := qrLastUpdatedTime
		if qLastUpdatedTime != "" {

			if err := r.SetQueryParam("last_updated_time", qLastUpdatedTime); err != nil {
				return err
			}
		}
	}

	if o.NetworkCompressionRatio != nil {

		// query param network_compression_ratio
		var qrNetworkCompressionRatio string

		if o.NetworkCompressionRatio != nil {
			qrNetworkCompressionRatio = *o.NetworkCompressionRatio
		}
		qNetworkCompressionRatio := qrNetworkCompressionRatio
		if qNetworkCompressionRatio != "" {

			if err := r.SetQueryParam("network_compression_ratio", qNetworkCompressionRatio); err != nil {
				return err
			}
		}
	}

	if o.OnDemandAttrs != nil {

		// query param on_demand_attrs
		var qrOnDemandAttrs string

		if o.OnDemandAttrs != nil {
			qrOnDemandAttrs = *o.OnDemandAttrs
		}
		qOnDemandAttrs := qrOnDemandAttrs
		if qOnDemandAttrs != "" {

			if err := r.SetQueryParam("on_demand_attrs", qOnDemandAttrs); err != nil {
				return err
			}
		}
	}

	if o.RelationshipDestinationClusterName != nil {

		// query param relationship.destination.cluster.name
		var qrRelationshipDestinationClusterName string

		if o.RelationshipDestinationClusterName != nil {
			qrRelationshipDestinationClusterName = *o.RelationshipDestinationClusterName
		}
		qRelationshipDestinationClusterName := qrRelationshipDestinationClusterName
		if qRelationshipDestinationClusterName != "" {

			if err := r.SetQueryParam("relationship.destination.cluster.name", qRelationshipDestinationClusterName); err != nil {
				return err
			}
		}
	}

	if o.RelationshipDestinationClusterUUID != nil {

		// query param relationship.destination.cluster.uuid
		var qrRelationshipDestinationClusterUUID string

		if o.RelationshipDestinationClusterUUID != nil {
			qrRelationshipDestinationClusterUUID = *o.RelationshipDestinationClusterUUID
		}
		qRelationshipDestinationClusterUUID := qrRelationshipDestinationClusterUUID
		if qRelationshipDestinationClusterUUID != "" {

			if err := r.SetQueryParam("relationship.destination.cluster.uuid", qRelationshipDestinationClusterUUID); err != nil {
				return err
			}
		}
	}

	if o.RelationshipDestinationConsistencyGroupVolumesName != nil {

		// query param relationship.destination.consistency_group_volumes.name
		var qrRelationshipDestinationConsistencyGroupVolumesName string

		if o.RelationshipDestinationConsistencyGroupVolumesName != nil {
			qrRelationshipDestinationConsistencyGroupVolumesName = *o.RelationshipDestinationConsistencyGroupVolumesName
		}
		qRelationshipDestinationConsistencyGroupVolumesName := qrRelationshipDestinationConsistencyGroupVolumesName
		if qRelationshipDestinationConsistencyGroupVolumesName != "" {

			if err := r.SetQueryParam("relationship.destination.consistency_group_volumes.name", qRelationshipDestinationConsistencyGroupVolumesName); err != nil {
				return err
			}
		}
	}

	if o.RelationshipDestinationLunsName != nil {

		// query param relationship.destination.luns.name
		var qrRelationshipDestinationLunsName string

		if o.RelationshipDestinationLunsName != nil {
			qrRelationshipDestinationLunsName = *o.RelationshipDestinationLunsName
		}
		qRelationshipDestinationLunsName := qrRelationshipDestinationLunsName
		if qRelationshipDestinationLunsName != "" {

			if err := r.SetQueryParam("relationship.destination.luns.name", qRelationshipDestinationLunsName); err != nil {
				return err
			}
		}
	}

	if o.RelationshipDestinationLunsUUID != nil {

		// query param relationship.destination.luns.uuid
		var qrRelationshipDestinationLunsUUID string

		if o.RelationshipDestinationLunsUUID != nil {
			qrRelationshipDestinationLunsUUID = *o.RelationshipDestinationLunsUUID
		}
		qRelationshipDestinationLunsUUID := qrRelationshipDestinationLunsUUID
		if qRelationshipDestinationLunsUUID != "" {

			if err := r.SetQueryParam("relationship.destination.luns.uuid", qRelationshipDestinationLunsUUID); err != nil {
				return err
			}
		}
	}

	if o.RelationshipDestinationPath != nil {

		// query param relationship.destination.path
		var qrRelationshipDestinationPath string

		if o.RelationshipDestinationPath != nil {
			qrRelationshipDestinationPath = *o.RelationshipDestinationPath
		}
		qRelationshipDestinationPath := qrRelationshipDestinationPath
		if qRelationshipDestinationPath != "" {

			if err := r.SetQueryParam("relationship.destination.path", qRelationshipDestinationPath); err != nil {
				return err
			}
		}
	}

	if o.RelationshipDestinationSvmName != nil {

		// query param relationship.destination.svm.name
		var qrRelationshipDestinationSvmName string

		if o.RelationshipDestinationSvmName != nil {
			qrRelationshipDestinationSvmName = *o.RelationshipDestinationSvmName
		}
		qRelationshipDestinationSvmName := qrRelationshipDestinationSvmName
		if qRelationshipDestinationSvmName != "" {

			if err := r.SetQueryParam("relationship.destination.svm.name", qRelationshipDestinationSvmName); err != nil {
				return err
			}
		}
	}

	if o.RelationshipDestinationSvmUUID != nil {

		// query param relationship.destination.svm.uuid
		var qrRelationshipDestinationSvmUUID string

		if o.RelationshipDestinationSvmUUID != nil {
			qrRelationshipDestinationSvmUUID = *o.RelationshipDestinationSvmUUID
		}
		qRelationshipDestinationSvmUUID := qrRelationshipDestinationSvmUUID
		if qRelationshipDestinationSvmUUID != "" {

			if err := r.SetQueryParam("relationship.destination.svm.uuid", qRelationshipDestinationSvmUUID); err != nil {
				return err
			}
		}
	}

	if o.RelationshipRestore != nil {

		// query param relationship.restore
		var qrRelationshipRestore bool

		if o.RelationshipRestore != nil {
			qrRelationshipRestore = *o.RelationshipRestore
		}
		qRelationshipRestore := swag.FormatBool(qrRelationshipRestore)
		if qRelationshipRestore != "" {

			if err := r.SetQueryParam("relationship.restore", qRelationshipRestore); err != nil {
				return err
			}
		}
	}

	// path param relationship.uuid
	if err := r.SetPathParam("relationship.uuid", o.RelationshipUUID); err != nil {
		return err
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

	if o.Snapshot != nil {

		// query param snapshot
		var qrSnapshot string

		if o.Snapshot != nil {
			qrSnapshot = *o.Snapshot
		}
		qSnapshot := qrSnapshot
		if qSnapshot != "" {

			if err := r.SetQueryParam("snapshot", qSnapshot); err != nil {
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

	if o.Throttle != nil {

		// query param throttle
		var qrThrottle int64

		if o.Throttle != nil {
			qrThrottle = *o.Throttle
		}
		qThrottle := swag.FormatInt64(qrThrottle)
		if qThrottle != "" {

			if err := r.SetQueryParam("throttle", qThrottle); err != nil {
				return err
			}
		}
	}

	if o.TotalDuration != nil {

		// query param total_duration
		var qrTotalDuration string

		if o.TotalDuration != nil {
			qrTotalDuration = *o.TotalDuration
		}
		qTotalDuration := qrTotalDuration
		if qTotalDuration != "" {

			if err := r.SetQueryParam("total_duration", qTotalDuration); err != nil {
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

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}