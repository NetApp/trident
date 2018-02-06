// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// LunMapListInfoRequest is a structure to represent a lun-map-list-info ZAPI request object
type LunMapListInfoRequest struct {
	XMLName xml.Name `xml:"lun-map-list-info"`

	PathPtr *string `xml:"path"`
}

// ToXML converts this object into an xml string representation
func (o *LunMapListInfoRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewLunMapListInfoRequest is a factory method for creating new instances of LunMapListInfoRequest objects
func NewLunMapListInfoRequest() *LunMapListInfoRequest { return &LunMapListInfoRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *LunMapListInfoRequest) ExecuteUsing(zr *ZapiRunner) (LunMapListInfoResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "LunMapListInfoRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return LunMapListInfoResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return LunMapListInfoResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n LunMapListInfoResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return LunMapListInfoResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("lun-map-list-info result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapListInfoRequest) String() string {
	var buffer bytes.Buffer
	if o.PathPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "path", *o.PathPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("path: nil\n"))
	}
	return buffer.String()
}

// Path is a fluent style 'getter' method that can be chained
func (o *LunMapListInfoRequest) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunMapListInfoRequest) SetPath(newValue string) *LunMapListInfoRequest {
	o.PathPtr = &newValue
	return o
}

// LunMapListInfoResponse is a structure to represent a lun-map-list-info ZAPI response object
type LunMapListInfoResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result LunMapListInfoResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapListInfoResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// LunMapListInfoResponseResult is a structure to represent a lun-map-list-info ZAPI object's result
type LunMapListInfoResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr   string                   `xml:"status,attr"`
	ResultReasonAttr   string                   `xml:"reason,attr"`
	ResultErrnoAttr    string                   `xml:"errno,attr"`
	InitiatorGroupsPtr []InitiatorGroupInfoType `xml:"initiator-groups>initiator-group-info"`
}

// ToXML converts this object into an xml string representation
func (o *LunMapListInfoResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewLunMapListInfoResponse is a factory method for creating new instances of LunMapListInfoResponse objects
func NewLunMapListInfoResponse() *LunMapListInfoResponse { return &LunMapListInfoResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapListInfoResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.InitiatorGroupsPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "initiator-groups", o.InitiatorGroupsPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("initiator-groups: nil\n"))
	}
	return buffer.String()
}

// InitiatorGroups is a fluent style 'getter' method that can be chained
func (o *LunMapListInfoResponseResult) InitiatorGroups() []InitiatorGroupInfoType {
	r := o.InitiatorGroupsPtr
	return r
}

// SetInitiatorGroups is a fluent style 'setter' method that can be chained
func (o *LunMapListInfoResponseResult) SetInitiatorGroups(newValue []InitiatorGroupInfoType) *LunMapListInfoResponseResult {
	newSlice := make([]InitiatorGroupInfoType, len(newValue))
	copy(newSlice, newValue)
	o.InitiatorGroupsPtr = newSlice
	return o
}
