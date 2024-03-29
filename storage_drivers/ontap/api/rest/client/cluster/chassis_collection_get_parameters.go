// Code generated by go-swagger; DO NOT EDIT.

package cluster

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

// NewChassisCollectionGetParams creates a new ChassisCollectionGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewChassisCollectionGetParams() *ChassisCollectionGetParams {
	return &ChassisCollectionGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewChassisCollectionGetParamsWithTimeout creates a new ChassisCollectionGetParams object
// with the ability to set a timeout on a request.
func NewChassisCollectionGetParamsWithTimeout(timeout time.Duration) *ChassisCollectionGetParams {
	return &ChassisCollectionGetParams{
		timeout: timeout,
	}
}

// NewChassisCollectionGetParamsWithContext creates a new ChassisCollectionGetParams object
// with the ability to set a context for a request.
func NewChassisCollectionGetParamsWithContext(ctx context.Context) *ChassisCollectionGetParams {
	return &ChassisCollectionGetParams{
		Context: ctx,
	}
}

// NewChassisCollectionGetParamsWithHTTPClient creates a new ChassisCollectionGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewChassisCollectionGetParamsWithHTTPClient(client *http.Client) *ChassisCollectionGetParams {
	return &ChassisCollectionGetParams{
		HTTPClient: client,
	}
}

