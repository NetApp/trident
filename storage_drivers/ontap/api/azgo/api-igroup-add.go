// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// IgroupAddRequest is a structure to represent a igroup-add Request ZAPI object
type IgroupAddRequest struct {
	XMLName               xml.Name `xml:"igroup-add"`
	ForcePtr              *bool    `xml:"force"`
	InitiatorPtr          *string  `xml:"initiator"`
	InitiatorGroupNamePtr *string  `xml:"initiator-group-name"`
}

// IgroupAddResponse is a structure to represent a igroup-add Response ZAPI object
type IgroupAddResponse struct {
	XMLName         xml.Name                `xml:"netapp"`
	ResponseVersion string                  `xml:"version,attr"`
	ResponseXmlns   string                  `xml:"xmlns,attr"`
	Result          IgroupAddResponseResult `xml:"results"`
}

// NewIgroupAddResponse is a factory method for creating new instances of IgroupAddResponse objects
func NewIgroupAddResponse() *IgroupAddResponse {
	return &IgroupAddResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupAddResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *IgroupAddResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// IgroupAddResponseResult is a structure to represent a igroup-add Response Result ZAPI object
type IgroupAddResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewIgroupAddRequest is a factory method for creating new instances of IgroupAddRequest objects
func NewIgroupAddRequest() *IgroupAddRequest {
	return &IgroupAddRequest{}
}

// NewIgroupAddResponseResult is a factory method for creating new instances of IgroupAddResponseResult objects
func NewIgroupAddResponseResult() *IgroupAddResponseResult {
	return &IgroupAddResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *IgroupAddRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *IgroupAddResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupAddRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupAddResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IgroupAddRequest) ExecuteUsing(zr *ZapiRunner) (*IgroupAddResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IgroupAddRequest) executeWithoutIteration(zr *ZapiRunner) (*IgroupAddResponse, error) {
	result, err := zr.ExecuteUsing(o, "IgroupAddRequest", NewIgroupAddResponse())
	if result == nil {
		return nil, err
	}
	return result.(*IgroupAddResponse), err
}

// Force is a 'getter' method
func (o *IgroupAddRequest) Force() bool {
	r := *o.ForcePtr
	return r
}

// SetForce is a fluent style 'setter' method that can be chained
func (o *IgroupAddRequest) SetForce(newValue bool) *IgroupAddRequest {
	o.ForcePtr = &newValue
	return o
}

// Initiator is a 'getter' method
func (o *IgroupAddRequest) Initiator() string {
	r := *o.InitiatorPtr
	return r
}

// SetInitiator is a fluent style 'setter' method that can be chained
func (o *IgroupAddRequest) SetInitiator(newValue string) *IgroupAddRequest {
	o.InitiatorPtr = &newValue
	return o
}

// InitiatorGroupName is a 'getter' method
func (o *IgroupAddRequest) InitiatorGroupName() string {
	r := *o.InitiatorGroupNamePtr
	return r
}

// SetInitiatorGroupName is a fluent style 'setter' method that can be chained
func (o *IgroupAddRequest) SetInitiatorGroupName(newValue string) *IgroupAddRequest {
	o.InitiatorGroupNamePtr = &newValue
	return o
}
