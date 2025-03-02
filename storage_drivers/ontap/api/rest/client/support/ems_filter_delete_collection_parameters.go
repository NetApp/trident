// Code generated by go-swagger; DO NOT EDIT.

package support

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"net/http"
	"time"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	cr "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// NewEmsFilterDeleteCollectionParams creates a new EmsFilterDeleteCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewEmsFilterDeleteCollectionParams() *EmsFilterDeleteCollectionParams {
	return &EmsFilterDeleteCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewEmsFilterDeleteCollectionParamsWithTimeout creates a new EmsFilterDeleteCollectionParams object
// with the ability to set a timeout on a request.
func NewEmsFilterDeleteCollectionParamsWithTimeout(timeout time.Duration) *EmsFilterDeleteCollectionParams {
	return &EmsFilterDeleteCollectionParams{
		timeout: timeout,
	}
}

// NewEmsFilterDeleteCollectionParamsWithContext creates a new EmsFilterDeleteCollectionParams object
// with the ability to set a context for a request.
func NewEmsFilterDeleteCollectionParamsWithContext(ctx context.Context) *EmsFilterDeleteCollectionParams {
	return &EmsFilterDeleteCollectionParams{
		Context: ctx,
	}
}

// NewEmsFilterDeleteCollectionParamsWithHTTPClient creates a new EmsFilterDeleteCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewEmsFilterDeleteCollectionParamsWithHTTPClient(client *http.Client) *EmsFilterDeleteCollectionParams {
	return &EmsFilterDeleteCollectionParams{
		HTTPClient: client,
	}
}

/*
EmsFilterDeleteCollectionParams contains all the parameters to send to the API endpoint

	for the ems filter delete collection operation.

	Typically these are written to a http.Request.
*/
type EmsFilterDeleteCollectionParams struct {

	/* AccessControlRoleName.

	   Filter by access_control_role.name
	*/
	AccessControlRoleName *string

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* Info.

	   Info specification
	*/
	Info EmsFilterDeleteCollectionBody

	/* Name.

	   Filter by name
	*/
	Name *string

	/* ReturnRecords.

	   The default is true for GET calls.  When set to false, only the number of records is returned.

	   Default: true
	*/
	ReturnRecords *bool

	/* ReturnTimeout.

	   The number of seconds to allow the call to execute before returning.  When iterating over a collection, the default is 15 seconds.  ONTAP returns earlier if either max records or the end of the collection is reached.

	   Default: 15
	*/
	ReturnTimeout *int64

	/* RulesIndex.

	   Filter by rules.index
	*/
	RulesIndex *int64

	/* RulesMessageCriteriaNamePattern.

	   Filter by rules.message_criteria.name_pattern
	*/
	RulesMessageCriteriaNamePattern *string

	/* RulesMessageCriteriaSeverities.

	   Filter by rules.message_criteria.severities
	*/
	RulesMessageCriteriaSeverities *string

	/* RulesMessageCriteriaSnmpTrapTypes.

	   Filter by rules.message_criteria.snmp_trap_types
	*/
	RulesMessageCriteriaSnmpTrapTypes *string

	/* RulesParameterCriteriaNamePattern.

	   Filter by rules.parameter_criteria.name_pattern
	*/
	RulesParameterCriteriaNamePattern *string

	/* RulesParameterCriteriaValuePattern.

	   Filter by rules.parameter_criteria.value_pattern
	*/
	RulesParameterCriteriaValuePattern *string

	/* RulesType.

	   Filter by rules.type
	*/
	RulesType *string

	/* SerialRecords.

	   Perform the operation on the records synchronously.
	*/
	SerialRecords *bool

	/* SystemDefined.

	   Filter by system_defined
	*/
	SystemDefined *bool

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the ems filter delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *EmsFilterDeleteCollectionParams) WithDefaults() *EmsFilterDeleteCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the ems filter delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *EmsFilterDeleteCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := EmsFilterDeleteCollectionParams{
		ContinueOnFailure: &continueOnFailureDefault,
		ReturnRecords:     &returnRecordsDefault,
		ReturnTimeout:     &returnTimeoutDefault,
		SerialRecords:     &serialRecordsDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) WithTimeout(timeout time.Duration) *EmsFilterDeleteCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) WithContext(ctx context.Context) *EmsFilterDeleteCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) WithHTTPClient(client *http.Client) *EmsFilterDeleteCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithAccessControlRoleName adds the accessControlRoleName to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) WithAccessControlRoleName(accessControlRoleName *string) *EmsFilterDeleteCollectionParams {
	o.SetAccessControlRoleName(accessControlRoleName)
	return o
}

