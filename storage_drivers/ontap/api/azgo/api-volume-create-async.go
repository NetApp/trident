// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// VolumeCreateAsyncRequest is a structure to represent a volume-create-async Request ZAPI object
type VolumeCreateAsyncRequest struct {
	XMLName                        xml.Name                                         `xml:"volume-create-async"`
	AggrListPtr                    *VolumeCreateAsyncRequestAggrList                `xml:"aggr-list"`
	AggrListMultiplierPtr          *int                                             `xml:"aggr-list-multiplier"`
	AutoProvisionAsPtr             *string                                          `xml:"auto-provision-as"`
	CacheRetentionPriorityPtr      *string                                          `xml:"cache-retention-priority"`
	CachingPolicyPtr               *string                                          `xml:"caching-policy"`
	ContainingAggrNamePtr          *string                                          `xml:"containing-aggr-name"`
	DataAggrListPtr                *VolumeCreateAsyncRequestDataAggrList            `xml:"data-aggr-list"`
	EfficiencyPolicyPtr            *string                                          `xml:"efficiency-policy"`
	EnableObjectStorePtr           *bool                                            `xml:"enable-object-store"`
	EnableSnapdiffPtr              *bool                                            `xml:"enable-snapdiff"`
	EncryptPtr                     *bool                                            `xml:"encrypt"`
	ExcludedFromAutobalancePtr     *bool                                            `xml:"excluded-from-autobalance"`
	ExportPolicyPtr                *string                                          `xml:"export-policy"`
	FlexcacheCachePolicyPtr        *string                                          `xml:"flexcache-cache-policy"`
	FlexcacheFillPolicyPtr         *string                                          `xml:"flexcache-fill-policy"`
	FlexcacheOriginVolumeNamePtr   *string                                          `xml:"flexcache-origin-volume-name"`
	GroupIdPtr                     *int                                             `xml:"group-id"`
	IsJunctionActivePtr            *bool                                            `xml:"is-junction-active"`
	IsManagedByServicePtr          *bool                                            `xml:"is-managed-by-service"`
	IsNvfailEnabledPtr             *bool                                            `xml:"is-nvfail-enabled"`
	IsVserverRootPtr               *bool                                            `xml:"is-vserver-root"`
	JunctionPathPtr                *string                                          `xml:"junction-path"`
	LanguageCodePtr                *string                                          `xml:"language-code"`
	MaxConstituentSizePtr          *int                                             `xml:"max-constituent-size"`
	MaxDataConstituentSizePtr      *int                                             `xml:"max-data-constituent-size"`
	MaxDirSizePtr                  *int                                             `xml:"max-dir-size"`
	MaxNamespaceConstituentSizePtr *int                                             `xml:"max-namespace-constituent-size"`
	NamespaceAggregatePtr          *string                                          `xml:"namespace-aggregate"`
	NamespaceMirrorAggrListPtr     *VolumeCreateAsyncRequestNamespaceMirrorAggrList `xml:"namespace-mirror-aggr-list"`
	ObjectWriteSyncPeriodPtr       *int                                             `xml:"object-write-sync-period"`
	OlsAggrListPtr                 *VolumeCreateAsyncRequestOlsAggrList             `xml:"ols-aggr-list"`
	OlsConstituentCountPtr         *int                                             `xml:"ols-constituent-count"`
	OlsConstituentSizePtr          *int                                             `xml:"ols-constituent-size"`
	PercentageSnapshotReservePtr   *int                                             `xml:"percentage-snapshot-reserve"`
	QosAdaptivePolicyGroupNamePtr  *string                                          `xml:"qos-adaptive-policy-group-name"`
	QosPolicyGroupNamePtr          *string                                          `xml:"qos-policy-group-name"`
	SizePtr                        *int                                             `xml:"size"`
	SnapshotPolicyPtr              *string                                          `xml:"snapshot-policy"`
	SpaceGuaranteePtr              *string                                          `xml:"space-guarantee"`
	SpaceReservePtr                *string                                          `xml:"space-reserve"`
	SpaceSloPtr                    *string                                          `xml:"space-slo"`
	StorageServicePtr              *string                                          `xml:"storage-service"`
	TieringPolicyPtr               *string                                          `xml:"tiering-policy"`
	UnixPermissionsPtr             *string                                          `xml:"unix-permissions"`
	UserIdPtr                      *int                                             `xml:"user-id"`
	VmAlignSectorPtr               *int                                             `xml:"vm-align-sector"`
	VmAlignSuffixPtr               *string                                          `xml:"vm-align-suffix"`
	VolumeCommentPtr               *string                                          `xml:"volume-comment"`
	VolumeNamePtr                  *string                                          `xml:"volume-name"`
	VolumeSecurityStylePtr         *string                                          `xml:"volume-security-style"`
	VolumeStatePtr                 *string                                          `xml:"volume-state"`
	VolumeTypePtr                  *string                                          `xml:"volume-type"`
	VserverDrProtectionPtr         *string                                          `xml:"vserver-dr-protection"`
}

