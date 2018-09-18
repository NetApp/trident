// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// LunResizeRequest is a structure to represent a lun-resize ZAPI request object
type LunResizeRequest struct {
	XMLName xml.Name `xml:"lun-resize"`

	ForcePtr *bool   `xml:"force"`
	PathPtr  *string `xml:"path"`
	SizePtr  *int    `xml:"size"`
}

// ToXML converts this object into an xml string representation
func (o *LunResizeRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewLunResizeRequest is a factory method for creating new instances of LunResizeRequest objects
func NewLunResizeRequest() *LunResizeRequest { return &LunResizeRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *LunResizeRequest) ExecuteUsing(zr *ZapiRunner) (LunResizeResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "LunResizeRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return LunResizeResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return LunResizeResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n LunResizeResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return LunResizeResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("lun-resize result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunResizeRequest) String() string {
	var buffer bytes.Buffer
	if o.ForcePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "force", *o.ForcePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("force: nil\n"))
	}
	if o.PathPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "path", *o.PathPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("path: nil\n"))
	}
	if o.SizePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "size", *o.SizePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("size: nil\n"))
	}
	return buffer.String()
}

// Force is a fluent style 'getter' method that can be chained
func (o *LunResizeRequest) Force() bool {
	r := *o.ForcePtr
	return r
}

// SetForce is a fluent style 'setter' method that can be chained
func (o *LunResizeRequest) SetForce(newValue bool) *LunResizeRequest {
	o.ForcePtr = &newValue
	return o
}

// Path is a fluent style 'getter' method that can be chained
func (o *LunResizeRequest) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunResizeRequest) SetPath(newValue string) *LunResizeRequest {
	o.PathPtr = &newValue
	return o
}

// Size is a fluent style 'getter' method that can be chained
func (o *LunResizeRequest) Size() int {
	r := *o.SizePtr
	return r
}

// SetSize is a fluent style 'setter' method that can be chained
func (o *LunResizeRequest) SetSize(newValue int) *LunResizeRequest {
	o.SizePtr = &newValue
	return o
}

// LunResizeResponse is a structure to represent a lun-resize ZAPI response object
type LunResizeResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result LunResizeResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunResizeResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// LunResizeResponseResult is a structure to represent a lun-resize ZAPI object's result
type LunResizeResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
	ActualSizePtr    *int   `xml:"actual-size"`
}

// ToXML converts this object into an xml string representation
func (o *LunResizeResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewLunResizeResponse is a factory method for creating new instances of LunResizeResponse objects
func NewLunResizeResponse() *LunResizeResponse { return &LunResizeResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunResizeResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.ActualSizePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "actual-size", *o.ActualSizePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("actual-size: nil\n"))
	}
	return buffer.String()
}

// ActualSize is a fluent style 'getter' method that can be chained
func (o *LunResizeResponseResult) ActualSize() int {
	r := *o.ActualSizePtr
	return r
}

// SetActualSize is a fluent style 'setter' method that can be chained
func (o *LunResizeResponseResult) SetActualSize(newValue int) *LunResizeResponseResult {
	o.ActualSizePtr = &newValue
	return o
}
