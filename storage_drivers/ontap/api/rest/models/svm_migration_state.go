// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"encoding/json"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/validate"
)

// SvmMigrationState Indicates the state of the migration.
//
// swagger:model svm_migration_state
type SvmMigrationState string

func NewSvmMigrationState(value SvmMigrationState) *SvmMigrationState {
	return &value
}

// Pointer returns a pointer to a freshly-allocated SvmMigrationState.
func (m SvmMigrationState) Pointer() *SvmMigrationState {
	return &m
}

const (

	// SvmMigrationStatePrecheckStarted captures enum value "precheck_started"
	SvmMigrationStatePrecheckStarted SvmMigrationState = "precheck_started"

	// SvmMigrationStateTransferring captures enum value "transferring"
	SvmMigrationStateTransferring SvmMigrationState = "transferring"

	// SvmMigrationStateReadyForCutover captures enum value "ready_for_cutover"
	SvmMigrationStateReadyForCutover SvmMigrationState = "ready_for_cutover"

	// SvmMigrationStateCutoverTriggered captures enum value "cutover_triggered"
	SvmMigrationStateCutoverTriggered SvmMigrationState = "cutover_triggered"

	// SvmMigrationStateCutoverStarted captures enum value "cutover_started"
	SvmMigrationStateCutoverStarted SvmMigrationState = "cutover_started"

	// SvmMigrationStateCutoverComplete captures enum value "cutover_complete"
	SvmMigrationStateCutoverComplete SvmMigrationState = "cutover_complete"

	// SvmMigrationStateMigratePaused captures enum value "migrate_paused"
	SvmMigrationStateMigratePaused SvmMigrationState = "migrate_paused"

	// SvmMigrationStateMigrateComplete captures enum value "migrate_complete"
	SvmMigrationStateMigrateComplete SvmMigrationState = "migrate_complete"

	// SvmMigrationStateMigrateCompleteWithWarnings captures enum value "migrate_complete_with_warnings"
	SvmMigrationStateMigrateCompleteWithWarnings SvmMigrationState = "migrate_complete_with_warnings"

	// SvmMigrationStateMigrateFailed captures enum value "migrate_failed"
	SvmMigrationStateMigrateFailed SvmMigrationState = "migrate_failed"

	// SvmMigrationStateCleanup captures enum value "cleanup"
	SvmMigrationStateCleanup SvmMigrationState = "cleanup"

	// SvmMigrationStateCleanupFailed captures enum value "cleanup_failed"
	SvmMigrationStateCleanupFailed SvmMigrationState = "cleanup_failed"

	// SvmMigrationStateDestinationCleanedUp captures enum value "destination_cleaned_up"
	SvmMigrationStateDestinationCleanedUp SvmMigrationState = "destination_cleaned_up"

	// SvmMigrationStateMigratePausing captures enum value "migrate_pausing"
	SvmMigrationStateMigratePausing SvmMigrationState = "migrate_pausing"

	// SvmMigrationStatePaused captures enum value "paused"
	SvmMigrationStatePaused SvmMigrationState = "paused"

	// SvmMigrationStateCompleted captures enum value "completed"
	SvmMigrationStateCompleted SvmMigrationState = "completed"

	// SvmMigrationStateFailed captures enum value "failed"
	SvmMigrationStateFailed SvmMigrationState = "failed"

	// SvmMigrationStateSourceCleanup captures enum value "source_cleanup"
	SvmMigrationStateSourceCleanup SvmMigrationState = "source_cleanup"

	// SvmMigrationStatePostCutover captures enum value "post_cutover"
	SvmMigrationStatePostCutover SvmMigrationState = "post_cutover"

	// SvmMigrationStatePostCutoverFailed captures enum value "post_cutover_failed"
	SvmMigrationStatePostCutoverFailed SvmMigrationState = "post_cutover_failed"

	// SvmMigrationStateReadyForSourceCleanup captures enum value "ready_for_source_cleanup"
	SvmMigrationStateReadyForSourceCleanup SvmMigrationState = "ready_for_source_cleanup"

	// SvmMigrationStateSetupConfiguration captures enum value "setup_configuration"
	SvmMigrationStateSetupConfiguration SvmMigrationState = "setup_configuration"

	// SvmMigrationStateSetupConfigurationFailed captures enum value "setup_configuration_failed"
	SvmMigrationStateSetupConfigurationFailed SvmMigrationState = "setup_configuration_failed"

	// SvmMigrationStateMigratePauseFailed captures enum value "migrate_pause_failed"
	SvmMigrationStateMigratePauseFailed SvmMigrationState = "migrate_pause_failed"

	// SvmMigrationStateMigrateAborting captures enum value "migrate_aborting"
	SvmMigrationStateMigrateAborting SvmMigrationState = "migrate_aborting"

	// SvmMigrationStateMigrateAbortFailed captures enum value "migrate_abort_failed"
	SvmMigrationStateMigrateAbortFailed SvmMigrationState = "migrate_abort_failed"

	// SvmMigrationStateMigrateAborted captures enum value "migrate_aborted"
	SvmMigrationStateMigrateAborted SvmMigrationState = "migrate_aborted"
)

// for schema
var svmMigrationStateEnum []interface{}

func init() {
	var res []SvmMigrationState
	if err := json.Unmarshal([]byte(`["precheck_started","transferring","ready_for_cutover","cutover_triggered","cutover_started","cutover_complete","migrate_paused","migrate_complete","migrate_complete_with_warnings","migrate_failed","cleanup","cleanup_failed","destination_cleaned_up","migrate_pausing","paused","completed","failed","source_cleanup","post_cutover","post_cutover_failed","ready_for_source_cleanup","setup_configuration","setup_configuration_failed","migrate_pause_failed","migrate_aborting","migrate_abort_failed","migrate_aborted"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		svmMigrationStateEnum = append(svmMigrationStateEnum, v)
	}
}

func (m SvmMigrationState) validateSvmMigrationStateEnum(path, location string, value SvmMigrationState) error {
	if err := validate.EnumCase(path, location, value, svmMigrationStateEnum, true); err != nil {
		return err
	}
	return nil
}

// Validate validates this svm migration state
func (m SvmMigrationState) Validate(formats strfmt.Registry) error {
	var res []error

	// value enum
	if err := m.validateSvmMigrationStateEnum("", "body", m); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// ContextValidate validate this svm migration state based on the context it is used
func (m SvmMigrationState) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := validate.ReadOnly(ctx, "", "body", SvmMigrationState(m)); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
