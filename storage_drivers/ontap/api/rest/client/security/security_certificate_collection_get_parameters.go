// Code generated by go-swagger; DO NOT EDIT.

package security

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

// NewSecurityCertificateCollectionGetParams creates a new SecurityCertificateCollectionGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewSecurityCertificateCollectionGetParams() *SecurityCertificateCollectionGetParams {
	return &SecurityCertificateCollectionGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewSecurityCertificateCollectionGetParamsWithTimeout creates a new SecurityCertificateCollectionGetParams object
// with the ability to set a timeout on a request.
func NewSecurityCertificateCollectionGetParamsWithTimeout(timeout time.Duration) *SecurityCertificateCollectionGetParams {
	return &SecurityCertificateCollectionGetParams{
		timeout: timeout,
	}
}

// NewSecurityCertificateCollectionGetParamsWithContext creates a new SecurityCertificateCollectionGetParams object
// with the ability to set a context for a request.
func NewSecurityCertificateCollectionGetParamsWithContext(ctx context.Context) *SecurityCertificateCollectionGetParams {
	return &SecurityCertificateCollectionGetParams{
		Context: ctx,
	}
}

// NewSecurityCertificateCollectionGetParamsWithHTTPClient creates a new SecurityCertificateCollectionGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewSecurityCertificateCollectionGetParamsWithHTTPClient(client *http.Client) *SecurityCertificateCollectionGetParams {
	return &SecurityCertificateCollectionGetParams{
		HTTPClient: client,
	}
}

/*
SecurityCertificateCollectionGetParams contains all the parameters to send to the API endpoint

	for the security certificate collection get operation.

	Typically these are written to a http.Request.
*/
type SecurityCertificateCollectionGetParams struct {

	/* AuthorityKeyIdentifier.

	   Filter by authority_key_identifier
	*/
	AuthorityKeyIdentifier *string

	/* Ca.

	   Filter by ca
	*/
	Ca *string

	/* CommonName.

	   Filter by common_name
	*/
	CommonName *string

	/* ExpiryTime.

	   Filter by expiry_time
	*/
	ExpiryTime *string

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* HashFunction.

	   Filter by hash_function
	*/
	HashFunction *string

	/* KeySize.

	   Filter by key_size
	*/
	KeySize *int64

	/* MaxRecords.

	   Limit the number of records returned.
	*/
	MaxRecords *int64

	/* Name.

	   Filter by name
	*/
	Name *string

	/* OrderBy.

	   Order results by specified fields and optional [asc|desc] direction. Default direction is 'asc' for ascending.
	*/
	OrderBy []string

	/* PublicCertificate.

	   Filter by public_certificate
	*/
	PublicCertificate *string

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

	/* Scope.

	   Filter by scope
	*/
	Scope *string

	/* SerialNumber.

	   Filter by serial_number
	*/
	SerialNumber *string

	/* SubjectAlternativesDNS.

	   Filter by subject_alternatives.dns
	*/
	SubjectAlternativesDNS *string

	/* SubjectAlternativesEmail.

	   Filter by subject_alternatives.email
	*/
	SubjectAlternativesEmail *string

	/* SubjectAlternativesIP.

	   Filter by subject_alternatives.ip
	*/
	SubjectAlternativesIP *string

	/* SubjectAlternativesURI.

	   Filter by subject_alternatives.uri
	*/
	SubjectAlternativesURI *string

	/* SubjectKeyIdentifier.

	   Filter by subject_key_identifier
	*/
	SubjectKeyIdentifier *string

	/* SvmName.

	   Filter by svm.name
	*/
	SvmName *string

	/* SvmUUID.

	   Filter by svm.uuid
	*/
	SvmUUID *string

	/* Type.

	   Filter by type
	*/
	Type *string

	/* UUID.

	   Filter by uuid
	*/
	UUID *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the security certificate collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *SecurityCertificateCollectionGetParams) WithDefaults() *SecurityCertificateCollectionGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the security certificate collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *SecurityCertificateCollectionGetParams) SetDefaults() {
	var (
		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)
	)

	val := SecurityCertificateCollectionGetParams{
		ReturnRecords: &returnRecordsDefault,
		ReturnTimeout: &returnTimeoutDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithTimeout(timeout time.Duration) *SecurityCertificateCollectionGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithContext(ctx context.Context) *SecurityCertificateCollectionGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithHTTPClient(client *http.Client) *SecurityCertificateCollectionGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithAuthorityKeyIdentifier adds the authorityKeyIdentifier to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithAuthorityKeyIdentifier(authorityKeyIdentifier *string) *SecurityCertificateCollectionGetParams {
	o.SetAuthorityKeyIdentifier(authorityKeyIdentifier)
	return o
}

// SetAuthorityKeyIdentifier adds the authorityKeyIdentifier to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetAuthorityKeyIdentifier(authorityKeyIdentifier *string) {
	o.AuthorityKeyIdentifier = authorityKeyIdentifier
}

