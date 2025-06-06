// Code generated by go-swagger; DO NOT EDIT.

package name_services

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

// DNSDeleteCollectionReader is a Reader for the DNSDeleteCollection structure.
type DNSDeleteCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *DNSDeleteCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewDNSDeleteCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewDNSDeleteCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewDNSDeleteCollectionOK creates a DNSDeleteCollectionOK with default headers values
func NewDNSDeleteCollectionOK() *DNSDeleteCollectionOK {
	return &DNSDeleteCollectionOK{}
}

/*
DNSDeleteCollectionOK describes a response with status code 200, with default header values.

OK
*/
type DNSDeleteCollectionOK struct {
}

// IsSuccess returns true when this dns delete collection o k response has a 2xx status code
func (o *DNSDeleteCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this dns delete collection o k response has a 3xx status code
func (o *DNSDeleteCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this dns delete collection o k response has a 4xx status code
func (o *DNSDeleteCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this dns delete collection o k response has a 5xx status code
func (o *DNSDeleteCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this dns delete collection o k response a status code equal to that given
func (o *DNSDeleteCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the dns delete collection o k response
func (o *DNSDeleteCollectionOK) Code() int {
	return 200
}

func (o *DNSDeleteCollectionOK) Error() string {
	return fmt.Sprintf("[DELETE /name-services/dns][%d] dnsDeleteCollectionOK", 200)
}

func (o *DNSDeleteCollectionOK) String() string {
	return fmt.Sprintf("[DELETE /name-services/dns][%d] dnsDeleteCollectionOK", 200)
}

func (o *DNSDeleteCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewDNSDeleteCollectionDefault creates a DNSDeleteCollectionDefault with default headers values
func NewDNSDeleteCollectionDefault(code int) *DNSDeleteCollectionDefault {
	return &DNSDeleteCollectionDefault{
		_statusCode: code,
	}
}

/*
DNSDeleteCollectionDefault describes a response with status code -1, with default header values.

Error
*/
type DNSDeleteCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this dns delete collection default response has a 2xx status code
func (o *DNSDeleteCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this dns delete collection default response has a 3xx status code
func (o *DNSDeleteCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this dns delete collection default response has a 4xx status code
func (o *DNSDeleteCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this dns delete collection default response has a 5xx status code
func (o *DNSDeleteCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this dns delete collection default response a status code equal to that given
func (o *DNSDeleteCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the dns delete collection default response
func (o *DNSDeleteCollectionDefault) Code() int {
	return o._statusCode
}

func (o *DNSDeleteCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /name-services/dns][%d] dns_delete_collection default %s", o._statusCode, payload)
}

func (o *DNSDeleteCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /name-services/dns][%d] dns_delete_collection default %s", o._statusCode, payload)
}

func (o *DNSDeleteCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *DNSDeleteCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
DNSDeleteCollectionBody DNS delete collection body
swagger:model DNSDeleteCollectionBody
*/
type DNSDeleteCollectionBody struct {

	// dns response inline records
	DNSResponseInlineRecords []*models.DNS `json:"records,omitempty"`
}

// Validate validates this DNS delete collection body
func (o *DNSDeleteCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateDNSResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *DNSDeleteCollectionBody) validateDNSResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.DNSResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.DNSResponseInlineRecords); i++ {
		if swag.IsZero(o.DNSResponseInlineRecords[i]) { // not required
			continue
		}

		if o.DNSResponseInlineRecords[i] != nil {
			if err := o.DNSResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this DNS delete collection body based on the context it is used
func (o *DNSDeleteCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateDNSResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *DNSDeleteCollectionBody) contextValidateDNSResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.DNSResponseInlineRecords); i++ {

		if o.DNSResponseInlineRecords[i] != nil {
			if err := o.DNSResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
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
func (o *DNSDeleteCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *DNSDeleteCollectionBody) UnmarshalBinary(b []byte) error {
	var res DNSDeleteCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
