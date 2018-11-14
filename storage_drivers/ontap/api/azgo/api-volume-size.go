package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeSizeRequest is a structure to represent a volume-size Request ZAPI object
type VolumeSizeRequest struct {
	XMLName    xml.Name `xml:"volume-size"`
	NewSizePtr *string  `xml:"new-size"`
	VolumePtr  *string  `xml:"volume"`
}

// VolumeSizeResponse is a structure to represent a volume-size Response ZAPI object
type VolumeSizeResponse struct {
	XMLName         xml.Name                 `xml:"netapp"`
	ResponseVersion string                   `xml:"version,attr"`
	ResponseXmlns   string                   `xml:"xmlns,attr"`
	Result          VolumeSizeResponseResult `xml:"results"`
}

// NewVolumeSizeResponse is a factory method for creating new instances of VolumeSizeResponse objects
func NewVolumeSizeResponse() *VolumeSizeResponse {
	return &VolumeSizeResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeSizeResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeSizeResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeSizeResponseResult is a structure to represent a volume-size Response Result ZAPI object
type VolumeSizeResponseResult struct {
	XMLName                  xml.Name `xml:"results"`
	ResultStatusAttr         string   `xml:"status,attr"`
	ResultReasonAttr         string   `xml:"reason,attr"`
	ResultErrnoAttr          string   `xml:"errno,attr"`
	IsFixedSizeFlexVolumePtr *bool    `xml:"is-fixed-size-flex-volume"`
	IsReadonlyFlexVolumePtr  *bool    `xml:"is-readonly-flex-volume"`
	IsReplicaFlexVolumePtr   *bool    `xml:"is-replica-flex-volume"`
	VolumeSizePtr            *string  `xml:"volume-size"`
}

// NewVolumeSizeRequest is a factory method for creating new instances of VolumeSizeRequest objects
func NewVolumeSizeRequest() *VolumeSizeRequest {
	return &VolumeSizeRequest{}
}

// NewVolumeSizeResponseResult is a factory method for creating new instances of VolumeSizeResponseResult objects
func NewVolumeSizeResponseResult() *VolumeSizeResponseResult {
	return &VolumeSizeResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeSizeRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeSizeResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeSizeRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeSizeResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeSizeRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeSizeResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeSizeRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeSizeResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeSizeRequest", NewVolumeSizeResponse())
	return result.(*VolumeSizeResponse), err
}

// NewSize is a 'getter' method
func (o *VolumeSizeRequest) NewSize() string {
	r := *o.NewSizePtr
	return r
}

// SetNewSize is a fluent style 'setter' method that can be chained
func (o *VolumeSizeRequest) SetNewSize(newValue string) *VolumeSizeRequest {
	o.NewSizePtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *VolumeSizeRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *VolumeSizeRequest) SetVolume(newValue string) *VolumeSizeRequest {
	o.VolumePtr = &newValue
	return o
}

// IsFixedSizeFlexVolume is a 'getter' method
func (o *VolumeSizeResponseResult) IsFixedSizeFlexVolume() bool {
	r := *o.IsFixedSizeFlexVolumePtr
	return r
}

// SetIsFixedSizeFlexVolume is a fluent style 'setter' method that can be chained
func (o *VolumeSizeResponseResult) SetIsFixedSizeFlexVolume(newValue bool) *VolumeSizeResponseResult {
	o.IsFixedSizeFlexVolumePtr = &newValue
	return o
}

// IsReadonlyFlexVolume is a 'getter' method
func (o *VolumeSizeResponseResult) IsReadonlyFlexVolume() bool {
	r := *o.IsReadonlyFlexVolumePtr
	return r
}

// SetIsReadonlyFlexVolume is a fluent style 'setter' method that can be chained
func (o *VolumeSizeResponseResult) SetIsReadonlyFlexVolume(newValue bool) *VolumeSizeResponseResult {
	o.IsReadonlyFlexVolumePtr = &newValue
	return o
}

// IsReplicaFlexVolume is a 'getter' method
func (o *VolumeSizeResponseResult) IsReplicaFlexVolume() bool {
	r := *o.IsReplicaFlexVolumePtr
	return r
}

// SetIsReplicaFlexVolume is a fluent style 'setter' method that can be chained
func (o *VolumeSizeResponseResult) SetIsReplicaFlexVolume(newValue bool) *VolumeSizeResponseResult {
	o.IsReplicaFlexVolumePtr = &newValue
	return o
}

// VolumeSize is a 'getter' method
func (o *VolumeSizeResponseResult) VolumeSize() string {
	r := *o.VolumeSizePtr
	return r
}

// SetVolumeSize is a fluent style 'setter' method that can be chained
func (o *VolumeSizeResponseResult) SetVolumeSize(newValue string) *VolumeSizeResponseResult {
	o.VolumeSizePtr = &newValue
	return o
}
