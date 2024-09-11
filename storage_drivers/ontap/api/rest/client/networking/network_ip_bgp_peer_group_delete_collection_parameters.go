// Code generated by go-swagger; DO NOT EDIT.

package networking

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

// NewNetworkIPBgpPeerGroupDeleteCollectionParams creates a new NetworkIPBgpPeerGroupDeleteCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewNetworkIPBgpPeerGroupDeleteCollectionParams() *NetworkIPBgpPeerGroupDeleteCollectionParams {
	return &NetworkIPBgpPeerGroupDeleteCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewNetworkIPBgpPeerGroupDeleteCollectionParamsWithTimeout creates a new NetworkIPBgpPeerGroupDeleteCollectionParams object
// with the ability to set a timeout on a request.
func NewNetworkIPBgpPeerGroupDeleteCollectionParamsWithTimeout(timeout time.Duration) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	return &NetworkIPBgpPeerGroupDeleteCollectionParams{
		timeout: timeout,
	}
}

// NewNetworkIPBgpPeerGroupDeleteCollectionParamsWithContext creates a new NetworkIPBgpPeerGroupDeleteCollectionParams object
// with the ability to set a context for a request.
func NewNetworkIPBgpPeerGroupDeleteCollectionParamsWithContext(ctx context.Context) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	return &NetworkIPBgpPeerGroupDeleteCollectionParams{
		Context: ctx,
	}
}

// NewNetworkIPBgpPeerGroupDeleteCollectionParamsWithHTTPClient creates a new NetworkIPBgpPeerGroupDeleteCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewNetworkIPBgpPeerGroupDeleteCollectionParamsWithHTTPClient(client *http.Client) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	return &NetworkIPBgpPeerGroupDeleteCollectionParams{
		HTTPClient: client,
	}
}

/*
NetworkIPBgpPeerGroupDeleteCollectionParams contains all the parameters to send to the API endpoint

	for the network ip bgp peer group delete collection operation.

	Typically these are written to a http.Request.
*/
type NetworkIPBgpPeerGroupDeleteCollectionParams struct {

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* Info.

	   Info specification
	*/
	Info NetworkIPBgpPeerGroupDeleteCollectionBody

	/* IpspaceName.

	   Filter by ipspace.name
	*/
	IpspaceName *string

	/* IpspaceUUID.

	   Filter by ipspace.uuid
	*/
	IpspaceUUID *string

	/* LocalInterfaceIPAddress.

	   Filter by local.interface.ip.address
	*/
	LocalInterfaceIPAddress *string

	/* LocalInterfaceName.

	   Filter by local.interface.name
	*/
	LocalInterfaceName *string

	/* LocalInterfaceUUID.

	   Filter by local.interface.uuid
	*/
	LocalInterfaceUUID *string

	/* LocalPortName.

	   Filter by local.port.name
	*/
	LocalPortName *string

	/* LocalPortNodeName.

	   Filter by local.port.node.name
	*/
	LocalPortNodeName *string

	/* LocalPortUUID.

	   Filter by local.port.uuid
	*/
	LocalPortUUID *string

	/* Name.

	   Filter by name
	*/
	Name *string

	/* PeerAddress.

	   Filter by peer.address
	*/
	PeerAddress *string

	/* PeerAsn.

	   Filter by peer.asn
	*/
	PeerAsn *int64

	/* PeerIsNextHop.

	   Filter by peer.is_next_hop
	*/
	PeerIsNextHop *bool

	/* PeerMd5Enabled.

	   Filter by peer.md5_enabled
	*/
	PeerMd5Enabled *bool

	/* PeerMd5Secret.

	   Filter by peer.md5_secret
	*/
	PeerMd5Secret *string

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

	/* SerialRecords.

	   Perform the operation on the records synchronously.
	*/
	SerialRecords *bool

	/* State.

	   Filter by state
	*/
	State *string

	/* UUID.

	   Filter by uuid
	*/
	UUID *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the network ip bgp peer group delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithDefaults() *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the network ip bgp peer group delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := NetworkIPBgpPeerGroupDeleteCollectionParams{
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

// WithTimeout adds the timeout to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithTimeout(timeout time.Duration) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithContext(ctx context.Context) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithHTTPClient(client *http.Client) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithContinueOnFailure adds the continueOnFailure to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithInfo adds the info to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithInfo(info NetworkIPBgpPeerGroupDeleteCollectionBody) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetInfo(info NetworkIPBgpPeerGroupDeleteCollectionBody) {
	o.Info = info
}

// WithIpspaceName adds the ipspaceName to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithIpspaceName(ipspaceName *string) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetIpspaceName(ipspaceName)
	return o
}

// SetIpspaceName adds the ipspaceName to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetIpspaceName(ipspaceName *string) {
	o.IpspaceName = ipspaceName
}

// WithIpspaceUUID adds the ipspaceUUID to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithIpspaceUUID(ipspaceUUID *string) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetIpspaceUUID(ipspaceUUID)
	return o
}

