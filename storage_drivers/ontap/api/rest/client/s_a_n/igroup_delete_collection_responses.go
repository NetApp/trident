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

// IgroupDeleteCollectionReader is a Reader for the IgroupDeleteCollection structure.
type IgroupDeleteCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *IgroupDeleteCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewIgroupDeleteCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewIgroupDeleteCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewIgroupDeleteCollectionOK creates a IgroupDeleteCollectionOK with default headers values
func NewIgroupDeleteCollectionOK() *IgroupDeleteCollectionOK {
	return &IgroupDeleteCollectionOK{}
}

/*
IgroupDeleteCollectionOK describes a response with status code 200, with default header values.

OK
*/
type IgroupDeleteCollectionOK struct {
}

// IsSuccess returns true when this igroup delete collection o k response has a 2xx status code
func (o *IgroupDeleteCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this igroup delete collection o k response has a 3xx status code
func (o *IgroupDeleteCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this igroup delete collection o k response has a 4xx status code
func (o *IgroupDeleteCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this igroup delete collection o k response has a 5xx status code
func (o *IgroupDeleteCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this igroup delete collection o k response a status code equal to that given
func (o *IgroupDeleteCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the igroup delete collection o k response
func (o *IgroupDeleteCollectionOK) Code() int {
	return 200
}

func (o *IgroupDeleteCollectionOK) Error() string {
	return fmt.Sprintf("[DELETE /protocols/san/igroups][%d] igroupDeleteCollectionOK", 200)
}

func (o *IgroupDeleteCollectionOK) String() string {
	return fmt.Sprintf("[DELETE /protocols/san/igroups][%d] igroupDeleteCollectionOK", 200)
}

func (o *IgroupDeleteCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewIgroupDeleteCollectionDefault creates a IgroupDeleteCollectionDefault with default headers values
func NewIgroupDeleteCollectionDefault(code int) *IgroupDeleteCollectionDefault {
	return &IgroupDeleteCollectionDefault{
		_statusCode: code,
	}
}

/*
	IgroupDeleteCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 1254213 | The initiator group is mapped to one or more LUNs and `allow_delete_while_mapped` has not been specified. |
| 5374760 | An error was reported by the peer cluster while deleting a replicated initiator group. The specific error will be included as a nested error. |
| 5374852 | The initiator group does not exist. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type IgroupDeleteCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this igroup delete collection default response has a 2xx status code
func (o *IgroupDeleteCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this igroup delete collection default response has a 3xx status code
func (o *IgroupDeleteCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this igroup delete collection default response has a 4xx status code
func (o *IgroupDeleteCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this igroup delete collection default response has a 5xx status code
func (o *IgroupDeleteCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this igroup delete collection default response a status code equal to that given
func (o *IgroupDeleteCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the igroup delete collection default response
func (o *IgroupDeleteCollectionDefault) Code() int {
	return o._statusCode
}

func (o *IgroupDeleteCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/san/igroups][%d] igroup_delete_collection default %s", o._statusCode, payload)
}

func (o *IgroupDeleteCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/san/igroups][%d] igroup_delete_collection default %s", o._statusCode, payload)
}

func (o *IgroupDeleteCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *IgroupDeleteCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
IgroupDeleteCollectionBody igroup delete collection body
swagger:model IgroupDeleteCollectionBody
*/
type IgroupDeleteCollectionBody struct {

	// igroup response inline records
	IgroupResponseInlineRecords []*models.Igroup `json:"records,omitempty"`
}

// Validate validates this igroup delete collection body
func (o *IgroupDeleteCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateIgroupResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *IgroupDeleteCollectionBody) validateIgroupResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.IgroupResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.IgroupResponseInlineRecords); i++ {
		if swag.IsZero(o.IgroupResponseInlineRecords[i]) { // not required
			continue
		}

		if o.IgroupResponseInlineRecords[i] != nil {
			if err := o.IgroupResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this igroup delete collection body based on the context it is used
func (o *IgroupDeleteCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateIgroupResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *IgroupDeleteCollectionBody) contextValidateIgroupResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.IgroupResponseInlineRecords); i++ {

		if o.IgroupResponseInlineRecords[i] != nil {
			if err := o.IgroupResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
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
func (o *IgroupDeleteCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *IgroupDeleteCollectionBody) UnmarshalBinary(b []byte) error {
	var res IgroupDeleteCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
