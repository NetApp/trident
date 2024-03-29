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

// NewExportRuleCollectionGetParams creates a new ExportRuleCollectionGetParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewExportRuleCollectionGetParams() *ExportRuleCollectionGetParams {
	return &ExportRuleCollectionGetParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewExportRuleCollectionGetParamsWithTimeout creates a new ExportRuleCollectionGetParams object
// with the ability to set a timeout on a request.
func NewExportRuleCollectionGetParamsWithTimeout(timeout time.Duration) *ExportRuleCollectionGetParams {
	return &ExportRuleCollectionGetParams{
		timeout: timeout,
	}
}

// NewExportRuleCollectionGetParamsWithContext creates a new ExportRuleCollectionGetParams object
// with the ability to set a context for a request.
func NewExportRuleCollectionGetParamsWithContext(ctx context.Context) *ExportRuleCollectionGetParams {
	return &ExportRuleCollectionGetParams{
		Context: ctx,
	}
}

// NewExportRuleCollectionGetParamsWithHTTPClient creates a new ExportRuleCollectionGetParams object
// with the ability to set a custom HTTPClient for a request.
func NewExportRuleCollectionGetParamsWithHTTPClient(client *http.Client) *ExportRuleCollectionGetParams {
	return &ExportRuleCollectionGetParams{
		HTTPClient: client,
	}
}

/*
ExportRuleCollectionGetParams contains all the parameters to send to the API endpoint

	for the export rule collection get operation.

	Typically these are written to a http.Request.
*/
type ExportRuleCollectionGetParams struct {

	/* AllowDeviceCreation.

	   Filter by allow_device_creation
	*/
	AllowDeviceCreation *bool

	/* AllowSuid.

	   Filter by allow_suid
	*/
	AllowSuid *bool

	/* AnonymousUser.

	   Filter by anonymous_user
	*/
	AnonymousUser *string

	/* ChownMode.

	   Filter by chown_mode
	*/
	ChownMode *string

	/* ClientsMatch.

	   Filter by clients.match
	*/
	ClientsMatch *string

	/* Fields.

	   Specify the fields to return.
	*/
	Fields []string

	/* Index.

	   Filter by index
	*/
	Index *int64

	/* MaxRecords.

	   Limit the number of records returned.
	*/
	MaxRecords *int64

	/* NtfsUnixSecurity.

	   Filter by ntfs_unix_security
	*/
	NtfsUnixSecurity *string

	/* OrderBy.

	   Order results by specified fields and optional [asc|desc] direction. Default direction is 'asc' for ascending.
	*/
	OrderBy []string

	/* PolicyID.

	   Export Policy ID
	*/
	PolicyID int64

	/* PolicyName.

	   Filter by policy.name
	*/
	PolicyName *string

	/* Protocols.

	   Filter by protocols
	*/
	Protocols *string

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

	/* RoRule.

	   Filter by ro_rule
	*/
	RoRule *string

	/* RwRule.

	   Filter by rw_rule
	*/
	RwRule *string

	/* Superuser.

	   Filter by superuser
	*/
	Superuser *string

	/* SvmName.

	   Filter by svm.name
	*/
	SvmName *string

	/* SvmUUID.

	   Filter by svm.uuid
	*/
	SvmUUID *string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the export rule collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *ExportRuleCollectionGetParams) WithDefaults() *ExportRuleCollectionGetParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the export rule collection get params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *ExportRuleCollectionGetParams) SetDefaults() {
	var (
		returnRecordsDefault = bool(true)

		returnTimeoutDefault = int64(15)
	)

	val := ExportRuleCollectionGetParams{
		ReturnRecords: &returnRecordsDefault,
		ReturnTimeout: &returnTimeoutDefault,
	}

	val.timeout = o.timeout
	val.Context = o.Context
	val.HTTPClient = o.HTTPClient
	*o = val
}

