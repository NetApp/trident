// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// IgroupDestroyRequest is a structure to represent a igroup-destroy ZAPI request object
type IgroupDestroyRequest struct {
	XMLName xml.Name `xml:"igroup-destroy"`

	ForcePtr              *bool   `xml:"force"`
	InitiatorGroupNamePtr *string `xml:"initiator-group-name"`
}

// ToXML converts this object into an xml string representation
func (o *IgroupDestroyRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewIgroupDestroyRequest is a factory method for creating new instances of IgroupDestroyRequest objects
func NewIgroupDestroyRequest() *IgroupDestroyRequest { return &IgroupDestroyRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *IgroupDestroyRequest) ExecuteUsing(zr *ZapiRunner) (IgroupDestroyResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "IgroupDestroyRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return IgroupDestroyResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return IgroupDestroyResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n IgroupDestroyResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return IgroupDestroyResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("igroup-destroy result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupDestroyRequest) String() string {
	var buffer bytes.Buffer
	if o.ForcePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "force", *o.ForcePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("force: nil\n"))
	}
	if o.InitiatorGroupNamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "initiator-group-name", *o.InitiatorGroupNamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("initiator-group-name: nil\n"))
	}
	return buffer.String()
}

// Force is a fluent style 'getter' method that can be chained
func (o *IgroupDestroyRequest) Force() bool {
	r := *o.ForcePtr
	return r
}

// SetForce is a fluent style 'setter' method that can be chained
func (o *IgroupDestroyRequest) SetForce(newValue bool) *IgroupDestroyRequest {
	o.ForcePtr = &newValue
	return o
}

// InitiatorGroupName is a fluent style 'getter' method that can be chained
func (o *IgroupDestroyRequest) InitiatorGroupName() string {
	r := *o.InitiatorGroupNamePtr
	return r
}

// SetInitiatorGroupName is a fluent style 'setter' method that can be chained
func (o *IgroupDestroyRequest) SetInitiatorGroupName(newValue string) *IgroupDestroyRequest {
	o.InitiatorGroupNamePtr = &newValue
	return o
}

// IgroupDestroyResponse is a structure to represent a igroup-destroy ZAPI response object
type IgroupDestroyResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result IgroupDestroyResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupDestroyResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// IgroupDestroyResponseResult is a structure to represent a igroup-destroy ZAPI object's result
type IgroupDestroyResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
}

// ToXML converts this object into an xml string representation
func (o *IgroupDestroyResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewIgroupDestroyResponse is a factory method for creating new instances of IgroupDestroyResponse objects
func NewIgroupDestroyResponse() *IgroupDestroyResponse { return &IgroupDestroyResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupDestroyResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	return buffer.String()
}
