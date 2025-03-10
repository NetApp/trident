// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/validate"
)

// LocalCifsGroupSid The security ID of the local group which uniquely identifies the group. The group SID is automatically generated in POST and it is retrieved using the GET method.
//
// Example: S-1-5-21-256008430-3394229847-3930036330-1001
//
// swagger:model local_cifs_group_sid
type LocalCifsGroupSid string

// Validate validates this local cifs group sid
func (m LocalCifsGroupSid) Validate(formats strfmt.Registry) error {
	return nil
}

// ContextValidate validate this local cifs group sid based on the context it is used
func (m LocalCifsGroupSid) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := validate.ReadOnly(ctx, "", "body", LocalCifsGroupSid(m)); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