// WithTimeout adds the timeout to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithTimeout(timeout time.Duration) *ExportRuleCollectionGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithContext(ctx context.Context) *ExportRuleCollectionGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithHTTPClient(client *http.Client) *ExportRuleCollectionGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithAllowDeviceCreation adds the allowDeviceCreation to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithAllowDeviceCreation(allowDeviceCreation *bool) *ExportRuleCollectionGetParams {
	o.SetAllowDeviceCreation(allowDeviceCreation)
	return o
}

// SetAllowDeviceCreation adds the allowDeviceCreation to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetAllowDeviceCreation(allowDeviceCreation *bool) {
	o.AllowDeviceCreation = allowDeviceCreation
}

// WithAllowSuid adds the allowSuid to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithAllowSuid(allowSuid *bool) *ExportRuleCollectionGetParams {
	o.SetAllowSuid(allowSuid)
	return o
}

// SetAllowSuid adds the allowSuid to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetAllowSuid(allowSuid *bool) {
	o.AllowSuid = allowSuid
}

// WithAnonymousUser adds the anonymousUser to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithAnonymousUser(anonymousUser *string) *ExportRuleCollectionGetParams {
	o.SetAnonymousUser(anonymousUser)
	return o
}

// SetAnonymousUser adds the anonymousUser to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetAnonymousUser(anonymousUser *string) {
	o.AnonymousUser = anonymousUser
}

// WithChownMode adds the chownMode to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithChownMode(chownMode *string) *ExportRuleCollectionGetParams {
	o.SetChownMode(chownMode)
	return o
}

// SetChownMode adds the chownMode to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetChownMode(chownMode *string) {
	o.ChownMode = chownMode
}

// WithClientsMatch adds the clientsMatch to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithClientsMatch(clientsMatch *string) *ExportRuleCollectionGetParams {
	o.SetClientsMatch(clientsMatch)
	return o
}

// SetClientsMatch adds the clientsMatch to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetClientsMatch(clientsMatch *string) {
	o.ClientsMatch = clientsMatch
}

// WithFields adds the fields to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithFields(fields []string) *ExportRuleCollectionGetParams {
	o.SetFields(fields)
	return o
}

// SetFields adds the fields to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetFields(fields []string) {
	o.Fields = fields
}

// WithIndex adds the index to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithIndex(index *int64) *ExportRuleCollectionGetParams {
	o.SetIndex(index)
	return o
}

// SetIndex adds the index to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetIndex(index *int64) {
	o.Index = index
}

// WithMaxRecords adds the maxRecords to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithMaxRecords(maxRecords *int64) *ExportRuleCollectionGetParams {
	o.SetMaxRecords(maxRecords)
	return o
}

// SetMaxRecords adds the maxRecords to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetMaxRecords(maxRecords *int64) {
	o.MaxRecords = maxRecords
}

// WithNtfsUnixSecurity adds the ntfsUnixSecurity to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithNtfsUnixSecurity(ntfsUnixSecurity *string) *ExportRuleCollectionGetParams {
	o.SetNtfsUnixSecurity(ntfsUnixSecurity)
	return o
}

// SetNtfsUnixSecurity adds the ntfsUnixSecurity to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetNtfsUnixSecurity(ntfsUnixSecurity *string) {
	o.NtfsUnixSecurity = ntfsUnixSecurity
}

// WithOrderBy adds the orderBy to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithOrderBy(orderBy []string) *ExportRuleCollectionGetParams {
	o.SetOrderBy(orderBy)
	return o
}

// SetOrderBy adds the orderBy to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetOrderBy(orderBy []string) {
	o.OrderBy = orderBy
}

// WithPolicyID adds the policyID to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithPolicyID(policyID int64) *ExportRuleCollectionGetParams {
	o.SetPolicyID(policyID)
	return o
}

// SetPolicyID adds the policyId to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetPolicyID(policyID int64) {
	o.PolicyID = policyID
}

