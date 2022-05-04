// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeUnmountRequest is a structure to represent a volume-unmount Request ZAPI object
type VolumeUnmountRequest struct {
	XMLName       xml.Name `xml:"volume-unmount"`
	ForcePtr      *bool    `xml:"force"`
	VolumeNamePtr *string  `xml:"volume-name"`
}

// VolumeUnmountResponse is a structure to represent a volume-unmount Response ZAPI object
type VolumeUnmountResponse struct {
	XMLName         xml.Name                    `xml:"netapp"`
	ResponseVersion string                      `xml:"version,attr"`
	ResponseXmlns   string                      `xml:"xmlns,attr"`
	Result          VolumeUnmountResponseResult `xml:"results"`
}

// NewVolumeUnmountResponse is a factory method for creating new instances of VolumeUnmountResponse objects
func NewVolumeUnmountResponse() *VolumeUnmountResponse {
	return &VolumeUnmountResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeUnmountResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeUnmountResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeUnmountResponseResult is a structure to represent a volume-unmount Response Result ZAPI object
type VolumeUnmountResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewVolumeUnmountRequest is a factory method for creating new instances of VolumeUnmountRequest objects
func NewVolumeUnmountRequest() *VolumeUnmountRequest {
	return &VolumeUnmountRequest{}
}

// NewVolumeUnmountResponseResult is a factory method for creating new instances of VolumeUnmountResponseResult objects
func NewVolumeUnmountResponseResult() *VolumeUnmountResponseResult {
	return &VolumeUnmountResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeUnmountRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeUnmountResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeUnmountRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeUnmountResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeUnmountRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeUnmountResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeUnmountRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeUnmountResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeUnmountRequest", NewVolumeUnmountResponse())
	if result == nil {
		return nil, err
	}
	return result.(*VolumeUnmountResponse), err
}

// Force is a 'getter' method
func (o *VolumeUnmountRequest) Force() bool {
	r := *o.ForcePtr
	return r
}

// SetForce is a fluent style 'setter' method that can be chained
func (o *VolumeUnmountRequest) SetForce(newValue bool) *VolumeUnmountRequest {
	o.ForcePtr = &newValue
	return o
}

// VolumeName is a 'getter' method
func (o *VolumeUnmountRequest) VolumeName() string {
	r := *o.VolumeNamePtr
	return r
}

// SetVolumeName is a fluent style 'setter' method that can be chained
func (o *VolumeUnmountRequest) SetVolumeName(newValue string) *VolumeUnmountRequest {
	o.VolumeNamePtr = &newValue
	return o
}