// SetAccessControlRoleName adds the accessControlRoleName to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) SetAccessControlRoleName(accessControlRoleName *string) {
	o.AccessControlRoleName = accessControlRoleName
}

// WithContinueOnFailure adds the continueOnFailure to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *EmsFilterDeleteCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithInfo adds the info to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) WithInfo(info EmsFilterDeleteCollectionBody) *EmsFilterDeleteCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) SetInfo(info EmsFilterDeleteCollectionBody) {
	o.Info = info
}

// WithName adds the name to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) WithName(name *string) *EmsFilterDeleteCollectionParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) SetName(name *string) {
	o.Name = name
}

// WithReturnRecords adds the returnRecords to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) WithReturnRecords(returnRecords *bool) *EmsFilterDeleteCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) WithReturnTimeout(returnTimeout *int64) *EmsFilterDeleteCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithRulesIndex adds the rulesIndex to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) WithRulesIndex(rulesIndex *int64) *EmsFilterDeleteCollectionParams {
	o.SetRulesIndex(rulesIndex)
	return o
}

// SetRulesIndex adds the rulesIndex to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) SetRulesIndex(rulesIndex *int64) {
	o.RulesIndex = rulesIndex
}

// WithRulesMessageCriteriaNamePattern adds the rulesMessageCriteriaNamePattern to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) WithRulesMessageCriteriaNamePattern(rulesMessageCriteriaNamePattern *string) *EmsFilterDeleteCollectionParams {
	o.SetRulesMessageCriteriaNamePattern(rulesMessageCriteriaNamePattern)
	return o
}

// SetRulesMessageCriteriaNamePattern adds the rulesMessageCriteriaNamePattern to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) SetRulesMessageCriteriaNamePattern(rulesMessageCriteriaNamePattern *string) {
	o.RulesMessageCriteriaNamePattern = rulesMessageCriteriaNamePattern
}

// WithRulesMessageCriteriaSeverities adds the rulesMessageCriteriaSeverities to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) WithRulesMessageCriteriaSeverities(rulesMessageCriteriaSeverities *string) *EmsFilterDeleteCollectionParams {
	o.SetRulesMessageCriteriaSeverities(rulesMessageCriteriaSeverities)
	return o
}

// SetRulesMessageCriteriaSeverities adds the rulesMessageCriteriaSeverities to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) SetRulesMessageCriteriaSeverities(rulesMessageCriteriaSeverities *string) {
	o.RulesMessageCriteriaSeverities = rulesMessageCriteriaSeverities
}

// WithRulesMessageCriteriaSnmpTrapTypes adds the rulesMessageCriteriaSnmpTrapTypes to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) WithRulesMessageCriteriaSnmpTrapTypes(rulesMessageCriteriaSnmpTrapTypes *string) *EmsFilterDeleteCollectionParams {
	o.SetRulesMessageCriteriaSnmpTrapTypes(rulesMessageCriteriaSnmpTrapTypes)
	return o
}

// SetRulesMessageCriteriaSnmpTrapTypes adds the rulesMessageCriteriaSnmpTrapTypes to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) SetRulesMessageCriteriaSnmpTrapTypes(rulesMessageCriteriaSnmpTrapTypes *string) {
	o.RulesMessageCriteriaSnmpTrapTypes = rulesMessageCriteriaSnmpTrapTypes
}

// WithRulesParameterCriteriaNamePattern adds the rulesParameterCriteriaNamePattern to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) WithRulesParameterCriteriaNamePattern(rulesParameterCriteriaNamePattern *string) *EmsFilterDeleteCollectionParams {
	o.SetRulesParameterCriteriaNamePattern(rulesParameterCriteriaNamePattern)
	return o
}

// SetRulesParameterCriteriaNamePattern adds the rulesParameterCriteriaNamePattern to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) SetRulesParameterCriteriaNamePattern(rulesParameterCriteriaNamePattern *string) {
	o.RulesParameterCriteriaNamePattern = rulesParameterCriteriaNamePattern
}

