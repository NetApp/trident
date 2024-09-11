// Code generated by go-swagger; DO NOT EDIT.

package security

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

// GroupRoleMappingsCollectionGetReader is a Reader for the GroupRoleMappingsCollectionGet structure.
type GroupRoleMappingsCollectionGetReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *GroupRoleMappingsCollectionGetReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewGroupRoleMappingsCollectionGetOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewGroupRoleMappingsCollectionGetDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewGroupRoleMappingsCollectionGetOK creates a GroupRoleMappingsCollectionGetOK with default headers values
func NewGroupRoleMappingsCollectionGetOK() *GroupRoleMappingsCollectionGetOK {
	return &GroupRoleMappingsCollectionGetOK{}
}

/*
GroupRoleMappingsCollectionGetOK describes a response with status code 200, with default header values.

OK
*/
type GroupRoleMappingsCollectionGetOK struct {
	Payload *models.GroupRoleMappingsResponse
}

// IsSuccess returns true when this group role mappings collection get o k response has a 2xx status code
func (o *GroupRoleMappingsCollectionGetOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this group role mappings collection get o k response has a 3xx status code
func (o *GroupRoleMappingsCollectionGetOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this group role mappings collection get o k response has a 4xx status code
func (o *GroupRoleMappingsCollectionGetOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this group role mappings collection get o k response has a 5xx status code
func (o *GroupRoleMappingsCollectionGetOK) IsServerError() bool {
	return false
}

// IsCode returns true when this group role mappings collection get o k response a status code equal to that given
func (o *GroupRoleMappingsCollectionGetOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the group role mappings collection get o k response
func (o *GroupRoleMappingsCollectionGetOK) Code() int {
	return 200
}

func (o *GroupRoleMappingsCollectionGetOK) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /security/group/role-mappings][%d] groupRoleMappingsCollectionGetOK %s", 200, payload)
}

func (o *GroupRoleMappingsCollectionGetOK) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /security/group/role-mappings][%d] groupRoleMappingsCollectionGetOK %s", 200, payload)
}

func (o *GroupRoleMappingsCollectionGetOK) GetPayload() *models.GroupRoleMappingsResponse {
	return o.Payload
}

func (o *GroupRoleMappingsCollectionGetOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.GroupRoleMappingsResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewGroupRoleMappingsCollectionGetDefault creates a GroupRoleMappingsCollectionGetDefault with default headers values
func NewGroupRoleMappingsCollectionGetDefault(code int) *GroupRoleMappingsCollectionGetDefault {
	return &GroupRoleMappingsCollectionGetDefault{
		_statusCode: code,
	}
}

/*
GroupRoleMappingsCollectionGetDefault describes a response with status code -1, with default header values.

Error
*/
type GroupRoleMappingsCollectionGetDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this group role mappings collection get default response has a 2xx status code
func (o *GroupRoleMappingsCollectionGetDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this group role mappings collection get default response has a 3xx status code
func (o *GroupRoleMappingsCollectionGetDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this group role mappings collection get default response has a 4xx status code
func (o *GroupRoleMappingsCollectionGetDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this group role mappings collection get default response has a 5xx status code
func (o *GroupRoleMappingsCollectionGetDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this group role mappings collection get default response a status code equal to that given
func (o *GroupRoleMappingsCollectionGetDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the group role mappings collection get default response
func (o *GroupRoleMappingsCollectionGetDefault) Code() int {
	return o._statusCode
}

func (o *GroupRoleMappingsCollectionGetDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /security/group/role-mappings][%d] group_role_mappings_collection_get default %s", o._statusCode, payload)
}

func (o *GroupRoleMappingsCollectionGetDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /security/group/role-mappings][%d] group_role_mappings_collection_get default %s", o._statusCode, payload)
}

func (o *GroupRoleMappingsCollectionGetDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *GroupRoleMappingsCollectionGetDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}