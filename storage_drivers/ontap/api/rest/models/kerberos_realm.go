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

// KerberosRealm kerberos realm
//
// swagger:model kerberos_realm
type KerberosRealm struct {

	// links
	Links *KerberosRealmInlineLinks `json:"_links,omitempty"`

	// ad server
	AdServer *KerberosRealmInlineAdServer `json:"ad_server,omitempty"`

	// Comment
	Comment *string `json:"comment,omitempty"`

	// encryption types
	// Read Only: true
	EncryptionTypes []*string `json:"encryption_types,omitempty"`

	// kdc
	Kdc *KerberosRealmInlineKdc `json:"kdc,omitempty"`

	// Kerberos realm
	Name *string `json:"name,omitempty"`

	// svm
	Svm *KerberosRealmInlineSvm `json:"svm,omitempty"`
}

// Validate validates this kerberos realm
func (m *KerberosRealm) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateAdServer(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateEncryptionTypes(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateKdc(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateSvm(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *KerberosRealm) validateLinks(formats strfmt.Registry) error {
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

func (m *KerberosRealm) validateAdServer(formats strfmt.Registry) error {
	if swag.IsZero(m.AdServer) { // not required
		return nil
	}

	if m.AdServer != nil {
		if err := m.AdServer.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("ad_server")
			}
			return err
		}
	}

	return nil
}

var kerberosRealmEncryptionTypesItemsEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["des","des3","aes_128","aes_256"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		kerberosRealmEncryptionTypesItemsEnum = append(kerberosRealmEncryptionTypesItemsEnum, v)
	}
}

func (m *KerberosRealm) validateEncryptionTypesItemsEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, kerberosRealmEncryptionTypesItemsEnum, true); err != nil {
		return err
	}
	return nil
}

func (m *KerberosRealm) validateEncryptionTypes(formats strfmt.Registry) error {
	if swag.IsZero(m.EncryptionTypes) { // not required
		return nil
	}

	for i := 0; i < len(m.EncryptionTypes); i++ {
		if swag.IsZero(m.EncryptionTypes[i]) { // not required
			continue
		}

		// value enum
		if err := m.validateEncryptionTypesItemsEnum("encryption_types"+"."+strconv.Itoa(i), "body", *m.EncryptionTypes[i]); err != nil {
			return err
		}

	}

	return nil
}

func (m *KerberosRealm) validateKdc(formats strfmt.Registry) error {
	if swag.IsZero(m.Kdc) { // not required
		return nil
	}

	if m.Kdc != nil {
		if err := m.Kdc.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("kdc")
			}
			return err
		}
	}

	return nil
}

func (m *KerberosRealm) validateSvm(formats strfmt.Registry) error {
	if swag.IsZero(m.Svm) { // not required
		return nil
	}

	if m.Svm != nil {
		if err := m.Svm.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("svm")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this kerberos realm based on the context it is used
func (m *KerberosRealm) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateAdServer(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateEncryptionTypes(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateKdc(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateSvm(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *KerberosRealm) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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

func (m *KerberosRealm) contextValidateAdServer(ctx context.Context, formats strfmt.Registry) error {

	if m.AdServer != nil {
		if err := m.AdServer.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("ad_server")
			}
			return err
		}
	}

	return nil
}

func (m *KerberosRealm) contextValidateEncryptionTypes(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "encryption_types", "body", []*string(m.EncryptionTypes)); err != nil {
		return err
	}

	for i := 0; i < len(m.EncryptionTypes); i++ {

		if err := validate.ReadOnly(ctx, "encryption_types"+"."+strconv.Itoa(i), "body", m.EncryptionTypes[i]); err != nil {
			return err
		}

	}

	return nil
}

func (m *KerberosRealm) contextValidateKdc(ctx context.Context, formats strfmt.Registry) error {

	if m.Kdc != nil {
		if err := m.Kdc.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("kdc")
			}
			return err
		}
	}

	return nil
}

