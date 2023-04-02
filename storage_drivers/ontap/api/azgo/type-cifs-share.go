// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// CifsShareType is a structure to represent a cifs-share ZAPI object
type CifsShareType struct {
	XMLName xml.Name          `xml:"cifs-share"`
	AclPtr  *CifsShareTypeAcl `xml:"acl"`
	// work in progress
	AttributeCacheTtlPtr      *int                          `xml:"attribute-cache-ttl"`
	CifsServerPtr             *string                       `xml:"cifs-server"`
	CommentPtr                *string                       `xml:"comment"`
	DirUmaskPtr               *int                          `xml:"dir-umask"`
	FileUmaskPtr              *int                          `xml:"file-umask"`
	ForceGroupForCreatePtr    *string                       `xml:"force-group-for-create"`
	MaxConnectionsPerSharePtr *int                          `xml:"max-connections-per-share"`
	OfflineFilesModePtr       *string                       `xml:"offline-files-mode"`
	PathPtr                   *string                       `xml:"path"`
	ShareNamePtr              *string                       `xml:"share-name"`
	SharePropertiesPtr        *CifsShareTypeShareProperties `xml:"share-properties"`
	// work in progress
	SymlinkPropertiesPtr *CifsShareTypeSymlinkProperties `xml:"symlink-properties"`
	// work in progress
	VolumePtr             *string                 `xml:"volume"`
	VscanFileopProfilePtr *VscanFileopProfileType `xml:"vscan-fileop-profile"`
	VserverPtr            *string                 `xml:"vserver"`
}

// NewCifsShareType is a factory method for creating new instances of CifsShareType objects
func NewCifsShareType() *CifsShareType {
	return &CifsShareType{}
}

// ToXML converts this object into an xml string representation
func (o *CifsShareType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CifsShareType) String() string {
	return ToString(reflect.ValueOf(o))
}

// CifsShareTypeAcl is a wrapper
type CifsShareTypeAcl struct {
	XMLName   xml.Name `xml:"acl"`
	StringPtr []string `xml:"string"`
}

// String is a 'getter' method
func (o *CifsShareTypeAcl) String() []string {
	r := o.StringPtr
	return r
}

// SetString is a fluent style 'setter' method that can be chained
func (o *CifsShareTypeAcl) SetString(newValue []string) *CifsShareTypeAcl {
	newSlice := make([]string, len(newValue))
	copy(newSlice, newValue)
	o.StringPtr = newSlice
	return o
}

// Acl is a 'getter' method
func (o *CifsShareType) Acl() CifsShareTypeAcl {
	var r CifsShareTypeAcl
	if o.AclPtr == nil {
		return r
	}
	r = *o.AclPtr
	return r
}

// SetAcl is a fluent style 'setter' method that can be chained
func (o *CifsShareType) SetAcl(newValue CifsShareTypeAcl) *CifsShareType {
	o.AclPtr = &newValue
	return o
}

// AttributeCacheTtl is a 'getter' method
func (o *CifsShareType) AttributeCacheTtl() int {
	var r int
	if o.AttributeCacheTtlPtr == nil {
		return r
	}
	r = *o.AttributeCacheTtlPtr
	return r
}

// SetAttributeCacheTtl is a fluent style 'setter' method that can be chained
func (o *CifsShareType) SetAttributeCacheTtl(newValue int) *CifsShareType {
	o.AttributeCacheTtlPtr = &newValue
	return o
}

// CifsServer is a 'getter' method
func (o *CifsShareType) CifsServer() string {
	var r string
	if o.CifsServerPtr == nil {
		return r
	}
	r = *o.CifsServerPtr
	return r
}

// SetCifsServer is a fluent style 'setter' method that can be chained
func (o *CifsShareType) SetCifsServer(newValue string) *CifsShareType {
	o.CifsServerPtr = &newValue
	return o
}

// Comment is a 'getter' method
func (o *CifsShareType) Comment() string {
	var r string
	if o.CommentPtr == nil {
		return r
	}
	r = *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *CifsShareType) SetComment(newValue string) *CifsShareType {
	o.CommentPtr = &newValue
	return o
}

// DirUmask is a 'getter' method
func (o *CifsShareType) DirUmask() int {
	var r int
	if o.DirUmaskPtr == nil {
		return r
	}
	r = *o.DirUmaskPtr
	return r
}

// SetDirUmask is a fluent style 'setter' method that can be chained
func (o *CifsShareType) SetDirUmask(newValue int) *CifsShareType {
	o.DirUmaskPtr = &newValue
	return o
}

// FileUmask is a 'getter' method
func (o *CifsShareType) FileUmask() int {
	var r int
	if o.FileUmaskPtr == nil {
		return r
	}
	r = *o.FileUmaskPtr
	return r
}

// SetFileUmask is a fluent style 'setter' method that can be chained
func (o *CifsShareType) SetFileUmask(newValue int) *CifsShareType {
	o.FileUmaskPtr = &newValue
	return o
}

// ForceGroupForCreate is a 'getter' method
func (o *CifsShareType) ForceGroupForCreate() string {
	var r string
	if o.ForceGroupForCreatePtr == nil {
		return r
	}
	r = *o.ForceGroupForCreatePtr
	return r
}

// SetForceGroupForCreate is a fluent style 'setter' method that can be chained
func (o *CifsShareType) SetForceGroupForCreate(newValue string) *CifsShareType {
	o.ForceGroupForCreatePtr = &newValue
	return o
}

