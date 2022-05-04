// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// PlexAttributesType is a structure to represent a plex-attributes ZAPI object
type PlexAttributesType struct {
	XMLName                       xml.Name                      `xml:"plex-attributes"`
	IsOnlinePtr                   *bool                         `xml:"is-online"`
	IsResyncingPtr                *bool                         `xml:"is-resyncing"`
	PlexNamePtr                   *string                       `xml:"plex-name"`
	PlexResyncPctWithPrecisionPtr *string                       `xml:"plex-resync-pct-with-precision"`
	PlexStatusPtr                 *string                       `xml:"plex-status"`
	PoolPtr                       *int                          `xml:"pool"`
	RaidgroupsPtr                 *PlexAttributesTypeRaidgroups `xml:"raidgroups"`
	// work in progress
	ResyncLevelPtr         *int `xml:"resync-level"`
	ResyncingPercentagePtr *int `xml:"resyncing-percentage"`
}

// NewPlexAttributesType is a factory method for creating new instances of PlexAttributesType objects
func NewPlexAttributesType() *PlexAttributesType {
	return &PlexAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *PlexAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o PlexAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// IsOnline is a 'getter' method
func (o *PlexAttributesType) IsOnline() bool {
	r := *o.IsOnlinePtr
	return r
}

// SetIsOnline is a fluent style 'setter' method that can be chained
func (o *PlexAttributesType) SetIsOnline(newValue bool) *PlexAttributesType {
	o.IsOnlinePtr = &newValue
	return o
}

// IsResyncing is a 'getter' method
func (o *PlexAttributesType) IsResyncing() bool {
	r := *o.IsResyncingPtr
	return r
}

// SetIsResyncing is a fluent style 'setter' method that can be chained
func (o *PlexAttributesType) SetIsResyncing(newValue bool) *PlexAttributesType {
	o.IsResyncingPtr = &newValue
	return o
}

// PlexName is a 'getter' method
func (o *PlexAttributesType) PlexName() string {
	r := *o.PlexNamePtr
	return r
}

// SetPlexName is a fluent style 'setter' method that can be chained
func (o *PlexAttributesType) SetPlexName(newValue string) *PlexAttributesType {
	o.PlexNamePtr = &newValue
	return o
}

// PlexResyncPctWithPrecision is a 'getter' method
func (o *PlexAttributesType) PlexResyncPctWithPrecision() string {
	r := *o.PlexResyncPctWithPrecisionPtr
	return r
}

// SetPlexResyncPctWithPrecision is a fluent style 'setter' method that can be chained
func (o *PlexAttributesType) SetPlexResyncPctWithPrecision(newValue string) *PlexAttributesType {
	o.PlexResyncPctWithPrecisionPtr = &newValue
	return o
}

// PlexStatus is a 'getter' method
func (o *PlexAttributesType) PlexStatus() string {
	r := *o.PlexStatusPtr
	return r
}

// SetPlexStatus is a fluent style 'setter' method that can be chained
func (o *PlexAttributesType) SetPlexStatus(newValue string) *PlexAttributesType {
	o.PlexStatusPtr = &newValue
	return o
}

// Pool is a 'getter' method
func (o *PlexAttributesType) Pool() int {
	r := *o.PoolPtr
	return r
}

// SetPool is a fluent style 'setter' method that can be chained
func (o *PlexAttributesType) SetPool(newValue int) *PlexAttributesType {
	o.PoolPtr = &newValue
	return o
}

// PlexAttributesTypeRaidgroups is a wrapper
type PlexAttributesTypeRaidgroups struct {
	XMLName                xml.Name                  `xml:"raidgroups"`
	RaidgroupAttributesPtr []RaidgroupAttributesType `xml:"raidgroup-attributes"`
}

// RaidgroupAttributes is a 'getter' method
func (o *PlexAttributesTypeRaidgroups) RaidgroupAttributes() []RaidgroupAttributesType {
	r := o.RaidgroupAttributesPtr
	return r
}

// SetRaidgroupAttributes is a fluent style 'setter' method that can be chained
func (o *PlexAttributesTypeRaidgroups) SetRaidgroupAttributes(newValue []RaidgroupAttributesType) *PlexAttributesTypeRaidgroups {
	newSlice := make([]RaidgroupAttributesType, len(newValue))
	copy(newSlice, newValue)
	o.RaidgroupAttributesPtr = newSlice
	return o
}

// Raidgroups is a 'getter' method
func (o *PlexAttributesType) Raidgroups() PlexAttributesTypeRaidgroups {
	r := *o.RaidgroupsPtr
	return r
}

// SetRaidgroups is a fluent style 'setter' method that can be chained
func (o *PlexAttributesType) SetRaidgroups(newValue PlexAttributesTypeRaidgroups) *PlexAttributesType {
	o.RaidgroupsPtr = &newValue
	return o
}

// ResyncLevel is a 'getter' method
func (o *PlexAttributesType) ResyncLevel() int {
	r := *o.ResyncLevelPtr
	return r
}

// SetResyncLevel is a fluent style 'setter' method that can be chained
func (o *PlexAttributesType) SetResyncLevel(newValue int) *PlexAttributesType {
	o.ResyncLevelPtr = &newValue
	return o
}

// ResyncingPercentage is a 'getter' method
func (o *PlexAttributesType) ResyncingPercentage() int {
	r := *o.ResyncingPercentagePtr
	return r
}

// SetResyncingPercentage is a fluent style 'setter' method that can be chained
func (o *PlexAttributesType) SetResyncingPercentage(newValue int) *PlexAttributesType {
	o.ResyncingPercentagePtr = &newValue
	return o
}
