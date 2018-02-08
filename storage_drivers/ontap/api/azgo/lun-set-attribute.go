// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/Sirupsen/logrus"
)

// LunSetAttributeRequest is a structure to represent a lun-set-attribute ZAPI request object
type LunSetAttributeRequest struct {
	XMLName xml.Name `xml:"lun-set-attribute"`

	NamePtr  *string `xml:"name"`
	PathPtr  *string `xml:"path"`
	ValuePtr *string `xml:"value"`
}

// ToXML converts this object into an xml string representation
func (o *LunSetAttributeRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewLunSetAttributeRequest is a factory method for creating new instances of LunSetAttributeRequest objects
func NewLunSetAttributeRequest() *LunSetAttributeRequest { return &LunSetAttributeRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *LunSetAttributeRequest) ExecuteUsing(zr *ZapiRunner) (LunSetAttributeResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "LunSetAttributeRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return LunSetAttributeResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return LunSetAttributeResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n LunSetAttributeResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return LunSetAttributeResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("lun-set-attribute result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunSetAttributeRequest) String() string {
	var buffer bytes.Buffer
	if o.NamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "name", *o.NamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("name: nil\n"))
	}
	if o.PathPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "path", *o.PathPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("path: nil\n"))
	}
	if o.ValuePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "value", *o.ValuePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("value: nil\n"))
	}
	return buffer.String()
}

// Name is a fluent style 'getter' method that can be chained
func (o *LunSetAttributeRequest) Name() string {
	r := *o.NamePtr
	return r
}

// SetName is a fluent style 'setter' method that can be chained
func (o *LunSetAttributeRequest) SetName(newValue string) *LunSetAttributeRequest {
	o.NamePtr = &newValue
	return o
}

// Path is a fluent style 'getter' method that can be chained
func (o *LunSetAttributeRequest) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunSetAttributeRequest) SetPath(newValue string) *LunSetAttributeRequest {
	o.PathPtr = &newValue
	return o
}

// Value is a fluent style 'getter' method that can be chained
func (o *LunSetAttributeRequest) Value() string {
	r := *o.ValuePtr
	return r
}

// SetValue is a fluent style 'setter' method that can be chained
func (o *LunSetAttributeRequest) SetValue(newValue string) *LunSetAttributeRequest {
	o.ValuePtr = &newValue
	return o
}

// LunSetAttributeResponse is a structure to represent a lun-set-attribute ZAPI response object
type LunSetAttributeResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result LunSetAttributeResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunSetAttributeResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// LunSetAttributeResponseResult is a structure to represent a lun-set-attribute ZAPI object's result
type LunSetAttributeResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
}

// ToXML converts this object into an xml string representation
func (o *LunSetAttributeResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewLunSetAttributeResponse is a factory method for creating new instances of LunSetAttributeResponse objects
func NewLunSetAttributeResponse() *LunSetAttributeResponse { return &LunSetAttributeResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunSetAttributeResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	return buffer.String()
}
