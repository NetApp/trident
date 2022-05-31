// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// InitiatorGroupInfoType is a structure to represent a initiator-group-info ZAPI object
type InitiatorGroupInfoType struct {
	XMLName                                xml.Name                          `xml:"initiator-group-info"`
	InitiatorGroupAluaEnabledPtr           *bool                             `xml:"initiator-group-alua-enabled"`
	InitiatorGroupDeleteOnUnmapPtr         *bool                             `xml:"initiator-group-delete-on-unmap"`
	InitiatorGroupNamePtr                  *string                           `xml:"initiator-group-name"`
	InitiatorGroupOsTypePtr                *InitiatorGroupOsTypeType         `xml:"initiator-group-os-type"`
	InitiatorGroupPortsetNamePtr           *string                           `xml:"initiator-group-portset-name"`
	InitiatorGroupReportScsiNameEnabledPtr *bool                             `xml:"initiator-group-report-scsi-name-enabled"`
	InitiatorGroupThrottleBorrowPtr        *bool                             `xml:"initiator-group-throttle-borrow"`
	InitiatorGroupThrottleReservePtr       *int                              `xml:"initiator-group-throttle-reserve"`
	InitiatorGroupTypePtr                  *string                           `xml:"initiator-group-type"`
	InitiatorGroupUsePartnerPtr            *bool                             `xml:"initiator-group-use-partner"`
	InitiatorGroupUuidPtr                  *string                           `xml:"initiator-group-uuid"`
	InitiatorGroupVsaEnabledPtr            *bool                             `xml:"initiator-group-vsa-enabled"`
	InitiatorsPtr                          *InitiatorGroupInfoTypeInitiators `xml:"initiators"`
	// work in progress
	LunIdPtr   *int    `xml:"lun-id"`
	VserverPtr *string `xml:"vserver"`
}

