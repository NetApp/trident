// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/Sirupsen/logrus"
)

// LunGetAttributeRequest is a structure to represent a lun-get-attribute ZAPI request object
type LunGetAttributeRequest struct {
	XMLName xml.Name `xml:"lun-get-attribute"`

	NamePtr *string `xml:"name"`
	PathPtr *string `xml:"path"`
}

// ToXML converts this object into an xml string representation
func (o *LunGetAttributeRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewLunGetAttributeRequest is a factory method for creating new instances of LunGetAttributeRequest objects
func NewLunGetAttributeRequest() *LunGetAttributeRequest { return &LunGetAttributeRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *LunGetAttributeRequest) ExecuteUsing(zr *ZapiRunner) (LunGetAttributeResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "LunGetAttributeRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return LunGetAttributeResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return LunGetAttributeResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n LunGetAttributeResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return LunGetAttributeResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("lun-get-attribute result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetAttributeRequest) String() string {
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
	return buffer.String()
}

// Name is a fluent style 'getter' method that can be chained
func (o *LunGetAttributeRequest) Name() string {
	r := *o.NamePtr
	return r
}

// SetName is a fluent style 'setter' method that can be chained
func (o *LunGetAttributeRequest) SetName(newValue string) *LunGetAttributeRequest {
	o.NamePtr = &newValue
	return o
}

// Path is a fluent style 'getter' method that can be chained
func (o *LunGetAttributeRequest) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunGetAttributeRequest) SetPath(newValue string) *LunGetAttributeRequest {
	o.PathPtr = &newValue
	return o
}

// LunGetAttributeResponse is a structure to represent a lun-get-attribute ZAPI response object
type LunGetAttributeResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result LunGetAttributeResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetAttributeResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// LunGetAttributeResponseResult is a structure to represent a lun-get-attribute ZAPI object's result
type LunGetAttributeResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string  `xml:"status,attr"`
	ResultReasonAttr string  `xml:"reason,attr"`
	ResultErrnoAttr  string  `xml:"errno,attr"`
	ValuePtr         *string `xml:"value"`
}

// ToXML converts this object into an xml string representation
func (o *LunGetAttributeResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewLunGetAttributeResponse is a factory method for creating new instances of LunGetAttributeResponse objects
func NewLunGetAttributeResponse() *LunGetAttributeResponse { return &LunGetAttributeResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetAttributeResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.ValuePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "value", *o.ValuePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("value: nil\n"))
	}
	return buffer.String()
}

// Value is a fluent style 'getter' method that can be chained
func (o *LunGetAttributeResponseResult) Value() string {
	r := *o.ValuePtr
	return r
}

// SetValue is a fluent style 'setter' method that can be chained
func (o *LunGetAttributeResponseResult) SetValue(newValue string) *LunGetAttributeResponseResult {
	o.ValuePtr = &newValue
	return o
}
