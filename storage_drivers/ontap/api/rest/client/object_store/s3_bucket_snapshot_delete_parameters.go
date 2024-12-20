// Code generated by go-swagger; DO NOT EDIT.

package object_store

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

// NewS3BucketSnapshotDeleteParams creates a new S3BucketSnapshotDeleteParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewS3BucketSnapshotDeleteParams() *S3BucketSnapshotDeleteParams {
	return &S3BucketSnapshotDeleteParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewS3BucketSnapshotDeleteParamsWithTimeout creates a new S3BucketSnapshotDeleteParams object
// with the ability to set a timeout on a request.
func NewS3BucketSnapshotDeleteParamsWithTimeout(timeout time.Duration) *S3BucketSnapshotDeleteParams {
	return &S3BucketSnapshotDeleteParams{
		timeout: timeout,
	}
}

// NewS3BucketSnapshotDeleteParamsWithContext creates a new S3BucketSnapshotDeleteParams object
// with the ability to set a context for a request.
func NewS3BucketSnapshotDeleteParamsWithContext(ctx context.Context) *S3BucketSnapshotDeleteParams {
	return &S3BucketSnapshotDeleteParams{
		Context: ctx,
	}
}

// NewS3BucketSnapshotDeleteParamsWithHTTPClient creates a new S3BucketSnapshotDeleteParams object
// with the ability to set a custom HTTPClient for a request.
func NewS3BucketSnapshotDeleteParamsWithHTTPClient(client *http.Client) *S3BucketSnapshotDeleteParams {
	return &S3BucketSnapshotDeleteParams{
		HTTPClient: client,
	}
}

/*
S3BucketSnapshotDeleteParams contains all the parameters to send to the API endpoint

	for the s3 bucket snapshot delete operation.

	Typically these are written to a http.Request.
*/
type S3BucketSnapshotDeleteParams struct {

	/* S3BucketUUID.

	   The unique identifier of the bucket.
	*/
	S3BucketUUID string

	/* SvmUUID.

	   UUID of the SVM to which this object belongs.
	*/
	SvmUUID string

	/* UUID.

	   The unique identifier of the S3 bucket snapshot.
	*/
	UUID string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the s3 bucket snapshot delete params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *S3BucketSnapshotDeleteParams) WithDefaults() *S3BucketSnapshotDeleteParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the s3 bucket snapshot delete params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *S3BucketSnapshotDeleteParams) SetDefaults() {
	// no default values defined for this parameter
}

// WithTimeout adds the timeout to the s3 bucket snapshot delete params
func (o *S3BucketSnapshotDeleteParams) WithTimeout(timeout time.Duration) *S3BucketSnapshotDeleteParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the s3 bucket snapshot delete params
func (o *S3BucketSnapshotDeleteParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the s3 bucket snapshot delete params
func (o *S3BucketSnapshotDeleteParams) WithContext(ctx context.Context) *S3BucketSnapshotDeleteParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the s3 bucket snapshot delete params
func (o *S3BucketSnapshotDeleteParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the s3 bucket snapshot delete params
func (o *S3BucketSnapshotDeleteParams) WithHTTPClient(client *http.Client) *S3BucketSnapshotDeleteParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the s3 bucket snapshot delete params
func (o *S3BucketSnapshotDeleteParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithS3BucketUUID adds the s3BucketUUID to the s3 bucket snapshot delete params
func (o *S3BucketSnapshotDeleteParams) WithS3BucketUUID(s3BucketUUID string) *S3BucketSnapshotDeleteParams {
	o.SetS3BucketUUID(s3BucketUUID)
	return o
}

// SetS3BucketUUID adds the s3BucketUuid to the s3 bucket snapshot delete params
func (o *S3BucketSnapshotDeleteParams) SetS3BucketUUID(s3BucketUUID string) {
	o.S3BucketUUID = s3BucketUUID
}

// WithSvmUUID adds the svmUUID to the s3 bucket snapshot delete params
func (o *S3BucketSnapshotDeleteParams) WithSvmUUID(svmUUID string) *S3BucketSnapshotDeleteParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the s3 bucket snapshot delete params
func (o *S3BucketSnapshotDeleteParams) SetSvmUUID(svmUUID string) {
	o.SvmUUID = svmUUID
}

// WithUUID adds the uuid to the s3 bucket snapshot delete params
func (o *S3BucketSnapshotDeleteParams) WithUUID(uuid string) *S3BucketSnapshotDeleteParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the s3 bucket snapshot delete params
func (o *S3BucketSnapshotDeleteParams) SetUUID(uuid string) {
	o.UUID = uuid
}

// WriteToRequest writes these params to a swagger request
func (o *S3BucketSnapshotDeleteParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	// path param s3_bucket.uuid
	if err := r.SetPathParam("s3_bucket.uuid", o.S3BucketUUID); err != nil {
		return err
	}

	// path param svm.uuid
	if err := r.SetPathParam("svm.uuid", o.SvmUUID); err != nil {
		return err
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
