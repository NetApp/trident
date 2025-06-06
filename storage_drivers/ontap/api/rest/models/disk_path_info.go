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

// DiskPathInfo disk path info
//
// swagger:model disk_path_info
type DiskPathInfo struct {

	// Path-based disk name.
	// Example: vsim4:3a.10
	DiskPathName *string `json:"disk_path_name,omitempty"`

	// Initiator port.
	// Example: 3a
	Initiator *string `json:"initiator,omitempty"`

	// Controller with the initiator port for this path.
	// Example: vsim4
	NodeName *string `json:"node.name,omitempty"`

	// Controller UUID, to identify node for this path.
	// Example: cf7fe057-526d-11ec-af4e-0050568e9df0
	NodeUUID *string `json:"node.uuid,omitempty"`

	// Name of the disk port.
	// Example: A
	PortName *string `json:"port_name,omitempty"`

	// Disk port type.
	// Example: sas
	// Enum: ["sas","fc","nvme"]
	PortType *string `json:"port_type,omitempty"`

	// Virtual disk hypervisor file name.
	// Example: xvds vol0a0567ae156ca59f6
	VmdiskHypervisorFileName *string `json:"vmdisk_hypervisor_file_name,omitempty"`

	// Target device's World Wide Node Name.
	// Example: 5000c2971c1b2b8c
	Wwnn *string `json:"wwnn,omitempty"`

	// Target device's World Wide Port Name.
	// Example: 5000c2971c1b2b8d
	Wwpn *string `json:"wwpn,omitempty"`
}

// Validate validates this disk path info
func (m *DiskPathInfo) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validatePortType(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

var diskPathInfoTypePortTypePropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["sas","fc","nvme"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		diskPathInfoTypePortTypePropEnum = append(diskPathInfoTypePortTypePropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// disk_path_info
	// DiskPathInfo
	// port_type
	// PortType
	// sas
	// END DEBUGGING
	// DiskPathInfoPortTypeSas captures enum value "sas"
	DiskPathInfoPortTypeSas string = "sas"

	// BEGIN DEBUGGING
	// disk_path_info
	// DiskPathInfo
	// port_type
	// PortType
	// fc
	// END DEBUGGING
	// DiskPathInfoPortTypeFc captures enum value "fc"
	DiskPathInfoPortTypeFc string = "fc"

	// BEGIN DEBUGGING
	// disk_path_info
	// DiskPathInfo
	// port_type
	// PortType
	// nvme
	// END DEBUGGING
	// DiskPathInfoPortTypeNvme captures enum value "nvme"
	DiskPathInfoPortTypeNvme string = "nvme"
)

// prop value enum
func (m *DiskPathInfo) validatePortTypeEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, diskPathInfoTypePortTypePropEnum, true); err != nil {
		return err
	}
	return nil
}

func (m *DiskPathInfo) validatePortType(formats strfmt.Registry) error {
	if swag.IsZero(m.PortType) { // not required
		return nil
	}

	// value enum
	if err := m.validatePortTypeEnum("port_type", "body", *m.PortType); err != nil {
		return err
	}

	return nil
}

// ContextValidate validates this disk path info based on context it is used
func (m *DiskPathInfo) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	return nil
}

// MarshalBinary interface implementation
func (m *DiskPathInfo) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *DiskPathInfo) UnmarshalBinary(b []byte) error {
	var res DiskPathInfo
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
