// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// LunCreateBySizeRequest is a structure to represent a lun-create-by-size Request ZAPI object
type LunCreateBySizeRequest struct {
	XMLName                    xml.Name       `xml:"lun-create-by-size"`
	ApplicationPtr             *string        `xml:"application"`
	CachingPolicyPtr           *string        `xml:"caching-policy"`
	ClassPtr                   *string        `xml:"class"`
	CommentPtr                 *string        `xml:"comment"`
	ForeignDiskPtr             *string        `xml:"foreign-disk"`
	OstypePtr                  *LunOsTypeType `xml:"ostype"`
	PathPtr                    *string        `xml:"path"`
	PrefixSizePtr              *int           `xml:"prefix-size"`
	QosPolicyGroupPtr          *string        `xml:"qos-policy-group"`
	QosAdaptivePolicyGroupPtr  *string        `xml:"qos-adaptive-policy-group"`
	SizePtr                    *int           `xml:"size"`
	SpaceAllocationEnabledPtr  *bool          `xml:"space-allocation-enabled"`
	SpaceReservationEnabledPtr *bool          `xml:"space-reservation-enabled"`
	TypePtr                    *LunOsTypeType `xml:"type"`
	UseExactSizePtr            *bool          `xml:"use-exact-size"`
}

// LunCreateBySizeResponse is a structure to represent a lun-create-by-size Response ZAPI object
type LunCreateBySizeResponse struct {
	XMLName         xml.Name                      `xml:"netapp"`
	ResponseVersion string                        `xml:"version,attr"`
	ResponseXmlns   string                        `xml:"xmlns,attr"`
	Result          LunCreateBySizeResponseResult `xml:"results"`
}

// NewLunCreateBySizeResponse is a factory method for creating new instances of LunCreateBySizeResponse objects
func NewLunCreateBySizeResponse() *LunCreateBySizeResponse {
	return &LunCreateBySizeResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunCreateBySizeResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *LunCreateBySizeResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// LunCreateBySizeResponseResult is a structure to represent a lun-create-by-size Response Result ZAPI object
type LunCreateBySizeResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
	ActualSizePtr    *int     `xml:"actual-size"`
}

// NewLunCreateBySizeRequest is a factory method for creating new instances of LunCreateBySizeRequest objects
func NewLunCreateBySizeRequest() *LunCreateBySizeRequest {
	return &LunCreateBySizeRequest{}
}

// NewLunCreateBySizeResponseResult is a factory method for creating new instances of LunCreateBySizeResponseResult objects
func NewLunCreateBySizeResponseResult() *LunCreateBySizeResponseResult {
	return &LunCreateBySizeResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *LunCreateBySizeRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *LunCreateBySizeResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunCreateBySizeRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunCreateBySizeResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunCreateBySizeRequest) ExecuteUsing(zr *ZapiRunner) (*LunCreateBySizeResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunCreateBySizeRequest) executeWithoutIteration(zr *ZapiRunner) (*LunCreateBySizeResponse, error) {
	result, err := zr.ExecuteUsing(o, "LunCreateBySizeRequest", NewLunCreateBySizeResponse())
	if result == nil {
		return nil, err
	}
	return result.(*LunCreateBySizeResponse), err
}

// Application is a 'getter' method
func (o *LunCreateBySizeRequest) Application() string {
	var r string
	if o.ApplicationPtr == nil {
		return r
	}
	r = *o.ApplicationPtr
	return r
}

// SetApplication is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetApplication(newValue string) *LunCreateBySizeRequest {
	o.ApplicationPtr = &newValue
	return o
}

// CachingPolicy is a 'getter' method
func (o *LunCreateBySizeRequest) CachingPolicy() string {
	var r string
	if o.CachingPolicyPtr == nil {
		return r
	}
	r = *o.CachingPolicyPtr
	return r
}

// SetCachingPolicy is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetCachingPolicy(newValue string) *LunCreateBySizeRequest {
	o.CachingPolicyPtr = &newValue
	return o
}

// Class is a 'getter' method
func (o *LunCreateBySizeRequest) Class() string {
	var r string
	if o.ClassPtr == nil {
		return r
	}
	r = *o.ClassPtr
	return r
}

// SetClass is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetClass(newValue string) *LunCreateBySizeRequest {
	o.ClassPtr = &newValue
	return o
}

// Comment is a 'getter' method
func (o *LunCreateBySizeRequest) Comment() string {
	var r string
	if o.CommentPtr == nil {
		return r
	}
	r = *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetComment(newValue string) *LunCreateBySizeRequest {
	o.CommentPtr = &newValue
	return o
}

// ForeignDisk is a 'getter' method
func (o *LunCreateBySizeRequest) ForeignDisk() string {
	var r string
	if o.ForeignDiskPtr == nil {
		return r
	}
	r = *o.ForeignDiskPtr
	return r
}

// SetForeignDisk is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetForeignDisk(newValue string) *LunCreateBySizeRequest {
	o.ForeignDiskPtr = &newValue
	return o
}