// VolumeCreateAsyncResponse is a structure to represent a volume-create-async Response ZAPI object
type VolumeCreateAsyncResponse struct {
	XMLName         xml.Name                        `xml:"netapp"`
	ResponseVersion string                          `xml:"version,attr"`
	ResponseXmlns   string                          `xml:"xmlns,attr"`
	Result          VolumeCreateAsyncResponseResult `xml:"results"`
}

// NewVolumeCreateAsyncResponse is a factory method for creating new instances of VolumeCreateAsyncResponse objects
func NewVolumeCreateAsyncResponse() *VolumeCreateAsyncResponse {
	return &VolumeCreateAsyncResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCreateAsyncResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeCreateAsyncResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeCreateAsyncResponseResult is a structure to represent a volume-create-async Response Result ZAPI object
type VolumeCreateAsyncResponseResult struct {
	XMLName               xml.Name `xml:"results"`
	ResultStatusAttr      string   `xml:"status,attr"`
	ResultReasonAttr      string   `xml:"reason,attr"`
	ResultErrnoAttr       string   `xml:"errno,attr"`
	ResultErrorCodePtr    *int     `xml:"result-error-code"`
	ResultErrorMessagePtr *string  `xml:"result-error-message"`
	ResultJobidPtr        *int     `xml:"result-jobid"`
	ResultStatusPtr       *string  `xml:"result-status"`
}

// NewVolumeCreateAsyncRequest is a factory method for creating new instances of VolumeCreateAsyncRequest objects
func NewVolumeCreateAsyncRequest() *VolumeCreateAsyncRequest {
	return &VolumeCreateAsyncRequest{}
}

// NewVolumeCreateAsyncResponseResult is a factory method for creating new instances of VolumeCreateAsyncResponseResult objects
func NewVolumeCreateAsyncResponseResult() *VolumeCreateAsyncResponseResult {
	return &VolumeCreateAsyncResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeCreateAsyncRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeCreateAsyncResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCreateAsyncRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCreateAsyncResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeCreateAsyncRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeCreateAsyncResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeCreateAsyncRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeCreateAsyncResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeCreateAsyncRequest", NewVolumeCreateAsyncResponse())
	if result == nil {
		return nil, err
	}
	return result.(*VolumeCreateAsyncResponse), err
}

// VolumeCreateAsyncRequestAggrList is a wrapper
type VolumeCreateAsyncRequestAggrList struct {
	XMLName     xml.Name       `xml:"aggr-list"`
	AggrNamePtr []AggrNameType `xml:"aggr-name"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCreateAsyncRequestAggrList) String() string {
	return ToString(reflect.ValueOf(o))
}

// AggrName is a 'getter' method
func (o *VolumeCreateAsyncRequestAggrList) AggrName() []AggrNameType {
	r := o.AggrNamePtr
	return r
}

// SetAggrName is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequestAggrList) SetAggrName(newValue []AggrNameType) *VolumeCreateAsyncRequestAggrList {
	newSlice := make([]AggrNameType, len(newValue))
	copy(newSlice, newValue)
	o.AggrNamePtr = newSlice
	return o
}

// AggrList is a 'getter' method
func (o *VolumeCreateAsyncRequest) AggrList() VolumeCreateAsyncRequestAggrList {
	var r VolumeCreateAsyncRequestAggrList
	if o.AggrListPtr == nil {
		return r
	}
	r = *o.AggrListPtr
	return r
}

// SetAggrList is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetAggrList(newValue VolumeCreateAsyncRequestAggrList) *VolumeCreateAsyncRequest {
	o.AggrListPtr = &newValue
	return o
}

// AggrListMultiplier is a 'getter' method
func (o *VolumeCreateAsyncRequest) AggrListMultiplier() int {
	var r int
	if o.AggrListMultiplierPtr == nil {
		return r
	}
	r = *o.AggrListMultiplierPtr
	return r
}

// SetAggrListMultiplier is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetAggrListMultiplier(newValue int) *VolumeCreateAsyncRequest {
	o.AggrListMultiplierPtr = &newValue
	return o
}

// AutoProvisionAs is a 'getter' method
func (o *VolumeCreateAsyncRequest) AutoProvisionAs() string {
	var r string
	if o.AutoProvisionAsPtr == nil {
		return r
	}
	r = *o.AutoProvisionAsPtr
	return r
}

// SetAutoProvisionAs is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetAutoProvisionAs(newValue string) *VolumeCreateAsyncRequest {
	o.AutoProvisionAsPtr = &newValue
	return o
}

// CacheRetentionPriority is a 'getter' method
func (o *VolumeCreateAsyncRequest) CacheRetentionPriority() string {
	var r string
	if o.CacheRetentionPriorityPtr == nil {
		return r
	}
	r = *o.CacheRetentionPriorityPtr
	return r
}

// SetCacheRetentionPriority is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetCacheRetentionPriority(newValue string) *VolumeCreateAsyncRequest {
	o.CacheRetentionPriorityPtr = &newValue
	return o
}

// CachingPolicy is a 'getter' method
func (o *VolumeCreateAsyncRequest) CachingPolicy() string {
	var r string
	if o.CachingPolicyPtr == nil {
		return r
	}
	r = *o.CachingPolicyPtr
	return r
}

// SetCachingPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetCachingPolicy(newValue string) *VolumeCreateAsyncRequest {
	o.CachingPolicyPtr = &newValue
	return o
}

// ContainingAggrName is a 'getter' method
func (o *VolumeCreateAsyncRequest) ContainingAggrName() string {
	var r string
	if o.ContainingAggrNamePtr == nil {
		return r
	}
	r = *o.ContainingAggrNamePtr
	return r
}

// SetContainingAggrName is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetContainingAggrName(newValue string) *VolumeCreateAsyncRequest {
	o.ContainingAggrNamePtr = &newValue
	return o
}

// VolumeCreateAsyncRequestDataAggrList is a wrapper
type VolumeCreateAsyncRequestDataAggrList struct {
	XMLName     xml.Name       `xml:"data-aggr-list"`
	AggrNamePtr []AggrNameType `xml:"aggr-name"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCreateAsyncRequestDataAggrList) String() string {
	return ToString(reflect.ValueOf(o))
}

// AggrName is a 'getter' method
func (o *VolumeCreateAsyncRequestDataAggrList) AggrName() []AggrNameType {
	r := o.AggrNamePtr
	return r
}

// SetAggrName is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequestDataAggrList) SetAggrName(newValue []AggrNameType) *VolumeCreateAsyncRequestDataAggrList {
	newSlice := make([]AggrNameType, len(newValue))
	copy(newSlice, newValue)
	o.AggrNamePtr = newSlice
	return o
}

