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
	r := *o.ExpectedAvailablePtr
	return r
}

// SetExpectedAvailable is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetExpectedAvailable(newValue int) *VolumeSpaceAttributesType {
	o.ExpectedAvailablePtr = &newValue
	return o
}

// FilesystemSize is a 'getter' method
func (o *VolumeSpaceAttributesType) FilesystemSize() int {
	r := *o.FilesystemSizePtr
	return r
}

// SetFilesystemSize is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetFilesystemSize(newValue int) *VolumeSpaceAttributesType {
	o.FilesystemSizePtr = &newValue
	return o
}

// IsFilesysSizeFixed is a 'getter' method
func (o *VolumeSpaceAttributesType) IsFilesysSizeFixed() bool {
	r := *o.IsFilesysSizeFixedPtr
	return r
}

// SetIsFilesysSizeFixed is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetIsFilesysSizeFixed(newValue bool) *VolumeSpaceAttributesType {
	o.IsFilesysSizeFixedPtr = &newValue
	return o
}

// IsSpaceEnforcementLogical is a 'getter' method
func (o *VolumeSpaceAttributesType) IsSpaceEnforcementLogical() bool {
	r := *o.IsSpaceEnforcementLogicalPtr
	return r
}

// SetIsSpaceEnforcementLogical is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetIsSpaceEnforcementLogical(newValue bool) *VolumeSpaceAttributesType {
	o.IsSpaceEnforcementLogicalPtr = &newValue
	return o
}

// IsSpaceGuaranteeEnabled is a 'getter' method
func (o *VolumeSpaceAttributesType) IsSpaceGuaranteeEnabled() bool {
	r := *o.IsSpaceGuaranteeEnabledPtr
	return r
}

// SetIsSpaceGuaranteeEnabled is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetIsSpaceGuaranteeEnabled(newValue bool) *VolumeSpaceAttributesType {
	o.IsSpaceGuaranteeEnabledPtr = &newValue
	return o
}

// IsSpaceReportingLogical is a 'getter' method
func (o *VolumeSpaceAttributesType) IsSpaceReportingLogical() bool {
	r := *o.IsSpaceReportingLogicalPtr
	return r
}

// SetIsSpaceReportingLogical is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetIsSpaceReportingLogical(newValue bool) *VolumeSpaceAttributesType {
	o.IsSpaceReportingLogicalPtr = &newValue
	return o
}

// IsSpaceSloEnabled is a 'getter' method
func (o *VolumeSpaceAttributesType) IsSpaceSloEnabled() string {
	r := *o.IsSpaceSloEnabledPtr
	return r
}

// SetIsSpaceSloEnabled is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetIsSpaceSloEnabled(newValue string) *VolumeSpaceAttributesType {
	o.IsSpaceSloEnabledPtr = &newValue
	return o
}

// LogicalAvailable is a 'getter' method
func (o *VolumeSpaceAttributesType) LogicalAvailable() int {
	r := *o.LogicalAvailablePtr
	return r
}

// SetLogicalAvailable is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetLogicalAvailable(newValue int) *VolumeSpaceAttributesType {
	o.LogicalAvailablePtr = &newValue
	return o
}

// LogicalUsed is a 'getter' method
func (o *VolumeSpaceAttributesType) LogicalUsed() int {
	r := *o.LogicalUsedPtr
	return r
}

// SetLogicalUsed is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetLogicalUsed(newValue int) *VolumeSpaceAttributesType {
	o.LogicalUsedPtr = &newValue
	return o
}

// LogicalUsedByAfs is a 'getter' method
func (o *VolumeSpaceAttributesType) LogicalUsedByAfs() int {
	r := *o.LogicalUsedByAfsPtr
	return r
}

// SetLogicalUsedByAfs is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetLogicalUsedByAfs(newValue int) *VolumeSpaceAttributesType {
	o.LogicalUsedByAfsPtr = &newValue
	return o
}

