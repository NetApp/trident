// Code generated by go-swagger; DO NOT EDIT.

package s_a_n

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

// StorageUnitSnapshotModifyReader is a Reader for the StorageUnitSnapshotModify structure.
type StorageUnitSnapshotModifyReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *StorageUnitSnapshotModifyReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewStorageUnitSnapshotModifyOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	case 202:
		result := NewStorageUnitSnapshotModifyAccepted()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewStorageUnitSnapshotModifyDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewStorageUnitSnapshotModifyOK creates a StorageUnitSnapshotModifyOK with default headers values
func NewStorageUnitSnapshotModifyOK() *StorageUnitSnapshotModifyOK {
	return &StorageUnitSnapshotModifyOK{}
}

/*
StorageUnitSnapshotModifyOK describes a response with status code 200, with default header values.

OK
*/
type StorageUnitSnapshotModifyOK struct {
	Payload *models.StorageUnitSnapshotJobLinkResponse
}

// IsSuccess returns true when this storage unit snapshot modify o k response has a 2xx status code
func (o *StorageUnitSnapshotModifyOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this storage unit snapshot modify o k response has a 3xx status code
func (o *StorageUnitSnapshotModifyOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this storage unit snapshot modify o k response has a 4xx status code
func (o *StorageUnitSnapshotModifyOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this storage unit snapshot modify o k response has a 5xx status code
func (o *StorageUnitSnapshotModifyOK) IsServerError() bool {
	return false
}

// IsCode returns true when this storage unit snapshot modify o k response a status code equal to that given
func (o *StorageUnitSnapshotModifyOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the storage unit snapshot modify o k response
func (o *StorageUnitSnapshotModifyOK) Code() int {
	return 200
}

func (o *StorageUnitSnapshotModifyOK) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /storage/storage-units/{storage_unit.uuid}/snapshots/{uuid}][%d] storageUnitSnapshotModifyOK %s", 200, payload)
}

func (o *StorageUnitSnapshotModifyOK) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /storage/storage-units/{storage_unit.uuid}/snapshots/{uuid}][%d] storageUnitSnapshotModifyOK %s", 200, payload)
}

func (o *StorageUnitSnapshotModifyOK) GetPayload() *models.StorageUnitSnapshotJobLinkResponse {
	return o.Payload
}

func (o *StorageUnitSnapshotModifyOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.StorageUnitSnapshotJobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewStorageUnitSnapshotModifyAccepted creates a StorageUnitSnapshotModifyAccepted with default headers values
func NewStorageUnitSnapshotModifyAccepted() *StorageUnitSnapshotModifyAccepted {
	return &StorageUnitSnapshotModifyAccepted{}
}

/*
StorageUnitSnapshotModifyAccepted describes a response with status code 202, with default header values.

Accepted
*/
type StorageUnitSnapshotModifyAccepted struct {
	Payload *models.StorageUnitSnapshotJobLinkResponse
}

// IsSuccess returns true when this storage unit snapshot modify accepted response has a 2xx status code
func (o *StorageUnitSnapshotModifyAccepted) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this storage unit snapshot modify accepted response has a 3xx status code
func (o *StorageUnitSnapshotModifyAccepted) IsRedirect() bool {
	return false
}

// IsClientError returns true when this storage unit snapshot modify accepted response has a 4xx status code
func (o *StorageUnitSnapshotModifyAccepted) IsClientError() bool {
	return false
}

// IsServerError returns true when this storage unit snapshot modify accepted response has a 5xx status code
func (o *StorageUnitSnapshotModifyAccepted) IsServerError() bool {
	return false
}

// IsCode returns true when this storage unit snapshot modify accepted response a status code equal to that given
func (o *StorageUnitSnapshotModifyAccepted) IsCode(code int) bool {
	return code == 202
}

// Code gets the status code for the storage unit snapshot modify accepted response
func (o *StorageUnitSnapshotModifyAccepted) Code() int {
	return 202
}

func (o *StorageUnitSnapshotModifyAccepted) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /storage/storage-units/{storage_unit.uuid}/snapshots/{uuid}][%d] storageUnitSnapshotModifyAccepted %s", 202, payload)
}

func (o *StorageUnitSnapshotModifyAccepted) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /storage/storage-units/{storage_unit.uuid}/snapshots/{uuid}][%d] storageUnitSnapshotModifyAccepted %s", 202, payload)
}

func (o *StorageUnitSnapshotModifyAccepted) GetPayload() *models.StorageUnitSnapshotJobLinkResponse {
	return o.Payload
}

func (o *StorageUnitSnapshotModifyAccepted) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.StorageUnitSnapshotJobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewStorageUnitSnapshotModifyDefault creates a StorageUnitSnapshotModifyDefault with default headers values
func NewStorageUnitSnapshotModifyDefault(code int) *StorageUnitSnapshotModifyDefault {
	return &StorageUnitSnapshotModifyDefault{
		_statusCode: code,
	}
}

/*
	StorageUnitSnapshotModifyDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 524508 | The snapshot was not renamed because the name entered is not valid. |
| 1638455 | Failed to set comment for snapshot. |
| 1638476 | You cannot rename a snapshot created for use as a reference snapshot by other jobs. |
| 1638477 | User-created snapshot names cannot begin with the specified prefix. |
| 1638518 | The specified snapshot name is invalid. |
| 1638522 | Snapshots can only be renamed on read/write (RW) volumes. |
| 1638523 | Failed to set the specified SnapMirror label for the snapshot. |
| 1638524 | Adding SnapMirror labels is not allowed in a mixed version cluster. |
| 1638539 | Cannot determine the status of the snapshot rename operation for the specified volume. |
| 1638554 | Failed to set expiry time for the snapshot. |
| 1638600 | The snapshot does not exist. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type StorageUnitSnapshotModifyDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this storage unit snapshot modify default response has a 2xx status code
func (o *StorageUnitSnapshotModifyDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this storage unit snapshot modify default response has a 3xx status code
func (o *StorageUnitSnapshotModifyDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this storage unit snapshot modify default response has a 4xx status code
func (o *StorageUnitSnapshotModifyDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this storage unit snapshot modify default response has a 5xx status code
func (o *StorageUnitSnapshotModifyDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this storage unit snapshot modify default response a status code equal to that given
func (o *StorageUnitSnapshotModifyDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the storage unit snapshot modify default response
func (o *StorageUnitSnapshotModifyDefault) Code() int {
	return o._statusCode
}

func (o *StorageUnitSnapshotModifyDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /storage/storage-units/{storage_unit.uuid}/snapshots/{uuid}][%d] storage_unit_snapshot_modify default %s", o._statusCode, payload)
}

func (o *StorageUnitSnapshotModifyDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /storage/storage-units/{storage_unit.uuid}/snapshots/{uuid}][%d] storage_unit_snapshot_modify default %s", o._statusCode, payload)
}

func (o *StorageUnitSnapshotModifyDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *StorageUnitSnapshotModifyDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}