// WithRulesParameterCriteriaValuePattern adds the rulesParameterCriteriaValuePattern to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) WithRulesParameterCriteriaValuePattern(rulesParameterCriteriaValuePattern *string) *EmsFilterDeleteCollectionParams {
	o.SetRulesParameterCriteriaValuePattern(rulesParameterCriteriaValuePattern)
	return o
}

// SetRulesParameterCriteriaValuePattern adds the rulesParameterCriteriaValuePattern to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) SetRulesParameterCriteriaValuePattern(rulesParameterCriteriaValuePattern *string) {
	o.RulesParameterCriteriaValuePattern = rulesParameterCriteriaValuePattern
}

// WithRulesType adds the rulesType to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) WithRulesType(rulesType *string) *EmsFilterDeleteCollectionParams {
	o.SetRulesType(rulesType)
	return o
}

// SetRulesType adds the rulesType to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) SetRulesType(rulesType *string) {
	o.RulesType = rulesType
}

// WithSerialRecords adds the serialRecords to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) WithSerialRecords(serialRecords *bool) *EmsFilterDeleteCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WithSystemDefined adds the systemDefined to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) WithSystemDefined(systemDefined *bool) *EmsFilterDeleteCollectionParams {
	o.SetSystemDefined(systemDefined)
	return o
}

// SetSystemDefined adds the systemDefined to the ems filter delete collection params
func (o *EmsFilterDeleteCollectionParams) SetSystemDefined(systemDefined *bool) {
	o.SystemDefined = systemDefined
}

