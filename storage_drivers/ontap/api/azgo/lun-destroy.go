// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/Sirupsen/logrus"
)

// LunDestroyRequest is a structure to represent a lun-destroy ZAPI request object
type LunDestroyRequest struct {
	XMLName xml.Name `xml:"lun-destroy"`

	DestroyFencedLunPtr *bool   `xml:"destroy-fenced-lun"`
	ForcePtr            *bool   `xml:"force"`
	PathPtr             *string `xml:"path"`
}

// ToXML converts this object into an xml string representation
func (o *LunDestroyRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewLunDestroyRequest is a factory method for creating new instances of LunDestroyRequest objects
func NewLunDestroyRequest() *LunDestroyRequest { return &LunDestroyRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *LunDestroyRequest) ExecuteUsing(zr *ZapiRunner) (LunDestroyResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "LunDestroyRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return LunDestroyResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return LunDestroyResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n LunDestroyResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return LunDestroyResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("lun-destroy result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunDestroyRequest) String() string {
	var buffer bytes.Buffer
	if o.DestroyFencedLunPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "destroy-fenced-lun", *o.DestroyFencedLunPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("destroy-fenced-lun: nil\n"))
	}
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
	return buffer.String()
}

// DestroyFencedLun is a fluent style 'getter' method that can be chained
func (o *LunDestroyRequest) DestroyFencedLun() bool {
	r := *o.DestroyFencedLunPtr
	return r
}

// SetDestroyFencedLun is a fluent style 'setter' method that can be chained
func (o *LunDestroyRequest) SetDestroyFencedLun(newValue bool) *LunDestroyRequest {
	o.DestroyFencedLunPtr = &newValue
	return o
}

// Force is a fluent style 'getter' method that can be chained
func (o *LunDestroyRequest) Force() bool {
	r := *o.ForcePtr
	return r
}

// SetForce is a fluent style 'setter' method that can be chained
func (o *LunDestroyRequest) SetForce(newValue bool) *LunDestroyRequest {
	o.ForcePtr = &newValue
	return o
}

// Path is a fluent style 'getter' method that can be chained
func (o *LunDestroyRequest) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunDestroyRequest) SetPath(newValue string) *LunDestroyRequest {
	o.PathPtr = &newValue
	return o
}

// LunDestroyResponse is a structure to represent a lun-destroy ZAPI response object
type LunDestroyResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result LunDestroyResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunDestroyResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// LunDestroyResponseResult is a structure to represent a lun-destroy ZAPI object's result
type LunDestroyResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
}

// ToXML converts this object into an xml string representation
func (o *LunDestroyResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewLunDestroyResponse is a factory method for creating new instances of LunDestroyResponse objects
func NewLunDestroyResponse() *LunDestroyResponse { return &LunDestroyResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunDestroyResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	return buffer.String()
}
