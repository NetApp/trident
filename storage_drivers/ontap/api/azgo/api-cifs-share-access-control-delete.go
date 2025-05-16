// Code generated automatically. DO NOT EDIT.
// Copyright 2025 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// CifsShareAccessControlDeleteRequest is a structure to represent a cifs-share-access-control-delete Request ZAPI object
type CifsShareAccessControlDeleteRequest struct {
XMLName          xml.Name `xml:"cifs-share-access-control-delete"`
SharePtr         *string  `xml:"share"`
UserGroupTypePtr *string  `xml:"user-group-type"`
UserOrGroupPtr   *string  `xml:"user-or-group"`
}

// CifsShareAccessControlDeleteResponse is a structure to represent a cifs-share-access-control-delete Response ZAPI object
type CifsShareAccessControlDeleteResponse struct {
XMLName         xml.Name                                   `xml:"netapp"`
ResponseVersion string                                     `xml:"version,attr"`
ResponseXmlns   string                                     `xml:"xmlns,attr"`
Result          CifsShareAccessControlDeleteResponseResult `xml:"results"`
}

// NewCifsShareAccessControlDeleteResponse is a factory method for creating new instances of CifsShareAccessControlDeleteResponse objects
func NewCifsShareAccessControlDeleteResponse() *CifsShareAccessControlDeleteResponse {
return &CifsShareAccessControlDeleteResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CifsShareAccessControlDeleteResponse) String() string {
return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *CifsShareAccessControlDeleteResponse) ToXML() (string, error) {
output, err := xml.MarshalIndent(o, " ", "    ")
if err != nil {
log.Errorf("error: %v", err)
}
return string(output), err
}

// CifsShareAccessControlDeleteResponseResult is a structure to represent a cifs-share-access-control-delete Response Result ZAPI object
type CifsShareAccessControlDeleteResponseResult struct {
XMLName          xml.Name `xml:"results"`
ResultStatusAttr string   `xml:"status,attr"`
ResultReasonAttr string   `xml:"reason,attr"`
ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewCifsShareAccessControlDeleteRequest is a factory method for creating new instances of CifsShareAccessControlDeleteRequest objects
func NewCifsShareAccessControlDeleteRequest() *CifsShareAccessControlDeleteRequest {
return &CifsShareAccessControlDeleteRequest{}
}

// NewCifsShareAccessControlDeleteResponseResult is a factory method for creating new instances of CifsShareAccessControlDeleteResponseResult objects
func NewCifsShareAccessControlDeleteResponseResult() *CifsShareAccessControlDeleteResponseResult {
return &CifsShareAccessControlDeleteResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *CifsShareAccessControlDeleteRequest) ToXML() (string, error) {
output, err := xml.MarshalIndent(o, " ", "    ")
if err != nil {
log.Errorf("error: %v", err)
}
return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *CifsShareAccessControlDeleteResponseResult) ToXML() (string, error) {
output, err := xml.MarshalIndent(o, " ", "    ")
if err != nil {
log.Errorf("error: %v", err)
}
return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CifsShareAccessControlDeleteRequest) String() string {
return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CifsShareAccessControlDeleteResponseResult) String() string {
return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *CifsShareAccessControlDeleteRequest) ExecuteUsing(zr *ZapiRunner) (*CifsShareAccessControlDeleteResponse, error) {
return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *CifsShareAccessControlDeleteRequest) executeWithoutIteration(zr *ZapiRunner) (*CifsShareAccessControlDeleteResponse, error) {
result, err := zr.ExecuteUsing(o, "CifsShareAccessControlDeleteRequest", NewCifsShareAccessControlDeleteResponse())
if result == nil {
return nil, err
}
return result.(*CifsShareAccessControlDeleteResponse), err
}

// Share is a 'getter' method
func (o *CifsShareAccessControlDeleteRequest) Share() string {
var r string
if o.SharePtr == nil {
return r
}
r = *o.SharePtr
return r
}

// SetShare is a fluent style 'setter' method that can be chained
func (o *CifsShareAccessControlDeleteRequest) SetShare(newValue string) *CifsShareAccessControlDeleteRequest {
o.SharePtr = &newValue
return o
}

// UserGroupType is a 'getter' method
func (o *CifsShareAccessControlDeleteRequest) UserGroupType() string {
var r string
if o.UserGroupTypePtr == nil {
return r
}
r = *o.UserGroupTypePtr
return r
}

// SetUserGroupType is a fluent style 'setter' method that can be chained
func (o *CifsShareAccessControlDeleteRequest) SetUserGroupType(newValue string) *CifsShareAccessControlDeleteRequest {
o.UserGroupTypePtr = &newValue
return o
}

// UserOrGroup is a 'getter' method
func (o *CifsShareAccessControlDeleteRequest) UserOrGroup() string {
var r string
if o.UserOrGroupPtr == nil {
return r
}
r = *o.UserOrGroupPtr
return r
}

// SetUserOrGroup is a fluent style 'setter' method that can be chained
func (o *CifsShareAccessControlDeleteRequest) SetUserOrGroup(newValue string) *CifsShareAccessControlDeleteRequest {
o.UserOrGroupPtr = &newValue
return o
}