// Ostype is a 'getter' method
func (o *LunCreateBySizeRequest) Ostype() LunOsTypeType {
	var r LunOsTypeType
	if o.OstypePtr == nil {
		return r
	}
	r = *o.OstypePtr
	return r
}

// SetOstype is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetOstype(newValue LunOsTypeType) *LunCreateBySizeRequest {
	o.OstypePtr = &newValue
	return o
}

// Path is a 'getter' method
func (o *LunCreateBySizeRequest) Path() string {
	var r string
	if o.PathPtr == nil {
		return r
	}
	r = *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetPath(newValue string) *LunCreateBySizeRequest {
	o.PathPtr = &newValue
	return o
}

// PrefixSize is a 'getter' method
func (o *LunCreateBySizeRequest) PrefixSize() int {
	var r int
	if o.PrefixSizePtr == nil {
		return r
	}
	r = *o.PrefixSizePtr
	return r
}

// SetPrefixSize is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetPrefixSize(newValue int) *LunCreateBySizeRequest {
	o.PrefixSizePtr = &newValue
	return o
}

// QosPolicyGroup is a 'getter' method
func (o *LunCreateBySizeRequest) QosPolicyGroup() string {
	var r string
	if o.QosPolicyGroupPtr == nil {
		return r
	}
	r = *o.QosPolicyGroupPtr
	return r
}

// SetQosPolicyGroup is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetQosPolicyGroup(newValue string) *LunCreateBySizeRequest {
	o.QosPolicyGroupPtr = &newValue
	return o
}

// QosAdaptivePolicyGroup is a 'getter' method
func (o *LunCreateBySizeRequest) QosAdaptivePolicyGroup() string {
	var r string
	if o.QosAdaptivePolicyGroupPtr != nil {
		r = *o.QosAdaptivePolicyGroupPtr
	}
	return r
}

// SetQosAdaptivePolicyGroup is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetQosAdaptivePolicyGroup(newValue string) *LunCreateBySizeRequest {
	o.QosAdaptivePolicyGroupPtr = &newValue
	return o
}

// Size is a 'getter' method
func (o *LunCreateBySizeRequest) Size() int {
	var r int
	if o.SizePtr == nil {
		return r
	}
	r = *o.SizePtr
	return r
}

// SetSize is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetSize(newValue int) *LunCreateBySizeRequest {
	o.SizePtr = &newValue
	return o
}

// SpaceAllocationEnabled is a 'getter' method
func (o *LunCreateBySizeRequest) SpaceAllocationEnabled() bool {
	var r bool
	if o.SpaceAllocationEnabledPtr == nil {
		return r
	}
	r = *o.SpaceAllocationEnabledPtr
	return r
}

// SetSpaceAllocationEnabled is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetSpaceAllocationEnabled(newValue bool) *LunCreateBySizeRequest {
	o.SpaceAllocationEnabledPtr = &newValue
	return o
}

// SpaceReservationEnabled is a 'getter' method
func (o *LunCreateBySizeRequest) SpaceReservationEnabled() bool {
	var r bool
	if o.SpaceReservationEnabledPtr == nil {
		return r
	}
	r = *o.SpaceReservationEnabledPtr
	return r
}

// SetSpaceReservationEnabled is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetSpaceReservationEnabled(newValue bool) *LunCreateBySizeRequest {
	o.SpaceReservationEnabledPtr = &newValue
	return o
}

// Type is a 'getter' method
func (o *LunCreateBySizeRequest) Type() LunOsTypeType {
	var r LunOsTypeType
	if o.TypePtr == nil {
		return r
	}
	r = *o.TypePtr
	return r
}

// SetType is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetType(newValue LunOsTypeType) *LunCreateBySizeRequest {
	o.TypePtr = &newValue
	return o
}

// UseExactSize is a 'getter' method
func (o *LunCreateBySizeRequest) UseExactSize() bool {
	var r bool
	if o.UseExactSizePtr == nil {
		return r
	}
	r = *o.UseExactSizePtr
	return r
}

// SetUseExactSize is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetUseExactSize(newValue bool) *LunCreateBySizeRequest {
	o.UseExactSizePtr = &newValue
	return o
}

// ActualSize is a 'getter' method
func (o *LunCreateBySizeResponseResult) ActualSize() int {
	var r int
	if o.ActualSizePtr == nil {
		return r
	}
	r = *o.ActualSizePtr
	return r
}

// SetActualSize is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeResponseResult) SetActualSize(newValue int) *LunCreateBySizeResponseResult {
	o.ActualSizePtr = &newValue
	return o
}