// DataAggrList is a 'getter' method
func (o *VolumeCreateAsyncRequest) DataAggrList() VolumeCreateAsyncRequestDataAggrList {
	var r VolumeCreateAsyncRequestDataAggrList
	if o.DataAggrListPtr == nil {
		return r
	}
	r = *o.DataAggrListPtr
	return r
}

// SetDataAggrList is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetDataAggrList(newValue VolumeCreateAsyncRequestDataAggrList) *VolumeCreateAsyncRequest {
	o.DataAggrListPtr = &newValue
	return o
}

// EfficiencyPolicy is a 'getter' method
func (o *VolumeCreateAsyncRequest) EfficiencyPolicy() string {
	var r string
	if o.EfficiencyPolicyPtr == nil {
		return r
	}
	r = *o.EfficiencyPolicyPtr
	return r
}

// SetEfficiencyPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetEfficiencyPolicy(newValue string) *VolumeCreateAsyncRequest {
	o.EfficiencyPolicyPtr = &newValue
	return o
}

// EnableObjectStore is a 'getter' method
func (o *VolumeCreateAsyncRequest) EnableObjectStore() bool {
	var r bool
	if o.EnableObjectStorePtr == nil {
		return r
	}
	r = *o.EnableObjectStorePtr
	return r
}

// SetEnableObjectStore is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetEnableObjectStore(newValue bool) *VolumeCreateAsyncRequest {
	o.EnableObjectStorePtr = &newValue
	return o
}

// EnableSnapdiff is a 'getter' method
func (o *VolumeCreateAsyncRequest) EnableSnapdiff() bool {
	var r bool
	if o.EnableSnapdiffPtr == nil {
		return r
	}
	r = *o.EnableSnapdiffPtr
	return r
}

// SetEnableSnapdiff is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetEnableSnapdiff(newValue bool) *VolumeCreateAsyncRequest {
	o.EnableSnapdiffPtr = &newValue
	return o
}

