package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// LunDestroyRequest is a structure to represent a lun-destroy Request ZAPI object
type LunDestroyRequest struct {
	XMLName                  xml.Name `xml:"lun-destroy"`
	DestroyApplicationLunPtr *bool    `xml:"destroy-application-lun"`
	DestroyFencedLunPtr      *bool    `xml:"destroy-fenced-lun"`
	ForcePtr                 *bool    `xml:"force"`
	PathPtr                  *string  `xml:"path"`
}

// LunDestroyResponse is a structure to represent a lun-destroy Response ZAPI object
type LunDestroyResponse struct {
	XMLName         xml.Name                 `xml:"netapp"`
	ResponseVersion string                   `xml:"version,attr"`
	ResponseXmlns   string                   `xml:"xmlns,attr"`
	Result          LunDestroyResponseResult `xml:"results"`
}

// NewLunDestroyResponse is a factory method for creating new instances of LunDestroyResponse objects
func NewLunDestroyResponse() *LunDestroyResponse {
	return &LunDestroyResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunDestroyResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *LunDestroyResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// LunDestroyResponseResult is a structure to represent a lun-destroy Response Result ZAPI object
type LunDestroyResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewLunDestroyRequest is a factory method for creating new instances of LunDestroyRequest objects
func NewLunDestroyRequest() *LunDestroyRequest {
	return &LunDestroyRequest{}
}

// NewLunDestroyResponseResult is a factory method for creating new instances of LunDestroyResponseResult objects
func NewLunDestroyResponseResult() *LunDestroyResponseResult {
	return &LunDestroyResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *LunDestroyRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *LunDestroyResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunDestroyRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunDestroyResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunDestroyRequest) ExecuteUsing(zr *ZapiRunner) (*LunDestroyResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunDestroyRequest) executeWithoutIteration(zr *ZapiRunner) (*LunDestroyResponse, error) {
	result, err := zr.ExecuteUsing(o, "LunDestroyRequest", NewLunDestroyResponse())
	if result == nil {
		return nil, err
	}
	return result.(*LunDestroyResponse), err
}

// DestroyApplicationLun is a 'getter' method
func (o *LunDestroyRequest) DestroyApplicationLun() bool {
	r := *o.DestroyApplicationLunPtr
	return r
}

// SetDestroyApplicationLun is a fluent style 'setter' method that can be chained
func (o *LunDestroyRequest) SetDestroyApplicationLun(newValue bool) *LunDestroyRequest {
	o.DestroyApplicationLunPtr = &newValue
	return o
}

// DestroyFencedLun is a 'getter' method
func (o *LunDestroyRequest) DestroyFencedLun() bool {
	r := *o.DestroyFencedLunPtr
	return r
}

// SetDestroyFencedLun is a fluent style 'setter' method that can be chained
func (o *LunDestroyRequest) SetDestroyFencedLun(newValue bool) *LunDestroyRequest {
	o.DestroyFencedLunPtr = &newValue
	return o
}

// Force is a 'getter' method
func (o *LunDestroyRequest) Force() bool {
	r := *o.ForcePtr
	return r
}

// SetForce is a fluent style 'setter' method that can be chained
func (o *LunDestroyRequest) SetForce(newValue bool) *LunDestroyRequest {
	o.ForcePtr = &newValue
	return o
}

// Path is a 'getter' method
func (o *LunDestroyRequest) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunDestroyRequest) SetPath(newValue string) *LunDestroyRequest {
	o.PathPtr = &newValue
	return o
}