// WithPolicyName adds the policyName to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithPolicyName(policyName *string) *ExportRuleCollectionGetParams {
	o.SetPolicyName(policyName)
	return o
}

// SetPolicyName adds the policyName to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetPolicyName(policyName *string) {
	o.PolicyName = policyName
}

// WithProtocols adds the protocols to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithProtocols(protocols *string) *ExportRuleCollectionGetParams {
	o.SetProtocols(protocols)
	return o
}

// SetProtocols adds the protocols to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetProtocols(protocols *string) {
	o.Protocols = protocols
}

// WithReturnRecords adds the returnRecords to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithReturnRecords(returnRecords *bool) *ExportRuleCollectionGetParams {
	o.SetReturnRecords(returnRecords)
	return o
}

// SetReturnRecords adds the returnRecords to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetReturnRecords(returnRecords *bool) {
	o.ReturnRecords = returnRecords
}

// WithReturnTimeout adds the returnTimeout to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithReturnTimeout(returnTimeout *int64) *ExportRuleCollectionGetParams {
	o.SetReturnTimeout(returnTimeout)
	return o
}

// SetReturnTimeout adds the returnTimeout to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetReturnTimeout(returnTimeout *int64) {
	o.ReturnTimeout = returnTimeout
}

// WithRoRule adds the roRule to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithRoRule(roRule *string) *ExportRuleCollectionGetParams {
	o.SetRoRule(roRule)
	return o
}

// SetRoRule adds the roRule to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetRoRule(roRule *string) {
	o.RoRule = roRule
}

// WithRwRule adds the rwRule to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithRwRule(rwRule *string) *ExportRuleCollectionGetParams {
	o.SetRwRule(rwRule)
	return o
}

// SetRwRule adds the rwRule to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetRwRule(rwRule *string) {
	o.RwRule = rwRule
}

// WithSuperuser adds the superuser to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithSuperuser(superuser *string) *ExportRuleCollectionGetParams {
	o.SetSuperuser(superuser)
	return o
}

// SetSuperuser adds the superuser to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetSuperuser(superuser *string) {
	o.Superuser = superuser
}

// WithSvmName adds the svmName to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithSvmName(svmName *string) *ExportRuleCollectionGetParams {
	o.SetSvmName(svmName)
	return o
}

// SetSvmName adds the svmName to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetSvmName(svmName *string) {
	o.SvmName = svmName
}

// WithSvmUUID adds the svmUUID to the export rule collection get params
func (o *ExportRuleCollectionGetParams) WithSvmUUID(svmUUID *string) *ExportRuleCollectionGetParams {
	o.SetSvmUUID(svmUUID)
	return o
}

// SetSvmUUID adds the svmUuid to the export rule collection get params
func (o *ExportRuleCollectionGetParams) SetSvmUUID(svmUUID *string) {
	o.SvmUUID = svmUUID
}

