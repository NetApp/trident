// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// IgroupRemoveRequest is a structure to represent a igroup-remove Request ZAPI object
type IgroupRemoveRequest struct {
	XMLName               xml.Name `xml:"igroup-remove"`
	ForcePtr              *bool    `xml:"force"`
	InitiatorPtr          *string  `xml:"initiator"`
	InitiatorGroupNamePtr *string  `xml:"initiator-group-name"`
}

// IgroupRemoveResponse is a structure to represent a igroup-remove Response ZAPI object
type IgroupRemoveResponse struct {
	XMLName         xml.Name                   `xml:"netapp"`
	ResponseVersion string                     `xml:"version,attr"`
	ResponseXmlns   string                     `xml:"xmlns,attr"`
	Result          IgroupRemoveResponseResult `xml:"results"`
}

// NewIgroupRemoveResponse is a factory method for creating new instances of IgroupRemoveResponse objects
func NewIgroupRemoveResponse() *IgroupRemoveResponse {
	return &IgroupRemoveResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupRemoveResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *IgroupRemoveResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// IgroupRemoveResponseResult is a structure to represent a igroup-remove Response Result ZAPI object
type IgroupRemoveResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewIgroupRemoveRequest is a factory method for creating new instances of IgroupRemoveRequest objects
func NewIgroupRemoveRequest() *IgroupRemoveRequest {
	return &IgroupRemoveRequest{}
}

// NewIgroupRemoveResponseResult is a factory method for creating new instances of IgroupRemoveResponseResult objects
func NewIgroupRemoveResponseResult() *IgroupRemoveResponseResult {
	return &IgroupRemoveResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *IgroupRemoveRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *IgroupRemoveResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupRemoveRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupRemoveResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IgroupRemoveRequest) ExecuteUsing(zr *ZapiRunner) (*IgroupRemoveResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IgroupRemoveRequest) executeWithoutIteration(zr *ZapiRunner) (*IgroupRemoveResponse, error) {
	result, err := zr.ExecuteUsing(o, "IgroupRemoveRequest", NewIgroupRemoveResponse())
	if result == nil {
		return nil, err
	}
	return result.(*IgroupRemoveResponse), err
}

// Force is a 'getter' method
func (o *IgroupRemoveRequest) Force() bool {
	r := *o.ForcePtr
	return r
}

// SetForce is a fluent style 'setter' method that can be chained
func (o *IgroupRemoveRequest) SetForce(newValue bool) *IgroupRemoveRequest {
	o.ForcePtr = &newValue
	return o
}

// Initiator is a 'getter' method
func (o *IgroupRemoveRequest) Initiator() string {
	r := *o.InitiatorPtr
	return r
}

// SetInitiator is a fluent style 'setter' method that can be chained
func (o *IgroupRemoveRequest) SetInitiator(newValue string) *IgroupRemoveRequest {
	o.InitiatorPtr = &newValue
	return o
}

// InitiatorGroupName is a 'getter' method
func (o *IgroupRemoveRequest) InitiatorGroupName() string {
	r := *o.InitiatorGroupNamePtr
	return r
}

// SetInitiatorGroupName is a fluent style 'setter' method that can be chained
func (o *IgroupRemoveRequest) SetInitiatorGroupName(newValue string) *IgroupRemoveRequest {
	o.InitiatorGroupNamePtr = &newValue
	return o
}
