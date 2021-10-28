// Code generated by go-swagger; DO NOT EDIT.

package name_services

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"
	"io"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// UnixGroupUserDeleteReader is a Reader for the UnixGroupUserDelete structure.
type UnixGroupUserDeleteReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *UnixGroupUserDeleteReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewUnixGroupUserDeleteOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewUnixGroupUserDeleteDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewUnixGroupUserDeleteOK creates a UnixGroupUserDeleteOK with default headers values
func NewUnixGroupUserDeleteOK() *UnixGroupUserDeleteOK {
	return &UnixGroupUserDeleteOK{}
}

/* UnixGroupUserDeleteOK describes a response with status code 200, with default header values.

OK
*/
type UnixGroupUserDeleteOK struct {
}

func (o *UnixGroupUserDeleteOK) Error() string {
	return fmt.Sprintf("[DELETE /name-services/unix-groups/{svm.uuid}/{unix_group.name}/users/{name}][%d] unixGroupUserDeleteOK ", 200)
}

func (o *UnixGroupUserDeleteOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewUnixGroupUserDeleteDefault creates a UnixGroupUserDeleteDefault with default headers values
func NewUnixGroupUserDeleteDefault(code int) *UnixGroupUserDeleteDefault {
	return &UnixGroupUserDeleteDefault{
		_statusCode: code,
	}
}

/* UnixGroupUserDeleteDefault describes a response with status code -1, with default header values.

 ONTAP Error Response Codes
| Error Code | Description |
| ---------- | ----------- |
| 3276897    | The specified UNIX group does not exist in the SVM. |

*/
type UnixGroupUserDeleteDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// Code gets the status code for the unix group user delete default response
func (o *UnixGroupUserDeleteDefault) Code() int {
	return o._statusCode
}

func (o *UnixGroupUserDeleteDefault) Error() string {
	return fmt.Sprintf("[DELETE /name-services/unix-groups/{svm.uuid}/{unix_group.name}/users/{name}][%d] unix_group_user_delete default  %+v", o._statusCode, o.Payload)
}
func (o *UnixGroupUserDeleteDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *UnixGroupUserDeleteDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}