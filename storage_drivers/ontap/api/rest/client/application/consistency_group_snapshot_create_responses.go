// Code generated by go-swagger; DO NOT EDIT.

package application

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"
	"io"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// ConsistencyGroupSnapshotCreateReader is a Reader for the ConsistencyGroupSnapshotCreate structure.
type ConsistencyGroupSnapshotCreateReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *ConsistencyGroupSnapshotCreateReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 201:
		result := NewConsistencyGroupSnapshotCreateCreated()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	case 202:
		result := NewConsistencyGroupSnapshotCreateAccepted()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewConsistencyGroupSnapshotCreateDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewConsistencyGroupSnapshotCreateCreated creates a ConsistencyGroupSnapshotCreateCreated with default headers values
func NewConsistencyGroupSnapshotCreateCreated() *ConsistencyGroupSnapshotCreateCreated {
	return &ConsistencyGroupSnapshotCreateCreated{}
}

/* ConsistencyGroupSnapshotCreateCreated describes a response with status code 201, with default header values.

Created
*/
type ConsistencyGroupSnapshotCreateCreated struct {
	Payload *models.ConsistencyGroupSnapshotResponse
}

func (o *ConsistencyGroupSnapshotCreateCreated) Error() string {
	return fmt.Sprintf("[POST /application/consistency-groups/{consistency_group.uuid}/snapshots][%d] consistencyGroupSnapshotCreateCreated  %+v", 201, o.Payload)
}
func (o *ConsistencyGroupSnapshotCreateCreated) GetPayload() *models.ConsistencyGroupSnapshotResponse {
	return o.Payload
}

func (o *ConsistencyGroupSnapshotCreateCreated) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ConsistencyGroupSnapshotResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewConsistencyGroupSnapshotCreateAccepted creates a ConsistencyGroupSnapshotCreateAccepted with default headers values
func NewConsistencyGroupSnapshotCreateAccepted() *ConsistencyGroupSnapshotCreateAccepted {
	return &ConsistencyGroupSnapshotCreateAccepted{}
}

/* ConsistencyGroupSnapshotCreateAccepted describes a response with status code 202, with default header values.

Accepted
*/
type ConsistencyGroupSnapshotCreateAccepted struct {
	Payload *models.JobLinkResponse
}

func (o *ConsistencyGroupSnapshotCreateAccepted) Error() string {
	return fmt.Sprintf("[POST /application/consistency-groups/{consistency_group.uuid}/snapshots][%d] consistencyGroupSnapshotCreateAccepted  %+v", 202, o.Payload)
}
func (o *ConsistencyGroupSnapshotCreateAccepted) GetPayload() *models.JobLinkResponse {
	return o.Payload
}

func (o *ConsistencyGroupSnapshotCreateAccepted) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.JobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewConsistencyGroupSnapshotCreateDefault creates a ConsistencyGroupSnapshotCreateDefault with default headers values
func NewConsistencyGroupSnapshotCreateDefault(code int) *ConsistencyGroupSnapshotCreateDefault {
	return &ConsistencyGroupSnapshotCreateDefault{
		_statusCode: code,
	}
}

/* ConsistencyGroupSnapshotCreateDefault describes a response with status code -1, with default header values.

Error
*/
type ConsistencyGroupSnapshotCreateDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// Code gets the status code for the consistency group snapshot create default response
func (o *ConsistencyGroupSnapshotCreateDefault) Code() int {
	return o._statusCode
}

func (o *ConsistencyGroupSnapshotCreateDefault) Error() string {
	return fmt.Sprintf("[POST /application/consistency-groups/{consistency_group.uuid}/snapshots][%d] consistency_group_snapshot_create default  %+v", o._statusCode, o.Payload)
}
func (o *ConsistencyGroupSnapshotCreateDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *ConsistencyGroupSnapshotCreateDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}