// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SystemGetOntapiVersionRequest is a structure to represent a system-get-ontapi-version Request ZAPI object
type SystemGetOntapiVersionRequest struct {
	XMLName xml.Name `xml:"system-get-ontapi-version"`
}

// SystemGetOntapiVersionResponse is a structure to represent a system-get-ontapi-version Response ZAPI object
type SystemGetOntapiVersionResponse struct {
	XMLName         xml.Name                             `xml:"netapp"`
	ResponseVersion string                               `xml:"version,attr"`
	ResponseXmlns   string                               `xml:"xmlns,attr"`
	Result          SystemGetOntapiVersionResponseResult `xml:"results"`
}

// NewSystemGetOntapiVersionResponse is a factory method for creating new instances of SystemGetOntapiVersionResponse objects
func NewSystemGetOntapiVersionResponse() *SystemGetOntapiVersionResponse {
	return &SystemGetOntapiVersionResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemGetOntapiVersionResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SystemGetOntapiVersionResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SystemGetOntapiVersionResponseResult is a structure to represent a system-get-ontapi-version Response Result ZAPI object
type SystemGetOntapiVersionResponseResult struct {
	XMLName              xml.Name                                               `xml:"results"`
	ResultStatusAttr     string                                                 `xml:"status,attr"`
	ResultReasonAttr     string                                                 `xml:"reason,attr"`
	ResultErrnoAttr      string                                                 `xml:"errno,attr"`
	MajorVersionPtr      *int                                                   `xml:"major-version"`
	MinorVersionPtr      *int                                                   `xml:"minor-version"`
	NodeOntapiDetailsPtr *SystemGetOntapiVersionResponseResultNodeOntapiDetails `xml:"node-ontapi-details"`
}

// NewSystemGetOntapiVersionRequest is a factory method for creating new instances of SystemGetOntapiVersionRequest objects
func NewSystemGetOntapiVersionRequest() *SystemGetOntapiVersionRequest {
	return &SystemGetOntapiVersionRequest{}
}

// NewSystemGetOntapiVersionResponseResult is a factory method for creating new instances of SystemGetOntapiVersionResponseResult objects
func NewSystemGetOntapiVersionResponseResult() *SystemGetOntapiVersionResponseResult {
	return &SystemGetOntapiVersionResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SystemGetOntapiVersionRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SystemGetOntapiVersionResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemGetOntapiVersionRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemGetOntapiVersionResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SystemGetOntapiVersionRequest) ExecuteUsing(zr *ZapiRunner) (*SystemGetOntapiVersionResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SystemGetOntapiVersionRequest) executeWithoutIteration(zr *ZapiRunner) (*SystemGetOntapiVersionResponse, error) {
	result, err := zr.ExecuteUsing(o, "SystemGetOntapiVersionRequest", NewSystemGetOntapiVersionResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SystemGetOntapiVersionResponse), err
}

// MajorVersion is a 'getter' method
func (o *SystemGetOntapiVersionResponseResult) MajorVersion() int {
	r := *o.MajorVersionPtr
	return r
}

// SetMajorVersion is a fluent style 'setter' method that can be chained
func (o *SystemGetOntapiVersionResponseResult) SetMajorVersion(newValue int) *SystemGetOntapiVersionResponseResult {
	o.MajorVersionPtr = &newValue
	return o
}

// MinorVersion is a 'getter' method
func (o *SystemGetOntapiVersionResponseResult) MinorVersion() int {
	r := *o.MinorVersionPtr
	return r
}

// SetMinorVersion is a fluent style 'setter' method that can be chained
func (o *SystemGetOntapiVersionResponseResult) SetMinorVersion(newValue int) *SystemGetOntapiVersionResponseResult {
	o.MinorVersionPtr = &newValue
	return o
}

// SystemGetOntapiVersionResponseResultNodeOntapiDetails is a wrapper
type SystemGetOntapiVersionResponseResultNodeOntapiDetails struct {
	XMLName                 xml.Name                   `xml:"node-ontapi-details"`
	NodeOntapiDetailInfoPtr []NodeOntapiDetailInfoType `xml:"node-ontapi-detail-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemGetOntapiVersionResponseResultNodeOntapiDetails) String() string {
	return ToString(reflect.ValueOf(o))
}

// NodeOntapiDetailInfo is a 'getter' method
func (o *SystemGetOntapiVersionResponseResultNodeOntapiDetails) NodeOntapiDetailInfo() []NodeOntapiDetailInfoType {
	r := o.NodeOntapiDetailInfoPtr
	return r
}

// SetNodeOntapiDetailInfo is a fluent style 'setter' method that can be chained
func (o *SystemGetOntapiVersionResponseResultNodeOntapiDetails) SetNodeOntapiDetailInfo(newValue []NodeOntapiDetailInfoType) *SystemGetOntapiVersionResponseResultNodeOntapiDetails {
	newSlice := make([]NodeOntapiDetailInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NodeOntapiDetailInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *SystemGetOntapiVersionResponseResultNodeOntapiDetails) values() []NodeOntapiDetailInfoType {
	r := o.NodeOntapiDetailInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *SystemGetOntapiVersionResponseResultNodeOntapiDetails) setValues(newValue []NodeOntapiDetailInfoType) *SystemGetOntapiVersionResponseResultNodeOntapiDetails {
	newSlice := make([]NodeOntapiDetailInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NodeOntapiDetailInfoPtr = newSlice
	return o
}

// NodeOntapiDetails is a 'getter' method
func (o *SystemGetOntapiVersionResponseResult) NodeOntapiDetails() SystemGetOntapiVersionResponseResultNodeOntapiDetails {
	r := *o.NodeOntapiDetailsPtr
	return r
}

// SetNodeOntapiDetails is a fluent style 'setter' method that can be chained
func (o *SystemGetOntapiVersionResponseResult) SetNodeOntapiDetails(newValue SystemGetOntapiVersionResponseResultNodeOntapiDetails) *SystemGetOntapiVersionResponseResult {
	o.NodeOntapiDetailsPtr = &newValue
	return o
}
