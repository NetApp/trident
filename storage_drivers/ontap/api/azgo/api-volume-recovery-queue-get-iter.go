package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeRecoveryQueueGetIterRequest is a structure to represent a volume-recovery-queue-get-iter Request ZAPI object
type VolumeRecoveryQueueGetIterRequest struct {
	XMLName              xml.Name                                            `xml:"volume-recovery-queue-get-iter"`
	DesiredAttributesPtr *VolumeRecoveryQueueGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                                `xml:"max-records"`
	QueryPtr             *VolumeRecoveryQueueGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                             `xml:"tag"`
}

// VolumeRecoveryQueueGetIterResponse is a structure to represent a volume-recovery-queue-get-iter Response ZAPI object
type VolumeRecoveryQueueGetIterResponse struct {
	XMLName         xml.Name                                 `xml:"netapp"`
	ResponseVersion string                                   `xml:"version,attr"`
	ResponseXmlns   string                                   `xml:"xmlns,attr"`
	Result          VolumeRecoveryQueueGetIterResponseResult `xml:"results"`
}

// NewVolumeRecoveryQueueGetIterResponse is a factory method for creating new instances of VolumeRecoveryQueueGetIterResponse objects
func NewVolumeRecoveryQueueGetIterResponse() *VolumeRecoveryQueueGetIterResponse {
	return &VolumeRecoveryQueueGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeRecoveryQueueGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeRecoveryQueueGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeRecoveryQueueGetIterResponseResult is a structure to represent a volume-recovery-queue-get-iter Response Result ZAPI object
type VolumeRecoveryQueueGetIterResponseResult struct {
	XMLName           xml.Name                                                `xml:"results"`
	ResultStatusAttr  string                                                  `xml:"status,attr"`
	ResultReasonAttr  string                                                  `xml:"reason,attr"`
	ResultErrnoAttr   string                                                  `xml:"errno,attr"`
	AttributesListPtr *VolumeRecoveryQueueGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                                 `xml:"next-tag"`
	NumRecordsPtr     *int                                                    `xml:"num-records"`
}

// NewVolumeRecoveryQueueGetIterRequest is a factory method for creating new instances of VolumeRecoveryQueueGetIterRequest objects
func NewVolumeRecoveryQueueGetIterRequest() *VolumeRecoveryQueueGetIterRequest {
	return &VolumeRecoveryQueueGetIterRequest{}
}

// NewVolumeRecoveryQueueGetIterResponseResult is a factory method for creating new instances of VolumeRecoveryQueueGetIterResponseResult objects
func NewVolumeRecoveryQueueGetIterResponseResult() *VolumeRecoveryQueueGetIterResponseResult {
	return &VolumeRecoveryQueueGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeRecoveryQueueGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeRecoveryQueueGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeRecoveryQueueGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeRecoveryQueueGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeRecoveryQueueGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeRecoveryQueueGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeRecoveryQueueGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeRecoveryQueueGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeRecoveryQueueGetIterRequest", NewVolumeRecoveryQueueGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*VolumeRecoveryQueueGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeRecoveryQueueGetIterRequest) executeWithIteration(zr *ZapiRunner) (*VolumeRecoveryQueueGetIterResponse, error) {
	combined := NewVolumeRecoveryQueueGetIterResponse()
	combined.Result.SetAttributesList(VolumeRecoveryQueueGetIterResponseResultAttributesList{})
	var nextTagPtr *string
	done := false
	for !done {
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
				combined.Result.SetAttributesList(VolumeRecoveryQueueGetIterResponseResultAttributesList{})
			}
			combinedAttributesList := combined.Result.AttributesList()
			combinedAttributes := combinedAttributesList.values()

			resultAttributesList := n.Result.AttributesList()
			resultAttributes := resultAttributesList.values()

			combined.Result.AttributesListPtr.setValues(append(combinedAttributes, resultAttributes...))
		}

		if done {

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

// VolumeRecoveryQueueGetIterRequestDesiredAttributes is a wrapper
type VolumeRecoveryQueueGetIterRequestDesiredAttributes struct {
	XMLName                    xml.Name                     `xml:"desired-attributes"`
	VolumeRecoveryQueueInfoPtr *VolumeRecoveryQueueInfoType `xml:"volume-recovery-queue-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeRecoveryQueueGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeRecoveryQueueInfo is a 'getter' method
func (o *VolumeRecoveryQueueGetIterRequestDesiredAttributes) VolumeRecoveryQueueInfo() VolumeRecoveryQueueInfoType {
	var r VolumeRecoveryQueueInfoType
	if o.VolumeRecoveryQueueInfoPtr == nil {
		return r
	}
	r = *o.VolumeRecoveryQueueInfoPtr

	return r
}

// SetVolumeRecoveryQueueInfo is a fluent style 'setter' method that can be chained
func (o *VolumeRecoveryQueueGetIterRequestDesiredAttributes) SetVolumeRecoveryQueueInfo(newValue VolumeRecoveryQueueInfoType) *VolumeRecoveryQueueGetIterRequestDesiredAttributes {
	o.VolumeRecoveryQueueInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *VolumeRecoveryQueueGetIterRequest) DesiredAttributes() VolumeRecoveryQueueGetIterRequestDesiredAttributes {
	var r VolumeRecoveryQueueGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr

	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeRecoveryQueueGetIterRequest) SetDesiredAttributes(newValue VolumeRecoveryQueueGetIterRequestDesiredAttributes) *VolumeRecoveryQueueGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *VolumeRecoveryQueueGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr

	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *VolumeRecoveryQueueGetIterRequest) SetMaxRecords(newValue int) *VolumeRecoveryQueueGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// VolumeRecoveryQueueGetIterRequestQuery is a wrapper
type VolumeRecoveryQueueGetIterRequestQuery struct {
	XMLName                    xml.Name                     `xml:"query"`
	VolumeRecoveryQueueInfoPtr *VolumeRecoveryQueueInfoType `xml:"volume-recovery-queue-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeRecoveryQueueGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeRecoveryQueueInfo is a 'getter' method
func (o *VolumeRecoveryQueueGetIterRequestQuery) VolumeRecoveryQueueInfo() VolumeRecoveryQueueInfoType {
	var r VolumeRecoveryQueueInfoType
	if o.VolumeRecoveryQueueInfoPtr == nil {
		return r
	}
	r = *o.VolumeRecoveryQueueInfoPtr

	return r
}

// SetVolumeRecoveryQueueInfo is a fluent style 'setter' method that can be chained
func (o *VolumeRecoveryQueueGetIterRequestQuery) SetVolumeRecoveryQueueInfo(newValue VolumeRecoveryQueueInfoType) *VolumeRecoveryQueueGetIterRequestQuery {
	o.VolumeRecoveryQueueInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *VolumeRecoveryQueueGetIterRequest) Query() VolumeRecoveryQueueGetIterRequestQuery {
	var r VolumeRecoveryQueueGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr

	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *VolumeRecoveryQueueGetIterRequest) SetQuery(newValue VolumeRecoveryQueueGetIterRequestQuery) *VolumeRecoveryQueueGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *VolumeRecoveryQueueGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr

	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *VolumeRecoveryQueueGetIterRequest) SetTag(newValue string) *VolumeRecoveryQueueGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// VolumeRecoveryQueueGetIterResponseResultAttributesList is a wrapper
type VolumeRecoveryQueueGetIterResponseResultAttributesList struct {
	XMLName                    xml.Name                      `xml:"attributes-list"`
	VolumeRecoveryQueueInfoPtr []VolumeRecoveryQueueInfoType `xml:"volume-recovery-queue-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeRecoveryQueueGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeRecoveryQueueInfo is a 'getter' method
func (o *VolumeRecoveryQueueGetIterResponseResultAttributesList) VolumeRecoveryQueueInfo() []VolumeRecoveryQueueInfoType {
	r := o.VolumeRecoveryQueueInfoPtr
	return r
}

// SetVolumeRecoveryQueueInfo is a fluent style 'setter' method that can be chained
func (o *VolumeRecoveryQueueGetIterResponseResultAttributesList) SetVolumeRecoveryQueueInfo(newValue []VolumeRecoveryQueueInfoType) *VolumeRecoveryQueueGetIterResponseResultAttributesList {
	newSlice := make([]VolumeRecoveryQueueInfoType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeRecoveryQueueInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *VolumeRecoveryQueueGetIterResponseResultAttributesList) values() []VolumeRecoveryQueueInfoType {
	r := o.VolumeRecoveryQueueInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *VolumeRecoveryQueueGetIterResponseResultAttributesList) setValues(newValue []VolumeRecoveryQueueInfoType) *VolumeRecoveryQueueGetIterResponseResultAttributesList {
	newSlice := make([]VolumeRecoveryQueueInfoType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeRecoveryQueueInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *VolumeRecoveryQueueGetIterResponseResult) AttributesList() VolumeRecoveryQueueGetIterResponseResultAttributesList {
	var r VolumeRecoveryQueueGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr

	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *VolumeRecoveryQueueGetIterResponseResult) SetAttributesList(newValue VolumeRecoveryQueueGetIterResponseResultAttributesList) *VolumeRecoveryQueueGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *VolumeRecoveryQueueGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr

	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *VolumeRecoveryQueueGetIterResponseResult) SetNextTag(newValue string) *VolumeRecoveryQueueGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *VolumeRecoveryQueueGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr

	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *VolumeRecoveryQueueGetIterResponseResult) SetNumRecords(newValue int) *VolumeRecoveryQueueGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
