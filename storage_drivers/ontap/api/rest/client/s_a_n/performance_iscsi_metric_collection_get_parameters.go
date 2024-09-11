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

// NewPerformanceIscsiMetricCollectionGetParams creates a new PerformanceIscsiMetricCollectionGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewPerformanceIscsiMetricCollectionGetParams() *PerformanceIscsiMetricCollectionGetParams {
	return &PerformanceIscsiMetricCollectionGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewPerformanceIscsiMetricCollectionGetParamsWithTimeout creates a new PerformanceIscsiMetricCollectionGetParams object
// with the ability to set a timeout on a request.
func NewPerformanceIscsiMetricCollectionGetParamsWithTimeout(timeout time.Duration) *PerformanceIscsiMetricCollectionGetParams {
	return &PerformanceIscsiMetricCollectionGetParams{
		timeout: timeout,
	}
}

// NewPerformanceIscsiMetricCollectionGetParamsWithContext creates a new PerformanceIscsiMetricCollectionGetParams object
// with the ability to set a context for a request.
func NewPerformanceIscsiMetricCollectionGetParamsWithContext(ctx context.Context) *PerformanceIscsiMetricCollectionGetParams {
	return &PerformanceIscsiMetricCollectionGetParams{
		Context: ctx,
	}
}

// NewPerformanceIscsiMetricCollectionGetParamsWithHTTPClient creates a new PerformanceIscsiMetricCollectionGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewPerformanceIscsiMetricCollectionGetParamsWithHTTPClient(client *http.Client) *PerformanceIscsiMetricCollectionGetParams {
	return &PerformanceIscsiMetricCollectionGetParams{
		HTTPClient: client,
	}
}

/*
PerformanceIscsiMetricCollectionGetParams contains all the parameters to send to the API endpoint

	for the performance iscsi metric collection get operation.

	Typically these are written to a http.Request.
*/
type PerformanceIscsiMetricCollectionGetParams struct {

	/* Duration.

	   Filter by duration
	*/
	Duration *string

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* Interval.

	     The time range for the data. Examples can be 1h, 1d, 1m, 1w, 1y.
	The period for each time range is as follows:
	* 1h: Metrics over the most recent hour sampled over 15 seconds.
	* 1d: Metrics over the most recent day sampled over 5 minutes.
	* 1w: Metrics over the most recent week sampled over 30 minutes.
	* 1m: Metrics over the most recent month sampled over 2 hours.
	* 1y: Metrics over the most recent year sampled over a day.


	     Default: "1h"
	*/
	Interval *string

	/* IopsOther.

	   Filter by iops.other
	*/
	IopsOther *int64

	/* IopsRead.

	   Filter by iops.read
	*/
	IopsRead *int64

	/* IopsTotal.

	   Filter by iops.total
	*/
	IopsTotal *int64

	/* IopsWrite.

	   Filter by iops.write
	*/
	IopsWrite *int64

	/* LatencyOther.

	   Filter by latency.other
	*/
	LatencyOther *int64

	/* LatencyRead.

	   Filter by latency.read
	*/
	LatencyRead *int64

	/* LatencyTotal.

	   Filter by latency.total
	*/
	LatencyTotal *int64

	/* LatencyWrite.

	   Filter by latency.write
	*/
	LatencyWrite *int64

	/* MaxRecords.

	   Limit the number of records returned.
	*/
	MaxRecords *int64

	/* OrderBy.

	   Order results by specified fields and optional [asc|desc] direction. Default direction is 'asc' for ascending.
	*/
	OrderBy []string

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

	/* Status.

	   Filter by status
	*/
	Status *string

	/* SvmUUID.

	   The unique identifier of the SVM.

	*/
	SvmUUID string

	/* ThroughputRead.

	   Filter by throughput.read
	*/
	ThroughputRead *int64

	/* ThroughputTotal.

	   Filter by throughput.total
	*/
	ThroughputTotal *int64

	/* ThroughputWrite.

	   Filter by throughput.write
	*/
	ThroughputWrite *int64

	/* Timestamp.

	   Filter by timestamp
	*/
	Timestamp *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the performance iscsi metric collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *PerformanceIscsiMetricCollectionGetParams) WithDefaults() *PerformanceIscsiMetricCollectionGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the performance iscsi metric collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *PerformanceIscsiMetricCollectionGetParams) SetDefaults() {
	var (
		intervalDefault = string("1h")

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)
	)

	val := PerformanceIscsiMetricCollectionGetParams{
		Interval:      &intervalDefault,
		ReturnRecords: &returnRecordsDefault,
		ReturnTimeout: &returnTimeoutDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithTimeout(timeout time.Duration) *PerformanceIscsiMetricCollectionGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithContext(ctx context.Context) *PerformanceIscsiMetricCollectionGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithHTTPClient(client *http.Client) *PerformanceIscsiMetricCollectionGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithDuration adds the duration to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithDuration(duration *string) *PerformanceIscsiMetricCollectionGetParams {
	o.SetDuration(duration)
	return o
}

// SetDuration adds the duration to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetDuration(duration *string) {
	o.Duration = duration
}

