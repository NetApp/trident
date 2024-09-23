// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeSpaceAttributesType is a structure to represent a volume-space-attributes ZAPI object
type VolumeSpaceAttributesType struct {
	XMLName                                   xml.Name          `xml:"volume-space-attributes"`
	ExpectedAvailablePtr                      *int              `xml:"expected-available"`
	FilesystemSizePtr                         *int              `xml:"filesystem-size"`
	IsFilesysSizeFixedPtr                     *bool             `xml:"is-filesys-size-fixed"`
	IsSpaceEnforcementLogicalPtr              *bool             `xml:"is-space-enforcement-logical"`
	IsSpaceGuaranteeEnabledPtr                *bool             `xml:"is-space-guarantee-enabled"`
	IsSpaceReportingLogicalPtr                *bool             `xml:"is-space-reporting-logical"`
	IsSpaceSloEnabledPtr                      *string           `xml:"is-space-slo-enabled"`
	LogicalAvailablePtr                       *int              `xml:"logical-available"`
	LogicalUsedPtr                            *int              `xml:"logical-used"`
	LogicalUsedByAfsPtr                       *int              `xml:"logical-used-by-afs"`
	LogicalUsedBySnapshotsPtr                 *int              `xml:"logical-used-by-snapshots"`
	LogicalUsedPercentPtr                     *int              `xml:"logical-used-percent"`
	MaxConstituentSizePtr                     *SizeType         `xml:"max-constituent-size"`
	OverProvisionedPtr                        *int              `xml:"over-provisioned"`
	OverwriteReservePtr                       *int              `xml:"overwrite-reserve"`
	OverwriteReserveRequiredPtr               *int              `xml:"overwrite-reserve-required"`
	OverwriteReserveUsedPtr                   *int              `xml:"overwrite-reserve-used"`
	OverwriteReserveUsedActualPtr             *int              `xml:"overwrite-reserve-used-actual"`
	PercentageFractionalReservePtr            *int              `xml:"percentage-fractional-reserve"`
	PercentageSizeUsedPtr                     *int              `xml:"percentage-size-used"`
	PercentageSnapshotReservePtr              *int              `xml:"percentage-snapshot-reserve"`
	PercentageSnapshotReserveUsedPtr          *int              `xml:"percentage-snapshot-reserve-used"`
	PerformanceTierInactiveUserDataPtr        *int              `xml:"performance-tier-inactive-user-data"`
	PerformanceTierInactiveUserDataPercentPtr *int              `xml:"performance-tier-inactive-user-data-percent"`
	PhysicalUsedPtr                           *int              `xml:"physical-used"`
	PhysicalUsedPercentPtr                    *int              `xml:"physical-used-percent"`
	SizePtr                                   *int              `xml:"size"`
	SizeAvailablePtr                          *int              `xml:"size-available"`
	SizeAvailableForSnapshotsPtr              *int              `xml:"size-available-for-snapshots"`
	SizeTotalPtr                              *int              `xml:"size-total"`
	SizeUsedPtr                               *int              `xml:"size-used"`
	SizeUsedBySnapshotsPtr                    *int              `xml:"size-used-by-snapshots"`
	SnapshotReserveAvailablePtr               *int              `xml:"snapshot-reserve-available"`
	SnapshotReserveSizePtr                    *int              `xml:"snapshot-reserve-size"`
	SpaceFullThresholdPercentPtr              *int              `xml:"space-full-threshold-percent"`
	SpaceGuaranteePtr                         *string           `xml:"space-guarantee"`
	SpaceMgmtOptionTryFirstPtr                *string           `xml:"space-mgmt-option-try-first"`
	SpaceNearlyFullThresholdPercentPtr        *int              `xml:"space-nearly-full-threshold-percent"`
	SpaceSloPtr                               *SpaceSloEnumType `xml:"space-slo"`
}

// NewVolumeSpaceAttributesType is a factory method for creating new instances of VolumeSpaceAttributesType objects
func NewVolumeSpaceAttributesType() *VolumeSpaceAttributesType {
	return &VolumeSpaceAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeSpaceAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeSpaceAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExpectedAvailable is a 'getter' method
func (o *VolumeSpaceAttributesType) ExpectedAvailable() int {
	var r int
	if o.ExpectedAvailablePtr == nil {
		return r
	}
	r = *o.ExpectedAvailablePtr
	return r
}

// SetExpectedAvailable is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetExpectedAvailable(newValue int) *VolumeSpaceAttributesType {
	o.ExpectedAvailablePtr = &newValue
	return o
}

