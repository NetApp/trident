package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeRecoveryQueuePurgeRequest is a structure to represent a volume-recovery-queue-purge Request ZAPI object
type VolumeRecoveryQueuePurgeRequest struct {
	XMLName        xml.Name `xml:"volume-recovery-queue-purge"`
	VolumeNamePtr  *string  `xml:"volume-name"`
	VserverNamePtr *string  `xml:"vserver-name"`
}

// VolumeRecoveryQueuePurgeResponse is a structure to represent a volume-recovery-queue-purge Response ZAPI object
type VolumeRecoveryQueuePurgeResponse struct {
	XMLName         xml.Name                               `xml:"netapp"`
	ResponseVersion string                                 `xml:"version,attr"`
	ResponseXmlns   string                                 `xml:"xmlns,attr"`
	Result          VolumeRecoveryQueuePurgeResponseResult `xml:"results"`
}

// NewVolumeRecoveryQueuePurgeResponse is a factory method for creating new instances of VolumeRecoveryQueuePurgeResponse objects
func NewVolumeRecoveryQueuePurgeResponse() *VolumeRecoveryQueuePurgeResponse {
	return &VolumeRecoveryQueuePurgeResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeRecoveryQueuePurgeResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeRecoveryQueuePurgeResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeRecoveryQueuePurgeResponseResult is a structure to represent a volume-recovery-queue-purge Response Result ZAPI object
type VolumeRecoveryQueuePurgeResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewVolumeRecoveryQueuePurgeRequest is a factory method for creating new instances of VolumeRecoveryQueuePurgeRequest objects
func NewVolumeRecoveryQueuePurgeRequest() *VolumeRecoveryQueuePurgeRequest {
	return &VolumeRecoveryQueuePurgeRequest{}
}

// NewVolumeRecoveryQueuePurgeResponseResult is a factory method for creating new instances of VolumeRecoveryQueuePurgeResponseResult objects
func NewVolumeRecoveryQueuePurgeResponseResult() *VolumeRecoveryQueuePurgeResponseResult {
	return &VolumeRecoveryQueuePurgeResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeRecoveryQueuePurgeRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeRecoveryQueuePurgeResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeRecoveryQueuePurgeRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeRecoveryQueuePurgeResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeRecoveryQueuePurgeRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeRecoveryQueuePurgeResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeRecoveryQueuePurgeRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeRecoveryQueuePurgeResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeRecoveryQueuePurgeRequest", NewVolumeRecoveryQueuePurgeResponse())
	if result == nil {
		return nil, err
	}
	return result.(*VolumeRecoveryQueuePurgeResponse), err
}

// VolumeName is a 'getter' method
func (o *VolumeRecoveryQueuePurgeRequest) VolumeName() string {
	var r string
	if o.VolumeNamePtr == nil {
		return r
	}
	r = *o.VolumeNamePtr

	return r
}

// SetVolumeName is a fluent style 'setter' method that can be chained
func (o *VolumeRecoveryQueuePurgeRequest) SetVolumeName(newValue string) *VolumeRecoveryQueuePurgeRequest {
	o.VolumeNamePtr = &newValue
	return o
}

// VserverName is a 'getter' method
func (o *VolumeRecoveryQueuePurgeRequest) VserverName() string {
	var r string
	if o.VserverNamePtr == nil {
		return r
	}
	r = *o.VserverNamePtr

	return r
}

// SetVserverName is a fluent style 'setter' method that can be chained
func (o *VolumeRecoveryQueuePurgeRequest) SetVserverName(newValue string) *VolumeRecoveryQueuePurgeRequest {
	o.VserverNamePtr = &newValue
	return o
}
