// Code generated by go-swagger; DO NOT EDIT.

package storage

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// SnapshotPolicyScheduleModifyCollectionReader is a Reader for the SnapshotPolicyScheduleModifyCollection structure.
type SnapshotPolicyScheduleModifyCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *SnapshotPolicyScheduleModifyCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewSnapshotPolicyScheduleModifyCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewSnapshotPolicyScheduleModifyCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewSnapshotPolicyScheduleModifyCollectionOK creates a SnapshotPolicyScheduleModifyCollectionOK with default headers values
func NewSnapshotPolicyScheduleModifyCollectionOK() *SnapshotPolicyScheduleModifyCollectionOK {
	return &SnapshotPolicyScheduleModifyCollectionOK{}
}

/*
SnapshotPolicyScheduleModifyCollectionOK describes a response with status code 200, with default header values.

OK
*/
type SnapshotPolicyScheduleModifyCollectionOK struct {
}

// IsSuccess returns true when this snapshot policy schedule modify collection o k response has a 2xx status code
func (o *SnapshotPolicyScheduleModifyCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this snapshot policy schedule modify collection o k response has a 3xx status code
func (o *SnapshotPolicyScheduleModifyCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this snapshot policy schedule modify collection o k response has a 4xx status code
func (o *SnapshotPolicyScheduleModifyCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this snapshot policy schedule modify collection o k response has a 5xx status code
func (o *SnapshotPolicyScheduleModifyCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this snapshot policy schedule modify collection o k response a status code equal to that given
func (o *SnapshotPolicyScheduleModifyCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the snapshot policy schedule modify collection o k response
func (o *SnapshotPolicyScheduleModifyCollectionOK) Code() int {
	return 200
}

func (o *SnapshotPolicyScheduleModifyCollectionOK) Error() string {
	return fmt.Sprintf("[PATCH /storage/snapshot-policies/{snapshot_policy.uuid}/schedules][%d] snapshotPolicyScheduleModifyCollectionOK", 200)
}

func (o *SnapshotPolicyScheduleModifyCollectionOK) String() string {
	return fmt.Sprintf("[PATCH /storage/snapshot-policies/{snapshot_policy.uuid}/schedules][%d] snapshotPolicyScheduleModifyCollectionOK", 200)
}

func (o *SnapshotPolicyScheduleModifyCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewSnapshotPolicyScheduleModifyCollectionDefault creates a SnapshotPolicyScheduleModifyCollectionDefault with default headers values
func NewSnapshotPolicyScheduleModifyCollectionDefault(code int) *SnapshotPolicyScheduleModifyCollectionDefault {
	return &SnapshotPolicyScheduleModifyCollectionDefault{
		_statusCode: code,
	}
}

/*
	SnapshotPolicyScheduleModifyCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Code

| Error Code | Description |
| ---------- | ----------- |
| 1638451    | This operation would result in total snapshot count for the policy to exceed maximum supported count. |
| 918253     | Incorrect format for the retention period, duration must be in the ISO-8601 format. |
*/
type SnapshotPolicyScheduleModifyCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this snapshot policy schedule modify collection default response has a 2xx status code
func (o *SnapshotPolicyScheduleModifyCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this snapshot policy schedule modify collection default response has a 3xx status code
func (o *SnapshotPolicyScheduleModifyCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this snapshot policy schedule modify collection default response has a 4xx status code
func (o *SnapshotPolicyScheduleModifyCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this snapshot policy schedule modify collection default response has a 5xx status code
func (o *SnapshotPolicyScheduleModifyCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this snapshot policy schedule modify collection default response a status code equal to that given
func (o *SnapshotPolicyScheduleModifyCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the snapshot policy schedule modify collection default response
func (o *SnapshotPolicyScheduleModifyCollectionDefault) Code() int {
	return o._statusCode
}

func (o *SnapshotPolicyScheduleModifyCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /storage/snapshot-policies/{snapshot_policy.uuid}/schedules][%d] snapshot_policy_schedule_modify_collection default %s", o._statusCode, payload)
}

func (o *SnapshotPolicyScheduleModifyCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /storage/snapshot-policies/{snapshot_policy.uuid}/schedules][%d] snapshot_policy_schedule_modify_collection default %s", o._statusCode, payload)
}

func (o *SnapshotPolicyScheduleModifyCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *SnapshotPolicyScheduleModifyCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
SnapshotPolicyScheduleModifyCollectionBody snapshot policy schedule modify collection body
swagger:model SnapshotPolicyScheduleModifyCollectionBody
*/
type SnapshotPolicyScheduleModifyCollectionBody struct {

	// links
	Links *models.SnapshotPolicyScheduleInlineLinks `json:"_links,omitempty"`

	// The number of snapshots to maintain for this schedule.
	Count *int64 `json:"count,omitempty"`

	// The prefix to use while creating snapshots at regular intervals.
	Prefix *string `json:"prefix,omitempty"`

	// The retention period of snapshots for this schedule.
	RetentionPeriod *string `json:"retention_period,omitempty"`

	// schedule
	Schedule *models.SnapshotPolicyScheduleInlineSchedule `json:"schedule,omitempty"`

	// Label for SnapMirror operations
	SnapmirrorLabel *string `json:"snapmirror_label,omitempty"`

	// snapshot policy
	SnapshotPolicy *models.SnapshotPolicyScheduleInlineSnapshotPolicy `json:"snapshot_policy,omitempty"`

	// snapshot policy schedule response inline records
	SnapshotPolicyScheduleResponseInlineRecords []*models.SnapshotPolicySchedule `json:"records,omitempty"`
}

// Validate validates this snapshot policy schedule modify collection body
func (o *SnapshotPolicyScheduleModifyCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateSchedule(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateSnapshotPolicy(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateSnapshotPolicyScheduleResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyScheduleModifyCollectionBody) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(o.Links) { // not required
		return nil
	}

	if o.Links != nil {
		if err := o.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

func (o *SnapshotPolicyScheduleModifyCollectionBody) validateSchedule(formats strfmt.Registry) error {
	if swag.IsZero(o.Schedule) { // not required
		return nil
	}

	if o.Schedule != nil {
		if err := o.Schedule.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "schedule")
			}
			return err
		}
	}

	return nil
}

func (o *SnapshotPolicyScheduleModifyCollectionBody) validateSnapshotPolicy(formats strfmt.Registry) error {
	if swag.IsZero(o.SnapshotPolicy) { // not required
		return nil
	}

	if o.SnapshotPolicy != nil {
		if err := o.SnapshotPolicy.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "snapshot_policy")
			}
			return err
		}
	}

	return nil
}

func (o *SnapshotPolicyScheduleModifyCollectionBody) validateSnapshotPolicyScheduleResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.SnapshotPolicyScheduleResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.SnapshotPolicyScheduleResponseInlineRecords); i++ {
		if swag.IsZero(o.SnapshotPolicyScheduleResponseInlineRecords[i]) { // not required
			continue
		}

		if o.SnapshotPolicyScheduleResponseInlineRecords[i] != nil {
			if err := o.SnapshotPolicyScheduleResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this snapshot policy schedule modify collection body based on the context it is used
func (o *SnapshotPolicyScheduleModifyCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateSchedule(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateSnapshotPolicy(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateSnapshotPolicyScheduleResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyScheduleModifyCollectionBody) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if o.Links != nil {
		if err := o.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

func (o *SnapshotPolicyScheduleModifyCollectionBody) contextValidateSchedule(ctx context.Context, formats strfmt.Registry) error {

	if o.Schedule != nil {
		if err := o.Schedule.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "schedule")
			}
			return err
		}
	}

	return nil
}

func (o *SnapshotPolicyScheduleModifyCollectionBody) contextValidateSnapshotPolicy(ctx context.Context, formats strfmt.Registry) error {

	if o.SnapshotPolicy != nil {
		if err := o.SnapshotPolicy.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "snapshot_policy")
			}
			return err
		}
	}

	return nil
}

func (o *SnapshotPolicyScheduleModifyCollectionBody) contextValidateSnapshotPolicyScheduleResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.SnapshotPolicyScheduleResponseInlineRecords); i++ {

		if o.SnapshotPolicyScheduleResponseInlineRecords[i] != nil {
			if err := o.SnapshotPolicyScheduleResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// MarshalBinary interface implementation
func (o *SnapshotPolicyScheduleModifyCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *SnapshotPolicyScheduleModifyCollectionBody) UnmarshalBinary(b []byte) error {
	var res SnapshotPolicyScheduleModifyCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
SnapshotPolicyScheduleInlineLinks snapshot policy schedule inline links
swagger:model snapshot_policy_schedule_inline__links
*/
type SnapshotPolicyScheduleInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this snapshot policy schedule inline links
func (o *SnapshotPolicyScheduleInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyScheduleInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(o.Self) { // not required
		return nil
	}

	if o.Self != nil {
		if err := o.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this snapshot policy schedule inline links based on the context it is used
func (o *SnapshotPolicyScheduleInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyScheduleInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if o.Self != nil {
		if err := o.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *SnapshotPolicyScheduleInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *SnapshotPolicyScheduleInlineLinks) UnmarshalBinary(b []byte) error {
	var res SnapshotPolicyScheduleInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
SnapshotPolicyScheduleInlineSchedule snapshot policy schedule inline schedule
swagger:model snapshot_policy_schedule_inline_schedule
*/
type SnapshotPolicyScheduleInlineSchedule struct {

	// links
	Links *models.SnapshotPolicyScheduleInlineScheduleInlineLinks `json:"_links,omitempty"`

	// Job schedule name
	// Example: weekly
	Name *string `json:"name,omitempty"`

	// Job schedule UUID
	// Example: 1cd8a442-86d1-11e0-ae1c-123478563412
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this snapshot policy schedule inline schedule
func (o *SnapshotPolicyScheduleInlineSchedule) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyScheduleInlineSchedule) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(o.Links) { // not required
		return nil
	}

	if o.Links != nil {
		if err := o.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "schedule" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this snapshot policy schedule inline schedule based on the context it is used
func (o *SnapshotPolicyScheduleInlineSchedule) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyScheduleInlineSchedule) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if o.Links != nil {
		if err := o.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "schedule" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *SnapshotPolicyScheduleInlineSchedule) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *SnapshotPolicyScheduleInlineSchedule) UnmarshalBinary(b []byte) error {
	var res SnapshotPolicyScheduleInlineSchedule
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
SnapshotPolicyScheduleInlineScheduleInlineLinks snapshot policy schedule inline schedule inline links
swagger:model snapshot_policy_schedule_inline_schedule_inline__links
*/
type SnapshotPolicyScheduleInlineScheduleInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this snapshot policy schedule inline schedule inline links
func (o *SnapshotPolicyScheduleInlineScheduleInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyScheduleInlineScheduleInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(o.Self) { // not required
		return nil
	}

	if o.Self != nil {
		if err := o.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "schedule" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this snapshot policy schedule inline schedule inline links based on the context it is used
func (o *SnapshotPolicyScheduleInlineScheduleInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyScheduleInlineScheduleInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if o.Self != nil {
		if err := o.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "schedule" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *SnapshotPolicyScheduleInlineScheduleInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *SnapshotPolicyScheduleInlineScheduleInlineLinks) UnmarshalBinary(b []byte) error {
	var res SnapshotPolicyScheduleInlineScheduleInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
SnapshotPolicyScheduleInlineSnapshotPolicy This is a reference to the snapshot policy.
swagger:model snapshot_policy_schedule_inline_snapshot_policy
*/
type SnapshotPolicyScheduleInlineSnapshotPolicy struct {

	// links
	Links *models.SnapshotPolicyScheduleInlineSnapshotPolicyInlineLinks `json:"_links,omitempty"`

	// name
	// Example: default
	Name *string `json:"name,omitempty"`

	// uuid
	// Example: 1cd8a442-86d1-11e0-ae1c-123478563412
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this snapshot policy schedule inline snapshot policy
func (o *SnapshotPolicyScheduleInlineSnapshotPolicy) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyScheduleInlineSnapshotPolicy) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(o.Links) { // not required
		return nil
	}

	if o.Links != nil {
		if err := o.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "snapshot_policy" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this snapshot policy schedule inline snapshot policy based on the context it is used
func (o *SnapshotPolicyScheduleInlineSnapshotPolicy) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyScheduleInlineSnapshotPolicy) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if o.Links != nil {
		if err := o.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "snapshot_policy" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *SnapshotPolicyScheduleInlineSnapshotPolicy) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *SnapshotPolicyScheduleInlineSnapshotPolicy) UnmarshalBinary(b []byte) error {
	var res SnapshotPolicyScheduleInlineSnapshotPolicy
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
SnapshotPolicyScheduleInlineSnapshotPolicyInlineLinks snapshot policy schedule inline snapshot policy inline links
swagger:model snapshot_policy_schedule_inline_snapshot_policy_inline__links
*/
type SnapshotPolicyScheduleInlineSnapshotPolicyInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this snapshot policy schedule inline snapshot policy inline links
func (o *SnapshotPolicyScheduleInlineSnapshotPolicyInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyScheduleInlineSnapshotPolicyInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(o.Self) { // not required
		return nil
	}

	if o.Self != nil {
		if err := o.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "snapshot_policy" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this snapshot policy schedule inline snapshot policy inline links based on the context it is used
func (o *SnapshotPolicyScheduleInlineSnapshotPolicyInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyScheduleInlineSnapshotPolicyInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if o.Self != nil {
		if err := o.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "snapshot_policy" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *SnapshotPolicyScheduleInlineSnapshotPolicyInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *SnapshotPolicyScheduleInlineSnapshotPolicyInlineLinks) UnmarshalBinary(b []byte) error {
	var res SnapshotPolicyScheduleInlineSnapshotPolicyInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
