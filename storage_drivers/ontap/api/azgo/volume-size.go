// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

// VolumeSizeRequest is a structure to represent a volume-size ZAPI request object
type VolumeSizeRequest struct {
	XMLName xml.Name `xml:"volume-size"`

	NewSizePtr *string `xml:"new-size"`
	VolumePtr  *string `xml:"volume"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeSizeRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewVolumeSizeRequest is a factory method for creating new instances of VolumeSizeRequest objects
func NewVolumeSizeRequest() *VolumeSizeRequest { return &VolumeSizeRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeSizeRequest) ExecuteUsing(zr *ZapiRunner) (VolumeSizeResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "VolumeSizeRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return VolumeSizeResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return VolumeSizeResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n VolumeSizeResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return VolumeSizeResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("volume-size result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeSizeRequest) String() string {
	var buffer bytes.Buffer
	if o.NewSizePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "new-size", *o.NewSizePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("new-size: nil\n"))
	}
	if o.VolumePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume", *o.VolumePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume: nil\n"))
	}
	return buffer.String()
}

// NewSize is a fluent style 'getter' method that can be chained
func (o *VolumeSizeRequest) NewSize() string {
	r := *o.NewSizePtr
	return r
}

// SetNewSize is a fluent style 'setter' method that can be chained
func (o *VolumeSizeRequest) SetNewSize(newValue string) *VolumeSizeRequest {
	o.NewSizePtr = &newValue
	return o
}

// Volume is a fluent style 'getter' method that can be chained
func (o *VolumeSizeRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *VolumeSizeRequest) SetVolume(newValue string) *VolumeSizeRequest {
	o.VolumePtr = &newValue
	return o
}

// VolumeSizeResponse is a structure to represent a volume-size ZAPI response object
type VolumeSizeResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result VolumeSizeResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeSizeResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// VolumeSizeResponseResult is a structure to represent a volume-size ZAPI object's result
type VolumeSizeResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr         string  `xml:"status,attr"`
	ResultReasonAttr         string  `xml:"reason,attr"`
	ResultErrnoAttr          string  `xml:"errno,attr"`
	IsFixedSizeFlexVolumePtr *bool   `xml:"is-fixed-size-flex-volume"`
	IsReadonlyFlexVolumePtr  *bool   `xml:"is-readonly-flex-volume"`
	IsReplicaFlexVolumePtr   *bool   `xml:"is-replica-flex-volume"`
	VolumeSizePtr            *string `xml:"volume-size"`
}

// ToXML converts this object into an xml string representation
func (o *VolumeSizeResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewVolumeSizeResponse is a factory method for creating new instances of VolumeSizeResponse objects
func NewVolumeSizeResponse() *VolumeSizeResponse { return &VolumeSizeResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeSizeResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	if o.IsFixedSizeFlexVolumePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "is-fixed-size-flex-volume", *o.IsFixedSizeFlexVolumePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("is-fixed-size-flex-volume: nil\n"))
	}
	if o.IsReadonlyFlexVolumePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "is-readonly-flex-volume", *o.IsReadonlyFlexVolumePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("is-readonly-flex-volume: nil\n"))
	}
	if o.IsReplicaFlexVolumePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "is-replica-flex-volume", *o.IsReplicaFlexVolumePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("is-replica-flex-volume: nil\n"))
	}
	if o.VolumeSizePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume-size", *o.VolumeSizePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume-size: nil\n"))
	}
	return buffer.String()
}

// IsFixedSizeFlexVolume is a fluent style 'getter' method that can be chained
func (o *VolumeSizeResponseResult) IsFixedSizeFlexVolume() bool {
	r := *o.IsFixedSizeFlexVolumePtr
	return r
}

// SetIsFixedSizeFlexVolume is a fluent style 'setter' method that can be chained
func (o *VolumeSizeResponseResult) SetIsFixedSizeFlexVolume(newValue bool) *VolumeSizeResponseResult {
	o.IsFixedSizeFlexVolumePtr = &newValue
	return o
}

// IsReadonlyFlexVolume is a fluent style 'getter' method that can be chained
func (o *VolumeSizeResponseResult) IsReadonlyFlexVolume() bool {
	r := *o.IsReadonlyFlexVolumePtr
	return r
}

// SetIsReadonlyFlexVolume is a fluent style 'setter' method that can be chained
func (o *VolumeSizeResponseResult) SetIsReadonlyFlexVolume(newValue bool) *VolumeSizeResponseResult {
	o.IsReadonlyFlexVolumePtr = &newValue
	return o
}

// IsReplicaFlexVolume is a fluent style 'getter' method that can be chained
func (o *VolumeSizeResponseResult) IsReplicaFlexVolume() bool {
	r := *o.IsReplicaFlexVolumePtr
	return r
}

// SetIsReplicaFlexVolume is a fluent style 'setter' method that can be chained
func (o *VolumeSizeResponseResult) SetIsReplicaFlexVolume(newValue bool) *VolumeSizeResponseResult {
	o.IsReplicaFlexVolumePtr = &newValue
	return o
}

// VolumeSize is a fluent style 'getter' method that can be chained
func (o *VolumeSizeResponseResult) VolumeSize() string {
	r := *o.VolumeSizePtr
	return r
}

// SetVolumeSize is a fluent style 'setter' method that can be chained
func (o *VolumeSizeResponseResult) SetVolumeSize(newValue string) *VolumeSizeResponseResult {
	o.VolumeSizePtr = &newValue
	return o
}
