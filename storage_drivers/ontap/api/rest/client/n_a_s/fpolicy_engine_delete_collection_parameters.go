// Code generated by go-swagger; DO NOT EDIT.

package n_a_s

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
	"github.com/go-openapi/swag"
)

// NewFpolicyEngineDeleteCollectionParams creates a new FpolicyEngineDeleteCollectionParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewFpolicyEngineDeleteCollectionParams() *FpolicyEngineDeleteCollectionParams {
	return &FpolicyEngineDeleteCollectionParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewFpolicyEngineDeleteCollectionParamsWithTimeout creates a new FpolicyEngineDeleteCollectionParams object
// with the ability to set a timeout on a request.
func NewFpolicyEngineDeleteCollectionParamsWithTimeout(timeout time.Duration) *FpolicyEngineDeleteCollectionParams {
	return &FpolicyEngineDeleteCollectionParams{
		timeout: timeout,
	}
}

// NewFpolicyEngineDeleteCollectionParamsWithContext creates a new FpolicyEngineDeleteCollectionParams object
// with the ability to set a context for a request.
func NewFpolicyEngineDeleteCollectionParamsWithContext(ctx context.Context) *FpolicyEngineDeleteCollectionParams {
	return &FpolicyEngineDeleteCollectionParams{
		Context: ctx,
	}
}

// NewFpolicyEngineDeleteCollectionParamsWithHTTPClient creates a new FpolicyEngineDeleteCollectionParams object
// with the ability to set a custom HTTPClient for a request.
func NewFpolicyEngineDeleteCollectionParamsWithHTTPClient(client *http.Client) *FpolicyEngineDeleteCollectionParams {
	return &FpolicyEngineDeleteCollectionParams{
		HTTPClient: client,
	}
}

