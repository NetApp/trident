// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// LunCreateBySizeRequest is a structure to represent a lun-create-by-size ZAPI request object
type LunCreateBySizeRequest struct {
	XMLName xml.Name `xml:"lun-create-by-size"`

	CachingPolicyPtr           *string `xml:"caching-policy"`
	ClassPtr                   *string `xml:"class"`
	CommentPtr                 *string `xml:"comment"`
	ForeignDiskPtr             *string `xml:"foreign-disk"`
	OstypePtr                  *string `xml:"ostype"`
	PathPtr                    *string `xml:"path"`
	PrefixSizePtr              *int    `xml:"prefix-size"`
	QosPolicyGroupPtr          *string `xml:"qos-policy-group"`
	SizePtr                    *int    `xml:"size"`
	SpaceAllocationEnabledPtr  *bool   `xml:"space-allocation-enabled"`
	SpaceReservationEnabledPtr *bool   `xml:"space-reservation-enabled"`
	TypePtr                    *string `xml:"type"`
}

// ToXML converts this object into an xml string representation
func (o *LunCreateBySizeRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewLunCreateBySizeRequest is a factory method for creating new instances of LunCreateBySizeRequest objects
func NewLunCreateBySizeRequest() *LunCreateBySizeRequest { return &LunCreateBySizeRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *LunCreateBySizeRequest) ExecuteUsing(zr *ZapiRunner) (LunCreateBySizeResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "LunCreateBySizeRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return LunCreateBySizeResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return LunCreateBySizeResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n LunCreateBySizeResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return LunCreateBySizeResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("lun-create-by-size result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunCreateBySizeRequest) String() string {
	var buffer bytes.Buffer
	if o.CachingPolicyPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "caching-policy", *o.CachingPolicyPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("caching-policy: nil\n"))
	}
	if o.ClassPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "class", *o.ClassPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("class: nil\n"))
	}
	if o.CommentPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "comment", *o.CommentPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("comment: nil\n"))
	}
	if o.ForeignDiskPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "foreign-disk", *o.ForeignDiskPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("foreign-disk: nil\n"))
	}
	if o.OstypePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "ostype", *o.OstypePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("ostype: nil\n"))
	}
	if o.PathPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "path", *o.PathPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("path: nil\n"))
	}
	if o.PrefixSizePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "prefix-size", *o.PrefixSizePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("prefix-size: nil\n"))
	}
	if o.QosPolicyGroupPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "qos-policy-group", *o.QosPolicyGroupPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("qos-policy-group: nil\n"))
	}
	if o.SizePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "size", *o.SizePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("size: nil\n"))
	}
	if o.SpaceAllocationEnabledPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "space-allocation-enabled", *o.SpaceAllocationEnabledPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("space-allocation-enabled: nil\n"))
	}
	if o.SpaceReservationEnabledPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "space-reservation-enabled", *o.SpaceReservationEnabledPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("space-reservation-enabled: nil\n"))
	}
	if o.TypePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "type", *o.TypePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("type: nil\n"))
	}
	return buffer.String()
}

// CachingPolicy is a fluent style 'getter' method that can be chained
func (o *LunCreateBySizeRequest) CachingPolicy() string {
	r := *o.CachingPolicyPtr
	return r
}

// SetCachingPolicy is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetCachingPolicy(newValue string) *LunCreateBySizeRequest {
	o.CachingPolicyPtr = &newValue
	return o
}

// Class is a fluent style 'getter' method that can be chained
func (o *LunCreateBySizeRequest) Class() string {
	r := *o.ClassPtr
	return r
}

// SetClass is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetClass(newValue string) *LunCreateBySizeRequest {
	o.ClassPtr = &newValue
	return o
}

// Comment is a fluent style 'getter' method that can be chained
func (o *LunCreateBySizeRequest) Comment() string {
	r := *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetComment(newValue string) *LunCreateBySizeRequest {
	o.CommentPtr = &newValue
	return o
}

// ForeignDisk is a fluent style 'getter' method that can be chained
func (o *LunCreateBySizeRequest) ForeignDisk() string {
	r := *o.ForeignDiskPtr
	return r
}

// SetForeignDisk is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetForeignDisk(newValue string) *LunCreateBySizeRequest {
	o.ForeignDiskPtr = &newValue
	return o
}

