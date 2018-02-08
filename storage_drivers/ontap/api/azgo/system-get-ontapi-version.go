// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/Sirupsen/logrus"
)

// SystemGetOntapiVersionRequest is a structure to represent a system-get-ontapi-version ZAPI request object
type SystemGetOntapiVersionRequest struct {
	XMLName xml.Name `xml:"system-get-ontapi-version"`
}

// ToXML converts this object into an xml string representation
func (o *SystemGetOntapiVersionRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewSystemGetOntapiVersionRequest is a factory method for creating new instances of SystemGetOntapiVersionRequest objects
func NewSystemGetOntapiVersionRequest() *SystemGetOntapiVersionRequest {
	return &SystemGetOntapiVersionRequest{}
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *SystemGetOntapiVersionRequest) ExecuteUsing(zr *ZapiRunner) (SystemGetOntapiVersionResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "SystemGetOntapiVersionRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return SystemGetOntapiVersionResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return SystemGetOntapiVersionResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n SystemGetOntapiVersionResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return SystemGetOntapiVersionResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("system-get-ontapi-version result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemGetOntapiVersionRequest) String() string {
	var buffer bytes.Buffer
	return buffer.String()
}

// SystemGetOntapiVersionResponse is a structure to represent a system-get-ontapi-version ZAPI response object
type SystemGetOntapiVersionResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result SystemGetOntapiVersionResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemGetOntapiVersionResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// SystemGetOntapiVersionResponseResult is a structure to represent a system-get-ontapi-version ZAPI object's result
type SystemGetOntapiVersionResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr     string                     `xml:"status,attr"`
	ResultReasonAttr     string                     `xml:"reason,attr"`
	ResultErrnoAttr      string                     `xml:"errno,attr"`
	MajorVersionPtr      *int                       `xml:"major-version"`
	MinorVersionPtr      *int                       `xml:"minor-version"`
	NodeOntapiDetailsPtr []NodeOntapiDetailInfoType `xml:"node-ontapi-details>node-ontapi-detail-info"`
}

// ToXML converts this object into an xml string representation
func (o *SystemGetOntapiVersionResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewSystemGetOntapiVersionResponse is a factory method for creating new instances of SystemGetOntapiVersionResponse objects
func NewSystemGetOntapiVersionResponse() *SystemGetOntapiVersionResponse {
	return &SystemGetOntapiVersionResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemGetOntapiVersionResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.MajorVersionPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "major-version", *o.MajorVersionPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("major-version: nil\n"))
	}
	if o.MinorVersionPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "minor-version", *o.MinorVersionPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("minor-version: nil\n"))
	}
	if o.NodeOntapiDetailsPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "node-ontapi-details", o.NodeOntapiDetailsPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("node-ontapi-details: nil\n"))
	}
	return buffer.String()
}

// MajorVersion is a fluent style 'getter' method that can be chained
func (o *SystemGetOntapiVersionResponseResult) MajorVersion() int {
	r := *o.MajorVersionPtr
	return r
}

// SetMajorVersion is a fluent style 'setter' method that can be chained
func (o *SystemGetOntapiVersionResponseResult) SetMajorVersion(newValue int) *SystemGetOntapiVersionResponseResult {
	o.MajorVersionPtr = &newValue
	return o
}

// MinorVersion is a fluent style 'getter' method that can be chained
func (o *SystemGetOntapiVersionResponseResult) MinorVersion() int {
	r := *o.MinorVersionPtr
	return r
}

// SetMinorVersion is a fluent style 'setter' method that can be chained
func (o *SystemGetOntapiVersionResponseResult) SetMinorVersion(newValue int) *SystemGetOntapiVersionResponseResult {
	o.MinorVersionPtr = &newValue
	return o
}

// NodeOntapiDetails is a fluent style 'getter' method that can be chained
func (o *SystemGetOntapiVersionResponseResult) NodeOntapiDetails() []NodeOntapiDetailInfoType {
	r := o.NodeOntapiDetailsPtr
	return r
}

// SetNodeOntapiDetails is a fluent style 'setter' method that can be chained
func (o *SystemGetOntapiVersionResponseResult) SetNodeOntapiDetails(newValue []NodeOntapiDetailInfoType) *SystemGetOntapiVersionResponseResult {
	newSlice := make([]NodeOntapiDetailInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NodeOntapiDetailsPtr = newSlice
	return o
}
