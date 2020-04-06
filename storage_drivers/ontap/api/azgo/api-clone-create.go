package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// CloneCreateRequest is a structure to represent a clone-create Request ZAPI object
type CloneCreateRequest struct {
	XMLName               xml.Name                       `xml:"clone-create"`
	AutodeletePtr         *bool                          `xml:"autodelete"`
	BlockRangesPtr        *CloneCreateRequestBlockRanges `xml:"block-ranges"`
	BypassLicenseCheckPtr *bool                          `xml:"bypass-license-check"`
	BypassThrottlePtr     *bool                          `xml:"bypass-throttle"`
	DestinationExistsPtr  *bool                          `xml:"destination-exists"`
	DestinationPathPtr    *string                        `xml:"destination-path"`
	DestinationVolumePtr  *string                        `xml:"destination-volume"`
	FixedBlockCountPtr    *int                           `xml:"fixed-block-count"`
	IgnoreLocksPtr        *bool                          `xml:"ignore-locks"`
	IgnoreStreamsPtr      *bool                          `xml:"ignore-streams"`
	IsBackupPtr           *bool                          `xml:"is-backup"`
	IsFixedBlockCountPtr  *bool                          `xml:"is-fixed-block-count"`
	IsVvolBackupPtr       *bool                          `xml:"is-vvol-backup"`
	LunSerialNumberPtr    *string                        `xml:"lun-serial-number"`
	NosplitEntryPtr       *bool                          `xml:"nosplit-entry"`
	QosPolicyGroupNamePtr *string                        `xml:"qos-policy-group-name"`
	QosAdaptivePolicyGroupNamePtr *string                `xml:"qos-adaptive-policy-group-name"`
	SnapshotNamePtr       *string                        `xml:"snapshot-name"`
	SourcePathPtr         *string                        `xml:"source-path"`
	SpaceReservePtr       *bool                          `xml:"space-reserve"`
	TokenUuidPtr          *string                        `xml:"token-uuid"`
	VolumePtr             *string                        `xml:"volume"`
}

// CloneCreateResponse is a structure to represent a clone-create Response ZAPI object
type CloneCreateResponse struct {
	XMLName         xml.Name                  `xml:"netapp"`
	ResponseVersion string                    `xml:"version,attr"`
	ResponseXmlns   string                    `xml:"xmlns,attr"`
	Result          CloneCreateResponseResult `xml:"results"`
}

// NewCloneCreateResponse is a factory method for creating new instances of CloneCreateResponse objects
func NewCloneCreateResponse() *CloneCreateResponse {
	return &CloneCreateResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CloneCreateResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *CloneCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// CloneCreateResponseResult is a structure to represent a clone-create Response Result ZAPI object
type CloneCreateResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewCloneCreateRequest is a factory method for creating new instances of CloneCreateRequest objects
func NewCloneCreateRequest() *CloneCreateRequest {
	return &CloneCreateRequest{}
}

// NewCloneCreateResponseResult is a factory method for creating new instances of CloneCreateResponseResult objects
func NewCloneCreateResponseResult() *CloneCreateResponseResult {
	return &CloneCreateResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *CloneCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *CloneCreateResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CloneCreateRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CloneCreateResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *CloneCreateRequest) ExecuteUsing(zr *ZapiRunner) (*CloneCreateResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *CloneCreateRequest) executeWithoutIteration(zr *ZapiRunner) (*CloneCreateResponse, error) {
	result, err := zr.ExecuteUsing(o, "CloneCreateRequest", NewCloneCreateResponse())
	if result == nil {
		return nil, err
	}
	return result.(*CloneCreateResponse), err
}

// Autodelete is a 'getter' method
func (o *CloneCreateRequest) Autodelete() bool {
	r := *o.AutodeletePtr
	return r
}

// SetAutodelete is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetAutodelete(newValue bool) *CloneCreateRequest {
	o.AutodeletePtr = &newValue
	return o
}

