// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/Sirupsen/logrus"
)

// IgroupAddRequest is a structure to represent a igroup-add ZAPI request object
type IgroupAddRequest struct {
	XMLName xml.Name `xml:"igroup-add"`

	ForcePtr              *bool   `xml:"force"`
	InitiatorPtr          *string `xml:"initiator"`
	InitiatorGroupNamePtr *string `xml:"initiator-group-name"`
}

// ToXML converts this object into an xml string representation
func (o *IgroupAddRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewIgroupAddRequest is a factory method for creating new instances of IgroupAddRequest objects
func NewIgroupAddRequest() *IgroupAddRequest { return &IgroupAddRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *IgroupAddRequest) ExecuteUsing(zr *ZapiRunner) (IgroupAddResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "IgroupAddRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return IgroupAddResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return IgroupAddResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n IgroupAddResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return IgroupAddResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("igroup-add result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupAddRequest) String() string {
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
func (o *IgroupAddRequest) Force() bool {
	r := *o.ForcePtr
	return r
}

// SetForce is a fluent style 'setter' method that can be chained
func (o *IgroupAddRequest) SetForce(newValue bool) *IgroupAddRequest {
	o.ForcePtr = &newValue
	return o
}

// Initiator is a fluent style 'getter' method that can be chained
func (o *IgroupAddRequest) Initiator() string {
	r := *o.InitiatorPtr
	return r
}

// SetInitiator is a fluent style 'setter' method that can be chained
func (o *IgroupAddRequest) SetInitiator(newValue string) *IgroupAddRequest {
	o.InitiatorPtr = &newValue
	return o
}

// InitiatorGroupName is a fluent style 'getter' method that can be chained
func (o *IgroupAddRequest) InitiatorGroupName() string {
	r := *o.InitiatorGroupNamePtr
	return r
}

// SetInitiatorGroupName is a fluent style 'setter' method that can be chained
func (o *IgroupAddRequest) SetInitiatorGroupName(newValue string) *IgroupAddRequest {
	o.InitiatorGroupNamePtr = &newValue
	return o
}

// IgroupAddResponse is a structure to represent a igroup-add ZAPI response object
type IgroupAddResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result IgroupAddResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupAddResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// IgroupAddResponseResult is a structure to represent a igroup-add ZAPI object's result
type IgroupAddResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
}

// ToXML converts this object into an xml string representation
func (o *IgroupAddResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewIgroupAddResponse is a factory method for creating new instances of IgroupAddResponse objects
func NewIgroupAddResponse() *IgroupAddResponse { return &IgroupAddResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupAddResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	return buffer.String()
}
