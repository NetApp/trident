// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"strconv"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/go-openapi/validate"
)

// TopMetricsSvmFileResponse top metrics svm file response
//
// swagger:model top_metrics_svm_file_response
type TopMetricsSvmFileResponse struct {

	// links
	Links *TopMetricsSvmFileResponseInlineLinks `json:"_links,omitempty"`

	// notice
	Notice *TopMetricsSvmFileResponseInlineNotice `json:"notice,omitempty"`

	// Number of records.
	// Example: 1
	NumRecords *int64 `json:"num_records,omitempty"`

	// List of volumes that are not included in the SVM activity tracking REST API.
	TopMetricsSvmFileResponseInlineExcludedVolumes []*TopMetricsSvmFileExcludedVolume `json:"excluded_volumes,omitempty"`

	// top metrics svm file response inline records
	TopMetricsSvmFileResponseInlineRecords []*TopMetricsSvmFile `json:"records,omitempty"`
}

// Validate validates this top metrics svm file response
func (m *TopMetricsSvmFileResponse) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateNotice(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateTopMetricsSvmFileResponseInlineExcludedVolumes(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateTopMetricsSvmFileResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *TopMetricsSvmFileResponse) validateLinks(formats strfmt.Registry) error {
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

func (m *TopMetricsSvmFileResponse) validateNotice(formats strfmt.Registry) error {
	if swag.IsZero(m.Notice) { // not required
		return nil
	}

	if m.Notice != nil {
		if err := m.Notice.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("notice")
			}
			return err
		}
	}

	return nil
}

func (m *TopMetricsSvmFileResponse) validateTopMetricsSvmFileResponseInlineExcludedVolumes(formats strfmt.Registry) error {
	if swag.IsZero(m.TopMetricsSvmFileResponseInlineExcludedVolumes) { // not required
		return nil
	}

	for i := 0; i < len(m.TopMetricsSvmFileResponseInlineExcludedVolumes); i++ {
		if swag.IsZero(m.TopMetricsSvmFileResponseInlineExcludedVolumes[i]) { // not required
			continue
		}

		if m.TopMetricsSvmFileResponseInlineExcludedVolumes[i] != nil {
			if err := m.TopMetricsSvmFileResponseInlineExcludedVolumes[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("excluded_volumes" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (m *TopMetricsSvmFileResponse) validateTopMetricsSvmFileResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(m.TopMetricsSvmFileResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(m.TopMetricsSvmFileResponseInlineRecords); i++ {
		if swag.IsZero(m.TopMetricsSvmFileResponseInlineRecords[i]) { // not required
			continue
		}

		if m.TopMetricsSvmFileResponseInlineRecords[i] != nil {
			if err := m.TopMetricsSvmFileResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this top metrics svm file response based on the context it is used
func (m *TopMetricsSvmFileResponse) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateNotice(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateTopMetricsSvmFileResponseInlineExcludedVolumes(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateTopMetricsSvmFileResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *TopMetricsSvmFileResponse) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

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

func (m *TopMetricsSvmFileResponse) contextValidateNotice(ctx context.Context, formats strfmt.Registry) error {

	if m.Notice != nil {
		if err := m.Notice.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("notice")
			}
			return err
		}
	}

	return nil
}

func (m *TopMetricsSvmFileResponse) contextValidateTopMetricsSvmFileResponseInlineExcludedVolumes(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(m.TopMetricsSvmFileResponseInlineExcludedVolumes); i++ {

		if m.TopMetricsSvmFileResponseInlineExcludedVolumes[i] != nil {
			if err := m.TopMetricsSvmFileResponseInlineExcludedVolumes[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("excluded_volumes" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (m *TopMetricsSvmFileResponse) contextValidateTopMetricsSvmFileResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(m.TopMetricsSvmFileResponseInlineRecords); i++ {

		if m.TopMetricsSvmFileResponseInlineRecords[i] != nil {
			if err := m.TopMetricsSvmFileResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// MarshalBinary interface implementation
func (m *TopMetricsSvmFileResponse) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *TopMetricsSvmFileResponse) UnmarshalBinary(b []byte) error {
	var res TopMetricsSvmFileResponse
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// TopMetricsSvmFileResponseInlineLinks top metrics svm file response inline links
//
// swagger:model top_metrics_svm_file_response_inline__links
type TopMetricsSvmFileResponseInlineLinks struct {

	// next
	Next *Href `json:"next,omitempty"`

	// self
	Self *Href `json:"self,omitempty"`
}

// Validate validates this top metrics svm file response inline links
func (m *TopMetricsSvmFileResponseInlineLinks) Validate(formats strfmt.Registry) error {
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

func (m *TopMetricsSvmFileResponseInlineLinks) validateNext(formats strfmt.Registry) error {
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

func (m *TopMetricsSvmFileResponseInlineLinks) validateSelf(formats strfmt.Registry) error {
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

// ContextValidate validate this top metrics svm file response inline links based on the context it is used
func (m *TopMetricsSvmFileResponseInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
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

func (m *TopMetricsSvmFileResponseInlineLinks) contextValidateNext(ctx context.Context, formats strfmt.Registry) error {

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

func (m *TopMetricsSvmFileResponseInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

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
func (m *TopMetricsSvmFileResponseInlineLinks) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *TopMetricsSvmFileResponseInlineLinks) UnmarshalBinary(b []byte) error {
	var res TopMetricsSvmFileResponseInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// TopMetricsSvmFileResponseInlineNotice Optional field that indicates why no records are returned by the SVM activity tracking REST API.
//
// swagger:model top_metrics_svm_file_response_inline_notice
type TopMetricsSvmFileResponseInlineNotice struct {

	// Warning code indicating why no records are returned.
	// Example: 111411207
	// Read Only: true
	Code *string `json:"code,omitempty"`

	// Details why no records are returned.
	// Example: The volume is offline.
	// Read Only: true
	Message *string `json:"message,omitempty"`
}

// Validate validates this top metrics svm file response inline notice
func (m *TopMetricsSvmFileResponseInlineNotice) Validate(formats strfmt.Registry) error {
	return nil
}

// ContextValidate validate this top metrics svm file response inline notice based on the context it is used
func (m *TopMetricsSvmFileResponseInlineNotice) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateCode(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateMessage(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *TopMetricsSvmFileResponseInlineNotice) contextValidateCode(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "notice"+"."+"code", "body", m.Code); err != nil {
		return err
	}

	return nil
}

func (m *TopMetricsSvmFileResponseInlineNotice) contextValidateMessage(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "notice"+"."+"message", "body", m.Message); err != nil {
		return err
	}

	return nil
}

// MarshalBinary interface implementation
func (m *TopMetricsSvmFileResponseInlineNotice) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *TopMetricsSvmFileResponseInlineNotice) UnmarshalBinary(b []byte) error {
	var res TopMetricsSvmFileResponseInlineNotice
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