// MaxConnectionsPerShare is a 'getter' method
func (o *CifsShareType) MaxConnectionsPerShare() int {
	var r int
	if o.MaxConnectionsPerSharePtr == nil {
		return r
	}
	r = *o.MaxConnectionsPerSharePtr
	return r
}

// SetMaxConnectionsPerShare is a fluent style 'setter' method that can be chained
func (o *CifsShareType) SetMaxConnectionsPerShare(newValue int) *CifsShareType {
	o.MaxConnectionsPerSharePtr = &newValue
	return o
}

// OfflineFilesMode is a 'getter' method
func (o *CifsShareType) OfflineFilesMode() string {
	var r string
	if o.OfflineFilesModePtr == nil {
		return r
	}
	r = *o.OfflineFilesModePtr
	return r
}

// SetOfflineFilesMode is a fluent style 'setter' method that can be chained
func (o *CifsShareType) SetOfflineFilesMode(newValue string) *CifsShareType {
	o.OfflineFilesModePtr = &newValue
	return o
}

// Path is a 'getter' method
func (o *CifsShareType) Path() string {
	var r string
	if o.PathPtr == nil {
		return r
	}
	r = *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *CifsShareType) SetPath(newValue string) *CifsShareType {
	o.PathPtr = &newValue
	return o
}

// ShareName is a 'getter' method
func (o *CifsShareType) ShareName() string {
	var r string
	if o.ShareNamePtr == nil {
		return r
	}
	r = *o.ShareNamePtr
	return r
}

// SetShareName is a fluent style 'setter' method that can be chained
func (o *CifsShareType) SetShareName(newValue string) *CifsShareType {
	o.ShareNamePtr = &newValue
	return o
}

// CifsShareTypeShareProperties is a wrapper
type CifsShareTypeShareProperties struct {
	XMLName                xml.Name                  `xml:"share-properties"`
	CifsSharePropertiesPtr []CifsSharePropertiesType `xml:"cifs-share-properties"`
}

// CifsShareProperties is a 'getter' method
func (o *CifsShareTypeShareProperties) CifsShareProperties() []CifsSharePropertiesType {
	r := o.CifsSharePropertiesPtr
	return r
}

// SetCifsShareProperties is a fluent style 'setter' method that can be chained
func (o *CifsShareTypeShareProperties) SetCifsShareProperties(newValue []CifsSharePropertiesType) *CifsShareTypeShareProperties {
	newSlice := make([]CifsSharePropertiesType, len(newValue))
	copy(newSlice, newValue)
	o.CifsSharePropertiesPtr = newSlice
	return o
}

// ShareProperties is a 'getter' method
func (o *CifsShareType) ShareProperties() CifsShareTypeShareProperties {
	var r CifsShareTypeShareProperties
	if o.SharePropertiesPtr == nil {
		return r
	}
	r = *o.SharePropertiesPtr
	return r
}

// SetShareProperties is a fluent style 'setter' method that can be chained
func (o *CifsShareType) SetShareProperties(newValue CifsShareTypeShareProperties) *CifsShareType {
	o.SharePropertiesPtr = &newValue
	return o
}

// CifsShareTypeSymlinkProperties is a wrapper
type CifsShareTypeSymlinkProperties struct {
	XMLName                       xml.Name                         `xml:"symlink-properties"`
	CifsShareSymlinkPropertiesPtr []CifsShareSymlinkPropertiesType `xml:"cifs-share-symlink-properties"`
}

// CifsShareSymlinkProperties is a 'getter' method
func (o *CifsShareTypeSymlinkProperties) CifsShareSymlinkProperties() []CifsShareSymlinkPropertiesType {
	r := o.CifsShareSymlinkPropertiesPtr
	return r
}

// SetCifsShareSymlinkProperties is a fluent style 'setter' method that can be chained
func (o *CifsShareTypeSymlinkProperties) SetCifsShareSymlinkProperties(newValue []CifsShareSymlinkPropertiesType) *CifsShareTypeSymlinkProperties {
	newSlice := make([]CifsShareSymlinkPropertiesType, len(newValue))
	copy(newSlice, newValue)
	o.CifsShareSymlinkPropertiesPtr = newSlice
	return o
}

// SymlinkProperties is a 'getter' method
func (o *CifsShareType) SymlinkProperties() CifsShareTypeSymlinkProperties {
	var r CifsShareTypeSymlinkProperties
	if o.SymlinkPropertiesPtr == nil {
		return r
	}
	r = *o.SymlinkPropertiesPtr
	return r
}

// SetSymlinkProperties is a fluent style 'setter' method that can be chained
func (o *CifsShareType) SetSymlinkProperties(newValue CifsShareTypeSymlinkProperties) *CifsShareType {
	o.SymlinkPropertiesPtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *CifsShareType) Volume() string {
	var r string
	if o.VolumePtr == nil {
		return r
	}
	r = *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *CifsShareType) SetVolume(newValue string) *CifsShareType {
	o.VolumePtr = &newValue
	return o
}

// VscanFileopProfile is a 'getter' method
func (o *CifsShareType) VscanFileopProfile() VscanFileopProfileType {
	var r VscanFileopProfileType
	if o.VscanFileopProfilePtr == nil {
		return r
	}
	r = *o.VscanFileopProfilePtr
	return r
}

// SetVscanFileopProfile is a fluent style 'setter' method that can be chained
func (o *CifsShareType) SetVscanFileopProfile(newValue VscanFileopProfileType) *CifsShareType {
	o.VscanFileopProfilePtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *CifsShareType) Vserver() string {
	var r string
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *CifsShareType) SetVserver(newValue string) *CifsShareType {
	o.VserverPtr = &newValue
	return o
}