// Ostype is a fluent style 'getter' method that can be chained
func (o *LunCreateBySizeRequest) Ostype() string {
	r := *o.OstypePtr
	return r
}

// SetOstype is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetOstype(newValue string) *LunCreateBySizeRequest {
	o.OstypePtr = &newValue
	return o
}

// Path is a fluent style 'getter' method that can be chained
func (o *LunCreateBySizeRequest) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetPath(newValue string) *LunCreateBySizeRequest {
	o.PathPtr = &newValue
	return o
}

// PrefixSize is a fluent style 'getter' method that can be chained
func (o *LunCreateBySizeRequest) PrefixSize() int {
	r := *o.PrefixSizePtr
	return r
}

// SetPrefixSize is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetPrefixSize(newValue int) *LunCreateBySizeRequest {
	o.PrefixSizePtr = &newValue
	return o
}

// QosPolicyGroup is a fluent style 'getter' method that can be chained
func (o *LunCreateBySizeRequest) QosPolicyGroup() string {
	r := *o.QosPolicyGroupPtr
	return r
}

// SetQosPolicyGroup is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetQosPolicyGroup(newValue string) *LunCreateBySizeRequest {
	o.QosPolicyGroupPtr = &newValue
	return o
}

// Size is a fluent style 'getter' method that can be chained
func (o *LunCreateBySizeRequest) Size() int {
	r := *o.SizePtr
	return r
}

// SetSize is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetSize(newValue int) *LunCreateBySizeRequest {
	o.SizePtr = &newValue
	return o
}

// SpaceAllocationEnabled is a fluent style 'getter' method that can be chained
func (o *LunCreateBySizeRequest) SpaceAllocationEnabled() bool {
	r := *o.SpaceAllocationEnabledPtr
	return r
}

// SetSpaceAllocationEnabled is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetSpaceAllocationEnabled(newValue bool) *LunCreateBySizeRequest {
	o.SpaceAllocationEnabledPtr = &newValue
	return o
}

// SpaceReservationEnabled is a fluent style 'getter' method that can be chained
func (o *LunCreateBySizeRequest) SpaceReservationEnabled() bool {
	r := *o.SpaceReservationEnabledPtr
	return r
}

// SetSpaceReservationEnabled is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetSpaceReservationEnabled(newValue bool) *LunCreateBySizeRequest {
	o.SpaceReservationEnabledPtr = &newValue
	return o
}

// Type is a fluent style 'getter' method that can be chained
func (o *LunCreateBySizeRequest) Type() string {
	r := *o.TypePtr
	return r
}

// SetType is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetType(newValue string) *LunCreateBySizeRequest {
	o.TypePtr = &newValue
	return o
}

// LunCreateBySizeResponse is a structure to represent a lun-create-by-size ZAPI response object
type LunCreateBySizeResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result LunCreateBySizeResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunCreateBySizeResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// LunCreateBySizeResponseResult is a structure to represent a lun-create-by-size ZAPI object's result
type LunCreateBySizeResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
	ActualSizePtr    *int   `xml:"actual-size"`
}

// ToXML converts this object into an xml string representation
func (o *LunCreateBySizeResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewLunCreateBySizeResponse is a factory method for creating new instances of LunCreateBySizeResponse objects
func NewLunCreateBySizeResponse() *LunCreateBySizeResponse { return &LunCreateBySizeResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunCreateBySizeResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.ActualSizePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "actual-size", *o.ActualSizePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("actual-size: nil\n"))
	}
	return buffer.String()
}

// ActualSize is a fluent style 'getter' method that can be chained
func (o *LunCreateBySizeResponseResult) ActualSize() int {
	r := *o.ActualSizePtr
	return r
}

// SetActualSize is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeResponseResult) SetActualSize(newValue int) *LunCreateBySizeResponseResult {
	o.ActualSizePtr = &newValue
	return o
}
