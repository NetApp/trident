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

// IPSubnetDeleteCollectionReader is a Reader for the IPSubnetDeleteCollection structure.
type IPSubnetDeleteCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *IPSubnetDeleteCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewIPSubnetDeleteCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewIPSubnetDeleteCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewIPSubnetDeleteCollectionOK creates a IPSubnetDeleteCollectionOK with default headers values
func NewIPSubnetDeleteCollectionOK() *IPSubnetDeleteCollectionOK {
	return &IPSubnetDeleteCollectionOK{}
}

/*
IPSubnetDeleteCollectionOK describes a response with status code 200, with default header values.

OK
*/
type IPSubnetDeleteCollectionOK struct {
}

// IsSuccess returns true when this ip subnet delete collection o k response has a 2xx status code
func (o *IPSubnetDeleteCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this ip subnet delete collection o k response has a 3xx status code
func (o *IPSubnetDeleteCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this ip subnet delete collection o k response has a 4xx status code
func (o *IPSubnetDeleteCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this ip subnet delete collection o k response has a 5xx status code
func (o *IPSubnetDeleteCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this ip subnet delete collection o k response a status code equal to that given
func (o *IPSubnetDeleteCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the ip subnet delete collection o k response
func (o *IPSubnetDeleteCollectionOK) Code() int {
	return 200
}

func (o *IPSubnetDeleteCollectionOK) Error() string {
	return fmt.Sprintf("[DELETE /network/ip/subnets][%d] ipSubnetDeleteCollectionOK", 200)
}

func (o *IPSubnetDeleteCollectionOK) String() string {
	return fmt.Sprintf("[DELETE /network/ip/subnets][%d] ipSubnetDeleteCollectionOK", 200)
}

func (o *IPSubnetDeleteCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewIPSubnetDeleteCollectionDefault creates a IPSubnetDeleteCollectionDefault with default headers values
func NewIPSubnetDeleteCollectionDefault(code int) *IPSubnetDeleteCollectionDefault {
	return &IPSubnetDeleteCollectionDefault{
		_statusCode: code,
	}
}

/*
	IPSubnetDeleteCollectionDefault describes a response with status code -1, with default header values.

	Fill error codes below.

ONTAP Error Response Codes
| Error Code | Description |
| ---------- | ----------- |
| 1377663 | The specified IP address range of subnet in IPspace contains an address already in use by a LIF. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type IPSubnetDeleteCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this ip subnet delete collection default response has a 2xx status code
func (o *IPSubnetDeleteCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this ip subnet delete collection default response has a 3xx status code
func (o *IPSubnetDeleteCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this ip subnet delete collection default response has a 4xx status code
func (o *IPSubnetDeleteCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this ip subnet delete collection default response has a 5xx status code
func (o *IPSubnetDeleteCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this ip subnet delete collection default response a status code equal to that given
func (o *IPSubnetDeleteCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the ip subnet delete collection default response
func (o *IPSubnetDeleteCollectionDefault) Code() int {
	return o._statusCode
}

func (o *IPSubnetDeleteCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /network/ip/subnets][%d] ip_subnet_delete_collection default %s", o._statusCode, payload)
}

func (o *IPSubnetDeleteCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /network/ip/subnets][%d] ip_subnet_delete_collection default %s", o._statusCode, payload)
}

func (o *IPSubnetDeleteCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *IPSubnetDeleteCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
IPSubnetDeleteCollectionBody IP subnet delete collection body
swagger:model IPSubnetDeleteCollectionBody
*/
type IPSubnetDeleteCollectionBody struct {

	// ip subnet response inline records
	IPSubnetResponseInlineRecords []*models.IPSubnet `json:"records,omitempty"`
}

// Validate validates this IP subnet delete collection body
func (o *IPSubnetDeleteCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateIPSubnetResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *IPSubnetDeleteCollectionBody) validateIPSubnetResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.IPSubnetResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.IPSubnetResponseInlineRecords); i++ {
		if swag.IsZero(o.IPSubnetResponseInlineRecords[i]) { // not required
			continue
		}

		if o.IPSubnetResponseInlineRecords[i] != nil {
			if err := o.IPSubnetResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this IP subnet delete collection body based on the context it is used
func (o *IPSubnetDeleteCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateIPSubnetResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *IPSubnetDeleteCollectionBody) contextValidateIPSubnetResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.IPSubnetResponseInlineRecords); i++ {

		if o.IPSubnetResponseInlineRecords[i] != nil {
			if err := o.IPSubnetResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
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
func (o *IPSubnetDeleteCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *IPSubnetDeleteCollectionBody) UnmarshalBinary(b []byte) error {
	var res IPSubnetDeleteCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