/*
FpolicyEngineDeleteCollectionParams contains all the parameters to send to the API endpoint

	for the fpolicy engine delete collection operation.

	Typically these are written to a http.Request.
*/
type FpolicyEngineDeleteCollectionParams struct {

	/* BufferSizeRecvBuffer.

	   Filter by buffer_size.recv_buffer
	*/
	BufferSizeRecvBuffer *int64

	/* BufferSizeSendBuffer.

	   Filter by buffer_size.send_buffer
	*/
	BufferSizeSendBuffer *int64

	/* CertificateCa.

	   Filter by certificate.ca
	*/
	CertificateCa *string

	/* CertificateName.

	   Filter by certificate.name
	*/
	CertificateName *string

	/* CertificateSerialNumber.

	   Filter by certificate.serial_number
	*/
	CertificateSerialNumber *string

	/* ContinueOnFailure.

	   Continue even when the operation fails on one of the records.
	*/
	ContinueOnFailure *bool

	/* Format.

	   Filter by format
	*/
	Format *string

	/* Info.

	   Info specification
	*/
	Info FpolicyEngineDeleteCollectionBody

	/* KeepAliveInterval.

	   Filter by keep_alive_interval
	*/
	KeepAliveInterval *string

	/* MaxServerRequests.

	   Filter by max_server_requests
	*/
	MaxServerRequests *int64

	/* Name.

	   Filter by name
	*/
	Name *string

	/* Port.

	   Filter by port
	*/
	Port *int64

	/* PrimaryServers.

	   Filter by primary_servers
	*/
	PrimaryServers *string

	/* RequestAbortTimeout.

	   Filter by request_abort_timeout
	*/
	RequestAbortTimeout *string

	/* RequestCancelTimeout.

	   Filter by request_cancel_timeout
	*/
	RequestCancelTimeout *string

	/* ResiliencyDirectoryPath.

	   Filter by resiliency.directory_path
	*/
	ResiliencyDirectoryPath *string

	/* ResiliencyEnabled.

	   Filter by resiliency.enabled
	*/
	ResiliencyEnabled *bool

	/* ResiliencyRetentionDuration.

	   Filter by resiliency.retention_duration
	*/
	ResiliencyRetentionDuration *string

	/* ReturnRecords.

	   The default is true for GET calls.  When set to false, only the number of records is returned.

	   Default: true
	*/
	ReturnRecords *bool

	/* ReturnTimeout.

	   The number of seconds to allow the call to execute before returning.  When iterating over a collection, the default is 15 seconds.  ONTAP returns earlier if either max records or the end of the collection is reached.

	   Default: 15
	*/
	ReturnTimeout *int64

	/* SecondaryServers.

	   Filter by secondary_servers
	*/
	SecondaryServers *string

	/* SerialRecords.

	   Perform the operation on the records synchronously.
	*/
	SerialRecords *bool

	/* ServerProgressTimeout.

	   Filter by server_progress_timeout
	*/
	ServerProgressTimeout *string

	/* SslOption.

	   Filter by ssl_option
	*/
	SslOption *string

	/* StatusRequestInterval.

	   Filter by status_request_interval
	*/
	StatusRequestInterval *string

	/* SvmUUID.

	   UUID of the SVM to which this object belongs.
	*/
	SvmUUID string

	/* Type.

	   Filter by type
	*/
	Type *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the fpolicy engine delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *FpolicyEngineDeleteCollectionParams) WithDefaults() *FpolicyEngineDeleteCollectionParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the fpolicy engine delete collection params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *FpolicyEngineDeleteCollectionParams) SetDefaults() {
	var (
		continueOnFailureDefault = bool(false)

		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)

		serialRecordsDefault = bool(false)
	)

	val := FpolicyEngineDeleteCollectionParams{
		ContinueOnFailure: &continueOnFailureDefault,
		ReturnRecords:     &returnRecordsDefault,
		ReturnTimeout:     &returnTimeoutDefault,
		SerialRecords:     &serialRecordsDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithTimeout(timeout time.Duration) *FpolicyEngineDeleteCollectionParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithContext(ctx context.Context) *FpolicyEngineDeleteCollectionParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithHTTPClient(client *http.Client) *FpolicyEngineDeleteCollectionParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithBufferSizeRecvBuffer adds the bufferSizeRecvBuffer to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithBufferSizeRecvBuffer(bufferSizeRecvBuffer *int64) *FpolicyEngineDeleteCollectionParams {
	o.SetBufferSizeRecvBuffer(bufferSizeRecvBuffer)
	return o
}

// SetBufferSizeRecvBuffer adds the bufferSizeRecvBuffer to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetBufferSizeRecvBuffer(bufferSizeRecvBuffer *int64) {
	o.BufferSizeRecvBuffer = bufferSizeRecvBuffer
}

// WithBufferSizeSendBuffer adds the bufferSizeSendBuffer to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithBufferSizeSendBuffer(bufferSizeSendBuffer *int64) *FpolicyEngineDeleteCollectionParams {
	o.SetBufferSizeSendBuffer(bufferSizeSendBuffer)
	return o
}

// SetBufferSizeSendBuffer adds the bufferSizeSendBuffer to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetBufferSizeSendBuffer(bufferSizeSendBuffer *int64) {
	o.BufferSizeSendBuffer = bufferSizeSendBuffer
}

// WithCertificateCa adds the certificateCa to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithCertificateCa(certificateCa *string) *FpolicyEngineDeleteCollectionParams {
	o.SetCertificateCa(certificateCa)
	return o
}

// SetCertificateCa adds the certificateCa to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetCertificateCa(certificateCa *string) {
	o.CertificateCa = certificateCa
}

// WithCertificateName adds the certificateName to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithCertificateName(certificateName *string) *FpolicyEngineDeleteCollectionParams {
	o.SetCertificateName(certificateName)
	return o
}

// SetCertificateName adds the certificateName to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetCertificateName(certificateName *string) {
	o.CertificateName = certificateName
}

