// Code generated by go-swagger; DO NOT EDIT.

package security

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

// NewAccountDeleteCollectionParams creates a new AccountDeleteCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewAccountDeleteCollectionParams() *AccountDeleteCollectionParams {
	return &AccountDeleteCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewAccountDeleteCollectionParamsWithTimeout creates a new AccountDeleteCollectionParams object
// with the ability to set a timeout on a request.
func NewAccountDeleteCollectionParamsWithTimeout(timeout time.Duration) *AccountDeleteCollectionParams {
	return &AccountDeleteCollectionParams{
		timeout: timeout,
	}
}

// NewAccountDeleteCollectionParamsWithContext creates a new AccountDeleteCollectionParams object
// with the ability to set a context for a request.
func NewAccountDeleteCollectionParamsWithContext(ctx context.Context) *AccountDeleteCollectionParams {
	return &AccountDeleteCollectionParams{
		Context: ctx,
	}
}

// NewAccountDeleteCollectionParamsWithHTTPClient creates a new AccountDeleteCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewAccountDeleteCollectionParamsWithHTTPClient(client *http.Client) *AccountDeleteCollectionParams {
	return &AccountDeleteCollectionParams{
		HTTPClient: client,
	}
}

/*
AccountDeleteCollectionParams contains all the parameters to send to the API endpoint

	for the account delete collection operation.

	Typically these are written to a http.Request.
*/
type AccountDeleteCollectionParams struct {

	/* ApplicationsApplication.

	   Filter by applications.application
	*/
	ApplicationsApplication *string

	/* ApplicationsAuthenticationMethods.

	   Filter by applications.authentication_methods
	*/
	ApplicationsAuthenticationMethods *string

	/* ApplicationsIsLdapFastbind.

	   Filter by applications.is_ldap_fastbind
	*/
	ApplicationsIsLdapFastbind *bool

	/* ApplicationsIsNsSwitchGroup.

	   Filter by applications.is_ns_switch_group
	*/
	ApplicationsIsNsSwitchGroup *bool

	/* ApplicationsSecondAuthenticationMethod.

	   Filter by applications.second_authentication_method
	*/
	ApplicationsSecondAuthenticationMethod *string

	/* Comment.

	   Filter by comment
	*/
	Comment *string

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* Info.

	   Info specification
	*/
	Info AccountDeleteCollectionBody

	/* Locked.

	   Filter by locked
	*/
	Locked *bool

	/* Name.

	   Filter by name
	*/
	Name *string

	/* OwnerName.

	   Filter by owner.name
	*/
	OwnerName *string

	/* OwnerUUID.

	   Filter by owner.uuid
	*/
	OwnerUUID *string

	/* PasswordHashAlgorithm.

	   Filter by password_hash_algorithm
	*/
	PasswordHashAlgorithm *string

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

	/* RoleName.

	   Filter by role.name
	*/
	RoleName *string

	/* Scope.

	   Filter by scope
	*/
	Scope *string

	/* SerialRecords.

	   Perform the operation on the records synchronously.
	*/
	SerialRecords *bool

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the account delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *AccountDeleteCollectionParams) WithDefaults() *AccountDeleteCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the account delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *AccountDeleteCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := AccountDeleteCollectionParams{
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

// WithTimeout adds the timeout to the account delete collection params
func (o *AccountDeleteCollectionParams) WithTimeout(timeout time.Duration) *AccountDeleteCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the account delete collection params
func (o *AccountDeleteCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the account delete collection params
func (o *AccountDeleteCollectionParams) WithContext(ctx context.Context) *AccountDeleteCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the account delete collection params
func (o *AccountDeleteCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the account delete collection params
func (o *AccountDeleteCollectionParams) WithHTTPClient(client *http.Client) *AccountDeleteCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the account delete collection params
func (o *AccountDeleteCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithApplicationsApplication adds the applicationsApplication to the account delete collection params
func (o *AccountDeleteCollectionParams) WithApplicationsApplication(applicationsApplication *string) *AccountDeleteCollectionParams {
	o.SetApplicationsApplication(applicationsApplication)
	return o
}

// SetApplicationsApplication adds the applicationsApplication to the account delete collection params
func (o *AccountDeleteCollectionParams) SetApplicationsApplication(applicationsApplication *string) {
	o.ApplicationsApplication = applicationsApplication
}

// WithApplicationsAuthenticationMethods adds the applicationsAuthenticationMethods to the account delete collection params
func (o *AccountDeleteCollectionParams) WithApplicationsAuthenticationMethods(applicationsAuthenticationMethods *string) *AccountDeleteCollectionParams {
	o.SetApplicationsAuthenticationMethods(applicationsAuthenticationMethods)
	return o
}

// SetApplicationsAuthenticationMethods adds the applicationsAuthenticationMethods to the account delete collection params
func (o *AccountDeleteCollectionParams) SetApplicationsAuthenticationMethods(applicationsAuthenticationMethods *string) {
	o.ApplicationsAuthenticationMethods = applicationsAuthenticationMethods
}

// WithApplicationsIsLdapFastbind adds the applicationsIsLdapFastbind to the account delete collection params
func (o *AccountDeleteCollectionParams) WithApplicationsIsLdapFastbind(applicationsIsLdapFastbind *bool) *AccountDeleteCollectionParams {
	o.SetApplicationsIsLdapFastbind(applicationsIsLdapFastbind)
	return o
}

// SetApplicationsIsLdapFastbind adds the applicationsIsLdapFastbind to the account delete collection params
func (o *AccountDeleteCollectionParams) SetApplicationsIsLdapFastbind(applicationsIsLdapFastbind *bool) {
	o.ApplicationsIsLdapFastbind = applicationsIsLdapFastbind
}

// WithApplicationsIsNsSwitchGroup adds the applicationsIsNsSwitchGroup to the account delete collection params
func (o *AccountDeleteCollectionParams) WithApplicationsIsNsSwitchGroup(applicationsIsNsSwitchGroup *bool) *AccountDeleteCollectionParams {
	o.SetApplicationsIsNsSwitchGroup(applicationsIsNsSwitchGroup)
	return o
}

// SetApplicationsIsNsSwitchGroup adds the applicationsIsNsSwitchGroup to the account delete collection params
func (o *AccountDeleteCollectionParams) SetApplicationsIsNsSwitchGroup(applicationsIsNsSwitchGroup *bool) {
	o.ApplicationsIsNsSwitchGroup = applicationsIsNsSwitchGroup
}

// WithApplicationsSecondAuthenticationMethod adds the applicationsSecondAuthenticationMethod to the account delete collection params
func (o *AccountDeleteCollectionParams) WithApplicationsSecondAuthenticationMethod(applicationsSecondAuthenticationMethod *string) *AccountDeleteCollectionParams {
	o.SetApplicationsSecondAuthenticationMethod(applicationsSecondAuthenticationMethod)
	return o
}

// SetApplicationsSecondAuthenticationMethod adds the applicationsSecondAuthenticationMethod to the account delete collection params
func (o *AccountDeleteCollectionParams) SetApplicationsSecondAuthenticationMethod(applicationsSecondAuthenticationMethod *string) {
	o.ApplicationsSecondAuthenticationMethod = applicationsSecondAuthenticationMethod
}

// WithComment adds the comment to the account delete collection params
func (o *AccountDeleteCollectionParams) WithComment(comment *string) *AccountDeleteCollectionParams {
	o.SetComment(comment)
	return o
}

// SetComment adds the comment to the account delete collection params
func (o *AccountDeleteCollectionParams) SetComment(comment *string) {
	o.Comment = comment
}

// WithContinueOnFailure adds the continueOnFailure to the account delete collection params
func (o *AccountDeleteCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *AccountDeleteCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the account delete collection params
func (o *AccountDeleteCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithInfo adds the info to the account delete collection params
func (o *AccountDeleteCollectionParams) WithInfo(info AccountDeleteCollectionBody) *AccountDeleteCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the account delete collection params
func (o *AccountDeleteCollectionParams) SetInfo(info AccountDeleteCollectionBody) {
	o.Info = info
}

// WithLocked adds the locked to the account delete collection params
func (o *AccountDeleteCollectionParams) WithLocked(locked *bool) *AccountDeleteCollectionParams {
	o.SetLocked(locked)
	return o
}

// SetLocked adds the locked to the account delete collection params
func (o *AccountDeleteCollectionParams) SetLocked(locked *bool) {
	o.Locked = locked
}

// WithName adds the name to the account delete collection params
func (o *AccountDeleteCollectionParams) WithName(name *string) *AccountDeleteCollectionParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the account delete collection params
func (o *AccountDeleteCollectionParams) SetName(name *string) {
	o.Name = name
}

// WithOwnerName adds the ownerName to the account delete collection params
func (o *AccountDeleteCollectionParams) WithOwnerName(ownerName *string) *AccountDeleteCollectionParams {
	o.SetOwnerName(ownerName)
	return o
}

// SetOwnerName adds the ownerName to the account delete collection params
func (o *AccountDeleteCollectionParams) SetOwnerName(ownerName *string) {
	o.OwnerName = ownerName
}

// WithOwnerUUID adds the ownerUUID to the account delete collection params
func (o *AccountDeleteCollectionParams) WithOwnerUUID(ownerUUID *string) *AccountDeleteCollectionParams {
	o.SetOwnerUUID(ownerUUID)
	return o
}

// SetOwnerUUID adds the ownerUuid to the account delete collection params
func (o *AccountDeleteCollectionParams) SetOwnerUUID(ownerUUID *string) {
	o.OwnerUUID = ownerUUID
}

// WithPasswordHashAlgorithm adds the passwordHashAlgorithm to the account delete collection params
func (o *AccountDeleteCollectionParams) WithPasswordHashAlgorithm(passwordHashAlgorithm *string) *AccountDeleteCollectionParams {
	o.SetPasswordHashAlgorithm(passwordHashAlgorithm)
	return o
}

// SetPasswordHashAlgorithm adds the passwordHashAlgorithm to the account delete collection params
func (o *AccountDeleteCollectionParams) SetPasswordHashAlgorithm(passwordHashAlgorithm *string) {
	o.PasswordHashAlgorithm = passwordHashAlgorithm
}

// WithReturnRecords adds the returnRecords to the account delete collection params
func (o *AccountDeleteCollectionParams) WithReturnRecords(returnRecords *bool) *AccountDeleteCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the account delete collection params
func (o *AccountDeleteCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the account delete collection params
func (o *AccountDeleteCollectionParams) WithReturnTimeout(returnTimeout *int64) *AccountDeleteCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the account delete collection params
func (o *AccountDeleteCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithRoleName adds the roleName to the account delete collection params
func (o *AccountDeleteCollectionParams) WithRoleName(roleName *string) *AccountDeleteCollectionParams {
	o.SetRoleName(roleName)
	return o
}

// SetRoleName adds the roleName to the account delete collection params
func (o *AccountDeleteCollectionParams) SetRoleName(roleName *string) {
	o.RoleName = roleName
}

// WithScope adds the scope to the account delete collection params
func (o *AccountDeleteCollectionParams) WithScope(scope *string) *AccountDeleteCollectionParams {
	o.SetScope(scope)
	return o
}

// SetScope adds the scope to the account delete collection params
func (o *AccountDeleteCollectionParams) SetScope(scope *string) {
	o.Scope = scope
}

// WithSerialRecords adds the serialRecords to the account delete collection params
func (o *AccountDeleteCollectionParams) WithSerialRecords(serialRecords *bool) *AccountDeleteCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the account delete collection params
func (o *AccountDeleteCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WriteToRequest writes these params to a swagger request
func (o *AccountDeleteCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.ApplicationsApplication != nil {

		// query param applications.application
		var qrApplicationsApplication string

		if o.ApplicationsApplication != nil {
			qrApplicationsApplication = *o.ApplicationsApplication
		}
		qApplicationsApplication := qrApplicationsApplication
		if qApplicationsApplication != "" {

			if err := r.SetQueryParam("applications.application", qApplicationsApplication); err != nil {
				return err
			}
		}
	}

	if o.ApplicationsAuthenticationMethods != nil {

		// query param applications.authentication_methods
		var qrApplicationsAuthenticationMethods string

		if o.ApplicationsAuthenticationMethods != nil {
			qrApplicationsAuthenticationMethods = *o.ApplicationsAuthenticationMethods
		}
		qApplicationsAuthenticationMethods := qrApplicationsAuthenticationMethods
		if qApplicationsAuthenticationMethods != "" {

			if err := r.SetQueryParam("applications.authentication_methods", qApplicationsAuthenticationMethods); err != nil {
				return err
			}
		}
	}

	if o.ApplicationsIsLdapFastbind != nil {

		// query param applications.is_ldap_fastbind
		var qrApplicationsIsLdapFastbind bool

		if o.ApplicationsIsLdapFastbind != nil {
			qrApplicationsIsLdapFastbind = *o.ApplicationsIsLdapFastbind
		}
		qApplicationsIsLdapFastbind := swag.FormatBool(qrApplicationsIsLdapFastbind)
		if qApplicationsIsLdapFastbind != "" {

			if err := r.SetQueryParam("applications.is_ldap_fastbind", qApplicationsIsLdapFastbind); err != nil {
				return err
			}
		}
	}

	if o.ApplicationsIsNsSwitchGroup != nil {

		// query param applications.is_ns_switch_group
		var qrApplicationsIsNsSwitchGroup bool

		if o.ApplicationsIsNsSwitchGroup != nil {
			qrApplicationsIsNsSwitchGroup = *o.ApplicationsIsNsSwitchGroup
		}
		qApplicationsIsNsSwitchGroup := swag.FormatBool(qrApplicationsIsNsSwitchGroup)
		if qApplicationsIsNsSwitchGroup != "" {

			if err := r.SetQueryParam("applications.is_ns_switch_group", qApplicationsIsNsSwitchGroup); err != nil {
				return err
			}
		}
	}

	if o.ApplicationsSecondAuthenticationMethod != nil {

		// query param applications.second_authentication_method
		var qrApplicationsSecondAuthenticationMethod string

		if o.ApplicationsSecondAuthenticationMethod != nil {
			qrApplicationsSecondAuthenticationMethod = *o.ApplicationsSecondAuthenticationMethod
		}
		qApplicationsSecondAuthenticationMethod := qrApplicationsSecondAuthenticationMethod
		if qApplicationsSecondAuthenticationMethod != "" {

			if err := r.SetQueryParam("applications.second_authentication_method", qApplicationsSecondAuthenticationMethod); err != nil {
				return err
			}
		}
	}

	if o.Comment != nil {

		// query param comment
		var qrComment string

		if o.Comment != nil {
			qrComment = *o.Comment
		}
		qComment := qrComment
		if qComment != "" {

			if err := r.SetQueryParam("comment", qComment); err != nil {
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

	if o.Locked != nil {

		// query param locked
		var qrLocked bool

		if o.Locked != nil {
			qrLocked = *o.Locked
		}
		qLocked := swag.FormatBool(qrLocked)
		if qLocked != "" {

			if err := r.SetQueryParam("locked", qLocked); err != nil {
				return err
			}
		}
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

	if o.OwnerName != nil {

		// query param owner.name
		var qrOwnerName string

		if o.OwnerName != nil {
			qrOwnerName = *o.OwnerName
		}
		qOwnerName := qrOwnerName
		if qOwnerName != "" {

			if err := r.SetQueryParam("owner.name", qOwnerName); err != nil {
				return err
			}
		}
	}

	if o.OwnerUUID != nil {

		// query param owner.uuid
		var qrOwnerUUID string

		if o.OwnerUUID != nil {
			qrOwnerUUID = *o.OwnerUUID
		}
		qOwnerUUID := qrOwnerUUID
		if qOwnerUUID != "" {

			if err := r.SetQueryParam("owner.uuid", qOwnerUUID); err != nil {
				return err
			}
		}
	}

	if o.PasswordHashAlgorithm != nil {

		// query param password_hash_algorithm
		var qrPasswordHashAlgorithm string

		if o.PasswordHashAlgorithm != nil {
			qrPasswordHashAlgorithm = *o.PasswordHashAlgorithm
		}
		qPasswordHashAlgorithm := qrPasswordHashAlgorithm
		if qPasswordHashAlgorithm != "" {

			if err := r.SetQueryParam("password_hash_algorithm", qPasswordHashAlgorithm); err != nil {
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

	if o.RoleName != nil {

		// query param role.name
		var qrRoleName string

		if o.RoleName != nil {
			qrRoleName = *o.RoleName
		}
		qRoleName := qrRoleName
		if qRoleName != "" {

			if err := r.SetQueryParam("role.name", qRoleName); err != nil {
				return err
			}
		}
	}

	if o.Scope != nil {

		// query param scope
		var qrScope string

		if o.Scope != nil {
			qrScope = *o.Scope
		}
		qScope := qrScope
		if qScope != "" {

			if err := r.SetQueryParam("scope", qScope); err != nil {
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

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}