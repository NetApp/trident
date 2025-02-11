package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeRecoveryQueueInfoType is a structure to represent a volume-recovery-queue-info ZAPI object
type VolumeRecoveryQueueInfoType struct {
	XMLName            xml.Name         `xml:"volume-recovery-queue-info"`
	DeletionTimePtr    *DateType        `xml:"deletion-time"`
	RetentionPeriodPtr *int             `xml:"retention-period"`
	VolumeNamePtr      *VolumeNameType  `xml:"volume-name"`
	VserverNamePtr     *VserverNameType `xml:"vserver-name"`
}

// NewVolumeRecoveryQueueInfoType is a factory method for creating new instances of VolumeRecoveryQueueInfoType objects
func NewVolumeRecoveryQueueInfoType() *VolumeRecoveryQueueInfoType {
	return &VolumeRecoveryQueueInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeRecoveryQueueInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeRecoveryQueueInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// DeletionTime is a 'getter' method
func (o *VolumeRecoveryQueueInfoType) DeletionTime() DateType {
	var r DateType
	if o.DeletionTimePtr == nil {
		return r
	}
	r = *o.DeletionTimePtr

	return r
}

// SetDeletionTime is a fluent style 'setter' method that can be chained
func (o *VolumeRecoveryQueueInfoType) SetDeletionTime(newValue DateType) *VolumeRecoveryQueueInfoType {
	o.DeletionTimePtr = &newValue
	return o
}

// RetentionPeriod is a 'getter' method
func (o *VolumeRecoveryQueueInfoType) RetentionPeriod() int {
	var r int
	if o.RetentionPeriodPtr == nil {
		return r
	}
	r = *o.RetentionPeriodPtr

	return r
}

// SetRetentionPeriod is a fluent style 'setter' method that can be chained
func (o *VolumeRecoveryQueueInfoType) SetRetentionPeriod(newValue int) *VolumeRecoveryQueueInfoType {
	o.RetentionPeriodPtr = &newValue
	return o
}

// VolumeName is a 'getter' method
func (o *VolumeRecoveryQueueInfoType) VolumeName() VolumeNameType {
	var r VolumeNameType
	if o.VolumeNamePtr == nil {
		return r
	}
	r = *o.VolumeNamePtr

	return r
}

// SetVolumeName is a fluent style 'setter' method that can be chained
func (o *VolumeRecoveryQueueInfoType) SetVolumeName(newValue VolumeNameType) *VolumeRecoveryQueueInfoType {
	o.VolumeNamePtr = &newValue
	return o
}

// VserverName is a 'getter' method
func (o *VolumeRecoveryQueueInfoType) VserverName() VserverNameType {
	var r VserverNameType
	if o.VserverNamePtr == nil {
		return r
	}
	r = *o.VserverNamePtr

	return r
}

// SetVserverName is a fluent style 'setter' method that can be chained
func (o *VolumeRecoveryQueueInfoType) SetVserverName(newValue VserverNameType) *VolumeRecoveryQueueInfoType {
	o.VserverNamePtr = &newValue
	return o
}
