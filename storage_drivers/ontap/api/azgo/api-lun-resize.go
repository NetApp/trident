// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// LunResizeRequest is a structure to represent a lun-resize Request ZAPI object
type LunResizeRequest struct {
	XMLName  xml.Name `xml:"lun-resize"`
	ForcePtr *bool    `xml:"force"`
	PathPtr  *string  `xml:"path"`
	SizePtr  *int     `xml:"size"`
}

// LunResizeResponse is a structure to represent a lun-resize Response ZAPI object
type LunResizeResponse struct {
	XMLName         xml.Name                `xml:"netapp"`
	ResponseVersion string                  `xml:"version,attr"`
	ResponseXmlns   string                  `xml:"xmlns,attr"`
	Result          LunResizeResponseResult `xml:"results"`
}

// NewLunResizeResponse is a factory method for creating new instances of LunResizeResponse objects
func NewLunResizeResponse() *LunResizeResponse {
	return &LunResizeResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunResizeResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *LunResizeResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// LunResizeResponseResult is a structure to represent a lun-resize Response Result ZAPI object
type LunResizeResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
	ActualSizePtr    *int     `xml:"actual-size"`
}

// NewLunResizeRequest is a factory method for creating new instances of LunResizeRequest objects
func NewLunResizeRequest() *LunResizeRequest {
	return &LunResizeRequest{}
}

// NewLunResizeResponseResult is a factory method for creating new instances of LunResizeResponseResult objects
func NewLunResizeResponseResult() *LunResizeResponseResult {
	return &LunResizeResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *LunResizeRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *LunResizeResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunResizeRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunResizeResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunResizeRequest) ExecuteUsing(zr *ZapiRunner) (*LunResizeResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunResizeRequest) executeWithoutIteration(zr *ZapiRunner) (*LunResizeResponse, error) {
	result, err := zr.ExecuteUsing(o, "LunResizeRequest", NewLunResizeResponse())
	if result == nil {
		return nil, err
	}
	return result.(*LunResizeResponse), err
}

// Force is a 'getter' method
func (o *LunResizeRequest) Force() bool {
	r := *o.ForcePtr
	return r
}

// SetForce is a fluent style 'setter' method that can be chained
func (o *LunResizeRequest) SetForce(newValue bool) *LunResizeRequest {
	o.ForcePtr = &newValue
	return o
}

// Path is a 'getter' method
func (o *LunResizeRequest) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunResizeRequest) SetPath(newValue string) *LunResizeRequest {
	o.PathPtr = &newValue
	return o
}

// Size is a 'getter' method
func (o *LunResizeRequest) Size() int {
	r := *o.SizePtr
	return r
}

// SetSize is a fluent style 'setter' method that can be chained
func (o *LunResizeRequest) SetSize(newValue int) *LunResizeRequest {
	o.SizePtr = &newValue
	return o
}

// ActualSize is a 'getter' method
func (o *LunResizeResponseResult) ActualSize() int {
	r := *o.ActualSizePtr
	return r
}

// SetActualSize is a fluent style 'setter' method that can be chained
func (o *LunResizeResponseResult) SetActualSize(newValue int) *LunResizeResponseResult {
	o.ActualSizePtr = &newValue
	return o
}
