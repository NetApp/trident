// Code generated by go-swagger; DO NOT EDIT.

package n_v_me

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

// NvmeSubsystemMapDeleteCollectionReader is a Reader for the NvmeSubsystemMapDeleteCollection structure.
type NvmeSubsystemMapDeleteCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *NvmeSubsystemMapDeleteCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewNvmeSubsystemMapDeleteCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewNvmeSubsystemMapDeleteCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewNvmeSubsystemMapDeleteCollectionOK creates a NvmeSubsystemMapDeleteCollectionOK with default headers values
func NewNvmeSubsystemMapDeleteCollectionOK() *NvmeSubsystemMapDeleteCollectionOK {
	return &NvmeSubsystemMapDeleteCollectionOK{}
}

/*
NvmeSubsystemMapDeleteCollectionOK describes a response with status code 200, with default header values.

OK
*/
type NvmeSubsystemMapDeleteCollectionOK struct {
	Payload *models.NvmeSubsystemMapResponse
}

// IsSuccess returns true when this nvme subsystem map delete collection o k response has a 2xx status code
func (o *NvmeSubsystemMapDeleteCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this nvme subsystem map delete collection o k response has a 3xx status code
func (o *NvmeSubsystemMapDeleteCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this nvme subsystem map delete collection o k response has a 4xx status code
func (o *NvmeSubsystemMapDeleteCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this nvme subsystem map delete collection o k response has a 5xx status code
func (o *NvmeSubsystemMapDeleteCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this nvme subsystem map delete collection o k response a status code equal to that given
func (o *NvmeSubsystemMapDeleteCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the nvme subsystem map delete collection o k response
func (o *NvmeSubsystemMapDeleteCollectionOK) Code() int {
	return 200
}

func (o *NvmeSubsystemMapDeleteCollectionOK) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/nvme/subsystem-maps][%d] nvmeSubsystemMapDeleteCollectionOK %s", 200, payload)
}

func (o *NvmeSubsystemMapDeleteCollectionOK) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/nvme/subsystem-maps][%d] nvmeSubsystemMapDeleteCollectionOK %s", 200, payload)
}

func (o *NvmeSubsystemMapDeleteCollectionOK) GetPayload() *models.NvmeSubsystemMapResponse {
	return o.Payload
}

func (o *NvmeSubsystemMapDeleteCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.NvmeSubsystemMapResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewNvmeSubsystemMapDeleteCollectionDefault creates a NvmeSubsystemMapDeleteCollectionDefault with default headers values
func NewNvmeSubsystemMapDeleteCollectionDefault(code int) *NvmeSubsystemMapDeleteCollectionDefault {
	return &NvmeSubsystemMapDeleteCollectionDefault{
		_statusCode: code,
	}
}

/*
	NvmeSubsystemMapDeleteCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 72090019 | The specified NVMe namespace is not mapped to the specified NVMe subsystem. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type NvmeSubsystemMapDeleteCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this nvme subsystem map delete collection default response has a 2xx status code
func (o *NvmeSubsystemMapDeleteCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this nvme subsystem map delete collection default response has a 3xx status code
func (o *NvmeSubsystemMapDeleteCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this nvme subsystem map delete collection default response has a 4xx status code
func (o *NvmeSubsystemMapDeleteCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this nvme subsystem map delete collection default response has a 5xx status code
func (o *NvmeSubsystemMapDeleteCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this nvme subsystem map delete collection default response a status code equal to that given
func (o *NvmeSubsystemMapDeleteCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the nvme subsystem map delete collection default response
func (o *NvmeSubsystemMapDeleteCollectionDefault) Code() int {
	return o._statusCode
}

func (o *NvmeSubsystemMapDeleteCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/nvme/subsystem-maps][%d] nvme_subsystem_map_delete_collection default %s", o._statusCode, payload)
}

func (o *NvmeSubsystemMapDeleteCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/nvme/subsystem-maps][%d] nvme_subsystem_map_delete_collection default %s", o._statusCode, payload)
}

func (o *NvmeSubsystemMapDeleteCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *NvmeSubsystemMapDeleteCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
NvmeSubsystemMapDeleteCollectionBody nvme subsystem map delete collection body
swagger:model NvmeSubsystemMapDeleteCollectionBody
*/
type NvmeSubsystemMapDeleteCollectionBody struct {

	// nvme subsystem map response inline records
	NvmeSubsystemMapResponseInlineRecords []*models.NvmeSubsystemMap `json:"records,omitempty"`
}

// Validate validates this nvme subsystem map delete collection body
func (o *NvmeSubsystemMapDeleteCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateNvmeSubsystemMapResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NvmeSubsystemMapDeleteCollectionBody) validateNvmeSubsystemMapResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.NvmeSubsystemMapResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.NvmeSubsystemMapResponseInlineRecords); i++ {
		if swag.IsZero(o.NvmeSubsystemMapResponseInlineRecords[i]) { // not required
			continue
		}

		if o.NvmeSubsystemMapResponseInlineRecords[i] != nil {
			if err := o.NvmeSubsystemMapResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this nvme subsystem map delete collection body based on the context it is used
func (o *NvmeSubsystemMapDeleteCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateNvmeSubsystemMapResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *NvmeSubsystemMapDeleteCollectionBody) contextValidateNvmeSubsystemMapResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.NvmeSubsystemMapResponseInlineRecords); i++ {

		if o.NvmeSubsystemMapResponseInlineRecords[i] != nil {
			if err := o.NvmeSubsystemMapResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
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
func (o *NvmeSubsystemMapDeleteCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *NvmeSubsystemMapDeleteCollectionBody) UnmarshalBinary(b []byte) error {
	var res NvmeSubsystemMapDeleteCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
