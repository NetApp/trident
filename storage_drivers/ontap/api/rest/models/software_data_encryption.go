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

// SoftwareDataEncryption Cluster-wide software data encryption related information.
//
// swagger:model software_data_encryption
type SoftwareDataEncryption struct {

	// Indicates whether or not software encryption conversion is enabled on the cluster. A PATCH request initiates the conversion of all non-encrypted metadata volumes in the cluster to encrypted metadata volumes and all non-NAE aggregates to NAE aggregates. For the PATCH request to start, the cluster must have either an Onboard or an external key manager set up and the aggregates should either be empty or have only metadata volumes. No data volumes should be present in any of the aggregates in the cluster. For MetroCluster configurations, a PATCH request enables conversion on all the aggregates and metadata volumes of both local and remote clusters and is not allowed when the MetroCluster is in switchover state.
	ConversionEnabled *bool `json:"conversion_enabled,omitempty"`

	// Indicates whether or not default software data at rest encryption is disabled on the cluster.
	DisabledByDefault *bool `json:"disabled_by_default,omitempty"`

	// Software data encryption state.<br>encrypted &dash; All the volumes are encrypted.<br>encrypting &dash; Encryption conversion operation is in progress.<br>partial &dash; Some volumes are encrypted, and others remains in plain text.<br>rekeying &dash; All volumes are currently being encrypted with a new key.<br>unencrypted &dash; None of the volumes are encrypted.<br>conversion_paused &dash; Encryption conversion operation is paused on one or more volumes.<br>rekey_paused &dash; Encryption rekey operation is paused on one or more volumes.
	// Read Only: true
	// Enum: ["encrypted","encrypting","partial","rekeying","unencrypted","conversion_paused","rekey_paused"]
	EncryptionState *string `json:"encryption_state,omitempty"`

	// rekey
	Rekey *bool `json:"rekey,omitempty"`
}

// Validate validates this software data encryption
func (m *SoftwareDataEncryption) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateEncryptionState(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

var softwareDataEncryptionTypeEncryptionStatePropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["encrypted","encrypting","partial","rekeying","unencrypted","conversion_paused","rekey_paused"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		softwareDataEncryptionTypeEncryptionStatePropEnum = append(softwareDataEncryptionTypeEncryptionStatePropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// software_data_encryption
	// SoftwareDataEncryption
	// encryption_state
	// EncryptionState
	// encrypted
	// END DEBUGGING
	// SoftwareDataEncryptionEncryptionStateEncrypted captures enum value "encrypted"
	SoftwareDataEncryptionEncryptionStateEncrypted string = "encrypted"

	// BEGIN DEBUGGING
	// software_data_encryption
	// SoftwareDataEncryption
	// encryption_state
	// EncryptionState
	// encrypting
	// END DEBUGGING
	// SoftwareDataEncryptionEncryptionStateEncrypting captures enum value "encrypting"
	SoftwareDataEncryptionEncryptionStateEncrypting string = "encrypting"

	// BEGIN DEBUGGING
	// software_data_encryption
	// SoftwareDataEncryption
	// encryption_state
	// EncryptionState
	// partial
	// END DEBUGGING
	// SoftwareDataEncryptionEncryptionStatePartial captures enum value "partial"
	SoftwareDataEncryptionEncryptionStatePartial string = "partial"

	// BEGIN DEBUGGING
	// software_data_encryption
	// SoftwareDataEncryption
	// encryption_state
	// EncryptionState
	// rekeying
	// END DEBUGGING
	// SoftwareDataEncryptionEncryptionStateRekeying captures enum value "rekeying"
	SoftwareDataEncryptionEncryptionStateRekeying string = "rekeying"

	// BEGIN DEBUGGING
	// software_data_encryption
	// SoftwareDataEncryption
	// encryption_state
	// EncryptionState
	// unencrypted
	// END DEBUGGING
	// SoftwareDataEncryptionEncryptionStateUnencrypted captures enum value "unencrypted"
	SoftwareDataEncryptionEncryptionStateUnencrypted string = "unencrypted"

	// BEGIN DEBUGGING
	// software_data_encryption
	// SoftwareDataEncryption
	// encryption_state
	// EncryptionState
	// conversion_paused
	// END DEBUGGING
	// SoftwareDataEncryptionEncryptionStateConversionPaused captures enum value "conversion_paused"
	SoftwareDataEncryptionEncryptionStateConversionPaused string = "conversion_paused"

	// BEGIN DEBUGGING
	// software_data_encryption
	// SoftwareDataEncryption
	// encryption_state
	// EncryptionState
	// rekey_paused
	// END DEBUGGING
	// SoftwareDataEncryptionEncryptionStateRekeyPaused captures enum value "rekey_paused"
	SoftwareDataEncryptionEncryptionStateRekeyPaused string = "rekey_paused"
)

// prop value enum
func (m *SoftwareDataEncryption) validateEncryptionStateEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, softwareDataEncryptionTypeEncryptionStatePropEnum, true); err != nil {
		return err
	}
	return nil
}

func (m *SoftwareDataEncryption) validateEncryptionState(formats strfmt.Registry) error {
	if swag.IsZero(m.EncryptionState) { // not required
		return nil
	}

	// value enum
	if err := m.validateEncryptionStateEnum("encryption_state", "body", *m.EncryptionState); err != nil {
		return err
	}

	return nil
}

// ContextValidate validate this software data encryption based on the context it is used
func (m *SoftwareDataEncryption) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateEncryptionState(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *SoftwareDataEncryption) contextValidateEncryptionState(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "encryption_state", "body", m.EncryptionState); err != nil {
		return err
	}

	return nil
}

// MarshalBinary interface implementation
func (m *SoftwareDataEncryption) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *SoftwareDataEncryption) UnmarshalBinary(b []byte) error {
	var res SoftwareDataEncryption
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
