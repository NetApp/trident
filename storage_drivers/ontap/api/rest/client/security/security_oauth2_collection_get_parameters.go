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

// NewSecurityOauth2CollectionGetParams creates a new SecurityOauth2CollectionGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewSecurityOauth2CollectionGetParams() *SecurityOauth2CollectionGetParams {
	return &SecurityOauth2CollectionGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewSecurityOauth2CollectionGetParamsWithTimeout creates a new SecurityOauth2CollectionGetParams object
// with the ability to set a timeout on a request.
func NewSecurityOauth2CollectionGetParamsWithTimeout(timeout time.Duration) *SecurityOauth2CollectionGetParams {
	return &SecurityOauth2CollectionGetParams{
		timeout: timeout,
	}
}

// NewSecurityOauth2CollectionGetParamsWithContext creates a new SecurityOauth2CollectionGetParams object
// with the ability to set a context for a request.
func NewSecurityOauth2CollectionGetParamsWithContext(ctx context.Context) *SecurityOauth2CollectionGetParams {
	return &SecurityOauth2CollectionGetParams{
		Context: ctx,
	}
}

// NewSecurityOauth2CollectionGetParamsWithHTTPClient creates a new SecurityOauth2CollectionGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewSecurityOauth2CollectionGetParamsWithHTTPClient(client *http.Client) *SecurityOauth2CollectionGetParams {
	return &SecurityOauth2CollectionGetParams{
		HTTPClient: client,
	}
}

/*
SecurityOauth2CollectionGetParams contains all the parameters to send to the API endpoint

	for the security oauth2 collection get operation.

	Typically these are written to a http.Request.
*/
type SecurityOauth2CollectionGetParams struct {

	/* Application.

	   Filter by application
	*/
	Application *string

	/* Audience.

	   Filter by audience
	*/
	Audience *string

	/* ClientID.

	   Filter by client_id
	*/
	ClientID *string

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* HashedClientSecret.

	   Filter by hashed_client_secret
	*/
	HashedClientSecret *string

	/* IntrospectionEndpointURI.

	   Filter by introspection.endpoint_uri
	*/
	IntrospectionEndpointURI *string

	/* IntrospectionInterval.

	   Filter by introspection.interval
	*/
	IntrospectionInterval *string

	/* Issuer.

	   Filter by issuer
	*/
	Issuer *string

	/* JwksProviderURI.

	   Filter by jwks.provider_uri
	*/
	JwksProviderURI *string

	/* JwksRefreshInterval.

	   Filter by jwks.refresh_interval
	*/
	JwksRefreshInterval *string

	/* MaxRecords.

	   Limit the number of records returned.
	*/
	MaxRecords *int64

	/* Name.

	   Filter by name
	*/
	Name *string

	/* OrderBy.

	   Order results by specified fields and optional [asc|desc] direction. Default direction is 'asc' for ascending.
	*/
	OrderBy []string

	/* OutgoingProxy.

	   Filter by outgoing_proxy
	*/
	OutgoingProxy *string

	/* Provider.

	   Filter by provider
	*/
	Provider *string

	/* RemoteUserClaim.

	   Filter by remote_user_claim
	*/
	RemoteUserClaim *string

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

	/* UseLocalRolesIfPresent.

	   Filter by use_local_roles_if_present
	*/
	UseLocalRolesIfPresent *bool

	/* UseMutualTLS.

	   Filter by use_mutual_tls
	*/
	UseMutualTLS *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the security oauth2 collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *SecurityOauth2CollectionGetParams) WithDefaults() *SecurityOauth2CollectionGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the security oauth2 collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *SecurityOauth2CollectionGetParams) SetDefaults() {
	var (
		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)
	)

	val := SecurityOauth2CollectionGetParams{
		ReturnRecords: &returnRecordsDefault,
		ReturnTimeout: &returnTimeoutDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithTimeout(timeout time.Duration) *SecurityOauth2CollectionGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithContext(ctx context.Context) *SecurityOauth2CollectionGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithHTTPClient(client *http.Client) *SecurityOauth2CollectionGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithApplication adds the application to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithApplication(application *string) *SecurityOauth2CollectionGetParams {
	o.SetApplication(application)
	return o
}

// SetApplication adds the application to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetApplication(application *string) {
	o.Application = application
}

