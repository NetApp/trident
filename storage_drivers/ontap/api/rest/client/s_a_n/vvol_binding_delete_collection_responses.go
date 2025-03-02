// Code generated by go-swagger; DO NOT EDIT.

package s_a_n

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

// VvolBindingDeleteCollectionReader is a Reader for the VvolBindingDeleteCollection structure.
type VvolBindingDeleteCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *VvolBindingDeleteCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewVvolBindingDeleteCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewVvolBindingDeleteCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewVvolBindingDeleteCollectionOK creates a VvolBindingDeleteCollectionOK with default headers values
func NewVvolBindingDeleteCollectionOK() *VvolBindingDeleteCollectionOK {
	return &VvolBindingDeleteCollectionOK{}
}

/*
VvolBindingDeleteCollectionOK describes a response with status code 200, with default header values.

OK
*/
type VvolBindingDeleteCollectionOK struct {
}

// IsSuccess returns true when this vvol binding delete collection o k response has a 2xx status code
func (o *VvolBindingDeleteCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this vvol binding delete collection o k response has a 3xx status code
func (o *VvolBindingDeleteCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this vvol binding delete collection o k response has a 4xx status code
func (o *VvolBindingDeleteCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this vvol binding delete collection o k response has a 5xx status code
func (o *VvolBindingDeleteCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this vvol binding delete collection o k response a status code equal to that given
func (o *VvolBindingDeleteCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the vvol binding delete collection o k response
func (o *VvolBindingDeleteCollectionOK) Code() int {
	return 200
}

func (o *VvolBindingDeleteCollectionOK) Error() string {
	return fmt.Sprintf("[DELETE /protocols/san/vvol-bindings][%d] vvolBindingDeleteCollectionOK", 200)
}

func (o *VvolBindingDeleteCollectionOK) String() string {
	return fmt.Sprintf("[DELETE /protocols/san/vvol-bindings][%d] vvolBindingDeleteCollectionOK", 200)
}

func (o *VvolBindingDeleteCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewVvolBindingDeleteCollectionDefault creates a VvolBindingDeleteCollectionDefault with default headers values
func NewVvolBindingDeleteCollectionDefault(code int) *VvolBindingDeleteCollectionDefault {
	return &VvolBindingDeleteCollectionDefault{
		_statusCode: code,
	}
}

/*
	VvolBindingDeleteCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 5374875 | The vVol binding was not found because the protocol endpoint or vVol LUN was not found. Use to the `target` property of the error object to differentiate between the protocol endpoint LUN and the vVol LUN. |
| 5374926 | The vVol binding was not found. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type VvolBindingDeleteCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this vvol binding delete collection default response has a 2xx status code
func (o *VvolBindingDeleteCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this vvol binding delete collection default response has a 3xx status code
func (o *VvolBindingDeleteCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this vvol binding delete collection default response has a 4xx status code
func (o *VvolBindingDeleteCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this vvol binding delete collection default response has a 5xx status code
func (o *VvolBindingDeleteCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this vvol binding delete collection default response a status code equal to that given
func (o *VvolBindingDeleteCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the vvol binding delete collection default response
func (o *VvolBindingDeleteCollectionDefault) Code() int {
	return o._statusCode
}

func (o *VvolBindingDeleteCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/san/vvol-bindings][%d] vvol_binding_delete_collection default %s", o._statusCode, payload)
}

func (o *VvolBindingDeleteCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/san/vvol-bindings][%d] vvol_binding_delete_collection default %s", o._statusCode, payload)
}

func (o *VvolBindingDeleteCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *VvolBindingDeleteCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
VvolBindingDeleteCollectionBody vvol binding delete collection body
swagger:model VvolBindingDeleteCollectionBody
*/
type VvolBindingDeleteCollectionBody struct {

	// vvol binding response inline records
	VvolBindingResponseInlineRecords []*models.VvolBinding `json:"records,omitempty"`
}

// Validate validates this vvol binding delete collection body
func (o *VvolBindingDeleteCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateVvolBindingResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *VvolBindingDeleteCollectionBody) validateVvolBindingResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.VvolBindingResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.VvolBindingResponseInlineRecords); i++ {
		if swag.IsZero(o.VvolBindingResponseInlineRecords[i]) { // not required
			continue
		}

		if o.VvolBindingResponseInlineRecords[i] != nil {
			if err := o.VvolBindingResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this vvol binding delete collection body based on the context it is used
func (o *VvolBindingDeleteCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateVvolBindingResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *VvolBindingDeleteCollectionBody) contextValidateVvolBindingResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.VvolBindingResponseInlineRecords); i++ {

		if o.VvolBindingResponseInlineRecords[i] != nil {
			if err := o.VvolBindingResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
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
func (o *VvolBindingDeleteCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *VvolBindingDeleteCollectionBody) UnmarshalBinary(b []byte) error {
	var res VvolBindingDeleteCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
