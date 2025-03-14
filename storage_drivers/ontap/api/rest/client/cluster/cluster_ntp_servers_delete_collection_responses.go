// Code generated by go-swagger; DO NOT EDIT.

package cluster

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

// ClusterNtpServersDeleteCollectionReader is a Reader for the ClusterNtpServersDeleteCollection structure.
type ClusterNtpServersDeleteCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *ClusterNtpServersDeleteCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewClusterNtpServersDeleteCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	case 202:
		result := NewClusterNtpServersDeleteCollectionAccepted()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewClusterNtpServersDeleteCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewClusterNtpServersDeleteCollectionOK creates a ClusterNtpServersDeleteCollectionOK with default headers values
func NewClusterNtpServersDeleteCollectionOK() *ClusterNtpServersDeleteCollectionOK {
	return &ClusterNtpServersDeleteCollectionOK{}
}

/*
ClusterNtpServersDeleteCollectionOK describes a response with status code 200, with default header values.

OK
*/
type ClusterNtpServersDeleteCollectionOK struct {
	Payload *models.NtpServerJobLinkResponse
}

// IsSuccess returns true when this cluster ntp servers delete collection o k response has a 2xx status code
func (o *ClusterNtpServersDeleteCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this cluster ntp servers delete collection o k response has a 3xx status code
func (o *ClusterNtpServersDeleteCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this cluster ntp servers delete collection o k response has a 4xx status code
func (o *ClusterNtpServersDeleteCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this cluster ntp servers delete collection o k response has a 5xx status code
func (o *ClusterNtpServersDeleteCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this cluster ntp servers delete collection o k response a status code equal to that given
func (o *ClusterNtpServersDeleteCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the cluster ntp servers delete collection o k response
func (o *ClusterNtpServersDeleteCollectionOK) Code() int {
	return 200
}

func (o *ClusterNtpServersDeleteCollectionOK) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /cluster/ntp/servers][%d] clusterNtpServersDeleteCollectionOK %s", 200, payload)
}

func (o *ClusterNtpServersDeleteCollectionOK) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /cluster/ntp/servers][%d] clusterNtpServersDeleteCollectionOK %s", 200, payload)
}

func (o *ClusterNtpServersDeleteCollectionOK) GetPayload() *models.NtpServerJobLinkResponse {
	return o.Payload
}

func (o *ClusterNtpServersDeleteCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.NtpServerJobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewClusterNtpServersDeleteCollectionAccepted creates a ClusterNtpServersDeleteCollectionAccepted with default headers values
func NewClusterNtpServersDeleteCollectionAccepted() *ClusterNtpServersDeleteCollectionAccepted {
	return &ClusterNtpServersDeleteCollectionAccepted{}
}

/*
ClusterNtpServersDeleteCollectionAccepted describes a response with status code 202, with default header values.

Accepted
*/
type ClusterNtpServersDeleteCollectionAccepted struct {
	Payload *models.NtpServerJobLinkResponse
}

// IsSuccess returns true when this cluster ntp servers delete collection accepted response has a 2xx status code
func (o *ClusterNtpServersDeleteCollectionAccepted) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this cluster ntp servers delete collection accepted response has a 3xx status code
func (o *ClusterNtpServersDeleteCollectionAccepted) IsRedirect() bool {
	return false
}

// IsClientError returns true when this cluster ntp servers delete collection accepted response has a 4xx status code
func (o *ClusterNtpServersDeleteCollectionAccepted) IsClientError() bool {
	return false
}

// IsServerError returns true when this cluster ntp servers delete collection accepted response has a 5xx status code
func (o *ClusterNtpServersDeleteCollectionAccepted) IsServerError() bool {
	return false
}

// IsCode returns true when this cluster ntp servers delete collection accepted response a status code equal to that given
func (o *ClusterNtpServersDeleteCollectionAccepted) IsCode(code int) bool {
	return code == 202
}

// Code gets the status code for the cluster ntp servers delete collection accepted response
func (o *ClusterNtpServersDeleteCollectionAccepted) Code() int {
	return 202
}

func (o *ClusterNtpServersDeleteCollectionAccepted) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /cluster/ntp/servers][%d] clusterNtpServersDeleteCollectionAccepted %s", 202, payload)
}

func (o *ClusterNtpServersDeleteCollectionAccepted) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /cluster/ntp/servers][%d] clusterNtpServersDeleteCollectionAccepted %s", 202, payload)
}

func (o *ClusterNtpServersDeleteCollectionAccepted) GetPayload() *models.NtpServerJobLinkResponse {
	return o.Payload
}

func (o *ClusterNtpServersDeleteCollectionAccepted) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.NtpServerJobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewClusterNtpServersDeleteCollectionDefault creates a ClusterNtpServersDeleteCollectionDefault with default headers values
func NewClusterNtpServersDeleteCollectionDefault(code int) *ClusterNtpServersDeleteCollectionDefault {
	return &ClusterNtpServersDeleteCollectionDefault{
		_statusCode: code,
	}
}

/*
ClusterNtpServersDeleteCollectionDefault describes a response with status code -1, with default header values.

Error
*/
type ClusterNtpServersDeleteCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this cluster ntp servers delete collection default response has a 2xx status code
func (o *ClusterNtpServersDeleteCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this cluster ntp servers delete collection default response has a 3xx status code
func (o *ClusterNtpServersDeleteCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this cluster ntp servers delete collection default response has a 4xx status code
func (o *ClusterNtpServersDeleteCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this cluster ntp servers delete collection default response has a 5xx status code
func (o *ClusterNtpServersDeleteCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this cluster ntp servers delete collection default response a status code equal to that given
func (o *ClusterNtpServersDeleteCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the cluster ntp servers delete collection default response
func (o *ClusterNtpServersDeleteCollectionDefault) Code() int {
	return o._statusCode
}

func (o *ClusterNtpServersDeleteCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /cluster/ntp/servers][%d] cluster_ntp_servers_delete_collection default %s", o._statusCode, payload)
}

func (o *ClusterNtpServersDeleteCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /cluster/ntp/servers][%d] cluster_ntp_servers_delete_collection default %s", o._statusCode, payload)
}

func (o *ClusterNtpServersDeleteCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *ClusterNtpServersDeleteCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
ClusterNtpServersDeleteCollectionBody cluster ntp servers delete collection body
swagger:model ClusterNtpServersDeleteCollectionBody
*/
type ClusterNtpServersDeleteCollectionBody struct {

	// ntp server response inline records
	NtpServerResponseInlineRecords []*models.NtpServer `json:"records,omitempty"`
}

// Validate validates this cluster ntp servers delete collection body
func (o *ClusterNtpServersDeleteCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateNtpServerResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *ClusterNtpServersDeleteCollectionBody) validateNtpServerResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.NtpServerResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.NtpServerResponseInlineRecords); i++ {
		if swag.IsZero(o.NtpServerResponseInlineRecords[i]) { // not required
			continue
		}

		if o.NtpServerResponseInlineRecords[i] != nil {
			if err := o.NtpServerResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this cluster ntp servers delete collection body based on the context it is used
func (o *ClusterNtpServersDeleteCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateNtpServerResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *ClusterNtpServersDeleteCollectionBody) contextValidateNtpServerResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.NtpServerResponseInlineRecords); i++ {

		if o.NtpServerResponseInlineRecords[i] != nil {
			if err := o.NtpServerResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
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
func (o *ClusterNtpServersDeleteCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *ClusterNtpServersDeleteCollectionBody) UnmarshalBinary(b []byte) error {
	var res ClusterNtpServersDeleteCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
