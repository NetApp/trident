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

// NewIpsecPolicyDeleteCollectionParams creates a new IpsecPolicyDeleteCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewIpsecPolicyDeleteCollectionParams() *IpsecPolicyDeleteCollectionParams {
	return &IpsecPolicyDeleteCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewIpsecPolicyDeleteCollectionParamsWithTimeout creates a new IpsecPolicyDeleteCollectionParams object
// with the ability to set a timeout on a request.
func NewIpsecPolicyDeleteCollectionParamsWithTimeout(timeout time.Duration) *IpsecPolicyDeleteCollectionParams {
	return &IpsecPolicyDeleteCollectionParams{
		timeout: timeout,
	}
}

// NewIpsecPolicyDeleteCollectionParamsWithContext creates a new IpsecPolicyDeleteCollectionParams object
// with the ability to set a context for a request.
func NewIpsecPolicyDeleteCollectionParamsWithContext(ctx context.Context) *IpsecPolicyDeleteCollectionParams {
	return &IpsecPolicyDeleteCollectionParams{
		Context: ctx,
	}
}

// NewIpsecPolicyDeleteCollectionParamsWithHTTPClient creates a new IpsecPolicyDeleteCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewIpsecPolicyDeleteCollectionParamsWithHTTPClient(client *http.Client) *IpsecPolicyDeleteCollectionParams {
	return &IpsecPolicyDeleteCollectionParams{
		HTTPClient: client,
	}
}

/*
IpsecPolicyDeleteCollectionParams contains all the parameters to send to the API endpoint

	for the ipsec policy delete collection operation.

	Typically these are written to a http.Request.
*/
type IpsecPolicyDeleteCollectionParams struct {

	/* Action.

	   Filter by action
	*/
	Action *string

	/* AuthenticationMethod.

	   Filter by authentication_method
	*/
	AuthenticationMethod *string

	/* CertificateName.

	   Filter by certificate.name
	*/
	CertificateName *string

	/* CertificateUUID.

	   Filter by certificate.uuid
	*/
	CertificateUUID *string

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* Enabled.

	   Filter by enabled
	*/
	Enabled *bool

	/* Info.

	   Info specification
	*/
	Info IpsecPolicyDeleteCollectionBody

	/* IpspaceName.

	   Filter by ipspace.name
	*/
	IpspaceName *string

	/* IpspaceUUID.

	   Filter by ipspace.uuid
	*/
	IpspaceUUID *string

	/* LocalEndpointAddress.

	   Filter by local_endpoint.address
	*/
	LocalEndpointAddress *string

	/* LocalEndpointFamily.

	   Filter by local_endpoint.family
	*/
	LocalEndpointFamily *string

	/* LocalEndpointNetmask.

	   Filter by local_endpoint.netmask
	*/
	LocalEndpointNetmask *string

	/* LocalEndpointPort.

	   Filter by local_endpoint.port
	*/
	LocalEndpointPort *string

	/* LocalIdentity.

	   Filter by local_identity
	*/
	LocalIdentity *string

	/* Name.

	   Filter by name
	*/
	Name *string

	/* Protocol.

	   Filter by protocol
	*/
	Protocol *string

	/* RemoteEndpointAddress.

	   Filter by remote_endpoint.address
	*/
	RemoteEndpointAddress *string

	/* RemoteEndpointFamily.

	   Filter by remote_endpoint.family
	*/
	RemoteEndpointFamily *string

	/* RemoteEndpointNetmask.

	   Filter by remote_endpoint.netmask
	*/
	RemoteEndpointNetmask *string

	/* RemoteEndpointPort.

	   Filter by remote_endpoint.port
	*/
	RemoteEndpointPort *string

	/* RemoteIdentity.

	   Filter by remote_identity
	*/
	RemoteIdentity *string

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

	/* Scope.

	   Filter by scope
	*/
	Scope *string

	/* SerialRecords.

	   Perform the operation on the records synchronously.
	*/
	SerialRecords *bool

	/* SvmName.

	   Filter by svm.name
	*/
	SvmName *string

	/* SvmUUID.

	   Filter by svm.uuid
	*/
	SvmUUID *string

	/* UUID.

	   Filter by uuid
	*/
	UUID *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the ipsec policy delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *IpsecPolicyDeleteCollectionParams) WithDefaults() *IpsecPolicyDeleteCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the ipsec policy delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *IpsecPolicyDeleteCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := IpsecPolicyDeleteCollectionParams{
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

// WithTimeout adds the timeout to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithTimeout(timeout time.Duration) *IpsecPolicyDeleteCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithContext(ctx context.Context) *IpsecPolicyDeleteCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithHTTPClient(client *http.Client) *IpsecPolicyDeleteCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithAction adds the action to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithAction(action *string) *IpsecPolicyDeleteCollectionParams {
	o.SetAction(action)
	return o
}

