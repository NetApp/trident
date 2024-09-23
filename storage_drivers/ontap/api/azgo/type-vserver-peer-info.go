// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VserverPeerInfoType is a structure to represent a vserver-peer-info ZAPI object
type VserverPeerInfoType struct {
	XMLName         xml.Name                         `xml:"vserver-peer-info"`
	ApplicationsPtr *VserverPeerInfoTypeApplications `xml:"applications"`
	// work in progress
	PeerClusterPtr       *string               `xml:"peer-cluster"`
	PeerStatePtr         *VserverPeerStateType `xml:"peer-state"`
	PeerVserverPtr       *string               `xml:"peer-vserver"`
	PeerVserverUuidPtr   *UuidType             `xml:"peer-vserver-uuid"`
	RemoteVserverNamePtr *string               `xml:"remote-vserver-name"`
	VserverPtr           *string               `xml:"vserver"`
	VserverUuidPtr       *UuidType             `xml:"vserver-uuid"`
}

// NewVserverPeerInfoType is a factory method for creating new instances of VserverPeerInfoType objects
func NewVserverPeerInfoType() *VserverPeerInfoType {
	return &VserverPeerInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *VserverPeerInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverPeerInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// VserverPeerInfoTypeApplications is a wrapper
type VserverPeerInfoTypeApplications struct {
	XMLName                   xml.Name                     `xml:"applications"`
	VserverPeerApplicationPtr []VserverPeerApplicationType `xml:"vserver-peer-application"`
}

// VserverPeerApplication is a 'getter' method
func (o *VserverPeerInfoTypeApplications) VserverPeerApplication() []VserverPeerApplicationType {
	r := o.VserverPeerApplicationPtr
	return r
}

// SetVserverPeerApplication is a fluent style 'setter' method that can be chained
func (o *VserverPeerInfoTypeApplications) SetVserverPeerApplication(newValue []VserverPeerApplicationType) *VserverPeerInfoTypeApplications {
	newSlice := make([]VserverPeerApplicationType, len(newValue))
	copy(newSlice, newValue)
	o.VserverPeerApplicationPtr = newSlice
	return o
}

// Applications is a 'getter' method
func (o *VserverPeerInfoType) Applications() VserverPeerInfoTypeApplications {
	var r VserverPeerInfoTypeApplications
	if o.ApplicationsPtr == nil {
		return r
	}
	r = *o.ApplicationsPtr
	return r
}

// SetApplications is a fluent style 'setter' method that can be chained
func (o *VserverPeerInfoType) SetApplications(newValue VserverPeerInfoTypeApplications) *VserverPeerInfoType {
	o.ApplicationsPtr = &newValue
	return o
}

// PeerCluster is a 'getter' method
func (o *VserverPeerInfoType) PeerCluster() string {
	var r string
	if o.PeerClusterPtr == nil {
		return r
	}
	r = *o.PeerClusterPtr
	return r
}

// SetPeerCluster is a fluent style 'setter' method that can be chained
func (o *VserverPeerInfoType) SetPeerCluster(newValue string) *VserverPeerInfoType {
	o.PeerClusterPtr = &newValue
	return o
}

// PeerState is a 'getter' method
func (o *VserverPeerInfoType) PeerState() VserverPeerStateType {
	var r VserverPeerStateType
	if o.PeerStatePtr == nil {
		return r
	}
	r = *o.PeerStatePtr
	return r
}

// SetPeerState is a fluent style 'setter' method that can be chained
func (o *VserverPeerInfoType) SetPeerState(newValue VserverPeerStateType) *VserverPeerInfoType {
	o.PeerStatePtr = &newValue
	return o
}

// PeerVserver is a 'getter' method
func (o *VserverPeerInfoType) PeerVserver() string {
	var r string
	if o.PeerVserverPtr == nil {
		return r
	}
	r = *o.PeerVserverPtr
	return r
}

// SetPeerVserver is a fluent style 'setter' method that can be chained
func (o *VserverPeerInfoType) SetPeerVserver(newValue string) *VserverPeerInfoType {
	o.PeerVserverPtr = &newValue
	return o
}

// PeerVserverUuid is a 'getter' method
func (o *VserverPeerInfoType) PeerVserverUuid() UuidType {
	var r UuidType
	if o.PeerVserverUuidPtr == nil {
		return r
	}
	r = *o.PeerVserverUuidPtr
	return r
}

// SetPeerVserverUuid is a fluent style 'setter' method that can be chained
func (o *VserverPeerInfoType) SetPeerVserverUuid(newValue UuidType) *VserverPeerInfoType {
	o.PeerVserverUuidPtr = &newValue
	return o
}

// RemoteVserverName is a 'getter' method
func (o *VserverPeerInfoType) RemoteVserverName() string {
	var r string
	if o.RemoteVserverNamePtr == nil {
		return r
	}
	r = *o.RemoteVserverNamePtr
	return r
}

// SetRemoteVserverName is a fluent style 'setter' method that can be chained
func (o *VserverPeerInfoType) SetRemoteVserverName(newValue string) *VserverPeerInfoType {
	o.RemoteVserverNamePtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *VserverPeerInfoType) Vserver() string {
	var r string
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *VserverPeerInfoType) SetVserver(newValue string) *VserverPeerInfoType {
	o.VserverPtr = &newValue
	return o
}

// VserverUuid is a 'getter' method
func (o *VserverPeerInfoType) VserverUuid() UuidType {
	var r UuidType
	if o.VserverUuidPtr == nil {
		return r
	}
	r = *o.VserverUuidPtr
	return r
}

// SetVserverUuid is a fluent style 'setter' method that can be chained
func (o *VserverPeerInfoType) SetVserverUuid(newValue UuidType) *VserverPeerInfoType {
	o.VserverUuidPtr = &newValue
	return o
}