// WithCa adds the ca to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithCa(ca *string) *SecurityCertificateCollectionGetParams {
	o.SetCa(ca)
	return o
}

// SetCa adds the ca to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetCa(ca *string) {
	o.Ca = ca
}

// WithCommonName adds the commonName to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithCommonName(commonName *string) *SecurityCertificateCollectionGetParams {
	o.SetCommonName(commonName)
	return o
}

// SetCommonName adds the commonName to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetCommonName(commonName *string) {
	o.CommonName = commonName
}

// WithExpiryTime adds the expiryTime to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithExpiryTime(expiryTime *string) *SecurityCertificateCollectionGetParams {
	o.SetExpiryTime(expiryTime)
	return o
}

// SetExpiryTime adds the expiryTime to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetExpiryTime(expiryTime *string) {
	o.ExpiryTime = expiryTime
}

// WithFields adds the fields to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithFields(fields []string) *SecurityCertificateCollectionGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithHashFunction adds the hashFunction to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithHashFunction(hashFunction *string) *SecurityCertificateCollectionGetParams {
	o.SetHashFunction(hashFunction)
	return o
}

// SetHashFunction adds the hashFunction to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetHashFunction(hashFunction *string) {
	o.HashFunction = hashFunction
}

// WithKeySize adds the keySize to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithKeySize(keySize *int64) *SecurityCertificateCollectionGetParams {
	o.SetKeySize(keySize)
	return o
}

// SetKeySize adds the keySize to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetKeySize(keySize *int64) {
	o.KeySize = keySize
}

// WithMaxRecords adds the maxRecords to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithMaxRecords(maxRecords *int64) *SecurityCertificateCollectionGetParams {
	o.SetMaxRecords(maxRecords)
	return o
}

// SetMaxRecords adds the maxRecords to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetMaxRecords(maxRecords *int64) {
	o.MaxRecords = maxRecords
}

// WithName adds the name to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithName(name *string) *SecurityCertificateCollectionGetParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetName(name *string) {
	o.Name = name
}

// WithOrderBy adds the orderBy to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithOrderBy(orderBy []string) *SecurityCertificateCollectionGetParams {
	o.SetOrderBy(orderBy)
	return o
}

// SetOrderBy adds the orderBy to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetOrderBy(orderBy []string) {
	o.OrderBy = orderBy
}

// WithPublicCertificate adds the publicCertificate to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithPublicCertificate(publicCertificate *string) *SecurityCertificateCollectionGetParams {
	o.SetPublicCertificate(publicCertificate)
	return o
}

// SetPublicCertificate adds the publicCertificate to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetPublicCertificate(publicCertificate *string) {
	o.PublicCertificate = publicCertificate
}

// WithReturnRecords adds the returnRecords to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithReturnRecords(returnRecords *bool) *SecurityCertificateCollectionGetParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithReturnTimeout(returnTimeout *int64) *SecurityCertificateCollectionGetParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithScope adds the scope to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithScope(scope *string) *SecurityCertificateCollectionGetParams {
	o.SetScope(scope)
	return o
}

// SetScope adds the scope to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetScope(scope *string) {
	o.Scope = scope
}

// WithSerialNumber adds the serialNumber to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithSerialNumber(serialNumber *string) *SecurityCertificateCollectionGetParams {
	o.SetSerialNumber(serialNumber)
	return o
}

// SetSerialNumber adds the serialNumber to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetSerialNumber(serialNumber *string) {
	o.SerialNumber = serialNumber
}

