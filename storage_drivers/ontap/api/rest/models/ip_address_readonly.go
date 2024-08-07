// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"

	"github.com/go-openapi/strfmt"
)

// IPAddressReadonly IPv4 or IPv6 address
// Example: 10.10.10.7
//
// swagger:model ip_address_readonly
type IPAddressReadonly string

// Validate validates this ip address readonly
func (m IPAddressReadonly) Validate(formats strfmt.Registry) error {
	return nil
}

// ContextValidate validates this ip address readonly based on context it is used
func (m IPAddressReadonly) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	return nil
}
