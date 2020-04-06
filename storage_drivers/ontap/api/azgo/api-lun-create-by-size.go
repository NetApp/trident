package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// LunCreateBySizeRequest is a structure to represent a lun-create-by-size Request ZAPI object
type LunCreateBySizeRequest struct {
	XMLName                    xml.Name       `xml:"lun-create-by-size"`
	ApplicationPtr             *string        `xml:"application"`
	CachingPolicyPtr           *string        `xml:"caching-policy"`
	ClassPtr                   *string        `xml:"class"`
	CommentPtr                 *string        `xml:"comment"`
	ForeignDiskPtr             *string        `xml:"foreign-disk"`
	OstypePtr                  *LunOsTypeType `xml:"ostype"`
	PathPtr                    *string        `xml:"path"`
	PrefixSizePtr              *int           `xml:"prefix-size"`
	QosPolicyGroupPtr          *string        `xml:"qos-policy-group"`
	QosAdaptivePolicyGroupPtr  *string        `xml:"qos-adaptive-policy-group"`
	SizePtr                    *int           `xml:"size"`
	SpaceAllocationEnabledPtr  *bool          `xml:"space-allocation-enabled"`
	SpaceReservationEnabledPtr *bool          `xml:"space-reservation-enabled"`
	TypePtr                    *LunOsTypeType `xml:"type"`
	UseExactSizePtr            *bool          `xml:"use-exact-size"`
}

// LunCreateBySizeResponse is a structure to represent a lun-create-by-size Response ZAPI object
type LunCreateBySizeResponse struct {
	XMLName         xml.Name                      `xml:"netapp"`
	ResponseVersion string                        `xml:"version,attr"`
	ResponseXmlns   string                        `xml:"xmlns,attr"`
	Result          LunCreateBySizeResponseResult `xml:"results"`
}

// NewLunCreateBySizeResponse is a factory method for creating new instances of LunCreateBySizeResponse objects
func NewLunCreateBySizeResponse() *LunCreateBySizeResponse {
	return &LunCreateBySizeResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunCreateBySizeResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *LunCreateBySizeResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// LunCreateBySizeResponseResult is a structure to represent a lun-create-by-size Response Result ZAPI object
type LunCreateBySizeResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
	ActualSizePtr    *int     `xml:"actual-size"`
}

// NewLunCreateBySizeRequest is a factory method for creating new instances of LunCreateBySizeRequest objects
func NewLunCreateBySizeRequest() *LunCreateBySizeRequest {
	return &LunCreateBySizeRequest{}
}

// NewLunCreateBySizeResponseResult is a factory method for creating new instances of LunCreateBySizeResponseResult objects
func NewLunCreateBySizeResponseResult() *LunCreateBySizeResponseResult {
	return &LunCreateBySizeResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *LunCreateBySizeRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *LunCreateBySizeResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunCreateBySizeRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunCreateBySizeResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunCreateBySizeRequest) ExecuteUsing(zr *ZapiRunner) (*LunCreateBySizeResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunCreateBySizeRequest) executeWithoutIteration(zr *ZapiRunner) (*LunCreateBySizeResponse, error) {
	result, err := zr.ExecuteUsing(o, "LunCreateBySizeRequest", NewLunCreateBySizeResponse())
	if result == nil {
		return nil, err
	}
	return result.(*LunCreateBySizeResponse), err
}

// Application is a 'getter' method
func (o *LunCreateBySizeRequest) Application() string {
	r := *o.ApplicationPtr
	return r
}

// SetApplication is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetApplication(newValue string) *LunCreateBySizeRequest {
	o.ApplicationPtr = &newValue
	return o
}

// CachingPolicy is a 'getter' method
func (o *LunCreateBySizeRequest) CachingPolicy() string {
	r := *o.CachingPolicyPtr
	return r
}

// SetCachingPolicy is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetCachingPolicy(newValue string) *LunCreateBySizeRequest {
	o.CachingPolicyPtr = &newValue
	return o
}

// Class is a 'getter' method
func (o *LunCreateBySizeRequest) Class() string {
	r := *o.ClassPtr
	return r
}

// SetClass is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetClass(newValue string) *LunCreateBySizeRequest {
	o.ClassPtr = &newValue
	return o
}

