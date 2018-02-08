// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/Sirupsen/logrus"
)

// SnapmirrorUpdateLsSetRequest is a structure to represent a snapmirror-update-ls-set ZAPI request object
type SnapmirrorUpdateLsSetRequest struct {
	XMLName xml.Name `xml:"snapmirror-update-ls-set"`

	SourceClusterPtr  *string `xml:"source-cluster"`
	SourceLocationPtr *string `xml:"source-location"`
	SourceVolumePtr   *string `xml:"source-volume"`
	SourceVserverPtr  *string `xml:"source-vserver"`
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorUpdateLsSetRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewSnapmirrorUpdateLsSetRequest is a factory method for creating new instances of SnapmirrorUpdateLsSetRequest objects
func NewSnapmirrorUpdateLsSetRequest() *SnapmirrorUpdateLsSetRequest {
	return &SnapmirrorUpdateLsSetRequest{}
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *SnapmirrorUpdateLsSetRequest) ExecuteUsing(zr *ZapiRunner) (SnapmirrorUpdateLsSetResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "SnapmirrorUpdateLsSetRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return SnapmirrorUpdateLsSetResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return SnapmirrorUpdateLsSetResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n SnapmirrorUpdateLsSetResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return SnapmirrorUpdateLsSetResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("snapmirror-update-ls-set result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorUpdateLsSetRequest) String() string {
	var buffer bytes.Buffer
	if o.SourceClusterPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "source-cluster", *o.SourceClusterPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("source-cluster: nil\n"))
	}
	if o.SourceLocationPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "source-location", *o.SourceLocationPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("source-location: nil\n"))
	}
	if o.SourceVolumePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "source-volume", *o.SourceVolumePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("source-volume: nil\n"))
	}
	if o.SourceVserverPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "source-vserver", *o.SourceVserverPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("source-vserver: nil\n"))
	}
	return buffer.String()
}

// SourceCluster is a fluent style 'getter' method that can be chained
func (o *SnapmirrorUpdateLsSetRequest) SourceCluster() string {
	r := *o.SourceClusterPtr
	return r
}

// SetSourceCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateLsSetRequest) SetSourceCluster(newValue string) *SnapmirrorUpdateLsSetRequest {
	o.SourceClusterPtr = &newValue
	return o
}

// SourceLocation is a fluent style 'getter' method that can be chained
func (o *SnapmirrorUpdateLsSetRequest) SourceLocation() string {
	r := *o.SourceLocationPtr
	return r
}

// SetSourceLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateLsSetRequest) SetSourceLocation(newValue string) *SnapmirrorUpdateLsSetRequest {
	o.SourceLocationPtr = &newValue
	return o
}

// SourceVolume is a fluent style 'getter' method that can be chained
func (o *SnapmirrorUpdateLsSetRequest) SourceVolume() string {
	r := *o.SourceVolumePtr
	return r
}

// SetSourceVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateLsSetRequest) SetSourceVolume(newValue string) *SnapmirrorUpdateLsSetRequest {
	o.SourceVolumePtr = &newValue
	return o
}

// SourceVserver is a fluent style 'getter' method that can be chained
func (o *SnapmirrorUpdateLsSetRequest) SourceVserver() string {
	r := *o.SourceVserverPtr
	return r
}

// SetSourceVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateLsSetRequest) SetSourceVserver(newValue string) *SnapmirrorUpdateLsSetRequest {
	o.SourceVserverPtr = &newValue
	return o
}

// SnapmirrorUpdateLsSetResponse is a structure to represent a snapmirror-update-ls-set ZAPI response object
type SnapmirrorUpdateLsSetResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result SnapmirrorUpdateLsSetResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorUpdateLsSetResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// SnapmirrorUpdateLsSetResponseResult is a structure to represent a snapmirror-update-ls-set ZAPI object's result
type SnapmirrorUpdateLsSetResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr      string  `xml:"status,attr"`
	ResultReasonAttr      string  `xml:"reason,attr"`
	ResultErrnoAttr       string  `xml:"errno,attr"`
	ResultErrorCodePtr    *int    `xml:"result-error-code"`
	ResultErrorMessagePtr *string `xml:"result-error-message"`
	ResultJobidPtr        *int    `xml:"result-jobid"`
	ResultStatusPtr       *string `xml:"result-status"`
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorUpdateLsSetResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewSnapmirrorUpdateLsSetResponse is a factory method for creating new instances of SnapmirrorUpdateLsSetResponse objects
func NewSnapmirrorUpdateLsSetResponse() *SnapmirrorUpdateLsSetResponse {
	return &SnapmirrorUpdateLsSetResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorUpdateLsSetResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.ResultErrorCodePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "result-error-code", *o.ResultErrorCodePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("result-error-code: nil\n"))
	}
	if o.ResultErrorMessagePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "result-error-message", *o.ResultErrorMessagePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("result-error-message: nil\n"))
	}
	if o.ResultJobidPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "result-jobid", *o.ResultJobidPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("result-jobid: nil\n"))
	}
	if o.ResultStatusPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "result-status", *o.ResultStatusPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("result-status: nil\n"))
	}
	return buffer.String()
}

// ResultErrorCode is a fluent style 'getter' method that can be chained
func (o *SnapmirrorUpdateLsSetResponseResult) ResultErrorCode() int {
	r := *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateLsSetResponseResult) SetResultErrorCode(newValue int) *SnapmirrorUpdateLsSetResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a fluent style 'getter' method that can be chained
func (o *SnapmirrorUpdateLsSetResponseResult) ResultErrorMessage() string {
	r := *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateLsSetResponseResult) SetResultErrorMessage(newValue string) *SnapmirrorUpdateLsSetResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a fluent style 'getter' method that can be chained
func (o *SnapmirrorUpdateLsSetResponseResult) ResultJobid() int {
	r := *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateLsSetResponseResult) SetResultJobid(newValue int) *SnapmirrorUpdateLsSetResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultStatus is a fluent style 'getter' method that can be chained
func (o *SnapmirrorUpdateLsSetResponseResult) ResultStatus() string {
	r := *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateLsSetResponseResult) SetResultStatus(newValue string) *SnapmirrorUpdateLsSetResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}
