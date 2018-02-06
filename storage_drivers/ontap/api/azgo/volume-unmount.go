// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// VolumeUnmountRequest is a structure to represent a volume-unmount ZAPI request object
type VolumeUnmountRequest struct {
	XMLName xml.Name `xml:"volume-unmount"`

	ForcePtr      *bool   `xml:"force"`
	VolumeNamePtr *string `xml:"volume-name"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeUnmountRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewVolumeUnmountRequest is a factory method for creating new instances of VolumeUnmountRequest objects
func NewVolumeUnmountRequest() *VolumeUnmountRequest { return &VolumeUnmountRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeUnmountRequest) ExecuteUsing(zr *ZapiRunner) (VolumeUnmountResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "VolumeUnmountRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return VolumeUnmountResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return VolumeUnmountResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n VolumeUnmountResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return VolumeUnmountResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("volume-unmount result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeUnmountRequest) String() string {
	var buffer bytes.Buffer
	if o.ForcePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "force", *o.ForcePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("force: nil\n"))
	}
	if o.VolumeNamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume-name", *o.VolumeNamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume-name: nil\n"))
	}
	return buffer.String()
}

// Force is a fluent style 'getter' method that can be chained
func (o *VolumeUnmountRequest) Force() bool {
	r := *o.ForcePtr
	return r
}

// SetForce is a fluent style 'setter' method that can be chained
func (o *VolumeUnmountRequest) SetForce(newValue bool) *VolumeUnmountRequest {
	o.ForcePtr = &newValue
	return o
}

// VolumeName is a fluent style 'getter' method that can be chained
func (o *VolumeUnmountRequest) VolumeName() string {
	r := *o.VolumeNamePtr
	return r
}

// SetVolumeName is a fluent style 'setter' method that can be chained
func (o *VolumeUnmountRequest) SetVolumeName(newValue string) *VolumeUnmountRequest {
	o.VolumeNamePtr = &newValue
	return o
}

// VolumeUnmountResponse is a structure to represent a volume-unmount ZAPI response object
type VolumeUnmountResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result VolumeUnmountResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeUnmountResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// VolumeUnmountResponseResult is a structure to represent a volume-unmount ZAPI object's result
type VolumeUnmountResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeUnmountResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewVolumeUnmountResponse is a factory method for creating new instances of VolumeUnmountResponse objects
func NewVolumeUnmountResponse() *VolumeUnmountResponse { return &VolumeUnmountResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeUnmountResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	return buffer.String()
}
