package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// QtreeCreateRequest is a structure to represent a qtree-create Request ZAPI object
type QtreeCreateRequest struct {
	XMLName          xml.Name `xml:"qtree-create"`
	ExportPolicyPtr  *string  `xml:"export-policy"`
	ModePtr          *string  `xml:"mode"`
	OplocksPtr       *string  `xml:"oplocks"`
	QtreePtr         *string  `xml:"qtree"`
	SecurityStylePtr *string  `xml:"security-style"`
	VolumePtr        *string  `xml:"volume"`
}

// QtreeCreateResponse is a structure to represent a qtree-create Response ZAPI object
type QtreeCreateResponse struct {
	XMLName         xml.Name                  `xml:"netapp"`
	ResponseVersion string                    `xml:"version,attr"`
	ResponseXmlns   string                    `xml:"xmlns,attr"`
	Result          QtreeCreateResponseResult `xml:"results"`
}

// NewQtreeCreateResponse is a factory method for creating new instances of QtreeCreateResponse objects
func NewQtreeCreateResponse() *QtreeCreateResponse {
	return &QtreeCreateResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeCreateResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *QtreeCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// QtreeCreateResponseResult is a structure to represent a qtree-create Response Result ZAPI object
type QtreeCreateResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewQtreeCreateRequest is a factory method for creating new instances of QtreeCreateRequest objects
func NewQtreeCreateRequest() *QtreeCreateRequest {
	return &QtreeCreateRequest{}
}

// NewQtreeCreateResponseResult is a factory method for creating new instances of QtreeCreateResponseResult objects
func NewQtreeCreateResponseResult() *QtreeCreateResponseResult {
	return &QtreeCreateResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *QtreeCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *QtreeCreateResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeCreateRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeCreateResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *QtreeCreateRequest) ExecuteUsing(zr *ZapiRunner) (*QtreeCreateResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *QtreeCreateRequest) executeWithoutIteration(zr *ZapiRunner) (*QtreeCreateResponse, error) {
	result, err := zr.ExecuteUsing(o, "QtreeCreateRequest", NewQtreeCreateResponse())
	if result == nil {
		return nil, err
	}
	return result.(*QtreeCreateResponse), err
}

// ExportPolicy is a 'getter' method
func (o *QtreeCreateRequest) ExportPolicy() string {
	r := *o.ExportPolicyPtr
	return r
}

// SetExportPolicy is a fluent style 'setter' method that can be chained
func (o *QtreeCreateRequest) SetExportPolicy(newValue string) *QtreeCreateRequest {
	o.ExportPolicyPtr = &newValue
	return o
}

// Mode is a 'getter' method
func (o *QtreeCreateRequest) Mode() string {
	r := *o.ModePtr
	return r
}

// SetMode is a fluent style 'setter' method that can be chained
func (o *QtreeCreateRequest) SetMode(newValue string) *QtreeCreateRequest {
	o.ModePtr = &newValue
	return o
}

// Oplocks is a 'getter' method
func (o *QtreeCreateRequest) Oplocks() string {
	r := *o.OplocksPtr
	return r
}

// SetOplocks is a fluent style 'setter' method that can be chained
func (o *QtreeCreateRequest) SetOplocks(newValue string) *QtreeCreateRequest {
	o.OplocksPtr = &newValue
	return o
}

// Qtree is a 'getter' method
func (o *QtreeCreateRequest) Qtree() string {
	r := *o.QtreePtr
	return r
}

// SetQtree is a fluent style 'setter' method that can be chained
func (o *QtreeCreateRequest) SetQtree(newValue string) *QtreeCreateRequest {
	o.QtreePtr = &newValue
	return o
}

// SecurityStyle is a 'getter' method
func (o *QtreeCreateRequest) SecurityStyle() string {
	r := *o.SecurityStylePtr
	return r
}

// SetSecurityStyle is a fluent style 'setter' method that can be chained
func (o *QtreeCreateRequest) SetSecurityStyle(newValue string) *QtreeCreateRequest {
	o.SecurityStylePtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *QtreeCreateRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *QtreeCreateRequest) SetVolume(newValue string) *QtreeCreateRequest {
	o.VolumePtr = &newValue
	return o
}
