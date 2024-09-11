// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"strconv"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// TapeDeviceJobLinkResponse tape device job link response
//
// swagger:model tape_device_job_link_response
type TapeDeviceJobLinkResponse struct {

	// job
	Job *JobLink `json:"job,omitempty"`

	// Number of records
	// Example: 1
	NumRecords int64 `json:"num_records,omitempty"`

	// records
	Records []*TapeDevice `json:"records"`
}

// Validate validates this tape device job link response
func (m *TapeDeviceJobLinkResponse) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateJob(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *TapeDeviceJobLinkResponse) validateJob(formats strfmt.Registry) error {
	if swag.IsZero(m.Job) { // not required
		return nil
	}

	if m.Job != nil {
		if err := m.Job.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("job")
			}
			return err
		}
	}

	return nil
}

func (m *TapeDeviceJobLinkResponse) validateRecords(formats strfmt.Registry) error {
	if swag.IsZero(m.Records) { // not required
		return nil
	}

	for i := 0; i < len(m.Records); i++ {
		if swag.IsZero(m.Records[i]) { // not required
			continue
		}

		if m.Records[i] != nil {
			if err := m.Records[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this tape device job link response based on the context it is used
func (m *TapeDeviceJobLinkResponse) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateJob(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *TapeDeviceJobLinkResponse) contextValidateJob(ctx context.Context, formats strfmt.Registry) error {

	if m.Job != nil {
		if err := m.Job.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("job")
			}
			return err
		}
	}

	return nil
}

func (m *TapeDeviceJobLinkResponse) contextValidateRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(m.Records); i++ {

		if m.Records[i] != nil {
			if err := m.Records[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// MarshalBinary interface implementation
func (m *TapeDeviceJobLinkResponse) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *TapeDeviceJobLinkResponse) UnmarshalBinary(b []byte) error {
	var res TapeDeviceJobLinkResponse
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}