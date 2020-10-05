package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// IscsiInitiatorGetAuthRequest is a structure to represent a iscsi-initiator-get-auth Request ZAPI object
type IscsiInitiatorGetAuthRequest struct {
	XMLName      xml.Name `xml:"iscsi-initiator-get-auth"`
	InitiatorPtr *string  `xml:"initiator"`
}

// IscsiInitiatorGetAuthResponse is a structure to represent a iscsi-initiator-get-auth Response ZAPI object
type IscsiInitiatorGetAuthResponse struct {
	XMLName         xml.Name                            `xml:"netapp"`
	ResponseVersion string                              `xml:"version,attr"`
	ResponseXmlns   string                              `xml:"xmlns,attr"`
	Result          IscsiInitiatorGetAuthResponseResult `xml:"results"`
}

// NewIscsiInitiatorGetAuthResponse is a factory method for creating new instances of IscsiInitiatorGetAuthResponse objects
func NewIscsiInitiatorGetAuthResponse() *IscsiInitiatorGetAuthResponse {
	return &IscsiInitiatorGetAuthResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorGetAuthResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorGetAuthResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// IscsiInitiatorGetAuthResponseResult is a structure to represent a iscsi-initiator-get-auth Response Result ZAPI object
type IscsiInitiatorGetAuthResponseResult struct {
	XMLName                   xml.Name                                                   `xml:"results"`
	ResultStatusAttr          string                                                     `xml:"status,attr"`
	ResultReasonAttr          string                                                     `xml:"reason,attr"`
	ResultErrnoAttr           string                                                     `xml:"errno,attr"`
	AuthChapPolicyPtr         *string                                                    `xml:"auth-chap-policy"`
	AuthTypePtr               *string                                                    `xml:"auth-type"`
	InitiatorAddressRangesPtr *IscsiInitiatorGetAuthResponseResultInitiatorAddressRanges `xml:"initiator-address-ranges"`
	OutboundUserNamePtr       *string                                                    `xml:"outbound-user-name"`
	UserNamePtr               *string                                                    `xml:"user-name"`
}

// NewIscsiInitiatorGetAuthRequest is a factory method for creating new instances of IscsiInitiatorGetAuthRequest objects
func NewIscsiInitiatorGetAuthRequest() *IscsiInitiatorGetAuthRequest {
	return &IscsiInitiatorGetAuthRequest{}
}

// NewIscsiInitiatorGetAuthResponseResult is a factory method for creating new instances of IscsiInitiatorGetAuthResponseResult objects
func NewIscsiInitiatorGetAuthResponseResult() *IscsiInitiatorGetAuthResponseResult {
	return &IscsiInitiatorGetAuthResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorGetAuthRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorGetAuthResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorGetAuthRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorGetAuthResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiInitiatorGetAuthRequest) ExecuteUsing(zr *ZapiRunner) (*IscsiInitiatorGetAuthResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiInitiatorGetAuthRequest) executeWithoutIteration(zr *ZapiRunner) (*IscsiInitiatorGetAuthResponse, error) {
	result, err := zr.ExecuteUsing(o, "IscsiInitiatorGetAuthRequest", NewIscsiInitiatorGetAuthResponse())
	if result == nil {
		return nil, err
	}
	return result.(*IscsiInitiatorGetAuthResponse), err
}

// Initiator is a 'getter' method
func (o *IscsiInitiatorGetAuthRequest) Initiator() string {
	r := *o.InitiatorPtr
	return r
}

// SetInitiator is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetAuthRequest) SetInitiator(newValue string) *IscsiInitiatorGetAuthRequest {
	o.InitiatorPtr = &newValue
	return o
}

// AuthChapPolicy is a 'getter' method
func (o *IscsiInitiatorGetAuthResponseResult) AuthChapPolicy() string {
	r := *o.AuthChapPolicyPtr
	return r
}

// SetAuthChapPolicy is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetAuthResponseResult) SetAuthChapPolicy(newValue string) *IscsiInitiatorGetAuthResponseResult {
	o.AuthChapPolicyPtr = &newValue
	return o
}

// AuthType is a 'getter' method
func (o *IscsiInitiatorGetAuthResponseResult) AuthType() string {
	r := *o.AuthTypePtr
	return r
}

// SetAuthType is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetAuthResponseResult) SetAuthType(newValue string) *IscsiInitiatorGetAuthResponseResult {
	o.AuthTypePtr = &newValue
	return o
}

// IscsiInitiatorGetAuthResponseResultInitiatorAddressRanges is a wrapper
type IscsiInitiatorGetAuthResponseResultInitiatorAddressRanges struct {
	XMLName          xml.Name            `xml:"initiator-address-ranges"`
	IpRangeOrMaskPtr []IpRangeOrMaskType `xml:"ip-range-or-mask"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorGetAuthResponseResultInitiatorAddressRanges) String() string {
	return ToString(reflect.ValueOf(o))
}

// IpRangeOrMask is a 'getter' method
func (o *IscsiInitiatorGetAuthResponseResultInitiatorAddressRanges) IpRangeOrMask() []IpRangeOrMaskType {
	r := o.IpRangeOrMaskPtr
	return r
}

// SetIpRangeOrMask is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetAuthResponseResultInitiatorAddressRanges) SetIpRangeOrMask(newValue []IpRangeOrMaskType) *IscsiInitiatorGetAuthResponseResultInitiatorAddressRanges {
	newSlice := make([]IpRangeOrMaskType, len(newValue))
	copy(newSlice, newValue)
	o.IpRangeOrMaskPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *IscsiInitiatorGetAuthResponseResultInitiatorAddressRanges) values() []IpRangeOrMaskType {
	r := o.IpRangeOrMaskPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetAuthResponseResultInitiatorAddressRanges) setValues(newValue []IpRangeOrMaskType) *IscsiInitiatorGetAuthResponseResultInitiatorAddressRanges {
	newSlice := make([]IpRangeOrMaskType, len(newValue))
	copy(newSlice, newValue)
	o.IpRangeOrMaskPtr = newSlice
	return o
}

// InitiatorAddressRanges is a 'getter' method
func (o *IscsiInitiatorGetAuthResponseResult) InitiatorAddressRanges() IscsiInitiatorGetAuthResponseResultInitiatorAddressRanges {
	r := *o.InitiatorAddressRangesPtr
	return r
}

// SetInitiatorAddressRanges is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetAuthResponseResult) SetInitiatorAddressRanges(newValue IscsiInitiatorGetAuthResponseResultInitiatorAddressRanges) *IscsiInitiatorGetAuthResponseResult {
	o.InitiatorAddressRangesPtr = &newValue
	return o
}

// OutboundUserName is a 'getter' method
func (o *IscsiInitiatorGetAuthResponseResult) OutboundUserName() string {
	r := *o.OutboundUserNamePtr
	return r
}

// SetOutboundUserName is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetAuthResponseResult) SetOutboundUserName(newValue string) *IscsiInitiatorGetAuthResponseResult {
	o.OutboundUserNamePtr = &newValue
	return o
}

// UserName is a 'getter' method
func (o *IscsiInitiatorGetAuthResponseResult) UserName() string {
	r := *o.UserNamePtr
	return r
}

// SetUserName is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetAuthResponseResult) SetUserName(newValue string) *IscsiInitiatorGetAuthResponseResult {
	o.UserNamePtr = &newValue
	return o
}
