// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// IscsiNodeGetNameRequest is a structure to represent a iscsi-node-get-name ZAPI request object
type IscsiNodeGetNameRequest struct {
	XMLName xml.Name `xml:"iscsi-node-get-name"`
}

// ToXML converts this object into an xml string representation
func (o *IscsiNodeGetNameRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewIscsiNodeGetNameRequest is a factory method for creating new instances of IscsiNodeGetNameRequest objects
func NewIscsiNodeGetNameRequest() *IscsiNodeGetNameRequest { return &IscsiNodeGetNameRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *IscsiNodeGetNameRequest) ExecuteUsing(zr *ZapiRunner) (IscsiNodeGetNameResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "IscsiNodeGetNameRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return IscsiNodeGetNameResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return IscsiNodeGetNameResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n IscsiNodeGetNameResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return IscsiNodeGetNameResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("iscsi-node-get-name result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiNodeGetNameRequest) String() string {
	var buffer bytes.Buffer
	return buffer.String()
}

// IscsiNodeGetNameResponse is a structure to represent a iscsi-node-get-name ZAPI response object
type IscsiNodeGetNameResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result IscsiNodeGetNameResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiNodeGetNameResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// IscsiNodeGetNameResponseResult is a structure to represent a iscsi-node-get-name ZAPI object's result
type IscsiNodeGetNameResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string  `xml:"status,attr"`
	ResultReasonAttr string  `xml:"reason,attr"`
	ResultErrnoAttr  string  `xml:"errno,attr"`
	NodeNamePtr      *string `xml:"node-name"`
}

// ToXML converts this object into an xml string representation
func (o *IscsiNodeGetNameResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewIscsiNodeGetNameResponse is a factory method for creating new instances of IscsiNodeGetNameResponse objects
func NewIscsiNodeGetNameResponse() *IscsiNodeGetNameResponse { return &IscsiNodeGetNameResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiNodeGetNameResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.NodeNamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "node-name", *o.NodeNamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("node-name: nil\n"))
	}
	return buffer.String()
}

// NodeName is a fluent style 'getter' method that can be chained
func (o *IscsiNodeGetNameResponseResult) NodeName() string {
	r := *o.NodeNamePtr
	return r
}

// SetNodeName is a fluent style 'setter' method that can be chained
func (o *IscsiNodeGetNameResponseResult) SetNodeName(newValue string) *IscsiNodeGetNameResponseResult {
	o.NodeNamePtr = &newValue
	return o
}
