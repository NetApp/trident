package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// LunGetSerialNumberRequest is a structure to represent a lun-get-serial-number Request ZAPI object
type LunGetSerialNumberRequest struct {
	XMLName xml.Name `xml:"lun-get-serial-number"`
	PathPtr *string  `xml:"path"`
}

// LunGetSerialNumberResponse is a structure to represent a lun-get-serial-number Response ZAPI object
type LunGetSerialNumberResponse struct {
	XMLName         xml.Name                         `xml:"netapp"`
	ResponseVersion string                           `xml:"version,attr"`
	ResponseXmlns   string                           `xml:"xmlns,attr"`
	Result          LunGetSerialNumberResponseResult `xml:"results"`
}

// NewLunGetSerialNumberResponse is a factory method for creating new instances of LunGetSerialNumberResponse objects
func NewLunGetSerialNumberResponse() *LunGetSerialNumberResponse {
	return &LunGetSerialNumberResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetSerialNumberResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *LunGetSerialNumberResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// LunGetSerialNumberResponseResult is a structure to represent a lun-get-serial-number Response Result ZAPI object
type LunGetSerialNumberResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
	SerialNumberPtr  *string  `xml:"serial-number"`
}

// NewLunGetSerialNumberRequest is a factory method for creating new instances of LunGetSerialNumberRequest objects
func NewLunGetSerialNumberRequest() *LunGetSerialNumberRequest {
	return &LunGetSerialNumberRequest{}
}

// NewLunGetSerialNumberResponseResult is a factory method for creating new instances of LunGetSerialNumberResponseResult objects
func NewLunGetSerialNumberResponseResult() *LunGetSerialNumberResponseResult {
	return &LunGetSerialNumberResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *LunGetSerialNumberRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *LunGetSerialNumberResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetSerialNumberRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetSerialNumberResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *LunGetSerialNumberRequest) ExecuteUsing(zr *ZapiRunner) (*LunGetSerialNumberResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *LunGetSerialNumberRequest) executeWithoutIteration(zr *ZapiRunner) (*LunGetSerialNumberResponse, error) {
	result, err := zr.ExecuteUsing(o, "LunGetSerialNumberRequest", NewLunGetSerialNumberResponse())
	return result.(*LunGetSerialNumberResponse), err
}

// Path is a 'getter' method
func (o *LunGetSerialNumberRequest) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunGetSerialNumberRequest) SetPath(newValue string) *LunGetSerialNumberRequest {
	o.PathPtr = &newValue
	return o
}

// SerialNumber is a 'getter' method
func (o *LunGetSerialNumberResponseResult) SerialNumber() string {
	r := *o.SerialNumberPtr
	return r
}

// SetSerialNumber is a fluent style 'setter' method that can be chained
func (o *LunGetSerialNumberResponseResult) SetSerialNumber(newValue string) *LunGetSerialNumberResponseResult {
	o.SerialNumberPtr = &newValue
	return o
}
