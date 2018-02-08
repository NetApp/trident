// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/Sirupsen/logrus"
)

// QtreeCreateRequest is a structure to represent a qtree-create ZAPI request object
type QtreeCreateRequest struct {
	XMLName xml.Name `xml:"qtree-create"`

	ExportPolicyPtr  *string `xml:"export-policy"`
	ModePtr          *string `xml:"mode"`
	OplocksPtr       *string `xml:"oplocks"`
	QtreePtr         *string `xml:"qtree"`
	SecurityStylePtr *string `xml:"security-style"`
	VolumePtr        *string `xml:"volume"`
}

// ToXML converts this object into an xml string representation
func (o *QtreeCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewQtreeCreateRequest is a factory method for creating new instances of QtreeCreateRequest objects
func NewQtreeCreateRequest() *QtreeCreateRequest { return &QtreeCreateRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *QtreeCreateRequest) ExecuteUsing(zr *ZapiRunner) (QtreeCreateResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "QtreeCreateRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return QtreeCreateResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return QtreeCreateResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n QtreeCreateResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return QtreeCreateResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("qtree-create result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeCreateRequest) String() string {
	var buffer bytes.Buffer
	if o.ExportPolicyPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "export-policy", *o.ExportPolicyPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("export-policy: nil\n"))
	}
	if o.ModePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "mode", *o.ModePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("mode: nil\n"))
	}
	if o.OplocksPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "oplocks", *o.OplocksPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("oplocks: nil\n"))
	}
	if o.QtreePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "qtree", *o.QtreePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("qtree: nil\n"))
	}
	if o.SecurityStylePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "security-style", *o.SecurityStylePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("security-style: nil\n"))
	}
	if o.VolumePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume", *o.VolumePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume: nil\n"))
	}
	return buffer.String()
}

// ExportPolicy is a fluent style 'getter' method that can be chained
func (o *QtreeCreateRequest) ExportPolicy() string {
	r := *o.ExportPolicyPtr
	return r
}

// SetExportPolicy is a fluent style 'setter' method that can be chained
func (o *QtreeCreateRequest) SetExportPolicy(newValue string) *QtreeCreateRequest {
	o.ExportPolicyPtr = &newValue
	return o
}

// Mode is a fluent style 'getter' method that can be chained
func (o *QtreeCreateRequest) Mode() string {
	r := *o.ModePtr
	return r
}

// SetMode is a fluent style 'setter' method that can be chained
func (o *QtreeCreateRequest) SetMode(newValue string) *QtreeCreateRequest {
	o.ModePtr = &newValue
	return o
}

// Oplocks is a fluent style 'getter' method that can be chained
func (o *QtreeCreateRequest) Oplocks() string {
	r := *o.OplocksPtr
	return r
}

// SetOplocks is a fluent style 'setter' method that can be chained
func (o *QtreeCreateRequest) SetOplocks(newValue string) *QtreeCreateRequest {
	o.OplocksPtr = &newValue
	return o
}

// Qtree is a fluent style 'getter' method that can be chained
func (o *QtreeCreateRequest) Qtree() string {
	r := *o.QtreePtr
	return r
}

// SetQtree is a fluent style 'setter' method that can be chained
func (o *QtreeCreateRequest) SetQtree(newValue string) *QtreeCreateRequest {
	o.QtreePtr = &newValue
	return o
}

// SecurityStyle is a fluent style 'getter' method that can be chained
func (o *QtreeCreateRequest) SecurityStyle() string {
	r := *o.SecurityStylePtr
	return r
}

// SetSecurityStyle is a fluent style 'setter' method that can be chained
func (o *QtreeCreateRequest) SetSecurityStyle(newValue string) *QtreeCreateRequest {
	o.SecurityStylePtr = &newValue
	return o
}

// Volume is a fluent style 'getter' method that can be chained
func (o *QtreeCreateRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *QtreeCreateRequest) SetVolume(newValue string) *QtreeCreateRequest {
	o.VolumePtr = &newValue
	return o
}

// QtreeCreateResponse is a structure to represent a qtree-create ZAPI response object
type QtreeCreateResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result QtreeCreateResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeCreateResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// QtreeCreateResponseResult is a structure to represent a qtree-create ZAPI object's result
type QtreeCreateResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
}

// ToXML converts this object into an xml string representation
func (o *QtreeCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewQtreeCreateResponse is a factory method for creating new instances of QtreeCreateResponse objects
func NewQtreeCreateResponse() *QtreeCreateResponse { return &QtreeCreateResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeCreateResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	return buffer.String()
}
