package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// IscsiInitiatorSetDefaultAuthRequest is a structure to represent a iscsi-initiator-set-default-auth Request ZAPI object
type IscsiInitiatorSetDefaultAuthRequest struct {
	XMLName               xml.Name `xml:"iscsi-initiator-set-default-auth"`
	AuthTypePtr           *string  `xml:"auth-type"`
	OutboundPassphrasePtr *string  `xml:"outbound-passphrase"`
	OutboundPasswordPtr   *string  `xml:"outbound-password"`
	OutboundUserNamePtr   *string  `xml:"outbound-user-name"`
	PassphrasePtr         *string  `xml:"passphrase"`
	PasswordPtr           *string  `xml:"password"`
	RadiusPtr             *bool    `xml:"radius"`
	UserNamePtr           *string  `xml:"user-name"`
}

// IscsiInitiatorSetDefaultAuthResponse is a structure to represent a iscsi-initiator-set-default-auth Response ZAPI object
type IscsiInitiatorSetDefaultAuthResponse struct {
	XMLName         xml.Name                                   `xml:"netapp"`
	ResponseVersion string                                     `xml:"version,attr"`
	ResponseXmlns   string                                     `xml:"xmlns,attr"`
	Result          IscsiInitiatorSetDefaultAuthResponseResult `xml:"results"`
}

// NewIscsiInitiatorSetDefaultAuthResponse is a factory method for creating new instances of IscsiInitiatorSetDefaultAuthResponse objects
func NewIscsiInitiatorSetDefaultAuthResponse() *IscsiInitiatorSetDefaultAuthResponse {
	return &IscsiInitiatorSetDefaultAuthResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorSetDefaultAuthResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorSetDefaultAuthResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// IscsiInitiatorSetDefaultAuthResponseResult is a structure to represent a iscsi-initiator-set-default-auth Response Result ZAPI object
type IscsiInitiatorSetDefaultAuthResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewIscsiInitiatorSetDefaultAuthRequest is a factory method for creating new instances of IscsiInitiatorSetDefaultAuthRequest objects
func NewIscsiInitiatorSetDefaultAuthRequest() *IscsiInitiatorSetDefaultAuthRequest {
	return &IscsiInitiatorSetDefaultAuthRequest{}
}

// NewIscsiInitiatorSetDefaultAuthResponseResult is a factory method for creating new instances of IscsiInitiatorSetDefaultAuthResponseResult objects
func NewIscsiInitiatorSetDefaultAuthResponseResult() *IscsiInitiatorSetDefaultAuthResponseResult {
	return &IscsiInitiatorSetDefaultAuthResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorSetDefaultAuthRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorSetDefaultAuthResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorSetDefaultAuthRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorSetDefaultAuthResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiInitiatorSetDefaultAuthRequest) ExecuteUsing(zr *ZapiRunner) (*IscsiInitiatorSetDefaultAuthResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiInitiatorSetDefaultAuthRequest) executeWithoutIteration(zr *ZapiRunner) (*IscsiInitiatorSetDefaultAuthResponse, error) {
	result, err := zr.ExecuteUsing(o, "IscsiInitiatorSetDefaultAuthRequest", NewIscsiInitiatorSetDefaultAuthResponse())
	if result == nil {
		return nil, err
	}
	return result.(*IscsiInitiatorSetDefaultAuthResponse), err
}

// AuthType is a 'getter' method
func (o *IscsiInitiatorSetDefaultAuthRequest) AuthType() string {
	r := *o.AuthTypePtr
	return r
}

// SetAuthType is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorSetDefaultAuthRequest) SetAuthType(newValue string) *IscsiInitiatorSetDefaultAuthRequest {
	o.AuthTypePtr = &newValue
	return o
}

// OutboundPassphrase is a 'getter' method
func (o *IscsiInitiatorSetDefaultAuthRequest) OutboundPassphrase() string {
	r := *o.OutboundPassphrasePtr
	return r
}

// SetOutboundPassphrase is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorSetDefaultAuthRequest) SetOutboundPassphrase(newValue string) *IscsiInitiatorSetDefaultAuthRequest {
	o.OutboundPassphrasePtr = &newValue
	return o
}

// OutboundPassword is a 'getter' method
func (o *IscsiInitiatorSetDefaultAuthRequest) OutboundPassword() string {
	r := *o.OutboundPasswordPtr
	return r
}

// SetOutboundPassword is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorSetDefaultAuthRequest) SetOutboundPassword(newValue string) *IscsiInitiatorSetDefaultAuthRequest {
	o.OutboundPasswordPtr = &newValue
	return o
}

// OutboundUserName is a 'getter' method
func (o *IscsiInitiatorSetDefaultAuthRequest) OutboundUserName() string {
	r := *o.OutboundUserNamePtr
	return r
}

// SetOutboundUserName is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorSetDefaultAuthRequest) SetOutboundUserName(newValue string) *IscsiInitiatorSetDefaultAuthRequest {
	o.OutboundUserNamePtr = &newValue
	return o
}

// Passphrase is a 'getter' method
func (o *IscsiInitiatorSetDefaultAuthRequest) Passphrase() string {
	r := *o.PassphrasePtr
	return r
}

// SetPassphrase is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorSetDefaultAuthRequest) SetPassphrase(newValue string) *IscsiInitiatorSetDefaultAuthRequest {
	o.PassphrasePtr = &newValue
	return o
}

// Password is a 'getter' method
func (o *IscsiInitiatorSetDefaultAuthRequest) Password() string {
	r := *o.PasswordPtr
	return r
}

// SetPassword is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorSetDefaultAuthRequest) SetPassword(newValue string) *IscsiInitiatorSetDefaultAuthRequest {
	o.PasswordPtr = &newValue
	return o
}

// Radius is a 'getter' method
func (o *IscsiInitiatorSetDefaultAuthRequest) Radius() bool {
	r := *o.RadiusPtr
	return r
}

// SetRadius is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorSetDefaultAuthRequest) SetRadius(newValue bool) *IscsiInitiatorSetDefaultAuthRequest {
	o.RadiusPtr = &newValue
	return o
}

// UserName is a 'getter' method
func (o *IscsiInitiatorSetDefaultAuthRequest) UserName() string {
	r := *o.UserNamePtr
	return r
}

// SetUserName is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorSetDefaultAuthRequest) SetUserName(newValue string) *IscsiInitiatorSetDefaultAuthRequest {
	o.UserNamePtr = &newValue
	return o
}
