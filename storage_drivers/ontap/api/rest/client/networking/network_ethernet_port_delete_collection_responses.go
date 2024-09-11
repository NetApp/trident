// Code generated by go-swagger; DO NOT EDIT.

package networking

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// NetworkEthernetPortDeleteCollectionReader is a Reader for the NetworkEthernetPortDeleteCollection structure.
type NetworkEthernetPortDeleteCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *NetworkEthernetPortDeleteCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewNetworkEthernetPortDeleteCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewNetworkEthernetPortDeleteCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewNetworkEthernetPortDeleteCollectionOK creates a NetworkEthernetPortDeleteCollectionOK with default headers values
func NewNetworkEthernetPortDeleteCollectionOK() *NetworkEthernetPortDeleteCollectionOK {
	return &NetworkEthernetPortDeleteCollectionOK{}
}

/*
NetworkEthernetPortDeleteCollectionOK describes a response with status code 200, with default header values.

OK
*/
type NetworkEthernetPortDeleteCollectionOK struct {
}

// IsSuccess returns true when this network ethernet port delete collection o k response has a 2xx status code
func (o *NetworkEthernetPortDeleteCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this network ethernet port delete collection o k response has a 3xx status code
func (o *NetworkEthernetPortDeleteCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this network ethernet port delete collection o k response has a 4xx status code
func (o *NetworkEthernetPortDeleteCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this network ethernet port delete collection o k response has a 5xx status code
func (o *NetworkEthernetPortDeleteCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this network ethernet port delete collection o k response a status code equal to that given
func (o *NetworkEthernetPortDeleteCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the network ethernet port delete collection o k response
func (o *NetworkEthernetPortDeleteCollectionOK) Code() int {
	return 200
}

func (o *NetworkEthernetPortDeleteCollectionOK) Error() string {
	return fmt.Sprintf("[DELETE /network/ethernet/ports][%d] networkEthernetPortDeleteCollectionOK", 200)
}

func (o *NetworkEthernetPortDeleteCollectionOK) String() string {
	return fmt.Sprintf("[DELETE /network/ethernet/ports][%d] networkEthernetPortDeleteCollectionOK", 200)
}

func (o *NetworkEthernetPortDeleteCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewNetworkEthernetPortDeleteCollectionDefault creates a NetworkEthernetPortDeleteCollectionDefault with default headers values
func NewNetworkEthernetPortDeleteCollectionDefault(code int) *NetworkEthernetPortDeleteCollectionDefault {
	return &NetworkEthernetPortDeleteCollectionDefault{
		_statusCode: code,
	}
}

/*
	NetworkEthernetPortDeleteCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 1376858 | Port already has an interface bound. |
| 1966189 | Port is the home port or current port of an interface. |
| 1966302 | This interface group is hosting VLAN interfaces that must be deleted before running this command. |
| 1967105 | Cannot delete a physical port. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type NetworkEthernetPortDeleteCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this network ethernet port delete collection default response has a 2xx status code
func (o *NetworkEthernetPortDeleteCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this network ethernet port delete collection default response has a 3xx status code
func (o *NetworkEthernetPortDeleteCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this network ethernet port delete collection default response has a 4xx status code
func (o *NetworkEthernetPortDeleteCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this network ethernet port delete collection default response has a 5xx status code
func (o *NetworkEthernetPortDeleteCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this network ethernet port delete collection default response a status code equal to that given
func (o *NetworkEthernetPortDeleteCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the network ethernet port delete collection default response
func (o *NetworkEthernetPortDeleteCollectionDefault) Code() int {
	return o._statusCode
}

func (o *NetworkEthernetPortDeleteCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /network/ethernet/ports][%d] network_ethernet_port_delete_collection default %s", o._statusCode, payload)
}

func (o *NetworkEthernetPortDeleteCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /network/ethernet/ports][%d] network_ethernet_port_delete_collection default %s", o._statusCode, payload)
}

func (o *NetworkEthernetPortDeleteCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *NetworkEthernetPortDeleteCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
NetworkEthernetPortDeleteCollectionBody network ethernet port delete collection body
swagger:model NetworkEthernetPortDeleteCollectionBody
*/
type NetworkEthernetPortDeleteCollectionBody struct {

	// port response inline records
	PortResponseInlineRecords []*models.Port `json:"records,omitempty"`
}

// Validate validates this network ethernet port delete collection body
func (o *NetworkEthernetPortDeleteCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validatePortResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NetworkEthernetPortDeleteCollectionBody) validatePortResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.PortResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.PortResponseInlineRecords); i++ {
		if swag.IsZero(o.PortResponseInlineRecords[i]) { // not required
			continue
		}

		if o.PortResponseInlineRecords[i] != nil {
			if err := o.PortResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this network ethernet port delete collection body based on the context it is used
func (o *NetworkEthernetPortDeleteCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidatePortResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NetworkEthernetPortDeleteCollectionBody) contextValidatePortResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.PortResponseInlineRecords); i++ {

		if o.PortResponseInlineRecords[i] != nil {
			if err := o.PortResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// MarshalBinary interface implementation
func (o *NetworkEthernetPortDeleteCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *NetworkEthernetPortDeleteCollectionBody) UnmarshalBinary(b []byte) error {
	var res NetworkEthernetPortDeleteCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}