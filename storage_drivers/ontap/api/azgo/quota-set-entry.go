// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// QuotaSetEntryRequest is a structure to represent a quota-set-entry ZAPI request object
type QuotaSetEntryRequest struct {
	XMLName xml.Name `xml:"quota-set-entry"`

	DiskLimitPtr          *string `xml:"disk-limit"`
	FileLimitPtr          *string `xml:"file-limit"`
	PerformUserMappingPtr *bool   `xml:"perform-user-mapping"`
	PolicyPtr             *string `xml:"policy"`
	QtreePtr              *string `xml:"qtree"`
	QuotaTargetPtr        *string `xml:"quota-target"`
	QuotaTypePtr          *string `xml:"quota-type"`
	SoftDiskLimitPtr      *string `xml:"soft-disk-limit"`
	SoftFileLimitPtr      *string `xml:"soft-file-limit"`
	ThresholdPtr          *string `xml:"threshold"`
	VolumePtr             *string `xml:"volume"`
}

// ToXML converts this object into an xml string representation
func (o *QuotaSetEntryRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewQuotaSetEntryRequest is a factory method for creating new instances of QuotaSetEntryRequest objects
func NewQuotaSetEntryRequest() *QuotaSetEntryRequest { return &QuotaSetEntryRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *QuotaSetEntryRequest) ExecuteUsing(zr *ZapiRunner) (QuotaSetEntryResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "QuotaSetEntryRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return QuotaSetEntryResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return QuotaSetEntryResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n QuotaSetEntryResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return QuotaSetEntryResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("quota-set-entry result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaSetEntryRequest) String() string {
	var buffer bytes.Buffer
	if o.DiskLimitPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "disk-limit", *o.DiskLimitPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("disk-limit: nil\n"))
	}
	if o.FileLimitPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "file-limit", *o.FileLimitPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("file-limit: nil\n"))
	}
	if o.PerformUserMappingPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "perform-user-mapping", *o.PerformUserMappingPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("perform-user-mapping: nil\n"))
	}
	if o.PolicyPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "policy", *o.PolicyPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("policy: nil\n"))
	}
	if o.QtreePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "qtree", *o.QtreePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("qtree: nil\n"))
	}
	if o.QuotaTargetPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "quota-target", *o.QuotaTargetPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("quota-target: nil\n"))
	}
	if o.QuotaTypePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "quota-type", *o.QuotaTypePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("quota-type: nil\n"))
	}
	if o.SoftDiskLimitPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "soft-disk-limit", *o.SoftDiskLimitPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("soft-disk-limit: nil\n"))
	}
	if o.SoftFileLimitPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "soft-file-limit", *o.SoftFileLimitPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("soft-file-limit: nil\n"))
	}
	if o.ThresholdPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "threshold", *o.ThresholdPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("threshold: nil\n"))
	}
	if o.VolumePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume", *o.VolumePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume: nil\n"))
	}
	return buffer.String()
}

// DiskLimit is a fluent style 'getter' method that can be chained
func (o *QuotaSetEntryRequest) DiskLimit() string {
	r := *o.DiskLimitPtr
	return r
}

// SetDiskLimit is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetDiskLimit(newValue string) *QuotaSetEntryRequest {
	o.DiskLimitPtr = &newValue
	return o
}

// FileLimit is a fluent style 'getter' method that can be chained
func (o *QuotaSetEntryRequest) FileLimit() string {
	r := *o.FileLimitPtr
	return r
}

// SetFileLimit is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetFileLimit(newValue string) *QuotaSetEntryRequest {
	o.FileLimitPtr = &newValue
	return o
}

// PerformUserMapping is a fluent style 'getter' method that can be chained
func (o *QuotaSetEntryRequest) PerformUserMapping() bool {
	r := *o.PerformUserMappingPtr
	return r
}

// SetPerformUserMapping is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetPerformUserMapping(newValue bool) *QuotaSetEntryRequest {
	o.PerformUserMappingPtr = &newValue
	return o
}

// Policy is a fluent style 'getter' method that can be chained
func (o *QuotaSetEntryRequest) Policy() string {
	r := *o.PolicyPtr
	return r
}

// SetPolicy is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetPolicy(newValue string) *QuotaSetEntryRequest {
	o.PolicyPtr = &newValue
	return o
}

// Qtree is a fluent style 'getter' method that can be chained
func (o *QuotaSetEntryRequest) Qtree() string {
	r := *o.QtreePtr
	return r
}

// SetQtree is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetQtree(newValue string) *QuotaSetEntryRequest {
	o.QtreePtr = &newValue
	return o
}

// QuotaTarget is a fluent style 'getter' method that can be chained
func (o *QuotaSetEntryRequest) QuotaTarget() string {
	r := *o.QuotaTargetPtr
	return r
}

// SetQuotaTarget is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetQuotaTarget(newValue string) *QuotaSetEntryRequest {
	o.QuotaTargetPtr = &newValue
	return o
}

// QuotaType is a fluent style 'getter' method that can be chained
func (o *QuotaSetEntryRequest) QuotaType() string {
	r := *o.QuotaTypePtr
	return r
}

// SetQuotaType is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetQuotaType(newValue string) *QuotaSetEntryRequest {
	o.QuotaTypePtr = &newValue
	return o
}

// SoftDiskLimit is a fluent style 'getter' method that can be chained
func (o *QuotaSetEntryRequest) SoftDiskLimit() string {
	r := *o.SoftDiskLimitPtr
	return r
}

// SetSoftDiskLimit is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetSoftDiskLimit(newValue string) *QuotaSetEntryRequest {
	o.SoftDiskLimitPtr = &newValue
	return o
}

// SoftFileLimit is a fluent style 'getter' method that can be chained
func (o *QuotaSetEntryRequest) SoftFileLimit() string {
	r := *o.SoftFileLimitPtr
	return r
}

// SetSoftFileLimit is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetSoftFileLimit(newValue string) *QuotaSetEntryRequest {
	o.SoftFileLimitPtr = &newValue
	return o
}

// Threshold is a fluent style 'getter' method that can be chained
func (o *QuotaSetEntryRequest) Threshold() string {
	r := *o.ThresholdPtr
	return r
}

// SetThreshold is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetThreshold(newValue string) *QuotaSetEntryRequest {
	o.ThresholdPtr = &newValue
	return o
}

// Volume is a fluent style 'getter' method that can be chained
func (o *QuotaSetEntryRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *QuotaSetEntryRequest) SetVolume(newValue string) *QuotaSetEntryRequest {
	o.VolumePtr = &newValue
	return o
}

// QuotaSetEntryResponse is a structure to represent a quota-set-entry ZAPI response object
type QuotaSetEntryResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result QuotaSetEntryResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaSetEntryResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// QuotaSetEntryResponseResult is a structure to represent a quota-set-entry ZAPI object's result
type QuotaSetEntryResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
}

// ToXML converts this object into an xml string representation
func (o *QuotaSetEntryResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewQuotaSetEntryResponse is a factory method for creating new instances of QuotaSetEntryResponse objects
func NewQuotaSetEntryResponse() *QuotaSetEntryResponse { return &QuotaSetEntryResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaSetEntryResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	return buffer.String()
}