// Comment is a 'getter' method
func (o *LunCreateBySizeRequest) Comment() string {
	r := *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetComment(newValue string) *LunCreateBySizeRequest {
	o.CommentPtr = &newValue
	return o
}

// ForeignDisk is a 'getter' method
func (o *LunCreateBySizeRequest) ForeignDisk() string {
	r := *o.ForeignDiskPtr
	return r
}

// SetForeignDisk is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetForeignDisk(newValue string) *LunCreateBySizeRequest {
	o.ForeignDiskPtr = &newValue
	return o
}

// Ostype is a 'getter' method
func (o *LunCreateBySizeRequest) Ostype() LunOsTypeType {
	r := *o.OstypePtr
	return r
}

// SetOstype is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetOstype(newValue LunOsTypeType) *LunCreateBySizeRequest {
	o.OstypePtr = &newValue
	return o
}

// Path is a 'getter' method
func (o *LunCreateBySizeRequest) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetPath(newValue string) *LunCreateBySizeRequest {
	o.PathPtr = &newValue
	return o
}

// PrefixSize is a 'getter' method
func (o *LunCreateBySizeRequest) PrefixSize() int {
	r := *o.PrefixSizePtr
	return r
}

// SetPrefixSize is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetPrefixSize(newValue int) *LunCreateBySizeRequest {
	o.PrefixSizePtr = &newValue
	return o
}

// QosPolicyGroup is a 'getter' method
func (o *LunCreateBySizeRequest) QosPolicyGroup() string {
	r := *o.QosPolicyGroupPtr
	return r
}

// SetQosPolicyGroup is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetQosPolicyGroup(newValue string) *LunCreateBySizeRequest {
	o.QosPolicyGroupPtr = &newValue
	return o
}

// QosAdaptivePolicyGroup is a 'getter' method
func (o *LunCreateBySizeRequest) QosAdaptivePolicyGroup() string {
	r := *o.QosAdaptivePolicyGroupPtr
	return r
}

// SetQosAdaptivePolicyGroup is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetQosAdaptivePolicyGroup(newValue string) *LunCreateBySizeRequest {
	o.QosAdaptivePolicyGroupPtr = &newValue
	return o
}

// Size is a 'getter' method
func (o *LunCreateBySizeRequest) Size() int {
	r := *o.SizePtr
	return r
}

// SetSize is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetSize(newValue int) *LunCreateBySizeRequest {
	o.SizePtr = &newValue
	return o
}

// SpaceAllocationEnabled is a 'getter' method
func (o *LunCreateBySizeRequest) SpaceAllocationEnabled() bool {
	r := *o.SpaceAllocationEnabledPtr
	return r
}

// SetSpaceAllocationEnabled is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetSpaceAllocationEnabled(newValue bool) *LunCreateBySizeRequest {
	o.SpaceAllocationEnabledPtr = &newValue
	return o
}

// SpaceReservationEnabled is a 'getter' method
func (o *LunCreateBySizeRequest) SpaceReservationEnabled() bool {
	r := *o.SpaceReservationEnabledPtr
	return r
}

// SetSpaceReservationEnabled is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetSpaceReservationEnabled(newValue bool) *LunCreateBySizeRequest {
	o.SpaceReservationEnabledPtr = &newValue
	return o
}

// Type is a 'getter' method
func (o *LunCreateBySizeRequest) Type() LunOsTypeType {
	r := *o.TypePtr
	return r
}

// SetType is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetType(newValue LunOsTypeType) *LunCreateBySizeRequest {
	o.TypePtr = &newValue
	return o
}

// UseExactSize is a 'getter' method
func (o *LunCreateBySizeRequest) UseExactSize() bool {
	r := *o.UseExactSizePtr
	return r
}

// SetUseExactSize is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeRequest) SetUseExactSize(newValue bool) *LunCreateBySizeRequest {
	o.UseExactSizePtr = &newValue
	return o
}

// ActualSize is a 'getter' method
func (o *LunCreateBySizeResponseResult) ActualSize() int {
	r := *o.ActualSizePtr
	return r
}

// SetActualSize is a fluent style 'setter' method that can be chained
func (o *LunCreateBySizeResponseResult) SetActualSize(newValue int) *LunCreateBySizeResponseResult {
	o.ActualSizePtr = &newValue
	return o
}
