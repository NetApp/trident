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

// HostRecord host record
//
// swagger:model host_record
type HostRecord struct {

	// Canonical name of the host.
	//
	// Example: localhost
	// Read Only: true
	CanonicalName *string `json:"canonical_name,omitempty"`

	// IP address or hostname.
	//
	// Example: localhost
	Host *string `json:"host,omitempty"`

	// List of IPv4 addresses.
	//
	// Example: ["127.0.0.1"]
	// Read Only: true
	HostRecordInlineIPV4Addresses []*string `json:"ipv4_addresses,omitempty"`

	// List of IPv6 addresses.
	//
	// Example: ["::1"]
	// Read Only: true
	HostRecordInlineIPV6Addresses []*string `json:"ipv6_addresses,omitempty"`

	// Hostname.
	//
	// Example: localhost
	// Read Only: true
	Hostname *string `json:"hostname,omitempty"`

	// Source used for lookup.
	//
	// Example: Files
	Source *string `json:"source,omitempty"`

	// svm
	Svm *HostRecordInlineSvm `json:"svm,omitempty"`
}

// Validate validates this host record
func (m *HostRecord) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateSvm(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *HostRecord) validateSvm(formats strfmt.Registry) error {
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

// ContextValidate validate this host record based on the context it is used
func (m *HostRecord) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateCanonicalName(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateHostRecordInlineIPV4Addresses(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateHostRecordInlineIPV6Addresses(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateHostname(ctx, formats); err != nil {
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

func (m *HostRecord) contextValidateCanonicalName(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "canonical_name", "body", m.CanonicalName); err != nil {
		return err
	}

	return nil
}

func (m *HostRecord) contextValidateHostRecordInlineIPV4Addresses(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "ipv4_addresses", "body", []*string(m.HostRecordInlineIPV4Addresses)); err != nil {
		return err
	}

	return nil
}

func (m *HostRecord) contextValidateHostRecordInlineIPV6Addresses(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "ipv6_addresses", "body", []*string(m.HostRecordInlineIPV6Addresses)); err != nil {
		return err
	}

	return nil
}

func (m *HostRecord) contextValidateHostname(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "hostname", "body", m.Hostname); err != nil {
		return err
	}

	return nil
}

func (m *HostRecord) contextValidateSvm(ctx context.Context, formats strfmt.Registry) error {

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
func (m *HostRecord) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *HostRecord) UnmarshalBinary(b []byte) error {
	var res HostRecord
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// HostRecordInlineSvm SVM, applies only to SVM-scoped objects.
//
// swagger:model host_record_inline_svm
type HostRecordInlineSvm struct {

	// links
	Links *HostRecordInlineSvmInlineLinks `json:"_links,omitempty"`

	// The name of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: svm1
	Name *string `json:"name,omitempty"`

	// The unique identifier of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: 02c9e252-41be-11e9-81d5-00a0986138f7
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this host record inline svm
func (m *HostRecordInlineSvm) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *HostRecordInlineSvm) validateLinks(formats strfmt.Registry) error {
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

// ContextValidate validate this host record inline svm based on the context it is used
func (m *HostRecordInlineSvm) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *HostRecordInlineSvm) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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
func (m *HostRecordInlineSvm) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *HostRecordInlineSvm) UnmarshalBinary(b []byte) error {
	var res HostRecordInlineSvm
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// HostRecordInlineSvmInlineLinks host record inline svm inline links
//
// swagger:model host_record_inline_svm_inline__links
type HostRecordInlineSvmInlineLinks struct {

	// self
	Self *Href `json:"self,omitempty"`
}

// Validate validates this host record inline svm inline links
func (m *HostRecordInlineSvmInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *HostRecordInlineSvmInlineLinks) validateSelf(formats strfmt.Registry) error {
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

// ContextValidate validate this host record inline svm inline links based on the context it is used
func (m *HostRecordInlineSvmInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *HostRecordInlineSvmInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

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
func (m *HostRecordInlineSvmInlineLinks) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *HostRecordInlineSvmInlineLinks) UnmarshalBinary(b []byte) error {
	var res HostRecordInlineSvmInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
