// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"encoding/json"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/go-openapi/validate"
)

// PerformanceQtreeMetricData Performance numbers, such as IOPS latency and throughput.
//
// swagger:model performance_qtree_metric_data
type PerformanceQtreeMetricData struct {

	// links
	Links *PerformanceQtreeMetricDataInlineLinks `json:"_links,omitempty"`

	// The duration over which this sample is calculated. The time durations are represented in the ISO-8601 standard format. Samples can be calculated over the following durations:
	//
	// Example: PT5M
	// Enum: ["PT4M","PT30M","PT2H","P1D","PT5M"]
	Duration *string `json:"duration,omitempty"`

	// iops
	Iops *PerformanceQtreeMetricDataInlineIops `json:"iops,omitempty"`

	// latency
	Latency *PerformanceQtreeMetricDataInlineLatency `json:"latency,omitempty"`

	// Errors associated with the sample. For example, if the aggregation of data over multiple nodes fails, then any partial errors might return "ok" on success or "error" on an internal uncategorized failure. Whenever a sample collection is missed but done at a later time, it is back filled to the previous 15 second timestamp and tagged with "backfilled_data". "Inconsistent_ delta_time" is encountered when the time between two collections is not the same for all nodes. Therefore, the aggregated value might be over or under inflated. "Negative_delta" is returned when an expected monotonically increasing value has decreased in value. "Inconsistent_old_data" is returned when one or more nodes do not have the latest data.
	// Example: ok
	// Read Only: true
	// Enum: ["ok","error","partial_no_data","partial_no_response","partial_other_error","negative_delta","not_found","backfilled_data","inconsistent_delta_time","inconsistent_old_data","partial_no_uuid"]
	Status *string `json:"status,omitempty"`

	// throughput
	Throughput *PerformanceQtreeMetricDataInlineThroughput `json:"throughput,omitempty"`

	// The timestamp of the performance data.
	// Example: 2017-01-25 11:20:13
	// Read Only: true
	// Format: date-time
	Timestamp *strfmt.DateTime `json:"timestamp,omitempty"`
}

// Validate validates this performance qtree metric data
func (m *PerformanceQtreeMetricData) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateDuration(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateIops(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateLatency(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateStatus(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateThroughput(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateTimestamp(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *PerformanceQtreeMetricData) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(m.Links) { // not required
		return nil
	}

	if m.Links != nil {
		if err := m.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("_links")
			}
			return err
		}
	}

	return nil
}

var performanceQtreeMetricDataTypeDurationPropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["PT4M","PT30M","PT2H","P1D","PT5M"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		performanceQtreeMetricDataTypeDurationPropEnum = append(performanceQtreeMetricDataTypeDurationPropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// performance_qtree_metric_data
	// PerformanceQtreeMetricData
	// duration
	// Duration
	// PT4M
	// END DEBUGGING
	// PerformanceQtreeMetricDataDurationPT4M captures enum value "PT4M"
	PerformanceQtreeMetricDataDurationPT4M string = "PT4M"

	// BEGIN DEBUGGING
	// performance_qtree_metric_data
	// PerformanceQtreeMetricData
	// duration
	// Duration
	// PT30M
	// END DEBUGGING
	// PerformanceQtreeMetricDataDurationPT30M captures enum value "PT30M"
	PerformanceQtreeMetricDataDurationPT30M string = "PT30M"

	// BEGIN DEBUGGING
	// performance_qtree_metric_data
	// PerformanceQtreeMetricData
	// duration
	// Duration
	// PT2H
	// END DEBUGGING
	// PerformanceQtreeMetricDataDurationPT2H captures enum value "PT2H"
	PerformanceQtreeMetricDataDurationPT2H string = "PT2H"

	// BEGIN DEBUGGING
	// performance_qtree_metric_data
	// PerformanceQtreeMetricData
	// duration
	// Duration
	// P1D
	// END DEBUGGING
	// PerformanceQtreeMetricDataDurationP1D captures enum value "P1D"
	PerformanceQtreeMetricDataDurationP1D string = "P1D"

	// BEGIN DEBUGGING
	// performance_qtree_metric_data
	// PerformanceQtreeMetricData
	// duration
	// Duration
	// PT5M
	// END DEBUGGING
	// PerformanceQtreeMetricDataDurationPT5M captures enum value "PT5M"
	PerformanceQtreeMetricDataDurationPT5M string = "PT5M"
)

// prop value enum
func (m *PerformanceQtreeMetricData) validateDurationEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, performanceQtreeMetricDataTypeDurationPropEnum, true); err != nil {
		return err
	}
	return nil
}

func (m *PerformanceQtreeMetricData) validateDuration(formats strfmt.Registry) error {
	if swag.IsZero(m.Duration) { // not required
		return nil
	}

	// value enum
	if err := m.validateDurationEnum("duration", "body", *m.Duration); err != nil {
		return err
	}

	return nil
}

func (m *PerformanceQtreeMetricData) validateIops(formats strfmt.Registry) error {
	if swag.IsZero(m.Iops) { // not required
		return nil
	}

	if m.Iops != nil {
		if err := m.Iops.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("iops")
			}
			return err
		}
	}

	return nil
}

