// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/Sirupsen/logrus"
)

// VolumeOfflineRequest is a structure to represent a volume-offline ZAPI request object
type VolumeOfflineRequest struct {
	XMLName xml.Name `xml:"volume-offline"`

	NamePtr *string `xml:"name"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeOfflineRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewVolumeOfflineRequest is a factory method for creating new instances of VolumeOfflineRequest objects
func NewVolumeOfflineRequest() *VolumeOfflineRequest { return &VolumeOfflineRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeOfflineRequest) ExecuteUsing(zr *ZapiRunner) (VolumeOfflineResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "VolumeOfflineRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return VolumeOfflineResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return VolumeOfflineResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n VolumeOfflineResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return VolumeOfflineResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("volume-offline result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeOfflineRequest) String() string {
	var buffer bytes.Buffer
	if o.NamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "name", *o.NamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("name: nil\n"))
	}
	return buffer.String()
}

// Name is a fluent style 'getter' method that can be chained
func (o *VolumeOfflineRequest) Name() string {
	r := *o.NamePtr
	return r
}

// SetName is a fluent style 'setter' method that can be chained
func (o *VolumeOfflineRequest) SetName(newValue string) *VolumeOfflineRequest {
	o.NamePtr = &newValue
	return o
}

// VolumeOfflineResponse is a structure to represent a volume-offline ZAPI response object
type VolumeOfflineResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result VolumeOfflineResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeOfflineResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// VolumeOfflineResponseResult is a structure to represent a volume-offline ZAPI object's result
type VolumeOfflineResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeOfflineResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewVolumeOfflineResponse is a factory method for creating new instances of VolumeOfflineResponse objects
func NewVolumeOfflineResponse() *VolumeOfflineResponse { return &VolumeOfflineResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeOfflineResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	return buffer.String()
}
