// Code generated by go-swagger; DO NOT EDIT.

package networking

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
)

// NetworkIPInterfaceDeleteReader is a Reader for the NetworkIPInterfaceDelete structure.
type NetworkIPInterfaceDeleteReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *NetworkIPInterfaceDeleteReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewNetworkIPInterfaceDeleteOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		return nil, runtime.NewAPIError("response status code does not match any response statuses defined for this endpoint in the swagger spec", response, response.Code())
	}
}

// NewNetworkIPInterfaceDeleteOK creates a NetworkIPInterfaceDeleteOK with default headers values
func NewNetworkIPInterfaceDeleteOK() *NetworkIPInterfaceDeleteOK {
	return &NetworkIPInterfaceDeleteOK{}
}

/*
NetworkIPInterfaceDeleteOK describes a response with status code 200, with default header values.

OK
*/
type NetworkIPInterfaceDeleteOK struct {
}

// IsSuccess returns true when this network Ip interface delete o k response has a 2xx status code
func (o *NetworkIPInterfaceDeleteOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this network Ip interface delete o k response has a 3xx status code
func (o *NetworkIPInterfaceDeleteOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this network Ip interface delete o k response has a 4xx status code
func (o *NetworkIPInterfaceDeleteOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this network Ip interface delete o k response has a 5xx status code
func (o *NetworkIPInterfaceDeleteOK) IsServerError() bool {
	return false
}

// IsCode returns true when this network Ip interface delete o k response a status code equal to that given
func (o *NetworkIPInterfaceDeleteOK) IsCode(code int) bool {
	return code == 200
}

func (o *NetworkIPInterfaceDeleteOK) Error() string {
	return fmt.Sprintf("[DELETE /network/ip/interfaces/{uuid}][%d] networkIpInterfaceDeleteOK ", 200)
}

func (o *NetworkIPInterfaceDeleteOK) String() string {
	return fmt.Sprintf("[DELETE /network/ip/interfaces/{uuid}][%d] networkIpInterfaceDeleteOK ", 200)
}

func (o *NetworkIPInterfaceDeleteOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
