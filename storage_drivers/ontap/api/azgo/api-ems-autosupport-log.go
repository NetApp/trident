// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// EmsAutosupportLogRequest is a structure to represent a ems-autosupport-log Request ZAPI object
type EmsAutosupportLogRequest struct {
	XMLName             xml.Name `xml:"ems-autosupport-log"`
	AppVersionPtr       *string  `xml:"app-version"`
	AutoSupportPtr      *bool    `xml:"auto-support"`
	CategoryPtr         *string  `xml:"category"`
	ComputerNamePtr     *string  `xml:"computer-name"`
	EventDescriptionPtr *string  `xml:"event-description"`
	EventIdPtr          *int     `xml:"event-id"`
	EventSourcePtr      *string  `xml:"event-source"`
	LogLevelPtr         *int     `xml:"log-level"`
}

// EmsAutosupportLogResponse is a structure to represent a ems-autosupport-log Response ZAPI object
type EmsAutosupportLogResponse struct {
	XMLName         xml.Name                        `xml:"netapp"`
	ResponseVersion string                          `xml:"version,attr"`
	ResponseXmlns   string                          `xml:"xmlns,attr"`
	Result          EmsAutosupportLogResponseResult `xml:"results"`
}

// NewEmsAutosupportLogResponse is a factory method for creating new instances of EmsAutosupportLogResponse objects
func NewEmsAutosupportLogResponse() *EmsAutosupportLogResponse {
	return &EmsAutosupportLogResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o EmsAutosupportLogResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *EmsAutosupportLogResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// EmsAutosupportLogResponseResult is a structure to represent a ems-autosupport-log Response Result ZAPI object
type EmsAutosupportLogResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewEmsAutosupportLogRequest is a factory method for creating new instances of EmsAutosupportLogRequest objects
func NewEmsAutosupportLogRequest() *EmsAutosupportLogRequest {
	return &EmsAutosupportLogRequest{}
}

// NewEmsAutosupportLogResponseResult is a factory method for creating new instances of EmsAutosupportLogResponseResult objects
func NewEmsAutosupportLogResponseResult() *EmsAutosupportLogResponseResult {
	return &EmsAutosupportLogResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *EmsAutosupportLogRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *EmsAutosupportLogResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o EmsAutosupportLogRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o EmsAutosupportLogResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *EmsAutosupportLogRequest) ExecuteUsing(zr *ZapiRunner) (*EmsAutosupportLogResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *EmsAutosupportLogRequest) executeWithoutIteration(zr *ZapiRunner) (*EmsAutosupportLogResponse, error) {
	result, err := zr.ExecuteUsing(o, "EmsAutosupportLogRequest", NewEmsAutosupportLogResponse())
	if result == nil {
		return nil, err
	}
	return result.(*EmsAutosupportLogResponse), err
}

// AppVersion is a 'getter' method
func (o *EmsAutosupportLogRequest) AppVersion() string {
	r := *o.AppVersionPtr
	return r
}

// SetAppVersion is a fluent style 'setter' method that can be chained
func (o *EmsAutosupportLogRequest) SetAppVersion(newValue string) *EmsAutosupportLogRequest {
	o.AppVersionPtr = &newValue
	return o
}

// AutoSupport is a 'getter' method
func (o *EmsAutosupportLogRequest) AutoSupport() bool {
	r := *o.AutoSupportPtr
	return r
}

// SetAutoSupport is a fluent style 'setter' method that can be chained
func (o *EmsAutosupportLogRequest) SetAutoSupport(newValue bool) *EmsAutosupportLogRequest {
	o.AutoSupportPtr = &newValue
	return o
}

// Category is a 'getter' method
func (o *EmsAutosupportLogRequest) Category() string {
	r := *o.CategoryPtr
	return r
}

// SetCategory is a fluent style 'setter' method that can be chained
func (o *EmsAutosupportLogRequest) SetCategory(newValue string) *EmsAutosupportLogRequest {
	o.CategoryPtr = &newValue
	return o
}

// ComputerName is a 'getter' method
func (o *EmsAutosupportLogRequest) ComputerName() string {
	r := *o.ComputerNamePtr
	return r
}

// SetComputerName is a fluent style 'setter' method that can be chained
func (o *EmsAutosupportLogRequest) SetComputerName(newValue string) *EmsAutosupportLogRequest {
	o.ComputerNamePtr = &newValue
	return o
}

// EventDescription is a 'getter' method
func (o *EmsAutosupportLogRequest) EventDescription() string {
	r := *o.EventDescriptionPtr
	return r
}

// SetEventDescription is a fluent style 'setter' method that can be chained
func (o *EmsAutosupportLogRequest) SetEventDescription(newValue string) *EmsAutosupportLogRequest {
	o.EventDescriptionPtr = &newValue
	return o
}

// EventId is a 'getter' method
func (o *EmsAutosupportLogRequest) EventId() int {
	r := *o.EventIdPtr
	return r
}

// SetEventId is a fluent style 'setter' method that can be chained
func (o *EmsAutosupportLogRequest) SetEventId(newValue int) *EmsAutosupportLogRequest {
	o.EventIdPtr = &newValue
	return o
}

// EventSource is a 'getter' method
func (o *EmsAutosupportLogRequest) EventSource() string {
	r := *o.EventSourcePtr
	return r
}

// SetEventSource is a fluent style 'setter' method that can be chained
func (o *EmsAutosupportLogRequest) SetEventSource(newValue string) *EmsAutosupportLogRequest {
	o.EventSourcePtr = &newValue
	return o
}

// LogLevel is a 'getter' method
func (o *EmsAutosupportLogRequest) LogLevel() int {
	r := *o.LogLevelPtr
	return r
}

// SetLogLevel is a fluent style 'setter' method that can be chained
func (o *EmsAutosupportLogRequest) SetLogLevel(newValue int) *EmsAutosupportLogRequest {
	o.LogLevelPtr = &newValue
	return o
}
