// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// SystemGetVersionRequest is a structure to represent a system-get-version ZAPI request object
type SystemGetVersionRequest struct {
	XMLName xml.Name `xml:"system-get-version"`
}

// ToXML converts this object into an xml string representation
func (o *SystemGetVersionRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewSystemGetVersionRequest is a factory method for creating new instances of SystemGetVersionRequest objects
func NewSystemGetVersionRequest() *SystemGetVersionRequest { return &SystemGetVersionRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *SystemGetVersionRequest) ExecuteUsing(zr *ZapiRunner) (SystemGetVersionResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "SystemGetVersionRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return SystemGetVersionResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return SystemGetVersionResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n SystemGetVersionResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return SystemGetVersionResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("system-get-version result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemGetVersionRequest) String() string {
	var buffer bytes.Buffer
	return buffer.String()
}

// SystemGetVersionResponse is a structure to represent a system-get-version ZAPI response object
type SystemGetVersionResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result SystemGetVersionResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemGetVersionResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// SystemGetVersionResponseResult is a structure to represent a system-get-version ZAPI object's result
type SystemGetVersionResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr      string                      `xml:"status,attr"`
	ResultReasonAttr      string                      `xml:"reason,attr"`
	ResultErrnoAttr       string                      `xml:"errno,attr"`
	BuildTimestampPtr     *int                        `xml:"build-timestamp"`
	IsClusteredPtr        *bool                       `xml:"is-clustered"`
	NodeVersionDetailsPtr []NodeVersionDetailInfoType `xml:"node-version-details>node-version-detail-info"`
	VersionPtr            *string                     `xml:"version"`
	VersionTuplePtr       *SystemVersionTupleType     `xml:"version-tuple>system-version-tuple"`
}

// ToXML converts this object into an xml string representation
func (o *SystemGetVersionResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewSystemGetVersionResponse is a factory method for creating new instances of SystemGetVersionResponse objects
func NewSystemGetVersionResponse() *SystemGetVersionResponse { return &SystemGetVersionResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemGetVersionResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.BuildTimestampPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "build-timestamp", *o.BuildTimestampPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("build-timestamp: nil\n"))
	}
	if o.IsClusteredPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "is-clustered", *o.IsClusteredPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("is-clustered: nil\n"))
	}
	if o.NodeVersionDetailsPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "node-version-details", o.NodeVersionDetailsPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("node-version-details: nil\n"))
	}
	if o.VersionPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "version", *o.VersionPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("version: nil\n"))
	}
	if o.VersionTuplePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "version-tuple", *o.VersionTuplePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("version-tuple: nil\n"))
	}
	return buffer.String()
}

// BuildTimestamp is a fluent style 'getter' method that can be chained
func (o *SystemGetVersionResponseResult) BuildTimestamp() int {
	r := *o.BuildTimestampPtr
	return r
}

// SetBuildTimestamp is a fluent style 'setter' method that can be chained
func (o *SystemGetVersionResponseResult) SetBuildTimestamp(newValue int) *SystemGetVersionResponseResult {
	o.BuildTimestampPtr = &newValue
	return o
}

// IsClustered is a fluent style 'getter' method that can be chained
func (o *SystemGetVersionResponseResult) IsClustered() bool {
	r := *o.IsClusteredPtr
	return r
}

// SetIsClustered is a fluent style 'setter' method that can be chained
func (o *SystemGetVersionResponseResult) SetIsClustered(newValue bool) *SystemGetVersionResponseResult {
	o.IsClusteredPtr = &newValue
	return o
}

// NodeVersionDetails is a fluent style 'getter' method that can be chained
func (o *SystemGetVersionResponseResult) NodeVersionDetails() []NodeVersionDetailInfoType {
	r := o.NodeVersionDetailsPtr
	return r
}

// SetNodeVersionDetails is a fluent style 'setter' method that can be chained
func (o *SystemGetVersionResponseResult) SetNodeVersionDetails(newValue []NodeVersionDetailInfoType) *SystemGetVersionResponseResult {
	newSlice := make([]NodeVersionDetailInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NodeVersionDetailsPtr = newSlice
	return o
}

// Version is a fluent style 'getter' method that can be chained
func (o *SystemGetVersionResponseResult) Version() string {
	r := *o.VersionPtr
	return r
}

// SetVersion is a fluent style 'setter' method that can be chained
func (o *SystemGetVersionResponseResult) SetVersion(newValue string) *SystemGetVersionResponseResult {
	o.VersionPtr = &newValue
	return o
}

// VersionTuple is a fluent style 'getter' method that can be chained
func (o *SystemGetVersionResponseResult) VersionTuple() SystemVersionTupleType {
	r := *o.VersionTuplePtr
	return r
}

// SetVersionTuple is a fluent style 'setter' method that can be chained
func (o *SystemGetVersionResponseResult) SetVersionTuple(newValue SystemVersionTupleType) *SystemGetVersionResponseResult {
	o.VersionTuplePtr = &newValue
	return o
}
