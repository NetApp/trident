package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// RaidgroupAttributesType is a structure to represent a raidgroup-attributes ZAPI object
type RaidgroupAttributesType struct {
	XMLName                        xml.Name `xml:"raidgroup-attributes"`
	ChecksumStylePtr               *string  `xml:"checksum-style"`
	IsCacheTierPtr                 *bool    `xml:"is-cache-tier"`
	IsRecomputingParityPtr         *bool    `xml:"is-recomputing-parity"`
	IsReconstructingPtr            *bool    `xml:"is-reconstructing"`
	RaidgroupNamePtr               *string  `xml:"raidgroup-name"`
	RecomputingParityPercentagePtr *int     `xml:"recomputing-parity-percentage"`
	ReconstructionPercentagePtr    *int     `xml:"reconstruction-percentage"`
}

// NewRaidgroupAttributesType is a factory method for creating new instances of RaidgroupAttributesType objects
func NewRaidgroupAttributesType() *RaidgroupAttributesType {
	return &RaidgroupAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *RaidgroupAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o RaidgroupAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// ChecksumStyle is a 'getter' method
func (o *RaidgroupAttributesType) ChecksumStyle() string {
	r := *o.ChecksumStylePtr
	return r
}

// SetChecksumStyle is a fluent style 'setter' method that can be chained
func (o *RaidgroupAttributesType) SetChecksumStyle(newValue string) *RaidgroupAttributesType {
	o.ChecksumStylePtr = &newValue
	return o
}

// IsCacheTier is a 'getter' method
func (o *RaidgroupAttributesType) IsCacheTier() bool {
	r := *o.IsCacheTierPtr
	return r
}

// SetIsCacheTier is a fluent style 'setter' method that can be chained
func (o *RaidgroupAttributesType) SetIsCacheTier(newValue bool) *RaidgroupAttributesType {
	o.IsCacheTierPtr = &newValue
	return o
}

// IsRecomputingParity is a 'getter' method
func (o *RaidgroupAttributesType) IsRecomputingParity() bool {
	r := *o.IsRecomputingParityPtr
	return r
}

// SetIsRecomputingParity is a fluent style 'setter' method that can be chained
func (o *RaidgroupAttributesType) SetIsRecomputingParity(newValue bool) *RaidgroupAttributesType {
	o.IsRecomputingParityPtr = &newValue
	return o
}

// IsReconstructing is a 'getter' method
func (o *RaidgroupAttributesType) IsReconstructing() bool {
	r := *o.IsReconstructingPtr
	return r
}

// SetIsReconstructing is a fluent style 'setter' method that can be chained
func (o *RaidgroupAttributesType) SetIsReconstructing(newValue bool) *RaidgroupAttributesType {
	o.IsReconstructingPtr = &newValue
	return o
}

// RaidgroupName is a 'getter' method
func (o *RaidgroupAttributesType) RaidgroupName() string {
	r := *o.RaidgroupNamePtr
	return r
}

// SetRaidgroupName is a fluent style 'setter' method that can be chained
func (o *RaidgroupAttributesType) SetRaidgroupName(newValue string) *RaidgroupAttributesType {
	o.RaidgroupNamePtr = &newValue
	return o
}

// RecomputingParityPercentage is a 'getter' method
func (o *RaidgroupAttributesType) RecomputingParityPercentage() int {
	r := *o.RecomputingParityPercentagePtr
	return r
}

// SetRecomputingParityPercentage is a fluent style 'setter' method that can be chained
func (o *RaidgroupAttributesType) SetRecomputingParityPercentage(newValue int) *RaidgroupAttributesType {
	o.RecomputingParityPercentagePtr = &newValue
	return o
}

// ReconstructionPercentage is a 'getter' method
func (o *RaidgroupAttributesType) ReconstructionPercentage() int {
	r := *o.ReconstructionPercentagePtr
	return r
}

// SetReconstructionPercentage is a fluent style 'setter' method that can be chained
func (o *RaidgroupAttributesType) SetReconstructionPercentage(newValue int) *RaidgroupAttributesType {
	o.ReconstructionPercentagePtr = &newValue
	return o
}
