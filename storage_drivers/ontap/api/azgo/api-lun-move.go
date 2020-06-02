package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// LunMoveRequest is a structure to represent a lun-move Request ZAPI object
type LunMoveRequest struct {
	XMLName    xml.Name `xml:"lun-move"`
	NewPathPtr *string  `xml:"new-path"`
	PathPtr    *string  `xml:"path"`
}

// LunMoveResponse is a structure to represent a lun-move Response ZAPI object
type LunMoveResponse struct {
	XMLName         xml.Name              `xml:"netapp"`
	ResponseVersion string                `xml:"version,attr"`
	ResponseXmlns   string                `xml:"xmlns,attr"`
	Result          LunMoveResponseResult `xml:"results"`
}

// NewLunMoveResponse is a factory method for creating new instances of LunMoveResponse objects
func NewLunMoveResponse() *LunMoveResponse {
	return &LunMoveResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMoveResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *LunMoveResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// LunMoveResponseResult is a structure to represent a lun-move Response Result ZAPI object
type LunMoveResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewLunMoveRequest is a factory method for creating new instances of LunMoveRequest objects
func NewLunMoveRequest() *LunMoveRequest {
	return &LunMoveRequest{}
}

// NewLunMoveResponseResult is a factory method for creating new instances of LunMoveResponseResult objects
func NewLunMoveResponseResult() *LunMoveResponseResult {
	return &LunMoveResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *LunMoveRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *LunMoveResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMoveRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMoveResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunMoveRequest) ExecuteUsing(zr *ZapiRunner) (*LunMoveResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunMoveRequest) executeWithoutIteration(zr *ZapiRunner) (*LunMoveResponse, error) {
	result, err := zr.ExecuteUsing(o, "LunMoveRequest", NewLunMoveResponse())
	if result == nil {
		return nil, err
	}
	return result.(*LunMoveResponse), err
}

// NewPath is a 'getter' method
func (o *LunMoveRequest) NewPath() string {
	r := *o.NewPathPtr
	return r
}

// SetNewPath is a fluent style 'setter' method that can be chained
func (o *LunMoveRequest) SetNewPath(newValue string) *LunMoveRequest {
	o.NewPathPtr = &newValue
	return o
}

// Path is a 'getter' method
func (o *LunMoveRequest) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunMoveRequest) SetPath(newValue string) *LunMoveRequest {
	o.PathPtr = &newValue
	return o
}
