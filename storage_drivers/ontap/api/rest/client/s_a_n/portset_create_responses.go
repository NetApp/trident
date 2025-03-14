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

// PortsetCreateReader is a Reader for the PortsetCreate structure.
type PortsetCreateReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *PortsetCreateReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 201:
		result := NewPortsetCreateCreated()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewPortsetCreateDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewPortsetCreateCreated creates a PortsetCreateCreated with default headers values
func NewPortsetCreateCreated() *PortsetCreateCreated {
	return &PortsetCreateCreated{}
}

/*
PortsetCreateCreated describes a response with status code 201, with default header values.

Created
*/
type PortsetCreateCreated struct {

	/* Useful for tracking the resource location
	 */
	Location string

	Payload *models.PortsetResponse
}

// IsSuccess returns true when this portset create created response has a 2xx status code
func (o *PortsetCreateCreated) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this portset create created response has a 3xx status code
func (o *PortsetCreateCreated) IsRedirect() bool {
	return false
}

// IsClientError returns true when this portset create created response has a 4xx status code
func (o *PortsetCreateCreated) IsClientError() bool {
	return false
}

// IsServerError returns true when this portset create created response has a 5xx status code
func (o *PortsetCreateCreated) IsServerError() bool {
	return false
}

// IsCode returns true when this portset create created response a status code equal to that given
func (o *PortsetCreateCreated) IsCode(code int) bool {
	return code == 201
}

// Code gets the status code for the portset create created response
func (o *PortsetCreateCreated) Code() int {
	return 201
}

func (o *PortsetCreateCreated) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[POST /protocols/san/portsets][%d] portsetCreateCreated %s", 201, payload)
}

func (o *PortsetCreateCreated) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[POST /protocols/san/portsets][%d] portsetCreateCreated %s", 201, payload)
}

func (o *PortsetCreateCreated) GetPayload() *models.PortsetResponse {
	return o.Payload
}

func (o *PortsetCreateCreated) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	// hydrates response header Location
	hdrLocation := response.GetHeader("Location")

	if hdrLocation != "" {
		o.Location = hdrLocation
	}

	o.Payload = new(models.PortsetResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewPortsetCreateDefault creates a PortsetCreateDefault with default headers values
func NewPortsetCreateDefault(code int) *PortsetCreateDefault {
	return &PortsetCreateDefault{
		_statusCode: code,
	}
}

/*
	PortsetCreateDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 1254259 | A portset with the same name already exists in the SVM. |
| 2621462 | The specified SVM does not exist. |
| 2621706 | The specified `svm.uuid` and `svm.name` do not refer to the same SVM. |
| 2621707 | No SVM was specified. Either `svm.name` or `svm.uuid` must be supplied. |
| 5373958 | The specified portset name contains invalid characters. |
| 5373982 | An invalid WWN was specified. The length is incorrect. |
| 5373983 | An invalid WWN was specified. The format is incorrect. |
| 5374905 | An invalid interfaces array element was specified. |
| 5374906 | A specified network interface was not found. |
| 5374907 | The specified network interface UUID and name don't identify the same network interface. |
| 5374914 | An attempt was made to add a network interface of an incompatible protocol to a portset. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type PortsetCreateDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this portset create default response has a 2xx status code
func (o *PortsetCreateDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this portset create default response has a 3xx status code
func (o *PortsetCreateDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this portset create default response has a 4xx status code
func (o *PortsetCreateDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this portset create default response has a 5xx status code
func (o *PortsetCreateDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this portset create default response a status code equal to that given
func (o *PortsetCreateDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the portset create default response
func (o *PortsetCreateDefault) Code() int {
	return o._statusCode
}

func (o *PortsetCreateDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[POST /protocols/san/portsets][%d] portset_create default %s", o._statusCode, payload)
}

func (o *PortsetCreateDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[POST /protocols/san/portsets][%d] portset_create default %s", o._statusCode, payload)
}

func (o *PortsetCreateDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *PortsetCreateDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
