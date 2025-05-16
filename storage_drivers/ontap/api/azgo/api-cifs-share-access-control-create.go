// Code generated automatically. DO NOT EDIT.
// Copyright 2025 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"

)

// CifsShareAccessControlCreateRequest is a structure to represent a cifs-share-access-control-create Request ZAPI object
type CifsShareAccessControlCreateRequest struct {
	XMLName          xml.Name `xml:"cifs-share-access-control-create"`
	SharePtr         *string  `xml:"share"`
	PermissionPtr    *string  `xml:"permission"`
	UserGroupTypePtr *string  `xml:"user-group-type"`
	UserOrGroupPtr   *string  `xml:"user-or-group"`
}

// CifsShareAccessControlCreateResponse is a structure to represent a cifs-share-access-control-create Response ZAPI object
type CifsShareAccessControlCreateResponse struct {
	XMLName         xml.Name                                   `xml:"netapp"`
	ResponseVersion string                                     `xml:"version,attr"`
	ResponseXmlns   string                                     `xml:"xmlns,attr"`
	Result          CifsShareAccessControlCreateResponseResult `xml:"results"`
}

// NewCifsShareAccessControlCreateResponse is a factory method for creating new instances of CifsShareAccessControlCreateResponse objects
func NewCifsShareAccessControlCreateResponse() *CifsShareAccessControlCreateResponse {
	return &CifsShareAccessControlCreateResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CifsShareAccessControlCreateResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *CifsShareAccessControlCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// CifsShareAccessControlCreateResponseResult is a structure to represent a cifs-share-access-control-create Response Result ZAPI object
type CifsShareAccessControlCreateResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewCifsShareAccessControlCreateRequest is a factory method for creating new instances of CifsShareAccessControlCreateRequest objects
func NewCifsShareAccessControlCreateRequest() *CifsShareAccessControlCreateRequest {
	return &CifsShareAccessControlCreateRequest{}
}

// NewCifsShareAccessControlCreateResponseResult is a factory method for creating new instances of CifsShareAccessControlCreateResponseResult objects
func NewCifsShareAccessControlCreateResponseResult() *CifsShareAccessControlCreateResponseResult {
	return &CifsShareAccessControlCreateResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *CifsShareAccessControlCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *CifsShareAccessControlCreateResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CifsShareAccessControlCreateRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CifsShareAccessControlCreateResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *CifsShareAccessControlCreateRequest) ExecuteUsing(zr *ZapiRunner) (*CifsShareAccessControlCreateResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *CifsShareAccessControlCreateRequest) executeWithoutIteration(zr *ZapiRunner) (*CifsShareAccessControlCreateResponse, error) {
	result, err := zr.ExecuteUsing(o, "CifsShareAccessControlCreateRequest", NewCifsShareAccessControlCreateResponse())
	if result == nil {
		return nil, err
	}
	return result.(*CifsShareAccessControlCreateResponse), err
}

// Share is a 'getter' method
func (o *CifsShareAccessControlCreateRequest) Share() string {
	var r string
	if o.SharePtr == nil {
		return r
	}
	r = *o.SharePtr
	return r
}

// SetShare is a fluent style 'setter' method that can be chained
func (o *CifsShareAccessControlCreateRequest) SetShare(newValue string) *CifsShareAccessControlCreateRequest {
	o.SharePtr = &newValue
	return o
}

// Permission is a 'getter' method
func (o *CifsShareAccessControlCreateRequest) Permission() string {
	var r string
	if o.PermissionPtr == nil {
		return r
	}
	r = *o.PermissionPtr
	return r
}

// SetPermission is a fluent style 'setter' method that can be chained
func (o *CifsShareAccessControlCreateRequest) SetPermission(newValue string) *CifsShareAccessControlCreateRequest {
	o.PermissionPtr = &newValue
	return o
}

// UserGroupType is a 'getter' method
func (o *CifsShareAccessControlCreateRequest) UserGroupType() string {
	var r string
	if o.UserGroupTypePtr == nil {
		return r
	}
	r = *o.UserGroupTypePtr
	return r
}

// SetUserGroupType is a fluent style 'setter' method that can be chained
func (o *CifsShareAccessControlCreateRequest) SetUserGroupType(newValue string) *CifsShareAccessControlCreateRequest {
	o.UserGroupTypePtr = &newValue
	return o
}

// UserOrGroup is a 'getter' method
func (o *CifsShareAccessControlCreateRequest) UserOrGroup() string {
	var r string
	if o.UserOrGroupPtr == nil {
		return r
	}
	r = *o.UserOrGroupPtr
	return r
}

// SetUserOrGroup is a fluent style 'setter' method that can be chained
func (o *CifsShareAccessControlCreateRequest) SetUserOrGroup(newValue string) *CifsShareAccessControlCreateRequest {
	o.UserOrGroupPtr = &newValue
	return o
}