// Code generated by go-swagger; DO NOT EDIT.

package support

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

// ConfigurationBackupFileDeleteReader is a Reader for the ConfigurationBackupFileDelete structure.
type ConfigurationBackupFileDeleteReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *ConfigurationBackupFileDeleteReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewConfigurationBackupFileDeleteOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewConfigurationBackupFileDeleteDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewConfigurationBackupFileDeleteOK creates a ConfigurationBackupFileDeleteOK with default headers values
func NewConfigurationBackupFileDeleteOK() *ConfigurationBackupFileDeleteOK {
	return &ConfigurationBackupFileDeleteOK{}
}

/*
ConfigurationBackupFileDeleteOK describes a response with status code 200, with default header values.

OK
*/
type ConfigurationBackupFileDeleteOK struct {
}

// IsSuccess returns true when this configuration backup file delete o k response has a 2xx status code
func (o *ConfigurationBackupFileDeleteOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this configuration backup file delete o k response has a 3xx status code
func (o *ConfigurationBackupFileDeleteOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this configuration backup file delete o k response has a 4xx status code
func (o *ConfigurationBackupFileDeleteOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this configuration backup file delete o k response has a 5xx status code
func (o *ConfigurationBackupFileDeleteOK) IsServerError() bool {
	return false
}

// IsCode returns true when this configuration backup file delete o k response a status code equal to that given
func (o *ConfigurationBackupFileDeleteOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the configuration backup file delete o k response
func (o *ConfigurationBackupFileDeleteOK) Code() int {
	return 200
}

func (o *ConfigurationBackupFileDeleteOK) Error() string {
	return fmt.Sprintf("[DELETE /support/configuration-backup/backups/{node.uuid}/{name}][%d] configurationBackupFileDeleteOK", 200)
}

func (o *ConfigurationBackupFileDeleteOK) String() string {
	return fmt.Sprintf("[DELETE /support/configuration-backup/backups/{node.uuid}/{name}][%d] configurationBackupFileDeleteOK", 200)
}

func (o *ConfigurationBackupFileDeleteOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewConfigurationBackupFileDeleteDefault creates a ConfigurationBackupFileDeleteDefault with default headers values
func NewConfigurationBackupFileDeleteDefault(code int) *ConfigurationBackupFileDeleteDefault {
	return &ConfigurationBackupFileDeleteDefault{
		_statusCode: code,
	}
}

/*
	ConfigurationBackupFileDeleteDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 5963826 | Failed to delete backup file. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type ConfigurationBackupFileDeleteDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this configuration backup file delete default response has a 2xx status code
func (o *ConfigurationBackupFileDeleteDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this configuration backup file delete default response has a 3xx status code
func (o *ConfigurationBackupFileDeleteDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this configuration backup file delete default response has a 4xx status code
func (o *ConfigurationBackupFileDeleteDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this configuration backup file delete default response has a 5xx status code
func (o *ConfigurationBackupFileDeleteDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this configuration backup file delete default response a status code equal to that given
func (o *ConfigurationBackupFileDeleteDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the configuration backup file delete default response
func (o *ConfigurationBackupFileDeleteDefault) Code() int {
	return o._statusCode
}

func (o *ConfigurationBackupFileDeleteDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /support/configuration-backup/backups/{node.uuid}/{name}][%d] configuration_backup_file_delete default %s", o._statusCode, payload)
}

func (o *ConfigurationBackupFileDeleteDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /support/configuration-backup/backups/{node.uuid}/{name}][%d] configuration_backup_file_delete default %s", o._statusCode, payload)
}

func (o *ConfigurationBackupFileDeleteDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *ConfigurationBackupFileDeleteDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
