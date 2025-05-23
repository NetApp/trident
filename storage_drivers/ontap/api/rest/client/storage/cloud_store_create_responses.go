// Code generated by go-swagger; DO NOT EDIT.

package storage

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// CloudStoreCreateReader is a Reader for the CloudStoreCreate structure.
type CloudStoreCreateReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *CloudStoreCreateReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 201:
		result := NewCloudStoreCreateCreated()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	case 202:
		result := NewCloudStoreCreateAccepted()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewCloudStoreCreateDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewCloudStoreCreateCreated creates a CloudStoreCreateCreated with default headers values
func NewCloudStoreCreateCreated() *CloudStoreCreateCreated {
	return &CloudStoreCreateCreated{}
}

/*
CloudStoreCreateCreated describes a response with status code 201, with default header values.

Created
*/
type CloudStoreCreateCreated struct {

	/* Useful for tracking the resource location
	 */
	Location string

	Payload *models.CloudStoreJobLinkResponse
}

// IsSuccess returns true when this cloud store create created response has a 2xx status code
func (o *CloudStoreCreateCreated) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this cloud store create created response has a 3xx status code
func (o *CloudStoreCreateCreated) IsRedirect() bool {
	return false
}

// IsClientError returns true when this cloud store create created response has a 4xx status code
func (o *CloudStoreCreateCreated) IsClientError() bool {
	return false
}

// IsServerError returns true when this cloud store create created response has a 5xx status code
func (o *CloudStoreCreateCreated) IsServerError() bool {
	return false
}

// IsCode returns true when this cloud store create created response a status code equal to that given
func (o *CloudStoreCreateCreated) IsCode(code int) bool {
	return code == 201
}

// Code gets the status code for the cloud store create created response
func (o *CloudStoreCreateCreated) Code() int {
	return 201
}

func (o *CloudStoreCreateCreated) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[POST /storage/aggregates/{aggregate.uuid}/cloud-stores][%d] cloudStoreCreateCreated %s", 201, payload)
}

func (o *CloudStoreCreateCreated) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[POST /storage/aggregates/{aggregate.uuid}/cloud-stores][%d] cloudStoreCreateCreated %s", 201, payload)
}

func (o *CloudStoreCreateCreated) GetPayload() *models.CloudStoreJobLinkResponse {
	return o.Payload
}

func (o *CloudStoreCreateCreated) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	// hydrates response header Location
	hdrLocation := response.GetHeader("Location")

	if hdrLocation != "" {
		o.Location = hdrLocation
	}

	o.Payload = new(models.CloudStoreJobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewCloudStoreCreateAccepted creates a CloudStoreCreateAccepted with default headers values
func NewCloudStoreCreateAccepted() *CloudStoreCreateAccepted {
	return &CloudStoreCreateAccepted{}
}

/*
CloudStoreCreateAccepted describes a response with status code 202, with default header values.

Accepted
*/
type CloudStoreCreateAccepted struct {

	/* Useful for tracking the resource location
	 */
	Location string

	Payload *models.CloudStoreJobLinkResponse
}

// IsSuccess returns true when this cloud store create accepted response has a 2xx status code
func (o *CloudStoreCreateAccepted) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this cloud store create accepted response has a 3xx status code
func (o *CloudStoreCreateAccepted) IsRedirect() bool {
	return false
}

// IsClientError returns true when this cloud store create accepted response has a 4xx status code
func (o *CloudStoreCreateAccepted) IsClientError() bool {
	return false
}

// IsServerError returns true when this cloud store create accepted response has a 5xx status code
func (o *CloudStoreCreateAccepted) IsServerError() bool {
	return false
}

// IsCode returns true when this cloud store create accepted response a status code equal to that given
func (o *CloudStoreCreateAccepted) IsCode(code int) bool {
	return code == 202
}

// Code gets the status code for the cloud store create accepted response
func (o *CloudStoreCreateAccepted) Code() int {
	return 202
}

func (o *CloudStoreCreateAccepted) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[POST /storage/aggregates/{aggregate.uuid}/cloud-stores][%d] cloudStoreCreateAccepted %s", 202, payload)
}

func (o *CloudStoreCreateAccepted) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[POST /storage/aggregates/{aggregate.uuid}/cloud-stores][%d] cloudStoreCreateAccepted %s", 202, payload)
}

func (o *CloudStoreCreateAccepted) GetPayload() *models.CloudStoreJobLinkResponse {
	return o.Payload
}

func (o *CloudStoreCreateAccepted) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	// hydrates response header Location
	hdrLocation := response.GetHeader("Location")

	if hdrLocation != "" {
		o.Location = hdrLocation
	}

	o.Payload = new(models.CloudStoreJobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewCloudStoreCreateDefault creates a CloudStoreCreateDefault with default headers values
func NewCloudStoreCreateDefault(code int) *CloudStoreCreateDefault {
	return &CloudStoreCreateDefault{
		_statusCode: code,
	}
}

/*
	CloudStoreCreateDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 787143 | Object store is already attached to FabricPool. |
| 787144 | Aggregate is not a FabricPool. |
| 787205 | The operation failed because the previous unmirror operation on the aggregate is still in progress. Check that the previous unmirror operation is complete and try the command again. |
| 787218 | The specified object store does not exist. |
| 7209876 | Aggregate is already a FabricPool Aggregate. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type CloudStoreCreateDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this cloud store create default response has a 2xx status code
func (o *CloudStoreCreateDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this cloud store create default response has a 3xx status code
func (o *CloudStoreCreateDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this cloud store create default response has a 4xx status code
func (o *CloudStoreCreateDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this cloud store create default response has a 5xx status code
func (o *CloudStoreCreateDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this cloud store create default response a status code equal to that given
func (o *CloudStoreCreateDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the cloud store create default response
func (o *CloudStoreCreateDefault) Code() int {
	return o._statusCode
}

func (o *CloudStoreCreateDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[POST /storage/aggregates/{aggregate.uuid}/cloud-stores][%d] cloud_store_create default %s", o._statusCode, payload)
}

func (o *CloudStoreCreateDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[POST /storage/aggregates/{aggregate.uuid}/cloud-stores][%d] cloud_store_create default %s", o._statusCode, payload)
}

func (o *CloudStoreCreateDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *CloudStoreCreateDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