// LogicalUsedBySnapshots is a 'getter' method
func (o *VolumeSpaceAttributesType) LogicalUsedBySnapshots() int {
	r := *o.LogicalUsedBySnapshotsPtr
	return r
}

// SetLogicalUsedBySnapshots is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetLogicalUsedBySnapshots(newValue int) *VolumeSpaceAttributesType {
	o.LogicalUsedBySnapshotsPtr = &newValue
	return o
}

// LogicalUsedPercent is a 'getter' method
func (o *VolumeSpaceAttributesType) LogicalUsedPercent() int {
	r := *o.LogicalUsedPercentPtr
	return r
}

// SetLogicalUsedPercent is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetLogicalUsedPercent(newValue int) *VolumeSpaceAttributesType {
	o.LogicalUsedPercentPtr = &newValue
	return o
}

// MaxConstituentSize is a 'getter' method
func (o *VolumeSpaceAttributesType) MaxConstituentSize() SizeType {
	r := *o.MaxConstituentSizePtr
	return r
}

// SetMaxConstituentSize is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetMaxConstituentSize(newValue SizeType) *VolumeSpaceAttributesType {
	o.MaxConstituentSizePtr = &newValue
	return o
}

// OverProvisioned is a 'getter' method
func (o *VolumeSpaceAttributesType) OverProvisioned() int {
	r := *o.OverProvisionedPtr
	return r
}

// SetOverProvisioned is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetOverProvisioned(newValue int) *VolumeSpaceAttributesType {
	o.OverProvisionedPtr = &newValue
	return o
}

// OverwriteReserve is a 'getter' method
func (o *VolumeSpaceAttributesType) OverwriteReserve() int {
	r := *o.OverwriteReservePtr
	return r
}

// SetOverwriteReserve is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetOverwriteReserve(newValue int) *VolumeSpaceAttributesType {
	o.OverwriteReservePtr = &newValue
	return o
}

// OverwriteReserveRequired is a 'getter' method
func (o *VolumeSpaceAttributesType) OverwriteReserveRequired() int {
	r := *o.OverwriteReserveRequiredPtr
	return r
}

// SetOverwriteReserveRequired is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetOverwriteReserveRequired(newValue int) *VolumeSpaceAttributesType {
	o.OverwriteReserveRequiredPtr = &newValue
	return o
}

// OverwriteReserveUsed is a 'getter' method
func (o *VolumeSpaceAttributesType) OverwriteReserveUsed() int {
	r := *o.OverwriteReserveUsedPtr
	return r
}

// SetOverwriteReserveUsed is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetOverwriteReserveUsed(newValue int) *VolumeSpaceAttributesType {
	o.OverwriteReserveUsedPtr = &newValue
	return o
}

// OverwriteReserveUsedActual is a 'getter' method
func (o *VolumeSpaceAttributesType) OverwriteReserveUsedActual() int {
	r := *o.OverwriteReserveUsedActualPtr
	return r
}

// SetOverwriteReserveUsedActual is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetOverwriteReserveUsedActual(newValue int) *VolumeSpaceAttributesType {
	o.OverwriteReserveUsedActualPtr = &newValue
	return o
}

// PercentageFractionalReserve is a 'getter' method
func (o *VolumeSpaceAttributesType) PercentageFractionalReserve() int {
	r := *o.PercentageFractionalReservePtr
	return r
}

// SetPercentageFractionalReserve is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetPercentageFractionalReserve(newValue int) *VolumeSpaceAttributesType {
	o.PercentageFractionalReservePtr = &newValue
	return o
}

// PercentageSizeUsed is a 'getter' method
func (o *VolumeSpaceAttributesType) PercentageSizeUsed() int {
	r := *o.PercentageSizeUsedPtr
	return r
}

// SetPercentageSizeUsed is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetPercentageSizeUsed(newValue int) *VolumeSpaceAttributesType {
	o.PercentageSizeUsedPtr = &newValue
	return o
}

