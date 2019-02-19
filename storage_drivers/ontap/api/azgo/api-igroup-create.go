package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// IgroupCreateRequest is a structure to represent a igroup-create Request ZAPI object
type IgroupCreateRequest struct {
	XMLName               xml.Name                  `xml:"igroup-create"`
	BindPortsetPtr        *string                   `xml:"bind-portset"`
	InitiatorGroupNamePtr *string                   `xml:"initiator-group-name"`
	InitiatorGroupTypePtr *string                   `xml:"initiator-group-type"`
	OsTypePtr             *InitiatorGroupOsTypeType `xml:"os-type"`
	OstypePtr             *InitiatorGroupOsTypeType `xml:"ostype"`
}

// IgroupCreateResponse is a structure to represent a igroup-create Response ZAPI object
type IgroupCreateResponse struct {
	XMLName         xml.Name                   `xml:"netapp"`
	ResponseVersion string                     `xml:"version,attr"`
	ResponseXmlns   string                     `xml:"xmlns,attr"`
	Result          IgroupCreateResponseResult `xml:"results"`
}

// NewIgroupCreateResponse is a factory method for creating new instances of IgroupCreateResponse objects
func NewIgroupCreateResponse() *IgroupCreateResponse {
	return &IgroupCreateResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupCreateResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *IgroupCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// IgroupCreateResponseResult is a structure to represent a igroup-create Response Result ZAPI object
type IgroupCreateResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewIgroupCreateRequest is a factory method for creating new instances of IgroupCreateRequest objects
func NewIgroupCreateRequest() *IgroupCreateRequest {
	return &IgroupCreateRequest{}
}

// NewIgroupCreateResponseResult is a factory method for creating new instances of IgroupCreateResponseResult objects
func NewIgroupCreateResponseResult() *IgroupCreateResponseResult {
	return &IgroupCreateResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *IgroupCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *IgroupCreateResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupCreateRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupCreateResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IgroupCreateRequest) ExecuteUsing(zr *ZapiRunner) (*IgroupCreateResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IgroupCreateRequest) executeWithoutIteration(zr *ZapiRunner) (*IgroupCreateResponse, error) {
	result, err := zr.ExecuteUsing(o, "IgroupCreateRequest", NewIgroupCreateResponse())
	if result == nil {
		return nil, err
	}
	return result.(*IgroupCreateResponse), err
}

// BindPortset is a 'getter' method
func (o *IgroupCreateRequest) BindPortset() string {
	r := *o.BindPortsetPtr
	return r
}

// SetBindPortset is a fluent style 'setter' method that can be chained
func (o *IgroupCreateRequest) SetBindPortset(newValue string) *IgroupCreateRequest {
	o.BindPortsetPtr = &newValue
	return o
}

// InitiatorGroupName is a 'getter' method
func (o *IgroupCreateRequest) InitiatorGroupName() string {
	r := *o.InitiatorGroupNamePtr
	return r
}

// SetInitiatorGroupName is a fluent style 'setter' method that can be chained
func (o *IgroupCreateRequest) SetInitiatorGroupName(newValue string) *IgroupCreateRequest {
	o.InitiatorGroupNamePtr = &newValue
	return o
}

// InitiatorGroupType is a 'getter' method
func (o *IgroupCreateRequest) InitiatorGroupType() string {
	r := *o.InitiatorGroupTypePtr
	return r
}

// SetInitiatorGroupType is a fluent style 'setter' method that can be chained
func (o *IgroupCreateRequest) SetInitiatorGroupType(newValue string) *IgroupCreateRequest {
	o.InitiatorGroupTypePtr = &newValue
	return o
}

// OsType is a 'getter' method
func (o *IgroupCreateRequest) OsType() InitiatorGroupOsTypeType {
	r := *o.OsTypePtr
	return r
}

// SetOsType is a fluent style 'setter' method that can be chained
func (o *IgroupCreateRequest) SetOsType(newValue InitiatorGroupOsTypeType) *IgroupCreateRequest {
	o.OsTypePtr = &newValue
	return o
}

// Ostype is a 'getter' method
func (o *IgroupCreateRequest) Ostype() InitiatorGroupOsTypeType {
	r := *o.OstypePtr
	return r
}

// SetOstype is a fluent style 'setter' method that can be chained
func (o *IgroupCreateRequest) SetOstype(newValue InitiatorGroupOsTypeType) *IgroupCreateRequest {
	o.OstypePtr = &newValue
	return o
}