/*
ChassisCollectionGetParams contains all the parameters to send to the API endpoint

	for the chassis collection get operation.

	Typically these are written to a http.Request.
*/
type ChassisCollectionGetParams struct {

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* FrusID.

	   Filter by frus.id
	*/
	FrusID *string

	/* FrusState.

	   Filter by frus.state
	*/
	FrusState *string

	/* FrusType.

	   Filter by frus.type
	*/
	FrusType *string

	/* ID.

	   Filter by id
	*/
	ID *string

	/* MaxRecords.

	   Limit the number of records returned.
	*/
	MaxRecords *int64

	/* NodesName.

	   Filter by nodes.name
	*/
	NodesName *string

	/* NodesPcisCardsDevice.

	   Filter by nodes.pcis.cards.device
	*/
	NodesPcisCardsDevice *string

	/* NodesPcisCardsInfo.

	   Filter by nodes.pcis.cards.info
	*/
	NodesPcisCardsInfo *string

	/* NodesPcisCardsSlot.

	   Filter by nodes.pcis.cards.slot
	*/
	NodesPcisCardsSlot *string

	/* NodesPosition.

	   Filter by nodes.position
	*/
	NodesPosition *string

	/* NodesUsbsEnabled.

	   Filter by nodes.usbs.enabled
	*/
	NodesUsbsEnabled *bool

	/* NodesUsbsPortsConnected.

	   Filter by nodes.usbs.ports.connected
	*/
	NodesUsbsPortsConnected *bool

	/* NodesUsbsSupported.

	   Filter by nodes.usbs.supported
	*/
	NodesUsbsSupported *bool

	/* NodesUUID.

	   Filter by nodes.uuid
	*/
	NodesUUID *string

	/* OrderBy.

	   Order results by specified fields and optional [asc|desc] direction. Default direction is 'asc' for ascending.
	*/
	OrderBy []string

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

	/* ShelvesUID.

	   Filter by shelves.uid
	*/
	ShelvesUID *string

	/* State.

	   Filter by state
	*/
	State *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the chassis collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *ChassisCollectionGetParams) WithDefaults() *ChassisCollectionGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the chassis collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *ChassisCollectionGetParams) SetDefaults() {
	var (
		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)
	)

	val := ChassisCollectionGetParams{
		ReturnRecords: &returnRecordsDefault,
		ReturnTimeout: &returnTimeoutDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the chassis collection get params
func (o *ChassisCollectionGetParams) WithTimeout(timeout time.Duration) *ChassisCollectionGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the chassis collection get params
func (o *ChassisCollectionGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the chassis collection get params
func (o *ChassisCollectionGetParams) WithContext(ctx context.Context) *ChassisCollectionGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the chassis collection get params
func (o *ChassisCollectionGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the chassis collection get params
func (o *ChassisCollectionGetParams) WithHTTPClient(client *http.Client) *ChassisCollectionGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the chassis collection get params
func (o *ChassisCollectionGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithFields adds the fields to the chassis collection get params
func (o *ChassisCollectionGetParams) WithFields(fields []string) *ChassisCollectionGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the chassis collection get params
func (o *ChassisCollectionGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithFrusID adds the frusID to the chassis collection get params
func (o *ChassisCollectionGetParams) WithFrusID(frusID *string) *ChassisCollectionGetParams {
	o.SetFrusID(frusID)
	return o
}

// SetFrusID adds the frusId to the chassis collection get params
func (o *ChassisCollectionGetParams) SetFrusID(frusID *string) {
	o.FrusID = frusID
}

// WithFrusState adds the frusState to the chassis collection get params
func (o *ChassisCollectionGetParams) WithFrusState(frusState *string) *ChassisCollectionGetParams {
	o.SetFrusState(frusState)
	return o
}

// SetFrusState adds the frusState to the chassis collection get params
func (o *ChassisCollectionGetParams) SetFrusState(frusState *string) {
	o.FrusState = frusState
}

// WithFrusType adds the frusType to the chassis collection get params
func (o *ChassisCollectionGetParams) WithFrusType(frusType *string) *ChassisCollectionGetParams {
	o.SetFrusType(frusType)
	return o
}

// SetFrusType adds the frusType to the chassis collection get params
func (o *ChassisCollectionGetParams) SetFrusType(frusType *string) {
	o.FrusType = frusType
}

// WithID adds the id to the chassis collection get params
func (o *ChassisCollectionGetParams) WithID(id *string) *ChassisCollectionGetParams {
	o.SetID(id)
	return o
}

// SetID adds the id to the chassis collection get params
func (o *ChassisCollectionGetParams) SetID(id *string) {
	o.ID = id
}

// WithMaxRecords adds the maxRecords to the chassis collection get params
func (o *ChassisCollectionGetParams) WithMaxRecords(maxRecords *int64) *ChassisCollectionGetParams {
	o.SetMaxRecords(maxRecords)
	return o
}

// SetMaxRecords adds the maxRecords to the chassis collection get params
func (o *ChassisCollectionGetParams) SetMaxRecords(maxRecords *int64) {
	o.MaxRecords = maxRecords
}

// WithNodesName adds the nodesName to the chassis collection get params
func (o *ChassisCollectionGetParams) WithNodesName(nodesName *string) *ChassisCollectionGetParams {
	o.SetNodesName(nodesName)
	return o
}

// SetNodesName adds the nodesName to the chassis collection get params
func (o *ChassisCollectionGetParams) SetNodesName(nodesName *string) {
	o.NodesName = nodesName
}

// WithNodesPcisCardsDevice adds the nodesPcisCardsDevice to the chassis collection get params
func (o *ChassisCollectionGetParams) WithNodesPcisCardsDevice(nodesPcisCardsDevice *string) *ChassisCollectionGetParams {
	o.SetNodesPcisCardsDevice(nodesPcisCardsDevice)
	return o
}

// SetNodesPcisCardsDevice adds the nodesPcisCardsDevice to the chassis collection get params
func (o *ChassisCollectionGetParams) SetNodesPcisCardsDevice(nodesPcisCardsDevice *string) {
	o.NodesPcisCardsDevice = nodesPcisCardsDevice
}

// WithNodesPcisCardsInfo adds the nodesPcisCardsInfo to the chassis collection get params
func (o *ChassisCollectionGetParams) WithNodesPcisCardsInfo(nodesPcisCardsInfo *string) *ChassisCollectionGetParams {
	o.SetNodesPcisCardsInfo(nodesPcisCardsInfo)
	return o
}

// SetNodesPcisCardsInfo adds the nodesPcisCardsInfo to the chassis collection get params
func (o *ChassisCollectionGetParams) SetNodesPcisCardsInfo(nodesPcisCardsInfo *string) {
	o.NodesPcisCardsInfo = nodesPcisCardsInfo
}

// WithNodesPcisCardsSlot adds the nodesPcisCardsSlot to the chassis collection get params
func (o *ChassisCollectionGetParams) WithNodesPcisCardsSlot(nodesPcisCardsSlot *string) *ChassisCollectionGetParams {
	o.SetNodesPcisCardsSlot(nodesPcisCardsSlot)
	return o
}

// SetNodesPcisCardsSlot adds the nodesPcisCardsSlot to the chassis collection get params
func (o *ChassisCollectionGetParams) SetNodesPcisCardsSlot(nodesPcisCardsSlot *string) {
	o.NodesPcisCardsSlot = nodesPcisCardsSlot
}

// WithNodesPosition adds the nodesPosition to the chassis collection get params
func (o *ChassisCollectionGetParams) WithNodesPosition(nodesPosition *string) *ChassisCollectionGetParams {
	o.SetNodesPosition(nodesPosition)
	return o
}

// SetNodesPosition adds the nodesPosition to the chassis collection get params
func (o *ChassisCollectionGetParams) SetNodesPosition(nodesPosition *string) {
	o.NodesPosition = nodesPosition
}

// WithNodesUsbsEnabled adds the nodesUsbsEnabled to the chassis collection get params
func (o *ChassisCollectionGetParams) WithNodesUsbsEnabled(nodesUsbsEnabled *bool) *ChassisCollectionGetParams {
	o.SetNodesUsbsEnabled(nodesUsbsEnabled)
	return o
}

// SetNodesUsbsEnabled adds the nodesUsbsEnabled to the chassis collection get params
func (o *ChassisCollectionGetParams) SetNodesUsbsEnabled(nodesUsbsEnabled *bool) {
	o.NodesUsbsEnabled = nodesUsbsEnabled
}

// WithNodesUsbsPortsConnected adds the nodesUsbsPortsConnected to the chassis collection get params
func (o *ChassisCollectionGetParams) WithNodesUsbsPortsConnected(nodesUsbsPortsConnected *bool) *ChassisCollectionGetParams {
	o.SetNodesUsbsPortsConnected(nodesUsbsPortsConnected)
	return o
}

// SetNodesUsbsPortsConnected adds the nodesUsbsPortsConnected to the chassis collection get params
func (o *ChassisCollectionGetParams) SetNodesUsbsPortsConnected(nodesUsbsPortsConnected *bool) {
	o.NodesUsbsPortsConnected = nodesUsbsPortsConnected
}

// WithNodesUsbsSupported adds the nodesUsbsSupported to the chassis collection get params
func (o *ChassisCollectionGetParams) WithNodesUsbsSupported(nodesUsbsSupported *bool) *ChassisCollectionGetParams {
	o.SetNodesUsbsSupported(nodesUsbsSupported)
	return o
}

// SetNodesUsbsSupported adds the nodesUsbsSupported to the chassis collection get params
func (o *ChassisCollectionGetParams) SetNodesUsbsSupported(nodesUsbsSupported *bool) {
	o.NodesUsbsSupported = nodesUsbsSupported
}

// WithNodesUUID adds the nodesUUID to the chassis collection get params
func (o *ChassisCollectionGetParams) WithNodesUUID(nodesUUID *string) *ChassisCollectionGetParams {
	o.SetNodesUUID(nodesUUID)
	return o
}

// SetNodesUUID adds the nodesUuid to the chassis collection get params
func (o *ChassisCollectionGetParams) SetNodesUUID(nodesUUID *string) {
	o.NodesUUID = nodesUUID
}

// WithOrderBy adds the orderBy to the chassis collection get params
func (o *ChassisCollectionGetParams) WithOrderBy(orderBy []string) *ChassisCollectionGetParams {
	o.SetOrderBy(orderBy)
	return o
}

// SetOrderBy adds the orderBy to the chassis collection get params
func (o *ChassisCollectionGetParams) SetOrderBy(orderBy []string) {
	o.OrderBy = orderBy
}

// WithReturnRecords adds the returnRecords to the chassis collection get params
func (o *ChassisCollectionGetParams) WithReturnRecords(returnRecords *bool) *ChassisCollectionGetParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the chassis collection get params
func (o *ChassisCollectionGetParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the chassis collection get params
func (o *ChassisCollectionGetParams) WithReturnTimeout(returnTimeout *int64) *ChassisCollectionGetParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the chassis collection get params
func (o *ChassisCollectionGetParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithShelvesUID adds the shelvesUID to the chassis collection get params
func (o *ChassisCollectionGetParams) WithShelvesUID(shelvesUID *string) *ChassisCollectionGetParams {
	o.SetShelvesUID(shelvesUID)
	return o
}

// SetShelvesUID adds the shelvesUid to the chassis collection get params
func (o *ChassisCollectionGetParams) SetShelvesUID(shelvesUID *string) {
	o.ShelvesUID = shelvesUID
}

// WithState adds the state to the chassis collection get params
func (o *ChassisCollectionGetParams) WithState(state *string) *ChassisCollectionGetParams {
	o.SetState(state)
	return o
}

// SetState adds the state to the chassis collection get params
func (o *ChassisCollectionGetParams) SetState(state *string) {
	o.State = state
}

// WriteToRequest writes these params to a swagger request
func (o *ChassisCollectionGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.Fields != nil {

		// binding items for fields
		joinedFields := o.bindParamFields(reg)

		// query array param fields
		if err := r.SetQueryParam("fields", joinedFields...); err != nil {
			return err
		}
	}

	if o.FrusID != nil {

		// query param frus.id
		var qrFrusID string

		if o.FrusID != nil {
			qrFrusID = *o.FrusID
		}
		qFrusID := qrFrusID
		if qFrusID != "" {

			if err := r.SetQueryParam("frus.id", qFrusID); err != nil {
				return err
			}
		}
	}

	if o.FrusState != nil {

		// query param frus.state
		var qrFrusState string

		if o.FrusState != nil {
			qrFrusState = *o.FrusState
		}
		qFrusState := qrFrusState
		if qFrusState != "" {

			if err := r.SetQueryParam("frus.state", qFrusState); err != nil {
				return err
			}
		}
	}

	if o.FrusType != nil {

		// query param frus.type
		var qrFrusType string

		if o.FrusType != nil {
			qrFrusType = *o.FrusType
		}
		qFrusType := qrFrusType
		if qFrusType != "" {

			if err := r.SetQueryParam("frus.type", qFrusType); err != nil {
				return err
			}
		}
	}

	if o.ID != nil {

		// query param id
		var qrID string

		if o.ID != nil {
			qrID = *o.ID
		}
		qID := qrID
		if qID != "" {

			if err := r.SetQueryParam("id", qID); err != nil {
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

	if o.NodesName != nil {

		// query param nodes.name
		var qrNodesName string

		if o.NodesName != nil {
			qrNodesName = *o.NodesName
		}
		qNodesName := qrNodesName
		if qNodesName != "" {

			if err := r.SetQueryParam("nodes.name", qNodesName); err != nil {
				return err
			}
		}
	}

	if o.NodesPcisCardsDevice != nil {

		// query param nodes.pcis.cards.device
		var qrNodesPcisCardsDevice string

		if o.NodesPcisCardsDevice != nil {
			qrNodesPcisCardsDevice = *o.NodesPcisCardsDevice
		}
		qNodesPcisCardsDevice := qrNodesPcisCardsDevice
		if qNodesPcisCardsDevice != "" {

			if err := r.SetQueryParam("nodes.pcis.cards.device", qNodesPcisCardsDevice); err != nil {
				return err
			}
		}
	}

	if o.NodesPcisCardsInfo != nil {

		// query param nodes.pcis.cards.info
		var qrNodesPcisCardsInfo string

		if o.NodesPcisCardsInfo != nil {
			qrNodesPcisCardsInfo = *o.NodesPcisCardsInfo
		}
		qNodesPcisCardsInfo := qrNodesPcisCardsInfo
		if qNodesPcisCardsInfo != "" {

			if err := r.SetQueryParam("nodes.pcis.cards.info", qNodesPcisCardsInfo); err != nil {
				return err
			}
		}
	}

	if o.NodesPcisCardsSlot != nil {

		// query param nodes.pcis.cards.slot
		var qrNodesPcisCardsSlot string

		if o.NodesPcisCardsSlot != nil {
			qrNodesPcisCardsSlot = *o.NodesPcisCardsSlot
		}
		qNodesPcisCardsSlot := qrNodesPcisCardsSlot
		if qNodesPcisCardsSlot != "" {

			if err := r.SetQueryParam("nodes.pcis.cards.slot", qNodesPcisCardsSlot); err != nil {
				return err
			}
		}
	}

	if o.NodesPosition != nil {

		// query param nodes.position
		var qrNodesPosition string

		if o.NodesPosition != nil {
			qrNodesPosition = *o.NodesPosition
		}
		qNodesPosition := qrNodesPosition
		if qNodesPosition != "" {

			if err := r.SetQueryParam("nodes.position", qNodesPosition); err != nil {
				return err
			}
		}
	}

	if o.NodesUsbsEnabled != nil {

		// query param nodes.usbs.enabled
		var qrNodesUsbsEnabled bool

		if o.NodesUsbsEnabled != nil {
			qrNodesUsbsEnabled = *o.NodesUsbsEnabled
		}
		qNodesUsbsEnabled := swag.FormatBool(qrNodesUsbsEnabled)
		if qNodesUsbsEnabled != "" {

			if err := r.SetQueryParam("nodes.usbs.enabled", qNodesUsbsEnabled); err != nil {
				return err
			}
		}
	}

	if o.NodesUsbsPortsConnected != nil {

		// query param nodes.usbs.ports.connected
		var qrNodesUsbsPortsConnected bool

		if o.NodesUsbsPortsConnected != nil {
			qrNodesUsbsPortsConnected = *o.NodesUsbsPortsConnected
		}
		qNodesUsbsPortsConnected := swag.FormatBool(qrNodesUsbsPortsConnected)
		if qNodesUsbsPortsConnected != "" {

			if err := r.SetQueryParam("nodes.usbs.ports.connected", qNodesUsbsPortsConnected); err != nil {
				return err
			}
		}
	}

	if o.NodesUsbsSupported != nil {

		// query param nodes.usbs.supported
		var qrNodesUsbsSupported bool

		if o.NodesUsbsSupported != nil {
			qrNodesUsbsSupported = *o.NodesUsbsSupported
		}
		qNodesUsbsSupported := swag.FormatBool(qrNodesUsbsSupported)
		if qNodesUsbsSupported != "" {

			if err := r.SetQueryParam("nodes.usbs.supported", qNodesUsbsSupported); err != nil {
				return err
			}
		}
	}

	if o.NodesUUID != nil {

		// query param nodes.uuid
		var qrNodesUUID string

		if o.NodesUUID != nil {
			qrNodesUUID = *o.NodesUUID
		}
		qNodesUUID := qrNodesUUID
		if qNodesUUID != "" {

			if err := r.SetQueryParam("nodes.uuid", qNodesUUID); err != nil {
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

	if o.ShelvesUID != nil {

		// query param shelves.uid
		var qrShelvesUID string

		if o.ShelvesUID != nil {
			qrShelvesUID = *o.ShelvesUID
		}
		qShelvesUID := qrShelvesUID
		if qShelvesUID != "" {

			if err := r.SetQueryParam("shelves.uid", qShelvesUID); err != nil {
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

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamChassisCollectionGet binds the parameter fields
func (o *ChassisCollectionGetParams) bindParamFields(formats strfmt.Registry) []string {
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

// bindParamChassisCollectionGet binds the parameter order_by
func (o *ChassisCollectionGetParams) bindParamOrderBy(formats strfmt.Registry) []string {
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
