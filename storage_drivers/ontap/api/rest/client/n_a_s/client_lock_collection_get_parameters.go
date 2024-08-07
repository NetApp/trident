// Code generated by go-swagger; DO NOT EDIT.

package n_a_s

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

// NewClientLockCollectionGetParams creates a new ClientLockCollectionGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewClientLockCollectionGetParams() *ClientLockCollectionGetParams {
	return &ClientLockCollectionGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewClientLockCollectionGetParamsWithTimeout creates a new ClientLockCollectionGetParams object
// with the ability to set a timeout on a request.
func NewClientLockCollectionGetParamsWithTimeout(timeout time.Duration) *ClientLockCollectionGetParams {
	return &ClientLockCollectionGetParams{
		timeout: timeout,
	}
}

// NewClientLockCollectionGetParamsWithContext creates a new ClientLockCollectionGetParams object
// with the ability to set a context for a request.
func NewClientLockCollectionGetParamsWithContext(ctx context.Context) *ClientLockCollectionGetParams {
	return &ClientLockCollectionGetParams{
		Context: ctx,
	}
}

// NewClientLockCollectionGetParamsWithHTTPClient creates a new ClientLockCollectionGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewClientLockCollectionGetParamsWithHTTPClient(client *http.Client) *ClientLockCollectionGetParams {
	return &ClientLockCollectionGetParams{
		HTTPClient: client,
	}
}

/*
ClientLockCollectionGetParams contains all the parameters to send to the API endpoint

	for the client lock collection get operation.

	Typically these are written to a http.Request.
*/
type ClientLockCollectionGetParams struct {

	/* ByteLockExclusive.

	   Filter by byte_lock.exclusive
	*/
	ByteLockExclusive *bool

	/* ByteLockLength.

	   Filter by byte_lock.length
	*/
	ByteLockLength *int64

	/* ByteLockMandatory.

	   Filter by byte_lock.mandatory
	*/
	ByteLockMandatory *bool

	/* ByteLockOffset.

	   Filter by byte_lock.offset
	*/
	ByteLockOffset *int64

	/* ByteLockSoft.

	   Filter by byte_lock.soft
	*/
	ByteLockSoft *bool

	/* ByteLockSuper.

	   Filter by byte_lock.super
	*/
	ByteLockSuper *bool

	/* ClientAddress.

	   Filter by client_address
	*/
	ClientAddress *string

	/* Constituent.

	   Filter by constituent
	*/
	Constituent *bool

	/* Delegation.

	   Filter by delegation
	*/
	Delegation *string

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* InterfaceIPAddress.

	   Filter by interface.ip.address
	*/
	InterfaceIPAddress *string

	/* InterfaceName.

	   Filter by interface.name
	*/
	InterfaceName *string

	/* InterfaceUUID.

	   Filter by interface.uuid
	*/
	InterfaceUUID *string

	/* MaxRecords.

	   Limit the number of records returned.
	*/
	MaxRecords *int64

	/* NodeName.

	   Filter by node.name
	*/
	NodeName *string

	/* NodeUUID.

	   Filter by node.uuid
	*/
	NodeUUID *string

	/* OplockLevel.

	   Filter by oplock_level
	*/
	OplockLevel *string

	/* OrderBy.

	   Order results by specified fields and optional [asc|desc] direction. Default direction is 'asc' for ascending.
	*/
	OrderBy []string

	/* OwnerID.

	   Filter by owner_id
	*/
	OwnerID *string

	/* Path.

	   Filter by path
	*/
	Path *string

	/* Protocol.

	   Filter by protocol
	*/
	Protocol *string

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

	/* ShareLockMode.

	   Filter by share_lock.mode
	*/
	ShareLockMode *string

	/* ShareLockSoft.

	   Filter by share_lock.soft
	*/
	ShareLockSoft *bool

	/* SmbConnectState.

	   Filter by smb.connect_state
	*/
	SmbConnectState *string

	/* SmbOpenGroupID.

	   Filter by smb.open_group_id
	*/
	SmbOpenGroupID *string

	/* SmbOpenType.

	   Filter by smb.open_type
	*/
	SmbOpenType *string

	/* State.

	   Filter by state
	*/
	State *string

	/* SvmName.

	   Filter by svm.name
	*/
	SvmName *string

	/* SvmUUID.

	   Filter by svm.uuid
	*/
	SvmUUID *string

	/* Type.

	   Filter by type
	*/
	Type *string

	/* UUID.

	   Filter by uuid
	*/
	UUID *string

	/* VolumeName.

	   Filter by volume.name
	*/
	VolumeName *string

	/* VolumeUUID.

	   Filter by volume.uuid
	*/
	VolumeUUID *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the client lock collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *ClientLockCollectionGetParams) WithDefaults() *ClientLockCollectionGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the client lock collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *ClientLockCollectionGetParams) SetDefaults() {
	var (
		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)
	)

	val := ClientLockCollectionGetParams{
		ReturnRecords: &returnRecordsDefault,
		ReturnTimeout: &returnTimeoutDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithTimeout(timeout time.Duration) *ClientLockCollectionGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithContext(ctx context.Context) *ClientLockCollectionGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithHTTPClient(client *http.Client) *ClientLockCollectionGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithByteLockExclusive adds the byteLockExclusive to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithByteLockExclusive(byteLockExclusive *bool) *ClientLockCollectionGetParams {
	o.SetByteLockExclusive(byteLockExclusive)
	return o
}

// SetByteLockExclusive adds the byteLockExclusive to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetByteLockExclusive(byteLockExclusive *bool) {
	o.ByteLockExclusive = byteLockExclusive
}

