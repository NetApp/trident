// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/Sirupsen/logrus"
)

// LunMapRequest is a structure to represent a lun-map ZAPI request object
type LunMapRequest struct {
	XMLName xml.Name `xml:"lun-map"`

	AdditionalReportingNodePtr *NodeNameType `xml:"additional-reporting-node>node-name"`
	ForcePtr                   *bool         `xml:"force"`
	InitiatorGroupPtr          *string       `xml:"initiator-group"`
	LunIdPtr                   *int          `xml:"lun-id"`
	PathPtr                    *string       `xml:"path"`
}

// ToXML converts this object into an xml string representation
func (o *LunMapRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewLunMapRequest is a factory method for creating new instances of LunMapRequest objects
func NewLunMapRequest() *LunMapRequest { return &LunMapRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *LunMapRequest) ExecuteUsing(zr *ZapiRunner) (LunMapResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "LunMapRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return LunMapResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return LunMapResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n LunMapResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return LunMapResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("lun-map result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapRequest) String() string {
	var buffer bytes.Buffer
	if o.AdditionalReportingNodePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "additional-reporting-node", *o.AdditionalReportingNodePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("additional-reporting-node: nil\n"))
	}
	if o.ForcePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "force", *o.ForcePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("force: nil\n"))
	}
	if o.InitiatorGroupPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "initiator-group", *o.InitiatorGroupPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("initiator-group: nil\n"))
	}
	if o.LunIdPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "lun-id", *o.LunIdPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("lun-id: nil\n"))
	}
	if o.PathPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "path", *o.PathPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("path: nil\n"))
	}
	return buffer.String()
}

// AdditionalReportingNode is a fluent style 'getter' method that can be chained
func (o *LunMapRequest) AdditionalReportingNode() NodeNameType {
	r := *o.AdditionalReportingNodePtr
	return r
}

// SetAdditionalReportingNode is a fluent style 'setter' method that can be chained
func (o *LunMapRequest) SetAdditionalReportingNode(newValue NodeNameType) *LunMapRequest {
	o.AdditionalReportingNodePtr = &newValue
	return o
}

// Force is a fluent style 'getter' method that can be chained
func (o *LunMapRequest) Force() bool {
	r := *o.ForcePtr
	return r
}

// SetForce is a fluent style 'setter' method that can be chained
func (o *LunMapRequest) SetForce(newValue bool) *LunMapRequest {
	o.ForcePtr = &newValue
	return o
}

// InitiatorGroup is a fluent style 'getter' method that can be chained
func (o *LunMapRequest) InitiatorGroup() string {
	r := *o.InitiatorGroupPtr
	return r
}

// SetInitiatorGroup is a fluent style 'setter' method that can be chained
func (o *LunMapRequest) SetInitiatorGroup(newValue string) *LunMapRequest {
	o.InitiatorGroupPtr = &newValue
	return o
}

// LunId is a fluent style 'getter' method that can be chained
func (o *LunMapRequest) LunId() int {
	r := *o.LunIdPtr
	return r
}

// SetLunId is a fluent style 'setter' method that can be chained
func (o *LunMapRequest) SetLunId(newValue int) *LunMapRequest {
	o.LunIdPtr = &newValue
	return o
}

// Path is a fluent style 'getter' method that can be chained
func (o *LunMapRequest) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunMapRequest) SetPath(newValue string) *LunMapRequest {
	o.PathPtr = &newValue
	return o
}

// LunMapResponse is a structure to represent a lun-map ZAPI response object
type LunMapResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result LunMapResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// LunMapResponseResult is a structure to represent a lun-map ZAPI object's result
type LunMapResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
	LunIdAssignedPtr *int   `xml:"lun-id-assigned"`
}

// ToXML converts this object into an xml string representation
func (o *LunMapResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewLunMapResponse is a factory method for creating new instances of LunMapResponse objects
func NewLunMapResponse() *LunMapResponse { return &LunMapResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.LunIdAssignedPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "lun-id-assigned", *o.LunIdAssignedPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("lun-id-assigned: nil\n"))
	}
	return buffer.String()
}

// LunIdAssigned is a fluent style 'getter' method that can be chained
func (o *LunMapResponseResult) LunIdAssigned() int {
	r := *o.LunIdAssignedPtr
	return r
}

// SetLunIdAssigned is a fluent style 'setter' method that can be chained
func (o *LunMapResponseResult) SetLunIdAssigned(newValue int) *LunMapResponseResult {
	o.LunIdAssignedPtr = &newValue
	return o
}
