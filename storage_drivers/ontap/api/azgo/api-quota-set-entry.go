// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// QuotaSetEntryRequest is a structure to represent a quota-set-entry Request ZAPI object
type QuotaSetEntryRequest struct {
	XMLName               xml.Name `xml:"quota-set-entry"`
	DiskLimitPtr          *string  `xml:"disk-limit"`
	FileLimitPtr          *string  `xml:"file-limit"`
	PerformUserMappingPtr *bool    `xml:"perform-user-mapping"`
	PolicyPtr             *string  `xml:"policy"`
	QtreePtr              *string  `xml:"qtree"`
	QuotaTargetPtr        *string  `xml:"quota-target"`
	QuotaTypePtr          *string  `xml:"quota-type"`
	SoftDiskLimitPtr      *string  `xml:"soft-disk-limit"`
	SoftFileLimitPtr      *string  `xml:"soft-file-limit"`
	ThresholdPtr          *string  `xml:"threshold"`
	VolumePtr             *string  `xml:"volume"`
}

// QuotaSetEntryResponse is a structure to represent a quota-set-entry Response ZAPI object
type QuotaSetEntryResponse struct {
	XMLName         xml.Name                    `xml:"netapp"`
	ResponseVersion string                      `xml:"version,attr"`
	ResponseXmlns   string                      `xml:"xmlns,attr"`
	Result          QuotaSetEntryResponseResult `xml:"results"`
}

// NewQuotaSetEntryResponse is a factory method for creating new instances of QuotaSetEntryResponse objects
func NewQuotaSetEntryResponse() *QuotaSetEntryResponse {
	return &QuotaSetEntryResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaSetEntryResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *QuotaSetEntryResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// QuotaSetEntryResponseResult is a structure to represent a quota-set-entry Response Result ZAPI object
type QuotaSetEntryResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewQuotaSetEntryRequest is a factory method for creating new instances of QuotaSetEntryRequest objects
func NewQuotaSetEntryRequest() *QuotaSetEntryRequest {
	return &QuotaSetEntryRequest{}
}

// NewQuotaSetEntryResponseResult is a factory method for creating new instances of QuotaSetEntryResponseResult objects
func NewQuotaSetEntryResponseResult() *QuotaSetEntryResponseResult {
	return &QuotaSetEntryResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *QuotaSetEntryRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *QuotaSetEntryResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaSetEntryRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaSetEntryResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *QuotaSetEntryRequest) ExecuteUsing(zr *ZapiRunner) (*QuotaSetEntryResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *QuotaSetEntryRequest) executeWithoutIteration(zr *ZapiRunner) (*QuotaSetEntryResponse, error) {
	result, err := zr.ExecuteUsing(o, "QuotaSetEntryRequest", NewQuotaSetEntryResponse())
	if result == nil {
		return nil, err
	}
	return result.(*QuotaSetEntryResponse), err
}

// DiskLimit is a 'getter' method
func (o *QuotaSetEntryRequest) DiskLimit() string {
	var r string
	if o.DiskLimitPtr == nil {
		return r
	}
	r = *o.DiskLimitPtr
	return r
}

// SetDiskLimit is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetDiskLimit(newValue string) *QuotaSetEntryRequest {
	o.DiskLimitPtr = &newValue
	return o
}

// FileLimit is a 'getter' method
func (o *QuotaSetEntryRequest) FileLimit() string {
	var r string
	if o.FileLimitPtr == nil {
		return r
	}
	r = *o.FileLimitPtr
	return r
}

// SetFileLimit is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetFileLimit(newValue string) *QuotaSetEntryRequest {
	o.FileLimitPtr = &newValue
	return o
}

// PerformUserMapping is a 'getter' method
func (o *QuotaSetEntryRequest) PerformUserMapping() bool {
	var r bool
	if o.PerformUserMappingPtr == nil {
		return r
	}
	r = *o.PerformUserMappingPtr
	return r
}

// SetPerformUserMapping is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetPerformUserMapping(newValue bool) *QuotaSetEntryRequest {
	o.PerformUserMappingPtr = &newValue
	return o
}

// Policy is a 'getter' method
func (o *QuotaSetEntryRequest) Policy() string {
	var r string
	if o.PolicyPtr == nil {
		return r
	}
	r = *o.PolicyPtr
	return r
}

// SetPolicy is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetPolicy(newValue string) *QuotaSetEntryRequest {
	o.PolicyPtr = &newValue
	return o
}

// Qtree is a 'getter' method
func (o *QuotaSetEntryRequest) Qtree() string {
	var r string
	if o.QtreePtr == nil {
		return r
	}
	r = *o.QtreePtr
	return r
}

// SetQtree is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetQtree(newValue string) *QuotaSetEntryRequest {
	o.QtreePtr = &newValue
	return o
}

// QuotaTarget is a 'getter' method
func (o *QuotaSetEntryRequest) QuotaTarget() string {
	var r string
	if o.QuotaTargetPtr == nil {
		return r
	}
	r = *o.QuotaTargetPtr
	return r
}

// SetQuotaTarget is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetQuotaTarget(newValue string) *QuotaSetEntryRequest {
	o.QuotaTargetPtr = &newValue
	return o
}

// QuotaType is a 'getter' method
func (o *QuotaSetEntryRequest) QuotaType() string {
	var r string
	if o.QuotaTypePtr == nil {
		return r
	}
	r = *o.QuotaTypePtr
	return r
}

// SetQuotaType is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetQuotaType(newValue string) *QuotaSetEntryRequest {
	o.QuotaTypePtr = &newValue
	return o
}

// SoftDiskLimit is a 'getter' method
func (o *QuotaSetEntryRequest) SoftDiskLimit() string {
	var r string
	if o.SoftDiskLimitPtr == nil {
		return r
	}
	r = *o.SoftDiskLimitPtr
	return r
}

// SetSoftDiskLimit is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetSoftDiskLimit(newValue string) *QuotaSetEntryRequest {
	o.SoftDiskLimitPtr = &newValue
	return o
}

// SoftFileLimit is a 'getter' method
func (o *QuotaSetEntryRequest) SoftFileLimit() string {
	var r string
	if o.SoftFileLimitPtr == nil {
		return r
	}
	r = *o.SoftFileLimitPtr
	return r
}

// SetSoftFileLimit is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetSoftFileLimit(newValue string) *QuotaSetEntryRequest {
	o.SoftFileLimitPtr = &newValue
	return o
}

// Threshold is a 'getter' method
func (o *QuotaSetEntryRequest) Threshold() string {
	var r string
	if o.ThresholdPtr == nil {
		return r
	}
	r = *o.ThresholdPtr
	return r
}

// SetThreshold is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetThreshold(newValue string) *QuotaSetEntryRequest {
	o.ThresholdPtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *QuotaSetEntryRequest) Volume() string {
	var r string
	if o.VolumePtr == nil {
		return r
	}
	r = *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetVolume(newValue string) *QuotaSetEntryRequest {
	o.VolumePtr = &newValue
	return o
}
