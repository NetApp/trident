// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapmirrorReleaseIterInfoType is a structure to represent a snapmirror-release-iter-info ZAPI object
type SnapmirrorReleaseIterInfoType struct {
	XMLName              xml.Name                                    `xml:"snapmirror-release-iter-info"`
	ErrorCodePtr         *int                                        `xml:"error-code"`
	ErrorMessagePtr      *string                                     `xml:"error-message"`
	ResultOperationIdPtr *UuidType                                   `xml:"result-operation-id"`
	SnapmirrorKeyPtr     *SnapmirrorReleaseIterInfoTypeSnapmirrorKey `xml:"snapmirror-key"`
	// work in progress
}

// NewSnapmirrorReleaseIterInfoType is a factory method for creating new instances of SnapmirrorReleaseIterInfoType objects
func NewSnapmirrorReleaseIterInfoType() *SnapmirrorReleaseIterInfoType {
	return &SnapmirrorReleaseIterInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorReleaseIterInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorReleaseIterInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// ErrorCode is a 'getter' method
func (o *SnapmirrorReleaseIterInfoType) ErrorCode() int {
	var r int
	if o.ErrorCodePtr == nil {
		return r
	}
	r = *o.ErrorCodePtr
	return r
}

// SetErrorCode is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterInfoType) SetErrorCode(newValue int) *SnapmirrorReleaseIterInfoType {
	o.ErrorCodePtr = &newValue
	return o
}

// ErrorMessage is a 'getter' method
func (o *SnapmirrorReleaseIterInfoType) ErrorMessage() string {
	var r string
	if o.ErrorMessagePtr == nil {
		return r
	}
	r = *o.ErrorMessagePtr
	return r
}

// SetErrorMessage is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterInfoType) SetErrorMessage(newValue string) *SnapmirrorReleaseIterInfoType {
	o.ErrorMessagePtr = &newValue
	return o
}

// ResultOperationId is a 'getter' method
func (o *SnapmirrorReleaseIterInfoType) ResultOperationId() UuidType {
	var r UuidType
	if o.ResultOperationIdPtr == nil {
		return r
	}
	r = *o.ResultOperationIdPtr
	return r
}

// SetResultOperationId is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterInfoType) SetResultOperationId(newValue UuidType) *SnapmirrorReleaseIterInfoType {
	o.ResultOperationIdPtr = &newValue
	return o
}

// SnapmirrorReleaseIterInfoTypeSnapmirrorKey is a wrapper
type SnapmirrorReleaseIterInfoTypeSnapmirrorKey struct {
	XMLName                      xml.Name                       `xml:"snapmirror-key"`
	SnapmirrorDestinationInfoPtr *SnapmirrorDestinationInfoType `xml:"snapmirror-destination-info"`
}

// SnapmirrorDestinationInfo is a 'getter' method
func (o *SnapmirrorReleaseIterInfoTypeSnapmirrorKey) SnapmirrorDestinationInfo() SnapmirrorDestinationInfoType {
	var r SnapmirrorDestinationInfoType
	if o.SnapmirrorDestinationInfoPtr == nil {
		return r
	}
	r = *o.SnapmirrorDestinationInfoPtr
	return r
}

// SetSnapmirrorDestinationInfo is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterInfoTypeSnapmirrorKey) SetSnapmirrorDestinationInfo(newValue SnapmirrorDestinationInfoType) *SnapmirrorReleaseIterInfoTypeSnapmirrorKey {
	o.SnapmirrorDestinationInfoPtr = &newValue
	return o
}

// SnapmirrorKey is a 'getter' method
func (o *SnapmirrorReleaseIterInfoType) SnapmirrorKey() SnapmirrorReleaseIterInfoTypeSnapmirrorKey {
	var r SnapmirrorReleaseIterInfoTypeSnapmirrorKey
	if o.SnapmirrorKeyPtr == nil {
		return r
	}
	r = *o.SnapmirrorKeyPtr
	return r
}

// SetSnapmirrorKey is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterInfoType) SetSnapmirrorKey(newValue SnapmirrorReleaseIterInfoTypeSnapmirrorKey) *SnapmirrorReleaseIterInfoType {
	o.SnapmirrorKeyPtr = &newValue
	return o
}