func (m *PerformanceQtreeMetricData) validateLatency(formats strfmt.Registry) error {
	if swag.IsZero(m.Latency) { // not required
		return nil
	}

	if m.Latency != nil {
		if err := m.Latency.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("latency")
			}
			return err
		}
	}

	return nil
}

var performanceQtreeMetricDataTypeStatusPropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["ok","error","partial_no_data","partial_no_response","partial_other_error","negative_delta","not_found","backfilled_data","inconsistent_delta_time","inconsistent_old_data","partial_no_uuid"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		performanceQtreeMetricDataTypeStatusPropEnum = append(performanceQtreeMetricDataTypeStatusPropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// performance_qtree_metric_data
	// PerformanceQtreeMetricData
	// status
	// Status
	// ok
	// END DEBUGGING
	// PerformanceQtreeMetricDataStatusOk captures enum value "ok"
	PerformanceQtreeMetricDataStatusOk string = "ok"

	// BEGIN DEBUGGING
	// performance_qtree_metric_data
	// PerformanceQtreeMetricData
	// status
	// Status
	// error
	// END DEBUGGING
	// PerformanceQtreeMetricDataStatusError captures enum value "error"
	PerformanceQtreeMetricDataStatusError string = "error"

	// BEGIN DEBUGGING
	// performance_qtree_metric_data
	// PerformanceQtreeMetricData
	// status
	// Status
	// partial_no_data
	// END DEBUGGING
	// PerformanceQtreeMetricDataStatusPartialNoData captures enum value "partial_no_data"
	PerformanceQtreeMetricDataStatusPartialNoData string = "partial_no_data"

	// BEGIN DEBUGGING
	// performance_qtree_metric_data
	// PerformanceQtreeMetricData
	// status
	// Status
	// partial_no_response
	// END DEBUGGING
	// PerformanceQtreeMetricDataStatusPartialNoResponse captures enum value "partial_no_response"
	PerformanceQtreeMetricDataStatusPartialNoResponse string = "partial_no_response"

	// BEGIN DEBUGGING
	// performance_qtree_metric_data
	// PerformanceQtreeMetricData
	// status
	// Status
	// partial_other_error
	// END DEBUGGING
	// PerformanceQtreeMetricDataStatusPartialOtherError captures enum value "partial_other_error"
	PerformanceQtreeMetricDataStatusPartialOtherError string = "partial_other_error"

	// BEGIN DEBUGGING
	// performance_qtree_metric_data
	// PerformanceQtreeMetricData
	// status
	// Status
	// negative_delta
	// END DEBUGGING
	// PerformanceQtreeMetricDataStatusNegativeDelta captures enum value "negative_delta"
	PerformanceQtreeMetricDataStatusNegativeDelta string = "negative_delta"

	// BEGIN DEBUGGING
	// performance_qtree_metric_data
	// PerformanceQtreeMetricData
	// status
	// Status
	// not_found
	// END DEBUGGING
	// PerformanceQtreeMetricDataStatusNotFound captures enum value "not_found"
	PerformanceQtreeMetricDataStatusNotFound string = "not_found"

	// BEGIN DEBUGGING
	// performance_qtree_metric_data
	// PerformanceQtreeMetricData
	// status
	// Status
	// backfilled_data
	// END DEBUGGING
	// PerformanceQtreeMetricDataStatusBackfilledData captures enum value "backfilled_data"
	PerformanceQtreeMetricDataStatusBackfilledData string = "backfilled_data"

	// BEGIN DEBUGGING
	// performance_qtree_metric_data
	// PerformanceQtreeMetricData
	// status
	// Status
	// inconsistent_delta_time
	// END DEBUGGING
	// PerformanceQtreeMetricDataStatusInconsistentDeltaTime captures enum value "inconsistent_delta_time"
	PerformanceQtreeMetricDataStatusInconsistentDeltaTime string = "inconsistent_delta_time"

	// BEGIN DEBUGGING
	// performance_qtree_metric_data
	// PerformanceQtreeMetricData
	// status
	// Status
	// inconsistent_old_data
	// END DEBUGGING
	// PerformanceQtreeMetricDataStatusInconsistentOldData captures enum value "inconsistent_old_data"
	PerformanceQtreeMetricDataStatusInconsistentOldData string = "inconsistent_old_data"

	// BEGIN DEBUGGING
	// performance_qtree_metric_data
	// PerformanceQtreeMetricData
	// status
	// Status
	// partial_no_uuid
	// END DEBUGGING
	// PerformanceQtreeMetricDataStatusPartialNoUUID captures enum value "partial_no_uuid"
	PerformanceQtreeMetricDataStatusPartialNoUUID string = "partial_no_uuid"
)

// prop value enum
func (m *PerformanceQtreeMetricData) validateStatusEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, performanceQtreeMetricDataTypeStatusPropEnum, true); err != nil {
		return err
	}
	return nil
}

