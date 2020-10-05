package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// IscsiInitiatorListEntryInfoType is a structure to represent a iscsi-initiator-list-entry-info ZAPI object
type IscsiInitiatorListEntryInfoType struct {
	XMLName               xml.Name                                           `xml:"iscsi-initiator-list-entry-info"`
	InitiatorAliasnamePtr *string                                            `xml:"initiator-aliasname"`
	InitiatorGroupListPtr *IscsiInitiatorListEntryInfoTypeInitiatorGroupList `xml:"initiator-group-list"`
	// work in progress
	InitiatorNodenamePtr *string `xml:"initiator-nodename"`
	IsidPtr              *string `xml:"isid"`
	TargetSessionIdPtr   *int    `xml:"target-session-id"`
	TpgroupNamePtr       *string `xml:"tpgroup-name"`
	TpgroupTagPtr        *int    `xml:"tpgroup-tag"`
	VserverPtr           *string `xml:"vserver"`
}

// NewIscsiInitiatorListEntryInfoType is a factory method for creating new instances of IscsiInitiatorListEntryInfoType objects
func NewIscsiInitiatorListEntryInfoType() *IscsiInitiatorListEntryInfoType {
	return &IscsiInitiatorListEntryInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorListEntryInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorListEntryInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// InitiatorAliasname is a 'getter' method
func (o *IscsiInitiatorListEntryInfoType) InitiatorAliasname() string {
	r := *o.InitiatorAliasnamePtr
	return r
}

// SetInitiatorAliasname is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorListEntryInfoType) SetInitiatorAliasname(newValue string) *IscsiInitiatorListEntryInfoType {
	o.InitiatorAliasnamePtr = &newValue
	return o
}

// IscsiInitiatorListEntryInfoTypeInitiatorGroupList is a wrapper
type IscsiInitiatorListEntryInfoTypeInitiatorGroupList struct {
	XMLName                   xml.Name                     `xml:"initiator-group-list"`
	InitiatorGroupListInfoPtr []InitiatorGroupListInfoType `xml:"initiator-group-list-info"`
}

// InitiatorGroupListInfo is a 'getter' method
func (o *IscsiInitiatorListEntryInfoTypeInitiatorGroupList) InitiatorGroupListInfo() []InitiatorGroupListInfoType {
	r := o.InitiatorGroupListInfoPtr
	return r
}

// SetInitiatorGroupListInfo is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorListEntryInfoTypeInitiatorGroupList) SetInitiatorGroupListInfo(newValue []InitiatorGroupListInfoType) *IscsiInitiatorListEntryInfoTypeInitiatorGroupList {
	newSlice := make([]InitiatorGroupListInfoType, len(newValue))
	copy(newSlice, newValue)
	o.InitiatorGroupListInfoPtr = newSlice
	return o
}

// InitiatorGroupList is a 'getter' method
func (o *IscsiInitiatorListEntryInfoType) InitiatorGroupList() IscsiInitiatorListEntryInfoTypeInitiatorGroupList {
	r := *o.InitiatorGroupListPtr
	return r
}

// SetInitiatorGroupList is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorListEntryInfoType) SetInitiatorGroupList(newValue IscsiInitiatorListEntryInfoTypeInitiatorGroupList) *IscsiInitiatorListEntryInfoType {
	o.InitiatorGroupListPtr = &newValue
	return o
}

// InitiatorNodename is a 'getter' method
func (o *IscsiInitiatorListEntryInfoType) InitiatorNodename() string {
	r := *o.InitiatorNodenamePtr
	return r
}

// SetInitiatorNodename is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorListEntryInfoType) SetInitiatorNodename(newValue string) *IscsiInitiatorListEntryInfoType {
	o.InitiatorNodenamePtr = &newValue
	return o
}

// Isid is a 'getter' method
func (o *IscsiInitiatorListEntryInfoType) Isid() string {
	r := *o.IsidPtr
	return r
}

// SetIsid is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorListEntryInfoType) SetIsid(newValue string) *IscsiInitiatorListEntryInfoType {
	o.IsidPtr = &newValue
	return o
}

// TargetSessionId is a 'getter' method
func (o *IscsiInitiatorListEntryInfoType) TargetSessionId() int {
	r := *o.TargetSessionIdPtr
	return r
}

// SetTargetSessionId is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorListEntryInfoType) SetTargetSessionId(newValue int) *IscsiInitiatorListEntryInfoType {
	o.TargetSessionIdPtr = &newValue
	return o
}

// TpgroupName is a 'getter' method
func (o *IscsiInitiatorListEntryInfoType) TpgroupName() string {
	r := *o.TpgroupNamePtr
	return r
}

// SetTpgroupName is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorListEntryInfoType) SetTpgroupName(newValue string) *IscsiInitiatorListEntryInfoType {
	o.TpgroupNamePtr = &newValue
	return o
}

// TpgroupTag is a 'getter' method
func (o *IscsiInitiatorListEntryInfoType) TpgroupTag() int {
	r := *o.TpgroupTagPtr
	return r
}

// SetTpgroupTag is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorListEntryInfoType) SetTpgroupTag(newValue int) *IscsiInitiatorListEntryInfoType {
	o.TpgroupTagPtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *IscsiInitiatorListEntryInfoType) Vserver() string {
	r := *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorListEntryInfoType) SetVserver(newValue string) *IscsiInitiatorListEntryInfoType {
	o.VserverPtr = &newValue
	return o
}
