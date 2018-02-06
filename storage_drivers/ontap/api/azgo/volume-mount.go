// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// VolumeMountRequest is a structure to represent a volume-mount ZAPI request object
type VolumeMountRequest struct {
	XMLName xml.Name `xml:"volume-mount"`

	ActivateJunctionPtr     *bool   `xml:"activate-junction"`
	ExportPolicyOverridePtr *bool   `xml:"export-policy-override"`
	JunctionPathPtr         *string `xml:"junction-path"`
	VolumeNamePtr           *string `xml:"volume-name"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeMountRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewVolumeMountRequest is a factory method for creating new instances of VolumeMountRequest objects
func NewVolumeMountRequest() *VolumeMountRequest { return &VolumeMountRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeMountRequest) ExecuteUsing(zr *ZapiRunner) (VolumeMountResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "VolumeMountRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return VolumeMountResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return VolumeMountResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n VolumeMountResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return VolumeMountResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("volume-mount result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeMountRequest) String() string {
	var buffer bytes.Buffer
	if o.ActivateJunctionPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "activate-junction", *o.ActivateJunctionPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("activate-junction: nil\n"))
	}
	if o.ExportPolicyOverridePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "export-policy-override", *o.ExportPolicyOverridePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("export-policy-override: nil\n"))
	}
	if o.JunctionPathPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "junction-path", *o.JunctionPathPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("junction-path: nil\n"))
	}
	if o.VolumeNamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume-name", *o.VolumeNamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume-name: nil\n"))
	}
	return buffer.String()
}

// ActivateJunction is a fluent style 'getter' method that can be chained
func (o *VolumeMountRequest) ActivateJunction() bool {
	r := *o.ActivateJunctionPtr
	return r
}

// SetActivateJunction is a fluent style 'setter' method that can be chained
func (o *VolumeMountRequest) SetActivateJunction(newValue bool) *VolumeMountRequest {
	o.ActivateJunctionPtr = &newValue
	return o
}

// ExportPolicyOverride is a fluent style 'getter' method that can be chained
func (o *VolumeMountRequest) ExportPolicyOverride() bool {
	r := *o.ExportPolicyOverridePtr
	return r
}

// SetExportPolicyOverride is a fluent style 'setter' method that can be chained
func (o *VolumeMountRequest) SetExportPolicyOverride(newValue bool) *VolumeMountRequest {
	o.ExportPolicyOverridePtr = &newValue
	return o
}

// JunctionPath is a fluent style 'getter' method that can be chained
func (o *VolumeMountRequest) JunctionPath() string {
	r := *o.JunctionPathPtr
	return r
}

// SetJunctionPath is a fluent style 'setter' method that can be chained
func (o *VolumeMountRequest) SetJunctionPath(newValue string) *VolumeMountRequest {
	o.JunctionPathPtr = &newValue
	return o
}

// VolumeName is a fluent style 'getter' method that can be chained
func (o *VolumeMountRequest) VolumeName() string {
	r := *o.VolumeNamePtr
	return r
}

// SetVolumeName is a fluent style 'setter' method that can be chained
func (o *VolumeMountRequest) SetVolumeName(newValue string) *VolumeMountRequest {
	o.VolumeNamePtr = &newValue
	return o
}

// VolumeMountResponse is a structure to represent a volume-mount ZAPI response object
type VolumeMountResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result VolumeMountResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeMountResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// VolumeMountResponseResult is a structure to represent a volume-mount ZAPI object's result
type VolumeMountResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeMountResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewVolumeMountResponse is a factory method for creating new instances of VolumeMountResponse objects
func NewVolumeMountResponse() *VolumeMountResponse { return &VolumeMountResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeMountResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	return buffer.String()
}
