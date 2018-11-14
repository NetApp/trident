package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// LunOnlineRequest is a structure to represent a lun-online Request ZAPI object
type LunOnlineRequest struct {
	XMLName  xml.Name `xml:"lun-online"`
	ForcePtr *bool    `xml:"force"`
	PathPtr  *string  `xml:"path"`
}

// LunOnlineResponse is a structure to represent a lun-online Response ZAPI object
type LunOnlineResponse struct {
	XMLName         xml.Name                `xml:"netapp"`
	ResponseVersion string                  `xml:"version,attr"`
	ResponseXmlns   string                  `xml:"xmlns,attr"`
	Result          LunOnlineResponseResult `xml:"results"`
}

// NewLunOnlineResponse is a factory method for creating new instances of LunOnlineResponse objects
func NewLunOnlineResponse() *LunOnlineResponse {
	return &LunOnlineResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunOnlineResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *LunOnlineResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// LunOnlineResponseResult is a structure to represent a lun-online Response Result ZAPI object
type LunOnlineResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewLunOnlineRequest is a factory method for creating new instances of LunOnlineRequest objects
func NewLunOnlineRequest() *LunOnlineRequest {
	return &LunOnlineRequest{}
}

// NewLunOnlineResponseResult is a factory method for creating new instances of LunOnlineResponseResult objects
func NewLunOnlineResponseResult() *LunOnlineResponseResult {
	return &LunOnlineResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *LunOnlineRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *LunOnlineResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunOnlineRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunOnlineResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *LunOnlineRequest) ExecuteUsing(zr *ZapiRunner) (*LunOnlineResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *LunOnlineRequest) executeWithoutIteration(zr *ZapiRunner) (*LunOnlineResponse, error) {
	result, err := zr.ExecuteUsing(o, "LunOnlineRequest", NewLunOnlineResponse())
	return result.(*LunOnlineResponse), err
}

// Force is a 'getter' method
func (o *LunOnlineRequest) Force() bool {
	r := *o.ForcePtr
	return r
}

// SetForce is a fluent style 'setter' method that can be chained
func (o *LunOnlineRequest) SetForce(newValue bool) *LunOnlineRequest {
	o.ForcePtr = &newValue
	return o
}

// Path is a 'getter' method
func (o *LunOnlineRequest) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunOnlineRequest) SetPath(newValue string) *LunOnlineRequest {
	o.PathPtr = &newValue
	return o
}