// PercentageSnapshotReserve is a 'getter' method
func (o *VolumeSpaceAttributesType) PercentageSnapshotReserve() int {
	r := *o.PercentageSnapshotReservePtr
	return r
}

// SetPercentageSnapshotReserve is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetPercentageSnapshotReserve(newValue int) *VolumeSpaceAttributesType {
	o.PercentageSnapshotReservePtr = &newValue
	return o
}

// PercentageSnapshotReserveUsed is a 'getter' method
func (o *VolumeSpaceAttributesType) PercentageSnapshotReserveUsed() int {
	r := *o.PercentageSnapshotReserveUsedPtr
	return r
}

// SetPercentageSnapshotReserveUsed is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetPercentageSnapshotReserveUsed(newValue int) *VolumeSpaceAttributesType {
	o.PercentageSnapshotReserveUsedPtr = &newValue
	return o
}

// PerformanceTierInactiveUserData is a 'getter' method
func (o *VolumeSpaceAttributesType) PerformanceTierInactiveUserData() int {
	r := *o.PerformanceTierInactiveUserDataPtr
	return r
}

// SetPerformanceTierInactiveUserData is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetPerformanceTierInactiveUserData(newValue int) *VolumeSpaceAttributesType {
	o.PerformanceTierInactiveUserDataPtr = &newValue
	return o
}

// PerformanceTierInactiveUserDataPercent is a 'getter' method
func (o *VolumeSpaceAttributesType) PerformanceTierInactiveUserDataPercent() int {
	r := *o.PerformanceTierInactiveUserDataPercentPtr
	return r
}

// SetPerformanceTierInactiveUserDataPercent is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetPerformanceTierInactiveUserDataPercent(newValue int) *VolumeSpaceAttributesType {
	o.PerformanceTierInactiveUserDataPercentPtr = &newValue
	return o
}

// PhysicalUsed is a 'getter' method
func (o *VolumeSpaceAttributesType) PhysicalUsed() int {
	r := *o.PhysicalUsedPtr
	return r
}

// SetPhysicalUsed is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetPhysicalUsed(newValue int) *VolumeSpaceAttributesType {
	o.PhysicalUsedPtr = &newValue
	return o
}

// PhysicalUsedPercent is a 'getter' method
func (o *VolumeSpaceAttributesType) PhysicalUsedPercent() int {
	r := *o.PhysicalUsedPercentPtr
	return r
}

// SetPhysicalUsedPercent is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetPhysicalUsedPercent(newValue int) *VolumeSpaceAttributesType {
	o.PhysicalUsedPercentPtr = &newValue
	return o
}

// Size is a 'getter' method
func (o *VolumeSpaceAttributesType) Size() int {
	r := *o.SizePtr
	return r
}

// SetSize is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSize(newValue int) *VolumeSpaceAttributesType {
	o.SizePtr = &newValue
	return o
}

// SizeAvailable is a 'getter' method
func (o *VolumeSpaceAttributesType) SizeAvailable() int {
	r := *o.SizeAvailablePtr
	return r
}

// SetSizeAvailable is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSizeAvailable(newValue int) *VolumeSpaceAttributesType {
	o.SizeAvailablePtr = &newValue
	return o
}

// SizeAvailableForSnapshots is a 'getter' method
func (o *VolumeSpaceAttributesType) SizeAvailableForSnapshots() int {
	r := *o.SizeAvailableForSnapshotsPtr
	return r
}

// SetSizeAvailableForSnapshots is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSizeAvailableForSnapshots(newValue int) *VolumeSpaceAttributesType {
	o.SizeAvailableForSnapshotsPtr = &newValue
	return o
}

// SizeTotal is a 'getter' method
func (o *VolumeSpaceAttributesType) SizeTotal() int {
	r := *o.SizeTotalPtr
	return r
}