// FilesystemSize is a 'getter' method
func (o *VolumeSpaceAttributesType) FilesystemSize() int {
	var r int
	if o.FilesystemSizePtr == nil {
		return r
	}
	r = *o.FilesystemSizePtr
	return r
}

// SetFilesystemSize is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetFilesystemSize(newValue int) *VolumeSpaceAttributesType {
	o.FilesystemSizePtr = &newValue
	return o
}

// IsFilesysSizeFixed is a 'getter' method
func (o *VolumeSpaceAttributesType) IsFilesysSizeFixed() bool {
	var r bool
	if o.IsFilesysSizeFixedPtr == nil {
		return r
	}
	r = *o.IsFilesysSizeFixedPtr
	return r
}

// SetIsFilesysSizeFixed is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetIsFilesysSizeFixed(newValue bool) *VolumeSpaceAttributesType {
	o.IsFilesysSizeFixedPtr = &newValue
	return o
}

// IsSpaceEnforcementLogical is a 'getter' method
func (o *VolumeSpaceAttributesType) IsSpaceEnforcementLogical() bool {
	var r bool
	if o.IsSpaceEnforcementLogicalPtr == nil {
		return r
	}
	r = *o.IsSpaceEnforcementLogicalPtr
	return r
}

// SetIsSpaceEnforcementLogical is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetIsSpaceEnforcementLogical(newValue bool) *VolumeSpaceAttributesType {
	o.IsSpaceEnforcementLogicalPtr = &newValue
	return o
}

// IsSpaceGuaranteeEnabled is a 'getter' method
func (o *VolumeSpaceAttributesType) IsSpaceGuaranteeEnabled() bool {
	var r bool
	if o.IsSpaceGuaranteeEnabledPtr == nil {
		return r
	}
	r = *o.IsSpaceGuaranteeEnabledPtr
	return r
}

// SetIsSpaceGuaranteeEnabled is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetIsSpaceGuaranteeEnabled(newValue bool) *VolumeSpaceAttributesType {
	o.IsSpaceGuaranteeEnabledPtr = &newValue
	return o
}

// IsSpaceReportingLogical is a 'getter' method
func (o *VolumeSpaceAttributesType) IsSpaceReportingLogical() bool {
	var r bool
	if o.IsSpaceReportingLogicalPtr == nil {
		return r
	}
	r = *o.IsSpaceReportingLogicalPtr
	return r
}

// SetIsSpaceReportingLogical is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetIsSpaceReportingLogical(newValue bool) *VolumeSpaceAttributesType {
	o.IsSpaceReportingLogicalPtr = &newValue
	return o
}

// IsSpaceSloEnabled is a 'getter' method
func (o *VolumeSpaceAttributesType) IsSpaceSloEnabled() string {
	var r string
	if o.IsSpaceSloEnabledPtr == nil {
		return r
	}
	r = *o.IsSpaceSloEnabledPtr
	return r
}

// SetIsSpaceSloEnabled is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetIsSpaceSloEnabled(newValue string) *VolumeSpaceAttributesType {
	o.IsSpaceSloEnabledPtr = &newValue
	return o
}

// LogicalAvailable is a 'getter' method
func (o *VolumeSpaceAttributesType) LogicalAvailable() int {
	var r int
	if o.LogicalAvailablePtr == nil {
		return r
	}
	r = *o.LogicalAvailablePtr
	return r
}

// SetLogicalAvailable is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetLogicalAvailable(newValue int) *VolumeSpaceAttributesType {
	o.LogicalAvailablePtr = &newValue
	return o
}

// LogicalUsed is a 'getter' method
func (o *VolumeSpaceAttributesType) LogicalUsed() int {
	var r int
	if o.LogicalUsedPtr == nil {
		return r
	}
	r = *o.LogicalUsedPtr
	return r
}

// SetLogicalUsed is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetLogicalUsed(newValue int) *VolumeSpaceAttributesType {
	o.LogicalUsedPtr = &newValue
	return o
}

// LogicalUsedByAfs is a 'getter' method
func (o *VolumeSpaceAttributesType) LogicalUsedByAfs() int {
	var r int
	if o.LogicalUsedByAfsPtr == nil {
		return r
	}
	r = *o.LogicalUsedByAfsPtr
	return r
}

// SetLogicalUsedByAfs is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetLogicalUsedByAfs(newValue int) *VolumeSpaceAttributesType {
	o.LogicalUsedByAfsPtr = &newValue
	return o
}

// LogicalUsedBySnapshots is a 'getter' method
func (o *VolumeSpaceAttributesType) LogicalUsedBySnapshots() int {
	var r int
	if o.LogicalUsedBySnapshotsPtr == nil {
		return r
	}
	r = *o.LogicalUsedBySnapshotsPtr
	return r
}

