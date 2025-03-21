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

// StorageUnitEncryption storage unit encryption
//
// swagger:model storage_unit_encryption
type StorageUnitEncryption struct {

	// Storage unit data encryption state.<br>_unencrypted_ &dash; Unencrypted.<br>_software_encrypted_ &dash; Software encryption enabled.<br>_software_conversion_queued_ &dash; Queued for software conversion.<br>_software_encrypting_ &dash; Software encryption is in progress.<br>_software_rekeying_ &dash; Encryption with a new key is in progress.<br>_software_conversion_paused_ &dash; Software conversion is paused.<br>_software_rekey_paused_ &dash; Encryption with a new key is paused.<br>_software_rekey_queued_ &dash; Queued for software rekey.
	//
	// Read Only: true
	// Enum: ["unencrypted","software_encrypted","software_conversion_queued","software_encrypting","software_rekeying","software_conversion_paused","software_rekey_paused","software_rekey_queued"]
	State *string `json:"state,omitempty"`
}

// Validate validates this storage unit encryption
func (m *StorageUnitEncryption) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateState(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

var storageUnitEncryptionTypeStatePropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["unencrypted","software_encrypted","software_conversion_queued","software_encrypting","software_rekeying","software_conversion_paused","software_rekey_paused","software_rekey_queued"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		storageUnitEncryptionTypeStatePropEnum = append(storageUnitEncryptionTypeStatePropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// storage_unit_encryption
	// StorageUnitEncryption
	// state
	// State
	// unencrypted
	// END DEBUGGING
	// StorageUnitEncryptionStateUnencrypted captures enum value "unencrypted"
	StorageUnitEncryptionStateUnencrypted string = "unencrypted"

	// BEGIN DEBUGGING
	// storage_unit_encryption
	// StorageUnitEncryption
	// state
	// State
	// software_encrypted
	// END DEBUGGING
	// StorageUnitEncryptionStateSoftwareEncrypted captures enum value "software_encrypted"
	StorageUnitEncryptionStateSoftwareEncrypted string = "software_encrypted"

	// BEGIN DEBUGGING
	// storage_unit_encryption
	// StorageUnitEncryption
	// state
	// State
	// software_conversion_queued
	// END DEBUGGING
	// StorageUnitEncryptionStateSoftwareConversionQueued captures enum value "software_conversion_queued"
	StorageUnitEncryptionStateSoftwareConversionQueued string = "software_conversion_queued"

	// BEGIN DEBUGGING
	// storage_unit_encryption
	// StorageUnitEncryption
	// state
	// State
	// software_encrypting
	// END DEBUGGING
	// StorageUnitEncryptionStateSoftwareEncrypting captures enum value "software_encrypting"
	StorageUnitEncryptionStateSoftwareEncrypting string = "software_encrypting"

	// BEGIN DEBUGGING
	// storage_unit_encryption
	// StorageUnitEncryption
	// state
	// State
	// software_rekeying
	// END DEBUGGING
	// StorageUnitEncryptionStateSoftwareRekeying captures enum value "software_rekeying"
	StorageUnitEncryptionStateSoftwareRekeying string = "software_rekeying"

	// BEGIN DEBUGGING
	// storage_unit_encryption
	// StorageUnitEncryption
	// state
	// State
	// software_conversion_paused
	// END DEBUGGING
	// StorageUnitEncryptionStateSoftwareConversionPaused captures enum value "software_conversion_paused"
	StorageUnitEncryptionStateSoftwareConversionPaused string = "software_conversion_paused"

	// BEGIN DEBUGGING
	// storage_unit_encryption
	// StorageUnitEncryption
	// state
	// State
	// software_rekey_paused
	// END DEBUGGING
	// StorageUnitEncryptionStateSoftwareRekeyPaused captures enum value "software_rekey_paused"
	StorageUnitEncryptionStateSoftwareRekeyPaused string = "software_rekey_paused"

	// BEGIN DEBUGGING
	// storage_unit_encryption
	// StorageUnitEncryption
	// state
	// State
	// software_rekey_queued
	// END DEBUGGING
	// StorageUnitEncryptionStateSoftwareRekeyQueued captures enum value "software_rekey_queued"
	StorageUnitEncryptionStateSoftwareRekeyQueued string = "software_rekey_queued"
)

// prop value enum
func (m *StorageUnitEncryption) validateStateEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, storageUnitEncryptionTypeStatePropEnum, true); err != nil {
		return err
	}
	return nil
}

func (m *StorageUnitEncryption) validateState(formats strfmt.Registry) error {
	if swag.IsZero(m.State) { // not required
		return nil
	}

	// value enum
	if err := m.validateStateEnum("state", "body", *m.State); err != nil {
		return err
	}

	return nil
}

// ContextValidate validate this storage unit encryption based on the context it is used
func (m *StorageUnitEncryption) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateState(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *StorageUnitEncryption) contextValidateState(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "state", "body", m.State); err != nil {
		return err
	}

	return nil
}

// MarshalBinary interface implementation
func (m *StorageUnitEncryption) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *StorageUnitEncryption) UnmarshalBinary(b []byte) error {
	var res StorageUnitEncryption
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
