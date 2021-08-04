// Code generated by go-swagger; DO NOT EDIT.

package storage

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"
	"io"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// SnapshotGetReader is a Reader for the SnapshotGet structure.
type SnapshotGetReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *SnapshotGetReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewSnapshotGetOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewSnapshotGetDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewSnapshotGetOK creates a SnapshotGetOK with default headers values
func NewSnapshotGetOK() *SnapshotGetOK {
	return &SnapshotGetOK{}
}

/* SnapshotGetOK describes a response with status code 200, with default header values.

OK
*/
type SnapshotGetOK struct {
	Payload *models.Snapshot
}

func (o *SnapshotGetOK) Error() string {
	return fmt.Sprintf("[GET /storage/volumes/{volume.uuid}/snapshots/{uuid}][%d] snapshotGetOK  %+v", 200, o.Payload)
}
func (o *SnapshotGetOK) GetPayload() *models.Snapshot {
	return o.Payload
}

func (o *SnapshotGetOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Snapshot)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewSnapshotGetDefault creates a SnapshotGetDefault with default headers values
func NewSnapshotGetDefault(code int) *SnapshotGetDefault {
	return &SnapshotGetDefault{
		_statusCode: code,
	}
}

/* SnapshotGetDefault describes a response with status code -1, with default header values.

Error
*/
type SnapshotGetDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// Code gets the status code for the snapshot get default response
func (o *SnapshotGetDefault) Code() int {
	return o._statusCode
}

func (o *SnapshotGetDefault) Error() string {
	return fmt.Sprintf("[GET /storage/volumes/{volume.uuid}/snapshots/{uuid}][%d] snapshot_get default  %+v", o._statusCode, o.Payload)
}
func (o *SnapshotGetDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *SnapshotGetDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}