// SetLogicalUsedBySnapshots is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetLogicalUsedBySnapshots(newValue int) *VolumeSpaceAttributesType {
	o.LogicalUsedBySnapshotsPtr = &newValue
	return o
}

// LogicalUsedPercent is a 'getter' method
func (o *VolumeSpaceAttributesType) LogicalUsedPercent() int {
	var r int
	if o.LogicalUsedPercentPtr == nil {
		return r
	}
	r = *o.LogicalUsedPercentPtr
	return r
}

// SetLogicalUsedPercent is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetLogicalUsedPercent(newValue int) *VolumeSpaceAttributesType {
	o.LogicalUsedPercentPtr = &newValue
	return o
}

// MaxConstituentSize is a 'getter' method
func (o *VolumeSpaceAttributesType) MaxConstituentSize() SizeType {
	var r SizeType
	if o.MaxConstituentSizePtr == nil {
		return r
	}
	r = *o.MaxConstituentSizePtr
	return r
}

// SetMaxConstituentSize is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetMaxConstituentSize(newValue SizeType) *VolumeSpaceAttributesType {
	o.MaxConstituentSizePtr = &newValue
	return o
}

// OverProvisioned is a 'getter' method
func (o *VolumeSpaceAttributesType) OverProvisioned() int {
	var r int
	if o.OverProvisionedPtr == nil {
		return r
	}
	r = *o.OverProvisionedPtr
	return r
}

// SetOverProvisioned is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetOverProvisioned(newValue int) *VolumeSpaceAttributesType {
	o.OverProvisionedPtr = &newValue
	return o
}

// OverwriteReserve is a 'getter' method
func (o *VolumeSpaceAttributesType) OverwriteReserve() int {
	var r int
	if o.OverwriteReservePtr == nil {
		return r
	}
	r = *o.OverwriteReservePtr
	return r
}

// SetOverwriteReserve is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetOverwriteReserve(newValue int) *VolumeSpaceAttributesType {
	o.OverwriteReservePtr = &newValue
	return o
}

// OverwriteReserveRequired is a 'getter' method
func (o *VolumeSpaceAttributesType) OverwriteReserveRequired() int {
	var r int
	if o.OverwriteReserveRequiredPtr == nil {
		return r
	}
	r = *o.OverwriteReserveRequiredPtr
	return r
}

// SetOverwriteReserveRequired is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetOverwriteReserveRequired(newValue int) *VolumeSpaceAttributesType {
	o.OverwriteReserveRequiredPtr = &newValue
	return o
}

// OverwriteReserveUsed is a 'getter' method
func (o *VolumeSpaceAttributesType) OverwriteReserveUsed() int {
	var r int
	if o.OverwriteReserveUsedPtr == nil {
		return r
	}
	r = *o.OverwriteReserveUsedPtr
	return r
}

// SetOverwriteReserveUsed is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetOverwriteReserveUsed(newValue int) *VolumeSpaceAttributesType {
	o.OverwriteReserveUsedPtr = &newValue
	return o
}

// OverwriteReserveUsedActual is a 'getter' method
func (o *VolumeSpaceAttributesType) OverwriteReserveUsedActual() int {
	var r int
	if o.OverwriteReserveUsedActualPtr == nil {
		return r
	}
	r = *o.OverwriteReserveUsedActualPtr
	return r
}

// SetOverwriteReserveUsedActual is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetOverwriteReserveUsedActual(newValue int) *VolumeSpaceAttributesType {
	o.OverwriteReserveUsedActualPtr = &newValue
	return o
}

// PercentageFractionalReserve is a 'getter' method
func (o *VolumeSpaceAttributesType) PercentageFractionalReserve() int {
	var r int
	if o.PercentageFractionalReservePtr == nil {
		return r
	}
	r = *o.PercentageFractionalReservePtr
	return r
}

// SetPercentageFractionalReserve is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetPercentageFractionalReserve(newValue int) *VolumeSpaceAttributesType {
	o.PercentageFractionalReservePtr = &newValue
	return o
}

// PercentageSizeUsed is a 'getter' method
func (o *VolumeSpaceAttributesType) PercentageSizeUsed() int {
	var r int
	if o.PercentageSizeUsedPtr == nil {
		return r
	}
	r = *o.PercentageSizeUsedPtr
	return r
}

// SetPercentageSizeUsed is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetPercentageSizeUsed(newValue int) *VolumeSpaceAttributesType {
	o.PercentageSizeUsedPtr = &newValue
	return o
}

