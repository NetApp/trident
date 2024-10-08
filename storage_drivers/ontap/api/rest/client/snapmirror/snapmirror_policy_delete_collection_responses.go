// Code generated by go-swagger; DO NOT EDIT.

package snapmirror

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

// SnapmirrorPolicyDeleteCollectionReader is a Reader for the SnapmirrorPolicyDeleteCollection structure.
type SnapmirrorPolicyDeleteCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *SnapmirrorPolicyDeleteCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewSnapmirrorPolicyDeleteCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	case 202:
		result := NewSnapmirrorPolicyDeleteCollectionAccepted()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewSnapmirrorPolicyDeleteCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewSnapmirrorPolicyDeleteCollectionOK creates a SnapmirrorPolicyDeleteCollectionOK with default headers values
func NewSnapmirrorPolicyDeleteCollectionOK() *SnapmirrorPolicyDeleteCollectionOK {
	return &SnapmirrorPolicyDeleteCollectionOK{}
}

/*
SnapmirrorPolicyDeleteCollectionOK describes a response with status code 200, with default header values.

OK
*/
type SnapmirrorPolicyDeleteCollectionOK struct {
	Payload *models.SnapmirrorPolicyJobLinkResponse
}

// IsSuccess returns true when this snapmirror policy delete collection o k response has a 2xx status code
func (o *SnapmirrorPolicyDeleteCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this snapmirror policy delete collection o k response has a 3xx status code
func (o *SnapmirrorPolicyDeleteCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this snapmirror policy delete collection o k response has a 4xx status code
func (o *SnapmirrorPolicyDeleteCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this snapmirror policy delete collection o k response has a 5xx status code
func (o *SnapmirrorPolicyDeleteCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this snapmirror policy delete collection o k response a status code equal to that given
func (o *SnapmirrorPolicyDeleteCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the snapmirror policy delete collection o k response
func (o *SnapmirrorPolicyDeleteCollectionOK) Code() int {
	return 200
}

func (o *SnapmirrorPolicyDeleteCollectionOK) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /snapmirror/policies][%d] snapmirrorPolicyDeleteCollectionOK %s", 200, payload)
}

func (o *SnapmirrorPolicyDeleteCollectionOK) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /snapmirror/policies][%d] snapmirrorPolicyDeleteCollectionOK %s", 200, payload)
}

func (o *SnapmirrorPolicyDeleteCollectionOK) GetPayload() *models.SnapmirrorPolicyJobLinkResponse {
	return o.Payload
}

func (o *SnapmirrorPolicyDeleteCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.SnapmirrorPolicyJobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewSnapmirrorPolicyDeleteCollectionAccepted creates a SnapmirrorPolicyDeleteCollectionAccepted with default headers values
func NewSnapmirrorPolicyDeleteCollectionAccepted() *SnapmirrorPolicyDeleteCollectionAccepted {
	return &SnapmirrorPolicyDeleteCollectionAccepted{}
}

/*
SnapmirrorPolicyDeleteCollectionAccepted describes a response with status code 202, with default header values.

Accepted
*/
type SnapmirrorPolicyDeleteCollectionAccepted struct {
	Payload *models.SnapmirrorPolicyJobLinkResponse
}

// IsSuccess returns true when this snapmirror policy delete collection accepted response has a 2xx status code
func (o *SnapmirrorPolicyDeleteCollectionAccepted) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this snapmirror policy delete collection accepted response has a 3xx status code
func (o *SnapmirrorPolicyDeleteCollectionAccepted) IsRedirect() bool {
	return false
}

// IsClientError returns true when this snapmirror policy delete collection accepted response has a 4xx status code
func (o *SnapmirrorPolicyDeleteCollectionAccepted) IsClientError() bool {
	return false
}

// IsServerError returns true when this snapmirror policy delete collection accepted response has a 5xx status code
func (o *SnapmirrorPolicyDeleteCollectionAccepted) IsServerError() bool {
	return false
}

// IsCode returns true when this snapmirror policy delete collection accepted response a status code equal to that given
func (o *SnapmirrorPolicyDeleteCollectionAccepted) IsCode(code int) bool {
	return code == 202
}

// Code gets the status code for the snapmirror policy delete collection accepted response
func (o *SnapmirrorPolicyDeleteCollectionAccepted) Code() int {
	return 202
}

func (o *SnapmirrorPolicyDeleteCollectionAccepted) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /snapmirror/policies][%d] snapmirrorPolicyDeleteCollectionAccepted %s", 202, payload)
}

