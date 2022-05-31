// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// VolumeCloneCreateAsyncRequest is a structure to represent a volume-clone-create-async Request ZAPI object
type VolumeCloneCreateAsyncRequest struct {
	XMLName                  xml.Name `xml:"volume-clone-create-async"`
	GidPtr                   *int     `xml:"gid"`
	JunctionActivePtr        *bool    `xml:"junction-active"`
	JunctionPathPtr          *string  `xml:"junction-path"`
	ParentSnapshotPtr        *string  `xml:"parent-snapshot"`
	ParentVolumePtr          *string  `xml:"parent-volume"`
	SpaceReservePtr          *string  `xml:"space-reserve"`
	UidPtr                   *int     `xml:"uid"`
	UseSnaprestoreLicensePtr *bool    `xml:"use-snaprestore-license"`
	VolumePtr                *string  `xml:"volume"`
	VserverDrProtectionPtr   *string  `xml:"vserver-dr-protection"`
}

// VolumeCloneCreateAsyncResponse is a structure to represent a volume-clone-create-async Response ZAPI object
type VolumeCloneCreateAsyncResponse struct {
	XMLName         xml.Name                             `xml:"netapp"`
	ResponseVersion string                               `xml:"version,attr"`
	ResponseXmlns   string                               `xml:"xmlns,attr"`
	Result          VolumeCloneCreateAsyncResponseResult `xml:"results"`
}

// NewVolumeCloneCreateAsyncResponse is a factory method for creating new instances of VolumeCloneCreateAsyncResponse objects
func NewVolumeCloneCreateAsyncResponse() *VolumeCloneCreateAsyncResponse {
	return &VolumeCloneCreateAsyncResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneCreateAsyncResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeCloneCreateAsyncResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeCloneCreateAsyncResponseResult is a structure to represent a volume-clone-create-async Response Result ZAPI object
type VolumeCloneCreateAsyncResponseResult struct {
	XMLName               xml.Name `xml:"results"`
	ResultStatusAttr      string   `xml:"status,attr"`
	ResultReasonAttr      string   `xml:"reason,attr"`
	ResultErrnoAttr       string   `xml:"errno,attr"`
	ResultErrorCodePtr    *int     `xml:"result-error-code"`
	ResultErrorMessagePtr *string  `xml:"result-error-message"`
	ResultJobidPtr        *int     `xml:"result-jobid"`
	ResultStatusPtr       *string  `xml:"result-status"`
}

// NewVolumeCloneCreateAsyncRequest is a factory method for creating new instances of VolumeCloneCreateAsyncRequest objects
func NewVolumeCloneCreateAsyncRequest() *VolumeCloneCreateAsyncRequest {
	return &VolumeCloneCreateAsyncRequest{}
}

// NewVolumeCloneCreateAsyncResponseResult is a factory method for creating new instances of VolumeCloneCreateAsyncResponseResult objects
func NewVolumeCloneCreateAsyncResponseResult() *VolumeCloneCreateAsyncResponseResult {
	return &VolumeCloneCreateAsyncResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeCloneCreateAsyncRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeCloneCreateAsyncResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneCreateAsyncRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneCreateAsyncResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeCloneCreateAsyncRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeCloneCreateAsyncResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeCloneCreateAsyncRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeCloneCreateAsyncResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeCloneCreateAsyncRequest", NewVolumeCloneCreateAsyncResponse())
	if result == nil {
		return nil, err
	}
	return result.(*VolumeCloneCreateAsyncResponse), err
}

// Gid is a 'getter' method
func (o *VolumeCloneCreateAsyncRequest) Gid() int {
	var r int
	if o.GidPtr == nil {
		return r
	}
	r = *o.GidPtr
	return r
}

// SetGid is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncRequest) SetGid(newValue int) *VolumeCloneCreateAsyncRequest {
	o.GidPtr = &newValue
	return o
}

// JunctionActive is a 'getter' method
func (o *VolumeCloneCreateAsyncRequest) JunctionActive() bool {
	var r bool
	if o.JunctionActivePtr == nil {
		return r
	}
	r = *o.JunctionActivePtr
	return r
}

// SetJunctionActive is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncRequest) SetJunctionActive(newValue bool) *VolumeCloneCreateAsyncRequest {
	o.JunctionActivePtr = &newValue
	return o
}

// JunctionPath is a 'getter' method
func (o *VolumeCloneCreateAsyncRequest) JunctionPath() string {
	var r string
	if o.JunctionPathPtr == nil {
		return r
	}
	r = *o.JunctionPathPtr
	return r
}