// WithAudience adds the audience to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithAudience(audience *string) *SecurityOauth2CollectionGetParams {
	o.SetAudience(audience)
	return o
}

// SetAudience adds the audience to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetAudience(audience *string) {
	o.Audience = audience
}

// WithClientID adds the clientID to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithClientID(clientID *string) *SecurityOauth2CollectionGetParams {
	o.SetClientID(clientID)
	return o
}

// SetClientID adds the clientId to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetClientID(clientID *string) {
	o.ClientID = clientID
}

// WithFields adds the fields to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithFields(fields []string) *SecurityOauth2CollectionGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithHashedClientSecret adds the hashedClientSecret to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithHashedClientSecret(hashedClientSecret *string) *SecurityOauth2CollectionGetParams {
	o.SetHashedClientSecret(hashedClientSecret)
	return o
}

// SetHashedClientSecret adds the hashedClientSecret to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetHashedClientSecret(hashedClientSecret *string) {
	o.HashedClientSecret = hashedClientSecret
}

// WithIntrospectionEndpointURI adds the introspectionEndpointURI to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithIntrospectionEndpointURI(introspectionEndpointURI *string) *SecurityOauth2CollectionGetParams {
	o.SetIntrospectionEndpointURI(introspectionEndpointURI)
	return o
}

// SetIntrospectionEndpointURI adds the introspectionEndpointUri to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetIntrospectionEndpointURI(introspectionEndpointURI *string) {
	o.IntrospectionEndpointURI = introspectionEndpointURI
}

// WithIntrospectionInterval adds the introspectionInterval to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithIntrospectionInterval(introspectionInterval *string) *SecurityOauth2CollectionGetParams {
	o.SetIntrospectionInterval(introspectionInterval)
	return o
}

// SetIntrospectionInterval adds the introspectionInterval to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetIntrospectionInterval(introspectionInterval *string) {
	o.IntrospectionInterval = introspectionInterval
}

// WithIssuer adds the issuer to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithIssuer(issuer *string) *SecurityOauth2CollectionGetParams {
	o.SetIssuer(issuer)
	return o
}

// SetIssuer adds the issuer to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetIssuer(issuer *string) {
	o.Issuer = issuer
}

// WithJwksProviderURI adds the jwksProviderURI to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithJwksProviderURI(jwksProviderURI *string) *SecurityOauth2CollectionGetParams {
	o.SetJwksProviderURI(jwksProviderURI)
	return o
}

// SetJwksProviderURI adds the jwksProviderUri to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetJwksProviderURI(jwksProviderURI *string) {
	o.JwksProviderURI = jwksProviderURI
}

// WithJwksRefreshInterval adds the jwksRefreshInterval to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithJwksRefreshInterval(jwksRefreshInterval *string) *SecurityOauth2CollectionGetParams {
	o.SetJwksRefreshInterval(jwksRefreshInterval)
	return o
}

// SetJwksRefreshInterval adds the jwksRefreshInterval to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetJwksRefreshInterval(jwksRefreshInterval *string) {
	o.JwksRefreshInterval = jwksRefreshInterval
}

// WithMaxRecords adds the maxRecords to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithMaxRecords(maxRecords *int64) *SecurityOauth2CollectionGetParams {
	o.SetMaxRecords(maxRecords)
	return o
}

// SetMaxRecords adds the maxRecords to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetMaxRecords(maxRecords *int64) {
	o.MaxRecords = maxRecords
}

// WithName adds the name to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithName(name *string) *SecurityOauth2CollectionGetParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetName(name *string) {
	o.Name = name
}

// WithOrderBy adds the orderBy to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithOrderBy(orderBy []string) *SecurityOauth2CollectionGetParams {
	o.SetOrderBy(orderBy)
	return o
}

// SetOrderBy adds the orderBy to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetOrderBy(orderBy []string) {
	o.OrderBy = orderBy
}

// WithOutgoingProxy adds the outgoingProxy to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithOutgoingProxy(outgoingProxy *string) *SecurityOauth2CollectionGetParams {
	o.SetOutgoingProxy(outgoingProxy)
	return o
}

// SetOutgoingProxy adds the outgoingProxy to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetOutgoingProxy(outgoingProxy *string) {
	o.OutgoingProxy = outgoingProxy
}

