// Code generated by go-swagger; DO NOT EDIT.

package storage

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

// NewVolumeEfficiencyPolicyModifyCollectionParams creates a new VolumeEfficiencyPolicyModifyCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewVolumeEfficiencyPolicyModifyCollectionParams() *VolumeEfficiencyPolicyModifyCollectionParams {
	return &VolumeEfficiencyPolicyModifyCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewVolumeEfficiencyPolicyModifyCollectionParamsWithTimeout creates a new VolumeEfficiencyPolicyModifyCollectionParams object
// with the ability to set a timeout on a request.
func NewVolumeEfficiencyPolicyModifyCollectionParamsWithTimeout(timeout time.Duration) *VolumeEfficiencyPolicyModifyCollectionParams {
	return &VolumeEfficiencyPolicyModifyCollectionParams{
		timeout: timeout,
	}
}

// NewVolumeEfficiencyPolicyModifyCollectionParamsWithContext creates a new VolumeEfficiencyPolicyModifyCollectionParams object
// with the ability to set a context for a request.
func NewVolumeEfficiencyPolicyModifyCollectionParamsWithContext(ctx context.Context) *VolumeEfficiencyPolicyModifyCollectionParams {
	return &VolumeEfficiencyPolicyModifyCollectionParams{
		Context: ctx,
	}
}

// NewVolumeEfficiencyPolicyModifyCollectionParamsWithHTTPClient creates a new VolumeEfficiencyPolicyModifyCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewVolumeEfficiencyPolicyModifyCollectionParamsWithHTTPClient(client *http.Client) *VolumeEfficiencyPolicyModifyCollectionParams {
	return &VolumeEfficiencyPolicyModifyCollectionParams{
		HTTPClient: client,
	}
}

/*
VolumeEfficiencyPolicyModifyCollectionParams contains all the parameters to send to the API endpoint

	for the volume efficiency policy modify collection operation.

	Typically these are written to a http.Request.
*/
type VolumeEfficiencyPolicyModifyCollectionParams struct {

	/* Comment.

	   Filter by comment
	*/
	Comment *string

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* Duration.

	   Filter by duration
	*/
	Duration *int64

	/* Enabled.

	   Filter by enabled
	*/
	Enabled *bool

	/* Info.

	   Info specification
	*/
	Info VolumeEfficiencyPolicyModifyCollectionBody

	/* Name.

	   Filter by name
	*/
	Name *string

	/* QosPolicy.

	   Filter by qos_policy
	*/
	QosPolicy *string

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

	/* ScheduleName.

	   Filter by schedule.name
	*/
	ScheduleName *string

	/* SerialRecords.

	   Perform the operation on the records synchronously.
	*/
	SerialRecords *bool

	/* StartThresholdPercent.

	   Filter by start_threshold_percent
	*/
	StartThresholdPercent *int64

	/* SvmName.

	   Filter by svm.name
	*/
	SvmName *string

	/* SvmUUID.

	   Filter by svm.uuid
	*/
	SvmUUID *string

	/* Type.

	   Filter by type
	*/
	Type *string

	/* UUID.

	   Filter by uuid
	*/
	UUID *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the volume efficiency policy modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithDefaults() *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the volume efficiency policy modify collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := VolumeEfficiencyPolicyModifyCollectionParams{
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

// WithTimeout adds the timeout to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithTimeout(timeout time.Duration) *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithContext(ctx context.Context) *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithHTTPClient(client *http.Client) *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithComment adds the comment to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithComment(comment *string) *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetComment(comment)
	return o
}

// SetComment adds the comment to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetComment(comment *string) {
	o.Comment = comment
}

// WithContinueOnFailure adds the continueOnFailure to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithDuration adds the duration to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithDuration(duration *int64) *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetDuration(duration)
	return o
}

// SetDuration adds the duration to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetDuration(duration *int64) {
	o.Duration = duration
}

