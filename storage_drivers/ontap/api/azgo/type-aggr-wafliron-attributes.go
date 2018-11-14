package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// AggrWaflironAttributesType is a structure to represent a aggr-wafliron-attributes ZAPI object
type AggrWaflironAttributesType struct {
	XMLName                  xml.Name `xml:"aggr-wafliron-attributes"`
	LastStartErrnoPtr        *int     `xml:"last-start-errno"`
	LastStartErrorInfoPtr    *string  `xml:"last-start-error-info"`
	ScanPercentagePtr        *int     `xml:"scan-percentage"`
	StatePtr                 *string  `xml:"state"`
	SummaryScanPercentagePtr *int     `xml:"summary-scan-percentage"`
}

// NewAggrWaflironAttributesType is a factory method for creating new instances of AggrWaflironAttributesType objects
func NewAggrWaflironAttributesType() *AggrWaflironAttributesType {
	return &AggrWaflironAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *AggrWaflironAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrWaflironAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// LastStartErrno is a 'getter' method
func (o *AggrWaflironAttributesType) LastStartErrno() int {
	r := *o.LastStartErrnoPtr
	return r
}

// SetLastStartErrno is a fluent style 'setter' method that can be chained
func (o *AggrWaflironAttributesType) SetLastStartErrno(newValue int) *AggrWaflironAttributesType {
	o.LastStartErrnoPtr = &newValue
	return o
}

// LastStartErrorInfo is a 'getter' method
func (o *AggrWaflironAttributesType) LastStartErrorInfo() string {
	r := *o.LastStartErrorInfoPtr
	return r
}

// SetLastStartErrorInfo is a fluent style 'setter' method that can be chained
func (o *AggrWaflironAttributesType) SetLastStartErrorInfo(newValue string) *AggrWaflironAttributesType {
	o.LastStartErrorInfoPtr = &newValue
	return o
}

// ScanPercentage is a 'getter' method
func (o *AggrWaflironAttributesType) ScanPercentage() int {
	r := *o.ScanPercentagePtr
	return r
}

// SetScanPercentage is a fluent style 'setter' method that can be chained
func (o *AggrWaflironAttributesType) SetScanPercentage(newValue int) *AggrWaflironAttributesType {
	o.ScanPercentagePtr = &newValue
	return o
}

// State is a 'getter' method
func (o *AggrWaflironAttributesType) State() string {
	r := *o.StatePtr
	return r
}

// SetState is a fluent style 'setter' method that can be chained
func (o *AggrWaflironAttributesType) SetState(newValue string) *AggrWaflironAttributesType {
	o.StatePtr = &newValue
	return o
}

// SummaryScanPercentage is a 'getter' method
func (o *AggrWaflironAttributesType) SummaryScanPercentage() int {
	r := *o.SummaryScanPercentagePtr
	return r
}

// SetSummaryScanPercentage is a fluent style 'setter' method that can be chained
func (o *AggrWaflironAttributesType) SetSummaryScanPercentage(newValue int) *AggrWaflironAttributesType {
	o.SummaryScanPercentagePtr = &newValue
	return o
}
