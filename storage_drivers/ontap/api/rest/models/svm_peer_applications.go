// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"encoding/json"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/validate"
)

// SvmPeerApplications Applications for an SVM peer relationship.
//
// swagger:model svm_peer_applications
type SvmPeerApplications string

func NewSvmPeerApplications(value SvmPeerApplications) *SvmPeerApplications {
	return &value
}

// Pointer returns a pointer to a freshly-allocated SvmPeerApplications.
func (m SvmPeerApplications) Pointer() *SvmPeerApplications {
	return &m
}

const (

	// SvmPeerApplicationsSnapmirror captures enum value "snapmirror"
	SvmPeerApplicationsSnapmirror SvmPeerApplications = "snapmirror"

	// SvmPeerApplicationsFileCopy captures enum value "file_copy"
	SvmPeerApplicationsFileCopy SvmPeerApplications = "file_copy"

	// SvmPeerApplicationsLunCopy captures enum value "lun_copy"
	SvmPeerApplicationsLunCopy SvmPeerApplications = "lun_copy"

	// SvmPeerApplicationsFlexcache captures enum value "flexcache"
	SvmPeerApplicationsFlexcache SvmPeerApplications = "flexcache"
)

// for schema
var svmPeerApplicationsEnum []interface{}

func init() {
	var res []SvmPeerApplications
	if err := json.Unmarshal([]byte(`["snapmirror","file_copy","lun_copy","flexcache"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		svmPeerApplicationsEnum = append(svmPeerApplicationsEnum, v)
	}
}

func (m SvmPeerApplications) validateSvmPeerApplicationsEnum(path, location string, value SvmPeerApplications) error {
	if err := validate.EnumCase(path, location, value, svmPeerApplicationsEnum, true); err != nil {
		return err
	}
	return nil
}

// Validate validates this svm peer applications
func (m SvmPeerApplications) Validate(formats strfmt.Registry) error {
	var res []error

	// value enum
	if err := m.validateSvmPeerApplicationsEnum("", "body", m); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// ContextValidate validates this svm peer applications based on context it is used
func (m SvmPeerApplications) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	return nil
}