// WithProvider adds the provider to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithProvider(provider *string) *SecurityOauth2CollectionGetParams {
	o.SetProvider(provider)
	return o
}

// SetProvider adds the provider to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetProvider(provider *string) {
	o.Provider = provider
}

// WithRemoteUserClaim adds the remoteUserClaim to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithRemoteUserClaim(remoteUserClaim *string) *SecurityOauth2CollectionGetParams {
	o.SetRemoteUserClaim(remoteUserClaim)
	return o
}

// SetRemoteUserClaim adds the remoteUserClaim to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetRemoteUserClaim(remoteUserClaim *string) {
	o.RemoteUserClaim = remoteUserClaim
}

// WithReturnRecords adds the returnRecords to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithReturnRecords(returnRecords *bool) *SecurityOauth2CollectionGetParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithReturnTimeout(returnTimeout *int64) *SecurityOauth2CollectionGetParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithUseLocalRolesIfPresent adds the useLocalRolesIfPresent to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithUseLocalRolesIfPresent(useLocalRolesIfPresent *bool) *SecurityOauth2CollectionGetParams {
	o.SetUseLocalRolesIfPresent(useLocalRolesIfPresent)
	return o
}

// SetUseLocalRolesIfPresent adds the useLocalRolesIfPresent to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetUseLocalRolesIfPresent(useLocalRolesIfPresent *bool) {
	o.UseLocalRolesIfPresent = useLocalRolesIfPresent
}

// WithUseMutualTLS adds the useMutualTLS to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) WithUseMutualTLS(useMutualTLS *string) *SecurityOauth2CollectionGetParams {
	o.SetUseMutualTLS(useMutualTLS)
	return o
}

// SetUseMutualTLS adds the useMutualTls to the security oauth2 collection get params
func (o *SecurityOauth2CollectionGetParams) SetUseMutualTLS(useMutualTLS *string) {
	o.UseMutualTLS = useMutualTLS
}

