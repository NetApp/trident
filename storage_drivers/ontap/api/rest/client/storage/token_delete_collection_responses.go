// Code generated by go-swagger; DO NOT EDIT.

package storage

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

// TokenDeleteCollectionReader is a Reader for the TokenDeleteCollection structure.
type TokenDeleteCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *TokenDeleteCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewTokenDeleteCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewTokenDeleteCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewTokenDeleteCollectionOK creates a TokenDeleteCollectionOK with default headers values
func NewTokenDeleteCollectionOK() *TokenDeleteCollectionOK {
	return &TokenDeleteCollectionOK{}
}

/*
TokenDeleteCollectionOK describes a response with status code 200, with default header values.

OK
*/
type TokenDeleteCollectionOK struct {
}

// IsSuccess returns true when this token delete collection o k response has a 2xx status code
func (o *TokenDeleteCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this token delete collection o k response has a 3xx status code
func (o *TokenDeleteCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this token delete collection o k response has a 4xx status code
func (o *TokenDeleteCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this token delete collection o k response has a 5xx status code
func (o *TokenDeleteCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this token delete collection o k response a status code equal to that given
func (o *TokenDeleteCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the token delete collection o k response
func (o *TokenDeleteCollectionOK) Code() int {
	return 200
}

func (o *TokenDeleteCollectionOK) Error() string {
	return fmt.Sprintf("[DELETE /storage/file/clone/tokens][%d] tokenDeleteCollectionOK", 200)
}

func (o *TokenDeleteCollectionOK) String() string {
	return fmt.Sprintf("[DELETE /storage/file/clone/tokens][%d] tokenDeleteCollectionOK", 200)
}

func (o *TokenDeleteCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewTokenDeleteCollectionDefault creates a TokenDeleteCollectionDefault with default headers values
func NewTokenDeleteCollectionDefault(code int) *TokenDeleteCollectionDefault {
	return &TokenDeleteCollectionDefault{
		_statusCode: code,
	}
}

/*
	TokenDeleteCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 13565958 | Failed to get information about token `uuid` for node `node.name`. |
| 13565961 | Failed to delete token for node `node.name`. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type TokenDeleteCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this token delete collection default response has a 2xx status code
func (o *TokenDeleteCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this token delete collection default response has a 3xx status code
func (o *TokenDeleteCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this token delete collection default response has a 4xx status code
func (o *TokenDeleteCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this token delete collection default response has a 5xx status code
func (o *TokenDeleteCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this token delete collection default response a status code equal to that given
func (o *TokenDeleteCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the token delete collection default response
func (o *TokenDeleteCollectionDefault) Code() int {
	return o._statusCode
}

func (o *TokenDeleteCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /storage/file/clone/tokens][%d] token_delete_collection default %s", o._statusCode, payload)
}

func (o *TokenDeleteCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /storage/file/clone/tokens][%d] token_delete_collection default %s", o._statusCode, payload)
}

func (o *TokenDeleteCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *TokenDeleteCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
TokenDeleteCollectionBody token delete collection body
swagger:model TokenDeleteCollectionBody
*/
type TokenDeleteCollectionBody struct {

	// token response inline records
	TokenResponseInlineRecords []*models.Token `json:"records,omitempty"`
}

// Validate validates this token delete collection body
func (o *TokenDeleteCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateTokenResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *TokenDeleteCollectionBody) validateTokenResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.TokenResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.TokenResponseInlineRecords); i++ {
		if swag.IsZero(o.TokenResponseInlineRecords[i]) { // not required
			continue
		}

		if o.TokenResponseInlineRecords[i] != nil {
			if err := o.TokenResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this token delete collection body based on the context it is used
func (o *TokenDeleteCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateTokenResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *TokenDeleteCollectionBody) contextValidateTokenResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.TokenResponseInlineRecords); i++ {

		if o.TokenResponseInlineRecords[i] != nil {
			if err := o.TokenResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
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
func (o *TokenDeleteCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *TokenDeleteCollectionBody) UnmarshalBinary(b []byte) error {
	var res TokenDeleteCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}