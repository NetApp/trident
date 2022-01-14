package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VserverPeerGetIterRequest is a structure to represent a vserver-peer-get-iter Request ZAPI object
type VserverPeerGetIterRequest struct {
	XMLName              xml.Name                                    `xml:"vserver-peer-get-iter"`
	DesiredAttributesPtr *VserverPeerGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                        `xml:"max-records"`
	QueryPtr             *VserverPeerGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                     `xml:"tag"`
}

// VserverPeerGetIterResponse is a structure to represent a vserver-peer-get-iter Response ZAPI object
type VserverPeerGetIterResponse struct {
	XMLName         xml.Name                         `xml:"netapp"`
	ResponseVersion string                           `xml:"version,attr"`
	ResponseXmlns   string                           `xml:"xmlns,attr"`
	Result          VserverPeerGetIterResponseResult `xml:"results"`
}

// NewVserverPeerGetIterResponse is a factory method for creating new instances of VserverPeerGetIterResponse objects
func NewVserverPeerGetIterResponse() *VserverPeerGetIterResponse {
	return &VserverPeerGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverPeerGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VserverPeerGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VserverPeerGetIterResponseResult is a structure to represent a vserver-peer-get-iter Response Result ZAPI object
type VserverPeerGetIterResponseResult struct {
	XMLName           xml.Name                                        `xml:"results"`
	ResultStatusAttr  string                                          `xml:"status,attr"`
	ResultReasonAttr  string                                          `xml:"reason,attr"`
	ResultErrnoAttr   string                                          `xml:"errno,attr"`
	AttributesListPtr *VserverPeerGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                         `xml:"next-tag"`
	NumRecordsPtr     *int                                            `xml:"num-records"`
}

// NewVserverPeerGetIterRequest is a factory method for creating new instances of VserverPeerGetIterRequest objects
func NewVserverPeerGetIterRequest() *VserverPeerGetIterRequest {
	return &VserverPeerGetIterRequest{}
}

// NewVserverPeerGetIterResponseResult is a factory method for creating new instances of VserverPeerGetIterResponseResult objects
func NewVserverPeerGetIterResponseResult() *VserverPeerGetIterResponseResult {
	return &VserverPeerGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VserverPeerGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VserverPeerGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverPeerGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverPeerGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VserverPeerGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*VserverPeerGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VserverPeerGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*VserverPeerGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "VserverPeerGetIterRequest", NewVserverPeerGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*VserverPeerGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VserverPeerGetIterRequest) executeWithIteration(zr *ZapiRunner) (*VserverPeerGetIterResponse, error) {
	combined := NewVserverPeerGetIterResponse()
	combined.Result.SetAttributesList(VserverPeerGetIterResponseResultAttributesList{})
	var nextTagPtr *string
	done := false
	for done != true {
		n, err := o.executeWithoutIteration(zr)

		if err != nil {
			return nil, err
		}
		nextTagPtr = n.Result.NextTagPtr
		if nextTagPtr == nil {
			done = true
		} else {
			o.SetTag(*nextTagPtr)
		}

		if n.Result.NumRecordsPtr == nil {
			done = true
		} else {
			recordsRead := n.Result.NumRecords()
			if recordsRead == 0 {
				done = true
			}
		}

		if n.Result.AttributesListPtr != nil {
			if combined.Result.AttributesListPtr == nil {
				combined.Result.SetAttributesList(VserverPeerGetIterResponseResultAttributesList{})
			}
			combinedAttributesList := combined.Result.AttributesList()
			combinedAttributes := combinedAttributesList.values()

			resultAttributesList := n.Result.AttributesList()
			resultAttributes := resultAttributesList.values()

			combined.Result.AttributesListPtr.setValues(append(combinedAttributes, resultAttributes...))
		}

		if done == true {

			combined.Result.ResultErrnoAttr = n.Result.ResultErrnoAttr
			combined.Result.ResultReasonAttr = n.Result.ResultReasonAttr
			combined.Result.ResultStatusAttr = n.Result.ResultStatusAttr

			combinedAttributesList := combined.Result.AttributesList()
			combinedAttributes := combinedAttributesList.values()
			combined.Result.SetNumRecords(len(combinedAttributes))

		}
	}
	return combined, nil
}

// VserverPeerGetIterRequestDesiredAttributes is a wrapper
type VserverPeerGetIterRequestDesiredAttributes struct {
	XMLName            xml.Name             `xml:"desired-attributes"`
	VserverPeerInfoPtr *VserverPeerInfoType `xml:"vserver-peer-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverPeerGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// VserverPeerInfo is a 'getter' method
func (o *VserverPeerGetIterRequestDesiredAttributes) VserverPeerInfo() VserverPeerInfoType {
	r := *o.VserverPeerInfoPtr
	return r
}

// SetVserverPeerInfo is a fluent style 'setter' method that can be chained
func (o *VserverPeerGetIterRequestDesiredAttributes) SetVserverPeerInfo(newValue VserverPeerInfoType) *VserverPeerGetIterRequestDesiredAttributes {
	o.VserverPeerInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *VserverPeerGetIterRequest) DesiredAttributes() VserverPeerGetIterRequestDesiredAttributes {
	r := *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *VserverPeerGetIterRequest) SetDesiredAttributes(newValue VserverPeerGetIterRequestDesiredAttributes) *VserverPeerGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *VserverPeerGetIterRequest) MaxRecords() int {
	r := *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *VserverPeerGetIterRequest) SetMaxRecords(newValue int) *VserverPeerGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// VserverPeerGetIterRequestQuery is a wrapper
type VserverPeerGetIterRequestQuery struct {
	XMLName            xml.Name             `xml:"query"`
	VserverPeerInfoPtr *VserverPeerInfoType `xml:"vserver-peer-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverPeerGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// VserverPeerInfo is a 'getter' method
func (o *VserverPeerGetIterRequestQuery) VserverPeerInfo() VserverPeerInfoType {
	r := *o.VserverPeerInfoPtr
	return r
}

// SetVserverPeerInfo is a fluent style 'setter' method that can be chained
func (o *VserverPeerGetIterRequestQuery) SetVserverPeerInfo(newValue VserverPeerInfoType) *VserverPeerGetIterRequestQuery {
	o.VserverPeerInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *VserverPeerGetIterRequest) Query() VserverPeerGetIterRequestQuery {
	r := *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *VserverPeerGetIterRequest) SetQuery(newValue VserverPeerGetIterRequestQuery) *VserverPeerGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *VserverPeerGetIterRequest) Tag() string {
	r := *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *VserverPeerGetIterRequest) SetTag(newValue string) *VserverPeerGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// VserverPeerGetIterResponseResultAttributesList is a wrapper
type VserverPeerGetIterResponseResultAttributesList struct {
	XMLName            xml.Name              `xml:"attributes-list"`
	VserverPeerInfoPtr []VserverPeerInfoType `xml:"vserver-peer-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverPeerGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// VserverPeerInfo is a 'getter' method
func (o *VserverPeerGetIterResponseResultAttributesList) VserverPeerInfo() []VserverPeerInfoType {
	r := o.VserverPeerInfoPtr
	return r
}

// SetVserverPeerInfo is a fluent style 'setter' method that can be chained
func (o *VserverPeerGetIterResponseResultAttributesList) SetVserverPeerInfo(newValue []VserverPeerInfoType) *VserverPeerGetIterResponseResultAttributesList {
	newSlice := make([]VserverPeerInfoType, len(newValue))
	copy(newSlice, newValue)
	o.VserverPeerInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *VserverPeerGetIterResponseResultAttributesList) values() []VserverPeerInfoType {
	r := o.VserverPeerInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *VserverPeerGetIterResponseResultAttributesList) setValues(newValue []VserverPeerInfoType) *VserverPeerGetIterResponseResultAttributesList {
	newSlice := make([]VserverPeerInfoType, len(newValue))
	copy(newSlice, newValue)
	o.VserverPeerInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *VserverPeerGetIterResponseResult) AttributesList() VserverPeerGetIterResponseResultAttributesList {
	r := *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *VserverPeerGetIterResponseResult) SetAttributesList(newValue VserverPeerGetIterResponseResultAttributesList) *VserverPeerGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *VserverPeerGetIterResponseResult) NextTag() string {
	r := *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *VserverPeerGetIterResponseResult) SetNextTag(newValue string) *VserverPeerGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *VserverPeerGetIterResponseResult) NumRecords() int {
	r := *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *VserverPeerGetIterResponseResult) SetNumRecords(newValue int) *VserverPeerGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