func (o *SnapmirrorPolicyDeleteCollectionAccepted) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /snapmirror/policies][%d] snapmirrorPolicyDeleteCollectionAccepted %s", 202, payload)
}

func (o *SnapmirrorPolicyDeleteCollectionAccepted) GetPayload() *models.SnapmirrorPolicyJobLinkResponse {
	return o.Payload
}

func (o *SnapmirrorPolicyDeleteCollectionAccepted) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.SnapmirrorPolicyJobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewSnapmirrorPolicyDeleteCollectionDefault creates a SnapmirrorPolicyDeleteCollectionDefault with default headers values
func NewSnapmirrorPolicyDeleteCollectionDefault(code int) *SnapmirrorPolicyDeleteCollectionDefault {
	return &SnapmirrorPolicyDeleteCollectionDefault{
		_statusCode: code,
	}
}

/*
SnapmirrorPolicyDeleteCollectionDefault describes a response with status code -1, with default header values.

Error
*/
type SnapmirrorPolicyDeleteCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this snapmirror policy delete collection default response has a 2xx status code
func (o *SnapmirrorPolicyDeleteCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this snapmirror policy delete collection default response has a 3xx status code
func (o *SnapmirrorPolicyDeleteCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this snapmirror policy delete collection default response has a 4xx status code
func (o *SnapmirrorPolicyDeleteCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this snapmirror policy delete collection default response has a 5xx status code
func (o *SnapmirrorPolicyDeleteCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this snapmirror policy delete collection default response a status code equal to that given
func (o *SnapmirrorPolicyDeleteCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the snapmirror policy delete collection default response
func (o *SnapmirrorPolicyDeleteCollectionDefault) Code() int {
	return o._statusCode
}

func (o *SnapmirrorPolicyDeleteCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /snapmirror/policies][%d] snapmirror_policy_delete_collection default %s", o._statusCode, payload)
}

func (o *SnapmirrorPolicyDeleteCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /snapmirror/policies][%d] snapmirror_policy_delete_collection default %s", o._statusCode, payload)
}

func (o *SnapmirrorPolicyDeleteCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *SnapmirrorPolicyDeleteCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
SnapmirrorPolicyDeleteCollectionBody snapmirror policy delete collection body
swagger:model SnapmirrorPolicyDeleteCollectionBody
*/
type SnapmirrorPolicyDeleteCollectionBody struct {

	// snapmirror policy response inline records
	SnapmirrorPolicyResponseInlineRecords []*models.SnapmirrorPolicy `json:"records,omitempty"`
}

// Validate validates this snapmirror policy delete collection body
func (o *SnapmirrorPolicyDeleteCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSnapmirrorPolicyResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapmirrorPolicyDeleteCollectionBody) validateSnapmirrorPolicyResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.SnapmirrorPolicyResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.SnapmirrorPolicyResponseInlineRecords); i++ {
		if swag.IsZero(o.SnapmirrorPolicyResponseInlineRecords[i]) { // not required
			continue
		}

		if o.SnapmirrorPolicyResponseInlineRecords[i] != nil {
			if err := o.SnapmirrorPolicyResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this snapmirror policy delete collection body based on the context it is used
func (o *SnapmirrorPolicyDeleteCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSnapmirrorPolicyResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *SnapmirrorPolicyDeleteCollectionBody) contextValidateSnapmirrorPolicyResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.SnapmirrorPolicyResponseInlineRecords); i++ {

		if o.SnapmirrorPolicyResponseInlineRecords[i] != nil {
			if err := o.SnapmirrorPolicyResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
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
func (o *SnapmirrorPolicyDeleteCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *SnapmirrorPolicyDeleteCollectionBody) UnmarshalBinary(b []byte) error {
	var res SnapmirrorPolicyDeleteCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