// Encrypt is a 'getter' method
func (o *VolumeCreateAsyncRequest) Encrypt() bool {
	var r bool
	if o.EncryptPtr == nil {
		return r
	}
	r = *o.EncryptPtr
	return r
}

// SetEncrypt is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetEncrypt(newValue bool) *VolumeCreateAsyncRequest {
	o.EncryptPtr = &newValue
	return o
}

// ExcludedFromAutobalance is a 'getter' method
func (o *VolumeCreateAsyncRequest) ExcludedFromAutobalance() bool {
	var r bool
	if o.ExcludedFromAutobalancePtr == nil {
		return r
	}
	r = *o.ExcludedFromAutobalancePtr
	return r
}

// SetExcludedFromAutobalance is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetExcludedFromAutobalance(newValue bool) *VolumeCreateAsyncRequest {
	o.ExcludedFromAutobalancePtr = &newValue
	return o
}

// ExportPolicy is a 'getter' method
func (o *VolumeCreateAsyncRequest) ExportPolicy() string {
	var r string
	if o.ExportPolicyPtr == nil {
		return r
	}
	r = *o.ExportPolicyPtr
	return r
}

// SetExportPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetExportPolicy(newValue string) *VolumeCreateAsyncRequest {
	o.ExportPolicyPtr = &newValue
	return o
}

// FlexcacheCachePolicy is a 'getter' method
func (o *VolumeCreateAsyncRequest) FlexcacheCachePolicy() string {
	var r string
	if o.FlexcacheCachePolicyPtr == nil {
		return r
	}
	r = *o.FlexcacheCachePolicyPtr
	return r
}

// SetFlexcacheCachePolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetFlexcacheCachePolicy(newValue string) *VolumeCreateAsyncRequest {
	o.FlexcacheCachePolicyPtr = &newValue
	return o
}

// FlexcacheFillPolicy is a 'getter' method
func (o *VolumeCreateAsyncRequest) FlexcacheFillPolicy() string {
	var r string
	if o.FlexcacheFillPolicyPtr == nil {
		return r
	}
	r = *o.FlexcacheFillPolicyPtr
	return r
}

// SetFlexcacheFillPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetFlexcacheFillPolicy(newValue string) *VolumeCreateAsyncRequest {
	o.FlexcacheFillPolicyPtr = &newValue
	return o
}

// FlexcacheOriginVolumeName is a 'getter' method
func (o *VolumeCreateAsyncRequest) FlexcacheOriginVolumeName() string {
	var r string
	if o.FlexcacheOriginVolumeNamePtr == nil {
		return r
	}
	r = *o.FlexcacheOriginVolumeNamePtr
	return r
}

// SetFlexcacheOriginVolumeName is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetFlexcacheOriginVolumeName(newValue string) *VolumeCreateAsyncRequest {
	o.FlexcacheOriginVolumeNamePtr = &newValue
	return o
}

// GroupId is a 'getter' method
func (o *VolumeCreateAsyncRequest) GroupId() int {
	var r int
	if o.GroupIdPtr == nil {
		return r
	}
	r = *o.GroupIdPtr
	return r
}

// SetGroupId is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetGroupId(newValue int) *VolumeCreateAsyncRequest {
	o.GroupIdPtr = &newValue
	return o
}

// IsJunctionActive is a 'getter' method
func (o *VolumeCreateAsyncRequest) IsJunctionActive() bool {
	var r bool
	if o.IsJunctionActivePtr == nil {
		return r
	}
	r = *o.IsJunctionActivePtr
	return r
}

// SetIsJunctionActive is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetIsJunctionActive(newValue bool) *VolumeCreateAsyncRequest {
	o.IsJunctionActivePtr = &newValue
	return o
}

// IsManagedByService is a 'getter' method
func (o *VolumeCreateAsyncRequest) IsManagedByService() bool {
	var r bool
	if o.IsManagedByServicePtr == nil {
		return r
	}
	r = *o.IsManagedByServicePtr
	return r
}

// SetIsManagedByService is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetIsManagedByService(newValue bool) *VolumeCreateAsyncRequest {
	o.IsManagedByServicePtr = &newValue
	return o
}

// IsNvfailEnabled is a 'getter' method
func (o *VolumeCreateAsyncRequest) IsNvfailEnabled() bool {
	var r bool
	if o.IsNvfailEnabledPtr == nil {
		return r
	}
	r = *o.IsNvfailEnabledPtr
	return r
}

