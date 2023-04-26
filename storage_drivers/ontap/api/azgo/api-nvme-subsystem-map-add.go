// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NvmeSubsystemMapAddRequest is a structure to represent a nvme-subsystem-map-add Request ZAPI object
type NvmeSubsystemMapAddRequest struct {
	XMLName      xml.Name     `xml:"nvme-subsystem-map-add"`
	PathPtr      *LunPathType `xml:"path"`
	SubsystemPtr *string      `xml:"subsystem"`
}

// NvmeSubsystemMapAddResponse is a structure to represent a nvme-subsystem-map-add Response ZAPI object
type NvmeSubsystemMapAddResponse struct {
	XMLName         xml.Name                          `xml:"netapp"`
	ResponseVersion string                            `xml:"version,attr"`
	ResponseXmlns   string                            `xml:"xmlns,attr"`
	Result          NvmeSubsystemMapAddResponseResult `xml:"results"`
}

// NewNvmeSubsystemMapAddResponse is a factory method for creating new instances of NvmeSubsystemMapAddResponse objects
func NewNvmeSubsystemMapAddResponse() *NvmeSubsystemMapAddResponse {
	return &NvmeSubsystemMapAddResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemMapAddResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemMapAddResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeSubsystemMapAddResponseResult is a structure to represent a nvme-subsystem-map-add Response Result ZAPI object
type NvmeSubsystemMapAddResponseResult struct {
	XMLName          xml.Name  `xml:"results"`
	ResultStatusAttr string    `xml:"status,attr"`
	ResultReasonAttr string    `xml:"reason,attr"`
	ResultErrnoAttr  string    `xml:"errno,attr"`
	NamespaceUuidPtr *UuidType `xml:"namespace-uuid"`
	NsidPtr          *int      `xml:"nsid"`
}

// NewNvmeSubsystemMapAddRequest is a factory method for creating new instances of NvmeSubsystemMapAddRequest objects
func NewNvmeSubsystemMapAddRequest() *NvmeSubsystemMapAddRequest {
	return &NvmeSubsystemMapAddRequest{}
}

// NewNvmeSubsystemMapAddResponseResult is a factory method for creating new instances of NvmeSubsystemMapAddResponseResult objects
func NewNvmeSubsystemMapAddResponseResult() *NvmeSubsystemMapAddResponseResult {
	return &NvmeSubsystemMapAddResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemMapAddRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemMapAddResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemMapAddRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemMapAddResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemMapAddRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeSubsystemMapAddResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemMapAddRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeSubsystemMapAddResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeSubsystemMapAddRequest", NewNvmeSubsystemMapAddResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeSubsystemMapAddResponse), err
}

// Path is a 'getter' method
func (o *NvmeSubsystemMapAddRequest) Path() LunPathType {
	var r LunPathType
	if o.PathPtr == nil {
		return r
	}
	r = *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemMapAddRequest) SetPath(newValue LunPathType) *NvmeSubsystemMapAddRequest {
	o.PathPtr = &newValue
	return o
}

// Subsystem is a 'getter' method
func (o *NvmeSubsystemMapAddRequest) Subsystem() string {
	var r string
	if o.SubsystemPtr == nil {
		return r
	}
	r = *o.SubsystemPtr
	return r
}

// SetSubsystem is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemMapAddRequest) SetSubsystem(newValue string) *NvmeSubsystemMapAddRequest {
	o.SubsystemPtr = &newValue
	return o
}

// NamespaceUuid is a 'getter' method
func (o *NvmeSubsystemMapAddResponseResult) NamespaceUuid() UuidType {
	var r UuidType
	if o.NamespaceUuidPtr == nil {
		return r
	}
	r = *o.NamespaceUuidPtr
	return r
}

// SetNamespaceUuid is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemMapAddResponseResult) SetNamespaceUuid(newValue UuidType) *NvmeSubsystemMapAddResponseResult {
	o.NamespaceUuidPtr = &newValue
	return o
}

// Nsid is a 'getter' method
func (o *NvmeSubsystemMapAddResponseResult) Nsid() int {
	var r int
	if o.NsidPtr == nil {
		return r
	}
	r = *o.NsidPtr
	return r
}

// SetNsid is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemMapAddResponseResult) SetNsid(newValue int) *NvmeSubsystemMapAddResponseResult {
	o.NsidPtr = &newValue
	return o
}