// WithByteLockLength adds the byteLockLength to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithByteLockLength(byteLockLength *int64) *ClientLockCollectionGetParams {
	o.SetByteLockLength(byteLockLength)
	return o
}

// SetByteLockLength adds the byteLockLength to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetByteLockLength(byteLockLength *int64) {
	o.ByteLockLength = byteLockLength
}

// WithByteLockMandatory adds the byteLockMandatory to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithByteLockMandatory(byteLockMandatory *bool) *ClientLockCollectionGetParams {
	o.SetByteLockMandatory(byteLockMandatory)
	return o
}

// SetByteLockMandatory adds the byteLockMandatory to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetByteLockMandatory(byteLockMandatory *bool) {
	o.ByteLockMandatory = byteLockMandatory
}

// WithByteLockOffset adds the byteLockOffset to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithByteLockOffset(byteLockOffset *int64) *ClientLockCollectionGetParams {
	o.SetByteLockOffset(byteLockOffset)
	return o
}

// SetByteLockOffset adds the byteLockOffset to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetByteLockOffset(byteLockOffset *int64) {
	o.ByteLockOffset = byteLockOffset
}

// WithByteLockSoft adds the byteLockSoft to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithByteLockSoft(byteLockSoft *bool) *ClientLockCollectionGetParams {
	o.SetByteLockSoft(byteLockSoft)
	return o
}

// SetByteLockSoft adds the byteLockSoft to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetByteLockSoft(byteLockSoft *bool) {
	o.ByteLockSoft = byteLockSoft
}

// WithByteLockSuper adds the byteLockSuper to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithByteLockSuper(byteLockSuper *bool) *ClientLockCollectionGetParams {
	o.SetByteLockSuper(byteLockSuper)
	return o
}

// SetByteLockSuper adds the byteLockSuper to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetByteLockSuper(byteLockSuper *bool) {
	o.ByteLockSuper = byteLockSuper
}

// WithClientAddress adds the clientAddress to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithClientAddress(clientAddress *string) *ClientLockCollectionGetParams {
	o.SetClientAddress(clientAddress)
	return o
}

// SetClientAddress adds the clientAddress to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetClientAddress(clientAddress *string) {
	o.ClientAddress = clientAddress
}

// WithConstituent adds the constituent to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithConstituent(constituent *bool) *ClientLockCollectionGetParams {
	o.SetConstituent(constituent)
	return o
}

// SetConstituent adds the constituent to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetConstituent(constituent *bool) {
	o.Constituent = constituent
}

// WithDelegation adds the delegation to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithDelegation(delegation *string) *ClientLockCollectionGetParams {
	o.SetDelegation(delegation)
	return o
}

// SetDelegation adds the delegation to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetDelegation(delegation *string) {
	o.Delegation = delegation
}

// WithFields adds the fields to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithFields(fields []string) *ClientLockCollectionGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithInterfaceIPAddress adds the interfaceIPAddress to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithInterfaceIPAddress(interfaceIPAddress *string) *ClientLockCollectionGetParams {
	o.SetInterfaceIPAddress(interfaceIPAddress)
	return o
}