// SetIpspaceUUID adds the ipspaceUuid to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetIpspaceUUID(ipspaceUUID *string) {
	o.IpspaceUUID = ipspaceUUID
}

// WithLocalInterfaceIPAddress adds the localInterfaceIPAddress to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithLocalInterfaceIPAddress(localInterfaceIPAddress *string) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetLocalInterfaceIPAddress(localInterfaceIPAddress)
	return o
}

// SetLocalInterfaceIPAddress adds the localInterfaceIpAddress to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetLocalInterfaceIPAddress(localInterfaceIPAddress *string) {
	o.LocalInterfaceIPAddress = localInterfaceIPAddress
}

// WithLocalInterfaceName adds the localInterfaceName to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithLocalInterfaceName(localInterfaceName *string) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetLocalInterfaceName(localInterfaceName)
	return o
}

// SetLocalInterfaceName adds the localInterfaceName to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetLocalInterfaceName(localInterfaceName *string) {
	o.LocalInterfaceName = localInterfaceName
}

// WithLocalInterfaceUUID adds the localInterfaceUUID to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithLocalInterfaceUUID(localInterfaceUUID *string) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetLocalInterfaceUUID(localInterfaceUUID)
	return o
}

// SetLocalInterfaceUUID adds the localInterfaceUuid to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetLocalInterfaceUUID(localInterfaceUUID *string) {
	o.LocalInterfaceUUID = localInterfaceUUID
}

// WithLocalPortName adds the localPortName to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithLocalPortName(localPortName *string) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetLocalPortName(localPortName)
	return o
}

// SetLocalPortName adds the localPortName to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetLocalPortName(localPortName *string) {
	o.LocalPortName = localPortName
}

// WithLocalPortNodeName adds the localPortNodeName to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithLocalPortNodeName(localPortNodeName *string) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetLocalPortNodeName(localPortNodeName)
	return o
}

// SetLocalPortNodeName adds the localPortNodeName to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetLocalPortNodeName(localPortNodeName *string) {
	o.LocalPortNodeName = localPortNodeName
}

// WithLocalPortUUID adds the localPortUUID to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithLocalPortUUID(localPortUUID *string) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetLocalPortUUID(localPortUUID)
	return o
}

// SetLocalPortUUID adds the localPortUuid to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetLocalPortUUID(localPortUUID *string) {
	o.LocalPortUUID = localPortUUID
}

// WithName adds the name to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithName(name *string) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetName(name *string) {
	o.Name = name
}

// WithPeerAddress adds the peerAddress to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithPeerAddress(peerAddress *string) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetPeerAddress(peerAddress)
	return o
}

// SetPeerAddress adds the peerAddress to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetPeerAddress(peerAddress *string) {
	o.PeerAddress = peerAddress
}

// WithPeerAsn adds the peerAsn to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithPeerAsn(peerAsn *int64) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetPeerAsn(peerAsn)
	return o
}

// SetPeerAsn adds the peerAsn to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetPeerAsn(peerAsn *int64) {
	o.PeerAsn = peerAsn
}

// WithPeerIsNextHop adds the peerIsNextHop to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithPeerIsNextHop(peerIsNextHop *bool) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetPeerIsNextHop(peerIsNextHop)
	return o
}

// SetPeerIsNextHop adds the peerIsNextHop to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetPeerIsNextHop(peerIsNextHop *bool) {
	o.PeerIsNextHop = peerIsNextHop
}

// WithPeerMd5Enabled adds the peerMd5Enabled to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithPeerMd5Enabled(peerMd5Enabled *bool) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetPeerMd5Enabled(peerMd5Enabled)
	return o
}

// SetPeerMd5Enabled adds the peerMd5Enabled to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetPeerMd5Enabled(peerMd5Enabled *bool) {
	o.PeerMd5Enabled = peerMd5Enabled
}

// WithPeerMd5Secret adds the peerMd5Secret to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithPeerMd5Secret(peerMd5Secret *string) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetPeerMd5Secret(peerMd5Secret)
	return o
}

// SetPeerMd5Secret adds the peerMd5Secret to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetPeerMd5Secret(peerMd5Secret *string) {
	o.PeerMd5Secret = peerMd5Secret
}

// WithReturnRecords adds the returnRecords to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithReturnRecords(returnRecords *bool) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithReturnTimeout(returnTimeout *int64) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithSerialRecords adds the serialRecords to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithSerialRecords(serialRecords *bool) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WithState adds the state to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithState(state *string) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetState(state)
	return o
}

// SetState adds the state to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetState(state *string) {
	o.State = state
}

// WithUUID adds the uuid to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WithUUID(uuid *string) *NetworkIPBgpPeerGroupDeleteCollectionParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the network ip bgp peer group delete collection params
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) SetUUID(uuid *string) {
	o.UUID = uuid
}