// WithCertificateSerialNumber adds the certificateSerialNumber to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithCertificateSerialNumber(certificateSerialNumber *string) *FpolicyEngineDeleteCollectionParams {
	o.SetCertificateSerialNumber(certificateSerialNumber)
	return o
}

// SetCertificateSerialNumber adds the certificateSerialNumber to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetCertificateSerialNumber(certificateSerialNumber *string) {
	o.CertificateSerialNumber = certificateSerialNumber
}

// WithContinueOnFailure adds the continueOnFailure to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithContinueOnFailure(continueOnFailure *bool) *FpolicyEngineDeleteCollectionParams {
	o.SetContinueOnFailure(continueOnFailure)
	return o
}

// SetContinueOnFailure adds the continueOnFailure to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetContinueOnFailure(continueOnFailure *bool) {
	o.ContinueOnFailure = continueOnFailure
}

// WithFormat adds the format to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithFormat(format *string) *FpolicyEngineDeleteCollectionParams {
	o.SetFormat(format)
	return o
}

// SetFormat adds the format to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetFormat(format *string) {
	o.Format = format
}

// WithInfo adds the info to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithInfo(info FpolicyEngineDeleteCollectionBody) *FpolicyEngineDeleteCollectionParams {
	o.SetInfo(info)
	return o
}

// SetInfo adds the info to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetInfo(info FpolicyEngineDeleteCollectionBody) {
	o.Info = info
}

// WithKeepAliveInterval adds the keepAliveInterval to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithKeepAliveInterval(keepAliveInterval *string) *FpolicyEngineDeleteCollectionParams {
	o.SetKeepAliveInterval(keepAliveInterval)
	return o
}

// SetKeepAliveInterval adds the keepAliveInterval to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetKeepAliveInterval(keepAliveInterval *string) {
	o.KeepAliveInterval = keepAliveInterval
}

// WithMaxServerRequests adds the maxServerRequests to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithMaxServerRequests(maxServerRequests *int64) *FpolicyEngineDeleteCollectionParams {
	o.SetMaxServerRequests(maxServerRequests)
	return o
}

// SetMaxServerRequests adds the maxServerRequests to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetMaxServerRequests(maxServerRequests *int64) {
	o.MaxServerRequests = maxServerRequests
}

// WithName adds the name to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithName(name *string) *FpolicyEngineDeleteCollectionParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetName(name *string) {
	o.Name = name
}

// WithPort adds the port to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithPort(port *int64) *FpolicyEngineDeleteCollectionParams {
	o.SetPort(port)
	return o
}

// SetPort adds the port to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetPort(port *int64) {
	o.Port = port
}

// WithPrimaryServers adds the primaryServers to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithPrimaryServers(primaryServers *string) *FpolicyEngineDeleteCollectionParams {
	o.SetPrimaryServers(primaryServers)
	return o
}

// SetPrimaryServers adds the primaryServers to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetPrimaryServers(primaryServers *string) {
	o.PrimaryServers = primaryServers
}

// WithRequestAbortTimeout adds the requestAbortTimeout to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithRequestAbortTimeout(requestAbortTimeout *string) *FpolicyEngineDeleteCollectionParams {
	o.SetRequestAbortTimeout(requestAbortTimeout)
	return o
}

// SetRequestAbortTimeout adds the requestAbortTimeout to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetRequestAbortTimeout(requestAbortTimeout *string) {
	o.RequestAbortTimeout = requestAbortTimeout
}

// WithRequestCancelTimeout adds the requestCancelTimeout to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithRequestCancelTimeout(requestCancelTimeout *string) *FpolicyEngineDeleteCollectionParams {
	o.SetRequestCancelTimeout(requestCancelTimeout)
	return o
}

// SetRequestCancelTimeout adds the requestCancelTimeout to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetRequestCancelTimeout(requestCancelTimeout *string) {
	o.RequestCancelTimeout = requestCancelTimeout
}

