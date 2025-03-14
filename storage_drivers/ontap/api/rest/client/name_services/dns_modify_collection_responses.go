// Code generated by go-swagger; DO NOT EDIT.

package name_services

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
	"github.com/go-openapi/validate"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// DNSModifyCollectionReader is a Reader for the DNSModifyCollection structure.
type DNSModifyCollectionReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *DNSModifyCollectionReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewDNSModifyCollectionOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewDNSModifyCollectionDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewDNSModifyCollectionOK creates a DNSModifyCollectionOK with default headers values
func NewDNSModifyCollectionOK() *DNSModifyCollectionOK {
	return &DNSModifyCollectionOK{}
}

/*
DNSModifyCollectionOK describes a response with status code 200, with default header values.

OK
*/
type DNSModifyCollectionOK struct {
}

// IsSuccess returns true when this dns modify collection o k response has a 2xx status code
func (o *DNSModifyCollectionOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this dns modify collection o k response has a 3xx status code
func (o *DNSModifyCollectionOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this dns modify collection o k response has a 4xx status code
func (o *DNSModifyCollectionOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this dns modify collection o k response has a 5xx status code
func (o *DNSModifyCollectionOK) IsServerError() bool {
	return false
}

// IsCode returns true when this dns modify collection o k response a status code equal to that given
func (o *DNSModifyCollectionOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the dns modify collection o k response
func (o *DNSModifyCollectionOK) Code() int {
	return 200
}

func (o *DNSModifyCollectionOK) Error() string {
	return fmt.Sprintf("[PATCH /name-services/dns][%d] dnsModifyCollectionOK", 200)
}

func (o *DNSModifyCollectionOK) String() string {
	return fmt.Sprintf("[PATCH /name-services/dns][%d] dnsModifyCollectionOK", 200)
}

func (o *DNSModifyCollectionOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewDNSModifyCollectionDefault creates a DNSModifyCollectionDefault with default headers values
func NewDNSModifyCollectionDefault(code int) *DNSModifyCollectionDefault {
	return &DNSModifyCollectionDefault{
		_statusCode: code,
	}
}

/*
	DNSModifyCollectionDefault describes a response with status code -1, with default header values.

	ONTAP Error Response Codes

| Error Code | Description |
| ---------- | ----------- |
| 8847360    | Only admin or data SVMs allowed |
| 8847361    | Exceeded the maximum number of domains allowed. Maximum of six domains only |
| 8847362    | Exceeded the maximum number of name servers allowed. Maximum of three name servers only |
| 8847376    | FQDN is mandatory if dynamic DNS update is being enabled. |
| 8847380    | Secure updates can be enabled only after a CIFS server or an Active Directory account has been created for the SVM. |
| 8847381    | A unique FQDN must be specified for each SVM. |
| 8847383    | The specified TTL exceeds the maximum supported value of 720 hours. |
| 8847392    | Domain name cannot be an IP address |
| 8847393    | Top level domain name is invalid |
| 8847394    | FQDN name violated the limitations |
| 8847399    | One or more of the specified DNS servers do not exist or cannot be reached |
| 8847404    | Dynamic DNS is applicable only for data SVMs |
| 8847405    | DNS parameters updated successfully; however the update of Dynamic DNS-related parameters has failed. |
| 9240587    | FQDN name cannot be empty |
| 9240588    | FQDN name is too long. Maximum supported length: 255 characters  |
| 9240590    | FQDN name is reserved. Following names are reserved: "all", "local" and "localhost" |
| 9240607    | One of the FQDN labels is too long. Maximum supported length is 63 characters |
| 9240608    | FQDN does not contain a ".". At least one "." is mandatory for an FQDN. |
| 9240607    | A label of the FQDN is too long (73 characters). Maximum supported length for each label is 63 characters. |
| 23724130   | Cannot use an IPv6 name server address because there are no IPv6 LIFs |
*/
type DNSModifyCollectionDefault struct {
	_statusCode int

	Payload *models.ErrorResponse
}

// IsSuccess returns true when this dns modify collection default response has a 2xx status code
func (o *DNSModifyCollectionDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this dns modify collection default response has a 3xx status code
func (o *DNSModifyCollectionDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this dns modify collection default response has a 4xx status code
func (o *DNSModifyCollectionDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this dns modify collection default response has a 5xx status code
func (o *DNSModifyCollectionDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this dns modify collection default response a status code equal to that given
func (o *DNSModifyCollectionDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the dns modify collection default response
func (o *DNSModifyCollectionDefault) Code() int {
	return o._statusCode
}

func (o *DNSModifyCollectionDefault) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /name-services/dns][%d] dns_modify_collection default %s", o._statusCode, payload)
}

func (o *DNSModifyCollectionDefault) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[PATCH /name-services/dns][%d] dns_modify_collection default %s", o._statusCode, payload)
}

func (o *DNSModifyCollectionDefault) GetPayload() *models.ErrorResponse {
	return o.Payload
}

func (o *DNSModifyCollectionDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*
DNSModifyCollectionBody DNS modify collection body
swagger:model DNSModifyCollectionBody
*/
type DNSModifyCollectionBody struct {

	// links
	Links *models.DNSInlineLinks `json:"_links,omitempty"`

	// Number of attempts allowed when querying the DNS name servers.
	//
	// Example: 1
	// Maximum: 4
	// Minimum: 1
	Attempts *int64 `json:"attempts,omitempty"`

	// List of IP addresses for a DNS service. Addresses can be IPv4, IPv6 or both.
	//
	// Example: ["10.224.65.20","2001:db08:a0b:12f0::1"]
	// Read Only: true
	DNSInlineServiceIps []*string `json:"service_ips,omitempty"`

	// Status of all the DNS name servers configured for the specified SVM.
	//
	// Read Only: true
	DNSInlineStatus []*models.Status `json:"status,omitempty"`

	// dns response inline records
	DNSResponseInlineRecords []*models.DNS `json:"records,omitempty"`

	// domains
	Domains models.DNSDomainsArrayInline `json:"domains"`

	// dynamic dns
	DynamicDNS *models.DNSInlineDynamicDNS `json:"dynamic_dns,omitempty"`

	// Indicates whether or not the query section of the reply packet is equal to that of the query packet.
	//
	PacketQueryMatch *bool `json:"packet_query_match,omitempty"`

	// Set to "svm" for DNS owned by an SVM, otherwise set to "cluster".
	//
	// Enum: ["svm","cluster"]
	Scope *string `json:"scope,omitempty"`

	// servers
	Servers models.NameServersArrayInline `json:"servers"`

	// Indicates whether or not the validation for the specified DNS configuration is disabled.
	//
	SkipConfigValidation *bool `json:"skip_config_validation,omitempty"`

	// Indicates whether or not the DNS responses are from a different IP address to the IP address the request was sent to.
	//
	SourceAddressMatch *bool `json:"source_address_match,omitempty"`

	// svm
	Svm *models.DNSInlineSvm `json:"svm,omitempty"`

	// Timeout values for queries to the name servers, in seconds.
	//
	// Example: 2
	// Maximum: 5
	// Minimum: 1
	Timeout *int64 `json:"timeout,omitempty"`

	// Enable or disable top-level domain (TLD) queries.
	//
	TldQueryEnabled *bool `json:"tld_query_enabled,omitempty"`

	// UUID of the DNS object.
	//
	// Example: 02c9e252-41be-11e9-81d5-00a0986138f7
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this DNS modify collection body
func (o *DNSModifyCollectionBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateAttempts(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateDNSInlineStatus(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateDNSResponseInlineRecords(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateDomains(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateDynamicDNS(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateScope(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateServers(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateSvm(formats); err != nil {
		res = append(res, err)
	}

	if err := o.validateTimeout(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *DNSModifyCollectionBody) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(o.Links) { // not required
		return nil
	}

	if o.Links != nil {
		if err := o.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

func (o *DNSModifyCollectionBody) validateAttempts(formats strfmt.Registry) error {
	if swag.IsZero(o.Attempts) { // not required
		return nil
	}

	if err := validate.MinimumInt("info"+"."+"attempts", "body", *o.Attempts, 1, false); err != nil {
		return err
	}

	if err := validate.MaximumInt("info"+"."+"attempts", "body", *o.Attempts, 4, false); err != nil {
		return err
	}

	return nil
}

func (o *DNSModifyCollectionBody) validateDNSInlineStatus(formats strfmt.Registry) error {
	if swag.IsZero(o.DNSInlineStatus) { // not required
		return nil
	}

	for i := 0; i < len(o.DNSInlineStatus); i++ {
		if swag.IsZero(o.DNSInlineStatus[i]) { // not required
			continue
		}

		if o.DNSInlineStatus[i] != nil {
			if err := o.DNSInlineStatus[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "status" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *DNSModifyCollectionBody) validateDNSResponseInlineRecords(formats strfmt.Registry) error {
	if swag.IsZero(o.DNSResponseInlineRecords) { // not required
		return nil
	}

	for i := 0; i < len(o.DNSResponseInlineRecords); i++ {
		if swag.IsZero(o.DNSResponseInlineRecords[i]) { // not required
			continue
		}

		if o.DNSResponseInlineRecords[i] != nil {
			if err := o.DNSResponseInlineRecords[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *DNSModifyCollectionBody) validateDomains(formats strfmt.Registry) error {
	if swag.IsZero(o.Domains) { // not required
		return nil
	}

	if err := o.Domains.Validate(formats); err != nil {
		if ve, ok := err.(*errors.Validation); ok {
			return ve.ValidateName("info" + "." + "domains")
		}
		return err
	}

	return nil
}

func (o *DNSModifyCollectionBody) validateDynamicDNS(formats strfmt.Registry) error {
	if swag.IsZero(o.DynamicDNS) { // not required
		return nil
	}

	if o.DynamicDNS != nil {
		if err := o.DynamicDNS.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "dynamic_dns")
			}
			return err
		}
	}

	return nil
}

var dnsModifyCollectionBodyTypeScopePropEnum []interface{}

func init() {
	var res []string
	if err := json.Unmarshal([]byte(`["svm","cluster"]`), &res); err != nil {
		panic(err)
	}
	for _, v := range res {
		dnsModifyCollectionBodyTypeScopePropEnum = append(dnsModifyCollectionBodyTypeScopePropEnum, v)
	}
}

const (

	// BEGIN DEBUGGING
	// DNSModifyCollectionBody
	// DNSModifyCollectionBody
	// scope
	// Scope
	// svm
	// END DEBUGGING
	// DNSModifyCollectionBodyScopeSvm captures enum value "svm"
	DNSModifyCollectionBodyScopeSvm string = "svm"

	// BEGIN DEBUGGING
	// DNSModifyCollectionBody
	// DNSModifyCollectionBody
	// scope
	// Scope
	// cluster
	// END DEBUGGING
	// DNSModifyCollectionBodyScopeCluster captures enum value "cluster"
	DNSModifyCollectionBodyScopeCluster string = "cluster"
)

// prop value enum
func (o *DNSModifyCollectionBody) validateScopeEnum(path, location string, value string) error {
	if err := validate.EnumCase(path, location, value, dnsModifyCollectionBodyTypeScopePropEnum, true); err != nil {
		return err
	}
	return nil
}

func (o *DNSModifyCollectionBody) validateScope(formats strfmt.Registry) error {
	if swag.IsZero(o.Scope) { // not required
		return nil
	}

	// value enum
	if err := o.validateScopeEnum("info"+"."+"scope", "body", *o.Scope); err != nil {
		return err
	}

	return nil
}

func (o *DNSModifyCollectionBody) validateServers(formats strfmt.Registry) error {
	if swag.IsZero(o.Servers) { // not required
		return nil
	}

	if err := o.Servers.Validate(formats); err != nil {
		if ve, ok := err.(*errors.Validation); ok {
			return ve.ValidateName("info" + "." + "servers")
		}
		return err
	}

	return nil
}

func (o *DNSModifyCollectionBody) validateSvm(formats strfmt.Registry) error {
	if swag.IsZero(o.Svm) { // not required
		return nil
	}

	if o.Svm != nil {
		if err := o.Svm.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "svm")
			}
			return err
		}
	}

	return nil
}

func (o *DNSModifyCollectionBody) validateTimeout(formats strfmt.Registry) error {
	if swag.IsZero(o.Timeout) { // not required
		return nil
	}

	if err := validate.MinimumInt("info"+"."+"timeout", "body", *o.Timeout, 1, false); err != nil {
		return err
	}

	if err := validate.MaximumInt("info"+"."+"timeout", "body", *o.Timeout, 5, false); err != nil {
		return err
	}

	return nil
}

// ContextValidate validate this DNS modify collection body based on the context it is used
func (o *DNSModifyCollectionBody) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateDNSInlineServiceIps(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateDNSInlineStatus(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateDNSResponseInlineRecords(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateDomains(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateDynamicDNS(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateServers(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := o.contextValidateSvm(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *DNSModifyCollectionBody) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if o.Links != nil {
		if err := o.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

func (o *DNSModifyCollectionBody) contextValidateDNSInlineServiceIps(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "info"+"."+"service_ips", "body", []*string(o.DNSInlineServiceIps)); err != nil {
		return err
	}

	return nil
}

func (o *DNSModifyCollectionBody) contextValidateDNSInlineStatus(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "info"+"."+"status", "body", []*models.Status(o.DNSInlineStatus)); err != nil {
		return err
	}

	for i := 0; i < len(o.DNSInlineStatus); i++ {

		if o.DNSInlineStatus[i] != nil {
			if err := o.DNSInlineStatus[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "status" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *DNSModifyCollectionBody) contextValidateDNSResponseInlineRecords(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(o.DNSResponseInlineRecords); i++ {

		if o.DNSResponseInlineRecords[i] != nil {
			if err := o.DNSResponseInlineRecords[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("info" + "." + "records" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (o *DNSModifyCollectionBody) contextValidateDomains(ctx context.Context, formats strfmt.Registry) error {

	if err := o.Domains.ContextValidate(ctx, formats); err != nil {
		if ve, ok := err.(*errors.Validation); ok {
			return ve.ValidateName("info" + "." + "domains")
		}
		return err
	}

	return nil
}

func (o *DNSModifyCollectionBody) contextValidateDynamicDNS(ctx context.Context, formats strfmt.Registry) error {

	if o.DynamicDNS != nil {
		if err := o.DynamicDNS.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "dynamic_dns")
			}
			return err
		}
	}

	return nil
}

func (o *DNSModifyCollectionBody) contextValidateServers(ctx context.Context, formats strfmt.Registry) error {

	if err := o.Servers.ContextValidate(ctx, formats); err != nil {
		if ve, ok := err.(*errors.Validation); ok {
			return ve.ValidateName("info" + "." + "servers")
		}
		return err
	}

	return nil
}

func (o *DNSModifyCollectionBody) contextValidateSvm(ctx context.Context, formats strfmt.Registry) error {

	if o.Svm != nil {
		if err := o.Svm.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "svm")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *DNSModifyCollectionBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *DNSModifyCollectionBody) UnmarshalBinary(b []byte) error {
	var res DNSModifyCollectionBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
DNSInlineLinks dns inline links
swagger:model dns_inline__links
*/
type DNSInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this dns inline links
func (o *DNSInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *DNSInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(o.Self) { // not required
		return nil
	}

	if o.Self != nil {
		if err := o.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this dns inline links based on the context it is used
func (o *DNSInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *DNSInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if o.Self != nil {
		if err := o.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *DNSInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *DNSInlineLinks) UnmarshalBinary(b []byte) error {
	var res DNSInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
DNSInlineDynamicDNS dns inline dynamic dns
swagger:model dns_inline_dynamic_dns
*/
type DNSInlineDynamicDNS struct {

	// Enable or disable Dynamic DNS (DDNS) updates for the specified SVM.
	//
	Enabled *bool `json:"enabled,omitempty"`

	// Fully Qualified Domain Name (FQDN) to be used for dynamic DNS updates.
	//
	// Example: example.com
	Fqdn *string `json:"fqdn,omitempty"`

	// Enable or disable FQDN validation.
	//
	SkipFqdnValidation *bool `json:"skip_fqdn_validation,omitempty"`

	// Time to live value for the dynamic DNS updates, in an ISO-8601 duration formatted string.
	// Maximum Time To Live is 720 hours(P30D in ISO-8601 format) and the default is 24 hours(P1D in ISO-8601 format).
	//
	// Example: P2D
	TimeToLive *string `json:"time_to_live,omitempty"`

	// Enable or disable secure dynamic DNS updates for the specified SVM.
	//
	UseSecure *bool `json:"use_secure,omitempty"`
}

// Validate validates this dns inline dynamic dns
func (o *DNSInlineDynamicDNS) Validate(formats strfmt.Registry) error {
	return nil
}

// ContextValidate validates this dns inline dynamic dns based on context it is used
func (o *DNSInlineDynamicDNS) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	return nil
}

// MarshalBinary interface implementation
func (o *DNSInlineDynamicDNS) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *DNSInlineDynamicDNS) UnmarshalBinary(b []byte) error {
	var res DNSInlineDynamicDNS
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
DNSInlineSvm SVM, applies only to SVM-scoped objects.
swagger:model dns_inline_svm
*/
type DNSInlineSvm struct {

	// links
	Links *models.DNSInlineSvmInlineLinks `json:"_links,omitempty"`

	// The name of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: svm1
	Name *string `json:"name,omitempty"`

	// The unique identifier of the SVM. This field cannot be specified in a PATCH method.
	//
	// Example: 02c9e252-41be-11e9-81d5-00a0986138f7
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this dns inline svm
func (o *DNSInlineSvm) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *DNSInlineSvm) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(o.Links) { // not required
		return nil
	}

	if o.Links != nil {
		if err := o.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "svm" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this dns inline svm based on the context it is used
func (o *DNSInlineSvm) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *DNSInlineSvm) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if o.Links != nil {
		if err := o.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "svm" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *DNSInlineSvm) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *DNSInlineSvm) UnmarshalBinary(b []byte) error {
	var res DNSInlineSvm
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*
DNSInlineSvmInlineLinks dns inline svm inline links
swagger:model dns_inline_svm_inline__links
*/
type DNSInlineSvmInlineLinks struct {

	// self
	Self *models.Href `json:"self,omitempty"`
}

// Validate validates this dns inline svm inline links
func (o *DNSInlineSvmInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *DNSInlineSvmInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(o.Self) { // not required
		return nil
	}

	if o.Self != nil {
		if err := o.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "svm" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this dns inline svm inline links based on the context it is used
func (o *DNSInlineSvmInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := o.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *DNSInlineSvmInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if o.Self != nil {
		if err := o.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("info" + "." + "svm" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *DNSInlineSvmInlineLinks) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *DNSInlineSvmInlineLinks) UnmarshalBinary(b []byte) error {
	var res DNSInlineSvmInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
