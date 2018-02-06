// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// VolumeCloneSplitStartRequest is a structure to represent a volume-clone-split-start ZAPI request object
type VolumeCloneSplitStartRequest struct {
	XMLName xml.Name `xml:"volume-clone-split-start"`

	VolumePtr *string `xml:"volume"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeCloneSplitStartRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewVolumeCloneSplitStartRequest is a factory method for creating new instances of VolumeCloneSplitStartRequest objects
func NewVolumeCloneSplitStartRequest() *VolumeCloneSplitStartRequest {
	return &VolumeCloneSplitStartRequest{}
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeCloneSplitStartRequest) ExecuteUsing(zr *ZapiRunner) (VolumeCloneSplitStartResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "VolumeCloneSplitStartRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return VolumeCloneSplitStartResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return VolumeCloneSplitStartResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n VolumeCloneSplitStartResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return VolumeCloneSplitStartResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("volume-clone-split-start result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneSplitStartRequest) String() string {
	var buffer bytes.Buffer
	if o.VolumePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume", *o.VolumePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume: nil\n"))
	}
	return buffer.String()
}

// Volume is a fluent style 'getter' method that can be chained
func (o *VolumeCloneSplitStartRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *VolumeCloneSplitStartRequest) SetVolume(newValue string) *VolumeCloneSplitStartRequest {
	o.VolumePtr = &newValue
	return o
}

// VolumeCloneSplitStartResponse is a structure to represent a volume-clone-split-start ZAPI response object
type VolumeCloneSplitStartResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result VolumeCloneSplitStartResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneSplitStartResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// VolumeCloneSplitStartResponseResult is a structure to represent a volume-clone-split-start ZAPI object's result
type VolumeCloneSplitStartResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr      string  `xml:"status,attr"`
	ResultReasonAttr      string  `xml:"reason,attr"`
	ResultErrnoAttr       string  `xml:"errno,attr"`
	ResultErrorCodePtr    *int    `xml:"result-error-code"`
	ResultErrorMessagePtr *string `xml:"result-error-message"`
	ResultJobidPtr        *int    `xml:"result-jobid"`
	ResultStatusPtr       *string `xml:"result-status"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeCloneSplitStartResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewVolumeCloneSplitStartResponse is a factory method for creating new instances of VolumeCloneSplitStartResponse objects
func NewVolumeCloneSplitStartResponse() *VolumeCloneSplitStartResponse {
	return &VolumeCloneSplitStartResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneSplitStartResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.ResultErrorCodePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "result-error-code", *o.ResultErrorCodePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("result-error-code: nil\n"))
	}
	if o.ResultErrorMessagePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "result-error-message", *o.ResultErrorMessagePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("result-error-message: nil\n"))
	}
	if o.ResultJobidPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "result-jobid", *o.ResultJobidPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("result-jobid: nil\n"))
	}
	if o.ResultStatusPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "result-status", *o.ResultStatusPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("result-status: nil\n"))
	}
	return buffer.String()
}

// ResultErrorCode is a fluent style 'getter' method that can be chained
func (o *VolumeCloneSplitStartResponseResult) ResultErrorCode() int {
	r := *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *VolumeCloneSplitStartResponseResult) SetResultErrorCode(newValue int) *VolumeCloneSplitStartResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a fluent style 'getter' method that can be chained
func (o *VolumeCloneSplitStartResponseResult) ResultErrorMessage() string {
	r := *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *VolumeCloneSplitStartResponseResult) SetResultErrorMessage(newValue string) *VolumeCloneSplitStartResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a fluent style 'getter' method that can be chained
func (o *VolumeCloneSplitStartResponseResult) ResultJobid() int {
	r := *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *VolumeCloneSplitStartResponseResult) SetResultJobid(newValue int) *VolumeCloneSplitStartResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultStatus is a fluent style 'getter' method that can be chained
func (o *VolumeCloneSplitStartResponseResult) ResultStatus() string {
	r := *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *VolumeCloneSplitStartResponseResult) SetResultStatus(newValue string) *VolumeCloneSplitStartResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}