// WithEnabled adds the enabled to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithEnabled(enabled *bool) *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetEnabled(enabled)
	return o
}

// SetEnabled adds the enabled to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetEnabled(enabled *bool) {
	o.Enabled = enabled
}

// WithInfo adds the info to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithInfo(info VolumeEfficiencyPolicyModifyCollectionBody) *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetInfo(info VolumeEfficiencyPolicyModifyCollectionBody) {
	o.Info = info
}

// WithName adds the name to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithName(name *string) *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetName(name *string) {
	o.Name = name
}

// WithQosPolicy adds the qosPolicy to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithQosPolicy(qosPolicy *string) *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetQosPolicy(qosPolicy)
	return o
}

// SetQosPolicy adds the qosPolicy to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetQosPolicy(qosPolicy *string) {
	o.QosPolicy = qosPolicy
}

// WithReturnRecords adds the returnRecords to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithReturnRecords(returnRecords *bool) *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithReturnTimeout(returnTimeout *int64) *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithScheduleName adds the scheduleName to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithScheduleName(scheduleName *string) *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetScheduleName(scheduleName)
	return o
}

// SetScheduleName adds the scheduleName to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetScheduleName(scheduleName *string) {
	o.ScheduleName = scheduleName
}

// WithSerialRecords adds the serialRecords to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithSerialRecords(serialRecords *bool) *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WithStartThresholdPercent adds the startThresholdPercent to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithStartThresholdPercent(startThresholdPercent *int64) *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetStartThresholdPercent(startThresholdPercent)
	return o
}

// SetStartThresholdPercent adds the startThresholdPercent to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetStartThresholdPercent(startThresholdPercent *int64) {
	o.StartThresholdPercent = startThresholdPercent
}

// WithSvmName adds the svmName to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithSvmName(svmName *string) *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetSvmName(svmName)
	return o
}

// SetSvmName adds the svmName to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetSvmName(svmName *string) {
	o.SvmName = svmName
}

// WithSvmUUID adds the svmUUID to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithSvmUUID(svmUUID *string) *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetSvmUUID(svmUUID *string) {
	o.SvmUUID = svmUUID
}

// WithType adds the typeVar to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithType(typeVar *string) *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetType(typeVar)
	return o
}

// SetType adds the type to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetType(typeVar *string) {
	o.Type = typeVar
}

// WithUUID adds the uuid to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WithUUID(uuid *string) *VolumeEfficiencyPolicyModifyCollectionParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the volume efficiency policy modify collection params
func (o *VolumeEfficiencyPolicyModifyCollectionParams) SetUUID(uuid *string) {
	o.UUID = uuid
}

// WriteToRequest writes these params to a swagger request
func (o *VolumeEfficiencyPolicyModifyCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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

	if o.Duration != nil {

		// query param duration
		var qrDuration int64

		if o.Duration != nil {
			qrDuration = *o.Duration
		}
		qDuration := swag.FormatInt64(qrDuration)
		if qDuration != "" {

			if err := r.SetQueryParam("duration", qDuration); err != nil {
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

	if o.QosPolicy != nil {

		// query param qos_policy
		var qrQosPolicy string

		if o.QosPolicy != nil {
			qrQosPolicy = *o.QosPolicy
		}
		qQosPolicy := qrQosPolicy
		if qQosPolicy != "" {

			if err := r.SetQueryParam("qos_policy", qQosPolicy); err != nil {
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

	if o.StartThresholdPercent != nil {

		// query param start_threshold_percent
		var qrStartThresholdPercent int64

		if o.StartThresholdPercent != nil {
			qrStartThresholdPercent = *o.StartThresholdPercent
		}
		qStartThresholdPercent := swag.FormatInt64(qrStartThresholdPercent)
		if qStartThresholdPercent != "" {

			if err := r.SetQueryParam("start_threshold_percent", qStartThresholdPercent); err != nil {
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
