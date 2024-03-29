// Code generated by go-swagger; DO NOT EDIT.

package name_services

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

// NewLdapCollectionGetParams creates a new LdapCollectionGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewLdapCollectionGetParams() *LdapCollectionGetParams {
	return &LdapCollectionGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewLdapCollectionGetParamsWithTimeout creates a new LdapCollectionGetParams object
// with the ability to set a timeout on a request.
func NewLdapCollectionGetParamsWithTimeout(timeout time.Duration) *LdapCollectionGetParams {
	return &LdapCollectionGetParams{
		timeout: timeout,
	}
}

// NewLdapCollectionGetParamsWithContext creates a new LdapCollectionGetParams object
// with the ability to set a context for a request.
func NewLdapCollectionGetParamsWithContext(ctx context.Context) *LdapCollectionGetParams {
	return &LdapCollectionGetParams{
		Context: ctx,
	}
}

// NewLdapCollectionGetParamsWithHTTPClient creates a new LdapCollectionGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewLdapCollectionGetParamsWithHTTPClient(client *http.Client) *LdapCollectionGetParams {
	return &LdapCollectionGetParams{
		HTTPClient: client,
	}
}

/*
LdapCollectionGetParams contains all the parameters to send to the API endpoint

	for the ldap collection get operation.

	Typically these are written to a http.Request.
*/
type LdapCollectionGetParams struct {

	/* AdDomain.

	   Filter by ad_domain
	*/
	AdDomain *string

	/* BaseDn.

	   Filter by base_dn
	*/
	BaseDn *string

	/* BaseScope.

	   Filter by base_scope
	*/
	BaseScope *string

	/* BindAsCifsServer.

	   Filter by bind_as_cifs_server
	*/
	BindAsCifsServer *bool

	/* BindDn.

	   Filter by bind_dn
	*/
	BindDn *string

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* GroupDn.

	   Filter by group_dn
	*/
	GroupDn *string

	/* GroupMembershipFilter.

	   Filter by group_membership_filter
	*/
	GroupMembershipFilter *string

	/* GroupScope.

	   Filter by group_scope
	*/
	GroupScope *string

	/* IsNetgroupByhostEnabled.

	   Filter by is_netgroup_byhost_enabled
	*/
	IsNetgroupByhostEnabled *bool

	/* IsOwner.

	   Filter by is_owner
	*/
	IsOwner *bool

	/* LdapsEnabled.

	   Filter by ldaps_enabled
	*/
	LdapsEnabled *bool

	/* MaxRecords.

	   Limit the number of records returned.
	*/
	MaxRecords *int64

	/* MinBindLevel.

	   Filter by min_bind_level
	*/
	MinBindLevel *string

	/* NetgroupByhostDn.

	   Filter by netgroup_byhost_dn
	*/
	NetgroupByhostDn *string

	/* NetgroupByhostScope.

	   Filter by netgroup_byhost_scope
	*/
	NetgroupByhostScope *string

	/* NetgroupDn.

	   Filter by netgroup_dn
	*/
	NetgroupDn *string

	/* NetgroupScope.

	   Filter by netgroup_scope
	*/
	NetgroupScope *string

	/* OrderBy.

	   Order results by specified fields and optional [asc|desc] direction. Default direction is 'asc' for ascending.
	*/
	OrderBy []string

	/* Port.

	   Filter by port
	*/
	Port *int64

	/* PreferredAdServers.

	   Filter by preferred_ad_servers
	*/
	PreferredAdServers *string

	/* QueryTimeout.

	   Filter by query_timeout
	*/
	QueryTimeout *int64

	/* ReferralEnabled.

	   Filter by referral_enabled
	*/
	ReferralEnabled *bool

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

	/* Schema.

	   Filter by schema
	*/
	Schema *string

	/* Servers.

	   Filter by servers
	*/
	Servers *string

	/* SessionSecurity.

	   Filter by session_security
	*/
	SessionSecurity *string

	/* StatusCode.

	   Filter by status.code
	*/
	StatusCode *int64

	/* StatusDnMessage.

	   Filter by status.dn_message
	*/
	StatusDnMessage *string

	/* StatusMessage.

	   Filter by status.message
	*/
	StatusMessage *string

	/* StatusState.

	   Filter by status.state
	*/
	StatusState *string

	/* SvmName.

	   Filter by svm.name
	*/
	SvmName *string

	/* SvmUUID.

	   Filter by svm.uuid
	*/
	SvmUUID *string

	/* TryChannelBinding.

	   Filter by try_channel_binding
	*/
	TryChannelBinding *bool

	/* UseStartTLS.

	   Filter by use_start_tls
	*/
	UseStartTLS *bool

	/* UserDn.

	   Filter by user_dn
	*/
	UserDn *string

	/* UserScope.

	   Filter by user_scope
	*/
	UserScope *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the ldap collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *LdapCollectionGetParams) WithDefaults() *LdapCollectionGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the ldap collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *LdapCollectionGetParams) SetDefaults() {
	var (
		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)
	)

	val := LdapCollectionGetParams{
		ReturnRecords: &returnRecordsDefault,
		ReturnTimeout: &returnTimeoutDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the ldap collection get params
func (o *LdapCollectionGetParams) WithTimeout(timeout time.Duration) *LdapCollectionGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the ldap collection get params
func (o *LdapCollectionGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the ldap collection get params
func (o *LdapCollectionGetParams) WithContext(ctx context.Context) *LdapCollectionGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the ldap collection get params
func (o *LdapCollectionGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the ldap collection get params
func (o *LdapCollectionGetParams) WithHTTPClient(client *http.Client) *LdapCollectionGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the ldap collection get params
func (o *LdapCollectionGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithAdDomain adds the adDomain to the ldap collection get params
func (o *LdapCollectionGetParams) WithAdDomain(adDomain *string) *LdapCollectionGetParams {
	o.SetAdDomain(adDomain)
	return o
}

// SetAdDomain adds the adDomain to the ldap collection get params
func (o *LdapCollectionGetParams) SetAdDomain(adDomain *string) {
	o.AdDomain = adDomain
}

// WithBaseDn adds the baseDn to the ldap collection get params
func (o *LdapCollectionGetParams) WithBaseDn(baseDn *string) *LdapCollectionGetParams {
	o.SetBaseDn(baseDn)
	return o
}

// SetBaseDn adds the baseDn to the ldap collection get params
func (o *LdapCollectionGetParams) SetBaseDn(baseDn *string) {
	o.BaseDn = baseDn
}

// WithBaseScope adds the baseScope to the ldap collection get params
func (o *LdapCollectionGetParams) WithBaseScope(baseScope *string) *LdapCollectionGetParams {
	o.SetBaseScope(baseScope)
	return o
}

// SetBaseScope adds the baseScope to the ldap collection get params
func (o *LdapCollectionGetParams) SetBaseScope(baseScope *string) {
	o.BaseScope = baseScope
}

// WithBindAsCifsServer adds the bindAsCifsServer to the ldap collection get params
func (o *LdapCollectionGetParams) WithBindAsCifsServer(bindAsCifsServer *bool) *LdapCollectionGetParams {
	o.SetBindAsCifsServer(bindAsCifsServer)
	return o
}

// SetBindAsCifsServer adds the bindAsCifsServer to the ldap collection get params
func (o *LdapCollectionGetParams) SetBindAsCifsServer(bindAsCifsServer *bool) {
	o.BindAsCifsServer = bindAsCifsServer
}

// WithBindDn adds the bindDn to the ldap collection get params
func (o *LdapCollectionGetParams) WithBindDn(bindDn *string) *LdapCollectionGetParams {
	o.SetBindDn(bindDn)
	return o
}

// SetBindDn adds the bindDn to the ldap collection get params
func (o *LdapCollectionGetParams) SetBindDn(bindDn *string) {
	o.BindDn = bindDn
}

// WithFields adds the fields to the ldap collection get params
func (o *LdapCollectionGetParams) WithFields(fields []string) *LdapCollectionGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the ldap collection get params
func (o *LdapCollectionGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithGroupDn adds the groupDn to the ldap collection get params
func (o *LdapCollectionGetParams) WithGroupDn(groupDn *string) *LdapCollectionGetParams {
	o.SetGroupDn(groupDn)
	return o
}

// SetGroupDn adds the groupDn to the ldap collection get params
func (o *LdapCollectionGetParams) SetGroupDn(groupDn *string) {
	o.GroupDn = groupDn
}

// WithGroupMembershipFilter adds the groupMembershipFilter to the ldap collection get params
func (o *LdapCollectionGetParams) WithGroupMembershipFilter(groupMembershipFilter *string) *LdapCollectionGetParams {
	o.SetGroupMembershipFilter(groupMembershipFilter)
	return o
}

// SetGroupMembershipFilter adds the groupMembershipFilter to the ldap collection get params
func (o *LdapCollectionGetParams) SetGroupMembershipFilter(groupMembershipFilter *string) {
	o.GroupMembershipFilter = groupMembershipFilter
}

// WithGroupScope adds the groupScope to the ldap collection get params
func (o *LdapCollectionGetParams) WithGroupScope(groupScope *string) *LdapCollectionGetParams {
	o.SetGroupScope(groupScope)
	return o
}

// SetGroupScope adds the groupScope to the ldap collection get params
func (o *LdapCollectionGetParams) SetGroupScope(groupScope *string) {
	o.GroupScope = groupScope
}

// WithIsNetgroupByhostEnabled adds the isNetgroupByhostEnabled to the ldap collection get params
func (o *LdapCollectionGetParams) WithIsNetgroupByhostEnabled(isNetgroupByhostEnabled *bool) *LdapCollectionGetParams {
	o.SetIsNetgroupByhostEnabled(isNetgroupByhostEnabled)
	return o
}

// SetIsNetgroupByhostEnabled adds the isNetgroupByhostEnabled to the ldap collection get params
func (o *LdapCollectionGetParams) SetIsNetgroupByhostEnabled(isNetgroupByhostEnabled *bool) {
	o.IsNetgroupByhostEnabled = isNetgroupByhostEnabled
}

// WithIsOwner adds the isOwner to the ldap collection get params
func (o *LdapCollectionGetParams) WithIsOwner(isOwner *bool) *LdapCollectionGetParams {
	o.SetIsOwner(isOwner)
	return o
}

// SetIsOwner adds the isOwner to the ldap collection get params
func (o *LdapCollectionGetParams) SetIsOwner(isOwner *bool) {
	o.IsOwner = isOwner
}

// WithLdapsEnabled adds the ldapsEnabled to the ldap collection get params
func (o *LdapCollectionGetParams) WithLdapsEnabled(ldapsEnabled *bool) *LdapCollectionGetParams {
	o.SetLdapsEnabled(ldapsEnabled)
	return o
}

// SetLdapsEnabled adds the ldapsEnabled to the ldap collection get params
func (o *LdapCollectionGetParams) SetLdapsEnabled(ldapsEnabled *bool) {
	o.LdapsEnabled = ldapsEnabled
}

// WithMaxRecords adds the maxRecords to the ldap collection get params
func (o *LdapCollectionGetParams) WithMaxRecords(maxRecords *int64) *LdapCollectionGetParams {
	o.SetMaxRecords(maxRecords)
	return o
}

// SetMaxRecords adds the maxRecords to the ldap collection get params
func (o *LdapCollectionGetParams) SetMaxRecords(maxRecords *int64) {
	o.MaxRecords = maxRecords
}

// WithMinBindLevel adds the minBindLevel to the ldap collection get params
func (o *LdapCollectionGetParams) WithMinBindLevel(minBindLevel *string) *LdapCollectionGetParams {
	o.SetMinBindLevel(minBindLevel)
	return o
}

// SetMinBindLevel adds the minBindLevel to the ldap collection get params
func (o *LdapCollectionGetParams) SetMinBindLevel(minBindLevel *string) {
	o.MinBindLevel = minBindLevel
}

// WithNetgroupByhostDn adds the netgroupByhostDn to the ldap collection get params
func (o *LdapCollectionGetParams) WithNetgroupByhostDn(netgroupByhostDn *string) *LdapCollectionGetParams {
	o.SetNetgroupByhostDn(netgroupByhostDn)
	return o
}

// SetNetgroupByhostDn adds the netgroupByhostDn to the ldap collection get params
func (o *LdapCollectionGetParams) SetNetgroupByhostDn(netgroupByhostDn *string) {
	o.NetgroupByhostDn = netgroupByhostDn
}

// WithNetgroupByhostScope adds the netgroupByhostScope to the ldap collection get params
func (o *LdapCollectionGetParams) WithNetgroupByhostScope(netgroupByhostScope *string) *LdapCollectionGetParams {
	o.SetNetgroupByhostScope(netgroupByhostScope)
	return o
}

// SetNetgroupByhostScope adds the netgroupByhostScope to the ldap collection get params
func (o *LdapCollectionGetParams) SetNetgroupByhostScope(netgroupByhostScope *string) {
	o.NetgroupByhostScope = netgroupByhostScope
}

// WithNetgroupDn adds the netgroupDn to the ldap collection get params
func (o *LdapCollectionGetParams) WithNetgroupDn(netgroupDn *string) *LdapCollectionGetParams {
	o.SetNetgroupDn(netgroupDn)
	return o
}

// SetNetgroupDn adds the netgroupDn to the ldap collection get params
func (o *LdapCollectionGetParams) SetNetgroupDn(netgroupDn *string) {
	o.NetgroupDn = netgroupDn
}

// WithNetgroupScope adds the netgroupScope to the ldap collection get params
func (o *LdapCollectionGetParams) WithNetgroupScope(netgroupScope *string) *LdapCollectionGetParams {
	o.SetNetgroupScope(netgroupScope)
	return o
}

// SetNetgroupScope adds the netgroupScope to the ldap collection get params
func (o *LdapCollectionGetParams) SetNetgroupScope(netgroupScope *string) {
	o.NetgroupScope = netgroupScope
}

// WithOrderBy adds the orderBy to the ldap collection get params
func (o *LdapCollectionGetParams) WithOrderBy(orderBy []string) *LdapCollectionGetParams {
	o.SetOrderBy(orderBy)
	return o
}

// SetOrderBy adds the orderBy to the ldap collection get params
func (o *LdapCollectionGetParams) SetOrderBy(orderBy []string) {
	o.OrderBy = orderBy
}

// WithPort adds the port to the ldap collection get params
func (o *LdapCollectionGetParams) WithPort(port *int64) *LdapCollectionGetParams {
	o.SetPort(port)
	return o
}

// SetPort adds the port to the ldap collection get params
func (o *LdapCollectionGetParams) SetPort(port *int64) {
	o.Port = port
}

// WithPreferredAdServers adds the preferredAdServers to the ldap collection get params
func (o *LdapCollectionGetParams) WithPreferredAdServers(preferredAdServers *string) *LdapCollectionGetParams {
	o.SetPreferredAdServers(preferredAdServers)
	return o
}

// SetPreferredAdServers adds the preferredAdServers to the ldap collection get params
func (o *LdapCollectionGetParams) SetPreferredAdServers(preferredAdServers *string) {
	o.PreferredAdServers = preferredAdServers
}

// WithQueryTimeout adds the queryTimeout to the ldap collection get params
func (o *LdapCollectionGetParams) WithQueryTimeout(queryTimeout *int64) *LdapCollectionGetParams {
	o.SetQueryTimeout(queryTimeout)
	return o
}

// SetQueryTimeout adds the queryTimeout to the ldap collection get params
func (o *LdapCollectionGetParams) SetQueryTimeout(queryTimeout *int64) {
	o.QueryTimeout = queryTimeout
}

// WithReferralEnabled adds the referralEnabled to the ldap collection get params
func (o *LdapCollectionGetParams) WithReferralEnabled(referralEnabled *bool) *LdapCollectionGetParams {
	o.SetReferralEnabled(referralEnabled)
	return o
}

// SetReferralEnabled adds the referralEnabled to the ldap collection get params
func (o *LdapCollectionGetParams) SetReferralEnabled(referralEnabled *bool) {
	o.ReferralEnabled = referralEnabled
}

// WithReturnRecords adds the returnRecords to the ldap collection get params
func (o *LdapCollectionGetParams) WithReturnRecords(returnRecords *bool) *LdapCollectionGetParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the ldap collection get params
func (o *LdapCollectionGetParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the ldap collection get params
func (o *LdapCollectionGetParams) WithReturnTimeout(returnTimeout *int64) *LdapCollectionGetParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the ldap collection get params
func (o *LdapCollectionGetParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithSchema adds the schema to the ldap collection get params
func (o *LdapCollectionGetParams) WithSchema(schema *string) *LdapCollectionGetParams {
	o.SetSchema(schema)
	return o
}

// SetSchema adds the schema to the ldap collection get params
func (o *LdapCollectionGetParams) SetSchema(schema *string) {
	o.Schema = schema
}

// WithServers adds the servers to the ldap collection get params
func (o *LdapCollectionGetParams) WithServers(servers *string) *LdapCollectionGetParams {
	o.SetServers(servers)
	return o
}

// SetServers adds the servers to the ldap collection get params
func (o *LdapCollectionGetParams) SetServers(servers *string) {
	o.Servers = servers
}

// WithSessionSecurity adds the sessionSecurity to the ldap collection get params
func (o *LdapCollectionGetParams) WithSessionSecurity(sessionSecurity *string) *LdapCollectionGetParams {
	o.SetSessionSecurity(sessionSecurity)
	return o
}

// SetSessionSecurity adds the sessionSecurity to the ldap collection get params
func (o *LdapCollectionGetParams) SetSessionSecurity(sessionSecurity *string) {
	o.SessionSecurity = sessionSecurity
}

// WithStatusCode adds the statusCode to the ldap collection get params
func (o *LdapCollectionGetParams) WithStatusCode(statusCode *int64) *LdapCollectionGetParams {
	o.SetStatusCode(statusCode)
	return o
}

// SetStatusCode adds the statusCode to the ldap collection get params
func (o *LdapCollectionGetParams) SetStatusCode(statusCode *int64) {
	o.StatusCode = statusCode
}

// WithStatusDnMessage adds the statusDnMessage to the ldap collection get params
func (o *LdapCollectionGetParams) WithStatusDnMessage(statusDnMessage *string) *LdapCollectionGetParams {
	o.SetStatusDnMessage(statusDnMessage)
	return o
}

// SetStatusDnMessage adds the statusDnMessage to the ldap collection get params
func (o *LdapCollectionGetParams) SetStatusDnMessage(statusDnMessage *string) {
	o.StatusDnMessage = statusDnMessage
}

// WithStatusMessage adds the statusMessage to the ldap collection get params
func (o *LdapCollectionGetParams) WithStatusMessage(statusMessage *string) *LdapCollectionGetParams {
	o.SetStatusMessage(statusMessage)
	return o
}

// SetStatusMessage adds the statusMessage to the ldap collection get params
func (o *LdapCollectionGetParams) SetStatusMessage(statusMessage *string) {
	o.StatusMessage = statusMessage
}

// WithStatusState adds the statusState to the ldap collection get params
func (o *LdapCollectionGetParams) WithStatusState(statusState *string) *LdapCollectionGetParams {
	o.SetStatusState(statusState)
	return o
}

// SetStatusState adds the statusState to the ldap collection get params
func (o *LdapCollectionGetParams) SetStatusState(statusState *string) {
	o.StatusState = statusState
}

// WithSvmName adds the svmName to the ldap collection get params
func (o *LdapCollectionGetParams) WithSvmName(svmName *string) *LdapCollectionGetParams {
	o.SetSvmName(svmName)
	return o
}

// SetSvmName adds the svmName to the ldap collection get params
func (o *LdapCollectionGetParams) SetSvmName(svmName *string) {
	o.SvmName = svmName
}

// WithSvmUUID adds the svmUUID to the ldap collection get params
func (o *LdapCollectionGetParams) WithSvmUUID(svmUUID *string) *LdapCollectionGetParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the ldap collection get params
func (o *LdapCollectionGetParams) SetSvmUUID(svmUUID *string) {
	o.SvmUUID = svmUUID
}

// WithTryChannelBinding adds the tryChannelBinding to the ldap collection get params
func (o *LdapCollectionGetParams) WithTryChannelBinding(tryChannelBinding *bool) *LdapCollectionGetParams {
	o.SetTryChannelBinding(tryChannelBinding)
	return o
}

// SetTryChannelBinding adds the tryChannelBinding to the ldap collection get params
func (o *LdapCollectionGetParams) SetTryChannelBinding(tryChannelBinding *bool) {
	o.TryChannelBinding = tryChannelBinding
}

// WithUseStartTLS adds the useStartTLS to the ldap collection get params
func (o *LdapCollectionGetParams) WithUseStartTLS(useStartTLS *bool) *LdapCollectionGetParams {
	o.SetUseStartTLS(useStartTLS)
	return o
}

// SetUseStartTLS adds the useStartTls to the ldap collection get params
func (o *LdapCollectionGetParams) SetUseStartTLS(useStartTLS *bool) {
	o.UseStartTLS = useStartTLS
}

// WithUserDn adds the userDn to the ldap collection get params
func (o *LdapCollectionGetParams) WithUserDn(userDn *string) *LdapCollectionGetParams {
	o.SetUserDn(userDn)
	return o
}

// SetUserDn adds the userDn to the ldap collection get params
func (o *LdapCollectionGetParams) SetUserDn(userDn *string) {
	o.UserDn = userDn
}

// WithUserScope adds the userScope to the ldap collection get params
func (o *LdapCollectionGetParams) WithUserScope(userScope *string) *LdapCollectionGetParams {
	o.SetUserScope(userScope)
	return o
}

// SetUserScope adds the userScope to the ldap collection get params
func (o *LdapCollectionGetParams) SetUserScope(userScope *string) {
	o.UserScope = userScope
}

// WriteToRequest writes these params to a swagger request
func (o *LdapCollectionGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.AdDomain != nil {

		// query param ad_domain
		var qrAdDomain string

		if o.AdDomain != nil {
			qrAdDomain = *o.AdDomain
		}
		qAdDomain := qrAdDomain
		if qAdDomain != "" {

			if err := r.SetQueryParam("ad_domain", qAdDomain); err != nil {
				return err
			}
		}
	}

	if o.BaseDn != nil {

		// query param base_dn
		var qrBaseDn string

		if o.BaseDn != nil {
			qrBaseDn = *o.BaseDn
		}
		qBaseDn := qrBaseDn
		if qBaseDn != "" {

			if err := r.SetQueryParam("base_dn", qBaseDn); err != nil {
				return err
			}
		}
	}

	if o.BaseScope != nil {

		// query param base_scope
		var qrBaseScope string

		if o.BaseScope != nil {
			qrBaseScope = *o.BaseScope
		}
		qBaseScope := qrBaseScope
		if qBaseScope != "" {

			if err := r.SetQueryParam("base_scope", qBaseScope); err != nil {
				return err
			}
		}
	}

	if o.BindAsCifsServer != nil {

		// query param bind_as_cifs_server
		var qrBindAsCifsServer bool

		if o.BindAsCifsServer != nil {
			qrBindAsCifsServer = *o.BindAsCifsServer
		}
		qBindAsCifsServer := swag.FormatBool(qrBindAsCifsServer)
		if qBindAsCifsServer != "" {

			if err := r.SetQueryParam("bind_as_cifs_server", qBindAsCifsServer); err != nil {
				return err
			}
		}
	}

	if o.BindDn != nil {

		// query param bind_dn
		var qrBindDn string

		if o.BindDn != nil {
			qrBindDn = *o.BindDn
		}
		qBindDn := qrBindDn
		if qBindDn != "" {

			if err := r.SetQueryParam("bind_dn", qBindDn); err != nil {
				return err
			}
		}
	}

	if o.Fields != nil {

		// binding items for fields
		joinedFields := o.bindParamFields(reg)

		// query array param fields
		if err := r.SetQueryParam("fields", joinedFields...); err != nil {
			return err
		}
	}

	if o.GroupDn != nil {

		// query param group_dn
		var qrGroupDn string

		if o.GroupDn != nil {
			qrGroupDn = *o.GroupDn
		}
		qGroupDn := qrGroupDn
		if qGroupDn != "" {

			if err := r.SetQueryParam("group_dn", qGroupDn); err != nil {
				return err
			}
		}
	}

	if o.GroupMembershipFilter != nil {

		// query param group_membership_filter
		var qrGroupMembershipFilter string

		if o.GroupMembershipFilter != nil {
			qrGroupMembershipFilter = *o.GroupMembershipFilter
		}
		qGroupMembershipFilter := qrGroupMembershipFilter
		if qGroupMembershipFilter != "" {

			if err := r.SetQueryParam("group_membership_filter", qGroupMembershipFilter); err != nil {
				return err
			}
		}
	}

	if o.GroupScope != nil {

		// query param group_scope
		var qrGroupScope string

		if o.GroupScope != nil {
			qrGroupScope = *o.GroupScope
		}
		qGroupScope := qrGroupScope
		if qGroupScope != "" {

			if err := r.SetQueryParam("group_scope", qGroupScope); err != nil {
				return err
			}
		}
	}

	if o.IsNetgroupByhostEnabled != nil {

		// query param is_netgroup_byhost_enabled
		var qrIsNetgroupByhostEnabled bool

		if o.IsNetgroupByhostEnabled != nil {
			qrIsNetgroupByhostEnabled = *o.IsNetgroupByhostEnabled
		}
		qIsNetgroupByhostEnabled := swag.FormatBool(qrIsNetgroupByhostEnabled)
		if qIsNetgroupByhostEnabled != "" {

			if err := r.SetQueryParam("is_netgroup_byhost_enabled", qIsNetgroupByhostEnabled); err != nil {
				return err
			}
		}
	}

	if o.IsOwner != nil {

		// query param is_owner
		var qrIsOwner bool

		if o.IsOwner != nil {
			qrIsOwner = *o.IsOwner
		}
		qIsOwner := swag.FormatBool(qrIsOwner)
		if qIsOwner != "" {

			if err := r.SetQueryParam("is_owner", qIsOwner); err != nil {
				return err
			}
		}
	}

	if o.LdapsEnabled != nil {

		// query param ldaps_enabled
		var qrLdapsEnabled bool

		if o.LdapsEnabled != nil {
			qrLdapsEnabled = *o.LdapsEnabled
		}
		qLdapsEnabled := swag.FormatBool(qrLdapsEnabled)
		if qLdapsEnabled != "" {

			if err := r.SetQueryParam("ldaps_enabled", qLdapsEnabled); err != nil {
				return err
			}
		}
	}

	if o.MaxRecords != nil {

		// query param max_records
		var qrMaxRecords int64

		if o.MaxRecords != nil {
			qrMaxRecords = *o.MaxRecords
		}
		qMaxRecords := swag.FormatInt64(qrMaxRecords)
		if qMaxRecords != "" {

			if err := r.SetQueryParam("max_records", qMaxRecords); err != nil {
				return err
			}
		}
	}

	if o.MinBindLevel != nil {

		// query param min_bind_level
		var qrMinBindLevel string

		if o.MinBindLevel != nil {
			qrMinBindLevel = *o.MinBindLevel
		}
		qMinBindLevel := qrMinBindLevel
		if qMinBindLevel != "" {

			if err := r.SetQueryParam("min_bind_level", qMinBindLevel); err != nil {
				return err
			}
		}
	}

	if o.NetgroupByhostDn != nil {

		// query param netgroup_byhost_dn
		var qrNetgroupByhostDn string

		if o.NetgroupByhostDn != nil {
			qrNetgroupByhostDn = *o.NetgroupByhostDn
		}
		qNetgroupByhostDn := qrNetgroupByhostDn
		if qNetgroupByhostDn != "" {

			if err := r.SetQueryParam("netgroup_byhost_dn", qNetgroupByhostDn); err != nil {
				return err
			}
		}
	}

	if o.NetgroupByhostScope != nil {

		// query param netgroup_byhost_scope
		var qrNetgroupByhostScope string

		if o.NetgroupByhostScope != nil {
			qrNetgroupByhostScope = *o.NetgroupByhostScope
		}
		qNetgroupByhostScope := qrNetgroupByhostScope
		if qNetgroupByhostScope != "" {

			if err := r.SetQueryParam("netgroup_byhost_scope", qNetgroupByhostScope); err != nil {
				return err
			}
		}
	}

	if o.NetgroupDn != nil {

		// query param netgroup_dn
		var qrNetgroupDn string

		if o.NetgroupDn != nil {
			qrNetgroupDn = *o.NetgroupDn
		}
		qNetgroupDn := qrNetgroupDn
		if qNetgroupDn != "" {

			if err := r.SetQueryParam("netgroup_dn", qNetgroupDn); err != nil {
				return err
			}
		}
	}

	if o.NetgroupScope != nil {

		// query param netgroup_scope
		var qrNetgroupScope string

		if o.NetgroupScope != nil {
			qrNetgroupScope = *o.NetgroupScope
		}
		qNetgroupScope := qrNetgroupScope
		if qNetgroupScope != "" {

			if err := r.SetQueryParam("netgroup_scope", qNetgroupScope); err != nil {
				return err
			}
		}
	}

	if o.OrderBy != nil {

		// binding items for order_by
		joinedOrderBy := o.bindParamOrderBy(reg)

		// query array param order_by
		if err := r.SetQueryParam("order_by", joinedOrderBy...); err != nil {
			return err
		}
	}

	if o.Port != nil {

		// query param port
		var qrPort int64

		if o.Port != nil {
			qrPort = *o.Port
		}
		qPort := swag.FormatInt64(qrPort)
		if qPort != "" {

			if err := r.SetQueryParam("port", qPort); err != nil {
				return err
			}
		}
	}

	if o.PreferredAdServers != nil {

		// query param preferred_ad_servers
		var qrPreferredAdServers string

		if o.PreferredAdServers != nil {
			qrPreferredAdServers = *o.PreferredAdServers
		}
		qPreferredAdServers := qrPreferredAdServers
		if qPreferredAdServers != "" {

			if err := r.SetQueryParam("preferred_ad_servers", qPreferredAdServers); err != nil {
				return err
			}
		}
	}

	if o.QueryTimeout != nil {

		// query param query_timeout
		var qrQueryTimeout int64

		if o.QueryTimeout != nil {
			qrQueryTimeout = *o.QueryTimeout
		}
		qQueryTimeout := swag.FormatInt64(qrQueryTimeout)
		if qQueryTimeout != "" {

			if err := r.SetQueryParam("query_timeout", qQueryTimeout); err != nil {
				return err
			}
		}
	}

	if o.ReferralEnabled != nil {

		// query param referral_enabled
		var qrReferralEnabled bool

		if o.ReferralEnabled != nil {
			qrReferralEnabled = *o.ReferralEnabled
		}
		qReferralEnabled := swag.FormatBool(qrReferralEnabled)
		if qReferralEnabled != "" {

			if err := r.SetQueryParam("referral_enabled", qReferralEnabled); err != nil {
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

	if o.Schema != nil {

		// query param schema
		var qrSchema string

		if o.Schema != nil {
			qrSchema = *o.Schema
		}
		qSchema := qrSchema
		if qSchema != "" {

			if err := r.SetQueryParam("schema", qSchema); err != nil {
				return err
			}
		}
	}

	if o.Servers != nil {

		// query param servers
		var qrServers string

		if o.Servers != nil {
			qrServers = *o.Servers
		}
		qServers := qrServers
		if qServers != "" {

			if err := r.SetQueryParam("servers", qServers); err != nil {
				return err
			}
		}
	}

	if o.SessionSecurity != nil {

		// query param session_security
		var qrSessionSecurity string

		if o.SessionSecurity != nil {
			qrSessionSecurity = *o.SessionSecurity
		}
		qSessionSecurity := qrSessionSecurity
		if qSessionSecurity != "" {

			if err := r.SetQueryParam("session_security", qSessionSecurity); err != nil {
				return err
			}
		}
	}

	if o.StatusCode != nil {

		// query param status.code
		var qrStatusCode int64

		if o.StatusCode != nil {
			qrStatusCode = *o.StatusCode
		}
		qStatusCode := swag.FormatInt64(qrStatusCode)
		if qStatusCode != "" {

			if err := r.SetQueryParam("status.code", qStatusCode); err != nil {
				return err
			}
		}
	}

	if o.StatusDnMessage != nil {

		// query param status.dn_message
		var qrStatusDnMessage string

		if o.StatusDnMessage != nil {
			qrStatusDnMessage = *o.StatusDnMessage
		}
		qStatusDnMessage := qrStatusDnMessage
		if qStatusDnMessage != "" {

			if err := r.SetQueryParam("status.dn_message", qStatusDnMessage); err != nil {
				return err
			}
		}
	}

	if o.StatusMessage != nil {

		// query param status.message
		var qrStatusMessage string

		if o.StatusMessage != nil {
			qrStatusMessage = *o.StatusMessage
		}
		qStatusMessage := qrStatusMessage
		if qStatusMessage != "" {

			if err := r.SetQueryParam("status.message", qStatusMessage); err != nil {
				return err
			}
		}
	}

	if o.StatusState != nil {

		// query param status.state
		var qrStatusState string

		if o.StatusState != nil {
			qrStatusState = *o.StatusState
		}
		qStatusState := qrStatusState
		if qStatusState != "" {

			if err := r.SetQueryParam("status.state", qStatusState); err != nil {
				return err
			}
		}
	}

	if o.SvmName != nil {

		// query param svm.name
		var qrSvmName string

		if o.SvmName != nil {
			qrSvmName = *o.SvmName
		}
		qSvmName := qrSvmName
		if qSvmName != "" {

			if err := r.SetQueryParam("svm.name", qSvmName); err != nil {
				return err
			}
		}
	}

	if o.SvmUUID != nil {

		// query param svm.uuid
		var qrSvmUUID string

		if o.SvmUUID != nil {
			qrSvmUUID = *o.SvmUUID
		}
		qSvmUUID := qrSvmUUID
		if qSvmUUID != "" {

			if err := r.SetQueryParam("svm.uuid", qSvmUUID); err != nil {
				return err
			}
		}
	}

	if o.TryChannelBinding != nil {

		// query param try_channel_binding
		var qrTryChannelBinding bool

		if o.TryChannelBinding != nil {
			qrTryChannelBinding = *o.TryChannelBinding
		}
		qTryChannelBinding := swag.FormatBool(qrTryChannelBinding)
		if qTryChannelBinding != "" {

			if err := r.SetQueryParam("try_channel_binding", qTryChannelBinding); err != nil {
				return err
			}
		}
	}

	if o.UseStartTLS != nil {

		// query param use_start_tls
		var qrUseStartTLS bool

		if o.UseStartTLS != nil {
			qrUseStartTLS = *o.UseStartTLS
		}
		qUseStartTLS := swag.FormatBool(qrUseStartTLS)
		if qUseStartTLS != "" {

			if err := r.SetQueryParam("use_start_tls", qUseStartTLS); err != nil {
				return err
			}
		}
	}

	if o.UserDn != nil {

		// query param user_dn
		var qrUserDn string

		if o.UserDn != nil {
			qrUserDn = *o.UserDn
		}
		qUserDn := qrUserDn
		if qUserDn != "" {

			if err := r.SetQueryParam("user_dn", qUserDn); err != nil {
				return err
			}
		}
	}

	if o.UserScope != nil {

		// query param user_scope
		var qrUserScope string

		if o.UserScope != nil {
			qrUserScope = *o.UserScope
		}
		qUserScope := qrUserScope
		if qUserScope != "" {

			if err := r.SetQueryParam("user_scope", qUserScope); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamLdapCollectionGet binds the parameter fields
func (o *LdapCollectionGetParams) bindParamFields(formats strfmt.Registry) []string {
	fieldsIR := o.Fields

	var fieldsIC []string
	for _, fieldsIIR := range fieldsIR { // explode []string

		fieldsIIV := fieldsIIR // string as string
		fieldsIC = append(fieldsIC, fieldsIIV)
	}

	// items.CollectionFormat: "csv"
	fieldsIS := swag.JoinByFormat(fieldsIC, "csv")

	return fieldsIS
}

// bindParamLdapCollectionGet binds the parameter order_by
func (o *LdapCollectionGetParams) bindParamOrderBy(formats strfmt.Registry) []string {
	orderByIR := o.OrderBy

	var orderByIC []string
	for _, orderByIIR := range orderByIR { // explode []string

		orderByIIV := orderByIIR // string as string
		orderByIC = append(orderByIC, orderByIIV)
	}

	// items.CollectionFormat: "csv"
	orderByIS := swag.JoinByFormat(orderByIC, "csv")

	return orderByIS
}
