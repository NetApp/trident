package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// LunSetQosPolicyGroupRequest is a structure to represent a lun-set-qos-policy-group Request ZAPI object
type LunSetQosPolicyGroupRequest struct {
	XMLName                   xml.Name     `xml:"lun-set-qos-policy-group"`
	PathPtr                   *LunPathType `xml:"path"`
	QosAdaptivePolicyGroupPtr *string      `xml:"qos-adaptive-policy-group"`
	QosPolicyGroupPtr         *string      `xml:"qos-policy-group"`
}

// LunSetQosPolicyGroupResponse is a structure to represent a lun-set-qos-policy-group Response ZAPI object
type LunSetQosPolicyGroupResponse struct {
	XMLName         xml.Name                           `xml:"netapp"`
	ResponseVersion string                             `xml:"version,attr"`
	ResponseXmlns   string                             `xml:"xmlns,attr"`
	Result          LunSetQosPolicyGroupResponseResult `xml:"results"`
}

// NewLunSetQosPolicyGroupResponse is a factory method for creating new instances of LunSetQosPolicyGroupResponse objects
func NewLunSetQosPolicyGroupResponse() *LunSetQosPolicyGroupResponse {
	return &LunSetQosPolicyGroupResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunSetQosPolicyGroupResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *LunSetQosPolicyGroupResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// LunSetQosPolicyGroupResponseResult is a structure to represent a lun-set-qos-policy-group Response Result ZAPI object
type LunSetQosPolicyGroupResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewLunSetQosPolicyGroupRequest is a factory method for creating new instances of LunSetQosPolicyGroupRequest objects
func NewLunSetQosPolicyGroupRequest() *LunSetQosPolicyGroupRequest {
	return &LunSetQosPolicyGroupRequest{}
}

// NewLunSetQosPolicyGroupResponseResult is a factory method for creating new instances of LunSetQosPolicyGroupResponseResult objects
func NewLunSetQosPolicyGroupResponseResult() *LunSetQosPolicyGroupResponseResult {
	return &LunSetQosPolicyGroupResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *LunSetQosPolicyGroupRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *LunSetQosPolicyGroupResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunSetQosPolicyGroupRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunSetQosPolicyGroupResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunSetQosPolicyGroupRequest) ExecuteUsing(zr *ZapiRunner) (*LunSetQosPolicyGroupResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunSetQosPolicyGroupRequest) executeWithoutIteration(zr *ZapiRunner) (*LunSetQosPolicyGroupResponse, error) {
	result, err := zr.ExecuteUsing(o, "LunSetQosPolicyGroupRequest", NewLunSetQosPolicyGroupResponse())
	if result == nil {
		return nil, err
	}
	return result.(*LunSetQosPolicyGroupResponse), err
}

// Path is a 'getter' method
func (o *LunSetQosPolicyGroupRequest) Path() LunPathType {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunSetQosPolicyGroupRequest) SetPath(newValue LunPathType) *LunSetQosPolicyGroupRequest {
	o.PathPtr = &newValue
	return o
}

// QosAdaptivePolicyGroup is a 'getter' method
func (o *LunSetQosPolicyGroupRequest) QosAdaptivePolicyGroup() string {
	r := *o.QosAdaptivePolicyGroupPtr
	return r
}

// SetQosAdaptivePolicyGroup is a fluent style 'setter' method that can be chained
func (o *LunSetQosPolicyGroupRequest) SetQosAdaptivePolicyGroup(newValue string) *LunSetQosPolicyGroupRequest {
	o.QosAdaptivePolicyGroupPtr = &newValue
	return o
}

// QosPolicyGroup is a 'getter' method
func (o *LunSetQosPolicyGroupRequest) QosPolicyGroup() string {
	r := *o.QosPolicyGroupPtr
	return r
}

// SetQosPolicyGroup is a fluent style 'setter' method that can be chained
func (o *LunSetQosPolicyGroupRequest) SetQosPolicyGroup(newValue string) *LunSetQosPolicyGroupRequest {
	o.QosPolicyGroupPtr = &newValue
	return o
}