// SetInterfaceIPAddress adds the interfaceIpAddress to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetInterfaceIPAddress(interfaceIPAddress *string) {
	o.InterfaceIPAddress = interfaceIPAddress
}

// WithInterfaceName adds the interfaceName to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithInterfaceName(interfaceName *string) *ClientLockCollectionGetParams {
	o.SetInterfaceName(interfaceName)
	return o
}

// SetInterfaceName adds the interfaceName to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetInterfaceName(interfaceName *string) {
	o.InterfaceName = interfaceName
}

// WithInterfaceUUID adds the interfaceUUID to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithInterfaceUUID(interfaceUUID *string) *ClientLockCollectionGetParams {
	o.SetInterfaceUUID(interfaceUUID)
	return o
}

// SetInterfaceUUID adds the interfaceUuid to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetInterfaceUUID(interfaceUUID *string) {
	o.InterfaceUUID = interfaceUUID
}

// WithMaxRecords adds the maxRecords to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithMaxRecords(maxRecords *int64) *ClientLockCollectionGetParams {
	o.SetMaxRecords(maxRecords)
	return o
}

// SetMaxRecords adds the maxRecords to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetMaxRecords(maxRecords *int64) {
	o.MaxRecords = maxRecords
}

// WithNodeName adds the nodeName to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithNodeName(nodeName *string) *ClientLockCollectionGetParams {
	o.SetNodeName(nodeName)
	return o
}

// SetNodeName adds the nodeName to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetNodeName(nodeName *string) {
	o.NodeName = nodeName
}

// WithNodeUUID adds the nodeUUID to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithNodeUUID(nodeUUID *string) *ClientLockCollectionGetParams {
	o.SetNodeUUID(nodeUUID)
	return o
}

// SetNodeUUID adds the nodeUuid to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetNodeUUID(nodeUUID *string) {
	o.NodeUUID = nodeUUID
}

// WithOplockLevel adds the oplockLevel to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithOplockLevel(oplockLevel *string) *ClientLockCollectionGetParams {
	o.SetOplockLevel(oplockLevel)
	return o
}

// SetOplockLevel adds the oplockLevel to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetOplockLevel(oplockLevel *string) {
	o.OplockLevel = oplockLevel
}

// WithOrderBy adds the orderBy to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithOrderBy(orderBy []string) *ClientLockCollectionGetParams {
	o.SetOrderBy(orderBy)
	return o
}

// SetOrderBy adds the orderBy to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetOrderBy(orderBy []string) {
	o.OrderBy = orderBy
}

// WithOwnerID adds the ownerID to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithOwnerID(ownerID *string) *ClientLockCollectionGetParams {
	o.SetOwnerID(ownerID)
	return o
}

// SetOwnerID adds the ownerId to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetOwnerID(ownerID *string) {
	o.OwnerID = ownerID
}

// WithPath adds the path to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithPath(path *string) *ClientLockCollectionGetParams {
	o.SetPath(path)
	return o
}

// SetPath adds the path to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetPath(path *string) {
	o.Path = path
}

// WithProtocol adds the protocol to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithProtocol(protocol *string) *ClientLockCollectionGetParams {
	o.SetProtocol(protocol)
	return o
}

// SetProtocol adds the protocol to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetProtocol(protocol *string) {
	o.Protocol = protocol
}

// WithReturnRecords adds the returnRecords to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithReturnRecords(returnRecords *bool) *ClientLockCollectionGetParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithReturnTimeout(returnTimeout *int64) *ClientLockCollectionGetParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithShareLockMode adds the shareLockMode to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithShareLockMode(shareLockMode *string) *ClientLockCollectionGetParams {
	o.SetShareLockMode(shareLockMode)
	return o
}

// SetShareLockMode adds the shareLockMode to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetShareLockMode(shareLockMode *string) {
	o.ShareLockMode = shareLockMode
}

// WithShareLockSoft adds the shareLockSoft to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithShareLockSoft(shareLockSoft *bool) *ClientLockCollectionGetParams {
	o.SetShareLockSoft(shareLockSoft)
	return o
}

// SetShareLockSoft adds the shareLockSoft to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetShareLockSoft(shareLockSoft *bool) {
	o.ShareLockSoft = shareLockSoft
}

// WithSmbConnectState adds the smbConnectState to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithSmbConnectState(smbConnectState *string) *ClientLockCollectionGetParams {
	o.SetSmbConnectState(smbConnectState)
	return o
}