func (m *KerberosRealm) contextValidateSvm(ctx context.Context, formats strfmt.Registry) error {

	if m.Svm != nil {
		if err := m.Svm.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("svm")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (m *KerberosRealm) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *KerberosRealm) UnmarshalBinary(b []byte) error {
	var res KerberosRealm
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// KerberosRealmInlineAdServer kerberos realm inline ad server
//
// swagger:model kerberos_realm_inline_ad_server
type KerberosRealmInlineAdServer struct {

	// Active Directory server IP address
	// Example: 1.2.3.4
	Address *string `json:"address,omitempty"`

	// Active Directory server name
	Name *string `json:"name,omitempty"`
}

// Validate validates this kerberos realm inline ad server
func (m *KerberosRealmInlineAdServer) Validate(formats strfmt.Registry) error {
	return nil
}

// ContextValidate validates this kerberos realm inline ad server based on context it is used
func (m *KerberosRealmInlineAdServer) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	return nil
}

// MarshalBinary interface implementation
func (m *KerberosRealmInlineAdServer) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *KerberosRealmInlineAdServer) UnmarshalBinary(b []byte) error {
	var res KerberosRealmInlineAdServer
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// KerberosRealmInlineKdc kerberos realm inline kdc
//
// swagger:model kerberos_realm_inline_kdc
type KerberosRealmInlineKdc struct {

	// KDC IP address
	// Example: 1.2.3.4
	IP *string `json:"ip,omitempty"`

	// KDC port
	// Example: 88
	// Maximum: 65535
	// Minimum: 1
	Port *int64 `json:"port,omitempty"`

	// Key Distribution Center (KDC) vendor. Following values are suported:
	// * microsoft - Microsoft Active Directory KDC
	// * other - MIT Kerberos KDC or other KDC
	//
	// Enum: [microsoft other]
	Vendor *string `json:"vendor,omitempty"`
}

// Validate validates this kerberos realm inline kdc
func (m *KerberosRealmInlineKdc) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validatePort(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateVendor(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *KerberosRealmInlineKdc) validatePort(formats strfmt.Registry) error {
	if swag.IsZero(m.Port) { // not required
		return nil
	}

	if err := validate.MinimumInt("kdc"+"."+"port", "body", *m.Port, 1, false); err != nil {
		return err
	}

	if err := validate.MaximumInt("kdc"+"."+"port", "body", *m.Port, 65535, false); err != nil {
		return err
	}

	return nil
}

var kerberosRealmInlineKdcTypeVendorPropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["microsoft","other"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		kerberosRealmInlineKdcTypeVendorPropEnum = append(kerberosRealmInlineKdcTypeVendorPropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// kerberos_realm_inline_kdc
	// KerberosRealmInlineKdc
	// vendor
	// Vendor
	// microsoft
	// END DEBUGGING
	// KerberosRealmInlineKdcVendorMicrosoft captures enum value "microsoft"
	KerberosRealmInlineKdcVendorMicrosoft string = "microsoft"

	// BEGIN DEBUGGING
	// kerberos_realm_inline_kdc
	// KerberosRealmInlineKdc
	// vendor
	// Vendor
	// other
	// END DEBUGGING
	// KerberosRealmInlineKdcVendorOther captures enum value "other"
	KerberosRealmInlineKdcVendorOther string = "other"
)

// prop value enum
func (m *KerberosRealmInlineKdc) validateVendorEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, kerberosRealmInlineKdcTypeVendorPropEnum, true); err != nil {
		return err
	}
	return nil
}

func (m *KerberosRealmInlineKdc) validateVendor(formats strfmt.Registry) error {
	if swag.IsZero(m.Vendor) { // not required
		return nil
	}

	// value enum
	if err := m.validateVendorEnum("kdc"+"."+"vendor", "body", *m.Vendor); err != nil {
		return err
	}

	return nil
}

// ContextValidate validates this kerberos realm inline kdc based on context it is used
func (m *KerberosRealmInlineKdc) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	return nil
}

// MarshalBinary interface implementation
func (m *KerberosRealmInlineKdc) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *KerberosRealmInlineKdc) UnmarshalBinary(b []byte) error {
	var res KerberosRealmInlineKdc
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// KerberosRealmInlineLinks kerberos realm inline links
//
// swagger:model kerberos_realm_inline__links
type KerberosRealmInlineLinks struct {

	// self
	Self *Href `json:"self,omitempty"`
}

// Validate validates this kerberos realm inline links
func (m *KerberosRealmInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *KerberosRealmInlineLinks) validateSelf(formats strfmt.Registry) error {
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

// ContextValidate validate this kerberos realm inline links based on the context it is used
func (m *KerberosRealmInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *KerberosRealmInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

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
func (m *KerberosRealmInlineLinks) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *KerberosRealmInlineLinks) UnmarshalBinary(b []byte) error {
	var res KerberosRealmInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// KerberosRealmInlineSvm kerberos realm inline svm
//
// swagger:model kerberos_realm_inline_svm
type KerberosRealmInlineSvm struct {

	// links
	Links *KerberosRealmInlineSvmInlineLinks `json:"_links,omitempty"`

	// The name of the SVM.
	//
	// Example: svm1
	Name *string `json:"name,omitempty"`

	// The unique identifier of the SVM.
	//
	// Example: 02c9e252-41be-11e9-81d5-00a0986138f7
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this kerberos realm inline svm
func (m *KerberosRealmInlineSvm) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *KerberosRealmInlineSvm) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(m.Links) { // not required
		return nil
	}

	if m.Links != nil {
		if err := m.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("svm" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this kerberos realm inline svm based on the context it is used
func (m *KerberosRealmInlineSvm) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *KerberosRealmInlineSvm) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if m.Links != nil {
		if err := m.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("svm" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (m *KerberosRealmInlineSvm) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *KerberosRealmInlineSvm) UnmarshalBinary(b []byte) error {
	var res KerberosRealmInlineSvm
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// KerberosRealmInlineSvmInlineLinks kerberos realm inline svm inline links
//
// swagger:model kerberos_realm_inline_svm_inline__links
type KerberosRealmInlineSvmInlineLinks struct {

	// self
	Self *Href `json:"self,omitempty"`
}

// Validate validates this kerberos realm inline svm inline links
func (m *KerberosRealmInlineSvmInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *KerberosRealmInlineSvmInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(m.Self) { // not required
		return nil
	}

	if m.Self != nil {
		if err := m.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("svm" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this kerberos realm inline svm inline links based on the context it is used
func (m *KerberosRealmInlineSvmInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *KerberosRealmInlineSvmInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if m.Self != nil {
		if err := m.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("svm" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (m *KerberosRealmInlineSvmInlineLinks) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *KerberosRealmInlineSvmInlineLinks) UnmarshalBinary(b []byte) error {
	var res KerberosRealmInlineSvmInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