// WriteToRequest writes these params to a swagger request
func (o *SecurityOauth2CollectionGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.Application != nil {

		// query param application
		var qrApplication string

		if o.Application != nil {
			qrApplication = *o.Application
		}
		qApplication := qrApplication
		if qApplication != "" {

			if err := r.SetQueryParam("application", qApplication); err != nil {
				return err
			}
		}
	}

	if o.Audience != nil {

		// query param audience
		var qrAudience string

		if o.Audience != nil {
			qrAudience = *o.Audience
		}
		qAudience := qrAudience
		if qAudience != "" {

			if err := r.SetQueryParam("audience", qAudience); err != nil {
				return err
			}
		}
	}

	if o.ClientID != nil {

		// query param client_id
		var qrClientID string

		if o.ClientID != nil {
			qrClientID = *o.ClientID
		}
		qClientID := qrClientID
		if qClientID != "" {

			if err := r.SetQueryParam("client_id", qClientID); err != nil {
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

	if o.HashedClientSecret != nil {

		// query param hashed_client_secret
		var qrHashedClientSecret string

		if o.HashedClientSecret != nil {
			qrHashedClientSecret = *o.HashedClientSecret
		}
		qHashedClientSecret := qrHashedClientSecret
		if qHashedClientSecret != "" {

			if err := r.SetQueryParam("hashed_client_secret", qHashedClientSecret); err != nil {
				return err
			}
		}
	}

	if o.IntrospectionEndpointURI != nil {

		// query param introspection.endpoint_uri
		var qrIntrospectionEndpointURI string

		if o.IntrospectionEndpointURI != nil {
			qrIntrospectionEndpointURI = *o.IntrospectionEndpointURI
		}
		qIntrospectionEndpointURI := qrIntrospectionEndpointURI
		if qIntrospectionEndpointURI != "" {

			if err := r.SetQueryParam("introspection.endpoint_uri", qIntrospectionEndpointURI); err != nil {
				return err
			}
		}
	}

	if o.IntrospectionInterval != nil {

		// query param introspection.interval
		var qrIntrospectionInterval string

		if o.IntrospectionInterval != nil {
			qrIntrospectionInterval = *o.IntrospectionInterval
		}
		qIntrospectionInterval := qrIntrospectionInterval
		if qIntrospectionInterval != "" {

			if err := r.SetQueryParam("introspection.interval", qIntrospectionInterval); err != nil {
				return err
			}
		}
	}

	if o.Issuer != nil {

		// query param issuer
		var qrIssuer string

		if o.Issuer != nil {
			qrIssuer = *o.Issuer
		}
		qIssuer := qrIssuer
		if qIssuer != "" {

			if err := r.SetQueryParam("issuer", qIssuer); err != nil {
				return err
			}
		}
	}

	if o.JwksProviderURI != nil {

		// query param jwks.provider_uri
		var qrJwksProviderURI string

		if o.JwksProviderURI != nil {
			qrJwksProviderURI = *o.JwksProviderURI
		}
		qJwksProviderURI := qrJwksProviderURI
		if qJwksProviderURI != "" {

			if err := r.SetQueryParam("jwks.provider_uri", qJwksProviderURI); err != nil {
				return err
			}
		}
	}

	if o.JwksRefreshInterval != nil {

		// query param jwks.refresh_interval
		var qrJwksRefreshInterval string

		if o.JwksRefreshInterval != nil {
			qrJwksRefreshInterval = *o.JwksRefreshInterval
		}
		qJwksRefreshInterval := qrJwksRefreshInterval
		if qJwksRefreshInterval != "" {

			if err := r.SetQueryParam("jwks.refresh_interval", qJwksRefreshInterval); err != nil {
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

	if o.OrderBy != nil {

		// binding items for order_by
		joinedOrderBy := o.bindParamOrderBy(reg)

		// query array param order_by
		if err := r.SetQueryParam("order_by", joinedOrderBy...); err != nil {
			return err
		}
	}

	if o.OutgoingProxy != nil {

		// query param outgoing_proxy
		var qrOutgoingProxy string

		if o.OutgoingProxy != nil {
			qrOutgoingProxy = *o.OutgoingProxy
		}
		qOutgoingProxy := qrOutgoingProxy
		if qOutgoingProxy != "" {

			if err := r.SetQueryParam("outgoing_proxy", qOutgoingProxy); err != nil {
				return err
			}
		}
	}

	if o.Provider != nil {

		// query param provider
		var qrProvider string

		if o.Provider != nil {
			qrProvider = *o.Provider
		}
		qProvider := qrProvider
		if qProvider != "" {

			if err := r.SetQueryParam("provider", qProvider); err != nil {
				return err
			}
		}
	}

	if o.RemoteUserClaim != nil {

		// query param remote_user_claim
		var qrRemoteUserClaim string

		if o.RemoteUserClaim != nil {
			qrRemoteUserClaim = *o.RemoteUserClaim
		}
		qRemoteUserClaim := qrRemoteUserClaim
		if qRemoteUserClaim != "" {

			if err := r.SetQueryParam("remote_user_claim", qRemoteUserClaim); err != nil {
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

	if o.UseLocalRolesIfPresent != nil {

		// query param use_local_roles_if_present
		var qrUseLocalRolesIfPresent bool

		if o.UseLocalRolesIfPresent != nil {
			qrUseLocalRolesIfPresent = *o.UseLocalRolesIfPresent
		}
		qUseLocalRolesIfPresent := swag.FormatBool(qrUseLocalRolesIfPresent)
		if qUseLocalRolesIfPresent != "" {

			if err := r.SetQueryParam("use_local_roles_if_present", qUseLocalRolesIfPresent); err != nil {
				return err
			}
		}
	}

	if o.UseMutualTLS != nil {

		// query param use_mutual_tls
		var qrUseMutualTLS string

		if o.UseMutualTLS != nil {
			qrUseMutualTLS = *o.UseMutualTLS
		}
		qUseMutualTLS := qrUseMutualTLS
		if qUseMutualTLS != "" {

			if err := r.SetQueryParam("use_mutual_tls", qUseMutualTLS); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamSecurityOauth2CollectionGet binds the parameter fields
func (o *SecurityOauth2CollectionGetParams) bindParamFields(formats strfmt.Registry) []string {
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

// bindParamSecurityOauth2CollectionGet binds the parameter order_by
func (o *SecurityOauth2CollectionGetParams) bindParamOrderBy(formats strfmt.Registry) []string {
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
