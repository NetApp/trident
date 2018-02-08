// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/Sirupsen/logrus"
)

// QuotaStatusRequest is a structure to represent a quota-status ZAPI request object
type QuotaStatusRequest struct {
	XMLName xml.Name `xml:"quota-status"`

	VolumePtr *string `xml:"volume"`
}

// ToXML converts this object into an xml string representation
func (o *QuotaStatusRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewQuotaStatusRequest is a factory method for creating new instances of QuotaStatusRequest objects
func NewQuotaStatusRequest() *QuotaStatusRequest { return &QuotaStatusRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *QuotaStatusRequest) ExecuteUsing(zr *ZapiRunner) (QuotaStatusResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "QuotaStatusRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return QuotaStatusResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return QuotaStatusResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n QuotaStatusResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return QuotaStatusResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("quota-status result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaStatusRequest) String() string {
	var buffer bytes.Buffer
	if o.VolumePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume", *o.VolumePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume: nil\n"))
	}
	return buffer.String()
}

// Volume is a fluent style 'getter' method that can be chained
func (o *QuotaStatusRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *QuotaStatusRequest) SetVolume(newValue string) *QuotaStatusRequest {
	o.VolumePtr = &newValue
	return o
}

// QuotaStatusResponse is a structure to represent a quota-status ZAPI response object
type QuotaStatusResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result QuotaStatusResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaStatusResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// QuotaStatusResponseResult is a structure to represent a quota-status ZAPI object's result
type QuotaStatusResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr   string  `xml:"status,attr"`
	ResultReasonAttr   string  `xml:"reason,attr"`
	ResultErrnoAttr    string  `xml:"errno,attr"`
	PercentCompletePtr *int    `xml:"percent-complete"`
	QuotaErrorsPtr     *string `xml:"quota-errors"`
	ReasonPtr          *string `xml:"reason"`
	StatusPtr          *string `xml:"status"`
	SubstatusPtr       *string `xml:"substatus"`
}

// ToXML converts this object into an xml string representation
func (o *QuotaStatusResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewQuotaStatusResponse is a factory method for creating new instances of QuotaStatusResponse objects
func NewQuotaStatusResponse() *QuotaStatusResponse { return &QuotaStatusResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaStatusResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.PercentCompletePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "percent-complete", *o.PercentCompletePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("percent-complete: nil\n"))
	}
	if o.QuotaErrorsPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "quota-errors", *o.QuotaErrorsPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("quota-errors: nil\n"))
	}
	if o.ReasonPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "reason", *o.ReasonPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("reason: nil\n"))
	}
	if o.StatusPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "status", *o.StatusPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("status: nil\n"))
	}
	if o.SubstatusPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "substatus", *o.SubstatusPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("substatus: nil\n"))
	}
	return buffer.String()
}

// PercentComplete is a fluent style 'getter' method that can be chained
func (o *QuotaStatusResponseResult) PercentComplete() int {
	r := *o.PercentCompletePtr
	return r
}

// SetPercentComplete is a fluent style 'setter' method that can be chained
func (o *QuotaStatusResponseResult) SetPercentComplete(newValue int) *QuotaStatusResponseResult {
	o.PercentCompletePtr = &newValue
	return o
}

// QuotaErrors is a fluent style 'getter' method that can be chained
func (o *QuotaStatusResponseResult) QuotaErrors() string {
	r := *o.QuotaErrorsPtr
	return r
}

// SetQuotaErrors is a fluent style 'setter' method that can be chained
func (o *QuotaStatusResponseResult) SetQuotaErrors(newValue string) *QuotaStatusResponseResult {
	o.QuotaErrorsPtr = &newValue
	return o
}

// Reason is a fluent style 'getter' method that can be chained
func (o *QuotaStatusResponseResult) Reason() string {
	r := *o.ReasonPtr
	return r
}

// SetReason is a fluent style 'setter' method that can be chained
func (o *QuotaStatusResponseResult) SetReason(newValue string) *QuotaStatusResponseResult {
	o.ReasonPtr = &newValue
	return o
}

// Status is a fluent style 'getter' method that can be chained
func (o *QuotaStatusResponseResult) Status() string {
	r := *o.StatusPtr
	return r
}

// SetStatus is a fluent style 'setter' method that can be chained
func (o *QuotaStatusResponseResult) SetStatus(newValue string) *QuotaStatusResponseResult {
	o.StatusPtr = &newValue
	return o
}

// Substatus is a fluent style 'getter' method that can be chained
func (o *QuotaStatusResponseResult) Substatus() string {
	r := *o.SubstatusPtr
	return r
}

// SetSubstatus is a fluent style 'setter' method that can be chained
func (o *QuotaStatusResponseResult) SetSubstatus(newValue string) *QuotaStatusResponseResult {
	o.SubstatusPtr = &newValue
	return o
}