// SetAction adds the action to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetAction(action *string) {
	o.Action = action
}

// WithAuthenticationMethod adds the authenticationMethod to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithAuthenticationMethod(authenticationMethod *string) *IpsecPolicyDeleteCollectionParams {
	o.SetAuthenticationMethod(authenticationMethod)
	return o
}

// SetAuthenticationMethod adds the authenticationMethod to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetAuthenticationMethod(authenticationMethod *string) {
	o.AuthenticationMethod = authenticationMethod
}

// WithCertificateName adds the certificateName to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithCertificateName(certificateName *string) *IpsecPolicyDeleteCollectionParams {
	o.SetCertificateName(certificateName)
	return o
}

// SetCertificateName adds the certificateName to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetCertificateName(certificateName *string) {
	o.CertificateName = certificateName
}

// WithCertificateUUID adds the certificateUUID to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithCertificateUUID(certificateUUID *string) *IpsecPolicyDeleteCollectionParams {
	o.SetCertificateUUID(certificateUUID)
	return o
}

// SetCertificateUUID adds the certificateUuid to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetCertificateUUID(certificateUUID *string) {
	o.CertificateUUID = certificateUUID
}

// WithContinueOnFailure adds the continueOnFailure to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *IpsecPolicyDeleteCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithEnabled adds the enabled to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithEnabled(enabled *bool) *IpsecPolicyDeleteCollectionParams {
	o.SetEnabled(enabled)
	return o
}

// SetEnabled adds the enabled to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetEnabled(enabled *bool) {
	o.Enabled = enabled
}

// WithInfo adds the info to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithInfo(info IpsecPolicyDeleteCollectionBody) *IpsecPolicyDeleteCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetInfo(info IpsecPolicyDeleteCollectionBody) {
	o.Info = info
}

// WithIpspaceName adds the ipspaceName to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithIpspaceName(ipspaceName *string) *IpsecPolicyDeleteCollectionParams {
	o.SetIpspaceName(ipspaceName)
	return o
}

// SetIpspaceName adds the ipspaceName to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetIpspaceName(ipspaceName *string) {
	o.IpspaceName = ipspaceName
}

// WithIpspaceUUID adds the ipspaceUUID to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithIpspaceUUID(ipspaceUUID *string) *IpsecPolicyDeleteCollectionParams {
	o.SetIpspaceUUID(ipspaceUUID)
	return o
}

// SetIpspaceUUID adds the ipspaceUuid to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetIpspaceUUID(ipspaceUUID *string) {
	o.IpspaceUUID = ipspaceUUID
}

// WithLocalEndpointAddress adds the localEndpointAddress to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithLocalEndpointAddress(localEndpointAddress *string) *IpsecPolicyDeleteCollectionParams {
	o.SetLocalEndpointAddress(localEndpointAddress)
	return o
}

// SetLocalEndpointAddress adds the localEndpointAddress to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetLocalEndpointAddress(localEndpointAddress *string) {
	o.LocalEndpointAddress = localEndpointAddress
}

// WithLocalEndpointFamily adds the localEndpointFamily to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithLocalEndpointFamily(localEndpointFamily *string) *IpsecPolicyDeleteCollectionParams {
	o.SetLocalEndpointFamily(localEndpointFamily)
	return o
}

// SetLocalEndpointFamily adds the localEndpointFamily to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetLocalEndpointFamily(localEndpointFamily *string) {
	o.LocalEndpointFamily = localEndpointFamily
}

// WithLocalEndpointNetmask adds the localEndpointNetmask to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithLocalEndpointNetmask(localEndpointNetmask *string) *IpsecPolicyDeleteCollectionParams {
	o.SetLocalEndpointNetmask(localEndpointNetmask)
	return o
}

// SetLocalEndpointNetmask adds the localEndpointNetmask to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetLocalEndpointNetmask(localEndpointNetmask *string) {
	o.LocalEndpointNetmask = localEndpointNetmask
}

