// Code generated by go-swagger; DO NOT EDIT.

package storage

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// AggregateDeleteCollectionReader is a Reader for the AggregateDeleteCollection structure.
type AggregateDeleteCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *AggregateDeleteCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewAggregateDeleteCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	case 202:
		result := NewAggregateDeleteCollectionAccepted()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewAggregateDeleteCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewAggregateDeleteCollectionOK creates a AggregateDeleteCollectionOK with default headers values
func NewAggregateDeleteCollectionOK() *AggregateDeleteCollectionOK {
	return &AggregateDeleteCollectionOK{}
}

/*
AggregateDeleteCollectionOK describes a response with status code 200, with default header values.

OK
*/
type AggregateDeleteCollectionOK struct {
	Payload *models.AggregateJobLinkResponse
}

// IsSuccess returns true when this aggregate delete collection o k response has a 2xx status code
func (o *AggregateDeleteCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this aggregate delete collection o k response has a 3xx status code
func (o *AggregateDeleteCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this aggregate delete collection o k response has a 4xx status code
func (o *AggregateDeleteCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this aggregate delete collection o k response has a 5xx status code
func (o *AggregateDeleteCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this aggregate delete collection o k response a status code equal to that given
func (o *AggregateDeleteCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the aggregate delete collection o k response
func (o *AggregateDeleteCollectionOK) Code() int {
	return 200
}

func (o *AggregateDeleteCollectionOK) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /storage/aggregates][%d] aggregateDeleteCollectionOK %s", 200, payload)
}

func (o *AggregateDeleteCollectionOK) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /storage/aggregates][%d] aggregateDeleteCollectionOK %s", 200, payload)
}

func (o *AggregateDeleteCollectionOK) GetPayload() *models.AggregateJobLinkResponse {
	return o.Payload
}

func (o *AggregateDeleteCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.AggregateJobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewAggregateDeleteCollectionAccepted creates a AggregateDeleteCollectionAccepted with default headers values
func NewAggregateDeleteCollectionAccepted() *AggregateDeleteCollectionAccepted {
	return &AggregateDeleteCollectionAccepted{}
}

/*
AggregateDeleteCollectionAccepted describes a response with status code 202, with default header values.

Accepted
*/
type AggregateDeleteCollectionAccepted struct {
	Payload *models.AggregateJobLinkResponse
}

// IsSuccess returns true when this aggregate delete collection accepted response has a 2xx status code
func (o *AggregateDeleteCollectionAccepted) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this aggregate delete collection accepted response has a 3xx status code
func (o *AggregateDeleteCollectionAccepted) IsRedirect() bool {
	return false
}

// IsClientError returns true when this aggregate delete collection accepted response has a 4xx status code
func (o *AggregateDeleteCollectionAccepted) IsClientError() bool {
	return false
}

// IsServerError returns true when this aggregate delete collection accepted response has a 5xx status code
func (o *AggregateDeleteCollectionAccepted) IsServerError() bool {
	return false
}

// IsCode returns true when this aggregate delete collection accepted response a status code equal to that given
func (o *AggregateDeleteCollectionAccepted) IsCode(code int) bool {
	return code == 202
}

// Code gets the status code for the aggregate delete collection accepted response
func (o *AggregateDeleteCollectionAccepted) Code() int {
	return 202
}

func (o *AggregateDeleteCollectionAccepted) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /storage/aggregates][%d] aggregateDeleteCollectionAccepted %s", 202, payload)
}

func (o *AggregateDeleteCollectionAccepted) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /storage/aggregates][%d] aggregateDeleteCollectionAccepted %s", 202, payload)
}

func (o *AggregateDeleteCollectionAccepted) GetPayload() *models.AggregateJobLinkResponse {
	return o.Payload
}

func (o *AggregateDeleteCollectionAccepted) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.AggregateJobLinkResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewAggregateDeleteCollectionDefault creates a AggregateDeleteCollectionDefault with default headers values
func NewAggregateDeleteCollectionDefault(code int) *AggregateDeleteCollectionDefault {
	return &AggregateDeleteCollectionDefault{
		_statusCode: code,
	}
}

