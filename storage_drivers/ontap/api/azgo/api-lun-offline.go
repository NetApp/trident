package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// LunOfflineRequest is a structure to represent a lun-offline Request ZAPI object
type LunOfflineRequest struct {
	XMLName xml.Name `xml:"lun-offline"`
	PathPtr *string  `xml:"path"`
}

// LunOfflineResponse is a structure to represent a lun-offline Response ZAPI object
type LunOfflineResponse struct {
	XMLName         xml.Name                 `xml:"netapp"`
	ResponseVersion string                   `xml:"version,attr"`
	ResponseXmlns   string                   `xml:"xmlns,attr"`
	Result          LunOfflineResponseResult `xml:"results"`
}

// NewLunOfflineResponse is a factory method for creating new instances of LunOfflineResponse objects
func NewLunOfflineResponse() *LunOfflineResponse {
	return &LunOfflineResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunOfflineResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *LunOfflineResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// LunOfflineResponseResult is a structure to represent a lun-offline Response Result ZAPI object
type LunOfflineResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewLunOfflineRequest is a factory method for creating new instances of LunOfflineRequest objects
func NewLunOfflineRequest() *LunOfflineRequest {
	return &LunOfflineRequest{}
}

// NewLunOfflineResponseResult is a factory method for creating new instances of LunOfflineResponseResult objects
func NewLunOfflineResponseResult() *LunOfflineResponseResult {
	return &LunOfflineResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *LunOfflineRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *LunOfflineResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunOfflineRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunOfflineResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunOfflineRequest) ExecuteUsing(zr *ZapiRunner) (*LunOfflineResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunOfflineRequest) executeWithoutIteration(zr *ZapiRunner) (*LunOfflineResponse, error) {
	result, err := zr.ExecuteUsing(o, "LunOfflineRequest", NewLunOfflineResponse())
	if result == nil {
		return nil, err
	}
	return result.(*LunOfflineResponse), err
}

// Path is a 'getter' method
func (o *LunOfflineRequest) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunOfflineRequest) SetPath(newValue string) *LunOfflineRequest {
	o.PathPtr = &newValue
	return o
}
