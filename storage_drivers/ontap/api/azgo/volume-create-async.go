// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// VolumeCreateAsyncRequest is a structure to represent a volume-create-async ZAPI request object
type VolumeCreateAsyncRequest struct {
	XMLName xml.Name `xml:"volume-create-async"`

	AggrListPtr                    []AggrNameType `xml:"aggr-list>aggr-name"`
	AggrListMultiplierPtr          *int           `xml:"aggr-list-multiplier"`
	CacheRetentionPriorityPtr      *string        `xml:"cache-retention-priority"`
	CachingPolicyPtr               *string        `xml:"caching-policy"`
	ContainingAggrNamePtr          *string        `xml:"containing-aggr-name"`
	DataAggrListPtr                []AggrNameType `xml:"data-aggr-list>aggr-name"`
	EfficiencyPolicyPtr            *string        `xml:"efficiency-policy"`
	EnableObjectStorePtr           *bool          `xml:"enable-object-store"`
	EnableSnapdiffPtr              *bool          `xml:"enable-snapdiff"`
	EncryptPtr                     *bool          `xml:"encrypt"`
	ExcludedFromAutobalancePtr     *bool          `xml:"excluded-from-autobalance"`
	ExportPolicyPtr                *string        `xml:"export-policy"`
	FlexcacheCachePolicyPtr        *string        `xml:"flexcache-cache-policy"`
	FlexcacheFillPolicyPtr         *string        `xml:"flexcache-fill-policy"`
	FlexcacheOriginVolumeNamePtr   *string        `xml:"flexcache-origin-volume-name"`
	GroupIdPtr                     *int           `xml:"group-id"`
	IsJunctionActivePtr            *bool          `xml:"is-junction-active"`
	IsManagedByServicePtr          *bool          `xml:"is-managed-by-service"`
	IsNvfailEnabledPtr             *bool          `xml:"is-nvfail-enabled"`
	IsVserverRootPtr               *bool          `xml:"is-vserver-root"`
	JunctionPathPtr                *string        `xml:"junction-path"`
	LanguageCodePtr                *string        `xml:"language-code"`
	MaxConstituentSizePtr          *int           `xml:"max-constituent-size"`
	MaxDataConstituentSizePtr      *int           `xml:"max-data-constituent-size"`
	MaxDirSizePtr                  *int           `xml:"max-dir-size"`
	MaxNamespaceConstituentSizePtr *int           `xml:"max-namespace-constituent-size"`
	NamespaceAggregatePtr          *string        `xml:"namespace-aggregate"`
	NamespaceMirrorAggrListPtr     []AggrNameType `xml:"namespace-mirror-aggr-list>aggr-name"`
	ObjectWriteSyncPeriodPtr       *int           `xml:"object-write-sync-period"`
	OlsAggrListPtr                 []AggrNameType `xml:"ols-aggr-list>aggr-name"`
	OlsConstituentCountPtr         *int           `xml:"ols-constituent-count"`
	OlsConstituentSizePtr          *int           `xml:"ols-constituent-size"`
	PercentageSnapshotReservePtr   *int           `xml:"percentage-snapshot-reserve"`
	QosPolicyGroupNamePtr          *string        `xml:"qos-policy-group-name"`
	SizePtr                        *int           `xml:"size"`
	SnapshotPolicyPtr              *string        `xml:"snapshot-policy"`
	SpaceGuaranteePtr              *string        `xml:"space-guarantee"`
	SpaceReservePtr                *string        `xml:"space-reserve"`
	SpaceSloPtr                    *string        `xml:"space-slo"`
	StorageServicePtr              *string        `xml:"storage-service"`
	UnixPermissionsPtr             *string        `xml:"unix-permissions"`
	UserIdPtr                      *int           `xml:"user-id"`
	VmAlignSectorPtr               *int           `xml:"vm-align-sector"`
	VmAlignSuffixPtr               *string        `xml:"vm-align-suffix"`
	VolumeCommentPtr               *string        `xml:"volume-comment"`
	VolumeNamePtr                  *string        `xml:"volume-name"`
	VolumeSecurityStylePtr         *string        `xml:"volume-security-style"`
	VolumeStatePtr                 *string        `xml:"volume-state"`
	VolumeTypePtr                  *string        `xml:"volume-type"`
	VserverDrProtectionPtr         *string        `xml:"vserver-dr-protection"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeCreateAsyncRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewVolumeCreateAsyncRequest is a factory method for creating new instances of VolumeCreateAsyncRequest objects
func NewVolumeCreateAsyncRequest() *VolumeCreateAsyncRequest { return &VolumeCreateAsyncRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeCreateAsyncRequest) ExecuteUsing(zr *ZapiRunner) (VolumeCreateAsyncResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "VolumeCreateAsyncRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return VolumeCreateAsyncResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return VolumeCreateAsyncResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n VolumeCreateAsyncResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		return VolumeCreateAsyncResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("volume-create-async result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCreateAsyncRequest) String() string {
	var buffer bytes.Buffer
	if o.AggrListPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "aggr-list", o.AggrListPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("aggr-list: nil\n"))
	}
	if o.AggrListMultiplierPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "aggr-list-multiplier", *o.AggrListMultiplierPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("aggr-list-multiplier: nil\n"))
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
	if o.ContainingAggrNamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "containing-aggr-name", *o.ContainingAggrNamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("containing-aggr-name: nil\n"))
	}
	if o.DataAggrListPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "data-aggr-list", o.DataAggrListPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("data-aggr-list: nil\n"))
	}
	if o.EfficiencyPolicyPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "efficiency-policy", *o.EfficiencyPolicyPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("efficiency-policy: nil\n"))
	}
	if o.EnableObjectStorePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "enable-object-store", *o.EnableObjectStorePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("enable-object-store: nil\n"))
	}
	if o.EnableSnapdiffPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "enable-snapdiff", *o.EnableSnapdiffPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("enable-snapdiff: nil\n"))
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
	if o.IsManagedByServicePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "is-managed-by-service", *o.IsManagedByServicePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("is-managed-by-service: nil\n"))
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
	if o.MaxConstituentSizePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "max-constituent-size", *o.MaxConstituentSizePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("max-constituent-size: nil\n"))
	}
	if o.MaxDataConstituentSizePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "max-data-constituent-size", *o.MaxDataConstituentSizePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("max-data-constituent-size: nil\n"))
	}
	if o.MaxDirSizePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "max-dir-size", *o.MaxDirSizePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("max-dir-size: nil\n"))
	}
	if o.MaxNamespaceConstituentSizePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "max-namespace-constituent-size", *o.MaxNamespaceConstituentSizePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("max-namespace-constituent-size: nil\n"))
	}
	if o.NamespaceAggregatePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "namespace-aggregate", *o.NamespaceAggregatePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("namespace-aggregate: nil\n"))
	}
	if o.NamespaceMirrorAggrListPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "namespace-mirror-aggr-list", o.NamespaceMirrorAggrListPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("namespace-mirror-aggr-list: nil\n"))
	}
	if o.ObjectWriteSyncPeriodPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "object-write-sync-period", *o.ObjectWriteSyncPeriodPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("object-write-sync-period: nil\n"))
	}
	if o.OlsAggrListPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "ols-aggr-list", o.OlsAggrListPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("ols-aggr-list: nil\n"))
	}
	if o.OlsConstituentCountPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "ols-constituent-count", *o.OlsConstituentCountPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("ols-constituent-count: nil\n"))
	}
	if o.OlsConstituentSizePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "ols-constituent-size", *o.OlsConstituentSizePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("ols-constituent-size: nil\n"))
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
	if o.SpaceGuaranteePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "space-guarantee", *o.SpaceGuaranteePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("space-guarantee: nil\n"))
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
	if o.VolumeCommentPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume-comment", *o.VolumeCommentPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume-comment: nil\n"))
	}
	if o.VolumeNamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume-name", *o.VolumeNamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume-name: nil\n"))
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

// AggrList is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) AggrList() []AggrNameType {
	r := o.AggrListPtr
	return r
}

// SetAggrList is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetAggrList(newValue []AggrNameType) *VolumeCreateAsyncRequest {
	newSlice := make([]AggrNameType, len(newValue))
	copy(newSlice, newValue)
	o.AggrListPtr = newSlice
	return o
}

// AggrListMultiplier is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) AggrListMultiplier() int {
	r := *o.AggrListMultiplierPtr
	return r
}

// SetAggrListMultiplier is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetAggrListMultiplier(newValue int) *VolumeCreateAsyncRequest {
	o.AggrListMultiplierPtr = &newValue
	return o
}

// CacheRetentionPriority is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) CacheRetentionPriority() string {
	r := *o.CacheRetentionPriorityPtr
	return r
}

// SetCacheRetentionPriority is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetCacheRetentionPriority(newValue string) *VolumeCreateAsyncRequest {
	o.CacheRetentionPriorityPtr = &newValue
	return o
}

// CachingPolicy is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) CachingPolicy() string {
	r := *o.CachingPolicyPtr
	return r
}

// SetCachingPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetCachingPolicy(newValue string) *VolumeCreateAsyncRequest {
	o.CachingPolicyPtr = &newValue
	return o
}

// ContainingAggrName is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) ContainingAggrName() string {
	r := *o.ContainingAggrNamePtr
	return r
}

// SetContainingAggrName is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetContainingAggrName(newValue string) *VolumeCreateAsyncRequest {
	o.ContainingAggrNamePtr = &newValue
	return o
}

// DataAggrList is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) DataAggrList() []AggrNameType {
	r := o.DataAggrListPtr
	return r
}

// SetDataAggrList is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetDataAggrList(newValue []AggrNameType) *VolumeCreateAsyncRequest {
	newSlice := make([]AggrNameType, len(newValue))
	copy(newSlice, newValue)
	o.DataAggrListPtr = newSlice
	return o
}

// EfficiencyPolicy is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) EfficiencyPolicy() string {
	r := *o.EfficiencyPolicyPtr
	return r
}

// SetEfficiencyPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetEfficiencyPolicy(newValue string) *VolumeCreateAsyncRequest {
	o.EfficiencyPolicyPtr = &newValue
	return o
}

// EnableObjectStore is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) EnableObjectStore() bool {
	r := *o.EnableObjectStorePtr
	return r
}

// SetEnableObjectStore is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetEnableObjectStore(newValue bool) *VolumeCreateAsyncRequest {
	o.EnableObjectStorePtr = &newValue
	return o
}

// EnableSnapdiff is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) EnableSnapdiff() bool {
	r := *o.EnableSnapdiffPtr
	return r
}

// SetEnableSnapdiff is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetEnableSnapdiff(newValue bool) *VolumeCreateAsyncRequest {
	o.EnableSnapdiffPtr = &newValue
	return o
}

// Encrypt is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) Encrypt() bool {
	r := *o.EncryptPtr
	return r
}

// SetEncrypt is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetEncrypt(newValue bool) *VolumeCreateAsyncRequest {
	o.EncryptPtr = &newValue
	return o
}

// ExcludedFromAutobalance is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) ExcludedFromAutobalance() bool {
	r := *o.ExcludedFromAutobalancePtr
	return r
}

// SetExcludedFromAutobalance is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetExcludedFromAutobalance(newValue bool) *VolumeCreateAsyncRequest {
	o.ExcludedFromAutobalancePtr = &newValue
	return o
}

// ExportPolicy is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) ExportPolicy() string {
	r := *o.ExportPolicyPtr
	return r
}

// SetExportPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetExportPolicy(newValue string) *VolumeCreateAsyncRequest {
	o.ExportPolicyPtr = &newValue
	return o
}

// FlexcacheCachePolicy is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) FlexcacheCachePolicy() string {
	r := *o.FlexcacheCachePolicyPtr
	return r
}

// SetFlexcacheCachePolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetFlexcacheCachePolicy(newValue string) *VolumeCreateAsyncRequest {
	o.FlexcacheCachePolicyPtr = &newValue
	return o
}

// FlexcacheFillPolicy is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) FlexcacheFillPolicy() string {
	r := *o.FlexcacheFillPolicyPtr
	return r
}

// SetFlexcacheFillPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetFlexcacheFillPolicy(newValue string) *VolumeCreateAsyncRequest {
	o.FlexcacheFillPolicyPtr = &newValue
	return o
}

// FlexcacheOriginVolumeName is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) FlexcacheOriginVolumeName() string {
	r := *o.FlexcacheOriginVolumeNamePtr
	return r
}

// SetFlexcacheOriginVolumeName is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetFlexcacheOriginVolumeName(newValue string) *VolumeCreateAsyncRequest {
	o.FlexcacheOriginVolumeNamePtr = &newValue
	return o
}

// GroupId is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) GroupId() int {
	r := *o.GroupIdPtr
	return r
}

// SetGroupId is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetGroupId(newValue int) *VolumeCreateAsyncRequest {
	o.GroupIdPtr = &newValue
	return o
}

// IsJunctionActive is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) IsJunctionActive() bool {
	r := *o.IsJunctionActivePtr
	return r
}

// SetIsJunctionActive is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetIsJunctionActive(newValue bool) *VolumeCreateAsyncRequest {
	o.IsJunctionActivePtr = &newValue
	return o
}

// IsManagedByService is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) IsManagedByService() bool {
	r := *o.IsManagedByServicePtr
	return r
}

// SetIsManagedByService is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetIsManagedByService(newValue bool) *VolumeCreateAsyncRequest {
	o.IsManagedByServicePtr = &newValue
	return o
}

// IsNvfailEnabled is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) IsNvfailEnabled() bool {
	r := *o.IsNvfailEnabledPtr
	return r
}

// SetIsNvfailEnabled is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetIsNvfailEnabled(newValue bool) *VolumeCreateAsyncRequest {
	o.IsNvfailEnabledPtr = &newValue
	return o
}

// IsVserverRoot is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) IsVserverRoot() bool {
	r := *o.IsVserverRootPtr
	return r
}

// SetIsVserverRoot is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetIsVserverRoot(newValue bool) *VolumeCreateAsyncRequest {
	o.IsVserverRootPtr = &newValue
	return o
}

// JunctionPath is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) JunctionPath() string {
	r := *o.JunctionPathPtr
	return r
}

// SetJunctionPath is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetJunctionPath(newValue string) *VolumeCreateAsyncRequest {
	o.JunctionPathPtr = &newValue
	return o
}

// LanguageCode is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) LanguageCode() string {
	r := *o.LanguageCodePtr
	return r
}

// SetLanguageCode is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetLanguageCode(newValue string) *VolumeCreateAsyncRequest {
	o.LanguageCodePtr = &newValue
	return o
}

// MaxConstituentSize is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) MaxConstituentSize() int {
	r := *o.MaxConstituentSizePtr
	return r
}

// SetMaxConstituentSize is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetMaxConstituentSize(newValue int) *VolumeCreateAsyncRequest {
	o.MaxConstituentSizePtr = &newValue
	return o
}

// MaxDataConstituentSize is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) MaxDataConstituentSize() int {
	r := *o.MaxDataConstituentSizePtr
	return r
}

// SetMaxDataConstituentSize is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetMaxDataConstituentSize(newValue int) *VolumeCreateAsyncRequest {
	o.MaxDataConstituentSizePtr = &newValue
	return o
}

// MaxDirSize is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) MaxDirSize() int {
	r := *o.MaxDirSizePtr
	return r
}

// SetMaxDirSize is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetMaxDirSize(newValue int) *VolumeCreateAsyncRequest {
	o.MaxDirSizePtr = &newValue
	return o
}

// MaxNamespaceConstituentSize is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) MaxNamespaceConstituentSize() int {
	r := *o.MaxNamespaceConstituentSizePtr
	return r
}

// SetMaxNamespaceConstituentSize is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetMaxNamespaceConstituentSize(newValue int) *VolumeCreateAsyncRequest {
	o.MaxNamespaceConstituentSizePtr = &newValue
	return o
}

// NamespaceAggregate is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) NamespaceAggregate() string {
	r := *o.NamespaceAggregatePtr
	return r
}

// SetNamespaceAggregate is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetNamespaceAggregate(newValue string) *VolumeCreateAsyncRequest {
	o.NamespaceAggregatePtr = &newValue
	return o
}

// NamespaceMirrorAggrList is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) NamespaceMirrorAggrList() []AggrNameType {
	r := o.NamespaceMirrorAggrListPtr
	return r
}

// SetNamespaceMirrorAggrList is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetNamespaceMirrorAggrList(newValue []AggrNameType) *VolumeCreateAsyncRequest {
	newSlice := make([]AggrNameType, len(newValue))
	copy(newSlice, newValue)
	o.NamespaceMirrorAggrListPtr = newSlice
	return o
}

// ObjectWriteSyncPeriod is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) ObjectWriteSyncPeriod() int {
	r := *o.ObjectWriteSyncPeriodPtr
	return r
}

// SetObjectWriteSyncPeriod is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetObjectWriteSyncPeriod(newValue int) *VolumeCreateAsyncRequest {
	o.ObjectWriteSyncPeriodPtr = &newValue
	return o
}

// OlsAggrList is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) OlsAggrList() []AggrNameType {
	r := o.OlsAggrListPtr
	return r
}

// SetOlsAggrList is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetOlsAggrList(newValue []AggrNameType) *VolumeCreateAsyncRequest {
	newSlice := make([]AggrNameType, len(newValue))
	copy(newSlice, newValue)
	o.OlsAggrListPtr = newSlice
	return o
}

// OlsConstituentCount is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) OlsConstituentCount() int {
	r := *o.OlsConstituentCountPtr
	return r
}

// SetOlsConstituentCount is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetOlsConstituentCount(newValue int) *VolumeCreateAsyncRequest {
	o.OlsConstituentCountPtr = &newValue
	return o
}

// OlsConstituentSize is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) OlsConstituentSize() int {
	r := *o.OlsConstituentSizePtr
	return r
}

// SetOlsConstituentSize is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetOlsConstituentSize(newValue int) *VolumeCreateAsyncRequest {
	o.OlsConstituentSizePtr = &newValue
	return o
}

// PercentageSnapshotReserve is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) PercentageSnapshotReserve() int {
	r := *o.PercentageSnapshotReservePtr
	return r
}

// SetPercentageSnapshotReserve is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetPercentageSnapshotReserve(newValue int) *VolumeCreateAsyncRequest {
	o.PercentageSnapshotReservePtr = &newValue
	return o
}

// QosPolicyGroupName is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) QosPolicyGroupName() string {
	r := *o.QosPolicyGroupNamePtr
	return r
}

// SetQosPolicyGroupName is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetQosPolicyGroupName(newValue string) *VolumeCreateAsyncRequest {
	o.QosPolicyGroupNamePtr = &newValue
	return o
}

// Size is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) Size() int {
	r := *o.SizePtr
	return r
}

// SetSize is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetSize(newValue int) *VolumeCreateAsyncRequest {
	o.SizePtr = &newValue
	return o
}

// SnapshotPolicy is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) SnapshotPolicy() string {
	r := *o.SnapshotPolicyPtr
	return r
}

// SetSnapshotPolicy is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetSnapshotPolicy(newValue string) *VolumeCreateAsyncRequest {
	o.SnapshotPolicyPtr = &newValue
	return o
}

// SpaceGuarantee is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) SpaceGuarantee() string {
	r := *o.SpaceGuaranteePtr
	return r
}

// SetSpaceGuarantee is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetSpaceGuarantee(newValue string) *VolumeCreateAsyncRequest {
	o.SpaceGuaranteePtr = &newValue
	return o
}

// SpaceReserve is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) SpaceReserve() string {
	r := *o.SpaceReservePtr
	return r
}

// SetSpaceReserve is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetSpaceReserve(newValue string) *VolumeCreateAsyncRequest {
	o.SpaceReservePtr = &newValue
	return o
}

// SpaceSlo is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) SpaceSlo() string {
	r := *o.SpaceSloPtr
	return r
}

// SetSpaceSlo is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetSpaceSlo(newValue string) *VolumeCreateAsyncRequest {
	o.SpaceSloPtr = &newValue
	return o
}

// StorageService is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) StorageService() string {
	r := *o.StorageServicePtr
	return r
}

// SetStorageService is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetStorageService(newValue string) *VolumeCreateAsyncRequest {
	o.StorageServicePtr = &newValue
	return o
}

// UnixPermissions is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) UnixPermissions() string {
	r := *o.UnixPermissionsPtr
	return r
}

// SetUnixPermissions is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetUnixPermissions(newValue string) *VolumeCreateAsyncRequest {
	o.UnixPermissionsPtr = &newValue
	return o
}

// UserId is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) UserId() int {
	r := *o.UserIdPtr
	return r
}

// SetUserId is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetUserId(newValue int) *VolumeCreateAsyncRequest {
	o.UserIdPtr = &newValue
	return o
}

// VmAlignSector is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) VmAlignSector() int {
	r := *o.VmAlignSectorPtr
	return r
}

// SetVmAlignSector is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetVmAlignSector(newValue int) *VolumeCreateAsyncRequest {
	o.VmAlignSectorPtr = &newValue
	return o
}

// VmAlignSuffix is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) VmAlignSuffix() string {
	r := *o.VmAlignSuffixPtr
	return r
}

// SetVmAlignSuffix is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetVmAlignSuffix(newValue string) *VolumeCreateAsyncRequest {
	o.VmAlignSuffixPtr = &newValue
	return o
}

// VolumeComment is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) VolumeComment() string {
	r := *o.VolumeCommentPtr
	return r
}

// SetVolumeComment is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetVolumeComment(newValue string) *VolumeCreateAsyncRequest {
	o.VolumeCommentPtr = &newValue
	return o
}

// VolumeName is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) VolumeName() string {
	r := *o.VolumeNamePtr
	return r
}

// SetVolumeName is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetVolumeName(newValue string) *VolumeCreateAsyncRequest {
	o.VolumeNamePtr = &newValue
	return o
}

// VolumeSecurityStyle is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) VolumeSecurityStyle() string {
	r := *o.VolumeSecurityStylePtr
	return r
}

// SetVolumeSecurityStyle is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetVolumeSecurityStyle(newValue string) *VolumeCreateAsyncRequest {
	o.VolumeSecurityStylePtr = &newValue
	return o
}

// VolumeState is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) VolumeState() string {
	r := *o.VolumeStatePtr
	return r
}

// SetVolumeState is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetVolumeState(newValue string) *VolumeCreateAsyncRequest {
	o.VolumeStatePtr = &newValue
	return o
}

// VolumeType is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) VolumeType() string {
	r := *o.VolumeTypePtr
	return r
}

// SetVolumeType is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetVolumeType(newValue string) *VolumeCreateAsyncRequest {
	o.VolumeTypePtr = &newValue
	return o
}

// VserverDrProtection is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncRequest) VserverDrProtection() string {
	r := *o.VserverDrProtectionPtr
	return r
}

// SetVserverDrProtection is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncRequest) SetVserverDrProtection(newValue string) *VolumeCreateAsyncRequest {
	o.VserverDrProtectionPtr = &newValue
	return o
}

// VolumeCreateAsyncResponse is a structure to represent a volume-create-async ZAPI response object
type VolumeCreateAsyncResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result VolumeCreateAsyncResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCreateAsyncResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// VolumeCreateAsyncResponseResult is a structure to represent a volume-create-async ZAPI object's result
type VolumeCreateAsyncResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr      string  `xml:"status,attr"`
	ResultReasonAttr      string  `xml:"reason,attr"`
	ResultErrnoAttr       string  `xml:"errno,attr"`
	ResultErrorCodePtr    *int    `xml:"result-error-code"`
	ResultErrorMessagePtr *string `xml:"result-error-message"`
	ResultJobidPtr        *int    `xml:"result-jobid"`
	ResultStatusPtr       *string `xml:"result-status"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeCreateAsyncResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewVolumeCreateAsyncResponse is a factory method for creating new instances of VolumeCreateAsyncResponse objects
func NewVolumeCreateAsyncResponse() *VolumeCreateAsyncResponse { return &VolumeCreateAsyncResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCreateAsyncResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.ResultErrorCodePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "result-error-code", *o.ResultErrorCodePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("result-error-code: nil\n"))
	}
	if o.ResultErrorMessagePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "result-error-message", *o.ResultErrorMessagePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("result-error-message: nil\n"))
	}
	if o.ResultJobidPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "result-jobid", *o.ResultJobidPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("result-jobid: nil\n"))
	}
	if o.ResultStatusPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "result-status", *o.ResultStatusPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("result-status: nil\n"))
	}
	return buffer.String()
}

// ResultErrorCode is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncResponseResult) ResultErrorCode() int {
	r := *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncResponseResult) SetResultErrorCode(newValue int) *VolumeCreateAsyncResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncResponseResult) ResultErrorMessage() string {
	r := *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncResponseResult) SetResultErrorMessage(newValue string) *VolumeCreateAsyncResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncResponseResult) ResultJobid() int {
	r := *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncResponseResult) SetResultJobid(newValue int) *VolumeCreateAsyncResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultStatus is a fluent style 'getter' method that can be chained
func (o *VolumeCreateAsyncResponseResult) ResultStatus() string {
	r := *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *VolumeCreateAsyncResponseResult) SetResultStatus(newValue string) *VolumeCreateAsyncResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}
