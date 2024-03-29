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

// SplitLoad split load
//
// swagger:model split_load
type SplitLoad struct {

	// links
	Links *SelfLink `json:"_links,omitempty"`

	// load
	Load *SplitLoadInlineLoad `json:"load,omitempty"`

	// node
	Node *NodeReference `json:"node,omitempty"`
}

// Validate validates this split load
func (m *SplitLoad) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateLoad(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateNode(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *SplitLoad) validateLinks(formats strfmt.Registry) error {
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

func (m *SplitLoad) validateLoad(formats strfmt.Registry) error {
	if swag.IsZero(m.Load) { // not required
		return nil
	}

	if m.Load != nil {
		if err := m.Load.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("load")
			}
			return err
		}
	}

	return nil
}

func (m *SplitLoad) validateNode(formats strfmt.Registry) error {
	if swag.IsZero(m.Node) { // not required
		return nil
	}

	if m.Node != nil {
		if err := m.Node.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("node")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this split load based on the context it is used
func (m *SplitLoad) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateLoad(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateNode(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *SplitLoad) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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

func (m *SplitLoad) contextValidateLoad(ctx context.Context, formats strfmt.Registry) error {

	if m.Load != nil {
		if err := m.Load.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("load")
			}
			return err
		}
	}

	return nil
}

func (m *SplitLoad) contextValidateNode(ctx context.Context, formats strfmt.Registry) error {

	if m.Node != nil {
		if err := m.Node.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("node")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (m *SplitLoad) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *SplitLoad) UnmarshalBinary(b []byte) error {
	var res SplitLoad
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// SplitLoadInlineLoad split load inline load
//
// swagger:model split_load_inline_load
type SplitLoadInlineLoad struct {

	// Specifies the available file clone split load on the node.
	// Read Only: true
	Allowable *int64 `json:"allowable,omitempty"`

	// Specifies the current on-going file clone split load on the node.
	// Read Only: true
	Current *int64 `json:"current,omitempty"`

	// Specifies the maximum allowable file clone split load on the node at any point in time.
	Maximum *int64 `json:"maximum,omitempty"`

	// Specifies the file clone split load on the node reserved for tokens.
	// Read Only: true
	TokenReserved *int64 `json:"token_reserved,omitempty"`
}

// Validate validates this split load inline load
func (m *SplitLoadInlineLoad) Validate(formats strfmt.Registry) error {
	return nil
}

// ContextValidate validate this split load inline load based on the context it is used
func (m *SplitLoadInlineLoad) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateAllowable(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateCurrent(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateTokenReserved(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *SplitLoadInlineLoad) contextValidateAllowable(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "load"+"."+"allowable", "body", m.Allowable); err != nil {
		return err
	}

	return nil
}

func (m *SplitLoadInlineLoad) contextValidateCurrent(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "load"+"."+"current", "body", m.Current); err != nil {
		return err
	}

	return nil
}

func (m *SplitLoadInlineLoad) contextValidateTokenReserved(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "load"+"."+"token_reserved", "body", m.TokenReserved); err != nil {
		return err
	}

	return nil
}

// MarshalBinary interface implementation
func (m *SplitLoadInlineLoad) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *SplitLoadInlineLoad) UnmarshalBinary(b []byte) error {
	var res SplitLoadInlineLoad
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
