// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/Sirupsen/logrus"
)

// VolumeGetRootNameRequest is a structure to represent a volume-get-root-name ZAPI request object
type VolumeGetRootNameRequest struct {
	XMLName xml.Name `xml:"volume-get-root-name"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeGetRootNameRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewVolumeGetRootNameRequest is a factory method for creating new instances of VolumeGetRootNameRequest objects
func NewVolumeGetRootNameRequest() *VolumeGetRootNameRequest { return &VolumeGetRootNameRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeGetRootNameRequest) ExecuteUsing(zr *ZapiRunner) (VolumeGetRootNameResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "VolumeGetRootNameRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return VolumeGetRootNameResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return VolumeGetRootNameResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n VolumeGetRootNameResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return VolumeGetRootNameResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("volume-get-root-name result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeGetRootNameRequest) String() string {
	var buffer bytes.Buffer
	return buffer.String()
}

// VolumeGetRootNameResponse is a structure to represent a volume-get-root-name ZAPI response object
type VolumeGetRootNameResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result VolumeGetRootNameResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeGetRootNameResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// VolumeGetRootNameResponseResult is a structure to represent a volume-get-root-name ZAPI object's result
type VolumeGetRootNameResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string  `xml:"status,attr"`
	ResultReasonAttr string  `xml:"reason,attr"`
	ResultErrnoAttr  string  `xml:"errno,attr"`
	VolumePtr        *string `xml:"volume"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeGetRootNameResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewVolumeGetRootNameResponse is a factory method for creating new instances of VolumeGetRootNameResponse objects
func NewVolumeGetRootNameResponse() *VolumeGetRootNameResponse { return &VolumeGetRootNameResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeGetRootNameResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.VolumePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume", *o.VolumePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume: nil\n"))
	}
	return buffer.String()
}

// Volume is a fluent style 'getter' method that can be chained
func (o *VolumeGetRootNameResponseResult) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *VolumeGetRootNameResponseResult) SetVolume(newValue string) *VolumeGetRootNameResponseResult {
	o.VolumePtr = &newValue
	return o
}