// SetIsNvfailEnabled is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetIsNvfailEnabled(newValue bool) *VolumeCreateAsyncRequest {
	o.IsNvfailEnabledPtr = &newValue
	return o
}

// IsVserverRoot is a 'getter' method
func (o *VolumeCreateAsyncRequest) IsVserverRoot() bool {
	var r bool
	if o.IsVserverRootPtr == nil {
		return r
	}
	r = *o.IsVserverRootPtr
	return r
}

// SetIsVserverRoot is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetIsVserverRoot(newValue bool) *VolumeCreateAsyncRequest {
	o.IsVserverRootPtr = &newValue
	return o
}

// JunctionPath is a 'getter' method
func (o *VolumeCreateAsyncRequest) JunctionPath() string {
	var r string
	if o.JunctionPathPtr == nil {
		return r
	}
	r = *o.JunctionPathPtr
	return r
}

// SetJunctionPath is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetJunctionPath(newValue string) *VolumeCreateAsyncRequest {
	o.JunctionPathPtr = &newValue
	return o
}

// LanguageCode is a 'getter' method
func (o *VolumeCreateAsyncRequest) LanguageCode() string {
	var r string
	if o.LanguageCodePtr == nil {
		return r
	}
	r = *o.LanguageCodePtr
	return r
}

// SetLanguageCode is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetLanguageCode(newValue string) *VolumeCreateAsyncRequest {
	o.LanguageCodePtr = &newValue
	return o
}

// MaxConstituentSize is a 'getter' method
func (o *VolumeCreateAsyncRequest) MaxConstituentSize() int {
	var r int
	if o.MaxConstituentSizePtr == nil {
		return r
	}
	r = *o.MaxConstituentSizePtr
	return r
}

// SetMaxConstituentSize is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetMaxConstituentSize(newValue int) *VolumeCreateAsyncRequest {
	o.MaxConstituentSizePtr = &newValue
	return o
}

// MaxDataConstituentSize is a 'getter' method
func (o *VolumeCreateAsyncRequest) MaxDataConstituentSize() int {
	var r int
	if o.MaxDataConstituentSizePtr == nil {
		return r
	}
	r = *o.MaxDataConstituentSizePtr
	return r
}

// SetMaxDataConstituentSize is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetMaxDataConstituentSize(newValue int) *VolumeCreateAsyncRequest {
	o.MaxDataConstituentSizePtr = &newValue
	return o
}

// MaxDirSize is a 'getter' method
func (o *VolumeCreateAsyncRequest) MaxDirSize() int {
	var r int
	if o.MaxDirSizePtr == nil {
		return r
	}
	r = *o.MaxDirSizePtr
	return r
}

// SetMaxDirSize is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetMaxDirSize(newValue int) *VolumeCreateAsyncRequest {
	o.MaxDirSizePtr = &newValue
	return o
}

// MaxNamespaceConstituentSize is a 'getter' method
func (o *VolumeCreateAsyncRequest) MaxNamespaceConstituentSize() int {
	var r int
	if o.MaxNamespaceConstituentSizePtr == nil {
		return r
	}
	r = *o.MaxNamespaceConstituentSizePtr
	return r
}

// SetMaxNamespaceConstituentSize is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetMaxNamespaceConstituentSize(newValue int) *VolumeCreateAsyncRequest {
	o.MaxNamespaceConstituentSizePtr = &newValue
	return o
}

// NamespaceAggregate is a 'getter' method
func (o *VolumeCreateAsyncRequest) NamespaceAggregate() string {
	var r string
	if o.NamespaceAggregatePtr == nil {
		return r
	}
	r = *o.NamespaceAggregatePtr
	return r
}

// SetNamespaceAggregate is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetNamespaceAggregate(newValue string) *VolumeCreateAsyncRequest {
	o.NamespaceAggregatePtr = &newValue
	return o
}