func (m *PerformanceQtreeMetricData) validateStatus(formats strfmt.Registry) error {
	if swag.IsZero(m.Status) { // not required
		return nil
	}

	// value enum
	if err := m.validateStatusEnum("status", "body", *m.Status); err != nil {
		return err
	}

	return nil
}

func (m *PerformanceQtreeMetricData) validateThroughput(formats strfmt.Registry) error {
	if swag.IsZero(m.Throughput) { // not required
		return nil
	}

	if m.Throughput != nil {
		if err := m.Throughput.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("throughput")
			}
			return err
		}
	}

	return nil
}

func (m *PerformanceQtreeMetricData) validateTimestamp(formats strfmt.Registry) error {
	if swag.IsZero(m.Timestamp) { // not required
		return nil
	}

	if err := validate.FormatOf("timestamp", "body", "date-time", m.Timestamp.String(), formats); err != nil {
		return err
	}

	return nil
}

// ContextValidate validate this performance qtree metric data based on the context it is used
func (m *PerformanceQtreeMetricData) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateIops(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateLatency(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateStatus(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateThroughput(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateTimestamp(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *PerformanceQtreeMetricData) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if m.Links != nil {
		if err := m.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("_links")
			}
			return err
		}
	}

	return nil
}

func (m *PerformanceQtreeMetricData) contextValidateIops(ctx context.Context, formats strfmt.Registry) error {

	if m.Iops != nil {
		if err := m.Iops.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("iops")
			}
			return err
		}
	}

	return nil
}

func (m *PerformanceQtreeMetricData) contextValidateLatency(ctx context.Context, formats strfmt.Registry) error {

	if m.Latency != nil {
		if err := m.Latency.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("latency")
			}
			return err
		}
	}

	return nil
}

func (m *PerformanceQtreeMetricData) contextValidateStatus(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "status", "body", m.Status); err != nil {
		return err
	}

	return nil
}

func (m *PerformanceQtreeMetricData) contextValidateThroughput(ctx context.Context, formats strfmt.Registry) error {

	if m.Throughput != nil {
		if err := m.Throughput.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("throughput")
			}
			return err
		}
	}

	return nil
}

func (m *PerformanceQtreeMetricData) contextValidateTimestamp(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "timestamp", "body", m.Timestamp); err != nil {
		return err
	}

	return nil
}