// SetSmbConnectState adds the smbConnectState to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetSmbConnectState(smbConnectState *string) {
	o.SmbConnectState = smbConnectState
}

// WithSmbOpenGroupID adds the smbOpenGroupID to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithSmbOpenGroupID(smbOpenGroupID *string) *ClientLockCollectionGetParams {
	o.SetSmbOpenGroupID(smbOpenGroupID)
	return o
}

// SetSmbOpenGroupID adds the smbOpenGroupId to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetSmbOpenGroupID(smbOpenGroupID *string) {
	o.SmbOpenGroupID = smbOpenGroupID
}

// WithSmbOpenType adds the smbOpenType to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithSmbOpenType(smbOpenType *string) *ClientLockCollectionGetParams {
	o.SetSmbOpenType(smbOpenType)
	return o
}

// SetSmbOpenType adds the smbOpenType to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetSmbOpenType(smbOpenType *string) {
	o.SmbOpenType = smbOpenType
}

// WithState adds the state to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithState(state *string) *ClientLockCollectionGetParams {
	o.SetState(state)
	return o
}

// SetState adds the state to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetState(state *string) {
	o.State = state
}

// WithSvmName adds the svmName to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithSvmName(svmName *string) *ClientLockCollectionGetParams {
	o.SetSvmName(svmName)
	return o
}

// SetSvmName adds the svmName to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetSvmName(svmName *string) {
	o.SvmName = svmName
}

// WithSvmUUID adds the svmUUID to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithSvmUUID(svmUUID *string) *ClientLockCollectionGetParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetSvmUUID(svmUUID *string) {
	o.SvmUUID = svmUUID
}

// WithType adds the typeVar to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithType(typeVar *string) *ClientLockCollectionGetParams {
	o.SetType(typeVar)
	return o
}

// SetType adds the type to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetType(typeVar *string) {
	o.Type = typeVar
}

// WithUUID adds the uuid to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithUUID(uuid *string) *ClientLockCollectionGetParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetUUID(uuid *string) {
	o.UUID = uuid
}

// WithVolumeName adds the volumeName to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithVolumeName(volumeName *string) *ClientLockCollectionGetParams {
	o.SetVolumeName(volumeName)
	return o
}

// SetVolumeName adds the volumeName to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetVolumeName(volumeName *string) {
	o.VolumeName = volumeName
}

// WithVolumeUUID adds the volumeUUID to the client lock collection get params
func (o *ClientLockCollectionGetParams) WithVolumeUUID(volumeUUID *string) *ClientLockCollectionGetParams {
	o.SetVolumeUUID(volumeUUID)
	return o
}

// SetVolumeUUID adds the volumeUuid to the client lock collection get params
func (o *ClientLockCollectionGetParams) SetVolumeUUID(volumeUUID *string) {
	o.VolumeUUID = volumeUUID
}

