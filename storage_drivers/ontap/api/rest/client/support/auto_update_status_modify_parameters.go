// Code generated by go-swagger; DO NOT EDIT.

package support

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"net/http"
	"time"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	cr "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
)

// NewAutoUpdateStatusModifyParams creates a new AutoUpdateStatusModifyParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewAutoUpdateStatusModifyParams() *AutoUpdateStatusModifyParams {
	return &AutoUpdateStatusModifyParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewAutoUpdateStatusModifyParamsWithTimeout creates a new AutoUpdateStatusModifyParams object
// with the ability to set a timeout on a request.
func NewAutoUpdateStatusModifyParamsWithTimeout(timeout time.Duration) *AutoUpdateStatusModifyParams {
	return &AutoUpdateStatusModifyParams{
		timeout: timeout,
	}
}

// NewAutoUpdateStatusModifyParamsWithContext creates a new AutoUpdateStatusModifyParams object
// with the ability to set a context for a request.
func NewAutoUpdateStatusModifyParamsWithContext(ctx context.Context) *AutoUpdateStatusModifyParams {
	return &AutoUpdateStatusModifyParams{
		Context: ctx,
	}
}

// NewAutoUpdateStatusModifyParamsWithHTTPClient creates a new AutoUpdateStatusModifyParams object
// with the ability to set a custom HTTPClient for a request.
func NewAutoUpdateStatusModifyParamsWithHTTPClient(client *http.Client) *AutoUpdateStatusModifyParams {
	return &AutoUpdateStatusModifyParams{
		HTTPClient: client,
	}
}

/*
AutoUpdateStatusModifyParams contains all the parameters to send to the API endpoint

	for the auto update status modify operation.

	Typically these are written to a http.Request.
*/
type AutoUpdateStatusModifyParams struct {

	/* Action.

	   Action to be performed on the update
	*/
	Action string

	/* ScheduleTime.

	   Date and Time for which update is to be scheduled. This parameter is required only when action is "schedule". It should be in the ISO-8601 Date and Time format. Example- "2020-12-05T09:12:23Z".

	   Format: date-time
	*/
	ScheduleTime *strfmt.DateTime

	/* UUID.

	   Update identifier
	*/
	UUID string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the auto update status modify params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *AutoUpdateStatusModifyParams) WithDefaults() *AutoUpdateStatusModifyParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the auto update status modify params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *AutoUpdateStatusModifyParams) SetDefaults() {
	// no default values defined for this parameter
}

// WithTimeout adds the timeout to the auto update status modify params
func (o *AutoUpdateStatusModifyParams) WithTimeout(timeout time.Duration) *AutoUpdateStatusModifyParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the auto update status modify params
func (o *AutoUpdateStatusModifyParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the auto update status modify params
func (o *AutoUpdateStatusModifyParams) WithContext(ctx context.Context) *AutoUpdateStatusModifyParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the auto update status modify params
func (o *AutoUpdateStatusModifyParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the auto update status modify params
func (o *AutoUpdateStatusModifyParams) WithHTTPClient(client *http.Client) *AutoUpdateStatusModifyParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the auto update status modify params
func (o *AutoUpdateStatusModifyParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithAction adds the action to the auto update status modify params
func (o *AutoUpdateStatusModifyParams) WithAction(action string) *AutoUpdateStatusModifyParams {
	o.SetAction(action)
	return o
}

// SetAction adds the action to the auto update status modify params
func (o *AutoUpdateStatusModifyParams) SetAction(action string) {
	o.Action = action
}

// WithScheduleTime adds the scheduleTime to the auto update status modify params
func (o *AutoUpdateStatusModifyParams) WithScheduleTime(scheduleTime *strfmt.DateTime) *AutoUpdateStatusModifyParams {
	o.SetScheduleTime(scheduleTime)
	return o
}

// SetScheduleTime adds the scheduleTime to the auto update status modify params
func (o *AutoUpdateStatusModifyParams) SetScheduleTime(scheduleTime *strfmt.DateTime) {
	o.ScheduleTime = scheduleTime
}

// WithUUID adds the uuid to the auto update status modify params
func (o *AutoUpdateStatusModifyParams) WithUUID(uuid string) *AutoUpdateStatusModifyParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the auto update status modify params
func (o *AutoUpdateStatusModifyParams) SetUUID(uuid string) {
	o.UUID = uuid
}

// WriteToRequest writes these params to a swagger request
func (o *AutoUpdateStatusModifyParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	// query param action
	qrAction := o.Action
	qAction := qrAction
	if qAction != "" {

		if err := r.SetQueryParam("action", qAction); err != nil {
			return err
		}
	}

	if o.ScheduleTime != nil {

		// query param schedule_time
		var qrScheduleTime strfmt.DateTime

		if o.ScheduleTime != nil {
			qrScheduleTime = *o.ScheduleTime
		}
		qScheduleTime := qrScheduleTime.String()
		if qScheduleTime != "" {

			if err := r.SetQueryParam("schedule_time", qScheduleTime); err != nil {
				return err
			}
		}
	}

	// path param uuid
	if err := r.SetPathParam("uuid", o.UUID); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
