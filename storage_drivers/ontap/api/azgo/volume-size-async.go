// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// VolumeSizeAsyncRequest is a structure to represent a volume-size-async ZAPI request object
type VolumeSizeAsyncRequest struct {
	XMLName xml.Name `xml:"volume-size-async"`

	NewSizePtr    *string `xml:"new-size"`
	VolumeNamePtr *string `xml:"volume-name"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeSizeAsyncRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v\n", err)
	}
	return string(output), err
}

// NewVolumeSizeAsyncRequest is a factory method for creating new instances of VolumeSizeAsyncRequest objects
func NewVolumeSizeAsyncRequest() *VolumeSizeAsyncRequest { return &VolumeSizeAsyncRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeSizeAsyncRequest) ExecuteUsing(zr *ZapiRunner) (VolumeSizeAsyncResponse, error) {
	resp, err := zr.SendZapi(o)
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	log.Debugf("response Body:\n%s", string(body))

	var n VolumeSizeAsyncResponse
	xml.Unmarshal(body, &n)
	if err != nil {
		log.Errorf("err: %v", err.Error())
	}
	log.Debugf("volume-size-async result:\n%s", n.Result)

	return n, err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeSizeAsyncRequest) String() string {
	var buffer bytes.Buffer
	if o.NewSizePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "new-size", *o.NewSizePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("new-size: nil\n"))
	}
	if o.VolumeNamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume-name", *o.VolumeNamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume-name: nil\n"))
	}
	return buffer.String()
}

// NewSize is a fluent style 'getter' method that can be chained
func (o *VolumeSizeAsyncRequest) NewSize() string {
	r := *o.NewSizePtr
	return r
}

// SetNewSize is a fluent style 'setter' method that can be chained
func (o *VolumeSizeAsyncRequest) SetNewSize(newValue string) *VolumeSizeAsyncRequest {
	o.NewSizePtr = &newValue
	return o
}

// VolumeName is a fluent style 'getter' method that can be chained
func (o *VolumeSizeAsyncRequest) VolumeName() string {
	r := *o.VolumeNamePtr
	return r
}

// SetVolumeName is a fluent style 'setter' method that can be chained
func (o *VolumeSizeAsyncRequest) SetVolumeName(newValue string) *VolumeSizeAsyncRequest {
	o.VolumeNamePtr = &newValue
	return o
}

// VolumeSizeAsyncResponse is a structure to represent a volume-size-async ZAPI response object
type VolumeSizeAsyncResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result VolumeSizeAsyncResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeSizeAsyncResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// VolumeSizeAsyncResponseResult is a structure to represent a volume-size-async ZAPI object's result
type VolumeSizeAsyncResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr      string  `xml:"status,attr"`
	ResultReasonAttr      string  `xml:"reason,attr"`
	ResultErrnoAttr       string  `xml:"errno,attr"`
	ResultErrorCodePtr    *int    `xml:"result-error-code"`
	ResultErrorMessagePtr *string `xml:"result-error-message"`
	ResultJobidPtr        *int    `xml:"result-jobid"`
	ResultStatusPtr       *string `xml:"result-status"`
	VolumeSizePtr         *string `xml:"volume-size"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeSizeAsyncResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Debugf("error: %v", err)
	}
	return string(output), err
}

// NewVolumeSizeAsyncResponse is a factory method for creating new instances of VolumeSizeAsyncResponse objects
func NewVolumeSizeAsyncResponse() *VolumeSizeAsyncResponse { return &VolumeSizeAsyncResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeSizeAsyncResponseResult) String() string {
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
	if o.VolumeSizePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume-size", *o.VolumeSizePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume-size: nil\n"))
	}
	return buffer.String()
}

// ResultErrorCode is a fluent style 'getter' method that can be chained
func (o *VolumeSizeAsyncResponseResult) ResultErrorCode() int {
	r := *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *VolumeSizeAsyncResponseResult) SetResultErrorCode(newValue int) *VolumeSizeAsyncResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a fluent style 'getter' method that can be chained
func (o *VolumeSizeAsyncResponseResult) ResultErrorMessage() string {
	r := *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *VolumeSizeAsyncResponseResult) SetResultErrorMessage(newValue string) *VolumeSizeAsyncResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a fluent style 'getter' method that can be chained
func (o *VolumeSizeAsyncResponseResult) ResultJobid() int {
	r := *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *VolumeSizeAsyncResponseResult) SetResultJobid(newValue int) *VolumeSizeAsyncResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultStatus is a fluent style 'getter' method that can be chained
func (o *VolumeSizeAsyncResponseResult) ResultStatus() string {
	r := *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *VolumeSizeAsyncResponseResult) SetResultStatus(newValue string) *VolumeSizeAsyncResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}

// VolumeSize is a fluent style 'getter' method that can be chained
func (o *VolumeSizeAsyncResponseResult) VolumeSize() string {
	r := *o.VolumeSizePtr
	return r
}

// SetVolumeSize is a fluent style 'setter' method that can be chained
func (o *VolumeSizeAsyncResponseResult) SetVolumeSize(newValue string) *VolumeSizeAsyncResponseResult {
	o.VolumeSizePtr = &newValue
	return o
}
