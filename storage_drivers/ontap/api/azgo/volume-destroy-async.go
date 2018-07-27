// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// VolumeDestroyAsyncRequest is a structure to represent a volume-destroy-async ZAPI request object
type VolumeDestroyAsyncRequest struct {
	XMLName xml.Name `xml:"volume-destroy-async"`

	ForcePtr             *bool   `xml:"force"`
	UnmountAndOfflinePtr *bool   `xml:"unmount-and-offline"`
	VolumeNamePtr        *string `xml:"volume-name"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeDestroyAsyncRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v\n", err)
	}
	return string(output), err
}

// NewVolumeDestroyAsyncRequest is a factory method for creating new instances of VolumeDestroyAsyncRequest objects
func NewVolumeDestroyAsyncRequest() *VolumeDestroyAsyncRequest { return &VolumeDestroyAsyncRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeDestroyAsyncRequest) ExecuteUsing(zr *ZapiRunner) (VolumeDestroyAsyncResponse, error) {
	resp, err := zr.SendZapi(o)
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	log.Debugf("response Body:\n%s", string(body))

	var n VolumeDestroyAsyncResponse
	xml.Unmarshal(body, &n)
	if err != nil {
		log.Errorf("err: %v", err.Error())
	}
	log.Debugf("volume-destroy-async result:\n%s", n.Result)

	return n, err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeDestroyAsyncRequest) String() string {
	var buffer bytes.Buffer
	if o.ForcePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "force", *o.ForcePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("force: nil\n"))
	}
	if o.UnmountAndOfflinePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "unmount-and-offline", *o.UnmountAndOfflinePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("unmount-and-offline: nil\n"))
	}
	if o.VolumeNamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume-name", *o.VolumeNamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume-name: nil\n"))
	}
	return buffer.String()
}

// Force is a fluent style 'getter' method that can be chained
func (o *VolumeDestroyAsyncRequest) Force() bool {
	r := *o.ForcePtr
	return r
}

// SetForce is a fluent style 'setter' method that can be chained
func (o *VolumeDestroyAsyncRequest) SetForce(newValue bool) *VolumeDestroyAsyncRequest {
	o.ForcePtr = &newValue
	return o
}

// UnmountAndOffline is a fluent style 'getter' method that can be chained
func (o *VolumeDestroyAsyncRequest) UnmountAndOffline() bool {
	r := *o.UnmountAndOfflinePtr
	return r
}

// SetUnmountAndOffline is a fluent style 'setter' method that can be chained
func (o *VolumeDestroyAsyncRequest) SetUnmountAndOffline(newValue bool) *VolumeDestroyAsyncRequest {
	o.UnmountAndOfflinePtr = &newValue
	return o
}

// VolumeName is a fluent style 'getter' method that can be chained
func (o *VolumeDestroyAsyncRequest) VolumeName() string {
	r := *o.VolumeNamePtr
	return r
}

// SetVolumeName is a fluent style 'setter' method that can be chained
func (o *VolumeDestroyAsyncRequest) SetVolumeName(newValue string) *VolumeDestroyAsyncRequest {
	o.VolumeNamePtr = &newValue
	return o
}

// VolumeDestroyAsyncResponse is a structure to represent a volume-destroy-async ZAPI response object
type VolumeDestroyAsyncResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result VolumeDestroyAsyncResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeDestroyAsyncResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// VolumeDestroyAsyncResponseResult is a structure to represent a volume-destroy-async ZAPI object's result
type VolumeDestroyAsyncResponseResult struct {
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
func (o *VolumeDestroyAsyncResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Debugf("error: %v", err)
	}
	return string(output), err
}

// NewVolumeDestroyAsyncResponse is a factory method for creating new instances of VolumeDestroyAsyncResponse objects
func NewVolumeDestroyAsyncResponse() *VolumeDestroyAsyncResponse { return &VolumeDestroyAsyncResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeDestroyAsyncResponseResult) String() string {
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
func (o *VolumeDestroyAsyncResponseResult) ResultErrorCode() int {
	r := *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *VolumeDestroyAsyncResponseResult) SetResultErrorCode(newValue int) *VolumeDestroyAsyncResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a fluent style 'getter' method that can be chained
func (o *VolumeDestroyAsyncResponseResult) ResultErrorMessage() string {
	r := *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *VolumeDestroyAsyncResponseResult) SetResultErrorMessage(newValue string) *VolumeDestroyAsyncResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a fluent style 'getter' method that can be chained
func (o *VolumeDestroyAsyncResponseResult) ResultJobid() int {
	r := *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *VolumeDestroyAsyncResponseResult) SetResultJobid(newValue int) *VolumeDestroyAsyncResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultStatus is a fluent style 'getter' method that can be chained
func (o *VolumeDestroyAsyncResponseResult) ResultStatus() string {
	r := *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *VolumeDestroyAsyncResponseResult) SetResultStatus(newValue string) *VolumeDestroyAsyncResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}