// WithResiliencyDirectoryPath adds the resiliencyDirectoryPath to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithResiliencyDirectoryPath(resiliencyDirectoryPath *string) *FpolicyEngineDeleteCollectionParams {
	o.SetResiliencyDirectoryPath(resiliencyDirectoryPath)
	return o
}

// SetResiliencyDirectoryPath adds the resiliencyDirectoryPath to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetResiliencyDirectoryPath(resiliencyDirectoryPath *string) {
	o.ResiliencyDirectoryPath = resiliencyDirectoryPath
}

// WithResiliencyEnabled adds the resiliencyEnabled to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithResiliencyEnabled(resiliencyEnabled *bool) *FpolicyEngineDeleteCollectionParams {
	o.SetResiliencyEnabled(resiliencyEnabled)
	return o
}

// SetResiliencyEnabled adds the resiliencyEnabled to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetResiliencyEnabled(resiliencyEnabled *bool) {
	o.ResiliencyEnabled = resiliencyEnabled
}

// WithResiliencyRetentionDuration adds the resiliencyRetentionDuration to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithResiliencyRetentionDuration(resiliencyRetentionDuration *string) *FpolicyEngineDeleteCollectionParams {
	o.SetResiliencyRetentionDuration(resiliencyRetentionDuration)
	return o
}

// SetResiliencyRetentionDuration adds the resiliencyRetentionDuration to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetResiliencyRetentionDuration(resiliencyRetentionDuration *string) {
	o.ResiliencyRetentionDuration = resiliencyRetentionDuration
}

// WithReturnRecords adds the returnRecords to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithReturnRecords(returnRecords *bool) *FpolicyEngineDeleteCollectionParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithReturnTimeout(returnTimeout *int64) *FpolicyEngineDeleteCollectionParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithSecondaryServers adds the secondaryServers to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithSecondaryServers(secondaryServers *string) *FpolicyEngineDeleteCollectionParams {
	o.SetSecondaryServers(secondaryServers)
	return o
}

// SetSecondaryServers adds the secondaryServers to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetSecondaryServers(secondaryServers *string) {
	o.SecondaryServers = secondaryServers
}

// WithSerialRecords adds the serialRecords to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithSerialRecords(serialRecords *bool) *FpolicyEngineDeleteCollectionParams {
	o.SetSerialRecords(serialRecords)
	return o
}

// SetSerialRecords adds the serialRecords to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetSerialRecords(serialRecords *bool) {
	o.SerialRecords = serialRecords
}

// WithServerProgressTimeout adds the serverProgressTimeout to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithServerProgressTimeout(serverProgressTimeout *string) *FpolicyEngineDeleteCollectionParams {
	o.SetServerProgressTimeout(serverProgressTimeout)
	return o
}

// SetServerProgressTimeout adds the serverProgressTimeout to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetServerProgressTimeout(serverProgressTimeout *string) {
	o.ServerProgressTimeout = serverProgressTimeout
}

// WithSslOption adds the sslOption to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithSslOption(sslOption *string) *FpolicyEngineDeleteCollectionParams {
	o.SetSslOption(sslOption)
	return o
}

// SetSslOption adds the sslOption to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetSslOption(sslOption *string) {
	o.SslOption = sslOption
}

// WithStatusRequestInterval adds the statusRequestInterval to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithStatusRequestInterval(statusRequestInterval *string) *FpolicyEngineDeleteCollectionParams {
	o.SetStatusRequestInterval(statusRequestInterval)
	return o
}

// SetStatusRequestInterval adds the statusRequestInterval to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetStatusRequestInterval(statusRequestInterval *string) {
	o.StatusRequestInterval = statusRequestInterval
}

// WithSvmUUID adds the svmUUID to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithSvmUUID(svmUUID string) *FpolicyEngineDeleteCollectionParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetSvmUUID(svmUUID string) {
	o.SvmUUID = svmUUID
}

