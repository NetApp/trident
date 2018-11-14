package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeDestroyRequest is a structure to represent a volume-destroy Request ZAPI object
type VolumeDestroyRequest struct {
	XMLName              xml.Name `xml:"volume-destroy"`
	NamePtr              *string  `xml:"name"`
	UnmountAndOfflinePtr *bool    `xml:"unmount-and-offline"`
}

// VolumeDestroyResponse is a structure to represent a volume-destroy Response ZAPI object
type VolumeDestroyResponse struct {
	XMLName         xml.Name                    `xml:"netapp"`
	ResponseVersion string                      `xml:"version,attr"`
	ResponseXmlns   string                      `xml:"xmlns,attr"`
	Result          VolumeDestroyResponseResult `xml:"results"`
}

// NewVolumeDestroyResponse is a factory method for creating new instances of VolumeDestroyResponse objects
func NewVolumeDestroyResponse() *VolumeDestroyResponse {
	return &VolumeDestroyResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeDestroyResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeDestroyResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeDestroyResponseResult is a structure to represent a volume-destroy Response Result ZAPI object
type VolumeDestroyResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewVolumeDestroyRequest is a factory method for creating new instances of VolumeDestroyRequest objects
func NewVolumeDestroyRequest() *VolumeDestroyRequest {
	return &VolumeDestroyRequest{}
}

// NewVolumeDestroyResponseResult is a factory method for creating new instances of VolumeDestroyResponseResult objects
func NewVolumeDestroyResponseResult() *VolumeDestroyResponseResult {
	return &VolumeDestroyResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeDestroyRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeDestroyResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeDestroyRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeDestroyResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeDestroyRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeDestroyResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeDestroyRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeDestroyResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeDestroyRequest", NewVolumeDestroyResponse())
	return result.(*VolumeDestroyResponse), err
}

// Name is a 'getter' method
func (o *VolumeDestroyRequest) Name() string {
	r := *o.NamePtr
	return r
}

// SetName is a fluent style 'setter' method that can be chained
func (o *VolumeDestroyRequest) SetName(newValue string) *VolumeDestroyRequest {
	o.NamePtr = &newValue
	return o
}

// UnmountAndOffline is a 'getter' method
func (o *VolumeDestroyRequest) UnmountAndOffline() bool {
	r := *o.UnmountAndOfflinePtr
	return r
}

// SetUnmountAndOffline is a fluent style 'setter' method that can be chained
func (o *VolumeDestroyRequest) SetUnmountAndOffline(newValue bool) *VolumeDestroyRequest {
	o.UnmountAndOfflinePtr = &newValue
	return o
}