// WithFields adds the fields to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithFields(fields []string) *PerformanceIscsiMetricCollectionGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithInterval adds the interval to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithInterval(interval *string) *PerformanceIscsiMetricCollectionGetParams {
	o.SetInterval(interval)
	return o
}

// SetInterval adds the interval to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetInterval(interval *string) {
	o.Interval = interval
}

// WithIopsOther adds the iopsOther to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithIopsOther(iopsOther *int64) *PerformanceIscsiMetricCollectionGetParams {
	o.SetIopsOther(iopsOther)
	return o
}

// SetIopsOther adds the iopsOther to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetIopsOther(iopsOther *int64) {
	o.IopsOther = iopsOther
}

// WithIopsRead adds the iopsRead to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithIopsRead(iopsRead *int64) *PerformanceIscsiMetricCollectionGetParams {
	o.SetIopsRead(iopsRead)
	return o
}

// SetIopsRead adds the iopsRead to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetIopsRead(iopsRead *int64) {
	o.IopsRead = iopsRead
}

// WithIopsTotal adds the iopsTotal to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithIopsTotal(iopsTotal *int64) *PerformanceIscsiMetricCollectionGetParams {
	o.SetIopsTotal(iopsTotal)
	return o
}

// SetIopsTotal adds the iopsTotal to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetIopsTotal(iopsTotal *int64) {
	o.IopsTotal = iopsTotal
}

// WithIopsWrite adds the iopsWrite to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithIopsWrite(iopsWrite *int64) *PerformanceIscsiMetricCollectionGetParams {
	o.SetIopsWrite(iopsWrite)
	return o
}

// SetIopsWrite adds the iopsWrite to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetIopsWrite(iopsWrite *int64) {
	o.IopsWrite = iopsWrite
}

// WithLatencyOther adds the latencyOther to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithLatencyOther(latencyOther *int64) *PerformanceIscsiMetricCollectionGetParams {
	o.SetLatencyOther(latencyOther)
	return o
}

// SetLatencyOther adds the latencyOther to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetLatencyOther(latencyOther *int64) {
	o.LatencyOther = latencyOther
}

// WithLatencyRead adds the latencyRead to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithLatencyRead(latencyRead *int64) *PerformanceIscsiMetricCollectionGetParams {
	o.SetLatencyRead(latencyRead)
	return o
}

// SetLatencyRead adds the latencyRead to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetLatencyRead(latencyRead *int64) {
	o.LatencyRead = latencyRead
}

// WithLatencyTotal adds the latencyTotal to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithLatencyTotal(latencyTotal *int64) *PerformanceIscsiMetricCollectionGetParams {
	o.SetLatencyTotal(latencyTotal)
	return o
}

// SetLatencyTotal adds the latencyTotal to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetLatencyTotal(latencyTotal *int64) {
	o.LatencyTotal = latencyTotal
}

// WithLatencyWrite adds the latencyWrite to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithLatencyWrite(latencyWrite *int64) *PerformanceIscsiMetricCollectionGetParams {
	o.SetLatencyWrite(latencyWrite)
	return o
}

// SetLatencyWrite adds the latencyWrite to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetLatencyWrite(latencyWrite *int64) {
	o.LatencyWrite = latencyWrite
}

// WithMaxRecords adds the maxRecords to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithMaxRecords(maxRecords *int64) *PerformanceIscsiMetricCollectionGetParams {
	o.SetMaxRecords(maxRecords)
	return o
}

// SetMaxRecords adds the maxRecords to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetMaxRecords(maxRecords *int64) {
	o.MaxRecords = maxRecords
}

// WithOrderBy adds the orderBy to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithOrderBy(orderBy []string) *PerformanceIscsiMetricCollectionGetParams {
	o.SetOrderBy(orderBy)
	return o
}

