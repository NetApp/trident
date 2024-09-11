// Code generated by go-swagger; DO NOT EDIT.

package n_a_s

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

// CifsOpenFileDeleteCollectionReader is a Reader for the CifsOpenFileDeleteCollection structure.
type CifsOpenFileDeleteCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *CifsOpenFileDeleteCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewCifsOpenFileDeleteCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewCifsOpenFileDeleteCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewCifsOpenFileDeleteCollectionOK creates a CifsOpenFileDeleteCollectionOK with default headers values
func NewCifsOpenFileDeleteCollectionOK() *CifsOpenFileDeleteCollectionOK {
	return &CifsOpenFileDeleteCollectionOK{}
}

/*
CifsOpenFileDeleteCollectionOK describes a response with status code 200, with default header values.

OK
*/
type CifsOpenFileDeleteCollectionOK struct {
}

// IsSuccess returns true when this cifs open file delete collection o k response has a 2xx status code
func (o *CifsOpenFileDeleteCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this cifs open file delete collection o k response has a 3xx status code
func (o *CifsOpenFileDeleteCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this cifs open file delete collection o k response has a 4xx status code
func (o *CifsOpenFileDeleteCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this cifs open file delete collection o k response has a 5xx status code
func (o *CifsOpenFileDeleteCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this cifs open file delete collection o k response a status code equal to that given
func (o *CifsOpenFileDeleteCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the cifs open file delete collection o k response
func (o *CifsOpenFileDeleteCollectionOK) Code() int {
	return 200
}

func (o *CifsOpenFileDeleteCollectionOK) Error() string {
	return fmt.Sprintf("[DELETE /protocols/cifs/session/files][%d] cifsOpenFileDeleteCollectionOK", 200)
}

func (o *CifsOpenFileDeleteCollectionOK) String() string {
	return fmt.Sprintf("[DELETE /protocols/cifs/session/files][%d] cifsOpenFileDeleteCollectionOK", 200)
}

func (o *CifsOpenFileDeleteCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewCifsOpenFileDeleteCollectionDefault creates a CifsOpenFileDeleteCollectionDefault with default headers values
func NewCifsOpenFileDeleteCollectionDefault(code int) *CifsOpenFileDeleteCollectionDefault {
	return &CifsOpenFileDeleteCollectionDefault{
		_statusCode: code,
	}
}

/*
CifsOpenFileDeleteCollectionDefault describes a response with status code -1, with default header values.

Error
*/
type CifsOpenFileDeleteCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this cifs open file delete collection default response has a 2xx status code
func (o *CifsOpenFileDeleteCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this cifs open file delete collection default response has a 3xx status code
func (o *CifsOpenFileDeleteCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this cifs open file delete collection default response has a 4xx status code
func (o *CifsOpenFileDeleteCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this cifs open file delete collection default response has a 5xx status code
func (o *CifsOpenFileDeleteCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this cifs open file delete collection default response a status code equal to that given
func (o *CifsOpenFileDeleteCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the cifs open file delete collection default response
func (o *CifsOpenFileDeleteCollectionDefault) Code() int {
	return o._statusCode
}

func (o *CifsOpenFileDeleteCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/cifs/session/files][%d] cifs_open_file_delete_collection default %s", o._statusCode, payload)
}

func (o *CifsOpenFileDeleteCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/cifs/session/files][%d] cifs_open_file_delete_collection default %s", o._statusCode, payload)
}

func (o *CifsOpenFileDeleteCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *CifsOpenFileDeleteCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
CifsOpenFileDeleteCollectionBody cifs open file delete collection body
swagger:model CifsOpenFileDeleteCollectionBody
*/
type CifsOpenFileDeleteCollectionBody struct {

	// cifs open file response inline records
	CifsOpenFileResponseInlineRecords []*models.CifsOpenFile `json:"records,omitempty"`
}

// Validate validates this cifs open file delete collection body
func (o *CifsOpenFileDeleteCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateCifsOpenFileResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *CifsOpenFileDeleteCollectionBody) validateCifsOpenFileResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.CifsOpenFileResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.CifsOpenFileResponseInlineRecords); i++ {
		if swag.IsZero(o.CifsOpenFileResponseInlineRecords[i]) { // not required
			continue
		}

		if o.CifsOpenFileResponseInlineRecords[i] != nil {
			if err := o.CifsOpenFileResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this cifs open file delete collection body based on the context it is used
func (o *CifsOpenFileDeleteCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateCifsOpenFileResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *CifsOpenFileDeleteCollectionBody) contextValidateCifsOpenFileResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.CifsOpenFileResponseInlineRecords); i++ {

		if o.CifsOpenFileResponseInlineRecords[i] != nil {
			if err := o.CifsOpenFileResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
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
func (o *CifsOpenFileDeleteCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *CifsOpenFileDeleteCollectionBody) UnmarshalBinary(b []byte) error {
	var res CifsOpenFileDeleteCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}