// VolumeCreateAsyncRequestNamespaceMirrorAggrList is a wrapper
type VolumeCreateAsyncRequestNamespaceMirrorAggrList struct {
	XMLName     xml.Name       `xml:"namespace-mirror-aggr-list"`
	AggrNamePtr []AggrNameType `xml:"aggr-name"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCreateAsyncRequestNamespaceMirrorAggrList) String() string {
	return ToString(reflect.ValueOf(o))
}

// AggrName is a 'getter' method
func (o *VolumeCreateAsyncRequestNamespaceMirrorAggrList) AggrName() []AggrNameType {
	r := o.AggrNamePtr
	return r
}

// SetAggrName is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequestNamespaceMirrorAggrList) SetAggrName(newValue []AggrNameType) *VolumeCreateAsyncRequestNamespaceMirrorAggrList {
	newSlice := make([]AggrNameType, len(newValue))
	copy(newSlice, newValue)
	o.AggrNamePtr = newSlice
	return o
}

// NamespaceMirrorAggrList is a 'getter' method
func (o *VolumeCreateAsyncRequest) NamespaceMirrorAggrList() VolumeCreateAsyncRequestNamespaceMirrorAggrList {
	var r VolumeCreateAsyncRequestNamespaceMirrorAggrList
	if o.NamespaceMirrorAggrListPtr == nil {
		return r
	}
	r = *o.NamespaceMirrorAggrListPtr
	return r
}

// SetNamespaceMirrorAggrList is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetNamespaceMirrorAggrList(newValue VolumeCreateAsyncRequestNamespaceMirrorAggrList) *VolumeCreateAsyncRequest {
	o.NamespaceMirrorAggrListPtr = &newValue
	return o
}

// ObjectWriteSyncPeriod is a 'getter' method
func (o *VolumeCreateAsyncRequest) ObjectWriteSyncPeriod() int {
	var r int
	if o.ObjectWriteSyncPeriodPtr == nil {
		return r
	}
	r = *o.ObjectWriteSyncPeriodPtr
	return r
}

// SetObjectWriteSyncPeriod is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetObjectWriteSyncPeriod(newValue int) *VolumeCreateAsyncRequest {
	o.ObjectWriteSyncPeriodPtr = &newValue
	return o
}

// VolumeCreateAsyncRequestOlsAggrList is a wrapper
type VolumeCreateAsyncRequestOlsAggrList struct {
	XMLName     xml.Name       `xml:"ols-aggr-list"`
	AggrNamePtr []AggrNameType `xml:"aggr-name"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCreateAsyncRequestOlsAggrList) String() string {
	return ToString(reflect.ValueOf(o))
}

// AggrName is a 'getter' method
func (o *VolumeCreateAsyncRequestOlsAggrList) AggrName() []AggrNameType {
	r := o.AggrNamePtr
	return r
}

// SetAggrName is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequestOlsAggrList) SetAggrName(newValue []AggrNameType) *VolumeCreateAsyncRequestOlsAggrList {
	newSlice := make([]AggrNameType, len(newValue))
	copy(newSlice, newValue)
	o.AggrNamePtr = newSlice
	return o
}

// OlsAggrList is a 'getter' method
func (o *VolumeCreateAsyncRequest) OlsAggrList() VolumeCreateAsyncRequestOlsAggrList {
	var r VolumeCreateAsyncRequestOlsAggrList
	if o.OlsAggrListPtr == nil {
		return r
	}
	r = *o.OlsAggrListPtr
	return r
}

// SetOlsAggrList is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetOlsAggrList(newValue VolumeCreateAsyncRequestOlsAggrList) *VolumeCreateAsyncRequest {
	o.OlsAggrListPtr = &newValue
	return o
}

// OlsConstituentCount is a 'getter' method
func (o *VolumeCreateAsyncRequest) OlsConstituentCount() int {
	var r int
	if o.OlsConstituentCountPtr == nil {
		return r
	}
	r = *o.OlsConstituentCountPtr
	return r
}

// SetOlsConstituentCount is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetOlsConstituentCount(newValue int) *VolumeCreateAsyncRequest {
	o.OlsConstituentCountPtr = &newValue
	return o
}

// OlsConstituentSize is a 'getter' method
func (o *VolumeCreateAsyncRequest) OlsConstituentSize() int {
	var r int
	if o.OlsConstituentSizePtr == nil {
		return r
	}
	r = *o.OlsConstituentSizePtr
	return r
}

// SetOlsConstituentSize is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetOlsConstituentSize(newValue int) *VolumeCreateAsyncRequest {
	o.OlsConstituentSizePtr = &newValue
	return o
}

// PercentageSnapshotReserve is a 'getter' method
func (o *VolumeCreateAsyncRequest) PercentageSnapshotReserve() int {
	var r int
	if o.PercentageSnapshotReservePtr == nil {
		return r
	}
	r = *o.PercentageSnapshotReservePtr
	return r
}

// SetPercentageSnapshotReserve is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetPercentageSnapshotReserve(newValue int) *VolumeCreateAsyncRequest {
	o.PercentageSnapshotReservePtr = &newValue
	return o
}

// QosAdaptivePolicyGroupName is a 'getter' method
func (o *VolumeCreateAsyncRequest) QosAdaptivePolicyGroupName() string {
	var r string
	if o.QosAdaptivePolicyGroupNamePtr == nil {
		return r
	}
	r = *o.QosAdaptivePolicyGroupNamePtr
	return r
}

