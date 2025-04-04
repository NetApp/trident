// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/go-openapi/validate"
)

// QosPolicyReference When "min_throughput_iops", "min_throughput_mbps", "max_throughput_iops" or "max_throughput_mbps" attributes are specified, the storage object is assigned to an auto-generated QoS policy group. If the attributes are later modified, the auto-generated QoS policy-group attributes are modified. Attributes can be removed by specifying "0" and policy group by specifying "none". Upon deletion of the storage object or if the attributes are removed, then the QoS policy-group is also removed.
//
// swagger:model qos_policy_reference
type QosPolicyReference struct {

	// links
	Links *QosPolicyReferenceInlineLinks `json:"_links,omitempty"`

	// Specifies the maximum throughput in IOPS, 0 means none. This is mutually exclusive with name and UUID during POST and PATCH.
	// Example: 10000
	// Maximum: 2.147483647e+09
	// Minimum: 0
	MaxThroughputIops *int64 `json:"max_throughput_iops,omitempty"`

	// Specifies the maximum throughput in Megabytes per sec, 0 means none. This is mutually exclusive with name and UUID during POST and PATCH.
	// Example: 500
	// Maximum: 4.194303e+06
	// Minimum: 0
	MaxThroughputMbps *int64 `json:"max_throughput_mbps,omitempty"`

	// Specifies the minimum throughput in IOPS, 0 means none. Setting "min_throughput" is supported on AFF platforms only, unless FabricPool tiering policies are set. This is mutually exclusive with name and UUID during POST and PATCH.
	// Example: 2000
	// Maximum: 2.147483647e+09
	// Minimum: 0
	MinThroughputIops *int64 `json:"min_throughput_iops,omitempty"`

	// Specifies the minimum throughput in Megabytes per sec, 0 means none. This is mutually exclusive with name and UUID during POST and PATCH.
	// Example: 500
	// Maximum: 4.194303e+06
	// Minimum: 0
	MinThroughputMbps *int64 `json:"min_throughput_mbps,omitempty"`

	// The QoS policy group name. This is mutually exclusive with UUID and other QoS attributes during POST and PATCH.
	// Example: performance
	Name *string `json:"name,omitempty"`

	// The QoS policy group UUID. This is mutually exclusive with name and other QoS attributes during POST and PATCH.
	// Example: 1cd8a442-86d1-11e0-ae1c-123478563412
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this qos policy reference
func (m *QosPolicyReference) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateMaxThroughputIops(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateMaxThroughputMbps(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateMinThroughputIops(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateMinThroughputMbps(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *QosPolicyReference) validateLinks(formats strfmt.Registry) error {
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

func (m *QosPolicyReference) validateMaxThroughputIops(formats strfmt.Registry) error {
	if swag.IsZero(m.MaxThroughputIops) { // not required
		return nil
	}

	if err := validate.MinimumInt("max_throughput_iops", "body", *m.MaxThroughputIops, 0, false); err != nil {
		return err
	}

	if err := validate.MaximumInt("max_throughput_iops", "body", *m.MaxThroughputIops, 2.147483647e+09, false); err != nil {
		return err
	}

	return nil
}

func (m *QosPolicyReference) validateMaxThroughputMbps(formats strfmt.Registry) error {
	if swag.IsZero(m.MaxThroughputMbps) { // not required
		return nil
	}

	if err := validate.MinimumInt("max_throughput_mbps", "body", *m.MaxThroughputMbps, 0, false); err != nil {
		return err
	}

	if err := validate.MaximumInt("max_throughput_mbps", "body", *m.MaxThroughputMbps, 4.194303e+06, false); err != nil {
		return err
	}

	return nil
}

func (m *QosPolicyReference) validateMinThroughputIops(formats strfmt.Registry) error {
	if swag.IsZero(m.MinThroughputIops) { // not required
		return nil
	}

	if err := validate.MinimumInt("min_throughput_iops", "body", *m.MinThroughputIops, 0, false); err != nil {
		return err
	}

	if err := validate.MaximumInt("min_throughput_iops", "body", *m.MinThroughputIops, 2.147483647e+09, false); err != nil {
		return err
	}

	return nil
}

func (m *QosPolicyReference) validateMinThroughputMbps(formats strfmt.Registry) error {
	if swag.IsZero(m.MinThroughputMbps) { // not required
		return nil
	}

	if err := validate.MinimumInt("min_throughput_mbps", "body", *m.MinThroughputMbps, 0, false); err != nil {
		return err
	}

	if err := validate.MaximumInt("min_throughput_mbps", "body", *m.MinThroughputMbps, 4.194303e+06, false); err != nil {
		return err
	}

	return nil
}

// ContextValidate validate this qos policy reference based on the context it is used
func (m *QosPolicyReference) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *QosPolicyReference) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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

// MarshalBinary interface implementation
func (m *QosPolicyReference) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *QosPolicyReference) UnmarshalBinary(b []byte) error {
	var res QosPolicyReference
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// QosPolicyReferenceInlineLinks qos policy reference inline links
//
// swagger:model qos_policy_reference_inline__links
type QosPolicyReferenceInlineLinks struct {

	// self
	Self *Href `json:"self,omitempty"`
}

// Validate validates this qos policy reference inline links
func (m *QosPolicyReferenceInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *QosPolicyReferenceInlineLinks) validateSelf(formats strfmt.Registry) error {
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

// ContextValidate validate this qos policy reference inline links based on the context it is used
func (m *QosPolicyReferenceInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *QosPolicyReferenceInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

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
func (m *QosPolicyReferenceInlineLinks) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *QosPolicyReferenceInlineLinks) UnmarshalBinary(b []byte) error {
	var res QosPolicyReferenceInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
