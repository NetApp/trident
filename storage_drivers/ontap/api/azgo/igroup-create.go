// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// IgroupCreateRequest is a structure to represent a igroup-create ZAPI request object
type IgroupCreateRequest struct {
	XMLName xml.Name `xml:"igroup-create"`

	BindPortsetPtr        *string `xml:"bind-portset"`
	InitiatorGroupNamePtr *string `xml:"initiator-group-name"`
	InitiatorGroupTypePtr *string `xml:"initiator-group-type"`
	OsTypePtr             *string `xml:"os-type"`
	OstypePtr             *string `xml:"ostype"`
}

// ToXML converts this object into an xml string representation
func (o *IgroupCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewIgroupCreateRequest is a factory method for creating new instances of IgroupCreateRequest objects
func NewIgroupCreateRequest() *IgroupCreateRequest { return &IgroupCreateRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *IgroupCreateRequest) ExecuteUsing(zr *ZapiRunner) (IgroupCreateResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "IgroupCreateRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return IgroupCreateResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return IgroupCreateResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n IgroupCreateResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return IgroupCreateResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("igroup-create result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupCreateRequest) String() string {
	var buffer bytes.Buffer
	if o.BindPortsetPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "bind-portset", *o.BindPortsetPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("bind-portset: nil\n"))
	}
	if o.InitiatorGroupNamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "initiator-group-name", *o.InitiatorGroupNamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("initiator-group-name: nil\n"))
	}
	if o.InitiatorGroupTypePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "initiator-group-type", *o.InitiatorGroupTypePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("initiator-group-type: nil\n"))
	}
	if o.OsTypePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "os-type", *o.OsTypePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("os-type: nil\n"))
	}
	if o.OstypePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "ostype", *o.OstypePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("ostype: nil\n"))
	}
	return buffer.String()
}

// BindPortset is a fluent style 'getter' method that can be chained
func (o *IgroupCreateRequest) BindPortset() string {
	r := *o.BindPortsetPtr
	return r
}

// SetBindPortset is a fluent style 'setter' method that can be chained
func (o *IgroupCreateRequest) SetBindPortset(newValue string) *IgroupCreateRequest {
	o.BindPortsetPtr = &newValue
	return o
}

// InitiatorGroupName is a fluent style 'getter' method that can be chained
func (o *IgroupCreateRequest) InitiatorGroupName() string {
	r := *o.InitiatorGroupNamePtr
	return r
}

// SetInitiatorGroupName is a fluent style 'setter' method that can be chained
func (o *IgroupCreateRequest) SetInitiatorGroupName(newValue string) *IgroupCreateRequest {
	o.InitiatorGroupNamePtr = &newValue
	return o
}

// InitiatorGroupType is a fluent style 'getter' method that can be chained
func (o *IgroupCreateRequest) InitiatorGroupType() string {
	r := *o.InitiatorGroupTypePtr
	return r
}

// SetInitiatorGroupType is a fluent style 'setter' method that can be chained
func (o *IgroupCreateRequest) SetInitiatorGroupType(newValue string) *IgroupCreateRequest {
	o.InitiatorGroupTypePtr = &newValue
	return o
}

// OsType is a fluent style 'getter' method that can be chained
func (o *IgroupCreateRequest) OsType() string {
	r := *o.OsTypePtr
	return r
}

// SetOsType is a fluent style 'setter' method that can be chained
func (o *IgroupCreateRequest) SetOsType(newValue string) *IgroupCreateRequest {
	o.OsTypePtr = &newValue
	return o
}

// Ostype is a fluent style 'getter' method that can be chained
func (o *IgroupCreateRequest) Ostype() string {
	r := *o.OstypePtr
	return r
}

// SetOstype is a fluent style 'setter' method that can be chained
func (o *IgroupCreateRequest) SetOstype(newValue string) *IgroupCreateRequest {
	o.OstypePtr = &newValue
	return o
}

// IgroupCreateResponse is a structure to represent a igroup-create ZAPI response object
type IgroupCreateResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result IgroupCreateResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupCreateResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// IgroupCreateResponseResult is a structure to represent a igroup-create ZAPI object's result
type IgroupCreateResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
}

// ToXML converts this object into an xml string representation
func (o *IgroupCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewIgroupCreateResponse is a factory method for creating new instances of IgroupCreateResponse objects
func NewIgroupCreateResponse() *IgroupCreateResponse { return &IgroupCreateResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupCreateResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	return buffer.String()
}