// SetSizeTotal is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSizeTotal(newValue int) *VolumeSpaceAttributesType {
	o.SizeTotalPtr = &newValue
	return o
}

// SizeUsed is a 'getter' method
func (o *VolumeSpaceAttributesType) SizeUsed() int {
	r := *o.SizeUsedPtr
	return r
}

// SetSizeUsed is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSizeUsed(newValue int) *VolumeSpaceAttributesType {
	o.SizeUsedPtr = &newValue
	return o
}

// SizeUsedBySnapshots is a 'getter' method
func (o *VolumeSpaceAttributesType) SizeUsedBySnapshots() int {
	r := *o.SizeUsedBySnapshotsPtr
	return r
}

// SetSizeUsedBySnapshots is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSizeUsedBySnapshots(newValue int) *VolumeSpaceAttributesType {
	o.SizeUsedBySnapshotsPtr = &newValue
	return o
}

// SnapshotReserveAvailable is a 'getter' method
func (o *VolumeSpaceAttributesType) SnapshotReserveAvailable() int {
	r := *o.SnapshotReserveAvailablePtr
	return r
}

// SetSnapshotReserveAvailable is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSnapshotReserveAvailable(newValue int) *VolumeSpaceAttributesType {
	o.SnapshotReserveAvailablePtr = &newValue
	return o
}

// SnapshotReserveSize is a 'getter' method
func (o *VolumeSpaceAttributesType) SnapshotReserveSize() int {
	r := *o.SnapshotReserveSizePtr
	return r
}

// SetSnapshotReserveSize is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSnapshotReserveSize(newValue int) *VolumeSpaceAttributesType {
	o.SnapshotReserveSizePtr = &newValue
	return o
}

// SpaceFullThresholdPercent is a 'getter' method
func (o *VolumeSpaceAttributesType) SpaceFullThresholdPercent() int {
	r := *o.SpaceFullThresholdPercentPtr
	return r
}

// SetSpaceFullThresholdPercent is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSpaceFullThresholdPercent(newValue int) *VolumeSpaceAttributesType {
	o.SpaceFullThresholdPercentPtr = &newValue
	return o
}

// SpaceGuarantee is a 'getter' method
func (o *VolumeSpaceAttributesType) SpaceGuarantee() string {
	r := *o.SpaceGuaranteePtr
	return r
}

// SetSpaceGuarantee is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSpaceGuarantee(newValue string) *VolumeSpaceAttributesType {
	o.SpaceGuaranteePtr = &newValue
	return o
}

// SpaceMgmtOptionTryFirst is a 'getter' method
func (o *VolumeSpaceAttributesType) SpaceMgmtOptionTryFirst() string {
	r := *o.SpaceMgmtOptionTryFirstPtr
	return r
}

// SetSpaceMgmtOptionTryFirst is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSpaceMgmtOptionTryFirst(newValue string) *VolumeSpaceAttributesType {
	o.SpaceMgmtOptionTryFirstPtr = &newValue
	return o
}

// SpaceNearlyFullThresholdPercent is a 'getter' method
func (o *VolumeSpaceAttributesType) SpaceNearlyFullThresholdPercent() int {
	r := *o.SpaceNearlyFullThresholdPercentPtr
	return r
}

// SetSpaceNearlyFullThresholdPercent is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSpaceNearlyFullThresholdPercent(newValue int) *VolumeSpaceAttributesType {
	o.SpaceNearlyFullThresholdPercentPtr = &newValue
	return o
}

// SpaceSlo is a 'getter' method
func (o *VolumeSpaceAttributesType) SpaceSlo() SpaceSloEnumType {
	r := *o.SpaceSloPtr
	return r
}

// SetSpaceSlo is a fluent style 'setter' method that can be chained
func (o *VolumeSpaceAttributesType) SetSpaceSlo(newValue SpaceSloEnumType) *VolumeSpaceAttributesType {
	o.SpaceSloPtr = &newValue
	return o
}
