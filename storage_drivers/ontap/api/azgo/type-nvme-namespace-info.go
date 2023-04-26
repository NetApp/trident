// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NvmeNamespaceInfoType is a structure to represent a nvme-namespace-info ZAPI object
type NvmeNamespaceInfoType struct {
	XMLName                xml.Name                `xml:"nvme-namespace-info"`
	AnagrpidPtr            *int                    `xml:"anagrpid"`
	ApplicationPtr         *string                 `xml:"application"`
	ApplicationUuidPtr     *UuidType               `xml:"application-uuid"`
	BlockSizePtr           *VdiskBlockSizeType     `xml:"block-size"`
	CommentPtr             *string                 `xml:"comment"`
	CreationTimestampPtr   *DatetimeType           `xml:"creation-timestamp"`
	IsReadOnlyPtr          *bool                   `xml:"is-read-only"`
	NodePtr                *FilerIdType            `xml:"node"`
	NsidPtr                *int                    `xml:"nsid"`
	OstypePtr              *NvmeNamespaceOsType    `xml:"ostype"`
	PathPtr                *LunPathType            `xml:"path"`
	QtreePtr               *QtreeNameType          `xml:"qtree"`
	RestoreInaccessiblePtr *bool                   `xml:"restore-inaccessible"`
	SizePtr                *SizeType               `xml:"size"`
	SizeUsedPtr            *SizeType               `xml:"size-used"`
	StatePtr               *NvmeNamespaceStateType `xml:"state"`
	SubsystemPtr           *string                 `xml:"subsystem"`
	UuidPtr                *UuidType               `xml:"uuid"`
	VolumePtr              *VolumeNameType         `xml:"volume"`
	VserverPtr             *VserverNameType        `xml:"vserver"`
	VserverUuidPtr         *UuidType               `xml:"vserver-uuid"`
}

// NewNvmeNamespaceInfoType is a factory method for creating new instances of NvmeNamespaceInfoType objects
func NewNvmeNamespaceInfoType() *NvmeNamespaceInfoType {
	return &NvmeNamespaceInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeNamespaceInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeNamespaceInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// Anagrpid is a 'getter' method
func (o *NvmeNamespaceInfoType) Anagrpid() int {
	var r int
	if o.AnagrpidPtr == nil {
		return r
	}
	r = *o.AnagrpidPtr
	return r
}

// SetAnagrpid is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetAnagrpid(newValue int) *NvmeNamespaceInfoType {
	o.AnagrpidPtr = &newValue
	return o
}

// Application is a 'getter' method
func (o *NvmeNamespaceInfoType) Application() string {
	var r string
	if o.ApplicationPtr == nil {
		return r
	}
	r = *o.ApplicationPtr
	return r
}

// SetApplication is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetApplication(newValue string) *NvmeNamespaceInfoType {
	o.ApplicationPtr = &newValue
	return o
}

// ApplicationUuid is a 'getter' method
func (o *NvmeNamespaceInfoType) ApplicationUuid() UuidType {
	var r UuidType
	if o.ApplicationUuidPtr == nil {
		return r
	}
	r = *o.ApplicationUuidPtr
	return r
}

// SetApplicationUuid is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetApplicationUuid(newValue UuidType) *NvmeNamespaceInfoType {
	o.ApplicationUuidPtr = &newValue
	return o
}

// BlockSize is a 'getter' method
func (o *NvmeNamespaceInfoType) BlockSize() VdiskBlockSizeType {
	var r VdiskBlockSizeType
	if o.BlockSizePtr == nil {
		return r
	}
	r = *o.BlockSizePtr
	return r
}

// SetBlockSize is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetBlockSize(newValue VdiskBlockSizeType) *NvmeNamespaceInfoType {
	o.BlockSizePtr = &newValue
	return o
}

// Comment is a 'getter' method
func (o *NvmeNamespaceInfoType) Comment() string {
	var r string
	if o.CommentPtr == nil {
		return r
	}
	r = *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetComment(newValue string) *NvmeNamespaceInfoType {
	o.CommentPtr = &newValue
	return o
}

// CreationTimestamp is a 'getter' method
func (o *NvmeNamespaceInfoType) CreationTimestamp() DatetimeType {
	var r DatetimeType
	if o.CreationTimestampPtr == nil {
		return r
	}
	r = *o.CreationTimestampPtr
	return r
}

// SetCreationTimestamp is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetCreationTimestamp(newValue DatetimeType) *NvmeNamespaceInfoType {
	o.CreationTimestampPtr = &newValue
	return o
}

// IsReadOnly is a 'getter' method
func (o *NvmeNamespaceInfoType) IsReadOnly() bool {
	var r bool
	if o.IsReadOnlyPtr == nil {
		return r
	}
	r = *o.IsReadOnlyPtr
	return r
}

// SetIsReadOnly is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetIsReadOnly(newValue bool) *NvmeNamespaceInfoType {
	o.IsReadOnlyPtr = &newValue
	return o
}

// Node is a 'getter' method
func (o *NvmeNamespaceInfoType) Node() FilerIdType {
	var r FilerIdType
	if o.NodePtr == nil {
		return r
	}
	r = *o.NodePtr
	return r
}