// WithLocalEndpointPort adds the localEndpointPort to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithLocalEndpointPort(localEndpointPort *string) *IpsecPolicyDeleteCollectionParams {
	o.SetLocalEndpointPort(localEndpointPort)
	return o
}

// SetLocalEndpointPort adds the localEndpointPort to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetLocalEndpointPort(localEndpointPort *string) {
	o.LocalEndpointPort = localEndpointPort
}

// WithLocalIdentity adds the localIdentity to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithLocalIdentity(localIdentity *string) *IpsecPolicyDeleteCollectionParams {
	o.SetLocalIdentity(localIdentity)
	return o
}

// SetLocalIdentity adds the localIdentity to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetLocalIdentity(localIdentity *string) {
	o.LocalIdentity = localIdentity
}

// WithName adds the name to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithName(name *string) *IpsecPolicyDeleteCollectionParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetName(name *string) {
	o.Name = name
}

// WithProtocol adds the protocol to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithProtocol(protocol *string) *IpsecPolicyDeleteCollectionParams {
	o.SetProtocol(protocol)
	return o
}

// SetProtocol adds the protocol to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetProtocol(protocol *string) {
	o.Protocol = protocol
}

// WithRemoteEndpointAddress adds the remoteEndpointAddress to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithRemoteEndpointAddress(remoteEndpointAddress *string) *IpsecPolicyDeleteCollectionParams {
	o.SetRemoteEndpointAddress(remoteEndpointAddress)
	return o
}

// SetRemoteEndpointAddress adds the remoteEndpointAddress to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetRemoteEndpointAddress(remoteEndpointAddress *string) {
	o.RemoteEndpointAddress = remoteEndpointAddress
}

// WithRemoteEndpointFamily adds the remoteEndpointFamily to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithRemoteEndpointFamily(remoteEndpointFamily *string) *IpsecPolicyDeleteCollectionParams {
	o.SetRemoteEndpointFamily(remoteEndpointFamily)
	return o
}

// SetRemoteEndpointFamily adds the remoteEndpointFamily to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetRemoteEndpointFamily(remoteEndpointFamily *string) {
	o.RemoteEndpointFamily = remoteEndpointFamily
}

// WithRemoteEndpointNetmask adds the remoteEndpointNetmask to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithRemoteEndpointNetmask(remoteEndpointNetmask *string) *IpsecPolicyDeleteCollectionParams {
	o.SetRemoteEndpointNetmask(remoteEndpointNetmask)
	return o
}

// SetRemoteEndpointNetmask adds the remoteEndpointNetmask to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetRemoteEndpointNetmask(remoteEndpointNetmask *string) {
	o.RemoteEndpointNetmask = remoteEndpointNetmask
}

// WithRemoteEndpointPort adds the remoteEndpointPort to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithRemoteEndpointPort(remoteEndpointPort *string) *IpsecPolicyDeleteCollectionParams {
	o.SetRemoteEndpointPort(remoteEndpointPort)
	return o
}

// SetRemoteEndpointPort adds the remoteEndpointPort to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetRemoteEndpointPort(remoteEndpointPort *string) {
	o.RemoteEndpointPort = remoteEndpointPort
}

// WithRemoteIdentity adds the remoteIdentity to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithRemoteIdentity(remoteIdentity *string) *IpsecPolicyDeleteCollectionParams {
	o.SetRemoteIdentity(remoteIdentity)
	return o
}

// SetRemoteIdentity adds the remoteIdentity to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetRemoteIdentity(remoteIdentity *string) {
	o.RemoteIdentity = remoteIdentity
}

// WithReturnRecords adds the returnRecords to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithReturnRecords(returnRecords *bool) *IpsecPolicyDeleteCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithReturnTimeout(returnTimeout *int64) *IpsecPolicyDeleteCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithScope adds the scope to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithScope(scope *string) *IpsecPolicyDeleteCollectionParams {
	o.SetScope(scope)
	return o
}

// SetScope adds the scope to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetScope(scope *string) {
	o.Scope = scope
}

// WithSerialRecords adds the serialRecords to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithSerialRecords(serialRecords *bool) *IpsecPolicyDeleteCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WithSvmName adds the svmName to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithSvmName(svmName *string) *IpsecPolicyDeleteCollectionParams {
	o.SetSvmName(svmName)
	return o
}

// SetSvmName adds the svmName to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetSvmName(svmName *string) {
	o.SvmName = svmName
}

