// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapmirrorDestinationInfoType is a structure to represent a snapmirror-destination-info ZAPI object
// WARNING: Keep unint/unint64 types as it is, changing them to some other type may cause failures
type SnapmirrorDestinationInfoType struct {
	XMLName                  xml.Name
	DestinationLocationPtr   *string `xml:"destination-location"`
	DestinationVolumePtr     *string `xml:"destination-volume"`
	DestinationVserverPtr    *string `xml:"destination-vserver"`
	IsConstituentPtr         *bool   `xml:"is-constituent"`
	PolicyTypePtr            *string `xml:"policy-type"`
	ProgressLastUpdatedPtr   *uint   `xml:"progress-last-updated"`
	RelationshipGroupTypePtr *string `xml:"relationship-group-type"`
	RelationshipIdPtr        *string `xml:"relationship-id"`
	RelationshipStatusPtr    *string `xml:"relationship-status"`
	RelationshipTypePtr      *string `xml:"relationship-type"`
	SourceLocationPtr        *string `xml:"source-location"`
	SourceVolumePtr          *string `xml:"source-volume"`
	SourceVolumeNodePtr      *string `xml:"source-volume-node"`
	SourceVserverPtr         *string `xml:"source-vserver"`
	TransferProgressPtr      *uint64 `xml:"transfer-progress"`
}

// NewSnapmirrorDestinationInfoType is a factory method for creating new instances of SnapmirrorDestinationInfoType objects
func NewSnapmirrorDestinationInfoType() *SnapmirrorDestinationInfoType {
	return &SnapmirrorDestinationInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorDestinationInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorDestinationInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// DestinationLocation is a 'getter' method
func (o *SnapmirrorDestinationInfoType) DestinationLocation() string {
	r := *o.DestinationLocationPtr
	return r
}

// SetDestinationLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetDestinationLocation(newValue string) *SnapmirrorDestinationInfoType {
	o.DestinationLocationPtr = &newValue
	return o
}

// DestinationVolume is a 'getter' method
func (o *SnapmirrorDestinationInfoType) DestinationVolume() string {
	r := *o.DestinationVolumePtr
	return r
}

// SetDestinationVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetDestinationVolume(newValue string) *SnapmirrorDestinationInfoType {
	o.DestinationVolumePtr = &newValue
	return o
}

// DestinationVserver is a 'getter' method
func (o *SnapmirrorDestinationInfoType) DestinationVserver() string {
	r := *o.DestinationVserverPtr
	return r
}

// SetDestinationVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetDestinationVserver(newValue string) *SnapmirrorDestinationInfoType {
	o.DestinationVserverPtr = &newValue
	return o
}

// SnapmirrorDestinationInfoTypeFileRestoreFileList is a wrapper
type SnapmirrorDestinationInfoTypeFileRestoreFileList struct {
	XMLName   xml.Name `xml:"file-restore-file-list"`
	StringPtr []string `xml:"string"`
}

// String is a 'getter' method
func (o *SnapmirrorDestinationInfoTypeFileRestoreFileList) String() []string {
	r := o.StringPtr
	return r
}

// SetString is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoTypeFileRestoreFileList) SetString(newValue []string) *SnapmirrorDestinationInfoTypeFileRestoreFileList {
	newSlice := make([]string, len(newValue))
	copy(newSlice, newValue)
	o.StringPtr = newSlice
	return o
}

// IsConstituent is a 'getter' method
func (o *SnapmirrorDestinationInfoType) IsConstituent() bool {
	r := *o.IsConstituentPtr
	return r
}

// SetIsConstituent is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetIsConstituent(newValue bool) *SnapmirrorDestinationInfoType {
	o.IsConstituentPtr = &newValue
	return o
}

// SnapmirrorDestinationInfoTypeLastTransferErrorCodes is a wrapper
type SnapmirrorDestinationInfoTypeLastTransferErrorCodes struct {
	XMLName    xml.Name `xml:"last-transfer-error-codes"`
	IntegerPtr []int    `xml:"integer"`
}

// Integer is a 'getter' method
func (o *SnapmirrorDestinationInfoTypeLastTransferErrorCodes) Integer() []int {
	r := o.IntegerPtr
	return r
}

