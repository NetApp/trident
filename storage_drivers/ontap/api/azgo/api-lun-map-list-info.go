// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// LunMapListInfoRequest is a structure to represent a lun-map-list-info Request ZAPI object
type LunMapListInfoRequest struct {
	XMLName xml.Name `xml:"lun-map-list-info"`
	PathPtr *string  `xml:"path"`
}

// LunMapListInfoResponse is a structure to represent a lun-map-list-info Response ZAPI object
type LunMapListInfoResponse struct {
	XMLName         xml.Name                     `xml:"netapp"`
	ResponseVersion string                       `xml:"version,attr"`
	ResponseXmlns   string                       `xml:"xmlns,attr"`
	Result          LunMapListInfoResponseResult `xml:"results"`
}

// NewLunMapListInfoResponse is a factory method for creating new instances of LunMapListInfoResponse objects
func NewLunMapListInfoResponse() *LunMapListInfoResponse {
	return &LunMapListInfoResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapListInfoResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *LunMapListInfoResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// LunMapListInfoResponseResult is a structure to represent a lun-map-list-info Response Result ZAPI object
type LunMapListInfoResponseResult struct {
	XMLName            xml.Name                                     `xml:"results"`
	ResultStatusAttr   string                                       `xml:"status,attr"`
	ResultReasonAttr   string                                       `xml:"reason,attr"`
	ResultErrnoAttr    string                                       `xml:"errno,attr"`
	InitiatorGroupsPtr *LunMapListInfoResponseResultInitiatorGroups `xml:"initiator-groups"`
}

// NewLunMapListInfoRequest is a factory method for creating new instances of LunMapListInfoRequest objects
func NewLunMapListInfoRequest() *LunMapListInfoRequest {
	return &LunMapListInfoRequest{}
}

// NewLunMapListInfoResponseResult is a factory method for creating new instances of LunMapListInfoResponseResult objects
func NewLunMapListInfoResponseResult() *LunMapListInfoResponseResult {
	return &LunMapListInfoResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *LunMapListInfoRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *LunMapListInfoResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapListInfoRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapListInfoResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunMapListInfoRequest) ExecuteUsing(zr *ZapiRunner) (*LunMapListInfoResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunMapListInfoRequest) executeWithoutIteration(zr *ZapiRunner) (*LunMapListInfoResponse, error) {
	result, err := zr.ExecuteUsing(o, "LunMapListInfoRequest", NewLunMapListInfoResponse())
	if result == nil {
		return nil, err
	}
	return result.(*LunMapListInfoResponse), err
}

// Path is a 'getter' method
func (o *LunMapListInfoRequest) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunMapListInfoRequest) SetPath(newValue string) *LunMapListInfoRequest {
	o.PathPtr = &newValue
	return o
}

// LunMapListInfoResponseResultInitiatorGroups is a wrapper
type LunMapListInfoResponseResultInitiatorGroups struct {
	XMLName               xml.Name                 `xml:"initiator-groups"`
	InitiatorGroupInfoPtr []InitiatorGroupInfoType `xml:"initiator-group-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapListInfoResponseResultInitiatorGroups) String() string {
	return ToString(reflect.ValueOf(o))
}

// InitiatorGroupInfo is a 'getter' method
func (o *LunMapListInfoResponseResultInitiatorGroups) InitiatorGroupInfo() []InitiatorGroupInfoType {
	r := o.InitiatorGroupInfoPtr
	return r
}

// SetInitiatorGroupInfo is a fluent style 'setter' method that can be chained
func (o *LunMapListInfoResponseResultInitiatorGroups) SetInitiatorGroupInfo(newValue []InitiatorGroupInfoType) *LunMapListInfoResponseResultInitiatorGroups {
	newSlice := make([]InitiatorGroupInfoType, len(newValue))
	copy(newSlice, newValue)
	o.InitiatorGroupInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *LunMapListInfoResponseResultInitiatorGroups) values() []InitiatorGroupInfoType {
	r := o.InitiatorGroupInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *LunMapListInfoResponseResultInitiatorGroups) setValues(newValue []InitiatorGroupInfoType) *LunMapListInfoResponseResultInitiatorGroups {
	newSlice := make([]InitiatorGroupInfoType, len(newValue))
	copy(newSlice, newValue)
	o.InitiatorGroupInfoPtr = newSlice
	return o
}

// InitiatorGroups is a 'getter' method
func (o *LunMapListInfoResponseResult) InitiatorGroups() LunMapListInfoResponseResultInitiatorGroups {
	r := *o.InitiatorGroupsPtr
	return r
}

// SetInitiatorGroups is a fluent style 'setter' method that can be chained
func (o *LunMapListInfoResponseResult) SetInitiatorGroups(newValue LunMapListInfoResponseResultInitiatorGroups) *LunMapListInfoResponseResult {
	o.InitiatorGroupsPtr = &newValue
	return o
}