/*
	AggregateDeleteCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 460770 | The aggregate delete job failed to delete the aggregate. |
| 460777 | Failed to get information on the delete job. |
| 786435 | Internal Error. Failed to create a communication handle. |
| 786451 | Failed to delete specified aggregate. |
| 786468 | VLDB is offline. |
| 786472 | Node that hosts the aggregate is offline. |
| 786497 | Cannot delete an aggregate that has volumes. |
| 786771 | Aggregate does not exist. |
| 786867 | Specified aggregate resides on the remote cluster. |
| 786897 | Specified aggregate cannot be deleted as it is a switched-over root aggregate. |
| 19726544 | DELETE on the aggregate endpoint is not supported on this version of ONTAP. |
Also see the table of common errors in the <a href="#Response_body">Response body</a> overview section of this documentation.
*/
type AggregateDeleteCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this aggregate delete collection default response has a 2xx status code
func (o *AggregateDeleteCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this aggregate delete collection default response has a 3xx status code
func (o *AggregateDeleteCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this aggregate delete collection default response has a 4xx status code
func (o *AggregateDeleteCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this aggregate delete collection default response has a 5xx status code
func (o *AggregateDeleteCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this aggregate delete collection default response a status code equal to that given
func (o *AggregateDeleteCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the aggregate delete collection default response
func (o *AggregateDeleteCollectionDefault) Code() int {
	return o._statusCode
}

func (o *AggregateDeleteCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /storage/aggregates][%d] aggregate_delete_collection default %s", o._statusCode, payload)
}

func (o *AggregateDeleteCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /storage/aggregates][%d] aggregate_delete_collection default %s", o._statusCode, payload)
}

func (o *AggregateDeleteCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *AggregateDeleteCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
AggregateDeleteCollectionBody aggregate delete collection body
swagger:model AggregateDeleteCollectionBody
*/
type AggregateDeleteCollectionBody struct {

	// Information on the aggregate's remaining hot spare disks.
	AggregateResponseInlineRecommendationSpares []*models.AggregateSpare `json:"recommendation_spares,omitempty"`

	// aggregate response inline records
	AggregateResponseInlineRecords []*models.Aggregate `json:"records,omitempty"`

	// aggregate response inline spares
	AggregateResponseInlineSpares []*models.AggregateSpare `json:"spares,omitempty"`

	// List of warnings and remediation advice for the aggregate recommendation.
	AggregateResponseInlineWarnings []*models.AggregateWarning `json:"warnings,omitempty"`

	// error
	Error *models.Error `json:"error,omitempty"`
}

// Validate validates this aggregate delete collection body
func (o *AggregateDeleteCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateAggregateResponseInlineRecommendationSpares(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateAggregateResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateAggregateResponseInlineSpares(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateAggregateResponseInlineWarnings(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateError(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *AggregateDeleteCollectionBody) validateAggregateResponseInlineRecommendationSpares(formats strfmt.Registry) error {
	if swag.IsZero(o.AggregateResponseInlineRecommendationSpares) { // not required
		return nil
	}

	for i := 0; i < len(o.AggregateResponseInlineRecommendationSpares); i++ {
		if swag.IsZero(o.AggregateResponseInlineRecommendationSpares[i]) { // not required
			continue
		}

		if o.AggregateResponseInlineRecommendationSpares[i] != nil {
			if err := o.AggregateResponseInlineRecommendationSpares[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "recommendation_spares" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *AggregateDeleteCollectionBody) validateAggregateResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.AggregateResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.AggregateResponseInlineRecords); i++ {
		if swag.IsZero(o.AggregateResponseInlineRecords[i]) { // not required
			continue
		}

		if o.AggregateResponseInlineRecords[i] != nil {
			if err := o.AggregateResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *AggregateDeleteCollectionBody) validateAggregateResponseInlineSpares(formats strfmt.Registry) error {
	if swag.IsZero(o.AggregateResponseInlineSpares) { // not required
		return nil
	}

	for i := 0; i < len(o.AggregateResponseInlineSpares); i++ {
		if swag.IsZero(o.AggregateResponseInlineSpares[i]) { // not required
			continue
		}

		if o.AggregateResponseInlineSpares[i] != nil {
			if err := o.AggregateResponseInlineSpares[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "spares" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *AggregateDeleteCollectionBody) validateAggregateResponseInlineWarnings(formats strfmt.Registry) error {
	if swag.IsZero(o.AggregateResponseInlineWarnings) { // not required
		return nil
	}

	for i := 0; i < len(o.AggregateResponseInlineWarnings); i++ {
		if swag.IsZero(o.AggregateResponseInlineWarnings[i]) { // not required
			continue
		}

		if o.AggregateResponseInlineWarnings[i] != nil {
			if err := o.AggregateResponseInlineWarnings[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "warnings" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *AggregateDeleteCollectionBody) validateError(formats strfmt.Registry) error {
	if swag.IsZero(o.Error) { // not required
		return nil
	}

	if o.Error != nil {
		if err := o.Error.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "error")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this aggregate delete collection body based on the context it is used
func (o *AggregateDeleteCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateAggregateResponseInlineRecommendationSpares(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateAggregateResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateAggregateResponseInlineSpares(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateAggregateResponseInlineWarnings(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateError(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *AggregateDeleteCollectionBody) contextValidateAggregateResponseInlineRecommendationSpares(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.AggregateResponseInlineRecommendationSpares); i++ {

		if o.AggregateResponseInlineRecommendationSpares[i] != nil {
			if err := o.AggregateResponseInlineRecommendationSpares[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "recommendation_spares" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *AggregateDeleteCollectionBody) contextValidateAggregateResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.AggregateResponseInlineRecords); i++ {

		if o.AggregateResponseInlineRecords[i] != nil {
			if err := o.AggregateResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *AggregateDeleteCollectionBody) contextValidateAggregateResponseInlineSpares(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.AggregateResponseInlineSpares); i++ {

		if o.AggregateResponseInlineSpares[i] != nil {
			if err := o.AggregateResponseInlineSpares[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "spares" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *AggregateDeleteCollectionBody) contextValidateAggregateResponseInlineWarnings(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.AggregateResponseInlineWarnings); i++ {

		if o.AggregateResponseInlineWarnings[i] != nil {
			if err := o.AggregateResponseInlineWarnings[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "warnings" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *AggregateDeleteCollectionBody) contextValidateError(ctx context.Context, formats strfmt.Registry) error {

	if o.Error != nil {
		if err := o.Error.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "error")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *AggregateDeleteCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *AggregateDeleteCollectionBody) UnmarshalBinary(b []byte) error {
	var res AggregateDeleteCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}