// SetQosAdaptivePolicyGroupName is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetQosAdaptivePolicyGroupName(newValue string) *VolumeCreateAsyncRequest {
	o.QosAdaptivePolicyGroupNamePtr = &newValue
	return o
}

// QosPolicyGroupName is a 'getter' method
func (o *VolumeCreateAsyncRequest) QosPolicyGroupName() string {
	var r string
	if o.QosPolicyGroupNamePtr == nil {
		return r
	}
	r = *o.QosPolicyGroupNamePtr
	return r
}

// SetQosPolicyGroupName is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetQosPolicyGroupName(newValue string) *VolumeCreateAsyncRequest {
	o.QosPolicyGroupNamePtr = &newValue
	return o
}

// Size is a 'getter' method
func (o *VolumeCreateAsyncRequest) Size() int {
	var r int
	if o.SizePtr == nil {
		return r
	}
	r = *o.SizePtr
	return r
}

// SetSize is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetSize(newValue int) *VolumeCreateAsyncRequest {
	o.SizePtr = &newValue
	return o
}

// SnapshotPolicy is a 'getter' method
func (o *VolumeCreateAsyncRequest) SnapshotPolicy() string {
	var r string
	if o.SnapshotPolicyPtr == nil {
		return r
	}
	r = *o.SnapshotPolicyPtr
	return r
}

// SetSnapshotPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetSnapshotPolicy(newValue string) *VolumeCreateAsyncRequest {
	o.SnapshotPolicyPtr = &newValue
	return o
}

// SpaceGuarantee is a 'getter' method
func (o *VolumeCreateAsyncRequest) SpaceGuarantee() string {
	var r string
	if o.SpaceGuaranteePtr == nil {
		return r
	}
	r = *o.SpaceGuaranteePtr
	return r
}

// SetSpaceGuarantee is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetSpaceGuarantee(newValue string) *VolumeCreateAsyncRequest {
	o.SpaceGuaranteePtr = &newValue
	return o
}

// SpaceReserve is a 'getter' method
func (o *VolumeCreateAsyncRequest) SpaceReserve() string {
	var r string
	if o.SpaceReservePtr == nil {
		return r
	}
	r = *o.SpaceReservePtr
	return r
}

// SetSpaceReserve is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetSpaceReserve(newValue string) *VolumeCreateAsyncRequest {
	o.SpaceReservePtr = &newValue
	return o
}

// SpaceSlo is a 'getter' method
func (o *VolumeCreateAsyncRequest) SpaceSlo() string {
	var r string
	if o.SpaceSloPtr == nil {
		return r
	}
	r = *o.SpaceSloPtr
	return r
}

// SetSpaceSlo is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetSpaceSlo(newValue string) *VolumeCreateAsyncRequest {
	o.SpaceSloPtr = &newValue
	return o
}

// StorageService is a 'getter' method
func (o *VolumeCreateAsyncRequest) StorageService() string {
	var r string
	if o.StorageServicePtr == nil {
		return r
	}
	r = *o.StorageServicePtr
	return r
}

// SetStorageService is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetStorageService(newValue string) *VolumeCreateAsyncRequest {
	o.StorageServicePtr = &newValue
	return o
}

// TieringPolicy is a 'getter' method
func (o *VolumeCreateAsyncRequest) TieringPolicy() string {
	var r string
	if o.TieringPolicyPtr == nil {
		return r
	}
	r = *o.TieringPolicyPtr
	return r
}

// SetTieringPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetTieringPolicy(newValue string) *VolumeCreateAsyncRequest {
	o.TieringPolicyPtr = &newValue
	return o
}

// UnixPermissions is a 'getter' method
func (o *VolumeCreateAsyncRequest) UnixPermissions() string {
	var r string
	if o.UnixPermissionsPtr == nil {
		return r
	}
	r = *o.UnixPermissionsPtr
	return r
}

// SetUnixPermissions is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetUnixPermissions(newValue string) *VolumeCreateAsyncRequest {
	o.UnixPermissionsPtr = &newValue
	return o
}

// UserId is a 'getter' method
func (o *VolumeCreateAsyncRequest) UserId() int {
	var r int
	if o.UserIdPtr == nil {
		return r
	}
	r = *o.UserIdPtr
	return r
}

// SetUserId is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetUserId(newValue int) *VolumeCreateAsyncRequest {
	o.UserIdPtr = &newValue
	return o
}

