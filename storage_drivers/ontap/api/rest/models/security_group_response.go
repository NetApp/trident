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

// SecurityGroupResponse security group response
//
// swagger:model security_group_response
type SecurityGroupResponse struct {

	// links
	Links *CollectionLinks `json:"_links,omitempty"`

	// Number of records.
	// Example: 1
	NumRecords *int64 `json:"num_records,omitempty"`

	// security group response inline records
	SecurityGroupResponseInlineRecords []*SecurityGroup `json:"records,omitempty"`
}

// Validate validates this security group response
func (m *SecurityGroupResponse) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateSecurityGroupResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *SecurityGroupResponse) validateLinks(formats strfmt.Registry) error {
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

func (m *SecurityGroupResponse) validateSecurityGroupResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(m.SecurityGroupResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(m.SecurityGroupResponseInlineRecords); i++ {
		if swag.IsZero(m.SecurityGroupResponseInlineRecords[i]) { // not required
			continue
		}

		if m.SecurityGroupResponseInlineRecords[i] != nil {
			if err := m.SecurityGroupResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this security group response based on the context it is used
func (m *SecurityGroupResponse) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateSecurityGroupResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *SecurityGroupResponse) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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

func (m *SecurityGroupResponse) contextValidateSecurityGroupResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(m.SecurityGroupResponseInlineRecords); i++ {

		if m.SecurityGroupResponseInlineRecords[i] != nil {
			if err := m.SecurityGroupResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
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
func (m *SecurityGroupResponse) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *SecurityGroupResponse) UnmarshalBinary(b []byte) error {
	var res SecurityGroupResponse
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}