// Code generated by go-swagger; DO NOT EDIT.

package n_v_me

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

// NvmeServiceDeleteReader is a Reader for the NvmeServiceDelete structure.
type NvmeServiceDeleteReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *NvmeServiceDeleteReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewNvmeServiceDeleteOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewNvmeServiceDeleteDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewNvmeServiceDeleteOK creates a NvmeServiceDeleteOK with default headers values
func NewNvmeServiceDeleteOK() *NvmeServiceDeleteOK {
	return &NvmeServiceDeleteOK{}
}

/*
NvmeServiceDeleteOK describes a response with status code 200, with default header values.

OK
*/
type NvmeServiceDeleteOK struct {
}

// IsSuccess returns true when this nvme service delete o k response has a 2xx status code
func (o *NvmeServiceDeleteOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this nvme service delete o k response has a 3xx status code
func (o *NvmeServiceDeleteOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this nvme service delete o k response has a 4xx status code
func (o *NvmeServiceDeleteOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this nvme service delete o k response has a 5xx status code
func (o *NvmeServiceDeleteOK) IsServerError() bool {
	return false
}

// IsCode returns true when this nvme service delete o k response a status code equal to that given
func (o *NvmeServiceDeleteOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the nvme service delete o k response
func (o *NvmeServiceDeleteOK) Code() int {
	return 200
}

func (o *NvmeServiceDeleteOK) Error() string {
	return fmt.Sprintf("[DELETE /protocols/nvme/services/{svm.uuid}][%d] nvmeServiceDeleteOK", 200)
}

func (o *NvmeServiceDeleteOK) String() string {
	return fmt.Sprintf("[DELETE /protocols/nvme/services/{svm.uuid}][%d] nvmeServiceDeleteOK", 200)
}

func (o *NvmeServiceDeleteOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewNvmeServiceDeleteDefault creates a NvmeServiceDeleteDefault with default headers values
func NewNvmeServiceDeleteDefault(code int) *NvmeServiceDeleteDefault {
	return &NvmeServiceDeleteDefault{
		_statusCode: code,
	}
}

/*
	NvmeServiceDeleteDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 2621462 | The supplied SVM does not exist. |
| 5376452 | Service POST and DELETE are not supported on ASA r2. |
| 72089651 | The supplied SVM does not have an NVMe service. |
| 72089653 | There are subsystems associated with the NVMe service SVM. The subsystems must be removed before deleting the NVMe service. |
| 72089654 | There are NVMe-oF LIFs associated with the NVMe service SVM. The LIFs must be removed before deleting the NVMe service. |
| 72090028 | The NVMe service is enabled. The NVMe service must be disabled before it can be deleted. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type NvmeServiceDeleteDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this nvme service delete default response has a 2xx status code
func (o *NvmeServiceDeleteDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this nvme service delete default response has a 3xx status code
func (o *NvmeServiceDeleteDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this nvme service delete default response has a 4xx status code
func (o *NvmeServiceDeleteDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this nvme service delete default response has a 5xx status code
func (o *NvmeServiceDeleteDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this nvme service delete default response a status code equal to that given
func (o *NvmeServiceDeleteDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the nvme service delete default response
func (o *NvmeServiceDeleteDefault) Code() int {
	return o._statusCode
}

func (o *NvmeServiceDeleteDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/nvme/services/{svm.uuid}][%d] nvme_service_delete default %s", o._statusCode, payload)
}

func (o *NvmeServiceDeleteDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /protocols/nvme/services/{svm.uuid}][%d] nvme_service_delete default %s", o._statusCode, payload)
}

func (o *NvmeServiceDeleteDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *NvmeServiceDeleteDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
