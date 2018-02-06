// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// VolumeDestroyRequest is a structure to represent a volume-destroy ZAPI request object
type VolumeDestroyRequest struct {
	XMLName xml.Name `xml:"volume-destroy"`

	NamePtr              *string `xml:"name"`
	UnmountAndOfflinePtr *bool   `xml:"unmount-and-offline"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeDestroyRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewVolumeDestroyRequest is a factory method for creating new instances of VolumeDestroyRequest objects
func NewVolumeDestroyRequest() *VolumeDestroyRequest { return &VolumeDestroyRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeDestroyRequest) ExecuteUsing(zr *ZapiRunner) (VolumeDestroyResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "VolumeDestroyRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return VolumeDestroyResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return VolumeDestroyResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n VolumeDestroyResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return VolumeDestroyResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("volume-destroy result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeDestroyRequest) String() string {
	var buffer bytes.Buffer
	if o.NamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "name", *o.NamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("name: nil\n"))
	}
	if o.UnmountAndOfflinePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "unmount-and-offline", *o.UnmountAndOfflinePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("unmount-and-offline: nil\n"))
	}
	return buffer.String()
}

// Name is a fluent style 'getter' method that can be chained
func (o *VolumeDestroyRequest) Name() string {
	r := *o.NamePtr
	return r
}

// SetName is a fluent style 'setter' method that can be chained
func (o *VolumeDestroyRequest) SetName(newValue string) *VolumeDestroyRequest {
	o.NamePtr = &newValue
	return o
}

// UnmountAndOffline is a fluent style 'getter' method that can be chained
func (o *VolumeDestroyRequest) UnmountAndOffline() bool {
	r := *o.UnmountAndOfflinePtr
	return r
}

// SetUnmountAndOffline is a fluent style 'setter' method that can be chained
func (o *VolumeDestroyRequest) SetUnmountAndOffline(newValue bool) *VolumeDestroyRequest {
	o.UnmountAndOfflinePtr = &newValue
	return o
}

// VolumeDestroyResponse is a structure to represent a volume-destroy ZAPI response object
type VolumeDestroyResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result VolumeDestroyResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeDestroyResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// VolumeDestroyResponseResult is a structure to represent a volume-destroy ZAPI object's result
type VolumeDestroyResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeDestroyResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewVolumeDestroyResponse is a factory method for creating new instances of VolumeDestroyResponse objects
func NewVolumeDestroyResponse() *VolumeDestroyResponse { return &VolumeDestroyResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeDestroyResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	return buffer.String()
}