// PercentageSnapshotReserve is a 'getter' method
func (o *VolumeSpaceAttributesType) PercentageSnapshotReserve() int {
	var r int
	if o.PercentageSnapshotReservePtr == nil {
		return r
	}
	r = *o.PercentageSnapshotReservePtr
	return r
}

// SetPercentageSnapshotReserve is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetPercentageSnapshotReserve(newValue int) *VolumeSpaceAttributesType {
	o.PercentageSnapshotReservePtr = &newValue
	return o
}

// PercentageSnapshotReserveUsed is a 'getter' method
func (o *VolumeSpaceAttributesType) PercentageSnapshotReserveUsed() int {
	var r int
	if o.PercentageSnapshotReserveUsedPtr == nil {
		return r
	}
	r = *o.PercentageSnapshotReserveUsedPtr
	return r
}

// SetPercentageSnapshotReserveUsed is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetPercentageSnapshotReserveUsed(newValue int) *VolumeSpaceAttributesType {
	o.PercentageSnapshotReserveUsedPtr = &newValue
	return o
}

// PerformanceTierInactiveUserData is a 'getter' method
func (o *VolumeSpaceAttributesType) PerformanceTierInactiveUserData() int {
	var r int
	if o.PerformanceTierInactiveUserDataPtr == nil {
		return r
	}
	r = *o.PerformanceTierInactiveUserDataPtr
	return r
}

// SetPerformanceTierInactiveUserData is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetPerformanceTierInactiveUserData(newValue int) *VolumeSpaceAttributesType {
	o.PerformanceTierInactiveUserDataPtr = &newValue
	return o
}

// PerformanceTierInactiveUserDataPercent is a 'getter' method
func (o *VolumeSpaceAttributesType) PerformanceTierInactiveUserDataPercent() int {
	var r int
	if o.PerformanceTierInactiveUserDataPercentPtr == nil {
		return r
	}
	r = *o.PerformanceTierInactiveUserDataPercentPtr
	return r
}

// SetPerformanceTierInactiveUserDataPercent is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetPerformanceTierInactiveUserDataPercent(newValue int) *VolumeSpaceAttributesType {
	o.PerformanceTierInactiveUserDataPercentPtr = &newValue
	return o
}

// PhysicalUsed is a 'getter' method
func (o *VolumeSpaceAttributesType) PhysicalUsed() int {
	var r int
	if o.PhysicalUsedPtr == nil {
		return r
	}
	r = *o.PhysicalUsedPtr
	return r
}

// SetPhysicalUsed is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetPhysicalUsed(newValue int) *VolumeSpaceAttributesType {
	o.PhysicalUsedPtr = &newValue
	return o
}

// PhysicalUsedPercent is a 'getter' method
func (o *VolumeSpaceAttributesType) PhysicalUsedPercent() int {
	var r int
	if o.PhysicalUsedPercentPtr == nil {
		return r
	}
	r = *o.PhysicalUsedPercentPtr
	return r
}

// SetPhysicalUsedPercent is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetPhysicalUsedPercent(newValue int) *VolumeSpaceAttributesType {
	o.PhysicalUsedPercentPtr = &newValue
	return o
}

// Size is a 'getter' method
func (o *VolumeSpaceAttributesType) Size() int {
	var r int
	if o.SizePtr == nil {
		return r
	}
	r = *o.SizePtr
	return r
}

// SetSize is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSize(newValue int) *VolumeSpaceAttributesType {
	o.SizePtr = &newValue
	return o
}

// SizeAvailable is a 'getter' method
func (o *VolumeSpaceAttributesType) SizeAvailable() int {
	var r int
	if o.SizeAvailablePtr == nil {
		return r
	}
	r = *o.SizeAvailablePtr
	return r
}

// SetSizeAvailable is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSizeAvailable(newValue int) *VolumeSpaceAttributesType {
	o.SizeAvailablePtr = &newValue
	return o
}

// SizeAvailableForSnapshots is a 'getter' method
func (o *VolumeSpaceAttributesType) SizeAvailableForSnapshots() int {
	var r int
	if o.SizeAvailableForSnapshotsPtr == nil {
		return r
	}
	r = *o.SizeAvailableForSnapshotsPtr
	return r
}

// SetSizeAvailableForSnapshots is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSizeAvailableForSnapshots(newValue int) *VolumeSpaceAttributesType {
	o.SizeAvailableForSnapshotsPtr = &newValue
	return o
}

// SizeTotal is a 'getter' method
func (o *VolumeSpaceAttributesType) SizeTotal() int {
	var r int
	if o.SizeTotalPtr == nil {
		return r
	}
	r = *o.SizeTotalPtr
	return r
}

