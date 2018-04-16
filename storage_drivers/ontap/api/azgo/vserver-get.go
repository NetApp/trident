// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// VserverGetRequest is a structure to represent a vserver-get ZAPI request object
type VserverGetRequest struct {
	XMLName xml.Name `xml:"vserver-get"`

	DesiredAttributesPtr *VserverInfoType `xml:"desired-attributes>vserver-info"`
}

// ToXML converts this object into an xml string representation
func (o *VserverGetRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewVserverGetRequest is a factory method for creating new instances of VserverGetRequest objects
func NewVserverGetRequest() *VserverGetRequest { return &VserverGetRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VserverGetRequest) ExecuteUsing(zr *ZapiRunner) (VserverGetResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "VserverGetRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return VserverGetResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return VserverGetResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n VserverGetResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return VserverGetResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("vserver-get result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverGetRequest) String() string {
	var buffer bytes.Buffer
	if o.DesiredAttributesPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "desired-attributes", *o.DesiredAttributesPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("desired-attributes: nil\n"))
	}
	return buffer.String()
}

// DesiredAttributes is a fluent style 'getter' method that can be chained
func (o *VserverGetRequest) DesiredAttributes() VserverInfoType {
	r := *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *VserverGetRequest) SetDesiredAttributes(newValue VserverInfoType) *VserverGetRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// VserverGetResponse is a structure to represent a vserver-get ZAPI response object
type VserverGetResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result VserverGetResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverGetResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// VserverGetResponseResult is a structure to represent a vserver-get ZAPI object's result
type VserverGetResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string           `xml:"status,attr"`
	ResultReasonAttr string           `xml:"reason,attr"`
	ResultErrnoAttr  string           `xml:"errno,attr"`
	AttributesPtr    *VserverInfoType `xml:"attributes>vserver-info"`
}

// ToXML converts this object into an xml string representation
func (o *VserverGetResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewVserverGetResponse is a factory method for creating new instances of VserverGetResponse objects
func NewVserverGetResponse() *VserverGetResponse { return &VserverGetResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverGetResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.AttributesPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "attributes", *o.AttributesPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("attributes: nil\n"))
	}
	return buffer.String()
}

// Attributes is a fluent style 'getter' method that can be chained
func (o *VserverGetResponseResult) Attributes() VserverInfoType {
	r := *o.AttributesPtr
	return r
}

// SetAttributes is a fluent style 'setter' method that can be chained
func (o *VserverGetResponseResult) SetAttributes(newValue VserverInfoType) *VserverGetResponseResult {
	o.AttributesPtr = &newValue
	return o
}