// WriteToRequest writes these params to a swagger request
func (o *ClientLockCollectionGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.ByteLockExclusive != nil {

		// query param byte_lock.exclusive
		var qrByteLockExclusive bool

		if o.ByteLockExclusive != nil {
			qrByteLockExclusive = *o.ByteLockExclusive
		}
		qByteLockExclusive := swag.FormatBool(qrByteLockExclusive)
		if qByteLockExclusive != "" {

			if err := r.SetQueryParam("byte_lock.exclusive", qByteLockExclusive); err != nil {
				return err
			}
		}
	}

	if o.ByteLockLength != nil {

		// query param byte_lock.length
		var qrByteLockLength int64

		if o.ByteLockLength != nil {
			qrByteLockLength = *o.ByteLockLength
		}
		qByteLockLength := swag.FormatInt64(qrByteLockLength)
		if qByteLockLength != "" {

			if err := r.SetQueryParam("byte_lock.length", qByteLockLength); err != nil {
				return err
			}
		}
	}

	if o.ByteLockMandatory != nil {

		// query param byte_lock.mandatory
		var qrByteLockMandatory bool

		if o.ByteLockMandatory != nil {
			qrByteLockMandatory = *o.ByteLockMandatory
		}
		qByteLockMandatory := swag.FormatBool(qrByteLockMandatory)
		if qByteLockMandatory != "" {

			if err := r.SetQueryParam("byte_lock.mandatory", qByteLockMandatory); err != nil {
				return err
			}
		}
	}

	if o.ByteLockOffset != nil {

		// query param byte_lock.offset
		var qrByteLockOffset int64

		if o.ByteLockOffset != nil {
			qrByteLockOffset = *o.ByteLockOffset
		}
		qByteLockOffset := swag.FormatInt64(qrByteLockOffset)
		if qByteLockOffset != "" {

			if err := r.SetQueryParam("byte_lock.offset", qByteLockOffset); err != nil {
				return err
			}
		}
	}

	if o.ByteLockSoft != nil {

		// query param byte_lock.soft
		var qrByteLockSoft bool

		if o.ByteLockSoft != nil {
			qrByteLockSoft = *o.ByteLockSoft
		}
		qByteLockSoft := swag.FormatBool(qrByteLockSoft)
		if qByteLockSoft != "" {

			if err := r.SetQueryParam("byte_lock.soft", qByteLockSoft); err != nil {
				return err
			}
		}
	}

	if o.ByteLockSuper != nil {

		// query param byte_lock.super
		var qrByteLockSuper bool

		if o.ByteLockSuper != nil {
			qrByteLockSuper = *o.ByteLockSuper
		}
		qByteLockSuper := swag.FormatBool(qrByteLockSuper)
		if qByteLockSuper != "" {

			if err := r.SetQueryParam("byte_lock.super", qByteLockSuper); err != nil {
				return err
			}
		}
	}

	if o.ClientAddress != nil {

		// query param client_address
		var qrClientAddress string

		if o.ClientAddress != nil {
			qrClientAddress = *o.ClientAddress
		}
		qClientAddress := qrClientAddress
		if qClientAddress != "" {

			if err := r.SetQueryParam("client_address", qClientAddress); err != nil {
				return err
			}
		}
	}

	if o.Constituent != nil {

		// query param constituent
		var qrConstituent bool

		if o.Constituent != nil {
			qrConstituent = *o.Constituent
		}
		qConstituent := swag.FormatBool(qrConstituent)
		if qConstituent != "" {

			if err := r.SetQueryParam("constituent", qConstituent); err != nil {
				return err
			}
		}
	}

	if o.Delegation != nil {

		// query param delegation
		var qrDelegation string

		if o.Delegation != nil {
			qrDelegation = *o.Delegation
		}
		qDelegation := qrDelegation
		if qDelegation != "" {

			if err := r.SetQueryParam("delegation", qDelegation); err != nil {
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

	if o.InterfaceIPAddress != nil {

		// query param interface.ip.address
		var qrInterfaceIPAddress string

		if o.InterfaceIPAddress != nil {
			qrInterfaceIPAddress = *o.InterfaceIPAddress
		}
		qInterfaceIPAddress := qrInterfaceIPAddress
		if qInterfaceIPAddress != "" {

			if err := r.SetQueryParam("interface.ip.address", qInterfaceIPAddress); err != nil {
				return err
			}
		}
	}

	if o.InterfaceName != nil {

		// query param interface.name
		var qrInterfaceName string

		if o.InterfaceName != nil {
			qrInterfaceName = *o.InterfaceName
		}
		qInterfaceName := qrInterfaceName
		if qInterfaceName != "" {

			if err := r.SetQueryParam("interface.name", qInterfaceName); err != nil {
				return err
			}
		}
	}

	if o.InterfaceUUID != nil {

		// query param interface.uuid
		var qrInterfaceUUID string

		if o.InterfaceUUID != nil {
			qrInterfaceUUID = *o.InterfaceUUID
		}
		qInterfaceUUID := qrInterfaceUUID
		if qInterfaceUUID != "" {

			if err := r.SetQueryParam("interface.uuid", qInterfaceUUID); err != nil {
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

	if o.NodeName != nil {

		// query param node.name
		var qrNodeName string

		if o.NodeName != nil {
			qrNodeName = *o.NodeName
		}
		qNodeName := qrNodeName
		if qNodeName != "" {

			if err := r.SetQueryParam("node.name", qNodeName); err != nil {
				return err
			}
		}
	}

	if o.NodeUUID != nil {

		// query param node.uuid
		var qrNodeUUID string

		if o.NodeUUID != nil {
			qrNodeUUID = *o.NodeUUID
		}
		qNodeUUID := qrNodeUUID
		if qNodeUUID != "" {

			if err := r.SetQueryParam("node.uuid", qNodeUUID); err != nil {
				return err
			}
		}
	}

	if o.OplockLevel != nil {

		// query param oplock_level
		var qrOplockLevel string

		if o.OplockLevel != nil {
			qrOplockLevel = *o.OplockLevel
		}
		qOplockLevel := qrOplockLevel
		if qOplockLevel != "" {

			if err := r.SetQueryParam("oplock_level", qOplockLevel); err != nil {
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

	if o.OwnerID != nil {

		// query param owner_id
		var qrOwnerID string

		if o.OwnerID != nil {
			qrOwnerID = *o.OwnerID
		}
		qOwnerID := qrOwnerID
		if qOwnerID != "" {

			if err := r.SetQueryParam("owner_id", qOwnerID); err != nil {
				return err
			}
		}
	}

	if o.Path != nil {

		// query param path
		var qrPath string

		if o.Path != nil {
			qrPath = *o.Path
		}
		qPath := qrPath
		if qPath != "" {

			if err := r.SetQueryParam("path", qPath); err != nil {
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

	if o.ShareLockMode != nil {

		// query param share_lock.mode
		var qrShareLockMode string

		if o.ShareLockMode != nil {
			qrShareLockMode = *o.ShareLockMode
		}
		qShareLockMode := qrShareLockMode
		if qShareLockMode != "" {

			if err := r.SetQueryParam("share_lock.mode", qShareLockMode); err != nil {
				return err
			}
		}
	}

	if o.ShareLockSoft != nil {

		// query param share_lock.soft
		var qrShareLockSoft bool

		if o.ShareLockSoft != nil {
			qrShareLockSoft = *o.ShareLockSoft
		}
		qShareLockSoft := swag.FormatBool(qrShareLockSoft)
		if qShareLockSoft != "" {

			if err := r.SetQueryParam("share_lock.soft", qShareLockSoft); err != nil {
				return err
			}
		}
	}

	if o.SmbConnectState != nil {

		// query param smb.connect_state
		var qrSmbConnectState string

		if o.SmbConnectState != nil {
			qrSmbConnectState = *o.SmbConnectState
		}
		qSmbConnectState := qrSmbConnectState
		if qSmbConnectState != "" {

			if err := r.SetQueryParam("smb.connect_state", qSmbConnectState); err != nil {
				return err
			}
		}
	}

	if o.SmbOpenGroupID != nil {

		// query param smb.open_group_id
		var qrSmbOpenGroupID string

		if o.SmbOpenGroupID != nil {
			qrSmbOpenGroupID = *o.SmbOpenGroupID
		}
		qSmbOpenGroupID := qrSmbOpenGroupID
		if qSmbOpenGroupID != "" {

			if err := r.SetQueryParam("smb.open_group_id", qSmbOpenGroupID); err != nil {
				return err
			}
		}
	}

	if o.SmbOpenType != nil {

		// query param smb.open_type
		var qrSmbOpenType string

		if o.SmbOpenType != nil {
			qrSmbOpenType = *o.SmbOpenType
		}
		qSmbOpenType := qrSmbOpenType
		if qSmbOpenType != "" {

			if err := r.SetQueryParam("smb.open_type", qSmbOpenType); err != nil {
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

	if o.Type != nil {

		// query param type
		var qrType string

		if o.Type != nil {
			qrType = *o.Type
		}
		qType := qrType
		if qType != "" {

			if err := r.SetQueryParam("type", qType); err != nil {
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

	if o.VolumeName != nil {

		// query param volume.name
		var qrVolumeName string

		if o.VolumeName != nil {
			qrVolumeName = *o.VolumeName
		}
		qVolumeName := qrVolumeName
		if qVolumeName != "" {

			if err := r.SetQueryParam("volume.name", qVolumeName); err != nil {
				return err
			}
		}
	}

	if o.VolumeUUID != nil {

		// query param volume.uuid
		var qrVolumeUUID string

		if o.VolumeUUID != nil {
			qrVolumeUUID = *o.VolumeUUID
		}
		qVolumeUUID := qrVolumeUUID
		if qVolumeUUID != "" {

			if err := r.SetQueryParam("volume.uuid", qVolumeUUID); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamClientLockCollectionGet binds the parameter fields
func (o *ClientLockCollectionGetParams) bindParamFields(formats strfmt.Registry) []string {
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

// bindParamClientLockCollectionGet binds the parameter order_by
func (o *ClientLockCollectionGetParams) bindParamOrderBy(formats strfmt.Registry) []string {
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