// MarshalBinary interface implementation
func (m *PerformanceQtreeMetricData) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *PerformanceQtreeMetricData) UnmarshalBinary(b []byte) error {
	var res PerformanceQtreeMetricData
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// PerformanceQtreeMetricDataInlineIops The rate of I/O operations observed at the storage object.
//
// swagger:model performance_qtree_metric_data_inline_iops
type PerformanceQtreeMetricDataInlineIops struct {

	// Performance metric for other I/O operations. Other I/O operations can be metadata operations, such as directory lookups and so on.
	Other *int64 `json:"other,omitempty"`

	// Performance metric for read I/O operations.
	// Example: 200
	Read *int64 `json:"read,omitempty"`

	// Performance metric aggregated over all types of I/O operations.
	// Example: 1000
	Total *int64 `json:"total,omitempty"`

	// Performance metric for write I/O operations.
	// Example: 100
	Write *int64 `json:"write,omitempty"`
}

// Validate validates this performance qtree metric data inline iops
func (m *PerformanceQtreeMetricDataInlineIops) Validate(formats strfmt.Registry) error {
	return nil
}

// ContextValidate validate this performance qtree metric data inline iops based on the context it is used
func (m *PerformanceQtreeMetricDataInlineIops) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// MarshalBinary interface implementation
func (m *PerformanceQtreeMetricDataInlineIops) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *PerformanceQtreeMetricDataInlineIops) UnmarshalBinary(b []byte) error {
	var res PerformanceQtreeMetricDataInlineIops
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// PerformanceQtreeMetricDataInlineLatency The round trip latency in microseconds observed at the storage object.
//
// swagger:model performance_qtree_metric_data_inline_latency
type PerformanceQtreeMetricDataInlineLatency struct {

	// Performance metric for other I/O operations. Other I/O operations can be metadata operations, such as directory lookups and so on.
	Other *int64 `json:"other,omitempty"`

	// Performance metric for read I/O operations.
	// Example: 200
	Read *int64 `json:"read,omitempty"`

	// Performance metric aggregated over all types of I/O operations.
	// Example: 1000
	Total *int64 `json:"total,omitempty"`

	// Performance metric for write I/O operations.
	// Example: 100
	Write *int64 `json:"write,omitempty"`
}

// Validate validates this performance qtree metric data inline latency
func (m *PerformanceQtreeMetricDataInlineLatency) Validate(formats strfmt.Registry) error {
	return nil
}

// ContextValidate validate this performance qtree metric data inline latency based on the context it is used
func (m *PerformanceQtreeMetricDataInlineLatency) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// MarshalBinary interface implementation
func (m *PerformanceQtreeMetricDataInlineLatency) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *PerformanceQtreeMetricDataInlineLatency) UnmarshalBinary(b []byte) error {
	var res PerformanceQtreeMetricDataInlineLatency
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// PerformanceQtreeMetricDataInlineLinks performance qtree metric data inline links
//
// swagger:model performance_qtree_metric_data_inline__links
type PerformanceQtreeMetricDataInlineLinks struct {

	// self
	Self *Href `json:"self,omitempty"`
}

// Validate validates this performance qtree metric data inline links
func (m *PerformanceQtreeMetricDataInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *PerformanceQtreeMetricDataInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(m.Self) { // not required
		return nil
	}

	if m.Self != nil {
		if err := m.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this performance qtree metric data inline links based on the context it is used
func (m *PerformanceQtreeMetricDataInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *PerformanceQtreeMetricDataInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if m.Self != nil {
		if err := m.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (m *PerformanceQtreeMetricDataInlineLinks) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *PerformanceQtreeMetricDataInlineLinks) UnmarshalBinary(b []byte) error {
	var res PerformanceQtreeMetricDataInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// PerformanceQtreeMetricDataInlineThroughput The rate of throughput bytes per second observed at the storage object.
//
// swagger:model performance_qtree_metric_data_inline_throughput
type PerformanceQtreeMetricDataInlineThroughput struct {

	// Performance metric for other I/O operations. Other I/O operations can be metadata operations, such as directory lookups and so on.
	Other *int64 `json:"other,omitempty"`

	// Performance metric for read I/O operations.
	// Example: 200
	Read *int64 `json:"read,omitempty"`

	// Performance metric aggregated over all types of I/O operations.
	// Example: 1000
	Total *int64 `json:"total,omitempty"`

	// Performance metric for write I/O operations.
	// Example: 100
	Write *int64 `json:"write,omitempty"`
}

// Validate validates this performance qtree metric data inline throughput
func (m *PerformanceQtreeMetricDataInlineThroughput) Validate(formats strfmt.Registry) error {
	return nil
}

// ContextValidate validate this performance qtree metric data inline throughput based on the context it is used
func (m *PerformanceQtreeMetricDataInlineThroughput) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// MarshalBinary interface implementation
func (m *PerformanceQtreeMetricDataInlineThroughput) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *PerformanceQtreeMetricDataInlineThroughput) UnmarshalBinary(b []byte) error {
	var res PerformanceQtreeMetricDataInlineThroughput
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