// WithSubjectAlternativesDNS adds the subjectAlternativesDNS to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithSubjectAlternativesDNS(subjectAlternativesDNS *string) *SecurityCertificateCollectionGetParams {
	o.SetSubjectAlternativesDNS(subjectAlternativesDNS)
	return o
}

// SetSubjectAlternativesDNS adds the subjectAlternativesDns to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetSubjectAlternativesDNS(subjectAlternativesDNS *string) {
	o.SubjectAlternativesDNS = subjectAlternativesDNS
}

// WithSubjectAlternativesEmail adds the subjectAlternativesEmail to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithSubjectAlternativesEmail(subjectAlternativesEmail *string) *SecurityCertificateCollectionGetParams {
	o.SetSubjectAlternativesEmail(subjectAlternativesEmail)
	return o
}

// SetSubjectAlternativesEmail adds the subjectAlternativesEmail to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetSubjectAlternativesEmail(subjectAlternativesEmail *string) {
	o.SubjectAlternativesEmail = subjectAlternativesEmail
}

// WithSubjectAlternativesIP adds the subjectAlternativesIP to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithSubjectAlternativesIP(subjectAlternativesIP *string) *SecurityCertificateCollectionGetParams {
	o.SetSubjectAlternativesIP(subjectAlternativesIP)
	return o
}

// SetSubjectAlternativesIP adds the subjectAlternativesIp to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetSubjectAlternativesIP(subjectAlternativesIP *string) {
	o.SubjectAlternativesIP = subjectAlternativesIP
}

// WithSubjectAlternativesURI adds the subjectAlternativesURI to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithSubjectAlternativesURI(subjectAlternativesURI *string) *SecurityCertificateCollectionGetParams {
	o.SetSubjectAlternativesURI(subjectAlternativesURI)
	return o
}

// SetSubjectAlternativesURI adds the subjectAlternativesUri to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetSubjectAlternativesURI(subjectAlternativesURI *string) {
	o.SubjectAlternativesURI = subjectAlternativesURI
}

// WithSubjectKeyIdentifier adds the subjectKeyIdentifier to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithSubjectKeyIdentifier(subjectKeyIdentifier *string) *SecurityCertificateCollectionGetParams {
	o.SetSubjectKeyIdentifier(subjectKeyIdentifier)
	return o
}

// SetSubjectKeyIdentifier adds the subjectKeyIdentifier to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetSubjectKeyIdentifier(subjectKeyIdentifier *string) {
	o.SubjectKeyIdentifier = subjectKeyIdentifier
}

// WithSvmName adds the svmName to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithSvmName(svmName *string) *SecurityCertificateCollectionGetParams {
	o.SetSvmName(svmName)
	return o
}

// SetSvmName adds the svmName to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetSvmName(svmName *string) {
	o.SvmName = svmName
}

// WithSvmUUID adds the svmUUID to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithSvmUUID(svmUUID *string) *SecurityCertificateCollectionGetParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetSvmUUID(svmUUID *string) {
	o.SvmUUID = svmUUID
}

// WithType adds the typeVar to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithType(typeVar *string) *SecurityCertificateCollectionGetParams {
	o.SetType(typeVar)
	return o
}

// SetType adds the type to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetType(typeVar *string) {
	o.Type = typeVar
}

// WithUUID adds the uuid to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) WithUUID(uuid *string) *SecurityCertificateCollectionGetParams {
	o.SetUUID(uuid)
	return o
}

// SetUUID adds the uuid to the security certificate collection get params
func (o *SecurityCertificateCollectionGetParams) SetUUID(uuid *string) {
	o.UUID = uuid
}