// SetSizeTotal is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSizeTotal(newValue int) *VolumeSpaceAttributesType {
	o.SizeTotalPtr = &newValue
	return o
}

// SizeUsed is a 'getter' method
func (o *VolumeSpaceAttributesType) SizeUsed() int {
	var r int
	if o.SizeUsedPtr == nil {
		return r
	}
	r = *o.SizeUsedPtr
	return r
}

// SetSizeUsed is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSizeUsed(newValue int) *VolumeSpaceAttributesType {
	o.SizeUsedPtr = &newValue
	return o
}

// SizeUsedBySnapshots is a 'getter' method
func (o *VolumeSpaceAttributesType) SizeUsedBySnapshots() int {
	var r int
	if o.SizeUsedBySnapshotsPtr == nil {
		return r
	}
	r = *o.SizeUsedBySnapshotsPtr
	return r
}

// SetSizeUsedBySnapshots is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSizeUsedBySnapshots(newValue int) *VolumeSpaceAttributesType {
	o.SizeUsedBySnapshotsPtr = &newValue
	return o
}

// SnapshotReserveAvailable is a 'getter' method
func (o *VolumeSpaceAttributesType) SnapshotReserveAvailable() int {
	var r int
	if o.SnapshotReserveAvailablePtr == nil {
		return r
	}
	r = *o.SnapshotReserveAvailablePtr
	return r
}

// SetSnapshotReserveAvailable is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSnapshotReserveAvailable(newValue int) *VolumeSpaceAttributesType {
	o.SnapshotReserveAvailablePtr = &newValue
	return o
}

// SnapshotReserveSize is a 'getter' method
func (o *VolumeSpaceAttributesType) SnapshotReserveSize() int {
	var r int
	if o.SnapshotReserveSizePtr == nil {
		return r
	}
	r = *o.SnapshotReserveSizePtr
	return r
}

// SetSnapshotReserveSize is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSnapshotReserveSize(newValue int) *VolumeSpaceAttributesType {
	o.SnapshotReserveSizePtr = &newValue
	return o
}

// SpaceFullThresholdPercent is a 'getter' method
func (o *VolumeSpaceAttributesType) SpaceFullThresholdPercent() int {
	var r int
	if o.SpaceFullThresholdPercentPtr == nil {
		return r
	}
	r = *o.SpaceFullThresholdPercentPtr
	return r
}

// SetSpaceFullThresholdPercent is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSpaceFullThresholdPercent(newValue int) *VolumeSpaceAttributesType {
	o.SpaceFullThresholdPercentPtr = &newValue
	return o
}

// SpaceGuarantee is a 'getter' method
func (o *VolumeSpaceAttributesType) SpaceGuarantee() string {
	var r string
	if o.SpaceGuaranteePtr == nil {
		return r
	}
	r = *o.SpaceGuaranteePtr
	return r
}

// SetSpaceGuarantee is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSpaceGuarantee(newValue string) *VolumeSpaceAttributesType {
	o.SpaceGuaranteePtr = &newValue
	return o
}

// SpaceMgmtOptionTryFirst is a 'getter' method
func (o *VolumeSpaceAttributesType) SpaceMgmtOptionTryFirst() string {
	var r string
	if o.SpaceMgmtOptionTryFirstPtr == nil {
		return r
	}
	r = *o.SpaceMgmtOptionTryFirstPtr
	return r
}

// SetSpaceMgmtOptionTryFirst is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSpaceMgmtOptionTryFirst(newValue string) *VolumeSpaceAttributesType {
	o.SpaceMgmtOptionTryFirstPtr = &newValue
	return o
}

// SpaceNearlyFullThresholdPercent is a 'getter' method
func (o *VolumeSpaceAttributesType) SpaceNearlyFullThresholdPercent() int {
	var r int
	if o.SpaceNearlyFullThresholdPercentPtr == nil {
		return r
	}
	r = *o.SpaceNearlyFullThresholdPercentPtr
	return r
}

// SetSpaceNearlyFullThresholdPercent is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSpaceNearlyFullThresholdPercent(newValue int) *VolumeSpaceAttributesType {
	o.SpaceNearlyFullThresholdPercentPtr = &newValue
	return o
}

// SpaceSlo is a 'getter' method
func (o *VolumeSpaceAttributesType) SpaceSlo() SpaceSloEnumType {
	var r SpaceSloEnumType
	if o.SpaceSloPtr == nil {
		return r
	}
	r = *o.SpaceSloPtr
	return r
}

// SetSpaceSlo is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSpaceSlo(newValue SpaceSloEnumType) *VolumeSpaceAttributesType {
	o.SpaceSloPtr = &newValue
	return o
}