// VmAlignSector is a 'getter' method
func (o *VolumeCreateAsyncRequest) VmAlignSector() int {
	var r int
	if o.VmAlignSectorPtr == nil {
		return r
	}
	r = *o.VmAlignSectorPtr
	return r
}

// SetVmAlignSector is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetVmAlignSector(newValue int) *VolumeCreateAsyncRequest {
	o.VmAlignSectorPtr = &newValue
	return o
}

// VmAlignSuffix is a 'getter' method
func (o *VolumeCreateAsyncRequest) VmAlignSuffix() string {
	var r string
	if o.VmAlignSuffixPtr == nil {
		return r
	}
	r = *o.VmAlignSuffixPtr
	return r
}

// SetVmAlignSuffix is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetVmAlignSuffix(newValue string) *VolumeCreateAsyncRequest {
	o.VmAlignSuffixPtr = &newValue
	return o
}

// VolumeComment is a 'getter' method
func (o *VolumeCreateAsyncRequest) VolumeComment() string {
	var r string
	if o.VolumeCommentPtr == nil {
		return r
	}
	r = *o.VolumeCommentPtr
	return r
}

// SetVolumeComment is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetVolumeComment(newValue string) *VolumeCreateAsyncRequest {
	o.VolumeCommentPtr = &newValue
	return o
}

// VolumeName is a 'getter' method
func (o *VolumeCreateAsyncRequest) VolumeName() string {
	var r string
	if o.VolumeNamePtr == nil {
		return r
	}
	r = *o.VolumeNamePtr
	return r
}

// SetVolumeName is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetVolumeName(newValue string) *VolumeCreateAsyncRequest {
	o.VolumeNamePtr = &newValue
	return o
}

// VolumeSecurityStyle is a 'getter' method
func (o *VolumeCreateAsyncRequest) VolumeSecurityStyle() string {
	var r string
	if o.VolumeSecurityStylePtr == nil {
		return r
	}
	r = *o.VolumeSecurityStylePtr
	return r
}

// SetVolumeSecurityStyle is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetVolumeSecurityStyle(newValue string) *VolumeCreateAsyncRequest {
	o.VolumeSecurityStylePtr = &newValue
	return o
}

// VolumeState is a 'getter' method
func (o *VolumeCreateAsyncRequest) VolumeState() string {
	var r string
	if o.VolumeStatePtr == nil {
		return r
	}
	r = *o.VolumeStatePtr
	return r
}

// SetVolumeState is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetVolumeState(newValue string) *VolumeCreateAsyncRequest {
	o.VolumeStatePtr = &newValue
	return o
}

// VolumeType is a 'getter' method
func (o *VolumeCreateAsyncRequest) VolumeType() string {
	var r string
	if o.VolumeTypePtr == nil {
		return r
	}
	r = *o.VolumeTypePtr
	return r
}

// SetVolumeType is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetVolumeType(newValue string) *VolumeCreateAsyncRequest {
	o.VolumeTypePtr = &newValue
	return o
}

// VserverDrProtection is a 'getter' method
func (o *VolumeCreateAsyncRequest) VserverDrProtection() string {
	var r string
	if o.VserverDrProtectionPtr == nil {
		return r
	}
	r = *o.VserverDrProtectionPtr
	return r
}

// SetVserverDrProtection is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetVserverDrProtection(newValue string) *VolumeCreateAsyncRequest {
	o.VserverDrProtectionPtr = &newValue
	return o
}

// ResultErrorCode is a 'getter' method
func (o *VolumeCreateAsyncResponseResult) ResultErrorCode() int {
	var r int
	if o.ResultErrorCodePtr == nil {
		return r
	}
	r = *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncResponseResult) SetResultErrorCode(newValue int) *VolumeCreateAsyncResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a 'getter' method
func (o *VolumeCreateAsyncResponseResult) ResultErrorMessage() string {
	var r string
	if o.ResultErrorMessagePtr == nil {
		return r
	}
	r = *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncResponseResult) SetResultErrorMessage(newValue string) *VolumeCreateAsyncResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a 'getter' method
func (o *VolumeCreateAsyncResponseResult) ResultJobid() int {
	var r int
	if o.ResultJobidPtr == nil {
		return r
	}
	r = *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncResponseResult) SetResultJobid(newValue int) *VolumeCreateAsyncResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultStatus is a 'getter' method
func (o *VolumeCreateAsyncResponseResult) ResultStatus() string {
	var r string
	if o.ResultStatusPtr == nil {
		return r
	}
	r = *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncResponseResult) SetResultStatus(newValue string) *VolumeCreateAsyncResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}
