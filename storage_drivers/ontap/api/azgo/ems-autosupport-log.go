// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// EmsAutosupportLogRequest is a structure to represent a ems-autosupport-log ZAPI request object
type EmsAutosupportLogRequest struct {
	XMLName xml.Name `xml:"ems-autosupport-log"`

	AppVersionPtr       *string `xml:"app-version"`
	AutoSupportPtr      *bool   `xml:"auto-support"`
	CategoryPtr         *string `xml:"category"`
	ComputerNamePtr     *string `xml:"computer-name"`
	EventDescriptionPtr *string `xml:"event-description"`
	EventIdPtr          *int    `xml:"event-id"`
	EventSourcePtr      *string `xml:"event-source"`
	LogLevelPtr         *int    `xml:"log-level"`
}

// ToXML converts this object into an xml string representation
func (o *EmsAutosupportLogRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewEmsAutosupportLogRequest is a factory method for creating new instances of EmsAutosupportLogRequest objects
func NewEmsAutosupportLogRequest() *EmsAutosupportLogRequest { return &EmsAutosupportLogRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *EmsAutosupportLogRequest) ExecuteUsing(zr *ZapiRunner) (EmsAutosupportLogResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "EmsAutosupportLogRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return EmsAutosupportLogResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return EmsAutosupportLogResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n EmsAutosupportLogResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return EmsAutosupportLogResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("ems-autosupport-log result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o EmsAutosupportLogRequest) String() string {
	var buffer bytes.Buffer
	if o.AppVersionPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "app-version", *o.AppVersionPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("app-version: nil\n"))
	}
	if o.AutoSupportPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "auto-support", *o.AutoSupportPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("auto-support: nil\n"))
	}
	if o.CategoryPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "category", *o.CategoryPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("category: nil\n"))
	}
	if o.ComputerNamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "computer-name", *o.ComputerNamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("computer-name: nil\n"))
	}
	if o.EventDescriptionPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "event-description", *o.EventDescriptionPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("event-description: nil\n"))
	}
	if o.EventIdPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "event-id", *o.EventIdPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("event-id: nil\n"))
	}
	if o.EventSourcePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "event-source", *o.EventSourcePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("event-source: nil\n"))
	}
	if o.LogLevelPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "log-level", *o.LogLevelPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("log-level: nil\n"))
	}
	return buffer.String()
}

// AppVersion is a fluent style 'getter' method that can be chained
func (o *EmsAutosupportLogRequest) AppVersion() string {
	r := *o.AppVersionPtr
	return r
}

// SetAppVersion is a fluent style 'setter' method that can be chained
func (o *EmsAutosupportLogRequest) SetAppVersion(newValue string) *EmsAutosupportLogRequest {
	o.AppVersionPtr = &newValue
	return o
}

// AutoSupport is a fluent style 'getter' method that can be chained
func (o *EmsAutosupportLogRequest) AutoSupport() bool {
	r := *o.AutoSupportPtr
	return r
}

// SetAutoSupport is a fluent style 'setter' method that can be chained
func (o *EmsAutosupportLogRequest) SetAutoSupport(newValue bool) *EmsAutosupportLogRequest {
	o.AutoSupportPtr = &newValue
	return o
}

// Category is a fluent style 'getter' method that can be chained
func (o *EmsAutosupportLogRequest) Category() string {
	r := *o.CategoryPtr
	return r
}

// SetCategory is a fluent style 'setter' method that can be chained
func (o *EmsAutosupportLogRequest) SetCategory(newValue string) *EmsAutosupportLogRequest {
	o.CategoryPtr = &newValue
	return o
}

// ComputerName is a fluent style 'getter' method that can be chained
func (o *EmsAutosupportLogRequest) ComputerName() string {
	r := *o.ComputerNamePtr
	return r
}

// SetComputerName is a fluent style 'setter' method that can be chained
func (o *EmsAutosupportLogRequest) SetComputerName(newValue string) *EmsAutosupportLogRequest {
	o.ComputerNamePtr = &newValue
	return o
}

// EventDescription is a fluent style 'getter' method that can be chained
func (o *EmsAutosupportLogRequest) EventDescription() string {
	r := *o.EventDescriptionPtr
	return r
}

// SetEventDescription is a fluent style 'setter' method that can be chained
func (o *EmsAutosupportLogRequest) SetEventDescription(newValue string) *EmsAutosupportLogRequest {
	o.EventDescriptionPtr = &newValue
	return o
}

// EventId is a fluent style 'getter' method that can be chained
func (o *EmsAutosupportLogRequest) EventId() int {
	r := *o.EventIdPtr
	return r
}

// SetEventId is a fluent style 'setter' method that can be chained
func (o *EmsAutosupportLogRequest) SetEventId(newValue int) *EmsAutosupportLogRequest {
	o.EventIdPtr = &newValue
	return o
}

// EventSource is a fluent style 'getter' method that can be chained
func (o *EmsAutosupportLogRequest) EventSource() string {
	r := *o.EventSourcePtr
	return r
}

// SetEventSource is a fluent style 'setter' method that can be chained
func (o *EmsAutosupportLogRequest) SetEventSource(newValue string) *EmsAutosupportLogRequest {
	o.EventSourcePtr = &newValue
	return o
}

// LogLevel is a fluent style 'getter' method that can be chained
func (o *EmsAutosupportLogRequest) LogLevel() int {
	r := *o.LogLevelPtr
	return r
}

// SetLogLevel is a fluent style 'setter' method that can be chained
func (o *EmsAutosupportLogRequest) SetLogLevel(newValue int) *EmsAutosupportLogRequest {
	o.LogLevelPtr = &newValue
	return o
}

// EmsAutosupportLogResponse is a structure to represent a ems-autosupport-log ZAPI response object
type EmsAutosupportLogResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result EmsAutosupportLogResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o EmsAutosupportLogResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// EmsAutosupportLogResponseResult is a structure to represent a ems-autosupport-log ZAPI object's result
type EmsAutosupportLogResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
}

// ToXML converts this object into an xml string representation
func (o *EmsAutosupportLogResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewEmsAutosupportLogResponse is a factory method for creating new instances of EmsAutosupportLogResponse objects
func NewEmsAutosupportLogResponse() *EmsAutosupportLogResponse { return &EmsAutosupportLogResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o EmsAutosupportLogResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	return buffer.String()
}