// SetNode is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetNode(newValue FilerIdType) *NvmeNamespaceInfoType {
	o.NodePtr = &newValue
	return o
}

// Nsid is a 'getter' method
func (o *NvmeNamespaceInfoType) Nsid() int {
	var r int
	if o.NsidPtr == nil {
		return r
	}
	r = *o.NsidPtr
	return r
}

// SetNsid is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetNsid(newValue int) *NvmeNamespaceInfoType {
	o.NsidPtr = &newValue
	return o
}

// Ostype is a 'getter' method
func (o *NvmeNamespaceInfoType) Ostype() NvmeNamespaceOsType {
	var r NvmeNamespaceOsType
	if o.OstypePtr == nil {
		return r
	}
	r = *o.OstypePtr
	return r
}

// SetOstype is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetOstype(newValue NvmeNamespaceOsType) *NvmeNamespaceInfoType {
	o.OstypePtr = &newValue
	return o
}

// Path is a 'getter' method
func (o *NvmeNamespaceInfoType) Path() LunPathType {
	var r LunPathType
	if o.PathPtr == nil {
		return r
	}
	r = *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetPath(newValue LunPathType) *NvmeNamespaceInfoType {
	o.PathPtr = &newValue
	return o
}

// Qtree is a 'getter' method
func (o *NvmeNamespaceInfoType) Qtree() QtreeNameType {
	var r QtreeNameType
	if o.QtreePtr == nil {
		return r
	}
	r = *o.QtreePtr
	return r
}

// SetQtree is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetQtree(newValue QtreeNameType) *NvmeNamespaceInfoType {
	o.QtreePtr = &newValue
	return o
}

// RestoreInaccessible is a 'getter' method
func (o *NvmeNamespaceInfoType) RestoreInaccessible() bool {
	var r bool
	if o.RestoreInaccessiblePtr == nil {
		return r
	}
	r = *o.RestoreInaccessiblePtr
	return r
}

// SetRestoreInaccessible is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetRestoreInaccessible(newValue bool) *NvmeNamespaceInfoType {
	o.RestoreInaccessiblePtr = &newValue
	return o
}

// Size is a 'getter' method
func (o *NvmeNamespaceInfoType) Size() SizeType {
	var r SizeType
	if o.SizePtr == nil {
		return r
	}
	r = *o.SizePtr
	return r
}

// SetSize is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetSize(newValue SizeType) *NvmeNamespaceInfoType {
	o.SizePtr = &newValue
	return o
}

// SizeUsed is a 'getter' method
func (o *NvmeNamespaceInfoType) SizeUsed() SizeType {
	var r SizeType
	if o.SizeUsedPtr == nil {
		return r
	}
	r = *o.SizeUsedPtr
	return r
}

// SetSizeUsed is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetSizeUsed(newValue SizeType) *NvmeNamespaceInfoType {
	o.SizeUsedPtr = &newValue
	return o
}

// State is a 'getter' method
func (o *NvmeNamespaceInfoType) State() NvmeNamespaceStateType {
	var r NvmeNamespaceStateType
	if o.StatePtr == nil {
		return r
	}
	r = *o.StatePtr
	return r
}

// SetState is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetState(newValue NvmeNamespaceStateType) *NvmeNamespaceInfoType {
	o.StatePtr = &newValue
	return o
}

// Subsystem is a 'getter' method
func (o *NvmeNamespaceInfoType) Subsystem() string {
	var r string
	if o.SubsystemPtr == nil {
		return r
	}
	r = *o.SubsystemPtr
	return r
}

// SetSubsystem is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetSubsystem(newValue string) *NvmeNamespaceInfoType {
	o.SubsystemPtr = &newValue
	return o
}

// Uuid is a 'getter' method
func (o *NvmeNamespaceInfoType) Uuid() UuidType {
	var r UuidType
	if o.UuidPtr == nil {
		return r
	}
	r = *o.UuidPtr
	return r
}

// SetUuid is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetUuid(newValue UuidType) *NvmeNamespaceInfoType {
	o.UuidPtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *NvmeNamespaceInfoType) Volume() VolumeNameType {
	var r VolumeNameType
	if o.VolumePtr == nil {
		return r
	}
	r = *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetVolume(newValue VolumeNameType) *NvmeNamespaceInfoType {
	o.VolumePtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *NvmeNamespaceInfoType) Vserver() VserverNameType {
	var r VserverNameType
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetVserver(newValue VserverNameType) *NvmeNamespaceInfoType {
	o.VserverPtr = &newValue
	return o
}

// VserverUuid is a 'getter' method
func (o *NvmeNamespaceInfoType) VserverUuid() UuidType {
	var r UuidType
	if o.VserverUuidPtr == nil {
		return r
	}
	r = *o.VserverUuidPtr
	return r
}

// SetVserverUuid is a fluent style 'setter' method that can be chained
func (o *NvmeNamespaceInfoType) SetVserverUuid(newValue UuidType) *NvmeNamespaceInfoType {
	o.VserverUuidPtr = &newValue
	return o
}