// SetJunctionPath is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncRequest) SetJunctionPath(newValue string) *VolumeCloneCreateAsyncRequest {
	o.JunctionPathPtr = &newValue
	return o
}

// ParentSnapshot is a 'getter' method
func (o *VolumeCloneCreateAsyncRequest) ParentSnapshot() string {
	var r string
	if o.ParentSnapshotPtr == nil {
		return r
	}
	r = *o.ParentSnapshotPtr
	return r
}

// SetParentSnapshot is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncRequest) SetParentSnapshot(newValue string) *VolumeCloneCreateAsyncRequest {
	o.ParentSnapshotPtr = &newValue
	return o
}

// ParentVolume is a 'getter' method
func (o *VolumeCloneCreateAsyncRequest) ParentVolume() string {
	var r string
	if o.ParentVolumePtr == nil {
		return r
	}
	r = *o.ParentVolumePtr
	return r
}

// SetParentVolume is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncRequest) SetParentVolume(newValue string) *VolumeCloneCreateAsyncRequest {
	o.ParentVolumePtr = &newValue
	return o
}

// SpaceReserve is a 'getter' method
func (o *VolumeCloneCreateAsyncRequest) SpaceReserve() string {
	var r string
	if o.SpaceReservePtr == nil {
		return r
	}
	r = *o.SpaceReservePtr
	return r
}

// SetSpaceReserve is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncRequest) SetSpaceReserve(newValue string) *VolumeCloneCreateAsyncRequest {
	o.SpaceReservePtr = &newValue
	return o
}

// Uid is a 'getter' method
func (o *VolumeCloneCreateAsyncRequest) Uid() int {
	var r int
	if o.UidPtr == nil {
		return r
	}
	r = *o.UidPtr
	return r
}

// SetUid is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncRequest) SetUid(newValue int) *VolumeCloneCreateAsyncRequest {
	o.UidPtr = &newValue
	return o
}

// UseSnaprestoreLicense is a 'getter' method
func (o *VolumeCloneCreateAsyncRequest) UseSnaprestoreLicense() bool {
	var r bool
	if o.UseSnaprestoreLicensePtr == nil {
		return r
	}
	r = *o.UseSnaprestoreLicensePtr
	return r
}

// SetUseSnaprestoreLicense is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncRequest) SetUseSnaprestoreLicense(newValue bool) *VolumeCloneCreateAsyncRequest {
	o.UseSnaprestoreLicensePtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *VolumeCloneCreateAsyncRequest) Volume() string {
	var r string
	if o.VolumePtr == nil {
		return r
	}
	r = *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncRequest) SetVolume(newValue string) *VolumeCloneCreateAsyncRequest {
	o.VolumePtr = &newValue
	return o
}

// VserverDrProtection is a 'getter' method
func (o *VolumeCloneCreateAsyncRequest) VserverDrProtection() string {
	var r string
	if o.VserverDrProtectionPtr == nil {
		return r
	}
	r = *o.VserverDrProtectionPtr
	return r
}

// SetVserverDrProtection is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncRequest) SetVserverDrProtection(newValue string) *VolumeCloneCreateAsyncRequest {
	o.VserverDrProtectionPtr = &newValue
	return o
}

// ResultErrorCode is a 'getter' method
func (o *VolumeCloneCreateAsyncResponseResult) ResultErrorCode() int {
	var r int
	if o.ResultErrorCodePtr == nil {
		return r
	}
	r = *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncResponseResult) SetResultErrorCode(newValue int) *VolumeCloneCreateAsyncResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a 'getter' method
func (o *VolumeCloneCreateAsyncResponseResult) ResultErrorMessage() string {
	var r string
	if o.ResultErrorMessagePtr == nil {
		return r
	}
	r = *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncResponseResult) SetResultErrorMessage(newValue string) *VolumeCloneCreateAsyncResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a 'getter' method
func (o *VolumeCloneCreateAsyncResponseResult) ResultJobid() int {
	var r int
	if o.ResultJobidPtr == nil {
		return r
	}
	r = *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncResponseResult) SetResultJobid(newValue int) *VolumeCloneCreateAsyncResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultStatus is a 'getter' method
func (o *VolumeCloneCreateAsyncResponseResult) ResultStatus() string {
	var r string
	if o.ResultStatusPtr == nil {
		return r
	}
	r = *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncResponseResult) SetResultStatus(newValue string) *VolumeCloneCreateAsyncResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}
