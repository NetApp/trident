package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// IscsiNodeGetNameRequest is a structure to represent a iscsi-node-get-name Request ZAPI object
type IscsiNodeGetNameRequest struct {
	XMLName xml.Name `xml:"iscsi-node-get-name"`
}

// IscsiNodeGetNameResponse is a structure to represent a iscsi-node-get-name Response ZAPI object
type IscsiNodeGetNameResponse struct {
	XMLName         xml.Name                       `xml:"netapp"`
	ResponseVersion string                         `xml:"version,attr"`
	ResponseXmlns   string                         `xml:"xmlns,attr"`
	Result          IscsiNodeGetNameResponseResult `xml:"results"`
}

// NewIscsiNodeGetNameResponse is a factory method for creating new instances of IscsiNodeGetNameResponse objects
func NewIscsiNodeGetNameResponse() *IscsiNodeGetNameResponse {
	return &IscsiNodeGetNameResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiNodeGetNameResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *IscsiNodeGetNameResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// IscsiNodeGetNameResponseResult is a structure to represent a iscsi-node-get-name Response Result ZAPI object
type IscsiNodeGetNameResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
	NodeNamePtr      *string  `xml:"node-name"`
}

// NewIscsiNodeGetNameRequest is a factory method for creating new instances of IscsiNodeGetNameRequest objects
func NewIscsiNodeGetNameRequest() *IscsiNodeGetNameRequest {
	return &IscsiNodeGetNameRequest{}
}

// NewIscsiNodeGetNameResponseResult is a factory method for creating new instances of IscsiNodeGetNameResponseResult objects
func NewIscsiNodeGetNameResponseResult() *IscsiNodeGetNameResponseResult {
	return &IscsiNodeGetNameResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *IscsiNodeGetNameRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *IscsiNodeGetNameResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiNodeGetNameRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiNodeGetNameResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *IscsiNodeGetNameRequest) ExecuteUsing(zr *ZapiRunner) (*IscsiNodeGetNameResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *IscsiNodeGetNameRequest) executeWithoutIteration(zr *ZapiRunner) (*IscsiNodeGetNameResponse, error) {
	result, err := zr.ExecuteUsing(o, "IscsiNodeGetNameRequest", NewIscsiNodeGetNameResponse())
	return result.(*IscsiNodeGetNameResponse), err
}

// NodeName is a 'getter' method
func (o *IscsiNodeGetNameResponseResult) NodeName() string {
	r := *o.NodeNamePtr
	return r
}

// SetNodeName is a fluent style 'setter' method that can be chained
func (o *IscsiNodeGetNameResponseResult) SetNodeName(newValue string) *IscsiNodeGetNameResponseResult {
	o.NodeNamePtr = &newValue
	return o
}
