// Code generated by go-swagger; DO NOT EDIT.

package name_services

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

// HostsSettingsModifyReader is a Reader for the HostsSettingsModify structure.
type HostsSettingsModifyReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *HostsSettingsModifyReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewHostsSettingsModifyOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewHostsSettingsModifyDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewHostsSettingsModifyOK creates a HostsSettingsModifyOK with default headers values
func NewHostsSettingsModifyOK() *HostsSettingsModifyOK {
	return &HostsSettingsModifyOK{}
}

/*
HostsSettingsModifyOK describes a response with status code 200, with default header values.

OK
*/
type HostsSettingsModifyOK struct {
}

// IsSuccess returns true when this hosts settings modify o k response has a 2xx status code
func (o *HostsSettingsModifyOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this hosts settings modify o k response has a 3xx status code
func (o *HostsSettingsModifyOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this hosts settings modify o k response has a 4xx status code
func (o *HostsSettingsModifyOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this hosts settings modify o k response has a 5xx status code
func (o *HostsSettingsModifyOK) IsServerError() bool {
	return false
}

// IsCode returns true when this hosts settings modify o k response a status code equal to that given
func (o *HostsSettingsModifyOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the hosts settings modify o k response
func (o *HostsSettingsModifyOK) Code() int {
	return 200
}

func (o *HostsSettingsModifyOK) Error() string {
	return fmt.Sprintf("[PATCH /name-services/cache/host/settings/{uuid}][%d] hostsSettingsModifyOK", 200)
}

func (o *HostsSettingsModifyOK) String() string {
	return fmt.Sprintf("[PATCH /name-services/cache/host/settings/{uuid}][%d] hostsSettingsModifyOK", 200)
}

func (o *HostsSettingsModifyOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewHostsSettingsModifyDefault creates a HostsSettingsModifyDefault with default headers values
func NewHostsSettingsModifyDefault(code int) *HostsSettingsModifyDefault {
	return &HostsSettingsModifyDefault{
		_statusCode: code,
	}
}

/*
	HostsSettingsModifyDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 23724055 | Internal error. Configuration for Vserver failed. Verify that the cluster is healthy, then try the command again. For further assistance, contact technical support. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type HostsSettingsModifyDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this hosts settings modify default response has a 2xx status code
func (o *HostsSettingsModifyDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this hosts settings modify default response has a 3xx status code
func (o *HostsSettingsModifyDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this hosts settings modify default response has a 4xx status code
func (o *HostsSettingsModifyDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this hosts settings modify default response has a 5xx status code
func (o *HostsSettingsModifyDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this hosts settings modify default response a status code equal to that given
func (o *HostsSettingsModifyDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the hosts settings modify default response
func (o *HostsSettingsModifyDefault) Code() int {
	return o._statusCode
}

func (o *HostsSettingsModifyDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /name-services/cache/host/settings/{uuid}][%d] hosts_settings_modify default %s", o._statusCode, payload)
}

func (o *HostsSettingsModifyDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /name-services/cache/host/settings/{uuid}][%d] hosts_settings_modify default %s", o._statusCode, payload)
}

func (o *HostsSettingsModifyDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *HostsSettingsModifyDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
