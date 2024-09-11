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

// StorageAvailabilityZoneResponse storage availability zone response
//
// swagger:model storage_availability_zone_response
type StorageAvailabilityZoneResponse struct {

	// links
	Links *CollectionLinks `json:"_links,omitempty"`

	// Number of Records
	NumRecords *int64 `json:"num_records,omitempty"`

	// storage availability zone response inline records
	StorageAvailabilityZoneResponseInlineRecords []*StorageAvailabilityZone `json:"records,omitempty"`
}

// Validate validates this storage availability zone response
func (m *StorageAvailabilityZoneResponse) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateStorageAvailabilityZoneResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *StorageAvailabilityZoneResponse) validateLinks(formats strfmt.Registry) error {
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

func (m *StorageAvailabilityZoneResponse) validateStorageAvailabilityZoneResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(m.StorageAvailabilityZoneResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(m.StorageAvailabilityZoneResponseInlineRecords); i++ {
		if swag.IsZero(m.StorageAvailabilityZoneResponseInlineRecords[i]) { // not required
			continue
		}

		if m.StorageAvailabilityZoneResponseInlineRecords[i] != nil {
			if err := m.StorageAvailabilityZoneResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this storage availability zone response based on the context it is used
func (m *StorageAvailabilityZoneResponse) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateStorageAvailabilityZoneResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *StorageAvailabilityZoneResponse) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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

func (m *StorageAvailabilityZoneResponse) contextValidateStorageAvailabilityZoneResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(m.StorageAvailabilityZoneResponseInlineRecords); i++ {

		if m.StorageAvailabilityZoneResponseInlineRecords[i] != nil {
			if err := m.StorageAvailabilityZoneResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
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
func (m *StorageAvailabilityZoneResponse) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *StorageAvailabilityZoneResponse) UnmarshalBinary(b []byte) error {
	var res StorageAvailabilityZoneResponse
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}