// SetOrderBy adds the orderBy to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetOrderBy(orderBy []string) {
	o.OrderBy = orderBy
}

// WithReturnRecords adds the returnRecords to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithReturnRecords(returnRecords *bool) *PerformanceIscsiMetricCollectionGetParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithReturnTimeout(returnTimeout *int64) *PerformanceIscsiMetricCollectionGetParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithStatus adds the status to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithStatus(status *string) *PerformanceIscsiMetricCollectionGetParams {
	o.SetStatus(status)
	return o
}

// SetStatus adds the status to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetStatus(status *string) {
	o.Status = status
}

// WithSvmUUID adds the svmUUID to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithSvmUUID(svmUUID string) *PerformanceIscsiMetricCollectionGetParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetSvmUUID(svmUUID string) {
	o.SvmUUID = svmUUID
}

// WithThroughputRead adds the throughputRead to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithThroughputRead(throughputRead *int64) *PerformanceIscsiMetricCollectionGetParams {
	o.SetThroughputRead(throughputRead)
	return o
}

// SetThroughputRead adds the throughputRead to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetThroughputRead(throughputRead *int64) {
	o.ThroughputRead = throughputRead
}

// WithThroughputTotal adds the throughputTotal to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithThroughputTotal(throughputTotal *int64) *PerformanceIscsiMetricCollectionGetParams {
	o.SetThroughputTotal(throughputTotal)
	return o
}

// SetThroughputTotal adds the throughputTotal to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetThroughputTotal(throughputTotal *int64) {
	o.ThroughputTotal = throughputTotal
}

// WithThroughputWrite adds the throughputWrite to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithThroughputWrite(throughputWrite *int64) *PerformanceIscsiMetricCollectionGetParams {
	o.SetThroughputWrite(throughputWrite)
	return o
}

// SetThroughputWrite adds the throughputWrite to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetThroughputWrite(throughputWrite *int64) {
	o.ThroughputWrite = throughputWrite
}

// WithTimestamp adds the timestamp to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) WithTimestamp(timestamp *string) *PerformanceIscsiMetricCollectionGetParams {
	o.SetTimestamp(timestamp)
	return o
}

// SetTimestamp adds the timestamp to the performance iscsi metric collection get params
func (o *PerformanceIscsiMetricCollectionGetParams) SetTimestamp(timestamp *string) {
	o.Timestamp = timestamp
}

