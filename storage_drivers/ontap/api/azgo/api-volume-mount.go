package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeMountRequest is a structure to represent a volume-mount Request ZAPI object
type VolumeMountRequest struct {
	XMLName                 xml.Name `xml:"volume-mount"`
	ActivateJunctionPtr     *bool    `xml:"activate-junction"`
	ExportPolicyOverridePtr *bool    `xml:"export-policy-override"`
	JunctionPathPtr         *string  `xml:"junction-path"`
	VolumeNamePtr           *string  `xml:"volume-name"`
}

// VolumeMountResponse is a structure to represent a volume-mount Response ZAPI object
type VolumeMountResponse struct {
	XMLName         xml.Name                  `xml:"netapp"`
	ResponseVersion string                    `xml:"version,attr"`
	ResponseXmlns   string                    `xml:"xmlns,attr"`
	Result          VolumeMountResponseResult `xml:"results"`
}

// NewVolumeMountResponse is a factory method for creating new instances of VolumeMountResponse objects
func NewVolumeMountResponse() *VolumeMountResponse {
	return &VolumeMountResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeMountResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeMountResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeMountResponseResult is a structure to represent a volume-mount Response Result ZAPI object
type VolumeMountResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewVolumeMountRequest is a factory method for creating new instances of VolumeMountRequest objects
func NewVolumeMountRequest() *VolumeMountRequest {
	return &VolumeMountRequest{}
}

// NewVolumeMountResponseResult is a factory method for creating new instances of VolumeMountResponseResult objects
func NewVolumeMountResponseResult() *VolumeMountResponseResult {
	return &VolumeMountResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeMountRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeMountResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeMountRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeMountResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeMountRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeMountResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeMountRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeMountResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeMountRequest", NewVolumeMountResponse())
	return result.(*VolumeMountResponse), err
}

// ActivateJunction is a 'getter' method
func (o *VolumeMountRequest) ActivateJunction() bool {
	r := *o.ActivateJunctionPtr
	return r
}

// SetActivateJunction is a fluent style 'setter' method that can be chained
func (o *VolumeMountRequest) SetActivateJunction(newValue bool) *VolumeMountRequest {
	o.ActivateJunctionPtr = &newValue
	return o
}

// ExportPolicyOverride is a 'getter' method
func (o *VolumeMountRequest) ExportPolicyOverride() bool {
	r := *o.ExportPolicyOverridePtr
	return r
}

// SetExportPolicyOverride is a fluent style 'setter' method that can be chained
func (o *VolumeMountRequest) SetExportPolicyOverride(newValue bool) *VolumeMountRequest {
	o.ExportPolicyOverridePtr = &newValue
	return o
}

// JunctionPath is a 'getter' method
func (o *VolumeMountRequest) JunctionPath() string {
	r := *o.JunctionPathPtr
	return r
}

// SetJunctionPath is a fluent style 'setter' method that can be chained
func (o *VolumeMountRequest) SetJunctionPath(newValue string) *VolumeMountRequest {
	o.JunctionPathPtr = &newValue
	return o
}

// VolumeName is a 'getter' method
func (o *VolumeMountRequest) VolumeName() string {
	r := *o.VolumeNamePtr
	return r
}

// SetVolumeName is a fluent style 'setter' method that can be chained
func (o *VolumeMountRequest) SetVolumeName(newValue string) *VolumeMountRequest {
	o.VolumeNamePtr = &newValue
	return o
}