// WriteToRequest writes these params to a swagger request
func (o *NetworkIPBgpPeerGroupDeleteCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

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

	if o.LocalInterfaceIPAddress != nil {

		// query param local.interface.ip.address
		var qrLocalInterfaceIPAddress string

		if o.LocalInterfaceIPAddress != nil {
			qrLocalInterfaceIPAddress = *o.LocalInterfaceIPAddress
		}
		qLocalInterfaceIPAddress := qrLocalInterfaceIPAddress
		if qLocalInterfaceIPAddress != "" {

			if err := r.SetQueryParam("local.interface.ip.address", qLocalInterfaceIPAddress); err != nil {
				return err
			}
		}
	}

	if o.LocalInterfaceName != nil {

		// query param local.interface.name
		var qrLocalInterfaceName string

		if o.LocalInterfaceName != nil {
			qrLocalInterfaceName = *o.LocalInterfaceName
		}
		qLocalInterfaceName := qrLocalInterfaceName
		if qLocalInterfaceName != "" {

			if err := r.SetQueryParam("local.interface.name", qLocalInterfaceName); err != nil {
				return err
			}
		}
	}

	if o.LocalInterfaceUUID != nil {

		// query param local.interface.uuid
		var qrLocalInterfaceUUID string

		if o.LocalInterfaceUUID != nil {
			qrLocalInterfaceUUID = *o.LocalInterfaceUUID
		}
		qLocalInterfaceUUID := qrLocalInterfaceUUID
		if qLocalInterfaceUUID != "" {

			if err := r.SetQueryParam("local.interface.uuid", qLocalInterfaceUUID); err != nil {
				return err
			}
		}
	}

	if o.LocalPortName != nil {

		// query param local.port.name
		var qrLocalPortName string

		if o.LocalPortName != nil {
			qrLocalPortName = *o.LocalPortName
		}
		qLocalPortName := qrLocalPortName
		if qLocalPortName != "" {

			if err := r.SetQueryParam("local.port.name", qLocalPortName); err != nil {
				return err
			}
		}
	}

	if o.LocalPortNodeName != nil {

		// query param local.port.node.name
		var qrLocalPortNodeName string

		if o.LocalPortNodeName != nil {
			qrLocalPortNodeName = *o.LocalPortNodeName
		}
		qLocalPortNodeName := qrLocalPortNodeName
		if qLocalPortNodeName != "" {

			if err := r.SetQueryParam("local.port.node.name", qLocalPortNodeName); err != nil {
				return err
			}
		}
	}

	if o.LocalPortUUID != nil {

		// query param local.port.uuid
		var qrLocalPortUUID string

		if o.LocalPortUUID != nil {
			qrLocalPortUUID = *o.LocalPortUUID
		}
		qLocalPortUUID := qrLocalPortUUID
		if qLocalPortUUID != "" {

			if err := r.SetQueryParam("local.port.uuid", qLocalPortUUID); err != nil {
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

	if o.PeerAddress != nil {

		// query param peer.address
		var qrPeerAddress string

		if o.PeerAddress != nil {
			qrPeerAddress = *o.PeerAddress
		}
		qPeerAddress := qrPeerAddress
		if qPeerAddress != "" {

			if err := r.SetQueryParam("peer.address", qPeerAddress); err != nil {
				return err
			}
		}
	}

	if o.PeerAsn != nil {

		// query param peer.asn
		var qrPeerAsn int64

		if o.PeerAsn != nil {
			qrPeerAsn = *o.PeerAsn
		}
		qPeerAsn := swag.FormatInt64(qrPeerAsn)
		if qPeerAsn != "" {

			if err := r.SetQueryParam("peer.asn", qPeerAsn); err != nil {
				return err
			}
		}
	}

	if o.PeerIsNextHop != nil {

		// query param peer.is_next_hop
		var qrPeerIsNextHop bool

		if o.PeerIsNextHop != nil {
			qrPeerIsNextHop = *o.PeerIsNextHop
		}
		qPeerIsNextHop := swag.FormatBool(qrPeerIsNextHop)
		if qPeerIsNextHop != "" {

			if err := r.SetQueryParam("peer.is_next_hop", qPeerIsNextHop); err != nil {
				return err
			}
		}
	}

	if o.PeerMd5Enabled != nil {

		// query param peer.md5_enabled
		var qrPeerMd5Enabled bool

		if o.PeerMd5Enabled != nil {
			qrPeerMd5Enabled = *o.PeerMd5Enabled
		}
		qPeerMd5Enabled := swag.FormatBool(qrPeerMd5Enabled)
		if qPeerMd5Enabled != "" {

			if err := r.SetQueryParam("peer.md5_enabled", qPeerMd5Enabled); err != nil {
				return err
			}
		}
	}

	if o.PeerMd5Secret != nil {

		// query param peer.md5_secret
		var qrPeerMd5Secret string

		if o.PeerMd5Secret != nil {
			qrPeerMd5Secret = *o.PeerMd5Secret
		}
		qPeerMd5Secret := qrPeerMd5Secret
		if qPeerMd5Secret != "" {

			if err := r.SetQueryParam("peer.md5_secret", qPeerMd5Secret); err != nil {
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

	if o.State != nil {

		// query param state
		var qrState string

		if o.State != nil {
			qrState = *o.State
		}
		qState := qrState
		if qState != "" {

			if err := r.SetQueryParam("state", qState); err != nil {
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