// CloneCreateRequestBlockRanges is a wrapper
type CloneCreateRequestBlockRanges struct {
	XMLName       xml.Name         `xml:"block-ranges"`
	BlockRangePtr []BlockRangeType `xml:"block-range"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CloneCreateRequestBlockRanges) String() string {
	return ToString(reflect.ValueOf(o))
}

// BlockRange is a 'getter' method
func (o *CloneCreateRequestBlockRanges) BlockRange() []BlockRangeType {
	r := o.BlockRangePtr
	return r
}

// SetBlockRange is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequestBlockRanges) SetBlockRange(newValue []BlockRangeType) *CloneCreateRequestBlockRanges {
	newSlice := make([]BlockRangeType, len(newValue))
	copy(newSlice, newValue)
	o.BlockRangePtr = newSlice
	return o
}

// BlockRanges is a 'getter' method
func (o *CloneCreateRequest) BlockRanges() CloneCreateRequestBlockRanges {
	r := *o.BlockRangesPtr
	return r
}

// SetBlockRanges is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetBlockRanges(newValue CloneCreateRequestBlockRanges) *CloneCreateRequest {
	o.BlockRangesPtr = &newValue
	return o
}

// BypassLicenseCheck is a 'getter' method
func (o *CloneCreateRequest) BypassLicenseCheck() bool {
	r := *o.BypassLicenseCheckPtr
	return r
}

// SetBypassLicenseCheck is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetBypassLicenseCheck(newValue bool) *CloneCreateRequest {
	o.BypassLicenseCheckPtr = &newValue
	return o
}

// BypassThrottle is a 'getter' method
func (o *CloneCreateRequest) BypassThrottle() bool {
	r := *o.BypassThrottlePtr
	return r
}

// SetBypassThrottle is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetBypassThrottle(newValue bool) *CloneCreateRequest {
	o.BypassThrottlePtr = &newValue
	return o
}

// DestinationExists is a 'getter' method
func (o *CloneCreateRequest) DestinationExists() bool {
	r := *o.DestinationExistsPtr
	return r
}

// SetDestinationExists is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetDestinationExists(newValue bool) *CloneCreateRequest {
	o.DestinationExistsPtr = &newValue
	return o
}

// DestinationPath is a 'getter' method
func (o *CloneCreateRequest) DestinationPath() string {
	r := *o.DestinationPathPtr
	return r
}

// SetDestinationPath is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetDestinationPath(newValue string) *CloneCreateRequest {
	o.DestinationPathPtr = &newValue
	return o
}

// DestinationVolume is a 'getter' method
func (o *CloneCreateRequest) DestinationVolume() string {
	r := *o.DestinationVolumePtr
	return r
}

// SetDestinationVolume is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetDestinationVolume(newValue string) *CloneCreateRequest {
	o.DestinationVolumePtr = &newValue
	return o
}

// FixedBlockCount is a 'getter' method
func (o *CloneCreateRequest) FixedBlockCount() int {
	r := *o.FixedBlockCountPtr
	return r
}

// SetFixedBlockCount is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetFixedBlockCount(newValue int) *CloneCreateRequest {
	o.FixedBlockCountPtr = &newValue
	return o
}

// IgnoreLocks is a 'getter' method
func (o *CloneCreateRequest) IgnoreLocks() bool {
	r := *o.IgnoreLocksPtr
	return r
}

// SetIgnoreLocks is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetIgnoreLocks(newValue bool) *CloneCreateRequest {
	o.IgnoreLocksPtr = &newValue
	return o
}

// IgnoreStreams is a 'getter' method
func (o *CloneCreateRequest) IgnoreStreams() bool {
	r := *o.IgnoreStreamsPtr
	return r
}

// SetIgnoreStreams is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetIgnoreStreams(newValue bool) *CloneCreateRequest {
	o.IgnoreStreamsPtr = &newValue
	return o
}

// IsBackup is a 'getter' method
func (o *CloneCreateRequest) IsBackup() bool {
	r := *o.IsBackupPtr
	return r
}

// SetIsBackup is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetIsBackup(newValue bool) *CloneCreateRequest {
	o.IsBackupPtr = &newValue
	return o
}

// IsFixedBlockCount is a 'getter' method
func (o *CloneCreateRequest) IsFixedBlockCount() bool {
	r := *o.IsFixedBlockCountPtr
	return r
}

// SetIsFixedBlockCount is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetIsFixedBlockCount(newValue bool) *CloneCreateRequest {
	o.IsFixedBlockCountPtr = &newValue
	return o
}

// IsVvolBackup is a 'getter' method
func (o *CloneCreateRequest) IsVvolBackup() bool {
	r := *o.IsVvolBackupPtr
	return r
}

// SetIsVvolBackup is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetIsVvolBackup(newValue bool) *CloneCreateRequest {
	o.IsVvolBackupPtr = &newValue
	return o
}

// LunSerialNumber is a 'getter' method
func (o *CloneCreateRequest) LunSerialNumber() string {
	r := *o.LunSerialNumberPtr
	return r
}

// SetLunSerialNumber is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetLunSerialNumber(newValue string) *CloneCreateRequest {
	o.LunSerialNumberPtr = &newValue
	return o
}

// NosplitEntry is a 'getter' method
func (o *CloneCreateRequest) NosplitEntry() bool {
	r := *o.NosplitEntryPtr
	return r
}

// SetNosplitEntry is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetNosplitEntry(newValue bool) *CloneCreateRequest {
	o.NosplitEntryPtr = &newValue
	return o
}

// QosPolicyGroupName is a 'getter' method
func (o *CloneCreateRequest) QosPolicyGroupName() string {
	r := *o.QosPolicyGroupNamePtr
	return r
}

// SetQosPolicyGroupName is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetQosPolicyGroupName(newValue string) *CloneCreateRequest {
	o.QosPolicyGroupNamePtr = &newValue
	return o
}

// QosAdaptivePolicyGroupName is a 'getter' method
func (o *CloneCreateRequest) QosAdaptivePolicyGroupName() string {
	r := *o.QosAdaptivePolicyGroupNamePtr
	return r
}

// SetQosAdaptivePolicyGroupName is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetQosAdaptivePolicyGroupName(newValue string) *CloneCreateRequest {
	o.QosAdaptivePolicyGroupNamePtr = &newValue
	return o
}

// SnapshotName is a 'getter' method
func (o *CloneCreateRequest) SnapshotName() string {
	r := *o.SnapshotNamePtr
	return r
}

// SetSnapshotName is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetSnapshotName(newValue string) *CloneCreateRequest {
	o.SnapshotNamePtr = &newValue
	return o
}

// SourcePath is a 'getter' method
func (o *CloneCreateRequest) SourcePath() string {
	r := *o.SourcePathPtr
	return r
}

// SetSourcePath is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetSourcePath(newValue string) *CloneCreateRequest {
	o.SourcePathPtr = &newValue
	return o
}

// SpaceReserve is a 'getter' method
func (o *CloneCreateRequest) SpaceReserve() bool {
	r := *o.SpaceReservePtr
	return r
}

// SetSpaceReserve is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetSpaceReserve(newValue bool) *CloneCreateRequest {
	o.SpaceReservePtr = &newValue
	return o
}

// TokenUuid is a 'getter' method
func (o *CloneCreateRequest) TokenUuid() string {
	r := *o.TokenUuidPtr
	return r
}

// SetTokenUuid is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetTokenUuid(newValue string) *CloneCreateRequest {
	o.TokenUuidPtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *CloneCreateRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *CloneCreateRequest) SetVolume(newValue string) *CloneCreateRequest {
	o.VolumePtr = &newValue
	return o
}
