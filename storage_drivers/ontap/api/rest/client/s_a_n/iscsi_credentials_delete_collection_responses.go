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

// IscsiCredentialsDeleteCollectionReader is a Reader for the IscsiCredentialsDeleteCollection structure.
type IscsiCredentialsDeleteCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *IscsiCredentialsDeleteCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewIscsiCredentialsDeleteCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewIscsiCredentialsDeleteCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewIscsiCredentialsDeleteCollectionOK creates a IscsiCredentialsDeleteCollectionOK with default headers values
func NewIscsiCredentialsDeleteCollectionOK() *IscsiCredentialsDeleteCollectionOK {
	return &IscsiCredentialsDeleteCollectionOK{}
}

/*
IscsiCredentialsDeleteCollectionOK describes a response with status code 200, with default header values.

OK
*/
type IscsiCredentialsDeleteCollectionOK struct {
}

// IsSuccess returns true when this iscsi credentials delete collection o k response has a 2xx status code
func (o *IscsiCredentialsDeleteCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this iscsi credentials delete collection o k response has a 3xx status code
func (o *IscsiCredentialsDeleteCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this iscsi credentials delete collection o k response has a 4xx status code
func (o *IscsiCredentialsDeleteCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this iscsi credentials delete collection o k response has a 5xx status code
func (o *IscsiCredentialsDeleteCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this iscsi credentials delete collection o k response a status code equal to that given
func (o *IscsiCredentialsDeleteCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the iscsi credentials delete collection o k response
func (o *IscsiCredentialsDeleteCollectionOK) Code() int {
	return 200
}

func (o *IscsiCredentialsDeleteCollectionOK) Error() string {
	return fmt.Sprintf("[DELETE /protocols/san/iscsi/credentials][%d] iscsiCredentialsDeleteCollectionOK", 200)
}

func (o *IscsiCredentialsDeleteCollectionOK) String() string {
	return fmt.Sprintf("[DELETE /protocols/san/iscsi/credentials][%d] iscsiCredentialsDeleteCollectionOK", 200)
}

func (o *IscsiCredentialsDeleteCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewIscsiCredentialsDeleteCollectionDefault creates a IscsiCredentialsDeleteCollectionDefault with default headers values
func NewIscsiCredentialsDeleteCollectionDefault(code int) *IscsiCredentialsDeleteCollectionDefault {
	return &IscsiCredentialsDeleteCollectionDefault{
		_statusCode: code,
	}
}

/*
	IscsiCredentialsDeleteCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 2621462 | An SVM with the specified UUID does not exist. |
| 2621706 | Both the SVM UUID and SVM name were supplied, but they do not refer to the same SVM. |
| 2621707 | No SVM was specified. Either `svm.name` or `svm.uuid` must be supplied. |
| 5374148 | The default security credential cannot be deleted for an SVM. |
| 5374895 | The iSCSI security credential does not exist on the specified SVM. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type IscsiCredentialsDeleteCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this iscsi credentials delete collection default response has a 2xx status code
func (o *IscsiCredentialsDeleteCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this iscsi credentials delete collection default response has a 3xx status code
func (o *IscsiCredentialsDeleteCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this iscsi credentials delete collection default response has a 4xx status code
func (o *IscsiCredentialsDeleteCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this iscsi credentials delete collection default response has a 5xx status code
func (o *IscsiCredentialsDeleteCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this iscsi credentials delete collection default response a status code equal to that given
func (o *IscsiCredentialsDeleteCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the iscsi credentials delete collection default response
func (o *IscsiCredentialsDeleteCollectionDefault) Code() int {
	return o._statusCode
}

func (o *IscsiCredentialsDeleteCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/san/iscsi/credentials][%d] iscsi_credentials_delete_collection default %s", o._statusCode, payload)
}

func (o *IscsiCredentialsDeleteCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/san/iscsi/credentials][%d] iscsi_credentials_delete_collection default %s", o._statusCode, payload)
}

func (o *IscsiCredentialsDeleteCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *IscsiCredentialsDeleteCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
IscsiCredentialsDeleteCollectionBody iscsi credentials delete collection body
swagger:model IscsiCredentialsDeleteCollectionBody
*/
type IscsiCredentialsDeleteCollectionBody struct {

	// iscsi credentials response inline records
	IscsiCredentialsResponseInlineRecords []*models.IscsiCredentials `json:"records,omitempty"`
}

// Validate validates this iscsi credentials delete collection body
func (o *IscsiCredentialsDeleteCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateIscsiCredentialsResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *IscsiCredentialsDeleteCollectionBody) validateIscsiCredentialsResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.IscsiCredentialsResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.IscsiCredentialsResponseInlineRecords); i++ {
		if swag.IsZero(o.IscsiCredentialsResponseInlineRecords[i]) { // not required
			continue
		}

		if o.IscsiCredentialsResponseInlineRecords[i] != nil {
			if err := o.IscsiCredentialsResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this iscsi credentials delete collection body based on the context it is used
func (o *IscsiCredentialsDeleteCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateIscsiCredentialsResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *IscsiCredentialsDeleteCollectionBody) contextValidateIscsiCredentialsResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.IscsiCredentialsResponseInlineRecords); i++ {

		if o.IscsiCredentialsResponseInlineRecords[i] != nil {
			if err := o.IscsiCredentialsResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
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
func (o *IscsiCredentialsDeleteCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *IscsiCredentialsDeleteCollectionBody) UnmarshalBinary(b []byte) error {
	var res IscsiCredentialsDeleteCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}