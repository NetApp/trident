package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// LunUnmapRequest is a structure to represent a lun-unmap Request ZAPI object
type LunUnmapRequest struct {
	XMLName           xml.Name `xml:"lun-unmap"`
	InitiatorGroupPtr *string  `xml:"initiator-group"`
	PathPtr           *string  `xml:"path"`
}

// LunUnmapResponse is a structure to represent a lun-unmap Response ZAPI object
type LunUnmapResponse struct {
	XMLName         xml.Name               `xml:"netapp"`
	ResponseVersion string                 `xml:"version,attr"`
	ResponseXmlns   string                 `xml:"xmlns,attr"`
	Result          LunUnmapResponseResult `xml:"results"`
}

// NewLunUnmapResponse is a factory method for creating new instances of LunUnmapResponse objects
func NewLunUnmapResponse() *LunUnmapResponse {
	return &LunUnmapResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunUnmapResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *LunUnmapResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// LunUnmapResponseResult is a structure to represent a lun-unmap Response Result ZAPI object
type LunUnmapResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewLunUnmapRequest is a factory method for creating new instances of LunUnmapRequest objects
func NewLunUnmapRequest() *LunUnmapRequest {
	return &LunUnmapRequest{}
}

// NewLunUnmapResponseResult is a factory method for creating new instances of LunUnmapResponseResult objects
func NewLunUnmapResponseResult() *LunUnmapResponseResult {
	return &LunUnmapResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *LunUnmapRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *LunUnmapResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunUnmapRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunUnmapResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunUnmapRequest) ExecuteUsing(zr *ZapiRunner) (*LunUnmapResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunUnmapRequest) executeWithoutIteration(zr *ZapiRunner) (*LunUnmapResponse, error) {
	result, err := zr.ExecuteUsing(o, "LunUnmapRequest", NewLunUnmapResponse())
	if result == nil {
		return nil, err
	}
	return result.(*LunUnmapResponse), err
}

// InitiatorGroup is a 'getter' method
func (o *LunUnmapRequest) InitiatorGroup() string {
	r := *o.InitiatorGroupPtr
	return r
}

// SetInitiatorGroup is a fluent style 'setter' method that can be chained
func (o *LunUnmapRequest) SetInitiatorGroup(newValue string) *LunUnmapRequest {
	o.InitiatorGroupPtr = &newValue
	return o
}

// Path is a 'getter' method
func (o *LunUnmapRequest) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunUnmapRequest) SetPath(newValue string) *LunUnmapRequest {
	o.PathPtr = &newValue
	return o
}