// WriteToRequest writes these params to a swagger request
func (o *PerformanceIscsiMetricCollectionGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.Duration != nil {

		// query param duration
		var qrDuration string

		if o.Duration != nil {
			qrDuration = *o.Duration
		}
		qDuration := qrDuration
		if qDuration != "" {

			if err := r.SetQueryParam("duration", qDuration); err != nil {
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

	if o.Interval != nil {

		// query param interval
		var qrInterval string

		if o.Interval != nil {
			qrInterval = *o.Interval
		}
		qInterval := qrInterval
		if qInterval != "" {

			if err := r.SetQueryParam("interval", qInterval); err != nil {
				return err
			}
		}
	}

	if o.IopsOther != nil {

		// query param iops.other
		var qrIopsOther int64

		if o.IopsOther != nil {
			qrIopsOther = *o.IopsOther
		}
		qIopsOther := swag.FormatInt64(qrIopsOther)
		if qIopsOther != "" {

			if err := r.SetQueryParam("iops.other", qIopsOther); err != nil {
				return err
			}
		}
	}

	if o.IopsRead != nil {

		// query param iops.read
		var qrIopsRead int64

		if o.IopsRead != nil {
			qrIopsRead = *o.IopsRead
		}
		qIopsRead := swag.FormatInt64(qrIopsRead)
		if qIopsRead != "" {

			if err := r.SetQueryParam("iops.read", qIopsRead); err != nil {
				return err
			}
		}
	}

	if o.IopsTotal != nil {

		// query param iops.total
		var qrIopsTotal int64

		if o.IopsTotal != nil {
			qrIopsTotal = *o.IopsTotal
		}
		qIopsTotal := swag.FormatInt64(qrIopsTotal)
		if qIopsTotal != "" {

			if err := r.SetQueryParam("iops.total", qIopsTotal); err != nil {
				return err
			}
		}
	}

	if o.IopsWrite != nil {

		// query param iops.write
		var qrIopsWrite int64

		if o.IopsWrite != nil {
			qrIopsWrite = *o.IopsWrite
		}
		qIopsWrite := swag.FormatInt64(qrIopsWrite)
		if qIopsWrite != "" {

			if err := r.SetQueryParam("iops.write", qIopsWrite); err != nil {
				return err
			}
		}
	}

	if o.LatencyOther != nil {

		// query param latency.other
		var qrLatencyOther int64

		if o.LatencyOther != nil {
			qrLatencyOther = *o.LatencyOther
		}
		qLatencyOther := swag.FormatInt64(qrLatencyOther)
		if qLatencyOther != "" {

			if err := r.SetQueryParam("latency.other", qLatencyOther); err != nil {
				return err
			}
		}
	}

	if o.LatencyRead != nil {

		// query param latency.read
		var qrLatencyRead int64

		if o.LatencyRead != nil {
			qrLatencyRead = *o.LatencyRead
		}
		qLatencyRead := swag.FormatInt64(qrLatencyRead)
		if qLatencyRead != "" {

			if err := r.SetQueryParam("latency.read", qLatencyRead); err != nil {
				return err
			}
		}
	}

	if o.LatencyTotal != nil {

		// query param latency.total
		var qrLatencyTotal int64

		if o.LatencyTotal != nil {
			qrLatencyTotal = *o.LatencyTotal
		}
		qLatencyTotal := swag.FormatInt64(qrLatencyTotal)
		if qLatencyTotal != "" {

			if err := r.SetQueryParam("latency.total", qLatencyTotal); err != nil {
				return err
			}
		}
	}

	if o.LatencyWrite != nil {

		// query param latency.write
		var qrLatencyWrite int64

		if o.LatencyWrite != nil {
			qrLatencyWrite = *o.LatencyWrite
		}
		qLatencyWrite := swag.FormatInt64(qrLatencyWrite)
		if qLatencyWrite != "" {

			if err := r.SetQueryParam("latency.write", qLatencyWrite); err != nil {
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

	if o.OrderBy != nil {

		// binding items for order_by
		joinedOrderBy := o.bindParamOrderBy(reg)

		// query array param order_by
		if err := r.SetQueryParam("order_by", joinedOrderBy...); err != nil {
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

	if o.Status != nil {

		// query param status
		var qrStatus string

		if o.Status != nil {
			qrStatus = *o.Status
		}
		qStatus := qrStatus
		if qStatus != "" {

			if err := r.SetQueryParam("status", qStatus); err != nil {
				return err
			}
		}
	}

	// path param svm.uuid
	if err := r.SetPathParam("svm.uuid", o.SvmUUID); err != nil {
		return err
	}

	if o.ThroughputRead != nil {

		// query param throughput.read
		var qrThroughputRead int64

		if o.ThroughputRead != nil {
			qrThroughputRead = *o.ThroughputRead
		}
		qThroughputRead := swag.FormatInt64(qrThroughputRead)
		if qThroughputRead != "" {

			if err := r.SetQueryParam("throughput.read", qThroughputRead); err != nil {
				return err
			}
		}
	}

	if o.ThroughputTotal != nil {

		// query param throughput.total
		var qrThroughputTotal int64

		if o.ThroughputTotal != nil {
			qrThroughputTotal = *o.ThroughputTotal
		}
		qThroughputTotal := swag.FormatInt64(qrThroughputTotal)
		if qThroughputTotal != "" {

			if err := r.SetQueryParam("throughput.total", qThroughputTotal); err != nil {
				return err
			}
		}
	}

	if o.ThroughputWrite != nil {

		// query param throughput.write
		var qrThroughputWrite int64

		if o.ThroughputWrite != nil {
			qrThroughputWrite = *o.ThroughputWrite
		}
		qThroughputWrite := swag.FormatInt64(qrThroughputWrite)
		if qThroughputWrite != "" {

			if err := r.SetQueryParam("throughput.write", qThroughputWrite); err != nil {
				return err
			}
		}
	}

	if o.Timestamp != nil {

		// query param timestamp
		var qrTimestamp string

		if o.Timestamp != nil {
			qrTimestamp = *o.Timestamp
		}
		qTimestamp := qrTimestamp
		if qTimestamp != "" {

			if err := r.SetQueryParam("timestamp", qTimestamp); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamPerformanceIscsiMetricCollectionGet binds the parameter fields
func (o *PerformanceIscsiMetricCollectionGetParams) bindParamFields(formats strfmt.Registry) []string {
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

// bindParamPerformanceIscsiMetricCollectionGet binds the parameter order_by
func (o *PerformanceIscsiMetricCollectionGetParams) bindParamOrderBy(formats strfmt.Registry) []string {
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