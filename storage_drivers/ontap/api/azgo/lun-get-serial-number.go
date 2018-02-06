// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// LunGetSerialNumberRequest is a structure to represent a lun-get-serial-number ZAPI request object
type LunGetSerialNumberRequest struct {
	XMLName xml.Name `xml:"lun-get-serial-number"`

	PathPtr *string `xml:"path"`
}

// ToXML converts this object into an xml string representation
func (o *LunGetSerialNumberRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewLunGetSerialNumberRequest is a factory method for creating new instances of LunGetSerialNumberRequest objects
func NewLunGetSerialNumberRequest() *LunGetSerialNumberRequest { return &LunGetSerialNumberRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *LunGetSerialNumberRequest) ExecuteUsing(zr *ZapiRunner) (LunGetSerialNumberResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "LunGetSerialNumberRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return LunGetSerialNumberResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return LunGetSerialNumberResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n LunGetSerialNumberResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return LunGetSerialNumberResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("lun-get-serial-number result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetSerialNumberRequest) String() string {
	var buffer bytes.Buffer
	if o.PathPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "path", *o.PathPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("path: nil\n"))
	}
	return buffer.String()
}

// Path is a fluent style 'getter' method that can be chained
func (o *LunGetSerialNumberRequest) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunGetSerialNumberRequest) SetPath(newValue string) *LunGetSerialNumberRequest {
	o.PathPtr = &newValue
	return o
}

// LunGetSerialNumberResponse is a structure to represent a lun-get-serial-number ZAPI response object
type LunGetSerialNumberResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result LunGetSerialNumberResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetSerialNumberResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// LunGetSerialNumberResponseResult is a structure to represent a lun-get-serial-number ZAPI object's result
type LunGetSerialNumberResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string  `xml:"status,attr"`
	ResultReasonAttr string  `xml:"reason,attr"`
	ResultErrnoAttr  string  `xml:"errno,attr"`
	SerialNumberPtr  *string `xml:"serial-number"`
}

// ToXML converts this object into an xml string representation
func (o *LunGetSerialNumberResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewLunGetSerialNumberResponse is a factory method for creating new instances of LunGetSerialNumberResponse objects
func NewLunGetSerialNumberResponse() *LunGetSerialNumberResponse { return &LunGetSerialNumberResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetSerialNumberResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.SerialNumberPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "serial-number", *o.SerialNumberPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("serial-number: nil\n"))
	}
	return buffer.String()
}

// SerialNumber is a fluent style 'getter' method that can be chained
func (o *LunGetSerialNumberResponseResult) SerialNumber() string {
	r := *o.SerialNumberPtr
	return r
}

// SetSerialNumber is a fluent style 'setter' method that can be chained
func (o *LunGetSerialNumberResponseResult) SetSerialNumber(newValue string) *LunGetSerialNumberResponseResult {
	o.SerialNumberPtr = &newValue
	return o
}
