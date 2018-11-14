package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeGetRootNameRequest is a structure to represent a volume-get-root-name Request ZAPI object
type VolumeGetRootNameRequest struct {
	XMLName xml.Name `xml:"volume-get-root-name"`
}

// VolumeGetRootNameResponse is a structure to represent a volume-get-root-name Response ZAPI object
type VolumeGetRootNameResponse struct {
	XMLName         xml.Name                        `xml:"netapp"`
	ResponseVersion string                          `xml:"version,attr"`
	ResponseXmlns   string                          `xml:"xmlns,attr"`
	Result          VolumeGetRootNameResponseResult `xml:"results"`
}

// NewVolumeGetRootNameResponse is a factory method for creating new instances of VolumeGetRootNameResponse objects
func NewVolumeGetRootNameResponse() *VolumeGetRootNameResponse {
	return &VolumeGetRootNameResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeGetRootNameResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeGetRootNameResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeGetRootNameResponseResult is a structure to represent a volume-get-root-name Response Result ZAPI object
type VolumeGetRootNameResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
	VolumePtr        *string  `xml:"volume"`
}

// NewVolumeGetRootNameRequest is a factory method for creating new instances of VolumeGetRootNameRequest objects
func NewVolumeGetRootNameRequest() *VolumeGetRootNameRequest {
	return &VolumeGetRootNameRequest{}
}

// NewVolumeGetRootNameResponseResult is a factory method for creating new instances of VolumeGetRootNameResponseResult objects
func NewVolumeGetRootNameResponseResult() *VolumeGetRootNameResponseResult {
	return &VolumeGetRootNameResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeGetRootNameRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeGetRootNameResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeGetRootNameRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeGetRootNameResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeGetRootNameRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeGetRootNameResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeGetRootNameRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeGetRootNameResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeGetRootNameRequest", NewVolumeGetRootNameResponse())
	return result.(*VolumeGetRootNameResponse), err
}

// Volume is a 'getter' method
func (o *VolumeGetRootNameResponseResult) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *VolumeGetRootNameResponseResult) SetVolume(newValue string) *VolumeGetRootNameResponseResult {
	o.VolumePtr = &newValue
	return o
}