// WriteToRequest writes these params to a swagger request
func (o *SecurityCertificateCollectionGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.AuthorityKeyIdentifier != nil {

		// query param authority_key_identifier
		var qrAuthorityKeyIdentifier string

		if o.AuthorityKeyIdentifier != nil {
			qrAuthorityKeyIdentifier = *o.AuthorityKeyIdentifier
		}
		qAuthorityKeyIdentifier := qrAuthorityKeyIdentifier
		if qAuthorityKeyIdentifier != "" {

			if err := r.SetQueryParam("authority_key_identifier", qAuthorityKeyIdentifier); err != nil {
				return err
			}
		}
	}

	if o.Ca != nil {

		// query param ca
		var qrCa string

		if o.Ca != nil {
			qrCa = *o.Ca
		}
		qCa := qrCa
		if qCa != "" {

			if err := r.SetQueryParam("ca", qCa); err != nil {
				return err
			}
		}
	}

	if o.CommonName != nil {

		// query param common_name
		var qrCommonName string

		if o.CommonName != nil {
			qrCommonName = *o.CommonName
		}
		qCommonName := qrCommonName
		if qCommonName != "" {

			if err := r.SetQueryParam("common_name", qCommonName); err != nil {
				return err
			}
		}
	}

	if o.ExpiryTime != nil {

		// query param expiry_time
		var qrExpiryTime string

		if o.ExpiryTime != nil {
			qrExpiryTime = *o.ExpiryTime
		}
		qExpiryTime := qrExpiryTime
		if qExpiryTime != "" {

			if err := r.SetQueryParam("expiry_time", qExpiryTime); err != nil {
				return err
			}
		}
	}

	if o.Fields != nil {

		// binding items for fields
		joinedFields := o.bindParamFields(reg)

		// query array param fields
		if err := r.SetQueryParam("fields", joinedFields...); err != nil {
			return err
		}
	}

	if o.HashFunction != nil {

		// query param hash_function
		var qrHashFunction string

		if o.HashFunction != nil {
			qrHashFunction = *o.HashFunction
		}
		qHashFunction := qrHashFunction
		if qHashFunction != "" {

			if err := r.SetQueryParam("hash_function", qHashFunction); err != nil {
				return err
			}
		}
	}

	if o.KeySize != nil {

		// query param key_size
		var qrKeySize int64

		if o.KeySize != nil {
			qrKeySize = *o.KeySize
		}
		qKeySize := swag.FormatInt64(qrKeySize)
		if qKeySize != "" {

			if err := r.SetQueryParam("key_size", qKeySize); err != nil {
				return err
			}
		}
	}

	if o.MaxRecords != nil {

		// query param max_records
		var qrMaxRecords int64

		if o.MaxRecords != nil {
			qrMaxRecords = *o.MaxRecords
		}
		qMaxRecords := swag.FormatInt64(qrMaxRecords)
		if qMaxRecords != "" {

			if err := r.SetQueryParam("max_records", qMaxRecords); err != nil {
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

	if o.OrderBy != nil {

		// binding items for order_by
		joinedOrderBy := o.bindParamOrderBy(reg)

		// query array param order_by
		if err := r.SetQueryParam("order_by", joinedOrderBy...); err != nil {
			return err
		}
	}

	if o.PublicCertificate != nil {

		// query param public_certificate
		var qrPublicCertificate string

		if o.PublicCertificate != nil {
			qrPublicCertificate = *o.PublicCertificate
		}
		qPublicCertificate := qrPublicCertificate
		if qPublicCertificate != "" {

			if err := r.SetQueryParam("public_certificate", qPublicCertificate); err != nil {
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

	if o.Scope != nil {

		// query param scope
		var qrScope string

		if o.Scope != nil {
			qrScope = *o.Scope
		}
		qScope := qrScope
		if qScope != "" {

			if err := r.SetQueryParam("scope", qScope); err != nil {
				return err
			}
		}
	}

	if o.SerialNumber != nil {

		// query param serial_number
		var qrSerialNumber string

		if o.SerialNumber != nil {
			qrSerialNumber = *o.SerialNumber
		}
		qSerialNumber := qrSerialNumber
		if qSerialNumber != "" {

			if err := r.SetQueryParam("serial_number", qSerialNumber); err != nil {
				return err
			}
		}
	}

	if o.SubjectAlternativesDNS != nil {

		// query param subject_alternatives.dns
		var qrSubjectAlternativesDNS string

		if o.SubjectAlternativesDNS != nil {
			qrSubjectAlternativesDNS = *o.SubjectAlternativesDNS
		}
		qSubjectAlternativesDNS := qrSubjectAlternativesDNS
		if qSubjectAlternativesDNS != "" {

			if err := r.SetQueryParam("subject_alternatives.dns", qSubjectAlternativesDNS); err != nil {
				return err
			}
		}
	}

	if o.SubjectAlternativesEmail != nil {

		// query param subject_alternatives.email
		var qrSubjectAlternativesEmail string

		if o.SubjectAlternativesEmail != nil {
			qrSubjectAlternativesEmail = *o.SubjectAlternativesEmail
		}
		qSubjectAlternativesEmail := qrSubjectAlternativesEmail
		if qSubjectAlternativesEmail != "" {

			if err := r.SetQueryParam("subject_alternatives.email", qSubjectAlternativesEmail); err != nil {
				return err
			}
		}
	}

	if o.SubjectAlternativesIP != nil {

		// query param subject_alternatives.ip
		var qrSubjectAlternativesIP string

		if o.SubjectAlternativesIP != nil {
			qrSubjectAlternativesIP = *o.SubjectAlternativesIP
		}
		qSubjectAlternativesIP := qrSubjectAlternativesIP
		if qSubjectAlternativesIP != "" {

			if err := r.SetQueryParam("subject_alternatives.ip", qSubjectAlternativesIP); err != nil {
				return err
			}
		}
	}

	if o.SubjectAlternativesURI != nil {

		// query param subject_alternatives.uri
		var qrSubjectAlternativesURI string

		if o.SubjectAlternativesURI != nil {
			qrSubjectAlternativesURI = *o.SubjectAlternativesURI
		}
		qSubjectAlternativesURI := qrSubjectAlternativesURI
		if qSubjectAlternativesURI != "" {

			if err := r.SetQueryParam("subject_alternatives.uri", qSubjectAlternativesURI); err != nil {
				return err
			}
		}
	}

	if o.SubjectKeyIdentifier != nil {

		// query param subject_key_identifier
		var qrSubjectKeyIdentifier string

		if o.SubjectKeyIdentifier != nil {
			qrSubjectKeyIdentifier = *o.SubjectKeyIdentifier
		}
		qSubjectKeyIdentifier := qrSubjectKeyIdentifier
		if qSubjectKeyIdentifier != "" {

			if err := r.SetQueryParam("subject_key_identifier", qSubjectKeyIdentifier); err != nil {
				return err
			}
		}
	}

	if o.SvmName != nil {

		// query param svm.name
		var qrSvmName string

		if o.SvmName != nil {
			qrSvmName = *o.SvmName
		}
		qSvmName := qrSvmName
		if qSvmName != "" {

			if err := r.SetQueryParam("svm.name", qSvmName); err != nil {
				return err
			}
		}
	}

	if o.SvmUUID != nil {

		// query param svm.uuid
		var qrSvmUUID string

		if o.SvmUUID != nil {
			qrSvmUUID = *o.SvmUUID
		}
		qSvmUUID := qrSvmUUID
		if qSvmUUID != "" {

			if err := r.SetQueryParam("svm.uuid", qSvmUUID); err != nil {
				return err
			}
		}
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

	if o.UUID != nil {

		// query param uuid
		var qrUUID string

		if o.UUID != nil {
			qrUUID = *o.UUID
		}
		qUUID := qrUUID
		if qUUID != "" {

			if err := r.SetQueryParam("uuid", qUUID); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamSecurityCertificateCollectionGet binds the parameter fields
func (o *SecurityCertificateCollectionGetParams) bindParamFields(formats strfmt.Registry) []string {
	fieldsIR := o.Fields

	var fieldsIC []string
	for _, fieldsIIR := range fieldsIR { // explode []string

		fieldsIIV := fieldsIIR // string as string
		fieldsIC = append(fieldsIC, fieldsIIV)
	}

	// items.CollectionFormat: "csv"
	fieldsIS := swag.JoinByFormat(fieldsIC, "csv")

	return fieldsIS
}

// bindParamSecurityCertificateCollectionGet binds the parameter order_by
func (o *SecurityCertificateCollectionGetParams) bindParamOrderBy(formats strfmt.Registry) []string {
	orderByIR := o.OrderBy

	var orderByIC []string
	for _, orderByIIR := range orderByIR { // explode []string

		orderByIIV := orderByIIR // string as string
		orderByIC = append(orderByIC, orderByIIV)
	}

	// items.CollectionFormat: "csv"
	orderByIS := swag.JoinByFormat(orderByIC, "csv")

	return orderByIS
}
