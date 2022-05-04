// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// BlockRangeType is a structure to represent a block-range ZAPI object
type BlockRangeType struct {
	XMLName                   xml.Name `xml:"block-range"`
	BlockCountPtr             *int     `xml:"block-count"`
	DestinationBlockNumberPtr *int     `xml:"destination-block-number"`
	SourceBlockNumberPtr      *int     `xml:"source-block-number"`
}

// NewBlockRangeType is a factory method for creating new instances of BlockRangeType objects
func NewBlockRangeType() *BlockRangeType {
	return &BlockRangeType{}
}

// ToXML converts this object into an xml string representation
func (o *BlockRangeType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o BlockRangeType) String() string {
	return ToString(reflect.ValueOf(o))
}

// BlockCount is a 'getter' method
func (o *BlockRangeType) BlockCount() int {
	r := *o.BlockCountPtr
	return r
}

// SetBlockCount is a fluent style 'setter' method that can be chained
func (o *BlockRangeType) SetBlockCount(newValue int) *BlockRangeType {
	o.BlockCountPtr = &newValue
	return o
}

// DestinationBlockNumber is a 'getter' method
func (o *BlockRangeType) DestinationBlockNumber() int {
	r := *o.DestinationBlockNumberPtr
	return r
}

// SetDestinationBlockNumber is a fluent style 'setter' method that can be chained
func (o *BlockRangeType) SetDestinationBlockNumber(newValue int) *BlockRangeType {
	o.DestinationBlockNumberPtr = &newValue
	return o
}

// SourceBlockNumber is a 'getter' method
func (o *BlockRangeType) SourceBlockNumber() int {
	r := *o.SourceBlockNumberPtr
	return r
}

// SetSourceBlockNumber is a fluent style 'setter' method that can be chained
func (o *BlockRangeType) SetSourceBlockNumber(newValue int) *BlockRangeType {
	o.SourceBlockNumberPtr = &newValue
	return o
}
