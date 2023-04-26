// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NvmeNamespaceDeleteRequest is a structure to represent a nvme-namespace-delete Request ZAPI object
type NvmeNamespaceDeleteRequest struct {
	XMLName                        xml.Name     `xml:"nvme-namespace-delete"`
	DestroyApplicationNamespacePtr *bool        `xml:"destroy-application-namespace"`
	PathPtr                        *LunPathType `xml:"path"`
	SkipMappedCheckPtr             *bool        `xml:"skip-mapped-check"`
}

// NvmeNamespaceDeleteResponse is a structure to represent a nvme-namespace-delete Response ZAPI object
type NvmeNamespaceDeleteResponse struct {
	XMLName         xml.Name                          `xml:"netapp"`
	ResponseVersion string                            `xml:"version,attr"`
	ResponseXmlns   string                            `xml:"xmlns,attr"`
	Result          NvmeNamespaceDeleteResponseResult `xml:"results"`
}

// NewNvmeNamespaceDeleteResponse is a factory method for creating new instances of NvmeNamespaceDeleteResponse objects
func NewNvmeNamespaceDeleteResponse() *NvmeNamespaceDeleteResponse {
	return &NvmeNamespaceDeleteResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeNamespaceDeleteResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeNamespaceDeleteResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeNamespaceDeleteResponseResult is a structure to represent a nvme-namespace-delete Response Result ZAPI object
type NvmeNamespaceDeleteResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewNvmeNamespaceDeleteRequest is a factory method for creating new instances of NvmeNamespaceDeleteRequest objects
func NewNvmeNamespaceDeleteRequest() *NvmeNamespaceDeleteRequest {
	return &NvmeNamespaceDeleteRequest{}
}

// NewNvmeNamespaceDeleteResponseResult is a factory method for creating new instances of NvmeNamespaceDeleteResponseResult objects
func NewNvmeNamespaceDeleteResponseResult() *NvmeNamespaceDeleteResponseResult {
	return &NvmeNamespaceDeleteResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeNamespaceDeleteRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeNamespaceDeleteResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeNamespaceDeleteRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeNamespaceDeleteResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeNamespaceDeleteRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeNamespaceDeleteResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeNamespaceDeleteRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeNamespaceDeleteResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeNamespaceDeleteRequest", NewNvmeNamespaceDeleteResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeNamespaceDeleteResponse), err
}

// DestroyApplicationNamespace is a 'getter' method
func (o *NvmeNamespaceDeleteRequest) DestroyApplicationNamespace() bool {
	var r bool
	if o.DestroyApplicationNamespacePtr == nil {
		return r
	}
	r = *o.DestroyApplicationNamespacePtr
	return r
}

// SetDestroyApplicationNamespace is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceDeleteRequest) SetDestroyApplicationNamespace(newValue bool) *NvmeNamespaceDeleteRequest {
	o.DestroyApplicationNamespacePtr = &newValue
	return o
}

// Path is a 'getter' method
func (o *NvmeNamespaceDeleteRequest) Path() LunPathType {
	var r LunPathType
	if o.PathPtr == nil {
		return r
	}
	r = *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceDeleteRequest) SetPath(newValue LunPathType) *NvmeNamespaceDeleteRequest {
	o.PathPtr = &newValue
	return o
}

// SkipMappedCheck is a 'getter' method
func (o *NvmeNamespaceDeleteRequest) SkipMappedCheck() bool {
	var r bool
	if o.SkipMappedCheckPtr == nil {
		return r
	}
	r = *o.SkipMappedCheckPtr
	return r
}

// SetSkipMappedCheck is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceDeleteRequest) SetSkipMappedCheck(newValue bool) *NvmeNamespaceDeleteRequest {
	o.SkipMappedCheckPtr = &newValue
	return o
}
