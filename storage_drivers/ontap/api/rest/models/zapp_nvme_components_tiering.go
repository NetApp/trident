// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/go-openapi/validate"
)

// ZappNvmeComponentsTiering application-components.tiering
//
// swagger:model zapp_nvme_components_tiering
type ZappNvmeComponentsTiering struct {

	// Storage tiering placement rules for the container(s)
	// Enum: ["required","best_effort","disallowed"]
	Control *string `json:"control,omitempty"`

	// The storage tiering type of the application component.
	// Enum: ["all","auto","none","snapshot_only"]
	Policy *string `json:"policy,omitempty"`

	// zapp nvme components tiering inline object stores
	ZappNvmeComponentsTieringInlineObjectStores []*ZappNvmeComponentsTieringInlineObjectStoresInlineArrayItem `json:"object_stores,omitempty"`
}

// Validate validates this zapp nvme components tiering
func (m *ZappNvmeComponentsTiering) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateControl(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validatePolicy(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateZappNvmeComponentsTieringInlineObjectStores(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

var zappNvmeComponentsTieringTypeControlPropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["required","best_effort","disallowed"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		zappNvmeComponentsTieringTypeControlPropEnum = append(zappNvmeComponentsTieringTypeControlPropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// zapp_nvme_components_tiering
	// ZappNvmeComponentsTiering
	// control
	// Control
	// required
	// END DEBUGGING
	// ZappNvmeComponentsTieringControlRequired captures enum value "required"
	ZappNvmeComponentsTieringControlRequired string = "required"

	// BEGIN DEBUGGING
	// zapp_nvme_components_tiering
	// ZappNvmeComponentsTiering
	// control
	// Control
	// best_effort
	// END DEBUGGING
	// ZappNvmeComponentsTieringControlBestEffort captures enum value "best_effort"
	ZappNvmeComponentsTieringControlBestEffort string = "best_effort"

	// BEGIN DEBUGGING
	// zapp_nvme_components_tiering
	// ZappNvmeComponentsTiering
	// control
	// Control
	// disallowed
	// END DEBUGGING
	// ZappNvmeComponentsTieringControlDisallowed captures enum value "disallowed"
	ZappNvmeComponentsTieringControlDisallowed string = "disallowed"
)

// prop value enum
func (m *ZappNvmeComponentsTiering) validateControlEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, zappNvmeComponentsTieringTypeControlPropEnum, true); err != nil {
		return err
	}
	return nil
}

func (m *ZappNvmeComponentsTiering) validateControl(formats strfmt.Registry) error {
	if swag.IsZero(m.Control) { // not required
		return nil
	}

	// value enum
	if err := m.validateControlEnum("control", "body", *m.Control); err != nil {
		return err
	}

	return nil
}

var zappNvmeComponentsTieringTypePolicyPropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["all","auto","none","snapshot_only"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		zappNvmeComponentsTieringTypePolicyPropEnum = append(zappNvmeComponentsTieringTypePolicyPropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// zapp_nvme_components_tiering
	// ZappNvmeComponentsTiering
	// policy
	// Policy
	// all
	// END DEBUGGING
	// ZappNvmeComponentsTieringPolicyAll captures enum value "all"
	ZappNvmeComponentsTieringPolicyAll string = "all"

	// BEGIN DEBUGGING
	// zapp_nvme_components_tiering
	// ZappNvmeComponentsTiering
	// policy
	// Policy
	// auto
	// END DEBUGGING
	// ZappNvmeComponentsTieringPolicyAuto captures enum value "auto"
	ZappNvmeComponentsTieringPolicyAuto string = "auto"

	// BEGIN DEBUGGING
	// zapp_nvme_components_tiering
	// ZappNvmeComponentsTiering
	// policy
	// Policy
	// none
	// END DEBUGGING
	// ZappNvmeComponentsTieringPolicyNone captures enum value "none"
	ZappNvmeComponentsTieringPolicyNone string = "none"

	// BEGIN DEBUGGING
	// zapp_nvme_components_tiering
	// ZappNvmeComponentsTiering
	// policy
	// Policy
	// snapshot_only
	// END DEBUGGING
	// ZappNvmeComponentsTieringPolicySnapshotOnly captures enum value "snapshot_only"
	ZappNvmeComponentsTieringPolicySnapshotOnly string = "snapshot_only"
)

// prop value enum
func (m *ZappNvmeComponentsTiering) validatePolicyEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, zappNvmeComponentsTieringTypePolicyPropEnum, true); err != nil {
		return err
	}
	return nil
}

func (m *ZappNvmeComponentsTiering) validatePolicy(formats strfmt.Registry) error {
	if swag.IsZero(m.Policy) { // not required
		return nil
	}

	// value enum
	if err := m.validatePolicyEnum("policy", "body", *m.Policy); err != nil {
		return err
	}

	return nil
}

func (m *ZappNvmeComponentsTiering) validateZappNvmeComponentsTieringInlineObjectStores(formats strfmt.Registry) error {
	if swag.IsZero(m.ZappNvmeComponentsTieringInlineObjectStores) { // not required
		return nil
	}

	for i := 0; i < len(m.ZappNvmeComponentsTieringInlineObjectStores); i++ {
		if swag.IsZero(m.ZappNvmeComponentsTieringInlineObjectStores[i]) { // not required
			continue
		}

		if m.ZappNvmeComponentsTieringInlineObjectStores[i] != nil {
			if err := m.ZappNvmeComponentsTieringInlineObjectStores[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("object_stores" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this zapp nvme components tiering based on the context it is used
func (m *ZappNvmeComponentsTiering) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateZappNvmeComponentsTieringInlineObjectStores(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *ZappNvmeComponentsTiering) contextValidateZappNvmeComponentsTieringInlineObjectStores(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(m.ZappNvmeComponentsTieringInlineObjectStores); i++ {

		if m.ZappNvmeComponentsTieringInlineObjectStores[i] != nil {
			if err := m.ZappNvmeComponentsTieringInlineObjectStores[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("object_stores" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// MarshalBinary interface implementation
func (m *ZappNvmeComponentsTiering) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *ZappNvmeComponentsTiering) UnmarshalBinary(b []byte) error {
	var res ZappNvmeComponentsTiering
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// ZappNvmeComponentsTieringInlineObjectStoresInlineArrayItem zapp nvme components tiering inline object stores inline array item
//
// swagger:model zapp_nvme_components_tiering_inline_object_stores_inline_array_item
type ZappNvmeComponentsTieringInlineObjectStoresInlineArrayItem struct {

	// The name of the object-store to use.
	// Max Length: 512
	// Min Length: 1
	Name *string `json:"name,omitempty"`
}

// Validate validates this zapp nvme components tiering inline object stores inline array item
func (m *ZappNvmeComponentsTieringInlineObjectStoresInlineArrayItem) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateName(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *ZappNvmeComponentsTieringInlineObjectStoresInlineArrayItem) validateName(formats strfmt.Registry) error {
	if swag.IsZero(m.Name) { // not required
		return nil
	}

	if err := validate.MinLength("name", "body", *m.Name, 1); err != nil {
		return err
	}

	if err := validate.MaxLength("name", "body", *m.Name, 512); err != nil {
		return err
	}

	return nil
}

// ContextValidate validates this zapp nvme components tiering inline object stores inline array item based on context it is used
func (m *ZappNvmeComponentsTieringInlineObjectStoresInlineArrayItem) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	return nil
}

// MarshalBinary interface implementation
func (m *ZappNvmeComponentsTieringInlineObjectStoresInlineArrayItem) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *ZappNvmeComponentsTieringInlineObjectStoresInlineArrayItem) UnmarshalBinary(b []byte) error {
	var res ZappNvmeComponentsTieringInlineObjectStoresInlineArrayItem
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
