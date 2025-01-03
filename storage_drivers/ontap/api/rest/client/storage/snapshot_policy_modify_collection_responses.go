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
	"github.com/go-openapi/validate"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// SnapshotPolicyModifyCollectionReader is a Reader for the SnapshotPolicyModifyCollection structure.
type SnapshotPolicyModifyCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *SnapshotPolicyModifyCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewSnapshotPolicyModifyCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewSnapshotPolicyModifyCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewSnapshotPolicyModifyCollectionOK creates a SnapshotPolicyModifyCollectionOK with default headers values
func NewSnapshotPolicyModifyCollectionOK() *SnapshotPolicyModifyCollectionOK {
	return &SnapshotPolicyModifyCollectionOK{}
}

/*
SnapshotPolicyModifyCollectionOK describes a response with status code 200, with default header values.

OK
*/
type SnapshotPolicyModifyCollectionOK struct {
}

// IsSuccess returns true when this snapshot policy modify collection o k response has a 2xx status code
func (o *SnapshotPolicyModifyCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this snapshot policy modify collection o k response has a 3xx status code
func (o *SnapshotPolicyModifyCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this snapshot policy modify collection o k response has a 4xx status code
func (o *SnapshotPolicyModifyCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this snapshot policy modify collection o k response has a 5xx status code
func (o *SnapshotPolicyModifyCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this snapshot policy modify collection o k response a status code equal to that given
func (o *SnapshotPolicyModifyCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the snapshot policy modify collection o k response
func (o *SnapshotPolicyModifyCollectionOK) Code() int {
	return 200
}

func (o *SnapshotPolicyModifyCollectionOK) Error() string {
	return fmt.Sprintf("[PATCH /storage/snapshot-policies][%d] snapshotPolicyModifyCollectionOK", 200)
}

func (o *SnapshotPolicyModifyCollectionOK) String() string {
	return fmt.Sprintf("[PATCH /storage/snapshot-policies][%d] snapshotPolicyModifyCollectionOK", 200)
}

func (o *SnapshotPolicyModifyCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewSnapshotPolicyModifyCollectionDefault creates a SnapshotPolicyModifyCollectionDefault with default headers values
func NewSnapshotPolicyModifyCollectionDefault(code int) *SnapshotPolicyModifyCollectionDefault {
	return &SnapshotPolicyModifyCollectionDefault{
		_statusCode: code,
	}
}

/*
	SnapshotPolicyModifyCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Code

| Error Code | Description |
| ---------- | ----------- |
| 1638414    | Cannot enable policy. Reason: Schedule not found. |
*/
type SnapshotPolicyModifyCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this snapshot policy modify collection default response has a 2xx status code
func (o *SnapshotPolicyModifyCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this snapshot policy modify collection default response has a 3xx status code
func (o *SnapshotPolicyModifyCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this snapshot policy modify collection default response has a 4xx status code
func (o *SnapshotPolicyModifyCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this snapshot policy modify collection default response has a 5xx status code
func (o *SnapshotPolicyModifyCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this snapshot policy modify collection default response a status code equal to that given
func (o *SnapshotPolicyModifyCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the snapshot policy modify collection default response
func (o *SnapshotPolicyModifyCollectionDefault) Code() int {
	return o._statusCode
}

func (o *SnapshotPolicyModifyCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /storage/snapshot-policies][%d] snapshot_policy_modify_collection default %s", o._statusCode, payload)
}

func (o *SnapshotPolicyModifyCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /storage/snapshot-policies][%d] snapshot_policy_modify_collection default %s", o._statusCode, payload)
}

func (o *SnapshotPolicyModifyCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *SnapshotPolicyModifyCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
SnapshotPolicyModifyCollectionBody snapshot policy modify collection body
swagger:model SnapshotPolicyModifyCollectionBody
*/
type SnapshotPolicyModifyCollectionBody struct {

	// links
	Links *models.SnapshotPolicyInlineLinks `json:"_links,omitempty"`

	// A comment associated with the snapshot policy.
	Comment *string `json:"comment,omitempty"`

	// Is the snapshot policy enabled?
	// Example: true
	Enabled *bool `json:"enabled,omitempty"`

	// Name of the snapshot policy.
	// Example: default
	Name *string `json:"name,omitempty"`

	// Set to "svm" when the request is on a data SVM, otherwise set to "cluster".
	// Read Only: true
	// Enum: ["svm","cluster"]
	Scope *string `json:"scope,omitempty"`

	// snapshot policy inline copies
	SnapshotPolicyInlineCopies []*models.SnapshotPolicyInlineCopiesInlineArrayItem `json:"copies,omitempty"`

	// snapshot policy response inline records
	SnapshotPolicyResponseInlineRecords []*models.SnapshotPolicy `json:"records,omitempty"`

	// svm
	Svm *models.SnapshotPolicyInlineSvm `json:"svm,omitempty"`

	// uuid
	// Example: 1cd8a442-86d1-11e0-ae1c-123478563412
	// Read Only: true
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this snapshot policy modify collection body
func (o *SnapshotPolicyModifyCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateScope(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateSnapshotPolicyInlineCopies(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateSnapshotPolicyResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateSvm(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyModifyCollectionBody) validateLinks(formats strfmt.Registry) error {
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

var snapshotPolicyModifyCollectionBodyTypeScopePropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["svm","cluster"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		snapshotPolicyModifyCollectionBodyTypeScopePropEnum = append(snapshotPolicyModifyCollectionBodyTypeScopePropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// SnapshotPolicyModifyCollectionBody
	// SnapshotPolicyModifyCollectionBody
	// scope
	// Scope
	// svm
	// END DEBUGGING
	// SnapshotPolicyModifyCollectionBodyScopeSvm captures enum value "svm"
	SnapshotPolicyModifyCollectionBodyScopeSvm string = "svm"

	// BEGIN DEBUGGING
	// SnapshotPolicyModifyCollectionBody
	// SnapshotPolicyModifyCollectionBody
	// scope
	// Scope
	// cluster
	// END DEBUGGING
	// SnapshotPolicyModifyCollectionBodyScopeCluster captures enum value "cluster"
	SnapshotPolicyModifyCollectionBodyScopeCluster string = "cluster"
)

// prop value enum
func (o *SnapshotPolicyModifyCollectionBody) validateScopeEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, snapshotPolicyModifyCollectionBodyTypeScopePropEnum, true); err != nil {
		return err
	}
	return nil
}

func (o *SnapshotPolicyModifyCollectionBody) validateScope(formats strfmt.Registry) error {
	if swag.IsZero(o.Scope) { // not required
		return nil
	}

	// value enum
	if err := o.validateScopeEnum("info"+"."+"scope", "body", *o.Scope); err != nil {
		return err
	}

	return nil
}

func (o *SnapshotPolicyModifyCollectionBody) validateSnapshotPolicyInlineCopies(formats strfmt.Registry) error {
	if swag.IsZero(o.SnapshotPolicyInlineCopies) { // not required
		return nil
	}

	for i := 0; i < len(o.SnapshotPolicyInlineCopies); i++ {
		if swag.IsZero(o.SnapshotPolicyInlineCopies[i]) { // not required
			continue
		}

		if o.SnapshotPolicyInlineCopies[i] != nil {
			if err := o.SnapshotPolicyInlineCopies[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "copies" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *SnapshotPolicyModifyCollectionBody) validateSnapshotPolicyResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.SnapshotPolicyResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.SnapshotPolicyResponseInlineRecords); i++ {
		if swag.IsZero(o.SnapshotPolicyResponseInlineRecords[i]) { // not required
			continue
		}

		if o.SnapshotPolicyResponseInlineRecords[i] != nil {
			if err := o.SnapshotPolicyResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *SnapshotPolicyModifyCollectionBody) validateSvm(formats strfmt.Registry) error {
	if swag.IsZero(o.Svm) { // not required
		return nil
	}

	if o.Svm != nil {
		if err := o.Svm.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "svm")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this snapshot policy modify collection body based on the context it is used
func (o *SnapshotPolicyModifyCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateScope(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateSnapshotPolicyInlineCopies(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateSnapshotPolicyResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateSvm(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateUUID(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyModifyCollectionBody) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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

func (o *SnapshotPolicyModifyCollectionBody) contextValidateScope(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "info"+"."+"scope", "body", o.Scope); err != nil {
		return err
	}

	return nil
}

func (o *SnapshotPolicyModifyCollectionBody) contextValidateSnapshotPolicyInlineCopies(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.SnapshotPolicyInlineCopies); i++ {

		if o.SnapshotPolicyInlineCopies[i] != nil {
			if err := o.SnapshotPolicyInlineCopies[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "copies" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *SnapshotPolicyModifyCollectionBody) contextValidateSnapshotPolicyResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.SnapshotPolicyResponseInlineRecords); i++ {

		if o.SnapshotPolicyResponseInlineRecords[i] != nil {
			if err := o.SnapshotPolicyResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *SnapshotPolicyModifyCollectionBody) contextValidateSvm(ctx context.Context, formats strfmt.Registry) error {

	if o.Svm != nil {
		if err := o.Svm.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "svm")
			}
			return err
		}
	}

	return nil
}

func (o *SnapshotPolicyModifyCollectionBody) contextValidateUUID(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "info"+"."+"uuid", "body", o.UUID); err != nil {
		return err
	}

	return nil
}

// MarshalBinary interface implementation
func (o *SnapshotPolicyModifyCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *SnapshotPolicyModifyCollectionBody) UnmarshalBinary(b []byte) error {
	var res SnapshotPolicyModifyCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
SnapshotPolicyInlineLinks snapshot policy inline links
swagger:model snapshot_policy_inline__links
*/
type SnapshotPolicyInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this snapshot policy inline links
func (o *SnapshotPolicyInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyInlineLinks) validateSelf(formats strfmt.Registry) error {
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

// ContextValidate validate this snapshot policy inline links based on the context it is used
func (o *SnapshotPolicyInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

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
func (o *SnapshotPolicyInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *SnapshotPolicyInlineLinks) UnmarshalBinary(b []byte) error {
	var res SnapshotPolicyInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
SnapshotPolicyInlineCopiesInlineArrayItem snapshot policy inline copies inline array item
swagger:model snapshot_policy_inline_copies_inline_array_item
*/
type SnapshotPolicyInlineCopiesInlineArrayItem struct {

	// The number of snapshots to maintain for this schedule.
	Count *int64 `json:"count,omitempty"`

	// The prefix to use while creating snapshots at regular intervals.
	Prefix *string `json:"prefix,omitempty"`

	// The retention period of snapshots for this schedule. The retention period value represents a duration and must be specified in the ISO-8601 duration format. The retention period can be in years, months, days, hours, and minutes. A period specified for years, months, and days is represented in the ISO-8601 format as "P<num>Y", "P<num>M", "P<num>D" respectively, for example "P10Y" represents a duration of 10 years. A duration in hours and minutes is represented by "PT<num>H" and "PT<num>M" respectively. The period string must contain only a single time element that is, either years, months, days, hours, or minutes. A duration which combines different periods is not supported, for example "P1Y10M" is not supported.
	RetentionPeriod *string `json:"retention_period,omitempty"`

	// schedule
	Schedule *models.SnapshotPolicyInlineCopiesInlineArrayItemInlineSchedule `json:"schedule,omitempty"`

	// Label for SnapMirror operations
	SnapmirrorLabel *string `json:"snapmirror_label,omitempty"`
}

// Validate validates this snapshot policy inline copies inline array item
func (o *SnapshotPolicyInlineCopiesInlineArrayItem) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSchedule(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyInlineCopiesInlineArrayItem) validateSchedule(formats strfmt.Registry) error {
	if swag.IsZero(o.Schedule) { // not required
		return nil
	}

	if o.Schedule != nil {
		if err := o.Schedule.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("schedule")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this snapshot policy inline copies inline array item based on the context it is used
func (o *SnapshotPolicyInlineCopiesInlineArrayItem) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSchedule(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyInlineCopiesInlineArrayItem) contextValidateSchedule(ctx context.Context, formats strfmt.Registry) error {

	if o.Schedule != nil {
		if err := o.Schedule.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("schedule")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *SnapshotPolicyInlineCopiesInlineArrayItem) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *SnapshotPolicyInlineCopiesInlineArrayItem) UnmarshalBinary(b []byte) error {
	var res SnapshotPolicyInlineCopiesInlineArrayItem
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
SnapshotPolicyInlineCopiesInlineArrayItemInlineSchedule snapshot policy inline copies inline array item inline schedule
swagger:model snapshot_policy_inline_copies_inline_array_item_inline_schedule
*/
type SnapshotPolicyInlineCopiesInlineArrayItemInlineSchedule struct {

	// links
	Links *models.SnapshotPolicyInlineCopiesInlineArrayItemInlineScheduleInlineLinks `json:"_links,omitempty"`

	// Job schedule name
	// Example: weekly
	Name *string `json:"name,omitempty"`

	// Job schedule UUID
	// Example: 1cd8a442-86d1-11e0-ae1c-123478563412
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this snapshot policy inline copies inline array item inline schedule
func (o *SnapshotPolicyInlineCopiesInlineArrayItemInlineSchedule) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyInlineCopiesInlineArrayItemInlineSchedule) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(o.Links) { // not required
		return nil
	}

	if o.Links != nil {
		if err := o.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("schedule" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this snapshot policy inline copies inline array item inline schedule based on the context it is used
func (o *SnapshotPolicyInlineCopiesInlineArrayItemInlineSchedule) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyInlineCopiesInlineArrayItemInlineSchedule) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if o.Links != nil {
		if err := o.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("schedule" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *SnapshotPolicyInlineCopiesInlineArrayItemInlineSchedule) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *SnapshotPolicyInlineCopiesInlineArrayItemInlineSchedule) UnmarshalBinary(b []byte) error {
	var res SnapshotPolicyInlineCopiesInlineArrayItemInlineSchedule
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
SnapshotPolicyInlineCopiesInlineArrayItemInlineScheduleInlineLinks snapshot policy inline copies inline array item inline schedule inline links
swagger:model snapshot_policy_inline_copies_inline_array_item_inline_schedule_inline__links
*/
type SnapshotPolicyInlineCopiesInlineArrayItemInlineScheduleInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this snapshot policy inline copies inline array item inline schedule inline links
func (o *SnapshotPolicyInlineCopiesInlineArrayItemInlineScheduleInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyInlineCopiesInlineArrayItemInlineScheduleInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(o.Self) { // not required
		return nil
	}

	if o.Self != nil {
		if err := o.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("schedule" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this snapshot policy inline copies inline array item inline schedule inline links based on the context it is used
func (o *SnapshotPolicyInlineCopiesInlineArrayItemInlineScheduleInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyInlineCopiesInlineArrayItemInlineScheduleInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if o.Self != nil {
		if err := o.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("schedule" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *SnapshotPolicyInlineCopiesInlineArrayItemInlineScheduleInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *SnapshotPolicyInlineCopiesInlineArrayItemInlineScheduleInlineLinks) UnmarshalBinary(b []byte) error {
	var res SnapshotPolicyInlineCopiesInlineArrayItemInlineScheduleInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
SnapshotPolicyInlineSvm SVM, applies only to SVM-scoped objects.
swagger:model snapshot_policy_inline_svm
*/
type SnapshotPolicyInlineSvm struct {

	// links
	Links *models.SnapshotPolicyInlineSvmInlineLinks `json:"_links,omitempty"`

	// The name of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: svm1
	Name *string `json:"name,omitempty"`

	// The unique identifier of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: 02c9e252-41be-11e9-81d5-00a0986138f7
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this snapshot policy inline svm
func (o *SnapshotPolicyInlineSvm) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyInlineSvm) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(o.Links) { // not required
		return nil
	}

	if o.Links != nil {
		if err := o.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "svm" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this snapshot policy inline svm based on the context it is used
func (o *SnapshotPolicyInlineSvm) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyInlineSvm) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if o.Links != nil {
		if err := o.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "svm" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *SnapshotPolicyInlineSvm) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *SnapshotPolicyInlineSvm) UnmarshalBinary(b []byte) error {
	var res SnapshotPolicyInlineSvm
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
SnapshotPolicyInlineSvmInlineLinks snapshot policy inline svm inline links
swagger:model snapshot_policy_inline_svm_inline__links
*/
type SnapshotPolicyInlineSvmInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this snapshot policy inline svm inline links
func (o *SnapshotPolicyInlineSvmInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyInlineSvmInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(o.Self) { // not required
		return nil
	}

	if o.Self != nil {
		if err := o.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "svm" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this snapshot policy inline svm inline links based on the context it is used
func (o *SnapshotPolicyInlineSvmInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapshotPolicyInlineSvmInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if o.Self != nil {
		if err := o.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "svm" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *SnapshotPolicyInlineSvmInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *SnapshotPolicyInlineSvmInlineLinks) UnmarshalBinary(b []byte) error {
	var res SnapshotPolicyInlineSvmInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
