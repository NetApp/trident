// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// IscsiInitiatorModifyChapParamsRequest is a structure to represent a iscsi-initiator-modify-chap-params Request ZAPI object
type IscsiInitiatorModifyChapParamsRequest struct {
	XMLName               xml.Name `xml:"iscsi-initiator-modify-chap-params"`
	InitiatorPtr          *string  `xml:"initiator"`
	OutboundPassphrasePtr *string  `xml:"outbound-passphrase"`
	OutboundPasswordPtr   *string  `xml:"outbound-password"`
	OutboundUserNamePtr   *string  `xml:"outbound-user-name"`
	PassphrasePtr         *string  `xml:"passphrase"`
	PasswordPtr           *string  `xml:"password"`
	RadiusPtr             *bool    `xml:"radius"`
	RemoveOutboundPtr     *bool    `xml:"remove-outbound"`
	UserNamePtr           *string  `xml:"user-name"`
}

// IscsiInitiatorModifyChapParamsResponse is a structure to represent a iscsi-initiator-modify-chap-params Response ZAPI object
type IscsiInitiatorModifyChapParamsResponse struct {
	XMLName         xml.Name                                     `xml:"netapp"`
	ResponseVersion string                                       `xml:"version,attr"`
	ResponseXmlns   string                                       `xml:"xmlns,attr"`
	Result          IscsiInitiatorModifyChapParamsResponseResult `xml:"results"`
}

// NewIscsiInitiatorModifyChapParamsResponse is a factory method for creating new instances of IscsiInitiatorModifyChapParamsResponse objects
func NewIscsiInitiatorModifyChapParamsResponse() *IscsiInitiatorModifyChapParamsResponse {
	return &IscsiInitiatorModifyChapParamsResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorModifyChapParamsResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorModifyChapParamsResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// IscsiInitiatorModifyChapParamsResponseResult is a structure to represent a iscsi-initiator-modify-chap-params Response Result ZAPI object
type IscsiInitiatorModifyChapParamsResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewIscsiInitiatorModifyChapParamsRequest is a factory method for creating new instances of IscsiInitiatorModifyChapParamsRequest objects
func NewIscsiInitiatorModifyChapParamsRequest() *IscsiInitiatorModifyChapParamsRequest {
	return &IscsiInitiatorModifyChapParamsRequest{}
}

// NewIscsiInitiatorModifyChapParamsResponseResult is a factory method for creating new instances of IscsiInitiatorModifyChapParamsResponseResult objects
func NewIscsiInitiatorModifyChapParamsResponseResult() *IscsiInitiatorModifyChapParamsResponseResult {
	return &IscsiInitiatorModifyChapParamsResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorModifyChapParamsRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorModifyChapParamsResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorModifyChapParamsRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorModifyChapParamsResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiInitiatorModifyChapParamsRequest) ExecuteUsing(zr *ZapiRunner) (*IscsiInitiatorModifyChapParamsResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiInitiatorModifyChapParamsRequest) executeWithoutIteration(zr *ZapiRunner) (*IscsiInitiatorModifyChapParamsResponse, error) {
	result, err := zr.ExecuteUsing(o, "IscsiInitiatorModifyChapParamsRequest", NewIscsiInitiatorModifyChapParamsResponse())
	if result == nil {
		return nil, err
	}
	return result.(*IscsiInitiatorModifyChapParamsResponse), err
}

// Initiator is a 'getter' method
func (o *IscsiInitiatorModifyChapParamsRequest) Initiator() string {
	r := *o.InitiatorPtr
	return r
}

// SetInitiator is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorModifyChapParamsRequest) SetInitiator(newValue string) *IscsiInitiatorModifyChapParamsRequest {
	o.InitiatorPtr = &newValue
	return o
}

// OutboundPassphrase is a 'getter' method
func (o *IscsiInitiatorModifyChapParamsRequest) OutboundPassphrase() string {
	r := *o.OutboundPassphrasePtr
	return r
}

// SetOutboundPassphrase is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorModifyChapParamsRequest) SetOutboundPassphrase(newValue string) *IscsiInitiatorModifyChapParamsRequest {
	o.OutboundPassphrasePtr = &newValue
	return o
}

// OutboundPassword is a 'getter' method
func (o *IscsiInitiatorModifyChapParamsRequest) OutboundPassword() string {
	r := *o.OutboundPasswordPtr
	return r
}

// SetOutboundPassword is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorModifyChapParamsRequest) SetOutboundPassword(newValue string) *IscsiInitiatorModifyChapParamsRequest {
	o.OutboundPasswordPtr = &newValue
	return o
}

// OutboundUserName is a 'getter' method
func (o *IscsiInitiatorModifyChapParamsRequest) OutboundUserName() string {
	r := *o.OutboundUserNamePtr
	return r
}

// SetOutboundUserName is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorModifyChapParamsRequest) SetOutboundUserName(newValue string) *IscsiInitiatorModifyChapParamsRequest {
	o.OutboundUserNamePtr = &newValue
	return o
}

// Passphrase is a 'getter' method
func (o *IscsiInitiatorModifyChapParamsRequest) Passphrase() string {
	r := *o.PassphrasePtr
	return r
}

// SetPassphrase is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorModifyChapParamsRequest) SetPassphrase(newValue string) *IscsiInitiatorModifyChapParamsRequest {
	o.PassphrasePtr = &newValue
	return o
}

// Password is a 'getter' method
func (o *IscsiInitiatorModifyChapParamsRequest) Password() string {
	r := *o.PasswordPtr
	return r
}

// SetPassword is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorModifyChapParamsRequest) SetPassword(newValue string) *IscsiInitiatorModifyChapParamsRequest {
	o.PasswordPtr = &newValue
	return o
}

// Radius is a 'getter' method
func (o *IscsiInitiatorModifyChapParamsRequest) Radius() bool {
	r := *o.RadiusPtr
	return r
}

// SetRadius is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorModifyChapParamsRequest) SetRadius(newValue bool) *IscsiInitiatorModifyChapParamsRequest {
	o.RadiusPtr = &newValue
	return o
}

// RemoveOutbound is a 'getter' method
func (o *IscsiInitiatorModifyChapParamsRequest) RemoveOutbound() bool {
	r := *o.RemoveOutboundPtr
	return r
}

// SetRemoveOutbound is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorModifyChapParamsRequest) SetRemoveOutbound(newValue bool) *IscsiInitiatorModifyChapParamsRequest {
	o.RemoveOutboundPtr = &newValue
	return o
}

// UserName is a 'getter' method
func (o *IscsiInitiatorModifyChapParamsRequest) UserName() string {
	r := *o.UserNamePtr
	return r
}

// SetUserName is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorModifyChapParamsRequest) SetUserName(newValue string) *IscsiInitiatorModifyChapParamsRequest {
	o.UserNamePtr = &newValue
	return o
}
