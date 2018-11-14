package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// QtreeRenameRequest is a structure to represent a qtree-rename Request ZAPI object
type QtreeRenameRequest struct {
	XMLName         xml.Name `xml:"qtree-rename"`
	NewQtreeNamePtr *string  `xml:"new-qtree-name"`
	QtreePtr        *string  `xml:"qtree"`
}

// QtreeRenameResponse is a structure to represent a qtree-rename Response ZAPI object
type QtreeRenameResponse struct {
	XMLName         xml.Name                  `xml:"netapp"`
	ResponseVersion string                    `xml:"version,attr"`
	ResponseXmlns   string                    `xml:"xmlns,attr"`
	Result          QtreeRenameResponseResult `xml:"results"`
}

// NewQtreeRenameResponse is a factory method for creating new instances of QtreeRenameResponse objects
func NewQtreeRenameResponse() *QtreeRenameResponse {
	return &QtreeRenameResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeRenameResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *QtreeRenameResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// QtreeRenameResponseResult is a structure to represent a qtree-rename Response Result ZAPI object
type QtreeRenameResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewQtreeRenameRequest is a factory method for creating new instances of QtreeRenameRequest objects
func NewQtreeRenameRequest() *QtreeRenameRequest {
	return &QtreeRenameRequest{}
}

// NewQtreeRenameResponseResult is a factory method for creating new instances of QtreeRenameResponseResult objects
func NewQtreeRenameResponseResult() *QtreeRenameResponseResult {
	return &QtreeRenameResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *QtreeRenameRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *QtreeRenameResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeRenameRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeRenameResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *QtreeRenameRequest) ExecuteUsing(zr *ZapiRunner) (*QtreeRenameResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *QtreeRenameRequest) executeWithoutIteration(zr *ZapiRunner) (*QtreeRenameResponse, error) {
	result, err := zr.ExecuteUsing(o, "QtreeRenameRequest", NewQtreeRenameResponse())
	return result.(*QtreeRenameResponse), err
}

// NewQtreeName is a 'getter' method
func (o *QtreeRenameRequest) NewQtreeName() string {
	r := *o.NewQtreeNamePtr
	return r
}

// SetNewQtreeName is a fluent style 'setter' method that can be chained
func (o *QtreeRenameRequest) SetNewQtreeName(newValue string) *QtreeRenameRequest {
	o.NewQtreeNamePtr = &newValue
	return o
}

// Qtree is a 'getter' method
func (o *QtreeRenameRequest) Qtree() string {
	r := *o.QtreePtr
	return r
}

// SetQtree is a fluent style 'setter' method that can be chained
func (o *QtreeRenameRequest) SetQtree(newValue string) *QtreeRenameRequest {
	o.QtreePtr = &newValue
	return o
}
