// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// VolumeCloneCreateRequest is a structure to represent a volume-clone-create Request ZAPI object
type VolumeCloneCreateRequest struct {
	XMLName                  xml.Name `xml:"volume-clone-create"`
	CachingPolicyPtr         *string  `xml:"caching-policy"`
	ParentSnapshotPtr        *string  `xml:"parent-snapshot"`
	ParentVolumePtr          *string  `xml:"parent-volume"`
	ParentVserverPtr         *string  `xml:"parent-vserver"`
	QosPolicyGroupNamePtr    *string  `xml:"qos-policy-group-name"`
	SpaceReservePtr          *string  `xml:"space-reserve"`
	UseSnaprestoreLicensePtr *bool    `xml:"use-snaprestore-license"`
	VolumePtr                *string  `xml:"volume"`
	VolumeTypePtr            *string  `xml:"volume-type"`
	VserverPtr               *string  `xml:"vserver"`
}

// VolumeCloneCreateResponse is a structure to represent a volume-clone-create Response ZAPI object
type VolumeCloneCreateResponse struct {
	XMLName         xml.Name                        `xml:"netapp"`
	ResponseVersion string                          `xml:"version,attr"`
	ResponseXmlns   string                          `xml:"xmlns,attr"`
	Result          VolumeCloneCreateResponseResult `xml:"results"`
}

// NewVolumeCloneCreateResponse is a factory method for creating new instances of VolumeCloneCreateResponse objects
func NewVolumeCloneCreateResponse() *VolumeCloneCreateResponse {
	return &VolumeCloneCreateResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneCreateResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeCloneCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeCloneCreateResponseResult is a structure to represent a volume-clone-create Response Result ZAPI object
type VolumeCloneCreateResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewVolumeCloneCreateRequest is a factory method for creating new instances of VolumeCloneCreateRequest objects
func NewVolumeCloneCreateRequest() *VolumeCloneCreateRequest {
	return &VolumeCloneCreateRequest{}
}

// NewVolumeCloneCreateResponseResult is a factory method for creating new instances of VolumeCloneCreateResponseResult objects
func NewVolumeCloneCreateResponseResult() *VolumeCloneCreateResponseResult {
	return &VolumeCloneCreateResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeCloneCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeCloneCreateResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneCreateRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneCreateResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeCloneCreateRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeCloneCreateResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeCloneCreateRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeCloneCreateResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeCloneCreateRequest", NewVolumeCloneCreateResponse())
	if result == nil {
		return nil, err
	}
	return result.(*VolumeCloneCreateResponse), err
}

// CachingPolicy is a 'getter' method
func (o *VolumeCloneCreateRequest) CachingPolicy() string {
	var r string
	if o.CachingPolicyPtr == nil {
		return r
	}
	r = *o.CachingPolicyPtr
	return r
}

// SetCachingPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetCachingPolicy(newValue string) *VolumeCloneCreateRequest {
	o.CachingPolicyPtr = &newValue
	return o
}

// ParentSnapshot is a 'getter' method
func (o *VolumeCloneCreateRequest) ParentSnapshot() string {
	var r string
	if o.ParentSnapshotPtr == nil {
		return r
	}
	r = *o.ParentSnapshotPtr
	return r
}

// SetParentSnapshot is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetParentSnapshot(newValue string) *VolumeCloneCreateRequest {
	o.ParentSnapshotPtr = &newValue
	return o
}

// ParentVolume is a 'getter' method
func (o *VolumeCloneCreateRequest) ParentVolume() string {
	var r string
	if o.ParentVolumePtr == nil {
		return r
	}
	r = *o.ParentVolumePtr
	return r
}

// SetParentVolume is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetParentVolume(newValue string) *VolumeCloneCreateRequest {
	o.ParentVolumePtr = &newValue
	return o
}

// ParentVserver is a 'getter' method
func (o *VolumeCloneCreateRequest) ParentVserver() string {
	var r string
	if o.ParentVserverPtr == nil {
		return r
	}
	r = *o.ParentVserverPtr
	return r
}

// SetParentVserver is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetParentVserver(newValue string) *VolumeCloneCreateRequest {
	o.ParentVserverPtr = &newValue
	return o
}

// QosPolicyGroupName is a 'getter' method
func (o *VolumeCloneCreateRequest) QosPolicyGroupName() string {
	var r string
	if o.QosPolicyGroupNamePtr == nil {
		return r
	}
	r = *o.QosPolicyGroupNamePtr
	return r
}

// SetQosPolicyGroupName is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetQosPolicyGroupName(newValue string) *VolumeCloneCreateRequest {
	o.QosPolicyGroupNamePtr = &newValue
	return o
}

// SpaceReserve is a 'getter' method
func (o *VolumeCloneCreateRequest) SpaceReserve() string {
	var r string
	if o.SpaceReservePtr == nil {
		return r
	}
	r = *o.SpaceReservePtr
	return r
}

// SetSpaceReserve is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetSpaceReserve(newValue string) *VolumeCloneCreateRequest {
	o.SpaceReservePtr = &newValue
	return o
}

// UseSnaprestoreLicense is a 'getter' method
func (o *VolumeCloneCreateRequest) UseSnaprestoreLicense() bool {
	var r bool
	if o.UseSnaprestoreLicensePtr == nil {
		return r
	}
	r = *o.UseSnaprestoreLicensePtr
	return r
}

// SetUseSnaprestoreLicense is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetUseSnaprestoreLicense(newValue bool) *VolumeCloneCreateRequest {
	o.UseSnaprestoreLicensePtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *VolumeCloneCreateRequest) Volume() string {
	var r string
	if o.VolumePtr == nil {
		return r
	}
	r = *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetVolume(newValue string) *VolumeCloneCreateRequest {
	o.VolumePtr = &newValue
	return o
}

// VolumeType is a 'getter' method
func (o *VolumeCloneCreateRequest) VolumeType() string {
	var r string
	if o.VolumeTypePtr == nil {
		return r
	}
	r = *o.VolumeTypePtr
	return r
}

// SetVolumeType is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetVolumeType(newValue string) *VolumeCloneCreateRequest {
	o.VolumeTypePtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *VolumeCloneCreateRequest) Vserver() string {
	var r string
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateRequest) SetVserver(newValue string) *VolumeCloneCreateRequest {
	o.VserverPtr = &newValue
	return o
}