// WithSvmUUID adds the svmUUID to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithSvmUUID(svmUUID *string) *IpsecPolicyDeleteCollectionParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetSvmUUID(svmUUID *string) {
	o.SvmUUID = svmUUID
}

// WithUUID adds the uuid to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) WithUUID(uuid *string) *IpsecPolicyDeleteCollectionParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the ipsec policy delete collection params
func (o *IpsecPolicyDeleteCollectionParams) SetUUID(uuid *string) {
	o.UUID = uuid
}

// WriteToRequest writes these params to a swagger request
func (o *IpsecPolicyDeleteCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.Action != nil {

		// query param action
		var qrAction string

		if o.Action != nil {
			qrAction = *o.Action
		}
		qAction := qrAction
		if qAction != "" {

			if err := r.SetQueryParam("action", qAction); err != nil {
				return err
			}
		}
	}

	if o.AuthenticationMethod != nil {

		// query param authentication_method
		var qrAuthenticationMethod string

		if o.AuthenticationMethod != nil {
			qrAuthenticationMethod = *o.AuthenticationMethod
		}
		qAuthenticationMethod := qrAuthenticationMethod
		if qAuthenticationMethod != "" {

			if err := r.SetQueryParam("authentication_method", qAuthenticationMethod); err != nil {
				return err
			}
		}
	}

	if o.CertificateName != nil {

		// query param certificate.name
		var qrCertificateName string

		if o.CertificateName != nil {
			qrCertificateName = *o.CertificateName
		}
		qCertificateName := qrCertificateName
		if qCertificateName != "" {

			if err := r.SetQueryParam("certificate.name", qCertificateName); err != nil {
				return err
			}
		}
	}

	if o.CertificateUUID != nil {

		// query param certificate.uuid
		var qrCertificateUUID string

		if o.CertificateUUID != nil {
			qrCertificateUUID = *o.CertificateUUID
		}
		qCertificateUUID := qrCertificateUUID
		if qCertificateUUID != "" {

			if err := r.SetQueryParam("certificate.uuid", qCertificateUUID); err != nil {
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

	if o.Enabled != nil {

		// query param enabled
		var qrEnabled bool

		if o.Enabled != nil {
			qrEnabled = *o.Enabled
		}
		qEnabled := swag.FormatBool(qrEnabled)
		if qEnabled != "" {

			if err := r.SetQueryParam("enabled", qEnabled); err != nil {
				return err
			}
		}
	}
	if err := r.SetBodyParam(o.Info); err != nil {
		return err
	}

	if o.IpspaceName != nil {

		// query param ipspace.name
		var qrIpspaceName string

		if o.IpspaceName != nil {
			qrIpspaceName = *o.IpspaceName
		}
		qIpspaceName := qrIpspaceName
		if qIpspaceName != "" {

			if err := r.SetQueryParam("ipspace.name", qIpspaceName); err != nil {
				return err
			}
		}
	}

	if o.IpspaceUUID != nil {

		// query param ipspace.uuid
		var qrIpspaceUUID string

		if o.IpspaceUUID != nil {
			qrIpspaceUUID = *o.IpspaceUUID
		}
		qIpspaceUUID := qrIpspaceUUID
		if qIpspaceUUID != "" {

			if err := r.SetQueryParam("ipspace.uuid", qIpspaceUUID); err != nil {
				return err
			}
		}
	}

	if o.LocalEndpointAddress != nil {

		// query param local_endpoint.address
		var qrLocalEndpointAddress string

		if o.LocalEndpointAddress != nil {
			qrLocalEndpointAddress = *o.LocalEndpointAddress
		}
		qLocalEndpointAddress := qrLocalEndpointAddress
		if qLocalEndpointAddress != "" {

			if err := r.SetQueryParam("local_endpoint.address", qLocalEndpointAddress); err != nil {
				return err
			}
		}
	}

	if o.LocalEndpointFamily != nil {

		// query param local_endpoint.family
		var qrLocalEndpointFamily string

		if o.LocalEndpointFamily != nil {
			qrLocalEndpointFamily = *o.LocalEndpointFamily
		}
		qLocalEndpointFamily := qrLocalEndpointFamily
		if qLocalEndpointFamily != "" {

			if err := r.SetQueryParam("local_endpoint.family", qLocalEndpointFamily); err != nil {
				return err
			}
		}
	}

	if o.LocalEndpointNetmask != nil {

		// query param local_endpoint.netmask
		var qrLocalEndpointNetmask string

		if o.LocalEndpointNetmask != nil {
			qrLocalEndpointNetmask = *o.LocalEndpointNetmask
		}
		qLocalEndpointNetmask := qrLocalEndpointNetmask
		if qLocalEndpointNetmask != "" {

			if err := r.SetQueryParam("local_endpoint.netmask", qLocalEndpointNetmask); err != nil {
				return err
			}
		}
	}

	if o.LocalEndpointPort != nil {

		// query param local_endpoint.port
		var qrLocalEndpointPort string

		if o.LocalEndpointPort != nil {
			qrLocalEndpointPort = *o.LocalEndpointPort
		}
		qLocalEndpointPort := qrLocalEndpointPort
		if qLocalEndpointPort != "" {

			if err := r.SetQueryParam("local_endpoint.port", qLocalEndpointPort); err != nil {
				return err
			}
		}
	}

	if o.LocalIdentity != nil {

		// query param local_identity
		var qrLocalIdentity string

		if o.LocalIdentity != nil {
			qrLocalIdentity = *o.LocalIdentity
		}
		qLocalIdentity := qrLocalIdentity
		if qLocalIdentity != "" {

			if err := r.SetQueryParam("local_identity", qLocalIdentity); err != nil {
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

	if o.Protocol != nil {

		// query param protocol
		var qrProtocol string

		if o.Protocol != nil {
			qrProtocol = *o.Protocol
		}
		qProtocol := qrProtocol
		if qProtocol != "" {

			if err := r.SetQueryParam("protocol", qProtocol); err != nil {
				return err
			}
		}
	}

	if o.RemoteEndpointAddress != nil {

		// query param remote_endpoint.address
		var qrRemoteEndpointAddress string

		if o.RemoteEndpointAddress != nil {
			qrRemoteEndpointAddress = *o.RemoteEndpointAddress
		}
		qRemoteEndpointAddress := qrRemoteEndpointAddress
		if qRemoteEndpointAddress != "" {

			if err := r.SetQueryParam("remote_endpoint.address", qRemoteEndpointAddress); err != nil {
				return err
			}
		}
	}

	if o.RemoteEndpointFamily != nil {

		// query param remote_endpoint.family
		var qrRemoteEndpointFamily string

		if o.RemoteEndpointFamily != nil {
			qrRemoteEndpointFamily = *o.RemoteEndpointFamily
		}
		qRemoteEndpointFamily := qrRemoteEndpointFamily
		if qRemoteEndpointFamily != "" {

			if err := r.SetQueryParam("remote_endpoint.family", qRemoteEndpointFamily); err != nil {
				return err
			}
		}
	}

	if o.RemoteEndpointNetmask != nil {

		// query param remote_endpoint.netmask
		var qrRemoteEndpointNetmask string

		if o.RemoteEndpointNetmask != nil {
			qrRemoteEndpointNetmask = *o.RemoteEndpointNetmask
		}
		qRemoteEndpointNetmask := qrRemoteEndpointNetmask
		if qRemoteEndpointNetmask != "" {

			if err := r.SetQueryParam("remote_endpoint.netmask", qRemoteEndpointNetmask); err != nil {
				return err
			}
		}
	}

	if o.RemoteEndpointPort != nil {

		// query param remote_endpoint.port
		var qrRemoteEndpointPort string

		if o.RemoteEndpointPort != nil {
			qrRemoteEndpointPort = *o.RemoteEndpointPort
		}
		qRemoteEndpointPort := qrRemoteEndpointPort
		if qRemoteEndpointPort != "" {

			if err := r.SetQueryParam("remote_endpoint.port", qRemoteEndpointPort); err != nil {
				return err
			}
		}
	}

	if o.RemoteIdentity != nil {

		// query param remote_identity
		var qrRemoteIdentity string

		if o.RemoteIdentity != nil {
			qrRemoteIdentity = *o.RemoteIdentity
		}
		qRemoteIdentity := qrRemoteIdentity
		if qRemoteIdentity != "" {

			if err := r.SetQueryParam("remote_identity", qRemoteIdentity); err != nil {
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

	if o.UUID != nil {

		// query param uuid
		var qrUUID string

		if o.UUID != nil {
			qrUUID = *o.UUID
		}
		qUUID := qrUUID
		if qUUID != "" {

			if err := r.SetQueryParam("uuid", qUUID); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