// SetInteger is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoTypeLastTransferErrorCodes) SetInteger(newValue []int) *SnapmirrorDestinationInfoTypeLastTransferErrorCodes {
	newSlice := make([]int, len(newValue))
	copy(newSlice, newValue)
	o.IntegerPtr = newSlice
	return o
}

// PolicyType is a 'getter' method
func (o *SnapmirrorDestinationInfoType) PolicyType() string {
	r := *o.PolicyTypePtr
	return r
}

// SetPolicyType is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetPolicyType(newValue string) *SnapmirrorDestinationInfoType {
	o.PolicyTypePtr = &newValue
	return o
}

// ProgressLastUpdated is a 'getter' method
func (o *SnapmirrorDestinationInfoType) ProgressLastUpdated() uint {
	r := *o.ProgressLastUpdatedPtr
	return r
}

// SetProgressLastUpdated is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetProgressLastUpdated(newValue uint) *SnapmirrorDestinationInfoType {
	o.ProgressLastUpdatedPtr = &newValue
	return o
}

// RelationshipGroupType is a 'getter' method
func (o *SnapmirrorDestinationInfoType) RelationshipGroupType() string {
	r := *o.RelationshipGroupTypePtr
	return r
}

// SetRelationshipGroupType is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetRelationshipGroupType(newValue string) *SnapmirrorDestinationInfoType {
	o.RelationshipGroupTypePtr = &newValue
	return o
}

// RelationshipId is a 'getter' method
func (o *SnapmirrorDestinationInfoType) RelationshipId() string {
	r := *o.RelationshipIdPtr
	return r
}

// SetRelationshipId is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetRelationshipId(newValue string) *SnapmirrorDestinationInfoType {
	o.RelationshipIdPtr = &newValue
	return o
}

// RelationshipStatus is a 'getter' method
func (o *SnapmirrorDestinationInfoType) RelationshipStatus() string {
	r := *o.RelationshipStatusPtr
	return r
}

// SetRelationshipStatus is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetRelationshipStatus(newValue string) *SnapmirrorDestinationInfoType {
	o.RelationshipStatusPtr = &newValue
	return o
}

// RelationshipType is a 'getter' method
func (o *SnapmirrorDestinationInfoType) RelationshipType() string {
	r := *o.RelationshipTypePtr
	return r
}

// SetRelationshipType is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetRelationshipType(newValue string) *SnapmirrorDestinationInfoType {
	o.RelationshipTypePtr = &newValue
	return o
}

// SourceLocation is a 'getter' method
func (o *SnapmirrorDestinationInfoType) SourceLocation() string {
	r := *o.SourceLocationPtr
	return r
}

// SetSourceLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetSourceLocation(newValue string) *SnapmirrorDestinationInfoType {
	o.SourceLocationPtr = &newValue
	return o
}

// SourceVolume is a 'getter' method
func (o *SnapmirrorDestinationInfoType) SourceVolume() string {
	r := *o.SourceVolumePtr
	return r
}

// SetSourceVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetSourceVolume(newValue string) *SnapmirrorDestinationInfoType {
	o.SourceVolumePtr = &newValue
	return o
}

// SourceVolumeNode is a 'getter' method
func (o *SnapmirrorDestinationInfoType) SourceVolumeNode() string {
	r := *o.SourceVolumeNodePtr
	return r
}

// SetSourceVolumeNode is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetSourceVolumeNode(newValue string) *SnapmirrorDestinationInfoType {
	o.SourceVolumeNodePtr = &newValue
	return o
}

// SourceVserver is a 'getter' method
func (o *SnapmirrorDestinationInfoType) SourceVserver() string {
	r := *o.SourceVserverPtr
	return r
}

// SetSourceVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetSourceVserver(newValue string) *SnapmirrorDestinationInfoType {
	o.SourceVserverPtr = &newValue
	return o
}

// TransferProgress is a 'getter' method
func (o *SnapmirrorDestinationInfoType) TransferProgress() uint64 {
	r := *o.TransferProgressPtr
	return r
}

// SetTransferProgress is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetTransferProgress(newValue uint64) *SnapmirrorDestinationInfoType {
	o.TransferProgressPtr = &newValue
	return o
}
