// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/Sirupsen/logrus"
)

// IgroupRemoveRequest is a structure to represent a igroup-remove ZAPI request object
type IgroupRemoveRequest struct {
	XMLName xml.Name `xml:"igroup-remove"`

	ForcePtr              *bool   `xml:"force"`
	InitiatorPtr          *string `xml:"initiator"`
	InitiatorGroupNamePtr *string `xml:"initiator-group-name"`
}

// ToXML converts this object into an xml string representation
func (o *IgroupRemoveRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewIgroupRemoveRequest is a factory method for creating new instances of IgroupRemoveRequest objects
func NewIgroupRemoveRequest() *IgroupRemoveRequest { return &IgroupRemoveRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *IgroupRemoveRequest) ExecuteUsing(zr *ZapiRunner) (IgroupRemoveResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "IgroupRemoveRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return IgroupRemoveResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return IgroupRemoveResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n IgroupRemoveResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return IgroupRemoveResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("igroup-remove result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupRemoveRequest) String() string {
	var buffer bytes.Buffer
	if o.ForcePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "force", *o.ForcePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("force: nil\n"))
	}
	if o.InitiatorPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "initiator", *o.InitiatorPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("initiator: nil\n"))
	}
	if o.InitiatorGroupNamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "initiator-group-name", *o.InitiatorGroupNamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("initiator-group-name: nil\n"))
	}
	return buffer.String()
}

// Force is a fluent style 'getter' method that can be chained
func (o *IgroupRemoveRequest) Force() bool {
	r := *o.ForcePtr
	return r
}

// SetForce is a fluent style 'setter' method that can be chained
func (o *IgroupRemoveRequest) SetForce(newValue bool) *IgroupRemoveRequest {
	o.ForcePtr = &newValue
	return o
}

// Initiator is a fluent style 'getter' method that can be chained
func (o *IgroupRemoveRequest) Initiator() string {
	r := *o.InitiatorPtr
	return r
}

// SetInitiator is a fluent style 'setter' method that can be chained
func (o *IgroupRemoveRequest) SetInitiator(newValue string) *IgroupRemoveRequest {
	o.InitiatorPtr = &newValue
	return o
}

// InitiatorGroupName is a fluent style 'getter' method that can be chained
func (o *IgroupRemoveRequest) InitiatorGroupName() string {
	r := *o.InitiatorGroupNamePtr
	return r
}

// SetInitiatorGroupName is a fluent style 'setter' method that can be chained
func (o *IgroupRemoveRequest) SetInitiatorGroupName(newValue string) *IgroupRemoveRequest {
	o.InitiatorGroupNamePtr = &newValue
	return o
}

// IgroupRemoveResponse is a structure to represent a igroup-remove ZAPI response object
type IgroupRemoveResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result IgroupRemoveResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupRemoveResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// IgroupRemoveResponseResult is a structure to represent a igroup-remove ZAPI object's result
type IgroupRemoveResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
}

// ToXML converts this object into an xml string representation
func (o *IgroupRemoveResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewIgroupRemoveResponse is a factory method for creating new instances of IgroupRemoveResponse objects
func NewIgroupRemoveResponse() *IgroupRemoveResponse { return &IgroupRemoveResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupRemoveResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	return buffer.String()
}