// NewInitiatorGroupInfoType is a factory method for creating new instances of InitiatorGroupInfoType objects
func NewInitiatorGroupInfoType() *InitiatorGroupInfoType {
	return &InitiatorGroupInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *InitiatorGroupInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o InitiatorGroupInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// InitiatorGroupAluaEnabled is a 'getter' method
func (o *InitiatorGroupInfoType) InitiatorGroupAluaEnabled() bool {
	var r bool
	if o.InitiatorGroupAluaEnabledPtr == nil {
		return r
	}
	r = *o.InitiatorGroupAluaEnabledPtr
	return r
}

// SetInitiatorGroupAluaEnabled is a fluent style 'setter' method that can be chained
func (o *InitiatorGroupInfoType) SetInitiatorGroupAluaEnabled(newValue bool) *InitiatorGroupInfoType {
	o.InitiatorGroupAluaEnabledPtr = &newValue
	return o
}

// InitiatorGroupDeleteOnUnmap is a 'getter' method
func (o *InitiatorGroupInfoType) InitiatorGroupDeleteOnUnmap() bool {
	var r bool
	if o.InitiatorGroupDeleteOnUnmapPtr == nil {
		return r
	}
	r = *o.InitiatorGroupDeleteOnUnmapPtr
	return r
}

// SetInitiatorGroupDeleteOnUnmap is a fluent style 'setter' method that can be chained
func (o *InitiatorGroupInfoType) SetInitiatorGroupDeleteOnUnmap(newValue bool) *InitiatorGroupInfoType {
	o.InitiatorGroupDeleteOnUnmapPtr = &newValue
	return o
}

// InitiatorGroupName is a 'getter' method
func (o *InitiatorGroupInfoType) InitiatorGroupName() string {
	var r string
	if o.InitiatorGroupNamePtr == nil {
		return r
	}
	r = *o.InitiatorGroupNamePtr
	return r
}

// SetInitiatorGroupName is a fluent style 'setter' method that can be chained
func (o *InitiatorGroupInfoType) SetInitiatorGroupName(newValue string) *InitiatorGroupInfoType {
	o.InitiatorGroupNamePtr = &newValue
	return o
}

// InitiatorGroupOsType is a 'getter' method
func (o *InitiatorGroupInfoType) InitiatorGroupOsType() InitiatorGroupOsTypeType {
	var r InitiatorGroupOsTypeType
	if o.InitiatorGroupOsTypePtr == nil {
		return r
	}
	r = *o.InitiatorGroupOsTypePtr
	return r
}

// SetInitiatorGroupOsType is a fluent style 'setter' method that can be chained
func (o *InitiatorGroupInfoType) SetInitiatorGroupOsType(newValue InitiatorGroupOsTypeType) *InitiatorGroupInfoType {
	o.InitiatorGroupOsTypePtr = &newValue
	return o
}

// InitiatorGroupPortsetName is a 'getter' method
func (o *InitiatorGroupInfoType) InitiatorGroupPortsetName() string {
	var r string
	if o.InitiatorGroupPortsetNamePtr == nil {
		return r
	}
	r = *o.InitiatorGroupPortsetNamePtr
	return r
}

// SetInitiatorGroupPortsetName is a fluent style 'setter' method that can be chained
func (o *InitiatorGroupInfoType) SetInitiatorGroupPortsetName(newValue string) *InitiatorGroupInfoType {
	o.InitiatorGroupPortsetNamePtr = &newValue
	return o
}

// InitiatorGroupReportScsiNameEnabled is a 'getter' method
func (o *InitiatorGroupInfoType) InitiatorGroupReportScsiNameEnabled() bool {
	var r bool
	if o.InitiatorGroupReportScsiNameEnabledPtr == nil {
		return r
	}
	r = *o.InitiatorGroupReportScsiNameEnabledPtr
	return r
}

// SetInitiatorGroupReportScsiNameEnabled is a fluent style 'setter' method that can be chained
func (o *InitiatorGroupInfoType) SetInitiatorGroupReportScsiNameEnabled(newValue bool) *InitiatorGroupInfoType {
	o.InitiatorGroupReportScsiNameEnabledPtr = &newValue
	return o
}

// InitiatorGroupThrottleBorrow is a 'getter' method
func (o *InitiatorGroupInfoType) InitiatorGroupThrottleBorrow() bool {
	var r bool
	if o.InitiatorGroupThrottleBorrowPtr == nil {
		return r
	}
	r = *o.InitiatorGroupThrottleBorrowPtr
	return r
}

// SetInitiatorGroupThrottleBorrow is a fluent style 'setter' method that can be chained
func (o *InitiatorGroupInfoType) SetInitiatorGroupThrottleBorrow(newValue bool) *InitiatorGroupInfoType {
	o.InitiatorGroupThrottleBorrowPtr = &newValue
	return o
}

// InitiatorGroupThrottleReserve is a 'getter' method
func (o *InitiatorGroupInfoType) InitiatorGroupThrottleReserve() int {
	var r int
	if o.InitiatorGroupThrottleReservePtr == nil {
		return r
	}
	r = *o.InitiatorGroupThrottleReservePtr
	return r
}

// SetInitiatorGroupThrottleReserve is a fluent style 'setter' method that can be chained
func (o *InitiatorGroupInfoType) SetInitiatorGroupThrottleReserve(newValue int) *InitiatorGroupInfoType {
	o.InitiatorGroupThrottleReservePtr = &newValue
	return o
}

// InitiatorGroupType is a 'getter' method
func (o *InitiatorGroupInfoType) InitiatorGroupType() string {
	var r string
	if o.InitiatorGroupTypePtr == nil {
		return r
	}
	r = *o.InitiatorGroupTypePtr
	return r
}

// SetInitiatorGroupType is a fluent style 'setter' method that can be chained
func (o *InitiatorGroupInfoType) SetInitiatorGroupType(newValue string) *InitiatorGroupInfoType {
	o.InitiatorGroupTypePtr = &newValue
	return o
}

// InitiatorGroupUsePartner is a 'getter' method
func (o *InitiatorGroupInfoType) InitiatorGroupUsePartner() bool {
	var r bool
	if o.InitiatorGroupUsePartnerPtr == nil {
		return r
	}
	r = *o.InitiatorGroupUsePartnerPtr
	return r
}

// SetInitiatorGroupUsePartner is a fluent style 'setter' method that can be chained
func (o *InitiatorGroupInfoType) SetInitiatorGroupUsePartner(newValue bool) *InitiatorGroupInfoType {
	o.InitiatorGroupUsePartnerPtr = &newValue
	return o
}

// InitiatorGroupUuid is a 'getter' method
func (o *InitiatorGroupInfoType) InitiatorGroupUuid() string {
	var r string
	if o.InitiatorGroupUuidPtr == nil {
		return r
	}
	r = *o.InitiatorGroupUuidPtr
	return r
}

// SetInitiatorGroupUuid is a fluent style 'setter' method that can be chained
func (o *InitiatorGroupInfoType) SetInitiatorGroupUuid(newValue string) *InitiatorGroupInfoType {
	o.InitiatorGroupUuidPtr = &newValue
	return o
}

// InitiatorGroupVsaEnabled is a 'getter' method
func (o *InitiatorGroupInfoType) InitiatorGroupVsaEnabled() bool {
	var r bool
	if o.InitiatorGroupVsaEnabledPtr == nil {
		return r
	}
	r = *o.InitiatorGroupVsaEnabledPtr
	return r
}

// SetInitiatorGroupVsaEnabled is a fluent style 'setter' method that can be chained
func (o *InitiatorGroupInfoType) SetInitiatorGroupVsaEnabled(newValue bool) *InitiatorGroupInfoType {
	o.InitiatorGroupVsaEnabledPtr = &newValue
	return o
}

// InitiatorGroupInfoTypeInitiators is a wrapper
type InitiatorGroupInfoTypeInitiators struct {
	XMLName          xml.Name            `xml:"initiators"`
	InitiatorInfoPtr []InitiatorInfoType `xml:"initiator-info"`
}

// InitiatorInfo is a 'getter' method
func (o *InitiatorGroupInfoTypeInitiators) InitiatorInfo() []InitiatorInfoType {
	r := o.InitiatorInfoPtr
	return r
}

// SetInitiatorInfo is a fluent style 'setter' method that can be chained
func (o *InitiatorGroupInfoTypeInitiators) SetInitiatorInfo(newValue []InitiatorInfoType) *InitiatorGroupInfoTypeInitiators {
	newSlice := make([]InitiatorInfoType, len(newValue))
	copy(newSlice, newValue)
	o.InitiatorInfoPtr = newSlice
	return o
}

// Initiators is a 'getter' method
func (o *InitiatorGroupInfoType) Initiators() InitiatorGroupInfoTypeInitiators {
	var r InitiatorGroupInfoTypeInitiators
	if o.InitiatorsPtr == nil {
		return r
	}
	r = *o.InitiatorsPtr
	return r
}

// SetInitiators is a fluent style 'setter' method that can be chained
func (o *InitiatorGroupInfoType) SetInitiators(newValue InitiatorGroupInfoTypeInitiators) *InitiatorGroupInfoType {
	o.InitiatorsPtr = &newValue
	return o
}

// LunId is a 'getter' method
func (o *InitiatorGroupInfoType) LunId() int {
	var r int
	if o.LunIdPtr == nil {
		return r
	}
	r = *o.LunIdPtr
	return r
}

// SetLunId is a fluent style 'setter' method that can be chained
func (o *InitiatorGroupInfoType) SetLunId(newValue int) *InitiatorGroupInfoType {
	o.LunIdPtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *InitiatorGroupInfoType) Vserver() string {
	var r string
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *InitiatorGroupInfoType) SetVserver(newValue string) *InitiatorGroupInfoType {
	o.VserverPtr = &newValue
	return o
}