// WithType adds the typeVar to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) WithType(typeVar *string) *FpolicyEngineDeleteCollectionParams {
	o.SetType(typeVar)
	return o
}

// SetType adds the type to the fpolicy engine delete collection params
func (o *FpolicyEngineDeleteCollectionParams) SetType(typeVar *string) {
	o.Type = typeVar
}

// WriteToRequest writes these params to a swagger request
func (o *FpolicyEngineDeleteCollectionParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.BufferSizeRecvBuffer != nil {

		// query param buffer_size.recv_buffer
		var qrBufferSizeRecvBuffer int64

		if o.BufferSizeRecvBuffer != nil {
			qrBufferSizeRecvBuffer = *o.BufferSizeRecvBuffer
		}
		qBufferSizeRecvBuffer := swag.FormatInt64(qrBufferSizeRecvBuffer)
		if qBufferSizeRecvBuffer != "" {

			if err := r.SetQueryParam("buffer_size.recv_buffer", qBufferSizeRecvBuffer); err != nil {
				return err
			}
		}
	}

	if o.BufferSizeSendBuffer != nil {

		// query param buffer_size.send_buffer
		var qrBufferSizeSendBuffer int64

		if o.BufferSizeSendBuffer != nil {
			qrBufferSizeSendBuffer = *o.BufferSizeSendBuffer
		}
		qBufferSizeSendBuffer := swag.FormatInt64(qrBufferSizeSendBuffer)
		if qBufferSizeSendBuffer != "" {

			if err := r.SetQueryParam("buffer_size.send_buffer", qBufferSizeSendBuffer); err != nil {
				return err
			}
		}
	}

	if o.CertificateCa != nil {

		// query param certificate.ca
		var qrCertificateCa string

		if o.CertificateCa != nil {
			qrCertificateCa = *o.CertificateCa
		}
		qCertificateCa := qrCertificateCa
		if qCertificateCa != "" {

			if err := r.SetQueryParam("certificate.ca", qCertificateCa); err != nil {
				return err
			}
		}
	}

	if o.CertificateName != nil {

		// query param certificate.name
		var qrCertificateName string

		if o.CertificateName != nil {
			qrCertificateName = *o.CertificateName
		}
		qCertificateName := qrCertificateName
		if qCertificateName != "" {

			if err := r.SetQueryParam("certificate.name", qCertificateName); err != nil {
				return err
			}
		}
	}

	if o.CertificateSerialNumber != nil {

		// query param certificate.serial_number
		var qrCertificateSerialNumber string

		if o.CertificateSerialNumber != nil {
			qrCertificateSerialNumber = *o.CertificateSerialNumber
		}
		qCertificateSerialNumber := qrCertificateSerialNumber
		if qCertificateSerialNumber != "" {

			if err := r.SetQueryParam("certificate.serial_number", qCertificateSerialNumber); err != nil {
				return err
			}
		}
	}

	if o.ContinueOnFailure != nil {

		// query param continue_on_failure
		var qrContinueOnFailure bool

		if o.ContinueOnFailure != nil {
			qrContinueOnFailure = *o.ContinueOnFailure
		}
		qContinueOnFailure := swag.FormatBool(qrContinueOnFailure)
		if qContinueOnFailure != "" {

			if err := r.SetQueryParam("continue_on_failure", qContinueOnFailure); err != nil {
				return err
			}
		}
	}

	if o.Format != nil {

		// query param format
		var qrFormat string

		if o.Format != nil {
			qrFormat = *o.Format
		}
		qFormat := qrFormat
		if qFormat != "" {

			if err := r.SetQueryParam("format", qFormat); err != nil {
				return err
			}
		}
	}
	if err := r.SetBodyParam(o.Info); err != nil {
		return err
	}

	if o.KeepAliveInterval != nil {

		// query param keep_alive_interval
		var qrKeepAliveInterval string

		if o.KeepAliveInterval != nil {
			qrKeepAliveInterval = *o.KeepAliveInterval
		}
		qKeepAliveInterval := qrKeepAliveInterval
		if qKeepAliveInterval != "" {

			if err := r.SetQueryParam("keep_alive_interval", qKeepAliveInterval); err != nil {
				return err
			}
		}
	}

	if o.MaxServerRequests != nil {

		// query param max_server_requests
		var qrMaxServerRequests int64

		if o.MaxServerRequests != nil {
			qrMaxServerRequests = *o.MaxServerRequests
		}
		qMaxServerRequests := swag.FormatInt64(qrMaxServerRequests)
		if qMaxServerRequests != "" {

			if err := r.SetQueryParam("max_server_requests", qMaxServerRequests); err != nil {
				return err
			}
		}
	}

	if o.Name != nil {

		// query param name
		var qrName string

		if o.Name != nil {
			qrName = *o.Name
		}
		qName := qrName
		if qName != "" {

			if err := r.SetQueryParam("name", qName); err != nil {
				return err
			}
		}
	}

	if o.Port != nil {

		// query param port
		var qrPort int64

		if o.Port != nil {
			qrPort = *o.Port
		}
		qPort := swag.FormatInt64(qrPort)
		if qPort != "" {

			if err := r.SetQueryParam("port", qPort); err != nil {
				return err
			}
		}
	}

	if o.PrimaryServers != nil {

		// query param primary_servers
		var qrPrimaryServers string

		if o.PrimaryServers != nil {
			qrPrimaryServers = *o.PrimaryServers
		}
		qPrimaryServers := qrPrimaryServers
		if qPrimaryServers != "" {

			if err := r.SetQueryParam("primary_servers", qPrimaryServers); err != nil {
				return err
			}
		}
	}

	if o.RequestAbortTimeout != nil {

		// query param request_abort_timeout
		var qrRequestAbortTimeout string

		if o.RequestAbortTimeout != nil {
			qrRequestAbortTimeout = *o.RequestAbortTimeout
		}
		qRequestAbortTimeout := qrRequestAbortTimeout
		if qRequestAbortTimeout != "" {

			if err := r.SetQueryParam("request_abort_timeout", qRequestAbortTimeout); err != nil {
				return err
			}
		}
	}

	if o.RequestCancelTimeout != nil {

		// query param request_cancel_timeout
		var qrRequestCancelTimeout string

		if o.RequestCancelTimeout != nil {
			qrRequestCancelTimeout = *o.RequestCancelTimeout
		}
		qRequestCancelTimeout := qrRequestCancelTimeout
		if qRequestCancelTimeout != "" {

			if err := r.SetQueryParam("request_cancel_timeout", qRequestCancelTimeout); err != nil {
				return err
			}
		}
	}

	if o.ResiliencyDirectoryPath != nil {

		// query param resiliency.directory_path
		var qrResiliencyDirectoryPath string

		if o.ResiliencyDirectoryPath != nil {
			qrResiliencyDirectoryPath = *o.ResiliencyDirectoryPath
		}
		qResiliencyDirectoryPath := qrResiliencyDirectoryPath
		if qResiliencyDirectoryPath != "" {

			if err := r.SetQueryParam("resiliency.directory_path", qResiliencyDirectoryPath); err != nil {
				return err
			}
		}
	}

	if o.ResiliencyEnabled != nil {

		// query param resiliency.enabled
		var qrResiliencyEnabled bool

		if o.ResiliencyEnabled != nil {
			qrResiliencyEnabled = *o.ResiliencyEnabled
		}
		qResiliencyEnabled := swag.FormatBool(qrResiliencyEnabled)
		if qResiliencyEnabled != "" {

			if err := r.SetQueryParam("resiliency.enabled", qResiliencyEnabled); err != nil {
				return err
			}
		}
	}

	if o.ResiliencyRetentionDuration != nil {

		// query param resiliency.retention_duration
		var qrResiliencyRetentionDuration string

		if o.ResiliencyRetentionDuration != nil {
			qrResiliencyRetentionDuration = *o.ResiliencyRetentionDuration
		}
		qResiliencyRetentionDuration := qrResiliencyRetentionDuration
		if qResiliencyRetentionDuration != "" {

			if err := r.SetQueryParam("resiliency.retention_duration", qResiliencyRetentionDuration); err != nil {
				return err
			}
		}
	}

	if o.ReturnRecords != nil {

		// query param return_records
		var qrReturnRecords bool

		if o.ReturnRecords != nil {
			qrReturnRecords = *o.ReturnRecords
		}
		qReturnRecords := swag.FormatBool(qrReturnRecords)
		if qReturnRecords != "" {

			if err := r.SetQueryParam("return_records", qReturnRecords); err != nil {
				return err
			}
		}
	}

	if o.ReturnTimeout != nil {

		// query param return_timeout
		var qrReturnTimeout int64

		if o.ReturnTimeout != nil {
			qrReturnTimeout = *o.ReturnTimeout
		}
		qReturnTimeout := swag.FormatInt64(qrReturnTimeout)
		if qReturnTimeout != "" {

			if err := r.SetQueryParam("return_timeout", qReturnTimeout); err != nil {
				return err
			}
		}
	}

	if o.SecondaryServers != nil {

		// query param secondary_servers
		var qrSecondaryServers string

		if o.SecondaryServers != nil {
			qrSecondaryServers = *o.SecondaryServers
		}
		qSecondaryServers := qrSecondaryServers
		if qSecondaryServers != "" {

			if err := r.SetQueryParam("secondary_servers", qSecondaryServers); err != nil {
				return err
			}
		}
	}

	if o.SerialRecords != nil {

		// query param serial_records
		var qrSerialRecords bool

		if o.SerialRecords != nil {
			qrSerialRecords = *o.SerialRecords
		}
		qSerialRecords := swag.FormatBool(qrSerialRecords)
		if qSerialRecords != "" {

			if err := r.SetQueryParam("serial_records", qSerialRecords); err != nil {
				return err
			}
		}
	}

	if o.ServerProgressTimeout != nil {

		// query param server_progress_timeout
		var qrServerProgressTimeout string

		if o.ServerProgressTimeout != nil {
			qrServerProgressTimeout = *o.ServerProgressTimeout
		}
		qServerProgressTimeout := qrServerProgressTimeout
		if qServerProgressTimeout != "" {

			if err := r.SetQueryParam("server_progress_timeout", qServerProgressTimeout); err != nil {
				return err
			}
		}
	}

	if o.SslOption != nil {

		// query param ssl_option
		var qrSslOption string

		if o.SslOption != nil {
			qrSslOption = *o.SslOption
		}
		qSslOption := qrSslOption
		if qSslOption != "" {

			if err := r.SetQueryParam("ssl_option", qSslOption); err != nil {
				return err
			}
		}
	}

	if o.StatusRequestInterval != nil {

		// query param status_request_interval
		var qrStatusRequestInterval string

		if o.StatusRequestInterval != nil {
			qrStatusRequestInterval = *o.StatusRequestInterval
		}
		qStatusRequestInterval := qrStatusRequestInterval
		if qStatusRequestInterval != "" {

			if err := r.SetQueryParam("status_request_interval", qStatusRequestInterval); err != nil {
				return err
			}
		}
	}

	// path param svm.uuid
	if err := r.SetPathParam("svm.uuid", o.SvmUUID); err != nil {
		return err
	}

	if o.Type != nil {

		// query param type
		var qrType string

		if o.Type != nil {
			qrType = *o.Type
		}
		qType := qrType
		if qType != "" {

			if err := r.SetQueryParam("type", qType); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}