// WriteToRequest writes these params to a swagger request
func (o *EmsFilterDeleteCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.AccessControlRoleName != nil {

		// query param access_control_role.name
		var qrAccessControlRoleName string

		if o.AccessControlRoleName != nil {
			qrAccessControlRoleName = *o.AccessControlRoleName
		}
		qAccessControlRoleName := qrAccessControlRoleName
		if qAccessControlRoleName != "" {

			if err := r.SetQueryParam("access_control_role.name", qAccessControlRoleName); err != nil {
				return err
			}
		}
	}

	if o.ContinueOnFailure != nil {

		// query param continue_on_failure
		var qrContinueOnFailure bool

		if o.ContinueOnFailure != nil {
			qrContinueOnFailure = *o.ContinueOnFailure
		}
		qContinueOnFailure := swag.FormatBool(qrContinueOnFailure)
		if qContinueOnFailure != "" {

			if err := r.SetQueryParam("continue_on_failure", qContinueOnFailure); err != nil {
				return err
			}
		}
	}
	if err := r.SetBodyParam(o.Info); err != nil {
		return err
	}

	if o.Name != nil {

		// query param name
		var qrName string

		if o.Name != nil {
			qrName = *o.Name
		}
		qName := qrName
		if qName != "" {

			if err := r.SetQueryParam("name", qName); err != nil {
				return err
			}
		}
	}

	if o.ReturnRecords != nil {

		// query param return_records
		var qrReturnRecords bool

		if o.ReturnRecords != nil {
			qrReturnRecords = *o.ReturnRecords
		}
		qReturnRecords := swag.FormatBool(qrReturnRecords)
		if qReturnRecords != "" {

			if err := r.SetQueryParam("return_records", qReturnRecords); err != nil {
				return err
			}
		}
	}

	if o.ReturnTimeout != nil {

		// query param return_timeout
		var qrReturnTimeout int64

		if o.ReturnTimeout != nil {
			qrReturnTimeout = *o.ReturnTimeout
		}
		qReturnTimeout := swag.FormatInt64(qrReturnTimeout)
		if qReturnTimeout != "" {

			if err := r.SetQueryParam("return_timeout", qReturnTimeout); err != nil {
				return err
			}
		}
	}

	if o.RulesIndex != nil {

		// query param rules.index
		var qrRulesIndex int64

		if o.RulesIndex != nil {
			qrRulesIndex = *o.RulesIndex
		}
		qRulesIndex := swag.FormatInt64(qrRulesIndex)
		if qRulesIndex != "" {

			if err := r.SetQueryParam("rules.index", qRulesIndex); err != nil {
				return err
			}
		}
	}

	if o.RulesMessageCriteriaNamePattern != nil {

		// query param rules.message_criteria.name_pattern
		var qrRulesMessageCriteriaNamePattern string

		if o.RulesMessageCriteriaNamePattern != nil {
			qrRulesMessageCriteriaNamePattern = *o.RulesMessageCriteriaNamePattern
		}
		qRulesMessageCriteriaNamePattern := qrRulesMessageCriteriaNamePattern
		if qRulesMessageCriteriaNamePattern != "" {

			if err := r.SetQueryParam("rules.message_criteria.name_pattern", qRulesMessageCriteriaNamePattern); err != nil {
				return err
			}
		}
	}

	if o.RulesMessageCriteriaSeverities != nil {

		// query param rules.message_criteria.severities
		var qrRulesMessageCriteriaSeverities string

		if o.RulesMessageCriteriaSeverities != nil {
			qrRulesMessageCriteriaSeverities = *o.RulesMessageCriteriaSeverities
		}
		qRulesMessageCriteriaSeverities := qrRulesMessageCriteriaSeverities
		if qRulesMessageCriteriaSeverities != "" {

			if err := r.SetQueryParam("rules.message_criteria.severities", qRulesMessageCriteriaSeverities); err != nil {
				return err
			}
		}
	}

	if o.RulesMessageCriteriaSnmpTrapTypes != nil {

		// query param rules.message_criteria.snmp_trap_types
		var qrRulesMessageCriteriaSnmpTrapTypes string

		if o.RulesMessageCriteriaSnmpTrapTypes != nil {
			qrRulesMessageCriteriaSnmpTrapTypes = *o.RulesMessageCriteriaSnmpTrapTypes
		}
		qRulesMessageCriteriaSnmpTrapTypes := qrRulesMessageCriteriaSnmpTrapTypes
		if qRulesMessageCriteriaSnmpTrapTypes != "" {

			if err := r.SetQueryParam("rules.message_criteria.snmp_trap_types", qRulesMessageCriteriaSnmpTrapTypes); err != nil {
				return err
			}
		}
	}

	if o.RulesParameterCriteriaNamePattern != nil {

		// query param rules.parameter_criteria.name_pattern
		var qrRulesParameterCriteriaNamePattern string

		if o.RulesParameterCriteriaNamePattern != nil {
			qrRulesParameterCriteriaNamePattern = *o.RulesParameterCriteriaNamePattern
		}
		qRulesParameterCriteriaNamePattern := qrRulesParameterCriteriaNamePattern
		if qRulesParameterCriteriaNamePattern != "" {

			if err := r.SetQueryParam("rules.parameter_criteria.name_pattern", qRulesParameterCriteriaNamePattern); err != nil {
				return err
			}
		}
	}

	if o.RulesParameterCriteriaValuePattern != nil {

		// query param rules.parameter_criteria.value_pattern
		var qrRulesParameterCriteriaValuePattern string

		if o.RulesParameterCriteriaValuePattern != nil {
			qrRulesParameterCriteriaValuePattern = *o.RulesParameterCriteriaValuePattern
		}
		qRulesParameterCriteriaValuePattern := qrRulesParameterCriteriaValuePattern
		if qRulesParameterCriteriaValuePattern != "" {

			if err := r.SetQueryParam("rules.parameter_criteria.value_pattern", qRulesParameterCriteriaValuePattern); err != nil {
				return err
			}
		}
	}

	if o.RulesType != nil {

		// query param rules.type
		var qrRulesType string

		if o.RulesType != nil {
			qrRulesType = *o.RulesType
		}
		qRulesType := qrRulesType
		if qRulesType != "" {

			if err := r.SetQueryParam("rules.type", qRulesType); err != nil {
				return err
			}
		}
	}

	if o.SerialRecords != nil {

		// query param serial_records
		var qrSerialRecords bool

		if o.SerialRecords != nil {
			qrSerialRecords = *o.SerialRecords
		}
		qSerialRecords := swag.FormatBool(qrSerialRecords)
		if qSerialRecords != "" {

			if err := r.SetQueryParam("serial_records", qSerialRecords); err != nil {
				return err
			}
		}
	}

	if o.SystemDefined != nil {

		// query param system_defined
		var qrSystemDefined bool

		if o.SystemDefined != nil {
			qrSystemDefined = *o.SystemDefined
		}
		qSystemDefined := swag.FormatBool(qrSystemDefined)
		if qSystemDefined != "" {

			if err := r.SetQueryParam("system_defined", qSystemDefined); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
