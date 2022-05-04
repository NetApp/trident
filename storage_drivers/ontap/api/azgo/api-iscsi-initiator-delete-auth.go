// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// IscsiInitiatorDeleteAuthRequest is a structure to represent a iscsi-initiator-delete-auth Request ZAPI object
type IscsiInitiatorDeleteAuthRequest struct {
	XMLName      xml.Name `xml:"iscsi-initiator-delete-auth"`
	InitiatorPtr *string  `xml:"initiator"`
}

// IscsiInitiatorDeleteAuthResponse is a structure to represent a iscsi-initiator-delete-auth Response ZAPI object
type IscsiInitiatorDeleteAuthResponse struct {
	XMLName         xml.Name                               `xml:"netapp"`
	ResponseVersion string                                 `xml:"version,attr"`
	ResponseXmlns   string                                 `xml:"xmlns,attr"`
	Result          IscsiInitiatorDeleteAuthResponseResult `xml:"results"`
}

// NewIscsiInitiatorDeleteAuthResponse is a factory method for creating new instances of IscsiInitiatorDeleteAuthResponse objects
func NewIscsiInitiatorDeleteAuthResponse() *IscsiInitiatorDeleteAuthResponse {
	return &IscsiInitiatorDeleteAuthResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorDeleteAuthResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorDeleteAuthResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// IscsiInitiatorDeleteAuthResponseResult is a structure to represent a iscsi-initiator-delete-auth Response Result ZAPI object
type IscsiInitiatorDeleteAuthResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewIscsiInitiatorDeleteAuthRequest is a factory method for creating new instances of IscsiInitiatorDeleteAuthRequest objects
func NewIscsiInitiatorDeleteAuthRequest() *IscsiInitiatorDeleteAuthRequest {
	return &IscsiInitiatorDeleteAuthRequest{}
}

// NewIscsiInitiatorDeleteAuthResponseResult is a factory method for creating new instances of IscsiInitiatorDeleteAuthResponseResult objects
func NewIscsiInitiatorDeleteAuthResponseResult() *IscsiInitiatorDeleteAuthResponseResult {
	return &IscsiInitiatorDeleteAuthResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorDeleteAuthRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorDeleteAuthResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorDeleteAuthRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorDeleteAuthResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiInitiatorDeleteAuthRequest) ExecuteUsing(zr *ZapiRunner) (*IscsiInitiatorDeleteAuthResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiInitiatorDeleteAuthRequest) executeWithoutIteration(zr *ZapiRunner) (*IscsiInitiatorDeleteAuthResponse, error) {
	result, err := zr.ExecuteUsing(o, "IscsiInitiatorDeleteAuthRequest", NewIscsiInitiatorDeleteAuthResponse())
	if result == nil {
		return nil, err
	}
	return result.(*IscsiInitiatorDeleteAuthResponse), err
}

// Initiator is a 'getter' method
func (o *IscsiInitiatorDeleteAuthRequest) Initiator() string {
	r := *o.InitiatorPtr
	return r
}

// SetInitiator is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorDeleteAuthRequest) SetInitiator(newValue string) *IscsiInitiatorDeleteAuthRequest {
	o.InitiatorPtr = &newValue
	return o
}
