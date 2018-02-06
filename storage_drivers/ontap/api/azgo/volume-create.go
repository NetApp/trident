// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// VolumeCreateRequest is a structure to represent a volume-create ZAPI request object
type VolumeCreateRequest struct {
	XMLName xml.Name `xml:"volume-create"`

	AntivirusOnAccessPolicyPtr      *string `xml:"antivirus-on-access-policy"`
	CacheRetentionPriorityPtr       *string `xml:"cache-retention-priority"`
	CachingPolicyPtr                *string `xml:"caching-policy"`
	ConstituentRolePtr              *string `xml:"constituent-role"`
	ContainingAggrNamePtr           *string `xml:"containing-aggr-name"`
	EfficiencyPolicyPtr             *string `xml:"efficiency-policy"`
	EncryptPtr                      *bool   `xml:"encrypt"`
	ExcludedFromAutobalancePtr      *bool   `xml:"excluded-from-autobalance"`
	ExportPolicyPtr                 *string `xml:"export-policy"`
	FlexcacheCachePolicyPtr         *string `xml:"flexcache-cache-policy"`
	FlexcacheFillPolicyPtr          *string `xml:"flexcache-fill-policy"`
	FlexcacheOriginVolumeNamePtr    *string `xml:"flexcache-origin-volume-name"`
	GroupIdPtr                      *int    `xml:"group-id"`
	IsJunctionActivePtr             *bool   `xml:"is-junction-active"`
	IsNvfailEnabledPtr              *string `xml:"is-nvfail-enabled"`
	IsVserverRootPtr                *bool   `xml:"is-vserver-root"`
	JunctionPathPtr                 *string `xml:"junction-path"`
	LanguageCodePtr                 *string `xml:"language-code"`
	MaxDirSizePtr                   *int    `xml:"max-dir-size"`
	MaxWriteAllocBlocksPtr          *int    `xml:"max-write-alloc-blocks"`
	PercentageSnapshotReservePtr    *int    `xml:"percentage-snapshot-reserve"`
	QosPolicyGroupNamePtr           *string `xml:"qos-policy-group-name"`
	SizePtr                         *string `xml:"size"`
	SnapshotPolicyPtr               *string `xml:"snapshot-policy"`
	SpaceReservePtr                 *string `xml:"space-reserve"`
	SpaceSloPtr                     *string `xml:"space-slo"`
	StorageServicePtr               *string `xml:"storage-service"`
	StripeAlgorithmPtr              *string `xml:"stripe-algorithm"`
	StripeConcurrencyPtr            *string `xml:"stripe-concurrency"`
	StripeConstituentVolumeCountPtr *int    `xml:"stripe-constituent-volume-count"`
	StripeOptimizePtr               *string `xml:"stripe-optimize"`
	StripeWidthPtr                  *int    `xml:"stripe-width"`
	UnixPermissionsPtr              *string `xml:"unix-permissions"`
	UserIdPtr                       *int    `xml:"user-id"`
	VmAlignSectorPtr                *int    `xml:"vm-align-sector"`
	VmAlignSuffixPtr                *string `xml:"vm-align-suffix"`
	VolumePtr                       *string `xml:"volume"`
	VolumeCommentPtr                *string `xml:"volume-comment"`
	VolumeSecurityStylePtr          *string `xml:"volume-security-style"`
	VolumeStatePtr                  *string `xml:"volume-state"`
	VolumeTypePtr                   *string `xml:"volume-type"`
	VserverDrProtectionPtr          *string `xml:"vserver-dr-protection"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewVolumeCreateRequest is a factory method for creating new instances of VolumeCreateRequest objects
func NewVolumeCreateRequest() *VolumeCreateRequest { return &VolumeCreateRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeCreateRequest) ExecuteUsing(zr *ZapiRunner) (VolumeCreateResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "VolumeCreateRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return VolumeCreateResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return VolumeCreateResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n VolumeCreateResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return VolumeCreateResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("volume-create result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCreateRequest) String() string {
	var buffer bytes.Buffer
	if o.AntivirusOnAccessPolicyPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "antivirus-on-access-policy", *o.AntivirusOnAccessPolicyPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("antivirus-on-access-policy: nil\n"))
	}
	if o.CacheRetentionPriorityPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "cache-retention-priority", *o.CacheRetentionPriorityPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("cache-retention-priority: nil\n"))
	}
	if o.CachingPolicyPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "caching-policy", *o.CachingPolicyPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("caching-policy: nil\n"))
	}
	if o.ConstituentRolePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "constituent-role", *o.ConstituentRolePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("constituent-role: nil\n"))
	}
	if o.ContainingAggrNamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "containing-aggr-name", *o.ContainingAggrNamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("containing-aggr-name: nil\n"))
	}
	if o.EfficiencyPolicyPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "efficiency-policy", *o.EfficiencyPolicyPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("efficiency-policy: nil\n"))
	}
	if o.EncryptPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "encrypt", *o.EncryptPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("encrypt: nil\n"))
	}
	if o.ExcludedFromAutobalancePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "excluded-from-autobalance", *o.ExcludedFromAutobalancePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("excluded-from-autobalance: nil\n"))
	}
	if o.ExportPolicyPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "export-policy", *o.ExportPolicyPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("export-policy: nil\n"))
	}
	if o.FlexcacheCachePolicyPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "flexcache-cache-policy", *o.FlexcacheCachePolicyPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("flexcache-cache-policy: nil\n"))
	}
	if o.FlexcacheFillPolicyPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "flexcache-fill-policy", *o.FlexcacheFillPolicyPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("flexcache-fill-policy: nil\n"))
	}
	if o.FlexcacheOriginVolumeNamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "flexcache-origin-volume-name", *o.FlexcacheOriginVolumeNamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("flexcache-origin-volume-name: nil\n"))
	}
	if o.GroupIdPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "group-id", *o.GroupIdPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("group-id: nil\n"))
	}
	if o.IsJunctionActivePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "is-junction-active", *o.IsJunctionActivePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("is-junction-active: nil\n"))
	}
	if o.IsNvfailEnabledPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "is-nvfail-enabled", *o.IsNvfailEnabledPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("is-nvfail-enabled: nil\n"))
	}
	if o.IsVserverRootPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "is-vserver-root", *o.IsVserverRootPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("is-vserver-root: nil\n"))
	}
	if o.JunctionPathPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "junction-path", *o.JunctionPathPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("junction-path: nil\n"))
	}
	if o.LanguageCodePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "language-code", *o.LanguageCodePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("language-code: nil\n"))
	}
	if o.MaxDirSizePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "max-dir-size", *o.MaxDirSizePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("max-dir-size: nil\n"))
	}
	if o.MaxWriteAllocBlocksPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "max-write-alloc-blocks", *o.MaxWriteAllocBlocksPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("max-write-alloc-blocks: nil\n"))
	}
	if o.PercentageSnapshotReservePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "percentage-snapshot-reserve", *o.PercentageSnapshotReservePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("percentage-snapshot-reserve: nil\n"))
	}
	if o.QosPolicyGroupNamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "qos-policy-group-name", *o.QosPolicyGroupNamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("qos-policy-group-name: nil\n"))
	}
	if o.SizePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "size", *o.SizePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("size: nil\n"))
	}
	if o.SnapshotPolicyPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "snapshot-policy", *o.SnapshotPolicyPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("snapshot-policy: nil\n"))
	}
	if o.SpaceReservePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "space-reserve", *o.SpaceReservePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("space-reserve: nil\n"))
	}
	if o.SpaceSloPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "space-slo", *o.SpaceSloPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("space-slo: nil\n"))
	}
	if o.StorageServicePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "storage-service", *o.StorageServicePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("storage-service: nil\n"))
	}
	if o.StripeAlgorithmPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "stripe-algorithm", *o.StripeAlgorithmPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("stripe-algorithm: nil\n"))
	}
	if o.StripeConcurrencyPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "stripe-concurrency", *o.StripeConcurrencyPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("stripe-concurrency: nil\n"))
	}
	if o.StripeConstituentVolumeCountPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "stripe-constituent-volume-count", *o.StripeConstituentVolumeCountPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("stripe-constituent-volume-count: nil\n"))
	}
	if o.StripeOptimizePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "stripe-optimize", *o.StripeOptimizePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("stripe-optimize: nil\n"))
	}
	if o.StripeWidthPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "stripe-width", *o.StripeWidthPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("stripe-width: nil\n"))
	}
	if o.UnixPermissionsPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "unix-permissions", *o.UnixPermissionsPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("unix-permissions: nil\n"))
	}
	if o.UserIdPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "user-id", *o.UserIdPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("user-id: nil\n"))
	}
	if o.VmAlignSectorPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "vm-align-sector", *o.VmAlignSectorPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("vm-align-sector: nil\n"))
	}
	if o.VmAlignSuffixPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "vm-align-suffix", *o.VmAlignSuffixPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("vm-align-suffix: nil\n"))
	}
	if o.VolumePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume", *o.VolumePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume: nil\n"))
	}
	if o.VolumeCommentPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume-comment", *o.VolumeCommentPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume-comment: nil\n"))
	}
	if o.VolumeSecurityStylePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume-security-style", *o.VolumeSecurityStylePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume-security-style: nil\n"))
	}
	if o.VolumeStatePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume-state", *o.VolumeStatePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume-state: nil\n"))
	}
	if o.VolumeTypePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume-type", *o.VolumeTypePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume-type: nil\n"))
	}
	if o.VserverDrProtectionPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "vserver-dr-protection", *o.VserverDrProtectionPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("vserver-dr-protection: nil\n"))
	}
	return buffer.String()
}

// AntivirusOnAccessPolicy is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) AntivirusOnAccessPolicy() string {
	r := *o.AntivirusOnAccessPolicyPtr
	return r
}

// SetAntivirusOnAccessPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetAntivirusOnAccessPolicy(newValue string) *VolumeCreateRequest {
	o.AntivirusOnAccessPolicyPtr = &newValue
	return o
}

// CacheRetentionPriority is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) CacheRetentionPriority() string {
	r := *o.CacheRetentionPriorityPtr
	return r
}

// SetCacheRetentionPriority is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetCacheRetentionPriority(newValue string) *VolumeCreateRequest {
	o.CacheRetentionPriorityPtr = &newValue
	return o
}

// CachingPolicy is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) CachingPolicy() string {
	r := *o.CachingPolicyPtr
	return r
}

// SetCachingPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetCachingPolicy(newValue string) *VolumeCreateRequest {
	o.CachingPolicyPtr = &newValue
	return o
}

// ConstituentRole is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) ConstituentRole() string {
	r := *o.ConstituentRolePtr
	return r
}

// SetConstituentRole is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetConstituentRole(newValue string) *VolumeCreateRequest {
	o.ConstituentRolePtr = &newValue
	return o
}

// ContainingAggrName is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) ContainingAggrName() string {
	r := *o.ContainingAggrNamePtr
	return r
}

// SetContainingAggrName is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetContainingAggrName(newValue string) *VolumeCreateRequest {
	o.ContainingAggrNamePtr = &newValue
	return o
}

// EfficiencyPolicy is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) EfficiencyPolicy() string {
	r := *o.EfficiencyPolicyPtr
	return r
}

// SetEfficiencyPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetEfficiencyPolicy(newValue string) *VolumeCreateRequest {
	o.EfficiencyPolicyPtr = &newValue
	return o
}

// Encrypt is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) Encrypt() bool {
	r := *o.EncryptPtr
	return r
}

// SetEncrypt is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetEncrypt(newValue bool) *VolumeCreateRequest {
	o.EncryptPtr = &newValue
	return o
}

// ExcludedFromAutobalance is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) ExcludedFromAutobalance() bool {
	r := *o.ExcludedFromAutobalancePtr
	return r
}

// SetExcludedFromAutobalance is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetExcludedFromAutobalance(newValue bool) *VolumeCreateRequest {
	o.ExcludedFromAutobalancePtr = &newValue
	return o
}

// ExportPolicy is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) ExportPolicy() string {
	r := *o.ExportPolicyPtr
	return r
}

// SetExportPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetExportPolicy(newValue string) *VolumeCreateRequest {
	o.ExportPolicyPtr = &newValue
	return o
}

// FlexcacheCachePolicy is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) FlexcacheCachePolicy() string {
	r := *o.FlexcacheCachePolicyPtr
	return r
}

// SetFlexcacheCachePolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetFlexcacheCachePolicy(newValue string) *VolumeCreateRequest {
	o.FlexcacheCachePolicyPtr = &newValue
	return o
}

// FlexcacheFillPolicy is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) FlexcacheFillPolicy() string {
	r := *o.FlexcacheFillPolicyPtr
	return r
}

// SetFlexcacheFillPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetFlexcacheFillPolicy(newValue string) *VolumeCreateRequest {
	o.FlexcacheFillPolicyPtr = &newValue
	return o
}

// FlexcacheOriginVolumeName is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) FlexcacheOriginVolumeName() string {
	r := *o.FlexcacheOriginVolumeNamePtr
	return r
}

// SetFlexcacheOriginVolumeName is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetFlexcacheOriginVolumeName(newValue string) *VolumeCreateRequest {
	o.FlexcacheOriginVolumeNamePtr = &newValue
	return o
}

// GroupId is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) GroupId() int {
	r := *o.GroupIdPtr
	return r
}

// SetGroupId is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetGroupId(newValue int) *VolumeCreateRequest {
	o.GroupIdPtr = &newValue
	return o
}

// IsJunctionActive is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) IsJunctionActive() bool {
	r := *o.IsJunctionActivePtr
	return r
}

// SetIsJunctionActive is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetIsJunctionActive(newValue bool) *VolumeCreateRequest {
	o.IsJunctionActivePtr = &newValue
	return o
}

// IsNvfailEnabled is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) IsNvfailEnabled() string {
	r := *o.IsNvfailEnabledPtr
	return r
}

// SetIsNvfailEnabled is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetIsNvfailEnabled(newValue string) *VolumeCreateRequest {
	o.IsNvfailEnabledPtr = &newValue
	return o
}

// IsVserverRoot is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) IsVserverRoot() bool {
	r := *o.IsVserverRootPtr
	return r
}

// SetIsVserverRoot is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetIsVserverRoot(newValue bool) *VolumeCreateRequest {
	o.IsVserverRootPtr = &newValue
	return o
}

// JunctionPath is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) JunctionPath() string {
	r := *o.JunctionPathPtr
	return r
}

// SetJunctionPath is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetJunctionPath(newValue string) *VolumeCreateRequest {
	o.JunctionPathPtr = &newValue
	return o
}

// LanguageCode is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) LanguageCode() string {
	r := *o.LanguageCodePtr
	return r
}

// SetLanguageCode is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetLanguageCode(newValue string) *VolumeCreateRequest {
	o.LanguageCodePtr = &newValue
	return o
}

// MaxDirSize is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) MaxDirSize() int {
	r := *o.MaxDirSizePtr
	return r
}

// SetMaxDirSize is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetMaxDirSize(newValue int) *VolumeCreateRequest {
	o.MaxDirSizePtr = &newValue
	return o
}

// MaxWriteAllocBlocks is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) MaxWriteAllocBlocks() int {
	r := *o.MaxWriteAllocBlocksPtr
	return r
}

// SetMaxWriteAllocBlocks is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetMaxWriteAllocBlocks(newValue int) *VolumeCreateRequest {
	o.MaxWriteAllocBlocksPtr = &newValue
	return o
}

// PercentageSnapshotReserve is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) PercentageSnapshotReserve() int {
	r := *o.PercentageSnapshotReservePtr
	return r
}

// SetPercentageSnapshotReserve is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetPercentageSnapshotReserve(newValue int) *VolumeCreateRequest {
	o.PercentageSnapshotReservePtr = &newValue
	return o
}

// QosPolicyGroupName is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) QosPolicyGroupName() string {
	r := *o.QosPolicyGroupNamePtr
	return r
}

// SetQosPolicyGroupName is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetQosPolicyGroupName(newValue string) *VolumeCreateRequest {
	o.QosPolicyGroupNamePtr = &newValue
	return o
}

// Size is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) Size() string {
	r := *o.SizePtr
	return r
}

// SetSize is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetSize(newValue string) *VolumeCreateRequest {
	o.SizePtr = &newValue
	return o
}

// SnapshotPolicy is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) SnapshotPolicy() string {
	r := *o.SnapshotPolicyPtr
	return r
}

// SetSnapshotPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetSnapshotPolicy(newValue string) *VolumeCreateRequest {
	o.SnapshotPolicyPtr = &newValue
	return o
}

// SpaceReserve is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) SpaceReserve() string {
	r := *o.SpaceReservePtr
	return r
}

// SetSpaceReserve is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetSpaceReserve(newValue string) *VolumeCreateRequest {
	o.SpaceReservePtr = &newValue
	return o
}

// SpaceSlo is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) SpaceSlo() string {
	r := *o.SpaceSloPtr
	return r
}

// SetSpaceSlo is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetSpaceSlo(newValue string) *VolumeCreateRequest {
	o.SpaceSloPtr = &newValue
	return o
}

// StorageService is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) StorageService() string {
	r := *o.StorageServicePtr
	return r
}

// SetStorageService is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetStorageService(newValue string) *VolumeCreateRequest {
	o.StorageServicePtr = &newValue
	return o
}

// StripeAlgorithm is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) StripeAlgorithm() string {
	r := *o.StripeAlgorithmPtr
	return r
}

// SetStripeAlgorithm is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetStripeAlgorithm(newValue string) *VolumeCreateRequest {
	o.StripeAlgorithmPtr = &newValue
	return o
}

// StripeConcurrency is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) StripeConcurrency() string {
	r := *o.StripeConcurrencyPtr
	return r
}

// SetStripeConcurrency is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetStripeConcurrency(newValue string) *VolumeCreateRequest {
	o.StripeConcurrencyPtr = &newValue
	return o
}

// StripeConstituentVolumeCount is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) StripeConstituentVolumeCount() int {
	r := *o.StripeConstituentVolumeCountPtr
	return r
}

// SetStripeConstituentVolumeCount is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetStripeConstituentVolumeCount(newValue int) *VolumeCreateRequest {
	o.StripeConstituentVolumeCountPtr = &newValue
	return o
}

// StripeOptimize is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) StripeOptimize() string {
	r := *o.StripeOptimizePtr
	return r
}

// SetStripeOptimize is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetStripeOptimize(newValue string) *VolumeCreateRequest {
	o.StripeOptimizePtr = &newValue
	return o
}

// StripeWidth is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) StripeWidth() int {
	r := *o.StripeWidthPtr
	return r
}

// SetStripeWidth is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetStripeWidth(newValue int) *VolumeCreateRequest {
	o.StripeWidthPtr = &newValue
	return o
}

// UnixPermissions is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) UnixPermissions() string {
	r := *o.UnixPermissionsPtr
	return r
}

// SetUnixPermissions is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetUnixPermissions(newValue string) *VolumeCreateRequest {
	o.UnixPermissionsPtr = &newValue
	return o
}

// UserId is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) UserId() int {
	r := *o.UserIdPtr
	return r
}

// SetUserId is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetUserId(newValue int) *VolumeCreateRequest {
	o.UserIdPtr = &newValue
	return o
}

// VmAlignSector is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) VmAlignSector() int {
	r := *o.VmAlignSectorPtr
	return r
}

// SetVmAlignSector is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetVmAlignSector(newValue int) *VolumeCreateRequest {
	o.VmAlignSectorPtr = &newValue
	return o
}

// VmAlignSuffix is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) VmAlignSuffix() string {
	r := *o.VmAlignSuffixPtr
	return r
}

// SetVmAlignSuffix is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetVmAlignSuffix(newValue string) *VolumeCreateRequest {
	o.VmAlignSuffixPtr = &newValue
	return o
}

// Volume is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetVolume(newValue string) *VolumeCreateRequest {
	o.VolumePtr = &newValue
	return o
}

// VolumeComment is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) VolumeComment() string {
	r := *o.VolumeCommentPtr
	return r
}

// SetVolumeComment is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetVolumeComment(newValue string) *VolumeCreateRequest {
	o.VolumeCommentPtr = &newValue
	return o
}

// VolumeSecurityStyle is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) VolumeSecurityStyle() string {
	r := *o.VolumeSecurityStylePtr
	return r
}

// SetVolumeSecurityStyle is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetVolumeSecurityStyle(newValue string) *VolumeCreateRequest {
	o.VolumeSecurityStylePtr = &newValue
	return o
}

// VolumeState is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) VolumeState() string {
	r := *o.VolumeStatePtr
	return r
}

// SetVolumeState is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetVolumeState(newValue string) *VolumeCreateRequest {
	o.VolumeStatePtr = &newValue
	return o
}

// VolumeType is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) VolumeType() string {
	r := *o.VolumeTypePtr
	return r
}

// SetVolumeType is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetVolumeType(newValue string) *VolumeCreateRequest {
	o.VolumeTypePtr = &newValue
	return o
}

// VserverDrProtection is a fluent style 'getter' method that can be chained
func (o *VolumeCreateRequest) VserverDrProtection() string {
	r := *o.VserverDrProtectionPtr
	return r
}

// SetVserverDrProtection is a fluent style 'setter' method that can be chained
func (o *VolumeCreateRequest) SetVserverDrProtection(newValue string) *VolumeCreateRequest {
	o.VserverDrProtectionPtr = &newValue
	return o
}

// VolumeCreateResponse is a structure to represent a volume-create ZAPI response object
type VolumeCreateResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result VolumeCreateResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCreateResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// VolumeCreateResponseResult is a structure to represent a volume-create ZAPI object's result
type VolumeCreateResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewVolumeCreateResponse is a factory method for creating new instances of VolumeCreateResponse objects
func NewVolumeCreateResponse() *VolumeCreateResponse { return &VolumeCreateResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCreateResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	return buffer.String()
}