// WriteToRequest writes these params to a swagger request
func (o *ExportRuleCollectionGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.AllowDeviceCreation != nil {

		// query param allow_device_creation
		var qrAllowDeviceCreation bool

		if o.AllowDeviceCreation != nil {
			qrAllowDeviceCreation = *o.AllowDeviceCreation
		}
		qAllowDeviceCreation := swag.FormatBool(qrAllowDeviceCreation)
		if qAllowDeviceCreation != "" {

			if err := r.SetQueryParam("allow_device_creation", qAllowDeviceCreation); err != nil {
				return err
			}
		}
	}

	if o.AllowSuid != nil {

		// query param allow_suid
		var qrAllowSuid bool

		if o.AllowSuid != nil {
			qrAllowSuid = *o.AllowSuid
		}
		qAllowSuid := swag.FormatBool(qrAllowSuid)
		if qAllowSuid != "" {

			if err := r.SetQueryParam("allow_suid", qAllowSuid); err != nil {
				return err
			}
		}
	}

	if o.AnonymousUser != nil {

		// query param anonymous_user
		var qrAnonymousUser string

		if o.AnonymousUser != nil {
			qrAnonymousUser = *o.AnonymousUser
		}
		qAnonymousUser := qrAnonymousUser
		if qAnonymousUser != "" {

			if err := r.SetQueryParam("anonymous_user", qAnonymousUser); err != nil {
				return err
			}
		}
	}

	if o.ChownMode != nil {

		// query param chown_mode
		var qrChownMode string

		if o.ChownMode != nil {
			qrChownMode = *o.ChownMode
		}
		qChownMode := qrChownMode
		if qChownMode != "" {

			if err := r.SetQueryParam("chown_mode", qChownMode); err != nil {
				return err
			}
		}
	}

	if o.ClientsMatch != nil {

		// query param clients.match
		var qrClientsMatch string

		if o.ClientsMatch != nil {
			qrClientsMatch = *o.ClientsMatch
		}
		qClientsMatch := qrClientsMatch
		if qClientsMatch != "" {

			if err := r.SetQueryParam("clients.match", qClientsMatch); err != nil {
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

	if o.Index != nil {

		// query param index
		var qrIndex int64

		if o.Index != nil {
			qrIndex = *o.Index
		}
		qIndex := swag.FormatInt64(qrIndex)
		if qIndex != "" {

			if err := r.SetQueryParam("index", qIndex); err != nil {
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

	if o.NtfsUnixSecurity != nil {

		// query param ntfs_unix_security
		var qrNtfsUnixSecurity string

		if o.NtfsUnixSecurity != nil {
			qrNtfsUnixSecurity = *o.NtfsUnixSecurity
		}
		qNtfsUnixSecurity := qrNtfsUnixSecurity
		if qNtfsUnixSecurity != "" {

			if err := r.SetQueryParam("ntfs_unix_security", qNtfsUnixSecurity); err != nil {
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

	// path param policy.id
	if err := r.SetPathParam("policy.id", swag.FormatInt64(o.PolicyID)); err != nil {
		return err
	}

	if o.PolicyName != nil {

		// query param policy.name
		var qrPolicyName string

		if o.PolicyName != nil {
			qrPolicyName = *o.PolicyName
		}
		qPolicyName := qrPolicyName
		if qPolicyName != "" {

			if err := r.SetQueryParam("policy.name", qPolicyName); err != nil {
				return err
			}
		}
	}

	if o.Protocols != nil {

		// query param protocols
		var qrProtocols string

		if o.Protocols != nil {
			qrProtocols = *o.Protocols
		}
		qProtocols := qrProtocols
		if qProtocols != "" {

			if err := r.SetQueryParam("protocols", qProtocols); err != nil {
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

	if o.RoRule != nil {

		// query param ro_rule
		var qrRoRule string

		if o.RoRule != nil {
			qrRoRule = *o.RoRule
		}
		qRoRule := qrRoRule
		if qRoRule != "" {

			if err := r.SetQueryParam("ro_rule", qRoRule); err != nil {
				return err
			}
		}
	}

	if o.RwRule != nil {

		// query param rw_rule
		var qrRwRule string

		if o.RwRule != nil {
			qrRwRule = *o.RwRule
		}
		qRwRule := qrRwRule
		if qRwRule != "" {

			if err := r.SetQueryParam("rw_rule", qRwRule); err != nil {
				return err
			}
		}
	}

	if o.Superuser != nil {

		// query param superuser
		var qrSuperuser string

		if o.Superuser != nil {
			qrSuperuser = *o.Superuser
		}
		qSuperuser := qrSuperuser
		if qSuperuser != "" {

			if err := r.SetQueryParam("superuser", qSuperuser); err != nil {
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

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

// bindParamExportRuleCollectionGet binds the parameter fields
func (o *ExportRuleCollectionGetParams) bindParamFields(formats strfmt.Registry) []string {
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

// bindParamExportRuleCollectionGet binds the parameter order_by
func (o *ExportRuleCollectionGetParams) bindParamOrderBy(formats strfmt.Registry) []string {
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
