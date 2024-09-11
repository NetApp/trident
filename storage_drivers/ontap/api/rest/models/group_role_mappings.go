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

// GroupRoleMappings group role mappings
//
// swagger:model group_role_mappings
type GroupRoleMappings struct {

	// links
	Links *GroupRoleMappingsInlineLinks `json:"_links,omitempty"`

	// Any comment regarding this group entry.
	Comment *string `json:"comment,omitempty"`

	// Group ID.
	// Example: 1
	// Maximum: 4.294967295e+09
	// Minimum: 1
	GroupID *int64 `json:"group_id,omitempty"`

	// ontap role
	OntapRole *GroupRoleMappingsInlineOntapRole `json:"ontap_role,omitempty"`

	// Scope of the entity. Set to "cluster" for cluster owned objects and to "svm" for SVM owned objects.
	// Read Only: true
	// Enum: ["cluster","svm"]
	Scope *string `json:"scope,omitempty"`
}

// Validate validates this group role mappings
func (m *GroupRoleMappings) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateGroupID(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateOntapRole(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateScope(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *GroupRoleMappings) validateLinks(formats strfmt.Registry) error {
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

func (m *GroupRoleMappings) validateGroupID(formats strfmt.Registry) error {
	if swag.IsZero(m.GroupID) { // not required
		return nil
	}

	if err := validate.MinimumInt("group_id", "body", *m.GroupID, 1, false); err != nil {
		return err
	}

	if err := validate.MaximumInt("group_id", "body", *m.GroupID, 4.294967295e+09, false); err != nil {
		return err
	}

	return nil
}

func (m *GroupRoleMappings) validateOntapRole(formats strfmt.Registry) error {
	if swag.IsZero(m.OntapRole) { // not required
		return nil
	}

	if m.OntapRole != nil {
		if err := m.OntapRole.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("ontap_role")
			}
			return err
		}
	}

	return nil
}

var groupRoleMappingsTypeScopePropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["cluster","svm"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		groupRoleMappingsTypeScopePropEnum = append(groupRoleMappingsTypeScopePropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// group_role_mappings
	// GroupRoleMappings
	// scope
	// Scope
	// cluster
	// END DEBUGGING
	// GroupRoleMappingsScopeCluster captures enum value "cluster"
	GroupRoleMappingsScopeCluster string = "cluster"

	// BEGIN DEBUGGING
	// group_role_mappings
	// GroupRoleMappings
	// scope
	// Scope
	// svm
	// END DEBUGGING
	// GroupRoleMappingsScopeSvm captures enum value "svm"
	GroupRoleMappingsScopeSvm string = "svm"
)

// prop value enum
func (m *GroupRoleMappings) validateScopeEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, groupRoleMappingsTypeScopePropEnum, true); err != nil {
		return err
	}
	return nil
}

func (m *GroupRoleMappings) validateScope(formats strfmt.Registry) error {
	if swag.IsZero(m.Scope) { // not required
		return nil
	}

	// value enum
	if err := m.validateScopeEnum("scope", "body", *m.Scope); err != nil {
		return err
	}

	return nil
}

// ContextValidate validate this group role mappings based on the context it is used
func (m *GroupRoleMappings) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateOntapRole(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateScope(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *GroupRoleMappings) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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

func (m *GroupRoleMappings) contextValidateOntapRole(ctx context.Context, formats strfmt.Registry) error {

	if m.OntapRole != nil {
		if err := m.OntapRole.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("ontap_role")
			}
			return err
		}
	}

	return nil
}

func (m *GroupRoleMappings) contextValidateScope(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "scope", "body", m.Scope); err != nil {
		return err
	}

	return nil
}

// MarshalBinary interface implementation
func (m *GroupRoleMappings) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *GroupRoleMappings) UnmarshalBinary(b []byte) error {
	var res GroupRoleMappings
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// GroupRoleMappingsInlineLinks group role mappings inline links
//
// swagger:model group_role_mappings_inline__links
type GroupRoleMappingsInlineLinks struct {

	// next
	Next *Href `json:"next,omitempty"`

	// self
	Self *Href `json:"self,omitempty"`
}

// Validate validates this group role mappings inline links
func (m *GroupRoleMappingsInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateNext(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *GroupRoleMappingsInlineLinks) validateNext(formats strfmt.Registry) error {
	if swag.IsZero(m.Next) { // not required
		return nil
	}

	if m.Next != nil {
		if err := m.Next.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("_links" + "." + "next")
			}
			return err
		}
	}

	return nil
}

func (m *GroupRoleMappingsInlineLinks) validateSelf(formats strfmt.Registry) error {
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

// ContextValidate validate this group role mappings inline links based on the context it is used
func (m *GroupRoleMappingsInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateNext(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *GroupRoleMappingsInlineLinks) contextValidateNext(ctx context.Context, formats strfmt.Registry) error {

	if m.Next != nil {
		if err := m.Next.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("_links" + "." + "next")
			}
			return err
		}
	}

	return nil
}

func (m *GroupRoleMappingsInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

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
func (m *GroupRoleMappingsInlineLinks) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *GroupRoleMappingsInlineLinks) UnmarshalBinary(b []byte) error {
	var res GroupRoleMappingsInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// GroupRoleMappingsInlineOntapRole Role name.
//
// swagger:model group_role_mappings_inline_ontap_role
type GroupRoleMappingsInlineOntapRole struct {

	// links
	Links *GroupRoleMappingsInlineOntapRoleInlineLinks `json:"_links,omitempty"`

	// Role name
	// Example: admin
	Name *string `json:"name,omitempty"`
}

// Validate validates this group role mappings inline ontap role
func (m *GroupRoleMappingsInlineOntapRole) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *GroupRoleMappingsInlineOntapRole) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(m.Links) { // not required
		return nil
	}

	if m.Links != nil {
		if err := m.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("ontap_role" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this group role mappings inline ontap role based on the context it is used
func (m *GroupRoleMappingsInlineOntapRole) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *GroupRoleMappingsInlineOntapRole) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if m.Links != nil {
		if err := m.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("ontap_role" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (m *GroupRoleMappingsInlineOntapRole) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *GroupRoleMappingsInlineOntapRole) UnmarshalBinary(b []byte) error {
	var res GroupRoleMappingsInlineOntapRole
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// GroupRoleMappingsInlineOntapRoleInlineLinks group role mappings inline ontap role inline links
//
// swagger:model group_role_mappings_inline_ontap_role_inline__links
type GroupRoleMappingsInlineOntapRoleInlineLinks struct {

	// self
	Self *Href `json:"self,omitempty"`
}

// Validate validates this group role mappings inline ontap role inline links
func (m *GroupRoleMappingsInlineOntapRoleInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *GroupRoleMappingsInlineOntapRoleInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(m.Self) { // not required
		return nil
	}

	if m.Self != nil {
		if err := m.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("ontap_role" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this group role mappings inline ontap role inline links based on the context it is used
func (m *GroupRoleMappingsInlineOntapRoleInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *GroupRoleMappingsInlineOntapRoleInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if m.Self != nil {
		if err := m.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("ontap_role" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (m *GroupRoleMappingsInlineOntapRoleInlineLinks) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *GroupRoleMappingsInlineOntapRoleInlineLinks) UnmarshalBinary(b []byte) error {
	var res GroupRoleMappingsInlineOntapRoleInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}