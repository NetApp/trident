// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/Sirupsen/logrus"
)

// SnapshotCreateRequest is a structure to represent a snapshot-create ZAPI request object
type SnapshotCreateRequest struct {
	XMLName xml.Name `xml:"snapshot-create"`

	AsyncPtr           *bool   `xml:"async"`
	CommentPtr         *string `xml:"comment"`
	SnapmirrorLabelPtr *string `xml:"snapmirror-label"`
	SnapshotPtr        *string `xml:"snapshot"`
	VolumePtr          *string `xml:"volume"`
}

// ToXML converts this object into an xml string representation
func (o *SnapshotCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewSnapshotCreateRequest is a factory method for creating new instances of SnapshotCreateRequest objects
func NewSnapshotCreateRequest() *SnapshotCreateRequest { return &SnapshotCreateRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *SnapshotCreateRequest) ExecuteUsing(zr *ZapiRunner) (SnapshotCreateResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "SnapshotCreateRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return SnapshotCreateResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return SnapshotCreateResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n SnapshotCreateResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return SnapshotCreateResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("snapshot-create result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotCreateRequest) String() string {
	var buffer bytes.Buffer
	if o.AsyncPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "async", *o.AsyncPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("async: nil\n"))
	}
	if o.CommentPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "comment", *o.CommentPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("comment: nil\n"))
	}
	if o.SnapmirrorLabelPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "snapmirror-label", *o.SnapmirrorLabelPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("snapmirror-label: nil\n"))
	}
	if o.SnapshotPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "snapshot", *o.SnapshotPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("snapshot: nil\n"))
	}
	if o.VolumePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "volume", *o.VolumePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("volume: nil\n"))
	}
	return buffer.String()
}

// Async is a fluent style 'getter' method that can be chained
func (o *SnapshotCreateRequest) Async() bool {
	r := *o.AsyncPtr
	return r
}

// SetAsync is a fluent style 'setter' method that can be chained
func (o *SnapshotCreateRequest) SetAsync(newValue bool) *SnapshotCreateRequest {
	o.AsyncPtr = &newValue
	return o
}

// Comment is a fluent style 'getter' method that can be chained
func (o *SnapshotCreateRequest) Comment() string {
	r := *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *SnapshotCreateRequest) SetComment(newValue string) *SnapshotCreateRequest {
	o.CommentPtr = &newValue
	return o
}

// SnapmirrorLabel is a fluent style 'getter' method that can be chained
func (o *SnapshotCreateRequest) SnapmirrorLabel() string {
	r := *o.SnapmirrorLabelPtr
	return r
}

// SetSnapmirrorLabel is a fluent style 'setter' method that can be chained
func (o *SnapshotCreateRequest) SetSnapmirrorLabel(newValue string) *SnapshotCreateRequest {
	o.SnapmirrorLabelPtr = &newValue
	return o
}

// Snapshot is a fluent style 'getter' method that can be chained
func (o *SnapshotCreateRequest) Snapshot() string {
	r := *o.SnapshotPtr
	return r
}

// SetSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapshotCreateRequest) SetSnapshot(newValue string) *SnapshotCreateRequest {
	o.SnapshotPtr = &newValue
	return o
}

// Volume is a fluent style 'getter' method that can be chained
func (o *SnapshotCreateRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *SnapshotCreateRequest) SetVolume(newValue string) *SnapshotCreateRequest {
	o.VolumePtr = &newValue
	return o
}

// SnapshotCreateResponse is a structure to represent a snapshot-create ZAPI response object
type SnapshotCreateResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result SnapshotCreateResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotCreateResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// SnapshotCreateResponseResult is a structure to represent a snapshot-create ZAPI object's result
type SnapshotCreateResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
}

// ToXML converts this object into an xml string representation
func (o *SnapshotCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewSnapshotCreateResponse is a factory method for creating new instances of SnapshotCreateResponse objects
func NewSnapshotCreateResponse() *SnapshotCreateResponse { return &SnapshotCreateResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotCreateResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	return buffer.String()
}
